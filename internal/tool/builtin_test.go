package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

func newBusContext() *types.BusContext {
	return &types.BusContext{}
}

func TestReadFile(t *testing.T) {
	t.Run("read temp file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "testread.txt")
		content := "hello from read test\nsecond line\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: tmpFile})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		// Summary now carries a "showing lines X-Y of N" banner so the
		// LLM cannot mistake a slice for the whole file. The trailing
		// newline in the source content makes Split produce 3 elements.
		if !strings.Contains(result.Summary, "showing lines 1-3 of 3") {
			t.Fatalf("expected banner with line range, got %q", result.Summary)
		}
		// Every line carries an absolute line number in the left
		// gutter so the LLM cites line numbers verbatim instead of
		// guessing. The gutter format is `%6d│ ` — see
		// renderWithLineGutter in builtin.go.
		if !strings.Contains(result.Summary, "     1│ hello from read test") {
			t.Fatalf("expected line 1 gutter, got %q", result.Summary)
		}
		if !strings.Contains(result.Summary, "     2│ second line") {
			t.Fatalf("expected line 2 gutter, got %q", result.Summary)
		}
	})

	t.Run("small file offset=0 with lazy limit expands to full file", func(t *testing.T) {
		// Regression: read_file used to honor offset/limit on tiny files,
		// so the LLM passing limit=20 on a 66-line source file would
		// silently miss the answer past line 20. Offset==0 + small Limit
		// on an inline-sized file is recognised as a training-distribution
		// default and expanded to the whole content with an override
		// banner.
		tmpFile := filepath.Join(t.TempDir(), "testoffset.txt")
		content := "line0\nline1\nline2\nline3\nline4\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: tmpFile, Offset: 0, Limit: 2})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "expanded to full file") {
			t.Fatalf("expected override banner, got %q", result.Summary)
		}
		// All lines must be present, not just the sliced two.
		for _, line := range []string{"line0", "line1", "line2", "line3", "line4"} {
			if !strings.Contains(result.Summary, line) {
				t.Fatalf("expected full content to contain %q, got %q", line, result.Summary)
			}
		}
	})

	t.Run("small file with non-zero offset honors slice", func(t *testing.T) {
		// Non-zero Offset is a strong signal of deliberate paging — the
		// override never fires even on a tiny file. This lets the LLM
		// re-read a specific section after having already consumed the
		// whole file earlier in the dispatch.
		tmpFile := filepath.Join(t.TempDir(), "testoffset_honor.txt")
		content := "line0\nline1\nline2\nline3\nline4\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: tmpFile, Offset: 1, Limit: 2})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "showing lines 2-3") {
			t.Fatalf("expected sliced banner, got %q", result.Summary)
		}
		// Must NOT contain the tail line that falls outside the slice.
		if strings.Contains(result.Summary, "line4") {
			t.Fatalf("expected tail to be sliced off, got %q", result.Summary)
		}
	})

	t.Run("offset at eof returns explicit empty range error", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "testoffset_eof.txt")
		content := "line0\nline1"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: tmpFile, Offset: 2, Limit: 100})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatalf("expected empty range to fail clearly, got success: %s", result.Summary)
		}
		for _, want := range []string{"read_file empty range", "line_offset=2", "2 total line(s)", "last valid line_offset is 1", "not a byte"} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("summary missing %q: %s", want, result.Summary)
			}
		}
		if strings.Contains(result.Summary, "showing lines 3-2") {
			t.Fatalf("must not render an inverted line window: %s", result.Summary)
		}
	})

	t.Run("line_offset aliases targeted line paging", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test_line_offset.txt")
		content := "line0\nline1\nline2\nline3\nline4\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &ReadFile{}
		params := json.RawMessage(fmt.Sprintf(`{"path":%q,"line_offset":2,"limit":2}`, tmpFile))
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		for _, want := range []string{
			"showing lines 3-4",
			"read_file coordinates",
			"line_offset is a zero-based line offset",
			"     3│ line2",
			"     4│ line3",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("summary missing %q: %s", want, result.Summary)
			}
		}
	})

	t.Run("small file with limit above threshold honors slice", func(t *testing.T) {
		// A Limit above ReadFileSmallLimitThreshold is a deliberate size
		// choice (LLMs rarely pick 150/200 lazily) — override must not
		// fire. Build a file with enough lines that Limit still cuts
		// content.
		var b strings.Builder
		for i := 0; i < 300; i++ {
			b.WriteString("line\n")
		}
		tmpFile := filepath.Join(t.TempDir(), "testoffset_big_limit.txt")
		if err := os.WriteFile(tmpFile, []byte(b.String()), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		prev := ReadFileSmallLimitThreshold
		t.Cleanup(func() { SetReadFileSmallLimitThreshold(prev) })
		SetReadFileSmallLimitThreshold(100)

		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: tmpFile, Offset: 0, Limit: 150})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "showing lines 1-150") {
			t.Fatalf("expected sliced banner (limit above threshold), got first 200 chars: %q", result.Summary[:min(200, len(result.Summary))])
		}
	})

	t.Run("threshold=0 disables override", func(t *testing.T) {
		// Setting readfile_small_limit_threshold to 0 disables the
		// override entirely — offset/limit is honored on any file size.
		tmpFile := filepath.Join(t.TempDir(), "testoffset_disabled.txt")
		content := "line0\nline1\nline2\nline3\nline4\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		prev := ReadFileSmallLimitThreshold
		t.Cleanup(func() { SetReadFileSmallLimitThreshold(prev) })
		SetReadFileSmallLimitThreshold(0)

		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: tmpFile, Offset: 0, Limit: 2})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "showing lines 1-2") {
			t.Fatalf("expected sliced banner (threshold disabled), got %q", result.Summary)
		}
		if strings.Contains(result.Summary, "line4") {
			t.Fatalf("expected tail to be sliced off, got %q", result.Summary)
		}
	})

	t.Run("large file honors offset and limit", func(t *testing.T) {
		// Files larger than MaxInlineBytes still page via offset/limit
		// because they cannot be returned whole.
		var b strings.Builder
		// Each line is 100 bytes; 400 lines ≈ 40 KB > MaxInlineBytes (32 KB).
		for i := 0; i < 400; i++ {
			b.WriteString(strings.Repeat("x", 99))
			b.WriteString("\n")
		}
		tmpFile := filepath.Join(t.TempDir(), "big.txt")
		if err := os.WriteFile(tmpFile, []byte(b.String()), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: tmpFile, Offset: 10, Limit: 5})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "showing lines 11-15 of 401") {
			t.Fatalf("expected sliced banner for large file, got first 200 chars: %q", result.Summary[:min(200, len(result.Summary))])
		}
	})

	t.Run("large slice banner clamped to visible lines", func(t *testing.T) {
		// When a slice exceeds MaxInlineBytes, StoreBlobHeadOnly truncates
		// to previewHeadBytes. The banner must reflect the clamped line
		// count so the LLM pages from the right offset — without this fix,
		// the banner claims "showing lines 1-400" but only ~245 lines are
		// visible, and the LLM skips lines 246-400 when paging.
		var b strings.Builder
		// 400 lines × 100 bytes = 40 KB > MaxInlineBytes (32 KB).
		for i := 0; i < 400; i++ {
			b.WriteString(strings.Repeat("x", 99))
			b.WriteString("\n")
		}
		tmpFile := filepath.Join(t.TempDir(), "big_clamp.txt")
		if err := os.WriteFile(tmpFile, []byte(b.String()), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &ReadFile{}
		// Request all 400 lines — the 40 KB slice exceeds MaxInlineBytes,
		// so the clamping logic should reduce sliceEnd to match what
		// StoreBlobHeadOnly will actually show (~245 lines at 24 KB head).
		params, _ := json.Marshal(readFileParams{Path: tmpFile, Offset: 0, Limit: 400})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}

		// The banner must NOT say "showing lines 1-400" — that would be
		// the unclamped value. It should say "showing lines 1-N" where
		// N < 400, matching the head budget.
		if strings.Contains(result.Summary, "showing lines 1-400") {
			t.Fatalf("banner was NOT clamped — still shows 1-400; clamping logic did not fire")
		}
		// Verify the banner is present and the end line is reasonable:
		// previewHeadBytes (24 KB) / 100 bytes per line ≈ 245 lines.
		if !strings.Contains(result.Summary, "of 401 total") {
			t.Fatalf("expected total=401 in banner, got first 200 chars: %q", result.Summary[:min(200, len(result.Summary))])
		}
		// The clamped content should fit inline (no blob truncation hint).
		if strings.Contains(result.Summary, "remaining") || strings.Contains(result.Summary, "NOT returned") {
			t.Fatalf("clamped content should fit inline without blob truncation, but got truncation hint")
		}
	})

	t.Run("log triage rejects repo files when no attachment blob exists", func(t *testing.T) {
		repoRoot := t.TempDir()
		src := filepath.Join(repoRoot, "internal", "agent")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		file := filepath.Join(src, "analyzer.go")
		if err := os.WriteFile(file, []byte("package agent\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		ctx := &types.BusContext{
			RepoRoot:      repoRoot,
			WorkDir:       t.TempDir(),
			PipelineStage: types.StageLogTriage,
			ActiveAgent:   types.AgentLogTriager,
			AttachedLog:   "panic: boom",
			Mutable:       types.NewMutableState("test"),
		}
		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: "internal/agent/analyzer.go"})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatalf("expected rejection, got success summary=%q", result.Summary)
		}
		if result.Repair == nil || result.Repair.Code != "attachment_only_read_file" {
			t.Fatalf("expected attachment-only repair, got %+v", result.Repair)
		}
	})

	t.Run("log triage allows attached-log blob basename", func(t *testing.T) {
		workDir := t.TempDir()
		blobPath := filepath.Join(workDir, "attached_log.txt")
		if err := os.WriteFile(blobPath, []byte("panic: boom\nframe\n"), 0o644); err != nil {
			t.Fatalf("write blob: %v", err)
		}

		ctx := &types.BusContext{
			WorkDir:       workDir,
			PipelineStage: types.StageLogTriage,
			ActiveAgent:   types.AgentLogTriager,
			AttachedLog:   strings.Repeat("x", 8192),
			Mutable:       types.NewMutableState("test"),
		}
		tool := &ReadFile{}
		params, _ := json.Marshal(readFileParams{Path: "attached_log.txt"})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "panic: boom") {
			t.Fatalf("expected blob contents, got %q", result.Summary)
		}
	})
}

