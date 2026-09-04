package cmd

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInCmdPrompts is cmd's marker in the glossarylint
// renderer roster (§40.52): the memory summarizer system prompt and
// tool schema live in root.go next to operator-facing CLI text, so the
// package uses the shape-bound prompt-surface lane.
func TestNoInternalTermsInCmdPrompts(t *testing.T) {
	glossarylint.RunPromptSurfaceScan(t, ".")
}
