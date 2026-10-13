package tool

import (
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Public queries exercise full statistics, native recursion, a missing wake
// event and two independent display/expansion budgets. Never derive a tree from
// sorted durations, and never replace the complete population with drawn rows.
func TestSleepDependenciesPublicComposition(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
	start, end := 5.0, 5.041
	rm := &types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetWaitOccurrences}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	for _, budget := range []int{1, 64} {
		t.Run(map[int]string{1: "bounded_expansion", 64: "branch_expansion"}[budget], func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("sleep dependencies")}
			r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "pid": 100, "time_start": start, "time_end": end, "min_duration_ms": 0.5, "max_chain_nodes": budget})
			ledger := types.ObservationLedger{Records: r.Observations}
			rows := RuntimeDiagramRelations(ledger, rm)
			pairs := map[string]bool{}
			for _, row := range rows {
				if row.Kind == types.DiagramRelWakeup {
					pairs[row.FromLabel+"->"+row.ToLabel] = true
				}
			}
			want := map[string]bool{"dep2-200->app-100": true, "dep3-300->dep2-200": true}
			if budget > 1 {
				want["dep4-400->dep2-200"] = true
			}
			if !reflect.DeepEqual(pairs, want) {
				t.Fatalf("native branch authority: got %v want %v", pairs, want)
			}
			beforeSet := types.CompileTraceCausalProjectionSet(ledger)
			knownPoints := sleepDependenciesClearInstancePoints(&beforeSet)
			if knownPoints == 0 {
				t.Fatal("native single-wait positive did not retain any exact closure")
			}
			before, _ := json.Marshal(beforeSet)
			for i := range ledger.Records {
				var kept []string
				for _, n := range ledger.Records[i].RichNotes {
					if !strings.HasPrefix(n, traceWakeupEventNote) {
						kept = append(kept, n)
					}
				}
				ledger.Records[i].RichNotes = kept
			}
			afterSet := types.CompileTraceCausalProjectionSet(ledger)
			if sleepDependenciesClearInstancePoints(&afterSet) != 0 {
				t.Fatal("missing native event carrier must not retain any exact closure")
			}
			after, _ := json.Marshal(afterSet)
			if string(before) != string(after) {
				t.Fatal("instance point qualification altered causal structure, navigation or references")
			}
		})
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("upstream summary")}
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "pid": 200, "time_start": start, "time_end": end, "max_chain_nodes": 1, "min_duration_ms": 1})
	found := false
	for _, record := range r.Observations {
		if record.Predicate == "target_sleep_state_summary" && strings.Contains(record.Summary, "4 intervals") {
			found = true
			for _, expected := range []string{"21.8ms", "5.45ms", "max=10ms"} {
				if !strings.Contains(record.Summary, expected) {
					t.Fatalf("full sleep population lost %s: %s", expected, record.Summary)
				}
			}
		}
	}
	if !found {
		t.Fatal("missing full upstream sleep summary including missing-wake and sub-threshold intervals")
	}
}

// Only closure coordinates now depend on the exact event-instance carrier.
// Every causal node, navigation target, evidence reference and accounting field
// remains part of the original byte-preservation assertion.
func sleepDependenciesClearInstancePoints(set *types.TraceCausalProjectionSet) int {
	known := 0
	clear := func(n *types.TraceCausalProjectionNode) {
		if n.DrilldownWakeupPointKnown {
			known++
		}
		n.DrilldownWakeupPointKnown, n.DrilldownWakeupTs, n.DrilldownWakeupLine = false, 0, 0
	}
	for i := range set.Projections {
		p := &set.Projections[i]
		for _, bucket := range [][]types.TraceCausalProjectionNode{p.PrimaryRootCauses, p.OnChainCauses, p.AdjacentCauses, p.BackgroundCauses, p.SemanticSpans, p.SupportingHops} {
			for i := range bucket {
				clear(&bucket[i])
			}
		}
		if p.PrimaryRootCause != nil {
			clear(p.PrimaryRootCause)
		}
	}
	return known
}

// A complete state census and its individual waits are overlapping views,
// not four independent waits whose durations can be added again.
func TestSleepDependenciesCensusDoesNotSumItsOccurrences(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("state census")}
	var ledger types.ObservationLedger
	for _, pid := range []int{100, 200} {
		r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "pid": pid, "time_start": 5.0, "time_end": 5.041, "trace_flavor": "harmony_hitrace"})
		ledger.Records = append(ledger.Records, r.Observations...)
	}
	set := types.CompileTraceCausalProjectionSet(ledger)
	found := false
	for _, p := range set.Projections {
		for _, bucket := range [][]types.TraceCausalProjectionNode{p.OnChainCauses, p.SupportingHops, p.BackgroundCauses, p.PrimaryRootCauses, p.AdjacentCauses} {
			for _, n := range bucket {
				if n.Subject == "dep2-200" && n.Object == "s_sleep" {
					found = true
					if math.Abs(n.ImpactMS-21.8) > 0.000001 {
						b, _ := json.MarshalIndent(n, "", "  ")
						t.Fatalf("complete 21.8ms account must not sum its own waits: %s", b)
					}
				}
			}
		}
	}
	if !found {
		b, _ := json.MarshalIndent(set, "", "  ")
		t.Fatalf("lost the upstream sleep account: %s", b)
	}
}
