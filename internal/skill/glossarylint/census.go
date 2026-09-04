package glossarylint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// RendererRoster is the single table of packages whose non-logger
// strings can reach a model prompt, each with the lint lane its marker
// test runs. Packages enter the roster in one of two ways:
//
//   - by shape: ProducerPackages finds a producer shape in them (a
//     BuildInitialInstruction method, a Tool Parameters() method, a
//     composite literal of a model-facing carrier, a `…SystemPrompt`
//     const, a Role:"system" message literal) — the census fails when
//     such a package is missing here;
//   - by data flow: the package builds text that a shape-bearing
//     package forwards verbatim (Why says which carrier).
//
// TestRendererRosterTotality pins roster == packages carrying a marker
// test and producers ⊆ roster, so a new renderer package, a stale row,
// or a marker without a row all fail the build naming the package.
var RendererRoster = []RosterEntry{
	{Dir: "cmd", Lane: LanePromptSurface, Why: "memory summarizer prompt + tool schema; the rest of the package is operator-facing CLI text"},
	{Dir: "internal/agent", Lane: LanePackage, Why: "evaluator BuildInitialInstruction renderers, LoopSignal hints, StageOutput.Error retry text; dynamic twin = prompt_snapshot_test"},
	{Dir: "internal/analysis/amplifier", Lane: LanePackage, Why: "Amplification.Reason rows render into the analyzer retry directive"},
	{Dir: "internal/analysis/contract", Lane: LanePackage, Why: "Violation.Detail/Repair and SuspectedRoot.Reason feed the retry-hint composer"},
	{Dir: "internal/analysis/criterion", Lane: LanePackage, Why: "Result.Detail becomes contract Violation.Detail"},
	{Dir: "internal/analysis/hint", Lane: LanePackage, Why: "the retry-hint composer (Hint fields + Allowed hints) renders straight into the next dispatch"},
	{Dir: "internal/context", Lane: LanePackage, Why: "BuildPromptContext assembles every system/user section"},
	{Dir: "internal/env/recommend", Lane: LanePackage, Why: "environment-recommendation tool schema + system prompt"},
	{Dir: "internal/orchestrator", Lane: LanePackage | LanePromptSurface, Why: "reviewer system prompts + tool schemas (surface lane; const values are skipped by the static policy) and validator Violation/SuspectedRoot/RepairDirective text (package lane)"},
	{Dir: "internal/repl", Lane: LanePromptSurface, Why: "turn-policy / chit-chat / operation / data-lane prompts and schemas; the rest of the package is operator-facing REPL text"},
	{Dir: "internal/tool", Lane: LanePackage, Why: "tool Description/Parameters schemas and ToolResult.Summary rejection text; roster twin = TestNoInternalTermsInToolSchemas"},
	{Dir: "internal/tool/ground", Lane: LanePackage, Why: "CitationReport.Reason is copied into ToolResult.Summary"},
	{Dir: "internal/tool/repomap", Lane: LanePackage, Why: "repo_map tool Description/Parameters"},
}

// Lane is the bitset of marker calls a roster entry's test must make.
type Lane uint8

const (
	// LanePackage — the test calls glossarylint.RunPackageScan.
	LanePackage Lane = 1 << iota
	// LanePromptSurface — the test calls glossarylint.RunPromptSurfaceScan.
	LanePromptSurface
)

func (l Lane) String() string {
	var parts []string
	if l&LanePackage != 0 {
		parts = append(parts, "RunPackageScan")
	}
	if l&LanePromptSurface != 0 {
		parts = append(parts, "RunPromptSurfaceScan")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "+")
}

// RosterEntry is one row of RendererRoster.
type RosterEntry struct {
	Dir  string // module-relative package directory
	Lane Lane
	Why  string
}

// Producer is one producer shape found in one package.
type Producer struct {
	Dir   string
	Shape string // closed set, see producerShapes
	Where string // "<file>:<line>"
}

// carrierTypes are the composite-literal types whose string fields reach a
// model, keyed by import path then type name.
var carrierTypes = map[string]map[string]bool{
	"github.com/hanchaoqun/codrax/internal/types": {"Violation": true, "SuspectedRoot": true, "ToolResult": true, "RepairDirective": true, "LoopSignal": true},
	llmImportPath: {"ToolSchema": true},
	"github.com/hanchaoqun/codrax/internal/analysis/hint": {"Hint": true, "Allowed": true},
}

// CensusRoots are the directory trees a repo-wide walker scans (§40.50
// ④: root-level files, internal/ and cmd/ only).
var CensusRoots = []string{".", "internal", "cmd"}

// ProducerPackages walks the census roots under moduleRoot and returns
// every package that carries a producer shape:
//
//   - a method named BuildInitialInstruction;
//   - a method Parameters() returning json.RawMessage, or ParametersFor;
//   - a composite literal of a carrier type (selector form through the
//     file's import alias, or identifier form through a local
//     `type X = pkg.X` alias). Identifier-form literals in the type's
//     own defining package are constructors, not renderers, and are
//     skipped; an identifier-form literal of a carrier name that the
//     package neither aliases nor defines is an unrecognized shape and
//     returns an error;
//   - a package-level const/var named `…SystemPrompt`;
//   - a composite literal carrying `Role: "system"` (a provider system
//     message built in place).
func ProducerPackages(moduleRoot string) ([]Producer, error) {
	dirs, err := censusPackageDirs(moduleRoot)
	if err != nil {
		return nil, err
	}
	var out []Producer
	var problems []string
	for _, dir := range dirs {
		files, err := listGoFiles(filepath.Join(moduleRoot, dir), false)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			continue
		}
		fset := token.NewFileSet()
		parsed := make([]*ast.File, 0, len(files))
		for _, path := range files {
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil, fmt.Errorf("glossarylint: parse %s: %v", path, err)
			}
			parsed = append(parsed, file)
		}
		aliasOf, definesType := localTypeDecls(parsed)
		for _, file := range parsed {
			importPaths := map[string]string{} // local name → import path
			for _, imp := range file.Imports {
				if imp.Path == nil {
					continue
				}
				p, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					continue
				}
				local := p[strings.LastIndex(p, "/")+1:]
				if imp.Name != nil {
					local = imp.Name.Name
				}
				importPaths[local] = p
			}
			add := func(shape string, pos token.Pos) {
				p := fset.Position(pos)
				out = append(out, Producer{Dir: dir, Shape: shape, Where: filepath.ToSlash(p.Filename) + ":" + strconv.Itoa(p.Line)})
			}
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if d.Recv == nil {
						continue
					}
					switch d.Name.Name {
					case "BuildInitialInstruction":
						add("BuildInitialInstruction", d.Pos())
					case "ParametersFor":
						add("ToolParameters", d.Pos())
					case "Parameters":
						if d.Type.Results != nil && len(d.Type.Results.List) == 1 && isSelectorType(d.Type.Results.List[0].Type, "json", "RawMessage") {
							add("ToolParameters", d.Pos())
						}
					}
				case *ast.GenDecl:
					if d.Tok != token.CONST && d.Tok != token.VAR {
						continue
					}
					for _, spec := range d.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range vs.Names {
								if strings.HasSuffix(name.Name, "SystemPrompt") {
									add("PromptConst", name.Pos())
								}
							}
						}
					}
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if isSystemMessageLiteral(cl) {
					add("SystemMessage", cl.Pos())
				}
				switch t := cl.Type.(type) {
				case *ast.SelectorExpr:
					x, ok := t.X.(*ast.Ident)
					if !ok {
						return true
					}
					if names := carrierTypes[importPaths[x.Name]]; names[t.Sel.Name] {
						add("Carrier:"+t.Sel.Name, cl.Pos())
					}
				case *ast.Ident:
					if !isCarrierName(t.Name) {
						return true
					}
					if target, ok := aliasOf[t.Name]; ok {
						if names := carrierTypes[importPaths[target.pkg]]; names[target.name] {
							add("Carrier:"+target.name, cl.Pos())
						}
						return true
					}
					if definesType[t.Name] {
						return true // constructor inside the defining package
					}
					p := fset.Position(cl.Pos())
					problems = append(problems, fmt.Sprintf("%s:%d composite literal %s{} is neither a local alias nor a locally defined type — unrecognized carrier shape", filepath.ToSlash(p.Filename), p.Line, t.Name))
				}
				return true
			})
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, fmt.Errorf("glossarylint: %d unrecognized producer shape(s):\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return out, nil
}

func isCarrierName(name string) bool {
	for _, names := range carrierTypes {
		if names[name] {
			return true
		}
	}
	return false
}

type aliasTarget struct{ pkg, name string }

// localTypeDecls returns the package's `type X = pkg.Y` aliases and the
// set of type names the package defines itself (struct/interface/etc).
func localTypeDecls(files []*ast.File) (map[string]aliasTarget, map[string]bool) {
	aliases := map[string]aliasTarget{}
	defines := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if ts.Assign.IsValid() {
					if sel, ok := ts.Type.(*ast.SelectorExpr); ok {
						if x, ok := sel.X.(*ast.Ident); ok {
							aliases[ts.Name.Name] = aliasTarget{pkg: x.Name, name: sel.Sel.Name}
							continue
						}
					}
				}
				defines[ts.Name.Name] = true
			}
		}
	}
	return aliases, defines
}

// MarkerPackages returns, for every package under the census roots, the
// lanes whose marker call appears in one of its _test.go files.
func MarkerPackages(moduleRoot string) (map[string]Lane, error) {
	dirs, err := censusPackageDirs(moduleRoot)
	if err != nil {
		return nil, err
	}
	const selfPath = "github.com/hanchaoqun/codrax/internal/skill/glossarylint"
	out := map[string]Lane{}
	for _, dir := range dirs {
		files, err := listGoFiles(filepath.Join(moduleRoot, dir), true)
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return nil, fmt.Errorf("glossarylint: parse %s: %v", path, err)
			}
			alias := importAlias(file, selfPath)
			if alias == "" {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != alias {
					return true
				}
				switch sel.Sel.Name {
				case "RunPackageScan":
					out[dir] |= LanePackage
				case "RunPromptSurfaceScan":
					out[dir] |= LanePromptSurface
				}
				return true
			})
		}
	}
	return out, nil
}

// censusPackageDirs lists every directory under the census roots that
// holds at least one .go file, as module-relative slash paths ("." for
// the root itself). Hidden directories and testdata trees are skipped.
func censusPackageDirs(moduleRoot string) ([]string, error) {
	seen := map[string]bool{}
	for _, root := range CensusRoots {
		abs := filepath.Join(moduleRoot, root)
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		err := filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if path != abs && (strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules") {
					return filepath.SkipDir
				}
				if root == "." && path != abs {
					// The root entry covers root-level files only; internal/
					// and cmd/ are walked as their own roots.
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), ".go") {
				rel, err := filepath.Rel(moduleRoot, filepath.Dir(path))
				if err != nil {
					return err
				}
				seen[filepath.ToSlash(rel)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var out []string
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, nil
}

// ForkExemptReason is the closed set of reasons a package's tests may
// keep a local banned-token oracle that repeats glossary entries.
type ForkExemptReason string

const (
	// ForkExemptVocabularyDependency: the package is imported by
	// internal/skill (the vocabulary owner), so its internal tests cannot
	// import the scanner without an import cycle. The row is verified
	// against skill's actual imports; a package skill stops importing
	// fails the census as a stale row.
	ForkExemptVocabularyDependency ForkExemptReason = "vocabulary_dependency"
)

// ForkExemption names one package directory exempt from the fork census.
type ForkExemption struct {
	Dir    string
	Reason ForkExemptReason
}

// ForkExemptions is the typed escape lane of the fork census.
var ForkExemptions = []ForkExemption{
	{Dir: "internal/types", Reason: ForkExemptVocabularyDependency},
}

// skillImportPath is the vocabulary owner; packages it imports cannot
// import the scanner from their internal tests.
const skillImportPath = "github.com/hanchaoqun/codrax/internal/skill"

// vocabularyDependencyDirs returns the module-relative directories of
// every package internal/skill's non-test files import directly.
func vocabularyDependencyDirs(moduleRoot string) (map[string]bool, error) {
	files, err := listGoFiles(filepath.Join(moduleRoot, "internal", "skill"), false)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, fmt.Errorf("glossarylint: parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if strings.HasPrefix(p, skillImportPath[:strings.LastIndex(skillImportPath, "/internal/")]+"/") {
				out[strings.TrimPrefix(p, skillImportPath[:strings.LastIndex(skillImportPath, "/internal/")]+"/")] = true
			}
		}
	}
	return out, nil
}

