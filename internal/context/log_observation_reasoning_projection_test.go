package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildPromptContextPeerLogObservationAuthorityIsConsistentAcrossStages(t *testing.T) {
	const label = "unbound culprit label"
	const interpretation = "unbound propagation interpretation"
	const evidence = "failure at item 7"
	bundle := &types.LogBundle{
		Errors: []types.LogError{
			{Type: "outer", Message: "request failed", Frames: []types.LogFrame{{ArtifactFile: "client.any", Line: 12, Func: "Client.send", Raw: "at Client.send (client.any:12)"}},
				Cause:         &types.LogError{Type: "inner", Message: "Caused by: inner", Frames: []types.LogFrame{{ArtifactFile: "service.any", Line: 15, Func: "Service.execute", Raw: "at Service.execute (service.any:15)"}}},
				CauseRelation: &types.LogCauseRelation{Authority: types.LogCauseAuthorityExplicitArtifactMarker, Marker: "Caused by: inner"}},
			{Type: "peer", Message: "separate event"},
		},
		Observations: []types.LogObservation{{Kind: types.LogObservationRuntimeEvent, Subject: label, Summary: interpretation, Evidence: evidence, LineStart: 4, Diagnostic: true}},
	}
	for _, tc := range []struct {
		name  types.AgentName
		stage types.PipelineStage
	}{
		{types.AgentAnalyzer, types.StageAnalyze},
		{types.AgentExplorer, types.StageExplore},
		{types.AgentFinalizer, types.StageFinalize},
	} {
		t.Run(string(tc.stage), func(t *testing.T) {
			mutable := types.NewMutableState("locate each failure")
			mutable.SetLogTriage(bundle)
			ac := &types.AgentContext{
				AgentName: tc.name, Stage: tc.stage, Objective: "locate each failure", LogTriage: bundle, Mutable: mutable,
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{LogTriage: bundle}},
			}
			prompt := BuildPromptContext(ac, &skill.Config{Name: "inspect", Goal: "report the observed frames"})
			var content strings.Builder
			for _, section := range prompt.UserSections {
				content.WriteString(section.Content)
				content.WriteString("\n")
			}
			for _, forbidden := range []string{label, interpretation} {
				if strings.Contains(content.String(), forbidden) {
					t.Errorf("%s received unbound interpretation through a secondary context surface: %q", tc.stage, forbidden)
				}
			}
			for _, required := range []string{evidence, "Client.send", "Service.execute", "explicit artifact marker: `Caused by: inner`", "separate event"} {
				if !strings.Contains(content.String(), required) {
					t.Errorf("%s lost raw observation or nested cause evidence %q", tc.stage, required)
				}
			}
		})
	}
}
