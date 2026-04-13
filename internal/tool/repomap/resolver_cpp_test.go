package repomap

import "testing"

// TestCppImportResolver covers the four resolution tiers plus a
// basename-collision case that the path-aware suffix index is
// supposed to fix relative to the legacy pure-basename match.
func TestCppImportResolver(t *testing.T) {
	files := []*FileInfo{
		{RelPath: "src/main.c", Language: LangC},
		{RelPath: "src/util.h", Language: LangC},
		{RelPath: "include/proj/api.h", Language: LangC},
		{RelPath: "src/net/http.h", Language: LangC},
		{RelPath: "src/db/http.h", Language: LangC}, // basename collides with net/http.h
		{RelPath: "third/libfoo/common.hpp", Language: LangCpp},
	}
	g := BuildGraph(t.TempDir(), files)

	r := &cppImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	cases := []struct {
		label   string
		fi      *FileInfo
		imp     string
		want    []string
		wantRsn string
	}{
		{
			label: "sibling in same directory",
			fi:    files[0], // src/main.c
			imp:   "util.h",
			want:  []string{"src/util.h"},
		},
		{
			label: "subdirectory relative from src/main.c",
			fi:    files[0],
			imp:   "net/http.h",
			want:  []string{"src/net/http.h"},
		},
		{
			label: "repo-root relative absolute include",
			fi:    files[0],
			imp:   "include/proj/api.h",
			want:  []string{"include/proj/api.h"},
		},
		{
			label: "partial path via suffix index (from a file outside src)",
			fi:    files[2], // include/proj/api.h
			imp:   "proj/api.h",
			want:  []string{"include/proj/api.h"},
		},
		{
			label: "path-aware disambiguation: db/http.h over net/http.h",
			fi:    files[2],
			imp:   "db/http.h",
			want:  []string{"src/db/http.h"},
		},
		{
			label: "pure-basename collision returns all candidates",
			fi:    files[2],
			imp:   "http.h",
			want:  []string{"src/net/http.h", "src/db/http.h"},
		},
		{
			label: "cpp header via suffix index",
			fi:    files[0],
			imp:   "libfoo/common.hpp",
			want:  []string{"third/libfoo/common.hpp"},
		},
		{
			label:   "stdlib external",
			fi:      files[0],
			imp:     "stdio.h",
			wantRsn: "cpp_external",
		},
		{
			label:   "cpp stdlib external with no extension",
			fi:      files[5],
			imp:     "iostream",
			wantRsn: "cpp_external",
		},
		{
			label:   "totally unknown include",
			fi:      files[0],
			imp:     "random/missing/thing.h",
			wantRsn: "cpp_external",
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

// TestCppImportResolverSharedInstance verifies Prepare is idempotent
// so the shared LangC/LangCpp registration in defaultResolvers
// doesn't double-index files.
func TestCppImportResolverSharedInstance(t *testing.T) {
	files := []*FileInfo{
		{RelPath: "a.h", Language: LangC},
		{RelPath: "b.hpp", Language: LangCpp},
	}
	g := BuildGraph(t.TempDir(), files)

	r := &cppImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatal(err)
	}
	first := len(r.suffixIndex)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.suffixIndex) != first {
		t.Errorf("second Prepare changed suffixIndex size: %d → %d",
			first, len(r.suffixIndex))
	}
	// Entries must not have grown either.
	for key, vs := range r.suffixIndex {
		if len(vs) > 1 {
			t.Errorf("suffixIndex[%q] double-populated: %v", key, vs)
		}
	}
}
