package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const lifelineHoneybirdQuestionHMC376 = "请分析这份蜂鸟众包冷启动采集里 11343.248 到 11343.310 秒主线程（tid 64156）的这次停顿。分开说明运行、就绪和休眠期间发生了什么，并画一张时序图，把这段启动工作和唤醒的先后关系讲清楚。能定位停顿原因吗？"

// The accepted public analysis of that question as one earlier run submitted
// it. It is replayed through the public analysis entry, never injected.
const lifelineHoneybirdAnalysisHMC376 = `{"answer_role_profile":{"confidence":0.85,"is_role_binding_requested":false,"rationale":"用户未请求选出某一类角色项，仅要求分析、说明与图示"},"answer_subject":{"confidence":0.6,"entity_axes":["线程 tid 64156 → 调度状态分区（运行/就绪/休眠）","线程 tid 64156 → 唤醒事件先后关系"],"kind":"generic"},"call_chain_endpoints":{"sink":"","sink_mode":"exact","source":""},"completeness_obligation":{"required":false,"source_quote":""},"complexity":"moderate","complexity_confidence":0.7,"diagram_hint":{"kind":"sequence","participants":[{"identity":"64156","role":"incident_required","source_quote":"主线程（tid 64156）"}],"relation_scope_quote":"画一张时序图，把这段启动工作和唤醒的先后关系讲清楚","required":true},"diagnostic_profile":{"confidence":0.8,"current_risk":false,"current_version_check":false,"historical_regression":false,"is_diagnostic":true,"observation_summary":"用户报告蜂鸟众包冷启动采集中主线程 tid 64156 在 11343.248~11343.310 秒出现一次停顿"},"entities":["64156"],"error_granularity_profile":{"confidence":0.85,"is_granularity_question":false,"rationale":"未询问失败在批次/成员间如何传播"},"history_selection_profile":{"confidence":0.9,"item_kind":"not_applicable","mode":"not_applicable","rationale":"非仓库历史问题"},"intent":"root_cause","intent_confidence":0.85,"keywords":["64156","主线程","sched_switch","sched_wakeup","sched_waking","sched_blocked_reason","runnable","D 状态","S 状态","R 状态","唤醒链","wakeup","冷启动","启动工作","停顿","阻塞","latency","prev_state","waker"],"kind_confidence":0.55,"language":"zh","predicate_axis":"flow","predicates":{"has_per_member_table":false,"is_category_enumeration":false,"is_count_question":false,"is_cross_component":false,"is_diagnostic_question":true,"is_history_lookup":false,"is_relational_lookup":false,"is_role_locate_lookup":false,"is_scalar_answer":false},"question_kind":"mechanism","requested_answer_dimensions":{"confidence":0.85,"dimensions":[{"index":1,"label":"运行、就绪和休眠期间发生了什么","required":true,"role":"observed_value","source_quote":"分开说明运行、就绪和休眠期间发生了什么"},{"index":2,"label":"时序图","required":true,"role":"diagram","source_quote":"画一张时序图"},{"index":3,"label":"启动工作和唤醒的先后关系","required":true,"role":"relation_path","source_quote":"把这段启动工作和唤醒的先后关系讲清楚"},{"index":4,"label":"停顿原因定位","required":true,"role":"causal_attribution","source_quote":"能定位停顿原因吗"}],"is_dimensioned_answer":true,"rationale":"用户显式要求状态分区说明、时序图、唤醒先后关系与原因定位四个可见面"},"runtime_artifact_scope_profile":{"confidence":0.9,"rationale":"用户给出精确起止秒区间，属于显式观测时间窗","requested_scope":"explicit_time_window","source_quote":"11343.248 到 11343.310 秒","time_end":11343.31,"time_start":11343.248},"runtime_question_profile":{"confidence":0.8,"frame_causality_requested":false,"rationale":"请求要求定位该次停顿的成因，属于独立原因发现；同时需要状态分区与唤醒时序作为支撑观测","runtime_work_relation_requested":false,"scope":"causal_diagnosis","source_quote":"能定位停顿原因吗"},"runtime_selection_profile":{"confidence":0.9,"is_selection_question":false,"source_quote":""},"runtime_target_profile":{"confidence":0.9,"declaration":"named_target","rationale":"用户显式指定主线程 tid 64156 为分析对象","source_quote":"主线程（tid 64156）"},"runtime_targets":[{"confidence":0.9,"description":"蜂鸟众包应用主线程，用户指定为本次停顿的分析目标","kind":"thread","pid":64156,"source":"user_explicit"}],"scenario":"root_cause","sub_topics":[{"entities":["64156"],"summary":"梳理主线程 64156 在 11343.248~11343.310 秒内运行、就绪、休眠三态的调度事件与各段耗时"},{"entities":["64156"],"summary":"梳理该窗口内主线程的唤醒/唤醒者与切换顺序，形成时序图"},{"entities":["64156"],"summary":"基于窗口内调度与阻塞证据判定本次停顿的可归因原因及证据强度"}]}`