func TestReadFilePathMissIncludesSourceInventorySameScopeHint(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("read file source inventory miss")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active: true,
		Scopes: []string{"src"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Members: []types.SourceInventoryObservationMember{{
				Name:     "run_alpha",
				Role:     types.AnswerCandidateRoleFunction,
				File:     "src/alpha/run.py",
				Line:     7,
				Language: "python",
			}, {
				Name:     "run_beta",
				Role:     types.AnswerCandidateRoleFunction,
				File:     "src/beta/run.py",
				Line:     9,
				Language: "python",
			}},
		}},
	})
	ctx := &types.BusContext{RepoRoot: repo, Mutable: mut}

	params, _ := json.Marshal(readFileParams{Path: "src/alpha/alpha.py"})
	result, err := (&ReadFile{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected missing path failure, got success: %+v", result)
	}
	for _, want := range []string{
		"read failed:",
		"Source-inventory hint (advisory)",
		"same scope `src/alpha`",
		"`src/alpha/run.py`",
		"function `run_alpha`@7",
		"not a whitelist",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("path-miss hint missing %q:\n%s", want, result.Summary)
		}
	}
	if strings.Contains(result.Summary, "run_beta") {
		t.Fatalf("path-miss hint leaked sibling scope candidate:\n%s", result.Summary)
	}

	params, _ = json.Marshal(readFileParams{Path: "src/gamma/missing.py"})
	result, err = (&ReadFile{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected missing path failure, got success: %+v", result)
	}
	if strings.Contains(result.Summary, "Source-inventory hint") {
		t.Fatalf("unmatched scope should not emit source-inventory suggestions:\n%s", result.Summary)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestListFiles(t *testing.T) {
	t.Run("list files in directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		files := []string{"aaa.txt", "bbb.go", "ccc.md"}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("x"), 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}

		tool := &ListFiles{}
		params, _ := json.Marshal(listFilesParams{Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}

		for _, f := range files {
			expected := filepath.Join(tmpDir, f)
			if !strings.Contains(result.Summary, expected) {
				t.Errorf("expected %s in output, got: %s", expected, result.Summary)
			}
		}
		// Params banner must record the directory the LLM asked for so
		// Turn B (extractor / finalizer) can see the scope of the listing.
		if !strings.Contains(result.Summary, "[list_files: path="+tmpDir+"]") {
			t.Errorf("expected params banner with path=%q, got: %s", tmpDir, result.Summary)
		}
	})
}

func TestListFiles_FileTypeFiltersPathNames(t *testing.T) {
	repo := t.TempDir()
	for pathValue, content := range map[string]string{
		"entry/src/main/ets/pages/Index.ets": "@Entry\n@Component\nstruct Index {}\n",
		"entry/src/main/ets/pages/helper.ts": "export const helper = 1\n",
		"README.md":                          "# docs\n",
	} {
		full := filepath.Join(repo, filepath.FromSlash(pathValue))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", pathValue, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", pathValue, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(repo, "fake.ets", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir fake .ets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "fake.ets", "nested", "not_arkts.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fake nested file: %v", err)
	}

	result, err := (&ListFiles{}).Execute(&types.BusContext{RepoRoot: repo}, json.RawMessage(`{"path":".","recursive":true,"file_type":"arkts"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "entry/src/main/ets/pages/Index.ets") {
		t.Fatalf("ArkTS file missing from filtered listing:\n%s", result.Summary)
	}
	for _, noise := range []string{"helper.ts", "README.md"} {
		if strings.Contains(result.Summary, noise) {
			t.Fatalf("file_type filter leaked %q:\n%s", noise, result.Summary)
		}
	}
	for _, line := range strings.Split(result.Summary, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case ".", "entry", "entry/src", "entry/src/main", "entry/src/main/ets", "entry/src/main/ets/pages", "fake.ets", "fake.ets/nested":
			t.Fatalf("file_type path discovery must not output directory row %q:\n%s", line, result.Summary)
		}
	}
	if !strings.Contains(result.Summary, "file_type=arkts") {
		t.Fatalf("banner should preserve file_type provenance:\n%s", result.Summary)
	}
}

func TestListFiles_FileTypeCanIncludeRepoOwnedAuxiliary(t *testing.T) {
	repo := t.TempDir()
	paths := []string{
		"entry/src/main/ets/pages/Index.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		"node_modules/pkg/noise.ets",
	}
	for _, pathValue := range paths {
		full := filepath.Join(repo, filepath.FromSlash(pathValue))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", pathValue, err)
		}
		if err := os.WriteFile(full, []byte("@Entry\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", pathValue, err)
		}
	}

	tool := &ListFiles{}
	bus := &types.BusContext{RepoRoot: repo}
	withoutAux, err := tool.Execute(bus, json.RawMessage(`{"path":".","recursive":true,"file_type":"arkts"}`))
	if err != nil {
		t.Fatalf("Execute without aux: %v", err)
	}
	if !strings.Contains(withoutAux.Summary, "entry/src/main/ets/pages/Index.ets") {
		t.Fatalf("default listing should keep primary production matches:\n%s", withoutAux.Summary)
	}
	if strings.Contains(withoutAux.Summary, "01_entry_component_minimal.ets") {
		t.Fatalf("default listing must keep auxiliary corpus filtered when primary matches exist:\n%s", withoutAux.Summary)
	}

	withAux, err := tool.Execute(bus, json.RawMessage(`{"path":".","recursive":true,"file_type":"arkts","include_auxiliary":true}`))
	if err != nil {
		t.Fatalf("Execute with aux: %v", err)
	}
	if !strings.Contains(withAux.Summary, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets") {
		t.Fatalf("include_auxiliary listing should surface repo-owned corpus files:\n%s", withAux.Summary)
	}
	if strings.Contains(withAux.Summary, "node_modules/pkg/noise.ets") {
		t.Fatalf("include_auxiliary must not reopen dependency/cache trees:\n%s", withAux.Summary)
	}
	if !strings.Contains(withAux.Summary, "include_auxiliary=true") {
		t.Fatalf("banner should preserve auxiliary-scope provenance:\n%s", withAux.Summary)
	}
}

func TestListFiles_FileTypeFallsBackToAuxiliaryWhenPrimaryEmpty(t *testing.T) {
	repo := t.TempDir()
	for _, pathValue := range []string{
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		"node_modules/pkg/noise.ets",
	} {
		full := filepath.Join(repo, filepath.FromSlash(pathValue))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", pathValue, err)
		}
		if err := os.WriteFile(full, []byte("@Entry\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", pathValue, err)
		}
	}

	result, err := (&ListFiles{}).Execute(&types.BusContext{RepoRoot: repo}, json.RawMessage(`{"path":".","recursive":true,"file_type":"arkts"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets") {
		t.Fatalf("empty primary source scan should fall back to repo-owned auxiliary sources:\n%s", result.Summary)
	}
	if strings.Contains(result.Summary, "node_modules/pkg/noise.ets") {
		t.Fatalf("auxiliary fallback must not reopen dependency/cache trees:\n%s", result.Summary)
	}
	if !strings.Contains(result.Summary, "include_auxiliary=true") {
		t.Fatalf("banner should expose deterministic fallback provenance:\n%s", result.Summary)
	}
}

func TestListFiles_IncludeGlobFallsBackToRecursiveAuxiliaryWhenRootEmpty(t *testing.T) {
	repo := t.TempDir()
	pathValue := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	full := filepath.Join(repo, filepath.FromSlash(pathValue))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir auxiliary source: %v", err)
	}
	if err := os.WriteFile(full, []byte("@Entry\n"), 0o644); err != nil {
		t.Fatalf("write auxiliary source: %v", err)
	}

	result, err := (&ListFiles{}).Execute(&types.BusContext{RepoRoot: repo}, json.RawMessage(`{"path":".","include":"*.ets"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, pathValue) {
		t.Fatalf("root targeted glob with no direct matches should fall back to recursive auxiliary scan:\n%s", result.Summary)
	}
	for _, want := range []string{"recursive=true", "include_auxiliary=true", "include=*.ets"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("fallback banner missing %q:\n%s", want, result.Summary)
		}
	}
}

func TestListFiles_FileTypeAutoIncludesAuxiliaryForTypedSourceInventory(t *testing.T) {
	repo := t.TempDir()
	paths := []string{
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		"vendor/harmony/runtime_component.ets",
		"eval/fixtures/testdata/harmony/fixture_component.ets",
		"eval/results/run-1.repo/harmony/result_noise.ets",
		"node_modules/pkg/noise.ets",
		".codrax/output/noise.ets",
	}
	for _, pathValue := range paths {
		full := filepath.Join(repo, filepath.FromSlash(pathValue))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", pathValue, err)
		}
		if err := os.WriteFile(full, []byte("@Entry\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", pathValue, err)
		}
	}

	bus := &types.BusContext{
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				SourceQuotes:      []string{"ArkTS sources"},
				Confidence:        0.9,
			},
		}},
	}

	result, err := (&ListFiles{}).Execute(bus, json.RawMessage(`{"path":".","recursive":true,"file_type":"arkts"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets") {
		t.Fatalf("typed source inventory should auto-include repo-owned auxiliary sources:\n%s", result.Summary)
	}
	for _, want := range []string{
		"vendor/harmony/runtime_component.ets",
		"eval/fixtures/testdata/harmony/fixture_component.ets",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("typed source inventory should include auxiliary source class %q:\n%s", want, result.Summary)
		}
	}
	for _, noise := range []string{
		"eval/results/run-1.repo/harmony/result_noise.ets",
		"node_modules/pkg/noise.ets",
		".codrax/output/noise.ets",
	} {
		if strings.Contains(result.Summary, noise) {
			t.Fatalf("auto auxiliary inclusion must not reopen dependency/cache noise %q:\n%s", noise, result.Summary)
		}
	}
	if !strings.Contains(result.Summary, "include_auxiliary=true") {
		t.Fatalf("banner should expose deterministic auto-inclusion provenance:\n%s", result.Summary)
	}
}

func TestListFiles_FileTypeDoesNotAutoIncludeAuxiliaryWithExplicitExclusion(t *testing.T) {
	repo := t.TempDir()
	pathValue := "internal/thirdparty/tree-sitter-arkts/corpus/sources/entry_component.ets"
	full := filepath.Join(repo, filepath.FromSlash(pathValue))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir auxiliary source: %v", err)
	}
	if err := os.WriteFile(full, []byte("@Entry\n"), 0o644); err != nil {
		t.Fatalf("write auxiliary source: %v", err)
	}

	bus := &types.BusContext{
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleFixture,
				},
				SourceQuotes: []string{"supporting corpus"},
				Confidence:   0.9,
			},
		}},
	}

	result, err := (&ListFiles{}).Execute(bus, json.RawMessage(`{"path":".","recursive":true,"file_type":"arkts"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	if strings.Contains(result.Summary, pathValue) {
		t.Fatalf("explicit typed auxiliary exclusion should keep fixture out of default listing:\n%s", result.Summary)
	}
	if strings.Contains(result.Summary, "include_auxiliary=true") {
		t.Fatalf("banner must not claim auto auxiliary inclusion when explicit exclusion blocks it:\n%s", result.Summary)
	}
}

func TestGrep_FileTypeAutoIncludesAuxiliaryForTypedSourceInventory(t *testing.T) {
	repo := t.TempDir()
	paths := []string{
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/entry_component.ets",
		"vendor/harmony/runtime_component.ets",
		"eval/fixtures/testdata/harmony/fixture_component.ets",
		"eval/results/run-1.repo/harmony/result_noise.ets",
		"node_modules/pkg/noise.ets",
		".codrax/output/noise.ets",
	}
	for _, pathValue := range paths {
		full := filepath.Join(repo, filepath.FromSlash(pathValue))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", pathValue, err)
		}
		if err := os.WriteFile(full, []byte("@Entry\n@Component\nstruct Demo {}\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", pathValue, err)
		}
	}

	bus := &types.BusContext{
		RepoRoot: repo,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
				Confidence:        0.9,
			},
		}},
	}

	result, err := (&GrepTool{}).Execute(bus, json.RawMessage(`{"path":".","file_type":"arkts","pattern":"@Entry","files_only":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	for _, want := range []string{
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/entry_component.ets",
		"vendor/harmony/runtime_component.ets",
		"eval/fixtures/testdata/harmony/fixture_component.ets",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("typed source inventory grep should include auxiliary source %q:\n%s", want, result.Summary)
		}
	}
	for _, noise := range []string{
		"eval/results/run-1.repo/harmony/result_noise.ets",
		"node_modules/pkg/noise.ets",
		".codrax/output/noise.ets",
	} {
		if strings.Contains(result.Summary, noise) {
			t.Fatalf("typed source inventory grep must not reopen runtime/dependency noise %q:\n%s", noise, result.Summary)
		}
	}
}

func TestGrepNoMatchPathDiscoveryAdvisory(t *testing.T) {
	got := grepNoMatchBody(&types.BusContext{}, grepToolParams{
		Pattern:   "Missing",
		Include:   "*.ets",
		FilesOnly: true,
	})
	if !strings.Contains(got, "path_discovery_advisory=") ||
		!strings.Contains(got, "list_files") ||
		!strings.Contains(got, "include/file_type") {
		t.Fatalf("grep include no-match should steer path discovery to list_files:\n%s", got)
	}
}

func TestExecCommandFindDiscoveryAdvisory(t *testing.T) {
	got := execCommandFileDiscoveryAdvisory(`find . -name "*.ets" | head`)
	if !strings.Contains(got, "list_files") ||
		!strings.Contains(got, "file_type") ||
		!strings.Contains(got, "include_auxiliary") {
		t.Fatalf("find discovery advisory should steer to typed list_files, got:\n%s", got)
	}
	if noise := execCommandFileDiscoveryAdvisory(`grep -R needle .`); noise != "" {
		t.Fatalf("non-find command should not get file discovery advisory: %q", noise)
	}
}

func TestExecCommandFindDiscoveryRepair(t *testing.T) {
	ctx := newBusContext()
	ctx.RepoRoot = t.TempDir()
	ctx.Mode = types.ModeRead
	ctx.PipelineStage = types.StageExplore
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.9,
		},
	}}
	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: `find . -name "*.ets" | head`})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("broad file discovery should be refused before execution, got %q", result.Summary)
	}
	if result.Repair == nil || result.Repair.Code != "exec_command_file_discovery_use_typed_tools" {
		t.Fatalf("expected typed file-discovery repair, got %+v", result.Repair)
	}
	if !strings.Contains(result.Summary, "repo_map") || !strings.Contains(result.Summary, "source_inventory") {
		t.Fatalf("source-inventory lane should steer to typed inventory tools, got %q", result.Summary)
	}
}

func TestExecCommandFindCountMeasurementAllowed(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "sample.go"), []byte("package sample\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := newBusContext()
	ctx.RepoRoot = repo
	ctx.Mode = types.ModeRead
	ctx.PipelineStage = types.StageExplore
	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: `find . -name "*.go" | wc -l`})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("deterministic find count should remain allowed, got %q", result.Summary)
	}
	if result.Repair != nil {
		t.Fatalf("count measurement should not produce file-discovery repair: %+v", result.Repair)
	}
	if !strings.Contains(result.Summary, "evidence_origin=command_measurement") {
		t.Fatalf("find count should remain a command measurement, got %q", result.Summary)
	}
}

func TestExecCommandFindCountMeasurementBlockedDuringSourceInventoryDebt(t *testing.T) {
	ctx := newBusContext()
	ctx.RepoRoot = t.TempDir()
	ctx.Mode = types.ModeRead
	ctx.PipelineStage = types.StageExplore
	ctx.Mutable = types.NewMutableState("source inventory debt")
	ctx.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Execution: &types.SourceInventoryExecutionState{
			Budgeted:                 true,
			CandidateBudgetTruncated: true,
		},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: false,
			Count:    1,
			Total:    20,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "observed",
				Key:           "observed",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "internal/tool/builtin.go",
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.9,
		},
	}}

	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: `find . -type f -name "*.go" | wc -l`})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("active source-inventory debt should refuse shell file-set count, got %q", result.Summary)
	}
	if result.Repair == nil || result.Repair.Code != "exec_command_source_inventory_debt_use_typed_inventory" {
		t.Fatalf("expected source-inventory debt repair, got %+v", result.Repair)
	}
	if !strings.Contains(result.Summary, "repo_map") || !strings.Contains(result.Summary, "source_inventory") {
		t.Fatalf("repair should steer to typed source inventory route, got %q", result.Summary)
	}
}

func TestExecCommandBroadGrepBlockedDuringSourceInventoryDebt(t *testing.T) {
	ctx := newBusContext()
	ctx.RepoRoot = t.TempDir()
	ctx.Mode = types.ModeRead
	ctx.PipelineStage = types.StageExplore
	ctx.Mutable = types.NewMutableState("source inventory debt")
	ctx.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Execution: &types.SourceInventoryExecutionState{
			Budgeted:                 true,
			CandidateBudgetTruncated: true,
		},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: false,
			Total:    20,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "observed",
				Key:           "observed",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "internal/tool/builtin.go",
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.9,
		},
	}}

	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: `grep -rn "^[[:space:]]*public[[:space:]]*class" --include="*.cj" . | head -80`})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("active source-inventory debt should refuse broad root grep enumeration, got %q", result.Summary)
	}
	if result.Repair == nil || result.Repair.Code != "exec_command_source_inventory_debt_use_typed_inventory" {
		t.Fatalf("expected source-inventory debt repair, got %+v", result.Repair)
	}
	if got := result.Repair.Metadata["reason_code"]; got != "source_inventory_debt_broad_content_enumeration" {
		t.Fatalf("reason_code=%q, want broad content enumeration", got)
	}
	if !strings.Contains(result.Summary, "repo_map") || !strings.Contains(result.Summary, "source_inventory") {
		t.Fatalf("repair should steer to typed source inventory route, got %q", result.Summary)
	}
}

func TestExecCommandBroadContentEnumerationClassifier(t *testing.T) {
	blocked := []string{
		`grep -rn "needle" --include="*.cj" . | head -30`,
		`grep -R "needle" .`,
		`rg "needle" .`,
		`rg -g '*.ets' '@Entry'`,
	}
	for _, command := range blocked {
		if !execCommandLooksLikeBroadRepoContentEnumeration(command) {
			t.Fatalf("expected broad repo content enumeration for %q", command)
		}
	}
	allowed := []string{
		`grep -rn "needle" internal/thirdparty/tree-sitter-cangjie/corpus/sources`,
		`grep -rn "needle" eval/fixtures/testdata/cangjie_minimal internal/thirdparty/tree-sitter-cangjie/corpus/sources`,
		`grep -c "needle" .`,
		`rg "needle" internal/tool`,
		`printf 'needle\n'`,
	}
	for _, command := range allowed {
		if execCommandLooksLikeBroadRepoContentEnumeration(command) {
			t.Fatalf("did not expect broad repo content enumeration for %q", command)
		}
	}
}

func TestExecCommandFileSetMeasurementClassifier(t *testing.T) {
	cases := []string{
		`find . -type f -name "*.go" | wc -l`,
		`git ls-files '*.go' | wc -l`,
		`rg --files -g '*.ets' | wc -l`,
		`ls internal/tool | wc -l`,
	}
	for _, command := range cases {
		if !execCommandLooksLikeFileSetMeasurement(command) {
			t.Fatalf("expected file-set measurement classification for %q", command)
		}
	}
	for _, command := range []string{
		`grep -R "needle" internal/tool | wc -l`,
		`printf '7\n'`,
		`git log --oneline -5 | wc -l`,
	} {
		if execCommandLooksLikeFileSetMeasurement(command) {
			t.Fatalf("did not expect file-set measurement classification for %q", command)
		}
	}
}

func TestExecCommand(t *testing.T) {
	t.Run("echo hello", func(t *testing.T) {
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "echo hello"})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		// Summary is now "[exec_command: $ echo hello]\nhello\n" — the
		// banner records the call so Turn B (extractor / finalizer) can
		// see the command that produced a scalar answer. Assert both
		// the banner line and the stdout are present.
		if !strings.Contains(result.Summary, "[exec_command: $ echo hello]") {
			t.Fatalf("expected params banner, got %q", result.Summary)
		}
		normalized := strings.ReplaceAll(result.Summary, "\r\n", "\n")
		if !strings.Contains(normalized, "hello\n") {
			t.Fatalf("expected stdout 'hello', got %q", result.Summary)
		}
		if strings.Contains(result.Summary, "evidence_origin=command_measurement") {
			t.Fatalf("non-measurement command should not be tagged as measurement: %q", result.Summary)
		}
	})

	t.Run("tags deterministic count measurement", func(t *testing.T) {
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "printf '7\\n'"})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "evidence_origin=command_measurement") {
			t.Fatalf("deterministic count command should be tagged as command measurement: %q", result.Summary)
		}
		if got, ok := types.DeterministicCountProofInteger(result.Summary); !ok || got != 7 {
			t.Fatalf("measurement origin line must not break count proof, got (%d,%v) from %q", got, ok, result.Summary)
		}
	})

	t.Run("tags git metadata command", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		ctx.Mode = types.ModeRead
		ctx.PipelineStage = types.StageExplore
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "git log -1 --oneline"})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "evidence_origin=vcs_metadata") {
			t.Fatalf("git metadata command should be tagged as VCS metadata: %q", result.Summary)
		}
		if strings.Contains(result.Summary, "diff_origin=vcs_diff") {
			t.Fatalf("plain git log should not be tagged as diff evidence: %q", result.Summary)
		}
	})

	t.Run("normalizes git show no-stat compatibility", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		ctx.Mode = types.ModeRead
		ctx.PipelineStage = types.StageExplore
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "git show HEAD --no-stat"})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected normalized command success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "removed unsupported `git show --no-stat`") {
			t.Fatalf("expected compatibility note, got %q", result.Summary)
		}
		if strings.Contains(result.Summary, "unrecognized argument") {
			t.Fatalf("normalized command should not reach git's --no-stat error: %q", result.Summary)
		}
	})

	t.Run("runtime grep pipeline success without line numbers gets advisory", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "record_trace.systrace")
		if err := os.WriteFile(tmpFile, []byte("com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "grep 'com.tencent.mm-36379' " + strconv.Quote(tmpFile)})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		for _, want := range []string{
			"exec_command advisory",
			"has no original line numbers",
			"grep -n",
			"read_file around the selected range",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("missing exec advisory %q:\n%s", want, result.Summary)
			}
		}
	})

	t.Run("runtime grep pipeline exit one gets no-match advisory", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "record_trace.systrace")
		if err := os.WriteFile(tmpFile, []byte("2942.124416: sched_switch\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "grep 'missing-event' " + strconv.Quote(tmpFile)})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatalf("grep no-match shell exit should remain unsuccessful, got: %s", result.Summary)
		}
		for _, want := range []string{
			"grep exited 1",
			"zero matches",
			"Split combined log/trace patterns",
			"grep -n",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("missing no-match advisory %q:\n%s", want, result.Summary)
			}
		}
	})

	t.Run("runtime grep pipeline broad alternation gets search-shape advisory", func(t *testing.T) {
		command := `grep -n "com.tencent.mm-36379\|2942\." "record_trace.systrace" | head -100`
		output := "130149:\tcom.tencent.mm-36379 (36379) [007] .... 2939.734658: sched_switch\n"
		got := execCommandSearchShapeAdvisory(command, output, nil)
		for _, want := range []string{
			"broad OR/alternation pattern",
			"conjunctive filtering or numeric filtering",
			"preserves original line numbers",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing broad-OR advisory %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "no original line numbers") {
			t.Fatalf("line-numbered output should not get no-line-number advisory:\n%s", got)
		}

		codeCommand := `grep -n "Foo\|Bar" "internal/agent/explorer.go"`
		if codeGot := execCommandSearchShapeAdvisory(codeCommand, "12: Foo\n", nil); codeGot != "" {
			t.Fatalf("code grep should not get runtime broad-OR advisory:\n%s", codeGot)
		}
	})

	t.Run("runtime awk filter without line numbers gets advisory", func(t *testing.T) {
		command := `awk '/2942\.1244[1-9]|2942\.260[0-1][0-9]/ && /\[GT\]ColdPool#5-36624/' record_trace.systrace`
		output := "  [GT]ColdPool#5-36624 (36379) [001] .... 2942.256055: sched_switch\n"
		got := execCommandSearchShapeAdvisory(command, output, nil)
		for _, want := range []string{
			"search/filter output has no original line numbers",
			"awk '... { printf",
			"read_file around the selected range",
			"broad OR/alternation pattern",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing awk advisory %q:\n%s", want, got)
			}
		}
	})

	t.Run("suffixless readable trace content gets runtime advisory", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "capture_without_suffix")
		if err := os.WriteFile(tmpFile, []byte("2942.124416: sched_switch: prev_comm=app prev_pid=10 prev_state=R ==> next_comm=worker next_pid=20\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		command := `awk '/sched_switch/ { print $0 }' ` + strconv.Quote(tmpFile)
		output := "2942.124416: sched_switch: prev_comm=app prev_pid=10 prev_state=R ==> next_comm=worker next_pid=20\n"
		got := execCommandSearchShapeAdvisory(command, output, nil)
		if !strings.Contains(got, "search/filter output has no original line numbers") {
			t.Fatalf("suffixless trace content should get runtime advisory:\n%s", got)
		}
	})

	t.Run("readable nonruntime suffix-looking file does not get runtime advisory", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "notes.systrace")
		if err := os.WriteFile(tmpFile, []byte("package demo\nfunc Execute() {}\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		command := `awk '/Execute/ { print $0 }' ` + strconv.Quote(tmpFile)
		output := "func Execute() {}\n"
		if got := execCommandSearchShapeAdvisory(command, output, nil); got != "" {
			t.Fatalf("readable nonruntime file should not get runtime advisory:\n%s", got)
		}
	})

	t.Run("runtime awk head and tail filters without line numbers get advisory", func(t *testing.T) {
		output := "  [GT]ColdPool#5-36624 (36379) [001] .... 2942.256055: sched_switch\n"
		commands := []string{
			`awk '/2942\.256/ && /\[GT\]ColdPool#5-36624/' record_trace.systrace | head -100`,
			`awk '/2942\.256/ && /prev_comm=\[GT\]ColdPool#5 prev_pid=36624/' record_trace.systrace | tail -20`,
		}
		for _, command := range commands {
			got := execCommandSearchShapeAdvisory(command, output, nil)
			if !strings.Contains(got, "search/filter output has no original line numbers") {
				t.Fatalf("missing no-line-number advisory for %q:\n%s", command, got)
			}
		}
	})

	t.Run("runtime awk filter with NR line numbers needs no line advisory", func(t *testing.T) {
		command := `awk '/2942\.256/ { printf "%d:%s\n", NR, $0 }' record_trace.systrace | head -100`
		output := "1102704:  [GT]ColdPool#5-36624 (36379) [001] .... 2942.256055: sched_switch\n"
		got := execCommandSearchShapeAdvisory(command, output, nil)
		if strings.Contains(got, "no original line numbers") {
			t.Fatalf("line-numbered awk output should not get no-line-number advisory:\n%s", got)
		}
	})

	t.Run("runtime scalar awk output and code awk commands do not get search advisory", func(t *testing.T) {
		scalarCommand := `awk 'END { print NR }' record_trace.systrace`
		if got := execCommandSearchShapeAdvisory(scalarCommand, "1525954\n", nil); got != "" {
			t.Fatalf("scalar runtime awk output should not get advisory:\n%s", got)
		}

		codeCommand := `awk '/func Execute/' internal/tool/builtin.go | head -20`
		codeOutput := "func (t *ExecCommand) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {\n"
		if got := execCommandSearchShapeAdvisory(codeCommand, codeOutput, nil); got != "" {
			t.Fatalf("code awk command should not get runtime advisory:\n%s", got)
		}
	})
}

