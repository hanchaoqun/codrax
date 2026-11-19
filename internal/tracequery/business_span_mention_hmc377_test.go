package tracequery

// business_span_mention_hmc377_test.go — HMC377 E3 pins: business clues on the
// chain. A chain member counts only inside its own dependency windows; the
// analysed thread's own phases form their own tier and carry the marker tree's
// self time and scheduler split; nested wrappers cannot crowd out the work
// inside them. Design: hmc377/design/business-clue-seats.md.

import (
	"fmt"
	"testing"
)

// spanvisMemberChain — target 100 waited [10.010, 10.020] and chain member 300
// ended that wait: the member's only typed dependency window.
func spanvisMemberChain() ChainResult {
	target := ThreadRef{PID: 100, Comm: "app.main"}
	member := ThreadRef{PID: 300, Comm: "worker"}
	return ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 10.0, EndTs: 10.1},
		Nodes: []ChainNode{
			{ID: "n1", Thread: target, Window: TimeWindow{StartTs: 10.010, EndTs: 10.020}},
			{ID: "n2", Thread: member, Depth: 1, Window: TimeWindow{StartTs: 10.010, EndTs: 10.020}},
		},
		Edges: []WakeupEdge{{From: "n2", To: "n1", Waker: member, Wakee: target, WakeupTs: 10.020, WakeupLine: 400}},
	}
}

// A chain member is on the chain only inside the window its wakeup closed. A
// span that encloses the window counts for the part inside it, with its raw
// wall beside it; a span outside every window is temporal proximity and is
// never a clue, however long. Before HMC377 E3 the member's whole spans
// (90ms and 50ms) were published as on-chain clues.
func TestBusinessSpanMentionChainMemberCountsOnlyInsideItsDependencyWindows(t *testing.T) {
	chain := spanvisMemberChain()
	inventory := []TraceSpanSummary{
		spanvisSpan(300, "worker", "LoadFeed", 10.000, 90.0, 100, 900),   // encloses the window: 10ms inside
		spanvisSpan(300, "worker", "ParseCard", 10.012, 3.0, 200, 300),   // wholly inside
		spanvisSpan(300, "worker", "UploadLogs", 10.030, 50.0, 950, 990), // after the wakeup: outside
		spanvisSpan(300, "worker", "Edge", 10.0195, 2.0, 380, 420),       // crosses the wakeup: 0.5ms inside, below the floor
	}
	res := spanvisCompute(spanvisQuery(), chain, wakeupChainThreadSet(chain), spanvisStats(inventory, nil))
	if res == nil {
		t.Fatal("the member's in-window work must stay a business clue")
	}
	byName := map[string]BusinessSpanMention{}
	for _, fam := range res.Families {
		byName[fam.Name] = fam
	}
	if _, ok := byName["UploadLogs"]; ok {
		t.Fatalf("a span of a chain member outside its dependency window is temporal proximity, never an on-chain clue: %+v", res.Families)
	}
	if _, ok := byName["Edge"]; ok || res.OmittedFamilies != 0 {
		t.Fatalf("a 0.5ms in-window part is below the 1ms floor and never counted: %+v", res)
	}
	feed := byName["LoadFeed"]
	if feed.OnChainBasis != BusinessSpanMentionBasisChainMember || feed.Count != 1 || microsBSM(feed.TotalMs) != microsBSM(10) ||
		microsBSM(feed.MaxSingleMs) != microsBSM(10) || microsBSM(feed.SpanWallMs) != microsBSM(90) || feed.StartLine != 100 || feed.EndLine != 900 {
		t.Fatalf("the enclosing span counts its 10ms inside the window and keeps its 90ms wall beside it: %+v", feed)
	}
	card := byName["ParseCard"]
	if microsBSM(card.TotalMs) != microsBSM(3) || microsBSM(card.SpanWallMs) != microsBSM(3) {
		t.Fatalf("a span wholly inside the window counts whole: %+v", card)
	}
	if len(res.Families) != 2 || res.Families[0].Name != "LoadFeed" {
		t.Fatalf("reading order inside the dependency tier is the counted total: %+v", res.Families)
	}
}

