package types

import (
	"sort"
	"strings"
)

const (
	defaultToolRefinementRequiredFieldLimit    = 8
	defaultToolRefinementCandidateValueLimit   = 4
	defaultToolRefinementSourceClassValueLimit = 6
)

// ToolRefinementPromptFieldOptions controls the presentation-only projection of
// ToolRefinementHint. It centralizes retry/finalizer prompt rendering so adding
// a new typed refinement field cannot silently reach one stage but not another.
type ToolRefinementPromptFieldOptions struct {
	HistoricalProducerLabels bool
	QuoteValues              bool
	FlagField                string
	ActionField              string
	RequiredFieldLimit       int
	CandidateValueLimit      int
	SourceClassValueLimit    int
}

func (o ToolRefinementPromptFieldOptions) flagField() string {
	if field := strings.TrimSpace(o.FlagField); field != "" {
		return field
	}
	return "refine_flags"
}

func (o ToolRefinementPromptFieldOptions) actionField() string {
	if field := strings.TrimSpace(o.ActionField); field != "" {
		return field
	}
	return "refine_action"
}

func (o ToolRefinementPromptFieldOptions) requiredFieldLimit() int {
	if o.RequiredFieldLimit > 0 {
		return o.RequiredFieldLimit
	}
	return defaultToolRefinementRequiredFieldLimit
}

func (o ToolRefinementPromptFieldOptions) candidateValueLimit() int {
	if o.CandidateValueLimit > 0 {
		return o.CandidateValueLimit
	}
	return defaultToolRefinementCandidateValueLimit
}

func (o ToolRefinementPromptFieldOptions) sourceClassValueLimit() int {
	if o.SourceClassValueLimit > 0 {
		return o.SourceClassValueLimit
	}
	return defaultToolRefinementSourceClassValueLimit
}

// ToolRefinementPromptFields returns a compact, stable list of key=value fields
// for model-facing soft guidance. It consumes only ToolRefinementHint typed
// fields and does not inspect user wording, model prose, localized status, or
// rendered tool summaries.
func ToolRefinementPromptFields(refinement *ToolRefinementHint, opts ToolRefinementPromptFieldOptions) []string {
	if refinement == nil {
		return nil
	}
	hint := NormalizeToolRefinementHint(*refinement)
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
		parts = append(parts, toolRefinementPromptKV(opts.flagField(), strings.Join(flags, ","), opts.QuoteValues))
	}
	if hint.UniverseExcludedReason != "" {
		parts = append(parts, toolRefinementPromptKV("excluded_reason", hint.UniverseExcludedReason, opts.QuoteValues))
	}
	if hint.ResultTruncated || hint.CandidateBudgetTruncated || hint.PreferredNextTool != "" || len(hint.PreferredParams) > 0 {
		parts = append(parts, toolRefinementPromptKV(opts.actionField(), "soft_narrow_if_answer_critical_else_caveat", opts.QuoteValues))
	}
	if hint.PreferredNextTool != "" {
		field := "preferred_tool"
		if opts.HistoricalProducerLabels {
			field = "prior_stage_preferred_tool"
		}
		parts = append(parts, toolRefinementPromptKV(field, hint.PreferredNextTool, opts.QuoteValues))
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
		parts = append(parts, toolRefinementPromptKV(field, strings.Join(values, ","), opts.QuoteValues))
	}
	if len(hint.RequiredFields) > 0 {
		field := "required_fields"
		if opts.HistoricalProducerLabels {
			field = "prior_stage_required_fields"
		}
		parts = append(parts, toolRefinementPromptKV(field, strings.Join(boundedToolRefinementPromptStrings(hint.RequiredFields, opts.requiredFieldLimit()), ","), opts.QuoteValues))
	}
	if hint.NextCursor != "" {
		field := "next_cursor"
		if opts.HistoricalProducerLabels {
			field = "prior_stage_next_cursor"
		}
		parts = append(parts, toolRefinementPromptKV(field, hint.NextCursor, opts.QuoteValues))
	}
	if len(hint.SkippedLargeCandidates) > 0 {
		parts = append(parts, toolRefinementPromptKV("skipped_large", strings.Join(boundedToolRefinementPromptStrings(hint.SkippedLargeCandidates, opts.candidateValueLimit()), ","), opts.QuoteValues))
	}
	if len(hint.ExcludedRoots) > 0 {
		parts = append(parts, toolRefinementPromptKV("excluded_roots", strings.Join(boundedToolRefinementPromptStrings(hint.ExcludedRoots, opts.candidateValueLimit()), ","), opts.QuoteValues))
	}
	if len(hint.TopSourceClasses) > 0 {
		classes := make([]string, 0, len(hint.TopSourceClasses))
		for _, role := range hint.TopSourceClasses {
			if role != "" {
				classes = append(classes, string(role))
			}
		}
		if len(classes) > 0 {
			parts = append(parts, toolRefinementPromptKV("top_source_classes", strings.Join(boundedToolRefinementPromptStrings(classes, opts.sourceClassValueLimit()), ","), opts.QuoteValues))
		}
	}
	return parts
}

func toolRefinementPromptKV(field, value string, quote bool) string {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	if field == "" {
		return ""
	}
	if quote {
		return field + "=" + quoteToolRefinementPromptValue(value)
	}
	return field + "=" + value
}

func quoteToolRefinementPromptValue(value string) string {
	value = trimToolHandoffText(value)
	if value == "" {
		return "\"\""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	return "`" + value + "`"
}

func boundedToolRefinementPromptStrings(values []string, limit int) []string {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}
