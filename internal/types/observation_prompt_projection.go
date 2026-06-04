package types

import (
	"fmt"
	"strings"
)

// ObservationPromptProjectionOptions configures the shared compact observation
// view consumed by finalizer and semantic-review prompts. It is deliberately a
// rendering projection over accepted ObservationRecord rows; it does not
// classify facts or inspect user/model prose.
type ObservationPromptProjectionOptions struct {
	Limit int

	SourceMaxLen  int
	ValueMaxLen   int
	SummaryMaxLen int
	NoteMaxLen    int

	ExcerptMaxLen          int
	PrincipalExcerptMaxLen int

	NoteLimit                             int
	PrincipalNoteLimit                    int
	StrongCurrentSourceNoteLimit          int
	OriginSpecificPrincipalNoteLimit      int
	IncludeCurrentSourceExcerpt           bool
	IncludeSummaryInNotesWhenOnlyNoteData bool
}

// ObservationPromptRecord is the shared prompt-facing projection of one
// ObservationRecord. It keeps source/origin/span boundaries explicit and keeps
// rich notes separate from Summary so prompt consumers do not duplicate or
// silently drop explorer-authored explanations.
type ObservationPromptRecord struct {
	ID              string
	Origin          AnswerEvidenceOrigin
	Producer        string
	Role            AnswerAggregateRole
	GroundingPolicy ClaimGroundingPolicy
	ProvenanceLane  ObservationProvenanceLane
	Source          string
	Span            string
	Claim           string
	Value           string
	Summary         string
	Excerpt         string
	Notes           []string
	Negative        bool
	ResultCount     *int
	SupportRefCount int
}

func DefaultObservationPromptProjectionOptions(limit int) ObservationPromptProjectionOptions {
	return ObservationPromptProjectionOptions{
		Limit:                            limit,
		SourceMaxLen:                     90,
		ValueMaxLen:                      120,
		SummaryMaxLen:                    180,
		NoteMaxLen:                       160,
		ExcerptMaxLen:                    180,
		PrincipalExcerptMaxLen:           260,
		NoteLimit:                        2,
		PrincipalNoteLimit:               4,
		StrongCurrentSourceNoteLimit:     3,
		OriginSpecificPrincipalNoteLimit: 4,
	}
}

func SemanticReviewObservationPromptProjectionOptions(limit int) ObservationPromptProjectionOptions {
	opts := DefaultObservationPromptProjectionOptions(limit)
	opts.ExcerptMaxLen = 120
	opts.PrincipalExcerptMaxLen = 180
	opts.PrincipalNoteLimit = 3
	opts.OriginSpecificPrincipalNoteLimit = 4
	return opts
}

// ProjectObservationPromptRecords returns the single compact ledger projection
// that finalizer/reviewer prompts should render. Ranking is delegated to
// PrioritizeObservationRecords so mixed-origin questions keep the same typed
// relevance order across consumers.
func ProjectObservationPromptRecords(records []ObservationRecord, rm *RequestModel, contract *AnswerContract, opts ObservationPromptProjectionOptions) []ObservationPromptRecord {
	if len(records) == 0 || opts.Limit <= 0 {
		return nil
	}
	opts = normalizeObservationPromptProjectionOptions(opts)
	prioritized := PrioritizeObservationRecords(records, rm, contract, opts.Limit)
	out := make([]ObservationPromptRecord, 0, len(prioritized))
	for _, record := range prioritized {
		summary := clampObservationPromptText(record.Summary, opts.SummaryMaxLen)
		value := clampObservationPromptText(record.Value, opts.ValueMaxLen)
		out = append(out, ObservationPromptRecord{
			ID:              strings.TrimSpace(record.ID),
			Origin:          record.Origin,
			Producer:        strings.TrimSpace(record.Producer),
			Role:            record.Role,
			GroundingPolicy: record.GroundingPolicy,
			ProvenanceLane:  record.ProvenanceLane,
			Source:          FormatObservationSourceRef(record.SourceRef, opts.SourceMaxLen),
			Span:            FormatObservationSpan(record.Span, 80),
			Claim:           observationPromptClaim(record.ClaimKey, summary, value),
			Value:           value,
			Summary:         summary,
			Excerpt:         observationPromptExcerpt(record, opts),
			Notes:           observationPromptNotes(record, opts),
			Negative:        record.Negative,
			ResultCount:     cloneObservationPromptResultCount(record.ResultCount),
			SupportRefCount: len(record.SupportRefs),
		})
	}
	return out
}

