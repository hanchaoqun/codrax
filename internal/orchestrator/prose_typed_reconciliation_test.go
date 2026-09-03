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

func TestTypedReconciliationWrongEquationShapeAloneSelectsTypedAccount(t *testing.T) {
	// None of these model-authored operands is an exact three-decimal value
	// from the typed account assembled by typedReconciliationHarness. This
	// therefore pins the equation-shape selector itself instead of accidentally
	// passing through the exact-value selector arm.
	prose := "状态估算 82.1 + 9.1 = 91.2ms。"
	for _, typedFace := range []string{"20.000", "30.000", "64.940", "114.940"} {
		if strings.Contains(prose, typedFace) {
			t.Fatalf("fixture accidentally contains typed exact value %q", typedFace)
		}
	}

	o, shipped := typedReconciliationHarness(t, prose)
	before, _ := json.Marshal(shipped)
	lines := o.collectSystemCrossCheckFindings()
	after, _ := json.Marshal(o.busCtx.Mutable.AnswerDocumentV2())
	if string(before) != string(after) {
		t.Fatalf("equation selector must not mutate model/system blocks")
	}

	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"对账参考:",
		"running 20.000ms + runnable 30.000ms + sleep 64.940ms",
		"= 114.940ms(分析窗 114.940ms)",
		"[E",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("wrong-equation shape did not select neutral typed account %q:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{"82.1", "9.1", "91.2", "模型", "错误", "不符"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("published row copied or judged noisy prose token %q:\n%s", forbidden, joined)
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

// TestTypedReconciliationTargetStateRowSpeaksTheQualifiedNonIOLaneWord —
// §40.49 合流复核收编 (G-target-state #1): the reconciliation row prints the
// five DISJOINT lanes, so its D term is the exclusive non-IO lane and must be
// labelled with the single-source customer-face word
// (tool.TraceStateNonIODStateWord), byte-equal with the body wall-clock
// partition / wait-coverage / fact-juxtaposition faces. The bare word
// "D-state" on a customer face is reserved for the published uninterruptible
// fold, which the body four-state line prints as "D-state …(其中 IO等待 …)".
//
// EVOLUTION RECORD (red→green): before the fix the row spelled the non-IO
// lane by hand as "+ D-state 4.039ms" while the same answer's body four-state
// line said "D-state 5.379ms(…,其中 IO等待 1.340ms)" — two calibers under one
// word on one customer page. This test was written first and failed on both
// the "+ D-state " ban and the qualified-word requirement; the renderer fix
// turned it green.
func TestTypedReconciliationTargetStateRowSpeaksTheQualifiedNonIOLaneWord(t *testing.T) {
	// Unit face: exact bytes of both languages on the ruling fixture
	// (running 60 / runnable 20 / sleep 29.561 / d_state 4.039 / io_wait
	// 1.340 = 114.940 = window).
	row := tool.RuntimeTraceReconciliationRow{
		Kind: tool.RuntimeTraceReconciliationTargetState, ArtifactLabel: "huadong.systrace",
		Subject: "app-10", EvidenceTag: "E2",
		WindowMS: 114.940, RunningMS: 60, RunnableMS: 20, SleepMS: 29.561,
		DStateMS: 4.039, IOWaitMS: 1.340, TotalMS: 114.940,
	}
	f := renderTargetStateReconciliation(row)
	wantZH := "对账参考: 工件 huadong.systrace · app-10 全窗状态分区 running 60.000ms + runnable 20.000ms + sleep 29.561ms + 非 IO D-state 4.039ms + io_wait 1.340ms = 114.940ms(分析窗 114.940ms) [E2]"
	if f.entryZH != wantZH {
		t.Fatalf("ZH reconciliation row bytes drifted:\n got %q\nwant %q", f.entryZH, wantZH)
	}
	wantEN := "Reconciliation reference: artifact huadong.systrace · app-10 full-window state partition: running 60.000ms + runnable 20.000ms + sleep 29.561ms + non-IO D-state 4.039ms + io_wait 1.340ms = 114.940ms (analysis window 114.940ms) [E2]"
	if f.entry != wantEN {
		t.Fatalf("EN reconciliation row bytes drifted:\n got %q\nwant %q", f.entry, wantEN)
	}
	// The label is the single-source word, not a hand copy.
	if !strings.Contains(f.entryZH, tool.TraceStateNonIODStateWord(true)+" 4.039ms") ||
		!strings.Contains(f.entry, tool.TraceStateNonIODStateWord(false)+" 4.039ms") {
		t.Fatalf("row must label its D term with tool.TraceStateNonIODStateWord: zh=%q en=%q", f.entryZH, f.entry)
	}
	for _, face := range []string{f.entryZH, f.entry} {
		if strings.Contains(face, "+ D-state ") {
			t.Fatalf("bare \"D-state\" term on the reconciliation row spells the non-IO lane under the fold's word: %q", face)
		}
	}

	// Whole-answer face: body four-state line (fold, labelled as the fold)
	// and appendix reconciliation row (non-IO lane, labelled as such) render
	// from ONE account without a bare-word contradiction.
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
	account := p6AccountRecord("app-10", 60, 20, 29.561, 4.039, 0, 114.940, 114.940)
	account.RichNotes = append(account.RichNotes, types.TraceNoteKeyIOWait+"=1.340")
	account.SupportRefs = []string{"sample.systrace:23-24"}
	account.Span.LineStart, account.Span.LineEnd = 23, 24

	mut := psgTraceMutable(rank, path, account)
	bus := psgBus(mut)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	doc := psgProseDoc("窗口内 io_wait 1.340ms 已单列。")
	result, err := tool.ApplyAndPersistMutation(bus, "typed_reconciliation_nonio_word_test", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("materialize trace projection: result=%+v err=%v", result, err)
	}
	shipped := mut.AnswerDocumentV2()
	if shipped == nil {
		t.Fatal("missing shipped document")
	}
	body, _ := json.Marshal(shipped)
	if !strings.Contains(string(body), "+ D-state 5.379ms(") || !strings.Contains(string(body), "其中 IO等待 1.340ms") {
		t.Fatalf("body four-state line must publish the fold under the bare word with its IO breakdown, got document without it")
	}
	o := &Orchestrator{busCtx: bus}
	joined := strings.Join(o.collectSystemCrossCheckFindings(), "\n")
	if !strings.Contains(joined, "非 IO D-state 4.039ms + io_wait 1.340ms") {
		t.Fatalf("appendix reconciliation row must qualify its non-IO D term:\n%s", joined)
	}
	if strings.Contains(joined, "+ D-state ") {
		t.Fatalf("appendix must not spell the non-IO lane under the bare fold word:\n%s", joined)
	}
}
