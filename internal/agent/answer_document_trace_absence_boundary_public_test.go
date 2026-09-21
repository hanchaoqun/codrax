package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// H4's actual producer publishes three different populations together: an
// empty D/IO-state roster, 50 blocked-reason records, and five closed Binder
// waits. The final instruction must not teach that the first empties the rest.
func TestTraceWaitAbsenceBoundaryH4PublicFinalInstruction(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const subject = ".ugc.aweme.lite-17267"
	start, end := 13762.791708, 13763.024898
	dir := t.TempDir()
	params, _ := json.Marshal(map[string]any{
		"source": "attached_trace", "view": "window_stats", "pid": 17267,
		"time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace",
	})
	queryBus := &types.BusContext{
		RepoRoot: dir, WorkDir: dir, AttachedHitrace: string(data), AttachedHitraceSource: "harmony_hitrace",
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "attachment"}}},
	}
	result, err := (&tool.TraceQuery{}).Execute(queryBus, params)
	if err != nil || !result.Success {
		t.Fatalf("public H4 query: %v; %s", err, result.Summary)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := tracePrincipalValueAuthorityTestContext(subject, 17267, result.Observations)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			rm := &ctx.AnalysisIR.RequestModel
			rm.Intent, rm.Language = types.IntentExplain, lang
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
				Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{
					types.RuntimeQuestionFactTargetSchedulerState,
					types.RuntimeQuestionFactTargetWaitOccurrences,
				},
			}
			rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end,
				SourceQuote: "13762.791708s 到 13763.024898s 窗口内",
			}
			ctx.Mutable.SetRequestModel(*rm)
			ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
				DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned conclusion remains unchanged."}},
			})
			ctx = ctxbuilder.BuildAgentContext(&types.BusContext{
				RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR,
			}, types.AgentFinalizer, types.StageFinalize)
			ledger := answerDocObservationLedger(ctx)
			waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
			if len(waits) != 1 || waits[0].Subject != subject || waits[0].Count != 0 ||
				waits[0].WindowStartTs != start || waits[0].WindowEndTs != end {
				t.Fatalf("H4 must publish a complete zero D/IO roster for the exact target/window: %+v", waits)
			}
			census, binder := false, false
			for _, record := range ledger.Records {
				if record.Subject != subject {
					continue
				}
				if record.Predicate == "blocked_reason_census" && record.Value == "50" {
					census = true
				}
				if record.Predicate == "target_binder_wait_inventory" && record.Value == "3.094" &&
					strings.Contains(record.Summary, "Verified closed Binder waits: 5,") {
					binder = true
				}
			}
			if !census || !binder {
				t.Fatalf("public H4 witness missing: blocked-reason50=%t Binder5/3.094ms=%t", census, binder)
			}
			before, _ := json.Marshal([]any{result, ledger, types.CompileTraceCausalProjectionSet(ledger), ctx.Mutable.AnswerDocumentV2()})
			instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{
				"## Runtime Trace Principal Values — Final Typed Recap",
				"occurrence_count=0; d_state_occurrences=0; io_wait_occurrences=0; sleep_iowait_occurrences=0;",
				"blocked_reason_records=50", "Verified closed Binder waits: 5, union=3.094ms",
			} {
				if !strings.Contains(instruction, want) {
					t.Fatalf("actual final instruction lost coexistence witness %q", want)
				}
			}
			t.Log("public H4 final instruction: D/IO roster=0, blocked-reason records=50, Binder waits=5/3.094ms")
			assertTraceWaitAbsenceBoundary(t, instruction, lang)
			afterLedger := answerDocObservationLedger(ctx)
			after, _ := json.Marshal([]any{result, afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), ctx.Mutable.AnswerDocumentV2()})
			if string(before) != string(after) {
				t.Fatal("soft absence guidance changed observations, projection, or model document")
			}
		})
	}
}

