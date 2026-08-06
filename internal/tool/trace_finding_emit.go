package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// resolveTraceFindingForEmit validates an optional TraceFindingV1 against the
// active TraceFindingContract and compiled candidate set. Shadow-optional mode
// may omit the finding; Required mode must include a valid one.
func resolveTraceFindingForEmit(ctx *types.BusContext, finding *types.TraceFindingV1) (*types.TraceFindingV1, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if !contract.Active() {
		if finding != nil {
			logging.Warning("[emit_answer_document] ignoring trace_finding because TraceFindingContract is inactive")
		}
		return nil, nil
	}
	if finding == nil {
		if contract.Required {
			return nil, fmt.Errorf("finding_missing: TraceFindingContract requires trace_finding")
		}
		return nil, nil
	}
	candidates, err := tracefinding.CompileTraceDecisionCandidateSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("finding_candidate_violation: compile candidates: %w", err)
	}
	if finding.SchemaVersion == 0 {
		finding.SchemaVersion = types.TraceFindingSchemaVersion
	}
	if strings.TrimSpace(finding.FindingID) == "" {
		finding.FindingID = "finding:" + candidates.CandidateSetID
	}
	if finding.Artifact.Label == "" {
		finding.Artifact = candidates.Artifact
	}
	if finding.Scope.WindowStart == 0 && finding.Scope.WindowEnd == 0 {
		finding.Scope = candidates.Scope
	}
	finding.Coverage.RosterComplete = candidates.RosterComplete
	if err := tracefinding.ValidateTraceFinding(finding, candidates); err != nil {
		return nil, err
	}
	return finding, nil
}
