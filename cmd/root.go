// Package cmd implements the Cobra-based CLI for codrax.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/gate"
	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/memlimit"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repomapindex "github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// Build-time variables injected via -ldflags.
var (
	version   = "dev"
	buildTime = "unknown"
)

// Code defaults for runtime settings.
//
// Two anchor domains:
//
//   - configAnchor (= exeDir) resolves providers_config. Configuration
//     files live next to the binary so one install = one config tree.
//   - runtimeAnchor (= <CWD>/.codrax) resolves log_dir / memory_dir /
//     cache_dir / blob sessions. Runtime artifacts follow the user's
//     workspace, not the install location.
//
// Relative defaults below are interpreted by the appropriate anchor
// in initApp; absolute YAML overrides and CLI flags pass through
// verbatim.
const (
	defaultLogDir           = "logs"
	defaultLogLevel         = "debug"
	defaultLogStdout        = false
	defaultMemoryDir        = "memory"
	defaultLang             = "zh"
	defaultThinkingTruncate = false
	defaultRepo             = "."
	defaultBranch           = "main"
	defaultMaxSteps         = 50
	defaultProvidersConfig  = "providers.yaml"
	runtimeAnchorDir        = ".codrax"
	blobSubdir              = "blob"
	defaultBlobMaxSessions  = 7

	// Final-answer transcript dump (read-mode only). The orchestrator
	// writes one .md file plus one self-contained .html sibling per
	// finalised answer to <runtime-anchor>/output/ after Mutable.SetResult
	// commits. outputSubdir is the directory name; defaultOutputMaxFiles is
	// the retention cap keyed by *.md (matching *.html siblings pruned with
	// them). Operator override
	// via codrax.yaml :: output_dump_enabled / output_max_files.
	outputSubdir          = "output"
	defaultOutputMaxFiles = 10
	defaultOutputDumpOn   = true

	// T3.1 keep-on-success worktree GC defaults.
	// 168 hours = 7 days; 20 worktrees keeps a busy operator's
	// disk under control without amputating active try-before-merge
	// flows. Both are pointer-overridable via codrax.yaml.
	defaultWorktreeKeepTTLHours = 168
	defaultWorktreeKeepMaxCount = 20
)

// CLI flag variables.
var (
	flagProviders string
	flagRepo      string
	flagBranch    string
	flagRequest   string
	flagMaxSteps  int
	flagLogDir    string
	flagLogLevel  string
	flagLogStdout bool
	flagMemoryDir string
	flagCacheDir  string
	flagLang      string
	flagColor     string
	// flagFocus is the multi-repo focus pin set passed at process
	// startup. Empty (default) = no pin; the routing fold's A
	// channel is empty and the auto-active set falls back to
	// language affinity / biggest-first selection. Each element is
	// a slug-or-RootRel (resolved through topology.Resolve so the
	// operator can use whichever form is more memorable). Repeated
	// `--focus` flags accumulate; comma-separated values inside one
	// flag are split during parse. Single-repo / no-git workspaces
	// silently ignore the flag — the topology has no sub-repo to
	// match. Designed for eval / scripted runs and operator
	// persistence; the REPL `/repos focus` command continues to work
	// session-locally on top of this.
	flagFocus []string
	// flagMultiRepoEnabled is the per-run override for
	// codrax.yaml :: multi_repo_enabled. The flag is a tristate:
	// nil (when --multi-repo isn't passed) → fall through to yaml;
	// true / false → override yaml regardless of its value. Read
	// by resolveRuntimeKnobsForApp once after yaml resolution.
	//
	// Why a tristate: the flag must distinguish "not passed → keep
	// yaml's choice" from "explicitly set to false → disable
	// regardless of yaml". Cobra's BoolVar can't do that on its
	// own; we use BoolVarP + cmd.Flags().Changed("multi-repo") to
	// detect whether the flag was actually set.
	flagMultiRepoEnabled bool
	flagMaxRetries       int
	flagMaxStageVisits   int
	flagMaxPrescanRounds int

	// Log-triage attach flags.
	//
	// `--log` is a REPEATABLE string slice: pass it multiple times
	// to attach several files in one Run (`--log a.log --log b.log`).
	// Each entry is either a filesystem path or `-` (stdin); only one
	// `-` is allowed across the whole flag set (stdin can be consumed
	// at most once per process). Files concatenate in CLI order with
	// a `# codrax-source: <path>\n` header per entry so the LLM can
	// distinguish boundaries.
	//
	// `--log-text` is a single inline payload (multi-paste is
	// uncommon and trivially worked around by `cat … | --log -`).
	// It and `--log` are mutually exclusive when both non-empty.
	flagAttachLog       []string
	flagAttachLogText   string
	flagLogSourcePrefix string

	// HiTrace / atrace / systrace / perfetto attach flags. Same
	// repeatable semantics as `--log`. Multiple captures from
	// different processes / time windows can be attached together;
	// the perf_triager / segmenter prompts handle multi-source
	// boundaries via the embedded `# codrax-source:` markers.
	flagAttachHitrace     []string
	flagAttachHitraceText string
	// --atrace / --atrace-text: aliases of --htrace / --htrace-text
	// for Android-spelling muscle memory. loadAttachedTrace promotes
	// either-or into the canonical flagAttachHitrace pipeline; setting
	// both htrace AND atrace simultaneously is rejected.
	flagAttachAtrace     []string
	flagAttachAtraceText string

	// Auto chit-chat classifier per-run toggle. When the flag is NOT
	// passed on the command line, the initApp merge falls back to the
	// codrax.yaml value (or the code default false). When passed,
	// overrides both. Useful for debugging classifier misjudgements
	// without editing yaml per run.
	flagChitchatClassifier bool

	// Single-shot CLI: opt-in to ASCII Mermaid rendering. Default
	// false preserves the pipeable-source contract — users who write
	// stdout to a file / markdown viewer / mermaid-cli get
	// authoritative source. Pass --mermaid-render to override for
	// terminal-only consumption (e.g. eval inspection). REPL is
	// unaffected — its renderer is always on.
	flagMermaidRender bool

	// B0 write-mode CLI flags. See resolveWriteMode for merge and
	// validation rules.
	//
	//   --mode        — read | plan | apply | verify. Empty (default)
	//                   coerces to "read" via PipelineMode.Normalize.
	//   --auto-apply  — required when --mode=apply is used in single-
	//                   shot (i.e. with --request). Acts as explicit
	//                   approval of the resulting ChangePlan without
	//                   an interactive prompt.
	//   --plan-out    — plan-mode output file path. When set, the
	//                   ChangePlan JSON writes here instead of the
	//                   default .codrax/plans/<id>.json.
	//   --plan-file   — apply/verify input plan file. REQUIRED when
	//                   --mode is apply or verify — those modes
	//                   consume an existing plan, they do not produce
	//                   one.
	flagMode      string
	flagAutoApply bool
	flagPlanOut   string
	flagPlanFile  string
	// flagAutoInitRepo authorizes the orchestrator to run `git init`
	// + an empty initial commit when the target repo is bare or has
	// no HEAD. Symmetric with --auto-apply: the user must explicitly
	// opt in for single-shot Runs that need scaffolding. yaml
	// equivalent: write_auto_init_repo. REPL falls back to an
	// interactive y/N prompt when neither flag nor yaml is set.
	flagAutoInitRepo bool

	// flagScaffold authorizes the planner to operate on a target
	// directory that contains NO existing source files (from-scratch
	// project creation). Distinct from flagAutoInitRepo: that flag
	// only authorizes `git init`; this one authorizes the planner
	// to invent files. Both are required for empty-dir scaffold
	// runs; non-empty bare dirs only need flagAutoInitRepo.
	// Default false. yaml equivalent: write_scaffold_enabled.
	flagScaffold bool
)

// defaultAttachedLogMaxBytes is the out-of-the-box cap on attached-
// log AND attached-hitrace payloads (each, independently). initApp
// overrides maxAttachedLogBytes from codrax.yaml ::
// log_attach_max_bytes; this const is both the default when the yaml
// key is absent and the fallback when the user sets a non-positive
// value.
//
// Rationale for the 512 MiB default (raised from 50 MiB in 2026-05 and
// then 256 MiB, then to 512 MiB in 2026-06 to
// match real systrace / perfetto / large hilog workflows):
//
//   - hdc shell hilog typically ships 10-50 MB per crash window
//   - hdc shell hitrace -t 30 produces 5-30 MB ftrace text
//   - adb logcat -d can be 20-100 MB on a busy device
//   - adb shell atrace -t 10 lands 5-20 MB
//
// 512 MiB is still bounded by the 1 GiB hard ceiling; the orchestrator's
// formatAttachedLog blob-offloads anything > 4 KB so the LLM prompt
// stays bounded regardless of body size. The cap is about ingestion
// memory safety and the truncation point; file/stdin attach paths read
// only up to this budget instead of slurping arbitrary multi-GB inputs.
// Users with bigger deployment needs can override via codrax.yaml ::
// log_attach_max_bytes, bounded by maxAttachedLogHardCeiling below.
const defaultAttachedLogMaxBytes = 512 * 1024 * 1024 // 512 MiB

// maxAttachedLogHardCeiling is the upper bound enforced on the
// resolved cap. A user who sets log_attach_max_bytes: 9999999999
// would otherwise expose codrax to OOM on a single oversized
// attachment. 1 GB is generous enough for "anything a human ever
// pastes or pipes through hdc / adb in one capture" while still
// fitting in commodity workstation RAM with headroom for the rest
// of the pipeline. Configurable via codrax.yaml only — there is no
// CLI flag because changing this is a deployment-policy decision,
// not a per-run knob.
const maxAttachedLogHardCeiling = 1 << 30 // 1 GiB

// maxAttachedLogBytes is the live cap read by loadAttachedLog +
// truncateAttachedLog. Initialised to defaultAttachedLogMaxBytes so
// unit tests that bypass initApp (e.g. call truncateAttachedLog
// directly) see the same out-of-box cap as production; initApp
// mutates it after merging codrax.yaml.
var maxAttachedLogBytes = defaultAttachedLogMaxBytes

// failureTaxonomyEnabled / MaxItems / DecayDays carry the
// yaml-merged knob values from the
// pipeline_failure_taxonomy_* family across the
// merge-block→wiring-block boundary in cmd. The store is
// constructed in the wiring block (which has flagRepo +
// flagCacheDir in scope but not the yaml struct); the merge
// block has the yaml struct but not the store. Package-level
// vars bridge the two, mirroring the same pattern
// maxAttachedTraceBytes uses for the trace-channel cap.
var (
	failureTaxonomyEnabled   = true
	failureTaxonomyMaxItems  = 0
	failureTaxonomyDecayDays = 0
)

// answerTaxonomyEnabled / MaxItems / DecayDays carry the
// pipeline_answer_taxonomy_* family (commit 51 Gap 3, mirror
// of failureTaxonomy). Same merge→wiring bridge.
var (
	answerTaxonomyEnabled   = true
	answerTaxonomyMaxItems  = 0
	answerTaxonomyDecayDays = 0
)

// richnessSofteningWarn carries the
// pipeline_richness_softening_warn yaml knob (B5-F2 / B5-F3,
// 2026-05-03) across merge→wiring. Default true so silent
// hard→soft / family-fit signals are visible at WARN level.
var richnessSofteningWarn = true

// selfConsistency* (commit 62) carry the pipeline_self_consistency_*
// family across the merge-block→wiring-block boundary. This-phase
// defaults: enabled=false, rewrite=true, min_confidence=0.92. Same
// rationale as answerTaxonomy package vars — yaml struct lives
// in the merge block, reviewer construction lives further down
// where flagRepo / providersCfg are in scope.
var (
	selfConsistencyEnabled = false
	selfConsistencyRewrite = true
	// 2026-05-10 P4 tightening: bumped 0.80 → 0.92. Forensic data
	// (May-9 sweep, 26 run / 19 case) showed the reviewer at the
	// 0.85-0.90 confidence band fired on borderline concerns the
	// answer-document V2 schema didn't structurally need to repair
	// (the answer was content-correct, the reviewer flagged minor
	// formatting / facet-anchor density). Raising the floor to
	// 0.92 keeps high-confidence factual contradictions (the
	// reviewer's load-bearing case) while filtering out the
	// noise band that contributed ~10-15% of the total
	// repair-loop activation rate. operator override path:
	// yaml `pipeline_self_consistency_min_confidence`.
	selfConsistencyMinConfidence = 0.92
)

// G5 (post_v2_runtime_gap_remediation, 2026-05-04). Semantic-quality
// reviewer wire-up state. Default false: deterministic final-answer
// checks remain on by default, but the extra post-finalize LLM reviewer
// is opt-in via pipeline_semantic_quality_review_enabled for operators
// who want the additional coverage signal despite the token/latency cost.
var (
	semanticQualityEnabled    = false
	strictAnswerReviewEnabled = true
	// P4 (2026-05-10): default floor for the G5 reviewer.
	// orchestrator.SemanticQualityMinConfidenceDefault is 0.92 — keep this
	// in sync (mirrored here as a Go var so the
	// PipelineSemanticQualityMinConfidence yaml override has a
	// distinct cmd-side seam to write into).
	semanticQualityMinConfidence = 0.92

	// P6 (2026-05-10): default cap on finalize-stage repair-loop
	// iterations before degrading to "ship doc + residual-concerns
	// caveat". Mirror of orchestrator.FinalizeRepairHardCapDefault (2)
	// so the PipelineFinalizeRepairHardCap yaml override has a
	// distinct cmd-side seam.
	finalizeRepairHardCap = 2

	// 2026-05-16: deterministic exhaustive member-set review
	// threshold. -1 → orchestrator falls back to
	// types.ExhaustiveMemberSetDefaultThreshold (30). 0 disables.
	// yaml override: pipeline_exhaustive_deterministic_review_threshold.
	exhaustiveDeterministicReviewThreshold = -1

	// 2026-05-16: per-reviewer wall-budget fraction. 0 keeps legacy
	// unguarded behaviour; default 0.4 from yaml gives each
	// reviewer at most 40% of remaining wall before the deadline
	// guard downgrades it to advisory.
	reviewerWallBudgetFraction = 0.4
)

// maxAttachedTraceBytes is the live cap for the perf-channel
// attachments (--htrace / --atrace / --htrace-text / --atrace-text /
// REPL /htrace / /atrace). Mirrors maxAttachedLogBytes for the log
// channel. Default behaviour is "inherit whatever log_attach_max_bytes
// resolves to" so a user who only sets log_attach_max_bytes still
// gets symmetric trace handling. When the yaml carries an explicit
// trace_attach_max_bytes value, it overrides this slot independently
// of the log cap.
//
// The hard ceiling clamp (maxAttachedLogHardCeiling, 1 GiB) applies
// to BOTH caps — neither channel may exceed the OOM-protection bound.
var maxAttachedTraceBytes = defaultAttachedLogMaxBytes

// appContext holds initialized state shared between subcommands.
type appContext struct {
	renderer              *render.Renderer
	orch                  *orchestrator.Orchestrator
	mcpRegistry           *mcp.Registry
	defaultLLM            llm.Adapter
	memorySummarizerLLM   llm.Adapter // providers.yaml :: agents.memory_summarizer, else defaultLLM
	reflectorLLM          llm.Adapter // providers.yaml :: agents.reflector, else defaultLLM
	planCriticLLM         llm.Adapter // providers.yaml :: agents.plan_critic, else defaultLLM
	acceptanceCheckerLLM  llm.Adapter // providers.yaml :: agents.acceptance_checker, else defaultLLM
	logger                *logging.Logger
	memorySettings        types.MemorySettings
	envRecommendSettings  types.EnvRecommendSettings
	markdownPreviewConfig preview.Config
	outputDumpDir         string
	outputDumpMax         int
	replPasteFoldMinChars int // 0 → repl.DefaultPasteFoldMinChars
	chitchatResponder     repl.ChitchatResponder
	chitchatClassifier    repl.ChitchatClassifier
	replHeaderPrinted     bool
	replModelListLine     string
	replModelSummaryLine  string
	operationRouteEnabled bool
	operationProviders    []operation.ProviderInfo
	// writeEnabled mirrors codrax.yaml :: write_enabled. Forwarded to
	// the REPL Config so /mode plan|apply|verify and /approve can be
	// rejected at the slash-command surface with a clear error pointing
	// at the yaml knob, instead of dispatching and surfacing a confusing
	// analyzer / planner failure deep inside the pipeline.
	writeEnabled bool
	// writeAutoInitRepo mirrors the resolved auto-init authorization
	// (yaml `write_auto_init_repo` OR CLI `--auto-init-repo`). When
	// true, the orchestrator's apply pre-hook will run `git init`
	// + empty initial commit on a bare/headless target instead of
	// failing. REPL also reads this to decide whether to skip the
	// interactive y/N consent prompt (pre-authorized → silent init).
	writeAutoInitRepo bool
	// writeScaffoldEnabled mirrors the resolved scaffold authorization
	// (yaml `write_scaffold_enabled` OR CLI `--allow-scaffold`). When
	// true, the orchestrator's plan pre-hook tolerates an empty
	// target dir (otherwise it fail-louds with a hint at the two
	// authorization surfaces). Forwarded to REPL Config so the same
	// authorization carries into interactive sessions.
	writeScaffoldEnabled bool
	// settingsPath is the resolved codrax.yaml path (or "" if none
	// found). Surfaced verbatim in the REPL's L2 gate error message
	// so the user knows WHICH file to edit. Set during initApp's
	// settings-resolution block.
	settingsPath string

	// worktreeKeepTTL + worktreeKeepMaxCount mirror the resolved
	// codrax.yaml :: worktree_keep_ttl_hours +
	// worktree_keep_max_count knobs (T3.1). Forwarded into the
	// REPL Config so /worktree gc applies the same caps as
	// startup. Zero on either disables that gate.
	worktreeKeepTTL      time.Duration
	worktreeKeepMaxCount int

	// topology is the multi-repo discovery snapshot for flagRepo.
	// Populated unconditionally at initApp (single-repo users get
	// a one-element snapshot via the §3.3.1 fast path; cost ~50µs).
	// Consumers (multigraph routing, REPL /repos command) read it;
	// it is NEVER mutated after initApp returns. Persisted under
	// <runtime-anchor>/cache/topology/<parent-slug>.json so warm
	// starts skip the BFS walk when the parent's special-files
	// fingerprint hasn't changed.
	topology *topology.RepoTopology

	// multiRepoEnabled mirrors codrax.yaml :: multi_repo_enabled
	// (default true). When false, Phase 4 multigraph routing falls
	// back to single-graph behaviour; discovery still runs because
	// the REPL `/repos` command depends on it (it just shows a hint
	// reminding the user the routing layer is disabled).
	multiRepoEnabled bool

	// multiRepoMaxActive mirrors codrax.yaml :: multi_repo_max_active
	// (config.MultiRepoMaxActiveDefault, 2; hard ceiling
	// config.MultiRepoMaxActiveCeiling, 3). The LRU cap on resident
	// sub-repo *Graphs.
	multiRepoMaxActive int

	// multiRepoInactivePreviewCount mirrors codrax.yaml ::
	// multi_repo_inactive_preview_count
	// (config.MultiRepoInactivePreviewCountDefault, 2; hard ceiling
	// config.MultiRepoInactivePreviewCountCeiling, 3). Caps how many
	// out-of-active sub-repos the L0 LLM advisory surfaces.
	multiRepoInactivePreviewCount int

	// multigraph is the session-shared MultiGraph carrier for the
	// multi-repo runtime. Constructed once in initApp from app.topology
	// after multi_repo_enabled/cap are resolved. nil when topology is
	// missing or multi_repo_enabled=false.
	//
	// Mutation surface: SetFocus/SetCap (REPL /repos focus + cap pushes
	// updates here so the next Run picks them up). Reconstruction on
	// /repos refresh swaps the pointer (provider closure reads the
	// latest value).
	//
	// Stored as any to avoid pulling multigraph into cmd's import
	// graph at the field level — the actual access goes through
	// repomap.MultiGraphFromContext / direct cast in the provider.
	multigraph any
}

