package context_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC314PublicRepresentativeExecutionHandoff(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	hmc285CatalogCall(t, bus, map[string]any{"action": "representatives", "catalog_id": c.ID(), "representatives": map[string]any{"per_cluster": 1}})
	selection := c.CurrentExecution().Selection
	if selection == nil || selection.Coverage.Selected != 2 {
		t.Fatalf("selection prerequisite: %+v", selection)
	}
	// Three genuine successful executions are not two failures followed by a
	// success. Also execute an unselected member without changing selection.
	for _, g := range selection.Clusters {
		for _, m := range g.Members {
			if g.Specification.Object.ID != "101" {
				continue
			}
			count := 1
			if m.Selected {
				count = 3
			}
			args, _ := json.Marshal(tracecatalog.QueryCall{Reference: tracecatalog.QueryReference{CatalogID: c.ID(), QueryID: m.QueryID}})
			for i := 0; i < count; i++ {
				out, err := (&tool.TraceQuery{}).Execute(bus, args)
				if err != nil || !out.Success {
					t.Fatalf("real native attempt: %v %s", err, out.Summary)
				}
			}
		}
	}
	want := c.CurrentExecution()
	before, _ := json.Marshal(want)
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		content := hmc259CatalogContext(t, bus, stage)
		if len(content) > 24<<10 {
			t.Fatalf("navigation exceeded original budget: %d", len(content))
		}
		rows := map[string]map[string]json.RawMessage{}
		for _, line := range strings.Split(content, "\n") {
			var row map[string]json.RawMessage
			if json.Unmarshal([]byte(line), &row) != nil {
				continue
			}
			for _, key := range []string{"selected_member", "unselected_member"} {
				var member tracecatalog.RepresentativeMember
				if json.Unmarshal(row[key], &member) == nil && member.QueryID != "" {
					rows[member.QueryID] = row
				}
			}
		}
		if len(rows) != 4 {
			t.Fatalf("%s: missing members: %d", stage, len(rows))
		}
		for _, g := range want.Selection.Clusters {
			for _, m := range g.Members {
				row := rows[m.QueryID]
				t.Run(string(stage)+"/"+m.QueryID+"/specification", func(t *testing.T) {
					var spec tracecatalog.QuerySpecification
					if json.Unmarshal(row["specification"], &spec) != nil || !reflect.DeepEqual(spec, g.Specification) {
						t.Fatalf("self-contained exact specification missing for selected=%t: %s", m.Selected, row["specification"])
					}
					var member tracecatalog.RepresentativeMember
					key := "unselected_member"
					if m.Selected {
						key = "selected_member"
					}
					if json.Unmarshal(row[key], &member) != nil || !reflect.DeepEqual(member, m) {
						t.Fatalf("member source/generation/selection/outcome lost: %s", row[key])
					}
				})
				t.Run(string(stage)+"/"+m.QueryID+"/attempts", func(t *testing.T) {
					var got map[tracecatalog.Outcome]int
					expected := map[tracecatalog.Outcome]int{}
					q, _ := c.Query(m.QueryID)
					for _, a := range q.Attempts {
						expected[a.Outcome]++
					}
					if json.Unmarshal(row["attempt_outcomes"], &got) != nil || !reflect.DeepEqual(got, expected) {
						t.Fatalf("per-query attempt history distribution missing: got=%s want=%v", row["attempt_outcomes"], expected)
					}
				})
			}
		}
	}
	after, _ := json.Marshal(c.CurrentExecution())
	if string(before) != string(after) {
		t.Fatal("display changed selection, permission or original execution history")
	}
}

func TestHMC314RepresentativeContextWholeUnitBudget(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	var queries []map[string]any
	for i := 0; i < 12; i++ {
		queries = append(queries, map[string]any{"view": "event_search", "pid": 101, "time_start": 10, "time_end": 10.05, "pattern": fmt.Sprintf("%d/%s", i, strings.Repeat("event", 240))})
	}
	hmc285CatalogCall(t, bus, map[string]any{"action": "discover", "queries": queries, "representatives": map[string]any{"per_cluster": 1, "max_clusters": 12}})
	before, _ := json.Marshal(c.CurrentExecution())
	want := c.CurrentExecution().RepresentativeNavigation()
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		content := hmc259CatalogContext(t, bus, stage)
		if len(content) > 24<<10 {
			t.Fatalf("original context budget exceeded: %d", len(content))
		}
		selected, unselected := 0, 0
		for _, line := range strings.Split(content, "\n") {
			if !strings.HasPrefix(line, "{") {
				continue
			}
			var row struct {
				Selected   *tracecatalog.RepresentativeMember `json:"selected_member"`
				Unselected *tracecatalog.RepresentativeMember `json:"unselected_member"`
				tracecatalog.RepresentativeQueryNavigation
			}
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatalf("partial JSON navigation unit: %v", err)
			}
			m := row.Selected
			if m != nil {
				selected++
			} else {
				m = row.Unselected
				if m != nil {
					unselected++
				}
			}
			if m != nil && !reflect.DeepEqual(row.RepresentativeQueryNavigation, want[m.QueryID]) {
				t.Fatal("budget cut a member's exact specification or outcome distribution")
			}
		}
		if selected+unselected >= len(want) || selected == 0 {
			t.Fatal("test did not exercise partial whole-member display")
		}
		for _, count := range []string{fmt.Sprintf("selected_member_records_displayed=%d omitted=%d", selected, 12-selected), fmt.Sprintf("unselected_member_records_displayed=%d omitted=%d", unselected, 12-unselected)} {
			if !strings.Contains(content, count) {
				t.Fatalf("wrong display vs selection census: %s", count)
			}
		}
	}
	after, _ := json.Marshal(c.CurrentExecution())
	if string(before) != string(after) {
		t.Fatal("budget display changed current plans or frozen selection")
	}
}
