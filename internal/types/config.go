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
//
// The Heuristics sub-struct holds the tunable thresholds for the
// explorer evaluator's mid-loop and soft-stop detection branches.
// Zero values mean "use the code default" — see
// DefaultExploreHeuristics().
type ExploreSettings struct {
	PerToolDefaultCap int               `yaml:"per_tool_default_cap"`
	Heuristics        ExploreHeuristics `yaml:"heuristics"`
}

// ExploreHeuristics carries the tunable thresholds for the explorer
// evaluator's mid-loop (observeMidLoop) and soft-stop (observeSoftStop)
// detection branches. Every field defaults to 0 = "use code default";
// DefaultExploreHeuristics() returns the code defaults.
type ExploreHeuristics struct {
	// --- Mid-loop thresholds ---

	// MidLoopMinIteration: mid-loop checks are skipped for iterations
	// below this value. The LLM needs a few rounds to establish a
	// pattern before hints are useful. Default 2.
	MidLoopMinIteration int `yaml:"midloop_min_iteration"`

	// SerialBatchThreshold: a tool-call batch of this size or smaller
	// counts as "serial-ish" and increments the serial streak counter.
	// Batches above this threshold reset the streak. Default 2 (catches
	// the common 1-grep + 1-read_file pair). Set to 1 for strict
	// single-call detection.
	SerialBatchThreshold int `yaml:"serial_batch_threshold"`

	// SerialStreakThreshold: the parallel-batching cue fires after this
	// many consecutive serial-ish rounds. Default 2.
	SerialStreakThreshold int `yaml:"serial_streak_threshold"`

	// PartialReadLineThreshold: when a partially-read function has this
	// many or fewer unread lines, the hint suggests a direct read_file;
	// above it, the hint suggests grep-then-read. Used by both mid-loop
	// and soft-stop. Default 150.
	PartialReadLineThreshold int `yaml:"partial_read_line_threshold"`

	// MidLoopEnumCoverage: the mid-loop enumeration check fires when
	// file coverage is below this ratio (0.0–1.0). This is the "early
	// warning" tier — a nudge, not a hard gate. Default 0.6.
	MidLoopEnumCoverage float64 `yaml:"midloop_enum_coverage"`

	// --- Soft-stop thresholds ---

	// SoftStopEnumCoverage: the soft-stop enumeration hard gate fires
	// when file coverage is below this ratio. Stricter than the mid-loop
	// tier — blocks finalization of incomplete enumerations. Default 0.8.
	SoftStopEnumCoverage float64 `yaml:"softstop_enum_coverage"`

	// Phase0MinDiscoveredFiles: the Phase 0 quality gate requires at
	// least this many discovered files before transitioning to Phase 1.
	// Default 3.
	Phase0MinDiscoveredFiles int `yaml:"phase0_min_discovered_files"`

	// Phase0MaxBroadenAttempts: maximum number of "broaden your search"
	// hints before giving up when Phase 0 discovers zero files.
	// Default 2.
	Phase0MaxBroadenAttempts int `yaml:"phase0_max_broaden_attempts"`

	// SymbolMinLenMethod: minimum symbol name length for methods and
	// functions in the unanalyzed-symbol detector. Default 3.
	SymbolMinLenMethod int `yaml:"symbol_min_len_method"`

	// SymbolMinLenOther: minimum symbol name length for types,
	// constants, and other non-method symbols. Default 8.
	SymbolMinLenOther int `yaml:"symbol_min_len_other"`

	// MaxPreScannedPushes: maximum number of "read unread files" pushes
	// before stopping idle-streak resets and letting the loop terminate.
	// Default 3.
	MaxPreScannedPushes int `yaml:"max_prescanned_pushes"`

	// CVPreviewMaxLen: maximum byte length for the concrete-values
	// preview injected into the soft-stop coverage hint. Default 1500.
	CVPreviewMaxLen int `yaml:"cv_preview_max_len"`

	// ParallelUnreadFloor: minimum number of unread files required for
	// the parallel-batching cue to fire. Default 2.
	ParallelUnreadFloor int `yaml:"parallel_unread_floor"`

	// EnumMidLoopUnreadFloor: minimum unread files required for the
	// mid-loop enumeration check to fire. Default 2.
	EnumMidLoopUnreadFloor int `yaml:"enum_midloop_unread_floor"`

	// ErmSuggestLimit: maximum number of ERM file suggestions in the
	// gap-directed hint. Default 3.
	ErmSuggestLimit int `yaml:"erm_suggest_limit"`
}

