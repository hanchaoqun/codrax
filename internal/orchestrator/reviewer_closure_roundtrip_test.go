package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestReviewerRoundTrip_PlanCriticAppendViolation pins that a
// PlanCriticVerdict carrying risks lands one ViolPlanCritic per Risk
// in the EvidenceClosure ledger, attributed to Stage="plan", with
// Confidence mapped from the verdict's label.
//
// This is a unit-level test of the helper logic — it exercises the
// closure writes the planPostHook performs (T1.3 hook) without
// running the full LLM dispatch path. The hook itself is exercised
// in stage_hooks_test.go.
func TestReviewerRoundTrip_PlanCriticAppendViolation(t *testing.T) {
	mu := types.NewMutableState("user goal")
	closure := mu.EvidenceClosure()

	verdict := PlanCriticVerdict{
		Risks: []string{
			"plan modifies auth.go but does not update auth_test.go",
			"summary mentions migration but no .sql file in changes",
		},
		Confidence: "high",
	}

	confidence := PlanCriticConfidenceFloat(verdict.Confidence)
	for i, risk := range verdict.Risks {
		closure.AppendViolation(types.Violation{
			Kind:   types.ViolPlanCritic,
			Detail: risk,
			Stage:  string(types.StagePlan),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "plan_critic_risk",
				Confidence: confidence,
			},
		})
		_ = i
	}

	vs := closure.ViolationsByKind(types.ViolPlanCritic)
	if len(vs) != 2 {
		t.Fatalf("expected 2 ViolPlanCritic; got %d", len(vs))
	}
	for _, v := range vs {
		if v.Stage != "plan" {
			t.Errorf("Stage = %q, want plan", v.Stage)
		}
		if v.SuspectedRoot.Confidence != 0.9 {
			t.Errorf("Confidence = %.2f, want 0.9 (mapped from high)", v.SuspectedRoot.Confidence)
		}
	}
	snap := closure.StageHealthSnapshot()
	if got := snap["plan"].Violations; got != 2 {
		t.Errorf("snap[plan].Violations = %d, want 2", got)
	}
}

// TestReviewerRoundTrip_ReflectorObservation pins the dual-track
// helper in stage_hooks.go::appendReflectorObservationToClosure.
// One Observation in → one ViolReflectorObservation out at
// Stage="verify".
func TestReviewerRoundTrip_ReflectorObservation(t *testing.T) {
	mu := types.NewMutableState("write task")
	closure := mu.EvidenceClosure()

	appendReflectorObservationToClosure(mu, "the previous plan failed to update foo.go", 1)
	appendReflectorObservationToClosure(mu, "", 2)        // empty must be skipped
	appendReflectorObservationToClosure(mu, "  \n\t", 2)  // whitespace-only too
	appendReflectorObservationToClosure(nil, "ignored", 1) // nil mutable must not panic

	vs := closure.ViolationsByKind(types.ViolReflectorObservation)
	if len(vs) != 1 {
		t.Fatalf("expected 1 ViolReflectorObservation; got %d", len(vs))
	}
	v := vs[0]
	if v.Stage != "verify" {
		t.Errorf("Stage = %q, want verify", v.Stage)
	}
	if !strings.Contains(v.Detail, "foo.go") {
		t.Errorf("Detail must surface verbatim observation; got %q", v.Detail)
	}
	snap := closure.StageHealthSnapshot()
	if snap["verify"].Violations != 1 {
		t.Errorf("snap[verify].Violations = %d, want 1", snap["verify"].Violations)
	}
}

// TestReviewerRoundTrip_AnswerReviewerLandsInClosure pins that the
// Block 1 hook in orchestrator.go's runAnswerReview path writes the
// distilled pattern to closure as a ViolAnswerReviewerDistilled at
// Stage="finalize". We construct an AnswerPattern manually and
// exercise the closure write directly (the orchestrator path
// re-uses this same shape).
func TestReviewerRoundTrip_AnswerReviewerLandsInClosure(t *testing.T) {
	mu := types.NewMutableState("explanation request")
	closure := mu.EvidenceClosure()

	// Mimic the orchestrator's closure write after answer_reviewer
	// returns a pattern.
	closure.AppendViolation(types.Violation{
		Kind:   types.ViolAnswerReviewerDistilled,
		Detail: "answer_reviewer distilled pitfall \"shape_intent_drift\" (explanation_classified_as_value): " + "summary names the wrong shape on this Run's question family",
		Stage:  string(types.StageFinalize),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_reviewer_distilled",
			Confidence: 0.7,
		},
	})

	vs := closure.ViolationsByKind(types.ViolAnswerReviewerDistilled)
	if len(vs) != 1 {
		t.Fatalf("expected 1 ViolAnswerReviewerDistilled; got %d", len(vs))
	}
	if vs[0].Stage != "finalize" {
		t.Errorf("Stage = %q, want finalize", vs[0].Stage)
	}
	snap := closure.StageHealthSnapshot()
	if snap["finalize"].Violations != 1 {
		t.Errorf("snap[finalize].Violations = %d, want 1", snap["finalize"].Violations)
	}
}

// TestReviewerRoundTrip_AppendAnswerRetryEventBumpsClosureRetry pins
// the cross-package wiring: when MutableState records an analyze
// retry event, the closure's StageHealthSnapshot["analyze"].Retries
// MUST tick up. This is the seam Block 3's yield delta reads to
// distinguish "stage X retried" from "stage X never moved".
func TestReviewerRoundTrip_AppendAnswerRetryEventBumpsClosureRetry(t *testing.T) {
	mu := types.NewMutableState("any question")
	mu.AppendAnswerRetryEvent("analyze", "gate rejected")
	mu.AppendAnswerRetryEvent("analyze", "gate rejected again")
	mu.AppendAnswerRetryEvent("explore", "stalled")
	snap := mu.EvidenceClosure().StageHealthSnapshot()
	if snap["analyze"].Retries != 2 {
		t.Errorf("analyze.Retries = %d, want 2", snap["analyze"].Retries)
	}
	if snap["explore"].Retries != 1 {
		t.Errorf("explore.Retries = %d, want 1", snap["explore"].Retries)
	}
}

// TestPlanCriticConfidenceFloat covers the four label paths.
func TestPlanCriticConfidenceFloat(t *testing.T) {
	cases := map[string]float64{
		"high":    0.9,
		"medium":  0.6,
		"low":     0.3,
		"":        0.5,
		"weird":   0.5,
		" High ":  0.9, // trim + lowercase
		"MEDIUM":  0.6,
	}
	for in, want := range cases {
		if got := PlanCriticConfidenceFloat(in); got != want {
			t.Errorf("PlanCriticConfidenceFloat(%q) = %.2f, want %.2f", in, got, want)
		}
	}
}
