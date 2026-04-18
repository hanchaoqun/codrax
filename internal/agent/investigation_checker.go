package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// investigation_checker.go — per-step satisfaction checks + answer
// completeness verification.
//
// CheckStepSatisfaction runs at each explorer midLoop / softStop to
// determine whether the current investigation step can be marked
// satisfied. The check dispatches on InvestStrategy:
//
//   StrategyLocate         → ≥1 evidence item per entity
//   StrategyTraceChain     → evidence forms a connected chain from
//                            entity[0] to entity[1]
//   StrategyEnumerate      → ≥3 distinct evidence items (same
//                            threshold as ERM enumeration)
//   StrategyVerifyCondition → every candidate from the prior step
//                            has ≥1 verdict evidence item
//   StrategyAggregate      → always satisfied (terminal step)
//   StrategyCompare        → ≥2 evidence items per entity
//   StrategyGeneric        → ≥2 evidence items (same as ERM default)

// CheckStepSatisfaction determines whether the current step's
// structural criterion is met given the accumulated evidence. Does
// NOT modify the plan; the caller decides whether to advance.
func CheckStepSatisfaction(step *types.InvestigationStep, allEvidence []types.EvidenceItem, notes []string) bool {
	if step == nil {
		return true
	}
	switch step.Strategy {
	case types.StrategyAggregate:
		return true // terminal, always satisfied

	case types.StrategyLocate:
		return checkLocateSatisfied(step, allEvidence, notes)

	case types.StrategyTraceChain:
		return checkTraceChainSatisfied(step, allEvidence, notes)

	case types.StrategyEnumerate:
		return checkEnumerateSatisfied(step, allEvidence, notes)

	case types.StrategyVerifyCondition:
		return checkVerifyConditionSatisfied(step, allEvidence, notes)

	case types.StrategyCompare:
		return checkCompareSatisfied(step, allEvidence, notes)

	case types.StrategyGeneric:
		return checkGenericSatisfied(step, allEvidence, notes)

	default:
		return len(allEvidence) >= 2
	}
}

// checkLocateSatisfied requires at least 1 evidence item whose
// Subject or Source mentions each entity.
func checkLocateSatisfied(step *types.InvestigationStep, evidence []types.EvidenceItem, notes []string) bool {
	if len(step.Entities) == 0 {
		return len(evidence) >= 1
	}
	notesJoined := strings.ToLower(strings.Join(notes, "\n"))
	for _, ent := range step.Entities {
		entLower := strings.ToLower(ent)
		found := false
		for _, ev := range evidence {
			if strings.Contains(strings.ToLower(ev.Subject+ev.Object+ev.Summary), entLower) {
				found = true
				break
			}
		}
		if !found && strings.Contains(notesJoined, entLower) {
			found = true
		}
		if !found {
			return false
		}
	}
	return true
}

