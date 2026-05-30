package index

import (
	"strings"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// parseKotlin is a test helper that mirrors parser.go's tree-sitter
// bootstrap so we can exercise extractKotlin directly.
func parseKotlin(t *testing.T, src []byte) *sitter.Node {
	t.Helper()
	root, ok := parseTreeSitterIfPossible(types.LangKotlin, src)
	if !ok {
		t.Fatalf("parseTreeSitterIfPossible(kotlin) returned !ok — tree-sitter-kotlin binding missing?")
	}
	return root
}

// TestExtractKotlin_Basics verifies the canonical Kotlin surface:
// package clause, imports, a class with a function, a data class,
// and a top-level function.
func TestExtractKotlin_Basics(t *testing.T) {
	src := []byte(`package com.example.app

import android.os.Bundle
import kotlinx.coroutines.launch

data class User(val id: Int, val name: String)

class MainActivity {
    fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
    }
}

fun helper(s: String): String = s.uppercase()
`)
	root := parseKotlin(t, src)
	pkg, syms, imps, _ := extractKotlin(root, src, "MainActivity.kt")

	if pkg != "com.example.app" {
		t.Errorf("package: want com.example.app, got %q", pkg)
	}
	if len(imps) < 2 {
		t.Errorf("want ≥2 imports, got %d", len(imps))
	}
	haveImport := map[string]bool{}
	for _, i := range imps {
		haveImport[i.Path] = true
	}
	for _, want := range []string{"android.os.Bundle", "kotlinx.coroutines.launch"} {
		if !haveImport[want] {
			t.Errorf("missing import %q; have %+v", want, haveImport)
		}
	}

	kinds := map[string]string{}
	for _, s := range syms {
		kinds[s.Name] = s.Kind
	}
	if kinds["User"] != "data-class" {
		t.Errorf("User: want kind=data-class, got %q", kinds["User"])
	}
	if kinds["MainActivity"] != "class" {
		t.Errorf("MainActivity: want kind=class, got %q", kinds["MainActivity"])
	}
	if kinds["helper"] != "function" {
		t.Errorf("top-level fn helper: want kind=function, got %q", kinds["helper"])
	}
	if kinds["onCreate"] != "method" {
		t.Errorf("member fn onCreate: want kind=method, got %q", kinds["onCreate"])
	}
}

// TestExtractKotlin_WildcardImportPreserved pins that a wildcard
// import keeps its `.*` suffix so kotlinImportResolver can tell a
// whole-package wildcard apart from a member import of the same
// dotted text (mirrors the Java extractor). Stripping it would make
// `import a.b.*` resolve to nothing.
func TestExtractKotlin_WildcardImportPreserved(t *testing.T) {
	src := []byte(`package com.example.app

import com.example.util.*
import android.os.Bundle
`)
	root := parseKotlin(t, src)
	_, _, imps, _ := extractKotlin(root, src, "Main.kt")
	have := map[string]bool{}
	for _, i := range imps {
		have[i.Path] = true
	}
	if !have["com.example.util.*"] {
		t.Errorf("wildcard import path not preserved; have %+v", have)
	}
}

// TestExtractKotlin_Visibility verifies the IsExported mapping
// derived from modifiers (Kotlin default is public).
func TestExtractKotlin_Visibility(t *testing.T) {
	src := []byte(`package v

class PubClass {}
internal class IntClass {}
private class PrivClass {}

fun pubFun() {}
private fun privFun() {}
internal fun intFun() {}
`)
	root := parseKotlin(t, src)
	_, syms, _, _ := extractKotlin(root, src, "v.kt")
	want := map[string]bool{
		"PubClass":  true,
		"IntClass":  false,
		"PrivClass": false,
		"pubFun":    true,
		"privFun":   false,
		"intFun":    false,
	}
	for _, s := range syms {
		exp, ok := want[s.Name]
		if !ok {
			continue
		}
		if s.Exported != exp {
			t.Errorf("%s: want Exported=%v (mods=%q), got %v", s.Name, exp, s.Doc, s.Exported)
		}
	}
}

// TestExtractKotlin_ObjectAndCompanion verifies that object
// declarations surface as Symbol.Kind=object. Companion object
// is lumped under object_declaration by tree-sitter-kotlin, which
// we pass through as-is.
func TestExtractKotlin_ObjectAndCompanion(t *testing.T) {
	src := []byte(`package demo

object Singleton {
    fun greet(): String = "hi"
}

class Container {
    companion object Factory {
        fun create(): Container = Container()
    }
}
`)
	root := parseKotlin(t, src)
	_, syms, _, _ := extractKotlin(root, src, "demo.kt")
	hasObject := false
	for _, s := range syms {
		if s.Name == "Singleton" && s.Kind == "object" {
			hasObject = true
		}
	}
	if !hasObject {
		t.Errorf("missing object symbol; got %+v", symKeys(syms))
	}
}

// TestExtractKotlin_SuspendAndDataClass proves the modifier
// refinement: `suspend` promotes function kind to `suspend-function`,
// and `data class` promotes class kind to `data-class`.
func TestExtractKotlin_SuspendAndDataClass(t *testing.T) {
	src := []byte(`package demo

data class Result(val value: Int)

suspend fun fetch(): Result = Result(42)
`)
	root := parseKotlin(t, src)
	_, syms, _, _ := extractKotlin(root, src, "demo.kt")
	kinds := map[string]string{}
	for _, s := range syms {
		kinds[s.Name] = s.Kind
	}
	if kinds["Result"] != "data-class" {
		t.Errorf("Result: want data-class, got %q", kinds["Result"])
	}
	if kinds["fetch"] != "suspend-function" {
		t.Errorf("fetch: want suspend-function, got %q", kinds["fetch"])
	}
}

// TestExtractKotlin_IsExtractorRobustToEmpty pins the no-crash
// invariant on empty / whitespace-only input.
func TestExtractKotlin_IsExtractorRobustToEmpty(t *testing.T) {
	cases := []string{"", "\n\n", "// just a comment\n"}
	for _, in := range cases {
		root := parseKotlin(t, []byte(in))
		pkg, syms, imps, rels := extractKotlin(root, []byte(in), "empty.kt")
		if pkg != "" || len(syms) != 0 || len(imps) != 0 || len(rels) != 0 {
			t.Errorf("empty input should produce empty extraction; got pkg=%q syms=%d imps=%d rels=%d for %q",
				pkg, len(syms), len(imps), len(rels), strings.TrimSpace(in))
		}
	}
}
