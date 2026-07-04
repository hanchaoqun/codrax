package tracequery

// §7.11 B-5 pins (docs/design/customer_dead_session_audit_20260703.md):
// the converter-synthetic lane-name vocabulary (db2systrace.py machine
// tokens, emitted byte-identically by codrax's own trace-db exporter in
// internal/hitraceconv/streamerdb_export_extended.go) is soft classification
// only — display/suggestion labels via traceSpanCategory. Near-miss
// human-authored lookalikes must not match, and no hard gate may consume the
// vocabulary (structural pin below).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConverterSyntheticLaneClassB5(t *testing.T) {
	hits := []struct {
		name string
		want string
	}{
		{"FrameActual-1917295", "frame_pacing"},    // db2systrace.py:543-549
		{"FrameExpected-1917295", "frame_pacing"},  // db2systrace.py:543-549
		{"sys_64", "syscall"},                      // db2systrace.py:649-654
		{"TaskPool-7", "task_pool"},                // db2systrace.py:679-685
		{"AppStartup:OnForeground", "app_startup"}, // db2systrace.py:700-705
		{"SoInit:libace.so", "so_init"},            // db2systrace.py:719-724
		// Converter NULL-identity fallbacks are verbatim machine tokens too
		// (hitraceconv/streamerdb_export_extended.go:471 and :535,
		// traceDBAnyText(…, "None")); TaskPool's fallback is "0" (:579) and
		// rides the digit shape ("TaskPool-7" above covers it).
		{"FrameActual-None", "frame_pacing"},
		{"FrameExpected-None", "frame_pacing"},
		{"sys_None", "syscall"},
	}
	for _, tc := range hits {
		class, ok := converterSyntheticLaneClass(tc.name)
		if !ok || class != tc.want {
			t.Fatalf("converterSyntheticLaneClass(%q) = %q ok=%v, want %q", tc.name, class, ok, tc.want)
		}
	}
	misses := []string{
		// numeric-identity forms require a non-empty all-digit suffix or the
		// verbatim converter fallback token "None"
		"FrameActual-", "FrameActual-abc", "FrameActual-12a",
		"TaskPool-", "TaskPool-Manager", "TaskPool-12a",
		"sys_", "sys_read", "sys_64x",
		// the "None" fallback is exact-case verbatim, and TaskPool never
		// emits it (its fallback is "0", streamerdb_export_extended.go:579)
		"FrameActual-none", "sys_NONE", "sys_None1", "TaskPool-None",
		// exact case is part of the machine shape
		"frameactual-123", "taskpool-7", "appstartup:x", "soinit:lib.so", "SYS_64",
		// prefixes are anchored at the start of the name
		"H:AppStartup:x", "MySoInit:lib.so", "XFrameActual-1",
		// colon forms require the colon
		"AppStartup", "SoInit",
		"",
	}
	for _, name := range misses {
		if class, ok := converterSyntheticLaneClass(name); ok {
			t.Fatalf("converterSyntheticLaneClass(%q) matched %q; near-miss strings must not be caught", name, class)
		}
	}
}

