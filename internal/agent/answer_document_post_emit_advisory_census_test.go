package agent

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// answer_document_post_emit_advisory_census_test.go — V3-3 (§40.51)
// tripwire: the post-emit advisory pass is ONE table, ONE latch, ONE arm.
//
//  ① Observe's accepted-document branch (the `len(docV2.Blocks) > 0` guard)
//     carries exactly one `if sig := e.<call>(…); sig.HintRequested { return
//     sig }` arm and its callee is postEmitAdvisorySignal; every other
//     statement in that branch must be the accepting Stop return. Any other
//     statement shape (a second sequential lane arm, a call, an assignment)
//     FAILS LOUD — a new post-emit lane is a row of postEmitAdvisoryLanes,
//     never a new arm.
//  ② The lane table is closed: len(postEmitAdvisoryLanes) equals the number
//     of postEmitAdvisoryLaneKind constants and every constant appears
//     exactly once as a row's kind.
//  ③ One latch: answerDocumentEvaluator has exactly one field named
//     postEmitAdvisoryDelivered, no field whose name ends in "Hinted", and
//     the latch is assigned only in postEmitAdvisorySignal (set) and
//     BuildInitialInstruction (reset).
//
// Self-red (TestPostEmitAdvisoryCensusFlagsEachEvasionShape): a second
// sequential arm, a stray latch, a table/constant count mismatch and a latch
// write outside the two owners are each flagged on synthetic sources; the
// canonical shape passes.

type postEmitAdvisoryCensus struct {
	offenders []string
	armCallee string
	laneRows  int
	kindConst int
}

const (
	postEmitAdvisoryEvaluatorType = "answerDocumentEvaluator"
	postEmitAdvisoryLatchField    = "postEmitAdvisoryDelivered"
	postEmitAdvisorySignalFunc    = "postEmitAdvisorySignal"
	postEmitAdvisoryTableVar      = "postEmitAdvisoryLanes"
	postEmitAdvisoryKindType      = "postEmitAdvisoryLaneKind"
)

