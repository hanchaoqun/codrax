package tool

import (
	"encoding/json"
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
			before, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
			for i := range ledger.Records {
				var kept []string
				for _, n := range ledger.Records[i].RichNotes {
					if !strings.HasPrefix(n, traceWakeupEventNote) {
						kept = append(kept, n)
					}
				}
				ledger.Records[i].RichNotes = kept
			}
			after, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
			if string(before) != string(after) {
				t.Fatal("diagram carrier altered causal projection")
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
