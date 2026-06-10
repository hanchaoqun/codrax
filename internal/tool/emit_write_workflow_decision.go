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
		return failStrictDecodeWithError(t.Name(), now, err, nil)
	}
	decision = writeflow.NormalizeWriteWorkflowDecision(decision)
	if errs := writeflow.ValidateWriteWorkflowDecision(decision); len(errs) > 0 {
		return errResult(t.Name(), "emit_write_workflow_decision rejected: "+strings.Join(errs, "; ")), nil
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
