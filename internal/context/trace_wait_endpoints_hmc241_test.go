package context

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc241WakeupRecord(path string, wakerTGID, wakeeTGID int) types.ObservationRecord {
	f := types.RuntimeWakeupEvent{IndexPath: path, From: "from", To: "to", Waker: types.RuntimeWakeupThread{Comm: "worker", PID: 11, TGID: wakerTGID}, Wakee: types.RuntimeWakeupThread{Comm: "target", PID: 42, TGID: wakeeTGID}, Timestamp: 1.003, Line: 4}
	b, _ := json.Marshal(f)
	return types.ObservationRecord{ID: path, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceObservedDirectCause,
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: path, QueryScopeID: "query", PayloadRef: "payload", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1.02}, Span: types.ObservationSpan{LineStart: 4, LineEnd: 4, StartTs: 1.003, EndTs: 1.003},
		Subject: "worker-11", Object: "target-42", Predicate: "wakeup_chain_edge", ClaimKey: "wakeup_chain_edge:worker-11->target-42", Value: "3", Unit: "ms", SupportRefs: []string{path + ":4"},
		RichNotes: []string{"wakeup_ts=1.003", "latency=3", types.TraceNoteKeyWakeupEventInstance + "=" + string(b)}}
}

func TestHMC241WakeupContextExactSourceAndValidatedCarrier(t *testing.T) {
	a, b := hmc241WakeupRecord("/a.trace", 10, 20), hmc241WakeupRecord("/b.trace", 30, 40)
	bad := hmc241WakeupRecord("/bad.trace", 777, 888)
	bad.SourceRef.Path = "/different.trace"
	for _, records := range [][]types.ObservationRecord{{a, b, a, bad}, {bad, b, a, b}} {
		before, _ := json.Marshal(records)
		text := formatTraceWaitWakeEvidenceFromLedger(types.ObservationLedger{Records: records}, nil)
		for _, want := range []string{"发起线程所属进程 TGID=10；被唤醒线程所属进程 TGID=20", "发起线程所属进程 TGID=30；被唤醒线程所属进程 TGID=40"} {
			if strings.Count(text, want) != 1 {
				t.Fatalf("source identities collapsed or republication duplicated %q:\n%s", want, text)
			}
		}
		if strings.Contains(text, "TGID=777") || strings.Contains(text, "TGID=888") {
			t.Fatalf("unverified carrier supplied process identity:\n%s", text)
		}
		after, _ := json.Marshal(records)
		if string(before) != string(after) {
			t.Fatal("context rendering changed source ledger")
		}
	}
}
