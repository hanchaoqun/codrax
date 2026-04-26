package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestProjectLintRegistry_OrderAndCoverage pins the V6 registry
// against silent drift. Currently 2 entries (ArkTS, Cangjie); the
// 3 build-system formats (CMake/Meson/Make) and the C/C++ family
// are intentionally V5-only (single-file). Adding a new V6 entry
// requires updating this assertion.
func TestProjectLintRegistry_OrderAndCoverage(t *testing.T) {
	want := []string{"ArkTS", "Cangjie"}
	if len(projectLintRegistry) != len(want) {
		t.Fatalf("projectLintRegistry size = %d, want %d", len(projectLintRegistry), len(want))
	}
	for i, exp := range want {
		row := projectLintRegistry[i]
		if row.Name != exp {
			t.Errorf("entry %d Name = %q, want %q", i, row.Name, exp)
		}
		if len(row.Extensions) == 0 {
			t.Errorf("entry %d %q has no extensions", i, exp)
		}
		if len(row.ManifestFiles) == 0 {
			t.Errorf("entry %d %q has no manifest files (V6 requires manifest detection)", i, exp)
		}
		if row.Binary == "" {
			t.Errorf("entry %d %q has empty Binary", i, exp)
		}
		if row.BuildArgs == nil {
			t.Errorf("entry %d %q has nil BuildArgs", i, exp)
		}
		if row.FailedFn == nil {
			t.Errorf("entry %d %q has nil FailedFn", i, exp)
		}
	}
}

// TestProjectLintExtensions_DisjointFromV5 enforces the design
// contract: V5 (single-file) and V6 (project-aware) must NEVER both
// claim the same extension, otherwise a single change would lint
// twice with potentially conflicting results. The V6 deferred set
// (.ets, .cj) was deferred from V5 PRECISELY because they need
// project context — V5 must remain blind to them.
func TestProjectLintExtensions_DisjointFromV5(t *testing.T) {
	v5Exts := make(map[string]string) // ext → V5 lang name
	for _, lang := range lintRegistry {
		for _, ext := range lang.Extensions {
			v5Exts[ext] = lang.Name
		}
	}
	for _, lang := range projectLintRegistry {
		for _, ext := range lang.Extensions {
			if name, ok := v5Exts[ext]; ok {
				t.Errorf("V6 %q claims extension %s but V5 %q also claims it; one must drop the extension to avoid double-linting",
					lang.Name, ext, name)
			}
		}
	}
}

// TestProjectLint_NoLangChange_NoOp: when changes contains no .ets
// / .cj entries, every V6 row short-circuits without spawning
// anything (mirrors V5's no-op behavior for non-matching plans).
func TestProjectLint_NoLangChange_NoOp(t *testing.T) {
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{Path: "main.go", Kind: "create", NewContent: "package x\n"}}
	if rej := validatePlanProjectLint(bus, changes); rej != "" {
		t.Errorf("Go-only change should not trigger any V6 row; got %q", rej)
	}
}

// TestProjectLint_NoManifest_NoOp: a plan that touches .ets files
// in a repo WITHOUT oh-package.json5 must skip — the file extension
// alone is ambiguous (.ts could be plain TS or ArkTS). Pins the
// "manifest = disambiguator" contract.
func TestProjectLint_NoManifest_NoOp(t *testing.T) {
	repo := t.TempDir()
	// Note: NO oh-package.json5 / build-profile.json5 in the repo.
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "Index.ets",
		Kind:       "create",
		NewContent: "@Component\nstruct Foo {}",
	}}
	if rej := validatePlanProjectLint(bus, changes); rej != "" {
		t.Errorf("ArkTS file in non-HarmonyOS repo should skip V6; got %q", rej)
	}
}

// TestProjectLint_BinaryMissing_NoOp: even with the manifest in
// place, a missing toolchain binary should silently skip rather
// than reject. Operators on a non-HarmonyOS-developer machine must
// be able to plan changes touching .ets files (e.g. someone on
// macOS reviewing a PR) without V6 falsely failing.
func TestProjectLint_BinaryMissing_NoOp(t *testing.T) {
	if _, err := exec.LookPath("hvigorw"); err == nil {
		t.Skip("hvigorw IS on PATH; this test pins the missing-binary path")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "oh-package.json5"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "Index.ets",
		Kind:       "create",
		NewContent: "@Component\nstruct Foo {}",
	}}
	if rej := validatePlanProjectLint(bus, changes); rej != "" {
		t.Errorf("missing hvigorw should silently skip even with manifest; got %q", rej)
	}
}

// TestProjectLint_ModifyKindSkipped: severity policy mirrors V5 —
// strict lint only on kind=create new files, modify/patch/delete
// skipped. ArkTS is the test bed since cjpm/hvigor isn't installed.
func TestProjectLint_ModifyKindSkipped(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "oh-package.json5"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "Index.ets",
		Kind:       "modify", // <-- modify, not create
		NewContent: "@Component\nstruct Foo {}",
	}}
	// Even if hvigorw were installed, modify-kind should skip the
	// V6 spawn before the binary lookup.
	if rej := validatePlanProjectLint(bus, changes); rej != "" {
		t.Errorf("kind=modify must skip V6 strict lint; got %q", rej)
	}
}

// TestProjectLint_DisabledViaSetting: SetLintEnabled(false) gates
// V6 the same way it gates V5. One master switch covers both
// validators.
func TestProjectLint_DisabledViaSetting(t *testing.T) {
	prev := LintEnabled()
	defer SetLintEnabled(prev)

	SetLintEnabled(false)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "oh-package.json5"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{Path: "Index.ets", Kind: "create", NewContent: "x"}}
	if rej := validatePlanProjectLint(bus, changes); rej != "" {
		t.Errorf("disabled lint must skip V6; got %q", rej)
	}
}

// TestProjectLintLanguagesEnabled_ReturnsRegistry exposes the
// operator-facing capability snapshot used by cmd/root.go's
// startup log. Mirrors V5's LintLanguagesEnabled test.
func TestProjectLintLanguagesEnabled_ReturnsRegistry(t *testing.T) {
	got := ProjectLintLanguagesEnabled()
	if len(got) != len(projectLintRegistry) {
		t.Fatalf("ProjectLintLanguagesEnabled len = %d, want %d", len(got), len(projectLintRegistry))
	}
	for i, st := range got {
		if st.Name != projectLintRegistry[i].Name {
			t.Errorf("entry %d Name = %q, want %q", i, st.Name, projectLintRegistry[i].Name)
		}
		if st.Binary != projectLintRegistry[i].Binary {
			t.Errorf("entry %d Binary = %q, want %q", i, st.Binary, projectLintRegistry[i].Binary)
		}
	}
}
