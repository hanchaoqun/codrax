package orchestrator

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC §377 D1: a follow-up that offered the root-cause selection but was never
// sent is reported as such, not as a model that never selected. The reason
// reads the typed follow-up receipt only. A rejected submission keeps its own
// reason, and a delivered follow-up, a follow-up without the selection or a
// reset dispatch leaves the never-selected arm unchanged.
func TestHMC377UndeliveredSelectionFollowUpHasItsOwnReason(t *testing.T) {
	mut := types.NewMutableState("trace investigation")
	mut.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true})
	notSent := &types.AnswerFollowUpDelivery{RootCauseSelectionOffered: true, NotSentCause: types.AnswerFollowUpNotSentRequestAllocation, RequestBytes: 707552, AllocationBytes: 700000}
	for _, c := range []struct {
		name    string
		receipt *types.AnswerFollowUpDelivery
		want    string
	}{
		{"no follow-up", nil, outputdump.RootCauseReasonValidSelectionUnavailable},
		{"sent and declined", &types.AnswerFollowUpDelivery{RootCauseSelectionOffered: true, Sent: true}, outputdump.RootCauseReasonValidSelectionUnavailable},
		{"not sent without the selection", &types.AnswerFollowUpDelivery{NotSentCause: types.AnswerFollowUpNotSentContextPressure}, outputdump.RootCauseReasonValidSelectionUnavailable},
		{"not sent with the selection", notSent, outputdump.RootCauseReasonSelectionFollowUpNotSent},
	} {
		mut.SetAnswerFollowUpDelivery(c.receipt)
		if got := traceRootCauseUnavailableReason(mut, nil); got != c.want {
			t.Fatalf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	mut.MarkTraceRootCauseSelectorRejected()
	if got := traceRootCauseUnavailableReason(mut, nil); got != outputdump.RootCauseReasonSelectionRejected {
		t.Fatalf("a rejected submission keeps its reason: %q", got)
	}
	if got := traceRootCauseUnavailableReason(mut, &types.TraceRootCauseReportV2{SchemaVersion: 2}); got != "" {
		t.Fatalf("a delivered report carries no unavailable reason: %q", got)
	}
	mut.SetTraceRootCauseReport(nil)
	mut.SetAnswerFollowUpDelivery(notSent)
	mut.ResetActiveAnswerDocumentV2ForFinalizeDispatch()
	if mut.AnswerFollowUpDelivery() != nil || traceRootCauseUnavailableReason(mut, nil) != outputdump.RootCauseReasonValidSelectionUnavailable {
		t.Fatal("a new finalizer dispatch must not inherit the previous follow-up receipt")
	}
}

func TestHMC377RecordTaskFinalizeWritesTheUndeliveredFollowUpReason(t *testing.T) {
	mut := types.NewMutableState("trace investigation")
	mut.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true})
	mut.SetAnswerFollowUpDelivery(&types.AnswerFollowUpDelivery{RootCauseSelectionOffered: true, NotSentCause: types.AnswerFollowUpNotSentRequestAllocation})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}, outputDumpDir: t.TempDir(), outputDumpMax: 10, emit: func(render.Event) {}}
	o.recordTaskFinalize(&agent.StageOutput{FinalAnswer: "original model answer"})
	body, err := os.ReadFile(mut.FinalAnswerRootCauseJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	var got outputdump.DefaultRootCauseArtifact
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unavailable" || got.RootCauses == nil || len(got.RootCauses) != 0 || got.ReasonCode != "root_cause_selection_follow_up_not_sent" {
		t.Fatalf("wrong artifact after an undelivered follow-up: %s", body)
	}
}
