package tracequery

// thread_state_universe_pin_test.go — TSH batch bidirectional ThreadState pin
// (memory reaudit P1-1; NEW-7 dynamic-legend model: emit points ⊆ registry AND
// registry ⊆ producible set).
//
//   - TestThreadStateUniverseGolden: the universe list, the const block in
//     types.go and the hardcoded wire golden must be the SAME closed set;
//     the dominant-lane sub-universe is pinned in priority order.
//   - TestThreadStateProductionWitness: every universe member has a witnessed
//     production point (stateFromPrevState code table + live timeline probes
//     for the running interval mint and the D→io_wait blocked-reason
//     reclassification). A member nobody can produce is a phantom — red.
//   - TestThreadStateSwitchConsumerCoverage: every switch in this package
//     whose case list names ThreadState members must cover the universe,
//     carry an explicit default, or declare its fall-through members in
//     threadStateSwitchFallthroughLedger with a rationale — the mechanical
//     form of "adding a state class must visit every consumer" (§7.11 B-1
//     shipped StateStopped/StateDead by hand-patching each switch; one missed
//     copy would have been a silent misclassification). The site golden also
//     pins the handled sets of default-carrying switches, so deleting a case
//     is red even where a default would swallow it.
//   - TestThreadStateComparisonConsumerCoverage (TSH review F3b): the SECOND
//     consumer lane — every ==/!= comparison against a ThreadState member
//     (bare State* ident or string(State*) conversion) in this package's
//     non-test files, aggregated per function into a literal golden (member
//     set + comparison count). The if-comparison surface is as real as the
//     switch surface (the reopen-guard twins lived here) and was previously
//     outside the pin; tampering one member of one comparison is now a
//     golden drift. matched==0 fatals, same as the sister pins in
//     internal/types / internal/tool.
//   - TestComputeSupplyMintProductionWitness (TSH review F4): the compute-
//     supply mint sites (add(td, string(State*)) in computeSupplySummaries)
//     mint dominant-lane wire tokens only, and computeSupplyDominantState
//     passes each minted lane word through verbatim — the path that reaches
//     RootCauseRankItem.DominantState via the root_cause_rank compute-supply
//     lane, so a raw or off-universe mint literal goes red here.
//
// Precise signals only: typed constants, AST case-expression identifiers,
// exact set equality. Test files are excluded from the scan (they carry
// fixture literals on purpose), same policy as the NKR/causal-token pins.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestThreadStateUniverseGolden(t *testing.T) {
	golden := []ThreadState{
		StateRunning,
		StateRunnable,
		StateSSleep,
		StateDSleep,
		StateIOWait,
		StateStopped,
		StateDead,
		StateUnknown,
	}
	if !reflect.DeepEqual(threadStateUniverse, golden) {
		t.Fatalf("threadStateUniverse drifted from the golden member list:\n got %v\nwant %v", threadStateUniverse, golden)
	}
	wire := []string{"running", "runnable", "s_sleep", "d_sleep", "io_wait", "stopped", "dead", "unknown"}
	for i, member := range golden {
		if string(member) != wire[i] {
			t.Errorf("universe member %d wire value drifted: got %q want %q (renaming a ThreadState is a wire-format change)", i, member, wire[i])
		}
	}

	// AST parity: the const block in types.go and the universe list must name
	// the same closed set — a constant added without a universe entry (or the
	// reverse) is exactly the "human memory sync" failure this pin removes.
	constValues := threadStateConstWireValues(t)
	sortedWire := append([]string(nil), wire...)
	sort.Strings(constValues)
	sort.Strings(sortedWire)
	if !reflect.DeepEqual(constValues, sortedWire) {
		t.Fatalf("types.go ThreadState const block and threadStateUniverse diverged:\n consts %v\n universe %v", constValues, sortedWire)
	}

	// Dominant-lane sub-universe: exact members AND exact priority order (the
	// order IS the tie-break of the shared dominant pick).
	expectedLanes := []ThreadState{StateIOWait, StateDSleep, StateRunnable, StateSSleep, StateRunning}
	if !reflect.DeepEqual(threadStateDominantLanes, expectedLanes) {
		t.Fatalf("threadStateDominantLanes drifted (members or PRIORITY ORDER):\n got %v\nwant %v", threadStateDominantLanes, expectedLanes)
	}
	if !reflect.DeepEqual(ThreadStateDominantLaneUniverse(), expectedLanes) {
		t.Fatalf("ThreadStateDominantLaneUniverse must expose the same lanes")
	}
}

