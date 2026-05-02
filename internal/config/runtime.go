package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RuntimeSettings holds the per-process knobs that the codrax binary
// exposes on the command line. The division of responsibility across
// the two YAML files in config/ is:
//
//   - providers.yaml — LLM provider credentials and per-agent model
//     routing. Per-user secrets, never committed.
//   - codrax.yaml    — this file. Runtime knobs that describe how
//     the binary should run on this machine/in this invocation: log
//     sink, memory sink, language, per-run step budget, target repo,
//     and a pointer to providers.yaml.
//
// The pipeline topology (4 stages × 4 agents) is hardcoded in
// internal/orchestrator/topology.go and has no YAML counterpart.
//
// Every field is a pointer so the merge logic in cmd/root.go can tell
// "user omitted this key in the YAML file" (nil) from "user set it
// to the zero value" (non-nil pointer to the zero value). This matters
// for LogStdout in particular: a default of false and a config file
// with `log_stdout: false` must both be allowed to override each
// other based on precedence.
type RuntimeSettings struct {
	// Log + memory + language (per-process UX).
	LogDir    *string `yaml:"log_dir"`
	LogLevel  *string `yaml:"log_level"`
	LogStdout *bool   `yaml:"log_stdout"`
	MemoryDir *string `yaml:"memory_dir"`
	Lang      *string `yaml:"lang"`

	// Per-invocation defaults.
	Repo     *string `yaml:"repo"`
	Branch   *string `yaml:"branch"`
	CacheDir *string `yaml:"cache_dir"`

	// Tool blob sizing knobs. Flat-prefixed `blob_*` to keep the
	// namespace obvious without nesting. All four accept any
	// positive integer; non-positive (or omitted) means "use the
	// code default in internal/tool/blob.go".
	//
	//   - blob_max_sessions: number of startup session directories
	//     retained under <CWD>/.codrax/blob/. Each codrax process
	//     creates <timestamp>-<pid>/ on startup; prune runs at next
	//     startup and skips sessions whose PID is still live. 0
	//     disables the persistent layout and reverts to a per-trace
	//     tmpdir cleaned up at the end of each Run. Default 7
	//     (mirrors log_max_files for symmetry).
	BlobMaxInlineBytes   *int `yaml:"blob_max_inline_bytes"`
	BlobPreviewHeadBytes *int `yaml:"blob_preview_head_bytes"`
	BlobPreviewTailBytes *int `yaml:"blob_preview_tail_bytes"`
	BlobMaxSessions      *int `yaml:"blob_max_sessions"`

	// Fraction-form version of blob_max_inline_bytes. When set AND the
	// adapter reports a positive context_window, the effective byte
	// threshold is `context_window * fraction * BytesPerToken` (conservative
	// 4 bytes/token estimate). When either condition fails (fraction
	// nil / adapter reports zero), resolution falls back to
	// BlobMaxInlineBytes, then to the code default. Fraction form
	// lets one providers.yaml switch drive the right blob budget
	// across heterogeneous models without per-agent absolute tuning:
	// a 1M-window model gets a 40 KB blob budget, an 8K-window model
	// gets 328 bytes — both from `0.01`.
	BlobMaxInlineFraction *float64 `yaml:"blob_max_inline_fraction"`

	// Log retention knob. Flat-prefixed `log_*` alongside log_dir
	// and log_level. Controls how many rotated log files are kept
	// per log directory before the sweeper starts deleting the
	// oldest (never deletes a file owned by a live peer process).
	// 0 or nil → use the code default (7) in internal/logging/logger.go.
	LogMaxFiles *int `yaml:"log_max_files"`

	// read_file-specific knobs. `readfile_*` prefix. Currently a
	// single knob.
	//
	//   - readfile_small_limit_threshold: on an inline-sized file,
	//     offset=0 + Limit in (0, this] is treated as a lazy default
	//     and expanded to the whole file (limit=20 on a 66-line file
	//     silently hides the tail — a common LLM failure mode). Any
	//     non-zero offset is always honored; a Limit above this value
	//     is always honored. 0 disables the override entirely. Default
	//     100 in internal/tool/builtin.go.
	ReadFileSmallLimitThreshold *int `yaml:"readfile_small_limit_threshold"`

	// emit_analysis runtime validation knobs. Flat-prefixed
	// `analysis_*` so the namespace stays visible alongside the
	// `blob_*` and `pipeline_*` groups. All three are optional:
	// absent = inherit the code default in
	// internal/tool/analysis_limits.go.
	//
	//   - analysis_warn_below_keywords: soft floor; the tool
	//     attaches a "[warn: ...]" tag to the ToolResult.Summary
	//     when the analyzer emits fewer keywords. 0 disables the
	//     warning. Default 8.
	//   - analysis_reject_below_keywords: hard floor; the tool
	//     fails the emit call with Success=false when the keyword
	//     count is below this value. 0 disables rejection (only
	//     the warning fires). Default 0.
	//   - analysis_generic_entity_blocklist: lowercase words the
	//     validator drops from the entities slice because they
	//     poison ERM ranking. Empty disables the filter. Default
	//     lives in DefaultAnalysisLimits().
	//   - analysis_max_prescan_rounds: runtime hard-enforcement of
	//     the "1-2 rounds then emit_analysis" ceiling in the analyze
	//     stage. After N successful pre-scan rounds (iterations
	//     whose last-executed tool is repo_map / grep / list_files),
	//     the analyzer's LoopController force-stops the dispatch
	//     and the ParseOutput failsafe synthesises a zero-value
	//     RequestModel. 0 disables the gate entirely. Default 2.
	//   - analysis_warn_below_keyword_hit_ratio: soft floor on the
	//     runtime quality probe's keyword_hit_ratio (0.0-1.0).
	//     When the fraction of emit_analysis.keywords observed in
	//     the pre-scan summary blob drops below this value, the
	//     emit_analysis tool attaches a `[warn: keyword_hit_ratio=X
	//     below floor Y]` line to its Summary. 0 disables the
	//     warning entirely (the probe is still computed and
	//     surfaced via analysis_quality_probe). Default 0.
	//   - analysis_warn_below_entity_hit_ratio: same soft floor for
	//     entity_hit_ratio. Entities are higher signal than
	//     keywords so this is the stricter of the two knobs in
	//     practice. 0 disables. Default 0.
	AnalysisWarnBelowKeywords        *int     `yaml:"analysis_warn_below_keywords"`
	AnalysisRejectBelowKeywords      *int     `yaml:"analysis_reject_below_keywords"`
	AnalysisGenericEntityBlocklist   []string `yaml:"analysis_generic_entity_blocklist"`
	AnalysisRejectMultipleEmit       *bool    `yaml:"analysis_reject_multiple_emit"`
	AnalysisMaxPrescanRounds         *int     `yaml:"analysis_max_prescan_rounds"`
	AnalysisWarnBelowKeywordHitRatio *float64 `yaml:"analysis_warn_below_keyword_hit_ratio"`
	AnalysisWarnBelowEntityHitRatio  *float64 `yaml:"analysis_warn_below_entity_hit_ratio"`

	// Evidence grounding knobs. `evidence_*` prefix mirrors the
	// analysis_* namespace. Shipped with the 2026-04-17 redesign.
	//
	//   - evidence_grounding_floor: minimum (grounded + recovered) /
	//     total ratio required by emit_investigation_complete. Range
	//     [0, 1]. 0 disables the gate entirely; 1 requires every item
	//     grounded. Default 0.5.
	EvidenceGroundingFloor *float64 `yaml:"evidence_grounding_floor"`

	//   - evidence_tier1_floor: minimum (Tier-1 proven grounded /
	//     total) ratio required by emit_investigation_complete.
	//     Range [0, 1]. 0 disables. Default 0.3. Blocks pure-recovery
	//     investigations where the LLM never read_file'd the cited
	//     sources; the finalizer grounder's stricter Tier 2 would
	//     otherwise drop every such citation at cite time.
	EvidenceTier1Floor *float64 `yaml:"evidence_tier1_floor"`

	// Pipeline budget knobs. Flat-prefixed `pipeline_*`. Precedence:
	// code default → codrax.yaml → CLI flag.
	PipelineMaxSteps           *int `yaml:"pipeline_max_steps"`
	PipelineMaxRetriesPerStage *int `yaml:"pipeline_max_retries_per_stage"`
	PipelineMaxStageVisits     *int `yaml:"pipeline_max_stage_visits"`

	// PipelineWriteRetryBudget caps the verify→plan retry cycle in
	// ModeApply, expressed as the number of retry rounds AFTER the
	// initial attempt (so 2 = up to 3 total attempts). T4 renamed
	// from PipelineMaxVerifyRetries when write modes folded into the
	// scheduler. Default 3 (seeded in cmd/root.go) — Reflexion /
	// Self-Refine / AutoCodeRover empirical sweet spot. Set yaml
	// `pipeline_write_retry_budget: 0` to opt into fail-loud
	// behaviour (one attempt, surface failure). Hard-capped by
	// PipelineWriteRetryBudgetCeil (default 5) inside
	// orchestrator.SetWriteRetryBudget so operator typos can't burn
	// an unbounded LLM budget on an unfixable plan.
	PipelineWriteRetryBudget *int `yaml:"pipeline_write_retry_budget"`

	// PipelineWriteRetryBudgetCeil overrides the absolute ceiling on
	// PipelineWriteRetryBudget enforced inside SetWriteRetryBudget.
	// Default 5; raise only after measuring that the LLM is producing
	// genuinely improved plans on attempts 6+, which is rare.
	PipelineWriteRetryBudgetCeil *int `yaml:"pipeline_write_retry_budget_ceil"`

	// PipelineMaxPhasesPerRun caps the LLM-emitted PhaseProposal in
	// stage II multi-phase write Runs. Default
	// types.MaxPhasesPerGroup (5). Operator override only useful
	// for genuinely multi-step migrations (schema → ORM → API →
	// docs); LLMs over-decomposing what should be a single plan
	// is a real failure mode the cap defends against. Hard-capped
	// by types.MaxPhasesPerGroupCeil (12) inside types.SetMaxPhases
	// so a yaml typo can't unleash a 30-phase fan-out.
	PipelineMaxPhasesPerRun *int `yaml:"pipeline_max_phases_per_run"`

	// PipelineFailureTaxonomyEnabled gates stage 3's per-repo
	// Failure Taxonomy: when false, the reflector still runs
	// but the LLM's emit_failure_pattern emits are dropped
	// (not persisted) and the planner's pitfalls section
	// is omitted. Useful for short-lived sandbox runs where
	// learned-pitfall accumulation is more noise than signal.
	// Default true (enable cross-Run learning by default).
	PipelineFailureTaxonomyEnabled *bool `yaml:"pipeline_failure_taxonomy_enabled"`

	// PipelineFailureTaxonomyMaxItems caps the per-repo
	// taxonomy size. Append-overflow drops the lowest-value
	// entries (Confidence × HitCount × recency). Default 50,
	// which fits an LLM context budget of ~3 KB when all are
	// injected as a worst case. Higher = larger prompt cost
	// when injecting; lower = faster forgetting.
	PipelineFailureTaxonomyMaxItems *int `yaml:"pipeline_failure_taxonomy_max_items"`

	// PipelineFailureTaxonomyDecayDays expires one-off (HitCount
	// < 2) patterns older than N days at next Append. Default
	// 90 — matches the heuristic that an unrepeated pattern
	// after 3 months was probably a one-off coincidence. Set
	// 0 to disable decay (patterns persist forever).
	PipelineFailureTaxonomyDecayDays *int `yaml:"pipeline_failure_taxonomy_decay_days"`

	// PipelineAnswerTaxonomyEnabled gates the read-mode mirror
	// (commit 51 Gap 3): when false, the end-of-Run answer
	// reviewer is not dispatched and analyzer-injection is
	// omitted. Default true.
	PipelineAnswerTaxonomyEnabled *bool `yaml:"pipeline_answer_taxonomy_enabled"`

	// PipelineAnswerTaxonomyMaxItems caps the per-repo answer
	// taxonomy size. Default 50, mirrors the failure taxonomy
	// cap.
	PipelineAnswerTaxonomyMaxItems *int `yaml:"pipeline_answer_taxonomy_max_items"`

	// PipelineAnswerTaxonomyDecayDays expires one-off answer
	// patterns. Default 90, mirrors the failure taxonomy.
	PipelineAnswerTaxonomyDecayDays *int `yaml:"pipeline_answer_taxonomy_decay_days"`

	// PipelineContractSoftKinds + PipelineContractStrictKinds
	// (commit 53 P3) override the default soft/strict gate
	// classification per ViolationKind. Soft kinds are mirrored
	// to EvidenceClosure for telemetry but do NOT flip
	// contract.Result.Passed=false (no finalize retry). Strict
	// kinds DO flip Passed=false. Default soft set covers the 3
	// commit-53 P2/P4 newcomers (shape_intent_mismatch,
	// sub_topic_count_mismatch, diagram_identifier_unverified);
	// every legacy kind defaults to strict for byte-identical
	// pre-commit-53 behaviour. Operators add to soft / strict
	// here to relax / tighten without code changes.
	PipelineContractSoftKinds   []string `yaml:"pipeline_contract_soft_kinds"`
	PipelineContractStrictKinds []string `yaml:"pipeline_contract_strict_kinds"`

	// PipelineFallbackPolicyOverrides (Block 3 architecture overhaul
	// 2026-05-02) lets operators override the per-ViolationKind
	// fallback target used by the selective upstream fallback
	// machinery (orchestrator/fallback_policy.go). Keys are kind
	// names (e.g. "self_contradiction"), values are target names
	// (one of "finalizer_only", "back_to_extract", "back_to_explore",
	// "back_to_analyze", "fail_loud"). Unknown keys / values are
	// silently dropped at apply time. Default policy lives in
	// orchestrator.DefaultFallbackPolicy().
	PipelineFallbackPolicyOverrides map[string]string `yaml:"pipeline_fallback_policy_overrides"`

	// PipelineMaxUpstreamFallbacksPerRun (Block 3 — 2026-05-02) caps
	// the number of selective upstream fallbacks (BackToExtract /
	// BackToExplore) any single Run may consume. Each round bumps
	// the counter; reaching the cap forces FailLoud on the next
	// violation. Default 2 — enough to give the LLM two
	// re-investigation passes, tight enough to bound wall-clock.
	// 0 disables the cap (not recommended); negative values fall
	// back to the default.
	PipelineMaxUpstreamFallbacksPerRun *int `yaml:"pipeline_max_upstream_fallbacks_per_run"`

	// DiagramIdentifierWhitelist (commit 53 P4) lists English /
	// domain words that the Mermaid bare-identifier oracle should
	// NOT flag even when they fail the SymbolOracle lookup. The
	// CamelCase regex already filters single-word labels (User /
	// Action / Service); this list catches multi-token cases like
	// "HttpRequest" the operator wants to allow as a generic term
	// rather than tying to a specific repo symbol. Empty default.
	DiagramIdentifierWhitelist []string `yaml:"diagram_identifier_whitelist"`

	// AnalyzerReconcileStrictMode (commit 61 Batch F.3, audit
	// red line "no system hard-cap") gates whether reconcileShape's
	// rule-based shape override is APPLIED to the IR or merely
	// LOGGED as advisory. Default false (advisory only) preserves
	// the LLM's emit_analysis shape decision unchanged. Operators
	// flip true to restore the pre-commit-61 hard-override behaviour
	// (5 hard-coded predicate-combination rules force a specific
	// shape regardless of LLM judgment).
	AnalyzerReconcileStrictMode *bool `yaml:"analyzer_reconcile_strict_mode"`

	// AnalysisGroundingFloor / AnalysisEvidenceTier1Floor (commit 61
	// Batch F.2): the strict-mode opt-in for emit_investigation_complete's
	// grounding-ratio gates. Default 0/0 (gates DISABLED). Operators
	// who want the pre-commit-61 strict behaviour set 0.5 / 0.3 (or
	// stricter) here. The LLM's "investigation complete" claim is
	// trusted by default; the finalizer's downstream citation
	// grounding still validates emitted citations against real lines.
	AnalysisGroundingFloor *float64 `yaml:"analysis_grounding_floor"`

	// AnalysisEvidenceTier1Floor mirrors above for the Tier-1 gate.
	AnalysisEvidenceTier1Floor *float64 `yaml:"analysis_evidence_tier1_floor"`

	// AnalyzerMentionedFileSiblingSuffixes overrides the default suffix
	// list that the file-shaped MentionedEntities seeder probes when
	// scanning for "variant of base file" siblings (.example /
	// .sample / .dist / .tmpl / .tpl). Each entry is a filesystem-
	// convention extension that names a versioned or template form of
	// a canonical base file (e.g. `codrax.yaml.example` ←→
	// `codrax.yaml`). Operators add or remove entries when their repo
	// uses a different convention (`.in` for autoconf,
	// `.template` for hugo themes). Empty / missing falls back to the
	// default list. Leading "." is auto-prepended if absent.
	AnalyzerMentionedFileSiblingSuffixes []string `yaml:"analyzer_mentioned_file_sibling_suffixes"`

	// AnalyzerRequiredFileMentionCountFloor is the minimum repo-wide
	// reference count for a file-shaped MentionedEntity hit to enter
	// the high-priority lane of the cap=3 RequiredFiles budget. A
	// "reference" is any other file in the repo that contains the
	// hit's basename as a substring (computed once per dispatch via
	// ripgrep / grep / native walker, ~30-50ms per unique basename).
	//
	// Above the floor (default 3): fs-hits take precedence over
	// generic repomap-ranker output — the repo treats this file as
	// load-bearing.
	// Below the floor: fs-hits join the merge tail and compete with
	// ranker QueryScore hits on equal footing.
	//
	// nil / 0 = use default (3). Lower (e.g. 1) trusts every existing
	// fs-hit; raise (e.g. 5) to be more conservative on noisy
	// repos with many incidental basename mentions.
	AnalyzerRequiredFileMentionCountFloor *int `yaml:"analyzer_required_file_mention_count_floor"`

	// AnalyzerMentionCountMaxGrep caps how many ripgrep / grep
	// invocations the file-shaped MentionedEntities seeder may issue
	// per analyzer dispatch. Each invocation is one unique basename
	// (~30-50 ms on a medium repo with ripgrep). Default 3 covers the
	// typical 1-3 fs-hits per request without grepping the world.
	// Beyond the cap, remaining fs-hits get MentionCount=0 and
	// drop into the low-priority lane (gracefully degrades to
	// equal-footing with ranker QueryScore hits — never blocks).
	AnalyzerMentionCountMaxGrep *int `yaml:"analyzer_mention_count_max_grep"`

	// PipelineSelfConsistencyReviewEnabled (commit 62, this-phase
	// default TRUE) gates the post-finalize self-consistency
	// reviewer LLM. When true, an independent reviewer reads
	// doc.Summary + body bullets and reports factual contradictions
	// between the two paragraphs. Real eval s1a-20260501-083611
	// motivated this gate. nil = use default (true).
	PipelineSelfConsistencyReviewEnabled *bool `yaml:"pipeline_self_consistency_review_enabled"`

	// PipelineSelfConsistencyRewriteOnContradiction (commit 62,
	// this-phase default TRUE) toggles whether ViolSelfContradiction
	// is treated as STRICT (triggers finalizer retry with reviewer's
	// contradictions injected as repair hint) or SOFT (telemetry
	// only; answer ships unchanged). nil = use default (true). Off
	// = soft, the answer with contradictions still goes to user but
	// reviewer findings are recorded for cross-Run learning.
	PipelineSelfConsistencyRewriteOnContradiction *bool `yaml:"pipeline_self_consistency_rewrite_on_contradiction"`

	// PipelineSelfConsistencyMinConfidence (commit 62, default 0.8)
	// is the reviewer self-rated confidence floor a verdict must
	// reach to be acted on. Below the floor, the verdict is silently
	// dropped (avoid cried-wolf noise). 0/nil = default 0.8.
	PipelineSelfConsistencyMinConfidence *float64 `yaml:"pipeline_self_consistency_min_confidence"`


	// RepomapMinParseTier (commit 53 P5) hard-gates files whose
	// repomap parse tier exceeds the floor (Tier 1=primary
	// grammar, 2=secondary salvage, 3=regex-only, 4=path-only).
	// 0 (default) = gate disabled (only the soft tier discount
	// applies, pre-commit-53 byte-identical). 1 = only Tier-1
	// files are ranked, 2 = Tier 1-2, etc. Useful when a repo
	// with mixed-language tree-sitter coverage produces too many
	// regex-only fallbacks that pollute the rank head.
	RepomapMinParseTier *int `yaml:"repomap_min_parse_tier"`

	// PipelineTransientRetryBudget caps how many times a single Run
	// will retry a stage that failed with a transient dispatch error
	// (LLM stream stall / first-byte timeout / 429 / 5xx / network
	// blip). Distinct counter from PipelineWriteRetryBudget so
	// transient blips never starve the verify→plan SC retry. Default 1.
	// Hard-capped by PipelineTransientRetryBudgetCeil (default 3).
	PipelineTransientRetryBudget *int `yaml:"pipeline_transient_retry_budget"`

	// PipelineTransientRetryBudgetCeil overrides the absolute ceiling
	// on PipelineTransientRetryBudget. Default 3.
	PipelineTransientRetryBudgetCeil *int `yaml:"pipeline_transient_retry_budget_ceil"`

	// PipelineMaxStepsCeil is the absolute ceiling on the multi-topic
	// scaled pipeline step budget. Default 100. Zero falls back.
	PipelineMaxStepsCeil *int `yaml:"pipeline_max_steps_ceil"`

	// PipelineForceFinalizeAttempts caps how many times the
	// force-finalize escape path retries when the dispatch fails
	// with a transient LLM stream error (unexpected EOF, stream
	// stalled, first-byte timeout, 429, network blip). Default 3
	// (1 initial + 2 retries) — empirically catches >95% of single-
	// connection drops while keeping the user-visible pause under
	// 3.5s in the worst case (500 ms + 1 s + 2 s backoff). Setting
	// to 1 reverts to the pre-2026-04-30 single-shot behaviour.
	// Hard-capped at 5 inside the orchestrator; values above 5
	// almost always mean misconfiguration that would burn LLM
	// tokens on a genuinely-down upstream.
	PipelineForceFinalizeAttempts *int `yaml:"pipeline_force_finalize_attempts"`

	// PipelineBaselineCaptureEnabled toggles the pre-apply test
	// snapshot that feeds CritNoRegression. When true, the apply stage hook
	// runs run_tests BEFORE the coder dispatches so the subsequent
	// verify stage has a Baseline vs Current diff to compare.
	// Default false — the extra test run doubles wall time. Cache
	// hits via PipelineBaselineCacheMax bypass the wall-time concern
	// (cached baselines are reused across Runs that touch the same
	// HEAD SHA), so the cache stays active regardless of this flag.
	PipelineBaselineCaptureEnabled *bool `yaml:"pipeline_baseline_capture_enabled"`

	// PipelineBaselineCacheMax sets the maximum number of cached
	// baseline reports the orchestrator stores under
	// <runtimeAnchor>/baselines/<sha8>.json. Each cache entry is
	// keyed by the main repo's HEAD SHA at apply-stage entry; when
	// the same SHA is observed in a future Run (the user did not
	// commit between Runs), the cached baseline is reused without
	// re-running the test suite. The cache itself is always active —
	// this knob bounds disk usage. Default 16. 0 disables caching
	// entirely (every Run captures fresh when enabled).
	PipelineBaselineCacheMax *int `yaml:"pipeline_baseline_cache_max"`

	// PipelineWriteMaxSeconds is the wall-clock ceiling for write-
	// mode Runs (plan / apply / verify). When the deadline expires,
	// the orchestrator cancels the in-flight stage and surfaces a
	// "write mode wall-time exceeded" error. Read-mode Runs are NOT
	// subject to this cap (read-mode latency expectations differ).
	// Default 600 seconds. 0 = no cap (legacy behaviour). Hard-
	// capped at 1800 inside the orchestrator setter so a typo
	// cannot starve a Run that is genuinely making progress.
	PipelineWriteMaxSeconds *int `yaml:"pipeline_write_max_seconds"`

	// PipelinePlanCriticEnabled gates the optional pre-apply plan
	// review LLM call (commit 4 P1-F). When true and a plan_critic
	// adapter is configured (providers.yaml :: agents.plan_critic
	// or fallback to default), the orchestrator dispatches a single
	// Chat call after planner emit + before apply to surface
	// shape-level concerns the deterministic 11-stage validator
	// cannot catch (alignment with user goal, suspect dependencies,
	// inconsistencies). Critique text is INFORMATIONAL only;
	// apply runs regardless. Default false (opt-in: each enabled
	// run pays one extra LLM call).
	PipelinePlanCriticEnabled *bool `yaml:"pipeline_plan_critic_enabled"`

	// PipelineKeepWorktreeOnSuccess, when true, preserves the git
	// worktree after a successful ModeApply so the user can review
	// the applied bytes and cherry-pick to main manually. Failure
	// paths always discard regardless of this flag. Default false.
	PipelineKeepWorktreeOnSuccess *bool `yaml:"pipeline_keep_worktree_on_success"`

	// WorktreeKeepTTLHours bounds how long preserved (keep-on-
	// success) worktrees survive at startup. Dirs in
	// <runtime-anchor>/worktrees/ whose mtime is older than the
	// TTL are removed at codrax launch. 0 = disabled (no age
	// reaping; quota still applies). Default 168 (7 days).
	WorktreeKeepTTLHours *int `yaml:"worktree_keep_ttl_hours"`

	// WorktreeKeepMaxCount caps how many preserved worktrees may
	// exist after age-reaping. When the survivor count exceeds
	// this, the oldest (by mtime) are LRU-evicted until count <=
	// the cap. 0 = disabled (no quota; only TTL applies). Default
	// 20.
	WorktreeKeepMaxCount *int `yaml:"worktree_keep_max_count"`

	// RepomapTierWarnRatio is the soft floor: per-language Tier-2+
	// fall-through ratio at which the repomap reporter emits an
	// INFO line ("trending toward extractor maintenance"). 0
	// inherits the code default (0.30). Operators get visibility
	// before degradation breaches the louder alert.
	RepomapTierWarnRatio *float64 `yaml:"repomap_tier_warn_ratio"`

	// RepomapTierAlertRatio is the per-language fall-through ratio
	// at which a WARN-level "consider extractor/grammar update"
	// banner fires. 0 inherits the code default (0.50; ArkTS/
	// Cangjie keep their stricter language-specific overrides).
	RepomapTierAlertRatio *float64 `yaml:"repomap_tier_alert_ratio"`

	// VerifyMemLimitMB caps the resident+virtual memory each
	// run_tests / exec_command invocation may use. Unix: enforced via
	// `ulimit -v` shell prefix. Windows: enforced via JobObject
	// JobMemoryLimit. Default 2048 MiB — large enough for most real
	// suites, tight enough that a 4 GiB host doesn't OOM the entire
	// system when an LLM-generated test allocates without bound.
	// Operators on dev hosts with abundant RAM can raise this; CI
	// runners often want lower (e.g. 512). Zero = use code default.
	//
	// Provenance: 2026-04-26 OOM event — pytest reached RSS 2.47 GiB
	// in a verify run on a 3.7 GiB host, killing the supervising
	// claude-code process via the system-wide OOM killer. Without a
	// memory cap the host has no defence against runaway test code.
	VerifyMemLimitMB *int `yaml:"verify_mem_limit_mb"`

	// VerifyCPULimitSeconds caps the total CPU seconds each
	// run_tests / exec_command invocation may consume across every
	// process in the supervised tree. Unix: `ulimit -t`, fires
	// SIGXCPU. Windows: PerJobUserTimeLimit on the JobObject.
	// Default 600 — distinct from the wall-clock timeout (300s) so a
	// CPU-burner on a multi-core host can't keep all cores at 100%
	// for the full wall budget. Zero = use code default.
	VerifyCPULimitSeconds *int `yaml:"verify_cpu_limit_seconds"`

	// PipelineLintEnabled gates the V5 lint validator family
	// (Python ruff, Go gofmt). When unset → default true; the
	// validator runs before the planner finalizes a plan and
	// rejects any kind=create entry whose new_content carries
	// dead-variable / unused-import / formatting defects. Operators
	// set false when their codebase carries lint debt and the
	// new-files-only filter still feels too noisy.
	PipelineLintEnabled *bool `yaml:"pipeline_lint_enabled"`

	// PipelineMermaidRenderabilityGate controls the renderability
	// gate inside emit_answer_document.go. Three modes:
	//
	//   off    — gate disabled; diagnostics still feed
	//            /mermaid stats counters but no submission is
	//            rejected.
	//   soft   — default. Diagnostics counted; LLM submissions
	//            never rejected for renderability reasons.
	//   strict — reject + retry when the terminal Mermaid renderer
	//            fails to parse a block (Mermaid-spec violation).
	//            Unsupported diagram kinds (classDiagram /
	//            stateDiagram / ...) and decoration-rune fallback
	//            paths NEVER reject regardless of mode — those are
	//            library-subset gaps absorbed by the
	//            compatibility shims, not LLM mistakes.
	//
	// nil → "soft" (code default).
	PipelineMermaidRenderabilityGate *string `yaml:"pipeline_mermaid_renderability_gate"`

	// Analyzer quality gate thresholds. Flat-prefixed `gate_*`.
	// All optional; zero/nil → code default in gate.Thresholds.
	GateCoverageMin           *float64 `yaml:"gate_coverage_min"`
	GateCoverageWeightSymbol  *float64 `yaml:"gate_coverage_weight_symbol"`
	GateCoverageWeightConfig  *float64 `yaml:"gate_coverage_weight_config"`
	GateCoverageWeightConcept *float64 `yaml:"gate_coverage_weight_concept"`
	GateHypothesisMinPriority *int     `yaml:"gate_hypothesis_min_priority"`

	// Explorer per-tool budget cap. 0 = no default ceiling; only
	// the analyzer's per-tool NodeBudgetHints govern.
	ExplorePerToolDefaultCap *int `yaml:"explore_per_tool_default_cap"`

	// Explorer heuristic thresholds. All optional; nil → code default
	// in types.DefaultExploreHeuristics().
	ExploreMidLoopMinIteration      *int     `yaml:"explore_midloop_min_iteration"`
	ExploreSerialBatchThreshold     *int     `yaml:"explore_serial_batch_threshold"`
	ExploreSerialStreakThreshold    *int     `yaml:"explore_serial_streak_threshold"`
	ExplorePartialReadLineThreshold *int     `yaml:"explore_partial_read_line_threshold"`
	ExploreMidLoopEnumCoverage      *float64 `yaml:"explore_midloop_enum_coverage"`
	ExploreSoftStopEnumCoverage     *float64 `yaml:"explore_softstop_enum_coverage"`
	ExplorePhase0MinDiscovered      *int     `yaml:"explore_phase0_min_discovered_files"`
	ExplorePhase0MaxBroaden         *int     `yaml:"explore_phase0_max_broaden_attempts"`
	ExploreSymbolMinLenMethod       *int     `yaml:"explore_symbol_min_len_method"`
	ExploreSymbolMinLenOther        *int     `yaml:"explore_symbol_min_len_other"`
	ExploreMaxPreScannedPushes      *int     `yaml:"explore_max_prescanned_pushes"`
	ExploreCVPreviewMaxLen          *int     `yaml:"explore_cv_preview_max_len"`
	ExploreParallelUnreadFloor      *int     `yaml:"explore_parallel_unread_floor"`
	ExploreEnumMidLoopUnreadFloor   *int     `yaml:"explore_enum_midloop_unread_floor"`
	ExploreErmSuggestLimit          *int     `yaml:"explore_erm_suggest_limit"`

	// Agent-level limits. All optional; nil → code default in
	// types.DefaultAgentSettings().
	AgentMaxIterations                 *int     `yaml:"agent_max_iterations"`
	AgentMaxToolHistoryBytes           *int     `yaml:"agent_max_tool_history_bytes"`
	// Fraction-form twin of AgentMaxToolHistoryBytes. Same resolution
	// rule as BlobMaxInlineFraction: fraction × context_window × 4
	// when both fraction and window present, else absolute, else code
	// default. Tool-history is the second-largest share of an
	// iteration's prompt (after the user message) so making it model-
	// aware closes the biggest gap in byte-budget portability.
	AgentMaxToolHistoryFraction        *float64 `yaml:"agent_max_tool_history_fraction"`
	// Context-pressure thresholds (BaseAgent watchdog). When the
	// adapter reports a positive context_window, the loop estimates
	// each iteration's assembled-prompt bytes and compares against
	// `context_window * BytesPerToken`. Breaching SoftRatio logs a
	// warning so operators see the approach; breaching HardRatio
	// force-stops the ReAct loop with an injected directive
	// preferring emit_investigation_complete. Zero (or both nil) on
	// a legacy yaml inherits the code default (0.7 / 0.9).
	AgentContextPressureSoftRatio *float64 `yaml:"agent_context_pressure_soft_ratio"`
	AgentContextPressureHardRatio *float64 `yaml:"agent_context_pressure_hard_ratio"`
	AgentLoopMinInjectInterval         *int     `yaml:"agent_loop_min_inject_interval"`
	AgentLoopMaxContinuations          *int     `yaml:"agent_loop_max_continuations"`
	AgentLoopMaxMidLoopInjects         *int     `yaml:"agent_loop_max_midloop_injects"`
	AgentLoopIdleStopThreshold         *int     `yaml:"agent_loop_idle_stop_threshold"`
	AgentFinalizerMaxCorrectionRetries *int     `yaml:"agent_finalizer_max_correction_retries"`
	AgentFinalizerPreservePriorProse   *bool    `yaml:"agent_finalizer_preserve_prior_prose"`
	AgentFinalizerShrinkageMinProseLen *int     `yaml:"agent_finalizer_shrinkage_min_prose_len"`
	AgentFinalizerShrinkageRatio       *float64 `yaml:"agent_finalizer_shrinkage_ratio"`
	AgentExtractorMaxCorrectionRetries *int     `yaml:"agent_extractor_max_correction_retries"`
	AgentSubTopicPrescanExtra          *int     `yaml:"agent_subtopic_prescan_extra"`
	AgentSubTopicExplorerExtra         *int     `yaml:"agent_subtopic_explorer_extra"`
	AgentSubTopicPlannerExtra          *int     `yaml:"agent_subtopic_planner_extra"`
	AgentPlannerComplexityExtra        *int     `yaml:"agent_planner_complexity_extra"`
	AgentSubTopicPipelineExtra         *int     `yaml:"agent_subtopic_pipeline_extra"`
	AgentSubTopicRetryExtra            *int     `yaml:"agent_subtopic_retry_extra"`
	AgentSubTopicExtractorExtra        *int     `yaml:"agent_subtopic_extractor_extra"`
	AgentExtractorComplexityExtra      *int     `yaml:"agent_extractor_complexity_extra"`
	AgentTargetPathsVerifierExtra      *int     `yaml:"agent_target_paths_verifier_extra"`
	AgentPrescanRoundsCeil             *int     `yaml:"agent_prescan_rounds_ceil"`
	AgentExplorerScaledIterMax         *int     `yaml:"agent_explorer_scaled_iter_max"`
	AgentPlannerScaledIterMax          *int     `yaml:"agent_planner_scaled_iter_max"`
	AgentExtractorScaledIterMax        *int     `yaml:"agent_extractor_scaled_iter_max"`
	AgentVerifierScaledIterMax         *int     `yaml:"agent_verifier_scaled_iter_max"`
	AgentMaxRetryBudgetCeil            *int     `yaml:"agent_max_retry_budget_ceil"`
	AgentPerfTriagerIterCap            *int     `yaml:"agent_perf_triager_iter_cap"`
	AgentLogTriagerIterCap             *int     `yaml:"agent_log_triager_iter_cap"`
	AgentInvestigationCompletePolicy   *string  `yaml:"agent_investigation_complete_policy"`
	AgentPriorConvPolicy               *string  `yaml:"agent_prior_conversation_policy"`

	// Per-evaluator iteration caps (soft / hard pair) — gates the
	// two-stage stop machinery in iterationCapShouldStop. Each pair
	// must satisfy hard > soft; ResolvedAgentSettings clamps invalid
	// pairs back to defaults. nil → code default in
	// types.DefaultAgentSettings.
	//
	// agent_planner_*    — emit_change_plan loop (default 6 / 9)
	// agent_extractor_*  — emit_answer_symbol loop (default 3 / 5)
	// agent_verifier_*   — emit_test_results loop  (default 5 / 8)
	// agent_coder_*      — apply_patch loop. soft = len(plan.TargetPaths)
	//                      + slack; hard = soft + recovery. Defaults:
	//                      slack=3, recovery=3.
	AgentPlannerSoftIterCap     *int `yaml:"agent_planner_soft_iter_cap"`
	AgentPlannerHardIterCap     *int `yaml:"agent_planner_hard_iter_cap"`
	AgentExtractorSoftIterCap   *int `yaml:"agent_extractor_soft_iter_cap"`
	AgentExtractorHardIterCap   *int `yaml:"agent_extractor_hard_iter_cap"`
	AgentVerifierSoftIterCap    *int `yaml:"agent_verifier_soft_iter_cap"`
	AgentVerifierHardIterCap    *int `yaml:"agent_verifier_hard_iter_cap"`
	AgentCoderSoftIterSlack     *int `yaml:"agent_coder_soft_iter_slack"`
	AgentCoderHardIterRecovery  *int `yaml:"agent_coder_hard_iter_recovery"`

	// Memory store limits. All optional; nil → code default in
	// types.DefaultMemorySettings().
	MemoryMaxRecentTurns         *int `yaml:"memory_max_recent_turns"`
	MemoryMaxRecentBytes         *int `yaml:"memory_max_recent_bytes"`
	MemoryMaxTurnBodyBytes       *int `yaml:"memory_max_turn_body_bytes"`
	MemoryMaxBuildContextMatches *int `yaml:"memory_max_build_context_matches"`

	// Memory retrieval-tuning knobs. Previously hardcoded as magic
	// numbers in internal/memory.scoreIndex (entityMinRunes=3,
	// sessionTieBreakerBonus=1) and policyFor (15 numbers across
	// 3 hardcoded policies). Surfaced here so operators with unusual
	// usage patterns (chat-heavy, plan-heavy, very long sessions)
	// can dial them without recompiling. nil → code default; missing
	// sub-policy fields → corresponding hardcoded default for that
	// Kind from types.DefaultMemoryKindPolicies.
	MemoryEntityMinRunes         *int                          `yaml:"memory_entity_min_runes"`
	MemorySessionTieBreakerBonus *int                          `yaml:"memory_session_tie_breaker_bonus"`

	// MemorySearchMaxLimit / MemoryListMaxLimit — hard caps on
	// Store.Search / Store.List `limit` opts. Default 20 / 30
	// (matching the historical hardcoded literals); raise only when
	// BuildContext's byte budget can clearly absorb the extra
	// matches.
	MemorySearchMaxLimit *int `yaml:"memory_search_max_limit"`
	MemoryListMaxLimit   *int `yaml:"memory_list_max_limit"`
	MemoryPolicyChitchat         *types.MemoryKindPolicy       `yaml:"memory_policy_chitchat,omitempty"`
	MemoryPolicyShell            *types.MemoryKindPolicy       `yaml:"memory_policy_shell,omitempty"`
	MemoryPolicyPipeline         *types.MemoryKindPolicy       `yaml:"memory_policy_pipeline,omitempty"`
	MemoryPolicyPlan             *types.MemoryKindPolicy       `yaml:"memory_policy_plan,omitempty"`
	MemoryPolicyDefault          *types.MemoryKindPolicy       `yaml:"memory_policy_default,omitempty"`

	// Per-shape Summary length ceilings enforced by
	// emit_answer_document and the shrinkage-salvage trimmer. All
	// optional; nil → code default in types.DefaultSummaryCapConfig().
	// StepList / ListOfSymbols scale with item count:
	//   cap = min(Max, Base + n*PerItem).
	//
	// summary_cap_enabled is the master switch. Default false → no
	// length enforcement runs at all (the numeric knobs below are
	// inert). Flip to true to activate the per-shape caps.
	SummaryCapEnabled         *bool `yaml:"summary_cap_enabled"`
	SummaryCapExplanation     *int  `yaml:"summary_cap_explanation"`
	SummaryCapValue           *int  `yaml:"summary_cap_value"`
	SummaryCapConfigValue     *int  `yaml:"summary_cap_config_value"`
	SummaryCapBoolean         *int  `yaml:"summary_cap_boolean"`
	SummaryCapStepListBase    *int  `yaml:"summary_cap_step_list_base"`
	SummaryCapStepListPerItem *int  `yaml:"summary_cap_step_list_per_item"`
	SummaryCapStepListMax     *int  `yaml:"summary_cap_step_list_max"`
	SummaryCapSymbolsBase     *int  `yaml:"summary_cap_symbols_base"`
	SummaryCapSymbolsPerItem  *int  `yaml:"summary_cap_symbols_per_item"`
	SummaryCapSymbolsMax      *int  `yaml:"summary_cap_symbols_max"`
	SummaryCapDefault         *int  `yaml:"summary_cap_default"`

	// CitationQuoteMaxChars bounds the preview length of each
	// Citation.Quote rendered in answer footers. Oversize Quotes are
	// truncated on a UTF-8 boundary; file+line anchors are always
	// preserved. Optional; nil → types.DefaultCitationMaxQuoteChars.
	// Raise this for codebases with routinely long source lines
	// (Kotlin DSLs, Scala implicits, deep package imports, long
	// fmt.Errorf / SQL / regex literals). Non-positive values are
	// ignored.
	CitationQuoteMaxChars *int `yaml:"citation_quote_max_chars"`

	// ChitchatEnabled gates the REPL's /chat slash command and the
	// optional classifier that precedes dispatch. When false, /chat
	// prints a "not configured" warning and the classifier (if
	// independently enabled) is not constructed. Default true — /chat
	// is explicit user action so the risk of enabling it is minimal.
	// Affects only the REPL; single-shot --request mode never touches
	// chit-chat paths regardless of this setting.
	ChitchatEnabled *bool `yaml:"chitchat_enabled"`

	// ChitchatClassifierEnabled turns on an automatic LLM-backed
	// classifier that runs once per REPL turn before the normal
	// dispatch. When the classifier decides a turn is casual, the
	// REPL reroutes it to the chit-chat responder without requiring
	// the user to prefix /chat; otherwise the turn proceeds to the
	// analysis pipeline exactly as before. Requires ChitchatEnabled
	// to also be true — the classifier has no effect without a
	// responder. Default true: fail-safe wiring routes any classifier
	// error back to the pipeline, so the worst case is a wasted LLM
	// call, not a misrouted code question. Operators who want to cap
	// the cost should route `chitchat_classifier` to a small model in
	// providers.yaml; those who want to disable entirely set this to
	// false or pass --chitchat-classifier=false at startup.
	ChitchatClassifierEnabled *bool `yaml:"chitchat_classifier_enabled"`

	// Chitchat tool-use numeric knobs. /chat now runs a bounded
	// 2-round ReAct loop with the recall_memory tool when memory
	// is wired; these caps shape the LLM's `limit` parameter.
	// nil → code default in types.DefaultChitchatSettings.
	ChitchatRecallDefaultLimit *int `yaml:"chitchat_recall_default_limit"`
	ChitchatRecallMaxLimit     *int `yaml:"chitchat_recall_max_limit"`

	// Chitchat list_memory tool caps. Mirror the recall_* pair but
	// for the browse-mode list tool: ListDefaultLimit is the default
	// when the LLM omits `limit`, ListMaxLimit clamps user-supplied
	// values. nil → code default in types.DefaultChitchatSettings
	// (10 / 30, matching the historical hardcoded literals).
	ChitchatListDefaultLimit *int `yaml:"chitchat_list_default_limit"`
	ChitchatListMaxLimit     *int `yaml:"chitchat_list_max_limit"`

	// env_recommend (environment diagnosis + install recommendation
	// pipeline). All optional; nil → code default in
	// types.DefaultEnvRecommendSettings.
	//
	// EnvRecommendEnabled is the master switch. false → fall back
	// to the legacy hardcoded `runnerInstallHint` strings.
	EnvRecommendEnabled       *bool `yaml:"env_recommend_enabled"`
	EnvRecommendLLMEnabled    *bool `yaml:"env_recommend_llm_enabled"`
	EnvRecommendLLMTimeoutSec *int  `yaml:"env_recommend_llm_timeout_sec"`
	RecommendGlobalInstall    *bool `yaml:"recommend_global_install"`
	EnvProbeNetwork           *bool `yaml:"env_probe_network"`
	EnvCacheTTLDays           *int  `yaml:"env_cache_ttl_days"`

	// CGEC (Citation-Grounded Evidence Closure) tunables. All
	// optional; nil → code default in
	// orchestrator.cgecForcedReadsPerRound /
	// cgecStallThresholdSoft / cgecStallThresholdHard.
	CGECForcedReadsPerRound *int `yaml:"cgec_forced_reads_per_round"`
	CGECStallThresholdSoft  *int `yaml:"cgec_stall_threshold_soft"`
	CGECStallThresholdHard  *int `yaml:"cgec_stall_threshold_hard"`

	// Phase1-unread pre-complete gate. Session 12. When the LLM calls
	// emit_investigation_complete and the explorer's keyword-search
	// top-K ranked files still have >= min_unread entries missing
	// from ReadSet AND the declared RequirementKind is a breadth-
	// intent, the gate downgrades the call and forces a follow-up
	// read round. Defaults: top_k=5, min_unread=2. Set top_k=0 to
	// disable.
	CGECPhase1UnreadTopK      *int `yaml:"cgec_phase1_unread_top_k"`
	CGECPhase1UnreadMinUnread *int `yaml:"cgec_phase1_unread_min_unread"`

	// Multi-path anchor-driven verification (replaces the retired
	// 2026-05-01 cgec_multi_path_coverage_parity_floor). The pre-
	// complete gate now decides per primary-anchor file via FIVE
	// layered signals, evaluated in internal/tool/multipath/:
	//
	//   L1. Symbol-anchored — every question-related symbol defined
	//       in the file has [def_line ± SymbolContextLines] covered
	//       → bypass. (Code files with repomap symbols.)
	//   L2. Grounded-evidence — ≥ MinGroundedPerAnchor grounded
	//       (Tier 1 / Tier 2) evidence items have been emitted from
	//       the file → bypass.
	//   L3. Keyword-anchored — non-symbolic files (no repomap
	//       symbols) where prior grep matched a question entity:
	//       each match line becomes a [L ± KeywordContextLines]
	//       anchor → demand the missing slivers (surgical).
	//   L4. Small-file full-read — non-symbolic files whose
	//       FileTotalLines <= SmallFileThreshold demand the unread
	//       slivers of the WHOLE file (bounded by file size; covers
	//       typical configs / READMEs / Dockerfiles where the answer
	//       might be on any line and there is no other anchor).
	//   L5. Opaque advisory — large non-symbolic files with no other
	//       signal raise a NON-BLOCKING advisory hint (does NOT
	//       enqueue a PendingRead).
	//
	// Why the rewrite: the predecessor gate demanded `total_lines *
	// 0.3` for every primary-anchor file, which on a 4 368-line
	// repl.go meant 1 311 lines of pagination even when the answer
	// lived in four ~30-line symbol regions totalling ~120 lines.
	// Reading thousands of irrelevant lines wasted budget AND
	// poisoned the LLM context with noise. The replacement targets
	// answer-bearing regions instead of file size, and handles
	// non-code files (config / docs / manifests) via L3-L5 instead
	// of leaving them silently uncovered.
	//
	// Defaults:
	//   MinGroundedPerAnchor=2, SymbolContextLines=15,
	//   KeywordContextLines=15, SmallFileThreshold=300.
	//
	// Disable knobs:
	//   MinGroundedPerAnchor=0  → disables L2.
	//   SmallFileThreshold=0    → disables L4 (small files fall to L5
	//                             advisory unless L3 fires).
	CGECMultiPathMinGroundedPerAnchor   *int `yaml:"cgec_multi_path_min_grounded_per_anchor"`
	CGECMultiPathSymbolContextLines     *int `yaml:"cgec_multi_path_symbol_context_lines"`
	CGECMultiPathKeywordContextLines    *int `yaml:"cgec_multi_path_keyword_context_lines"`
	CGECMultiPathSmallFileThreshold     *int `yaml:"cgec_multi_path_small_file_threshold"`
	CGECMultiPathMaxKeywordAnchorsPerFile *int `yaml:"cgec_multi_path_max_keyword_anchors_per_file"`

	// CGECExternalArtifactDecodedFloor is the minimum fraction of
	// triaged-bundle non-path tokens (LogBundle Errors[].Type +
	// Frame.Symbol + Signal name + Cause-chain; PerfBundle
	// trigger_span + stall.symbol + jank.reason + startup.mode)
	// that the finalizer's draft answer text must reference.
	// Default 0.4 (~40%); set to 0 to disable the gate (read-mode
	// runs without --log / --htrace are unaffected either way
	// because the criterion is structurally vacuous when no bundle
	// is attached).
	//
	// The gate's purpose: when the user pastes a runtime log /
	// perf trace and the system extracts a typed payload from it
	// (Errors / Frames / Stalls / etc.), the answer must DECODE /
	// EXPLAIN that payload — not just cite repo file:line and
	// silently drop the diagnostic specifics the user actually
	// pasted. Pre-2026-05-02 the contract checker had no awareness
	// of the bundle, so the LLM could ship an answer that ignored
	// SIGSEGV / addr=0x0 / parameter-value evidence in a panic.
	CGECExternalArtifactDecodedFloor *float64 `yaml:"cgec_external_artifact_decoded_floor"`

	// Log-triage knobs. `log_triage_*` prefix groups the log-ingestion
	// feature settings. When log_triage_enabled=false the log_triage
	// pre-stage Guard short-circuits and every downstream consumer
	// sees a nil bundle (the --log / /log commands silently set an
	// unused AttachedLog). log_triage_source_prefix mirrors the
	// --log-source-prefix CLI flag for users who prefer persistent
	// config over CLI flags; the CLI flag wins when both are set.
	// Additional knobs tune the LLM-driven extractor and its two-
	// step fallback:
	//
	//   LogTriageMinBytes        — skip when len(AttachedLog) < N (default 50)
	//   LogTriageMaxRetries      — per-stage fail-loud retry budget (default 1)
	//   LogTriageTwoStepEnabled  — toggle two-step fallback (default true)
	//   LogTriageTwoStepBytes    — straight-to-two-step byte threshold (default 32 KB)
	//   LogTriageTwoStepCoverage — single-shot coverage floor before escalating (default 0.3)
	//   LogTriageMaxLLMCalls     — hard cap on total LLM calls per stage run (default 12)
	//
	// All tuning knobs are pointer-typed so the merge preserves
	// "absent vs explicit zero" semantics — cmd/root.go fills missing
	// fields with DefaultLogTriageSettings values.
	LogTriageEnabled         *bool    `yaml:"log_triage_enabled"`
	LogTriageSourcePrefix    *string  `yaml:"log_triage_source_prefix"`
	LogTriageMinBytes        *int     `yaml:"log_triage_min_bytes"`
	LogTriageMaxRetries      *int     `yaml:"log_triage_max_retries"`
	LogTriageTwoStepEnabled  *bool    `yaml:"log_triage_two_step_enabled"`
	LogTriageTwoStepBytes    *int     `yaml:"log_triage_two_step_bytes"`
	LogTriageTwoStepCoverage *float64 `yaml:"log_triage_two_step_coverage"`
	LogTriageMaxLLMCalls     *int     `yaml:"log_triage_max_llm_calls"`

	// Perf-triage knobs (perf_triage_*). Exact shape mirror of
	// log_triage_* so an operator who tuned log triage transfers
	// every assumption directly. Same nil-pointer semantics for the
	// "absent vs explicit zero" distinction. Defaults supplied by
	// agent.DefaultPerfTriageSettings.
	//
	//   PerfTriageEnabled         — toggle the perf_triage pre-stage (default true)
	//   PerfTriageMinBytes        — minimum trace size before the stage fires (default 200)
	//   PerfTriageMaxRetries      — fail-loud retry budget for emit_perf_trace (default 1)
	//   PerfTriageTwoStepEnabled  — toggle two-step fallback (default true)
	//   PerfTriageTwoStepBytes    — straight-to-two-step byte threshold (default 64 KB)
	//   PerfTriageTwoStepCoverage — single-shot coverage floor before escalating (default 0.3)
	//   PerfTriageMaxLLMCalls     — hard cap on total LLM calls per stage run (default 12)
	PerfTriageEnabled         *bool    `yaml:"perf_triage_enabled"`
	PerfTriageMinBytes        *int     `yaml:"perf_triage_min_bytes"`
	PerfTriageMaxRetries      *int     `yaml:"perf_triage_max_retries"`
	PerfTriageTwoStepEnabled  *bool    `yaml:"perf_triage_two_step_enabled"`
	PerfTriageTwoStepBytes    *int     `yaml:"perf_triage_two_step_bytes"`
	PerfTriageTwoStepCoverage *float64 `yaml:"perf_triage_two_step_coverage"`
	PerfTriageMaxLLMCalls     *int     `yaml:"perf_triage_max_llm_calls"`

	// Log-attach knob. `log_attach_*` prefix groups the ingestion
	// caps that fire BEFORE log_triage sees the payload. Applies to
	// every attach surface — `--log <file>`, `--log -` (stdin),
	// `--log-text <inline>`, REPL `/log <path>`, REPL `/log` paste
	// mode, and the REPL auto-route pasted-log detector. Fires even
	// when log_triage_enabled=false; the cap is about memory safety,
	// not triage quality.
	//
	//   LogAttachMaxBytes — hard ceiling on the attached-log byte
	//                       length. Oversize payloads truncate to
	//                       the first N bytes with a WARN log line;
	//                       stdin uses io.LimitReader(N+1) so the
	//                       process never buffers more than N+1
	//                       bytes even for multi-GB pipes. Default:
	//                       1 MB (1048576). Raising this lets you
	//                       attach larger logs but multiplies the
	//                       downstream LLM token cost linearly.
	//                       Values ≤ 0 fall back to the default.
	LogAttachMaxBytes *int `yaml:"log_attach_max_bytes"`

	// TraceAttachMaxBytes is the perf-channel companion to
	// LogAttachMaxBytes. Bounds every --htrace / --atrace /
	// --htrace-text / --atrace-text / REPL /htrace / /atrace
	// surface BEFORE perf_triage sees the payload. Defaults to
	// the resolved log_attach_max_bytes value (so a user who only
	// configures the log knob still gets symmetric trace handling);
	// set explicitly to override independently. Same 1 GiB hard-
	// ceiling clamp as the log cap.
	TraceAttachMaxBytes *int `yaml:"trace_attach_max_bytes"`

	// B0 write-mode knobs. `write_*` prefix groups the write-mode
	// lifecycle settings (plan / apply / verify stages). All
	// optional; nil values coerce to safe-by-default behavior so
	// pre-B0 codrax.yaml files continue to produce read-mode-only
	// behavior byte-identically.
	//
	//   WriteEnabled       — master switch. When false (default),
	//                        any --mode=plan|apply|verify is
	//                        rejected at flag-parse time. YAML-only
	//                        by design (no --write-enabled CLI flag);
	//                        deploy-time configuration, not per-run.
	//   WriteDefaultMode   — default --mode value when the CLI flag
	//                        is omitted. Legal values: "read", "plan".
	//                        "apply" / "verify" are REJECTED here
	//                        because they are inherently side-effecting
	//                        and must be opted into per-run via CLI.
	//                        Empty / nil coerces to "read" at Run
	//                        entry via PipelineMode.Normalize.
	//   WriteAutoApproval  — yaml-level default for --auto-apply.
	//                        Today unused in B0 scope (single-shot
	//                        L4 gate uses the CLI flag directly).
	//                        Reserved for REPL /approve interactive
	//                        default and batch-mode workflows post B0.
	//   WritePlanDir       — override the default .codrax/plans/
	//                        directory where ChangePlan JSONs land.
	//                        Absolute or runtime-anchor-relative;
	//                        cmd/root.go anchors non-absolute paths.
	//   WriteAutoInitRepo  — yaml-level pre-authorization for the
	//                        bare-directory scaffolding flow. When the
	//                        target dir is not a git repo (or has no
	//                        commits), worktree provisioning normally
	//                        fail-louds; with this true, the orchestrator
	//                        runs `git init` (if needed) and an empty
	//                        initial commit before `git worktree add`.
	//                        Same semantics as the CLI --auto-init-repo
	//                        flag but applied to every Run from this
	//                        deploy. REPL retains an interactive y/N
	//                        prompt as the third tier when neither
	//                        flag nor yaml is set.
	//   WriteScaffoldEnabled — yaml-level pre-authorization for the
	//                        empty-directory scaffold flow. When the
	//                        target dir contains NO source files (only
	//                        .git or .codrax may be present), the
	//                        planner refuses to dispatch unless THIS
	//                        flag is true — even when WriteAutoInitRepo
	//                        is set. The two flags are orthogonal:
	//                        WriteAutoInitRepo authorizes `git init`,
	//                        WriteScaffoldEnabled authorizes inventing
	//                        files for a 0-source-file target. CLI
	//                        equivalent: --allow-scaffold. Default
	//                        false (fail-loud on empty + no scaffold
	//                        authorization, with a clear hint).
	WriteEnabled         *bool   `yaml:"write_enabled"`
	WriteDefaultMode     *string `yaml:"write_default_mode"`
	WriteAutoApproval    *bool   `yaml:"write_auto_approval"`
	WritePlanDir         *string `yaml:"write_plan_dir"`
	WriteAutoInitRepo    *bool   `yaml:"write_auto_init_repo"`
	WriteScaffoldEnabled *bool   `yaml:"write_scaffold_enabled"`

	// REPL interactive knobs. `repl_*` prefix groups runtime tweaks
	// to the interactive prompt. Today only the paste-fold threshold
	// is exposed; more can be added without breaking users.
	//
	//   ReplPasteFoldMinChars — pastes with >= this many runes fold
	//                           to a `[Pasted text #N …]` placeholder.
	//                           Multi-line pastes fold unconditionally
	//                           regardless of length. Default 120
	//                           (~2 visual lines at typical widths;
	//                           see repl.DefaultPasteFoldMinChars).
	//                           Unit is Unicode characters, not bytes.
	ReplPasteFoldMinChars *int `yaml:"repl_paste_fold_min_chars"`

	// Pointer to providers.yaml. A single
	// `CODRAX_SETTINGS=path/to/codrax.yaml` bootstraps an entire
	// environment (dev, staging, prod) from one entry point.
	ProvidersConfig *string `yaml:"providers_config"`
}

