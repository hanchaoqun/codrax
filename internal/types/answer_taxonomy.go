package types

import (
	"strings"
	"time"
)

// AnswerPattern is one entry in the per-repo Answer Taxonomy
// (read-mode mirror of FailurePattern, commit 51 Gap 3): an
// abstract-but-actionable pitfall the analyzer should regard
// before classifying a new request in this repo. Distinct from
// a single answer rejection (which lives on the per-Run
// AnalyzerRetryHint) — a pitfall describes the *kind* of
// mis-classification or mis-shaping that produced the rejection,
// in language general enough that the next request can recognise
// the analogue and avoid it.
//
// Authoring contract: patterns are emitted by the answer_reviewer
// LLM after a successful read-mode Run that had retry hints (see
// internal/orchestrator/answer_reviewer.go). The emit is structured
// + bounded so a runaway LLM cannot accumulate a 10 KB taxonomy of
// fluff. The store applies dedup + decay; the analyzer's injection
// layer scores relevance per request.
//
// AppliesToScenarios + AppliesToFamilies scope a pattern to specific
// request shapes (mirror of FailurePattern.AppliesToKinds +
// AppliesToPaths). Empty = applies to all.
type AnswerPattern struct {
	// ID is a stable identifier for the pattern within this
	// repo's taxonomy. Format: "ap-<unix-nano>-<pid>" mirrors
	// FailurePattern's "fp-..." shape.
	ID string `json:"id"`

	// Name is a short label (≤ 60 chars) the analyzer sees when
	// deciding which pitfalls are relevant. Examples:
	//   - "enumeration questions count interfaces not implementations"
	//   - "explain shape on scalar subject mis-classifies as Numeric"
	// NOT a specific function name or file:line — those go in
	// Example*. Names should generalise.
	Name string `json:"name"`

	// Description is the 2-4 sentence "what is the pattern, how is
	// it recognised, why does it matter" prose. Read by the
	// analyzer via the planning-context injection; optimised for
	// LLM consumption (terse, declarative).
	Description string `json:"description"`

	// Trigger is the 1-2 sentence "when does this pitfall fire"
	// precondition. Used both as relevance filter (does it apply
	// to the current request?) and as an analyzer reminder.
	// Examples:
	//   - "when the user asks 'how many X' and X resolves to 2+ domains"
	//   - "when the request mixes a scalar subject with explanation shape"
	Trigger string `json:"trigger"`

	// Consequence is the 1 sentence "what happens if you don't
	// avoid it" — usually a category of answer rejection
	// (coherence-gate fail / shape-subject mismatch / hypothesis-
	// coverage gap / multi-topic under-decomposition). Helps the
	// analyzer predict downstream cost.
	Consequence string `json:"consequence"`

	// ExampleFile + ExampleLine pin the pattern to a real past
	// occurrence in this repo. Optional but encouraged — operators
	// reading /answer-pitfalls show want to see the concrete
	// instance the abstraction was distilled from.
	ExampleFile string `json:"example_file,omitempty"`
	ExampleLine int    `json:"example_line,omitempty"`

	// Confidence is the reviewer's self-reported certainty 0.0-1.0.
	// Patterns < 0.5 are dropped at emit time (cried-wolf noise
	// reduction); patterns ≥ 0.5 + low HitCount also decay faster.
	Confidence float64 `json:"confidence"`

	// FirstSeen + LastSeen track recency for the analyzer's
	// relevance score AND the decay sweep.
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	// HitCount tracks how many times this pattern has recurred.
	// Each Run that emits a pattern matching an existing one (by
	// SimilarTo) bumps HitCount instead of creating a duplicate.
	// High HitCount = pattern is a real recurring trap; low + old
	// = candidate for decay.
	HitCount int `json:"hit_count"`

	// AppliesToScenarios restricts injection to specific Scenario
	// values from RequestModel (e.g. "explain", "root_cause",
	// "enumerate"). Empty = applies to all scenarios. Used by the
	// analyzer's relevance filter to avoid showing
	// "enumeration-shape pitfalls" on a root-cause request.
	AppliesToScenarios []string `json:"applies_to_scenarios,omitempty"`

	// AppliesToFamilies restricts injection to specific
	// QuestionFamily values (e.g. "QFEnumeration", "QFRoleLookup",
	// "QFExplanation"). Empty = no family constraint. Lets the
	// analyzer skip pitfalls clearly unrelated to the family this
	// request will route to. Pre-PR4 this field was named
	// AppliesToShapes (json `applies_to_shapes`); the rename
	// mirrors the AnswerShape → QuestionFamily migration. Reader
	// path (`reviewer.go`) keeps `applies_to_shapes` as a one-way
	// JSON alias so on-disk caches written by older builds keep
	// loading; new writes only emit `applies_to_families`.
	AppliesToFamilies []string `json:"applies_to_families,omitempty"`
}