// The analysed thread's own phases are a tier of their own: dependency
// threads can never take their places, whatever their size, and the one
// omitted count discloses what either tier's selection left out. Before
// HMC377 E3 nine 9ms member families took all eight seats and the target's
// own three phases vanished into the omitted count.
func TestBusinessSpanMentionOwnPhasesNeverCrowdedOutByDependencyThreads(t *testing.T) {
	chain := spanvisMemberChain()
	chain.Nodes[1].Window = TimeWindow{StartTs: 10.0, EndTs: 10.1}
	chain.Nodes[0].Window = chain.Nodes[1].Window
	var inventory []TraceSpanSummary
	for i, ms := range []float64{2, 3, 4} {
		inventory = append(inventory, spanvisSpan(100, "app.main", fmt.Sprintf("own%d", i), 10.001+float64(i)*0.005, ms, 10+i, 20+i))
	}
	for i := 0; i < 9; i++ {
		inventory = append(inventory, spanvisSpan(300, "worker", fmt.Sprintf("dep%d", i), 10.001+float64(i)*0.001, 9, 100+i, 200+i))
	}
	res := spanvisCompute(spanvisQuery(), chain, wakeupChainThreadSet(chain), spanvisStats(inventory, nil))
	if res == nil || len(res.Families) != 3+BusinessSpanMentionFamilyCap || res.OmittedFamilies != 1 {
		t.Fatalf("three own phases plus the dependency tier's eight, one dependency family omitted: %+v", res)
	}
	for i, want := range []string{"own2", "own1", "own0"} {
		if got := res.Families[i]; got.Name != want || got.OnChainBasis != BusinessSpanMentionBasisSelf {
			t.Fatalf("own phases read first, by total: position %d = %+v", i, got)
		}
	}
	for _, fam := range res.Families[3:] {
		if fam.OnChainBasis != BusinessSpanMentionBasisChainMember {
			t.Fatalf("the dependency tier follows the own tier: %+v", res.Families)
		}
	}
}

// A thread whose only link to the target is a wakeup delivered while the
// target was already running ended no wait: its earlier long span is temporal
// proximity. Before HMC377 E3 the host-edge arm published the whole 68ms span
// as a pre-edge clue.
const businessClueLateWakeupTrace = `# tracer: nop
app-7 (7) [000] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=7 next_prio=120
cache-11 (11) [002] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=cache next_pid=11 next_prio=120
app-7 (7) [000] .... 1.000000: tracing_mark_write: B|7|StartActivity
cache-11 (11) [002] .... 1.002000: tracing_mark_write: B|11|WarmCache
cache-11 (11) [002] .... 1.070000: tracing_mark_write: E|11
cache-11 (11) [002] .... 1.080000: sched_wakeup: comm=app pid=7 prio=120 target_cpu=000
app-7 (7) [000] .... 1.090000: tracing_mark_write: E|7
app-7 (7) [000] .... 1.099000: sched_switch: prev_comm=app prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`

func TestBusinessSpanMentionWakeupThatEndedNoWaitIsNotAClue(t *testing.T) {
	idx := buildTraceIndex(t, "late-wakeup.systrace", businessClueLateWakeupTrace)
	res := Run(idx, Query{View: "root_cause_rank", PID: 7, TargetScope: TargetScopeThread, TimeStart: 1.0, TimeEnd: 1.1})
	if res.RootCauseRank == nil || res.RootCauseRank.BusinessSpanMentions == nil {
		t.Fatalf("the target's own phase must still be mentioned: %+v", res.RootCauseRank)
	}
	for _, fam := range res.RootCauseRank.BusinessSpanMentions.Families {
		if fam.Thread.PID == 11 {
			t.Fatalf("a wakeup that found the target running ended no wait; the waker's earlier span is not a clue: %+v", fam)
		}
	}
	if fams := res.RootCauseRank.BusinessSpanMentions.Families; len(fams) != 1 || fams[0].Name != "StartActivity" {
		t.Fatalf("only the own phase remains: %+v", fams)
	}
}

