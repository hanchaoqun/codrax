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
	want := "对持有者 NetworkKit_AssetsUtil_Operate_0-42067(持有点 AssetManager.list(AssetManager.java:1258))在重叠窗执行 trace_query view=thread_timeline/wakeup_chain"
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
	want := "对对端 OS_IPC_4939_437-43722 在重叠窗执行 trace_query view=thread_timeline/wakeup_chain"
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
	if strings.Contains(joined, "在重叠窗执行 trace_query view=thread_timeline/wakeup_chain") {
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
	want := "Run trace_query view=thread_timeline/wakeup_chain over the overlapping window for the lock holder HolderThread-42067 (holding site SomeManager.list(SomeManager.java:1258))"
	if !strings.Contains(strings.Join(texts, "\n"), want) {
		t.Fatalf("EN named holder row missing:\nwant %q\n got %q", want, texts)
	}
}
