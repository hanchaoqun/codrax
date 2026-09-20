package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func supplementBusinessFocusFixture(t *testing.T, span string) (*types.BusContext, types.TraceBusinessSpanRef) {
	t.Helper()
	data, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx, path := businessRefTestContext(t, string(data))
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}}
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": span, "time_start": .999, "time_end": 1.05})
	if !r.Success || len(r.TraceBusinessSpanRefs) != 1 {
		t.Fatalf("one published paired instance required: %+v", r)
	}
	return ctx, r.TraceBusinessSpanRefs[0]
}

func supplementAcceptBusinessFocus(t *testing.T, ctx *types.BusContext, ref *types.TraceBusinessSpanRef) {
	t.Helper()
	ticket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
	if !ctx.Mutable.AcceptInvestigationCompleteWithBusinessSpanRef("accepted measured instance", ref) {
		t.Fatal("focus acceptance failed")
	}
	ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, true)
}

func supplementAssertBusinessWindow(t *testing.T, ctx *types.BusContext, ref types.TraceBusinessSpanRef) {
	t.Helper()
	d := ref.Data()
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || meta.WindowStart != d.StartTs || meta.WindowEnd != d.EndTs || meta.TargetPID != d.TID || meta.TargetSource != "accepted_business_instance" {
		t.Fatalf("supplement did not bind the full accepted instance: %+v, want %+v", meta, d)
	}
	results := ctx.Mutable.SystemTraceSupplementResults()
	if len(results) == 0 {
		t.Fatal("accepted instance produced no supplement")
	}
	for _, result := range results {
		if !types.TraceBusinessSpanResultSourceMatches(ref, result) {
			t.Fatal("supplement changed physical source generation")
		}
		for _, record := range result.Observations {
			s := record.SourceRef
			if !s.QueryWindowKnown || s.QueryWindowStartTs != d.StartTs || s.QueryWindowEndTs != d.EndTs || s.QueryTargetPID != d.TID || s.QueryTargetScope != "thread" {
				t.Fatalf("mixed focus scope on %s: %+v", record.Predicate, s)
			}
		}
	}
}

func TestTraceSupplementBusinessFocusPublicExactInstance(t *testing.T) {
	for _, span := range []string{"OpenDocument", "LoadDocumentIndex"} {
		t.Run(span, func(t *testing.T) {
			ctx, ref := supplementBusinessFocusFixture(t, span)
			// The model's cursor still covers 51ms and may name a different TID.
			old := businessRefTestQuery(t, ctx, map[string]any{"path": ref.Data().Path, "view": "root_cause_rank", "pid": 100, "time_start": .999, "time_end": 1.05})
			if !old.Success {
				t.Fatal(old.Summary)
			}
			old = businessRefTestQuery(t, ctx, map[string]any{"path": ref.Data().Path, "view": "critical_blocking_calls", "pid": 100, "time_start": .999, "time_end": 1.05})
			if !old.Success {
				t.Fatal(old.Summary)
			}
			before := ctx.Mutable.TraceQueryCallWindows()
			dispatched := len(ctx.Mutable.DispatchToolResults())
			supplementAcceptBusinessFocus(t, ctx, &ref)
			out := RunTraceQuerySystemSupplement(ctx)
			if len(out.Executed) != 2 {
				t.Fatalf("old cursor families suppressed the full instance: %+v", out)
			}
			supplementAssertBusinessWindow(t, ctx, ref)
			if !reflect.DeepEqual(before, ctx.Mutable.TraceQueryCallWindows()) || len(ctx.Mutable.DispatchToolResults()) != dispatched {
				t.Fatal("system supplement leaked into exploration history")
			}
		})
	}
}

func TestTraceSupplementBusinessFocusNoCursorAndNoDefaultSource(t *testing.T) {
	ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
	supplementAcceptBusinessFocus(t, ctx, &ref)
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) != 2 {
		t.Fatalf("native accepted ref must not require a default attachment or an exploration target: %+v", out)
	}
	supplementAssertBusinessWindow(t, ctx, ref)
}

