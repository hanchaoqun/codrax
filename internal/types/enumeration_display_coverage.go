package types

import (
	"fmt"
	"strings"
)

// EnumerationDisplayCoverage is the shared typed authority for whether an
// accepted principal enumeration row-set is already visible in the answer and
// grounded by row-compatible citations. It is derived from structured answer
// blocks/items, citation_ref indexes, and EnumerationDisplayRow source fields;
// it does not inspect user text, model rationale, localized renderer prose, or
// raw tool summaries.
type EnumerationDisplayCoverage struct {
	SetCount    int
	RowCount    int
	CoveredRows int
	MissingRows []EnumerationDisplayRow
}

// EnumerationDisplayCoverageView is the shared projection for accepted
// principal enumeration rows against the current answer document. It lets
// final-answer repair, source-inventory pre-emit authority, and caveat
// materialization consume one row/count visibility view instead of each
// rebuilding coverage from local partial state.
type EnumerationDisplayCoverageView struct {
	Sets     []EnumerationDisplaySet
	Coverage EnumerationDisplayCoverage
}

// Complete reports whether every row in the accepted row-set surface is
// already represented by a visible principal item with a compatible citation.
func (c EnumerationDisplayCoverage) Complete() bool {
	return c.RowCount > 0 && c.CoveredRows == c.RowCount && len(c.MissingRows) == 0
}

// Complete reports whether the current answer document covers every accepted
// row in the view.
func (v EnumerationDisplayCoverageView) Complete() bool {
	return v.Coverage.Complete()
}

// RowsFullyCited reports whether every accepted row is backed by a concrete
// row-local source citation. This is a typed row property, not a check over
// model prose or rendered output.
func (v EnumerationDisplayCoverageView) RowsFullyCited() bool {
	if len(v.Sets) == 0 {
		return false
	}
	for _, set := range v.Sets {
		if len(set.Rows) == 0 {
			return false
		}
		for _, row := range set.Rows {
			if !row.HasCitation || strings.TrimSpace(row.Source) == "" || row.LineStart <= 0 {
				return false
			}
		}
	}
	return true
}

// AnswerDocumentAcceptedEnumerationDisplayCoverage compiles accepted principal
// enumeration row sets from the bus context and evaluates their visible,
// citation-compatible coverage in doc. If rm is nil, the AnalysisIR request
// model from ctx is used. This is the canonical row/list/count coverage
// projector; hard paths should consume this view instead of rebuilding
// source-inventory-specific coverage locally.
func AnswerDocumentAcceptedEnumerationDisplayCoverage(ctx *BusContext, rm *RequestModel, doc *AnswerDocumentV2) EnumerationDisplayCoverageView {
	if ctx == nil {
		return EnumerationDisplayCoverageView{}
	}
	if rm == nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	plan := BuildAnswerSurfacePlanForBusContext(ctx)
	sets := CompileEnumerationDisplaySets(rm, plan)
	return EnumerationDisplayCoverageView{
		Sets:     sets,
		Coverage: AnswerDocumentEnumerationDisplayCoverage(doc, sets),
	}
}

// AnswerDocumentEnumerationDisplayCoverage computes row-level visible coverage
// for accepted principal enumeration display sets. Callers in final-answer
// repair and caveat materialization should consume this single projection
// instead of recomputing row obligations from partial local state.
func AnswerDocumentEnumerationDisplayCoverage(doc *AnswerDocumentV2, sets []EnumerationDisplaySet) EnumerationDisplayCoverage {
	var out EnumerationDisplayCoverage
	if doc == nil || len(sets) == 0 {
		return out
	}
	out.SetCount = len(sets)
	for _, set := range sets {
		for _, row := range set.Rows {
			out.RowCount++
			if AnswerDocumentCoversEnumerationDisplayRow(doc, row) {
				out.CoveredRows++
				continue
			}
			out.MissingRows = append(out.MissingRows, row)
		}
	}
	return out
}

// AnswerDocumentCoversEnumerationDisplaySets reports whether all rows in all
// accepted display sets are already visible and cited in the principal answer.
func AnswerDocumentCoversEnumerationDisplaySets(doc *AnswerDocumentV2, sets []EnumerationDisplaySet) bool {
	return AnswerDocumentEnumerationDisplayCoverage(doc, sets).Complete()
}

