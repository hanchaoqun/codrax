package types

// PipelineSettings controls optional pipeline budget knobs loaded
// from codrax.yaml. The stage topology itself is hardcoded in
// internal/orchestrator/topology.go — only per-run budget limits
// and gate/explore thresholds still live in config.
type PipelineSettings struct {
	// MaxRetriesPerStage bounds the number of times a single
	// stage may be re-dispatched after a failure. The analyze
	// stage reads it via runAnalyzePhase — when the LLM fails to
	// call emit_analysis or the quality gate rejects the IR, the
	// stage is re-dispatched up to this many times before the
	// whole Run terminates with an error.
	MaxRetriesPerStage int `yaml:"max_retries_per_stage"`

	// MaxStageVisits caps how many times any single stage may be
	// entered during one Run before the orchestrator gives up and
	// forces finalize. A value <= 0 falls back to DefaultMaxStageVisits.
	MaxStageVisits int `yaml:"max_stage_visits"`

	// GateThresholds tunes the analyzer quality gate's numeric
	// cutoffs. Defaults live in internal/analysis/gate.Thresholds
	// and are applied when the zero value is loaded.
	GateThresholds GateThresholdSettings `yaml:"gate_thresholds"`

	// Explore carries the explorer-side budget knobs used by the
	// sourcemix throttler.
	Explore ExploreSettings `yaml:"explore"`
}

// GateThresholdSettings mirrors gate.Thresholds through the YAML
// surface. All zero values fall through to package-level defaults.
type GateThresholdSettings struct {
	CoverageMin           float32 `yaml:"coverage_min"`
	CoverageWeightSymbol  float32 `yaml:"coverage_weight_symbol"`
	CoverageWeightConfig  float32 `yaml:"coverage_weight_config"`
	CoverageWeightConcept float32 `yaml:"coverage_weight_concept"`
	HypothesisMinPriority int     `yaml:"hypothesis_min_priority"`
}

// ExploreSettings carries the explorer-side knobs. PerToolDefaultCap
// is used as the fallback per-tool cap when the analyzer's
// NodeBudgetHints leaves a tool unspecified; a cap of 0 means "no
// default cap" and only the hints-declared tools get throttled.
type ExploreSettings struct {
	PerToolDefaultCap int `yaml:"per_tool_default_cap"`
}

// DefaultMaxStageVisits is the fallback value used when
// PipelineSettings.MaxStageVisits is zero or negative.
const DefaultMaxStageVisits = 4
