package skill

import (
	"fmt"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Config defines the strategy for a specific type of work.
//
// Tier-based restructure (P5, 2026-05-10): the original Workflow
// and Prohibitions slices stay as Tier A — load-bearing rules that
// always render. New WorkflowTierB / ProhibitionsTierB fields hold
// applicability-gated style polish that only renders when the
// dispatch context matches (e.g. log-attached question, diagram
// required, retry path with matching ViolKind). Skills that don't
// declare Tier B fields keep their behaviour byte-identical.
//
// Design doc: docs/design/finalizer_skill_restructure.md.
type Config struct {
	Name            string   `json:"name" yaml:"name"`
	Goal            string   `json:"goal" yaml:"goal"`
	Workflow        []string `json:"workflow" yaml:"workflow"`
	ToolSuggestions []string `json:"tool_suggestions" yaml:"tool_suggestions"`
	OutputFormat    string   `json:"output_format" yaml:"output_format"`
	Prohibitions    []string `json:"prohibitions" yaml:"prohibitions"`

	// WorkflowTierB carries Workflow items that are conditionally
	// applicable. Each item declares an AppliesToFilter; the
	// renderer hides items whose filter does not match the
	// current dispatch. Empty slice → byte-identical to pre-P5
	// behaviour for that skill.
	WorkflowTierB []TierBItem `json:"workflow_tier_b,omitempty" yaml:"workflow_tier_b,omitempty"`

	// ProhibitionsTierB mirrors WorkflowTierB for the Prohibitions
	// surface. Same applicability gating semantics.
	ProhibitionsTierB []TierBItem `json:"prohibitions_tier_b,omitempty" yaml:"prohibitions_tier_b,omitempty"`
}

// TierBItem is one applicability-gated rule (Workflow or
// Prohibition). Body holds the rule prose verbatim from the
// pre-P5 single-string surface — P5-B does NOT reword any rule;
// only categorises them.
//
// AppliesTo describes WHEN to render this item. OnViolation lists
// the typed ViolationKind values that, when present in the prior
// emit's RetryState, force this item to render even when the
// AppliesTo filter would otherwise hide it (the rule just fired —
// the LLM needs to see it).
type TierBItem struct {
	Body        string                `json:"body" yaml:"body"`
	AppliesTo   AppliesToFilter       `json:"applies_to,omitempty" yaml:"applies_to,omitempty"`
	OnViolation []types.ViolationKind `json:"on_violation,omitempty" yaml:"on_violation,omitempty"`
}

// AppliesToFilter describes the dispatch contexts a TierBItem is
// relevant to. Multiple flags AND together for the typed shape
// requirements; the slices are OR-matched against the dispatch's
// principal kind / intent.
//
// Always=true short-circuits the renderer to "render unconditionally"
// (used for items the design says are Tier B but should always render
// — rare; reserved as an escape hatch).
type AppliesToFilter struct {
	Always           bool                     `json:"always,omitempty" yaml:"always,omitempty"`
	PrincipalKinds   []types.AnswerBlockKind  `json:"principal_kinds,omitempty" yaml:"principal_kinds,omitempty"`
	Intents          []types.Intent           `json:"intents,omitempty" yaml:"intents,omitempty"`
	RequiresDiagram  bool                     `json:"requires_diagram,omitempty" yaml:"requires_diagram,omitempty"`
	RequiresLog      bool                     `json:"requires_log,omitempty" yaml:"requires_log,omitempty"`
	RequiresBuckets  bool                     `json:"requires_buckets,omitempty" yaml:"requires_buckets,omitempty"`
	AbsenceContract  bool                     `json:"absence_contract,omitempty" yaml:"absence_contract,omitempty"`
}

// AppliesToContext is the runtime projection the renderer reads to
// evaluate AppliesToFilter — narrow snapshot built once per
// BuildInitialInstruction call so each filter check is O(1).
//
// Populated from BusContext / AgentContext / view at the renderer
// entry point; nil-safe.
type AppliesToContext struct {
	PrincipalKind   types.AnswerBlockKind
	Intent          types.Intent
	HasDiagram      bool
	HasLog          bool
	HasBuckets      bool
	IsAbsence       bool
	RetryViolations []types.ViolationKind
}

// MatchesAppliesTo returns true when the filter's gating conditions
// are met by the runtime context. Empty filter (zero-value) matches
// everything — defensive default for migration steps that haven't
// added filters yet.
//
// Match logic:
//   - Always=true → match unconditionally
//   - PrincipalKinds non-empty → kind must be in the list (else
//     this OR branch is unsatisfied; falls through to other gates)
//   - RequiresDiagram → ctx.HasDiagram must be true
//   - RequiresLog → ctx.HasLog must be true
//   - RequiresBuckets → ctx.HasBuckets must be true
//   - AbsenceContract → ctx.IsAbsence must be true
//   - Intents non-empty → intent must be in the list (additive OR
//     with PrincipalKinds — either gates the item visible)
//
// All true gates AND together; satisfying any of the typed slices
// admits the item. This matches the design's "applicability
// filter" semantics — if any relevant signal points at the rule,
// surface it.
func (f AppliesToFilter) MatchesAppliesTo(ctx AppliesToContext) bool {
	if f.Always {
		return true
	}
	// AND-match the boolean gates.
	if f.RequiresDiagram && !ctx.HasDiagram {
		return false
	}
	if f.RequiresLog && !ctx.HasLog {
		return false
	}
	if f.RequiresBuckets && !ctx.HasBuckets {
		return false
	}
	if f.AbsenceContract && !ctx.IsAbsence {
		return false
	}
	// OR-match the slice gates: when both are empty, it's a
	// pure-AND-gates filter that has already passed above; admit.
	if len(f.PrincipalKinds) == 0 && len(f.Intents) == 0 {
		// All bool gates passed (or were unset and no slice
		// gates declared) → admit.
		return f.RequiresDiagram || f.RequiresLog || f.RequiresBuckets || f.AbsenceContract
	}
	for _, k := range f.PrincipalKinds {
		if k == ctx.PrincipalKind {
			return true
		}
	}
	for _, i := range f.Intents {
		if i == ctx.Intent {
			return true
		}
	}
	return false
}

// ShouldRender returns true when the TierBItem should appear in
// the rendered prompt for the given dispatch context. Two paths
// admit:
//
//  1. AppliesTo matches: standard applicability gate (designed for
//     first-emit relevance filtering)
//  2. OnViolation contains a kind in ctx.RetryViolations: the
//     specific violation just fired in the prior emit, so the LLM
//     needs to see this rule on retry even when the AppliesTo
//     filter would normally hide it
//
// Either path admits; both can fire on the same item (a diagram
// rule that's also pinned to ViolDiagramEdgeLabelMismatch will
// render on first emit when the diagram is required AND on retry
// when the violation appears, even on a non-diagram dispatch).
func (item TierBItem) ShouldRender(ctx AppliesToContext) bool {
	if item.AppliesTo.MatchesAppliesTo(ctx) {
		return true
	}
	if len(item.OnViolation) > 0 && len(ctx.RetryViolations) > 0 {
		for _, k := range item.OnViolation {
			for _, viol := range ctx.RetryViolations {
				if k == viol {
					return true
				}
			}
		}
	}
	return false
}

// Registry manages skill configurations.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Config
}

func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Config)}
}

func (r *Registry) Register(cfg *Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[cfg.Name] = cfg
}

func (r *Registry) Get(name string) (*Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return s, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}
