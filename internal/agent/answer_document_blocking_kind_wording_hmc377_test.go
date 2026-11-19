package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 D2 — cross-check appendix and finalizer handoff faces of a blocking
// row. Batch 376 case B printed 「关键阻塞：urs_CalculatePu-11876 ->
// ease.cloudmusic-11077 … 观测类型：持锁阻塞」 for a fellow waiter of the main
// thread's GC completion wait (sealed 0212 L838–L843) and handed the model
// permitted_reader_cause_label="持锁阻塞" for the same rows.

func hmc377FellowWaiterRecord() types.ObservationRecord {
	return types.ObservationRecord{
		ID: "trace_query:x#critical_blocking:5", ClaimKey: "critical_blocking:blocking_span", Predicate: "critical_blocking",
		Subject: "urs_CalculatePu-11876", Object: "ease.cloudmusic-11077", Value: "123.462", Unit: "ms",
		RichNotes: []string{
			"type=blocking_span", "peer=ease.cloudmusic-11077", "peer_source=wakeup_edge",
			"wait_object=GC: Wait For Completion Alloc", "blocking_value_basis=wait_segments",
			types.TraceNoteKeyBlockingWakeRelay + "=true", types.TraceNoteKeyBlockingWakeRelayUpstream + "=HeapTaskDaemon-11098",
			"chain_relevance=background", "peer_state_dominant=s_sleep", "peer_state_sleep=123.003",
		},
	}
}

func TestHMC377AppendixFellowWaiterReadsAsPassedOnWait(t *testing.T) {
	got := traceQueryObservationSupplementText(hmc377FellowWaiterRecord(), true)
	for _, want := range []string{
		"背景支撑观测：urs_CalculatePu-11876 -> ease.cloudmusic-11077",
		"唤醒转发：结束等待的唤醒方同在这项等待中，先被 HeapTaskDaemon-11098 唤醒后才唤醒本线程，不是本次等待所依赖的一方",
		"观测类型：阻塞等待",
		"结束等待的唤醒方：ease.cloudmusic-11077",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("appendix line lacks %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"持锁", "关键阻塞", "锁竞争"} {
		if strings.Contains(got, banned) {
			t.Fatalf("appendix line of a recorded wait without lock evidence says %q:\n%s", banned, got)
		}
	}
	en := traceQueryObservationSupplementText(hmc377FellowWaiterRecord(), false)
	for _, want := range []string{"background supporting observation", "observation type: blocking wait", "thread whose wakeup ended the wait: ease.cloudmusic-11077", "wakeup passed on"} {
		if !strings.Contains(en, want) {
			t.Fatalf("EN appendix line lacks %q:\n%s", want, en)
		}
	}
}

// The waiting main thread's own row keeps its family label on the chain and
// names the collector as the thread whose wakeup ended the wait.
func TestHMC377AppendixOnChainWaitNamesTheWaker(t *testing.T) {
	record := types.ObservationRecord{
		ClaimKey: "critical_blocking:blocking_span", Predicate: "critical_blocking",
		Subject: "ease.cloudmusic-11077", Object: "HeapTaskDaemon-11098", Value: "123.003", Unit: "ms",
		RichNotes: []string{"type=blocking_span", "peer=HeapTaskDaemon-11098", "peer_source=wakeup_edge",
			"blocking_value_basis=wait_segments", "chain_relevance=on_chain", "peer_state_dominant=running"},
	}
	got := traceQueryObservationSupplementText(record, true)
	if !strings.Contains(got, "关键阻塞：ease.cloudmusic-11077 -> HeapTaskDaemon-11098") ||
		!strings.Contains(got, "观测类型：阻塞等待") || !strings.Contains(got, "结束等待的唤醒方：HeapTaskDaemon-11098") ||
		strings.Contains(got, "持锁") {
		t.Fatalf("on-chain recorded wait line drifted:\n%s", got)
	}
}

// Real lock evidence keeps its words: the holder-subject rank record of a
// monitor contention reads 持锁阻塞; the waiter-subject critical_blocking row
// of the same lock reads 锁竞争等待 (a waiter never holds the lock).
func TestHMC377AppendixRealLockKeepsLockWords(t *testing.T) {
	holder := types.ObservationRecord{
		ClaimKey: "root_cause_primary:blocking_span", Predicate: "root_cause_primary",
		Subject: "#LogReaper-11293", Object: "blocking_span", Value: "50.184", Unit: "ms",
		RichNotes: []string{"type=blocking_span", "blocking_kind=monitor_contention", "subject_is_lock_holder=true",
			"peer=.ugc.aweme.lite-11105", "chain_relevance=on_chain", "rank=1"},
	}
	if got := traceQueryObservationSupplementText(holder, true); !strings.Contains(got, "#LogReaper-11293 -> 持锁阻塞") {
		t.Fatalf("holder rank record lost its lock word:\n%s", got)
	}
	waiter := types.ObservationRecord{
		ClaimKey: "critical_blocking:blocking_span", Predicate: "critical_blocking",
		Subject: ".ugc.aweme.lite-11105", Object: "#LogReaper-11293", Value: "50.184", Unit: "ms",
		RichNotes: []string{"type=blocking_span", "peer=#LogReaper-11293", "blocking_kind=monitor_contention",
			"holder_source=wakeup_edge", "chain_relevance=on_chain"},
	}
	got := traceQueryObservationSupplementText(waiter, true)
	if !strings.Contains(got, "阻塞类型：监视器竞争") || !strings.Contains(got, "观测类型：锁竞争等待") ||
		!strings.Contains(got, "关联线程：#LogReaper-11293") || strings.Contains(got, "持锁阻塞") {
		t.Fatalf("waiter-subject lock row line drifted:\n%s", got)
	}
}

// The model-facing permitted cause word follows the row's typed kind: a
// recorded wait never hands the model a lock word, a real holder row does.
func TestHMC377HandoffPermittedWordFollowsBlockingKind(t *testing.T) {
	recorded := types.TraceCausalProjectionNode{EvidenceID: "cb-5", Subject: "urs_CalculatePu-11878", Predicate: "critical_blocking",
		Object: "unknown-thread", TypeToken: "blocking_span", ChainRelevance: "background", BlockingValueBasis: "wait_segments", ImpactMS: 123.745}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{BackgroundCauses: []types.TraceCausalProjectionNode{recorded}}}}
	got := renderTraceFinalReaderFacingLanguageHandoff(set, nil, "zh")
	if !strings.Contains(got, `permitted_reader_cause_label="阻塞等待"`) || strings.Contains(got, "持锁") {
		t.Fatalf("recorded wait handoff word drifted:\n%s", got)
	}
	holder := types.TraceCausalProjectionNode{EvidenceID: "rk-1", Subject: "#LogReaper-11293", Predicate: "root_cause_primary",
		Object: "blocking_span", TypeToken: "blocking_span", BlockingKind: "monitor_contention", BlockingSubjectIsHolder: true,
		Rank: 1, ChainRelevance: "on_chain", ImpactMS: 50.184}
	set.Projections[0].PrimaryRootCauses = []types.TraceCausalProjectionNode{holder}
	got = renderTraceFinalReaderFacingLanguageHandoff(set, nil, "zh")
	if !strings.Contains(got, `permitted_reader_cause_label="持锁阻塞"`) || !strings.Contains(got, `permitted_reader_cause_label="阻塞等待"`) {
		t.Fatalf("each blocking kind must hand over its own word once:\n%s", got)
	}
	if strings.Count(got, `permitted_reader_cause_label="持锁阻塞"`) != 1 {
		t.Fatalf("a blocking word was handed over twice:\n%s", got)
	}
}
