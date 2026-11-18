package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// One recorded thread that is both woken and a waker, explored with the two
// ordinary public views. A lookup that records the woken side without its
// process number publishes that thread under a second lane; the lifeline a
// reader draws still stands for the one recorded thread. Every case below
// enters through the public analysis, query and answer tools.
const lifelineBothRolesTraceHMC376 = `# tracer: nop
#           TASK-PID   CPU#    TIMESTAMP  FUNCTION
target-42 (20) [001] .... 1.000000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
loader-11 (10) [000] .... 1.003000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (20) [001] .... 1.004000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
target-42 (20) [001] .... 1.005000: sched_wakeup: comm=helper pid=12 prio=120 target_cpu=2
target-42 (20) [001] .... 1.006000: sched_switch: prev_comm=target prev_pid=42 prev_prio=110 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
helper-12 (10) [002] .... 1.009000: sched_wakeup: comm=target pid=42 prio=110 target_cpu=1
target-42 (20) [001] .... 1.010000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=110
`

func lifelineFixtureHMC376(t *testing.T, trace string, views ...string) (*types.BusContext, []RuntimeDiagramRelation) {
	t.Helper()
	ctx, payload, _ := requestedThreadAnalysisInputHMC242(t, "", nil)
	path := filepath.Join(t.TempDir(), "both-roles.systrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace = trace
	raw, _ := json.Marshal(payload)
	if out, err := (&EmitAnalysis{}).Execute(ctx, raw); err != nil || !out.Success {
		t.Fatalf("public analysis: %v %s", err, out.Summary)
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil || rm.DiagramHint == nil || len(rm.DiagramHint.Participants) != 1 {
		t.Fatalf("analysis lost the requested participant: %+v", rm)
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, RequiredKind: rm.DiagramHint.Kind, Participants: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)}}}
	var results []types.ToolResult
	for _, view := range views {
		q := map[string]any{"path": path, "view": view, "pid": 42, "time_start": 1.0, "time_end": 1.02}
		if view == "event_search" {
			q["event_types"] = []string{"sched_wakeup"}
		}
		r := businessRefTestQuery(t, ctx, q)
		if !r.Success {
			t.Fatalf("%s: %s", view, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx, runtimeDiagramRelationsForContext(ctx)
}

// The wake rows the authoring capsule presents, in recorded order.
func lifelinePresentedWakesHMC376(rows []RuntimeDiagramRelation) []RuntimeDiagramRelation {
	var out []RuntimeDiagramRelation
	for _, r := range runtimeDiagramPresentationRows(rows) {
		if r.Kind == types.DiagramRelWakeup && r.threadEndpoints != nil && r.eventOrder != nil {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].eventOrder.timestamp < out[j].eventOrder.timestamp })
	return out
}

type lifelineNodeHMC376 func(row RuntimeDiagramRelation, from bool) (id, label string)

// Exactly what the capsule publishes for a row: its node ids and labels.
func lifelinePublishedNodeHMC376(row RuntimeDiagramRelation, from bool) (string, string) {
	if from {
		return row.FromNode, row.FromLabel
	}
	return row.ToNode, row.ToLabel
}

// One model-chosen lifeline per recorded thread number.
func lifelinePerThreadNodeHMC376(row RuntimeDiagramRelation, from bool) (string, string) {
	thread, label := row.threadEndpoints.to, row.ToLabel
	if from {
		thread, label = row.threadEndpoints.from, row.FromLabel
	}
	return fmt.Sprintf("T%d", thread.PID), label
}

func lifelineDocHMC376(rows, edges []RuntimeDiagramRelation, node lifelineNodeHMC376) *types.AnswerDocumentV2 {
	doc := requestedThreadDocHMC242(rows, false)
	doc.Blocks[0].Text = "该窗口记录数次唤醒；线程身份来自同一份原生事件，不表示唤醒者造成全部等待。"
	block := &doc.Blocks[1]
	block.EdgeAnchors = nil
	var b strings.Builder
	b.WriteString("sequenceDiagram\n")
	seen := map[string]bool{}
	for _, row := range edges {
		for _, from := range []bool{true, false} {
			if id, label := node(row, from); !seen[id] {
				seen[id] = true
				fmt.Fprintf(&b, "    participant %s as %s\n", id, label)
			}
		}
	}
	for i, row := range edges {
		from, _ := node(row, true)
		to, _ := node(row, false)
		label := fmt.Sprintf("唤醒%d", i+1)
		fmt.Fprintf(&b, "    %s->>%s: %s\n", from, to, label)
		block.EdgeAnchors = append(block.EdgeAnchors, types.DiagramEdgeAnchor{FromNode: from, ToNode: to, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: label})
	}
	block.Diagram.Body = strings.TrimRight(b.String(), "\n")
	return doc
}

func lifelineEmitHMC376(t *testing.T, ctx *types.BusContext, doc *types.AnswerDocumentV2) types.ToolResult {
	t.Helper()
	raw, _ := json.Marshal(doc)
	out, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil {
		t.Fatalf("public emit: %v", err)
	}
	return out
}

func lifelineLanesByThreadHMC376(rows []RuntimeDiagramRelation) map[int]map[string]bool {
	lanes := map[int]map[string]bool{}
	for _, r := range rows {
		if r.threadEndpoints == nil {
			continue
		}
		for _, side := range []struct {
			tid  int
			node string
		}{{r.threadEndpoints.from.PID, r.FromNode}, {r.threadEndpoints.to.PID, r.ToNode}} {
			if lanes[side.tid] == nil {
				lanes[side.tid] = map[string]bool{}
			}
			lanes[side.tid][side.node] = true
		}
	}
	return lanes
}

// The class invariant: every row the capsule publishes can be emitted back
// verbatim (its own node ids, labels and identities) and is accepted.
func TestHMC376PublicPublishedRowsEmitBackVerbatim(t *testing.T) {
	for _, mix := range [][]string{{"wakeup_chain"}, {"wakeup_chain", "event_search"}, {"event_search", "wakeup_chain"}} {
		t.Run(strings.Join(mix, "+"), func(t *testing.T) {
			ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, mix...)
			presented := lifelinePresentedWakesHMC376(rows)
			if len(mix) == 2 {
				if len(presented) != 3 || len(lifelineLanesByThreadHMC376(rows)[42]) < 2 {
					t.Fatalf("fixture must publish the both-roles thread under several lanes: presented=%d lanes=%d", len(presented), len(lifelineLanesByThreadHMC376(rows)[42]))
				}
			}
			out := lifelineEmitHMC376(t, ctx, lifelineDocHMC376(rows, presented, lifelinePublishedNodeHMC376))
			if !out.Success {
				t.Fatalf("a diagram copied from the published rows was rejected: %s", out.Summary)
			}
			got := ctx.Mutable.AnswerDocumentV2()
			if got == nil || len(got.Blocks[1].EdgeAnchors) != len(presented) {
				t.Fatalf("accepted document lost recorded wake arrows: %+v", got)
			}
			view := types.BuildAnswerSemanticViewForBusContext(ctx)
			if mismatches := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, got, view, nil); len(mismatches) != 0 {
				t.Fatalf("post gate disagrees with public acceptance: %+v", mismatches)
			}
			if mismatches := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, got, view, nil); len(mismatches) != 0 {
				t.Fatalf("post relation gate disagrees with public acceptance: %+v", mismatches)
			}
		})
	}
}