// threadStateConstWireValues extracts the string values of every ThreadState
// constant declared in types.go.
func threadStateConstWireValues(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "types.go", nil, 0)
	if err != nil {
		t.Fatalf("parse types.go: %v", err)
	}
	var out []string
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
			typeIdent, ok := vs.Type.(*ast.Ident)
			if !ok || typeIdent.Name != "ThreadState" {
				continue
			}
			for _, value := range vs.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				raw, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote ThreadState const value %s: %v", lit.Value, err)
				}
				out = append(out, raw)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no ThreadState constants found in types.go — the parity pin is checking nothing")
	}
	return out
}

func TestThreadStateProductionWitness(t *testing.T) {
	produced := map[ThreadState]bool{}

	// Parse-face production: the prev_state code table (§7.11 B-1 single-char
	// extensions included). Golden per code family; drift here is a parse-face
	// behavior change, not a test refresh.
	codeGolden := map[string]ThreadState{
		"D":   StateDSleep,
		"D|K": StateDSleep,
		"S":   StateSSleep,
		"R":   StateRunnable,
		"R+":  StateRunnable,
		"I":   StateSSleep, // TASK_IDLE books to the interruptible-sleep family
		"I|K": StateSSleep,
		"T":   StateStopped,
		"t":   StateStopped,
		"X":   StateDead,
		"Z":   StateDead,
		"":    StateUnknown,
		"??":  StateUnknown,
	}
	for code, want := range codeGolden {
		got := stateFromPrevState(code)
		if got != want {
			t.Errorf("stateFromPrevState(%q) = %q, want %q", code, got, want)
		}
		produced[got] = true
	}

	// Interval-face production: StateRunning is minted by the timeline
	// builders (never by stateFromPrevState) and StateIOWait by the
	// blocked-reason D→io_wait reclassification — witness both live.
	idx := buildTraceIndex(t, "thread_state_production.systrace", `
        svc-9 (    9) [000] .... 1.000000: sched_switch: prev_comm=svc prev_pid=9 prev_prio=120 prev_state=S ==> next_comm=app next_pid=100 next_prio=120
        app-100 (  100) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=D ==> next_comm=svc next_pid=9 next_prio=120
        svc-9 (    9) [000] .... 1.012000: sched_blocked_reason: pid=100 iowait=1 caller=blkdev_issue_rw
        svc-9 (    9) [000] .... 1.020000: sched_wakeup: comm=app pid=100 prio=120 target_cpu=000
        svc-9 (    9) [000] .... 1.030000: sched_switch: prev_comm=svc prev_pid=9 prev_prio=120 prev_state=S ==> next_comm=app next_pid=100 next_prio=120
        app-100 (  100) [000] .... 1.040000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=svc next_pid=9 next_prio=120
	`)
	tl := ThreadTimeline(idx, Query{PID: 100})
	var sawRunning, sawIOWait bool
	for _, iv := range tl.Intervals {
		produced[iv.State] = true
		switch iv.State {
		case StateRunning:
			sawRunning = true
		case StateIOWait:
			sawIOWait = true
		}
	}
	if !sawRunning {
		t.Fatalf("fixture drift: the timeline must mint a StateRunning interval: %+v", tl.Intervals)
	}
	if !sawIOWait {
		t.Fatalf("fixture drift: the D interval with an iowait=1 sched_blocked_reason must reclassify to StateIOWait: %+v", tl.Intervals)
	}

	for _, member := range threadStateUniverse {
		if !produced[member] {
			t.Errorf("universe member %q has NO witnessed production point — a state class nobody can produce is a phantom; add a parse/interval witness or remove the member", member)
		}
	}
}