// LoadRuntimeSettings reads path as a YAML document into a
// RuntimeSettings. The empty-but-present case (a file with no keys)
// is valid and returns a zero-value struct, which the caller treats
// as "inherit all defaults". A missing file is signaled by
// os.ErrNotExist so the caller can decide whether silence (default
// path) or a loud error (explicit --settings path) is appropriate.
//
// Migration warnings: when codrax.yaml carries deprecated/renamed
// keys (e.g. T4 renamed pipeline_max_verify_retries →
// pipeline_write_retry_budget), LoadRuntimeSettings prints a single
// stderr line per recognised legacy key naming the new name. The
// load itself still succeeds — yaml.Unmarshal silently drops
// unknown keys — so the user gets a helpful warning instead of a
// silent ignore.
func LoadRuntimeSettings(path string) (*RuntimeSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s RuntimeSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse runtime settings: %w", err)
	}
	warnLegacyKeys(path, data)
	return &s, nil
}

// warnLegacyKeys scans the raw yaml bytes for deprecated key names
// and prints a migration hint to stderr. Cheap substring scan;
// avoids a second yaml unmarshal pass.
//
// Add new entries here when renaming an exported yaml field. The
// stderr-warning channel matches how cmd/root.go surfaces other
// non-fatal config issues.
func warnLegacyKeys(path string, data []byte) {
	type rename struct {
		old, new string
	}
	renames := []rename{
		{"pipeline_max_verify_retries", "pipeline_write_retry_budget"},
	}
	body := string(data)
	for _, r := range renames {
		// Match `<key>:` at line start (yaml convention) so a string
		// literal containing the old name in a comment/value doesn't
		// false-positive.
		needle := "\n" + r.old + ":"
		if strings.Contains("\n"+body, needle) {
			fmt.Fprintf(os.Stderr,
				"[config] %s: deprecated yaml key %q renamed to %q (see CHANGELOG); old key is silently ignored\n",
				path, r.old, r.new)
		}
	}
}

// IsNotExist is re-exported so main.go can keep its runtime-config
// branching self-contained without importing os just for this check.
func IsNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }
