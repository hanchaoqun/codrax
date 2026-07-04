package tool

// trace_query_dominant_state_pin_test.go — TSH review F2: the cross-package
// consumer pin for tracequery DominantState wire words inside this package's
// trace_query.go. The two drilldown-plan switches
// (traceQueryCausalImpactRecommendedViews / traceQueryCausalImpactNeedsChain)
// consumed the dominant-lane tokens behind a silent default with NO pin
// scanning them — deleting either switch entirely left every test green, and
// the RN-11 (§7.9) runnable re-pointing shipped into their tracequery twins
// (stateDrilldownRecommendedViews / stateDrilldownNeedsWakeupChain, 7c5c236d)
// while this copy was missed. Rules:
//
//  1. Site golden: every switch whose case list names tracequery.State*
//     members is pinned (exact handled set in dominant-lane priority order +
//     default marker) — deleting a case or the whole switch is red even
//     though both switches carry defaults.
//  2. Coverage ∪ ledger == dominant-lane universe: every
//     tracequery.ThreadStateDominantLaneUniverse() member must be handled or
//     declared in the fall-through ledger with a rationale, default or not —
//     the RN-11 divergence lives in the ledger as reviewable text, never as
//     silence behind a default.
//  3. Comparison lane: every ==/!= comparison against a tracequery.State*
//     member in the same file is registered per function (member set +
//     count) — traceQueryCausalImpactRecursive's RN-11 runnable branch lives
//     here.
//
// matched==0 fatals on both lanes. Test files excluded (fixture literals),
// same policy as the tracequery/types/tool sister pins.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// traceQueryDominantStatePinFiles is the scanned consumer surface: the
// trace_query producer file that maps DominantState words to drilldown plans.
var traceQueryDominantStatePinFiles = []string{"trace_query.go"}

type traceQueryDominantStateFallthroughDecl struct {
	missing string // comma-joined members in dominant-lane priority order
	why     string
}

var traceQueryDominantStateSwitchFallthroughLedger = map[string]traceQueryDominantStateFallthroughDecl{
	"trace_query.go:traceQueryCausalImpactNeedsChain#1": {
		missing: "runnable,running",
		why: "runnable: RN-11 (§7.9) — a runnable-dominant node is CPU competition, not a wakeup dependency; " +
			"the engine twin stateDrilldownNeedsWakeupChain dropped it in 7c5c236d and this copy was the missed " +
			"twin (re-synced by TSH F2, 2026-07-05). running: occupancy — never a chain target; the " +
			"perf/compute-supply surfaces own it (same absence as the engine twin since birth, 9d4e6958).",
	},
}

// traceQueryDominantStateSwitchSiteGolden pins each switch's handled member
// set (dominant-lane priority order: io_wait,d_sleep,runnable,s_sleep,running)
// plus the default marker.
var traceQueryDominantStateSwitchSiteGolden = map[string]string{
	"trace_query.go:traceQueryCausalImpactRecommendedViews#1": "io_wait,d_sleep,runnable,s_sleep,running|default",
	"trace_query.go:traceQueryCausalImpactNeedsChain#1":       "io_wait,d_sleep,s_sleep|default",
}

// traceQueryDominantStateComparisonSiteGolden pins the per-function ==/!=
// comparison surface against tracequery.State* members in the same file.
var traceQueryDominantStateComparisonSiteGolden = map[string]string{
	"trace_query.go:traceQueryCausalImpactRecursive": "runnable#1",
}

type traceQueryDominantStateSwitchSite struct {
	key        string
	pos        string
	handled    map[tracequery.ThreadState]bool
	hasDefault bool
}

type traceQueryDominantStateComparisonSite struct {
	key     string
	members map[tracequery.ThreadState]bool
	count   int
}