func TestExecCommandTypedOrigins(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		output     string
		want       []string
		mustAbsent []string
	}{
		{
			name:    "quoted git text is not command",
			command: "printf 'git log -1\\n'",
			output:  "git log -1\n",
			mustAbsent: []string{
				"vcs_metadata",
				"command_measurement",
			},
		},
		{
			name:    "git show default is metadata and diff",
			command: "git show HEAD",
			output:  "commit abc\n--- a/file\n+++ b/file\n",
			want: []string{
				"evidence_origin=vcs_metadata",
				"diff_origin=vcs_diff",
			},
		},
		{
			name:    "git show no patch is metadata only",
			command: "git show --no-patch HEAD",
			output:  "commit abc\n",
			want: []string{
				"evidence_origin=vcs_metadata",
			},
			mustAbsent: []string{"diff_origin=vcs_diff"},
		},
		{
			name:    "git diff is diff evidence",
			command: "git -C . diff --stat",
			output:  " file.go | 2 +-\n",
			want: []string{
				"evidence_origin=vcs_diff",
			},
			mustAbsent: []string{"vcs_metadata"},
		},
		{
			name:    "git history count carries metadata and measurement",
			command: "git log --format=%H -20 -- internal/orchestrator | awk 'END { print \"answer_count=3\" }'",
			output:  "answer_count=3\n",
			want: []string{
				"evidence_origin=vcs_metadata",
				"measurement_origin=command_measurement",
				"measurement=count",
			},
		},
		{
			name:    "env wrapper is accepted",
			command: "GIT_CONFIG_NOSYSTEM=1 env git rev-list --count HEAD",
			output:  "7\n",
			want: []string{
				"evidence_origin=vcs_metadata",
				"measurement_origin=command_measurement",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execCommandTypedOriginLine(tt.command, tt.output)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("origin line missing %q for command %q: %q", want, tt.command, got)
				}
			}
			for _, absent := range tt.mustAbsent {
				if strings.Contains(got, absent) {
					t.Fatalf("origin line should not contain %q for command %q: %q", absent, tt.command, got)
				}
			}
		})
	}
}

func TestExecCommand_ReadModeShellWriteGate(t *testing.T) {
	t.Run("rejects heredoc redirection without writing", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := newBusContext()
		ctx.RepoRoot = tmpDir
		ctx.Mode = types.ModeRead
		ctx.PipelineStage = types.StageExplore
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{
			Command: "cat > docs/ARCHITECTURE.md << 'EOF'\nhello\nEOF",
		})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatalf("read-mode write redirection must be refused; got %q", result.Summary)
		}
		if !strings.Contains(result.Summary, "read-mode shell commands must be read-only") {
			t.Fatalf("expected read-only refusal, got %q", result.Summary)
		}
		if !strings.Contains(result.Summary, tmpDir) {
			t.Fatalf("refusal should tell the model the active repo command root %q, got %q", tmpDir, result.Summary)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "docs", "ARCHITECTURE.md")); !os.IsNotExist(err) {
			t.Fatalf("refused command must not create docs/ARCHITECTURE.md; stat err=%v", err)
		}
	})

	t.Run("rejects mutating commands", func(t *testing.T) {
		for _, command := range []string{
			"mkdir -p docs/diagrams",
			"printf hi | tee docs/out.txt",
			"find . -name '*.tmp' -delete",
			"git clean -fd",
			"sed -i 's/a/b/' file.txt",
			"find . -print0 | xargs -0 rm -f",
			"find . -print0 | xargs -0 git clean -fd",
			"find . -print0 | xargs -0 sed -i 's/a/b/'",
			"find . -print0 | xargs -p wc -l",
			"git -C /home/mindie log -1",
			"git --git-dir=/home/mindie/.git log -1",
			"GIT_DIR=/home/mindie/.git git log -1",
			"cd /home/mindie && git log -1",
			"cd ../other && git log -1",
			"git log -1 2>1",
			"git log -1 1>2",
		} {
			if err := validateReadOnlyExecCommand(command); err == nil {
				t.Fatalf("validateReadOnlyExecCommand(%q) unexpectedly allowed", command)
			}
		}
	})

	t.Run("allows read-only pipelines and quoted operators", func(t *testing.T) {
		for _, command := range []string{
			"printf 'a > b\\n' | wc -l",
			"find . -name '*.go' | wc -l",
			"find . -type f -name '*.go' -print0 | xargs -0 wc -l",
			"find . -type f -name '*.go' -print0 | xargs -0 -n 50 wc -l",
			"find . -type f -name '*.go' -print0 | xargs -0r --max-args=50 wc -l",
			"git status --short && git diff --stat",
			"git log -1 --format=%H 2>/dev/null || echo not_found",
			"git diff HEAD~1..HEAD --stat >/dev/null",
			"git show HEAD 2>&1 | head -20",
			"git -C . log -1",
			"git -C subdir log -1",
			"git --git-dir=.git log -1",
			"cd subdir && git log -1",
			"grep -R 'StageExplore' internal | head -5",
		} {
			if err := validateReadOnlyExecCommand(command); err != nil {
				t.Fatalf("validateReadOnlyExecCommand(%q) = %v, want allowed", command, err)
			}
		}
	})

	t.Run("write stage refuses shell mutation before execution", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := newBusContext()
		ctx.RepoRoot = tmpDir
		ctx.Mode = types.ModeApply
		ctx.PipelineStage = types.StageApply
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{
			Command: "mkdir -p out && printf hi > out/result.txt",
		})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			t.Fatalf("write-stage exec_command mutation must be refused; got %q", result.Summary)
		}
		if !strings.Contains(result.Summary, "write-mode shell commands are restricted to read-only") {
			t.Fatalf("expected write-mode refusal, got %q", result.Summary)
		}
		if strings.Contains(result.Summary, "read mode") {
			t.Fatalf("write-mode refusal should not leak read-mode wording, got %q", result.Summary)
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "out", "result.txt")); !os.IsNotExist(err) {
			t.Fatalf("refused write-stage command must not create file; stat err=%v", err)
		}
	})

	t.Run("write stage allows read-only observation", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := newBusContext()
		ctx.RepoRoot = tmpDir
		ctx.Mode = types.ModeApply
		ctx.PipelineStage = types.StageApply
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "pwd"})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("write-stage read-only exec_command should run; got %q", result.Summary)
		}
	})
}

func TestDecideWriteModeExecPermission(t *testing.T) {
	allowed := decideWriteModeExecPermission("pwd")
	if allowed.Action != safety.PermissionAllow || allowed.ReasonCode != "write_exec_observation" {
		t.Fatalf("pwd should be allowed observation, got %+v", allowed)
	}
	denied := decideWriteModeExecPermission("printf hi > out.txt")
	if denied.Action != safety.PermissionDeny || denied.ReasonCode != "write_exec_not_observation" {
		t.Fatalf("shell mutation should be denied, got %+v", denied)
	}
}

func TestEvalDisableGitHistoryGates(t *testing.T) {
	ctx := &types.BusContext{EvalDisableGitHistory: true}
	t.Run("exec command refuses git history", func(t *testing.T) {
		tool := &ExecCommand{}
		params, _ := json.Marshal(execCommandParams{Command: "git show HEAD"})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success || !strings.Contains(result.Summary, "--eval-disable-git-history") {
			t.Fatalf("git show should be refused by eval history gate, got success=%v summary=%q", result.Success, result.Summary)
		}
	})

	t.Run("structured history tools refuse", func(t *testing.T) {
		cases := []struct {
			name string
			tool interface {
				Execute(*types.BusContext, json.RawMessage) (types.ToolResult, error)
			}
			params any
		}{
			{"git_show", &GitShow{}, gitShowParams{Ref: "HEAD"}},
			{"git_log", &GitLog{}, gitLogParams{Count: 1}},
			{"git_history_search", &GitHistorySearch{}, gitHistorySearchParams{WindowPath: ".", Contains: "needle"}},
			{"git_diff_ref", &GitDiff{}, gitDiffParams{Ref: "HEAD~1..HEAD"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				params, _ := json.Marshal(tc.params)
				result, err := tc.tool.Execute(ctx, params)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Success || !strings.Contains(result.Summary, "--eval-disable-git-history") {
					t.Fatalf("%s should be refused by eval history gate, got success=%v summary=%q", tc.name, result.Success, result.Summary)
				}
			})
		}
	})

	t.Run("working tree git diff remains available", func(t *testing.T) {
		diffCtx := gitWorktreeFixture(t, "old\n")
		diffCtx.EvalDisableGitHistory = true
		if err := os.WriteFile(filepath.Join(diffCtx.RepoRoot, "file.txt"), []byte("old\nnew\n"), 0o644); err != nil {
			t.Fatalf("modify fixture file: %v", err)
		}
		params, _ := json.Marshal(gitDiffParams{NameOnly: true})
		result, err := (&GitDiff{}).Execute(diffCtx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success || !strings.Contains(result.Summary, "file.txt") {
			t.Fatalf("working-tree git_diff should remain available, got success=%v summary=%q", result.Success, result.Summary)
		}
	})
}

// fakeActiveSetGater is a stand-in for *multigraph.MultiGraph that
// can't be imported here without a cycle (internal/tool/repomap →
// internal/tool). We only need to verify that ExecCommand wires
// the gate in — the multigraph package's own tests cover the
// command-classification logic.
type fakeActiveSetGater struct {
	pathFn func(*types.BusContext, string, string, func(string) bool) types.ActiveSetGateResult
	cmdFn  func(*types.BusContext, string, string) types.ActiveSetGateResult
}

func (f *fakeActiveSetGater) ResolveActiveSetPath(ctx *types.BusContext, toolName, llmPath string, fileExists func(string) bool) types.ActiveSetGateResult {
	if f.pathFn != nil {
		return f.pathFn(ctx, toolName, llmPath, fileExists)
	}
	return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
}

func (f *fakeActiveSetGater) ResolveActiveSetCommand(ctx *types.BusContext, toolName, command string) types.ActiveSetGateResult {
	if f.cmdFn != nil {
		return f.cmdFn(ctx, toolName, command)
	}
	return types.ActiveSetGateResult{Allowed: true}
}

func TestExecCommand_ActiveSetGateRefusesInactive(t *testing.T) {
	// When the gate refuses, ExecCommand must short-circuit with
	// the gate's RefusalProse as Summary. The shell command must
	// NOT execute (we'd see "should-not-run-here" in stdout).
	gater := &fakeActiveSetGater{
		cmdFn: func(_ *types.BusContext, toolName, _ string) types.ActiveSetGateResult {
			return types.ActiveSetGateResult{
				Allowed:      false,
				RefusalProse: toolName + " refused: the command references \"repo-greet-go\", which is currently outside the active sub-repo set. The active sub-repos are: repo-stub-rust. If your investigation needs that sub-repo, surface the requirement in plain prose at the top of your final answer — the user will adjust the workspace scope and re-ask.",
			}
		},
	}
	ctx := &types.BusContext{MultiGraph: gater}
	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: "echo should-not-run-here"})

	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("gate refusal must produce Success=false; got Summary=%q", result.Summary)
	}
	if !strings.Contains(result.Summary, "exec_command refused") {
		t.Errorf("expected refusal text in Summary; got %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "outside the active sub-repo") {
		t.Errorf("expected canonical active-set phrasing; got %q", result.Summary)
	}
	if strings.Contains(result.Summary, "should-not-run-here") {
		t.Errorf("shell ran despite refusal; got %q", result.Summary)
	}
	// R6: the gate refusal must not leak internal pipeline terms
	// (no "L1", "L2" layer designators, no Go-style PascalCase names).
	for _, banned := range []string{"L1 gate", "L2 gate", "L3 gate", "L4 gate", "MultiRepoActiveSetGater", "BusContext"} {
		if strings.Contains(result.Summary, banned) {
			t.Errorf("internal term %q leaked into LLM-facing refusal: %q", banned, result.Summary)
		}
	}
}

func TestExecCommand_ActiveSetGateAllowsLetsCommandRun(t *testing.T) {
	// When the gate allows the call, the shell must run normally.
	gater := &fakeActiveSetGater{
		cmdFn: func(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
			return types.ActiveSetGateResult{Allowed: true}
		},
	}
	ctx := &types.BusContext{MultiGraph: gater}
	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: "echo gate-passed"})

	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("gate allow → command should run; got Summary=%q", result.Summary)
	}
	normalized := strings.ReplaceAll(result.Summary, "\r\n", "\n")
	if !strings.Contains(normalized, "gate-passed\n") {
		t.Errorf("expected command stdout 'gate-passed', got %q", result.Summary)
	}
}

func TestExecCommand_NoMultiGraph_NoGate(t *testing.T) {
	// Single-repo (or no multigraph at all): the gate is bypassed
	// — exec_command must run unchanged for backwards compat.
	tool := &ExecCommand{}
	params, _ := json.Marshal(execCommandParams{Command: "echo single-repo-mode"})
	result, err := tool.Execute(newBusContext(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("nil multigraph → command should run; got Summary=%q", result.Summary)
	}
	normalized := strings.ReplaceAll(result.Summary, "\r\n", "\n")
	if !strings.Contains(normalized, "single-repo-mode\n") {
		t.Errorf("expected stdout, got %q", result.Summary)
	}
}

func TestGrepShouldUseNativeBackend(t *testing.T) {
	tests := []struct {
		name             string
		backend          string
		hasRepoRoot      bool
		searchPathIsFile bool
		want             bool
	}{
		{
			name:             "native fallback always uses Go walker",
			backend:          "native",
			hasRepoRoot:      true,
			searchPathIsFile: true,
			want:             true,
		},
		{
			name:             "system grep handles explicit file targets",
			backend:          "grep",
			hasRepoRoot:      true,
			searchPathIsFile: true,
			want:             false,
		},
		{
			name:             "system grep directory search keeps native path-aware exclusions",
			backend:          "grep",
			hasRepoRoot:      true,
			searchPathIsFile: false,
			want:             true,
		},
		{
			name:             "grep without repo root shells out",
			backend:          "grep",
			hasRepoRoot:      false,
			searchPathIsFile: false,
			want:             false,
		},
		{
			name:             "ripgrep never uses native",
			backend:          "rg",
			hasRepoRoot:      true,
			searchPathIsFile: false,
			want:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grepShouldUseNativeBackend(tt.backend, tt.hasRepoRoot, tt.searchPathIsFile)
			if got != tt.want {
				t.Fatalf("grepShouldUseNativeBackend(%q, %v, %v) = %v, want %v", tt.backend, tt.hasRepoRoot, tt.searchPathIsFile, got, tt.want)
			}
		})
	}
}