func TestTraceSupplementBusinessFocusInvalidCannotFallbackToExploration(t *testing.T) {
	for _, reason := range []string{"source_replaced", "failed_dispatch", "source_replaced_with_user_tid", "failed_dispatch_with_user_tid"} {
		t.Run(reason, func(t *testing.T) {
			ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
			// Make the legacy route independently executable, so the negative
			// assertion cannot pass just because a source or target is absent.
			data, _ := os.ReadFile(ref.Data().Path)
			if err := os.WriteFile(filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename), data, 0600); err != nil {
				t.Fatal(err)
			}
			old := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "pid": 100, "time_start": .999, "time_end": 1.05})
			if !old.Success {
				t.Fatal(old.Summary)
			}
			if strings.HasSuffix(reason, "with_user_tid") {
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}}
			}
			ticket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
			if !ctx.Mutable.AcceptInvestigationCompleteWithBusinessSpanRef("selected", &ref) {
				t.Fatal("accept failed")
			}
			ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, !strings.HasPrefix(reason, "failed_dispatch"))
			if strings.HasPrefix(reason, "source_replaced") {
				if err := os.WriteFile(ref.Data().Path, append(data, []byte("# new generation\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out := RunTraceQuerySystemSupplement(ctx)
			if len(out.Executed) != 0 || len(ctx.Mutable.SystemTraceSupplementResults()) != 0 {
				t.Fatalf("invalid accepted focus revived old attached/cursor window: %+v", out)
			}
		})
	}
}

func TestTraceSupplementBusinessFocusExplicitWindowRemainsAuthoritative(t *testing.T) {
	ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
	data, _ := os.ReadFile(ref.Data().Path)
	if err := os.WriteFile(filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename), data, 0600); err != nil {
		t.Fatal(err)
	}
	start, end := .999, 1.049
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "requested clipped window"}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 200, Source: "user_explicit"}}
	supplementAcceptBusinessFocus(t, ctx, &ref)
	out := RunTraceQuerySystemSupplement(ctx)
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if len(out.Executed) == 0 || meta == nil || meta.WindowStart != start || meta.WindowEnd != end || meta.TargetPID != 200 || meta.TargetSource != "user" {
		t.Fatalf("accepted focus replaced explicit user scope: %+v %+v", out, meta)
	}
}

func TestTraceSupplementBusinessFocusNativeReceiptRequiredForFamilySuppression(t *testing.T) {
	for _, variant := range []string{"exact", "different_window", "start_plus_1us", "start_minus_half_us", "end_minus_1us", "different_tid", "different_source", "old_generation", "replayed"} {
		t.Run(variant, func(t *testing.T) {
			ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
			path, pid, start, end := ref.Data().Path, 100, 1.0, 1.05
			if variant == "different_window" {
				start = .999
			}
			if variant == "start_plus_1us" {
				start += .000001
			}
			if variant == "start_minus_half_us" {
				start -= .0000005
			}
			if variant == "end_minus_1us" {
				end -= .000001
			}
			if variant == "different_tid" {
				pid = 200
			}
			if variant == "different_source" {
				data, _ := os.ReadFile(path)
				path = filepath.Join(t.TempDir(), "other.systrace")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var results []types.ToolResult
			for _, view := range []string{"root_cause_rank", "critical_blocking_calls"} {
				raw, _ := json.Marshal(map[string]any{"path": path, "view": view, "pid": pid, "time_start": start, "time_end": end})
				r, err := (&TraceQuery{}).Execute(ctx, raw)
				if err != nil || !r.Success {
					t.Fatalf("native query failed: %v %s", err, r.Summary)
				}
				if variant == "replayed" {
					wire, _ := json.Marshal(r)
					r = types.ToolResult{}
					if err := json.Unmarshal(wire, &r); err != nil {
						t.Fatal(err)
					}
				}
				results = append(results, r)
			}
			if variant == "old_generation" {
				data, _ := os.ReadFile(path)
				if err := os.WriteFile(path, append(data, []byte("# replaced physical generation\n")...), 0600); err != nil {
					t.Fatal(err)
				}
				discovery := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": "OpenDocument"})
				if !discovery.Success || len(discovery.TraceBusinessSpanRefs) != 1 {
					t.Fatal("new generation did not publish a fresh instance")
				}
				ref = discovery.TraceBusinessSpanRefs[0]
			}
			ctx.ToolResults = results
			wanted := traceSupplementViewsForRequest(ctx, traceSupplementFamilies(types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: results})), false, false)
			supplementAcceptBusinessFocus(t, ctx, &ref)
			out := RunTraceQuerySystemSupplement(ctx)
			if variant == "exact" {
				if len(wanted) >= 2 || !reflect.DeepEqual(out.Executed, wanted) {
					t.Fatalf("matching native full-window families did not waive measured work: %+v, want %v", out, wanted)
				}
			} else {
				if len(out.Executed) != 2 {
					t.Fatalf("unrelated/serialized rows waived accepted-instance work: %+v", out)
				}
				supplementAssertBusinessWindow(t, ctx, ref)
			}
		})
	}
}

