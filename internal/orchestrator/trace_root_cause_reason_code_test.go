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

// §40.44 residual (a): the customer sidecar distinguishes "the model never
// submitted a valid selection" (valid_model_root_cause_selection_unavailable)
// from "the model submitted a selector and the binder rejected it, with no
// valid selection since" (model_root_cause_selection_rejected). The reason is
// chosen from typed state only: the rejected mark set by the selector commit
// tail, cleared whenever a report is stored or a valid submission is staged.
func TestTraceRootCauseUnavailableReasonSplitsRejectedFromNeverSelected(t *testing.T) {
	mut := types.NewMutableState("trace investigation")
	mut.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true})
	if got := traceRootCauseUnavailableReason(mut, nil); got != outputdump.RootCauseReasonValidSelectionUnavailable {
		t.Fatalf("never selected: %q", got)
	}
	mut.MarkTraceRootCauseSelectorRejected()
	if got := traceRootCauseUnavailableReason(mut, nil); got != outputdump.RootCauseReasonSelectionRejected {
		t.Fatalf("rejected submission must be reported as rejected: %q", got)
	}
	// A validly bound submission staged on a structurally rejected emit
	// (§40.31.1 ★16) supersedes the rejected mark.
	mut.SetPendingTraceRootCauseReport(&types.TraceRootCauseReportV2{SchemaVersion: 2})
	if got := traceRootCauseUnavailableReason(mut, nil); got != outputdump.RootCauseReasonValidSelectionUnavailable {
		t.Fatalf("a later valid staged submission clears the rejected arm: %q", got)
	}
	mut.MarkTraceRootCauseSelectorRejected()
	mut.SetTraceRootCauseReport(&types.TraceRootCauseReportV2{SchemaVersion: 2})
	if got := traceRootCauseUnavailableReason(mut, nil); got != outputdump.RootCauseReasonValidSelectionUnavailable {
		t.Fatalf("a stored report clears the rejected arm: %q", got)
	}
	if got := traceRootCauseUnavailableReason(mut, &types.TraceRootCauseReportV2{SchemaVersion: 2}); got != "" {
		t.Fatalf("a delivered report carries no unavailable reason: %q", got)
	}
	// The mark never outranks the coarser typed arms.
	mut.MarkTraceRootCauseSelectorRejected()
	mut.ResetAnswerDocumentV2()
	if got := traceRootCauseUnavailableReason(mut, nil); got != outputdump.RootCauseReasonContractNotActive {
		t.Fatalf("dispatch reset clears the mark and the contract arm wins: %q", got)
	}
	if got := traceRootCauseUnavailableReason(nil, nil); got != outputdump.RootCauseReasonRuntimeUnavailable {
		t.Fatalf("runtime arm: %q", got)
	}
}

func TestRecordTaskFinalizeWritesRejectedReasonCodeAfterRejectedSelector(t *testing.T) {
	mut := types.NewMutableState("trace investigation")
	mut.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true})
	mut.MarkTraceRootCauseSelectorRejected()
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
	if got.Status != "unavailable" || got.RootCauses == nil || len(got.RootCauses) != 0 || got.ReasonCode != "model_root_cause_selection_rejected" {
		t.Fatalf("wrong artifact after a rejected selector: %s", body)
	}
}
