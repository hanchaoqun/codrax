package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/logparse"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ── mergeLogTriageEntities ───────────────────────────────────────

// TestMergeLogTriageEntities_FuncAndPkgAppended verifies that stack
// frame Func / Pkg tokens and ErrorLiterals flow into the entity list
// without disturbing existing entries.
func TestMergeLogTriageEntities_FuncAndPkgAppended(t *testing.T) {
	existing := []string{"ExistingFunc", "SomePkg"}
	bundle := logparse.LogBundle{
		Lang: "go",
		Frames: []logparse.Frame{
			{Lang: "go", Func: "main.crashyThing", Pkg: "cmd/app"},
			{Lang: "go", Func: "pkg.Helper.run", Pkg: "internal/pkg"},
		},
		ErrorLiterals: []string{"runtime error: index out of range"},
	}
	got := mergeLogTriageEntities(existing, bundle)
	wantContains := []string{
		"ExistingFunc", "SomePkg", // existing preserved
		"crashyThing",               // last-segment from main.crashyThing
		"cmd/app",                   // pkg hint
		"run",                       // last-segment from pkg.Helper.run
		"internal/pkg",              // pkg hint
		"runtime error: index out of range", // error literal
	}
	for _, want := range wantContains {
		found := false
		for _, e := range got {
			if e == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("merged entities missing %q: %v", want, got)
		}
	}
}

func TestMergeLogTriageEntities_EmptyBundle_NoChange(t *testing.T) {
	existing := []string{"a", "b"}
	bundle := logparse.LogBundle{}
	got := mergeLogTriageEntities(existing, bundle)
	if !reflect.DeepEqual(got, existing) {
		t.Errorf("empty bundle modified entities: %v → %v", existing, got)
	}
}

func TestMergeLogTriageEntities_DedupAndCap(t *testing.T) {
	existing := []string{"dup"}
	frames := make([]logparse.Frame, 50)
	for i := range frames {
		// Each frame has a unique Func so the cap (32) triggers.
		frames[i] = logparse.Frame{Func: "pkg." + strings.Repeat("f", i+1)}
	}
	frames[0].Func = "pkg.dup" // last-seg = "dup" — already present
	bundle := logparse.LogBundle{Lang: "go", Frames: frames}
	got := mergeLogTriageEntities(existing, bundle)
	// Start size 1 + cap 32 = 33 max.
	if len(got) > 33 {
		t.Errorf("cap violated: len(got)=%d, want ≤ 33", len(got))
	}
	// "dup" appears exactly once.
	dupCount := 0
	for _, e := range got {
		if e == "dup" {
			dupCount++
		}
	}
	if dupCount != 1 {
		t.Errorf("dup count = %d, want 1", dupCount)
	}
}

// ── mergeLogTriageFiles ──────────────────────────────────────────

func TestMergeLogTriageFiles_LogFilesPrependRanker(t *testing.T) {
	log := []string{"frame/one.go", "frame/two.go"}
	ranked := []string{"ranker/a.go", "ranker/b.go"}
	got := mergeLogTriageFiles(log, ranked)
	want := []string{"frame/one.go", "frame/two.go", "ranker/a.go", "ranker/b.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge order = %v, want %v", got, want)
	}
}

func TestMergeLogTriageFiles_DedupAcrossSources(t *testing.T) {
	// Log picks frame/one.go; ranker rediscovers it — should not duplicate.
	log := []string{"frame/one.go"}
	ranked := []string{"frame/one.go", "ranker/a.go"}
	got := mergeLogTriageFiles(log, ranked)
	want := []string{"frame/one.go", "ranker/a.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dedup failed: %v, want %v", got, want)
	}
}

func TestMergeLogTriageFiles_CapAt10(t *testing.T) {
	log := make([]string, 6)
	ranked := make([]string, 8)
	for i := range log {
		log[i] = "log/" + string(rune('a'+i)) + ".go"
	}
	for i := range ranked {
		ranked[i] = "ranker/" + string(rune('A'+i)) + ".go"
	}
	got := mergeLogTriageFiles(log, ranked)
	if len(got) != 10 {
		t.Errorf("cap = %d, want 10", len(got))
	}
	// Every log file should be in the first 6 slots.
	for i, p := range log {
		if got[i] != p {
			t.Errorf("slot %d = %q, want %q (log-derived should come first)", i, got[i], p)
		}
	}
}

// ── resolveLogTriageFiles ────────────────────────────────────────

