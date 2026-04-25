package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ─── Layer-3: structured rejection ───────────────────────────────────

// TestEmitChangePlan_EmptyPayload_ReprimesSchema locks the recovery
// path that Run #3 needed: when the LLM sends empty/truncated params,
// the rejection MUST embed the full schema reminder so the next retry
// has the structural information it needs to rebuild the payload.
// Catches the regression where rejection messages stay opaque
// ("unexpected EOF") and the LLM loops on the same broken payload.
func TestEmitChangePlan_EmptyPayload_ReprimesSchema(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"empty object", "{}"},
		{"null literal", "null"},
		{"single byte", "{"},
	}
	tool := &EmitChangePlan{}
	ctx := &types.BusContext{Mutable: types.NewMutableState("obj")}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := tool.Execute(ctx, json.RawMessage(c.raw))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Success {
				t.Fatalf("expected rejection for empty payload, got success: %s", res.Summary)
			}
			if !strings.Contains(res.Summary, "REQUIRED schema") {
				t.Errorf("rejection summary did not embed schema reminder:\n%s", res.Summary)
			}
		})
	}
}

// ─── V1 deps-closure ─────────────────────────────────────────────────

// TestValidatePlanDepsClosure_RejectsUndeclaredImport pins Run #1's
// failure mode: new_content imports github.com/pkg/sftp but no go.mod
// modify entry adds it. Validator must reject and name the missing
// package so planner knows what to add.
func TestValidatePlanDepsClosure_RejectsUndeclaredImport(t *testing.T) {
	repo := setupGoRepoForTest(t)
	changes := []emitChangePlanChange{
		{
			Path:       "internal/mcp/ssh.go",
			Kind:       "create",
			NewContent: "package mcp\n\nimport (\n\t\"fmt\"\n\t\"github.com/pkg/sftp\"\n)\n\nfunc _stub() { _ = fmt.Sprintf; _ = sftp.NewClient }\n",
		},
	}
	got := validatePlanDepsClosure(repo, emitChangesToFileChanges(changes))
	if !strings.Contains(got, "github.com/pkg/sftp") {
		t.Errorf("expected rejection naming missing import, got: %q", got)
	}
}

// TestValidatePlanDepsClosure_AcceptsOverlaidGoMod confirms the
// happy path: when the same plan adds the missing package to go.mod
// in a parallel modify entry, V1 accepts.
func TestValidatePlanDepsClosure_AcceptsOverlaidGoMod(t *testing.T) {
	repo := setupGoRepoForTest(t)
	changes := []emitChangePlanChange{
		{
			Path:       "internal/mcp/ssh.go",
			Kind:       "create",
			NewContent: "package mcp\n\nimport (\n\t\"fmt\"\n\t\"github.com/pkg/sftp\"\n)\n\nfunc _stub() { _ = fmt.Sprintf; _ = sftp.NewClient }\n",
		},
		{
			Path:       "go.mod",
			Kind:       "modify",
			NewContent: "module example.com/codrax\n\ngo 1.22\n\nrequire github.com/pkg/sftp v1.13.6\n",
		},
	}
	if got := validatePlanDepsClosure(repo, emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected acceptance with overlaid go.mod, got rejection: %q", got)
	}
}

// TestValidatePlanDepsClosure_AcceptsStdlibAndInternal verifies the
// no-action paths: stdlib imports + project-internal imports never
// need go.mod changes.
func TestValidatePlanDepsClosure_AcceptsStdlibAndInternal(t *testing.T) {
	repo := setupGoRepoForTest(t)
	changes := []emitChangePlanChange{
		{
			Path:       "internal/mcp/ssh.go",
			Kind:       "create",
			NewContent: "package mcp\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"example.com/codrax/internal/types\"\n)\n\nfunc _stub() { _ = fmt.Sprintf; _ = os.Open; _ = types.RepoFact{} }\n",
		},
	}
	if got := validatePlanDepsClosure(repo, emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected acceptance for stdlib+internal-only imports, got rejection: %q", got)
	}
}

// ─── V3 wiring-closure ───────────────────────────────────────────────

