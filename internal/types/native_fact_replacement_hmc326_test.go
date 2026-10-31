package types

import "testing"

// This isolates the admission-commit race after producer validation, not a
// synthetic authority path. Real producer and ledger behavior are exercised
// by the public HMC326 lifecycle cases in internal/agent.
func TestHMC326NativeAdmissionRechecksReplacementToken(t *testing.T) {
	for _, omitted := range []bool{false, true} {
		m := NewMutableState("replacement during native admission")
		generation, token := m.traceSourceReadGeneration, m.turnAReplacementTokenRef()
		fact := &retainedNativeFact{key: "storage-only", encoded: "complete", queries: 1, generation: generation}
		if omitted {
			// Storage-only capacity fixture, not a producer or a ledger fact.
			generation.nativeFacts.admit(&retainedNativeFact{key: "existing-capacity", encoded: "existing", queries: nativeFactRetentionQueryLimit, generation: generation})
		}
		quota := func() [3]int {
			generation.nativeFacts.mu.Lock()
			defer generation.nativeFacts.mu.Unlock()
			return [3]int{generation.nativeFacts.queries, generation.nativeFacts.bytes, len(generation.nativeFacts.entries)}
		}
		beforeQuota := quota()
		ready, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() {
			close(ready)
			<-finish
			m.completeNativeFactAdmission(generation, token, fact, "old-source", "old-query")
			close(done)
		}()
		<-ready
		m.SetTurnAArtifacts(TurnAArtifacts{})
		close(finish)
		<-done
		if got := m.NativeFactRetentionStatus(); got.RetainedQueries != 0 || got.CapacityOmitted {
			t.Fatal("late admission or omission crossed the explicit replacement boundary")
		}
		if afterQuota := quota(); afterQuota != beforeQuota {
			t.Fatalf("expired token consumed shared capacity: before=%v after=%v", beforeQuota, afterQuota)
		}
		m.completeNativeFactAdmission(generation, m.turnAReplacementTokenRef(), fact, "current-source", "current-query")
		got := m.NativeFactRetentionStatus()
		if omitted && !got.CapacityOmitted || !omitted && got.RetainedQueries != 1 {
			t.Fatal("current collection admission was blocked by an earlier replacement")
		}
		wantQuota := beforeQuota
		if !omitted {
			wantQuota = [3]int{1, len(fact.encoded), 1}
		}
		if afterQuota := quota(); afterQuota != wantQuota {
			t.Fatalf("current token changed capacity incorrectly: got=%v want=%v", afterQuota, wantQuota)
		}
	}
}

func TestHMC326ZeroValueMutableReplacementToken(t *testing.T) {
	var parent MutableState
	child := parent.ForkForExploreDispatch()
	if parent.turnAReplacementToken == nil || child.exploreForkTurnAReplacementBase != parent.turnAReplacementToken {
		t.Fatal("zero-value mutable state did not establish a private fork baseline")
	}
	child.SetTurnAArtifacts(TurnAArtifacts{InvestigationNotes: []string{"old child"}})
	parent.SetTurnAArtifacts(TurnAArtifacts{UserQuestion: "replacement"})
	parent.MergeExploreFork(child)
	parent.MergeExploreForkPublishedTools(child)
	if got := parent.TurnAArtifacts(); got == nil || got.UserQuestion != "replacement" || len(got.InvestigationNotes) != 0 {
		t.Fatal("zero-value parent accepted an old fork after replacement")
	}
}

// Pause after the public entry's token capture, then resume the exact common
// append implementation. This boundary unit test covers raw history/registry
// publication as well as retention, without inventing a producer receipt.
func TestHMC326DispatchAppendRechecksWholeCollection(t *testing.T) {
	for _, reset := range []bool{false, true} {
		m := NewMutableState("whole append boundary")
		token := m.turnAReplacementTokenRef()
		ready, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		result := ToolResult{ToolName: "trace_query", Success: false, Summary: "failed attempt stays failed"}
		go func() {
			close(ready)
			<-finish
			m.appendDispatchToolResultInCollection(result, token)
			close(done)
		}()
		<-ready
		if reset {
			m.ResetTurnAArtifacts()
		} else {
			m.SetTurnAArtifacts(TurnAArtifacts{})
		}
		m.ResetDispatchToolResults()
		close(finish)
		<-done
		if len(m.DispatchToolResults()) != 0 || len(m.traceQueryPublishedBlobRefs) != 0 || len(m.traceQuerySourceReads) != 0 ||
			len(m.traceBusinessSpanRefs) != 0 || m.NativeFactRetentionStatus().RetainedQueries != 0 {
			t.Fatal("old append crossed replacement through raw history or a publication registry")
		}
		m.AppendDispatchToolResult(result)
		if got := m.DispatchToolResults(); len(got) != 1 || got[0].Success || got[0].Summary != result.Summary {
			t.Fatal("current public append dropped or relabeled a failed attempt")
		}
	}
}

func TestHMC326TaskResetRejectsOldForkStateButWindowResetKeepsIt(t *testing.T) {
	for _, taskReset := range []bool{false, true} {
		parent := NewMutableState("reset boundary")
		child := parent.ForkForExploreDispatch()
		child.AppendDispatchToolResult(ToolResult{ToolName: "read_file", Success: true, Summary: "old dispatch"})
		child.SetTurnAArtifacts(TurnAArtifacts{InvestigationNotes: []string{"old note"}})
		child.SetInvestigationComplete("accepted child decision")
		if taskReset {
			parent.ResetTurnAArtifacts()
		} else {
			parent.ResetInvestigationComplete()
		}
		parent.MergeExploreFork(child)
		parent.MergeExploreForkPublishedTools(child)
		if taskReset {
			if parent.TurnAArtifacts() != nil || parent.IsInvestigationComplete() || len(parent.DispatchToolResults()) != 0 {
				t.Fatal("old fork revived raw tools, notes or completion after a task reset")
			}
		} else if !parent.IsInvestigationComplete() || parent.StableInvestigationCompleteReason() != "accepted child decision" {
			t.Fatal("a window reset incorrectly revoked a current accepted fork decision")
		}
	}
}
