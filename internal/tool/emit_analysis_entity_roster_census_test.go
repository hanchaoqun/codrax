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

const (
	emitAnalysisDecodeRosterIdent = "entities"
	emitAnalysisFrozenRosterIdent = "modelEntities"
	emitAnalysisFrozenRosterType  = "modelEntityRoster"
	emitAnalysisFreezeCall        = "freezeModelEntityRoster(entities)"
	emitAnalysisFrozenAccessor    = "Entities"
)

type emitAnalysisEntityRosterCensusResult struct {
	offenders   []string
	assignments int // assignments to the decode slice
	captures    int // assignments to the frozen value
	gateReads   int // frozen-accessor reads inside registered gate bodies
	frozenUses  int // frozen-value uses inside Execute after capture
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

	// Pass 3: registered gate bodies read only the frozen parameter.
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
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "entities" {
				offenders = append(offenders, fset.Position(sel.Pos()).String()+" touches a frozen-roster backing field outside "+emitAnalysisFrozenRosterType)
			}
			return true
		})
	}
	return offenders, nil
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
		if !strings.Contains(string(body), ".entities") {
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
