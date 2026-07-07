// answer_document_mutation_runtime_sg_test.go — SG 软引导批 (账本
// real_trace_campaign_20260705.md §10.2/§11.2/§12.2) item 4: named
// holder/peer next-step synthesis (§10-D1② + Q4-K 修5).
//
// Acceptance shapes come straight from the two customer specimens:
//   - q1 (binder): critical_blocking type=binder_wait with a RESOLVED peer
//     label in Object (OS_IPC_4939_437-43722) → a named 对端 drilldown row;
//   - q4 (lock): blocking_span with the parsed §7.30.3 D1 payload
//     (blocking_kind=monitor_contention, peer=NetworkKit_AssetsUtil_Operate_
//     0-42067, holder_site=AssetManager.list(AssetManager.java:1258)) → a
//     named 持有者 row carrying the holding site.
//
// The generic s_sleep template row ("排查反复唤醒它的对端线程、binder等待、
// 锁与条件变量等待") yields ONLY when a named row actually rendered; an
// unresolved peer keeps the legacy list byte-identical.
package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sgCriticalBlockingRecord(id, subject, object string, notes ...string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "critical_blocking",
		Subject:         subject,
		Object:          object,
		Value:           "112.223",
		Unit:            "ms",
		RichNotes:       notes,
	}
}

// sgGenericSleepRecord carries the generic s_sleep next-step template — the
// row the named holder/peer entries must displace.
func sgGenericSleepRecord(id string) types.ObservationRecord {
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "state_churn",
		Subject: "main-16547", Value: "112.000", Unit: "ms",
		RichNotes: []string{
			"next_step=investigate the peer threads that repeatedly wake it, binder waits, lock and condition waits",
			"next_step_kind=s_sleep",
		},
	}
}

func sgNextStepTexts(t *testing.T, bus *types.BusContext) []string {
	t.Helper()
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}
	return texts
}

// TestRuntimeTraceNextStepNamedLockHolder — q4 acceptance shape: the parsed
// lock payload synthesizes a named 持有者 row (holder + holding site), and the
// generic s_sleep template stands down.
func TestRuntimeTraceNextStepNamedLockHolder(t *testing.T) {
	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			sgCriticalBlockingRecord("cb-1", "main-16547", "NetworkKit_AssetsUtil_Operate_0-42067",
				"type=blocking_span",
				"blocking_kind=monitor_contention",
				"peer=NetworkKit_AssetsUtil_Operate_0-42067",
				"holder_site=AssetManager.list(AssetManager.java:1258)",
				"chain_relevance=on_chain",
			),
			sgGenericSleepRecord("ns-1"),
		},
	}}
	texts := sgNextStepTexts(t, bus)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 在重叠窗执行 trace_query view=… → 在重叠的查询窗内查看其线程时间线与唤醒链(…) (下一步族)
	want := "对持有者 NetworkKit_AssetsUtil_Operate_0-42067(持有点 AssetManager.list(AssetManager.java:1258))在重叠的查询窗内查看其线程时间线与唤醒链(thread_timeline/wakeup_chain)"
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, want) {
		t.Fatalf("named lock-holder row missing:\nwant %q\n got %q", want, joined)
	}
	if strings.Contains(joined, "排查反复唤醒它的对端线程") {
		t.Fatalf("generic s_sleep template must yield to the named holder row:\n%s", joined)
	}
}

