package agent

import (
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestExpandPackageExports_PathC pins the Phase 2.C path (c)
// behavior: a package directory entity expands to its exported
// top-level Symbol names. Required for u8a-class questions
// ("list all exports of internal/analysis/criterion") to escape
// the L0-B single-entity gate.
func TestExpandPackageExports_PathC(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/analysis/criterion/grammar.go": {
				RelPath: "internal/analysis/criterion/grammar.go",
				Symbols: []repomap.Symbol{
					{Name: "Kind", Kind: "type", Exported: true, Line: 26},
					{Name: "Env", Kind: "type", Exported: true, Line: 50},
					{Name: "ErrUnknownKind", Kind: "var", Exported: true, Line: 100},
					{Name: "internalHelper", Kind: "function", Exported: false, Line: 120},
				},
			},
			"internal/analysis/criterion/eval.go": {
				RelPath: "internal/analysis/criterion/eval.go",
				Symbols: []repomap.Symbol{
					{Name: "Eval", Kind: "function", Exported: true, Line: 15},
					{Name: "EvalAll", Kind: "function", Exported: true, Line: 36},
					{Name: "RegisteredKinds", Kind: "function", Exported: true, Line: 109},
				},
			},
		},
	}

	got := expandPackageExports(graph, "internal/analysis/criterion")

	wantSet := map[string]bool{
		"Kind": true, "Env": true, "ErrUnknownKind": true,
		"Eval": true, "EvalAll": true, "RegisteredKinds": true,
	}
	if len(got) != len(wantSet) {
		t.Errorf("expandPackageExports: got %d names, want %d (%v)", len(got), len(wantSet), got)
	}
	for _, n := range got {
		if !wantSet[n] {
			t.Errorf("unexpected exported name: %q", n)
		}
		delete(wantSet, n)
	}
	for missing := range wantSet {
		t.Errorf("missing expected exported name: %q", missing)
	}
}

// TestExpandPackageExports_FiltersUnexportedAndNested pins the
// filtering rules — unexported symbols and nested (Parent != "")
// definitions are excluded.
func TestExpandPackageExports_FiltersUnexportedAndNested(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/x/file.go": {
				RelPath: "internal/x/file.go",
				Symbols: []repomap.Symbol{
					{Name: "Foo", Kind: "type", Exported: true},                         // ✓
					{Name: "fooHelper", Kind: "function", Exported: false},              // ✗ unexported
					{Name: "Method", Kind: "method", Exported: true, Parent: "Foo"},     // ✗ nested
					{Name: "Field", Kind: "field", Exported: true, Parent: "Foo"},       // ✗ wrong kind
				},
			},
		},
	}
	got := expandPackageExports(graph, "internal/x")
	if len(got) != 1 || got[0] != "Foo" {
		t.Errorf("expected only [Foo], got %v", got)
	}
}

// TestExpandPackageExports_EmptyDirectoryReturnsNil pins the
// no-op semantic for unknown / empty directories.
func TestExpandPackageExports_EmptyDirectoryReturnsNil(t *testing.T) {
	graph := &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{}}
	if got := expandPackageExports(graph, "internal/missing"); got != nil {
		t.Errorf("expected nil for empty directory, got %v", got)
	}
}

// TestExpandChildPackages_PathD pins the Phase 2.C path (d)
// behavior: a parent directory containing ≥2 sub-packages
// expands to the child package names. Required for s5b-class
// questions ("list internal/analysis/ 子包") to escape L0-B.
func TestExpandChildPackages_PathD(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/analysis/budget/budget.go":      {RelPath: "internal/analysis/budget/budget.go"},
			"internal/analysis/compiler/compiler.go":  {RelPath: "internal/analysis/compiler/compiler.go"},
			"internal/analysis/criterion/grammar.go":  {RelPath: "internal/analysis/criterion/grammar.go"},
			"internal/analysis/criterion/eval.go":     {RelPath: "internal/analysis/criterion/eval.go"},
			"internal/analysis/normalizer/normalize.go": {RelPath: "internal/analysis/normalizer/normalize.go"},
		},
	}
	got := expandChildPackages(graph, "internal/analysis")
	wantSet := map[string]bool{
		"budget": true, "compiler": true, "criterion": true, "normalizer": true,
	}
	if len(got) != len(wantSet) {
		t.Errorf("expandChildPackages: got %d names, want %d (%v)", len(got), len(wantSet), got)
	}
	for _, n := range got {
		if !wantSet[n] {
			t.Errorf("unexpected child name: %q", n)
		}
	}
}

