package types

import (
	"sort"
	"strings"
)

// question_structure.go — typed model for the structural obligations a
// user's question carries that downstream stages must enforce.
//
// Pre-2026-05-02, the only modeled obligation was
// RequestedEnumerationBoundary (count: "9 of X", "the 7 checks").
// Plan E (2026-05-02) generalises the surface so analyzer-emitted
// question-side contracts can also encode:
//
//   - Completeness ("all"/"every"/"complete list" — the answer
//     must be exhaustive, not lower_bound)
//   - Bucket partition ("X for A, Y for B" / "分别...A 和 B" — the
//     answer must carry per-bucket structure with verbatim labels)
//
// Each axis is optional; a zero-value model means "no obligation
// declared", which preserves byte-identical pre-Plan-E behaviour.
//
// Why three independent fields on RequestModel rather than one nested
// struct: every existing consumer reads rm.EnumerationBoundary
// directly (30+ call sites). Wrapping it in a sub-struct would break
// those callers without value beyond aesthetics. The unified accessor
// QuestionStructure() on RequestModel below offers a synthetic view
// for code that wants to ask "does this question carry any structural
// obligation".

// CompletenessObligation captures the analyzer's finding that the user
// asked for an EXHAUSTIVE answer. The trigger is a verbatim phrase
// from the question that signals universal coverage ("all the X" /
// "every X" / "complete list" / "exhaustive").
//
// Required=true downstream:
//   - emit_investigation_complete: refuse result_kind=resolved when
//     grep-discovered candidates exceed read_file'd files (Turn A
//     stopped before all candidates verified)
//   - emit_answer_symbol: reject completeness=lower_bound (the LLM
//     declared the slate is partial — but the question asks for ALL,
//     so partial is non-honest); completeness=unknown is the honest
//     escape when the investigation legitimately could not determine.
//
// SourceQuote must verbatim match RawRequest (validated like
// RequestedEnumerationBoundary), so the LLM cannot fabricate an
// obligation by inventing a quote.
//
// KnownCandidateCount is populated by the system (NOT by the LLM)
// when grep / repo_map / list_files surfaces a count of candidate
// matches. Tracked on Mutable.QuestionObligationCounts so multiple
// passes can refine it; the analyzer leaves it zero.
type CompletenessObligation struct {
	Required    bool   `json:"required"`
	SourceQuote string `json:"source_quote,omitempty"`

	// G1's coverage gate (computePreCompleteCoverageGap in
	// emit_investigation_complete.go) reads ScannedSet vs ReadSet
	// directly off the closure rather than threading a count through
	// this struct. If a future gate needs an explicit count
	// (e.g. "n items expected, m emitted" drift), populate from a
	// closure-side derivation at gate time, not at analyze time —
	// the analyzer doesn't know the count yet.
}

// IsActive reports whether the obligation is meaningfully populated —
// Required=true AND a verbatim source phrase. Zero-value short-
// circuits all gate checks.
func (c *CompletenessObligation) IsActive() bool {
	if c == nil {
		return false
	}
	return c.Required && strings.TrimSpace(c.SourceQuote) != ""
}

// NormalizeCompletenessObligation validates a raw LLM-emitted
// obligation against the current RawRequest. Returns nil when the
// LLM left the field empty OR when the source quote is not present
// in the raw request (mirrors NormalizeRequestedEnumerationBoundary's
// quote-grounding contract).
func NormalizeCompletenessObligation(raw string, in *CompletenessObligation) *CompletenessObligation {
	if in == nil || !in.Required {
		return nil
	}
	quote := strings.TrimSpace(in.SourceQuote)
	if quote == "" {
		return nil
	}
	if !requestedEnumerationQuotePresent(raw, quote) {
		return nil
	}
	return &CompletenessObligation{
		Required:    true,
		SourceQuote: quote,
	}
}

