package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeDiagramSupportResolverEmptyDoesNotBypassProvider(t *testing.T) {
	// A legitimate wakeup exercises both the standalone fallback and the
	// important distinction between no resolver and an authoritative no-match.
	f := RuntimeWakeupEvent{IndexPath: "/trace", From: "a", To: "b", Waker: RuntimeWakeupThread{PID: 1}, Wakee: RuntimeWakeupThread{PID: 2}, Timestamp: 1.5, Line: 2}
	data, _ := json.Marshal(f)
	r := ObservationRecord{Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard, ProvenanceLane: ObservationProvenanceObservedDirectCause,
		Predicate: "wakeup_chain_edge", Subject: "pid=1", Object: "pid=2", ClaimKey: "wakeup_chain_edge:pid=1->pid=2", Unit: "ms",
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/trace", QueryScopeID: "q", PayloadRef: "p", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 2},
		Span:      ObservationSpan{LineStart: 2, LineEnd: 2, StartTs: 1.5, EndTs: 1.5}, SupportRefs: []string{"/trace:2"}, RichNotes: []string{TraceNoteKeyWakeupEventInstance + "=" + string(data)}}
	ledger := ObservationLedger{Records: []ObservationRecord{r}}
	if len(RuntimeWakeupDiagramEvents(ledger, nil)) != 1 {
		t.Fatal("fallback fixture is not a validated physical wakeup")
	}
	runtimeDiagramSupportMu.RLock()
	previous := runtimeDiagramSupportResolver
	runtimeDiagramSupportMu.RUnlock()
	t.Cleanup(func() {
		runtimeDiagramSupportMu.Lock()
		runtimeDiagramSupportResolver = previous
		runtimeDiagramSupportMu.Unlock()
	})
	// External-package tests may import tool and register at process startup.
	// This non-parallel test isolates private state; production has no reset API.
	runtimeDiagramSupportMu.Lock()
	runtimeDiagramSupportResolver = nil
	runtimeDiagramSupportMu.Unlock()
	if got := runtimeSupportedDiagramKinds(ledger, nil); !reflect.DeepEqual(got, []DiagramKind{DiagramSequence, DiagramFlow, DiagramCallDAG}) {
		t.Fatalf("types-only wakeup fallback changed: %v", got)
	}
	called := false
	request := &RequestModel{Language: "zh"}
	RegisterRuntimeDiagramSupportResolver(func(got ObservationLedger, rm *RequestModel) []DiagramKind {
		called = true
		if !reflect.DeepEqual(got, ledger) || rm != request {
			t.Fatal("resolver lost original ledger or request scope")
		}
		return nil
	})
	if got := runtimeSupportedDiagramKinds(ledger, request); len(got) != 0 || !called {
		t.Fatalf("empty canonical provider bypassed through wakeup fallback: %v", got)
	}
	for _, resolver := range []RuntimeDiagramSupportResolver{nil, func(ObservationLedger, *RequestModel) []DiagramKind { return []DiagramKind{DiagramSequence} }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("nil/replacement registration did not reject")
				}
			}()
			RegisterRuntimeDiagramSupportResolver(resolver)
		}()
	}
	if got := runtimeSupportedDiagramKinds(ledger, request); len(got) != 0 {
		t.Fatal("rejected replacement mutated canonical provider")
	}
}
