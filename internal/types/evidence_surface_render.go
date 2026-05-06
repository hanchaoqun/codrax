package types

import (
	"fmt"
	"strings"
)

func prependEvidenceKind(includeKind bool, item EvidenceItem, text string) string {
	text = strings.TrimSpace(text)
	if !includeKind {
		return text
	}
	if text == "" {
		return fmt.Sprintf("[%s]", item.Kind)
	}
	return fmt.Sprintf("[%s] %s", item.Kind, text)
}

func evidenceAnchorLocalSurfaceText(item EvidenceItem, includeKind bool) string {
	snippet := strings.TrimSpace(item.Snippet)
	locationName := strings.TrimSpace(firstNonEmptySurfaceString(item.OwnerSymbol, item.AnchorSymbol, item.Subject))
	switch item.AnchorKind {
	case AnchorAssignment:
		if snippet != "" {
			return prependEvidenceKind(includeKind, item, snippet)
		}
		switch {
		case locationName != "" && strings.TrimSpace(item.AnchorSymbol) != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s assignment anchor for %s", locationName, strings.TrimSpace(item.AnchorSymbol)))
		case locationName != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s assignment anchor", locationName))
		case strings.TrimSpace(item.AnchorSymbol) != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s assignment anchor", strings.TrimSpace(item.AnchorSymbol)))
		}
	case AnchorReturn:
		if snippet != "" {
			return prependEvidenceKind(includeKind, item, snippet)
		}
		switch {
		case locationName != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s return statement", locationName))
		case strings.TrimSpace(item.AnchorSymbol) != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s return statement", strings.TrimSpace(item.AnchorSymbol)))
		}
	case AnchorDefinition:
		if snippet != "" {
			return prependEvidenceKind(includeKind, item, snippet)
		}
	case AnchorCondition:
		if snippet != "" {
			return prependEvidenceKind(includeKind, item, snippet)
		}
		cond := strings.TrimSpace(item.Condition)
		switch {
		case locationName != "" && cond != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s guard condition IF %s", locationName, cond))
		case cond != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("guard condition IF %s", cond))
		case locationName != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s guard anchor", locationName))
		case strings.TrimSpace(item.AnchorSymbol) != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s guard anchor", strings.TrimSpace(item.AnchorSymbol)))
		}
	case AnchorImport:
		if snippet != "" {
			return prependEvidenceKind(includeKind, item, snippet)
		}
	}
	return ""
}

func evidenceSurfaceText(item EvidenceItem, includeKind bool, allowSummaryFallback bool) string {
	if line := evidenceAnchorLocalSurfaceText(item, includeKind); line != "" {
		return line
	}

	parts := make([]string, 0, 4)
	if includeKind {
		parts = append(parts, fmt.Sprintf("[%s]", item.Kind))
	}

	subject := firstNonEmptySurfaceString(item.Subject, item.AnchorSymbol, item.OwnerSymbol)
	if subject != "" {
		parts = append(parts, subject)
	}
	if predicate := strings.TrimSpace(item.Predicate); predicate != "" {
		parts = append(parts, predicate)
	}
	if object := strings.TrimSpace(item.Object); object != "" {
		parts = append(parts, object)
	}

	line := strings.Join(parts, " ")
	if cond := strings.TrimSpace(item.Condition); cond != "" {
		if strings.TrimSpace(line) != "" {
			line += " IF " + cond
		} else {
			line = cond
		}
	}

	kindOnly := includeKind && strings.TrimSpace(line) == fmt.Sprintf("[%s]", item.Kind)
	if strings.TrimSpace(line) != "" && !kindOnly {
		return strings.TrimSpace(line)
	}

	if snippet := strings.TrimSpace(item.Snippet); snippet != "" {
		return snippet
	}
	if allowSummaryFallback {
		if summary := strings.TrimSpace(item.Summary); summary != "" {
			return summary
		}
	}
	return ""
}

// EvidenceStructuredSemanticLine renders the structured semantic core of an
// evidence item without relying on its free-form summary text. It is the
// shared conservative formatter used when downstream stages need a stable,
// cross-stage-safe surface.
func EvidenceStructuredSemanticLine(item EvidenceItem, includeKind bool) string {
	return evidenceSurfaceText(item, includeKind, false)
}

// EvidenceAuthoritativeSurfaceText returns a summary-free, anchor-local
// surface for downstream channels that the system treats as
// authoritative prompt material. Unlike EvidenceDeterministicSurfaceText,
// it NEVER falls back to item.Summary and it keeps definition / call /
// import anchors on neutral typed wording unless the grounded snippet
// itself can be replayed verbatim.
func EvidenceAuthoritativeSurfaceText(item EvidenceItem, includeKind bool) string {
	if line := evidenceAnchorLocalSurfaceText(item, includeKind); line != "" {
		return line
	}

	name := strings.TrimSpace(firstNonEmptySurfaceString(item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object))
	subject := strings.TrimSpace(item.Subject)
	object := strings.TrimSpace(item.Object)

	switch item.AnchorKind {
	case AnchorDefinition:
		if name != "" {
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("definition anchor for %s", name))
		}
	case AnchorCall:
		switch {
		case subject != "" && object != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s calls %s", subject, object))
		case name != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("call anchor for %s", name))
		}
	case AnchorImport:
		switch {
		case subject != "" && object != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("%s imports %s", subject, object))
		case name != "":
			return prependEvidenceKind(includeKind, item, fmt.Sprintf("import anchor for %s", name))
		}
	}

	return EvidenceStructuredSemanticLine(item, includeKind)
}

// EvidenceDeterministicSurfaceText renders the most conservative typed
// surface for an evidence item. It prefers structured fields and
// snippets; free-form Summary is used only as a last resort when the
// item has no better typed representation.
//
// Use this helper for deterministic / authoritative downstream
// channels such as Primary Evidence, Resolution Chains, or other
// prompt sections that are meant to be a stable replay of typed
// evidence rather than a prose-rich interpretation.
func EvidenceDeterministicSurfaceText(item EvidenceItem, includeKind bool) string {
	if line := evidenceSurfaceText(item, includeKind, false); line != "" {
		return line
	}
	return strings.TrimSpace(item.Summary)
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
