package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

var lifelineLongHexHMC376 = regexp.MustCompile(`[0-9a-f]{32,}`)

func lifelineRelationDeltaHMC376(t *testing.T, out types.ToolResult) types.AnswerDiagramRelationRepairDelta {
	t.Helper()
	var delta types.AnswerDiagramRelationRepairDelta
	if out.Repair == nil || json.Unmarshal([]byte(out.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta) != nil {
		t.Fatalf("rejection published no relation repair delta: %s", out.Summary)
	}
	return delta
}

// The numbered correction of one rejection that names the given issue.
func lifelineHintHMC376(t *testing.T, summary, issue string) string {
	t.Helper()
	for _, part := range strings.Split(summary, "\n  ") {
		if strings.Contains(part, "issue="+issue) {
			return part
		}
	}
	t.Fatalf("no correction names issue %q: %s", issue, summary)
	return ""
}

// Two different recorded threads on one lifeline stay rejected. The rejection
// carries its own issue name, lists only the arrows that disagree with the
// lifeline's first arrow, and one replace per listed arrow repairs it without
// removing any recorded wake arrow.
func TestHMC376PublicTwoThreadsOnOneLifelineRejected(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	if len(presented) != 3 || presented[0].threadEndpoints.from.PID != 11 || presented[1].threadEndpoints.to.PID != 12 || presented[2].threadEndpoints.from.PID != 12 {
		t.Fatalf("fixture changed: %+v", presented)
	}
	// loader-11 and helper-12 are different threads drawn as one lifeline.
	shared := func(row RuntimeDiagramRelation, from bool) (string, string) {
		id, label := lifelinePerThreadNodeHMC376(row, from)
		if id == "T11" || id == "T12" {
			return "Helpers", "辅助线程"
		}
		return id, label
	}
	doc := lifelineDocHMC376(rows, presented, shared)
	out := lifelineEmitHMC376(t, ctx, doc)
	if out.Success {
		t.Fatal("one lifeline for two different recorded threads was accepted")
	}
	if issues := lifelineIssuesHMC376(out.Summary); len(issues) != 1 || issues[0] != types.DiagramRelationFailureRuntimeLifelineTwoThreads {
		t.Fatalf("want only the lifeline issue, got %v: %s", issues, out.Summary)
	}
	hint := lifelineHintHMC376(t, out.Summary, types.DiagramRelationFailureRuntimeLifelineTwoThreads)
	for _, want := range []string{`Lifeline "Helpers" is thread loader-11 (its first arrow)`, `Arrow "唤醒2" (target-42 → helper-12) ends at thread helper-12: set its to_node to a lifeline of its own`,
		`Arrow "唤醒3" (helper-12 → target-42) starts from thread helper-12: set its from_node to a lifeline of its own`, `action "replace"`, "do not delete them"} {
		if !strings.Contains(hint, want) {
			t.Errorf("repair text lacks %q: %s", want, hint)
		}
	}
	if strings.Contains(hint, `"唤醒1"`) {
		t.Errorf("the arrow that agrees with the lifeline was listed: %s", hint)
	}
	if lifelineLongHexHMC376.MatchString(hint) || strings.Contains(hint, "from_node must denote from_identity") {
		t.Errorf("repair text carries long ids or the source-code direction teaching: %s", hint)
	}
	if len(hint) > 1500 {
		t.Errorf("repair text for two arrows is %d bytes", len(hint))
	}
	delta := lifelineRelationDeltaHMC376(t, out)
	if len(delta.Failures) != 2 {
		t.Fatalf("want the two odd arrows only, got %+v", delta.Failures)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(ctx.Mutable.LastRejectedAnswerDocumentV2(), delta.Failures, delta.AllowedAdditions)
	if lease == nil {
		t.Fatalf("no executable repair: %+v", delta)
	}
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	var edits []map[string]any
	for _, failure := range lease.Failures {
		if failure.Issue != types.DiagramRelationFailureRuntimeLifelineTwoThreads || !failure.AllowsAction("replace") || failure.FromIdentity == presented[0].FromIdentity {
			t.Fatalf("odd arrow lacks its replace operation or the first arrow was listed: %+v", failure)
		}
		edit := map[string]any{"failure_ref": failure.FailureRef, "action": "replace"}
		if failure.FromNode == "Helpers" {
			edit["edge"] = map[string]any{"from_node": "T12", "to_node": failure.ToNode, "visible_label": "唤醒3"}
			edit["from_node_visible_label"] = "helper-12"
		} else {
			edit["edge"] = map[string]any{"from_node": failure.FromNode, "to_node": "T12", "visible_label": "唤醒2"}
			edit["to_node_visible_label"] = "helper-12"
		}
		edits = append(edits, edit)
	}
	raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": edits})
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !patched.Success {
		t.Fatalf("one replace per listed arrow did not repair the diagram: %v %s", err, patched.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks[1].EdgeAnchors) != 3 {
		t.Fatalf("repair lost a recorded wake arrow: %+v", got)
	}
	for i, anchor := range got.Blocks[1].EdgeAnchors {
		if anchor.FromIdentity != presented[i].FromIdentity || anchor.ToIdentity != presented[i].ToIdentity {
			t.Fatalf("repair changed the recorded identity or order of arrow %d: %+v", i+1, got.Blocks[1].EdgeAnchors)
		}
	}
}

const lifelineTwoProcessesTraceHMC376 = `# tracer: nop
#           TASK-PID   CPU#    TIMESTAMP  FUNCTION
target-42 (20) [001] .... 1.000000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
helper-12 (10) [000] .... 1.003000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (20) [001] .... 1.004000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
target-42 (20) [001] .... 1.006000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
helper-12 (30) [002] .... 1.009000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (20) [001] .... 1.010000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
`

// One thread number recorded under two process numbers is two tasks. Their
// lanes are not joined, so one lifeline for both stays rejected.
func TestHMC376PublicSameNumberTwoProcessesNotJoined(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineTwoProcessesTraceHMC376, "wakeup_chain")
	presented := lifelinePresentedWakesHMC376(rows)
	if len(presented) != 2 || presented[0].threadEndpoints.from != (types.RuntimeWakeupThread{Comm: "helper", PID: 12, TGID: 10}) ||
		presented[1].threadEndpoints.from != (types.RuntimeWakeupThread{Comm: "helper", PID: 12, TGID: 30}) || presented[0].FromNode == presented[1].FromNode {
		t.Fatalf("fixture must record thread number 12 under two process numbers on two lanes: %+v", presented)
	}
	if groups := runtimeDiagramThreadGroups(rows); runtimeDiagramSameThreadGroup(groups, presented[0].FromNode, presented[1].FromNode) ||
		!runtimeDiagramSameThreadGroup(groups, presented[0].ToNode, presented[1].ToNode) {
		t.Fatal("two process numbers joined, or the uncontradicted woken thread did not")
	}
	out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, presented, lifelinePerThreadNodeHMC376))
	if out.Success {
		t.Fatal("one lifeline for one thread number under two process numbers was accepted")
	}
	if issues := lifelineIssuesHMC376(out.Summary); len(issues) != 1 || issues[0] != types.DiagramRelationFailureRuntimeLifelineTwoThreads {
		t.Fatalf("want only the lifeline issue, got %v: %s", issues, out.Summary)
	}
	hint := lifelineHintHMC376(t, out.Summary, types.DiagramRelationFailureRuntimeLifelineTwoThreads)
	if !strings.Contains(hint, "the recordings of thread number 12 are kept apart because two different process numbers are recorded for it") || strings.Contains(hint, `"唤醒1"`) {
		t.Fatalf("rejection does not explain the kept-apart recordings or lists the first arrow: %s", hint)
	}
	// Drawn as published, on two lifelines, the same rows are accepted.
	ctx, rows = lifelineFixtureHMC376(t, lifelineTwoProcessesTraceHMC376, "wakeup_chain")
	if out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, lifelinePresentedWakesHMC376(rows), lifelinePublishedNodeHMC376)); !out.Success {
		t.Fatalf("published rows rejected: %s", out.Summary)
	}
}

