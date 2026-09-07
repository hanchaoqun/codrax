package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// B1596: the rank producer intentionally prices this seat as runnable plus
// discounted running. That value remains valid on the eliminable board, but
// cannot be republished as a second raw scheduler-state measurement.
func TestB1596OccupancyRealTraceKeepsRawAndPricedAxesSeparate(t *testing.T) {
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	var records []types.ObservationRecord
	for _, view := range []string{"root_cause_rank", "wakeup_chain"} {
		result := tracequery.Run(idx, tracequery.Query{View: view, PID: 59566,
			TimeStart: 34579.490, TimeEnd: 34579.500, Limit: 40,
			MaxBranches: 8, MaxChainNodes: 32, IncludeWindowStats: true,
			TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace})
		records = append(records, traceQueryTypedObservations(result, "donghu_tieba_frame.systrace", "b1596-"+view, "raw", "", time.Unix(1751600000, 0).UTC())...)
	}
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	before, _ := json.Marshal(projection)
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		block := runtimeTraceCausalProjectionOccupancyBlock(projection, model, zh, "b1596", "", nil, nil)
		if block == nil {
			t.Fatal("real typed occupancy disappeared")
		}
		foundRaw := false
		for _, item := range block.Items {
			if !strings.Contains(item.Cells[1], "NetworkService-60595") {
				continue
			}
			if item.Cells[2] == "5.951ms" {
				t.Fatalf("priced runnable + discounted running is not raw occupancy: %v", item.Cells)
			}
			if item.Cells[2] == "5.930ms" && strings.Contains(item.Cells[1], "runnable") {
				foundRaw = true
			}
		}
		if !foundRaw {
			t.Fatalf("original runnable account must remain visible: %+v", block.Items)
		}
		lang := "zh-CN"
		if !zh {
			lang = "en"
		}
		cluster := runtimeTraceCausalProjectionClusterFor(projection, lang, runtimeTraceProjUserFocus{}, "b1596", "")
		priced := false
		for _, b := range cluster {
			if b.ID == "b1596" && strings.Contains(b.Text, "5.951ms") {
				priced = true
			}
		}
		if !priced {
			t.Fatal("raw display correction must retain the original priced causal board")
		}
	}
	after, _ := json.Marshal(projection)
	if !bytes.Equal(before, after) {
		t.Fatal("raw display must not mutate projection prices, ranks, or credentials")
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: records}}
	owned := types.AnswerBlock{ID: "model_conclusion", Kind: types.BlockSection, Title: "模型结论", Text: "原始工作量和按规则估计的改进机会需要分别解释。"}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{owned}}
	if !materializeRuntimeTraceCausalProjectionBlock(doc, bus) {
		t.Fatal("real materialization entry must retain the typed trace projection")
	}
	ownedBefore, _ := json.Marshal(owned)
	ownedAfter, _ := json.Marshal(doc.Blocks[0])
	if !bytes.Equal(ownedBefore, ownedAfter) {
		t.Fatal("system occupancy publication must not rewrite the model-owned conclusion")
	}
}

func TestB1596OccupancyUsesOriginalMeasurementBeforeImpactAndCaptions(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		raw, priced float64
	}{
		{"runnable overlap", "runnable", 12, 3},
		{"running supply", "running", 9, 2},
		{"periodic sleep", "sleep", 8, .5},
		{"D wait", "d_state", 7, 4},
		{"IO wait", "io_wait", 6, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := types.TraceCausalProjectionNode{Subject: "worker-77", StateKind: tc.state,
				Predicate: "root_cause_primary", Object: "opaque_price_kind", Unit: "ms",
				ImpactMS: tc.priced, EffectiveImpactMS: tc.priced, EffectiveImpactPublished: true,
				CumulativeImpactMS: 31, ActualImpactMS: 45,
				StartTs: 1, EndTs: 1.100, EvidenceID: "original"}
			switch tc.state {
			case "runnable":
				node.RunnableMS = tc.raw
			case "running":
				node.RunningMS = tc.raw
			case "sleep":
				node.SleepMS, node.PeriodicSource = tc.raw, true
			case "d_state":
				node.DStateSplitMS = tc.raw
			case "io_wait":
				node.IOWaitSplitMS = tc.raw
			}
			for _, zh := range []bool{true, false} {
				rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{{Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true}}}, nil, zh)
				if len(rows) != 1 || rows[0].totalMS != tc.raw || strings.Contains(rows[0].subject, "opaque_price_kind") {
					t.Fatalf("original %s account must win without cause-type guessing; got %+v", tc.state, rows)
				}
				if strings.Contains(rows[0].caliber, fmt.Sprintf("%.3fms", node.ActualImpactMS)) {
					t.Fatalf("cross-window actual must not supply the raw value: %+v", rows)
				}
			}
		})
	}
}

