package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type staticAnswerReviewer struct {
	pattern *types.AnswerPattern
	err     error
}

func (r staticAnswerReviewer) ReviewAnswer(context.Context, AnswerReviewerInput) (*types.AnswerPattern, error) {
	return r.pattern, r.err
}

func TestReadFailureMemorySoftOnly(t *testing.T) {
	store := NewAnswerTaxonomyStore(t.TempDir(), "repo", 50, 90)
	mu := types.NewMutableState("explain the code architecture")
	mu.AppendAnswerRetryEvent(string(types.StageExplore), "first retry reason should not drive routing")
	mu.AppendAnswerRetryEvent(string(types.StageExplore), "second retry reason should not drive routing")
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:    types.ModeRead,
			Mutable: mu,
			AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			}},
		},
		answerTaxonomyStore: store,
	}

	o.runAnswerReviewerOnSuccess()

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("read failure memory patterns = %d, want 1: %+v", len(all), all)
	}
	p := all[0]
	if p.Confidence < 0.5 {
		t.Fatalf("confidence = %.2f, want >= 0.5", p.Confidence)
	}
	if got := store.RelevantTo(string(types.ScenarioArchitectureExplain), string(types.QFArchitecture), 5); len(got) != 1 {
		t.Fatalf("RelevantTo architecture returned %d patterns, want 1", len(got))
	}
	if got := store.RelevantTo(string(types.ScenarioRootCause), string(types.QFRootCauseTrace), 5); len(got) != 0 {
		t.Fatalf("pattern must stay scoped to typed scenario/family; got %d unrelated matches", len(got))
	}
	if got := mu.EvidenceClosure().ViolationsByKind(types.ViolAnswerReviewerDistilled); len(got) != 0 {
		t.Fatalf("deterministic soft memory must not add current-run answer-reviewer violations; got %+v", got)
	}
	if got := mu.EvidenceClosure().ActiveRepairs(); len(got) != 0 {
		t.Fatalf("deterministic soft memory must not create hard repair work; got %+v", got)
	}
}

func TestReadFailureMemoryIgnoresRetryReasonText(t *testing.T) {
	const poison = "POISON_RAW_RETRY_REASON"
	mu := types.NewMutableState("explain the code architecture")
	mu.AppendAnswerRetryEvent(string(types.StageExplore), poison)
	bus := &types.BusContext{
		Mode:    types.ModeRead,
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
		}},
	}

	pattern := buildReadFailureMemoryPattern(collectReadFailureMemorySignal(bus, mu))
	if pattern == nil {
		t.Fatal("expected a pattern from typed retry signal")
	}
	corpus := strings.Join([]string{
		pattern.Name,
		pattern.Description,
		pattern.Trigger,
		pattern.Consequence,
	}, "\n")
	if strings.Contains(corpus, poison) {
		t.Fatalf("raw retry reason leaked into deterministic memory pattern: %q", corpus)
	}
}

func TestReadFailureMemoryDoesNotDuplicateSuccessfulReviewerPattern(t *testing.T) {
	store := NewAnswerTaxonomyStore(t.TempDir(), "repo", 50, 90)
	mu := types.NewMutableState("explain the code architecture")
	mu.AppendAnswerRetryEvent(string(types.StageExplore), "retry reason should stay reviewer input only")
	reviewerPattern := validPattern("reviewer supplied architecture proof")
	reviewerPattern.AppliesToScenarios = []string{string(types.ScenarioArchitectureExplain)}
	reviewerPattern.AppliesToFamilies = []string{string(types.QFArchitecture)}
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:    types.ModeRead,
			Mutable: mu,
			AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			}},
		},
		answerTaxonomyStore: store,
		answerReviewer:      staticAnswerReviewer{pattern: reviewerPattern},
	}

	o.runAnswerReviewerOnSuccess()

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("successful reviewer should suppress deterministic fallback duplication; got %d patterns: %+v", len(all), all)
	}
	if all[0].Name != reviewerPattern.Name {
		t.Fatalf("persisted pattern name = %q, want reviewer pattern %q", all[0].Name, reviewerPattern.Name)
	}
}

func TestReadFailureMemorySkipsSingleSoftViolationWithoutRetry(t *testing.T) {
	mu := types.NewMutableState("explain the code architecture")
	mu.EvidenceClosure().AppendViolation(types.Violation{
		Kind:  types.ViolAnswerSemanticUnderfilled,
		Stage: string(types.StageFinalize),
	})
	bus := &types.BusContext{Mode: types.ModeRead, Mutable: mu}

	if pattern := buildReadFailureMemoryPattern(collectReadFailureMemorySignal(bus, mu)); pattern != nil {
		t.Fatalf("single soft violation without retry/repair should not create noisy taxonomy memory: %+v", pattern)
	}
}