// Thread number 12 exits and is created again inside the window.
const lifelineReusedNumberTraceHMC376 = `# tracer: nop
#           TASK-PID   CPU#    TIMESTAMP  FUNCTION
target-42 (20) [001] .... 1.000000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
helper-12 (10) [000] .... 1.003000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (20) [001] .... 1.004000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
helper-12 (10) [000] .... 1.005000: sched_switch: prev_comm=helper prev_pid=12 prev_prio=120 prev_state=X ==> next_comm=idle next_pid=0 next_prio=120
target-42 (20) [001] .... 1.006000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
spawner-11 (10) [002] .... 1.007000: sched_wakeup_new: comm=fresh pid=12 prio=120 target_cpu=0
fresh-12 (10) [000] .... 1.009000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (20) [001] .... 1.010000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
`

// A recorded exit and reuse of one thread number inside the capture means the
// number may denote two tasks. Its lanes are not joined, so one lifeline for
// both recordings stays rejected even though one process number is recorded.
// The lifecycle record comes from one lookup and the wake rows from another;
// both speak about the same capture.
func TestHMC376PublicIncarnationBoundaryNotJoined(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineReusedNumberTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	if len(presented) != 2 || presented[0].threadEndpoints.from.PID != 12 || presented[1].threadEndpoints.from.PID != 12 ||
		presented[0].threadEndpoints.from.TGID != presented[1].threadEndpoints.from.TGID || presented[0].FromNode == presented[1].FromNode {
		t.Fatalf("fixture must record the reused thread number on two lanes with one process number: %+v", presented)
	}
	// Re-based with the fail-closed review ruling: the records of one lookup do
	// not list every reused number, so the woken thread, which no record names,
	// is unproven too. The named number keeps its exact reason.
	for _, row := range presented {
		if row.threadEndpoints.fromLifecycle != runtimeDiagramThreadLifecycleReused || row.threadEndpoints.toLifecycle != runtimeDiagramThreadLifecycleIncomplete {
			t.Fatalf("the recorded exit and reuse of thread number 12 did not reach the wake rows exactly: %+v", row.threadEndpoints)
		}
	}
	groups := runtimeDiagramThreadGroups(rows)
	if runtimeDiagramSameThreadGroup(groups, presented[0].FromNode, presented[1].FromNode) {
		t.Fatal("a reused thread number joined")
	}
	if runtimeDiagramLaneJoined(groups, presented[0].ToNode) {
		t.Fatal("a thread number of a capture with a lifecycle record joined")
	}
	out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, presented, lifelinePerThreadNodeHMC376))
	if out.Success {
		t.Fatal("one lifeline across a recorded exit and reuse of its thread number was accepted")
	}
	if issues := lifelineIssuesHMC376(out.Summary); len(issues) != 1 || issues[0] != types.DiagramRelationFailureRuntimeLifelineTwoThreads {
		t.Fatalf("want only the lifeline issue, got %v: %s", issues, out.Summary)
	}
	hint := lifelineHintHMC376(t, out.Summary, types.DiagramRelationFailureRuntimeLifelineTwoThreads)
	if !strings.Contains(hint, "the recordings of thread number 12 are kept apart because the capture records an exit and reuse of this thread number") {
		t.Fatalf("rejection does not explain the kept-apart recordings: %s", hint)
	}
	ctx, rows = lifelineFixtureHMC376(t, lifelineReusedNumberTraceHMC376, "wakeup_chain", "event_search")
	if out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, lifelinePresentedWakesHMC376(rows), lifelinePublishedNodeHMC376)); !out.Success {
		t.Fatalf("published rows rejected: %s", out.Summary)
	}
}

