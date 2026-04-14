package tool

// P2.1 Session 1 Phase 2 — emit_answer_symbol structural tests.
//
// Mirrors emit_evidence_test.go in shape but adds three P2.1-specific
// invariants:
//
//   - line == 0 is REJECTED (Pattern 2 line-hallucination guard;
//     emit_evidence allows line == 0 because [DIRECT] tags can describe
//     file-wide facts, but answer symbols must always cite a concrete
//     definition site).
//
//   - file inside ctx.WorkDir is REJECTED (blob-file path leak gate,
//     UNRESOLVED bug N=1; see memory/project_blob_file_leak_unresolved.md).
//
//   - kind enum is closed; aliases (func/function, struct/type/class)
//     normalize to canonical forms.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newAnswerSymbolCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState(types.TaskList{})}
}

func TestEmitAnswerSymbol_AcceptsValidBatch(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{
        "items": [
          {"name": "ExplorerAgent", "file": "internal/agent/explorer.go", "line": 23, "kind": "type"},
          {"name": "NewExplorerAgent", "file": "internal/agent/explorer.go", "line": 50, "kind": "func", "chain": "Register binds NewExplorerAgent → Name() returns explorer", "rationale": "named explorer"}
        ],
        "completeness": "complete"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer, got %d", len(got))
	}
	if claim != types.CompletenessComplete {
		t.Errorf("completeness claim not stored: got %q, want complete", claim)
	}
	if got[0].Name != "ExplorerAgent" || got[0].Line != 23 || got[0].Kind != "type" {
		t.Errorf("first item not preserved: %+v", got[0])
	}
	if got[1].Kind != "function" {
		t.Errorf("kind alias 'func' should normalize to 'function', got %q", got[1].Kind)
	}
	if got[1].Chain == "" || got[1].Rationale == "" {
		t.Errorf("optional fields dropped: chain=%q rationale=%q", got[1].Chain, got[1].Rationale)
	}
}

