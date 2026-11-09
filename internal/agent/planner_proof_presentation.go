package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a reader projection of the current dispatch, not a new proof
// format or a grant. Legacy controller criteria stay durable and unchanged.
type plannerProofPresentation struct {
	batch *types.WriteWorkflowBatch
	grant *types.NativeTestRegistrationAuthorization
}

func plannerNativeRegistrationFor(ctx *types.AgentContext, run *types.WriteWorkflowRun) *types.NativeTestRegistrationAuthorization {
	if ctx == nil || ctx.Mutable == nil || run == nil || ctx.RepoRoot == "" {
		return nil
	}
	g := ctx.Mutable.NativeTestRegistrationAuthorization()
	if g == nil || g.RepositoryRoot != ctx.RepoRoot || g.RunID != run.RunID || g.BatchID != run.ActiveBatchID {
		return nil
	}
	return g
}

func currentPlannerProofPresentation(ctx *types.AgentContext, run *types.WriteWorkflowRun) *plannerProofPresentation {
	batch := plannerProofFollowupBatch(ctx, run)
	if batch == nil {
		return nil
	}
	return &plannerProofPresentation{batch: batch, grant: plannerNativeRegistrationFor(ctx, run)}
}

func (v *plannerProofPresentation) ownsCriterion(row string) bool {
	if v == nil {
		return false
	}
	for _, current := range v.batch.SuccessCriteria {
		if row == current {
			return true
		}
	}
	return false
}

// Only a controller-shaped behavior obligation for an exact contract in the
// private grant can be presented as eligible for either form. An unrecognized
// or probe-only row keeps its original wording. No task or answer prose is
// read, and this projection is never consumed by a validation gate.
func (v *plannerProofPresentation) criterion(row string) string {
	if v == nil || v.grant == nil {
		return row
	}
	fields := strings.Fields(row)
	known := map[string]string{}
	required := -1
	for i, f := range fields {
		key, val, ok := strings.Cut(f, "=")
		if !ok {
			return row
		}
		switch key {
		case "impact_obligation", "kind", "contract_ref", "evidence_ref", "verification_probe_required":
			if _, duplicate := known[key]; duplicate {
				return row
			}
			known[key] = val
			if f == "verification_probe_required=true" {
				required = i
			}
		}
	}
	if required < 0 || known["kind"] != "behavior_contract" || known["impact_obligation"] == "" {
		return row
	}
	id := known["contract_ref"]
	if id == "" {
		id = known["evidence_ref"]
	}
	for _, contract := range v.grant.Contracts {
		if id != "" && id == contract.ID {
			fields[required] = "fresh_verification_evidence_required=true"
			return strings.Join(fields, " ")
		}
	}
	return row
}
