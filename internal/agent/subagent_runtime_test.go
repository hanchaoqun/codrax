package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeSubAgentScopesDedupeAndRepoRelative(t *testing.T) {
	repo := t.TempDir()
	got, err := normalizeSubAgentScopes(&types.BusContext{RepoRoot: repo}, []string{
		filepath.Join(repo, "src"),
		"src",
		"src/",
	})
	if err != nil {
		t.Fatalf("normalizeSubAgentScopes returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "src" {
		t.Fatalf("normalized scopes = %v, want [src]", got)
	}
}

func TestNormalizeSubAgentScopesRejectsParentTraversal(t *testing.T) {
	for _, raw := range []string{"../outside", "src/../sibling", `src\..\sibling`} {
		if _, err := normalizeSubAgentScopes(&types.BusContext{RepoRoot: t.TempDir()}, []string{raw}); err == nil {
			t.Fatalf("expected parent traversal scope %q to be rejected", raw)
		}
	}
}

func TestNormalizeSubAgentScopesRejectsAbsoluteEscape(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if _, err := normalizeSubAgentScopes(&types.BusContext{RepoRoot: repo}, []string{outside}); err == nil {
		t.Fatal("expected absolute path outside repo to be rejected")
	}
}

func TestNormalizeSubAgentScopesHonorsActiveSetGate(t *testing.T) {
	gater := fakeActiveSetGater{
		paths: map[string]types.ActiveSetGateResult{
			"src": {Allowed: true, ResolvedPath: "repo-a/src"},
		},
	}
	got, err := normalizeSubAgentScopes(&types.BusContext{
		RepoRoot:   t.TempDir(),
		MultiGraph: gater,
	}, []string{"src"})
	if err != nil {
		t.Fatalf("normalizeSubAgentScopes returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "repo-a/src" {
		t.Fatalf("normalized scopes = %v, want [repo-a/src]", got)
	}
}

func TestNormalizeSubAgentScopesRejectsActiveSetDenial(t *testing.T) {
	gater := fakeActiveSetGater{
		paths: map[string]types.ActiveSetGateResult{
			"inactive/src": {
				Allowed:      false,
				RefusalProse: "that path is outside the active repository scope",
			},
		},
	}
	_, err := normalizeSubAgentScopes(&types.BusContext{
		RepoRoot:   t.TempDir(),
		MultiGraph: gater,
	}, []string{"inactive/src"})
	if err == nil {
		t.Fatal("expected active-set denial to reject sub-agent scope")
	}
	if !strings.Contains(err.Error(), "outside the active repository boundary") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeSubAgentScopesKeepsParentChildOverlapAdvisory(t *testing.T) {
	got, err := normalizeSubAgentScopes(&types.BusContext{RepoRoot: t.TempDir()}, []string{
		"src",
		"src/service",
	})
	if err != nil {
		t.Fatalf("normalizeSubAgentScopes returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("overlapping scopes should be kept advisory, got %v", got)
	}
	if !subAgentScopesOverlap(got[0], got[1]) {
		t.Fatalf("expected overlap helper to identify %v", got)
	}
}

func TestSubAgentRequestScopeStatsCountsOverlapsWithoutRejecting(t *testing.T) {
	scopeCount, overlaps := subAgentRequestScopeStats([]*types.SubAgentRequest{
		{Scope: []string{"src", "src/service"}},
		{Scope: []string{"pkg/a", "pkg/b"}},
	})
	if scopeCount != 4 {
		t.Fatalf("scopeCount = %d, want 4", scopeCount)
	}
	if overlaps != 1 {
		t.Fatalf("overlaps = %d, want 1", overlaps)
	}
}

func TestCollectSubAgentRuntimeStatsCountsToolsAndDuplicates(t *testing.T) {
	stats := collectSubAgentRuntimeStats([]*types.SubAgentResult{
		{
			RequestID:          "r1",
			SubAgent:           "explorer",
			Facts:              []types.RepoFact{{Key: "k"}},
			EvidenceItems:      []types.EvidenceItem{{Subject: "A"}},
			FlowFindings:       []types.FlowFindingDigest{{ID: "flow-1"}},
			InvestigationNotes: []string{"rich prose"},
			Output:             []byte(`{"ok":true}`),
			Tools: []types.ToolResult{
				{ToolName: "read_file", Summary: "[a.go:1-10]", Success: true},
				{ToolName: "grep", Summary: "a.go:3: foo", Success: true},
				{ToolName: "repo_map", Summary: "source inventory", Success: true},
			},
		},
		{
			RequestID: "r2",
			SubAgent:  "explorer",
			Tools: []types.ToolResult{
				{ToolName: "read_file", Summary: "[a.go:1-10]", Success: true},
				{ToolName: "grep", Summary: "a.go:3: foo", Success: false},
			},
		},
		nil,
	})
	if stats.Branches != 3 || stats.OK != 2 || stats.Errors != 1 {
		t.Fatalf("branch status stats = %+v", stats)
	}
	if stats.Tools != 5 || stats.SuccessfulTools != 4 || stats.FailedTools != 1 {
		t.Fatalf("tool stats = %+v", stats)
	}
	if stats.DuplicateToolResults != 2 || stats.DuplicateReadFile != 1 || stats.DuplicateGrep != 1 {
		t.Fatalf("duplicate stats = %+v", stats)
	}
	if stats.RepoMapTools != 1 || stats.Facts != 1 || stats.Evidence != 1 || stats.Flow != 1 || stats.AdvisoryNotes != 1 || stats.Outputs != 1 {
		t.Fatalf("content stats = %+v", stats)
	}
}

type fakeActiveSetGater struct {
	paths map[string]types.ActiveSetGateResult
}

func (f fakeActiveSetGater) ResolveActiveSetPath(_ *types.BusContext, _ string, llmPath string, _ func(absPath string) bool) types.ActiveSetGateResult {
	if res, ok := f.paths[llmPath]; ok {
		return res
	}
	return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
}

func (f fakeActiveSetGater) ResolveActiveSetCommand(_ *types.BusContext, _ string, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true}
}