var app appContext

// rootCmd is the top-level Cobra command.
var rootCmd = &cobra.Command{
	Use:   "codrax [request]",
	Short: "AI-powered code analysis and implementation pipeline",
	Long: `Codrax is a read-only code analysis tool with a 6-layer, 4-agent
pipeline (analyze → explore → extract → finalize).

When invoked with a request, runs the pipeline once and exits.
When invoked with no arguments, enters interactive REPL mode.`,
	Args:              cobra.MaximumNArgs(1),
	PersistentPreRunE: initApp,
	RunE:              rootRun,
	SilenceUsage:      true,
	SilenceErrors:     true,
	// Version powers cobra's auto-wired --version / -v flag. Format
	// matches the dedicated `codrax version` subcommand so the two
	// surfaces are interchangeable.
	Version: fmt.Sprintf("%s (built %s)", version, buildTime),
}

func init() {
	f := rootCmd.PersistentFlags()
	f.StringVar(&flagProviders, "providers", "", "path to providers config")
	f.StringVar(&flagRepo, "repo", ".", "repository root path")
	f.StringVar(&flagBranch, "branch", "main", "git branch")
	f.StringVarP(&flagRequest, "request", "r", "", "user request (alternative to positional arg)")
	f.IntVar(&flagMaxSteps, "pipeline-max-steps", defaultMaxSteps, "maximum total pipeline steps per Run()")
	f.StringVar(&flagLogDir, "log-dir", "", "directory for log files")
	f.StringVar(&flagLogLevel, "log-level", defaultLogLevel, "log level: error|warning|info|debug")
	f.BoolVar(&flagLogStdout, "log-stdout", false, "also mirror logs to stdout")
	f.StringVar(&flagMemoryDir, "memory-dir", "", "directory for conversation memory")
	f.StringVar(&flagCacheDir, "cache-dir", "", "base directory for repo map caches (empty = ~/.codrax/cache; %USERPROFILE%\\.codrax\\cache on Windows)")
	f.StringVar(&flagLang, "lang", defaultLang, "default response language (zh/en/...); 'off' to disable")
	f.StringVar(&flagColor, "color", "auto", "color mode for diff rendering: auto (TTY-detect, default) | always | never. NO_COLOR env always forces never.")
	// --focus pins one or more sub-repos into the multi-repo active
	// set at startup. Repeatable + comma-separated values supported
	// (`--focus repoA --focus repoB,repoC`). Each value is a
	// sub-repo slug OR its path-from-parent (RootRel) — both are
	// surfaced by `/repos`. Single-repo / no-git workspaces silently
	// ignore the flag. Equivalent to running `/repos focus <slug>`
	// once per value at REPL boot, but also works in scripted /
	// non-REPL invocations and persists across REPL re-entries when
	// captured in a wrapper.
	f.StringSliceVar(&flagFocus, "focus", nil, "multi-repo: pin one or more sub-repos by slug or path (repeatable, comma-separated). Single-repo / no-git workspaces ignore the flag.")
	// --multi-repo overrides codrax.yaml :: multi_repo_enabled for
	// this Run. Detected via cmd.Flags().Changed("multi-repo") so
	// the default (false) at the BoolVar level is ignored when the
	// operator did NOT pass the flag — yaml's choice (or its nil →
	// true default) wins in that case.
	f.BoolVar(&flagMultiRepoEnabled, "multi-repo", false, "enable/disable multi-repo discovery for this run (overrides codrax.yaml :: multi_repo_enabled when passed; no-op when omitted). Use --multi-repo=false to skip discovery on a parent dir that contains many sub-repos.")
	f.IntVar(&flagMaxRetries, "pipeline-max-retries", 0, "override max consecutive failures per stage; 0 = inherit from codrax.yaml")
	f.IntVar(&flagMaxStageVisits, "pipeline-max-stage-visits", 0, "override max entries per stage per Run; 0 = inherit from codrax.yaml")
	f.IntVar(&flagMaxPrescanRounds, "max-prescan-rounds", 0, "override analyzer prescan budget rounds (codrax.yaml :: analysis_max_prescan_rounds); 0 = inherit from yaml/default. Multi-topic questions still get a +1 bump on top, capped at agent_prescan_rounds_ceil (default 4).")
	f.StringArrayVar(&flagAttachLog, "log", nil, "attach a runtime log excerpt (panic / exception / traceback) from a file path, or '-' for stdin. Repeatable: --log a.log --log b.log attaches both, joined with `# codrax-source: <path>` headers so the LLM can distinguish boundaries. Total bytes capped by codrax.yaml :: log_attach_max_bytes.")
	f.StringVar(&flagAttachLogText, "log-text", "", "inline runtime log excerpt (mutually exclusive with --log); for scripted / piped usage")
	f.StringArrayVar(&flagAttachHitrace, "htrace", nil, "attach an ftrace-compatible trace from file path (or '-' for stdin). Covers HarmonyOS `hdc shell hitrace`, Android `adb shell atrace`, systrace, and perfetto text dumps. Repeatable: --htrace a.trace --htrace b.trace. --atrace is an alias.")
	f.StringVar(&flagAttachHitraceText, "htrace-text", "", "inline trace payload (mutually exclusive with --htrace)")
	// --atrace / --atrace-text: Android-flavored aliases. Backed by
	// the same channel as --htrace; the merge in loadAttachedTrace
	// rejects setting both flavours. No CLI semantic difference —
	// purely a usability accommodation so `adb` users don't translate.
	f.StringArrayVar(&flagAttachAtrace, "atrace", nil, "alias of --htrace (repeatable). Use whichever name matches your capture tool — both feed the perf_triage stage identically.")
	f.StringVar(&flagAttachAtraceText, "atrace-text", "", "alias of --htrace-text (inline trace payload)")
	f.StringVar(&flagLogSourcePrefix, "log-source-prefix", "", "strip this path prefix from C/C++ stack-frame files before repo lookup (override for build-machine absolute paths)")
	f.BoolVar(&flagChitchatClassifier, "chitchat-classifier", false, "enable/disable the auto chit-chat classifier for this run (overrides codrax.yaml :: chitchat_classifier_enabled when passed; no-op when omitted)")
	f.BoolVar(&flagMermaidRender, "mermaid-render", false, "single-shot only: render ```mermaid``` fences as aligned ASCII for terminal viewing. Default false ships raw Mermaid source so output piped to file / markdown viewer / mermaid-cli stays authoritative. REPL renders unconditionally regardless of this flag.")
	// B6 (block_only_carrier.md, 2026-05-03) — V2 carrier per-run override.
	// auto = follow yaml pipeline_emit_v2_default (default true at B6).
	// on = force V2 carrier; off = force V1 (rollback). Removed at B8-T7.
	// B0 write-mode flags. Gated by codrax.yaml :: write_enabled
	// (default false) — registering the flags always is fine because
	// resolveWriteMode validates the combination before orch.SetMode.
	f.StringVar(&flagMode, "mode", "", "pipeline mode: read|plan|apply|verify (empty = read; requires codrax.yaml :: write_enabled=true for non-read modes)")
	f.BoolVar(&flagAutoApply, "auto-apply", false, "approve the generated ChangePlan without prompting (required with --mode=apply in single-shot)")
	f.StringVar(&flagPlanOut, "plan-out", "", "plan-mode: path to write the generated ChangePlan JSON (default: .codrax/plans/<id>.json)")
	f.StringVar(&flagPlanFile, "plan-file", "", "apply/verify-mode: path to an existing ChangePlan JSON to consume (required with --mode=apply|verify)")
	f.BoolVar(&flagAutoInitRepo, "auto-init-repo", false, "authorize codrax to run `git init` + empty initial commit when the target dir is bare (yaml: write_auto_init_repo)")
	f.BoolVar(&flagScaffold, "allow-scaffold", false, "authorize the planner to invent files for a 0-source-file target dir (from-scratch project creation; yaml: write_scaffold_enabled). Required IN ADDITION TO --auto-init-repo for empty-dir runs.")

	rootCmd.AddCommand(versionCmd)

	// Match the `codrax version` subcommand output exactly so the two
	// surfaces are interchangeable; cobra's default template prepends
	// "codrax version " which would otherwise diverge.
	rootCmd.SetVersionTemplate(fmt.Sprintf("codrax %s (built %s)\n", version, buildTime))
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("codrax %s (built %s)\n", version, buildTime)
	},
}

// Execute runs the root command.
func Execute() error {
	return ExecuteArgs(os.Args[1:])
}

// ExecuteArgs runs the root command with an explicit argv slice.
// A small compatibility normalizer rewrites accidentally-single-dashed
// long flags like `-request` to their canonical `--request` form so
// historical docs and muscle memory do not silently corrupt the value
// into `-r equest`.
func ExecuteArgs(args []string) error {
	rootCmd.SetArgs(normalizeCompatArgs(args))
	return rootCmd.Execute()
}

// compatLongFlagNames is the allowlist of long-form codrax flags we
// accept in the legacy single-dash form for CLI compatibility.
var compatLongFlagNames = map[string]struct{}{
	"providers":                 {},
	"repo":                      {},
	"branch":                    {},
	"request":                   {},
	"pipeline-max-steps":        {},
	"log-dir":                   {},
	"log-level":                 {},
	"log-stdout":                {},
	"memory-dir":                {},
	"cache-dir":                 {},
	"lang":                      {},
	"pipeline-max-retries":      {},
	"pipeline-max-stage-visits": {},
	"log":                       {},
	"log-text":                  {},
	"htrace":                    {},
	"htrace-text":               {},
	"atrace":                    {},
	"atrace-text":               {},
	"log-source-prefix":         {},
	"chitchat-classifier":       {},
	"mode":                      {},
	"auto-apply":                {},
	"plan-out":                  {},
	"plan-file":                 {},
	"auto-init-repo":            {},
	"allow-scaffold":            {},
}

// normalizeCompatArgs rewrites known codrax long flags from the
// historical single-dash spelling (`-request`) to the canonical POSIX
// form (`--request`). It is intentionally narrow:
//
//   - only names in compatLongFlagNames are rewritten
//   - short flags like `-r` are left untouched
//   - `--` terminates rewriting, preserving positional args verbatim
func normalizeCompatArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}

	out := make([]string, 0, len(args))
	stop := false
	for _, arg := range args {
		if stop || arg == "--" {
			out = append(out, arg)
			if arg == "--" {
				stop = true
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") || arg == "-" {
			out = append(out, arg)
			continue
		}

		name, value, hasEq := strings.Cut(strings.TrimPrefix(arg, "-"), "=")
		if len(name) <= 1 {
			out = append(out, arg)
			continue
		}
		if _, ok := compatLongFlagNames[name]; !ok {
			out = append(out, arg)
			continue
		}

		if hasEq {
			out = append(out, "--"+name+"="+value)
			continue
		}
		out = append(out, "--"+name)
	}
	return out
}

// rootRun dispatches to single-shot or REPL mode.
func rootRun(cmd *cobra.Command, args []string) error {
	if app.mcpRegistry != nil {
		defer app.mcpRegistry.Close()
	}

	// Resolve request: --request flag or positional arg.
	request := flagRequest
	if request == "" && len(args) > 0 {
		request = args[0]
	}

	// Resolve attached log (file / stdin / inline text) before Run so
	// both CLI single-shot and REPL sticky-log paths share one source.
	attached, err := loadAttachedLog()
	if err != nil {
		return err
	}
	if attached != "" {
		app.orch.SetAttachedLog(attached)
		logging.Info("[cmd] attached runtime log: %d bytes", len(attached))
	}
	// Attach HiTrace / Android systrace separately so the independent
	// perf_triage pre-stage can consume it without polluting the
	// log_triage channel.
	trace, traceSource, err := loadAttachedTrace()
	if err != nil {
		return err
	}
	if trace != "" {
		app.orch.SetAttachedHitrace(trace)
		app.orch.SetAttachedHitraceSource(traceSource)
		logging.Info("[cmd] attached hitrace: %d bytes", len(trace))
	}
	if flagLogSourcePrefix != "" {
		logtriage.SetSourcePrefix(flagLogSourcePrefix)
		logging.Info("[cmd] log source prefix override: %s", flagLogSourcePrefix)
	}

	// Apply / verify single-shot fallback: when --mode=apply or
	// --mode=verify is supplied with --plan-file but NO --request, the
	// natural intent is "run apply/verify on this saved plan", not
	// "open the REPL." Hydrate the request from the plan file so
	// runSingleShot can dispatch with a meaningful prompt.
	if request == "" && flagPlanFile != "" {
		mode := strings.TrimSpace(flagMode)
		if mode == string(types.ModeApply) || mode == string(types.ModeVerify) {
			if plan, perr := types.LoadChangePlanFromFile(flagPlanFile); perr == nil && plan != nil {
				request = strings.TrimSpace(plan.Request)
				if request == "" {
					request = "Apply the saved ChangePlan and verify its tests pass."
				}
				logging.Info("[cmd] %s mode hydrated request from plan file (no --request given)", mode)
			}
		}
	}

	if request == "" {
		return runREPL(cmd)
	}
	return runSingleShot(cmd, request)
}

// loadAttachedLog returns the merged --log / --log-text payload
// (size-capped at log_attach_max_bytes). Trace attachments are
// loaded separately by loadAttachedTrace and bounded by the
// independent trace_attach_max_bytes cap.
//
// Rules:
//   - --log (slice) and --log-text are mutually exclusive when both
//     are non-empty
//   - exactly one `-` is allowed across BOTH --log and the trace
//     entries combined (only one stdin consumer per Run); the
//     stdin gate is enforced in this function before any read
//   - multiple --log entries are concatenated in CLI order, each
//     prefixed by a `# codrax-source: <path>\n` header so the LLM
//     can distinguish file boundaries (e.g. multi-process panics)
//   - the combined byte length is capped at maxAttachedLogBytes;
//     excess bytes are tail-truncated with a WARN
func loadAttachedLog() (string, error) {
	if len(flagAttachLog) > 0 && flagAttachLogText != "" {
		return "", fmt.Errorf("--log and --log-text are mutually exclusive")
	}
	if err := enforceStdinExclusivity(); err != nil {
		return "", err
	}
	body, err := loadMultiPathSlice("log", flagAttachLog, flagAttachLogText, maxAttachedLogBytes)
	if err != nil {
		return "", err
	}
	if body == "" {
		return "", nil
	}
	return truncateAttachedToCap(body, maxAttachedLogBytes, "log"), nil
}

// loadAttachedTrace returns the merged trace payload (size-capped).
// Accepts any combination of --htrace / --atrace (slices) plus
// --htrace-text / --atrace-text (single inline strings). Within-
// flavour (file vs inline) and cross-flavour (htrace vs atrace) are
// mutually exclusive when both are non-empty. Multiple --htrace
// (or --atrace) entries concatenate with `# codrax-source:` headers
// the same way --log does.
func loadAttachedTrace() (string, string, error) {
	if len(flagAttachHitrace) > 0 && flagAttachHitraceText != "" {
		return "", "", fmt.Errorf("--htrace and --htrace-text are mutually exclusive")
	}
	if len(flagAttachAtrace) > 0 && flagAttachAtraceText != "" {
		return "", "", fmt.Errorf("--atrace and --atrace-text are mutually exclusive")
	}
	if (len(flagAttachHitrace) > 0 || flagAttachHitraceText != "") &&
		(len(flagAttachAtrace) > 0 || flagAttachAtraceText != "") {
		return "", "", fmt.Errorf("--htrace and --atrace are aliases — set only one")
	}
	// Promote --atrace* into --htrace* so the rest of the loader only
	// sees one shape.
	paths := flagAttachHitrace
	text := flagAttachHitraceText
	source := "harmony_hitrace"
	if len(flagAttachAtrace) > 0 {
		paths = flagAttachAtrace
		source = "android_atrace"
	}
	if flagAttachAtraceText != "" {
		text = flagAttachAtraceText
		source = "android_atrace"
	}
	if err := enforceStdinExclusivity(); err != nil {
		return "", "", err
	}
	body, err := loadMultiPathSlice("trace", paths, text, maxAttachedTraceBytes)
	if err != nil {
		return "", "", err
	}
	if body == "" {
		return "", "", nil
	}
	return truncateAttachedToCap(body, maxAttachedTraceBytes, "trace"), source, nil
}

// enforceStdinExclusivity checks that at most one `-` appears across
// every repeated --log / --htrace / --atrace entry. Returns an error
// when two or more would consume stdin (impossible on a single
// process). Called from both loaders before any actual read so the
// failure is reported once at flag-validation time rather than on
// the second read where stdin is already drained.
func enforceStdinExclusivity() error {
	stdinCount := 0
	for _, p := range flagAttachLog {
		if p == "-" {
			stdinCount++
		}
	}
	for _, p := range flagAttachHitrace {
		if p == "-" {
			stdinCount++
		}
	}
	for _, p := range flagAttachAtrace {
		if p == "-" {
			stdinCount++
		}
	}
	if stdinCount > 1 {
		return fmt.Errorf("only one of --log/--htrace/--atrace can consume stdin (`-`) per run; got %d", stdinCount)
	}
	return nil
}