// threadStateSwitchSite is one consumer switch whose case list names
// ThreadState members.
type threadStateSwitchSite struct {
	key        string // file:enclosingFunc#ordinal
	pos        string
	handled    map[ThreadState]bool
	hasDefault bool
}

// threadStateFallthroughDecl is one no-default switch's EXPLICIT declaration
// of the universe members it deliberately lets fall through, with rationale.
// This is the typed escape lane (§1.6): silence is red, a declared skip is
// green and reviewable.
type threadStateFallthroughDecl struct {
	missing string // comma-joined member names in universe order
	why     string
}

var threadStateSwitchFallthroughLedger = map[string]threadStateFallthroughDecl{
	"query.go:computeOffCPUStats#1": {
		missing: "running,stopped,dead,unknown",
		why:     "off-CPU wait lanes only: running is on-CPU by definition; stopped/dead/unknown own no lane (§7.11 B-1)",
	},
	"query.go:computeOffCPUStats#2": {
		missing: "running,stopped,dead,unknown",
		why:     "same off-CPU lane booking as #1 (switch-in close path)",
	},
	"query.go:computeOffCPUStats#3": {
		missing: "running,stopped,dead,unknown",
		why:     "same off-CPU lane booking as #1 (window-end flush path)",
	},
	"query.go:addStateChurnInterval#1": {
		missing: "stopped,dead,unknown",
		why:     "churn five-lane accumulator; stopped/dead/unknown never open a segment (stateChurnOpenIneligible upstream, §7.11 B-1 sequel)",
	},
	"query.go:applySemanticTraceSpanState#1": {
		missing: "stopped,dead,unknown",
		why:     "semantic-span state attribution rides the five published per-state ms fields only",
	},
	"query.go:summarizeThreadStateBreakdown#1": {
		missing: "stopped,dead,unknown",
		why:     "五-lane peer breakdown: stopped/dead intervals still book TotalMs/fragments honestly but own no per-state lane (§7.11 B-1)",
	},
	"query.go:summarizeWakeupCausalImpact#1": {
		missing: "stopped,dead,unknown",
		why:     "five-lane causal-impact accumulation; stopped/dead intervals book TotalMs/fragments only (§7.11 B-1)",
	},
	"query.go:priorityInversionGatedMs#1": {
		missing: "s_sleep,d_sleep,io_wait,stopped,dead,unknown",
		why:     "R5d ruling (§7.30.1): inversion impact counts ONLY runnable time plus weak-core running deficit; the dependency's own sleep/D/IO is its own upstream problem",
	},
	"stream_search.go:addStreamStateClusterInterval#1": {
		missing: "stopped,dead,unknown",
		why:     "streaming twin of addStateChurnInterval — same five-lane booking, same upstream open gate",
	},
	"thread_state_universe.go:stateChurnOpenIneligible#1": {
		missing: "running,runnable,s_sleep,d_sleep,io_wait",
		why:     "the five active lanes ARE eligible (return false); only unknown/stopped/dead are rejected",
	},
	"thread_state_universe.go:dominantStateFromLanes#1": {
		missing: "stopped,dead,unknown",
		why:     "dominant pick reads the five lane accumulators; stopped/dead/unknown own no lane (§7.11 B-1)",
	},
	"thread_state_universe.go:dominantStateIsDStateOrIOWait#1": {
		missing: "running,runnable,s_sleep,stopped,dead,unknown",
		why:     "membership test for the uninterruptible/IO blocking family only",
	},
}