// Each helper thread number wakes the target, exits, is created again under
// another name and wakes the target once more.
func lifelineReusedNumbersTraceHMC376(numbers int) string {
	var b strings.Builder
	b.WriteString("# tracer: nop\n#           TASK-PID   CPU#    TIMESTAMP  FUNCTION\n")
	ts := 1.0
	tick := func() string { ts += 0.0002; return fmt.Sprintf("%.6f", ts) }
	sleep := func() {
		fmt.Fprintf(&b, "target-42 (20) [001] .... %s: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n", tick())
	}
	run := func() {
		fmt.Fprintf(&b, "target-42 (20) [001] .... %s: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110\n", tick())
	}
	for tid := 12; tid < 12+numbers; tid++ {
		sleep()
		fmt.Fprintf(&b, "old%d-%d (10) [000] .... %s: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1\n", tid, tid, tick())
		run()
		fmt.Fprintf(&b, "old%d-%d (10) [000] .... %s: sched_switch: prev_comm=old%d prev_pid=%d prev_prio=120 prev_state=X ==> next_comm=idle next_pid=0 next_prio=120\n", tid, tid, tick(), tid, tid)
		sleep()
		fmt.Fprintf(&b, "spawner-11 (10) [002] .... %s: sched_wakeup_new: comm=new%d pid=%d prio=120 target_cpu=0\n", tick(), tid, tid)
		fmt.Fprintf(&b, "new%d-%d (10) [000] .... %s: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1\n", tid, tid, tick())
		run()
	}
	return b.String()
}

