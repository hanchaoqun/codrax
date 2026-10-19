package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This public RED uses only existing APIs: explicit conversion can select the
// member, but the ordinary query JSON must reach that same shared preparation.
func TestHMC263ZIPExactMemberPublicEntry(t *testing.T) {
	hmc176DisableExternalProviders(t)
	const member = "采样/正常名称.data"
	body := hmc176Zip(t, map[string][]byte{member: hmc176PerfData(true), "调度/capture.data": hmc17NamedBinaryTailFixture()})
	path := hmc176WriteSource(t, body)
	t.Run("existing_explicit_conversion", func(t *testing.T) {
		converted, err := hitraceconv.ConvertFile(context.Background(), hitraceconv.Options{InputPath: path, OutputPath: filepath.Join(t.TempDir(), "out.systrace"), ArchiveMember: member, TraceEngine: "builtin"})
		if err != nil || converted.ArchiveProvenance == nil || converted.ArchiveProvenance.Member != member {
			t.Fatalf("existing converter control failed: result=%+v err=%v", converted, err)
		}
	})
	t.Run("ordinary_unselected_remains_ambiguous", func(t *testing.T) {
		bus, _, _ := hmc17NamedPathContext(t)
		result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"})
		if result.Success || !strings.Contains(result.Summary, "multiple_trace_members") {
			t.Fatalf("unselected query guessed a member: %+v", result)
		}
	})
	t.Run("ordinary_exact_member", func(t *testing.T) {
		bus, _, _ := hmc17NamedPathContext(t)
		result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": member, "view": "event_search"})
		payload := hmc17NamedPayload(t, result)
		if payload.EventCount != 1 || len(payload.Events) != 1 || payload.Events[0].Type != tracequery.EventPerfSample {
			t.Fatalf("selected member did not retain its own population: %+v", payload)
		}
	})
}

func TestHMC263ZIPMembersKeepIndependentColdWarmSources(t *testing.T) {
	hmc176DisableExternalProviders(t)
	members := map[string][]byte{"a/capture.data": hmc176PerfData(true), "b/capture.data": hmc17NamedBinaryTailFixture(), "same-bytes/capture.data": hmc176PerfData(true)}
	body := hmc176Zip(t, members)
	path := hmc176WriteSource(t, body)
	bus, prep, conversions := hmc17NamedPathContext(t)
	bus.AttachedHitrace = hmc176UnrelatedText
	paths := map[string]string{}
	for range 2 {
		for _, name := range []string{"a/capture.data", "b/capture.data", "same-bytes/capture.data"} {
			out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": name, "view": "event_search", "limit": 100})
			got := hmc17NamedPayload(t, out)
			want := 1
			if name == "b/capture.data" {
				want = 12
			}
			if got.EventCount != want || len(got.Events) != want {
				t.Fatalf("wrong member population %s: %+v", name, got)
			}
			material, err := prep.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: name})
			if err != nil || got.SourcePath != material.QueryPath() || !material.MatchesSelection(path, name) || material.MatchesPath(path) {
				t.Fatalf("member alias mismatch: %v", err)
			}
			bundle := hmc176bReadBundle(t, material.QueryPath())
			p := bundle.ArchiveProvenance
			inner, outer := sha256.Sum256(members[name]), sha256.Sum256(body)
			if p == nil || p.Member != name || p.MemberSHA256 != hex.EncodeToString(inner[:]) || p.ArchiveSHA256 != hex.EncodeToString(outer[:]) || p.Selection != "explicit_member" {
				t.Fatalf("wrong member provenance: %+v", p)
			}
			if old := paths[name]; old != "" && old != got.SourcePath {
				t.Fatal("warm member changed query coordinate")
			}
			paths[name] = got.SourcePath
			for _, event := range got.Events {
				hmc176bAssertManaged(t, filepath.Dir(material.QueryPath()), event.SourcePath)
			}
		}
	}
	if conversions.Load() != 3 || len(prep.PreparedMaterials()) != 3 || paths["a/capture.data"] == paths["same-bytes/capture.data"] || paths["a/capture.data"] == paths["b/capture.data"] {
		t.Fatal("member cache merged or reconverted")
	}
	if out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"}); out.Success {
		t.Fatal("warm unselected ZIP elected cached member")
	}
	if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
		t.Fatal("named query replaced attachment")
	}
	hmc176AssertOriginal(t, path, body)
}

