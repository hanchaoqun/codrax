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

	// Agent carries the per-agent tunable limits (iteration caps,
	// tool-history budget, loop-policy defaults, correction retries).
	Agent AgentSettings `yaml:"agent"`

	// ViolationBudget controls Session 11 F5 retry-yield kill
	// switch + fail-loud warning. Zero-value-safe: MinRetryYield=0
	// disables the yield gate (historical behaviour).
	ViolationBudget ViolationBudgetSettings `yaml:"violation_budget"`

	// RetryBudgetByKind is the Session 11 C6 per-ViolationKind
	// retry cap. When non-zero, overrides MaxRetriesPerStage for
	// the relevant kind. Zero entries fall back to
	// MaxRetriesPerStage so legacy tests stay deterministic.
	RetryBudgetByKind RetryBudgetByKindSettings `yaml:"retry_budget_by_kind"`
}

// ViolationBudgetSettings — Session 11 F5 controls. See the
// full-design doc §4 for the semantics.
type ViolationBudgetSettings struct {
	// MaxPatchesPerRun is the global F3 IR-patch cap. The F3
	// IRPatchEngine refuses new PatchRequests once this count is
	// reached; the fail-loud warning fires after that. Default 4.
	MaxPatchesPerRun int `yaml:"max_patches_per_run"`

	// MaxPatchesPerField is the per-IR-field patch cap (same
	// field cannot be patched more than this many times in a
	// single Run, preventing oscillation). Default 2.
	MaxPatchesPerField int `yaml:"max_patches_per_field"`

	// MinRetryYield is the threshold below which the yield check
	// kills a retry: after each window, the scheduler computes
	// Δforced_reads + Δpatches + Δevidence + Δscanned_set; when
	// the sum is less than this value, retry is denied. Default 1.
	// Setting to 0 disables the yield gate entirely.
	MinRetryYield int `yaml:"min_retry_yield"`

	// FailLoudEnabled, when true, prepends an "⚠️ pipeline
	// terminated with unresolved violations" warning to the final
	// answer when the yield kill fires or any budget is exhausted.
	// Default true — never silently hide a failure.
	FailLoudEnabled bool `yaml:"fail_loud_enabled"`

	// YieldKillStage, when true, makes the yield kill per-stage
	// rather than per-Run. Default true; setting false pauses the
	// entire Run on any stage's zero-yield retry.
	YieldKillStage bool `yaml:"yield_kill_stage"`
}

// RetryBudgetByKindSettings — Session 11 C6. Zero entries mean
// "use MaxRetriesPerStage"; positive integers cap that specific
// kind. See ViolationKind constants in types/violation.go.
type RetryBudgetByKindSettings struct {
	ShapeViolation    int `yaml:"shape_violation"`
	CitationViolation int `yaml:"citation_violation"`
	LiteralFormFailed int `yaml:"literal_form_failed"`
	GhostAnchor       int `yaml:"ghost_anchor"`
	SelfRefLiteral    int `yaml:"self_ref_literal"`
	Other             int `yaml:"other"`
}

// For returns the configured retry cap for kind, or fallback when
// the kind has a zero entry. Centralising this lookup so callers
// don't hand-roll the switch.
func (r RetryBudgetByKindSettings) For(kind ViolationKind, fallback int) int {
	pick := func(v int) int {
		if v <= 0 {
			return fallback
		}
		return v
	}
	switch kind {
	case ViolShape, ViolShapeSwap:
		return pick(r.ShapeViolation)
	case ViolCitation:
		return pick(r.CitationViolation)
	case ViolLiteralFormFailed:
		return pick(r.LiteralFormFailed)
	case ViolGhostAnchor:
		return pick(r.GhostAnchor)
	case ViolSelfRefLiteral:
		return pick(r.SelfRefLiteral)
	}
	return pick(r.Other)
}

// DefaultViolationBudgetSettings returns the full-design §4
// defaults: 4/2/1/true/true.
func DefaultViolationBudgetSettings() ViolationBudgetSettings {
	return ViolationBudgetSettings{
		MaxPatchesPerRun:   4,
		MaxPatchesPerField: 2,
		MinRetryYield:      1,
		FailLoudEnabled:    true,
		YieldKillStage:     true,
	}
}