func TestGrepTool(t *testing.T) {
	t.Run("grep for pattern in file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "sample.txt")
		content := "alpha\nbeta\ngamma\ndelta\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "gamma", Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "gamma") {
			t.Fatalf("expected output to contain 'gamma', got: %s", result.Summary)
		}
	})

	t.Run("smart-case lowercase pattern matches CamelCase", func(t *testing.T) {
		// Regression: a grep for "subagent" used to silently miss
		// SubAgent / SubAgentRegistry / RegisterDefaultSubAgents because
		// the tool ran case-sensitive grep with no -i. Now an all-lowercase
		// pattern is treated as case-blind (ripgrep -S behavior).
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "code.go")
		content := "type SubAgent interface{}\nfunc RegisterDefaultSubAgents() {}\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "subagent", Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "SubAgent") {
			t.Fatalf("expected smart-case to match SubAgent, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "RegisterDefaultSubAgents") {
			t.Fatalf("expected smart-case to match RegisterDefaultSubAgents, got: %s", result.Summary)
		}
	})

	t.Run("smart-case uppercase pattern is exact", func(t *testing.T) {
		// Pattern containing any uppercase character keeps the original
		// case-sensitive behavior, so the LLM can still pin a specific
		// casing when it needs to.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "code.go")
		content := "lowercase subagent here\nupper SubAgent here\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "SubAgent", Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "upper SubAgent here") {
			t.Fatalf("expected exact-case match, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "lowercase subagent here") {
			t.Fatalf("uppercase pattern should NOT match lowercase, got: %s", result.Summary)
		}
	})

	t.Run("fixed string literal handles log punctuation safely", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "trace.log")
		content := "[GT]codraxNode1>prio=20\nGTcodraxNode1 prio=20\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{
			Pattern:     "[GT]codraxNode1>prio=20",
			Path:        tmpFile,
			FixedString: true,
		})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "fixed_string=true") {
			t.Fatalf("params banner should expose fixed_string=true:\n%s", result.Summary)
		}
		if !strings.Contains(result.Summary, "[GT]codraxNode1>prio=20") {
			t.Fatalf("expected literal match in output:\n%s", result.Summary)
		}
		if strings.Contains(result.Summary, "GTcodraxNode1 prio=20") {
			t.Fatalf("fixed_string must not treat [] as regex char class:\n%s", result.Summary)
		}
	})

	t.Run("single file line window narrows log search", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "events.log")
		content := "needle before\nnoise\nneedle in window\nneedle after\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{
			Pattern:   "needle",
			Path:      tmpFile,
			LineStart: 2,
			LineEnd:   3,
		})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		for _, want := range []string{"line_start=2", "line_end=3", "needle in window"} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("line-window grep missing %q:\n%s", want, result.Summary)
			}
		}
		if strings.Contains(result.Summary, "before") || strings.Contains(result.Summary, "after") {
			t.Fatalf("line-window grep leaked outside range:\n%s", result.Summary)
		}
	})

	t.Run("context lines banner says output lines include context", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "sample.log")
		content := "before\nneedle\n after\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		contextLines := 1
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{
			Pattern:      "needle",
			Path:         tmpFile,
			ContextLines: &contextLines,
		})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "matching lines (including context output lines)") {
			t.Fatalf("context grep banner should disambiguate output lines:\n%s", result.Summary)
		}
	})

	t.Run("runtime artifact no match teaches literal and line-window recovery", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "record_trace.systrace")
		if err := os.WriteFile(tmpFile, []byte("2942.124002: sched_switch\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{
			Pattern: `2942\.\d{3} MissingEvent`,
			Path:    tmpFile,
		})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		for _, want := range []string{
			"no_match_advisory=single runtime/log/trace artifact searched",
			"split combined patterns",
			"preserve the observed field order",
			"fixed_string=true",
			"line_start/line_end",
			"grep -n",
			"regex_compatibility_note=",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("runtime no-match hint missing %q:\n%s", want, result.Summary)
			}
		}
		if strings.Contains(result.Summary, "relation_map") {
			t.Fatalf("runtime artifact no-match should not suggest repo relation_map:\n%s", result.Summary)
		}
	})

	t.Run("runtime artifact trace marker begin points to span_window", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "record_trace.systrace")
		trace := strings.Join([]string{
			`my.carlist.www-49209 (11029) [005] .... 1858.767865: tracing_mark_write: B|11029|bindApplication`,
			`my.carlist.www-49209 (11029) [010] .... 1858.770335: tracing_mark_write: E|11029`,
		}, "\n")
		if err := os.WriteFile(tmpFile, []byte(trace), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{
			Pattern:     "bindApplication",
			Path:        tmpFile,
			FixedString: true,
		})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{
			"trace_marker_span_begin_hint=",
			`span_name="bindApplication"`,
			`trace_query(view="span_window"`,
			`path="` + tmpFile + `"`,
			"line_start=1",
			"do not infer the end by grepping for E|<pid>|<span_name>",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("trace marker begin hint missing %q:\n%s", want, result.Summary)
			}
		}
	})

	t.Run("runtime artifact named end no match is not missing span proof", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "record_trace.systrace")
		trace := strings.Join([]string{
			`my.carlist.www-49209 (11029) [005] .... 1858.767865: tracing_mark_write: B|11029|bindApplication`,
			`my.carlist.www-49209 (11029) [010] .... 1858.770335: tracing_mark_write: E|11029`,
		}, "\n")
		if err := os.WriteFile(tmpFile, []byte(trace), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{
			Pattern:     "E|11029|bindApplication",
			Path:        tmpFile,
			FixedString: true,
		})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{
			"no matches found",
			"trace_marker_span_end_shape=",
			`span_name="bindApplication"`,
			"not evidence that the span never ended",
			`trace_query(view="span_window"`,
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("named-end no-match hint missing %q:\n%s", want, result.Summary)
			}
		}
	})

	t.Run("skipped large file hint names capped candidates", func(t *testing.T) {
		got := grepSkippedLargeFilePathHint([]string{"record_trace.systrace", "large.log"}, 3)
		for _, want := range []string{`"record_trace.systrace"`, `"large.log"`, "+1 more"} {
			if !strings.Contains(got, want) {
				t.Fatalf("skipped-large-file hint missing %q: %q", want, got)
			}
		}
	})

	t.Run("skipped large runtime artifact no match gives explicit single-file recovery", func(t *testing.T) {
		got := grepSkippedLargeFilesNoMatchBody([]string{"record_trace.systrace", "huge.generated"}, 2)
		for _, want := range []string{
			"searched_subset_no_matches=true",
			"skipped_large_candidates=\"record_trace.systrace\",\"huge.generated\"",
			"directory_scan_safety_skip=",
			"not absence proof",
			"single_file_grep_supported=true",
			`next_call=grep(path="record_trace.systrace"`,
			"files_only=false",
			"context_lines=0",
			"do not start read_file at the file head",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("skipped-runtime recovery missing %q:\n%s", want, got)
			}
		}
		refinement := grepSkippedLargeRefinement([]string{"record_trace.systrace", "huge.generated"}, 2)
		if refinement == nil {
			t.Fatal("expected typed refinement")
		}
		if refinement.ReasonCode != "skipped_large_candidates" || refinement.PreferredNextTool != "grep" {
			t.Fatalf("unexpected skipped-large refinement: %+v", refinement)
		}
		if got := refinement.PreferredParams["path"]; got != "record_trace.systrace" {
			t.Fatalf("preferred path = %q", got)
		}
		if _, ok := refinement.PreferredParams["pattern"]; ok {
			t.Fatalf("pattern should remain model-selected, not a placeholder param: %+v", refinement.PreferredParams)
		}
		if len(refinement.RequiredFields) != 1 || refinement.RequiredFields[0] != "pattern" {
			t.Fatalf("required fields = %+v", refinement.RequiredFields)
		}
	})

	t.Run("explicit ignore_case false overrides smart-case", func(t *testing.T) {
		// The LLM can force exact-case matching even on a lowercase
		// pattern by passing ignore_case=false explicitly.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "code.go")
		content := "lowercase subagent here\nupper SubAgent here\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		falseVal := false
		params, _ := json.Marshal(grepToolParams{Pattern: "subagent", Path: tmpDir, IgnoreCase: &falseVal})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "lowercase subagent here") {
			t.Fatalf("expected lowercase match, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "SubAgent here") {
			t.Fatalf("ignore_case=false should NOT match SubAgent, got: %s", result.Summary)
		}
	})

	// ERE regex tests: LLMs write ERE/PCRE patterns naturally.
	// Without -E, grep uses BRE where ?, +, {}, () are literal.

	t.Run("ERE question-mark quantifier", func(t *testing.T) {
		// Regression: "sub[-]?agent" matched literal "sub-?agent" in BRE
		// instead of "subagent" or "sub-agent" as the LLM intended.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "code.go")
		content := "type SubAgent struct{}\nfunc NewSub_Agent() {}\nvar sub_agent = 1\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "sub[-_]?agent", Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "SubAgent") {
			t.Fatalf("expected ? quantifier to match SubAgent (zero separator), got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "sub_agent") {
			t.Fatalf("expected ? quantifier to match sub_agent (underscore separator), got: %s", result.Summary)
		}
	})

	t.Run("ERE plus quantifier", func(t *testing.T) {
		// LLMs use + for "one or more", e.g. "func\\s+\\w+"
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "code.go")
		content := "func Run() {}\nfunc  Execute() {}\nfuncNope() {}\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: `func +[A-Z]`, Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		// + means one or more spaces: "func Run" and "func  Execute" match
		if !strings.Contains(result.Summary, "func Run") {
			t.Fatalf("expected + to match 'func Run', got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "func  Execute") {
			t.Fatalf("expected + to match 'func  Execute', got: %s", result.Summary)
		}
		// "funcNope" has zero spaces — should NOT match
		if strings.Contains(result.Summary, "funcNope") {
			t.Fatalf("+ should require at least one space, but matched funcNope: %s", result.Summary)
		}
	})

	t.Run("ERE alternation with parentheses", func(t *testing.T) {
		// LLMs use (a|b) for alternation
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "code.go")
		content := "var ErrorNotFound = 1\nvar ErrorTimeout = 2\nvar WarningDeprecated = 3\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: `(NotFound|Timeout)`, Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "ErrorNotFound") {
			t.Fatalf("expected alternation to match ErrorNotFound, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "ErrorTimeout") {
			t.Fatalf("expected alternation to match ErrorTimeout, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "WarningDeprecated") {
			t.Fatalf("alternation should not match WarningDeprecated, got: %s", result.Summary)
		}
	})

	t.Run("ERE brace repetition", func(t *testing.T) {
		// LLMs use {n,m} for bounded repetition, e.g. "[0-9]{2,4}"
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "data.txt")
		content := "id: 1\nid: 42\nid: 123\nid: 9999\nid: 55555\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: `[0-9]{2,4}`, Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		// {2,4} should match 2-4 digit numbers
		if !strings.Contains(result.Summary, "42") {
			t.Fatalf("expected {2,4} to match 42, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "123") {
			t.Fatalf("expected {2,4} to match 123, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "9999") {
			t.Fatalf("expected {2,4} to match 9999, got: %s", result.Summary)
		}
		// "1" is only 1 digit — should NOT match
		if strings.Contains(result.Summary, "id: 1\n") {
			t.Fatalf("{2,4} should not match single digit, got: %s", result.Summary)
		}
	})

	t.Run("ERE works with files_only mode", func(t *testing.T) {
		// Verify ERE is active in -rl mode too (Phase 1 breadth scan)
		tmpDir := t.TempDir()
		f1 := filepath.Join(tmpDir, "match.go")
		f2 := filepath.Join(tmpDir, "nomatch.go")
		if err := os.WriteFile(f1, []byte("type SubAgent struct{}\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(f2, []byte("type Config struct{}\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "sub[-_]?agent", Path: tmpDir, FilesOnly: true})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "match.go") {
			t.Fatalf("expected files_only + ERE to list match.go, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "nomatch.go") {
			t.Fatalf("files_only should not list nomatch.go, got: %s", result.Summary)
		}
	})

	t.Run("analyze files_only separates production and auxiliary matches", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "src", "App.ets"), []byte("const marker = \"NeedleRole\"\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "src", "App.test.ets"), []byte("const marker = \"NeedleRole\"\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		ctx.PipelineStage = types.StageAnalyze
		ctx.RepoRoot = tmpDir
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "NeedleRole", Path: ".", FilesOnly: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		prodIdx := strings.Index(result.Summary, "[prescan production matches]")
		auxIdx := strings.Index(result.Summary, "[prescan auxiliary matches - not production proof]")
		if prodIdx < 0 || auxIdx < 0 || auxIdx <= prodIdx {
			t.Fatalf("expected production and auxiliary sections in order, got:\n%s", result.Summary)
		}
		prodPathIdx := strings.Index(result.Summary, "src/App.ets")
		auxPathIdx := strings.Index(result.Summary, "src/App.test.ets")
		if prodPathIdx < 0 || auxPathIdx < 0 || !(prodIdx < prodPathIdx && prodPathIdx < auxIdx && auxIdx < auxPathIdx) {
			t.Fatalf("expected ArkTS production path before auxiliary test path, got:\n%s", result.Summary)
		}
	})

	t.Run("file_type filters repomap languages without rg native type support", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "src", "Entry.ets"), []byte("@Entry\nstruct Entry { build() {} }\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "src", "Entry.ts"), []byte("@Entry\nexport class EntryTs {}\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "src", "Service.cj"), []byte("public class Service {}\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		ctx.RepoRoot = tmpDir
		tool := &GrepTool{}

		params, _ := json.Marshal(grepToolParams{Pattern: "@Entry", Path: ".", FileType: "arkts", FilesOnly: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected arkts error: %v", err)
		}
		if !result.Success {
			t.Fatalf("arkts search should not fail on backend type support; got:\n%s", result.Summary)
		}
		if !strings.Contains(result.Summary, "src/Entry.ets") {
			t.Fatalf("arkts file_type should include .ets files, got:\n%s", result.Summary)
		}
		if strings.Contains(result.Summary, "src/Entry.ts") {
			t.Fatalf("arkts file_type should not broaden to ordinary .ts without a project probe, got:\n%s", result.Summary)
		}

		params, _ = json.Marshal(grepToolParams{Pattern: "Service", Path: ".", FileType: "cangjie", FilesOnly: true})
		result, err = tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected cangjie error: %v", err)
		}
		if !result.Success {
			t.Fatalf("cangjie search should not fail on backend type support; got:\n%s", result.Summary)
		}
		if !strings.Contains(result.Summary, "src/Service.cj") {
			t.Fatalf("cangjie file_type should include .cj files, got:\n%s", result.Summary)
		}
	})

	t.Run("analyze files_only marks auxiliary-only matches without production proof", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmpDir, "tests", "cangjie"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "tests", "cangjie", "feature_test.cj"), []byte("let marker = \"NeedleOnly\"\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		ctx.PipelineStage = types.StageAnalyze
		ctx.RepoRoot = tmpDir
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "NeedleOnly", Path: ".", FilesOnly: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "no non-auxiliary matches found") ||
			!strings.Contains(result.Summary, "[prescan auxiliary matches - not production proof]") ||
			!strings.Contains(result.Summary, "tests/cangjie/feature_test.cj") {
			t.Fatalf("expected Cangjie test-only match to be labelled auxiliary-only, got:\n%s", result.Summary)
		}
	})

	t.Run("analyze line grep labels auxiliary-only context output", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmpDir, "src", "__tests__"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "src", "__tests__", "Widget.spec.ets"), []byte("const marker = \"NeedleLine\"\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		ctx.PipelineStage = types.StageAnalyze
		ctx.RepoRoot = tmpDir
		tool := &GrepTool{}
		contextLines := 1
		params, _ := json.Marshal(grepToolParams{Pattern: "NeedleLine", Path: ".", ContextLines: &contextLines})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "no non-auxiliary matches found") ||
			!strings.Contains(result.Summary, "[prescan auxiliary matches - not production proof]") ||
			!strings.Contains(result.Summary, "Widget.spec.ets") {
			t.Fatalf("expected ArkTS test-only line grep to be labelled auxiliary-only, got:\n%s", result.Summary)
		}
	})

	t.Run("explore grep separates production and auxiliary matches", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(tmpDir, "tests", "cangjie"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "src", "feature.cj"), []byte("let marker = \"NeedleScope\"\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "tests", "cangjie", "feature_test.cj"), []byte("let marker = \"NeedleScope\"\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		ctx.PipelineStage = types.StageExplore
		ctx.RepoRoot = tmpDir
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "NeedleScope", Path: ".", FilesOnly: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		prodIdx := strings.Index(result.Summary, "[grep production matches]")
		auxIdx := strings.Index(result.Summary, "[grep auxiliary matches - supporting context, not production proof]")
		if prodIdx < 0 || auxIdx < 0 || auxIdx <= prodIdx {
			t.Fatalf("expected generic grep production and auxiliary sections in order, got:\n%s", result.Summary)
		}
		if strings.Contains(result.Summary, "[prescan production matches]") {
			t.Fatalf("explore grep should not use analyzer-prescan labels, got:\n%s", result.Summary)
		}
		prodPathIdx := strings.Index(result.Summary, "src/feature.cj")
		auxPathIdx := strings.Index(result.Summary, "tests/cangjie/feature_test.cj")
		if prodPathIdx < 0 || auxPathIdx < 0 || !(prodIdx < prodPathIdx && prodPathIdx < auxIdx && auxIdx < auxPathIdx) {
			t.Fatalf("expected Cangjie production path before auxiliary test path, got:\n%s", result.Summary)
		}
	})

	t.Run("files_only broad result is compacted with raw blob preserved", func(t *testing.T) {
		tmpDir := t.TempDir()
		for i := 0; i < 130; i++ {
			dir := filepath.Join(tmpDir, "src", "pkg")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("setup: %v", err)
			}
			ext := ".go"
			if i%3 == 1 {
				ext = ".java"
			} else if i%3 == 2 {
				ext = ".cj"
			}
			name := filepath.Join(dir, "Prod"+strconv.Itoa(i)+ext)
			if err := os.WriteFile(name, []byte("const WideNeedle = true\n"), 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}
		for i := 0; i < 35; i++ {
			dir := filepath.Join(tmpDir, "tests", "feature")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("setup: %v", err)
			}
			name := filepath.Join(dir, "Feature"+strconv.Itoa(i)+".test.ts")
			if err := os.WriteFile(name, []byte("const WideNeedle = true\n"), 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}
		if err := os.MkdirAll(filepath.Join(tmpDir, "docs"), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "docs", "wide.md"), []byte("WideNeedle\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		ctx.PipelineStage = types.StageExplore
		ctx.RepoRoot = tmpDir
		ctx.WorkDir = t.TempDir()
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "WideNeedle", Path: ".", FilesOnly: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		for _, want := range []string{
			"[grep retrieval governor]",
			"decision=broad_result_compacted mode=files_only",
			"[grep production matches]",
			"[grep auxiliary matches - supporting context, not production proof]",
			"omitted",
			"full_raw_saved=",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("expected %q in compacted summary:\n%s", want, result.Summary)
			}
		}
		if result.RawRef == "" {
			t.Fatalf("expected raw ref for compacted broad result")
		}
		if _, err := os.Stat(result.RawRef); err != nil {
			t.Fatalf("expected raw blob to exist: %v", err)
		}
		prodIdx := strings.Index(result.Summary, "[grep production matches]")
		auxIdx := strings.Index(result.Summary, "[grep auxiliary matches - supporting context, not production proof]")
		if prodIdx < 0 || auxIdx < 0 || prodIdx > auxIdx {
			t.Fatalf("expected production section before auxiliary section:\n%s", result.Summary)
		}
	})

	t.Run("line broad result is compacted without losing exact raw output", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "src", "main", "java", "app")
		testDir := filepath.Join(tmpDir, "src", "test", "java", "app")
		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.MkdirAll(testDir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		var prod strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&prod, "String value%d = \"LineWideNeedle prod %03d\";\n", i, i)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "Controller.java"), []byte(prod.String()), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		var testContent strings.Builder
		for i := 0; i < 30; i++ {
			fmt.Fprintf(&testContent, "String value%d = \"LineWideNeedle test %03d\";\n", i, i)
		}
		if err := os.WriteFile(filepath.Join(testDir, "ControllerTest.java"), []byte(testContent.String()), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := newBusContext()
		ctx.PipelineStage = types.StageExplore
		ctx.RepoRoot = tmpDir
		ctx.WorkDir = t.TempDir()
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "LineWideNeedle", Path: "."})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "decision=broad_result_compacted mode=line_output") {
			t.Fatalf("expected line-output compaction:\n%s", result.Summary)
		}
		if strings.Contains(result.Summary, "prod 099") {
			t.Fatalf("late production line should be omitted from inline compacted summary:\n%s", result.Summary)
		}
		if result.RawRef == "" {
			t.Fatalf("expected raw ref for compacted line output")
		}
		raw, err := os.ReadFile(result.RawRef)
		if err != nil {
			t.Fatalf("read raw blob: %v", err)
		}
		if !strings.Contains(string(raw), "prod 099") || !strings.Contains(string(raw), "ControllerTest.java") {
			t.Fatalf("raw blob should preserve omitted production and auxiliary output:\n%s", string(raw))
		}
		if result.Refinement == nil {
			t.Fatalf("expected typed refinement for compacted grep result")
		}
		if result.Refinement.ReasonCode != "grep_result_truncated" || !result.Refinement.ResultTruncated {
			t.Fatalf("unexpected refinement: %+v", result.Refinement)
		}
		if result.Refinement.PreferredNextTool != "grep" {
			t.Fatalf("preferred tool = %q", result.Refinement.PreferredNextTool)
		}
		if got := result.Refinement.PreferredParams["path"]; got != "./src/main/java/app/Controller.java" {
			t.Fatalf("preferred path = %q; refinement=%+v", got, result.Refinement)
		}
	})

	t.Run("broad relation-shaped grep suggests relation map without making it mandatory", func(t *testing.T) {
		ctx := newBusContext()
		ctx.AnalysisIR = &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				PredicateAxis: types.AxisCall,
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqCallChain),
					Keywords: []string{"update_elem"},
				},
			},
		}
		var raw strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&raw, "src/dispatch/file%03d.go:%d:func target%d() { update_elem() }\n", i%7, i+1, i)
		}

		got, _, ok := compactBroadGrepOutput(ctx, grepToolParams{Pattern: "update_elem"}, "", "", raw.String(), raw.String())
		if !ok {
			t.Fatalf("expected broad grep to compact")
		}
		for _, want := range []string{
			"relation_navigation_hint=",
			`repo_map(view="relation_map"`,
			`sources=["src/dispatch/file000.go", "src/dispatch/file001.go", "src/dispatch/file002.go"]`,
			`relation_kinds=["called-by"]`,
			"This is only a navigation hint",
			"not absence proof",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("broad relation grep missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("broad runtime artifact grep does not suggest repo relation map", func(t *testing.T) {
		ctx := newBusContext()
		ctx.AnalysisIR = &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				PredicateAxis: types.AxisCall,
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqCallChain),
					Keywords: []string{"sched_switch"},
				},
			},
		}
		var raw strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&raw, "record_trace.systrace:%d: com.tencent.mm-36379 (36379) [004] .... 2942.%06d: sched_switch: prev_state=S ==> next_comm=main\n", 1000+i, i)
		}
		contextLines := 3

		got, _, ok := compactBroadGrepOutput(ctx, grepToolParams{
			Pattern:      "com.tencent.mm-36379",
			Path:         "record_trace.systrace",
			ContextLines: &contextLines,
		}, "[grep: 100 matching lines]\n", "[grep params: pattern=com.tencent.mm-36379 path=record_trace.systrace context_lines=3]\n", raw.String(), raw.String())
		if !ok {
			t.Fatalf("expected broad grep to compact")
		}
		for _, want := range []string{
			"next_shape=single large runtime artifact matched too broadly",
			"narrow with one exact timestamp/literal/thread id",
			"read_file around the returned line numbers",
			"preserves original line numbers",
			"line_window_hint=first returned match is record_trace.systrace:1000",
			"path=\"record_trace.systrace\" line_offset=979 limit=41",
			"line_start=980 line_end=1020",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("runtime-artifact broad grep missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "relation_navigation_hint=") || strings.Contains(got, `repo_map(view="relation_map"`) {
			t.Fatalf("runtime artifact grep must not suggest repo relation_map:\n%s", got)
		}
	})

	t.Run("broad runtime artifact grep emits multiple line windows", func(t *testing.T) {
		ctx := newBusContext()
		var raw strings.Builder
		for _, base := range []int{1000, 5000, 9000} {
			for i := 0; i < 30; i++ {
				fmt.Fprintf(&raw, "record_trace.systrace:%d: com.tencent.mm-36379 (36379) [004] .... 2942.%06d: sched_wakeup\n", base+i, base+i)
			}
		}
		got, _, ok := compactBroadGrepOutput(ctx, grepToolParams{
			Pattern: "com.tencent.mm-36379",
			Path:    "record_trace.systrace",
		}, "[grep: 90 matching lines]\n", "[grep params: pattern=com.tencent.mm-36379 path=record_trace.systrace]\n", raw.String(), raw.String())
		if !ok {
			t.Fatalf("expected broad grep to compact")
		}
		for _, want := range []string{
			"line_window_hint=first returned match is record_trace.systrace:1000",
			"line_windows=record_trace.systrace:980-1049(matches=30); record_trace.systrace:4980-5049(matches=30); record_trace.systrace:8980-9049(matches=30)",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("runtime-artifact broad grep missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("line window parser ignores numeric hyphen path segments", func(t *testing.T) {
		line := `.codrax/blob/20260530-095935-000-40270/grep-full-afd155fa.txt:130149:  com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch`
		path, lineNo, match, ok := parseGrepOutputLineLocation(line)
		if !ok || !match {
			t.Fatalf("expected grep location, got ok=%v match=%v path=%q line=%d", ok, match, path, lineNo)
		}
		if path != ".codrax/blob/20260530-095935-000-40270/grep-full-afd155fa.txt" || lineNo != 130149 {
			t.Fatalf("wrong parse: path=%q line=%d", path, lineNo)
		}
	})

	t.Run("fixed string regex-looking no-match gives recovery note", func(t *testing.T) {
		dir := t.TempDir()
		tracePath := filepath.Join(dir, "record_trace.systrace")
		if err := os.WriteFile(tracePath, []byte("com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ctx := newBusContext()
		params, _ := json.Marshal(grepToolParams{
			Pattern:     `com.tencent.mm-36379.*sched_switch`,
			Path:        tracePath,
			FixedString: true,
		})
		res, err := (&GrepTool{}).Execute(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"no matches found",
			"fixed_string_regex_note=",
			"fixed_string=true treats regex syntax as literal text",
			".*",
			"fixed_string=false",
		} {
			if !strings.Contains(res.Summary, want) {
				t.Fatalf("fixed-string regex advisory missing %q:\n%s", want, res.Summary)
			}
		}
	})

	t.Run("runtime artifact grep surfaces parameter advisory", func(t *testing.T) {
		ctx := newBusContext()
		contextLines := 3
		var raw strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&raw, "record_trace.systrace:%d: com.tencent.mm-36379 (36379) [004] .... 2942.%06d: sched_switch\n", 1000+i, i)
		}
		got, _, ok := compactBroadGrepOutput(ctx, grepToolParams{
			Pattern:      "com.tencent.mm-36379",
			Path:         "record_trace.systrace",
			FileType:     "config",
			ContextLines: &contextLines,
		}, "[grep: 100 matching lines (including context output lines)]\n", "[grep params: pattern=com.tencent.mm-36379 path=record_trace.systrace file_type=config context_lines=3]\n", raw.String(), raw.String())
		if !ok {
			t.Fatalf("expected broad grep to compact")
		}
		for _, want := range []string{
			"artifact_search_advisory=",
			"file_type/include filters are redundant",
			"context_lines expands broad artifact searches",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("runtime-artifact advisory missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("broad runtime artifact grep preserves full raw output without inline flood", func(t *testing.T) {
		tmpDir := t.TempDir()
		tracePath := filepath.Join(tmpDir, "record_trace.systrace")
		var raw strings.Builder
		for i := 0; i < 120; i++ {
			fmt.Fprintf(&raw, "record_trace.systrace:%d: com.tencent.mm-36379 (36379) [004] .... 2942.%06d: sched_switch: prev_state=S ==> next_comm=main\n", 1000+i, i)
		}
		if err := os.WriteFile(tracePath, []byte(raw.String()), 0o644); err != nil {
			t.Fatalf("write trace: %v", err)
		}

		ctx := newBusContext()
		ctx.WorkDir = t.TempDir()
		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{
			Pattern: "com.tencent.mm-36379",
			Path:    tracePath,
		})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		for _, want := range []string{
			"[grep retrieval governor]",
			"decision=broad_result_compacted mode=line_output",
			"full_raw_saved=",
			"next_shape=single large runtime artifact matched too broadly",
			"line_window_hint=first returned match is",
			"line_start=1 line_end=21",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("broad runtime summary missing %q:\n%s", want, result.Summary)
			}
		}
		if strings.Contains(result.Summary, "2942.000119") {
			t.Fatalf("late line should not flood inline summary:\n%s", result.Summary)
		}
		if result.RawRef == "" {
			t.Fatalf("expected raw ref for broad runtime artifact grep")
		}
		full, err := os.ReadFile(result.RawRef)
		if err != nil {
			t.Fatalf("read raw ref: %v", err)
		}
		if !strings.Contains(string(full), "2942.000119") {
			t.Fatalf("raw ref must preserve omitted late lines:\n%s", string(full))
		}
	})

	t.Run("broad non-relation grep does not suggest relation map", func(t *testing.T) {
		ctx := newBusContext()
		ctx.AnalysisIR = &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentReturnValue},
		}
		var raw strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&raw, "src/config/file%03d.go:%d:return ConfigValue%d\n", i%5, i+1, i)
		}

		got, _, ok := compactBroadGrepOutput(ctx, grepToolParams{Pattern: "ConfigValue"}, "", "", raw.String(), raw.String())
		if !ok {
			t.Fatalf("expected broad grep to compact")
		}
		if strings.Contains(got, "relation_navigation_hint=") || strings.Contains(got, `repo_map(view="relation_map"`) {
			t.Fatalf("non-relation broad grep should not nudge relation_map:\n%s", got)
		}
	})

	t.Run("broad result ranks structured relevance before generic production noise", func(t *testing.T) {
		ctx := newBusContext()
		ctx.Mutable = types.NewMutableState("relation lookup")
		ctx.AnalysisIR = &types.AnalysisIR{
			EvidencePlan: types.EvidencePlan{
				RequiredFiles: []string{"internal/agent/subagent.go"},
			},
		}
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
			ReadFiles: []string{"internal/agent/sub_explorer.go"},
			EvidenceItems: []types.EvidenceItem{{
				Source: "internal/agent/subagent_runtime.go",
			}},
		})
		ctx.Mutable.SetPhase1Ranking([]types.Phase1RankedFile{
			{Path: "internal/agent/agent.go", Score: 99},
		})
		raw := strings.Join([]string{
			"internal/tool/builtin.go:855: comment mentions RegisterDefaultSubAgents and SubAgentRegistry",
			"internal/agent/agent.go:1649: schemas = append(schemas, proposeSubAgents)",
			"internal/agent/subagent_runtime.go:82: type SubAgentRuntime struct {}",
			"internal/agent/sub_explorer.go:31: func (s *SubExplorer) Name() string { return \"explorer\" }",
			"internal/agent/subagent.go:64: registry.Register(NewSubExplorer(deps))",
			"internal/orchestrator/orchestrator.go:6065: extractSubAgentProposal(output, agentName)",
			"docs/subagent.md:1: RegisterDefaultSubAgents notes",
		}, "\n")

		production, auxiliary, other, stats := partitionGrepOutputByRelevance(ctx, grepToolParams{Pattern: "RegisterDefaultSubAgents"}, raw)
		if len(other) != 0 {
			t.Fatalf("did not expect passthrough output: %#v", other)
		}
		if len(auxiliary) != 1 || !strings.Contains(auxiliary[0], "docs/subagent.md") {
			t.Fatalf("expected docs match to stay auxiliary, got %#v", auxiliary)
		}
		wantOrder := []string{
			"internal/agent/subagent.go",
			"internal/agent/sub_explorer.go",
			"internal/agent/subagent_runtime.go",
			"internal/agent/agent.go",
			"internal/tool/builtin.go",
			"internal/orchestrator/orchestrator.go",
		}
		if len(production) != len(wantOrder) {
			t.Fatalf("production len=%d want %d: %#v", len(production), len(wantOrder), production)
		}
		for i, want := range wantOrder {
			if !strings.Contains(production[i], want) {
				t.Fatalf("production[%d] = %q, want path %q; full=%#v", i, production[i], want, production)
			}
		}
		for _, want := range []string{"required=1", "read_or_evidence=2", "phase1_ranked=1", "production_rest=2", "auxiliary_rest=1"} {
			if !strings.Contains(stats, want) {
				t.Fatalf("stats missing %q: %q", want, stats)
			}
		}
	})

	t.Run("broad result preserves backend order within equal relevance tier", func(t *testing.T) {
		ctx := newBusContext()
		raw := strings.Join([]string{
			"src/a.go:1: EqualTierNeedle",
			"src/b.go:1: EqualTierNeedle",
			"src/c.go:1: EqualTierNeedle",
		}, "\n")
		production, auxiliary, other, _ := partitionGrepOutputByRelevance(ctx, grepToolParams{Pattern: "EqualTierNeedle"}, raw)
		if len(auxiliary) != 0 || len(other) != 0 {
			t.Fatalf("unexpected non-production output aux=%#v other=%#v", auxiliary, other)
		}
		if got := strings.Join(production, "\n"); got != raw {
			t.Fatalf("equal-tier production order changed:\n got: %s\nwant: %s", got, raw)
		}
	})

	t.Run("narrow files_only result ranks required file before generic production", func(t *testing.T) {
		ctx := newBusContext()
		ctx.AnalysisIR = &types.AnalysisIR{
			EvidencePlan: types.EvidencePlan{
				RequiredFiles: []string{"internal/agent/subagent.go"},
			},
		}
		raw := strings.Join([]string{
			"internal/tool/defaults.go",
			"internal/agent/subagent.go",
			"internal/skill/defaults.go",
		}, "\n")
		got := annotateGrepOutputByRelevance(ctx, grepToolParams{FilesOnly: true}, raw)
		if strings.Contains(got, "[grep retrieval governor]") {
			t.Fatalf("narrow annotation must not add governor metadata:\n%s", got)
		}
		requiredIdx := strings.Index(got, "internal/agent/subagent.go")
		toolIdx := strings.Index(got, "internal/tool/defaults.go")
		if requiredIdx < 0 || toolIdx < 0 || requiredIdx > toolIdx {
			t.Fatalf("required file should rank before generic production:\n%s", got)
		}
		if !strings.Contains(got, "[grep production matches]") {
			t.Fatalf("reordered narrow output should use the existing production section header:\n%s", got)
		}
	})

	t.Run("narrow line result ranks read evidence before generic production", func(t *testing.T) {
		ctx := newBusContext()
		ctx.Mutable = types.NewMutableState("relation lookup")
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
			ReadFiles: []string{"internal/agent/subagent.go"},
		})
		raw := strings.Join([]string{
			"internal/tool/defaults.go:6:func RegisterDefaults(r *Registry) {",
			"internal/agent/subagent.go:63:func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {",
			"internal/skill/defaults.go:12:func RegisterDefaults(r *Registry) {",
			"internal/tool/defaults_test.go:8:func TestRegisterDefaults(t *testing.T) {",
		}, "\n")
		got := annotateGrepOutputByRelevance(ctx, grepToolParams{}, raw)
		if strings.Contains(got, "[grep retrieval governor]") {
			t.Fatalf("narrow annotation must not add governor metadata:\n%s", got)
		}
		readIdx := strings.Index(got, "internal/agent/subagent.go:63")
		genericIdx := strings.Index(got, "internal/tool/defaults.go:6")
		auxIdx := strings.Index(got, "[grep auxiliary matches - supporting context, not production proof]")
		if readIdx < 0 || genericIdx < 0 || readIdx > genericIdx {
			t.Fatalf("read/evidence file should rank before generic production:\n%s", got)
		}
		if auxIdx < 0 || auxIdx < strings.Index(got, "internal/skill/defaults.go:12") {
			t.Fatalf("auxiliary section should remain after production matches:\n%s", got)
		}
	})

	t.Run("legacy path-role wrapper stays removed from grep output path", func(t *testing.T) {
		srcBytes, err := os.ReadFile("builtin.go")
		if err != nil {
			t.Fatalf("read builtin.go: %v", err)
		}
		src := string(srcBytes)
		if strings.Contains(src, "func annotateGrepOutputByPathRole(") {
			t.Fatal("legacy annotateGrepOutputByPathRole wrapper should stay deleted; use annotateGrepOutputByRelevance")
		}
		start := strings.Index(src, "func finalizeGrepOutput(")
		if start < 0 {
			t.Fatal("finalizeGrepOutput not found")
		}
		endRel := strings.Index(src[start:], "\n}\n\nfunc compactBroadGrepOutput(")
		if endRel < 0 {
			t.Fatal("finalizeGrepOutput end marker not found")
		}
		body := src[start : start+endRel]
		if strings.Contains(body, "annotateGrepOutputByPathRole") {
			t.Fatalf("finalizeGrepOutput must not use legacy path-role wrapper:\n%s", body)
		}
		if !strings.Contains(body, "annotateGrepOutputByRelevance") {
			t.Fatalf("finalizeGrepOutput must call relevance-aware annotation:\n%s", body)
		}
	})

	// Noise exclusion tests: grep should skip directories that produce
	// noise without useful signal (.git, logs, memory, node_modules, etc.)

	t.Run("excludes .git directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "main.go")
		os.WriteFile(srcFile, []byte("SubAgent code\n"), 0o644)
		gitDir := filepath.Join(tmpDir, ".git")
		os.MkdirAll(gitDir, 0o755)
		os.WriteFile(filepath.Join(gitDir, "COMMIT_EDITMSG"), []byte("SubAgent commit\n"), 0o644)

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "SubAgent", Path: tmpDir, FilesOnly: true})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "main.go") {
			t.Fatalf("expected main.go in results, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "COMMIT_EDITMSG") {
			t.Fatalf(".git/ files should be excluded, got: %s", result.Summary)
		}
	})

	t.Run("excludes node_modules and vendor", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "app.js")
		os.WriteFile(srcFile, []byte("import foo\n"), 0o644)
		nmDir := filepath.Join(tmpDir, "node_modules", "pkg")
		os.MkdirAll(nmDir, 0o755)
		os.WriteFile(filepath.Join(nmDir, "index.js"), []byte("import foo\n"), 0o644)
		vendorDir := filepath.Join(tmpDir, "vendor", "lib")
		os.MkdirAll(vendorDir, 0o755)
		os.WriteFile(filepath.Join(vendorDir, "dep.go"), []byte("import foo\n"), 0o644)

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "import", Path: tmpDir, FilesOnly: true})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "app.js") {
			t.Fatalf("expected app.js in results, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "index.js") {
			t.Fatalf("node_modules/ files should be excluded, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "dep.go") {
			t.Fatalf("vendor/ files should be excluded, got: %s", result.Summary)
		}
	})

	t.Run("explicit excluded subtree path remains searchable", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo, "dist", "bundle"), 0o755); err != nil {
			t.Fatalf("mkdir dist: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repo, "dist", "bundle", "app.js"), []byte("UNIQUE_DIST_TOKEN\n"), 0o644); err != nil {
			t.Fatalf("write dist file: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "UNIQUE_DIST_TOKEN", Path: "dist", FilesOnly: true})
		result, err := tool.Execute(&types.BusContext{RepoRoot: repo}, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "app.js") {
			t.Fatalf("explicit dist/ target should still be searchable, got: %s", result.Summary)
		}
	})

	t.Run("root only noise dirs do not hide nested project dirs", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo, "logs"), 0o755); err != nil {
			t.Fatalf("mkdir logs: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(repo, "internal", "logs"), 0o755); err != nil {
			t.Fatalf("mkdir internal/logs: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repo, "logs", "root_noise.go"), []byte("NESTED_LOGS_TOKEN\n"), 0o644); err != nil {
			t.Fatalf("write root logs file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repo, "internal", "logs", "nested_logger.go"), []byte("NESTED_LOGS_TOKEN\n"), 0o644); err != nil {
			t.Fatalf("write nested logs file: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "NESTED_LOGS_TOKEN", Path: ".", FilesOnly: true})
		result, err := tool.Execute(&types.BusContext{RepoRoot: repo}, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "root_noise.go") {
			t.Fatalf("root logs/ should be filtered from broad grep, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "nested_logger.go") {
			t.Fatalf("nested internal/logs/ should remain searchable, got: %s", result.Summary)
		}
	})

	t.Run("skips binary files", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "code.go")
		os.WriteFile(srcFile, []byte("SubAgent code\n"), 0o644)
		// Binary file with null bytes
		binFile := filepath.Join(tmpDir, "output.bin")
		os.WriteFile(binFile, []byte("SubAgent\x00\x01\x02data\n"), 0o644)

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "SubAgent", Path: tmpDir, FilesOnly: true})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "code.go") {
			t.Fatalf("expected code.go in results, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "output.bin") {
			t.Fatalf("binary file should be excluded by -I, got: %s", result.Summary)
		}
	})

	t.Run("grep no match", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "empty_match.txt")
		if err := os.WriteFile(tmpFile, []byte("nothing here\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "zzzznotfound", Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success (no matches), got: %s", result.Summary)
		}
		// Summary is "[grep params: ...]\nno matches found" — the
		// params banner survives into Turn B so the extractor / finalizer
		// can see which pattern turned up nothing, not just "the grep
		// was empty".
		if !strings.Contains(result.Summary, "[grep params: pattern=zzzznotfound") {
			t.Fatalf("expected params banner, got %q", result.Summary)
		}
		if !strings.Contains(result.Summary, "no matches found") {
			t.Fatalf("expected 'no matches found' body, got %q", result.Summary)
		}
	})

	t.Run("context_lines shows surrounding code", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "ctx.go")
		// 7 lines: match is on line 4 ("target"), context=2 should show lines 2-6
		content := "line1\nline2\nline3\ntarget\nline5\nline6\nline7\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		ctx2 := 2
		params, _ := json.Marshal(grepToolParams{Pattern: "target", Path: tmpDir, ContextLines: &ctx2})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		// With -C 2, should see line3 (before) and line5 (after) in addition to target
		if !strings.Contains(result.Summary, "line3") {
			t.Errorf("expected context before match (line3), got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "target") {
			t.Errorf("expected match line (target), got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "line5") {
			t.Errorf("expected context after match (line5), got: %s", result.Summary)
		}
	})

	t.Run("context_lines ignored for files_only", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "ctx.go")
		if err := os.WriteFile(tmpFile, []byte("target here\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		ctx5 := 5
		params, _ := json.Marshal(grepToolParams{Pattern: "target", Path: tmpDir, ContextLines: &ctx5, FilesOnly: true})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		// files_only should still work; context_lines is silently ignored
		if !strings.Contains(result.Summary, "matching files") {
			t.Errorf("expected files_only output, got: %s", result.Summary)
		}
	})

	t.Run("context_lines clamped to 20", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "ctx.go")
		content := "target\n"
		if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		ctx999 := 999
		params, _ := json.Marshal(grepToolParams{Pattern: "target", Path: tmpDir, ContextLines: &ctx999})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should not error — just clamp silently
		if !result.Success {
			t.Fatalf("expected success with clamped context, got: %s", result.Summary)
		}
	})

	t.Run("file_type filters by language", func(t *testing.T) {
		tmpDir := t.TempDir()
		goFile := filepath.Join(tmpDir, "code.go")
		pyFile := filepath.Join(tmpDir, "code.py")
		if err := os.WriteFile(goFile, []byte("target in go\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(pyFile, []byte("target in python\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "target", Path: tmpDir, FileType: "go"})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "target in go") {
			t.Errorf("expected go file match, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "target in python") {
			t.Errorf("file_type=go should exclude .py file, got: %s", result.Summary)
		}
	})

	t.Run("file_type with files_only", func(t *testing.T) {
		tmpDir := t.TempDir()
		goFile := filepath.Join(tmpDir, "main.go")
		txtFile := filepath.Join(tmpDir, "notes.txt")
		if err := os.WriteFile(goFile, []byte("func main() {}\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(txtFile, []byte("func main() {}\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "func", Path: tmpDir, FileType: "go", FilesOnly: true})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "main.go") {
			t.Errorf("expected main.go in results, got: %s", result.Summary)
		}
		if strings.Contains(result.Summary, "notes.txt") {
			t.Errorf("file_type=go should exclude .txt, got: %s", result.Summary)
		}
	})

	t.Run("summary preserves [grep: ...] prefix for downstream parsers", func(t *testing.T) {
		// contract_check.go and explorer.go both key on strings.HasPrefix
		// (Summary, "[grep:") and strings.Contains(Summary, "matching
		// {files,lines}]") to classify grep results. Adding a params
		// banner must never break that contract — the count banner stays
		// line 1, the params banner goes on line 2.
		tmpDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpDir, "hit.go"), []byte("target\n"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &GrepTool{}
		params, _ := json.Marshal(grepToolParams{Pattern: "target", Path: tmpDir, FilesOnly: true})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.HasPrefix(result.Summary, "[grep:") {
			t.Errorf("expected '[grep:' prefix (contract for contract_check/explorer), got first 40 chars: %q",
				result.Summary[:min(40, len(result.Summary))])
		}
		if !strings.Contains(result.Summary, "matching files]") {
			t.Errorf("expected 'matching files]' in summary, got %q", result.Summary)
		}
		if !strings.Contains(result.Summary, "[grep params: pattern=target") {
			t.Errorf("expected params banner with pattern, got %q", result.Summary)
		}
		if !strings.Contains(result.Summary, "files_only=true") {
			t.Errorf("expected params banner to record files_only, got %q", result.Summary)
		}
	})
}

