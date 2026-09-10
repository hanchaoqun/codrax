package agent

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

func b1644IOCompletionContext(t *testing.T, family, closure, lang string) *types.AgentContext {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "completion.ftrace")
	issue, complete := "block_rq_issue: 8,0 R 4096 () 123 + 8 [target]", "block_rq_complete: 8,0 R () 123 + 8 [0]"
	if family == "block_bio" {
		issue, complete = "block_bio_queue: 8,0 R 123 + 8 [target]", "block_bio_complete: 8,0 R 123 + 8 [0]"
	}
	var body strings.Builder
	body.WriteString("idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n")
	fmt.Fprintf(&body, "target-41 (41) [001] .... 5.000100: %s\n", issue)
	if closure == "shared_batch_wakeup" {
		fmt.Fprintf(&body, "target-41 (41) [001] .... 5.000110: %s\n", strings.Replace(issue, "123 + 8", "456 + 8", 1))
	}
	if closure != "missing_blocking_transition" {
		body.WriteString("target-41 (41) [001] .... 5.000120: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n")
	}
	if closure == "earlier_closed_wait" {
		body.WriteString("other-3 (3) [001] .... 5.000150: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001\n")
		body.WriteString("idle-0 (0) [001] .... 5.000160: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n")
	}
	fmt.Fprintf(&body, "irq-2 (2) [001] .... 5.000200: %s\n", complete)
	if closure == "shared_batch_wakeup" {
		fmt.Fprintf(&body, "irq-2 (2) [001] .... 5.000205: %s\n", strings.Replace(complete, "123 + 8", "456 + 8", 1))
	}
	if closure != "absent_wakeup" {
		waker := "irq-2 (2)"
		if closure == "different_waker" {
			waker = "other-3 (3)"
		}
		fmt.Fprintf(&body, "%s [001] .... 5.000210: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001\n", waker)
	}
	body.WriteString("idle-0 (0) [001] .... 5.000230: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 5.0, 5.001
	ctx := &types.AgentContext{
		RepoRoot: dir, WorkDir: dir, Language: lang,
		Mutable: types.NewMutableState("compare the target's IO request and blocking measurements"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: lang, Intent: types.IntentTrace,
			RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit"}},
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart: &start, TimeEnd: &end, SourceQuote: "5.0 to 5.001"},
		}},
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 41, "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("actual trace query failed: %v; %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned wording and values remain unchanged: 8.765 ms."}},
	})
	return ctx
}

func b1644IOAuditLine(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "predicate=`io_latency`") && strings.Contains(line, "request_residence=`0.100`") {
			return line
		}
	}
	return ""
}

func TestB1644ActualQueryToFinalContextExplainsCompletionProof(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, family := range []string{"block_rq", "block_bio"} {
			for _, closure := range []string{"proven", "absent_wakeup", "different_waker", "earlier_closed_wait", "missing_blocking_transition"} {
				t.Run(lang+"/"+family+"/"+closure, func(t *testing.T) {
					ctx := b1644IOCompletionContext(t, family, closure, lang)
					ledger := answerDocObservationLedger(ctx)
					var pairs []types.ObservationRecord
					for _, r := range ledger.Records {
						if r.Predicate == "io_latency" && traceQueryObservationSupplementNoteValue(r, types.TraceNoteKeyIORequestResidenceCaliber) != "" {
							pairs = append(pairs, r)
						}
					}
					if len(pairs) != 1 || pairs[0].Value != "0.100" || pairs[0].SourceRef.Path == "" || pairs[0].SourceRef.QueryScopeID == "" {
						t.Fatalf("actual source/one request/residence premise failed: %+v", pairs)
					}
					proof := closure == "proven"
					if got := traceQueryObservationSupplementNoteValue(pairs[0], types.TraceNoteKeyIOCompletionWokeIssuer); got != fmt.Sprint(proof) {
						t.Fatalf("native %s closure premise: got %q", closure, got)
					}
					authorities := types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
					if proof && (len(authorities) != 1 || fmt.Sprintf("%.3f", authorities[0].ObservedMS) != "0.090") || !proof && len(authorities) != 0 {
						t.Fatalf("original positive blocking qualification changed: %+v", authorities)
					}
					before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
					modelBefore, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
					line := b1644IOAuditLine((&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil))
					if line == "" || !strings.Contains(line, "completion_woke_issuer=`"+fmt.Sprint(proof)+"`") {
						t.Fatalf("real final context lost original IO audit fields: %s", line)
					}
					want := "No independent completion-to-issuer wakeup proof was established for this request; this does not prove that no wakeup occurred"
					if lang == "zh" {
						want = "未形成该请求独立的完成方唤醒发送线程证明；这不等于已证明未唤醒"
					}
					if proof {
						want = "The completion emitter's directed wakeup of the issuing thread is proven"
						if lang == "zh" {
							want = "已证明完成事件发出者定向唤醒发送线程"
						}
					}
					if !strings.Contains(line, want) {
						t.Errorf("final-context proof Boolean lacks its reader meaning %q: %s", want, line)
					}
					after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
					if string(before) != string(after) {
						t.Fatal("reader teaching changed the original query, observations, receipts or values")
					}
					modelAfter, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
					if string(modelBefore) != string(modelAfter) {
						t.Fatal("reader teaching rewrote the accepted model document")
					}
					if got := types.BuildTraceBlockingWallClockAuthorities(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel); !reflect.DeepEqual(got, authorities) {
						t.Fatal("reader teaching changed blocking authority")
					}
				})
			}
		}
	}
}

