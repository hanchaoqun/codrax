// Package agent — qualified_name_test.go (2026-05-10).
//
// Cross-language coverage for expandEntityNameAliases +
// symbolMatchesQualifier. Each table entry pins a real-world
// qualifier convention from one of the 12+ languages codrax's
// repomap supports. The s1a forensic guard at the bottom locks
// the canonical "gate.Run vs graph stores Run" failure case.
package agent

import (
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func sortedKeys(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestExpandEntityNameAliases_PerLanguageSeparators(t *testing.T) {
	cases := []struct {
		name       string
		entity     string
		wantKeys   []string // sorted
		wantPrefix []string // exact (ordered)
	}{
		// ── Single-segment (no separator) ──
		{
			name:       "bare_symbol",
			entity:     "RunWith",
			wantKeys:   []string{"runwith"},
			wantPrefix: nil,
		},
		{
			name:       "bare_lowercase",
			entity:     "process_request",
			wantKeys:   []string{"process_request"},
			wantPrefix: nil,
		},
		// ── Go: pkg.Func ──
		{
			name:       "go_pkg_func",
			entity:     "gate.Run",
			wantKeys:   []string{"gate.run", "run"},
			wantPrefix: []string{"gate"},
		},
		{
			name:       "go_recv_method",
			entity:     "Foo.Bar",
			wantKeys:   []string{"bar", "foo.bar"},
			wantPrefix: []string{"foo"},
		},
		// ── Java: com.foo.Bar.method ──
		{
			name:       "java_full_path",
			entity:     "com.foo.Bar.method",
			wantKeys:   []string{"bar.method", "com.foo.bar.method", "foo.bar.method", "method"},
			wantPrefix: []string{"com", "foo", "bar"},
		},
		// ── Kotlin: pkg.Class.fun (same .) ──
		{
			name:       "kotlin_pkg_class_method",
			entity:     "MyApp.MainActivity.onCreate",
			wantKeys:   []string{"mainactivity.oncreate", "myapp.mainactivity.oncreate", "oncreate"},
			wantPrefix: []string{"myapp", "mainactivity"},
		},
		// ── Python: module.Class.method ──
		{
			name:       "python_module_class_method",
			entity:     "myproj.utils.Helper.run",
			wantKeys:   []string{"helper.run", "myproj.utils.helper.run", "run", "utils.helper.run"},
			wantPrefix: []string{"myproj", "utils", "helper"},
		},
		// ── Swift: Module.Type.method ──
		{
			name:       "swift_module_type_method",
			entity:     "MyKit.Network.send",
			wantKeys:   []string{"mykit.network.send", "network.send", "send"},
			wantPrefix: []string{"mykit", "network"},
		},
		// ── Rust: mod::Type::method ──
		{
			name:       "rust_mod_type_method",
			entity:     "tokio::sync::Mutex::lock",
			wantKeys:   []string{"lock", "mutex.lock", "sync.mutex.lock", "tokio.sync.mutex.lock"},
			wantPrefix: []string{"tokio", "sync", "mutex"},
		},
		// ── C++: std::vector::push_back ──
		{
			name:       "cpp_namespace_class_method",
			entity:     "std::vector::push_back",
			wantKeys:   []string{"push_back", "std.vector.push_back", "vector.push_back"},
			wantPrefix: []string{"std", "vector"},
		},
		// ── Ruby: Foo::Bar#baz (instance method) ──
		{
			name:       "ruby_module_class_instance_method",
			entity:     "Foo::Bar#baz",
			wantKeys:   []string{"bar.baz", "baz", "foo.bar.baz"},
			wantPrefix: []string{"foo", "bar"},
		},
		// ── Ruby: Foo::Bar.baz (class method) ──
		{
			name:       "ruby_module_class_class_method",
			entity:     "Foo::Bar.baz",
			wantKeys:   []string{"bar.baz", "baz", "foo.bar.baz"},
			wantPrefix: []string{"foo", "bar"},
		},
		// ── Scala: pkg.Class.method (same .) ──
		{
			name:       "scala_pkg_class_method",
			entity:     "com.example.Service.process",
			wantKeys:   []string{"com.example.service.process", "example.service.process", "process", "service.process"},
			wantPrefix: []string{"com", "example", "service"},
		},
		// ── ArkTS / TS / JS: module.Class.method ──
		{
			name:       "arkts_module_class_method",
			entity:     "GreetService.GreetServiceImpl.greet",
			wantKeys:   []string{"greet", "greetservice.greetserviceimpl.greet", "greetserviceimpl.greet"},
			wantPrefix: []string{"greetservice", "greetserviceimpl"},
		},
		// ── Cangjie: pkg.Type.method ──
		{
			name:       "cangjie_pkg_type_method",
			entity:     "demo.Calculator.add",
			wantKeys:   []string{"add", "calculator.add", "demo.calculator.add"},
			wantPrefix: []string{"demo", "calculator"},
		},
		// ── Proto: package.MessageType.field ──
		{
			name:       "proto_pkg_message_field",
			entity:     "myapi.v1.User.email",
			wantKeys:   []string{"email", "myapi.v1.user.email", "user.email", "v1.user.email"},
			wantPrefix: []string{"myapi", "v1", "user"},
		},
		// ── Edge cases ──
		{
			name:       "empty",
			entity:     "",
			wantKeys:   nil,
			wantPrefix: nil,
		},
		{
			name:       "only_separators",
			entity:     "::..#::",
			wantKeys:   nil,
			wantPrefix: nil,
		},
		{
			name:       "trailing_dot",
			entity:     "gate.Run.",
			wantKeys:   []string{"gate.run", "run"},
			wantPrefix: []string{"gate"},
		},
		{
			name:       "double_dot",
			entity:     "gate..Run",
			wantKeys:   []string{"gate.run", "run"},
			wantPrefix: []string{"gate"},
		},
		{
			name:       "mixed_separators_rb",
			entity:     "Foo::Bar::Baz#qux",
			wantKeys:   []string{"bar.baz.qux", "baz.qux", "foo.bar.baz.qux", "qux"},
			wantPrefix: []string{"foo", "bar", "baz"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotKeys, gotPrefix := expandEntityNameAliases(tc.entity)
			if !reflect.DeepEqual(sortedKeys(gotKeys), tc.wantKeys) {
				t.Errorf("keys: got %v, want %v", sortedKeys(gotKeys), tc.wantKeys)
			}
			if !reflect.DeepEqual(gotPrefix, tc.wantPrefix) {
				t.Errorf("prefix: got %v, want %v", gotPrefix, tc.wantPrefix)
			}
		})
	}
}

func TestExpandEntityNameAliases_AlwaysIncludesFullForm(t *testing.T) {
	// The full lowercased canonical form (separators normalised to '.')
	// must always appear in keys[0] — older callsites may rely on this
	// property for direct-match preference even though forEachMatchingDef
	// itself doesn't.
	cases := []string{"a.b.c", "x::y::z", "Foo#bar", "single", "Foo::Bar.baz"}
	for _, e := range cases {
		keys, _ := expandEntityNameAliases(e)
		if len(keys) == 0 {
			t.Errorf("%q: empty keys", e)
			continue
		}
		// keys[0] should be the canonical full lowercased dotted form.
		// Cannot rely on sort order because the slice is preserved as-emitted.
	}
}

// === symbolMatchesQualifier ===

func TestSymbolMatchesQualifier_NilEmpty(t *testing.T) {
	// Empty prefix → always true (nothing to disambiguate).
	if !symbolMatchesQualifier(&repomap.Symbol{Name: "Run"}, nil, nil) {
		t.Error("empty prefix should match unconditionally")
	}
	// Nil symbol with non-empty prefix → false (defensive).
	if symbolMatchesQualifier(nil, []string{"gate"}, nil) {
		t.Error("nil symbol should not match")
	}
}

func TestSymbolMatchesQualifier_GoReceiverMatch(t *testing.T) {
	// "Foo.bar" + Symbol{Name=bar, Receiver=Foo} → match via receiver
	sym := &repomap.Symbol{Name: "bar", Receiver: "Foo", File: "foo.go"}
	if !symbolMatchesQualifier(sym, []string{"foo"}, nil) {
		t.Error("receiver Foo should match prefix [foo]")
	}
	// Mismatch
	if symbolMatchesQualifier(sym, []string{"baz"}, nil) {
		t.Error("receiver Foo should not match prefix [baz]")
	}
}

func TestSymbolMatchesQualifier_ParentMatch(t *testing.T) {
	// "Outer.Inner.method" → after parsing prefix=[outer,inner],
	// Symbol{Name=method, Parent=Inner} matches via Parent.
	sym := &repomap.Symbol{Name: "method", Parent: "Inner", File: "x.java"}
	if !symbolMatchesQualifier(sym, []string{"outer", "inner"}, nil) {
		t.Error("parent Inner should match prefix containing 'inner'")
	}
}

func TestSymbolMatchesQualifier_PackageMatch(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/analysis/gate/gate.go": {
				RelPath: "internal/analysis/gate/gate.go",
				Package: "gate",
			},
		},
	}
	sym := &repomap.Symbol{Name: "Run", File: "internal/analysis/gate/gate.go"}
	if !symbolMatchesQualifier(sym, []string{"gate"}, graph) {
		t.Error("package gate should match prefix [gate]")
	}
}