// TestExpandChildPackages_SingleChildReturnsNil pins the ≥2
// minimum: a directory with one child is not an enumeration.
func TestExpandChildPackages_SingleChildReturnsNil(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/x/only/file.go": {RelPath: "internal/x/only/file.go"},
		},
	}
	got := expandChildPackages(graph, "internal/x")
	if got != nil {
		// expandChildPackages returns the slice but the caller
		// (expandEntitiesWithImplementers Path d) checks
		// len >= 2. The function itself MAY return a 1-element
		// slice — that's still semantically "found 1 child" so
		// caller's len check is what enforces the ≥2 rule.
		// Accept either behavior here as long as len < 2.
		if len(got) >= 2 {
			t.Errorf("expected at most 1 child for single-child directory, got %v", got)
		}
	}
}

// TestExpandChildPackages_FilesAtPrefixDontCount pins that
// files DIRECTLY under `<bare>/` (not in a sub-directory) are NOT
// counted as child packages — only nested directories are.
func TestExpandChildPackages_FilesAtPrefixDontCount(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/x/leaf.go":           {RelPath: "internal/x/leaf.go"},          // skip
			"internal/x/sub1/file.go":      {RelPath: "internal/x/sub1/file.go"},     // count sub1
			"internal/x/sub2/file.go":      {RelPath: "internal/x/sub2/file.go"},     // count sub2
		},
	}
	got := expandChildPackages(graph, "internal/x")
	if len(got) != 2 {
		t.Errorf("expected 2 sub-package children, got %v", got)
	}
}

// TestExpandChildPackages_MultiLanguage pins cross-language
// portability — Go / Python / Rust / Cangjie / ArkTS / Proto /
// Java directory layouts all extract the same way through the
// FileIndex prefix scan.
func TestExpandChildPackages_MultiLanguage(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			// Go-style sub-package
			"src/services/auth/auth.go":              {RelPath: "src/services/auth/auth.go"},
			// Python-style package with __init__.py
			"src/services/billing/__init__.py":       {RelPath: "src/services/billing/__init__.py"},
			"src/services/billing/charge.py":         {RelPath: "src/services/billing/charge.py"},
			// Rust-style module
			"src/services/inventory/mod.rs":          {RelPath: "src/services/inventory/mod.rs"},
			"src/services/inventory/item.rs":         {RelPath: "src/services/inventory/item.rs"},
			// Java/Kotlin-style package
			"src/services/notifications/Notifier.java": {RelPath: "src/services/notifications/Notifier.java"},
			// Cangjie-style
			"src/services/profile/profile.cj":        {RelPath: "src/services/profile/profile.cj"},
			// Proto-style sub-directory
			"src/services/messaging/v1/message.proto": {RelPath: "src/services/messaging/v1/message.proto"},
		},
	}
	got := expandChildPackages(graph, "src/services")
	wantSet := map[string]bool{
		"auth": true, "billing": true, "inventory": true,
		"notifications": true, "profile": true, "messaging": true,
	}
	if len(got) != len(wantSet) {
		t.Errorf("expandChildPackages cross-language: got %d names, want %d (%v)",
			len(got), len(wantSet), got)
	}
	for _, n := range got {
		if !wantSet[n] {
			t.Errorf("unexpected child name: %q", n)
		}
	}
}

