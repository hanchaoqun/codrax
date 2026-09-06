package types

import (
	"fmt"
	"strings"
)

// ValidateAnswerDocumentPatchBaseIdentity checks whether block IDs address one
// retained block each. A conflicting draft remains useful for recovery, but no
// ID-based edit may choose between its model-authored versions implicitly.
func ValidateAnswerDocumentPatchBaseIdentity(doc *AnswerDocumentV2) error {
	if doc == nil || len(doc.Blocks) == 0 {
		return fmt.Errorf("patch base has no blocks; use emit_answer_document for a complete answer")
	}
	seen := make(map[string]bool, len(doc.Blocks))
	for i, block := range doc.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" || id != block.ID {
			return fmt.Errorf("patch base blocks[%d] has an invalid block ID; use emit_answer_document with unique, non-empty block IDs", i)
		}
		if seen[id] {
			return fmt.Errorf("patch base contains duplicate block ID %q; use emit_answer_document with unique block IDs to resolve the retained draft, not an ID-based patch", id)
		}
		seen[id] = true
	}
	return nil
}