func TestTraceSupplementBusinessFocusAtomicWireAndNoNewIntent(t *testing.T) {
	for _, family := range []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency, types.RuntimeQuestionFactCountOrDuration} {
		t.Run(string(family), func(t *testing.T) {
			ctx, ref := supplementBusinessFocusFixture(t, "LoadDocumentIndex")
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{family}}
			var calls int
			old := traceSupplementBusinessFocusParamsHook
			t.Cleanup(func() { traceSupplementBusinessFocusParamsHook = old })
			traceSupplementBusinessFocusParamsHook = func(raw []byte) {
				var params map[string]any
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatal(err)
				}
				if len(params) != 2 || params["view"] != "window_stats" || params["business_span_ref"] != ref.Token() {
					t.Fatalf("accepted instance split into synthetic coordinates: %s", raw)
				}
				calls++
			}
			supplementAcceptBusinessFocus(t, ctx, &ref)
			out := RunTraceQuerySystemSupplement(ctx)
			if family == types.RuntimeQuestionFactIOLatency {
				if calls != 1 || !reflect.DeepEqual(out.Executed, []string{"window_stats"}) {
					t.Fatalf("bounded IO must retain its old single view: %+v calls=%d", out, calls)
				}
				supplementAssertBusinessWindow(t, ctx, ref)
			} else if calls != 0 || len(out.Executed) != 0 {
				t.Fatalf("selection invented causal/IO intent: %+v", out)
			}
		})
	}
}

func TestTraceSupplementBusinessFocusExplicitTargetAgreement(t *testing.T) {
	for _, kind := range []string{"same_tid", "different_tid", "process", "ambiguous", "named_unresolved"} {
		t.Run(kind, func(t *testing.T) {
			ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
			data, _ := os.ReadFile(ref.Data().Path)
			if err := os.WriteFile(filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename), data, 0600); err != nil {
				t.Fatal(err)
			}
			ctx.AnalysisIR.RequestModel.RuntimeTargetProfile = &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "requested target"}
			target := types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}
			if kind == "different_tid" {
				target.PID = 200
			}
			if kind == "process" {
				target.Kind = types.RuntimeTargetKindProcess
			}
			ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{target}
			if kind == "ambiguous" {
				target.PID = 200
				ctx.AnalysisIR.RequestModel.RuntimeTargets = append(ctx.AnalysisIR.RequestModel.RuntimeTargets, target)
			}
			if kind == "named_unresolved" {
				ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
			}
			supplementAcceptBusinessFocus(t, ctx, &ref)
			out := RunTraceQuerySystemSupplement(ctx)
			if kind == "same_tid" {
				supplementAssertBusinessWindow(t, ctx, ref)
				return
			}
			if kind == "ambiguous" || kind == "named_unresolved" {
				if len(out.Executed) != 0 || out.SkipReason != types.TraceSupplementReasonNoTypedTarget {
					t.Fatalf("focus bypassed unresolved explicit user target: %+v", out)
				}
				return
			}
			meta := ctx.Mutable.SystemTraceSupplementMeta()
			if len(out.Executed) == 0 || meta == nil || meta.TargetSource != "user" || meta.TargetPID != target.PID || meta.WindowStart != .999 {
				t.Fatalf("explicit user identity replaced: %+v %+v", out, meta)
			}
		})
	}
}

