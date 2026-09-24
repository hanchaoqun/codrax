package tool

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The fixture has four accepted read pairs but six in-window issue events:
// three accepted arrivals, two ambiguous arrivals and one unpaired arrival.
// Its fourth accepted pair starts before the window. Exercise the real query
// and bound publication, not a hand-authored table or a second pairing model.
func TestIOInFlightPublicReceiptMetricPopulations(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_io_inflight/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "population.systrace")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("Describe IO counts, members and timeline")}
	query, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1, "time_end": 1.01})
	result, err := (&TraceQuery{}).Execute(ctx, query)
	if err != nil || !result.Success {
		t.Fatalf("real query failed: %v / %s", err, result.Summary)
	}
	var observation types.ObservationRecord
	for _, row := range result.Observations {
		if row.Predicate == "io_inflight" && strings.Contains(strings.Join(row.RichNotes, "\n"), "endpoint_family=block_rq dev=8,0 op=R") {
			if observation.ID != "" {
				t.Fatal("ambiguous fixture group")
			}
			observation = row
		}
	}
	if observation.ID == "" || observation.Role != types.AnswerAggregateRoleSupportingCoverage {
		t.Fatal("query did not publish the noncausal read population")
	}
	data, err := os.ReadFile(observation.SourceRef.PayloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(data, &native); err != nil || native.WindowStats == nil || native.WindowStats.IOInFlight == nil {
		t.Fatalf("native payload missing: %v", err)
	}
	stats := native.WindowStats.IOInFlight
	if stats.GroupCount != 4 || stats.Window == nil || stats.Window.StartTs != 1 || stats.Window.EndTs != 1.01 {
		t.Fatalf("query population/window changed: %+v", stats)
	}
	var group *tracequery.IOInFlightGroup
	for i := range stats.Groups {
		g := &stats.Groups[i]
		if g.EndpointFamily == "block_rq" && g.Dev == "8,0" && g.Operation == "R" {
			group = g
		}
	}
	if group == nil || group.AcceptedPairCount != 4 || group.IssueCount != 6 || len(group.Members) != 4 || group.Values == nil || group.Values.PeakRequests != 2 ||
		math.Abs(group.Values.MeanRequests-1.4) > 1e-9 || math.Abs(group.Values.BusyMs-10) > 1e-9 || math.Abs(group.Values.RequestMs-14) > 1e-9 {
		t.Fatalf("original native counts/measurements changed: %+v", group)
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	tables := map[types.RuntimeMeasurementView]types.RuntimeMeasurementTable{}
	for _, view := range []types.RuntimeMeasurementView{types.RuntimeMeasurementSummary, types.RuntimeMeasurementMembers, types.RuntimeMeasurementTimeline} {
		receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: observation.ID, View: view}
		if !types.BindRuntimeMeasurementReceipt(receipt, contract) {
			t.Fatalf("real publication cannot bind %s", view)
		}
		tables[view] = receipt.BoundTable.Clone()
	}
	for view, fragments := range map[types.RuntimeMeasurementView][]string{
		types.RuntimeMeasurementSummary:  {"peak and mean", "busy time", "request-time area", "complete-pair count", "successfully paired requests", "separate count", "retained by source and identity checks", "unpaired or ambiguous", "excludes start endpoints outside the selected range"},
		types.RuntimeMeasurementMembers:  {"Accepted complete pairs", "all accepted pairs", "do not enumerate", "issue-event count"},
		types.RuntimeMeasurementTimeline: {"series uses successfully paired requests", "full series", "do not infer", "issue-event counts"},
	} {
		t.Run(string(view), func(t *testing.T) {
			notes := strings.Join(tables[view].Notes, "\n")
			for _, want := range append(fragments, "All issuing threads", "not thread waiting", "causal attribution") {
				if !strings.Contains(notes, want) {
					t.Errorf("%s omitted population/boundary concept %q: %s", view, want, notes)
				}
			}
			// This old shared sentence incorrectly qualified the Starts column.
			for _, note := range tables[view].Notes {
				if strings.HasPrefix(note, "All issuing threads") && strings.Contains(note, "paired requests") {
					t.Error("shared issuer scope still falsely restricts all metrics to complete pairs")
				}
			}
		})
	}
	if got := tables[types.RuntimeMeasurementSummary].Rows; !reflect.DeepEqual(got, [][]string{{"2", "1.4", "10", "14", "4", "6"}}) {
		t.Fatalf("summary changed accepted-pair values or observed arrival count: %v", got)
	}
	members := tables[types.RuntimeMeasurementMembers].Rows
	if len(members) != 4 {
		t.Fatalf("member witnesses incorrectly enumerate six starts: %v", members)
	}
	for i, member := range group.Members {
		row := members[i]
		start, startErr := strconv.ParseFloat(row[4], 64)
		end, endErr := strconv.ParseFloat(row[5], 64)
		contribution, contributionErr := strconv.ParseFloat(row[7], 64)
		if startErr != nil || endErr != nil || contributionErr != nil || start != member.ActualStartTs || end != member.ActualEndTs ||
			member.WindowContributionMs == nil || math.Abs(contribution-*member.WindowContributionMs) > 1e-9 {
			t.Fatalf("member endpoints/contribution changed: %v / %+v", row, member)
		}
	}
	if members[0][4] != "0.998000" || members[0][5] != "1.004000" || members[0][6] != "[1.000000, 1.004000)" || members[0][7] != "4" ||
		members[3][4] != "1.008000" || members[3][5] != "1.012000" || members[3][6] != "[1.008000, 1.010000)" || members[3][7] != "2" {
		t.Fatalf("carry-in/out endpoints were clipped or their window contributions changed: %v", members)
	}
	timeline := tables[types.RuntimeMeasurementTimeline].Rows
	if len(timeline) != len(group.Segments) {
		t.Fatal("timeline presentation changed its segment population")
	}
	for i, segment := range group.Segments {
		start, _ := strconv.ParseFloat(timeline[i][0], 64)
		end, _ := strconv.ParseFloat(timeline[i][1], 64)
		depth, _ := strconv.Atoi(timeline[i][2])
		if start != segment.StartTs || end != segment.EndTs || depth != segment.Requests {
			t.Fatalf("timeline no longer matches native accepted-pair series: %v / %+v", timeline[i], segment)
		}
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, body) {
		t.Fatal("read query mutated its source trace")
	}
}

