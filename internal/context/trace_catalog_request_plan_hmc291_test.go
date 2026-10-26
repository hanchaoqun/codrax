package context_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC291AcceptedWorkNavigationAndActualSelectionStaySeparate(t *testing.T) {
	bus := hmc259CatalogBus(t)
	start, end := 10., 10.05
	rm := types.RequestModel{
		RuntimeArtifactWorkProfile:  &types.RuntimeArtifactWorkProfile{Mode: types.RuntimeArtifactWorkRepresentative, PerSpecificationLimit: 1, SourceQuote: "each group one representative"},
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "threads101and202"},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 101, Source: "user_explicit"}, {Kind: types.RuntimeTargetKindThread, PID: 202, Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10..10.05s"},
	}
	bus.Mutable.SetRequestModel(rm)
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	before := hmc259CatalogContext(t, bus, types.StageExplore)
	for _, want := range []string{`runtime_artifact_work_request={"mode":"representative","per_specification_limit":1}`, "request_plan", `representative_selection_state="not_selected"`} {
		if !strings.Contains(before, want) {
			t.Fatalf("accepted intent lost or promoted to execution: missing %s in %s", want, before)
		}
	}
	out, err := (&tool.TraceCatalog{}).Execute(bus, []byte(`{"request_plan":{"view":"resource_stack"}}`))
	if err != nil || !out.Success {
		t.Fatalf("public request plan: %v %s", err, out.Summary)
	}
	var page struct {
		Calls map[string]json.RawMessage `json:"query_calls"`
	}
	if err := json.Unmarshal([]byte(out.Summary), &page); err != nil || len(page.Calls) != 2 {
		t.Fatal("missing selected calls")
	}
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		content := hmc259CatalogContext(t, bus, stage)
		if !strings.Contains(content, `representative_selection_state="selected"`) || !strings.Contains(content, `"attempts":0`) || !strings.Contains(content, "not_executed") || strings.Contains(content, "ImageCache::reserve") {
			t.Fatalf("%s navigation invented native facts/execution: %s", stage, content)
		}
	}
	if got := hmc259CatalogContext(t, bus, types.StageAnalyze); strings.Contains(got, "runtime_artifact_work_request") {
		t.Fatal("work navigation opened analyzer tools")
	}
}

func TestHMC291AcceptedWorkVisibleBeforeAnyDiscovery(t *testing.T) {
	bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("choose two from each group")}
	rm := types.RequestModel{RuntimeArtifactWorkProfile: &types.RuntimeArtifactWorkProfile{Mode: types.RuntimeArtifactWorkRepresentative, PerSpecificationLimit: 2, SourceQuote: "two from each group"}}
	bus.Mutable.SetRequestModel(rm)
	content := hmc259CatalogContext(t, bus, types.StageExplore)
	if !strings.Contains(content, `runtime_artifact_work_request={"mode":"representative","per_specification_limit":2}`) || !strings.Contains(content, "request_plan") || strings.Contains(content, "query_calls=") || len(bus.Mutable.TraceCatalogs()) != 0 {
		t.Fatalf("pre-discovery intent missing or falsely executed: %s", content)
	}
	rm.RuntimeArtifactWorkProfile = nil
	bus.Mutable.SetRequestModel(rm)
	if content := hmc259CatalogContext(t, bus, types.StageExplore); strings.Contains(content, "runtime_artifact_work_request") {
		t.Fatal("question prose alone minted a work profile")
	}
}
