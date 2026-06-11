package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// EmitWriteWorkflowDecision is the write_controller agent's structured exit
// channel. It stores a schema-normalized WriteWorkflowDecision payload for the
// outer write DAG; hard routing consumers must unmarshal this JSON and switch
// on the typed action enum.
type EmitWriteWorkflowDecision struct {
	ReadOnly
	NonEvidenceTool
}

func (t *EmitWriteWorkflowDecision) Name() string { return "emit_write_workflow_decision" }

func (t *EmitWriteWorkflowDecision) Description() string {
	return "Emits one typed write workflow controller decision. The system validates action/payload consistency and stores the normalized JSON; prose does not affect routing."
}

func (t *EmitWriteWorkflowDecision) Parameters() json.RawMessage {
	return writeflow.WriteWorkflowDecisionSchema()
}

// ParametersFor projects the decision schema per dispatch: the action enum
// is restricted to the typed mode's allowed set (ModePlan drops apply_plan /
// verify_batch) so masked actions are structurally invisible to the model
// instead of failing after emission. Mirrors EmitAnswerDocument.ParametersFor.
func (t *EmitWriteWorkflowDecision) ParametersFor(ctx *types.AgentContext) json.RawMessage {
	if ctx == nil {
		return t.Parameters()
	}
	return writeflow.WriteWorkflowDecisionSchemaForActions(writeflow.WorkflowActionsForMode(ctx.Mode))
}

func (t *EmitWriteWorkflowDecision) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_write_workflow_decision requires a writable context",
			Timestamp: now,
		}, nil
	}

	params = applyStructuredPayloadCompat(t.Name(), params, t.Parameters())
	var decision writeflow.WriteWorkflowDecision
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decision); err != nil {
		return failStrictDecodeWithError(t.Name(), now, err, nil, params)
	}
	decision = writeflow.NormalizeWriteWorkflowDecision(decision)
	if errs := writeflow.ValidateWriteWorkflowDecision(decision); len(errs) > 0 {
		return errResult(t.Name(), "emit_write_workflow_decision rejected: "+strings.Join(errs, "; ")), nil
	}
	// Runtime mode mask (defense in depth behind the per-dispatch schema
	// projection): a masked action is rejected with a typed repair that
	// lists the mode's allowed set, never routed onward.
	if !writeflow.WorkflowActionAllowedInMode(decision.Action, ctx.Mode) {
		allowed := writeflow.WorkflowActionsForMode(ctx.Mode)
		names := make([]string, 0, len(allowed))
		for _, a := range allowed {
			names = append(names, string(a))
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("emit_write_workflow_decision rejected: action=%s is not available in the current mode", decision.Action),
			Timestamp: now,
			Repair: &types.ToolRepair{
				Code: "workflow_action_not_in_mode",
				Hint: "Choose one of the actions offered by the tool schema for this mode: " + strings.Join(names, ", ") + ".",
				Metadata: map[string]string{
					"action": string(decision.Action),
					"mode":   string(ctx.Mode),
				},
			},
		}, nil
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		return errResult(t.Name(), "emit_write_workflow_decision rejected: marshal normalized decision: "+err.Error()), nil
	}
	ctx.Mutable.SetWriteWorkflowDecisionJSON(raw)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   fmt.Sprintf("write workflow decision stored: action=%s reason=%s", decision.Action, decision.ReasonCode),
		Timestamp: now,
	}, nil
}
