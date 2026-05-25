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