func TestFileTypeToGlobs(t *testing.T) {
	tests := []struct {
		input    string
		wantLen  int
		wantGlob string // at least one glob must match this
	}{
		{"go", 1, "*.go"},
		{"py", 1, "*.py"},
		{"python", 1, "*.py"},
		{"js", 3, "*.jsx"},
		{"ts", 2, "*.tsx"},
		{"java", 3, "*.java"},
		{"arkts", 1, "*.ets"},
		{"cangjie", 1, "*.cj"},
		{"yaml", 2, "*.yml"},
		{"config", 4, "*.ini"},
		{"unknown_lang", 1, "*.unknown_lang"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			globs := fileTypeToGlobs(tt.input)
			if len(globs) != tt.wantLen {
				t.Errorf("fileTypeToGlobs(%q) returned %d globs, want %d: %v", tt.input, len(globs), tt.wantLen, globs)
			}
			found := false
			for _, g := range globs {
				if g == tt.wantGlob {
					found = true
				}
			}
			if !found {
				t.Errorf("fileTypeToGlobs(%q) = %v, missing expected %q", tt.input, globs, tt.wantGlob)
			}
		})
	}
}

// TestRepoMap moved to internal/tool/repomap/ package (tree-sitter powered).

func TestGitDiff(t *testing.T) {
	t.Run("description teaches VCS diff lane", func(t *testing.T) {
		desc := (&GitDiff{}).Description()
		for _, want := range []string{"VCS diff evidence", "not current checkout file:line evidence", "do not mirror old/deleted diff lines through emit_evidence"} {
			if !strings.Contains(desc, want) {
				t.Fatalf("git_diff description missing %q:\n%s", want, desc)
			}
		}
	})

	t.Run("skip if not in git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		tool := &GitDiff{}
		params, _ := json.Marshal(gitDiffParams{Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			// If it succeeds, that is fine too (e.g., parent dir is a git repo).
			return
		}
		t.Skip("not in a git repo")
	})

	t.Run("supports stat and name-only modes", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "old\n")
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "file.txt"), []byte("old\nnew\n"), 0o644); err != nil {
			t.Fatalf("modify fixture file: %v", err)
		}
		tool := &GitDiff{}
		for _, tc := range []struct {
			name string
			in   gitDiffParams
			want string
		}{
			{"stat", gitDiffParams{Stat: true}, "file.txt"},
			{"name-only", gitDiffParams{NameOnly: true}, "file.txt"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				params, _ := json.Marshal(tc.in)
				result, err := tool.Execute(ctx, params)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success || !strings.Contains(result.Summary, tc.want) {
					t.Fatalf("git_diff %s failed or missed %q: success=%v summary=%q", tc.name, tc.want, result.Success, result.Summary)
				}
				if !strings.Contains(result.Summary, "evidence_origin=vcs_diff") {
					t.Fatalf("git_diff %s should tag VCS diff origin in banner, got %q", tc.name, result.Summary)
				}
			})
		}
		params, _ := json.Marshal(gitDiffParams{Stat: true, NameOnly: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success || !strings.Contains(result.Summary, "mutually exclusive") {
			t.Fatalf("stat+name_only should be rejected, got success=%v summary=%q", result.Success, result.Summary)
		}
	})

	t.Run("rejects path outside repo root", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		tool := &GitDiff{}
		params, _ := json.Marshal(gitDiffParams{Path: filepath.Dir(ctx.RepoRoot)})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success || !strings.Contains(result.Summary, "outside repository root") {
			t.Fatalf("outside git_diff path should be rejected, got success=%v summary=%q", result.Success, result.Summary)
		}
	})
}

