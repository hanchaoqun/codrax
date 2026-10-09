package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func renderingFixture(t *testing.T, body string) *Index {
	t.Helper()
	p := filepath.Join(t.TempDir(), "render.systrace")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func renderingMarker(thread string, tid, tgid int, ts float64, name string) string {
	owner := "-----"
	if tgid > 0 {
		owner = fmt.Sprint(tgid)
	}
	return fmt.Sprintf("%s-%d (%s) [000] .... %.9f: tracing_mark_write: B|%d|%s\n", thread, tid, owner, ts, tid, name)
}

func renderingRun(t *testing.T, idx *Index, q Query) *RenderingCandidatesResult {
	t.Helper()
	q.View = ViewRenderingCandidates
	r := Run(idx, q)
	if r.RenderingCandidates == nil || !ValidRenderingCandidates(*r.RenderingCandidates) {
		t.Fatalf("invalid candidate result: %+v / %v", r.RenderingCandidates, r.Caveats)
	}
	if r.RootCauseRank != nil || r.ResourceStack != nil || r.CPUStateFrequency != nil {
		t.Fatal("navigation acquired other view authority")
	}
	return r.RenderingCandidates
}

func TestRenderingCandidatesPublicCoexistenceAndUnknown(t *testing.T) {
	idx := renderingFixture(t, renderingMarker("app", 11, 10, 1.001, "OnVsyncCallback")+renderingMarker("1.raster", 12, 10, 1.002, "H:flutter::GPURasterizer::Draw")+renderingMarker("web", 21, 20, 1.003, "H:ExternalBeginFrameSourceOHOS::OnVSyncImpl")+renderingMarker("rn", 31, 30, 1.004, "H:FireTriggerLifecycleFunc[RNViewBase]")+renderingMarker("kmp", 41, 40, 1.005, "Recomposer::recompose")+renderingMarker("game", 51, 50, 1.006, "Unity::PlayerLoop")+renderingMarker("ordinary", 61, 60, 1.007, "Draw FlushBuffer")+renderingMarker("plain", 71, -1, 1.008, "unrelated"))
	p := renderingRun(t, idx, Query{TimeStart: 1, TimeEnd: 1.05})
	if p.TotalCandidates != 6 {
		t.Fatalf("frameworks: %+v", p)
	}
	owners := map[int]int{}
	for _, c := range p.Candidates {
		owners[c.OwnerID]++
		if c.Framework == "HARMONY_GAME_ENGINE" && c.DefinitionStatus != "unsupported_definition" {
			t.Fatal("index-only definition claimed support")
		}
	}
	if owners[10] != 2 || owners[60] != 0 || owners[71] != 0 {
		t.Fatalf("winner/default invented: %v", owners)
	}
	q := renderingRun(t, idx, Query{PID: 71, TimeStart: 1, TimeEnd: 1.05})
	if q.TotalCandidates != 0 {
		t.Fatal("unknown defaulted to ArkUI")
	}
}

func TestRenderingCandidatesOwnerBoundaryAndNoMarkerPIDBorrowing(t *testing.T) {
	body := renderingMarker("1.ui", 11, -1, 1.001, "H:flutter::PlatformConfiguration::BeginFrame") + renderingMarker("1.raster", 12, -1, 1.002, "H:flutter::GPURasterizer::Draw") + renderingMarker("renamed", 13, 10, 1.05, "OnVsyncCallback")
	// A marker process payload does not establish the emitting thread's TGID.
	body = strings.ReplaceAll(body, "B|11|", "B|10|")
	idx := renderingFixture(t, body)
	p := renderingRun(t, idx, Query{TimeStart: 1, TimeEnd: 1.05})
	if p.TotalCandidates != 2 || p.Candidates[0].OwnerScope != TargetScopeThread || p.Candidates[0].OwnerID != 11 || p.Candidates[1].OwnerID != 12 {
		t.Fatalf("unknown owners merged or right edge included: %+v", p)
	}
	if q := renderingRun(t, idx, Query{PID: 10, TargetScope: TargetScopeProcess, TimeStart: 1, TimeEnd: 1.05}); q.TotalCandidates != 0 {
		t.Fatal("payload pid supplied process identity")
	}
	if q := renderingRun(t, idx, Query{Thread: "1.ui", TimeStart: 1, TimeEnd: 1.05}); q.TotalCandidates != 1 {
		t.Fatal("thread selector widened")
	}
	if q := renderingRun(t, idx, Query{Thread: "1.ui-11", TimeStart: 1, TimeEnd: 1.05}); q.TotalCandidates != 1 || q.Candidates[0].OwnerID != 11 {
		t.Fatal("comm-tid selector changed")
	}
	if q := renderingRun(t, idx, Query{TimeStart: 1, TimeEnd: 1.05, LineStart: 2, LineEnd: 3}); q.TotalCandidates != 1 || q.Candidates[0].OwnerID != 12 {
		t.Fatal("line/time conjunction lost")
	}
	if q := renderingRun(t, idx, Query{}); q.TotalCandidates != 3 || !q.Window.EndInclusive {
		t.Fatal("default whole-capture endpoint lost")
	}
}

func TestRenderingCandidatesSourceIsolationAndBudgets(t *testing.T) {
	idx := renderingFixture(t, renderingMarker("app", 11, 10, 1.001, "OnVsyncCallback")+renderingMarker("app", 11, 10, 1.002, "OnVsyncCallback"))
	idx.TraceArtifacts = []TraceArtifactSource{{SourcePath: "/first", VirtualLineBase: 0, LocalLineCount: 1, CausalCompatible: true}, {SourcePath: "/second", VirtualLineBase: 1, LocalLineCount: 1, CausalCompatible: true}}
	p := renderingRun(t, idx, Query{TimeStart: 1, TimeEnd: 2})
	if p.TotalCandidates != 2 || p.Candidates[0].SourcePath == p.Candidates[1].SourcePath || p.Candidates[1].Signals[0].Examples[0].SourceLine != 1 {
		t.Fatal("same numeric owner joined across sources")
	}
	idx.TraceArtifacts[1].VirtualLineBase = 0
	if q := renderingRun(t, idx, Query{TimeStart: 1, TimeEnd: 2}); q.TotalCandidates != 0 {
		t.Fatal("ambiguous source row accepted")
	}
	var text strings.Builder
	for owner := 1; owner <= 40; owner++ {
		for event := 0; event < 6; event++ {
			text.WriteString(renderingMarker("app", owner, owner, 1+float64(event)/1000, "OnVsyncCallback"))
		}
	}
	p = renderingRun(t, renderingFixture(t, text.String()), Query{TimeStart: 1, TimeEnd: 2, Limit: 2})
	if p.TotalCandidates != 40 || len(p.Candidates) != 2 || p.OmittedCandidates != 38 || p.Candidates[0].Signals[0].Count != 6 || p.Candidates[0].Signals[0].OmittedExamples != 2 {
		t.Fatalf("display was treated as total: %+v", p)
	}
	// Input order must not turn canonical top-K replacement into partial
	// occurrence counts for a retained owner.
	var reversed strings.Builder
	for owner := 40; owner >= 1; owner-- {
		for event := 0; event < 6; event++ {
			reversed.WriteString(renderingMarker("app", owner, owner, 1+float64(event)/1000, "OnVsyncCallback"))
		}
	}
	reordered := renderingRun(t, renderingFixture(t, reversed.String()), Query{TimeStart: 1, TimeEnd: 2, Limit: 2})
	if reordered.Candidates[0].OwnerID != 1 || reordered.Candidates[0].Signals[0].Count != 6 || reordered.TotalCandidates != 40 {
		t.Fatal("bounded retained payload lost earlier rows")
	}
}

func TestRenderingCandidatesPublicBundleAndCancellation(t *testing.T) {
	dir := t.TempDir()
	primary, sibling, manifest := filepath.Join(dir, "primary.systrace"), filepath.Join(dir, "sibling.systrace"), filepath.Join(dir, "capture.tracebundle.json")
	writeBundleProvenanceFixture(t, primary, renderingMarker("app", 11, 10, 1.001, "FlushMessages"))
	writeBundleProvenanceFixture(t, sibling, renderingMarker("app", 11, 10, 1.001, "flutter::Draw"))
	writeTraceBundleV2ForTest(t, manifest, []byte(`{"version":"test","systrace":"primary.systrace","artifacts":[{"type":"systrace","path":"primary.systrace"},{"type":"systrace","path":"sibling.systrace"}]}`))
	idx, err := BuildIndex(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	p := renderingRun(t, idx, Query{TimeStart: 1, TimeEnd: 2})
	if p.TotalCandidates != 1 || p.Candidates[0].Framework != "HARMONY_ARKUI" || p.Candidates[0].SourcePath != canonicalTraceIndexPath(primary) {
		t.Fatal("isolated sibling injected framework")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Run(idx, (Query{View: ViewRenderingCandidates, TimeStart: 1, TimeEnd: 2}).WithRunContext(ctx))
	if r.RenderingCandidates != nil || r.ViewCancellation == nil {
		t.Fatal("canceled partial candidates published")
	}
	// A physical row without an emitting owner cannot borrow a marker payload
	// owner; normal rows in the same file remain available.
	unknown := renderingFixture(t, "<...>-0 (-----) [000] .... 1.000000: tracing_mark_write: B|10|flutter::Draw\n"+renderingMarker("app", 11, 10, 1.001, "FlushMessages"))
	if p := renderingRun(t, unknown, Query{TimeStart: 1, TimeEnd: 2}); p.TotalCandidates != 1 || p.Candidates[0].Framework != "HARMONY_ARKUI" {
		t.Fatal("unknown emitter acquired marker owner")
	}
}

func TestRenderingCandidatesValidationAndRegistry(t *testing.T) {
	idx := renderingFixture(t, renderingMarker("1.ui", 11, 10, 1, "flutter::BeginFrame"))
	p := renderingRun(t, idx, Query{})
	data, _ := json.Marshal(p)
	for _, mutation := range []string{"owner", "time", "count", "duplicate", "definition", "role", "empty"} {
		var q RenderingCandidatesResult
		_ = json.Unmarshal(data, &q)
		switch mutation {
		case "owner":
			q.Candidates[0].OwnerID++
		case "time":
			q.Candidates[0].Signals[0].Examples[0].Ts++
		case "count":
			q.Candidates[0].Signals[0].Count++
		case "duplicate":
			q.Candidates = append(q.Candidates, q.Candidates[0])
			q.TotalCandidates++
		case "definition":
			q.Candidates[0].DefinitionStatus = "unsupported_definition"
		case "role":
			q.Candidates[0].Signals[0].RoleCandidate = "confirmed main thread"
		case "empty":
			q.Candidates[0].Signals = nil
			q.Candidates[0].TotalSignals = 0
		}
		if ValidRenderingCandidates(q) {
			t.Fatalf("accepted %s", mutation)
		}
	}
	if !reflect.DeepEqual(data, mustRenderingJSON(t, p)) {
		t.Fatal("validation mutated source")
	}
	c, err := TraceCapabilities(ViewRenderingCandidates, true)
	if err != nil || len(c.Views) != 1 || c.Views[0].View != ViewRenderingCandidates || RelationScopedView(ViewRenderingCandidates) {
		t.Fatalf("registry: %+v %v", c, err)
	}
}

func mustRenderingJSON(t *testing.T, p any) []byte {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
