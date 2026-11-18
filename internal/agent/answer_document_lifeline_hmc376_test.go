package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The requested thread is woken twice and wakes one helper.
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

var (
	lifelineRowThreadsHMC376 = regexp.MustCompile(`；发起线程号=(\d+)；被唤醒线程号=(\d+)；edge_anchor=`)
	lifelineRowTimeHMC376    = regexp.MustCompile(`；唤醒时刻=([0-9.]+)秒；`)
	lifelineRowLabelsHMC376  = regexp.MustCompile(`^- "([^"]+)" → "([^"]+)"；`)
)

// The actual finalizer instruction, built by the real IR compiler and the
// two ordinary lookups, teaches the lifeline rule once and publishes enough
// typed fields for it: a diagram drawn only from that instruction, with one
// lifeline per published thread number, is accepted by the public answer
// tool with every published wake arrow kept.
func TestHMC376LifelineActualFinalizerInstruction(t *testing.T) {
	ctx, raw := requestedThreadAgentInputHMC242(t)
	path := filepath.Join(t.TempDir(), "both-roles.systrace")
	if err := os.WriteFile(path, []byte(lifelineBothRolesTraceHMC376), 0600); err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace, ctx.AttachedHitraceSource = lifelineBothRolesTraceHMC376, path
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: path, Signals: []string{"sched_switch", "sched_wakeup"}}})
	res, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), raw)
	if err != nil || !res.Success {
		t.Fatalf("public analyzer: %v %s", err, res.Summary)
	}
	if ctx.AnalysisIR, err = buildAnalysisIR(ctx); err != nil {
		t.Fatal(err)
	}
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	var results []types.ToolResult
	for _, query := range []string{
		`{"source":"attached_trace","view":"wakeup_chain","pid":42,"time_start":1,"time_end":1.02}`,
		`{"source":"attached_trace","view":"event_search","pid":42,"event_types":["sched_wakeup"],"time_start":1,"time_end":1.02}`,
	} {
		res, err = (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(query))
		if err != nil || !res.Success {
			t.Fatalf("public native query: %v %s", err, res.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(res)
		results = append(results, res)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Count(prompt, "One lifeline per recorded thread") != 1 || !strings.Contains(prompt, "Two different threads never share a lifeline.") {
		t.Fatalf("the finalizer instruction must state the lifeline rule exactly once")
	}
	for _, gone := range []string{"cannot merge different producer lanes", "Do not create a second generic lane", "不能混用端点"} {
		if strings.Contains(prompt, gone) {
			t.Fatalf("the finalizer instruction still carries the contradicting teaching %q", gone)
		}
	}
	if got := strings.Count(prompt, "requested_participant_binding="); got != 2 {
		t.Fatalf("want the request binding under both published wakes of the requested thread, got %d", got)
	}

	type published struct {
		ts         float64
		from, to   int
		anchor     types.DiagramEdgeAnchor
		fromL, toL string
	}
	var rows []published
	for _, line := range strings.Split(prompt, "\n") {
		threads := lifelineRowThreadsHMC376.FindStringSubmatch(line)
		at, labels := lifelineRowTimeHMC376.FindStringSubmatch(line), lifelineRowLabelsHMC376.FindStringSubmatch(line)
		if threads == nil || at == nil || labels == nil {
			continue
		}
		row := published{fromL: labels[1], toL: labels[2]}
		row.from, _ = strconv.Atoi(threads[1])
		row.to, _ = strconv.Atoi(threads[2])
		row.ts, _ = strconv.ParseFloat(at[1], 64)
		if err := json.Unmarshal([]byte(line[strings.Index(line, "edge_anchor=")+len("edge_anchor="):]), &row.anchor); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if len(rows) != 3 {
		t.Fatalf("the instruction must publish the three recorded wakes with their thread numbers, got %d", len(rows))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ts < rows[j].ts })
	// The same thread number is published under different node ids.
	if rows[0].anchor.ToNode == rows[1].anchor.FromNode || rows[0].to != rows[1].from {
		t.Fatalf("fixture must publish thread 42 under two node ids: %+v", rows)
	}
	var body strings.Builder
	body.WriteString("sequenceDiagram\n")
	seen := map[int]bool{}
	for _, row := range rows {
		for _, side := range []struct {
			tid   int
			label string
		}{{row.from, row.fromL}, {row.to, row.toL}} {
			if !seen[side.tid] {
				seen[side.tid] = true
				fmt.Fprintf(&body, " participant T%d as %s\n", side.tid, side.label)
			}
		}
	}
	var anchors []types.DiagramEdgeAnchor
	for i, row := range rows {
		label := fmt.Sprintf("唤醒%d", i+1)
		fmt.Fprintf(&body, " T%d->>T%d: %s\n", row.from, row.to, label)
		anchor := row.anchor
		anchor.FromNode, anchor.ToNode, anchor.VisibleLabel = fmt.Sprintf("T%d", row.from), fmt.Sprintf("T%d", row.to), label
		anchors = append(anchors, anchor)
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "记录三次唤醒，不表示唤醒者造成了全部等待。", SurfaceRole: types.SurfacePrincipal,
			TraceCausalClaimCaliber: types.TraceCausalClaimNoConclusion, FacetIDs: []string{string(types.FacetObservedArtifactFact)}},
		{ID: "wakes", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: body.String()},
			EdgeAnchors: anchors, FacetIDs: []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}},
	}}
	payload, _ := json.Marshal(doc)
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, payload)
	if err != nil || !result.Success {
		t.Fatalf("a diagram drawn from the finalizer instruction, one lifeline per thread, was rejected: %v %s", err, result.Summary)
	}
	accepted := ctx.Mutable.AnswerDocumentV2()
	if accepted == nil || len(accepted.Blocks) < 2 || len(accepted.Blocks[1].EdgeAnchors) != 3 {
		t.Fatalf("acceptance lost a recorded wake arrow: %+v", accepted)
	}
	if got := tool.DiagramParticipantCoverageMismatchesWithRuntimeContext(bus, accepted, types.BuildAnswerSemanticViewForBusContext(bus), nil); len(got) > 0 {
		t.Fatalf("finalizer coverage disagrees with public acceptance: %+v", got)
	}
}