func TestGitShow(t *testing.T) {
	ctx := gitWorktreeFixture(t, "old\n")
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "file.txt"), []byte("old\nnew\n"), 0o644); err != nil {
		t.Fatalf("modify fixture file: %v", err)
	}
	cmd, cancel := NewGitCommand(nil, "add", "file.txt")
	defer cancel()
	cmd.Dir = ctx.RepoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd, cancel = NewGitCommand(nil, "commit", "-q", "-m", "show target")
	defer cancel()
	cmd.Dir = ctx.RepoRoot
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	tool := &GitShow{}
	for _, tc := range []struct {
		name           string
		in             gitShowParams
		want           string
		wantDiffOrigin bool
	}{
		{"no-patch", gitShowParams{Ref: "HEAD", NoPatch: true, Format: "oneline"}, "show target", false},
		{"name-only", gitShowParams{Ref: "HEAD", NameOnly: true}, "file.txt", true},
		{"stat", gitShowParams{Ref: "HEAD", Stat: true}, "exact_changed_paths", true},
		{"default patch", gitShowParams{Ref: "HEAD"}, "+new", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(tc.in)
			result, err := tool.Execute(ctx, params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Success || !strings.Contains(result.Summary, tc.want) {
				t.Fatalf("git_show %s failed or missed %q: success=%v summary=%q", tc.name, tc.want, result.Success, result.Summary)
			}
			if !strings.Contains(result.Summary, "evidence_origin=vcs_metadata") {
				t.Fatalf("git_show %s should tag VCS metadata origin in banner, got %q", tc.name, result.Summary)
			}
			hasDiffOrigin := strings.Contains(result.Summary, "diff_origin=vcs_diff")
			if hasDiffOrigin != tc.wantDiffOrigin {
				t.Fatalf("git_show %s diff origin = %v, want %v; summary=%q", tc.name, hasDiffOrigin, tc.wantDiffOrigin, result.Summary)
			}
			if tc.name == "stat" && !strings.Contains(result.Summary, `exact_path commit=HEAD path="file.txt"`) {
				t.Fatalf("git_show stat should expose exact changed path, got %q", result.Summary)
			}
		})
	}
	params, _ := json.Marshal(gitShowParams{Ref: "HEAD", NoPatch: true, Stat: true})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success || !strings.Contains(result.Summary, "mutually exclusive") {
		t.Fatalf("no_patch+stat should be rejected, got success=%v summary=%q", result.Success, result.Summary)
	}
}

