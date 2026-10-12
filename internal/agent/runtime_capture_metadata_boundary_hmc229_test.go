package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC229MetadataBadSideAliasAndForkIsolation(t *testing.T) {
	for _, bad := range []int{0, 1} {
		t.Run([]string{"bad_baseline", "bad_current"}[bad], func(t *testing.T) {
			dir := t.TempDir()
			var paths [2]string
			var original [2]string
			for i, name := range []string{"baseline", "current"} {
				body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/" + name + ".data")
				if err != nil {
					t.Fatal(err)
				}
				if i == bad {
					body = nil
				}
				original[i] = filepath.Join(dir, name+".data")
				if err := os.WriteFile(original[i], body, 0600); err != nil {
					t.Fatal(err)
				}
				paths[i] = filepath.Join(dir, name+".alias")
				if err := os.Symlink(original[i], paths[i]); err != nil {
					t.Fatal(err)
				}
			}
			scope := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeUnspecified, Comparison: &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: paths[0], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[0]}}, Current: types.RuntimeArtifactComparisonSide{Path: paths[1], RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: paths[1]}}}}
			rm := types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, RuntimeArtifactScopeProfile: scope, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("采集信息"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, "runtime")})}
			ctx.Mutable.SetRequestModel(rm)
			catalog, err := tracecatalog.DiscoverIndependentPair(t.Context(), paths)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ctx.Mutable.InstallTraceCatalog(catalog); err != nil {
				t.Fatal(err)
			}
			query := func(path string) any { return map[string]any{"source": "path", "path": path, "view": "measurements"} }
			raw, _ := json.Marshal(map[string]any{"comparison": map[string]any{"baseline": query(paths[0]), "current": query(paths[1])}})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !result.Success {
				t.Fatalf("pair %v %s", err, result.Summary)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			status := func() types.RuntimeMeasurementTable {
				for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
					if strings.HasPrefix(table.ObservationID, "trace_measurement_acquisition:") {
						return table
					}
				}
				t.Fatal("missing current shared status")
				return types.RuntimeMeasurementTable{}
			}
			table := status()
			healthy := 1 - bad
			want := []string{"1176", "1080"}[healthy]
			if len(table.Rows) != 2 || !strings.Contains(table.Rows[healthy][5], want) || strings.Contains(table.Rows[bad][5], want) || table.Rows[bad][1] == "available" {
				t.Fatalf("failed/alias side borrowed metadata: %+v", table.Rows)
			}
			fork := ctx.Mutable.ForkForExploreDispatch()
			if err := os.WriteFile(original[healthy], []byte("changed original"), 0600); err != nil {
				t.Fatal(err)
			}
			table = status()
			if strings.Contains(table.Rows[healthy][5], want) || table.Rows[healthy][1] != "stale" {
				t.Fatalf("changed source metadata survived: %+v", table.Rows)
			}
			ctx.Mutable.ResetTurnAArtifacts()
			ctx.Mutable.MergeExploreFork(fork)
			encoded, _ := json.Marshal(types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices())
			if strings.Contains(string(encoded), "physical_width=observed") {
				t.Fatal("old fork revived metadata", string(encoded))
			}
		})
	}
}
