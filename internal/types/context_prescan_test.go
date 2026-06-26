package types

import (
	"strings"
	"testing"
)

func TestMutableState_AppendPrescanToolResultUsesTypedPathDiscovery(t *testing.T) {
	mu := NewMutableState("question")
	mu.AppendPrescanToolResult(ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "ShouldNotAppear internal/agent/summary_only.go",
		PathDiscovery: &ToolPathDiscovery{
			Kind:           ToolPathDiscoveryKindGrep,
			Pattern:        "AgentName",
			Path:           "internal/types",
			FilesOnly:      true,
			ResultCount:    1,
			CandidateFiles: []string{"internal/types/enums.go"},
		},
	})

	blob := mu.PrescanSummaryBlob()
	if !strings.Contains(blob, "agentname") || !strings.Contains(blob, "internal/types/enums.go") {
		t.Fatalf("typed path discovery not projected into corpus: %q", blob)
	}
	if strings.Contains(blob, "shouldnotappear") || strings.Contains(blob, "summary_only.go") {
		t.Fatalf("rendered summary leaked into typed corpus: %q", blob)
	}
	if got := mu.PrescanRoundCount(); got != 1 {
		t.Fatalf("PrescanRoundCount=%d, want 1", got)
	}
}

func TestMutableState_AppendPrescanToolResultUsesSourceInventory(t *testing.T) {
	mu := NewMutableState("question")
	mu.AppendPrescanToolResult(ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary:  "ShouldNotAppear AgentName",
		SourceInventory: &SourceInventoryObservation{
			Active:        true,
			Complete:      true,
			Scopes:        []string{"internal/types"},
			RepoLanguages: []SourceInventoryLanguageCount{{Language: "go", Count: 3}},
			SourceClasses: []SourceInventorySourceClassCount{{
				Role:     SourcePathRoleProduction,
				Count:    3,
				Complete: true,
				Samples:  []string{"internal/types/enums.go"},
			}},
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleType,
				Complete: true,
				Members: []SourceInventoryObservationMember{{
					Name:       "AgentName",
					File:       "internal/types/enums.go",
					Line:       127,
					SupportRef: "internal/types/enums.go:127",
					Language:   "go",
				}},
			}},
		},
	})

	blob := mu.PrescanSummaryBlob()
	for _, want := range []string{"source_inventory", "agentname", "internal/types/enums.go", "support_ref=internal/types/enums.go:127", "repo_language language=go"} {
		if !strings.Contains(blob, want) {
			t.Fatalf("corpus missing %q in %q", want, blob)
		}
	}
	if strings.Contains(blob, "shouldnotappear") {
		t.Fatalf("rendered summary leaked into typed corpus: %q", blob)
	}
}