// wide repeats the lookup mix of the two runs that searched wake rows by the
// requested thread number; otherwise the peer thread's events were searched.
func lifelineHoneybirdContextHMC376(t *testing.T, path string, wide bool) (*types.BusContext, []RuntimeDiagramRelation) {
	t.Helper()
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	ctx.Language = "zh"
	ctx.Mutable = types.NewMutableState(lifelineHoneybirdQuestionHMC376)
	ctx.AttachedHitraceSource = path
	ctx.AttachedHitrace = "# tracer: nop\n"
	if out, err := (&EmitAnalysis{}).Execute(ctx, json.RawMessage(lifelineHoneybirdAnalysisHMC376)); err != nil || !out.Success {
		t.Fatalf("public analysis: %v %s", err, out.Summary)
	}
	rm := ctx.Mutable.RequestModel()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramSequence, Participants: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)}}}
	queries := []map[string]any{
		{"source": "path", "path": path, "view": "wakeup_chain", "pid": 64156, "time_start": 11343.248, "time_end": 11343.31},
		{"source": "path", "path": path, "view": "root_cause_rank", "pid": 64156, "time_start": 11343.248, "time_end": 11343.31},
	}
	if wide {
		queries = append(queries, map[string]any{"source": "path", "path": path, "view": "event_search", "event_types": []string{"sched_wakeup"}, "pattern": "64156", "limit": 40, "time_start": 11343.248, "time_end": 11343.31})
	} else {
		queries = append(queries, map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 64176, "time_start": 11343.24, "time_end": 11343.302, "limit": 40})
	}
	var results []types.ToolResult
	for _, q := range queries {
		r := hmc17NamedQuery(t, ctx, q)
		if !r.Success {
			t.Fatalf("query %v: %s", q["view"], r.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(r)
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx, runtimeDiagramRelationsForContext(ctx)
}

type lifelineHoneybirdEdgeHMC376 struct {
	from, to     string // the draft's own lifeline ids; empty = a published lane id is used
	ts           float64
	waker, wakee int
	// "<ts>:from|to" = the published lane of that presented event side is
	// used as the lifeline id, as the draft did.
	fromLane, toLane string
}

type lifelineHoneybirdShapeHMC376 struct {
	name       string
	wide       bool
	labels     map[string]string
	edges      []lifelineHoneybirdEdgeHMC376
	selections map[string]string // lifeline id -> "<ts>:from|to"
	accepted   bool
	issue      string   // the one genuine issue of a rejected shape
	names      []string // what that one rejection must name
}

var lifelineIssueTokenHMC376 = regexp.MustCompile(`issue=([a-z_]+)`)

func lifelineIssuesHMC376(summary string) []string {
	seen := map[string]bool{}
	var out []string
	for _, match := range lifelineIssueTokenHMC376.FindAllStringSubmatch(summary, -1) {
		if !seen[match[1]] {
			seen[match[1]] = true
			out = append(out, match[1])
		}
	}
	if strings.Contains(summary, "native event") && strings.Contains(summary, "before body line") {
		out = append(out, types.DiagramRelationFailureNativeSequenceEventOrder)
	}
	sort.Strings(out)
	return out
}

func lifelineHoneybirdDocHMC376(t *testing.T, rows []RuntimeDiagramRelation, shape lifelineHoneybirdShapeHMC376) *types.AnswerDocumentV2 {
	t.Helper()
	presented := lifelinePresentedWakesHMC376(rows)
	find := func(ts float64, waker, wakee int) *RuntimeDiagramRelation {
		for i := range presented {
			r := &presented[i]
			if math.Abs(r.eventOrder.timestamp-ts) < 5e-7 && r.threadEndpoints.from.PID == waker && r.threadEndpoints.to.PID == wakee {
				return r
			}
		}
		t.Fatalf("%s: the presented rows lack the recorded wake %.6f %d->%d", shape.name, ts, waker, wakee)
		return nil
	}
	resolve := func(ref string) (string, string) {
		var ts float64
		var side string
		if _, err := fmt.Sscanf(ref, "%f:%s", &ts, &side); err != nil {
			t.Fatal(err)
		}
		for i := range presented {
			if r := &presented[i]; math.Abs(r.eventOrder.timestamp-ts) < 5e-7 {
				return lifelinePublishedNodeHMC376(*r, side == "from")
			}
		}
		t.Fatalf("%s: no presented lane for %s", shape.name, ref)
		return "", ""
	}
	doc := requestedThreadDocHMC242(rows, false)
	doc.Blocks[0].Text = "该窗口记录数次唤醒；线程身份来自同一份原生事件，不表示唤醒者造成全部等待。"
	block := &doc.Blocks[1]
	block.EdgeAnchors = nil
	var declarations, body strings.Builder
	seen := map[string]bool{}
	declare := func(id, label string) {
		if !seen[id] {
			seen[id] = true
			fmt.Fprintf(&declarations, "    participant %s as %s\n", id, label)
		}
	}
	for _, edge := range shape.edges {
		row := find(edge.ts, edge.waker, edge.wakee)
		from, to := edge.from, edge.to
		if edge.fromLane != "" {
			id, label := resolve(edge.fromLane)
			from = id
			declare(id, label)
		} else {
			declare(from, shape.labels[from])
		}
		if edge.toLane != "" {
			id, label := resolve(edge.toLane)
			to = id
			declare(id, label)
		} else {
			declare(to, shape.labels[to])
		}
		label := fmt.Sprintf("唤醒 %.6f", edge.ts)
		fmt.Fprintf(&body, "    %s->>%s: %s\n", from, to, label)
		block.EdgeAnchors = append(block.EdgeAnchors, types.DiagramEdgeAnchor{FromNode: from, ToNode: to, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: label})
	}
	block.Diagram.Body = strings.TrimRight("sequenceDiagram\n"+declarations.String()+body.String(), "\n")
	aliases := make([]string, 0, len(shape.selections))
	for alias := range shape.selections {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		id, _ := resolve(shape.selections[alias])
		block.Diagram.RuntimeParticipantsV1 = append(block.Diagram.RuntimeParticipantsV1, types.RuntimeDiagramParticipantSelection{NodeID: alias, NativeParticipant: id})
	}
	return doc
}

// Whole original, not an excerpt. Six first-draft shapes that earlier runs
// submitted on this capture are rebuilt against fresh public lookups (stored
// opaque ids cannot be reused; each arrow is re-selected by its recorded
// timestamp and thread numbers). A lifeline that hosts every published
// observation of one recorded thread is accepted; the two shapes that carry
// a genuine slip are rejected for exactly that slip.
func TestHMC376HoneybirdSealedShapes(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_HONEYBIRD")
	if path == "" {
		t.Skip("requires the pinned complete Honeybird ftrace")
	}
	const digest = "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e"
	if h, n := hmc176bRepresentativeDigest(t, path); h != digest || n != 106896120 {
		t.Fatal("not the pinned whole Honeybird capture")
	}
	const (
		tIrq1, tPeer, tPool, tMain, t98, tIrq2, t201, t202, t203 = 11343.248371, 11343.250632, 11343.296385, 11343.297396, 11343.297964, 11343.298341, 11343.301985, 11343.302175, 11343.302243
	)
	lane := func(ts float64, side string) string { return fmt.Sprintf("%.6f:%s", ts, side) }
	eight := func(ordered bool) []lifelineHoneybirdEdgeHMC376 {
		edges := []lifelineHoneybirdEdgeHMC376{
			{from: "Main", to: "Irq", ts: tIrq1, waker: 64156, wakee: 86}, {from: "Main", to: "Sig", ts: tPeer, waker: 64156, wakee: 64176}, {from: "Sig", to: "Main", ts: tMain, waker: 64176, wakee: 64156},
			{from: "Main", to: "Irq", ts: tIrq2, waker: 64156, wakee: 86}, {from: "Main", to: "W98", ts: t98, waker: 64156, wakee: 64198}, {from: "Main", to: "W201", ts: t201, waker: 64156, wakee: 64201},
			{from: "Main", to: "W202", ts: t202, waker: 64156, wakee: 64202}, {from: "Main", to: "W203", ts: t203, waker: 64156, wakee: 64203}}
		if ordered {
			edges[3], edges[4] = edges[4], edges[3]
		}
		return edges
	}
	eightLabels := map[string]string{"Main": "主线程 ele.crowdsource-64156", "Sig": "Signal Catcher-64176", "Irq": "dh-irq-bind-4-86", "W98": "ele.crowdsource-64198", "W201": "ele.crowdsource-64201", "W202": "ele.crowdsource-64202", "W203": "ele.crowdsource-64203"}
	shapes := []lifelineHoneybirdShapeHMC376{
		{name: "split aliases for the waking and the woken side of the main thread", wide: true, accepted: true,
			labels: map[string]string{"MAIN_OUT": "主线程 ele.crowdsource-64156（唤醒发起侧）", "MAIN_IN": "主线程 ele.crowdsource-64156（被唤醒侧）", "SC": "Signal Catcher-64176", "P98": "ele.crowdsource-64198"},
			edges:  []lifelineHoneybirdEdgeHMC376{{from: "SC", to: "MAIN_IN", ts: tMain, waker: 64176, wakee: 64156}, {from: "MAIN_OUT", to: "P98", ts: t98, waker: 64156, wakee: 64198}}},
		{name: "published lane ids, the wake of the main thread not drawn as an arrow", issue: "required_participant_identity_not_visible",
			// The one recorded wake of the requested thread, by its row.
			names: []string{`Published row to draw: "Signal Catcher-64176" → "ele.crowdsource-64156"；唤醒时刻=11343.297396秒`, "；发起线程号=64176；被唤醒线程号=64156", `diagram_edge_edits action "add"`},
			edges: []lifelineHoneybirdEdgeHMC376{{ts: tPeer, waker: 64156, wakee: 64176, fromLane: lane(tPeer, "from"), toLane: lane(tPeer, "to")},
				{ts: tPool, waker: 64176, wakee: 67, fromLane: lane(tPool, "from"), toLane: lane(tPool, "to")}}},
		{name: "own aliases with explicit selections across lookup copies", accepted: true,
			labels:     map[string]string{"Target": "主线程 ele.crowdsource-64156", "Peer": "线程 ele.crowdsource-64176", "SignalCatcher": "Signal Catcher-64176"},
			edges:      []lifelineHoneybirdEdgeHMC376{{from: "Target", to: "Peer", ts: tPeer, waker: 64156, wakee: 64176}, {from: "SignalCatcher", to: "Target", ts: tMain, waker: 64176, wakee: 64156}},
			selections: map[string]string{"Target": lane(tMain, "to"), "Peer": lane(tPeer, "to"), "SignalCatcher": lane(tMain, "from")}},
		{name: "published lane ids, main thread on its waking lane", accepted: true,
			edges: []lifelineHoneybirdEdgeHMC376{{ts: tPeer, waker: 64156, wakee: 64176, fromLane: lane(tPeer, "from"), toLane: lane(tPeer, "to")},
				{ts: tPool, waker: 64176, wakee: 67, fromLane: lane(tPool, "from"), toLane: lane(tPool, "to")},
				{ts: tMain, waker: 64176, wakee: 64156, fromLane: lane(tPool, "from"), toLane: lane(tPeer, "from")}}},
		{name: "own aliases, eight recorded wake arrows, two adjacent arrows out of time order", wide: true, issue: types.DiagramRelationFailureNativeSequenceEventOrder,
			// The two adjacent arrows that really are out of recorded order.
			names:  []string{"11343.297964", "11343.298341"},
			labels: eightLabels, edges: eight(false)},
		{name: "published lane ids, main thread on its woken lane", accepted: true,
			edges: []lifelineHoneybirdEdgeHMC376{{ts: tPeer, waker: 64156, wakee: 64176, fromLane: lane(tMain, "to"), toLane: lane(tPeer, "to")},
				{ts: tPool, waker: 64176, wakee: 67, fromLane: lane(tPool, "from"), toLane: lane(tPool, "to")},
				{ts: tMain, waker: 64176, wakee: 64156, fromLane: lane(tPool, "from"), toLane: lane(tMain, "to")}}},
	}
	accepted := 0
	for _, shape := range shapes {
		ctx, rows := lifelineHoneybirdContextHMC376(t, path, shape.wide)
		out := lifelineEmitHMC376(t, ctx, lifelineHoneybirdDocHMC376(t, rows, shape))
		issues := lifelineIssuesHMC376(out.Summary)
		t.Logf("%-78s accepted=%t result_bytes=%d issues=%v", shape.name, out.Success, len(out.Summary), issues)
		if out.Success {
			accepted++
		}
		if shape.accepted {
			if !out.Success {
				t.Errorf("%s: rejected: %s", shape.name, out.Summary)
			}
			continue
		}
		if out.Success || len(issues) != 1 || issues[0] != shape.issue {
			t.Errorf("%s: want exactly the one genuine issue %q, got accepted=%t issues=%v: %s", shape.name, shape.issue, out.Success, issues, out.Summary)
		}
		for _, want := range shape.names {
			if !strings.Contains(out.Summary, want) {
				t.Errorf("%s: the rejection does not name %q: %s", shape.name, want, out.Summary)
			}
		}
		if strings.Contains(out.Summary, types.DiagramRelationFailureRuntimeLifelineTwoThreads) || strings.Contains(out.Summary, "edge_anchor_node_identity_conflict") {
			t.Errorf("%s: a lifeline of one recorded thread was reported as a conflict: %s", shape.name, out.Summary)
		}
	}
	if accepted != 4 {
		t.Errorf("accepted %d of %d shapes, want 4", accepted, len(shapes))
	}

	// No recorded wake arrow of the eight-arrow draft is pushed out: with its
	// two adjacent arrows in recorded time order the same draft is accepted
	// whole.
	ctx, rows := lifelineHoneybirdContextHMC376(t, path, true)
	ordered := lifelineHoneybirdShapeHMC376{name: "eight recorded wake arrows in time order", wide: true, labels: eightLabels, edges: eight(true)}
	out := lifelineEmitHMC376(t, ctx, lifelineHoneybirdDocHMC376(t, rows, ordered))
	if !out.Success {
		t.Fatalf("eight recorded wake arrows on one lifeline per thread were rejected: %s", out.Summary)
	}
	if got := ctx.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks[1].EdgeAnchors) != 8 {
		t.Fatalf("accepted document lost recorded wake arrows: %+v", got)
	}
	if h, _ := hmc176bRepresentativeDigest(t, path); h != digest {
		t.Fatal("whole capture changed")
	}
}
