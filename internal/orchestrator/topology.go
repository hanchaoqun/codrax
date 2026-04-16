package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// pipelineTopology is the hardcoded 4-stage × 4-agent pipeline.
// Each entry names the agent and skill bound to the stage.
//
// Before the 2026-04-14 simplification this topology lived in
// config/orchestrator.yaml alongside 5 extra write-pipeline stages
// and priority-weighted transition tables. Both are gone — the
// orchestrator now walks the DAG produced by the analyzer and only
// needs agent/skill lookups per stage.
var pipelineTopology = map[types.PipelineStage]struct {
	Agent    types.AgentName
	Skill    string
	Terminal bool
}{
	types.StageAnalyze:  {Agent: types.AgentAnalyzer, Skill: "analysis-skill"},
	types.StageExplore:  {Agent: types.AgentExplorer, Skill: "explore-skill"},
	types.StageExtract:  {Agent: types.AgentExtractor, Skill: "extract-skill"},
	types.StageFinalize: {Agent: types.AgentFinalizer, Skill: "answer-document-skill", Terminal: true},
}
