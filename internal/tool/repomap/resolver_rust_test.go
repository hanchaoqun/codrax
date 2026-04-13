package repomap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRustImportResolver builds a two-crate workspace on disk, runs
// Prepare, and exercises the self/super/crate anchors plus the
// walker's file.rs-vs-mod.rs fallback and the glob/group/type stops.
func TestRustImportResolver(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(rel, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Main crate "app"
	writeFile("app/Cargo.toml", `
[package]
name = "app"
version = "0.1.0"
`)
	writeFile("app/src/lib.rs", "")
	writeFile("app/src/a.rs", "")
	writeFile("app/src/a/b.rs", "")
	writeFile("app/src/mid/mod.rs", "")
	writeFile("app/src/mid/leaf.rs", "")

	// Sibling crate "util" with main.rs only
	writeFile("util/Cargo.toml", `
[package]
name = "util"
`)
	writeFile("util/src/main.rs", "")
	writeFile("util/src/helper.rs", "")

	files := []*FileInfo{
		{RelPath: "app/Cargo.toml", IsSpecial: true},
		{RelPath: "util/Cargo.toml", IsSpecial: true},
		{RelPath: "app/src/lib.rs", Language: LangRust},
		{RelPath: "app/src/a.rs", Language: LangRust},
		{RelPath: "app/src/a/b.rs", Language: LangRust},
		{RelPath: "app/src/mid/mod.rs", Language: LangRust},
		{RelPath: "app/src/mid/leaf.rs", Language: LangRust},
		{RelPath: "util/src/main.rs", Language: LangRust},
		{RelPath: "util/src/helper.rs", Language: LangRust},
	}
	g := BuildGraph(dir, files)

	var r rustImportResolver
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, ok := r.crates["app"]; !ok {
		t.Fatalf("crate 'app' missing; got %+v", r.crates)
	}
	if _, ok := r.crates["util"]; !ok {
		t.Fatalf("crate 'util' missing; got %+v", r.crates)
	}
	if r.fileCrate["app/src/a/b.rs"] != "app" {
		t.Errorf("fileCrate[a/b.rs] = %q, want app", r.fileCrate["app/src/a/b.rs"])
	}
	if r.fileCrate["util/src/helper.rs"] != "util" {
		t.Errorf("fileCrate[helper] = %q, want util", r.fileCrate["util/src/helper.rs"])
	}

	cases := []struct {
		label     string
		fi        *FileInfo
		imp       string
		want      []string
		wantRsn   string
	}{
		{
			label: "crate walk to .rs leaf",
			fi:    files[2], // app/src/lib.rs
			imp:   "crate::a::b",
			want:  []string{"app/src/a/b.rs"},
		},
		{
			label: "crate walk stops at Type segment",
			fi:    files[2],
			imp:   "crate::a::b::SomeType",
			want:  []string{"app/src/a/b.rs"},
		},
		{
			label: "crate walk to mod.rs module",
			fi:    files[2],
			imp:   "crate::mid::leaf",
			want:  []string{"app/src/mid/leaf.rs"},
		},
		{
			label: "crate walk to mid itself",
			fi:    files[2],
			imp:   "crate::mid",
			want:  []string{"app/src/mid/mod.rs"},
		},
		{
			label: "self relative walk",
			fi:    files[3], // app/src/a.rs
			imp:   "self::b::Thing",
			want:  []string{"app/src/a/b.rs"},
		},
		{
			label: "super relative walk",
			fi:    files[4], // app/src/a/b.rs
			imp:   "super::b", // parent dir is "app/src/a", so b → a/b.rs
			want:  []string{"app/src/a/b.rs"},
		},
		{
			label: "workspace cross-crate import",
			fi:    files[2],
			imp:   "util::helper::Helper",
			want:  []string{"util/src/helper.rs"},
		},
		{
			label: "workspace cross-crate bare name resolves to root",
			fi:    files[2],
			imp:   "util::SomeThing",
			want:  []string{"util/src/main.rs"},
		},
		{
			label: "glob is stripped, walks to parent module",
			fi:    files[2],
			imp:   "crate::a::b::*",
			want:  []string{"app/src/a/b.rs"},
		},
		{
			label: "group is stripped, walks to parent module",
			fi:    files[2],
			imp:   "crate::a::{X, Y}",
			want:  []string{"app/src/a.rs"},
		},
		{
			label: "`as` alias is stripped",
			fi:    files[2],
			imp:   "crate::a::b as rebranded",
			want:  []string{"app/src/a/b.rs"},
		},
		{
			label:   "external crate",
			fi:      files[2],
			imp:     "serde::Deserialize",
			wantRsn: "rust_external",
		},
		{
			label:   "bare single-identifier non-existent mod",
			fi:      files[2],
			imp:     "nonexistent",
			wantRsn: "rust_external",
		},
		{
			label: "bare `mod a` reference to existing sibling file",
			fi:    files[2],
			imp:   "a",
			want:  []string{"app/src/a.rs"},
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			before := len(ctx.Unresolved)
			got := r.Resolve(g, c.fi, Import{Path: c.imp}, ctx)
			if !sameStringSet(got, c.want) {
				t.Errorf("Resolve(%q) = %v, want %v", c.imp, got, c.want)
			}
			if c.wantRsn != "" {
				if len(ctx.Unresolved) != before+1 {
					t.Fatalf("expected one Unresolved append, got delta %d",
						len(ctx.Unresolved)-before)
				}
				if ctx.Unresolved[before].Reason != c.wantRsn {
					t.Errorf("Reason = %q, want %q",
						ctx.Unresolved[before].Reason, c.wantRsn)
				}
			} else if len(ctx.Unresolved) != before {
				t.Errorf("unexpected Unresolved append: %+v",
					ctx.Unresolved[before:])
			}
		})
	}
}

// TestCleanRustImportPath locks the extractor-output normalization
// that the resolver relies on before splitting segments.
func TestCleanRustImportPath(t *testing.T) {
	cases := map[string]string{
		"crate::a::b":                "crate::a::b",
		"use crate::a::b;":           "crate::a::b",
		"use crate::a::b::*;":        "crate::a::b",
		"use crate::a::{X, Y};":      "crate::a",
		"use crate::a::b as rename;": "crate::a::b",
		"pub use crate::a::b;":       "crate::a::b",
		"":                           "",
	}
	for in, want := range cases {
		if got := cleanRustImportPath(in); got != want {
			t.Errorf("cleanRustImportPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseCargoToml verifies the minimal TOML parser only picks up
// [package].name and [lib].path, ignoring other sections and inline
// comments.
func TestParseCargoToml(t *testing.T) {
	src := []byte(`
# top-level comment
[package]
name = "foo-crate"  # inline comment
version = "0.1.0"

[dependencies]
serde = "1"

[lib]
path = "src/entry.rs"
`)
	info := parseCargoToml(src)
	if info.packageName != "foo-crate" {
		t.Errorf("packageName = %q, want foo-crate", info.packageName)
	}
	if info.libPath != "src/entry.rs" {
		t.Errorf("libPath = %q, want src/entry.rs", info.libPath)
	}
}
