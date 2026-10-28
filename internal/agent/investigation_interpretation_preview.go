package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type investigationInterpretationPreview struct {
	text                       string
	totalBytes, publishedBytes int
	sanitized                  bool
}

// Budget complete declared units, never a claim separated from its limits.
// The legacy paragraph lane is explicitly incomplete when anything is omitted;
// paragraph boundaries are not a claim that semantic limitations are complete.
func previewInvestigationInterpretation(h *types.InvestigationNativeFactHandoff, budget int, sanitize func(string) string) investigationInterpretationPreview {
	return previewInvestigationInterpretationWithNativeReferences(h, budget, sanitize, false)
}

func previewInvestigationInterpretationWithNativeReferences(h *types.InvestigationNativeFactHandoff, budget int, sanitize func(string) string, nativeReferencesCurrent bool) investigationInterpretationPreview {
	var out investigationInterpretationPreview
	if h == nil || h.InterpretationRole == types.InvestigationNativeRestatement {
		return out
	}
	budget = max(0, budget)
	var parts []string
	var eligible []bool
	unitMode := len(h.InterpretationUnits) > 0
	if unitMode {
		unchangedContent := func(s string) bool {
			// The existing draft sanitizer normalizes paragraph whitespace.
			// Formatting alone is not content removal; retain original bytes
			// (and charge them) while still dropping a whole filtered unit.
			return interpretationParagraphWhitespace(sanitize(s)) == interpretationParagraphWhitespace(s)
		}
		for i, unit := range h.InterpretationUnits {
			raw, _ := json.Marshal(unit)
			part := fmt.Sprintf("- unit[%d]: %s\n\n", i, raw)
			if len(unit.NativeRefs) > 0 {
				status := "unverified_address"
				if nativeReferencesCurrent {
					status = "current_exact_rows"
				}
				part += "  native_reference_status=" + status + "; references only the original table rows and that selector's source/query/notes. Address validation does not validate text, arithmetic, causality or diagram notes, and does not certify row publication.\n\n"
			}
			parts = append(parts, part)
			unchanged := unchangedContent(unit.Text)
			for _, fields := range [][]string{unit.Boundaries, unit.SupportRefs} {
				for _, field := range fields {
					unchanged = unchanged && unchangedContent(field)
				}
			}
			eligible = append(eligible, unit.Valid() && unchanged)
			out.sanitized = out.sanitized || !unchanged
		}
	} else {
		original := strings.TrimSpace(h.NarrativeInterpretation())
		clean := sanitize(original)
		out.sanitized = clean != original
		if clean == "" {
			return out
		}
		// Preserve byte identity for small legacy narratives. Only an oversized
		// legacy narrative uses whole paragraphs, with explicit incompleteness.
		if len(clean) <= budget {
			out.text, out.totalBytes, out.publishedBytes = clean, len(clean), len(clean)
			return out
		}
		for _, paragraph := range strings.Split(clean, "\n\n") {
			if paragraph = strings.TrimSpace(paragraph); paragraph != "" {
				parts = append(parts, paragraph+"\n\n")
				eligible = append(eligible, true)
			}
		}
	}
	var b strings.Builder
	var shown []int
	for i, part := range parts {
		out.totalBytes += len(part)
		if eligible[i] && len(part) <= budget-out.publishedBytes {
			b.WriteString(part)
			out.publishedBytes += len(part)
			shown = append(shown, i)
		}
	}
	if len(shown) < len(parts) {
		kind := "legacy paragraphs"
		if unitMode {
			kind = "declared units"
		}
		fmt.Fprintf(&b, "Interpretation is partial: %d of %d %s shown whole (source indices %v); %d bytes omitted. Omitted material may contain essential limitations: do not treat this preview as a complete interpretation. Full original native_fact_handoff remains available for audit.\n", len(shown), len(parts), kind, shown, out.totalBytes-out.publishedBytes)
	}
	out.text = b.String()
	return out
}

func interpretationParagraphWhitespace(s string) string {
	var paragraphs []string
	for _, p := range priorDraftParaSplitRe.Split(s, -1) {
		if p = strings.TrimSpace(p); p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	return strings.Join(paragraphs, "\n\n")
}
