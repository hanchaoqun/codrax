package types

// investigation_step.go — the InvestigationStep decomposition model.
//
// InvestigationStep replaces the flat ERM requirement list with a
// DAG of ordered investigation sub-goals. Each step carries an
// Objective (what to find), a Strategy (how to find it), a DependsOn
// list (prerequisites), and a Status (lifecycle). The explorer
// navigates steps sequentially: only when the current step is
// satisfied does it advance to the next. This gives the
// investigation a STRUCTURAL completion criterion — "connected
// chain from A to B" — instead of a NUMERICAL one (count>=N
// evidence items), eliminating the "按下葫芦浮起瓢" problem where
// raising one question type's threshold breaks another.
//
// Design invariants:
//
//  1. Simple questions decompose into exactly 1 step whose
//     Strategy is derived from question_kind. This degrades to the
//     pre-InvestigationStep behavior so existing tests pass
//     without modification.
//  2. Steps form an acyclic chain (DependsOn is monotonically
//     increasing step IDs). No cycles, no diamond merges — the
//     explorer advances linearly.
//  3. The decomposition is deterministic (no LLM call). Rules in
//     DecomposeQuestion fire on (question_kind × entity_count ×
//     complexity × relational_verb_presence). The LLM's SubTopics
//     field feeds into entity lists per step but does NOT control
//     step count or strategy.
//  4. Dynamic refinement (P4) APPENDS steps or SPLITS a step
//     in-place but never DELETES or REORDERS existing satisfied
//     steps. This keeps evidence attribution stable.

// InvestStrategy classifies the reasoning approach an
// InvestigationStep requires. The explorer uses Strategy to:
//   - select which files to prioritize (e.g. StrategyLocate prefers
//     definition files; StrategyTraceChain prefers dispatch files)
//   - decide when the step is satisfied (e.g. StrategyTraceChain
//     checks for a connected evidence path between entities)
//   - generate appropriate mid-loop / soft-stop hints
type InvestStrategy string

const (
	// StrategyLocate — find where something is defined, registered,
	// or configured. Satisfied when at least one file:line citation
	// for each target entity is grounded.
	StrategyLocate InvestStrategy = "locate"

	// StrategyTraceChain — follow a call/data flow from entity A to
	// entity B across files. Satisfied when the evidence contains a
	// connected path (hop-by-hop) from the source entity to the
	// target entity. This is the structural completion criterion
	// that prevents the "read 3 files and declare mechanism
	// satisfied" failure.
	StrategyTraceChain InvestStrategy = "trace_chain"

	// StrategyEnumerate — list ALL instances of a category.
	// Satisfied when the evidence covers ≥80% of discovered
	// candidates (same threshold as the existing enumeration
	// coverage gate).
	StrategyEnumerate InvestStrategy = "enumerate"

	// StrategyVerifyCondition — for each candidate from a prior
	// enumerate step, check whether a specific condition holds.
	// Satisfied when every candidate has a verdict (meets / does
	// not meet). Prevents the "answer the total count without
	// checking the condition" short-circuit.
	StrategyVerifyCondition InvestStrategy = "verify_condition"

	// StrategyAggregate — summarise/count/compare results from
	// prior steps. Terminal step, always satisfied once prior
	// dependencies are done (the aggregation is the LLM's job at
	// synthesis time, not an evidence-collection task).
	StrategyAggregate InvestStrategy = "aggregate"

	// StrategyCompare — collect parallel evidence about two or
	// more entities and identify differences. Satisfied when each
	// entity has ≥2 evidence items.
	StrategyCompare InvestStrategy = "compare"

	// StrategyGeneric — fallback for simple questions that don't
	// match any specialised strategy. Satisfied by the existing
	// ERM mechanism (count-based threshold). This is the
	// degradation path for 1-step decompositions.
	StrategyGeneric InvestStrategy = "generic"
)

// StepStatus tracks the lifecycle of an InvestigationStep within
// the explorer's ReAct loop.
type StepStatus string

const (
	StepPending    StepStatus = "pending"     // not yet started (dependencies unmet)
	StepActive     StepStatus = "active"      // currently being investigated
	StepSatisfied  StepStatus = "satisfied"   // completion criterion met
	StepBlocked    StepStatus = "blocked"     // dependency failed or unreachable
)

