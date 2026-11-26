package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

type finalizerRuntimeReceiptPatch struct {
	draft   map[string]json.RawMessage
	edits   []map[string]json.RawMessage
	index   int
	blockID string
}

func newFinalizerRuntimeReceiptPatch(result types.ToolResult, call llm.ToolCall) *finalizerRuntimeReceiptPatch {
	if result.Success || result.ToolName != call.Name || result.Repair == nil || len(result.Repair.Fields) != 1 {
		return nil
	}
	field := result.Repair.Fields[0]
	const receiptField = "runtime_work_relation"
	if call.Name == "emit_answer_document" && strings.HasPrefix(field, "blocks[id=") && strings.HasSuffix(field, "]."+receiptField) {
		id := strings.TrimSuffix(strings.TrimPrefix(field, "blocks[id="), "]."+receiptField)
		raw, _ := json.Marshal(id)
		return &finalizerRuntimeReceiptPatch{
			draft:   map[string]json.RawMessage{},
			edits:   []map[string]json.RawMessage{{"block_id": raw, "field": json.RawMessage(`"runtime_work_relation"`)}},
			blockID: id,
		}
	}
	if call.Name != "emit_answer_document_patch" || result.Repair.Code != "answer_doc_patch_receipt_branch_not_published" ||
		result.Repair.Metadata["field"] != receiptField || result.Repair.Metadata["reason"] != "value_not_published" {
		return nil
	}
	p := &finalizerRuntimeReceiptPatch{blockID: result.Repair.Metadata["block_id"]}
	if _, err := fmt.Sscanf(field, "block_receipt_edits_v1[%d]", &p.index); err != nil ||
		json.Unmarshal(call.Params, &p.draft) != nil || json.Unmarshal(p.draft["block_receipt_edits_v1"], &p.edits) != nil ||
		p.index < 0 || p.index >= len(p.edits) {
		return nil
	}
	return p
}

func (p *finalizerRuntimeReceiptPatch) project(ctx *types.AgentContext, original llm.ToolSchema) (llm.ToolSchema, llm.Message, map[string]json.RawMessage) {
	var schema struct {
		Properties struct {
			Edits struct {
				Items struct {
					Branches []struct {
						Properties struct {
							BlockID struct {
								Enum []string `json:"enum"`
							} `json:"block_id"`
							Field struct {
								Const string `json:"const"`
							} `json:"field"`
							Value struct {
								Choices []struct {
									Properties map[string]struct {
										Const string `json:"const"`
									} `json:"properties"`
								} `json:"oneOf"`
							} `json:"value"`
						} `json:"properties"`
					} `json:"oneOf"`
				} `json:"items"`
			} `json:"block_receipt_edits_v1"`
		} `json:"properties"`
	}
	if json.Unmarshal(original.Parameters, &schema) != nil {
		return llm.ToolSchema{}, llm.Message{}, nil
	}
	rows := map[string]types.RuntimeWorkRelationRow{}
	if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view != nil && view.RuntimeWorkRelationContract.Active() {
		for _, row := range view.RuntimeWorkRelationContract.Rows {
			rows[row.ObservationID] = row
		}
	}
	choices := map[string]json.RawMessage{}
	var ids, notes []string
	for _, branch := range schema.Properties.Edits.Items.Branches {
		props := branch.Properties
		if props.Field.Const != "runtime_work_relation" || len(props.BlockID.Enum) > 0 && !slices.Contains(props.BlockID.Enum, p.blockID) {
			continue
		}
		for _, value := range props.Value.Choices {
			observation, conclusion := value.Properties["observation_id"].Const, value.Properties["conclusion"].Const
			row := rows[observation]
			id := fmt.Sprintf("r%d", len(ids)+1)
			ids = append(ids, id)
			choices[id], _ = json.Marshal(map[string]string{"observation_id": observation, "conclusion": conclusion})
			notes = append(notes, fmt.Sprintf("%s: %s %s %.3fms; %s; %s", id, row.Subject, row.WorkLabel, row.MeasuredDurationMS, conclusion, row.Boundary))
		}
	}
	if len(ids) == 0 {
		return llm.ToolSchema{}, llm.Message{}, nil
	}
	var text string
	if base := answerDocumentPatchBaseDocumentAcrossMutableState(ctx, nil); base != nil {
		for _, b := range base.Blocks {
			if b.ID == p.blockID {
				text = b.Text
				break
			}
		}
	}
	params, _ := json.Marshal(map[string]any{
		"type": "object", "properties": map[string]any{"receipt": map[string]any{"type": "string", "enum": ids}},
		"required": []string{"receipt"}, "additionalProperties": false,
	})
	return llm.ToolSchema{Name: original.Name, Description: "Select the exact replacement receipt for the existing answer block.", Parameters: params},
		llm.Message{Role: "user", Content: "Repair only the current block's evidence receipt. Select a listed row and its evidence-bounded verdict. A wakeup identifies the waker; it cannot establish the waiting object or prove that rendering blocked the target. Keep the answer and citations unchanged.\nBlock: " + text + "\nCurrent choices:\n" + strings.Join(notes, "\n")}, choices
}

func (p *finalizerRuntimeReceiptPatch) expand(raw json.RawMessage, choices map[string]json.RawMessage) json.RawMessage {
	var reply struct {
		Receipt string `json:"receipt"`
	}
	value := json.RawMessage(`{}`)
	if json.Unmarshal(raw, &reply) == nil && choices[reply.Receipt] != nil {
		value = choices[reply.Receipt]
	}
	p.edits[p.index]["value"] = value
	p.draft["block_receipt_edits_v1"], _ = json.Marshal(p.edits)
	out, _ := json.Marshal(p.draft)
	return out
}