func TestGitLog(t *testing.T) {
	t.Run("description teaches merge history lane", func(t *testing.T) {
		desc := (&GitLog{}).Description()
		for _, want := range []string{"merges_only=true", "first_parent=true", "最近一次合入", "Git metadata is VCS provenance"} {
			if !strings.Contains(desc, want) {
				t.Fatalf("git_log description missing %q:\n%s", want, desc)
			}
		}
		if strings.Contains(desc, "--no-stat") {
			t.Fatalf("git_log description must not suggest unsupported git show flags:\n%s", desc)
		}
	})

	t.Run("skip if not in git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		tool := &GitLog{}
		params, _ := json.Marshal(gitLogParams{Path: tmpDir, Count: 5})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success {
			return
		}
		t.Skip("not in a git repo")
	})

	t.Run("rejects path outside repo root", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		tool := &GitLog{}
		params, _ := json.Marshal(gitLogParams{Path: filepath.Dir(ctx.RepoRoot), Count: 1})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success || !strings.Contains(result.Summary, "outside repository root") {
			t.Fatalf("outside git_log path should be rejected, got success=%v summary=%q", result.Success, result.Summary)
		}
	})

	t.Run("tags VCS metadata origin", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		tool := &GitLog{}
		params, _ := json.Marshal(gitLogParams{Count: 1, Format: "oneline"})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("git_log failed: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "evidence_origin=vcs_metadata") {
			t.Fatalf("git_log should tag VCS metadata origin in banner, got %q", result.Summary)
		}
	})

	t.Run("supports stat name-only and pathspec for recent summaries", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		run := func(args ...string) {
			t.Helper()
			cmd, cancel := NewGitCommand(nil, args...)
			defer cancel()
			cmd.Dir = ctx.RepoRoot
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
				"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "sub"), 0o755); err != nil {
			t.Fatalf("mkdir sub: %v", err)
		}
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "sub", "feature.txt"), []byte("feature\n"), 0o644); err != nil {
			t.Fatalf("write sub file: %v", err)
		}
		run("add", "sub/feature.txt")
		run("commit", "-q", "-m", "feature change")

		tool := &GitLog{}
		for _, tc := range []struct {
			name string
			in   gitLogParams
			want string
		}{
			{"stat", gitLogParams{Count: 2, Format: "medium", Stat: true, Pathspec: "sub/"}, "sub/feature.txt"},
			{"name-only", gitLogParams{Count: 2, Format: "oneline", NameOnly: true, Pathspec: "sub/"}, "sub/feature.txt"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				params, _ := json.Marshal(tc.in)
				result, err := tool.Execute(ctx, params)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success || !strings.Contains(result.Summary, tc.want) {
					t.Fatalf("git_log %s failed or missed %q: success=%v summary=%q", tc.name, tc.want, result.Success, result.Summary)
				}
				if !strings.Contains(result.Summary, "pathspec=sub/") ||
					!strings.Contains(result.Summary, "evidence_origin=vcs_metadata") {
					t.Fatalf("git_log %s should tag pathspec and VCS metadata, got %q", tc.name, result.Summary)
				}
			})
		}

		params, _ := json.Marshal(gitLogParams{Stat: true, NameOnly: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success || !strings.Contains(result.Summary, "mutually exclusive") {
			t.Fatalf("stat+name_only should be rejected, got success=%v summary=%q", result.Success, result.Summary)
		}
	})

	t.Run("stat includes exact changed paths before abbreviated stat display", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		run := func(args ...string) {
			t.Helper()
			cmd, cancel := NewGitCommand(nil, args...)
			defer cancel()
			cmd.Dir = ctx.RepoRoot
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
				"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		exactPath := "docs/design/answer_surface_safety_batch_20260523.md"
		if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, filepath.Dir(exactPath)), 0o755); err != nil {
			t.Fatalf("mkdir nested path: %v", err)
		}
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, exactPath), []byte(strings.Repeat("line\n", 20)), 0o644); err != nil {
			t.Fatalf("write nested path: %v", err)
		}
		run("add", exactPath)
		run("commit", "-q", "-m", "add exact path doc")

		tool := &GitLog{}
		params, _ := json.Marshal(gitLogParams{Count: 1, Format: "medium", Stat: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("git_log stat failed: %s", result.Summary)
		}
		for _, want := range []string{
			"exact_changed_paths: copy these exact paths",
			`path="docs/design/answer_surface_safety_batch_20260523.md"`,
			"do not expand abbreviated --stat paths",
		} {
			if !strings.Contains(result.Summary, want) {
				t.Fatalf("git_log stat exact path output missing %q:\n%s", want, result.Summary)
			}
		}
	})

	t.Run("supports merge and ref filters without shell fragments", func(t *testing.T) {
		ctx := gitWorktreeFixture(t, "seed\n")
		run := func(args ...string) string {
			t.Helper()
			cmd, cancel := NewGitCommand(nil, args...)
			defer cancel()
			cmd.Dir = ctx.RepoRoot
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
				"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
			return strings.TrimSpace(string(out))
		}
		writeAndCommit := func(name, body, subject string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(ctx.RepoRoot, name), []byte(body), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
			run("add", name)
			run("commit", "-q", "-m", subject)
		}

		baseBranch := run("branch", "--show-current")
		if baseBranch == "" {
			baseBranch = "master"
		}
		writeAndCommit("main.txt", "main\n", "main feature")
		run("checkout", "-q", "-b", "topic", "HEAD~1")
		writeAndCommit("topic.txt", "topic\n", "topic feature")
		run("checkout", "-q", baseBranch)
		run("merge", "--no-ff", "-q", "-m", "merge topic feature", "topic")

		tool := &GitLog{}
		for _, tc := range []struct {
			name    string
			in      gitLogParams
			want    string
			mustNot string
		}{
			{
				name: "latest merge",
				in: gitLogParams{
					Count:       1,
					Format:      "oneline",
					MergesOnly:  true,
					FirstParent: true,
				},
				want: "merge topic feature",
			},
			{
				name: "exclude merges",
				in: gitLogParams{
					Count:    3,
					Format:   "oneline",
					NoMerges: true,
				},
				want:    "main feature",
				mustNot: "merge topic feature",
			},
			{
				name: "ref starts history",
				in: gitLogParams{
					Count:  1,
					Format: "oneline",
					Ref:    "HEAD~1",
				},
				want: "main feature",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				params, _ := json.Marshal(tc.in)
				result, err := tool.Execute(ctx, params)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success || !strings.Contains(result.Summary, tc.want) {
					t.Fatalf("git_log %s failed or missed %q: success=%v summary=%q", tc.name, tc.want, result.Success, result.Summary)
				}
				for _, want := range []string{"ref=", "merges_only=", "first_parent=", "evidence_origin=vcs_metadata"} {
					if !strings.Contains(result.Summary, want) {
						t.Fatalf("git_log %s banner missing %q: %q", tc.name, want, result.Summary)
					}
				}
				if tc.mustNot != "" && strings.Contains(result.Summary, tc.mustNot) {
					t.Fatalf("git_log %s unexpectedly included %q:\n%s", tc.name, tc.mustNot, result.Summary)
				}
			})
		}

		params, _ := json.Marshal(gitLogParams{MergesOnly: true, NoMerges: true})
		result, err := tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success || !strings.Contains(result.Summary, "merges_only and no_merges are mutually exclusive") {
			t.Fatalf("merges_only+no_merges should be rejected, got success=%v summary=%q", result.Success, result.Summary)
		}

		params, _ = json.Marshal(gitLogParams{Ref: "--all"})
		result, err = tool.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Success || !strings.Contains(result.Summary, "ref must not start") {
			t.Fatalf("unsafe ref should be rejected, got success=%v summary=%q", result.Success, result.Summary)
		}
	})
}

func TestGitHistorySearchCountsBoundedDiffMatches(t *testing.T) {
	desc := (&GitHistorySearch{}).Description()
	for _, want := range []string{"VCS metadata/diff provenance", "not current-source file:line evidence", "aggregate_facts/reason"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("git_history_search description missing %q:\n%s", want, desc)
		}
	}

	ctx := gitWorktreeFixture(t, "seed\n")
	run := func(args ...string) {
		t.Helper()
		cmd, cancel := NewGitCommand(nil, args...)
		defer cancel()
		cmd.Dir = ctx.RepoRoot
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeAndCommit := func(body, subject string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "file.txt"), []byte(body), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		run("add", "file.txt")
		run("commit", "-q", "-m", subject)
	}
	writeAndCommit("seed\nno match here\n", "other change")
	writeAndCommit("seed\nno match here\nrunTaskGraph\n", "touch runTaskGraph")

	tool := &GitHistorySearch{}
	params, _ := json.Marshal(gitHistorySearchParams{
		WindowPath:  "file.txt",
		WindowCount: 2,
		DiffPath:    "file.txt",
		Contains:    "runTaskGraph",
	})
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("git_history_search failed: %s", res.Summary)
	}
	for _, want := range []string{"evidence_origin=vcs_metadata", "window_size=2", "answer_count=1", "touch runTaskGraph", "unmatched=1"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestGitHistorySearchOldestOrderFindsEarliestMatch(t *testing.T) {
	ctx := gitWorktreeFixture(t, "seed\n")
	run := func(args ...string) {
		t.Helper()
		cmd, cancel := NewGitCommand(nil, args...)
		defer cancel()
		cmd.Dir = ctx.RepoRoot
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeAndCommit := func(body, subject string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "file.txt"), []byte(body), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		run("add", "file.txt")
		run("commit", "-q", "-m", subject)
	}
	writeAndCommit("seed\nEvidenceClosure introduced\n", "first EvidenceClosure")
	writeAndCommit("seed\nEvidenceClosure introduced\nlater mention\n", "later EvidenceClosure")

	tool := &GitHistorySearch{}
	params, _ := json.Marshal(gitHistorySearchParams{
		WindowPath:  "file.txt",
		WindowCount: 1,
		Order:       "oldest",
		DiffPath:    "file.txt",
		Contains:    "EvidenceClosure",
	})
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("git_history_search failed: %s", res.Summary)
	}
	for _, want := range []string{"order=oldest", "window_size=2", "answer_count=1", "first EvidenceClosure"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "later EvidenceClosure") {
		t.Fatalf("oldest order with window_count=1 should not return the later commit:\n%s", res.Summary)
	}
}

func TestGitHistorySearchNormalizesAbsolutePathspecs(t *testing.T) {
	ctx := gitWorktreeFixture(t, "seed\n")
	path := filepath.Join(ctx.RepoRoot, "file.txt")
	if got := normalizeGitHistoryPathspec(path, ctx.RepoRoot); got != "file.txt" {
		t.Fatalf("normalizeGitHistoryPathspec(abs) = %q, want file.txt", got)
	}
	if got := normalizeGitHistoryPathspec("./dir/../file.txt", ctx.RepoRoot); got != "file.txt" {
		t.Fatalf("normalizeGitHistoryPathspec(./) = %q, want file.txt", got)
	}
}

func TestGitHistorySearchRejectsOversizedWindow(t *testing.T) {
	ctx := gitWorktreeFixture(t, "seed\n")
	tool := &GitHistorySearch{}
	params, _ := json.Marshal(gitHistorySearchParams{
		WindowPath:  "file.txt",
		WindowCount: gitHistorySearchMaxWindowCount + 1,
		Contains:    "runTaskGraph",
	})
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "window_count max") {
		t.Fatalf("oversized window should fail loudly without silent truncation, got success=%v summary=%q", res.Success, res.Summary)
	}
}

func TestGitHistorySearchRejectsAbsolutePathOutsideRepo(t *testing.T) {
	ctx := gitWorktreeFixture(t, "seed\n")
	tool := &GitHistorySearch{}
	params, _ := json.Marshal(gitHistorySearchParams{
		WindowPath: "/tmp/outside-codrax-history.txt",
		Contains:   "runTaskGraph",
	})
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "repo-relative") {
		t.Fatalf("outside absolute path should fail loudly, got success=%v summary=%q", res.Success, res.Summary)
	}
}

func TestGitHistorySearchRejectsRepoPathOutsideRepo(t *testing.T) {
	ctx := gitWorktreeFixture(t, "seed\n")
	tool := &GitHistorySearch{}
	params, _ := json.Marshal(gitHistorySearchParams{
		RepoPath:   filepath.Dir(ctx.RepoRoot),
		WindowPath: ".",
		Contains:   "runTaskGraph",
	})
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "outside repository root") {
		t.Fatalf("outside repo_path should fail loudly, got success=%v summary=%q", res.Success, res.Summary)
	}
}

