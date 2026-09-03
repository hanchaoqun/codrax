package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_finding_teaching_retirement_test.go — V1-5 (§40.16) teaching face +
// V2-3 (§40.19 ①) single-source teaching.
//
// Red at HEAD: CompileCandidateContract minted Required=true and
// renderTraceFindingContract rendered the "Trace Finding Contract (Required
// Typed Sidecar)" section for it — a lane no schema published. The compiled
// contract now never teaches `trace_finding`, and the selector's failure
// contract (dropped + reason on the tool result + fix in the next patch) is
// worded ONCE (types.TraceRootCauseSelectorOutcomeTeaching) on both the
// schema property description and the selector context.
func TestCompiledTraceContractNeverTeachesTheRetiredFindingLane(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	contract, err := tracefinding.CompileCandidateContract(types.ObservationLedger{Records: []types.ObservationRecord{{ID: "E1"}}}, types.TraceCausalProjectionSet{
		Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "E1", Subject: "worker", Rank: 1, TypeToken: "scheduler_latency", ChainRelevance: "on_chain", ImpactMS: 8,
		}}}}}, tracefinding.SeatFrameCausalityAuthority{Applicable: true, Index: tracefinding.SeatFrameCausalityIndex{"E1": true}})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = len(tracefinding.SelectableRootCauseCandidates(contract)) > 0
	if !contract.RootCauseReportEnabled {
		t.Fatalf("fixture must offer a selectable roster: %+v", contract.Candidates)
	}
	ctx.Mutable.SetTraceFindingContract(contract)
	text := renderTraceFindingContract(ctx)
	for _, retired := range []string{"Trace Finding Contract", "`trace_finding`", "Required Typed Sidecar"} {
		if strings.Contains(text, retired) {
			t.Fatalf("the retired trace_finding lane must not be taught (%q):\n%s", retired, text)
		}
	}
	if !strings.Contains(text, "Optional Trace Root Cause JSON") {
		t.Fatalf("the live selector teaching must remain:\n%s", text)
	}
	teaching := types.TraceRootCauseSelectorOutcomeTeaching()
	if !strings.Contains(text, teaching) {
		t.Fatalf("selector context must carry the single-source outcome teaching verbatim:\n%s", text)
	}
	for _, internal := range []string{"OptionalCarrierOutcome", "failEmit", "logging", "retry"} {
		if strings.Contains(teaching, internal) {
			t.Fatalf("model-facing teaching leaks an internal name %q: %q", internal, teaching)
		}
	}
	// Schema face: the same sentence, byte-identical.
	agentCtx := &types.AgentContext{Mutable: types.NewMutableState("trace")}
	agentCtx.Mutable.SetTraceFindingContract(contract)
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&tool.EmitAnswerDocument{}).ParametersFor(agentCtx), &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema.Properties["trace_root_causes"].Description; got != teaching {
		t.Fatalf("schema description drifted from the single source:\n%q\n%q", got, teaching)
	}
	if _, retired := schema.Properties["trace_finding"]; retired {
		t.Fatal("the retired trace_finding property must never be published")
	}
}
