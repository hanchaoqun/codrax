package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public query and the real projection/overview lowering. These
// PIC seats have intersecting locators but disjoint runnable components.
func TestTraceElimEnvelopePublicDisjointComponents(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
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
			if renamed {
				trace = strings.NewReplacer("cookie", "session", "network", "transport", "threadpool", "executor").Replace(trace)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "components.ftrace")
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success || len(result.Observations) == 0 {
				t.Fatalf("public query: %v %+v", err, result)
			}
			payload, err := os.ReadFile(result.Observations[0].SourceRef.PayloadRef)
			if err != nil {
				t.Fatal(err)
			}
			var wire tracequery.Result
			if err := json.Unmarshal(payload, &wire); err != nil || wire.WindowStats == nil {
				t.Fatalf("public scheduler evidence: %v", err)
			}
			var components []tracequery.ThreadDuration
			for _, cpu := range wire.WindowStats.CPUPressure {
				components = append(components, cpu.TopRunnable...)
			}
			if len(components) != 3 {
				t.Fatalf("fixture must retain three runnable components, got %+v", components)
			}
			for i, a := range components {
				for _, b := range components[i+1:] {
					if types.TraceCausalProjectionIntervalsOverlap(a.StartTs, a.EndTs, b.StartTs, b.EndTs) {
						t.Fatalf("counterexample requires disjoint actual components: %+v / %+v", a, b)
					}
				}
			}
			projection := types.TraceCausalProjectionFromObservationRecords(result.Observations)
			before, _ := json.Marshal(result.Observations)
			projectionBefore, _ := json.Marshal(projection)
			sections := TraceAnswerDecisionDirectionSections(projection)
			found := false
			for _, section := range sections {
				if section.Direction == "lock_priority" {
					found = true
					if len(section.Members) != len(components) || section.Arithmetic != types.TraceAnswerDirectionArithmeticOverlap || section.SubtotalMS != 0 {
						t.Fatalf("conservative arithmetic/roster changed: %+v", section)
					}
				}
			}
			if !found {
				t.Fatal("PIC section disappeared")
			}
			for _, zh := range []bool{true, false} {
				model, fence := elimRenderOverview(t, projection, zh)
				traceElimAssertEnvelopeDisclosure(t, fence, strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, zh), "\n"), zh)
				if tree := runtimeTraceProjTreeFence(model, zh); !strings.Contains(tree, "trace-causal-projection") || !strings.Contains(tree, "app-100") {
					t.Fatalf("causal projection disappeared: %s", tree)
				}
			}
			after, _ := json.Marshal(result.Observations)
			projectionAfter, _ := json.Marshal(projection)
			if string(before) != string(after) || string(projectionBefore) != string(projectionAfter) {
				t.Fatal("display changed source evidence, values, ranking or causal qualification")
			}
		})
	}
}

func traceElimAssertEnvelopeDisclosure(t *testing.T, head, legend string, zh bool) {
	t.Helper()
	want, boundary := "定位范围相交,合计不可直加", "不代表计量分量真实重叠"
	if !zh {
		want, boundary = "locator ranges overlap; do not add", "does not establish overlap of the measured components"
	}
	if !strings.Contains(head, want) || !strings.Contains(legend, boundary) {
		t.Errorf("envelope-only signal needs locator disclosure and component boundary (zh=%t):\n%s\n%s", zh, head, legend)
	}
	for _, wrong := range []string{"成员区间重叠", "member intervals overlap", "直接相加会重复计费", "adding the values would double-bill"} {
		if strings.Contains(head+legend, wrong) {
			t.Errorf("locator overlap minted a physical-component claim: %s", wrong)
		}
	}
}

// No mechanism, dominant-state label, equal value or PIC component split can
// turn a broad locator into a physical support interval. Vary both scalars.
func TestTraceElimEnvelopeHeterogeneousMeasures(t *testing.T) {
	for _, kind := range []string{"runnable_wait", "d_state_or_io_wait", "priority_inversion_candidate", "mixed_pic", "running_only_pic", "sleep"} {
		t.Run(kind, func(t *testing.T) {
			members := []types.TraceCausalProjectionNode{
				elimv2DirectionNode("a", "alpha-12", kind, "runnable", 1, 7.405, 10, "lock_priority", 10.010, 10.090),
				elimv2DirectionNode("b", "beta-13", kind, "s_sleep", 2, 4.710, 30, "lock_priority", 10.030, 10.110),
			}
			section := runtimeTraceProjElimSection{direction: "lock_priority", maxEff: members[0].EffectiveImpactMS}
			for i := range members {
				n := &members[i]
				n.RankBoardTarget, n.RankBoardParamsFingerprint = "target-1", "one-board"
				n.RankQueryWindowStartTs, n.RankQueryWindowEndTs = 10, 10.2
				if kind == "priority_inversion_candidate" || kind == "mixed_pic" || kind == "running_only_pic" {
					n.TypeToken = "priority_inversion_candidate"
					n.GatedRunnableMS = n.EffectiveImpactMS
					if kind == "mixed_pic" {
						n.GatedRunnableMS, n.GatedRunningDeficitMS = n.EffectiveImpactMS/2, n.EffectiveImpactMS/2
					} else if kind == "running_only_pic" {
						n.GatedRunnableMS, n.GatedRunningDeficitMS = 0, n.EffectiveImpactMS
					}
				}
				section.entries = append(section.entries, runtimeTraceProjElimEntry{row: runtimeTraceProjTreeRow{Node: *n, HasData: true}})
			}
			before, _ := json.Marshal([]types.TraceCausalProjectionNode{section.entries[0].row.Node, section.entries[1].row.Node})
			for _, zh := range []bool{true, false} {
				marks := &runtimeTraceProjMarkSet{}
				head := runtimeTraceProjElimSectionHeadLine(section, false, true, "", marks, zh)
				traceElimAssertEnvelopeDisclosure(t, head, strings.Join(runtimeTraceProjLegendGroupLines(marks, zh), "\n"), zh)
				if arithmetic, subtotal := runtimeTraceProjElimSectionLadder(section, false, true); arithmetic != types.TraceAnswerDirectionArithmeticOverlap || subtotal != 0 {
					t.Fatalf("arithmetic changed: %s %.3f", arithmetic, subtotal)
				}
			}
			after, _ := json.Marshal([]types.TraceCausalProjectionNode{section.entries[0].row.Node, section.entries[1].row.Node})
			if string(before) != string(after) {
				t.Fatal("display changed section members")
			}
		})
	}
}