// threadStateSwitchSiteGolden pins every consumer switch's handled member set
// (universe order) and default marker: "<members>" or "<members>|default".
// Deleting a case is red HERE even when the switch carries a default.
var threadStateSwitchSiteGolden = map[string]string{
	"query.go:computeOffCPUStats#1":                            "runnable,s_sleep,d_sleep,io_wait",
	"query.go:computeOffCPUStats#2":                            "runnable,s_sleep,d_sleep,io_wait",
	"query.go:computeOffCPUStats#3":                            "runnable,s_sleep,d_sleep,io_wait",
	"query.go:addStateChurnInterval#1":                         "running,runnable,s_sleep,d_sleep,io_wait",
	"query.go:stateChurnNextStep#1":                            "running,runnable,s_sleep,d_sleep,io_wait|default",
	"query.go:stateChurnNextStepKind#1":                        "running,runnable,s_sleep,d_sleep,io_wait|default",
	"query.go:stateDrilldownRecommendedViews#1":                "running,runnable,s_sleep,d_sleep,io_wait|default",
	"query.go:stateDrilldownNeedsWakeupChain#1":                "s_sleep,d_sleep,io_wait|default",
	"query.go:applySemanticTraceSpanState#1":                   "running,runnable,s_sleep,d_sleep,io_wait",
	"query.go:actualAggregateBlockingMs#1":                     "running,runnable,s_sleep,d_sleep,io_wait|default",
	"query.go:aggregateRootCauseIsPrioritySensitive#1":         "runnable,d_sleep,io_wait|default",
	"query.go:stateChurnRootCauseType#1":                       "running,runnable,s_sleep,d_sleep,io_wait|default",
	"query.go:computeSupplyDominantState#1":                    "running,runnable|default",
	"query.go:rootCauseItemHasDStateOrIO#1":                    "d_sleep,io_wait|default",
	"query.go:rootCauseItemHasRunnableOrRunning#1":             "running,runnable|default",
	"query.go:summarizeThreadStateBreakdown#1":                 "running,runnable,s_sleep,d_sleep,io_wait",
	"query.go:expandChain#1":                                   "running,runnable,s_sleep,d_sleep,io_wait|default",
	"query.go:summarizeWakeupCausalImpact#1":                   "running,runnable,s_sleep,d_sleep,io_wait",
	"query.go:priorityInversionGatedMs#1":                      "running,runnable",
	"query.go:actualCausalImpactBlockingMs#1":                  "running,runnable,s_sleep,d_sleep,io_wait|default",
	"query.go:interestingIntervals#1":                          "running,runnable,s_sleep,d_sleep,io_wait|default",
	"stream_search.go:addStreamStateClusterInterval#1":         "running,runnable,s_sleep,d_sleep,io_wait",
	"thread_state_universe.go:stateChurnOpenIneligible#1":      "stopped,dead,unknown",
	"thread_state_universe.go:dominantStateFromLanes#1":        "running,runnable,s_sleep,d_sleep,io_wait",
	"thread_state_universe.go:dominantStateIsDStateOrIOWait#1": "d_sleep,io_wait",
	"thread_state_universe.go:rootTypeForDominantState#1":      "running,runnable,s_sleep,d_sleep,io_wait|default",
}

