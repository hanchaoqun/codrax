package cmd

import (
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/reasoninggraph"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repomapindex "github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This file is the part of start-up that decides what a pipeline run asks a
// model: the settings a settings file (or its absence) resolves to, the skill,
// tool and agent registries, and the orchestrator with the knobs those
// settings drive. initApp builds the pipeline through it, and so does anything
// that reproduces a recorded run, so the two cannot drift apart unnoticed.
//
// What stays with the caller is what only the caller can know: where the
// provider credentials live (the resolver that hands each agent its adapter,
// and the per-provider parameter compatibility table), where output goes (the
// emitter), and the directories of this process.

// resolvePipelineSettings turns the runtime settings file into the pipeline's
// settings. rs is nil when no settings file was found; the result is then the
// code defaults. flagChanged reports whether a command-line flag was passed.
// Besides returning the settings it installs the process-wide knobs the same
// file drives, exactly once per start-up.
func resolvePipelineSettings(rs *config.RuntimeSettings, flagChanged func(name string) bool, defaultLLM llm.Adapter) types.PipelineSettings {
	// Build pipeline settings from codrax.yaml overrides.
	// Defaults that should NOT be the type's zero value are seeded here
	// so an empty codrax.yaml gets meaningful behaviour:
	//   - WriteRetryBudget=3: aligned with Reflexion / Self-Refine /
	//     AutoCodeRover empirical sweet spot (literature consensus is
	//     3-5 retries; past 5 marginal lift is negligible). Zero is
	//     fail-loud, opt-in by setting yaml `pipeline_write_retry_budget: 0`.
	pipelineSettings := types.PipelineSettings{
		// Defaults that should NOT be the type's zero value (and aren't
		// owned by the runtime config layer's pointer-typed override
		// path) are seeded here so an empty codrax.yaml gets meaningful
		// behaviour:
		//   - WriteRetryBudget=3: literature consensus (Reflexion /
		//     Self-Refine / AutoCodeRover all use 3-5).
		//   - MaxRetriesPerStage=2: Batch E surfaced bowling failing
		//     because the analyzer occasionally ignores the must-emit
		//     hint and a single attempt is the whole pipeline. 2
		//     attempts catches the LLM compliance flake without
		//     burning tokens on truly broken inputs.
		WriteRetryBudget: 3,
		// MaxRetriesPerStage=3: a 2-stage coherence-gate retry chain
		// (R1.2 → R1.5, or any other gated rule pair) needs 3 attempts
		// to converge cleanly — attempt 1 trips gate A, attempt 2 fixes
		// A but trips gate B, attempt 3 fixes both. Pre-2026-05-02-3
		// this was 2, leaving the pipeline 1-attempt short on questions
		// that are structurally complex enough to fail two distinct
		// gates in sequence. Empirical: bx246b3qn audit run on the
		// "对比 read 模式 explorer 和 write 模式 verify 的 retry 机制"
		// question exhausted at 2 attempts when R1.2 fired then R1.5
		// fired. base=3 catches that case while costing 0 tokens on the
		// happy path (the loop exits on first success).
		// EstimateSubTopicCount-based dynamic uplift still applies on
		// top, so multi-topic questions get up to ceiling=5.
		MaxRetriesPerStage: 3,
		// TransientRetryBudget=3: three scheduler-level retries ride
		// through bursty EOF / stream-stall / dial failures without
		// consuming semantic retry or pipeline-step budget. Structural
		// hard-stop remains guarded by the precise repeated non-empty
		// stream-stall tool signature; ambiguous no-tool and pure
		// transport failures stay budget-governed. Distinct counter
		// from WriteRetryBudget so transient blips never starve
		// verify→plan SC retry. Cap 3 in TransientRetryBudgetCeil.
		TransientRetryBudget: 3,
		// Block 3 (architecture overhaul 2026-05-02) — selective
		// upstream fallback cap. 2 = give the LLM two
		// re-investigation passes per Run; further attempts force
		// FailLoud so wall-clock stays bounded.
		MaxUpstreamFallbacksPerRun: 2,
		// MaxParallelism=2: shared cap for orchestrator-owned fan-out
		// surfaces (reviewers, sub-agents, multi-sub_topic explorer).
		// Operators can set pipeline_max_parallelism: 1 for serial
		// debug/rate-limit mode or 0 for uncapped fan-out.
		MaxParallelism: types.DefaultPipelineMaxParallelism,
		// Write mode is controller-first. The historical
		// write_workflow_engine yaml key is still parsed for
		// compatibility but no longer selects a public legacy path.
		WriteWorkflowEngine: types.WriteWorkflowEngineController,
		// B4-F1 (2026-05-04) — wire DefaultViolationBudgetSettings
		// into the production seed so the documented defaults
		// (FailLoudEnabled=true, UserVisibleViolationCaveat=false)
		// actually take effect. Pre-B4-F1 the default-table was
		// referenced only by tests; production used the zero value
		// so FailLoudEnabled was silently false. The
		// UserVisibleViolationCaveat=false default keeps the
		// per-violation digest off the user panel by default
		// (operators flip it via codrax.yaml ::
		// violation_budget.user_visible_caveat).
		ViolationBudget: types.DefaultViolationBudgetSettings(),
	}
	// Default-LLM context window for fraction-form byte budget
	// resolution. The fallback path in MaxContextTokens guarantees a
	// positive value (128 000 when providers.yaml didn't declare),
	// so downstream resolvers always have a non-zero divisor.
	ctxWindow := 0
	if defaultLLM != nil {
		ctxWindow = defaultLLM.MaxContextTokens()
	}
	if rs != nil {
		// Resolve blob_max_inline from fraction form → absolute form →
		// code default. Fraction wins when both are set because it
		// tracks the model's actual window; the absolute form is kept
		// for deployments that know exactly how many bytes they want.
		blobMax := config.ResolveByteBudget(rs.BlobMaxInlineFraction, rs.BlobMaxInlineBytes, 0, ctxWindow)
		blobHead, blobTail := 0, 0
		if rs.BlobPreviewHeadBytes != nil {
			blobHead = *rs.BlobPreviewHeadBytes
		}
		if rs.BlobPreviewTailBytes != nil {
			blobTail = *rs.BlobPreviewTailBytes
		}
		tool.SetBlobLimits(blobMax, blobHead, blobTail)
		if blobMax > 0 && rs.BlobMaxInlineFraction != nil && *rs.BlobMaxInlineFraction > 0 {
			logging.Info("[config] blob_max_inline resolved: %d bytes (fraction=%.3f × context_window=%d × %d B/token)",
				blobMax, *rs.BlobMaxInlineFraction, ctxWindow, config.BytesPerToken)
		}

		if rs.ReadFileSmallLimitThreshold != nil {
			tool.SetReadFileSmallLimitThreshold(*rs.ReadFileSmallLimitThreshold)
		}

		// Data-lane per-file read bound (data_task_max_file_bytes).
		// One-shot injection mirroring SetBlobLimits above; absent or
		// non-positive keeps the 32 MiB code default in internal/dataquery.
		if rs.DataTaskMaxFileBytes != nil && *rs.DataTaskMaxFileBytes > 0 {
			dataquery.SetDefaultMaxFileBytes(int64(*rs.DataTaskMaxFileBytes))
		}

		// Central width governor (tool_width_* prefix group). One-shot
		// injection mirroring SetBlobLimits above: absent/non-positive
		// fields keep the code defaults in internal/tool/width;
		// cross-field sanity clamps log WARN inside SetWidthGovernor.
		{
			intOr0 := func(p *int) int {
				if p == nil {
					return 0
				}
				return *p
			}
			tool.SetWidthGovernor(width.Overrides{
				GrepLineEntryThreshold:      intOr0(rs.ToolWidthGrepLineEntryThreshold),
				GrepFileEntryThreshold:      intOr0(rs.ToolWidthGrepFileEntryThreshold),
				GrepByteThreshold:           intOr0(rs.ToolWidthGrepByteThreshold),
				GrepProductionCap:           intOr0(rs.ToolWidthGrepProductionCap),
				GrepDirScanMaxFileBytes:     int64(intOr0(rs.ToolWidthGrepDirscanMaxFileBytes)),
				PathDiscoveryCandidateLimit: intOr0(rs.ToolWidthPathDiscoveryCandidateLimit),
				ReadFilePageWindowMax:       intOr0(rs.ToolWidthReadFilePageWindowMax),
				TraceQueryEventSearchLimit:  intOr0(rs.ToolWidthTraceQueryEventSearchLimit),
				RepoMapNavigationMaxRows:    intOr0(rs.ToolWidthRepoMapNavigationMaxRows),
			})
		}

		// SUPP-CORE trace supplement knobs (trace_supplement_* prefix
		// group). One-shot injection mirroring SetWidthGovernor above:
		// enabled defaults ON (explicit false = kill switch); absent /
		// non-positive budgets keep the code defaults (2048 MiB cold /
		// 20000 ms between-view deadline / 120 s window span).
		{
			enabled := true
			if rs.TraceSupplementEnabled != nil {
				enabled = *rs.TraceSupplementEnabled
			}
			var maxColdBytes int64
			if rs.TraceSupplementMaxColdMB != nil && *rs.TraceSupplementMaxColdMB > 0 {
				maxColdBytes = int64(*rs.TraceSupplementMaxColdMB) << 20
			}
			var maxDuration time.Duration
			if rs.TraceSupplementMaxDurationMS != nil && *rs.TraceSupplementMaxDurationMS > 0 {
				maxDuration = time.Duration(*rs.TraceSupplementMaxDurationMS) * time.Millisecond
			}
			var maxWindowSpanS float64
			if rs.TraceSupplementMaxWindowSpanSec != nil && *rs.TraceSupplementMaxWindowSpanSec > 0 {
				maxWindowSpanS = *rs.TraceSupplementMaxWindowSpanSec
			}
			tool.SetTraceQuerySupplementConfig(enabled, maxColdBytes, maxDuration, maxWindowSpanS)
		}

		// Per-shape Summary caps. Start from code defaults and
		// overlay any non-nil yaml fields; a partial override leaves
		// the other entries at their default.
		{
			scc := types.DefaultSummaryCapConfig()
			if rs.SummaryCapEnabled != nil {
				scc.Enabled = *rs.SummaryCapEnabled
			}
			if rs.SummaryCapExplanation != nil {
				scc.Explanation = *rs.SummaryCapExplanation
			}
			if rs.SummaryCapValue != nil {
				scc.Value = *rs.SummaryCapValue
			}
			if rs.SummaryCapConfigValue != nil {
				scc.ConfigValue = *rs.SummaryCapConfigValue
			}
			if rs.SummaryCapBoolean != nil {
				scc.Boolean = *rs.SummaryCapBoolean
			}
			if rs.SummaryCapStepListBase != nil {
				scc.StepListBase = *rs.SummaryCapStepListBase
			}
			if rs.SummaryCapStepListPerItem != nil {
				scc.StepListPerItem = *rs.SummaryCapStepListPerItem
			}
			if rs.SummaryCapStepListMax != nil {
				scc.StepListMax = *rs.SummaryCapStepListMax
			}
			if rs.SummaryCapSymbolsBase != nil {
				scc.SymbolsBase = *rs.SummaryCapSymbolsBase
			}
			if rs.SummaryCapSymbolsPerItem != nil {
				scc.SymbolsPerItem = *rs.SummaryCapSymbolsPerItem
			}
			if rs.SummaryCapSymbolsMax != nil {
				scc.SymbolsMax = *rs.SummaryCapSymbolsMax
			}
			if rs.SummaryCapDefault != nil {
				scc.Default = *rs.SummaryCapDefault
			}
			types.SetSummaryCapConfig(scc)
		}

		// Citation Quote preview ceiling. Non-positive values are
		// ignored by the setter so a malformed override leaves the
		// default intact.
		if rs.CitationQuoteMaxChars != nil {
			types.SetCitationMaxQuoteChars(*rs.CitationQuoteMaxChars)
		}

		// CGEC (Citation-Grounded Evidence Closure) tunables. Each
		// is a positive integer; SetCGECPolicy ignores zero/negative
		// values and keeps the current default, so a partial override
		// is safe.
		{
			forced, soft, hard := orchestrator.CGECPolicy()
			if rs.CGECForcedReadsPerRound != nil {
				forced = *rs.CGECForcedReadsPerRound
			}
			if rs.CGECStallThresholdSoft != nil {
				soft = *rs.CGECStallThresholdSoft
			}
			if rs.CGECStallThresholdHard != nil {
				hard = *rs.CGECStallThresholdHard
			}
			orchestrator.SetCGECPolicy(forced, soft, hard)
		}

		// emit_analysis runtime validation knobs. Start from the
		// code defaults and overlay any non-nil yaml fields so a
		// partial override leaves the other knobs alone.
		analysisLimits := tool.DefaultAnalysisLimits()
		if rs.AnalysisWarnBelowKeywords != nil {
			analysisLimits.WarnBelowKeywords = *rs.AnalysisWarnBelowKeywords
		}
		if rs.AnalysisRejectBelowKeywords != nil {
			analysisLimits.RejectBelowKeywords = *rs.AnalysisRejectBelowKeywords
		}
		if rs.AnalysisGenericEntityBlocklist != nil {
			// Non-nil but possibly empty slice — empty explicitly
			// disables the filter, honoring operator intent.
			analysisLimits.GenericEntityBlocklist = append(
				[]string(nil), rs.AnalysisGenericEntityBlocklist...)
		}
		if rs.AnalysisRejectMultipleEmit != nil {
			analysisLimits.RejectMultipleEmit = *rs.AnalysisRejectMultipleEmit
		}
		if rs.AnalysisMaxPrescanRounds != nil {
			analysisLimits.MaxPrescanRounds = *rs.AnalysisMaxPrescanRounds
		}
		if rs.AnalysisEmitOnlyCorrectionRetries != nil {
			analysisLimits.EmitOnlyCorrectionRetries = *rs.AnalysisEmitOnlyCorrectionRetries
		}
		if rs.AnalysisTerminalEmitOnlyTimeoutSeconds != nil {
			analysisLimits.TerminalEmitOnlyRequestTimeoutSeconds = *rs.AnalysisTerminalEmitOnlyTimeoutSeconds
		}
		// CLI override wins last (precedence: code default → yaml →
		// CLI). 0 means "not set" (cobra default), so we honor only
		// positive values to avoid a zero flag silently disabling the
		// pre-scan loop.
		if flagChanged("max-prescan-rounds") && flagMaxPrescanRounds > 0 {
			analysisLimits.MaxPrescanRounds = flagMaxPrescanRounds
		}
		if rs.AnalysisWarnBelowKeywordHitRatio != nil {
			analysisLimits.WarnBelowKeywordHitRatio = *rs.AnalysisWarnBelowKeywordHitRatio
		}
		if rs.AnalysisWarnBelowEntityHitRatio != nil {
			analysisLimits.WarnBelowEntityHitRatio = *rs.AnalysisWarnBelowEntityHitRatio
		}
		if rs.CGECPhase1UnreadTopK != nil {
			analysisLimits.Phase1UnreadTopK = *rs.CGECPhase1UnreadTopK
		}
		if rs.CGECPhase1UnreadMinUnread != nil {
			analysisLimits.Phase1UnreadMinUnread = *rs.CGECPhase1UnreadMinUnread
		}
		if rs.CGECMultiPathMinGroundedPerAnchor != nil {
			analysisLimits.MultiPathMinGroundedPerAnchor = *rs.CGECMultiPathMinGroundedPerAnchor
		}
		if rs.CGECMultiPathSymbolContextLines != nil {
			analysisLimits.MultiPathSymbolContextLines = *rs.CGECMultiPathSymbolContextLines
		}
		if rs.CGECMultiPathKeywordContextLines != nil {
			analysisLimits.MultiPathKeywordContextLines = *rs.CGECMultiPathKeywordContextLines
		}
		if rs.CGECMultiPathSmallFileThreshold != nil {
			analysisLimits.MultiPathSmallFileThreshold = *rs.CGECMultiPathSmallFileThreshold
		}
		if rs.CGECMultiPathMaxKeywordAnchorsPerFile != nil {
			analysisLimits.MultiPathMaxKeywordAnchorsPerFile = *rs.CGECMultiPathMaxKeywordAnchorsPerFile
		}
		if rs.CGECExternalArtifactDecodedFloor != nil {
			criterion.SetExternalArtifactFloor(*rs.CGECExternalArtifactDecodedFloor)
		}
		tool.SetAnalysisLimits(analysisLimits)

		// Evidence grounding policy. analysis_evidence_profile is the
		// operator-facing selector; legacy numeric fields remain
		// precise overrides and switch the runtime profile to custom.
		groundingPolicy := tool.DefaultGroundingPolicy()
		if rs.AnalysisEvidenceProfile != nil {
			if p, ok := tool.GroundingPolicyForProfile(*rs.AnalysisEvidenceProfile); ok {
				groundingPolicy = p
			} else {
				logging.Warning("[config] unknown analysis_evidence_profile=%q; using %s", *rs.AnalysisEvidenceProfile, groundingPolicy.Profile)
			}
		}
		if rs.EvidenceGroundingFloor != nil {
			groundingPolicy = groundingPolicy.WithGroundingFloor(*rs.EvidenceGroundingFloor)
		}
		if rs.EvidenceTier1Floor != nil {
			groundingPolicy = groundingPolicy.WithTier1Floor(*rs.EvidenceTier1Floor)
		}
		tool.SetGroundingPolicy(groundingPolicy)

		if rs.PipelineMaxRetriesPerStage != nil {
			pipelineSettings.MaxRetriesPerStage = *rs.PipelineMaxRetriesPerStage
		}
		if rs.PipelineMaxStageVisits != nil {
			pipelineSettings.MaxStageVisits = *rs.PipelineMaxStageVisits
		}
		if rs.PipelineMaxStepsCeil != nil {
			pipelineSettings.MaxStepsCeil = *rs.PipelineMaxStepsCeil
		}
		if rs.PipelineMaxParallelism != nil {
			pipelineSettings.MaxParallelism = *rs.PipelineMaxParallelism
		}
		// T4: verify→plan retry cap. Stored alongside the rest of the
		// pipelineSettings snapshot; orchestrator.SetWriteRetryBudget
		// applies it below where other orch getters are wired.
		if rs.PipelineWriteRetryBudget != nil {
			pipelineSettings.WriteRetryBudget = *rs.PipelineWriteRetryBudget
		}
		if rs.PipelineWriteRetryBudgetCeil != nil {
			pipelineSettings.WriteRetryBudgetCeil = *rs.PipelineWriteRetryBudgetCeil
		}
		if rs.PipelineMaxPhasesPerRun != nil {
			// Stage II phase cap. Pushes into the types package's
			// process-global slot read by emit_write_analysis when
			// truncating an over-eager LLM phase proposal.
			// types.SetMaxPhases clamps to MaxPhasesPerGroupCeil
			// internally; non-positive resets to default.
			types.SetMaxPhases(*rs.PipelineMaxPhasesPerRun)
		}
		// Stage 3 yaml knobs. Pinned into package vars so the
		// downstream FTX wiring block (which has flagRepo +
		// flagCacheDir but no rs) can read them.
		if rs.PipelineFailureTaxonomyEnabled != nil {
			failureTaxonomyEnabled = *rs.PipelineFailureTaxonomyEnabled
		}
		if rs.PipelineFailureTaxonomyMaxItems != nil {
			failureTaxonomyMaxItems = *rs.PipelineFailureTaxonomyMaxItems
		}
		if rs.PipelineFailureTaxonomyDecayDays != nil {
			failureTaxonomyDecayDays = *rs.PipelineFailureTaxonomyDecayDays
		}
		// B5-F2 / B5-F3 richness telemetry WARN gate.
		if rs.PipelineRichnessSofteningWarn != nil {
			richnessSofteningWarn = *rs.PipelineRichnessSofteningWarn
		}
		orchestrator.SetRichnessSofteningWarnEnabled(richnessSofteningWarn)
		// EVALFIX-2E (CLASS 5, 2026-07-30) degradation-disclosure footer
		// gate. Function indirection mirrors the richness WARN wiring
		// above. Default true; off restores pre-footer answer bytes but
		// never stops ledger recording or the WARN lines.
		degradationFooter := true
		if rs.PipelineDegradationFooter != nil {
			degradationFooter = *rs.PipelineDegradationFooter
		}
		agent.SetDegradationFooterEnabled(degradationFooter)
		// R2.2 (post_shape_residual_audit.md, 2026-05-04) finalizer-
		// local retry budget for the fallback picker downgrade.
		if rs.PipelineFinalizerLocalRetriesBeforeEscalate != nil {
			orchestrator.SetFinalizerLocalRetryBudget(*rs.PipelineFinalizerLocalRetriesBeforeEscalate)
		}
		// FRCAP (§29.12, 2026-07-10) — same-error-class retry cap and
		// cross-scope per-root attempt cap, both formerly hardcoded
		// finalize-loop bounds. nil keeps the shipped defaults (1 / 3)
		// byte-for-byte.
		if rs.PipelineSameErrorClassRetryCap != nil {
			orchestrator.SetSameErrorClassRetryCap(*rs.PipelineSameErrorClassRetryCap)
		}
		if rs.PipelineMaxRepairAttemptsPerRoot != nil {
			orchestrator.SetMaxRepairAttemptsPerRoot(*rs.PipelineMaxRepairAttemptsPerRoot)
		}
		// v3 B1 (2026-05-04) cluster-state closure stable-budget knob.
		if rs.PipelineClusterStableBudget != nil {
			orchestrator.SetClusterStableBudget(*rs.PipelineClusterStableBudget)
		}
		// B6-F4 finalizer retry-iter think_aloud dynamic disable.
		finalizerRetryNoThink := true
		if rs.PipelineFinalizerRetryNoThink != nil {
			finalizerRetryNoThink = *rs.PipelineFinalizerRetryNoThink
		}
		orchestrator.SetFinalizerRetryNoThinkEnabled(finalizerRetryNoThink)
		// CGEC frequency-to-violation bridge thresholds. -1 sentinel
		// preserves default behaviour when the yaml knob is absent;
		// the orchestrator setter treats negative values as "reset
		// to default", 0 as "explicit disable".
		demotionStorm := -1
		forcedReadStorm := -1
		if rs.PipelineDemotionStormThreshold != nil {
			demotionStorm = *rs.PipelineDemotionStormThreshold
		}
		if rs.PipelineForcedReadStormThreshold != nil {
			forcedReadStorm = *rs.PipelineForcedReadStormThreshold
		}
		orchestrator.SetCGECStormThresholds(demotionStorm, forcedReadStorm)
		// Read-mode Answer Taxonomy yaml knobs (commit 51 Gap 3).
		if rs.PipelineAnswerTaxonomyEnabled != nil {
			answerTaxonomyEnabled = *rs.PipelineAnswerTaxonomyEnabled
		}
		if rs.PipelineAnswerTaxonomyMaxItems != nil {
			answerTaxonomyMaxItems = *rs.PipelineAnswerTaxonomyMaxItems
		}
		if rs.PipelineAnswerTaxonomyDecayDays != nil {
			answerTaxonomyDecayDays = *rs.PipelineAnswerTaxonomyDecayDays
		}
		// Commit 53 P3: contract gate soft/strict classification.
		// Always apply so explicit empty yaml restores defaults.
		// Commit 62: extend the strict/soft list with self_contradiction
		// based on pipeline_self_consistency_rewrite_on_contradiction
		// (default true = strict, triggers retry/rewrite). Computed
		// here at startup so the runtime check in hasAnyStrictViolation
		// sees the correct membership for ViolSelfContradiction.
		if rs.PipelineSelfConsistencyReviewEnabled != nil {
			selfConsistencyEnabled = *rs.PipelineSelfConsistencyReviewEnabled
		}
		if rs.PipelineSelfConsistencyRewriteOnContradiction != nil {
			selfConsistencyRewrite = *rs.PipelineSelfConsistencyRewriteOnContradiction
		}
		if rs.PipelineSelfConsistencyMinConfidence != nil {
			selfConsistencyMinConfidence = *rs.PipelineSelfConsistencyMinConfidence
		}
		// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — second-
		// layer semantic-quality reviewer gate.
		if rs.PipelineSemanticQualityReviewEnabled != nil {
			semanticQualityEnabled = *rs.PipelineSemanticQualityReviewEnabled
		}
		// P4 (2026-05-10) — operator-tunable G5 reviewer floor.
		// Symmetric with PipelineSelfConsistencyMinConfidence above.
		if rs.PipelineSemanticQualityMinConfidence != nil {
			semanticQualityMinConfidence = *rs.PipelineSemanticQualityMinConfidence
		}
		if rs.PipelineStrictAnswerReviewEnabled != nil {
			strictAnswerReviewEnabled = *rs.PipelineStrictAnswerReviewEnabled
		}
		// P6 (2026-05-10) — operator-tunable finalize repair hard cap.
		// nil → orchestrator falls back to FinalizeRepairHardCapDefault (2).
		if rs.PipelineFinalizeRepairHardCap != nil {
			finalizeRepairHardCap = *rs.PipelineFinalizeRepairHardCap
		}
		// 2026-05-16 — deterministic exhaustive member-set review.
		// nil → types.ExhaustiveMemberSetDefaultThreshold (30).
		// 0  → deterministic path disabled.
		if rs.PipelineExhaustiveDeterministicReviewThreshold != nil {
			exhaustiveDeterministicReviewThreshold = *rs.PipelineExhaustiveDeterministicReviewThreshold
		}
		// 2026-05-16 — per-reviewer wall budget fraction.
		// nil → 0.4 default kept; 0 disables the deadline guard.
		if rs.PipelineReviewerWallBudgetFraction != nil {
			reviewerWallBudgetFraction = *rs.PipelineReviewerWallBudgetFraction
		}
		extraSoft := append([]string{}, rs.PipelineContractSoftKinds...)
		extraStrict := append([]string{}, rs.PipelineContractStrictKinds...)
		if strictAnswerReviewEnabled && selfConsistencyEnabled && selfConsistencyRewrite {
			extraStrict = append(extraStrict, "self_contradiction")
		} else {
			// reviewer off OR soft mode → ViolSelfContradiction is soft
			extraSoft = append(extraSoft, "self_contradiction")
		}
		// AuthorityCeiling axis: ViolAuthorityOverreach is STRICT —
		// stripping system-injected hedge sentinels triggers a
		// finalizer retry with hedge repair. The axis itself is
		// always-on (no yaml gate); SymbolLocatorProvider wires the
		// repomap-side adapter so the projection can run drift
		// detection without importing repomap directly.
		extraStrict = append(extraStrict, string(types.ViolAuthorityOverreach))
		orchestrator.SetSoftViolationKinds(extraSoft, extraStrict)
		// Block 3 (architecture overhaul 2026-05-02) — apply
		// per-kind FallbackTarget overrides from yaml. Defaults
		// in orchestrator.DefaultFallbackPolicy are tuned for the
		// reviewer-specific repair semantics; operators tweak via
		// pipeline_fallback_policy_overrides without code changes.
		if len(rs.PipelineFallbackPolicyOverrides) > 0 {
			orchestrator.SetFallbackPolicyOverrides(rs.PipelineFallbackPolicyOverrides)
		}
		authority.SetSymbolLocatorProvider(func(graph any) types.SymbolLocator {
			if g, ok := graph.(*rmtypes.Graph); ok {
				return repomap.NewSymbolLocator(g)
			}
			return nil
		})
		// Cross-sub-repo locator provider — drift detection in
		// authority.ComputeForEvidence prefers this when
		// BusContext.MultiGraph is wired (multi_repo_enabled).
		// Single-repo posture mg.Locator() forwards through one
		// graph locator (byte-equivalent); multi-repo posture fans
		// out via multiGraphLocator across every active sub-repo.
		authority.SetMultiGraphLocatorProvider(func(mgAny any) types.SymbolLocator {
			if mg, ok := mgAny.(*multigraph.MultiGraph); ok && mg != nil {
				return mg.Locator()
			}
			return nil
		})
		// Backfill Authority/Origin on bypass-path items
		// (concrete_value extractor, mechanism_scan, bridge_literal
		// merge) that build EvidenceItem literals without going
		// through emit_evidence's hook. Without this, those items
		// have Authority="" and HighestAuthorityFor treats them as
		// factual-equivalent — letting drift-bounded answers dodge
		// hedging when a sibling deterministic value happens to
		// share the same anchor.
		types.RegisterEvidenceProjector(authority.BackfillEvidenceProjector())
		// Commit 53 P4: diagram bare-identifier whitelist.
		tool.SetDiagramIdentifierWhitelist(rs.DiagramIdentifierWhitelist)
		// Legacy analyzer reconcile strict-mode compatibility gate.
		if rs.AnalyzerReconcileStrictMode != nil {
			agent.SetReconcileStrictMode(*rs.AnalyzerReconcileStrictMode)
		}
		// File-shaped MentionedEntities seeder — operator-tunable
		// sibling-suffix list. nil leaves the default
		// (.example / .sample / .dist / .tmpl / .tpl). Empty slice
		// also restores defaults inside the setter (intentional —
		// "remove this knob from yaml" must not silently disable
		// the layer; operators who want a strictly empty suffix list
		// should disable the seeder by leaving MentionedEntities
		// empty in the analyzer's emit instead).
		if rs.AnalyzerMentionedFileSiblingSuffixes != nil {
			agent.SetMentionedFileSiblingSuffixes(rs.AnalyzerMentionedFileSiblingSuffixes)
		}
		// MentionCount floor — operator tuning for the high/low
		// priority split of file-shaped MentionedEntities. Default 3.
		if rs.AnalyzerRequiredFileMentionCountFloor != nil {
			agent.SetRequiredFileMentionCountFloor(*rs.AnalyzerRequiredFileMentionCountFloor)
		}
		// MentionCount grep cap — limits how many ripgrep / grep
		// invocations the seeder issues per analyzer dispatch. Default 3.
		if rs.AnalyzerMentionCountMaxGrep != nil {
			agent.SetMentionCountMaxGrep(*rs.AnalyzerMentionCountMaxGrep)
		}
		// concrete_values L2/L3 DiagramRole projection knobs.
		if rs.ConcreteValuesConfigLayerExtensions != nil {
			agent.SetConcreteValueConfigLayerExtensions(rs.ConcreteValuesConfigLayerExtensions)
		}
		if rs.ConcreteValuesRuntimeMethodPrefixes != nil {
			agent.SetConcreteValueRuntimeMethodPrefixes(rs.ConcreteValuesRuntimeMethodPrefixes)
		}
		if rs.ConcreteValuesDefaultMethodPrefixes != nil {
			agent.SetConcreteValueDefaultMethodPrefixes(rs.ConcreteValuesDefaultMethodPrefixes)
		}
		// Phase 4 (Semantic Surface Contract) master switch.
		if rs.PipelineFacetValidatorsEnabled != nil {
			orchestrator.SetFacetValidatorsEnabled(*rs.PipelineFacetValidatorsEnabled)
		}
		// B8-T7 (block_only_carrier.md §5.8, 2026-05-03): V2-rollout
		// gating knobs (--emit-v2 CLI / pipeline_emit_v2_default yaml /
		// pipeline_v1_oracle_strict_mode yaml) deleted. V2 is the only
		// answer carrier; V1 schema is rejected at runtime since B8-T3.
		// Operators who previously rolled back via these knobs revert
		// the entire B8 commit range instead.
		// Commit 61 Batch F.2: yaml-overridable grounding floors.
		// Defaults are 0/0 (gates disabled); explicit positive values
		// restore strict-mode rejection of low-grounded
		// emit_investigation_complete calls.
		if rs.AnalysisGroundingFloor != nil || rs.AnalysisEvidenceTier1Floor != nil {
			policy := tool.CurrentGroundingPolicy()
			if rs.AnalysisGroundingFloor != nil {
				policy = policy.WithGroundingFloor(*rs.AnalysisGroundingFloor)
			}
			if rs.AnalysisEvidenceTier1Floor != nil {
				policy = policy.WithTier1Floor(*rs.AnalysisEvidenceTier1Floor)
			}
			tool.SetGroundingPolicy(policy)
		}
		// Commit 53 P5: repomap parse-tier hard gate.
		if rs.RepomapMinParseTier != nil {
			retrieve.SetMinParseTierFloor(*rs.RepomapMinParseTier)
		}
		if rs.PipelineTransientRetryBudget != nil {
			pipelineSettings.TransientRetryBudget = *rs.PipelineTransientRetryBudget
		}
		if rs.PipelineTransientRetryBudgetCeil != nil {
			pipelineSettings.TransientRetryBudgetCeil = *rs.PipelineTransientRetryBudgetCeil
		}
		if rs.PipelineForceFinalizeAttempts != nil {
			pipelineSettings.ForceFinalizeAttempts = *rs.PipelineForceFinalizeAttempts
		}
		if rs.PipelineMaxUpstreamFallbacksPerRun != nil {
			pipelineSettings.MaxUpstreamFallbacksPerRun = *rs.PipelineMaxUpstreamFallbacksPerRun
		}
		// Baseline capture toggle for CritNoRegression (Item 1).
		// Pointer-typed yaml so explicit false is distinguishable
		// from unset; both resolve to false on the orch but the
		// distinction matters for yaml-merge precedence.
		if rs.PipelineBaselineCaptureEnabled != nil {
			pipelineSettings.BaselineCaptureEnabled = *rs.PipelineBaselineCaptureEnabled
		}
		// Baseline disk-cache cap (commit 2 P0 A1). Default 16; 0
		// disables caching entirely.
		pipelineSettings.BaselineCacheMax = 16
		if rs.PipelineBaselineCacheMax != nil {
			pipelineSettings.BaselineCacheMax = *rs.PipelineBaselineCacheMax
		}
		// Write-mode wall-time cap (commit 2 P0 C). Default 1800 s;
		// 0 disables the cap; hard-capped at 1800 inside the orch
		// setter. The default intentionally matches the hard ceiling so
		// an active verify-failure replan is not cancelled halfway through.
		pipelineSettings.WriteMaxSeconds = defaultPipelineWriteMaxSeconds
		if rs.PipelineWriteMaxSeconds != nil {
			pipelineSettings.WriteMaxSeconds = *rs.PipelineWriteMaxSeconds
		}
		if rs.PipelinePlanCriticEnabled != nil {
			pipelineSettings.PlanCriticEnabled = *rs.PipelinePlanCriticEnabled
		}
		if rs.WriteWorkflowEngine != nil {
			raw := strings.TrimSpace(*rs.WriteWorkflowEngine)
			if raw != "" && !strings.EqualFold(raw, types.WriteWorkflowEngineController) {
				logging.Warning("[config] write_workflow_engine=%q is deprecated; write mode uses %s",
					raw, types.WriteWorkflowEngineController)
			}
			pipelineSettings.WriteWorkflowEngine = types.NormalizeWriteWorkflowEngine(raw)
		}
		// Keep-worktree-on-success toggle (Fix 4 "try before merge").
		// Same pointer-typed yaml shape so explicit false is
		// distinguishable from unset.
		if rs.PipelineKeepWorktreeOnSuccess != nil {
			pipelineSettings.KeepWorktreeOnSuccess = *rs.PipelineKeepWorktreeOnSuccess
		}
		// V5 lint master switch (default true; tool.SetLintEnabled
		// applies the operator override). Falls through to the
		// per-language toolchain probes inside validatePlanLint —
		// this knob just lets operators veto the entire family.
		if rs.PipelineLintEnabled != nil {
			tool.SetLintEnabled(*rs.PipelineLintEnabled)
		}
		// Run-scoped pure-tool memo kill switch (EVALFIX-2B 类2; default
		// true — tool.SetPureToolMemoEnabled applies the operator veto).
		if rs.PipelinePureToolMemoEnabled != nil {
			tool.SetPureToolMemoEnabled(*rs.PipelinePureToolMemoEnabled)
		}
		// Mermaid renderability gate (off / soft / strict; default
		// soft when unset). Boundary: gate flags ONLY Mermaid-spec
		// violations, never library-subset gaps — see comment on
		// PipelineMermaidRenderabilityGate in runtime.go.
		if rs.PipelineMermaidRenderabilityGate != nil {
			switch strings.ToLower(strings.TrimSpace(*rs.PipelineMermaidRenderabilityGate)) {
			case "off":
				tool.SetMermaidGateMode(tool.MermaidGateOff)
			case "soft", "":
				tool.SetMermaidGateMode(tool.MermaidGateSoft)
			case "strict":
				tool.SetMermaidGateMode(tool.MermaidGateStrict)
			default:
				logging.Warning("unknown pipeline_mermaid_renderability_gate=%q; expected off/soft/strict; defaulting to soft", *rs.PipelineMermaidRenderabilityGate)
				tool.SetMermaidGateMode(tool.MermaidGateSoft)
			}
		}
		// T3.2 repomap tier-degradation thresholds. Both knobs
		// inherit the code default when unset / zero. Setting
		// just one is fine — SetTierThresholds skips the unset
		// side. Plumb BEFORE any ScanFiles can fire.
		if rs.RepomapTierWarnRatio != nil || rs.RepomapTierAlertRatio != nil {
			warn := 0.0
			alert := 0.0
			if rs.RepomapTierWarnRatio != nil {
				warn = *rs.RepomapTierWarnRatio
			}
			if rs.RepomapTierAlertRatio != nil {
				alert = *rs.RepomapTierAlertRatio
			}
			repomapindex.SetTierThresholds(warn, alert)
		}
		// Verify resource caps — applied to every run_tests +
		// exec_command supervised invocation. Either knob unset =
		// package default (2048 MiB / 600 CPU-s / 900 wall-s); see
		// SetVerifyResourceCaps and SetRunTestsDefaultTimeoutSeconds.
		// Provenance: 2026-04-26 OOM event (pytest RSS 2.47 GiB on a
		// 3.7 GiB host); see internal/tool/exec_resource_caps.go.
		memMB := 0
		if rs.VerifyMemLimitMB != nil {
			memMB = *rs.VerifyMemLimitMB
		}
		cpuSec := 0
		if rs.VerifyCPULimitSeconds != nil {
			cpuSec = *rs.VerifyCPULimitSeconds
		}
		tool.SetVerifyResourceCaps(memMB, cpuSec)
		wallSec := 0
		if rs.VerifyWallTimeoutSeconds != nil {
			wallSec = *rs.VerifyWallTimeoutSeconds
		}
		tool.SetRunTestsDefaultTimeoutSeconds(wallSec)

		// Gate thresholds → package-global in gate package.
		var gt gate.Thresholds
		if rs.GateCoverageMin != nil {
			gt.CoverageMin = float32(*rs.GateCoverageMin)
		}
		if rs.GateCoverageWeightSymbol != nil {
			gt.CoverageWeights.Symbol = float32(*rs.GateCoverageWeightSymbol)
		}
		if rs.GateCoverageWeightConfig != nil {
			gt.CoverageWeights.Config = float32(*rs.GateCoverageWeightConfig)
		}
		if rs.GateCoverageWeightConcept != nil {
			gt.CoverageWeights.Concept = float32(*rs.GateCoverageWeightConcept)
		}
		if rs.GateHypothesisMinPriority != nil {
			gt.HypothesisMinPrio = *rs.GateHypothesisMinPriority
		}
		gate.SetGlobalThresholds(gt)

		// Explorer per-tool default cap.
		if rs.ExplorePerToolDefaultCap != nil {
			pipelineSettings.Explore.PerToolDefaultCap = *rs.ExplorePerToolDefaultCap
		}

		// Explorer heuristic thresholds.
		h := &pipelineSettings.Explore.Heuristics
		if rs.ExploreMidLoopMinIteration != nil {
			h.MidLoopMinIteration = *rs.ExploreMidLoopMinIteration
		}
		if rs.ExploreSerialBatchThreshold != nil {
			h.SerialBatchThreshold = *rs.ExploreSerialBatchThreshold
		}
		if rs.ExploreSerialStreakThreshold != nil {
			h.SerialStreakThreshold = *rs.ExploreSerialStreakThreshold
		}
		if rs.ExplorePartialReadLineThreshold != nil {
			h.PartialReadLineThreshold = *rs.ExplorePartialReadLineThreshold
		}
		if rs.ExploreMidLoopEnumCoverage != nil {
			h.MidLoopEnumCoverage = *rs.ExploreMidLoopEnumCoverage
		}
		if rs.ExploreSoftStopEnumCoverage != nil {
			h.SoftStopEnumCoverage = *rs.ExploreSoftStopEnumCoverage
		}
		if rs.ExplorePhase0MinDiscovered != nil {
			h.Phase0MinDiscoveredFiles = *rs.ExplorePhase0MinDiscovered
		}
		if rs.ExplorePhase0MaxBroaden != nil {
			h.Phase0MaxBroadenAttempts = *rs.ExplorePhase0MaxBroaden
		}
		if rs.ExploreSymbolMinLenMethod != nil {
			h.SymbolMinLenMethod = *rs.ExploreSymbolMinLenMethod
		}
		if rs.ExploreSymbolMinLenOther != nil {
			h.SymbolMinLenOther = *rs.ExploreSymbolMinLenOther
		}
		if rs.ExploreMaxPreScannedPushes != nil {
			h.MaxPreScannedPushes = *rs.ExploreMaxPreScannedPushes
		}
		if rs.ExploreCVPreviewMaxLen != nil {
			h.CVPreviewMaxLen = *rs.ExploreCVPreviewMaxLen
		}
		if rs.ExploreParallelUnreadFloor != nil {
			h.ParallelUnreadFloor = *rs.ExploreParallelUnreadFloor
		}
		if rs.ExploreEnumMidLoopUnreadFloor != nil {
			h.EnumMidLoopUnreadFloor = *rs.ExploreEnumMidLoopUnreadFloor
		}
		if rs.ExploreErmSuggestLimit != nil {
			h.ErmSuggestLimit = *rs.ExploreErmSuggestLimit
		}
	}
	// CLI flag overrides for pipeline budget.
	if flagChanged("pipeline-max-retries") && flagMaxRetries > 0 {
		pipelineSettings.MaxRetriesPerStage = flagMaxRetries
	}
	if flagChanged("pipeline-max-stage-visits") && flagMaxStageVisits > 0 {
		pipelineSettings.MaxStageVisits = flagMaxStageVisits
	}
	// Resolve zero-valued heuristic fields to code defaults.
	pipelineSettings.Explore.Heuristics = types.ResolvedExploreHeuristics(pipelineSettings.Explore.Heuristics)

	// Agent-level limits from YAML.
	if rs != nil {
		a := &pipelineSettings.Agent
		if rs.AgentMaxIterations != nil {
			a.MaxIterations = *rs.AgentMaxIterations
		}
		// Resolve agent_max_tool_history from fraction → absolute →
		// code default (0 = inherit BaseAgent's internal default).
		// Same precedence rule as blob_max_inline. A non-zero absolute
		// that is ALSO accompanied by a fraction means the operator
		// declared both in case the adapter reports zero context_window
		// — the fraction-form resolver prefers fraction and the
		// absolute becomes a safety net.
		if histBytes := config.ResolveByteBudget(
			rs.AgentMaxToolHistoryFraction, rs.AgentMaxToolHistoryBytes, 0, ctxWindow,
		); histBytes > 0 {
			a.MaxToolHistoryBytes = histBytes
			if rs.AgentMaxToolHistoryFraction != nil && *rs.AgentMaxToolHistoryFraction > 0 {
				logging.Info("[config] agent_max_tool_history resolved: %d bytes (fraction=%.3f × context_window=%d × %d B/token)",
					histBytes, *rs.AgentMaxToolHistoryFraction, ctxWindow, config.BytesPerToken)
			}
		}
		if rs.AgentLoopMinInjectInterval != nil {
			a.LoopMinInjectInterval = *rs.AgentLoopMinInjectInterval
		}
		if rs.AgentLoopMaxContinuations != nil {
			a.LoopMaxContinuations = *rs.AgentLoopMaxContinuations
		}
		if rs.AgentLoopMaxMidLoopInjects != nil {
			a.LoopMaxMidLoopInjects = *rs.AgentLoopMaxMidLoopInjects
		}
		if rs.AgentLoopIdleStopThreshold != nil {
			a.LoopIdleStopThreshold = *rs.AgentLoopIdleStopThreshold
		}
		if rs.AgentFinalizerMaxCorrectionRetries != nil {
			a.FinalizerMaxCorrectionRetries = *rs.AgentFinalizerMaxCorrectionRetries
		}
		// FRCAP (§29.12, 2026-07-10) — F7 empty-blocks reject breaker
		// threshold, formerly a hardcoded const. Package-var setter
		// (not AgentSettings) so the knob lands without widening the
		// shared types surface. nil keeps the shipped default (3).
		if rs.AgentFinalizerEmptyBlocksBreakerMaxStreak != nil {
			agent.SetFinalizerEmptyBlocksBreakerMaxStreak(*rs.AgentFinalizerEmptyBlocksBreakerMaxStreak)
		}
		// XGAP-FIX ② (§29.104.8) — F8-T4 member-set coverage same-cause
		// breaker threshold. Package-var setter, same shape as the F7
		// knob above. nil keeps the shipped default (3).
		if rs.AgentFinalizerMemberSetBreakerMaxStrikes != nil {
			agent.SetFinalizerMemberSetBreakerMaxStrikes(*rs.AgentFinalizerMemberSetBreakerMaxStrikes)
		}
		if rs.AgentFinalizerPreservePriorProse != nil {
			a.FinalizerPreservePriorProse = rs.AgentFinalizerPreservePriorProse
		}
		if rs.AgentFinalizerShrinkageMinProseLen != nil {
			a.FinalizerShrinkageMinProseLen = *rs.AgentFinalizerShrinkageMinProseLen
		}
		if rs.AgentFinalizerShrinkageRatio != nil {
			a.FinalizerShrinkageRatio = *rs.AgentFinalizerShrinkageRatio
		}
		if rs.AgentExtractorMaxCorrectionRetries != nil {
			a.ExtractorMaxCorrectionRetries = *rs.AgentExtractorMaxCorrectionRetries
		}
		if rs.AgentSubTopicPrescanExtra != nil {
			a.SubTopicPrescanBudgetExtra = *rs.AgentSubTopicPrescanExtra
		}
		if rs.AgentSubTopicExplorerExtra != nil {
			a.SubTopicExplorerBudgetExtra = *rs.AgentSubTopicExplorerExtra
		}
		if rs.AgentSubTopicPlannerExtra != nil {
			a.SubTopicPlannerBudgetExtra = *rs.AgentSubTopicPlannerExtra
		}
		if rs.AgentPlannerComplexityExtra != nil {
			a.PlannerComplexityBudgetExtra = *rs.AgentPlannerComplexityExtra
		}
		if rs.AgentSubTopicPipelineExtra != nil {
			a.SubTopicPipelineStepsExtra = *rs.AgentSubTopicPipelineExtra
		}
		if rs.AgentSubTopicRetryExtra != nil {
			a.SubTopicRetryBudgetExtra = *rs.AgentSubTopicRetryExtra
		}
		if rs.AgentSubTopicExtractorExtra != nil {
			a.SubTopicExtractorBudgetExtra = *rs.AgentSubTopicExtractorExtra
		}
		if rs.AgentExtractorComplexityExtra != nil {
			a.ExtractorComplexityBudgetExtra = *rs.AgentExtractorComplexityExtra
		}
		if rs.AgentTargetPathsVerifierExtra != nil {
			a.TargetPathsVerifierBudgetExtra = *rs.AgentTargetPathsVerifierExtra
		}
		if rs.AgentPrescanRoundsCeil != nil {
			a.PrescanRoundsCeil = *rs.AgentPrescanRoundsCeil
		}
		if rs.AgentExplorerScaledIterMax != nil {
			a.ExplorerScaledIterMax = *rs.AgentExplorerScaledIterMax
		}
		if rs.AgentPlannerScaledIterMax != nil {
			a.PlannerScaledIterMax = *rs.AgentPlannerScaledIterMax
		}
		if rs.AgentExtractorScaledIterMax != nil {
			a.ExtractorScaledIterMax = *rs.AgentExtractorScaledIterMax
		}
		if rs.AgentVerifierScaledIterMax != nil {
			a.VerifierScaledIterMax = *rs.AgentVerifierScaledIterMax
		}
		if rs.AgentMaxRetryBudgetCeil != nil {
			a.MaxRetryBudgetCeil = *rs.AgentMaxRetryBudgetCeil
		}
		if rs.AgentPerfTriagerIterCap != nil {
			a.PerfTriagerIterCap = *rs.AgentPerfTriagerIterCap
		}
		if rs.AgentLogTriagerIterCap != nil {
			a.LogTriagerIterCap = *rs.AgentLogTriagerIterCap
		}
		if rs.AgentInvestigationCompletePolicy != nil {
			a.InvestigationCompletePolicy = *rs.AgentInvestigationCompletePolicy
		}
		if rs.AgentDowngradeConvergenceHard != nil {
			a.DowngradeConvergenceHard = *rs.AgentDowngradeConvergenceHard
		}
		if rs.AgentPriorConvPolicy != nil {
			a.PriorConvPolicy = *rs.AgentPriorConvPolicy
		}
		if rs.AgentContextPressureSoftRatio != nil {
			a.ContextPressureSoftRatio = *rs.AgentContextPressureSoftRatio
		}
		if rs.AgentContextPressureHardRatio != nil {
			a.ContextPressureHardRatio = *rs.AgentContextPressureHardRatio
		}
		// Per-evaluator iteration caps. ResolvedAgentSettings (called
		// just below) clamps invalid soft/hard pairs back to defaults,
		// so a typo here can't crash startup.
		if rs.AgentPlannerSoftIterCap != nil {
			a.PlannerSoftIterCap = *rs.AgentPlannerSoftIterCap
		}
		if rs.AgentPlannerHardIterCap != nil {
			a.PlannerHardIterCap = *rs.AgentPlannerHardIterCap
		}
		if rs.AgentExtractorSoftIterCap != nil {
			a.ExtractorSoftIterCap = *rs.AgentExtractorSoftIterCap
		}
		if rs.AgentExtractorHardIterCap != nil {
			a.ExtractorHardIterCap = *rs.AgentExtractorHardIterCap
		}
		if rs.AgentVerifierSoftIterCap != nil {
			a.VerifierSoftIterCap = *rs.AgentVerifierSoftIterCap
		}
		if rs.AgentVerifierHardIterCap != nil {
			a.VerifierHardIterCap = *rs.AgentVerifierHardIterCap
		}
		if rs.AgentCoderSoftIterSlack != nil {
			a.CoderSoftIterSlack = *rs.AgentCoderSoftIterSlack
		}
		if rs.AgentCoderHardIterRecovery != nil {
			a.CoderHardIterRecovery = *rs.AgentCoderHardIterRecovery
		}
	}
	pipelineSettings.Agent = types.ResolvedAgentSettings(pipelineSettings.Agent)
	// Mirror the resolved investigation_complete_policy into the
	// tool layer so emit_investigation_complete's preCompleteContractCheck
	// can honor "override" mode (skip pre-complete gates when the
	// orchestrator's DAG scheduler is already configured to skip
	// criteria).
	tool.SetInvestigationCompletePolicy(pipelineSettings.Agent.InvestigationCompletePolicy)
	tool.SetDowngradeConvergenceHardThreshold(pipelineSettings.Agent.DowngradeConvergenceHard)
	return pipelineSettings
}

// resolveLogTriageSettings overlays the settings file on the log-triage defaults.
func resolveLogTriageSettings(rs *config.RuntimeSettings) agent.LogTriageSettings {
	// Build LogTriageSettings from codrax.yaml with defaults filling
	// any missing knob. cmd/root.go's settings loader already populated
	// rs with nil-pointer absence semantics — overlay selectively.
	triageSettings := agent.DefaultLogTriageSettings()
	if rs != nil {
		if rs.LogTriageEnabled != nil {
			triageSettings.Enabled = *rs.LogTriageEnabled
		}
		if rs.LogTriageMinBytes != nil {
			triageSettings.MinBytes = *rs.LogTriageMinBytes
		}
		if rs.LogTriageMaxRetries != nil {
			triageSettings.MaxRetries = *rs.LogTriageMaxRetries
		}
		if rs.LogTriageTwoStepEnabled != nil {
			triageSettings.TwoStepEnabled = *rs.LogTriageTwoStepEnabled
		}
		if rs.LogTriageTwoStepBytes != nil {
			triageSettings.TwoStepBytes = *rs.LogTriageTwoStepBytes
		}
		if rs.LogTriageTwoStepCoverage != nil {
			triageSettings.TwoStepCoverage = *rs.LogTriageTwoStepCoverage
		}
		if rs.LogTriageMaxLLMCalls != nil {
			triageSettings.MaxLLMCalls = *rs.LogTriageMaxLLMCalls
		}
	}
	return triageSettings
}

// resolvePerfTriageSettings overlays the settings file on the trace-triage defaults.
func resolvePerfTriageSettings(rs *config.RuntimeSettings) agent.PerfTriageSettings {
	// Build PerfTriageSettings from codrax.yaml — same overlay
	// semantics as triageSettings above. Knob shapes are deliberately
	// parallel to log_triage_* so an operator who already understands
	// the log triage knobs configures perf without re-learning.
	perfSettings := agent.DefaultPerfTriageSettings()
	if rs != nil {
		if rs.PerfTriageEnabled != nil {
			perfSettings.Enabled = *rs.PerfTriageEnabled
		}
		if rs.PerfTriageMinBytes != nil {
			perfSettings.MinBytes = *rs.PerfTriageMinBytes
		}
		if rs.PerfTriageMaxRetries != nil {
			perfSettings.MaxRetries = *rs.PerfTriageMaxRetries
		}
		if rs.PerfTriageTwoStepEnabled != nil {
			perfSettings.TwoStepEnabled = *rs.PerfTriageTwoStepEnabled
		}
		if rs.PerfTriageTwoStepBytes != nil {
			perfSettings.TwoStepBytes = *rs.PerfTriageTwoStepBytes
		}
		if rs.PerfTriageTwoStepCoverage != nil {
			perfSettings.TwoStepCoverage = *rs.PerfTriageTwoStepCoverage
		}
		if rs.PerfTriageMaxLLMCalls != nil {
			perfSettings.MaxLLMCalls = *rs.PerfTriageMaxLLMCalls
		}
		if rs.PerfTriageLLMMaxBytes != nil {
			perfSettings.LLMMaxBytes = *rs.PerfTriageLLMMaxBytes
		}
	}
	return perfSettings
}

// newPipelineSkillRegistry returns the default skills. The registry is handed
// to composePipeline, which appends the explorer's run-time tools to
// explore-skill; a registry is therefore composed once.
func newPipelineSkillRegistry() *skill.Registry {
	skillRegistry := skill.NewRegistry()
	skill.RegisterDefaults(skillRegistry)
	logging.Info("registered %d skills", len(skillRegistry.List()))
	return skillRegistry
}

// pipelineCompositionInputs is everything the registries and the orchestrator
// are built from besides the package-level knobs resolvePipelineSettings
// installs.
type pipelineCompositionInputs struct {
	Settings types.PipelineSettings
	// DefaultLLM serves sub-agents and any agent the resolver has no adapter for.
	DefaultLLM llm.Adapter
	// Resolver supplies each agent's adapter. An adapter's declared context
	// window and output cap size that agent's requests, so a caller that
	// reproduces a recorded run declares the recorded values here.
	Resolver               agent.LLMResolver
	ToolParamCompatByAgent map[types.AgentName]types.ToolParamCompatConfig
	MCPServers             *mcp.Registry
	Skills                 *skill.Registry
	Emit                   render.EventEmitter
	LogTriage              agent.LogTriageSettings
	PerfTriage             agent.PerfTriageSettings
	MaxSteps               int
	Language               string
	BlobSessionDir         string
	RuntimeAnchor          string
	OutputDumpDir          string
	OutputMaxFiles         int
	// FinalizerDynamicContextClock is nil for a live run, which reads the wall
	// clock. See agent.Dependencies.
	FinalizerDynamicContextClock agent.DynamicContextClock
}

// pipelineComposition is what composePipeline built.
type pipelineComposition struct {
	Tools              *tool.Registry
	SubAgents          *agent.SubAgentRegistry
	Agents             *agent.Registry
	ReasoningCollector *reasoninggraph.EventCollector
	Orchestrator       *orchestrator.Orchestrator
}

// composePipeline registers the tools, sub-agents and agents and creates the
// orchestrator over them.
func composePipeline(in pipelineCompositionInputs) pipelineComposition {
	pipelineSettings := in.Settings
	skillRegistry := in.Skills
	mcpRegistry := in.MCPServers
	toolParamCompatByAgent := in.ToolParamCompatByAgent
	resolver := in.Resolver
	triageSettings, perfSettings := in.LogTriage, in.PerfTriage
	runtimeAnchor, blobSessionDir := in.RuntimeAnchor, in.BlobSessionDir
	outputDumpDir, outputMaxFiles := in.OutputDumpDir, in.OutputMaxFiles
	toolRegistry := tool.NewRegistry()
	tool.RegisterDefaults(toolRegistry)
	toolRegistry.Register(&repomap.RepoMapV2{})
	toolRegistry.Register(tool.NewProposeSubAgents())

	// Runtime tool registration. After the 2026-04-14 simplification
	// all three runtime flags (evidence_tool_mode / two_turn_explorer_mode
	// / answer_document_mode) were deleted; every structured tool
	// below is unconditionally registered. The explore-skill gets
	// a runtime append for emit_evidence so the declarative skill
	// config does not need to hard-code the tool name.
	toolRegistry.Register(&tool.EmitEvidence{})
	toolRegistry.Register(&tool.EmitInvestigationComplete{})
	if exploreSkill, err := skillRegistry.Get("explore-skill"); err == nil {
		exploreSkill.ToolSuggestions = append(exploreSkill.ToolSuggestions, "emit_evidence", "emit_investigation_complete")
	}
	toolRegistry.Register(&tool.EmitAnswerSymbol{})
	toolRegistry.Register(&tool.EmitHypothesisVerdict{})
	toolRegistry.Register(&tool.EmitAnswerDocument{})
	// Protocol-level retry preservation. Registered alongside
	// emit_answer_document so the LLM can choose patch vs full
	// re-emit on retry paths.
	toolRegistry.Register(&tool.EmitAnswerDocumentPatch{})
	toolRegistry.Register(&tool.EmitLogTriage{})
	toolRegistry.Register(&tool.EmitLogSegmentation{})
	toolRegistry.Register(&tool.EmitPerfTrace{})
	toolRegistry.Register(&tool.EmitPerfSegmentation{})
	toolRegistry.Register(&tool.EmitWriteAnalysis{})
	toolRegistry.Register(&tool.EmitWriteWorkflowDecision{})

	subAgentRegistry := agent.NewSubAgentRegistry()

	agentCfg := pipelineSettings.Agent
	reasoningCollector := reasoninggraph.NewEventCollector(runtimeReasoningGraphID(runtimeAnchor))
	deps := &agent.Dependencies{
		LLM:                          in.DefaultLLM,
		Tools:                        toolRegistry,
		MCPServers:                   mcpRegistry,
		SubAgents:                    subAgentRegistry,
		Skills:                       skillRegistry,
		ReasoningObserver:            reasoningCollector,
		MaxIterations:                agentCfg.MaxIterations,
		Emit:                         in.Emit,
		ExploreHeuristics:            pipelineSettings.Explore.Heuristics,
		AgentSettings:                agentCfg,
		ToolParamCompatByAgent:       toolParamCompatByAgent,
		LoopPolicy:                   loopPolicyFromAgentSettings(agentCfg),
		FinalizerDynamicContextClock: in.FinalizerDynamicContextClock,
	}

	agent.RegisterDefaultSubAgents(subAgentRegistry, deps)
	logging.Info("registered %d tools, %d sub-agents", len(toolRegistry.List()), len(subAgentRegistry.Names()))
	// Lazy-bind the cancel-probe callback BEFORE RegisterDefaults so
	// every agent constructed in this loop captures a non-nil
	// deps.CancelChecker via the dereference-copy in NewBaseAgent.
	// Pre-fix the assignment lived AFTER RegisterDefaults — by then
	// every BaseAgent had already snapshotted deps with CancelChecker
	// = nil, silently disabling Ctrl+C interruption (the user-trigger
	// for this commit). The closure resolves orch lazily; orch is
	// declared below and assigned after RegisterDefaults completes.
	var orch *orchestrator.Orchestrator
	deps.CancelChecker = func() error {
		if orch == nil {
			return nil
		}
		return orch.CancelChecker()()
	}

	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry, deps, resolver, triageSettings, perfSettings)
	logging.Info("registered %d agents", len(agentRegistry.List()))

	orch = orchestrator.New(pipelineSettings, agentRegistry, skillRegistry, subAgentRegistry)
	orch.SetReadRunEnvironmentBuildInfo(version, buildTime)
	orch.SetReasoningObserver(reasoningCollector)
	orch.SetMaxSteps(in.MaxSteps)
	orch.SetLanguage(in.Language)
	orch.SetEmitter(in.Emit)
	orch.SetBlobSessionDir(blobSessionDir)
	orch.SetTraceRuntimeAnchor(runtimeAnchor)
	orch.SetOutputDump(outputDumpDir, outputMaxFiles)
	// B2.3 verify→plan retry cap. Zero keeps B1 fail-loud semantics.
	orch.SetWriteRetryBudget(pipelineSettings.WriteRetryBudget)
	// Transient-dispatch retry cap. Decoupled from WriteRetryBudget so
	// LLM stream stalls / network blips never drain the SC retry budget.
	orch.SetTransientRetryBudget(pipelineSettings.TransientRetryBudget)
	// Force-finalize retry cap. Catches transient connection drops on
	// the last-resort composition call so a single EOF doesn't kill
	// the Run with no answer at all. Default 3 = 1 + 2 retries.
	orch.SetForceFinalizeAttempts(pipelineSettings.ForceFinalizeAttempts)
	// Block 3 (architecture overhaul 2026-05-02) — selective
	// upstream fallback cap. Default 2 read from PipelineSettings.
	orch.SetMaxUpstreamFallbacksPerRun(pipelineSettings.MaxUpstreamFallbacksPerRun)
	// P6 (2026-05-10) — finalize repair-loop hard cap. Reads the
	// CLI-level finalizeRepairHardCap (already populated from yaml
	// PipelineFinalizeRepairHardCap above). Pass 0 → orchestrator
	// falls back to FinalizeRepairHardCapDefault (2).
	orch.SetFinalizeRepairHardCap(finalizeRepairHardCap)
	// 2026-05-16 — deterministic exhaustive member-set review
	// threshold. Negative → orchestrator uses types default (30).
	orch.SetExhaustiveDeterministicReviewThreshold(exhaustiveDeterministicReviewThreshold)
	// 2026-05-16 — per-reviewer wall budget fraction.
	orch.SetReviewerWallBudgetFraction(reviewerWallBudgetFraction)
	orch.SetStrictAnswerReviewEnabled(strictAnswerReviewEnabled)
	return pipelineComposition{
		Tools:              toolRegistry,
		SubAgents:          subAgentRegistry,
		Agents:             agentRegistry,
		ReasoningCollector: reasoningCollector,
		Orchestrator:       orch,
	}
}