// AnswerDocumentCoversEnumerationDisplaySet reports whether one accepted
// display set is already visible and cited in the principal answer.
func AnswerDocumentCoversEnumerationDisplaySet(doc *AnswerDocumentV2, set EnumerationDisplaySet) bool {
	return AnswerDocumentCoversEnumerationDisplaySets(doc, []EnumerationDisplaySet{set})
}

// AnswerDocumentCoversEnumerationDisplayRow reports whether one accepted
// enumeration row is represented by a structured principal item with a
// compatible citation. Source rows without a usable typed source anchor are not
// treated as covered here; origin-specific prose-only fallbacks remain the
// responsibility of their specialized runtime answer policies.
func AnswerDocumentCoversEnumerationDisplayRow(doc *AnswerDocumentV2, row EnumerationDisplayRow) bool {
	ob, ok := enumerationDisplayRowSupportMemberObligation(row)
	if !ok {
		return false
	}
	return AnswerDocumentCoversSupportMember(doc, ob)
}

func enumerationDisplayRowSupportMemberObligation(row EnumerationDisplayRow) (AnswerSupportMemberObligation, bool) {
	location, source, start, end, ok := enumerationDisplayRowSourceLocation(row)
	if !ok {
		return AnswerSupportMemberObligation{}, false
	}
	label := firstNonEmptyEnumerationCoverageString(row.DisplayLabel, row.Member, row.AnchorSymbol, row.Subject, row.Object)
	if label == "" {
		return AnswerSupportMemberObligation{}, false
	}
	terms := enumerationDisplayRowSurfaceTerms(row)
	return AnswerSupportMemberObligation{
		EvidenceID:          strings.TrimSpace(row.EvidenceID),
		Location:            location,
		Label:               label,
		ClaimForm:           row.ClaimForm,
		SurfaceTerms:        terms,
		Source:              source,
		LineStart:           start,
		LineEnd:             end,
		EquivalentLocations: cloneEnumerationCoverageStringSlice(row.EquivalentLocations),
	}, true
}

func enumerationDisplayRowSourceLocation(row EnumerationDisplayRow) (location, source string, start, end int, ok bool) {
	source = strings.TrimSpace(row.Source)
	start = row.LineStart
	end = row.LineEnd
	if source != "" && start > 0 {
		return fmt.Sprintf("%s:%d", source, start), source, start, end, true
	}
	for _, raw := range []string{row.Location, row.CitationKey, row.Member, row.DisplayLabel} {
		if loc, parsed := ParseAnswerSourceLocationSurface(raw); parsed && strings.TrimSpace(loc.File) != "" && loc.LineStart > 0 {
			return fmt.Sprintf("%s:%d", loc.File, loc.LineStart), loc.File, loc.LineStart, loc.LineEnd, true
		}
		if _, loc, parsed := ParseAnswerSupportRefMemberLocation(raw); parsed &&
			strings.TrimSpace(loc.File) != "" && loc.LineStart > 0 {
			return fmt.Sprintf("%s:%d", loc.File, loc.LineStart), loc.File, loc.LineStart, loc.LineEnd, true
		}
	}
	return "", "", 0, 0, false
}

func enumerationDisplayRowSurfaceTerms(row EnumerationDisplayRow) []string {
	seen := map[string]bool{}
	var out []string
	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		key := strings.ToLower(term)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, term)
	}
	add(row.DisplayLabel)
	add(row.Member)
	add(row.AnchorSymbol)
	add(row.Subject)
	add(row.Object)
	add(row.OwnerSymbol)
	for _, term := range row.SurfaceTerms {
		add(term)
	}
	for _, candidate := range []string{row.DisplayLabel, row.Member} {
		left, right, ok := AnswerAggregateMemberRelationParts(candidate)
		if !ok {
			continue
		}
		add(left)
		add(right)
	}
	return out
}

func firstNonEmptyEnumerationCoverageString(values ...string) string {
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			return s
		}
	}
	return ""
}

func cloneEnumerationCoverageStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
