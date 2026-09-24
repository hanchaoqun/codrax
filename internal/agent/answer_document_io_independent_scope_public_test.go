package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestIOPublicIndependentWaitSurvivesUnknownQueryPopulation(t *testing.T) {
	for _, tc := range []struct {
		name, closure, query string
		admit                bool
	}{
		{"line_time_proved", "proven", "line_time", true},
		{"line_without_selected_window", "proven", "line", false},
		{"point", "proven", "point", false},
		{"absent_wakeup", "absent_wakeup", "line_time", false},
		{"different_waker", "different_waker", "line_time", false},
		{"missing_blocking_transition", "missing_blocking_transition", "line_time", false},
		{"foreign_target", "proven", "line_time", false},
		{"outside_user_window", "proven", "line_time", false},
		{"conflicting_time_arguments", "proven", "line_time", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Produce real closure evidence, then execute a different public query.
			// No accepted row, source-window field, or proof note is fabricated.
			ctx := b1644IOCompletionContext(t, "block_rq", tc.closure, "en")
			causalIOInstructionContext(ctx)
			var path string
			for _, row := range answerDocObservationLedger(ctx).Records {
				if row.Predicate == "io_latency" {
					path = row.SourceRef.Path
					break
				}
			}
			if path == "" {
				t.Fatal("native IO source missing")
			}
			params := map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 41}
			if tc.query == "point" {
				params["time_start"], params["time_end"] = 5.00015, 5.00015
			} else {
				params["line_start"], params["line_end"] = 1, 20
				if tc.query == "line_time" {
					params["time_start"], params["time_end"] = 5.0, 5.001
				}
			}
			if tc.name == "conflicting_time_arguments" {
				params["time_start"], params["time_end"] = 40.0, 41.0
			}
			ctx.Mutable = types.NewMutableState("Explain the target's response delay in the selected interval")
			data, _ := json.Marshal(params)
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), data)
			if err != nil || !result.Success {
				t.Fatalf("actual query failed: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			if tc.name == "foreign_target" {
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 99, Thread: "foreign-99", Source: "user_explicit"}}
			}
			if tc.name == "outside_user_window" {
				start, end := 6.0, 6.001
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "6..6.001"}
			}
			ledger := answerDocObservationLedger(ctx)
			var ioRows []types.ObservationRecord
			for _, row := range ledger.Records {
				if row.Predicate == "io_latency" {
					ioRows = append(ioRows, row)
					if row.SourceRef.Path != path || row.Span.LineStart <= 0 || row.Span.LineEnd < row.Span.LineStart {
						t.Fatalf("native source/physical support lines missing: %+v", row)
					}
				}
			}
			if len(ioRows) == 0 {
				t.Fatal("native accepted request evidence missing")
			}
			before := b1670ScopeSnapshot(t, ctx)
			// Both the public instruction and the actual finalizer adapter route
			// must preserve the same independent ruler and population boundary.
			for _, message := range []string{(&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil), dependencyObservationMessages(t, ctx)} {
				causal := ioWindowPublicSection(message, "### IO Measurements For Causal Interpretation")
				if (strings.Contains(causal, "owner_scope=`independent_wait_evidence`")) != tc.admit {
					t.Fatalf("independent wait admission=%t want=%t: %s", causal != "", tc.admit, causal)
				}
				if tc.admit {
					for _, want := range []string{"issuer_blocked=`0.090`", "query_window=`unknown`", "source_path=", path} {
						if !strings.Contains(causal, want) {
							t.Errorf("independent source ruler lost %q", want)
						}
					}
					blocking := ioWindowPublicSection(message, "### Trace Target Blocking Wall-Clock Authority")
					for _, want := range []string{"proven_blocking_wall_clock=0.090ms", "coverage_scope=`observed_occurrences_only`", "query_population=`unverified`"} {
						if !strings.Contains(blocking, want) {
							t.Errorf("blocking ruler lost %q", want)
						}
					}
				}
				for _, heading := range []string{"### Requested Runtime Fact Authority", queriedIOSemanticsHeading, "#### IO In-Flight Measurements"} {
					section := ioWindowPublicSection(message, heading)
					for _, row := range ioRows {
						if strings.Contains(section, "id=`"+row.ID+"`") {
							t.Errorf("independent wait was put back in population pool %s", heading)
						}
					}
				}
				if strings.Contains(causal, "owner_scope=`target_owned`") || strings.Contains(causal, "owner_scope=`selected_window_context`") {
					t.Fatal("unknown population regained window ownership")
				}
			}
			if b1670ScopeSnapshot(t, ctx) != before {
				t.Fatal("independent display mutated evidence, causal projection, or closure authority")
			}
		})
	}
}

func TestIOPublicIndependentLegacyWaitUsesExistingProofGate(t *testing.T) {
	for _, variant := range []string{"valid", "missing_source", "invalid_duration", "missing_window", "invalid_window", "outside_occurrence", "foreign_target", "soft_grounding"} {
		t.Run(variant, func(t *testing.T) {
			all := b1670BlockingScopeRecords("capture-local", "client-100", 10, "io")
			r := all[len(all)-1]
			setNote := func(key, value string) {
				for i, note := range r.RichNotes {
					if strings.HasPrefix(note, key+"=") {
						r.RichNotes[i] = key + "=" + value
					}
				}
			}
			switch variant {
			case "missing_source":
				r.SourceRef = types.ObservationSourceRef{}
			case "invalid_duration":
				setNote("issuer_blocked", "7.000")
			case "missing_window":
				setNote("selected_window", "")
			case "invalid_window":
				setNote("selected_window", "10..10")
			case "outside_occurrence":
				setNote("issuer_blocked_start", "12.000000")
				setNote("issuer_blocked_end", "12.001000")
			case "foreign_target":
				r.Subject = "worker-200"
			case "soft_grounding":
				r.GroundingPolicy = types.ClaimGroundingSoft
			}
			// IO-only exercises the early-return path with no principal rows.
			ctx := b1670BlockingScopePublicContext([]types.ObservationRecord{r}, "en")
			before := b1670ScopeSnapshot(t, ctx)
			message := dependencyObservationMessages(t, ctx)
			causal := ioWindowPublicSection(message, "### IO Measurements For Causal Interpretation")
			if strings.Contains(causal, "owner_scope=`independent_wait_evidence`") != (variant == "valid") {
				t.Fatalf("existing proof gate did not decide admission: %s", causal)
			}
			if strings.Contains(message, "proven_blocking_wall_clock=1.000ms") != (variant == "valid") {
				t.Fatal("independent blocking ruler bypassed the source/target/window/duration proof")
			}
			if !strings.Contains(message, "### Separately Scoped IO Background") || b1670ScopeSnapshot(t, ctx) != before {
				t.Fatal("unknown population or original evidence disappeared")
			}
		})
	}
}