// DefaultRetryBudgetByKindSettings returns the full-design §4
// per-kind budgets: 1/1/2/2/2/1.
//
// CitationViolation dropped from 3 → 1 after field data showed the
// LLM re-emits the same citation set on every retry when it cannot
// produce another grounded cite; MinRetryYield did not kill the
// loop because explorer would often read one more file (nonzero
// yield) that did not translate into an extra citation. The
// fail-loud warning path preserves the original answer and surfaces
// the gap honestly, so an extra retry only spends time.
func DefaultRetryBudgetByKindSettings() RetryBudgetByKindSettings {
	return RetryBudgetByKindSettings{
		ShapeViolation:    1,
		CitationViolation: 1,
		LiteralFormFailed: 2,
		GhostAnchor:       2,
		SelfRefLiteral:    2,
		Other:             1,
	}
}

// AgentSettings carries per-agent tunable limits. Zero values mean
// "use code default" — see DefaultAgentSettings().
type AgentSettings struct {
	// MaxIterations is the ReAct loop ceiling for all agents.
	// Default 20. When i reaches maxIter the ReAct for-loop exits
	// directly and ParseOutput builds StageOutput from whatever was
	// collected — there is NO fallback at the ceiling (explorer's
	// Fallback S1 only fires on LLM soft-stop, not on hard truncation
	// of an actively tool-calling dispatch). Multi-topic scaling in
	// orchestrator.go adds SubTopicExplorerBudgetExtra × subTopics
	// on top, capped at 35; single-topic questions get no scaling.
	MaxIterations int `yaml:"max_iterations"`

	// MaxToolHistoryBytes is the cumulative byte budget for "tool"
	// role messages kept verbatim in the ReAct conversation. Older
	// messages beyond this budget are stubbed. Default 153600 (150 KB).
	MaxToolHistoryBytes int `yaml:"max_tool_history_bytes"`

	// LoopPolicy defaults. See LoopPolicy in internal/agent/loop_policy.go.
	// MinInjectInterval: min iterations between two accepted hint
	// injections. Default 3.
	LoopMinInjectInterval int `yaml:"loop_min_inject_interval"`
	// MaxContinuations: soft-stop continuation hints per dispatch.
	// Default 5.
	LoopMaxContinuations int `yaml:"loop_max_continuations"`
	// MaxMidLoopInjects: mid-loop hint injections per dispatch.
	// Default 6.
	LoopMaxMidLoopInjects int `yaml:"loop_max_midloop_injects"`
	// IdleStopThreshold: consecutive idle iterations before force-stop.
	// Default 2.
	LoopIdleStopThreshold int `yaml:"loop_idle_stop_threshold"`

	// FinalizerMaxCorrectionRetries: soft-stop correction retries when
	// emit_answer_document is missing or rejected (missing required
	// field, non-zero forbidden field, over-cap summary). Default 3.
	//
	// Bumped from 2 to 3 in the 2026-04-17 rejection-over-scrub
	// hardening: forbidden fields that LLMs habitually emit (zombie
	// boolean{}, cross-shape value{}) now fail the call rather than
	// being silently scrubbed, so a realistic dispatch may burn one
	// retry to clear the field before the real answer lands.
	FinalizerMaxCorrectionRetries int `yaml:"finalizer_max_correction_retries"`

	// FinalizerPreservePriorProse toggles the pre-tool-call draft
	// salvage in the finalizer ParseOutput. When the model writes a
	// rich answer as plain prose, fails to call emit_answer_document,
	// and then on the correction retry emits a compressed paraphrase
	// as `summary`, the salvage replaces the shrunken summary with
	// the richer prior draft. Pointer-typed so nil (the absent-YAML
	// state) is distinguishable from an explicit false. Default true.
	FinalizerPreservePriorProse *bool `yaml:"finalizer_preserve_prior_prose"`

	// FinalizerShrinkageMinProseLen is the minimum length (bytes) of
	// the prior prose draft that qualifies for shrinkage salvage.
	// Drafts shorter than this are treated as placeholder content,
	// not a real answer body, and are not copied into the summary.
	// Default 400.
	FinalizerShrinkageMinProseLen int `yaml:"finalizer_shrinkage_min_prose_len"`

	// FinalizerShrinkageRatio is the ratio threshold below which the
	// emitted summary is considered "shrunken" relative to the prior
	// prose draft. When len(summary) / len(prior) falls under this
	// value AND the prior draft meets the min-length floor, the
	// salvage replaces the summary. Default 0.5.
	FinalizerShrinkageRatio float64 `yaml:"finalizer_shrinkage_ratio"`

	// ExtractorMaxCorrectionRetries: soft-stop correction retries when
	// emit_answer_symbol is missing on list_of_symbols questions.
	// Default 1.
	ExtractorMaxCorrectionRetries int `yaml:"extractor_max_correction_retries"`

	// SubTopicPrescanBudgetExtra is the number of extra prescan rounds
	// granted per 2 sub-topics when RequestModel.SubTopics > 1.
	// Default 1. The adjusted prescan budget is capped at base + 2.
	SubTopicPrescanBudgetExtra int `yaml:"subtopic_prescan_budget_extra"`

	// SubTopicExplorerBudgetExtra is the number of extra explorer
	// iterations granted per sub-topic. Default 3. The adjusted
	// MaxIterations is capped at 35.
	SubTopicExplorerBudgetExtra int `yaml:"subtopic_explorer_budget_extra"`

	// SubTopicPipelineStepsExtra is the number of extra pipeline steps
	// granted per sub-topic. Default 5. The adjusted step budget is
	// capped at 100.
	SubTopicPipelineStepsExtra int `yaml:"subtopic_pipeline_steps_extra"`

	// SubTopicRetryBudgetExtra is the number of extra retry budget
	// granted per 2 sub-topics. Default 1. The adjusted retry budget
	// is capped at 5.
	SubTopicRetryBudgetExtra int `yaml:"subtopic_retry_budget_extra"`

	// InvestigationCompletePolicy controls how the DAG scheduler treats
	// nodes when the LLM has called emit_investigation_complete.
	//
	//   "soft"     (default) — inject InvestigationComplete=true into
	//              criterion.Env so evidence_count thresholds drop to
	//              >=1 instead of the template's declared floor. The
	//              node still needs at least 1 evidence item.
	//
	//   "override" — skip SuccessCriteria evaluation entirely for
	//              explore-type nodes and mark them done. Fastest, but
	//              relies entirely on the AnswerContract checker at
	//              finalize for quality assurance.
	//
	//   "strict"   — ignore emit_investigation_complete at the DAG
	//              level; the template's declared thresholds are
	//              enforced unconditionally. Historical behaviour
	//              before this config existed.
	InvestigationCompletePolicy string `yaml:"investigation_complete_policy"`

	// PriorConvPolicy controls how the REPL-assembled Prior Conversation
	// block is surfaced to the 4 pipeline stages. The REPL always stores
	// and retrieves prior turns; this knob gates VISIBILITY, not
	// persistence, so flipping the policy takes effect on the next
	// dispatch without any data migration.
	//
	//   "always"   — historical behaviour; every stage sees Prior.
	//                Preserved as the opt-out for continuity-sensitive
	//                debugging sessions.
	//
	//   "analyzer" (default) — only the analyzer stage sees Prior, where
	//                it is useful for entity disambiguation ("它 = last
	//                turn's subject"). Explorer / extractor / finalizer
	//                stay blind so they cannot copy a prior-turn wrong
	//                answer verbatim. AnalysisIR.RequestModel.Entities
	//                carries any disambiguated identifiers downstream.
	//
	//   "continue" — analyzer always; explorer/extractor/finalizer see
	//                Prior only when the current request is a
	//                continuation per types.IsContinuation (leading
	//                "再/继续/more on/..." or bare pronoun head).
	//
	//   "never"    — no stage sees Prior. Extreme isolation, not
	//                recommended outside stress tests.
	PriorConvPolicy string `yaml:"prior_conversation_policy"`
}