func TestB1644ActualBatchWakeupDoesNotGiveEveryRequestAnIndependentProof(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, family := range []string{"block_rq", "block_bio"} {
			t.Run(lang+"/"+family, func(t *testing.T) {
				ctx := b1644IOCompletionContext(t, family, "shared_batch_wakeup", lang)
				ledger := answerDocObservationLedger(ctx)
				authorities := types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
				if len(authorities) != 1 || len(authorities[0].Occurrences) != 1 || fmt.Sprintf("%.3f", authorities[0].ObservedMS) != "0.090" {
					t.Fatalf("one real batch wake must keep exactly one proven wait: %+v", authorities)
				}
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				falseLine := b1644IOAuditLine(prompt)
				trueLine := ""
				for _, line := range strings.Split(prompt, "\n") {
					if strings.Contains(line, "predicate=`io_latency`") && strings.Contains(line, "request_residence=`0.095`") {
						trueLine = line
					}
				}
				if !strings.Contains(falseLine, "completion_woke_issuer=`false`") || !strings.Contains(trueLine, "completion_woke_issuer=`true`") {
					t.Fatalf("shared wake's two independent request facts disappeared: false=%s true=%s", falseLine, trueLine)
				}
				negative, positive := "does not prove that no wakeup occurred", "directed wakeup of the issuing thread is proven"
				if lang == "zh" {
					negative, positive = "这不等于已证明未唤醒", "已证明完成事件发出者定向唤醒发送线程"
				}
				if !strings.Contains(falseLine, negative) || strings.Contains(falseLine, "issuer_blocked=`") || !strings.Contains(trueLine, positive) || !strings.Contains(trueLine, "issuer_blocked=`0.090`") {
					t.Fatalf("batch ambiguity was turned into negative wake/positive blocking: false=%s true=%s", falseLine, trueLine)
				}
			})
		}
	}
}

func TestB1644FinalContextKeepsMissingProofAndNonpositiveBlockingUnknown(t *testing.T) {
	// Compatibility row variants are not new native measurements: use one
	// actual producer receipt, then independently exercise absent/invalid legacy
	// proof metadata and proof-without-positive-measurement presentation.
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct{ name, proof, blocked string }{
			{"missing", "", ""}, {"unrecognized", "undetermined", ""},
			{"true_missing_duration", "true", ""}, {"true_zero_duration", "true", "0.000"},
			{"true_negative_duration", "true", "-0.090"}, {"false_with_duration", "false", "0.090"},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				ctx := b1644IOCompletionContext(t, "block_rq", "proven", lang)
				a := ctx.Mutable.TurnAArtifacts()
				var row types.ObservationRecord
				for _, r := range a.ToolResults[0].Observations {
					if r.Predicate == "io_latency" && traceQueryObservationSupplementNoteValue(r, types.TraceNoteKeyIORequestResidenceCaliber) != "" {
						row = r
					}
				}
				if row.ID == "" {
					t.Fatal("actual request premise missing")
				}
				var notes []string
				for _, note := range row.RichNotes {
					if !strings.HasPrefix(note, types.TraceNoteKeyIOCompletionWokeIssuer+"=") && !strings.HasPrefix(note, types.TraceNoteKeyIOIssuerBlocked+"=") {
						notes = append(notes, note)
					}
				}
				if tc.proof != "" {
					notes = append(notes, types.TraceNoteKeyIOCompletionWokeIssuer+"="+tc.proof)
				}
				if tc.blocked != "" {
					notes = append(notes, types.TraceNoteKeyIOIssuerBlocked+"="+tc.blocked)
				}
				row.RichNotes = notes
				a.ToolResults[0].Observations = []types.ObservationRecord{row}
				ctx.Mutable = types.NewMutableState("compatibility snapshot")
				ctx.Mutable.SetTurnAArtifacts(*a)
				ledger := answerDocObservationLedger(ctx)
				if got := types.BuildTraceBlockingWallClockAuthorities(ledger, &ctx.AnalysisIR.RequestModel); len(got) != 0 {
					t.Fatalf("unproven/nonpositive compatibility row must not acquire blocking authority: %+v", got)
				}
				before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				line := b1644IOAuditLine((&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil))
				want := "a missing measurement is not zero"
				if lang == "zh" {
					want = "缺少该测量时不能记为零"
				}
				if line == "" || !strings.Contains(line, want) {
					t.Fatalf("proof without a positive blocking measurement needs a nonzero-inference boundary: %s", line)
				}
				if tc.proof == "" || tc.proof == "undetermined" {
					unknown := "unpublished or unrecognized"
					if lang == "zh" {
						unknown = "未发布或未识别"
					}
					if !strings.Contains(line, unknown) || strings.Contains(line, "completion_woke_issuer=`false`") || strings.Contains(line, "completion_woke_issuer=`true`") {
						t.Fatalf("unknown proof was rewritten into a Boolean event fact: %s", line)
					}
				}
				after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
				if string(before) != string(after) {
					t.Fatal("proof explanation normalized or rewrote the source row")
				}
			})
		}
	}
}
