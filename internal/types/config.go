package types

// PipelineSettings controls optional pipeline budget knobs loaded
// from codrax.yaml. The stage topology itself is hardcoded in
// internal/orchestrator/topology.go — only per-run budget limits
// still live in config.
type PipelineSettings struct {
	MaxRetriesPerStage int `yaml:"max_retries_per_stage"`

	// MaxStageVisits caps how many times any single stage may be
	// entered during one Run before the orchestrator gives up and
	// forces finalize. A value <= 0 falls back to DefaultMaxStageVisits.
	// This is the second guard against runaway loops (in addition to
	// max-steps) and fires earlier when the pipeline is bouncing
	// between the same stages without making progress.
	MaxStageVisits int `yaml:"max_stage_visits"`
}

// DefaultMaxStageVisits is the fallback value used when
// PipelineSettings.MaxStageVisits is zero or negative.
const DefaultMaxStageVisits = 4
