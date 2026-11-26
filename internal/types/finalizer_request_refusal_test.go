package types

import (
	"strings"
	"sync"
	"testing"
)

func TestFinalizerRequestRefusalKindCountsWhatReachedTheModel(t *testing.T) {
	var none *FinalizerRequestRefusal
	if none.NothingSent() || (*MutableState)(nil).FinalizerRequestRefusal() != nil {
		t.Fatal("no record is not a refusal")
	}
	if _, same := none.InputsChangedSince(TypedInputSample{}); same {
		t.Fatal("no record has no inputs to equal")
	}
	(*MutableState)(nil).RecordFinalizeDispatchNotRefused()
	if (*MutableState)(nil).RecordFinalizeRequestRefusal(2, 1, 0, TypedInputSample{}) != nil {
		t.Fatal("a missing state records nothing")
	}

	bus := &BusContext{Mutable: NewMutableState("question")}
	inputs := sampleOf(bus)
	first := bus.Mutable.RecordFinalizeRequestRefusal(742912, 700000, 0, inputs)
	if first.Kind != FinalizerRefusalFirstRequest || !first.NothingSent() ||
		first.RequestBytes != 742912 || first.AllocationBytes != 700000 || first.RequestsSent != 0 || first.InputDigest != inputs.Digest() || first.InputDigest == "" {
		t.Fatalf("a refused first request with nothing sent: %+v", first)
	}
	// The record is written after the sample it carries and is not an input.
	if !inputs.Same(sampleOf(bus)) {
		t.Fatal("recording the refusal must not change the inputs it describes")
	}
	// The caller's copy is not the stored record, from either accessor.
	first.Kind = FinalizerRefusalLaterRequest
	if !bus.Mutable.FinalizerRequestRefusal().NothingSent() {
		t.Fatal("the stored record must not alias the returned one")
	}
	bus.Mutable.FinalizerRequestRefusal().Kind = FinalizerRefusalLaterRequest
	if !bus.Mutable.FinalizerRequestRefusal().NothingSent() {
		t.Fatal("the stored record must not alias the one handed to a reader")
	}
	// A second refusal, still with nothing sent in the run, is again a first
	// request.
	if again := bus.Mutable.RecordFinalizeRequestRefusal(742912, 700000, 0, inputs); !again.NothingSent() {
		t.Fatalf("nothing has reached the model yet: %+v", again)
	}

	// A refusal after requests were sent is a repair or follow-up request.
	if later := bus.Mutable.RecordFinalizeRequestRefusal(800000, 700000, 2, inputs); later.Kind != FinalizerRefusalLaterRequest || later.NothingSent() || later.RequestsSent != 2 {
		t.Fatalf("a refused later request: %+v", later)
	}
	// From then on the run has sent answer requests: a dispatch refused at
	// its own first request is still not "nothing sent".
	if after := bus.Mutable.RecordFinalizeRequestRefusal(742912, 700000, 0, inputs); after.Kind != FinalizerRefusalLaterRequest || after.NothingSent() {
		t.Fatalf("an earlier dispatch of the run sent requests: %+v", after)
	}

	// A dispatch that was not refused clears the record and counts as sent.
	fresh := &BusContext{Mutable: NewMutableState("question")}
	fresh.Mutable.RecordFinalizeRequestRefusal(742912, 700000, 0, sampleOf(fresh))
	fresh.Mutable.RecordFinalizeDispatchNotRefused()
	if fresh.Mutable.FinalizerRequestRefusal() != nil {
		t.Fatal("the record describes the run's last answer dispatch only")
	}
	if after := fresh.Mutable.RecordFinalizeRequestRefusal(742912, 700000, 0, sampleOf(fresh)); after.NothingSent() {
		t.Fatalf("a dispatch that was not refused may have sent a request: %+v", after)
	}
}

func TestFinalizerRequestRefusalComparesItsInputsByDigest(t *testing.T) {
	bus := &BusContext{Mutable: NewMutableState("question")}
	record := bus.Mutable.RecordFinalizeRequestRefusal(742912, 700000, 0, sampleOf(bus))
	if changed, same := record.InputsChangedSince(sampleOf(bus)); !same || len(changed) != 0 {
		t.Fatalf("an untouched state is the same inputs: %v", changed)
	}
	bus.ToolResults = append(bus.ToolResults, ToolResult{ToolName: "read_file", Summary: "late.go"})
	bus.Mutable.AppendDispatchToolResult(ToolResult{ToolName: "read_file", Summary: "late.go"})
	changed, same := record.InputsChangedSince(sampleOf(bus))
	named := strings.Join(changed, ",")
	if same || !strings.Contains(named, "bus.ToolResults") || !strings.Contains(named, "state.dispatchToolResults") {
		t.Fatalf("a drained read is new input and is named: same=%t changed=%v", same, changed)
	}

	// A state that cannot be sampled is never "the same inputs", on either
	// side of the comparison.
	opaque := &BusContext{Mutable: NewMutableState("question")}
	opaque.Mutable.repairExecutionPlan = &sync.Map{}
	unread := opaque.Mutable.RecordFinalizeRequestRefusal(742912, 700000, 0, sampleOf(opaque))
	if unread.InputDigest != "" {
		t.Fatalf("an unreadable state has no digest: %q", unread.InputDigest)
	}
	if _, same := unread.InputsChangedSince(sampleOf(opaque)); same {
		t.Fatal("an unsampled refusal must not suppress a rebuild")
	}
	// It names no carrier as changed either: nothing was read to compare.
	if changed, same := unread.InputsChangedSince(sampleOf(bus)); same || len(changed) != 0 {
		t.Fatalf("an unsampled refusal compared with a readable state: same=%t changed=%v", same, changed)
	}
	if _, same := record.InputsChangedSince(sampleOf(opaque)); same {
		t.Fatal("an unsampled later state must not suppress a rebuild")
	}
}