func TestEmitAnswerSymbol_RejectsLineZero(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":0,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success")
	}
	if !strings.Contains(res.Summary, "line must be > 0") {
		t.Errorf("expected line guard diagnosis, got: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "Pattern 2") {
		t.Errorf("expected Pattern 2 attribution in error, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsLineNegative(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":-5,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure on negative line")
	}
}

func TestEmitAnswerSymbol_RejectsBlobPath(t *testing.T) {
	// Blob-file path leak gate. ctx.WorkDir is the per-trace temp
	// directory where StoreBlob persists large tool outputs. If the
	// LLM mistakes a blob path for a repo path the answer cite will
	// resolve to a self-erasing tempdir entry.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.WorkDir = "/tmp/codrax-trace-abc123"
	params := json.RawMessage(`{
        "items": [
          {"name": "Foo", "file": "/tmp/codrax-trace-abc123/grep-xyz.txt", "line": 42, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on blob-file path")
	}
	if !strings.Contains(res.Summary, "lives inside the per-trace WorkDir") {
		t.Errorf("expected blob-leak diagnosis, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_AcceptsSiblingDirectory(t *testing.T) {
	// The blob check uses normalized prefix matching with a trailing
	// slash. A sibling directory whose name shares a prefix with the
	// WorkDir must NOT be rejected — this pins the prefix-match
	// boundary.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.WorkDir = "/tmp/codrax-trace-abc"
	params := json.RawMessage(`{
        "items": [
          {"name": "Foo", "file": "/tmp/codrax-trace-abc-sibling/repo/file.go", "line": 5, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("sibling directory should be accepted, got: %s", res.Summary)
	}
}

func TestEmitAnswerSymbol_AcceptsRepoPathWhenWorkDirEmpty(t *testing.T) {
	// Empty WorkDir = no blob staging configured (unit tests, CI).
	// The check must skip; otherwise every test would reject every
	// cite.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	ctx.WorkDir = ""
	params := json.RawMessage(`{
        "items": [
          {"name": "Foo", "file": "internal/agent/foo.go", "line": 10, "kind": "function"}
        ],
        "completeness": "complete"
    }`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("expected success when WorkDir empty, got: %s", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsUnknownKind(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"trait"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown kind")
	}
	if !strings.Contains(res.Summary, "unknown kind") {
		t.Errorf("expected kind enum diagnosis, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_RejectsMissingName(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"","file":"a.go","line":1,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on missing name")
	}
}

func TestEmitAnswerSymbol_RejectsBareFilename(t *testing.T) {
	// looksLikePath requires '/' or '.'. A bare token without either
	// would also be rejected by emit_evidence; pin the same shape.
	// 'README' has neither.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"README","line":1,"kind":"const"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on non-path-shaped file")
	}
}

func TestEmitAnswerSymbol_RejectsUnknownFields(t *testing.T) {
	// DisallowUnknownFields parity with emit_evidence. A hallucinated
	// 'comment' field must fail loud rather than be silently ignored.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function","comment":"extra"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown field")
	}
}

func TestEmitAnswerSymbol_RejectsEmptyItems(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on empty items")
	}
}

func TestEmitAnswerSymbol_RequiresMutable(t *testing.T) {
	// Sub-agent dispatch path passes a BusContext with Mutable nil.
	// The tool must refuse rather than silently drop the writes.
	tool := &EmitAnswerSymbol{}
	ctx := &types.BusContext{}
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

// -----------------------------------------------------------------------------
// P2.1 Phase 9 — completeness claim schema tests
// -----------------------------------------------------------------------------

func TestEmitAnswerSymbol_MissingCompleteness_Rejected(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	// items are fine, but no completeness — schema requires it
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("missing completeness must be rejected (required field)")
	}
	if !strings.Contains(res.Summary, "completeness") {
		t.Errorf("error must mention completeness, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_EmptyCompleteness_Rejected(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":""}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("empty completeness string must be rejected — silent coercion is the exact bug we are closing")
	}
}

func TestEmitAnswerSymbol_UnknownCompleteness_Rejected(t *testing.T) {
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()
	params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"definitely_complete"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("unknown completeness enum value must be rejected")
	}
	if !strings.Contains(res.Summary, "unknown completeness") {
		t.Errorf("error must diagnose unknown value, got: %q", res.Summary)
	}
}

func TestEmitAnswerSymbol_CompletenessValues_AllAccepted(t *testing.T) {
	cases := []struct {
		raw   string
		stored types.CompletenessClaim
	}{
		{"complete", types.CompletenessComplete},
		{"lower_bound", types.CompletenessLowerBound},
		{"unknown", types.CompletenessUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			tool := &EmitAnswerSymbol{}
			ctx := newAnswerSymbolCtx()
			params := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"` + tc.raw + `"}`)
			res, _ := tool.Execute(ctx, params)
			if !res.Success {
				t.Fatalf("completeness=%q should succeed, got: %s", tc.raw, res.Summary)
			}
			_, claim := ctx.Mutable.EmittedAnswerSymbols()
			if claim != tc.stored {
				t.Errorf("completeness=%q stored as %q, want %q", tc.raw, claim, tc.stored)
			}
			if !strings.Contains(res.Summary, "completeness="+string(tc.stored)) && !strings.Contains(res.Summary, "completeness=unknown") {
				// summary always includes the claim for audit
				t.Errorf("summary missing completeness tag: %q", res.Summary)
			}
		})
	}
}

func TestEmitAnswerSymbol_SecondCallReplaces(t *testing.T) {
	// Set semantics: a second Execute call REPLACES the first slate
	// rather than appending. This is the retry contract — on a
	// downgrade retry the LLM calls again with a different set, and
	// that second call must fully overwrite.
	tool := &EmitAnswerSymbol{}
	ctx := newAnswerSymbolCtx()

	first := json.RawMessage(`{"items":[{"name":"Foo","file":"a.go","line":1,"kind":"function"}],"completeness":"complete"}`)
	if res, _ := tool.Execute(ctx, first); !res.Success {
		t.Fatalf("first call failed: %s", res.Summary)
	}

	second := json.RawMessage(`{"items":[{"name":"Bar","file":"b.go","line":2,"kind":"method"},{"name":"Baz","file":"c.go","line":3,"kind":"type"}],"completeness":"lower_bound"}`)
	if res, _ := tool.Execute(ctx, second); !res.Success {
		t.Fatalf("second call failed: %s", res.Summary)
	}

	syms, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(syms) != 2 {
		t.Errorf("second call should replace with 2 items, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "Bar" {
		t.Errorf("first item should be Bar (from second call), got %q", syms[0].Name)
	}
	if claim != types.CompletenessLowerBound {
		t.Errorf("claim should be lower_bound after second call, got %q", claim)
	}
}

// Sister test: emit_evidence now also enforces the blob-leak gate on
// its source field. Pin the parallel behavior so future refactors of
// the shared isInsideWorkDir helper cannot accidentally diverge the
// two channels.
func TestEmitEvidence_RejectsBlobSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.WorkDir = "/tmp/codrax-trace-xyz"
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "/tmp/codrax-trace-xyz/grep-out.txt", "line_start": 100}
        ]
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("emit_evidence must also reject blob-staged source paths")
	}
	if !strings.Contains(res.Summary, "lives inside the per-trace WorkDir") {
		t.Errorf("expected blob-leak diagnosis, got: %q", res.Summary)
	}
}

// isInsideWorkDir unit boundary tests — the structural prefix logic
// is load-bearing for both tools, so test it directly without going
// through Execute.
func TestIsInsideWorkDir_Boundary(t *testing.T) {
	cases := []struct {
		name     string
		filePath string
		workDir  string
		want     bool
	}{
		{"empty workdir", "internal/agent/foo.go", "", false},
		{"empty file", "", "/tmp/work", false},
		{"exact match", "/tmp/work", "/tmp/work", true},
		{"inside subdir", "/tmp/work/grep.txt", "/tmp/work", true},
		{"inside nested", "/tmp/work/sub/dir/file.go", "/tmp/work", true},
		{"sibling prefix", "/tmp/work-other/file.go", "/tmp/work", false},
		{"workdir trailing slash", "/tmp/work/x.txt", "/tmp/work/", true},
		{"workdir only slash", "x.txt", "/", false},
		{"unrelated path", "internal/agent/foo.go", "/tmp/work", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isInsideWorkDir(tc.filePath, tc.workDir); got != tc.want {
				t.Errorf("isInsideWorkDir(%q, %q) = %v, want %v", tc.filePath, tc.workDir, got, tc.want)
			}
		})
	}
}