func TestHMC263ZIPSelectorRejectsWrongCoordinatesAndPreservesRetry(t *testing.T) {
	hmc176DisableExternalProviders(t)
	path := hmc176WriteSource(t, hmc176Zip(t, map[string][]byte{"good.data": hmc176PerfData(true), "inventory.data": hmc176PerfData(false)}))
	bus, prep, _ := hmc17NamedPathContext(t)
	bus.AttachedHitrace = hmc176UnrelatedText
	for _, member := range []string{"missing.data", "../good.data", " good.data", "inventory.data"} {
		for range 2 {
			out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": member, "view": "event_search"})
			if out.Success || len(out.Observations) != 0 || out.RawRef != "" {
				t.Fatalf("invalid selection acquired facts: %+v", out)
			}
		}
	}
	if len(prep.PreparedMaterials()) != 0 {
		t.Fatal("failed member cached material")
	}
	got := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": "good.data", "view": "event_search"}))
	plain := hmc176WriteSource(t, hmc176PerfData(true))
	for _, args := range []map[string]any{
		{"source": "path", "path": plain, "archive_member": "good.data", "view": "event_search"},
		{"source": "path", "path": got.SourcePath, "archive_member": "good.data", "view": "event_search"},
		{"source": "attached_trace", "archive_member": "good.data", "view": "event_search"},
		{"source": "path", "path": path, "archive_member": "good.data", "business_span_ref": "untrusted", "view": "event_search"},
	} {
		if out := hmc17NamedQuery(t, bus, args); out.Success || len(out.Observations) != 0 {
			t.Fatalf("wrong coordinate selected member: %+v", out)
		}
	}
	bus.TraceInputPreparer = nil
	if out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": "good.data", "view": "event_search"}); out.Success {
		t.Fatal("selector ignored without preparation service")
	}
}

func TestHMC263ZIPMembersConcurrentAndSourceGeneration(t *testing.T) {
	hmc176DisableExternalProviders(t)
	path := hmc176WriteSource(t, hmc176Zip(t, map[string][]byte{"a.data": hmc176PerfData(true), "b.data": hmc17NamedBinaryTailFixture(), "new.data": hmc176PerfData(true)}))
	bus, prep, conversions := hmc17NamedPathContext(t)
	type result struct {
		name string
		out  types.ToolResult
		err  error
	}
	done := make(chan result, 4)
	for _, name := range []string{"a.data", "b.data", "a.data", "b.data"} {
		fork := bus.ShallowClone()
		fork.Mutable = types.NewMutableState("query member")
		fork.WorkDir = t.TempDir()
		raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "archive_member": name, "view": "event_search", "limit": 100})
		go func() { out, err := (&TraceQuery{}).Execute(fork, raw); done <- result{name, out, err} }()
	}
	for range 4 {
		got := <-done
		if got.err != nil {
			t.Fatal(got.err)
		}
		data := hmc17NamedPayload(t, got.out)
		want := 1
		if got.name == "b.data" {
			want = 12
		}
		if data.EventCount != want {
			t.Fatal("concurrent member mixed")
		}
	}
	if conversions.Load() != 2 || len(prep.PreparedMaterials()) != 2 {
		t.Fatal("concurrent key did not singleflight by member")
	}
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, hmc176Zip(t, map[string][]byte{"new.data": hmc176PerfData(true)}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.data", "b.data", "new.data"} {
		out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": name, "view": "event_search"})
		if out.Success || len(out.Observations) != 0 {
			t.Fatalf("source mutation bypassed through %s", name)
		}
	}
	if conversions.Load() != 2 {
		t.Fatal("new member bypassed observed source generation")
	}
}