const (
	// ICPolicySoft is the default: lower evidence_count threshold to 1.
	ICPolicySoft = "soft"
	// ICPolicyOverride skips criteria entirely.
	ICPolicyOverride = "override"
	// ICPolicyStrict ignores investigation_complete at DAG level.
	ICPolicyStrict = "strict"

	// PriorConvPolicyAlways: every stage sees Prior Conversation.
	// Historical behaviour; kept as an opt-out.
	PriorConvPolicyAlways = "always"
	// PriorConvPolicyAnalyzer (default): only the analyzer sees Prior.
	PriorConvPolicyAnalyzer = "analyzer"
	// PriorConvPolicyContinue: analyzer always; downstream stages see
	// Prior only when types.IsContinuation returns true on the current
	// request.
	PriorConvPolicyContinue = "continue"
	// PriorConvPolicyNever: no stage sees Prior. Extreme isolation.
	PriorConvPolicyNever = "never"
)

// DefaultAgentSettings returns the code defaults for all agent limits.
func DefaultAgentSettings() AgentSettings {
	t := true
	return AgentSettings{
		MaxIterations:                 20,
		MaxToolHistoryBytes:           150 * 1024,
		LoopMinInjectInterval:         3,
		LoopMaxContinuations:          5,
		LoopMaxMidLoopInjects:         6,
		LoopIdleStopThreshold:         2,
		FinalizerMaxCorrectionRetries: 3,
		FinalizerPreservePriorProse:   &t,
		FinalizerShrinkageMinProseLen: 400,
		FinalizerShrinkageRatio:       0.5,
		ExtractorMaxCorrectionRetries: 1,
		SubTopicPrescanBudgetExtra:    1,
		SubTopicExplorerBudgetExtra:   3,
		SubTopicPipelineStepsExtra:    5,
		SubTopicRetryBudgetExtra:      1,
		InvestigationCompletePolicy:   ICPolicySoft,
		PriorConvPolicy:               PriorConvPolicyAnalyzer,
	}
}