// The lookup publishes fewer lifecycle records than there are reused numbers.
// A number the roster omits is not proven to be one task, so its two tasks
// never share one lifeline, while the rows copied as published stay accepted.
func TestHMC376PublicEveryReusedNumberStaysApart(t *testing.T) {
	for _, numbers := range []int{2, 3, 5, 6} {
		t.Run(fmt.Sprintf("%d reused numbers", numbers), func(t *testing.T) {
			trace := lifelineReusedNumbersTraceHMC376(numbers)
			ctx, rows := lifelineFixtureHMC376(t, trace, "wakeup_chain", "event_search")
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			named := map[string]bool{}
			for _, record := range ledger.Records {
				if record.Predicate == "thread_incarnation_suppression" {
					named[traceSupplementRichNoteValue(record.RichNotes, "conflict_tid")] = true
				}
			}
			last := 12 + numbers - 1
			if len(named) == 0 || named[fmt.Sprint(last)] {
				t.Fatalf("fixture must leave thread number %d off a non-empty roster: %v", last, named)
			}
			seen := map[string]bool{}
			var edges []RuntimeDiagramRelation
			for _, row := range rows {
				if row.Kind == types.DiagramRelWakeup && row.threadEndpoints != nil && row.eventOrder != nil && row.threadEndpoints.from.PID == last && !seen[row.SupportRefs[0]] {
					seen[row.SupportRefs[0]] = true
					edges = append(edges, row)
				}
			}
			sort.SliceStable(edges, func(i, j int) bool { return edges[i].eventOrder.timestamp < edges[j].eventOrder.timestamp })
			if len(edges) != 2 || edges[0].threadEndpoints.from.Comm == edges[1].threadEndpoints.from.Comm || edges[0].FromNode == edges[1].FromNode {
				t.Fatalf("fixture must record two tasks of thread number %d on two lanes: %+v", last, edges)
			}
			out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, edges, lifelinePerThreadNodeHMC376))
			if out.Success {
				t.Fatalf("two tasks of thread number %d were accepted on one lifeline", last)
			}
			if issues := lifelineIssuesHMC376(out.Summary); len(issues) != 1 || issues[0] != types.DiagramRelationFailureRuntimeLifelineTwoThreads {
				t.Fatalf("want only the lifeline issue, got %v: %s", issues, out.Summary)
			}
			hint := lifelineHintHMC376(t, out.Summary, types.DiagramRelationFailureRuntimeLifelineTwoThreads)
			if want := fmt.Sprintf("the recordings of thread number %d are kept apart because the capture's record of thread exits and reuses is incomplete", last); !strings.Contains(hint, want) {
				t.Fatalf("rejection does not explain the kept-apart recordings: %s", hint)
			}
			ctx, rows = lifelineFixtureHMC376(t, trace, "wakeup_chain", "event_search")
			var published []RuntimeDiagramRelation
			for _, row := range rows {
				if row.Kind == types.DiagramRelWakeup && row.threadEndpoints != nil && row.eventOrder != nil && row.threadEndpoints.from.PID == last && len(row.requestedTo) > 0 {
					published = append(published, row)
				}
			}
			sort.SliceStable(published, func(i, j int) bool { return published[i].eventOrder.timestamp < published[j].eventOrder.timestamp })
			if out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, published[:2], lifelinePublishedNodeHMC376)); !out.Success {
				t.Fatalf("published rows rejected: %s", out.Summary)
			}
		})
	}
}

