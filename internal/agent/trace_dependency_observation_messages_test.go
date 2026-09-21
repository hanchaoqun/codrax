package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Stop at the actual adapter boundary. This exercises both the ordinary
// handoff and the compact final tail without a model call or answer rewrite.
func dependencyObservationMessages(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	before := b1670ScopeSnapshot(t, ctx)
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.EmitAnswerDocument{})
	reg.Register(&toolpkg.EmitAnswerDocumentPatch{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured dependency observation request")}
	agent := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := agent.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("expected one actual finalizer request: calls=%d err=%v", capture.calls, err)
	}
	if after := b1670ScopeSnapshot(t, ctx); before != after {
		t.Fatal("prompt changed observations, measured values, scope, blocking authority or model-owned answer")
	}
	var messages strings.Builder
	for _, message := range capture.messages {
		messages.WriteString(message.Content)
		messages.WriteByte('\n')
	}
	return messages.String()
}

func assertDependencyObservationTeaching(t *testing.T, messages string) {
	t.Helper()
	for _, want := range []string{
		"## Trace Decision Inputs (Model Owns The Conclusion)",
		"## Final Trace Decision Boundary (Typed Facts; Model-Owned Conclusion)",
		"upstream on-chain dependency observation",
		"mechanism_ceiling=`on_chain_prewakeup_observation_candidate_only`",
		"only as an on-chain dependency observation",
		"same capture, target, and window",
		"independently proved waits, including Binder or completion-closed IO",
		"do not by themselves establish a holder relation or root-cause eligibility",
		"Do not compute a residual by adding or subtracting overlapping rows",
		"The row's typed state or semantic span, not this phase, distinguishes waiting, running, or semantic work; unclassified rows remain unspecified",
	} {
		if !strings.Contains(messages, want) {
			t.Errorf("actual finalizer messages omitted %q", want)
		}
	}
	for _, forbidden := range []string{
		"remaining on-chain work as unpriced or unresolved",
		"is upstream on-chain work overlapping",
		"This seat is a ranked work candidate",
		"only as on-chain work overlapping",
		"mechanism_ceiling=`on_chain_prewakeup_work_candidate_only`",
	} {
		if strings.Contains(messages, forbidden) {
			t.Errorf("untyped phase still teaches execution/work: %q", forbidden)
		}
	}
}

func TestDependencyObservationNativeQueryActualFinalizerMessages(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("missing native trace fixture")
	}
	trace, _, ok = strings.Cut(trace, "\n'\n")
	if !ok {
		t.Fatal("unterminated native trace fixture")
	}
	for _, renamed := range []bool{false, true} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(fmt.Sprintf("renamed=%t/%s", renamed, lang), func(t *testing.T) {
				source, target, sleeper, terminal := trace, "app-100", "cookie-200", "threadpool-400"
				if renamed {
					source = strings.NewReplacer("app", "client", "cookie", "session", "network", "transport", "threadpool", "executor").Replace(source)
					target, sleeper, terminal = "client-100", "session-200", "executor-400"
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "native.ftrace")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				var records []types.ObservationRecord
				for _, view := range []string{"root_cause_rank", "wakeup_chain", "window_stats"} {
					params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
					result, err := (&toolpkg.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
					if err != nil || !result.Success {
						t.Fatalf("public %s query failed: %v %s", view, err, result.Summary)
					}
					records = append(records, result.Observations...)
				}
				ctx := b1670BlockingScopePublicContext(records, lang)
				start, end := 2.0, 2.020
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.SourceQuote = "2.000000..2.020000"
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Thread = target
				messages := dependencyObservationMessages(t, ctx)
				for _, want := range []string{
					"subject=`" + sleeper + "`; state_kind=`s_sleep`; measured_state_occupancy=17.000ms",
					"subject=`" + terminal + "`; state_kind=`io_wait`; measured_state_occupancy=11.000ms",
					"measured_state_occupancy=14.000ms",
					"subject=`" + sleeper + "`; target=`" + target + "`",
					"query_window=`2.000000..2.020000`",
					"cause_decomposition_status=`not_closed_by_state_partition_or_ranked_seat_roster`",
				} {
					if !strings.Contains(messages, want) {
						t.Fatalf("native public evidence did not reach actual finalizer: %q", want)
					}
				}
				assertDependencyObservationTeaching(t, messages)
			})
		}
	}
}