// loadMultiPathSlice reads every entry in `paths` (or returns
// `inlineText` when the slice is empty), prefixing each file body
// with `# codrax-source: <path>\n`. Per-file stdin read is bounded
// by `cap` (channel-specific) so a single oversized file cannot OOM
// the process; the final aggregate cap applies in
// truncateAttachedToCap. `kind` is "log" / "trace" for error context.
func loadMultiPathSlice(kind string, paths []string, inlineText string, cap int) (string, error) {
	if inlineText != "" {
		return inlineText, nil
	}
	if len(paths) == 0 {
		return "", nil
	}
	if cap <= 0 {
		cap = defaultAttachedLogMaxBytes
	}
	var b strings.Builder
	remaining := cap + 1
	for _, p := range paths {
		if remaining <= 0 {
			break
		}
		// Header keeps file boundaries visible to the LLM. Single-
		// path attachments still get a header for symmetry; the
		// log-triage / perf-triage skill prompts reference the
		// `# codrax-source:` token by literal so this is part of
		// the contract, not just decoration.
		if b.Len() > 0 {
			b.WriteByte('\n')
			remaining--
			if remaining <= 0 {
				break
			}
		}
		header := fmt.Sprintf("# codrax-source: %s\n", p)
		if len(header) > remaining {
			b.WriteString(header[:remaining])
			break
		}
		b.WriteString(header)
		remaining -= len(header)
		if remaining <= 0 {
			break
		}
		data, err := readAttachedSourceLimited(p, remaining)
		if err != nil {
			return "", fmt.Errorf("load attached %s %q: %w", kind, p, err)
		}
		b.Write(data)
		remaining -= len(data)
	}
	return b.String(), nil
}

func readAttachedSourceLimited(path string, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, nil
	}
	if path == "-" {
		return io.ReadAll(io.LimitReader(os.Stdin, int64(limit)))
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(limit)))
}

func truncateAttachedLog(s string) string {
	return truncateAttachedToCap(s, maxAttachedLogBytes, "log")
}

// truncateAttachedToCap is the per-channel truncation entry point.
// Mirrors truncateAttachedLog but parameterised by cap + a label so
// the WARN message names the right channel ("log" vs "trace") when
// the caps differ.
func truncateAttachedToCap(s string, cap int, kind string) string {
	if len(s) <= cap {
		return s
	}
	logging.Warning("[cmd] attached %s truncated: %d → %d bytes", kind, len(s), cap)
	return s[:cap]
}

// writeModeInputs groups the parameters resolveWriteMode inspects
// so the function stays pure (no global-state access) and the unit
// tests can exercise every combination without constructing a full
// RuntimeSettings + cobra.Command. initApp assembles this struct
// from rs + flag values and hands it off.
type writeModeInputs struct {
	// YamlEnabled is codrax.yaml :: write_enabled. nil = field
	// absent in yaml (fall through to default false).
	YamlEnabled *bool
	// YamlDefaultMode is codrax.yaml :: write_default_mode. nil =
	// field absent (fall through to empty, which Normalize coerces
	// to ModeRead at Run entry).
	YamlDefaultMode *string
	// CLIFlagPassed is cmd.Flags().Changed("mode") — true iff the
	// user typed --mode on the command line, even with the same
	// value as yaml. Distinguishes "CLI explicitly read" from
	// "CLI defaulted to yaml value".
	CLIFlagPassed bool
	// CLIFlagMode is the --mode flag string (empty when
	// CLIFlagPassed=false).
	CLIFlagMode string
	// HasRequest indicates single-shot invocation (flagRequest or
	// positional arg is non-empty). REPL mode passes false, and
	// the L4 single-shot-apply-needs-auto-apply validation is
	// skipped because REPL's /approve command handles interactive
	// approval.
	HasRequest bool
	// AutoApply is --auto-apply.
	AutoApply bool
	// PlanFile is --plan-file (pre-absolutization — validation
	// only cares whether it is empty).
	PlanFile string
}

// resolveWriteMode merges yaml + CLI into a final PipelineMode and
// validates the combination. Errors are terminal — the caller
// (initApp) returns them to cobra which prints and exits before any
// Run() starts.
//
// Precedence (lowest → highest): code default ("" → ModeRead at
// Run entry) → yaml write_default_mode → CLI --mode. Each tier only
// overrides when it holds a non-empty value; an empty CLI flag
// (default) falls back to yaml, which falls back to "".
//
// Validation rules, evaluated in order:
//
//  1. If CLIFlagMode is non-empty, it must parse to a valid
//     PipelineMode (read / plan / apply / verify). Unknown values
//     are rejected with "unknown mode".
//
//  2. If YamlDefaultMode is non-empty, same validity check. Plus
//     a yaml-specific rule: the defaults "apply" and "verify" are
//     rejected because those modes have real side effects (git
//     worktree creation, test execution) and must be opted into
//     per-run via --mode rather than silently enabled by a shared
//     config file.
//
//  3. If the resolved mode is a write mode (plan / apply / verify)
//     and YamlEnabled is nil-or-false, the whole configuration is
//     rejected with "write mode disabled". This is the L2 red line
//     in session 33's B0 plan — operators must explicitly opt in
//     via codrax.yaml :: write_enabled: true before any non-read
//     mode can run.
//
//  4. If HasRequest (single-shot) AND the resolved mode is
//     ModeApply AND AutoApply is false, reject. The L4 red line:
//     apply runs real side effects, so the single-shot workflow
//     must confirm with --auto-apply. REPL mode skips this check
//     because /approve provides interactive confirmation.
//
//  5. If the resolved mode is ModeApply or ModeVerify, PlanFile
//     must be non-empty — those modes consume an existing plan,
//     they do not produce one.
//
// On success returns the effective PipelineMode (possibly "" for
// the default read case; caller passes it to orch.SetMode which
// normalizes at Run entry).
func resolveWriteMode(in writeModeInputs) (types.PipelineMode, error) {
	// Tier resolution.
	yamlEnabled := false
	if in.YamlEnabled != nil {
		yamlEnabled = *in.YamlEnabled
	}

	// Start with code default (empty).
	mode := types.PipelineMode("")

	// Apply yaml default.
	if in.YamlDefaultMode != nil {
		ymode := types.PipelineMode(strings.TrimSpace(*in.YamlDefaultMode))
		if ymode != "" {
			if !ymode.IsValid() {
				return "", fmt.Errorf("codrax.yaml write_default_mode invalid: %q (must be one of: read, plan)", string(ymode))
			}
			if ymode == types.ModeApply || ymode == types.ModeVerify {
				return "", fmt.Errorf("codrax.yaml write_default_mode cannot be %q (apply/verify require per-run --mode flag; set to 'read' or 'plan' instead)", string(ymode))
			}
			mode = ymode
		}
	}

	// Apply CLI override.
	if in.CLIFlagPassed {
		cmode := types.PipelineMode(strings.TrimSpace(in.CLIFlagMode))
		if cmode != "" {
			if !cmode.IsValid() {
				return "", fmt.Errorf("unknown pipeline mode %q (must be one of: read, plan, apply, verify)", string(cmode))
			}
			mode = cmode
		} else {
			// Explicit --mode="" means "revert to empty / read".
			mode = ""
		}
	}

	// Write-enabled gate (L2).
	if !yamlEnabled && mode.IsWrite() {
		return "", fmt.Errorf("write mode disabled in codrax.yaml (write_enabled: false or unset); cannot use --mode=%s", string(mode))
	}

	// Single-shot apply needs --auto-apply (L4).
	if in.HasRequest && mode == types.ModeApply && !in.AutoApply {
		return "", fmt.Errorf("--mode=apply in single-shot (--request) requires --auto-apply for approval")
	}

	// Apply / verify need a plan file.
	if (mode == types.ModeApply || mode == types.ModeVerify) && strings.TrimSpace(in.PlanFile) == "" {
		return "", fmt.Errorf("--mode=%s requires --plan-file <path>", string(mode))
	}

	return mode, nil
}

// runSingleShot executes one pipeline run and prints the result.
// Unlike the REPL path, single-shot outputs raw markdown (no glamour
// ANSI rendering) so the output is clean when piped to another tool
// or written to a file. The REPL owns the glamour rendering step via
// its own RenderResult callback.
func runSingleShot(_ *cobra.Command, request string) error {
	logging.Info("starting pipeline for request: %s", request)
	app.renderer.SetOutput(os.Stderr)
	// Single-shot CLI: ship mermaid source as-is in the default
	// path so output piped to file / markdown viewer / mermaid-cli
	// stays authoritative. The render hook is intentionally NOT
	// installed at any intermediate stage — the only place mermaid
	// rendering happens in single-shot is the final user-output
	// step below (gated by --mermaid-render). REPL has its own
	// renderer at the user-output boundary; both paths follow the
	// "render only at the user-facing edge" rule.
	busCtx, err := app.orch.Run(request, flagRepo, flagBranch)
	if err != nil {
		logging.Error("pipeline failed: %v", err)
		return fmt.Errorf("pipeline failed: %w", err)
	}
	if busCtx == nil {
		fmt.Println("(no result)")
		return nil
	}
	if busCtx.TaskState.LastError != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n", busCtx.TaskState.LastError)
	}

	// Plan-mode disk writer (B0 Day 5). When the orchestrator ran
	// ModePlan and the planner successfully emitted a ChangePlan,
	// serialize it to disk at the user-requested --plan-out path or
	// the default .codrax/plans/<id>.json. Failure here is non-
	// fatal for single-shot: the summary still prints to stdout,
	// and the error goes to stderr so scripts can detect the
	// partial success. REPL Runs bypass this path — PlanStore
	// (Day 6) handles REPL plan persistence.
	if busCtx.Mutable != nil && busCtx.Mode == types.ModePlan {
		if plan := busCtx.Mutable.ChangePlan(); plan != nil {
			path, perr := writePlanFile(plan)
			if perr != nil {
				logging.Warning("[cmd] plan file write failed: %v", perr)
				fmt.Fprintf(os.Stderr, "plan file write failed: %v\n", perr)
			} else {
				fmt.Fprintf(os.Stderr, "\n[plan written: %s]\n", path)
				logging.Info("[cmd] plan written: %s", path)
			}
		}
	}

	if busCtx.Mutable != nil {
		if result := busCtx.Mutable.Result(); result != "" {
			logging.Info("final answer:\n%s", result)
			// --mermaid-render: opt-in ASCII transform for terminal
			// consumption. Default off ships raw Mermaid source for
			// downstream piping (file / mermaid-cli / markdown viewer).
			if flagMermaidRender {
				result = render.RenderMermaidBlocks(result)
			}
			// Separator between thinking trace (stderr) and answer (stdout).
			// On a terminal both streams interleave visually, so the
			// separator helps the user see where the answer begins.
			fmt.Fprintf(os.Stderr, "\n━━━\n\n")
			fmt.Print(result)
			if !strings.HasSuffix(result, "\n") {
				fmt.Println()
			}
			return nil
		}
	}
	fmt.Println("(no result)")
	return nil
}

// writePlanFile serializes a ChangePlan to disk as JSON. Target path
// precedence: --plan-out flag > codrax.yaml :: write_plan_dir / <id>.json
// > default <runtime-anchor>/plans/<id>.json. Ensures the parent
// directory exists (MkdirAll idempotent). Returns the absolute path
// written or an error with context.
//
// JSON format is compact-with-indent for readability — operators
// inspecting a plan by hand see structured output, not a single line.
// Day 6 REPL PlanStore will reuse this helper for sticky-plan persistence.
func writePlanFile(plan *types.ChangePlan) (string, error) {
	if plan == nil {
		return "", fmt.Errorf("writePlanFile: nil plan")
	}
	targetPath := strings.TrimSpace(flagPlanOut)
	if targetPath == "" {
		// Default: <runtime-anchor>/plans/<id>.json. The
		// runtime-anchor was already absolutised at initApp entry.
		runtimeAnchor := runtimeAnchorDir
		if abs, err := filepath.Abs(runtimeAnchor); err == nil {
			runtimeAnchor = abs
		}
		targetPath = filepath.Join(runtimeAnchor, "plans", plan.ID+".json")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(targetPath), err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal plan: %w", err)
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", targetPath, err)
	}
	return targetPath, nil
}

// runREPL starts the interactive multi-turn session.
func runREPL(_ *cobra.Command) error {
	app.renderer.SetOutput(os.Stdout)
	summarizer := newLLMSummarizer(app.memorySummarizerLLM)
	store, err := memory.NewStore(flagMemoryDir, summarizer, app.memorySettings)
	if err != nil {
		logging.Error("memory store init failed: %v", err)
		return fmt.Errorf("memory store init failed: %w", err)
	}
	// Wire the read-side adapter into the orchestrator so each Run's
	// BusContext.Memory carries it. Tools (recall_memory) consume it
	// via the types.MemoryReader interface — no memory-package import
	// from the tool layer. Adapter is nil-safe; single-shot CLI never
	// reaches this branch (runREPL path only).
	app.orch.SetMemoryReader(memory.NewAdapter(store))
	app.orch.SetEnvRecommendSettings(app.envRecommendSettings)
	env.SetCacheTTL(app.envRecommendSettings.CacheTTLDays)
	// LLM adapter for env_recommend was wired earlier in runApp's
	// setup phase (where providersCfg is in scope) and reaches here
	// via env.SetLLMAdapter — REPL setup just confirms the
	// settings via SetEnvRecommendSettings.
	renderFn := func(busCtx *types.BusContext) string {
		return app.renderer.RenderResult(busCtx)
	}
	// PlanStore persistence directory: <runtime-anchor>/plans.
	// Mirror the blob session path anchoring so operators see the
	// write-mode plan files next to their logs and memory.
	runtimeAnchor := runtimeAnchorDir
	if abs, err := filepath.Abs(runtimeAnchor); err == nil {
		runtimeAnchor = abs
	}
	planStore := repl.NewPlanStore(filepath.Join(runtimeAnchor, "plans"))
	// Stage II: PlanGroupStore is the on-disk persister for
	// multi-phase PlanGroups. Same plan dir; lives under
	// /groups/ subdir. Wired into the orchestrator below so
	// runPhaseGroup can persist transitions.
	planGroupStore := repl.NewPlanGroupStore(filepath.Join(runtimeAnchor, "plans"))
	app.orch.SetPlanGroupStore(planGroupStore)
	// Stage II: per-phase ChangePlan persistence. Multi-phase
	// runs go through ModeApply, which bypasses the REPL's
	// ModePlan-only auto-save and the CLI's writePlanFile path,
	// so phase plans need their own hook into PlanStore.
	app.orch.SetPlanSaver(planStore)
	// FailureTaxonomyStore is now wired in initApp (commit 40)
	// so single-shot CLI flows also get cross-Run learning.
	var markdownPreview repl.MarkdownPreviewer
	if app.markdownPreviewConfig.Enabled() {
		srv, err := preview.NewServer(app.markdownPreviewConfig)
		if err != nil {
			logging.Warning("[markdown_preview] init failed; preview URLs disabled: %v", err)
		} else {
			markdownPreview = srv
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if err := srv.Close(ctx); err != nil {
					logging.Warning("[markdown_preview] shutdown failed: %v", err)
				}
			}()
		}
	}
	r := repl.New(repl.Config{
		Runner:             app.orch,
		Store:              store,
		Render:             renderFn,
		Renderer:           app.renderer,
		RepoRoot:           flagRepo,
		Branch:             flagBranch,
		Out:                os.Stdout,
		PasteFoldMinChars:  app.replPasteFoldMinChars,
		Version:            version,
		BuildTime:          buildTime,
		Language:           flagLang,
		HeaderPrinted:      app.replHeaderPrinted,
		ModelListLine:      app.replModelListLine,
		ModelSummaryLine:   app.replModelSummaryLine,
		MarkdownPreview:    markdownPreview,
		OutputDumpDir:      app.outputDumpDir,
		OutputDumpMax:      app.outputDumpMax,
		ChitchatResponder:  app.chitchatResponder,
		ChitchatClassifier: app.chitchatClassifier,
		OperationEnabled:   app.operationRouteEnabled,
		OperationProviders: append([]operation.ProviderInfo(nil), app.operationProviders...),
		// Hand the memory adapter to REPL so the chitchat tool-use
		// loop can call recall_memory without a separate wiring step.
		// The same adapter is also wired into the orchestrator above,
		// so pipeline and chitchat see one source of truth.
		Memory:                        memory.NewAdapter(store),
		EnvSettings:                   app.envRecommendSettings,
		ColorMode:                     render.ParseColorMode(flagColor),
		RuntimeAnchor:                 runtimeAnchor,
		WorktreeKeepTTL:               app.worktreeKeepTTL,
		WorktreeKeepMaxCount:          app.worktreeKeepMaxCount,
		PlanStore:                     planStore,
		PlanGroupStore:                planGroupStore,
		FailureTaxonomy:               app.orch.FailureTaxonomyStore(),
		AttachedLogMaxBytes:           maxAttachedLogBytes,
		AttachedTraceMaxBytes:         maxAttachedTraceBytes,
		WriteEnabled:                  app.writeEnabled,
		WriteAutoInitRepo:             app.writeAutoInitRepo,
		WriteScaffoldEnabled:          app.writeScaffoldEnabled,
		SettingsPath:                  app.settingsPath,
		Topology:                      app.topology,
		MultiRepoEnabled:              app.multiRepoEnabled,
		MultiRepoMaxActive:            app.multiRepoMaxActive,
		MultiRepoInactivePreviewCount: app.multiRepoInactivePreviewCount,
		// Phase 5 (2026-05-08): /repos listing reads the LRU-resident
		// slug set from the multigraph carrier through this handle so
		// auto-active rows can be color-marked. Stored as any to keep
		// the REPL package free of an internal/tool/repomap/multigraph
		// import.
		Multigraph: app.multigraph,
		// --focus CLI flag → REPL pre-pinned focus map. resolved
		// against the topology earlier (drops unmatched tokens with
		// a single Warning); single-repo / no-git workspaces have
		// no matching sub-repos so the slice is empty and the REPL
		// boots with the existing no-pin posture.
		InitialFocusSlugs: resolveFocusFlagAgainstTopology(app.topology, flagFocus),
		// Push REPL /repos mutations into the session-shared
		// MultiGraph so the next Run picks them up.
		OnMultiRepoFocusChange: func(slugs []string) {
			if mg, ok := app.multigraph.(interface{ SetFocus([]string) }); ok && mg != nil {
				mg.SetFocus(slugs)
			}
		},
		OnMultiRepoCapChange: func(n int) {
			if mg, ok := app.multigraph.(interface{ SetCap(int) }); ok && mg != nil {
				mg.SetCap(n)
			}
		},
		OnMultiRepoRefresh: func(newTopo *topology.RepoTopology) {
			// Rebuild MultiGraph from the freshly-discovered topology
			// so the next Run sees the new sub-repo set + a clean LRU.
			app.topology = newTopo
			if app.multiRepoEnabled {
				if mg, err := repomap.BuildOrLoadMultiGraph(newTopo, "", app.multiRepoMaxActive, nil, nil); err == nil {
					app.multigraph = mg
				}
			}
		},
	})
	if err := r.Loop(); err != nil {
		logging.Error("repl exited with error: %v", err)
		return err
	}
	return nil
}

