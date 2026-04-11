package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLogicalFactSource(t *testing.T) {
	got := logicalFactSource("[internal/context/builder.go: showing lines 1-20 of 200 total]", "read_file")
	if got != "internal/context/builder.go" {
		t.Fatalf("logicalFactSource() = %q, want internal/context/builder.go", got)
	}

	fallback := logicalFactSource("[grep: 3 matching lines]", "grep")
	if fallback != "tool:grep" {
		t.Fatalf("logicalFactSource() fallback = %q, want tool:grep", fallback)
	}
}

func TestExplorerAndSubExplorerFactsUseLogicalSourceAndEvidenceRef(t *testing.T) {
	toolResults := []types.ToolResult{
		{
			ToolName: "read_file",
			Summary:  "[internal/context/builder.go: showing lines 1-40 of 420 total]\npackage context\n...",
			RawRef:   "blob://trace/tool-result-1.txt",
			Success:  true,
		},
	}
	msgs := []llm.Message{{Role: "assistant", Content: "done"}}

	explorer := &explorerEvaluator{}
	mainOut, err := explorer.ParseOutput(&types.AgentContext{}, msgs, toolResults, nil)
	if err != nil {
		t.Fatalf("explorer ParseOutput error: %v", err)
	}
	if len(mainOut.NewFacts) != 1 {
		t.Fatalf("explorer facts = %d, want 1", len(mainOut.NewFacts))
	}
	if mainOut.NewFacts[0].Source != "internal/context/builder.go" {
		t.Fatalf("explorer fact source = %q, want logical path", mainOut.NewFacts[0].Source)
	}
	if mainOut.NewFacts[0].EvidenceRef != "blob://trace/tool-result-1.txt" {
		t.Fatalf("explorer fact evidence_ref = %q", mainOut.NewFacts[0].EvidenceRef)
	}

	sub := &subExplorerEvaluator{}
	subOut, err := sub.ParseOutput(&types.AgentContext{}, msgs, toolResults, nil)
	if err != nil {
		t.Fatalf("sub-explorer ParseOutput error: %v", err)
	}
	if len(subOut.NewFacts) != 1 {
		t.Fatalf("sub-explorer facts = %d, want 1", len(subOut.NewFacts))
	}
	if subOut.NewFacts[0].Source != "internal/context/builder.go" {
		t.Fatalf("sub-explorer fact source = %q, want logical path", subOut.NewFacts[0].Source)
	}
	if subOut.NewFacts[0].EvidenceRef != "blob://trace/tool-result-1.txt" {
		t.Fatalf("sub-explorer fact evidence_ref = %q", subOut.NewFacts[0].EvidenceRef)
	}
}
