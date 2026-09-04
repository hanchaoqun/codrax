package tool

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_analysis_entity_roster_census_test.go — V4-3 structural tripwire
// (colleague_merge_audit §40.21 ③, §40.47 fold-in A5; template:
// diagram_identity_authority_census).
//
// Rule: a hard gate's input must be the model's ORIGINAL emission or a
// lossless normalization of it. Inside (*EmitAnalysis).Execute the decode
// slice `entities` may be assigned only from the model emission
// (trimStringSlice(p.Entities)) and its subset-preserving blocklist filter
// (val.FilteredEntities); it is then frozen once into `modelEntities`
// (freezeModelEntityRoster) and dies — every later reader, the participant
// gate and the persisted RequestModel included, takes only the frozen value
// through its copying accessor. The census is total over the write shapes
// that could reach a gate: assignment, index write, address-taking, passing
// to an unregistered function, a RequestModel roster field rewrite, a second
// capture, a field access on the frozen value, and a gate body reading any
// roster other than the frozen one. The companion property pin asserts that
// every registered producer is subset-preserving and leaves its input
// untouched.
//
// §40.47 fold-in round five: the census additionally pins the OTHER SIDE of
// each lane, so a rewritten roster cannot be smuggled around the identifier
// judgements —
//   - every registered gate CALL passes the bare frozen ident at the
//     modelEntityRoster parameter position (no inline freeze, no second
//     variable, no roster literal);
//   - freezeModelEntityRoster appears exactly once in this file (the
//     capture) and nowhere else in the package, and no modelEntityRoster
//     composite literal exists outside the frozen type's own file;
//   - the persisted roster mint is pinned at its source: every `Entities:` /
//     `PrimaryEntities:` key-value inside Execute carries exactly
//     `modelEntities.Entities()`;
//   - `&rm` may be handed only to registered RequestModel mutators, and each
//     registered mutator's body (package-wide) never assigns a roster field.

// emitAnalysisEntityRosterProducers is the single declared registry of RHS
// expressions allowed to assign the decode slice `entities` inside Execute.
var emitAnalysisEntityRosterProducers = map[string]bool{
	"trimStringSlice(p.Entities)": true,
	"val.FilteredEntities":        true,
}

// emitAnalysisEntityRosterPreCaptureConsumers may receive the decode slice
// before the capture (they produce the registered filter result).
var emitAnalysisEntityRosterPreCaptureConsumers = map[string]bool{
	"validateAnalysisInput": true,
}

// emitAnalysisFrozenRosterGates are the only functions the frozen value may
// be passed to; each must take it as a modelEntityRoster parameter and read
// no other roster.
var emitAnalysisFrozenRosterGates = map[string]bool{
	"validateRequiredFlowDiagramParticipantProvenance": true,
}

// emitAnalysisRequestModelMutators is the single declared registry of
// functions allowed to receive `&rm` inside Execute. Registration is the
// review surface: a registered mutator's body is additionally pinned
// (package-wide) to never assign the persisted roster fields, so a pointer
// helper can never rewrite Entities/PrimaryEntities behind the gate.
var emitAnalysisRequestModelMutators = map[string]bool{
	"projectRuntimeArtifactPathHintsFromRawRequest":           true,
	"attachRuntimeArtifactsToRequestModel":                    true,
	"dropSourceInventoryProfileForTypedRelation":              true,
	"dropSourceInventoryProfileForObservationOnlyRuntime":     true,
	"softenAnswerRoleProfileForPerMemberRelation":             true,
	"synthesizeSourceInventoryProfileForTypedEnumeration":     true,
	"enrichSourceInventoryProfileFromAnalyzerPrescan":         true,
	"normalizeSourceInventoryProductionScope":                 true,
	"normalizeSourceInventoryConstructOnlySourceScope":        true,
	"normalizeSourceInventoryAuxiliaryExclusion":              true,
	"normalizeSingleTargetExplicitWindowCausalSubTopics":      true,
	"normalizeSourceInventorySubTopicsFromProfile":            true,
	"normalizeSourceInventoryKeywordsFromProfile":             true,
	"projectAnalyzerPrescanRequiredFileHints":                 true,
	"normalizeUnbackedExternalObservationAllowToDefault":      true,
	"normalizeUnbackedExternalObservationCurrentVersionCheck": true,
}

const (
	emitAnalysisDecodeRosterIdent = "entities"
	emitAnalysisFrozenRosterIdent = "modelEntities"
	emitAnalysisFrozenRosterType  = "modelEntityRoster"
	emitAnalysisFreezeCall        = "freezeModelEntityRoster(entities)"
	emitAnalysisFreezeIdent       = "freezeModelEntityRoster"
	emitAnalysisFrozenAccessor    = "Entities"
	emitAnalysisRequestModelIdent = "rm"
	emitAnalysisRosterMintExpr    = "modelEntities.Entities()"
)