// initApp implements the 3-tier config precedence and initializes all
// subsystems. It runs as PersistentPreRunE so it executes before any
// subcommand, including the implicit root run.
func initApp(cmd *cobra.Command, args []string) error {
	// Skip init for utility subcommands that do not need providers,
	// repo discovery, memory, or orchestrator state.
	if cmd.Name() == "version" || cmd.CommandPath() == "codrax trace convert" {
		return nil
	}

	// --- Phase 1: locate runtime settings file ---
	//
	// Lookup order (flat-first, exe-owned):
	//
	//   1. $CODRAX_SETTINGS (absolute override)
	//   2. <exeDir>/codrax.yaml               ← canonical new location
	//   3. <exeDir>/codrax/codrax.yaml        (bin + share split)
	//   4. <exeDir>/config/codrax.yaml        legacy; deprecation-warned
	//   5. <exeDir>/../config/codrax.yaml     legacy; deprecation-warned
	//   6. <CWD>/config/codrax.yaml           legacy; deprecation-warned
	//
	// Paths 4-6 stay functional so existing installs don't break; a
	// warning is emitted after logger init so operators see exactly
	// which file needs to move.
	exeDir := executableDir()
	settingsPath := ""
	settingsExplicit := false
	legacySettingsPath := ""
	if p := os.Getenv("CODRAX_SETTINGS"); p != "" {
		settingsPath = p
		settingsExplicit = true
	} else {
		type candidate struct {
			path   string
			legacy bool
		}
		var candidates []candidate
		if exeDir != "" {
			candidates = append(candidates,
				candidate{filepath.Join(exeDir, "codrax.yaml"), false},
				candidate{filepath.Join(exeDir, "codrax", "codrax.yaml"), false},
				candidate{filepath.Join(exeDir, "config", "codrax.yaml"), true},
				candidate{filepath.Join(exeDir, "..", "config", "codrax.yaml"), true},
			)
		}
		candidates = append(candidates, candidate{"config/codrax.yaml", true})
		for _, c := range candidates {
			if _, err := os.Stat(c.path); err == nil {
				settingsPath = c.path
				if c.legacy {
					legacySettingsPath = c.path
				}
				break
			}
		}
	}

	// --- Anchor resolution ---
	//
	// configAnchor roots configuration files (providers.yaml) to the
	// binary's directory. Falls back to CWD when os.Executable() is
	// unavailable (rare, e.g. some embedded scenarios).
	configAnchor := exeDir
	if configAnchor == "" {
		if cwd, err := os.Getwd(); err == nil {
			configAnchor = cwd
		}
	}
	// runtimeAnchor roots runtime artifacts to <CWD>/.codrax/. Absolute
	// so later joins are stable even if a tool chdir's (which codrax
	// doesn't do today, but the contract should still hold).
	runtimeAnchor := runtimeAnchorDir
	if abs, err := filepath.Abs(runtimeAnchor); err == nil {
		runtimeAnchor = abs
	}

	// Start with code defaults.
	mergedLogDir := defaultLogDir
	mergedLogLevel := defaultLogLevel
	mergedLogStdout := defaultLogStdout
	mergedMemoryDir := defaultMemoryDir
	mergedCacheDir := ""
	mergedLang := defaultLang
	mergedThinkingTruncate := defaultThinkingTruncate
	mergedRepo := defaultRepo
	mergedBranch := defaultBranch
	mergedMaxSteps := defaultMaxSteps
	mergedProvidersConfig := defaultProvidersConfig
	app.operationRouteEnabled = true

	// Overlay config file values.
	var rs *config.RuntimeSettings
	if settingsPath != "" {
		loaded, err := config.LoadRuntimeSettings(settingsPath)
		if err == nil {
			rs = loaded
		} else if !config.IsNotExist(err) || settingsExplicit {
			return fmt.Errorf("failed to load runtime settings from %s: %w", settingsPath, err)
		}
	}
	if rs != nil {
		// log_triage_source_prefix mirrors the --log-source-prefix CLI
		// flag for users who prefer persistent config. The CLI flag
		// still wins because rootRun applies it after initApp returns.
		// The log_triage_enabled knob is read later into LogTriageSettings
		// at agent construction, not here — the log_triager agent's
		// per-Execute gate reads settings.Enabled rather than a
		// package-level toggle.
		if rs.LogTriageSourcePrefix != nil && *rs.LogTriageSourcePrefix != "" {
			logtriage.SetSourcePrefix(*rs.LogTriageSourcePrefix)
		}
		// log_attach_max_bytes drives both loadAttachedLog (CLI / stdin)
		// and the REPL paste caps. Non-positive values (incl. explicit 0)
		// fall back to the default rather than degrading to zero-byte
		// truncation, which would silently break every /log path.
		if rs.LogAttachMaxBytes != nil && *rs.LogAttachMaxBytes > 0 {
			maxAttachedLogBytes = *rs.LogAttachMaxBytes
			if maxAttachedLogBytes > maxAttachedLogHardCeiling {
				logging.Warning("[cmd] log_attach_max_bytes=%d clamped to %d (hard ceiling)",
					maxAttachedLogBytes, maxAttachedLogHardCeiling)
				maxAttachedLogBytes = maxAttachedLogHardCeiling
			}
		}
		// Trace cap inheritance: by default the trace channel
		// shares the log cap (so a user who only sets
		// log_attach_max_bytes gets symmetric behaviour). When
		// trace_attach_max_bytes is set explicitly, it overrides
		// independently. Same hard-ceiling clamp applies.
		maxAttachedTraceBytes = maxAttachedLogBytes
		if rs.TraceAttachMaxBytes != nil && *rs.TraceAttachMaxBytes > 0 {
			maxAttachedTraceBytes = *rs.TraceAttachMaxBytes
			if maxAttachedTraceBytes > maxAttachedLogHardCeiling {
				logging.Warning("[cmd] trace_attach_max_bytes=%d clamped to %d (hard ceiling)",
					maxAttachedTraceBytes, maxAttachedLogHardCeiling)
				maxAttachedTraceBytes = maxAttachedLogHardCeiling
			}
		}
		if rs.LogDir != nil {
			mergedLogDir = *rs.LogDir
		}
		if rs.LogLevel != nil {
			mergedLogLevel = *rs.LogLevel
		}
		if rs.LogStdout != nil {
			mergedLogStdout = *rs.LogStdout
		}
		if rs.MemoryDir != nil {
			mergedMemoryDir = *rs.MemoryDir
		}
		if rs.CacheDir != nil {
			mergedCacheDir = *rs.CacheDir
		}
		if rs.Lang != nil {
			mergedLang = *rs.Lang
		}
		if rs.ThinkingTruncate != nil {
			mergedThinkingTruncate = *rs.ThinkingTruncate
		}
		if rs.Repo != nil {
			mergedRepo = *rs.Repo
		}
		if rs.Branch != nil {
			mergedBranch = *rs.Branch
		}
		if rs.PipelineMaxSteps != nil {
			mergedMaxSteps = *rs.PipelineMaxSteps
		}
		if rs.ProvidersConfig != nil {
			mergedProvidersConfig = *rs.ProvidersConfig
		}
		if rs.ReplPasteFoldMinChars != nil {
			app.replPasteFoldMinChars = *rs.ReplPasteFoldMinChars
		}
		if rs.OperationRouteEnabled != nil {
			app.operationRouteEnabled = *rs.OperationRouteEnabled
		}
	}

	// Anchor relative paths.
	//
	// Runtime artifacts go under runtimeAnchor (<CWD>/.codrax/):
	// log_dir, memory_dir, cache_dir. Configuration files go under
	// configAnchor (exeDir): providers_config. Absolute YAML overrides
	// pass through anchorPath unchanged.
	mergedLogDir = anchorPath(runtimeAnchor, mergedLogDir)
	mergedMemoryDir = anchorPath(runtimeAnchor, mergedMemoryDir)
	if mergedCacheDir != "" {
		mergedCacheDir = anchorPath(runtimeAnchor, mergedCacheDir)
	}
	mergedProvidersConfig = anchorPath(configAnchor, mergedProvidersConfig)

	// Per-target-repo namespacing.
	logDirBase := mergedLogDir
	memoryDirBase := mergedMemoryDir
	if slug := repoSlug(mergedRepo); slug != "" {
		mergedLogDir = filepath.Join(logDirBase, slug)
		mergedMemoryDir = filepath.Join(memoryDirBase, slug)
	}

	// --- Phase 2: apply CLI flag overrides (3rd tier) ---
	// Only override if the flag was explicitly set on the command line.
	if !cmd.Flags().Changed("providers") {
		flagProviders = mergedProvidersConfig
	}
	if !cmd.Flags().Changed("repo") {
		flagRepo = mergedRepo
	}
	if !cmd.Flags().Changed("branch") {
		flagBranch = mergedBranch
	}
	if !cmd.Flags().Changed("pipeline-max-steps") {
		flagMaxSteps = mergedMaxSteps
	}
	if !cmd.Flags().Changed("log-dir") {
		flagLogDir = mergedLogDir
	}
	if !cmd.Flags().Changed("log-level") {
		flagLogLevel = mergedLogLevel
	}
	if !cmd.Flags().Changed("log-stdout") {
		flagLogStdout = mergedLogStdout
	}
	if !cmd.Flags().Changed("memory-dir") {
		flagMemoryDir = mergedMemoryDir
	}
	if !cmd.Flags().Changed("cache-dir") {
		flagCacheDir = mergedCacheDir
	}
	if !cmd.Flags().Changed("lang") {
		flagLang = mergedLang
	}

	// Re-slug if -repo was overridden but log/memory dirs were not.
	if cmd.Flags().Changed("repo") {
		if slug := repoSlug(flagRepo); slug != "" {
			if !cmd.Flags().Changed("log-dir") {
				flagLogDir = filepath.Join(logDirBase, slug)
			}
			if !cmd.Flags().Changed("memory-dir") {
				flagMemoryDir = filepath.Join(memoryDirBase, slug)
			}
		}
	}

	// --- Phase 3: initialize subsystems ---
	//
	// Create the runtime anchor up-front so the logger + memory store
	// can rely on their parent directory existing. MkdirAll is idempotent
	// so repeated invocations in the same CWD are free.
	if err := os.MkdirAll(runtimeAnchor, 0o755); err != nil {
		return fmt.Errorf("create runtime anchor %s: %w", runtimeAnchor, err)
	}

	// If the runtime anchor is `<repo>/.codrax/` AND the surrounding
	// directory is a git repo, make sure `.gitignore` excludes the
	// runtime artifacts. Without this, the user's next `git add -A`
	// sweeps logs / memory / blob caches / worktrees / plans into
	// version control — observed in the field. Best-effort: any
	// failure (no git, .gitignore unwritable, parent isn't a repo)
	// downgrades to a debug log so a non-git workspace doesn't
	// produce noise. Idempotent: EnsureCodraxGitignore only appends
	// missing entries.
	parentDir := filepath.Dir(runtimeAnchor)
	if state, derr := worktree.DetectRepoState(parentDir); derr == nil && state == worktree.RepoReady {
		if gerr := worktree.EnsureCodraxGitignore(parentDir); gerr != nil {
			logging.Debug("[cmd] ensure .gitignore in %s: %v", parentDir, gerr)
		}
	}

	// Apply log retention override before constructing the logger so
	// its startup sweep (pruneOldFiles) honors the configured cap.
	if rs != nil && rs.LogMaxFiles != nil {
		logging.SetMaxTotalFiles(*rs.LogMaxFiles)
	}

	logger, err := logging.NewFromFlags(flagLogDir, flagLogLevel, flagLogStdout)
	if err != nil {
		return fmt.Errorf("logging init failed: %w", err)
	}
	app.logger = logger
	logging.SetDefault(logger)
	log.SetOutput(logger.InfoWriter())
	log.SetFlags(0)

	if legacySettingsPath != "" {
		logging.Warning("runtime settings loaded from legacy path %s; move to <exeDir>/codrax.yaml to silence this warning", legacySettingsPath)
	}

	// Set up the blob session directory. With blob_max_sessions > 0
	// (the default, matching log retention), each codrax process owns
	// <CWD>/.codrax/blob/<timestamp>-<pid>/ for the lifetime of the
	// process; the orchestrator injects this into BusContext.WorkDir
	// on every Run so tool blobs land in one grep-able tree. A config
	// of 0 disables the layout and lets the orchestrator fall back to
	// the historical per-trace os.MkdirTemp + RemoveAll behavior.
	blobMaxSessions := defaultBlobMaxSessions
	if rs != nil && rs.BlobMaxSessions != nil {
		blobMaxSessions = *rs.BlobMaxSessions
	}
	blobSessionDir := ""
	if blobMaxSessions > 0 {
		blobBase := filepath.Join(runtimeAnchor, blobSubdir)
		if err := os.MkdirAll(blobBase, 0o755); err != nil {
			logging.Warning("create blob base %s failed: %v (falling back to per-trace tmpdir)", blobBase, err)
		} else {
			tool.PruneBlobSessions(blobBase, blobMaxSessions)
			blobSessionDir = filepath.Join(blobBase, tool.SessionDirName())
			if err := os.MkdirAll(blobSessionDir, 0o755); err != nil {
				logging.Warning("create blob session %s failed: %v (falling back to per-trace tmpdir)", blobSessionDir, err)
				blobSessionDir = ""
			}
		}
	}

	// Final-answer transcript dump. Resolves alongside the blob layout
	// so a future operator who greps for ".codrax/" sees both artefact
	// kinds together. outputDumpDir is empty when the feature is gated
	// off (output_dump_enabled: false); the orchestrator no-ops when
	// it sees an empty dir. Failures here only WARN — the dump must
	// never block startup.
	outputDumpEnabled := defaultOutputDumpOn
	if rs != nil && rs.OutputDumpEnabled != nil {
		outputDumpEnabled = *rs.OutputDumpEnabled
	}
	outputMaxFiles := defaultOutputMaxFiles
	if rs != nil && rs.OutputMaxFiles != nil && *rs.OutputMaxFiles > 0 {
		outputMaxFiles = *rs.OutputMaxFiles
	}
	outputDumpDir := ""
	if outputDumpEnabled {
		outputDumpDir = filepath.Join(runtimeAnchor, outputSubdir)
	}
	markdownPreviewCfg := preview.Config{Mode: preview.ModeAuto}
	if rs != nil {
		if rs.MarkdownPreviewServer != nil {
			markdownPreviewCfg.Mode = *rs.MarkdownPreviewServer
		}
		if rs.MarkdownPreviewHost != nil {
			markdownPreviewCfg.Host = *rs.MarkdownPreviewHost
		}
		if rs.MarkdownPreviewPort != nil {
			markdownPreviewCfg.Port = *rs.MarkdownPreviewPort
		}
	}
	if err := preview.ValidateConfig(markdownPreviewCfg); err != nil {
		logging.Warning("[markdown_preview] invalid config; preview URLs disabled: %v", err)
		markdownPreviewCfg.Mode = preview.ModeOff
	}
	markdownPreviewCfg = preview.NormalizeConfig(markdownPreviewCfg)
	if !outputDumpEnabled && markdownPreviewCfg.Enabled() {
		logging.Warning("[markdown_preview] disabled because output_dump_enabled=false")
		markdownPreviewCfg.Mode = preview.ModeOff
	}
	app.markdownPreviewConfig = markdownPreviewCfg
	app.outputDumpDir = outputDumpDir
	app.outputDumpMax = outputMaxFiles

	// Resolve --repo to an absolute path before any tool / orchestrator
	// reads it. Tools key file-system access off BusContext.RepoRoot, so
	// a literal "." here would silently fall through to the process CWD
	// at every Execute (codrax's own dir, not the user's --repo target).
	// EvalSymlinks normalizes module-cache paths so two -repo invocations
	// pointing at the same content produce the same canonical RepoRoot.
	if abs, err := filepath.Abs(flagRepo); err == nil {
		if resolved, err2 := filepath.EvalSymlinks(abs); err2 == nil {
			flagRepo = resolved
		} else {
			flagRepo = abs
		}
	}
	repomap.SetCacheDir(flagCacheDir)
	logging.Info("paths: repo=%s log-dir=%s memory-dir=%s cache-dir=%s blob-session=%s", flagRepo, flagLogDir, flagMemoryDir, flagCacheDir, blobSessionDir)

	// Large-repo memory resilience. Install a soft heap limit so a
	// repomap full scan of a very large repository pressures the GC
	// before host RAM is exhausted, and let an interrupted scan resume
	// from chunks it already wrote rather than restarting from zero.
	// See docs/design/large_repo_memory_resilience.md.
	heapLimitCfg := memlimit.Config{Enabled: true, Fraction: 0.8}
	resumeInterruptedScan := true
	if rs != nil {
		if rs.MemorySoftLimitEnabled != nil {
			heapLimitCfg.Enabled = *rs.MemorySoftLimitEnabled
		}
		if rs.MemorySoftLimitFraction != nil {
			heapLimitCfg.Fraction = *rs.MemorySoftLimitFraction
		}
		if rs.MemorySoftLimitBytes != nil && *rs.MemorySoftLimitBytes > 0 {
			heapLimitCfg.ExplicitBytes = *rs.MemorySoftLimitBytes
		}
		if rs.RepomapResumeInterruptedScan != nil {
			resumeInterruptedScan = *rs.RepomapResumeInterruptedScan
		}
	}
	logging.Info("memory: %s", memlimit.Apply(heapLimitCfg))
	repomap.SetResumeInterruptedScan(resumeInterruptedScan)

	// Scan CPU headroom. A repomap full scan is CPU-bound across every
	// core; on a small remote host that can starve sshd and drop the
	// operator's SSH session. RepomapScanReserveCPUs caps GOMAXPROCS
	// during a scan so the entire Go runtime (parse, GC, graph build)
	// leaves that many cores free for interactive processes. Default 0
	// (opt-in); operators on small remote hosts set it to 1. See
	// docs/design/large_repo_memory_resilience.md.
	scanReserveCPUs := 0
	if rs != nil && rs.RepomapScanReserveCPUs != nil {
		scanReserveCPUs = *rs.RepomapScanReserveCPUs
	}
	if scanReserveCPUs < 0 {
		scanReserveCPUs = 0
	}
	repomap.SetScanReserveCPUs(scanReserveCPUs)
	logging.Info("cpu: repomap scan reserves %d core(s) (GOMAXPROCS cap)", scanReserveCPUs)
	parseTimeoutEnabled := true
	parseTimeout := 120 * time.Second
	if rs != nil {
		if rs.RepomapParseTimeoutEnabled != nil {
			parseTimeoutEnabled = *rs.RepomapParseTimeoutEnabled
		}
		if rs.RepomapParseTimeoutSeconds != nil {
			if *rs.RepomapParseTimeoutSeconds <= 0 {
				parseTimeout = 0
			} else {
				parseTimeout = time.Duration(*rs.RepomapParseTimeoutSeconds) * time.Second
			}
		}
	}
	if !parseTimeoutEnabled {
		parseTimeout = 0
	}
	repomap.SetTreeSitterParseTimeout(parseTimeout)
	if parseTimeout > 0 {
		logging.Info("repo_map: tree-sitter parse timeout per file is %s", parseTimeout)
	} else {
		logging.Info("repo_map: tree-sitter parse timeout per file is disabled")
	}

	// Multi-repo discovery. Always runs — the §3.3.1 fast path keeps
	// single-repo cost at ~50µs (one os.Stat for flagRepo/.git, one
	// sha256 over the abs path). Phase 4 wires the result into
	// BusContext / multigraph routing — until then this is a passive
	// snapshot consumed by the REPL `/repos` command. Depth / MinFiles
	// honour codrax.yaml :: multi_repo_discovery_depth and
	// :: multi_repo_min_files (see internal/config/runtime.go); the
	// `multi_repo_enabled` knob is read by Phase 4 wiring, not here —
	// Discovery itself is unconditional (its product is informational
	// to the REPL `/repos` command even when routing is disabled).
	topoDepth := 4
	topoMinFiles := 1
	if rs != nil {
		if rs.MultiRepoDiscoveryDepth != nil && *rs.MultiRepoDiscoveryDepth > 0 {
			topoDepth = *rs.MultiRepoDiscoveryDepth
		}
		if rs.MultiRepoMinFiles != nil && *rs.MultiRepoMinFiles > 0 {
			topoMinFiles = *rs.MultiRepoMinFiles
		}
	}
	topoOpts := topology.Options{
		Depth:         topoDepth,
		MinFiles:      topoMinFiles,
		RuntimeAnchor: runtimeAnchor,
	}
	// Resolve enabled + cap flags onto app — REPL Config and Phase 4
	// multigraph wiring both read these. Default true / 3.
	app.multiRepoEnabled = true
	app.multiRepoMaxActive = config.MultiRepoMaxActiveDefault
	app.multiRepoInactivePreviewCount = config.MultiRepoInactivePreviewCountDefault
	if rs != nil {
		if rs.MultiRepoEnabled != nil {
			app.multiRepoEnabled = *rs.MultiRepoEnabled
		}
		if rs.MultiRepoMaxActive != nil {
			// ClampMultiRepoMaxActive enforces the hard ceiling
			// (config.MultiRepoMaxActiveCeiling, 3) even if yaml
			// requests a higher value. Returns the default when
			// the yaml value is non-positive.
			app.multiRepoMaxActive = config.ClampMultiRepoMaxActive(*rs.MultiRepoMaxActive)
		}
		if rs.MultiRepoInactivePreviewCount != nil {
			// Same clamp pattern: hard ceiling
			// MultiRepoInactivePreviewCountCeiling (3).
			app.multiRepoInactivePreviewCount = config.ClampMultiRepoInactivePreviewCount(*rs.MultiRepoInactivePreviewCount)
		}
	}
	// CLI override — only applied when the operator passed --multi-repo
	// explicitly. cmd.Flags().Changed("multi-repo") distinguishes the
	// "not passed" case from the BoolVar's default false. Logged so the
	// operator can confirm the override took effect.
	if cmd.Flags().Changed("multi-repo") {
		app.multiRepoEnabled = flagMultiRepoEnabled
		logging.Info("[multi-repo] CLI override: enabled=%t (--multi-repo flag)", flagMultiRepoEnabled)
	}
	parentSlug := repomapindex.CacheDirSlug(flagRepo)
	finishTopoNotice := startTopologyStartupNotice(strings.TrimSpace(flagRequest) == "" && len(args) == 0, flagLang)
	topoStart := time.Now()
	topo, topoErr := topology.LoadOrDiscover(flagRepo, runtimeAnchor, topoOpts, parentSlug)
	finishTopoNotice(topo, topoErr, time.Since(topoStart))
	if topoErr == nil && topo != nil {
		app.topology = topo
		// One INFO line summarising what we found. Single-repo
		// (RootRel == ".") gets a terse form; multi-repo lists count
		// and the top sub-repo names so operators can sanity-check
		// the BFS at a glance without enabling the REPL slash command.
		if topo.IsSingle() {
			logging.Info("multi-repo: single-repo mode parent=%s slug=%s", topo.ParentRoot, topo.ParentSlug)
		} else {
			names := make([]string, 0, len(topo.Repos))
			for _, sr := range topo.Repos {
				names = append(names, sr.RootRel)
			}
			preview := names
			if len(preview) > 5 {
				preview = preview[:5]
			}
			logging.Info("multi-repo: discovered %d sub-repos under %s preview=%v", len(topo.Repos), topo.ParentRoot, preview)
		}
	} else if topoErr != nil {
		// Discovery should never fail for a well-formed flagRepo, but
		// surface any non-nil error rather than swallowing — fail-loud.
		logging.Info("multi-repo: discovery error for %s: %v (falling back to single-repo behaviour)", flagRepo, topoErr)
	}

	// Build the session-shared *MultiGraph carrier when topology is
	// populated AND multi_repo_enabled (default true). nil here =
	// orchestrator falls back to legacy single-graph behaviour.
	//
	// Single- and multi-repo repomap builds now use the same typed
	// scan-progress channel. In multi-repo mode the app layer enriches
	// the event with RootRel so the renderer can say "sub-repo index"
	// without MultiGraph also emitting its older start/end wrapper
	// lines. That keeps one visible source of truth for scan progress:
	// start/done in permanent scrollback, live counts in the status row.
	repomap.SetScanNotifier(func(ev types.RepoMapScanEvent) {
		if app.orch == nil {
			return
		}
		ev = annotateRepoMapScanEvent(ev)
		app.orch.EmitRepoMapScanNotice(ev)
	})
	if app.topology != nil && app.multiRepoEnabled {
		// Resolve --focus inputs (slug-or-path) against the topology.
		// Single-repo / no-git workspaces have no sub-repo to match
		// so the resolved slug list stays empty and the carrier
		// builds with focusSlugs=nil — same posture as no-flag boot.
		focusSlugs := resolveFocusFlagAgainstTopology(app.topology, flagFocus)
		mg, err := repomap.BuildOrLoadMultiGraph(
			app.topology, "", app.multiRepoMaxActive, focusSlugs,
			nil,
		)
		if err != nil {
			logging.Info("multi-repo: BuildOrLoadMultiGraph failed (%v) — falling back to single-graph", err)
		} else {
			app.multigraph = mg
		}
	}

	// B0 write-mode startup housekeeping.
	//
	// PruneDeadSessions reaps worktree directories under
	// <runtime-anchor>/worktrees/ whose embedded PID is no longer a
	// live process. Covers the SIGKILL path where the outer
	// orchestrator defer never ran. Missing base dir is tolerated
	// silently; read-mode users who never exercise write-mode see
	// zero side effect (no empty directory ever created).
	//
	// InstallSignalHandler adds a process-wide SIGINT/SIGTERM
	// handler that walks the worktree package's active-sessions
	// registry, calls Discard on each, then re-raises the signal so
	// the process still exits with the canonical signal exit code.
	// Idempotent — calling twice is a no-op. Install once at
	// startup, before any Run() has a chance to create a worktree.
	worktreeBase := filepath.Join(runtimeAnchor, "worktrees")
	worktree.PruneDeadSessions(worktreeBase, flagRepo)
	// T3.1 TTL + quota reaping for preserved (keep-on-success)
	// worktrees. Runs AFTER PruneDeadSessions so the dead-PID
	// orphans are already gone — leaving only kept dirs whose
	// mtime / count we want to bound. yaml defaults: 168 hours
	// (7 days) TTL, 20-count quota. Zero on either knob disables
	// that gate; explicit zero from yaml is honoured.
	keepTTL := time.Duration(defaultWorktreeKeepTTLHours) * time.Hour
	keepMaxCount := defaultWorktreeKeepMaxCount
	if rs != nil {
		if rs.WorktreeKeepTTLHours != nil {
			keepTTL = time.Duration(*rs.WorktreeKeepTTLHours) * time.Hour
		}
		if rs.WorktreeKeepMaxCount != nil {
			keepMaxCount = *rs.WorktreeKeepMaxCount
		}
	}
	worktree.PruneByAgeAndQuota(worktreeBase, flagRepo, keepTTL, keepMaxCount)
	// Persist resolved values on app so runREPL → REPL Config can
	// thread them into /worktree gc without re-resolving the yaml.
	app.worktreeKeepTTL = keepTTL
	app.worktreeKeepMaxCount = keepMaxCount
	worktree.InstallSignalHandler()

	// One-shot environment probe: logs which external binaries
	// (rg/grep, sh/bash/cmd, git) were actually found. Each miss
	// carries a platform-specific install hint so operators do not
	// have to dig into runtime errors. Never aborts startup.
	tool.LogCapabilities()

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
	if app.defaultLLM != nil {
		ctxWindow = app.defaultLLM.MaxContextTokens()
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
		// CLI override wins last (precedence: code default → yaml →
		// CLI). 0 means "not set" (cobra default), so we honor only
		// positive values to avoid a zero flag silently disabling the
		// pre-scan loop.
		if cmd.Flags().Changed("max-prescan-rounds") && flagMaxPrescanRounds > 0 {
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
		// R2.2 (post_shape_residual_audit.md, 2026-05-04) finalizer-
		// local retry budget for the fallback picker downgrade.
		if rs.PipelineFinalizerLocalRetriesBeforeEscalate != nil {
			orchestrator.SetFinalizerLocalRetryBudget(*rs.PipelineFinalizerLocalRetriesBeforeEscalate)
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
		// Write-mode wall-time cap (commit 2 P0 C). Default 600 s;
		// 0 disables the cap; hard-capped at 1800 inside the orch
		// setter.
		pipelineSettings.WriteMaxSeconds = 600
		if rs.PipelineWriteMaxSeconds != nil {
			pipelineSettings.WriteMaxSeconds = *rs.PipelineWriteMaxSeconds
		}
		if rs.PipelinePlanCriticEnabled != nil {
			pipelineSettings.PlanCriticEnabled = *rs.PipelinePlanCriticEnabled
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
		// package default (2048 MiB / 600s); see SetVerifyResourceCaps.
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
	if cmd.Flags().Changed("pipeline-max-retries") && flagMaxRetries > 0 {
		pipelineSettings.MaxRetriesPerStage = flagMaxRetries
	}
	if cmd.Flags().Changed("pipeline-max-stage-visits") && flagMaxStageVisits > 0 {
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

	// Memory store limits from YAML.
	var memCfg types.MemorySettings
	if rs != nil {
		if rs.MemoryMaxRecentTurns != nil {
			memCfg.MaxRecentTurns = *rs.MemoryMaxRecentTurns
		}
		if rs.MemoryMaxRecentBytes != nil {
			memCfg.MaxRecentBytes = *rs.MemoryMaxRecentBytes
		}
		if rs.MemoryMaxTurnBodyBytes != nil {
			memCfg.MaxTurnBodyBytes = *rs.MemoryMaxTurnBodyBytes
		}
		if rs.MemoryMaxBuildContextMatches != nil {
			memCfg.MaxBuildContextMatches = *rs.MemoryMaxBuildContextMatches
		}
		if rs.MemoryEntityMinRunes != nil {
			memCfg.EntityMinRunes = *rs.MemoryEntityMinRunes
		}
		if rs.MemorySessionTieBreakerBonus != nil {
			memCfg.SessionTieBreakerBonus = *rs.MemorySessionTieBreakerBonus
		}
		if rs.MemorySearchMaxLimit != nil {
			memCfg.SearchMaxLimit = *rs.MemorySearchMaxLimit
		}
		if rs.MemoryListMaxLimit != nil {
			memCfg.ListMaxLimit = *rs.MemoryListMaxLimit
		}
		// Per-Kind retrieval policy override. Only allocate the
		// MemoryRetrievalPolicy struct when at least one sub-policy
		// is supplied — keeps memCfg.Policy nil for the typical
		// operator who never touches these knobs.
		if rs.MemoryPolicyChitchat != nil || rs.MemoryPolicyShell != nil ||
			rs.MemoryPolicyPipeline != nil || rs.MemoryPolicyPlan != nil ||
			rs.MemoryPolicyDefault != nil {
			memCfg.Policy = &types.MemoryRetrievalPolicy{
				Chitchat: rs.MemoryPolicyChitchat,
				Shell:    rs.MemoryPolicyShell,
				Pipeline: rs.MemoryPolicyPipeline,
				Plan:     rs.MemoryPolicyPlan,
				Default:  rs.MemoryPolicyDefault,
			}
		}
	}
	app.memorySettings = types.ResolvedMemorySettings(memCfg)

	// env_recommend settings — Stage 1 (deterministic table) is on
	// by default; LLM fallback is also on by default (cheap-model
	// routed, can be disabled via yaml). Operators with strict
	// audit requirements can `env_recommend_enabled: false` to
	// fully disable and fall back to legacy runnerInstallHint.
	envCfg := types.DefaultEnvRecommendSettings()
	if rs != nil {
		if rs.EnvRecommendEnabled != nil {
			envCfg.Enabled = *rs.EnvRecommendEnabled
		}
		if rs.EnvRecommendLLMEnabled != nil {
			envCfg.LLMEnabled = *rs.EnvRecommendLLMEnabled
		}
		if rs.EnvRecommendLLMTimeoutSec != nil {
			envCfg.LLMTimeoutSec = *rs.EnvRecommendLLMTimeoutSec
		}
		if rs.RecommendGlobalInstall != nil {
			envCfg.RecommendGlobalInstall = *rs.RecommendGlobalInstall
		}
		if rs.EnvProbeNetwork != nil {
			envCfg.ProbeNetwork = *rs.EnvProbeNetwork
		}
		if rs.EnvCacheTTLDays != nil {
			envCfg.CacheTTLDays = *rs.EnvCacheTTLDays
		}
	}
	app.envRecommendSettings = types.ResolvedEnvRecommendSettings(envCfg)
	logging.Info("env_recommend: enabled=%t llm=%t timeout=%ds global_install=%t probe_network=%t cache_ttl=%dd",
		app.envRecommendSettings.Enabled, app.envRecommendSettings.LLMEnabled,
		app.envRecommendSettings.LLMTimeoutSec, app.envRecommendSettings.RecommendGlobalInstall,
		app.envRecommendSettings.ProbeNetwork, app.envRecommendSettings.CacheTTLDays)

	logging.Info("pipeline_settings: max_steps=%d max_retries_per_stage=%d max_stage_visits=%d",
		flagMaxSteps,
		pipelineSettings.MaxRetriesPerStage,
		pipelineSettings.MaxStageVisits)
	logging.Info("blob_limits: max_inline_bytes=%d preview_head_bytes=%d preview_tail_bytes=%d",
		tool.MaxInlineBytes, tool.PreviewHeadBytesValue(), tool.PreviewTailBytesValue())
	al := tool.CurrentAnalysisLimits()
	logging.Info("analysis_limits: warn_below_keywords=%d reject_below_keywords=%d generic_entity_blocklist=%d reject_multiple_emit=%t max_prescan_rounds=%d emit_only_correction_retries=%d warn_kw_hit_ratio=%.2f warn_ent_hit_ratio=%.2f",
		al.WarnBelowKeywords, al.RejectBelowKeywords, len(al.GenericEntityBlocklist), al.RejectMultipleEmit, al.MaxPrescanRounds, al.EmitOnlyCorrectionRetries,
		al.WarnBelowKeywordHitRatio, al.WarnBelowEntityHitRatio)

	providersCfg, err := config.LoadProviders(flagProviders)
	if err != nil {
		return fmt.Errorf("failed to load providers config: %w", err)
	}
	anchorProviderTLSCAFiles(providersCfg, configAnchor)
	llm.SetDisplayLanguage(flagLang)
	interactiveStartup := strings.TrimSpace(flagRequest) == "" && len(args) == 0
	installLLMStartupUXHooks(interactiveStartup)
	app.defaultLLM, err = createDefaultAdapter(providersCfg, flagProviders)
	if err != nil {
		return err
	}
	if interactiveStartup {
		modelNotices := llm.ConsumeModelSelectionNotices()
		autoSelected := len(modelNotices) > 0
		app.replModelListLine = replModelListLine(flagLang, modelNotices)
		app.replModelSummaryLine = replModelSummaryLine(flagLang, app.defaultLLM, autoSelected)
	}
	// Surface the resolved context-window value at INFO level so
	// operators can sanity-check their providers.yaml declaration
	// against the actual adapter, and notice when context_window was
	// omitted (adapter reports the 128K fallback). Downstream budget
	// resolvers (fraction-form caps) and the pressure watchdog all
	// consume this same value, so the startup log is the canonical
	// "what codrax believes the model can take" breadcrumb.
	if app.defaultLLM != nil {
		logging.Info("[llm] default adapter: model=%s context_window=%d tokens %s timeout=%s retry=%d",
			app.defaultLLM.ModelID(),
			app.defaultLLM.MaxContextTokens(),
			formatOutputCap(app.defaultLLM.MaxOutputTokens()),
			app.defaultLLM.RequestTimeout(),
			app.defaultLLM.RetryMaxAttempts())
	}
	toolParamCompatByAgent, err := resolveToolParamCompatByAgent(providersCfg)
	if err != nil {
		return err
	}

	// Memory summarizer adapter routing. When providers.yaml carries
	// an explicit `agents.memory_summarizer` entry, route the one-shot
	// memory compaction LLM call to its (typically cheaper) model;
	// otherwise reuse app.defaultLLM so the pre-session-30 behaviour
	// is preserved byte-for-byte. This is a pure cost knob — every
	// summarizer failure path falls back to the heuristic IndexEntry,
	// so a mis-routed or slow cheap model at worst reverts memory
	// quality to the fallback, it cannot break the REPL.
	app.memorySummarizerLLM = app.defaultLLM
	if _, ok := providersCfg.LLM.Agents["memory_summarizer"]; ok {
		resolved := config.ResolveProvider(providersCfg, "memory_summarizer")
		if adapter, err := llm.NewFromConfig(resolved); err != nil {
			logging.Warning("[memory] summarizer adapter init failed; falling back to default LLM: %v", err)
		} else {
			app.memorySummarizerLLM = adapter
		}
	}

	// Reflector adapter routing + timeout shortening. The reflector
	// is a side-channel critic dispatched between verify failure and
	// re-plan; clearForReplan already gracefully degrades to the
	// heuristic hint on any reflector error. Failure is non-fatal —
	// what we MUST avoid is the reflector blocking the retry loop
	// for the full 120 s default HTTP timeout when the upstream LLM
	// stalls. Observed in eval (pig-latin attempt 2): reflector
	// waited ~2.5 min on a stalled Chat call before
	// http.Client.Timeout fired, burning a full retry slot for zero
	// signal. clampReflectorTimeout enforces 30 s default — 5-10×
	// headroom over a typical fast critique (1-3 s) while keeping
	// retry-loop wall time bounded.
	app.reflectorLLM = app.defaultLLM
	{
		_, hasExplicit := providersCfg.LLM.Agents["reflector"]
		resolved := config.ResolveProvider(providersCfg, "reflector")
		resolved.RequestTimeoutSeconds = clampReflectorTimeout(resolved.RequestTimeoutSeconds, hasExplicit)
		if adapter, err := llm.NewFromConfig(resolved); err != nil {
			logging.Warning("[reflector] adapter init failed; falling back to default LLM (will inherit %ds timeout): %v",
				int(app.defaultLLM.RequestTimeout().Seconds()), err)
		} else {
			app.reflectorLLM = adapter
		}
	}

	// Plan critic adapter routing. Mirrors reflector — opt-in side
	// channel that gets a clamped timeout so a slow LLM cannot
	// stall the apply path. Gated by pipeline_plan_critic_enabled
	// downstream; the adapter is resolved unconditionally so
	// flipping the yaml gate doesn't require a config restart.
	app.planCriticLLM = app.defaultLLM
	{
		_, hasExplicit := providersCfg.LLM.Agents["plan_critic"]
		resolved := config.ResolveProvider(providersCfg, "plan_critic")
		resolved.RequestTimeoutSeconds = clampReflectorTimeout(resolved.RequestTimeoutSeconds, hasExplicit)
		if adapter, err := llm.NewFromConfig(resolved); err != nil {
			logging.Warning("[plan_critic] adapter init failed; falling back to default LLM (will inherit %ds timeout): %v",
				int(app.defaultLLM.RequestTimeout().Seconds()), err)
		} else {
			app.planCriticLLM = adapter
		}
	}

	// Acceptance checker adapter routing (commit 19, stage II).
	// Mirrors plan_critic — opt-in side channel that runs once
	// per multi-phase phase to judge "did this phase satisfy
	// its goal". Always resolved (cheap fallback to default LLM)
	// because the cost is paid only inside multi-phase Runs;
	// installing unconditionally is safe.
	app.acceptanceCheckerLLM = app.defaultLLM
	{
		_, hasExplicit := providersCfg.LLM.Agents["acceptance_checker"]
		resolved := config.ResolveProvider(providersCfg, "acceptance_checker")
		resolved.RequestTimeoutSeconds = clampReflectorTimeout(resolved.RequestTimeoutSeconds, hasExplicit)
		if adapter, err := llm.NewFromConfig(resolved); err != nil {
			logging.Warning("[acceptance_checker] adapter init failed; falling back to default LLM (will inherit %ds timeout): %v",
				int(app.defaultLLM.RequestTimeout().Seconds()), err)
		} else {
			app.acceptanceCheckerLLM = adapter
		}
	}

	// Initialize registries.
	mcpRegistry := mcp.NewRegistry()
	if rs != nil && len(rs.MCPServers) > 0 {
		maxServers := mcp.DefaultMaxServers
		if rs.MCPMaxServers != nil && *rs.MCPMaxServers > 0 {
			maxServers = *rs.MCPMaxServers
		}
		if err := mcp.LoadServers(mcpRegistry, rs.MCPServers, maxServers); err != nil {
			_ = mcpRegistry.Close()
			return fmt.Errorf("load mcp servers: %w", err)
		}
		worktree.RegisterSignalCleanupHook(func() { _ = mcpRegistry.Close() })
	}
	app.mcpRegistry = mcpRegistry
	app.operationProviders = operationProvidersFromMCPConfigs(mcpRegistry, rsMCPServers(rs))
	logging.Info("registered %d MCP servers", len(mcpRegistry.List()))

	skillRegistry := skill.NewRegistry()
	skill.RegisterDefaults(skillRegistry)
	logging.Info("registered %d skills", len(skillRegistry.List()))

	renderer := render.New(os.Stdout, false)
	renderer.SetLang(flagLang)
	renderer.SetThinkingTruncate(mergedThinkingTruncate)
	// log_stdout + TTY conflict: when logger is mirrored to stdout
	// AND stdout is a terminal, the dock's 3-row anchor would tear
	// every time the logger writes. Force dock-disabled so events
	// degrade to the non-TTY commit path; the user keeps full log
	// visibility at the cost of the live dock.
	if flagLogStdout {
		renderer.SetDockEnabled(false)
		logging.Info("[render] dock disabled because --log-stdout writes to the same stream")
	}
	app.renderer = renderer
	if strings.TrimSpace(flagRequest) == "" && len(args) == 0 {
		printMCPRegistryStartupNotice(mcpRegistry, flagLang)
	}

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

	subAgentRegistry := agent.NewSubAgentRegistry()

	agentCfg := pipelineSettings.Agent
	deps := &agent.Dependencies{
		LLM:                    app.defaultLLM,
		Tools:                  toolRegistry,
		MCPServers:             mcpRegistry,
		SubAgents:              subAgentRegistry,
		Skills:                 skillRegistry,
		MaxIterations:          agentCfg.MaxIterations,
		Emit:                   renderer.Emitter(),
		ExploreHeuristics:      pipelineSettings.Explore.Heuristics,
		AgentSettings:          agentCfg,
		ToolParamCompatByAgent: toolParamCompatByAgent,
		LoopPolicy:             loopPolicyFromAgentSettings(agentCfg),
	}

	agent.RegisterDefaultSubAgents(subAgentRegistry, deps)
	logging.Info("registered %d tools, %d sub-agents", len(toolRegistry.List()), len(subAgentRegistry.Names()))

	resolver := func(name types.AgentName) llm.Adapter {
		resolved := config.ResolveProvider(providersCfg, string(name))
		adapter, err := llm.NewFromConfig(resolved)
		if err != nil {
			logging.Error("[llm] resolver for agent=%s: %v", name, err)
			return nil
		}
		// Optional fallback adapter. providers.yaml :: agents.<name>_fallback
		// declares a secondary provider/model that the existing
		// llm.FallbackAdapter (variadic try-each-on-error wrapper)
		// rotates through when the primary returns ANY error. The
		// fallback's config goes through the same ResolveProvider
		// default-override merge as the primary so an operator can
		// specify just the model name and inherit api_key /
		// base_url / etc.
		fallbackKey := string(name) + "_fallback"
		if _, hasFallback := providersCfg.LLM.Agents[fallbackKey]; hasFallback {
			resolvedFb := config.ResolveProvider(providersCfg, fallbackKey)
			if fbAdapter, fbErr := llm.NewFromConfig(resolvedFb); fbErr != nil {
				logging.Warning("[llm] %s fallback adapter init failed (continuing without fallback): %v",
					fallbackKey, fbErr)
			} else {
				logging.Info("[llm] agent=%s primary=%s fallback=%s wired",
					name, adapter.ModelID(), fbAdapter.ModelID())
				adapter = llm.NewFallbackAdapter(adapter, fbAdapter)
			}
		}
		return adapter
	}

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
	}

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
	orch.SetMaxSteps(flagMaxSteps)
	orch.SetLanguage(flagLang)
	orch.SetEmitter(renderer.Emitter())
	orch.SetBlobSessionDir(blobSessionDir)
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
	// Reflexion-pattern critic. Resolved above; nil-safe inside
	// orchestrator (clearForReplan falls back to heuristic-only hint
	// when adapter is missing). Tied to the same retry-budget knob —
	// reflector only fires when the budget allows another iteration.
	if app.reflectorLLM != nil {
		orch.SetReflector(orchestrator.NewReflector(
			withRenderLLMTelemetry(app.reflectorLLM, renderer, types.AgentName("reflector"), types.StagePlan),
		))
	}
	// Commit 4 P1-F: plan_critic. Only install when the operator
	// explicitly turned the gate on — default off because each
	// enabled run pays one extra LLM call.
	planCriticEnabled := pipelineSettings.PlanCriticEnabled
	// Acceptance checker: install unconditionally. Only fires
	// inside multi-phase runs (stage II runPhaseGroup); single-
	// phase Runs pay nothing.
	if app.acceptanceCheckerLLM != nil {
		orch.SetAcceptanceChecker(orchestrator.NewAcceptanceChecker(
			withRenderLLMTelemetry(app.acceptanceCheckerLLM, renderer, types.AgentName("acceptance_checker"), types.StageVerify),
		))
	}

	// Continuation classifier (commit 48 read-mode Gap 1).
	// Replaces the keyword-table heuristic in
	// types.IsContinuation when configured. Routed via
	// providers.yaml :: agents.continuation_classifier;
	// recommended cheap-model routing (the call is bounded —
	// one prior + one current line; sub-100-token LLM round-
	// trip per Run). Falls back to keyword heuristic when the
	// adapter is missing.
	{
		resolved := config.ResolveProvider(providersCfg, "continuation_classifier")
		if adapter, err := llm.NewFromConfig(resolved); err == nil {
			orch.SetContinuationClassifier(orchestrator.NewContinuationClassifier(
				withRenderLLMTelemetry(adapter, renderer, types.AgentName("continuation_classifier"), types.StageAnalyze),
			))
		} else {
			logging.Debug("[continuation_classifier] adapter init failed; staying on keyword heuristic: %v", err)
		}
	}

	if planCriticEnabled && app.planCriticLLM != nil {
		orch.SetPlanCritic(orchestrator.NewPlanCritic(
			withRenderLLMTelemetry(app.planCriticLLM, renderer, types.AgentName("plan_critic"), types.StagePlan),
		))
		// Surface the model so operators see "extra Opus call per
		// plan" upfront rather than discovering it on the bill.
		// Recommend routing to a cheap model (Haiku-class) via
		// providers.yaml :: agents.plan_critic.
		logging.Info("[plan_critic] enabled — model=%s (one Chat per plan emit; route to a cheap model via providers.yaml :: agents.plan_critic for low cost)",
			app.planCriticLLM.ModelID())
	}
	// Item 1: baseline capture for CritNoRegression. Default false
	// (test-suite wall time doubles when enabled).
	orch.SetBaselineCaptureEnabled(pipelineSettings.BaselineCaptureEnabled)
	// Commit 2 P0 A1: baseline disk cache keyed by main-repo HEAD
	// SHA. Cache hits give a free baseline on subsequent Runs that
	// observe the same SHA, regardless of the capture-enabled flag.
	orch.SetBaselineCache(orchestrator.NewBaselineCache(runtimeAnchor, pipelineSettings.BaselineCacheMax))
	// Commit 2 P0 C: write-mode wall-clock deadline. 0 disables;
	// the orch setter clamps positive values at 1800.
	orch.SetWriteMaxSeconds(pipelineSettings.WriteMaxSeconds)
	// Fix 4: preserve worktree after successful ModeApply. Default
	// false; enable to keep the "try before merge" workflow.
	orch.SetKeepWorktreeOnSuccess(pipelineSettings.KeepWorktreeOnSuccess)

	// Resolve per-agent think_aloud from providers.yaml. The default
	// is true (directive included); per-agent overrides can disable it.
	taMap := make(map[types.AgentName]bool)
	defaultTA := true
	if providersCfg.LLM.Default.ThinkAloud != nil {
		defaultTA = *providersCfg.LLM.Default.ThinkAloud
	}
	for _, name := range []types.AgentName{
		types.AgentAnalyzer, types.AgentExplorer,
		types.AgentExtractor, types.AgentFinalizer,
	} {
		resolved := config.ResolveProvider(providersCfg, string(name))
		if resolved.ThinkAloud != nil {
			taMap[name] = *resolved.ThinkAloud
		} else {
			taMap[name] = defaultTA
		}
	}
	orch.SetThinkAloudMap(taMap)
	app.orch = orch

	// Multi-repo wiring (Phase 4.2). Both providers close over `app`
	// so REPL state changes (/repos focus, /repos cap, /repos refresh)
	// propagate to the next Run via app.multigraph + app.topology.
	app.orch.SetMultiGraphProvider(func() any {
		return app.multigraph
	})
	app.orch.SetMultiRepoSnapshotProvider(func() ([]types.SubRepoSnapshot, []string) {
		if app.topology == nil {
			return nil, nil
		}
		subs := repomap.SubRepoSnapshotsFromTopology(app.topology)
		var pending []string
		if mg := repomap.MultiGraphFromContext(&types.BusContext{MultiGraph: app.multigraph}); mg != nil {
			pending = mg.PendingSubRepoNames()
		}
		return subs, pending
	})
	// Stamp the L0-advisory inactive preview cap (clamped at yaml load).
	// Read by context/builder.go::formatMultiRepoActiveSetAdvisory.
	app.orch.SetMultiRepoInactivePreviewCount(app.multiRepoInactivePreviewCount)

	// Stage 3 (commit 40): per-repo Failure Taxonomy wiring.
	// Lifted out of runREPL into initApp so the single-shot CLI
	// path (`--mode=apply --plan-file=...`) ALSO gets cross-Run
	// learning when --cache-dir is set. Pre-commit-40 the
	// wiring lived inside runREPL only; runSingleShot bypassed
	// it entirely + every CLI run paid the LLM emit-tokens cost
	// for emit_failure_pattern with zero persistence. Disabled
	// (no-op) when flagCacheDir or repoSlug is empty, OR yaml
	// pipeline_failure_taxonomy_enabled=false. The store
	// itself is opt-in: every dispatch path (reflector,
	// planner, REPL /pitfalls) reads o.failureTaxonomyStore!=nil
	// before doing anything, so a nil here is a clean disable.
	if slug := repoSlug(flagRepo); slug != "" && flagCacheDir != "" && failureTaxonomyEnabled {
		store := orchestrator.NewFailureTaxonomyStore(flagCacheDir, slug,
			failureTaxonomyMaxItems, failureTaxonomyDecayDays)
		app.orch.SetFailureTaxonomyStore(store)
	}

	// Read-mode Answer Taxonomy wiring (commit 51 Gap 3, mirror).
	// Same gates as failure taxonomy: needs slug + cacheDir +
	// yaml-enabled. The reviewer adapter routes through
	// providers.yaml :: agents.answer_reviewer (cheap-model
	// recommended); falls back to the default LLM when absent.
	if slug := repoSlug(flagRepo); slug != "" && flagCacheDir != "" && answerTaxonomyEnabled {
		store := orchestrator.NewAnswerTaxonomyStore(flagCacheDir, slug,
			answerTaxonomyMaxItems, answerTaxonomyDecayDays)
		// Commit 60 Batch E.2 (audit MEDIUM #11): wire stale-
		// pattern eviction. The closure resolves repo-relative
		// paths against the absolute --repo arg + os.Stat for
		// existence. Patterns whose ExampleFile no longer exists
		// in this repo are skipped at RelevantTo time so the
		// analyzer doesn't get nudged by stale-pitfall hints from
		// code that's been refactored away.
		repoAbs := flagRepo
		store.SetExampleFileValidator(func(relPath string) bool {
			if relPath == "" || repoAbs == "" {
				return true
			}
			abs := filepath.Join(repoAbs, relPath)
			info, err := os.Stat(abs)
			return err == nil && !info.IsDir()
		})
		app.orch.SetAnswerTaxonomyStore(store)
		resolved := config.ResolveProvider(providersCfg, "answer_reviewer")
		if adapter, err := llm.NewFromConfig(resolved); err == nil {
			app.orch.SetAnswerReviewer(orchestrator.NewAnswerReviewer(
				withRenderLLMTelemetry(adapter, app.renderer, types.AgentName("answer_reviewer"), types.StageFinalize),
			))
		}
	}

	// Commit 62: self-consistency reviewer wiring. Default:
	// enabled=false, rewrite_on_contradiction=true,
	// min_confidence=0.92. Provider routed via providers.yaml ::
	// agents.self_consistency_reviewer (cheap-model recommended;
	// task is plain prose↔prose comparison). Falls back to
	// chitchat_classifier or default LLM when absent.
	if selfConsistencyEnabled && strictAnswerReviewEnabled {
		resolved := config.ResolveProvider(providersCfg, "self_consistency_reviewer")
		if adapter, err := llm.NewFromConfig(resolved); err == nil {
			app.orch.SetSelfConsistencyReviewer(orchestrator.NewSelfConsistencyReviewer(
				withRenderLLMTelemetry(adapter, app.renderer, types.AgentName("self_consistency_reviewer"), types.StageFinalize),
			))
			app.orch.SetSelfConsistencyRewriteOnContradiction(selfConsistencyRewrite)
			app.orch.SetSelfConsistencyMinConfidence(selfConsistencyMinConfidence)
			logging.Info("[self_consistency] reviewer enabled: model=%s rewrite_on_contradiction=%v min_confidence=%.2f",
				adapter.ModelID(), selfConsistencyRewrite, selfConsistencyMinConfidence)
		} else {
			logging.Warning("[self_consistency] adapter resolve failed (%v); reviewer disabled this Run", err)
		}
	}

	// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — second-layer
	// semantic-quality reviewer wiring. Routed via providers.yaml ::
	// agents.semantic_quality_reviewer when present, falls back to
	// the self_consistency_reviewer slot (same task class:
	// independent reviewer LLM with structured emit).
	if semanticQualityEnabled && strictAnswerReviewEnabled {
		resolved := config.ResolveProvider(providersCfg, "semantic_quality_reviewer")
		if adapter, err := llm.NewFromConfig(resolved); err == nil {
			app.orch.SetSemanticQualityReviewer(orchestrator.NewSemanticQualityReviewer(
				withRenderLLMTelemetry(adapter, app.renderer, types.AgentName("semantic_quality_reviewer"), types.StageFinalize),
			))
			// P4 (2026-05-10): apply yaml-resolved floor (default 0.92).
			app.orch.SetSemanticQualityMinConfidence(semanticQualityMinConfidence)
			logging.Info("[semantic_quality] reviewer enabled: model=%s min_confidence=%.2f",
				adapter.ModelID(), semanticQualityMinConfidence)
		} else {
			logging.Warning("[semantic_quality] adapter resolve failed (%v); reviewer disabled this Run", err)
		}
	}

	// B0 write-mode resolution. Assemble inputs from yaml + CLI,
	// validate via the pure resolveWriteMode, and install the result
	// on the orchestrator. Errors here are terminal — an invalid
	// --mode combination aborts before any Run() can start, so the
	// user sees a clean cobra error message rather than a runtime
	// failure from the orchestrator's default switch branch.
	//
	// Single-shot detection: flagRequest is the --request / -r flag;
	// cobra passes the positional request into initApp's args before
	// rootRun stores it in the local request variable. Count both so
	// write-mode validation and startup-only REPL notices do not
	// accidentally treat positional single-shot CLI as interactive.
	hasRequest := strings.TrimSpace(flagRequest) != "" || len(args) > 0
	writeInputs := writeModeInputs{
		CLIFlagPassed: cmd.Flags().Changed("mode"),
		CLIFlagMode:   flagMode,
		HasRequest:    hasRequest,
		AutoApply:     flagAutoApply,
		PlanFile:      flagPlanFile,
	}
	if rs != nil {
		writeInputs.YamlEnabled = rs.WriteEnabled
		writeInputs.YamlDefaultMode = rs.WriteDefaultMode
	}
	// Cache the resolved yaml gate on app so the REPL Config can
	// forward it to repl.New. nil pointer (yaml absent OR field
	// omitted) → false (refuse non-read modes by default; matches
	// resolveWriteMode's posture).
	if rs != nil && rs.WriteEnabled != nil && *rs.WriteEnabled {
		app.writeEnabled = true
	}
	// Resolve auto-init authorization. CLI flag overrides yaml when
	// set explicitly (cobra reports it as "Changed"); otherwise yaml
	// is the source. Default false → interactive consent (REPL) or
	// fail-loud (single-shot without flag).
	if cmd.Flags().Changed("auto-init-repo") {
		app.writeAutoInitRepo = flagAutoInitRepo
	} else if rs != nil && rs.WriteAutoInitRepo != nil {
		app.writeAutoInitRepo = *rs.WriteAutoInitRepo
	}
	// Resolve scaffold authorization. CLI overrides yaml when set
	// explicitly. Default false → empty-dir plan/apply runs fail-loud
	// on the planPreHook scaffold gate with a clear hint.
	if cmd.Flags().Changed("allow-scaffold") {
		app.writeScaffoldEnabled = flagScaffold
	} else if rs != nil && rs.WriteScaffoldEnabled != nil {
		app.writeScaffoldEnabled = *rs.WriteScaffoldEnabled
	}
	// Cache the resolved yaml path for the REPL's L2 gate so the user
	// gets pointed at an exact file rather than a generic "codrax.yaml".
	if _, err := os.Stat(settingsPath); err == nil {
		app.settingsPath = settingsPath
	}
	effectiveMode, err := resolveWriteMode(writeInputs)
	if err != nil {
		return err
	}
	if effectiveMode != "" && effectiveMode != types.ModeRead {
		logging.Info("[cmd] pipeline mode: %s", string(effectiveMode))
	}
	// Absolutize plan file paths so downstream tools (Day 5) see a
	// canonical form regardless of the user's CWD.
	if flagPlanFile != "" {
		if abs, err := filepath.Abs(flagPlanFile); err == nil {
			flagPlanFile = abs
		}
	}
	if flagPlanOut != "" {
		if abs, err := filepath.Abs(flagPlanOut); err == nil {
			flagPlanOut = abs
		}
	}
	orch.SetMode(effectiveMode)
	// B1.2 wiring: plumb the --plan-file value + the worktree base
	// directory into the orchestrator so the apply stage hook can load the
	// ChangePlan and provision a worktree. Both calls are safe to
	// invoke with empty arguments (read-mode callers pass empty
	// flagPlanFile and the orch setters simply record them; the
	// apply-phase guards surface a clear error when the values
	// are genuinely missing at apply time).
	orch.SetPlanPath(flagPlanFile)
	orch.SetWorktreeBase(worktreeBase)
	// Pre-authorized auto-init? Forward to the orchestrator so the
	// apply pre-hook can transparently scaffold a bare/headless
	// repo. False (default) keeps the existing fail-loud behaviour;
	// the REPL handleApproveCmd path additionally runs an interactive
	// y/N prompt before lifting the gate, so single-shot CLI calls
	// without the flag see a clean error and a hint.
	orch.SetAutoInitRepo(app.writeAutoInitRepo)
	orch.SetScaffoldEnabled(app.writeScaffoldEnabled)

	// Chit-chat responder. Follows the llmSummarizer pattern: one
	// direct adapter.Chat call, no agent framework. Default ON — the
	// feature is a user-triggered /chat slash command, so simply
	// making the responder available imposes zero cost on users who
	// never type /chat. Operators disable via codrax.yaml
	// `chitchat_enabled: false` when they want the command to print a
	// "not configured" warning (e.g. restricted deployments).
	chitchatEnabled := true
	if rs != nil && rs.ChitchatEnabled != nil {
		chitchatEnabled = *rs.ChitchatEnabled
	}
	if chitchatEnabled {
		resolved := config.ResolveProvider(providersCfg, "chitchat_responder")
		if adapter, err := llm.NewFromConfig(resolved); err != nil {
			logging.Warning("[chitchat] responder adapter init failed; /chat will print a warning: %v", err)
		} else {
			responder := repl.NewChitchatResponder(
				withRenderLLMTelemetry(adapter, renderer, types.AgentName("chitchat_responder"), ""),
			)
			// Apply the chitchat tool-use limits from yaml. The
			// responder's exported SetChitchatSettings setter is the
			// single seam that keeps the constructor signature
			// stable across this addition.
			var chitchatCfg types.ChitchatSettings
			if rs != nil {
				if rs.ChitchatRecallDefaultLimit != nil {
					chitchatCfg.RecallDefaultLimit = *rs.ChitchatRecallDefaultLimit
				}
				if rs.ChitchatRecallMaxLimit != nil {
					chitchatCfg.RecallMaxLimit = *rs.ChitchatRecallMaxLimit
				}
				if rs.ChitchatListDefaultLimit != nil {
					chitchatCfg.ListDefaultLimit = *rs.ChitchatListDefaultLimit
				}
				if rs.ChitchatListMaxLimit != nil {
					chitchatCfg.ListMaxLimit = *rs.ChitchatListMaxLimit
				}
			}
			if r, ok := responder.(interface{ SetChitchatSettings(types.ChitchatSettings) }); ok {
				r.SetChitchatSettings(types.ResolvedChitchatSettings(chitchatCfg))
			}
			app.chitchatResponder = responder
		}
	}

	// env_recommend LLM fallback adapter. Routed via providers.yaml
	// agents.env_recommender; falls back to chitchat_classifier
	// (cheap model). nil → Stage 3 LLM fallback short-circuits and
	// the recommender returns whatever Stage 1/2 produced (or empty).
	if app.envRecommendSettings.LLMEnabled {
		envProvider := config.ResolveProvider(providersCfg, "env_recommender")
		if _, has := providersCfg.LLM.Agents["env_recommender"]; !has {
			envProvider = config.ResolveProvider(providersCfg, "chitchat_classifier")
		}
		if adapter, err := llm.NewFromConfig(envProvider); err != nil {
			logging.Warning("[env_recommend] LLM adapter init failed; deterministic-only: %v", err)
		} else {
			env.SetLLMAdapter(adapter)
		}
	}

	// Optional auto-classifier. Only wired when both the feature
	// master switch and the classifier switch are on AND the responder
	// was constructed successfully — a classifier without a responder
	// is useless (would reroute turns to an inert /chat).
	//
	// Default: ON. Every REPL turn makes one cheap LLM call (the
	// classifier) before the normal pipeline decision. Fail-safe: any
	// classifier error routes the turn to the pipeline unchanged, so
	// a broken or slow classifier never silently misroutes a real code
	// question. Operators wanting to cut cost should route
	// `chitchat_classifier` to a small model in providers.yaml; those
	// wanting to disable entirely set codrax.yaml
	// chitchat_classifier_enabled: false or pass
	// --chitchat-classifier=false.
	//
	// Precedence (lowest wins last): code default (true) → codrax.yaml
	// → --chitchat-classifier CLI flag. cmd.Flags().Changed()
	// distinguishes "flag not passed" (yaml wins) from "flag passed
	// with value false" (CLI forces off).
	classifierEnabled := true
	if rs != nil && rs.ChitchatClassifierEnabled != nil {
		classifierEnabled = *rs.ChitchatClassifierEnabled
	}
	if cmd.Flags().Changed("chitchat-classifier") {
		classifierEnabled = flagChitchatClassifier
		logging.Info("[chitchat] classifier: %t (via --chitchat-classifier flag)", classifierEnabled)
	}
	if chitchatEnabled && classifierEnabled && app.chitchatResponder != nil {
		resolved := config.ResolveProvider(providersCfg, "chitchat_classifier")
		if adapter, err := llm.NewFromConfig(resolved); err != nil {
			logging.Warning("[chitchat] classifier adapter init failed; auto-routing disabled: %v", err)
		} else {
			app.chitchatClassifier = repl.NewChitchatClassifier(
				withRenderLLMTelemetry(adapter, renderer, types.AgentName("chitchat_classifier"), ""),
			)
			logging.Info("[chitchat] auto-classifier: ON (model=%s). Disable with codrax.yaml chitchat_classifier_enabled: false or --chitchat-classifier=false. Route to a cheap model via providers.yaml agents.chitchat_classifier", adapter.ModelID())
		}
	}

	return nil
}

func withRenderLLMTelemetry(adapter llm.Adapter, renderer *render.Renderer, agentName types.AgentName, stage types.PipelineStage) llm.Adapter {
	if adapter == nil || renderer == nil {
		return adapter
	}
	emit := renderer.Emitter()
	return llm.NewTelemetryAdapter(adapter, func(t llm.RequestTelemetry) {
		emit(render.Event{
			Kind:                  render.EventLLMRequestStart,
			Timestamp:             time.Now(),
			Agent:                 agentName,
			Stage:                 stage,
			ModelID:               t.ModelID,
			ContextTokensEstimate: t.ContextTokensEstimate,
			ContextWindowTokens:   t.ContextWindowTokens,
		})
	})
}

// --- Helper functions (moved from main.go) ---

func executableDir() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

func repoSlug(repo string) string {
	if repo == "" {
		repo = "."
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	base := filepath.Base(abs)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "repo"
	}
	base = sanitizeSlug(base)
	h := fnv.New32a()
	_, _ = h.Write([]byte(abs))
	return fmt.Sprintf("%s-%08x", base, h.Sum32())
}

func sanitizeSlug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "repo"
	}
	return out
}

func anchorPath(anchor, p string) string {
	if anchor == "" || p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(anchor, p))
}

func anchorProviderTLSCAFiles(cfg *types.ProvidersConfig, anchor string) {
	if cfg == nil {
		return
	}
	anchorProviderTLSCAFile(&cfg.LLM.Default, anchor)
	for name, provider := range cfg.LLM.Agents {
		anchorProviderTLSCAFile(&provider, anchor)
		cfg.LLM.Agents[name] = provider
	}
}

func anchorProviderTLSCAFile(provider *types.LLMProviderConfig, anchor string) {
	if provider == nil {
		return
	}
	caFile := strings.TrimSpace(provider.TLSCAFile)
	provider.TLSCAFile = caFile
	if anchor == "" || caFile == "" || filepath.IsAbs(caFile) {
		return
	}
	provider.TLSCAFile = filepath.Clean(filepath.Join(anchor, caFile))
}

func createDefaultAdapter(cfg *types.ProvidersConfig, providersPath string) (llm.Adapter, error) {
	resolved := config.ResolveProvider(cfg, "")
	adapter, err := llm.NewFromConfig(resolved)
	if err != nil {
		logging.Error("[llm] default adapter: %v", err)
		return nil, providerConfigError(providersPath, err)
	}
	if strings.TrimSpace(cfg.LLM.Default.Model) == "" && adapter != nil {
		cfg.LLM.Default.Model = adapter.ModelID()
	}
	return adapter, nil
}

func providerConfigError(providersPath string, cause error) error {
	path := strings.TrimSpace(providersPath)
	if path == "" {
		path = defaultProvidersConfig
	}
	absPath := path
	if abs, err := filepath.Abs(path); err == nil {
		absPath = abs
	}
	status := "found"
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			status = "not found"
		} else {
			status = err.Error()
		}
	}
	return fmt.Errorf(
		"LLM provider is not configured, so Codrax cannot start the REPL or run a CLI request.\n"+
			"没有可用的模型 provider 配置，Codrax 已停止本次运行，避免使用占位模型产生误导结果。\n\n"+
			"Provider config: %s (%s)\n"+
			"Reason: %v\n\n"+
			"How to fix:\n"+
			"  - Put a valid providers.yaml next to the codrax binary, or pass --providers /path/to/providers.yaml.\n"+
			"  - Static API-key setup: configure llm.default.provider, api_key, model, and base_url.\n"+
			"  - OAuth setup: configure llm.default.provider, base_url, auth, models_path or model, and any provider-specific headers/request_extra.\n"+
			"  - If tls_ca_file is set, make sure the file exists, is readable, and contains valid PEM certificates.\n"+
			"  - For environment-only static setup, export LLM_PROVIDER, OPENAI_API_KEY, OPENAI_MODEL, and OPENAI_BASE_URL.",
		absPath, status, cause,
	)
}

