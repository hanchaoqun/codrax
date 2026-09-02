package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestOccupancyStatisticsNeverSubstituteCumulativeForSingle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		node  types.TraceCausalProjectionNode
		count int
		max   float64
	}{
		{"one aggregate row", types.TraceCausalProjectionNode{ImpactMS: 21}, 0, 0},
		{"family missing maximum", types.TraceCausalProjectionNode{FamilyMemberCount: 4, ImpactMS: 21}, 4, 0},
		{"merge missing maximum", types.TraceCausalProjectionNode{MergedCount: 4, ImpactMS: 21}, 4, 0},
		{"family published maximum", types.TraceCausalProjectionNode{FamilyMemberCount: 4, FamilyMemberMaxMS: 9, ImpactMS: 21}, 4, 9},
		{"union record maximum", types.TraceCausalProjectionNode{MergedCount: 4, MergedMaxMS: 9, MergedIntervalUnion: true, ImpactMS: 21}, 4, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, maximum := runtimeTraceOccupancyNodeCountAndMax(tc.node)
			if count != tc.count || maximum != tc.max {
				t.Fatalf("record statistics were invented from cumulative occupancy: count=%d max=%v; want %d/%v", count, maximum, tc.count, tc.max)
			}
		})
	}
}

func TestOccupancyTableSeparatesRecordsFromPhysicalSpanOccurrences(t *testing.T) {
	for _, zh := range []bool{true, false} {
		unknown := types.TraceCausalProjectionNode{Subject: "worker-9", StateKind: "running", ImpactMS: 21, StartTs: 1, EndTs: 1.1}
		merged := types.TraceCausalProjectionNode{Subject: "worker-10", StateKind: "d_state", ImpactMS: 12, MergedCount: 4, MergedMaxMS: 8}
		model := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true, Node: unknown},
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true, Node: merged},
		}}
		projection := types.TraceCausalProjection{BusinessSpanMentions: []types.TraceCausalProjectionBusinessSpanMention{
			{Subject: "worker-11", Name: "business work", Count: 6, TotalMS: 18, MaxMS: 5, Basis: "self"},
		}}
		before, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		block := runtimeTraceCausalProjectionOccupancyBlock(projection, model, zh, "test", "", nil, nil)
		if block == nil || len(block.Items) != 3 {
			t.Fatalf("all occupancy values must remain visible: %+v", block)
		}
		after, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("occupancy statistics must not mutate the causal model")
		}
		for _, item := range block.Items {
			cells := item.Cells
			switch {
			case strings.Contains(cells[1], "worker-9"):
				if cells[2] != "21.000ms" || cells[3] != "—" || cells[4] != "—" {
					t.Fatalf("single aggregate row cannot mint one physical occurrence: %v", cells)
				}
			case strings.Contains(cells[1], "worker-10"):
				label := "统计记录"
				if !zh {
					label = "records"
				}
				if !strings.Contains(cells[4], label) || !strings.Contains(cells[3], "8.000ms") || cells[3] == "8.000ms" {
					t.Fatalf("grouped rows need record-count/record-maximum labels: %v", cells)
				}
			case strings.Contains(cells[1], "worker-11"):
				if !strings.Contains(cells[3], "5.000ms") || cells[4] != "6" {
					t.Fatalf("producer-proven physical business span stats changed: %v", cells)
				}
			}
		}
	}
}

// The committed real trace reproduces a one-row running account and four
// per-CPU D-state groups. Neither display grouping is an occurrence census.
func TestOccupancyStatisticsDonghuRealWitness(t *testing.T) {
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	var records []types.ObservationRecord
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank"} {
		q := tracequery.Query{View: view, Thread: "CompThread_0-2955", TimeStart: 13762.791708, TimeEnd: 13763.024898,
			TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
		if view == "wakeup_chain" {
			q.MaxBranches, q.MaxChainNodes, q.IncludeWindowStats = 8, 32, true
		}
		result := tracequery.Run(idx, q)
		records = append(records, traceQueryTypedObservations(result, "donghu.ftrace", "p-"+view, "r", "", time.Unix(1751600000, 0).UTC())...)
	}
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		block := runtimeTraceCausalProjectionOccupancyBlock(projection, model, zh, "witness", "", nil, nil)
		if block == nil {
			t.Fatal("real trace lost the occupancy table")
		}
		running, groupedWait := false, false
		for _, item := range block.Items {
			cells := item.Cells
			if !strings.Contains(cells[1], "CompThread_0-2955") {
				continue
			}
			if cells[2] == "74.915ms" {
				running = true
				if cells[3] != "—" || cells[4] != "—" {
					t.Fatalf("cumulative running still looks like one long occurrence: %v", cells)
				}
			}
			if cells[2] == "36.757ms" {
				groupedWait = true
				if cells[4] == "4" || cells[3] == "16.064ms" || cells[3] == "36.757ms" {
					t.Fatalf("per-CPU/merged wait account still claims physical single-event stats: %v", cells)
				}
			}
			t.Logf("zh=%v: %s", zh, strings.Join(cells, " | "))
		}
		if !running || !groupedWait {
			t.Fatalf("real typed witness lost cumulative running/wait values: running=%v wait=%v", running, groupedWait)
		}
	}
}
