package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDryBuildRust_NoCargoTomlNoOp confirms the helper bails when
// the repo has no Cargo.toml — the next-language helper runs.
func TestDryBuildRust_NoCargoTomlNoOp(t *testing.T) {
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{Path: "main.rs", Kind: "create", NewContent: "fn main() {}\n"}}
	if rej := dryBuildRust(bus, changes); rej != "" {
		t.Errorf("no Cargo.toml → no Rust dry-build → no rejection; got %q", rej)
	}
}

// TestDryBuildRust_NoRsChangeNoOp confirms the helper bails when
// the plan touches zero .rs files even with a Cargo.toml present.
func TestDryBuildRust_NoRsChangeNoOp(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "Cargo.toml"), []byte(`[package]
name = "stub"
version = "0.1.0"
`), 0o644); err != nil {
		t.Fatalf("seed Cargo.toml: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{Path: "main.go", Kind: "create", NewContent: "package main\n"}}
	if rej := dryBuildRust(bus, changes); rej != "" {
		t.Errorf("no .rs change → no Rust dry-build → no rejection; got %q", rej)
	}
}

// TestDryBuildRust_DeleteIgnored confirms kind=delete on a .rs file
// is treated as no-op for dry-build (deletions don't need to compile).
func TestDryBuildRust_DeleteIgnored(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "Cargo.toml"), []byte("[package]\nname = \"x\"\nversion = \"0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{Path: "src/old.rs", Kind: "delete"}}
	if rej := dryBuildRust(bus, changes); rej != "" {
		t.Errorf("kind=delete .rs change should be no-op; got %q", rej)
	}
}

// TestDryBuildSwift_NoPackageNoOp: no Package.swift → skip.
func TestDryBuildSwift_NoPackageNoOp(t *testing.T) {
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{Path: "Foo.swift", Kind: "create", NewContent: "func foo() {}\n"}}
	if rej := dryBuildSwift(bus, changes); rej != "" {
		t.Errorf("no Package.swift → no Swift dry-build; got %q", rej)
	}
}

// TestDryBuildSwift_NoSwiftChangeNoOp: Package.swift present but no
// .swift change → skip.
func TestDryBuildSwift_NoSwiftChangeNoOp(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "Package.swift"), []byte("// stub\n"), 0o644); err != nil {
		t.Fatalf("seed Package.swift: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{Path: "main.go", Kind: "create", NewContent: "package main\n"}}
	if rej := dryBuildSwift(bus, changes); rej != "" {
		t.Errorf("no .swift change → no Swift dry-build; got %q", rej)
	}
}

// TestDryBuildJava_NoManifestNoOp: .java change but no pom.xml /
// build.gradle → skip silently (vanilla javac project, dry-build
// can't reliably help).
func TestDryBuildJava_NoManifestNoOp(t *testing.T) {
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{Path: "Foo.java", Kind: "create", NewContent: "class Foo {}\n"}}
	if rej := dryBuildJava(bus, changes); rej != "" {
		t.Errorf("no Maven/Gradle manifest → no Java dry-build; got %q", rej)
	}
}

// TestDryBuildJava_NoJavaChangeNoOp: pom.xml present but plan
// touches zero .java files → skip.
func TestDryBuildJava_NoJavaChangeNoOp(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "pom.xml"), []byte("<project/>\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{Path: "x.go", Kind: "create", NewContent: "package main\n"}}
	if rej := dryBuildJava(bus, changes); rej != "" {
		t.Errorf("no .java change → no Java dry-build; got %q", rej)
	}
}

// TestDryBuildKotlin_NoKtChangeNoOp: zero .kt changes → skip.
func TestDryBuildKotlin_NoKtChangeNoOp(t *testing.T) {
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{Path: "main.go", Kind: "create", NewContent: "package main\n"}}
	if rej := dryBuildKotlin(bus, changes); rej != "" {
		t.Errorf("no .kt change → no Kotlin dry-build; got %q", rej)
	}
}

// TestDryBuildRust_SyntaxErrorRejected exercises the rejection path
// when the cargo binary IS available. Skipped on machines without
// the toolchain (most CI). Uses a minimal cargo project shape.
func TestDryBuildRust_SyntaxErrorRejected(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo not on PATH; skip Rust rejection test")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "Cargo.toml"), []byte(`[package]
name = "drybuild_test"
version = "0.1.0"
edition = "2021"

[[bin]]
name = "drybuild_test"
path = "src/main.rs"
`), 0o644); err != nil {
		t.Fatalf("seed Cargo.toml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "main.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatalf("seed main.rs: %v", err)
	}
	// Plan emits a syntactically invalid replacement.
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "src/main.rs",
		Kind:       "modify",
		NewContent: "fn main( {\n", // unmatched paren — guaranteed parse error
	}}
	rej := dryBuildRust(bus, changes)
	if rej == "" {
		t.Fatal("Rust dry-build must reject syntactically broken new_content")
	}
	if !strings.Contains(rej, "dry-build failed") {
		t.Errorf("rejection should carry dry-build prefix; got %q", rej)
	}
	if !strings.Contains(rej, "Rust") {
		t.Errorf("rejection should name the language; got %q", rej)
	}
}

// TestDryBuildKotlin_SyntaxErrorRejected exercises the per-file
// kotlinc rejection path. Skipped without kotlinc on PATH.
func TestDryBuildKotlin_SyntaxErrorRejected(t *testing.T) {
	if _, err := exec.LookPath("kotlinc"); err != nil {
		t.Skip("kotlinc not on PATH; skip Kotlin rejection test")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "Foo.kt",
		Kind:       "create",
		NewContent: "fun foo( {}\n", // unmatched paren
	}}
	rej := dryBuildKotlin(bus, changes)
	if rej == "" {
		t.Fatal("Kotlin dry-build must reject syntactically broken new_content")
	}
	if !strings.Contains(rej, "dry-build failed") {
		t.Errorf("rejection should carry dry-build prefix; got %q", rej)
	}
	if !strings.Contains(rej, "Kotlin") {
		t.Errorf("rejection should name the language; got %q", rej)
	}
}

// TestFileExists covers the tiny helper used by the Java path.
func TestFileExists(t *testing.T) {
	d := t.TempDir()
	present := filepath.Join(d, "real.txt")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !fileExists(present) {
		t.Errorf("fileExists should return true for an existing file")
	}
	if fileExists(filepath.Join(d, "missing.txt")) {
		t.Errorf("fileExists should return false for a missing file")
	}
}
