package ground

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestGroundItemScoped_DispatchesByScope confirms each scope routes
// to its own grounder — a guard against future refactors collapsing
// the dispatch table back into a single line-only path.
func TestGroundItemScoped_DispatchesByScope(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("")}
	gc := BuildContext(ctx)
	cases := []struct {
		name      string
		scope     types.EvidenceScope
		minimumOK func(t *testing.T, it *types.EvidenceItem) // soft check
	}{
		{
			name:  "line scope routes to line grounder",
			scope: types.ScopeLine,
		},
		{
			name:  "line_range scope routes to range grounder",
			scope: types.ScopeLineRange,
		},
		{
			name:  "section scope routes to section grounder",
			scope: types.ScopeSection,
		},
		{
			name:  "file scope routes to file grounder",
			scope: types.ScopeFile,
		},
		{
			name:  "crossfile scope routes to crossfile grounder",
			scope: types.ScopeCrossfile,
		},
		{
			name:  "negative scope routes to negative grounder",
			scope: types.ScopeNegative,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := &types.EvidenceItem{Scope: c.scope}
			rep := GroundItemScoped(it, gc)
			// Whatever the grounder did, it must have set Status —
			// the report carries it back.
			if rep.Status == "" {
				t.Errorf("GroundItemScoped did not set Status for scope=%s", c.scope)
			}
			if it.GroundingStatus == "" {
				t.Errorf("GroundItemScoped did not set item.GroundingStatus for scope=%s", c.scope)
			}
		})
	}
}

// TestGroundScopeFile_RoleConsistency pins the path-shape consistency
// rule: FileRoleConfigCanonical requires LooksLikeConfigFilePath;
// FileRoleManifest requires a known manifest filename.
func TestGroundScopeFile_RoleConsistency(t *testing.T) {
	tmp := t.TempDir()
	yamlPath := filepath.Join(tmp, "codrax.yaml")
	if err := os.WriteFile(yamlPath, []byte("# test config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goPath := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(goPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gc := &Context{RepoRoot: tmp}

	t.Run("config file with config_canonical → grounded", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope:         types.ScopeFile,
			Source:        "codrax.yaml",
			FileRoleLabel: types.FileRoleConfigCanonical,
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingGrounded {
			t.Errorf("status=%s, want grounded; note=%s", rep.Status, rep.Note)
		}
	})

	t.Run("go file with config_canonical → ungrounded (role mismatch)", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope:         types.ScopeFile,
			Source:        "main.go",
			FileRoleLabel: types.FileRoleConfigCanonical,
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingUngrounded {
			t.Errorf("expected ungrounded for go-file with config role; got %s", rep.Status)
		}
		if !strings.Contains(rep.Note, "inconsistent") {
			t.Errorf("expected role-mismatch note; got %q", rep.Note)
		}
	})
}

// TestGroundScopeCrossfile_AssertsOverActualFiles verifies the
// crossfile grounder REALLY scans the files. The LLM cannot cheat by
// asserting "no match" against a file that does match.
func TestGroundScopeCrossfile_AssertsOverActualFiles(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "main.go"),
		[]byte("flag.StringVar(&FooFlag, \"explore-foo\", \"\", \"\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gc := &Context{RepoRoot: tmp}

	t.Run("forbidden assertion that the LLM lied about → ungrounded", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope: types.ScopeCrossfile,
			CrossfileQuery: &types.CrossfileQuery{
				Files:   []string{"main.go"},
				Pattern: `flag\.StringVar`,
			},
			CrossfileAssertion: &types.CrossfileAssertion{Kind: types.CrossfileForbidden},
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingUngrounded {
			t.Errorf("status=%s, want ungrounded (LLM claim wrong); note=%s", rep.Status, rep.Note)
		}
		if !strings.Contains(rep.Note, "FAILED") {
			t.Errorf("expected FAILED in note; got %q", rep.Note)
		}
	})

	t.Run("forbidden assertion that's actually true → grounded", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope: types.ScopeCrossfile,
			CrossfileQuery: &types.CrossfileQuery{
				Files:   []string{"main.go"},
				Pattern: `flag\.StringVar.*Explore[A-Z]`, // looks for "ExploreXxx" specifically
			},
			CrossfileAssertion: &types.CrossfileAssertion{Kind: types.CrossfileForbidden},
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingGrounded {
			t.Errorf("status=%s, want grounded; note=%s", rep.Status, rep.Note)
		}
	})

	t.Run("exists assertion validated", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope: types.ScopeCrossfile,
			CrossfileQuery: &types.CrossfileQuery{
				Files:   []string{"main.go"},
				Pattern: `flag\.StringVar`,
			},
			CrossfileAssertion: &types.CrossfileAssertion{Kind: types.CrossfileExists},
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingGrounded {
			t.Errorf("status=%s, want grounded; note=%s", rep.Status, rep.Note)
		}
	})
}

