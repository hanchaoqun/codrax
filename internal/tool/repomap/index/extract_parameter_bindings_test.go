package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestBackfillCallableParameterBindingsTypedLanguages(t *testing.T) {
	cases := []struct {
		name, language, source, callable, binding, declaredType string
	}{
		{
			name: "go", language: types.LangGo,
			source:   "package x\nfunc Build(bus *BusContext, stage Stage) { _ = bus.Mutable }\n",
			callable: "Build", binding: "bus", declaredType: "*BusContext",
		},
		{
			name: "java", language: types.LangJava,
			source:   "class P { void build(BusContext bus, Stage stage) { bus.read(); } }\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
		{
			name: "rust", language: types.LangRust,
			source:   "fn build(bus: &BusContext, stage: Stage) { bus.read(); }\n",
			callable: "build", binding: "bus", declaredType: "&BusContext",
		},
		{
			name: "typescript", language: types.LangTypeScript,
			source:   "function build(bus: BusContext, stage: Stage): void { bus.read(); }\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
		{
			name: "arkts", language: types.LangArkTS,
			source:   "function build(bus: BusContext, stage: Stage): void { bus.read(); }\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
		{
			name: "python", language: types.LangPython,
			source:   "def build(bus: BusContext, stage: Stage):\n    bus.read()\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
		{
			name: "kotlin", language: types.LangKotlin,
			source:   "fun build(bus: BusContext, stage: Stage) { bus.read() }\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
		{
			name: "swift", language: types.LangSwift,
			source:   "func build(bus: BusContext, stage: Stage) { bus.read() }\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
		{
			name: "c", language: types.LangC,
			source:   "void build(BusContext *bus, Stage stage) { bus->value = 1; }\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
		{
			name: "cpp", language: types.LangCpp,
			source:   "void build(BusContext *bus, Stage stage) { bus->value = 1; }\n",
			callable: "build", binding: "bus", declaredType: "BusContext",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := parseSourceFor(t, tc.language, tc.source)
			syms := []types.Symbol{{Name: tc.callable, Kind: "function", Line: 1, EndLine: 3}}
			if tc.language == types.LangGo {
				syms[0].Line = 2
			}
			backfillCallableParameterBindings(root, []byte(tc.source), syms)
			if !hasCallableParameterBinding(syms[0].ParameterBindings, tc.binding, tc.declaredType) {
				t.Fatalf("parameter binding %s:%s missing: %+v; tree=%s", tc.binding, tc.declaredType, syms[0].ParameterBindings, root.String())
			}
		})
	}
}

func TestCangjieCallableParameterBindings(t *testing.T) {
	_, syms, _, _, tier := extractCangjie([]byte(`package demo
func build(bus: BusContext, stage: Stage): Unit {
    bus.read()
}`), "pipeline.cj")
	if tier != 1 {
		t.Fatalf("cangjie tier=%d, want 1", tier)
	}
	for _, sym := range syms {
		if sym.Name == "build" {
			if !hasCallableParameterBinding(sym.ParameterBindings, "bus", "BusContext") ||
				!hasCallableParameterBinding(sym.ParameterBindings, "stage", "Stage") {
				t.Fatalf("cangjie parameter bindings incomplete: %+v", sym.ParameterBindings)
			}
			return
		}
	}
	t.Fatalf("cangjie build symbol missing: %+v", syms)
}

func TestBackfillCallableParameterBindingsLeavesUntypedLanguagesEmpty(t *testing.T) {
	cases := []struct{ language, source string }{
		{types.LangJavaScript, "function build(bus, stage) { bus.read(); }\n"},
		{types.LangRuby, "def build(bus, stage)\n  bus.read\nend\n"},
		{types.LangLua, "function build(bus, stage)\n  bus.read()\nend\n"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			root := parseSourceFor(t, tc.language, tc.source)
			syms := []types.Symbol{{Name: "build", Kind: "function", Line: 1, EndLine: 3}}
			backfillCallableParameterBindings(root, []byte(tc.source), syms)
			if len(syms[0].ParameterBindings) != 0 {
				t.Fatalf("untyped parameters must not mint static identity: %+v; tree=%s", syms[0].ParameterBindings, root.String())
			}
		})
	}
}

func TestParseOneFilePublishesCallableParameterBindings(t *testing.T) {
	cases := []struct {
		language, rel, source, callable, binding, declaredType string
	}{
		{types.LangGo, "builder.go", "package p\nfunc Build(bus *BusContext) { _ = bus.Mutable }\n", "Build", "bus", "*BusContext"},
		{types.LangArkTS, "builder.ets", "function build(bus: BusContext): void { bus.read(); }\n", "build", "bus", "BusContext"},
		{types.LangCangjie, "builder.cj", "package p\nfunc build(bus: BusContext): Unit { bus.read() }\n", "build", "bus", "BusContext"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.rel)
			if err := os.WriteFile(path, []byte(tc.source), 0o600); err != nil {
				t.Fatal(err)
			}
			fi := parseOneFile(FileEntry{AbsPath: path, RelPath: tc.rel, Language: tc.language, Size: int64(len(tc.source))})
			for _, sym := range fi.Symbols {
				if sym.Name == tc.callable {
					if !hasCallableParameterBinding(sym.ParameterBindings, tc.binding, tc.declaredType) {
						t.Fatalf("parseOneFile parameter binding missing: %+v", sym)
					}
					return
				}
			}
			t.Fatalf("callable %s missing from %+v", tc.callable, fi.Symbols)
		})
	}
}

func hasCallableParameterBinding(bindings []types.CallableParameterBinding, name, declaredType string) bool {
	for _, binding := range bindings {
		if binding.Binding == name && binding.Type == declaredType {
			return true
		}
	}
	return false
}