func TestThreadStateSwitchConsumerCoverage(t *testing.T) {
	sites := collectThreadStateSwitchSites(t)
	if len(sites) == 0 {
		t.Fatal("no ThreadState consumer switches matched — the pin is checking nothing; update the scan alongside the refactor")
	}

	// Golden direction: exact handled-set + default-marker parity.
	got := map[string]string{}
	for _, site := range sites {
		got[site.key] = renderThreadStateMembers(site.handled) + map[bool]string{true: "|default", false: ""}[site.hasDefault]
	}
	if !reflect.DeepEqual(got, threadStateSwitchSiteGolden) {
		var lines []string
		for key, value := range got {
			lines = append(lines, fmt.Sprintf("\t%q: %q,", key, value))
		}
		sort.Strings(lines)
		t.Errorf("consumer switch sites drifted from threadStateSwitchSiteGolden — review EVERY change (a lost case is a silent misclassification), then update the golden. Current scan:\n%s", strings.Join(lines, "\n"))
	}

	// Universe direction: every no-default switch must handle or explicitly
	// ledger every universe member.
	for _, site := range sites {
		decl, hasDecl := threadStateSwitchFallthroughLedger[site.key]
		if site.hasDefault {
			if hasDecl {
				t.Errorf("%s (%s): has an explicit default AND a fall-through ledger entry — remove the stale ledger row", site.key, site.pos)
			}
			continue
		}
		declared := map[ThreadState]bool{}
		if hasDecl {
			for _, name := range strings.Split(decl.missing, ",") {
				member := ThreadState(strings.TrimSpace(name))
				if !threadStateUniverseHas(member) {
					t.Errorf("%s: ledger declares unknown member %q", site.key, name)
					continue
				}
				if site.handled[member] {
					t.Errorf("%s (%s): ledger declares %q as fall-through but the switch HANDLES it — stale ledger row", site.key, site.pos, member)
				}
				declared[member] = true
			}
		}
		for _, member := range threadStateUniverse {
			if site.handled[member] || declared[member] {
				continue
			}
			t.Errorf("%s (%s): universe member %q is neither handled nor declared as deliberate fall-through — add the case or a threadStateSwitchFallthroughLedger declaration with rationale", site.key, site.pos, member)
		}
	}

	// Ledger hygiene: no orphan entries for vanished sites.
	known := map[string]bool{}
	for _, site := range sites {
		known[site.key] = true
	}
	for key := range threadStateSwitchFallthroughLedger {
		if !known[key] {
			t.Errorf("threadStateSwitchFallthroughLedger entry %q matches no scanned switch — remove or rekey it", key)
		}
	}
}

func threadStateUniverseHas(member ThreadState) bool {
	for _, m := range threadStateUniverse {
		if m == member {
			return true
		}
	}
	return false
}

func renderThreadStateMembers(handled map[ThreadState]bool) string {
	var parts []string
	for _, member := range threadStateUniverse {
		if handled[member] {
			parts = append(parts, string(member))
		}
	}
	return strings.Join(parts, ",")
}

// threadStateMemberFromExpr extracts the ThreadState named by one expression:
// a bare State* identifier or a string(State*) conversion. Shared by the
// switch-case lane, the comparison lane and the compute-supply mint witness.
func threadStateMemberFromExpr(expr ast.Expr) (ThreadState, bool) {
	identToState := map[string]ThreadState{
		"StateRunning":  StateRunning,
		"StateRunnable": StateRunnable,
		"StateSSleep":   StateSSleep,
		"StateDSleep":   StateDSleep,
		"StateIOWait":   StateIOWait,
		"StateStopped":  StateStopped,
		"StateDead":     StateDead,
		"StateUnknown":  StateUnknown,
	}
	switch e := expr.(type) {
	case *ast.Ident:
		state, ok := identToState[e.Name]
		return state, ok
	case *ast.CallExpr:
		fn, ok := e.Fun.(*ast.Ident)
		if !ok || fn.Name != "string" || len(e.Args) != 1 {
			return "", false
		}
		arg, ok := e.Args[0].(*ast.Ident)
		if !ok {
			return "", false
		}
		state, ok := identToState[arg.Name]
		return state, ok
	}
	return "", false
}

func collectThreadStateSwitchSites(t *testing.T) []threadStateSwitchSite {
	t.Helper()
	caseExprState := threadStateMemberFromExpr

	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(names)
	var sites []threadStateSwitchSite
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
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
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sw, ok := n.(*ast.SwitchStmt)
				if !ok {
					return true
				}
				handled := map[ThreadState]bool{}
				hasDefault := false
				for _, stmt := range sw.Body.List {
					clause, ok := stmt.(*ast.CaseClause)
					if !ok {
						continue
					}
					if clause.List == nil {
						hasDefault = true
						continue
					}
					for _, expr := range clause.List {
						if state, ok := caseExprState(expr); ok {
							handled[state] = true
						}
					}
				}
				if len(handled) == 0 {
					return true
				}
				ordinal++
				sites = append(sites, threadStateSwitchSite{
					key:        fmt.Sprintf("%s:%s#%d", name, fn.Name.Name, ordinal),
					pos:        fset.Position(sw.Pos()).String(),
					handled:    handled,
					hasDefault: hasDefault,
				})
				return true
			})
		}
	}
	return sites
}

