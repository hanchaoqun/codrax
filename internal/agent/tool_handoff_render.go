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
		if refs := renderTypedToolHandoffObservationRefs(carrier.ObservationRefs, maxRefs); refs != "" {
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
		parts = append(parts, fmt.Sprintf("accepted_evidence=%d", len(carrier.AcceptedEvidence)))
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
	if refinement == nil {
		return nil
	}
	hint := types.NormalizeToolRefinementHint(*refinement)
	if hint.Empty() {
		return nil
	}
	parts := []string{}
	flags := []string{}
	if hint.ResultTruncated {
		flags = append(flags, "result_truncated")
	}
	if hint.CandidateBudgetTruncated {
		flags = append(flags, "candidate_budget_truncated")
	}
	if len(flags) > 0 {
		parts = append(parts, "refine_flags="+quoteHandoffValue(strings.Join(flags, ",")))
	}
	if hint.UniverseExcludedReason != "" {
		parts = append(parts, "excluded_reason="+quoteHandoffValue(hint.UniverseExcludedReason))
	}
	if hint.ResultTruncated || hint.CandidateBudgetTruncated || hint.PreferredNextTool != "" || len(hint.PreferredParams) > 0 {
		parts = append(parts, "refine_action=`soft_narrow_if_answer_critical_else_caveat`")
	}
	if hint.PreferredNextTool != "" {
		field := "preferred_tool"
		if opts.HistoricalProducerLabels {
			field = "prior_stage_preferred_tool"
		}
		parts = append(parts, field+"="+quoteHandoffValue(hint.PreferredNextTool))
	}
	if len(hint.PreferredParams) > 0 {
		keys := make([]string, 0, len(hint.PreferredParams))
		for key := range hint.PreferredParams {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, key+"="+hint.PreferredParams[key])
		}
		field := "preferred_params"
		if opts.HistoricalProducerLabels {
			field = "prior_stage_preferred_params"
		}
		parts = append(parts, field+"="+quoteHandoffValue(strings.Join(values, ",")))
	}
	if len(hint.RequiredFields) > 0 {
		field := "required_fields"
		if opts.HistoricalProducerLabels {
			field = "prior_stage_required_fields"
		}
		parts = append(parts, field+"="+quoteHandoffValue(strings.Join(boundedStringSlice(hint.RequiredFields, 8), ",")))
	}
	if hint.NextCursor != "" {
		field := "next_cursor"
		if opts.HistoricalProducerLabels {
			field = "prior_stage_next_cursor"
		}
		parts = append(parts, field+"="+quoteHandoffValue(hint.NextCursor))
	}
	if len(hint.SkippedLargeCandidates) > 0 {
		parts = append(parts, "skipped_large="+quoteHandoffValue(strings.Join(boundedStringSlice(hint.SkippedLargeCandidates, 4), ",")))
	}
	if len(hint.ExcludedRoots) > 0 {
		parts = append(parts, "excluded_roots="+quoteHandoffValue(strings.Join(boundedStringSlice(hint.ExcludedRoots, 4), ",")))
	}
	if len(hint.TopSourceClasses) > 0 {
		classes := make([]string, 0, len(hint.TopSourceClasses))
		for _, role := range hint.TopSourceClasses {
			if role != "" {
				classes = append(classes, string(role))
			}
		}
		if len(classes) > 0 {
			parts = append(parts, "top_source_classes="+quoteHandoffValue(strings.Join(boundedStringSlice(classes, 6), ",")))
		}
	}
	return parts
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

func quoteHandoffValue(value string) string {
	value = truncateExtractorPromptText(strings.TrimSpace(value), 180)
	if value == "" {
		return "\"\""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	return "`" + value + "`"
}
