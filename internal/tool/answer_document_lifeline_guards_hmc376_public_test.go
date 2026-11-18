package tool

import (
	"encoding/json"
	"regexp"
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
	for _, row := range presented {
		if row.threadEndpoints.fromLifecycle != runtimeDiagramThreadLifecycleReused || row.threadEndpoints.toLifecycle != runtimeDiagramThreadLifecycleNone {
			t.Fatalf("the recorded exit and reuse of thread number 12 did not reach the wake rows exactly: %+v", row.threadEndpoints)
		}
	}
	if groups := runtimeDiagramThreadGroups(rows); runtimeDiagramSameThreadGroup(groups, presented[0].FromNode, presented[1].FromNode) ||
		!runtimeDiagramSameThreadGroup(groups, presented[0].ToNode, presented[1].ToNode) {
		t.Fatal("a reused thread number joined, or the unaffected woken thread did not")
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