func runPostEmitAdvisoryCensus(fset *token.FileSet, files []*ast.File) *postEmitAdvisoryCensus {
	c := &postEmitAdvisoryCensus{}
	var observe *ast.FuncDecl
	latchWriters := map[string]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil && postEmitAdvisoryRecvType(d) == postEmitAdvisoryEvaluatorType && d.Name.Name == "Observe" {
					observe = d
				}
				// ③ latch writers
				if d.Body != nil {
					ast.Inspect(d.Body, func(n ast.Node) bool {
						as, ok := n.(*ast.AssignStmt)
						if !ok {
							return true
						}
						for _, lhs := range as.Lhs {
							if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == postEmitAdvisoryLatchField {
								latchWriters[d.Name.Name]++
							}
						}
						return true
					})
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.Name != postEmitAdvisoryEvaluatorType {
							continue
						}
						st, ok := s.Type.(*ast.StructType)
						if !ok {
							continue
						}
						latch := 0
						for _, field := range st.Fields.List {
							for _, name := range field.Names {
								if name.Name == postEmitAdvisoryLatchField {
									latch++
								} else if strings.HasSuffix(name.Name, "Hinted") {
									c.offenders = append(c.offenders, fmt.Sprintf("%s: evaluator field %s is a per-lane latch; the post-emit advisory pass has ONE latch (%s)", fset.Position(name.Pos()), name.Name, postEmitAdvisoryLatchField))
								}
							}
						}
						if latch != 1 {
							c.offenders = append(c.offenders, fmt.Sprintf("evaluator must declare exactly one %s field, found %d", postEmitAdvisoryLatchField, latch))
						}
					case *ast.ValueSpec:
						// ② constants of the kind type / the table literal
						if ident, ok := s.Type.(*ast.Ident); ok && ident.Name == postEmitAdvisoryKindType && d.Tok == token.CONST {
							c.kindConst += len(s.Names)
						}
						for i, name := range s.Names {
							if name.Name != postEmitAdvisoryTableVar || i >= len(s.Values) {
								continue
							}
							lit, ok := s.Values[i].(*ast.CompositeLit)
							if !ok {
								c.offenders = append(c.offenders, fmt.Sprintf("%s: %s must be a composite literal table", fset.Position(name.Pos()), postEmitAdvisoryTableVar))
								continue
							}
							c.laneRows = len(lit.Elts)
							seenKinds := map[string]int{}
							for _, elt := range lit.Elts {
								row, ok := elt.(*ast.CompositeLit)
								if !ok {
									c.offenders = append(c.offenders, fmt.Sprintf("%s: %s row is not a composite literal", fset.Position(elt.Pos()), postEmitAdvisoryTableVar))
									continue
								}
								kind := ""
								for _, kv := range row.Elts {
									pair, ok := kv.(*ast.KeyValueExpr)
									if !ok {
										continue
									}
									if key, ok := pair.Key.(*ast.Ident); ok && key.Name == "kind" {
										if v, ok := pair.Value.(*ast.Ident); ok {
											kind = v.Name
										}
									}
								}
								if kind == "" {
									c.offenders = append(c.offenders, fmt.Sprintf("%s: %s row must name its kind by a %s constant", fset.Position(row.Pos()), postEmitAdvisoryTableVar, postEmitAdvisoryKindType))
									continue
								}
								seenKinds[kind]++
							}
							for kind, n := range seenKinds {
								if n != 1 {
									c.offenders = append(c.offenders, fmt.Sprintf("%s: lane kind %s appears %d times in %s", fset.Position(name.Pos()), kind, n, postEmitAdvisoryTableVar))
								}
							}
						}
					}
				}
			}
		}
	}
	if c.laneRows != c.kindConst {
		c.offenders = append(c.offenders, fmt.Sprintf("%s has %d rows but %d %s constants exist — the table must be the closed set", postEmitAdvisoryTableVar, c.laneRows, c.kindConst, postEmitAdvisoryKindType))
	}
	for name, n := range latchWriters {
		if name != postEmitAdvisorySignalFunc && name != "BuildInitialInstruction" {
			c.offenders = append(c.offenders, fmt.Sprintf("%s writes %s (%d×); only %s (set) and BuildInitialInstruction (reset) own the latch", name, postEmitAdvisoryLatchField, n, postEmitAdvisorySignalFunc))
		}
	}
	for _, owner := range []string{postEmitAdvisorySignalFunc, "BuildInitialInstruction"} {
		if latchWriters[owner] == 0 {
			c.offenders = append(c.offenders, fmt.Sprintf("%s must write %s", owner, postEmitAdvisoryLatchField))
		}
	}
	// ① Observe's accepted-document branch
	if observe == nil {
		c.offenders = append(c.offenders, "answerDocumentEvaluator.Observe not found")
		sort.Strings(c.offenders)
		return c
	}
	var branch *ast.BlockStmt
	ast.Inspect(observe.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok || branch != nil {
			return branch == nil
		}
		if postEmitAdvisoryCondIsNonEmptyDoc(ifs.Cond) {
			branch = ifs.Body
			return false
		}
		return true
	})
	if branch == nil {
		c.offenders = append(c.offenders, "Observe: the `len(docV2.Blocks) > 0` accepted-document branch was not found")
		sort.Strings(c.offenders)
		return c
	}
	arms := 0
	for _, stmt := range branch.List {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			callee, ok := postEmitAdvisoryHintArm(s)
			if !ok {
				c.offenders = append(c.offenders, fmt.Sprintf("%s: Observe accepted-document branch holds an if-statement that is not a `if sig := e.<call>(…); sig.HintRequested { return sig }` arm (fail-loud)", fset.Position(s.Pos())))
				continue
			}
			arms++
			c.armCallee = callee
			if callee != postEmitAdvisorySignalFunc {
				c.offenders = append(c.offenders, fmt.Sprintf("%s: Observe accepted-document branch arm calls %s; the only post-emit arm is %s — add a lane as a row of %s", fset.Position(s.Pos()), callee, postEmitAdvisorySignalFunc, postEmitAdvisoryTableVar))
			}
		case *ast.ReturnStmt:
			if !postEmitAdvisoryIsStopReturn(s) {
				c.offenders = append(c.offenders, fmt.Sprintf("%s: Observe accepted-document branch return is not the accepting Stop literal (fail-loud)", fset.Position(s.Pos())))
			}
		default:
			c.offenders = append(c.offenders, fmt.Sprintf("%s: Observe accepted-document branch holds a %T; only the single advisory arm and the Stop return are allowed (fail-loud)", fset.Position(stmt.Pos()), stmt))
		}
	}
	if arms != 1 {
		c.offenders = append(c.offenders, fmt.Sprintf("Observe accepted-document branch must hold exactly one advisory arm, found %d", arms))
	}
	sort.Strings(c.offenders)
	return c
}

