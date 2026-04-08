package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/design/internal/types"
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
		if result.Summary != content {
			t.Fatalf("expected %q, got %q", content, result.Summary)
		}
	})

	t.Run("read with offset and limit", func(t *testing.T) {
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
		if result.Summary != "line1\nline2" {
			t.Fatalf("expected %q, got %q", "line1\nline2", result.Summary)
		}
	})
}

func TestWriteFile(t *testing.T) {
	t.Run("write to temp file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "subdir", "testwrite.txt")
		content := "written content here"

		tool := &WriteFile{}
		params, _ := json.Marshal(writeFileParams{Path: tmpFile, Content: content})
		result, err := tool.Execute(newBusContext(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got: %s", result.Summary)
		}

		data, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("failed to read written file: %v", err)
		}
		if string(data) != content {
			t.Fatalf("expected %q, got %q", content, string(data))
		}
	})
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
}