// DefaultExploreHeuristics returns the code-default values for every
// tunable explorer threshold. Used when the YAML config omits a field
// (zero value) or when tests need a known baseline.
func DefaultExploreHeuristics() ExploreHeuristics {
	return ExploreHeuristics{
		MidLoopMinIteration:      2,
		SerialBatchThreshold:     2,
		SerialStreakThreshold:    2,
		PartialReadLineThreshold: 150,
		MidLoopEnumCoverage:      0.6,
		SoftStopEnumCoverage:     0.8,
		Phase0MinDiscoveredFiles: 3,
		Phase0MaxBroadenAttempts: 2,
		SymbolMinLenMethod:       3,
		SymbolMinLenOther:        8,
		MaxPreScannedPushes:      3,
		CVPreviewMaxLen:          1500,
		ParallelUnreadFloor:      2,
		EnumMidLoopUnreadFloor:   2,
		ErmSuggestLimit:          3,
	}
}

// ResolvedExploreHeuristics returns h with zero fields filled from
// DefaultExploreHeuristics(). Call once at startup.
func ResolvedExploreHeuristics(h ExploreHeuristics) ExploreHeuristics {
	d := DefaultExploreHeuristics()
	if h.MidLoopMinIteration == 0 {
		h.MidLoopMinIteration = d.MidLoopMinIteration
	}
	if h.SerialBatchThreshold == 0 {
		h.SerialBatchThreshold = d.SerialBatchThreshold
	}
	if h.SerialStreakThreshold == 0 {
		h.SerialStreakThreshold = d.SerialStreakThreshold
	}
	if h.PartialReadLineThreshold == 0 {
		h.PartialReadLineThreshold = d.PartialReadLineThreshold
	}
	if h.MidLoopEnumCoverage == 0 {
		h.MidLoopEnumCoverage = d.MidLoopEnumCoverage
	}
	if h.SoftStopEnumCoverage == 0 {
		h.SoftStopEnumCoverage = d.SoftStopEnumCoverage
	}
	if h.Phase0MinDiscoveredFiles == 0 {
		h.Phase0MinDiscoveredFiles = d.Phase0MinDiscoveredFiles
	}
	if h.Phase0MaxBroadenAttempts == 0 {
		h.Phase0MaxBroadenAttempts = d.Phase0MaxBroadenAttempts
	}
	if h.SymbolMinLenMethod == 0 {
		h.SymbolMinLenMethod = d.SymbolMinLenMethod
	}
	if h.SymbolMinLenOther == 0 {
		h.SymbolMinLenOther = d.SymbolMinLenOther
	}
	if h.MaxPreScannedPushes == 0 {
		h.MaxPreScannedPushes = d.MaxPreScannedPushes
	}
	if h.CVPreviewMaxLen == 0 {
		h.CVPreviewMaxLen = d.CVPreviewMaxLen
	}
	if h.ParallelUnreadFloor == 0 {
		h.ParallelUnreadFloor = d.ParallelUnreadFloor
	}
	if h.EnumMidLoopUnreadFloor == 0 {
		h.EnumMidLoopUnreadFloor = d.EnumMidLoopUnreadFloor
	}
	if h.ErmSuggestLimit == 0 {
		h.ErmSuggestLimit = d.ErmSuggestLimit
	}
	return h
}

// DefaultMaxStageVisits is the fallback value used when
// PipelineSettings.MaxStageVisits is zero or negative.
const DefaultMaxStageVisits = 4