func postEmitAdvisoryRecvType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch t := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// postEmitAdvisoryCondIsNonEmptyDoc matches `… && len(<x>.Blocks) > 0`.
func postEmitAdvisoryCondIsNonEmptyDoc(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok || bin.Op != token.GTR {
			return true
		}
		call, ok := bin.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "len" || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Args[0].(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Blocks" {
			return true
		}
		if lit, ok := bin.Y.(*ast.BasicLit); ok && lit.Value == "0" {
			found = true
		}
		return !found
	})
	return found
}

// postEmitAdvisoryHintArm recognizes `if sig := e.<callee>(…); sig.HintRequested { return sig }`.
func postEmitAdvisoryHintArm(ifs *ast.IfStmt) (string, bool) {
	init, ok := ifs.Init.(*ast.AssignStmt)
	if !ok || len(init.Lhs) != 1 || len(init.Rhs) != 1 || init.Tok != token.DEFINE {
		return "", false
	}
	sigIdent, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return "", false
	}
	call, ok := init.Rhs[0].(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	cond, ok := ifs.Cond.(*ast.SelectorExpr)
	if !ok || cond.Sel.Name != "HintRequested" {
		return "", false
	}
	if id, ok := cond.X.(*ast.Ident); !ok || id.Name != sigIdent.Name {
		return "", false
	}
	if ifs.Else != nil || len(ifs.Body.List) != 1 {
		return "", false
	}
	ret, ok := ifs.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return "", false
	}
	if id, ok := ret.Results[0].(*ast.Ident); !ok || id.Name != sigIdent.Name {
		return "", false
	}
	return sel.Sel.Name, true
}

// postEmitAdvisoryIsStopReturn recognizes `return LoopSignal{StopRequested: true, …}`.
func postEmitAdvisoryIsStopReturn(ret *ast.ReturnStmt) bool {
	if len(ret.Results) != 1 {
		return false
	}
	lit, ok := ret.Results[0].(*ast.CompositeLit)
	if !ok {
		return false
	}
	if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "LoopSignal" {
		return false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "StopRequested" {
			if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
				return true
			}
		}
	}
	return false
}

func TestPostEmitAdvisoryIsOneTableOneLatchOneArm(t *testing.T) {
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range []string{"answer_document_evaluator.go", "answer_document_post_emit_advisory.go"} {
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, file)
	}
	c := runPostEmitAdvisoryCensus(fset, files)
	for _, o := range c.offenders {
		t.Errorf("post-emit advisory: %s", o)
	}
	if c.armCallee != postEmitAdvisorySignalFunc {
		t.Fatalf("Observe's accepted-document arm must call %s, found %q", postEmitAdvisorySignalFunc, c.armCallee)
	}
	if c.laneRows < 4 || c.kindConst < 4 {
		t.Fatalf("the four live lanes must be rows (rows=%d, kinds=%d)", c.laneRows, c.kindConst)
	}
	if len(postEmitAdvisoryLanes) != c.laneRows {
		t.Fatalf("runtime table has %d rows, source literal has %d", len(postEmitAdvisoryLanes), c.laneRows)
	}
}

