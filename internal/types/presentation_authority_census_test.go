package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// presentation_authority_census_test.go — V7-4 structural tripwire
// (colleague_merge_audit §40.54; census discipline per §40.36/§40.50: total
// over shapes, fail loud on an unrecognized shape, self-red per shape).
//
// Claim: the emit_analysis hard-diagram gate and the analyzer prompt read the
// SAME field through the SAME accessor. Concretely —
//
//	R1  The raw boolean `PresentationDiagramRequired` is READ in production
//	    code only inside the two `PresentationAuthority()` accessors and the
//	    two field mirrors (ToolBusContext / SubAgentContext); every other
//	    occurrence is a struct field declaration or a registered copy key
//	    whose right-hand side is pinned. Any other shape fails loud.
//	R2  No LLM-facing package carries a non-logger, non-tag string literal
//	    naming the classifier wire key (`requires_diagram`) or the retired
//	    "out-of-band" framing that taught the model to infer the hard
//	    requirement from prose.
//
// Scope: root-level *.go, internal/**, cmd/** (non-test files). internal/repl
// owns the wire schema and is the intended home of `requires_diagram`; it is
// outside R2 by the LLM-facing package list below, not by an exclusion.

const presentationCensusIdent = "PresentationDiagramRequired"

// presentationCensusReaders are the only (file :: func) sites that may read
// the boolean through a selector outside a registered mirror.
var presentationCensusReaders = map[string]bool{
	"internal/types/presentation_authority.go::BusContext.PresentationAuthority":   true,
	"internal/types/presentation_authority.go::AgentContext.PresentationAuthority": true,
}

// presentationCensusMirrors are the field-mirror sites: a composite key
// `PresentationDiagramRequired: <x>.PresentationDiagramRequired`.
var presentationCensusMirrors = map[string]bool{
	"internal/types/bus_context_projection.go::ToolBusContext":  true,
	"internal/types/bus_context_projection.go::SubAgentContext": true,
}

// presentationCensusCopyKeys are composite keys whose value is NOT a mirror
// selector; the exact right-hand side is pinned so the agent view can only be
// filled from the accessor and the orchestrator only from its typed setter.
var presentationCensusCopyKeys = map[string]string{
	"internal/context/builder.go::BuildAgentContext":          "presentationAuthority.DiagramRequired",
	"internal/orchestrator/orchestrator.go::Orchestrator.Run": "presentationDiagramRequired",
}

// presentationCensusFieldDecls are the two struct declarations.
var presentationCensusFieldDecls = map[string]bool{
	"internal/types/context.go::BusContext":   true,
	"internal/types/context.go::AgentContext": true,
}

// presentationCensusLLMFacingDirs are the packages whose string literals may
// reach a model (prompt builders, tools, skills, agents, orchestrator hints,
// and the types package that carries shared teaching).
var presentationCensusLLMFacingDirs = []string{
	"internal/agent", "internal/context", "internal/orchestrator",
	"internal/skill", "internal/tool", "internal/types",
}

var presentationCensusBannedLiterals = []string{"requires_diagram", "out-of-band"}

// presentationCensusBlocklistRegistry is the one variable whose string
// elements may spell a banned token: the glossary blocklist
// (internal/skill/glossary.go :: InternalTermsBlocklist). Batch-six merge
// ruling (§40.52 × §40.54): the wire key is deliberately NOT a glossary entry
// — internal/repl is under the §40.52 prompt-surface scan and, as the wire
// owner, must spell its own emit_turn_policy schema property, and §40.52
// forbids term allow-lists — so R2 over the LLM-facing packages is enforced
// by THIS census (presentationCensusBannedLiterals) and the direct negative
// pins in the skill/builder/emit_analysis tests, not by the glossary lints.
// The registry exemption stays as a recognized structural shape.
const presentationCensusBlocklistRegistry = "InternalTermsBlocklist"

func TestPresentationAuthorityCensus_SingleReaderAndNoWireLiterals(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s has no go.mod: %v", root, err)
	}
	files := presentationCensusListFiles(t, root)
	if len(files) == 0 {
		t.Fatal("census found no production files")
	}
	fset := token.NewFileSet()
	var findings []string
	seenReaders := map[string]bool{}
	seenMirrors := map[string]bool{}
	seenCopyKeys := map[string]bool{}
	seenDecls := map[string]bool{}
	for _, rel := range files {
		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		res := censusPresentationAuthorityFile(fset, file, rel)
		findings = append(findings, res.findings...)
		for k := range res.readers {
			seenReaders[k] = true
		}
		for k := range res.mirrors {
			seenMirrors[k] = true
		}
		for k := range res.copyKeys {
			seenCopyKeys[k] = true
		}
		for k := range res.decls {
			seenDecls[k] = true
		}
	}
	// Every registered site must still exist: a registry entry that no
	// longer matches code is stale review surface.
	for site := range presentationCensusReaders {
		if !seenReaders[site] {
			findings = append(findings, "registered reader no longer present: "+site)
		}
	}
	for site := range presentationCensusMirrors {
		if !seenMirrors[site] {
			findings = append(findings, "registered mirror no longer present: "+site)
		}
	}
	for site := range presentationCensusCopyKeys {
		if !seenCopyKeys[site] {
			findings = append(findings, "registered copy key no longer present: "+site)
		}
	}
	for site := range presentationCensusFieldDecls {
		if !seenDecls[site] {
			findings = append(findings, "registered field declaration no longer present: "+site)
		}
	}
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Fatalf("presentation authority census (V7-4 §40.54) found %d finding(s):\n  %s",
			len(findings), strings.Join(findings, "\n  "))
	}
}

// presentationCensusListFiles returns non-test .go files (slash-relative to
// root) from the root level, internal/, and cmd/.
func presentationCensusListFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	add := func(path string) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, filepath.ToSlash(rel))
	}
	rootEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range rootEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			add(filepath.Join(root, e.Name()))
		}
	}
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				add(path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	sort.Strings(files)
	return files
}

type presentationCensusResult struct {
	findings []string
	readers  map[string]bool
	mirrors  map[string]bool
	copyKeys map[string]bool
	decls    map[string]bool
}

// censusPresentationAuthorityFile classifies every occurrence of the boolean
// identifier and every banned literal in one file. It is pure over its
// inputs so the self-red subtests can feed synthetic sources.
func censusPresentationAuthorityFile(fset *token.FileSet, file *ast.File, rel string) presentationCensusResult {
	res := presentationCensusResult{
		readers:  map[string]bool{},
		mirrors:  map[string]bool{},
		copyKeys: map[string]bool{},
		decls:    map[string]bool{},
	}
	pos := func(n ast.Node) string {
		p := fset.Position(n.Pos())
		return rel + ":" + strconv.Itoa(p.Line)
	}
	// recognized: identifier nodes already classified by an enclosing shape.
	recognized := map[*ast.Ident]bool{}
	// enclosing tracks the current FuncDecl / TypeSpec name for site keys.
	var enclosing string
	site := func() string { return rel + "::" + enclosing }

	isIdent := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && id.Name == presentationCensusIdent
	}
	isSelector := func(e ast.Expr) (*ast.SelectorExpr, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		return sel, ok && sel.Sel.Name == presentationCensusIdent
	}

	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			name := x.Name.Name
			if x.Recv != nil && len(x.Recv.List) == 1 {
				name = presentationCensusRecvName(x.Recv.List[0].Type) + "." + name
			}
			saved := enclosing
			enclosing = name
			if x.Recv != nil {
				ast.Inspect(x.Recv, visit)
			}
			ast.Inspect(x.Type, visit)
			if x.Body != nil {
				ast.Inspect(x.Body, visit)
			}
			enclosing = saved
			return false
		case *ast.TypeSpec:
			saved := enclosing
			enclosing = x.Name.Name
			ast.Inspect(x.Type, visit)
			enclosing = saved
			return false
		case *ast.Field:
			for _, name := range x.Names {
				if name.Name == presentationCensusIdent {
					recognized[name] = true
					if !presentationCensusFieldDecls[site()] {
						res.findings = append(res.findings, pos(name)+": unregistered field declaration of "+presentationCensusIdent+" in "+site())
					} else {
						res.decls[site()] = true
					}
				}
			}
			return true
		case *ast.KeyValueExpr:
			if isIdent(x.Key) {
				recognized[x.Key.(*ast.Ident)] = true
				if sel, ok := isSelector(x.Value); ok {
					recognized[sel.Sel] = true
					if presentationCensusMirrors[site()] {
						res.mirrors[site()] = true
					} else {
						res.findings = append(res.findings, pos(x)+": unregistered field mirror of "+presentationCensusIdent+" in "+site())
					}
					return true
				}
				want, ok := presentationCensusCopyKeys[site()]
				got := presentationCensusExprString(x.Value)
				switch {
				case !ok:
					res.findings = append(res.findings, pos(x)+": unregistered copy key of "+presentationCensusIdent+" in "+site()+" (value "+got+")")
				case got != want:
					res.findings = append(res.findings, pos(x)+": copy key of "+presentationCensusIdent+" in "+site()+" has value "+got+", pinned "+want)
				default:
					res.copyKeys[site()] = true
				}
				return true
			}
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if sel, ok := isSelector(lhs); ok {
					recognized[sel.Sel] = true
					res.findings = append(res.findings, pos(sel)+": selector write to "+presentationCensusIdent+" in "+site()+" (no production write site is registered)")
				}
			}
		case *ast.SelectorExpr:
			if x.Sel.Name == presentationCensusIdent && !recognized[x.Sel] {
				recognized[x.Sel] = true
				if presentationCensusReaders[site()] {
					res.readers[site()] = true
				} else {
					res.findings = append(res.findings, pos(x)+": reader of "+presentationCensusIdent+" outside the accessor in "+site())
				}
			}
		case *ast.Ident:
			if x.Name == presentationCensusIdent && !recognized[x] {
				recognized[x] = true
				res.findings = append(res.findings, pos(x)+": unrecognized shape for "+presentationCensusIdent+" in "+site()+" (bare identifier)")
			}
		}
		return true
	}
	ast.Inspect(file, visit)

	// R2 — banned literals outside struct tags, imports, and logger arguments.
	if !presentationCensusLLMFacing(rel) {
		return res
	}
	excluded := presentationCensusExcludedLiterals(file)
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || excluded[lit.Pos()] {
			return true
		}
		raw, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		for _, banned := range presentationCensusBannedLiterals {
			if strings.Contains(raw, banned) {
				res.findings = append(res.findings, pos(lit)+": string literal names "+strconv.Quote(banned)+" in an LLM-facing package")
			}
		}
		return true
	})
	return res
}

func presentationCensusLLMFacing(rel string) bool {
	for _, dir := range presentationCensusLLMFacingDirs {
		if strings.HasPrefix(rel, dir+"/") {
			return true
		}
	}
	return false
}

// presentationCensusExcludedLiterals collects positions of struct tags,
// import paths, and direct string arguments of logger / print calls.
func presentationCensusExcludedLiterals(file *ast.File) map[token.Pos]bool {
	excluded := map[token.Pos]bool{}
	for _, imp := range file.Imports {
		excluded[imp.Path.Pos()] = true
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			// The glossary blocklist registry itself names the banned tokens
			// so the three prompt lints can enforce them; it is the one
			// non-LLM-facing literal home of a banned token.
			for i, name := range x.Names {
				if name.Name != presentationCensusBlocklistRegistry || i >= len(x.Values) {
					continue
				}
				if lit, ok := x.Values[i].(*ast.CompositeLit); ok {
					for _, elt := range lit.Elts {
						if s, ok := elt.(*ast.BasicLit); ok {
							excluded[s.Pos()] = true
						}
					}
				}
			}
		case *ast.Field:
			if x.Tag != nil {
				excluded[x.Tag.Pos()] = true
			}
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			logger := pkg.Name == "logging" || pkg.Name == "log" ||
				(pkg.Name == "fmt" && (strings.HasPrefix(sel.Sel.Name, "Print") || strings.HasPrefix(sel.Sel.Name, "Fprint")))
			if !logger {
				return true
			}
			for _, arg := range x.Args {
				if lit, ok := arg.(*ast.BasicLit); ok {
					excluded[lit.Pos()] = true
				}
			}
		}
		return true
	})
	return excluded
}

func presentationCensusRecvName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.StarExpr:
		return presentationCensusRecvName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return presentationCensusRecvName(x.X)
	case *ast.IndexListExpr:
		return presentationCensusRecvName(x.X)
	}
	return "?"
}

func presentationCensusExprString(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return presentationCensusExprString(x.X) + "." + x.Sel.Name
	case *ast.CallExpr:
		return presentationCensusExprString(x.Fun) + "(...)"
	case *ast.StarExpr:
		return "*" + presentationCensusExprString(x.X)
	case *ast.UnaryExpr:
		return x.Op.String() + presentationCensusExprString(x.X)
	case *ast.BasicLit:
		return x.Value
	}
	return "<unrecognized expression>"
}

