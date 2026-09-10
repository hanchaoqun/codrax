package agent

// B1645 exercises the causal finalizer with a real completion-closed S wait.
// Native S/D positive and absent/wrong-waker qualification guards live in B1648.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1645ActualCausalIOContext(t *testing.T, state, lang string, wake bool) *types.AgentContext {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "completion.ftrace")
	var body strings.Builder
	body.WriteString("idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n")
	body.WriteString("target-41 (41) [001] .... 5.000100: block_rq_issue: 8,0 R 4096 () 123 + 8 [target]\n")
	fmt.Fprintf(&body, "target-41 (41) [001] .... 5.000120: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120\n", state)
	body.WriteString("irq-2 (2) [001] .... 5.000200: block_rq_complete: 8,0 R () 123 + 8 [0]\n")
	if wake {
		body.WriteString("irq-2 (2) [001] .... 5.000210: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001\n")
	}
	// A later sched-in without a sched_wakeup is deliberately not the missing
	// completion-to-issuer proof in the negative control.
	body.WriteString("idle-0 (0) [001] .... 5.000230: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 5.0, 5.001
	ctx := &types.AgentContext{
		RepoRoot: dir, WorkDir: dir, Language: lang,
		Mutable: types.NewMutableState("explain the target's measured causal contributions"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: lang, Intent: types.IntentTrace,
			RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit"}},
			// The actual schema name is causal_diagnosis, not causal_overview.
			// Do not use bounded_fact_set: that would test B1644's other reader.
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start, TimeEnd: &end, SourceQuote: "5.0 to 5.001",
			},
		}},
	}
	for _, view := range []string{"window_stats", "root_cause_rank"} {
		params, err := json.Marshal(map[string]any{
			"source": "path", "path": path, "view": view, "pid": 41,
			"time_start": start, "time_end": end,
			// The fixture's independently closed interval is 90 microseconds;
			// preserve the real chain's explicit sub-millisecond query threshold.
			"min_duration_ms": 0.001, "limit": 12,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("producer premise failed (%s): err=%v summary=%s", view, err, result.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary,
			Text: "Model-owned wording and values remain unchanged: 8.765 ms."}},
	})
	return ctx
}