func TestPostEmitAdvisoryCensusFlagsEachEvasionShape(t *testing.T) {
	const canonicalEvaluator = `package agent

type answerDocumentEvaluator struct {
	mu *mutable
	postEmitAdvisoryDelivered bool
}

func (e *answerDocumentEvaluator) BuildInitialInstruction() string {
	e.postEmitAdvisoryDelivered = false
	return ""
}

func (e *answerDocumentEvaluator) Observe(obs LoopObservation) LoopSignal {
	if obs.Phase == PhaseMidLoop {
		if e.mu != nil {
			if docV2 := e.mu.AnswerDocumentV2(); docV2 != nil && len(docV2.Blocks) > 0 {
				if sig := e.postEmitAdvisorySignal(docV2); sig.HintRequested {
					return sig
				}
				return LoopSignal{StopRequested: true, StopReason: "emit_answer_document called"}
			}
		}
	}
	return LoopSignal{}
}
`
	const canonicalAdvisory = `package agent

type postEmitAdvisoryLaneKind string

const (
	laneA postEmitAdvisoryLaneKind = "a"
	laneB postEmitAdvisoryLaneKind = "b"
)

type postEmitAdvisoryLane struct {
	kind   postEmitAdvisoryLaneKind
	detect func() bool
}

var postEmitAdvisoryLanes = []postEmitAdvisoryLane{
	{kind: laneA, detect: func() bool { return false }},
	{kind: laneB, detect: func() bool { return false }},
}

func (e *answerDocumentEvaluator) postEmitAdvisorySignal(doc *doc) LoopSignal {
	if e.postEmitAdvisoryDelivered {
		return LoopSignal{}
	}
	e.postEmitAdvisoryDelivered = true
	return LoopSignal{HintRequested: true}
}
`
	cases := []struct {
		name      string
		evaluator string
		advisory  string
		want      string
	}{
		{name: "canonical shape passes", evaluator: canonicalEvaluator, advisory: canonicalAdvisory, want: ""},
		{
			name: "second sequential lane arm is red",
			evaluator: strings.Replace(canonicalEvaluator,
				"				return LoopSignal{StopRequested: true, StopReason: \"emit_answer_document called\"}\n",
				"				if sig := e.someOtherLaneSignal(docV2); sig.HintRequested {\n					return sig\n				}\n				return LoopSignal{StopRequested: true, StopReason: \"emit_answer_document called\"}\n", 1),
			advisory: canonicalAdvisory,
			want:     "arm calls someOtherLaneSignal",
		},
		{
			name: "stray statement in the branch fails loud",
			evaluator: strings.Replace(canonicalEvaluator,
				"				return LoopSignal{StopRequested: true, StopReason: \"emit_answer_document called\"}\n",
				"				e.observeSomething(docV2)\n				return LoopSignal{StopRequested: true, StopReason: \"emit_answer_document called\"}\n", 1),
			advisory: canonicalAdvisory,
			want:     "holds a *ast.ExprStmt",
		},
		{
			name: "per-lane latch field is red",
			evaluator: strings.Replace(canonicalEvaluator,
				"	postEmitAdvisoryDelivered bool\n",
				"	postEmitAdvisoryDelivered bool\n	traceEntityHinted bool\n", 1),
			advisory: canonicalAdvisory,
			want:     "traceEntityHinted is a per-lane latch",
		},
		{
			name:      "table smaller than the closed set is red",
			evaluator: canonicalEvaluator,
			advisory:  strings.Replace(canonicalAdvisory, "	{kind: laneB, detect: func() bool { return false }},\n", "", 1),
			want:      "has 1 rows but 2 postEmitAdvisoryLaneKind constants",
		},
		{
			name:      "duplicate kind row is red",
			evaluator: canonicalEvaluator,
			advisory:  strings.Replace(canonicalAdvisory, "	{kind: laneB, detect: func() bool { return false }},\n", "	{kind: laneA, detect: func() bool { return false }},\n", 1),
			want:      "lane kind laneA appears 2 times",
		},
		{
			name:      "latch written outside its owners is red",
			evaluator: canonicalEvaluator,
			advisory:  canonicalAdvisory + "\nfunc (e *answerDocumentEvaluator) sneak() { e.postEmitAdvisoryDelivered = true }\n",
			want:      "sneak writes postEmitAdvisoryDelivered",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			fset := token.NewFileSet()
			var files []*ast.File
			for name, src := range map[string]string{"evaluator.go": tc.evaluator, "advisory.go": tc.advisory} {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
				file, err := parser.ParseFile(fset, path, nil, 0)
				if err != nil {
					t.Fatalf("parse %s: %v", name, err)
				}
				files = append(files, file)
			}
			c := runPostEmitAdvisoryCensus(fset, files)
			if tc.want == "" {
				if len(c.offenders) != 0 {
					t.Fatalf("canonical shape must be clean, got %v", c.offenders)
				}
				return
			}
			for _, o := range c.offenders {
				if strings.Contains(o, tc.want) {
					return
				}
			}
			t.Fatalf("expected an offender containing %q, got %v", tc.want, c.offenders)
		})
	}
}
