package types

import (
	"strconv"
	"strings"
)

// enumeration_boundary.go — typed-only surface for the user-declared
// enumeration boundary. v3.1 (2026-05-05) removed the regex-driven
// fallback recovery path:
//
// Pre-v3.1, when the analyzer LLM omitted enumeration_boundary, a
// regex chain (4 hardcoded counter-word patterns: "first/top N X",
// "前 N 个/项/...", "[1-9]+ 个/项/条/步/层/类/种/处/段 X", and English
// "[1-9]+ X") attempted to extract a count from the raw question.
// That chain was a keyword-table band-aid — it covered only EN/ZH
// counter words from the original eval set and missed:
//   - Chinese counters not in the 8-word list (道/关/类/...)
//   - Japanese (個/枚) / Korean (개) / other languages
//   - Mixed-language questions
//   - English "first" / "top" leading patterns missing the prefix
//
// Per feedback_no_custom_keyword_matching.md red line ("LLM 分类错就是
// prompt 歧义,不是加关键词表绕过"), the regex fallback was deleted in
// v3.1. EnumerationBoundary now sources only from emit_analysis output:
// the LLM either correctly identifies the explicit count or the
// pipeline routes through QFGeneric / QFArchitecture / etc. with an
// honest "may have missed an item" caveat. honest weakness >
// dishonest confidence.
//
// What survived the cleanup:
//   - RequestedEnumerationBoundary struct (typed payload)
//   - NormalizeRequestedEnumerationBoundary (typed validation:
//     verifies LLM-emitted SourceQuote is verbatim in RawRequest and
//     carries the declared decimal count; no regex, no keywords)
//   - RequestedEnumerationBoundaryOwner (entity-based check via
//     MentionedEntitiesFromRawRequest; used by step backbone
//     enrichment in answer_surface_plan.go and the boundary-scope
//     reconciler)
//   - EnumerationBoundaryCountString (typed integer → string)

// RequestedEnumerationBoundary captures a user-declared set boundary
// that should survive downstream synthesis. Typical examples are
// questions like "the 7 checks", "the first 3 handlers", or "top 5
// stages". The LLM emits the boundary at analyze time; the system
// validates SourceQuote against the current RawRequest before any
// downstream stage consumes it. The same quote must carry the explicit
// decimal count; "all/every" style requests without a number are
// modeled by CompletenessObligation instead.
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
	if !requestedEnumerationQuoteCarriesDeclaredCount(quote, in.DeclaredCount) {
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

func requestedEnumerationQuoteCarriesDeclaredCount(quote string, declared int) bool {
	if declared <= 0 {
		return false
	}
	want := strconv.Itoa(declared)
	for i := 0; i < len(quote); {
		if quote[i] < '0' || quote[i] > '9' {
			i++
			continue
		}
		start := i
		for i < len(quote) && quote[i] >= '0' && quote[i] <= '9' {
			i++
		}
		digits := quote[start:i]
		if digits == want {
			return true
		}
		trimmed := strings.TrimLeft(digits, "0")
		if trimmed == "" {
			trimmed = "0"
		}
		if trimmed == want {
			return true
		}
	}
	return false
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
//
// Owner detection is purely entity-based — it consults
// AnalyzerHints.{ExactTargets, MentionedEntities, PrimaryEntities}
// (all already verified verbatim against RawRequest by upstream
// helpers) and never inspects raw text via regex. Used downstream by
// step-backbone enrichment (answer_surface_plan.go) and the
// boundary-scope reconciler.
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