// Nested own phases: by inclusive time an enclosing wrapper always ranks at
// least as high as the work inside it, so eight wrappers whose time is all in
// their children fill both inclusive criteria. The self account from the
// synchronous marker tree selects the work itself, and every own family with
// closed tree instances carries its self time and scheduler split. A family
// without tree accounts (asynchronous spans) carries no phase account.
func TestBusinessSpanMentionOwnPhaseSelfCriterionKeepsWorkInsideWrappers(t *testing.T) {
	chain := spanvisChain()
	var inventory []TraceSpanSummary
	var nodes []TraceMarkerTreeNode
	add := func(name string, start, ms, selfMs float64, line int, states *TraceMarkerTreeStateValues, tree bool) {
		inventory = append(inventory, spanvisSpan(100, "app.main", name, start, ms, line, line+1))
		if tree {
			nodes = append(nodes, spanvisTreeNode(100, line, ms, selfMs, states))
		}
	}
	// Eight wrappers, 60..53ms inclusive, 0.1ms self each.
	for i := 0; i < 8; i++ {
		add(fmt.Sprintf("wrap%d", i), 10.001, float64(60-i), 0.1, 10+i*10, &TraceMarkerTreeStateValues{RunningMs: float64(60 - i)}, true)
	}
	// The work inside them: one 20ms lock wait (all self), and a decompress
	// step repeated ten times (1ms self each).
	add("lockWait", 10.010, 20, 20, 500, &TraceMarkerTreeStateValues{RunningMs: 0.5, RunnableMs: 0.5, SleepMs: 19}, true)
	for i := 0; i < 10; i++ {
		add("decompress", 10.040+float64(i)*0.001, 1, 1, 600+i*2, &TraceMarkerTreeStateValues{RunningMs: 0.8, DStateMs: 0.2}, true)
	}
	// An asynchronous span the synchronous tree does not hold.
	add("asyncLoad", 10.050, 5, 0, 900, nil, false)
	// A tree instance of another thread with the same begin line never
	// attaches to the target's family.
	nodes = append(nodes, spanvisTreeNode(300, 500, 20, 1, nil))
	stats := spanvisStats(inventory, nil)
	stats.BusinessTree = &TraceMarkerTreeStats{allNodes: nodes}
	res := spanvisCompute(spanvisQuery(), chain, wakeupChainThreadSet(chain), stats)
	if res == nil {
		t.Fatal("own phases must be mentioned")
	}
	byName := map[string]BusinessSpanMention{}
	for _, fam := range res.Families {
		byName[fam.Name] = fam
	}
	lock, ok := byName["lockWait"]
	if !ok || lock.Phase == nil || microsBSM(lock.Phase.SelfMs) != microsBSM(20) || lock.Phase.States == nil ||
		microsBSM(lock.Phase.States.SleepMs) != microsBSM(19) || microsBSM(lock.Phase.States.RunnableMs) != microsBSM(0.5) {
		t.Fatalf("the 20ms wait inside the wrappers must be selected with its self time and scheduler split: %+v", res.Families)
	}
	dec, ok := byName["decompress"]
	if !ok || dec.Count != 10 || dec.Phase == nil || microsBSM(dec.Phase.SelfMs) != microsBSM(10) || microsBSM(dec.Phase.MaxSelfMs) != microsBSM(1) ||
		dec.Phase.States == nil || microsBSM(dec.Phase.States.RunningMs) != microsBSM(8) || microsBSM(dec.Phase.States.DStateMs) != microsBSM(2) {
		t.Fatalf("the repeated step must be selected with its summed self time and split: %+v", dec)
	}
	for i := 0; i < 8; i++ {
		if w, ok := byName[fmt.Sprintf("wrap%d", i)]; !ok || w.Phase == nil || microsBSM(w.Phase.SelfMs) != microsBSM(0.1) {
			t.Fatalf("the enclosing phases keep their inclusive seats and show how little of it is their own: %+v", res.Families)
		}
	}
	if async, ok := byName["asyncLoad"]; ok && async.Phase != nil {
		t.Fatalf("a span without a tree instance carries no phase account: %+v", async)
	}
	if res.Families[0].Name != "wrap0" {
		t.Fatalf("reading order stays the inclusive total: %+v", res.Families)
	}
}

