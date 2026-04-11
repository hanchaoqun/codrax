package patcher

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newTestPatcher(t *testing.T) (*Patcher, string) {
	t.Helper()
	root := t.TempDir()
	p, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}

func TestWrite_NewFile(t *testing.T) {
	p, root := newTestPatcher(t)
	res, err := p.Apply(context.Background(), Request{
		Path:    "new/hello.txt",
		Op:      OpWrite,
		Content: "hi\n",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusApplied {
		t.Errorf("status = %s, want applied", res.Status)
	}
	got := readFile(t, filepath.Join(root, "new/hello.txt"))
	if got != "hi\n" {
		t.Errorf("content = %q, want %q", got, "hi\n")
	}
}

func TestWrite_Unchanged(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "same\n")

	res, err := p.Apply(context.Background(), Request{
		Path:    "a.txt",
		Op:      OpWrite,
		Content: "same\n",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusUnchanged {
		t.Errorf("status = %s, want unchanged", res.Status)
	}
}

func TestEdit_Success(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.go")
	writeFile(t, path, "package main\n\nfunc foo() {}\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.go",
		Op:        OpEdit,
		OldString: "foo",
		NewString: "bar",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusApplied {
		t.Errorf("status = %s", res.Status)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "func bar()") {
		t.Errorf("edit not applied: %q", got)
	}
	if res.DiffPreview == "" {
		t.Error("expected diff preview")
	}
}

func TestEdit_NoMatch(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "hello\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.txt",
		Op:        OpEdit,
		OldString: "goodbye",
		NewString: "hi",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusNoMatch {
		t.Errorf("status = %s, want no_match", res.Status)
	}
	if readFile(t, path) != "hello\n" {
		t.Error("file should be untouched")
	}
}

func TestEdit_Ambiguous(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "a\na\na\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.txt",
		Op:        OpEdit,
		OldString: "a",
		NewString: "b",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusAmbiguous {
		t.Errorf("status = %s, want ambiguous", res.Status)
	}
	if res.MatchesFound != 3 {
		t.Errorf("matches = %d, want 3", res.MatchesFound)
	}
}

func TestEdit_AlreadyApplied(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "after\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.txt",
		Op:        OpEdit,
		OldString: "before",
		NewString: "after",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusAlreadyApplied {
		t.Errorf("status = %s, want already_applied", res.Status)
	}
}

func TestEdit_FileNotFound(t *testing.T) {
	p, _ := newTestPatcher(t)
	_, err := p.Apply(context.Background(), Request{
		Path:      "missing.txt",
		Op:        OpEdit,
		OldString: "x",
		NewString: "y",
	})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestInsertBefore_Success(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.go")
	writeFile(t, path, "func main() {}\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.go",
		Op:        OpInsertBefore,
		OldString: "func main()",
		NewString: "// entry\n",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusApplied {
		t.Errorf("status = %s", res.Status)
	}
	got := readFile(t, path)
	want := "// entry\nfunc main() {}\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInsertAfter_Success(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.go")
	writeFile(t, path, "package main\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.go",
		Op:        OpInsertAfter,
		OldString: "package main\n",
		NewString: "\nimport \"fmt\"\n",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusApplied {
		t.Errorf("status = %s", res.Status)
	}
	got := readFile(t, path)
	want := "package main\n\nimport \"fmt\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInsertBefore_AlreadyApplied(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.go")
	writeFile(t, path, "// entry\nfunc main() {}\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.go",
		Op:        OpInsertBefore,
		OldString: "func main()",
		NewString: "// entry\n",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusAlreadyApplied {
		t.Errorf("status = %s, want already_applied", res.Status)
	}
}

func TestInsertAfter_AlreadyApplied(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.go")
	writeFile(t, path, "package main\nimport \"fmt\"\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.go",
		Op:        OpInsertAfter,
		OldString: "package main\n",
		NewString: "import \"fmt\"\n",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusAlreadyApplied {
		t.Errorf("status = %s, want already_applied", res.Status)
	}
}

func TestEOL_PreservesCRLF(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "line1\r\nline2\r\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.txt",
		Op:        OpEdit,
		OldString: "line2",
		NewString: "LINE2",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.EOL != "\r\n" {
		t.Errorf("EOL = %q, want \\r\\n", res.EOL)
	}
	got := readFile(t, path)
	want := "line1\r\nLINE2\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEOL_NormalizesOldStringAcrossLines(t *testing.T) {
	// old_string is written with \n but file uses \r\n — should still match.
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "foo\r\nbar\r\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.txt",
		Op:        OpEdit,
		OldString: "foo\nbar",
		NewString: "foo\nBAZ",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusApplied {
		t.Errorf("status = %s, want applied", res.Status)
	}
	got := readFile(t, path)
	want := "foo\r\nBAZ\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafety_PathEscapeRejected(t *testing.T) {
	p, _ := newTestPatcher(t)
	_, err := p.Apply(context.Background(), Request{
		Path:    "../escape.txt",
		Op:      OpWrite,
		Content: "x",
	})
	if err == nil {
		t.Error("expected error for path escape")
	}
}

func TestSafety_BinaryRejected(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.bin")
	writeFile(t, path, "abc\x00def")

	_, err := p.Apply(context.Background(), Request{
		Path:      "a.bin",
		Op:        OpEdit,
		OldString: "abc",
		NewString: "xyz",
	})
	if err == nil {
		t.Error("expected error for binary file")
	}
}

func TestDryRun_DoesNotWrite(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "foo\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.txt",
		Op:        OpEdit,
		OldString: "foo",
		NewString: "bar",
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusApplied {
		t.Errorf("status = %s", res.Status)
	}
	if readFile(t, path) != "foo\n" {
		t.Error("file should be unchanged in dry run")
	}
}

func TestWrite_PreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve POSIX executable bits via os.Chmod")
	}

	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.sh")
	writeFile(t, path, "old")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	_, err := p.Apply(context.Background(), Request{
		Path:    "a.sh",
		Op:      OpWrite,
		Content: "new",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 0755", info.Mode().Perm())
	}
}

func TestInsert_AnchorNotFound(t *testing.T) {
	p, root := newTestPatcher(t)
	path := filepath.Join(root, "a.txt")
	writeFile(t, path, "hello\n")

	res, err := p.Apply(context.Background(), Request{
		Path:      "a.txt",
		Op:        OpInsertBefore,
		OldString: "missing",
		NewString: "x",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Status != StatusNoMatch {
		t.Errorf("status = %s, want no_match", res.Status)
	}
}
