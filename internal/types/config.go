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

	// WriteRetryBudget caps T4's verify→plan retry cycle inside
	// ModeApply (the scheduler-driven write DAG runs verify → plan
	// via EdgeValidationFeedback when verify SuccessCriteria fail).
	// Zero preserves fail-loud semantics (one attempt, surface
	// failure). Positive values enable retry with PlanningHint
	// seeded from the failure summary. Hard-capped by
	// WriteRetryBudgetCeil (default 5) inside SetWriteRetryBudget.
	WriteRetryBudget int `yaml:"write_retry_budget"`

	// WriteRetryBudgetCeil is the absolute ceiling enforced by
	// SetWriteRetryBudget on WriteRetryBudget. Default 5; higher
	// values almost always mean misconfiguration that would burn
	// LLM tokens on an unfixable plan. Zero falls back to default.
	WriteRetryBudgetCeil int `yaml:"write_retry_budget_ceil"`

	// TransientRetryBudget caps how many times a single Run will
	// retry a stage that failed with a transient dispatch error
	// (LLM stream stall / first-byte timeout / 429 / 5xx / network
	// blip). Distinct from WriteRetryBudget and the per-kind
	// content-retry counters because the consumer semantics differ:
	//   - Transient retry has no learning signal — same prompt is
	//     re-sent; only the network attempt is fresh.
	//   - SC / contract retry has structured feedback to learn from.
	// Sharing a single counter (the pre-fix shape) caused 3 plan
	// stalls to drain the verify→plan budget so that a real verify
	// failure could not retry. Default 1: one transient retry is
	// enough to ride through a brief network blip; structurally
	// stuck cases are caught by the stall plateau detector before
	// burning even that. Hard-capped by TransientRetryBudgetCeil.
	TransientRetryBudget int `yaml:"transient_retry_budget"`

	// TransientRetryBudgetCeil is the absolute ceiling enforced by
	// SetTransientRetryBudget on TransientRetryBudget. Default 3;
	// raising this rarely helps because if a stage is structurally
	// stuck the stall plateau detector already short-circuits, and
	// if it is transient blip a single retry recovers the vast
	// majority of cases.
	TransientRetryBudgetCeil int `yaml:"transient_retry_budget_ceil"`

	// MaxStepsCeil is the absolute ceiling on the multi-topic-scaled
	// pipeline step budget after SubTopicPipelineStepsExtra adds
	// uplift. Default 100. Zero falls back to default.
	MaxStepsCeil int `yaml:"max_steps_ceil"`

	// ForceFinalizeAttempts caps the number of dispatch attempts the
	// force-finalize escape path makes when the previous attempt
	// errored with a transient LLM stream failure (unexpected EOF,
	// stream stalled, 429, etc.). Default 3 = 1 initial + 2 retries;
	// 1 disables retry. Hard-capped at 5 by the orchestrator setter
	// to keep the user-visible pause bounded. Zero falls back to
	// DefaultForceFinalizeAttempts.
	ForceFinalizeAttempts int `yaml:"force_finalize_attempts"`

	// MaxUpstreamFallbacksPerRun (Block 3 architecture overhaul
	// 2026-05-02) caps the number of selective upstream fallbacks
	// (BackToExtract / BackToExplore) any single Run may consume.
	// Reaching the cap forces the next fallback to FailLoud.
	// Default 2 — enough for two re-investigation passes, tight
	// enough to bound wall-clock. 0 disables the cap (not
	// recommended); negative values fall back to the default.
	MaxUpstreamFallbacksPerRun int `yaml:"max_upstream_fallbacks_per_run"`

	// BaselineCaptureEnabled toggles the pre-apply test snapshot
	// that feeds CritNoRegression. When true, the apply stage hook runs
	// run_tests once BEFORE the coder dispatches (against the fresh
	// worktree which is still byte-identical to main repo HEAD),
	// stores the result as Mutable.BaselineReport, and the
	// post-apply verify phase populates Mutable.ChangeReport as
	// usual — evalNoRegression then diffs the two. Default false
	// because baseline runs double the test-suite wall time; only
	// enable when the repo has test regressions to worry about.
	// Cache hits via BaselineCacheMax bypass the wall-time concern
	// (cached baselines reuse across Runs that touch the same HEAD
	// SHA), so the cache stays active regardless of this flag.
	BaselineCaptureEnabled bool `yaml:"baseline_capture_enabled"`

	// BaselineCacheMax sets the maximum number of cached baseline
	// reports stored under <runtimeAnchor>/baselines/<sha8>.json.
	// Each entry is keyed by the main repo's HEAD SHA at apply-stage
	// entry; reused across Runs that observe the same SHA so the
	// test suite does not re-run for an unchanged repo. Default 16
	// (set in cmd/root.go). 0 disables caching entirely; the
	// Capture-Enabled flag above then controls every Run from
	// scratch as before.
	BaselineCacheMax int `yaml:"baseline_cache_max"`

	// WriteMaxSeconds is the wall-clock ceiling for write-mode Runs
	// (plan / apply / verify). The orchestrator starts a deadline
	// timer at write-mode entry and cancels the in-flight Run when
	// the timer fires; LastError surfaces a "write mode wall-time
	// exceeded" message. Read-mode Runs are unaffected. Default 600
	// (set in cmd/root.go); 0 disables the cap (legacy behaviour).
	// Hard-capped at 1800 inside the orchestrator setter.
	WriteMaxSeconds int `yaml:"write_max_seconds"`

	// PlanCriticEnabled gates the optional pre-apply plan review LLM
	// call. Default false; when true and a plan_critic adapter is
	// configured, the orchestrator dispatches a single critique after
	// planner emit + before apply.
	PlanCriticEnabled bool `yaml:"plan_critic_enabled"`

	// KeepWorktreeOnSuccess preserves the git worktree after a
	// successful ModeApply so the user can review the applied bytes
	// and cherry-pick to main manually. Failure paths always discard
	// regardless of this flag. Default false preserves historical
	// "Run ends, worktree gone" behaviour.
	KeepWorktreeOnSuccess bool `yaml:"keep_worktree_on_success"`

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
	// MaxPatchesPerRun is the global F3 IR-patch cap. Reserved for
	// the not-yet-wired G6/G7 hookup that will route real patch
	// requests through internal/analysis/patcher.Engine; default 4
	// is duplicated by patcher.New's zero-Budget fallback so user
	// yaml writes are inert today (no production caller of
	// patcher.New consumes this value yet).
	MaxPatchesPerRun int `yaml:"max_patches_per_run"`

	// MaxPatchesPerField is the per-IR-field patch cap. Same
	// G6/G7-pending wiring story as MaxPatchesPerRun above; default
	// 2 today comes from patcher.New's fallback rather than this
	// field. Wire when the patch engine acquires a real entry point.
	MaxPatchesPerField int `yaml:"max_patches_per_field"`

	// MinRetryYield is the threshold below which the yield check
	// kills a retry: after each window, the scheduler computes
	// Δforced_reads + Δpatches + Δevidence + Δscanned_set; when
	// the sum is less than this value, retry is denied. Default 1.
	// Setting to 0 disables the yield gate entirely.
	MinRetryYield int `yaml:"min_retry_yield"`

	// FailLoudEnabled, when true, prepends an "· pipeline
	// terminated with unresolved violations" warning to the final
	// answer when the yield kill fires or any budget is exhausted.
	// Default true — never silently hide a failure.
	FailLoudEnabled bool `yaml:"fail_loud_enabled"`
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
// defaults: 4/2/1/true.
func DefaultViolationBudgetSettings() ViolationBudgetSettings {
	return ViolationBudgetSettings{
		MaxPatchesPerRun:   4,
		MaxPatchesPerField: 2,
		MinRetryYield:      1,
		FailLoudEnabled:    true,
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

// BytesPerToken is the conservative byte-to-token estimate shared by
// every byte-budget resolver (fraction-form caps in config.ResolveByteBudget)
// AND the BaseAgent context-pressure watchdog in internal/agent. 4
// covers typical English prose (~4 chars/token for OpenAI BPE) AND
// most source code; over-estimates CJK-heavy content (1-2 bytes/token
// after UTF-8) which yields a LARGER effective byte budget than the
// model can absorb — acceptable because CJK-dominant deployments that
// hit the wall can tighten the fraction explicitly.
//
// This constant is a shared runtime-configuration estimate, NOT a
// tokenizer. Callers needing actual token counts must tokenize via
// the provider's own tokenizer endpoint.
const BytesPerToken = 4

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

	// Per-evaluator iteration caps for the streaming-truncation
	// recovery pattern. Each evaluator's ShouldStop runs BEFORE the
	// current iteration's tool calls execute; a flat hard cap therefore
	// discards a clean retry that lands exactly on the cap iter (e.g.
	// when the previous attempt's emit_X arguments were truncated by
	// the LLM streaming gateway). The two-stage cap pattern:
	//
	//   iter < SoftIterCap                     → continue
	//   SoftIterCap <= iter < HardIterCap      → continue ONLY when
	//       the LLM is currently calling that evaluator's structured
	//       emit tool (recovery window)
	//   iter >= HardIterCap                    → stop unconditionally
	//
	// HardIterCap MUST be > SoftIterCap, otherwise the recovery window
	// collapses to zero. ResolvedAgentSettings enforces this and falls
	// back to defaults when violated.
	//
	// All zero (the absent-YAML state) inherits the code defaults
	// from DefaultAgentSettings(). Common adjustment: bump SoftIterCap
	// when investigation prep regularly runs long, OR raise HardIterCap
	// when streaming truncation rates are elevated on a slow gateway.

	// PlannerSoftIterCap / PlannerHardIterCap — emit_change_plan loop.
	// Default 6 / 9.
	PlannerSoftIterCap int `yaml:"planner_soft_iter_cap"`
	PlannerHardIterCap int `yaml:"planner_hard_iter_cap"`

	// CoderSoftIterSlack / CoderHardIterRecovery — apply_patch loop.
	// Soft cap = len(plan.TargetPaths) + CoderSoftIterSlack; hard cap
	// = soft cap + CoderHardIterRecovery. Default slack=3, recovery=3.
	CoderSoftIterSlack    int `yaml:"coder_soft_iter_slack"`
	CoderHardIterRecovery int `yaml:"coder_hard_iter_recovery"`

	// VerifierSoftIterCap / VerifierHardIterCap — emit_test_results
	// loop. Default 5 / 8.
	VerifierSoftIterCap int `yaml:"verifier_soft_iter_cap"`
	VerifierHardIterCap int `yaml:"verifier_hard_iter_cap"`

	// ExtractorSoftIterCap / ExtractorHardIterCap — emit_answer_symbol
	// + emit_hypothesis_verdict loop. Default 3 / 5.
	ExtractorSoftIterCap int `yaml:"extractor_soft_iter_cap"`
	ExtractorHardIterCap int `yaml:"extractor_hard_iter_cap"`

	// SubTopicPrescanBudgetExtra is the number of extra prescan rounds
	// granted per 2 sub-topics when RequestModel.SubTopics > 1.
	// Default 1. The adjusted prescan budget is capped at base + 2.
	SubTopicPrescanBudgetExtra int `yaml:"subtopic_prescan_budget_extra"`

	// SubTopicExplorerBudgetExtra is the number of extra explorer
	// iterations granted per sub-topic. Default 3. The adjusted
	// MaxIterations is capped at 35.
	SubTopicExplorerBudgetExtra int `yaml:"subtopic_explorer_budget_extra"`

	// SubTopicPlannerBudgetExtra is the number of extra planner
	// soft-cap iterations granted per sub-topic when
	// RequestModel.SubTopics > 1, applied on top of PlannerSoftIterCap.
	// Default 3 — a typical sub-topic is "one new file area to read",
	// and the planner skill's Workflow step 1 ("read and understand")
	// needs ~3 iters per area for a real feature-add task. The
	// orchestrator's per-dispatch scaling block consumes this and
	// writes the adjusted value to AgentContext.MaxIterOverride; the
	// planner reads it in BuildInitialInstruction. Adjusted soft cap
	// is hard-capped at 20 because the planner is single-emit and
	// any legitimate completion path stays well under that.
	SubTopicPlannerBudgetExtra int `yaml:"subtopic_planner_budget_extra"`

	// PlannerComplexityBudgetExtra is the per-complexity-level
	// uplift to the planner soft cap, multiplied by the analyzer's
	// classification: ComplexitySimple = 0×, ComplexityModerate = 1×,
	// ComplexityComplex = 2×. Default 2 → Moderate gets +2, Complex
	// gets +4. Stacks with SubTopicPlannerBudgetExtra so a
	// "complex + 3 sub-topics" task gets base + 9 + 4 iterations
	// before the soft cap. This decouples the "how much surface
	// area" signal (sub-topics) from the "how subtle the change"
	// signal (complexity); both are real workload drivers and the
	// analyzer emits both independently.
	PlannerComplexityBudgetExtra int `yaml:"planner_complexity_budget_extra"`

	// SubTopicPipelineStepsExtra is the number of extra pipeline steps
	// granted per sub-topic. Default 5. The adjusted step budget is
	// capped at 100.
	SubTopicPipelineStepsExtra int `yaml:"subtopic_pipeline_steps_extra"`

	// SubTopicRetryBudgetExtra is the number of extra retry budget
	// granted per 2 sub-topics. Default 1. The adjusted retry budget
	// is capped at MaxRetryBudgetCeil.
	SubTopicRetryBudgetExtra int `yaml:"subtopic_retry_budget_extra"`

	// SubTopicExtractorBudgetExtra is the number of extra extractor
	// soft-cap iterations granted per sub-topic when
	// RequestModel.SubTopics > 1. Default 1. Multi-topic explanations
	// require one Key-Anchor row per sub-topic so a static
	// ExtractorSoftIterCap=3 starves at 4+ sub-topics; this knob lifts
	// the soft cap by N per sub-topic, capped at ExtractorScaledIterMax.
	SubTopicExtractorBudgetExtra int `yaml:"subtopic_extractor_budget_extra"`

	// ExtractorComplexityBudgetExtra is the per-complexity-level
	// uplift to the extractor soft cap. Mirrors the planner's
	// PlannerComplexityBudgetExtra mechanism: ComplexitySimple = 0×,
	// ComplexityModerate = 1×, ComplexityComplex = 2×. Default 1.
	ExtractorComplexityBudgetExtra int `yaml:"extractor_complexity_budget_extra"`

	// TargetPathsVerifierBudgetExtra is the number of extra verifier
	// soft-cap iterations granted per ChangePlan target path when
	// the orchestrator dispatches StageVerify. Default 1. Mirrors
	// CoderSoftIterSlack but for the runner-iteration cost on
	// multi-language monorepo plans.
	TargetPathsVerifierBudgetExtra int `yaml:"target_paths_verifier_budget_extra"`

	// PrescanRoundsCeil is the absolute ceiling on analyzer pre-scan
	// rounds after SubTopicPrescanBudgetExtra adds uplift. Default 4.
	// Pre-scan is repo_map / grep / list_files only — beyond 4 rounds
	// the LLM is almost certainly drifting rather than discovering.
	PrescanRoundsCeil int `yaml:"prescan_rounds_ceil"`

	// ExplorerScaledIterMax is the absolute ceiling on the explorer
	// outer-loop iteration count after SubTopicExplorerBudgetExtra
	// adds uplift. Default 35.
	ExplorerScaledIterMax int `yaml:"explorer_scaled_iter_max"`

	// PlannerScaledIterMax is the absolute ceiling on the planner
	// soft cap after SubTopicPlannerBudgetExtra + PlannerComplexity
	// BudgetExtra add uplift. Default 20. Planner is single-emit;
	// legitimate completion never needs more than this many ReAct
	// turns of preparation.
	PlannerScaledIterMax int `yaml:"planner_scaled_iter_max"`

	// ExtractorScaledIterMax is the absolute ceiling on the extractor
	// soft cap after SubTopicExtractorBudgetExtra +
	// ExtractorComplexityBudgetExtra add uplift. Default 8.
	ExtractorScaledIterMax int `yaml:"extractor_scaled_iter_max"`

	// VerifierScaledIterMax is the absolute ceiling on the verifier
	// soft cap after TargetPathsVerifierBudgetExtra adds uplift.
	// Default 12.
	VerifierScaledIterMax int `yaml:"verifier_scaled_iter_max"`

	// MaxRetryBudgetCeil is the absolute ceiling on the dynamic
	// analyzer retry budget after SubTopicRetryBudgetExtra adds uplift.
	// Default 5. Higher values almost always mean the LLM is failing
	// the same gate repeatedly; bounded retry contains LLM token cost.
	MaxRetryBudgetCeil int `yaml:"max_retry_budget_ceil"`

	// PerfTriagerIterCap is the hard outer-loop cap on the perf_triager
	// agent. Default 6. perf_triager is single-emit (emit_perf_trace
	// terminates the loop); the cap only matters when a misbehaving
	// LLM never calls the emit tool — in that case this prevents
	// unbounded token spend.
	PerfTriagerIterCap int `yaml:"perf_triager_iter_cap"`

	// LogTriagerIterCap is the hard outer-loop cap on the log_triager
	// agent. Default 6. Same single-emit semantics as PerfTriagerIterCap.
	LogTriagerIterCap int `yaml:"log_triager_iter_cap"`

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

	// Context-pressure watchdog thresholds (BaseAgent per-iteration
	// prompt-byte tracker). The watchdog only activates when the
	// active adapter reports a positive context_window (either
	// declared in providers.yaml or the 128 000 fallback); deployments
	// on pure-unknown adapters skip pressure tracking altogether.
	//
	// SoftRatio = log a warning when prompt bytes / (window × 4) ≥
	// this threshold — the model is approaching its window and
	// retries are about to truncate tool history aggressively.
	//
	// HardRatio = force the current ReAct dispatch to stop next
	// iteration AND inject a "context pressure" directive that asks
	// the LLM to wrap up via emit_investigation_complete (or the
	// stage-appropriate terminal tool). Callers can still finish
	// the current in-flight call before the stop takes effect.
	//
	// Both zero (the absent-YAML state) inherits the code defaults
	// below. Set Soft=0 explicitly to disable just the soft warning;
	// Hard=0 disables the force-stop. Set both to 0 to fully disable
	// the watchdog on a specific deployment.
	ContextPressureSoftRatio float64 `yaml:"context_pressure_soft_ratio"`
	ContextPressureHardRatio float64 `yaml:"context_pressure_hard_ratio"`
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
		PlannerSoftIterCap:            6,
		PlannerHardIterCap:            9,
		CoderSoftIterSlack:            3,
		CoderHardIterRecovery:         3,
		VerifierSoftIterCap:           5,
		VerifierHardIterCap:           8,
		ExtractorSoftIterCap:          3,
		ExtractorHardIterCap:          5,
		SubTopicPrescanBudgetExtra:    1,
		SubTopicExplorerBudgetExtra:   3,
		SubTopicPlannerBudgetExtra:    3,
		PlannerComplexityBudgetExtra:  2,
		SubTopicPipelineStepsExtra:    5,
		SubTopicRetryBudgetExtra:      1,

		SubTopicExtractorBudgetExtra:   1,
		ExtractorComplexityBudgetExtra: 1,
		TargetPathsVerifierBudgetExtra: 1,

		PrescanRoundsCeil:      4,
		ExplorerScaledIterMax:  35,
		PlannerScaledIterMax:   20,
		ExtractorScaledIterMax: 8,
		VerifierScaledIterMax:  12,
		MaxRetryBudgetCeil:     5,

		PerfTriagerIterCap: 6,
		LogTriagerIterCap:  6,

		InvestigationCompletePolicy: ICPolicySoft,
		PriorConvPolicy:             PriorConvPolicyAnalyzer,
		// Context-pressure defaults: warn at 70 % of the estimated
		// byte capacity, force-stop at 90 %. These are empirical —
		// the watchdog uses a 4 B/token conservative multiplier so
		// actual model behaviour stays comfortable at 90 % absolute
		// (English-heavy text often tokenises at ~4 B/tok; CJK-heavy
		// at ~2 B/tok runs out of headroom slightly earlier, which
		// is why Hard kicks in before 100 %).
		ContextPressureSoftRatio: 0.7,
		ContextPressureHardRatio: 0.9,
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
	// Per-evaluator iteration caps. Zero → default. Recovery-window
	// invariant: hard > soft; violations fall back to defaults rather
	// than erroring (mis-tuned yaml shouldn't crash startup).
	if s.PlannerSoftIterCap <= 0 {
		s.PlannerSoftIterCap = d.PlannerSoftIterCap
	}
	if s.PlannerHardIterCap <= 0 {
		s.PlannerHardIterCap = d.PlannerHardIterCap
	}
	if s.PlannerHardIterCap <= s.PlannerSoftIterCap {
		s.PlannerSoftIterCap = d.PlannerSoftIterCap
		s.PlannerHardIterCap = d.PlannerHardIterCap
	}
	if s.CoderSoftIterSlack <= 0 {
		s.CoderSoftIterSlack = d.CoderSoftIterSlack
	}
	if s.CoderHardIterRecovery <= 0 {
		s.CoderHardIterRecovery = d.CoderHardIterRecovery
	}
	if s.VerifierSoftIterCap <= 0 {
		s.VerifierSoftIterCap = d.VerifierSoftIterCap
	}
	if s.VerifierHardIterCap <= 0 {
		s.VerifierHardIterCap = d.VerifierHardIterCap
	}
	if s.VerifierHardIterCap <= s.VerifierSoftIterCap {
		s.VerifierSoftIterCap = d.VerifierSoftIterCap
		s.VerifierHardIterCap = d.VerifierHardIterCap
	}
	if s.ExtractorSoftIterCap <= 0 {
		s.ExtractorSoftIterCap = d.ExtractorSoftIterCap
	}
	if s.ExtractorHardIterCap <= 0 {
		s.ExtractorHardIterCap = d.ExtractorHardIterCap
	}
	if s.ExtractorHardIterCap <= s.ExtractorSoftIterCap {
		s.ExtractorSoftIterCap = d.ExtractorSoftIterCap
		s.ExtractorHardIterCap = d.ExtractorHardIterCap
	}
	if s.SubTopicPrescanBudgetExtra == 0 {
		s.SubTopicPrescanBudgetExtra = d.SubTopicPrescanBudgetExtra
	}
	if s.SubTopicExplorerBudgetExtra == 0 {
		s.SubTopicExplorerBudgetExtra = d.SubTopicExplorerBudgetExtra
	}
	if s.SubTopicPlannerBudgetExtra == 0 {
		s.SubTopicPlannerBudgetExtra = d.SubTopicPlannerBudgetExtra
	}
	if s.PlannerComplexityBudgetExtra == 0 {
		s.PlannerComplexityBudgetExtra = d.PlannerComplexityBudgetExtra
	}
	if s.SubTopicPipelineStepsExtra == 0 {
		s.SubTopicPipelineStepsExtra = d.SubTopicPipelineStepsExtra
	}
	if s.SubTopicRetryBudgetExtra == 0 {
		s.SubTopicRetryBudgetExtra = d.SubTopicRetryBudgetExtra
	}
	if s.SubTopicExtractorBudgetExtra == 0 {
		s.SubTopicExtractorBudgetExtra = d.SubTopicExtractorBudgetExtra
	}
	if s.ExtractorComplexityBudgetExtra == 0 {
		s.ExtractorComplexityBudgetExtra = d.ExtractorComplexityBudgetExtra
	}
	if s.TargetPathsVerifierBudgetExtra == 0 {
		s.TargetPathsVerifierBudgetExtra = d.TargetPathsVerifierBudgetExtra
	}
	if s.PrescanRoundsCeil <= 0 {
		s.PrescanRoundsCeil = d.PrescanRoundsCeil
	}
	if s.ExplorerScaledIterMax <= 0 {
		s.ExplorerScaledIterMax = d.ExplorerScaledIterMax
	}
	if s.PlannerScaledIterMax <= 0 {
		s.PlannerScaledIterMax = d.PlannerScaledIterMax
	}
	if s.ExtractorScaledIterMax <= 0 {
		s.ExtractorScaledIterMax = d.ExtractorScaledIterMax
	}
	if s.VerifierScaledIterMax <= 0 {
		s.VerifierScaledIterMax = d.VerifierScaledIterMax
	}
	if s.MaxRetryBudgetCeil <= 0 {
		s.MaxRetryBudgetCeil = d.MaxRetryBudgetCeil
	}
	if s.PerfTriagerIterCap <= 0 {
		s.PerfTriagerIterCap = d.PerfTriagerIterCap
	}
	if s.LogTriagerIterCap <= 0 {
		s.LogTriagerIterCap = d.LogTriagerIterCap
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
	// Context-pressure ratios use zero-as-sentinel for "inherit code
	// default"; a caller that wants to fully disable one side sets
	// a negative explicitly. ResolvedAgentSettings keeps zero → default.
	if s.ContextPressureSoftRatio == 0 {
		s.ContextPressureSoftRatio = d.ContextPressureSoftRatio
	}
	if s.ContextPressureHardRatio == 0 {
		s.ContextPressureHardRatio = d.ContextPressureHardRatio
	}
	return s
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

// DefaultForceFinalizeAttempts is the fallback for
// PipelineSettings.ForceFinalizeAttempts when zero. Sized to ride
// through the typical single-connection blip (one drop is by far
// the most common pattern in production traces) while keeping the
// worst-case pause bounded to ~3.5s of backoff.
const DefaultForceFinalizeAttempts = 3

// MaxForceFinalizeAttempts is the absolute ceiling enforced by the
// orchestrator's SetForceFinalizeAttempts setter on operator-
// supplied values. Higher counts almost always mean misconfiguration
// that would burn LLM tokens on a genuinely-down upstream rather
// than recovering from transient blips.
const MaxForceFinalizeAttempts = 5

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
	// stored to disk. Larger turns are tail-truncated. Default 16384
	// (16 KB) — REPL paste-folding already keeps display-form requests
	// compact, so this is a defensive tail cap for scripted input and
	// rare long glamour-rendered responses.
	MaxTurnBodyBytes int `yaml:"max_turn_body_bytes"`

	// MaxBuildContextMatches: maximum number of compacted index entries
	// to inline in BuildContext. Default 3.
	MaxBuildContextMatches int `yaml:"max_build_context_matches"`

	// EntityMinRunes: a recorded entity's surface form must have at
	// least this many runes to participate in scoreIndex matching.
	// Below this, the substring check would over-fire on noise tokens
	// like "id", "x", "go". Default 3 — 3 runes covers typical short
	// symbols (e.g. "fnB", "rpc") without admitting noise.
	EntityMinRunes int `yaml:"entity_min_runes"`

	// SessionTieBreakerBonus: extra score added to a candidate
	// IndexEntry whose SessionID matches the current Run when the
	// entry already has positive relevance. Pure session membership
	// is intentionally NOT enough to surface an irrelevant entry —
	// this only breaks ties between equally-relevant cross-session
	// matches. Default 1.
	SessionTieBreakerBonus int `yaml:"session_tie_breaker_bonus"`

	// Policy holds per-Kind retrieval tuning. nil means "use the
	// hard-coded defaults" — typical operators leave it that way.
	// Operators with unusual usage patterns (chat-heavy, deep-pipe,
	// etc.) can override per Kind without touching the struct's
	// other fields.
	Policy *MemoryRetrievalPolicy `yaml:"policy,omitempty"`

	// SearchMaxLimit is the absolute hard cap on Store.Search's
	// `limit` opt — values above this are clamped before retrieval
	// runs so a misbehaving caller cannot exhaust BuildContext's
	// byte budget by asking for too many matches. Default 20.
	SearchMaxLimit int `yaml:"search_max_limit"`

	// ListMaxLimit is the absolute hard cap on Store.List's `limit`
	// opt (browse-mode listing). Default 30 — slightly larger than
	// SearchMaxLimit because browse mode does not pay the same
	// per-entry token cost as relevance-ranked retrieval.
	ListMaxLimit int `yaml:"list_max_limit"`
}

// MemoryRetrievalPolicy bundles the per-Kind retrieval policies
// BuildContext applies. Each pointer is independent: nil → use
// the per-Kind hard-coded default; non-nil → override that Kind only.
// New Kinds added later add a new pointer field here without
// breaking existing yaml.
type MemoryRetrievalPolicy struct {
	// Chitchat: KindChitchat retrieval. Defaults pin 3 same-session
	// recent turns at up to 1200 chars, 3 compacted matches, entities
	// weighted 2x keywords, 1-hop Refs chain.
	Chitchat *MemoryKindPolicy `yaml:"chitchat,omitempty"`

	// Shell: KindShell retrieval. Defaults same as Chitchat (the
	// natural follow-up to `!cmd` is "explain that output", which
	// has the same recent-thread bias as chitchat).
	Shell *MemoryKindPolicy `yaml:"shell,omitempty"`

	// Pipeline: KindPipeline retrieval. Defaults pin 2 recent turns
	// (no body inline — pipeline answers anchor on the Index entries),
	// 5 compacted matches, entities weighted 3x, 2-hop Refs chain.
	Pipeline *MemoryKindPolicy `yaml:"pipeline,omitempty"`

	// Plan: KindPlan retrieval. Defaults to the legacy fallback
	// today (no per-Kind tuning); exposed here so operators with a
	// plan-heavy workflow can dial it up without code changes.
	Plan *MemoryKindPolicy `yaml:"plan,omitempty"`

	// Default: legacy fallback for empty Kind / unknown Kind.
	// Defaults: 0 pin / 0 body / -1 cap (unbounded; the byte budget
	// elsewhere caps the actual injection) / 2x entity / 0 refs.
	Default *MemoryKindPolicy `yaml:"default,omitempty"`
}

// MemoryKindPolicy is one Kind's retrieval tuning. All five fields
// echo the kindPolicy struct in internal/memory; a yaml override
// replaces the corresponding hard-coded default verbatim.
type MemoryKindPolicy struct {
	SessionPinCount   int `yaml:"session_pin_count"`
	RecentBodyChars   int `yaml:"recent_body_chars"`
	CompactedMatchCap int `yaml:"compacted_match_cap"`
	EntityScoreMul    int `yaml:"entity_score_mul"`
	RefsChainDepth    int `yaml:"refs_chain_depth"`
}

// DefaultMemoryKindPolicies returns the per-Kind defaults the memory
// package uses when MemorySettings.Policy is nil or a sub-policy is
// nil. Mirrors internal/memory.policyFor — single source of truth so
// adding a Kind only touches one switch.
func DefaultMemoryKindPolicies() map[string]MemoryKindPolicy {
	chitchat := MemoryKindPolicy{
		SessionPinCount:   3,
		RecentBodyChars:   1200,
		CompactedMatchCap: 3,
		EntityScoreMul:    2,
		RefsChainDepth:    1,
	}
	pipeline := MemoryKindPolicy{
		SessionPinCount:   2,
		RecentBodyChars:   0,
		CompactedMatchCap: 5,
		EntityScoreMul:    3,
		RefsChainDepth:    2,
	}
	def := MemoryKindPolicy{
		SessionPinCount:   0,
		RecentBodyChars:   0,
		CompactedMatchCap: -1,
		EntityScoreMul:    2,
		RefsChainDepth:    0,
	}
	return map[string]MemoryKindPolicy{
		"chitchat": chitchat,
		"shell":    chitchat,
		"pipeline": pipeline,
		"plan":     def,
		"default":  def,
	}
}

// DefaultMemorySettings returns the code defaults for memory store limits.
func DefaultMemorySettings() MemorySettings {
	return MemorySettings{
		MaxRecentTurns:         6,
		MaxRecentBytes:         20 * 1024,
		MaxTurnBodyBytes:       16 * 1024,
		MaxBuildContextMatches: 3,
		EntityMinRunes:         3,
		SessionTieBreakerBonus: 1,
		SearchMaxLimit:         20,
		ListMaxLimit:           30,
		// Policy stays nil here. ResolvedMemorySettings does NOT
		// hydrate it — internal/memory falls back to
		// DefaultMemoryKindPolicies() per-Kind on a missing override.
		// Keeping Policy nil means an operator who never sets it
		// pays no marshal cost and YAML stays clean.
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
	if s.EntityMinRunes == 0 {
		s.EntityMinRunes = d.EntityMinRunes
	}
	if s.SessionTieBreakerBonus == 0 {
		s.SessionTieBreakerBonus = d.SessionTieBreakerBonus
	}
	if s.SearchMaxLimit <= 0 {
		s.SearchMaxLimit = d.SearchMaxLimit
	}
	if s.ListMaxLimit <= 0 {
		s.ListMaxLimit = d.ListMaxLimit
	}
	// s.Policy stays as-is: nil or partially populated. The memory
	// package's policyFor lookup handles the "field-by-field merge
	// against DefaultMemoryKindPolicies" semantics.
	return s
}

// ChitchatSettings carries the tunable numeric knobs for the /chat
// tool-use loop. Zero values mean "use code default" — see
// DefaultChitchatSettings.
type ChitchatSettings struct {
	// RecallDefaultLimit is the default `limit` arg the responder
	// applies when the LLM calls recall_memory without supplying
	// one. Tracks the explorer-side default to keep behaviour
	// predictable across paths. Default 5.
	RecallDefaultLimit int `yaml:"recall_default_limit"`

	// RecallMaxLimit is the hard ceiling on the `limit` arg —
	// values above this are clamped before Search runs so a
	// misbehaving LLM cannot drown the second-round Chat call's
	// token budget. Smaller than the explorer's 20 because chitchat
	// is one-shot conversation, not iterative investigation.
	// Default 10.
	RecallMaxLimit int `yaml:"recall_max_limit"`

	// ListDefaultLimit is the default `limit` arg the chitchat
	// list_memory tool applies when the LLM calls it without
	// supplying one. Browse-mode listing is broader than relevance
	// search; default 10 is the historical literal.
	ListDefaultLimit int `yaml:"list_default_limit"`

	// ListMaxLimit is the hard ceiling on the `limit` arg of
	// the chitchat list_memory tool. Default 30 — slightly larger
	// than RecallMaxLimit because list mode bypasses the relevance
	// score and only carries Topic + Summary in the rendered tool
	// result, so the per-entry token cost is lower.
	ListMaxLimit int `yaml:"list_max_limit"`
}

// DefaultChitchatSettings returns the code defaults for the chitchat
// tool-use numeric knobs.
func DefaultChitchatSettings() ChitchatSettings {
	return ChitchatSettings{
		RecallDefaultLimit: 5,
		RecallMaxLimit:     10,
		ListDefaultLimit:   10,
		ListMaxLimit:       30,
	}
}

// ResolvedChitchatSettings hydrates zero fields from defaults, same
// pattern as ResolvedMemorySettings.
func ResolvedChitchatSettings(s ChitchatSettings) ChitchatSettings {
	d := DefaultChitchatSettings()
	if s.RecallDefaultLimit == 0 {
		s.RecallDefaultLimit = d.RecallDefaultLimit
	}
	if s.RecallMaxLimit == 0 {
		s.RecallMaxLimit = d.RecallMaxLimit
	}
	if s.ListDefaultLimit == 0 {
		s.ListDefaultLimit = d.ListDefaultLimit
	}
	if s.ListMaxLimit == 0 {
		s.ListMaxLimit = d.ListMaxLimit
	}
	return s
}
