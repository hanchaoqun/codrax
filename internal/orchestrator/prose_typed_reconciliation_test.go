package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func typedReconciliationHarness(t *testing.T, prose string) (*Orchestrator, *types.AnswerDocumentV2) {
	t.Helper()
	const selected = "selected_window=34579.472865..34579.587805"
	rank := lexiconBoardRankRecord(1, "worker-7", "runnable_wait",
		selected, "rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"impact_ms=30.000", "cumulative_impact_ms=30.000", "effective_impact_ms=30.000",
		"type=runnable_wait", "fix_direction=lock_priority", "rank_board_target=app-10")
	rank.Value = "30.000"
	rank.SupportRefs = []string{"sample.systrace:10-20"}
	path := psgTraceRecord("trace_query:q#wakeup_chain:path", "wakeup_chain:path", "0.000", selected, "path=worker-7 -> app-10")
	path.Predicate = "wakeup_chain"
	path.Subject = "app-10"
	path.Object = "worker-7 -> app-10"
	path.SupportRefs = []string{"sample.systrace:21-22"}
	account := p6AccountRecord("app-10", 20, 30, 64.940, 0, 0, 114.940, 114.940)
	account.SupportRefs = []string{"sample.systrace:23-24"}
	account.Span.LineStart, account.Span.LineEnd = 23, 24

	mut := psgTraceMutable(rank, path, account)
	bus := psgBus(mut)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	doc := psgProseDoc(prose)
	result, err := tool.ApplyAndPersistMutation(bus, "typed_reconciliation_test", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("materialize trace projection: result=%+v err=%v", result, err)
	}
	shipped := mut.AnswerDocumentV2()
	if shipped == nil {
		t.Fatal("missing shipped document")
	}
	return &Orchestrator{busCtx: bus}, shipped
}

func TestTypedReconciliationPublishesNeutralEvidenceBackedRows(t *testing.T) {
	prose := "核心丢帧原因是 worker-7。状态对账 20.000+30.000+64.940=114.940ms。\n\n修复方向:\n- 减少锁竞争\n- 缩短业务 span"
	o, shipped := typedReconciliationHarness(t, prose)
	typedRows := tool.RuntimeTraceReconciliationRows(o.busCtx)
	before, _ := json.Marshal(shipped)
	lines := o.collectSystemCrossCheckFindings()
	after, _ := json.Marshal(o.busCtx.Mutable.AnswerDocumentV2())
	if string(before) != string(after) {
		t.Fatalf("typed reconciliation must not mutate model/system blocks")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"对账参考:",
		"running 20.000ms + runnable 30.000ms + sleep 64.940ms",
		"根因排序#1 worker-7",
		"根因排序#1 的修向=锁与优先级",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("neutral typed reconciliation missing %q:\n%s\ntyped rows=%+v", want, joined, typedRows)
		}
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "对账参考:") && !strings.HasPrefix(line, "同尺并置:") {
			continue
		}
		if !strings.Contains(line, "[E") {
			t.Fatalf("every reconciliation row must carry a visible E# pointer: %s", line)
		}
		for _, banned := range []string{"模型", "错误", "不符", "遗漏", "正文错", "正文将", "正文称"} {
			if strings.Contains(line, banned) {
				t.Fatalf("neutral row contains judgment word %q: %s", banned, line)
			}
		}
	}
}

func TestTypedReconciliationSelectorsNeverEmitModelClaimText(t *testing.T) {
	row := tool.RuntimeTraceReconciliationRow{
		Kind: tool.RuntimeTraceReconciliationRankOne, Subject: "typed-worker-7",
		Rank: 1, EffectiveMS: 12.345, CauseToken: "priority_inversion_gated",
		FixDirection: "lock_priority", EvidenceTag: "E2",
	}
	modelClaim := "invented-model-cause-999"
	lines := []proseScalarBindingFinding{
		renderRankOneReconciliation(row),
		renderDirectionReconciliation(row),
	}
	for _, line := range lines {
		for _, face := range []string{line.entryZH, line.entry} {
			if strings.Contains(face, modelClaim) || !strings.Contains(face, "[E2]") {
				t.Fatalf("render must use typed row only: %q", face)
			}
		}
	}
	if got := proseSelectedTypedReconciliationFindings(psgProseDoc("普通证据说明，无算式、首因或修向清单。"), psgBus(psgTraceMutable())); len(got) != 0 {
		t.Fatalf("no selector and no typed projection must stay silent: %+v", got)
	}
}

func TestTypedReconciliationEnglishFacesAreNeutral(t *testing.T) {
	account := tool.RuntimeTraceReconciliationRow{
		Kind: tool.RuntimeTraceReconciliationTargetState, Subject: "app-10", EvidenceTag: "E4",
		WindowMS: 100, RunningMS: 20, RunnableMS: 30, SleepMS: 50, TotalMS: 100,
	}
	rank := tool.RuntimeTraceReconciliationRow{
		Kind: tool.RuntimeTraceReconciliationRankOne, Subject: "worker-7", EvidenceTag: "E1",
		Rank: 1, EffectiveMS: 30, CauseToken: "priority_inversion_gated", FixDirection: "lock_priority",
	}
	for _, face := range []string{
		renderTargetStateReconciliation(account).entry,
		renderRankOneReconciliation(rank).entry,
		renderDirectionReconciliation(rank).entry,
	} {
		if (!strings.HasPrefix(face, "Reconciliation reference:") && !strings.HasPrefix(face, "Like-for-like reference:")) || !strings.Contains(face, "[E") {
			t.Fatalf("unexpected EN neutral face: %s", face)
		}
		for _, banned := range []string{"wrong", "error", "missing", "omitted", "model says"} {
			if strings.Contains(strings.ToLower(face), banned) {
				t.Fatalf("EN neutral face contains verdict %q: %s", banned, face)
			}
		}
	}
}
