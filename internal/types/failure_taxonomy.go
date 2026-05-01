package types

import (
	"strings"
	"time"
)

// FailurePattern is one entry in the per-repo Failure Taxonomy
// (stage 3): an abstract-but-actionable pitfall the planner
// should regard before emitting a new ChangePlan in this
// repo. Distinct from a single test failure (which lives on
// ChangeReport) — a pitfall describes the *kind* of mistake
// that produced the failure, in language general enough that
// the next plan can recognise the analogue and avoid it.
//
// Authoring contract: pitfalls are emitted by the reflector
// LLM after a verify→plan retry cycle (see
// internal/orchestrator/reflector.go). The emit is structured
// + bounded so a runaway LLM cannot accumulate a 10 KB
// taxonomy of fluff. The store applies dedup + decay; the
// planner's injection layer scores relevance per task.
type FailurePattern struct {
	// ID is a stable identifier for the pattern within this
	// repo's taxonomy. Format: "fp-<unix-nano>-<pid>" mirrors
	// PlanGroup / ChangePlan id shape.
	ID string `json:"id"`

	// Name is a short label (≤ 60 chars) the planner sees
	// when deciding which pitfalls are relevant. Examples:
	//   - "lock-scope drift on new public method"
	//   - "test fixture imports rot when production interface widens"
	// NOT a specific test name or file:line — those go in
	// Example*. Names should generalise.
	Name string `json:"name"`

	// Description is the 2-4 sentence "what is the pattern,
	// how is it recognised, why does it matter" prose. Read
	// by the planner via the planning-context injection;
	// optimised for LLM consumption (terse, declarative).
	Description string `json:"description"`

	// Trigger is the 1-2 sentence "when does this pitfall
	// fire" precondition. Used both as relevance filter
	// (does it apply to the current task?) and as a planner
	// reminder. Examples:
	//   - "when adding a method to a struct shared across goroutines"
	//   - "when widening an interface that downstream tests stub"
	Trigger string `json:"trigger"`

	// Consequence is the 1 sentence "what happens if you
	// don't avoid it" — usually a category of test failure
	// (race / panic / assertion drift / build break). Helps
	// the planner predict downstream cost of ignoring.
	Consequence string `json:"consequence"`

	// ExampleFile + ExampleLine pin the pattern to a real
	// past occurrence in this repo. Optional but encouraged
	// — operators reading /pitfalls show want to see the
	// concrete instance the abstraction was distilled from.
	ExampleFile string `json:"example_file,omitempty"`
	ExampleLine int    `json:"example_line,omitempty"`

	// Confidence is the reflector's self-reported certainty
	// 0.0-1.0. Patterns < 0.5 are dropped at emit time
	// (cried-wolf noise reduction); patterns ≥ 0.5 + low
	// HitCount also decay faster.
	Confidence float64 `json:"confidence"`

	// FirstSeen + LastSeen track recency for the planner's
	// relevance score AND the decay sweep. UnixNano so
	// JSON round-trip is integer-safe across platforms.
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	// HitCount tracks how many times this pattern has
	// recurred. Each verify→plan retry that emits a pattern
	// matching an existing one (by similar Name + Trigger)
	// bumps HitCount instead of creating a duplicate.
	// High HitCount = pattern is a real recurring trap;
	// low + old = candidate for decay.
	HitCount int `json:"hit_count"`

	// AppliesToKinds restricts injection to specific
	// WriteTask.Kind values (feature / bug_fix / refactor /
	// misc). Empty = applies to all kinds. Used by the
	// planner's relevance filter to avoid showing
	// "concurrent-code pitfalls" on a doc-rename task.
	AppliesToKinds []string `json:"applies_to_kinds,omitempty"`

	// AppliesToPaths is a path-prefix filter (e.g.
	// "internal/auth/" matches every file under that dir).
	// Empty = no path constraint. Lets the planner skip
	// pitfalls clearly unrelated to the files this task
	// touches.
	AppliesToPaths []string `json:"applies_to_paths,omitempty"`
}

// IsValid reports whether the pattern carries the minimum
// content fields the store + planner require. Does NOT
// require ID — the store auto-assigns IDs at Append time, so
// emit-time validation runs before ID assignment.
// Used by emit-time validation + load-time filtering.
func (p *FailurePattern) IsValid() bool {
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

// IsLoadValid is the load-time gate: an on-disk entry
// without an ID is malformed (we always wrote one) and the
// load filter drops it. IsValid (without ID) is the emit-
// time gate.
func (p *FailurePattern) IsLoadValid() bool {
	if p == nil || strings.TrimSpace(p.ID) == "" {
		return false
	}
	return p.IsValid()
}

// FailureTaxonomy is the per-repo cache of distilled pitfalls.
// One file per repo at <user-cache-dir>/codrax/<repo-slug>/
// failure_taxonomy.json. Persisted via atomic rename, mirror
// of PlanGroupStore's persistence pattern.
type FailureTaxonomy struct {
	// RepoSlug is the deterministic id derived from the main
	// repo's absolute path (basename + fnv32 hash). Stable
	// across machines that mount the same path, distinct
	// across machines that don't. Embedded in the JSON so a
	// sanity-check on load can detect when the operator
	// moved a cache file to a different repo.
	RepoSlug string `json:"repo_slug"`

	// Patterns is the active list. Append-only at emit time;
	// dedup + decay sweep runs at load time so the planner
	// always sees the most relevant N entries.
	Patterns []FailurePattern `json:"patterns"`

	// SchemaVersion guards against silent decode of a future
	// shape. Bump when the FailurePattern schema changes
	// incompatibly.
	SchemaVersion int `json:"schema_version"`
}

// CurrentSchemaVersion is the active FailureTaxonomy schema
// version. Files with a different (older) value are rejected
// at load time so a future-version upgrade can detect the
// migration boundary explicitly.
const CurrentSchemaVersion = 1

// SimilarTo reports whether two patterns are close enough to
// dedup (treated as the same pitfall, HitCount bumped on the
// existing one rather than appending). Heuristic: same kind +
// 50%+ shared first-significant-token from Name.
//
// Deliberately loose: the LLM names patterns slightly
// differently each emit; bumping a sibling rather than
// inflating the taxonomy keeps the cache useful at scale.
func (p *FailurePattern) SimilarTo(other *FailurePattern) bool {
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
	// Token-overlap heuristic: split on whitespace, lowercase,
	// drop stopwords, count shared. Small denominators (2-3
	// tokens) only need 1 shared; longer (5+) need 2.
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

// normaliseName lowercases + trims punctuation for token
// comparison. Kept package-private; SimilarTo is the only
// caller.
func normaliseName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == ' ', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// taxonomyStopwords filters generic English / framing words
// out of the SimilarTo overlap so "the failing test" doesn't
// match "the failing build" via a "failing"+"the" double-hit.
var taxonomyStopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "of": {}, "on": {}, "in": {},
	"to": {}, "for": {}, "and": {}, "or": {}, "with": {},
	"when": {}, "if": {}, "is": {}, "are": {}, "was": {},
	"new": {}, "old": {}, "can": {}, "may": {}, "be": {},
}

func significantTokens(s string) []string {
	out := make([]string, 0, 4)
	for _, t := range strings.Fields(s) {
		if _, drop := taxonomyStopwords[t]; drop {
			continue
		}
		if len(t) <= 2 {
			continue
		}
		out = append(out, t)
	}
	return out
}