func resolveToolParamCompatByAgent(cfg *types.ProvidersConfig) (map[types.AgentName]types.ToolParamCompatConfig, error) {
	if cfg != nil && cfg.LLM.Default.ToolParamCompat != nil {
		if mode := cfg.LLM.Default.ToolParamCompat.NormalizedMode(); mode == "" {
			return nil, fmt.Errorf("providers.yaml: llm.default.tool_param_compat.mode must be one of off, audit, repair; got %q",
				cfg.LLM.Default.ToolParamCompat.Mode)
		}
	}
	if cfg != nil {
		for name, raw := range cfg.LLM.Agents {
			if raw.ToolParamCompat == nil {
				continue
			}
			if mode := raw.ToolParamCompat.NormalizedMode(); mode == "" {
				return nil, fmt.Errorf("providers.yaml: llm.agents.%s.tool_param_compat.mode must be one of off, audit, repair; got %q",
					name, raw.ToolParamCompat.Mode)
			}
		}
	}

	out := make(map[types.AgentName]types.ToolParamCompatConfig)
	for _, name := range types.AllAgentNames() {
		compat := types.DefaultToolParamCompatConfig()
		if cfg != nil {
			if cfg.LLM.Default.ToolParamCompat != nil {
				compat = *cfg.LLM.Default.ToolParamCompat
			}
			if cfg.LLM.Agents != nil {
				if raw, ok := cfg.LLM.Agents[string(name)]; ok && raw.ToolParamCompat != nil {
					compat = *raw.ToolParamCompat
				}
			}
		}
		mode := compat.NormalizedMode()
		if mode == types.ToolParamCompatOff {
			continue
		}
		out[name] = compat
		logging.Info("[tool_param_compat] agent=%s mode=%s split_string_arrays=%t",
			name, mode, compat.SplitStringArraysEnabled())
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func loopPolicyFromAgentSettings(settings types.AgentSettings) agent.LoopPolicy {
	policy := agent.DefaultLoopPolicy()
	policy.MinInjectInterval = settings.LoopMinInjectInterval
	policy.MaxContinuations = settings.LoopMaxContinuations
	policy.MaxMidLoopInjects = settings.LoopMaxMidLoopInjects
	policy.IdleStopThreshold = settings.LoopIdleStopThreshold
	return policy
}

// placeholderAdapter is a minimal LLM adapter for testing the pipeline structure.
type placeholderAdapter struct{}

func (p *placeholderAdapter) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	var lastMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" || messages[i].Role == "system" {
			lastMsg = messages[i].Content
			break
		}
	}
	return llm.Response{
		Content:    fmt.Sprintf("Processed: %s", truncate(lastMsg, 200)),
		StopReason: "end_turn",
		Usage: llm.TokenUsage{
			InputTokens:  len(lastMsg),
			OutputTokens: 50,
		},
	}, nil
}

