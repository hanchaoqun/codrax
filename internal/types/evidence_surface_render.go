package types

import (
	"fmt"
	"strings"
)

// EvidenceStructuredSemanticLine renders the structured semantic core of an
// evidence item without relying on its free-form summary text. It is the
// shared conservative formatter used when downstream stages need a stable,
// cross-stage-safe surface.
func EvidenceStructuredSemanticLine(item EvidenceItem, includeKind bool) string {
	parts := make([]string, 0, 4)
	if includeKind {
		parts = append(parts, fmt.Sprintf("[%s]", item.Kind))
	}
	if item.Subject != "" {
		parts = append(parts, item.Subject)
	}
	if item.Predicate != "" {
		parts = append(parts, item.Predicate)
	}
	if item.Object != "" {
		parts = append(parts, item.Object)
	}
	if len(parts) == 0 && item.AnchorSymbol != "" {
		parts = append(parts, item.AnchorSymbol)
	}
	line := strings.Join(parts, " ")
	if item.Condition != "" {
		line += " IF " + item.Condition
	}
	if strings.TrimSpace(line) == "" || (includeKind && strings.TrimSpace(line) == fmt.Sprintf("[%s]", item.Kind)) {
		switch {
		case strings.TrimSpace(item.Snippet) != "":
			line = strings.TrimSpace(item.Snippet)
		case strings.TrimSpace(item.Summary) != "":
			line = strings.TrimSpace(item.Summary)
		}
	}
	return strings.TrimSpace(line)
}

// EvidencePreferredSurfaceText returns the evidence text that should be shown
// to downstream LLM stages. For ordinary evidence it preserves the existing
// summary-first behaviour. For exact-resolution surface-relevant evidence it
// drops back to the structured semantic core so nearby-context prose cannot
// re-introduce target mentions or operational notes into later stages.
func EvidencePreferredSurfaceText(item EvidenceItem, contract *ExactResolutionContract, includeKind bool) string {
	if contract != nil && ExactResolutionContextSurfaceRelevant(contract, item) {
		if line := EvidenceStructuredSemanticLine(item, includeKind); line != "" {
			return line
		}
	}
	if line := strings.TrimSpace(item.Summary); line != "" {
		return line
	}
	return EvidenceStructuredSemanticLine(item, includeKind)
}

