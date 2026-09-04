package repl

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

// command_operation_request_display_census_test.go — request/display
// argument-order census (colleague_merge_audit §40.52, batch six fold-in
// review round three #7). The operation route carries two texts per turn:
// the REQUEST (what the planner plans from and the answerer's
// `## user_request` — an expanded prompt template, a replayed pasted
// follow-up, the clarification-combined request, a parked plan's
// RequestText) and the DISPLAY (what the history echoes — the typed line,
// "[pasted N line(s)] …", the clarification reply, "/approve"). Every
// dispatcher takes them as (request, display) in that order, and the
// direct pin (command_operation_request_display_test.go) only proves the
// swapped function; a swap at any producing call site upstream stayed
// green. This census binds every call site of the request/display
// dispatchers in package repl's non-test files to the lane of the
// expression it passes, by data flow from the producing variable:
//
//	request lane   — a `.RequestText` / `.OriginalLine` selector (a plan's
//	                 or a parked clarification's request), the first result
//	                 of r.expandTemplateCommand, a
//	                 commandOperationFollowupRequestText call, a parameter
//	                 named `request`;
//	display lane   — a `.Display` selector, a followUpDisplayText call, a
//	                 parameter named `display`, the second result of
//	                 r.readInputPair, a string literal ("/approve" is a
//	                 display form; the user's request is never a literal);
//	raw lane       — a line as typed, valid in either slot: a parameter
//	                 named `line`, a `.Text` selector (a queued follow-up
//	                 entry, a provider request), the first result of
//	                 r.readInputPair;
//	strings.TrimSpace/TrimPrefix/TrimSuffix(x, …) / (x) carry x's lane and
//	so does the first result of splitPastedLog(x); `a + b` merges operand
//	lanes (string literals contribute nothing; raw folds into request);
//	a local carries the merged lane of every assignment to it in the
//	enclosing function EXCEPT an empty-fallback fill (`if x == "" { x = … }`
//	or `if strings.TrimSpace(x) == "" { x = … }`), which never widens the
//	lane, and a self-referential re-assignment (`line = cleaned` where
//	cleaned derives from line), which carries the lane unchanged.
//
// The request slot accepts request or raw; the display slot accepts
// display or raw. A local with conflicting lanes, a parameter outside the
// three names, a selector outside the field set, a call outside the helper
// set, or any other expression is an unrecognized shape and is red (§40.50
// ruling — the census fails loud instead of skipping). Floors keep a
// silently empty scan red. Self-red: swapping (line, display) at any
// operationDispatch site, (expanded, line) at the template dispatch site,
// or handing a literal in the request slot (the round-three #5 shape
// `executeCommandOperationPlan(plan, "/approve", "/approve")`) is red.
func TestCommandOperationRequestDisplayArgumentOrderCensus(t *testing.T) {
	problems, counts := requestDisplayCensus(t, ".")
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("request/display argument-order census: %d problem(s):\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	floors := map[string]int{
		"operationDispatch":                  3, // clarification resume, user-mode arm, RouteOperation arm
		"executeCommandOperationPlan":        3, // validate-and-run, initial auto-execute, /approve
		"executeCommandOperationPlanAttempt": 5,
		"dispatch":                           5, // follow-up replay ×3, template expansion, typed line, one-shot user mode
	}
	for callee, floor := range floors {
		if counts[callee] < floor {
			t.Fatalf("census floor: %d %s call site(s) classified, want ≥ %d (the scan went silently narrow)", counts[callee], callee, floor)
		}
	}
	t.Logf("request/display census: classified call sites per dispatcher = %v", counts)
}

// requestDisplayDispatchers maps each dispatcher to the (request, display)
// argument indices of its call.
var requestDisplayDispatchers = map[string][2]int{
	"operationDispatch":                   {0, 1},
	"executeCommandOperationPlan":         {1, 2},
	"executeCommandOperationPlanAttempt":  {1, 2},
	"dispatch":                            {0, 1},
	"dispatchWithUserMode":                {0, 1},
	"resumeCommandOperationClarification": {0, 1},
}

type textLane int