func b1645Snapshot(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func b1645PublishedIOMechanismLine(prompt, lang string) string {
	label := tool.TraceRootCauseTypeDisplayLabel("io_latency", lang == "zh")
	labelLine := fmt.Sprintf("permitted_reader_cause_label=%q", label)
	lines := strings.Split(prompt, "\n")
	for i, line := range lines {
		if strings.Contains(line, labelLine) && i+1 < len(lines) &&
			strings.Contains(lines[i+1], "permitted_reader_mechanism_scope=") {
			return lines[i+1]
		}
	}
	return ""
}

func TestB1645ActualCausalQueryFinalContextPreservesClosedSleepIORuler(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name, state, typedState string
			wake                    bool
		}{
			{"S_closed", "S", "s_sleep", true},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				ctx := b1645ActualCausalIOContext(t, tc.state, lang, tc.wake)
				ledger := answerDocObservationLedger(ctx)
				requests := 0
				for _, record := range ledger.Records {
					if record.Predicate != "io_latency" || traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIORequestResidenceCaliber) == "" {
						continue
					}
					requests++
					if record.Value != "0.100" || record.SourceRef.Path == "" || record.SourceRef.QueryScopeID == "" {
						t.Fatalf("producer premise: exact request residence/source missing: %+v", record)
					}
					if got := traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOCompletionWokeIssuer); got != fmt.Sprint(tc.wake) {
						t.Fatalf("producer premise: completion proof=%q, want %t", got, tc.wake)
					}
					if tc.wake {
						if got := traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOIssuerBlocked); got != "0.090" {
							t.Fatalf("producer premise: closed blocked ruler=%q, want 0.090, not request residence 0.100", got)
						}
						if got := traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOIssuerBlockedState); got != tc.typedState {
							t.Fatalf("producer premise: blocked state=%q, want %s", got, tc.typedState)
						}
					}
				}
				if requests == 0 {
					t.Fatal("producer premise: actual query did not publish any IO request record")
				}
				authorities := types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
				closedAuthorities := 0
				for _, authority := range authorities {
					if authority.Type == "block_io_completion_closed_issuer_wait" {
						closedAuthorities++
						if !tc.wake || fmt.Sprintf("%.3f", authority.ObservedMS) != "0.090" {
							t.Fatalf("producer premise: original blocking qualification/ruler wrong: %+v", authority)
						}
					}
				}
				if tc.wake && closedAuthorities == 0 {
					t.Fatal("producer premise: actual closed wait did not reach blocking authority")
				}
				set := types.CompileTraceCausalProjectionSet(ledger)
				ioNodes, closedNodes := 0, 0
				for _, projection := range set.Projections {
					for _, pool := range [][]types.TraceCausalProjectionNode{
						projection.PrimaryRootCauses, projection.RankedSeats, projection.OnChainCauses,
						projection.AdjacentCauses, projection.BackgroundCauses,
					} {
						for _, node := range pool {
							if node.TypeToken != "io_latency" {
								continue
							}
							ioNodes++
							if node.ChainRelevance == "on_chain" && node.ResourceCompletionClosure {
								closedNodes++
								if !tc.wake || fmt.Sprintf("%.3f", node.EffectiveImpactMS) != "0.090" {
									t.Fatalf("producer premise: IO root used residence/no-wake as closed impact: %+v", node)
								}
							}
							if !tc.wake && (node.ResourceCompletionClosure || node.ChainRelevance == "on_chain") {
								t.Fatalf("producer premise: missing wake must not acquire IO root closure: %+v", node)
							}
						}
					}
				}
				if ioNodes == 0 || tc.wake && closedNodes == 0 {
					t.Fatalf("producer premise: root_cause_rank did not reach its actual IO mechanism lane: nodes=%d closed=%d", ioNodes, closedNodes)
				}
				before, modelBefore := b1645Snapshot(t, ctx.Mutable.TurnAArtifacts()), b1645Snapshot(t, ctx.Mutable.AnswerDocumentV2())
				projectionBefore := b1645Snapshot(t, set)
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if !strings.Contains(prompt, "## Final Trace Decision Boundary") || strings.Contains(prompt, "### Requested Runtime Fact Authority") {
					t.Fatal("entry premise: must reach the causal finalizer, not the bounded-IO fact reader")
				}
				line := b1645PublishedIOMechanismLine(prompt, lang)
				if line == "" {
					t.Fatal("entry premise: actual final instruction did not publish the IO mechanism helper")
				}
				// Only assertions below this point diagnose the B1645 teaching gap.
				// The permission is conditional on BOTH proof and a separate measured
				// blocked interval; not every IO row is promoted to that ruler merely
				// because it shares this type token. B1648 tests native negative proofs.
				wants := []string{"issue-to-completion residence", "separately measured S/D blocked interval", "independent completion-to-issuer wakeup proof", "do not add these rulers", "those identities and joins require their own structured evidence"}
				if lang == "zh" {
					wants = []string{"发起到完成的驻留时长", "独立完成唤醒证明闭合", "单独计量", "提交线程 S/D 阻塞区间", "各口径不得直接相加", "这些身份和关联必须由独立证据给出"}
				}
				for _, want := range wants {
					if !strings.Contains(line, want) {
						t.Errorf("B1645: actual IO mechanism instruction omits %q:\n%s", want, line)
					}
				}
				if got := b1645Snapshot(t, ctx.Mutable.TurnAArtifacts()); got != before {
					t.Fatal("reader teaching mutated query results, observations, sources or original values")
				}
				if got := b1645Snapshot(t, ctx.Mutable.AnswerDocumentV2()); got != modelBefore {
					t.Fatal("reader teaching rewrote the accepted model document")
				}
				if got := types.BuildTraceBlockingWallClockAuthorities(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel); !reflect.DeepEqual(got, authorities) {
					t.Fatal("reader teaching changed blocking authority")
				}
				if got := b1645Snapshot(t, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))); got != projectionBefore {
					t.Fatal("reader teaching changed projection values, ranking or causal qualifications")
				}
			})
		}
	}
}

func TestB1645IOFamilyMechanismScopesShareConditionalRulers(t *testing.T) {
	for _, zh := range []bool{false, true} {
		permission, unproved := traceFinalReaderMechanismScope("io_latency", zh)
		if permission == "" || unproved == "" {
			t.Fatal("IO scope must preserve both permission and proof limits")
		}
		for _, token := range []string{"d_state_or_io_wait", "io_wait", "fragmented_d_state_or_io_wait", "io_burst_episode"} {
			got, limit := traceFinalReaderMechanismScope(token, zh)
			if got != permission || limit != unproved {
				t.Fatalf("%s diverged from the shared conditional IO rulers", token)
			}
		}
		if got, limit := traceFinalReaderMechanismScope("unknown_io_family", zh); got != "" || limit != "" {
			t.Fatal("an unknown family must not acquire IO mechanism permission")
		}
	}
}
