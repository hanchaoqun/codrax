package tool

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type threadGroupSideHMC376 struct {
	lane   string
	thread types.RuntimeWakeupThread
}

func threadGroupRowHMC376(path string, line int, clock string, from, to threadGroupSideHMC376) RuntimeDiagramRelation {
	return RuntimeDiagramRelation{Kind: types.DiagramRelWakeup,
		FromNode: from.lane, ToNode: to.lane, FromLabel: fmt.Sprintf("%s-%d", from.thread.Comm, from.thread.PID), ToLabel: fmt.Sprintf("%s-%d", to.thread.Comm, to.thread.PID),
		FromIdentity: fmt.Sprintf("runtime_instance_%s_%d_from", from.lane, line), ToIdentity: fmt.Sprintf("runtime_instance_%s_%d_to", to.lane, line),
		SupportRefs:     []string{fmt.Sprintf("%s:%d", path, line)},
		eventOrder:      &runtimeDiagramEventOrder{timestamp: float64(line)},
		threadEndpoints: &runtimeDiagramThreadEndpoints{source: types.ObservationSourceRef{Path: path, TimeDomain: clock, CanonicalTimeDomain: clock}, from: from.thread, to: to.thread}}
}

// The same-recorded-thread predicate, one typed coordinate at a time. Each
// withheld join is paired with the same rows minus the withholding signal,
// which must join: the signal itself is what keeps the lanes apart.
func TestRuntimeDiagramThreadGroupsHMC376(t *testing.T) {
	const capture, clock = "/captures/a.ftrace", "boot"
	peer := threadGroupSideHMC376{"rt_thread_peer", types.RuntimeWakeupThread{Comm: "peer", PID: 7, TGID: 7}}
	side := func(lane string, tid, tgid int) threadGroupSideHMC376 {
		return threadGroupSideHMC376{lane, types.RuntimeWakeupThread{Comm: "worker", PID: tid, TGID: tgid}}
	}
	pair := func(a, b RuntimeDiagramRelation) []RuntimeDiagramRelation { return []RuntimeDiagramRelation{a, b} }
	base := func() []RuntimeDiagramRelation {
		return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 42, 40), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", 42, 0)))
	}
	cases := []struct {
		name   string
		rows   func() []RuntimeDiagramRelation
		joined bool
	}{
		{"one thread number, process recorded on one lane only", base, true},
		{"renamed thread keeps its number", func() []RuntimeDiagramRelation {
			rows := base()
			rows[1].threadEndpoints.to.Comm = "renamed"
			return rows
		}, true},
		{"different thread numbers", func() []RuntimeDiagramRelation {
			return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 42, 40), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", 43, 40)))
		}, false},
		{"different capture files", func() []RuntimeDiagramRelation {
			return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 42, 40), peer), threadGroupRowHMC376("/captures/b.ftrace", 20, clock, peer, side("rt_thread_b", 42, 40)))
		}, false},
		{"different clocks", func() []RuntimeDiagramRelation {
			return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 42, 40), peer), threadGroupRowHMC376(capture, 20, "mono", peer, side("rt_thread_b", 42, 40)))
		}, false},
		{"different canonical clocks", func() []RuntimeDiagramRelation {
			rows := base()
			rows[1].threadEndpoints.source.CanonicalTimeDomain = "aligned"
			return rows
		}, false},
		{"two recorded process numbers", func() []RuntimeDiagramRelation {
			return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 42, 40), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", 42, 41)))
		}, false},
		{"recorded exit and reuse of the thread number", func() []RuntimeDiagramRelation {
			rows := base()
			rows[0].threadEndpoints.fromLifecycle = runtimeDiagramThreadLifecycleReused
			return rows
		}, false},
		{"incomplete exit and reuse record for the capture", func() []RuntimeDiagramRelation {
			rows := base()
			rows[1].threadEndpoints.toLifecycle = runtimeDiagramThreadLifecycleIncomplete
			return rows
		}, false},
		{"no typed endpoints", func() []RuntimeDiagramRelation {
			rows := base()
			rows[1].threadEndpoints = nil
			return rows
		}, false},
		{"two support refs are not one physical point", func() []RuntimeDiagramRelation {
			rows := base()
			rows[1].SupportRefs = append(rows[1].SupportRefs, capture+":21")
			return rows
		}, false},
		{"support ref without a line", func() []RuntimeDiagramRelation {
			rows := base()
			rows[1].SupportRefs = []string{capture}
			return rows
		}, false},
		{"one lane id carrying two thread numbers", func() []RuntimeDiagramRelation {
			rows := base()
			return append(rows, threadGroupRowHMC376(capture, 30, clock, peer, side("rt_thread_b", 43, 0)))
		}, false},
		{"the per-CPU idle task is not one thread", func() []RuntimeDiagramRelation {
			return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 0, 0), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", 0, 0)))
		}, false},
		{"thread number above the system limit", func() []RuntimeDiagramRelation {
			return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", types.RuntimeTargetMaxPID+1, 0), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", types.RuntimeTargetMaxPID+1, 0)))
		}, false},
		{"event-local nodes are not thread lanes", func() []RuntimeDiagramRelation {
			return pair(threadGroupRowHMC376(capture, 10, clock, side("rt_a", 42, 40), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_b", 42, 0)))
		}, false},
		{"another relation kind", func() []RuntimeDiagramRelation {
			rows := base()
			rows[1].Kind = types.DiagramRelContain
			return rows
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := tc.rows()
			a, b := rows[0].FromNode, rows[1].ToNode
			groups := runtimeDiagramThreadGroups(rows)
			if got := a != b && runtimeDiagramSameThreadGroup(groups, a, b); got != tc.joined {
				t.Fatalf("joined=%t, want %t: %v", got, tc.joined, groups)
			}
			if tc.joined && !(runtimeDiagramLaneJoined(groups, a) && runtimeDiagramLaneJoined(groups, b)) {
				t.Fatalf("joined lanes are not reported as proven thread groups: %v", groups)
			}
			// A lane is always the same thread as itself, joined or not.
			if !runtimeDiagramSameThreadGroup(groups, a, a) || !runtimeDiagramSameThreadGroup(groups, "rt_thread_unpublished", "rt_thread_unpublished") {
				t.Fatal("a lane is not the same thread as itself")
			}
			if runtimeDiagramSameThreadGroup(groups, a, peer.lane) {
				t.Fatal("two different thread numbers joined")
			}
		})
	}
	// Self-red: each guard signal alone withholds the join. Removing only
	// that signal from the same rows must join them.
	twoProcesses := pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 42, 40), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", 42, 41)))
	if runtimeDiagramSameThreadGroup(runtimeDiagramThreadGroups(twoProcesses), "rt_thread_a", "rt_thread_b") {
		t.Fatal("two process numbers joined")
	}
	twoProcesses[1].threadEndpoints.to.TGID = 0
	if !runtimeDiagramSameThreadGroup(runtimeDiagramThreadGroups(twoProcesses), "rt_thread_a", "rt_thread_b") {
		t.Fatal("the process contradiction is not what withheld the join")
	}
	for _, state := range []runtimeDiagramThreadLifecycle{runtimeDiagramThreadLifecycleReused, runtimeDiagramThreadLifecycleIncomplete} {
		reused := base()
		reused[1].threadEndpoints.toLifecycle = state
		if runtimeDiagramSameThreadGroup(runtimeDiagramThreadGroups(reused), "rt_thread_a", "rt_thread_b") {
			t.Fatal("a thread number with a lifecycle record joined")
		}
		reused[1].threadEndpoints.toLifecycle = runtimeDiagramThreadLifecycleNone
		if !runtimeDiagramSameThreadGroup(runtimeDiagramThreadGroups(reused), "rt_thread_a", "rt_thread_b") {
			t.Fatal("the lifecycle record is not what withheld the join")
		}
	}
	// A contradiction on one thread number never spreads to another.
	other := append(pair(threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 42, 40), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", 42, 41))),
		threadGroupRowHMC376(capture, 30, clock, side("rt_thread_c", 50, 40), peer), threadGroupRowHMC376(capture, 40, clock, peer, side("rt_thread_d", 50, 0)))
	if groups := runtimeDiagramThreadGroups(other); !runtimeDiagramSameThreadGroup(groups, "rt_thread_c", "rt_thread_d") || runtimeDiagramSameThreadGroup(groups, "rt_thread_a", "rt_thread_b") {
		t.Fatalf("a contradiction leaked across thread numbers: %v", groups)
	}
}