// TestRuntimeTraceNextStepNamedBinderPeer — q1 acceptance shape: a resolved
// binder_wait peer synthesizes a named 对端 row (no holding site), generic
// template yields; duplicate records dedupe to one row (R4-3 typed key).
func TestRuntimeTraceNextStepNamedBinderPeer(t *testing.T) {
	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			sgCriticalBlockingRecord("cb-1", "main-4939", "OS_IPC_4939_437-43722",
				"type=binder_wait", "peer=OS_IPC_4939_437-43722", "chain_relevance=on_chain",
			),
			sgCriticalBlockingRecord("cb-2", "main-4939", "OS_IPC_4939_437-43722",
				"type=binder_wait", "peer=OS_IPC_4939_437-43722", "chain_relevance=on_chain",
			),
			sgGenericSleepRecord("ns-1"),
		},
	}}
	texts := sgNextStepTexts(t, bus)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 在重叠窗执行 trace_query view=… → 在重叠的查询窗内查看其线程时间线与唤醒链(…) (下一步族)
	want := "对对端 OS_IPC_4939_437-43722 在重叠的查询窗内查看其线程时间线与唤醒链(thread_timeline/wakeup_chain)"
	count := 0
	for _, text := range texts {
		if text == want {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("named binder-peer row must appear exactly once, got %d in %q", count, texts)
	}
	if strings.Contains(strings.Join(texts, "\n"), "排查反复唤醒它的对端线程") {
		t.Fatalf("generic s_sleep template must yield to the named peer row:\n%v", texts)
	}
}

// TestRuntimeTraceNextStepUnresolvedPeerKeepsGenericTemplate — the
// unknown-thread sentinel admits NO named row, and the generic s_sleep
// guidance keeps its seat (legacy behavior preserved on the unresolved
// shape: soft guidance never fabricates a peer).
func TestRuntimeTraceNextStepUnresolvedPeerKeepsGenericTemplate(t *testing.T) {
	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			sgCriticalBlockingRecord("cb-1", "main-4939", "unknown-thread",
				"type=binder_wait", "peer=unknown-thread", "chain_relevance=on_chain",
			),
			sgGenericSleepRecord("ns-1"),
		},
	}}
	texts := sgNextStepTexts(t, bus)
	joined := strings.Join(texts, "\n")
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: negative pin migrated 在重叠窗执行 trace_query view=… → 在重叠的查询窗内查看其线程时间线与唤醒链 (下一步族)
	if strings.Contains(joined, "在重叠的查询窗内查看其线程时间线与唤醒链") {
		t.Fatalf("unresolved peer must not synthesize a named row:\n%s", joined)
	}
	if !strings.Contains(joined, "排查反复唤醒它的对端线程") {
		t.Fatalf("generic s_sleep template must survive when no named row rendered:\n%s", joined)
	}
}

// TestRuntimeTraceNextStepNamedPeerEnglishWording — EN surface keeps the
// same admission logic with the English holder/peer wording.
func TestRuntimeTraceNextStepNamedPeerEnglishWording(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Language = "en"
	bus.AnalysisIR = &types.AnalysisIR{AnswerContract: types.AnswerContract{Language: "en"}}
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			sgCriticalBlockingRecord("cb-1", "main-16547", "HolderThread-42067",
				"type=blocking_span",
				"blocking_kind=monitor_contention",
				"peer=HolderThread-42067",
				"holder_site=SomeManager.list(SomeManager.java:1258)",
				"chain_relevance=on_chain",
			),
		},
	}}
	texts := sgNextStepTexts(t, bus)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: Run trace_query view=… over the overlapping window → Inspect the thread timeline and wakeup chain (…) over the overlapping query window (下一步族 EN)
	want := "Inspect the thread timeline and wakeup chain (thread_timeline/wakeup_chain) of the lock holder HolderThread-42067 over the overlapping query window (holding site SomeManager.list(SomeManager.java:1258))"
	if !strings.Contains(strings.Join(texts, "\n"), want) {
		t.Fatalf("EN named holder row missing:\nwant %q\n got %q", want, texts)
	}
}