// GlossaryForks finds banned-token oracles in _test.go files under the
// census roots that repeat two or more glossary entries verbatim — a
// hand-mirrored vocabulary that will drift from glossary.go. The oracle
// shape is bound by data flow, not by content: a `[]string{…}` literal
// (inline or bound to a local name) that is the range expression of a
// loop whose body passes the loop element to strings.Contains /
// strings.Index / strings.HasPrefix / strings.HasSuffix. Fixture data
// slices that merely list Go names are not oracles and are not
// flagged. Tests that need extra per-surface tokens keep only the
// extras and call ScanTextWith. The glossarylint package's own tests
// are exempt (they must name entries to prove the matcher).
func GlossaryForks(moduleRoot string) ([]string, error) {
	dirs, err := censusPackageDirs(moduleRoot)
	if err != nil {
		return nil, err
	}
	terms := map[string]bool{}
	for _, t := range Terms() {
		terms[t] = true
	}
	deps, err := vocabularyDependencyDirs(moduleRoot)
	if err != nil {
		return nil, err
	}
	exempt := map[string]bool{}
	for _, e := range ForkExemptions {
		switch e.Reason {
		case ForkExemptVocabularyDependency:
			if !deps[e.Dir] {
				return nil, fmt.Errorf("glossarylint: stale fork exemption %s (%s): internal/skill no longer imports it", e.Dir, e.Reason)
			}
		default:
			return nil, fmt.Errorf("glossarylint: fork exemption %s has reason %q outside the closed set", e.Dir, e.Reason)
		}
		exempt[e.Dir] = true
	}
	var forks []string
	for _, dir := range dirs {
		if strings.HasSuffix(dir, "internal/skill/glossarylint") || exempt[dir] {
			continue
		}
		files, err := listGoFiles(filepath.Join(moduleRoot, dir), true)
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		for _, path := range files {
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil, fmt.Errorf("glossarylint: parse %s: %v", path, err)
			}
			for _, lit := range bannedOracleLiterals(file) {
				var repeated []string
				for _, elt := range lit.Elts {
					bl, ok := elt.(*ast.BasicLit)
					if !ok || bl.Kind != token.STRING {
						continue
					}
					raw, err := strconv.Unquote(bl.Value)
					if err != nil {
						continue
					}
					if terms[raw] {
						repeated = append(repeated, raw)
					}
				}
				if len(repeated) >= 2 {
					p := fset.Position(lit.Pos())
					forks = append(forks, fmt.Sprintf("%s:%d repeats glossary entries %q — keep only per-surface extras and call glossarylint.ScanTextWith", filepath.ToSlash(p.Filename), p.Line, repeated))
				}
			}
		}
	}
	sort.Strings(forks)
	return forks, nil
}

// bannedOracleLiterals returns every []string composite literal in file
// that is ranged over by a loop whose body feeds the loop element into a
// strings.Contains-family call (the banned-token oracle shape).
func bannedOracleLiterals(file *ast.File) []*ast.CompositeLit {
	bound := map[string]*ast.CompositeLit{} // local name → literal
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || i >= len(v.Rhs) {
					continue
				}
				if cl := stringSliceLiteral(v.Rhs[i]); cl != nil {
					bound[id.Name] = cl
				}
			}
		case *ast.ValueSpec:
			for i, name := range v.Names {
				if i < len(v.Values) {
					if cl := stringSliceLiteral(v.Values[i]); cl != nil {
						bound[name.Name] = cl
					}
				}
			}
		}
		return true
	})
	var out []*ast.CompositeLit
	seen := map[*ast.CompositeLit]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		rs, ok := n.(*ast.RangeStmt)
		if !ok || rs.Value == nil {
			return true
		}
		elem, ok := rs.Value.(*ast.Ident)
		if !ok {
			return true
		}
		lit := stringSliceLiteral(rs.X)
		if lit == nil {
			if id, ok := rs.X.(*ast.Ident); ok {
				lit = bound[id.Name]
			}
		}
		if lit == nil || seen[lit] || !bodyFeedsStringsMatcher(rs.Body, elem.Name) {
			return true
		}
		seen[lit] = true
		out = append(out, lit)
		return true
	})
	return out
}

func stringSliceLiteral(e ast.Expr) *ast.CompositeLit {
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	at, ok := cl.Type.(*ast.ArrayType)
	if !ok {
		return nil
	}
	if id, ok := at.Elt.(*ast.Ident); !ok || id.Name != "string" {
		return nil
	}
	return cl
}

// bodyFeedsStringsMatcher reports whether body contains a
// strings.Contains / Index / HasPrefix / HasSuffix call whose second
// argument is the identifier elem.
func bodyFeedsStringsMatcher(body *ast.BlockStmt, elem string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "strings" {
			return true
		}
		switch sel.Sel.Name {
		case "Contains", "Index", "HasPrefix", "HasSuffix", "Count", "LastIndex":
		default:
			return true
		}
		if id, ok := call.Args[1].(*ast.Ident); ok && id.Name == elem {
			found = true
		}
		return !found
	})
	return found
}