// QuestionBucket represents one user-named partition of the answer.
// Triggered by questions that explicitly carve the response into
// labeled groups: "X for A, Y for B" / "分别列出 A 和 B 的 ..." /
// "compare A vs B" / "for read mode and write mode separately".
//
// The bucket Label MUST be verbatim from RawRequest so downstream
// stages (the answer-document renderer, the bucket-alignment gate)
// can render the user's exact labels back as section headers, and
// the user's mental partition survives end-to-end.
//
// Anchors[] lists the LLM-resolved entities that this bucket
// represents (e.g. for bucket "Turn A": ["explorer", "Turn A"]).
// Used by the bucket-alignment gate to verify that the rendered
// answer's per-bucket items are disjoint across siblings (one
// entity must belong to at most one bucket).
//
// Index is 1-based and follows the bucket's order in the question
// (so "A 和 B" yields Index=1 for A, Index=2 for B), so the
// renderer can preserve the user's stated order even when downstream
// re-orders by relevance.
type QuestionBucket struct {
	Label   string   `json:"label"`
	Anchors []string `json:"anchors,omitempty"`
	Index   int      `json:"index"`
}

// IsValid reports whether a bucket carries at least a non-empty
// label. Anchors[] empty is allowed (the LLM may not have resolved
// concrete entities yet); the gate-side coherence rule R1.7 catches
// "buckets emitted but anchors all empty" cases.
func (b QuestionBucket) IsValid() bool {
	return strings.TrimSpace(b.Label) != ""
}

