package types

import "testing"

func TestPathInsidePendingSubRepo(t *testing.T) {
	pending := []string{"claude/codrax", "small/codrax-small"}
	cases := []struct {
		file string
		want bool
	}{
		{"claude/codrax/internal/x.go", true},
		{"claude/codrax", true},
		{"small/codrax-small/foo", true},
		{"codrax/internal/x.go", false},
		{"opencode/src/x.ts", false},
		{"claude/codrax-extra/x.go", false}, // prefix bleed guard
		{"", false},
	}
	for _, c := range cases {
		if got := PathInsidePendingSubRepo(c.file, pending); got != c.want {
			t.Errorf("PathInsidePendingSubRepo(%q) = %v, want %v", c.file, got, c.want)
		}
	}
}

func TestPathInsidePendingSubRepo_EmptyInputs(t *testing.T) {
	if PathInsidePendingSubRepo("codrax/x.go", nil) {
		t.Errorf("nil pending list should return false")
	}
	if PathInsidePendingSubRepo("", []string{"codrax"}) {
		t.Errorf("empty file should return false")
	}
}

func TestPathInsidePendingSubRepo_SkipsEmptyRootRels(t *testing.T) {
	// "." and "" are skipped so single-repo topologies (where the
	// RootRel might be ".") do not produce false positives.
	if PathInsidePendingSubRepo("codrax/x.go", []string{".", ""}) {
		t.Errorf("'.' / '' rootRels must not match any path")
	}
}

func TestPathInsidePendingSubRepo_BackslashNormalisation(t *testing.T) {
	// Windows-style paths normalize to POSIX slashes for the prefix
	// match (matches the same normalisation in
	// MultiRepoActiveSetGater.ResolveActiveSetPath).
	if !PathInsidePendingSubRepo("claude\\codrax\\x.go", []string{"claude/codrax"}) {
		t.Errorf("backslash paths must normalise for prefix match")
	}
}
