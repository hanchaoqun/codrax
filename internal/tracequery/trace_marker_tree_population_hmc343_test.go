package tracequery

import (
	"encoding/json"
	"fmt"
	"testing"
)

// The fixture's independent census is 40 background and 45 selected-thread
// instances. Selection orders the existing 32-row display, not its population.
// Read the public JSON so the original producer can fail behaviorally before
// any new Go metadata fields exist.
func TestHMC343BusinessTreePopulationPublicCensus(t *testing.T) {
	idx := buildTraceIndex(t, "census.ftrace", hmc328MarkerSelectionTrace(false))
	for _, tc := range []struct {
		name              string
		q                 Query
		tid, total, shown int
	}{
		{"selected", Query{PID: 7}, 7, 45, 32},
		{"background_selected", Query{PID: 9}, 9, 40, 32},
		{"missing_thread_is_known_zero", Query{PID: 99}, 99, 0, 0},
		{"all_emitters", Query{}, 0, 0, 0},
		{"process_is_not_emitter_tid", Query{PID: 77, TargetScope: TargetScopeProcess}, 0, 0, 0},
		{"name_is_not_unique_thread", Query{Thread: "worker"}, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q
			q.TimeStart, q.TimeEnd = 1, 4
			tree := markerTreePublic(t, idx, q)
			if tree.NodeCount != 85 || tree.OmittedNodes != 53 || len(tree.Nodes) != 32 {
				t.Fatal("original all-emitter population or display budget changed")
			}
			data, err := json.Marshal(tree)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Population *struct {
					Scope          string `json:"scope"`
					DisplayedNodes int    `json:"displayed_nodes"`
					Target         *struct {
						TID            int `json:"tid"`
						NodeCount      int `json:"node_count"`
						DisplayedNodes int `json:"displayed_nodes"`
						OmittedNodes   int `json:"omitted_nodes"`
					} `json:"target"`
				} `json:"population"`
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Population == nil || got.Population.Scope != "all_observed_emitters" || got.Population.DisplayedNodes != 32 {
				t.Fatalf("public population omits the all-emitter scope/display count: %s", data)
			}
			p := got.Population.Target
			if tc.tid == 0 {
				if p != nil {
					t.Fatal("unresolved/process/name selector invented a physical-thread census")
				}
			} else if p == nil || p.TID != tc.tid || p.NodeCount != tc.total || p.DisplayedNodes != tc.shown || p.OmittedNodes != tc.total-tc.shown {
				t.Fatalf("selected-thread census became the display or global population: got=%+v want=%s", p, fmt.Sprint(tc.tid, tc.total, tc.shown))
			}
		})
	}
}

func TestHMC343BusinessPopulationExactArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name           string
		p              *TraceMarkerTreePopulation
		total, omitted int
		valid          bool
	}{
		{"legacy", nil, 85, 53, true},
		{"all", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters, DisplayedNodes: 32}, 85, 53, true},
		{"target", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters, DisplayedNodes: 32, Target: &TraceMarkerTreeTargetPopulation{TID: 7, NodeCount: 45, DisplayedNodes: 32, OmittedNodes: 13}}, 85, 53, true},
		{"zero", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters, DisplayedNodes: 32, Target: &TraceMarkerTreeTargetPopulation{TID: 99}}, 85, 53, true},
		{"unknown_scope", &TraceMarkerTreePopulation{Scope: "all", DisplayedNodes: 32}, 85, 53, false},
		{"omitted_over_total", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters}, 1, 2, false},
		{"negative_total", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters}, -1, 0, false},
		{"target_display_over_total", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters, DisplayedNodes: 32, Target: &TraceMarkerTreeTargetPopulation{TID: 7, NodeCount: 1, DisplayedNodes: 2, OmittedNodes: 0}}, 85, 53, false},
		{"negative_target", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters, DisplayedNodes: 32, Target: &TraceMarkerTreeTargetPopulation{TID: 7, NodeCount: -1}}, 85, 53, false},
		{"foreign_rows_impossible", &TraceMarkerTreePopulation{Scope: TraceMarkerTreePopulationAllEmitters, DisplayedNodes: 32, Target: &TraceMarkerTreeTargetPopulation{TID: 7, NodeCount: 84, DisplayedNodes: 0, OmittedNodes: 84}}, 85, 53, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.p.ValidCounts(tc.total, tc.omitted) != tc.valid {
				t.Fatal("census arithmetic admitted inconsistent typed counts")
			}
		})
	}
}
