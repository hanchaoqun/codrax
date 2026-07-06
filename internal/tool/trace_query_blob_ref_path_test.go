package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Q5-A tool-layer half of the trace_query blob ref escape lane: registered
// refs must resolve to their verbatim blob path BEFORE the generic
// repo-root join sends a bare basename to <repo>/<basename> (which does
// not exist) or an outside-repo spelling into a scope rejection. The blob
// lives under runtimeAnchor (<CWD>/.codrax), which is NOT the repo root
// whenever codrax runs from a different directory — the tests model that
// split with two separate temp dirs.

func blobRefPathTestContext(t *testing.T) (*types.BusContext, string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	anchor := t.TempDir() // stands in for <CWD>/.codrax/blob/<session>
	blobDir := filepath.Join(anchor, ".codrax", "blob", "20260704-120000-000-1")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	blobPath := filepath.Join(blobDir, "trace-query-result-ef56ab78.json")
	content := "{\"state_total\": {\"runnable_ms\": 1042.5}}\n"
	if err := os.WriteFile(blobPath, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mut := types.NewMutableState("blob ref path")
	mut.RegisterTraceQueryBlobRef(blobPath)
	return &types.BusContext{RepoRoot: repoRoot, Mutable: mut}, blobPath, content
}

func TestResolveReadFilePath_TraceQueryBlobRefEscapeLane(t *testing.T) {
	ctx, blobPath, _ := blobRefPathTestContext(t)

	for _, requested := range []string{
		blobPath,                               // verbatim absolute
		"trace-query-result-ef56ab78.json",     // basename only
		strings.ReplaceAll(blobPath, "/", `\`), // Windows backslash spelling
	} {
		resolved, reject := resolveReadFilePath(ctx, requested)
		if reject != nil {
			t.Fatalf("registered blob ref %q must not be rejected: %+v", requested, reject)
		}
		if resolved != blobPath {
			t.Fatalf("registered blob ref %q resolved to %q, want %q", requested, resolved, blobPath)
		}
	}

	// Unregistered paths keep the plain repo-root join.
	resolved, reject := resolveReadFilePath(ctx, "internal/foo.go")
	if reject != nil {
		t.Fatalf("plain path must not be rejected: %+v", reject)
	}
	if want := filepath.Join(ctx.RepoRoot, "internal/foo.go"); resolved != want {
		t.Fatalf("plain path resolution changed: got %q want %q", resolved, want)
	}

	// Empty registry: identical resolution for the basename shape.
	bare := &types.BusContext{RepoRoot: ctx.RepoRoot, Mutable: types.NewMutableState("empty")}
	resolved, reject = resolveReadFilePath(bare, "trace-query-result-ef56ab78.json")
	if reject != nil {
		t.Fatalf("empty-registry resolution must not reject: %+v", reject)
	}
	if want := filepath.Join(ctx.RepoRoot, "trace-query-result-ef56ab78.json"); resolved != want {
		t.Fatalf("empty registry must keep repo-root join semantics: got %q want %q", resolved, want)
	}
}

func TestReadFile_TraceQueryBlobRefBasenameReadsBlobWithoutCurrentSourceCitation(t *testing.T) {
	ctx, _, content := blobRefPathTestContext(t)
	tool := &ReadFile{}
	params, _ := json.Marshal(readFileParams{Path: "trace-query-result-ef56ab78.json"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("registered blob basename read should succeed, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, strings.TrimSpace(content)) {
		t.Fatalf("blob content missing from summary: %s", result.Summary)
	}
	// Citation hygiene: blob bytes are runtime state, never current source.
	if result.ReadCoverage != nil {
		t.Fatalf("blob read must not publish current-source read coverage: %+v", result.ReadCoverage)
	}
	if len(result.Observations) != 0 {
		t.Fatalf("blob read must not publish current-source observations: %+v", result.Observations)
	}
	// P1-2 typed marker: the read must carry RuntimeArtifactRead so
	// ground.buildLineIndex skips it on a precise signal, not banner text.
	if result.RuntimeArtifactRead == nil || !result.RuntimeArtifactRead.TraceQueryBlob {
		t.Fatalf("blob read must stamp the typed RuntimeArtifactRead(TraceQueryBlob) marker, got %+v", result.RuntimeArtifactRead)
	}
}

func TestReadFile_TraceQueryBlobRefAbsoluteReadNoCurrentSourceCitation(t *testing.T) {
	ctx, blobPath, content := blobRefPathTestContext(t)
	tool := &ReadFile{}
	params, _ := json.Marshal(readFileParams{Path: blobPath})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("registered blob absolute read should succeed, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, strings.TrimSpace(content)) {
		t.Fatalf("blob content missing from summary: %s", result.Summary)
	}
	if result.ReadCoverage != nil || len(result.Observations) != 0 {
		t.Fatalf("blob read must stay citation-inert: coverage=%+v observations=%+v", result.ReadCoverage, result.Observations)
	}
}

func TestGrep_TraceQueryBlobRefBasenameSearchesBlob(t *testing.T) {
	ctx, _, _ := blobRefPathTestContext(t)
	tool := &GrepTool{}
	// line_start/line_end force the deterministic native backend so this
	// test does not depend on which search binary the host has.
	params, _ := json.Marshal(map[string]any{
		"pattern":    "state_total",
		"path":       "trace-query-result-ef56ab78.json",
		"line_start": 1,
		"line_end":   5,
	})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("grep on registered blob basename should succeed, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "state_total") {
		t.Fatalf("grep should find the pattern inside the blob, got: %s", result.Summary)
	}
}

func TestReadFileTypedSourcePath_TraceQueryBlobAndCodraxPathsAreCitationInert(t *testing.T) {
	mut := types.NewMutableState("blob ref path")
	mut.RegisterTraceQueryBlobRef("/anchor/.codrax/blob/s1/trace_query-ab12cd34.txt")
	repoRoot := t.TempDir()
	ctx := &types.BusContext{RepoRoot: repoRoot, Mutable: mut}

	if got := readFileTypedSourcePath(ctx, "trace_query-ab12cd34.txt", "/anchor/.codrax/blob/s1/trace_query-ab12cd34.txt"); got != "" {
		t.Fatalf("registered blob basename must be citation-inert, got %q", got)
	}
	if got := readFileTypedSourcePath(ctx, ".codrax/blob/s1/other.txt", filepath.Join(repoRoot, ".codrax/blob/s1/other.txt")); got != "" {
		t.Fatalf(".codrax-prefixed source path must be citation-inert, got %q", got)
	}
	if got := readFileTypedSourcePath(ctx, "internal/foo.go", filepath.Join(repoRoot, "internal/foo.go")); got != "internal/foo.go" {
		t.Fatalf("plain repo path must keep publishing coverage, got %q", got)
	}
}

// P1-2 typed-marker control: an ordinary current-source read carries NO
// RuntimeArtifactRead marker, so buildLineIndex still indexes it.
func TestReadFile_OrdinarySourceReadHasNoRuntimeArtifactMarker(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "foo.go"), []byte("package foo\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ctx := &types.BusContext{RepoRoot: repoRoot, Mutable: types.NewMutableState("plain")}
	tool := &ReadFile{}
	params, _ := json.Marshal(readFileParams{Path: "foo.go"})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.RuntimeArtifactRead != nil {
		t.Fatalf("ordinary current-source read must NOT carry a runtime-artifact marker: %+v", result.RuntimeArtifactRead)
	}
	if result.ReadCoverage == nil || len(result.Observations) == 0 {
		t.Fatalf("ordinary read must still publish current-source coverage/observations")
	}
}

// P1-1 resolve-time refusal: a foreign path can never be registered (the
// blob-root prefix constraint rejects it), so resolveTraceQueryBlobRefPath
// never claims it — the escape lane's readable set stays confined to
// .codrax/blob/.
func TestResolveTraceQueryBlobRefPath_ForeignPathRefused(t *testing.T) {
	mut := types.NewMutableState("injection")
	// Direct register attempts of foreign paths are refused.
	mut.RegisterTraceQueryBlobRef("/etc/passwd")
	mut.RegisterTraceQueryBlobRef("/tmp/exfil.json")
	mut.RegisterTraceQueryBlobRef("/work/.codrax/logs/session.log") // .codrax but not blob/
	if refs := mut.TraceQueryBlobRefs(); len(refs) != 0 {
		t.Fatalf("foreign / non-blob paths must not register, got %v", refs)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), Mutable: mut}
	for _, p := range []string{"/etc/passwd", "passwd", "/tmp/exfil.json", "session.log"} {
		if _, ok := resolveTraceQueryBlobRefPath(ctx, p); ok {
			t.Fatalf("escape-lane resolver must not claim foreign path %q", p)
		}
	}
	// A genuine blob path still resolves through the escape lane.
	mut.RegisterTraceQueryBlobRef("/work/.codrax/blob/s1/trace_query-ab12cd34.txt")
	if resolved, ok := resolveTraceQueryBlobRefPath(ctx, "trace_query-ab12cd34.txt"); !ok || resolved != "/work/.codrax/blob/s1/trace_query-ab12cd34.txt" {
		t.Fatalf("genuine blob path must resolve via escape lane, got %q ok=%v", resolved, ok)
	}
}
