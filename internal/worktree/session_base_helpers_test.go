package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// session_base_helpers_test.go — V5-1: the exact helpers the analysis-base
// stamp relies on (commit existence and the dirty tracked roster).
func TestCommitExistsAndDirtyTrackedPaths(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "codrax-test@example.invalid")
	git("config", "user.name", "Codrax Test")
	for _, rel := range []string{"a.txt", "b.txt", "文档.c"} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	head, err := HeadSHA(root)
	if err != nil || len(head) != 40 {
		t.Fatalf("HeadSHA = %q %v", head, err)
	}
	if !CommitExists(root, head) || CommitExists(root, "0123456789abcdef0123456789abcdef01234567") {
		t.Fatal("CommitExists must be exact")
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "文档.c"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := DirtyTrackedPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"b.txt": true, "文档.c": true}
	if len(dirty) != 2 || !want[dirty[0]] || !want[dirty[1]] {
		t.Fatalf("dirty tracked roster = %v (untracked files excluded, non-ASCII unquoted)", dirty)
	}
}