type emitAnalysisEntityRosterCensusResult struct {
	offenders   []string
	assignments int // assignments to the decode slice
	captures    int // assignments to the frozen value
	gateReads   int // frozen-accessor reads inside registered gate bodies
	frozenUses  int // frozen-value uses inside Execute after capture
	gateCalls   int // calls to registered gates inside Execute
	freezeCalls int // occurrences of freezeModelEntityRoster in the file
	rosterMints int // Entities/PrimaryEntities key-values inside Execute
	rmHandoffs  int // &rm handoffs to registered RequestModel mutators
}

func emitAnalysisEntityRosterCensus(src string) (emitAnalysisEntityRosterCensusResult, error) {
	var res emitAnalysisEntityRosterCensusResult
	fset := token.NewFileSet()
	file, perr := parser.ParseFile(fset, "emit_analysis.go", src, 0)
	if perr != nil {
		return res, perr
	}
	var execute *ast.FuncDecl
	gates := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == "Execute" && emitAnalysisReceiver(fn) {
			execute = fn
		}
		if fn.Recv == nil && emitAnalysisFrozenRosterGates[fn.Name.Name] {
			gates[fn.Name.Name] = fn
		}
	}
	if execute == nil {
		return res, errEmitAnalysisExecuteNotFound
	}
	pos := func(n ast.Node) string { return fset.Position(n.Pos()).String() }
	print := func(n ast.Node) string {
		var b strings.Builder
		_ = printer.Fprint(&b, fset, n)
		return b.String()
	}
	report := func(n ast.Node, what string) {
		res.offenders = append(res.offenders, pos(n)+" "+what)
	}
	parents := emitAnalysisParentMap(execute.Body)

	// Pass 1: assignments. Locate the capture and judge every roster write.
	var capture, captureEnd token.Pos
	var lastDecodeAssign token.Pos
	ast.Inspect(execute.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			rhs := "<multi-value>"
			if len(assign.Rhs) == len(assign.Lhs) {
				rhs = print(assign.Rhs[i])
			}
			switch x := lhs.(type) {
			case *ast.Ident:
				switch x.Name {
				case emitAnalysisDecodeRosterIdent:
					res.assignments++
					lastDecodeAssign = assign.Pos()
					if !emitAnalysisEntityRosterProducers[rhs] {
						report(assign, "entities = "+rhs+" (unregistered roster producer)")
					}
				case emitAnalysisFrozenRosterIdent:
					res.captures++
					if res.captures > 1 {
						report(assign, "second write to the frozen roster "+emitAnalysisFrozenRosterIdent)
					}
					if rhs != emitAnalysisFreezeCall || assign.Tok != token.DEFINE {
						report(assign, emitAnalysisFrozenRosterIdent+" = "+rhs+" (capture must be exactly "+emitAnalysisFreezeCall+")")
					}
					capture = assign.Pos()
					captureEnd = assign.End()
				}
			case *ast.SelectorExpr:
				if x.Sel.Name == "Entities" || x.Sel.Name == "PrimaryEntities" {
					report(assign, "RequestModel roster field rewrite "+print(x)+" = "+rhs)
				}
			case *ast.IndexExpr:
				if id, ok := x.X.(*ast.Ident); ok && (id.Name == emitAnalysisDecodeRosterIdent || id.Name == emitAnalysisFrozenRosterIdent) {
					report(assign, "index write "+print(x))
				}
			}
		}
		return true
	})
	if res.captures == 0 {
		report(execute, "Execute never freezes the roster ("+emitAnalysisFreezeCall+" missing)")
		return res, nil
	}
	if capture < lastDecodeAssign {
		report(execute, "the roster is frozen before its last decode assignment — the gate would judge a pre-filter roster")
	}

	// Pass 2: every identifier occurrence of the decode slice and of the
	// frozen value inside Execute, judged by its syntactic parent.
	ast.Inspect(execute.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || (id.Name != emitAnalysisDecodeRosterIdent && id.Name != emitAnalysisFrozenRosterIdent) {
			return true
		}
		parent := parents[id]
		if sel, ok := parent.(*ast.SelectorExpr); ok && sel.Sel == id {
			return true // a field/method NAME, not the variable
		}
		if kv, ok := parent.(*ast.KeyValueExpr); ok && kv.Key == id {
			return true // a struct key, not the variable
		}
		if assign, ok := parent.(*ast.AssignStmt); ok && emitAnalysisIsLHS(assign, id) {
			return true // judged in pass 1
		}
		if id.Name == emitAnalysisDecodeRosterIdent {
			if id.Pos() > captureEnd {
				report(id, "decode slice `entities` used after the capture ("+print(parent)+") — read modelEntities.Entities() instead")
				return true
			}
			call, ok := parent.(*ast.CallExpr)
			if !ok {
				report(id, "decode slice `entities` in a non-call context before capture: "+print(parent))
				return true
			}
			callee := emitAnalysisCallee(call)
			if callee != "freezeModelEntityRoster" && !emitAnalysisEntityRosterPreCaptureConsumers[callee] {
				report(id, "decode slice `entities` passed to unregistered "+callee)
			}
			return true
		}
		// Frozen value.
		res.frozenUses++
		switch p := parent.(type) {
		case *ast.SelectorExpr:
			call, ok := parents[p].(*ast.CallExpr)
			if p.Sel.Name != emitAnalysisFrozenAccessor || !ok || call.Fun != p {
				report(id, "frozen roster field/method access "+print(parents[p])+" (only ."+emitAnalysisFrozenAccessor+"() is allowed)")
			}
		case *ast.CallExpr:
			callee := emitAnalysisCallee(p)
			if !emitAnalysisFrozenRosterGates[callee] {
				report(id, "frozen roster passed to unregistered "+callee)
			}
		default:
			report(id, "frozen roster in a non-gate context: "+print(parent))
		}
		return true
	})

	// Pass 3 (§40.47 round five): the whole file — the freeze call is unique
	// (a second freeze under any name is a second capture) and no roster
	// composite literal mints an unfrozen roster.
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if x.Name == emitAnalysisFreezeIdent {
				res.freezeCalls++
			}
		case *ast.CompositeLit:
			if id, ok := x.Type.(*ast.Ident); ok && id.Name == emitAnalysisFrozenRosterType {
				report(x, "roster literal "+print(x)+" mints an unfrozen roster (only "+emitAnalysisFreezeIdent+" may)")
			}
		}
		return true
	})
	if res.freezeCalls > 1 {
		report(execute, emitAnalysisFreezeIdent+" appears "+strconv.Itoa(res.freezeCalls)+" times — the roster is frozen exactly once, at the capture")
	}

	// Pass 4 (§40.47 round five): gate call sites, persisted roster mints and
	// &rm handoffs inside Execute.
	gateRosterArg := map[string]int{}
	for name, fn := range gates {
		idx := 0
		for _, field := range fn.Type.Params.List {
			n := len(field.Names)
			if n == 0 {
				n = 1
			}
			if id, ok := field.Type.(*ast.Ident); ok && id.Name == emitAnalysisFrozenRosterType {
				gateRosterArg[name] = idx
			}
			idx += n
		}
	}
	ast.Inspect(execute.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.KeyValueExpr:
			key, ok := x.Key.(*ast.Ident)
			if !ok || (key.Name != "Entities" && key.Name != "PrimaryEntities") {
				return true
			}
			res.rosterMints++
			if print(x.Value) != emitAnalysisRosterMintExpr {
				report(x, "persisted "+key.Name+" minted from "+print(x.Value)+" — the mint source must be exactly "+emitAnalysisRosterMintExpr)
			}
		case *ast.UnaryExpr:
			if x.Op != token.AND {
				return true
			}
			id, ok := x.X.(*ast.Ident)
			if !ok || id.Name != emitAnalysisRequestModelIdent {
				return true
			}
			callee := ""
			if call, ok := parents[x].(*ast.CallExpr); ok && call.Fun != x {
				callee = emitAnalysisCallee(call)
			}
			if callee == "" || !emitAnalysisRequestModelMutators[callee] {
				report(x, "&"+emitAnalysisRequestModelIdent+" handed to unregistered "+callee+" — a pointer helper could rewrite the persisted roster behind the gate")
				return true
			}
			res.rmHandoffs++
		case *ast.CallExpr:
			callee := emitAnalysisCallee(x)
			if !emitAnalysisFrozenRosterGates[callee] {
				return true
			}
			res.gateCalls++
			idx, ok := gateRosterArg[callee]
			if !ok || idx >= len(x.Args) {
				report(x, "gate "+callee+" call does not match its "+emitAnalysisFrozenRosterType+" signature")
				return true
			}
			if id, ok := x.Args[idx].(*ast.Ident); !ok || id.Name != emitAnalysisFrozenRosterIdent {
				report(x, "gate "+callee+" called with "+print(x.Args[idx])+" — the roster argument must be the frozen ident "+emitAnalysisFrozenRosterIdent)
			}
		}
		return true
	})

	// Pass 5: registered gate bodies read only the frozen parameter.
	for name := range emitAnalysisFrozenRosterGates {
		fn := gates[name]
		if fn == nil {
			report(execute, "registered gate "+name+" not found")
			continue
		}
		param := ""
		for _, field := range fn.Type.Params.List {
			if id, ok := field.Type.(*ast.Ident); ok && id.Name == emitAnalysisFrozenRosterType && len(field.Names) == 1 {
				param = field.Names[0].Name
			}
		}
		if param == "" {
			report(fn, "gate "+name+" takes no "+emitAnalysisFrozenRosterType+" parameter")
			continue
		}
		gateParents := emitAnalysisParentMap(fn.Body)
		// §40.43 round-six #12: bind the judged roster by DATA FLOW to the
		// frozen capture. A frozen read (`param.Entities()`) either feeds a
		// recognized consumption (call argument, range operand, …) or binds
		// exactly one local ident; a BOUND ident may never be reassigned
		// (`entities = foreignRosterNames(rm)` made the registered gate
		// judge an arbitrary non-frozen roster while the census stayed
		// green), a discarded read (`_ = roster.Entities()`) is red (it
		// satisfied the gateReads floor while the gate judged something
		// else), and a bound ident that is never read afterwards is red too
		// (a dead bind is the same laundering with one more step).
		boundRosters := map[string]bool{}
		bindingStmts := map[ast.Node]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != emitAnalysisFrozenAccessor {
				return true
			}
			x, isIdent := sel.X.(*ast.Ident)
			call, isCall := gateParents[sel].(*ast.CallExpr)
			if !isIdent || x.Name != param || !isCall || call.Fun != sel {
				return true
			}
			switch p := gateParents[call].(type) {
			case *ast.AssignStmt:
				if len(p.Lhs) == 1 {
					if id, ok := p.Lhs[0].(*ast.Ident); ok {
						if id.Name == "_" {
							report(p, "gate "+name+" discards the frozen read (`_ = "+print(sel)+"()`) — a dead read satisfies no judgement")
							return true
						}
						boundRosters[id.Name] = true
						bindingStmts[p] = true
					}
				}
			}
			return true
		})
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || bindingStmts[assign] {
				return true
			}
			for _, lhs := range assign.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && boundRosters[id.Name] {
					report(assign, "gate "+name+" rebinds the judged roster "+id.Name+" from a non-frozen source: "+print(assign)+" — the gate may judge only the frozen roster")
				}
			}
			return true
		})
		for bound := range boundRosters {
			reads := 0
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok || id.Name != bound {
					return true
				}
				if assign, ok := gateParents[id].(*ast.AssignStmt); ok && emitAnalysisIsLHS(assign, id) {
					return true
				}
				reads++
				return true
			})
			if reads == 0 {
				report(fn, "gate "+name+" binds the frozen read to "+bound+" but never judges it — a dead bind launders the gateReads floor")
			}
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Entities", "PrimaryEntities":
				x, isIdent := sel.X.(*ast.Ident)
				call, isCall := gateParents[sel].(*ast.CallExpr)
				if isIdent && x.Name == param && sel.Sel.Name == emitAnalysisFrozenAccessor && isCall && call.Fun == sel {
					res.gateReads++
					return true
				}
				report(sel, "gate "+name+" reads a roster other than the frozen parameter: "+print(sel))
			case "entities":
				report(sel, "gate "+name+" touches the frozen backing field: "+print(sel))
			}
			return true
		})
	}
	return res, nil
}