func TestDependencyObservationTypedStatesAndBoardsActualFinalizerMessages(t *testing.T) {
	for _, state := range []string{"s_sleep", "io_wait", "d_state", "runnable", "running", "semantic", "unknown"} {
		for _, complete := range []bool{false, true} {
			for _, lang := range []string{"zh", "en"} {
				t.Run(fmt.Sprintf("%s/board=%t/%s", state, complete, lang), func(t *testing.T) {
					records := b1670BlockingScopeRecords("capture-A", "client-100", 10, "both")
					row := &records[1]
					row.Object = state
					if state == "semantic" {
						row.Object = "class_verification"
						row.RichNotes = append(row.RichNotes, "semantic_class=class_verification", "span_name=Observed verification")
					} else {
						row.RichNotes = append(row.RichNotes, "dominant_state="+state)
					}
					if !complete {
						var notes []string
						for _, note := range row.RichNotes {
							if !strings.HasPrefix(note, types.TraceNoteKeyRankBoardParams+"=") {
								notes = append(notes, note)
							}
						}
						row.RichNotes = notes
					}
					if state == "semantic" {
						span := *row
						span.ID, span.Predicate, span.ClaimKey = "semantic-span", "trace_semantic_span", "trace_semantic_span:worker-200"
						span.RichNotes = []string{"selected_window=10.000000..10.020000", "semantic_class=class_verification", "span_name=Observed verification", "chain_relevance=on_chain", "causality=on_wakeup_chain", "on_chain_basis=" + types.TraceCausalOnChainBasisHostWakeupEdgeSpan}
						records = append(records, span)
					}
					ctx := b1670BlockingScopePublicContext(records, lang)
					messages := dependencyObservationMessages(t, ctx)
					for _, want := range []string{"subject=`worker-200`", "effective_attribution=2.000ms", "verified_wait_union=2.000ms", "proven_blocking_wall_clock=1.000ms"} {
						if !strings.Contains(messages, want) {
							t.Fatalf("fixture lost native/independent authority %q", want)
						}
					}
					if !complete && !strings.Contains(messages, "not as a shared direction leader") {
						t.Fatal("incomplete query board acquired a shared leader")
					}
					if state == "semantic" {
						for _, want := range []string{"deterministic_semantic_spans (typed in-window work inventory", "semantic_class=`class_verification`", "span=`Observed verification`", "total=5.000ms"} {
							if !strings.Contains(messages, want) {
								t.Errorf("legitimate semantic-work inventory lost %q", want)
							}
						}
					} else if !strings.Contains(messages, "state_kind=`"+state+"`") {
						t.Fatalf("fixture did not publish its typed state %q", state)
					}
					if state == "running" && (!strings.Contains(messages, "fix_direction=`self_workload`") || !strings.Contains(messages, "high-cost work that current formulas do not price")) {
						t.Error("neutral phase erased the typed running workload direction")
					}
					assertDependencyObservationTeaching(t, messages)
				})
			}
		}
	}
}

func TestDependencyObservationActualFinalizerAuthorityExceptions(t *testing.T) {
	for _, lane := range []string{"none", "binder", "io", "both", "target-blocker", "foreign-blocker", "no-phase", "bounded"} {
		t.Run(lane, func(t *testing.T) {
			independent := lane
			if lane == "target-blocker" || lane == "foreign-blocker" || lane == "no-phase" || lane == "bounded" {
				independent = "both"
			}
			rows := b1670BlockingScopeRecords("capture-A", "client-100", 10, independent)
			if lane == "target-blocker" || lane == "foreign-blocker" {
				blocker := rows[1]
				blocker.ID, blocker.Subject, blocker.Predicate, blocker.Object = "blocker", "client-100", "critical_blocking", "monitor_contention"
				blocker.ClaimKey = "critical_blocking:client-100"
				blocker.RichNotes = []string{"selected_window=10.000000..10.020000", "blocking_kind=monitor_contention", "peer=holder-300", "chain_relevance=on_chain"}
				if lane == "foreign-blocker" {
					blocker.Subject = "other-900"
				}
				rows = append(rows, blocker)
			}
			if lane == "no-phase" {
				for i, note := range rows[1].RichNotes {
					if note == "chain_depth=1" {
						rows[1].RichNotes[i] = "chain_depth=0"
					}
				}
			}
			ctx := b1670BlockingScopePublicContext(rows, "en")
			if lane == "bounded" {
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}
			}
			messages := dependencyObservationMessages(t, ctx)
			mechanism := b1670ScopeLines(messages, "- final_answer_mechanism_scope")
			if lane == "bounded" {
				if strings.Contains(messages, "## Trace Decision Inputs") || strings.Contains(messages, "## Final Trace Decision Boundary") || mechanism != "" {
					t.Fatal("collected causal rows widened a bounded fact request")
				}
				return
			}
			if independent == "binder" || independent == "both" {
				if !strings.Contains(messages, "verified_wait_union=2.000ms") {
					t.Fatal("independent Binder wait was lost")
				}
			}
			if independent == "io" || independent == "both" {
				if !strings.Contains(messages, "proven_blocking_wall_clock=1.000ms") {
					t.Fatal("independent completion-closed IO wait was lost")
				}
			}
			if lane == "target-blocker" || lane == "no-phase" {
				if mechanism != "" {
					t.Fatalf("phase ceiling ignored stronger authority or missing phase: %s", mechanism)
				}
				if lane == "target-blocker" && !strings.Contains(messages, "direct_blocking_decision=`established_by_typed_relation`") {
					t.Fatal("typed target blocker was erased")
				}
			} else {
				assertDependencyObservationTeaching(t, messages)
			}
		})
	}
}

func TestDependencyObservationActualFinalizerKeepsPreviewBudgets(t *testing.T) {
	rows := b1670BlockingScopeRecords("capture-A", "client-100", 10, "none")
	seed := rows[1]
	rows = rows[:1]
	for i := 0; i < 10; i++ {
		row := seed
		row.ID, row.Subject, row.ClaimKey = fmt.Sprintf("row-%02d", i), fmt.Sprintf("worker-%02d", i), fmt.Sprintf("root_cause_primary:worker-%02d", i)
		row.RichNotes = append([]string(nil), seed.RichNotes...)
		for j, note := range row.RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyRankBoardParams+"=") {
				row.RichNotes[j] = fmt.Sprintf("%s=params-%02d", types.TraceNoteKeyRankBoardParams, i)
			}
		}
		rows = append(rows, row)
	}
	messages := dependencyObservationMessages(t, b1670BlockingScopePublicContext(rows, "en"))
	if strings.Count(b1670ScopeLines(messages, "- final_answer_mechanism_scope"), "final_answer_mechanism_scope ") != 3 {
		t.Fatal("mechanism preview changed its three-row cap")
	}
	for _, want := range []string{"omitted_rows=4", "omitted_board_groups=4", "omitted_by_leader_preview=4", "omitted_by_mechanism_preview=3"} {
		if !strings.Contains(messages, want) {
			t.Errorf("actual request lost unchanged preview accounting %q", want)
		}
	}
}