func TestB1596OccupancyUnknownZeroAndFoldedMeasurementsRemainHonest(t *testing.T) {
	base := types.TraceCausalProjectionNode{Subject: "worker-77", StateKind: "running", Unit: "ms",
		Predicate: "root_cause_primary", Object: "future_price_kind", EvidenceID: "row",
		ImpactMS: 3, CumulativeImpactMS: 31, ActualImpactMS: 45,
		EffectiveImpactMS: 3, EffectiveImpactPublished: true, StartTs: 1, EndTs: 1.100}
	for _, tc := range []struct {
		name        string
		mutate      func(*types.TraceCausalProjectionNode)
		want        string
		unavailable bool
	}{
		{"unknown generic price", func(*types.TraceCausalProjectionNode) {}, "—", true},
		{"zero price original measured work", func(n *types.TraceCausalProjectionNode) { n.RunningMS, n.ImpactMS, n.EffectiveImpactMS = 9, 0, 0 }, "9.000ms", false},
		{"display folded seed is not population", func(n *types.TraceCausalProjectionNode) { n.RunningMS, n.MergedCount, n.MergedMaxMS = 9, 2, 3 }, "—", true},
		{"raw predicate cannot certify mixed display fold", func(n *types.TraceCausalProjectionNode) {
			n.Predicate, n.Object, n.MergedCount = "wakeup_causal_impact", "running", 2
		}, "—", true},
		{"engine family retains complete original partition", func(n *types.TraceCausalProjectionNode) {
			n.RunningMS, n.FamilyMemberCount, n.FamilyMemberMaxMS = 9, 2, 3
		}, "9.000ms", false},
		{"legacy raw state observation", func(n *types.TraceCausalProjectionNode) { n.Predicate, n.Object = "state_drilldown", "running" }, "3.000ms", false},
		{"legacy pure state rank contract", func(n *types.TraceCausalProjectionNode) { n.TypeToken, n.Object = "running", "running" }, "3.000ms", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := base
			tc.mutate(&node)
			before, _ := json.Marshal(node)
			for _, zh := range []bool{true, false} {
				block := runtimeTraceCausalProjectionOccupancyBlock(types.TraceCausalProjection{}, runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{{Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true}}}, zh, "b1596", "", nil, nil)
				if block == nil || len(block.Items) != 1 || block.Items[0].Cells[2] != tc.want {
					t.Fatalf("must retain the measurement or explicit unavailability, not borrow a price/seed: %+v", block)
				}
				if tc.unavailable && (block.Items[0].Cells[3] != "—" || block.Items[0].Cells[4] != "—") {
					t.Fatalf("unknown raw population cannot borrow priced record statistics: %v", block.Items[0].Cells)
				}
			}
			after, _ := json.Marshal(node)
			if !bytes.Equal(before, after) {
				t.Fatal("display changed the original record")
			}
		})
	}

	// The measured-zero bit comes from the real observation decoder, not from
	// a zero-valued float or a hand-authored private bit mask.
	record := traceProjectionObservation("measured-zero", "worker-77", "future_price_kind", "3", "31", 1)
	record.RichNotes = append(record.RichNotes, "dominant_state=running", "running=0", "effective_impact_ms=3")
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{record})
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	block := runtimeTraceCausalProjectionOccupancyBlock(projection, model, true, "b1596", "", nil, nil)
	found := false
	if block != nil {
		for _, item := range block.Items {
			if strings.Contains(item.Cells[1], "worker-77") && item.Cells[2] == "0.000ms" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("published measured zero must stay distinguishable from unavailable: %+v", block)
	}
}

func TestB1596OccupancyRawDedupAndRulerUseSameMeasurement(t *testing.T) {
	base := types.TraceCausalProjectionNode{Subject: "app-100", StateKind: "runnable",
		Object: "opaque_price_kind", ImpactMS: 3, RunnableMS: .8,
		StartTs: 5.005, EndTs: 5.0058, EvidenceID: "a"}
	other := base
	other.EvidenceID, other.ImpactMS = "b", 7
	account := &types.TraceCausalProjectionTargetStateAccount{Subject: "app-100", RunnableMS: .8, TotalMS: .8}
	model := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{
		{Node: base, Kind: runtimeTraceProjTreeRowSelf, HasData: true},
		{Node: other, Kind: runtimeTraceProjTreeRowSelf, HasData: true},
	}}
	rows := runtimeTraceOccupancyPathCandidates(model, account, true)
	if len(rows) != 1 || rows[0].totalMS != .8 || strings.Contains(rows[0].caliber, "非该状态全窗合计") {
		t.Fatalf("one exact raw interval must converge regardless of separate prices; raw whole-window equality is not a subset: %+v", rows)
	}
	model.SelfRows[0].Node.StateAccountKey = "state_account:v2:a"
	model.SelfRows[1].Node.StateAccountKey = "state_account:v2:b"
	rows = runtimeTraceOccupancyPathCandidates(model, account, true)
	if len(rows) != 2 {
		t.Fatalf("conflicting exact producer accounts must not be guessed equivalent: %+v", rows)
	}
}

func TestB1596EqualTotalsDoNotProveStateMaximumOwnership(t *testing.T) {
	node := types.TraceCausalProjectionNode{Subject: "worker-77", Predicate: "root_cause_primary", Object: "future_price_kind",
		StateKind: "runnable", RunnableMS: 10, ImpactMS: 10, EffectiveImpactMS: 10,
		FamilyMemberCount: 2, FamilyMemberMaxMS: 6}
	rows := runtimeTraceOccupancyPathCandidates(runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{{Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true}}}, nil, true)
	if len(rows) != 1 || rows[0].totalMS != 10 || rows[0].maxMS != 0 || rows[0].count != 2 {
		t.Fatalf("equal totals do not establish raw per-state maxima; source record count remains a record count: %+v", rows)
	}
}