// checkTraceChainSatisfied requires evidence that forms a
// connected hop-by-hop path from entities[0] to entities[1]. Uses
// evidence Summary/Subject/Object text to detect mentions of each
// entity along the chain. This is the structural criterion that
// prevents "4 items from one file = satisfied" on multi-hop
// mechanism questions.
func checkTraceChainSatisfied(step *types.InvestigationStep, evidence []types.EvidenceItem, notes []string) bool {
	if len(step.Entities) < 2 {
		return len(evidence) >= 2
	}
	src := strings.ToLower(step.Entities[0])
	tgt := strings.ToLower(step.Entities[1])

	// Both source and target entities must appear in evidence.
	srcFound := false
	tgtFound := false
	for _, ev := range evidence {
		text := strings.ToLower(ev.Subject + " " + ev.Object + " " + ev.Summary)
		if strings.Contains(text, src) {
			srcFound = true
		}
		if strings.Contains(text, tgt) {
			tgtFound = true
		}
	}

	// Also check investigation notes for entity mentions.
	notesJoined := strings.ToLower(strings.Join(notes, "\n"))
	if !srcFound && strings.Contains(notesJoined, src) {
		srcFound = true
	}
	if !tgtFound && strings.Contains(notesJoined, tgt) {
		tgtFound = true
	}

	if !srcFound || !tgtFound {
		logging.Debug("[planner] trace_chain: src=%q found=%v tgt=%q found=%v",
			src, srcFound, tgt, tgtFound)
		return false
	}

	// Require at least 3 evidence items mentioning either entity —
	// a 2-hop chain (A→B→C) needs at minimum: A's definition,
	// the dispatch call, B's definition. Lower counts usually mean
	// the chain has gaps.
	relevant := 0
	for _, ev := range evidence {
		text := strings.ToLower(ev.Subject + " " + ev.Object + " " + ev.Summary)
		if strings.Contains(text, src) || strings.Contains(text, tgt) {
			relevant++
		}
	}
	if relevant < 3 {
		logging.Debug("[planner] trace_chain: only %d relevant evidence items (need ≥3)", relevant)
		return false
	}

	return true
}

// checkEnumerateSatisfied requires ≥3 distinct evidence items to
// ensure the LLM has found a meaningful set of candidates.
func checkEnumerateSatisfied(step *types.InvestigationStep, evidence []types.EvidenceItem, _ []string) bool {
	return len(evidence) >= 3
}

// checkVerifyConditionSatisfied checks that the investigation notes
// contain verdicts for multiple candidates. Uses heuristic: notes
// must mention at least 2 distinct "yes/no" or "满足/不满足" pattern.
func checkVerifyConditionSatisfied(step *types.InvestigationStep, _ []types.EvidenceItem, notes []string) bool {
	if len(notes) == 0 {
		return false
	}
	notesJoined := strings.ToLower(strings.Join(notes, "\n"))
	verdictCount := 0
	for _, pattern := range []string{
		"can ", "cannot ", "does ", "does not ",
		"可以", "不可以", "能", "不能",
		"satisfies", "does not satisfy",
		"满足", "不满足",
	} {
		verdictCount += strings.Count(notesJoined, pattern)
	}
	return verdictCount >= 2
}

// checkCompareSatisfied requires ≥2 evidence items per entity.
func checkCompareSatisfied(step *types.InvestigationStep, evidence []types.EvidenceItem, _ []string) bool {
	if len(step.Entities) < 2 {
		return len(evidence) >= 4
	}
	for _, ent := range step.Entities {
		entLower := strings.ToLower(ent)
		count := 0
		for _, ev := range evidence {
			if strings.Contains(strings.ToLower(ev.Subject+ev.Summary), entLower) {
				count++
			}
		}
		if count < 2 {
			return false
		}
	}
	return true
}

// checkGenericSatisfied — fallback; ≥2 evidence items.
func checkGenericSatisfied(_ *types.InvestigationStep, evidence []types.EvidenceItem, _ []string) bool {
	return len(evidence) >= 2
}

// CheckAnswerStepCoverage verifies that the final answer covers
// all investigation steps. Returns a list of gaps (steps whose
// objectives are not reflected in the answer text). Used by the
// CompletenessChecker to generate retry hints.
func CheckAnswerStepCoverage(plan *types.InvestigationPlan, answerText string) []string {
	if plan == nil || len(plan.Steps) <= 1 {
		return nil
	}
	lower := strings.ToLower(answerText)
	var gaps []string
	for _, step := range plan.Steps {
		if step.Strategy == types.StrategyAggregate {
			continue // terminal step, covered by the answer itself
		}
		// Check if the answer mentions at least one entity from the step
		covered := false
		for _, ent := range step.Entities {
			if strings.Contains(lower, strings.ToLower(ent)) {
				covered = true
				break
			}
		}
		if !covered {
			gaps = append(gaps, step.Objective)
		}
	}
	return gaps
}