func assertTraceWaitAbsenceBoundary(t *testing.T, got, lang string) {
	t.Helper()
	wants := []string{
		"Absence boundary: If a complete D/IO-state wait roster reports zero",
		"only no matching D/io_wait or S-with-IO-marker intervals in that roster",
		"does not imply an empty blocked-reason census, Binder wait inventory, or independently completion-closed IO wait inventory",
		"Missing or unavailable rosters are not zero",
		"does not classify an S interval as cooperative or voluntary sleep",
		"Name a mechanism only from separate syscall, span, blocked-reason, wakeup, or dependency evidence",
	}
	if lang == "zh" {
		wants = []string{
			"缺失边界：若完整的 D/IO 状态等待清单报告为零",
			"仅表示该清单内没有匹配的 D/io_wait 或带 IO 等待标记的 S 状态区间",
			"不表示阻塞原因记录、Binder 等待清单或独立 IO 完成闭合等待清单为空",
			"清单缺失或不可用不等于零",
			"不能据此把 S 状态判成主动休眠",
			"只有独立的系统调用、业务片段、阻塞原因、唤醒或依赖关系证据才能命名具体等待机制",
		}
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("missing conditional narrow-roster boundary %q", want)
		}
	}
	for _, bad := range []string{
		"缺失边界：目标窗口内没有匹配的等待原因记录",
		"Absence boundary: no matching target-window wait-reason records",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("narrow D/IO absence incorrectly stated as wait-reason absence: %s", bad)
		}
	}
}

func TestTraceWaitAbsenceBoundaryDoesNotInventZero(t *testing.T) {
	inventory := b1607BinderPromptRecords()[0]
	inventory.SourceRef.PayloadRef = "wait-query.json"
	count := 1
	roster := types.ObservationRecord{
		ID: "query#target_window_wait_occurrences", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		GroundingPolicy: types.ClaimGroundingHard, SourceRef: inventory.SourceRef,
		Subject: inventory.Subject, Predicate: "target_window_wait_occurrences", Object: "complete",
		Span: types.ObservationSpan{StartTs: 10, EndTs: 11}, Value: "1", ResultCount: &count,
		RichNotes: []string{types.TraceNoteKeySelectedWindow + "=10.000000..11.000000"},
	}
	leaf := types.ObservationRecord{
		ID: "query#target_window_wait_occurrence:1", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		GroundingPolicy: types.ClaimGroundingHard, SourceRef: inventory.SourceRef,
		Subject: inventory.Subject, Predicate: "target_window_wait_occurrence", Object: "state=d_sleep;iowait=0;caller=kernel_site",
		Span: types.ObservationSpan{StartTs: 10.1, EndTs: 10.103}, Value: "3.000", Unit: "ms",
	}
	unavailable := roster
	unavailable.Object, unavailable.Value, unavailable.ResultCount = "unavailable", "0", nil
	for _, tc := range []struct {
		name    string
		records []types.ObservationRecord
		wantOne bool
	}{
		{"nonzero", []types.ObservationRecord{inventory, roster, leaf}, true},
		{"missing_roster", []types.ObservationRecord{inventory}, false},
		{"unavailable_roster", []types.ObservationRecord{inventory, unavailable}, false},
		{"missing_data", nil, false},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				ctx := tracePrincipalValueAuthorityTestContext("client-100", 100, tc.records)
				ctx.Language = lang
				before, _ := json.Marshal(answerDocObservationLedger(ctx))
				got := renderAnswerDocTracePrincipalValueAuthority(ctx)
				if len(tc.records) == 0 {
					if got != "" {
						t.Fatal("absent evidence fabricated a principal recap")
					}
					return
				}
				assertTraceWaitAbsenceBoundary(t, got, lang)
				if !strings.Contains(got, "Verified closed Binder waits: 5, union=3.094ms") {
					t.Fatal("D/IO roster status suppressed independent Binder evidence")
				}
				if strings.Contains(got, "principal_wait_occurrences:") != tc.wantOne {
					t.Fatalf("missing/unavailable roster must not mint exact zero authority: %s", got)
				}
				if tc.wantOne && (!strings.Contains(got, "occurrence_count=1; d_state_occurrences=1;") || !strings.Contains(got, "wall_clock_sum=3.000ms")) {
					t.Fatalf("nonzero D/IO authority lost its own count/ruler: %s", got)
				}
				after, _ := json.Marshal(answerDocObservationLedger(ctx))
				if string(before) != string(after) {
					t.Fatal("absence guidance changed typed records")
				}
			})
		}
	}
}