// TestResolveToolPath_RootsRelativeAtRepoRoot pins the boundary
// behavior that fixes the Q2 glamour-vs-codrax bug: when the LLM passes
// "." or a relative path, tools must operate against ctx.RepoRoot, not
// the codrax process CWD. A regression here means foreign-repo
// analysis silently scans codrax itself.
func TestResolveToolPath_RootsRelativeAtRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	ctx := &types.BusContext{RepoRoot: repoRoot}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", repoRoot},
		{"dot", ".", repoRoot},
		{"relative_file", "internal/foo.go", filepath.Join(repoRoot, "internal/foo.go")},
		{"relative_dir", "subdir", filepath.Join(repoRoot, "subdir")},
		{"absolute_passthrough", "/etc/hostname", "/etc/hostname"},
		{"windows_drive_passthrough", `C:\temp\foo.txt`, `C:\temp\foo.txt`},
		{"unc_passthrough", `\\server\share\foo.txt`, `\\server\share\foo.txt`},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases,
			struct {
				name string
				in   string
				want string
			}{"git_bash_drive_path", "/c/Users/tester/repo/internal/foo.go", `C:\Users\tester\repo\internal\foo.go`},
			struct {
				name string
				in   string
				want string
			}{"wsl_mnt_drive_path", "/mnt/c/Users/tester/repo/internal/foo.go", `C:\Users\tester\repo\internal\foo.go`},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveToolPath(ctx, tc.in)
			if got != tc.want {
				t.Fatalf("resolveToolPath(%q): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// Zero-value RepoRoot must pass the path through unchanged so
	// existing unit tests that build a bare BusContext keep working.
	t.Run("empty_RepoRoot_unchanged", func(t *testing.T) {
		got := resolveToolPath(&types.BusContext{}, "internal/foo.go")
		if got != "internal/foo.go" {
			t.Fatalf("with empty RepoRoot expected pass-through, got %q", got)
		}
	})
}

func TestResolveToolPath_WriteModeRemapsMainRepoAbsolutePathToActiveWorktree(t *testing.T) {
	parent := t.TempDir()
	mainRoot := filepath.Join(parent, "repo")
	worktreeRoot := filepath.Join(mainRoot, ".codrax", "worktrees", "trace-1")
	if err := os.MkdirAll(filepath.Join(mainRoot, "pkg"), 0o755); err != nil {
		t.Fatalf("setup main root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(worktreeRoot, "pkg"), 0o755); err != nil {
		t.Fatalf("setup worktree root: %v", err)
	}
	ctx := &types.BusContext{
		Mode:         types.ModeApply,
		RepoRoot:     worktreeRoot,
		MainRepoRoot: mainRoot,
		WorktreePath: worktreeRoot,
		WorkDir:      filepath.Join(mainRoot, ".codrax"),
	}

	mainAbs := filepath.Join(mainRoot, "pkg", "file.py")
	want := filepath.Join(worktreeRoot, "pkg", "file.py")
	if got := resolveToolPath(ctx, mainAbs); got != want {
		t.Fatalf("resolveToolPath main absolute: got %q, want %q", got, want)
	}

	dirGot, errText := resolveRepoScopedToolDir(ctx, mainAbs)
	if errText != "" {
		t.Fatalf("resolveRepoScopedToolDir rejected remappable main absolute path: %s", errText)
	}
	if dirGot != want {
		t.Fatalf("resolveRepoScopedToolDir main absolute: got %q, want %q", dirGot, want)
	}

	alreadyWorktree := filepath.Join(worktreeRoot, "pkg", "file.py")
	if got := resolveToolPath(ctx, alreadyWorktree); got != alreadyWorktree {
		t.Fatalf("worktree absolute path should not be remapped again: got %q, want %q", got, alreadyWorktree)
	}

	mainActiveCtx := *ctx
	mainActiveCtx.RepoRoot = mainRoot
	if got := resolveToolPath(&mainActiveCtx, mainAbs); got != mainAbs {
		t.Fatalf("inactive worktree should preserve main absolute path: got %q, want %q", got, mainAbs)
	}

	outside := filepath.Join(parent, "outside", "file.py")
	if got := resolveToolPath(ctx, outside); got != outside {
		t.Fatalf("outside absolute path should stay unchanged: got %q, want %q", got, outside)
	}

	runtimeArtifact := filepath.Join(ctx.WorkDir, "logs", "codrax.log")
	if got := resolveToolPath(ctx, runtimeArtifact); got != runtimeArtifact {
		t.Fatalf("runtime artifact absolute path should stay unchanged: got %q, want %q", got, runtimeArtifact)
	}

	planArtifact := filepath.Join(mainRoot, ".codrax", "plans", "plan-1.attempt-1.diff")
	if got := resolveToolPath(ctx, planArtifact); got != planArtifact {
		t.Fatalf("durable plan artifact absolute path should stay unchanged: got %q, want %q", got, planArtifact)
	}
}

func TestResolveToolPath_StripsActiveRepoLabelPrefixWhenUnambiguous(t *testing.T) {
	parent := t.TempDir()
	repoRoot := filepath.Join(parent, "codrax-small")
	if err := os.MkdirAll(filepath.Join(repoRoot, "internal"), 0o755); err != nil {
		t.Fatalf("setup repo: %v", err)
	}
	target := filepath.Join(repoRoot, "internal", "foo.go")
	if err := os.WriteFile(target, []byte("package internal\n"), 0o644); err != nil {
		t.Fatalf("setup file: %v", err)
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	got := resolveToolPath(ctx, "codrax-small/internal/foo.go")
	if got != target {
		t.Fatalf("repo-label path should rewrite to in-root target, got %q want %q", got, target)
	}

	dir, msg := resolveRepoScopedToolDir(ctx, "codrax-small/internal")
	if msg != "" {
		t.Fatalf("repo-scoped rewrite should not reject: %s", msg)
	}
	if want := filepath.Join(repoRoot, "internal"); dir != want {
		t.Fatalf("repo-scoped dir = %q, want %q", dir, want)
	}
}

func TestResolveToolPath_DoesNotStripRealTopLevelRepoLabelDirectory(t *testing.T) {
	parent := t.TempDir()
	repoRoot := filepath.Join(parent, "codrax-small")
	realNested := filepath.Join(repoRoot, "codrax-small", "internal")
	if err := os.MkdirAll(realNested, 0o755); err != nil {
		t.Fatalf("setup nested dir: %v", err)
	}
	nestedFile := filepath.Join(realNested, "foo.go")
	if err := os.WriteFile(nestedFile, []byte("package nested\n"), 0o644); err != nil {
		t.Fatalf("setup nested file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "internal"), 0o755); err != nil {
		t.Fatalf("setup internal dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "internal", "foo.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatalf("setup internal file: %v", err)
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	got := resolveToolPath(ctx, "codrax-small/internal/foo.go")
	if got != nestedFile {
		t.Fatalf("real top-level repo-label directory must win over rewrite, got %q want %q", got, nestedFile)
	}
}

// TestReadFile_ResolvesAgainstRepoRoot confirms that a foreign-repo
// scenario (codrax CWD ≠ --repo) opens the right file. Without the
// fix, read_file(path="hello.txt") would try to open hello.txt in the
// codrax process CWD — invariably failing or, worse, silently reading
// a same-named file from codrax itself.
func TestReadFile_ResolvesAgainstRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	target := "hello.txt"
	want := "from the right repo\n"
	if err := os.WriteFile(filepath.Join(repoRoot, target), []byte(want), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	tool := &ReadFile{}
	params, _ := json.Marshal(readFileParams{Path: target})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, want) {
		t.Fatalf("expected file content in summary, got: %s", result.Summary)
	}
	// Banner echoes the LLM-supplied (relative) path so downstream
	// extractFileCoverage / canonicaliser keep operating on
	// repo-relative keys.
	if !strings.Contains(result.Summary, "["+target+":") {
		t.Fatalf("banner should echo LLM-supplied relative path %q, got: %s", target, result.Summary)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("read_file should publish one typed observation, got %+v", result.Observations)
	}
	obs := result.Observations[0]
	if obs.Origin != types.AnswerEvidenceOriginCurrentSource ||
		obs.SourceRef.Kind != types.ObservationSourceCurrentSource ||
		obs.SourceRef.Path != target ||
		obs.Producer != "read_file" ||
		obs.Span.LineStart != 1 ||
		obs.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("unexpected read_file observation: %+v", obs)
	}
}

// TestListFiles_ResolvesAgainstRepoRoot confirms the same boundary
// for list_files. The output paths preserve the LLM's relative form
// so the LLM can feed them straight back into read_file / grep.
func TestListFiles_ResolvesAgainstRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(repoRoot, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	tool := &ListFiles{}
	params, _ := json.Marshal(listFilesParams{Path: "."})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	for _, want := range []string{"a.go", "b.go"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("expected listing to contain %q, got: %s", want, result.Summary)
		}
	}
	// Output should NOT carry the absolute repoRoot prefix — the LLM
	// expects repo-relative paths it can immediately reuse.
	if strings.Contains(result.Summary, repoRoot) {
		t.Fatalf("listing leaked absolute repoRoot prefix, got: %s", result.Summary)
	}
}

// TestGrep_ResolvesAgainstRepoRoot exercises the grep tool through
// the same boundary. The matched line must come from the file in
// ctx.RepoRoot, not from any same-named file in the process CWD.
func TestGrep_ResolvesAgainstRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "needle.txt"), []byte("UNIQUE_TOKEN_X9Q\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	tool := &GrepTool{}
	params, _ := json.Marshal(grepToolParams{Pattern: "UNIQUE_TOKEN_X9Q", Path: "."})
	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "needle.txt") {
		t.Fatalf("grep should find needle.txt in RepoRoot, got: %s", result.Summary)
	}
}

func TestConfiguredSearchExcludeRoots_SurfaceTypedRefinementForBroadTools(t *testing.T) {
	oldRoots := SearchRuntimeArtifactRoots()
	SetSearchRuntimeArtifactRoots([]string{"out"})
	t.Cleanup(func() { SetSearchRuntimeArtifactRoots(oldRoots) })

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "out", "generated"), 0o755); err != nil {
		t.Fatalf("seed generated dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "out", "generated", "app.js"), []byte("CONFIGURED_ONLY_TOKEN\n"), 0o644); err != nil {
		t.Fatalf("seed generated file: %v", err)
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	grepTool := &GrepTool{}
	broadGrep, err := grepTool.Execute(ctx, json.RawMessage(`{"pattern":"CONFIGURED_ONLY_TOKEN","path":"."}`))
	if err != nil {
		t.Fatalf("broad grep: %v", err)
	}
	if !broadGrep.Success {
		t.Fatalf("broad grep failed: %s", broadGrep.Summary)
	}
	if strings.Contains(broadGrep.Summary, "out/generated/app.js") {
		t.Fatalf("broad grep should honor configured exclusion:\n%s", broadGrep.Summary)
	}
	if broadGrep.Refinement == nil ||
		broadGrep.Refinement.UniverseExcludedReason != "configured_search_exclude_roots" ||
		len(broadGrep.Refinement.ExcludedRoots) != 1 ||
		broadGrep.Refinement.ExcludedRoots[0] != "out" {
		t.Fatalf("broad grep should expose typed excluded-universe refinement: %+v", broadGrep.Refinement)
	}

	explicitGrep, err := grepTool.Execute(ctx, json.RawMessage(`{"pattern":"CONFIGURED_ONLY_TOKEN","path":"out"}`))
	if err != nil {
		t.Fatalf("explicit grep: %v", err)
	}
	if !explicitGrep.Success || !strings.Contains(explicitGrep.Summary, "out/generated/app.js") {
		t.Fatalf("explicit target under configured root should remain searchable:\n%s", explicitGrep.Summary)
	}
	if explicitGrep.Refinement != nil && explicitGrep.Refinement.UniverseExcludedReason != "" {
		t.Fatalf("explicit target should not expose broad-root excluded-universe refinement: %+v", explicitGrep.Refinement)
	}

	listTool := &ListFiles{}
	broadList, err := listTool.Execute(ctx, json.RawMessage(`{"path":".","recursive":true}`))
	if err != nil {
		t.Fatalf("broad list_files: %v", err)
	}
	if broadList.Refinement == nil ||
		broadList.Refinement.UniverseExcludedReason != "configured_search_exclude_roots" ||
		len(broadList.Refinement.ExcludedRoots) != 1 ||
		broadList.Refinement.ExcludedRoots[0] != "out" {
		t.Fatalf("broad list_files should expose typed excluded-universe refinement: %+v", broadList.Refinement)
	}

	explicitList, err := listTool.Execute(ctx, json.RawMessage(`{"path":"out","recursive":true}`))
	if err != nil {
		t.Fatalf("explicit list_files: %v", err)
	}
	if !explicitList.Success || !strings.Contains(explicitList.Summary, "out/generated/app.js") {
		t.Fatalf("explicit list_files should show configured root contents:\n%s", explicitList.Summary)
	}
	if explicitList.Refinement != nil && explicitList.Refinement.UniverseExcludedReason != "" {
		t.Fatalf("explicit list_files should not expose broad-root excluded-universe refinement: %+v", explicitList.Refinement)
	}
}

func TestListFilesBroadResult_SurfaceTypedRefinement(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "src", "pkg"), 0o755); err != nil {
		t.Fatalf("seed src dir: %v", err)
	}
	for i := 0; i < grepGovernorFileEntryThreshold+5; i++ {
		name := filepath.Join(repoRoot, "src", "pkg", fmt.Sprintf("file_%03d.go", i))
		if err := os.WriteFile(name, []byte("package pkg\n"), 0o644); err != nil {
			t.Fatalf("seed source %d: %v", i, err)
		}
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	result, err := (&ListFiles{}).Execute(ctx, json.RawMessage(`{"path":".","recursive":true}`))
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	if !result.Success {
		t.Fatalf("list_files failed: %s", result.Summary)
	}
	if result.Refinement == nil {
		t.Fatalf("broad list_files should expose typed refinement")
	}
	if result.Refinement.ReasonCode != "list_files_result_truncated" || !result.Refinement.ResultTruncated {
		t.Fatalf("unexpected refinement: %+v", result.Refinement)
	}
	if result.Refinement.PreferredNextTool != "list_files" {
		t.Fatalf("preferred tool = %q", result.Refinement.PreferredNextTool)
	}
	if got := result.Refinement.PreferredParams["path"]; got != "src/pkg" {
		t.Fatalf("preferred path = %q; refinement=%+v", got, result.Refinement)
	}
	if !sameStringSliceForTest(result.Refinement.RequiredFields, []string{"include", "file_type"}) {
		t.Fatalf("required fields = %v", result.Refinement.RequiredFields)
	}
}

func TestListFilesBroadResult_PreservesTypedFiltersInRefinement(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "app", "models"), 0o755); err != nil {
		t.Fatalf("seed app dir: %v", err)
	}
	for i := 0; i < grepGovernorFileEntryThreshold+5; i++ {
		name := filepath.Join(repoRoot, "app", "models", fmt.Sprintf("model_%03d.py", i))
		if err := os.WriteFile(name, []byte("class M: pass\n"), 0o644); err != nil {
			t.Fatalf("seed source %d: %v", i, err)
		}
	}

	ctx := &types.BusContext{RepoRoot: repoRoot}
	result, err := (&ListFiles{}).Execute(ctx, json.RawMessage(`{"path":".","recursive":true,"file_type":"py"}`))
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	if !result.Success {
		t.Fatalf("list_files failed: %s", result.Summary)
	}
	if result.Refinement == nil {
		t.Fatalf("broad filtered list_files should expose typed refinement")
	}
	if result.Refinement.ReasonCode != "list_files_result_truncated" {
		t.Fatalf("reason = %q; refinement=%+v", result.Refinement.ReasonCode, result.Refinement)
	}
	if got := result.Refinement.PreferredParams["file_type"]; got != "py" {
		t.Fatalf("file_type = %q; refinement=%+v", got, result.Refinement)
	}
	if got := result.Refinement.PreferredParams["path"]; got != "app/models" {
		t.Fatalf("preferred path = %q; refinement=%+v", got, result.Refinement)
	}
	if len(result.Refinement.RequiredFields) != 0 {
		t.Fatalf("filtered broad result should not require include/file_type again: %+v", result.Refinement)
	}
}

func sameStringSliceForTest(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestListFiles_NoiseDirsFilteredInBothModes pins the contract that
// list_files skips the central tool.ExcludeDirsAnyLevelSet noise
// dirs (.git, .codrax, node_modules, vendor, target, dist, build,
// __pycache__, .venv, .idea, .vscode, .next, .nuxt, .turbo,
// .pnpm-store, .pytest_cache, .mypy_cache, .gradle, .cargo, .tox,
// .hg, .svn, venv) in BOTH recursive and non-recursive paths.
//
// Bug provenance: eval Batch I+J surfaced an asymmetry — the
// non-recursive list_files branch had ZERO filter, returning .git /
// .codrax / node_modules verbatim. The recursive branch had an
// inline shorter list missing target / dist / build / __pycache__
// / .codrax. Centralised on ExcludeDirsAnyLevelSet so both branches
// agree.
func TestListFiles_NoiseDirsFilteredInBothModes(t *testing.T) {
	repo := t.TempDir()
	// Seed a representative noisy tree.
	noiseDirs := []string{
		".git", ".codrax", "node_modules", "vendor", "target",
		"dist", "build", "__pycache__", ".venv", "venv",
		".idea", ".vscode", ".next", ".pnpm-store", ".pytest_cache",
	}
	for _, d := range noiseDirs {
		if err := os.MkdirAll(filepath.Join(repo, d, "deep"), 0o755); err != nil {
			t.Fatalf("seed %s: %v", d, err)
		}
		if err := os.WriteFile(filepath.Join(repo, d, "deep", "noise.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("seed file: %v", err)
		}
	}
	// Seed legitimate source files / dirs.
	legitDirs := []string{"src", "internal", "cmd"}
	for _, d := range legitDirs {
		if err := os.MkdirAll(filepath.Join(repo, d), 0o755); err != nil {
			t.Fatalf("seed %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}

	bus := &types.BusContext{RepoRoot: repo}
	tool := &ListFiles{}

	t.Run("non-recursive filters noise + keeps legit", func(t *testing.T) {
		res, _ := tool.Execute(bus, json.RawMessage(`{"path":".","recursive":false}`))
		out := res.Summary
		for _, n := range noiseDirs {
			if strings.Contains(out, n) {
				t.Errorf("non-recursive list_files leaked noise dir %q in output:\n%s", n, out)
			}
		}
		for _, l := range legitDirs {
			if !strings.Contains(out, l) {
				t.Errorf("non-recursive list_files dropped legit dir %q from output:\n%s", l, out)
			}
		}
		if !strings.Contains(out, "main.go") {
			t.Errorf("non-recursive list_files must include legit file main.go; got:\n%s", out)
		}
	})

	t.Run("recursive filters noise + keeps legit", func(t *testing.T) {
		res, _ := tool.Execute(bus, json.RawMessage(`{"path":".","recursive":true}`))
		out := res.Summary
		for _, n := range noiseDirs {
			// Allow root-level dir name to appear in the listing as
			// the dir itself (filepath.WalkDir visits the dir before
			// SkipDir suppresses children) — what matters is its
			// CONTENTS don't leak. Check no noise/deep/noise.txt.
			if strings.Contains(out, filepath.Join(n, "deep", "noise.txt")) {
				t.Errorf("recursive list_files recursed into noise dir %q — saw deep/noise.txt:\n%s", n, out)
			}
		}
		// Legit files / dirs still present.
		if !strings.Contains(out, "main.go") {
			t.Errorf("recursive list_files must include main.go; got:\n%s", out)
		}
	})

	t.Run("explicit root-only target remains listable", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(repo, "eval", "cases"), 0o755); err != nil {
			t.Fatalf("seed eval/cases: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repo, "eval", "cases", "u11a.case"), []byte("case"), 0o644); err != nil {
			t.Fatalf("seed eval case: %v", err)
		}
		res, _ := tool.Execute(bus, json.RawMessage(`{"path":"eval","recursive":true}`))
		out := res.Summary
		if !strings.Contains(out, filepath.Join("eval", "cases", "u11a.case")) {
			t.Fatalf("explicit eval/ target should remain listable, got:\n%s", out)
		}
	})
}

// TestExcludeDirsAnyLevel_Coverage pins the central blocklist
// against drift. Adding a new noise dir requires updating this
// assertion explicitly.
func TestExcludeDirsAnyLevel_Coverage(t *testing.T) {
	mustHave := []string{
		".git", ".hg", ".svn", ".codrax",
		"node_modules", "vendor", "__pycache__", ".tox", ".venv", "venv",
		".mypy_cache", ".pytest_cache", ".idea", ".vscode", ".vs",
		"target", "dist", "build", ".gradle", ".cargo",
		".next", ".nuxt", ".turbo", ".pnpm-store",
	}
	for _, name := range mustHave {
		if !ExcludeDirsAnyLevelSet[name] {
			t.Errorf("ExcludeDirsAnyLevelSet missing required entry %q — list_files / grep / repomap will leak it", name)
		}
	}
}

func TestExcludeDirPatterns_CoverTransientToolCaches(t *testing.T) {
	for _, name := range []string{".gotmp-tool", ".gotmp-eval", ".gocache", ".gocache-worktree", ".vs"} {
		if !IsExcludedDirName(name) {
			t.Fatalf("IsExcludedDirName(%q)=false, want true", name)
		}
	}
	if !IsExcludedRelativePath(filepath.Join("sub", ".gotmp-tool", "file.go")) {
		t.Fatal("nested .gotmp* directory should be excluded")
	}
	if !IsExcludedRelativePath(filepath.Join(".gocache-all", "obj", "x.go")) {
		t.Fatal("root .gocache* directory should be excluded")
	}
	for _, rel := range []string{
		filepath.Join("internal", "memory", "cache.go"),
		filepath.Join("pkg", "gotmp_helper", "main.go"),
	} {
		if IsExcludedRelativePath(rel) {
			t.Fatalf("IsExcludedRelativePath(%q) should be false", rel)
		}
	}
}

func TestIsWindowsReservedDevicePath(t *testing.T) {
	wantReserved := runtime.GOOS == "windows"
	for _, name := range []string{
		"nul",
		"NUL",
		"nul.txt",
		filepath.Join("sub", "con.log"),
		filepath.Join("nested", "LPT1"),
	} {
		if got := IsWindowsReservedDevicePath(name); got != wantReserved {
			t.Fatalf("IsWindowsReservedDevicePath(%q) = %v, want %v on %s", name, got, wantReserved, runtime.GOOS)
		}
	}
	for _, name := range []string{"null.go", "console.txt", "auxiliary.md", "component.go"} {
		if IsWindowsReservedDevicePath(name) {
			t.Fatalf("IsWindowsReservedDevicePath(%q) should be false", name)
		}
	}
}