func lifelineQueryFixtureHMC376(t *testing.T, trace string, queries []map[string]any) (*types.BusContext, []RuntimeDiagramRelation) {
	t.Helper()
	ctx, _ := lifelineFixtureHMC376(t, trace)
	path := filepath.Join(t.TempDir(), "lookups.systrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	var results []types.ToolResult
	for _, query := range queries {
		query["path"] = path
		result := businessRefTestQuery(t, ctx, query)
		if !result.Success {
			t.Fatalf("%v: %s", query, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx, runtimeDiagramRelationsForContext(ctx)
}

// Thread number 42 exits and is created again after the bound wake.
const lifelineRequestedReusedTraceHMC376 = `# tracer: nop
#           TASK-PID   CPU#    TIMESTAMP  FUNCTION
target-42 (20) [001] .... 1.000000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
loader-11 (10) [000] .... 1.003000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (20) [001] .... 1.004000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
target-42 (20) [001] .... 1.005000: sched_wakeup: comm=helper pid=12 prio=120 target_cpu=2
target-42 (20) [001] .... 1.006000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=X ==> next_comm=idle next_pid=0 next_prio=120
spawner-11 (10) [002] .... 1.007000: sched_wakeup_new: comm=target pid=42 prio=120 target_cpu=1
helper-12 (10) [002] .... 1.009000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (30) [001] .... 1.010000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
`

// One lookup binds the requested thread on one wake only. The other lookup's
// copy of that wake carries the binding, but the other wakes on that copy's
// lane were bound by no lookup and do not cover the requested participant.
func TestHMC376PublicCopiedBindingCoversItsOwnEventOnly(t *testing.T) {
	for name, trace := range map[string]string{"one task": lifelineBothRolesTraceHMC376, "the number is reused after the bound wake": lifelineRequestedReusedTraceHMC376} {
		t.Run(name, func(t *testing.T) {
			lookups := func() []map[string]any {
				return []map[string]any{
					{"view": "wakeup_chain", "pid": 42, "time_start": 1.0, "time_end": 1.0045},
					{"view": "event_search", "pid": 42, "event_types": []string{"sched_wakeup"}, "time_start": 1.0, "time_end": 1.02},
				}
			}
			wake := func(rows []RuntimeDiagramRelation, at float64) RuntimeDiagramRelation {
				var copies []RuntimeDiagramRelation
				for _, row := range rows {
					if row.threadEndpoints != nil && row.eventOrder != nil && row.eventOrder.timestamp == at && row.threadEndpoints.to.PID == 42 && len(row.requestedToCopied)+len(row.requestedTo) == 0 {
						copies = append(copies, row)
					}
				}
				if at == 1.003 {
					for _, row := range rows {
						if row.threadEndpoints != nil && row.eventOrder != nil && row.eventOrder.timestamp == at && len(row.requestedToCopied) == 1 {
							copies = append(copies, row)
						}
					}
				}
				if len(copies) != 1 {
					t.Fatalf("fixture must publish one such row at %v, got %d: %+v", at, len(copies), rows)
				}
				return copies[0]
			}
			ctx, rows := lifelineQueryFixtureHMC376(t, trace, lookups())
			late := wake(rows, 1.009)
			out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, []RuntimeDiagramRelation{late}, lifelinePublishedNodeHMC376))
			if issues := lifelineIssuesHMC376(out.Summary); out.Success || len(issues) != 1 || issues[0] != string(DiagramParticipantCoverageIdentityMissing) {
				t.Fatalf("a wake no lookup bound covered the requested participant: success=%t %v: %s", out.Success, issues, out.Summary)
			}
			// The copy of the bound wake, on that same lane, does cover it.
			ctx, rows = lifelineQueryFixtureHMC376(t, trace, lookups())
			bound := wake(rows, 1.003)
			if late := wake(rows, 1.009); bound.ToNode != late.ToNode {
				t.Fatalf("fixture must publish both wakes on one lane: %s %s", bound.ToNode, late.ToNode)
			}
			if out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, []RuntimeDiagramRelation{bound}, lifelinePublishedNodeHMC376)); !out.Success {
				t.Fatalf("the copy of the bound wake was rejected: %s", out.Summary)
			}
		})
	}
}

