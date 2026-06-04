package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aymanbagabas/go-udiff"
	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/analysis/stopcond"
	"github.com/hanchaoqun/codrax/internal/config"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Pipeline-budget ceiling defaults are now sourced from the
// resolved AgentSettings / PipelineSettings. The historical
// `plannerScaledIterMax = 20` literal lives in
// AgentSettings.PlannerScaledIterMax (yaml `agent_planner_scaled_iter_max`).
// Mirror knobs cover the explorer (35), pipeline-step ceil (100),
// extractor (8), verifier (12), prescan rounds (4), retry budget (5),
// and the perf/log triager iter caps (6 each).

// Orchestrator is the Layer 1 component that drives the pipeline state
// machine. It walks the hardcoded 4-stage topology (see topology.go),
// manages BusContext, and dispatches agents.
type Orchestrator struct {
	settings              types.PipelineSettings
	agents                *agent.Registry
	skills                *skill.Registry
	busCtx                *types.BusContext
	maxSteps              int
	subRuntime            *agent.SubAgentRuntime
	language              string
	emit                  render.EventEmitter
	thinkAloudMap         map[types.AgentName]bool // per-agent think-aloud override
	blobSessionDir        string                   // persistent per-process blob dir; empty = tmpdir fallback
	attachedLog           string                   // runtime log excerpt attached via --log / /log
	attachedHitrace       string                   // HiTrace / atrace excerpt attached via --htrace / /htrace
	attachedHitraceSource string                   // advisory trace flavor/source hint from --htrace/--atrace spelling
	// presentationDirective is a per-run typed display requirement
	// from the REPL turn policy. It is intentionally not concatenated
	// into the objective string, because the objective feeds status
	// lines, repo_map task-map queries, and memory.
	presentationDirective string

	// turnRouteHint is per-run typed routing metadata produced before
	// read-mode analysis starts. Like presentationDirective, it is
	// consume-once so REPL turns cannot leak route posture into the
	// next request.
	turnRouteHint types.TurnRouteHint

	// outputDumpDir is the absolute directory final-answer markdown
	// transcripts are written into. Empty string disables the dump
	// entirely (cmd/root.go leaves it empty when output_dump_enabled
	// is false). outputDumpMax bounds retention by file count; the
	// dump helper prunes the oldest files past this cap before writing
	// the new one.
	outputDumpDir           string
	outputDumpMax           int
	outputTranscriptRequest string

	// memoryReader is the read handle into the REPL memory store.
	// Wired by cmd/root.go via SetMemoryReader after the Store is
	// constructed; nil in single-shot CLI / non-REPL test fixtures.
	// Run() copies it into BusContext.Memory so tools (recall_memory)
	// can query without a memory-package import.
	memoryReader types.MemoryReader

	// envSettings is the resolved env_recommend yaml config. Run()
	// uses it to decide whether to probe + populates
	// BusContext.EnvRecommendSettings so tools can gate.
	envSettings types.EnvRecommendSettings

	// multiGraphProvider returns the *multigraph.MultiGraph that the
	// upcoming Run should attach to BusContext. Wired by cmd/root.go
	// via SetMultiGraphProvider; nil in single-shot tests / fixtures.
	// Run() consults it at entry — REPL state changes (focus / cap)
	// take effect on the next Run because the provider closes over
	// the latest mutable state.
	//
	// Stored as func() any to keep types/orchestrator free of the
	// import cycle the multigraph package would otherwise create
	// (multigraph already imports types). Run() stashes the result
	// on BusContext.MultiGraph as-is.
	multiGraphProvider func() any

	// multiRepoSnapshotProvider supplies the BusContext-shape
	// snapshot of the topology + cap-trimmed pending list. Wired by
	// cmd/root.go alongside multiGraphProvider; both consulted at
	// every Run entry.
	multiRepoSnapshotProvider func() ([]types.SubRepoSnapshot, []string)

	// multiRepoInactivePreviewCount is the L0-advisory cap stamped on
	// every BusContext at Run entry. cmd/root.go calls
	// SetMultiRepoInactivePreviewCount once after yaml load.
	multiRepoInactivePreviewCount int
	// mode controls the B0 write-mode dispatch in Run(). Zero value
	// ("") is treated as ModeRead by busCtx.Mode.Normalize at Run
	// entry, so every pre-B0 caller sees identical read-only
	// behavior. Set by the CLI / REPL layer via SetMode; immutable
	// for the lifetime of a single Run().
	mode types.PipelineMode

	// planPath is the absolute path of an existing ChangePlan
	// JSON file that the apply stage hook should load before dispatching
	// the coder agent. Populated via SetPlanPath (called by
	// cmd/root.go from the --plan-file flag value). Empty when
	// Mode is ModeRead or ModePlan (plan is produced, not consumed,
	// in those modes). Copied into BusContext.PlanPath at Run entry
	// so the phase functions read a single authoritative source.
	planPath string

	// worktreeBase is the directory root under which the apply stage hook
	// asks worktree.Create to provision new worktree sessions.
	// Typically <CWD>/.codrax/worktrees — cmd/root.go computes it
	// during initApp and passes it here via SetWorktreeBase.
	// Empty disables the worktree provisioning path: the apply stage hook
	// refuses to dispatch without a base dir so a misconfigured
	// install surfaces a clean error rather than silently writing
	// patches into the main repo.
	worktreeBase string

	// writeRetryBudget caps the number of verify→plan retry cycles
	// within ModeApply. Default 0 preserves fail-loud semantics
	// (one attempt, surface failure). Values >0 enable the T4
	// verify→plan loop: a failed verify SuccessCriteria fires
	// EdgeValidationFeedback to requeue the plan node, seeds
	// PlanningHint with the failure narrative, and re-dispatches
	// the planner for a revised ChangePlan. Capped at 5 defensively
	// so an adversarial test harness cannot burn the LLM token
	// budget on an unfixable plan.
	writeRetryBudget int

	// writeApprovalPolicy is the write-lane approval policy consumed
	// when ModeApply generates a fresh ChangePlan inside the scheduler
	// (retry or multi-phase). User-reviewed plan files skip the first
	// plan visit, so this gate does not reinterpret an already approved
	// plan. It only prevents newly generated high/critical batches from
	// being applied without the typed approval policy.
	writeApprovalPolicy writeflow.ApprovalPolicy

	// transientRetryBudget caps how many times a single Run will
	// retry a stage that failed with a transient dispatch error
	// (LLM stream stall / first-byte timeout / 429 / 5xx / network
	// blip). DECOUPLED from writeRetryBudget because the budget
	// consumer semantics differ: transient retry has no learning
	// signal (same prompt re-sent, only the network attempt is
	// fresh) while SC retry has structured feedback. Sharing one
	// counter let 3 plan stalls drain the verify→plan SC budget so
	// real verify failures could not retry. Set via
	// SetTransientRetryBudget. Default 3.
	transientRetryBudget int

	// forceFinalizeAttempts caps the number of dispatch attempts
	// the force-finalize escape path makes when the previous
	// attempt errored with a transient LLM stream failure
	// (unexpected EOF, stream stalled, 429, network blip). Set via
	// SetForceFinalizeAttempts. Default 3 = 1 initial + 2 retries.
	// Hard-capped at MaxForceFinalizeAttempts (5) inside the setter.
	forceFinalizeAttempts int

	// maxUpstreamFallbacksPerRun (Block 3 architecture overhaul
	// 2026-05-02) caps the number of selective upstream fallbacks
	// (BackToExtract / BackToExplore) any single Run may consume.
	// Reaching the cap forces the next fallback to FailLoud
	// regardless of the violation kind's policy mapping. Default 2;
	// 0 disables the cap entirely (not recommended).
	maxUpstreamFallbacksPerRun int

	// reflector is the optional Reflexion-pattern critic invoked
	// from clearForReplan between a verify failure and the planner
	// re-dispatch. Nil = disabled (clearForReplan falls back to the
	// heuristic PlanningHint built by buildRetryHint). Set via
	// SetReflector — wired in cmd/root.go from providers.yaml ::
	// agents.reflector or the default LLM. See reflector.go for
	// the pattern (mirrors chitchat_classifier / memory_summarizer).
	reflector Reflector

	// planCritic is the optional pre-apply review LLM. Mirrors
	// reflector but fires earlier in the pipeline (after planner
	// emit, before apply). Nil when the operator left
	// pipeline_plan_critic_enabled false (default) — apply runs
	// straight without critique. See plan_critic.go for the
	// pattern.
	planCritic PlanCritic

	// acceptanceChecker is the per-phase verdict surface used by
	// stage II's runPhaseGroup (commit 18+). After a phase's
	// verify passes, this LLM judges whether the phase
	// satisfied its stated goal. Always installed (cheap
	// fallback to default LLM); fires only inside multi-phase
	// runs, so single-phase Runs pay nothing. See
	// acceptance_checker.go for the pattern.
	acceptanceChecker AcceptanceChecker

	// planGroupStore persists multi-phase PlanGroups to disk
	// (stage II). Wired from cmd/root.go. Nil disables stage II
	// entirely — runPhaseGroup short-circuits when the store is
	// absent, falling back to the single-phase TaskGraph path
	// even if the LLM emitted a sequential proposal. Tests bypass
	// the wiring via SetPlanGroupStore(nil) for back-compat
	// fixtures.
	planGroupStore PlanGroupSaver

	// planSaver persists per-phase ChangePlans to the same
	// PlanStore the REPL uses for single-phase plans. Without
	// this, multi-phase Runs (Mode==ModeApply, no PlanPath set)
	// emit phases whose plan files never reach disk, so /plan
	// show / /history / /approve <id> can't see them.
	// Single-phase Runs do not use this — they go through the
	// REPL's post-Run auto-save path (ModePlan only) or
	// cmd/root.go's writePlanFile (CLI). Stage II's ModeApply
	// path bypasses both, so this exists. Nil = best-effort
	// skip (back-compat with tests + CLI single-shot).
	planSaver PlanSaver

	// nextPhaseHint is the consume-once carry-over from the
	// previous phase's acceptance check (NextHint field). The
	// next phase's seedPlanningHintFromPhase reads this slot
	// and wipes it.
	nextPhaseHint string

	// failureTaxonomyStore is the per-repo learned-pitfall
	// cache (stage 3). The reflector emits patterns alongside
	// observations; stage_hooks.clearForReplan persists them
	// here. The planner's BuildInitialInstruction reads
	// RelevantTo to inject active pitfalls into the planning
	// context. Nil disables the feature (single-shot CLI
	// flows that don't want cross-Run learning).
	failureTaxonomyStore *FailureTaxonomyStore

	// answerTaxonomyStore is the read-mode mirror (commit 51
	// Gap 3): per-repo cache of distilled answer pitfalls.
	// answer_reviewer emits patterns at end-of-Run;
	// dispatchStage reads RelevantTo at analyze entry to
	// populate AgentContext.ActiveAnswerPitfalls. Nil disables
	// the feature.
	answerTaxonomyStore *AnswerTaxonomyStore

	// answerReviewer is the optional end-of-Run independent-
	// review LLM (commit 51 Gap 3). Mirror of reflector but for
	// read-mode: at successful Run end with retry events, it
	// reads the final answer + retry history and may distill an
	// answer pitfall to persist into answerTaxonomyStore. Nil =
	// feature disabled (no LLM dispatch, no taxonomy growth).
	answerReviewer AnswerReviewer

	// selfConsistencyReviewer is the commit-62 prose-coherence
	// reviewer dispatched after finalize to detect contradictions
	// between the answer's summary section and its body bullets
	// (real eval s1a-20260501-083611 surfaced the gap). Nil =
	// disabled (no review, no rewrite). Wired from cmd/root.go
	// when pipeline_self_consistency_review_enabled=true.
	selfConsistencyReviewer SelfConsistencyReviewer

	// semanticQualityReviewer (G5 post_v2_runtime_gap_remediation,
	// 2026-05-04) is the second-layer reviewer that catches "answer
	// ships clean but thin" — promoted facets uncovered, diagram
	// edge minimums short, richness candidates with available
	// evidence not surfaced. Independent from
	// selfConsistencyReviewer so each can be tuned (confidence
	// floor, prompt scope) independently. Nil = disabled.
	semanticQualityReviewer SemanticQualityReviewer

	// selfConsistencyRewriteOnContradiction (commit 62 yaml gate)
	// expresses whether contradictions are eligible for answer
	// re-dispatch. The effective decision still flows through the
	// global soft/strict violation policy: default-soft
	// ViolSelfContradiction ships with telemetry/caveat, while
	// strict-promoted deployments may trigger a real finalizer retry.
	selfConsistencyRewriteOnContradiction bool

	// selfConsistencyMinConfidence (commit 62) is the floor a
	// reviewer's self-rated confidence must reach for the verdict
	// to be acted on. Default 0.92 (P4 2026-05-10 tightening; was
	// 0.8 pre-fix). yaml override: pipeline_self_consistency_min_confidence.
	selfConsistencyMinConfidence float64

	// semanticQualityMinConfidence (P4 2026-05-10) is the symmetric
	// floor for the G5 reviewer. Default
	// SemanticQualityMinConfidenceDefault (0.92). yaml override:
	// pipeline_semantic_quality_min_confidence.
	semanticQualityMinConfidence float64

	// strictAnswerReviewDisabled gates the post-finalizer review and
	// rewrite loop. Zero value keeps commercial-grade contract
	// enforcement. True ships the first accepted structured draft and
	// renders deterministic concerns as supplementary context instead
	// of burning reviewer / rewrite rounds.
	strictAnswerReviewDisabled bool

	// finalizeRepairHardCap (P6 2026-05-10) caps the number of
	// finalize-stage repair-loop iterations a single Run will
	// attempt before degrading to "accept doc + residual-concerns
	// caveat". Independent of ExecutionPolicy.RetryBudget (which
	// can be set loosely per task graph) and FinalizerLocalRetryBudget
	// (which only caps the finalize-only-downgrade path). 0 keeps
	// the FinalizeRepairHardCapDefault (2). Forensic data (May-9
	// sweep, 26 run): ~8% of runs took ≥3 repair_exec rounds,
	// with diminishing returns; bounding at 2 caps wall-clock
	// without changing the typical-case behaviour.
	finalizeRepairHardCap int

	// exhaustiveDeterministicReviewThreshold (2026-05-16) is the
	// principal-member-count above which the deterministic
	// exhaustive member-set reviewer replaces self-consistency +
	// semantic-quality LLM dispatchers. Resolved from
	// codrax.yaml :: pipeline_exhaustive_deterministic_review_threshold;
	// 0 disables; <0 falls through to types default.
	exhaustiveDeterministicReviewThreshold int

	// reviewerWallBudgetFraction (2026-05-16) sets the fraction
	// of remaining wall budget each LLM reviewer may consume
	// before deadline guard fires. 0 keeps legacy unguarded
	// behaviour; default 0.4 from yaml.
	reviewerWallBudgetFraction float64

	// continuationClassifier is the LLM-driven decision for
	// "is the current REPL turn a continuation of the prior
	// conversation?" (commit 48 read-mode Gap 1). Pre-commit-48
	// the decision lived in types.IsContinuation's hardcoded
	// continuationPrefixes keyword list — exactly the pattern
	// feedback_no_custom_keyword_matching.md flags as anti-
	// pattern. Now resolved once per Run via this classifier;
	// the result is cached on continuationClassification so
	// per-stage priorConvVisibleForStage can read without
	// re-dispatching the LLM. Nil = fall back to the keyword
	// heuristic (graceful degradation; tests bypass the LLM).
	continuationClassifier ContinuationClassifier

	// continuationClassification caches the per-Run decision.
	// Tri-state via pointer: nil = not yet computed (the
	// keyword fallback applies), &true / &false = LLM
	// decided. Reset at Run() entry so a stale cross-Run
	// value never leaks.
	continuationClassification *bool

	// phaseContextPrefix is the sticky "## Phase X of Y: <goal>"
	// header set by seedPlanningHintFromPhase at every phase
	// entry. Distinct from PlanningHint (which is consume-once,
	// drained by planner.BuildInitialInstruction on the FIRST
	// dispatch of the phase): this slot survives the consume so
	// clearForReplan can re-prepend the phase header onto the
	// retry hint. Without this, an intra-phase verify→plan retry
	// would lose the "you are still in phase 2" context and the
	// next planner dispatch could drift toward a different phase
	// boundary. Cleared at the next phase entry.
	phaseContextPrefix string

	// runTaskPhaseFn is a test seam: production runs use the
	// real method (o.runTaskPhase) which requires the full
	// agent stack; tests substitute a stub that pre-seeds
	// Mutable.ChangePlan + Mutable.ChangeReport so the
	// runPhaseGroup outer loop becomes unit-testable without
	// stand-up of fake agent fleets. Nil = use the real
	// method (production path).
	runTaskPhaseFn func(*int) error

	// baselineCaptureEnabled gates the pre-apply test snapshot
	// that feeds CritNoRegression. Default false (test doubling
	// is opt-in). When true, the apply stage hook dispatches run_tests
	// once before the coder + moves the result to
	// Mutable.BaselineReport so evalNoRegression can diff against
	// the post-apply ChangeReport.
	baselineCaptureEnabled bool

	// baselineCache reuses cached baseline reports across Runs that
	// observe the same main-repo HEAD SHA. Cache hits bypass the
	// test-suite re-run entirely, regardless of
	// baselineCaptureEnabled — once a baseline exists, it costs
	// nothing to use. Nil when commit 2 P0 A1 wiring is absent or
	// the operator set baseline_cache_max=0. Per-runtime-anchor
	// directory layout — see baseline_cache.go.
	baselineCache *BaselineCache

	// writeMaxSeconds is the wall-clock ceiling for write-mode
	// Runs. When > 0, Run() arms a deadline timer at write-mode
	// entry that cancels the in-flight Run when the timer fires
	// (LastError surfaces "write mode wall-time exceeded"). Read-
	// mode Runs are unaffected — the timer is only armed when Mode
	// is plan / apply / verify. Hard-capped at 1800 seconds inside
	// SetWriteMaxSeconds. Default 0 (no cap, legacy behaviour);
	// cmd/root.go sets 600 from yaml.
	writeMaxSeconds int

	// keepWorktreeOnSuccess, when true, skips the post-Run worktree
	// discard if the Run finished a successful ModeApply (apply +
	// verify both passed). Default false preserves historical "Run
	// ends, worktree gone" behaviour. When true, the user gets the
	// worktree path in the Result so they can `cd` there, review
	// the applied bytes, and cherry-pick / rebase to main manually
	// — the "try before merge" workflow.
	//
	// Failure paths always discard regardless of this flag so a
	// misbehaving planner cannot accumulate broken worktrees on
	// disk. Only the happy path is preserved.
	keepWorktreeOnSuccess bool

	// skipVerify, when true, short-circuits the verify stage in
	// ModeApply: the apply node still runs (bytes land in the
	// worktree), but the write scheduler marks the verify node
	// done without dispatching the verifier agent. Used by REPL
	// `/approve --skip-verify` for cases where the operator can't
	// run integration tests locally (DB / GPU / external API)
	// and prefers to defer testing to CI on push.
	//
	// Scope: per-Run. The REPL caller defers SetSkipVerify(false)
	// after Run() returns so the override doesn't leak across
	// /approve invocations against different plans.
	skipVerify bool

	// autoInitRepo, when true, lets the apply pre-hook silently
	// transition a bare or commitless target through worktree.
	// EnsureInitialCommit before calling worktree.Create. Default
	// false — the pre-hook fail-louds with a hint at the three
	// authorization surfaces (CLI --auto-init-repo, yaml
	// write_auto_init_repo, REPL interactive consent). Set via
	// SetAutoInitRepo from cmd/root.go's flag/yaml resolver.
	//
	// SEMANTIC: this flag authorizes git initialization ONLY. It does
	// NOT authorize codrax to invent files for an empty target dir —
	// that is the separate scaffoldEnabled flag below. The two are
	// orthogonal: a non-empty bare dir (existing source, no .git)
	// only needs autoInitRepo; an empty dir from-scratch scaffold
	// needs BOTH.
	autoInitRepo bool

	// scaffoldEnabled, when true, authorizes the planner to produce a
	// ChangePlan for a target directory that contains NO existing
	// source files. Default false — without explicit authorization,
	// planPreHook fails fast on empty target dirs even when
	// autoInitRepo is set, because "make my empty dir into a project"
	// is a more aggressive operation than "git init my dir" and
	// users routinely conflated the two before this gate existed.
	//
	// Yaml: write_scaffold_enabled. CLI: --allow-scaffold. The flag
	// is checked ONLY when planPreHook detects an effectively-empty
	// dir (no source files outside .git/.codrax); non-empty targets
	// bypass it because the planner has real code to read.
	scaffoldEnabled bool

	// reuseWorktreePath, when set, tells the verify pre-hook to swap
	// busCtx.RepoRoot to this existing path INSTEAD of creating a
	// fresh worktree. Used by REPL `/verify <plan-id>` to re-verify
	// against the worktree the original apply preserved
	// (pipeline_keep_worktree_on_success). The path is NOT mirrored
	// onto busCtx.WorktreePath — that would cause the outer cleanup
	// defer to discard the preserved tree on Run exit, defeating the
	// preservation. Empty disables the override (default).
	reuseWorktreePath string

	// emitStageRetryAttempt is set by retry-aware phase loops (today:
	// runAnalyzePhase) before each dispatchStage call so the agent
	// layer can activate terminal forcing — literal tool-call template
	// in the prompt + named-function tool_choice on the wire — when
	// attempt > 0. Read once and cleared by dispatchStage so callers
	// without an explicit retry counter (most stages) keep attempt=0.
	// Internal field; not part of the public API.
	emitStageRetryAttempt int

	// reportDir is captured at Run entry from the first non-empty
	// busCtx.PlanPath (or the orchestrator's planPath flag) and
	// preserved across the verify→plan retry loop, so
	// saveChangeReport can persist post-retry reports even after
	// clearForReplan wipes busCtx.PlanPath. Without this, a
	// successful retry's ChangeReport never lands on disk and the
	// authoritative report.json keeps the failed first iteration's
	// data.
	//
	// Bug provenance: Batch L forth-py — Fix 1's restoreBestIfRegressed
	// correctly restored Mutable.ChangeReport to 51/54 (the best
	// iteration), but saveChangeReport saw an empty PlanPath and
	// skipped the write, leaving 48/54 (the FIRST iteration) on
	// disk. Operators reading runs/<id>.report.json saw a stale
	// FAIL when the in-memory state reflected a strictly-better PASS.
	reportDir string

	// currentIterCommitSHA is the worktree-git HEAD SHA captured
	// after the most recent apply stage committed its changes. Set
	// by applyPostHook on a successful coder run; cleared at Run
	// entry. Read by clearForReplan to track the SHA against the
	// best slot.
	currentIterCommitSHA string

	// bestAppliedCommitSHA is the worktree-git HEAD SHA of the
	// best-known-good iteration's applied content. Updated whenever
	// SetBestPlanReport accepts a new best, in lockstep with
	// Mutable.bestPlan / Mutable.bestReport.
	//
	// Load-bearing for the warm-worktree retry pattern: when the
	// next iteration's clearForReplan runs, instead of discarding
	// the worktree and resetting RepoRoot back to the original stub
	// at MainRepoRoot, it `git reset --hard <bestAppliedCommitSHA>`
	// the existing worktree so the planner's next dispatch reads
	// the BEST iteration's applied code as its starting baseline.
	// This converts a 4-iteration retry budget from "4 independent
	// from-stub attempts, take max" into "4 iterative refinement
	// attempts on top of the running best" — the difference between
	// taking max(LLM tries) and converging by patching.
	//
	// Empty when no iteration has produced a verify-eligible
	// ChangeReport yet (e.g., apply-only failures, mid-iter-0
	// regressions). In that case clearForReplan falls back to its
	// historical "discard + reset to main" behavior.
	bestAppliedCommitSHA string

	// cancelToken backs the user-driven Run cancellation surface.
	// Allocated at Run() entry; nil between Runs (tests / single-shot
	// CLI never need to interact with it). REPL holds an
	// orchestrator-side handle exposed via Cancel(reason); the
	// pipeline polls IsCanceled() at well-known checkpoints
	// (dispatchStage front, agent loop top, before tool exec) and
	// unwinds with a CanceledError.
	cancelToken *CancelToken
}

// New creates a new Orchestrator.
func New(settings types.PipelineSettings, agents *agent.Registry, skills *skill.Registry, subAgents *agent.SubAgentRegistry) *Orchestrator {
	subRuntime := agent.NewSubAgentRuntime(subAgents)
	subRuntime.SetMaxParallelism(settings.MaxParallelism)
	return &Orchestrator{
		settings:            settings,
		agents:              agents,
		skills:              skills,
		maxSteps:            50,
		subRuntime:          subRuntime,
		emit:                render.NopEmitter,
		writeApprovalPolicy: writeflow.ApprovalPolicyAutoSafe,
	}
}

// SetEmitter attaches an event emitter for real-time CLI rendering.
// Must be called before Run(). Passing nil restores the no-op default.
func (o *Orchestrator) SetEmitter(emit render.EventEmitter) {
	if emit == nil {
		emit = render.NopEmitter
	}
	o.emit = emit
}

// EmitMultiRepoScanNotice surfaces a per-sub-repo scan-progress
// notice through the orchestrator's event emitter. Phase 4 (2026-
// 05-08) calls this from MultiGraph.EnsureLoaded via the
// multiRepoScanNotifier callback so the dock + log render a
// transient append-only line for each scan start / end.
//
// Safe to call before SetEmitter: emit defaults to render.NopEmitter,
// so the call quietly drops if the emitter has not been wired yet
// (single-shot tests / partial-init paths).
//
// 2026-05-08 fix (Phase 4 lang regression): read o.language directly
// instead of o.busCtx.Language. The routing fold runs at Run entry
// BEFORE Run sets `o.busCtx.Language = o.language` (orchestrator.go:
// 1476), so scans triggered by the routing fold's EnsureMany — i.e.
// the operator's pinned sub-repos — fired with an empty
// busCtx.Language and dropped to the English fallback even when
// --lang=zh. Mid-Run scans (post-line-1476) saw the populated
// busCtx.Language and rendered Chinese correctly. The mixed-language
// surface hid two issues at once. Reading o.language (set once at
// SetLanguage time and stable across the orchestrator's lifetime)
// removes the dependency on per-Run timing.
func (o *Orchestrator) EmitMultiRepoScanNotice(rootRel, slug string, started, ok bool, elapsedMs int64) {
	if o == nil || o.emit == nil {
		return
	}
	lang := o.language
	if lang == "" && o.busCtx != nil {
		// Defensive fallback for any caller that constructs an
		// Orchestrator without SetLanguage. Should not fire in
		// production paths because cmd/root.go always calls
		// SetLanguage before any Run.
		lang = o.busCtx.Language
	}
	var noticeKind render.OrchestratorNoticeKind
	var msg string
	switch {
	case started:
		noticeKind = render.NoticeMultiRepoScanStart
		msg = multiRepoScanStartMessage(lang, rootRel)
	case ok:
		noticeKind = render.NoticeMultiRepoScanOK
		msg = multiRepoScanEndMessage(lang, rootRel, elapsedMs, true)
	default:
		noticeKind = render.NoticeMultiRepoScanFail
		msg = multiRepoScanEndMessage(lang, rootRel, elapsedMs, false)
	}
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		Stage:      o.currentNoticeStage(),
		NoticeKind: noticeKind,
		Reasoning:  msg,
	})
}

// EmitRepoMapScanNotice surfaces single-repo repomap scan progress.
// The scan often happens before EventPipelineStart while analyzer
// context is being prepared, so this path intentionally goes through
// the already-wired emitter instead of waiting for a stage row.
func (o *Orchestrator) EmitRepoMapScanNotice(ev types.RepoMapScanEvent) {
	if o == nil || o.emit == nil {
		return
	}
	lang := o.language
	if lang == "" && o.busCtx != nil {
		lang = o.busCtx.Language
	}
	noticeKind := render.NoticeRepoMapScanStart
	switch {
	case ev.Finished && ev.OK:
		noticeKind = render.NoticeRepoMapScanOK
	case ev.Finished:
		noticeKind = render.NoticeRepoMapScanFail
	case ev.Progress:
		noticeKind = render.NoticeRepoMapScanProgress
	}
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		Stage:      o.currentNoticeStage(),
		NoticeKind: noticeKind,
		Reasoning:  repoMapScanMessage(lang, ev),
	})
}

func (o *Orchestrator) currentNoticeStage() types.PipelineStage {
	if o == nil || o.busCtx == nil {
		return ""
	}
	if o.busCtx.PipelineStage != "" {
		return o.busCtx.PipelineStage
	}
	return o.busCtx.TaskState.Stage
}

// SetMaxSteps overrides the maximum number of pipeline steps (default 50).
func (o *Orchestrator) SetMaxSteps(n int) {
	o.maxSteps = n
}

// SetLanguage configures the default response language injected into
// every agent's system prompt via BusContext.Preferences. The empty
// string, "off", and "none" disable the injection so the pipeline
// behaves exactly as before. Any other value is passed through to
// languageDirective which maps well-known codes to explicit wording.
func (o *Orchestrator) SetLanguage(lang string) {
	o.language = lang
}

// SetThinkAloudMap installs the per-agent think-aloud overrides
// resolved from providers.yaml. Keys are agent names; values are
// the resolved boolean. Agents not in the map inherit the default.
func (o *Orchestrator) SetThinkAloudMap(m map[types.AgentName]bool) {
	o.thinkAloudMap = m
}

// SetBlobSessionDir installs the per-process blob session directory
// (typically <CWD>/.codrax/blob/<timestamp>-<pid>/) created by cmd/root.go.
// When non-empty, Run() uses it directly as BusContext.WorkDir and
// skips the per-trace cleanup — the session directory is shared across
// every Run() made by this process and pruned by the next startup's
// tool.PruneBlobSessions sweep, mirroring the log retention policy.
// Empty restores the historical per-trace os.MkdirTemp + RemoveAll
// behavior (used by tests, and when blob_max_sessions=0 disables the
// persistent layout).
func (o *Orchestrator) SetBlobSessionDir(dir string) {
	o.blobSessionDir = dir
}

// SetOutputDump configures the per-Run final-answer transcript dump.
// `dir` is the absolute output directory (typically
// <CWD>/.codrax/output/); empty disables the feature. `max` bounds
// retention — the dump helper deletes the oldest *.md files in the
// directory until count <= max-1 before writing a new file, so the
// directory never grows past `max` entries. Non-positive `max` is
// treated as "no cap" by the helper but cmd/root.go always passes a
// concrete default (10), so the no-cap branch is reserved for tests.
func (o *Orchestrator) SetOutputDump(dir string, max int) {
	o.outputDumpDir = dir
	o.outputDumpMax = max
}

// SetOutputTranscriptRequest installs the exact current-turn user
// request to render in final-answer markdown/html transcripts. REPL
// uses this to pass the paste-expanded request while keeping memory's
// folded display text compact. Empty restores the historical fallback
// that derives the request from Mutable.Objective().
func (o *Orchestrator) SetOutputTranscriptRequest(request string) {
	o.outputTranscriptRequest = request
}

// SetMemoryReader wires the REPL memory store's read handle so each
// Run's BusContext gets it propagated. Single-shot CLI / tests may
// leave this unset; tools that depend on it nil-check before use.
func (o *Orchestrator) SetMemoryReader(m types.MemoryReader) {
	o.memoryReader = m
}

// SetEnvRecommendSettings stashes the resolved yaml config so each
// Run's BusContext receives it. cmd/root.go calls this once after
// loading codrax.yaml.
func (o *Orchestrator) SetEnvRecommendSettings(s types.EnvRecommendSettings) {
	o.envSettings = types.ResolvedEnvRecommendSettings(s)
}

// SetMultiGraphProvider installs the multi-repo carrier provider.
// Run() consults the provider at every Run entry — REPL mutations
// (/repos focus, /repos cap, /repos refresh) take effect on the
// next Run because the closure captures the latest mutable state.
//
// `provider` returns the *multigraph.MultiGraph as `any` to keep
// orchestrator free of the import cycle that would otherwise arise
// (multigraph already imports types/SymbolOracle). Pass nil to
// disable multi-repo wiring; Run falls back to legacy single-graph
// behaviour.
func (o *Orchestrator) SetMultiGraphProvider(provider func() any) {
	o.multiGraphProvider = provider
}

// SetMultiRepoSnapshotProvider installs the BusContext-shape
// snapshot provider. Run() copies the result into BusContext.SubRepos +
// PendingSubRepos so non-MultiGraph-aware consumers (REPL renderer,
// telemetry, write-mode fail-loud paths) read multi-repo state
// without a *multigraph.MultiGraph reference.
func (o *Orchestrator) SetMultiRepoSnapshotProvider(provider func() ([]types.SubRepoSnapshot, []string)) {
	o.multiRepoSnapshotProvider = provider
}

// SetMultiRepoInactivePreviewCount stamps the LLM-advisory cap that
// context/builder.go reads when composing the L0 active-set advisory
// section. Default 0 → builder falls back to
// config.MultiRepoInactivePreviewCountDefault. cmd/root.go calls this
// once at orchestrator construction with the clamped yaml value.
func (o *Orchestrator) SetMultiRepoInactivePreviewCount(n int) {
	o.multiRepoInactivePreviewCount = n
}

// Cancel marks the in-flight Run as canceled with the given reason.
// Safe to call from any goroutine (the REPL's signal handler runs in
// its own goroutine). Idempotent: subsequent calls preserve the FIRST
// reason so a "Ctrl+C" cannot be overwritten by a follow-on
// "/cancel". No-op when no Run is in flight (idle Orchestrator).
//
// The cancellation lands at the next pipeline checkpoint
// (dispatchStage front, agent loop top, before tool exec). Worst-case
// delay is the duration of the currently-running LLM Chat call —
// Phase 1 by design accepts that latency in exchange for a tiny
// surface area; Phase 2 will plumb context.Context through the LLM
// adapter for immediate HTTP-level cancellation.
func (o *Orchestrator) Cancel(reason string) {
	if o == nil || o.cancelToken == nil {
		return
	}
	o.cancelToken.Cancel(reason)
}

// IsCanceled reports whether a Run is in flight AND has been canceled.
// Used by the REPL to drive the "✗ canceled" rendering path on top
// of the standard Run() return. Cheap atomic read; safe under any
// concurrent caller pattern.
func (o *Orchestrator) IsCanceled() bool {
	return o != nil && o.cancelToken != nil && o.cancelToken.IsCanceled()
}

// checkCanceled is the internal hot-path helper. Returns a populated
// CanceledError when the current Run has been canceled, else nil.
// Used by dispatchStage / runTaskGraph / agent loop checkpoints.
func (o *Orchestrator) checkCanceled(stage string, iter int) error {
	if o == nil || o.cancelToken == nil || !o.cancelToken.IsCanceled() {
		return nil
	}
	return &CanceledError{
		Reason:  o.cancelToken.Reason(),
		AtStage: stage,
		Iter:    iter,
	}
}

// CancelChecker returns the cancel-probe callback wired into agent
// Dependencies.CancelChecker. The agent loop polls it at iteration
// boundaries and tool dispatches; a non-nil return unwinds the loop
// with the same CanceledError shape dispatchStage emits, so the
// REPL's "✗ canceled" rendering path is uniform regardless of which
// checkpoint detected the cancel.
//
// Stage label is "agent_loop" because the agent layer doesn't know
// which pipeline stage owns its dispatch — orchestrator-level
// checkpoints (dispatchStage) carry the precise stage. The user-
// facing message picks the most specific label observed.
func (o *Orchestrator) CancelChecker() func() error {
	if o == nil {
		return nil
	}
	return func() error {
		return o.checkCanceled("agent_loop", 0)
	}
}

// CancelContext returns the cancellation-aware context.Context
// backing the current Run's CancelToken. HTTP-level callers (LLM
// Adapter, exec.CommandContext, worktree git operations) derive
// from this ctx so Cancel produces immediate interruption instead
// of waiting for a cooperative checkpoint. Returns context.TODO()
// when no Run is in flight or no token is allocated — callers can
// always derive from the returned ctx without nil-checking.
func (o *Orchestrator) CancelContext() context.Context {
	if o == nil || o.cancelToken == nil {
		return context.TODO()
	}
	return o.cancelToken.Context()
}

// SetAttachedLog stores a runtime log excerpt (panic, exception stack,
// sanitizer diagnostic, traceback) that every subsequent Run() should
// attach to BusContext.AttachedLog so the log_triage pre-stage
// can extract stack-frame anchors.
//
// REPL sticky lifetime: the REPL's /log command sets this once and it
// persists across turns until the REPL's /log clear command passes
// an empty string to reset it. CLI single-shot mode calls it at most
// once with the --log / --log-text payload before the single Run().
// Empty string clears any previously attached log.
func (o *Orchestrator) SetAttachedLog(log string) {
	o.attachedLog = log
}

// AttachedLog returns the current attached-log payload. Read surface
// for the REPL's /log show handler.
func (o *Orchestrator) AttachedLog() string {
	return o.attachedLog
}

// SetAttachedHitrace stores a HarmonyOS HiTrace / Android systrace
// excerpt that every subsequent Run() attaches to
// BusContext.AttachedHitrace. The StagePerfTriage pre-stage reads it
// and dispatches perf_triager to extract a PerfBundle (jank spans,
// main-thread stalls, cold-start timing).
//
// Sticky lifetime is identical to SetAttachedLog: CLI sets once from
// --htrace / --htrace-text before Run(); REPL /htrace can carry it
// across turns until /htrace clear. Empty string clears.
func (o *Orchestrator) SetAttachedHitrace(trace string) {
	o.attachedHitrace = trace
}

// AttachedHitrace returns the current attached-trace payload.
func (o *Orchestrator) AttachedHitrace() string {
	return o.attachedHitrace
}

func (o *Orchestrator) SetAttachedHitraceSource(source string) {
	o.attachedHitraceSource = source
}

func (o *Orchestrator) AttachedHitraceSource() string {
	return o.attachedHitraceSource
}

// SetPresentationDirective installs the current turn's typed display
// requirement for the next Run. Unlike /log and /htrace this is not
// sticky: Run consumes and clears it at entry, and REPL dispatch calls
// the setter with "" on turns without a directive.
func (o *Orchestrator) SetPresentationDirective(directive string) {
	o.presentationDirective = strings.TrimSpace(directive)
}

// SetTurnRouteHint installs typed current-turn routing metadata for the next
// Run. The hint is advisory prompt context only: it must not replace
// emit_analysis and must not decide final-answer gates. Run consumes and
// clears it at entry.
func (o *Orchestrator) SetTurnRouteHint(hint types.TurnRouteHint) {
	o.turnRouteHint = hint
}

// SetMode installs the pipeline mode for subsequent Run() calls. Any
// invalid value (not one of ModeRead / ModePlan / ModeApply /
// ModeVerify and not empty) is still stored verbatim — SetMode does
// NOT silently coerce or reject, because the CLI layer needs to
// surface a clean "unknown mode" error to the user before Run() even
// starts. Empty string is explicitly valid and coerces to ModeRead
// at Run() entry via PipelineMode.Normalize.
//
// For the L1 red line (read-mode byte-identity) the important
// invariant is that a caller who never invokes SetMode has
// o.mode == "", which Normalize → ModeRead, which the Mode switch
// dispatches to the unchanged runTaskPhase call path.
func (o *Orchestrator) SetMode(m types.PipelineMode) {
	o.mode = m
}

// Mode returns the currently configured pipeline mode (post-
// SetMode, pre-Normalize). Useful for logging / CLI echo. The
// empty string is returned when SetMode has never been called.
func (o *Orchestrator) Mode() types.PipelineMode {
	return o.mode
}

// SetPlanPath installs the plan file path for subsequent Run()
// calls. Used when Mode is ModeApply / ModeVerify to tell
// the apply stage hook where to load the ChangePlan from. Empty string
// clears any prior value (useful for REPL workflows that alternate
// between modes).
func (o *Orchestrator) SetPlanPath(path string) {
	o.planPath = path
}

// PlanPath returns the currently configured plan file path.
func (o *Orchestrator) PlanPath() string {
	return o.planPath
}

// SetWorktreeBase installs the directory root the apply stage hook passes
// to worktree.Create when provisioning per-Run worktree sessions.
// cmd/root.go computes the path at initApp time from the runtime
// anchor and calls this once per process.
func (o *Orchestrator) SetWorktreeBase(dir string) {
	o.worktreeBase = dir
}

// SetWriteRetryBudget installs the T4 verify→plan retry cap. Zero
// disables retry (fail-loud semantics). Values are clamped to
// [0, PipelineSettings.WriteRetryBudgetCeil] — the resolved settings
// fill a default of 5 when the yaml leaves it blank — anything higher
// is almost certainly a misconfiguration that would burn LLM tokens
// on an unfixable plan.
func (o *Orchestrator) SetWriteRetryBudget(n int) {
	if n < 0 {
		n = 0
	}
	hardCap := o.settings.WriteRetryBudgetCeil
	if hardCap <= 0 {
		hardCap = 5
	}
	if n > hardCap {
		n = hardCap
	}
	o.writeRetryBudget = n
}

// SetWriteApprovalPolicy installs the write-lane approval policy. It is
// independent from write_enabled, which is enforced by cmd/repl before write
// mode dispatches. Empty or unknown values normalize to auto_safe for
// orchestrator-created runs so tests and legacy construction do not silently
// fall back to a stricter manual policy.
func (o *Orchestrator) SetWriteApprovalPolicy(policy writeflow.ApprovalPolicy) {
	if policy == "" {
		o.writeApprovalPolicy = writeflow.ApprovalPolicyAutoSafe
		return
	}
	o.writeApprovalPolicy = writeflow.NormalizeApprovalPolicy(policy)
}

// WriteRetryBudget returns the currently configured retry cap.
func (o *Orchestrator) WriteRetryBudget() int {
	return o.writeRetryBudget
}

// SetForceFinalizeAttempts installs the cap on force-finalize
// dispatch attempts. The force-finalize path is the user's last
// chance to recover ANY answer when the regular DAG path stalls or
// rejects every draft, so a transient connection drop on its single
// attempt would terminate the Run without a result. Default 3 (1
// initial + 2 retries) catches the typical single-blip case while
// keeping the worst-case pause bounded. Clamped to
// [1, MaxForceFinalizeAttempts]; zero or negative reverts to the
// type-default (DefaultForceFinalizeAttempts).
func (o *Orchestrator) SetForceFinalizeAttempts(n int) {
	if n <= 0 {
		n = types.DefaultForceFinalizeAttempts
	}
	if n > types.MaxForceFinalizeAttempts {
		n = types.MaxForceFinalizeAttempts
	}
	o.forceFinalizeAttempts = n
}

// SetTransientRetryBudget installs the cap on transient dispatch
// retries (stream stalls / first-byte timeouts / 429 / 5xx / network
// blips). Decoupled from SetWriteRetryBudget so a brief upstream
// hiccup never starves the verify→plan SC retry budget. Clamped to
// [0, PipelineSettings.TransientRetryBudgetCeil]; cmd/root.go fills a
// default ceil of 3 when yaml leaves it blank.
func (o *Orchestrator) SetTransientRetryBudget(n int) {
	if n < 0 {
		n = 0
	}
	hardCap := o.settings.TransientRetryBudgetCeil
	if hardCap <= 0 {
		hardCap = 3
	}
	if n > hardCap {
		n = hardCap
	}
	o.transientRetryBudget = n
}

// TransientRetryBudget returns the currently configured transient
// retry cap. Read by the read-mode and write-mode schedulers.
func (o *Orchestrator) TransientRetryBudget() int {
	return o.transientRetryBudget
}

// SetMaxUpstreamFallbacksPerRun (Block 3 architecture overhaul
// 2026-05-02) installs the cap on selective upstream fallbacks
// (BackToExtract / BackToExplore) the contract-failure path may
// consume per Run. Negative values restore the default (2); zero
// disables the cap (not recommended).
func (o *Orchestrator) SetMaxUpstreamFallbacksPerRun(n int) {
	if n < 0 {
		n = 2
	}
	o.maxUpstreamFallbacksPerRun = n
}

// MaxUpstreamFallbacksPerRun returns the currently configured cap
// for stats / tests.
func (o *Orchestrator) MaxUpstreamFallbacksPerRun() int {
	return o.maxUpstreamFallbacksPerRun
}

// SetReflector installs the optional Reflexion-pattern critic. Nil
// is legal and disables reflection (clearForReplan falls back to the
// heuristic-only PlanningHint). cmd/root.go wires this from
// providers.yaml :: agents.reflector or the default LLM.
func (o *Orchestrator) SetReflector(r Reflector) {
	o.reflector = r
}

// SetPlanCritic installs the optional pre-apply plan-review LLM.
// Nil disables — planPostHook then runs without invoking the
// critic. yaml-gated by pipeline_plan_critic_enabled in
// cmd/root.go: when the gate is off, this stays nil regardless of
// providers.yaml routing.
func (o *Orchestrator) SetPlanCritic(c PlanCritic) {
	o.planCritic = c
}

// SetAcceptanceChecker installs the per-phase verdict LLM used
// by stage II's runPhaseGroup. Nil is legal and disables
// per-phase acceptance gating (every phase auto-advances after
// verify passes). cmd/root.go wires this from providers.yaml ::
// agents.acceptance_checker or the default LLM. The cost is
// only paid inside multi-phase Runs (single-phase work never
// invokes runPhaseGroup), so installing unconditionally is
// safe.
func (o *Orchestrator) SetAcceptanceChecker(c AcceptanceChecker) {
	o.acceptanceChecker = c
}

// SetPlanGroupStore installs the on-disk PlanGroup persister
// (stage II). Nil disables multi-phase execution entirely —
// runPhaseGroup short-circuits when the store is absent and
// falls back to the single-phase TaskGraph path. Wired from
// cmd/root.go's PlanStore construction site so the same plan
// directory hosts both single-phase plans and multi-phase
// groups.
func (o *Orchestrator) SetPlanGroupStore(s PlanGroupSaver) {
	o.planGroupStore = s
}

// PlanGroupSaver is the orchestrator-side interface for the
// on-disk PlanGroup store. Defined here (not in repl/) to
// avoid an orchestrator → repl import cycle. The concrete impl
// in internal/repl/plan_group_store.go satisfies this
// interface; tests may inject a mock.
type PlanGroupSaver interface {
	Save(g *types.PlanGroup) (string, error)
}

// PlanSaver is the orchestrator-side interface to the
// individual-plan store. Used by runPhaseGroup to persist each
// phase's ChangePlan so the REPL's /plan show / /history /
// /approve <id> can resolve them. internal/repl/PlanStore
// satisfies this interface.
type PlanSaver interface {
	Save(plan *types.ChangePlan) (string, error)
}

// SetPlanSaver installs the per-plan persister. Wired from
// cmd/root.go alongside SetPlanGroupStore. Nil = no-op (single-
// phase paths persist plans elsewhere; CLI single-shot writes
// directly via cmd/root.go's writePlanFile).
func (o *Orchestrator) SetPlanSaver(s PlanSaver) {
	o.planSaver = s
}

// SetContinuationClassifier installs the LLM-driven
// continuation classifier (commit 48 Gap 1). Wired from
// cmd/root.go via providers.yaml :: agents.continuation_classifier.
// Nil leaves the orchestrator on the keyword-heuristic
// fallback path in types.IsContinuation.
func (o *Orchestrator) SetContinuationClassifier(c ContinuationClassifier) {
	o.continuationClassifier = c
}

// SetFailureTaxonomyStore installs the stage-3 per-repo
// failure-pattern cache. Wired from cmd/root.go using the
// derived repo slug + user cache dir. Nil = feature disabled
// (the reflector still runs but its dispatch omits
// failurePatternTool entirely so no LLM tokens are wasted on
// a schema with no persist destination; planner injection
// skips its pitfall section).
//
// Side effect: also toggles the reflector's pattern-tool
// inclusion (commit 40). Reflector implementations that
// satisfy patternToolToggler get notified; others are no-ops.
func (o *Orchestrator) SetFailureTaxonomyStore(s *FailureTaxonomyStore) {
	o.failureTaxonomyStore = s
	if toggler, ok := o.reflector.(patternToolToggler); ok {
		toggler.SetPatternToolEnabled(s != nil)
	}
}

// patternToolToggler is the optional interface a Reflector
// can satisfy to receive the "you have a destination for
// emit_failure_pattern" signal. The default llmReflector
// satisfies it; tests with a stub Reflector can opt out.
type patternToolToggler interface {
	SetPatternToolEnabled(on bool)
}

// FailureTaxonomyStore returns the wired store (or nil when
// disabled). Exposed for the planner agent's
// BuildInitialInstruction to read RelevantTo without coupling
// to the orchestrator's full surface.
func (o *Orchestrator) FailureTaxonomyStore() *FailureTaxonomyStore {
	if o == nil {
		return nil
	}
	return o.failureTaxonomyStore
}

// SetAnswerTaxonomyStore wires the per-repo answer-pitfall cache
// (commit 51 Gap 3, read-mode mirror of failureTaxonomyStore).
// Nil disables the feature (no end-of-Run reviewer dispatch, no
// per-Run injection into the analyzer's planning context).
func (o *Orchestrator) SetAnswerTaxonomyStore(s *AnswerTaxonomyStore) {
	if o == nil {
		return
	}
	o.answerTaxonomyStore = s
}

// AnswerTaxonomyStore returns the wired store (or nil). Exposed
// for the REPL's /answer-pitfalls handler.
func (o *Orchestrator) AnswerTaxonomyStore() *AnswerTaxonomyStore {
	if o == nil {
		return nil
	}
	return o.answerTaxonomyStore
}

// SetAnswerReviewer wires the optional end-of-Run answer reviewer
// (commit 51 Gap 3). Nil disables the dispatch — the per-repo
// taxonomy can still be pre-populated by another build, but no
// new patterns will be emitted from this process.
func (o *Orchestrator) SetAnswerReviewer(r AnswerReviewer) {
	if o == nil {
		return
	}
	o.answerReviewer = r
}

// SetSemanticQualityReviewer wires the G5 second-layer reviewer
// (post_v2_runtime_gap_remediation, 2026-05-04). Nil disables the
// dispatch surface entirely; runContractCheck skips the review when
// the field is nil.
func (o *Orchestrator) SetSemanticQualityReviewer(r SemanticQualityReviewer) {
	if o == nil {
		return
	}
	o.semanticQualityReviewer = r
}

// SetStrictAnswerReviewEnabled toggles the post-finalizer review and
// rewrite loop. The zero-value orchestrator keeps strict mode enabled
// through strictAnswerReviewEnabledValue so older tests and fixtures
// preserve behaviour unless they explicitly opt out.
func (o *Orchestrator) SetStrictAnswerReviewEnabled(enabled bool) {
	if o == nil {
		return
	}
	o.strictAnswerReviewDisabled = !enabled
}

func (o *Orchestrator) strictAnswerReviewEnabledValue() bool {
	if o == nil {
		return true
	}
	return !o.strictAnswerReviewDisabled
}

// FinalizeRepairHardCapDefault is the conservative default cap on
// finalize-stage repair-loop iterations. P6 (2026-05-10): 2 means
// "after two repair attempts the answer ships with a residual-
// concerns caveat instead of a third LLM round".
const FinalizeRepairHardCapDefault = 2

// SetFinalizeRepairHardCap installs the operator-tunable hard cap.
// 0 (or out-of-range) → FinalizeRepairHardCapDefault.
func (o *Orchestrator) SetFinalizeRepairHardCap(n int) {
	if o == nil {
		return
	}
	if n <= 0 {
		n = FinalizeRepairHardCapDefault
	}
	o.finalizeRepairHardCap = n
}

// finalizeRepairHardCapValue returns the effective cap, falling
// back to FinalizeRepairHardCapDefault when the field is unset.
func (o *Orchestrator) finalizeRepairHardCapValue() int {
	if o == nil || o.finalizeRepairHardCap <= 0 {
		return FinalizeRepairHardCapDefault
	}
	return o.finalizeRepairHardCap
}

// SetExhaustiveDeterministicReviewThreshold installs the operator-
// tunable principal-member count above which the deterministic
// exhaustive member-set reviewer replaces LLM reviewers. -1 (or
// any negative) falls through to the types default; 0 disables
// entirely; positive values override the default.
func (o *Orchestrator) SetExhaustiveDeterministicReviewThreshold(n int) {
	if o == nil {
		return
	}
	if n < 0 {
		n = types.ExhaustiveMemberSetDefaultThreshold
	}
	o.exhaustiveDeterministicReviewThreshold = n
}

// exhaustiveDeterministicReviewThresholdValue returns the effective
// threshold. Returning 0 means "disabled" (deterministic reviewer
// never replaces LLM reviewers). Positive values are used as-is.
func (o *Orchestrator) exhaustiveDeterministicReviewThresholdValue() int {
	if o == nil {
		return types.ExhaustiveMemberSetDefaultThreshold
	}
	return o.exhaustiveDeterministicReviewThreshold
}

// SetReviewerWallBudgetFraction installs the per-reviewer wall
// budget fraction. <=0 keeps the legacy unguarded path; values
// outside (0, 1] are clamped to the (0, 1] range.
func (o *Orchestrator) SetReviewerWallBudgetFraction(f float64) {
	if o == nil {
		return
	}
	if f <= 0 {
		o.reviewerWallBudgetFraction = 0
		return
	}
	if f > 1 {
		f = 1
	}
	o.reviewerWallBudgetFraction = f
}

func (o *Orchestrator) reviewerWallBudgetFractionValue() float64 {
	if o == nil {
		return 0
	}
	return o.reviewerWallBudgetFraction
}

// reviewerDispatchContext returns the context.Context used to bound
// LLM reviewer dispatches. It mirrors CancelContext but is exposed
// as a distinct accessor so the deadline guard's call site is clear
// in contract_check.go: reviewers respect cancellation AND the per-
// reviewer wall budget fraction.
func (o *Orchestrator) reviewerDispatchContext() context.Context {
	if o == nil {
		return context.Background()
	}
	return o.CancelContext()
}

// injectResidualConcernsCaveat is the P6 chokepoint helper that
// surfaces the unresolved typed concern list when the finalize
// repair hard cap is reached AND emits a dock notice so the user-
// facing UI knows the repair-loop terminated by design.
//
// P7 (2026-05-10) channel correction: the caveat now flows through
// the SYSTEM channel ("**补充说明：**" /
// "**Additional notes:**" heading), NOT the LLM-authored
// AnswerDocumentV2.Caveats[] slot. The prior P6 implementation
// wrote into doc.Caveats[] which violates
// feedback_no_system_backfill_to_user_panel — that slot is for
// content gaps the LLM itself identified, not orchestrator
// decisions about repair-loop termination.
//
// Returns the answer string with the caveat appended via the
// system channel; caller assigns back to out.FinalAnswer. Empty
// (nothing to inject) input returns the answer unchanged.
//
// Caller has already decided the cap was exceeded (see the
// finalizeRepairHardCapValue / state.retryUsed comparison in the
// contract-failure loop). The dock notice is rendered through
// softFinalizeRepairCapMessage (CN/EN aware, red-line audited);
// the answer body receives detailed concern lines derived only from
// typed Violation data.
func (o *Orchestrator) injectResidualConcernsCaveat(out *agent.StageOutput, violations []types.Violation) {
	if o == nil || o.busCtx == nil || out == nil {
		return
	}
	concernCount := len(violations)
	// Multi-repo posture detection: only suggest /repos sub-repo
	// adjustment when the workspace actually has ≥2 sub-repos.
	// IsSingle() short-circuits cleanly for legacy single-repo
	// runs so the caveat doesn't render an inapplicable hint.
	multiRepo := false
	if mg, ok := o.busCtx.MultiGraph.(*multigraph.MultiGraph); ok && mg != nil && !mg.IsSingle() {
		multiRepo = true
	}
	caveat := softFinalizeRepairCapMessage(o.busCtx.Language, concernCount, multiRepo)
	// System-channel append: writes typed concern detail to the
	// trailing "**补充说明：**" / "**Additional notes:**" section,
	// distinct from the LLM-authored "**说明**：" / "**Caveats:**"
	// channel. The dock summary below intentionally does NOT get
	// appended to the answer; it is a progress/status message.
	out.FinalAnswer = AppendResidualConcernDetailsToAnswer(out.FinalAnswer, violations, o.busCtx.Language)
	// Surface the cap event to the dock so the user knows the
	// loop terminated by design rather than silently shipping with
	// hidden caveats. P7: dedicated NoticeFinalizeRepairCap kind
	// (info-class gray) instead of reusing NoticeFallbackFailLoud
	// (warning yellow) — the answer DID ship, this is informational
	// not a fail-loud.
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeFinalizeRepairCap,
		Reasoning:  caveat,
	})
}

// SetSemanticQualityMinConfidence sets the G5 reviewer's self-rated
// confidence floor below which verdicts are silently dropped. 0
// (or out-of-range) keeps the SemanticQualityMinConfidenceDefault
// (0.92, set in P4 2026-05-10).
//
// Symmetric with SetSelfConsistencyMinConfidence — both reviewer
// thresholds are operator-tunable via codrax.yaml. The 0.92 default
// is calibrated against the May-9 sweep (forensic doc:
// docs/design/finalizer_pretrip_prevention.md §1) — a lower floor
// adds borderline-confidence noise back; a higher floor risks
// missing genuine facet-coverage gaps.
func (o *Orchestrator) SetSemanticQualityMinConfidence(f float64) {
	if o == nil {
		return
	}
	if f <= 0 || f > 1 {
		f = SemanticQualityMinConfidenceDefault
	}
	o.semanticQualityMinConfidence = f
}

// SetSelfConsistencyReviewer wires the commit-62 prose-coherence
// reviewer. Nil disables the post-finalize consistency review
// entirely (no LLM dispatch, no rewrite trigger).
func (o *Orchestrator) SetSelfConsistencyReviewer(r SelfConsistencyReviewer) {
	if o == nil {
		return
	}
	o.selfConsistencyReviewer = r
}

// SetSelfConsistencyRewriteOnContradiction toggles whether
// ViolSelfContradiction is allowed to request a rewrite. The actual retry
// decision still respects the active soft/strict violation policy, so
// default-soft contradictions are logged and caveated unless operators promote
// the kind via pipeline_contract_strict_kinds. false: telemetry only.
func (o *Orchestrator) SetSelfConsistencyRewriteOnContradiction(on bool) {
	if o == nil {
		return
	}
	o.selfConsistencyRewriteOnContradiction = on
}

// SetSelfConsistencyMinConfidence sets the reviewer's self-rated
// confidence floor below which verdicts are silently dropped.
// 0 keeps the default (0.8). Negative or > 1 → clamped to default.
func (o *Orchestrator) SetSelfConsistencyMinConfidence(f float64) {
	if o == nil {
		return
	}
	if f <= 0 || f > 1 {
		f = 0.8
	}
	o.selfConsistencyMinConfidence = f
}

// SetBaselineCaptureEnabled toggles the pre-apply test snapshot.
// Pointer-typed parameter so callers can distinguish "explicit
// false" from "yaml absent"; absent falls through to code default
// (false).
func (o *Orchestrator) SetBaselineCaptureEnabled(on bool) {
	o.baselineCaptureEnabled = on
}

// BaselineCaptureEnabled returns the current setting.
func (o *Orchestrator) BaselineCaptureEnabled() bool {
	return o.baselineCaptureEnabled
}

// SetBaselineCache installs (or clears) the per-anchor baseline
// disk cache. Pass nil to disable. Wired from cmd/root.go using the
// resolved baseline_cache_max + runtimeAnchor.
func (o *Orchestrator) SetBaselineCache(c *BaselineCache) {
	o.baselineCache = c
}

// BaselineCache returns the current cache (nil when disabled).
func (o *Orchestrator) BaselineCache() *BaselineCache {
	return o.baselineCache
}

// SetWriteMaxSeconds bounds the wall-clock duration of a write-mode
// Run. 0 disables the cap (legacy behaviour). Negative values are
// coerced to 0. Values above 1800 are clamped to 1800 to defend
// against operator typos that would let a Run grind for hours
// before the deadline fired.
func (o *Orchestrator) SetWriteMaxSeconds(n int) {
	if n < 0 {
		n = 0
	}
	const hardCap = 1800
	if n > hardCap {
		logging.Warning("[orchestrator] write_max_seconds %d clamped to %d (operator hard cap)", n, hardCap)
		n = hardCap
	}
	o.writeMaxSeconds = n
}

// WriteMaxSeconds returns the current cap (0 = disabled).
func (o *Orchestrator) WriteMaxSeconds() int {
	return o.writeMaxSeconds
}

// SetAutoInitRepo authorizes the apply pre-hook to run `git init`
// + an empty initial commit on a bare or commitless main repo
// before provisioning the worktree. False (default) preserves the
// fail-loud behaviour: a non-repo target surfaces a clear error
// pointing the operator at the --auto-init-repo flag, the
// write_auto_init_repo yaml knob, or the REPL's interactive consent
// prompt. Wired from cmd/root.go after resolving the flag/yaml
// precedence.
func (o *Orchestrator) SetAutoInitRepo(on bool) {
	o.autoInitRepo = on
}

// AutoInitRepo returns the current setting (consumed by REPL when
// it needs to know whether to skip the y/N consent prompt).
func (o *Orchestrator) AutoInitRepo() bool {
	return o.autoInitRepo
}

// SetScaffoldEnabled authorizes the planner to operate on an empty
// target directory (from-scratch project creation). Distinct from
// SetAutoInitRepo: that one only authorizes `git init`, this one
// authorizes inventing files. Default false. Wired from cmd/root.go
// (CLI --allow-scaffold or yaml write_scaffold_enabled).
func (o *Orchestrator) SetScaffoldEnabled(on bool) {
	o.scaffoldEnabled = on
}

// ScaffoldEnabled returns the current setting (consumed by REPL +
// stallPlateauMessage to decide which authorization tier to surface).
func (o *Orchestrator) ScaffoldEnabled() bool {
	return o.scaffoldEnabled
}

// SetKeepWorktreeOnSuccess toggles the post-Run worktree preservation
// on a successful ModeApply. Failure paths always discard regardless
// of this flag.
func (o *Orchestrator) SetKeepWorktreeOnSuccess(on bool) {
	o.keepWorktreeOnSuccess = on
}

// SetSkipVerify toggles the verify-stage short-circuit for
// ModeApply Runs. When true, the write scheduler marks the verify
// node done immediately after apply succeeds — no run_tests
// dispatch, no FailureSummary, no verify→plan retry. Per-Run
// scope: REPL `/approve --skip-verify` flips this on then defers
// SetSkipVerify(false) so the override doesn't bleed into the
// next Run.
//
// Use case: integration tests need infra the operator's box can't
// run (DB / GPU / external API). The operator reviews the plan
// diff, decides to land bytes locally, and lets CI run tests on
// push. The plan's on-disk Status is set to `applied` (clean
// success path) — semantically different from a verify_failed
// plan where tests ran and failed.
func (o *Orchestrator) SetSkipVerify(on bool) {
	o.skipVerify = on
}

// SkipVerify returns the current setting.
func (o *Orchestrator) SkipVerify() bool {
	return o.skipVerify
}

// SetReuseWorktreePath installs an existing worktree directory the
// verify pre-hook should swap RepoRoot to instead of provisioning a
// fresh checkout. Used by REPL `/verify <plan-id>` to re-test against
// the bytes the original apply landed (preserved via
// pipeline_keep_worktree_on_success). Empty (default) disables the
// override and falls back to the standard "test the main repo" path.
//
// The path is NOT mirrored onto busCtx.WorktreePath — doing so would
// trigger the outer Run() worktree-cleanup defer, discarding the
// preserved tree the user explicitly wanted to keep.
func (o *Orchestrator) SetReuseWorktreePath(path string) {
	o.reuseWorktreePath = path
}

// KeepWorktreeOnSuccess returns the current setting.
func (o *Orchestrator) KeepWorktreeOnSuccess() bool {
	return o.keepWorktreeOnSuccess
}

// Run executes the full pipeline for a user request.
//
// The pipeline runs in two phases:
//
//   - Phase 1 — analyze: dispatch StageAnalyze once. The analyzer
//     emits an AnalysisIR via emit_analysis and the post-processing
//     pipeline deterministically builds TaskGraph / EvidencePlan /
//     AnswerContract / HypothesisSet from it.
//
//   - Phase 2 — per-task: iterate over pending tasks (typically one),
//     running a mini-pipeline (explore → extract → finalize) for each
//     via runTaskGraph. Per-task state (Signals, MissingPiece,
//     PipelineStage, oscillation counter) resets between tasks;
//     shared state accumulates.
//
// The maxSteps budget is enforced globally across both phases.
func (o *Orchestrator) Run(request string, repoRoot string, branch string) (*types.BusContext, error) {
	// Allocate a fresh CancelToken for this Run. REPL grabs it via
	// the public Cancel() method to drive Ctrl+C / `/cancel`. Cleared
	// in the defer below so idle Orchestrators (between Runs) cannot
	// leak a stale "canceled" state into the next Run.
	o.cancelToken = NewCancelToken()
	defer func() { o.cancelToken = nil }()

	// Defensive reset of cross-Run sticky slots. The Orchestrator
	// instance is reused across Runs in the REPL, so any field
	// that was set during a previous multi-phase Run must be
	// wiped before the next Run starts — else a stage II run
	// followed by a single-phase Run would leak the prior phase
	// header into the new Run's clearForReplan. Single-phase
	// Runs never write these slots, so the reset is a no-op for
	// the read-mode path.
	o.phaseContextPrefix = ""
	o.nextPhaseHint = ""
	o.continuationClassification = nil
	presentationDirective := strings.TrimSpace(o.presentationDirective)
	o.presentationDirective = ""
	turnRouteHint := o.turnRouteHint
	o.turnRouteHint = types.TurnRouteHint{}

	// Wall-clock deadline for write-mode Runs. The timer fires at
	// most once per Run; the AfterFunc closure cancels the token
	// with a typed reason so the caller's "✗ canceled" rendering
	// distinguishes user-cancelled from time-cancelled. Read-mode
	// Runs are explicitly unaffected — the timer is only armed when
	// the operator-set Mode is plan / apply / verify. Stop() is
	// best-effort: a fired timer is a no-op.
	if o.writeMaxSeconds > 0 && o.mode != types.ModeRead && o.mode != "" {
		deadline := time.Duration(o.writeMaxSeconds) * time.Second
		timer := time.AfterFunc(deadline, func() {
			o.Cancel(fmt.Sprintf("write mode wall-time exceeded (%ds)", o.writeMaxSeconds))
		})
		defer timer.Stop()
	}

	// Defensive live-preview cleanup. The finalizer streaming preview
	// area opens on EventLivePreviewChunk and the orchestrator emits
	// the matching EventLivePreviewClear at three known finalize
	// outcomes (contract pass / contract fail / dispatch error). Any
	// OTHER early-exit path — step budget exhaustion, Ctrl+C / ctx
	// cancel before contract check, an analyzer fail-loud that never
	// reaches finalize, a panic — would otherwise leave the preview
	// area drawn on the user's terminal, ghosting under whatever the
	// REPL prints next. Emit a redundant Clear here so the renderer's
	// handlePreviewClearLocked tears the area down on EVERY Run exit
	// path. The handler is a no-op when no area is open, so the
	// happy-path emission is harmless. Marked PreviewRejected=true so
	// the area gets the "[已重写]" flash on stranded-cleanup paths
	// where the in-flight draft was implicitly thrown away by an
	// abnormal exit.
	defer func() {
		o.emit(render.Event{
			Kind:            render.EventLivePreviewClear,
			Timestamp:       time.Now(),
			Stage:           types.StageFinalize,
			PreviewRejected: true,
		})
	}()

	// Initialize BusContext
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageAnalyze,
		RepoRoot:      repoRoot,
		// MainRepoRoot mirrors RepoRoot at Run entry and stays
		// constant for the lifetime of the Run. Write-mode phases
		// may swap RepoRoot to a worktree path; MainRepoRoot
		// preserves the original so the worktree-cleanup defer can
		// run `git worktree prune` against the canonical repo.
		// Read-mode Runs see MainRepoRoot == RepoRoot throughout.
		MainRepoRoot:          repoRoot,
		Branch:                branch,
		TraceID:               fmt.Sprintf("trace-%d", time.Now().UnixNano()),
		PresentationDirective: presentationDirective,
		TurnRouteHint:         turnRouteHint,
		// Mode normalization turns zero-value ("") into ModeRead so
		// downstream switch equality is exact. The L1 red line
		// depends on this — a caller who never invokes SetMode
		// reaches this line with o.mode == "" and the switch below
		// dispatches to the unchanged runTaskPhase.
		Mode: o.mode.Normalize(),
		// PlanPath propagates the --plan-file flag / REPL state into
		// the bus so the apply stage hook / the verify stage hook read a single
		// authoritative source. Empty for plan-mode and read-mode
		// (plan is produced, not consumed, there).
		PlanPath: o.planPath,
		Mutable:  types.NewMutableState(request),
		TaskState: types.TaskState{
			Stage:   types.StageAnalyze,
			Missing: types.MissingUnderstanding,
		},
		Memory:               o.memoryReader,
		EnvRecommendSettings: o.envSettings,
		// Phase 2 cancellation: BusContext.Ctx is the standard ctx
		// surface tools / agents derive from. Cancel propagates to
		// HTTP / subprocess / any ctx-aware path immediately rather
		// than waiting for a cooperative checkpoint.
		Ctx: o.cancelToken.Context(),
	}
	if transcriptRequest := strings.TrimSpace(o.outputTranscriptRequest); transcriptRequest != "" {
		// Freeze the REPL-provided expanded request onto this Run's
		// MutableState. The Orchestrator itself is reused across REPL
		// turns, so final output artifacts must read this run-scoped
		// copy rather than the cross-turn setter field at finalize time.
		o.busCtx.Mutable.SetOutputTranscriptRequest(transcriptRequest)
	}

	// Multi-repo wiring (Phase 4.2). Both providers are nil in
	// pre-multi-repo callers (single-shot tests, eval harness without
	// app context) and the BusContext fields stay zero-valued, which
	// downstream consumers tolerate (legacy single-repo path).
	if o.multiGraphProvider != nil {
		o.busCtx.MultiGraph = o.multiGraphProvider()
	}
	if o.multiRepoSnapshotProvider != nil {
		o.busCtx.SubRepos, o.busCtx.PendingSubRepos = o.multiRepoSnapshotProvider()
	}
	o.busCtx.MultiRepoInactivePreviewCount = o.multiRepoInactivePreviewCount
	// Phase 4.F routing fold (design §3.5). Pre-trim active set per
	// Run so the typed-lane queries hit a deterministic, focus-aware
	// subset. Channels at Run entry:
	//   A — REPL /repos focus (mg.FocusSlugs), strict and user-pinned
	//   D — heuristic language affinity from request keywords
	//   E — FileCount desc fallback (marked as fallback_preview)
	// Channels B (analyzer.RequiredFiles) + C (log frame files) are
	// applied incrementally: log_triage's validateFrame triggers
	// EnsureLoadedFor when a frame resolves to an inactive sub-repo,
	// and the analyzer post-emit hook does the same for RequiredFiles.
	// Single-repo posture short-circuits inside RouteActiveSet.
	if mg, _ := o.busCtx.MultiGraph.(*multigraph.MultiGraph); mg != nil {
		focusSlugs := mg.FocusSlugs()
		inputs := multigraph.RoutingInputs{}
		if len(focusSlugs) > 0 {
			if len(focusSlugs) > mg.Cap() {
				return o.busCtx, fmt.Errorf("multi-repo focus selects %d sub-repos but multi_repo_max_active is %d; reduce focus or raise the cap (hard ceiling %d)", len(focusSlugs), mg.Cap(), config.MultiRepoMaxActiveCeiling)
			}
			inputs.FocusSlugs = focusSlugs
			inputs.StrictFocus = true
			inputs.DisableFallback = true
			o.busCtx.MultiRepoFocusDecision = multiRepoFocusDecisionForSlugs(mg, focusSlugs, types.MultiRepoFocusSourceUserPinned, 1.0, "user-pinned focus")
		} else {
			if dec := tryExactMultiRepoFocus(mg, request); dec != nil {
				if slugs := multiRepoFocusDecisionSlugs(mg, dec); len(slugs) > 0 {
					inputs.ExactPrescanSlugs = slugs
					inputs.DisableFallback = true
					o.busCtx.MultiRepoFocusDecision = dec
				}
			}
			if len(inputs.ExactPrescanSlugs) == 0 {
				if dec := o.trySelectMultiRepoFocus(mg); dec != nil && dec.Confidence >= 0.35 {
					if slugs := multiRepoFocusDecisionSlugs(mg, dec); len(slugs) > 0 {
						inputs.ModelRecommendedSlugs = slugs
						inputs.DisableFallback = true
						o.busCtx.MultiRepoFocusDecision = dec
					}
				}
			}
			if len(inputs.ExactPrescanSlugs) == 0 && len(inputs.ModelRecommendedSlugs) == 0 {
				inputs.QueryLanguages = inferQueryLanguages(request)
				o.busCtx.MultiRepoFocusDecision = &types.MultiRepoFocusDecision{
					Source:    types.MultiRepoFocusSourceFallbackPreview,
					Fallback:  true,
					Rationale: "no explicit multi-repo focus was available; using legacy language/size fallback preview",
				}
			}
		}
		decision := mg.RouteActiveSet(inputs)
		if err := mg.EnsureMany(decision.Active); err != nil {
			// Fail-loud per design §3.4 R3 — caller fold should have
			// pre-trimmed; if we trip here, log and degrade to whatever
			// loaded (single-repo fallback path stays consistent).
			logging.Warning("multigraph: routing fold EnsureMany failed: %v", err)
		}
		// Refresh PendingSubRepos using the routing decision (more
		// precise than the snapshot-time approximation — surfaces the
		// sub-repos the fold deliberately left out).
		//
		// CRITICAL — refresh ALWAYS runs, even when decision.Inactive
		// is empty. Pre-fix the conditional `if len(decision.Inactive)
		// > 0` left PendingSubRepos in its snapshot-time state, which
		// at Run() entry is "ALL sub-repos" because the LRU is empty
		// before any EnsureMany call. When the routing decision
		// declared every sub-repo active (cap ≥ total sub-repos AND
		// no focus pin → decision.Inactive empty), the stale "ALL
		// pending" carried into the L1 active-set gate, computing
		// `active = topology - PendingSubRepos = empty`, refusing
		// every tool with "workspace has no active sub-repos".
		// mr_focus_single hit this exact path during the 2026-05-09
		// sweep (cap=3 from multirepo_settings.yaml, no focus pin,
		// 3 discovered sub-repos → Inactive empty → refresh skipped
		// → workspace looked empty to the gate). Always rewriting
		// PendingSubRepos from the routing decision ensures the
		// Active = topology - PendingSubRepos invariant the gate
		// relies on stays correct in the cap-meets-total case.
		pendingNames := make([]string, 0, len(decision.Inactive))
		for _, slug := range decision.Inactive {
			if sr := mg.Topology().SubRepoBySlug(slug); sr != nil {
				pendingNames = append(pendingNames, sr.RootRel)
			}
		}
		o.busCtx.PendingSubRepos = pendingNames
	}
	// Phase 6 multi-repo telemetry. One log line at Run exit so
	// operators see resident sub-repo count, eviction pressure, and
	// thrashing state without having to enable a special debug mode.
	// Skipped silently when MultiGraph is unset (single-shot tests).
	//
	// 2026-05-08 audit: previously emitted up to 3 lines per Run
	// (summary + thrashing + typed-lane-partial), but the partial
	// line's data (`Pending=[...]`) was already in the summary's
	// `pending=[X,Y]` field — the extra line only added a one-shot
	// remediation hint. Collapse to ONE line per Run by appending
	// the hint to the summary when pending is non-empty for multi-
	// repo. Thrashing keeps its own Warning-level line because the
	// urgency is different (operators may want a separate red flag
	// to grep for in dashboards). Single-repo never sees pending so
	// the hint is suppressed; single-shot / nil-MultiGraph paths
	// still render zero lines.
	if mg, _ := o.busCtx.MultiGraph.(*multigraph.MultiGraph); mg != nil {
		defer func() {
			snap := mg.Snapshot()
			line := snap.FormatLogLine()
			if len(snap.Pending) > 0 && !snap.IsSingle {
				line += " — raise cap or `/repos focus <slug-or-path>` to include the pending sub-repos"
			}
			logging.Info("%s", line)
			if snap.Thrashing {
				logging.Warning("multigraph: thrashing detected (>5 evictions/60s) — raise multi_repo_max_active in codrax.yaml or `/repos cap N` to fit your active sub-repo set")
			}
		}()
	}
	// env_recommend: probe once at Run entry when enabled. Cached
	// on BusContext for the lifetime of the Run; tools (run_tests,
	// chitchat fallback) read EnvFacts to power the recommend
	// pipeline. Probe never errors fatally — partial facts beat
	// no facts. Skipped when env_recommend_enabled=false (R6 red
	// line: legacy hardcoded hint stays the only signal).
	if o.envSettings.Enabled {
		o.busCtx.EnvFacts = env.Probe(env.ProbeOptions{
			RepoRoot:     repoRoot,
			ProbeNetwork: o.envSettings.ProbeNetwork,
		})
		// One-shot INFO when the target dir is not a fully initialised
		// git repo. Read-mode never auto-scaffolds, but operators see a
		// clear signal here instead of being puzzled by downstream
		// stages that quietly degrade. The bare-dir authorization gate
		// only surfaces in write mode (apply_pre_hook / plan_pre_hook).
		if facts := o.busCtx.EnvFacts; facts != nil {
			switch facts.GitRepoState {
			case "not_initialized":
				logging.Info("[orchestrator] target %s is not a git repo; read-mode continues, write modes will require auto-init authorization (CLI --auto-init-repo / yaml write_auto_init_repo / REPL consent)", repoRoot)
			case "no_commits":
				logging.Info("[orchestrator] target %s is a git repo with no commits; read-mode continues, write modes will require auto-init authorization", repoRoot)
			case "git_missing":
				logging.Warning("[orchestrator] git binary not found on PATH; some pipeline features (worktree, repo_map walk, write mode) will be unavailable")
			}
		}
	}

	// Empty-repo read-mode short-circuit. When a user points read mode
	// at a dir with no analyzable source files (fresh tmp dir, just-
	// created project skeleton), the analyzer has nothing to pre-scan,
	// the explorer has nothing to read, and the finalizer falls back
	// to the weakest hard-fallback Generic answer. The downstream
	// degraded result reads as confused rather than honest. Surface a
	// short, actionable message instead so the user immediately sees
	// what to do (point --repo at real source, or switch modes to
	// scaffold from scratch). Cross-platform: dirIsEffectivelyEmpty
	// uses os.ReadDir so Windows / macOS / Linux all walk the same
	// path. Skipped when Mode != ModeRead — write modes hit
	// planPreHook / applyPreHook which produce their own
	// authorization message via bareDirAuthorizationMessage.
	if o.busCtx.Mode == types.ModeRead && dirIsEffectivelyEmpty(repoRoot) {
		msg := emptyRepoReadIntro(o.busCtx.Language, repoRoot)
		logging.Info("[orchestrator] target %s is effectively empty; read-mode short-circuit with intro message", repoRoot)
		o.busCtx.Mutable.SetResultPlain(msg)
		o.busCtx.TaskState.IsTerminal = true
		o.busCtx.TaskState.Stage = types.StageFinalize
		o.busCtx.PipelineStage = types.StageFinalize
		o.emit(render.Event{
			Kind:      render.EventPipelineEnd,
			Timestamp: time.Now(),
			TraceID:   o.busCtx.TraceID,
		})
		return o.busCtx, nil
	}

	// Thread repoRoot into MutableState so the lazy-init
	// EvidenceClosure canonicaliser (session 22) can strip an
	// absolute-path prefix the LLM may use in read_file banners
	// and evidence source fields. Without this call the closure
	// falls back to the strip-"./" canonicaliser and absolute
	// paths mismatch the repo-relative CGEC ReadSet.
	o.busCtx.Mutable.SetRepoRoot(repoRoot)

	// Fail-loud guard for REPL control inputs that escaped the slash
	// dispatcher. The customer trigger was a CLI invocation that
	// passed `/approve plan-XXX` literally as `--request`: the
	// orchestrator dispatched the analyzer, which iterated 12+ times
	// rejecting its own emit_analysis call (`IsREPLControlInput`
	// returns true) before being killed by SIGINT. The slash form is
	// never a code question; refusing here before any LLM call saves
	// the round-trip cost and gives the operator a clear signal.
	// The guard strips the REPL conversation prefix so a legitimate
	// REPL turn whose Prior Conversation memory contains a `/approve`
	// line in summary doesn't false-fire.
	if probe := types.StripConversationPrefix(request); types.IsREPLControlInput(probe) {
		err := fmt.Errorf("orchestrator: request %q is a REPL control command, not a code question — slash commands must be intercepted by the REPL dispatcher (or removed from CLI --request); refusing to start analysis because it would iterate to its budget cap rejecting its own emit_analysis call", probe)
		o.busCtx.TaskState.LastError = err.Error()
		return o.busCtx, err
	}

	// Capture a stable report directory before any retry-loop reset
	// can wipe busCtx.PlanPath. saveChangeReport reads o.reportDir as
	// a fallback so post-retry reports still land on disk under the
	// same dir as the original --plan-file. Empty when no plan-file
	// path is supplied (plan-mode e2e flow); the existing skip-with-
	// log behavior in saveChangeReport handles that case.
	if o.planPath != "" {
		o.reportDir = filepath.Dir(o.planPath)
	} else {
		o.reportDir = ""
	}
	// Per-Run reset of the warm-worktree retry bookkeeping. Both SHAs
	// are populated by applyPostHook + clearForReplan once Run() is
	// underway; cleared here so a previous Run's state cannot leak
	// into this one through a long-lived Orchestrator instance (REPL
	// case).
	o.currentIterCommitSHA = ""
	o.bestAppliedCommitSHA = ""

	// Module C: a fresh Run starts with an empty iteration ledger.
	// The ledger only accumulates between verify→plan retries within
	// THIS Run; a new Run's planner must not see prior-Run history
	// (lessons-library is the place for cross-Run memory, and
	// Voyager-pattern is intentionally out of scope per the redesign).
	o.busCtx.Mutable.ResetIterationLedger()
	// Module E: same shape for plan-stage probe reports — fresh Run
	// starts with no probe history.
	o.busCtx.Mutable.ResetPlanStageProbeReports()
	// Write exploration handoff is per write-task/per batch planning context.
	// A fresh Run must not inherit read-only source findings from a previous
	// REPL turn.
	o.busCtx.Mutable.ResetWriteExplorationRequest()
	o.busCtx.Mutable.ResetWriteExplorationHandoff()
	// Commit 51 Gap 3: read-mode answer-retry log resets per Run.
	// Events accumulate across retries within a Run; the end-of-Run
	// answer_reviewer dispatch (orchestrator.runAnswerReviewerOnSuccess)
	// reads them.
	o.busCtx.Mutable.ResetAnswerRetryEvents()
	// Commit 59 Batch E.1 (audit HIGH #12): learning-failure log
	// resets per Run. Surface counted at Run end so operators see
	// when reflector / answer_reviewer / taxonomy persistence broke.
	o.busCtx.Mutable.ResetLearningFailures()

	o.busCtx.Language = o.language
	o.busCtx.AttachedLog = o.attachedLog
	o.busCtx.AttachedHitrace = o.attachedHitrace
	o.busCtx.AttachedHitraceSource = o.attachedHitraceSource

	logging.Info("[orchestrator] starting pipeline: trace=%s", o.busCtx.TraceID)

	o.emit(render.Event{
		Kind:      render.EventPipelineStart,
		Timestamp: time.Now(),
		TraceID:   o.busCtx.TraceID,
	})

	// Working directory for tool blob storage. Tools that produce
	// large outputs offload to this dir and return a path in
	// ToolResult.RawRef so the LLM can re-read slices on demand
	// instead of carrying full content through the message history.
	//
	// Two layouts, selected by cmd/root.go at startup:
	//
	//   - Session (default): a persistent directory created by
	//     cmd/root.go at process start, shared across every Run(),
	//     pruned at next startup. No teardown here.
	//   - Per-trace tmpdir (legacy / blob_max_sessions=0 / test
	//     fixtures): os.MkdirTemp + deferred RemoveAll.
	if o.blobSessionDir != "" {
		o.busCtx.WorkDir = o.blobSessionDir
		logging.Info("[orchestrator] work dir (session): %s", o.blobSessionDir)
	} else if workDir, err := os.MkdirTemp("", "codrax-"+o.busCtx.TraceID+"-"); err != nil {
		logging.Warning("[orchestrator] could not create work dir: %v (blob storage disabled)", err)
	} else {
		o.busCtx.WorkDir = workDir
		logging.Info("[orchestrator] work dir (tmp): %s", workDir)
		defer func() {
			if rmErr := os.RemoveAll(workDir); rmErr != nil {
				logging.Warning("[orchestrator] work dir cleanup failed: %v", rmErr)
			}
		}()
	}

	// Worktree cleanup (B0 write-mode). The plan / apply / verify
	// stages may populate busCtx.WorktreePath + MainRepoRoot as they
	// provision a git worktree to run write actions inside. We fire
	// a defer here so both panic unwind and normal return reach
	// worktree.DiscardByPath. Read-mode Runs never set either field,
	// so DiscardByPath short-circuits and this block is a free no-op
	// for them (protecting the L1 "read mode byte-identical" red
	// line).
	//
	// Preserve-on-success exception: when keepWorktreeOnSuccess is
	// true AND the Run completed a ModeApply with no LastError, the
	// discard is skipped so the user can review the applied bytes
	// and cherry-pick to main manually. Failure paths always discard
	// so a misbehaving planner cannot fill disk with broken trees.
	//
	// SIGINT / SIGTERM does NOT run this defer (Go's default signal
	// disposition is os.Exit without unwind). The worktree package
	// installs a signal handler at process start that walks its
	// own activeSessions registry and discards outstanding sessions
	// before re-raising. This defer covers normal-return and panic;
	// the handler covers signal paths.
	defer func() {
		if o.busCtx == nil || o.busCtx.WorktreePath == "" {
			return
		}
		// Preserve-on-success: keep_on_success yaml knob OR
		// --skip-verify implies "user wants the bytes".
		// --skip-verify in particular: the operator's whole reason
		// for skipping verify is "I'll test in CI / I trust this
		// patch — just give me the changes." Discarding the
		// worktree after a successful skip-verify apply makes the
		// command meaningless (apply succeeded, then bytes vanished,
		// then /merge can't find anything because plan.WorktreePath
		// was never persisted to disk in the first place).
		preserve := o.busCtx.Mode == types.ModeApply &&
			o.busCtx.TaskState.LastError == "" &&
			(o.keepWorktreeOnSuccess || o.skipVerify)
		if preserve {
			// Persist the worktree path to the on-disk plan JSON
			// so /merge / /worktree list / /verify <plan-id>
			// can find it later. persistPlanStatus uses
			// UpdatePlanStatusOnDisk which honours empty
			// worktreePath as "don't touch", so we have to write
			// it explicitly here. The status was already set
			// (applied) by the verify post-hook or the
			// skip-verify shortcut; we re-stamp with the same
			// status to attach the path.
			if plan := o.busCtx.Mutable.ChangePlan(); plan != nil {
				now := time.Now()
				if err := types.UpdatePlanStatusOnDisk(o.busCtx.PlanPath,
					types.PlanStatusApplied, &now, o.busCtx.WorktreePath); err != nil {
					logging.Warning("[orchestrator] worktree preserve: persist path on plan %s failed: %v",
						plan.ID, err)
				}
			}
			reason := "keep_on_success"
			if o.skipVerify {
				reason = "skip_verify (apply succeeded; user wants bytes)"
			}
			logging.Info("[orchestrator] worktree preserved (%s): %s", reason, o.busCtx.WorktreePath)
			return
		}
		if err := worktree.DiscardByPath(o.busCtx.WorktreePath, o.busCtx.MainRepoRoot); err != nil {
			logging.Warning("[orchestrator] worktree cleanup failed: %v", err)
		}
		// Zero out busCtx.WorktreePath now that the directory is
		// gone. Without this, downstream consumers (the REPL's
		// "worktree preserved: <path>" message, /merge handler,
		// /worktree list) read a stale path that no longer exists
		// on the filesystem and falsely tell the user the bytes are
		// available. The applied bytes ARE still recoverable via
		// refs/codrax/applied/<plan-id> set by the apply post-hook;
		// /merge falls back to that ref when WorktreePath is empty.
		o.busCtx.WorktreePath = ""
	}()

	stepsUsed := 0

	// Pre-Phase-1: conditional pre-stages (log_triage + perf_triage).
	// Each pre-stage runs at most once. Guard decides whether the
	// stage fires for this Run. Failure is non-fatal — the main
	// pipeline continues with the stage's BusContext side-effect at
	// zero (e.g. bus.Mutable.LogTriage() / .PerfTrace() stays nil,
	// and downstream nil-checks degrade cleanly).
	//
	// Steps consumed by pre-stages count toward the same o.maxSteps
	// budget as main stages so a runaway pre-stage cannot starve the
	// main pipeline.
	for _, pre := range preStages {
		if !pre.Guard(o.busCtx) {
			continue
		}
		o.busCtx.PipelineStage = pre.Stage
		o.busCtx.TaskState.Stage = pre.Stage
		out, err := o.dispatchStage(pre.Stage)
		if err != nil {
			logging.Warning("[orchestrator] pre-stage %s failed: %v (main pipeline continues)",
				pre.Stage, err)
		} else if out != nil && out.Error != "" {
			logging.Warning("[orchestrator] pre-stage %s degraded: %s (main pipeline continues)",
				pre.Stage, out.Error)
		}
		stepsUsed++
	}

	// Phase 1: analyze. Fail-loud: when analyze exhausts its retry
	// budget the whole Run terminates without entering phase 2.
	// On success, emit EventAnalysisReady so the renderer can switch
	// from stage-dispatch rows to the analyzer's actual task / sub-
	// task breakdown.
	//
	// IMPORTANT — write modes defer EventAnalysisReady. The analyzer
	// runs in write mode purely as a classifier; the TaskGraph it
	// produces is irrelevant because BuildWriteTaskGraph replaces it
	// in the next block (line ~1011). Firing EventAnalysisReady here
	// would tell the dock to populate evidence/validate/finalize rows
	// from the read graph, then write_scheduler would emit
	// EventTaskNodeStart for plan/apply/verify nodes that the dock
	// never created — findNodeRow returns nil and the dock sits on
	// "等待派发" forever (customer-reported on a fresh
	// "用 python 写一个俄罗斯方块" plan-mode request).
	if used, err := o.runAnalyzePhase(); err != nil {
		if llm.IsStreamLevelRetryable(err) {
			logging.Error("[orchestrator] analyze phase failed on transient model transport after retry budget: %v", err)
			o.busCtx.TaskState.LastError = forcedFinalizeFailureMessage(err, o.busCtx.Language)
			o.busCtx.TaskState.SoftAnalyzerError = fmt.Sprintf("analyze transport: %v", err)
			_ = used
		} else {
			// Commit 59 Batch E.1 (audit HIGH #9): when analyze exhausts
			// retries, install a minimal degraded-mode IR instead of
			// hard-killing the Run. Single finalize node + explanation
			// shape lets the finalizer produce an honest "I cannot
			// classify this question — please rephrase / break it down /
			// add more context" answer with the original request quoted,
			// instead of returning an empty result. The user still sees a
			// reply; the Error field carries the diagnostic for operators.
			logging.Error("[orchestrator] analyze phase failed (degrading to recovery IR): %v", err)
			// Fix-A (2026-05-10): when the analyzer emit_analysis'd at
			// least once before the gate rejected it, preserve the
			// partial RequestModel + AnalyzerHints + Predicates so
			// explorer/extractor still have semantic context to work
			// with. The richer TaskGraph (probe → evidence → reconcile
			// → finalize) ensures explore/extract dispatch. The older
			// single-finalize recovery graph skipped those stages and could
			// leave the user with an empty-answer symptom; this branch is the
			// current fix, not that legacy behaviour.
			//
			// When o.busCtx.AnalysisIR is nil (analyzer NEVER emit_analysis'd),
			// buildDegradedSemanticIR delegates to the legacy zero-info
			// builder so behaviour is byte-identical for that branch.
			o.busCtx.AnalysisIR = buildDegradedSemanticIR(
				o.busCtx.Mutable.Objective(),
				o.busCtx.AnalysisIR,
				err,
			)
			// 2026-05-10 P-audit critical fix: do NOT set TaskState.LastError
			// here. Line ~1921 below guards `if LastError == "" { runTaskPhase }`
			// — setting it before the degraded scheduler runs causes the
			// entire Phase 2 (explorer / extractor / finalizer dispatch)
			// to be SKIPPED, recreating the legacy empty-answer symptom that
			// this recovery path is designed to avoid. The analyzer error is
			// preserved on the new AnalysisIR.QualityGate.Detail
			// ("degraded_semantic_recovery" check) for operator visibility,
			// and the SoftAnalyzerError field carries the original error
			// for user-panel hint composition without blocking the
			// scheduler.
			o.busCtx.TaskState.SoftAnalyzerError = fmt.Sprintf("analyze: %v", err)
			// dispatchStage/applyStageOutput records the failed analyzer
			// StageOutput.Error on LastError before runAnalyzePhase can
			// decide to degrade. Clear that stale hard-fail marker here;
			// otherwise the Phase-2 guard below skips the degraded graph
			// and the REPL renders the raw analyzer gate text instead of
			// letting explorer/finalizer produce an honest fallback answer.
			o.busCtx.TaskState.LastError = ""
			// Continue execution with the recovery IR. Step budget is
			// bounded by the per-stage caps already in place.
			_ = used
		}
	} else {
		stepsUsed += used
		if o.busCtx.Mode == types.ModeRead {
			o.emitAnalysisReady()
		}
	}

	// Phase 2: unified scheduler. Read mode walks the analyzer's
	// emitted TaskGraph; write modes substitute a fixed
	// plan→apply→verify graph from BuildWriteTaskGraph. Both share
	// runTaskGraph (which dispatches to runReadSchedulerLoop or
	// runWriteSchedulerLoop based on graph shape).
	//
	// L1 red line: the ModeRead branch leaves AnalysisIR.TaskGraph
	// untouched — runTaskGraph reads exactly what the analyzer
	// emitted, identical to pre-T4 behaviour.
	switch o.busCtx.Mode {
	case types.ModeRead:
		// Existing analyzer-emitted TaskGraph stays in place.
	case types.ModePlan, types.ModeApply, types.ModeVerify:
		// Substitute the linear write TaskGraph. The analyzer still
		// ran above as a classifier (its IR is on AnalysisIR but its
		// TaskGraph is replaced here). RetryBudget on the write graph
		// drives the verify→plan cycle in runWriteSchedulerLoop.
		//
		// Defensive nil check: runAnalyzePhase fails-loud + early-
		// returns on a nil IR, but a misbehaving analyzer mock could
		// return a clean StageOutput WITHOUT populating the IR.
		// Surface that fail-loud here rather than panic-derefing.
		if o.busCtx.AnalysisIR == nil {
			o.busCtx.TaskState.LastError = "write mode: analyzer returned no AnalysisIR — cannot build write TaskGraph"
			break
		}
		// Phase 1.5: write_analyzer dispatch. Produces WriteAnalysisIR
		// on Mutable so planner / verifier / reflector can read
		// task-shape facts (kind / scope / risk / constraints /
		// outcomes) directly. Failure is non-fatal — when the LLM
		// can't emit, downstream agents see Mutable.WriteAnalysisIR()
		// == nil and degrade to the existing AnalysisIR-only path.
		if used, err := o.runWriteAnalyzePhase(); err != nil {
			logging.Warning("[orchestrator] write_analyze degraded: %v (planner falls back to AnalysisIR-only context)", err)
		} else {
			stepsUsed += used
		}
		writeGraph := BuildWriteTaskGraph(o.busCtx.Mode, o.planPath, o.writeRetryBudget)
		o.busCtx.AnalysisIR.TaskGraph = writeGraph
		// Per-Run reset: best-known-good plan/report slot tracks
		// retry-loop state that is per-task; clear it so a previous
		// task's high-water mark cannot leak into this Run.
		o.busCtx.Mutable.ResetBestPlanReport()
		// Crash recovery (commit 6 P3 I2): if a prior Run on the
		// same plan persisted a best (plan, report) pair to disk
		// and the current Run is targeting that plan, seed the
		// in-memory latch from the disk copy. Without this a
		// process killed between iteration N's best-update and
		// iteration N+1's apply would lose the high-water mark
		// and surface iteration N+1's worse plan as the final
		// answer. Failure to load is non-fatal — we just start
		// with no latch, equivalent to pre-recovery behaviour.
		if o.planPath != "" {
			if bp, br, err := types.LoadBestPlanReportPair(o.planPath); err != nil {
				logging.Warning("[orchestrator] best-plan disk recovery failed: %v (continuing without latch)", err)
			} else if bp != nil && br != nil {
				o.busCtx.Mutable.SetBestPlanReport(bp, br)
				logging.Info("[orchestrator] best-plan restored from disk (plan=%s) — retry resumes from prior high-water mark", bp.ID)
			}
		}
		// Now that the write graph is installed, emit
		// EventAnalysisReady so the dock populates plan/apply/verify
		// node rows. The corresponding read-mode emission was
		// deliberately suppressed in phase 1 above.
		o.emitAnalysisReady()
	default:
		logging.Error("[orchestrator] unknown pipeline mode %q", o.busCtx.Mode)
		o.busCtx.TaskState.LastError = fmt.Sprintf("unknown pipeline mode %q", o.busCtx.Mode)
	}

	if o.busCtx.TaskState.LastError == "" {
		// Multi-phase fork (commit 20, stage II). When the
		// LLM emitted a sequential proposal AND we have a
		// PlanGroupStore wired AND we're in ModeApply, drive
		// runPhaseGroup instead of single runTaskPhase. The
		// gate's preconditions are all OR'd to false in the
		// single-phase / ModePlan / ModeVerify / no-store
		// case, so existing flows are byte-identical.
		if o.isMultiPhaseRun() {
			ir := o.busCtx.Mutable.WriteAnalysisIR()
			group := o.buildPlanGroupFromProposal(ir)
			logging.Info("[orchestrator] multi-phase run: group=%s phases=%d",
				group.ID, len(group.Phases))
			if err := o.runPhaseGroup(group, &stepsUsed); err != nil {
				logging.Error("[orchestrator] phase group %s: %v", group.ID, err)
				if o.busCtx.TaskState.LastError == "" {
					o.busCtx.TaskState.LastError = err.Error()
				}
			}
		} else if err := o.runTaskPhase(&stepsUsed); err != nil {
			logging.Error("[orchestrator] task phase error: %v", err)
			if o.busCtx.TaskState.LastError == "" {
				o.busCtx.TaskState.LastError = err.Error()
			}
		}
	}

	// Commit 51 Gap 3: end-of-Run answer-taxonomy reviewer dispatch.
	// Fires only on a clean read-mode Run that had at least one
	// retry event. Failure modes are intentionally swallowed — a
	// reviewer error must NEVER mask the answer the user is about
	// to receive. See runAnswerReviewerOnSuccess for the gates.
	o.runAnswerReviewerOnSuccess()

	// Commit 59 Batch E.1 (audit HIGH #12): surface learning-chain
	// failures at Run end. Pre-fix these were silently logged at
	// WARN level and lost; an operator running a long REPL session
	// could not tell that 80% of Runs had broken learning. The
	// summary line is at INFO level so it lands in the standard
	// log view alongside other end-of-Run events.
	if mu := o.busCtx.Mutable; mu != nil {
		failures := mu.LearningFailures()
		if len(failures) > 0 {
			stages := make([]string, 0, len(failures))
			for _, f := range failures {
				stages = append(stages, f.Stage)
			}
			logging.Info("[orchestrator] learning-chain failures this Run: %d (stages: %s) — cross-Run pitfall accumulation skipped for this Run",
				len(failures), strings.Join(stages, ", "))
		}
	}

	o.busCtx.TaskState.IsTerminal = true

	errMsg := ""
	if o.busCtx.TaskState.LastError != "" {
		errMsg = o.busCtx.TaskState.LastError
	}
	o.emit(render.Event{
		Kind:          render.EventPipelineEnd,
		Timestamp:     time.Now(),
		TraceID:       o.busCtx.TraceID,
		ToolCallCount: len(o.busCtx.ToolResults),
		MCPCallCount:  len(o.busCtx.MCPResponses),
		FactCount:     len(o.busCtx.RepoFacts),
		Error:         errMsg,
	})

	return o.busCtx, nil
}

// dynamicAnalyzeRetries scales the analyzer retry budget by the
// estimated number of sub-topics in the user's request. Multi-topic
// questions are more likely to fail the analyzer's coherence /
// quality gate on the first emit (the LLM has more independent
// fields to align), so granting one extra retry per pair of
// estimated sub-topics gives the gate-driven re-emit path room to
// converge without inflating the cost on simple single-topic
// questions. Capped at AgentSettings.MaxRetryBudgetCeil (default 5).
func (o *Orchestrator) dynamicAnalyzeRetries(base int) int {
	if base < 1 {
		base = 1
	}
	objective := ""
	if o.busCtx != nil && o.busCtx.Mutable != nil {
		objective = o.busCtx.Mutable.Objective()
	}
	if objective == "" {
		return base
	}
	est := agent.EstimateSubTopicCount(objective)
	if est < 2 {
		return base
	}
	extra := (est / 2) * o.settings.Agent.SubTopicRetryBudgetExtra
	adjusted := base + extra
	ceil := o.settings.Agent.MaxRetryBudgetCeil
	if ceil <= 0 {
		ceil = 5
	}
	if adjusted > ceil {
		adjusted = ceil
	}
	if adjusted > base {
		logging.Debug("[orchestrator] analyze retry scaling: estimated=%d sub-topics, retry budget %d → %d",
			est, base, adjusted)
	}
	return adjusted
}

// runAnalyzePhase dispatches the analyze stage with hard fail-loud
// retry semantics. Each attempt is counted; the loop exits early
// on a clean StageOutput (no Error, non-nil AnalysisIR). After the
// retry budget is exhausted the phase returns an error so Run
// terminates without entering the per-task phase.
func (o *Orchestrator) runAnalyzePhase() (int, error) {
	o.busCtx.PipelineStage = types.StageAnalyze
	o.busCtx.TaskState.Stage = types.StageAnalyze
	o.busCtx.TaskState.Missing = types.MissingUnderstanding

	// Approved-plan fast path: when the user has supplied a vetted
	// ChangePlan via --plan-file (CLI single-shot) or /approve (REPL),
	// the analyzer has nothing useful to do. The plan-mode pipeline
	// already classified the request that produced this plan; the
	// apply / verify stages do not consume AnalysisIR (the planner
	// would, but plan-stage SkipOnFirstVisit on these flows skips it
	// when planPath != ""). Running the analyzer here wastes ~30-60s
	// of LLM time, and worse: when the plan creates a NEW file the
	// analyzer's task_map fuzzy-matches the (yet-uncreated) file
	// against unrelated repo files at high score, surfacing
	// "Pre-scored relevant files" that mislead any downstream prompt
	// that consumes them. Install a stub IR so the Mode-switch in
	// Run() (line ~612) finds AnalysisIR != nil; TaskGraph is
	// overwritten by BuildWriteTaskGraph immediately afterwards so
	// the empty stub is never read.
	if (o.busCtx.Mode == types.ModeApply || o.busCtx.Mode == types.ModeVerify) && o.planPath != "" {
		logging.Info("[orchestrator] analyze skipped: %s mode with --plan-file / /approve (using stub IR)", string(o.busCtx.Mode))
		o.busCtx.AnalysisIR = &types.AnalysisIR{}
		return 0, nil
	}

	max := o.dynamicAnalyzeRetries(o.settings.MaxRetriesPerStage)
	if max < 1 {
		max = 1
	}
	// B6 (2026-05-10): retry storm detector. When the same gate
	// failure SHAPE (Fingerprint) repeats ≥ ceil(max/2) consecutive
	// attempts, the LLM is stuck in a loop and additional retries
	// are wasted LLM round-trips. Early-exit to the degraded path
	// with a user-actionable diagnostic. Forensic anchor:
	// 2026-05-10 chatpp-7d46dee4 log L1320 + L2714 (TWO turns
	// each burning the full 3-attempt budget on the same
	// subtopic_coherence gate, then degrading anyway).
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.6.
	storm := newAnalyzeRetryStormDetector(max)
	var lastErr string
	used := 0
	transientUsed := 0
	for attempt := 0; attempt < max; {
		// Terminal forcing on retry: signal the agent layer that this is
		// a re-dispatch after a "tool_choice=required produced no tool
		// call" failure so it can escalate the prompt + tool_choice
		// shape. Cleared inside dispatchStage so non-retry callers keep
		// attempt=0.
		o.emitStageRetryAttempt = attempt
		out, err := o.dispatchStage(types.StageAnalyze)
		if err != nil && o.busCtx.Mode == types.ModeRead && llm.IsStreamLevelRetryable(err) &&
			out != nil && out.Error == "" && out.AnalysisIR != nil {
			used++
			o.busCtx.AnalysisIR = out.AnalysisIR
			logging.Warning("[orchestrator] analyze transient dispatch error after usable IR; preserving analyzed request and continuing: %v", err)
			return used, nil
		}
		if err != nil && o.busCtx.Mode == types.ModeRead && llm.IsStreamLevelRetryable(err) {
			if transientUsed < o.transientRetryBudget {
				transientUsed++
				lastErr = err.Error()
				logging.Warning("[orchestrator] retrying analyze after transient dispatch error (%d/%d transient budget; semantic attempt %d/%d unchanged): %v",
					transientUsed, o.transientRetryBudget, attempt+1, max, err)
				o.emit(render.Event{
					Kind:       render.EventOrchestratorNotice,
					Timestamp:  time.Now(),
					Agent:      "orchestrator",
					NoticeKind: render.NoticeRetry,
					Reasoning:  softTransportRetryHintForStage(o.busCtx.Language, types.StageAnalyze),
				})
				continue
			}
			return used, fmt.Errorf("analyze transient dispatch failed after %d/%d retry attempt(s): %w",
				transientUsed, o.transientRetryBudget, err)
		}
		used++
		attempt++
		if err == nil && out != nil && out.Error == "" && out.AnalysisIR != nil {
			// Analyzer retries are attempt-scoped. applyStageOutput keeps
			// the first non-nil IR for degraded-recovery context, so a
			// later clean retry must explicitly promote its own IR before
			// downstream TaskGraph / EvidencePlan consumers run.
			o.busCtx.AnalysisIR = out.AnalysisIR
			return used, nil
		}
		if out != nil {
			lastErr = out.Error
		}
		if err != nil {
			lastErr = err.Error()
		}
		logging.Warning("[orchestrator] analyze attempt %d/%d failed: %s", attempt, max, lastErr)
		// Read-mode answer-taxonomy signal: every retry within
		// runAnalyzePhase records an event so the end-of-Run
		// answer_reviewer sees how hard the analyze stage worked
		// (commit 51 Gap 3). Recorded here, not at re-dispatch
		// time, so a successful re-attempt ALSO leaves a trace.
		if o.busCtx != nil && o.busCtx.Mutable != nil {
			o.busCtx.Mutable.AppendAnswerRetryEvent(string(types.StageAnalyze), lastErr)
		}
		// Fix-D / B7 (2026-05-10..11) — typed analyzer
		// auto-corrections. applyStageOutput stores AnalysisIR
		// write-once, so a failed retry can leave busCtx pointing at
		// stale gate data. Mirror the latest failed emit before any
		// deterministic correction or retry-storm accounting.
		if o.busCtx != nil && out != nil && out.AnalysisIR != nil {
			o.busCtx.AnalysisIR = out.AnalysisIR
			if o.autoCorrectAnalyzerStageOutput(out, attempt) {
				return used, nil
			}
		}
		// Storm detection: the AnalysisIR may carry a typed
		// QualityGate.Fingerprint. When non-empty, feed it into
		// the storm detector; same fingerprint ≥ threshold times
		// → break out early.
		if o.busCtx != nil && o.busCtx.AnalysisIR != nil {
			fp := o.busCtx.AnalysisIR.QualityGate.Fingerprint
			storm.observe(fp)
			if storm.exhausted() && attempt < max {
				logging.Warning("[orchestrator] analyze retry storm: fingerprint %q repeated %d×; breaking early to degraded path", fp, storm.repeatCount())
				return used, fmt.Errorf("analyze stage early-exit on retry storm (fingerprint=%s, attempts=%d): %s — %s", fp, used, lastErr, retryStormUserCaveat(o.busCtx))
			}
		}
	}
	// P7 (2026-05-10) — user-facing wrapper. Prior shape
	// "analyze stage exhausted after N attempt(s)" was a raw
	// orchestration phrase ("analyze stage" is internal stage codename;
	// "attempt(s)" is operator vocabulary). The wrapper appends a
	// CN/EN-aware prose hint so the user sees what happened + a
	// concrete next step. The original technical detail stays in
	// the error string so ops/eval can still grep it.
	userHint := analyzeExhaustedUserHint(o.busCtx)
	return used, fmt.Errorf("analyze stage exhausted after %d attempt(s): %s — %s", max, lastErr, userHint)
}

// analyzeExhaustedUserHint returns the CN/EN-aware advisory tail
// appended to the analyze-stage exhaustion error so users see
// actionable next steps (narrow the question / add log attachment /
// adjust active sub-repo set when multi-repo) instead of a bare
// "analyze stage exhausted" technical message.
//
// Red-line audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: lang + multiRepo bool gate the variants
//   - R4 generic: no project / case names
//   - R5: error wrapper, never writes answer body
//   - R6 no internal vocab: "分类" / "classification" are
//     user-vocab paraphrases of "analyze stage"
//   - R7: gives concrete next steps the user can take
//   - SST: shape mirrors retryStormUserCaveat
//   - CN+EN-only per 2026-05-10 user direction (each branch
//     mono-language; CLI command `/repos` rendered as inline
//     code identifier in both branches)
//
// r22AutoCorrectShapeSubject inspects the AnalysisIR's QualityGate
// for the R2.2 longform_scalar_subject shape contradiction. When
// found AND the IR carries a scalar AnswerSubject.Kind, the kind
// is cleared (set to SubjectUnknown with confidence 0) so the
// downstream auto-inference path (analyzer.go inferAnswerSubject)
// re-derives the right kind from question_kind.
//
// Returns true when a correction was made (caller should re-run
// gate); false when:
//   - the gate did NOT fire R2.2 (different failure mode)
//   - the AnswerSubject was already non-scalar / Unknown (nothing
//     to correct)
//
// Cross-language: operates on typed IR fields, no string-keyed
// signal so works regardless of question or repo language.
//
// Fix-D 2026-05-10. Design doc:
// docs/design/analyzer_failure_remediation.md §3.4.
func r22AutoCorrectShapeSubject(ir *types.AnalysisIR) bool {
	if ir == nil || !ir.QualityGate.Rejected {
		return false
	}
	// Walk every failed check. R2.2 must be present AND no other
	// check may be failed in the same report. The post-correction
	// re-run uses a nil resolver (the orchestrator can't easily
	// reconstruct the symbol resolver), and resolver-backed
	// coherence rules (R1.4 / R1.5 in subtopic_coherence) are
	// no-ops under nil resolver. Without this guard, an R2.2 fix
	// would silently bypass a co-firing coherence failure when
	// the re-run "passes" against an unrun resolver.
	hasR22 := false
	otherFails := 0
	for _, c := range ir.QualityGate.Checks {
		if c.Passed {
			continue
		}
		if c.Name == "shape_subject_coherence" && strings.HasPrefix(strings.TrimSpace(c.Detail), "R2.2 longform_scalar_subject:") {
			hasR22 = true
			continue
		}
		// Any other failed check (subtopic_coherence /
		// dag_closure / contract_complete / etc.) blocks the
		// auto-correct; the LLM-side retry path must surface
		// these too, and our fast-path re-run can't validate
		// them (nil resolver / lost dispatch context).
		otherFails++
	}
	if !hasR22 || otherFails > 0 {
		return false
	}
	// Only correct when the AnswerSubject is actually scalar; the
	// gate could fire for other reasons in the future and we don't
	// want to broaden the auto-correct surface.
	switch ir.RequestModel.AnswerSubject.Kind {
	case types.SubjectStringLiteral, types.SubjectNumeric, types.SubjectReturnValue:
		ir.RequestModel.AnswerSubject.Kind = types.SubjectUnknown
		ir.RequestModel.AnswerSubject.Confidence = 0
		return true
	}
	return false
}

func (o *Orchestrator) autoCorrectAnalyzerStageOutput(output *agent.StageOutput, attempt int) bool {
	if output == nil || output.AnalysisIR == nil || output.Error == "" {
		return false
	}
	mode := ""
	if o != nil && o.busCtx != nil && o.busCtx.Mode != "" {
		mode = string(o.busCtx.Mode)
	}
	if r14AutoCollapseEnumerationSubtopics(output.AnalysisIR) {
		logging.Warning("[orchestrator] analyze R1.4 auto-correct on attempt %d: collapsed single-axis enumeration sub_topics; re-running gate", attempt+1)
		if err := rebuildAnalysisIRCompiledArtifactsAfterAutoCorrect(output.AnalysisIR); err != nil {
			logging.Warning("[orchestrator] analyze R1.4 auto-correct rebuild failed: %v", err)
			return false
		}
		if rerunAnalyzerGateReport(output.AnalysisIR, mode) {
			clearAnalyzerStageOutputError(output)
			if o != nil && o.busCtx != nil {
				o.busCtx.TaskState.LastError = ""
				if o.busCtx.Mutable != nil {
					o.busCtx.Mutable.SetRequestModel(output.AnalysisIR.RequestModel)
				}
			}
			logging.Info("[orchestrator] analyze R1.4 auto-correct succeeded — gate now passes; continuing pipeline")
			return true
		}
		logging.Warning("[orchestrator] analyze R1.4 auto-correct re-run still rejected; falling through to retry storm detector")
	}
	// Fix-D (2026-05-10) — R2.2 auto-correction. When the
	// gate rejected the IR purely because of a scalar AnswerSubject in
	// a long-form family, and we're past the first attempt so the LLM
	// had a chance to revise on retry, AUTO-CLEAR the scalar
	// declaration and re-run the gate. The helper refuses mixed
	// failures so the nil resolver re-run cannot bypass
	// resolver-backed checks.
	if attempt >= 1 && r22AutoCorrectShapeSubject(output.AnalysisIR) {
		logging.Warning("[orchestrator] analyze R2.2 auto-correct on attempt %d: cleared scalar AnswerSubject.Kind; re-running gate", attempt+1)
		if err := rebuildAnalysisIRCompiledArtifactsAfterAutoCorrect(output.AnalysisIR); err != nil {
			logging.Warning("[orchestrator] analyze R2.2 auto-correct rebuild failed: %v", err)
			return false
		}
		if rerunAnalyzerGateReport(output.AnalysisIR, mode) {
			clearAnalyzerStageOutputError(output)
			if o != nil && o.busCtx != nil {
				o.busCtx.TaskState.LastError = ""
				if o.busCtx.Mutable != nil {
					o.busCtx.Mutable.SetRequestModel(output.AnalysisIR.RequestModel)
				}
			}
			logging.Info("[orchestrator] analyze R2.2 auto-correct succeeded — gate now passes; continuing pipeline")
			return true
		}
		logging.Warning("[orchestrator] analyze R2.2 auto-correct re-run still rejected; falling through to retry storm detector")
	}
	return false
}

func clearAnalyzerStageOutputError(output *agent.StageOutput) {
	if output == nil {
		return
	}
	output.Error = ""
	output.MissingPiece = types.MissingNone
	output.RetryHint = ""
}

// r14AutoCollapseEnumerationSubtopics inspects the AnalysisIR's
// QualityGate for the R1.4 axis_collapse shape. When the analyzer
// already typed the request as a single-axis enumeration and the
// only failing gate check is R1.4, it collapses SubTopics back to the
// canonical single-topic representation. The enumerated values remain
// available as Entities / DerivedEntities so downstream search keeps
// breadth, while the compiler no longer treats each value as an
// independently-answerable evidence lane.
//
// This is deliberately structural and language-agnostic:
//   - it consumes typed predicates/intent plus the schema-validated
//     gate code, never request-text keywords;
//   - it refuses mixed failures because the post-correction gate
//     re-run cannot reconstruct the resolver;
//   - it mirrors gate R1.4's self-judged cross-component carve-out.
func r14AutoCollapseEnumerationSubtopics(ir *types.AnalysisIR) bool {
	if ir == nil || !ir.QualityGate.Rejected {
		return false
	}
	rm := &ir.RequestModel
	if len(rm.SubTopics) < 2 || rm.Predicates.IsCrossComponent ||
		(!rm.Predicates.IsCategoryEnumeration && rm.Intent != types.IntentEnumerate) {
		return false
	}
	hasR14 := false
	for _, c := range ir.QualityGate.Checks {
		if c.Passed {
			continue
		}
		if c.Name == "subtopic_coherence" && strings.HasPrefix(strings.TrimSpace(c.Detail), "R1.4 axis_collapse:") {
			hasR14 = true
			continue
		}
		return false
	}
	if !hasR14 {
		return false
	}

	var subtopicTerms []string
	for _, st := range rm.SubTopics {
		subtopicTerms = append(subtopicTerms, st.Entities...)
		subtopicTerms = append(subtopicTerms, st.Scopes...)
	}
	rm.AnalyzerHints.Entities = dedupedStrings(append(rm.AnalyzerHints.Entities, subtopicTerms...))
	rm.AnalyzerHints.DerivedEntities = dedupedStrings(append(rm.AnalyzerHints.DerivedEntities, subtopicTerms...))
	rm.SubTopics = nil
	ir.QualityGate = types.GateReport{}
	return true
}

// rerunAnalyzerGateReport re-runs the analyzer gate on a
// structurally-corrected IR and writes the fresh report back. Resolver
// is intentionally nil; callers must prove that any resolver-backed
// failure was either removed structurally (R1.4) or absent (R2.2).
func rerunAnalyzerGateReport(ir *types.AnalysisIR, mode string) bool {
	if ir == nil {
		return false
	}
	report := gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{})
	ir.QualityGate = report
	return !report.Rejected
}

func analyzeExhaustedUserHint(busCtx *types.BusContext) string {
	lang := ""
	multiRepo := false
	if busCtx != nil {
		lang = busCtx.Language
		if mg, ok := busCtx.MultiGraph.(*multigraph.MultiGraph); ok && mg != nil && !mg.IsSingle() {
			multiRepo = true
		}
	}
	if preferZhMessage(lang) {
		if multiRepo {
			return "问题理解阶段反复失败，已无法继续。可尝试细化问题、补充日志/堆栈附件，或通过 `/repos` 调整活跃子仓集合后重提"
		}
		return "问题理解阶段反复失败，已无法继续。可尝试细化问题或补充日志/堆栈附件后重提"
	}
	if multiRepo {
		return "Question understanding repeatedly failed and cannot proceed. Try narrowing the question, attaching a log/stacktrace, or adjusting the active sub-repo set via `/repos` and re-asking"
	}
	return "Question understanding repeatedly failed and cannot proceed. Try narrowing the question or attaching a log/stacktrace and re-asking"
}

// newAnalyzeRetryStormDetector returns a detector seeded with a
// threshold of ceil(max/2). When the SAME non-empty fingerprint is
// observed `threshold` consecutive times, exhausted() returns true
// and the orchestrator's retry loop breaks early. Threshold floor
// is 2 — single-attempt budgets disable the detector implicitly
// because exhausted() can never reach 2.
func newAnalyzeRetryStormDetector(maxAttempts int) *analyzeRetryStormDetector {
	threshold := (maxAttempts + 1) / 2 // ceil(max/2)
	if threshold < 2 {
		threshold = 2
	}
	return &analyzeRetryStormDetector{max: maxAttempts, threshold: threshold}
}

type analyzeRetryStormDetector struct {
	max       int
	threshold int
	lastFP    string
	repeats   int
}

// observe records one attempt's fingerprint. Empty fingerprint resets
// the run counter (the LLM made progress in shape, even if the
// result still failed). Non-empty fingerprint matching the previous
// observation increments the run counter; differing fingerprint
// resets it to 1.
func (d *analyzeRetryStormDetector) observe(fp string) {
	if d == nil {
		return
	}
	if fp == "" {
		d.lastFP = ""
		d.repeats = 0
		return
	}
	if fp == d.lastFP {
		d.repeats++
		return
	}
	d.lastFP = fp
	d.repeats = 1
}

// exhausted reports whether the consecutive same-fingerprint streak
// has reached the threshold. Returns false when no fingerprint has
// been observed.
func (d *analyzeRetryStormDetector) exhausted() bool {
	return d != nil && d.repeats >= d.threshold
}

// repeatCount returns the current consecutive same-fingerprint streak.
func (d *analyzeRetryStormDetector) repeatCount() int {
	if d == nil {
		return 0
	}
	return d.repeats
}

// retryStormUserCaveat is the user-facing diagnostic the early-exit
// path appends to the analyze stage error. NO pipeline-internal
// vocab (R6); generic Chinese+English user vocabulary; gives
// concrete next steps the user can take.
//
// P7 (2026-05-10) refresh: split single-repo / multi-repo branches
// (single-repo MUST NOT mention `/repos` — misleading) and purify
// each language branch (no "REPL" / "active" English prose mid-CN
// sentence; CLI command names rendered as inline `code identifiers`
// stay legitimate per the same convention used by P6).
//
// Audit (R3/R4/R5/R6/R7/SST/CN+EN-only):
//   - R3 typed: fingerprint hex carries the typed signal upstream
//   - R4 no over-fitting: doesn't name specific gates / cases
//   - R5 no system backfill: explicitly states answer is degraded
//   - R6 no internal vocab: "结构性检查" / "structural check"
//     are user-vocabulary descriptions; "subtopic_coherence" never
//     surfaces
//   - R7 user intent: gives concrete next steps (narrow question,
//     attach log; multi-repo additionally suggests /repos)
//   - CN+EN-only: monolingual branches, /repos as inline code
func retryStormUserCaveat(busCtx *types.BusContext) string {
	lang := ""
	multiRepo := false
	if busCtx != nil {
		lang = busCtx.Language
		if mg, ok := busCtx.MultiGraph.(*multigraph.MultiGraph); ok && mg != nil && !mg.IsSingle() {
			multiRepo = true
		}
	}
	if preferZhMessage(lang) {
		if multiRepo {
			return "分析阶段在同一类结构性检查上反复未通过，已切换到部分降级答案。可尝试细化问题、补充日志/堆栈附件，或通过 `/repos focus <子仓名>` 缩小到单子仓后再次提问"
		}
		return "分析阶段在同一类结构性检查上反复未通过，已切换到部分降级答案。可尝试细化问题或补充日志/堆栈附件后再次提问"
	}
	if multiRepo {
		return "Analysis stage repeatedly failed on the same structural check; switched to a partially degraded answer. Try narrowing the question, attaching a log/stacktrace, or scoping to a single sub-repo via `/repos focus <slug>` and re-asking"
	}
	return "Analysis stage repeatedly failed on the same structural check; switched to a partially degraded answer. Try narrowing the question or attaching a log/stacktrace and re-asking"
}

// runWriteAnalyzePhase dispatches the write_analyze stage in
// write-mode Runs. Produces a WriteAnalysisIR on Mutable for the
// downstream write agents (planner / verifier / reflector) to read
// directly. Failure is non-fatal — a missing IR means downstream
// agents fall back to their existing AnalysisIR-only context, which
// is the historical behaviour. Returns the steps consumed and any
// dispatch error (the caller logs and continues, since this is a
// degradable enrichment stage rather than a hard pre-requisite).
//
// Approved-plan fast path: when --plan-file / /approve has supplied
// a vetted plan, the planner is going to be skipped on its first
// visit anyway (BuildWriteTaskGraph sets SkipOnFirstVisit=true) so
// running write_analyze burns LLM time for no consumer. Skip in
// that case for the same reason runAnalyzePhase installs a stub IR
// for the same path.
func (o *Orchestrator) runWriteAnalyzePhase() (int, error) {
	if (o.busCtx.Mode == types.ModeApply || o.busCtx.Mode == types.ModeVerify) && o.planPath != "" {
		logging.Info("[orchestrator] write_analyze skipped: %s mode with --plan-file / /approve", string(o.busCtx.Mode))
		return 0, nil
	}
	// Cost optimisation (commit 9 #3): if the active plan was emitted
	// with a pinned WriteAnalysisIR, applyPreHook will restore it
	// onto Mutable. If we're already past plan-load and Mutable
	// carries an IR, dispatching write_analyzer again is wasted LLM
	// work — skip and let downstream consumers read the pinned IR.
	if o.busCtx != nil && o.busCtx.Mutable != nil && o.busCtx.Mutable.WriteAnalysisIR() != nil {
		logging.Info("[orchestrator] write_analyze skipped: pinned IR already present on Mutable (reused from plan snapshot)")
		return 0, nil
	}
	o.busCtx.PipelineStage = types.StageWriteAnalyze
	o.busCtx.TaskState.Stage = types.StageWriteAnalyze

	// Modest retry budget for emit_write_analysis. The schema is
	// shallow (one tool call, ~10 fields), so two attempts cover
	// the common "LLM dropped a required field on first emit" case.
	// More retries are not justified — write_analyze is an
	// enrichment stage; if the second attempt still fails, the
	// downstream agents simply read AnalysisIR-only context, which
	// is the historical pre-commit-1 behaviour.
	//
	// Commit 10 #11: retry attempts past the first carry the prior
	// failure narrative on Mutable.AnalyzerRetryHint so the
	// write_analyzer's prompt can render "previous attempt failed
	// because: <reason>". Without this, retry was effectively a
	// blind redo and could repeat the same emit error.
	const maxAttempts = 2
	var lastErr error
	used := 0
	for attempt := 0; attempt < maxAttempts; attempt++ {
		used++
		o.emitStageRetryAttempt = attempt
		started := time.Now()
		out, err := o.dispatchStage(types.StageWriteAnalyze)
		elapsed := time.Since(started)
		if err == nil && (out == nil || out.Error == "") {
			if o.busCtx.Mutable.WriteAnalysisIR() != nil {
				return used, nil
			}
		}
		if out != nil && out.Error != "" {
			lastErr = fmt.Errorf("%s", out.Error)
		}
		if err != nil {
			lastErr = err
		}
		if attempt+1 < maxAttempts {
			logging.Warning("[orchestrator] write_analyze attempt %d/%d failed in %v: %v (retrying with prior-failure hint)",
				attempt+1, maxAttempts, elapsed, lastErr)
			// Seed AnalyzerRetryHint so the next dispatch's
			// prompt sees "## Previous attempt rejected" with
			// the rejection reason. The same channel the read
			// analyzer uses for its quality-gate retry hints —
			// reusing it keeps the agent prompt-rendering layer
			// uniform across read/write analyzers.
			if o.busCtx != nil && o.busCtx.Mutable != nil && lastErr != nil {
				o.busCtx.Mutable.SetAnalyzerRetryHint(
					fmt.Sprintf("Previous emit_write_analysis attempt was rejected: %v. Re-emit with all required fields filled (raw_request, task.kind, task.scope, task.summary, risk.affects_public_api, risk.changes_persistence, risk.changes_build_system, risk.overall).",
						lastErr))
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("write_analyze produced no IR after %d attempts", maxAttempts)
	}
	if o.busCtx != nil && o.busCtx.Mutable != nil && o.busCtx.Mutable.WriteAnalysisIR() == nil {
		ir := fallbackWriteAnalysisIR(o.busCtx.Mutable.Objective(), lastErr)
		o.busCtx.Mutable.SetWriteAnalysisIR(ir)
		logging.Warning("[orchestrator] write_analyze degraded; installed fallback WriteAnalysisIR (kind=%s scope=%s): %v",
			ir.Request.Task.Kind, ir.Request.Task.Scope, lastErr)
	}
	return used, lastErr
}

func fallbackWriteAnalysisIR(objective string, cause error) *types.WriteAnalysisIR {
	raw := strings.TrimSpace(types.StripConversationPrefix(objective))
	summary := "Follow the user's requested code-change task"
	if raw != "" {
		summary = firstLineTrimmed(raw)
	}
	if cause != nil {
		summary = summary + " (write-analysis fallback)"
	}
	return &types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			RawRequest: raw,
			Task: types.WriteTask{
				Kind:    types.WriteTaskMisc,
				Scope:   types.ScopeMicro,
				Summary: summary,
			},
			Risk: types.WriteRiskProfile{Overall: types.RiskBandLow},
			Constraints: []types.WriteConstraint{{
				Kind: "preserve_user_request",
				Note: "write_analyzer did not emit a structured IR; planner must follow the original user request directly and avoid inheriting read-analyzer diagnostic framing",
			}},
		},
		PhaseProposal: types.PhaseProposal{Split: "single"},
	}
}

func firstLineTrimmed(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len([]rune(s)) <= 140 {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:140])) + "..."
}

// runTaskPhase dispatches the single task graph for the run. After
// the v3 analyzer simplification every request maps to exactly one
// task, so this is a direct call into runTaskGraph — no loop, no
// pending-queue bookkeeping. The budget check still runs so a
// pathologically expensive analyze phase cannot silently starve the
// per-task path.
//
// runTaskGraph internally dispatches to runReadSchedulerLoop or
// runWriteSchedulerLoop based on whether the graph carries write
// nodes. T4 fold-in: write modes assemble their own TaskGraph
// upstream in Run(), so this function is mode-agnostic.
func (o *Orchestrator) runTaskPhase(stepsUsed *int) error {
	if *stepsUsed >= o.maxSteps {
		logging.Error("[orchestrator] global max-steps (%d) exhausted before task phase", o.maxSteps)
		o.busCtx.Mutable.SetResult("")
		return nil
	}

	// Strip the REPL conversation prefix before handing the objective
	// to the renderer. Mutable.Objective() carries the full
	// "## Prior conversation\n...\n## Current request\n<user text>"
	// blob in REPL mode; rendering that verbatim as the header line
	// replaced every clean sub-topic row with the whole prior-turn
	// memory dump the moment runTaskPhase ran. In single-shot mode
	// the strip is a no-op.
	objective := types.StripConversationPrefix(o.busCtx.Mutable.Objective())
	o.emit(render.Event{
		Kind:      render.EventObjectiveStarted,
		Timestamp: time.Now(),
		Objective: objective,
	})

	used := o.runTaskGraph(o.maxSteps - *stepsUsed)
	*stepsUsed += used
	return nil
}

// renderApplySummary formats the apply-stage Result message. Used
// by the REPL / single-shot renderer to show the user what landed
// in the worktree. Intentionally markdown-adjacent so the content
// survives glamour rendering without structural mangling.
//
// Bilingual: lang follows the BusContext.Language convention
// (zh-default, en-only flips to English). Pre-fix this rendered in
// English regardless of --lang; the customer's lang=zh terminal
// showed mixed Chinese / English depending on the rendering layer
// (REPL chrome was zh, orchestrator-injected sections were en).
func renderApplySummary(plan *types.ChangePlan, applied map[string]bool, worktreePath, recoveryRef string, willPreserve bool, lang string) string {
	zh := isLangZh(lang)
	if plan == nil {
		if zh {
			return "apply 阶段:没有可用的 ChangePlan"
		}
		return "apply phase: no ChangePlan available"
	}
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "## Apply 结果:%s\n\n", plan.ID)
		fmt.Fprintf(&b, "已在 worktree `%s` 中 apply **%d** 个变更。\n\n", worktreePath, len(applied))
		if len(plan.Changes) > 0 {
			b.WriteString("**变更列表**:\n\n")
			for i, c := range plan.Changes {
				status := "✓"
				if !applied[c.Path] {
					status = "✗"
				}
				fmt.Fprintf(&b, "%d. %s **%s** (`%s`)\n", i+1, status, c.Kind, c.Path)
			}
			b.WriteString("\n")
		}
		if willPreserve {
			fmt.Fprintf(&b, "worktree 已保留。落地:在提示符里输入 `/merge`,或直接粘贴 `!git cherry-pick %s` 回车执行(`!` 前缀执行系统命令)。\n",
				recoveryRef)
		} else {
			fmt.Fprintf(&b, "worktree 进程退出时销毁,但 apply commit 已 pin 到主仓 ref `%s`,bytes 可恢复:\n\n"+
				"- 在提示符里: `/merge`(主仓有未提交跟踪改动时先 commit 或 stash;未跟踪文件不影响)\n"+
				"- 复制粘贴执行: `!git cherry-pick %s`(`!` 前缀执行系统命令)\n\n"+
				"想保留 worktree 直接审阅,在配置文件里设 `pipeline_keep_worktree_on_success: true`。\n",
				recoveryRef, recoveryRef)
		}
		return b.String()
	}
	fmt.Fprintf(&b, "## Apply result: %s\n\n", plan.ID)
	fmt.Fprintf(&b, "Applied **%d** change(s) into worktree `%s`.\n\n", len(applied), worktreePath)
	if len(plan.Changes) > 0 {
		b.WriteString("**Changes**:\n\n")
		for i, c := range plan.Changes {
			status := "✓"
			if !applied[c.Path] {
				status = "✗"
			}
			fmt.Fprintf(&b, "%d. %s **%s** (`%s`)\n", i+1, status, c.Kind, c.Path)
		}
		b.WriteString("\n")
	}
	if willPreserve {
		fmt.Fprintf(&b, "Worktree preserved. Land via `/merge` at the prompt, or paste `!git cherry-pick %s` and press enter — the `!` prefix runs system commands inline.\n",
			recoveryRef)
	} else {
		fmt.Fprintf(&b, "The worktree dir is discarded on Run exit, but the apply commit is pinned in the main repo at ref `%s` so the bytes are recoverable:\n\n"+
			"- at the prompt: `/merge` (commit or stash any modified tracked files in main first; untracked files do not block)\n"+
			"- copy-paste: `!git cherry-pick %s` (`!` prefix runs the command inline)\n\n"+
			"To keep the worktree for direct inspection, set `pipeline_keep_worktree_on_success: true` in your config file.\n",
			recoveryRef, recoveryRef)
	}
	return b.String()
}

// isLangZh — mirror of the REPL-side language matcher to avoid an
// internal/orchestrator → internal/repl edge. zh-default; only
// "en" (case-insensitive) flips to English; everything else stays
// zh because the orchestrator's user-visible chrome is not the
// same surface as answer text and zh covers the majority user
// base. Same convention as messages.go::isZh in the REPL.
func isLangZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}

// persistPlanStatus updates the on-disk plan JSON's Status field
// (+ AppliedAt when provided) so /plan list reflects the current
// lifecycle state. Non-fatal: every failure is a warning because
// losing the status update doesn't invalidate the Run's actual
// apply/verify behaviour, just its audit trail.
//
// Skips silently when PlanPath is empty (Mutable-only plan mode
// without an on-disk sibling, typical of pure-memory tests) or
// when Mutable has no plan at all.
// persistPlanStatusWithApplied is the commit-42 variant of
// persistPlanStatus that ALSO records the AppliedPaths subset
// when the apply landed bytes for some-but-not-all targets.
// /plan show reads this slot to render which files landed on
// partially_applied plans (P0 P0 #2 fix). Nil appliedPaths
// leaves the on-disk slot untouched (back-compat with
// callers that don't know).
func (o *Orchestrator) persistPlanStatusWithApplied(status string, appliedAt *time.Time, appliedPaths []string) {
	if o.busCtx == nil {
		return
	}
	path := o.busCtx.PlanPath
	if path == "" {
		return
	}
	wt := ""
	if status == types.PlanStatusApplied &&
		(o.keepWorktreeOnSuccess || o.skipVerify) &&
		o.busCtx.WorktreePath != "" {
		wt = o.busCtx.WorktreePath
	}
	if err := types.UpdatePlanStatusOnDiskWithApplied(path, status, appliedAt, wt, appliedPaths); err != nil {
		logging.Warning("[orchestrator] plan status update failed: %v", err)
		return
	}
	logging.Info("[orchestrator] plan status persisted: %s applied_paths=%d", status, len(appliedPaths))
}

// collectAppliedTargetPaths returns the subset of plan.TargetPaths
// that successfully landed in the worktree per WriteClosure.
// Used by applyPostHook to populate ChangePlan.AppliedPaths on
// partially_applied transitions. Returns nil when no plan or
// no applied set.
func (o *Orchestrator) collectAppliedTargetPaths() []string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil || len(plan.TargetPaths) == 0 {
		return nil
	}
	applied := o.busCtx.Mutable.WriteClosure().AppliedSet()
	if len(applied) == 0 {
		return nil
	}
	out := make([]string, 0, len(plan.TargetPaths))
	for _, p := range plan.TargetPaths {
		if applied[p] {
			out = append(out, p)
		}
	}
	return out
}

func (o *Orchestrator) persistPlanStatus(status string, appliedAt *time.Time) {
	if o.busCtx == nil {
		return
	}
	path := o.busCtx.PlanPath
	if path == "" {
		// In REPL plan-mode flow, the PlanStore writes the file AFTER
		// Run returns, so there's nothing on disk to update from here.
		// The REPL layer is responsible for that post-Run save.
		return
	}
	// Persist the worktree path alongside the status ONLY when
	// Fix 4's preserve-on-success fires (status=applied + yaml knob
	// on + real worktree). Any other status leaves the field
	// untouched so a later /reject or retry doesn't leak a stale
	// path onto the plan JSON.
	wt := ""
	// Persist worktree path on every successful apply that ends up
	// preserved — that's keep_on_success yaml knob OR --skip-verify
	// (which implies preserve, see the outer Run() defer).
	// Without persisting, /merge / /worktree list / /verify <id>
	// can't find the worktree even though it's still on disk.
	if status == types.PlanStatusApplied &&
		(o.keepWorktreeOnSuccess || o.skipVerify) &&
		o.busCtx.WorktreePath != "" {
		wt = o.busCtx.WorktreePath
	}
	if err := types.UpdatePlanStatusOnDisk(path, status, appliedAt, wt); err != nil {
		logging.Warning("[orchestrator] plan status update failed: %v", err)
	} else {
		logging.Info("[orchestrator] plan status updated: %s → %s", path, status)
	}
}

// buildRetryHint synthesises the failure narrative the planner's
// next dispatch will see. Kept bounded (<1500 chars total) so it
// doesn't blow up the planner prompt while still carrying enough
// signal to steer a useful revision:
//
//   - FailureSummary (trimmed to 300 chars) — runner-agnostic
//     one-line verdict from the parser
//   - Top 3 failing tests with their first-line FailureDetail
//     excerpt (≤140 chars each) — specific errors the planner
//     can diff against the code it just wrote
//   - Files the previous plan modified (ChangePlan.TargetPaths,
//     cap 10) — the suspect list. Planner doesn't have to guess
//     which edits broke which test; every failing test touched
//     one of these files (transitively, at minimum)
//
// plan may be nil when the retry fires after an apply-phase
// failure that never produced a ChangeReport; report may be nil
// in the same case. Both branches degrade gracefully.
//
// Generalisation: only uses fields present on every runner's
// TestResult (AssertionID, Suite, FailureDetail) and on every
// ChangePlan (TargetPaths). No language-specific parsing.
// buildIterationRecord composes one ledger row from the previous
// attempt's plan + report. Verbatim summary text (no truncation —
// the planner needs the COMPLETE error, blob ref handles oversize
// inline rendering downstream). Empty fields when prev state is nil
// (apply-or-earlier failure with no plan / no report) — the planner
// reads a partial row as evidence that an attempt happened but
// didn't reach verify.
func buildIterationRecord(attempt int, plan *types.ChangePlan, report *types.ChangeReport) types.IterationRecord {
	rec := types.IterationRecord{
		Attempt:   attempt,
		Timestamp: time.Now(),
	}
	if plan != nil {
		rec.PlanID = plan.ID
		rec.PlanSummary = plan.Summary
		if len(plan.TargetPaths) > 0 {
			rec.ChangedFiles = append(rec.ChangedFiles, plan.TargetPaths...)
		}
	}
	if report != nil {
		for _, tr := range report.TestResults {
			if tr.Passed {
				rec.PassedCount++
			} else {
				rec.FailedCount++
			}
		}
		rec.FailureSummary = report.FailureSummary
		// Module D: when the runner blobbed the full stderr (large
		// output path in run_tests.go), thread the ref through so
		// the planner can call read_file with offset/limit to read
		// past the inline summary.
		rec.FailureSummaryBlobRef = report.FailureSummaryBlobRef
	}
	return rec
}

func buildRetryHint(report *types.ChangeReport, plan *types.ChangePlan, prevAttempt int) string {
	var b strings.Builder
	if report == nil {
		fmt.Fprintf(&b, "Previous attempt %d failed without producing a ChangeReport (apply or runner error). ", prevAttempt)
	} else {
		fmt.Fprintf(&b, "Previous attempt %d verify failed. ", prevAttempt)
		// Resource-exhaustion classifications get a kind-specific
		// header BEFORE the test summary so the planner reads "your
		// code allocated unboundedly / spun a CPU loop" first, not
		// the inevitable downstream "tests didn't complete" symptom.
		// Without this surfacing the planner re-derives the same
		// wrong corrective direction from buried stderr — the OOM
		// event that motivated this code is the textbook instance.
		// Failure-mode classifications stay as neutral one-line tags
		// — the model reads the raw FailureSummary below and decides
		// the fix. Pre-2026-04-30 these branches injected paragraphs
		// of system-prescribed corrective directions ("Most common
		// causes: ... Revise the plan to: ... DO NOT raise the cap")
		// — that violated the "feed error to model, let model decide"
		// red line. The structural label is enough: the model sees
		// the kind plus the stderr and chooses how to respond.
		switch report.FailureKind {
		case types.FailureKindOOM:
			b.WriteString("\n\n## Failure mode: out-of-memory (memory limit fired). ")
		case types.FailureKindCPULimit:
			b.WriteString("\n\n## Failure mode: CPU-time limit exceeded. ")
		case types.FailureKindTimeout:
			b.WriteString("\n\n## Failure mode: wall-clock timeout. ")
		}
		if report.FailureSummary != "" {
			// Pass the full FailureSummary verbatim — the model needs
			// complete unambiguous error context to decide the fix.
			// Pre-2026-04-30 truncation at 300 chars dropped the line
			// that named the actual error (pytest's `E ` line is
			// ~10-15 lines into a fixture trace), forcing the model
			// to guess from header noise. Operator-facing log lines
			// keep their own caps; THIS path is LLM-facing context.
			fmt.Fprintf(&b, "Summary: %s ", report.FailureSummary)
		}
		const (
			maxFailingTests = 3
			maxDetailChars  = 600 // upgraded from 140: the previous floor took only the first line, which is pytest's "self = <Test fixture>" header — useless. 600 fits the assertion + expected/actual + 1-2 stack frames.
		)
		shown := 0
		for _, tr := range report.TestResults {
			if tr.Passed {
				continue
			}
			if shown == 0 {
				b.WriteString("Failing tests:")
			}
			shown++
			// FailureDetail extraction. ExtractFailureSignal isolates
			// the actually-error-bearing lines (pytest E-marked lines,
			// go test "FAIL:" / panic frames, JUnit assertion-failed
			// messages) instead of the first line, which on most
			// runners is fixture / setup boilerplate. Bug fix from
			// Batch E robot-name analysis: previously `SplitN("\n",
			// 2)[0]` returned `self = <Test fixture>`, leaving the
			// reflector blind to the actual assertion failure.
			detail := ExtractFailureSignal(tr.FailureDetail, maxDetailChars)
			if tr.Suite != "" {
				fmt.Fprintf(&b, "\n  - %s (%s)", tr.AssertionID, tr.Suite)
			} else {
				fmt.Fprintf(&b, "\n  - %s", tr.AssertionID)
			}
			if detail != "" {
				fmt.Fprintf(&b, ": %s", detail)
			}
			if shown >= maxFailingTests {
				break
			}
		}
		if shown > 0 {
			b.WriteString("\n")
		}
	}
	if plan != nil && len(plan.TargetPaths) > 0 {
		const maxPaths = 10
		paths := plan.TargetPaths
		extra := 0
		if len(paths) > maxPaths {
			extra = len(paths) - maxPaths
			paths = paths[:maxPaths]
		}
		b.WriteString("\nFiles modified by the previous plan (suspect list — the regression is in the edits to these files):\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
		if extra > 0 {
			fmt.Fprintf(&b, "  - … (+%d more)\n", extra)
		}
		b.WriteString("\nRepair scope: keep the next plan focused on the failing batch and these suspect paths unless read-only investigation proves another file is required. If the scope expands, state the new path and reason in the plan summary.\n")
	}
	b.WriteString("Revise the plan to address these failures; do not repeat the same changes.")
	return b.String()
}

// buildRetryHintWithBest extends buildRetryHint with a "current vs
// best" delta when an earlier retry iteration produced a strictly-
// better (passed, total) score than the current iteration. Lets the
// planner see what it just lost — generic across runners since it
// only uses the (passed, total) score returned by ChangeReport.Score.
//
// When the current iteration IS the best (or no prior iteration was
// better), the output equals buildRetryHint's exactly so the existing
// behaviour is preserved on monotonic-improvement trajectories.
//
// On regression, the hint also includes a unified diff between the
// best plan's NewContent (or Patch) and the current iteration's,
// per overlapping path. Without the diff, the planner only sees
// "regressed from 51 to 45" and a file list — it has to reconstruct
// from memory which specific edits broke things. Showing the actual
// code delta closes that information gap so the LLM can revert
// targeted lines instead of re-deriving the whole solution.
//
// Bug provenance: Batch L forth-py — best at iter 1 was 51/54;
// iters 2-4 regressed to 46→45→0 because reflector hints were
// abstract ("preserve the lazy-resolution snapshot") and the LLM
// kept rebuilding the wrong pieces. Showing the diff would have
// surfaced "you changed lookup() in this exact way; the test that
// regressed exercises lookup()".
func buildRetryHintWithBest(curReport *types.ChangeReport, curPlan *types.ChangePlan, bestReport *types.ChangeReport, bestPlan *types.ChangePlan, prevAttempt int) string {
	base := buildRetryHint(curReport, curPlan, prevAttempt)
	if bestReport == nil || !bestReport.IsBetterThan(curReport) {
		return base
	}
	bp, bt := bestReport.Score()
	cp, ct := curReport.Score()
	delta := fmt.Sprintf(
		"\n\n## Regression detected\nCurrent attempt scored %d/%d; an earlier attempt in this retry loop scored %d/%d. The plan is moving in the WRONG direction. Re-examine which previous edits were correct and preserve them; isolate the change that introduced the regression.",
		cp, ct, bp, bt,
	)
	if bestPlan != nil && len(bestPlan.TargetPaths) > 0 {
		const maxPaths = 10
		paths := bestPlan.TargetPaths
		extra := 0
		if len(paths) > maxPaths {
			extra = len(paths) - maxPaths
			paths = paths[:maxPaths]
		}
		delta += "\n\nFiles modified by the best-scoring earlier plan (the better baseline you regressed FROM):\n"
		for _, p := range paths {
			delta += fmt.Sprintf("  - %s\n", p)
		}
		if extra > 0 {
			delta += fmt.Sprintf("  - … (+%d more)\n", extra)
		}
	}
	if diff := buildPlanContentDiff(bestPlan, curPlan, retryHintDiffMaxBytes); diff != "" {
		delta += "\n\nDiff from the best-scoring earlier plan to your current attempt (`-` = best, `+` = current — the lines marked `+` are what you added that REGRESSED). Revert them or refactor so the best version's behaviour is preserved while still addressing the failing tests:\n```diff\n" + diff + "```\n"
	}
	return base + delta
}

// retryHintDiffMaxBytes caps the unified-diff section appended to
// retry hints on regression. 4 KB fits typical small-file
// edits (an exercism-shape stub diff is usually <500 B; a complex
// refactor diff is usually <2 KB) while leaving headroom for the
// reflector critique + heuristic hint above it. Diffs that exceed
// the cap are truncated mid-hunk with an explicit "(truncated …)"
// marker; the planner still sees the first hunks, which usually
// carry the regression signal.
const retryHintDiffMaxBytes = 4096

// buildPlanContentDiff returns a unified diff between best and current
// plan contents, keyed by overlapping FileChange.Path. Empty string
// when either plan is nil, no paths overlap, or all overlapping paths
// have identical content.
//
// Per FileChange.Kind:
//   - "create" / "modify": diff the two NewContent blobs. Most common
//     case for exercism-shape tasks where the LLM rewrites a stub.
//   - "patch": diff the two Patch payloads (each is itself a unified
//     diff; the resulting "diff of diffs" is admittedly noisy but
//     still informative — you can see which hunks the planner removed
//     or added between iterations).
//   - "delete": no diff produced (kind change between best and current
//     is rare; if it happens, the path-list section above flags it).
//
// Output order: alphabetical by path so the prompt is deterministic
// across runs (otherwise prompt cache invalidates on every regenerate).
//
// The total diff is capped at maxBytes; once the cap is reached, the
// remaining paths are summarized as "(N more files truncated)" so the
// hint stays bounded.
func buildPlanContentDiff(best *types.ChangePlan, current *types.ChangePlan, maxBytes int) string {
	if best == nil || current == nil {
		return ""
	}
	bestByPath := make(map[string]types.FileChange, len(best.Changes))
	for _, c := range best.Changes {
		bestByPath[c.Path] = c
	}
	overlapping := make([]string, 0, len(current.Changes))
	for _, c := range current.Changes {
		if _, ok := bestByPath[c.Path]; ok {
			overlapping = append(overlapping, c.Path)
		}
	}
	if len(overlapping) == 0 {
		return ""
	}
	sort.Strings(overlapping)
	curByPath := make(map[string]types.FileChange, len(current.Changes))
	for _, c := range current.Changes {
		curByPath[c.Path] = c
	}
	var b strings.Builder
	truncatedAt := -1
	for i, p := range overlapping {
		if b.Len() >= maxBytes {
			truncatedAt = i
			break
		}
		bc := bestByPath[p]
		cc := curByPath[p]
		var bestText, curText string
		switch {
		case bc.Kind == "patch" || cc.Kind == "patch":
			// Both kind=patch (or kind transitioned) — diff the patch
			// payloads themselves so the planner sees how the patch
			// content shifted. If exactly one side has Patch and the
			// other has NewContent, fall through to NewContent diff
			// against empty for the patch side (rough but informative).
			bestText = bc.Patch
			curText = cc.Patch
			if bestText == "" {
				bestText = bc.NewContent
			}
			if curText == "" {
				curText = cc.NewContent
			}
		default:
			bestText = bc.NewContent
			curText = cc.NewContent
		}
		if bestText == curText {
			continue
		}
		d := udiff.Unified("best/"+p, "current/"+p, bestText, curText)
		if d == "" {
			continue
		}
		// Keep the diff bounded per file too — a single 10K-line file
		// rewrite shouldn't crowd out other paths.
		const perFileCap = 2048
		if len(d) > perFileCap {
			d = d[:perFileCap] + "\n… (per-file diff truncated)\n"
		}
		// If appending d would overflow the total cap, truncate to
		// what fits + a marker.
		if b.Len()+len(d) > maxBytes {
			remaining := maxBytes - b.Len()
			if remaining > 64 {
				b.WriteString(d[:remaining])
				b.WriteString("\n… (truncated)\n")
			}
			truncatedAt = i + 1
			break
		}
		b.WriteString(d)
	}
	if truncatedAt > 0 && truncatedAt < len(overlapping) {
		fmt.Fprintf(&b, "\n(+%d more files omitted)\n", len(overlapping)-truncatedAt)
	}
	return b.String()
}

// captureBaseline runs the project's test suite against the
// pre-apply worktree and installs the result on
// Mutable.BaselineReport. Called from the apply stage hook substep 3b
// when baselineCaptureEnabled is true.
//
// Data flow:
//  1. Call tool.RunTests{}.Execute — it installs the report on
//     Mutable.ChangeReport (the shared slot verify will populate
//     later) and returns a ToolResult that we ignore here
//     (Mutable is the source of truth).
//  2. Move the installed report from ChangeReport → BaselineReport
//     via SetBaselineReport + ResetChangeReport. The post-apply
//     verify phase will then fill ChangeReport fresh, and
//     CritNoRegression in criterion.Env will see both halves.
//  3. Optionally persist the baseline to disk as
//     <plan-id>.baseline.json beside the plan file for operator
//     audit. Disk failure is a warning, not fatal.
//
// Non-fatal on error: a missing test runner, a pre-existing test
// failure, or an unparseable output surfaces a warning and we
// proceed without a baseline. evalNoRegression short-circuits to
// Satisfied=true when BaselineReport is nil, so the missing data
// does not perturb the verify gate.
//
// Concurrency: called while RepoRoot is swapped to the worktree,
// so run_tests executes inside the sandboxed copy (same invariant
// verify relies on). The swap is restored by the outer defer.
func (o *Orchestrator) captureBaseline() {
	logging.Info("[orchestrator] apply phase: capturing pre-apply baseline test snapshot")
	runTests := &tool.RunTests{}
	// Empty params → run all tests with default timeout.
	res, err := runTests.Execute(o.busCtx, nil)
	if err != nil {
		logging.Warning("[orchestrator] baseline capture: run_tests error: %v", err)
		return
	}
	if !res.Success {
		// "Not success" here includes the normal case of a test
		// suite that has some pre-existing failures. That's still
		// a valid baseline — we want to record it so evalNoRegression
		// can distinguish "was already broken" from "regressed".
		logging.Info("[orchestrator] baseline capture: %s", res.Summary)
	}
	baseline := o.busCtx.Mutable.ChangeReport()
	if baseline == nil {
		logging.Warning("[orchestrator] baseline capture: run_tests returned but ChangeReport slot is empty; skipping")
		return
	}
	// Move ChangeReport → BaselineReport. After this, the slot is
	// empty again and the verify stage hook populates it fresh from the
	// post-apply test run.
	o.busCtx.Mutable.SetBaselineReport(baseline)
	o.busCtx.Mutable.ResetChangeReport()
	logging.Info("[orchestrator] baseline captured: %d test results, passed=%v",
		len(baseline.TestResults), baseline.Passed)
	o.saveBaselineReport(baseline)
}

// saveBaselineReport persists the pre-apply snapshot to disk as
// <plan-dir>/<plan-id>.baseline.json, paired with the plan JSON
// and the later .report.json. Failure is logged as warning and
// the in-memory baseline stays intact — evalNoRegression consumes
// Mutable.BaselineReport directly, not the disk file.
func (o *Orchestrator) saveBaselineReport(baseline *types.ChangeReport) {
	if baseline == nil || baseline.PlanID == "" {
		return
	}
	if o.busCtx == nil || o.busCtx.PlanPath == "" {
		// No plan dir resolvable (plan-mode e2e without --plan-file);
		// skip silently because the REPL will never re-read this file.
		return
	}
	path := filepath.Join(filepath.Dir(o.busCtx.PlanPath), baseline.PlanID+".baseline.json")
	if err := types.WriteChangeReportToFile(baseline, path); err != nil {
		logging.Warning("[orchestrator] baseline disk save failed: %v", err)
		return
	}
	logging.Info("[orchestrator] baseline report saved: %s", path)
}

// saveChangeReport writes the verify-stage report to disk. Target
// path convention: same directory as the plan (typically
// <runtime-anchor>/plans/) with a .report.json suffix keyed off
// plan.ID. Failures are logged but do not abort verify — the
// report still lives on Mutable and the stdout summary.
func (o *Orchestrator) saveChangeReport(report *types.ChangeReport) {
	if report == nil {
		return
	}
	if report.PlanID == "" {
		logging.Warning("[orchestrator] skipping ChangeReport disk save: PlanID empty")
		return
	}
	// Derive the plan directory in priority order:
	//  1. busCtx.PlanPath — set on first apply (--plan-file mode)
	//  2. o.reportDir — captured at Run entry from o.planPath. Survives
	//     the verify→plan retry loop where clearForReplan wipes
	//     busCtx.PlanPath but cannot touch this stable orchestrator
	//     field. This is the load-bearing fallback that lets
	//     restoreBestIfRegressed persist the restored ChangeReport on
	//     terminal failure even after multiple retry iterations.
	//  3. (skip with log) — plan-mode e2e flow with no on-disk artifact.
	var planDir string
	if o.busCtx.PlanPath != "" {
		planDir = filepath.Dir(o.busCtx.PlanPath)
	} else if o.reportDir != "" {
		planDir = o.reportDir
	}
	if planDir == "" {
		logging.Warning("[orchestrator] skipping ChangeReport disk save: no plan dir resolvable (plan-mode e2e path)")
		return
	}
	reportPath := filepath.Join(planDir, report.PlanID+".report.json")
	if err := types.WriteChangeReportToFile(report, reportPath); err != nil {
		logging.Warning("[orchestrator] ChangeReport disk save failed: %v", err)
		return
	}
	logging.Info("[orchestrator] ChangeReport saved: %s", reportPath)
}

// renderVerifyFailure builds the Mutable.Result message for a
// verify-stage failure. Three blocks, each at most a few lines:
//
//  1. Header — plain language ("测试未通过" / "Tests did not pass"),
//     no internal "Verify" jargon.
//  2. Reason — exactly ONE source: report.FailureSummary if non-
//     empty, else the agent-side message with the "verify failed: "
//     prefix stripped, else a count-only fallback. Capped at
//     verifyFailureSummaryMaxChars so a multi-megabyte stderr dump
//     cannot drown the rest of the prompt.
//  3. Failing test list — only when failing test names add
//     information beyond the summary. Skipped entirely when every
//     failing test name already appears verbatim in the summary
//     (otherwise the user reads the same names twice). Capped at
//     verifyFailureMaxNamesShown.
//  4. Next step — one short sentence pointing at the retry path.
//
// Pre-2026-04-30 this rendered as: "Verify FAILED" header + the full
// summary + the literal "agentError" (which started with the same
// summary again as a "verify failed: ..." prefix, so users saw the
// reason printed twice) + a 10-name list (which usually duplicated
// names already in the summary) + a tip. Three sources of the same
// reason in one block.
func renderVerifyFailure(report *types.ChangeReport, agentError, lang string) string {
	zh := isLangZh(lang)
	var b strings.Builder

	// Header — uses the same "did not pass" wording as the stage
	// row's failed phrase so the inline message and the dock label
	// agree.
	if zh {
		b.WriteString("## 测试未通过\n\n")
	} else {
		b.WriteString("## Tests did not pass\n\n")
	}

	// Reason — single source, capped.
	reason := strings.TrimSpace(verifyFailureReason(report, agentError))
	if reason != "" {
		if len([]rune(reason)) > verifyFailureSummaryMaxChars {
			rs := []rune(reason)
			reason = string(rs[:verifyFailureSummaryMaxChars]) + "…"
		}
		b.WriteString(reason)
		b.WriteString("\n\n")
	}

	// Failing test list — skipped when redundant with the reason.
	if report != nil {
		failedNames := failingAssertionNames(report.TestResults)
		if len(failedNames) > 0 && !reasonNamesEveryFailure(reason, failedNames) {
			shown := failedNames
			if len(shown) > verifyFailureMaxNamesShown {
				shown = shown[:verifyFailureMaxNamesShown]
			}
			if zh {
				fmt.Fprintf(&b, "失败测试: %s", strings.Join(shown, ", "))
				if len(failedNames) > len(shown) {
					fmt.Fprintf(&b, " (还有 %d 个)", len(failedNames)-len(shown))
				}
				b.WriteString("\n\n")
			} else {
				fmt.Fprintf(&b, "Failing tests: %s", strings.Join(shown, ", "))
				if len(failedNames) > len(shown) {
					fmt.Fprintf(&b, " (+%d more)", len(failedNames)-len(shown))
				}
				b.WriteString("\n\n")
			}
		}
	}

	// Next step — one short line.
	if zh {
		b.WriteString("下一步:`/mode plan` 后再发请求,失败上下文会自动带进去。")
	} else {
		b.WriteString("Next: `/mode plan` and re-send the request; failure context is carried in automatically.")
	}
	return b.String()
}

// verifyFailureSummaryMaxChars caps the runner stderr / failure
// summary the user sees inline. Anything past this is truncated with
// "…" — the full content stays in .codrax/plans/<id>.report.json.
// 800 runes is roughly 12-15 visual lines at typical widths, enough
// for a panic + 5 stack frames or 3 assertion explanations.
const verifyFailureSummaryMaxChars = 800

// verifyFailureMaxNamesShown caps the "Failing tests: a, b, c" list.
// 5 is enough to disambiguate without overwhelming when the runner
// emitted dozens of test names.
const verifyFailureMaxNamesShown = 5

// verifyFailureReason picks ONE source for the inline failure
// reason. Priority:
//
//  1. report.FailureSummary — already curated by the runner parser.
//  2. agentError minus the "verify failed: " prefix (which is the
//     verifier's structural wrapper around (1); when (1) is empty
//     this fallback at least carries the count line).
//  3. Count fallback — N test(s) failed.
//  4. Empty.
//
// Splitting these three sources into a helper makes the dedup
// rule above ("don't append a redundant test list") readable and
// individually unit-testable.
func verifyFailureReason(report *types.ChangeReport, agentError string) string {
	if report != nil && strings.TrimSpace(report.FailureSummary) != "" {
		return report.FailureSummary
	}
	clean := strings.TrimSpace(agentError)
	clean = strings.TrimPrefix(clean, "verify failed: ")
	if clean != "" {
		return clean
	}
	if report != nil {
		failed := 0
		for _, r := range report.TestResults {
			if !r.Passed {
				failed++
			}
		}
		if failed > 0 {
			return fmt.Sprintf("%d test(s) failed", failed)
		}
	}
	return ""
}

// failingAssertionNames returns the AssertionIDs of failing tests
// in the order they appear in the report. Empty slice when nothing
// failed. Helper so the dedup check below operates on a clean list
// without re-walking TestResults at every comparison.
func failingAssertionNames(results []types.TestResult) []string {
	var names []string
	for _, r := range results {
		if r.Passed {
			continue
		}
		if r.AssertionID != "" {
			names = append(names, r.AssertionID)
		}
	}
	return names
}

// reasonNamesEveryFailure reports whether every failing assertion
// name already appears verbatim in the reason text. When true, the
// caller should skip the explicit "Failing tests: ..." list because
// it would repeat names the user just read in the summary.
//
// Conservative: requires every name to be present (any one missing
// → the list adds information and should be shown). Substring
// match because some runner summaries embed names in larger
// context like "FAIL: TestX (0.02s)".
func reasonNamesEveryFailure(reason string, names []string) bool {
	if reason == "" || len(names) == 0 {
		return false
	}
	for _, n := range names {
		if !strings.Contains(reason, n) {
			return false
		}
	}
	return true
}

// renderVerifySuccess renders a short "all good" note appended to
// the apply summary. Kept compact so it pairs visually with the
// apply-phase output already in Mutable.Result. Bilingual.
//
// Header wording mirrors renderVerifyFailure's "测试未通过" /
// "Tests did not pass" so the success and failure surfaces use the
// SAME vocabulary axis ("测试通过" ↔ "测试未通过"); previously this
// function used "Verify PASSED" / "Verify 通过" which leaked the
// internal stage name.
func renderVerifySuccess(report *types.ChangeReport, lang string) string {
	zh := isLangZh(lang)
	if report == nil {
		if zh {
			return "\n## 测试通过\n\n本次未产出测试报告,验证阶段已完成但没有可显示的细节。\n"
		}
		return "\n## Tests verified\n\nNo report produced; the verify step completed but emitted no details.\n"
	}
	total := len(report.TestResults)
	if zh {
		return fmt.Sprintf("\n## 测试通过\n\n%d 个测试通过。报告已存到 .codrax/plans/%s.report.json。\n",
			total, report.PlanID)
	}
	return fmt.Sprintf("\n## Tests verified\n\n%d test(s) passed. Report saved to .codrax/plans/%s.report.json.\n",
		total, report.PlanID)
}

// renderVerifyUnverified is the "verify ran cleanly but no tests
// were discovered for the changed code" message. The plan's bytes
// are on disk in the worktree, but no assertion proved they work.
// Deliberately worded so the operator immediately understands this
// is NOT a failure (the change applied) and NOT a success (the
// change is unverified) — it's an explicit middle state requiring
// either /merge (accept as-is) or adding tests (verify
// retroactively).
func renderVerifyUnverified(report *types.ChangeReport, lang string) string {
	zh := isLangZh(lang)
	runners := ""
	planID := ""
	if report != nil {
		runners = strings.Join(report.NoTestsRunners, ", ")
		planID = report.PlanID
	}
	if zh {
		return fmt.Sprintf("\n## 未验证 (unverified)\n\n"+
			"代码改动已落到 worktree,但测试运行器 (%s) 没有发现任何测试,所以没有断言验证过这次改动。\n\n"+
			"建议:\n"+
			"- 添加测试覆盖再 /verify,或\n"+
			"- 直接 /merge 接受这次改动,或\n"+
			"- /reject 退回到 plan。\n\n"+
			"报告已存到 .codrax/plans/%s.report.json。\n",
			runners, planID)
	}
	return fmt.Sprintf("\n## Unverified\n\n"+
		"The change applied to the worktree, but the test runner (%s) discovered zero tests "+
		"for the changed code, so no assertion verified the change.\n\n"+
		"Next steps:\n"+
		"- add test coverage and /verify, or\n"+
		"- /merge to accept the change as-is, or\n"+
		"- /reject to roll back.\n\n"+
		"Report saved to .codrax/plans/%s.report.json.\n",
		runners, planID)
}

// renderChangePlanSummary formats a ChangePlan as a human-readable
// multi-line string suitable for the REPL or single-shot stdout.
// Day 5 ships a deliberately simple format (markdown-adjacent) —
// richer rendering can land later if user feedback demands it.
// Kept internal to orchestrator because no downstream package
// consumes the rendered form.
func renderChangePlanSummary(plan *types.ChangePlan, lang string) string {
	if plan == nil {
		return ""
	}
	zh := isLangZh(lang)
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "## 提议的 ChangePlan:%s\n\n", plan.ID)
		if plan.Request != "" {
			fmt.Fprintf(&b, "**请求**:%s\n\n", plan.Request)
		}
		if plan.Summary != "" {
			fmt.Fprintf(&b, "**摘要**:%s\n\n", plan.Summary)
		}
		if len(plan.Changes) > 0 {
			b.WriteString("**变更列表**:\n\n")
			for i, c := range plan.Changes {
				fmt.Fprintf(&b, "%d. **%s** (`%s`) — %s\n", i+1, c.Kind, c.Path, c.Rationale)
			}
			b.WriteString("\n")
		}
		if len(plan.TargetPaths) > 0 {
			fmt.Fprintf(&b, "**目标路径**:%d 个文件 — %s\n\n",
				len(plan.TargetPaths), strings.Join(plan.TargetPaths, ", "))
		}
		if len(plan.AcceptanceTests) > 0 {
			b.WriteString("**验收测试**:\n\n")
			for _, t := range plan.AcceptanceTests {
				fmt.Fprintf(&b, "- %s\n", t)
			}
			b.WriteString("\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "## Proposed change plan: %s\n\n", plan.ID)
	if plan.Request != "" {
		fmt.Fprintf(&b, "**Request**: %s\n\n", plan.Request)
	}
	if plan.Summary != "" {
		fmt.Fprintf(&b, "**Summary**: %s\n\n", plan.Summary)
	}
	if len(plan.Changes) > 0 {
		b.WriteString("**Changes**:\n\n")
		for i, c := range plan.Changes {
			fmt.Fprintf(&b, "%d. **%s** (`%s`) — %s\n", i+1, c.Kind, c.Path, c.Rationale)
		}
		b.WriteString("\n")
	}
	if len(plan.TargetPaths) > 0 {
		fmt.Fprintf(&b, "**Target paths**: %d file(s) — %s\n\n",
			len(plan.TargetPaths), strings.Join(plan.TargetPaths, ", "))
	}
	if len(plan.AcceptanceTests) > 0 {
		b.WriteString("**Acceptance tests**:\n\n")
		for _, t := range plan.AcceptanceTests {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// runTaskGraph dispatches to the read or write scheduler loop based
// on whether the TaskGraph carries write nodes (NodePlan / NodeApply
// / NodeVerify). Read TaskGraphs come from the analyzer; write
// TaskGraphs come from BuildWriteTaskGraph emitted by Run() before
// reaching this entry point.
func (o *Orchestrator) runTaskGraph(stepBudget int) int {
	ir := o.busCtx.AnalysisIR
	if ir == nil || len(ir.TaskGraph.Nodes) == 0 {
		// Defensive: analyzer (read) or BuildWriteTaskGraph (write)
		// should always produce a non-empty TaskGraph; an empty graph
		// means upstream failed and we cannot execute the task.
		logging.Error("[orchestrator] task: empty TaskGraph — upstream failed to produce a valid graph")
		o.busCtx.Mutable.SetResult("")
		o.busCtx.TaskState.LastError = "empty TaskGraph"
		return 0
	}
	if IsWriteGraph(ir.TaskGraph) {
		return o.runWriteSchedulerLoop(stepBudget)
	}
	return o.runReadSchedulerLoop(stepBudget)
}

func (o *Orchestrator) investigationStructurallyEmpty() bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	ta := o.busCtx.Mutable.TurnAArtifacts()
	if len(o.busCtx.EvidenceItems) > 0 {
		return false
	}
	if ta == nil {
		return false
	}
	return agent.InvestigationStructurallyEmpty(ta, nil)
}

func (o *Orchestrator) structurallyEmptyInvestigationMessage() string {
	return "investigation structurally empty: the previous explore attempt produced no successful read/search/evidence results. Re-run exploration and produce at least one grounded investigation result before extract/finalize."
}

func (o *Orchestrator) handleStructurallyEmptyInvestigation(state *graphState, finID string) (*agent.StageOutput, string, bool) {
	if !o.investigationStructurallyEmpty() {
		return nil, "", false
	}
	msg := o.structurallyEmptyInvestigationMessage()
	logging.Warning("[orchestrator] %s", msg)
	if state != nil && !state.retryBudgetExhausted() && o.busCtx != nil && o.busCtx.AnalysisIR != nil {
		if finID != "" {
			state.requeue(finID)
		}
		for _, n := range o.busCtx.AnalysisIR.TaskGraph.Nodes {
			if n.Type == types.NodeFinalize {
				continue
			}
			switch state.status[n.ID] {
			case nodeDone, nodeFailed:
				state.requeue(n.ID)
			}
		}
		state.recordRetry()
		o.emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      "orchestrator",
			NoticeKind: render.NoticeRetry,
			Reasoning:  softRetryHintMessage(o.busCtx.Language),
		})
		return nil, msg, true
	}
	return &agent.StageOutput{
		FinalAnswer: structurallyEmptyAnswerForUser(o.busCtx.Language),
		Error:       msg,
	}, "", true
}

func readExploreOutputRequestsFactRetry(output *agent.StageOutput) bool {
	if output == nil || output.MissingPiece != types.MissingFacts {
		return false
	}
	if strings.TrimSpace(output.RetryHint) == "" {
		return false
	}
	return output.SignalUpdates == nil || !output.SignalUpdates.HasEnoughFacts
}

func (o *Orchestrator) requeueExploreWindowForFactRetry(state *graphState, window []*types.TaskNode, output *agent.StageOutput) bool {
	if !readExploreOutputRequestsFactRetry(output) {
		return false
	}
	if o.acceptedSourceInventoryClosureSuppressesReconcileFactRetry(window) {
		logging.Info("[orchestrator] ignored reconcile fact retry because accepted source-inventory closure already covers an exact candidate universe")
		return false
	}
	if state == nil || state.retryBudgetExhausted() {
		logging.Warning("[orchestrator] explore requested fact retry but retry budget is exhausted; continuing toward finalize")
		return false
	}
	if checkpoint := o.buildExploreFactRetryContinuationHint(output); strings.TrimSpace(checkpoint) != "" {
		output.RetryHint = prependRetryHint(checkpoint, output.RetryHint)
		logging.Debug("[orchestrator] explore fact retry checkpoint installed len=%d", len(checkpoint))
	}
	for _, n := range window {
		if n == nil {
			continue
		}
		state.requeue(n.ID)
	}
	state.recordRetry()
	logging.Info("[orchestrator] explore requested fact retry; requeued %d node(s)", len(window))
	return true
}

// structurallyEmptyAnswerForUser returns a user-facing prose message
// for the degenerate case where the explore phase produced no
// successful read / search / evidence — the operator-style msg is
// preserved on out.Error for telemetry, but the user sees only
// natural-language prose.
func structurallyEmptyAnswerForUser(lang string) string {
	if isChineseLang(lang) {
		return "系统未能就这个问题收集到足够的代码证据，无法给出可靠回答。建议补充更具体的描述或限定检索范围后重试。"
	}
	return "The system could not gather enough code evidence to answer this question reliably. Please refine the request or narrow the search scope and retry."
}

func appendFinalizerTransientFailureCaveat(answer, lang string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return answer
	}
	if isChineseLang(lang) {
		return answer + "\n\n## 补充说明\n\n后续成文重试因为模型响应中断未能完成，系统保留上一版草稿给你参考；其中可能仍有未完全通过结构化校验的表达。"
	}
	return answer + "\n\n## Note\n\nA later final-answer retry was interrupted by the model response stream, so the system preserved the previous draft for reference; some structural checks may still be incomplete."
}

func appendRuntimeDispatchAdvisoriesToAnswer(answer string, advisories []string, lang string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" || len(advisories) == 0 {
		return answer
	}
	unique := make([]string, 0, len(advisories))
	seen := map[string]struct{}{}
	for _, advisory := range advisories {
		advisory = strings.TrimSpace(advisory)
		if advisory == "" {
			continue
		}
		if strings.Contains(answer, advisory) {
			continue
		}
		if _, ok := seen[advisory]; ok {
			continue
		}
		seen[advisory] = struct{}{}
		unique = append(unique, advisory)
	}
	if len(unique) == 0 {
		return answer
	}
	if isChineseLang(lang) {
		var b strings.Builder
		b.WriteString(answer)
		b.WriteString("\n\n## 运行提示\n\n")
		for _, advisory := range unique {
			b.WriteString("- ")
			b.WriteString(advisory)
			b.WriteByte('\n')
		}
		return strings.TrimSpace(b.String())
	}
	var b strings.Builder
	b.WriteString(answer)
	b.WriteString("\n\n## Run Note\n\n")
	for _, advisory := range unique {
		b.WriteString("- ")
		b.WriteString(advisory)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func appendUniqueRuntimeAdvisory(advisories []string, advisory string) []string {
	advisory = strings.TrimSpace(advisory)
	if advisory == "" {
		return advisories
	}
	for _, existing := range advisories {
		if strings.TrimSpace(existing) == advisory {
			return advisories
		}
	}
	return append(advisories, advisory)
}

func runtimeDispatchAdvisoryForStage(stage types.PipelineStage, err error, lang string) string {
	if !isLikelyTransientModelDispatchFailure(err) {
		return ""
	}
	stageName := localizedPipelineStageName(stage, lang)
	if isChineseLang(lang) {
		return fmt.Sprintf("本次%s阶段遇到上游模型响应超时、连接中断或服务临时不可用，系统已基于当时已收集到的信息继续生成答案；如果答案缺少证据、链路细节或关键结论，建议重试一次。", stageName)
	}
	return fmt.Sprintf("The %s stage hit an upstream model timeout, connection interruption, or temporary service issue. The system continued with the information already collected; if the answer is missing evidence, chain details, or key conclusions, retrying once is recommended.", stageName)
}

func localizedPipelineStageName(stage types.PipelineStage, lang string) string {
	if isChineseLang(lang) {
		switch stage {
		case types.StageAnalyze:
			return "分析"
		case types.StageExplore:
			return "探索"
		case types.StageExtract:
			return "提炼"
		case types.StageFinalize:
			return "成文"
		default:
			return "处理"
		}
	}
	switch stage {
	case types.StageAnalyze:
		return "analysis"
	case types.StageExplore:
		return "exploration"
	case types.StageExtract:
		return "extraction"
	case types.StageFinalize:
		return "final-answer"
	default:
		return "processing"
	}
}

func isLikelyTransientModelDispatchFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, llm.ErrStreamFirstByteTimeout) ||
		errors.Is(err, llm.ErrStreamStalled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	probe := strings.ToLower(err.Error())
	if strings.Contains(probe, "no usable sse data") ||
		strings.Contains(probe, "stream first-byte timeout") ||
		strings.Contains(probe, "first-byte timeout") ||
		strings.Contains(probe, "stream stalled") ||
		strings.Contains(probe, "http request before response headers") ||
		strings.Contains(probe, "connection reset") ||
		strings.Contains(probe, "connection refused") ||
		strings.Contains(probe, "deadline exceeded") ||
		strings.Contains(probe, "i/o timeout") ||
		strings.Contains(probe, "rate limit") ||
		strings.Contains(probe, " 429") ||
		strings.Contains(probe, "status 429") ||
		strings.Contains(probe, "server 5") ||
		strings.Contains(probe, "status 5") {
		return true
	}
	if strings.Contains(probe, "all retries exhausted by adapter") &&
		(strings.Contains(probe, "timeout") ||
			strings.Contains(probe, "context canceled") ||
			strings.Contains(probe, "context cancelled") ||
			strings.Contains(probe, "upstream llm")) {
		return true
	}
	return false
}

// runReadSchedulerLoop walks the read-mode AnalysisIR.TaskGraph with
// criterion-aware scheduling. Each round:
//
//  1. Build a criterion.Env from current BusContext state.
//  2. Check stopcond.ShouldStop; if true, forceCloseExploreWindow
//     and jump directly to finalize.
//  3. readyExplorerWindow returns nodes whose hard deps are done
//     AND whose EntryConditions all pass. Dispatch them as one
//     explore window. After the explore dispatch, evaluate each
//     window node's SuccessCriteria: successful ones are marked
//     done; failed ones are requeued.
//  4. A failed validate node's SuccessCriteria triggers
//     requeueValidationTargets — only the specific upstream
//     evidence nodes named by EdgeValidationFeedback get requeued,
//     not the whole window.
//  5. Finalize dispatch + contract check on the same contract-
//     checker retry semantics as before.
func (o *Orchestrator) runReadSchedulerLoop(stepBudget int) int {
	ir := o.busCtx.AnalysisIR
	if ir == nil || len(ir.TaskGraph.Nodes) == 0 {
		logging.Error("[orchestrator] task: no AnalysisIR.TaskGraph — analyzer failed to produce a valid IR")
		o.busCtx.Mutable.SetResult("")
		o.busCtx.TaskState.LastError = "analyzer failed to produce TaskGraph"
		return 0
	}

	// Per-task state reset so a multi-task run does not drag signals
	// across the task boundary.
	o.busCtx.Signals = types.ExecutionSignals{}
	o.busCtx.TaskState.Missing = types.MissingFacts

	// Cross-task reset of the Turn A/B handoff surface. Multi-task runs (REPL turns, batched analysis, task
	// list with >1 entry) otherwise drag stale state from task N
	// into task N+1: the previous task's TurnAArtifacts would still
	// be visible to this task's extractor, the previous task's
	// answer-symbol slate would still be drained into this task's
	// StageOutput, and the previous task's hypothesis verdicts would
	// still populate the finalizer prompt. Each Reset is a no-op
	// when the buffer is already empty, so it is safe to call
	// unconditionally at the top of every per-task dispatch.
	if o.busCtx.Mutable != nil {
		o.busCtx.Mutable.ResetTurnAArtifacts()
		o.busCtx.Mutable.ResetEmittedAnswerSymbols()
		o.busCtx.Mutable.ResetEmittedHypothesisVerdicts()
		o.busCtx.Mutable.ResetEmittedEvidence()
		// AnswerDocument is the finalizer's structured output buffer;
		// reset it alongside the extractor buffers so a multi-task run
		// cannot drag a stale document from task N into task N+1.
		o.busCtx.Mutable.ResetAnswerDocumentV2()
		// CGEC: per-task reset of the EvidenceClosure (PendingReads,
		// CitedRefs, Fingerprints, Repairs queue). Mirrors the other
		// per-task resets above; without this a stall fingerprint
		// from task N would carry into task N+1 and trigger a false
		// hard-stall on the very first round.
		o.busCtx.Mutable.ResetEvidenceClosure()
		// W2.6 (2026-05-05): per-task reset of the cross-scope
		// repair-attempt history. A fresh task starts with zero
		// attempts on every (kind, fp); without this a stale
		// "exhausted" verdict from task N would short-circuit task
		// N+1's first retry round.
		o.busCtx.Mutable.ResetRepairAttempts()
	}
	// AnswerSymbolCompleteness is a BusContext field, not a
	// MutableState field — reset it here too so the applyStageOutput
	// "last non-empty writer wins" merge rule does not accidentally
	// keep the previous task's claim alive when the current task's
	// extractor emits CompletenessUnknown.
	o.busCtx.AnswerSymbolCompleteness = types.CompletenessUnknown

	state := newGraphState(ir.TaskGraph)
	resolveSurface := termSurfaceLookup(ir)

	// Install the ExploreBudget derived from the analyzer's
	// NodeBudgetHints. Explorer's ReAct loop reads this through
	// ctx.Mutable.ExploreBudget() to throttle per-tool calls.
	hints := ir.EvidencePlan.NodeBudgetHints
	o.busCtx.Mutable.SetExploreBudget(&types.ExploreBudget{
		PerToolCap:  hints.PerToolCap,
		PerToolUsed: map[string]int{},
		OverallCap:  hints.OverallCap,
	})

	var pendingViolation string
	var pendingStageRetry string
	var pendingValidationTargets []string
	lowGroundingWarned := false

	if b := ir.EvidencePlan.Budget.MaxReactIters; b > 0 && b < stepBudget {
		stepBudget = b
	}

	// Adaptive budget scaling for multi-topic questions. When the
	// analyzer detected >1 SubTopics, the pipeline needs more steps
	// to investigate each sub-topic thoroughly.
	if nSub := len(ir.RequestModel.SubTopics); nSub > 1 {
		agentCfg := o.settings.Agent
		extraSteps := nSub * agentCfg.SubTopicPipelineStepsExtra
		adjusted := stepBudget + extraSteps
		ceil := o.settings.MaxStepsCeil
		if ceil <= 0 {
			ceil = 100
		}
		if adjusted > ceil {
			adjusted = ceil
		}
		if adjusted > stepBudget {
			logging.Info("[orchestrator] multi-topic scaling: %d sub-topics, step budget %d → %d",
				nSub, stepBudget, adjusted)
			stepBudget = adjusted
		}
	}

	stepsUsed := 0
	var lastFinalize *agent.StageOutput
	var firstFinalizeDraft string
	var firstFinalizeConcerns []types.Violation
	firstFinalizeRejectedForRewrite := false
	preFinalizeExtractCompleted := false
	var runtimeDispatchAdvisories []string

	// lastFallbackFinalizerOnly latches when the previous loop
	// iteration picked FallbackFinalizerOnly as the fallback target —
	// upstream extractor / explorer state was preserved verbatim, so
	// the next iteration's pre-finalize extract dispatch is wasted
	// LLM work (the answer-symbol slate is already correct;
	// re-running extract just re-emits the same items + verdicts on
	// the same evidence). B3-F3 (post_shape_retirement_consolidated_
	// audit.md §8 Batch B3, 2026-05-04) skips extract on this path.
	// Reset to false on any non-FinalizerOnly fallback so retries
	// that DO mutate upstream state get a fresh extract dispatch.
	lastFallbackFinalizerOnly := false

	// R2.2 (post_shape_residual_audit.md, 2026-05-04): per-Run
	// counter for the finalize-local priority downgrade. Each time
	// FallbackTargetForViolationsWithBudget chooses to downgrade a
	// deeper primary locus to FinalizerOnly, this counter increments;
	// when it reaches FinalizerLocalRetryBudget(), downgrade stops
	// and the deeper primary-locus pick activates.
	finalizerLocalRetriesUsed := 0

	// forceFinalizeTriggered latches once stopcond.ShouldStop has fired
	// and forceCloseExploreWindow has marked every non-finalize node
	// as done. The flag serves two distinct purposes:
	//
	//  1. Prevent hot-loop. criterion.Env is pure-read over BusContext
	//     and stopcond is deterministic over that env, so every later
	//     iteration would re-fire the same stop condition trivially.
	//     Without the latch, the stop path would loop forever because
	//     forceCloseExploreWindow only mutates graph state, not
	//     BusContext, leaving the predicate input unchanged.
	//
	//  2. Protect retry-after-finalize-failure semantics. Once the
	//     user/IR has declared "explore is over, go to finalize", a
	//     subsequent contract violation should be able to requeue
	//     explore nodes and re-investigate — but WITHOUT the
	//     force-close re-firing and closing those retries immediately.
	//     The latch stays set for the remainder of runTaskGraph so
	//     retries use the normal explore→extract→finalize flow.
	//
	// Design intent match: the `runTaskGraph` doc comment says "if
	// stop fires, forceCloseExploreWindow and jump directly to
	// finalize." The original code used `continue` after force-close
	// which re-entered the loop head — that's NOT a direct jump, it's
	// a loop restart. The fixed code falls through in the same
	// iteration: readyExplorerWindow returns empty (all force-closed),
	// firstFinalizeReadyMerged returns the finalize node, and the
	// existing fin-branch dispatches extract + finalize immediately.
	// This matches the doc's "jump directly" wording and saves one
	// loop tick of no-op work.
	forceFinalizeTriggered := false

	// Defer the "investigation done, preparing the answer" notice
	// until the scheduler has emitted every explore-family terminal
	// row. Accepted emit_investigation_complete can arrive while the
	// current window is still validate/evidence; the following loop
	// may auto-complete a reconcile node. Emitting the notice at the
	// acceptance point makes the scrollback read 2/4 → 3/4 → 2/4.
	investigationReadyNoticePending := false

	// Shape-guard state for the scheduler's pure-read predicates.
	// `lastStopShape` holds the envShape from the iteration where
	// `stopcond.ShouldStop` was last evaluated — if the current shape
	// equals it, re-evaluating would produce the same verdict and the
	// call is skipped. `lastSCFailShape` tracks per-validate-node the
	// envShape of the most recent SuccessCriteria failure; when a
	// validate node fails again with an identical shape, re-investigation
	// would not advance evidence (classic scenario: hypothesis about
	// foreign code that the repo provably does not contain) so the
	// scheduler escapes by auto-injecting HypInconclusive for the
	// stuck hypotheses and marks the validate node done. Shape-guard
	// is structural defence: it detects and breaks hot-loops whose
	// predicate input is pure-read over BusContext — the session-20
	// architectural fix that replaced the session-19 instance-specific
	// latch approach.
	var lastStopShape *envShape
	lastSCFailShape := make(map[string]envShape)
	// lastSCFailHypProgress is the session-22 companion to
	// lastSCFailShape: it tracks hypothesis-scope progress per
	// validate node so the scheduler also detects "global env
	// advanced but no unknown hypothesis inched closer to decidable"
	// stalls. Wired with OR semantics: either fingerprint matching
	// prev triggers the inconclusive-injection escape. See
	// scheduler.go::hypProgress for the rationale and field docs.
	lastSCFailHypProgress := make(map[string]hypProgress)

	buildEnv := func(draft string, draftCitations int) criterion.Env {
		env := criterion.Env{
			IR:             ir,
			Evidence:       o.busCtx.EvidenceItems,
			AnswerSymbols:  o.busCtx.AnswerSymbols,
			AnswerChains:   o.busCtx.AnswerChains,
			AggregateFacts: o.busCtx.Mutable.StableInvestigationAggregateFacts(),
			ToolResults:    o.busCtx.ToolResults,
			PrescanBlob:    o.busCtx.Mutable.PrescanSummaryBlob(),
			Signals:        o.busCtx.Signals,
			DraftAnswer:    draft,
			DraftCitations: draftCitations,
			ReactItersUsed: stepsUsed,
		}
		// Write-mode fields: populated for every Run. In read-only
		// pipelines these remain nil / zero-valued because the
		// write stages never ran; the corresponding evaluators
		// (CritPlanReady / PatchApplies / TestsPass / NoRegression)
		// short-circuit to Satisfied=true when the slot is empty.
		if o.busCtx.Mutable != nil {
			env.ChangePlan = o.busCtx.Mutable.ChangePlan()
			env.ChangeReport = o.busCtx.Mutable.ChangeReport()
			env.BaselineReport = o.busCtx.Mutable.BaselineReport()
			// WriteClosure is lazily-initialized; always non-nil for
			// a valid Mutable. Evaluators still check for zero-value
			// state (empty AppliedSet / empty VerifyResults) because
			// a closure in read-mode is alive but unused.
			env.WriteClosure = o.busCtx.Mutable.WriteClosure()
			// Triaged-artifact fields for CritExternalArtifactDecoded.
			// Nil when the user did not attach a runtime log / perf
			// trace at Run entry (the typical read-mode case);
			// evalExternalArtifactDecoded short-circuits to
			// Satisfied=true on nil bundles so existing eval cases
			// without --log / --htrace stay byte-identical.
			env.LogTriage = o.busCtx.Mutable.LogTriage()
			env.PerfTrace = o.busCtx.Mutable.PerfTrace()
			env.InvestigationComplete = o.busCtx.Mutable.IsInvestigationComplete()
			if ta := o.busCtx.Mutable.TurnAArtifacts(); ta != nil {
				env.ObservationOnlyCompletion = ta.RuntimeObservationOnlyCompletion
				if len(o.busCtx.MCPResponses) == 0 {
					env.MCPResponses = append([]types.MCPResponse(nil), ta.MCPResponses...)
				}
			}
		}
		if len(env.MCPResponses) == 0 && len(o.busCtx.MCPResponses) > 0 {
			env.MCPResponses = append([]types.MCPResponse(nil), o.busCtx.MCPResponses...)
		}
		return env
	}

	for stepsUsed < stepBudget && !state.allDone() {
		// Phase 2 cancel checkpoint at the top of every window-merge
		// iteration. Pre-this-fix the only cancel-poll was inside
		// BaseAgent.Execute (one per ReAct iter); a Cancel arriving
		// between window dispatches sat un-noticed until the next
		// agent dispatched and hit its own checkpoint.
		if cerr := o.checkCanceled("scheduler", stepsUsed); cerr != nil {
			o.busCtx.TaskState.LastError = cerr.Error()
			return stepsUsed
		}
		stopLocal := o.startSchedulerLocalWork(types.StageExplore, "build_env")
		env := buildEnv("", 0)
		if o.finishSchedulerLocalWork(stopLocal, "build_env", stepsUsed) {
			return stepsUsed
		}

		if !forceFinalizeTriggered {
			// Shape-gate: stopcond.ShouldStop is a pure function over
			// criterion.Env; identical envShape → identical verdict.
			// Skip re-evaluation when the Env cursor has not advanced
			// since the last check. The forceFinalizeTriggered latch
			// above still carries the one-shot "stop was terminal"
			// business semantic (protects post-finalize retry from
			// re-force-closing newly requeued explore nodes); the
			// shape-gate is belt-and-suspenders protection against
			// the same-shape hot-loop pattern at structural level, so
			// any FUTURE pure-read predicate added to this tight loop
			// automatically inherits the same protection without
			// requiring a per-predicate latch.
			currentStopShape := computeEnvShape(o.busCtx, env)
			if lastStopShape == nil || !lastStopShape.equals(currentStopShape) {
				stopLocal = o.startSchedulerLocalWork(types.StageExplore, "stop_condition")
				if stop, reason := stopcond.ShouldStop(ir.EvidencePlan, env); stop {
					logging.Info("[orchestrator] stop condition fired: %s", reason)
					state.forceCloseExploreWindow()
					forceFinalizeTriggered = true
					// No `continue` here — fall through in the SAME
					// iteration so readyExplorerWindow (below) sees the
					// just-closed nodes and firstFinalizeReadyMerged
					// returns the ready finalize node immediately. This
					// matches the "jump directly to finalize" wording in
					// runTaskGraph's doc comment.
				}
				if o.finishSchedulerLocalWork(stopLocal, "stop_condition", stepsUsed) {
					return stepsUsed
				}
				lastStopShape = &currentStopShape
			}
		}

		stopLocal = o.startSchedulerLocalWork(types.StageExplore, "ready_window")
		window, blocked, err := state.readyExplorerWindowContext(o.CancelContext(), env)
		if o.finishSchedulerLocalWork(stopLocal, "ready_window", stepsUsed) || err != nil {
			if err != nil {
				if cerr := o.checkCanceled("scheduler.ready_window", stepsUsed); cerr != nil {
					o.busCtx.TaskState.LastError = cerr.Error()
				} else {
					o.busCtx.TaskState.LastError = err.Error()
				}
			}
			return stepsUsed
		}
		fin := state.firstFinalizeReadyMerged()

		if len(window) > 0 {
			stopLocal = o.startSchedulerLocalWork(types.StageExplore, "reconcile_autocomplete")
			window = o.autoCompleteReadyReconcileNodes(state, window, env)
			if o.finishSchedulerLocalWork(stopLocal, "reconcile_autocomplete", stepsUsed) {
				return stepsUsed
			}
			if len(window) == 0 {
				o.applyWindowHint("")
				continue
			}
			if o.shouldAutoCompleteExploreWindowFromAcceptedClosure(pendingValidationTargets, pendingViolation, pendingStageRetry) {
				if o.busCtx != nil {
					o.busCtx.Signals.HasEnoughFacts = true
				}
				env.Signals.HasEnoughFacts = true
				o.recordAcceptedClosureValidationBoundaries(window, env, "pre_dispatch_auto_complete")
				for _, n := range window {
					state.markRunning(n.ID)
					o.emitNodeStart(n.ID)
					state.markDone(n.ID)
					o.emitNodeEnd(n.ID, true, "skipped: accepted investigation closure")
				}
				logging.Info("[orchestrator] auto-completed %d explore node(s) from accepted investigation closure", len(window))
				continue
			}
			// E' (2026-05-17 architecture §1.5/§1.6) — DAG node-level
			// dispatch. When the analyzer's expandEvidenceNodes
			// produced N independent evidence sibling nodes
			// (`{prefix}_t{i}` per SubTopic), give each sibling its
			// own focused explorer dispatch instead of coalescing them
			// into one merged explorer LLM call. The outer scheduler
			// loop re-fetches the remaining siblings via
			// readyExplorerWindowContext on the next iteration; each
			// sibling completes through the existing per-node
			// markRunning → markDone state machine and emits its own
			// EventTaskNodeStart/End so the /dag UI shows N rows
			// transitioning independently. See
			// dag_node_dispatch.go for the typed-signal predicate and
			// split helper. When pipeline_max_parallelism permits, the
			// split windows below run concurrently; when the cap is 1,
			// the same helper degrades to the original serial-per-node
			// flow.
			var parallelWindows [][]*types.TaskNode
			parallelism := 1
			dispatchWindows := exploreWindowDispatchGroups(o.busCtx, window)
			if len(dispatchWindows) > 1 {
				parallelism = effectiveParallelism(o.orchestratorMaxParallelism(), len(dispatchWindows))
				if parallelism > 1 {
					parallelWindows = dispatchWindows
				} else {
					window = dispatchWindows[0]
				}
			} else if len(dispatchWindows) == 1 {
				window = dispatchWindows[0]
			}
			// CGEC D2: drain pending RepairDirectives from the
			// closure so each fires exactly once. ConsumeRepairs is
			// atomic — it returns the queue and clears the field in
			// one step.
			var pendingRepairs []types.RepairDirective
			if o.busCtx.Mutable != nil {
				pendingRepairs = o.busCtx.Mutable.EvidenceClosure().ConsumeRepairs()
			}
			stopLocal = o.startSchedulerLocalWork(types.StageExplore, "window_hint")
			var hint string
			var parallelHints []string
			if len(parallelWindows) > 0 {
				parallelHints = make([]string, len(parallelWindows))
				for i, w := range parallelWindows {
					var bw []nodeBlock
					var vt []string
					var pv, ps string
					var repairs []types.RepairDirective
					if i == 0 {
						bw = blocked
						vt = pendingValidationTargets
						pv = pendingViolation
						ps = pendingStageRetry
						repairs = pendingRepairs
					}
					parallelHints[i], err = renderWindowHintContext(o.CancelContext(), w, bw, vt, resolveSurface, pv, ps, repairs)
					if err != nil {
						break
					}
				}
			} else {
				hint, err = renderWindowHintContext(o.CancelContext(), window, blocked, pendingValidationTargets, resolveSurface, pendingViolation, pendingStageRetry, pendingRepairs)
			}
			if o.finishSchedulerLocalWork(stopLocal, "window_hint", stepsUsed) || err != nil {
				if err != nil {
					o.busCtx.TaskState.LastError = err.Error()
				}
				return stepsUsed
			}
			if checkpointHint := state.consumeTransientRetryHint(); checkpointHint != "" {
				if len(parallelWindows) > 0 {
					for i := range parallelHints {
						parallelHints[i] = prependRetryHint(checkpointHint, parallelHints[i])
					}
				} else {
					hint = prependRetryHint(checkpointHint, hint)
				}
			}
			pendingViolation = ""
			pendingStageRetry = ""
			pendingValidationTargets = nil
			if len(parallelWindows) > 0 {
				o.applyWindowHint("")
			} else {
				o.applyWindowHint(hint)
			}
			for _, n := range window {
				state.markRunning(n.ID)
				o.emitNodeStart(n.ID)
			}

			o.busCtx.PipelineStage = types.StageExplore
			o.busCtx.TaskState.Stage = types.StageExplore
			// Reset per-tool usage counters and investigation-complete
			// flag so a retry window (validation_feedback requeue or
			// contract backtrack) starts fresh.
			o.busCtx.Mutable.ResetInvestigationComplete()
			if eb := o.busCtx.Mutable.ExploreBudget(); eb != nil {
				o.busCtx.Mutable.SetExploreBudget(&types.ExploreBudget{
					PerToolCap:  eb.PerToolCap,
					PerToolUsed: map[string]int{},
					OverallCap:  eb.OverallCap,
				})
			}
			// CGEC A3: force-read any PendingReads that accumulated
			// during the previous finalize pass (A1 mirrors grounder
			// RepairReadFile into PendingReads). Run this BEFORE
			// dispatch so the explorer LLM sees the [forced_read]
			// ToolResults in its ReAct loop and can emit_evidence
			// over them in the SAME dispatch, rather than waiting
			// for the next retry round. Harmless no-op when
			// PendingReads is empty.
			preseededRequiredFiles := o.seedRequiredFileHintForcedReadsBeforeExplore()
			if preseededRequiredFiles > 0 {
				logging.Info("[CGEC] pre-dispatch seeded %d required-file forced-read(s)", preseededRequiredFiles)
			}
			stopLocal = o.startSchedulerLocalWork(types.StageExplore, "forced_reads_pre_dispatch")
			read := o.runForcedReads()
			if o.finishSchedulerLocalWork(stopLocal, "forced_reads_pre_dispatch", stepsUsed) {
				return stepsUsed
			}
			if read > 0 {
				logging.Info("[CGEC] E2 pre-dispatch forced-read %d file(s) before explore retry", read)
			}
			var out *agent.StageOutput
			var dispatchErr error
			dispatchStepCount := 1
			if len(parallelWindows) > 0 {
				stageStart := emitParallelExploreStageStart(o)
				out, dispatchErr = o.dispatchExploreWindowsParallel(parallelWindows, parallelHints, parallelism)
				stageErr := ""
				if dispatchErr != nil {
					stageErr = dispatchErr.Error()
				} else if out != nil && out.Error != "" {
					stageErr = out.Error
				}
				o.emit(render.Event{
					Kind:      render.EventStageEnd,
					Timestamp: time.Now(),
					Stage:     types.StageExplore,
					Agent:     types.AgentExplorer,
					Error:     stageErr,
				})
				logging.Debug("[orchestrator] parallel explore stage elapsed=%s windows=%d parallelism=%d",
					time.Since(stageStart).Round(time.Millisecond), len(parallelWindows), parallelism)
				dispatchStepCount = len(parallelWindows)
			} else {
				prevDispatchKey := o.busCtx.ExploreDispatchKey
				prevDispatchKind := o.busCtx.ExploreDispatchKind
				o.busCtx.ExploreDispatchKey = exploreDispatchKeyForWindow(window)
				o.busCtx.ExploreDispatchKind = exploreDispatchKindForWindow(window)
				out, dispatchErr = o.dispatchStage(types.StageExplore)
				o.busCtx.ExploreDispatchKey = prevDispatchKey
				o.busCtx.ExploreDispatchKind = prevDispatchKind
			}
			if dispatchErr != nil {
				logging.Error("[orchestrator] DAG explore window failed: %v", dispatchErr)
				if o.retryReadStageDispatchError(state, types.StageExplore, window, nil, dispatchErr) {
					continue
				}
				runtimeDispatchAdvisories = appendUniqueRuntimeAdvisory(
					runtimeDispatchAdvisories,
					runtimeDispatchAdvisoryForStage(types.StageExplore, dispatchErr, o.busCtx.Language),
				)
				stepsUsed += dispatchStepCount
				for _, n := range window {
					state.markFailed(n.ID)
					o.emitNodeEnd(n.ID, false, dispatchErr.Error())
				}
			} else {
				stepsUsed += dispatchStepCount
				if o.requeueExploreWindowForFactRetry(state, window, out) {
					pendingStageRetry = out.RetryHint
					continue
				}
				// Post-dispatch criterion evaluation. Separate
				// validate-node failure from non-validate failure:
				// validate failures trigger fine-grained
				// requeueValidationTargets, others just mark the
				// node requeued.
				icComplete := o.busCtx.Mutable != nil && o.busCtx.Mutable.IsInvestigationComplete()
				icPolicy := o.effectiveInvestigationCompletePolicy()

				// When emit_investigation_complete has passed the tool
				// layer's pre-complete gates, the model has made an
				// explicit typed closure decision. In the default "soft"
				// policy, trust that accepted closure for DAG-level
				// explore criteria and let extractor/finalizer contracts
				// handle answer-surface quality. The "strict" policy is
				// the opt-in mode for deployments that still want template
				// SuccessCriteria to reopen exploration after completion.
				if icComplete && (icPolicy == types.ICPolicySoft || icPolicy == types.ICPolicyOverride) {
					o.busCtx.Signals.HasEnoughFacts = true
					stopLocal = o.startSchedulerLocalWork(types.StageExplore, "accepted_closure_validation_boundary")
					envAfterClosure := buildEnv("", 0)
					o.recordAcceptedClosureValidationBoundaries(window, envAfterClosure, "post_dispatch_accept")
					if o.finishSchedulerLocalWork(stopLocal, "accepted_closure_validation_boundary", stepsUsed) {
						return stepsUsed
					}
					for _, n := range window {
						state.markDone(n.ID)
						o.emitNodeEnd(n.ID, true, "")
					}
					investigationReadyNoticePending = true
					stopLocal = o.startSchedulerLocalWork(types.StageExplore, "auto_verdicts")
					o.runAutoVerdicts()
					if o.finishSchedulerLocalWork(stopLocal, "auto_verdicts", stepsUsed) {
						return stepsUsed
					}
					stopLocal = o.startSchedulerLocalWork(types.StageExplore, "drain_hypothesis_verdicts")
					o.drainHypothesisVerdicts()
					if o.finishSchedulerLocalWork(stopLocal, "drain_hypothesis_verdicts", stepsUsed) {
						return stepsUsed
					}
					continue
				}

				stopLocal = o.startSchedulerLocalWork(types.StageExplore, "build_env_after_explore")
				envAfter := buildEnv("", 0)
				if o.finishSchedulerLocalWork(stopLocal, "build_env_after_explore", stepsUsed) {
					return stepsUsed
				}
				var valFailed *types.TaskNode
				stopLocal = o.startSchedulerLocalWork(types.StageExplore, "success_criteria")
				var successCriteriaErr error
				for _, n := range window {
					ok, failed, err := state.markSuccessCriteriaFailedContext(o.CancelContext(), n, envAfter)
					if err != nil {
						successCriteriaErr = err
						break
					}
					if ok {
						state.markDone(n.ID)
						o.emitNodeEnd(n.ID, true, "")
						continue
					}
					logging.Info("[orchestrator] node %s success criteria failed: %+v", n.ID, failed)
					// Surface a soft user-facing retry cue. The full
					// criterion kind / expr / detail breakdown is in
					// the INFO log above — the user-facing event stays
					// jargon-free per the user_messages.go contract.
					o.emit(render.Event{
						Kind:       render.EventOrchestratorNotice,
						Timestamp:  time.Now(),
						Agent:      "orchestrator",
						NoticeKind: render.NoticeRetry,
						Reasoning:  softRetryHintMessage(o.busCtx.Language),
					})
					// No EventTaskNodeEnd on requeue — the renderer treats
					// the node as still "running" until the next
					// EventTaskNodeStart flips it back in.
					if n.Type == types.NodeValidate {
						valFailed = n
					} else {
						state.requeue(n.ID)
					}
				}
				if o.finishSchedulerLocalWork(stopLocal, "success_criteria", stepsUsed) || successCriteriaErr != nil {
					if successCriteriaErr != nil {
						o.busCtx.TaskState.LastError = successCriteriaErr.Error()
					}
					return stepsUsed
				}
				if valFailed != nil {
					// Shape-stuck detection — OR semantics across two
					// complementary fingerprints:
					//
					//   envShape    — full BusContext cursor (8 dims).
					//                 Catches "nothing advanced at all"
					//                 stalls (original session-20 case:
					//                 LLM kept emitting the same tool
					//                 result so every counter pinned).
					//
					//   hypProgress — per-validate-node hypothesis-scope
					//                 cursor (UnknownCount + sum of
					//                 satisfied-RequiredEvidence hits
					//                 over unknowns). Catches "global
					//                 env advanced but no hypothesis
					//                 inched closer" stalls (session-22
					//                 case: traceback paths outside repo,
					//                 explorer emits irrelevant evidence
					//                 about codrax's own infrastructure
					//                 so EvidenceCount/ToolResultCount
					//                 grow but no unknown hypothesis
					//                 gets nearer a decision).
					//
					// Either fingerprint matching its previous failure
					// → stuck → inject HypInconclusive so finalize can
					// ship with an honest caveat. Without this escape,
					// the validate loop requeues explore, the explorer
					// re-reads or fishes for more evidence, and the
					// loop runs until step budget drains.
					stopLocal = o.startSchedulerLocalWork(types.StageExplore, "validate_stall_fingerprint")
					currentShape := computeEnvShape(o.busCtx, envAfter)
					currentHyp, err := computeHypProgressContext(o.CancelContext(), envAfter)
					if o.finishSchedulerLocalWork(stopLocal, "validate_stall_fingerprint", stepsUsed) || err != nil {
						if err != nil {
							o.busCtx.TaskState.LastError = err.Error()
						}
						return stepsUsed
					}
					prevShape, seenShape := lastSCFailShape[valFailed.ID]
					prevHyp, seenHyp := lastSCFailHypProgress[valFailed.ID]
					shapeStuck := seenShape && prevShape.equals(currentShape)
					// hypStuck guard: only fire when there is an unknown
					// hypothesis to auto-inconclusive. Some validate
					// templates carry non-hypothesis SC (e.g.
					// ScenarioArchitectureExplain uses
					// CritAnswerSetBounded). For those, a trivially-
					// pinned {UnknownCount:0, SatisfiedReqSum:0}
					// fingerprint would falsely match across two SC
					// failures and mask the real SC failure by skipping
					// the validate node to finalize. The envShape check
					// already handles the "nothing at all advanced"
					// case for those paths; hypStuck is scoped to the
					// "global env advanced but no unknown hypothesis
					// progressed" pathology.
					hypStuck := seenHyp && prevHyp.equals(currentHyp) && currentHyp.UnknownCount > 0
					if shapeStuck || hypStuck {
						injected := o.injectInconclusiveForStuckHypotheses(valFailed.ID)
						state.markDone(valFailed.ID)
						o.emitNodeEnd(valFailed.ID, true, "")
						// Operator breadcrumb — which fingerprint caught
						// the stall (envShape / hypProgress / both) plus
						// the inconclusive-injection count. User sees the
						// softConvergenceStallMessage above instead.
						trigger := "envShape"
						switch {
						case shapeStuck && hypStuck:
							trigger = "envShape+hypProgress"
						case hypStuck:
							trigger = "hypProgress"
						}
						logging.Info("[scheduler/stuck] validate %s: trigger=%s injected=%d inconclusive verdict(s)",
							valFailed.ID, trigger, injected)
						o.emit(render.Event{
							Kind:       render.EventOrchestratorNotice,
							Timestamp:  time.Now(),
							Agent:      "orchestrator",
							NoticeKind: render.NoticeConvergenceStall,
							Reasoning:  softConvergenceStallMessage(o.busCtx.Language),
						})
					} else {
						// Neither fingerprint matched — run the normal
						// requeue path.
						targets := state.requeueValidationTargets(valFailed.ID)
						if len(targets) == 0 {
							// No upstream evidence edges found — fall
							// back to requeueing the validate node only.
							state.requeue(valFailed.ID)
						} else {
							pendingValidationTargets = targets
							state.recordRetry()
						}
						lastSCFailShape[valFailed.ID] = currentShape
						lastSCFailHypProgress[valFailed.ID] = currentHyp
					}
				}

				// Lightweight auto-verdict after each explore window:
				// evaluate criterion-based hypothesis verdicts without
				// an LLM call. The full extract dispatch (with LLM)
				// runs once just before finalize.
				stopLocal = o.startSchedulerLocalWork(types.StageExplore, "auto_verdicts")
				o.runAutoVerdicts()
				if o.finishSchedulerLocalWork(stopLocal, "auto_verdicts", stepsUsed) {
					return stepsUsed
				}

				// CGEC E2 + I4: after each explore round, check for
				// pending forced reads (LLM skipped framework-queued
				// files) and convergence stall (3 identical
				// fingerprints → force-finalize). Both run silently
				// when state is fresh; runForcedReads may inject
				// synthesized read_file results into the dispatch
				// buffer so the next round sees them in extractFileCoverage.
				stopLocal = o.startSchedulerLocalWork(types.StageExplore, "forced_reads_post_dispatch")
				_ = o.runForcedReads()
				if o.finishSchedulerLocalWork(stopLocal, "forced_reads_post_dispatch", stepsUsed) {
					return stepsUsed
				}
				stopLocal = o.startSchedulerLocalWork(types.StageExplore, "stall_detection")
				stalled := o.detectStallAndAct()
				if o.finishSchedulerLocalWork(stopLocal, "stall_detection", stepsUsed) {
					return stepsUsed
				}
				if stalled {
					// Hard stall — break out of the explore loop and
					// let the finalize path run with whatever evidence
					// was gathered.
					state.forceCloseExploreWindow()
					continue
				}
				stopLocal = o.startSchedulerLocalWork(types.StageExplore, "drain_hypothesis_verdicts")
				o.drainHypothesisVerdicts()
				if o.finishSchedulerLocalWork(stopLocal, "drain_hypothesis_verdicts", stepsUsed) {
					return stepsUsed
				}
			}
			continue
		}

		if fin == nil {
			// No ready window (or every node blocked) AND no ready
			// finalize. If blocked nodes exist we can make progress
			// only by waiting for a future env change; since env is
			// pure-read we would loop forever. Break to forced
			// finalize.
			if len(blocked) > 0 {
				logging.Warning("[orchestrator] %d node(s) blocked on entry conditions; forcing finalize", len(blocked))
			} else {
				logging.Warning("[orchestrator] DAG scheduler stalled; forcing finalize")
			}
			break
		}

		// Pre-extract Tier-1 floor gate (session 8, log
		// 1776446668535115555). emit_investigation_complete's Tier-1
		// floor only fires when the LLM calls that tool. An explorer
		// that exits via ShouldStop / idle-stop / soft-stop bypasses
		// the tool, so pure-recovery investigations still reach Turn
		// B. The orchestrator is the single choke point where all
		// exit paths converge, so the same floor runs here against
		// Mutable.EmittedEvidence() before we burn LLM calls on
		// extract + finalize.
		//
		// On fail-with-budget: requeue all non-finalize explore nodes
		// + finalize, inject the diagnostic as pendingViolation (the
		// existing contract-backtrack retry path), record a retry
		// tick, and continue the loop — next round builds a window
		// that includes the "need more read_file" hint.
		//
		// On fail-budget-exhausted: log a warning and fall through;
		// downstream contract check will still catch the problem and
		// fail-loud.
		stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "tier1_floor")
		msg, proceed, exhausted := o.checkTier1Floor(ir, state)
		if o.finishSchedulerLocalWork(stopLocal, "tier1_floor", stepsUsed) {
			return stepsUsed
		}
		if !proceed {
			if exhausted {
				logging.Warning("[orchestrator] pre-finalize Tier-1 floor failed but retry budget exhausted: %s", msg)
			} else {
				state.requeue(fin.ID)
				for _, n := range ir.TaskGraph.Nodes {
					if n.Type == types.NodeFinalize {
						continue
					}
					if state.status[n.ID] == nodeDone {
						state.requeue(n.ID)
					}
				}
				state.recordRetry()
				pendingViolation = msg
				investigationReadyNoticePending = false
				// Tier-1 floor violation is semantically "we do not yet
				// have enough evidence to build a trustworthy answer,
				// running one more pass". The full msg (with tool-name
				// / floor-threshold / intent-class jargon) is logged
				// via the pendingViolation pathway and the WARN above,
				// not leaked to the user-facing event.
				o.emit(render.Event{
					Kind:       render.EventOrchestratorNotice,
					Timestamp:  time.Now(),
					Agent:      "orchestrator",
					NoticeKind: render.NoticeRetry,
					Reasoning:  softRetryHintMessage(o.busCtx.Language),
				})
				continue
			}
		}
		o.warnLowGroundingIfNeeded(ir, &lowGroundingWarned)

		stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "structural_empty_check")
		out, retryMsg, handled := o.handleStructurallyEmptyInvestigation(state, fin.ID)
		if o.finishSchedulerLocalWork(stopLocal, "structural_empty_check", stepsUsed) {
			return stepsUsed
		}
		if handled {
			if out == nil {
				pendingViolation = retryMsg
				investigationReadyNoticePending = false
				continue
			}
			lastFinalize = out
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}
		if investigationReadyNoticePending {
			o.emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				NoticeKind: render.NoticeInvestigationReady,
				Reasoning:  softInvestigationReadyMessage(o.busCtx.Language),
			})
			investigationReadyNoticePending = false
		}

		// Full Turn B extract dispatch — runs once, just before
		// finalize, with complete accumulated evidence from all
		// explore windows. Answer-symbol selection + LLM hypothesis
		// verdicts happen here.
		//
		// B3-F3 (2026-05-04) skip path: when the previous iteration
		// fell back FinalizerOnly the upstream evidence + answer-
		// symbol slate are already correct (FinalizerOnly explicitly
		// preserved them). Re-running extract just re-emits the
		// same items + verdicts on the same evidence — wasted LLM
		// turns. Skip the re-dispatch on this path; the existing
		// AnswerSymbols / HypothesisVerdicts in BusContext stay
		// valid from the prior iteration. (The skip is one-shot:
		// we reset the latch immediately so any later non-Finalizer
		// fallback re-runs extract as before.)
		if lastFallbackFinalizerOnly && (preFinalizeExtractCompleted || o.hasReusableTurnBSlateForFinalize()) {
			logging.Info("[orchestrator] B3-F3 skip pre-finalize extract: prior FinalizerOnly fallback preserved Turn-B state (extract_completed=%t %s)",
				preFinalizeExtractCompleted, o.reusableTurnBSlateSummary())
			lastFallbackFinalizerOnly = false
		} else {
			o.busCtx.PipelineStage = types.StageExtract
			o.busCtx.TaskState.Stage = types.StageExtract
			for {
				if _, exErr := o.dispatchStage(types.StageExtract); exErr != nil {
					if o.completeExtractAfterTransientProgress(exErr) {
						stepsUsed++
						preFinalizeExtractCompleted = true
						break
					}
					if o.retryReadStandaloneDispatchError(state, types.StageExtract, exErr) {
						continue
					}
					stepsUsed++
					logging.Warning("[orchestrator] pre-finalize extract dispatch failed (continuing): %v", exErr)
					runtimeDispatchAdvisories = appendUniqueRuntimeAdvisory(
						runtimeDispatchAdvisories,
						runtimeDispatchAdvisoryForStage(types.StageExtract, exErr, o.busCtx.Language),
					)
					// Soft notice so the user sees WHY the next event is
					// "正在生成最终答案" instead of an extract success
					// row. Without this the scrollback jumps from
					// "✗/⟳ 未能提炼关键发现" straight to finalize with no
					// explanation of the transition.
					o.emit(render.Event{
						Kind:       render.EventOrchestratorNotice,
						Timestamp:  time.Now(),
						Agent:      "orchestrator",
						NoticeKind: render.NoticeProceedingWithoutExtract,
						Reasoning:  softProceedingWithoutExtractMessage(o.busCtx.Language),
					})
					break
				}
				stepsUsed++
				preFinalizeExtractCompleted = true
				stopLocal = o.startSchedulerLocalWork(types.StageExtract, "drain_hypothesis_verdicts")
				o.drainHypothesisVerdicts()
				if o.finishSchedulerLocalWork(stopLocal, "drain_hypothesis_verdicts", stepsUsed) {
					return stepsUsed
				}
				break
			}
			lastFallbackFinalizerOnly = false
		}

		// Phase 2.B Tier 2 ERM completeness hard gate runs
		// AFTER finalize composes the answer, not here pre-finalize.
		// See the post-finalize hook below (after runContractCheck +
		// ComputeRootCauseClosure) for the new wire-up. The previous
		// Phase 2 pre-finalize advisory was retired in favour of
		// post-finalize hard-gate enforcement so failures flow into
		// the existing violation→retry→caveat machinery.

		state.markRunning(fin.ID)
		o.emitNodeStart(fin.ID)
		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize
		// Bug 4 (trace 1776448040358685830): a prior retry round's
		// AnswerDocument lingers in Mutable across pipeline retries.
		// The finalizer's evaluator Observe short-circuits on the
		// stale doc ("emit_answer_document called") and stops the
		// ReAct loop WITHOUT giving the LLM a chance to correct after
		// a tool-level reject in the current dispatch. Reset the
		// buffer before every finalize dispatch so each round starts
		// from a clean slate. Safe for round 0 (doc was already nil),
		// correct for round 1+ (clears the stale doc from round N-1).
		if o.busCtx.Mutable != nil {
			o.busCtx.Mutable.ResetActiveAnswerDocumentV2ForFinalizeDispatch()
		}
		// Pre-dispatch cue: finalize runs one synchronous LLM call
		// without intermediate tool activity, so without this the task
		// row sits silent on "thinking" for the full composition
		// window. Users lose any signal that the answer is imminent.
		o.emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      "orchestrator",
			NoticeKind: render.NoticeFinalizing,
			Reasoning:  softFinalizingMessage(o.busCtx.Language),
		})
		out, err = o.dispatchStage(types.StageFinalize)
		if err != nil {
			if o.canUseFinalizerOutputAfterTransientProgress(out, err) {
				logging.Warning("[orchestrator] finalize transient dispatch error after usable answer document; preserving structured answer and continuing checks: %v", err)
				err = nil
			}
		}
		if err != nil {
			if fallback := o.finalizerNoVisibleOutputFallback(err); fallback != nil {
				logging.Warning("[orchestrator] finalize produced no visible output within request timeout; delivering degraded answer from existing Turn-B/evidence slate: %v", err)
				lastFinalize = fallback
				o.emit(render.Event{
					Kind:            render.EventLivePreviewClear,
					Timestamp:       time.Now(),
					Stage:           types.StageFinalize,
					PreviewRejected: false,
				})
				o.emit(render.Event{
					Kind:       render.EventOrchestratorNotice,
					Timestamp:  time.Now(),
					Agent:      "orchestrator",
					NoticeKind: render.NoticeFallbackFailLoud,
					Reasoning:  softFinalizerProseFallbackMessage(o.busCtx.Language),
				})
				state.markDone(fin.ID)
				o.emitNodeEnd(fin.ID, true, "")
				break
			}
			logging.Error("[orchestrator] DAG finalize failed: %v", err)
			// Live preview cleanup: dispatch error path treats the
			// just-streamed draft as rejected. The renderer flashes
			// "已重写" briefly then erases the area before the retry
			// (or terminal failure) text prints.
			o.emit(render.Event{
				Kind:            render.EventLivePreviewClear,
				Timestamp:       time.Now(),
				Stage:           types.StageFinalize,
				PreviewRejected: true,
			})
			if o.retryReadStageDispatchError(state, types.StageFinalize, nil, fin, err) {
				lastFallbackFinalizerOnly = true
				continue
			}
			stepsUsed++
			if lastFinalize != nil && strings.TrimSpace(lastFinalize.FinalAnswer) != "" {
				logging.Warning("[orchestrator] finalize dispatch failed after retry budget; delivering previous draft with transient-failure caveat")
				lastFinalize.FinalAnswer = appendFinalizerTransientFailureCaveat(lastFinalize.FinalAnswer, o.busCtx.Language)
				state.markDone(fin.ID)
				o.emitNodeEnd(fin.ID, true, "")
				break
			}
			if recovered := o.recoverRejectedFinalizerDraftAfterTransientFailure(ir.AnswerContract); recovered != nil {
				logging.Warning("[orchestrator] finalize dispatch failed after retry budget; delivering repaired rejected draft instead of re-running upstream stages")
				lastFinalize = recovered
				state.markDone(fin.ID)
				o.emitNodeEnd(fin.ID, true, "")
				break
			}
			state.markFailed(fin.ID)
			o.emitNodeEnd(fin.ID, false, err.Error())
			break
		}
		stepsUsed++
		lastFinalize = out
		if strings.TrimSpace(firstFinalizeDraft) == "" {
			firstFinalizeDraft = strings.TrimSpace(out.FinalAnswer)
		}
		if out.SkipAnswerChecks {
			logging.Warning("[orchestrator] finalizer returned degraded answer; skipping structured answer checks reason=%s",
				out.DegradeReason)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			o.emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				NoticeKind: render.NoticeFallbackFailLoud,
				Reasoning:  softFinalizerProseFallbackMessage(o.busCtx.Language),
			})
			o.emit(render.Event{
				Kind:            render.EventLivePreviewClear,
				Timestamp:       time.Now(),
				Stage:           types.StageFinalize,
				PreviewRejected: false,
			})
			logEvidenceUtilization(o, lastFinalize)
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}

		// Evaluate finalize node's SuccessCriteria alongside
		// the AnswerContract check. SuccessCriteria on finalize
		// nodes carry citation / symbol constraints the compiler
		// declared; failing them is treated like a contract
		// violation for backtrack purposes.
		//
		// Pre-2026-04-17 these failures only produced a log line
		// and the answer shipped regardless. They are now merged
		// into res.Violations so the retry-budget / requeue /
		// pendingViolation branch below treats them uniformly with
		// contract.Check failures.
		// DraftCitations counts grounded citation support, not just
		// the raw citation pool. The V2 pool is still authoritative
		// when present, but rendered file:line anchors that match the
		// collected evidence / answer-symbol set also count so a
		// harmless under-filled pool does not force a full finalizer
		// rewrite.
		stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "build_env_after_finalize")
		citationCount := finalizerCitationSupportCount(o.busCtx, out)
		envFin := buildEnv(out.FinalAnswer, citationCount)
		if o.finishSchedulerLocalWork(stopLocal, "build_env_after_finalize", stepsUsed) {
			return stepsUsed
		}
		stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "finalize_success_criteria")
		scOK, scFailed, err := state.markSuccessCriteriaFailedContext(o.CancelContext(), fin, envFin)
		if o.finishSchedulerLocalWork(stopLocal, "finalize_success_criteria", stepsUsed) || err != nil {
			if err != nil {
				o.busCtx.TaskState.LastError = err.Error()
			}
			return stepsUsed
		}

		// Contract check. runContractCheck consults
		// Mutable.AnswerDocument to decide IsAbsence and skips
		// MinCitations when the doc is a justified zero.
		stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "answer_contract_check")
		res := runContractCheck(out, ir.AnswerContract, o.busCtx.Mutable, o)
		if o.finishSchedulerLocalWork(stopLocal, "answer_contract_check", stepsUsed) {
			return stepsUsed
		}

		if !scOK {
			// Absence answers legitimately have no file:line to cite;
			// citation_count_ge SC failures on them are not real
			// retry triggers (the retry would produce the same 0).
			// Other SC failures still merge into res.Violations.
			absence := isJustifiedAbsenceAnswer(o.busCtx.Mutable)
			for _, f := range scFailed {
				if absence && string(f.Kind) == string(types.CritCitationCountGE) {
					logging.Info("[orchestrator] finalize success criteria failed: %s %s — %s (waived: justified absence answer)", f.Kind, f.Expr, f.Detail)
					continue
				}
				if string(f.Kind) == string(types.CritCitationCountGE) && isDriftBoundedCitationAnswer(o.busCtx, out) {
					logging.Info("[orchestrator] finalize success criteria failed: %s %s – %s (waived: drift-bounded root-cause surface carries the minimum grounded citation set for the current checkout)", f.Kind, f.Expr, f.Detail)
					continue
				}
				logging.Info("[orchestrator] finalize success criteria failed: %s %s — %s", f.Kind, f.Expr, f.Detail)
				v := contract.Violation{
					Kind:   contract.ViolSuccessCriterion,
					Detail: fmt.Sprintf("finalize success_criterion %s %s failed: %s", f.Kind, f.Expr, f.Detail),
				}
				if locus := successCriterionRepairLocusOverride(o.busCtx, f.Kind, f.Expr); locus != "" {
					v.RepairLocusOverride = locus
					v.Repair = "repair the final answer using already-collected citation-grade evidence"
				}
				res.Violations = append(res.Violations, v)
			}
			// res.Passed stays true when every SC failure was waived.
			if len(res.Violations) > 0 {
				res.Passed = false
			}
		}

		// W2.4 (2026-05-05): root-cause closure runs once per
		// contract result so downstream consumers (retry hint
		// builder, repair planner, materializer) see the SAME
		// IsDerived / RootKind classification. Without this central
		// pass the LLM gets a retry hint listing 4 violations when
		// only 1 root cause exists, then "fixes" one and the others
		// rotate (qfa-mr3 forensic — see
		// docs/design/iteration_inflation_remediation.md §3 #1).
		//
		// The closure is non-mutating w.r.t. the underlying validator
		// outputs — it only stamps IsDerived / RootKind for in-batch
		// suppression. Operator telemetry (ledger, closure stats)
		// continues to receive every violation; only the LLM-facing
		// surfaces honour the IsDerived flag.
		if len(res.Violations) > 0 {
			stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "violation_root_cause")
			res.Violations = ComputeRootCauseClosure(res.Violations)
			if o.finishSchedulerLocalWork(stopLocal, "violation_root_cause", stepsUsed) {
				return stepsUsed
			}
		}

		if scOK && len(res.Violations) > 0 && o.tryAutoRepairFinalizerAnswerDocument(out, res.Violations) {
			logging.Info("[orchestrator] finalizer auto-repair applied for deterministic answer-document metadata/inline-code issues; re-running contract check")
			stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "answer_contract_check_auto_repair")
			var repairCheckOptions []contractCheckOption
			if res.Passed {
				// The first pass already ran any eligible LLM reviewers.
				// Deterministic auto-repair only mutates metadata / carrier
				// structure, so the follow-up pass should verify typed
				// contracts without re-emitting identical reviewer status
				// rows or spending another pair of reviewer calls. If the
				// first pass failed a strict deterministic gate, reviewers
				// were intentionally skipped and remain eligible after repair.
				repairCheckOptions = append(repairCheckOptions, contractCheckSkipLLMReview())
			}
			res = runContractCheck(out, ir.AnswerContract, o.busCtx.Mutable, o, repairCheckOptions...)
			if len(res.Violations) > 0 {
				res.Violations = ComputeRootCauseClosure(res.Violations)
			}
			if o.finishSchedulerLocalWork(stopLocal, "answer_contract_check_auto_repair", stepsUsed) {
				return stepsUsed
			}
		}

		// Phase 2.B Tier 2 ERM completeness hard gate (post-finalize,
		// answer-aware). Runs AFTER finalize composes the
		// AnswerDocumentV2 + AFTER contract violations have been
		// computed and root-cause-collapsed. The validators read
		// AnswerDocumentV2 (typed blocks first, prose second,
		// evidence pool fallback) to detect structural-coverage gaps
		// the Tier 1 breadth heuristic missed. Failures append a
		// typed Violation to the contract result; the existing
		// retry / FallbackTarget / caveat machinery handles the
		// rest (per-kind retry budget bounds the loop, exhausted
		// budget routes to AppendUserCaveatsToAnswer for graceful
		// degradation).
		//
		// Why post-finalize (not pre-finalize like Phase 2 advisory
		// was): the pre-finalize signal couldn't see the LLM's
		// actually-composed answer, only the evidence pool. An
		// answer that successfully routed around the gap (e.g.
		// LLM ran exec_command in mid-explore even though the
		// evidence projection didn't carry the result) would still
		// fire the gate. Post-finalize answer-aware validation is
		// false-positive safe — it only fires when the COMPOSED
		// ANSWER has the gap. See
		// docs/design/commercial_grade_3_pattern_remediation.md
		// Phase 2.B for the full design rationale.
		if o.busCtx.AnalysisIR != nil && o.busCtx.Mutable != nil {
			stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "tier2_completeness")
			view := types.BuildAnswerSemanticView(o.busCtx.AnalysisIR, nil)
			if view != nil {
				input := agent.ValidatorInput{
					IR:               o.busCtx.AnalysisIR,
					EvidenceItems:    o.busCtx.EvidenceItems,
					AnswerSymbols:    o.busCtx.AnswerSymbols,
					ToolResults:      o.busCtx.ToolResults,
					RepoFacts:        o.busCtx.RepoFacts,
					AggregateFacts:   o.busCtx.Mutable.StableInvestigationAggregateFacts(),
					AnswerDocumentV2: o.busCtx.Mutable.AnswerDocumentV2(),
				}
				if fail := agent.RunFamilyValidators(view.Family, input); fail != nil {
					logging.Warning("[orchestrator] Tier 2 hard-gate %s/%s emits Violation: %s",
						view.Family, fail.Dimension, fail.Reason)
					o.busCtx.Mutable.SetTier2CompletenessHint(fail.FixHint)
					// Append a typed Violation. The literal-Kind
					// switch (rather than fail.ViolationKind) is
					// required by TestAllViolationKindsHaveProducer
					// which grep-pins producer sites by matching
					// `Kind: ViolXxx` literals — variable refs are
					// invisible to the pin. The validator's
					// ViolationKind() return MUST match the constant
					// chosen here, enforced by the test below the
					// switch.
					var v types.Violation
					switch fail.Dimension {
					case agent.DimensionScalarCount:
						v = types.Violation{
							Kind:       types.ViolScalarCountUnsourced,
							Stage:      string(types.StageFinalize),
							ClusterKey: "tier2/scalar_count",
							Detail:     fail.Reason,
							Repair:     fail.FixHint,
						}
					case agent.DimensionPathDepth:
						v = types.Violation{
							Kind:       types.ViolPathDepthInsufficient,
							Stage:      string(types.StageFinalize),
							ClusterKey: "tier2/path_depth",
							Detail:     fail.Reason,
							Repair:     fail.FixHint,
						}
					case agent.DimensionCardinality:
						v = types.Violation{
							Kind:       types.ViolCardinalityShort,
							Stage:      string(types.StageFinalize),
							ClusterKey: "tier2/cardinality",
							Detail:     fail.Reason,
							Repair:     fail.FixHint,
						}
					case agent.DimensionEntityParity:
						v = types.Violation{
							Kind:       types.ViolEntityParityImbalanced,
							Stage:      string(types.StageFinalize),
							ClusterKey: "tier2/entity_parity",
							Detail:     fail.Reason,
							Repair:     fail.FixHint,
						}
					default:
						// Unknown dimension — defer to the
						// validator's declared kind (kept in sync
						// by validator design).
						v = types.Violation{
							Kind:       fail.ViolationKind,
							Stage:      string(types.StageFinalize),
							ClusterKey: "tier2/" + string(fail.Dimension),
							Detail:     fail.Reason,
							Repair:     fail.FixHint,
						}
					}
					res.Violations = append(res.Violations, v)
					// Re-run root-cause closure with the new
					// violation so IsDerived / RootKind classifiers
					// are consistent across the full set.
					res.Violations = ComputeRootCauseClosure(res.Violations)
					// The result is no longer "passed" if a Tier 2
					// violation was added.
					res.Passed = false
					o.emit(render.Event{
						Kind:       render.EventOrchestratorNotice,
						Timestamp:  time.Now(),
						Agent:      "orchestrator",
						NoticeKind: render.NoticeRetry,
						Reasoning:  softCompletenessGapMessage(o.busCtx.Language, fail),
					})
				}
			}
			if o.finishSchedulerLocalWork(stopLocal, "tier2_completeness", stepsUsed) {
				return stepsUsed
			}
		}

		if !o.strictAnswerReviewEnabledValue() {
			if actionable := FilterFinalizerRetryRootViolationsForBus(res.Violations, o.busCtx); len(actionable) > 0 {
				o.attachDraftReviewNote(out, strictReviewDisabledTitle(o.busCtx.Language), actionable)
			}
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			o.emit(render.Event{
				Kind:            render.EventLivePreviewClear,
				Timestamp:       time.Now(),
				Stage:           types.StageFinalize,
				PreviewRejected: false,
			})
			logEvidenceUtilization(o, lastFinalize)
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}

		if res.Passed {
			out.FinalAnswer = AppendSoftContractCaveatsToAnswerForBus(out.FinalAnswer, res.Violations, o.busCtx.Language, o.busCtx)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			// Live preview cleanup: contract pass means the draft
			// just streamed IS the final answer (modulo the
			// deterministic re-render). Erase the preview area
			// cleanly with no rejected marker — the bordered styled
			// answer prints next.
			o.emit(render.Event{
				Kind:            render.EventLivePreviewClear,
				Timestamp:       time.Now(),
				Stage:           types.StageFinalize,
				PreviewRejected: false,
			})
			// Observability: log how many evidence items the answer
			// actually cited vs how many the explorer collected. Used
			// to spot over-investigation patterns ("explorer collected
			// 70 evidence; finalizer cited 5") that motivate budget /
			// gate tuning. One INFO line, no behaviour change.
			logEvidenceUtilization(o, lastFinalize)
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}
		retryViolations := FilterFinalizerRetryRootViolationsForBus(res.Violations, o.busCtx)
		if len(retryViolations) == 0 {
			logging.Info("[orchestrator] contract check produced only soft/non-actionable violation(s); accepting answer without LLM retry")
			out.FinalAnswer = AppendSoftContractCaveatsToAnswerForBus(out.FinalAnswer, res.Violations, o.busCtx.Language, o.busCtx)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			o.emit(render.Event{
				Kind:            render.EventLivePreviewClear,
				Timestamp:       time.Now(),
				Stage:           types.StageFinalize,
				PreviewRejected: false,
			})
			logEvidenceUtilization(o, lastFinalize)
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}
		if !firstFinalizeRejectedForRewrite {
			firstFinalizeRejectedForRewrite = true
			firstFinalizeConcerns = append([]types.Violation(nil), retryViolations...)
		}
		retryRes := res
		retryRes.Violations = retryViolations
		// Contract failed (or SC failed). The just-streamed draft
		// is rejected — emit Clear with rejected=true so the
		// renderer flashes "已重写" briefly before erasing. Whether
		// the orchestrator now retries (next finalize round will
		// open a new preview area) or surfaces fail-loud, the
		// previous preview must be torn down here.
		o.emit(render.Event{
			Kind:            render.EventLivePreviewClear,
			Timestamp:       time.Now(),
			Stage:           types.StageFinalize,
			PreviewRejected: true,
		})

		logging.Info("[orchestrator] contract check failed (%d actionable/%d total violation(s)); retryUsed=%d/%d",
			len(retryRes.Violations), len(res.Violations), state.retryUsed, ir.TaskGraph.ExecutionPolicy.RetryBudget)
		// Per-violation debug so operators can tell, from a single log
		// line per violation, exactly which gate fired and whether the
		// retry is well-founded. Includes the is-absence flag and the
		// grounded citation-support count so the usual "why didn't the
		// absence waiver apply?" question has the data at hand.
		if logging.IsDebug() {
			absence := isJustifiedAbsenceAnswer(o.busCtx.Mutable)
			poolCount := finalizerCitationSupportCount(o.busCtx, out)
			carrier := "none"
			blockCount := 0
			if doc := o.busCtx.Mutable.AnswerDocumentV2(); doc != nil {
				carrier = "v2"
				blockCount = len(doc.Blocks)
			}
			logging.Debug("[orchestrator] contract check state: is_absence=%v carrier=%q blocks=%d citation_pool=%d",
				absence, carrier, blockCount, poolCount)
			for i, v := range res.Violations {
				logging.Debug("[orchestrator]   violation[%d] kind=%s detail=%q repair=%q",
					i, v.Kind, v.Detail, v.Repair)
			}
		}

		// P6 (2026-05-10) finalize repair hard cap. Pre-emptive
		// chokepoint: when this Run has already burnt
		// finalizeRepairHardCap finalize-stage retries, ship the
		// current draft + a single residual-concerns caveat instead
		// of another LLM round. Independent of every existing budget
		// (retryBudgetExhausted / per-kind / cross-scope attempts /
		// upstream-fallback cap) — those gates have looser defaults
		// and don't cap finalize-stage iterations directly. Forensic
		// anchor: ~8% of runs in the May-9 sweep took ≥3 repair_exec
		// rounds with diminishing returns.
		//
		// Caveat injection lives in injectResidualConcernsCaveat —
		// writes one user-vocab line into AnswerDocumentV2.Caveats
		// (matching CN/EN per BusContext.Language); does NOT modify
		// answer body. Bypasses applyContractViolations (which is
		// destructive on text); the doc reaches the user with its
		// content preserved + transparent residual disclosure.
		if hardCap := o.finalizeRepairHardCapValue(); hardCap > 0 && state.retryUsed >= hardCap {
			// P7.1 (2026-05-12) channel split: the dock gets the
			// concise "answer delivered + N concerns" status, while
			// the answer's supplementary section lists the actual
			// typed unresolved concerns. Do not append the dock
			// status line into the answer body, and do not collapse
			// concrete violations back into one generic family caveat.
			o.injectResidualConcernsCaveat(out, retryRes.Violations)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			logging.Warning("[orchestrator] P6 finalize repair hard cap reached (%d/%d); accepting doc with residual-concerns caveat",
				state.retryUsed, hardCap)
			lastFinalize = out
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}

		// W2.6 (2026-05-05): cross-scope (kind, fp) attempt budget.
		// Increments the per-Run RepairAttemptHistory for each ROOT
		// violation; when every root has reached
		// MaxRepairAttemptsPerRoot, retry is structurally pointless —
		// the same root has been dispatched MaxAttempts times across
		// any combination of mid-loop / fallback / contract retry
		// scopes. Ship with caveats instead of dispatching another
		// wasted round.
		if IncrementAttemptsAndCheckExhausted(retryRes.Violations, o.busCtx.Mutable.RepairAttempts()) {
			out.FinalAnswer = o.applyContractViolations(out.FinalAnswer, retryRes)
			out.FinalAnswer = AppendUserCaveatsToAnswer(out.FinalAnswer, retryRes.Violations, o.busCtx.Language)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			logging.Warning("[orchestrator] cross-scope repair attempts exhausted on every root; %d violation(s) materialised as user caveat",
				len(retryRes.Violations))
			lastFinalize = out
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}

		if state.retryBudgetExhausted() {
			// W1.3 (2026-05-05): replace prependFailLoudWarning's
			// internal-jargon header with materialized user-facing
			// caveats appended to the answer. Operator telemetry
			// stays on logging.Warning + closure stats; the user
			// sees natural-language caveats only.
			out.FinalAnswer = o.applyContractViolations(out.FinalAnswer, retryRes)
			out.FinalAnswer = AppendUserCaveatsToAnswer(out.FinalAnswer, retryRes.Violations, o.busCtx.Language)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			logging.Warning("[orchestrator] retry budget exhausted; %d violation(s) materialized as user caveat", len(retryRes.Violations))
			lastFinalize = out
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}

		// Session 11 F5 per-kind retry budget gate. Each kind has a
		// configurable cap (see types.RetryBudgetByKindSettings);
		// when consumed, the retry stops at the finalize contract
		// boundary without re-requeueing the explore window — the
		// LLM already had N chances to address this specific
		// violation family.
		if kind := dominantViolationKind(retryRes); kind != "" {
			cap := o.settings.RetryBudgetByKind.For(kind, ir.TaskGraph.ExecutionPolicy.RetryBudget)
			if state.retryUsedForKind(kind) >= cap {
				logging.Warning("[orchestrator] retry budget for kind=%s exhausted (%d/%d) — accepting answer with caveat",
					kind, state.retryUsedForKind(kind), cap)
				out.FinalAnswer = o.applyContractViolations(out.FinalAnswer, retryRes)
				out.FinalAnswer = AppendUserCaveatsToAnswer(out.FinalAnswer, retryRes.Violations, o.busCtx.Language)
				out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
				lastFinalize = out
				state.markDone(fin.ID)
				o.emitNodeEnd(fin.ID, true, "")
				break
			}
		}

		// Same-error-class retry governor. Per-kind caps prevent one
		// violation from looping forever, but finalizer loops can
		// rotate across sibling kinds in the same typed repair family
		// (for example one answer-coverage field fixed, another
		// answer-coverage field appears). After one retry for the
		// dominant typed class, ship the current core answer with
		// caveats instead of spending another LLM round on mechanical
		// schema churn.
		if class := dominantViolationClass(retryRes); class != "" {
			cap := sameErrorClassRetryCap()
			if cap > 0 && state.retryUsedForClass(class) >= cap {
				logging.Warning("[orchestrator] retry budget for class=%s exhausted (%d/%d) — accepting answer with caveat",
					class, state.retryUsedForClass(class), cap)
				out.FinalAnswer = o.applyContractViolations(out.FinalAnswer, retryRes)
				out.FinalAnswer = AppendUserCaveatsToAnswer(out.FinalAnswer, retryRes.Violations, o.busCtx.Language)
				out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
				lastFinalize = out
				state.markDone(fin.ID)
				o.emitNodeEnd(fin.ID, true, "")
				break
			}
		}

		// Session 11 F5 yield check. Before committing to another
		// retry window, verify the last window actually produced
		// new information (forced reads / scanned-set growth /
		// ledger drift). Zero-yield retries indicate the LLM is
		// looping — failing loud with the original answer beneath
		// a warning is better than infinitely spinning.
		stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "retry_yield_snapshot")
		currentSnapshot := captureYieldSnapshot(o.busCtx.Mutable.EvidenceClosure())
		delta := yieldDelta(state.lastYieldSnapshot, currentSnapshot)
		if o.finishSchedulerLocalWork(stopLocal, "retry_yield_snapshot", stepsUsed) {
			return stepsUsed
		}
		minYield := o.settings.ViolationBudget.MinRetryYield
		yieldFallback := FallbackTargetForViolationsWithBudget(retryRes.Violations, finalizerLocalRetriesUsed)
		finalizerOnlyTextRepair := yieldFallback == FallbackFinalizerOnly
		if shouldStopForLowRetryYield(minYield, state.retryUsed, delta, yieldFallback) {
			state.yieldKillCount++
			logging.Warning("[orchestrator] F5 yield kill: Δ=%d below MinRetryYield=%d — stopping retry loop",
				delta, minYield)
			// Audit followup (A.2, 2026-05-02): surface the kill to
			// the dock so the user knows the retry stopped early by
			// design (no new progress on any tracked axis), not a
			// silent jump from "retry" to fail-loud answer header.
			o.emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				NoticeKind: render.NoticeYieldKill,
				Reasoning:  softYieldKillMessage(o.busCtx.Language),
			})
			out.FinalAnswer = o.applyContractViolations(out.FinalAnswer, retryRes)
			out.FinalAnswer = AppendUserCaveatsToAnswer(out.FinalAnswer, retryRes.Violations, o.busCtx.Language)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			lastFinalize = out
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			break
		}
		if minYield > 0 && state.retryUsed > 0 && delta < minYield && finalizerOnlyTextRepair {
			logging.Info("[orchestrator] F5 yield check skipped for finalizer-only text repair: Δ=%d below MinRetryYield=%d target=%s",
				delta, minYield, yieldFallback)
		}
		state.lastYieldSnapshot = currentSnapshot

		// Block 3 (architecture overhaul 2026-05-02) — selective
		// upstream fallback. Pre-Block-3 this site requeued the
		// finalize node + every "done" explorer node unconditionally
		// (one-shot full backtrack), which burned wall-clock on
		// finalize-only failures (diagram_identifier,
		// shape_intent_mismatch) by re-investigating evidence that
		// was already correct.
		//
		// Coordination with existing budget gates above (T3.5 / T3.6):
		//   - retryBudgetExhausted (overall cap) breaks BEFORE this
		//   - per-kind RetryBudgetByKind exhaustion breaks BEFORE this
		//     so the most-strict gate wins
		//   - yield-kill check uses captureYieldSnapshot axes
		//     (ForcedReads + ScannedSetSize naturally correlate to
		//     explore activity; ViolationsLogged covers all stages),
		//     so upstream retries with no new evidence stop early.
		//     FinalizerOnly text repairs are exempt from the yield
		//     kill because their intended delta is a rewritten answer
		//     document, not new forced reads or scan growth; per-kind,
		//     cross-scope, and hard-cap gates still bound the loop.
		//
		// The new flow:
		//   1. Compute the deepest FallbackTarget across the
		//      violation set via FallbackTargetForViolations.
		//   2. If FailLoud, treat like retryBudgetExhausted (ship
		//      with caveats).
		//   3. If FinalizerOnly, just requeue the finalize node —
		//      keep evidence + extractor state intact.
		//   4. If BackToExtract, also requeue extract nodes (none
		//      in read mode today; reserved for future surface).
		//   5. If BackToExplore, requeue explore-stage nodes too.
		//      Cap: maxUpstreamFallbacksPerRun. Reaching the cap
		//      forces FailLoud on the next iteration so the LLM
		//      cannot endlessly re-investigate.
		//
		// requeueToStage handles the "always-include-finalize"
		// invariant centrally; partial Mutable reset keeps the
		// next dispatch's prompt clean.
		// R2.2: pass the per-Run downgrade counter so the picker
		// can prefer FinalizerOnly until the budget is exhausted.
		// G1 (post_v2_runtime_gap_remediation, 2026-05-04): route
		// through AdvanceRepairExecutionPlan which persists the
		// dispatch-ready owner queue across retries — when the prev
		// owner closes its cluster, the next-shallower owner
		// activates without re-running BuildRepairPlan from scratch.
		// Behaviour is byte-equivalent to FallbackTargetForViolationsWithBudget
		// on the FIRST failed dispatch (initial Build); subsequent
		// retries advance through the queue when fresh violations
		// are a strict subset of the previous set. The legacy
		// pickers (FallbackTargetForViolations / WithBudget) remain
		// for back-compat callers + tests.
		stopLocal = o.startSchedulerLocalWork(types.StageFinalize, "repair_plan")
		execPlan, fallback, preDowngrade := AdvanceRepairExecutionPlan(o.busCtx.Mutable, retryRes.Violations, finalizerLocalRetriesUsed)
		if o.finishSchedulerLocalWork(stopLocal, "repair_plan", stepsUsed) {
			return stepsUsed
		}
		// A4 + G1 telemetry: surface BOTH the underlying cluster
		// structure (RepairPlan) AND the dispatch-ready queue
		// (RepairExecutionPlan). Operators can grep `repair_plan=`
		// for cluster composition and `repair_exec=` for queue
		// state. Both lines are purely observational.
		logging.Info("[orchestrator] repair_plan: %s target=%s",
			SummarizeRepairPlan(BuildRepairPlan(retryRes.Violations)), fallback)
		logging.Info("[orchestrator] repair_exec: %s",
			SummarizeRepairExecutionPlan(execPlan))
		if fallback == FallbackFinalizerOnly && preDowngrade != FallbackFinalizerOnly {
			finalizerLocalRetriesUsed++
			logging.Info("[orchestrator] R2.2 finalize-local priority: primary-locus pick=%s downgraded to finalizer_only (used %d/%d)",
				preDowngrade, finalizerLocalRetriesUsed, FinalizerLocalRetryBudget())
		}
		// Cap upstream fallbacks per Run. When exceeded, force
		// FailLoud regardless of policy mapping.
		capReached := false
		if (fallback == FallbackBackToExtract || fallback == FallbackBackToExplore) &&
			o.maxUpstreamFallbacksPerRun > 0 &&
			state.upstreamFallbacksUsed >= o.maxUpstreamFallbacksPerRun {
			logging.Warning("[orchestrator] upstream fallback cap reached (%d/%d); forcing fail-loud despite kind→%s mapping",
				state.upstreamFallbacksUsed, o.maxUpstreamFallbacksPerRun, fallback)
			fallback = FallbackFailLoud
			capReached = true
			// Audit followup (2026-05-02): surface the cap event to
			// the dock so the user knows the loop terminated by
			// design — not a silent jump from "retry" to "fail-loud
			// header in answer text".
			o.emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				NoticeKind: render.NoticeUpstreamCap,
				Reasoning: softUpstreamFallbackCapMessage(o.busCtx.Language,
					state.upstreamFallbacksUsed, o.maxUpstreamFallbacksPerRun),
			})
		}
		_ = capReached
		switch fallback {
		case FallbackFailLoud:
			out.FinalAnswer = o.applyContractViolations(out.FinalAnswer, retryRes)
			out.FinalAnswer = AppendUserCaveatsToAnswer(out.FinalAnswer, retryRes.Violations, o.busCtx.Language)
			out.FinalAnswer = o.appendInactiveScopeSystemCaveatToAnswer(out.FinalAnswer)
			lastFinalize = out
			state.markDone(fin.ID)
			o.emitNodeEnd(fin.ID, true, "")
			goto contractFailureBreak
		case FallbackFinalizerOnly:
			// R14-c3: populate typed retry-state BEFORE state reset
			// so the next dispatch's BuildInitialInstruction sees the
			// prev emit + active violations. Reset clears AnswerDocV2,
			// so this MUST happen first.
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				prevAttempt := 0
				if rs := o.busCtx.Mutable.RetryState(); rs != nil {
					prevAttempt = rs.Attempt
				}
				populateRetryState(o.busCtx.Mutable, retryRes, prevAttempt)
			}
			state.requeue(fin.ID)
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				o.busCtx.Mutable.ResetForFallback(types.FallbackResetTargetFinalizer)
			}
			// B3-F3: latch — the next iteration's pre-finalize
			// extract dispatch is redundant (upstream state
			// preserved). The skip lives at line ~3406.
			lastFallbackFinalizerOnly = true
		case FallbackBackToExtract:
			// R14-c3: populate retry-state BEFORE Mutable reset.
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				prevAttempt := 0
				if rs := o.busCtx.Mutable.RetryState(); rs != nil {
					prevAttempt = rs.Attempt
				}
				populateRetryState(o.busCtx.Mutable, retryRes, prevAttempt)
			}
			// Read-mode TaskGraph has no NodeExtract today (extract
			// is implicit in the explore pass); selective requeue
			// reduces to FinalizerOnly + extractor state reset.
			state.requeue(fin.ID)
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				o.busCtx.Mutable.ResetForFallback(types.FallbackResetTargetExtract)
			}
			state.upstreamFallbacksUsed++
			// extract DID change (we cleared the slate) — next
			// iteration's pre-finalize extract dispatch is correct.
			lastFallbackFinalizerOnly = false
		case FallbackBackToExplore:
			// R14-c3: populate retry-state BEFORE Mutable reset.
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				prevAttempt := 0
				if rs := o.busCtx.Mutable.RetryState(); rs != nil {
					prevAttempt = rs.Attempt
				}
				populateRetryState(o.busCtx.Mutable, retryRes, prevAttempt)
			}
			requeued := state.requeueToStage(types.StageExplore, false)
			state.requeue(fin.ID)
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				cleared := o.busCtx.Mutable.ResetForFallback(types.FallbackResetTargetExplore)
				logging.Info("[orchestrator] selective fallback explore: requeued=%v cleared=%v",
					requeued, cleared)
			}
			state.upstreamFallbacksUsed++
			lastFallbackFinalizerOnly = false
		default:
			// Unrecognised target — degrade to FinalizerOnly so
			// retries always make progress.
			state.requeue(fin.ID)
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				o.busCtx.Mutable.ResetForFallback(types.FallbackResetTargetFinalizer)
			}
			lastFallbackFinalizerOnly = true
		}
		state.recordRetry()
		// Block 1 (architecture overhaul 2026-05-02) — also bump the
		// closure's stage-wise retry counter so StageHealthSnapshot
		// surfaces "finalize re-dispatched N times during this Run"
		// without callers re-walking the scheduler state. The retry
		// happens because a finalize contract violation triggered
		// requeue above, so the StageFinalize bucket is the
		// authoritative location for the retry signal.
		if o.busCtx != nil && o.busCtx.Mutable != nil {
			if cl := o.busCtx.Mutable.EvidenceClosure(); cl != nil {
				cl.IncrementStageRetry(string(types.StageFinalize))
			}
		}
		// C6: bump the per-kind counter for the dominant violation
		// so subsequent iterations see the cap getting tighter.
		if kind := dominantViolationKind(retryRes); kind != "" {
			state.recordRetryByKind(kind)
		}
		if class := dominantViolationClass(retryRes); class != "" {
			state.recordRetryByClass(class)
		}
		pendingViolation = renderViolations(retryRes)

		// Surface the backtrack to the user so they know the pipeline
		// is re-investigating, not stalled. The full violation
		// breakdown (rendered by renderViolations) carries criterion
		// kind names and internal field jargon; it's retained on
		// pendingViolation (which the NEXT window hint will consume
		// to target the retry) but stripped from the user-facing
		// event per the user_messages.go contract.
		// Audit followup (2026-05-02): surface the SELECTIVE-FALLBACK
		// target to the dock so the user can see exactly which layer
		// is being re-run (just polishing answer / restructuring /
		// re-investigating). Pre-this-fix the same generic
		// "answer needs another pass" message rendered for every
		// target, hiding the real scope of the retry.
		o.emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      "orchestrator",
			NoticeKind: noticeKindForFallbackTarget(fallback),
			Reasoning:  softFallbackTargetMessage(o.busCtx.Language, fallback),
		})
	}

contractFailureBreak:
	if lastFinalize == nil {
		// Force one finalize dispatch so the task always terminates
		// with a Result.
		logging.Warning("[orchestrator] DAG run produced no finalize output; forcing finalize")

		if recovered := o.recoverRejectedFinalizerDraftAfterTransientFailure(ir.AnswerContract); recovered != nil {
			logging.Warning("[orchestrator] recovered prior rejected finalizer draft before forced finalize; skipping upstream extract replay")
			lastFinalize = recovered
		}

		if lastFinalize == nil {
			if out, _, handled := o.handleStructurallyEmptyInvestigation(state, ""); handled {
				if out != nil {
					lastFinalize = out
				}
			}
		}
		if lastFinalize != nil {
			lastFinalize.FinalAnswer = appendRuntimeDispatchAdvisoriesToAnswer(lastFinalize.FinalAnswer, runtimeDispatchAdvisories, o.busCtx.Language)
			o.recordTaskFinalize(lastFinalize)
			o.emitCGECSummary()
			return stepsUsed
		}

		if lastFinalize == nil {
			// Extract before forced finalize only when upstream state is
			// genuinely missing. If Turn B has already produced typed
			// answer symbols, chains, or hypothesis verdicts, replaying
			// extract after a finalizer transport/schema failure causes a
			// visible 4/4 → 3/4 stage regression and burns another LLM
			// round without adding evidence.
			if preFinalizeExtractCompleted || o.hasReusableTurnBSlateForFinalize() {
				logging.Info("[orchestrator] skip pre-forced-finalize extract: Turn-B state already available (extract_completed=%t %s)",
					preFinalizeExtractCompleted, o.reusableTurnBSlateSummary())
				lastFallbackFinalizerOnly = false
			} else {
				o.busCtx.PipelineStage = types.StageExtract
				o.busCtx.TaskState.Stage = types.StageExtract
				for {
					if _, exErr := o.dispatchStage(types.StageExtract); exErr != nil {
						if o.completeExtractAfterTransientProgress(exErr) {
							stepsUsed++
							preFinalizeExtractCompleted = true
							break
						}
						if o.retryReadStandaloneDispatchError(state, types.StageExtract, exErr) {
							continue
						}
						stepsUsed++
						logging.Warning("[orchestrator] pre-forced-finalize extract dispatch failed (continuing): %v", exErr)
						runtimeDispatchAdvisories = appendUniqueRuntimeAdvisory(
							runtimeDispatchAdvisories,
							runtimeDispatchAdvisoryForStage(types.StageExtract, exErr, o.busCtx.Language),
						)
						// Force-finalize escape path: same transparency requirement
						// as the normal DAG branch — let the user see that we are
						// dropping extract results before finalize starts, instead
						// of silently jumping from "未能提炼关键发现" to "正在生成
						// 最终答案".
						o.emit(render.Event{
							Kind:       render.EventOrchestratorNotice,
							Timestamp:  time.Now(),
							Agent:      "orchestrator",
							NoticeKind: render.NoticeProceedingWithoutExtract,
							Reasoning:  softProceedingWithoutExtractMessage(o.busCtx.Language),
						})
						break
					}
					stepsUsed++
					preFinalizeExtractCompleted = true
					stopLocal := o.startSchedulerLocalWork(types.StageExtract, "drain_hypothesis_verdicts")
					o.drainHypothesisVerdicts()
					if o.finishSchedulerLocalWork(stopLocal, "drain_hypothesis_verdicts", stepsUsed) {
						return stepsUsed
					}
					break
				}
			}
		}

		o.busCtx.PipelineStage = types.StageFinalize
		o.busCtx.TaskState.Stage = types.StageFinalize
		// Same pre-dispatch cue as the normal DAG path — the stall
		// escape still runs a composition-only LLM call with no tool
		// activity.
		o.emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      "orchestrator",
			NoticeKind: render.NoticeFinalizing,
			Reasoning:  softFinalizingMessage(o.busCtx.Language),
		})
		// Force-finalize transient retry. Pre-2026-04-30 the
		// force-finalize was a single-shot dispatch; an
		// `unexpected EOF` / `stream stalled` blip on the
		// composition-only call (which is the user's only chance
		// to recover ANY answer at this point) immediately
		// terminated the Run with a raw English error string.
		// Network connections drop occasionally; a 2-retry budget
		// (3 total attempts) with short backoff catches the
		// overwhelming majority of transient blips while still
		// bailing out promptly when the upstream is genuinely down.
		// Operator override via codrax.yaml ::
		// pipeline_force_finalize_attempts (default 3, capped at 5).
		forceFinalizeMaxAttempts := o.forceFinalizeAttempts
		if forceFinalizeMaxAttempts <= 0 {
			forceFinalizeMaxAttempts = types.DefaultForceFinalizeAttempts
		}
		var (
			out *agent.StageOutput
			err error
		)
		for attempt := 0; attempt < forceFinalizeMaxAttempts; attempt++ {
			if o.busCtx.Mutable != nil {
				o.busCtx.Mutable.ResetActiveAnswerDocumentV2ForFinalizeDispatch()
			}
			out, err = o.dispatchStage(types.StageFinalize)
			if err == nil {
				stepsUsed++
				break
			}
			// L5 force-finalize retry is restricted to stream-level
			// errors. HTTP 429 / 5xx are L1's domain — L1's 6-attempt
			// × 62-second budget already exhausted by the time we
			// see them, so additional retry burns wall-clock without
			// recovery benefit. Stream-level errors (EOF / stalled /
			// first-byte timeout / network blip) are NOT retried by
			// L1 (they could duplicate streamed content), so L5's
			// retries are the only coverage they get.
			if !llm.IsStreamLevelRetryable(err) {
				stepsUsed++
				break
			}
			if attempt+1 >= forceFinalizeMaxAttempts {
				stepsUsed++
				break
			}
			logging.Warning("[orchestrator] forced finalize transient failure (attempt %d/%d): %v — retrying",
				attempt+1, forceFinalizeMaxAttempts, err)
			// Surface a brief recovery cue to the user so the
			// spinner area shows we're not stuck silent.
			o.emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				NoticeKind: render.NoticeRetry,
				Reasoning:  softTransportRetryHintForStage(o.busCtx.Language, types.StageFinalize),
			})
			// Reuse the LLM adapter's production-tested backoff
			// schedule (Retry-After header > quota long-ramp >
			// standard exponential). Cap at 5s so the worst-case
			// user-visible pause across the full force-finalize
			// retry window stays under ~10s — appropriate for the
			// last-resort path which the user already tolerates as
			// "the answer is being composed".
			backoff := llm.NextRetryDelay(err, attempt)
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			// Cancellation-aware wait: a /cancel during the
			// force-finalize retry window must not be stranded
			// waiting up to 15s (5s × 3 attempts). select on
			// busCtx.Ctx.Done so the loop bails immediately. nil
			// ctx (test fixtures) falls through to a regular timer.
			if o.busCtx != nil {
				if c := o.busCtx.Context(); c != nil {
					timer := time.NewTimer(backoff)
					select {
					case <-c.Done():
						timer.Stop()
						o.busCtx.Mutable.SetResult("")
						o.busCtx.TaskState.LastError = forcedFinalizeFailureMessage(c.Err(), o.busCtx.Language)
						return stepsUsed
					case <-timer.C:
					}
					continue
				}
			}
			time.Sleep(backoff)
		}
		if err != nil {
			if fallback := o.finalizerNoVisibleOutputFallback(err); fallback != nil {
				logging.Warning("[orchestrator] forced finalize produced no visible output; delivering degraded answer from existing Turn-B/evidence slate: %v", err)
				lastFinalize = fallback
				err = nil
			}
		}
		if err != nil {
			logging.Error("[orchestrator] forced finalize failed after %d attempts: %v",
				forceFinalizeMaxAttempts, err)
			o.busCtx.Mutable.SetResult("")
			o.busCtx.TaskState.LastError = forcedFinalizeFailureMessage(err, o.busCtx.Language)
			return stepsUsed
		}
		lastFinalize = out
	}

	if o.strictAnswerReviewEnabledValue() {
		o.attachFirstDraftReference(lastFinalize, firstFinalizeDraft, firstFinalizeConcerns, firstFinalizeRejectedForRewrite)
	}
	if lastFinalize != nil {
		lastFinalize.FinalAnswer = appendRuntimeDispatchAdvisoriesToAnswer(lastFinalize.FinalAnswer, runtimeDispatchAdvisories, o.busCtx.Language)
	}
	o.recordTaskFinalize(lastFinalize)
	o.emitCGECSummary()
	return stepsUsed
}

// computeRichnessCoverage (Phase 5 of Semantic Surface Contract,
// 2026-05-02) returns the (covered, total) pair for the Run's
// FacetCoverageContract.Optional entries. Total is len(Optional)
// at compile time; covered is total minus the count of
// ViolRichnessRegression entries in the closure ledger. Returns
// (0, 0) when there is no active question / no optional facets /
// no closure — caller skips the [CGEC] summary tail.
//
// Recompiles the contract from RequestModel + EmittedEvidence at
// summary time so the count reflects the post-extract evidence
// pool (matches what runRichnessTelemetryOracle saw). Cheap:
// CompileFacetCoverage runs in microseconds on the typical surface.
func (o *Orchestrator) computeRichnessCoverage() (covered, total int) {
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return 0, 0
	}
	mut := o.busCtx.Mutable
	rm := mut.RequestModel()
	if rm == nil {
		return 0, 0
	}
	contract := types.CompileFacetCoverage(*rm, mut.EmittedEvidence(), mut)
	if contract == nil {
		return 0, 0
	}
	total = len(contract.Optional)
	if total == 0 {
		return 0, 0
	}
	closure := mut.EvidenceClosure()
	regressions := 0
	if closure != nil {
		regressions = len(closure.ViolationsByKind(types.ViolRichnessRegression))
	}
	covered = total - regressions
	if covered < 0 {
		covered = 0
	}
	return covered, total
}

// emitCGECSummary renders the per-task CGEC counter snapshot to the
// operator trace. Always emits a single line so operators can grep
// [CGEC] summary even on no-op tasks — a "no enforcer fired" line
// is a positive signal that the closure is quiet, which is itself
// diagnostic information. Called at the end of runTaskGraph after
// all stages have exited.
//
// This line is log-only: it carries internal counter names that are
// noise to end users, and the renderer already surfaces task
// completion via stage-end events. Operator-facing only.
func (o *Orchestrator) emitCGECSummary() {
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	stats := closure.Stats()
	// CGEC frequency bridge: high-frequency CGEC events get a
	// SOFT violation entry so operators see the storm in the
	// closure ledger / by_field tally, not just the raw counter
	// in the [CGEC] summary INFO line. Telemetry-only (Severity=
	// Soft per types.DeriveSeverity); never blocks shipping.
	emitCGECStormViolations(closure, stats)
	var line string
	if !stats.HasActivity() {
		line = "[CGEC] summary: no enforcer fired (contract quiet)"
	} else {
		line = fmt.Sprintf(
			"[CGEC] summary: chains_demoted=%d unverified=%d repairs_raised=%d expand_search=%d shape_swap=%d pre_complete_downgrades=%d forced_reads=%d stall_soft=%d stall_hard=%d",
			stats.ChainsDemoted, stats.UnverifiedFinds, stats.RepairsRaised,
			stats.ExpandSearchRaised, stats.ViewSwapRaised,
			stats.PreCompleteDowngrades, stats.ForcedReads,
			stats.StallSoftHits, stats.StallHardHits)
		// Session 11 F1: extended summary with ViolationLedger view.
		// Keep the extension tail-appended so existing log parsers
		// that match the pre-session-11 prefix still work. The tail
		// prints nothing when the ledger is empty (zero cost when
		// F1 hookups are a no-op on a healthy run).
		if stats.ViolationsLogged > 0 {
			if topField, topCount, topConf := closure.TopSuspectedField(); topField != "" {
				line = fmt.Sprintf("%s violations=%d by_field=%s top_suspected=(%s,conf=%.2f,events=%d)",
					line, stats.ViolationsLogged,
					formatViolationFieldTally(closure.ViolationFieldTally()),
					topField, topConf, topCount)
			} else {
				line = fmt.Sprintf("%s violations=%d (no_suspected_root)", line, stats.ViolationsLogged)
			}
		}
	}
	// Phase 5 (Semantic Surface Contract, 2026-05-02) — richness
	// telemetry tail. Prints optional_facets_covered=N/M when the
	// active question had any FacetOptional / TierEnrichment entries.
	// Always tail-appended so legacy log parsers stay byte-stable
	// on Runs without optional facets.
	if covered, total := o.computeRichnessCoverage(); total > 0 {
		line = fmt.Sprintf("%s optional_facets_covered=%d/%d", line, covered, total)
	}
	logging.Info("%s", line)

	// Block 1 (architecture overhaul 2026-05-02) — stage-wise
	// breakdown line. Distinct INFO record so existing log parsers
	// that grep the [CGEC] summary: prefix do not parse the
	// stage-wise data accidentally. Empty (no stage activity) prints
	// nothing — the byte-identical pre-2026-05-02 path stays clean
	// for healthy Runs.
	if snapshot := closure.StageHealthSnapshot(); len(snapshot) > 0 {
		stageLine := formatStageHealthSnapshot(snapshot)
		if stageLine != "" {
			logging.Info("[CGEC] perStage: %s", stageLine)
		}
	}

	// B5-F2 / B5-F3 richness telemetry surface. Each signal
	// renders a single WARN line so operators can grep the silent
	// rule firings (facet softening / family underrepresentation).
	// Gated by pipeline_richness_softening_warn (default true);
	// telemetry is always recorded regardless of the WARN gate.
	if richnessSofteningWarnEnabled() {
		for _, sig := range o.busCtx.Mutable.RichnessTelemetry() {
			logging.Warning("[richness] %s family=%s facet_id=%s facet_kind=%s buckets=%d reason=%s",
				sig.Kind, sig.Family, sig.FacetID, sig.FacetKind, sig.BucketCount, sig.Reason)
		}
	}
}

// CGEC frequency bridge thresholds (R10). When the per-Run scalar
// counter exceeds the corresponding threshold, the bridge emits a
// SOFT-by-default violation so operators see the storm signal in
// closure ledger / [CGEC] by_field tally rather than only the raw
// counter in the summary line. Wired via cmd/root.go from yaml
// pipeline_demotion_storm_threshold / pipeline_forced_read_storm_threshold.
//
// Setting either threshold to 0 disables the corresponding bridge
// (no storm violation ever emits). Negative values reset to default.
var (
	demotionStormThreshold   = 10
	forcedReadStormThreshold = 8
)

// SetCGECStormThresholds is wired from cmd/root.go yaml merge.
// Non-positive values reset to defaults (10 / 8). Pass any positive
// integer to tune.
func SetCGECStormThresholds(demotion, forcedRead int) {
	if demotion > 0 {
		demotionStormThreshold = demotion
	} else if demotion == 0 {
		demotionStormThreshold = 0 // explicit disable
	} else {
		demotionStormThreshold = 10
	}
	if forcedRead > 0 {
		forcedReadStormThreshold = forcedRead
	} else if forcedRead == 0 {
		forcedReadStormThreshold = 0 // explicit disable
	} else {
		forcedReadStormThreshold = 8
	}
}

// emitCGECStormViolations is the R10 frequency-to-violation bridge.
// At end-of-Run, when ClosureStats.ChainsDemoted exceeds the
// configured threshold, append one ViolDemotionStorm entry;
// similarly for ChainsForcedReads → ViolForcedReadStorm. SOFT
// kinds — never block shipping, just visible in operator tooling.
//
// Idempotent within a Run: emits at most one ViolDemotionStorm +
// one ViolForcedReadStorm per Run (driven by the same counter, so
// re-calling emitCGECSummary would just produce the same dup —
// harmless because closure.AppendViolation appends regardless;
// callers shouldn't re-invoke emitCGECSummary anyway).
func emitCGECStormViolations(closure *types.EvidenceClosure, stats types.ClosureStats) {
	if closure == nil {
		return
	}
	if demotionStormThreshold > 0 && stats.ChainsDemoted >= demotionStormThreshold {
		closure.AppendViolation(types.Violation{
			Kind:       types.ViolDemotionStorm,
			ClusterKey: types.RootClusterKey("explorer_chain_quality"),
			Detail: fmt.Sprintf(
				"chains_demoted=%d (threshold %d) — explorer over-produced low-quality chains; review keyword_search noise",
				stats.ChainsDemoted, demotionStormThreshold),
			Repair: "operator-side: review explorer's keyword_search ranking + chain promotion thresholds (no LLM action required — telemetry only).",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "explorer_chain_quality",
				Reason:     "chains_demoted exceeds storm threshold",
				Confidence: 0.6,
			},
			Stage: string(types.StageFinalize),
		})
	}
	if forcedReadStormThreshold > 0 && stats.ForcedReads >= forcedReadStormThreshold {
		closure.AppendViolation(types.Violation{
			Kind:       types.ViolForcedReadStorm,
			ClusterKey: types.RootClusterKey("explorer_anchor_coverage"),
			Detail: fmt.Sprintf(
				"forced_reads=%d (threshold %d) — explorer skipped primary anchors that the orchestrator had to page in via Lazy Auto-Read",
				stats.ForcedReads, forcedReadStormThreshold),
			Repair: "operator-side: review keyword_search coverage of primary entities; Lazy Auto-Read should be a fallback, not the main path.",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "explorer_anchor_coverage",
				Reason:     "forced_reads exceeds storm threshold",
				Confidence: 0.6,
			},
			Stage: string(types.StageFinalize),
		})
	}
}

// richnessSofteningWarnEnabled is set by cmd/root.go after merging
// codrax.yaml. We use a function indirection so the orchestrator
// package doesn't take a hard import on cmd. Default true preserves
// "silent rule visibility" — operators must opt out explicitly.
var richnessSofteningWarnEnabled = func() bool { return true }

// SetRichnessSofteningWarnEnabled is wired by cmd/root.go to thread
// the pipeline_richness_softening_warn yaml knob.
func SetRichnessSofteningWarnEnabled(enabled bool) {
	richnessSofteningWarnEnabled = func() bool { return enabled }
}

// finalizerRetryNoThinkEnabled gates B6-F4 — dynamic think_aloud
// disable on finalizer retry iterations. Function indirection mirrors
// SetRichnessSofteningWarnEnabled so the orchestrator package does
// not take a hard import on cmd. Default true (savings on by default;
// the per-agent ThinkAloud yaml is unchanged for iteration 0).
var finalizerRetryNoThinkEnabled = func() bool { return true }

// SetFinalizerRetryNoThinkEnabled is wired by cmd/root.go to thread
// the pipeline_finalizer_retry_no_think yaml knob.
func SetFinalizerRetryNoThinkEnabled(enabled bool) {
	finalizerRetryNoThinkEnabled = func() bool { return enabled }
}

// formatStageHealthSnapshot renders a closure's StageHealthSnapshot
// into a stable single-line breakdown. Stages emit in canonical
// pipeline order (analyze / explore / extract / finalize / plan /
// apply / verify); empty-stats stages are dropped so the line stays
// lean. Field order inside each stage record is fixed so the line
// is grep-friendly across Runs.
func formatStageHealthSnapshot(snapshot map[string]types.StageStats) string {
	if len(snapshot) == 0 {
		return ""
	}
	canonicalOrder := []string{
		string(types.StageAnalyze),
		string(types.StageExplore),
		string(types.StageExtract),
		string(types.StageFinalize),
		string(types.StagePlan),
		string(types.StageApply),
		string(types.StageVerify),
	}
	var parts []string
	for _, stage := range canonicalOrder {
		st, ok := snapshot[stage]
		if !ok || st.IsEmpty() {
			continue
		}
		parts = append(parts, fmt.Sprintf(
			"%s={retries:%d,violations:%d,contradictions:%d,repairs:%d,pending_reads:%d}",
			stage, st.Retries, st.Violations, st.Contradictions, st.Repairs, st.PendingReads))
	}
	return strings.Join(parts, " ")
}

// applyWindowHint writes the rendered DAG-window hint into the
// shared TaskState.RetryHint slot so BuildAgentContext picks it up
// and BuildPromptContext renders it as the "Retry Directive (READ
// FIRST)" section. This is the only state field the DAG scheduler
// modifies on BusContext outside the standard PipelineStage / Stage
// fields. Empty hint clears the slot.
func (o *Orchestrator) applyWindowHint(hint string) {
	o.busCtx.TaskState.RetryHint = hint
	const hintKey = "orchestrator.dag-window"
	if hint == "" {
		logging.Debug("[orchestrator] window hint cleared key=%q", hintKey)
		return
	}
	logging.Debug("[orchestrator] window hint applied key=%q len=%d body=%q",
		hintKey, len(hint), logging.Truncate(hint, logging.HintBodyMax))
}

func prependRetryHint(prefix, hint string) string {
	prefix = strings.TrimSpace(prefix)
	hint = strings.TrimSpace(hint)
	switch {
	case prefix == "":
		return hint
	case hint == "":
		return prefix
	default:
		return prefix + "\n\n" + hint
	}
}

func (o *Orchestrator) autoCompleteReadyReconcileNodes(state *graphState, window []*types.TaskNode, env criterion.Env) []*types.TaskNode {
	if len(window) == 0 {
		return window
	}
	remaining := make([]*types.TaskNode, 0, len(window))
	skipped := 0
	for _, n := range window {
		if !o.shouldAutoCompleteReadyReconcileNode(n, env) {
			remaining = append(remaining, n)
			continue
		}
		state.markRunning(n.ID)
		o.emitNodeStart(n.ID)
		state.markDone(n.ID)
		o.emitNodeEnd(n.ID, true, "skipped: reconciled from existing evidence")
		skipped++
		logging.Info("[orchestrator] auto-completed reconcile node %s from existing evidence", n.ID)
	}
	if skipped > 0 && len(remaining) == 0 {
		o.drainIgnorableReconcileRepairs()
	}
	return remaining
}

func (o *Orchestrator) shouldAutoCompleteReadyReconcileNode(n *types.TaskNode, env criterion.Env) bool {
	if n == nil || n.Type != types.NodeReconcile {
		return false
	}
	acceptedClosureEnough := o.acceptedClosureCanSatisfyReconcileEnoughFacts()
	if !acceptedClosureEnough && !env.Signals.HasEnoughFacts && (o.busCtx == nil || !o.busCtx.Signals.HasEnoughFacts) {
		return false
	}
	if len(n.SuccessCriteria) > 0 {
		evalEnv := env
		if acceptedClosureEnough {
			evalEnv.Signals.HasEnoughFacts = true
			evalEnv.InvestigationComplete = true
			if o.busCtx != nil && o.busCtx.Mutable != nil {
				evalEnv.AggregateFacts = o.busCtx.Mutable.StableInvestigationAggregateFacts()
			}
		}
		ok, _ := criterion.EvalAll(n.SuccessCriteria, evalEnv)
		if !ok {
			return false
		}
	}
	if !o.hasReconcileEvidenceContext() {
		return false
	}
	return !o.hasBlockingReconcileRepair()
}

func (o *Orchestrator) acceptedClosureCanSatisfyReconcileEnoughFacts() bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	if !o.hasReconcileEvidenceContext() {
		return false
	}
	policy := o.effectiveInvestigationCompletePolicy()
	if policy != types.ICPolicySoft && policy != types.ICPolicyOverride {
		return false
	}
	mut := o.busCtx.Mutable
	if !mut.IsInvestigationComplete() && strings.TrimSpace(mut.StableInvestigationCompleteReason()) == "" {
		return false
	}
	if policy == types.ICPolicyOverride {
		return true
	}
	if missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete(); len(missing) > 0 {
		logging.Info("[orchestrator] accepted investigation closure cannot auto-complete reconcile node; missing_origin_lanes=%s",
			formatAnswerEvidenceOriginsForLog(missing))
		return false
	}
	closure := mut.EvidenceClosure()
	if closure == nil {
		return true
	}
	for _, repair := range closure.PendingRepairs() {
		if o.repairBlocksAcceptedClosure(repair) {
			return false
		}
	}
	for _, pending := range closure.PendingReads() {
		if pendingReadBlocksAcceptedReconcileClosure(pending) {
			return false
		}
	}
	return true
}

func pendingReadBlocksAcceptedReconcileClosure(p types.PendingRead) bool {
	origin := strings.TrimSpace(p.Origin)
	if strings.HasPrefix(origin, "chain_promotion.") {
		return false
	}
	return types.PendingReadBlocksAcceptedClosure(p)
}

func (o *Orchestrator) recordAcceptedClosureValidationBoundaries(window []*types.TaskNode, env criterion.Env, source string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || len(window) == 0 {
		return
	}
	notes := acceptedClosureValidationBoundaryNotes(window, env, source)
	if len(notes) == 0 {
		return
	}
	var snapshot types.TurnAArtifacts
	if existing := o.busCtx.Mutable.TurnAArtifacts(); existing != nil {
		snapshot = *existing
	}
	seen := make(map[string]bool, len(snapshot.ValidationBoundaryNotes)+len(notes))
	for _, note := range snapshot.ValidationBoundaryNotes {
		if trimmed := strings.TrimSpace(note); trimmed != "" {
			seen[trimmed] = true
		}
	}
	added := 0
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" || seen[note] {
			continue
		}
		snapshot.ValidationBoundaryNotes = append(snapshot.ValidationBoundaryNotes, note)
		seen[note] = true
		added++
	}
	if added == 0 {
		return
	}
	o.busCtx.Mutable.SetTurnAArtifacts(snapshot)
	logging.Info("[orchestrator] recorded %d accepted-closure validation boundary note(s)", added)
}

func acceptedClosureValidationBoundaryNotes(window []*types.TaskNode, env criterion.Env, source string) []string {
	const maxBoundaryNotes = 8
	var notes []string
	for _, n := range window {
		if n == nil || len(n.SuccessCriteria) == 0 {
			continue
		}
		if ok, failed := criterion.EvalAll(n.SuccessCriteria, env); !ok {
			for _, result := range failed {
				note := formatAcceptedClosureValidationBoundary(n, result, source)
				if note == "" {
					continue
				}
				notes = append(notes, note)
				if len(notes) >= maxBoundaryNotes {
					return notes
				}
			}
		}
	}
	return notes
}

func formatAcceptedClosureValidationBoundary(n *types.TaskNode, result criterion.Result, source string) string {
	kind := strings.TrimSpace(string(result.Kind))
	if kind == "" {
		kind = "unknown"
	}
	nodeID := ""
	if n != nil {
		nodeID = strings.TrimSpace(n.ID)
	}
	if nodeID == "" {
		nodeID = "unknown"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "accepted_closure"
	}
	expr := strings.TrimSpace(result.Expr)
	detail := truncateBoundaryNoteField(result.Detail, 260)
	advice := acceptedClosureBoundaryAdvice(kind)
	parts := []string{
		fmt.Sprintf("source=%s", source),
		fmt.Sprintf("node=%s", nodeID),
		fmt.Sprintf("criterion=%s", kind),
	}
	if expr != "" {
		parts = append(parts, fmt.Sprintf("expr=%q", expr))
	}
	if detail != "" {
		parts = append(parts, fmt.Sprintf("detail=%q", detail))
	}
	if advice != "" {
		parts = append(parts, fmt.Sprintf("downstream=%q", advice))
	}
	return strings.Join(parts, "; ")
}

func acceptedClosureBoundaryAdvice(kind string) string {
	switch kind {
	case types.CritAnswerSetBounded:
		return "candidate set remained broader than the requested/compiled bound; answer composition should group, rank, or summarize the supported candidates and disclose the boundary instead of forcing another exploration pass"
	case types.CritAllHypothesesDecided:
		return "some hypotheses remain unresolved; answer composition should preserve the unresolved branch as uncertainty/caveat rather than inventing a decision"
	case types.CritEvidenceCount, types.CritHasEnoughFacts, types.CritNoRelevantEvidence:
		return "evidence remained below the preferred validation threshold after accepted closure; answer composition should stay within collected evidence and disclose the scope/evidence boundary"
	default:
		return "answer composition should make the user-facing boundary clear when it affects certainty; do not invent facts solely to satisfy the validation criterion"
	}
}

func truncateBoundaryNoteField(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max == 1 {
		return "."
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func (o *Orchestrator) shouldAutoCompleteExploreWindowFromAcceptedClosure(pendingValidationTargets []string, pendingViolation, pendingStageRetry string) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	policy := o.effectiveInvestigationCompletePolicy()
	if policy != types.ICPolicySoft && policy != types.ICPolicyOverride {
		return false
	}
	mut := o.busCtx.Mutable
	if !mut.IsInvestigationComplete() && strings.TrimSpace(mut.StableInvestigationCompleteReason()) == "" {
		return false
	}
	if policy == types.ICPolicyOverride {
		return true
	}
	if len(pendingValidationTargets) > 0 || strings.TrimSpace(pendingViolation) != "" || strings.TrimSpace(pendingStageRetry) != "" {
		return false
	}
	if missing := o.acceptedClosureMissingRequiredOriginsForAutoComplete(); len(missing) > 0 {
		logging.Info("[orchestrator] accepted investigation closure cannot auto-complete mixed-origin explore window; missing_origin_lanes=%s",
			formatAnswerEvidenceOriginsForLog(missing))
		return false
	}
	closure := mut.EvidenceClosure()
	if closure == nil {
		return true
	}
	for _, repair := range closure.PendingRepairs() {
		if o.repairBlocksAcceptedClosure(repair) {
			return false
		}
	}
	for _, pending := range closure.PendingReads() {
		if types.PendingReadBlocksAcceptedClosure(pending) {
			return false
		}
	}
	return true
}

func (o *Orchestrator) acceptedClosureMissingRequiredOriginsForAutoComplete() []types.AnswerEvidenceOrigin {
	if o == nil || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return nil
	}
	rm := o.busCtx.AnalysisIR.RequestModel
	contract := &o.busCtx.AnalysisIR.AnswerContract
	if !parallelExploreMixedOriginNeedsSiblingHandoffs(rm, contract) {
		return nil
	}
	intentContract := types.CompileAnswerIntentContract(rm, contract)
	required := requiredMixedOriginAutoCompleteLanes(intentContract)
	if len(required) == 0 {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, 128))
	return missingObservationOrigins(required, ledger)
}

func requiredMixedOriginAutoCompleteLanes(contract types.AnswerIntentContract) []types.AnswerEvidenceOrigin {
	hasCurrent := false
	hasNonSource := false
	for _, origin := range contract.Origins {
		switch {
		case origin == types.AnswerEvidenceOriginCurrentSource:
			hasCurrent = true
		case origin != types.AnswerEvidenceOriginUnknown && types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin):
			hasNonSource = true
		}
	}
	if !hasCurrent || !hasNonSource {
		return nil
	}
	var out []types.AnswerEvidenceOrigin
	for _, origin := range contract.Origins {
		if origin == types.AnswerEvidenceOriginCurrentSource || types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			out = append(out, origin)
		}
	}
	return out
}

func missingObservationOrigins(required []types.AnswerEvidenceOrigin, ledger types.ObservationLedger) []types.AnswerEvidenceOrigin {
	if len(required) == 0 {
		return nil
	}
	present := make(map[types.AnswerEvidenceOrigin]bool, len(required))
	for _, record := range ledger.Records {
		switch record.Origin {
		case types.AnswerEvidenceOriginUnknown:
			continue
		case types.AnswerEvidenceOriginCurrentSource:
			if types.ObservationRecordHasCurrentSourceLineSpan(record) {
				present[record.Origin] = true
			}
		default:
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
				present[record.Origin] = true
			}
		}
	}
	var missing []types.AnswerEvidenceOrigin
	for _, origin := range required {
		if origin == types.AnswerEvidenceOriginUnknown || present[origin] {
			continue
		}
		missing = append(missing, origin)
	}
	return missing
}

func formatAnswerEvidenceOriginsForLog(origins []types.AnswerEvidenceOrigin) string {
	if len(origins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin == types.AnswerEvidenceOriginUnknown {
			continue
		}
		parts = append(parts, string(origin))
	}
	return strings.Join(parts, ",")
}

func (o *Orchestrator) effectiveInvestigationCompletePolicy() string {
	if o == nil {
		return types.ICPolicySoft
	}
	switch o.settings.Agent.InvestigationCompletePolicy {
	case types.ICPolicySoft, types.ICPolicyOverride, types.ICPolicyStrict:
		return o.settings.Agent.InvestigationCompletePolicy
	default:
		return types.ICPolicySoft
	}
}

func (o *Orchestrator) hasReconcileEvidenceContext() bool {
	if o == nil || o.busCtx == nil {
		return false
	}
	if len(o.busCtx.EvidenceItems) > 0 || len(o.busCtx.FlowFindings) > 0 || len(o.busCtx.AnswerChains) > 0 || len(o.busCtx.AnswerSymbols) > 0 {
		return true
	}
	if o.busCtx.Mutable == nil {
		return false
	}
	if len(o.busCtx.Mutable.EmittedEvidence()) > 0 {
		return true
	}
	if turnA := o.busCtx.Mutable.TurnAArtifacts(); turnA != nil {
		if len(turnA.EvidenceItems) > 0 || len(turnA.FlowFindings) > 0 || len(turnA.AcceptedAggregateFacts) > 0 {
			return true
		}
		if strings.TrimSpace(turnA.AcceptedClosureReason) != "" {
			return true
		}
	}
	return false
}

func (o *Orchestrator) hasReusableTurnBSlateForFinalize() bool {
	if o == nil || o.busCtx == nil {
		return false
	}
	if len(o.busCtx.AnswerSymbols) > 0 || len(o.busCtx.AnswerChains) > 0 {
		return true
	}
	if o.busCtx.Mutable == nil {
		return false
	}
	if symbols, _ := o.busCtx.Mutable.EmittedAnswerSymbols(); len(symbols) > 0 {
		return true
	}
	if len(o.busCtx.Mutable.EmittedHypothesisVerdicts()) > 0 {
		return true
	}
	return false
}

func (o *Orchestrator) completeExtractAfterTransientProgress(err error) bool {
	if o == nil || o.busCtx == nil || !llm.IsStreamLevelRetryable(err) {
		return false
	}
	if !o.hasReusableTurnBSlateForFinalize() {
		return false
	}
	o.busCtx.TaskState.LastError = ""
	o.drainHypothesisVerdicts()
	logging.Warning("[orchestrator] suppressing extract transient retry after reusable Turn-B slate; proceeding with collected findings: %v", err)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeFinalizing,
		Reasoning:  softProceedWithExtractedFindingsMessage(o.busCtx.Language),
	})
	return true
}

func (o *Orchestrator) canUseFinalizerOutputAfterTransientProgress(output *agent.StageOutput, err error) bool {
	if o == nil || !llm.IsStreamLevelRetryable(err) || output == nil {
		return false
	}
	if output.AnswerDegraded || output.SkipAnswerChecks {
		return false
	}
	return strings.TrimSpace(output.FinalAnswer) != ""
}

func (o *Orchestrator) reusableTurnBSlateSummary() string {
	if o == nil || o.busCtx == nil {
		return "none"
	}
	mutableSymbols := 0
	verdicts := 0
	if o.busCtx.Mutable != nil {
		if symbols, _ := o.busCtx.Mutable.EmittedAnswerSymbols(); len(symbols) > 0 {
			mutableSymbols = len(symbols)
		}
		verdicts = len(o.busCtx.Mutable.EmittedHypothesisVerdicts())
	}
	return fmt.Sprintf("answer_symbols=%d emitted_answer_symbols=%d answer_chains=%d hypothesis_verdicts=%d",
		len(o.busCtx.AnswerSymbols), mutableSymbols, len(o.busCtx.AnswerChains), verdicts)
}

func (o *Orchestrator) hasBlockingReconcileRepair() bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	for _, r := range o.busCtx.Mutable.EvidenceClosure().ActiveRepairs() {
		if o.repairBlocksReconcileAutoComplete(r) {
			return true
		}
	}
	return false
}

func (o *Orchestrator) repairBlocksReconcileAutoComplete(r types.RepairDirective) bool {
	switch r.Kind {
	case types.RepairExpandSearch, types.RepairForceCompleteDowngrade:
		return false
	default:
		return o.repairBlocksAcceptedClosure(r)
	}
}

func (o *Orchestrator) repairBlocksAcceptedClosure(r types.RepairDirective) bool {
	if !types.RepairBlocksAcceptedClosure(r) {
		return false
	}
	if o.sourceInventoryClosureMakesSubjectRebindAdvisory(r) {
		logging.Info("[orchestrator] treating source-inventory subject rebind as advisory after exact-universe accepted closure: subject=%q origin=%q",
			r.Subject, r.Origin)
		return false
	}
	return true
}

func (o *Orchestrator) sourceInventoryClosureMakesSubjectRebindAdvisory(r types.RepairDirective) bool {
	if r.Kind != types.RepairRebindSubject {
		return false
	}
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.AnalysisIR == nil {
		return false
	}
	profile := o.busCtx.AnalysisIR.RequestModel.SourceInventoryProfile
	subject, ok := types.AnswerSubjectForSourceInventoryProfile(profile)
	if !ok || subject.Kind == types.SubjectUnknown {
		return false
	}
	if strings.TrimSpace(r.Subject) != string(subject.Kind) {
		return false
	}
	return tool.SourceInventoryAcceptedClosureCoversExactUniverse(
		o.busCtx,
		o.busCtx.Mutable.StableInvestigationAggregateFacts(),
	)
}

func (o *Orchestrator) acceptedSourceInventoryClosureSuppressesReconcileFactRetry(window []*types.TaskNode) bool {
	if !exploreWindowOnlyReconcile(window) {
		return false
	}
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	return tool.SourceInventoryAcceptedClosureCoversExactUniverse(
		o.busCtx,
		o.busCtx.Mutable.StableInvestigationAggregateFacts(),
	)
}

func exploreWindowOnlyReconcile(window []*types.TaskNode) bool {
	if len(window) == 0 {
		return false
	}
	for _, n := range window {
		if n == nil || n.Type != types.NodeReconcile {
			return false
		}
	}
	return true
}

func (o *Orchestrator) drainIgnorableReconcileRepairs() {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || o.hasBlockingReconcileRepair() {
		return
	}
	drained := o.busCtx.Mutable.EvidenceClosure().ConsumeRepairs()
	if len(drained) > 0 {
		logging.Info("[orchestrator] drained %d non-blocking reconcile repair(s) after auto-complete", len(drained))
	}
}

// emitAnalysisReady projects AnalysisIR.TaskGraph into the renderer-
// facing TaskNodeInfo list and fires EventAnalysisReady. Hidden
// nodes (counterfactual, probe) are filtered here so the renderer
// can show one row per user-visible task without re-implementing
// the filtering rules.
//
// Probe nodes are pre-scan placeholders the analyzer uses internally;
// they do not correspond to a piece of the user's question. Counter-
// factual nodes are speculative branches that may never actually run
// — surfacing them as task rows would mislead the user about what
// the pipeline is committed to investigating.
//
// Finalize is intentionally kept in the projection so the user sees
// the "synthesise answer" step at the bottom of the list and the
// row turns green when the answer is ready.
func (o *Orchestrator) emitAnalysisReady() {
	if o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	if tool.PublishSourceInventoryAdvisoryFromTypedRequest(o.busCtx) {
		logging.Info("[orchestrator] pre-explore source-inventory advisory published")
	}
	nodes := o.busCtx.AnalysisIR.TaskGraph.Nodes
	investigationPlan := types.CompileInvestigationPlan(o.busCtx.AnalysisIR.RequestModel, &o.busCtx.AnalysisIR.AnswerContract)
	presentationContract := types.CompileAnswerPresentationContract(o.busCtx.AnalysisIR, nil)
	o.busCtx.ExploreLanePlan = types.CompileExploreLanePlan(o.busCtx.AnalysisIR.RequestModel, &o.busCtx.AnalysisIR.AnswerContract, presentationContract)
	out := make([]render.TaskNodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.IsCounterfactual {
			continue
		}
		if n.Type == types.NodeProbe {
			continue
		}
		info := render.TaskNodeInfo{
			ID:        n.ID,
			Type:      string(n.Type),
			Objective: n.Objective,
		}
		if n.Type == types.NodeEvidence {
			if unit, ok := investigationPlan.InvestigationUnitForEvidenceNode(n.ID); ok {
				info.HasInvestigationUnit = true
				info.InvestigationUnit = unit
			}
		}
		out = append(out, info)
	}
	if len(out) == 0 {
		return
	}
	o.emit(render.Event{
		Kind:      render.EventAnalysisReady,
		Timestamp: time.Now(),
		TraceID:   o.busCtx.TraceID,
		TaskNodes: out,
	})
}

// emitNodeStart / emitNodeEnd are thin wrappers around o.emit that
// also look up the node's Type and Objective so the renderer never
// needs to cross-reference the AnalysisIR. Called from runTaskGraph
// at every state.markRunning / markDone / markFailed / requeue site
// so the renderer's row state stays in lockstep with the scheduler.
func (o *Orchestrator) emitNodeStart(id string) {
	if id == "" || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	n := findNode(o.busCtx.AnalysisIR.TaskGraph, id)
	if n == nil {
		return
	}
	o.emit(render.Event{
		Kind:          render.EventTaskNodeStart,
		Timestamp:     time.Now(),
		NodeID:        id,
		NodeKind:      string(n.Type),
		NodeObjective: n.Objective,
	})
}

func (o *Orchestrator) emitNodeEnd(id string, ok bool, errMsg string) {
	if id == "" || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	n := findNode(o.busCtx.AnalysisIR.TaskGraph, id)
	if n == nil {
		return
	}
	ev := render.Event{
		Kind:          render.EventTaskNodeEnd,
		Timestamp:     time.Now(),
		NodeID:        id,
		NodeKind:      string(n.Type),
		NodeObjective: n.Objective,
	}
	if !ok {
		ev.Error = errMsg
		if ev.Error == "" {
			ev.Error = "criteria not met"
		}
	} else if strings.HasPrefix(errMsg, "skipped") {
		// Success-path "skipped" signal: SkipOnFirstVisit (plan
		// loaded from disk) or --skip-verify short-circuit. The
		// renderer picks a "已加载" / "已跳过" phrase variant so the
		// dock's done line doesn't claim the LLM did work it didn't.
		ev.NodeSkipped = true
	}
	o.emit(ev)
}

func findNode(g types.TaskGraph, id string) *types.TaskNode {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// drainHypothesisVerdicts is the P2.1 Phase 10 hook invoked after a
// successful StageExtract dispatch. It reads the Turn B verdict
// buffer, applies MarkHypothesis for each entry, and LEAVES the
// buffer populated so the finalizer's prompt builder can render the
// rationale / citation text back to the user.
//
// Error handling policy:
//
//   - Unknown hypothesis id: log a warning and skip. The v3
//     schema-level emit_hypothesis_verdict tool already rejects
//     malformed calls at decode time, so reaching this path means
//     the LLM emitted a verdict for an id not in the hypothesis set
//     (hallucinated id or a typo). We never let a hallucinated id
//     corrupt the IR.
//
//   - Unknown status: same as above. MarkHypothesis validates the
//     enum and returns an error. Skip + warn.
//
//   - Nil AnalysisIR: the extractor dispatched without an analyzer
//     run (REPL bootstrap, unit tests). Skip the drain entirely;
//     the verdicts stay in the buffer but have no IR to write
//     through. This is the same fail-closed policy as Phase 11's
//     nil-Mutable check in the explorer.
//
// The function is a no-op when the buffer is empty, so it is always
// safe to call after any extract dispatch regardless of whether the
// LLM actually used emit_hypothesis_verdict.
func (o *Orchestrator) drainHypothesisVerdicts() {
	if o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	verdicts := o.busCtx.Mutable.EmittedHypothesisVerdicts()
	if len(verdicts) == 0 {
		return
	}
	applied := 0
	for _, v := range verdicts {
		if err := o.busCtx.AnalysisIR.MarkHypothesis(v.HypothesisID, v.Status); err != nil {
			logging.Warning("[orchestrator] hypothesis verdict drain: %v (rationale=%q citation=%q)",
				err, v.Rationale, v.Citation)
			continue
		}
		applied++
	}
	logging.Debug("[orchestrator] applied %d/%d hypothesis verdicts to IR; buffer retained for finalizer rendering",
		applied, len(verdicts))
}

// runAutoVerdicts evaluates criterion-based hypothesis auto-verdicts
// without dispatching the extractor LLM. Falsification conditions
// that are satisfied inject a "rejected" verdict; hypotheses whose
// RequiredEvidence is fully satisfied (but no LLM verdict exists)
// get an "inconclusive" verdict. This is the lightweight post-
// explore-window hook that replaced the per-window extract dispatch
// — the full LLM-backed extract runs once just before finalize.
func (o *Orchestrator) runAutoVerdicts() {
	if o.busCtx == nil || o.busCtx.AnalysisIR == nil || len(o.busCtx.AnalysisIR.HypothesisSet) == 0 {
		return
	}
	mu := o.busCtx.Mutable
	if mu == nil {
		return
	}
	if o.busCtx.AnalysisIR.RequestModel.HasObservationOnlyRuntimeArtifact() {
		logging.Debug("[orchestrator] auto-verdict skipped for observation-only runtime artifact")
		return
	}
	var taToolResults []types.ToolResult
	if ta := mu.TurnAArtifacts(); ta != nil {
		taToolResults = ta.ToolResults
	}
	env := criterion.Env{
		IR:             o.busCtx.AnalysisIR,
		Evidence:       o.busCtx.EvidenceItems,
		ToolResults:    taToolResults,
		AnswerSymbols:  o.busCtx.AnswerSymbols,
		AggregateFacts: mu.StableInvestigationAggregateFacts(),
		PrescanBlob:    mu.PrescanSummaryBlob(),
	}
	existing := mu.EmittedHypothesisVerdicts()
	byID := make(map[string]bool, len(existing))
	for _, v := range existing {
		byID[v.HypothesisID] = true
	}
	var injected []types.HypothesisVerdict
	for _, h := range o.busCtx.AnalysisIR.HypothesisSet {
		fals := criterion.Eval(h.FalsificationCondition, env)
		if fals.Satisfied {
			if byID[h.ID] {
				logging.Warning("[orchestrator] auto-verdict: falsification satisfied for %s: forcing rejected", h.ID)
			}
			injected = append(injected, types.HypothesisVerdict{
				HypothesisID: h.ID,
				Status:       types.HypRejected,
				Rationale:    "falsification condition satisfied: " + fals.Detail,
			})
			continue
		}
		if byID[h.ID] {
			continue
		}
		okReq, _ := criterion.EvalAll(h.RequiredEvidence, env)
		if okReq && len(h.RequiredEvidence) > 0 {
			injected = append(injected, types.HypothesisVerdict{
				HypothesisID: h.ID,
				Status:       types.HypInconclusive,
				Rationale:    "required evidence satisfied but no LLM verdict emitted",
			})
		}
	}
	if len(injected) > 0 {
		mu.AppendEmittedHypothesisVerdicts(injected)
		logging.Info("[orchestrator] injected %d auto-verdict(s) from criterion evaluation", len(injected))
	}
}

// injectInconclusiveForStuckHypotheses is the scheduler's escape hatch
// for hypothesis-validate loops that cannot make progress. It mirrors
// runAutoVerdicts' "inject inconclusive" path but IGNORES
// RequiredEvidence — it applies when the scheduler has detected that
// re-investigation would not advance the env (identical envShape across
// two consecutive SuccessCriteria evaluations) and therefore no amount
// of explore retries will resolve HypUnknown. Applied per validate
// node, so only hypotheses the stuck validate gate's SC references
// are affected; unrelated hypotheses stay untouched.
//
// The rationale is explicit about the give-up — downstream renderers
// surface it as a caveat so the user sees that the answer shipped
// with unresolved hypotheses. The scheduler logs a single
// "[scheduler/stuck]" line at INFO level so operators can grep a
// trace for the exact escape event.
//
// Returns the number of hypotheses newly marked inconclusive.
func (o *Orchestrator) injectInconclusiveForStuckHypotheses(stuckNodeID string) int {
	if o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return 0
	}
	mu := o.busCtx.Mutable
	if mu == nil {
		return 0
	}
	existing := mu.EmittedHypothesisVerdicts()
	byID := make(map[string]bool, len(existing))
	for _, v := range existing {
		byID[v.HypothesisID] = true
	}
	var injected []types.HypothesisVerdict
	for _, h := range o.busCtx.AnalysisIR.HypothesisSet {
		if h.Status != types.HypUnknown && h.Status != "" {
			continue
		}
		if byID[h.ID] {
			continue
		}
		injected = append(injected, types.HypothesisVerdict{
			HypothesisID: h.ID,
			Status:       types.HypInconclusive,
			Rationale: fmt.Sprintf(
				"re-investigation did not advance evidence "+
					"(stuck at stable env shape on validate node %s); "+
					"marking inconclusive to unblock finalize",
				stuckNodeID),
		})
	}
	if len(injected) == 0 {
		return 0
	}
	mu.AppendEmittedHypothesisVerdicts(injected)
	logging.Info("[scheduler/stuck] validate %s: injected %d inconclusive verdict(s) (evidence stable across retries)",
		stuckNodeID, len(injected))
	return len(injected)
}

func (o *Orchestrator) attachFirstDraftReference(out *agent.StageOutput, firstDraft string, concerns []types.Violation, rejectedForRewrite bool) {
	if o == nil || out == nil {
		return
	}
	// This is the only final-answer path that may show a full
	// "first draft" reference. It must be reserved for an actually
	// rejected draft that caused a later finalizer rewrite. Soft
	// concerns on an accepted first draft are surfaced as caveats or
	// notes, not by duplicating the answer under a comparison panel.
	if !rejectedForRewrite {
		return
	}
	firstDraft = strings.TrimSpace(firstDraft)
	finalAnswer := strings.TrimSpace(out.FinalAnswer)
	if firstDraft == "" || finalAnswer == "" || firstDraft == finalAnswer {
		return
	}
	lang := ""
	if o.busCtx != nil {
		lang = o.busCtx.Language
	}
	body := firstDraft
	if note := draftConcernSummary(lang, concerns, true); note != "" {
		body = note + "\n\n" + body
	}
	o.appendAnswerDisplayAttachment(out, types.AnswerDisplayAttachment{
		Kind:   types.AnswerDisplayAttachmentMarkdown,
		Title:  draftReferenceTitle(lang),
		Body:   body,
		Source: "orchestrator.first_finalize_draft",
	})
}

func (o *Orchestrator) attachDraftReviewNote(out *agent.StageOutput, title string, concerns []types.Violation) {
	if o == nil || out == nil || len(concerns) == 0 {
		return
	}
	lang := ""
	if o.busCtx != nil {
		lang = o.busCtx.Language
	}
	body := draftConcernSummary(lang, concerns, false)
	if body == "" {
		return
	}
	if strings.TrimSpace(title) == "" {
		title = draftReviewNoteTitle(lang)
	}
	o.appendAnswerDisplayAttachment(out, types.AnswerDisplayAttachment{
		Kind:   types.AnswerDisplayAttachmentMarkdown,
		Title:  title,
		Body:   body,
		Source: "orchestrator.strict_review_disabled",
	})
}

func (o *Orchestrator) appendAnswerDisplayAttachment(out *agent.StageOutput, att types.AnswerDisplayAttachment) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || out == nil {
		return
	}
	if strings.TrimSpace(att.Body) == "" {
		return
	}
	attachments := o.busCtx.Mutable.AnswerDisplayAttachments()
	attachments = append(attachments, att)
	o.busCtx.Mutable.SetAnswerDisplayAttachments(attachments)
	if doc := o.busCtx.Mutable.AnswerDocumentV2(); doc != nil {
		out.FinalAnswer = render.RenderAnswerDocumentWithAttachments(
			doc,
			o.busCtx.Mutable.AnswerDisplayAttachments(),
			o.busCtx.Language,
		)
		return
	}
	out.FinalAnswer = strings.TrimRight(out.FinalAnswer, "\n") + "\n\n---\n\n**" +
		strings.TrimSpace(att.Title) + "**\n\n" + strings.TrimSpace(att.Body) + "\n\n---\n"
}

func draftReferenceTitle(lang string) string {
	if lang == "zh" {
		return "第一稿答案（校验前参考）"
	}
	return "First Draft Answer (Pre-review Reference)"
}

func draftReviewNoteTitle(lang string) string {
	if lang == "zh" {
		return "第一稿校验提示"
	}
	return "First Draft Review Notes"
}

func strictReviewDisabledTitle(lang string) string {
	if lang == "zh" {
		return "第一稿答案：强校验已关闭"
	}
	return "First Draft Answer: Strict Review Disabled"
}

func draftConcernSummary(lang string, concerns []types.Violation, rewritten bool) string {
	count := 0
	for _, v := range concerns {
		if v.IsDerived {
			continue
		}
		count++
	}
	if count == 0 {
		return ""
	}
	if lang == "zh" {
		if rewritten {
			return fmt.Sprintf("系统在强校验中发现第一稿仍有 %d 项待补充，所以下方最终稿可能已经调整；这里保留第一稿，便于对照。", count)
		}
		return fmt.Sprintf("强校验模式已关闭。系统检测到这份第一稿仍有 %d 项可能待补充，本次不会触发 reviewer 或重写。", count)
	}
	if rewritten {
		return fmt.Sprintf("Strict review found %d remaining concern(s) in the first draft, so the final answer may have changed; the draft is preserved here for comparison.", count)
	}
	return fmt.Sprintf("Strict review mode is disabled. The first draft still has %d potential concern(s), but this run will not invoke reviewers or rewrite it.", count)
}

// recordTaskFinalize copies the finalizer's FinalAnswer into
// Mutable.result and emits the objective-done event. Empty answers
// are still recorded — callers downstream (render layer) treat an
// empty result as "no answer" and display the fail state instead.
func (o *Orchestrator) recordTaskFinalize(out *agent.StageOutput) {
	answer := ""
	if out != nil {
		answer = out.FinalAnswer
	}
	o.busCtx.Mutable.SetResult(answer)
	// INFO (post-2026-04-30): default log level captures the agent-
	// emitted raw markdown so post-run audit can find the answer
	// without enabling DEBUG. Pre-fix this was DEBUG, which meant
	// the only INFO-level record of the final answer was the REPL/
	// single-shot dispatch's own log line; if those changed shape
	// or got truncated, the orchestrator-level record was silently
	// invisible. Promotion is cheap (final answer is one log entry
	// per Run, ≤ 30 KB typical, no rotation impact).
	logging.Info("[orchestrator] final answer (len=%d):\n%s\n---", len(answer), answer)

	// Final-answer transcript dump. Persist every non-empty final
	// answer, including best-effort fallback answers that never landed
	// an AnswerDocumentV2. The dump is the raw markdown answer body
	// that feeds REPL rendering, not ANSI/border terminal chrome.
	// Best-effort: the helper logs and swallows every IO error so the
	// dump never affects the rest of the pipeline.
	if o.outputDumpDir != "" && strings.TrimSpace(answer) != "" {
		if result := writeFinalOutputDumpResult(dumpFinalOutputArgs{
			dir:      o.outputDumpDir,
			max:      o.outputDumpMax,
			request:  o.outputTranscriptRequestForDump(),
			answer:   answer,
			hasLog:   o.attachedLog != "",
			logBytes: len(o.attachedLog),
			hasTrace: o.attachedHitrace != "",
			traceB:   len(o.attachedHitrace),
			now:      time.Now(),
			pid:      os.Getpid(),
		}); result.MarkdownPath != "" {
			o.busCtx.Mutable.SetFinalAnswerOutputPaths(result.MarkdownPath, result.HTMLPath)
		}
	}

	o.emit(render.Event{
		Kind:      render.EventObjectiveDone,
		Timestamp: time.Now(),
		Objective: o.busCtx.Mutable.Objective(),
	})
}

func (o *Orchestrator) outputTranscriptRequestForDump() string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		if o != nil && strings.TrimSpace(o.outputTranscriptRequest) != "" {
			return o.outputTranscriptRequest
		}
		return ""
	}
	if request := strings.TrimSpace(o.busCtx.Mutable.OutputTranscriptRequest()); request != "" {
		return request
	}
	return types.StripConversationPrefix(o.busCtx.Mutable.Objective())
}

// dispatchStage runs the agent bound to the given stage and returns
// the StageOutput it produced. The output has already been routed
// through applyStageOutput by the time this function returns, so
// callers don't need to apply it again — they can just inspect
// fields like FinalAnswer that are useful for per-stage reactions
// (runTaskGraph uses this to write the finalizer's answer onto the
// task's Result).
func (o *Orchestrator) dispatchStage(stage types.PipelineStage) (*agent.StageOutput, error) {
	// Cancel checkpoint: between two stage dispatches the user's Ctrl+C
	// / `/cancel` lands here first. We bail out before touching the
	// agent registry / skill registry / per-stage hooks so partial
	// state changes are minimised. Stage-aware so the user-facing
	// "canceled at stage=explore" message has the right label.
	if err := o.checkCanceled(string(stage), 0); err != nil {
		return nil, err
	}
	info, ok := pipelineTopology[stage]
	if !ok {
		return nil, fmt.Errorf("unknown pipeline stage: %s", stage)
	}
	agentName := info.Agent
	skillName := info.Skill

	ag, err := o.agents.Get(agentName)
	if err != nil {
		return nil, fmt.Errorf("get agent %s: %w", agentName, err)
	}

	sk, err := o.skills.Get(skillName)
	if err != nil {
		return nil, fmt.Errorf("get skill %s: %w", skillName, err)
	}

	o.busCtx.ActiveAgent = agentName
	// Clear the run-level error surface at the start of each new stage
	// attempt. Retry loops and advisory pre-stages are allowed to
	// proceed after a failed dispatch; if a later attempt succeeds, the
	// stale LastError must not poison phase gating or the final
	// PipelineEnd surface.
	if o.busCtx != nil {
		o.busCtx.TaskState.LastError = ""
	}
	preflightStart := time.Now()
	ctxBuildStart := time.Now()
	logging.Debug("[orchestrator] preflight: stage=%s agent=%s phase=build_agent_context start", stage, agentName)
	stopLocal := o.startSchedulerLocalWork(stage, "build_agent_context")
	agentCtx := ctxbuilder.BuildAgentContext(o.busCtx, agentName, stage)
	if stopLocal != nil {
		stopLocal()
	}
	logging.Debug("[orchestrator] preflight: stage=%s agent=%s phase=build_agent_context done elapsed=%s evidence=%d reports=%d symbols=%d",
		stage, agentName, time.Since(ctxBuildStart).Round(time.Millisecond),
		len(agentCtx.EvidenceItems), len(agentCtx.PriorReports), len(agentCtx.AnswerSymbols))
	if err := o.checkCanceled(string(stage), 0); err != nil {
		return nil, err
	}
	if ta, ok := o.thinkAloudMap[agentName]; ok {
		agentCtx.ThinkAloud = ta
	}
	// Consume + clear the per-dispatch emit-retry counter set by retry-
	// aware phase loops (e.g., runAnalyzePhase). Non-retry callers
	// always see 0; the agent layer uses attempt > 0 to escalate the
	// prompt + tool_choice shape (terminal forcing for emit-required
	// stages). Cleared after read so a single retry counter cannot
	// leak across stages.
	agentCtx.EmitStageRetryAttempt = o.emitStageRetryAttempt
	o.emitStageRetryAttempt = 0

	// Per-dispatch marker — gives operators a precise dispatch count
	// distinct from the per-iteration "ASSISTANT content_len=" lines.
	// Operators previously misread `explorer_iters=40` as a single
	// long-running ReAct loop when it was actually 4 dispatches × ~10
	// iters/dispatch; eval/run.sh's <agent>_dispatches metric counts
	// this line.
	logging.Debug("[diag %s] DISPATCH stage=%s attempt=%d",
		agentName, stage, agentCtx.EmitStageRetryAttempt)

	// B6-F4 (post-shape consolidated audit, 2026-05-04): dynamically
	// disable think_aloud on FINALIZER retry attempts. Iteration 0
	// (the first dispatch) keeps think_aloud per the operator's yaml
	// configuration so the LLM's first-pass reasoning chain is
	// auditable; iteration ≥1 (retries triggered by contract.Check
	// failure) drops the chain to (a) shave ~30-50% LLM tokens off
	// the retry, and (b) prevent the LLM from re-using its first-
	// pass reasoning as gospel — the retry should respond to the
	// violation Detail / Repair, not re-prosecute the original line
	// of thought. Gated by pipeline_finalizer_retry_no_think
	// (default true). Non-finalizer agents and iter==0 finalizer
	// dispatches are unaffected.
	if finalizerRetryNoThinkEnabled() &&
		agentName == types.AgentFinalizer &&
		agentCtx.EmitStageRetryAttempt > 0 {
		agentCtx.ThinkAloud = false
	}

	// Prior Conversation visibility. The Objective always carries the
	// full prior+current payload so StripConversationPrefix /
	// SplitConversation keep working; this flag gates whether the
	// prompt builder renders the user-facing Prior Conversation
	// section. See types.AgentSettings.PriorConvPolicy for rationale.
	priorVisible := o.orchestratorPriorConvVisibleForStage(
		o.settings.Agent.PriorConvPolicy, stage, agentCtx.Objective)
	agentCtx.PriorConvHidden = !priorVisible
	// Only log when a prior block actually exists — otherwise the
	// flag is moot and the line is noise in single-shot traces.
	if prior, _ := types.SplitConversation(agentCtx.Objective); prior != "" {
		logging.Debug("[orchestrator] prior_conv: stage=%s policy=%s visible=%t",
			stage, o.settings.Agent.PriorConvPolicy, priorVisible)
	}

	// Per-dispatch iteration budget scaling. Single source of truth
	// for "how many iterations does THIS dispatch get" — driven by
	// analyzer-supplied complexity signals (sub-topic count, complexity
	// classification). Each agent has its own per-dispatch channel so
	// outer-loop and inner-evaluator budgets stay decoupled:
	//
	//   - explorer: AgentContext.MaxIterOverride — outer ReAct loop
	//     ceiling (BaseAgent.Execute's for-loop bound).
	//   - planner: AgentContext.PlannerSoftIterCapOverride — inner
	//     two-stage soft cap (recovery slack added by the evaluator
	//     on top). Outer loop stays at the agent-settings default
	//     (currently 20) so the soft→hard recovery window has room
	//     to run. Conflating these into one field would force the
	//     outer loop to terminate at the inner soft cap, eliminating
	//     the recovery window the inner pair was designed to provide.
	//
	// The architectural rule: agent iteration budgets are NEVER
	// hardcoded at construction. The orchestrator owns per-dispatch
	// budget because it is the only layer that has all the relevant
	// inputs (AnalysisIR + AgentSettings + stage). Hardcoded inner
	// caps that ignore the analyzer's complexity signal are the
	// anti-pattern this block exists to prevent.
	// Commit 51 Gap 3: read-mode Answer Taxonomy injection. The
	// analyzer runs BEFORE AnalysisIR is set, so we cannot filter
	// by Scenario/Family on the first dispatch. RelevantTo("","",5)
	// returns only unconstrained patterns (patterns whose author
	// declared no AppliesToScenarios / AppliesToFamilies), which
	// is exactly the "broadly applicable in this repo" set we want
	// for analyze entry. On post-analyze stages the IR-aware
	// switch below has the Scenario/Family and could re-inject
	// with proper filters; we keep it analyze-only for now to
	// match the "guide classification" purpose.
	if o.answerTaxonomyStore != nil && stage == types.StageAnalyze {
		scenario := ""
		family := ""
		if o.busCtx.AnalysisIR != nil {
			scenario = string(o.busCtx.AnalysisIR.RequestModel.Scenario)
			family = string(types.ResolveQuestionFamily(o.busCtx.AnalysisIR.RequestModel))
		}
		if pitfalls := o.answerTaxonomyStore.RelevantTo(scenario, family, 5); len(pitfalls) > 0 {
			agentCtx.ActiveAnswerPitfalls = pitfalls
			logging.Debug("[orchestrator] answer_taxonomy: %d active pitfalls injected at analyze entry", len(pitfalls))
		}
	}

	if o.busCtx.AnalysisIR != nil {
		nSub := len(o.busCtx.AnalysisIR.RequestModel.SubTopics)
		complexity := o.busCtx.AnalysisIR.RequestModel.Complexity
		agentCfg := o.settings.Agent
		switch stage {
		case types.StageExplore:
			if base, adjusted, reason, ok := applyExploreIterationScalingForRequest(agentCtx, o.busCtx.AnalysisIR.RequestModel, agentCfg); ok {
				logging.Debug("[orchestrator] explorer scaling: reason=%s sub-topics=%d iterations %d → %d",
					reason, nSub, base, adjusted)
			}
		case types.StagePlan:
			// Planner soft-cap scaling. The default soft cap (6) is
			// calibrated for ComplexitySimple single-topic plans
			// (e.g. "fix this typo"). Real feature-add tasks are
			// typically ComplexityModerate or higher with 2-3
			// sub-topics, and the planner skill's Workflow step 1
			// ("read and understand the current shape") is open-
			// ended — it cannot be done in 6 iterations across a
			// 3-sub-topic surface. Two orthogonal uplifts:
			//
			//   - SubTopicPlannerBudgetExtra (yaml:
			//     agent_subtopic_planner_extra, default 3) per
			//     sub-topic, applied when nSubTopics > 1.
			//   - PlannerComplexityBudgetExtra (yaml:
			//     agent_planner_complexity_extra, default 2) per
			//     complexity level (Simple=0, Moderate=1, Complex=2)
			//     so single-topic-but-subtle tasks still get
			//     headroom.
			//
			// Hard cap at plannerScaledIterMax (20) bounds worst-
			// case cost on a runaway dispatch — the planner is
			// single-emit so legitimate completion never needs
			// that many iterations.
			base := agentCfg.PlannerSoftIterCap
			extra := 0
			if nSub > 1 {
				extra += nSub * agentCfg.SubTopicPlannerBudgetExtra
			}
			switch complexity {
			case types.ComplexityModerate:
				extra += agentCfg.PlannerComplexityBudgetExtra
			case types.ComplexityComplex:
				extra += 2 * agentCfg.PlannerComplexityBudgetExtra
			}
			adjusted := base + extra
			ceil := agentCfg.PlannerScaledIterMax
			if ceil <= 0 {
				ceil = 20
			}
			if adjusted > ceil {
				adjusted = ceil
			}
			if adjusted > base {
				agentCtx.PlannerSoftIterCapOverride = adjusted
				logging.Debug("[orchestrator] planner scaling: complexity=%s sub-topics=%d, soft cap %d → %d",
					complexity, nSub, base, adjusted)
			}
			// Stage 3: inject relevant Failure Taxonomy entries
			// the planner should regard before emitting. The
			// relevance score combines task kind + target-paths
			// overlap + recency; we read RelevantTo with the
			// current task's kind + WriteAnalysisIR scope
			// anchors as the path hint set.
			//
			// Multi-phase carve-out (commit 40 P2): when the
			// orchestrator is mid-phase, also pass the phase
			// goal as a keyword-boost hint so phase 2's planner
			// sees phase 2's relevant pitfalls higher than
			// phase 1's. Without this, every phase in a group
			// shares the same task kind + scope anchors and
			// pitfalls land identically ranked.
			if o.failureTaxonomyStore != nil {
				kind := ""
				var paths []string
				if ir := o.busCtx.Mutable.WriteAnalysisIR(); ir != nil {
					kind = string(ir.Request.Task.Kind)
					for _, p := range ir.Request.ScopeAnchors {
						if t := strings.TrimSpace(p); t != "" {
							paths = append(paths, t)
						}
					}
				}
				var keywords []string
				if o.phaseContextPrefix != "" {
					// Strip the "## Phase X of Y: " prefix; the
					// remainder is the phase goal text we want
					// for keyword overlap.
					if goal := extractPhaseGoalFromPrefix(o.phaseContextPrefix); goal != "" {
						keywords = []string{goal}
					}
				}
				if pitfalls := o.failureTaxonomyStore.RelevantToWithKeywords(kind, paths, keywords, 5); len(pitfalls) > 0 {
					agentCtx.ActivePitfalls = pitfalls
					logging.Debug("[orchestrator] failure_taxonomy: %d active pitfalls injected", len(pitfalls))
				}
			}
		case types.StageExtract:
			// Extractor soft-cap scaling. The default soft cap (3) is
			// calibrated for single-topic answers where one or two
			// emit_answer_symbol calls suffice. Multi-topic explanation
			// answers require one Key-Anchor row per sub-topic
			// (5a356ec); a static 3 starves at 4+ sub-topics. Mirrors
			// the planner's two-axis scaling: per-sub-topic uplift
			// (SubTopicExtractorBudgetExtra default 1) plus complexity
			// uplift (ExtractorComplexityBudgetExtra: Simple=0×,
			// Moderate=1×, Complex=2×). Hard cap at
			// ExtractorScaledIterMax (default 8).
			base := agentCfg.ExtractorSoftIterCap
			extra := 0
			if nSub > 1 {
				extra += nSub * agentCfg.SubTopicExtractorBudgetExtra
			}
			switch complexity {
			case types.ComplexityModerate:
				extra += agentCfg.ExtractorComplexityBudgetExtra
			case types.ComplexityComplex:
				extra += 2 * agentCfg.ExtractorComplexityBudgetExtra
			}
			adjusted := base + extra
			ceil := agentCfg.ExtractorScaledIterMax
			if ceil <= 0 {
				ceil = 8
			}
			if adjusted > ceil {
				adjusted = ceil
			}
			if adjusted > base {
				agentCtx.ExtractorSoftIterCapOverride = adjusted
				logging.Debug("[orchestrator] extractor scaling: complexity=%s sub-topics=%d, soft cap %d → %d",
					complexity, nSub, base, adjusted)
			}
		case types.StageVerify:
			// Verifier soft-cap scaling. Driven by ChangePlan target-path
			// count rather than sub-topic count: a multi-language
			// monorepo plan with N target paths may need N runner
			// invocations (each an iteration of run_tests). Mirrors
			// CoderSoftIterSlack but lifted from the static 5 default
			// when an installed plan justifies it. Hard cap at
			// VerifierScaledIterMax (default 12).
			if o.busCtx.Mutable != nil {
				if plan := o.busCtx.Mutable.ChangePlan(); plan != nil && len(plan.TargetPaths) > 0 {
					base := agentCfg.VerifierSoftIterCap
					extra := len(plan.TargetPaths) * agentCfg.TargetPathsVerifierBudgetExtra
					adjusted := base + extra
					ceil := agentCfg.VerifierScaledIterMax
					if ceil <= 0 {
						ceil = 12
					}
					if adjusted > ceil {
						adjusted = ceil
					}
					if adjusted > base {
						agentCtx.VerifierSoftIterCapOverride = adjusted
						logging.Debug("[orchestrator] verifier scaling: target_paths=%d, soft cap %d → %d",
							len(plan.TargetPaths), base, adjusted)
					}
				}
			}
		}
	}

	logging.Info("[orchestrator] dispatching agent=%s skill=%s", agentName, skillName)
	logging.Debug("[orchestrator] preflight: stage=%s agent=%s ready elapsed=%s", stage, agentName, time.Since(preflightStart).Round(time.Millisecond))
	if err := o.checkCanceled(string(stage), 0); err != nil {
		return nil, err
	}

	stageStart := time.Now()
	o.emit(render.Event{
		Kind:      render.EventStageStart,
		Timestamp: stageStart,
		Stage:     stage,
		Agent:     agentName,
		Skill:     skillName,
	})
	o.emit(render.Event{
		Kind:      render.EventSkillBound,
		Timestamp: stageStart,
		Stage:     stage,
		Agent:     agentName,
		Skill:     skillName,
	})

	output, err := ag.Execute(agentCtx, sk)
	if err != nil {
		// Preserve any structured side effects the agent managed to
		// salvage before the transient failure surfaced. BaseAgent
		// returns a non-nil StageOutput on stream stalls / first-byte
		// timeouts after tool progress; dropping it here erases accepted
		// evidence, Turn-A handoff data, tool results, and repair
		// directives, causing downstream stages to see a structurally
		// empty investigation even though the tool layer had already
		// accepted facts. The error still propagates so retry policy and
		// user-facing notices keep their existing semantics.
		if output != nil {
			o.applyStageOutput(output)
		}
		o.emit(render.Event{
			Kind:      render.EventStageEnd,
			Timestamp: time.Now(),
			Stage:     stage,
			Agent:     agentName,
			Error:     err.Error(),
		})
		// Return BOTH the structured output AND the error. The
		// scheduler distinguishes "agent runtime crashed (output ==
		// nil)" from "agent returned a structured failure (output !=
		// nil)" to drive the verify→plan retry loop. Discarding the
		// output here was the bug that made retry budget effectively
		// dead code: even when the verifier produced a meaningful
		// StageOutput (test counts + failure summary + ChangeReport
		// installed on Mutable), scheduler saw nil and bailed out.
		// Preserving the output lets the post-hook persist
		// report.json and the SC-fail branch fire the retry path.
		return output, fmt.Errorf("agent %s execution: %w", agentName, err)
	}

	// SubAgent decomposition path: replace the original output with
	// the merged sub-agent output for the rest of the pipeline.
	if proposal := extractSubAgentProposal(output, agentName); proposal != nil {
		logging.Info("[orchestrator] sub-agent proposal: %s (%d sub_tasks)", proposal.Reason, len(proposal.SubTasks))

		subTitle := ""
		if len(proposal.SubTasks) > 0 {
			subTitle = proposal.SubTasks[0].Title
		}
		o.emit(render.Event{
			Kind:         render.EventSubAgentStart,
			Timestamp:    time.Now(),
			Stage:        stage,
			SubAgentName: string(agentName),
			SubAgentID:   o.busCtx.TraceID + "-subagent",
			SubTaskTitle: subTitle,
			SubTaskCount: len(proposal.SubTasks),
		})

		merged, runErr := o.subRuntime.Run(o.busCtx, proposal)

		subErr := ""
		subTools := 0
		subFacts := 0
		if runErr != nil {
			logging.Error("[orchestrator] sub-agent run failed: %v, using original output", runErr)
			subErr = runErr.Error()
		} else {
			subTools = len(merged.ToolResults)
			subFacts = len(merged.NewFacts)
			output = merged
		}

		o.emit(render.Event{
			Kind:          render.EventSubAgentEnd,
			Timestamp:     time.Now(),
			Stage:         stage,
			SubAgentName:  string(agentName),
			SubAgentID:    o.busCtx.TraceID + "-subagent",
			ToolCallCount: subTools,
			FactCount:     subFacts,
			Error:         subErr,
		})
	}

	if stage == types.StageAnalyze {
		o.autoCorrectAnalyzerStageOutput(output, o.emitStageRetryAttempt)
	}

	o.applyStageOutput(output)
	o.busCtx.TaskState.Completed = append(o.busCtx.TaskState.Completed, string(stage))

	stageErr := ""
	if output.Error != "" {
		stageErr = output.Error
	}
	o.emit(render.Event{
		Kind:      render.EventStageEnd,
		Timestamp: time.Now(),
		Stage:     stage,
		Agent:     agentName,
		Error:     stageErr,
	})

	return output, nil
}

// applyStageOutput updates BusContext with the results from an agent execution.
func (o *Orchestrator) applyStageOutput(output *agent.StageOutput) {
	if output == nil {
		return
	}

	// Append tool results
	o.busCtx.ToolResults = append(o.busCtx.ToolResults, output.ToolResults...)

	// Append MCP responses
	o.busCtx.MCPResponses = append(o.busCtx.MCPResponses, output.MCPResponses...)

	// Append new facts
	o.busCtx.RepoFacts = append(o.busCtx.RepoFacts, output.NewFacts...)

	// Merge-deduplicate structured evidence, dataflow findings, and
	// answer chains/symbols. These four slices are "truth sets" that
	// downstream prompt builders render verbatim; without dedup a
	// stage self-loop (explore → explore) would accumulate the same
	// items N times, because the explorer's ParseOutput re-emits the
	// full snapshot of its cumulative investigation on every entry.
	// See memory/project_applystage_dedup.md for the full rationale
	// and the stability lock tests in
	// internal/orchestrator/apply_stage_output_dedup_test.go.
	//
	// Tool results, MCP responses, and repo facts are LEFT appending
	// because they are per-call history logs, not dedupable truth
	// items — each entry corresponds to a distinct tool invocation
	// and the downstream consumers (e.g. ReAct history pruning,
	// debug logs) rely on that per-call granularity.
	var evidenceChanged, findingsChanged bool
	o.busCtx.EvidenceItems, evidenceChanged = agent.MergeEvidenceItemsIfChanged(o.busCtx.EvidenceItems, output.EvidenceItems)
	o.busCtx.FlowFindings, findingsChanged = agent.MergeFlowFindingsIfChanged(o.busCtx.FlowFindings, output.FlowFindings)
	if len(output.EvidenceItems) > 0 || len(output.FlowFindings) > 0 {
		logging.Debug("[orchestrator] applyStageOutput truth merge: evidence_in=%d evidence_total=%d changed=%t flow_in=%d flow_total=%d changed=%t",
			len(output.EvidenceItems), len(o.busCtx.EvidenceItems), evidenceChanged,
			len(output.FlowFindings), len(o.busCtx.FlowFindings), findingsChanged)
	}
	o.busCtx.AnswerChains = types.MergeAnswerChains(o.busCtx.AnswerChains, output.AnswerChains)
	o.busCtx.AnswerSymbols = types.MergeAnswerSymbols(o.busCtx.AnswerSymbols, output.AnswerSymbols)
	if evidenceChanged || findingsChanged || len(output.AnswerChains) > 0 || len(output.AnswerSymbols) > 0 {
		o.busCtx.InvalidateAnswerPlanCache()
	}

	// P2.1 AnswerSymbolCompleteness — last non-empty writer wins. The
	// zero value (CompletenessUnknown) means "no claim attached" and
	// must not overwrite a previously-written complete/lower_bound. On
	// an explorer→extractor hand-off the extractor's claim is always
	// more authoritative because it has seen Turn A's TerminalEvidenceCount
	// plus the emit_answer_symbol LLM claim; the "last writer wins"
	// rule reflects that ordering without encoding stage names. Invalid
	// values (should be impossible under the schema validator) are
	// silently dropped so a malformed stage output cannot corrupt the
	// BusContext field.
	if output.AnswerSymbolCompleteness != types.CompletenessUnknown && output.AnswerSymbolCompleteness.IsValid() {
		o.busCtx.AnswerSymbolCompleteness = output.AnswerSymbolCompleteness
		o.busCtx.InvalidateAnswerPlanCache()
	}

	// Append the stage's synthesized narrative so downstream stages
	// can read prior reasoning. The active agent/stage at this point
	// is whatever just executed.
	if strings.TrimSpace(output.StageReport) != "" {
		o.busCtx.StageReports = append(o.busCtx.StageReports, types.StageReport{
			Stage:    o.busCtx.PipelineStage,
			Agent:    o.busCtx.ActiveAgent,
			Findings: output.StageReport,
		})
	}

	// Preserve reducer-produced advisory investigation notes in the existing
	// Turn A handoff. These notes are model-authored context only: they are
	// not citations, do not satisfy evidence gates, and are rendered downstream
	// through TurnAArtifacts' bounded advisory channel.
	if len(output.InvestigationNotes) > 0 && o.busCtx.Mutable != nil {
		artifacts := o.busCtx.Mutable.TurnAArtifacts()
		if artifacts == nil {
			artifacts = &types.TurnAArtifacts{}
		}
		artifacts.InvestigationNotes = appendUniqueInvestigationNotes(
			artifacts.InvestigationNotes,
			output.InvestigationNotes,
		)
		o.busCtx.Mutable.SetTurnAArtifacts(*artifacts)
	}

	// Carry the agent's own retry diagnosis through to the next
	// dispatch. CGEC B3: only overwrite when the stage produced a
	// non-empty hint of its own. An empty output.RetryHint leaves
	// the orchestrator-written window hint (from renderWindowHint)
	// in place so the View Reconcile / Subject Constraint /
	// Forced Read List sections persist through explore → extract →
	// finalize within the same retry round. The hint is reset at
	// the start of the NEXT window via applyWindowHint.
	if output.RetryHint != "" {
		o.busCtx.TaskState.RetryHint = output.RetryHint
		// Surface a soft, localized retry cue so the user sees the
		// pipeline is still working — NOT a verbatim dump of the
		// LLM-directed RetryHint body, which leaks internal
		// terminology (## Evidence Gaps / [MISSING] / (entities: …))
		// users cannot interpret. The full body stays in the debug
		// log via `[<agent>] retry hint built key=…` for operators.
		o.emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      "orchestrator",
			NoticeKind: render.NoticeRetry,
			Reasoning:  softAgentOutputRetryMessage(o.busCtx, o.busCtx.Language, o.busCtx.PipelineStage, output.MissingPiece),
		})
	}

	// Store the Analyzer v3 structured output on the first non-nil
	// value and never overwrite it. Subsequent re-dispatches of
	// analyze (rare but possible under retry budget) do not mutate
	// the IR in place.
	if output.AnalysisIR != nil && o.busCtx.AnalysisIR == nil {
		o.busCtx.AnalysisIR = output.AnalysisIR
		o.busCtx.InvalidateAnswerPlanCache()
	}

	// Update signals — only HasEnoughFacts survives after the
	// write-pipeline deletion.
	if output.SignalUpdates != nil && output.SignalUpdates.HasEnoughFacts {
		o.busCtx.Signals.HasEnoughFacts = true
	}

	// FinalAnswer is not captured here. runTaskGraph reads it
	// directly from the StageOutput returned by dispatchStage and
	// writes it onto the task's Result field via
	// Mutable.UpdateTaskResult.

	// Drain CGEC RepairDirectives into the per-Run EvidenceClosure.
	// Each enforcer (citation grounder, pre-complete check, stall
	// detector) attaches its repairs to the StageOutput; the closure
	// queues them until the next renderWindowHint pass consumes
	// them. De-dup is enforced inside AddRepair.
	if len(output.Repairs) > 0 && o.busCtx.Mutable != nil {
		closure := o.busCtx.Mutable.EvidenceClosure()
		for _, r := range output.Repairs {
			// AddRepair bumps stats.RepairsRaised internally so we
			// never double-count (the per-tool side channel
			// emit_answer_document.Execute writes via AddRepair too).
			closure.AddRepair(r)
		}
	}

	// Update missing piece
	o.busCtx.TaskState.Missing = output.MissingPiece

	// Record error if any
	if output.Error != "" {
		o.busCtx.TaskState.LastError = output.Error
	}
}

func appendUniqueInvestigationNotes(existing, incoming []string) []string {
	if len(incoming) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	for _, note := range existing {
		key := strings.TrimSpace(note)
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	out := append([]string(nil), existing...)
	for _, note := range incoming {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		if _, ok := seen[note]; ok {
			continue
		}
		seen[note] = struct{}{}
		out = append(out, note)
	}
	return out
}

// BusContext returns the current bus context (for inspection/testing).
func (o *Orchestrator) BusContext() *types.BusContext {
	return o.busCtx
}

// extractSubAgentProposal scans tool results for a propose_sub_agents call
// and parses the proposal. Each sub_task is routed to a SubAgent of the same
// name as the calling Agent, so sub_agent is filled in from agentName here
// (the LLM-visible schema omits this field entirely).
func extractSubAgentProposal(output *agent.StageOutput, agentName types.AgentName) *types.SubAgentProposal {
	if output == nil {
		return nil
	}
	for _, r := range output.ToolResults {
		if r.ToolName == "propose_sub_agents" && r.Success {
			var proposal types.SubAgentProposal
			if err := json.Unmarshal([]byte(r.Summary), &proposal); err != nil {
				continue
			}
			if len(proposal.SubTasks) == 0 {
				continue
			}
			for i := range proposal.SubTasks {
				proposal.SubTasks[i].SubAgent = string(agentName)
			}
			return &proposal
		}
	}
	return nil
}

// dominantViolationKind returns the most common ViolationKind in
// the failed contract result. When violations are distributed
// evenly, returns the first violation's Kind (stable ordering). An
// empty result returns "" so the caller can treat it as "no
// per-kind budget applicable".
//
// Session 11 C6 — used by the retry-budget gate to pick which
// kind's cap to consult. The dominance rule keeps the gate
// sticky: a run that keeps producing shape_swap events stays on
// the shape-violation budget even when occasional citation
// violations sneak in.
func dominantViolationKind(res contract.Result) types.ViolationKind {
	if res.Passed || len(res.Violations) == 0 {
		return ""
	}
	violations := FilterFinalizerRetryRootViolations(res.Violations)
	if len(violations) == 0 {
		return ""
	}
	counts := make(map[types.ViolationKind]int)
	for _, v := range violations {
		counts[v.Kind]++
	}
	var (
		bestKind  types.ViolationKind
		bestCount int
	)
	for _, v := range violations {
		if counts[v.Kind] > bestCount {
			bestCount = counts[v.Kind]
			bestKind = v.Kind
		}
	}
	return bestKind
}

func sameErrorClassRetryCap() int { return 1 }

func dominantViolationClass(res contract.Result) string {
	if res.Passed || len(res.Violations) == 0 {
		return ""
	}
	roots := FilterFinalizerRetryRootViolations(res.Violations)
	if len(roots) == 0 {
		return ""
	}
	counts := make(map[string]int, len(roots))
	for _, v := range roots {
		class := violationRetryClass(v)
		if class == "" {
			continue
		}
		counts[class]++
	}
	var (
		bestClass string
		bestCount int
	)
	for _, v := range roots {
		class := violationRetryClass(v)
		if class == "" {
			continue
		}
		if counts[class] > bestCount {
			bestCount = counts[class]
			bestClass = class
		}
	}
	return bestClass
}

func violationRetryClass(v types.Violation) string {
	target := FallbackTargetForViolation(v)
	if spec, ok := types.ViolKindSpecFor(v.Kind); ok {
		family := strings.TrimSpace(string(spec.CaveatFamilyID))
		if family != "" {
			return string(target) + ":" + family
		}
	}
	if v.Kind == "" {
		return ""
	}
	return string(target) + ":kind:" + string(v.Kind)
}

// applyContractViolations is the **single decision point** for
// where the contract checker's per-violation digest goes
// (user panel vs operator log). Replaces the 4 in-line calls to
// the pre-B4 appendViolationsToAnswer in the contract-failure
// switch (orchestrator.go retry / yield / per-kind / fallback
// terminations). Introduced in B4-F1 (2026-05-04).
//
// Channel decision per
// settings.ViolationBudget.UserVisibleViolationCaveat:
//
//	true  — prepend digest to answer (pre-B4 default).
//	false — write digest to logger.Warning only; answer
//	        stays clean (post-B4 default).
//
// Either way the digest also flows into per-stage CGEC summary
// telemetry via the closure's violation ledger, so operators
// always have the data; the toggle only controls whether the
// USER sees it.
//
// Returns the (possibly mutated) answer string. Empty / passing
// results are passthrough no-ops (no logger noise either).
func (o *Orchestrator) applyContractViolations(answer string, res contract.Result) string {
	digest := formatViolationsForLogger(res)
	if digest == "" {
		return answer
	}
	// Operator log line — fires regardless of channel choice so
	// operators always see the failure summary in trace logs.
	logging.Warning("[orchestrator] %s", digest)
	if !o.settings.ViolationBudget.UserVisibleViolationCaveat {
		return answer
	}
	// User-panel mode: prepend the digest with separator.
	return digest + "\n\n" + answer
}

// formatViolationFieldTally renders the ledger's per-field histogram
// as a compact stable string for the CGEC summary line:
// "{question_kind:3,ScannedSet:8,answer_subject.kind:12}". Keys are
// sorted so log diffs are deterministic; empty input returns "{}".
// Session 11 F1 — used only by emitCGECSummary.
func formatViolationFieldTally(tally map[string]int) string {
	if len(tally) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%s:%d", k, tally[k])
	}
	b.WriteString("}")
	return b.String()
}

// buildDegradedFallbackIR (commit 59 Batch E.1, audit HIGH #9)
// returns the minimal AnalysisIR the orchestrator falls back to
// when runAnalyzePhase exhausts retries with no usable IR. The
// IR carries:
//
//   - A single finalize node so the scheduler has SOMETHING to
//     dispatch (empty TaskGraph would loop or fail another gate).
//   - CitationReq.Required=false so the Run isn't blocked by
//     citation floors that explore never had a chance to fill.
//   - An AnalyzerHints carrying the original objective text so
//     the finalizer can quote what the user asked.
//
// Result: the user gets a degraded but honest reply ("I couldn't
// classify this question after N retries — please rephrase or
// break it down") instead of a hard failure with empty result.
// The TaskState.LastError still carries the diagnostic so wrapping
// CLI / REPL layers can surface it to the operator.
//
// Shape: pre-PR2 the fallback set AnalyzerHints.Shape and
// AnswerContract.RequiredAnswerShape to ShapeExplanation. With
// AnswerShape retired (PR1), the V2 block-only carrier derives
// rendering from AnswerSemanticView, so the fallback no longer
// touches shape fields — they are zero-valued and downstream
// consumers treat that as "no contract declared", which in
// degraded mode is the correct semantic.
func buildDegradedFallbackIR(objective string, analyzerErr error) *types.AnalysisIR {
	finalizeNode := types.TaskNode{
		ID:   "finalize",
		Type: types.NodeFinalize,
	}
	rm := types.RequestModel{
		RawRequest: objective,
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioGeneric,
		Complexity: types.ComplexitySimple,
	}
	return &types.AnalysisIR{
		RequestModel: rm,
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{finalizeNode},
		},
		AnswerContract: types.AnswerContract{
			CitationReq: types.CitationReq{Required: false},
		},
		QualityGate: types.GateReport{
			Rejected: false,
			Checks: []types.GateCheck{
				{
					Name:   "degraded_fallback",
					Passed: true,
					Detail: fmt.Sprintf("analyzer exhausted: %v — degraded to minimal IR", analyzerErr),
				},
			},
		},
	}
}

// buildDegradedSemanticIR (Fix-A 2026-05-10) builds a degraded
// AnalysisIR that PRESERVES the analyzer's last successful partial
// emit (RequestModel + AnalyzerHints) and includes a richer
// TaskGraph (probe → evidence → reconcile → finalize) so explorer
// + extractor still run even when the analyzer's quality gate
// rejected the IR repeatedly. Compared with buildDegradedFallbackIR
// (legacy zero-info path), this path keeps the LLM's already-
// extracted keywords / entities / sub_topics so downstream stages
// can build a useful answer instead of the legacy empty-answer
// symptom.
//
// Behaviour rules:
//   - When partialIR is nil (analyzer never emit_analysis'd):
//     fall back to the legacy zero-info IR.
//   - Auto-correct AnswerSubject: scalar kinds → Unknown / 0
//     (R2.2 family-shape contradiction would otherwise still
//     reject downstream gates if they re-run).
//   - Drop EvidencePlan / HypothesisSet / TermGraph (these were
//     malformed enough to fail the quality gate; keeping them
//     would just trigger downstream contract failures).
//   - Set CitationReq.Required = false (relaxed; the analyzer
//     was uncertain so the answer can't be expected to ground
//     to specific file:lines).
//   - TaskGraph: probe + 1 evidence + reconcile + finalize so the
//     scheduler's readyExplorerWindow returns non-empty and
//     explorer + extractor + finalizer all dispatch. Step budget
//     is bounded by the per-stage caps already in place.
//
// Cross-language: all preservation is on typed IR fields, not
// strings, so works regardless of question or repo language.
//
// Design doc: docs/design/analyzer_failure_remediation.md §3.1.
func buildDegradedSemanticIR(objective string, partialIR *types.AnalysisIR, analyzerErr error) *types.AnalysisIR {
	if partialIR == nil {
		return buildDegradedFallbackIR(objective, analyzerErr)
	}
	// Preserve the LLM's already-classified RequestModel core but
	// auto-correct any scalar AnswerSubject (R2.2 contradiction).
	rm := types.RequestModel{
		RawRequest: objective,
		Language:   partialIR.RequestModel.Language,
		Intent:     fallbackIntent(partialIR.RequestModel.Intent),
		Scenario:   fallbackScenario(partialIR.RequestModel.Scenario),
		Complexity: fallbackComplexity(partialIR.RequestModel.Complexity),
		AnalyzerHints: types.AnalyzerHints{
			Keywords:        dedupedStrings(partialIR.RequestModel.AnalyzerHints.Keywords),
			Entities:        dedupedStrings(partialIR.RequestModel.AnalyzerHints.Entities),
			PrimaryEntities: dedupedStrings(partialIR.RequestModel.AnalyzerHints.PrimaryEntities),
			// 2026-05-10 P2 audit follow-up: preserve the L3 / L4
			// typed channels (RequiredFileHints / IrrelevantFiles)
			// + adjacent multi-repo lanes (PrimaryScopes /
			// MentionedEntities / DerivedEntities). These signals
			// were emitted by the LLM and survived the analyzer's
			// emit-time decoder, so they're trustworthy independent
			// of whatever made the gate reject. Dropping them in
			// degraded recovery would force the explorer to fall
			// back to the deterministic resolver right after the
			// LLM made its strongest file-relevance judgement
			// available.
			MentionedEntities: dedupedStrings(partialIR.RequestModel.AnalyzerHints.MentionedEntities),
			DerivedEntities:   dedupedStrings(partialIR.RequestModel.AnalyzerHints.DerivedEntities),
			PrimaryScopes:     dedupedStrings(partialIR.RequestModel.AnalyzerHints.PrimaryScopes),
			ExactTargets:      dedupedStrings(partialIR.RequestModel.AnalyzerHints.ExactTargets),
			ExactContextTerms: dedupedStrings(partialIR.RequestModel.AnalyzerHints.ExactContextTerms),
			ExactContextRoles: append([]types.EvidenceDiagramRole(nil), partialIR.RequestModel.AnalyzerHints.ExactContextRoles...),
			RequiredFileHints: append([]types.RequiredFileHint(nil), partialIR.RequestModel.AnalyzerHints.RequiredFileHints...),
			IrrelevantFiles:   dedupedStrings(partialIR.RequestModel.AnalyzerHints.IrrelevantFiles),
			Kind:              partialIR.RequestModel.AnalyzerHints.Kind,
		},
		// Predicates copy keeps semantic flags (is_count_question,
		// is_role_locate_lookup etc.) so downstream stages still
		// see the LLM's original semantic classification even if
		// the rest of the IR was malformed.
		Predicates:    partialIR.RequestModel.Predicates,
		PredicateAxis: partialIR.RequestModel.PredicateAxis,
	}
	// Auto-correct R2.2 contradiction without relying on the prior
	// gate report — defensive: if R2.2 fired and was bypassed via
	// auto-correct elsewhere, rm.AnswerSubject is already Unknown;
	// if not, we clear it here for the same reason.
	switch partialIR.RequestModel.AnswerSubject.Kind {
	case types.SubjectStringLiteral, types.SubjectNumeric, types.SubjectReturnValue:
		rm.AnswerSubject = types.AnswerSubject{Kind: types.SubjectUnknown, Confidence: 0}
	default:
		rm.AnswerSubject = partialIR.RequestModel.AnswerSubject
	}
	// Build a richer TaskGraph: probe → evidence → reconcile →
	// finalize. The IDs and per-node SuccessCriteria mirror the
	// shape used by templateArchitectureExplain (the simplest
	// generic template) — explorer/extract loop them as a unit.
	probe := types.TaskNode{
		ID: "probe", Type: types.NodeProbe,
		Objective: "Breadth scan based on analyzer-extracted keywords (degraded recovery path).",
		Inputs:    []string{"user_question"},
		Outputs:   []string{"file_candidates"},
	}
	evidence := types.TaskNode{
		ID: "evidence", Type: types.NodeEvidence,
		Objective: "Collect grounded evidence from the candidate files (degraded recovery path).",
		Inputs:    []string{"file_candidates"},
		Outputs:   []string{"evidence_items"},
	}
	reconcile := types.TaskNode{
		ID: "reconcile", Type: types.NodeReconcile,
		Objective: "Reconcile collected evidence into a coherent answer (degraded recovery path).",
		Inputs:    []string{"evidence_items"},
		Outputs:   []string{"answer_summary"},
	}
	finalizeNode := types.TaskNode{
		ID:   "finalize",
		Type: types.NodeFinalize,
	}
	return &types.AnalysisIR{
		RequestModel: rm,
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{probe, evidence, reconcile, finalizeNode},
		},
		AnswerContract: types.AnswerContract{
			CitationReq: types.CitationReq{Required: false},
		},
		QualityGate: types.GateReport{
			Rejected: false,
			Checks: []types.GateCheck{
				{
					Name:   "degraded_semantic_recovery",
					Passed: true,
					Detail: fmt.Sprintf("analyzer exhausted but partial emit preserved: %v — running degraded probe → evidence → reconcile → finalize with kept keywords/entities/predicates", analyzerErr),
				},
			},
		},
	}
}

// dedupedStrings returns a trimmed deduplicated copy of in. Empty
// input returns nil so omitempty JSON tags drop the field cleanly.
func dedupedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		t := strings.TrimSpace(s)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fallbackIntent returns the partial-IR intent if non-empty, else
// IntentExplain (legacy degraded default).
func fallbackIntent(in types.Intent) types.Intent {
	if in == "" {
		return types.IntentExplain
	}
	return in
}

func fallbackScenario(in types.Scenario) types.Scenario {
	if in == "" {
		return types.ScenarioGeneric
	}
	return in
}

func fallbackComplexity(in types.Complexity) types.Complexity {
	if in == "" {
		return types.ComplexitySimple
	}
	return in
}

func successCriterionRepairLocusOverride(bus *types.BusContext, kind criterion.Kind, expr string) types.RepairLocus {
	if string(kind) != types.CritCitationCountGE {
		return ""
	}
	required, err := strconv.Atoi(strings.TrimSpace(expr))
	if err != nil || required <= 0 {
		return ""
	}
	if citationGradeAnchorCapacity(bus) >= required {
		return types.LocusFinalizer
	}
	return ""
}

func citationGradeAnchorCapacity(bus *types.BusContext) int {
	return len(citationGradeAnchorKeys(bus))
}

// runAnswerReviewerOnSuccess dispatches the end-of-Run
// answer-taxonomy reviewer when (a) it's a successful read-mode
// Run, (b) the reviewer is wired, (c) the store is wired, (d)
// the Run accumulated at least one retry event. Distilled
// patterns are persisted via store.Append (which dedups + decays).
//
// All failure modes are non-fatal and intentionally swallowed —
// the user's answer has already been computed, and a reviewer
// error must never mask it. The only side effect on a happy path
// is one additional Chat call + one disk write of the updated
// taxonomy.
func (o *Orchestrator) runAnswerReviewerOnSuccess() {
	if o == nil || o.busCtx == nil {
		return
	}
	if o.busCtx.TaskState.LastError != "" {
		return // failed Run; reviewer doesn't get a useful answer to read
	}
	// Read-mode only — write modes have their own reflector path.
	if o.busCtx.Mode != types.ModeRead && o.busCtx.Mode != "" {
		return
	}
	if o.answerReviewer == nil || o.answerTaxonomyStore == nil {
		return
	}
	if !o.answerTaxonomyStore.Enabled() {
		return
	}
	mu := o.busCtx.Mutable
	if mu == nil {
		return
	}
	events := mu.AnswerRetryEvents()
	// Commit 55 Batch A.2 (HIGH #1 fix): trigger condition broadened.
	// Pre-commit-55 the reviewer fired only on retry events. But a
	// Run that produced ONLY soft contract violations (commit 53 P3)
	// has zero retry events by construction (soft never re-queues)
	// — so the entire learning loop missed exactly the cases where
	// the analyzer-shape mismatch is most stable. Now: trigger when
	// EITHER retries happened OR the closure carries enough soft
	// violations (≥ 2) to suggest a generalisable pattern.
	// Round-10: count NON-PLUMBING violations only for the trigger
	// threshold. Plumbing kinds (ViolAuthorityOverreach etc.) describe
	// pipeline state, not answer-content quality; counting them in
	// the threshold inflates "we have learning surface" perception
	// and triggers a dispatch that the inner Round-6 filter then
	// aborts. Counting only content-class violations makes the
	// threshold semantic match the dispatch semantic.
	closureViolationCount := 0
	if cl := mu.EvidenceClosure(); cl != nil {
		for _, v := range cl.Violations() {
			if isPlumbingFailureViolation(v.Kind) {
				continue
			}
			closureViolationCount++
		}
	}
	const softViolationLearnFloor = 2
	if len(events) == 0 && closureViolationCount < softViolationLearnFloor {
		return
	}

	in := AnswerReviewerInput{
		OriginalRequest: mu.Objective(),
		IterationLedger: mu.IterationLedger(),
	}
	if doc := mu.AnswerDocumentV2(); doc != nil {
		// B8-T4: V2 carrier — the answer reviewer wants the prose
		// summary text. V2 carries this as a kind=summary block;
		// concatenate any present summary block(s).
		var sum strings.Builder
		for _, b := range doc.Blocks {
			if b.Kind == types.BlockSummary && strings.TrimSpace(b.Text) != "" {
				if sum.Len() > 0 {
					sum.WriteString("\n\n")
				}
				sum.WriteString(b.Text)
			}
		}
		in.FinalAnswer = sum.String()
	}
	if o.busCtx.AnalysisIR != nil {
		in.Scenario = string(o.busCtx.AnalysisIR.RequestModel.Scenario)
		in.Family = string(types.ResolveQuestionFamily(o.busCtx.AnalysisIR.RequestModel))
	}
	for _, e := range events {
		in.RetryEvents = append(in.RetryEvents, AnswerRetryEvent{Stage: e.Stage, Reason: e.Reason})
	}
	// Commit 55: synthesise pseudo-events from soft violations so the
	// reviewer sees the structured shape even when no retries fired.
	// The Stage tag distinguishes them from real retries; the LLM
	// reads them as "advisory observations" via the same prompt path.
	//
	// Round-6 filter: skip violation kinds that are plumbing-failure
	// signals rather than answer-quality signals. Feeding these to
	// answer_reviewer pollutes the persistent answer_taxonomy with
	// patterns about renderer bypass / structured-emit failure that
	// the next Run's analyzer will inject as "Known answer pitfalls"
	// — and the LLM has no actionable handle on those (they're
	// pipeline issues, not answer-content issues). Result pre-fix:
	// the analyzer's pitfalls section grows with noise that distracts
	// LLM attention from real answer-quality lessons. Post-fix:
	// answer_taxonomy stays focused on answer-quality patterns only.
	if len(events) == 0 && closureViolationCount > 0 {
		if cl := mu.EvidenceClosure(); cl != nil {
			for _, v := range cl.Violations() {
				if isPlumbingFailureViolation(v.Kind) {
					continue
				}
				in.RetryEvents = append(in.RetryEvents, AnswerRetryEvent{
					Stage:  fmt.Sprintf("contract.%s", v.Kind),
					Reason: v.Detail,
				})
			}
		}
	}
	// If after filtering plumbing-noise we have nothing left to feed
	// the reviewer, abort the dispatch — there's no answer-quality
	// signal worth learning from.
	if len(in.RetryEvents) == 0 {
		return
	}

	ctx := o.busCtx.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	pattern, err := o.answerReviewer.ReviewAnswer(ctx, in)
	if err != nil {
		if isReviewerNoToolCallError(err) {
			logging.Info("[answer_reviewer] skipped: reviewer returned prose without the required tool_call")
			return
		}
		// Commit 59 Batch E.1 (audit HIGH #12): record + log; the
		// Run-end summary surfaces these counts so operators see
		// learning chain health.
		mu.AppendLearningFailure("answer_reviewer", err.Error())
		logging.Warning("[answer_reviewer] dispatch failed (non-fatal): %v", err)
		return
	}
	if pattern == nil {
		// Reviewer judged the difficulty too specific to abstract.
		return
	}
	if _, err := o.answerTaxonomyStore.Append(pattern); err != nil {
		mu.AppendLearningFailure("answer_taxonomy_append", err.Error())
		logging.Warning("[answer_taxonomy] append failed (non-fatal): %v", err)
		return
	}
	logging.Info("[answer_taxonomy] persisted pattern name=%q from end-of-Run reviewer", pattern.Name)

	// Block 1 (architecture overhaul 2026-05-02) — also fold the
	// distilled pattern into the EvidenceClosure ledger of the CURRENT
	// Run so the stage-wise health snapshot at end-of-Run includes
	// "answer_reviewer surfaced N abstract pitfalls". Cross-Run
	// learning still flows through the failure_taxonomy store above
	// (separate sink, separate consumer); the closure write is
	// telemetry-only for the current Run. Soft-by-default — never
	// blocks anything; the answer has already shipped by this point.
	closure := mu.EvidenceClosure()
	if closure != nil {
		closure.AppendViolation(types.Violation{
			Kind:       types.ViolAnswerReviewerDistilled,
			ClusterKey: types.RootClusterKey("answer_reviewer_distilled"),
			Detail: fmt.Sprintf("answer_reviewer distilled pitfall %q (%s): %s",
				pattern.Name, pattern.Trigger, pattern.Description),
			Repair: "informational telemetry — the pattern has been persisted for cross-Run learning and will surface on similar future questions.",
			Stage:  string(types.StageFinalize),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_reviewer_distilled",
				Reason:     "end-of-Run reviewer abstracted a recurring answer-quality pitfall",
				Confidence: float64(pattern.Confidence),
			},
		})
	}
}