func observationPromptClaim(raw, summary, value string) string {
	claim := strings.TrimSpace(raw)
	key := observationPromptTextKey(claim)
	if key == "" {
		return ""
	}
	if key == observationPromptTextKey(summary) || key == observationPromptTextKey(value) {
		return ""
	}
	return claim
}

func normalizeObservationPromptProjectionOptions(opts ObservationPromptProjectionOptions) ObservationPromptProjectionOptions {
	defaults := DefaultObservationPromptProjectionOptions(opts.Limit)
	if opts.SourceMaxLen <= 0 {
		opts.SourceMaxLen = defaults.SourceMaxLen
	}
	if opts.ValueMaxLen <= 0 {
		opts.ValueMaxLen = defaults.ValueMaxLen
	}
	if opts.SummaryMaxLen <= 0 {
		opts.SummaryMaxLen = defaults.SummaryMaxLen
	}
	if opts.NoteMaxLen <= 0 {
		opts.NoteMaxLen = defaults.NoteMaxLen
	}
	if opts.ExcerptMaxLen <= 0 {
		opts.ExcerptMaxLen = defaults.ExcerptMaxLen
	}
	if opts.PrincipalExcerptMaxLen <= 0 {
		opts.PrincipalExcerptMaxLen = defaults.PrincipalExcerptMaxLen
	}
	if opts.NoteLimit <= 0 {
		opts.NoteLimit = defaults.NoteLimit
	}
	if opts.PrincipalNoteLimit <= 0 {
		opts.PrincipalNoteLimit = defaults.PrincipalNoteLimit
	}
	if opts.StrongCurrentSourceNoteLimit <= 0 {
		opts.StrongCurrentSourceNoteLimit = defaults.StrongCurrentSourceNoteLimit
	}
	if opts.OriginSpecificPrincipalNoteLimit <= 0 {
		opts.OriginSpecificPrincipalNoteLimit = defaults.OriginSpecificPrincipalNoteLimit
	}
	return opts
}

// FormatObservationSpan renders an origin-local span without implying that
// artifact, VCS, JSON, or trace coordinates are current-source citations.
func FormatObservationSpan(span ObservationSpan, maxValueLen int) string {
	parts := make([]string, 0, 8)
	if span.LineStart > 0 {
		if span.LineEnd > 0 && span.LineEnd != span.LineStart {
			parts = append(parts, fmt.Sprintf("lines %d-%d", span.LineStart, span.LineEnd))
		} else {
			parts = append(parts, fmt.Sprintf("line %d", span.LineStart))
		}
	}
	if span.OldLine > 0 {
		parts = append(parts, fmt.Sprintf("old_line %d", span.OldLine))
	}
	if span.NewLine > 0 {
		parts = append(parts, fmt.Sprintf("new_line %d", span.NewLine))
	}
	if span.HunkHeader != "" {
		parts = append(parts, fmt.Sprintf("hunk %q", clampObservationPromptText(span.HunkHeader, maxValueLen)))
	}
	if span.Row > 0 {
		parts = append(parts, fmt.Sprintf("row %d", span.Row))
	}
	if span.StartTs != 0 || span.EndTs != 0 {
		parts = append(parts, fmt.Sprintf("%.6f-%.6fs", span.StartTs, span.EndTs))
	}
	if span.StartTsMs != 0 || span.EndTsMs != 0 {
		parts = append(parts, fmt.Sprintf("%.3f-%.3fms", span.StartTsMs, span.EndTsMs))
	}
	if span.Selector != "" {
		parts = append(parts, fmt.Sprintf("selector %q", clampObservationPromptText(span.Selector, maxValueLen)))
	}
	if span.JSONPointer != "" {
		parts = append(parts, fmt.Sprintf("json %q", clampObservationPromptText(span.JSONPointer, maxValueLen)))
	}
	if span.Paragraph > 0 {
		parts = append(parts, fmt.Sprintf("paragraph %d", span.Paragraph))
	}
	if span.TextStart > 0 || span.TextEnd > 0 {
		parts = append(parts, fmt.Sprintf("text %d-%d", span.TextStart, span.TextEnd))
	}
	return strings.Join(parts, ", ")
}