// Two selected aliases of one thread, one of them also carrying arrows of the
// lane the other selected, cannot be displayed consistently. The draft comes
// back unchanged with both aliases named, in either selection order.
func TestHMC376PublicSelectionsHostingEachOtherAreRepairable(t *testing.T) {
	build := func(t *testing.T) (*types.BusContext, []RuntimeDiagramRelation, *types.AnswerDocumentV2, []RuntimeDiagramRelation) {
		ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
		presented := lifelinePresentedWakesHMC376(rows)
		if presented[0].ToNode != presented[2].ToNode || presented[0].ToNode == presented[1].FromNode {
			t.Fatalf("fixture must publish the thread on two lanes: %+v", presented)
		}
		doc := lifelineDocHMC376(rows, presented, func(row RuntimeDiagramRelation, from bool) (string, string) {
			id, label := lifelinePerThreadNodeHMC376(row, from)
			if id == "T42" && row.FromIdentity == presented[2].FromIdentity {
				return "U42", label
			}
			return id, label
		})
		return ctx, rows, doc, presented
	}
	for name, selections := range map[string]func(p []RuntimeDiagramRelation) []types.RuntimeDiagramParticipantSelection{
		"the first arrow's lane is selected by the other alias": func(p []RuntimeDiagramRelation) []types.RuntimeDiagramParticipantSelection {
			return []types.RuntimeDiagramParticipantSelection{{NodeID: "T42", NativeParticipant: p[1].FromNode}, {NodeID: "U42", NativeParticipant: p[2].ToNode}}
		},
		"the alias hosts the lane selected first": func(p []RuntimeDiagramRelation) []types.RuntimeDiagramParticipantSelection {
			return []types.RuntimeDiagramParticipantSelection{{NodeID: "T42", NativeParticipant: p[2].ToNode}, {NodeID: "U42", NativeParticipant: p[1].FromNode}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, _, doc, presented := build(t)
			doc.Blocks[1].Diagram.RuntimeParticipantsV1 = selections(presented)
			before, _ := json.Marshal(doc)
			out := lifelineEmitHMC376(t, ctx, doc)
			if out.Success {
				t.Fatal("two selected aliases hosting each other's lane were accepted")
			}
			for _, want := range []string{`"T42"`, `"U42"`, "both selected for one recorded thread", "keep only that participant's selection"} {
				if !strings.Contains(out.Summary, want) {
					t.Fatalf("rejection lacks %q: %s", want, out.Summary)
				}
			}
			if strings.Contains(out.Summary, "must select one unique existing declaration once") || lifelineLongHexHMC376.MatchString(out.Summary) {
				t.Fatalf("rejection reports a declaration the draft does have, or long ids: %s", out.Summary)
			}
			if after, _ := json.Marshal(doc); string(before) != string(after) {
				t.Fatal("the rejected draft was rewritten")
			}
		})
	}
	// Each alias hosting only its own lane stays two lifelines: the display
	// choice that is not merged by this lane.
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	doc := lifelineDocHMC376(rows, presented, func(row RuntimeDiagramRelation, from bool) (string, string) {
		id, label := lifelinePerThreadNodeHMC376(row, from)
		if id == "T42" && (from && row.FromNode == presented[1].FromNode || !from && row.ToNode == presented[1].FromNode) {
			return "U42", label
		}
		return id, label
	})
	doc.Blocks[1].Diagram.RuntimeParticipantsV1 = []types.RuntimeDiagramParticipantSelection{{NodeID: "T42", NativeParticipant: presented[0].ToNode}, {NodeID: "U42", NativeParticipant: presented[1].FromNode}}
	if out := lifelineEmitHMC376(t, ctx, doc); !out.Success {
		t.Fatalf("two aliases that each host their own lane were rejected: %s", out.Summary)
	}
}

// Current limit, recorded and pinned: the native order gate compares events of
// one order scope, and the scope includes the lookup's scan window (an earlier
// ruling pinned by TestNativeSequenceOrderHMC230ScopeComparability). Two
// lookups of one capture therefore stay incomparable to the gate, so reversed
// arrows on one joined lifeline are not rejected. The capsule teaches the
// recorded order for such arrows; a hard gate needs an owner ruling.
func TestHMC376PublicOrderAcrossLookupsIsTaughtNotGated(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	var early, late *RuntimeDiagramRelation
	for i := range rows {
		row := &rows[i]
		if row.threadEndpoints == nil || row.eventOrder == nil || row.threadEndpoints.to.PID != 42 {
			continue
		}
		if row.eventOrder.timestamp == 1.003 && early == nil {
			early = row
		}
	}
	for i := range rows {
		row := &rows[i]
		if row.threadEndpoints != nil && row.eventOrder != nil && row.threadEndpoints.to.PID == 42 && row.eventOrder.timestamp == 1.009 && row.eventOrder.scope != early.eventOrder.scope {
			late = row
		}
	}
	if early == nil || late == nil {
		t.Fatalf("fixture must publish the two wakes from two lookups: %+v", rows)
	}
	if out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, []RuntimeDiagramRelation{*late, *early}, lifelinePerThreadNodeHMC376)); !out.Success {
		t.Fatalf("the order gate's scope changed; this needs the owner ruling named above: %s", out.Summary)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	if text := RenderRuntimeDiagramRelationRecipes(ledger, &ctx.AnalysisIR.RequestModel); !strings.Contains(text, "the arrows of one lifeline whichever lookup published them, in native timestamp order") {
		t.Fatalf("the capsule does not teach the recorded order across lookups: %s", text)
	}
}
