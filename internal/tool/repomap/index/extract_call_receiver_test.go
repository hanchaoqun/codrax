package index

import (
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestCallGraphLanguageMatrixCoversEverySupportedReadLanguage(t *testing.T) {
	// semantic: statically declared receiver types are promoted when unique;
	// source: dynamic receiver expressions are retained without guessed types;
	// function: ordinary function calls have no receiver axis;
	// declarative: Proto carries rpc declaration relations, not executable calls.
	capability := map[string]string{
		types.LangGo:         "semantic",
		types.LangPython:     "source",
		types.LangJavaScript: "source",
		types.LangTypeScript: "semantic",
		types.LangJava:       "semantic",
		types.LangKotlin:     "semantic",
		types.LangRust:       "semantic",
		types.LangC:          "function",
		types.LangCpp:        "semantic",
		types.LangRuby:       "source",
		types.LangSwift:      "semantic",
		types.LangLua:        "source",
		types.LangProto:      "declarative",
		types.LangArkTS:      "semantic",
		types.LangCangjie:    "semantic",
	}
	got := make([]string, 0, len(capability))
	for lang := range capability {
		got = append(got, lang)
	}
	sort.Strings(got)
	want := types.SupportedReadLanguages()
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("call graph capability matrix drift: got=%v want all supported=%v", got, want)
	}
	if capability[types.LangProto] != "declarative" {
		t.Fatal("Proto must not be presented as an executable source-call language")
	}
}

func requireResolvedCallTarget(t *testing.T, fi *types.FileInfo, operation, owner string) {
	t.Helper()
	graph := BuildGraph(t.TempDir(), []*types.FileInfo{fi})
	for _, rel := range fi.Relations {
		if rel.Kind != "call" || rel.ToEP.Name != operation {
			continue
		}
		if target := graph.ResolveCallTarget(fi, rel); target != nil && target.Name == operation &&
			(target.Parent == owner || target.Receiver == owner) {
			return
		}
	}
	t.Fatalf("no %s call resolved to %s in relations=%+v symbols=%+v", operation, owner, fi.Relations, fi.Symbols)
}

func TestStaticLanguageReceiverTypesResolveToDefinitionEndpoints(t *testing.T) {
	t.Run("typescript", func(t *testing.T) {
		src := []byte("class Repository { load() {} } function run(repo: Repository) { repo.load(); }\n")
		root := parseSourceFor(t, types.LangTypeScript, string(src))
		pkg, syms, _, rels := extractJS(root, src, "service.ts", true)
		requireResolvedCallTarget(t, &types.FileInfo{RelPath: "service.ts", Language: types.LangTypeScript, Package: pkg, Symbols: syms, Relations: rels}, "load", "Repository")
	})
	t.Run("kotlin", func(t *testing.T) {
		src := []byte("class Repository {\n  fun load() {}\n}\nfun run(repo: Repository) {\n  repo.load()\n}\n")
		root := parseSourceFor(t, types.LangKotlin, string(src))
		pkg, syms, _, rels := extractKotlin(root, src, "Service.kt")
		requireResolvedCallTarget(t, &types.FileInfo{RelPath: "Service.kt", Language: types.LangKotlin, Package: pkg, Symbols: syms, Relations: rels}, "load", "Repository")
	})
	t.Run("swift", func(t *testing.T) {
		src := []byte("class Repository {\n  func load() {}\n}\nfunc run(repo: Repository) {\n  repo.load()\n}\n")
		root := parseSourceFor(t, types.LangSwift, string(src))
		pkg, syms, _, rels := extractSwift(root, src, "Service.swift")
		requireResolvedCallTarget(t, &types.FileInfo{RelPath: "Service.swift", Language: types.LangSwift, Package: pkg, Symbols: syms, Relations: rels}, "load", "Repository")
	})
	t.Run("rust", func(t *testing.T) {
		src := []byte("struct Repository {} impl Repository { fn load(&self) {} } fn run(repo: &Repository) { repo.load(); }\n")
		root := parseSourceFor(t, types.LangRust, string(src))
		pkg, syms, _, rels := extractRust(root, src, "service.rs")
		requireResolvedCallTarget(t, &types.FileInfo{RelPath: "service.rs", Language: types.LangRust, Package: pkg, Symbols: syms, Relations: rels}, "load", "Repository")
	})
	t.Run("cpp", func(t *testing.T) {
		src := []byte("class Repository { public: void load() {} }; void run(Repository& repo) { repo.load(); }\n")
		root := parseSourceFor(t, types.LangCpp, string(src))
		pkg, syms, _, rels := extractCCpp(root, src, "service.cpp", types.LangCpp)
		requireResolvedCallTarget(t, &types.FileInfo{RelPath: "service.cpp", Language: types.LangCpp, Package: pkg, Symbols: syms, Relations: rels}, "load", "Repository")
	})
	t.Run("cangjie", func(t *testing.T) {
		src := []byte("package demo\nclass Repository { func load(): Unit {} }\nfunc run(repo: Repository): Unit { repo.load() }\n")
		pkg, syms, _, rels, _ := extractCangjie(src, "service.cj")
		requireResolvedCallTarget(t, &types.FileInfo{RelPath: "service.cj", Language: types.LangCangjie, Package: pkg, Symbols: syms, Relations: rels}, "load", "Repository")
	})
	t.Run("arkts", func(t *testing.T) {
		src := []byte("class Repository { load() {} } function run(repo: Repository) { repo.load(); }\n")
		pkg, syms, _, rels, _ := extractArkTS(src, "service.ets")
		requireResolvedCallTarget(t, &types.FileInfo{RelPath: "service.ets", Language: types.LangArkTS, Package: pkg, Symbols: syms, Relations: rels}, "load", "Repository")
	})
}