func observationPromptExcerpt(record ObservationRecord, opts ObservationPromptProjectionOptions) string {
	if record.Origin == AnswerEvidenceOriginCurrentSource && !opts.IncludeCurrentSourceExcerpt {
		return ""
	}
	excerpt := strings.Join(strings.Fields(strings.TrimSpace(record.RawExcerpt)), " ")
	if excerpt == "" || observationPromptTextKey(excerpt) == observationPromptTextKey(record.Summary) {
		return ""
	}
	limit := opts.ExcerptMaxLen
	if NormalizeAnswerAggregateRole(record.Role).IsPrincipal() ||
		AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
		limit = opts.PrincipalExcerptMaxLen
	}
	return clampObservationPromptText(excerpt, limit)
}

func observationPromptNotes(record ObservationRecord, opts ObservationPromptProjectionOptions) []string {
	limit := observationPromptNoteLimit(record, opts)
	if limit <= 0 || len(record.RichNotes) == 0 {
		return nil
	}
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	summaryKey := observationPromptTextKey(record.Summary)
	for _, note := range record.RichNotes {
		note = clampObservationPromptText(note, opts.NoteMaxLen)
		key := observationPromptTextKey(note)
		if key == "" {
			continue
		}
		if key == summaryKey && !(opts.IncludeSummaryInNotesWhenOnlyNoteData && len(record.RichNotes) == 1) {
			continue
		}
		if observationPromptBareTokenCoveredBySummaryOrClaim(note, record) {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, note)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func observationPromptBareTokenCoveredBySummaryOrClaim(note string, record ObservationRecord) bool {
	note = strings.TrimSpace(note)
	if note == "" || strings.ContainsAny(note, " \t\r\n，。；：、,.!?;:()[]{}<>/\\|") {
		return false
	}
	haystack := " " + record.Summary + " " + record.ClaimKey + " " + record.Subject + " " + record.Object + " "
	return strings.Contains(haystack, note)
}

func observationPromptNoteLimit(record ObservationRecord, opts ObservationPromptProjectionOptions) int {
	limit := opts.NoteLimit
	if NormalizeAnswerAggregateRole(record.Role).IsPrincipal() {
		limit = opts.PrincipalNoteLimit
	}
	if record.Origin == AnswerEvidenceOriginCurrentSource &&
		ObservationRecordHasStrongCurrentSourceAnchor(record) &&
		limit < opts.StrongCurrentSourceNoteLimit {
		limit = opts.StrongCurrentSourceNoteLimit
	}
	if AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) &&
		NormalizeAnswerAggregateRole(record.Role).IsPrincipal() &&
		limit < opts.OriginSpecificPrincipalNoteLimit {
		limit = opts.OriginSpecificPrincipalNoteLimit
	}
	return limit
}

func cloneObservationPromptResultCount(n *int) *int {
	if n == nil {
		return nil
	}
	v := *n
	return &v
}

func clampObservationPromptText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}

func observationPromptTextKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}
