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

// AnswerSymbol is the structured, deterministic form of a single
// answer. Produced by extractAnswerSymbols from the chain strings
// identifyAnswerChains returns, it is the bridge between the
// deterministic pipeline (which identifies the correct symbol) and
// the finalizer (which must render it as prose without adding or
// removing names).
//
// L0-2 design: the finalizer receives a list of AnswerSymbol, not
// raw chain text, so the LLM's answer-translation step is reduced to
// a structural enumeration it cannot hallucinate around. See
// project_L0_2_extract_then_express_design.md.
type AnswerSymbol struct {
	Name      string `json:"name"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Chain     string `json:"chain"`             // full chain text that yielded this symbol
	Kind      string `json:"kind"`              // question_kind at extraction time
	Rationale string `json:"rationale,omitempty"` // optional: why this terminal was picked
}

// MergeAnswerChains combines multiple answer-chain slices into one,
// preserving first-seen order and dropping exact duplicates. Used by
// the orchestrator to deduplicate BusContext.AnswerChains on stage
// self-loops where the explorer's ParseOutput re-emits the full
// snapshot each run. See memory/project_applystage_dedup.md.
func MergeAnswerChains(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, g := range groups {
		for _, c := range g {
			if _, ok := seen[c]; ok {
				continue
			}
			seen[c] = struct{}{}
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