func requireCallReceiver(t *testing.T, rels []types.Relation, name, receiver string) {
	t.Helper()
	for _, rel := range rels {
		if rel.Kind == "call" && rel.ToEP.Name == name {
			if rel.ToEP.Receiver != receiver {
				t.Fatalf("%s receiver=%q, want %q: %+v", name, rel.ToEP.Receiver, receiver, rel)
			}
			return
		}
	}
	t.Fatalf("missing call relation for %s in %+v", name, rels)
}

func TestExistingCallExtractorsPreserveSourceReceiverIdentity(t *testing.T) {
	t.Run("python", func(t *testing.T) {
		src := []byte("def run(repo):\n    return repo.load()\n")
		root := parseSourceFor(t, types.LangPython, string(src))
		_, _, _, rels := extractPython(root, src, "service.py")
		requireCallReceiver(t, rels, "load", "repo")
	})
	t.Run("javascript", func(t *testing.T) {
		src := []byte("function run(repo) { return repo.load(); }\n")
		root := parseSourceFor(t, types.LangJavaScript, string(src))
		_, _, _, rels := extractJS(root, src, "service.js", false)
		requireCallReceiver(t, rels, "load", "repo")
	})
	t.Run("typescript", func(t *testing.T) {
		src := []byte("function run(repo: Repository) { return repo.load(); }\n")
		root := parseSourceFor(t, types.LangTypeScript, string(src))
		_, _, _, rels := extractJS(root, src, "service.ts", true)
		requireCallReceiver(t, rels, "load", "Repository")
	})
	t.Run("rust", func(t *testing.T) {
		src := []byte("fn run(repo: &Repository) { repo.load(); Repository::open(); }\n")
		root := parseSourceFor(t, types.LangRust, string(src))
		_, _, _, rels := extractRust(root, src, "service.rs")
		requireCallReceiver(t, rels, "load", "Repository")
		requireCallReceiver(t, rels, "open", "Repository")
	})
	t.Run("cpp", func(t *testing.T) {
		src := []byte("void run(Repository& repo) { repo.load(); Repository::open(); }\n")
		root := parseSourceFor(t, types.LangCpp, string(src))
		_, _, _, rels := extractCCpp(root, src, "service.cpp", types.LangCpp)
		requireCallReceiver(t, rels, "load", "Repository")
		requireCallReceiver(t, rels, "open", "Repository")
	})
}

func TestNewLanguageCallExtractorsPreserveDirectionAndReceiver(t *testing.T) {
	t.Run("kotlin", func(t *testing.T) {
		src := []byte("class Caller { fun run(repo: Repository) { repo.load(); helper() } }\n")
		root := parseSourceFor(t, types.LangKotlin, string(src))
		_, _, _, rels := extractKotlin(root, src, "Caller.kt")
		requireCallReceiver(t, rels, "load", "Repository")
		requireCallReceiver(t, rels, "helper", "")
	})
	t.Run("ruby", func(t *testing.T) {
		src := []byte("class Caller\n  def run(repo)\n    repo.load\n    helper(1)\n  end\nend\n")
		root := parseSourceFor(t, types.LangRuby, string(src))
		_, _, _, rels := extractRuby(root, src, "caller.rb")
		requireCallReceiver(t, rels, "load", "repo")
		requireCallReceiver(t, rels, "helper", "")
	})
	t.Run("swift", func(t *testing.T) {
		src := []byte("class Caller { func run(repo: Repository) { repo.load(); helper() } }\n")
		root := parseSourceFor(t, types.LangSwift, string(src))
		_, _, _, rels := extractSwift(root, src, "Caller.swift")
		requireCallReceiver(t, rels, "load", "Repository")
		requireCallReceiver(t, rels, "helper", "")
	})
	t.Run("lua", func(t *testing.T) {
		src := []byte("local socket = require \"socket\"\nfunction M.run(repo)\n  repo:load()\n  helper()\nend\n")
		root := parseSourceFor(t, types.LangLua, string(src))
		_, _, _, rels := extractLua(root, src, "caller.lua")
		requireCallReceiver(t, rels, "load", "repo")
		requireCallReceiver(t, rels, "helper", "")
		requireCallReceiver(t, rels, "require", "")
	})
	t.Run("cangjie", func(t *testing.T) {
		src := []byte("package demo\nfunc run(repo: Repository): Unit { repo.load(); this.service.save(); helper() }\n")
		_, _, _, rels, _ := extractCangjie(src, "caller.cj")
		requireCallReceiver(t, rels, "load", "Repository")
		requireCallReceiver(t, rels, "save", "this.service")
		requireCallReceiver(t, rels, "helper", "")
		for _, rel := range rels {
			if rel.Kind == "call" && rel.ToEP.Name == "run" {
				t.Fatalf("function declaration was misclassified as call: %+v", rel)
			}
		}
	})
	t.Run("arkts", func(t *testing.T) {
		src := []byte("@Entry\n@Component\nstruct Index {\n  build() { this.service.load(); Text('ok') }\n}\n")
		_, _, _, rels, tier := extractArkTS(src, "Index.ets")
		if tier != 1 {
			t.Fatalf("ArkTS fixture tier=%d, want TS-backed tier 1", tier)
		}
		requireCallReceiver(t, rels, "load", "this.service")
		requireCallReceiver(t, rels, "Text", "")
	})
}
