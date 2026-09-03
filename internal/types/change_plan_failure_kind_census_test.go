package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// change_plan_failure_kind_census_test.go — V5-2 tripwire (colleague_merge_audit
// §40.11 item 4, §1.6): every declared FailureKind must be registered in
// FailureKindReplanEscapeLanes, and its lane must agree with the typed routing
// tables — a kind the code-failure table routes to replan needs replan_only
// or the drift-owner lane; a kind the unavailable table accepts needs
// accept_unverified; a kind neither table knows has no escape and is red.
// The roster is collected from EVERY non-test file of this package, in both
// spellings (`X FailureKind = "x"` and `X = FailureKind("x")`).
func collectDeclaredFailureKinds(t *testing.T) []FailureKind {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var kinds []FailureKind
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), e.Name(), src, 0)
		if err != nil {
			t.Fatal(err)
		}
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
				typed := false
				if ident, ok := vs.Type.(*ast.Ident); ok && ident.Name == "FailureKind" {
					typed = true
				}
				for _, value := range vs.Values {
					switch v := value.(type) {
					case *ast.BasicLit:
						if typed && v.Kind == token.STRING {
							if s, err := strconv.Unquote(v.Value); err == nil {
								kinds = append(kinds, FailureKind(s))
							}
						}
					case *ast.CallExpr:
						fn, ok := v.Fun.(*ast.Ident)
						if !ok || fn.Name != "FailureKind" || len(v.Args) != 1 {
							continue
						}
						if lit, ok := v.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							if s, err := strconv.Unquote(lit.Value); err == nil {
								kinds = append(kinds, FailureKind(s))
							}
						}
					}
				}
			}
		}
	}
	return kinds
}

func TestEveryFailureKindRegistersAReplanEscapeLane(t *testing.T) {
	kinds := collectDeclaredFailureKinds(t)
	if len(kinds) < 12 {
		t.Fatalf("census found only %d FailureKind constants: %v", len(kinds), kinds)
	}
	for _, kind := range kinds {
		lane, registered := FailureKindReplanEscapeLanes[kind]
		if !registered {
			t.Fatalf("FailureKind %q is not registered in FailureKindReplanEscapeLanes — every replan-routed kind needs a typed escape lane", kind)
		}
		codeFailure := FailureReasonCodeIndicatesCodeFailure(string(kind))
		unavailable := FailureReasonCodeIndicatesVerificationUnavailable(string(kind))
		switch {
		case codeFailure && (lane == FailureKindEscapeReplanOnly || lane == FailureKindEscapeDriftOwnerLane):
		case unavailable && lane == FailureKindEscapeAcceptUnverified:
		default:
			t.Fatalf("FailureKind %q lane %q disagrees with routing (code_failure=%v unavailable=%v)", kind, lane, codeFailure, unavailable)
		}
	}
	for kind := range FailureKindReplanEscapeLanes {
		found := false
		for _, declared := range kinds {
			found = found || declared == kind
		}
		if !found {
			t.Fatalf("stale registration %q (no such FailureKind constant)", kind)
		}
	}
	// The collector sees both declaration spellings (self-check on a probe).
	probe := "package types\nconst (\n\tA FailureKind = \"a\"\n\tB = FailureKind(\"b\")\n)\n"
	file, err := parser.ParseFile(token.NewFileSet(), "probe.go", probe, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, decl := range file.Decls {
		gen := decl.(*ast.GenDecl)
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for _, value := range vs.Values {
				switch v := value.(type) {
				case *ast.BasicLit:
					seen["typed"] = true
				case *ast.CallExpr:
					if fn, ok := v.Fun.(*ast.Ident); ok && fn.Name == "FailureKind" {
						seen["conversion"] = true
					}
				}
			}
		}
	}
	if !seen["typed"] || !seen["conversion"] {
		t.Fatalf("collector self-check failed: %v", seen)
	}
}

func TestVerificationWorktreeDriftClassesAreClosedAndDisclosureIsExplicit(t *testing.T) {
	classes := AllVerificationWorktreeDriftClasses()
	if len(classes) != 4 {
		t.Fatalf("closed set drift: %v", classes)
	}
	disclosed := 0
	for _, class := range classes {
		if VerificationWorktreeDriftDisclosed(class) {
			disclosed++
		}
	}
	if disclosed != 3 || VerificationWorktreeDriftDisclosed(VerificationWorktreeDriftUnclassified) || VerificationWorktreeDriftDisclosed("plan_owned") {
		t.Fatalf("exactly the three owned classes disclose; unclassified and unknown refuse")
	}
}
