package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestExtractCangjie_PackageAndFunc pins the minimum surface: a
// package_clause must become FileInfo.Package (red line L-Cangjie-2)
// and a plain `main` function must be recognised.
func TestExtractCangjie_PackageAndFunc(t *testing.T) {
	src := []byte(`
package demo.hello

main(): Int64 {
    println("hi")
    return 0
}
`)
	pkg, syms, _, _, tier := extractCangjie(src, "main.cj")
	if pkg != "demo.hello" {
		t.Errorf("package from package_clause: want demo.hello, got %q", pkg)
	}
	if tier != 1 {
		t.Errorf("minimal Cangjie should be Tier 1; got %d", tier)
	}
	if !containsSymbol(syms, "main", "function") {
		t.Errorf("missing main function; got %+v", symKeys(syms))
	}
}

// TestExtractCangjie_PackageMustComeFromClause enforces red line
// L-Cangjie-2 — never infer package from path. A file in a deeply
// nested dir without a package_clause must have Package="".
func TestExtractCangjie_PackageMustComeFromClause(t *testing.T) {
	src := []byte(`
// No package statement.
public func helper(): Unit {}
`)
	pkg, syms, _, _, _ := extractCangjie(src, "src/utils/inner/helper.cj")
	if pkg != "" {
		t.Errorf("package MUST come from package_clause, not path; got %q", pkg)
	}
	if !containsSymbol(syms, "helper", "function") {
		t.Errorf("helper function should still extract without package; got %+v", symKeys(syms))
	}
}

// TestExtractCangjie_ExtendBlock verifies `extend` yields both a
// Symbol{Kind:extend} and an inheritance Relation — the graph
// tracks type-extension edges.
func TestExtractCangjie_ExtendBlock(t *testing.T) {
	src := []byte(`
package demo.strext

extend String {
    operator func >>(n: Int64): String {
        this
    }
}
`)
	_, syms, _, rels, _ := extractCangjie(src, "strext.cj")
	if !containsSymbol(syms, "String", "extend") {
		t.Errorf("expected extend symbol on String; got %+v", symKeys(syms))
	}
	found := false
	for _, r := range rels {
		if r.Kind == "inheritance" && r.ToEP.Name == "String" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected inheritance relation to String; got %+v", rels)
	}
}

// TestExtractCangjie_ForeignAndOperator pins the two less-common
// declaration shapes that the skill prompt mentions.
func TestExtractCangjie_ForeignAndOperator(t *testing.T) {
	src := []byte(`
package demo.ffi

foreign func native_add(a: Int64, b: Int64): Int64

public class V {
    public operator func +(other: V): V {
        this
    }
}
`)
	_, syms, _, _, _ := extractCangjie(src, "ffi.cj")
	if !containsSymbol(syms, "native_add", "foreign-func") {
		t.Errorf("missing foreign-func; got %+v", symKeys(syms))
	}
	foundOp := false
	for _, s := range syms {
		if s.Kind == "operator" && s.Name == "operator +" {
			foundOp = true
		}
	}
	if !foundOp {
		t.Errorf("missing operator + symbol; got %+v", symKeys(syms))
	}
}

// TestExtractCangjie_PublicVsInternalVisibility verifies the
// IsExported mapping (public → Exported, default → not).
func TestExtractCangjie_PublicVsInternalVisibility(t *testing.T) {
	src := []byte(`
package demo.vis

public class Public1 {}
class Package1 {}
public func pf(): Unit {}
func packagef(): Unit {}
`)
	_, syms, _, _, _ := extractCangjie(src, "vis.cj")
	want := map[string]bool{"Public1": true, "Package1": false, "pf": true, "packagef": false}
	for _, s := range syms {
		exp, ok := want[s.Name]
		if !ok {
			continue
		}
		if s.Exported != exp {
			t.Errorf("%s: want Exported=%v, got %v", s.Name, exp, s.Exported)
		}
	}
}

func TestExtractCangjie_PublicModifierSequencesRemainTypeDeclarations(t *testing.T) {
	src := []byte(`
package demo.modifiers

public sealed class Animal {}
public abstract class Service {}
open class Hook {}
internal class Cache {}
`)
	_, syms, _, _, tier := extractCangjie(src, "modifiers.cj")
	if tier != 1 {
		t.Fatalf("modifier sequence fixture should stay on Tier 1 parser, got tier %d", tier)
	}
	want := map[string]struct {
		kind     string
		exported bool
		doc      string
	}{
		"Animal":  {"class", true, "public sealed"},
		"Service": {"class", true, "public abstract"},
		"Hook":    {"class", true, "open"},
		"Cache":   {"class", false, "internal"},
	}
	for _, s := range syms {
		exp, ok := want[s.Name]
		if !ok {
			continue
		}
		if s.Kind != exp.kind || s.Exported != exp.exported || s.Doc != exp.doc {
			t.Fatalf("%s symbol = kind=%q exported=%v doc=%q, want kind=%q exported=%v doc=%q; all=%+v",
				s.Name, s.Kind, s.Exported, s.Doc, exp.kind, exp.exported, exp.doc, symKeys(syms))
		}
		delete(want, s.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing modifier-sequence symbols %v in %v", want, symKeys(syms))
	}
}

func TestExtractCangjie_EndLinesPresentForBodies(t *testing.T) {
	src := []byte(`
package demo.lines

extend String {
    operator func >>(n: Int64): String {
        this
    }
}

public class Greeter {
    init() {
        println("hi")
    }

    public func greet(): String {
        return "hi"
    }
}

foreign func native_add(a: Int64, b: Int64): Int64
`)
	_, syms, _, _, _ := extractCangjie(src, "lines.cj")
	assertEndLine := func(name, kind string, wantGreater bool) {
		t.Helper()
		for _, s := range syms {
			if s.Name == name && s.Kind == kind {
				if wantGreater && s.EndLine <= s.Line {
					t.Fatalf("%s/%s EndLine=%d should be > Line=%d", name, kind, s.EndLine, s.Line)
				}
				if !wantGreater && s.EndLine != s.Line {
					t.Fatalf("%s/%s EndLine=%d should equal Line=%d", name, kind, s.EndLine, s.Line)
				}
				return
			}
		}
		t.Fatalf("missing symbol %s/%s in %+v", name, kind, symKeys(syms))
	}

	assertEndLine("String", "extend", true)
	assertEndLine("Greeter", "class", true)
	assertEndLine("init", "ctor", true)
	assertEndLine("greet", "method", true)
	assertEndLine("native_add", "foreign-func", false)
}

func containsSymbol(syms []types.Symbol, name, kind string) bool {
	for _, s := range syms {
		if s.Name == name && s.Kind == kind {
			return true
		}
	}
	return false
}
