package types

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// sourceInventoryPrincipalRowAcceptsStructuredLabel recognizes only aliases
// carried by the exact typed declaration row: its canonical member, an exact
// parser/lens SurfaceTerm, or the exact base of a structured parenthesized
// decoration. This lets `Bridge` and `public class Bridge` resolve to the same
// declaration without introducing a prose/similarity alias lane.
func sourceInventoryPrincipalRowAcceptsStructuredLabel(row SourceInventoryRow, label string) bool {
	label = strings.TrimSpace(label)
	canonical := sourceInventoryPrincipalRowMemberLabel(row)
	if canonical == "" {
		return false
	}
	if aggregateSupportRefCanDescribeMember(label, canonical) {
		return true
	}
	if base, _, ok := AnswerAggregateDecoratedLabelParts(label); ok &&
		strings.EqualFold(strings.TrimSpace(base), canonical) {
		return true
	}
	for _, term := range row.Member.SurfaceTerms {
		if strings.EqualFold(strings.TrimSpace(term), label) &&
			sourceInventoryTypedSurfaceTermNamesMember(term, canonical) {
			return true
		}
	}
	return false
}

func sourceInventoryTypedSurfaceTermNamesMember(term, member string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	member = strings.ToLower(strings.TrimSpace(member))
	if term == "" || member == "" || !strings.HasSuffix(term, member) {
		return false
	}
	prefix := strings.TrimSuffix(term, member)
	if prefix == "" {
		return true
	}
	last, _ := utf8.DecodeLastRuneInString(prefix)
	return !unicode.IsLetter(last) && !unicode.IsDigit(last) && last != '_' && last != '$'
}