// IsValid reports whether the pattern carries the minimum content
// fields the store + analyzer require. Does NOT require ID — the
// store auto-assigns IDs at Append time, so emit-time validation
// runs before ID assignment. Used by emit-time validation +
// load-time filtering.
func (p *AnswerPattern) IsValid() bool {
	if p == nil {
		return false
	}
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 {
		return false
	}
	if len(strings.TrimSpace(p.Description)) < 30 {
		return false
	}
	if len(strings.TrimSpace(p.Trigger)) < 15 {
		return false
	}
	if p.Confidence < 0 || p.Confidence > 1 {
		return false
	}
	return true
}

// IsLoadValid is the load-time gate: an on-disk entry without an
// ID is malformed (we always wrote one) and the load filter drops
// it. IsValid (without ID) is the emit-time gate.
func (p *AnswerPattern) IsLoadValid() bool {
	if p == nil || strings.TrimSpace(p.ID) == "" {
		return false
	}
	return p.IsValid()
}

// AnswerTaxonomy is the per-repo cache of distilled answer
// pitfalls. One file per repo at <user-cache-dir>/codrax/
// <repo-slug>/answer_taxonomy.json. Persisted via atomic rename,
// mirror of FailureTaxonomy.
type AnswerTaxonomy struct {
	// RepoSlug is the deterministic id derived from the main
	// repo's absolute path (basename + fnv32 hash). Stable across
	// machines that mount the same path, distinct across machines
	// that don't.
	RepoSlug string `json:"repo_slug"`

	// Patterns is the active list. Append-only at emit time; dedup
	// + decay sweep runs at append + load time so the analyzer
	// always sees the most relevant N entries.
	Patterns []AnswerPattern `json:"patterns"`

	// SchemaVersion guards against silent decode of a future
	// shape. Bump when AnswerPattern changes incompatibly.
	SchemaVersion int `json:"schema_version"`
}

// CurrentAnswerSchemaVersion is the active AnswerTaxonomy schema
// version. Files with a different (older) value are rejected at
// load time so a future-version upgrade can detect the migration
// boundary explicitly.
const CurrentAnswerSchemaVersion = 1

// SimilarTo reports whether two patterns are close enough to dedup
// (treated as the same pitfall, HitCount bumped on the existing
// one rather than appending). Mirror of FailurePattern.SimilarTo:
// 50%+ shared first-significant-token from Name; small-set
// allowance (<=3 tokens needs only 1 shared).
func (p *AnswerPattern) SimilarTo(other *AnswerPattern) bool {
	if p == nil || other == nil {
		return false
	}
	a := normaliseName(p.Name)
	b := normaliseName(other.Name)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	at := significantTokens(a)
	bt := significantTokens(b)
	if len(at) == 0 || len(bt) == 0 {
		return false
	}
	shared := 0
	bset := make(map[string]struct{}, len(bt))
	for _, t := range bt {
		bset[t] = struct{}{}
	}
	for _, t := range at {
		if _, ok := bset[t]; ok {
			shared++
		}
	}
	min := len(at)
	if len(bt) < min {
		min = len(bt)
	}
	if min <= 3 {
		return shared >= 1
	}
	return shared*2 >= min
}