func (p *placeholderAdapter) ModelID() string               { return "placeholder-v1" }
func (p *placeholderAdapter) MaxContextTokens() int         { return 200000 }
func (p *placeholderAdapter) MaxOutputTokens() int          { return 0 }
func (p *placeholderAdapter) RequestTimeout() time.Duration { return 120 * time.Second }
func (p *placeholderAdapter) RetryMaxAttempts() int         { return 6 }

// formatOutputCap renders the resolved max_output_tokens for the
// startup log. Zero means "no client-side cap, server uses model
// ceiling" — surface that intent explicitly so operators don't read
// "max_output=0" as "broken config." Positive values render as the
// literal token count.
func formatOutputCap(n int) string {
	if n <= 0 {
		return "max_output=server-default"
	}
	return fmt.Sprintf("max_output=%d tokens", n)
}

func installLLMStartupUXHooks(interactiveStartup bool) {
	llm.DeferModelSelectionNotices(interactiveStartup)
	if !interactiveStartup {
		llm.SetOAuthAuthorizationNoticeHook(nil)
		llm.SetOAuthAuthorizationCompleteNoticeHook(nil)
		return
	}
	llm.SetOAuthAuthorizationNoticeHook(func(notice llm.OAuthAuthorizationNotice) bool {
		ensureREPLStartupHeaderPrinted()
		repl.PrintOAuthAuthorizationPrompt(os.Stdout, flagLang, notice.URL)
		return true
	})
	llm.SetOAuthAuthorizationCompleteNoticeHook(func(notice llm.OAuthAuthorizationCompleteNotice) bool {
		ensureREPLStartupHeaderPrinted()
		repl.PrintOAuthAuthorizationComplete(os.Stdout, flagLang)
		return true
	})
}