func TestHMC263ZIPSelectedMemberRequestWindowAndReplay(t *testing.T) {
	hmc176DisableExternalProviders(t)
	path := hmc176WriteSource(t, hmc176Zip(t, map[string][]byte{"a.data": hmc176PerfData(true), "b.data": hmc17NamedBinaryTailFixture()}))
	bus, _, _ := hmc17NamedPathContext(t)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: requestWindowTestProfile(1.0105, 1.012)}}
	bus.RuntimeArtifactPreflight.Artifacts = []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Carrier: "request_path", Source: path}}
	args := map[string]any{"source": "path", "path": path, "archive_member": "b.data", "view": "event_search", "limit": 100}
	out := hmc17NamedQuery(t, bus, args)
	got := hmc17NamedPayload(t, out)
	if len(got.Events) != 1 || got.TimeStart != 1.0105 || got.TimeEnd != 1.012 || !strings.Contains(out.Summary, "trace_query_request_window_inherited=") {
		t.Fatalf("selected member lost single request window: %+v", got)
	}
	var p traceQueryParams
	original, _ := json.Marshal(args)
	_ = json.Unmarshal(original, &p)
	p.TimeStart, p.TimeEnd = traceSecondFromAutoWindow(got.TimeStart), traceSecondFromAutoWindow(got.TimeEnd)
	raw := traceQueryWindowReplayParams(p, got.SourcePath, "path", original)
	if len(raw) == 0 || strings.Contains(string(raw), "archive_member") {
		t.Fatalf("member replay retained outer selector: %s", raw)
	}
	replayed, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil || !replayed.Success {
		t.Fatalf("derived replay failed: %v %s", err, replayed.Summary)
	}
	args["time_start"], args["time_end"] = 1, 1.02
	if data := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, args)); data.EventCount != 12 || data.TimeStart != 1 || data.TimeEnd != 1.02 {
		t.Fatal("explicit window changed")
	}
	delete(args, "time_start")
	delete(args, "time_end")
	bus.RuntimeArtifactPreflight.Artifacts = append(bus.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Carrier: "request_path", Source: hmc176WriteSource(t, hmc176PerfData(true))})
	if out := hmc17NamedQuery(t, bus, args); strings.Contains(out.Summary, "trace_query_request_window_inherited=") {
		t.Fatal("ambiguous originals elected a member window")
	}
}

func TestHMC263ZIPExecutionLedgerOwnsOriginalAndExactMember(t *testing.T) {
	hmc176DisableExternalProviders(t)
	bus, prep, _ := hmc17NamedPathContext(t)
	path := filepath.Join(bus.RepoRoot, "captures.zip")
	if err := os.WriteFile(path, hmc176Zip(t, map[string][]byte{"a.data": hmc176PerfData(true), "b.data": hmc17NamedBinaryTailFixture()}), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := (&TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"event_search"}]}`))
	if err != nil || !out.Success {
		t.Fatalf("catalog: %v %s", err, out.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	alias := filepath.Join(bus.RepoRoot, "selected-alias.zip")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.data", "b.data", "a.data"} {
		selected := path
		if name == "b.data" {
			selected = alias
		}
		out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": selected, "archive_member": name, "view": "event_search"})
		if !out.Success {
			t.Fatalf("member registration: %s", out.Summary)
		}
	}
	m, err := prep.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: "a.data"})
	if err != nil {
		t.Fatal(err)
	}
	if out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": m.QueryPath(), "view": "event_search"}); !out.Success {
		t.Fatal(out.Summary)
	}
	seen := map[string]int{}
	revision := ""
	for _, q := range c.CurrentExecution().Snapshot.Queries {
		if len(q.Attempts) == 0 {
			continue
		}
		var args map[string]any
		if json.Unmarshal(q.Parameters, &args) != nil {
			t.Fatal("bad query params")
		}
		member, _ := args["archive_member"].(string)
		if member != "a.data" && member != "b.data" {
			t.Fatalf("derived query became whole archive: %s", q.Parameters)
		}
		if q.Outcome != tracecatalog.OutcomeSuccess {
			t.Fatalf("member switch invalidated sibling: %+v", q)
		}
		for _, a := range q.Attempts {
			var executed map[string]any
			_ = json.Unmarshal(a.ExecutedParameters, &executed)
			if executed["archive_member"] != member || a.PayloadRef == "" || a.SourceRevision == "" {
				t.Fatalf("lost actual selection: %+v", a)
			}
			if revision != "" && revision != a.SourceRevision {
				t.Fatal("member switch changed outer generation")
			}
			revision = a.SourceRevision
			seen[member]++
		}
	}
	if seen["a.data"] != 3 || seen["b.data"] != 1 {
		t.Fatalf("attempts collapsed or duplicated: %+v", seen)
	}
	// A damaged derived A publication cannot poison healthy B's outer binding.
	if err := os.WriteFile(m.QueryPath()+".replacement", []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(m.QueryPath()+".replacement", m.QueryPath()); err != nil {
		t.Fatal(err)
	}
	if out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "archive_member": "b.data", "view": "event_search"}); !out.Success {
		t.Fatalf("damaged A poisoned B: %s", out.Summary)
	}
}

func TestHMC263ZIPSelectionSchemaAndIndependentPair(t *testing.T) {
	hmc176DisableExternalProviders(t)
	var schema map[string]any
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	props := schema["properties"].(map[string]any)
	if props["archive_member"] == nil {
		t.Fatal("missing member schema")
	}
	side := props["comparison"].(map[string]any)["properties"].(map[string]any)["baseline"].(map[string]any)["properties"].(map[string]any)
	if side["archive_member"] == nil {
		t.Fatal("comparison member schema drift")
	}
	path := hmc176WriteSource(t, hmc176Zip(t, map[string][]byte{"a.data": hmc176PerfData(true), "b.data": hmc17NamedBinaryTailFixture()}))
	bus, _, _ := hmc17NamedPathContext(t)
	out := hmc17NamedQuery(t, bus, map[string]any{"comparison": map[string]any{"baseline": map[string]any{"source": "path", "path": path, "archive_member": "missing.data", "view": "window_stats"}, "current": map[string]any{"source": "path", "path": path, "archive_member": "b.data", "view": "window_stats"}}})
	report, valid := out.RuntimeMeasurementPair.Report()
	if !valid || !out.Success {
		t.Fatalf("pair: %v %s", valid, out.Summary)
	}
	if report.Sides[0].Status == "available" || report.Sides[1].Status != "available" {
		t.Fatalf("member failure crossed sides: %+v", report)
	}
}

func TestHMC263ZIPAutomaticMemberExecutionReceipt(t *testing.T) {
	hmc176DisableExternalProviders(t)
	bus, _, _ := hmc17NamedPathContext(t)
	path := filepath.Join(bus.RepoRoot, "capture.zip")
	if err := os.WriteFile(path, hmc176Zip(t, map[string][]byte{"captures/one.data": hmc176PerfData(true)}), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := (&TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"event_search"}]}`))
	if err != nil || !out.Success {
		t.Fatalf("catalog: %v %s", err, out.Summary)
	}
	if out := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"}); !out.Success {
		t.Fatal(out.Summary)
	}
	attempts := 0
	for _, q := range bus.Mutable.TraceCatalogs()[0].CurrentExecution().Snapshot.Queries {
		for _, a := range q.Attempts {
			attempts++
			var args map[string]any
			_ = json.Unmarshal(a.ExecutedParameters, &args)
			if args["archive_member"] != "captures/one.data" || a.PayloadRef == "" {
				t.Fatalf("unique actual member omitted: %+v", a)
			}
		}
	}
	if attempts != 1 {
		t.Fatalf("unique query attempt count=%d", attempts)
	}
}

