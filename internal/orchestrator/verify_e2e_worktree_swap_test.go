package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// End-to-end ModeVerify against a preserved worktree. Locks the
// /verify swap fix (911b787): the verifier MUST see RepoRoot ==
// preserved-worktree, not the main repo. Walking through:
//
//  1. Build a real git repo with main.go containing "main-repo".
//  2. Build a separate "preserved worktree" dir with main.go
//     containing "applied-bytes".
//  3. SetMode(ModeVerify) + SetReuseWorktreePath(preserved).
//  4. Run() — analyzer fires as classifier, write graph emits
//     verify-only node, verifyPreHook swaps RepoRoot.
//  5. Mock verifier reads main.go via ctx.Mutable.RepoRoot() —
//     MUST see "applied-bytes" (proves the swap landed).
//
// Without 911b787 this test would see "main-repo" because the
// orchestrator never told the verifier where the preserved tree
// was — it ran tests against the main repo.
func TestE2E_VerifyMode_SwapsToPreservedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	mainRepo := setupGitFixture(t, "main.go", `package main
func main() { _ = "main-repo" }
`)
	// Pretend this is a preserved worktree from a prior apply.
	preserved := t.TempDir()
	if err := os.WriteFile(filepath.Join(preserved, "main.go"),
		[]byte(`package main
func main() { _ = "applied-bytes" }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// The verifier mock reads main.go from ctx.Mutable.RepoRoot()
	// and reports back via ChangeReport.FailureSummary so the test
	// can assert which dir was active.
	var verifierSawPath string
	var verifierSawContent string
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentVerifier: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			verifierSawPath = ctx.Mutable.RepoRoot()
			body, err := os.ReadFile(filepath.Join(verifierSawPath, "main.go"))
			if err != nil {
				ctx.Mutable.SetChangeReport(&types.ChangeReport{Passed: false, FailureSummary: "read failed"})
				return &agent.StageOutput{MissingPiece: types.MissingFacts, Error: "read failed: " + err.Error()}, nil
			}
			verifierSawContent = string(body)
			ctx.Mutable.SetChangeReport(&types.ChangeReport{
				PlanID:                "plan-verify-e2e",
				Passed:                true,
				TestResults:           []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "swap_check", Passed: true}},
				RegressionAssertions:  []string{},
				PreexistingAssertions: []string{},
				FixedAssertions:       []string{},
			})
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModeVerify)

	// Pre-seed the plan onto Mutable directly via a saved plan file —
	// verifyPreHook will load it. We craft the plan to point its
	// WorktreePath at the preserved dir, exercising the auto-pickup
	// path (the same path REPL /verify and CLI both rely on).
	planDir := t.TempDir()
	planPath := filepath.Join(planDir, "plan.json")
	writePlanJSON(t, planPath, &types.ChangePlan{
		ID:           "plan-verify-e2e",
		Request:      "verify reuse",
		Summary:      "auto-pickup test",
		Changes:      []types.FileChange{{Path: "main.go", Kind: "modify", Rationale: "x"}},
		TargetPaths:  []string{"main.go"},
		Status:       types.PlanStatusApplied,
		WorktreePath: preserved,
		CreatedAt:    mustParseRFC3339(t, "2026-01-01T00:00:00Z"),
	})
	o.SetPlanPath(planPath)

	busCtx, err := o.Run("/verify plan-verify-e2e", mainRepo, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if verifierSawPath != preserved {
		t.Errorf("verifier ran against %q; want preserved worktree %q", verifierSawPath, preserved)
	}
	if verifierSawContent == "" {
		t.Fatalf("verifier saw empty content (read failed)")
	}
	if !stringContains(verifierSawContent, "applied-bytes") {
		t.Errorf("verifier saw main-repo bytes instead of applied-bytes; content=%q", verifierSawContent)
	}
	// Outer worktree-cleanup defer must NOT discard the preserved
	// tree — it's what the user wanted to keep around.
	if _, err := os.Stat(preserved); err != nil {
		t.Errorf("preserved worktree was discarded by Run() cleanup; stat err = %v", err)
	}
	// LastError must be empty (verify passed).
	if busCtx.TaskState.LastError != "" {
		t.Errorf("verify success should leave LastError empty; got %q", busCtx.TaskState.LastError)
	}
}

func stringContains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func mustParseRFC3339(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return ts
}