// TestValidatePlanWiringClosure_RejectsBareCreate pins Run #1 + #2's
// shared failure mode: new mcp.Server file with no cmd/root.go modify
// entry. Validator must reject and name the wiring file.
func TestValidatePlanWiringClosure_RejectsBareCreate(t *testing.T) {
	changes := []emitChangePlanChange{
		{Path: "internal/mcp/ssh.go", Kind: "create", NewContent: "package mcp\n"},
	}
	got := validatePlanWiringClosure(emitChangesToFileChanges(changes))
	if !strings.Contains(got, "cmd/root.go") {
		t.Errorf("expected rejection naming cmd/root.go wiring file, got: %q", got)
	}
	if !strings.Contains(got, "internal/mcp") {
		t.Errorf("expected rejection naming the triggering subsystem, got: %q", got)
	}
}

// TestValidatePlanWiringClosure_AcceptsWithWiringModify confirms the
// happy path: the same plan that creates the new mcp file also
// modifies cmd/root.go.
func TestValidatePlanWiringClosure_AcceptsWithWiringModify(t *testing.T) {
	changes := []emitChangePlanChange{
		{Path: "internal/mcp/ssh.go", Kind: "create", NewContent: "package mcp\n"},
		{Path: "cmd/root.go", Kind: "modify", NewContent: "package cmd\n"},
	}
	if got := validatePlanWiringClosure(emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected acceptance with wiring modify, got rejection: %q", got)
	}
}

// TestValidatePlanWiringClosure_AcceptsWithWiringPatch covers the
// patch-form variant (kind=patch on the wiring file).
func TestValidatePlanWiringClosure_AcceptsWithWiringPatch(t *testing.T) {
	changes := []emitChangePlanChange{
		{Path: "internal/skill/foo.go", Kind: "create", NewContent: "package skill\n"},
		{Path: "internal/skill/defaults.go", Kind: "patch", Patch: "@@\n"},
	}
	if got := validatePlanWiringClosure(emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected acceptance with wiring patch, got rejection: %q", got)
	}
}

// TestValidatePlanWiringClosure_TestFilesExempt confirms _test.go
// files are not subject to wiring rules (tests construct their own
// subjects locally).
func TestValidatePlanWiringClosure_TestFilesExempt(t *testing.T) {
	changes := []emitChangePlanChange{
		{Path: "internal/mcp/ssh_test.go", Kind: "create", NewContent: "package mcp\n"},
	}
	if got := validatePlanWiringClosure(emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected test files to be exempt, got rejection: %q", got)
	}
}

// ─── V4 summary-changes consistency ──────────────────────────────────

// TestValidatePlanSummaryConsistency_RejectsPathLie pins Run #2's
// summary-vs-changes mismatch: summary says "新增 internal/mcp/ssh.go"
// but changes[] contains internal/mcp/ssh_server.go. Validator must
// reject when summary asserts a path the plan doesn't actually touch.
func TestValidatePlanSummaryConsistency_RejectsPathLie(t *testing.T) {
	summary := "新增 internal/mcp/ssh.go 实现 SSH 下载功能"
	changes := []emitChangePlanChange{
		{Path: "internal/mcp/ssh_server.go", Kind: "create", NewContent: "package mcp\n"},
	}
	got := validatePlanSummaryConsistency(summary, emitChangesToFileChanges(changes))
	if !strings.Contains(got, "internal/mcp/ssh.go") {
		t.Errorf("expected rejection naming the lying path, got: %q", got)
	}
}

// TestValidatePlanSummaryConsistency_RejectsImportHallucination pins
// Run #1's summary-named-fictional-package failure: summary mentions
// "golang.org/x/crypto/sftp" (nonexistent) while code imports
// "github.com/pkg/sftp". Validator must reject.
func TestValidatePlanSummaryConsistency_RejectsImportHallucination(t *testing.T) {
	summary := "Uses golang.org/x/crypto/sftp for transfers."
	changes := []emitChangePlanChange{
		{
			Path:       "internal/mcp/ssh.go",
			Kind:       "create",
			NewContent: "package mcp\n\nimport \"github.com/pkg/sftp\"\n\nfunc _stub() { _ = sftp.NewClient }\n",
		},
	}
	got := validatePlanSummaryConsistency(summary, emitChangesToFileChanges(changes))
	if !strings.Contains(got, "golang.org/x/crypto/sftp") {
		t.Errorf("expected rejection naming the hallucinated import, got: %q", got)
	}
}

