package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A row's adjacent accounting role is not evidence that its thread has no
// wakeup edge. Exercise native observations through the public final renderer;
// neither promoting the row nor dropping a known edge may resolve this clash.
func TestProjectionAdjacentRolePreservesKnownWakeupWithoutDenyingEdge(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("missing trace fixture")
	}
	body, _, ok = strings.Cut(body, "\n'\n")
	if !ok {
		t.Fatal("unterminated trace fixture")
	}
	for _, renamed := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed=%t", renamed), func(t *testing.T) {
			trace := body
			cookie, network, pool := "cookie-200", "network-300", "threadpool-400"
			if renamed {
				trace = strings.NewReplacer("app", "client", "cookie", "session", "network", "transport", "threadpool", "executor", "logger", "recorder").Replace(trace)
				cookie, network, pool = "session-200", "transport-300", "executor-400"
			}
			path := hmc081WriteTrace(t, trace)
			bus, _, _ := hmc17NamedPathContext(t)
			var records []types.ObservationRecord
			for _, view := range []string{"root_cause_rank", "wakeup_chain", "window_stats"} {
				result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
				if !result.Success {
					t.Fatalf("public %s: %s", view, result.Summary)
				}
				records = append(records, result.Observations...)
			}
			before, err := json.Marshal(records)
			if err != nil {
				t.Fatal(err)
			}
			for _, lang := range []string{"zh", "en"} {
				md := p3mRenderUserFace(t, records, lang)
				want := "邻近支撑(不计入链上影响)"
				if lang == "en" {
					want = "adjacent support (not counted as on-chain impact)"
				}
				for _, denied := range []string{"无直接唤醒边", "no direct wake edge"} {
					if strings.Contains(md, denied) {
						t.Errorf("adjacent role denied independent wakeup evidence (%s): %s", lang, denied)
					}
				}
				for _, pair := range []struct{ subject, edge, value string }{
					{cookie, network + " → " + cookie + " @ 2.018000s", "17.000ms"},
					{network, pool + " → " + network + " @ 2.016000s", "14.000ms"},
				} {
					found := false
					for _, block := range strings.Split(md, "**[") {
						heading, _, _ := strings.Cut(block, "\n")
						if strings.Contains(heading, pair.subject) && strings.Contains(block, want) && strings.Contains(block, pair.edge) {
							found = true
						}
					}
					occupancyFound := false
					for _, line := range strings.Split(md, "\n") {
						if strings.HasPrefix(line, "|") && strings.Contains(line, pair.subject) && strings.Contains(line, "sleep") && strings.Contains(line, pair.value) {
							occupancyFound = true
						}
					}
					if !found || !occupancyFound {
						t.Errorf("final %s must keep adjacent role, known edge and occupancy for %s; matching detail=%t, matching occupancy row=%t", lang, pair.subject, found, occupancyFound)
					}
				}
				for _, value := range []string{"20.000ms", "11.000ms", "1.000ms", "19.500ms"} {
					if !strings.Contains(md, value) {
						t.Errorf("display repair lost native %s (%s)", value, lang)
					}
				}
			}
			after, err := json.Marshal(records)
			if err != nil || string(before) != string(after) {
				t.Fatal("display changed native observation identities, values, scopes or causal roles")
			}
		})
	}
}

func TestProjectionAdjacentRoleDoesNotGrantOrRemoveWakeupPoint(t *testing.T) {
	for _, known := range []bool{false, true} {
		for _, zh := range []bool{false, true} {
			row := runtimeTraceProjTreeRow{
				Kind: runtimeTraceProjTreeRowAdjacent,
				Node: types.TraceCausalProjectionNode{
					Subject: "worker-200", StateKind: "sleep", ChainRelevance: "adjacent",
					ImpactMS: 7, DrilldownTarget: "upstream-300", DrilldownWakeupTs: 2.018,
					DrilldownWakeupLine: 13, DrilldownWakeupPointKnown: known,
				},
			}
			before, _ := json.Marshal(row)
			want := "adjacent support (not counted as on-chain impact)"
			if zh {
				want = "邻近支撑(不计入链上影响)"
			}
			if got := runtimeTraceProjDetailRelationCell(row, zh, false); got != want {
				t.Errorf("role must not depend on whether an independent edge is known: got %q, want %q", got, want)
			}
			point := runtimeTraceProjDetailWakeupPoint(row.Node, zh)
			if known && !strings.Contains(point, "upstream-300 → worker-200 @ 2.018000s") {
				t.Errorf("lost independently known edge: %q", point)
			}
			if !known && point != "" {
				t.Errorf("adjacent role invented a wakeup point: %q", point)
			}
			after, _ := json.Marshal(row)
			if string(before) != string(after) {
				t.Fatal("display changed causal role or native values")
			}
		}
	}
}
