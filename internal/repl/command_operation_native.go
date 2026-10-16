package repl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/types"
)

const commandNativeReaderStatus = "needs_native_reader"
const commandNativeReaderTeaching = "On the initial plan only, if native observation analysis is needed instead of command execution, emit status=needs_native_reader with input_paths naming exact regular files in the current request, steps=[], continue_after=false, risk_level=none and requires_confirmation=false. This delegates the unchanged request to the read-only analysis pipeline before any command executes; named inputs are navigation, not admitted evidence or prepared content. It is unavailable for replan, continuation, independent computer actions, source changes or approval-required work. Source explanations remain independent obligations."

// This conditional is shared with the public tool schema. The native-reader
// branch is validated before legacy flexible step decoding can drop any rows.
const commandNativeReaderShape = `{"if":{"properties":{"status":{"const":"needs_native_reader"}},"required":["status"]},"then":{"properties":{"input_paths":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string","minLength":1}},"steps":{"type":"array","maxItems":0},"continue_after":{"const":false},"requires_confirmation":{"const":false},"risk_level":{"const":"none"},"questions":{"type":"array","maxItems":0}},"required":["input_paths","steps"]}}`

type nativeCommandHandoff struct{ paths []string }

func (h *nativeCommandHandoff) Error() string {
	return "command planning delegated to read-only native analysis"
}
func (h *nativeCommandHandoff) NativeObservationInputs() []string {
	return append([]string(nil), h.paths...)
}

func commandNativeReaderPolicyEligible(p TurnPolicy) bool {
	if p.Route != RouteOperation || p.Operation != "investigate" || p.WriteIntent == WriteIntentExplicitChange ||
		p.NeedsDataAccess || p.DataTaskKind != "" || p.RequiresConfirmation ||
		p.RequiredOutcomes.Has(types.TurnOutcomeComputerAction|types.TurnOutcomeSourceChange) ||
		p.DeliveryOwner != "" && p.DeliveryOwner != analysisPipelineDeliveryOwner {
		return false
	}
	if p.OperationKind != "" && p.OperationKind != "computer_operation" {
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

// NativeOperationHandoffPolicy accepts only the private, validated initial-plan
// carrier. Neither a status string nor a source filename grants this transition.
func NativeOperationHandoffPolicy(policy TurnPolicy, err error) (TurnPolicy, bool) {
	var handoff *nativeCommandHandoff
	if !commandNativeReaderPolicyEligible(policy) || !errors.As(err, &handoff) || handoff == nil || len(handoff.paths) == 0 {
		return policy, false
	}
	policy.Route, policy.Operation, policy.OperationKind = RouteRepo, "investigate", ""
	policy.NeedsRepoAccess, policy.NeedsOperationAccess = true, false
	policy.SideEffects, policy.TargetSurface, policy.RiskLevel = nil, "", "none"
	policy.WriteIntent = WriteIntentAnalysisOnly
	if policy.RequiredOutcomes.Has(types.TurnOutcomeExternalArtifact) {
		policy.DeliveryOwner = analysisPipelineDeliveryOwner
	}
	return policy, true
}

func validateCommandNativeReaderDraft(d commandPlanDraft, req commandOperationPlannerRequest) error {
	if string(d.Status) != commandNativeReaderStatus {
		return nil
	}
	if req.Replan || req.Continuation || len(req.Records) != 0 || req.PreviousPlan.ID != "" || len(req.PreviousPlan.Steps) != 0 || req.Result.PlanID != "" || req.Result.Status != "" || len(req.Result.StepResults) != 0 ||
		!commandNativeReaderPolicyEligible(req.Policy) || len(d.InputPaths) == 0 || len(d.Steps) != 0 || bool(d.ContinueAfter) || bool(d.RequiresConfirmation) {
		return &replStructuredToolParamError{ToolName: commandOperationPlanTool.Name, Scope: "initial native reader navigation", Err: errors.New("needs_native_reader conflicts with executed/continuing work or an independent operation obligation"), Hint: "Keep replan/continuation and independent operations in their current workflow; native reader delegation is only a command-free initial analysis decision."}
	}
	return nil
}

func initialCommandNativeHandoff(ctx context.Context, req commandOperationPlannerRequest, d commandPlanDraft) error {
	if string(d.Status) != commandNativeReaderStatus {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := validateCommandNativeReaderDraft(d, req); err != nil {
		return err
	}
	available := map[string]bool{}
	for _, p := range outputdump.NamedInputPathsFromRequest(req.UserLine, req.RepoRoot, 8) {
		available[p] = true
	}
	paths := make([]string, 0, len(d.InputPaths))
	for _, p := range d.InputPaths {
		abs := strings.TrimSpace(p)
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(req.RepoRoot, abs)
		}
		abs, err := filepath.Abs(abs)
		if err != nil || !available[abs] {
			return fmt.Errorf("native reader input %q is not an exact current named regular file", p)
		}
		paths = append(paths, abs)
	}
	return &nativeCommandHandoff{paths: paths}
}