// TestGroundScopeNegative_AbsenceCheckIsReal confirms negative scope
// is verified by ACTUAL file scan, not LLM assertion alone.
func TestGroundScopeNegative_AbsenceCheckIsReal(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "codrax.yaml"),
		[]byte("# explore_midloop_min_iteration: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gc := &Context{RepoRoot: tmp}

	t.Run("absence claim that's true → grounded", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope: types.ScopeNegative,
			Kind:  types.EvidenceAbsent,
			NegativeQuery: &types.NegativeQuery{
				File:    "codrax.yaml",
				Pattern: "explore_mid_loop_hint_budget",
			},
			NegativeScope: types.NegativeScopeFile,
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingGrounded {
			t.Errorf("status=%s, want grounded; note=%s", rep.Status, rep.Note)
		}
	})

	t.Run("absence claim that's false → ungrounded", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope: types.ScopeNegative,
			Kind:  types.EvidenceAbsent,
			NegativeQuery: &types.NegativeQuery{
				File:    "codrax.yaml",
				Pattern: "explore_midloop_min_iteration", // actually present (commented)
			},
			NegativeScope: types.NegativeScopeFile,
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingUngrounded {
			t.Errorf("status=%s, want ungrounded; note=%s", rep.Status, rep.Note)
		}
		if !strings.Contains(rep.Note, "FAILED") {
			t.Errorf("expected FAILED in note; got %q", rep.Note)
		}
	})

	t.Run("range absence ignores matches outside range", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(tmp, "ridge.py"),
			[]byte("store_cv_values = True\n\nclass RidgeClassifierCV:\n    pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		it := &types.EvidenceItem{
			Scope:     types.ScopeNegative,
			Kind:      types.EvidenceAbsent,
			LineStart: 3,
			LineEnd:   4,
			NegativeQuery: &types.NegativeQuery{
				File:    "ridge.py",
				Pattern: "store_cv_values",
			},
			NegativeScope: types.NegativeScopeRange,
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingGrounded {
			t.Errorf("status=%s, want grounded; note=%s", rep.Status, rep.Note)
		}
		if !strings.Contains(rep.Note, "range 3-4") {
			t.Errorf("expected range note; got %q", rep.Note)
		}
	})

	t.Run("range absence fails on match inside range", func(t *testing.T) {
		it := &types.EvidenceItem{
			Scope:     types.ScopeNegative,
			Kind:      types.EvidenceAbsent,
			LineStart: 1,
			LineEnd:   1,
			NegativeQuery: &types.NegativeQuery{
				File:    "ridge.py",
				Pattern: "store_cv_values",
			},
			NegativeScope: types.NegativeScopeRange,
		}
		rep := GroundItemScoped(it, gc)
		if rep.Status != types.GroundingUngrounded {
			t.Errorf("status=%s, want ungrounded; note=%s", rep.Status, rep.Note)
		}
	})
}

// TestGroundScopeNegative_OversizedFileFailsSoft pins the customer-OOM fix
// (2026-07-03): a model-supplied negative/crossfile file naming a GiB-scale
// artifact must not be slurped by the grounder — the stat-first bound makes
// the read fail and the item degrades to Ungrounded with the read error in
// the note, never an allocation.
func TestGroundScopeNegative_OversizedFileFailsSoft(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "berlin.systrace")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(width.SourceReadMaxBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	gc := &Context{RepoRoot: tmp}
	it := &types.EvidenceItem{
		Scope: types.ScopeNegative,
		Kind:  types.EvidenceAbsent,
		NegativeQuery: &types.NegativeQuery{
			File:    "berlin.systrace",
			Pattern: "anything",
		},
		NegativeScope: types.NegativeScopeFile,
	}
	rep := GroundItemScoped(it, gc)
	if rep.Status != types.GroundingUngrounded {
		t.Fatalf("oversized file must fail soft as ungrounded, got %s (note=%s)", rep.Status, rep.Note)
	}
	if !strings.Contains(rep.Note, "exceeds") {
		t.Fatalf("note should carry the bound diagnosis: %q", rep.Note)
	}
}