// A tree instance whose clipped inclusive wall differs from the span's
// in-window duration is not the same physical account: the family publishes
// no phase account rather than a mismatched one.
func TestBusinessSpanMentionPhaseAccountRequiresSameInstance(t *testing.T) {
	chain := spanvisChain()
	inventory := []TraceSpanSummary{spanvisSpan(100, "app.main", "phase", 10.001, 5, 10, 20)}
	stats := spanvisStats(inventory, nil)
	stats.BusinessTree = &TraceMarkerTreeStats{allNodes: []TraceMarkerTreeNode{spanvisTreeNode(100, 10, 4.9, 4.9, nil)}}
	res := spanvisCompute(spanvisQuery(), chain, wakeupChainThreadSet(chain), stats)
	if res == nil || len(res.Families) != 1 || res.Families[0].Phase != nil {
		t.Fatalf("a mismatched tree account must not attach: %+v", res)
	}
}

// spanvisTreeNode is one closed synchronous marker instance with its
// completed inclusive / self accounts (states on the inclusive extent).
func spanvisTreeNode(tid, line int, inclusiveMs, selfMs float64, states *TraceMarkerTreeStateValues) TraceMarkerTreeNode {
	n := TraceMarkerTreeNode{Thread: ThreadRef{PID: tid}, StartLine: line, Closure: "closed",
		Inclusive: &TraceMarkerTreeAccount{DurationMs: inclusiveMs}, Self: &TraceMarkerTreeAccount{DurationMs: selfMs}}
	if states != nil {
		n.Inclusive.States = &TraceMarkerTreeStates{Coverage: "complete", Values: states}
	}
	return n
}

// The same thread addressed by name gets the same own-phase accounts as when
// addressed by id (the tree keeps every instance; the face indexes the chain
// target's).
func TestBusinessSpanMentionOwnPhaseAccountsDoNotDependOnTheSelector(t *testing.T) {
	idx := buildTraceIndex(t, "business-clue.systrace", businessClueSyntheticTrace)
	byID := Run(idx, Query{View: "root_cause_rank", PID: 7, TargetScope: TargetScopeThread, TimeStart: 1.0, TimeEnd: 1.1})
	byName := Run(idx, Query{View: "root_cause_rank", Thread: "app", TargetScope: TargetScopeThread, TimeStart: 1.0, TimeEnd: 1.1})
	if byID.RootCauseRank == nil || byName.RootCauseRank == nil || byID.RootCauseRank.BusinessSpanMentions == nil || byName.RootCauseRank.BusinessSpanMentions == nil {
		t.Fatal("both selectors must publish the business-clue face")
	}
	a, b := byID.RootCauseRank.BusinessSpanMentions.Families, byName.RootCauseRank.BusinessSpanMentions.Families
	if len(a) != len(b) || len(a) == 0 || a[0].Phase == nil || b[0].Phase == nil || microsBSM(a[0].Phase.SelfMs) != microsBSM(b[0].Phase.SelfMs) {
		t.Fatalf("id and name selectors disagree:\n%+v\n%+v", a, b)
	}
}