// NormalizeBuckets validates a slice of buckets:
//   - drops entries whose Label is missing OR whose Label is not
//     verbatim in raw (a fabricated label is dropped, not the entire
//     slice);
//   - dedupes by Label (case-insensitive);
//   - re-indexes 1..N in input order;
//   - returns nil for an empty result so callers can branch on nil.
//
// Verbatim presence reuses requestedEnumerationQuotePresent so the
// whitespace-folding contract (case-insensitive + multi-space
// collapse) is identical to EnumerationBoundary's quote validation.
// A bucket with a label like "Turn  A" (extra whitespace) still
// matches a raw containing "Turn A".
func NormalizeBuckets(raw string, in []QuestionBucket) []QuestionBucket {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]QuestionBucket, 0, len(in))
	for _, b := range in {
		label := strings.TrimSpace(b.Label)
		if label == "" {
			continue
		}
		if !requestedEnumerationQuotePresent(raw, label) {
			continue
		}
		key := strings.ToLower(label)
		if seen[key] {
			continue
		}
		seen[key] = true
		anchors := make([]string, 0, len(b.Anchors))
		for _, a := range b.Anchors {
			a = strings.TrimSpace(a)
			if a != "" {
				anchors = append(anchors, a)
			}
		}
		out = append(out, QuestionBucket{
			Label:   label,
			Anchors: anchors,
			Index:   len(out) + 1,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// QuestionStructureView is a read-only synthetic view of a
// RequestModel's structural obligations. Returned by
// RequestModel.QuestionStructure() so consumers can ask "does this
// question carry any obligation" with a single call.
//
// The view is computed on demand and not cached — RequestModel is
// already an in-memory struct, the cost is three pointer reads.
type QuestionStructureView struct {
	EnumerationBoundary    *RequestedEnumerationBoundary
	CompletenessObligation *CompletenessObligation
	Buckets                []QuestionBucket
}

// HasAnyObligation reports whether ANY axis is populated. Used by
// the orchestrator's question-obligation gate to fast-path skip
// gate evaluation when the question is structurally unbounded.
func (v QuestionStructureView) HasAnyObligation() bool {
	if v.EnumerationBoundary != nil && v.EnumerationBoundary.DeclaredCount > 0 {
		return true
	}
	if v.CompletenessObligation.IsActive() {
		return true
	}
	if len(v.Buckets) >= 2 {
		return true
	}
	return false
}

// ResolvedDeclaredCount returns the effective declared count: from
// EnumerationBoundary directly (s1a-style "9 of X"). Returns 0 when
// no count obligation. CompletenessObligation does NOT have a count
// (it just demands exhaustiveness) — use KnownCandidateCount for
// that instead.
func (v QuestionStructureView) ResolvedDeclaredCount() int {
	if v.EnumerationBoundary != nil {
		return v.EnumerationBoundary.DeclaredCount
	}
	return 0
}

// QuestionStructure returns the synthetic view of all structural
// obligations on this RequestModel. Cheap (3 pointer reads) so
// downstream consumers can call repeatedly without caching.
func (rm RequestModel) QuestionStructure() QuestionStructureView {
	return QuestionStructureView{
		EnumerationBoundary:    rm.EnumerationBoundary,
		CompletenessObligation: rm.CompletenessObligation,
		Buckets:                EffectiveQuestionBuckets(rm),
	}
}

// EffectiveQuestionBuckets returns the analyzer-emitted bucket
// partition when present, with a typed safety net for cross-file /
// cross-component comparison-like questions where the analyzer omitted
// buckets but already emitted all ingredients needed to preserve the
// user's partition downstream:
//
//   - IsCrossComponent=true,
//   - two or more analyzer sub-topics, and
//   - two or more high-confidence RequiredFileHints whose visible file
//     labels are present in RawRequest.
//
// This deliberately does NOT scan the raw request for comparison cue
// words. RawRequest is used only as a precise provenance check for the
// bucket labels, just like NormalizeBuckets. The inferred buckets are
// structure, not answer content: they let QFComparison keep both sides
// visible instead of letting an enumeration/completeness obligation
// flatten support evidence into principal members.
func EffectiveQuestionBuckets(rm RequestModel) []QuestionBucket {
	if len(rm.Buckets) >= 2 {
		return rm.Buckets
	}
	if len(rm.Buckets) > 0 {
		return rm.Buckets
	}
	return inferRequiredFileComparisonBuckets(rm)
}

func inferRequiredFileComparisonBuckets(rm RequestModel) []QuestionBucket {
	if !rm.Predicates.IsCrossComponent || len(rm.SubTopics) < 2 {
		return nil
	}
	raw := strings.TrimSpace(rm.RawRequest)
	if raw == "" || len(rm.AnalyzerHints.RequiredFileHints) < 2 {
		return nil
	}
	var candidates []requiredFileBucketCandidate
	seen := map[string]bool{}
	for sourceIdx, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		label, order := requiredFileBucketLabel(raw, hint.Path)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, requiredFileBucketCandidate{
			label:  label,
			path:   strings.TrimSpace(hint.Path),
			order:  order,
			source: sourceIdx,
		})
	}
	if len(candidates) < 2 {
		return nil
	}
	sortQuestionBucketCandidates(candidates)
	out := make([]QuestionBucket, 0, len(candidates))
	for i, c := range candidates {
		out = append(out, QuestionBucket{
			Label:   c.label,
			Anchors: []string{c.path},
			Index:   i + 1,
		})
	}
	return out
}

type requiredFileBucketCandidate struct {
	label  string
	path   string
	order  int
	source int
}

func requiredFileBucketLabel(raw, path string) (string, int) {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	if path == "" {
		return "", -1
	}
	base := path
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	for _, label := range []string{base, path} {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if order := foldedSurfaceIndex(raw, label); order >= 0 {
			return label, order
		}
	}
	return "", -1
}

func foldedSurfaceIndex(raw, label string) int {
	foldRaw := foldEnumerationBoundarySurface(raw)
	foldLabel := foldEnumerationBoundarySurface(label)
	if foldRaw == "" || foldLabel == "" {
		return -1
	}
	return strings.Index(foldRaw, foldLabel)
}

func sortQuestionBucketCandidates(candidates []requiredFileBucketCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].order != candidates[j].order {
			return candidates[i].order < candidates[j].order
		}
		return candidates[i].source < candidates[j].source
	})
}
