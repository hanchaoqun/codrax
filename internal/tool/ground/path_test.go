package ground

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalRepoRelative(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		repoRoot string
		want     string
	}{
		{
			name: "empty path returns empty",
			path: "", repoRoot: "/repo", want: "",
		},
		{
			name: "absolute inside repoRoot is stripped",
			path: "/mnt/d/repo/internal/x.go", repoRoot: "/mnt/d/repo", want: "internal/x.go",
		},
		{
			name: "absolute path equals repoRoot",
			path: "/mnt/d/repo", repoRoot: "/mnt/d/repo", want: ".",
		},
		{
			name: "absolute outside repoRoot returns cleaned original",
			path: "/etc/passwd", repoRoot: "/mnt/d/repo", want: "/etc/passwd",
		},
		{
			name: "empty repoRoot leaves path unchanged (cleaned)",
			path: "internal/./x.go", repoRoot: "", want: "internal/x.go",
		},
		{
			name: "relative path is cleaned",
			path: "./internal/../internal/x.go", repoRoot: "/repo", want: "internal/x.go",
		},
		{
			name: "repoRoot with trailing slash still matches",
			path: "/mnt/d/repo/README.md", repoRoot: "/mnt/d/repo/", want: "README.md",
		},
		{
			name: "absolute path that would escape via `..` returns cleaned absolute",
			path: "/mnt/e/other/x.go", repoRoot: "/mnt/d/repo", want: "/mnt/e/other/x.go",
		},
		{
			name: "README at repo top-level",
			path: "/mnt/d/temp/donghu/iSulad-master/README.md",
			repoRoot: "/mnt/d/temp/donghu/iSulad-master",
			want: "README.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalRepoRelative(tt.path, tt.repoRoot)
			if got != tt.want {
				t.Errorf("CanonicalRepoRelative(%q, %q) = %q, want %q",
					tt.path, tt.repoRoot, got, tt.want)
			}
		})
	}
}

// TestCanonicalRepoRelative_RelativeRepoRoot reproduces the original
// field failure (iSulad architecture question, kept=0/12 again after
// the first canonicalisation commit). The CLI default is `-repo .`
// (relative); the LLM's read_file banners land as absolute paths
// because the prescan / repo_map output references files by their
// absolute CWD-prefixed form. Without Abs-resolving repoRoot,
// filepath.Rel errors out on the absolute-target / relative-base
// mismatch and the helper returns the uncanonicalised input, which
// then fails every whitelist / LineIndex / FileIndex lookup.
//
// The fix runs repoRoot through filepath.Abs against the process
// CWD before Rel. For this test we chdir into a temp dir that plays
// the role of the iSulad repo.
func TestCanonicalRepoRelative_RelativeRepoRoot(t *testing.T) {
	// Create a directory to stand in for the repo root.
	dir := t.TempDir()
	// Resolve symlinks so /tmp vs /private/tmp on macOS agrees with
	// the chdir result — otherwise filepath.Abs(".") can come back
	// with a different prefix than `dir` and we'd be testing the
	// wrong thing.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	// Chdir into the repo; restore on exit so other tests aren't
	// affected by the CWD change.
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(resolved); err != nil {
		t.Fatalf("Chdir(%q): %v", resolved, err)
	}

	absFile := filepath.Join(resolved, "README.md")

	// The canonicaliser must strip the absolute prefix even though
	// repoRoot is the bare "." string — Abs(".") resolves to the
	// current repo directory.
	got := CanonicalRepoRelative(absFile, ".")
	if got != "README.md" {
		t.Errorf("relative repoRoot (`.`): CanonicalRepoRelative(%q, \".\") = %q, want %q",
			absFile, got, "README.md")
	}

	// Symmetric case: relative citation + `-repo .` must also yield
	// the canonical basename. The whitelist / LineIndex / FileIndex
	// lookups will compare "README.md" == "README.md" and pass.
	got = CanonicalRepoRelative("README.md", ".")
	if got != "README.md" {
		t.Errorf("relative citation + `-repo .`: got %q, want %q", got, "README.md")
	}
}
