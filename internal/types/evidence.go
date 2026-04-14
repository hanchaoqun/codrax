package types

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// EvidenceKind classifies a structured evidence item produced by either
// the LLM investigation notes or the deterministic analysis layers.
type EvidenceKind string

const (
	EvidenceDirect       EvidenceKind = "direct"
	EvidenceConditional  EvidenceKind = "conditional"
	EvidenceRegistration EvidenceKind = "registration"
	EvidenceMechanism    EvidenceKind = "mechanism"
	EvidenceRelationship EvidenceKind = "relationship"
	EvidenceAbsent       EvidenceKind = "absent"
	EvidenceConcrete     EvidenceKind = "concrete_value"
	EvidenceDataflowPath EvidenceKind = "dataflow_path"
	EvidenceConflict     EvidenceKind = "conflict"
	EvidenceUnresolved   EvidenceKind = "unresolved"
	EvidenceTruncated    EvidenceKind = "analysis_truncated"
)

// EvidenceItem is the normalized, structured representation of a
// single evidence statement that can be carried across agents/stages.
type EvidenceItem struct {
	ID          string       `json:"id"`
	Kind        EvidenceKind `json:"kind"`
	Subject     string       `json:"subject,omitempty"`
	Predicate   string       `json:"predicate,omitempty"`
	Object      string       `json:"object,omitempty"`
	Summary     string       `json:"summary,omitempty"`
	Condition   string       `json:"condition,omitempty"`
	Source      string       `json:"source,omitempty"`
	EvidenceRef string       `json:"evidence_ref,omitempty"`
	LineStart   int          `json:"line_start,omitempty"`
	LineEnd     int          `json:"line_end,omitempty"`
	DerivedFrom []string     `json:"derived_from,omitempty"`
	Confidence  float64      `json:"confidence,omitempty"`
	Producer    string       `json:"producer,omitempty"`
}

// FlowFindingDigest is the compact, stage-safe output of the dataflow
// engine. It intentionally omits the full graph and preserves only the
// user-facing path plus the evidence IDs needed for replay.
type FlowFindingDigest struct {
	ID                string   `json:"id"`
	Path              []string `json:"path,omitempty"`
	Conditions        []string `json:"conditions,omitempty"`
	Sources           []string `json:"sources,omitempty"`
	Sinks             []string `json:"sinks,omitempty"`
	Hops              []string `json:"hops,omitempty"`
	EvidenceIDs       []string `json:"evidence_ids,omitempty"`
	UnsupportedReason string   `json:"unsupported_reason,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
}

// StableEvidenceID returns a deterministic ID derived from the
// semantically relevant evidence fields. Callers should use this for
// dedupe/merge so that the same evidence coming from main/sub explorers
// or deterministic passes coalesces cleanly.
func StableEvidenceID(kind EvidenceKind, subject, predicate, object, condition, source string, lineStart, lineEnd int) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join([]string{
		string(kind),
		subject,
		predicate,
		object,
		condition,
		source,
		fmt.Sprintf("%d", lineStart),
		fmt.Sprintf("%d", lineEnd),
	}, "\x1f")))
	return fmt.Sprintf("ev-%x", h.Sum64())
}

// CompletenessClaim is the set-level authority the producer of an
// AnswerSymbol slate attaches to that slate. It answers "how
// authoritative is this list?" — a question that extractAnswerSymbols
// (the pre-P2.1 selection layer) could never answer and that the
// downstream rendering layer (context/builder.go §Answer Symbols)
// silently defaulted to "complete", producing the UNRESOLVED #1 bug
// where a partial LLM-derived allowlist was sold to the finalizer as
// a verified-complete answer. See memory/project_p2_1_session_1_shipped.md.
//
// The three claims form a strict authority ladder:
//
//   - CompletenessComplete: these are ALL the answers. The finalizer
//     runs its Translation mode prompt with "MUST NOT add or remove
//     symbols" directive. Safe only when the upstream producer has
//     structurally validated that no answers were missed.
//
//   - CompletenessLowerBound: these are confirmed present, but more
//     may exist. The finalizer runs a softened prompt: "MUST include
//     at least these names; MAY add more if evidence supports them."
//     This is the honest default when a partial allowlist is the best
//     we have — it preserves the floor without forbidding the ceiling.
//
//   - CompletenessUnknown: no authority at all. The finalizer drops
//     the answer-symbols section entirely and falls back to the
//     shape-based prompt (step_list / explanation / etc.). Zero-value
//     of the type so an un-set field naturally means "no claim".
//
// The enum is closed — the emit_answer_symbol schema in P2.1 P9 will
// reject any other string. CompletenessUnknown is the zero value so
// that legacy code paths that do not set the field degrade safely.
type CompletenessClaim string

const (
	// CompletenessUnknown is the zero value. It means the producer
	// either did not run a cardinality check at all, or ran one and
	// could not reach a definitive verdict. The rendering layer must
	// treat the answer-symbol slate as non-authoritative and fall back
	// to the shape-based prompt. Leaving the field at zero value is
	// always safe; it is the "fail closed" default for P2.1's
	// completeness contract.
	CompletenessUnknown CompletenessClaim = ""

	// CompletenessComplete asserts that the attached slate lists every
	// symbol that answers the question. The finalizer runs Translation
	// mode with "MUST NOT add or remove symbols". Producers MUST have
	// run a structural completeness check (Turn A's deterministic
	// extraction with a ≥1 baseline, or Turn B's emit_answer_symbol
	// claim validated against Turn A's TerminalEvidenceCount and the
	// AnalysisIR.AnswerContract.MustInclude cross-ref) before writing
	// this value. Writing it without the check is forbidden by the
	// P9 schema validator.
	CompletenessComplete CompletenessClaim = "complete"

	// CompletenessLowerBound asserts that the attached slate is a
	// proper floor on the true answer set — every listed symbol is
	// confirmed present, but the slate may be missing additional
	// symbols that also answer the question. The finalizer runs the
	// softened Translation prompt that preserves the floor without
	// forbidding the ceiling. This is the right claim when
	// investigation was bounded (read-file budget hit, grep recall
	// partial, etc.) but produced high-confidence matches.
	CompletenessLowerBound CompletenessClaim = "lower_bound"
)

// IsValid reports whether c is one of the three defined CompletenessClaim
// values. Used by the emit_answer_symbol schema validator and by
// applyStageOutput's merge rule so an invalid value cannot leak into
// BusContext.
func (c CompletenessClaim) IsValid() bool {
	switch c {
	case CompletenessUnknown, CompletenessComplete, CompletenessLowerBound:
		return true
	}
	return false
}

// AnswerSymbol is the structured, deterministic form of a single
// answer. Produced by Turn B's extractor via emit_answer_symbol,
// it is the bridge between the deterministic pipeline (which
// identifies the correct symbol) and the finalizer (which must
// render it as prose without adding or removing names).
type AnswerSymbol struct {
	Name      string `json:"name"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Chain     string `json:"chain"`               // full chain text that yielded this symbol
	Kind      string `json:"kind"`                // question_kind at extraction time
	Rationale string `json:"rationale,omitempty"` // optional: why this terminal was picked
}

