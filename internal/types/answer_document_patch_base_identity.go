package types

import (
	"fmt"
	"strings"
)

// ValidateAnswerDocumentPatchBaseIdentity checks whether block IDs address one
// retained block each. A conflicting draft remains useful for recovery, but no
// ID-based edit may choose between its model-authored versions implicitly.
func ValidateAnswerDocumentPatchBaseIdentity(doc *AnswerDocumentV2) error {
	if violations := collectAnswerDocumentPatchBaseIdentityViolations(doc); len(violations) > 0 {
		return fmt.Errorf("patch base: %s; use emit_answer_document with unique, non-empty block IDs to resolve the retained draft, not an ID-based patch", strings.Join(violations, "; "))
	}
	return nil
}

func collectAnswerDocumentPatchBaseIdentityViolations(doc *AnswerDocumentV2) []string {
	if doc == nil || len(doc.Blocks) == 0 {
		return []string{"no blocks"}
	}
	seen := make(map[string]bool, len(doc.Blocks))
	var violations []string
	for i, block := range doc.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" || id != block.ID {
			violations = append(violations, fmt.Sprintf("blocks[%d] has an invalid block ID", i))
		}
		if seen[id] {
			violations = append(violations, fmt.Sprintf("duplicate block ID %q", id))
		}
		seen[id] = true
	}
	return violations
}