// One lifeline per recorded thread under the author's own aliases, every
// arrow keeping its own recorded identities.
func TestHMC376PublicOneLifelinePerThread(t *testing.T) {
	ctx, rows := lifelineFixtureHMC376(t, lifelineBothRolesTraceHMC376, "wakeup_chain", "event_search")
	presented := lifelinePresentedWakesHMC376(rows)
	doc := lifelineDocHMC376(rows, presented, lifelinePerThreadNodeHMC376)
	out := lifelineEmitHMC376(t, ctx, doc)
	if !out.Success {
		t.Fatalf("one lifeline per recorded thread was rejected: %s", out.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks[1].EdgeAnchors) != 3 {
		t.Fatalf("accepted document lost recorded wake arrows: %+v", got)
	}
	for i, anchor := range got.Blocks[1].EdgeAnchors {
		want := doc.Blocks[1].EdgeAnchors[i]
		if anchor.FromNode != want.FromNode || anchor.ToNode != want.ToNode || anchor.FromIdentity != want.FromIdentity || anchor.ToIdentity != want.ToIdentity {
			t.Fatalf("acceptance rewrote arrow %d: got %+v want %+v", i+1, anchor, want)
		}
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if mismatches := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, got, view, nil); len(mismatches) != 0 {
		t.Fatalf("post gate disagrees with public acceptance: %+v", mismatches)
	}
}