func TestTraceSpanCategoryConverterLanesB5(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"FrameActual-1917295", "frame_pacing"},
		{"FrameExpected-1917295", "frame_pacing"},
		{"sys_64", "syscall"},
		{"TaskPool-7", "task_pool"},
		{"AppStartup:OnForeground", "app_startup"},
		// Before B-5 these fell into the generic Contains chain and were
		// mislabeled by accidental substring hits: "libaudio.so" read "audio"
		// (the audio case runs early in the chain) and "libfileshare.so"
		// read "file_io". The machine token now wins for both.
		{"SoInit:libaudio.so", "so_init"},
		{"SoInit:libfileshare.so", "so_init"},
		// Converter NULL-identity fallback tokens ride the same lane
		// (hitraceconv/streamerdb_export_extended.go:471/:535).
		{"FrameActual-None", "frame_pacing"},
		{"FrameExpected-None", "frame_pacing"},
		{"sys_None", "syscall"},
	}
	for _, tc := range cases {
		if got := traceSpanCategory(tc.name); got != tc.want {
			t.Fatalf("traceSpanCategory(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
	// Subcategory surface is untouched by the vocabulary: frame identity
	// subcategories keep coming from the existing generic chain.
	if got := traceSpanSubcategory("FrameActual-1917295"); got != "actual" {
		t.Fatalf("traceSpanSubcategory(FrameActual-…) = %q, want actual", got)
	}
	if got := traceSpanSubcategory("FrameExpected-1917295"); got != "expected" {
		t.Fatalf("traceSpanSubcategory(FrameExpected-…) = %q, want expected", got)
	}
	// Ranking surface untouched: none of the machine forms may classify as
	// semantic work (root-cause candidate lane).
	for _, name := range []string{"FrameActual-1917295", "FrameExpected-1917295", "FrameActual-None", "FrameExpected-None", "sys_64", "sys_None", "TaskPool-7", "AppStartup:OnForeground", "SoInit:libace.so"} {
		if _, ok := traceSpanSemanticWorkClass(name); ok {
			t.Fatalf("traceSpanSemanticWorkClass(%q) matched; converter lane names must stay out of the ranking vocabulary", name)
		}
	}
	// Human-authored near-miss keeps its existing generic category.
	if got := traceSpanCategory("TaskPool-Manager"); got == "task_pool" {
		t.Fatalf("near-miss TaskPool-Manager must not take the machine label")
	}
}

// TestConverterSyntheticLaneVocabConsumersPinnedB5 is the structural half of
// the §7.11 B-5 red line: the vocabulary must only feed soft classification.
// It ast.Inspects EVERY declaration of every non-test file in this package —
// not just FuncDecl bodies — and asserts the ONLY function referencing
// converterSyntheticLaneClass is traceSpanCategory, with ZERO references
// outside function declarations. That closes the three bypass shapes a
// body-only walk would miss: (1) package-level GenDecl tables
// (`var m = map[...]...{...: converterSyntheticLaneClass}`), (2) function
// values captured in var initializers (`var f = converterSyntheticLaneClass`),
// and (3) closures declared inside GenDecl initializers. The self-check
// below constructively proves the scanner catches all three. Adding a new
// consumer fails this test on purpose: re-verify the consumer is a soft
// display/suggestion surface (never a hard gate, filter predicate on result
// admission, candidate promotion, or score) before extending the allowlist.
func TestConverterSyntheticLaneVocabConsumersPinnedB5(t *testing.T) {
	const vocabIdent = "converterSyntheticLaneClass"
	allowed := map[string]bool{
		vocabIdent:          true, // its own declaration
		"traceSpanCategory": true, // soft classification consumer
	}
	fset := token.NewFileSet()
	// Scanner self-check against the three known bypass shapes.
	bypass, err := parser.ParseFile(fset, "b5_bypass_shapes.go", `package tracequery

var laneTable = map[string]func(string) (string, bool){"t": converterSyntheticLaneClass}

var laneFn = converterSyntheticLaneClass

var laneClosure = func(n string) bool { _, ok := converterSyntheticLaneClass(n); return ok }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, outside := scanVocabConsumersB5(fset, bypass, vocabIdent); len(outside) != 3 {
		t.Fatalf("scanner self-check failed: want 3 outside-FuncDecl hits (GenDecl table, function value, closure), got %d: %v", len(outside), outside)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fileSeen, outside := scanVocabConsumersB5(fset, file, vocabIdent)
		if len(outside) > 0 {
			t.Fatalf("%s is referenced outside any function declaration (%s); §7.11 B-5 red line forbids package-level tables / function values / closures over the vocabulary", vocabIdent, strings.Join(outside, ", "))
		}
		for fn := range fileSeen {
			seen[fn] = true
		}
	}
	if !seen["traceSpanCategory"] {
		t.Fatalf("traceSpanCategory no longer consumes %s; update the B-5 pin alongside the refactor", vocabIdent)
	}
	for fn := range seen {
		if !allowed[fn] {
			t.Fatalf("%s is consumed by %s; §7.11 B-5 red line allows soft classification consumers only (allowlist: traceSpanCategory). Verify the new consumer is display/suggestion-only before extending the pin.", vocabIdent, fn)
		}
	}
}

// scanVocabConsumersB5 walks every node of file. A vocabIdent reference
// inside a FuncDecl (anywhere in it — body, nested closure, result
// expression) is attributed to that function's name; a reference anywhere
// else (GenDecl tables, var-initializer function values, closures inside
// GenDecl initializers) is returned as an outside hit with its position.
// The declaring FuncDecl's own Name ident is not a consumption.
func scanVocabConsumersB5(fset *token.FileSet, file *ast.File, vocabIdent string) (seen map[string]bool, outside []string) {
	seen = map[string]bool{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			ast.Inspect(fn, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok || ident.Name != vocabIdent || ident == fn.Name {
					return true
				}
				seen[fn.Name.Name] = true
				return true
			})
			continue
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == vocabIdent {
				outside = append(outside, fset.Position(ident.Pos()).String())
			}
			return true
		})
	}
	return seen, outside
}