// The three gates read the group, and each stays closed across groups.
func TestRuntimeDiagramThreadGroupGatesHMC376(t *testing.T) {
	const capture, clock = "/captures/a.ftrace", "boot"
	peer := threadGroupSideHMC376{"rt_thread_peer", types.RuntimeWakeupThread{Comm: "peer", PID: 7, TGID: 7}}
	side := func(lane string, tgid int) threadGroupSideHMC376 {
		return threadGroupSideHMC376{lane, types.RuntimeWakeupThread{Comm: "worker", PID: 42, TGID: tgid}}
	}
	build := func(tgid int) ([]RuntimeDiagramRelation, []types.DiagramEdgeAnchor) {
		rows := []RuntimeDiagramRelation{threadGroupRowHMC376(capture, 10, clock, side("rt_thread_a", 40), peer), threadGroupRowHMC376(capture, 20, clock, peer, side("rt_thread_b", tgid))}
		anchors := []types.DiagramEdgeAnchor{
			{FromNode: "W", ToNode: "P", FromIdentity: rows[0].FromIdentity, ToIdentity: rows[0].ToIdentity, RelationKind: types.DiagramRelWakeup},
			{FromNode: "P", ToNode: "W", FromIdentity: rows[1].FromIdentity, ToIdentity: rows[1].ToIdentity, RelationKind: types.DiagramRelWakeup}}
		return rows, anchors
	}
	aliasConflict := func(rows []RuntimeDiagramRelation, anchors []types.DiagramEdgeAnchor, node string) bool {
		return len(runtimeDiagramLifelineOddSides(rows, runtimeDiagramThreadGroups(rows), anchors, node)) > 0
	}
	block := func(anchors []types.DiagramEdgeAnchor, selections ...types.RuntimeDiagramParticipantSelection) types.AnswerBlock {
		return types.AnswerBlock{ID: "d", Kind: types.BlockDiagram, EdgeAnchors: anchors, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence,
			Body: "sequenceDiagram\n participant W\n participant P\n W->>P: a\n P->>W: b", RuntimeParticipantsV1: selections}}
	}
	for _, tc := range []struct {
		name   string
		tgid   int
		joined bool
	}{{"one recorded thread", 0, true}, {"two process numbers", 41, false}} {
		t.Run(tc.name, func(t *testing.T) {
			rows, anchors := build(tc.tgid)
			if got := aliasConflict(rows, anchors, "W"); got == tc.joined {
				t.Fatalf("alias binding conflict=%t", got)
			}
			if aliasConflict(rows, anchors, "P") {
				t.Fatal("the peer lifeline hosts one lane only")
			}
			published := types.DiagramEdgeAnchor{FromNode: "rt_thread_peer", ToNode: "rt_thread_a", FromIdentity: rows[1].FromIdentity, ToIdentity: rows[1].ToIdentity, RelationKind: types.DiagramRelWakeup}
			if got := runtimeDiagramAnchorAliasConflict(rows, published); got == tc.joined {
				t.Fatalf("published lane id hosting the other copy: conflict=%t", got)
			}
			if _, toSide := runtimeDiagramAnchorAliasConflictSides(rows, published); toSide == tc.joined {
				t.Fatalf("the conflicting side is not reported exactly: %t", toSide)
			}
			_, err := runtimeDiagramParticipantBindings(block(anchors, types.RuntimeDiagramParticipantSelection{NodeID: "W", NativeParticipant: "rt_thread_b"}), rows)
			if (err == nil) != tc.joined {
				t.Fatalf("participant binding error=%v", err)
			}
			odd := runtimeDiagramLifelineOddArrows(rows, block(anchors))
			if tc.joined != (len(odd) == 0) {
				t.Fatalf("odd arrows=%v", odd)
			}
			if !tc.joined {
				// Only the arrow that disagrees with the first arrow is odd.
				if _, first := odd[0]; first || odd[1] == "" {
					t.Fatalf("want only the second arrow listed: %v", odd)
				}
			}
		})
	}
	// A published identity of a third thread never matches a joined lane.
	rows, _ := build(0)
	foreign := types.DiagramEdgeAnchor{FromNode: "rt_thread_a", ToNode: "rt_thread_peer", FromIdentity: rows[1].FromIdentity, ToIdentity: rows[1].ToIdentity, RelationKind: types.DiagramRelWakeup}
	if from, to := runtimeDiagramAnchorAliasConflictSides(rows, foreign); !from || !to {
		t.Fatalf("a lane hosted an identity of another thread: %t %t", from, to)
	}
	// Rows without typed endpoints keep the exact same-lane rule.
	untyped, anchors := build(0)
	for i := range untyped {
		untyped[i].threadEndpoints = nil
	}
	if !aliasConflict(untyped, anchors, "W") {
		t.Fatal("untyped lanes joined")
	}
}

func TestMarkRuntimeDiagramThreadLifecycleBoundariesHMC376(t *testing.T) {
	const capture, physical, clock = "/captures/bundle", "/captures/a.ftrace", "boot"
	rows := func() []RuntimeDiagramRelation {
		a := threadGroupRowHMC376(physical, 10, clock, threadGroupSideHMC376{"rt_thread_a", types.RuntimeWakeupThread{PID: 12}}, threadGroupSideHMC376{"rt_thread_b", types.RuntimeWakeupThread{PID: 42}})
		a.threadEndpoints.source.Path = capture
		b := threadGroupRowHMC376("/captures/other.ftrace", 10, clock, threadGroupSideHMC376{"rt_thread_c", types.RuntimeWakeupThread{PID: 12}}, threadGroupSideHMC376{"rt_thread_d", types.RuntimeWakeupThread{PID: 42}})
		return []RuntimeDiagramRelation{a, b, {Kind: types.DiagramRelContain}}
	}
	record := func(path string, refs []string, notes ...string) types.ObservationRecord {
		return types.ObservationRecord{Predicate: "thread_incarnation_suppression", SourceRef: types.ObservationSourceRef{Path: path}, SupportRefs: refs, RichNotes: notes}
	}
	type states = [4]runtimeDiagramThreadLifecycle
	flags := func(rows []RuntimeDiagramRelation) states {
		return states{rows[0].threadEndpoints.fromLifecycle, rows[0].threadEndpoints.toLifecycle, rows[1].threadEndpoints.fromLifecycle, rows[1].threadEndpoints.toLifecycle}
	}
	const none, incomplete, reused = runtimeDiagramThreadLifecycleNone, runtimeDiagramThreadLifecycleIncomplete, runtimeDiagramThreadLifecycleReused
	for _, tc := range []struct {
		name    string
		records []types.ObservationRecord
		want    states
	}{
		{"no lifecycle record", nil, states{}},
		{"another predicate", []types.ObservationRecord{{Predicate: "thread_selector_exact_name_mismatch", SourceRef: types.ObservationSourceRef{Path: capture}, RichNotes: []string{"conflict_tid=12"}}}, states{}},
		{"that thread number in that capture, by query source", []types.ObservationRecord{record(capture, nil, "conflict_tid=12", "signal=sched_wakeup_new")}, states{reused, incomplete, none, none}},
		{"that thread number in that capture, by physical file", []types.ObservationRecord{record("/captures/elsewhere", []string{physical + ":6-8"}, "conflict_tid=12")}, states{reused, incomplete, none, none}},
		// The roster of one lookup is bounded and can stop silently, so a record
		// for another number does not prove that this number was not reused.
		{"another thread number of that capture", []types.ObservationRecord{record(capture, nil, "conflict_tid=13")}, states{incomplete, incomplete, none, none}},
		{"another capture", []types.ObservationRecord{record("/captures/unrelated", nil, "conflict_tid=12")}, states{}},
		{"a capped audit names no thread number", []types.ObservationRecord{record(capture, nil, "signal=lifecycle_audit_truncated")}, states{incomplete, incomplete, none, none}},
		{"a capped audit with a selector number", []types.ObservationRecord{record(capture, nil, "conflict_tid=99", "signal=lifecycle_audit_truncated")}, states{incomplete, incomplete, none, none}},
		{"an unreadable thread number fails closed", []types.ObservationRecord{record(capture, nil, "conflict_tid=many")}, states{incomplete, incomplete, none, none}},
		{"a recorded reuse beside a capped audit", []types.ObservationRecord{record(capture, nil, "signal=lifecycle_audit_truncated"), record(capture, nil, "conflict_tid=12")}, states{reused, incomplete, none, none}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := rows()
			shared := input[0].threadEndpoints
			got := markRuntimeDiagramThreadLifecycleBoundaries(input, types.ObservationLedger{Records: tc.records})
			if flags(got) != tc.want {
				t.Fatalf("flags=%v want %v", flags(got), tc.want)
			}
			if shared.fromLifecycle != none || shared.toLifecycle != none {
				t.Fatal("the provider's endpoint carrier was mutated in place")
			}
			// Any recorded state withholds the join; the reason names it.
			index := runtimeDiagramThreadIndexOf(append(got[:1:1], threadGroupRowHMC376(physical, 20, clock, threadGroupSideHMC376{"rt_thread_a2", types.RuntimeWakeupThread{PID: 12, TGID: 5}}, threadGroupSideHMC376{"rt_thread_b2", types.RuntimeWakeupThread{PID: 42, TGID: 6}})))
			for i, lanes := range [][2]string{{"rt_thread_a", "rt_thread_a2"}, {"rt_thread_b", "rt_thread_b2"}} {
				joined, reason := runtimeDiagramSameThreadGroup(index.groups, lanes[0], lanes[1]), index.withheldReason(lanes[0], lanes[1])
				wantReason := map[runtimeDiagramThreadLifecycle]string{none: "", reused: "the capture records an exit and reuse of this thread number", incomplete: "the capture's record of thread exits and reuses is incomplete"}[tc.want[i]]
				if joined != (tc.want[i] == none) || (wantReason == "") != (reason == "") || !strings.Contains(reason, wantReason) {
					t.Fatalf("side %d: joined=%t reason=%q for state %d", i, joined, reason, tc.want[i])
				}
			}
			if got[0].threadEndpoints.source != shared.source || got[0].threadEndpoints.from != shared.from || got[0].threadEndpoints.to != shared.to || got[0].FromNode != "rt_thread_a" {
				t.Fatal("marking changed an endpoint, lane or source")
			}
		})
	}
}

