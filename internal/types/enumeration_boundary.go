package types

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reEnglishLeadingEnumerationBoundary = regexp.MustCompile(`(?i)\b(?:the\s+)?(?:first|top)\s+([1-9]\d{0,2})\s+([A-Za-z][A-Za-z0-9_-]*(?:\s+[A-Za-z][A-Za-z0-9_-]*){0,3})\b`)
	reEnglishCountEnumerationBoundary   = regexp.MustCompile(`(?i)\b([1-9]\d{0,2})\s+([A-Za-z][A-Za-z0-9_-]*(?:\s+[A-Za-z][A-Za-z0-9_-]*){0,3})\b`)
	reChineseLeadingEnumerationBoundary = regexp.MustCompile(`前\s*([1-9]\d{0,2})\s*(?:个|项|条|步|层|类|种|处|段)?\s*([\p{Han}A-Za-z_]{0,12})`)
	reChineseCountEnumerationBoundary   = regexp.MustCompile(`([1-9]\d{0,2})\s*(个|项|条|步|层|类|种|处|段)\s*([\p{Han}A-Za-z_]{0,12})`)
)

// RequestedEnumerationBoundary captures a user-declared set boundary
// that should survive downstream synthesis. Typical examples are
// questions like "the 7 checks", "the first 3 handlers", or "top 5
// stages". The LLM recommends the boundary at analyze time, and the
// system validates it against the current RawRequest before any
// downstream stage consumes it.
//
// The boundary is intentionally narrow:
//   - it is request-grounded (SourceQuote must come from RawRequest)
//   - it carries only the declared count and the grounding quote
//   - it says nothing about WHICH items belong to the set; downstream
//     stages still need grounded code evidence for that
type RequestedEnumerationBoundary struct {
	DeclaredCount int    `json:"declared_count"`
	SourceQuote   string `json:"source_quote"`
}

// NormalizeRequestedEnumerationBoundary validates the LLM-recommended
// boundary against the current request text and returns the canonical
// stored form. Nil means "do not trust / do not use this boundary".
func NormalizeRequestedEnumerationBoundary(raw string, in *RequestedEnumerationBoundary) *RequestedEnumerationBoundary {
	if in == nil || in.DeclaredCount <= 0 {
		return nil
	}
	quote := strings.TrimSpace(in.SourceQuote)
	if quote == "" {
		return nil
	}
	if !requestedEnumerationQuotePresent(raw, quote) {
		return nil
	}
	return &RequestedEnumerationBoundary{
		DeclaredCount: in.DeclaredCount,
		SourceQuote:   quote,
	}
}

func requestedEnumerationQuotePresent(raw, quote string) bool {
	raw = strings.TrimSpace(raw)
	quote = strings.TrimSpace(quote)
	if raw == "" || quote == "" {
		return false
	}
	if strings.Contains(raw, quote) {
		return true
	}
	foldRaw := foldEnumerationBoundarySurface(raw)
	foldQuote := foldEnumerationBoundarySurface(quote)
	if foldRaw == "" || foldQuote == "" {
		return false
	}
	return strings.Contains(foldRaw, foldQuote)
}

func foldEnumerationBoundarySurface(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// EnumerationBoundaryCountString returns the decimal count string used
// by prompt renderers and tests. Empty when boundary is nil/invalid.
func EnumerationBoundaryCountString(b *RequestedEnumerationBoundary) string {
	if b == nil || b.DeclaredCount <= 0 {
		return ""
	}
	return strconv.Itoa(b.DeclaredCount)
}

// RequestedEnumerationBoundaryOwner returns the single request-mentioned
// owner entity for a bounded-set question. Empty means the request
// either did not name a single owner or stayed ambiguous.
func RequestedEnumerationBoundaryOwner(rm RequestModel) string {
	if len(rm.AnalyzerHints.ExactTargets) > 0 {
		if mentioned := MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.ExactTargets); len(mentioned) == 1 {
			return mentioned[0]
		}
	}
	if len(rm.AnalyzerHints.MentionedEntities) == 1 {
		return rm.AnalyzerHints.MentionedEntities[0]
	}
	if recovered := MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.PrimaryEntities); len(recovered) == 1 {
		return recovered[0]
	}
	return ""
}

// RecoverRequestedEnumerationBoundary is the deterministic fallback
// for bounded principal-set questions when the analyzer LLM omitted
// enumeration_boundary. It only fires for single-owner, non-count,
// non-cross-component mechanism/enumeration-style questions, so the
// recovery stays request-structural rather than repository-specific.
func RecoverRequestedEnumerationBoundary(rm RequestModel) *RequestedEnumerationBoundary {
	if rm.EnumerationBoundary != nil {
		return NormalizeRequestedEnumerationBoundary(rm.RawRequest, rm.EnumerationBoundary)
	}
	if strings.TrimSpace(rm.RawRequest) == "" ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCrossComponent ||
		rm.Predicates.IsRelationalLookup {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(rm.AnalyzerHints.Kind)) {
	case "mechanism", "enumeration", "call_chain", "conditional":
	default:
		return nil
	}
	if RequestedEnumerationBoundaryOwner(rm) == "" {
		return nil
	}
	return detectRequestedEnumerationBoundaryFromRaw(rm.RawRequest)
}

func detectRequestedEnumerationBoundaryFromRaw(raw string) *RequestedEnumerationBoundary {
	type candidate struct {
		count int
		quote string
		score int
	}
	var best candidate
	consider := func(count int, quote string, score int) {
		boundary := NormalizeRequestedEnumerationBoundary(raw, &RequestedEnumerationBoundary{
			DeclaredCount: count,
			SourceQuote:   strings.TrimSpace(quote),
		})
		if boundary == nil {
			return
		}
		if score > best.score || (score == best.score && len(boundary.SourceQuote) > len(best.quote)) {
			best = candidate{count: boundary.DeclaredCount, quote: boundary.SourceQuote, score: score}
		}
	}
	for _, match := range reEnglishLeadingEnumerationBoundary.FindAllStringSubmatch(raw, -1) {
		count, _ := strconv.Atoi(match[1])
		consider(count, match[0], 4)
	}
	for _, match := range reChineseLeadingEnumerationBoundary.FindAllStringSubmatch(raw, -1) {
		count, _ := strconv.Atoi(match[1])
		consider(count, strings.TrimSpace(match[0]), 4)
	}
	for _, match := range reChineseCountEnumerationBoundary.FindAllStringSubmatch(raw, -1) {
		count, _ := strconv.Atoi(match[1])
		consider(count, strings.TrimSpace(match[0]), 3)
	}
	for _, match := range reEnglishCountEnumerationBoundary.FindAllStringSubmatch(raw, -1) {
		count, _ := strconv.Atoi(match[1])
		consider(count, match[0], 2)
	}
	if best.count <= 0 || strings.TrimSpace(best.quote) == "" {
		return nil
	}
	return &RequestedEnumerationBoundary{
		DeclaredCount: best.count,
		SourceQuote:   best.quote,
	}
}
