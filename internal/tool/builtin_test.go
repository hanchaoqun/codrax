package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	})
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
}

func TestGitLog(t *testing.T) {
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
