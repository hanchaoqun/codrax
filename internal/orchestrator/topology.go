package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// pipelineTopology is the hardcoded 4-stage × 4-agent main pipeline.
// Each entry names the agent and skill bound to the stage.
//
// Before the 2026-04-14 simplification this topology lived in
// config/orchestrator.yaml alongside 5 extra write-pipeline stages
// and priority-weighted transition tables. Both are gone — the
// orchestrator now walks the DAG produced by the analyzer and only
// needs agent/skill lookups per stage.
//
// Conditional pre-stages (log_triage + perf_triage) live in
// preStages below and are dispatched once each before analyze when
// their Guard returns true. Pre-stages are ADVISORY: on failure
// the orchestrator logs a warning and the main pipeline continues
// with the relevant BusContext side-effect (e.g. bus.Mutable.LogTriage()
// or bus.Mutable.PerfTrace()) staying at its zero value.
var pipelineTopology = map[types.PipelineStage]struct {
	Agent    types.AgentName
	Skill    string
	Terminal bool
}{
	types.StageLogTriage:  {Agent: types.AgentLogTriager, Skill: "log-triage-skill"},
	types.StagePerfTriage: {Agent: types.AgentPerfTriager, Skill: "perf-triage-skill"},
	types.StageAnalyze:    {Agent: types.AgentAnalyzer, Skill: "analysis-skill"},
	types.StageExplore:   {Agent: types.AgentExplorer, Skill: "explore-skill"},
	types.StageExtract:   {Agent: types.AgentExtractor, Skill: "extract-skill"},
	types.StageFinalize:  {Agent: types.AgentFinalizer, Skill: "answer-document-skill", Terminal: true},

	// B0 write-mode stages. Only dispatched when BusContext.Mode is
	// Plan / Apply / Verify — runPlanPhase / runApplyPhase /
	// runVerifyPhase are the exclusive callers, and they invoke
	// dispatchStage directly rather than routing through
	// runTaskGraph (the scheduler never sees these stages, so no
	// stageMapping entry is needed). This keeps write-mode
	// orthogonal to the read-mode scheduler and preserves the L1
	// byte-identity red line.
	types.StagePlan:   {Agent: types.AgentPlanner, Skill: "change-plan-skill"},
	types.StageApply:  {Agent: types.AgentCoder, Skill: "code-write-skill"},
	types.StageVerify: {Agent: types.AgentVerifier, Skill: "test-execute-skill"},
}

// preStageEntry describes one conditional pre-stage. Guard is called
// exactly once per Run; a false return skips the stage entirely. The
// orchestrator iterates preStages in declaration order before Phase 1
// analyze.
type preStageEntry struct {
	Stage types.PipelineStage
	Guard func(*types.BusContext) bool
}

// preStages is the declarative list of conditional pre-stages.
// Current membership:
//
//   - StageLogTriage: fires when BusContext.AttachedLog is non-empty.
//     Writes bus.Mutable.LogTriage() on success.
//
// The list is explicitly a slice (not a map) so ordering is stable
// and future additions (e.g. a profiler-output ingester, a strace
// decoder) land in a predictable position relative to log triage.
var preStages = []preStageEntry{
	{
		Stage: types.StageLogTriage,
		Guard: func(bus *types.BusContext) bool {
			return bus != nil && strings.TrimSpace(bus.AttachedLog) != ""
		},
	},
	{
		// StagePerfTriage fires when the user attached a HiTrace /
		// Android systrace payload via --htrace. Mirror of log_triage
		// for the performance channel — writes bus.Mutable.PerfTrace()
		// on success. Advisory; main pipeline continues on failure.
		Stage: types.StagePerfTriage,
		Guard: func(bus *types.BusContext) bool {
			return bus != nil && strings.TrimSpace(bus.AttachedHitrace) != ""
		},
	},
}
