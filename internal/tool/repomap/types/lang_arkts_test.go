package types

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsArkTSProject_SubRepoBoundary verifies the L-ArkTS-3 sub-repo
// boundary rule: an oh-package.json5 sitting in the parent tree must
// NOT promote .ts files inside a sibling sub-repo (the multi-repo
// ArkTS leak guarded against in lang.go::IsArkTSProject).
//
// Layout under tmp:
//
//	parent/
//	  oh-package.json5            ← outer ArkTS marker
//	  sub-arkts/
//	    .git/                     ← sub-repo boundary
//	    oh-package.json5          ← own marker (positive case)
//	    feature/foo.ts
//	  sub-pure-ts/
//	    .git/                     ← sub-repo boundary
//	    src/bar.ts                ← MUST NOT inherit parent's oh-package.json5
func TestIsArkTSProject_SubRepoBoundary(t *testing.T) {
	parent := t.TempDir()

	// Outer parent ArkTS marker (the leak source).
	if err := os.WriteFile(filepath.Join(parent, "oh-package.json5"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write parent oh-package.json5: %v", err)
	}

	// sub-arkts: own .git + oh-package.json5
	subA := filepath.Join(parent, "sub-arkts")
	if err := os.MkdirAll(filepath.Join(subA, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sub-arkts/.git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subA, "oh-package.json5"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write sub-arkts/oh-package.json5: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(subA, "feature"), 0o755); err != nil {
		t.Fatalf("mkdir feature: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subA, "feature", "foo.ts"), []byte(""), 0o644); err != nil {
		t.Fatalf("write foo.ts: %v", err)
	}

	// sub-pure-ts: own .git, NO oh-package.json5
	subB := filepath.Join(parent, "sub-pure-ts")
	if err := os.MkdirAll(filepath.Join(subB, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir sub-pure-ts/.git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(subB, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subB, "src", "bar.ts"), []byte(""), 0o644); err != nil {
		t.Fatalf("write bar.ts: %v", err)
	}

	cases := []struct {
		name    string
		relPath string
		want    bool
	}{
		{
			name:    "sub-arkts feature/foo.ts is ArkTS (own marker, no boundary cross)",
			relPath: "sub-arkts/feature/foo.ts",
			want:    true,
		},
		{
			name:    "sub-pure-ts src/bar.ts is NOT ArkTS — .git boundary stops walk before parent oh-package.json5 leaks in",
			relPath: "sub-pure-ts/src/bar.ts",
			want:    false,
		},
		{
			name:    "file at parent root level inherits parent oh-package.json5",
			relPath: "stray.ts",
			want:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsArkTSProject(parent, tc.relPath)
			if got != tc.want {
				t.Errorf("IsArkTSProject(%q, %q) = %v, want %v", parent, tc.relPath, got, tc.want)
			}
		})
	}
}

// TestIsArkTSProject_GitFilePointer verifies that .git as a regular
// file (worktree/submodule pointer) also counts as a sub-repo boundary.
func TestIsArkTSProject_GitFilePointer(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "oh-package.json5"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write parent oh-package.json5: %v", err)
	}
	sub := filepath.Join(parent, "submod")
	if err := os.MkdirAll(filepath.Join(sub, "src"), 0o755); err != nil {
		t.Fatalf("mkdir submod/src: %v", err)
	}
	// .git as a regular file (git worktree/submodule pointer form)
	if err := os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: ../.git/worktrees/submod\n"), 0o644); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "src", "x.ts"), []byte(""), 0o644); err != nil {
		t.Fatalf("write x.ts: %v", err)
	}
	if got := IsArkTSProject(parent, "submod/src/x.ts"); got {
		t.Errorf("submod/src/x.ts should NOT be ArkTS (.git pointer is a sub-repo boundary), got true")
	}
}

// TestIsArkTSProject_Single verifies the legacy single-repo behaviour
// stays intact: oh-package.json5 at repoRoot promotes .ts files
// arbitrarily deep, so long as no inner .git boundary intervenes.
func TestIsArkTSProject_Single(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "oh-package.json5"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write oh-package.json5: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "entry", "src", "main", "ets", "pages"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	rel := "entry/src/main/ets/pages/Index.ts"
	if err := os.WriteFile(filepath.Join(repo, rel), []byte(""), 0o644); err != nil {
		t.Fatalf("write Index.ts: %v", err)
	}
	if got := IsArkTSProject(repo, rel); !got {
		t.Errorf("deep file in single ArkTS repo should be ArkTS, got false")
	}
}

// TestIsArkTSProject_EmptyRoot guards the documented contract.
func TestIsArkTSProject_EmptyRoot(t *testing.T) {
	if got := IsArkTSProject("", "any/file.ts"); got {
		t.Errorf("empty repoRoot must return false, got true")
	}
}