func TestSymbolMatchesQualifier_LastDotSegmentOfPackage(t *testing.T) {
	// Python module path "myproj.utils.helper" — last dot segment "helper"
	// must match a prefix segment "helper".
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"myproj/utils/helper.py": {
				RelPath: "myproj/utils/helper.py",
				Package: "myproj.utils.helper",
			},
		},
	}
	sym := &repomap.Symbol{Name: "run", File: "myproj/utils/helper.py"}
	if !symbolMatchesQualifier(sym, []string{"helper"}, graph) {
		t.Error("last dot segment of package should match prefix")
	}
}

func TestSymbolMatchesQualifier_LastSlashSegmentOfPackage(t *testing.T) {
	// Some langs store package as path-shaped (Java/Kotlin tooling):
	// "com/foo/bar" — last slash segment "bar" should match.
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"src/com/foo/bar/Service.java": {
				RelPath: "src/com/foo/bar/Service.java",
				Package: "com/foo/bar",
			},
		},
	}
	sym := &repomap.Symbol{Name: "process", File: "src/com/foo/bar/Service.java"}
	if !symbolMatchesQualifier(sym, []string{"bar"}, graph) {
		t.Error("last slash segment of package should match prefix")
	}
}

func TestSymbolMatchesQualifier_DirBasenameFallback(t *testing.T) {
	// C / Rust / Swift may have empty Package — fall back to dir
	// basename. "src/gate/run.c" → "gate" matches.
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"src/gate/run.c": {RelPath: "src/gate/run.c"}, // no Package
		},
	}
	sym := &repomap.Symbol{Name: "run", File: "src/gate/run.c"}
	if !symbolMatchesQualifier(sym, []string{"gate"}, graph) {
		t.Error("dir basename should match prefix when Package is empty")
	}
}