func TestTraceSupplementBusinessFocusScopeAndNoChoiceCompatibility(t *testing.T) {
	for _, kind := range []string{"multi_window", "full_artifact", "unanchored_scope", "bounded_selector", "cleared", "none"} {
		t.Run(kind, func(t *testing.T) {
			ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
			data, _ := os.ReadFile(ref.Data().Path)
			if err := os.WriteFile(filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename), data, 0600); err != nil {
				t.Fatal(err)
			}
			old := businessRefTestQuery(t, ctx, map[string]any{"view": "thread_timeline", "pid": 100, "time_start": .999, "time_end": 1.05})
			if !old.Success {
				t.Fatal(old.Summary)
			}
			start1, end1, start2, end2 := 1.001, 1.011, 1.02, 1.04
			switch kind {
			case "multi_window":
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeWindows: []types.RuntimeArtifactTimeWindow{{TimeStart: &start1, TimeEnd: &end1, SourceQuote: "first"}, {TimeStart: &start2, TimeEnd: &end2, SourceQuote: "second"}}}
			case "full_artifact":
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: "whole capture"}
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState}}
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}}
			case "unanchored_scope":
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact}
			case "bounded_selector":
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeBoundedSelector, SourceQuote: "this business operation"}
			}
			if kind != "none" {
				supplementAcceptBusinessFocus(t, ctx, &ref)
			}
			if kind == "cleared" {
				supplementAcceptBusinessFocus(t, ctx, nil)
			}
			out := RunTraceQuerySystemSupplement(ctx)
			meta := ctx.Mutable.SystemTraceSupplementMeta()
			if kind == "unanchored_scope" || kind == "bounded_selector" {
				supplementAssertBusinessWindow(t, ctx, ref)
				return
			}
			if len(out.Executed) == 0 || meta == nil || meta.TargetSource == "accepted_business_instance" {
				t.Fatalf("old explicit/no-selection lane changed: %s %+v %+v", kind, out, meta)
			}
			if kind == "multi_window" && len(meta.MemberWindows) != 2 {
				t.Fatalf("multiple requested windows lost: %+v", meta)
			}
			if kind == "full_artifact" && meta.RequestedArtifactScope != types.RuntimeArtifactScopeFullArtifact {
				t.Fatalf("full artifact narrowed to instance: %+v", meta)
			}
			if (kind == "cleared" || kind == "none") && meta.WindowStart != .999 {
				t.Fatalf("no-selection resurrected instance bounds: %+v", meta)
			}
		})
	}
}