// TestBLKHolderSubjectRowRendersHoldNotReversedWait — BLK §15.C bug②/③ (q6 东湖
// doFrame 复跑): a resolved lock rank row whose SUBJECT is the holder carries
// subject_is_lock_holder=true. The renderer must read it as a HOLD ("持锁阻塞",
// waiter=BlockingPeer), NEVER the reversed "锁竞争等待(持有者 <waiter>)"; and the
// next-step must NOT name the waiter as a holder to drill into. After the
// §15.C ① single-publication fold this rank node is the ONLY observation the
// physical span publishes (the waiter-subject critical_blocking twin is
// skipped at emission), so the holder drilldown comes from THIS node's own
// subject — one hint, the true holder.
func TestBLKHolderSubjectRowRendersHoldNotReversedWait(t *testing.T) {
	// E3 shape: holder=RxComputation is the subject, waiter=main is BlockingPeer.
	e3 := types.TraceCausalProjectionNode{
		Subject:                 "#RxComputationT-16816",
		BlockingKind:            "monitor_contention",
		BlockingPeer:            ".ugc.aweme.lite-16547",
		BlockingHolderSite:      "AssetManager.list(AssetManager.java:1258)",
		BlockingSubjectIsHolder: true,
	}
	nameZH := runtimeTraceCausalProjectionBlockingName(e3, true)
	if strings.Contains(nameZH, "持有者") || strings.Contains(nameZH, "锁竞争等待") {
		t.Fatalf("holder-subject row must not render the reversed lock-WAIT wording, got %q", nameZH)
	}
	if !strings.Contains(nameZH, "持锁阻塞") || !strings.Contains(nameZH, ".ugc.aweme.lite-16547") {
		t.Fatalf("holder-subject row must render a HOLD naming the waiter, got %q", nameZH)
	}
	cellZH := runtimeTraceCausalProjectionNodeSubjectCell(e3, true)
	if !strings.Contains(cellZH, "#RxComputationT-16816") || !strings.Contains(cellZH, "持锁阻塞") {
		t.Fatalf("holder-subject detail cell must lead with the holder subject and a HOLD, got %q", cellZH)
	}
	if strings.Contains(cellZH, "持有者 .ugc.aweme.lite-16547") {
		t.Fatalf("holder-subject detail cell must not name the waiter as holder, got %q", cellZH)
	}
	// bug②, shape cell: the holder was never blocked — its duration is 持锁,
	// not 阻塞.
	if shape, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(e3, true); shape != "锁竞争·持锁" {
		t.Fatalf("holder-subject shape cell must say 持锁, got %q", shape)
	}
	if shape, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(e3, false); shape != "lock contention · holder" {
		t.Fatalf("EN holder-subject shape cell must say holder, got %q", shape)
	}
	// bug③: the drilldown names the HOLDER (this node's subject) — never the
	// reversed "对持有者 <waiter>".
	peer, site, lockShape := runtimeTraceNextStepResolvedPeer(e3)
	if peer != "#RxComputationT-16816" || !lockShape {
		t.Fatalf("holder-subject node must name ITSELF (the holder) as the drilldown target, got peer=%q lock=%v", peer, lockShape)
	}
	if site != "AssetManager.list(AssetManager.java:1258)" {
		t.Fatalf("holder drilldown must keep the holding site, got %q", site)
	}
	// A sentinel subject yields no row at all rather than a wrong identity.
	broken := e3
	broken.Subject = ""
	if p, _, l := runtimeTraceNextStepResolvedPeer(broken); p != "" || l {
		t.Fatalf("sentinel holder subject must synthesize no drilldown, got peer=%q lock=%v", p, l)
	}

	// E1 reciprocal (waiter subject): the correct waiter→holder direction is
	// UNCHANGED — holder wording + a named holder drilldown.
	e1 := types.TraceCausalProjectionNode{
		Subject:            ".ugc.aweme.lite-16547",
		BlockingKind:       "monitor_contention",
		BlockingPeer:       "#RxComputationT-16816",
		BlockingHolderSite: "AssetManager.list(AssetManager.java:1258)",
	}
	e1Name := runtimeTraceCausalProjectionBlockingName(e1, true)
	if e1Name != "锁竞争等待(持有者 #RxComputationT-16816)" {
		t.Fatalf("waiter-subject row must keep the lock-WAIT holder wording, got %q", e1Name)
	}
	p1, _, lock1 := runtimeTraceNextStepResolvedPeer(e1)
	if p1 != "#RxComputationT-16816" || !lock1 {
		t.Fatalf("waiter-subject node must name the true holder as drilldown, got peer=%q lock=%v", p1, lock1)
	}
	if shape, _ := runtimeTraceCausalProjectionImpactShapeCellTyped(e1, true); shape != "锁竞争·阻塞" {
		t.Fatalf("waiter-subject shape cell keeps the blocked wording, got %q", shape)
	}
}