const (
	laneUnknown textLane = iota
	laneRequest
	laneDisplay
	laneRaw
	// laneSelf marks an expression that resolves back to the identifier
	// being classified (`line = strings.TrimSpace(line)`): it carries the
	// lane unchanged and contributes nothing to a merge.
	laneSelf
)

func (l textLane) String() string {
	switch l {
	case laneRequest:
		return "request"
	case laneDisplay:
		return "display"
	case laneRaw:
		return "raw"
	case laneSelf:
		return "self"
	}
	return "unknown"
}

// requestDisplayCensus classifies every dispatcher call site in dir's
// non-test files and returns the problems plus per-callee site counts.
func requestDisplayCensus(t *testing.T, dir string) ([]string, map[string]int) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var problems []string
	counts := map[string]int{}
	files := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files++
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			c := newLaneClassifier(fset, fd)
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if recv, ok := sel.X.(*ast.Ident); !ok || recv.Name != "r" {
					return true
				}
				idx, ok := requestDisplayDispatchers[sel.Sel.Name]
				if !ok {
					return true
				}
				counts[sel.Sel.Name]++
				site := fmt.Sprintf("%s:%d %s.%s", name, fset.Position(call.Pos()).Line, fd.Name.Name, sel.Sel.Name)
				if len(call.Args) <= idx[1] {
					problems = append(problems, site+": fewer arguments than the (request, display) slots — unrecognized call shape")
					return true
				}
				reqLane, reqWhy := c.lane(call.Args[idx[0]])
				dispLane, dispWhy := c.lane(call.Args[idx[1]])
				if reqLane != laneRequest && reqLane != laneRaw {
					problems = append(problems, fmt.Sprintf("%s: request slot carries %s (%s lane: %s) — want the request text or the raw line", site, exprText(call.Args[idx[0]]), reqLane, reqWhy))
				}
				if dispLane != laneDisplay && dispLane != laneRaw {
					problems = append(problems, fmt.Sprintf("%s: display slot carries %s (%s lane: %s) — want the display form or the raw line", site, exprText(call.Args[idx[1]]), dispLane, dispWhy))
				}
				return true
			})
		}
	}
	if files < 20 {
		t.Fatalf("census scanned %d non-test files in %s, want ≥ 20 (wrong directory?)", files, dir)
	}
	return problems, counts
}

// laneClassifier resolves the lane of an expression inside one function
// by data flow from the producing variable.
type laneClassifier struct {
	fset    *token.FileSet
	fn      *ast.FuncDecl
	params  map[string]textLane
	assigns map[string][]laneAssign // local name → every non-fallback assignment
	memo    map[string]textLane
	memoWhy map[string]string
	active  map[string]bool // names being resolved — a re-entry is a self-reference
	selfHit int             // bumped on every self-reference; a resolution that saw one is not memoized
}

type laneAssign struct {
	rhs   ast.Expr
	index int  // result index for multi-value call assignments, else -1
	add   bool // `x += rhs`
}

func newLaneClassifier(fset *token.FileSet, fn *ast.FuncDecl) *laneClassifier {
	c := &laneClassifier{
		fset:    fset,
		fn:      fn,
		params:  map[string]textLane{},
		assigns: map[string][]laneAssign{},
		memo:    map[string]textLane{},
		memoWhy: map[string]string{},
		active:  map[string]bool{},
	}
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, id := range field.Names {
				switch id.Name {
				case "line":
					c.params[id.Name] = laneRaw
				case "display":
					c.params[id.Name] = laneDisplay
				case "request":
					c.params[id.Name] = laneRequest
				default:
					c.params[id.Name] = laneUnknown
				}
			}
		}
	}
	// parent links so an assignment can be recognized as an
	// empty-fallback fill (`if x == "" { x = … }`).
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range as.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			if isEmptyFallbackFill(id.Name, as, parents) {
				continue
			}
			switch {
			case as.Tok == token.ADD_ASSIGN:
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: as.Rhs[0], index: -1, add: true})
			case len(as.Lhs) == len(as.Rhs):
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: as.Rhs[i], index: -1})
			case len(as.Rhs) == 1:
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: as.Rhs[0], index: i})
			default:
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: nil, index: -1})
			}
		}
		return true
	})
	return c
}