// TestPresentationAuthorityCensus_SelfRed feeds one synthetic source per
// shape so every lane of the classifier is proven to fire (or to stay
// silent) independently of the live tree.
func TestPresentationAuthorityCensus_SelfRed(t *testing.T) {
	cases := []struct {
		name string
		rel  string
		src  string
		want string // substring of exactly one finding; "" = must be clean
	}{
		{
			name: "selector read outside accessor",
			rel:  "internal/tool/x.go",
			src:  "package tool\nfunc gate(c *C) bool { return c.PresentationDiagramRequired }\n",
			want: "reader of PresentationDiagramRequired outside the accessor in internal/tool/x.go::gate",
		},
		{
			name: "registered accessor read is clean",
			rel:  "internal/types/presentation_authority.go",
			src:  "package types\nfunc (c *BusContext) PresentationAuthority() bool { return c.PresentationDiagramRequired }\n",
		},
		{
			name: "bare identifier is an unrecognized shape",
			rel:  "internal/tool/x.go",
			src:  "package tool\nfunc f() { _ = PresentationDiagramRequired }\n",
			want: "unrecognized shape",
		},
		{
			name: "selector write has no registered site",
			rel:  "internal/context/x.go",
			src:  "package context\nfunc f(a *A) { a.PresentationDiagramRequired = true }\n",
			want: "selector write to PresentationDiagramRequired",
		},
		{
			name: "mirror in unregistered function",
			rel:  "internal/context/x.go",
			src:  "package context\nfunc f(b *B) *A { return &A{PresentationDiagramRequired: b.PresentationDiagramRequired} }\n",
			want: "unregistered field mirror",
		},
		{
			name: "registered mirror is clean",
			rel:  "internal/types/bus_context_projection.go",
			src:  "package types\nfunc ToolBusContext(ctx *A) *B { return &B{PresentationDiagramRequired: ctx.PresentationDiagramRequired} }\n",
		},
		{
			name: "copy key in unregistered site",
			rel:  "internal/context/x.go",
			src:  "package context\nfunc f() *A { return &A{PresentationDiagramRequired: true} }\n",
			want: "unregistered copy key",
		},
		{
			name: "copy key with unpinned value (prose inference)",
			rel:  "internal/context/builder.go",
			src:  "package context\nfunc BuildAgentContext(d string) *A { return &A{PresentationDiagramRequired: strings.Contains(d, \"diagram\")} }\n",
			want: "has value strings.Contains(...), pinned presentationAuthority.DiagramRequired",
		},
		{
			name: "registered copy key with pinned value is clean",
			rel:  "internal/context/builder.go",
			src:  "package context\nfunc BuildAgentContext(p P) *A { return &A{PresentationDiagramRequired: presentationAuthority.DiagramRequired} }\n",
		},
		{
			name: "field declaration outside the two structs",
			rel:  "internal/tool/x.go",
			src:  "package tool\ntype Other struct { PresentationDiagramRequired bool }\n",
			want: "unregistered field declaration",
		},
		{
			name: "registered field declaration is clean",
			rel:  "internal/types/context.go",
			src:  "package types\ntype BusContext struct { PresentationDiagramRequired bool `json:\"presentation_diagram_required,omitempty\"` }\n",
		},
		{
			name: "wire key literal in LLM-facing package",
			rel:  "internal/tool/x.go",
			src:  "package tool\nvar s = \"confirmed by the requires_diagram signal\"\n",
			want: "string literal names \"requires_diagram\"",
		},
		{
			name: "retired framing literal in LLM-facing package",
			rel:  "internal/skill/x.go",
			src:  "package skill\nvar s = \"out-of-band typed directive\"\n",
			want: "string literal names \"out-of-band\"",
		},
		{
			name: "logger argument literal is excluded",
			rel:  "internal/tool/x.go",
			src:  "package tool\nfunc f() { logging.Warning(\"requires_diagram=%t\", true) }\n",
		},
		{
			name: "struct tag literal is excluded",
			rel:  "internal/skill/x.go",
			src:  "package skill\ntype F struct { R bool `json:\"requires_diagram,omitempty\"` }\n",
		},
		{
			name: "glossary blocklist registry element is excluded",
			rel:  "internal/skill/glossary.go",
			src:  "package skill\nvar InternalTermsBlocklist = []string{\"requires_diagram\"}\n",
		},
		{
			name: "banned token in any other registry is a finding",
			rel:  "internal/skill/glossary.go",
			src:  "package skill\nvar otherList = []string{\"requires_diagram\"}\n",
			want: "string literal names \"requires_diagram\"",
		},
		{
			name: "literal outside LLM-facing packages is out of scope",
			rel:  "internal/repl/x.go",
			src:  "package repl\nvar s = \"requires_diagram\"\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, c.rel, c.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse synthetic source: %v", err)
			}
			res := censusPresentationAuthorityFile(fset, file, c.rel)
			if c.want == "" {
				if len(res.findings) != 0 {
					t.Fatalf("expected clean, got %v", res.findings)
				}
				return
			}
			if len(res.findings) != 1 || !strings.Contains(res.findings[0], c.want) {
				t.Fatalf("expected exactly one finding containing %q, got %v", c.want, res.findings)
			}
		})
	}
}
