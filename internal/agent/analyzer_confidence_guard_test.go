package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestExplorer_ShouldStartDeclarativeDepth_HighConfidence proves the
// schema-v4 confidence guard does NOT block narrowing on a confident
// classification — preserves the post-0e23263 behaviour.
func TestExplorer_ShouldStartDeclarativeDepth_HighConfidence(t *testing.T) {
	e := &explorerEvaluator{
		declarativeAnchorFiles: []string{"internal/topology.go"},
		predicateAxis:          types.AxisRegister,
		kindConfidence:         0.9,
	}
	if !e.shouldStartDeclarativeDepth("registration") {
		t.Error("high-confidence registration should start declarative depth")
	}
}

// TestExplorer_ShouldStartDeclarativeDepth_LowConfidenceBlocks proves
// the v4 guard suppresses aggressive narrowing on a hesitant
// classification — the explorer keeps the broader read set so a
// misclassification cannot drop answer-bearing files.
func TestExplorer_ShouldStartDeclarativeDepth_LowConfidenceBlocks(t *testing.T) {
	e := &explorerEvaluator{
		declarativeAnchorFiles: []string{"internal/topology.go"},
		predicateAxis:          types.AxisRegister,
		kindConfidence:         0.4,
	}
	if e.shouldStartDeclarativeDepth("registration") {
		t.Error("low-confidence kind must block declarative-depth narrowing")
	}
}

// TestExplorer_ShouldStartDeclarativeDepth_ZeroConfidenceUnconstrained:
// kindConfidence=0 means the LLM did not emit the field — preserves
// pre-v4 behaviour (no guard) so existing unit tests / sub-agents
// that bypass the analyzer still work without setting the field.
func TestExplorer_ShouldStartDeclarativeDepth_ZeroConfidenceUnconstrained(t *testing.T) {
	e := &explorerEvaluator{
		declarativeAnchorFiles: []string{"internal/topology.go"},
		predicateAxis:          types.AxisRegister,
		kindConfidence:         0, // unset — no guard
	}
	if !e.shouldStartDeclarativeDepth("registration") {
		t.Error("zero confidence (unset) should preserve pre-v4 behaviour")
	}
}

// TestExplorer_ShouldStartDeclarativeDepth_BoundaryAtFloor: exactly
// at the floor must allow narrowing. The strict-less-than comparison
// in the guard is the documented contract.
func TestExplorer_ShouldStartDeclarativeDepth_BoundaryAtFloor(t *testing.T) {
	e := &explorerEvaluator{
		declarativeAnchorFiles: []string{"internal/topology.go"},
		predicateAxis:          types.AxisRegister,
		kindConfidence:         0.7,
	}
	if !e.shouldStartDeclarativeDepth("registration") {
		t.Error("kindConfidence == floor should NOT block (strict < contract)")
	}
}
