package hitraceconv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// trace_validation_fold_in4_test.go — fold-in round four (colleague_merge_audit
// §40.38 四轮收编):
//   - P: the witness excerpt bound is the PAIR (64 bytes of body before
//     escaping, at most 4× = 256 bytes after escaping) — pinned exactly,
//     including the worst case that reaches 256;
//   - O (hitraceconv half): AllTraceEventInvalidKinds is the closed kind set
//     the cmd advertisement census binds to — every TraceEventInvalidKind
//     constant declared in this package is a member (go/ast).

func TestTraceEventInvalidWitnessExcerptBoundIsBodyBytesThenFourTimesEscaped(t *testing.T) {
	if maxTraceEventInvalidWitnessBodyBytes != 64 || maxTraceEventInvalidWitnessExcerptBytes != 256 {
		t.Fatalf("bound pair = (%d body, %d escaped), want (64, 256)", maxTraceEventInvalidWitnessBodyBytes, maxTraceEventInvalidWitnessExcerptBytes)
	}
	cases := map[string]string{
		"ascii_at_budget":                strings.Repeat("a", 200),
		"all_invalid_bytes":              strings.Repeat("\xff", 200),
		"continuation_run_beyond_budget": strings.Repeat("\x80", 200),
		"mixed_invalid_and_ascii":        strings.Repeat("a\xff", 100),
		"cjk":                            strings.Repeat("中", 100),
		"emoji":                          strings.Repeat("😀", 60),
	}
	for name, body := range cases {
		got := traceEventInvalidWitnessExcerpt(body)
		if !utf8.ValidString(got) || got == "" {
			t.Fatalf("%s: excerpt empty or invalid UTF-8: %q", name, got)
		}
		if len(got) > maxTraceEventInvalidWitnessExcerptBytes {
			t.Fatalf("%s: escaped excerpt is %d bytes, over the %d-byte escaped bound", name, len(got), maxTraceEventInvalidWitnessExcerptBytes)
		}
		// Bytes of BODY represented (each `\xNN` stands for one body byte)
		// never exceed the 64-byte body budget.
		represented := len(got) - 3*strings.Count(got, `\x`)
		if represented > maxTraceEventInvalidWitnessBodyBytes {
			t.Fatalf("%s: excerpt represents %d body bytes, over the %d-byte body budget: %q", name, represented, maxTraceEventInvalidWitnessBodyBytes, got)
		}
	}
	// The worst case reaches the escaped bound exactly: 64 invalid bytes →
	// 64 × `\xNN` = 256 bytes.
	if got := traceEventInvalidWitnessExcerpt(strings.Repeat("\xff", 200)); len(got) != maxTraceEventInvalidWitnessExcerptBytes ||
		got != strings.Repeat(`\xff`, maxTraceEventInvalidWitnessBodyBytes) {
		t.Fatalf("64 invalid bytes must escape to exactly %d bytes: %d %q", maxTraceEventInvalidWitnessExcerptBytes, len(got), got)
	}
	// Valid bodies never exceed the body budget after escaping (no escapes).
	if got := traceEventInvalidWitnessExcerpt(strings.Repeat("a", 200)); len(got) != maxTraceEventInvalidWitnessBodyBytes {
		t.Fatalf("a valid body cuts at the %d-byte body budget: %d", maxTraceEventInvalidWitnessBodyBytes, len(got))
	}
}

func TestAllTraceEventInvalidKindsIsTheDeclaredClosedSet(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "trace_validation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if ident, ok := vs.Type.(*ast.Ident); !ok || ident.Name != "TraceEventInvalidKind" {
				continue
			}
			for i := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					if value, err := strconv.Unquote(lit.Value); err == nil {
						declared[value] = true
					}
				}
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("no TraceEventInvalidKind constants declared")
	}
	members := map[string]bool{}
	for _, kind := range AllTraceEventInvalidKinds() {
		if members[string(kind)] {
			t.Fatalf("AllTraceEventInvalidKinds repeats %q", kind)
		}
		members[string(kind)] = true
	}
	for value := range declared {
		if !members[value] {
			t.Errorf("declared kind %q is missing from AllTraceEventInvalidKinds", value)
		}
	}
	for value := range members {
		if !declared[value] {
			t.Errorf("AllTraceEventInvalidKinds names %q, which is not a declared constant", value)
		}
	}
	if !members[string(TraceEventInvalidTraceDBRecordSequenceForeignRow)] {
		t.Fatal("the foreign-row split must be a member")
	}
}