// InvestigationStep is one sub-goal in the explorer's investigation
// plan. Steps are ordered by DependsOn — the explorer advances
// through them sequentially, only starting a step when all its
// dependencies are satisfied.
type InvestigationStep struct {
	// ID is a stable identifier like "step-0", "step-1", etc.
	// Used by DependsOn references and logging.
	ID string `json:"id"`

	// Objective is a one-sentence description of what this step
	// must establish. Rendered into the explorer's mid-loop /
	// soft-stop hints so the LLM knows what to focus on.
	Objective string `json:"objective"`

	// Strategy classifies the reasoning approach. The explorer
	// dispatches on this to decide which files to prioritize and
	// when the step is complete.
	Strategy InvestStrategy `json:"strategy"`

	// Entities are the code identifiers this step is about. For
	// StrategyTraceChain, Entities[0] is the source and
	// Entities[1] is the target of the chain.
	Entities []string `json:"entities,omitempty"`

	// DependsOn lists the IDs of steps that must be satisfied
	// before this step can start. Empty means "ready immediately".
	DependsOn []string `json:"depends_on,omitempty"`

	// Status tracks the lifecycle. Set by the explorer during the
	// ReAct loop; initial value is StepPending.
	Status StepStatus `json:"status"`

	// Evidence collected specifically for this step. Populated by
	// the step-aware explorer; used by CompletenessChecker to
	// verify answer coverage.
	Evidence []EvidenceItem `json:"evidence,omitempty"`

	// Verdict is the step-level conclusion. For
	// StrategyVerifyCondition, this is the per-candidate verdict
	// list. For StrategyAggregate, this is the final count or
	// summary. Empty until the step is satisfied.
	Verdict string `json:"verdict,omitempty"`

	// RetryCount tracks how many times the explorer retried this
	// step without making progress. Used by DynamicReplan to
	// decide when to refine/split.
	RetryCount int `json:"retry_count,omitempty"`
}

// InvestigationPlan is the ordered list of steps for one
// exploration dispatch. Stored on the explorerEvaluator and
// consumed by the step-aware soft-stop / mid-loop logic.
type InvestigationPlan struct {
	Steps []InvestigationStep `json:"steps"`
}

// CurrentStep returns the first non-satisfied step, or nil when
// all steps are done. The explorer calls this at each iteration
// to decide what to focus on.
func (p *InvestigationPlan) CurrentStep() *InvestigationStep {
	if p == nil {
		return nil
	}
	for i := range p.Steps {
		if p.Steps[i].Status != StepSatisfied {
			return &p.Steps[i]
		}
	}
	return nil
}

// AllSatisfied returns true when every step has Status==StepSatisfied.
func (p *InvestigationPlan) AllSatisfied() bool {
	if p == nil || len(p.Steps) == 0 {
		return true
	}
	for _, s := range p.Steps {
		if s.Status != StepSatisfied {
			return false
		}
	}
	return true
}

// ReadySteps returns steps whose dependencies are all satisfied
// and whose own status is still pending.
func (p *InvestigationPlan) ReadySteps() []*InvestigationStep {
	if p == nil {
		return nil
	}
	satisfied := make(map[string]bool, len(p.Steps))
	for _, s := range p.Steps {
		if s.Status == StepSatisfied {
			satisfied[s.ID] = true
		}
	}
	var ready []*InvestigationStep
	for i := range p.Steps {
		s := &p.Steps[i]
		if s.Status != StepPending {
			continue
		}
		allDeps := true
		for _, dep := range s.DependsOn {
			if !satisfied[dep] {
				allDeps = false
				break
			}
		}
		if allDeps {
			ready = append(ready, s)
		}
	}
	return ready
}

// MarkSatisfied sets a step's status to satisfied and records
// the verdict. No-op if the step is already satisfied.
func (p *InvestigationPlan) MarkSatisfied(stepID, verdict string) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Status = StepSatisfied
			p.Steps[i].Verdict = verdict
			return
		}
	}
}

// Activate sets a pending step to active. Called when the
// explorer starts working on it.
func (p *InvestigationPlan) Activate(stepID string) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID && p.Steps[i].Status == StepPending {
			p.Steps[i].Status = StepActive
			return
		}
	}
}

// AppendEvidence adds evidence items to a specific step.
func (p *InvestigationPlan) AppendEvidence(stepID string, items ...EvidenceItem) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].Evidence = append(p.Steps[i].Evidence, items...)
			return
		}
	}
}

// IncrRetry bumps the retry counter for a step. Used by
// DynamicReplan to detect stalls.
func (p *InvestigationPlan) IncrRetry(stepID string) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			p.Steps[i].RetryCount++
			return
		}
	}
}

// InsertAfter inserts newSteps immediately after the step with
// the given ID. Used by DynamicReplan to refine a stalled step
// into sub-steps.
func (p *InvestigationPlan) InsertAfter(afterID string, newSteps ...InvestigationStep) {
	if p == nil {
		return
	}
	for i := range p.Steps {
		if p.Steps[i].ID == afterID {
			tail := append([]InvestigationStep{}, p.Steps[i+1:]...)
			p.Steps = append(p.Steps[:i+1], newSteps...)
			p.Steps = append(p.Steps, tail...)
			return
		}
	}
}

// Summary returns a compact one-line status string for logging:
// "[step-0 ✓] [step-1 ▶] [step-2 ○]"
func (p *InvestigationPlan) Summary() string {
	if p == nil || len(p.Steps) == 0 {
		return "(no plan)"
	}
	var parts []string
	for _, s := range p.Steps {
		icon := "○" // pending
		switch s.Status {
		case StepActive:
			icon = "▶"
		case StepSatisfied:
			icon = "✓"
		case StepBlocked:
			icon = "✗"
		}
		parts = append(parts, "["+s.ID+" "+icon+"]")
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}
