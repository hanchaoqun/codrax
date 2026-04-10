package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
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
		if !strings.Contains(result.Summary, content) {
			t.Fatalf("expected body to contain original content, got %q", result.Summary)
		}
	})

	t.Run("small file ignores offset and limit", func(t *testing.T) {
		// Regression: read_file used to honor offset/limit on tiny files,
		// so the LLM passing limit=20 on a 66-line source file would
		// silently miss the answer past line 20. Now small files (<=
		// MaxInlineBytes) always return the whole content with a banner
		// announcing the override.
		tmpFile := filepath.Join(t.TempDir(), "testoffset.txt")
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
		if !strings.Contains(result.Summary, "offset/limit ignored because the file fits inline") {
			t.Fatalf("expected passthrough banner, got %q", result.Summary)
		}
		// All lines must be present, not just the sliced two.
		for _, line := range []string{"line0", "line1", "line2", "line3", "line4"} {
			if !strings.Contains(result.Summary, line) {
				t.Fatalf("expected full content to contain %q, got %q", line, result.Summary)
			}
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
		if strings.TrimSpace(result.Summary) != "hello" {
			t.Fatalf("expected 'hello', got %q", result.Summary)
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
		if result.Summary != "no matches found" {
			t.Fatalf("expected 'no matches found', got %q", result.Summary)
		}
	})
}

func TestRepoMap(t *testing.T) {
	t.Run("generate tree for temp dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "src")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("hi"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		tool := &RepoMap{}
		params, _ := json.Marshal(repoMapParams{Path: tmpDir})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "src/") {
			t.Errorf("expected 'src/' in tree output, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "main.go") {
			t.Errorf("expected 'main.go' in tree output, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "README.md") {
			t.Errorf("expected 'README.md' in tree output, got: %s", result.Summary)
		}
	})
}

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

func TestRunTests(t *testing.T) {
	t.Run("run echo command", func(t *testing.T) {
		tool := &RunTests{}
		params, _ := json.Marshal(runTestsParams{Command: "echo test-passed"})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}
		if !strings.Contains(result.Summary, "test-passed") {
			t.Fatalf("expected 'test-passed' in output, got: %s", result.Summary)
		}
	})

	// Empty / whitespace-only command must surface as a failed tool
	// result, NOT silently fall through to a hardcoded language
	// default. The previous "go test ./..." fallback produced
	// confusing failures in non-Go repositories and gave the LLM a
	// free pass to skip thinking about which tests to run.
	t.Run("empty command rejected", func(t *testing.T) {
		tool := &RunTests{}
		for _, cmd := range []string{"", "   ", "\t\n"} {
			params, _ := json.Marshal(runTestsParams{Command: cmd})
			result, err := tool.Execute(newBusContext(), params)
			if err != nil {
				t.Fatalf("Execute(%q): unexpected error: %v", cmd, err)
			}
			if result.Success {
				t.Errorf("Execute(%q): expected failure, got success", cmd)
			}
			if !strings.Contains(result.Summary, "requires the 'command' parameter") {
				t.Errorf("Execute(%q): expected actionable message, got %q", cmd, result.Summary)
			}
		}
	})
}
