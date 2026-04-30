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
var pipelineTopology = func() map[types.PipelineStage]struct {
	Agent    types.AgentName
	Skill    string
	Terminal bool
} {
	out := make(map[types.PipelineStage]struct {
		Agent    types.AgentName
		Skill    string
		Terminal bool
	}, len(types.AllStageBindings()))
	for _, binding := range types.AllStageBindings() {
		out[binding.Stage] = struct {
			Agent    types.AgentName
			Skill    string
			Terminal bool
		}{
			Agent:    binding.Agent,
			Skill:    binding.Skill,
			Terminal: binding.Terminal,
		}
	}
	return out
}()

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