func TestTraceQueryDominantStateSwitchConsumerCoverage(t *testing.T) {
	lanes := tracequery.ThreadStateDominantLaneUniverse()
	if len(lanes) == 0 {
		t.Fatal("empty dominant-lane universe — the pin is checking nothing")
	}
	sites, comparisons := collectTraceQueryDominantStateSites(t)
	if len(sites) == 0 {
		t.Fatal("no DominantState consumer switches matched — the pin is checking nothing; update the scan alongside the refactor")
	}
	if len(comparisons) == 0 {
		t.Fatal("no DominantState comparison expressions matched — the pin is checking nothing; update the scan alongside the refactor")
	}

	// Rule 1: exact site goldens, both lanes.
	gotSwitches := map[string]string{}
	for _, site := range sites {
		gotSwitches[site.key] = renderDominantLaneMembers(lanes, site.handled) + map[bool]string{true: "|default", false: ""}[site.hasDefault]
	}
	if !reflect.DeepEqual(gotSwitches, traceQueryDominantStateSwitchSiteGolden) {
		t.Errorf("DominantState switch sites drifted from the golden — review every change (a lost case is a silent drilldown-plan change), then update. Current scan:\n%s", renderDominantStateScan(gotSwitches))
	}
	gotComparisons := map[string]string{}
	for _, site := range comparisons {
		gotComparisons[site.key] = fmt.Sprintf("%s#%d", renderDominantLaneMembers(lanes, site.members), site.count)
	}
	if !reflect.DeepEqual(gotComparisons, traceQueryDominantStateComparisonSiteGolden) {
		t.Errorf("DominantState comparison sites drifted from the golden — review every change, then update. Current scan:\n%s", renderDominantStateScan(gotComparisons))
	}

	// Rule 2: handled ∪ ledgered == dominant-lane universe for EVERY switch,
	// default or not — a member routed to the default without a ledger row is
	// exactly the unregistered RN-11 divergence this pin exists to prevent.
	known := map[string]bool{}
	for _, site := range sites {
		known[site.key] = true
		decl, hasDecl := traceQueryDominantStateSwitchFallthroughLedger[site.key]
		declared := map[tracequery.ThreadState]bool{}
		if hasDecl {
			if strings.TrimSpace(decl.why) == "" {
				t.Errorf("%s: ledger entry has no rationale", site.key)
			}
			for _, name := range strings.Split(decl.missing, ",") {
				member := tracequery.ThreadState(strings.TrimSpace(name))
				if !dominantLaneHas(lanes, member) {
					t.Errorf("%s: ledger declares %q which is not a dominant-lane member", site.key, name)
					continue
				}
				if site.handled[member] {
					t.Errorf("%s (%s): ledger declares %q as fall-through but the switch HANDLES it — stale ledger row", site.key, site.pos, member)
				}
				declared[member] = true
			}
		}
		for _, member := range lanes {
			if site.handled[member] || declared[member] {
				continue
			}
			t.Errorf("%s (%s): dominant-lane member %q is neither handled nor ledgered — add the case or a ledger declaration with rationale (default alone is not an accounting)", site.key, site.pos, member)
		}
	}
	for key := range traceQueryDominantStateSwitchFallthroughLedger {
		if !known[key] {
			t.Errorf("fall-through ledger entry %q matches no scanned switch — remove or rekey it", key)
		}
	}
}

func dominantLaneHas(lanes []tracequery.ThreadState, member tracequery.ThreadState) bool {
	for _, lane := range lanes {
		if lane == member {
			return true
		}
	}
	return false
}

func renderDominantLaneMembers(lanes []tracequery.ThreadState, handled map[tracequery.ThreadState]bool) string {
	var parts []string
	for _, lane := range lanes {
		if handled[lane] {
			parts = append(parts, string(lane))
		}
	}
	return strings.Join(parts, ",")
}

func renderDominantStateScan(got map[string]string) string {
	var lines []string
	for key, value := range got {
		lines = append(lines, fmt.Sprintf("\t%q: %q,", key, value))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// traceQueryDominantStateMemberFromExpr extracts the ThreadState named by a
// qualified member expression: tracequery.StateX or string(tracequery.StateX).
func traceQueryDominantStateMemberFromExpr(expr ast.Expr) (tracequery.ThreadState, bool) {
	selToState := map[string]tracequery.ThreadState{
		"StateRunning":  tracequery.StateRunning,
		"StateRunnable": tracequery.StateRunnable,
		"StateSSleep":   tracequery.StateSSleep,
		"StateDSleep":   tracequery.StateDSleep,
		"StateIOWait":   tracequery.StateIOWait,
		"StateStopped":  tracequery.StateStopped,
		"StateDead":     tracequery.StateDead,
		"StateUnknown":  tracequery.StateUnknown,
	}
	fromSelector := func(e ast.Expr) (tracequery.ThreadState, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "tracequery" {
			return "", false
		}
		state, ok := selToState[sel.Sel.Name]
		return state, ok
	}
	if state, ok := fromSelector(expr); ok {
		return state, true
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "string" || len(call.Args) != 1 {
		return "", false
	}
	return fromSelector(call.Args[0])
}

func collectTraceQueryDominantStateSites(t *testing.T) ([]traceQueryDominantStateSwitchSite, []traceQueryDominantStateComparisonSite) {
	t.Helper()
	var switches []traceQueryDominantStateSwitchSite
	var comparisons []traceQueryDominantStateComparisonSite
	for _, name := range traceQueryDominantStatePinFiles {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ordinal := 0
			compare := traceQueryDominantStateComparisonSite{
				key:     fmt.Sprintf("%s:%s", name, fn.Name.Name),
				members: map[tracequery.ThreadState]bool{},
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.SwitchStmt:
					handled := map[tracequery.ThreadState]bool{}
					hasDefault := false
					for _, stmt := range node.Body.List {
						clause, ok := stmt.(*ast.CaseClause)
						if !ok {
							continue
						}
						if clause.List == nil {
							hasDefault = true
							continue
						}
						for _, expr := range clause.List {
							if state, ok := traceQueryDominantStateMemberFromExpr(expr); ok {
								handled[state] = true
							}
						}
					}
					if len(handled) == 0 {
						return true
					}
					ordinal++
					switches = append(switches, traceQueryDominantStateSwitchSite{
						key:        fmt.Sprintf("%s:%s#%d", name, fn.Name.Name, ordinal),
						pos:        fset.Position(node.Pos()).String(),
						handled:    handled,
						hasDefault: hasDefault,
					})
				case *ast.BinaryExpr:
					if node.Op != token.EQL && node.Op != token.NEQ {
						return true
					}
					for _, operand := range []ast.Expr{node.X, node.Y} {
						if state, ok := traceQueryDominantStateMemberFromExpr(operand); ok {
							compare.members[state] = true
							compare.count++
							break
						}
					}
				}
				return true
			})
			if compare.count > 0 {
				comparisons = append(comparisons, compare)
			}
		}
	}
	return switches, comparisons
}
