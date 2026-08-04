package index

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

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

func TestCReceiverParameterBindingsStayInsideDeclaringFunction(t *testing.T) {
	src := []byte(`typedef struct Worker { void (*run)(void); } Worker;
typedef struct Other { void (*run)(void); } Other;
void parameter_call(Worker *worker) { worker->run(); }
void local_call(void) { Other *worker; worker->run(); }
`)
	root := parseSourceFor(t, types.LangC, string(src))
	_, _, _, rels := extractCCpp(root, src, "service.c", types.LangC)
	want := map[int]string{3: "Worker", 4: "worker"}
	for _, rel := range rels {
		if rel.Kind != "call" || rel.ToEP.Name != "run" {
			continue
		}
		if receiver, ok := want[rel.Line]; ok {
			if rel.ToEP.Receiver != receiver {
				t.Fatalf("line %d receiver=%q, want %q: %+v", rel.Line, rel.ToEP.Receiver, receiver, rel)
			}
			delete(want, rel.Line)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing scoped C receiver calls: %+v; relations=%+v", want, rels)
	}
}

func TestNavigationReceiverCensusIgnoresInitializerTypes(t *testing.T) {
	tests := []struct {
		name     string
		language string
		file     string
		source   string
		extract  func(*sitter.Node, []byte, string) []types.Relation
	}{
		{
			name: "kotlin", language: types.LangKotlin, file: "Service.kt",
			source: "class Noise\nfun run() {\n  val repo = Noise()\n  repo.load()\n}\n",
			extract: func(root *sitter.Node, src []byte, file string) []types.Relation {
				return extractNavigationCalls(root, src, file, "kotlin_ast_navigation_call")
			},
		},
		{
			name: "swift", language: types.LangSwift, file: "Service.swift",
			source: "class Noise {}\nfunc run() {\n  let repo = Noise()\n  repo.load()\n}\n",
			extract: func(root *sitter.Node, src []byte, file string) []types.Relation {
				return extractNavigationCalls(root, src, file, "swift_ast_navigation_call")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			root := parseSourceFor(t, tc.language, tc.source)
			rels := tc.extract(root, src, tc.file)
			requireCallReceiver(t, rels, "load", "repo")
		})
	}
}

func TestNavigationReceiverBindingsStayInsideLexicalScope(t *testing.T) {
	tests := []struct {
		name     string
		language string
		file     string
		source   string
		want     map[int]string
	}{
		{
			name: "kotlin", language: types.LangKotlin, file: "Service.kt",
			source: "class Worker { fun run() {} }\nclass Other { fun run() {} }\nfun typed(repo: Worker) { repo.run() }\nfun inferred() { val repo = Other(); repo.run() }\n",
			want:   map[int]string{3: "Worker", 4: "repo"},
		},
		{
			name: "swift", language: types.LangSwift, file: "Service.swift",
			source: "class Worker { func run() {} }\nclass Other { func run() {} }\nfunc typed(repo: Worker) { repo.run() }\nfunc inferred() { let repo = Other(); repo.run() }\n",
			want:   map[int]string{3: "Worker", 4: "repo"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			root := parseSourceFor(t, tc.language, tc.source)
			rels := extractNavigationCalls(root, src, tc.file, tc.name+"_ast_navigation_call")
			want := make(map[int]string, len(tc.want))
			for line, receiver := range tc.want {
				want[line] = receiver
			}
			for _, rel := range rels {
				if rel.Kind != "call" || rel.ToEP.Name != "run" {
					continue
				}
				if receiver, ok := want[rel.Line]; ok {
					if rel.ToEP.Receiver != receiver {
						t.Fatalf("line %d receiver=%q, want %q: %+v", rel.Line, rel.ToEP.Receiver, receiver, rel)
					}
					delete(want, rel.Line)
				}
			}
			if len(want) != 0 {
				t.Fatalf("missing scoped navigation calls: %+v; relations=%+v", want, rels)
			}
		})
	}
}

func TestStaticReceiverBindingsStayInsideLanguageLexicalScopes(t *testing.T) {
	tests := []struct {
		name     string
		language string
		file     string
		source   string
		extract  func([]byte, string) []types.Relation
		want     map[int]string
	}{
		{
			name: "typescript", language: types.LangTypeScript, file: "service.ts",
			source: "class Worker { run() {} }\nclass Other { run() {} }\nfunction typed(repo: Worker) { repo.run(); }\nfunction inferred() { const repo = new Other(); repo.run(); }\n",
			extract: func(src []byte, file string) []types.Relation {
				root := parseSourceFor(t, types.LangTypeScript, string(src))
				_, _, _, rels := extractJS(root, src, file, true)
				return rels
			},
			want: map[int]string{3: "Worker", 4: "repo"},
		},
		{
			name: "arkts", language: types.LangArkTS, file: "service.ets",
			source: "class Worker { run() {} }\nclass Other { run() {} }\nfunction typed(repo: Worker) { repo.run(); }\nfunction inferred() { const repo = new Other(); repo.run(); }\n",
			extract: func(src []byte, file string) []types.Relation {
				_, _, _, rels, _ := extractArkTS(src, file)
				return rels
			},
			want: map[int]string{3: "Worker", 4: "repo"},
		},
		{
			name: "rust", language: types.LangRust, file: "service.rs",
			source: "struct Worker {}\nstruct Other {}\nfn typed(repo: &Worker) { repo.run(); }\nfn inferred() { let repo = Other {}; repo.run(); }\n",
			extract: func(src []byte, file string) []types.Relation {
				root := parseSourceFor(t, types.LangRust, string(src))
				_, _, _, rels := extractRust(root, src, file)
				return rels
			},
			want: map[int]string{3: "Worker", 4: "repo"},
		},
		{
			name: "cangjie", language: types.LangCangjie, file: "service.cj",
			source: "package demo\nclass Worker {}\nclass Other {}\nfunc typed(repo: Worker): Unit { repo.run() }\nfunc inferred(): Unit { let repo = Other(); repo.run() }\n",
			extract: func(src []byte, file string) []types.Relation {
				_, _, _, rels, _ := extractCangjie(src, file)
				return rels
			},
			want: map[int]string{4: "Worker", 5: "repo"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rels := tc.extract([]byte(tc.source), tc.file)
			want := make(map[int]string, len(tc.want))
			for line, receiver := range tc.want {
				want[line] = receiver
			}
			for _, rel := range rels {
				if rel.Kind != "call" || rel.ToEP.Name != "run" {
					continue
				}
				if receiver, ok := want[rel.Line]; ok {
					if rel.ToEP.Receiver != receiver {
						t.Fatalf("line %d receiver=%q, want %q: %+v", rel.Line, rel.ToEP.Receiver, receiver, rel)
					}
					delete(want, rel.Line)
				}
			}
			if len(want) != 0 {
				t.Fatalf("missing scoped %s calls: %+v; relations=%+v", tc.language, want, rels)
			}
		})
	}
}

func TestCangjieNamedArgumentDoesNotCorruptParameterBinding(t *testing.T) {
	src := []byte(`package demo
class Width { func load(): Unit {} }
func submit(width: Width): Unit {}
func run(width: Width, payload: Width): Unit {
    submit(width: payload)
    width.load()
}`)
	_, _, _, rels, _ := extractCangjie(src, "service.cj")
	requireCallReceiver(t, rels, "load", "Width")
}

func TestCallReceiverExtractorCacheEpochFloors(t *testing.T) {
	// b50f49233 changed call/receiver output semantics in these shared
	// extractor lanes. Every affected persisted language domain must reject
	// pre-change warm caches, including languages sharing one implementation.
	floors := map[string]int{
		types.LangJava: 6, types.LangPython: 6,
		types.LangJavaScript: 5, types.LangTypeScript: 6, types.LangArkTS: 6,
		types.LangCangjie: 5, types.LangKotlin: 6, types.LangRuby: 4,
		types.LangSwift: 5, types.LangLua: 5, types.LangRust: 5,
		types.LangC: 4, types.LangCpp: 4,
	}
	for language, floor := range floors {
		if got := extractorVersions[language]; got < floor {
			t.Errorf("extractorVersions[%q]=%d, want >=%d after receiver/call semantic change", language, got, floor)
		}
	}
}

func TestLuaNoWhitespaceCallSugarKeepsASTCalleeIdentity(t *testing.T) {
	source := "function run()\n  printer\"hello\"\n  repo:load\"key\"\n  repo.save()\nend\n"
	src := []byte(source)
	root := parseSourceFor(t, types.LangLua, source)
	rels := luaExtractCalls(root, src, "service.lua")
	requireCallReceiver(t, rels, "printer", "")
	requireCallReceiver(t, rels, "load", "repo")
	requireCallReceiver(t, rels, "save", "repo")
	for _, rel := range rels {
		if strings.ContainsAny(rel.ToEP.Name, `"'`) {
			t.Fatalf("Lua argument bytes leaked into callee identity: %+v; tree=%s", rel, root.String())
		}
	}
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
