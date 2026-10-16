package repl

import "github.com/hanchaoqun/codrax/internal/types"

// Delivery ownership is routing metadata, never filesystem permission or a
// receipt. Native producers still validate their own accepted export requests.
const analysisPipelineDeliveryOwner = "analysis_pipeline"
const independentOperationDeliveryOwner = "independent_operation"

const turnDeliveryOwnershipContract = "delivery_owner distinguishes who produces a requested external_artifact. analysis_pipeline means the analysis pipeline's own evidence package or final report, derived from its admitted observations; keep operation=investigate and route=repo/hybrid, with operation_kind empty. This delivery is not an independent computer action and does not need operation side_effects or a target_surface. independent_operation means a separately requested file/computer/provider workflow, such as copying or editing files, creating office artifacts, or submitting externally; keep its operation and approval requirements. Leave delivery_owner empty when no external artifact is requested. Required source explanations remain independent; neither output location nor delivery ownership makes current source required or optional. This classification grants no export, write, execution, or completion authority."

func analysisOwnsRequestedDelivery(p TurnPolicy) bool {
	if p.DeliveryOwner != analysisPipelineDeliveryOwner || p.Operation != "investigate" || p.OperationKind != "" ||
		p.WriteIntent == WriteIntentExplicitChange || p.NeedsDataAccess || p.DataTaskKind != "" || p.RequiresConfirmation ||
		!p.RequiredOutcomes.Has(types.TurnOutcomeExternalArtifact) ||
		!p.RequiredOutcomes.Has(types.TurnOutcomeAnswer|types.TurnOutcomeMeasurement|types.TurnOutcomeSourceExplanation) ||
		p.RequiredOutcomes.Has(types.TurnOutcomeComputerAction|types.TurnOutcomeSourceChange) {
		return false
	}
	switch p.Route {
	case RouteRepo, RouteHybrid, RouteOperation:
	default:
		return false
	}
	switch p.Source {
	case "repo", "mixed", "artifact", "external_tool":
	default:
		return false
	}
	switch p.RiskLevel {
	case "", "none", "low":
	default:
		return false
	}
	switch p.TargetSurface {
	case "", "unknown", "file_artifact":
	default:
		return false
	}
	for _, effect := range p.SideEffects {
		if effect != "local_file_write" {
			return false
		}
	}
	return true
}

func reconcileAnalysisDelivery(p TurnPolicy) TurnPolicy {
	if !analysisOwnsRequestedDelivery(p) {
		return p
	}
	if p.Route == RouteOperation {
		p.Route = RouteRepo
	}
	p.NeedsRepoAccess, p.NeedsOperationAccess = true, false
	p.SideEffects, p.TargetSurface, p.RiskLevel = nil, "", "none"
	return p
}