func TestIOInFlightPublicReceiptLineSelectionPopulation(t *testing.T) {
	const body = "reader-40 (40) [001] .... 1.001000: block_rq_issue: 8,0 R 4096 () 8 + 8 [reader]\n" +
		"irq-2 (2) [001] .... 1.003000: block_rq_complete: 8,0 R () 8 + 8 [0]\n" +
		"reader-41 (41) [001] .... 1.006000: block_rq_issue: 8,0 R 4096 () 16 + 8 [reader]\n" +
		"irq-2 (2) [001] .... 1.008000: block_rq_complete: 8,0 R () 16 + 8 [0]\n"
	// The explicit time range covers the first pair, but the selected source
	// lines cover only the second. Line precedence must not invent a time window.
	result, stats, row, _ := ioInFlightPrecisionPublicQuery(t, body, 1, 1.004, 3, 4)
	g := stats.Groups[0]
	if stats.Window != nil || stats.LineStart != 3 || stats.LineEnd != 4 || g.Values != nil || g.AcceptedPairCount != 1 || g.IssueCount != 1 ||
		len(g.Members) != 1 || g.Members[0].ActualStartTs != 1.006 || g.Members[0].WindowContributionMs != nil {
		t.Fatalf("line precedence or unknown time measures changed: %+v / %+v", stats, g)
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: row.ID, View: types.RuntimeMeasurementSummary}
	if !types.BindRuntimeMeasurementReceipt(receipt, contract) {
		t.Fatal("line-selected native summary cannot bind")
	}
	if !reflect.DeepEqual(receipt.BoundTable.Rows, [][]string{{"unavailable", "unavailable", "unavailable", "unavailable", "1", "1"}}) {
		t.Fatalf("line selection became a measured time window: %v", receipt.BoundTable.Rows)
	}
	notes := strings.Join(receipt.BoundTable.Notes, "\n")
	for _, want := range []string{"retained by source and identity checks", "outside the selected range", "line selection takes precedence", "No continuous time denominator"} {
		if !strings.Contains(notes, want) {
			t.Errorf("line-selected population omitted %q: %s", want, notes)
		}
	}
	if strings.Contains(notes, "before the query window") || strings.Contains(notes, "before query window") {
		t.Fatal("arrival explanation invents a continuous window for a line-selected query")
	}
}