func TestTraceSupplementBusinessFocusCensusRemainsSourceWideAndCurrent(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "expires_after_core"}[expire], func(t *testing.T) {
			data, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
			if err != nil {
				t.Fatal(err)
			}
			ctx, path := businessRefTestContext(t, string(data)+"VSyncGenerator-610 (610) [002] .... 2.000000: print: C|610|VSYNC-app|1\n")
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause, AnalyzerHints: types.AnalyzerHints{Keywords: []string{"vsync"}}, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}}}}
			r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": "LoadDocumentIndex"})
			if !r.Success || len(r.TraceBusinessSpanRefs) != 1 {
				t.Fatal("missing instance")
			}
			ref := r.TraceBusinessSpanRefs[0]
			old := traceSupplementAfterViewHook
			t.Cleanup(func() { traceSupplementAfterViewHook = old })
			if expire {
				traceSupplementAfterViewHook = func(string) { ctx.Mutable.ResetInvestigationComplete() }
			}
			supplementAcceptBusinessFocus(t, ctx, &ref)
			out := RunTraceQuerySystemSupplement(ctx)
			meta := ctx.Mutable.SystemTraceSupplementMeta()
			if meta == nil || meta.WindowStart != ref.Data().StartTs || meta.WindowEnd != ref.Data().EndTs || !reflect.DeepEqual(meta.Views, []string{"window_stats"}) {
				t.Fatalf("core instance window was replaced by census: %+v %+v", out, meta)
			}
			if expire {
				if meta.CensusLite || len(out.Executed) != 1 {
					t.Fatalf("revoked focus continued source-level census: %+v %+v", out, meta)
				}
				return
			}
			if !meta.CensusLite || !reflect.DeepEqual(out.Executed, []string{"window_stats", "event_search"}) {
				t.Fatalf("source-level census disappeared: %+v %+v", out, meta)
			}
			var census bool
			for _, result := range ctx.Mutable.SystemTraceSupplementResults() {
				if !types.TraceBusinessSpanResultSourceMatches(ref, result) {
					t.Fatal("census borrowed another source")
				}
				for _, row := range result.Observations {
					if row.Predicate != "vsync_generator_census" {
						continue
					}
					census = true
					if row.SourceRef.QueryTargetPID != 0 || row.SourceRef.QueryWindowKnown {
						t.Fatalf("whole-source census claimed instance window/target: %+v", row.SourceRef)
					}
				}
			}
			if !census {
				t.Fatal("census typed result missing")
			}
		})
	}
}

// Keep a business name a display concern, never a target heuristic.
func TestTraceSupplementBusinessFocusNeverParsesQuestion(t *testing.T) {
	ctx, _ := supplementBusinessFocusFixture(t, "OpenDocument")
	ctx.AnalysisIR.RequestModel.RawRequest = strings.Repeat("OpenDocument 100 1.0 1.05 root cause ", 3)
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) != 0 {
		t.Fatalf("unaccepted discovery or request prose chose an instance: %+v", out)
	}
}