// emitAnalysisFrozenFieldCensus reports every `.entities` selector in the
// package's non-test files outside the frozen type's own constructor and
// methods — the backing array is reachable nowhere else.
func emitAnalysisFrozenFieldCensus(fileName, src string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fileName, src, 0)
	if err != nil {
		return nil, err
	}
	var offenders []string
	for _, decl := range file.Decls {
		fn, isFn := decl.(*ast.FuncDecl)
		owner := false
		if isFn && fileName == "emit_analysis_entity_roster.go" {
			if fn.Name.Name == "freezeModelEntityRoster" {
				owner = true
			}
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok && id.Name == emitAnalysisFrozenRosterType {
					owner = true
				}
			}
		}
		if owner {
			continue
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if x.Sel.Name == "entities" {
					offenders = append(offenders, fset.Position(x.Pos()).String()+" touches a frozen-roster backing field outside "+emitAnalysisFrozenRosterType)
				}
			case *ast.CompositeLit:
				if id, ok := x.Type.(*ast.Ident); ok && id.Name == emitAnalysisFrozenRosterType {
					offenders = append(offenders, fset.Position(x.Pos()).String()+" mints a "+emitAnalysisFrozenRosterType+" literal outside its owner (only "+emitAnalysisFreezeIdent+" may)")
				}
			}
			return true
		})
	}
	return offenders, nil
}

// emitAnalysisMutatorBodyOffenders inspects the bodies of registered
// RequestModel mutators in one file: none may assign the persisted roster
// fields. Returns the offenders and the mutators the file defines.
func emitAnalysisMutatorBodyOffenders(fileName, src string, mutators map[string]bool) ([]string, map[string]bool, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fileName, src, 0)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	var offenders []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Recv != nil || !mutators[fn.Name.Name] {
			continue
		}
		seen[fn.Name.Name] = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Entities" || sel.Sel.Name == "PrimaryEntities") {
					offenders = append(offenders, fset.Position(sel.Pos()).String()+" registered RequestModel mutator "+fn.Name.Name+" rewrites the persisted roster field "+sel.Sel.Name)
				}
			}
			return true
		})
	}
	return offenders, seen, nil
}

func emitAnalysisParentMap(root ast.Node) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}

func emitAnalysisIsLHS(assign *ast.AssignStmt, id *ast.Ident) bool {
	for _, lhs := range assign.Lhs {
		if lhs == id {
			return true
		}
	}
	return false
}

type emitAnalysisExecuteNotFound struct{}

func (emitAnalysisExecuteNotFound) Error() string { return "(*EmitAnalysis).Execute not found" }

var errEmitAnalysisExecuteNotFound = emitAnalysisExecuteNotFound{}

func emitAnalysisReceiver(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "EmitAnalysis"
}

func TestEmitAnalysisEntityRosterCensus(t *testing.T) {
	src, err := os.ReadFile("emit_analysis.go")
	if err != nil {
		t.Fatal(err)
	}
	res, err := emitAnalysisEntityRosterCensus(string(src))
	if err != nil {
		t.Fatalf("census parse failed (a silent green would defeat the tripwire): %v", err)
	}
	if res.assignments < 2 || res.captures != 1 || res.gateReads < 1 || res.frozenUses < 3 {
		t.Fatalf("census lost its subject: assignments=%d captures=%d gateReads=%d frozenUses=%d", res.assignments, res.captures, res.gateReads, res.frozenUses)
	}
	if res.gateCalls < 1 || res.freezeCalls != 1 || res.rosterMints < 2 || res.rmHandoffs < 1 {
		t.Fatalf("census lost its round-five subject: gateCalls=%d freezeCalls=%d rosterMints=%d rmHandoffs=%d", res.gateCalls, res.freezeCalls, res.rosterMints, res.rmHandoffs)
	}
	if len(res.offenders) > 0 {
		sort.Strings(res.offenders)
		t.Fatalf("entity roster written or read outside the frozen lane (§40.21 ③ — hard gates judge the model's original emission or a lossless normalization of it):\n  %s", strings.Join(res.offenders, "\n  "))
	}

	// Repo-wide companions: no emit-side tool may call a normalizer that
	// rewrites the roster (CompleteSlashPairEntities is the only slash-pair
	// lane and it lives after the gates in agent/analyzer.go; the retired
	// CanonicalizeSlashPairEntities must not return under any name), and the
	// frozen backing field is reachable only inside the frozen type.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fieldSites := 0
	mutatorSeen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, rerr := os.ReadFile(e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		if strings.Contains(string(body), "normalizer.Canonicalize") || strings.Contains(string(body), "normalizer.CompleteSlashPairEntities") {
			t.Fatalf("%s calls an entity-rewriting normalizer inside the emit-side tool package", e.Name())
		}
		if e.Name() != "emit_analysis.go" && e.Name() != "emit_analysis_entity_roster.go" && strings.Contains(string(body), emitAnalysisFreezeIdent) {
			t.Fatalf("%s uses %s — the roster is frozen only inside (*EmitAnalysis).Execute", e.Name(), emitAnalysisFreezeIdent)
		}
		definesMutator := false
		for name := range emitAnalysisRequestModelMutators {
			if strings.Contains(string(body), "func "+name+"(") {
				definesMutator = true
				break
			}
		}
		if definesMutator {
			mutatorOffenders, seen, merr := emitAnalysisMutatorBodyOffenders(e.Name(), string(body), emitAnalysisRequestModelMutators)
			if merr != nil {
				t.Fatal(merr)
			}
			for name := range seen {
				mutatorSeen[name] = true
			}
			if len(mutatorOffenders) > 0 {
				t.Fatalf("registered RequestModel mutator rewrites the persisted roster: %v", mutatorOffenders)
			}
		}
		if !strings.Contains(string(body), ".entities") && !strings.Contains(string(body), emitAnalysisFrozenRosterType) {
			continue
		}
		fieldSites++
		offenders, cerr := emitAnalysisFrozenFieldCensus(e.Name(), string(body))
		if cerr != nil {
			t.Fatal(cerr)
		}
		if len(offenders) > 0 {
			t.Fatalf("frozen roster backing field reachable outside its type: %v", offenders)
		}
	}
	if fieldSites == 0 {
		t.Fatal("frozen-field census saw no file — it lost its subject")
	}
	for name := range emitAnalysisRequestModelMutators {
		if !mutatorSeen[name] {
			t.Fatalf("stale RequestModel mutator registry row %q — the function no longer exists in the package; prune it", name)
		}
	}

	// Self-red: one mutation per evasion shape must be reported.
	gateAnchor := "	if conflict := validateRequiredFlowDiagramParticipantProvenance(rm, modelEntities); conflict != \"\" {"
	decodeAnchor := "	entities = val.FilteredEntities\n"
	mutations := map[string]struct{ anchor, insert string }{
		"retired split reinserted":        {decodeAnchor, decodeAnchor + "	entities = normalizer.CanonicalizeSlashPairEntities(raw, entities)\n"},
		"index write before capture":      {decodeAnchor, decodeAnchor + "	entities[0] = canonicalFoo(raw, entities[0])\n"},
		"address-taking before capture":   {decodeAnchor, decodeAnchor + "	splitFooInPlace(raw, &entities)\n"},
		"unregistered consumer":           {decodeAnchor, decodeAnchor + "	splitFooInPlace(raw, entities)\n"},
		"decode slice used after capture": {gateAnchor, "	splitFooInPlace(raw, entities)\n" + gateAnchor},
		"RequestModel roster rewrite":     {gateAnchor, "	rm.AnalyzerHints.Entities = splitFoo(raw, rm.AnalyzerHints.Entities)\n" + gateAnchor},
		"PrimaryEntities rewrite":         {gateAnchor, "	rm.AnalyzerHints.PrimaryEntities = splitFoo(raw, rm.AnalyzerHints.PrimaryEntities)\n" + gateAnchor},
		"second capture":                  {gateAnchor, "	modelEntities = freezeModelEntityRoster(splitFoo(raw, modelEntities.Entities()))\n" + gateAnchor},
		"frozen field write":              {gateAnchor, "	modelEntities.entities[0] = \"x\"\n" + gateAnchor},
		"frozen address-taking":           {gateAnchor, "	splitFooInPlace(raw, &modelEntities)\n" + gateAnchor},
		"frozen passed to unregistered":   {gateAnchor, "	splitFoo(raw, modelEntities)\n" + gateAnchor},
		"frozen ranged directly":          {gateAnchor, "	for range modelEntities.entities {\n	}\n" + gateAnchor},
		"gate reads RequestModel roster":  {"	entities := roster.Entities()\n", "	entities := rm.AnalyzerHints.Entities\n"},
		"gate touches backing field":      {"	entities := roster.Entities()\n", "	entities := roster.entities\n"},
		// §40.43 round-six #12: pass 5 used to scan only roster-named
		// SelectorExprs, so a registered gate could judge a foreign roster
		// through a helper reassignment, a discarded frozen read, or a dead
		// bind — all while gateReads stayed >= 1.
		"gate rebinds the judged roster via helper": {"	entities := roster.Entities()\n",
			"	entities := roster.Entities()\n	entities = foreignRosterNames(rm)\n"},
		"gate discards the frozen read": {"	entities := roster.Entities()\n",
			"	entities := foreignRosterNames(rm)\n	_ = roster.Entities()\n"},
		"gate dead-binds the frozen read": {"	entities := roster.Entities()\n",
			"	unused := roster.Entities()\n	entities := foreignRosterNames(rm)\n"},
		// §40.47 round five: the five gate-input / persisted-mint evasions.
		"inline freeze in the gate argument": {gateAnchor,
			"	if conflict := validateRequiredFlowDiagramParticipantProvenance(rm, freezeModelEntityRoster(splitFoo(raw, modelEntities.Entities()))); conflict != \"\" {"},
		"second frozen roster under another name": {gateAnchor,
			"	alt := freezeModelEntityRoster(splitFoo(raw, modelEntities.Entities()))\n	_ = alt\n" + gateAnchor},
		"gate called with a non-frozen roster ident": {gateAnchor,
			"	if conflict := validateRequiredFlowDiagramParticipantProvenance(rm, altRoster); conflict != \"\" {"},
		"roster literal in the gate argument": {gateAnchor,
			"	if conflict := validateRequiredFlowDiagramParticipantProvenance(rm, modelEntityRoster{entities: splitFoo(raw, nil)}); conflict != \"\" {"},
		"persisted Entities minted from a rewrite": {"			Entities:          modelEntities.Entities(),\n",
			"			Entities:          splitFoo(raw, modelEntities.Entities()),\n"},
		"rm handed to an unregistered pointer helper": {gateAnchor,
			"	splitFooInPlace(raw, &rm)\n" + gateAnchor},
	}
	for name, m := range mutations {
		mutated := strings.Replace(string(src), m.anchor, m.insert, 1)
		if mutated == string(src) {
			t.Fatalf("self-red %s: anchor not found", name)
		}
		res, err := emitAnalysisEntityRosterCensus(mutated)
		if err != nil {
			t.Fatalf("self-red %s: %v", name, err)
		}
		if len(res.offenders) == 0 {
			t.Fatalf("self-red %s: census stayed green", name)
		}
	}
	fieldOffenders, err := emitAnalysisFrozenFieldCensus("emit_analysis_foo.go", "package tool\nfunc splitFoo(r modelEntityRoster) { r.entities[0] = \"x\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(fieldOffenders) == 0 {
		t.Fatal("self-red: a helper touching the frozen backing field outside its type must be reported")
	}
	litOffenders, err := emitAnalysisFrozenFieldCensus("emit_analysis_foo.go", "package tool\nfunc mintFoo(xs []string) modelEntityRoster { return modelEntityRoster{entities: xs} }\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(litOffenders) == 0 {
		t.Fatal("self-red: a roster literal minted outside the frozen type's owner must be reported")
	}
	mutOffenders, mutSeen, err := emitAnalysisMutatorBodyOffenders("emit_analysis_foo.go",
		"package tool\nfunc normalizeSourceInventoryProductionScope(rm *types.RequestModel) string {\n	rm.AnalyzerHints.Entities = nil\n	return \"\"\n}\n",
		emitAnalysisRequestModelMutators)
	if err != nil {
		t.Fatal(err)
	}
	if !mutSeen["normalizeSourceInventoryProductionScope"] || len(mutOffenders) == 0 {
		t.Fatalf("self-red: a registered mutator assigning a roster field must be reported, got seen=%v offenders=%v", mutSeen, mutOffenders)
	}
}