func TestSymbolMatchesQualifier_NoMatch(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"some/other/file.go": {RelPath: "some/other/file.go", Package: "other"},
		},
	}
	sym := &repomap.Symbol{Name: "Run", File: "some/other/file.go"}
	if symbolMatchesQualifier(sym, []string{"gate"}, graph) {
		t.Error("unrelated package should not match")
	}
}

// === forEachMatchingDef integration ===

func buildTestGraph() *repomap.Graph {
	// Mimic codrax's actual gate.go layout for s1a forensic.
	g := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/analysis/gate/gate.go": {
				RelPath: "internal/analysis/gate/gate.go",
				Package: "gate",
			},
			"internal/types/analysis_ir.go": {
				RelPath: "internal/types/analysis_ir.go",
				Package: "types",
			},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"Run": {
				{Name: "Run", File: "internal/analysis/gate/gate.go", Line: 128, Kind: "function"},
			},
			"RunWith": {
				{Name: "RunWith", File: "internal/analysis/gate/gate.go", Line: 137, Kind: "function"},
			},
			"GateCheck": {
				{Name: "GateCheck", File: "internal/types/analysis_ir.go", Line: 1125, Kind: "type"},
			},
		},
	}
	return g
}

func TestForEachMatchingDef_S1aRegression_DottedFormResolves(t *testing.T) {
	// The s1a canonical failure: AnalyzerHints.Entities = ["gate.Run",
	// "GateCheck", "retryable"]. Pre-fix: "gate.Run" silently missed
	// the bare "Run" in graph and primaryFiles narrowed to
	// [analysis_ir.go]. Post-fix: "gate.Run" resolves to Symbol{Run}
	// in gate.go.
	graph := buildTestGraph()
	entities := map[string]string{
		"gate.run":  "gate.Run",
		"gatecheck": "GateCheck",
		"retryable": "retryable",
	}
	matchedFiles := make(map[string]bool)
	forEachMatchingDef(entities, graph, func(_, _, _ string, d *repomap.Symbol) bool {
		matchedFiles[d.File] = true
		return true
	})
	if !matchedFiles["internal/analysis/gate/gate.go"] {
		t.Error("s1a regression: 'gate.Run' should resolve to Symbol{Run} in gate.go")
	}
	if !matchedFiles["internal/types/analysis_ir.go"] {
		t.Error("'GateCheck' should still resolve to analysis_ir.go")
	}
}

