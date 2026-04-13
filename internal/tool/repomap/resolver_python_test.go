package repomap

import "testing"

// TestPythonImportResolver exercises the Python dotted-module index
// plus relative-import delegation + parent-package fallback. All
// fixtures are synthetic FileInfos with extractor-shaped Package
// strings (path-derived dotted names, __init__ suffix for package
// files).
func TestPythonImportResolver(t *testing.T) {
	files := []*FileInfo{
		{RelPath: "app/main.py", Language: LangPython, Package: "app.main"},
		{RelPath: "app/__init__.py", Language: LangPython, Package: "app.__init__"},
		{RelPath: "app/core/engine.py", Language: LangPython, Package: "app.core.engine"},
		{RelPath: "app/core/__init__.py", Language: LangPython, Package: "app.core.__init__"},
		{RelPath: "app/util/strings.py", Language: LangPython, Package: "app.util.strings"},
	}
	g := BuildGraph(t.TempDir(), files)

	var r pythonImportResolver
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
			label: "absolute module import",
			fi:    files[0],
			imp:   "app.core.engine",
			want:  []string{"app/core/engine.py"},
		},
		{
			label: "package __init__ resolution",
			fi:    files[0],
			imp:   "app.core",
			want:  []string{"app/core/__init__.py"},
		},
		{
			label: "top-level package via __init__",
			fi:    files[2],
			imp:   "app",
			want:  []string{"app/__init__.py"},
		},
		{
			label: "from-import where name is re-exported by parent",
			fi:    files[0],
			imp:   "app.core.Engine", // not a module; Engine is a class re-exported by app.core
			want:  []string{"app/core/__init__.py"},
		},
		{
			label:   "stdlib external",
			fi:      files[0],
			imp:     "os",
			wantRsn: "python_external",
		},
		{
			label:   "third-party external",
			fi:      files[0],
			imp:     "numpy.linalg",
			wantRsn: "python_external",
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