// End to end through the engine's public Run on a synthetic capture: the
// target's nested phases carry the tree's self time and scheduler split, and
// the waker contributes only its work inside the wait it ended.
const businessClueSyntheticTrace = `# tracer: nop
app-7 (7) [000] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=7 next_prio=120
worker-9 (9) [001] .... 0.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=9 next_prio=120
app-7 (7) [000] .... 1.000000: tracing_mark_write: B|7|StartActivity
app-7 (7) [000] .... 1.001000: tracing_mark_write: B|7|InflateLayout
worker-9 (9) [001] .... 1.001000: tracing_mark_write: B|9|PrefetchFeed
app-7 (7) [000] .... 1.004000: tracing_mark_write: E|7
app-7 (7) [000] .... 1.005000: sched_switch: prev_comm=app prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
worker-9 (9) [001] .... 1.030000: sched_wakeup: comm=app pid=7 prio=120 target_cpu=000
app-7 (7) [000] .... 1.030100: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=7 next_prio=120
worker-9 (9) [001] .... 1.060000: tracing_mark_write: E|9
worker-9 (9) [001] .... 1.061000: tracing_mark_write: B|9|UploadLogs
app-7 (7) [000] .... 1.090000: tracing_mark_write: E|7
worker-9 (9) [001] .... 1.095000: tracing_mark_write: E|9
app-7 (7) [000] .... 1.099000: sched_switch: prev_comm=app prev_pid=7 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`

func TestBusinessSpanMentionEngineRunOwnPhasesAndDependencyPart(t *testing.T) {
	idx := buildTraceIndex(t, "business-clue.systrace", businessClueSyntheticTrace)
	res := Run(idx, Query{View: "root_cause_rank", PID: 7, TargetScope: TargetScopeThread, TimeStart: 1.0, TimeEnd: 1.1})
	if res.RootCauseRank == nil || res.RootCauseRank.BusinessSpanMentions == nil {
		t.Fatalf("no business clue face: %+v", res.RootCauseRank)
	}
	byName := map[string]BusinessSpanMention{}
	for _, fam := range res.RootCauseRank.BusinessSpanMentions.Families {
		byName[fam.Name] = fam
	}
	start, ok := byName["StartActivity"]
	if !ok || start.OnChainBasis != BusinessSpanMentionBasisSelf || microsBSM(start.TotalMs) != microsBSM(90) || start.Phase == nil ||
		microsBSM(start.Phase.SelfMs) != microsBSM(87) || start.Phase.States == nil ||
		microsBSM(start.Phase.States.SleepMs) != microsBSM(25) || microsBSM(start.Phase.States.RunningMs+start.Phase.States.RunnableMs) != microsBSM(65) {
		t.Fatalf("the own phase must carry inclusive 90ms, self 87ms and its scheduler split: %+v", start)
	}
	inflate, ok := byName["InflateLayout"]
	if !ok || inflate.Phase == nil || microsBSM(inflate.Phase.SelfMs) != microsBSM(3) {
		t.Fatalf("the nested own phase must carry its own account: %+v", inflate)
	}
	feed, ok := byName["PrefetchFeed"]
	if !ok || feed.OnChainBasis != BusinessSpanMentionBasisChainMember || microsBSM(feed.TotalMs) != microsBSM(25) || microsBSM(feed.SpanWallMs) != microsBSM(59) || feed.Phase != nil {
		t.Fatalf("the waker's span counts its 25ms inside the wait it ended, beside its 59ms wall: %+v", feed)
	}
	if _, ok := byName["UploadLogs"]; ok {
		t.Fatalf("the waker's work after the wakeup is not an on-chain clue: %+v", res.RootCauseRank.BusinessSpanMentions.Families)
	}
	fams := res.RootCauseRank.BusinessSpanMentions.Families
	if len(fams) < 3 || fams[0].Name != "StartActivity" || fams[1].Name != "InflateLayout" || fams[len(fams)-1].Name != "PrefetchFeed" {
		t.Fatalf("own phases read first, then the dependency thread: %+v", fams)
	}
}
