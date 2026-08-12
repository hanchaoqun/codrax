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

type toolHandoffRenderOptions struct {
	HistoricalProducerLabels bool
	CurrentStageAllowedTools []string
	MaxCarriers              int
	MaxRefs                  int
	// ObservationDetails is an optional, ID-keyed value projection for
	// observation families whose exact bounded values must survive beside a
	// handoff ref. The generic carrier remains identity-only.
	ObservationDetails map[string]types.ObservationPromptRecord
}

func renderTypedToolHandoffCarriers(title string, carriers []types.ToolHandoffCarrier, options ...toolHandoffRenderOptions) string {
	opts := toolHandoffRenderOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	carriers = types.NormalizeToolHandoffCarriers(carriers)
	if len(carriers) == 0 {
		return ""
	}
	sort.SliceStable(carriers, func(i, j int) bool {
		return types.ToolHandoffCarrierProjectionRank(carriers[i]) < types.ToolHandoffCarrierProjectionRank(carriers[j])
	})
	maxCarriers := opts.MaxCarriers
	if maxCarriers <= 0 {
		maxCarriers = toolHandoffRenderMaxCarriers
	}
	maxRefs := opts.MaxRefs
	if maxRefs <= 0 {
		maxRefs = toolHandoffRenderMaxRefs
	}
	if len(carriers) > maxCarriers {
		carriers = carriers[:maxCarriers]
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "### Typed tool handoff carriers"
	}
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString("Typed carrier fields from prior stages. Hard gates consume the underlying ToolRepair, ObservationRecord, and EvidenceItem artifacts; this view is a bounded prompt projection.\n\n")
	if opts.HistoricalProducerLabels {
		b.WriteString("Tool names below identify prior-stage producers or prior-stage refinements, not current-stage callable tools.")
		if len(opts.CurrentStageAllowedTools) > 0 {
			b.WriteString(" Current callable tools here: `")
			b.WriteString(strings.Join(opts.CurrentStageAllowedTools, "`, `"))
			b.WriteString("`.")
		}
		b.WriteString(" Do not reopen investigation from this handoff; convert unresolved prior-stage refinements into a caveat or lower-confidence structured emit.\n\n")
	}
	for _, carrier := range carriers {
		line := renderTypedToolHandoffCarrierLine(carrier, opts)
		if line == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
		if refs := renderTypedToolHandoffEvidenceRefs(carrier.AcceptedEvidence, maxRefs); refs != "" {
			b.WriteString(refs)
		}
		if refs := renderTypedToolHandoffObservationRefs(carrier.ObservationRefs, maxRefs, opts.ObservationDetails); refs != "" {
			b.WriteString(refs)
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderTypedToolHandoffCarrierLine(carrier types.ToolHandoffCarrier, options ...toolHandoffRenderOptions) string {
	opts := toolHandoffRenderOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	carrier = types.NormalizeToolHandoffCarrier(carrier)
	parts := []string{}
	if carrier.ToolName != "" {
		field := "tool"
		if opts.HistoricalProducerLabels {
			field = "producer_tool"
		}
		parts = append(parts, field+"="+quoteHandoffValue(carrier.ToolName))
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
		if len(carrier.SupportedJSON.AcceptedFieldPaths) > 0 {
			parts = append(parts, "accepted_json_fields="+quoteHandoffValue(strings.Join(boundedStringSlice(carrier.SupportedJSON.AcceptedFieldPaths, 12), ",")))
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
		parts = append(parts, fmt.Sprintf("evidence_refs=%d", len(carrier.AcceptedEvidence)))
		if counts := handoffEvidenceGroundingCounts(carrier.AcceptedEvidence); counts != "" {
			parts = append(parts, "evidence_grounding="+quoteHandoffValue(counts))
		}
	}
	if len(carrier.ObservationRefs) > 0 {
		parts = append(parts, fmt.Sprintf("observation_refs=%d", len(carrier.ObservationRefs)))
	}
	parts = append(parts, renderTypedToolRefinementParts(carrier.Refinement, opts)...)
	return strings.Join(parts, " · ")
}

func renderTypedToolRefinementParts(refinement *types.ToolRefinementHint, options ...toolHandoffRenderOptions) []string {
	opts := toolHandoffRenderOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return types.ToolRefinementPromptFields(refinement, types.ToolRefinementPromptFieldOptions{
		HistoricalProducerLabels: opts.HistoricalProducerLabels,
		QuoteValues:              true,
	})
}

func boundedStringSlice(values []string, limit int) []string {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
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
		if ref.ClaimForm != types.ClaimUnknown {
			fmt.Fprintf(&b, " claim_form=%s", quoteHandoffValue(string(ref.ClaimForm)))
		}
		if boundary := types.MechanismAuthorityBoundaryForClaimForm(ref.ClaimForm); boundary != "" {
			fmt.Fprintf(&b, " authority=%s", quoteHandoffValue(boundary))
		}
		if ref.SourcePathRole != "" {
			fmt.Fprintf(&b, " role=%s", quoteHandoffValue(string(ref.SourcePathRole)))
		}
		status := strings.TrimSpace(string(ref.GroundingStatus))
		if status == "" {
			status = "unspecified"
		}
		fmt.Fprintf(&b, " grounding=%s", quoteHandoffValue(status))
		b.WriteString("\n")
	}
	return b.String()
}

func handoffEvidenceGroundingCounts(refs []types.AcceptedEvidenceRef) string {
	if len(refs) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, ref := range refs {
		status := strings.TrimSpace(string(ref.GroundingStatus))
		if status == "" {
			status = "unspecified"
		}
		counts[status]++
	}
	order := []string{
		string(types.GroundingGrounded),
		string(types.GroundingRecovered),
		string(types.GroundingUngrounded),
		"unspecified",
	}
	parts := make([]string, 0, len(counts))
	for _, status := range order {
		if count := counts[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", status, count))
			delete(counts, status)
		}
	}
	if len(counts) > 0 {
		rest := make([]string, 0, len(counts))
		for status := range counts {
			rest = append(rest, status)
		}
		sort.Strings(rest)
		for _, status := range rest {
			parts = append(parts, fmt.Sprintf("%s:%d", status, counts[status]))
		}
	}
	return strings.Join(parts, ",")
}

func renderTypedToolHandoffObservationRefs(refs []types.ToolObservationRef, limit int, details map[string]types.ObservationPromptRecord) string {
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
		if record, ok := details[strings.TrimSpace(ref.ID)]; ok {
			for _, note := range targetWaitOccurrenceHandoffNotes(record.Notes) {
				fmt.Fprintf(&b, "    - observation_value=%q\n", note)
			}
		}
	}
	return b.String()
}

func targetWaitOccurrenceHandoffNotes(notes []string) []string {
	if len(notes) == 0 {
		return nil
	}
	prefixes := []string{
		types.TraceNoteKeyTargetWaitOccurrencePrompt + "=",
		types.TraceNoteKeyTargetWaitOccurrencePromptSum + "=",
		types.TraceNoteKeyTargetWaitOccurrence + "=",
	}
	out := make([]string, 0, len(notes))
	for _, note := range notes {
		note = strings.TrimSpace(note)
		for _, prefix := range prefixes {
			if strings.HasPrefix(note, prefix) {
				out = append(out, note)
				break
			}
		}
	}
	return out
}

func quoteHandoffValue(value string) string {
	value = truncateExtractorPromptText(strings.TrimSpace(value), 180)
	if value == "" {
		return "\"\""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	return "`" + value + "`"
}