func TestPropagateRequestedParticipantsHMC376(t *testing.T) {
	const capture, clock = "/captures/a.ftrace", "boot"
	waker := func(lane string, tgid int) threadGroupSideHMC376 {
		return threadGroupSideHMC376{lane, types.RuntimeWakeupThread{Comm: "waker", PID: 12, TGID: tgid}}
	}
	wakee := func(lane string, tgid int) threadGroupSideHMC376 {
		return threadGroupSideHMC376{lane, types.RuntimeWakeupThread{Comm: "wakee", PID: 42, TGID: tgid}}
	}
	bound := func() RuntimeDiagramRelation {
		row := threadGroupRowHMC376(capture, 10, clock, waker("rt_thread_w1", 10), wakee("rt_thread_t1", 40))
		row.requestedTo = []string{"requested"}
		return row
	}
	other := func(mutate func(*RuntimeDiagramRelation)) RuntimeDiagramRelation {
		row := threadGroupRowHMC376(capture, 10, clock, waker("rt_thread_w2", 10), wakee("rt_thread_t2", 0))
		if mutate != nil {
			mutate(&row)
		}
		return row
	}
	for _, tc := range []struct {
		name   string
		copy   RuntimeDiagramRelation
		wantTo []string
	}{
		{"another lookup's copy of the same physical row", other(nil), []string{"requested"}},
		{"another line", threadGroupRowHMC376(capture, 11, clock, waker("rt_thread_w2", 10), wakee("rt_thread_t2", 0)), nil},
		{"another capture file", threadGroupRowHMC376("/captures/b.ftrace", 10, clock, waker("rt_thread_w2", 10), wakee("rt_thread_t2", 0)), nil},
		{"another clock", threadGroupRowHMC376(capture, 10, "mono", waker("rt_thread_w2", 10), wakee("rt_thread_t2", 0)), nil},
		{"another timestamp", other(func(r *RuntimeDiagramRelation) { r.eventOrder = &runtimeDiagramEventOrder{timestamp: 99} }), nil},
		{"another woken thread number", other(func(r *RuntimeDiagramRelation) { r.threadEndpoints.to.PID = 43 }), nil},
		{"the woken side records another process number", other(func(r *RuntimeDiagramRelation) { r.threadEndpoints.to.TGID = 41 }), nil},
		{"no native event order", other(func(r *RuntimeDiagramRelation) { r.eventOrder = nil }), nil},
		{"no typed endpoints", other(func(r *RuntimeDiagramRelation) { r.threadEndpoints = nil }), nil},
		{"two support refs", other(func(r *RuntimeDiagramRelation) { r.SupportRefs = append(r.SupportRefs, capture+":11") }), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := bound()
			before := tc.copy
			rows := propagateRequestedParticipantsAcrossEventCopies([]RuntimeDiagramRelation{source, tc.copy})
			if !reflect.DeepEqual(rows[1].requestedTo, tc.wantTo) || len(rows[1].requestedFrom) != 0 {
				t.Fatalf("copy binding=%v/%v want to=%v", rows[1].requestedFrom, rows[1].requestedTo, tc.wantTo)
			}
			// The proved copy keeps its binding unless its own side is
			// contradicted by another copy of that same physical row.
			if !reflect.DeepEqual(rows[0].requestedTo, []string{"requested"}) {
				t.Fatalf("the proved copy lost its binding: %v", rows[0].requestedTo)
			}
			// A copied binding is marked as copied; the lookup's own is not.
			if len(tc.wantTo) > 0 && !reflect.DeepEqual(rows[1].requestedToCopied, tc.wantTo) {
				t.Fatalf("the copied binding is not marked as copied: %v", rows[1].requestedToCopied)
			}
			if len(rows[0].requestedToCopied) != 0 {
				t.Fatalf("the lookup's own binding was marked as copied: %v", rows[0].requestedToCopied)
			}
			rows[1].requestedTo, rows[1].requestedFrom, rows[1].requestedToCopied = nil, nil, nil
			if !reflect.DeepEqual(rows[1], before) {
				t.Fatal("propagation changed more than the request binding")
			}
			again := propagateRequestedParticipantsAcrossEventCopies(propagateRequestedParticipantsAcrossEventCopies([]RuntimeDiagramRelation{bound(), tc.copy}))
			if !reflect.DeepEqual(again[1].requestedTo, tc.wantTo) {
				t.Fatalf("propagation is not idempotent: %v", again[1].requestedTo)
			}
		})
	}
	// Without a proved copy nothing is minted: a search alone stays unbound.
	rows := propagateRequestedParticipantsAcrossEventCopies([]RuntimeDiagramRelation{other(nil), other(func(r *RuntimeDiagramRelation) { r.FromNode = "rt_thread_w3" })})
	for _, row := range rows {
		if len(row.requestedFrom)+len(row.requestedTo) != 0 {
			t.Fatalf("a binding was minted without a proved copy: %+v", row)
		}
	}
	// The waking side keeps the same guard as the woken side: copies that
	// record two process numbers for it receive no binding.
	wakerBound := bound()
	wakerBound.requestedTo, wakerBound.requestedFrom = nil, []string{"requested"}
	rows = propagateRequestedParticipantsAcrossEventCopies([]RuntimeDiagramRelation{wakerBound, other(func(r *RuntimeDiagramRelation) { r.threadEndpoints.from.TGID = 11 })})
	if len(rows[1].requestedFrom) != 0 || len(rows[1].requestedFromCopied) != 0 {
		t.Fatalf("a binding crossed copies that record two process numbers on the waking side: %+v", rows[1])
	}
	// The binding follows the side: a waker-side binding never lands on the
	// woken side.
	source := bound()
	source.requestedTo, source.requestedFrom = nil, []string{"requested"}
	rows = propagateRequestedParticipantsAcrossEventCopies([]RuntimeDiagramRelation{source, other(nil)})
	if !reflect.DeepEqual(rows[1].requestedFrom, []string{"requested"}) || len(rows[1].requestedTo) != 0 {
		t.Fatalf("binding crossed sides: %+v", rows[1])
	}
}