func TestTraceSupplementBusinessFocusFinalProjectionUsesAcceptedInstance(t *testing.T) {
	ctx, ref := supplementBusinessFocusFixture(t, "OpenDocument")
	for _, start := range []float64{.999, .998} {
		for _, view := range []string{"root_cause_rank", "critical_blocking_calls"} {
			got := businessRefTestQuery(t, ctx, map[string]any{"path": ref.Data().Path, "view": view, "pid": 100, "time_start": start, "time_end": 1.05})
			if !got.Success {
				t.Fatal(got.Summary)
			}
		}
	}
	supplementAcceptBusinessFocus(t, ctx, &ref)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) != 2 {
		t.Fatalf("accepted instance not supplemented: %+v", out)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("single source must have one active projection: %+v", set)
	}
	p := set.Projections[0]
	corpus := suppFullCapCorpus(runtimeTraceCausalProjectionCluster(p, "zh", runtimeTraceProjUserFocus{}))
	t.Logf("projection=%.9f..%.9f target=%+v\n%s", p.WindowStartTs, p.WindowEndTs, p.TargetStateAccount, corpus)
	if p.WindowStartTs != ref.Data().StartTs || p.WindowEndTs != ref.Data().EndTs || p.TargetStateAccount == nil || p.TargetStateAccount.WindowStartTs != ref.Data().StartTs || p.TargetStateAccount.WindowEndTs != ref.Data().EndTs {
		t.Fatal("final projection selected an earlier exploratory account instead of the accepted business instance")
	}
	if math.Abs(p.TargetStateAccount.TotalMS-50) > 1e-6 {
		t.Fatal("instance account widened beyond 50ms")
	}
	var request, chainIO, backgroundIO bool
	for _, row := range ledger.Records {
		if row.Predicate == "io_latency" && row.Subject == "document-worker-200" && row.SourceRef.QueryWindowStartTs == 1 && row.SourceRef.QueryWindowEndTs == 1.05 && row.Value == "35.000" {
			request = true
			for key, want := range map[string]string{types.TraceNoteKeyIOIssuerBlocked: "31.000", types.TraceNoteKeyIOIssuerBlockedState: "s_sleep", types.TraceNoteKeyIORequestResidenceCaliber: "block_rq_issue_to_complete", types.TraceNoteKeyIONonAdditiveWithBlocked: "true"} {
				if traceSupplementRichNoteValue(row.RichNotes, key) != want {
					t.Fatalf("native request/blocking rulers mixed: %s=%s want %s", key, traceSupplementRichNoteValue(row.RichNotes, key), want)
				}
			}
		}
	}
	for _, node := range append(append([]types.TraceCausalProjectionNode(nil), p.PrimaryRootCauses...), p.OnChainCauses...) {
		if strings.Contains(node.Subject, "backup-900") {
			t.Fatal("independent background IO crowned as causal")
		}
		if node.Subject == "document-worker-200" && node.Object == "io_latency" && math.Abs(node.ImpactMS-31) < 1e-6 {
			chainIO = true
		}
	}
	for _, node := range p.BackgroundCauses {
		if strings.Contains(node.Subject, "backup-900") && math.Abs(node.ImpactMS-47) < 1e-6 {
			backgroundIO = true
		}
	}
	if !request || !chainIO || !backgroundIO {
		t.Fatalf("lost separate native request/causal blocking/background rulers: request=%t chainIO=%t backgroundIO=%t", request, chainIO, backgroundIO)
	}
}

func TestTraceSupplementBusinessFocusConflictCannotChooseOldCursor(t *testing.T) {
	ctx, first := supplementBusinessFocusFixture(t, "OpenDocument")
	discovery := businessRefTestQuery(t, ctx, map[string]any{"path": first.Data().Path, "view": "span_window", "span_name": "LoadDocumentIndex"})
	if !discovery.Success || len(discovery.TraceBusinessSpanRefs) != 1 {
		t.Fatal("missing second instance")
	}
	second := discovery.TraceBusinessSpanRefs[0]
	data, _ := os.ReadFile(first.Data().Path)
	if err := os.WriteFile(filepath.Join(ctx.WorkDir, types.AttachedTraceBlobBasename), data, 0600); err != nil {
		t.Fatal(err)
	}
	old := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "pid": 100, "time_start": .999, "time_end": 1.05})
	if !old.Success {
		t.Fatal(old.Summary)
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}}
	parentTicket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
	var forks []*types.MutableState
	for _, ref := range []types.TraceBusinessSpanRef{first, second} {
		fork := ctx.Mutable.ForkForExploreDispatch()
		ticket := fork.BeginTraceBusinessFocusDispatch()
		if !fork.AcceptInvestigationCompleteWithBusinessSpanRef("selected", &ref) {
			t.Fatal("fork acceptance failed")
		}
		fork.SettleTraceBusinessFocusDispatch(ticket, true)
		forks = append(forks, fork)
	}
	ctx.Mutable.SettleParallelTraceBusinessFocusDispatch(parentTicket, forks[:1], forks, true)
	if status, _ := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusConflict {
		t.Fatalf("fixture did not create a genuine conflict: %v", status)
	}
	if out := RunTraceQuerySystemSupplement(ctx); len(out.Executed) != 0 || out.SkipReason != types.TraceSupplementReasonWindowInconsistent {
		t.Fatalf("conflicting accepted instances chose an old cursor: %+v", out)
	}
}
