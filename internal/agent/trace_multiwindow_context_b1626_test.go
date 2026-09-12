package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func multiWindowContextProfileB1626(t *testing.T) *types.RuntimeArtifactScopeProfile {
	t.Helper()
	var profile types.RuntimeArtifactScopeProfile
	if err := json.Unmarshal([]byte(`{"requested_scope":"explicit_time_window","source_quote":"2..2.02 and 3..3.03","time_windows":[{"time_start":2,"time_end":2.02,"source_quote":"2..2.02"},{"time_start":3,"time_end":3.03,"source_quote":"3..3.03"}]}`), &profile); err != nil {
		t.Fatal(err)
	}
	return &profile
}

func TestMultiWindowFinalizerContextB1626(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.02, false)
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = multiWindowContextProfileB1626(t)
	var records []types.ObservationRecord
	for i, win := range [][2]float64{{2, 2.02}, {3, 3.03}, {2.005, 2.01}, {3.01, 3.02}, {2.01, 3.01}, {2.02, 3}, {1, 4}} {
		records = append(records, types.ObservationRecord{
			ID: fmt.Sprintf("q%d", i), Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			RichNotes: []string{fmt.Sprintf("selected_window=%.6f..%.6f", win[0], win[1])},
		})
	}
	records = append(records, types.ObservationRecord{ID: "unknown", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query"}, types.ObservationRecord{ID: "source", Producer: "read_file"})
	before, _ := json.Marshal(records)
	got, omitted := answerDocSelectedWindowObservationRecords(ctx, records)
	var ids []string
	for _, record := range got {
		ids = append(ids, record.ID)
	}
	if omitted != 3 || !reflect.DeepEqual(ids, []string{"q0", "q1", "q2", "q3", "unknown", "source"}) {
		t.Fatalf("keep each member and its drilldowns, never the inter-member envelope: ids=%v omitted=%d", ids, omitted)
	}
	after, _ := json.Marshal(records)
	if string(before) != string(after) {
		t.Fatal("context selection mutated evidence")
	}
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	if got, omitted := answerDocSelectedWindowObservationRecords(ctx, records); omitted != 0 || !reflect.DeepEqual(got, records) {
		t.Fatal("exploration must keep all original observations")
	}
}

func TestMultiWindowScopeTeachingAndDiscoveryB1626(t *testing.T) {
	ctx := traceRequestedScopeTestContext(2.02, false)
	profile := multiWindowContextProfileB1626(t)
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = profile
	ctx.AnalysisIR.RequestModel.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "app-100"}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit", Confidence: 1}}
	if !runtimeArtifactTraceDiscoveryInputsComplete(ctx.AnalysisIR.RequestModel) {
		t.Error("validated member windows and an explicit target complete discovery; no prose re-discovery")
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Source = types.RuntimeTargetSourceExplicitToolCall
	if runtimeArtifactTraceDiscoveryInputsComplete(ctx.AnalysisIR.RequestModel) {
		t.Error("an exploration-only target cannot bind requested members")
	}
	for _, lang := range []string{"zh", "en"} {
		record := types.ObservationRecord{Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", SourceRef: types.ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 3, QueryWindowEndTs: 3.03}, RichNotes: []string{"selected_window=3.000000..3.030000"}}
		text := traceQueryObservationRequestedScopeNote(record, profile, lang)
		if !strings.Contains(text, "3.000000") || !strings.Contains(text, "3.030000") || strings.Contains(text, "2.000000–3.030000") {
			t.Errorf("%s: member scope missing or replaced by envelope: %q", lang, text)
		}
		record.SourceRef = types.ObservationSourceRef{}
		unknown := traceQueryObservationRequestedScopeNote(record, profile, lang)
		if strings.Contains(unknown, "3.000000") || strings.Contains(unknown, "2/2") {
			t.Fatalf("native local ruler granted a parent scope: %s", unknown)
		}
	}
}

func TestMultiWindowReaderParentSourceB1626(t *testing.T) {
	profile := multiWindowContextProfileB1626(t)
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture.trace", RawRef: "raw", QueryScopeID: "B", QueryWindowKnown: true, QueryWindowStartTs: 3, QueryWindowEndTs: 3.03}
	record := types.ObservationRecord{ID: "recursive", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref, Predicate: "wakeup_chain", RichNotes: []string{"selected_window=3.01..3.02", types.TraceNoteKeyChainPathBranch + "=2"}}
	projection := types.TraceCausalProjection{ArtifactPath: "/capture.trace", WindowStartTs: 3, WindowEndTs: 3.03, WindowScope: types.ResolveTraceQueryWindowScope(profile, 3, 3.03), QuerySourceRef: &ref, WakeupPathBranch: 2}
	a := projection
	aRef := ref
	aRef.QueryScopeID, aRef.QueryWindowStartTs, aRef.QueryWindowEndTs = "A", 2, 2.02
	a.QuerySourceRef, a.WindowStartTs, a.WindowEndTs, a.WakeupPathBranch = &aRef, 2, 2.02, 1
	a.WindowScope = types.ResolveTraceQueryWindowScope(profile, 2, 2.02)
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{a, projection}}
	if start, end, ok := traceQueryObservationProjectionWindow(record, set); !ok || start != 3 || end != 3.03 {
		t.Fatalf("recursive B borrowed A: %g %g %v", start, end, ok)
	}
	if branch, elected := traceQueryObservationElectedBranchPathHead(record, set); branch != 2 || !elected {
		t.Fatalf("recursive B lost its own election: %d %v", branch, elected)
	}
	filtered := record
	filtered.SourceRef.QueryLineRangeKnown, filtered.SourceRef.QueryLineStart = true, 10
	if _, _, ok := traceQueryObservationProjectionWindow(filtered, set); ok {
		t.Fatal("same-window different filter borrowed projection")
	}
	set.Projections = append(set.Projections, projection)
	if _, _, ok := traceQueryObservationProjectionWindow(record, set); ok {
		t.Fatal("ambiguous source must not pick first projection")
	}
	if _, elected := traceQueryObservationElectedBranchPathHead(record, set); elected {
		t.Fatal("ambiguous source must not borrow election")
	}
	zeroRef := ref
	zeroRef.QueryWindowStartTs, zeroRef.QueryWindowEndTs = 0, .02
	zero := projection
	zero.QuerySourceRef, zero.WindowStartTs, zero.WindowEndTs = &zeroRef, 0, .02
	zeroRecord := record
	zeroRecord.SourceRef = zeroRef
	if start, end, ok := traceQueryObservationProjectionWindow(zeroRecord, types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{zero}}); !ok || start != 0 || end != .02 {
		t.Fatalf("valid zero-origin parent window became unavailable: %g %g %v", start, end, ok)
	}
	ctx := traceRequestedScopeTestContext(2.02, false)
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = profile
	// Its local recursive window crosses the gap; only the producer-owned
	// parent query, not that native child ruler, controls context admission.
	record.RichNotes = []string{"selected_window=2.01..3.01"}
	if got, omitted := answerDocSelectedWindowObservationRecords(ctx, []types.ObservationRecord{record}); len(got) != 1 || omitted != 0 {
		t.Fatal("native recursive child was removed from a valid B query")
	}
}