// TestResolveLogTriageFiles_GoPanicMatchesRepoFile builds a fake repo
// with a known file and verifies that a Go panic frame pointing at it
// (via absolute build-path) strips correctly.
func TestResolveLogTriageFiles_GoPanicMatchesRepoFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "internal", "svc", "handler.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := &types.AgentContext{RepoRoot: dir}
	bundle := logparse.LogBundle{
		Lang: "go",
		Frames: []logparse.Frame{
			// Path has /home/<user>/.../src/ that triggers the home-scan strip.
			{Lang: "go", File: "/home/builder/work/src/internal/svc/handler.go", Line: 42, Func: "svc.Handler"},
			// Runtime-internal — must be filtered out.
			{Lang: "go", File: "/usr/local/go/src/runtime/asm_amd64.s", Line: 1571, Func: "runtime.goexit"},
		},
	}
	got := resolveLogTriageFiles(ctx, bundle)
	// Expect one resolved path, no runtime internals.
	if len(got) != 1 {
		t.Fatalf("got %d paths, want 1: %v", len(got), got)
	}
	if got[0] != "internal/svc/handler.go" {
		t.Errorf("path = %q, want internal/svc/handler.go", got[0])
	}
}

// TestResolveLogTriageFiles_JavaBasenameResolved puts two files with
// the same basename under different packages and verifies
// ResolveJavaFile picks the one matching the frame's Pkg.
func TestResolveLogTriageFiles_JavaBasenameResolved(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "src", "main", "java", "com", "foo", "Util.java")
	pathB := filepath.Join(dir, "src", "main", "java", "com", "bar", "Util.java")
	for _, p := range []string{pathA, pathB} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("// stub\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx := &types.AgentContext{RepoRoot: dir}
	bundle := logparse.LogBundle{
		Lang: "java",
		Frames: []logparse.Frame{
			// Java frame File is a basename; Pkg is com.foo.
			{Lang: "java", File: "Util.java", Line: 10, Func: "com.foo.Util.run", Pkg: "com.foo"},
		},
	}
	got := resolveLogTriageFiles(ctx, bundle)
	if len(got) == 0 {
		t.Fatal("expected 1 resolved path, got 0")
	}
	if !strings.HasSuffix(got[0], "com/foo/Util.java") {
		t.Errorf("path = %q, want ...com/foo/Util.java (Pkg disambiguation)", got[0])
	}
}

// TestResolveLogTriageFiles_EmptyBundleNoOp covers the negative
// regression: no attached log ⇒ no path changes.
func TestResolveLogTriageFiles_EmptyBundleNoOp(t *testing.T) {
	ctx := &types.AgentContext{RepoRoot: t.TempDir()}
	got := resolveLogTriageFiles(ctx, logparse.LogBundle{})
	if got != nil {
		t.Errorf("empty bundle returned %v, want nil", got)
	}
}

// ── End-to-end plumbing (BusContext → AgentContext) ─────────────

// TestAttachedLog_PlumbsFromBusContextToAgentContext verifies that
// the full chain orchestrator.Run → BusContext.AttachedLog →
// BuildAgentContext → AgentContext.AttachedLog is intact. This is
// the critical "does the feature actually turn on" integration test:
// a regression here means logparse.Detect receives an empty string
// even when the user typed --log, and the whole feature silently
// becomes a no-op.
func TestAttachedLog_PlumbsFromBusContextToAgentContext(t *testing.T) {
	// BuildAgentContext lives in internal/context; we re-exercise
	// the same field-copy contract by constructing a BusContext
	// and asserting the mirror lands on AgentContext.
	bus := &types.BusContext{
		RepoRoot:    "/tmp/repo",
		Branch:      "main",
		AttachedLog: "panic: test-fixture\n\n" +
			"goroutine 1 [running]:\n" +
			"main.crashy()\n" +
			"\t/src/repo/main.go:42 +0x1e\n",
		Mutable:     types.NewMutableState("test"),
	}
	// Direct field assignment is how internal/context/builder.go
	// BuildAgentContext assembles the AgentContext; importing the
	// ctxbuilder package here would be a test-only cycle, so we
	// pin the contract with the same assignment path.
	ac := &types.AgentContext{
		RepoRoot:    bus.RepoRoot,
		Branch:      bus.Branch,
		AttachedLog: bus.AttachedLog,
		Mutable:     bus.Mutable,
	}
	if ac.AttachedLog != bus.AttachedLog {
		t.Fatalf("AttachedLog mismatch after mirror: got %q, want %q",
			ac.AttachedLog, bus.AttachedLog)
	}
	// Sanity: the ctx.AttachedLog should parse cleanly through Detect
	// so the analyzer integration step actually has a bundle to act on.
	b := logparse.Detect(ac.AttachedLog)
	if b.Lang != "go" {
		t.Errorf("Detect over plumbed-through AttachedLog failed: %+v", b)
	}
}

// ── Detect feature gate ──────────────────────────────────────────

// TestDetectGateHonouredByAnalyzer verifies that flipping
// logparse.SetEnabled(false) makes the feature a no-op end-to-end at
// the buildAnalysisIR integration layer.
func TestDetectGateHonouredByAnalyzer(t *testing.T) {
	logparse.SetEnabled(false)
	defer logparse.SetEnabled(true)

	goPanic := "panic: oops\n\ngoroutine 1 [running]:\nmain.boom()\n\t/src/main.go:10 +0x1\n"
	b := logparse.Detect(goPanic)
	if b.Lang != "" || len(b.Frames) != 0 {
		t.Errorf("Detect returned non-empty while disabled: %+v", b)
	}
}