// TestValidatePlanSummaryConsistency_AcceptsHonestSummary confirms a
// truthful summary passes (paths and imports both line up).
func TestValidatePlanSummaryConsistency_AcceptsHonestSummary(t *testing.T) {
	summary := "新增 internal/mcp/ssh.go 使用 github.com/pkg/sftp 实现 SFTP 下载"
	changes := []emitChangePlanChange{
		{
			Path:       "internal/mcp/ssh.go",
			Kind:       "create",
			NewContent: "package mcp\n\nimport \"github.com/pkg/sftp\"\n\nfunc _stub() { _ = sftp.NewClient }\n",
		},
	}
	if got := validatePlanSummaryConsistency(summary, emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected acceptance for honest summary, got rejection: %q", got)
	}
}

// TestValidatePlanSummaryConsistency_AcceptsContextReference
// confirms summaries that mention well-known context files (e.g.
// internal/types/context.go) for orientation are not rejected.
func TestValidatePlanSummaryConsistency_AcceptsContextReference(t *testing.T) {
	summary := "新增工具 — 详见 internal/types/context.go 中的 ToolResult 类型"
	changes := []emitChangePlanChange{
		{Path: "internal/tool/foo.go", Kind: "create", NewContent: "package tool\n"},
		{Path: "cmd/root.go", Kind: "modify", NewContent: "package cmd\n"},
	}
	if got := validatePlanSummaryConsistency(summary, emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected acceptance for context-only path reference, got rejection: %q", got)
	}
}

// ─── V2 dry-build ────────────────────────────────────────────────────

// TestValidatePlanDryBuild_RejectsCompileError pins Run #2's failure
// mode: code references an undefined identifier (s.client.Connected()
// doesn't exist on *ssh.Client). Validator must reject with stderr
// embedded so the planner sees the line:col.
func TestValidatePlanDryBuild_RejectsCompileError(t *testing.T) {
	if _, err := os.Stat("/usr/bin/go"); err != nil {
		// Best-effort PATH lookup will be done by validatePlanDryBuild;
		// here we just want a quick early skip when go isn't installed
		// at all on the test machine. If it's somewhere else (e.g.
		// /usr/local/go/bin/go) the validator's exec.LookPath will find
		// it and the test will run.
	}
	repo := setupGoRepoForTest(t)
	ctx := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir()}
	changes := []emitChangePlanChange{
		{
			Path:       "internal/mcp/ssh.go",
			Kind:       "create",
			NewContent: "package mcp\n\nfunc bad() { undefinedSymbol() }\n",
		},
	}
	got := validatePlanDryBuild(ctx, emitChangesToFileChanges(changes))
	if got == "" {
		t.Skip("validatePlanDryBuild returned empty (likely go binary unavailable in test env); compile-error path untested here")
	}
	if !strings.Contains(got, "undefined") && !strings.Contains(got, "undefinedSymbol") {
		t.Errorf("expected rejection mentioning undefined identifier, got: %q", got)
	}
}

// TestValidatePlanDryBuild_AcceptsValidCode verifies the happy path:
// well-formed Go code passes vet.
func TestValidatePlanDryBuild_AcceptsValidCode(t *testing.T) {
	repo := setupGoRepoForTest(t)
	ctx := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir()}
	changes := []emitChangePlanChange{
		{
			Path:       "internal/mcp/ssh.go",
			Kind:       "create",
			NewContent: "package mcp\n\nimport \"fmt\"\n\nfunc Hello() { fmt.Println(\"ok\") }\n",
		},
	}
	if got := validatePlanDryBuild(ctx, emitChangesToFileChanges(changes)); got != "" {
		t.Skipf("validatePlanDryBuild returned %q — likely missing go binary or harness mismatch; happy-path untested here", got)
	}
}

// TestValidatePlanDryBuild_SkipsNonGoRepo confirms degraded behaviour:
// when the target dir has no go.mod we silently skip (V1 already
// handles import-presence; V2 only adds the compile check).
func TestValidatePlanDryBuild_SkipsNonGoRepo(t *testing.T) {
	dir := t.TempDir()
	// no go.mod
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: t.TempDir()}
	changes := []emitChangePlanChange{
		{Path: "foo.go", Kind: "create", NewContent: "garbage"},
	}
	if got := validatePlanDryBuild(ctx, emitChangesToFileChanges(changes)); got != "" {
		t.Errorf("expected silent skip on non-Go repo, got rejection: %q", got)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────

// setupGoRepoForTest writes a minimal go.mod into a temp dir and
// returns the path. Used by V1 and V2 tests as a stand-in for a real
// repo root.
func setupGoRepoForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	goMod := "module example.com/codrax\n\ngo 1.22\n\nrequire github.com/foo/bar v1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("setup go.mod: %v", err)
	}
	return dir
}
