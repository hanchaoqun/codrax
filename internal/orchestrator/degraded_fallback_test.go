package orchestrator

import (
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildDegradedFallbackIR_HasFinalizeNode pins the commit-59
// Batch E.1 fix (audit HIGH #9): when analyze exhausts retries,
// the orchestrator falls back to a minimal IR with a single
// finalize node so phase 2 can produce a degraded-but-honest
// reply. Pre-fix the Run died with empty Result and the user got
// nothing.
func TestBuildDegradedFallbackIR_HasFinalizeNode(t *testing.T) {
	ir := buildDegradedFallbackIR("how does X work?", errors.New("exhausted"))
	if ir == nil {
		t.Fatal("fallback IR is nil")
	}
	if len(ir.TaskGraph.Nodes) != 1 {
		t.Errorf("expected 1 node (finalize); got %d", len(ir.TaskGraph.Nodes))
	}
	if ir.TaskGraph.Nodes[0].Type != types.NodeFinalize {
		t.Errorf("expected NodeFinalize; got %s", ir.TaskGraph.Nodes[0].Type)
	}
}

// TestBuildDegradedFallbackIR_PreservesObjective ensures the
// original user request is plumbed through so the finalizer can
// quote what was asked.
func TestBuildDegradedFallbackIR_PreservesObjective(t *testing.T) {
	ir := buildDegradedFallbackIR("explain the explorer module", errors.New("e"))
	if !strings.Contains(ir.RequestModel.RawRequest, "explorer module") {
		t.Errorf("RawRequest missing original text: %q", ir.RequestModel.RawRequest)
	}
}

// TestBuildDegradedFallbackIR_ContractDoesNotBlockReply pins that
// the fallback's AnswerContract has no required-citation floor:
// otherwise finalize would fail another gate when explore never
// produced citations.
func TestBuildDegradedFallbackIR_ContractDoesNotBlockReply(t *testing.T) {
	ir := buildDegradedFallbackIR("q", errors.New("e"))
	if ir.AnswerContract.CitationReq.Required {
		t.Error("fallback contract must NOT require citations — explore never ran")
	}
	if ir.AnswerContract.RequiredAnswerShape != types.ShapeExplanation {
		t.Errorf("expected ShapeExplanation; got %s", ir.AnswerContract.RequiredAnswerShape)
	}
}

// TestLearningFailures_ResetAndAppend pins the commit-59 Batch
// E.1 audit-HIGH-#12 telemetry: failures are recorded per Run +
// reset between Runs.
func TestLearningFailures_ResetAndAppend(t *testing.T) {
	mu := types.NewMutableState("test")
	if got := mu.LearningFailures(); got != nil {
		t.Errorf("fresh Mutable should have no failures; got %v", got)
	}
	mu.AppendLearningFailure("reflector", "boom")
	mu.AppendLearningFailure("answer_reviewer", "quota exceeded")
	failures := mu.LearningFailures()
	if len(failures) != 2 {
		t.Fatalf("expected 2 failures; got %d", len(failures))
	}
	if failures[0].Stage != "reflector" || failures[1].Reason != "quota exceeded" {
		t.Errorf("failure record drift: %+v", failures)
	}
	mu.ResetLearningFailures()
	if got := mu.LearningFailures(); got != nil {
		t.Errorf("Reset should clear failures; got %v", got)
	}
}

// TestLearningFailures_NilSafe pins the back-compat: nil receiver
// behaviour matches other Mutable APIs.
func TestLearningFailures_NilSafe(t *testing.T) {
	var mu *types.MutableState
	mu.AppendLearningFailure("x", "y") // must not panic
	if got := mu.LearningFailures(); got != nil {
		t.Errorf("nil receiver should return nil; got %v", got)
	}
	mu.ResetLearningFailures() // must not panic
}