func TestHMC263ZIPHealthyPairKeepsChildSourcesAndSideInvalidation(t *testing.T) {
	hmc176DisableExternalProviders(t)
	path := hmc176WriteSource(t, hmc176Zip(t, map[string][]byte{"a.data": hmc17NamedBinaryTailFixture(), "b.data": hmc176Profiler("bytrace_plugin", []byte(strings.Repeat(hmc176ScheduleText, 30)+hmc176bTailText))}))
	bus, prep, _ := hmc17NamedPathContext(t)
	request := map[string]any{"comparison": map[string]any{"baseline": map[string]any{"source": "path", "path": path, "archive_member": "a.data", "view": "window_stats"}, "current": map[string]any{"source": "path", "path": path, "archive_member": "b.data", "view": "window_stats"}}}
	out := hmc17NamedQuery(t, bus, request)
	report, valid := out.RuntimeMeasurementPair.Report()
	if !valid || report.Sides[0].Status != "available" || report.Sides[1].Status != "available" {
		t.Fatalf("healthy members: %+v", report)
	}
	materials := map[string]*attachment.TraceMaterial{}
	for _, m := range prep.PreparedMaterials() {
		materials[m.ArchiveMember()] = m
	}
	for i, name := range []string{"a.data", "b.data"} {
		if len(report.Sides[i].Publications) == 0 {
			t.Fatal("missing child publication")
		}
		for _, p := range report.Sides[i].Publications {
			if !materials[name].OwnsQueryArtifact(p.Source.Path) || materials[[]string{"b.data", "a.data"}[i]].OwnsQueryArtifact(p.Source.Path) {
				t.Fatal("publication crossed exact member")
			}
		}
	}
	m := materials["a.data"]
	if err := os.WriteFile(m.QueryPath()+".replacement", []byte("damaged own fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(m.QueryPath()+".replacement", m.QueryPath()); err != nil {
		t.Fatal(err)
	}
	report, valid = out.RuntimeMeasurementPair.Report()
	if !valid || report.Sides[0].Status != "stale" || report.Sides[1].Status != "available" || len(report.Sides[0].Publications) != 0 || len(report.Sides[1].Publications) == 0 {
		t.Fatalf("one changed member poisoned or reused sibling: %+v", report)
	}
}