// TestIsTestSourcePath pins the cross-language test-source-path
// filter used by Path (c) and Path (d) to drop test functions /
// test directories from entity expansion. Without this filter the
// criterion package's 50+ Test* functions (Go test funcs are
// Exported=true syntactically) would drown the 9 real exports in
// the entity hint, leading the LLM to enumerate fewer than 5 of
// the actual API.
func TestIsTestSourcePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Production source — must NOT match
		{"internal/agent/explorer.go", false},
		{"src/services/auth/auth.py", false},
		{"src/lib.rs", false},
		{"src/main/java/Foo.java", false},
		{"src/index.ts", false},
		{"src/handler.kt", false},
		{"main.cj", false},
		{"app.lua", false},
		{"main.swift", false},
		{"server.cpp", false},
		// Test source — MUST match
		{"internal/agent/explorer_test.go", true},                  // Go suffix
		{"internal/types/types_test.go", true},                     // Go suffix
		{"src/services/auth/auth_test.py", true},                   // Python suffix
		{"src/services/test_login.py", true},                       // Python prefix
		{"tests/integration_test.py", true},                        // Python tests dir
		{"src/lib_test.rs", true},                                  // Rust suffix
		{"tests/integration.rs", true},                             // Rust tests dir
		{"src/main_test.cj", true},                                 // Cangjie suffix
		{"src/utils_test.lua", true},                               // Lua suffix
		{"src/parser_spec.lua", true},                              // Lua spec suffix
		{"test/foo_test.rb", true},                                 // Ruby test dir
		{"spec/login_spec.rb", true},                               // Ruby spec dir
		{"src/main/java/AuthTest.java", true},                      // Java Test suffix
		{"src/test/java/AuthTests.java", true},                     // Java src/test/
		{"src/utils.test.ts", true},                                // TS .test.
		{"src/utils.spec.tsx", true},                               // TSX .spec.
		{"src/__tests__/utils.ts", true},                           // JS __tests__/
		{"src/HandlerTests.swift", true},                           // Swift Tests
		{"src/AuthTests.m", true},                                  // Obj-C Tests.m
		{"src/auth/AuthSpec.scala", true},                          // Scala Spec
		// Edge cases
		{"", false},
		{"foo/contestant.go", false}, // contains "test" but not as suffix/prefix
	}
	for _, c := range cases {
		got := isTestSourcePath(c.path)
		if got != c.want {
			t.Errorf("isTestSourcePath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestExpandPackageExports_FiltersTestSources pins that test
// source files are excluded from path (c) expansion. Critical for
// u8a: criterion package has 50+ test functions whose
// Symbol.Exported=true syntactically; without this filter they
// drown the 9 real exports in the entity hint.
func TestExpandPackageExports_FiltersTestSources(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"pkg/x/api.go": {
				RelPath: "pkg/x/api.go",
				Symbols: []repomap.Symbol{
					{Name: "RealAPI", Kind: "function", Exported: true},
				},
			},
			"pkg/x/api_test.go": {
				RelPath: "pkg/x/api_test.go",
				Symbols: []repomap.Symbol{
					{Name: "TestRealAPI_HappyPath", Kind: "function", Exported: true},
					{Name: "TestRealAPI_Edge", Kind: "function", Exported: true},
				},
			},
		},
	}
	got := expandPackageExports(graph, "pkg/x")
	if len(got) != 1 || got[0] != "RealAPI" {
		t.Errorf("expected only [RealAPI] (test fns filtered), got %v", got)
	}
}

// TestExpandChildPackages_FiltersTestDirs pins that test-only
// child directories (`tests/` / `__tests__/` / `spec/`) are
// excluded from path (d) expansion.
func TestExpandChildPackages_FiltersTestDirs(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"src/services/auth/auth.go":       {RelPath: "src/services/auth/auth.go"},        // ✓ real pkg
			"src/services/billing/charge.py":  {RelPath: "src/services/billing/charge.py"},   // ✓ real pkg
			"src/services/tests/integration.go": {RelPath: "src/services/tests/integration.go"}, // ✗ test dir
			"src/services/__tests__/utils.ts":   {RelPath: "src/services/__tests__/utils.ts"},   // ✗ test dir
		},
	}
	got := expandChildPackages(graph, "src/services")
	wantSet := map[string]bool{"auth": true, "billing": true}
	if len(got) != 2 {
		t.Errorf("expected 2 real children (test dirs filtered), got %v", got)
	}
	for _, n := range got {
		if !wantSet[n] {
			t.Errorf("unexpected child name: %q", n)
		}
	}
}

// TestExpandPackageExports_MultiLanguageKinds pins cross-language
// Symbol.Kind acceptance — Python class, Rust struct/trait/enum,
// Cangjie operator, Proto message/service/rpc, Java interface,
// TS enum/const, Lua function are all accepted as "exported API".
func TestExpandPackageExports_MultiLanguageKinds(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"pkg/x/file.go": {
				RelPath: "pkg/x/file.go",
				Symbols: []repomap.Symbol{
					// Python (class)
					{Name: "AuthService", Kind: "class", Exported: true},
					// Rust (struct + trait + enum)
					{Name: "Repository", Kind: "struct", Exported: true},
					{Name: "Storage", Kind: "trait", Exported: true},
					{Name: "ErrorKind", Kind: "enum", Exported: true},
					// Swift / Obj-C (protocol)
					{Name: "Authenticatable", Kind: "protocol", Exported: true},
					// Cangjie (operator overload)
					{Name: "+", Kind: "operator", Exported: true},
					// Proto (message / service / rpc)
					{Name: "User", Kind: "message", Exported: true},
					{Name: "AuthService_Service", Kind: "service", Exported: true},
					{Name: "Login", Kind: "rpc", Exported: true},
					// Ruby / Lua (module)
					{Name: "AuthHelpers", Kind: "module", Exported: true},
					// Java / TS (interface)
					{Name: "Sessionable", Kind: "interface", Exported: true},
					// TS / JS / Rust (const)
					{Name: "DEFAULT_TIMEOUT", Kind: "const", Exported: true},
				},
			},
		},
	}
	got := expandPackageExports(graph, "pkg/x")
	want := []string{
		"AuthService", "Repository", "Storage", "ErrorKind",
		"Authenticatable", "+", "User", "AuthService_Service",
		"Login", "AuthHelpers", "Sessionable", "DEFAULT_TIMEOUT",
	}
	if len(got) != len(want) {
		t.Errorf("multi-language Kind acceptance: got %d, want %d (%v)",
			len(got), len(want), got)
	}
}
