package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Pin for the Q5-A citation-hygiene assertion: evidence sources under
// codrax's own runtime-state directory (.codrax — blob refs, logs,
// worktrees) never become current-source citations, in BOTH spellings:
//   - relative ".codrax/..." (CWD == repo root: the canonicaliser strips
//     the root and leaves the relative form), and
//   - absolute "/…/.codrax/..." (CWD != repo root: blob paths live under
//     runtimeAnchor outside the repo and stay absolute after
//     canonicalisation).
func TestExtractorEvidenceRepoSource_DropsCodraxRuntimeStatePaths(t *testing.T) {
	repoRoot := "/repo/root"
	ctx := &types.AgentContext{RepoRoot: repoRoot}

	drops := []string{
		".codrax/blob/20260704-120000-000-1/trace-query-result-ef56ab78.json",
		repoRoot + "/.codrax/blob/20260704-120000-000-1/trace_query-ab12cd34.txt",
		"/elsewhere/anchor/.codrax/blob/s1/trace-query-result-ef56ab78.json",
		`\elsewhere\anchor\.codrax\blob\s1\trace_query-ab12cd34.txt`,
	}
	for _, source := range drops {
		if got := extractorEvidenceRepoSource(ctx, source); got != "" {
			t.Fatalf("runtime-state path %q must not become a citation source, got %q", source, got)
		}
		if got := extractorEvidenceRepoSource(nil, source); got != "" {
			t.Fatalf("runtime-state path %q must not become a citation source without ctx, got %q", source, got)
		}
	}

	keeps := map[string]string{
		"internal/tracequery/query.go":        "internal/tracequery/query.go",
		repoRoot + "/internal/agent/agent.go": "internal/agent/agent.go",
	}
	for source, want := range keeps {
		if got := extractorEvidenceRepoSource(ctx, source); got != want {
			t.Fatalf("repo source %q should canonicalise to %q, got %q", source, want, got)
		}
	}
}
