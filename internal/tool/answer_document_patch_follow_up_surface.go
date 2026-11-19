package tool

import (
	"encoding/json"
	"strings"
)

// answer_document_patch_follow_up_surface.go — HMC §377 D1: the request
// surface of the one optional follow-up offered after an accepted answer.
//
// The follow-up offers named patch operations (for example the root-cause
// selection alone). Advertising the whole compatibility patch surface for it
// cost more than the complete answer tool of the first request (batch 376 B:
// 144,176 B against 99,292 B) and pushed the request past its unchanged
// allocation, so the offer was never sent. The follow-up surface is the
// dispatch-projected patch schema with only the offered top-level operations
// kept; every kept property node is the one the dispatch projection already
// built (one source per field rule), nothing is rewritten or added.
//
// This is a request-surface projection, not a capability gate: the executor
// is unchanged, so a call that also carries an operation outside the offer
// behaves as it does today. No request, model or answer text is read.

// AnswerDocumentPatchFollowUpDescription is the tool description of a
// projected follow-up surface. Field rules stay on their property schemas.
const AnswerDocumentPatchFollowUpDescription = "Optional follow-up to your accepted structured answer. The parameters publish only the operations this follow-up offers, and each field description is that field's rule. Blocks you do not name stay exactly as accepted, and nothing changes unless the merged document validates. The system selects no candidate, wording, layout, or conclusion."

// ProjectAnswerDocumentPatchFollowUpParameters keeps only the named top-level
// operations of an already dispatch-projected patch schema. It returns false,
// and the caller keeps the whole surface, when the schema is not an object
// schema or does not publish every named operation: a follow-up never
// advertises an operation the dispatch projection did not build.
func ProjectAnswerDocumentPatchFollowUpParameters(raw json.RawMessage, operations []string) (json.RawMessage, bool) {
	if len(operations) == 0 {
		return raw, false
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw, false
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return raw, false
	}
	kept := make(map[string]any, len(operations))
	for _, operation := range operations {
		node, ok := properties[strings.TrimSpace(operation)]
		if !ok {
			return raw, false
		}
		kept[operation] = node
	}
	root["properties"] = kept
	out, err := json.Marshal(root)
	if err != nil || !json.Valid(out) {
		return raw, false
	}
	return out, true
}