// ---- comparison lane (TSH review F3b) --------------------------------------

// threadStateComparisonSite aggregates, per enclosing function, every ==/!=
// comparison whose operand names a ThreadState member.
type threadStateComparisonSite struct {
	key     string // file:enclosingFunc
	pos     string // first comparison position
	members map[ThreadState]bool
	count   int // number of ==/!= expressions naming a member
}

// threadStateComparisonSiteGolden pins the per-function comparison surface:
// "<members in universe order>#<comparison count>". Registered from the real
// AST scan (comments are invisible here — the review's grep count included a
// doc-comment line, TSH F8); tampering one member or adding/removing a
// comparison is a drift that must be reviewed against §7.11 B-1 semantics.
var threadStateComparisonSiteGolden = map[string]string{
	"cpu_occupancy.go:computeIdleRunnableMismatchMs":            "runnable#1",
	"query.go:addStateChurnInterval":                            "d_sleep#1",
	"query.go:buildSchedulerLatencyStatsFromStats":              "runnable#1",
	"query.go:buildStateDrilldownPlanForTarget":                 "s_sleep#1",
	"query.go:computeOffCPUStats":                               "runnable,s_sleep,d_sleep,io_wait#5",
	"query.go:detectPeriodicWakeupSource":                       "s_sleep#1",
	"query.go:enrichBlockedReasonIntervalsWithSelection":        "d_sleep#1",
	"query.go:enrichRootCauseRankWithScheduler":                 "running,runnable#2",
	"query.go:enrichStateChurnWithCPUPressure":                  "runnable#1",
	"query.go:findBinderWaitsForChain":                          "s_sleep,d_sleep,io_wait#3",
	"query.go:interestingIntervals":                             "running#1",
	"query.go:isFragmentedSleepChurn":                           "s_sleep#1",
	"query.go:isIntermediateSleepAggregate":                     "s_sleep#1",
	"query.go:isIntermediateSleepImpact":                        "s_sleep#1",
	"query.go:offCPUIntervals":                                  "runnable#1",
	"query.go:offCPUStateIsIOWait":                              "d_sleep,io_wait#2",
	"query.go:stateDrilldownNeedsRecursiveChainForSource":       "runnable#1",
	"query.go:stateDrilldownNeedsWakeupChainForSource":          "s_sleep#1",
	"query.go:stateDrilldownRecommendedViewsForSource":          "s_sleep#1",
	"query.go:summarizeWakeupCausalImpact":                      "running,runnable#3",
	"query.go:traceCompletenessCaveats":                         "s_sleep#1",
	"stream_search.go:addStreamStateClusterInterval":            "d_sleep#1",
	"supply_fold.go:supplyFoldRunningIntervals":                 "running#1",
	"thread_state_universe.go:stateChurnWakeupReopenIneligible": "running,runnable#2",
}

func TestThreadStateComparisonConsumerCoverage(t *testing.T) {
	sites := collectThreadStateComparisonSites(t)
	if len(sites) == 0 {
		t.Fatal("no ThreadState comparison expressions matched — the pin is checking nothing; update the scan alongside the refactor")
	}
	total := 0
	got := map[string]string{}
	for _, site := range sites {
		got[site.key] = fmt.Sprintf("%s#%d", renderThreadStateMembers(site.members), site.count)
		total += site.count
	}
	if total == 0 {
		t.Fatal("comparison lane matched zero expressions — sentinel")
	}
	if !reflect.DeepEqual(got, threadStateComparisonSiteGolden) {
		var lines []string
		for key, value := range got {
			lines = append(lines, fmt.Sprintf("\t%q: %q,", key, value))
		}
		sort.Strings(lines)
		t.Errorf("ThreadState comparison sites drifted from threadStateComparisonSiteGolden — review EVERY change (a tampered member is a silent misclassification), then update the golden. Current scan:\n%s", strings.Join(lines, "\n"))
	}
}