// ResolvedAgentSettings returns s with zero fields filled from
// DefaultAgentSettings(). Call once at startup.
func ResolvedAgentSettings(s AgentSettings) AgentSettings {
	d := DefaultAgentSettings()
	if s.MaxIterations == 0 {
		s.MaxIterations = d.MaxIterations
	}
	if s.MaxToolHistoryBytes == 0 {
		s.MaxToolHistoryBytes = d.MaxToolHistoryBytes
	}
	if s.LoopMinInjectInterval == 0 {
		s.LoopMinInjectInterval = d.LoopMinInjectInterval
	}
	if s.LoopMaxContinuations == 0 {
		s.LoopMaxContinuations = d.LoopMaxContinuations
	}
	if s.LoopMaxMidLoopInjects == 0 {
		s.LoopMaxMidLoopInjects = d.LoopMaxMidLoopInjects
	}
	if s.LoopIdleStopThreshold == 0 {
		s.LoopIdleStopThreshold = d.LoopIdleStopThreshold
	}
	if s.FinalizerMaxCorrectionRetries == 0 {
		s.FinalizerMaxCorrectionRetries = d.FinalizerMaxCorrectionRetries
	}
	if s.FinalizerPreservePriorProse == nil {
		s.FinalizerPreservePriorProse = d.FinalizerPreservePriorProse
	}
	if s.FinalizerShrinkageMinProseLen == 0 {
		s.FinalizerShrinkageMinProseLen = d.FinalizerShrinkageMinProseLen
	}
	if s.FinalizerShrinkageRatio == 0 {
		s.FinalizerShrinkageRatio = d.FinalizerShrinkageRatio
	}
	if s.ExtractorMaxCorrectionRetries == 0 {
		s.ExtractorMaxCorrectionRetries = d.ExtractorMaxCorrectionRetries
	}
	if s.SubTopicPrescanBudgetExtra == 0 {
		s.SubTopicPrescanBudgetExtra = d.SubTopicPrescanBudgetExtra
	}
	if s.SubTopicExplorerBudgetExtra == 0 {
		s.SubTopicExplorerBudgetExtra = d.SubTopicExplorerBudgetExtra
	}
	if s.SubTopicPipelineStepsExtra == 0 {
		s.SubTopicPipelineStepsExtra = d.SubTopicPipelineStepsExtra
	}
	if s.SubTopicRetryBudgetExtra == 0 {
		s.SubTopicRetryBudgetExtra = d.SubTopicRetryBudgetExtra
	}
	switch s.InvestigationCompletePolicy {
	case ICPolicySoft, ICPolicyOverride, ICPolicyStrict:
		// valid
	default:
		s.InvestigationCompletePolicy = d.InvestigationCompletePolicy
	}
	switch s.PriorConvPolicy {
	case PriorConvPolicyAlways, PriorConvPolicyAnalyzer, PriorConvPolicyContinue, PriorConvPolicyNever:
		// valid
	default:
		s.PriorConvPolicy = d.PriorConvPolicy
	}
	return s
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

// MemorySettings carries tunable limits for the multi-turn REPL memory
// store. Zero values mean "use code default" — see DefaultMemorySettings().
type MemorySettings struct {
	// MaxRecentTurns: number of recent turns kept verbatim in memory
	// before the oldest is LLM-summarized into MEMORY.md. Default 6.
	MaxRecentTurns int `yaml:"max_recent_turns"`

	// MaxRecentBytes: total byte budget for the recent turn buffer.
	// When exceeded, the oldest turn is compacted regardless of
	// MaxRecentTurns. Default 20480 (20 KB).
	MaxRecentBytes int `yaml:"max_recent_bytes"`

	// MaxTurnBodyBytes: maximum size of a single turn's request+response
	// stored to disk. Larger turns are tail-truncated. Default 65536 (64 KB).
	MaxTurnBodyBytes int `yaml:"max_turn_body_bytes"`

	// MaxBuildContextMatches: maximum number of compacted index entries
	// to inline in BuildContext. Default 3.
	MaxBuildContextMatches int `yaml:"max_build_context_matches"`

	// MaxInlinedTurnBytes: maximum bytes from a single matched turn
	// file to inline in BuildContext. Default 8192 (8 KB).
	MaxInlinedTurnBytes int `yaml:"max_inlined_turn_bytes"`

	// MaxBuildContextTotalBytes: total byte budget for all inlined
	// compacted turns in BuildContext. Default 32768 (32 KB).
	MaxBuildContextTotalBytes int `yaml:"max_build_context_total_bytes"`
}

// DefaultMemorySettings returns the code defaults for memory store limits.
func DefaultMemorySettings() MemorySettings {
	return MemorySettings{
		MaxRecentTurns:            6,
		MaxRecentBytes:            20 * 1024,
		MaxTurnBodyBytes:          64 * 1024,
		MaxBuildContextMatches:    3,
		MaxInlinedTurnBytes:       8 * 1024,
		MaxBuildContextTotalBytes: 32 * 1024,
	}
}

// ResolvedMemorySettings returns s with zero fields filled from
// DefaultMemorySettings().
func ResolvedMemorySettings(s MemorySettings) MemorySettings {
	d := DefaultMemorySettings()
	if s.MaxRecentTurns == 0 {
		s.MaxRecentTurns = d.MaxRecentTurns
	}
	if s.MaxRecentBytes == 0 {
		s.MaxRecentBytes = d.MaxRecentBytes
	}
	if s.MaxTurnBodyBytes == 0 {
		s.MaxTurnBodyBytes = d.MaxTurnBodyBytes
	}
	if s.MaxBuildContextMatches == 0 {
		s.MaxBuildContextMatches = d.MaxBuildContextMatches
	}
	if s.MaxInlinedTurnBytes == 0 {
		s.MaxInlinedTurnBytes = d.MaxInlinedTurnBytes
	}
	if s.MaxBuildContextTotalBytes == 0 {
		s.MaxBuildContextTotalBytes = d.MaxBuildContextTotalBytes
	}
	return s
}