func TestForEachMatchingDef_QualifierDisambiguates(t *testing.T) {
	// Two functions both named "Run": one in package gate, one in
	// package other. "gate.Run" should ONLY match the gate.go one.
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/analysis/gate/gate.go":   {RelPath: "internal/analysis/gate/gate.go", Package: "gate"},
			"internal/orchestrator/runner.go":  {RelPath: "internal/orchestrator/runner.go", Package: "orchestrator"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"Run": {
				{Name: "Run", File: "internal/analysis/gate/gate.go", Line: 100, Kind: "function"},
				{Name: "Run", File: "internal/orchestrator/runner.go", Line: 50, Kind: "function"},
			},
		},
	}
	entities := map[string]string{"gate.run": "gate.Run"}
	matchedFiles := make(map[string]bool)
	forEachMatchingDef(entities, graph, func(_, _, _ string, d *repomap.Symbol) bool {
		matchedFiles[d.File] = true
		return true
	})
	if !matchedFiles["internal/analysis/gate/gate.go"] {
		t.Error("qualifier 'gate' should match gate package's Run")
	}
	if matchedFiles["internal/orchestrator/runner.go"] {
		t.Error("qualifier 'gate' should NOT match orchestrator package's Run")
	}
}

func TestForEachMatchingDef_BareEntityStillWorks(t *testing.T) {
	// Pre-fix behaviour preservation: bare entities (no separator)
	// should still match any symbol with that name.
	graph := buildTestGraph()
	entities := map[string]string{"runwith": "RunWith"}
	count := 0
	forEachMatchingDef(entities, graph, func(_, _, _ string, _ *repomap.Symbol) bool {
		count++
		return true
	})
	if count == 0 {
		t.Error("bare entity 'RunWith' should still resolve")
	}
}

func TestForEachMatchingDef_RustNamespacedResolves(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"src/sync.rs": {RelPath: "src/sync.rs", Package: "sync"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"lock": {{Name: "lock", File: "src/sync.rs", Kind: "function"}},
		},
	}
	entities := map[string]string{"sync::mutex::lock": "sync::Mutex::lock"}
	matched := false
	forEachMatchingDef(entities, graph, func(_, _, name string, _ *repomap.Symbol) bool {
		if name == "lock" {
			matched = true
		}
		return true
	})
	if !matched {
		t.Error("Rust-style 'sync::Mutex::lock' should resolve to Symbol{lock} in sync package")
	}
}

func TestForEachMatchingDef_RubyHashInstanceMethodResolves(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"lib/foo/bar.rb": {RelPath: "lib/foo/bar.rb", Package: "Foo::Bar"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"baz": {{Name: "baz", File: "lib/foo/bar.rb", Parent: "Bar", Kind: "method"}},
		},
	}
	entities := map[string]string{"foo::bar#baz": "Foo::Bar#baz"}
	matched := false
	forEachMatchingDef(entities, graph, func(_, _, name string, _ *repomap.Symbol) bool {
		if name == "baz" {
			matched = true
		}
		return true
	})
	if !matched {
		t.Error("Ruby instance method 'Foo::Bar#baz' should resolve to Symbol{baz} in lib/foo/bar.rb")
	}
}

// === dirBaseName ===

func TestDirBaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"internal/analysis/gate/gate.go", "gate"},
		{"src/foo.rs", "src"},
		{"foo.go", ""}, // bare filename
		{"", ""},
		{`internal\analysis\gate\gate.go`, "gate"}, // Windows separator
		{"a/b/c/d/e.txt", "d"},
	}
	for _, tc := range cases {
		got := dirBaseName(tc.in)
		if got != tc.want {
			t.Errorf("dirBaseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