// isEmptyFallbackFill reports whether as is `name = …` directly inside the
// body of `if name == "" { … }` or `if strings.TrimSpace(name) == "" { … }`
// — a fill of an empty value that never widens the variable's lane.
func isEmptyFallbackFill(name string, as *ast.AssignStmt, parents map[ast.Node]ast.Node) bool {
	if as.Tok != token.ASSIGN {
		return false
	}
	block, ok := parents[as].(*ast.BlockStmt)
	if !ok {
		return false
	}
	ifStmt, ok := parents[block].(*ast.IfStmt)
	if !ok || ifStmt.Body != block {
		return false
	}
	cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.EQL {
		return false
	}
	lit, ok := cond.Y.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING || lit.Value != `""` {
		return false
	}
	x := cond.X
	if call, ok := x.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "strings" && fun.Sel.Name == "TrimSpace" {
				x = call.Args[0]
			}
		}
	}
	id, ok := x.(*ast.Ident)
	return ok && id.Name == name
}

// lane classifies expr; the second result explains the classification
// for the census message.
func (c *laneClassifier) lane(expr ast.Expr) (textLane, string) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return laneDisplay, "string literal is a display form"
		}
		return laneUnknown, "non-string literal"
	case *ast.ParenExpr:
		return c.lane(v.X)
	case *ast.Ident:
		return c.identLane(v.Name)
	case *ast.SelectorExpr:
		switch v.Sel.Name {
		case "RequestText", "OriginalLine":
			return laneRequest, "." + v.Sel.Name + " selector"
		case "Display":
			return laneDisplay, ".Display selector"
		case "Text":
			return laneRaw, ".Text selector (a line as typed)"
		}
		return laneUnknown, "selector ." + v.Sel.Name + " is outside the request/display field set"
	case *ast.CallExpr:
		switch fun := v.Fun.(type) {
		case *ast.Ident:
			switch fun.Name {
			case "commandOperationFollowupRequestText":
				return laneRequest, "commandOperationFollowupRequestText call"
			case "followUpDisplayText":
				return laneDisplay, "followUpDisplayText call"
			}
			return laneUnknown, "call " + fun.Name + " is outside the request/display helper set"
		case *ast.SelectorExpr:
			if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "strings" && len(v.Args) >= 1 {
				switch fun.Sel.Name {
				case "TrimSpace", "TrimPrefix", "TrimSuffix":
					// trims carry the lane of the text they trim
					return c.lane(v.Args[0])
				}
			}
			return laneUnknown, "call " + exprText(fun) + " is outside the request/display helper set"
		}
		return laneUnknown, "call of an unrecognized shape"
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return laneUnknown, "binary " + v.Op.String()
		}
		return c.mergeLanes([]ast.Expr{v.X, v.Y})
	}
	return laneUnknown, fmt.Sprintf("%T is an unrecognized shape", expr)
}

// mergeLanes folds operand lanes: string literals contribute nothing, raw
// folds into request, and any other mix is unknown.
func (c *laneClassifier) mergeLanes(operands []ast.Expr) (textLane, string) {
	result := laneUnknown
	why := "concatenation of literals only"
	for _, op := range operands {
		if lit, ok := op.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			continue
		}
		l, w := c.lane(op)
		switch {
		case l == laneSelf:
			continue // the identifier under resolution itself: lane unchanged
		case l == laneUnknown:
			return laneUnknown, w
		case result == laneUnknown || result == l:
			result, why = l, w
		case result == laneRaw && l == laneRequest, result == laneRequest && l == laneRaw:
			result, why = laneRequest, "request text concatenated with a raw line"
		default:
			return laneUnknown, fmt.Sprintf("concatenation mixes %s and %s lanes", result, l)
		}
	}
	return result, why
}

func (c *laneClassifier) identLane(name string) (textLane, string) {
	if l, ok := c.memo[name]; ok {
		return l, c.memoWhy[name]
	}
	if c.active[name] {
		c.selfHit++
		return laneSelf, "self-reference to " + name
	}
	c.active[name] = true
	defer delete(c.active, name)
	before := c.selfHit
	lane, why := c.resolveIdent(name)
	// A resolution that ran into an outer name's self-reference (`cleaned`
	// resolved while `line` was active) is relative to that outer name and
	// must not be memoized as this name's own lane.
	if c.selfHit == before {
		c.memo[name], c.memoWhy[name] = lane, why
	}
	return lane, why
}