func collectThreadStateComparisonSites(t *testing.T) []threadStateComparisonSite {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(names)
	var sites []threadStateComparisonSite
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
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
			site := threadStateComparisonSite{
				key:     fmt.Sprintf("%s:%s", name, fn.Name.Name),
				members: map[ThreadState]bool{},
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				bin, ok := n.(*ast.BinaryExpr)
				if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
					return true
				}
				matched := false
				for _, operand := range []ast.Expr{bin.X, bin.Y} {
					if state, ok := threadStateMemberFromExpr(operand); ok {
						site.members[state] = true
						matched = true
					}
				}
				if matched {
					site.count++
					if site.pos == "" {
						site.pos = fset.Position(bin.Pos()).String()
					}
				}
				return true
			})
			if site.count > 0 {
				sites = append(sites, site)
			}
		}
	}
	return sites
}

// ---- compute-supply mint witness (TSH review F4) ---------------------------

// TestComputeSupplyMintProductionWitness pins the compute-supply DominantState
// production path: computeSupplySummaries mints ComputeSupplySummary.State via
// add(td, string(State*)) — those wire words flow through
// computeSupplyDominantState's verbatim passthrough into
// RootCauseRankItem.DominantState (root_cause_rank compute-supply lane). The
// witness (a) AST-pins the mint sites to typed string(State*) conversions of
// dominant-lane members with an exact mint-set golden (a raw literal or an
// off-lane member is red), and (b) behavior-checks the passthrough so every
// minted word survives to the published DominantState unchanged.
func TestComputeSupplyMintProductionWitness(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "query.go", nil, 0)
	if err != nil {
		t.Fatalf("parse query.go: %v", err)
	}
	var minted []ThreadState
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "computeSupplySummaries" || fn.Body == nil {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "add" || len(call.Args) != 2 {
				return true
			}
			state, ok := threadStateMemberFromExpr(call.Args[1])
			if !ok {
				t.Errorf("%s: compute-supply mint passes a non-typed state argument — mint only string(State*) dominant-lane tokens (TSH F4)", fset.Position(call.Args[1].Pos()))
				return true
			}
			minted = append(minted, state)
			return true
		})
	}
	if !found {
		t.Fatal("computeSupplySummaries not found in query.go — the mint witness is checking nothing; update the scan alongside the refactor")
	}
	wantMints := []ThreadState{StateRunnable, StateRunning}
	if !reflect.DeepEqual(minted, wantMints) {
		t.Fatalf("compute-supply mint set drifted: got %v want %v (a new mint lane must extend the dominant-lane/state-kind universes in the same change)", minted, wantMints)
	}
	laneSet := map[ThreadState]bool{}
	for _, lane := range threadStateDominantLanes {
		laneSet[lane] = true
	}
	for _, state := range minted {
		if !laneSet[state] {
			t.Errorf("compute-supply mints %q which is NOT a dominant-lane member — RootCauseRankItem.DominantState would carry an unregistered word", state)
		}
		if got := computeSupplyDominantState(ComputeSupplySummary{State: string(state)}); got != string(state) {
			t.Errorf("computeSupplyDominantState(%q) = %q — the verbatim passthrough to RootCauseRankItem.DominantState changed", state, got)
		}
	}
	// The default branch is a TrimSpace passthrough by design; the minted
	// canonical words must ALSO pass through the canonical cases unchanged
	// when padded, exactly as the historical switch behaved.
	if got := computeSupplyDominantState(ComputeSupplySummary{State: "  " + string(StateRunnable) + " "}); got != string(StateRunnable) {
		t.Errorf("computeSupplyDominantState should trim+canonicalize the runnable mint, got %q", got)
	}
}
