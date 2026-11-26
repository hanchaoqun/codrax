package tool

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/types"
)

func normalizeFlatRuntimeWorkBinding(raw json.RawMessage) (json.RawMessage, bool) {
	var document map[string]json.RawMessage
	if json.Unmarshal(raw, &document) != nil {
		return raw, false
	}
	var binding emitRuntimeWorkBinding
	if json.Unmarshal(document["runtime_work_binding"], &binding.BlockID) != nil ||
		json.Unmarshal(document["observation_id"], &binding.ObservationID) != nil ||
		json.Unmarshal(document["conclusion"], &binding.Conclusion) != nil ||
		binding.BlockID == "" || binding.ObservationID == "" || !types.RuntimeWorkRelationConclusion(binding.Conclusion).IsValid() {
		return raw, false
	}
	document["runtime_work_binding"], _ = json.Marshal(binding)
	delete(document, "observation_id")
	delete(document, "conclusion")
	out, _ := json.Marshal(document)
	return out, true
}

func normalizeRuntimeWorkReceiptAliases(raw json.RawMessage, contract *types.RuntimeWorkRelationContract) (json.RawMessage, bool) {
	if !contract.Active() {
		return raw, false
	}
	var document map[string]json.RawMessage
	var edits []map[string]json.RawMessage
	if json.Unmarshal(raw, &document) != nil || json.Unmarshal(document["block_receipt_edits_v1"], &edits) != nil {
		return raw, false
	}
	changed := false
	for _, edit := range edits {
		var field string
		var value map[string]json.RawMessage
		var receipt types.AnswerRuntimeWorkRelationReceipt
		if json.Unmarshal(edit["field"], &field) != nil || field != string(types.AnswerBlockReceiptFieldRuntimeWorkRelation) ||
			json.Unmarshal(edit["value"], &value) != nil || json.Unmarshal(edit["value"], &receipt) != nil {
			continue
		}
		original := receipt.ObservationID
		if types.BindRuntimeWorkRelationReceipt(&receipt, contract) && receipt.ObservationID != original {
			value["observation_id"], _ = json.Marshal(receipt.ObservationID)
			edit["value"], _ = json.Marshal(value)
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	document["block_receipt_edits_v1"], _ = json.Marshal(edits)
	out, _ := json.Marshal(document)
	return out, true
}
