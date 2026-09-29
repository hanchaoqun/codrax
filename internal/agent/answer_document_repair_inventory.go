package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Recompute from the current patch base, including after a rejected malformed
// patch. A transient patch-id error must not hide the still-missing carrier.
// This is bounded guidance, not a write lease or a new rejection condition.
func answerDocPatchContentPreservationHint(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) string {
	if doc == nil || view == nil {
		return ""
	}
	var missing []*types.AnswerBlockCountRepair
	for _, req := range view.RequiredBlocks {
		r := types.NewAnswerBlockCountRepair(req, types.CountAnswerBlocksForRequirement(doc.Blocks, req))
		if r != nil && r.Operation == "add_blocks" {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	type payload struct {
		ID           string `json:"id"`
		Items        int    `json:"items,omitempty"`
		Columns      int    `json:"columns,omitempty"`
		TextBytes    int    `json:"text_bytes,omitempty"`
		DiagramBytes int    `json:"diagram_bytes,omitempty"`
	}
	var inventory []payload
	const maxInventory = 24
	omitted := 0
	for _, block := range doc.Blocks {
		p := payload{ID: block.ID, Items: len(block.Items), Columns: len(block.Columns), TextBytes: len(block.Text)}
		if block.Diagram != nil {
			p.DiagramBytes = len(block.Diagram.Body)
		}
		if p.Items+p.Columns+p.TextBytes+p.DiagramBytes == 0 {
			continue
		}
		if len(inventory) == maxInventory {
			omitted++
			continue
		}
		inventory = append(inventory, p)
	}
	var b strings.Builder
	if len(inventory) > 0 {
		raw, _ := json.Marshal(inventory)
		fmt.Fprintf(&b, " Existing payload inventory (counts, not proof of correctness): `%s`.", raw)
		if omitted > 0 {
			fmt.Fprintf(&b, " %d further payload-bearing blocks are not repeated here.", omitted)
		}
	}
	b.WriteString(" Block ids are opaque: an id spelling never determines kind. Keep unrelated payloads inherited; replacing a table/list/diagram with a summary removes its content, it does not add a summary.")
	for i, repair := range missing {
		if i == 4 {
			fmt.Fprintf(&b, " %d further count deficits remain in the answer contract.", len(missing)-i)
			break
		}
		b.WriteString(" ")
		b.WriteString(repair.Instruction())
		b.WriteByte('.')
	}
	return b.String()
}
