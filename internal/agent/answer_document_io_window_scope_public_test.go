package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The query is executed, published, and compiled through the same public
// dispatch/TurnA path as finalization. No source-window fields are edited.
func TestIOPublicQueryWindowDoesNotBorrowRequestedAuthority(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		params                map[string]any
		principal, background bool
	}{
		{"line_only", map[string]any{"line_start": 5, "line_end": 6}, false, true},
		{"line_over_time", map[string]any{"line_start": 5, "line_end": 6, "time_start": 1, "time_end": 1.01}, false, true},
		{"point", map[string]any{"time_start": 1.004, "time_end": 1.004}, false, true},
		{"missing", map[string]any{}, false, false},
		{"selected", map[string]any{"time_start": 1, "time_end": 1.01}, true, false},
		{"contained", map[string]any{"time_start": 1.002, "time_end": 1.008}, true, false},
		{"explicit_zero", map[string]any{"time_start": 0, "time_end": 1.01}, true, false},
		{"multi_member", map[string]any{"time_start": 5, "time_end": 5.01}, true, false},
		{"multi_envelope", map[string]any{"time_start": 1, "time_end": 5.01}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "io.systrace")
			body := "# tracer: nop\n" +
				"reader-40 (40) [001] .... 0.900000: sched_wakeup: comm=reader pid=40 prio=120 target_cpu=1\n" +
				"reader-40 (40) [001] .... 1.001000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [reader]\n" +
				"irq-80 (2) [001] .... 1.006000: block_rq_complete: 8,0 R () 1000 + 8 [0]\n" +
				"reader-40 (40) [001] .... 5.000000: block_rq_issue: 8,0 R 4096 () 2000 + 8 [reader]\n" +
				"irq-80 (2) [001] .... 5.006000: block_rq_complete: 8,0 R () 2000 + 8 [0]\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			start, end := 1.0, 1.01
			if tc.name == "explicit_zero" {
				start = 0
			}
			ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("Describe IO measurements in the selected time window"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "en", PerfTrace: &types.PerfBundle{},
					RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 40, Source: "user_request"}},
					RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency, types.RuntimeQuestionFactResourcePressure}},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.010 seconds"},
				}}}
			if strings.HasPrefix(tc.name, "multi_") {
				secondStart, secondEnd := 5.0, 5.01
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
					RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeWindows:    []types.RuntimeArtifactTimeWindow{{TimeStart: &start, TimeEnd: &end, SourceQuote: "1..1.01"}, {TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "5..5.01"}},
				}
			}
			if !ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
				t.Fatal("fixture must carry a validated explicit user window")
			}
			tc.params["source"], tc.params["path"], tc.params["view"] = "path", path, "window_stats"
			params, _ := json.Marshal(tc.params)
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success {
				t.Fatalf("public query failed: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			ledger := answerDocObservationLedger(ctx)
			before, _ := json.Marshal(ledger.Records)
			projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjection(ledger))
			originalRequest := ctx.AnalysisIR.RequestModel
			measured := 0
			for _, row := range ledger.Records {
				if !answerDocBoundedRuntimeIOLatencyPredicate(row.Predicate) && row.Predicate != "io_inflight" && row.Predicate != "io_inflight_coverage" {
					continue
				}
				if row.Predicate == "io_latency" && traceQueryObservationSupplementNoteValue(row, types.TraceNoteKeyIORequestResidence) != "" {
					measured++
				}
				if tc.name == "line_only" || tc.name == "point" {
					if row.SourceRef.QueryWindowKnown {
						t.Fatal("native unknown-window premise changed")
					}
				} else if !row.SourceRef.QueryWindowKnown {
					t.Fatal("native known query premise changed")
				}
			}
			if measured == 0 {
				t.Fatal("public query must publish actual accepted request measurements")
			}
			for _, lane := range []string{"finite", "causal", "generic"} {
				t.Run(lane, func(t *testing.T) {
					rm := &ctx.AnalysisIR.RequestModel
					rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 40, Source: "user_request"}}
					rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency, types.RuntimeQuestionFactResourcePressure}}
					if lane == "causal" {
						rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
					}
					if lane == "generic" {
						rm.RuntimeTargets = nil
						rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}
					}
					prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
					principal := ""
					for _, heading := range []string{"### Requested Runtime Fact Authority", "### IO Measurements For Causal Interpretation", queriedIOSemanticsHeading, "#### IO In-Flight Measurements"} {
						principal += ioWindowPublicSection(prompt, heading)
					}
					background := ioWindowPublicSection(prompt, "### Separately Scoped IO Background")
					if (background != "") != tc.background {
						t.Fatalf("background presence=%t want=%t", background != "", tc.background)
					}
					for _, row := range ledger.Records {
						if !answerDocIOWindowScopedPredicate(row.Predicate) {
							continue
						}
						if !tc.principal && strings.Contains(principal, "id=`"+row.ID+"`") {
							t.Errorf("%s borrowed requested-window authority", row.Predicate)
						}
						if tc.background && !strings.Contains(background, "id=`"+row.ID+"`") {
							t.Errorf("background lost %s", row.Predicate)
						}
					}
					wantResidence := "5.000"
					if tc.name == "multi_member" {
						wantResidence = "6.000"
					}
					if tc.principal && !strings.Contains(principal, "request_residence=`"+wantResidence+"`") {
						t.Fatal("contained query lost original full request residence")
					}
					if tc.background {
						for _, want := range []string{"owner_scope=`separate_query_context`", "query_window=`unknown`", "not use their counts, distributions or durations as requested-window statistics", "Unknown occupancy is not measured zero"} {
							if !strings.Contains(background, want) {
								t.Errorf("background lost %q", want)
							}
						}
						for _, bad := range []string{"owner_scope=`selected_window_context`", "owner_scope=`target_owned`", "query_window=`1.000000..1.010000`"} {
							if strings.Contains(background, bad) {
								t.Errorf("background borrowed %q", bad)
							}
						}
					}
				})
			}
			ctx.AnalysisIR.RequestModel = originalRequest
			afterLedger := answerDocObservationLedger(ctx)
			after, _ := json.Marshal(afterLedger.Records)
			projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjection(afterLedger))
			if string(before) != string(after) || string(projectionBefore) != string(projectionAfter) {
				t.Fatal("display projection mutated accepted observations or causal projection")
			}
		})
	}
}