func ensureREPLStartupHeaderPrinted() {
	if app.replHeaderPrinted {
		return
	}
	repl.PrintStartupHeader(os.Stdout, version, flagRepo)
	app.replHeaderPrinted = true
}

func replModelSummaryLine(lang string, adapter llm.Adapter, autoSelected bool) string {
	if adapter == nil {
		return ""
	}
	model := strings.TrimSpace(adapter.ModelID())
	if model == "" {
		model = "(unknown)"
	}
	ctx := formatContextWindow(adapter.MaxContextTokens())
	out := formatOutputCap(adapter.MaxOutputTokens())
	out = strings.TrimPrefix(out, "max_")
	selection := ""
	if autoSelected {
		if isZhLang(lang) {
			selection = " · 自动选择列表首项"
		} else {
			selection = " · auto-selected"
		}
	}
	if isZhLang(lang) {
		return fmt.Sprintf("模型: %s · 上下文=%s · %s · 超时=%s · 重试=%d%s",
			model, ctx, localizeOutputCap(lang, out), adapter.RequestTimeout(), adapter.RetryMaxAttempts(), selection)
	}
	return fmt.Sprintf("Model: %s · ctx=%s · %s · timeout=%s · retry=%d%s",
		model, ctx, out, adapter.RequestTimeout(), adapter.RetryMaxAttempts(), selection)
}