func (c *laneClassifier) resolveIdent(name string) (textLane, string) {
	assigns := c.assigns[name]
	// The parameter lane seeds the merge: a parameter that is only ever
	// re-assigned from itself or filled when empty keeps its lane.
	result := laneUnknown
	why := ""
	if l, ok := c.params[name]; ok {
		if l == laneUnknown {
			return laneUnknown, "parameter " + name + " is outside the line/display/request names"
		}
		result, why = l, "parameter "+name
	} else if len(assigns) == 0 {
		return laneUnknown, "identifier " + name + " has no assignment in " + c.fn.Name.Name + " and is not a parameter"
	}
	sawSelf := false
	for _, a := range assigns {
		var l textLane
		var w string
		switch {
		case a.rhs == nil:
			return laneUnknown, name + " is assigned through an unrecognized multi-value shape"
		case a.index >= 0:
			l, w = c.resultLane(a.rhs, a.index)
		default:
			// `x = rhs` and `x += rhs` alike: rhs's lane merges below with
			// the lane the earlier assignments established.
			l, w = c.lane(a.rhs)
		}
		switch {
		case l == laneSelf:
			sawSelf = true // `x = f(x)` re-assignment: lane unchanged
			continue
		case l == laneUnknown:
			return laneUnknown, name + " ← " + w
		case result == laneUnknown || result == l:
			result, why = l, name+" ← "+w
		case result == laneRaw && l == laneRequest, result == laneRequest && l == laneRaw:
			result, why = laneRequest, name+" ← request text and a raw line"
		default:
			return laneUnknown, fmt.Sprintf("%s is assigned %s and %s lanes in %s", name, result, l, c.fn.Name.Name)
		}
	}
	if result == laneUnknown {
		if sawSelf {
			// every producing assignment resolved back to the name under
			// resolution (`cleaned, _ := splitPastedLog(line)` while
			// resolving line): this local is that name's own text
			return laneSelf, name + " derives from the name under resolution"
		}
		return laneUnknown, name + " has only empty-fill assignments in " + c.fn.Name.Name + " and no producing one"
	}
	return result, why
}

// resultLane classifies result index i of a multi-value call assignment.
func (c *laneClassifier) resultLane(rhs ast.Expr, i int) (textLane, string) {
	call, ok := rhs.(*ast.CallExpr)
	if !ok {
		return laneUnknown, "multi-value assignment from a non-call"
	}
	if fun, ok := call.Fun.(*ast.Ident); ok {
		// splitPastedLog(x) → (cleaned, detected): the cleaned text is x
		// minus the auto-detected log and keeps x's lane.
		if fun.Name == "splitPastedLog" && i == 0 && len(call.Args) == 1 {
			return c.lane(call.Args[0])
		}
		return laneUnknown, "multi-value call " + fun.Name + " is outside the request/display producer set"
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return laneUnknown, "multi-value call of an unrecognized shape"
	}
	if recv, ok := sel.X.(*ast.Ident); !ok || recv.Name != "r" {
		return laneUnknown, "multi-value call " + exprText(call.Fun) + " is outside the request/display producer set"
	}
	switch {
	case sel.Sel.Name == "readInputPair" && i == 0:
		return laneRaw, "readInputPair line result"
	case sel.Sel.Name == "readInputPair" && i == 1:
		return laneDisplay, "readInputPair display result"
	case sel.Sel.Name == "expandTemplateCommand" && i == 0:
		return laneRequest, "expandTemplateCommand expanded result"
	}
	return laneUnknown, fmt.Sprintf("result %d of r.%s is outside the request/display producer set", i, sel.Sel.Name)
}

func exprText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.BasicLit:
		return v.Value
	case *ast.SelectorExpr:
		return exprText(v.X) + "." + v.Sel.Name
	case *ast.CallExpr:
		return exprText(v.Fun) + "(…)"
	case *ast.ParenExpr:
		return "(" + exprText(v.X) + ")"
	case *ast.BinaryExpr:
		return exprText(v.X) + " " + v.Op.String() + " " + exprText(v.Y)
	}
	return fmt.Sprintf("%T", e)
}