func ioWindowPublicSection(prompt, heading string) string {
	_, section, ok := strings.Cut(prompt, heading+"\n\n")
	if !ok {
		return ""
	}
	section, _, _ = strings.Cut(section, "\n\n")
	return section
}

func TestIOWindowBackgroundBudgetAndNonFinalizerPreservation(t *testing.T) {
	ctx := queriedIOSemanticsUnitContext()
	start, end := 10.0, 11.0
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10..11 seconds"}
	var records []types.ObservationRecord
	for i := 0; i < 15; i++ {
		r := ioDetailCoverageTestRecord("1", "1", "0")
		r.ID, r.SourceRef.QueryScopeID = fmt.Sprint("io-query-", i), fmt.Sprint("query-", i)
		r.SourceRef.QueryWindowKnown = false
		records = append(records, r)
	}
	principal, background, outside := answerDocIOWindowObservationRecords(ctx, records)
	if len(principal) != 0 || len(background) != 15 || outside != 0 {
		t.Fatal("unknown query disposition changed")
	}
	text := renderAnswerDocSeparateIOQueryContext(ctx, background)
	if !strings.Contains(text, "rendered_background_rows=10; omitted_background_rows=5") || strings.Count(text, "owner_scope=`separate_query_context`") != 10 {
		t.Fatal("background budget/omissions missing")
	}
	ctx.Stage, ctx.AgentName = types.StageExplore, types.AgentExplorer
	principal, background, outside = answerDocIOWindowObservationRecords(ctx, records)
	if len(principal) != 15 || len(background) != 0 || outside != 0 {
		t.Fatal("final answer projection changed exploration")
	}
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
	principal, background, outside = answerDocIOWindowObservationRecords(ctx, records)
	if len(principal) != 15 || len(background) != 0 || outside != 0 {
		t.Fatal("no explicit user time window must preserve prior behavior")
	}
}
