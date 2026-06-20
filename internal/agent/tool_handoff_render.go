package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	toolHandoffRenderMaxCarriers = 8
	toolHandoffRenderMaxRefs     = 12
)

func renderTypedToolHandoffCarriers(title string, carriers []types.ToolHandoffCarrier) string {
	carriers = types.NormalizeToolHandoffCarriers(carriers)
	if len(carriers) == 0 {
		return ""
	}
	sort.SliceStable(carriers, func(i, j int) bool {
		return toolHandoffCarrierRank(carriers[i]) < toolHandoffCarrierRank(carriers[j])
	})
	if len(carriers) > toolHandoffRenderMaxCarriers {
		carriers = carriers[:toolHandoffRenderMaxCarriers]
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "### Typed tool handoff carriers"
	}
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString("Typed carrier fields from prior stages. Hard gates consume the underlying ToolRepair, ObservationRecord, and EvidenceItem artifacts; this view is a bounded prompt projection.\n\n")
	for _, carrier := range carriers {
		line := renderTypedToolHandoffCarrierLine(carrier)
		if line == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
		if refs := renderTypedToolHandoffEvidenceRefs(carrier.AcceptedEvidence, toolHandoffRenderMaxRefs); refs != "" {
			b.WriteString(refs)
		}
		if refs := renderTypedToolHandoffObservationRefs(carrier.ObservationRefs, toolHandoffRenderMaxRefs); refs != "" {
			b.WriteString(refs)
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderTypedToolHandoffCarrierLine(carrier types.ToolHandoffCarrier) string {
	carrier = types.NormalizeToolHandoffCarrier(carrier)
	parts := []string{}
	if carrier.ToolName != "" {
		parts = append(parts, "tool="+quoteHandoffValue(carrier.ToolName))
	}
	if carrier.ReasonCode != "" {
		parts = append(parts, "reason="+quoteHandoffValue(carrier.ReasonCode))
	}
	if carrier.RepairCode != "" {
		parts = append(parts, "repair="+quoteHandoffValue(carrier.RepairCode))
	}
	if carrier.SupportedJSON != nil {
		if len(carrier.SupportedJSON.FailingFieldPaths) > 0 {
			parts = append(parts, "json_fields="+quoteHandoffValue(strings.Join(carrier.SupportedJSON.FailingFieldPaths, ",")))
		}
		if len(carrier.SupportedJSON.AcceptedEnums) > 0 {
			keys := make([]string, 0, len(carrier.SupportedJSON.AcceptedEnums))
			for key := range carrier.SupportedJSON.AcceptedEnums {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parts = append(parts, "enum_fields="+quoteHandoffValue(strings.Join(keys, ",")))
		}
	}
	if len(carrier.AcceptedEvidence) > 0 {
		parts = append(parts, fmt.Sprintf("accepted_evidence=%d", len(carrier.AcceptedEvidence)))
	}
	if len(carrier.ObservationRefs) > 0 {
		parts = append(parts, fmt.Sprintf("observation_refs=%d", len(carrier.ObservationRefs)))
	}
	return strings.Join(parts, " · ")
}

func renderTypedToolHandoffEvidenceRefs(refs []types.AcceptedEvidenceRef, limit int) string {
	if len(refs) == 0 || limit <= 0 {
		return ""
	}
	if len(refs) > limit {
		refs = refs[:limit]
	}
	var b strings.Builder
	for _, ref := range refs {
		if ref.Empty() {
			continue
		}
		fmt.Fprintf(&b, "  - evidence=%s", quoteHandoffValue(ref.ID))
		if ref.Source != "" {
			loc := ref.Source
			if ref.LineStart > 0 {
				loc = fmt.Sprintf("%s:%d", loc, ref.LineStart)
			}
			fmt.Fprintf(&b, " @ %s", quoteHandoffValue(loc))
		}
		if ref.OwnerSymbol != "" {
			fmt.Fprintf(&b, " owner=%s", quoteHandoffValue(ref.OwnerSymbol))
		}
		if ref.AnchorSymbol != "" {
			fmt.Fprintf(&b, " anchor=%s", quoteHandoffValue(ref.AnchorSymbol))
		}
		if ref.SourcePathRole != "" {
			fmt.Fprintf(&b, " role=%s", quoteHandoffValue(string(ref.SourcePathRole)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderTypedToolHandoffObservationRefs(refs []types.ToolObservationRef, limit int) string {
	if len(refs) == 0 || limit <= 0 {
		return ""
	}
	if len(refs) > limit {
		refs = refs[:limit]
	}
	var b strings.Builder
	for _, ref := range refs {
		if ref.Empty() {
			continue
		}
		fmt.Fprintf(&b, "  - observation=%s", quoteHandoffValue(ref.ID))
		if ref.Source != "" {
			loc := ref.Source
			if ref.LineStart > 0 {
				loc = fmt.Sprintf("%s:%d", loc, ref.LineStart)
			}
			fmt.Fprintf(&b, " @ %s", quoteHandoffValue(loc))
		}
		if ref.Producer != "" {
			fmt.Fprintf(&b, " producer=%s", quoteHandoffValue(ref.Producer))
		}
		if ref.ClaimKey != "" {
			fmt.Fprintf(&b, " claim=%s", quoteHandoffValue(ref.ClaimKey))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func toolHandoffCarrierRank(carrier types.ToolHandoffCarrier) int {
	switch {
	case carrier.SupportedJSON != nil || carrier.PlanRepairPack != nil || carrier.Repair != nil:
		return 0
	case len(carrier.AcceptedEvidence) > 0:
		return 1
	case len(carrier.ObservationRefs) > 0:
		return 2
	default:
		return 3
	}
}

func quoteHandoffValue(value string) string {
	value = truncateExtractorPromptText(strings.TrimSpace(value), 180)
	if value == "" {
		return "\"\""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	return "`" + value + "`"
}