// AnswerChain is the typed ranked-and-scored answer-relevance envelope
// around an EvidenceItem. Produced by identifyAnswerChains (pure Go,
// deterministic, no LLM) from the explorer's structuredEvidence pool.
//
// Design rationale: identifyAnswerChains used to return []string where
// each string was a pre-flattened render of the underlying evidence
// (ev.Summary plus a " (file:line)" suffix). That violated the
// architecture principle "prose only at the LLM boundary" because the
// flattening happened in Go mid-pipeline, losing the structured fields
// for every downstream consumer. The typed form keeps the underlying
// EvidenceItem intact and defers rendering to context/builder.go's
// prompt assembly step (the single legal flatten point).
//
// Fields:
//   - Item: the underlying EvidenceItem (all structured fields —
//     Subject / Predicate / Object / Source / LineStart / Summary /
//     Kind / Confidence). Downstream consumers that need a specific
//     field access Item directly instead of parsing a display string.
//   - Score: the relevance score identifyAnswerChains computed. Kept
//     in the envelope because (a) it is committed API semantics
//     ("how well does this chain answer the question"), (b) debug
//     / eval tooling wants to see why the ranking came out this way,
//     (c) recomputing is expensive.
//   - StrictOK: whether the chain passed the L0-1 terminal + origin
//     predicates. The strict subset is used to compute β for Turn B's
//     cardinality validator. Non-strict chains are retained in the
//     slate as Ground Truth fallback but never contribute to β.
type AnswerChain struct {
	Item     EvidenceItem `json:"item"`
	Score    float64      `json:"score"`
	StrictOK bool         `json:"strict_ok"`
}

// MergeAnswerChains combines multiple AnswerChain slices into one,
// preserving first-seen order and dropping duplicates by the chain
// identity tuple (Summary, Source, LineStart, Subject, Predicate,
// Object). The key matches identifyAnswerChains's own dedup key so a
// merged slice has the same cardinality as a freshly-produced one.
// Used by the orchestrator to deduplicate BusContext.AnswerChains on
// stage self-loops where the explorer's ParseOutput re-emits the
// full snapshot each run.
func MergeAnswerChains(groups ...[]AnswerChain) []AnswerChain {
	seen := make(map[string]struct{})
	var out []AnswerChain
	for _, g := range groups {
		for _, c := range g {
			key := fmt.Sprintf("%s|%s|%d|%s|%s|%s",
				c.Item.Summary, c.Item.Source, c.Item.LineStart,
				c.Item.Subject, c.Item.Predicate, c.Item.Object)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, c)
		}
	}
	return out
}

// MergeAnswerSymbols combines multiple AnswerSymbol slices into one,
// preserving first-seen order. Dedup key is a composite of Name +
// File + Line — identity fields the finalizer's translation-mode
// renderer uses to emit a unique bullet. Chain / Kind / Rationale
// are treated as metadata that may vary run-to-run without implying
// a different symbol.
func MergeAnswerSymbols(groups ...[]AnswerSymbol) []AnswerSymbol {
	seen := make(map[string]struct{})
	var out []AnswerSymbol
	for _, g := range groups {
		for _, s := range g {
			key := s.Name + "\x1f" + s.File + "\x1f" + itoa(s.Line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// itoa is a tiny in-package int-to-string to avoid importing strconv
// in this file just for the dedup key builder.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// StableFlowFindingID returns a deterministic ID for a compact
// dataflow finding based on its path/condition shape.
func StableFlowFindingID(path, conditions, sources, sinks []string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join([]string{
		strings.Join(path, "\x1e"),
		strings.Join(conditions, "\x1e"),
		strings.Join(sources, "\x1e"),
		strings.Join(sinks, "\x1e"),
	}, "\x1f")))
	return fmt.Sprintf("flow-%x", h.Sum64())
}
