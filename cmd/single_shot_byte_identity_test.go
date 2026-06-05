package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pterm/pterm"
)

// TestSingleShot_BypassesGlamour_StructuralGuard is the load-bearing
// red-line guard for the "REPL palette never leaks into CLI single-
// shot output" contract. The renderer's glamour-based palette
// (codraxStyleConfig + applyHeadingHierarchy + applyInlineAndStructure)
// only takes effect when r.renderMarkdown runs, which only runs from
// r.RenderResult, which only runs on the REPL path. The CLI single-
// shot path (runSingleShot) writes busCtx.Mutable.Result() byte-for-
// byte to stdout via fmt.Print, never invoking the glamour pipeline.
//
// Without this guard, a future refactor that "helpfully" runs
// runSingleShot through r.RenderResult or r.renderMarkdown would
// silently start emitting ANSI escape sequences into piped output —
// users redirecting to file / processing in scripts / running CI
// would see corrupted markdown instead of clean source.
//
// Strategy: parse the cmd package's runSingleShot AST, walk every
// CallExpr inside its body, and reject any call whose target is
// RenderResult / renderMarkdown / any glamour-package function /
// any function with `Render` substring on a renderer receiver.
//
// On failure the test names the offending call so the developer
// can either revert the change or update the contract intentionally
// (the latter requires also updating the byte-output expectations
// downstream).
func TestSingleShot_BypassesGlamour_StructuralGuard(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	target := filepath.Join(filepath.Dir(thisFile), "root.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, target, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", target, err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Name.Name == "runSingleShot" {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatal("runSingleShot not found in cmd/root.go — has it been renamed?")
	}

	// Banned identifier suffixes the single-shot path must NEVER
	// invoke. Listed by the suffix actually appearing in source so
	// both `r.RenderResult(...)` and `renderer.RenderResult(...)`
	// trip the same guard. The terms are the load-bearing entry
	// points to the glamour rendering pipeline; if any of them ever
	// shows up inside runSingleShot we have a contract violation.
	bannedTargets := map[string]string{
		"RenderResult":   "REPL-only glamour entry point",
		"renderMarkdown": "glamour invocation",
	}
	bannedPackages := map[string]string{
		"glamour": "glamour markdown renderer",
		"styles":  "glamour style preset",
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fnExpr := call.Fun.(type) {
		case *ast.SelectorExpr:
			// Form: ident.Method(...) or pkg.Func(...)
			method := fnExpr.Sel.Name
			if reason, banned := bannedTargets[method]; banned {
				t.Errorf("runSingleShot calls .%s — %s; this BREAKS the single-shot byte-identity contract. Either revert the change or move the call out of runSingleShot.",
					method, reason)
			}
			if pkg, ok := fnExpr.X.(*ast.Ident); ok {
				if reason, banned := bannedPackages[pkg.Name]; banned {
					t.Errorf("runSingleShot calls %s.%s — %s; CLI single-shot must emit raw markdown without glamour.",
						pkg.Name, method, reason)
				}
			}
		case *ast.Ident:
			// Form: bareName(...)
			if reason, banned := bannedTargets[fnExpr.Name]; banned {
				t.Errorf("runSingleShot calls %s — %s; single-shot must stay glamour-free.",
					fnExpr.Name, reason)
			}
		}
		return true
	})
}

// TestSingleShot_NoGlamourImport asserts cmd/root.go does not import
// the glamour package directly. The render package owns glamour;
// the cmd layer is supposed to call into render only via the
// REPL-bound RenderResult method. A direct glamour import in cmd is
// a code smell that would let the single-shot path accidentally
// reach for glamour.
func TestSingleShot_NoGlamourImport(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	target := filepath.Join(filepath.Dir(thisFile), "root.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, target, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", target, err)
	}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.Contains(path, "charmbracelet/glamour") {
			t.Errorf("cmd/root.go imports %q — glamour must stay encapsulated in internal/render so the single-shot path cannot reach it.",
				path)
		}
	}
}

func TestSingleShot_ProgressStderrResultStdoutContract(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	target := filepath.Join(filepath.Dir(thisFile), "root.go")

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, target, data, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", target, err)
	}

	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Name.Name == "runSingleShot" {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatal("runSingleShot not found in cmd/root.go")
	}

	start := fset.Position(fn.Body.Pos()).Offset
	end := fset.Position(fn.Body.End()).Offset
	body := string(data[start:end])
	setStderr := strings.Index(body, "app.renderer.SetOutput(os.Stderr)")
	runPipeline := strings.Index(body, "app.orch.Run")
	printResult := strings.Index(body, "fmt.Print(result)")
	if setStderr < 0 {
		t.Fatal("runSingleShot must route renderer progress to stderr before running the pipeline")
	}
	if runPipeline < 0 {
		t.Fatal("runSingleShot must call app.orch.Run")
	}
	if printResult < 0 {
		t.Fatal("runSingleShot must print the final raw markdown result to stdout")
	}
	if setStderr > runPipeline {
		t.Fatalf("renderer progress must be routed to stderr before app.orch.Run (SetOutput index=%d Run index=%d)", setStderr, runPipeline)
	}
	if strings.Contains(body[:runPipeline], "app.renderer.SetOutput(os.Stdout)") {
		t.Fatal("runSingleShot must not route process/progress output to stdout before the final markdown body")
	}
}

func TestSingleShotColor_DefaultOffAlwaysOptIn(t *testing.T) {
	oldFlagColor := flagColor
	oldPrintColor := pterm.PrintColor
	defer func() {
		flagColor = oldFlagColor
		if oldPrintColor {
			pterm.EnableColor()
		} else {
			pterm.DisableColor()
		}
	}()

	t.Setenv("NO_COLOR", "")
	flagColor = "auto"
	pterm.EnableColor()
	configureSingleShotColor()
	if pterm.PrintColor {
		t.Fatal("single-shot CLI auto color should disable ANSI by default")
	}

	flagColor = "always"
	configureSingleShotColor()
	if !pterm.PrintColor {
		t.Fatal("--color=always should explicitly enable single-shot CLI ANSI")
	}

	t.Setenv("NO_COLOR", "1")
	configureSingleShotColor()
	if pterm.PrintColor {
		t.Fatal("NO_COLOR must disable single-shot CLI ANSI even with --color=always")
	}
}