func replModelListLine(lang string, notices []llm.ModelSelectionNotice) string {
	if len(notices) == 0 {
		return ""
	}
	notice := notices[len(notices)-1]
	selected := strings.TrimSpace(notice.Selected)
	if selected == "" || len(notice.Models) == 0 {
		return ""
	}
	const maxInlineModels = 5
	items := make([]string, 0, minInt(len(notice.Models), maxInlineModels))
	for i, m := range notice.Models {
		if i >= maxInlineModels {
			break
		}
		name := strings.TrimSpace(m.Name)
		if name == "" {
			continue
		}
		suffix := ""
		if name == selected {
			if isZhLang(lang) {
				suffix = " ← 已选择"
			} else {
				suffix = " ← selected"
			}
		}
		items = append(items, fmt.Sprintf("%d. %s%s", i+1, name, suffix))
	}
	if len(items) == 0 {
		return ""
	}
	tail := ""
	if len(notice.Models) > maxInlineModels {
		if isZhLang(lang) {
			tail = fmt.Sprintf("; … 共 %d 个", len(notice.Models))
		} else {
			tail = fmt.Sprintf("; … %d total", len(notice.Models))
		}
	}
	if isZhLang(lang) {
		return "可用模型: " + strings.Join(items, "; ") + tail
	}
	return "Available models: " + strings.Join(items, "; ") + tail
}

func localizeOutputCap(lang, out string) string {
	if !isZhLang(lang) {
		return out
	}
	switch out {
	case "output=server-default":
		return "输出=服务端默认"
	default:
		return strings.Replace(out, "output=", "输出=", 1)
	}
}

func formatContextWindow(n int) string {
	if n <= 0 {
		return "unknown"
	}
	if n%1000 == 0 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%d tokens", n)
}

func isZhLang(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// clampReflectorTimeout returns the request_timeout_seconds the
// reflector LLM adapter should use. The reflector is a side channel
// (clearForReplan degrades to heuristic on any error), so a slow
// upstream blocking the retry loop for the default 120s HTTP
// timeout wastes a full retry slot. 30s gives 5-10× headroom over
// a typical fast critique (1-3s) while keeping retry-loop wall
// time bounded.
//
// resolvedSeconds is the value config.ResolveProvider returned for
// the "reflector" agent. hasExplicitConfig is true when the
// operator declared `agents.reflector` in providers.yaml. Rules:
//
//   - explicit config + positive value → operator wins (their
//     decision; presumed deliberate)
//   - explicit config + zero/negative → safety clamp (operator
//     forgot to set request_timeout_seconds)
//   - no explicit config → safety clamp (default LLM's timeout
//     would otherwise apply, often 120s)
//
// All paths converge on a positive return value, never zero.
func clampReflectorTimeout(resolvedSeconds int, hasExplicitConfig bool) int {
	const safetyClampSeconds = 30
	if hasExplicitConfig && resolvedSeconds > 0 {
		return resolvedSeconds
	}
	return safetyClampSeconds
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// memorySummarizerTool is the structured-output schema the LLM
// emits to compact one conversation turn into a memory index entry.
// Kept LOCAL (not registered in tool.Registry) because the summarizer
// bypasses the agent framework entirely and calls adapter.Chat
// directly — the main pipeline agents never see this tool. Same
// pattern as internal/repl/chitchat.go :: chitchatClassifierTool.
//
// Session-32 additions: `entities` (stable symbol-level anchors,
// weighted higher than keywords during retrieval) and `refs` (prior
// turn IDs the turn explicitly references, used for 1-hop chain
// traversal in BuildContext). Both are required fields in the schema
// so the model always emits *something* — empty arrays are valid and
// simply skip the boost paths in BuildContext.
var memorySummarizerTool = llm.ToolSchema{
	Name:        "emit_memory_summary",
	Description: "Compact one conversation turn into a memory index entry. Exactly one call.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "topic":    {"type": "string", "description": "Short phrase (≤ 12 words) naming the subject of the turn. Used as the header in prior-conversation lookups."},
    "keywords": {"type": "array", "items": {"type": "string"}, "description": "Up to 8 concrete keywords — identifiers, file names, or domain terms — that future prior-conversation keyword matching should latch on. Prefer tokens that appeared verbatim in the request or response over invented synonyms."},
    "entities": {"type": "array", "items": {"type": "string"}, "description": "3-6 stable symbol-level anchors actually named in the turn: file paths, function or method names, class or type names, package names, identifiers. These weigh more than free-form keywords during retrieval, so prefer tokens that are unlikely to be typed accidentally elsewhere. Emit an empty array when the turn discussed no concrete symbols."},
    "refs":     {"type": "array", "items": {"type": "string"}, "description": "Prior conversation turn IDs this turn explicitly continues or cites, if any. Leave empty when the turn stands alone (the common case). Do NOT invent IDs — only emit when the turn text literally references a prior exchange."},
    "summary":  {"type": "string", "description": "One-paragraph recap (~150 words) covering the question asked and the key facts from the assistant's answer. Stay grounded in the turn content; do not invent details that were not discussed."}
  },
  "required": ["topic", "keywords", "entities", "refs", "summary"]
}`),
}

const memorySummarizerSystemPrompt = `You compact one conversation turn into a memory index entry.

Emit exactly one call to emit_memory_summary. Do NOT return free-form text; the caller ignores assistant content and parses only the tool call arguments.

Stay grounded in the turn content — topic, keywords, and entities must name things that actually appeared in the request or response, and the summary must describe what was asked and answered rather than speculating about adjacent topics.

Keywords vs entities: keywords are free-form tokens that a future user might type when asking a follow-up ("authentication", "panic", "timeout"). Entities are concrete source-code or project-level anchors that appeared verbatim in the turn ("authService.Login", "internal/config/runtime.go", "ChitchatResponder"). Entities are more stable retrieval keys than keywords, so emit them whenever the turn discussed real symbols. Empty entities is legitimate only for purely conversational turns (greetings, meta-questions).

Refs: emit a turn ID under refs only when the current turn text literally references a prior turn — e.g. "continuing from the memory-store discussion yesterday" quoting a concrete earlier topic. Do not fabricate IDs and do not emit refs when the turn stands alone (the common case).`

// llmSummarizer adapts an LLM adapter into a memory.Summarizer. It
// runs a single adapter.Chat call with tool_choice=required and the
// local memorySummarizerTool schema; every failure path (nil adapter,
// chat error, no tool call, malformed JSON) returns a heuristic
// fallback IndexEntry so memory compaction cannot break the REPL.
type llmSummarizer struct {
	adapter llm.Adapter
}

func newLLMSummarizer(adapter llm.Adapter) *llmSummarizer {
	return &llmSummarizer{adapter: adapter}
}

func (s *llmSummarizer) Summarize(_ context.Context, turn memory.Turn) (memory.IndexEntry, error) {
	fallback := memory.IndexEntry{
		ID:       turn.ID,
		Topic:    truncate(strings.ReplaceAll(turn.Request, "\n", " "), 80),
		Keywords: extractKeywords(turn.Request),
		Summary:  truncate(strings.ReplaceAll(turn.Response, "\n", " "), 400),
	}
	if s.adapter == nil {
		return fallback, nil
	}

	userContent := "USER REQUEST:\n" + turn.Request + "\n\nASSISTANT RESPONSE:\n" + turn.Response
	messages := []llm.Message{
		{Role: "system", Content: memorySummarizerSystemPrompt},
		{Role: "user", Content: userContent},
	}
	tools := []llm.ToolSchema{memorySummarizerTool}
	// Memory summarizer is a background compaction worker — no
	// user-cancel surface today. context.Background() declares
	// "no cancel" explicitly; when memory compaction grows a
	// cancel surface (e.g. /clear pre-empts in-flight compaction)
	// thread the right ctx here.
	resp, err := s.adapter.Chat(context.Background(), messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		logging.Warning("[memory] summarizer LLM error, using fallback: %v", err)
		return fallback, nil
	}
	if len(resp.ToolCalls) == 0 {
		// Some providers (notably MiniMax) do not strictly honour
		// tool_choice="required" — they return finish_reason=end_turn
		// with hundreds of tokens of natural-language summary in
		// resp.Content instead of an emit_memory_summary call.
		// Pre-2026-05-02 this site discarded the prose entirely and
		// fell back to an 80-char heuristic Topic + 400-char Response
		// truncate, throwing away the LLM's actual work.
		//
		// Salvage: when content is non-empty, treat it as the Summary
		// (truncated to a generous 800 chars so structured fields
		// like extractKeywords-derived Keywords still anchor index
		// matches). Topic stays heuristic because the prose may not
		// surface a clean one-line topic — combining LLM Summary
		// with heuristic Topic is the best of both signals when the
		// tool channel breaks.
		if content := strings.TrimSpace(resp.Content); content != "" {
			logging.Warning("[memory] summarizer LLM returned text instead of tool_call (provider may not honour tool_choice=required); salvaging %d chars of prose as Summary",
				len(content))
			return memory.IndexEntry{
				ID:       turn.ID,
				Topic:    fallback.Topic,
				Keywords: fallback.Keywords,
				Summary:  truncate(content, 800),
			}, nil
		}
		logging.Warning("[memory] summarizer LLM returned no tool_call AND empty content, using heuristic fallback")
		return fallback, nil
	}
	call := resp.ToolCalls[0]
	if call.Name != memorySummarizerTool.Name {
		logging.Warning("[memory] summarizer LLM emitted unexpected tool %q, using fallback", call.Name)
		return fallback, nil
	}
	var parsed struct {
		Topic    string   `json:"topic"`
		Keywords []string `json:"keywords"`
		Entities []string `json:"entities"`
		Refs     []string `json:"refs"`
		Summary  string   `json:"summary"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		logging.Warning("[memory] summarizer JSON parse error, using fallback: %v", err)
		return fallback, nil
	}
	// Partial-fill defence: if the model emitted the tool call but
	// left one of the fields empty, backfill from the heuristic so
	// downstream matchIndex still has something to work with. Topic
	// and Summary are independent so treat them independently.
	// Entities / Refs are session-32 additions and may legitimately
	// be empty (short casual turns with no symbols, or turns that
	// stand alone) — leave them nil in that case so appendIndexEntry
	// omits the frontmatter line rather than writing `entities: []`.
	if strings.TrimSpace(parsed.Topic) == "" {
		parsed.Topic = fallback.Topic
	}
	if strings.TrimSpace(parsed.Summary) == "" {
		parsed.Summary = fallback.Summary
	}
	if len(parsed.Keywords) == 0 {
		parsed.Keywords = fallback.Keywords
	}
	return memory.IndexEntry{
		ID:       turn.ID,
		Topic:    parsed.Topic,
		Keywords: parsed.Keywords,
		Summary:  parsed.Summary,
		Entities: parsed.Entities,
		Refs:     parsed.Refs,
		// SessionID + Kind are propagated from Turn → IndexEntry by
		// compactOldest so this function doesn't have to care about
		// them. Leaving both empty here is correct.
	}, nil
}

func extractKeywords(text string) []string {
	stop := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "is": true,
		"are": true, "was": true, "were": true, "to": true, "of": true, "in": true,
		"on": true, "for": true, "with": true, "this": true, "that": true, "it": true,
		"as": true, "be": true, "by": true, "at": true, "from": true,
	}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	seen := map[string]bool{}
	var out []string
	for _, w := range words {
		if len(w) < 3 || stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// resolveFocusFlagAgainstTopology turns the --focus flag's slice of
// slug-or-RootRel tokens into the slug list the multigraph layer
// expects. Each token is run through topology.Resolve so the
// operator can use whichever form is more natural (REPL surfaces
// both). Tokens that fail to resolve are surfaced as a single
// Warning line and dropped from the focus set; the run continues
// with the matched subset rather than failing loud — a typo on a
// stale slug should not block the entire request, and the
// remaining matches still pin correctly.
//
// Single-repo / no-git workspaces have at most one (or zero)
// sub-repo entries, so most --focus tokens won't match and the
// helper returns nil — the caller's focusSlugs=nil branch is the
// existing pre-flag behaviour.
func resolveFocusFlagAgainstTopology(topo *topology.RepoTopology, raw []string) []string {
	if topo == nil || len(raw) == 0 {
		return nil
	}
	var slugs []string
	var unresolved []string
	seen := map[string]bool{}
	for _, token := range raw {
		t := strings.TrimSpace(token)
		if t == "" {
			continue
		}
		sr := topo.Resolve(t)
		if sr == nil {
			unresolved = append(unresolved, t)
			continue
		}
		if seen[sr.Slug] {
			continue
		}
		seen[sr.Slug] = true
		slugs = append(slugs, sr.Slug)
	}
	if len(unresolved) > 0 {
		logging.Warning("multi-repo: --focus values did not match any sub-repo and were dropped: %v (run with --log-level=info to see the discovered sub-repo list)", unresolved)
	}
	return slugs
}

func annotateRepoMapScanEvent(ev types.RepoMapScanEvent) types.RepoMapScanEvent {
	if app.topology == nil || app.topology.IsSingle() {
		return ev
	}
	root := canonicalScanRoot(ev.RepoRoot)
	if root == "" {
		return ev
	}
	for _, sr := range app.topology.Repos {
		if sr.RootRel == "." {
			continue
		}
		if canonicalScanRoot(sr.RootAbs) != root {
			continue
		}
		ev.DisplayName = sr.RootRel
		ev.SubRepoRootRel = sr.RootRel
		return ev
	}
	return ev
}

func canonicalScanRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func startTopologyStartupNotice(enabled bool, lang string) func(*topology.RepoTopology, error, time.Duration) {
	if !enabled {
		return func(*topology.RepoTopology, error, time.Duration) {}
	}
	ensureREPLStartupHeaderPrinted()
	repl.PrintTopologyDiscoveryStart(os.Stdout, lang)
	return func(topo *topology.RepoTopology, err error, elapsed time.Duration) {
		if err != nil {
			repl.PrintTopologyDiscoveryError(os.Stdout, lang, err, formatStartupElapsed(elapsed))
			return
		}
		count := 0
		if topo != nil {
			count = len(topo.Repos)
		}
		repl.PrintTopologyDiscoveryComplete(os.Stdout, lang, count, formatStartupElapsed(elapsed))
	}
}

func prefersChineseStartup(lang string) bool {
	lang = strings.ToLower(strings.TrimSpace(lang))
	return lang == "" || strings.HasPrefix(lang, "zh")
}

func printMCPRegistryStartupNotice(reg *mcp.Registry, lang string) {
	if reg == nil {
		return
	}
	servers := len(reg.List())
	if servers == 0 {
		return
	}
	tools := len(reg.ListAllTools())
	resources := len(reg.ListAllResources())
	if prefersChineseStartup(lang) {
		fmt.Fprintf(os.Stdout, "· MCP 外部工具已就绪：%d 个 server，%d 个工具，%d 个资源\n", servers, tools, resources)
		return
	}
	fmt.Fprintf(os.Stdout, "· MCP external tools ready: %d server(s), %d tool(s), %d resource(s)\n", servers, tools, resources)
}

func rsMCPServers(rs *config.RuntimeSettings) []types.MCPServerConfig {
	if rs == nil {
		return nil
	}
	return rs.MCPServers
}

func operationProvidersFromMCPConfigs(reg *mcp.Registry, cfgs []types.MCPServerConfig) []operation.ProviderInfo {
	if reg == nil || len(cfgs) == 0 {
		return nil
	}
	registered := map[string]bool{}
	for _, name := range reg.List() {
		registered[name] = true
	}
	var out []operation.ProviderInfo
	for _, cfg := range cfgs {
		if cfg.OperationProvider == nil || !*cfg.OperationProvider {
			continue
		}
		name := strings.TrimSpace(cfg.Name)
		if name == "" || !registered[name] {
			continue
		}
		kinds := cleanOperationConfigList(cfg.OperationKinds)
		if len(kinds) == 0 {
			kinds = []string{"*"}
		}
		for _, kind := range kinds {
			out = append(out, operation.ProviderInfo{
				Name:         "mcp:" + name,
				Kind:         kind,
				Surfaces:     cleanOperationConfigList(cfg.OperationSurfaces),
				SideEffects:  cleanOperationConfigList(cfg.OperationSideEffects),
				RequiresGate: cfg.OperationRequiresConfirmation == nil || *cfg.OperationRequiresConfirmation,
			})
		}
	}
	return out
}

func cleanOperationConfigList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func formatStartupElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