// TestEmitAnalysisEntityNormalizersNeverMintOrSplit is the property twin of
// the census: every registered producer is subset-preserving — each output
// element is a verbatim trimmed input element, and the count never grows.
// A drop (the documented generic-noun blocklist) is allowed; minting a new
// surface or splitting one surface into two is not.
func TestEmitAnalysisEntityNormalizersNeverMintOrSplit(t *testing.T) {
	inputs := [][]string{
		{"analyzer", "Mutable/BusContext"},
		{" Mutable/BusContext ", "internal/agent", "client/server"},
		{"analyzer", "count", "handler", "FastTokenizer.tokenize", "retry_budget/cap_limit"},
		{"Config", "", "thing"},
	}
	limits := CurrentAnalysisLimits()
	producers := map[string]func([]string) []string{
		"trimStringSlice(p.Entities)": trimStringSlice,
		"val.FilteredEntities": func(in []string) []string {
			return validateAnalysisInput([]string{"k1", "k2", "k3"}, trimStringSlice(in), limits, "", 0).FilteredEntities
		},
	}
	for name := range emitAnalysisEntityRosterProducers {
		if producers[name] == nil {
			t.Fatalf("registered producer %q has no property-pin implementation — register both", name)
		}
	}
	for name := range emitAnalysisEntityRosterPreCaptureConsumers {
		if producers["val.FilteredEntities"] == nil || name != "validateAnalysisInput" {
			t.Fatalf("pre-capture consumer %q has no property-pin implementation — register both", name)
		}
	}
	for name, produce := range producers {
		for _, in := range inputs {
			given := append([]string(nil), in...)
			out := produce(given)
			if !reflect.DeepEqual(given, in) {
				t.Fatalf("%s rewrote its input in place: before=%v after=%v", name, in, given)
			}
			if len(out) > len(in) {
				t.Fatalf("%s minted entities: in=%v out=%v", name, in, out)
			}
			allowed := map[string]bool{}
			for _, e := range in {
				allowed[strings.TrimSpace(e)] = true
			}
			for _, e := range out {
				if !allowed[e] {
					t.Fatalf("%s produced %q which is not a verbatim trimmed input element: in=%v out=%v", name, e, in, out)
				}
			}
		}
	}
}

func TestParticipantRosterCoversEntity(t *testing.T) {
	raw := "请用 Mermaid 架构图画出 analyzer、Mutable/BusContext 之间的数据流，并读 internal/agent 与 client/server"
	roster := func(identities ...string) (map[string]bool, map[string]bool) {
		keys, alias := map[string]bool{}, map[string]bool{}
		for _, id := range identities {
			keys[diagramParticipantProvenanceKey(id)] = true
			alias[diagramParticipantIdentityAliasKey(id)] = true
		}
		return keys, alias
	}
	cases := []struct {
		name   string
		entity string
		rows   []string
		want   bool
	}{
		{"direct row", "analyzer", []string{"analyzer"}, true},
		{"alias row (snake vs Camel)", "emit_answer_document", []string{"EmitAnswerDocument"}, true},
		{"request pair, both halves as rows", "Mutable/BusContext", []string{"Mutable", "BusContext"}, true},
		{"request pair, joined row", "Mutable/BusContext", []string{"Mutable/BusContext"}, true},
		{"request pair, one half only", "Mutable/BusContext", []string{"Mutable"}, false},
		{"request pair, no rows", "Mutable/BusContext", nil, false},
		{"repository path never forms a pair", "internal/agent", []string{"internal", "agent"}, false},
		{"lowercase word pair never forms a pair (shape guard)", "client/server", []string{"client", "server"}, false},
		{"half is not covered by the joined row", "Mutable", []string{"Mutable/BusContext"}, false},
		{"empty entity", "  ", []string{"analyzer"}, false},
	}
	for _, tc := range cases {
		keys, alias := roster(tc.rows...)
		if got := participantRosterCoversEntity(raw, tc.entity, keys, alias); got != tc.want {
			t.Fatalf("%s: participantRosterCoversEntity(%q, rows=%v)=%v want %v", tc.name, tc.entity, tc.rows, got, tc.want)
		}
	}
}

// Acceptance pin (§40.21): request pair `Mutable/BusContext`, model emits ONE
// entity `Mutable/BusContext` plus one participant row per half → accepted,
// and the persisted roster is the model's emission, not a rewritten one.
func TestEmitAnalysis_Execute_KeepsModelJoinedEntityWhenBothHalvesAreParticipants(t *testing.T) {
	raw := "请用 Mermaid 架构图画出 analyzer、Mutable/BusContext 之间的数据流"
	mu := types.NewMutableState(raw)
	payload := `{
		"intent":"explain",
		"scenario":"architecture_explain",
		"complexity":"moderate",
		"keywords":["analyzer","Mutable","BusContext","数据流"],
		"entities":["analyzer","Mutable/BusContext"],
		"question_kind":"mechanism",
		"predicate_axis":"flow",
		"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"analyzer、Mutable/BusContext 之间的数据流","participants":[
			{"identity":"analyzer","role":"incident_required","source_quote":"analyzer"},
			{"identity":"Mutable","role":"incident_required","source_quote":"Mutable/BusContext"},
			{"identity":"BusContext","role":"incident_required","source_quote":"Mutable/BusContext"}
		]}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
		Mutable: mu, PresentationDirective: "Mermaid architecture", PresentationDiagramRequired: true,
	}, json.RawMessage(withV4Required(payload)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("acceptance shape rejected: %s", res.Summary)
	}
	hints := mu.RequestModel().AnalyzerHints
	for _, got := range [][]string{hints.Entities, hints.PrimaryEntities} {
		if !reflect.DeepEqual(got, []string{"analyzer", "Mutable/BusContext"}) {
			t.Fatalf("persisted roster must be the model's emission, got %v", got)
		}
	}
	if strings.Contains(res.Summary, "canonicalized") {
		t.Fatalf("roster was rewritten before the gate: %s", res.Summary)
	}
	if len(mu.RequestModel().DiagramHint.Participants) != 3 {
		t.Fatalf("participants=%+v", mu.RequestModel().DiagramHint.Participants)
	}
}

// §40.47 fold-in (A3): the repair hint follows the single-source teaching
// ("one row per half; the joined token is never one participant"). When the
// model's rows cover exactly ONE half of a request pair it emitted as one
// entity, both arms name the uncovered half — never the joined token the
// model itself emitted — and the gate outcome is unchanged (still rejected;
// adding the named half is accepted).
func TestEmitAnalysis_Execute_JoinedEntityOneHalfRowNamesMissingHalfNotThePair(t *testing.T) {
	raw := "请用 Mermaid 架构图画出 analyzer、Mutable/BusContext 之间的数据流"
	run := func(rows string) (types.ToolResult, *types.MutableState) {
		mu := types.NewMutableState(raw)
		payload := `{
			"intent":"explain",
			"scenario":"architecture_explain",
			"complexity":"moderate",
			"keywords":["analyzer","Mutable","BusContext","数据流"],
			"entities":["analyzer","Mutable/BusContext"],
			"question_kind":"mechanism",
			"predicate_axis":"flow",
			"diagram_hint":{"kind":"architecture","required":true,"relation_scope_quote":"analyzer、Mutable/BusContext 之间的数据流","participants":[` + rows + `]}
		}`
		res, err := (&EmitAnalysis{}).Execute(&types.BusContext{
			Mutable: mu, PresentationDirective: "Mermaid architecture", PresentationDiagramRequired: true,
		}, json.RawMessage(withV4Required(payload)))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return res, mu
	}
	oneHalf := `{"identity":"analyzer","role":"incident_required","source_quote":"analyzer"},
		{"identity":"Mutable","role":"incident_required","source_quote":"Mutable/BusContext"}`
	res, _ := run(oneHalf)
	if res.Success {
		t.Fatalf("one covered half must still be rejected: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "identity/entities [BusContext] but no matching participant row remains") {
		t.Fatalf("omitted arm must name the uncovered half only:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "also names typed relation entity/entities [BusContext] but they have no participant row") {
		t.Fatalf("co-listed arm must name the uncovered half only:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, "[Mutable/BusContext") || strings.Contains(res.Summary, "Mutable/BusContext BusContext]") {
		t.Fatalf("repair hint names the joined token the teaching forbids as a participant:\n%s", res.Summary)
	}
	// Following the hint converges in one step, and the roster is untouched.
	res, mu := run(oneHalf + `,
		{"identity":"BusContext","role":"incident_required","source_quote":"Mutable/BusContext"}`)
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("adding the named half must be accepted: %s", res.Summary)
	}
	if got := mu.RequestModel().AnalyzerHints.Entities; !reflect.DeepEqual(got, []string{"analyzer", "Mutable/BusContext"}) {
		t.Fatalf("persisted roster must be the model's emission, got %v", got)
	}
}
