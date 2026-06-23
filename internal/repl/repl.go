// Package repl implements the interactive multi-turn loop for codrax.
//
// When the binary is launched with no --request, main.go hands control
// to this package. Each user line is dispatched as a fresh
// orchestrator.Run, with prior conversation injected into the request
// string via memory.Store.BuildContext. Slash commands manipulate the
// store directly without going through the orchestrator.
//
// Multi-line input is supported: end a line with \ to continue on the
// next line. The continuation lines are joined with newlines.
package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
	"github.com/hanchaoqun/codrax/internal/env"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Runner is the orchestrator-shaped surface the REPL needs. Defined
// here as an interface so tests can stub it without pulling in the
// full pipeline.
type Runner interface {
	Run(request, repoRoot, branch string) (*types.BusContext, error)
}

// MarkdownPreviewer registers the markdown transcript written by the
// orchestrator and returns a browser URL for the current REPL process.
// Kept as a small interface so tests can stub it and the REPL does not
// depend on the concrete HTTP server package.
type MarkdownPreviewer interface {
	RegisterMarkdown(path string) (string, error)
}

// runnerCanceller is the optional capability the REPL probes on its
// Runner to drive Ctrl+C / `/cancel`. Real orchestrators implement
// Cancel(reason); test stubs that omit it gracefully degrade —
// the SIGINT path falls through to the previous "exit on first
// signal" behaviour, and `/cancel` reports "no Run in flight".
type runnerCanceller interface {
	Cancel(reason string)
	IsCanceled() bool
}

// attachedLogSetter is the optional capability the REPL probes on
// its Runner to propagate a sticky /log attachment before dispatch.
// Real orchestrators implement SetAttachedLog; test stubs that omit
// it simply don't see log-triage attachments — the REPL falls back
// to the pre-flag behaviour.
type attachedLogSetter interface {
	SetAttachedLog(string)
}

// attachedHitraceSetter is the perf-channel companion to
// attachedLogSetter. The REPL's /htrace command propagates the
// captured trace body via this method before each dispatch so the
// orchestrator's perf_triage pre-stage can pick it up. Stubs that
// don't implement it cleanly degrade — /htrace becomes a no-op
// with a warning.
type attachedHitraceSetter interface {
	SetAttachedHitrace(string)
}

type attachedHitraceSourceSetter interface {
	SetAttachedHitraceSource(string)
}

// attachedHitraceGetter mirrors attachedLogGetter for the perf channel.
type attachedHitraceGetter interface {
	AttachedHitrace() string
}

type attachedHitraceSourceGetter interface {
	AttachedHitraceSource() string
}

// presentationDirectiveSetter is the typed current-turn display
// requirement channel. It deliberately stays out of the runner
// request string: the request string is user-authored objective text
// and feeds status lines, repo_map task maps, and memory. Real
// orchestrators copy this into BusContext.PresentationDirective so
// prompt construction can render it as scoped presentation metadata.
type presentationDirectiveSetter interface {
	SetPresentationDirective(string)
}

// turnRouteHintSetter is the typed current-turn routing metadata channel.
// It stays out of the request string and is consumed by analyzer prompt
// construction as advisory context only.
type turnRouteHintSetter interface {
	SetTurnRouteHint(types.TurnRouteHint)
}

// outputTranscriptRequestSetter is the REPL-only channel for the
// final-answer markdown/html transcript's "question" section. The
// pipeline request may include prior-conversation scaffolding, while
// REPL memory intentionally stores paste-folded display text. The
// transcript, however, should show the current user request exactly as
// dispatched after paste-placeholder expansion.
type outputTranscriptRequestSetter interface {
	SetOutputTranscriptRequest(string)
}

// readRunSnapshotSeedSetter is the explicit fail-loud recovery channel for
// /read-runs resume. It is intentionally not part of the base Runner surface.
type readRunSnapshotSeedSetter interface {
	SetReadRunSnapshotSeed(*types.ReadRunSnapshot)
}

// readRunSnapshotAutoSeedSetter is the soft routine resume channel for read
// turns with exact typed identity matches. Mismatch falls back to a fresh run.
type readRunSnapshotAutoSeedSetter interface {
	SetReadRunSnapshotAutoSeed(*types.ReadRunSnapshot)
}

// modeSetter is the optional capability the REPL probes on its
// Runner to propagate the current pipeline Mode before dispatch.
// Real orchestrators implement SetMode; test stubs that omit it
// simply always run read-mode regardless of the REPL's /mode
// state — the REPL degrades cleanly to pre-B0 behaviour. Session
// 33 Day 3 added SetMode to the real Orchestrator.
type modeSetter interface {
	SetMode(types.PipelineMode)
}

// planPathSetter is the companion capability for /approve: when the
// REPL triggers a second Run to consume a pending ChangePlan, it
// must first hand the file path to the orchestrator so
// the apply stage hook's type.LoadChangePlanFromFile step picks it up.
// B1.2 added SetPlanPath to the real Orchestrator; test stubs that
// omit it make /approve a no-op with a warning.
type planPathSetter interface {
	SetPlanPath(string)
}

type dataTaskTerminalAudit struct {
	Status        string
	Reason        string
	DataRounds    int
	RepairRounds  int
	Records       []dataTaskWorkflowRecord
	Result        *dataquery.Result
	ProcessEvents []dataworkflow.WorkflowJournalEvent
	Guards        []dataworkflow.GuardResult
}

type dataTaskTerminalAuditSnapshot = dataworkflow.WorkflowJournal

// autoInitRepoSetter is the optional capability `/approve` flips on
// after the user (or the yaml/CLI pre-authorization) consents to
// scaffolding a bare/headless target via `git init` + empty initial
// commit. The orchestrator setter primes the apply pre-hook to
// transparently transition through worktree.EnsureInitialCommit
// instead of fail-louding. Test stubs that omit this method are fine
// — auto-init only matters in real-orchestrator paths.
type autoInitRepoSetter interface {
	SetAutoInitRepo(bool)
}

// skipVerifySetter lets `/approve --skip-verify` short-circuit the
// verify stage for one Run. The orchestrator setter is scoped per-
// Run via defer-restore so the override doesn't leak across
// /approve invocations targeting different plans.
type skipVerifySetter interface {
	SetSkipVerify(bool)
}

// reuseWorktreeSetter is the capability `/verify <plan-id>` needs:
// before kicking off a ModeVerify Run against an applied plan, the
// REPL hands the orchestrator the preserved worktree path so the
// verify pre-hook swaps RepoRoot to it instead of running tests
// against the main repo. Without this, /verify would run tests
// against unmodified bytes, producing a misleading verdict.
type reuseWorktreeSetter interface {
	SetReuseWorktreePath(string)
}

// ResultRenderer turns a finished BusContext into the user-facing
// response text. main.go owns the canonical implementation.
type ResultRenderer func(*types.BusContext) string

// HitraceConvertFunc is the REPL's narrow conversion seam. Production uses
// hitraceconv.ConvertFile; tests inject a stub to assert slash-command option
// plumbing without fabricating a full binary trace container.
type HitraceConvertFunc func(context.Context, hitraceconv.Options) (hitraceconv.Result, error)

// Config holds all dependencies for constructing a REPL. Using a
// struct keeps the constructor readable as the field count grows and
// lets tests inject an io.Reader for scripted input.
type Config struct {
	Runner   Runner
	Store    *memory.Store
	Render   ResultRenderer
	Renderer *render.Renderer
	RepoRoot string
	Branch   string
	In       io.Reader // nil → interactive (bubbletea); non-nil → line-oriented
	Out      io.Writer

	// UI customization (used by line-oriented mode).
	Prompt           string // primary prompt, e.g. ">"
	PromptCont       string // continuation prompt, e.g. "."
	Banner           string // printed once at start; empty → default badge
	HeaderPrinted    bool   // true when cmd already rendered the CODRAX startup header
	ModelListLine    string // optional model-list summary shown before model config
	ModelSummaryLine string // optional resolved model summary

	// PasteFoldMinChars is the rune-count threshold above which a
	// single-line paste gets folded into a placeholder. Multi-line
	// pastes always fold regardless of length. Zero or negative →
	// DefaultPasteFoldMinChars. Surfaces
	// codrax.yaml :: repl_paste_fold_min_chars.
	PasteFoldMinChars int

	// Version and BuildTime are the build-stamped identifiers passed
	// from cmd. Empty strings render as "dev" / "unknown" so a bare
	// `go run` still produces a coherent banner / /version line.
	Version   string
	BuildTime string

	// Language toggles banner hint text between zh and en. Mirrors
	// codrax.yaml `lang` / CLI `--lang`. Only "en" (case-insensitive)
	// flips to English; every other value — including empty, "zh",
	// "off", "fr" — renders zh (the codrax.yaml default).
	Language string

	// MarkdownPreview is optional REPL UX: when the final answer markdown
	// transcript is written, this registers the file and prints a browser
	// preview URL below the existing "Markdown saved" line. Nil disables
	// preview hints without affecting output dumping.
	MarkdownPreview MarkdownPreviewer

	// OutputDumpDir and OutputDumpMax mirror the orchestrator's final
	// answer transcript dump settings for REPL-local answer paths
	// (local transform / summarize / translate and /chat). Pipeline
	// runs still write via the orchestrator so BusContext can carry the
	// path; local paths write here because they bypass Runner.Run.
	OutputDumpDir string
	OutputDumpMax int

	// HitraceConvert optionally overrides the /htrace convert executor.
	// Nil uses hitraceconv.ConvertFile.
	HitraceConvert HitraceConvertFunc

	// ChitchatResponder handles the /chat slash command. When nil,
	// /chat prints a "not configured" warning and takes no LLM action
	// — the command is recognised but inert. cmd/root.go constructs
	// the default LLM-backed responder from providers.yaml when
	// codrax.yaml `chitchat_enabled: true` (the default). Exposing
	// this as an interface lets unit tests inject a deterministic stub
	// without needing an LLM.
	ChitchatResponder ChitchatResponder

	// Memory is the read-side handle into the conversation memory
	// store. cmd/root.go wires memory.NewAdapter(store) here so the
	// chitchat tool-use loop can call recall_memory directly. nil
	// disables the tool-use path (chitchat falls back to single-shot
	// Chat with the keyword-injected priorContext only).
	Memory types.MemoryReader

	// EnvSettings forwards codrax.yaml's env_recommend_* knobs into
	// the REPL so /env probe / explain / cache use the right
	// timeouts and strategy filters. Zero value falls through to
	// types.DefaultEnvRecommendSettings via ResolvedEnvRecommendSettings.
	EnvSettings types.EnvRecommendSettings

	// ColorMode controls ANSI escape emission for diff rendering
	// (and any other code blocks added later). Default ColorAuto:
	// on for TTY, off for pipes. NO_COLOR env wins over everything.
	// Surfaces --color={auto,always,never} on the CLI.
	ColorMode render.ColorMode

	// ChitchatClassifier optionally runs a single LLM call before each
	// normal dispatch to decide whether to reroute the turn to the
	// chit-chat path. nil disables the gate; the REPL falls back to
	// explicit /chat only. Requires ChitchatResponder to be non-nil;
	// cmd/root.go ties both wires together via codrax.yaml
	// `chitchat_classifier_enabled: true`. Fail-safe: any classifier
	// error routes to the pipeline unchanged.
	ChitchatClassifier ChitchatClassifier
	// UserMode is the explicit user-facing task lane. Auto preserves the
	// existing classifier-driven route; code/operation/data/write bypass
	// classification for users who know which lane they want.
	UserMode UserMode

	// OperationEnabled is the feature gate for the future independent
	// computer-operation / artifact-generation route. Batch 1 wires only
	// classification safety: when false, route=operation is refused at the
	// REPL surface instead of falling into the source-analysis pipeline.
	OperationEnabled bool
	// OperationProviders are optional side-effect capable providers for the
	// operation route. Empty means plan-only: REPL shows the typed operation
	// plan and stops before execution.
	OperationProviders []operation.ProviderInfo
	// OperationPlanner is an optional LLM-backed command planner for
	// route=operation + operation_kind=computer_operation.
	OperationPlanner CommandOperationPlanner
	// DataTaskPlanner is an optional LLM-backed planner for route=data. It is
	// intentionally independent from the source-analysis pipeline and the
	// command-operation approval loop.
	DataTaskPlanner DataTaskPlanner
	// DataMaterialExtractor is an optional multimodal extractor used only when
	// a data plan declares non-text materials as required and no text evidence
	// is available to the deterministic runner.
	DataMaterialExtractor DataMaterialExtractor
	// DataTaskMaxRepairRounds bounds script-failure repair attempts in the
	// read-only data lane. Zero uses DefaultDataTaskMaxRepairRounds.
	DataTaskMaxRepairRounds int
	// DataTaskMaxDataRounds bounds execute/evaluate/continue batches in the
	// read-only data lane. Zero uses DefaultDataTaskMaxDataRounds.
	DataTaskMaxDataRounds int
	// OperationCommandPolicy controls command-operation approval/execution.
	OperationCommandPolicy operation.CommandPolicy
	// MCPServers is used only for explicitly configured operation providers.
	// Explorer/read-mode MCP exposure continues to live in the agent layer.
	MCPServers *mcp.Registry
	// MCPServerConfigs lets explicitly lazy operation providers start only
	// after a user approves the operation plan. Eager MCP exposure remains
	// owned by cmd/root.go and the agent layer.
	MCPServerConfigs []types.MCPServerConfig
	// OperationSkillConfigs are local manifest-backed operation providers. They
	// are descriptors until typed operation routing plus approval reaches the
	// provider execution path.
	OperationSkillConfigs []types.OperationSkillConfig
	// OperationPendingStore persists one unresolved operation-lane decision so
	// restarts can surface pending approvals/clarifications. It is independent
	// from write-mode PlanStore.
	OperationPendingStore *OperationPendingStore

	// PlanStore persists B0 write-mode ChangePlans for the REPL
	// session. Nil disables the /plan slash command family —
	// useful for tests and for single-shot invocations that never
	// construct a REPL. cmd/root.go wires a real store under
	// <runtime-anchor>/plans when the REPL starts.
	PlanStore *PlanStore

	// PlanGroupStore persists stage II multi-phase PlanGroups
	// for the REPL's /phase slash command family. Nil disables
	// /phase entirely — single-phase plans remain fully usable
	// via /plan / /approve as before. cmd/root.go wires a real
	// store under <runtime-anchor>/plans/groups/ alongside
	// PlanStore.
	PlanGroupStore *PlanGroupStore

	// WriteWorkflowRunStore persists the controller-engine write
	// workflow DAG under <runtime-anchor>/plans/workflows/. Nil
	// disables the write half of /workflow while leaving operation
	// provider workflows unchanged.
	WriteWorkflowRunStore *WriteWorkflowRunStore

	// ReadRunSnapshotStore persists typed read-mode execution snapshots
	// under <runtime-anchor>/plans/read_runs/. Nil disables the advanced
	// /read-runs audit command family.
	ReadRunSnapshotStore *ReadRunSnapshotStore

	// FailureTaxonomy is the stage-3 reader interface for
	// /pitfalls inspection. The REPL only reads (list / clear);
	// the orchestrator owns Append. Nil = /pitfalls reports
	// "feature disabled."
	FailureTaxonomy FailureTaxonomyReader

	// AttachedLogMaxBytes caps every REPL attach surface (`/log
	// <path>`, `/log` paste mode, splitPastedLog auto-route) so a
	// runaway paste cannot balloon the REPL process memory. Mirrors
	// cmd's maxAttachedLogBytes — both are driven by
	// codrax.yaml :: log_attach_max_bytes. Zero or negative →
	// DefaultAttachedLogMaxBytes (512 MiB), matching the CLI default.
	AttachedLogMaxBytes int

	// AttachedTraceMaxBytes caps the perf-channel attach surface
	// (`/htrace <path>` and `/htrace append <path>` and the
	// `/atrace` aliases). Defaults to AttachedLogMaxBytes when zero
	// or negative — a user who only configures the log cap still
	// gets symmetric trace handling. Set independently to override.
	AttachedTraceMaxBytes int

	// SettingsPath is the resolved codrax.yaml the CLI loaded (or "" if
	// none was found). Surfaced verbatim by the L2 gate's reject
	// message so the user knows WHICH file to edit. Optional; empty
	// falls back to a generic "in codrax.yaml" phrasing.
	SettingsPath string

	// WriteEnabled mirrors codrax.yaml :: write_enabled. Gates every
	// REPL transition into a non-read mode (`/mode write|apply|verify`,
	// `/approve`). When false, the REPL refuses the transition with a
	// clear error pointing at the yaml knob — the alternative was the
	// pre-fix silent state where /mode write accepted, the planner
	// dispatched, the analyzer failed in a confusing way ("hypothesis
	// coverage" / "context canceled"), and the user had no idea
	// write_enabled was the cause. cmd/root.go forwards
	// runtime_settings.WriteEnabled.
	WriteEnabled bool

	// WriteApprovalPolicy controls /approve for write ChangePlans.
	// Defaults to auto_safe when empty. It is independent from command
	// operation approval and from write_enabled's capability gate.
	WriteApprovalPolicy writeflow.ApprovalPolicy

	// WriteAutoInitRepo mirrors the resolved auto-init authorization
	// (yaml `write_auto_init_repo` OR CLI `--auto-init-repo`). When
	// true, the REPL's /approve flow skips the interactive y/N
	// consent prompt for bare/headless repos and silently
	// authorizes the orchestrator to scaffold. When false (default),
	// /approve runs DetectRepoState before dispatching and prompts
	// for consent if the target needs init.
	WriteAutoInitRepo bool

	// WriteScaffoldEnabled mirrors the resolved scaffold authorization
	// (yaml `write_scaffold_enabled` OR CLI `--allow-scaffold`). When
	// true, the orchestrator's plan pre-hook tolerates an empty target
	// directory; without it, empty-dir plan/apply dispatches fail-loud
	// with a hint at the two authorization surfaces. Forwarded into
	// the orchestrator on REPL boot so REPL sessions inherit the
	// startup-time authorization without the user having to repeat
	// the flag.
	WriteScaffoldEnabled bool

	// RuntimeAnchor is <CWD>/.codrax/ — used by /worktree gc to
	// locate the worktree base under <RuntimeAnchor>/worktrees/.
	// Empty disables the gc subcommand (the slash dispatcher
	// surfaces a typed warning).
	RuntimeAnchor string

	// WorktreeKeepTTL + WorktreeKeepMaxCount mirror the resolved
	// codrax.yaml knobs so /worktree gc uses the same caps as
	// startup. Zero on either disables that gate.
	WorktreeKeepTTL      time.Duration
	WorktreeKeepMaxCount int

	// Topology is the multi-repo discovery snapshot for RepoRoot.
	// Populated by cmd/root.go::initApp; consumed by /repos. Nil
	// disables the slash command (handler surfaces a typed warning).
	Topology *topology.RepoTopology

	// MultiRepoEnabled mirrors codrax.yaml :: multi_repo_enabled.
	// When false, /repos still works (it shows a hint pointing at
	// the yaml gate) but Phase 4 multigraph routing is bypassed.
	MultiRepoEnabled bool

	// MultiRepoMaxActive mirrors codrax.yaml :: multi_repo_max_active.
	// /repos cap <N> overrides this value session-locally.
	MultiRepoMaxActive int

	// MultiRepoInactivePreviewCount mirrors codrax.yaml ::
	// multi_repo_inactive_preview_count. Threaded through to BusContext
	// so context/builder.go's L0 LLM advisory can decide how many
	// out-of-active sub-repos to surface in the prompt.
	MultiRepoInactivePreviewCount int

	// Multigraph is the cmd/root-supplied carrier handle the REPL
	// reads to render the /repos listing (auto-active row marker
	// + color). Stored as any so the REPL stays free of an
	// internal/tool/repomap/multigraph import. The REPL probes for
	// the ActiveSlugSnapshot() method via type-assert; nil disables
	// the auto-active marker so the listing falls back to the
	// 2-state pinned/inactive view.
	Multigraph any

	// InitialFocusSlugs pre-populates the session focus pin map at
	// REPL boot. Used by the --focus CLI flag (cmd/root.go); REPL
	// users who pin via /repos focus reach the same map through the
	// command handler. Empty / nil = no pin (default REPL boot).
	InitialFocusSlugs []string

	// OnMultiRepoFocusChange is invoked by the REPL when the user
	// runs /repos focus or /repos unfocus. cmd/root.go wires this to
	// app.multigraph.SetFocus so the next Run picks up the change.
	// Nil disables the propagation (focus state stays REPL-local).
	OnMultiRepoFocusChange func(slugs []string)

	// OnMultiRepoCapChange is invoked by /repos cap. cmd/root.go
	// wires this to app.multigraph.SetCap.
	OnMultiRepoCapChange func(n int)

	// OnMultiRepoRefresh is invoked by /repos refresh. cmd/root.go
	// rebuilds app.multigraph from the freshly-discovered topology
	// so the next Run sees the new sub-repo set.
	OnMultiRepoRefresh func(newTopology *topology.RepoTopology)
}

// REPL drives the interactive prompt.
type REPL struct {
	runner                    Runner
	store                     *memory.Store
	render                    ResultRenderer
	renderer                  *render.Renderer
	repoRoot                  string
	runtimeAnchor             string
	activeDataWorkflowRuntime *dataworkflow.WorkflowRuntime
	worktreeKeepTTL           time.Duration
	worktreeKeepMaxCount      int

	// Multi-repo state. topology is a pointer because /repos refresh
	// rebuilds the snapshot in place by swapping the pointer (the
	// embedded slice is treated as immutable per-snapshot, so older
	// references don't tear). multiRepoFocus is the session-local
	// pin set populated by /repos focus <slug>; Phase 4 routing
	// reads it through ActiveSubRepoFocus(). multiRepoMaxActiveOverride
	// is 0 when no /repos cap <N> has been issued (use Config value),
	// else the session-local override.
	topology         *topology.RepoTopology
	multiRepoEnabled bool

	// multigraphForListing is the cmd/root-supplied handle the REPL
	// uses to read the LRU-resident slug set when rendering /repos.
	// Stored as any so this package stays free of an
	// internal/tool/repomap/multigraph import (which would create a
	// cycle through the repomap package). The type-assert in
	// multiRepoLRUSnapshot probes for the ActiveSlugSnapshot method;
	// nil here disables the auto-active marker (the listing falls
	// back to a 2-state pinned/inactive view).
	multigraphForListing       any
	multiRepoMaxActive         int // canonical value (Config.MultiRepoMaxActive)
	multiRepoFocus             map[string]bool
	multiRepoMaxActiveOverride int
	multiRepoMu                sync.Mutex // guards focus + override + topology pointer
	onMultiRepoFocusChange     func(slugs []string)
	onMultiRepoCapChange       func(n int)
	onMultiRepoRefresh         func(newTopology *topology.RepoTopology)

	// approveSuccessCount is the per-session tally of clean
	// /approve completions (apply + verify both green). Used
	// by maybeNudgeWorktreeGC (commit 46) to surface a soft
	// hint every Nth success when preserved worktrees might
	// be accumulating on disk. Reset each REPL boot.
	approveSuccessCount int
	branch              string
	in                  io.Reader
	out                 io.Writer
	prompt              string
	promptCont          string
	bannerText          string
	headerPrinted       bool
	modelListLine       string
	modelSummaryLine    string
	scanner             *bufio.Scanner // lazy-init for line-oriented mode
	pasteFoldMinChars   int            // per-session paste-fold threshold (runes)
	version             string
	buildTime           string
	language            string
	markdownPreview     MarkdownPreviewer
	outputDumpDir       string
	outputDumpMax       int
	hitraceConvert      HitraceConvertFunc

	// attachedLog holds the runtime log excerpt the user attached.
	// Lifetime depends on how it got here:
	//   - explicit /log or `--log`: sticky across turns until /log
	//     clear — users typically investigate the same panic from
	//     several angles ("root cause?" → "safe fix?" → "regression
	//     risk?").
	//   - splitPastedLog auto-route: one-shot, cleared after the
	//     dispatch it was attached for (see attachedLogAutoRouted).
	// Propagated to the runner via attachedLogSetter before each
	// dispatch.
	attachedLog string

	// attachedHitrace mirrors attachedLog for the HiTrace channel. Set
	// by /htrace (sticky across turns until /htrace clear). Sent to the
	// orchestrator via attachedHitraceSetter before every dispatch so
	// the perf_triage pre-stage can pick it up.
	attachedHitrace       string
	attachedHitraceSource string
	pendingHitraceSource  string

	// attachedLogAutoRouted marks attachedLog as installed by the
	// splitPastedLog auto-routing path (pasted log body detected
	// inline in a request), as opposed to the explicit /log or
	// --log flow. Auto-routed attachments are one-shot: cleared
	// after the dispatch that consumed them so log_triage does not
	// re-run on every subsequent turn. Explicit /log remains sticky
	// because the user's intent there is a sustained investigation.
	attachedLogAutoRouted bool

	// pendingPaste is the line-oriented fallback for terminals /
	// tmux configurations that swallow bracketed paste (most common
	// in SSH → tmux environments). `/paste` captures lines via a
	// plain bufio.Scanner until `/end`; the captured content is
	// stashed here, and the next interactive readInput call injects
	// it into the new Bubble Tea model as a placeholder seed. Single-
	// use; cleared on consumption or on abort of the seeded turn.
	pendingPaste string

	// envFacts is the cached environment snapshot the /env command
	// surfaces and the chitchat/pipeline path can use for diagnosis
	// when a runner failure surfaces. Populated lazily on first
	// /env call; refreshed by /env probe.
	envFacts *types.EnvFacts

	// envSettings carries the resolved env_recommend yaml config.
	// cmd/root.go populates this before REPL Loop starts.
	envSettings types.EnvRecommendSettings

	// colorMode is the resolved color policy for diff rendering.
	// Default ColorAuto inherits TTY detection at write time. Set
	// from cmd/root.go via Config.ColorMode (ultimately from --color).
	colorMode render.ColorMode

	// memory is the read-side handle into the conversation memory
	// store the chitchat tool-use loop hands the responder. Wired by
	// cmd/root.go from memory.NewAdapter(store) so the responder can
	// invoke recall_memory inline. Nil disables the tool-use path
	// (chitchat falls back to the legacy single-shot Chat call).
	memory types.MemoryReader

	// chitchatResponder is the /chat handler; nil means the feature
	// is disabled and /chat prints a warning. See Config for wiring.
	chitchatResponder ChitchatResponder

	// chitchatClassifier optionally runs before every normal dispatch
	// to reroute casual turns to the chit-chat responder. nil disables
	// the gate. See Config.ChitchatClassifier for wiring.
	chitchatClassifier ChitchatClassifier

	// operationEnabled gates the future independent operation/artifact
	// route. When false, a structured route=operation is surfaced as a
	// capability-not-enabled message and never falls through to the repo
	// analysis pipeline.
	operationEnabled            bool
	operationProviders          []operation.ProviderInfo
	operationPlanner            CommandOperationPlanner
	dataTaskPlanner             DataTaskPlanner
	dataMaterialExtractor       DataMaterialExtractor
	dataTaskMaxRepairRounds     int
	dataTaskMaxDataRounds       int
	operationPolicy             operation.CommandPolicy
	pendingOperation            *operation.CommandOperationPlan
	pendingCommandClarification *pendingCommandClarification
	pendingProviderOperation    *pendingProviderOperation
	providerWorkflow            *operation.WorkflowInstance
	operationHistory            []operation.CommandOperationPlan
	operationResults            []commandOperationResultRecord
	providerOperationResults    []providerOperationResultRecord
	operationMemory             *operation.MemoryStore
	lastAnswerOrigin            replAnswerOrigin
	operationPendingStore       *OperationPendingStore
	mcpServers                  *mcp.Registry
	mcpServerConfigs            []types.MCPServerConfig
	operationSkillConfigs       []types.OperationSkillConfig

	// sessionID identifies this REPL session. Stamped onto every
	// recorded Turn so memory.BuildContext can session-pin recent
	// turns (keep them in prior-conversation context even when
	// keyword matching would drop them). Set once at New() from
	// time.Now().UnixNano() + pid; stable for the lifetime of the
	// REPL. Format: sess-<nano>-<pid>.
	sessionID string

	// currentMode is the B0 sticky pipeline mode for this REPL
	// session. Zero value is ModeRead (preserves pre-B0 REPL
	// behaviour). `/mode <x>` rewrites it; every subsequent
	// dispatch calls runner.SetMode(currentMode) before Run so
	// the orchestrator sees the up-to-date value. The REPL never
	// auto-downgrades — once user enters plan mode they stay there
	// until explicit /mode auto.
	currentMode types.PipelineMode

	// userMode is the sticky user-facing task lane. Auto preserves the
	// classifier-driven route; code/operation/data/write are explicit
	// escape hatches that do not parse raw user prose.
	userMode UserMode

	// settingsPath is the resolved codrax.yaml path (or "" when no
	// yaml was found / loaded). Surfaced verbatim by the write_enabled
	// gate's error message so the user knows WHICH file to edit.
	// Without this, "set write_enabled: true in codrax.yaml" left the
	// user hunting through three default lookup locations.
	settingsPath string

	// echoTag is the sticky-state marker (promptStickyTag output) for
	// the current turn. Loop sets this BEFORE each readInputPair so
	// the Bubble Tea echo can re-render the same chrome above the
	// inline viewport. Empty (read mode + no attachments) means no
	// prefix on the echo line.
	echoTag string

	// writeEnabled mirrors Config.WriteEnabled. Read by handleModeCmd
	// (rejects `/mode write|apply|verify`) and by handleApprove (rejects
	// /approve) so a user with `write_enabled: false` in codrax.yaml
	// gets a clean error at the slash-command surface instead of a
	// confusing analyzer/planner failure deep inside the pipeline.
	writeEnabled bool

	// writeApprovalPolicy is the resolved write-mode approval policy for
	// /approve. Critical risk denies before apply; high risk remains manual;
	// low/medium can auto-run under auto_safe.
	writeApprovalPolicy writeflow.ApprovalPolicy

	// writeAutoInitRepo mirrors Config.WriteAutoInitRepo. When true
	// the /approve flow skips the y/N consent prompt and silently
	// authorizes the orchestrator's auto-init path on bare /
	// commitless repos. When false, /approve calls
	// worktree.DetectRepoState first; if NeedsInit() returns true,
	// the user is prompted before any state mutation.
	writeAutoInitRepo bool

	// writeScaffoldEnabled mirrors Config.WriteScaffoldEnabled.
	// Forwarded once into the orchestrator at REPL boot so plan/apply
	// dispatches against an empty target directory honour the
	// startup-time scaffold authorization. The REPL itself does NOT
	// consult this field — the orchestrator's planPreHook owns the
	// gate; storing it on the REPL only keeps the value reachable
	// for diagnostics if the gate fails.
	writeScaffoldEnabled bool

	// pendingPlanPath is the filesystem path of the last plan
	// auto-saved by this REPL (via planStore.Save after a
	// successful plan-mode dispatch). Used by /plan show to
	// display the current pending plan without re-reading the
	// filesystem. Cleared by /plan clear or by a fresh plan
	// emission. Empty string means "no pending plan in this
	// session".
	pendingPlanPath string

	// planStore persists ChangePlans to disk so /plan show / clear
	// survive across REPL turns. Nil disables the /plan command
	// family — cmd/root.go always wires a real store in runREPL,
	// but tests that bypass cmd/ keep it nil.
	planStore *PlanStore

	// planGroupStore persists multi-phase PlanGroups for the
	// /phase slash command family (stage II). Nil disables
	// /phase. cmd/root.go wires a real store alongside
	// planStore.
	planGroupStore *PlanGroupStore

	// writeWorkflowRunStore persists controller-engine write
	// workflow DAGs. /workflow show/list reads this store; /approve
	// and /reject use it only to bind the active batch's typed
	// PlanID when the operator did not pass a plan id explicitly.
	writeWorkflowRunStore *WriteWorkflowRunStore

	// readRunSnapshotStore backs the advanced /read-runs audit command.
	// Routine read mode does not auto-resume from this store.
	readRunSnapshotStore *ReadRunSnapshotStore

	// failureTaxonomy is the read-side handle for /pitfalls.
	// Same interface the orchestrator's FailureTaxonomyStore
	// satisfies; REPL only reads (list / clear).
	failureTaxonomy FailureTaxonomyReader

	// attachedLogMaxBytes is the per-session cap on every log-channel
	// attach surface (/log <path>, /log paste, /log append,
	// splitPastedLog auto-route). Seeded from
	// Config.AttachedLogMaxBytes; non-positive falls back to
	// DefaultAttachedLogMaxBytes.
	attachedLogMaxBytes int

	// attachedTraceMaxBytes is the perf-channel companion. Used by
	// /htrace and /atrace handlers. Seeded from
	// Config.AttachedTraceMaxBytes; non-positive falls back to
	// attachedLogMaxBytes (so the trace channel inherits whatever
	// the log channel resolved to).
	attachedTraceMaxBytes int

	// runInFlight reports whether the REPL is currently inside a
	// runner.Run() call. Set true around dispatchTurn / handleApprove
	// / handleChat etc.; cleared when the call returns. Drives the
	// SIGINT handler's behaviour: in-flight + first signal → cancel
	// the Run; in-flight + second signal within doubleSigCancelWindow
	// → re-raise so worktree cleanup + os.Exit can run; idle → ignore
	// (bubbletea / readline owns the input line's Ctrl+C semantics).
	runInFlight   atomic.Bool
	cancelSigOnce sync.Once    // installs the signal handler exactly once per REPL lifetime
	lastCancelSig atomic.Int64 // unix nano of the previous Ctrl+C while in-flight; backs the double-tap escalation

	// turnCancelMu + turnCancel form the per-turn HTTP-level
	// cancellation surface for non-pipeline paths (chitchat
	// classifier + responder). The signal handler invokes turnCancel
	// when Ctrl+C arrives during a chitchat turn so the in-flight
	// LLM HTTP request unwinds immediately rather than waiting for
	// its natural completion. Pipeline turns don't use this — they
	// route through orchestrator's CancelToken which has its own
	// ctx already plumbed via BusContext.Ctx.
	turnCancelMu sync.Mutex
	turnCancel   context.CancelFunc
}

// doubleSigCancelWindow caps how long we accept a second Ctrl+C as
// "really exit codrax". Outside this window the second signal is a
// fresh cancel (idempotent — the runner already canceled). Two
// seconds matches the convention every Python REPL / Jupyter /
// ipython established; operators don't have to retrain.
const doubleSigCancelWindow = 2 * time.Second

// New constructs a REPL from a Config.
func New(cfg Config) *REPL {
	r := &REPL{
		runner:                  cfg.Runner,
		store:                   cfg.Store,
		render:                  cfg.Render,
		renderer:                cfg.Renderer,
		repoRoot:                cfg.RepoRoot,
		runtimeAnchor:           cfg.RuntimeAnchor,
		worktreeKeepTTL:         cfg.WorktreeKeepTTL,
		worktreeKeepMaxCount:    cfg.WorktreeKeepMaxCount,
		topology:                cfg.Topology,
		multiRepoEnabled:        cfg.MultiRepoEnabled,
		multiRepoMaxActive:      cfg.MultiRepoMaxActive,
		multigraphForListing:    cfg.Multigraph,
		multiRepoFocus:          focusMapFromSlugs(cfg.InitialFocusSlugs),
		onMultiRepoFocusChange:  cfg.OnMultiRepoFocusChange,
		onMultiRepoCapChange:    cfg.OnMultiRepoCapChange,
		onMultiRepoRefresh:      cfg.OnMultiRepoRefresh,
		branch:                  cfg.Branch,
		in:                      cfg.In,
		out:                     cfg.Out,
		prompt:                  cfg.Prompt,
		promptCont:              cfg.PromptCont,
		bannerText:              cfg.Banner,
		headerPrinted:           cfg.HeaderPrinted,
		modelListLine:           cfg.ModelListLine,
		modelSummaryLine:        cfg.ModelSummaryLine,
		pasteFoldMinChars:       cfg.PasteFoldMinChars,
		version:                 cfg.Version,
		buildTime:               cfg.BuildTime,
		language:                cfg.Language,
		markdownPreview:         cfg.MarkdownPreview,
		outputDumpDir:           cfg.OutputDumpDir,
		outputDumpMax:           cfg.OutputDumpMax,
		hitraceConvert:          cfg.HitraceConvert,
		chitchatResponder:       cfg.ChitchatResponder,
		memory:                  cfg.Memory,
		envSettings:             types.ResolvedEnvRecommendSettings(cfg.EnvSettings),
		colorMode:               cfg.ColorMode,
		chitchatClassifier:      cfg.ChitchatClassifier,
		operationEnabled:        cfg.OperationEnabled,
		operationProviders:      append([]operation.ProviderInfo(nil), cfg.OperationProviders...),
		operationPlanner:        cfg.OperationPlanner,
		dataTaskPlanner:         cfg.DataTaskPlanner,
		dataMaterialExtractor:   cfg.DataMaterialExtractor,
		dataTaskMaxRepairRounds: normalizeDataTaskMaxRepairRounds(cfg.DataTaskMaxRepairRounds),
		dataTaskMaxDataRounds:   normalizeDataTaskMaxDataRounds(cfg.DataTaskMaxDataRounds),
		operationPolicy:         cfg.OperationCommandPolicy,
		mcpServers:              cfg.MCPServers,
		mcpServerConfigs:        append([]types.MCPServerConfig(nil), cfg.MCPServerConfigs...),
		operationSkillConfigs:   append([]types.OperationSkillConfig(nil), cfg.OperationSkillConfigs...),
		operationPendingStore:   cfg.OperationPendingStore,
		// Session ID embeds nano + pid so two codrax REPLs launched
		// in the same clock tick (test harness, race) still get
		// disjoint IDs. Consumed by memory.BuildContext via BuildOpts.
		sessionID:             fmt.Sprintf("sess-%d-%d", time.Now().UnixNano(), os.Getpid()),
		currentMode:           types.ModeRead, // B0 sticky mode; /mode rewrites
		userMode:              cfg.UserMode.Normalize(),
		planStore:             cfg.PlanStore,
		planGroupStore:        cfg.PlanGroupStore,
		writeWorkflowRunStore: cfg.WriteWorkflowRunStore,
		readRunSnapshotStore:  cfg.ReadRunSnapshotStore,
		failureTaxonomy:       cfg.FailureTaxonomy,
		attachedLogMaxBytes:   cfg.AttachedLogMaxBytes,
		attachedTraceMaxBytes: cfg.AttachedTraceMaxBytes,
		writeEnabled:          cfg.WriteEnabled,
		writeApprovalPolicy:   normalizeREPLWriteApprovalPolicy(cfg.WriteApprovalPolicy),
		writeAutoInitRepo:     cfg.WriteAutoInitRepo,
		writeScaffoldEnabled:  cfg.WriteScaffoldEnabled,
		settingsPath:          cfg.SettingsPath,
	}
	r.applyPtermColorMode()
	if r.hitraceConvert == nil {
		r.hitraceConvert = hitraceconv.ConvertFile
	}
	if r.version == "" {
		r.version = "dev"
	}
	if !r.userMode.IsValid() {
		r.userMode = UserModeAuto
	}
	if r.userMode == UserModeWrite {
		r.currentMode = types.ModeApply
	}
	if r.buildTime == "" {
		r.buildTime = "unknown"
	}
	if r.attachedLogMaxBytes <= 0 {
		r.attachedLogMaxBytes = DefaultAttachedLogMaxBytes
	}
	// Trace cap inherits the (now-resolved) log cap when not
	// explicitly set, so a caller that only configured
	// AttachedLogMaxBytes still gets symmetric perf-channel
	// behaviour.
	if r.attachedTraceMaxBytes <= 0 {
		r.attachedTraceMaxBytes = r.attachedLogMaxBytes
	}
	if isZeroOperationCommandPolicy(r.operationPolicy) {
		r.operationPolicy = operation.DefaultCommandPolicy()
	}
	if strings.TrimSpace(r.operationPolicy.DefaultWorkDir) == "" {
		r.operationPolicy.DefaultWorkDir = r.repoRoot
	}
	if strings.TrimSpace(r.runtimeAnchor) != "" {
		r.operationMemory = operation.NewMemoryStore(filepath.Join(r.runtimeAnchor, "operation", "memory.jsonl"))
	}
	r.restorePendingOperationState()
	// Seed sticky log from whatever the runner already has (CLI set
	// `--log` before handing off to the REPL). Keeps the invariant
	// "REPL.attachedLog is the single source of truth" from the
	// first /log show onward.
	if getter, ok := cfg.Runner.(interface{ AttachedLog() string }); ok {
		r.attachedLog = getter.AttachedLog()
	}
	if getter, ok := cfg.Runner.(attachedHitraceGetter); ok {
		r.attachedHitrace = getter.AttachedHitrace()
	}
	if getter, ok := cfg.Runner.(attachedHitraceSourceGetter); ok {
		r.attachedHitraceSource = getter.AttachedHitraceSource()
	}
	return r
}

// interactive returns true when the REPL is attached to an
// interactive terminal (huh-based input), false when driven by a
// scripted io.Reader.
func (r *REPL) interactive() bool { return r.in == nil }

// REPL prefix printers — subdued variants of pterm's defaults.
// pterm.Info / Success / Warning / Error use the library's themed
// styles which paint a bright-white-on-coloured-background block
// for the prefix label and bold the message body. On screenshots
// the result reads as "the REPL is shouting at me about an info
// message" — a UX cost the codrax style guide consistently avoids.
//
// These replacements:
//   - drop the background colour entirely (foreground-only prefix)
//   - use a single Unicode symbol instead of a 4-7 char label
//     (less visual noise, same intent legibility)
//   - leave the message body in the terminal's default text style
//     so the user's chrome / theme stays in control
//
// All four printers carry a coloured glyph (so the user can scan
// for the meaning at a glance) and a FgDarkGray message body —
// the body is informational chrome, not the answer, and should
// recede visually. Customer feedback was that the prior shape
// (default-coloured body text) "抢眼" / dominated attention next
// to the bordered final answer.
var (
	subduedInfoPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgCyan),
			Text:  "•",
		},
		MessageStyle: pterm.NewStyle(pterm.FgDarkGray),
	}
	subduedSuccessPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgGreen),
			Text:  "✓",
		},
		MessageStyle: pterm.NewStyle(pterm.FgDarkGray),
	}
	subduedWarnPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgYellow),
			Text:  "·",
		},
		MessageStyle: pterm.NewStyle(pterm.FgDarkGray),
	}
	subduedErrorPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgRed),
			Text:  "✗",
		},
		// Errors keep the default text colour — when the REPL
		// surfaces an error it's a real failure the user must
		// notice. Info / success / warn are cosmetic chrome and
		// stay dim; errors do not.
	}
)

// info prints an informational message. In interactive mode it uses
// the subdued cyan-prefix printer; in line-oriented mode it writes
// plain text to r.out so tests can capture it.
func (r *REPL) info(msg string) {
	if r.interactive() {
		subduedInfoPrinter.Println(msg)
	} else {
		fmt.Fprintln(r.out, msg)
	}
}

// success prints a success message.
func (r *REPL) success(msg string) {
	if r.interactive() {
		subduedSuccessPrinter.Println(msg)
	} else {
		fmt.Fprintln(r.out, msg)
	}
}

// errorf prints an error message.
func (r *REPL) errorf(format string, args ...interface{}) {
	if r.interactive() {
		subduedErrorPrinter.Printf(format, args...)
	} else {
		fmt.Fprintf(r.out, format, args...)
	}
}

// warn prints a warning message.
func (r *REPL) warn(format string, args ...interface{}) {
	if r.interactive() {
		subduedWarnPrinter.Printf(format, args...)
	} else {
		fmt.Fprintf(r.out, format, args...)
	}
}

// memoryUnderPressure reports whether the in-memory recent-turns
// buffer or index has crossed a soft threshold beyond which retrieval
// quality begins to degrade. The thresholds are deliberately generous
// — they're meant to catch "pasted three 50KB stack traces over an
// hour" pathologies, not normal usage.
//
// Heuristic: 30+ recent turns OR 60+ index entries. Both are well
// above typical session sizes (most sessions stop at <10 turns) but
// well below the hard memory-store cap. Once tripped, the prompt
// shows [mem!] and a one-shot nudge runs.
func (r *REPL) memoryUnderPressure() bool {
	if r.store == nil {
		return false
	}
	const (
		recentThreshold = 30
		indexThreshold  = 60
	)
	return len(r.store.Recent()) >= recentThreshold ||
		len(r.store.Index()) >= indexThreshold
}

// memoryStats returns (recentTurns, indexEntries) for the pressure
// nudge to surface concrete counts. Returns (0, 0) when the store
// isn't wired (test fixtures).
func (r *REPL) memoryStats() (int, int) {
	if r.store == nil {
		return 0, 0
	}
	return len(r.store.Recent()), len(r.store.Index())
}

// buildPriorTurnHint produces the compact 1-line hint the chitchat
// classifier consumes to disambiguate continuation references
// ("expand 10" after a list_memory listing should route back to
// chitchat, not to the pipeline). Format:
//
//	kind=<chitchat|pipeline|plan|shell> topic=<one-line, ≤100 chars>
//
// Empty string on first turn / nil store / empty Recent buffer —
// classifier MUST behave byte-identically to the no-hint shape on
// empty hint (regression-locked by test). The 200-rune cap (with
// ellipsis) prevents pathological topics from inflating classifier
// prompt budget.
func (r *REPL) buildPriorTurnHint() string {
	if r.store == nil {
		return ""
	}
	recent := r.store.Recent()
	if len(recent) == 0 {
		return ""
	}
	last := recent[len(recent)-1]
	topic := oneLine(last.Request)
	if len([]rune(topic)) > 100 {
		topic = string([]rune(topic)[:100]) + "…"
	}
	hint := fmt.Sprintf("kind=%s topic=%s", string(last.Kind), topic)
	if len([]rune(hint)) > 200 {
		hint = string([]rune(hint)[:200]) + "…"
	}
	return hint
}

// lastAnswerText returns the most recent non-empty assistant
// response from the memory store's recent-buffer slice. Used by the
// TurnPolicy dispatcher (a) to decide hasPriorAnswer and (b) to
// hand the local responder the actual prior text to transform /
// summarize. Skips memory-sanitised error placeholders so the local
// responder is never asked to "render the previous answer" when the
// previous answer was a pipeline error.
//
// Returns "" when no usable prior answer exists — first turn, nil
// store, recent buffer fully cleared, or every recent turn carried
// only the error sentinel.
func (r *REPL) lastAnswerText() string {
	if r.store == nil {
		return ""
	}
	recent := r.store.Recent()
	for i := len(recent) - 1; i >= 0; i-- {
		resp := strings.TrimSpace(recent[i].Response)
		if resp == "" {
			continue
		}
		// dispatch() persists this exact placeholder when the
		// previous pipeline turn ended with TaskState.LastError.
		// Using the text instead of a flag is intentional — error
		// pollution is the load-bearing trigger for skipping, not a
		// separate boolean we'd have to re-derive here.
		if strings.HasPrefix(resp, "(previous attempt ended in error") {
			continue
		}
		return resp
	}
	return ""
}

// localDispatch handles route=local. The local responder reuses the
// previous answer + conversation context + current message; it
// MUST NOT read the repository or invent code-level evidence
// (constraint enforced by localResponderSystemPrompt). When the
// wired ChitchatResponder satisfies LocalResponder we use it; when
// it doesn't (e.g. a test stub that only implements Respond), we
// fall back to a single Respond call with an enriched priorContext
// so the local-route guarantees still hold (no runner.Run call).
func (r *REPL) localDispatch(line, display string, policy TurnPolicy, lastAnswer string) {
	if r.chitchatResponder == nil {
		if r.renderer != nil && r.renderer.SpinnerActive() {
			r.renderer.MarkRunFatal()
			r.renderer.StopSpinner()
		}
		r.warn("local-route fallback: chit-chat responder not configured\n")
		return
	}

	ctx := r.startTurn()
	defer r.endTurn()

	prior := ""
	if r.store != nil {
		prior = r.store.BuildContext(line, memory.BuildOpts{
			Kind:      memory.KindChitchat,
			SessionID: r.sessionID,
		})
	}

	// Arm the dock shutdown override BEFORE StartSpinner so the
	// commitDockShutdownLocked call inside StopSpinner swaps the
	// default "◆ 已结束 · 0 阶段 · 总耗时 Xs" — which would render
	// misleading zeros for a route that never traverses the
	// pipeline — for "◇ 本地回复 · 未读仓库 · 纯模型生成 · 总耗时 Xs".
	// Same color family, hollow diamond signalling the lighter
	// path. Armed unconditionally: the route classification is a
	// statement about WHICH path ran, independent of whether the
	// responder errored — the error surface below is a separate
	// concern from the route badge.
	label, segs := localRouteSummary(r.language, policy)
	if r.renderer != nil {
		// Light route has no pipeline stages; SetTotalStages(0) tells
		// the dock to skip the K/N counter (renders blank instead of
		// inheriting the prior /mode's denominator). Route identity is
		// already carried by SetRouteSummary which folds into the
		// shutdown line, AND surfaces in row 2's stage-label slot via
		// composeCurrentDockRows when no focus/fallback row exists.
		r.renderer.SetTotalStages(0)
		r.renderer.SetRouteSummary(label, segs)
		r.renderer.StartSpinner()
	}
	var (
		reply string
		err   error
	)
	if local, ok := r.chitchatResponder.(LocalResponder); ok {
		reply, err = local.RespondLocal(ctx, line, prior, lastAnswer, policy.PresentationDirective)
	} else {
		// Fallback: bake the structured channels into the
		// priorContext blob a vanilla Respond consumer expects.
		// The constraint surface is weaker here (the local system
		// prompt isn't applied) but the dispatch still bypasses
		// the runner — so the user does not pay the pipeline cost
		// even when the responder is a thin Respond-only stub.
		var b strings.Builder
		if d := strings.TrimSpace(policy.PresentationDirective); d != "" {
			b.WriteString("## Presentation directive\n")
			b.WriteString(d)
			b.WriteString("\n\n")
		}
		if last := strings.TrimSpace(lastAnswer); last != "" {
			b.WriteString("## Previous answer (full text — reuse, do not invent new repo facts)\n")
			b.WriteString(last)
			b.WriteString("\n\n")
		}
		if pc := strings.TrimSpace(prior); pc != "" {
			b.WriteString("## Prior conversation\n")
			b.WriteString(pc)
		}
		reply, err = r.chitchatResponder.Respond(ctx, line, b.String())
	}
	if r.renderer != nil {
		armDockTerminalState(r.renderer, err)
		r.renderer.StopSpinner()
	}

	if err != nil {
		logging.Warning("[repl/turn_policy] local responder failed: %v", err)
		r.errorf("local: %s\n", friendlyRunError(r.language, err))
		return
	}
	if strings.TrimSpace(reply) == "" {
		fmt.Fprintln(r.out, emptyResponseHint(r.language))
		return
	}

	logging.Info("[repl/turn_policy] local reply (len=%d):\n%s", len(reply), reply)

	// Renderer-nil fallback: tests / scripted mode without a TTY
	// renderer never run StartSpinner / StopSpinner, so the dock
	// shutdown override never fires. Emit the same line directly
	// using the shared formatter so the route badge still shows.
	if r.renderer == nil {
		fmt.Fprintln(r.out, render.FormatLightRouteSummary(label, segs, "", r.language))
	}
	// Run mermaid → ASCII + glamour markdown styling before
	// rendering. The pipeline path gets this via RenderResult; the
	// local path used to bypass it, so the user saw raw ```mermaid```
	// blocks and literal ## / **bold** / | table | source in the
	// REPL output. renderRichResponse is the single source of
	// rendering for both off-pipeline answer surfaces.
	r.renderBordered(r.renderRichResponse(reply))
	if result := r.writeLocalMarkdownTranscript(line, reply); result.MarkdownPath != "" {
		r.emitMarkdownTranscriptHints(result)
	}
	r.lastAnswerOrigin = replAnswerOriginLocal
	// Persist as KindPipeline. A local-route turn is structurally a
	// derivative of a previous pipeline answer (transform /
	// summarize / translate / elaborate of last_answer), so its
	// retrieval semantics match pipeline-side retention: full body
	// preservation in the recent buffer + entity-weighted scoring
	// in compacted recall. Persisting as KindChitchat (the pre-fix
	// shape) capped recent_body_chars at 800 — when the local
	// reply was a multi-row markdown table or a sizeable diagram,
	// the next turn's lastAnswerText would only see the prefix and
	// any "transform of THIS transform" turn would read truncated
	// context. KindPipeline is the existing channel that already
	// preserves long-form derived content; reusing it keeps the
	// memory schema unchanged (no new Kind constant needed).
	r.recordTurn(display, line, reply, memory.KindPipeline)
}

// clarifyDispatch handles route=clarify. No LLM call, no pipeline,
// no memory write — the user's request was structurally
// unanswerable in this session (e.g. "换成 mermaid" with no prior
// answer) and asking again with a fresh framing is the correct
// recovery path. Persisting a stub turn would just inflate
// BuildContext on the next dispatch with no useful content.
func (r *REPL) clarifyDispatch(line, display string, policy TurnPolicy) {
	logging.Info("[repl/turn_policy] clarify route — line=%q reason=%q",
		oneLine(line), oneLineClamp(policy.Reason, 120))
	_ = display // intentional: we don't echo or persist
	label, segs := clarifyRouteSummary(r.language)
	if r.renderer != nil {
		if r.renderer.SpinnerActive() {
			r.renderer.SetTotalStages(0)
			r.renderer.SetRouteSummary(label, segs)
			r.renderer.StopSpinner()
			r.lastAnswerOrigin = replAnswerOriginLocal
			return
		}
		r.renderer.EmitLightRouteSummary(label, segs)
		r.lastAnswerOrigin = replAnswerOriginLocal
		return
	}
	fmt.Fprintln(r.out, render.FormatLightRouteSummary(label, segs, "", r.language))
	r.lastAnswerOrigin = replAnswerOriginLocal
}

// operationUnavailableDispatch handles route=operation before the independent
// operation pipeline exists. The important invariant is negative: this route
// must not fall through into source analysis or execute any side-effecting
// tool just because the classifier learned the new enum earlier than the
// executor was installed.
func (r *REPL) operationUnavailableDispatch(line, display string, policy TurnPolicy) {
	logging.Info("[repl/turn_policy] operation route unavailable — line=%q operation=%q kind=%q risk=%q side_effects=%q reason=%q",
		oneLine(line),
		oneLineClamp(policy.Operation, 80),
		oneLineClamp(policy.OperationKind, 80),
		oneLineClamp(policy.RiskLevel, 40),
		oneLineClamp(strings.Join(policy.SideEffects, ","), 120),
		oneLineClamp(policy.Reason, 120))
	r.finishOperationRouteSpinner(operation.StatusBlocked)
	msg := operationUnavailableMsg(r.language, policy)
	if g := operationWritePipelineGuidance(r.language, policy); g != "" {
		msg += "\n\n" + g
	}
	r.warn("%s\n", msg)
	r.lastAnswerOrigin = replAnswerOriginLocal
	r.recordTurn(display, line, msg, memory.KindPipeline)
}

func (r *REPL) dataTaskDispatch(line, display string, policy TurnPolicy) {
	if r.dataTaskPlanner == nil {
		msg := dataTaskUnavailableMarkdown(r.language)
		r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "blocked", Reason: "data task planner is not configured"})
		r.finishDataTaskRouteSpinner("blocked")
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginLocal
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return
	}
	r.startDataTaskPlanningSpinner()
	ctx := r.startTurn()
	candidates, err := dataquery.DiscoverCandidateFiles(r.repoRoot, 240)
	if err != nil {
		r.endTurn()
		reason := fmt.Sprintf("discover data files: %v", err)
		msg := dataTaskErrorMarkdown(r.language, reason)
		r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason})
		r.finishDataTaskRouteSpinner("failed")
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginLocal
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return
	}
	plan, err := r.dataTaskPlanner.PlanDataTask(ctx, line, r.repoRoot, policy, candidates)
	r.endTurn()
	r.emitReplLLMTrace(r.dataTaskPlanner, "data_task_planner", types.AgentName("data_planner"), types.PipelineStage("data"))
	if err != nil {
		reason := fmt.Sprintf("plan data task: %v", err)
		msg := dataTaskErrorMarkdown(r.language, reason)
		r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason})
		r.finishDataTaskRouteSpinner("failed")
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginLocal
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return
	}
	if normalized, notes := normalizeDataTaskPlanShapeForPolicy(plan, policy); len(notes) > 0 {
		logging.Info("[repl/data] normalized initial data task plan: %s", strings.Join(notes, "; "))
		plan = normalized
	}
	var records []dataTaskWorkflowRecord
	workflowRuntime := dataworkflow.NewWorkflowRuntime(dataquery.TaskPlan{})
	r.activeDataWorkflowRuntime = workflowRuntime
	defer func() {
		if r.activeDataWorkflowRuntime == workflowRuntime {
			r.activeDataWorkflowRuntime = nil
		}
	}()
	appendRecord := func(record dataTaskWorkflowRecord) {
		records = workflowRuntime.AppendRecord(record)
	}
	recordsWith := func(record dataTaskWorkflowRecord) []dataTaskWorkflowRecord {
		return workflowRuntime.RecordsWith(record)
	}
	attachLastEvaluation := func(eval dataquery.Evaluation) {
		records = workflowRuntime.AttachLastEvaluation(eval)
	}
	attachLastError := func(errText string) {
		records = workflowRuntime.AttachLastError(errText)
	}
	emitWorkflowEvent := func(event dataworkflow.WorkflowJournalEvent, opts dataTaskWorkflowEventRenderOptions) {
		workflowRuntime.AppendProcessEvent(event)
		r.emitDataTaskWorkflowEvent(event, opts)
	}
	emitWorkflowReason := func(kind string, round int, reason string) {
		event := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
			Kind:   strings.TrimSpace(kind),
			Round:  round,
			Reason: reason,
		})
		if event.Kind == "continue" {
			return
		}
		emitWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{})
	}
	emitWorkflowFailure := func(kind string, round int, reason string) {
		emitWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
			Kind:   strings.TrimSpace(kind),
			Round:  round,
			Status: "failed",
			Reason: reason,
		}), dataTaskWorkflowEventRenderOptions{IncludeFailure: true})
	}
	emitWorkflowGuard := func(kind string, round int, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) {
		event := workflowRuntime.AppendGuardEvent(kind, round, plan, guard)
		if event.Guard == nil {
			return
		}
		r.emitDataTaskWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true, IncludeFailure: true, IncludeAudit: true})
	}
	protectPlan := func(p dataquery.TaskPlan) dataquery.TaskPlan {
		return prepareDataTaskWorkflowPlanForExecution(line, candidates, records, p)
	}
	discardDeferredPlan := func(round int, reason string) {
		deferredPlan := workflowRuntime.DeferredPlan()
		if len(deferredPlan.Actions) > 0 {
			status := dataTaskDeferredQueueStatus(records, deferredPlan)
			workflowRuntime.DiscardDeferred(round, status, reason)
			detail := dataTaskDeferredQueueDiscardedSegment(r.language, deferredPlan, reason)
			emitWorkflowReason("deferred", round, detail)
			first := deferredPlan.Actions[0]
			logging.Info("[repl/data] deferred data action queue discarded actions=%d first_action=%s:%s reason=%q", len(deferredPlan.Actions), first.ID, first.Kind, reason)
			return
		}
		workflowRuntime.ClearDeferred(round, reason)
	}
	currentDeferredPlan := func() dataquery.TaskPlan {
		return workflowRuntime.DeferredPlan()
	}
	renderAdmissionEvents := func(events []dataworkflow.WorkflowJournalEvent) {
		for _, event := range events {
			if event.Kind != "plan_transition" {
				continue
			}
			r.emitDataTaskWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true, IncludeAudit: true})
		}
	}
	acceptCandidatePlan := func(scope string, round int, candidate dataquery.TaskPlan) dataquery.TaskPlan {
		admission := workflowRuntime.AdmitCandidatePlanAndQueueRemainder(round, scope, dataTaskPreflightWorkflowPlanInput(records, candidate, protectPlan))
		records = workflowRuntime.Records()
		preflight := admission.Admission
		if preflight.Rewritten {
			emitWorkflowReason("continue", round, preflight.Reason)
		}
		if strings.TrimSpace(preflight.FinalGuardErr) != "" {
			r.auditDataTaskPlan("rejected", round, preflight.Plan)
			logging.Info("[repl/data] data task candidate plan rejected scope=%s round=%d reason=%q", admission.Source, round, preflight.FinalGuardErr)
			return preflight.Plan
		}
		if admission.DeferredQueued {
			r.auditDataTaskPlan("deferred", round, admission.DeferredPlan)
			emitWorkflowReason("deferred", round, dataTaskDeferredQueueSavedSegment(r.language, admission.DeferredPlan, admission.DeferredReason))
		}
		renderAdmissionEvents(admission.ProcessEvents)
		r.emitDataTaskPlanAudit(preflight.Plan)
		r.auditDataTaskPlan(admission.Source, round, preflight.Plan)
		return admission.Plan
	}
	plan = acceptCandidatePlan("initial", 0, plan)
	if handled := r.handleTerminalDataTaskPlan(plan, nil, display, line, 0, 0); handled {
		return
	}
	runner := dataquery.Runner{
		RepoRoot: r.repoRoot,
		TempRoot: filepath.Join(firstNonEmptyString(r.runtimeAnchor, r.repoRoot), "data"),
	}
	actionRunner := dataquery.ActionRunner{
		RepoRoot: r.repoRoot,
		TempRoot: filepath.Join(firstNonEmptyString(r.runtimeAnchor, r.repoRoot), "data"),
	}
	currentPlan := plan
	setCurrentPlan := func(source string, round int, next dataquery.TaskPlan, reason string) dataquery.TaskPlan {
		switchDecision := workflowRuntime.SwitchCurrentPlanWithEvents(round, source, next, reason)
		for _, event := range switchDecision.ProcessEvents {
			if event.Kind != "plan_transition" {
				continue
			}
			r.emitDataTaskWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true, IncludeAudit: true})
		}
		return switchDecision.Plan
	}
	repairRounds := 0
	dataRounds := 0
	workflowRuntime.SetRounds(dataRounds, repairRounds)
	runtimeView := func() dataTaskWorkflowRuntimeView {
		view := dataTaskWorkflowRuntimeViewFrom(workflowRuntime, records, currentPlan, currentDeferredPlan(), dataRounds, repairRounds)
		records = view.Records
		currentPlan = view.CurrentPlan
		dataRounds = view.DataRounds
		repairRounds = view.RepairRounds
		return view
	}
	syncIteration := func(iteration dataworkflow.WorkflowIterationDecision) {
		records = iteration.Records
		if dataTaskPlanHasRuntimeShape(iteration.CurrentPlan) {
			currentPlan = iteration.CurrentPlan
		}
		dataRounds = iteration.DataRounds
		repairRounds = iteration.RepairRounds
	}
	beginDataIteration := func() int {
		syncIteration(workflowRuntime.BeginDataIteration())
		return dataRounds
	}
	beginRepairIteration := func() int {
		syncIteration(workflowRuntime.BeginRepairIteration())
		return repairRounds
	}
	applyRepairResult := func(result dataTaskRepairPlanResult) {
		if strings.TrimSpace(result.FallbackReason) != "" {
			emitWorkflowReason("continue", dataRounds, result.FallbackReason)
			r.emitDataTaskPlanAudit(result.Plan)
			r.auditDataTaskPlan("continue", dataRounds+1, result.Plan)
			currentPlan = setCurrentPlan("continue", dataRounds+1, result.Plan, result.FallbackReason)
			return
		}
		currentPlan = acceptCandidatePlan("repair", repairRounds, result.Plan)
	}
	for {
		if guard := dataTaskTerminalPlanCompletionGateGuardResultWithRepo(r.repoRoot, records, currentPlan); !guard.Empty() {
			errText := guard.ErrorText()
			emitWorkflowGuard("completion_gate", dataRounds, currentPlan, guard)
			transition := dataworkflow.ValidationFailureTransition{
				Action:    dataworkflow.ValidationFailureNeedsRepair,
				Reason:    errText,
				ErrorText: errText,
				Guard:     guard,
			}
			if result, ok := latestDataTaskResult(records); ok {
				transition = dataTaskValidationFailureTransitionWithRepo(r.repoRoot, records, currentPlan, result, guard)
			}
			_, repairReady := r.dataTaskPlanner.(DataTaskRepairPlanner)
			recovery := dataworkflow.DecideValidationFailureRecovery(transition, repairReady, repairRounds < r.dataTaskMaxRepairRounds, errText)
			if recovery.Action == dataworkflow.FailureRecoveryFallbackPlan && recovery.HasPlan() {
				runtimeRecovery := workflowRuntime.ApplyFailureRecoveryDecision(dataRounds+1, "ledger", recovery, protectPlan)
				completionPlan := runtimeRecovery.Plan
				r.emitDataTaskPlanAudit(completionPlan)
				r.auditDataTaskPlan("ledger", dataRounds+1, completionPlan)
				renderAdmissionEvents(runtimeRecovery.Switch.ProcessEvents)
				currentPlan = completionPlan
				continue
			}
			if recovery.Action == dataworkflow.FailureRecoveryFail {
				msg := dataTaskErrorMarkdown(r.language, recovery.Reason)
				r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: recovery.Reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
				r.finishDataTaskRouteSpinner("failed")
				r.renderBordered(msg)
				r.lastAnswerOrigin = replAnswerOriginLocal
				r.recordTurn(display, line, msg, memory.KindPipeline)
				return
			}
			if repairReady {
				repairRounds = beginRepairIteration()
				repairView := runtimeView()
				repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, errText, "terminal-completion repaired")
				if repairErr != nil {
					reason := fmt.Sprintf("%s\nrepair data task completion: %v", errText, repairErr)
					msg := dataTaskErrorMarkdown(r.language, reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				applyRepairResult(repairResult)
				continue
			}
			msg := dataTaskErrorMarkdown(r.language, errText)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: errText, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
			r.finishDataTaskRouteSpinner("failed")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		}
		preRunDecision, preRunResult, preRunHasResult := dataTaskWorkflowPreRunDecisionWithRepo(r.repoRoot, records, currentPlan, dataRounds, r.dataTaskMaxDataRounds)
		switch preRunDecision.Action {
		case dataworkflow.WorkflowPreRunTerminalWorkflowFallback:
			reason := preRunDecision.Reason
			appendRecord(dataTaskWorkflowRecord{Plan: currentPlan, Err: reason})
			emitWorkflowReason("continue", dataRounds, reason)
			fallback := protectPlan(preRunDecision.Plan)
			r.emitDataTaskPlanAudit(fallback)
			r.auditDataTaskPlan("continue", dataRounds+1, fallback)
			currentPlan = setCurrentPlan("continue", dataRounds+1, fallback, reason)
			continue
		case dataworkflow.WorkflowPreRunTerminalWorkflowGuard:
			guard := preRunDecision.Guard
			errText := guard.ErrorText()
			guardRecord := dataTaskWorkflowRecordForGuard(currentPlan, guard)
			guardRecords := recordsWith(guardRecord)
			writeDataTaskWorkflowCheckpointFileWithDeferredQueue(r.runtimeAnchor, r.repoRoot, workflowRuntime, guardRecords, currentPlan, workflowRuntime.DeferredQueue(), dataRounds, repairRounds, "terminal workflow guard blocked current plan", "repl", guard)
			if _, ok := r.dataTaskPlanner.(DataTaskRepairPlanner); ok && repairRounds < r.dataTaskMaxRepairRounds {
				repairRounds = beginRepairIteration()
				appendRecord(guardRecord)
				emitWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
					Kind:         "repair",
					Round:        repairRounds,
					Plan:         currentPlan,
					AuditDetails: []string{guard.Code},
					Guard:        &guard,
				}), dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true, IncludeFailure: true, IncludeAudit: true})
				repairView := runtimeView()
				repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, errText, "terminal-workflow repaired")
				if repairErr != nil {
					reason := fmt.Sprintf("%s\nrepair data task terminal workflow: %v", errText, repairErr)
					msg := dataTaskErrorMarkdown(r.language, reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				applyRepairResult(repairResult)
				continue
			}
			msg := dataTaskErrorMarkdown(r.language, errText)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: errText, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
			r.finishDataTaskRouteSpinner("failed")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		case dataworkflow.WorkflowPreRunTerminalPlan:
			if handled := r.handleTerminalDataTaskPlan(currentPlan, records, display, line, dataRounds, repairRounds); handled {
				return
			}
		case dataworkflow.WorkflowPreRunBudgetFail:
			if !preRunDecision.Guard.Empty() {
				emitWorkflowGuard("completion_gate", dataRounds, currentPlan, preRunDecision.Guard)
			}
			reason := preRunDecision.Reason
			msg := dataTaskErrorMarkdown(r.language, reason)
			var resultPtr *dataquery.Result
			if preRunHasResult {
				resultCopy := preRunResult
				resultPtr = &resultCopy
			}
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "budget_exhausted", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: resultPtr})
			r.finishDataTaskRouteSpinner("budget_exhausted")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		case dataworkflow.WorkflowPreRunBudgetReturnResult:
			if preRunHasResult {
				r.finishDataTaskWithFinalResult("budget_exhausted", preRunDecision.Reason, records, currentPlan, preRunResult, display, line, dataRounds, repairRounds)
				return
			}
		case dataworkflow.WorkflowPreRunPreExecutionFallback:
			reason := preRunDecision.Reason
			appendRecord(dataTaskWorkflowRecord{Plan: currentPlan, Err: reason})
			emitWorkflowReason("continue", dataRounds, reason)
			fallback := protectPlan(preRunDecision.Plan)
			r.emitDataTaskPlanAudit(fallback)
			r.auditDataTaskPlan("continue", dataRounds+1, fallback)
			currentPlan = setCurrentPlan("continue", dataRounds+1, fallback, reason)
			continue
		case dataworkflow.WorkflowPreRunPreExecutionGuard:
			guard := preRunDecision.Guard
			errText := guard.ErrorText()
			guardRecord := dataTaskWorkflowRecordForGuard(currentPlan, guard)
			guardRecords := recordsWith(guardRecord)
			writeDataTaskWorkflowCheckpointFileWithDeferredQueue(r.runtimeAnchor, r.repoRoot, workflowRuntime, guardRecords, currentPlan, workflowRuntime.DeferredQueue(), dataRounds, repairRounds, "staging guard blocked current batch", "repl", guard)
			switch guardRecovery := dataTaskStagingGuardRecoveryDecision(records, currentPlan, guard); guardRecovery.Action {
			case dataworkflow.GuardRecoveryFallbackPlan:
				appendRecord(guardRecord)
				guardRuntime := workflowRuntime.ApplyGuardRecoveryDecision(dataRounds+1, guardRecovery, protectPlan)
				reason := guardRuntime.Reason
				emitWorkflowReason("continue", dataRounds, reason)
				fallback := guardRuntime.Plan
				r.emitDataTaskPlanAudit(fallback)
				r.auditDataTaskPlan("continue", dataRounds+1, fallback)
				renderAdmissionEvents(guardRuntime.Switch.ProcessEvents)
				currentPlan = fallback
				if guardRuntime.DeferredQueued {
					r.auditDataTaskPlan("deferred", dataRounds+1, guardRuntime.Deferred.QueuedPlan)
					emitWorkflowReason("deferred", dataRounds+1, dataTaskDeferredQueueSavedSegment(r.language, guardRuntime.Deferred.QueuedPlan, reason))
				}
				continue
			}
			appendRecord(guardRecord)
			if _, ok := r.dataTaskPlanner.(DataTaskRepairPlanner); ok && repairRounds < r.dataTaskMaxRepairRounds {
				repairRounds = beginRepairIteration()
				emitWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
					Kind:         "repair",
					Round:        repairRounds,
					Plan:         currentPlan,
					AuditDetails: []string{guard.Code},
					Guard:        &guard,
				}), dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true, IncludeFailure: true, IncludeAudit: true})
				repairView := runtimeView()
				repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, errText, "repaired")
				if repairErr != nil {
					reason := fmt.Sprintf("%s\nrepair data task: %v", errText, repairErr)
					msg := dataTaskErrorMarkdown(r.language, reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				applyRepairResult(repairResult)
				continue
			}
			msg := dataTaskErrorMarkdown(r.language, errText)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: errText, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
			r.finishDataTaskRouteSpinner("failed")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		}
		if errText := r.prepareDataTaskNonTextMaterials(context.Background(), &candidates, currentPlan); errText != "" {
			appendRecord(dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
			if _, ok := r.dataTaskPlanner.(DataTaskRepairPlanner); ok && repairRounds < r.dataTaskMaxRepairRounds {
				repairRounds = beginRepairIteration()
				emitWorkflowFailure("repair", repairRounds, errText)
				repairView := runtimeView()
				repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, errText, "repaired")
				if repairErr != nil {
					reason := fmt.Sprintf("%s\nrepair data task: %v", errText, repairErr)
					msg := dataTaskErrorMarkdown(r.language, reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				applyRepairResult(repairResult)
				continue
			}
			msg := dataTaskErrorMarkdown(r.language, errText)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: errText, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
			r.finishDataTaskRouteSpinner("failed")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		}
		dataRounds = beginDataIteration()
		r.emitDataTaskRunnerCall(currentPlan, dataRounds)
		emitWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
			Kind:  "execute",
			Round: dataRounds,
			Plan:  currentPlan,
		}), dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true})
		var result dataquery.Result
		if len(currentPlan.Actions) > 0 {
			seededActionRunner := actionRunner
			seededActionRunner.Seed = dataTaskActionRunnerSeed(records)
			result, err = seededActionRunner.Run(context.Background(), currentPlan)
		} else {
			result, err = runner.Run(context.Background(), currentPlan)
		}
		if err != nil {
			if _, ok := r.dataTaskPlanner.(DataTaskResultPatchPlanner); ok {
				var validationErr *dataquery.DataResultValidationError
				if errors.As(err, &validationErr) && dataTaskPatchCandidate(validationErr.Result, validationErr.Violations) {
					ctx := r.startTurn()
					patchView := runtimeView()
					patched, patchedOK, attempted, reason := tryPatchDataTaskResultWithRuntimeView(ctx, r.dataTaskPlanner, line, patchView.CurrentPlan, err, patchView, r.language)
					r.endTurn()
					if attempted {
						r.emitReplLLMTrace(r.dataTaskPlanner, "data_result_patch_planner", types.AgentName("data_planner"), types.PipelineStage("data"))
					}
					if patchedOK {
						result = patched
						emitWorkflowReason("patch", dataRounds, reason)
						err = nil
					}
				}
			}
		}
		if err != nil {
			errText := fmt.Sprintf("execute data task: %v", err)
			violation := dataquery.ClassifyExecutionFailure(err)
			discardDeferredPlan(dataRounds, fmt.Sprintf("execution failure: %s", clampDataTaskWorkflowText(errText, 240)))
			r.auditDataTaskError(dataRounds, errText)
			executionRecord := dataTaskWorkflowRecordWithExecutionViolation(currentPlan, result, errText, violation)
			transition := dataTaskExecutionFailureTransition(records, currentPlan, result, errText, violation)
			_, continuationReady := r.dataTaskPlanner.(DataTaskContinuationPlanner)
			_, repairReady := r.dataTaskPlanner.(DataTaskRepairPlanner)
			recovery := dataworkflow.DecideExecutionFailureRecovery(transition, continuationReady, repairReady, repairRounds < r.dataTaskMaxRepairRounds, errText)
			recordedErr := false
			if recovery.Action == dataworkflow.FailureRecoveryFallbackPlan && recovery.HasPlan() {
				appendRecord(executionRecord)
				recordedErr = true
				emitWorkflowReason("continue", dataRounds, recovery.Reason)
				fallback := recovery.Plan
				fallback = protectPlan(fallback)
				r.emitDataTaskPlanAudit(fallback)
				r.auditDataTaskPlan("continue", dataRounds+1, fallback)
				currentPlan = setCurrentPlan("continue", dataRounds+1, fallback, recovery.Reason)
				continue
			}
			if recovery.Action == dataworkflow.FailureRecoveryContinuation {
				if continuer, ok := r.dataTaskPlanner.(DataTaskContinuationPlanner); ok {
					appendRecord(executionRecord)
					recordedErr = true
					emitWorkflowReason("continue", dataRounds, recovery.Reason)
					ctx := r.startTurn()
					view := runtimeView()
					continuationResult, contErr := dataTaskRunContinuationPlannerWithRuntimeView(ctx, continuer, line, r.repoRoot, policy, candidates, view, errText)
					r.endTurn()
					r.emitReplLLMTrace(r.dataTaskPlanner, "data_task_continuation_planner", types.AgentName("data_planner"), types.PipelineStage("data"))
					if contErr == nil {
						if strings.TrimSpace(continuationResult.FallbackReason) != "" {
							emitWorkflowReason("continue", dataRounds, continuationResult.FallbackReason)
							fallback := protectPlan(continuationResult.Plan)
							r.emitDataTaskPlanAudit(fallback)
							r.auditDataTaskPlan("continue", dataRounds+1, fallback)
							currentPlan = setCurrentPlan("continue", dataRounds+1, fallback, continuationResult.FallbackReason)
							continue
						}
						nextPlan := continuationResult.Plan
						if normalized, notes := normalizeDataTaskPlanShapeForPolicy(nextPlan, policy); len(notes) > 0 {
							logging.Info("[repl/data] normalized continuation data task plan: %s", strings.Join(notes, "; "))
							nextPlan = normalized
						}
						currentPlan = acceptCandidatePlan("continue", dataRounds+1, nextPlan)
						continue
					}
				}
				recovery = dataworkflow.DecideExecutionFailureRecovery(dataworkflow.ExecutionFailureTransition{
					Action: dataworkflow.ExecutionFailureNeedsRepair,
					Reason: errText,
				}, false, repairReady, repairRounds < r.dataTaskMaxRepairRounds, errText)
			}
			if recovery.Action == dataworkflow.FailureRecoveryFail {
				if !recordedErr {
					appendRecord(executionRecord)
				}
				msg := dataTaskErrorMarkdown(r.language, recovery.Reason)
				r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: recovery.Reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
				r.finishDataTaskRouteSpinner("failed")
				r.renderBordered(msg)
				r.lastAnswerOrigin = replAnswerOriginLocal
				r.recordTurn(display, line, msg, memory.KindPipeline)
				return
			}
			if !recordedErr {
				appendRecord(executionRecord)
			}
			if _, ok := r.dataTaskPlanner.(DataTaskRepairPlanner); ok && repairRounds < r.dataTaskMaxRepairRounds {
				repairRounds = beginRepairIteration()
				emitWorkflowFailure("repair", repairRounds, errText)
				repairView := runtimeView()
				repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, errText, "repaired")
				if repairErr != nil {
					reason := fmt.Sprintf("%s\nrepair data task: %v", errText, repairErr)
					msg := dataTaskErrorMarkdown(r.language, reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				applyRepairResult(repairResult)
				continue
			}
			msg := dataTaskErrorMarkdown(r.language, errText)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: errText, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
			r.finishDataTaskRouteSpinner("failed")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		}
		if shouldValidateDataTaskWorkflowResult(currentPlan) {
			if gateErr := validateDataTaskWorkflowResult(records, currentPlan, result); gateErr != nil {
				errText := fmt.Sprintf("validate data workflow result: %v", gateErr)
				r.auditDataTaskError(dataRounds, errText)
				transition := dataworkflow.ValidationFailureTransition{
					Action:    dataworkflow.ValidationFailureNeedsRepair,
					Reason:    errText,
					ErrorText: errText,
				}
				guard := dataTaskWorkflowCompletionGateGuardResultWithRepo(r.repoRoot, records, currentPlan, result)
				if !guard.Empty() {
					transition = dataTaskValidationFailureTransitionWithRepo(r.repoRoot, records, currentPlan, result, guard)
				}
				_, repairReady := r.dataTaskPlanner.(DataTaskRepairPlanner)
				recovery := dataworkflow.DecideValidationFailureRecovery(transition, repairReady, repairRounds < r.dataTaskMaxRepairRounds, errText)
				if recovery.Action == dataworkflow.FailureRecoveryFallbackPlan && recovery.HasPlan() {
					appendRecord(dataTaskWorkflowRecordWithOptionalResult(currentPlan, result, errText))
					runtimeRecovery := workflowRuntime.ApplyFailureRecoveryDecision(dataRounds+1, "ledger", recovery, protectPlan)
					completionPlan := runtimeRecovery.Plan
					r.emitDataTaskPlanAudit(completionPlan)
					r.auditDataTaskPlan("ledger", dataRounds+1, completionPlan)
					renderAdmissionEvents(runtimeRecovery.Switch.ProcessEvents)
					currentPlan = completionPlan
					continue
				}
				if recovery.Action == dataworkflow.FailureRecoveryFail {
					appendRecord(dataTaskWorkflowRecordWithOptionalResult(currentPlan, result, errText))
					msg := dataTaskErrorMarkdown(r.language, recovery.Reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: recovery.Reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				appendRecord(dataTaskWorkflowRecordWithOptionalResult(currentPlan, result, errText))
				if _, ok := r.dataTaskPlanner.(DataTaskRepairPlanner); ok && repairRounds < r.dataTaskMaxRepairRounds {
					repairRounds = beginRepairIteration()
					emitWorkflowFailure("repair", repairRounds, errText)
					repairView := runtimeView()
					repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, errText, "repaired")
					if repairErr != nil {
						reason := fmt.Sprintf("%s\nrepair data task: %v", errText, repairErr)
						msg := dataTaskErrorMarkdown(r.language, reason)
						r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
						r.finishDataTaskRouteSpinner("failed")
						r.renderBordered(msg)
						r.lastAnswerOrigin = replAnswerOriginLocal
						r.recordTurn(display, line, msg, memory.KindPipeline)
						return
					}
					applyRepairResult(repairResult)
					continue
				}
				msg := dataTaskErrorMarkdown(r.language, errText)
				r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: errText, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
				r.finishDataTaskRouteSpinner("failed")
				r.renderBordered(msg)
				r.lastAnswerOrigin = replAnswerOriginLocal
				r.recordTurn(display, line, msg, memory.KindPipeline)
				return
			}
		}
		appendRecord(dataTaskWorkflowRecord{Plan: currentPlan, Result: &result, Admission: dataTaskAdmissionDecisionForPlan(workflowRuntime.Admission(), currentPlan)})
		r.auditDataTaskResult(dataRounds, result)
		writeDataTaskWorkflowCheckpointFileWithDeferredQueue(r.runtimeAnchor, r.repoRoot, workflowRuntime, records, currentPlan, workflowRuntime.DeferredQueue(), dataRounds, repairRounds, "batch result completed", "repl")
		resultForEvent := result
		emitWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
			Kind:   "result",
			Round:  dataRounds,
			Plan:   currentPlan,
			Result: &resultForEvent,
		}), dataTaskWorkflowEventRenderOptions{IncludeAudit: true})
		switch postResultDecision := dataTaskPostResultDecisionWithRepo(r.repoRoot, records, currentPlan, workflowRuntime.DeferredPlan()); postResultDecision.Action {
		case dataworkflow.PostResultDispatchDeferred:
			emitWorkflowReason("continue", dataRounds, postResultDecision.Reason)
			nextDeferred := protectPlan(postResultDecision.Plan)
			r.emitDataTaskPlanAudit(nextDeferred)
			r.auditDataTaskPlan("continue", dataRounds+1, nextDeferred)
			currentPlan = setCurrentPlan("deferred_dispatch", dataRounds+1, nextDeferred, postResultDecision.Reason)
			runtimePostResultDecision := postResultDecision
			runtimePostResultDecision.Plan = nextDeferred
			advance := workflowRuntime.ApplyPostResultDecision(dataRounds, runtimePostResultDecision).Advance
			remainingDeferred := dataworkflow.DeferredQueuePlan(advance.Queue)
			if len(remainingDeferred.Actions) > 0 {
				r.auditDataTaskPlan("deferred", dataRounds+1, remainingDeferred)
				emitWorkflowReason("deferred", dataRounds+1, dataTaskDeferredQueueSavedSegment(r.language, remainingDeferred, "remaining deferred typed data action rank(s)"))
			}
			continue
		case dataworkflow.PostResultUpdateDeferred:
			deferredPlan := postResultDecision.DeferredPlan
			status := postResultDecision.DeferredStatus
			advance := workflowRuntime.ApplyPostResultDecision(dataRounds, postResultDecision).Advance
			switch advance.Action {
			case dataworkflow.DeferredQueueTransitionRetain:
				emitWorkflowReason("deferred", dataRounds, dataTaskDeferredQueueRetainedSegment(r.language, status))
				logging.Info("[repl/data] deferred data action queue retained actions=%d first_action=%s:%s reason_code=%q reason=%q",
					len(deferredPlan.Actions), status.FirstActionID, status.FirstActionKind, advance.Lifecycle.ReasonCode, oneLineClamp(advance.Lifecycle.Reason, 500))
			case dataworkflow.DeferredQueueTransitionDiscard:
				if strings.TrimSpace(status.Reason) != "" {
					appendRecord(dataTaskWorkflowRecord{Plan: deferredPlan, Err: status.Reason})
				}
				emitWorkflowReason("deferred", dataRounds, dataTaskDeferredQueueDiscardedSegment(r.language, deferredPlan, advance.Reason))
				first := deferredPlan.Actions[0]
				logging.Info("[repl/data] deferred data action queue discarded actions=%d first_action=%s:%s reason=%q", len(deferredPlan.Actions), first.ID, first.Kind, advance.Reason)
			}
		case dataworkflow.PostResultFallbackPlan:
			reason := postResultDecision.Reason
			if postResultDecision.Source == "coverage" {
				appendRecord(dataTaskWorkflowRecord{Plan: currentPlan, Err: reason})
			}
			emitWorkflowReason("continue", dataRounds, reason)
			fallback := protectPlan(postResultDecision.Plan)
			r.emitDataTaskPlanAudit(fallback)
			r.auditDataTaskPlan("continue", dataRounds+1, fallback)
			currentPlan = setCurrentPlan("continue", dataRounds+1, fallback, reason)
			continue
		default:
			workflowRuntime.ApplyPostResultDecision(dataRounds, postResultDecision)
		}
		evaluator, evalOK := r.dataTaskPlanner.(DataTaskEvaluator)
		continuer, contOK := r.dataTaskPlanner.(DataTaskContinuationPlanner)
		if !evalOK {
			if !currentPlan.ContinueAfter {
				r.finishDataTaskWithFinalResult("complete", "data task result completed without evaluator", records, currentPlan, result, display, line, dataRounds, repairRounds)
				return
			}
			r.finishDataTaskWithFinalResult("partial_answer_possible", "data task produced a result but requested continuation and evaluator is unavailable", records, currentPlan, result, display, line, dataRounds, repairRounds)
			return
		}
		view := runtimeView()
		stateForEvent := dataTaskWorkflowStateFromRuntimeView(view)
		emitWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
			Kind:     "evaluate",
			Round:    view.DataRounds,
			Plan:     view.CurrentPlan,
			Decision: stateForEvent.Decision,
		}), dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true, IncludeAudit: true})
		ctx := r.startTurn()
		eval, evalErr := evaluateDataTaskWithRuntimeViewIfSupported(ctx, evaluator, line, view, r.language)
		r.endTurn()
		r.emitReplLLMTrace(r.dataTaskPlanner, "data_task_evaluator", types.AgentName("data_planner"), types.PipelineStage("data"))
		if evalErr != nil {
			reason := fmt.Sprintf("evaluate data task: %v", evalErr)
			msg := dataTaskErrorMarkdown(r.language, reason)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
			r.finishDataTaskRouteSpinner("failed")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		}
		if len(records) > 0 {
			attachLastEvaluation(eval)
		}
		_, repairOK := r.dataTaskPlanner.(DataTaskRepairPlanner)
		evalDecision := dataTaskEvaluationDecisionWithRepo(r.repoRoot, records, currentPlan, result, eval, contOK, repairOK, repairRounds, r.dataTaskMaxRepairRounds)
		switch evalDecision.Action {
		case dataworkflow.EvaluationDecisionReturnAnswer:
			r.finishDataTaskWithFinalResult(evalDecision.Status, evalDecision.Reason, records, currentPlan, result, display, line, dataRounds, repairRounds)
			return
		case dataworkflow.EvaluationDecisionReturnEvaluation:
			msg := dataTaskEvaluationMarkdown(r.language, eval)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: evalDecision.Status, Reason: evalDecision.Reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
			r.finishDataTaskRouteSpinner(evalDecision.Status)
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		case dataworkflow.EvaluationDecisionFallbackPlan:
			if !evalDecision.Guard.Empty() {
				emitWorkflowGuard("completion_gate", dataRounds, currentPlan, evalDecision.Guard)
				r.auditDataTaskError(dataRounds, evalDecision.Reason)
				if len(records) > 0 {
					attachLastError(evalDecision.Reason)
				}
			}
			scope := "continue"
			if evalDecision.Source == "ledger" || evalDecision.Source == "completion_gate" {
				scope = "ledger"
			}
			fallback := protectPlan(evalDecision.Plan)
			r.emitDataTaskPlanAudit(fallback)
			r.auditDataTaskPlan(scope, dataRounds+1, fallback)
			currentPlan = setCurrentPlan(scope, dataRounds+1, fallback, evalDecision.Reason)
			continue
		case dataworkflow.EvaluationDecisionContinuePlan:
			emitWorkflowReason("continue", dataRounds, "")
			ctx := r.startTurn()
			view = runtimeView()
			continuationResult, contErr := dataTaskRunContinuationPlannerWithRuntimeView(ctx, continuer, line, r.repoRoot, policy, candidates, view, "")
			r.endTurn()
			r.emitReplLLMTrace(r.dataTaskPlanner, "data_task_continuation_planner", types.AgentName("data_planner"), types.PipelineStage("data"))
			if contErr != nil {
				msg := dataTaskErrorMarkdown(r.language, contErr.Error())
				r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: contErr.Error(), DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
				r.finishDataTaskRouteSpinner("failed")
				r.renderBordered(msg)
				r.lastAnswerOrigin = replAnswerOriginLocal
				r.recordTurn(display, line, msg, memory.KindPipeline)
				return
			}
			if strings.TrimSpace(continuationResult.FallbackReason) != "" {
				emitWorkflowReason("continue", dataRounds, continuationResult.FallbackReason)
				fallback := protectPlan(continuationResult.Plan)
				r.emitDataTaskPlanAudit(fallback)
				r.auditDataTaskPlan("continue", dataRounds+1, fallback)
				currentPlan = setCurrentPlan("continue", dataRounds+1, fallback, continuationResult.FallbackReason)
				continue
			}
			nextPlan := continuationResult.Plan
			if normalized, notes := normalizeDataTaskPlanShapeForPolicy(nextPlan, policy); len(notes) > 0 {
				logging.Info("[repl/data] normalized continuation data task plan: %s", strings.Join(notes, "; "))
				nextPlan = normalized
			}
			currentPlan = acceptCandidatePlan("continue", dataRounds+1, nextPlan)
			continue
		case dataworkflow.EvaluationDecisionRepairPlan:
			if evalDecision.Source == "completion_gate" {
				errText := evalDecision.Reason
				if !evalDecision.Guard.Empty() {
					emitWorkflowGuard("completion_gate", dataRounds, currentPlan, evalDecision.Guard)
					r.auditDataTaskError(dataRounds, errText)
					if len(records) > 0 {
						attachLastError(errText)
					}
				}
				transition := dataworkflow.ValidationFailureTransition{
					Action:    dataworkflow.ValidationFailureNeedsRepair,
					Reason:    errText,
					ErrorText: errText,
					Guard:     evalDecision.Guard,
				}
				if !evalDecision.Guard.Empty() {
					transition = dataTaskValidationFailureTransitionWithRepo(r.repoRoot, records, currentPlan, result, evalDecision.Guard)
				}
				recovery := dataworkflow.DecideValidationFailureRecovery(transition, repairOK, repairRounds < r.dataTaskMaxRepairRounds, errText)
				if recovery.Action == dataworkflow.FailureRecoveryFallbackPlan && recovery.HasPlan() {
					runtimeRecovery := workflowRuntime.ApplyFailureRecoveryDecision(dataRounds+1, "ledger", recovery, protectPlan)
					fallback := runtimeRecovery.Plan
					r.emitDataTaskPlanAudit(fallback)
					r.auditDataTaskPlan("ledger", dataRounds+1, fallback)
					renderAdmissionEvents(runtimeRecovery.Switch.ProcessEvents)
					currentPlan = fallback
					continue
				}
				if recovery.Action == dataworkflow.FailureRecoveryFail {
					msg := dataTaskErrorMarkdown(r.language, recovery.Reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: recovery.Reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				repairRounds = beginRepairIteration()
				repairView := runtimeView()
				repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, errText, "completion-repair")
				if repairErr != nil {
					reason := fmt.Sprintf("%s\nrepair data task completion: %v", errText, repairErr)
					msg := dataTaskErrorMarkdown(r.language, reason)
					r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
					r.finishDataTaskRouteSpinner("failed")
					r.renderBordered(msg)
					r.lastAnswerOrigin = replAnswerOriginLocal
					r.recordTurn(display, line, msg, memory.KindPipeline)
					return
				}
				applyRepairResult(repairResult)
				continue
			}
			view = runtimeView()
			repairRounds = beginRepairIteration()
			repairReason := evalDecision.Reason
			emitWorkflowFailure("repair", repairRounds, repairReason)
			repairView := runtimeView()
			repairResult, repairErr := r.repairDataTaskPlanForREPL(line, policy, candidates, repairView, repairReason, "repaired")
			if repairErr != nil {
				reason := fmt.Sprintf("%s\nrepair data task node: %v", repairReason, repairErr)
				msg := dataTaskErrorMarkdown(r.language, reason)
				r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
				r.finishDataTaskRouteSpinner("failed")
				r.renderBordered(msg)
				r.lastAnswerOrigin = replAnswerOriginLocal
				r.recordTurn(display, line, msg, memory.KindPipeline)
				return
			}
			applyRepairResult(repairResult)
			continue
		case dataworkflow.EvaluationDecisionFail:
			if !evalDecision.Guard.Empty() {
				emitWorkflowGuard("completion_gate", dataRounds, currentPlan, evalDecision.Guard)
				r.auditDataTaskError(dataRounds, evalDecision.Reason)
			}
			msg := dataTaskErrorMarkdown(r.language, evalDecision.Reason)
			r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "failed", Reason: evalDecision.Reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
			r.finishDataTaskRouteSpinner("failed")
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginLocal
			r.recordTurn(display, line, msg, memory.KindPipeline)
			return
		default:
			r.finishDataTaskWithFinalResult("partial_answer_possible", evalDecision.Reason, records, currentPlan, result, display, line, dataRounds, repairRounds)
			return
		}
	}
}

// operationDispatch is the independent operation/artifact route. Batch 2 is
// intentionally plan-only unless a provider is explicitly registered: it gives
// the user a structured, localized operation preflight without entering the
// source-analysis pipeline or executing side-effecting tools.
func (r *REPL) operationDispatch(line, display string, policy TurnPolicy) {
	if isCommandOperationPolicy(policy) {
		req := fallbackCommandOperationClarification(line, r.repoRoot)
		if r.operationPlanner != nil {
			r.startOperationSynthesisSpinner()
			ctx := r.startTurn()
			snapshot := r.commandOperationCapabilitySnapshot()
			var (
				planned operation.CommandOperationRequest
				err     error
			)
			if handoffPlanner, ok := r.operationPlanner.(CommandOperationPlannerWithHandoff); ok {
				planned, err = handoffPlanner.PlanCommandOperationWithHandoff(ctx, line, r.repoRoot, policy, snapshot, r.renderCommandOperationHandoff())
			} else if snapshotPlanner, ok := r.operationPlanner.(CommandOperationPlannerWithSnapshot); ok {
				planned, err = snapshotPlanner.PlanCommandOperationWithSnapshot(ctx, line, r.repoRoot, policy, snapshot)
			} else {
				planned, err = r.operationPlanner.PlanCommandOperation(ctx, line, r.repoRoot, policy)
			}
			r.endTurn()
			r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
			if err != nil {
				logging.Warning("[repl/operation] command planner failed; asking for clarification: %v", err)
			} else {
				req = planned
			}
		}
		plan := operation.BuildCommandOperationPlan(req, r.operationPolicy)
		if plan.Status == operation.StatusReady && !operation.LintCommandOperationPlan(plan).OK() {
			r.pendingCommandClarification = nil
			r.clearPendingOperationState()
			r.finishOperationAutoStartSpinner()
			r.renderBordered(commandOperationAutoValidateMarkdown(r.language, plan))
			r.executeCommandOperationPlan(plan, display, line)
			return
		}
		initialDecision := operation.DecideCommandPlanApproval(r.operationPolicy, plan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalInitial})
		plan = operation.ApplyCommandPlanApprovalDecision(plan, initialDecision)
		if plan.Status == operation.StatusReady && initialDecision.AutoExecute() {
			r.pendingCommandClarification = nil
			r.clearPendingOperationState()
			r.finishOperationAutoStartSpinner()
			r.renderBordered(commandOperationAutoExecuteMarkdown(r.language, plan))
			r.executeCommandOperationPlan(plan, display, line)
			return
		}
		if plan.Status == operation.StatusReady {
			r.pendingOperation = &plan
			r.pendingCommandClarification = nil
			r.savePendingOperationState()
		} else if plan.Status == operation.StatusNeedsClarification {
			r.pendingCommandClarification = &pendingCommandClarification{
				OriginalLine: line,
				Display:      display,
				Policy:       policy,
				Plan:         plan,
			}
			r.pendingOperation = nil
			r.savePendingOperationState()
		} else {
			r.pendingCommandClarification = nil
			r.pendingOperation = nil
			r.clearPendingOperationState()
		}
		r.operationHistory = append(r.operationHistory, plan)
		logging.Info("[repl/operation] command plan status=%s risk=%q approval=%q steps=%d request=%q",
			plan.Status, plan.RiskLevel, plan.ApprovalMode, len(plan.Steps), oneLineClamp(line, 160))
		msg := commandOperationPlanMarkdown(r.language, plan)
		r.finishOperationRouteSpinner(plan.Status)
		r.renderBordered(msg)
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return
	}

	plan := operation.BuildPlan(operation.Request{
		Text:                 line,
		Operation:            policy.Operation,
		OperationKind:        policy.OperationKind,
		Source:               policy.Source,
		NeedsRepoAccess:      policy.NeedsRepoAccess,
		RiskLevel:            policy.RiskLevel,
		SideEffects:          policy.SideEffects,
		TargetSurface:        policy.TargetSurface,
		RequiresConfirmation: policy.RequiresConfirmation,
	}, r.operationProviders)
	if plan.Provider != "" && plan.ProviderTool != "" {
		pending := pendingProviderOperation{
			Plan: plan,
			Request: operation.Request{
				Text:                 line,
				Operation:            policy.Operation,
				OperationKind:        policy.OperationKind,
				Source:               policy.Source,
				NeedsRepoAccess:      policy.NeedsRepoAccess,
				RiskLevel:            policy.RiskLevel,
				SideEffects:          policy.SideEffects,
				TargetSurface:        policy.TargetSurface,
				RequiresConfirmation: policy.RequiresConfirmation,
			},
			Display: display,
		}
		r.startProviderWorkflow(&pending)
		if plan.CanExecute {
			r.pendingProviderOperation = nil
			r.clearPendingOperationState()
			r.executeProviderOperationFlow(context.Background(), pending)
			return
		}
		r.pendingProviderOperation = &pending
		r.savePendingOperationState()
	} else {
		r.pendingProviderOperation = nil
		r.providerWorkflow = nil
		r.clearPendingOperationState()
	}
	logging.Info("[repl/operation] planned operation kind=%q target=%q risk=%q needs_repo=%t executable=%t provider=%q missing=%q",
		oneLineClamp(plan.Kind, 80),
		oneLineClamp(plan.TargetSurface, 60),
		oneLineClamp(plan.RiskLevel, 40),
		plan.NeedsRepoAccess,
		plan.CanExecute,
		oneLineClamp(plan.Provider, 80),
		oneLineClamp(plan.MissingCapability, 160))
	msg := operationPlanMarkdown(r.language, plan)
	if g := operationWritePipelineGuidance(r.language, policy); g != "" {
		msg += "\n\n" + g
	}
	r.finishProviderOperationPlanHeader(plan)
	r.renderBordered(msg)
	r.recordTurn(display, line, msg, memory.KindPipeline)
}

func (r *REPL) maybeDispatchCommandOperationFollowup(line, display string, policy, rawPolicy TurnPolicy) bool {
	if r.lastAnswerOrigin != replAnswerOriginCommandOperationFinal ||
		len(r.operationResults) == 0 ||
		r.operationPlanner == nil ||
		!commandOperationFollowupCandidatePolicy(policy, rawPolicy) {
		return false
	}
	continuer, ok := r.operationPlanner.(CommandOperationContinuationPlanner)
	if !ok || continuer == nil {
		return false
	}
	records := append([]commandOperationResultRecord(nil), r.operationResults...)
	r.startOperationSynthesisSpinner()
	ctx := r.startTurn()
	snapshot := r.commandOperationCapabilitySnapshot()
	next, err := continuer.ContinueCommandOperation(ctx, strings.TrimSpace(line), r.repoRoot, commandOperationFollowupPolicy(policy), snapshot, records)
	r.endTurn()
	r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
	if err != nil {
		logging.Warning("[repl/operation] command follow-up continuation planning failed: %v", err)
		return false
	}
	if next.Complete {
		logging.Info("[repl/operation] command follow-up judged complete; falling back to local route reason=%q",
			oneLineClamp(next.Reason, 160))
		return false
	}
	requestText := commandOperationFollowupRequestText(line, records)
	req := next.Request
	req.Text = requestText
	nextPlan := operation.BuildCommandOperationPlan(req, r.operationPolicy)
	previousPlan := records[len(records)-1].Plan
	decision := operation.DecideCommandPlanApproval(r.operationPolicy, nextPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalContinuation, PreviousPlan: &previousPlan})
	nextPlan = operation.ApplyCommandPlanApprovalDecision(nextPlan, decision)
	logging.Info("[repl/operation] command follow-up generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d",
		previousPlan.ID, nextPlan.Status, nextPlan.RiskLevel, nextPlan.ApprovalMode, decision.Action, decision.ReasonCode, len(nextPlan.Steps))
	if nextPlan.Status == operation.StatusReady {
		if lint := operation.LintCommandOperationPlan(nextPlan); !lint.OK() {
			r.pendingCommandClarification = nil
			r.clearPendingOperationState()
			r.finishOperationAutoStartSpinner()
			r.renderBordered(commandOperationAutoValidateMarkdown(r.language, nextPlan))
			r.executeCommandOperationPlanAttempt(nextPlan, requestText, display, 0, records)
			return true
		}
		if decision.AutoExecute() {
			msg := commandOperationContinuationIntro(r.language, nextPlan)
			msg += "\n\n"
			msg += commandOperationAutoExecuteMarkdown(r.language, nextPlan)
			r.finishCommandOperationPlanHeader(nextPlan)
			r.renderBordered(msg)
			r.executeCommandOperationPlanAttempt(nextPlan, requestText, display, 0, records)
			return true
		}
		r.pendingOperation = &nextPlan
		r.savePendingOperationState()
	}
	r.operationHistory = append(r.operationHistory, nextPlan)
	msg := commandOperationContinuationIntro(r.language, nextPlan)
	msg += "\n\n"
	msg += commandOperationPlanMarkdown(r.language, nextPlan)
	r.finishOperationRouteSpinner(nextPlan.Status)
	r.renderBorderedCompact(msg)
	r.lastAnswerOrigin = replAnswerOriginLocal
	r.recordTurn(display, requestText, msg, memory.KindPipeline)
	return true
}

func commandOperationFollowupCandidatePolicy(policy, rawPolicy TurnPolicy) bool {
	switch policy.Route {
	case RouteLocal, RouteRepo, RouteHybrid:
	default:
		return false
	}
	rawSource := strings.ToLower(strings.TrimSpace(rawPolicy.Source))
	if rawSource == "" {
		rawSource = strings.ToLower(strings.TrimSpace(policy.Source))
	}
	switch rawSource {
	case "last_answer", "prior_context":
	default:
		return false
	}
	if rawPolicy.NeedsRepoAccess {
		return false
	}
	if rawPolicy.Route == RouteOperation || rawPolicy.NeedsOperationAccess {
		return true
	}
	switch rawPolicy.Route {
	case RouteLocal, RouteRepo, RouteHybrid:
	case "":
	default:
		return false
	}
	source := strings.ToLower(strings.TrimSpace(policy.Source))
	switch source {
	case "last_answer", "prior_context":
	default:
		return false
	}
	switch strings.ToLower(strings.TrimSpace(policy.Operation)) {
	case "chat", "transform", "summarize", "translate":
		return false
	}
	return true
}

func commandOperationFollowupPolicy(policy TurnPolicy) TurnPolicy {
	policy.Route = RouteOperation
	policy.NeedsOperationAccess = true
	if strings.TrimSpace(policy.Operation) == "" || strings.TrimSpace(policy.Operation) == "elaborate" {
		policy.Operation = "computer_operation"
	}
	if strings.TrimSpace(policy.OperationKind) == "" {
		policy.OperationKind = "computer_operation"
	}
	if strings.TrimSpace(policy.TargetSurface) == "" {
		policy.TargetSurface = "desktop"
	}
	if strings.TrimSpace(policy.Source) == "" {
		policy.Source = "prior_context"
	}
	return policy
}

func commandOperationFollowupRequestText(line string, records []commandOperationResultRecord) string {
	line = strings.TrimSpace(line)
	base := ""
	for i := len(records) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(records[i].Plan.RequestText); text != "" {
			base = text
			break
		}
	}
	if base == "" {
		return line
	}
	if line == "" || line == base {
		return base
	}
	return base + "\n\nFollow-up request:\n" + line
}

func (r *REPL) resumeCommandOperationClarification(line, display string) {
	pending := r.pendingCommandClarification
	if pending == nil {
		return
	}
	r.pendingCommandClarification = nil
	if r.renderer != nil {
		r.setRendererTotalStagesForCurrentMode()
		r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	}
	combined := strings.TrimSpace(pending.OriginalLine)
	if combined == "" {
		combined = strings.TrimSpace(pending.Display)
	}
	if combined == "" {
		combined = strings.TrimSpace(line)
	} else {
		combined += "\n\nUser supplied missing operation details:\n" + strings.TrimSpace(line)
	}
	resumeDisplay := strings.TrimSpace(display)
	if resumeDisplay == "" {
		resumeDisplay = strings.TrimSpace(line)
	}
	logging.Info("[repl/operation] resuming command clarification plan_id=%s answer=%q",
		pending.Plan.ID, oneLineClamp(line, 160))
	r.clearPendingOperationState()
	r.operationDispatch(combined, resumeDisplay, pending.Policy)
}

func (r *REPL) finishOperationRouteSpinner(status operation.OperationStatus) {
	if r.renderer == nil {
		return
	}
	label, segs := operationRouteSummary(r.language, status)
	r.emitOperationLightRouteSummary(label, segs)
}

func (r *REPL) finishDataTaskRouteSpinner(status string) {
	if r.renderer == nil {
		return
	}
	label := "数据处理"
	segs := []string{"已完成", "未读源码"}
	if !isZh(r.language) {
		label = "Data task"
		segs = []string{"complete", "no source read"}
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "needs_clarification":
		if isZh(r.language) {
			segs[0] = "需要补充信息"
		} else {
			segs[0] = "needs input"
		}
	case "blocked":
		if isZh(r.language) {
			segs[0] = "已阻止"
		} else {
			segs[0] = "blocked"
		}
	case "failed":
		if isZh(r.language) {
			segs[0] = "未完成"
		} else {
			segs[0] = "failed"
		}
	case "budget_exhausted":
		if isZh(r.language) {
			segs[0] = "预算用尽"
		} else {
			segs[0] = "budget exhausted"
		}
	case "partial_answer_possible":
		if isZh(r.language) {
			segs[0] = "部分结果"
		} else {
			segs[0] = "partial"
		}
	}
	r.emitOperationLightRouteSummary(label, segs)
}

func (r *REPL) repairDataTaskPlanForREPL(line string, policy TurnPolicy, candidates []dataquery.CandidateFile, view dataTaskWorkflowRuntimeView, errText, normalizeScope string) (dataTaskRepairPlanResult, error) {
	repairer, ok := r.dataTaskPlanner.(DataTaskRepairPlanner)
	if !ok {
		return dataTaskRepairPlanResult{}, fmt.Errorf("data task repair planner is unavailable")
	}
	if !dataTaskPlanHasRuntimeShape(view.CurrentPlan) {
		return dataTaskRepairPlanResult{}, fmt.Errorf("data task repair planner has no current workflow plan")
	}
	ctx := r.startTurn()
	repairedPlan, repairErr := dataTaskRunRepairPlannerWithRuntimeView(ctx, repairer, line, r.repoRoot, policy, candidates, view.CurrentPlan, errText, view)
	r.endTurn()
	r.emitReplLLMTrace(r.dataTaskPlanner, "data_task_repair_planner", types.AgentName("data_planner"), types.PipelineStage("data"))
	if repairErr != nil {
		if fallback, reason, ok := r.dataTaskRepairFailureContinuationFallback(line, policy, candidates, view, repairErr); ok {
			return dataTaskRepairPlanResult{Plan: fallback, FallbackReason: reason}, nil
		}
		return dataTaskRepairPlanResult{}, repairErr
	}
	if normalized, notes := normalizeDataTaskPlanShapeForPolicy(repairedPlan, policy); len(notes) > 0 {
		normalizeScope = strings.TrimSpace(normalizeScope)
		if normalizeScope == "" {
			normalizeScope = "repaired"
		}
		logging.Info("[repl/data] normalized %s data task plan: %s", normalizeScope, strings.Join(notes, "; "))
		repairedPlan = normalized
	}
	return dataTaskRepairPlanResult{Plan: repairedPlan}, nil
}

func (r *REPL) dataTaskRepairFailureContinuationFallback(line string, policy TurnPolicy, candidates []dataquery.CandidateFile, view dataTaskWorkflowRuntimeView, repairErr error) (dataquery.TaskPlan, string, bool) {
	if !dataTaskRepairFailureContinuationAvailable(r.dataTaskPlanner, view, repairErr) {
		return dataquery.TaskPlan{}, "", false
	}
	ctx := r.startTurn()
	result, _, ok, contErr := dataTaskRepairFailureContinuationWithRuntimeView(ctx, r.dataTaskPlanner, line, r.repoRoot, policy, candidates, view, repairErr)
	r.endTurn()
	r.emitReplLLMTrace(r.dataTaskPlanner, "data_task_continuation_after_repair_failure", types.AgentName("data_planner"), types.PipelineStage("data"))
	if contErr != nil {
		logging.Warning("[repl/data] repair planner continuation fallback failed: %v", contErr)
		return dataquery.TaskPlan{}, "", false
	}
	if !ok {
		return dataquery.TaskPlan{}, "", false
	}
	nextPlan := result.Plan
	if normalized, notes := normalizeDataTaskPlanShapeForPolicy(nextPlan, policy); len(notes) > 0 {
		logging.Info("[repl/data] normalized continuation-after-repair data task plan: %s", strings.Join(notes, "; "))
		nextPlan = normalized
	}
	return prepareDataTaskWorkflowPlanForExecution(line, candidates, view.Records, nextPlan), result.FallbackReason, true
}

func (r *REPL) startDataTaskPlanningSpinner() {
	if r.renderer == nil {
		return
	}
	label, segs := dataTaskPlanningSummary(r.language)
	r.renderer.SetTotalStages(0)
	r.renderer.SetRouteSummary(label, segs)
	if r.renderer.SpinnerActive() {
		r.renderer.SetLightRouteActivity(label)
		return
	}
	r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	r.renderer.SetLightRouteActivity(label)
}

func dataTaskPlanningSummary(lang string) (string, []string) {
	if isZh(lang) {
		return "数据任务", []string{"规划中", "未读源码"}
	}
	return "data task", []string{"planning", "no source read"}
}

func (r *REPL) emitReplLLMTrace(source any, fallbackScope string, agent types.AgentName, stage types.PipelineStage) {
	provider, ok := source.(replLLMTraceProvider)
	if !ok {
		return
	}
	trace := provider.LastReplLLMTrace()
	scope := firstNonEmptyString(strings.TrimSpace(trace.Scope), strings.TrimSpace(fallbackScope))
	if strings.TrimSpace(trace.Reasoning) != "" {
		logging.Debug("[diag %s] phase=repl_llm_trace reasoning_len=%d\n%s", firstNonEmptyString(scope, "repl_llm"), len(trace.Reasoning), trace.Reasoning)
	}
	if strings.TrimSpace(trace.Content) != "" {
		logging.Debug("[diag %s] phase=repl_llm_trace content_len=%d\n%s", firstNonEmptyString(scope, "repl_llm"), len(trace.Content), trace.Content)
	}
	if strings.TrimSpace(trace.ToolName) != "" {
		logging.Debug("[diag %s] phase=repl_llm_trace tool=%s params_len=%d params:\n%s\n---",
			firstNonEmptyString(scope, "repl_llm"),
			trace.ToolName,
			len(trace.ToolParams),
			string(trace.ToolParams))
	}
	if r.renderer == nil {
		return
	}
	if strings.TrimSpace(trace.Reasoning) != "" {
		r.renderer.Emitter()(render.Event{
			Kind:      render.EventAgentReasoning,
			Timestamp: time.Now(),
			Agent:     agent,
			Stage:     stage,
			Reasoning: trace.Reasoning,
		})
	}
	if strings.TrimSpace(trace.ToolName) != "" {
		r.renderer.Emitter()(render.Event{
			Kind:          render.EventAgentToolCallBatch,
			Timestamp:     time.Now(),
			Agent:         agent,
			Stage:         stage,
			ToolName:      trace.ToolName,
			ToolNames:     []string{trace.ToolName},
			ToolCallCount: 1,
			ToolDetail:    replLLMTraceScopeLabel(scope, r.language),
		})
	}
}

func replLLMTraceScopeLabel(scope, lang string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return ""
	}
	if isZh(lang) {
		switch scope {
		case "turn_policy_classifier":
			return "结构化路由判定"
		case "data_task_planner":
			return "数据任务计划"
		case "data_task_repair_planner":
			return "数据任务修复"
		case "data_task_evaluator":
			return "数据目标评估"
		case "data_task_continuation_planner":
			return "数据继续计划"
		case "data_task_structured_tool_repair":
			return "数据结构化参数修复"
		case "command_operation_planner":
			return "操作计划"
		case "command_operation_evaluator":
			return "操作目标评估"
		case "command_operation_answerer":
			return "操作结果成文"
		case "provider_operation_evaluator":
			return "外部 provider 目标评估"
		case "provider_operation_answerer":
			return "外部 provider 成文"
		case "operation_structured_tool_repair":
			return "结构化参数修复"
		}
	}
	return strings.ReplaceAll(scope, "_", " ")
}

func (r *REPL) emitTurnPolicyAudit(policy TurnPolicy) {
	if r.renderer == nil {
		return
	}
	label, segs := turnPolicyAuditSummary(policy, r.language)
	r.renderer.EmitLightRouteSummary(label, segs)
}

func turnPolicyAuditSummary(policy TurnPolicy, lang string) (string, []string) {
	route := turnPolicyRouteDisplay(policy.Route, lang)
	conf := fmt.Sprintf("%.0f%%", policy.Confidence*100)
	if isZh(lang) {
		segs := []string{route, "置信 " + conf}
		if policy.NeedsRepoAccess {
			segs = append(segs, "需读源码/观测")
		} else {
			segs = append(segs, "未读源码")
		}
		if strings.TrimSpace(policy.Operation) != "" {
			segs = append(segs, "意图 "+strings.TrimSpace(policy.Operation))
		}
		return "路由判定", segs
	}
	segs := []string{route, "confidence " + conf}
	if policy.NeedsRepoAccess {
		segs = append(segs, "repo/external read")
	} else {
		segs = append(segs, "no source read")
	}
	if strings.TrimSpace(policy.Operation) != "" {
		segs = append(segs, "intent "+strings.TrimSpace(policy.Operation))
	}
	return "route decision", segs
}

func turnPolicyRouteDisplay(route TurnRoute, lang string) string {
	raw := strings.TrimSpace(string(route))
	if raw == "" {
		raw = "unknown"
	}
	if !isZh(lang) {
		return raw
	}
	switch route {
	case RouteOperation:
		return "操作"
	case RouteData:
		return "数据"
	case RouteRepo:
		return "源码"
	case RouteWrite:
		return "写代码"
	case RouteHybrid:
		return "混合"
	case RouteLocal:
		return "本地回复"
	case RouteClarify:
		return "澄清"
	default:
		return raw
	}
}

func (r *REPL) emitDataTaskPlanAudit(plan dataquery.TaskPlan) {
	if r.renderer == nil {
		return
	}
	label, segs := dataTaskPlanAuditSummary(plan, r.language)
	r.renderer.EmitLightRouteSummary(label, segs)
	if details := dataTaskPlanAuditDetailLines(plan, r.language); len(details) > 0 {
		r.renderBorderedDataProcessCompact(strings.Join(details, "\n"))
	}
}

type dataTaskAuditArtifact struct {
	PlanPath          string
	ScriptPath        string
	ActionGraphPath   string
	ArtifactGraphPath string
	ResultPath        string
	ErrorPath         string
}

type dataTaskActionAuditSnapshot struct {
	Scope   string                            `json:"scope,omitempty"`
	Round   int                               `json:"round,omitempty"`
	Actions []dataTaskActionAuditSnapshotItem `json:"actions,omitempty"`
}

type dataTaskActionAuditSnapshotItem struct {
	Original   dataquery.DataAction    `json:"original_action,omitempty"`
	Normalized dataquery.DataAction    `json:"normalized_action,omitempty"`
	Node       dataworkflow.ActionNode `json:"action_node,omitempty"`
}

func (r *REPL) auditDataTaskPlan(scope string, round int, plan dataquery.TaskPlan) dataTaskAuditArtifact {
	artifact := r.writeDataTaskPlanArtifact(scope, round, plan)
	r.logDataTaskPlanArtifact(scope, round, plan, artifact)
	r.emitDataTaskPlanPreview(scope, round, plan, artifact)
	return artifact
}

func (r *REPL) writeDataTaskPlanArtifact(scope string, round int, plan dataquery.TaskPlan) dataTaskAuditArtifact {
	dir := filepath.Join(firstNonEmptyString(r.runtimeAnchor, r.repoRoot, "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[repl/data] create audit dir failed: %v", err)
		return dataTaskAuditArtifact{}
	}
	stamp := dataTaskAuditStamp()
	prefix := fmt.Sprintf("%s-%d-%s-r%d", stamp, os.Getpid(), dataTaskAuditNamePart(scope), round)
	artifact := dataTaskAuditArtifact{}
	planPath := filepath.Join(dir, prefix+".plan.json")
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		logging.Warning("[repl/data] marshal data task plan failed scope=%s round=%d: %v", scope, round, err)
	} else if err := os.WriteFile(planPath, raw, 0600); err != nil {
		logging.Warning("[repl/data] write data task plan audit failed path=%s: %v", planPath, err)
	} else {
		artifact.PlanPath = planPath
	}
	if strings.TrimSpace(plan.Script) != "" {
		scriptPath := filepath.Join(dir, prefix+".script.py")
		if err := os.WriteFile(scriptPath, []byte(plan.Script), 0600); err != nil {
			logging.Warning("[repl/data] write data task script audit failed path=%s: %v", scriptPath, err)
		} else {
			artifact.ScriptPath = scriptPath
		}
	}
	if len(plan.Actions) > 0 {
		actionsPath := filepath.Join(dir, prefix+".actions.json")
		snapshot := dataTaskActionAuditSnapshotFor(scope, round, plan.Actions)
		raw, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			logging.Warning("[repl/data] marshal data task action audit failed scope=%s round=%d: %v", scope, round, err)
		} else if err := os.WriteFile(actionsPath, raw, 0600); err != nil {
			logging.Warning("[repl/data] write data task action audit failed path=%s: %v", actionsPath, err)
		} else {
			artifact.ActionGraphPath = actionsPath
		}
	}
	return artifact
}

func (r *REPL) logDataTaskPlanArtifact(scope string, round int, plan dataquery.TaskPlan, artifact dataTaskAuditArtifact) {
	logging.Info("[repl/data] data task plan scope=%s round=%d status=%s inputs=%d required=%d actions=%d script_lines=%d plan_path=%s script_path=%s actions_path=%s",
		scope, round, strings.TrimSpace(plan.Status), len(plan.InputPaths), len(plan.CoverageContract.RequiredPaths()),
		len(plan.Actions), dataTaskScriptLineCount(plan.Script), artifact.PlanPath, artifact.ScriptPath, artifact.ActionGraphPath)
	if strings.TrimSpace(plan.Script) != "" {
		logging.Info("[repl/data] data task script full scope=%s round=%d path=%s\n%s",
			scope, round, artifact.ScriptPath, plan.Script)
	}
	if raw, err := json.MarshalIndent(plan, "", "  "); err == nil {
		logging.Info("[repl/data] data task plan full scope=%s round=%d path=%s\n%s",
			scope, round, artifact.PlanPath, string(raw))
	}
	if len(plan.Actions) > 0 {
		snapshot := dataTaskActionAuditSnapshotFor(scope, round, plan.Actions)
		if raw, err := json.MarshalIndent(snapshot, "", "  "); err == nil {
			logging.Info("[repl/data] data task actions full scope=%s round=%d path=%s\n%s",
				scope, round, artifact.ActionGraphPath, string(raw))
		}
	}
}

func dataTaskActionAuditSnapshotFor(scope string, round int, actions []dataquery.DataAction) dataTaskActionAuditSnapshot {
	status := dataworkflow.ActionStatusReady
	if strings.EqualFold(strings.TrimSpace(scope), "deferred") {
		status = dataworkflow.ActionStatusDeferred
	}
	out := dataTaskActionAuditSnapshot{
		Scope: strings.TrimSpace(scope),
		Round: round,
	}
	for _, action := range actions {
		normalized, _ := dataworkflow.NormalizeRolePathAction(action)
		out.Actions = append(out.Actions, dataTaskActionAuditSnapshotItem{
			Original:   action,
			Normalized: normalized,
			Node:       dataworkflow.ActionNodeFor(action, status),
		})
	}
	return out
}

func (r *REPL) emitDataTaskPlanPreview(scope string, round int, plan dataquery.TaskPlan, artifact dataTaskAuditArtifact) {
	script := strings.TrimSpace(plan.Script)
	if r.renderer == nil || (script == "" && len(plan.Actions) == 0) {
		return
	}
	lines := dataTaskScriptLineCount(script)
	label := "data script"
	if isZh(r.language) {
		label = "数据脚本"
	}
	if script == "" && len(plan.Actions) > 0 {
		label = "data actions"
		if isZh(r.language) {
			label = "数据动作"
		}
	}
	segs := []string{dataTaskAuditScopeLabel(scope, round, r.language)}
	if len(plan.Actions) > 0 {
		if isZh(r.language) {
			segs = append(segs, fmt.Sprintf("%d 个动作", len(plan.Actions)))
		} else {
			segs = append(segs, fmt.Sprintf("%d action(s)", len(plan.Actions)))
		}
	}
	if lines > 0 {
		if isZh(r.language) {
			segs = append(segs, fmt.Sprintf("%d 行", lines))
		} else {
			segs = append(segs, fmt.Sprintf("%d lines", lines))
		}
	}
	if artifact.ScriptPath != "" {
		if isZh(r.language) {
			segs = append(segs, "完整脚本 "+artifact.ScriptPath)
		} else {
			segs = append(segs, "full script "+artifact.ScriptPath)
		}
	}
	r.renderer.EmitLightRouteSummary(label, segs)
	var b strings.Builder
	if script == "" && len(plan.Actions) > 0 {
		if isZh(r.language) {
			b.WriteString("动作预览：\n")
		} else {
			b.WriteString("Action preview:\n")
		}
		b.WriteString(dataTaskPanelPreview(renderDataTaskActionsPreview(plan.Actions), r.language))
	} else if isZh(r.language) {
		b.WriteString("脚本预览：\n")
	} else {
		b.WriteString("Script preview:\n")
	}
	if script != "" {
		b.WriteString(dataTaskPanelPreview(script, r.language))
	}
	if artifact.ScriptPath != "" {
		if isZh(r.language) {
			fmt.Fprintf(&b, "\n\n完整脚本：`%s`", artifact.ScriptPath)
		} else {
			fmt.Fprintf(&b, "\n\nFull script: `%s`", artifact.ScriptPath)
		}
	}
	if artifact.PlanPath != "" {
		if isZh(r.language) {
			fmt.Fprintf(&b, "\n完整计划：`%s`", artifact.PlanPath)
		} else {
			fmt.Fprintf(&b, "\nFull plan: `%s`", artifact.PlanPath)
		}
	}
	if artifact.ActionGraphPath != "" {
		if isZh(r.language) {
			fmt.Fprintf(&b, "\n完整动作图：`%s`", artifact.ActionGraphPath)
		} else {
			fmt.Fprintf(&b, "\nFull action graph: `%s`", artifact.ActionGraphPath)
		}
	}
	r.renderBorderedMutedCompact(b.String())
}

func (r *REPL) auditDataTaskResult(round int, result dataquery.Result) dataTaskAuditArtifact {
	artifact := r.writeDataTaskResultArtifact(round, result)
	r.logDataTaskResultArtifact(round, result, artifact)
	r.emitDataTaskResultPreview(round, result, artifact)
	return artifact
}

func (r *REPL) auditDataTaskError(round int, errText string) dataTaskAuditArtifact {
	artifact := r.writeDataTaskErrorArtifact(round, errText)
	r.logDataTaskErrorArtifact(round, errText, artifact)
	r.emitDataTaskErrorPreview(round, errText, artifact)
	return artifact
}

func (r *REPL) writeDataTaskErrorArtifact(round int, errText string) dataTaskAuditArtifact {
	dir := filepath.Join(firstNonEmptyString(r.runtimeAnchor, r.repoRoot, "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[repl/data] create audit dir failed: %v", err)
		return dataTaskAuditArtifact{}
	}
	stamp := dataTaskAuditStamp()
	errorPath := filepath.Join(dir, fmt.Sprintf("%s-%d-error-r%d.txt", stamp, os.Getpid(), round))
	if err := os.WriteFile(errorPath, []byte(errText), 0600); err != nil {
		logging.Warning("[repl/data] write data task error audit failed path=%s: %v", errorPath, err)
		return dataTaskAuditArtifact{}
	}
	return dataTaskAuditArtifact{ErrorPath: errorPath}
}

func (r *REPL) logDataTaskErrorArtifact(round int, errText string, artifact dataTaskAuditArtifact) {
	logging.Info("[repl/data] data task error round=%d error_path=%s\n%s", round, artifact.ErrorPath, errText)
}

func (r *REPL) logDataTaskTerminal(a dataTaskTerminalAudit) {
	status := firstNonEmptyString(strings.TrimSpace(a.Status), "unknown")
	reason := strings.TrimSpace(a.Reason)
	lastErr := dataTaskLatestError(a.Records)
	resultSummary := dataTaskTerminalResultSummary(a.Result, a.Records)
	terminal := writeDataTaskTerminalArtifactFileWithRuntimeDetailed(r.runtimeAnchor, r.repoRoot, r.activeDataWorkflowRuntime, a, status, reason, lastErr, resultSummary, "repl")
	terminalPath := terminal.Path
	status = firstNonEmptyString(strings.TrimSpace(terminal.Snapshot.Status), status)
	reason = firstNonEmptyString(strings.TrimSpace(terminal.Snapshot.Reason), reason)
	lastErr = terminal.LastError
	lastNonterminalErr := terminal.LastNonterminalError
	resultSummary = terminal.ResultSummary
	recordCount := terminal.RecordCount
	if recordCount == 0 {
		recordCount = len(a.Records)
	}
	logging.Info("[repl/data] terminal status=%s data_rounds=%d repair_rounds=%d records=%d result=%s reason=%q last_error=%q last_nonterminal_error=%q terminal_path=%s",
		status, terminal.DataRounds, terminal.RepairRounds, recordCount, resultSummary,
		oneLineClamp(reason, 500), oneLineClamp(lastErr, 500), oneLineClamp(lastNonterminalErr, 500), terminalPath)
	if reason != "" || lastErr != "" || lastNonterminalErr != "" {
		logging.Info("[repl/data] terminal detail status=%s\nreason:\n%s\nlast_error:\n%s\nlast_nonterminal_error:\n%s",
			status, reason, lastErr, lastNonterminalErr)
	}
	r.emitDataTaskTerminalAuditPath(status, terminalPath)
}

func (r *REPL) emitDataTaskTerminalAuditPath(status, terminalPath string) {
	if r.renderer == nil || strings.TrimSpace(terminalPath) == "" {
		return
	}
	label := "data audit"
	segs := []string{strings.TrimSpace(status), "terminal " + terminalPath}
	if isZh(r.language) {
		label = "数据审计"
		segs = []string{strings.TrimSpace(status), "完整终态 " + terminalPath}
	}
	r.renderer.EmitLightRouteSummary(label, cleanDataTaskStrings(segs))
}

func (r *REPL) writeDataTaskTerminalArtifact(a dataTaskTerminalAudit, status, reason, lastErr, resultSummary string) string {
	return writeDataTaskTerminalArtifactFile(r.runtimeAnchor, r.repoRoot, a, status, reason, lastErr, resultSummary, "repl")
}

func writeDataTaskTerminalArtifactFile(runtimeAnchor, repoRoot string, a dataTaskTerminalAudit, status, reason, lastErr, resultSummary, logScope string) string {
	return writeDataTaskTerminalArtifactFileWithRuntime(runtimeAnchor, repoRoot, nil, a, status, reason, lastErr, resultSummary, logScope)
}

func writeDataTaskTerminalArtifactFileWithRuntime(runtimeAnchor, repoRoot string, workflowRuntime *dataworkflow.WorkflowRuntime, a dataTaskTerminalAudit, status, reason, lastErr, resultSummary, logScope string) string {
	return writeDataTaskTerminalArtifactFileWithRuntimeDetailed(runtimeAnchor, repoRoot, workflowRuntime, a, status, reason, lastErr, resultSummary, logScope).Path
}

type dataTaskTerminalArtifactWrite struct {
	Path                 string
	Snapshot             dataworkflow.WorkflowJournal
	LastError            string
	LastNonterminalError string
	ResultSummary        string
	DataRounds           int
	RepairRounds         int
	RecordCount          int
}

func writeDataTaskTerminalArtifactFileWithRuntimeDetailed(runtimeAnchor, repoRoot string, workflowRuntime *dataworkflow.WorkflowRuntime, a dataTaskTerminalAudit, status, reason, lastErr, resultSummary, logScope string) dataTaskTerminalArtifactWrite {
	out := dataTaskTerminalArtifactWrite{
		LastError:     lastErr,
		ResultSummary: resultSummary,
		DataRounds:    a.DataRounds,
		RepairRounds:  a.RepairRounds,
		RecordCount:   len(a.Records),
	}
	dir := filepath.Join(firstNonEmptyString(runtimeAnchor, repoRoot, "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[%s/data] create audit dir failed: %v", firstNonEmptyString(logScope, "data"), err)
		return out
	}
	records := a.Records
	current := dataquery.TaskPlan{}
	deferredQueue := dataworkflow.DeferredQueueState{}
	deferred := dataquery.TaskPlan{}
	if workflowRuntime != nil {
		runtimeSnapshot := workflowRuntime.Snapshot()
		records = runtimeSnapshot.Records
		if runtimeSnapshot.DataRounds > 0 {
			a.DataRounds = runtimeSnapshot.DataRounds
		}
		if runtimeSnapshot.RepairRounds > 0 {
			a.RepairRounds = runtimeSnapshot.RepairRounds
		}
		current = runtimeSnapshot.CurrentPlan
		deferredQueue = runtimeSnapshot.DeferredQueue
		deferred = runtimeSnapshot.DeferredPlan
		lastErr = dataTaskLatestError(records)
		resultSummary = dataTaskTerminalResultSummary(a.Result, records)
	}
	out.LastError = lastErr
	out.ResultSummary = resultSummary
	out.DataRounds = a.DataRounds
	out.RepairRounds = a.RepairRounds
	out.RecordCount = len(records)
	state := dataTaskWorkflowStateFromRuntimeView(dataTaskWorkflowRuntimeView{
		Records:       records,
		CurrentPlan:   current,
		DeferredQueue: deferredQueue,
		DeferredPlan:  deferred,
		DataRounds:    a.DataRounds,
		RepairRounds:  a.RepairRounds,
	})
	snapshot := workflowRuntime.BuildJournalSnapshot(dataworkflow.WorkflowJournalBuildInput{
		Status:        status,
		Reason:        reason,
		DataRounds:    a.DataRounds,
		RepairRounds:  a.RepairRounds,
		ResultSummary: resultSummary,
		LastError:     lastErr,
		Records:       records,
		CurrentPlan:   current,
		DeferredPlan:  deferred,
		ProcessEvents: a.ProcessEvents,
		State:         state.Snapshot(),
		Guards:        a.Guards,
		GuardRound:    a.DataRounds,
	})
	out.Snapshot = snapshot
	out.LastError = snapshot.LastError
	out.LastNonterminalError = snapshot.LastNonterminalError
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		logging.Warning("[%s/data] marshal data task terminal audit failed: %v", firstNonEmptyString(logScope, "data"), err)
		return out
	}
	stamp := dataTaskAuditStamp()
	path := filepath.Join(dir, fmt.Sprintf("%s-%d-terminal.json", stamp, os.Getpid()))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		logging.Warning("[%s/data] write data task terminal audit failed path=%s: %v", firstNonEmptyString(logScope, "data"), path, err)
		return out
	}
	out.Path = path
	logging.Info("[%s/data] terminal full path=%s\n%s", firstNonEmptyString(logScope, "data"), path, string(raw))
	return out
}

func writeDataTaskWorkflowCheckpointFile(runtimeAnchor, repoRoot string, records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan, dataRounds, repairRounds int, reason, logScope string, guards ...dataworkflow.GuardResult) string {
	return writeDataTaskWorkflowCheckpointFileWithDeferredQueue(runtimeAnchor, repoRoot, nil, records, current, dataworkflow.NewDeferredQueue(deferred), dataRounds, repairRounds, reason, logScope, guards...)
}

func writeDataTaskWorkflowCheckpointFileWithDeferredQueue(runtimeAnchor, repoRoot string, workflowRuntime *dataworkflow.WorkflowRuntime, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState, dataRounds, repairRounds int, reason, logScope string, guards ...dataworkflow.GuardResult) string {
	dir := filepath.Join(firstNonEmptyString(runtimeAnchor, repoRoot, "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[%s/data] create audit dir failed: %v", firstNonEmptyString(logScope, "data"), err)
		return ""
	}
	if workflowRuntime != nil {
		runtimeSnapshot := workflowRuntime.Snapshot()
		records = runtimeSnapshot.Records
		current = runtimeSnapshot.CurrentPlan
		if runtimeSnapshot.DataRounds > 0 {
			dataRounds = runtimeSnapshot.DataRounds
		}
		if runtimeSnapshot.RepairRounds > 0 {
			repairRounds = runtimeSnapshot.RepairRounds
		}
		deferredQueue = runtimeSnapshot.DeferredQueue
	}
	deferred := dataworkflow.DeferredQueuePlan(deferredQueue)
	state := dataTaskWorkflowStateFromRuntimeView(dataTaskWorkflowRuntimeView{
		Records:       records,
		CurrentPlan:   current,
		DeferredQueue: deferredQueue,
		DeferredPlan:  deferred,
		DataRounds:    dataRounds,
		RepairRounds:  repairRounds,
	})
	if len(deferred.Actions) > 0 {
		status := dataTaskDeferredQueueStatus(records, deferred)
		state.ActionGraph.DeferredQueue = dataworkflow.DeferredQueueSnapshotForStatus(status, dataworkflow.DecideDeferredQueueLifecycle(status))
	}
	snapshot := workflowRuntime.BuildJournalSnapshot(dataworkflow.WorkflowJournalBuildInput{
		Status:        "checkpoint",
		Reason:        reason,
		DataRounds:    dataRounds,
		RepairRounds:  repairRounds,
		ResultSummary: dataTaskTerminalResultSummary(nil, records),
		LastError:     dataTaskLatestError(records),
		Records:       records,
		CurrentPlan:   current,
		DeferredPlan:  deferred,
		State:         state.Snapshot(),
		Guards:        guards,
		GuardRound:    dataRounds,
	})
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		logging.Warning("[%s/data] marshal data task checkpoint failed: %v", firstNonEmptyString(logScope, "data"), err)
		return ""
	}
	stamp := dataTaskAuditStamp()
	path := filepath.Join(dir, fmt.Sprintf("%s-%d-checkpoint-r%d.json", stamp, os.Getpid(), dataRounds))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		logging.Warning("[%s/data] write data task checkpoint failed path=%s: %v", firstNonEmptyString(logScope, "data"), path, err)
		return ""
	}
	logging.Info("[%s/data] checkpoint full path=%s\n%s", firstNonEmptyString(logScope, "data"), path, string(raw))
	return path
}

func dataTaskWorkflowJournalEvents(records []dataTaskWorkflowRecord) []dataworkflow.WorkflowJournalEvent {
	return dataworkflow.BuildWorkflowJournalEvents(records)
}

func appendArtifactSchemaProjections(base []dataworkflow.ArtifactSchemaProjection, extra ...dataworkflow.ArtifactSchemaProjection) []dataworkflow.ArtifactSchemaProjection {
	seen := map[string]bool{}
	var out []dataworkflow.ArtifactSchemaProjection
	for _, list := range [][]dataworkflow.ArtifactSchemaProjection{base, extra} {
		for _, artifact := range list {
			key := strings.Join(append([]string{strings.TrimSpace(artifact.ID)}, artifact.Aliases...), "\x00")
			if key == "" {
				key = strings.TrimSpace(artifact.Kind) + "\x00" + strings.Join(artifact.SourcePaths, "\x00")
			}
			if key != "" && seen[key] {
				continue
			}
			if key != "" {
				seen[key] = true
			}
			out = append(out, artifact)
		}
	}
	return out
}

func dataTaskLatestError(records []dataTaskWorkflowRecord) string {
	for i := len(records) - 1; i >= 0; i-- {
		if errText := strings.TrimSpace(records[i].Err); errText != "" {
			return errText
		}
	}
	return ""
}

func dataTaskTerminalResultSummary(result *dataquery.Result, records []dataTaskWorkflowRecord) string {
	if result == nil {
		for i := len(records) - 1; i >= 0; i-- {
			if records[i].Result != nil {
				result = records[i].Result
				break
			}
		}
	}
	if result == nil {
		return "none"
	}
	reconcileStatus := ""
	if result.Reconcile != nil {
		reconcileStatus = result.Reconcile.Status.String()
	}
	return fmt.Sprintf("answer_len=%d decisions=%d rules=%d contributions=%d resolutions=%d consumed=%d warnings=%d reconcile=%q",
		len(result.Answer), len(result.Rows), len(result.RuleCoverage), len(result.Contributions),
		len(result.EntityResolutions), len(result.ConsumedPaths), len(result.ContractWarnings), reconcileStatus)
}

func (r *REPL) emitDataTaskErrorPreview(round int, errText string, artifact dataTaskAuditArtifact) {
	if r.renderer == nil {
		return
	}
	label := "data error"
	segs := []string{fmt.Sprintf("batch %d", round)}
	if isZh(r.language) {
		label = "数据错误"
		segs = []string{fmt.Sprintf("第 %d 批", round)}
	}
	if artifact.ErrorPath != "" {
		if isZh(r.language) {
			segs = append(segs, "完整错误 "+artifact.ErrorPath)
		} else {
			segs = append(segs, "full error "+artifact.ErrorPath)
		}
	}
	r.renderer.EmitLightRouteSummary(label, segs)
	var b strings.Builder
	if isZh(r.language) {
		b.WriteString("错误预览：\n")
	} else {
		b.WriteString("Error preview:\n")
	}
	b.WriteString(dataTaskPanelPreview(errText, r.language))
	if artifact.ErrorPath != "" {
		if isZh(r.language) {
			fmt.Fprintf(&b, "\n\n完整错误：`%s`", artifact.ErrorPath)
		} else {
			fmt.Fprintf(&b, "\n\nFull error: `%s`", artifact.ErrorPath)
		}
	}
	r.renderBorderedMutedCompact(b.String())
}

func (r *REPL) writeDataTaskResultArtifact(round int, result dataquery.Result) dataTaskAuditArtifact {
	dir := filepath.Join(firstNonEmptyString(r.runtimeAnchor, r.repoRoot, "."), "data-audit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warning("[repl/data] create audit dir failed: %v", err)
		return dataTaskAuditArtifact{}
	}
	stamp := dataTaskAuditStamp()
	resultPath := filepath.Join(dir, fmt.Sprintf("%s-%d-result-r%d.json", stamp, os.Getpid(), round))
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logging.Warning("[repl/data] marshal data task result failed round=%d: %v", round, err)
		return dataTaskAuditArtifact{}
	}
	if err := os.WriteFile(resultPath, raw, 0600); err != nil {
		logging.Warning("[repl/data] write data task result audit failed path=%s: %v", resultPath, err)
		return dataTaskAuditArtifact{}
	}
	artifact := dataTaskAuditArtifact{ResultPath: resultPath}
	if len(result.Artifacts) > 0 {
		graphPath := filepath.Join(dir, fmt.Sprintf("%s-%d-artifacts-r%d.json", stamp, os.Getpid(), round))
		graph := dataworkflow.BuildArtifactGraphState(result.Artifacts, 128)
		graphRaw, err := json.MarshalIndent(graph, "", "  ")
		if err != nil {
			logging.Warning("[repl/data] marshal data task artifact graph failed round=%d: %v", round, err)
		} else if err := os.WriteFile(graphPath, graphRaw, 0600); err != nil {
			logging.Warning("[repl/data] write data task artifact graph audit failed path=%s: %v", graphPath, err)
		} else {
			artifact.ArtifactGraphPath = graphPath
		}
	}
	return artifact
}

func (r *REPL) logDataTaskResultArtifact(round int, result dataquery.Result, artifact dataTaskAuditArtifact) {
	reconcileStatus := ""
	if result.Reconcile != nil {
		reconcileStatus = strings.TrimSpace(result.Reconcile.Status.String())
	}
	logging.Info("[repl/data] data task result round=%d answer_len=%d decisions=%d rules=%d contributions=%d resolutions=%d reconcile=%s consumed=%d result_path=%s artifact_graph_path=%s",
		round, len(result.Answer), len(result.Rows), len(result.RuleCoverage), len(result.Contributions),
		len(result.EntityResolutions), reconcileStatus, len(result.ConsumedPaths), artifact.ResultPath, artifact.ArtifactGraphPath)
	if raw, err := json.MarshalIndent(result, "", "  "); err == nil {
		logging.Info("[repl/data] data task result full round=%d path=%s\n%s", round, artifact.ResultPath, string(raw))
	}
	if len(result.Artifacts) > 0 {
		graph := dataworkflow.BuildArtifactGraphState(result.Artifacts, 128)
		if raw, err := json.MarshalIndent(graph, "", "  "); err == nil {
			logging.Info("[repl/data] data task artifact graph full round=%d path=%s\n%s", round, artifact.ArtifactGraphPath, string(raw))
		}
	}
}

func (r *REPL) emitDataTaskResultPreview(round int, result dataquery.Result, artifact dataTaskAuditArtifact) {
	if r.renderer == nil {
		return
	}
	label := "data result"
	segs := []string{fmt.Sprintf("batch %d", round)}
	if isZh(r.language) {
		label = "数据结果"
		segs = []string{fmt.Sprintf("第 %d 批", round)}
	}
	if len(result.Rows) > 0 {
		if isZh(r.language) {
			segs = append(segs, fmt.Sprintf("决策记录 %d", len(result.Rows)))
		} else {
			segs = append(segs, fmt.Sprintf("%d decision records", len(result.Rows)))
		}
	}
	if len(result.RuleCoverage) > 0 {
		if isZh(r.language) {
			segs = append(segs, fmt.Sprintf("规则覆盖 %d", len(result.RuleCoverage)))
		} else {
			segs = append(segs, fmt.Sprintf("%d rule records", len(result.RuleCoverage)))
		}
	}
	if len(result.Contributions) > 0 {
		if isZh(r.language) {
			segs = append(segs, fmt.Sprintf("贡献记录 %d", len(result.Contributions)))
		} else {
			segs = append(segs, fmt.Sprintf("%d contributions", len(result.Contributions)))
		}
	}
	if len(result.EntityResolutions) > 0 {
		if isZh(r.language) {
			segs = append(segs, fmt.Sprintf("实体归一 %d", len(result.EntityResolutions)))
		} else {
			segs = append(segs, fmt.Sprintf("%d resolutions", len(result.EntityResolutions)))
		}
	}
	if result.Reconcile != nil && strings.TrimSpace(result.Reconcile.Status.String()) != "" {
		if isZh(r.language) {
			segs = append(segs, "对账 "+strings.TrimSpace(result.Reconcile.Status.String()))
		} else {
			segs = append(segs, "reconcile "+strings.TrimSpace(result.Reconcile.Status.String()))
		}
	}
	if artifact.ResultPath != "" {
		if isZh(r.language) {
			segs = append(segs, "完整结果 "+artifact.ResultPath)
		} else {
			segs = append(segs, "full result "+artifact.ResultPath)
		}
	}
	r.renderer.EmitLightRouteSummary(label, segs)
	if strings.TrimSpace(result.Answer) == "" && artifact.ResultPath == "" {
		return
	}
	var b strings.Builder
	if isZh(r.language) {
		b.WriteString("结果预览：\n")
	} else {
		b.WriteString("Result preview:\n")
	}
	b.WriteString(dataTaskPanelPreview(result.Answer, r.language))
	if artifact.ResultPath != "" {
		if isZh(r.language) {
			fmt.Fprintf(&b, "\n\n完整结果：`%s`", artifact.ResultPath)
		} else {
			fmt.Fprintf(&b, "\n\nFull result: `%s`", artifact.ResultPath)
		}
	}
	if artifact.ArtifactGraphPath != "" {
		if isZh(r.language) {
			fmt.Fprintf(&b, "\n完整产物图：`%s`", artifact.ArtifactGraphPath)
		} else {
			fmt.Fprintf(&b, "\nFull artifact graph: `%s`", artifact.ArtifactGraphPath)
		}
	}
	r.renderBorderedMutedCompact(b.String())
}

func dataTaskAuditNamePart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "plan"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "plan"
	}
	return out
}

func dataTaskAuditScopeLabel(scope string, round int, lang string) string {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if isZh(lang) {
		switch scope {
		case "initial":
			return "初始计划"
		case "repair":
			return fmt.Sprintf("修复第 %d 次", round)
		case "continue":
			return fmt.Sprintf("继续第 %d 批", round)
		default:
			if round > 0 {
				return fmt.Sprintf("%s %d", scope, round)
			}
			return scope
		}
	}
	switch scope {
	case "initial":
		return "initial plan"
	case "repair":
		return fmt.Sprintf("repair %d", round)
	case "continue":
		return fmt.Sprintf("continue batch %d", round)
	default:
		if round > 0 {
			return fmt.Sprintf("%s %d", scope, round)
		}
		return scope
	}
}

func dataTaskPanelPreview(value, lang string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const maxRunes = 1200
	const maxLines = 18
	lines := strings.Split(value, "\n")
	truncatedByLine := false
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncatedByLine = true
	}
	preview := strings.Join(lines, "\n")
	runes := []rune(preview)
	truncatedByRunes := false
	if len(runes) > maxRunes {
		preview = string(runes[:maxRunes])
		truncatedByRunes = true
	}
	if !truncatedByLine && !truncatedByRunes {
		return preview
	}
	omitted := len([]rune(value)) - len([]rune(preview))
	if omitted < 0 {
		omitted = 0
	}
	if isZh(lang) {
		return fmt.Sprintf("%s\n...[面板预览已截断 %d 字符；完整内容见上方路径]", preview, omitted)
	}
	return fmt.Sprintf("%s\n...[panel preview truncated %d chars; see path above for full content]", preview, omitted)
}

func renderDataTaskActionsPreview(actions []dataquery.DataAction) string {
	if len(actions) == 0 {
		return ""
	}
	var b strings.Builder
	for i, action := range actions {
		fmt.Fprintf(&b, "%d. %s", i+1, strings.TrimSpace(string(action.Kind)))
		if id := strings.TrimSpace(action.ID); id != "" {
			fmt.Fprintf(&b, " `%s`", id)
		}
		if purpose := strings.TrimSpace(action.Purpose); purpose != "" {
			fmt.Fprintf(&b, " — %s", purpose)
		}
		if len(action.InputPaths) > 0 {
			fmt.Fprintf(&b, "\n   inputs: %s", strings.Join(action.InputPaths, ", "))
		}
		if artifact := strings.TrimSpace(action.OutputArtifact); artifact != "" {
			fmt.Fprintf(&b, "\n   output: %s", artifact)
		}
		if lines := dataTaskScriptLineCount(action.Script); lines > 0 {
			fmt.Fprintf(&b, "\n   script: %d line(s)", lines)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (r *REPL) dataTaskMutedPreview(value string) string {
	preview := dataTaskPanelPreview(value, r.language)
	if strings.TrimSpace(preview) == "" {
		return preview
	}
	if !r.replColorEnabled() {
		return preview
	}
	return pterm.NewStyle(pterm.FgWhite, pterm.Fuzzy).Sprint(preview)
}

func (r *REPL) emitDataTaskRunnerCall(plan dataquery.TaskPlan, round int) {
	if r.renderer == nil {
		return
	}
	round--
	if round < 0 {
		round = 0
	}
	r.renderer.Emitter()(render.Event{
		Kind:          render.EventAgentToolCallBatch,
		Timestamp:     time.Now(),
		Agent:         types.AgentName("data_planner"),
		Stage:         types.PipelineStage("data"),
		Iteration:     round,
		ToolName:      "data_runner",
		ToolNames:     []string{"data_runner"},
		ToolCallCount: 1,
		ToolDetail:    dataTaskRunnerToolDetail(plan, r.language),
	})
}

func dataTaskRunnerToolDetail(plan dataquery.TaskPlan, lang string) string {
	parts := []string{}
	if len(plan.Actions) > 0 {
		if isZh(lang) {
			parts = append(parts, fmt.Sprintf("动作=%d", len(plan.Actions)))
		} else {
			parts = append(parts, fmt.Sprintf("actions=%d", len(plan.Actions)))
		}
	}
	if len(plan.InputPaths) > 0 {
		if isZh(lang) {
			parts = append(parts, fmt.Sprintf("输入=%d", len(plan.InputPaths)))
		} else {
			parts = append(parts, fmt.Sprintf("inputs=%d", len(plan.InputPaths)))
		}
	}
	if lines := dataTaskScriptLineCount(plan.Script); lines > 0 {
		if isZh(lang) {
			parts = append(parts, fmt.Sprintf("脚本=%d行", lines))
		} else {
			parts = append(parts, fmt.Sprintf("script=%d lines", lines))
		}
	}
	if n := len(plan.CoverageContract.RequiredPaths()); n > 0 {
		if isZh(lang) {
			parts = append(parts, fmt.Sprintf("必需材料=%d", n))
		} else {
			parts = append(parts, fmt.Sprintf("required=%d", n))
		}
	}
	if plan.CoverageContract.DecisionRecordsRequired {
		if isZh(lang) {
			parts = append(parts, "需决策记录")
		} else {
			parts = append(parts, "decision records required")
		}
	}
	parts = append(parts, dataTaskValidationFlagSegments(plan.CoverageContract, lang)...)
	return strings.Join(parts, " ")
}

func dataTaskEvaluationRepairReason(eval dataquery.Evaluation) string {
	return dataworkflow.EvaluationRepairReason(eval)
}

func (r *REPL) emitDataTaskWorkflowEvent(event dataworkflow.WorkflowJournalEvent, opts dataTaskWorkflowEventRenderOptions) {
	if r.renderer == nil {
		return
	}
	display := dataworkflow.BuildWorkflowProcessDisplay(event, r.language)
	r.renderer.EmitLightRouteSummary(display.Label, display.Segments)
	if lines := dataTaskWorkflowEventDetailLines(event, r.language, opts); len(lines) > 0 {
		r.renderBorderedDataProcessCompact(strings.Join(lines, "\n"))
	}
}

func (r *REPL) emitDataTaskWorkflowReason(kind string, round int, reason string) {
	r.emitDataTaskWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind:   strings.TrimSpace(kind),
		Round:  round,
		Reason: reason,
	}), dataTaskWorkflowEventRenderOptions{})
}

func (r *REPL) emitDataTaskWorkflowFailure(kind string, round int, reason string) {
	r.emitDataTaskWorkflowEvent(dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind:   strings.TrimSpace(kind),
		Round:  round,
		Status: "failed",
		Reason: reason,
	}), dataTaskWorkflowEventRenderOptions{IncludeFailure: true})
}

type dataTaskWorkflowEventRenderOptions struct {
	IncludeGoal    bool
	IncludeBatch   bool
	IncludeNext    bool
	IncludeActions bool
	IncludeFailure bool
	IncludeAudit   bool
}

func dataTaskWorkflowEventDetailLines(event dataworkflow.WorkflowJournalEvent, lang string, opts dataTaskWorkflowEventRenderOptions) []string {
	zh := isZh(lang)
	var lines []string
	add := func(prefix, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		lines = append(lines, prefix+oneLineClamp(value, 180))
	}
	display := dataworkflow.BuildWorkflowProcessDisplay(event, lang)
	var businessLines []string
	var decisionValues []string
	var failureValues []string
	var auditValues []string
	for _, detail := range display.Details {
		text := strings.TrimSpace(detail.Text)
		value := strings.TrimSpace(firstNonEmptyString(detail.Value, detail.Text))
		if text == "" && value == "" {
			continue
		}
		switch strings.TrimSpace(detail.Class) {
		case "business":
			switch strings.TrimSpace(detail.Key) {
			case "goal":
				if opts.IncludeGoal {
					businessLines = append(businessLines, text)
				}
			case "batch":
				if opts.IncludeBatch {
					businessLines = append(businessLines, text)
				}
			case "next":
				if opts.IncludeNext {
					businessLines = append(businessLines, text)
				}
			case "actions":
				if opts.IncludeActions {
					businessLines = append(businessLines, text)
				}
			case "default":
				businessLines = append(businessLines, text)
			}
		case "decision":
			decisionValues = append(decisionValues, value)
		case "failure":
			if opts.IncludeFailure {
				failureValues = append(failureValues, value)
			}
		case "audit":
			if opts.IncludeAudit {
				auditValues = append(auditValues, value)
			}
		}
	}
	if len(businessLines) > 0 && dataTaskWorkflowShouldShowBusinessDetails(event.Kind, auditValues) {
		lines = append(lines, businessLines...)
	} else {
		defaultDisplay := dataworkflow.BuildWorkflowProcessDisplay(dataworkflow.WorkflowJournalEvent{Kind: strings.TrimSpace(event.Kind), Round: event.Round}, lang)
		for _, detail := range defaultDisplay.Details {
			if text := strings.TrimSpace(detail.Text); text != "" {
				lines = append(lines, oneLineClamp(text, 180))
			}
		}
	}
	for _, detail := range decisionValues {
		if zh {
			add("判断：", detail)
		} else {
			add("Decision: ", detail)
		}
	}
	for _, detail := range failureValues {
		if zh {
			add("原因：", detail)
		} else {
			add("Reason: ", detail)
		}
	}
	for _, detail := range auditValues {
		if zh {
			add("审计：", detail)
		} else {
			add("Audit: ", detail)
		}
	}
	return lines
}

func dataTaskWorkflowShouldShowBusinessDetails(kind string, auditDetails []string) bool {
	switch strings.TrimSpace(kind) {
	case "result", "evaluate":
		return false
	default:
		return true
	}
}

func (r *REPL) handleTerminalDataTaskPlan(plan dataquery.TaskPlan, records []dataTaskWorkflowRecord, display, line string, dataRounds, repairRounds int) bool {
	switch strings.ToLower(strings.TrimSpace(plan.Status)) {
	case "needs_clarification":
		msg := dataTaskClarificationMarkdown(r.language, plan)
		r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "needs_clarification", Reason: firstNonEmptyString(plan.BlockReason, strings.Join(plan.MissingObservations, "; ")), DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
		r.finishDataTaskRouteSpinner("needs_clarification")
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginLocal
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return true
	case "blocked":
		msg := dataTaskBlockedMarkdown(r.language, plan)
		r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "blocked", Reason: plan.BlockReason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
		r.finishDataTaskRouteSpinner("blocked")
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginLocal
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return true
	case "complete":
		if result, ok := latestDataTaskResult(records); ok {
			r.finishDataTaskWithFinalResult("complete", firstNonEmptyString(plan.BlockReason, "terminal data plan completed with latest result"), records, plan, result, display, line, dataRounds, repairRounds)
			return true
		}
		reason := firstNonEmptyString(plan.BlockReason, "data workflow completed without a computed result")
		msg := dataTaskBlockedMarkdown(r.language, dataquery.TaskPlan{BlockReason: reason})
		r.logDataTaskTerminal(dataTaskTerminalAudit{Status: "blocked", Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
		r.finishDataTaskRouteSpinner("blocked")
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginLocal
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return true
	case "budget_exhausted", "partial_answer_possible":
		if result, ok := latestDataTaskResult(records); ok {
			r.finishDataTaskWithFinalResult(plan.Status, firstNonEmptyString(plan.BlockReason, "terminal data plan ended with latest result"), records, plan, result, display, line, dataRounds, repairRounds)
			return true
		}
		reason := firstNonEmptyString(plan.BlockReason, "data workflow ended before producing a computed result")
		msg := dataTaskErrorMarkdown(r.language, reason)
		r.logDataTaskTerminal(dataTaskTerminalAudit{Status: plan.Status, Reason: reason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records})
		r.finishDataTaskRouteSpinner(plan.Status)
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginLocal
		r.recordTurn(display, line, msg, memory.KindPipeline)
		return true
	default:
		return false
	}
}

func (r *REPL) finishDataTaskWithFinalResult(status, reason string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, display, line string, dataRounds, repairRounds int) {
	msg, err := finalDataTaskAnswerForCLI(r.repoRoot, records, current, result, r.language)
	finalStatus := firstNonEmptyString(strings.TrimSpace(status), "complete")
	finalReason := strings.TrimSpace(reason)
	if err != nil {
		finalStatus = "failed"
		finalReason = err.Error()
		msg = dataTaskErrorMarkdown(r.language, finalReason)
	}
	r.logDataTaskTerminal(dataTaskTerminalAudit{Status: finalStatus, Reason: finalReason, DataRounds: dataRounds, RepairRounds: repairRounds, Records: records, Result: &result})
	r.finishDataTaskRouteSpinner(finalStatus)
	r.renderBordered(msg)
	r.lastAnswerOrigin = replAnswerOriginLocal
	r.recordTurn(display, line, msg, memory.KindPipeline)
}

func dataTaskPlanAuditSummary(plan dataquery.TaskPlan, lang string) (string, []string) {
	status := dataTaskStatusDisplay(plan.Status, lang)
	format := dataTaskOutputFormatDisplay(plan.OutputContract.Normalize().Format, lang)
	if format == "" {
		format = "freeform"
	}
	if isZh(lang) {
		segs := []string{status, fmt.Sprintf("输入 %d", len(plan.InputPaths)), "输出 " + format}
		if len(plan.Actions) > 0 {
			segs = append(segs, fmt.Sprintf("动作 %d", len(plan.Actions)))
		}
		if lines := dataTaskScriptLineCount(plan.Script); lines > 0 {
			segs = append(segs, fmt.Sprintf("脚本 %d 行", lines))
		}
		if n := len(plan.CoverageContract.RequiredPaths()); n > 0 {
			segs = append(segs, fmt.Sprintf("工作流必需 %d", n))
		}
		if plan.CoverageContract.DecisionRecordsRequired {
			segs = append(segs, "需决策记录")
		}
		segs = append(segs, dataTaskValidationFlagSegments(plan.CoverageContract, lang)...)
		if !plan.OutputContract.ExplanationAllowed {
			segs = append(segs, "纯输出")
		}
		return "数据计划", segs
	}
	segs := []string{status, fmt.Sprintf("%d input(s)", len(plan.InputPaths)), "output " + format}
	if len(plan.Actions) > 0 {
		segs = append(segs, fmt.Sprintf("%d action(s)", len(plan.Actions)))
	}
	if lines := dataTaskScriptLineCount(plan.Script); lines > 0 {
		segs = append(segs, fmt.Sprintf("%d script line(s)", lines))
	}
	if n := len(plan.CoverageContract.RequiredPaths()); n > 0 {
		segs = append(segs, fmt.Sprintf("%d workflow-required material(s)", n))
	}
	if plan.CoverageContract.DecisionRecordsRequired {
		segs = append(segs, "decision records required")
	}
	segs = append(segs, dataTaskValidationFlagSegments(plan.CoverageContract, lang)...)
	if !plan.OutputContract.ExplanationAllowed {
		segs = append(segs, "output-only")
	}
	return "data plan", segs
}

func dataTaskPlanAuditDetailLines(plan dataquery.TaskPlan, lang string) []string {
	event := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind: "plan",
		Plan: plan,
	})
	return dataTaskWorkflowEventDetailLines(event, lang, dataTaskWorkflowEventRenderOptions{IncludeGoal: true, IncludeBatch: true, IncludeNext: true, IncludeActions: true})
}

func dataTaskValidationFlagSegments(contract dataquery.CoverageContract, lang string) []string {
	var segs []string
	if isZh(lang) {
		if contract.RuleCoverageRequired {
			segs = append(segs, "需规则覆盖")
		}
		if contract.ContributionLedgerRequired {
			segs = append(segs, "需贡献记录")
		}
		if contract.EntityResolutionRequired {
			segs = append(segs, "需实体归一")
		}
		if contract.ReconcileRequired {
			segs = append(segs, "需对账")
		}
		return segs
	}
	if contract.RuleCoverageRequired {
		segs = append(segs, "rule coverage required")
	}
	if contract.ContributionLedgerRequired {
		segs = append(segs, "contributions required")
	}
	if contract.EntityResolutionRequired {
		segs = append(segs, "entity resolution required")
	}
	if contract.ReconcileRequired {
		segs = append(segs, "reconcile required")
	}
	return segs
}

func dataTaskScriptLineCount(script string) int {
	script = strings.TrimSpace(script)
	if script == "" {
		return 0
	}
	return len(strings.Split(script, "\n"))
}

func dataTaskStatusDisplay(status, lang string) string {
	raw := strings.TrimSpace(status)
	if raw == "" {
		raw = "ready"
	}
	if !isZh(lang) {
		return raw
	}
	switch strings.ToLower(raw) {
	case "ready":
		return "就绪"
	case "needs_clarification":
		return "需补充信息"
	case "blocked":
		return "已阻止"
	case "complete", "completed":
		return "已完成"
	case "failed":
		return "已失败"
	case "budget_exhausted":
		return "预算用尽"
	case "partial_answer_possible":
		return "部分结果"
	default:
		return raw
	}
}

func dataTaskOutputFormatDisplay(format dataquery.OutputFormat, lang string) string {
	raw := strings.TrimSpace(string(format))
	if raw == "" {
		raw = string(dataquery.OutputFreeform)
	}
	if !isZh(lang) {
		return raw
	}
	switch format {
	case dataquery.OutputPlainSingleLine:
		return "单行文本"
	case dataquery.OutputCSVLine:
		return "CSV 行"
	case dataquery.OutputJSONOnly:
		return "JSON"
	case dataquery.OutputMarkdownTable:
		return "Markdown 表格"
	case dataquery.OutputMarkdown:
		return "Markdown"
	case dataquery.OutputFilePath:
		return "文件路径"
	case dataquery.OutputFreeform:
		return "自由文本"
	default:
		return raw
	}
}

func (r *REPL) finishCommandOperationPlanHeader(plan operation.CommandOperationPlan) {
	if r.renderer == nil {
		return
	}
	if plan.Status == operation.StatusReady && plan.ApprovalMode == operation.ApprovalAutoLowRisk {
		r.finishOperationAutoStartSpinner()
		return
	}
	r.finishOperationRouteSpinner(plan.Status)
}

func (r *REPL) finishOperationFinalAnswerHeader() {
	if r.renderer == nil {
		return
	}
	label, segs := operationFinalAnswerSummary(r.language)
	r.emitOperationLightRouteSummary(label, segs)
}

func (r *REPL) finishProviderOperationPlanHeader(plan operation.Plan) {
	if r.renderer == nil {
		return
	}
	label, segs := providerOperationPlanSummary(r.language, plan)
	r.emitOperationLightRouteSummary(label, segs)
}

func (r *REPL) emitOperationLightRouteSummary(label string, segs []string) {
	if r.renderer == nil {
		return
	}
	r.renderer.SetTotalStages(0)
	r.renderer.SetRouteSummary(label, segs)
	if r.renderer.SpinnerActive() {
		r.renderer.StopSpinner()
		return
	}
	r.renderer.EmitLightRouteSummary(label, segs)
}

func (r *REPL) finishOperationAutoStartSpinner() {
	if r.renderer == nil {
		return
	}
	label, segs := operationAutoStartSummary(r.language)
	r.emitOperationLightRouteSummary(label, segs)
}

func operationAutoStartSummary(lang string) (string, []string) {
	if isZh(lang) {
		return "操作计划", []string{"自动执行", "未读仓库"}
	}
	return "operation plan", []string{"auto-run", "no repo read"}
}

func providerOperationPlanSummary(lang string, plan operation.Plan) (string, []string) {
	if isZh(lang) {
		switch {
		case plan.CanExecute:
			return "操作计划", []string{"自动执行", "未读仓库"}
		case providerOperationPlanAwaitingApproval(plan):
			return "操作计划", []string{"等待批准", "未读仓库"}
		case strings.TrimSpace(plan.MissingCapability) != "":
			return "操作计划", []string{"能力缺失", "未执行", "未读仓库"}
		default:
			return "操作计划", []string{"未执行", "未读仓库"}
		}
	}
	switch {
	case plan.CanExecute:
		return "operation plan", []string{"auto-run", "no repo read"}
	case providerOperationPlanAwaitingApproval(plan):
		return "operation plan", []string{"awaiting approval", "no repo read"}
	case strings.TrimSpace(plan.MissingCapability) != "":
		return "operation plan", []string{"capability missing", "not executed", "no repo read"}
	default:
		return "operation plan", []string{"not executed", "no repo read"}
	}
}

func providerOperationPlanAwaitingApproval(plan operation.Plan) bool {
	return strings.TrimSpace(plan.Provider) != "" &&
		strings.TrimSpace(plan.ProviderTool) != "" &&
		!plan.CanExecute &&
		plan.RequiresConfirmation
}

func (r *REPL) startOperationExecutionSpinner() {
	if r.renderer == nil {
		return
	}
	label, segs := operationRunningSummary(r.language)
	r.renderer.SetTotalStages(0)
	r.renderer.SetRouteSummary(label, segs)
	if r.renderer.SpinnerActive() {
		r.renderer.SetLightRouteActivity(label)
		return
	}
	r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	r.renderer.SetLightRouteActivity(label)
}

func (r *REPL) startOperationSynthesisSpinner() {
	if r.renderer == nil {
		return
	}
	label, segs := operationSynthesisSummary(r.language)
	r.renderer.SetTotalStages(0)
	r.renderer.SetRouteSummary(label, segs)
	if r.renderer.SpinnerActive() {
		r.renderer.SetLightRouteActivity(label)
		return
	}
	r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	r.renderer.SetLightRouteActivity(label)
}

func (r *REPL) renderOperationProgress(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	logging.Info("[repl/operation] progress: %s", oneLineClamp(msg, 180))
	if r.renderer != nil && r.renderer.SpinnerActive() {
		r.renderer.SetLightRouteActivity(msg)
	}
}

func (r *REPL) renderOperationStepProgress(step operation.CommandStep) {
	msg := strings.TrimSpace(commandOperationProgressMsg(r.language, step))
	if msg == "" {
		return
	}
	logging.Info("[repl/operation] progress: %s", oneLineClamp(msg, 180))
	if r.renderer == nil || !r.renderer.SpinnerActive() {
		return
	}
	timeout := time.Duration(step.TimeoutMS) * time.Millisecond
	if timeout <= 0 && r.operationPolicy.TimeoutMS > 0 {
		timeout = time.Duration(r.operationPolicy.TimeoutMS) * time.Millisecond
	}
	if timeout > 0 {
		r.renderer.SetLightRouteActivityDeadline(msg, time.Now().Add(timeout), timeout)
		return
	}
	r.renderer.SetLightRouteActivity(msg)
}

func operationRunningSummary(lang string) (string, []string) {
	if isZh(lang) {
		return "操作执行", []string{"运行中", "未读仓库"}
	}
	return "operation execution", []string{"running", "no repo read"}
}

func operationSynthesisSummary(lang string) (string, []string) {
	if isZh(lang) {
		return "整理结果", []string{"未读仓库"}
	}
	return "summarizing result", []string{"no repo read"}
}

func operationFinalAnswerSummary(lang string) (string, []string) {
	if isZh(lang) {
		return "最终答案", []string{"已生成", "未读仓库"}
	}
	return "final answer", []string{"ready", "no repo read"}
}

func operationRouteSummary(lang string, status operation.OperationStatus) (string, []string) {
	if isZh(lang) {
		switch status {
		case operation.StatusNeedsClarification:
			return "操作计划", []string{"需要补充信息", "未读仓库"}
		case operation.StatusReady:
			return "操作计划", []string{"等待批准", "未读仓库"}
		case operation.StatusBlocked:
			return "操作计划", []string{"策略阻止", "未读仓库"}
		case operation.StatusComplete:
			return "操作执行", []string{"已完成", "未读仓库"}
		case operation.StatusBudgetExhausted:
			return "操作执行", []string{"预算耗尽", "未读仓库"}
		case operation.StatusPartialAnswer:
			return "操作执行", []string{"部分完成", "未读仓库"}
		case operation.StatusExecuted:
			return "操作执行", []string{"已完成", "未读仓库"}
		case operation.StatusFailed:
			return "操作执行", []string{"已失败", "未读仓库"}
		case operation.StatusCancelled:
			return "操作执行", []string{"已取消", "未读仓库"}
		}
		return "操作计划", []string{"未读仓库"}
	}
	switch status {
	case operation.StatusNeedsClarification:
		return "operation plan", []string{"needs clarification", "no repo read"}
	case operation.StatusReady:
		return "operation plan", []string{"awaiting approval", "no repo read"}
	case operation.StatusBlocked:
		return "operation plan", []string{"blocked by policy", "no repo read"}
	case operation.StatusComplete:
		return "operation execution", []string{"completed", "no repo read"}
	case operation.StatusBudgetExhausted:
		return "operation execution", []string{"budget exhausted", "no repo read"}
	case operation.StatusPartialAnswer:
		return "operation execution", []string{"partial", "no repo read"}
	case operation.StatusExecuted:
		return "operation execution", []string{"completed", "no repo read"}
	case operation.StatusFailed:
		return "operation execution", []string{"failed", "no repo read"}
	case operation.StatusCancelled:
		return "operation execution", []string{"cancelled", "no repo read"}
	}
	return "operation plan", []string{"no repo read"}
}

func (r *REPL) commandOperationCapabilitySnapshot() operation.CapabilitySnapshot {
	facts := r.envFacts
	if facts == nil {
		// Operation planning needs local capability signal, but it must stay
		// bounded and side-effect-free. Avoid the network probe here; the
		// planner only needs local tool availability to choose command paths.
		facts = env.Probe(env.ProbeOptions{RepoRoot: r.repoRoot, ProbeNetwork: false})
		r.envFacts = facts
	}
	return operation.BuildCapabilitySnapshotWithProviders(facts, r.repoRoot, r.operationPolicy, r.operationProviders)
}

func isZeroOperationCommandPolicy(policy operation.CommandPolicy) bool {
	return strings.TrimSpace(policy.Approval) == "" &&
		!policy.AutoApprove &&
		!policy.AutoLowRisk &&
		strings.TrimSpace(policy.UnknownProgram) == "" &&
		strings.TrimSpace(policy.ShellPolicy) == "" &&
		strings.TrimSpace(policy.NetworkPolicy) == "" &&
		strings.TrimSpace(policy.InstallPolicy) == "" &&
		strings.TrimSpace(policy.OverwritePolicy) == "" &&
		len(policy.AllowedWriteRoots) == 0 &&
		policy.TimeoutMS == 0 &&
		policy.OutputPreviewBytes == 0 &&
		strings.TrimSpace(policy.DefaultWorkDir) == "" &&
		!policy.HardDenyDestructive &&
		!policy.AllowMkdirPAutoCreate
}

func isCommandOperationPolicy(policy TurnPolicy) bool {
	kind := strings.TrimSpace(policy.OperationKind)
	if kind == "" {
		kind = strings.TrimSpace(policy.Operation)
	}
	return kind == "computer_operation"
}

func fallbackCommandOperationClarification(line, workDir string) operation.CommandOperationRequest {
	return operation.CommandOperationRequest{
		Text:    line,
		WorkDir: workDir,
		ClarifyingQuestions: []operation.ClarifyingQuestion{{
			ID:       "command_or_target",
			Question: "请补充要执行的命令、目标路径或约束条件。",
			Suggestions: []string{
				"例如: 查询 node 版本",
				"例如: 把 ./a.log 移动到 ./logs/",
				"例如: 在当前目录创建 reports 目录",
			},
		}},
	}
}

func (r *REPL) handleOperationCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/operation"))
	switch rest {
	case "", "show":
		if r.pendingOperation != nil {
			r.renderBordered(commandOperationPlanMarkdown(r.language, *r.pendingOperation))
			return
		}
		if r.pendingCommandClarification != nil {
			r.renderBordered(commandOperationPlanMarkdown(r.language, r.pendingCommandClarification.Plan))
			return
		}
		if r.providerWorkflow != nil {
			r.renderBordered(providerWorkflowMarkdown(r.language, *r.providerWorkflow))
			return
		}
		if r.pendingProviderOperation != nil {
			r.renderBordered(operationPlanMarkdown(r.language, r.pendingProviderOperation.Plan))
			return
		}
		r.info(operationNoPendingMsg(r.language))
	case "history":
		if len(r.operationHistory) == 0 {
			r.info(operationNoHistoryMsg(r.language))
			return
		}
		limit := len(r.operationHistory)
		if limit > 5 {
			limit = 5
		}
		for i := len(r.operationHistory) - limit; i < len(r.operationHistory); i++ {
			plan := r.operationHistory[i]
			r.info(operationHistoryRow(r.language, plan.ID, string(plan.Status), plan.RiskLevel))
		}
	case "auto":
		r.info(operationAutoStatusMsg(r.language))
	default:
		r.info(operationHelpMsg(r.language))
	}
}

func (r *REPL) restorePendingOperationState() {
	if r.operationPendingStore == nil {
		return
	}
	snap, ok, err := r.operationPendingStore.Load()
	if err != nil {
		logging.Warning("[repl/operation] restore pending operation failed: %v", err)
		return
	}
	if !ok {
		return
	}
	switch snap.Kind {
	case operationPendingKindCommandPlan:
		r.pendingOperation = snap.CommandPlan
	case operationPendingKindCommandClarification:
		r.pendingCommandClarification = snap.CommandClarification
	case operationPendingKindProviderOperation:
		r.pendingProviderOperation = snap.ProviderOperation
		r.providerWorkflow = snap.ProviderWorkflow
	default:
		logging.Warning("[repl/operation] ignoring unknown pending operation kind=%q", snap.Kind)
		return
	}
	logging.Info("[repl/operation] restored pending operation kind=%s store=%s", snap.Kind, r.operationPendingStore.Path())
}

func (r *REPL) savePendingOperationState() {
	if r.operationPendingStore == nil {
		return
	}
	var err error
	switch {
	case r.pendingOperation != nil:
		err = r.operationPendingStore.SaveCommandPlan(*r.pendingOperation)
	case r.pendingCommandClarification != nil:
		err = r.operationPendingStore.SaveCommandClarification(*r.pendingCommandClarification)
	case r.pendingProviderOperation != nil:
		err = r.operationPendingStore.SaveProviderOperation(*r.pendingProviderOperation, r.providerWorkflow)
	default:
		err = r.operationPendingStore.Clear()
	}
	if err != nil {
		logging.Warning("[repl/operation] save pending operation failed: %v", err)
	}
}

func (r *REPL) clearPendingOperationState() {
	if r.operationPendingStore == nil {
		return
	}
	if err := r.operationPendingStore.Clear(); err != nil {
		logging.Warning("[repl/operation] clear pending operation failed: %v", err)
	}
}

func (r *REPL) pendingOperationBannerLine() string {
	switch {
	case r.pendingOperation != nil:
		return pendingOperationBanner(r.language, r.pendingOperation.ID, string(r.pendingOperation.Status))
	case r.pendingCommandClarification != nil:
		return pendingOperationClarificationBanner(r.language, r.pendingCommandClarification.Plan.ID)
	case r.pendingProviderOperation != nil:
		id := firstNonEmptyString(r.pendingProviderOperation.Plan.Provider, r.pendingProviderOperation.Plan.Kind)
		if id == "" {
			id = "provider-operation"
		}
		if r.providerWorkflow != nil && strings.TrimSpace(r.providerWorkflow.ID) != "" {
			id = id + " / " + r.providerWorkflow.ID
		}
		return pendingOperationBanner(r.language, id, string(operation.StatusReady))
	default:
		return ""
	}
}

func (r *REPL) handleWorkflowCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/workflow"))
	switch {
	case rest == "" || rest == "show" || strings.HasPrefix(rest, "show "):
		if r.renderWriteWorkflowShow(rest) {
			return
		}
		if r.providerWorkflow == nil {
			r.info(workflowNoActiveMsg(r.language))
			return
		}
		r.renderBordered(providerWorkflowMarkdown(r.language, *r.providerWorkflow))
	case rest == "list":
		if r.handleWriteWorkflowList() {
			return
		}
		r.info(workflowHelpMsg(r.language))
	case rest == "resume" || strings.HasPrefix(rest, "resume "):
		if r.handleWriteWorkflowResume(rest) {
			return
		}
		r.info(workflowHelpMsg(r.language))
	case rest == "clear" || strings.HasPrefix(rest, "clear "):
		if r.handleWriteWorkflowClear(rest) {
			return
		}
		r.info(workflowHelpMsg(r.language))
	case rest == "cancel":
		if r.providerWorkflow == nil {
			r.info(workflowNoActiveMsg(r.language))
			return
		}
		id := r.providerWorkflow.ID
		r.providerWorkflow.Cancelled = true
		r.pendingProviderOperation = nil
		r.providerWorkflow = nil
		r.clearPendingOperationState()
		r.info(workflowCancelledMsg(r.language, id))
	default:
		r.info(workflowHelpMsg(r.language))
	}
}

func (r *REPL) handleOperationApproveCmd(line string) {
	_ = line
	plan := *r.pendingOperation
	if plan.Status != operation.StatusReady {
		r.info(commandOperationPlanMarkdown(r.language, plan))
		return
	}
	r.clearPendingOperationState()
	r.executeCommandOperationPlan(plan, "/approve", "/approve")
}

func (r *REPL) handleOperationRejectCmd(line string) {
	plan := *r.pendingOperation
	reason := strings.TrimSpace(strings.TrimPrefix(line, "/reject"))
	plan.Status = operation.StatusRejected
	r.operationHistory = append(r.operationHistory, plan)
	r.pendingOperation = nil
	r.clearPendingOperationState()
	msg := operationRejectedMsg(r.language, plan.ID, reason)
	r.success(msg)
	request := "/reject"
	if reason != "" {
		request += " " + reason
	}
	r.recordTurn(request, request, msg, memory.KindPipeline)
}

func (r *REPL) executeCommandOperationPlan(plan operation.CommandOperationPlan, request, display string) {
	r.executeCommandOperationPlanAttempt(plan, request, display, 0, nil)
}

func (r *REPL) executeCommandOperationPlanAttempt(plan operation.CommandOperationPlan, request, display string, replanAttempts int, records []commandOperationResultRecord) {
	r.pendingOperation = nil
	r.clearPendingOperationState()
	r.runInFlight.Store(true)
	ctx := r.startTurn()
	r.startOperationExecutionSpinner()
	defer func() {
		r.endTurn()
		r.runInFlight.Store(false)
	}()

	executor := operation.CommandExecutor{
		Policy:    r.operationPolicy,
		OutputDir: r.operationOutputDir(),
		OnStepStart: func(step operation.CommandStep) {
			logging.Info("[repl/operation] command step start plan_id=%s step_id=%s cmd=%q risk=%q side_effects=%q",
				plan.ID,
				step.ID,
				oneLineClamp(commandOperationStepCommand(step), 220),
				step.RiskLevel,
				strings.Join(step.SideEffects, ","))
			r.renderOperationStepProgress(step)
		},
	}
	logging.Info("[repl/operation] command execute start plan_id=%s steps=%d approval=%q risk=%q replan_attempt=%d request=%q",
		plan.ID, len(plan.Steps), plan.ApprovalMode, plan.RiskLevel, replanAttempts, oneLineClamp(plan.RequestText, 160))
	currentPlan := plan
	repairRounds := replanAttempts
	commandRounds := commandOperationExecutedRoundCount(records)
	var syntheticResult *operation.CommandOperationResult
	for {
		result := operation.CommandOperationResult{}
		if syntheticResult != nil {
			result = *syntheticResult
			syntheticResult = nil
			currentPlan.Status = result.Status
			r.operationHistory = append(r.operationHistory, currentPlan)
			r.appendCommandOperationResult(currentPlan, result)
			records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: result})
			logging.Info("[repl/operation] command synthetic result plan_id=%s status=%s failure_class=%q repair_rounds=%d command_rounds=%d",
				currentPlan.ID, result.Status, commandOperationPrimaryFailureClass(result), repairRounds, commandRounds)
		} else if lint := operation.LintCommandOperationPlan(currentPlan); !lint.OK() {
			result = commandOperationResultFromPlanLint(currentPlan, lint)
			currentPlan.Status = result.Status
			r.operationHistory = append(r.operationHistory, currentPlan)
			r.appendCommandOperationResult(currentPlan, result)
			records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: result})
			logging.Info("[repl/operation] command plan lint failed plan_id=%s issues=%d summary=%q repair_rounds=%d command_rounds=%d",
				currentPlan.ID, len(lint.Issues), oneLineClamp(lint.Summary(), 240), repairRounds, commandRounds)
		} else {
			if commandRounds >= commandOperationMaxCommandRounds {
				result = commandOperationBudgetResult(currentPlan, "command operation command-round budget exhausted before the user goal was fully satisfied")
				currentPlan.Status = result.Status
				r.operationHistory = append(r.operationHistory, currentPlan)
				r.appendCommandOperationResult(currentPlan, result)
				records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: result})
			} else {
				commandRounds++
				result = executor.Execute(ctx, currentPlan)
				currentPlan.Status = result.Status
				r.operationHistory = append(r.operationHistory, currentPlan)
				r.appendCommandOperationResult(currentPlan, result)
				records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: result})
				for _, stepResult := range result.StepResults {
					planStep := commandOperationStepByID(currentPlan, stepResult.StepID)
					logging.Info("[repl/operation] command step result plan_id=%s step_id=%s cmd=%q status=%s exit_code=%d timed_out=%t failure_class=%q verification=%s payload_ref=%s output_preview=%q error=%q",
						currentPlan.ID,
						stepResult.StepID,
						oneLineClamp(commandOperationStepCommand(planStep), 220),
						stepResult.Status,
						stepResult.ExitCode,
						stepResult.TimedOut,
						stepResult.FailureClass,
						stepResult.Verification.Status,
						stepResult.PayloadRef,
						oneLineClamp(stepResult.OutputPreview, 240),
						oneLineClamp(stepResult.Error, 240))
				}
			}
		}
		logging.Info("[repl/operation] command loop result plan_id=%s status=%s step_results=%d repair_rounds=%d command_rounds=%d",
			currentPlan.ID, result.Status, len(result.StepResults), repairRounds, commandRounds)
		r.renderCommandOperationRoundResult(currentPlan, result)

		if result.Status == operation.StatusFailed && !commandResultTimedOut(result) && repairRounds < commandOperationMaxRepairRounds && r.operationPlanner != nil {
			replanner, ok := r.operationPlanner.(CommandOperationReplanner)
			if ok {
				r.startOperationSynthesisSpinner()
				snapshot := r.commandOperationCapabilitySnapshot()
				revisedReq, err := replanner.ReplanCommandOperation(ctx, currentPlan.RequestText, r.repoRoot, commandOperationPolicyFromPlan(currentPlan), snapshot, currentPlan, result)
				r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
				if err != nil {
					logging.Warning("[repl/operation] command replan failed: %v", err)
					if degraded, ok := commandOperationStructuredToolParamFailureResult(currentPlan, err, r.language); ok {
						r.appendCommandOperationResult(currentPlan, degraded)
						records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: degraded})
						r.renderCommandOperationRoundResult(currentPlan, degraded)
						r.startOperationSynthesisSpinner()
						msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
						r.finishOperationFinalAnswerHeader()
						r.emitOperationVisibleThoughts(thoughts)
						r.renderBordered(msg)
						r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
						r.recordTurn(display, request, msg, memory.KindPipeline)
						return
					}
				} else {
					repairRounds++
					revisedReq = dropRepeatedFailedCommandSteps(revisedReq, currentPlan, result)
					revisedPlan := operation.BuildCommandOperationPlan(revisedReq, r.operationPolicy)
					replanDecision := operation.DecideCommandPlanApproval(r.operationPolicy, revisedPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalReplan, PreviousPlan: &currentPlan})
					revisedPlan = operation.ApplyCommandPlanApprovalDecision(revisedPlan, replanDecision)
					logging.Info("[repl/operation] command replan generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d repair_rounds=%d",
						currentPlan.ID, revisedPlan.Status, revisedPlan.RiskLevel, revisedPlan.ApprovalMode, replanDecision.Action, replanDecision.ReasonCode, len(revisedPlan.Steps), repairRounds)
					if commandOperationTerminalAfterReplan(revisedPlan, records) {
						terminal := commandOperationTerminalResult(revisedPlan)
						r.operationHistory = append(r.operationHistory, revisedPlan)
						r.appendCommandOperationResult(revisedPlan, terminal)
						records = append(records, commandOperationResultRecord{Plan: revisedPlan, Result: terminal})
						logging.Info("[repl/operation] command replan terminal previous_plan_id=%s terminal_plan_id=%s status=%s reason=%q records=%d",
							currentPlan.ID, revisedPlan.ID, revisedPlan.Status, oneLineClamp(revisedPlan.BlockReason, 180), len(records))
						r.renderCommandOperationRoundResult(revisedPlan, terminal)
						r.startOperationSynthesisSpinner()
						msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
						r.finishOperationFinalAnswerHeader()
						r.emitOperationVisibleThoughts(thoughts)
						r.renderBordered(msg)
						r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
						r.recordTurn(display, request, msg, memory.KindPipeline)
						return
					}
					if revisedPlan.Status == operation.StatusReady {
						if lint := commandRevisedPlanPreflightLint(revisedPlan, currentPlan, result); !lint.OK() {
							lintResult := commandOperationResultFromPlanLint(revisedPlan, lint)
							currentPlan = revisedPlan
							syntheticResult = &lintResult
							continue
						}
						if replanDecision.AutoExecute() {
							msg := commandOperationReplanIntro(r.language, revisedPlan)
							msg += "\n\n"
							msg += commandOperationAutoExecuteMarkdown(r.language, revisedPlan)
							r.finishCommandOperationPlanHeader(revisedPlan)
							r.renderBordered(msg)
							currentPlan = revisedPlan
							r.startOperationExecutionSpinner()
							continue
						}
						r.pendingOperation = &revisedPlan
						r.savePendingOperationState()
					}
					r.operationHistory = append(r.operationHistory, revisedPlan)
					msg := commandOperationReplanIntro(r.language, revisedPlan)
					msg += "\n\n"
					msg += commandOperationPlanMarkdown(r.language, revisedPlan)
					r.finishOperationRouteSpinner(revisedPlan.Status)
					r.renderBordered(msg)
					r.recordTurn(display, request, msg, memory.KindPipeline)
					return
				}
			}
		}
		if commandOperationRepairBudgetExhausted(result, repairRounds) {
			budget := commandOperationBudgetResult(currentPlan, "command operation repair budget exhausted before the user goal was fully satisfied")
			r.appendCommandOperationResult(currentPlan, budget)
			records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: budget})
			result = budget
			r.renderCommandOperationRoundResult(currentPlan, result)
		}
		if commandOperationContinuationBudgetExhausted(currentPlan, result, records) {
			budget := commandOperationBudgetResult(currentPlan, "command operation command-round budget exhausted before the user goal was fully satisfied")
			r.appendCommandOperationResult(currentPlan, budget)
			records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: budget})
			result = budget
			r.renderCommandOperationRoundResult(currentPlan, result)
		}
		if result.Status == operation.StatusExecuted &&
			commandOperationShouldRunMaterialEvaluator(records) &&
			commandRounds < commandOperationMaxCommandRounds &&
			r.operationPlanner != nil {
			if evaluator, ok := r.operationPlanner.(CommandOperationEvaluator); ok {
				r.startOperationSynthesisSpinner()
				eval, err := evaluator.EvaluateCommandOperation(ctx, currentPlan.RequestText, records, r.language)
				r.emitReplLLMTrace(r.operationPlanner, "command_operation_evaluator", types.AgentName("operation_planner"), types.PipelineStage("operation"))
				if err != nil {
					logging.Warning("[repl/operation] command operation evaluation failed: %v", err)
				} else {
					records = commandOperationAttachEvaluation(records, eval)
					logging.Info("[repl/operation] command evaluation status=%s confidence=%q reason=%q materials=%d rounds=%d",
						eval.Status, oneLineClamp(eval.Confidence, 40), oneLineClamp(eval.Reason, 180), len(eval.Materials), len(records))
					if terminal, ok := commandOperationEvaluationTerminalResult(currentPlan, eval); ok {
						r.appendCommandOperationResult(currentPlan, terminal)
						records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: terminal})
						r.renderCommandOperationRoundResult(currentPlan, terminal)
						r.startOperationSynthesisSpinner()
						msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
						r.finishOperationFinalAnswerHeader()
						r.emitOperationVisibleThoughts(thoughts)
						r.renderBordered(msg)
						r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
						r.recordTurn(display, request, msg, memory.KindPipeline)
						return
					}
					if eval.Status == operation.EvalContinueCommand {
						if continuer, ok := r.operationPlanner.(CommandOperationContinuationPlanner); ok {
							r.startOperationSynthesisSpinner()
							snapshot := r.commandOperationCapabilitySnapshot()
							next, err := continuer.ContinueCommandOperation(ctx, currentPlan.RequestText, r.repoRoot, commandOperationPolicyFromPlan(currentPlan), snapshot, records)
							r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
							if err != nil {
								logging.Warning("[repl/operation] command evaluation continuation planning failed: %v", err)
								if degraded, ok := commandOperationStructuredToolParamFailureResult(currentPlan, err, r.language); ok {
									r.appendCommandOperationResult(currentPlan, degraded)
									records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: degraded})
									r.renderCommandOperationRoundResult(currentPlan, degraded)
									r.startOperationSynthesisSpinner()
									msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
									r.finishOperationFinalAnswerHeader()
									r.emitOperationVisibleThoughts(thoughts)
									r.renderBordered(msg)
									r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
									r.recordTurn(display, request, msg, memory.KindPipeline)
									return
								}
							} else if !next.Complete {
								nextPlan := operation.BuildCommandOperationPlan(next.Request, r.operationPolicy)
								continuationDecision := operation.DecideCommandPlanApproval(r.operationPolicy, nextPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalContinuation, PreviousPlan: &currentPlan})
								nextPlan = operation.ApplyCommandPlanApprovalDecision(nextPlan, continuationDecision)
								logging.Info("[repl/operation] command evaluation continuation generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d rounds=%d",
									currentPlan.ID, nextPlan.Status, nextPlan.RiskLevel, nextPlan.ApprovalMode, continuationDecision.Action, continuationDecision.ReasonCode, len(nextPlan.Steps), len(records))
								if commandOperationTerminalAfterReplan(nextPlan, records) {
									terminal := commandOperationTerminalResult(nextPlan)
									r.operationHistory = append(r.operationHistory, nextPlan)
									r.appendCommandOperationResult(nextPlan, terminal)
									records = append(records, commandOperationResultRecord{Plan: nextPlan, Result: terminal})
									logging.Info("[repl/operation] command evaluation continuation terminal previous_plan_id=%s terminal_plan_id=%s status=%s reason=%q records=%d",
										currentPlan.ID, nextPlan.ID, nextPlan.Status, oneLineClamp(nextPlan.BlockReason, 180), len(records))
									r.renderCommandOperationRoundResult(nextPlan, terminal)
									r.startOperationSynthesisSpinner()
									msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
									r.finishOperationFinalAnswerHeader()
									r.emitOperationVisibleThoughts(thoughts)
									r.renderBordered(msg)
									r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
									r.recordTurn(display, request, msg, memory.KindPipeline)
									return
								}
								if lint := operation.LintCommandOperationPlan(nextPlan); !lint.OK() {
									lintResult := commandOperationResultFromPlanLint(nextPlan, lint)
									currentPlan = nextPlan
									syntheticResult = &lintResult
									continue
								}
								if nextPlan.Status == operation.StatusReady && continuationDecision.AutoExecute() {
									msg := commandOperationContinuationIntro(r.language, nextPlan)
									msg += "\n\n"
									msg += commandOperationAutoExecuteMarkdown(r.language, nextPlan)
									r.finishCommandOperationPlanHeader(nextPlan)
									r.renderBordered(msg)
									currentPlan = nextPlan
									r.startOperationExecutionSpinner()
									continue
								}
								if nextPlan.Status == operation.StatusReady {
									r.pendingOperation = &nextPlan
									r.savePendingOperationState()
								}
								r.operationHistory = append(r.operationHistory, nextPlan)
								msg := commandOperationContinuationIntro(r.language, nextPlan)
								msg += "\n\n"
								msg += commandOperationPlanMarkdown(r.language, nextPlan)
								r.finishOperationRouteSpinner(nextPlan.Status)
								r.renderBorderedCompact(msg)
								r.recordTurn(display, request, msg, memory.KindPipeline)
								return
							}
						}
					}
				}
			}
		}
		if result.Status == operation.StatusExecuted && currentPlan.ContinueAfter && r.operationPlanner != nil && commandRounds < commandOperationMaxCommandRounds {
			continuer, ok := r.operationPlanner.(CommandOperationContinuationPlanner)
			if ok {
				r.startOperationSynthesisSpinner()
				snapshot := r.commandOperationCapabilitySnapshot()
				next, err := continuer.ContinueCommandOperation(ctx, currentPlan.RequestText, r.repoRoot, commandOperationPolicyFromPlan(currentPlan), snapshot, records)
				r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
				if err != nil {
					logging.Warning("[repl/operation] command continuation planning failed: %v", err)
					if degraded, ok := commandOperationStructuredToolParamFailureResult(currentPlan, err, r.language); ok {
						r.appendCommandOperationResult(currentPlan, degraded)
						records = append(records, commandOperationResultRecord{Plan: currentPlan, Result: degraded})
						r.renderCommandOperationRoundResult(currentPlan, degraded)
						r.startOperationSynthesisSpinner()
						msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
						r.finishOperationFinalAnswerHeader()
						r.emitOperationVisibleThoughts(thoughts)
						r.renderBordered(msg)
						r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
						r.recordTurn(display, request, msg, memory.KindPipeline)
						return
					}
				} else if next.Complete {
					logging.Info("[repl/operation] command continuation complete plan_id=%s reason=%q rounds=%d",
						currentPlan.ID, oneLineClamp(next.Reason, 160), len(records))
				} else {
					nextPlan := operation.BuildCommandOperationPlan(next.Request, r.operationPolicy)
					continuationDecision := operation.DecideCommandPlanApproval(r.operationPolicy, nextPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalContinuation, PreviousPlan: &currentPlan})
					nextPlan = operation.ApplyCommandPlanApprovalDecision(nextPlan, continuationDecision)
					logging.Info("[repl/operation] command continuation generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d rounds=%d",
						currentPlan.ID, nextPlan.Status, nextPlan.RiskLevel, nextPlan.ApprovalMode, continuationDecision.Action, continuationDecision.ReasonCode, len(nextPlan.Steps), len(records))
					if commandOperationTerminalAfterReplan(nextPlan, records) {
						terminal := commandOperationTerminalResult(nextPlan)
						r.operationHistory = append(r.operationHistory, nextPlan)
						r.appendCommandOperationResult(nextPlan, terminal)
						records = append(records, commandOperationResultRecord{Plan: nextPlan, Result: terminal})
						logging.Info("[repl/operation] command continuation terminal previous_plan_id=%s terminal_plan_id=%s status=%s reason=%q records=%d",
							currentPlan.ID, nextPlan.ID, nextPlan.Status, oneLineClamp(nextPlan.BlockReason, 180), len(records))
						r.renderCommandOperationRoundResult(nextPlan, terminal)
						r.startOperationSynthesisSpinner()
						msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
						r.finishOperationFinalAnswerHeader()
						r.emitOperationVisibleThoughts(thoughts)
						r.renderBordered(msg)
						r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
						r.recordTurn(display, request, msg, memory.KindPipeline)
						return
					}
					if lint := operation.LintCommandOperationPlan(nextPlan); !lint.OK() {
						lintResult := commandOperationResultFromPlanLint(nextPlan, lint)
						currentPlan = nextPlan
						syntheticResult = &lintResult
						continue
					}
					if nextPlan.Status == operation.StatusNeedsClarification && len(nextPlan.ClarifyingQuestions) == 0 {
						logging.Warning("[repl/operation] command continuation returned needs_clarification without actionable questions; falling back to final synthesis")
					} else if nextPlan.Status == operation.StatusReady && continuationDecision.AutoExecute() {
						msg := commandOperationContinuationIntro(r.language, nextPlan)
						msg += "\n\n"
						msg += commandOperationAutoExecuteMarkdown(r.language, nextPlan)
						r.finishCommandOperationPlanHeader(nextPlan)
						r.renderBordered(msg)
						currentPlan = nextPlan
						r.startOperationExecutionSpinner()
						continue
					} else {
						if nextPlan.Status == operation.StatusReady {
							r.pendingOperation = &nextPlan
							r.savePendingOperationState()
						}
						r.operationHistory = append(r.operationHistory, nextPlan)
						msg := commandOperationContinuationIntro(r.language, nextPlan)
						msg += "\n\n"
						msg += commandOperationPlanMarkdown(r.language, nextPlan)
						r.finishOperationRouteSpinner(nextPlan.Status)
						r.renderBorderedCompact(msg)
						r.recordTurn(display, request, msg, memory.KindPipeline)
						return
					}
				}
			}
		}
		r.startOperationSynthesisSpinner()
		msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
		r.finishOperationFinalAnswerHeader()
		r.emitOperationVisibleThoughts(thoughts)
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
		r.recordTurn(display, request, msg, memory.KindPipeline)
		return
	}
}

func (r *REPL) renderCommandOperationRoundResult(plan operation.CommandOperationPlan, result operation.CommandOperationResult) {
	msg := commandOperationResultMarkdown(r.language, plan, result)
	if strings.TrimSpace(msg) == "" {
		return
	}
	r.finishOperationRouteSpinner(result.Status)
	r.renderBordered(msg)
}

func (r *REPL) clearOperationRouteSpinnerForInlineResult() {
	if r.renderer == nil || !r.renderer.SpinnerActive() {
		return
	}
	r.renderer.StopSpinnerWithoutSummary()
}

func (r *REPL) commandOperationFinalMessage(ctx context.Context, userLine string, records []commandOperationResultRecord) (string, []string) {
	details := commandOperationRecordsMarkdown(r.language, records)
	if len(records) == 0 {
		return operationFinalReportFallback(r.language, operation.StatusFailed, 0), nil
	}
	last := records[len(records)-1]
	fallback := operationFinalReportFallback(r.language, last.Result.Status, len(records))
	if recordsAnswerer, ok := r.operationPlanner.(CommandOperationRecordsAnswerer); ok && recordsAnswerer != nil {
		answer, err := recordsAnswerer.AnswerCommandOperationRecords(ctx, userLine, records, r.language)
		r.emitReplLLMTrace(r.operationPlanner, "command_operation_answerer", types.AgentName("operation_planner"), types.PipelineStage("operation"))
		if err != nil {
			logging.Warning("[repl/operation] command result answer synthesis failed: %v", err)
			return fallback, nil
		}
		answer = strings.TrimSpace(answer)
		thoughts, answer := splitVisibleThinkBlocks(answer)
		if answer == "" {
			return fallback, thoughts
		}
		answer = operationFinalReportWithDetails(r.language, answer, details)
		return operationFinalReportWithRecordStatus(r.language, answer, records), thoughts
	}
	answerer, ok := r.operationPlanner.(CommandOperationAnswerer)
	if !ok || answerer == nil {
		return fallback, nil
	}
	answer, err := answerer.AnswerCommandOperationResult(ctx, userLine, last.Plan, last.Result, r.language)
	r.emitReplLLMTrace(r.operationPlanner, "command_operation_answerer", types.AgentName("operation_planner"), types.PipelineStage("operation"))
	if err != nil {
		logging.Warning("[repl/operation] command result answer synthesis failed: %v", err)
		return fallback, nil
	}
	answer = strings.TrimSpace(answer)
	thoughts, answer := splitVisibleThinkBlocks(answer)
	if answer == "" {
		return fallback, thoughts
	}
	answer = operationFinalReportWithDetails(r.language, answer, details)
	return operationFinalReportWithRecordStatus(r.language, answer, records), thoughts
}

type replAnswerOrigin string

const (
	replAnswerOriginLocal                  replAnswerOrigin = "local"
	replAnswerOriginPipeline               replAnswerOrigin = "pipeline"
	replAnswerOriginCommandOperationFinal  replAnswerOrigin = "command_operation_final"
	replAnswerOriginProviderOperationFinal replAnswerOrigin = "provider_operation_final"
)

func (r *REPL) emitOperationVisibleThoughts(thoughts []string) {
	if len(thoughts) == 0 {
		return
	}
	logging.Info("[repl/operation] moved %d visible think block(s) outside operation answer", len(thoughts))
	for i, thought := range thoughts {
		thought = strings.TrimSpace(thought)
		if thought == "" {
			continue
		}
		r.info(operationThoughtsHeaderMsg(r.language, i+1, len(thoughts)))
		for _, line := range strings.Split(thought, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			r.info("  " + line)
		}
	}
}

const (
	commandOperationMaxRepairRounds  = 3
	commandOperationMaxCommandRounds = 5
)

func commandOperationTerminalAfterReplan(plan operation.CommandOperationPlan, records []commandOperationResultRecord) bool {
	switch plan.Status {
	case operation.StatusComplete, operation.StatusBudgetExhausted, operation.StatusPartialAnswer:
		return true
	case operation.StatusBlocked:
		return len(plan.Steps) == 0 &&
			commandOperationRiskRank(plan.RiskLevel) <= commandOperationRiskRank("low") &&
			commandOperationRecordsHaveExecutedStep(records)
	default:
		return false
	}
}

func commandOperationTerminalResult(plan operation.CommandOperationPlan) operation.CommandOperationResult {
	status := plan.Status
	if status == operation.StatusBlocked {
		status = operation.StatusComplete
	}
	return operation.CommandOperationResult{
		PlanID:        plan.ID,
		Status:        status,
		OutputPreview: strings.TrimSpace(plan.BlockReason),
	}
}

func commandOperationRiskRank(risk string) int {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "none", "":
		return 1
	default:
		return 3
	}
}

func commandOperationRecordsHaveExecutedStep(records []commandOperationResultRecord) bool {
	for _, rec := range records {
		for _, step := range rec.Result.StepResults {
			if step.Status == operation.StatusExecuted {
				return true
			}
		}
	}
	return false
}

func (r *REPL) maybeReplanCommandOperation(ctx context.Context, failedPlan operation.CommandOperationPlan, result operation.CommandOperationResult, request, display string, replanAttempts int, records []commandOperationResultRecord) bool {
	if result.Status != operation.StatusFailed || replanAttempts >= commandOperationMaxRepairRounds || r.operationPlanner == nil {
		return false
	}
	if commandResultTimedOut(result) {
		return false
	}
	replanner, ok := r.operationPlanner.(CommandOperationReplanner)
	if !ok {
		return false
	}
	r.startOperationSynthesisSpinner()
	snapshot := r.commandOperationCapabilitySnapshot()
	revisedReq, err := replanner.ReplanCommandOperation(ctx, failedPlan.RequestText, r.repoRoot, commandOperationPolicyFromPlan(failedPlan), snapshot, failedPlan, result)
	r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
	if err != nil {
		logging.Warning("[repl/operation] command replan failed: %v", err)
		if degraded, ok := commandOperationStructuredToolParamFailureResult(failedPlan, err, r.language); ok {
			r.appendCommandOperationResult(failedPlan, degraded)
			records = append(records, commandOperationResultRecord{Plan: failedPlan, Result: degraded})
			r.renderCommandOperationRoundResult(failedPlan, degraded)
			r.startOperationSynthesisSpinner()
			msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
			r.finishOperationFinalAnswerHeader()
			r.emitOperationVisibleThoughts(thoughts)
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
			r.recordTurn(display, request, msg, memory.KindPipeline)
			return true
		}
		return false
	}
	revisedReq = dropRepeatedFailedCommandSteps(revisedReq, failedPlan, result)
	revisedPlan := operation.BuildCommandOperationPlan(revisedReq, r.operationPolicy)
	replanDecision := operation.DecideCommandPlanApproval(r.operationPolicy, revisedPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalReplan, PreviousPlan: &failedPlan})
	revisedPlan = operation.ApplyCommandPlanApprovalDecision(revisedPlan, replanDecision)
	logging.Info("[repl/operation] command replan generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d",
		failedPlan.ID, revisedPlan.Status, revisedPlan.RiskLevel, revisedPlan.ApprovalMode, replanDecision.Action, replanDecision.ReasonCode, len(revisedPlan.Steps))
	if commandOperationTerminalAfterReplan(revisedPlan, records) {
		terminal := commandOperationTerminalResult(revisedPlan)
		r.operationHistory = append(r.operationHistory, revisedPlan)
		r.appendCommandOperationResult(revisedPlan, terminal)
		records = append(records, commandOperationResultRecord{Plan: revisedPlan, Result: terminal})
		logging.Info("[repl/operation] command replan terminal previous_plan_id=%s terminal_plan_id=%s status=%s reason=%q records=%d",
			failedPlan.ID, revisedPlan.ID, revisedPlan.Status, oneLineClamp(revisedPlan.BlockReason, 180), len(records))
		r.renderCommandOperationRoundResult(revisedPlan, terminal)
		r.startOperationSynthesisSpinner()
		msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
		r.finishOperationFinalAnswerHeader()
		r.emitOperationVisibleThoughts(thoughts)
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
		r.recordTurn(display, request, msg, memory.KindPipeline)
		return true
	}
	if revisedPlan.Status == operation.StatusReady {
		if lint := commandRevisedPlanPreflightLint(revisedPlan, failedPlan, result); !lint.OK() {
			lintResult := commandOperationResultFromPlanLint(revisedPlan, lint)
			revisedPlan.Status = lintResult.Status
			r.operationHistory = append(r.operationHistory, revisedPlan)
			r.appendCommandOperationResult(revisedPlan, lintResult)
			records = append(records, commandOperationResultRecord{Plan: revisedPlan, Result: lintResult})
			logging.Info("[repl/operation] revised command plan preflight failed previous_plan_id=%s revised_plan_id=%s issues=%d summary=%q replan_attempt=%d",
				failedPlan.ID, revisedPlan.ID, len(lint.Issues), oneLineClamp(lint.Summary(), 240), replanAttempts+1)
			if r.maybeReplanCommandOperation(ctx, revisedPlan, lintResult, request, display, replanAttempts+1, records) {
				return true
			}
			if commandOperationRepairBudgetExhausted(lintResult, replanAttempts+1) {
				budget := commandOperationBudgetResult(revisedPlan, "command operation repair budget exhausted before the user goal was fully satisfied")
				r.appendCommandOperationResult(revisedPlan, budget)
				records = append(records, commandOperationResultRecord{Plan: revisedPlan, Result: budget})
			}
			r.startOperationSynthesisSpinner()
			msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
			r.finishOperationFinalAnswerHeader()
			r.emitOperationVisibleThoughts(thoughts)
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
			r.recordTurn(display, request, msg, memory.KindPipeline)
			return true
		}
		if replanDecision.AutoExecute() {
			msg := commandOperationResultMarkdown(r.language, failedPlan, result)
			msg += "\n\n"
			msg += commandOperationReplanIntro(r.language, revisedPlan)
			msg += "\n\n"
			msg += commandOperationAutoExecuteMarkdown(r.language, revisedPlan)
			r.finishCommandOperationPlanHeader(revisedPlan)
			r.renderBordered(msg)
			r.executeCommandOperationPlanAttempt(revisedPlan, request, display, replanAttempts+1, records)
			return true
		}
		r.pendingOperation = &revisedPlan
		r.savePendingOperationState()
	}
	r.operationHistory = append(r.operationHistory, revisedPlan)
	msg := commandOperationResultMarkdown(r.language, failedPlan, result)
	msg += "\n\n"
	msg += commandOperationReplanIntro(r.language, revisedPlan)
	msg += "\n\n"
	msg += commandOperationPlanMarkdown(r.language, revisedPlan)
	r.finishOperationRouteSpinner(revisedPlan.Status)
	r.renderBordered(msg)
	r.recordTurn(display, request, msg, memory.KindPipeline)
	return true
}

func (r *REPL) maybeContinueCommandOperation(ctx context.Context, plan operation.CommandOperationPlan, result operation.CommandOperationResult, request, display string, records []commandOperationResultRecord) bool {
	if result.Status != operation.StatusExecuted || !plan.ContinueAfter || commandOperationExecutedRoundCount(records) >= commandOperationMaxCommandRounds || r.operationPlanner == nil {
		return false
	}
	continuer, ok := r.operationPlanner.(CommandOperationContinuationPlanner)
	if !ok {
		return false
	}
	r.startOperationSynthesisSpinner()
	snapshot := r.commandOperationCapabilitySnapshot()
	next, err := continuer.ContinueCommandOperation(ctx, plan.RequestText, r.repoRoot, commandOperationPolicyFromPlan(plan), snapshot, records)
	r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
	if err != nil {
		logging.Warning("[repl/operation] command continuation planning failed: %v", err)
		if degraded, ok := commandOperationStructuredToolParamFailureResult(plan, err, r.language); ok {
			r.appendCommandOperationResult(plan, degraded)
			records = append(records, commandOperationResultRecord{Plan: plan, Result: degraded})
			r.renderCommandOperationRoundResult(plan, degraded)
			r.startOperationSynthesisSpinner()
			msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
			r.finishOperationFinalAnswerHeader()
			r.emitOperationVisibleThoughts(thoughts)
			r.renderBordered(msg)
			r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
			r.recordTurn(display, request, msg, memory.KindPipeline)
			return true
		}
		return false
	}
	if next.Complete {
		logging.Info("[repl/operation] command continuation complete plan_id=%s reason=%q rounds=%d",
			plan.ID, oneLineClamp(next.Reason, 160), len(records))
		return false
	}
	nextPlan := operation.BuildCommandOperationPlan(next.Request, r.operationPolicy)
	continuationDecision := operation.DecideCommandPlanApproval(r.operationPolicy, nextPlan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalContinuation, PreviousPlan: &plan})
	nextPlan = operation.ApplyCommandPlanApprovalDecision(nextPlan, continuationDecision)
	logging.Info("[repl/operation] command continuation generated previous_plan_id=%s status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d rounds=%d",
		plan.ID, nextPlan.Status, nextPlan.RiskLevel, nextPlan.ApprovalMode, continuationDecision.Action, continuationDecision.ReasonCode, len(nextPlan.Steps), len(records))
	if commandOperationTerminalAfterReplan(nextPlan, records) {
		terminal := commandOperationTerminalResult(nextPlan)
		r.operationHistory = append(r.operationHistory, nextPlan)
		r.appendCommandOperationResult(nextPlan, terminal)
		records = append(records, commandOperationResultRecord{Plan: nextPlan, Result: terminal})
		logging.Info("[repl/operation] command continuation terminal previous_plan_id=%s terminal_plan_id=%s status=%s reason=%q records=%d",
			plan.ID, nextPlan.ID, nextPlan.Status, oneLineClamp(nextPlan.BlockReason, 180), len(records))
		r.renderCommandOperationRoundResult(nextPlan, terminal)
		r.startOperationSynthesisSpinner()
		msg, thoughts := r.commandOperationFinalMessage(ctx, request, records)
		r.finishOperationFinalAnswerHeader()
		r.emitOperationVisibleThoughts(thoughts)
		r.renderBordered(msg)
		r.lastAnswerOrigin = replAnswerOriginCommandOperationFinal
		r.recordTurn(display, request, msg, memory.KindPipeline)
		return true
	}
	if nextPlan.Status == operation.StatusNeedsClarification && len(nextPlan.ClarifyingQuestions) == 0 {
		logging.Warning("[repl/operation] command continuation returned needs_clarification without actionable questions; falling back to final synthesis")
		return false
	}
	if nextPlan.Status == operation.StatusReady && continuationDecision.AutoExecute() {
		msg := commandOperationContinuationIntro(r.language, nextPlan)
		msg += "\n\n"
		msg += commandOperationAutoExecuteMarkdown(r.language, nextPlan)
		r.finishCommandOperationPlanHeader(nextPlan)
		r.renderBordered(msg)
		r.executeCommandOperationPlanAttempt(nextPlan, request, display, 0, records)
		return true
	}
	if nextPlan.Status == operation.StatusReady {
		r.pendingOperation = &nextPlan
		r.savePendingOperationState()
	}
	r.operationHistory = append(r.operationHistory, nextPlan)
	msg := commandOperationRecordsMarkdown(r.language, records)
	msg += "\n\n"
	msg += commandOperationContinuationIntro(r.language, nextPlan)
	msg += "\n\n"
	msg += commandOperationPlanMarkdown(r.language, nextPlan)
	r.finishOperationRouteSpinner(nextPlan.Status)
	r.renderBorderedCompact(msg)
	r.recordTurn(display, request, msg, memory.KindPipeline)
	return true
}

type commandOperationResultRecord struct {
	Plan       operation.CommandOperationPlan
	Result     operation.CommandOperationResult
	Evaluation *operation.OperationEvaluation
}

type providerOperationResultRecord struct {
	Plan    operation.Plan
	Request operation.Request
	Result  providerOperationResult
}

type pendingCommandClarification struct {
	OriginalLine string
	Display      string
	Policy       TurnPolicy
	Plan         operation.CommandOperationPlan
}

type pendingProviderOperation struct {
	Plan             operation.Plan
	Request          operation.Request
	Display          string
	Input            map[string]any
	WorkflowState    operation.WorkflowState
	WorkflowDepth    int
	WorkflowActionID string
}

func (r *REPL) appendCommandOperationResult(plan operation.CommandOperationPlan, result operation.CommandOperationResult) {
	r.operationResults = append(r.operationResults, commandOperationResultRecord{Plan: plan, Result: result})
	if len(r.operationResults) > 6 {
		r.operationResults = r.operationResults[len(r.operationResults)-6:]
	}
	if r.operationMemory != nil {
		entries := operation.BuildMemoryEntries(plan, result, r.commandOperationCapabilitySnapshot())
		if err := r.operationMemory.Append(entries...); err != nil {
			logging.Warning("[repl/operation] append operation memory failed: %v", err)
		}
	}
}

func (r *REPL) appendProviderOperationResult(plan operation.Plan, request operation.Request, result providerOperationResult) {
	r.providerOperationResults = append(r.providerOperationResults, providerOperationResultRecord{Plan: plan, Request: request, Result: result})
	if len(r.providerOperationResults) > 6 {
		r.providerOperationResults = r.providerOperationResults[len(r.providerOperationResults)-6:]
	}
	if r.operationMemory != nil {
		entry := operation.BuildProviderMemoryEntry(
			result.Provider,
			result.Tool,
			firstNonEmptyString(plan.Kind, request.OperationKind, request.Operation),
			firstNonEmptyString(plan.TargetSurface, request.TargetSurface),
			result.Status,
			result.Summary,
			result.PayloadRef,
			result.Error,
			result.ArtifactRefs,
			result.Observations,
			result.NextActions,
			result.ReturnAction,
			result.WorkflowState,
			r.commandOperationCapabilitySnapshot(),
		)
		if err := r.operationMemory.Append(entry); err != nil {
			logging.Warning("[repl/operation] append provider operation memory failed: %v", err)
		}
	}
}

func (r *REPL) renderCommandOperationHandoff() string {
	start := len(r.operationResults) - 2
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	for i := start; i < len(r.operationResults); i++ {
		rec := r.operationResults[i]
		fmt.Fprintf(&b, "operation_result plan_id=%s status=%s request=%q\n",
			rec.Plan.ID, rec.Result.Status, oneLineClamp(rec.Plan.RequestText, 200))
		if strings.TrimSpace(rec.Plan.Goal) != "" {
			fmt.Fprintf(&b, "  goal=%q\n", oneLineClamp(rec.Plan.Goal, 240))
		}
		if len(rec.Plan.KnownConstraints) > 0 {
			fmt.Fprintf(&b, "  known_constraints=%q\n", oneLineClamp(strings.Join(rec.Plan.KnownConstraints, " | "), 360))
		}
		if len(rec.Plan.MissingObservations) > 0 {
			fmt.Fprintf(&b, "  missing_observations=%q\n", oneLineClamp(strings.Join(rec.Plan.MissingObservations, " | "), 360))
		}
		if len(rec.Plan.SuccessCriteria) > 0 {
			fmt.Fprintf(&b, "  success_criteria=%q\n", oneLineClamp(strings.Join(rec.Plan.SuccessCriteria, " | "), 360))
		}
		for _, step := range rec.Result.StepResults {
			planStep := commandOperationStepByID(rec.Plan, step.StepID)
			cmd := commandOperationStepCommand(planStep)
			preview := oneLineClamp(step.OutputPreview, 1200)
			summary := operation.SummarizeStepOutput(step)
			materials := operation.MaterialsFromCommandStep(step)
			fmt.Fprintf(&b, "  step id=%s cmd=%q status=%s exit_code=%d timed_out=%t failure_class=%s verification_status=%s verification_kind=%s verification_summary=%q output_kind=%s output_summary=%q material_refs=%q error=%q output_preview=%q payload_ref=%s\n",
				step.StepID, cmd, step.Status, step.ExitCode, step.TimedOut, step.FailureClass, step.Verification.Status, step.Verification.Kind, oneLineClamp(step.Verification.Summary, 220), summary.Kind, oneLineClamp(summary.Summary, 260), operation.RenderMaterialsInline(materials, 4), oneLineClamp(step.Error, 240), preview, step.PayloadRef)
		}
	}
	providerStart := len(r.providerOperationResults) - 2
	if providerStart < 0 {
		providerStart = 0
	}
	for i := providerStart; i < len(r.providerOperationResults); i++ {
		rec := r.providerOperationResults[i]
		materials := operation.MaterialsFromProviderResult(
			rec.Result.Provider,
			rec.Result.Tool,
			rec.Result.Summary,
			rec.Result.PayloadRef,
			rec.Result.ArtifactRefs,
			rec.Result.Observations,
			rec.Result.NextActions,
			rec.Result.ReturnAction,
			rec.Result.WorkflowState,
		)
		fmt.Fprintf(&b, "provider_operation_result provider=%q tool=%q kind=%q target=%q status=%s observations=%d request=%q summary=%q error=%q payload_ref=%s artifact_refs=%s material_refs=%q verification_status=%s verification_summary=%q next_actions=%d return_action=%q workflow_state=%q diagnostics=%q\n",
			oneLineClamp(rec.Result.Provider, 120),
			oneLineClamp(rec.Result.Tool, 120),
			oneLineClamp(firstNonEmptyString(rec.Plan.Kind, rec.Request.OperationKind, rec.Request.Operation), 80),
			oneLineClamp(firstNonEmptyString(rec.Plan.TargetSurface, rec.Request.TargetSurface), 80),
			rec.Result.Status,
			rec.Result.Observations,
			oneLineClamp(rec.Request.Text, 200),
			oneLineClamp(rec.Result.Summary, 500),
			oneLineClamp(rec.Result.Error, 260),
			rec.Result.PayloadRef,
			oneLineClamp(strings.Join(rec.Result.ArtifactRefs, ","), 500),
			operation.RenderMaterialsInline(materials, 8),
			rec.Result.VerificationStatus,
			oneLineClamp(rec.Result.VerificationSummary, 220),
			len(rec.Result.NextActions),
			oneLineClamp(rec.Result.ReturnAction.Compact(), 260),
			oneLineClamp(rec.Result.WorkflowState.Compact(), 260),
			oneLineClamp(strings.Join(rec.Result.WorkflowDiagnostics, "; "), 360))
		for j, action := range rec.Result.NextActions {
			if j >= 3 {
				break
			}
			fmt.Fprintf(&b, "  next_action[%d] %s\n", j, oneLineClamp(action.Compact(), 500))
		}
	}
	if r.operationMemory != nil {
		if entries, err := r.operationMemory.RecentMatches(r.commandOperationCapabilitySnapshot(), 4); err == nil {
			if rendered := operation.RenderMemoryForPrompt(entries); rendered != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(rendered)
				b.WriteString("\n")
			}
		} else {
			logging.Warning("[repl/operation] read operation memory failed: %v", err)
		}
	}
	return strings.TrimSpace(b.String())
}

func commandOperationStepByID(plan operation.CommandOperationPlan, id string) operation.CommandStep {
	for _, step := range plan.Steps {
		if step.ID == id {
			return step
		}
	}
	return operation.CommandStep{ID: id}
}

func commandOperationStepCommand(step operation.CommandStep) string {
	if strings.TrimSpace(step.Shell) != "" {
		return strings.TrimSpace(step.Shell)
	}
	return strings.TrimSpace(strings.TrimSpace(step.Program) + " " + strings.Join(step.Args, " "))
}

func commandResultTimedOut(result operation.CommandOperationResult) bool {
	for _, step := range result.StepResults {
		if step.TimedOut || step.Status == operation.StatusCancelled {
			return true
		}
	}
	return false
}

func commandOperationPrimaryFailureClass(result operation.CommandOperationResult) string {
	for _, step := range result.StepResults {
		if fc := strings.TrimSpace(step.FailureClass); fc != "" {
			return fc
		}
	}
	return ""
}

func commandOperationRepairBudgetExhausted(result operation.CommandOperationResult, replanAttempts int) bool {
	return result.Status == operation.StatusFailed &&
		replanAttempts >= commandOperationMaxRepairRounds &&
		!commandResultTimedOut(result)
}

func commandOperationContinuationBudgetExhausted(plan operation.CommandOperationPlan, result operation.CommandOperationResult, records []commandOperationResultRecord) bool {
	return result.Status == operation.StatusExecuted &&
		plan.ContinueAfter &&
		commandOperationExecutedRoundCount(records) >= commandOperationMaxCommandRounds
}

func commandOperationExecutedRoundCount(records []commandOperationResultRecord) int {
	count := 0
	for _, record := range records {
		if record.Result.Status == operation.StatusExecuted {
			count++
		}
	}
	return count
}

func commandOperationBudgetResult(plan operation.CommandOperationPlan, reason string) operation.CommandOperationResult {
	return operation.CommandOperationResult{
		PlanID:        plan.ID,
		Status:        operation.StatusFailed,
		OutputPreview: reason,
		StepResults: []operation.CommandStepResult{{
			Status:        operation.StatusFailed,
			OutputPreview: reason,
			Error:         reason,
			FailureClass:  "budget_exhausted",
		}},
	}
}

func commandOperationStructuredToolParamFailureResult(plan operation.CommandOperationPlan, err error, lang string) (operation.CommandOperationResult, bool) {
	var paramErr *replStructuredToolParamError
	if !errors.As(err, &paramErr) {
		return operation.CommandOperationResult{}, false
	}
	reason := "structured tool parameters were malformed after compact repair; no further operation commands were executed"
	if isZh(lang) {
		reason = "结构化工具参数在紧凑修复后仍不合法；系统未执行新的操作命令，已基于现有结果降级作答"
	}
	return operation.CommandOperationResult{
		PlanID:        plan.ID,
		Status:        operation.StatusFailed,
		OutputPreview: reason,
		StepResults: []operation.CommandStepResult{{
			Status:        operation.StatusFailed,
			OutputPreview: reason,
			Error:         reason,
			FailureClass:  "structured_tool_params",
		}},
	}, true
}

func commandOperationResultFromPlanLint(plan operation.CommandOperationPlan, lint operation.CommandPlanLintResult) operation.CommandOperationResult {
	result := operation.CommandOperationResult{
		PlanID: plan.ID,
		Status: operation.StatusFailed,
	}
	if len(lint.Issues) == 0 {
		result.StepResults = append(result.StepResults, operation.CommandStepResult{
			Status:       operation.StatusFailed,
			Error:        "operation plan failed lint",
			FailureClass: "invalid_plan",
		})
		return result
	}
	for _, issue := range lint.Issues {
		msg := strings.TrimSpace(issue.Message)
		if msg == "" {
			msg = strings.TrimSpace(issue.Code)
		}
		result.StepResults = append(result.StepResults, operation.CommandStepResult{
			StepID:        issue.StepID,
			Status:        operation.StatusFailed,
			OutputPreview: fmt.Sprintf("[operation command was not run: %s]", msg),
			Error:         msg,
			FailureClass:  "invalid_plan",
		})
	}
	result.OutputPreview = lint.Summary()
	return result
}

func commandRevisedPlanPreflightLint(revisedPlan, failedPlan operation.CommandOperationPlan, result operation.CommandOperationResult) operation.CommandPlanLintResult {
	lint := operation.LintCommandOperationPlan(revisedPlan)
	failedCommands := map[string]bool{}
	for _, stepResult := range result.StepResults {
		if stepResult.Status != operation.StatusFailed {
			continue
		}
		failedStep := commandOperationStepByID(failedPlan, stepResult.StepID)
		if key := normalizedCommandStepKey(failedStep); key != "" {
			failedCommands[key] = true
		}
	}
	if len(failedCommands) == 0 {
		return lint
	}
	for _, step := range revisedPlan.Steps {
		key := normalizedCommandStepKey(step)
		if key == "" || !failedCommands[key] {
			continue
		}
		lint.Issues = append(lint.Issues, operation.CommandPlanLintIssue{
			Code:    operation.PlanLintRepeatedFailedStep,
			StepID:  step.ID,
			Message: fmt.Sprintf("revised plan repeats an already failed command instead of using the failure output to choose a new action: %s", commandOperationStepCommand(step)),
		})
	}
	return lint
}

func dropRepeatedFailedCommandSteps(req operation.CommandOperationRequest, failedPlan operation.CommandOperationPlan, result operation.CommandOperationResult) operation.CommandOperationRequest {
	if len(req.Steps) <= 1 || len(result.StepResults) == 0 {
		return req
	}
	failedCommands := map[string]bool{}
	for _, stepResult := range result.StepResults {
		if stepResult.Status != operation.StatusFailed {
			continue
		}
		failedStep := commandOperationStepByID(failedPlan, stepResult.StepID)
		if key := normalizedCommandStepKey(failedStep); key != "" {
			failedCommands[key] = true
		}
	}
	if len(failedCommands) == 0 {
		return req
	}
	filtered := req.Steps[:0]
	dropped := 0
	for _, step := range req.Steps {
		if failedCommands[normalizedCommandStepKey(step)] {
			dropped++
			continue
		}
		filtered = append(filtered, step)
	}
	if dropped == 0 || len(filtered) == 0 {
		return req
	}
	req.Steps = filtered
	logging.Info("[repl/operation] dropped %d repeated failed command step(s) from revised plan", dropped)
	return req
}

func normalizedCommandStepKey(step operation.CommandStep) string {
	if strings.TrimSpace(step.Shell) != "" {
		return "shell:" + strings.Join(strings.Fields(step.Shell), " ")
	}
	program := strings.TrimSpace(step.Program)
	if program == "" {
		return ""
	}
	parts := append([]string{program}, step.Args...)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return "argv:" + strings.Join(parts, "\x00")
}

func commandReplanCanAutoExecute(policy operation.CommandPolicy, previous, revised operation.CommandOperationPlan) bool {
	return operation.DecideCommandPlanApproval(policy, revised, operation.CommandApprovalOptions{Phase: operation.CommandApprovalReplan, PreviousPlan: &previous}).AutoExecute()
}

func commandOperationPolicyFromPlan(plan operation.CommandOperationPlan) TurnPolicy {
	return TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "current_message",
		RiskLevel:            plan.RiskLevel,
		TargetSurface:        "desktop",
		RequiresConfirmation: plan.ApprovalMode == operation.ApprovalManual,
	}
}

func (r *REPL) operationOutputDir() string {
	if strings.TrimSpace(r.runtimeAnchor) == "" {
		return ""
	}
	return filepath.Join(r.runtimeAnchor, "operation")
}

// currentStickyTag returns the per-turn sticky-state marker promptStickyTag
// would compose for the REPL's current state. Single call site so every
// prompt-rendering surface (main input, paste/log capture mode, multi-line
// continuation, scripted-mode echo) sees the SAME tag for a given turn —
// without this helper each surface re-derived the tag (or worse, omitted it)
// and the user lost visibility of [task:*] / [phase:*] / [log] / [trace] /
// [plan] / [mem!] mid-flow.
func (r *REPL) currentStickyTag() string {
	// Probe git branch fresh per call — this fires once per user
	// input, so the cost is bounded (one local git exec ~1ms) and
	// the user sees branch changes from another terminal at the
	// VERY NEXT prompt cycle. No caching; correctness > 1ms.
	return promptStickyTagWithAttachmentLabels(
		string(r.userMode.Normalize()),
		string(r.currentMode),
		gitBranchProbe(r.repoRoot),
		attachmentPromptLabel("log", r.attachedLog),
		traceAttachmentPromptLabel(r.attachedHitrace),
		r.pendingPlanPath != "",
		r.memoryUnderPressure(),
		r.currentFocusSlice(),
	)
}

func attachmentPromptLabel(kind, body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	count := len(attachedSourcePaths(body))
	if count <= 1 {
		return kind
	}
	return fmt.Sprintf("%s:%d", kind, count)
}

func traceAttachmentPromptLabel(body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	paths := attachedSourcePaths(body)
	if len(paths) == 0 {
		if strings.Contains(body, "perf_sample:") {
			return "trace+perf"
		}
		return "trace"
	}
	kinds := map[string]int{}
	for _, path := range paths {
		kinds[traceAttachmentKind(path)]++
	}
	if len(paths) == 1 {
		switch {
		case kinds["perftrace"] > 0:
			return "perftrace"
		case kinds["tracebundle"] > 0:
			return "tracebundle"
		case kinds["perfdata"] > 0:
			return "perfdata"
		default:
			return "trace"
		}
	}
	label := fmt.Sprintf("trace:%d", len(paths))
	if kinds["perftrace"] > 0 || kinds["perfdata"] > 0 || strings.Contains(body, "perf_sample:") {
		label += "+perf"
	}
	if kinds["tracebundle"] > 0 {
		label += "+bundle"
	}
	return label
}

func attachedSourcePaths(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "# codrax-source: "); ok {
			path = strings.TrimSpace(path)
			if path != "" {
				out = append(out, path)
			}
		}
	}
	return out
}

func traceAttachmentKind(path string) string {
	p := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(p, ".tracebundle.json"):
		return "tracebundle"
	case strings.HasSuffix(p, ".perftrace"):
		return "perftrace"
	case strings.HasSuffix(p, ".perf.data") || filepath.Base(p) == "perf.data":
		return "perfdata"
	default:
		return "trace"
	}
}

// currentFocusSlice returns the operator-pinned sub-repo RootRel
// list, sorted alphabetically for deterministic prompt rendering.
// Empty when no pin has been set or when the topology was never
// populated (single-repo / pre-multi-repo).
func (r *REPL) currentFocusSlice() []string {
	r.multiRepoMu.Lock()
	if len(r.multiRepoFocus) == 0 || r.topology == nil {
		r.multiRepoMu.Unlock()
		return nil
	}
	out := make([]string, 0, len(r.multiRepoFocus))
	for slug := range r.multiRepoFocus {
		// Map slug → RootRel for the prompt display. Slug stays
		// internal; the LLM-visible / operator-visible name is the
		// path-derived RootRel.
		if sr := r.topology.SubRepoBySlug(slug); sr != nil {
			out = append(out, sr.RootRel)
		}
	}
	r.multiRepoMu.Unlock()
	sort.Strings(out)
	return out
}

// Loop runs the prompt until /exit, /quit, or EOF.
//
// The interactive readInput session renders its own echo + divider
// above the inline viewport via tea.Printf, so this function only
// owns echo in the line-oriented (scripted/piped) branch.
// installCancelSignalHandler arms a SIGINT listener that drives the
// Run-in-flight cancel path. Idempotent (sync.Once); a single REPL
// instance never reaches two handlers. Suppresses worktree's
// package-level handler so the two don't race — REPL takes full
// ownership of SIGINT for its lifetime and is responsible for
// invoking worktree.CleanActiveSessions on its own exit paths.
//
// Unified double-tap semantic across BOTH states (in-Run AND idle
// prompt): a SINGLE Ctrl+C never quits the process. The dock
// message has always advertised "Ctrl+C 取消(连按 2 次强制退出)";
// pre-2026-05-02 the idle-prompt path silently exited on the first
// signal, contradicting the message and trapping operators who hit
// Ctrl+C at the prompt expecting "cancel any in-flight thinking and
// stay at the prompt".
//
// Behaviour by state:
//
//   - First signal anywhere within doubleSigCancelWindow:
//
//   - runInFlight: call runner.Cancel("Ctrl+C") + cancelTurn() so
//     the pipeline / chitchat unwinds at the next checkpoint;
//     operator sees "✗ canceled" rendering when Run returns.
//
//   - idle prompt: print "再按一次 Ctrl+C 退出 codrax" warning;
//     operator can press once more to confirm OR keep using REPL.
//
//   - Second signal within the window: escalate to clean exit
//     regardless of state. Drive worktree cleanup ourselves since
//     the package handler is suppressed; print "Goodbye!" and
//     os.Exit(130). Two consecutive Ctrl+C presses (within 2 s) are
//     the universal "I really want out" signal.
//
//   - Window expiry: stamp goes stale automatically via the
//     time.Duration(now-prev) < doubleSigCancelWindow comparison —
//     no separate timer needed. After 2 s with no follow-up, the
//     next single tap is treated as fresh, returning to the
//     "warn / cancel only" first-tap behaviour.
//
// The handler runs in its own goroutine; signal channel is buffered
// to drop replays we cannot service in real time.
func (r *REPL) installCancelSignalHandler() {
	canceller, ok := r.runner.(runnerCanceller)
	if !ok {
		return // test stub or single-shot Runner — Ctrl+C keeps default semantics
	}
	r.cancelSigOnce.Do(func() {
		// Take ownership of SIGINT for the REPL's lifetime. The
		// worktree handler installed at cmd/root.go startup goes
		// passive while this flag is set; the REPL drives cleanup
		// on its own exit paths via worktree.CleanActiveSessions().
		worktree.SetSignalHandlerSuppressed(true)

		ch := make(chan os.Signal, 4)
		signal.Notify(ch, syscall.SIGINT)
		go func() {
			for range ch {
				now := time.Now().UnixNano()
				prev := r.lastCancelSig.Swap(now)
				doubleTap := prev > 0 && time.Duration(now-prev) < doubleSigCancelWindow
				if doubleTap {
					// Second signal within the window — escalate to
					// clean exit, regardless of in-Run / idle state.
					// Drive cleanup ourselves since the package handler is
					// suppressed, but do not let cleanup block the advertised
					// force-exit path. On a swapping host or a stuck worktree
					// filesystem, the second Ctrl+C must still terminate.
					done := make(chan struct{})
					go func() {
						worktree.CleanActiveSessions()
						close(done)
					}()
					select {
					case <-done:
					case <-time.After(500 * time.Millisecond):
					}
					fmt.Fprintln(r.out, "  Goodbye!")
					os.Exit(130)
				}
				if r.runInFlight.Load() {
					// First signal during Run: cancel pipeline.
					canceller.Cancel("Ctrl+C")
					// Also cancel any in-flight chitchat turn ctx —
					// chitchat runs on the REPL goroutine outside
					// runInFlightWrap, so orchestrator's Cancel
					// doesn't reach it. The per-turn cancel func
					// (set by chitchatDispatch / classifier dispatch)
					// is the surface that closes the chitchat HTTP
					// socket immediately.
					r.cancelTurn()
					r.warn("%s\n", cancelInProgressMsg(r.language))
					continue
				}
				// First signal at idle prompt: warn + ask for
				// confirmation. Operator presses Ctrl+C again within
				// 2 s to exit, or carries on using the REPL.
				r.warn("%s\n", idleConfirmExitMsg(r.language))
			}
		}()
	})
}

// startTurn allocates a fresh per-turn ctx and stores its cancel
// func so the SIGINT handler can fire it. Pairs with endTurn (defer
// safe). Returns the ctx to plumb into chitchat / classifier calls.
func (r *REPL) startTurn() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	r.turnCancelMu.Lock()
	if prev := r.turnCancel; prev != nil {
		// Previous turn never reached endTurn (panic mid-dispatch?)
		// — release its goroutines explicitly so we don't leak ctx
		// channels.
		prev()
	}
	r.turnCancel = cancel
	r.turnCancelMu.Unlock()
	return ctx
}

// endTurn cancels + clears the per-turn ctx. Safe to call multiple
// times; idempotent. Defer this from the entry of any non-pipeline
// LLM dispatch so a panic / early return still releases resources.
func (r *REPL) endTurn() {
	r.turnCancelMu.Lock()
	defer r.turnCancelMu.Unlock()
	if r.turnCancel != nil {
		r.turnCancel()
		r.turnCancel = nil
	}
}

// cancelTurn fires the per-turn cancel func without clearing it
// (the dispatch goroutine still owns the lifecycle and will clear
// via endTurn / defer). Used by the SIGINT handler.
func (r *REPL) cancelTurn() {
	r.turnCancelMu.Lock()
	defer r.turnCancelMu.Unlock()
	if r.turnCancel != nil {
		r.turnCancel()
	}
}

// runInFlightWrap is the dispatch helper every Run-driving slash
// handler / pipeline turn calls. Sets runInFlight true, fires fn,
// clears the flag in a defer (panic-safe), and returns fn's outputs.
// Centralising the lifecycle here means a future change to the
// in-flight semantics — say, also disabling certain slash commands
// while running — has exactly one place to land.
//
// Two cancel surfaces are armed for the duration of fn:
//
//  1. SIGINT handler — fires on Ctrl+C from any environment. Always
//     installed (idempotent sync.Once). Primary path for TTY
//     operators.
//  2. Stdin cancel listener — only starts when stdin is non-TTY
//     (pipe / redirect / scripted test). Reads `/cancel` lines and
//     drives the same Cancel(reason) entry point. Skipped silently
//     when stdin is a real TTY because bubbletea owns the input box
//     during prompts and a concurrent reader would race with the
//     next bubbletea iteration. TTY operators rely on Ctrl+C.
func (r *REPL) runInFlightWrap(fn func() (*types.BusContext, error)) (*types.BusContext, error) {
	r.installCancelSignalHandler()
	canceller, _ := r.runner.(runnerCanceller)
	listener := startCancelListener(r.in, canceller, r.warn)
	defer listener.stop() // nil-safe
	r.runInFlight.Store(true)
	defer r.runInFlight.Store(false)
	return fn()
}

func (r *REPL) Loop() error {
	r.banner()
	memNudgeShown := false
	for {
		// Sticky-state tag prepended to the prompt so the user has a
		// constant visual reminder of what attachments / mode / plan
		// are live for this turn. Empty when nothing sticky.
		tag := r.currentStickyTag()
		// One-shot memory pressure hint — surface ABOVE the prompt
		// the first time the threshold trips per session, so the
		// nudge is unmissable but doesn't spam every turn. The
		// [mem!] tag in the prompt remains visible after that.
		if !memNudgeShown && r.memoryUnderPressure() {
			recent, idx := r.memoryStats()
			r.warn("%s\n", memoryPressureHint(r.language, recent, idx))
			memNudgeShown = true
		}
		// Reset the one-shot latch when the user actually compacts
		// or clears, so a fresh accumulation re-warns once.
		if memNudgeShown && !r.memoryUnderPressure() {
			memNudgeShown = false
		}
		// Stash the tag so readInputBubble can re-render it on the echo
		// line above the inline viewport.
		r.echoTag = tag
		line, display, err := r.readInputPair(tag + "❯❯")
		if err != nil {
			fmt.Fprintln(r.out)
			fmt.Fprintln(r.out, "  Goodbye!")
			return nil
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !r.interactive() {
			// Scanner mode: preserve the "> " marker so piped test output
			// still contains the visual assertion target. Prepend the
			// sticky-state tag (mode / attachments) when non-empty so
			// scripted assertions can verify mode-tagged echoes too.
			if tag != "" {
				fmt.Fprintf(r.out, "  %s > %s\n", tag, display)
			} else {
				fmt.Fprintf(r.out, "  > %s\n", display)
			}
		}
		// Bang-prefix shell escape. Lets the user run a one-shot
		// system command without leaving the REPL — typical use is
		// `!ls`, `!cat file.go`, `!grep -n foo *.go`. We intercept
		// before the slash dispatcher and the LLM dispatch so a
		// line that starts with `!` never reaches the analyzer.
		// Multi-line / paste shapes that happen to start with `!`
		// are also caught here; that's a rare collision and the
		// shell error is informative enough.
		if strings.HasPrefix(line, "!") {
			r.handleShellBangCmd(line[1:])
			continue
		}
		if cmd := types.NormalizeREPLCommandAlias(line); cmd != "" {
			r.pendingHitraceSource = replTraceSourceHint(line)
			if quit := r.handleSlash(cmd); quit {
				r.pendingHitraceSource = ""
				return nil
			}
			r.pendingHitraceSource = ""
			continue
		}
		r.dispatch(line, display)
	}
}

func (r *REPL) banner() {
	if r.bannerText != "" {
		fmt.Fprintln(r.out, r.bannerText)
		return
	}
	if !r.headerPrinted {
		PrintStartupHeader(r.out, r.version, r.repoRoot)
	}
	// One-line capability summary so the user sees at startup which
	// modes are available and which yaml file backs the config. With
	// write_enabled=false the line names the gate explicitly so the
	// user does not have to wait until /mode write to discover it.
	// Banner-row width budget (commit 42 P1): banners are
	// dim FgDarkGray status lines aligned at column 2; long
	// settings paths or long pitfall counts could push them
	// past 120 columns and wrap onto a second visual line,
	// breaking vertical alignment with other rows. Clamp at
	// emit time so wraps are explicit ellipses rather than
	// terminal wrap chaos.
	const bannerMaxWidth = 120
	if line := strings.TrimSpace(r.modelListLine); line != "" {
		fmt.Fprintf(r.out, "  %s\n", pterm.FgDarkGray.Sprint(clampToTermWidth(line, bannerMaxWidth)))
	}
	if line := strings.TrimSpace(r.modelSummaryLine); line != "" {
		fmt.Fprintf(r.out, "  %s\n", pterm.FgDarkGray.Sprint(clampToTermWidth(line, bannerMaxWidth)))
	}
	if cap := bannerCapabilityLine(r.language, r.writeEnabled, r.settingsPath); cap != "" {
		fmt.Fprintf(r.out, "  %s\n", pterm.FgDarkGray.Sprint(clampToTermWidth(cap, bannerMaxWidth)))
	}
	// UX#4 (commit 41): when the per-repo Failure Taxonomy is
	// non-empty, surface the count at startup so /pitfalls
	// becomes discoverable for users who never read the full
	// /help output. Disabled-store + zero-pitfall states stay
	// silent.
	if r.failureTaxonomy != nil {
		if pitfalls := r.failureTaxonomy.All(); len(pitfalls) > 0 {
			fmt.Fprintf(r.out, "  %s\n",
				pterm.FgDarkGray.Sprint(clampToTermWidth(
					failureTaxonomyBannerLine(r.language, len(pitfalls)), bannerMaxWidth)))
		}
	}
	// Multi-repo workspace summary (P4-2026-05-08). Shown only when
	// topology has > 1 sub-repo so single-repo users (the dominant
	// case) see no extra chrome. Surfaces sub-repo count + active
	// cap + focus-pin count + "/repos" command discovery.
	if r.topology != nil && !r.topology.IsSingle() {
		focusCount := 0
		r.multiRepoMu.Lock()
		focusCount = len(r.multiRepoFocus)
		capN := r.activeMultiRepoMaxActiveLocked(r.multiRepoMaxActiveOverride)
		r.multiRepoMu.Unlock()
		if line := multiRepoBannerLine(r.language, r.multiRepoEnabled, len(r.topology.Repos), focusCount, capN); line != "" {
			line = clampToTermWidth(line, bannerMaxWidth)
			style := pterm.NewStyle(pterm.FgYellow, pterm.Bold)
			fmt.Fprintf(r.out, "  %s\n",
				style.Sprint(line))
		}
	}
	// Active write workflow state is surfaced before the legacy
	// unsettled-plan banner so users see the controller's typed next action
	// instead of an internal plan lifecycle when a durable run exists.
	if line := r.activeWriteWorkflowBannerLine(); line != "" {
		fmt.Fprintf(r.out, "  %s\n",
			pterm.FgDarkGray.Sprint(clampToTermWidth(line, bannerMaxWidth)))
	} else if blocker, ok := r.detectUnsettledPlan(); ok {
		fmt.Fprintf(r.out, "  %s\n",
			pterm.FgDarkGray.Sprint(clampToTermWidth(
				unsettledBanner(r.language, blocker.ID, blocker.Status, blocker.WorktreeMissing),
				bannerMaxWidth)))
	}
	if line := r.pendingOperationBannerLine(); line != "" {
		fmt.Fprintf(r.out, "  %s\n",
			pterm.FgDarkGray.Sprint(clampToTermWidth(line, bannerMaxWidth)))
	}
	if summary := r.memorySummaryLine(); summary != "" {
		fmt.Fprintf(r.out, "  %s\n", summary)
	}
	for _, h := range r.degradedEnvHints() {
		fmt.Fprintf(r.out, "  %s\n", h)
	}
	fmt.Fprintln(r.out)
}

// PrintStartupHeader renders the top CODRAX identity row. cmd/root uses this
// when OAuth needs to prompt before the REPL object exists, so the browser URL
// still appears under the same visual header the eventual REPL banner uses.
func PrintStartupHeader(out io.Writer, version, repoRoot string) {
	if out == nil {
		out = os.Stdout
	}
	badge := pterm.NewStyle(pterm.BgBlue, pterm.FgWhite, pterm.Bold).Sprint(" CODRAX ")
	// Compact banner version: strip the -dirty build-state suffix so a
	// long-running interactive session isn't dominated by build noise.
	// The full identifier is still available via /version.
	shortVer := strings.TrimSuffix(version, "-dirty")
	ver := pterm.FgGray.Sprintf("v%s", shortVer)
	hint := pterm.FgDarkGray.Sprint("/help · /exit")
	// Surface git branch up-front so the operator sees which branch
	// codrax is operating against (and which one /merge will fast-
	// forward into when --branch is omitted) before they type any
	// command. Empty when the path isn't a git repo (rare for
	// codrax usage; possible during /mode write auto-init scaffold).
	branchInfo := ""
	if br := gitBranchProbe(repoRoot); br != "" {
		branchInfo = pterm.FgDarkGray.Sprintf("  git:%s", br)
	}
	fmt.Fprintf(out, "\n  %s %s%s  %s\n", badge, ver, branchInfo, hint)
}

// PrintOAuthAuthorizationPrompt renders the pre-REPL OAuth browser prompt with
// the same subdued prefix language as normal REPL info rows. cmd/root calls
// this before the REPL object exists; keeping the styling here avoids a second
// UX dialect in the startup path.
func PrintOAuthAuthorizationPrompt(out io.Writer, lang, url string) {
	if out == nil {
		out = os.Stdout
	}
	msg := "需要完成 LLM OAuth 授权。请在浏览器打开："
	if !isZh(lang) {
		msg = "LLM OAuth authorization required. Open this URL in a browser:"
	}
	prefix := pterm.NewStyle(pterm.FgCyan).Sprint("›")
	body := pterm.NewStyle(pterm.FgDarkGray).Sprint(msg)
	link := pterm.NewStyle(pterm.FgLightBlue).Sprint(strings.TrimSpace(url))
	fmt.Fprintf(out, "  %s %s\n    %s\n", prefix, body, link)
}

// PrintOAuthAuthorizationComplete renders the successful end of an OAuth
// browser authorization flow. It deliberately avoids token/cache details.
func PrintOAuthAuthorizationComplete(out io.Writer, lang string) {
	if out == nil {
		out = os.Stdout
	}
	msg := "LLM OAuth 授权已完成，正在继续初始化。"
	if !isZh(lang) {
		msg = "LLM OAuth authorization complete; continuing startup."
	}
	fmt.Fprintf(out, "  %s %s\n", pterm.FgGreen.Sprint("✓"), pterm.FgDarkGray.Sprint(msg))
}

// PrintTopologyDiscoveryStart renders the startup-only workspace topology
// discovery status in the same visual dialect as the REPL banner rows.
func PrintTopologyDiscoveryStart(out io.Writer, lang string) {
	if out == nil {
		out = os.Stdout
	}
	msg := "正在发现工作区子仓拓扑（仅元数据，不构建 repo_map 索引）"
	if !isZh(lang) {
		msg = "Discovering workspace sub-repo topology (metadata only; not building repo_map indexes)"
	}
	fmt.Fprintf(out, "  %s %s\n", pterm.FgDarkGray.Sprint("·"), pterm.FgDarkGray.Sprint(msg))
}

// PrintTopologyDiscoveryComplete renders the successful end of startup
// topology discovery. elapsed should already be formatted for display.
func PrintTopologyDiscoveryComplete(out io.Writer, lang string, count int, elapsed string) {
	if out == nil {
		out = os.Stdout
	}
	msg := fmt.Sprintf("工作区子仓拓扑已就绪：%d 个子仓 (%s)", count, elapsed)
	if !isZh(lang) {
		msg = fmt.Sprintf("Workspace topology ready: %d sub-repo(s) (%s)", count, elapsed)
	}
	if count > 1 {
		if isZh(lang) {
			msg += "；非跨仓任务可用 --multi-repo=false 跳过多仓路由"
		} else {
			msg += "; not a cross-repo task? use --multi-repo=false to skip multi-repo routing"
		}
	}
	fmt.Fprintf(out, "  %s %s\n", pterm.FgGreen.Sprint("✓"), pterm.FgDarkGray.Sprint(msg))
}

// PrintTopologyDiscoveryError renders a startup topology discovery failure.
func PrintTopologyDiscoveryError(out io.Writer, lang string, err error, elapsed string) {
	if out == nil {
		out = os.Stdout
	}
	msg := fmt.Sprintf("工作区子仓拓扑发现失败：%v (%s)", err, elapsed)
	if !isZh(lang) {
		msg = fmt.Sprintf("Workspace topology discovery failed: %v (%s)", err, elapsed)
	}
	fmt.Fprintf(out, "  %s %s\n", pterm.FgRed.Sprint("✗"), pterm.FgDarkGray.Sprint(msg))
}

// degradedEnvHints returns zero-or-more lines describing environment
// degradations (sub-optimal search backend, missing git) that the
// operator should notice at startup. Rendered under the memory
// summary line so the banner stays empty on a healthy box.
//
// The search-backend rung has three tiers — rg / grep / native —
// mirroring the log line emitted by tool.SearchCommand. Anything
// that is not rg triggers an install-ripgrep nudge; the text varies
// so users can tell "slightly slower" from "much slower" at a glance.
//
// Language: banners are rendered in zh by default (matching
// codrax.yaml's default) and only switch to en when --lang=en is set
// explicitly. Any other value (fr, ja, off, …) falls back to zh —
// the banner is UI chrome, not answer text.
func (r *REPL) degradedEnvHints() []string {
	zh := !strings.EqualFold(strings.TrimSpace(r.language), "en")
	return renderDegradedEnvHints(zh, tool.SearchCommand(), !tool.GitAvailable())
}

// renderDegradedEnvHints is the pure renderer — split out from the
// live accessor so tests can drive the bilingual output matrix
// without stubbing tool package globals. Keeps the UI text localised
// using the same matcher the orchestrator uses; order is
// search-backend first, git second so the most user-visible
// degradation is rendered on the top line.
func renderDegradedEnvHints(zh bool, searchBackend string, gitMissing bool) []string {
	var hints []string
	warn := pterm.FgYellow.Sprint
	dim := pterm.FgDarkGray.Sprint
	switch searchBackend {
	case "grep":
		// ripgrep missing but GNU/BSD grep still usable — slow-ish,
		// not catastrophic. Nudge toward ripgrep without crying wolf.
		if zh {
			hints = append(hints, warn("· 搜索后端:grep")+dim(" (装 ripgrep 可进一步提速)"))
		} else {
			hints = append(hints, warn("· Search backend: grep")+dim(" (install ripgrep for faster scans)"))
		}
	case "native":
		// Both missing — Go regex walker. Correct but noticeably
		// slower on large repos.
		if zh {
			hints = append(hints, warn("· 搜索后端:Go 内置扫描器")+dim(" (装 ripgrep 可大幅提速)"))
		} else {
			hints = append(hints, warn("· Search backend: native Go scanner")+dim(" (install ripgrep for a 10× speedup)"))
		}
	}
	// "rg" → no hint
	if gitMissing {
		if zh {
			hints = append(hints, warn("· 未检测到 git")+dim(" (repomap 走文件遍历;git_diff / git_log 不可用)"))
		} else {
			hints = append(hints, warn("· git not detected")+dim(" (repomap falls back to filesystem walk; git_diff / git_log disabled)"))
		}
	}
	return hints
}

// memorySummaryLine returns a one-line dim-gray digest of the memory
// store's current state, or "" when the store is empty or absent.
// Format: "memory: <recent> recent turn(s) + <idx> compacted, <N> B total".
//
// Rendered in the banner instead of dumping the full history: the
// turns are already replayed into every dispatch via
// Store.BuildContext, so showing them verbatim in the banner would
// be duplicate noise. A short stat line lets the user see "yes, my
// previous N turns are still in play" without scrolling.
func (r *REPL) memorySummaryLine() string {
	if r.store == nil {
		return ""
	}
	recent := r.store.Recent()
	idx := r.store.Index()
	if len(recent) == 0 && len(idx) == 0 {
		return ""
	}
	bytes := 0
	for _, t := range recent {
		bytes += len(t.Request) + len(t.Response)
	}
	for _, e := range idx {
		bytes += len(e.Topic) + len(e.Summary)
	}
	if isZh(r.language) {
		return pterm.FgDarkGray.Sprintf("记忆: %d 条近期 + %d 条压缩, %s",
			len(recent), len(idx), humanByteSize(bytes))
	}
	return pterm.FgDarkGray.Sprintf("Memory: %d recent + %d compacted, %s",
		len(recent), len(idx), humanByteSize(bytes))
}

// humanByteSize renders a byte count in the smallest unit that keeps
// the integer part ≤ 3 digits. "1023 B", "9.4 KB", "2.1 MB".
func humanByteSize(n int) string {
	const (
		kb = 1024
		mb = 1024 * 1024
	)
	switch {
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
}

// inputTheme returns a minimal huh theme — no borders, no decorations,
// just a clean prompt and cursor.
func inputTheme() *huh.Theme {
	t := huh.ThemeBase()

	// Clean cursor and prompt styling.
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")) // bright cyan
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")).Bold(true) // bright cyan bold
	t.Focused.TextInput.Text = lipgloss.NewStyle()
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")) // dim gray

	// Remove all borders and decorations.
	t.Focused.Base = lipgloss.NewStyle().PaddingLeft(1)
	t.Focused.Title = lipgloss.NewStyle().Width(0).MaxWidth(0)
	t.Focused.Description = lipgloss.NewStyle().Width(0).MaxWidth(0)

	t.Blurred = t.Focused
	return t
}

// readInput reads a (possibly multi-line) request from the user.
// In interactive mode it delegates to a native real-cursor input
// session (readInputInteractive) that carries paste-folding, history,
// and slash-command autocomplete. When driven by an io.Reader (tests,
// pipes) it reads lines via bufio.Scanner unchanged.
//
// Returned display/expanded split: expanded is what reaches the
// orchestrator (placeholders inlined to their original pasted
// payload); display is what the user actually saw on their input line
// (placeholders kept compact). The Bubble Tea session echoes display
// itself via tea.Printf, so Loop() no longer needs to echo in the
// interactive branch.
func (r *REPL) readInput(prompt string) (string, error) {
	expanded, _, err := r.readInputPair(prompt)
	return expanded, err
}

// readInputPair returns (expanded, display, error). In line-oriented
// mode expanded and display are identical.
func (r *REPL) readInputPair(prompt string) (string, string, error) {
	if r.interactive() {
		return r.readInputInteractive(prompt)
	}
	s, err := r.readInputLines(prompt)
	return s, s, err
}

// readInputInteractive runs the interactive input session, looping for
// trailing-"\" continuation. Each invocation can fold its own pastes
// into independent placeholder slots.
func (r *REPL) readInputInteractive(prompt string) (string, string, error) {
	cur := prompt
	var expandedParts, displayParts []string
	for {
		res, err := r.readInputNative(cur, len(expandedParts) > 0)
		if errors.Is(err, errNativeInputUnavailable) {
			res, err = r.readInputBubble(cur, len(expandedParts) > 0)
		}
		if err != nil {
			return "", "", err
		}
		if res.aborted {
			return "", "", io.EOF
		}
		expandedParts = append(expandedParts, res.expanded)
		displayParts = append(displayParts, res.display)
		if !res.continues {
			expanded := strings.TrimSpace(strings.Join(expandedParts, "\n"))
			display := strings.TrimSpace(strings.Join(displayParts, "\n"))
			return expanded, display, nil
		}
		// Continuation: keep the sticky tag visible so a multi-line
		// plan-mode input does not lose the [task:write] marker after
		// the first line. Pre-fix dropped to a bare "…" and the user
		// could lose track mid-paste.
		cur = r.currentStickyTag() + "…"
	}
}

// scriptedScanner returns the single long-lived bufio.Scanner that
// reads from r.in in scripted mode. Two callers share it:
// readInputLines for normal lines and handle{Log,Paste}Cmd for the
// multi-line capture loops. Creating a fresh Scanner inside the
// capture helpers would conflict with readInputLines' prior read-
// ahead buffering (the capture's Scan() would see EOF because the
// outer scanner has already consumed bytes into its internal buffer).
// Caller must check r.in != nil first.
func (r *REPL) scriptedScanner() *bufio.Scanner {
	if r.scanner == nil {
		r.scanner = bufio.NewScanner(r.in)
		// Lift the token cap from bufio's 64 KiB default so a single
		// long pasted line (a minified JSON blob, a flattened stack
		// trace) doesn't silently truncate — matches the paste/log
		// capture ceiling enforced elsewhere.
		r.scanner.Buffer(make([]byte, 64*1024), r.attachedLogMaxBytes+1)
	}
	return r.scanner
}

// captureScanner is the bufio.Scanner used by /log paste and /paste
// for multi-line capture. In scripted mode it reuses r.scanner so
// reads pick up where readInputLines left off (a second scanner would
// miss any bytes the first has already buffered ahead). In
// interactive mode the Bubble Tea session has already quit and
// restored cooked mode, so a fresh os.Stdin scanner is fine — nothing
// else is reading from stdin during capture.
func (r *REPL) captureScanner() *bufio.Scanner {
	if r.in != nil {
		return r.scriptedScanner()
	}
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 64*1024), r.attachedLogMaxBytes+1)
	return s
}

// readInputLines reads from r.in using a scanner. Supports multi-line
// continuation (trailing \) and prints prompt/promptCont to r.out so
// test assertions can verify continuation behavior.
//
// `prompt` is the caller's per-turn prompt (already includes the
// sticky-state tag from promptStickyTag). Empty string falls back to
// the static r.prompt so confirmation callers that print their own
// title and just need a basic input echo keep working unchanged.
func (r *REPL) readInputLines(prompt string) (string, error) {
	r.scriptedScanner()
	if prompt == "" {
		prompt = r.prompt
	}
	if prompt != "" {
		fmt.Fprint(r.out, prompt)
	}
	var parts []string
	for r.scanner.Scan() {
		line := r.scanner.Text()
		if strings.HasSuffix(line, "\\") {
			parts = append(parts, strings.TrimSuffix(line, "\\"))
			// Print continuation prompt.
			if r.promptCont != "" {
				fmt.Fprint(r.out, r.promptCont)
			}
			continue
		}
		parts = append(parts, line)
		return strings.TrimSpace(strings.Join(parts, "\n")), nil
	}
	if err := r.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

func (r *REPL) setRendererTotalStagesForCurrentMode() {
	if r == nil || r.renderer == nil {
		return
	}
	// Mode-aware K/N denominator. Write modes always run the analyzer
	// first as a classifier, then the write controller drives the
	// per-mode lane:
	//
	//   ModePlan:   analyze + plan                  = 2
	//   ModeApply:  analyze + plan + apply + verify = 4
	//   ModeVerify: analyze + verify                = 2
	//
	// Read mode has four visible dispatch boundaries: analyze +
	// explore + extract + finalize.
	switch r.currentMode {
	case types.ModePlan:
		r.renderer.SetTotalStages(2)
	case types.ModeApply:
		r.renderer.SetTotalStages(4)
	case types.ModeVerify:
		r.renderer.SetTotalStages(2)
	default:
		r.renderer.SetTotalStages(4)
	}
}

// dispatch runs one user request through the orchestrator and prints
// the result, then records the turn in memory.
//
// line is the placeholder-expanded text that reaches the orchestrator
// (carries full pasted payloads). display is the placeholder-folded
// view the user actually saw; it is what gets persisted to memory so
// prior-conversation blocks in future turns stay compact regardless of
// paste size.
func (r *REPL) dispatch(line, display string) {
	// T1.1: auto-route pasted log content into AttachedLog so the
	// log_triage pre-stage parses it with the LLM instead of letting
	// the analyzer's TermGraph / keyword_search be poisoned by log
	// timestamps and stack-frame literals. Only fires when the user
	// has NOT already set an explicit log via /log — explicit > auto.
	if r.attachedLog == "" {
		cleaned, detected := splitPastedLog(line)
		if detected != "" {
			if len(detected) > r.attachedLogMaxBytes {
				r.warn("auto-detected log hit %d-byte cap; truncating\n", r.attachedLogMaxBytes)
				detected = detected[:r.attachedLogMaxBytes]
			}
			r.attachedLog = detected
			r.attachedLogAutoRouted = true
			r.info(fmt.Sprintf("auto-attached log: %d bytes (one-shot for this request; use /log to attach persistently across turns)", len(detected)))
			line = cleaned
			// Apply the same cleanup to display. When display holds a
			// paste placeholder (interactive bracketed paste), the
			// placeholder token doesn't match splitPastedLog's line-
			// oriented patterns so this is a no-op. When the log was
			// typed/streamed inline (scripted mode, terminals that
			// swallow bracketed paste), display == line and both get
			// trimmed identically.
			cleanedDisplay, _ := splitPastedLog(display)
			display = cleanedDisplay
			if strings.TrimSpace(line) == "" {
				// All the user typed was the log. Supply a minimal
				// placeholder so the analyzer still has a request
				// string; log_triage will drive the intent.
				line = "分析附带的日志"
			}
			if strings.TrimSpace(display) == "" {
				display = line
			}
		}
	}

	// One-shot semantics for auto-routed logs: after this turn
	// completes via any path (success, pipeline error, empty
	// response), drop the attachment so subsequent turns don't
	// re-run log_triage against the same bytes. Explicit /log is
	// unaffected; only the auto-routed bit triggers the clear.
	defer func() {
		if r.attachedLogAutoRouted {
			r.attachedLog = ""
			r.attachedLogAutoRouted = false
			if setter, ok := r.runner.(attachedLogSetter); ok {
				setter.SetAttachedLog("")
			}
		}
	}()

	if r.pendingCommandClarification != nil && r.currentMode == types.ModeRead && r.operationEnabled {
		r.resumeCommandOperationClarification(line, display)
		return
	}

	// Bare-directory write consent (pytest forensics #2, 2026-06-12):
	// a write-phase request against a target that still needs git
	// init asks for consent in place of the --auto-init-repo /
	// --allow-scaffold flag ceremony. Interactive REPL only; the
	// prompt must run BEFORE the spinner takes over the terminal.
	proceed, restoreBareDirAuth := r.preRunBareDirConsent()
	if !proceed {
		return
	}
	if restoreBareDirAuth != nil {
		defer restoreBareDirAuth()
	}

	spinnerStarted := false
	if r.renderer != nil {
		r.setRendererTotalStagesForCurrentMode()
		r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
		spinnerStarted = true
	}

	// Auto chit-chat classification. Opt-in; nil classifier means the
	// REPL falls back to explicit /chat only. An attached log (sticky
	// or auto-routed) is strong evidence of a code question, so the
	// classifier is skipped in that case to save an LLM call. Fail-
	// safe: any classifier error (nil adapter, chat error, unparseable
	// response, unknown decision) routes to the pipeline unchanged,
	// so a broken classifier cannot silently misroute real questions.
	// `/mode write|apply|verify` is an explicit user intent to drive
	// the write pipeline — skip the classifier so a "no repo context"
	// verdict cannot reroute a write-mode turn to chit-chat.
	// `/mode write|apply|verify` is an explicit user intent to drive
	// the write pipeline — skip the classifier so a "no repo context"
	// verdict cannot reroute a write-mode turn to chit-chat.
	//
	// Pre-T2.2 the gate also skipped when attachedLog / attachedHitrace
	// was sticky, on the theory that "user attached a log → must want
	// pipeline analysis." That hard-coded routing rule conflicts with
	// `feedback_no_custom_keyword_matching.md` (LLM mis-classification
	// is a prompt-quality issue, not a Go-side override). The
	// attachment signal is now passed to the classifier as part of
	// priorTurnHint so the LLM judges by content: a log-related code
	// question with attachedLog still routes to repo_question (LLM
	// reads the attachment + current message and decides), while a
	// pure memory-meta question that happens to follow a sticky
	// attached log correctly routes to chitchat (chitchat doesn't
	// consume the attachment).
	// presentationDirective carries a typed current-turn display
	// request into the runner via SetPresentationDirective. It can
	// come from route=hybrid (fresh repo + previous-answer transform)
	// or route=repo (fresh repo question whose own wording asks for a
	// specific view such as a diagram/table). Critical invariant:
	// `line` is the USER's authoritative phrasing — it must NOT be
	// overwritten with system-generated headers, because recordTurn
	// at the end of dispatch persists `line` into memory verbatim.
	// Mutating line would inject "## Presentation directive ..."
	// headers into BuildContext on every subsequent turn (#6). The
	// directive instead rides on a separate typed metadata channel.
	presentationDirective := ""
	turnRouteHint := types.TurnRouteHint{}
	hasAttach := r.attachedLog != "" || r.attachedHitrace != "" ||
		r.attachedLogAutoRouted

	activeUserMode := r.userMode.Normalize()
	if activeUserMode != UserModeAuto {
		if policy, ok := activeUserMode.TurnPolicy(); ok {
			policy = ApplyTurnPolicyGuards(policy, r.lastAnswerText() != "", hasAttach)
			turnRouteHint = TurnRouteHintFromPolicy(policy)
			debugLogTurnPolicy(policy)
			r.emitTurnPolicyAudit(policy)
			switch activeUserMode {
			case UserModeOperation:
				if !r.operationEnabled {
					r.operationUnavailableDispatch(line, display, policy)
					return
				}
				r.operationDispatch(line, display, policy)
				return
			case UserModeData:
				r.dataTaskDispatch(line, display, policy)
				return
			case UserModeCode:
				presentationDirective = policy.PresentationDirective
				logging.Info("[repl/turn_policy] explicit code mode → pipeline")
			}
		}
	}

	if activeUserMode == UserModeAuto && r.currentMode == types.ModeRead &&
		r.chitchatClassifier != nil && r.chitchatResponder != nil {
		// Build a compact 1-line hint from the most recent turn so
		// the classifier can disambiguate continuation references
		// (e.g. "expand 10" after a list_memory listing). Empty
		// hint on first turn / no recent buffer / nil store
		// preserves byte-identical pre-fix behaviour.
		hint := r.buildPriorTurnHint()
		// Add structured attachment signal so the LLM can route
		// based on whether the user is referencing the attachment.
		// Format mirrors the priorTurn shape: space-separated
		// key=value pairs the prompt teaches as load-bearing.
		if hasAttach {
			if hint != "" {
				hint += " attachment=true"
			} else {
				hint = "attachment=true"
			}
		}

		// Prefer the structured-policy classifier when the wired
		// implementation supports it. Production cmd/root.go's
		// *llmChitchatClassifier satisfies BOTH ChitchatClassifier
		// and TurnPolicyClassifier; tests that wire a stub-only-
		// Classify implementation transparently fall through to the
		// legacy binary path below.
		if tpc, ok := r.chitchatClassifier.(TurnPolicyClassifier); ok {
			lastAnswer := r.lastAnswerText()
			classifierCtx := r.startTurn()
			policy, err := tpc.ClassifyPolicy(classifierCtx, line, hint, lastAnswer != "")
			r.endTurn()
			if err != nil {
				logging.Warning("[repl/turn_policy] classifier error: %v — trying legacy binary classifier fallback", err)
				if r.tryLegacyChitchatFallback(line, display, hint, err) {
					return
				}
				r.info(turnPolicyClassifierFallbackHint(r.language))
			} else {
				rawPolicy := policy
				policy = ApplyTurnPolicyGuards(policy, lastAnswer != "", hasAttach)
				turnRouteHint = TurnRouteHintFromPolicy(policy)
				debugLogTurnPolicy(policy)
				if policy.Route == RouteOperation || policy.Route == RouteData {
					r.emitReplLLMTrace(tpc, "turn_policy_classifier", types.AgentName("turn_policy"), types.StageAnalyze)
					r.emitTurnPolicyAudit(policy)
				}
				switch policy.Route {
				case RouteLocal:
					if r.maybeDispatchCommandOperationFollowup(line, display, policy, rawPolicy) {
						return
					}
					r.localDispatch(line, display, policy, lastAnswer)
					return
				case RouteClarify:
					r.clarifyDispatch(line, display, policy)
					return
				case RouteOperation:
					if !r.operationEnabled {
						r.operationUnavailableDispatch(line, display, policy)
						return
					}
					r.operationDispatch(line, display, policy)
					return
				case RouteData:
					r.dataTaskDispatch(line, display, policy)
					return
				case RouteWrite:
					r.emitReplLLMTrace(tpc, "turn_policy_classifier", types.AgentName("turn_policy"), types.StageAnalyze)
					r.emitTurnPolicyAudit(policy)
					r.dispatchWithUserMode(line, display, UserModeWrite)
					return
				case RouteHybrid:
					if r.maybeDispatchCommandOperationFollowup(line, display, policy, rawPolicy) {
						return
					}
					// Carry the directive into the effective
					// request below — NEVER mutate `line`
					// (memory invariant; see top-of-block
					// comment).
					presentationDirective = policy.PresentationDirective
					logging.Info("[repl/turn_policy] hybrid → pipeline with directive=%q",
						oneLineClamp(presentationDirective, 80))
				case RouteRepo:
					if r.maybeDispatchCommandOperationFollowup(line, display, policy, rawPolicy) {
						return
					}
					presentationDirective = policy.PresentationDirective
					if strings.TrimSpace(presentationDirective) != "" {
						logging.Info("[repl/turn_policy] repo → pipeline with directive=%q (confidence=%.2f)",
							oneLineClamp(presentationDirective, 80), policy.Confidence)
					} else {
						logging.Info("[repl/turn_policy] repo → pipeline (confidence=%.2f)", policy.Confidence)
					}
				}
			}
		} else {
			// Legacy binary path. Preserved verbatim so test stubs
			// implementing ChitchatClassifier-only continue to work
			// byte-for-byte.
			classifierCtx := r.startTurn()
			isChat, err := r.chitchatClassifier.Classify(classifierCtx, line, hint)
			r.endTurn()
			if err != nil {
				logging.Warning("[repl/chitchat] classifier error: %v — falling back to pipeline", err)
			} else if isChat {
				logging.Info("[repl/chitchat] classifier routed turn to chit-chat: %s", oneLine(line))
				r.chitchatDispatch(line, display)
				return
			}
		}
	}

	prior := r.store.BuildContext(line, memory.BuildOpts{
		Kind:      memory.KindPipeline,
		SessionID: r.sessionID,
	})
	// Effective request = optional prior-conversation block + ORIGINAL line.
	// The presentation directive is propagated out-of-band below so status
	// lines, repo_map task maps, and memory stay user-authentic.
	effective := composeEffectiveRequest(prior, line)

	logging.Info("[repl] dispatching request: %s", oneLine(line))

	// Propagate the typed current-turn display directive. Always call the
	// setter, including with "", so a reused orchestrator cannot leak a
	// previous turn's diagram/table preference into the next request.
	if setter, ok := r.runner.(presentationDirectiveSetter); ok {
		setter.SetPresentationDirective(presentationDirective)
	}
	if setter, ok := r.runner.(turnRouteHintSetter); ok {
		setter.SetTurnRouteHint(turnRouteHint)
	}
	// Persist the expanded current request in output artifacts. Keep
	// this out-of-band from the model request: `effective` may include
	// prior-conversation context, while `display` may include folded
	// paste placeholders for REPL memory compactness.
	if setter, ok := r.runner.(outputTranscriptRequestSetter); ok {
		setter.SetOutputTranscriptRequest(line)
		defer setter.SetOutputTranscriptRequest("")
	}

	// Propagate sticky attached-log to the runner. Runners without
	// SetAttachedLog simply skip this step (tests).
	if setter, ok := r.runner.(attachedLogSetter); ok {
		setter.SetAttachedLog(r.attachedLog)
	}
	// Same propagation for the perf channel.
	if setter, ok := r.runner.(attachedHitraceSetter); ok {
		setter.SetAttachedHitrace(r.attachedHitrace)
	}
	if setter, ok := r.runner.(attachedHitraceSourceSetter); ok {
		setter.SetAttachedHitraceSource(r.attachedHitraceSource)
	}

	// Propagate sticky pipeline mode. Runners without SetMode
	// (test stubs) fall through to the pre-B0 read-only path
	// regardless of r.currentMode — the user's /mode selection is
	// ignored silently there, which is the correct degradation
	// because a test runner that does not implement SetMode was
	// not designed to exercise write-mode behaviour.
	if setter, ok := r.runner.(modeSetter); ok {
		setter.SetMode(r.currentMode)
	}

	autoResumeRunID, clearAutoResumeSeed := r.installReadRunAutoResumeSeed(effective)
	if clearAutoResumeSeed != nil {
		defer clearAutoResumeSeed()
	}
	if autoResumeRunID != "" {
		logging.Info("[repl/read-runs] auto-resume candidate installed: run=%s", autoResumeRunID)
	}

	if r.renderer != nil {
		r.setRendererTotalStagesForCurrentMode()
		if !spinnerStarted {
			r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
		}
	}

	busCtx, err := r.runInFlightWrap(func() (*types.BusContext, error) {
		return r.runner.Run(effective, r.repoRoot, r.branch)
	})

	if r.renderer != nil {
		// Red line: StopSpinner BEFORE printing response so the dock
		// is gone before RenderResult runs. paintDock + answer
		// markdown writes can't share the same stdout stream.
		armDockTerminalState(r.renderer, err)
		r.renderer.StopSpinner()
	}

	if err != nil {
		logging.Error("[repl] orchestrator error: %v", err)
		// Translate well-known underlying errors into user-actionable
		// text. "context canceled" surfacing as raw stream-level error
		// gives no recovery hint; friendlyRunError says "interrupted"
		// or "timed out" so the user knows whether to retry vs report.
		r.errorf("error: %s\n", friendlyRunError(r.language, err))
		return
	}

	var postAnswerHints []string

	// Plan-mode auto-save. When Mode=ModePlan produced a
	// ChangePlan, persist it via PlanStore so /plan show + /plan
	// clear survive subsequent turns. Failure is non-fatal — the
	// plan summary still prints to stdout, the error goes to the
	// REPL's warn channel. PlanStore nil (test runners, planless
	// configs) silently skips this path.
	if r.planStore != nil && busCtx != nil && busCtx.Mutable != nil &&
		busCtx.Mode == types.ModePlan {
		if plan := busCtx.Mutable.ChangePlan(); plan != nil {
			if path, saveErr := r.planStore.Save(plan); saveErr != nil {
				logging.Warning("[repl] plan auto-save failed: %v", saveErr)
				r.warn("plan auto-save failed: %v\n", saveErr)
			} else {
				r.pendingPlanPath = path
				postAnswerHints = append(postAnswerHints, fmt.Sprintf("plan saved: %s", path))
				// Action nudge: surface the next-step slash commands so
				// the user does not have to remember /approve / /reject.
				// Keep the nudge BELOW the answer panel: if printed here
				// immediately, it lands above the bordered plan summary and
				// users miss it after reading downward.
				postAnswerHints = append(postAnswerHints, planReadyNudge(r.language, plan.ID, len(plan.Changes))...)
				// Multi-phase signal (commit 41 UX#1): when the
				// LLM emitted a sequential PhaseProposal with
				// >=2 phases, surface that fact at plan-emit
				// time so the operator knows /approve will
				// drive N phases + /phase show is the
				// inspection tool. Single-phase plans skip.
				if ir := busCtx.Mutable.WriteAnalysisIR(); ir != nil &&
					ir.PhaseProposal.Split == "sequential" &&
					len(ir.PhaseProposal.Phases) >= 2 {
					postAnswerHints = append(postAnswerHints, planReadyMultiPhaseNudge(r.language, len(ir.PhaseProposal.Phases))...)
				}
			}
		}
	}

	response := strings.TrimSpace(r.render(busCtx))
	// Log fidelity: INFO captures the agent's raw markdown so a
	// later audit reads cleanly without ANSI escapes (the rendered
	// `response` carries dozens of \x1b[...m sequences from glamour
	// that turn the log into noise). The rendered version is still
	// available at DEBUG for users who want to see what landed on
	// the user's terminal verbatim.
	rawMarkdown := ""
	if busCtx != nil && busCtx.Mutable != nil {
		rawMarkdown = strings.TrimSpace(busCtx.Mutable.Result())
	}
	if rawMarkdown != "" {
		logging.Info("[repl] final answer (len=%d):\n%s", len(rawMarkdown), rawMarkdown)
	} else {
		// Pipeline returned without a Mutable.Result (error path or
		// empty). Fall back to the rendered string so the log line
		// at INFO level is never silent — users still see SOMETHING
		// at the default level.
		logging.Info("[repl] final answer (rendered, no raw available):\n%s", response)
	}
	logging.Debug("[repl] final answer (rendered for terminal):\n%s", response)

	// Decide what to persist in memory. When the pipeline ended with
	// a TaskState.LastError, the rendered response includes an
	// "error: ..." prefix carrying the internal error text (task UUIDs,
	// oscillation guard messages, OpenAI 400 payloads, etc.). Persisting
	// that verbatim pollutes every subsequent REPL turn's
	// "### Recent conversation" block — the LLM sees historical errors
	// in its prior-conversation context and may be swayed into thinking
	// those failures are current state. The user still sees the full
	// error in their terminal via the rendering below; only what
	// reaches memory is sanitized.
	//
	// See memory/project_repl_memory_error_pollution.md for the
	// diagnostic trail.
	memResponse := response
	if rawMarkdown != "" {
		// Memory is model-facing context for later turns, so keep the
		// authoritative markdown answer rather than terminal-only
		// presentation transforms such as Mermaid-to-ASCII rendering.
		memResponse = rawMarkdown
	}
	if busCtx != nil && busCtx.TaskState.LastError != "" {
		memResponse = "(previous attempt ended in error — details omitted from memory)"
	}

	// Skip rendering if no meaningful content.
	if response == "" || response == "(no result)" {
		fmt.Fprintln(r.out, emptyResponseHint(r.language))
		for _, line := range postAnswerHints {
			r.info(line)
		}
		r.lastAnswerOrigin = replAnswerOriginPipeline
		r.recordTurn(display, line, memResponse, memory.KindPipeline)
		return
	}

	r.renderBordered(response)
	for _, line := range postAnswerHints {
		r.info(line)
	}
	if hint := finalAnswerMarkdownNotice(busCtx, r.language); hint != "" {
		r.info(hint)
	}
	if hint := finalAnswerHTMLNotice(busCtx, r.language); hint != "" {
		r.info(hint)
	}
	if hint := r.finalAnswerMarkdownPreviewNotice(busCtx); hint != "" {
		r.info(hint)
	}
	r.lastAnswerOrigin = replAnswerOriginPipeline
	r.recordTurn(display, line, memResponse, memory.KindPipeline)
}

func (r *REPL) tryLegacyChitchatFallback(line, display, hint string, cause error) bool {
	if r.chitchatClassifier == nil {
		return false
	}
	classifierCtx := r.startTurn()
	isChat, err := r.chitchatClassifier.Classify(classifierCtx, line, hint)
	r.endTurn()
	if err != nil {
		logging.Warning("[repl/chitchat] legacy fallback classifier error after turn-policy failure (%v): %v", cause, err)
		return false
	}
	if isChat {
		logging.Info("[repl/chitchat] legacy fallback routed turn to chit-chat after turn-policy failure: %s", oneLine(line))
		r.chitchatDispatch(line, display)
		return true
	}
	logging.Info("[repl/chitchat] legacy fallback classified as repo question after turn-policy failure")
	return false
}

func finalAnswerMarkdownNotice(busCtx *types.BusContext, lang string) string {
	if busCtx == nil || busCtx.Mutable == nil {
		return ""
	}
	return markdownTranscriptNotice(busCtx.Mutable.FinalAnswerMarkdownPath(), lang)
}

func finalAnswerHTMLNotice(busCtx *types.BusContext, lang string) string {
	if busCtx == nil || busCtx.Mutable == nil {
		return ""
	}
	return htmlTranscriptNotice(busCtx.Mutable.FinalAnswerHTMLPath(), lang)
}

func (r *REPL) finalAnswerMarkdownPreviewNotice(busCtx *types.BusContext) string {
	if r == nil || r.markdownPreview == nil || busCtx == nil || busCtx.Mutable == nil {
		return ""
	}
	return r.markdownPreviewNotice(busCtx.Mutable.FinalAnswerMarkdownPath())
}

func (r *REPL) writeLocalMarkdownTranscript(request, answer string) outputdump.Result {
	if r == nil || strings.TrimSpace(answer) == "" || strings.TrimSpace(r.outputDumpDir) == "" {
		return outputdump.Result{}
	}
	request = strings.TrimSpace(request)
	return outputdump.WriteResult(outputdump.Args{
		Dir:        r.outputDumpDir,
		Max:        r.outputDumpMax,
		Language:   r.language,
		Request:    request,
		Answer:     answer,
		HasLog:     strings.TrimSpace(r.attachedLog) != "",
		LogBytes:   len(r.attachedLog),
		HasTrace:   strings.TrimSpace(r.attachedHitrace) != "",
		TraceBytes: len(r.attachedHitrace),
		RuntimeArtifacts: outputdump.MergeRuntimeArtifacts(
			outputdump.RuntimeArtifactsFromRequest(request),
			outputdump.RuntimeArtifactsFromAttachment("log", r.attachedLog),
			outputdump.RuntimeArtifactsFromAttachment("trace", r.attachedHitrace),
		),
		Now: time.Now(),
		PID: os.Getpid(),
	})
}

func (r *REPL) emitMarkdownTranscriptHints(result outputdump.Result) {
	if hint := markdownTranscriptNotice(result.MarkdownPath, r.language); hint != "" {
		r.info(hint)
	}
	if hint := htmlTranscriptNotice(result.HTMLPath, r.language); hint != "" {
		r.info(hint)
	}
	if hint := r.markdownPreviewNotice(result.MarkdownPath); hint != "" {
		r.info(hint)
	}
}

func markdownTranscriptNotice(path, lang string) string {
	path = displayFinalAnswerMarkdownPath(path)
	if path == "" {
		return ""
	}
	if isZh(lang) {
		return "Markdown 原文已保存：" + path
	}
	return "Raw Markdown saved: " + path
}

func htmlTranscriptNotice(path, lang string) string {
	path = displayFinalAnswerMarkdownPath(path)
	if path == "" {
		return ""
	}
	if isZh(lang) {
		return "HTML 单页已保存：" + path
	}
	return "Standalone HTML saved: " + path
}

func (r *REPL) markdownPreviewNotice(path string) string {
	if r == nil || r.markdownPreview == nil {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	previewURL, err := r.markdownPreview.RegisterMarkdown(path)
	if err != nil {
		logging.Warning("[markdown_preview] register %s failed: %v", path, err)
		return ""
	}
	previewURL = strings.TrimSpace(previewURL)
	if previewURL == "" {
		return ""
	}
	if isZh(r.language) {
		return "浏览器预览：" + previewURL
	}
	return "Browser preview: " + previewURL
}

func displayFinalAnswerMarkdownPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return path
	}
	return rel
}

// renderRichResponse runs `text` through the full presentation
// pipeline the pipeline-dispatch path already gets via
// render.Renderer.RenderResult: first
// render.RenderMermaidBlocks (turns ```mermaid``` fences into
// aligned ASCII grids), then the glamour-backed
// Renderer.RenderMarkdown (styles headings / bold / lists /
// tables / fenced code).
//
// Pre-2026-04-30 the local + chitchat dispatch paths called
// renderBordered directly on the LLM's raw markdown source, so
// users saw `## Heading` / `**bold**` / `| col |` literals AND
// raw ```mermaid``` source instead of styled output — the bug
// reported as "图表/markdown 未渲染". Pipeline path was
// unaffected because RenderResult already wires both passes.
//
// Nil-renderer safe: when Config.Renderer is unset (tests, scripts
// that bypass cmd/), the function still runs RenderMermaidBlocks
// (which is a self-contained package function) and skips the
// glamour pass — preserves byte-identical behaviour for every
// existing test fixture that asserted on the raw response shape.
func (r *REPL) renderRichResponse(text string) string {
	text = render.RenderMermaidBlocks(text)
	if r.renderer != nil {
		text = r.renderer.RenderMarkdown(text)
	}
	return text
}

// renderBordered prints model output with a continuous left border.
// Shared by the pipeline dispatch path and the /chat chitchat path so
// both answer surfaces get identical visual treatment. Prose trailing
// ANSI escapes and whitespace are stripped so the bar aligns cleanly;
// visual blocks keep their physical rows byte-for-byte because
// trimming/wrapping diagrams and tables corrupts the user's content.
func (r *REPL) renderBordered(response string) {
	r.renderBorderedWithTrailingBlank(response, true)
}

func (r *REPL) renderBorderedCompact(response string) {
	r.renderBorderedWithTrailingBlank(response, false)
}

func (r *REPL) renderBorderedMutedCompact(response string) {
	r.renderBorderedStyled(response, false, replQuietWhite, replQuietWhite)
}

func (r *REPL) renderBorderedDataProcessCompact(response string) {
	r.renderBorderedStyled(response, false, replQuietWhite, replQuietWhite)
}

func (r *REPL) renderBorderedWithTrailingBlank(response string, trailingBlank bool) {
	r.renderBorderedStyled(response, trailingBlank, replWhite, nil)
}

type replLineStyle func(string) string

func replWhite(s string) string {
	return replPtermOrANSI(pterm.FgWhite.Sprint(s), s, "\x1b[97m")
}

func replGray(s string) string {
	return replPtermOrANSI(pterm.FgGray.Sprint(s), s, "\x1b[37m")
}

func replQuietWhite(s string) string {
	return replPtermOrANSI(pterm.NewStyle(pterm.FgWhite, pterm.Fuzzy).Sprint(s), s, "\x1b[2;97m")
}

func replDarkGray(s string) string {
	return replPtermOrANSI(pterm.NewStyle(pterm.FgDarkGray).Sprint(s), s, "\x1b[90m")
}

func replPtermOrANSI(styled, plain, sgr string) string {
	if styled != plain {
		return styled
	}
	if plain == "" {
		return plain
	}
	return sgr + plain + "\x1b[0m"
}

func (r *REPL) renderBorderedStyled(response string, trailingBlank bool, borderStyle, contentStyle replLineStyle) {
	body := r.borderedResponseBodyStyled(response, trailingBlank, borderStyle, contentStyle)
	if r.renderer != nil {
		r.renderer.EmitScrollbackBlock(body)
		return
	}
	fmt.Fprint(r.out, body)
}

func (r *REPL) borderedResponseBody(response string, trailingBlank bool) string {
	return r.borderedResponseBodyStyled(response, trailingBlank, replWhite, nil)
}

func (r *REPL) borderedResponseBodyStyled(response string, trailingBlank bool, borderStyle, contentStyle replLineStyle) string {
	lines := borderedResponseLines(response)
	color := r.replColorEnabled()
	if color {
		pterm.EnableColor()
	} else {
		pterm.DisableColor()
	}
	bar := "│"
	if color && borderStyle != nil {
		bar = borderStyle(bar)
	}
	// Wrap each line to fit the terminal, accounting for the
	// "  │ " prefix (4 display cols + 2 margin).
	maxContent := 60
	if w, _, werr := term.GetSize(int(os.Stdout.Fd())); werr == nil && w > 10 {
		maxContent = w - 6
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s\n", bar)
	for _, ln := range lines {
		wrapped := borderedLineFragmentsPreserve(ln.text, maxContent, ln.visual)
		for _, wl := range wrapped {
			visualOverflow := (ln.visual || shouldPreserveVisualLine(wl)) && displayWidth(wl) > maxContent
			if color && contentStyle != nil && strings.TrimSpace(wl) != "" {
				wl = contentStyle(wl)
			}
			b.WriteString(borderedContentLine(bar, wl, visualOverflow && terminalAutoWrapControlSupported(r.out)))
		}
	}
	if trailingBlank {
		fmt.Fprintf(&b, "  %s\n\n", bar)
		return b.String()
	}
	fmt.Fprintf(&b, "  %s\n", bar)
	return b.String()
}

func (r *REPL) applyPtermColorMode() {
	if r.replColorEnabled() {
		pterm.EnableColor()
		return
	}
	pterm.DisableColor()
}

func (r *REPL) replColorEnabled() bool {
	return render.ResolveColorMode(r.colorMode, r.colorWriter()) != render.ColorNever
}

func (r *REPL) colorWriter() io.Writer {
	if r != nil && r.out != nil {
		return r.out
	}
	return os.Stdout
}

func (r *REPL) writeBorderedContentLine(bar, line string, visual bool, maxContent int) {
	fmt.Fprint(r.out, borderedContentLine(bar, line, visual && displayWidth(line) > maxContent && terminalAutoWrapControlSupported(r.out)))
}

func borderedContentLine(bar, line string, disableAutoWrap bool) string {
	if disableAutoWrap {
		// Visual rows are atomic. Letting the terminal auto-wrap a
		// diagram/table row corrupts its columns and edge routing, so
		// temporarily disable DECAWM for this one physical write. The
		// row content is still emitted verbatim; no clipping or width
		// rewriting happens in Codrax.
		return fmt.Sprintf("\x1b[?7l  %s %s\x1b[?7h\r\n", bar, line)
	}
	return fmt.Sprintf("  %s %s\n", bar, line)
}

func terminalAutoWrapControlSupported(w io.Writer) bool {
	if f, ok := w.(*os.File); ok && f != nil {
		return term.IsTerminal(int(f.Fd()))
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// recordTurn persists the user-visible form of a request plus the
// sanitized response into memory. display is what gets stored so
// prior-conversation blocks in future BuildContext calls stay
// compact; expanded is the paste-expanded form handed to the
// summarizer at compaction time so the resulting IndexEntry
// keywords/topic reflect actual paste content, not the
// "[Pasted text #N]" placeholder. expanded==display in scripted
// mode and when no paste happened.
//
// kind labels the turn as chitchat or pipeline so BuildContext can
// later pick the right retrieval policy for a follow-up of the same
// kind. The session id is the current REPL's r.sessionID — same for
// every turn in one REPL lifetime — so BuildContext can session-pin
// recent turns regardless of keyword match.
func (r *REPL) recordTurn(request, expanded, response string, kind memory.Kind) {
	turn := memory.Turn{
		ID:        fmt.Sprintf("turn-%d", time.Now().UnixNano()),
		Request:   request,
		Response:  response,
		Timestamp: time.Now(),
		SessionID: r.sessionID,
		Kind:      kind,
	}
	if expanded != "" && expanded != request {
		turn.RequestForSummary = expanded
	}
	if err := r.store.Append(turn); err != nil {
		logging.Warning("[repl] memory append failed: %v", err)
	}
}

// DefaultAttachedLogMaxBytes is the out-of-the-box 512 MiB cap on
// every REPL attach surface (/log + /htrace). Consumed by New when
// Config.AttachedLogMaxBytes is not set; the cmd layer populates
// Config from codrax.yaml :: log_attach_max_bytes so both CLI and
// REPL paths honour the same override. Mirrors
// cmd/root.go :: defaultAttachedLogMaxBytes.
// DefaultAttachedLogMaxBytes is the REPL-side default cap. Mirrors
// cmd.defaultAttachedLogMaxBytes — the two constants must agree so
// a unit test that bypasses initApp sees the same baseline as a
// real CLI run. Raised from 50 MiB → 256 MiB in 2026-05 and then
// to 512 MiB in 2026-06 to match systrace / perfetto / large hilog
// captures while keeping the
// ingestion hard ceiling in place.
const DefaultAttachedLogMaxBytes = 512 * 1024 * 1024 // 512 MiB

// handleSlash returns true if the loop should exit.
func (r *REPL) handleSlash(line string) bool {
	cmd := strings.Fields(line)[0]
	switch cmd {
	case "/log":
		r.handleLogCmd(line)
		return false
	case "/htrace":
		// /atrace is an Android-flavored alias of /htrace; the
		// alias is resolved by NormalizeREPLCommandAlias BEFORE
		// handleSlash runs, so by the time we reach this dispatch
		// the line has already been canonicalised to /htrace.
		// Both spellings hit this single case as a result.
		r.handleHitraceCmd(line)
		return false
	case "/paste":
		r.handlePasteCmd()
		return false
	case "/chat":
		args := strings.TrimSpace(strings.TrimPrefix(line, cmd))
		if args == "" {
			r.info("/chat <message> — reply without invoking the analysis pipeline")
			return false
		}
		r.chitchatDispatch(args, args)
		return false
	case "/code":
		r.handleOneShotUserModeCmd(line, cmd, UserModeCode)
		return false
	case "/op":
		r.handleOneShotUserModeCmd(line, cmd, UserModeOperation)
		return false
	case "/data":
		r.handleOneShotUserModeCmd(line, cmd, UserModeData)
		return false
	case "/write":
		r.handleOneShotUserModeCmd(line, cmd, UserModeWrite)
		return false
	case "/mode":
		r.handleModeCmd(line)
		return false
	case "/plan":
		r.handlePlanCmd(line)
		return false
	case "/approve":
		r.handleApproveCmd(line)
		return false
	case "/reject":
		r.handleRejectCmd(line)
		return false
	case "/operation":
		r.handleOperationCmd(line)
		return false
	case "/workflow":
		r.handleWorkflowCmd(line)
		return false
	case "/read-runs":
		r.handleReadRunsCmd(line)
		return false
	case "/verify":
		r.handleVerifyCmd(line)
		return false
	case "/worktree":
		r.handleWorktreeCmd(line)
		return false
	case "/merge":
		r.handleMergeCmd(line)
		return false
	case "/branch":
		r.handleBranchCmd(line)
		return false
	case "/env":
		r.handleEnvCmd(line)
		return false
	case "/baseline":
		r.handleBaselineCmd(line)
		return false
	case "/phase":
		r.handlePhaseCmd(line)
		return false
	case "/pitfalls":
		r.handlePitfallsCmd(line)
		return false
	case "/mermaid":
		r.handleMermaidCmd(line)
		return false
	case "/cancel":
		// Slash-command fallback for terminals where Ctrl+C is
		// swallowed by tmux, screen, or a terminal multiplexer the
		// operator can't reconfigure. Emits the same Cancel(reason)
		// the Ctrl+C path emits, so the rendering / state-saving
		// behaviour is identical.
		r.handleCancelCmd(line)
		return false
	case "/repos":
		r.handleReposCmd(line)
		return false
	case "/exit", "/quit":
		fmt.Fprintln(r.out, "  Goodbye!")
		return true
	case "/clear":
		peers, perr := r.store.LivePeerCount()
		if perr != nil {
			logging.Warning("[repl] live peer count failed: %v", perr)
		}
		msg := memoryClearConfirmTitle(r.language, peers)
		confirmed := false
		if r.interactive() {
			// Affirmative / Negative labels EMBED the hotkey hint so
			// the operator sees the y/n keyboard shortcut without
			// consulting docs. huh.NewConfirm binds the first
			// alphanumeric character of each label as the hotkey by
			// default — putting "[Y]" first preserves that contract
			// while making the shortcut visible. Bilingual via
			// memoryClearConfirmAffirmative / Negative so both label
			// + title respect r.language.
			err := huh.NewConfirm().
				Title(msg).
				Affirmative(memoryClearConfirmAffirmative(r.language)).
				Negative(memoryClearConfirmNegative(r.language)).
				Value(&confirmed).
				Run()
			if err != nil {
				confirmed = false
			}
		} else {
			// Line-oriented mode: print message and read y/n.
			fmt.Fprintln(r.out, msg)
			fmt.Fprintln(r.out, memoryClearConfirmLineHint(r.language))
			line, err := r.readInputLines("")
			if err == nil {
				confirmed = strings.TrimSpace(strings.ToLower(line)) == "y"
			}
		}
		if !confirmed {
			r.info(memoryClearCancelled(r.language))
			break
		}
		if err := r.store.Clear(); err != nil {
			r.errorf("clear failed: %v\n", err)
		} else {
			if err := r.clearOperationContextForClear(); err != nil {
				logging.Warning("[repl/operation] clear operation context failed: %v", err)
			}
			r.success(memoryClearedMsg(r.language))
		}
	case "/history":
		recent := r.store.Recent()
		idx := r.store.Index()
		planRows := r.collectPlanHistory()
		if len(recent) == 0 && len(idx) == 0 && len(planRows) == 0 {
			r.info(memoryEmpty(r.language))
			return false
		}
		if len(idx) > 0 {
			fmt.Fprintln(r.out, "  compacted index:")
			for _, e := range idx {
				fmt.Fprintf(r.out, "    - [%s] %s — keywords: %s\n", e.ID, e.Topic, strings.Join(e.Keywords, ", "))
			}
		}
		if len(recent) > 0 {
			fmt.Fprintln(r.out, "  recent turns:")
			for _, t := range recent {
				kindLabel := ""
				if t.Kind != "" {
					kindLabel = " [" + string(t.Kind) + "]"
				}
				fmt.Fprintf(r.out, "    - [%s]%s %s\n",
					t.Timestamp.Format("15:04:05"), kindLabel, oneLine(t.Request))
			}
		}
		if len(planRows) > 0 {
			fmt.Fprintln(r.out, "  plans applied/attempted:")
			for _, row := range planRows {
				fmt.Fprintln(r.out, "    - "+row)
			}
		}
	case "/compact":
		if err := r.store.Compact(); err != nil {
			r.errorf("compact failed: %v\n", err)
		} else {
			r.success(fmt.Sprintf("compaction done. recent=%d index=%d", len(r.store.Recent()), len(r.store.Index())))
		}
	case "/version":
		r.info(fmt.Sprintf("codrax %s (built %s)", r.version, r.buildTime))
	case "/help":
		fields := strings.Fields(line)
		lines := helpLines(r.language)
		if len(fields) > 1 {
			switch strings.ToLower(fields[1]) {
			case "all", "full", "--all":
				// Full help is still generated from slashCommands so
				// autocomplete and operator documentation cannot drift.
				lines = helpLinesAll(r.language)
			}
		}
		for _, line := range lines {
			r.info(line)
		}
	default:
		r.warn("%s", unknownSlashCommand(r.language, cmd))
	}
	return false
}

// handleModeCmd dispatches the `/mode` subcommands. Recognised forms:
//
//	/mode                                — print current user mode
//	/mode auto|code|operation|data|write — set sticky user mode
//
// The user mode is sticky for the REPL session. Auto preserves classifier-
// driven routing. code/operation/data/write are explicit escape hatches and do
// not parse user prose. Write mode maps to the controller Auto Pilot apply
// lane: the controller can explore, plan, apply in a worktree, verify, and
// replan while risk/approval/worktree gates keep the boundary typed.
func (r *REPL) handleModeCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/mode"))
	if rest == "" {
		r.info(currentUserModeMsg(r.language, string(r.userMode.Normalize())))
		return
	}
	target, err := ParseUserMode(rest)
	if err != nil {
		r.warn("%s", unknownModeValue(r.language, rest))
		return
	}
	target = target.Normalize()
	if target == UserModeWrite && !r.canEnterWriteWorkflow("/mode write") {
		return
	}
	r.userMode = target
	if target == UserModeWrite {
		r.currentMode = types.ModeApply
	} else {
		r.currentMode = types.ModeRead
	}
	r.success(modeSwitched(r.language, string(target)))
	// Workflow hint: explain in 1-3 lines what the new mode actually
	// does. Empty for ModeRead (no special workflow). Surfaced once per
	// /mode transition so a user new to write mode does not have to
	// read the docs to understand what the automatic lane will do.
	for _, line := range modeWorkflowHint(r.language, string(target)) {
		r.info(line)
	}
}

// canEnterWriteWorkflow centralizes the write Auto Pilot entry gate used by
// /mode write, /write <request>, and structured auto-route=write. It only
// authorizes entering the controller; apply/merge still go through typed
// approval, risk, and worktree gates.
func (r *REPL) canEnterWriteWorkflow(command string) bool {
	if !r.writeEnabled {
		for _, line := range writeModeDisabled(r.language, command, r.settingsPath) {
			r.warn("%s\n", line)
		}
		return false
	}
	if r.planStore != nil {
		if blocker, ok := r.detectUnsettledPlan(); ok {
			for _, line := range unsettledModePlanReject(r.language, blocker.ID, blocker.Status) {
				r.info(line)
			}
			return false
		}
	}
	return true
}

func (r *REPL) handleOneShotUserModeCmd(line, cmd string, mode UserMode) {
	args := strings.TrimSpace(strings.TrimPrefix(line, cmd))
	if args == "" {
		r.info(oneShotUserModeUsage(r.language, cmd, mode))
		return
	}
	r.dispatchWithUserMode(args, args, mode)
}

func (r *REPL) dispatchWithUserMode(line, display string, mode UserMode) {
	mode = mode.Normalize()
	if mode == UserModeWrite && !r.canEnterWriteWorkflow("/write") {
		return
	}
	oldUserMode := r.userMode
	oldPipelineMode := r.currentMode
	r.userMode = mode
	if mode == UserModeWrite {
		r.currentMode = types.ModeApply
	} else {
		r.currentMode = types.ModeRead
	}
	defer func() {
		r.userMode = oldUserMode
		r.currentMode = oldPipelineMode
	}()
	r.dispatch(line, display)
}

func (r *REPL) clearOperationContextForClear() error {
	r.pendingOperation = nil
	r.pendingCommandClarification = nil
	r.pendingProviderOperation = nil
	r.providerWorkflow = nil
	r.operationHistory = nil
	r.operationResults = nil
	r.providerOperationResults = nil
	r.lastAnswerOrigin = ""
	r.clearPendingOperationState()
	if r.operationMemory != nil {
		return r.operationMemory.Clear()
	}
	return nil
}

// handlePlanCmd dispatches the `/plan` subcommands. Recognised forms:
//
//	/plan                       — synonym for /plan show
//	/plan show                  — display the current pending plan
//	/plan show <plan-id>        — display a specific plan from PlanStore
//	                              (any status; useful after /plan list)
//	/plan clear                 — remove the pending plan's file
//	/plan list                  — enumerate every saved plan (newest first)
//
// All subcommands require a non-nil PlanStore (cmd/root.go wires
// one for the real REPL; tests that bypass cmd/ leave it nil and
// see a "/plan disabled" message).
func (r *REPL) handlePlanCmd(line string) {
	if r.planStore == nil {
		r.info(commandDisabled(r.language, "/plan", noPlanStoreReason(r.language)))
		return
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/plan"))
	if rest == "" {
		rest = "show"
	}
	// `/plan show --summary` strips the diff preview and renders a
	// path | kind | one-line rationale table instead. Useful for
	// big plans where the unified diff would scroll past the
	// terminal. Detect + strip the flag before the existing
	// "show <id>" path so they compose: `/plan show --summary` and
	// `/plan show <id> --summary` both work.
	summaryOnly := false
	if trimmed := strings.TrimSpace(strings.TrimSuffix(rest, "--summary")); trimmed != rest {
		summaryOnly = true
		rest = strings.TrimSpace(trimmed)
		if rest == "" {
			rest = "show"
		}
	}
	// /plan show <plan-id> — explicit target. Falls through to the
	// existing show path with the loaded plan's ID; r.pendingPlanPath
	// is rebound to the named plan so subsequent /approve targets the
	// same plan without retyping the ID.
	if showID := strings.TrimSpace(strings.TrimPrefix(rest, "show ")); showID != rest && showID != "" {
		// Cross-command nudge: a "group-" prefix is a PlanGroup
		// id, not a ChangePlan id. Point the operator at /phase
		// show before the load fails with a generic "not found".
		if strings.HasPrefix(showID, "group-") {
			r.info(fmt.Sprintf("plan show: %q is a plan-group id; use `/phase show %s` instead\n", showID, showID))
			return
		}
		full, err := r.planStore.Load(showID)
		if err != nil || full == nil {
			r.errorf("plan show: %s", planNotFound(r.language, showID))
			return
		}
		r.pendingPlanPath = filepath.Join(r.planStore.PlanDir(), showID+".json")
		rest = "show"
	}
	// /plan clear <plan-id|--all|--status=X> — extends the bare
	// `/plan clear` (which only drops the pending pointer) to let
	// the user delete a specific plan from PlanStore by ID, or
	// bulk-wipe by status. Bulk paths require interactive y/N
	// confirmation so a stray scripted `/plan clear --all` can't
	// silently nuke history.
	if clearArg := strings.TrimSpace(strings.TrimPrefix(rest, "clear ")); clearArg != rest && clearArg != "" {
		r.handlePlanClearArg(clearArg)
		return
	}
	switch rest {
	case "show":
		if r.pendingPlanPath == "" {
			// Recovery: an earlier /approve may have failed and dropped
			// the in-session pointer (older builds), or this REPL just
			// started in a dir whose PlanStore already has a pending
			// plan from a prior session. Walk PlanStore.List for the
			// most recent Status=pending_approval entry and rebind.
			// This makes the plan visible again without forcing the
			// user to remember its file path or restart codrax.
			if recovered, ok := r.recoverPendingPlanFromStore(); ok {
				r.pendingPlanPath = recovered.Path
				r.info(recoveredPendingPlan(r.language, recovered.ID))
			} else {
				// When write is disabled, "/mode write" itself would bounce
				// off the L2 gate — surface that root cause instead of
				// telling the user to run a command they can't.
				if !r.writeEnabled {
					for _, line := range noPendingPlanWriteDisabled(r.language) {
						r.info(line)
					}
					return
				}
				r.info(noPendingPlan(r.language))
				return
			}
		}
		id := strings.TrimSuffix(filepath.Base(r.pendingPlanPath), ".json")
		plan, err := r.planStore.Load(id)
		if err != nil {
			r.errorf("plan show: %v\n", err)
			return
		}
		fmt.Fprintf(r.out, "  current plan: %s\n", r.pendingPlanPath)
		fmt.Fprintf(r.out, "    id:      %s\n", plan.ID)
		fmt.Fprintf(r.out, "    status:  %s\n", plan.Status)
		fmt.Fprintf(r.out, "    changes: %d file(s)\n", len(plan.Changes))
		// Phase membership when this plan is one of N in a multi-
		// phase group. Lets the operator follow the cross-link to
		// /phase show <group-id> for the broader group state.
		// Single-phase plans leave PhaseGroupID empty so this
		// section stays absent.
		if plan.PhaseGroupID != "" {
			phaseTotal := r.phaseTotalForGroup(plan.PhaseGroupID)
			if phaseTotal > 0 {
				fmt.Fprintf(r.out, "    phase:   %d of %d in group %s (use `/phase show %s`)\n",
					plan.PhaseIndex+1, phaseTotal, plan.PhaseGroupID, plan.PhaseGroupID)
			} else {
				fmt.Fprintf(r.out, "    phase:   %d in group %s (use `/phase show %s`)\n",
					plan.PhaseIndex+1, plan.PhaseGroupID, plan.PhaseGroupID)
			}
		}
		if len(plan.TargetPaths) > 0 {
			fmt.Fprintf(r.out, "    targets: %s\n", strings.Join(plan.TargetPaths, ", "))
		}
		// Apply-progress visibility (commit 42 P0): for
		// partially_applied plans, surface WHICH files actually
		// landed vs which still pending so /approve --retry's
		// scope is glanceable. AppliedPaths is the subset of
		// TargetPaths the apply post-hook persisted.
		if plan.Status == types.PlanStatusPartiallyApplied && len(plan.AppliedPaths) > 0 {
			pending := make([]string, 0, len(plan.TargetPaths))
			appliedSet := make(map[string]struct{}, len(plan.AppliedPaths))
			for _, p := range plan.AppliedPaths {
				appliedSet[p] = struct{}{}
			}
			for _, t := range plan.TargetPaths {
				if _, ok := appliedSet[t]; !ok {
					pending = append(pending, t)
				}
			}
			fmt.Fprintf(r.out, "    applied: %d/%d (%s)\n",
				len(plan.AppliedPaths), len(plan.TargetPaths),
				strings.Join(plan.AppliedPaths, ", "))
			if len(pending) > 0 {
				fmt.Fprintf(r.out, "    pending: %s\n", strings.Join(pending, ", "))
			}
		}
		if plan.Summary != "" {
			fmt.Fprintf(r.out, "    summary: %s\n", oneLine(plan.Summary))
		}
		for _, line := range renderWriteRiskAssessment(r.language, plan, r.writeApprovalPolicy) {
			fmt.Fprintln(r.out, line)
		}
		// Static-check unvalidated reasons (commit 7 P1-E gap-fix).
		// Surfaces languages whose dry-build was skipped because the
		// toolchain was missing. Distinct from "validated and passed":
		// a non-empty list means one or more languages reached apply
		// without their static gate firing — operator should know.
		if len(plan.UnvalidatedReasons) > 0 {
			fmt.Fprintln(r.out, "\n  · unvalidated stages:")
			for _, reason := range plan.UnvalidatedReasons {
				fmt.Fprintf(r.out, "    - %s\n", reason)
			}
		}
		// Plan critic review (commit 7 P1-F gap-fix). Persisted on
		// the plan struct so REPL restart preserves the critique.
		// Empty when the critic was disabled or produced no risks
		// (or commit 10 #7 suppressed a low-confidence critique).
		// Render with a leading · so it stands out against the
		// rest of the plan summary — operators have repeatedly
		// rubber-stamped /approve when the critique was rendered
		// as just another markdown paragraph.
		if strings.TrimSpace(plan.PlanCritique) != "" {
			fmt.Fprintln(r.out, "\n  · review feedback:")
			for _, line := range strings.Split(strings.TrimSpace(plan.PlanCritique), "\n") {
				fmt.Fprintf(r.out, "    %s\n", line)
			}
		}
		// Summary view skips the diff and prints only a per-change
		// path / kind / rationale table. Big plans (50+ files) would
		// otherwise scroll the diff off-screen; --summary is the
		// quick "what's in this plan" inspection.
		if summaryOnly {
			fmt.Fprintln(r.out, "\n  changes (summary):")
			for _, c := range plan.Changes {
				rationale := oneLine(c.Rationale)
				if len(rationale) > 80 {
					rationale = rationale[:77] + "..."
				}
				// Render rename rows with a leading ⇄ so they
				// stand out (commit 12 #9). Rename is a git-history-
				// preserving operation that's easy to overlook in a
				// big plan; the marker draws the operator's eye.
				if c.Kind == "rename" {
					fmt.Fprintf(r.out, "    ⇄ [rename] %s → %s — %s\n",
						c.Path, c.NewPath, rationale)
					continue
				}
				dest := ""
				if c.NewPath != "" {
					dest = " → " + c.NewPath
				}
				fmt.Fprintf(r.out, "    [%s] %s%s — %s\n", c.Kind, c.Path, dest, rationale)
			}
			// Health flags also surface in --summary mode (commit 13 #2).
			// Without this, an operator using the compact view to scan
			// big plans would never see "this plan has 2 unvalidated
			// languages" or "the critic flagged 3 risks" — both of
			// which are decision-relevant before /approve.
			if len(plan.UnvalidatedReasons) > 0 {
				fmt.Fprintln(r.out, "\n  · unvalidated stages:")
				for _, reason := range plan.UnvalidatedReasons {
					fmt.Fprintf(r.out, "    - %s\n", reason)
				}
			}
			if strings.TrimSpace(plan.PlanCritique) != "" {
				fmt.Fprintln(r.out, "\n  · review feedback:")
				for _, line := range strings.Split(strings.TrimSpace(plan.PlanCritique), "\n") {
					fmt.Fprintf(r.out, "    %s\n", line)
				}
			}
		} else {
			// Per-change diff preview so the user /approves with full
			// knowledge. Caps: 16 KB total / 4 KB per change — enough to
			// inspect a small surgical plan, bounded so a monster plan
			// doesn't flood the terminal; the cap message tells the user
			// to read the plan JSON for full detail.
			fmt.Fprintln(r.out, "\n  diff preview:")
			fmt.Fprint(r.out, render.ColorizeUnifiedDiff(
				renderPlanDiff(plan, r.repoRoot, 16*1024, 4*1024),
				r.colorMode, r.out))
		}
		// Footer: name the next slash commands so the user does not
		// have to remember them after reading the diff. Status-aware
		// (commit 41 UX#5): each lifecycle state has its own
		// recovery vocabulary — verify_failed wants --retry,
		// partially_applied wants --retry without /merge, applied
		// wants /merge etc. Footer is suppressed for terminal
		// states (rejected / merged / applied_failed) where there
		// are no actionable next steps.
		switch plan.Status {
		case types.PlanStatusPending,
			types.PlanStatusApplied,
			types.PlanStatusVerifyFailed,
			types.PlanStatusUnverified,
			types.PlanStatusPartiallyApplied:
			for _, line := range planShowFooter(r.language, plan.Status) {
				fmt.Fprintln(r.out, line)
			}
		}
	case "clear":
		if r.pendingPlanPath == "" {
			r.info(noPendingPlanToClear(r.language))
			return
		}
		id := strings.TrimSuffix(filepath.Base(r.pendingPlanPath), ".json")
		if err := r.planStore.Clear(id); err != nil {
			r.errorf("plan clear: %v\n", err)
			return
		}
		// Discard the worktree along with the plan file. Workspace
		// and plan are tied (single-pending invariant); /plan clear
		// is the no-audit settle path so the worktree should not
		// linger. Best-effort: discard failures log but don't fail
		// the clear.
		r.discardWorktreeForRejected(id)
		r.success(fmt.Sprintf("pending plan cleared: %s", r.pendingPlanPath))
		r.pendingPlanPath = ""
	case "list":
		infos, err := r.planStore.List()
		if err != nil {
			r.errorf("plan list: %v\n", err)
			return
		}
		if len(infos) == 0 {
			r.info(noPlansInStore(r.language, r.planStore.PlanDir()))
			return
		}
		fmt.Fprintf(r.out, "  %d plan(s) in %s (newest first):\n", len(infos), r.planStore.PlanDir())
		for _, inf := range infos {
			ts := time.Unix(0, inf.ModTime).Format("2006-01-02 15:04:05")
			status := inf.Status
			if status == "" {
				status = "unknown"
			}
			// Plan health flags (commit 10 #6) shown alongside
			// status so the operator can spot at a glance which
			// plans have skipped static checks or carry an
			// unread critic review.
			flags := ""
			if inf.UnvalidatedCount > 0 {
				flags += fmt.Sprintf(" · unvalidated:%d", inf.UnvalidatedCount)
			}
			if inf.HasCritique {
				flags += " · review"
			}
			// Phase membership (commit 25): when a plan is one
			// of N in a multi-phase group, surface the link so
			// /plan list groups visibly cluster together.
			if inf.PhaseGroupID != "" {
				flags += fmt.Sprintf(" group=%s phase=%d", inf.PhaseGroupID, inf.PhaseIndex+1)
			}
			fmt.Fprintf(r.out, "    - [%s] %s  status=%s  (%d bytes)%s\n",
				ts, inf.ID, status, inf.SizeB, flags)
		}
	default:
		r.warn("%s", unknownPlanSubcommand(r.language, rest))
	}
}

// handlePlanClearArg services the three argument forms of
// `/plan clear <X>`:
//
//	/plan clear <plan-id>          — delete one named plan from
//	                                  PlanStore (any status)
//	/plan clear --all              — wipe every plan in store
//	                                  (interactive y/N confirm)
//	/plan clear --status=<state>   — wipe every plan whose Status
//	                                  matches (interactive y/N
//	                                  confirm)
//
// Sibling `<id>.report.json` files are removed alongside their
// plan so /history doesn't keep dangling reports. The pending
// pointer (r.pendingPlanPath) is cleared if the deleted plan
// happens to be the pending one.
func (r *REPL) handlePlanClearArg(arg string) {
	// --all bulk wipe
	if arg == "--all" {
		r.handlePlanClearBulk("", "all plans")
		return
	}
	// --status=<state>
	if strings.HasPrefix(arg, "--status=") {
		want := strings.TrimSpace(strings.TrimPrefix(arg, "--status="))
		if want == "" {
			r.errorf("plan clear: --status= requires a value (e.g. rejected, applied_failed, verify_failed)\n")
			return
		}
		r.handlePlanClearBulk(want, fmt.Sprintf("status=%s", want))
		return
	}
	// Otherwise treat as a plan ID.
	planID := arg
	full, err := r.planStore.Load(planID)
	if err != nil || full == nil {
		r.errorf("plan clear: %s", planNotFound(r.language, planID))
		return
	}
	if err := r.planStore.Clear(planID); err != nil {
		r.errorf("plan clear: %v\n", err)
		return
	}
	// Sibling report file (if any) — best-effort delete.
	reportPath := filepath.Join(r.planStore.PlanDir(), planID+".report.json")
	if _, statErr := os.Stat(reportPath); statErr == nil {
		_ = os.Remove(reportPath)
	}
	// Drop the pending pointer if it was pointing at this plan so
	// the next /plan show / /approve doesn't surface a phantom.
	if filepath.Base(r.pendingPlanPath) == planID+".json" {
		r.pendingPlanPath = ""
	}
	r.success(fmt.Sprintf("plan cleared: %s (status was %q)", planID, full.Status))
}

// handlePlanClearBulk wipes every plan in PlanStore that matches
// the filter. statusFilter "" means "all plans"; otherwise only
// plans whose Status equals statusFilter are deleted. Interactive
// y/N confirmation is required when r.interactive() is true; in
// scripted mode the bulk delete proceeds without confirmation
// (caller is responsible for the script's correctness).
//
// label is the human-readable description of what's about to be
// deleted, used in the confirmation prompt and the success
// message ("all plans" / "status=rejected" / etc.).
func (r *REPL) handlePlanClearBulk(statusFilter, label string) {
	infos, err := r.planStore.List()
	if err != nil {
		r.errorf("plan clear: list: %v\n", err)
		return
	}
	var matches []PlanInfo
	for _, inf := range infos {
		if statusFilter != "" && inf.Status != statusFilter {
			continue
		}
		matches = append(matches, inf)
	}
	if len(matches) == 0 {
		r.info(fmt.Sprintf("plan clear: no plans match %s", label))
		return
	}
	if r.interactive() {
		title := fmt.Sprintf("Delete %d plan(s) matching %s? This is irreversible.", len(matches), label)
		confirmed := false
		if err := huh.NewConfirm().
			Title(title).
			Affirmative("Yes").
			Negative("No").
			Value(&confirmed).
			Run(); err != nil {
			confirmed = false
		}
		if !confirmed {
			r.info("plan clear cancelled")
			return
		}
	}
	deleted := 0
	for _, inf := range matches {
		if err := r.planStore.Clear(inf.ID); err != nil {
			r.warn("plan clear: %s: %v\n", inf.ID, err)
			continue
		}
		reportPath := filepath.Join(r.planStore.PlanDir(), inf.ID+".report.json")
		_ = os.Remove(reportPath)
		if filepath.Base(r.pendingPlanPath) == inf.ID+".json" {
			r.pendingPlanPath = ""
		}
		deleted++
	}
	r.success(fmt.Sprintf("plan clear: deleted %d/%d plan(s) matching %s", deleted, len(matches), label))
}

// recoverPendingPlanFromStore walks PlanStore.List for the most
// recent plan whose Status is pending_approval and returns its
// PlanInfo. Used by /plan show when r.pendingPlanPath is empty —
// e.g. after a pre-flight /approve failure (older builds dropped
// the pointer), or when a fresh REPL session lands in a dir whose
// PlanStore inherited a pending plan from a prior process.
//
// Returns (PlanInfo{}, false) when:
//   - PlanStore is nil
//   - List errored (warning is logged)
//   - no entry with Status=pending_approval exists
//
// PlanStore.List is already newest-first sorted, so the first match
// is the right one.
// detectUnsettledPlan returns the single unsettled plan blocking
// new-plan creation, or (zero, false) when there is none.
//
// "Unsettled" = types.IsUnsettledStatus(s) — pending_approval /
// applied / verify_failed. The single-pending-plan invariant
// (enforced at PlanStore.Save) means there is at most ONE such
// plan in the project at any moment; we still walk newest-first
// for defensive robustness against legacy data.
//
// Three layers consume this:
//   - banner() prints a one-line dim status when REPL starts up
//     and an unsettled plan is on disk
//   - handleModeCmd("/mode write") refuses to switch and surfaces
//     a 3-way menu (merge / reject / clear)
//   - handleApprove / handleRejectCmd cold-start recovery rebinds
//     pendingPlanPath for the in-session pointer
func (r *REPL) detectUnsettledPlan() (PlanInfo, bool) {
	if r.planStore == nil {
		return PlanInfo{}, false
	}
	infos, err := r.planStore.List()
	if err != nil {
		logging.Warning("[repl] detect unsettled: list failed: %v", err)
		return PlanInfo{}, false
	}
	for _, inf := range infos {
		if types.IsUnsettledStatus(inf.Status) {
			// Commit 42 P1: validate the worktree path still
			// exists on disk for statuses that depend on it
			// (applied / verify_failed / unverified /
			// partially_applied — all four record a worktree
			// the user might still need). If the worktree was
			// deleted out-of-band, mark inf.WorktreeMissing
			// so the banner can render an "(orphaned worktree)"
			// tag instead of pretending the plan is still
			// fully actionable.
			if inf.Status != types.PlanStatusPending {
				if full, lerr := r.planStore.Load(inf.ID); lerr == nil && full != nil &&
					full.WorktreePath != "" {
					if _, statErr := os.Stat(full.WorktreePath); os.IsNotExist(statErr) {
						inf.WorktreeMissing = true
					}
				}
			}
			return inf, true
		}
	}
	return PlanInfo{}, false
}

func (r *REPL) recoverPendingPlanFromStore() (PlanInfo, bool) {
	if r.planStore == nil {
		return PlanInfo{}, false
	}
	infos, err := r.planStore.List()
	if err != nil {
		logging.Warning("[repl] recover pending plan: list failed: %v", err)
		return PlanInfo{}, false
	}
	for _, inf := range infos {
		if inf.Status == types.PlanStatusPending {
			return inf, true
		}
	}
	return PlanInfo{}, false
}

// countOtherPendingPlans enumerates PlanStore for re-approvable
// plans (Status=pending_approval OR verify_failed) and returns
// how many exist BESIDES `excludeID`. Used by /approve to surface
// a "N other approvable plans" hint so the user notices when
// /plan list has multiple candidates and the most-recent default
// might not be the one they wanted.
//
// Robustness: nil store returns 0. List error logs at warning and
// returns 0 — the hint is best-effort, never blocks the approve
// flow.
func countOtherPendingPlans(store *PlanStore, excludeID string) int {
	if store == nil {
		return 0
	}
	infos, err := store.List()
	if err != nil {
		logging.Warning("[repl] countOtherPendingPlans: list failed: %v", err)
		return 0
	}
	n := 0
	for _, inf := range infos {
		if inf.ID == excludeID {
			continue
		}
		if inf.Status == types.PlanStatusPending || inf.Status == types.PlanStatusVerifyFailed {
			n++
		}
	}
	return n
}

// collectPlanHistory enumerates saved plans and pairs each with its
// sibling ChangeReport (if one exists on disk). Returns one oneLine
// row per plan for /history rendering. An empty return means no
// plans exist OR PlanStore is nil (B-polish.6).
//
// Example row shapes:
//
//	plan-abc  status=applied           tests=12/12 passed
//	plan-def  status=verify_failed     tests=3/5 passed: TestA, TestB
//	plan-ghi  status=pending_approval  (no report)
func (r *REPL) collectPlanHistory() []string {
	if r.planStore == nil {
		return nil
	}
	infos, err := r.planStore.List()
	if err != nil {
		logging.Warning("[repl] plan history: list failed: %v", err)
		return nil
	}
	if len(infos) == 0 {
		return nil
	}
	// Group aggregation (commit 43): walk plans newest-first and
	// collect same-PhaseGroupID plans into one visual block. The
	// PlanStore.List output is sorted by ModTime descending so
	// later phases of the same group appear first; we sort the
	// child rows by PhaseIndex ascending within each block so the
	// reader sees the natural lifecycle order. Singleton plans
	// (PhaseGroupID == "") render as flat rows alongside groups
	// in the order their newest member appeared.
	type childInfo struct {
		info PlanInfo
		row  string
	}
	type bucket struct {
		groupID      string      // empty for singletons
		children     []childInfo // group members sorted by PhaseIndex ascending
		single       *childInfo  // populated when groupID == ""; children empty
		firstSeenIdx int         // index in original infos for stable ordering
	}
	bucketByGroup := map[string]*bucket{}
	var ordered []*bucket
	for i, inf := range infos {
		row := r.formatPlanHistoryRow(inf)
		ci := childInfo{info: inf, row: row}
		if inf.PhaseGroupID == "" {
			b := &bucket{single: &ci, firstSeenIdx: i}
			ordered = append(ordered, b)
			continue
		}
		if existing, ok := bucketByGroup[inf.PhaseGroupID]; ok {
			existing.children = append(existing.children, ci)
			continue
		}
		b := &bucket{groupID: inf.PhaseGroupID, children: []childInfo{ci}, firstSeenIdx: i}
		bucketByGroup[inf.PhaseGroupID] = b
		ordered = append(ordered, b)
	}
	circles := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩", "⑪", "⑫"}
	var rows []string
	for _, b := range ordered {
		if b.single != nil {
			rows = append(rows, b.single.row)
			continue
		}
		// Sort children by PhaseIndex ascending so phase 1 appears
		// before phase 2 inside the group block.
		for i := 0; i < len(b.children); i++ {
			for j := i + 1; j < len(b.children); j++ {
				if b.children[j].info.PhaseIndex < b.children[i].info.PhaseIndex {
					b.children[i], b.children[j] = b.children[j], b.children[i]
				}
			}
		}
		// Group header line + indented child rows with circle
		// markers. Mirrors formatSubTopicsBlock's enumeration
		// pattern from the read-mode dock so multi-phase
		// groups feel coherent visually.
		rows = append(rows, fmt.Sprintf("%s (%d phases):", b.groupID, len(b.children)))
		for i, c := range b.children {
			mark := fmt.Sprintf("%d.", i+1)
			if i < len(circles) {
				mark = circles[i]
			}
			rows = append(rows, fmt.Sprintf("  %s %s", mark, c.row))
		}
	}
	return rows
}

// formatPlanHistoryRow extracts the per-plan row formatting used
// by collectPlanHistory so the group-aggregation loop can call
// it for every member. Behaviour byte-identical to the
// pre-commit-43 inline implementation.
func (r *REPL) formatPlanHistoryRow(inf PlanInfo) string {
	status := inf.Status
	if status == "" {
		status = "unknown"
	}
	reportPath := strings.TrimSuffix(inf.Path, ".json") + ".report.json"
	row := fmt.Sprintf("%s  status=%s", inf.ID, status)
	data, rerr := os.ReadFile(reportPath)
	if rerr != nil {
		return row + "  (no report)"
	}
	var probe struct {
		Passed      bool `json:"passed"`
		TestResults []struct {
			AssertionID string `json:"assertion_id"`
			Passed      bool   `json:"passed"`
		} `json:"test_results"`
	}
	if jerr := json.Unmarshal(data, &probe); jerr != nil {
		return row + "  (report unreadable)"
	}
	total := len(probe.TestResults)
	passed := 0
	var failed []string
	for _, tr := range probe.TestResults {
		if tr.Passed {
			passed++
		} else {
			failed = append(failed, tr.AssertionID)
		}
	}
	row += fmt.Sprintf("  tests=%d/%d passed", passed, total)
	if len(failed) > 0 {
		const maxFailingShown = 3
		shown := failed
		suffix := ""
		if len(shown) > maxFailingShown {
			shown = shown[:maxFailingShown]
			suffix = fmt.Sprintf(" (+%d more)", len(failed)-maxFailingShown)
		}
		row += ": " + strings.Join(shown, ", ") + suffix
	}
	return row
}

// handleApproveCmd consumes the REPL's pending ChangePlan by triggering
// a one-shot Run with Mode=ModeApply + PlanPath pre-seeded. The
// orchestrator's ModeApply branch skips the plan stage hook when PlanPath is
// set (see orchestrator.go:case types.ModeApply), so the saved plan
// is applied as-is rather than regenerated.
//
// Preconditions probed in order:
//  1. PlanStore configured — no store means the single-shot path
//     never persisted a plan; nothing to approve.
//  2. pendingPlanPath non-empty — user must run a /mode write
//     dispatch first (or have one from a prior session).
//  3. Plan file still loadable — user may have /plan clear'd or
//     removed the file by hand since emission.
//  4. Runner implements modeSetter + planPathSetter — test stubs
//     that omit these make /approve a no-op with a warning so
//     tests don't accidentally run real apply.
//
// After those checks, an interactive huh.NewConfirm (or scripted
// y/n read) gives the user a final abort. On confirm, the handler:
//   - saves the sticky mode
//   - calls SetMode(ModeApply) + SetPlanPath(pendingPlanPath)
//   - dispatches Run with a synthetic request string
//   - restores SetMode(originalMode) + SetPlanPath("") in a defer
//   - renders the Result, records the turn, and clears
//     pendingPlanPath regardless of success (apply is terminal;
//     the user re-plans if they want to try again)
func (r *REPL) handleApproveCmd(line string) {
	if r.pendingOperation != nil {
		r.handleOperationApproveCmd(line)
		return
	}
	if r.pendingProviderOperation != nil {
		r.handleProviderOperationApproveCmd(line)
		return
	}
	// Parse all /approve arguments in one pass:
	//   `/approve` — operate on the most recent pending plan
	//   `/approve <plan-id>` — explicitly target a plan; useful when
	//      /plan list shows multiple pending plans
	//   `/approve --plan-id=<id>` — long-flag form of the above
	//   `/approve --merge-to=<branch>` — chain a merge after clean
	//      apply+verify
	//   `/approve --skip-verify` — apply only, skip the verify stage
	//      (use when local box can't run integration tests; review
	//      the diff carefully — no test-pass guarantee)
	planArg, mergeTo, skipVerify, retry := parseApproveArgsAll(line)
	if r.planStore == nil {
		r.info(commandDisabled(r.language, "/approve", noPlanStoreReason(r.language)))
		return
	}
	// L2 gate companion: /approve dispatches Mode=ModeApply, which the
	// orchestrator gates on write_enabled. Reject here so the user gets
	// the same clean error the /mode handler produces — without this,
	// a user who somehow reached pendingPlanPath != "" (CLI single-shot
	// then dropped into REPL?) would see a deep-pipeline failure.
	if !r.writeEnabled {
		for _, line := range writeModeDisabled(r.language, "/approve", r.settingsPath) {
			r.warn("%s\n", line)
		}
		return
	}

	// Plan resolution. Three paths:
	//   1. `/approve <plan-id>` or `--plan-id=<id>` — explicit
	//      target. Required when /plan list shows multiple pending
	//      plans and the user wants a specific one (not the
	//      latest).
	//   2. `/approve` with r.pendingPlanPath set — the most-recent-
	//      pending pointer set by the prior plan-mode dispatch.
	//   3. `/approve` with empty pointer — fall through to
	//      noPendingPlan info; the user runs /mode write or /plan
	//      list to see what's available.
	if planArg != "" {
		full, err := r.planStore.Load(planArg)
		if err != nil || full == nil {
			r.errorf("approve: %s", planNotFound(r.language, planArg))
			return
		}
		r.pendingPlanPath = filepath.Join(r.planStore.PlanDir(), planArg+".json")
	}
	// Cold-start recovery: when the REPL just started in a directory
	// whose PlanStore already has a pending_approval plan from a
	// prior session, r.pendingPlanPath is empty. The single-pending
	// invariant (enforced at PlanStore.Save) means there should be
	// at most one such plan; we auto-rebind to it. If somehow
	// multiple pending plans exist (legacy data), pick the newest
	// — PlanStore.List is newest-first.
	if planArg == "" && r.pendingPlanPath == "" {
		r.bindActiveWriteWorkflowPlanIfIdle("/approve")
	}
	if r.pendingPlanPath == "" {
		if recovered, ok := r.recoverPendingPlanFromStore(); ok {
			r.pendingPlanPath = recovered.Path
			r.info(recoveredPendingPlan(r.language, recovered.ID))
		}
	}
	if r.pendingPlanPath == "" {
		r.info(noPendingPlan(r.language))
		return
	}

	id := strings.TrimSuffix(filepath.Base(r.pendingPlanPath), ".json")
	plan, err := r.planStore.Load(id)
	if err != nil {
		r.errorf("approve: %v\n", err)
		// Stale pendingPlanPath: file was removed out-of-band. Clear
		// the pointer so /plan show / /approve stop nagging.
		r.pendingPlanPath = ""
		return
	}

	// /approve accepts pending_approval AND verify_failed plans.
	// pending_approval is the obvious case (a fresh plan straight
	// out of /mode write). verify_failed is the env-fix retry path:
	// the plan applied cleanly but tests failed because the runner
	// binary was missing, the database wasn't running, network
	// flakes, etc. After the operator fixes the environment, they
	// want to re-run apply+verify against the SAME reviewed plan
	// without regenerating it. Re-approving applied / applied_failed
	// / rejected plans is refused because they are either already
	// landed (re-running would wastefully re-apply) or were rejected
	// by the operator's earlier deliberate /reject (re-approving
	// would silently undo that decision).
	switch plan.Status {
	case types.PlanStatusPending, types.PlanStatusVerifyFailed, "":
		// re-approvable; "" is legacy unset, treated as pending
	case types.PlanStatusPartiallyApplied, types.PlanStatusUnverified:
		// Only re-approvable with --retry (commit 9 #1). Without
		// --retry, surface a hint that names the flag rather than
		// the generic refused-status message — the user is
		// already mid-flow with a partial / unverified plan and
		// needs the actionable command, not just "no".
		if !retry {
			r.warn("plan %s is in status %q; use `/approve --retry` to continue from current AppliedSet, or `/reject` to discard the partial state\n",
				plan.ID, plan.Status)
			r.pendingPlanPath = ""
			return
		}
	default:
		r.warn("%s", approveRefusedStatusMsg(r.language,
			plan.ID, plan.Status,
			types.PlanStatusPending, types.PlanStatusVerifyFailed))
		r.pendingPlanPath = ""
		return
	}
	if plan.Status == types.PlanStatusVerifyFailed {
		r.info(fmt.Sprintf("re-approving plan %s (status was verify_failed; assuming env-fix retry)", plan.ID))
	}
	if retry && plan.Status == types.PlanStatusPartiallyApplied {
		// Commit 17 #1: honest message. /approve --retry
		// dispatches a fresh ModeApply Run; applyPreHook
		// provisions a NEW worktree, so the prior partial-
		// apply state is gone. Coder re-applies every target
		// path from scratch. apply_patch is idempotent for
		// existing-file create/modify (the unit lands again
		// without harm) so the only practical effect is
		// re-running through the failure point — useful only
		// if the operator changed something in the source repo
		// or environment between attempts. Pre-commit-17 the
		// message claimed "coder skips already-applied units"
		// which was wrong: the new Run's worktree is empty.
		r.info(fmt.Sprintf("retrying plan %s — apply re-runs all %d target paths in a fresh worktree (apply_patch is idempotent for create/modify; the failure point that produced partially_applied will be retried).",
			plan.ID, len(plan.TargetPaths)))
		r.info("  if the previous failure was caused by repo / environment state, fix that first; otherwise expect the same outcome.")
	}
	if retry && plan.Status == types.PlanStatusUnverified {
		// Commit 17 #1: --retry on unverified is dubious — the
		// bytes already landed; the operator wants to test, not
		// re-apply. /verify <plan-id> is the right command.
		// Don't refuse outright (the operator may have a reason)
		// but surface the better tool.
		r.info(fmt.Sprintf("retrying plan %s — note: status was unverified, meaning apply already landed but no tests verified the change. /verify %s re-runs the test suite without re-applying; if you want to re-apply (different worktree state, etc.), --retry continues here.",
			plan.ID, plan.ID))
	}

	// Multi-pending hint. When PlanStore carries more than one
	// pending plan, surface the count + how to target a specific
	// one before the confirm prompt — this prevents the operator
	// from approving "the wrong one" when /plan list has many
	// rows and they forget which is the latest.
	if extras := countOtherPendingPlans(r.planStore, plan.ID); extras > 0 {
		r.info(otherPendingPlansHint(r.language, plan.ID, extras))
	}

	assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
	decision := writeflow.DecideWriteApproval(r.writeApprovalPolicy, assessment)
	// Stricter-wins: the plan-time gate may have read typed graph evidence
	// (declaration-span corroboration) the REPL does not have. When the
	// recorded approval for this exact plan fingerprint demanded a manual
	// decision and the local recompute would auto-execute, keep the
	// stricter recorded requirement instead of silently downgrading.
	if plan.Approval != nil &&
		plan.Approval.Action == string(writeflow.ApprovalActionManual) &&
		plan.Approval.PlanFingerprint == types.PlanFingerprint(plan) &&
		decision.Action == writeflow.ApprovalActionAutoExecute {
		decision.Action = writeflow.ApprovalActionManual
		decision.ReasonCode = plan.Approval.ReasonCode
		decision.Reason = "recorded plan-time approval requires manual confirmation"
	}
	if decision.Action == writeflow.ApprovalActionDeny {
		r.warn("%s", writeApprovalDenied(r.language, plan.ID, assessment, decision))
		return
	}

	// Probe both setters up-front. Running Mode=ModeApply against a
	// runner without SetMode would silently fall through to read
	// mode in the orchestrator; without SetPlanPath the apply phase
	// would error trying to load nil. Fail loud rather than dispatching.
	mSetter, mOK := r.runner.(modeSetter)
	pSetter, pOK := r.runner.(planPathSetter)
	if !mOK || !pOK {
		r.warn("%s", approveStubRunnerMsg(r.language))
		return
	}

	title := approveTitlePromptWithContext(r.language, r.writeApprovalPromptContext(plan, skipVerify))
	confirmed := decision.Action == writeflow.ApprovalActionAutoExecute
	if confirmed {
		r.info(writeApprovalAutoProceeding(r.language, plan.ID, assessment, decision))
	} else {
		if r.interactive() {
			if err := huh.NewConfirm().
				Title(title).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed).
				Run(); err != nil {
				confirmed = false
			}
		} else {
			fmt.Fprintln(r.out, title)
			s, err := r.readInputLines("")
			if err == nil {
				confirmed = strings.TrimSpace(strings.ToLower(s)) == "y"
			}
		}
	}
	if !confirmed {
		r.info(approveCancelled(r.language))
		return
	}
	userDecision := "approved"
	if decision.Action == writeflow.ApprovalActionAutoExecute {
		userDecision = "auto"
	}
	plan.Approval = writeflow.NewApprovalRecord(
		assessment,
		decision,
		"repl_approve",
		userDecision,
		types.PlanFingerprint(plan),
		worktree.OperatorIdentity(r.repoRoot),
	)
	writeflow.BindApprovalRecordEffect(plan.Approval, plan, r.activeWriteWorkflowRunForPlan(plan.ID))
	if strings.TrimSpace(r.pendingPlanPath) != "" {
		if err := types.WritePlanToFile(plan, r.pendingPlanPath); err != nil {
			r.warn("could not persist write approval record for plan %s: %v\n", plan.ID, err)
			if decision.Action == writeflow.ApprovalActionManual {
				return
			}
		}
	}

	// Bare-directory scaffolding gate. Before handing the plan to the
	// orchestrator, probe the target repo. If it's not a git repo, or
	// the repo has no HEAD commit yet, `git worktree add --detach HEAD`
	// inside worktree.Create will fail. Three-tier authorization:
	//
	//   1. yaml/CLI pre-authorized on every needed tier → forward to
	//      the orchestrator and proceed silently.
	//   2. interactive REPL → prompt y/N (the title covers the
	//      scaffold tier too when the dir is effectively empty, so
	//      consent is informed and the user is not walled twice);
	//      on yes, forward; on no, print the recovery hint and bail
	//      (plan stays pending).
	//   3. non-interactive REPL (scripted) without init pre-auth →
	//      bail with the same hint. With init pre-auth but the
	//      scaffold tier missing, fall through and let the apply
	//      pre-hook fail loud with the precise scaffold recovery
	//      text — scripted runs keep the flag ceremony.
	//
	// The orchestrator's apply pre-hook does the actual EnsureInitialCommit
	// + worktree.Create call so all writes happen in one place.
	if state, derr := worktree.DetectRepoState(r.repoRoot); derr == nil && state.NeedsInit() {
		empty := worktree.DirIsEffectivelyEmpty(r.repoRoot)
		scaffoldConsent := r.writeScaffoldEnabled
		switch {
		case !bareDirConsentNeeded(r.writeAutoInitRepo, r.writeScaffoldEnabled, empty):
			// Tier 1: pre-authorized — forward silently below.
		case !r.interactive():
			if !r.writeAutoInitRepo {
				r.errorf("%s", approveBareDirNoAuthMsg(r.language, r.repoRoot, state.String()))
				return
			}
		default:
			title := autoInitConsentTitle(r.language, r.repoRoot, state.String())
			if empty {
				title = autoInitScaffoldConsentTitle(r.language, r.repoRoot, state.String())
			}
			autoConsent := false
			if err := huh.NewConfirm().
				Title(title).
				Affirmative("Yes").
				Negative("No").
				Value(&autoConsent).
				Run(); err != nil {
				autoConsent = false
			}
			if !autoConsent {
				for _, line := range autoInitDeclined(r.language) {
					r.info(line)
				}
				return
			}
			r.info(autoInitProceeding(r.language, r.repoRoot))
			if empty {
				scaffoldConsent = true
			}
		}
		// Per-Run authorization: forward to orchestrator. The defer
		// restores boot-time values so subsequent /approve calls
		// against a different (already-initialized) repo don't keep
		// the gates lifted.
		restoreBareDirAuth := r.forwardBareDirAuthorization(empty && scaffoldConsent)
		defer restoreBareDirAuth()
	}

	originalMode := r.currentMode
	mSetter.SetMode(types.ModeApply)
	pSetter.SetPlanPath(r.pendingPlanPath)
	defer func() {
		mSetter.SetMode(originalMode)
		pSetter.SetPlanPath("")
	}()

	// `--skip-verify` lifts the verify stage for this Run only. Use
	// case: the operator's local box can't run integration tests
	// (DB / GPU / external API), they reviewed the diff, and they
	// want apply to land bytes immediately — verification will
	// happen in CI on push. The orchestrator setter scopes the
	// override to one Run; defer-restore prevents it from leaking
	// to subsequent /approve calls against different plans.
	if skipVerify {
		if setter, ok := r.runner.(skipVerifySetter); ok {
			setter.SetSkipVerify(true)
			defer setter.SetSkipVerify(false)
			r.info(skipVerifyAcknowledged(r.language))
		} else {
			r.warn("%s", approveSkipVerifyStubMsg(r.language))
		}
	}

	// Synthesize a natural-language request the analyzer can chew on.
	// A literal "/approve <id>" looks like a REPL control input — the
	// analyzer's emit_analysis tool rejects it (see
	// internal/tool/emit_analysis.go::IsREPLControlInput) and the
	// classifier loop spins to its iter cap before yielding. Even
	// though the orchestrator discards the read AnalysisIR in write mode,
	// the classifier still has to terminate cleanly. Use plan.Summary so
	// the analyzer sees code-question content; fall back to a generic
	// phrasing when the planner left the summary blank.
	request := approveDispatchRequest(plan)
	logging.Info("[repl] dispatching approve: plan=%s path=%s", plan.ID, r.pendingPlanPath)

	if r.renderer != nil {
		// /approve always runs apply-mode → 4-stage K/N denominator
		// (analyze + plan + apply + verify). Pre-fix this was 3,
		// which left the dock K/N count one short for the entire
		// approve flow.
		r.renderer.SetTotalStages(4)
		r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	}
	busCtx, runErr := r.runInFlightWrap(func() (*types.BusContext, error) {
		return r.runner.Run(request, r.repoRoot, r.branch)
	})
	if r.renderer != nil {
		armDockTerminalState(r.renderer, runErr)
		r.renderer.StopSpinner()
	}
	if runErr != nil {
		logging.Error("[repl] approve failed: %v", runErr)
		r.errorf("approve: %s\n", friendlyRunError(r.language, runErr))
		// Pre-flight failures (worktree provisioning, missing git, etc.)
		// never touched the plan — its on-disk Status stays
		// pending_approval. Keep r.pendingPlanPath set so /plan show
		// still works and the user can fix the environment + /approve
		// again, or /reject the plan deliberately.
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	logging.Info("[repl] approve result:\n%s", response)

	memResponse := response
	if busCtx != nil && busCtx.TaskState.LastError != "" {
		memResponse = "(approve ended in error — details omitted from memory)"
	}
	if response == "" || response == "(no result)" {
		fmt.Fprintln(r.out, emptyResponseHint(r.language))
	} else {
		r.renderBordered(response)
	}
	// Surface preserved worktree path when Fix 4 (keep_on_success)
	// fired — after a successful ModeApply with the yaml knob on,
	// busCtx.WorktreePath survives the orchestrator defer so the
	// user can `cd <path>` to review the applied bytes.
	if busCtx != nil && busCtx.WorktreePath != "" && busCtx.TaskState.LastError == "" {
		fmt.Fprintf(r.out, "  worktree preserved: %s\n", busCtx.WorktreePath)
	}
	// Failure path nudge — the orchestrator's renderVerifyFailure
	// adds an italic "Re-plan with /mode write..." line INSIDE the
	// markdown body, but bordered+italic is easy to miss. Print a
	// plain stderr-style hint OUTSIDE the bordered render so the
	// next-steps are unambiguous. /approve does NOT auto-retry-replan
	// (intentional: the approve gate's contract is "exactly the plan
	// I reviewed lands"), so the recovery path is explicit user
	// action — re-run plan-mode incorporating the failure.
	if busCtx != nil && busCtx.TaskState.LastError != "" {
		// No out-of-frame nudge on failure: renderVerifyFailure
		// already carries one short "下一步: /mode write ..." line
		// as its last block. A second nudge here duplicated that
		// guidance, and the past wording "approve 失败" mis-read
		// as "your /approve action was rejected" — when /approve
		// dispatched cleanly and the failure actually came from
		// the verify step. The bordered Result is the single
		// source of truth for the failure surface.
	} else {
		// Success path nudge + auto-switch to read mode. After a
		// clean /approve the next user question is almost always
		// "what does this do" / "how do I run it" / "did it
		// install everything" — read-mode questions, NOT another
		// plan. Pre-fix the REPL stayed sticky in plan mode and
		// the user's follow-up "ModuleNotFoundError 怎么修复" got
		// dispatched to the planner, which correctly returned
		// prose (no code change needed) but the orchestrator
		// surfaced the internal "planner did not call
		// emit_change_plan" error. Switching to read mode here
		// breaks that loop without taking away the user's ability
		// to /mode write again on demand.
		r.currentMode = types.ModeRead
		for _, line := range applyDoneNudge(r.language) {
			r.info(line)
		}
		// Commit 46: nudge the operator toward /worktree gc
		// after every Nth successful apply so disk doesn't fill
		// silently. Triggered only when keep_on_success is on
		// (otherwise worktrees are auto-discarded). Threshold
		// is per-session, soft.
		r.maybeNudgeWorktreeGC()
	}
	// KindPlan classifies this turn distinctly from chitchat /
	// pipeline so future /history filters + the memory retrieval
	// policy can surface plan outcomes explicitly.
	r.recordTurn(request, request, memResponse, memory.KindPlan)

	// Clear pendingPlanPath only on a CLEAN success (apply + verify
	// both passed). On TaskState.LastError, the plan is already
	// stamped applied_failed / verify_failed on disk; /approve a
	// second time would refuse on the status check, so keeping the
	// pointer is harmless and lets /plan show display the failed
	// plan + diff for post-mortem inspection.
	cleanSuccess := busCtx != nil && busCtx.TaskState.LastError == ""
	lastErr := ""
	if busCtx != nil {
		lastErr = busCtx.TaskState.LastError
	}
	r.markWriteWorkflowBatchAfterApprove(plan.ID, cleanSuccess, lastErr)

	// `--merge-to=<branch>` opt-in chains the merge step right
	// after a successful approve. Skipped on any failure path
	// because the worktree was already discarded (or contains a
	// half-applied plan) and merging would be meaningless.
	if cleanSuccess && mergeTo != "" {
		if busCtx == nil || busCtx.WorktreePath == "" {
			r.warn("%s", mergeToIgnoredNoWorktreeMsg(r.language, mergeTo))
		} else {
			r.runMerge(busCtx.WorktreePath, mergeTo)
		}
	}

	if cleanSuccess {
		r.pendingPlanPath = ""
	}
}

// parseApproveArgs parses every recognised /approve argument in
// one pass. Returns (planID, mergeTo, skipVerify):
//   - planID: positional plan-id (e.g. "plan-1730834521-12345") or
//     --plan-id=<id> long form. Empty when neither is supplied.
//   - mergeTo: --merge-to=<branch> (or --merge-to <branch>); empty
//     when not supplied.
//   - skipVerify: true when --skip-verify is on the command line.
//
// Tolerant of leading "/approve" prefix so tests can pass the raw
// flag string. Tolerant of split-token forms (`--merge-to <branch>`
// with whitespace separator). Unknown tokens are silently ignored
// — handleApproveCmd handles unknown shapes by falling through to
// the "load most recent pending plan" path.
func parseApproveArgs(line string) (planID, mergeTo string, skipVerify bool) {
	planID, mergeTo, skipVerify, _ = parseApproveArgsAll(line)
	return planID, mergeTo, skipVerify
}

// parseApproveArgsAll mirrors parseApproveArgs but additionally
// returns the --retry flag. Kept separate so existing call sites
// that don't care about retry don't need updating.
func parseApproveArgsAll(line string) (planID, mergeTo string, skipVerify, retry bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/approve"))
	if rest == "" {
		return "", "", false, false
	}
	tokens := strings.Fields(rest)
	for i, tok := range tokens {
		switch {
		case tok == "--skip-verify":
			skipVerify = true
		case tok == "--retry":
			// Accept partially_applied / unverified plans for re-
			// dispatch. Without this flag, those statuses are
			// rejected at the status gate below.
			retry = true
		case strings.HasPrefix(tok, "--merge-to="):
			mergeTo = strings.TrimSpace(strings.TrimPrefix(tok, "--merge-to="))
		case strings.HasPrefix(tok, "--plan-id="):
			planID = strings.TrimSpace(strings.TrimPrefix(tok, "--plan-id="))
		case tok == "--merge-to" && i+1 < len(tokens):
			mergeTo = tokens[i+1]
		case tok == "--plan-id" && i+1 < len(tokens):
			planID = tokens[i+1]
		case strings.HasPrefix(tok, "plan-") && planID == "":
			// Positional plan-id form. Only the first token shaped
			// like `plan-<...>` wins so a flag value that happens
			// to begin with `plan-` doesn't get re-interpreted.
			planID = tok
		}
	}
	return planID, mergeTo, skipVerify, retry
}

// parseMergeToArg is the legacy single-purpose helper kept as a
// thin wrapper for tests + downstream callers that already use it.
// New code should prefer parseApproveArgs.
func parseMergeToArg(line string) string {
	_, mergeTo, _ := parseApproveArgs(line)
	return mergeTo
}

// (commit 13's approveRetryProgress + approveRetryPendingPaths
// were removed in commit 17 #1. Their purpose — showing "X of Y
// already applied" on /approve --retry — was based on a wrong
// model: /approve dispatches a fresh ModeApply Run with a NEW
// worktree, so the prior partial-apply state never survives into
// the next Run. The helpers walked plan.WorktreePath which is a
// stale field by the time --retry fires. The honest message
// printed in handleApproveCmd's --retry branch replaces them.)

// runMerge is the shared body for the auto-merge tail of /approve
// and the explicit /merge handler. worktreePath must be the
// absolute path of a preserved worktree; targetBranch is the user-
// requested integration branch (== r.branch for fast-forward, any
// other name for cherry-pick onto a new branch). All print
// statements go through r.info / r.warn / r.success so the same
// terminal styling rules apply.
func (r *REPL) runMerge(worktreePath, targetBranch string) {
	if mainSetter, ok := r.runner.(modeSetter); ok && mainSetter == nil {
		_ = mainSetter // silence unused if interface check is the only use
	}
	mainRoot := r.repoRoot
	// Base branch defaults to the live git HEAD of the main repo
	// — that's the branch the worktree forked from when /approve
	// fired, so it's the natural fast-forward target. Falls back
	// to r.branch (the --branch flag) when HEAD is detached.
	base := defaultMergeTarget(mainRoot, r.branch)
	if targetBranch == "" {
		targetBranch = base
	}
	res, err := worktree.MergeIntoBranch(worktree.MergeOptions{
		MainRepoRoot: mainRoot,
		WorktreePath: worktreePath,
		BaseBranch:   base,
		TargetBranch: targetBranch,
		Mode:         worktree.MergeAuto,
	})
	if err != nil {
		if errors.Is(err, worktree.ErrNothingToMerge) {
			r.info(mergeNothingToDo(r.language, r.branch))
			return
		}
		for _, line := range mergeFailure(r.language, err.Error()) {
			r.warn("%s\n", line)
		}
		return
	}
	// Auto-switch back to read mode after a clean merge for the
	// same reason /approve does: the user's next question is
	// almost always a read-mode follow-up ("does it work" / "how
	// do I push it"). Sticky plan mode after /merge would route
	// the next question to the planner.
	r.currentMode = types.ModeRead
	for _, line := range mergeSuccess(r.language, res.Strategy, res.FinalBranch, len(res.CommitsLanded)) {
		r.info(line)
	}
	r.info(autoModeReadAfterMergeNudge(r.language))
	// Worktree successfully folded back — discard it so /worktree
	// list stays honest.
	if err := worktree.DiscardByPath(worktreePath, mainRoot); err != nil {
		logging.Warning("[repl] post-merge worktree discard failed: %v", err)
	}
	// Settle the plan as merged so the single-pending-plan invariant
	// recognises it as terminal and the next /mode write can proceed.
	// Pre-2026-04-30 only WorktreePath was cleared while Status
	// stayed `applied`, leaving the plan permanently "unsettled" from
	// the lifecycle's point of view.
	if id := r.lookupPlanIDByWorktreePath(worktreePath); id != "" {
		if err := r.planStore.Settle(id, types.PlanStatusMerged, ""); err != nil {
			logging.Warning("[repl] post-merge settle failed: %v", err)
		}
	}
}

// runMergeFromRef is the no-worktree counterpart to runMerge. It
// drives worktree.MergeFromRef when /merge finds a plan whose
// AppliedCommitSHA was persisted by the apply post-hook but whose
// worktree directory has been discarded (the keep_on_success=false
// default). Behaves the same way runMerge does on success: surfaces
// the strategy + commit count + final branch, and on failure echoes
// the raw error from MergeFromRef so operators can inspect.
//
// Note: there is no post-merge cleanup here — the worktree is
// already gone, and we deliberately leave the recovery ref in place
// so the operator can re-run /merge if they want to land on a
// different branch later. /worktree discard is the explicit
// teardown path.
func (r *REPL) runMergeFromRef(planID, ref, targetBranch string) {
	if r == nil || r.runner == nil {
		return
	}
	mainRoot := r.repoRoot
	base := defaultMergeTarget(mainRoot, r.branch)
	if targetBranch == "" {
		targetBranch = base
	}
	res, err := worktree.MergeFromRef(worktree.MergeFromRefOptions{
		MainRepoRoot: mainRoot,
		Ref:          ref,
		BaseBranch:   base,
		TargetBranch: targetBranch,
		Mode:         worktree.MergeAuto,
	})
	if err != nil {
		if errors.Is(err, worktree.ErrNothingToMerge) {
			r.info(mergeNothingToDo(r.language, r.branch))
			return
		}
		for _, line := range mergeFailure(r.language, err.Error()) {
			r.warn("%s\n", line)
		}
		return
	}
	// Same auto-switch-to-read on success as the worktree path.
	r.currentMode = types.ModeRead
	for _, line := range mergeSuccess(r.language, res.Strategy, res.FinalBranch, len(res.CommitsLanded)) {
		r.info(line)
	}
	r.info(autoModeReadAfterMergeNudge(r.language))
	// Same Settle-to-merged as the worktree path: lifecycle reaches
	// terminal state so the next /mode write can proceed.
	if r.planStore != nil && planID != "" {
		if err := r.planStore.Settle(planID, types.PlanStatusMerged, ""); err != nil {
			logging.Warning("[repl] post-merge-from-ref settle failed: %v", err)
		}
	}
	logging.Info("[repl] post-merge: plan %s landed via recovery ref %s (worktree was already discarded)",
		planID, ref)
}

// lookupPlanIDByWorktreePath walks PlanStore for the entry whose
// WorktreePath equals `path`. Empty string when no plan matches
// (worktree was already discarded, or the plan was never tracked).
// Used by /merge success path to call PlanStore.Settle on the
// matched plan, transitioning Status applied → merged.
func (r *REPL) lookupPlanIDByWorktreePath(path string) string {
	if r.planStore == nil || strings.TrimSpace(path) == "" {
		return ""
	}
	infos, err := r.planStore.List()
	if err != nil {
		return ""
	}
	for _, inf := range infos {
		full, lerr := r.planStore.Load(inf.ID)
		if lerr != nil || full == nil {
			continue
		}
		if full.WorktreePath == path {
			return inf.ID
		}
	}
	return ""
}

// discardWorktreeForRejected best-effort discards the worktree of
// the given plan when /reject or /plan clear settles it. Workspace
// and plan are tied; settling the plan should reclaim the disk
// space too. Failure logs but does not block the settle decision.
//
// Loads the plan to find WorktreePath; nothing to do when:
//   - PlanStore is nil
//   - plan has no WorktreePath (apply never ran with
//     pipeline_keep_worktree_on_success)
//   - the worktree directory was already discarded out-of-band
func (r *REPL) discardWorktreeForRejected(planID string) {
	if r.planStore == nil || planID == "" {
		return
	}
	plan, err := r.planStore.Load(planID)
	if err != nil || plan == nil || strings.TrimSpace(plan.WorktreePath) == "" {
		return
	}
	if err := worktree.DiscardByPath(plan.WorktreePath, r.repoRoot); err != nil {
		logging.Warning("[repl] discard worktree on settle (%s): %v", planID, err)
	}
}

// clearWorktreePathOnAppliedPlan walks PlanStore.List for the entry
// whose WorktreePath equals path and writes back an empty
// WorktreePath. Idempotent — running on a plan whose path was
// already cleared is a no-op.
//
// UpdatePlanStatusOnDisk treats empty worktreePath as "don't touch",
// so this helper does the load/marshal directly rather than route
// through it.
func (r *REPL) clearWorktreePathOnAppliedPlan(path string) error {
	if r.planStore == nil {
		return nil
	}
	infos, err := r.planStore.List()
	if err != nil {
		return err
	}
	for _, inf := range infos {
		full, lerr := r.planStore.Load(inf.ID)
		if lerr != nil || full == nil {
			continue
		}
		if full.WorktreePath != path {
			continue
		}
		full.WorktreePath = ""
		data, merr := json.MarshalIndent(full, "", "  ")
		if merr != nil {
			return merr
		}
		return os.WriteFile(inf.Path, data, 0o644)
	}
	return nil
}

// handleMergeCmd is the explicit `/merge [--branch=<name>]` slash
// command. Used after /approve preserved a worktree (i.e. with
// `pipeline_keep_worktree_on_success: true`) when the user wants
// a deliberate review-and-merge step rather than the inline
// `/approve --merge-to=` shortcut.
//
// Default target branch is the LIVE git HEAD (gitBranchProbe of
// the main repo), giving a fast-forward into whatever the user
// is currently on. Detached HEAD or git-missing falls back to
// r.branch (the --branch flag). `--branch=<name>` forces the
// new-branch path (PR workflow). Either way the helper goes
// through MergeIntoBranch's safety machinery: clean rollback on
// conflict, refusal on dirty working tree, no surprise pushes.
func (r *REPL) handleMergeCmd(line string) {
	if !r.writeEnabled {
		for _, line := range writeModeDisabled(r.language, "/merge", r.settingsPath) {
			r.warn("%s\n", line)
		}
		return
	}
	if r.planStore == nil {
		r.info(commandDisabled(r.language, "/merge", noPlanStoreReason(r.language)))
		return
	}
	// Default merge target = live git HEAD of the main repo.
	// Falls back to r.branch (the --branch flag) when HEAD is
	// detached or git is missing. Rationale: the user expects
	// `/merge` (no flags) to fast-forward whatever branch they're
	// currently on, not a stale branch name they configured at
	// codrax startup. /merge --branch=<name> still overrides.
	target := defaultMergeTarget(r.repoRoot, r.branch)
	includeFailed := false
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/merge"))
	// Group-level merge (commit 31): when the user passes
	// `/merge group-<id> [flags]`, fold the group's cumulative
	// worktree once + settle every phase plan as merged in a
	// single call. Detected by positional token starting with
	// "group-"; flags after it (e.g. --branch=<x>) still apply.
	for _, tok := range strings.Fields(rest) {
		if strings.HasPrefix(tok, "group-") {
			r.handleMergeGroupCmd(tok, rest)
			return
		}
	}
	for _, tok := range strings.Fields(rest) {
		switch {
		case strings.HasPrefix(tok, "--branch="):
			target = strings.TrimSpace(strings.TrimPrefix(tok, "--branch="))
		case tok == "--include-failed", tok == "--force", tok == "--skip-verify":
			// --include-failed (preferred historical name), --force, and
			// --skip-verify (UX alias) allow merging a verify_failed or
			// unverified plan. Use case: tests require infra the local box
			// can't run (integration DB, external API, GPU), or apply was
			// deliberately run with --skip-verify; the operator reviews the
			// diff, accepts that local verification is absent/red, and
			// merges to run tests in CI. Without this flag /merge skips
			// non-verified plans because the safe default is "only merge
			// code that passed verify".
			includeFailed = true
		}
	}
	if target == "" {
		target = r.branch
	}

	// Locate the most recent preserved worktree from PlanStore.
	// Default: Status=applied only. With --include-failed: also
	// accept Status=verify_failed (the operator override path).
	// applied_failed / rejected are NEVER mergeable: those plans
	// either never landed bytes (apply rejected by W1/W1b) or were
	// deliberately discarded by the operator.
	infos, err := r.planStore.List()
	if err != nil {
		r.errorf("%s", mergeListPlansFailedMsg(r.language, err))
		return
	}
	// Two recovery surfaces, in priority order:
	//   1. A preserved worktree directory (keep_on_success=true path).
	//      Full MergeIntoBranch flow with rollback-on-conflict.
	//   2. The pinned recovery ref refs/codrax/applied/<plan-id>
	//      written by the apply post-hook. Works regardless of
	//      keep_on_success — the worktree is gone but the apply
	//      commit lives in main_repo/.git/objects, kept reachable
	//      by this ref. /merge falls back to it via MergeFromRef.
	var (
		wt          string
		planID      string
		planStatus  string
		recoverySHA string // populated when surface 1 misses but surface 2 hits
		recoveryRef string
	)
	for _, inf := range infos {
		// unverified plans (apply succeeded; runner discovered no
		// tests) are a valid /merge candidate alongside verify_failed
		// when --include-failed is supplied — the bytes are on disk,
		// the operator just couldn't verify locally and is choosing
		// to land them anyway. partially_applied stays excluded from
		// /merge entirely: a partial diff would land an incoherent
		// state on main, which /merge can never be the right tool for.
		eligible := inf.Status == types.PlanStatusApplied ||
			(includeFailed && (inf.Status == types.PlanStatusVerifyFailed ||
				inf.Status == types.PlanStatusUnverified))
		if !eligible {
			continue
		}
		full, lerr := r.planStore.Load(inf.ID)
		if lerr != nil || full == nil {
			continue
		}
		// Surface 1: preserved worktree path that still exists.
		if full.WorktreePath != "" {
			if _, statErr := os.Stat(full.WorktreePath); statErr == nil {
				wt = full.WorktreePath
				planID = full.ID
				planStatus = full.Status
				break
			}
		}
		// Surface 2: pinned recovery ref. SHA was persisted by the
		// apply post-hook; the ref name is derived from plan ID.
		if recoverySHA == "" && strings.TrimSpace(full.AppliedCommitSHA) != "" {
			recoverySHA = full.AppliedCommitSHA
			recoveryRef = worktree.AppliedRef(full.ID)
			planID = full.ID
			planStatus = full.Status
		}
	}
	if includeFailed && planStatus == types.PlanStatusVerifyFailed {
		for _, line := range mergeForceFailedWarning(r.language, planID) {
			r.warn("%s\n", line)
		}
	}
	if includeFailed && planStatus == types.PlanStatusUnverified {
		for _, line := range mergeSkipVerifyWarning(r.language, planID) {
			r.warn("%s\n", line)
		}
	}
	if wt == "" && recoveryRef == "" {
		// Commit 10 #4: when the no-eligible-plan path fires, scan
		// once more for partially_applied / unverified plans so we
		// can name the actionable next step. Without this, the user
		// sees a generic "no preserved worktree" message and can't
		// tell whether they have nothing in flight, or have a
		// partial state that /merge specifically refuses to touch.
		var partialID, unverifiedID string
		for _, inf := range infos {
			if partialID == "" && inf.Status == types.PlanStatusPartiallyApplied {
				partialID = inf.ID
			}
			if unverifiedID == "" && inf.Status == types.PlanStatusUnverified && !includeFailed {
				unverifiedID = inf.ID
			}
		}
		if partialID != "" {
			r.warn("/merge skipped: plan %s is partially_applied (some files landed before apply hit a rejection). half-applying to main would land incoherent code; use `/approve --retry` to finish, or `/reject` to discard the partial state.\n",
				partialID)
			return
		}
		if unverifiedID != "" {
			r.info(mergeUnverifiedNeedsSkipVerifyMsg(r.language, unverifiedID))
			return
		}
		for _, line := range mergeNoApplyYet(r.language) {
			r.info(line)
		}
		return
	}

	// Probe the merge plan first so the confirm prompt can describe
	// what will land. We skip this when not interactive — scripted
	// callers want one-shot semantics, not a y/N round-trip.
	if r.interactive() {
		// Best-effort enumerate strategy + commit count by running
		// the same logic in dry-mode. We don't have a separate
		// "preview" path; instead we synthesize a confirm title
		// from BaseBranch == TargetBranch (ff likely) vs different
		// (cherry-pick branch). The post-merge surface still tells
		// the truth even if our prediction was off.
		strategy := "cherry_pick_branch"
		if target == r.branch {
			strategy = "fast_forward"
		}
		title := mergeConfirmTitle(r.language, strategy, target, 1)
		confirmed := false
		if err := huh.NewConfirm().
			Title(title).
			Affirmative("Yes").
			Negative("No").
			Value(&confirmed).
			Run(); err != nil {
			confirmed = false
		}
		if !confirmed {
			r.info(approveCancelled(r.language))
			return
		}
	}
	if wt != "" {
		logging.Info("[repl] /merge plan=%s target=%s worktree=%s", planID, target, wt)
		r.runMerge(wt, target)
		return
	}
	logging.Info("[repl] /merge plan=%s target=%s ref=%s (worktree was discarded; using recovery ref)",
		planID, target, recoveryRef)
	r.runMergeFromRef(planID, recoveryRef, target)
}

type providerOperationResult struct {
	Status              operation.OperationStatus
	Provider            string
	Tool                string
	Summary             string
	PayloadRef          string
	ArtifactRefs        []string
	VerificationStatus  string
	VerificationSummary string
	Error               string
	Observations        int
	NextActions         []operation.WorkflowNextAction
	ReturnAction        operation.WorkflowNextAction
	WorkflowState       operation.WorkflowState
	WorkflowDiagnostics []string
}

func (r *REPL) handleProviderOperationApproveCmd(line string) {
	_ = line
	pending := r.pendingProviderOperation
	if pending == nil {
		r.info(operationNoPendingMsg(r.language))
		return
	}
	r.pendingProviderOperation = nil
	r.clearPendingOperationState()
	r.executeProviderOperationFlow(context.Background(), *pending)
}

func (r *REPL) executeProviderOperationFlow(ctx context.Context, first pendingProviderOperation) {
	if ctx == nil {
		ctx = context.Background()
	}
	current := &first
	var records []providerOperationResultRecord
	for actions := 0; current != nil && actions < maxOperationProviderWorkflowActions; actions++ {
		result := r.executeProviderOperation(ctx, *current)
		r.queueProviderNextAction(current, &result)
		rec := providerOperationResultRecord{Plan: current.Plan, Request: current.Request, Result: result}
		records = append(records, rec)
		r.appendProviderOperationResult(current.Plan, current.Request, result)
		r.renderProviderOperationRoundResult(current.Plan, result)
		if result.Status != operation.StatusExecuted {
			break
		}
		next := r.pendingProviderOperation
		if next == nil {
			break
		}
		if !next.Plan.CanExecute {
			break
		}
		r.pendingProviderOperation = nil
		current = next
		r.startOperationExecutionSpinner()
	}
	if len(records) == 0 {
		return
	}
	if r.maybeContinueProviderOperationWithCommand(ctx, first, records) {
		return
	}
	r.startOperationSynthesisSpinner()
	msg, thoughts := r.providerOperationFinalMessage(ctx, first.Request.Text, records)
	if pending := r.pendingProviderOperation; pending != nil {
		if strings.TrimSpace(msg) != "" {
			msg += "\n\n"
		}
		if isZh(r.language) {
			msg += "后续动作需要批准后继续：\n\n"
		} else {
			msg += "The next action is waiting for approval:\n\n"
		}
		msg += operationPlanMarkdown(r.language, pending.Plan)
	}
	if r.pendingProviderOperation != nil {
		r.savePendingOperationState()
	} else {
		r.clearPendingOperationState()
	}
	r.finishOperationFinalAnswerHeader()
	r.emitOperationVisibleThoughts(thoughts)
	r.renderBordered(msg)
	r.lastAnswerOrigin = replAnswerOriginProviderOperationFinal
	r.recordTurn(first.Display, first.Request.Text, msg, memory.KindPipeline)
}

func (r *REPL) maybeContinueProviderOperationWithCommand(ctx context.Context, first pendingProviderOperation, records []providerOperationResultRecord) bool {
	if r.pendingProviderOperation != nil || len(records) == 0 || r.operationPlanner == nil {
		return false
	}
	evaluator, ok := r.operationPlanner.(ProviderOperationEvaluator)
	if !ok || evaluator == nil {
		return false
	}
	last := records[len(records)-1]
	if last.Result.Status != operation.StatusExecuted {
		return false
	}
	r.startOperationSynthesisSpinner()
	eval, err := evaluator.EvaluateProviderOperation(ctx, first.Request.Text, records, r.language)
	r.emitReplLLMTrace(r.operationPlanner, "provider_operation_evaluator", types.AgentName("operation_planner"), types.PipelineStage("operation"))
	if err != nil {
		logging.Warning("[repl/operation] provider operation evaluation failed: %v", err)
		return false
	}
	logging.Info("[repl/operation] provider evaluation status=%s confidence=%q reason=%q materials=%d",
		eval.Status, oneLineClamp(eval.Confidence, 40), oneLineClamp(eval.Reason, 180), len(eval.Materials))
	if eval.Status != operation.EvalContinueCommand {
		return false
	}
	planner, ok := r.operationPlanner.(CommandOperationPlannerWithHandoff)
	if !ok || planner == nil {
		logging.Warning("[repl/operation] provider evaluation requested command continuation but command planner does not support handoff")
		return false
	}
	policy := TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "operation_evaluator",
		RiskLevel:            "low",
		TargetSurface:        "desktop",
		Confidence:           1.0,
		Reason:               firstNonEmptyString(eval.Reason, "provider operation evaluator requested bounded command follow-up"),
	}
	handoff := r.renderCommandOperationHandoff()
	if strings.TrimSpace(eval.Reason) != "" || len(eval.Materials) > 0 {
		if strings.TrimSpace(handoff) != "" {
			handoff += "\n\n"
		}
		handoff += "operation_evaluation status=continue_command"
		if strings.TrimSpace(eval.Reason) != "" {
			handoff += fmt.Sprintf(" reason=%q", oneLineClamp(eval.Reason, 240))
		}
		if rendered := operation.RenderMaterialsForPrompt(eval.Materials, 8); rendered != "" {
			handoff += "\n" + rendered
		}
	}
	snapshot := r.commandOperationCapabilitySnapshot()
	planned, err := planner.PlanCommandOperationWithHandoff(ctx, first.Request.Text, r.repoRoot, policy, snapshot, handoff)
	r.emitReplLLMTrace(r.operationPlanner, "command_operation_planner", types.AgentName("operation_planner"), types.PipelineStage("operation"))
	if err != nil {
		logging.Warning("[repl/operation] provider-to-command continuation planning failed: %v", err)
		return false
	}
	plan := operation.BuildCommandOperationPlan(planned, r.operationPolicy)
	continuationDecision := operation.DecideCommandPlanApproval(r.operationPolicy, plan, operation.CommandApprovalOptions{Phase: operation.CommandApprovalContinuation})
	plan = operation.ApplyCommandPlanApprovalDecision(plan, continuationDecision)
	logging.Info("[repl/operation] provider-to-command continuation plan status=%s risk=%q approval=%q decision=%s reason_code=%s steps=%d",
		plan.Status, plan.RiskLevel, plan.ApprovalMode, continuationDecision.Action, continuationDecision.ReasonCode, len(plan.Steps))
	if plan.Status == operation.StatusReady && continuationDecision.AutoExecute() {
		msg := commandOperationContinuationIntro(r.language, plan)
		msg += "\n\n"
		msg += commandOperationAutoExecuteMarkdown(r.language, plan)
		r.finishCommandOperationPlanHeader(plan)
		r.renderBorderedCompact(msg)
		r.executeCommandOperationPlanAttempt(plan, first.Request.Text, first.Display, 0, nil)
		return true
	}
	if plan.Status == operation.StatusReady {
		r.pendingOperation = &plan
		r.savePendingOperationState()
	}
	r.operationHistory = append(r.operationHistory, plan)
	msg := commandOperationContinuationIntro(r.language, plan)
	msg += "\n\n"
	msg += commandOperationPlanMarkdown(r.language, plan)
	r.finishOperationRouteSpinner(plan.Status)
	r.renderBorderedCompact(msg)
	r.recordTurn(first.Display, first.Request.Text, msg, memory.KindPipeline)
	return true
}

func (r *REPL) renderProviderOperationRoundResult(plan operation.Plan, result providerOperationResult) {
	msg := providerOperationResultMarkdown(r.language, plan, result)
	if strings.TrimSpace(msg) == "" {
		return
	}
	r.finishOperationRouteSpinner(result.Status)
	r.renderBordered(msg)
}

func (r *REPL) providerOperationFinalMessage(ctx context.Context, userLine string, records []providerOperationResultRecord) (string, []string) {
	var details strings.Builder
	for i, rec := range records {
		if i > 0 {
			details.WriteString("\n\n")
		}
		details.WriteString(providerOperationResultMarkdown(r.language, rec.Plan, rec.Result))
	}
	detailText := strings.TrimSpace(details.String())
	if len(records) == 0 {
		return operationFinalReportFallback(r.language, operation.StatusFailed, 0), nil
	}
	last := records[len(records)-1]
	fallback := operationFinalReportFallback(r.language, last.Result.Status, len(records))
	answerer, ok := r.operationPlanner.(ProviderOperationAnswerer)
	if !ok || answerer == nil {
		return fallback, nil
	}
	answer, err := answerer.AnswerProviderOperationResult(ctx, userLine, records, r.language)
	r.emitReplLLMTrace(r.operationPlanner, "provider_operation_answerer", types.AgentName("operation_planner"), types.PipelineStage("operation"))
	if err != nil {
		logging.Warning("[repl/operation] provider result answer synthesis failed: %v", err)
		return fallback, nil
	}
	answer = strings.TrimSpace(answer)
	thoughts, answer := splitVisibleThinkBlocks(answer)
	if answer == "" {
		return fallback, thoughts
	}
	return operationFinalReportWithDetails(r.language, answer, detailText), thoughts
}

func (r *REPL) executeProviderOperation(ctx context.Context, pending pendingProviderOperation) providerOperationResult {
	plan := pending.Plan
	r.renderOperationProgress(providerOperationProgressMsg(r.language, pending))
	provider := strings.TrimSpace(plan.Provider)
	switch {
	case strings.HasPrefix(provider, "mcp:"):
		return r.executeMCPOperationProvider(ctx, pending)
	case strings.HasPrefix(provider, "skill:"):
		return r.executeLocalOperationSkillProvider(ctx, pending)
	default:
		return providerOperationResult{
			Status:   operation.StatusFailed,
			Provider: plan.Provider,
			Tool:     plan.ProviderTool,
			Error:    fmt.Sprintf("unsupported operation provider %q", plan.Provider),
		}
	}
}

func providerExecutionEnvelope(plan operation.Plan, pending pendingProviderOperation) map[string]any {
	out := map[string]any{
		"request":               pending.Request.Text,
		"operation":             firstNonEmptyString(pending.Request.Operation, pending.Request.OperationKind, plan.Kind),
		"operation_kind":        plan.Kind,
		"target_surface":        plan.TargetSurface,
		"risk_level":            plan.RiskLevel,
		"side_effects":          plan.SideEffects,
		"requires_confirmation": plan.RequiresConfirmation,
		"provider":              plan.Provider,
		"tool":                  firstNonEmptyString(plan.ProviderTool, "run"),
	}
	if len(pending.Input) > 0 {
		out["input"] = pending.Input
	}
	if !pending.WorkflowState.IsZero() {
		out["workflow_state"] = pending.WorkflowState
	}
	if pending.WorkflowDepth > 0 {
		out["workflow_depth"] = pending.WorkflowDepth
	}
	return out
}

func (r *REPL) executeMCPOperationProvider(ctx context.Context, pending pendingProviderOperation) providerOperationResult {
	plan := pending.Plan
	result := providerOperationResult{
		Status:   operation.StatusFailed,
		Provider: plan.Provider,
		Tool:     plan.ProviderTool,
	}
	serverName := strings.TrimPrefix(strings.TrimSpace(plan.Provider), "mcp:")
	if serverName == "" || strings.TrimSpace(plan.ProviderTool) == "" {
		result.Error = "operation provider is missing MCP server or operation_tool"
		return result
	}
	if r.mcpServers == nil {
		result.Error = "MCP registry is not available for operation provider execution"
		return result
	}
	if err := r.ensureLazyOperationMCPServer(serverName); err != nil {
		result.Error = err.Error()
		return result
	}
	args := providerExecutionEnvelope(plan, pending)
	raw, err := json.Marshal(args)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	resp, err := r.mcpServers.CallTool(serverName, plan.ProviderTool, raw)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Observations = len(resp.Observations)
	result.PayloadRef = firstNonEmptyString(resp.PayloadRef, resp.RawRef, resp.RowSetRef)
	result.ArtifactRefs = providerArtifactRefsFromMCPResponse(resp)
	result.Summary = strings.TrimSpace(resp.Summary)
	if result.Summary == "" && result.Observations > 0 {
		result.Summary = fmt.Sprintf("MCP provider returned %d typed observation(s).", result.Observations)
	}
	if !resp.Success {
		result.Error = firstNonEmptyString(resp.Summary, "MCP operation provider returned success=false")
		return result
	}
	result.Status = operation.StatusExecuted
	if result.Summary != "" && utf8.RuneCountInString(result.Summary) > 4096 {
		ref, preview := r.storeProviderOperationPayload(plan.Provider, plan.ProviderTool, result.Summary)
		if ref != "" {
			result.PayloadRef = firstNonEmptyString(result.PayloadRef, ref)
			result.Summary = preview
		}
	}
	return result
}

const (
	defaultOperationSkillTimeout       = 30 * time.Second
	defaultOperationSkillMaxOutputSize = 256 * 1024
	defaultOperationSkillInputMode     = "stdin_json"
)

func (r *REPL) executeLocalOperationSkillProvider(ctx context.Context, pending pendingProviderOperation) providerOperationResult {
	plan := pending.Plan
	result := providerOperationResult{
		Status:   operation.StatusFailed,
		Provider: plan.Provider,
		Tool:     plan.ProviderTool,
	}
	skillName := strings.TrimPrefix(strings.TrimSpace(plan.Provider), "skill:")
	cfg, ok := r.localOperationSkillConfig(skillName)
	if !ok {
		result.Error = fmt.Sprintf("local operation skill %q is not configured", skillName)
		return result
	}
	if strings.TrimSpace(cfg.Command) == "" {
		result.Error = fmt.Sprintf("local operation skill %q has no command", skillName)
		return result
	}
	envelope := providerExecutionEnvelope(plan, pending)
	envelope["repo_root"] = r.repoRoot
	raw, err := json.Marshal(envelope)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	timeout := defaultOperationSkillTimeout
	if cfg.TimeoutMS != nil && *cfg.TimeoutMS > 0 {
		timeout = time.Duration(*cfg.TimeoutMS) * time.Millisecond
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, display, err := r.buildLocalOperationSkillCommand(runCtx, cfg, raw)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	maxOutput := defaultOperationSkillMaxOutputSize
	if cfg.MaxOutputBytes != nil && *cfg.MaxOutputBytes > 0 {
		maxOutput = *cfg.MaxOutputBytes
	}
	stdoutCap, err := r.newProviderProcessOutputCapture(plan.Provider, "stdout", maxOutput)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer stdoutCap.Close()
	stderrCap, err := r.newProviderProcessOutputCapture(plan.Provider, "stderr", maxOutput/4)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer stderrCap.Close()
	cmd.Stdout = stdoutCap
	cmd.Stderr = stderrCap
	runErr := cmd.Run()
	stdoutText := strings.TrimSpace(stdoutCap.String())
	stderrText := strings.TrimSpace(stderrCap.String())
	if stdoutCap.Truncated() {
		result.PayloadRef = stdoutCap.Path()
	}
	parsed, parsedOK := parseLocalOperationSkillJSONResult(stdoutText)
	if parsedOK {
		result.Summary = strings.TrimSpace(parsed.Summary)
		result.PayloadRef = firstNonEmptyString(result.PayloadRef, parsed.PayloadRef)
		result.ArtifactRefs = cleanOperationRefs(parsed.ArtifactRefs)
		result.VerificationStatus = strings.TrimSpace(parsed.VerificationStatus)
		result.VerificationSummary = strings.TrimSpace(parsed.VerificationSummary)
		result.Observations = parsed.Observations
		result.NextActions = parsed.NextActions
		result.ReturnAction = parsed.ReturnAction
		result.WorkflowState = parsed.WorkflowState
		if parsed.Error != "" {
			result.Error = parsed.Error
		}
	} else {
		result.Summary = stdoutText
	}
	if result.Summary == "" && stdoutCap.Truncated() {
		result.Summary = fmt.Sprintf("local operation skill output exceeded inline preview; full stdout saved to %s", stdoutCap.Path())
	}
	if stderrCap.Truncated() && result.Error == "" {
		result.Error = fmt.Sprintf("stderr exceeded inline preview; full stderr saved to %s", stderrCap.Path())
	}
	if runErr != nil {
		result.Error = firstNonEmptyString(result.Error, localOperationSkillRunError(runErr, runCtx.Err(), display, stderrText))
		return result
	}
	if parsedOK && parsed.Success != nil && !*parsed.Success {
		result.Error = firstNonEmptyString(result.Error, "local operation skill returned success=false")
		return result
	}
	result.Status = operation.StatusExecuted
	if result.Summary != "" && utf8.RuneCountInString(result.Summary) > 4096 {
		ref, preview := r.storeProviderOperationPayload(plan.Provider, firstNonEmptyString(plan.ProviderTool, "run"), result.Summary)
		if ref != "" {
			result.PayloadRef = firstNonEmptyString(result.PayloadRef, ref)
			result.Summary = preview
		}
	}
	r.markOperationProviderLoaded(plan.Provider)
	logging.Info("[repl/operation] local operation skill executed provider=%s command=%q", plan.Provider, display)
	return result
}

const maxOperationProviderWorkflowDepth = 6
const maxOperationProviderWorkflowActions = 24

func (r *REPL) startProviderWorkflow(pending *pendingProviderOperation) {
	if pending == nil {
		return
	}
	wfID := fmt.Sprintf("provider-workflow-%d", len(r.providerOperationResults)+1)
	wf := operation.NewWorkflowInstance(wfID, pending.Request.Text, maxOperationProviderWorkflowDepth, maxOperationProviderWorkflowActions)
	action, ok, diag := wf.AddAction("", "", operation.WorkflowAction{
		Plan:          pending.Plan,
		Request:       pending.Request,
		Input:         pending.Input,
		WorkflowState: pending.WorkflowState,
		Depth:         pending.WorkflowDepth,
		Status:        operation.WorkflowActionPending,
	})
	if !ok {
		logging.Warning("[repl/operation] create provider workflow failed: %s", diag)
		r.providerWorkflow = nil
		return
	}
	pending.WorkflowActionID = action.ID
	r.providerWorkflow = &wf
}

func (r *REPL) queueProviderNextAction(pending *pendingProviderOperation, result *providerOperationResult) {
	if pending == nil || result == nil {
		return
	}
	r.completeProviderWorkflowAction(pending, result)
	if result.Status != operation.StatusExecuted || len(result.NextActions) == 0 {
		r.advanceProviderWorkflow()
		return
	}
	if pending.WorkflowDepth >= maxOperationProviderWorkflowDepth {
		result.WorkflowDiagnostics = append(result.WorkflowDiagnostics, fmt.Sprintf("workflow depth limit %d reached; no next action was queued", maxOperationProviderWorkflowDepth))
		r.advanceProviderWorkflow()
		return
	}
	queued := 0
	for i, action := range result.NextActions {
		plan, req, diag, ok := r.providerPlanFromWorkflowAction(pending, action)
		if diag != "" {
			result.WorkflowDiagnostics = append(result.WorkflowDiagnostics, fmt.Sprintf("next_action[%d]: %s", i, diag))
		}
		if !ok {
			continue
		}
		nextPending := pendingProviderOperation{
			Plan:          plan,
			Request:       req,
			Display:       firstNonEmptyString(action.Request, pending.Display, pending.Request.Text),
			Input:         action.Input,
			WorkflowState: result.WorkflowState,
			WorkflowDepth: pending.WorkflowDepth + 1,
		}
		if r.enqueueProviderWorkflowAction(pending, &nextPending, action, operation.WorkflowEdgeNext, result) {
			queued++
		}
	}
	if strings.TrimSpace(result.ReturnAction.Compact()) != "" {
		plan, req, diag, ok := r.providerPlanFromWorkflowAction(pending, result.ReturnAction)
		if diag != "" {
			result.WorkflowDiagnostics = append(result.WorkflowDiagnostics, fmt.Sprintf("return_action: %s", diag))
		}
		if ok {
			returnPending := pendingProviderOperation{
				Plan:          plan,
				Request:       req,
				Display:       firstNonEmptyString(result.ReturnAction.Request, pending.Display, pending.Request.Text),
				Input:         result.ReturnAction.Input,
				WorkflowState: result.WorkflowState,
				WorkflowDepth: pending.WorkflowDepth + 1,
			}
			if r.enqueueProviderWorkflowAction(pending, &returnPending, result.ReturnAction, operation.WorkflowEdgeReturn, result) {
				queued++
			}
		}
	}
	if queued > 0 {
		result.WorkflowDiagnostics = append(result.WorkflowDiagnostics, fmt.Sprintf("queued next workflow action(s): %d; run /approve to continue", queued))
	}
	r.advanceProviderWorkflow()
}

func (r *REPL) completeProviderWorkflowAction(pending *pendingProviderOperation, result *providerOperationResult) {
	if r.providerWorkflow == nil || pending == nil || strings.TrimSpace(pending.WorkflowActionID) == "" {
		return
	}
	for i := range r.providerWorkflow.Actions {
		if r.providerWorkflow.Actions[i].ID != pending.WorkflowActionID {
			continue
		}
		switch result.Status {
		case operation.StatusExecuted:
			r.providerWorkflow.Actions[i].Status = operation.WorkflowActionExecuted
		case operation.StatusFailed:
			r.providerWorkflow.Actions[i].Status = operation.WorkflowActionFailed
		default:
			r.providerWorkflow.Actions[i].Status = operation.WorkflowActionSkipped
		}
		if len(result.WorkflowDiagnostics) > 0 {
			r.providerWorkflow.Actions[i].Diagnostics = append(r.providerWorkflow.Actions[i].Diagnostics, result.WorkflowDiagnostics...)
		}
		break
	}
	r.removeProviderWorkflowQueueID(pending.WorkflowActionID)
}

func (r *REPL) enqueueProviderWorkflowAction(parent *pendingProviderOperation, pending *pendingProviderOperation, suggested operation.WorkflowNextAction, edgeKind operation.WorkflowEdgeKind, result *providerOperationResult) bool {
	if pending == nil {
		return false
	}
	if r.providerWorkflow == nil {
		r.pendingProviderOperation = pending
		return true
	}
	action, ok, diag := r.providerWorkflow.AddAction(parent.WorkflowActionID, edgeKind, operation.WorkflowAction{
		SourceProvider: firstNonEmptyString(parent.Plan.Provider, parent.Request.Source),
		NextAction:     suggested,
		Plan:           pending.Plan,
		Request:        pending.Request,
		Input:          pending.Input,
		WorkflowState:  pending.WorkflowState,
		Depth:          pending.WorkflowDepth,
		Status:         operation.WorkflowActionQueued,
	})
	if diag != "" {
		result.WorkflowDiagnostics = append(result.WorkflowDiagnostics, diag)
	}
	if !ok {
		return false
	}
	pending.WorkflowActionID = action.ID
	return true
}

func (r *REPL) advanceProviderWorkflow() {
	if r.providerWorkflow == nil {
		return
	}
	for len(r.providerWorkflow.Queue) > 0 {
		id := strings.TrimSpace(r.providerWorkflow.Queue[0])
		action, ok := r.providerWorkflow.ActionByID(id)
		if !ok {
			r.providerWorkflow.Queue = r.providerWorkflow.Queue[1:]
			continue
		}
		if action.Status != operation.WorkflowActionQueued && action.Status != operation.WorkflowActionPending {
			r.providerWorkflow.Queue = r.providerWorkflow.Queue[1:]
			continue
		}
		r.providerWorkflow.CurrentID = id
		r.setProviderWorkflowActionStatus(id, operation.WorkflowActionPending)
		r.pendingProviderOperation = r.pendingProviderOperationFromWorkflowAction(action)
		return
	}
	r.providerWorkflow.CurrentID = ""
	r.pendingProviderOperation = nil
}

func (r *REPL) pendingProviderOperationFromWorkflowAction(action operation.WorkflowAction) *pendingProviderOperation {
	return &pendingProviderOperation{
		Plan:             action.Plan,
		Request:          action.Request,
		Display:          firstNonEmptyString(action.Request.Text, action.NextAction.Request),
		Input:            action.Input,
		WorkflowState:    action.WorkflowState,
		WorkflowDepth:    action.Depth,
		WorkflowActionID: action.ID,
	}
}

func (r *REPL) setProviderWorkflowActionStatus(id string, status operation.WorkflowActionStatus) {
	if r.providerWorkflow == nil {
		return
	}
	for i := range r.providerWorkflow.Actions {
		if r.providerWorkflow.Actions[i].ID == id {
			r.providerWorkflow.Actions[i].Status = status
			return
		}
	}
}

func (r *REPL) removeProviderWorkflowQueueID(id string) {
	if r.providerWorkflow == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	filtered := r.providerWorkflow.Queue[:0]
	for _, existing := range r.providerWorkflow.Queue {
		if strings.TrimSpace(existing) == id {
			continue
		}
		filtered = append(filtered, existing)
	}
	r.providerWorkflow.Queue = filtered
}

func (r *REPL) providerPlanFromWorkflowAction(pending *pendingProviderOperation, action operation.WorkflowNextAction) (operation.Plan, operation.Request, string, bool) {
	providerName := strings.TrimSpace(action.Provider)
	kind := firstNonEmptyString(action.OperationKind, action.Operation)
	target := strings.TrimSpace(action.TargetSurface)
	providers := r.operationProviders
	if providerName != "" {
		provider, ok := r.operationProviderByName(providerName)
		if !ok {
			return operation.Plan{}, operation.Request{}, fmt.Sprintf("configured provider %q not found", providerName), false
		}
		providers = []operation.ProviderInfo{provider}
		kind = firstNonEmptyString(kind, provider.Kind)
		if target == "" && len(provider.Surfaces) == 1 {
			target = provider.Surfaces[0]
		}
		if action.Tool != "" && provider.ToolName != "" && action.Tool != provider.ToolName {
			// The configured provider descriptor remains authoritative.
			action.Tool = provider.ToolName
		}
	}
	req := operation.Request{
		Text:                 firstNonEmptyString(action.Request, pending.Request.Text),
		Operation:            firstNonEmptyString(action.Operation, kind),
		OperationKind:        firstNonEmptyString(kind, pending.Request.OperationKind, pending.Request.Operation),
		Source:               "external_tool",
		NeedsRepoAccess:      false,
		RiskLevel:            firstNonEmptyString(action.RiskLevel, "medium"),
		SideEffects:          cleanOperationRefs(action.SideEffects),
		TargetSurface:        firstNonEmptyString(target, pending.Request.TargetSurface),
		RequiresConfirmation: true,
	}
	plan := operation.BuildPlan(req, providers)
	if strings.TrimSpace(plan.Provider) == "" || strings.TrimSpace(plan.ProviderTool) == "" {
		return plan, req, firstNonEmptyString(plan.MissingCapability, "next action did not match a configured operation provider"), false
	}
	return plan, req, "", true
}

func (r *REPL) operationProviderByName(providerName string) (operation.ProviderInfo, bool) {
	providerName = strings.TrimSpace(providerName)
	for _, provider := range r.operationProviders {
		if strings.TrimSpace(provider.Name) == providerName {
			return provider, true
		}
	}
	return operation.ProviderInfo{}, false
}

func (r *REPL) localOperationSkillConfig(skillName string) (types.OperationSkillConfig, bool) {
	skillName = strings.TrimSpace(skillName)
	for _, cfg := range r.operationSkillConfigs {
		if strings.TrimSpace(cfg.Name) == skillName {
			return cfg, true
		}
	}
	return types.OperationSkillConfig{}, false
}

func (r *REPL) buildLocalOperationSkillCommand(ctx context.Context, cfg types.OperationSkillConfig, inputJSON []byte) (*exec.Cmd, string, error) {
	workDir := strings.TrimSpace(cfg.WorkDir)
	if workDir == "" {
		workDir = strings.TrimSpace(r.repoRoot)
	}
	if workDir != "" && !filepath.IsAbs(workDir) {
		root := strings.TrimSpace(r.repoRoot)
		if root == "" {
			root = "."
		}
		workDir = filepath.Join(root, workDir)
	}
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return nil, "", fmt.Errorf("local operation skill command is empty")
	}
	execPath := command
	if hasPathSeparator(command) && !filepath.IsAbs(command) && workDir != "" {
		execPath = filepath.Join(workDir, command)
	}
	args := append([]string{}, cfg.Args...)
	inputMode := strings.TrimSpace(cfg.InputMode)
	if inputMode == "" {
		inputMode = defaultOperationSkillInputMode
	}
	switch inputMode {
	case "stdin_json":
	case "args_json":
		args = append(args, string(inputJSON))
	default:
		return nil, "", fmt.Errorf("unsupported local operation skill input_mode %q", inputMode)
	}
	cmd := exec.CommandContext(ctx, execPath, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if inputMode == "stdin_json" {
		cmd.Stdin = strings.NewReader(string(inputJSON))
	}
	cmd.Env = localOperationSkillEnv(cfg)
	display := strings.TrimSpace(command + " " + strings.Join(args, " "))
	return cmd, display, nil
}

func hasPathSeparator(path string) bool {
	return strings.Contains(path, "/") || strings.Contains(path, "\\")
}

func localOperationSkillEnv(cfg types.OperationSkillConfig) []string {
	inherit := true
	if cfg.InheritEnv != nil {
		inherit = *cfg.InheritEnv
	}
	envMap := map[string]string{}
	if inherit {
		for _, item := range os.Environ() {
			if k, v, ok := strings.Cut(item, "="); ok {
				envMap[k] = v
			}
		}
	}
	for k, v := range cfg.Env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		envMap[k] = v
	}
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func localOperationSkillRunError(err error, ctxErr error, display, stderrText string) string {
	if ctxErr != nil {
		return fmt.Sprintf("local operation skill timed out or was cancelled while running %q: %v", display, ctxErr)
	}
	base := err.Error()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		base = fmt.Sprintf("local operation skill exited with code %d", exitErr.ExitCode())
	}
	if strings.TrimSpace(stderrText) != "" {
		return base + ": " + oneLineClamp(stderrText, 800)
	}
	return base
}

type localOperationSkillJSONResult struct {
	Success             *bool
	Summary             string
	PayloadRef          string
	ArtifactRefs        []string
	VerificationStatus  string
	VerificationSummary string
	Observations        int
	Error               string
	NextActions         []operation.WorkflowNextAction
	ReturnAction        operation.WorkflowNextAction
	WorkflowState       operation.WorkflowState
}

func parseLocalOperationSkillJSONResult(text string) (localOperationSkillJSONResult, bool) {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "{") {
		return localOperationSkillJSONResult{}, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return localOperationSkillJSONResult{}, false
	}
	var out localOperationSkillJSONResult
	out.Success = optionalBoolJSON(raw["success"])
	out.Summary = stringJSON(raw["summary"])
	out.PayloadRef = stringJSON(raw["payload_ref"])
	out.ArtifactRefs = stringListJSON(raw["artifact_refs"])
	out.VerificationStatus = stringJSON(raw["verification_status"])
	out.VerificationSummary = stringJSON(raw["verification_summary"])
	out.Observations = observationsCountJSON(raw["observations"])
	out.Error = stringJSON(raw["error"])
	out.NextActions = workflowNextActionsJSON(raw)
	out.ReturnAction, _ = workflowReturnActionJSON(raw)
	out.WorkflowState = workflowStateJSON(raw["workflow_state"])
	return out, true
}

func workflowReturnActionJSON(raw map[string]json.RawMessage) (operation.WorkflowNextAction, bool) {
	if raw == nil {
		return operation.WorkflowNextAction{}, false
	}
	actionRaw := raw["return_action"]
	if len(actionRaw) == 0 {
		actionRaw = raw["return_to_action"]
	}
	if len(actionRaw) == 0 {
		actionRaw = raw["callback_action"]
	}
	if len(actionRaw) == 0 {
		actionRaw = raw["return"]
	}
	if len(actionRaw) == 0 {
		return operation.WorkflowNextAction{}, false
	}
	return workflowNextActionJSON(actionRaw)
}

func workflowNextActionsJSON(raw map[string]json.RawMessage) []operation.WorkflowNextAction {
	if raw == nil {
		return nil
	}
	actionRaw := raw["next_actions"]
	if len(actionRaw) == 0 {
		actionRaw = raw["next_action"]
	}
	if len(actionRaw) == 0 {
		return nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(actionRaw, &list); err != nil {
		var single json.RawMessage
		if err := json.Unmarshal(actionRaw, &single); err != nil || len(single) == 0 {
			return nil
		}
		list = []json.RawMessage{single}
	}
	out := make([]operation.WorkflowNextAction, 0, len(list))
	for _, item := range list {
		action, ok := workflowNextActionJSON(item)
		if !ok {
			continue
		}
		out = append(out, action)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func workflowNextActionJSON(raw json.RawMessage) (operation.WorkflowNextAction, bool) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &fields) != nil {
		return operation.WorkflowNextAction{}, false
	}
	action := operation.WorkflowNextAction{
		Provider:      stringJSONAlias(fields, "provider", "provider_name"),
		Tool:          stringJSONAlias(fields, "tool", "operation_tool"),
		Operation:     stringJSONAlias(fields, "operation"),
		OperationKind: stringJSONAlias(fields, "operation_kind", "kind"),
		TargetSurface: stringJSONAlias(fields, "target_surface", "surface"),
		RiskLevel:     stringJSONAlias(fields, "risk_level", "risk"),
		SideEffects:   stringListJSONAlias(fields, "side_effects", "effects"),
		Request:       stringJSONAlias(fields, "request", "prompt", "instruction"),
		Input:         mapJSONAlias(fields, "input", "args", "arguments"),
	}
	if b := optionalBoolJSONAlias(fields, "requires_confirmation", "requires_approval"); b != nil {
		action.RequiresConfirmation = *b
	}
	if strings.TrimSpace(action.Provider) == "" &&
		strings.TrimSpace(action.OperationKind) == "" &&
		strings.TrimSpace(action.Operation) == "" &&
		strings.TrimSpace(action.Request) == "" {
		return operation.WorkflowNextAction{}, false
	}
	return action, true
}

func workflowStateJSON(raw json.RawMessage) operation.WorkflowState {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &fields) != nil {
		return operation.WorkflowState{}
	}
	return operation.WorkflowState{
		WorkflowID: stringJSONAlias(fields, "workflow_id", "id"),
		Step:       stringJSONAlias(fields, "step", "phase"),
		ReturnTo:   stringJSONAlias(fields, "return_to", "return_provider"),
		Data:       mapJSONAlias(fields, "data", "state"),
	}
}

func stringJSONAlias(fields map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		if value := stringJSON(fields[name]); value != "" {
			return value
		}
	}
	return ""
}

func stringListJSONAlias(fields map[string]json.RawMessage, names ...string) []string {
	for _, name := range names {
		if values := stringListJSON(fields[name]); len(values) > 0 {
			return values
		}
	}
	return nil
}

func optionalBoolJSONAlias(fields map[string]json.RawMessage, names ...string) *bool {
	for _, name := range names {
		if b := optionalBoolJSON(fields[name]); b != nil {
			return b
		}
	}
	return nil
}

func mapJSONAlias(fields map[string]json.RawMessage, names ...string) map[string]any {
	for _, name := range names {
		raw := fields[name]
		if len(raw) == 0 {
			continue
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err == nil && len(out) > 0 {
			return out
		}
	}
	return nil
}

func optionalBoolJSON(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil
	}
	return &b
}

func stringJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringListJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return cleanOperationRefs(list)
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return cleanOperationRefs([]string{one})
	}
	return nil
}

func observationsCountJSON(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil && n > 0 {
		return n
	}
	var list []any
	if err := json.Unmarshal(raw, &list); err == nil {
		return len(list)
	}
	return 0
}

func cleanOperationRefs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func providerArtifactRefsFromMCPResponse(resp types.MCPResponse) []string {
	var refs []string
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			seen := false
			for _, existing := range refs {
				if existing == value {
					seen = true
					break
				}
			}
			if !seen {
				refs = append(refs, value)
			}
		}
	}
	add(resp.PayloadRef, resp.RawRef, resp.RowSetRef, resp.PageRef, resp.ResourceURI)
	for _, obs := range resp.Observations {
		add(obs.PayloadRef, obs.RawRef, obs.RowSetRef, obs.PageRef, obs.ResourceURI)
	}
	if len(refs) > 8 {
		return refs[:8]
	}
	return refs
}

func (r *REPL) ensureLazyOperationMCPServer(serverName string) error {
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return fmt.Errorf("operation provider server name is empty")
	}
	if r.mcpServers == nil {
		return fmt.Errorf("MCP registry is not available for operation provider execution")
	}
	if _, err := r.mcpServers.Get(serverName); err == nil {
		r.markOperationProviderLoaded("mcp:" + serverName)
		return nil
	}
	cfg, ok := r.lazyOperationMCPConfig(serverName)
	if !ok {
		return fmt.Errorf("MCP operation provider %q is not loaded and has no operation_lazy_start config", serverName)
	}
	server, err := mcp.StartServerFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("start lazy MCP operation provider %q: %w", serverName, err)
	}
	if err := r.mcpServers.Register(server); err != nil {
		_ = server.Close()
		return fmt.Errorf("register lazy MCP operation provider %q: %w", serverName, err)
	}
	r.markOperationProviderLoaded("mcp:" + serverName)
	logging.Info("[repl/operation] lazy MCP operation provider loaded server=%s", serverName)
	return nil
}

func (r *REPL) lazyOperationMCPConfig(serverName string) (types.MCPServerConfig, bool) {
	for _, cfg := range r.mcpServerConfigs {
		if strings.TrimSpace(cfg.Name) != serverName {
			continue
		}
		if cfg.OperationProvider == nil || !*cfg.OperationProvider {
			continue
		}
		if cfg.OperationLazyStart == nil || !*cfg.OperationLazyStart {
			continue
		}
		return cfg, true
	}
	return types.MCPServerConfig{}, false
}

func (r *REPL) markOperationProviderLoaded(providerName string) {
	providerName = strings.TrimSpace(providerName)
	for i := range r.operationProviders {
		if r.operationProviders[i].Name == providerName {
			r.operationProviders[i].Loaded = true
		}
	}
}

func (r *REPL) storeProviderOperationPayload(provider, toolName, summary string) (string, string) {
	dir := filepath.Join(firstNonEmptyString(strings.TrimSpace(r.runtimeAnchor), ".codrax"), "operation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logging.Warning("[repl/operation] store provider payload failed: %v", err)
		return "", summary
	}
	name := fmt.Sprintf("provider-%s-%s-%d.txt", sanitizeProviderPayloadToken(provider), sanitizeProviderPayloadToken(toolName), time.Now().UnixNano())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
		logging.Warning("[repl/operation] write provider payload failed: %v", err)
		return "", summary
	}
	preview := summary
	if utf8.RuneCountInString(preview) > 4096 {
		preview = string([]rune(preview)[:4096]) + fmt.Sprintf("\n[operation provider output truncated; full output saved to %s]", path)
	}
	return path, preview
}

type providerProcessOutputCapture struct {
	file      *os.File
	path      string
	buf       *boundedBuffer
	truncated bool
}

func (r *REPL) newProviderProcessOutputCapture(provider, stream string, capBytes int) (*providerProcessOutputCapture, error) {
	if capBytes <= 0 {
		capBytes = defaultOperationSkillMaxOutputSize
	}
	dir := filepath.Join(firstNonEmptyString(strings.TrimSpace(r.runtimeAnchor), ".codrax"), "operation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("provider-%s-%s-%d.txt", sanitizeProviderPayloadToken(provider), sanitizeProviderPayloadToken(stream), time.Now().UnixNano())
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &providerProcessOutputCapture{
		file: file,
		path: path,
		buf:  newBoundedBuffer(capBytes),
	}, nil
}

func (c *providerProcessOutputCapture) Write(p []byte) (int, error) {
	if c.file != nil {
		if _, err := c.file.Write(p); err != nil {
			return 0, err
		}
	}
	if c.buf != nil {
		_, _ = c.buf.Write(p)
		if c.buf.Truncated() {
			c.truncated = true
		}
	}
	return len(p), nil
}

func (c *providerProcessOutputCapture) String() string {
	if c == nil || c.buf == nil {
		return ""
	}
	text := c.buf.String()
	if c.truncated {
		text += fmt.Sprintf("\n[operation provider output truncated; full output saved to %s]", c.path)
	}
	return text
}

func (c *providerProcessOutputCapture) Truncated() bool {
	return c != nil && c.truncated
}

func (c *providerProcessOutputCapture) Path() string {
	if c == nil {
		return ""
	}
	return c.path
}

func (c *providerProcessOutputCapture) Close() {
	if c == nil || c.file == nil {
		return
	}
	_ = c.file.Close()
	if !c.truncated && c.path != "" {
		_ = os.Remove(c.path)
	}
}

func sanitizeProviderPayloadToken(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "provider"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// handleRejectCmd discards the pending ChangePlan without invoking
// the runner. Optional reason text trailing /reject is recorded in
// memory so the conversation's prior-turns block reflects why this
// handleBranchCmd dispatches `/branch [<args>]`:
//
//	/branch                   — print the current git branch (or
//	                            "detached@<sha7>" / "(not a git
//	                            repo)" depending on state).
//	/branch <name>            — `git checkout <name>` in repoRoot;
//	                            forwards extra args verbatim so
//	                            `/branch -b feature-x` (create +
//	                            switch) and `/branch -b feature-x
//	                            origin/main` (create from a
//	                            specific point) work.
//
// Output streams to the REPL's writer so the user sees git's own
// stdout/stderr (branch tracking notice, divergence warnings,
// etc.). After a successful checkout the next prompt's sticky
// tag automatically picks up the new branch via gitBranchProbe.
// handleCancelCmd is the `/cancel` slash command — terminal
// multiplexers (tmux/screen) sometimes swallow Ctrl+C; this gives
// operators a typed alternative. Behaviour mirrors the SIGINT path:
//
//   - Run in flight: drives runner.Cancel("/cancel"). The pipeline
//     unwinds at its next checkpoint.
//   - No Run: prints a one-line "nothing to cancel" notice. Does
//     NOT exit — Ctrl+D / `/exit` are the explicit exit verbs.
func (r *REPL) handleCancelCmd(line string) {
	_ = line // no args today; kept signature consistent with peers
	if r.pendingOperation != nil {
		plan := *r.pendingOperation
		plan.Status = operation.StatusCancelled
		r.operationHistory = append(r.operationHistory, plan)
		r.pendingOperation = nil
		r.clearPendingOperationState()
		r.info(operationCancelledMsg(r.language, plan.ID))
		return
	}
	if r.pendingCommandClarification != nil {
		plan := r.pendingCommandClarification.Plan
		plan.Status = operation.StatusCancelled
		r.operationHistory = append(r.operationHistory, plan)
		r.pendingCommandClarification = nil
		r.clearPendingOperationState()
		r.info(operationCancelledMsg(r.language, plan.ID))
		return
	}
	if r.pendingProviderOperation != nil {
		plan := r.pendingProviderOperation.Plan
		r.pendingProviderOperation = nil
		r.providerWorkflow = nil
		r.clearPendingOperationState()
		r.info(operationCancelledMsg(r.language, firstNonEmptyString(plan.Provider, plan.Kind)))
		return
	}
	canceller, ok := r.runner.(runnerCanceller)
	if !ok || !r.runInFlight.Load() {
		r.info(cancelNothingRunningMsg(r.language))
		return
	}
	canceller.Cancel("/cancel")
	r.warn("%s\n", cancelInProgressMsg(r.language))
}

func (r *REPL) handleBranchCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/branch"))
	if rest == "" {
		cur := gitBranchProbe(r.repoRoot)
		if cur == "" {
			r.info(fmt.Sprintf("(%s is not a git repo, or git missing on PATH)", r.repoRoot))
			return
		}
		r.info(fmt.Sprintf("current branch: %s", cur))
		return
	}
	args := append([]string{"-C", r.repoRoot, "checkout"}, strings.Fields(rest)...)
	cmd := exec.Command("git", args...)
	cmd.Stdout = r.out
	cmd.Stderr = r.out
	if err := cmd.Run(); err != nil {
		r.warn("%s", branchCheckoutFailedMsg(r.language, err))
		return
	}
	if cur := gitBranchProbe(r.repoRoot); cur != "" {
		r.success(fmt.Sprintf("now on branch: %s", cur))
	}
}

// handleShellBangCmd executes a system shell command on behalf of
// the user. Triggered by typing `!<cmd>` at the prompt — the
// leading `!` is stripped before this handler sees the line.
//
// stdout + stderr stream to r.out so the user sees the command's
// real output (not buffered, not summarised). Working directory
// is r.repoRoot so commands like `!ls` / `!cat <file>` /
// `!grep <pat>` operate on the repo by default. The exit status
// is reported back as a one-line warning when non-zero so the
// user knows the command failed.
//
// The command + captured output are also persisted as a memory
// turn (KindShell) so the next pipeline / chat turn's BuildContext
// can surface them as prior conversation context. The operator
// can then ask follow-up questions ("explain this error", "fix
// the diff above") against the actual command output rather than
// having to paste it back in. Captured output is capped at
// shellBangCaptureCap to bound memory growth on commands that
// dump multi-MB output.
//
// `cd` is special-cased: each `!` invocation spawns a fresh
// shell, so `!cd <dir>` has no effect on subsequent commands.
// We detect the shape and surface a clear message rather than
// silently no-oping (the surprising failure mode is "I typed
// `!cd ..` and the next `!ls` showed the same files").
func (r *REPL) handleShellBangCmd(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		r.info(shellBangEmpty(r.language))
		return
	}
	first := strings.Fields(line)[0]
	if first == "cd" || first == "pushd" || first == "popd" {
		r.warn("%s", shellBangCdNonPersistent(r.language, first))
		// Fall through and run anyway — `!cd /x && cat foo` IS
		// a useful shape (the && side-effects are within one
		// shell). Only the bare `!cd /x` is the no-op shape.
		// Surface the warning either way so the user learns.
	}
	// io.MultiWriter splits the subprocess's output: one branch
	// streams to the user's terminal as before; the other branch
	// fills a bounded buffer the memory turn captures. The bounded
	// writer drops bytes past the cap so a runaway `!find /` or
	// `!cat huge_file` cannot blow up REPL memory.
	captureBuf := newBoundedBuffer(shellBangCaptureCap)
	writer := io.MultiWriter(r.out, captureBuf)
	cmd := tool.NewShellCommandContext(context.Background(), line)
	cmd.Dir = r.repoRoot
	cmd.Stdout = writer
	cmd.Stderr = writer
	runErr := cmd.Run()
	if runErr != nil {
		r.warn("%s", shellBangExit(r.language, runErr))
	}
	captured := captureBuf.String()
	if captureBuf.Truncated() {
		captured += fmt.Sprintf("\n[output truncated; first %d bytes captured for context — full output rendered above]", shellBangCaptureCap)
	}
	if runErr != nil {
		captured = strings.TrimRight(captured, "\n") + fmt.Sprintf("\n[exit error: %v]", runErr)
	}
	// Record so the next /chat / pipeline turn picks it up via
	// memory.BuildContext. Request and display both prefixed with
	// `!` so the index entry is unambiguously a shell turn at a
	// glance; Response is the captured stream.
	r.recordTurn("!"+line, "!"+line, captured, memory.KindShell)
}

// shellBangCaptureCap is the upper bound on bytes captured from a
// `!<cmd>` invocation for memory persistence. The full stream still
// reaches the terminal — this only affects what the next turn's
// prior-conversation context sees. 32 KiB covers typical ls / git
// status / short tracebacks; large paste flows would dilute recent
// context anyway, and compaction summarises the rest.
const shellBangCaptureCap = 32 * 1024

// boundedBuffer is an io.Writer that accepts up to cap bytes and
// silently drops everything after. Truncated() reports whether the
// drop happened so the caller can append a clear marker.
type boundedBuffer struct {
	buf       []byte
	cap       int
	truncated bool
}

func newBoundedBuffer(cap int) *boundedBuffer {
	return &boundedBuffer{cap: cap}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.cap - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) <= remaining {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}
	b.buf = append(b.buf, p[:remaining]...)
	b.truncated = true
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	return string(b.buf)
}

func (b *boundedBuffer) Truncated() bool {
	return b.truncated
}

// plan was dropped.
//
// Behaviour mirrors /plan clear with two differences:
//  1. Accepts a free-form reason argument.
//  2. Records a memory turn so /history shows the rejection.
func (r *REPL) handleRejectCmd(line string) {
	if r.pendingOperation != nil {
		r.handleOperationRejectCmd(line)
		return
	}
	if r.pendingProviderOperation != nil {
		plan := r.pendingProviderOperation.Plan
		r.pendingProviderOperation = nil
		r.providerWorkflow = nil
		r.clearPendingOperationState()
		reason := strings.TrimSpace(strings.TrimPrefix(line, "/reject"))
		r.info(operationRejectedMsg(r.language, firstNonEmptyString(plan.Provider, plan.Kind), reason))
		return
	}
	if r.planStore == nil {
		r.info(commandDisabled(r.language, "/reject", noPlanStoreReason(r.language)))
		return
	}
	if r.pendingPlanPath == "" {
		r.bindActiveWriteWorkflowPlanIfIdle("/reject")
	}
	// Same cold-start recovery /approve uses. Single-pending
	// invariant means at most one match.
	if r.pendingPlanPath == "" {
		if recovered, ok := r.recoverPendingPlanFromStore(); ok {
			r.pendingPlanPath = recovered.Path
			r.info(recoveredPendingPlan(r.language, recovered.ID))
		}
	}
	if r.pendingPlanPath == "" {
		r.info(noPendingPlanReject(r.language))
		return
	}
	reason := strings.TrimSpace(strings.TrimPrefix(line, "/reject"))
	// Group-level reject (commit 31): when the first token of
	// the reason is a group id, settle every phase plan in the
	// group + discard the cumulative worktree in one call.
	// Remaining text (after the group id) becomes the per-plan
	// rejection reason recorded on each settled plan.
	if firstTok := strings.Fields(reason); len(firstTok) > 0 && strings.HasPrefix(firstTok[0], "group-") {
		groupID := firstTok[0]
		groupReason := strings.TrimSpace(strings.TrimPrefix(reason, firstTok[0]))
		r.handleRejectGroupCmd(groupID, groupReason)
		return
	}
	id := strings.TrimSuffix(filepath.Base(r.pendingPlanPath), ".json")
	// Settle (keep the file with Status=rejected + RejectionReason
	// for audit trail) instead of Clear (delete file). The user
	// can still recover the rejected plan via /plan show <id> or
	// /plan list. /plan clear is the "no-audit" alternative for
	// experimental / mistaken plans.
	if err := r.planStore.Settle(id, types.PlanStatusRejected, reason); err != nil {
		r.errorf("reject: %v\n", err)
		return
	}
	// Discard the worktree (if any was preserved by a successful
	// apply). Workspace and plan are tied: settling the plan also
	// reclaims the disk space. Best-effort: a discard failure logs
	// but doesn't block the reject decision.
	r.discardWorktreeForRejected(id)
	r.markWriteWorkflowBatchRejected(id, reason)
	r.pendingPlanPath = ""

	var msg string
	if reason == "" {
		msg = rejectConfirmedNoReason(r.language, id)
	} else {
		msg = rejectConfirmedWithReason(r.language, id, reason)
	}
	r.success(msg)

	request := "/reject"
	if reason != "" {
		request = "/reject " + reason
	}
	// Reject is still a plan-lifecycle event; use KindPlan so it
	// shows up in the same /history filter as approvals.
	r.recordTurn(request, request, msg, memory.KindPlan)
}

// handleLogCmd dispatches the `/log` subcommands. Recognised forms:
//
//	/log <path>   — load a log file from disk (replaces any current)
//	/log clear    — drop the current attached log
//	/log show     — print the first 20 lines + total byte count
//	/log          — enter paste mode; subsequent lines form the log
//	                until a lone `/end` line terminates capture
//
// Sticky semantics: an attachment survives across turns and across
// /clear (which only wipes conversation memory). Only /log clear or
// a new /log path/paste replaces it.
func (r *REPL) handleLogCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/log"))
	switch {
	case rest == "":
		r.handleLogPaste()
	case rest == "clear":
		if r.attachedLog == "" {
			r.info(noLogAttached(r.language))
			return
		}
		r.attachedLog = ""
		r.attachedLogAutoRouted = false
		if setter, ok := r.runner.(attachedLogSetter); ok {
			setter.SetAttachedLog("")
		}
		r.success(attachedLogClearedMsg(r.language))
	case rest == "show":
		r.handleLogShow()
	case strings.HasPrefix(rest, "append "):
		r.handleLogAppend(strings.TrimSpace(strings.TrimPrefix(rest, "append")))
	default:
		r.handleLogLoad(rest)
	}
}

// handleLogAppend reads `path` and APPENDS to the sticky log buffer
// with a `# codrax-source: <path>\n` header — symmetric with the CLI
// `--log a.log --log b.log` repeatable behaviour. Replaces the prior
// attachment when no log is currently attached (`/log append` when
// empty acts as `/log <path>`). Total bytes still capped by
// attachedLogMaxBytes; excess tail-truncates with a WARN.
func (r *REPL) handleLogAppend(path string) {
	if path == "" {
		r.errorf("/log append <path> — missing path argument\n")
		return
	}
	header := "# codrax-source: " + path + "\n"
	combined := r.attachedLog
	if combined != "" {
		combined += "\n"
	}
	combined += header
	remaining := r.attachedLogMaxBytes - len(combined)
	if remaining < 0 {
		combined = combined[:r.attachedLogMaxBytes]
		r.warn("appended log truncated at %d-byte cap\n", r.attachedLogMaxBytes)
		r.attachedLog = combined
		r.attachedLogAutoRouted = false
		r.success(fmt.Sprintf("appended %s (0 bytes added; total %d bytes)", path, len(r.attachedLog)))
		return
	}
	data, truncated, err := readFileLimited(path, remaining)
	if err != nil {
		r.errorf("append log: %v\n", err)
		return
	}
	if r.reportAttachmentTextIssue(attachment.ValidateText(attachment.KindLog, path, data, truncated)) {
		return
	}
	combined += string(data)
	if truncated {
		r.warn("appended log truncated at %d-byte cap\n", r.attachedLogMaxBytes)
	}
	r.attachedLog = combined
	r.attachedLogAutoRouted = false
	r.success(fmt.Sprintf("appended %s (%d bytes added; total %d bytes)", path, len(data), len(r.attachedLog)))
}

// handleHitraceCmd is the perf-channel companion to handleLogCmd.
// Mirrors the surface for HiTrace / Android-systrace attachments:
//
//	/htrace <path>                  — load file from disk (replaces any prior)
//	/htrace append <path>           — append additional file with source header
//	/htrace convert [opts] <binary> [out]  — convert binary HiTrace to text systrace
//	/htrace tools-status            — print trace_streamer/engine/gate status
//	/htrace clear                   — clear the current attachment
//	/htrace show                    — print byte count + head of the attachment
//	/htrace                         — bare invocation prints usage; no paste
//	                          mode (traces are usually multi-MB hdc
//	                          / adb dumps, not hand-typed)
func (r *REPL) handleHitraceCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/htrace"))
	switch {
	case rest == "":
		r.info(htraceUsage(r.language))
	case rest == "clear":
		if r.attachedHitrace == "" {
			r.info(noTraceAttached(r.language))
			return
		}
		r.attachedHitrace = ""
		r.attachedHitraceSource = ""
		if setter, ok := r.runner.(attachedHitraceSetter); ok {
			setter.SetAttachedHitrace("")
		}
		if setter, ok := r.runner.(attachedHitraceSourceSetter); ok {
			setter.SetAttachedHitraceSource("")
		}
		r.success(attachedTraceClearedMsg(r.language))
	case rest == "show":
		if r.attachedHitrace == "" {
			r.info(noTraceAttached(r.language))
			return
		}
		head := r.attachedHitrace
		if len(head) > 800 {
			head = head[:800] + "…"
		}
		r.info(fmt.Sprintf("hitrace: %d bytes\n%s", len(r.attachedHitrace), head))
	case strings.HasPrefix(rest, "append "):
		r.handleHitraceAppend(strings.TrimSpace(strings.TrimPrefix(rest, "append")))
	case rest == "convert" || strings.HasPrefix(rest, "convert "):
		r.handleHitraceConvert(strings.TrimSpace(strings.TrimPrefix(rest, "convert")))
	case rest == "tools-status" || strings.HasPrefix(rest, "tools-status "):
		r.handleHitraceToolsStatus(strings.TrimSpace(strings.TrimPrefix(rest, "tools-status")))
	default:
		// Single-path load also gets the source header so the LLM
		// sees a consistent boundary marker shape regardless of
		// how the trace got attached (single load / append / CLI).
		header := "# codrax-source: " + rest + "\n"
		remaining := r.attachedTraceMaxBytes - len(header)
		if remaining < 0 {
			r.warn("hitrace truncated at %d-byte cap\n", r.attachedTraceMaxBytes)
			r.attachedHitrace = header[:r.attachedTraceMaxBytes]
			r.attachedHitraceSource = mergeTraceSourceHints("", r.currentTraceSourceHint(rest))
			r.success(fmt.Sprintf("attached hitrace loaded: %s (%d bytes)", rest, 0))
			return
		}
		data, truncated, err := readFileLimited(rest, remaining)
		if err != nil {
			r.errorf("load hitrace: %v\n", err)
			return
		}
		if r.reportAttachmentTextIssue(attachment.ValidateText(attachment.KindTrace, rest, data, truncated)) {
			return
		}
		if truncated {
			r.warn("hitrace truncated at %d-byte cap\n", r.attachedTraceMaxBytes)
		}
		body := header + string(data)
		r.attachedHitrace = body
		r.attachedHitraceSource = mergeTraceSourceHints("", r.currentTraceSourceHint(rest))
		r.success(fmt.Sprintf("attached hitrace loaded: %s (%d bytes)", rest, len(data)))
	}
}

func (r *REPL) handleHitraceToolsStatus(args string) {
	if strings.TrimSpace(args) != "" {
		r.errorf("%s\n", htraceToolsStatusUsage(r.language))
		return
	}
	status, err := hitraceconv.BuildTraceToolStatus(hitraceconv.Options{TraceEngine: "auto"})
	if err != nil {
		r.errorf("%s\n", htraceToolsStatusFailedMsg(r.language, err))
		return
	}
	for _, line := range htraceToolsStatusMsgs(r.language, status) {
		r.info(line)
	}
}

func (r *REPL) handleHitraceConvert(args string) {
	fields := strings.Fields(args)
	if len(fields) == 0 || isHelpArg(fields[0]) {
		r.info(htraceConvertUsage(r.language))
		return
	}
	parsed, err := parseHitraceConvertArgs(fields)
	if err != nil {
		r.errorf("%s\n", htraceConvertUsage(r.language))
		return
	}
	opts := hitraceconv.Options{
		InputPath:         parsed.input,
		OutputPath:        parsed.output,
		Flavor:            "harmony_hitrace",
		TraceEngine:       parsed.traceEngine,
		TraceDBOutputPath: parsed.traceDBOutputPath,
		KeepTraceDB:       parsed.keepTraceDB,
	}
	opts.Progress = func(event hitraceconv.ProgressEvent) {
		r.info(htraceConvertProgressMsg(r.language, event))
	}
	result, err := r.hitraceConvert(context.Background(), opts)
	if err != nil {
		r.errorf("%s\n", htraceConvertFailedMsg(r.language, err))
		return
	}
	r.success(htraceConvertSuccess(r.language, result.OutputPath, result.EventsWritten))
	for _, artifact := range result.Artifacts {
		if artifact.Type == hitraceconv.ArtifactSystrace {
			continue
		}
		r.info(htraceConvertArtifactMsg(r.language, artifact.Type, artifact.Path, hitraceConvertArtifactDetail(r.language, artifact)))
	}
	for _, line := range htraceConvertCoverageMsgs(r.language, "trace_coverage", result.TraceCoverage) {
		r.info(line)
	}
	for _, line := range htraceConvertCoverageMsgs(r.language, "trace_db_coverage", result.TraceDBCoverage) {
		r.info(line)
	}
	if len(result.Caveats) > 0 {
		for _, caveat := range result.Caveats {
			r.warn("%s", htraceConvertCaveatMsg(r.language, caveat))
		}
	}
	r.info(htraceConvertNextMsg(r.language, result.OutputPath, result.BundlePath, hitraceResultHasArtifact(result, hitraceconv.ArtifactPerfTrace)))
}

type hitraceConvertParsedArgs struct {
	input             string
	output            string
	traceEngine       string
	traceDBOutputPath string
	keepTraceDB       bool
}

func parseHitraceConvertArgs(fields []string) (hitraceConvertParsedArgs, error) {
	var parsed hitraceConvertParsedArgs
	var positional []string
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		switch {
		case field == "--trace-engine" || field == "--trace_engine":
			if i+1 >= len(fields) || strings.HasPrefix(fields[i+1], "-") {
				return hitraceConvertParsedArgs{}, fmt.Errorf("missing trace engine")
			}
			i++
			if !validHitraceConvertTraceEngine(fields[i]) {
				return hitraceConvertParsedArgs{}, fmt.Errorf("unsupported trace engine")
			}
			parsed.traceEngine = fields[i]
		case strings.HasPrefix(field, "--trace-engine=") || strings.HasPrefix(field, "--trace_engine="):
			_, value, _ := strings.Cut(field, "=")
			if !validHitraceConvertTraceEngine(value) {
				return hitraceConvertParsedArgs{}, fmt.Errorf("unsupported trace engine")
			}
			parsed.traceEngine = value
		case field == "--trace-db-output" || field == "--trace_db_output":
			if i+1 >= len(fields) || strings.HasPrefix(fields[i+1], "-") {
				return hitraceConvertParsedArgs{}, fmt.Errorf("missing trace db output")
			}
			i++
			parsed.traceDBOutputPath = fields[i]
		case strings.HasPrefix(field, "--trace-db-output=") || strings.HasPrefix(field, "--trace_db_output="):
			_, value, _ := strings.Cut(field, "=")
			if strings.TrimSpace(value) == "" {
				return hitraceConvertParsedArgs{}, fmt.Errorf("missing trace db output")
			}
			parsed.traceDBOutputPath = value
		case field == "--keep-trace-db" || field == "--keep_trace_db":
			parsed.keepTraceDB = true
		case strings.HasPrefix(field, "--keep-trace-db=") || strings.HasPrefix(field, "--keep_trace_db="):
			_, value, _ := strings.Cut(field, "=")
			keep, ok := parseHitraceConvertBool(value)
			if !ok {
				return hitraceConvertParsedArgs{}, fmt.Errorf("invalid keep trace db")
			}
			parsed.keepTraceDB = keep
		case strings.HasPrefix(field, "-"):
			return hitraceConvertParsedArgs{}, fmt.Errorf("unsupported option")
		default:
			positional = append(positional, field)
		}
	}
	if len(positional) == 0 || len(positional) > 2 {
		return hitraceConvertParsedArgs{}, fmt.Errorf("invalid positional arguments")
	}
	parsed.input = positional[0]
	if len(positional) == 2 {
		parsed.output = positional[1]
	}
	return parsed, nil
}

func parseHitraceConvertBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, true
	case "0", "f", "false", "n", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func validHitraceConvertTraceEngine(value string) bool {
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_") {
	case "auto", "trace_streamer", "builtin":
		return true
	default:
		return false
	}
}

func hitraceConvertArtifactDetail(lang string, artifact hitraceconv.Artifact) string {
	var details []string
	if format := hitraceConvertArtifactFormatDetail(lang, artifact.Type); format != "" {
		details = append(details, format)
	}
	if artifact.Bytes > 0 {
		details = append(details, hitraceConvertDetailKV(lang, "bytes", fmt.Sprintf("%d", artifact.Bytes)))
	}
	if artifact.Converter != "" {
		details = append(details, hitraceConvertDetailKV(lang, "converter", artifact.Converter))
	}
	if artifact.DataType != 0 {
		details = append(details, hitraceConvertDetailKV(lang, "data_type", fmt.Sprintf("%d", artifact.DataType)))
	}
	if artifact.PluginName != "" {
		details = append(details, hitraceConvertDetailKV(lang, "plugin", artifact.PluginName))
	}
	if artifact.PluginVersion != "" {
		details = append(details, hitraceConvertDetailKV(lang, "plugin_version", artifact.PluginVersion))
	}
	if artifact.SourceOffset > 0 {
		details = append(details, hitraceConvertDetailKV(lang, "source_offset", fmt.Sprintf("%d", artifact.SourceOffset)))
	}
	if artifact.SourceBytes > 0 {
		details = append(details, hitraceConvertDetailKV(lang, "source_bytes", fmt.Sprintf("%d", artifact.SourceBytes)))
	}
	if len(artifact.Caveats) > 0 {
		localized := make([]string, 0, len(artifact.Caveats))
		for _, caveat := range artifact.Caveats {
			localized = append(localized, hitraceconv.LocalizeConvertMessage(lang, caveat))
		}
		details = append(details, hitraceConvertDetailKV(lang, "caveats", strings.Join(localized, "; ")))
	}
	return strings.Join(details, " ")
}

func hitraceConvertArtifactFormatDetail(lang, typ string) string {
	switch typ {
	case hitraceconv.ArtifactPerfData:
		if isZh(lang) {
			return "格式=二进制 perf.data sidecar"
		}
		return "format=binary_perf_data_sidecar"
	case hitraceconv.ArtifactPerfTrace:
		if isZh(lang) {
			return "格式=文本 perftrace"
		}
		return "format=text_perftrace"
	case hitraceconv.ArtifactTraceDB:
		if isZh(lang) {
			return "格式=trace_streamer SQLite DB"
		}
		return "format=trace_streamer_sqlite_db"
	case hitraceconv.ArtifactTraceBundle:
		if isZh(lang) {
			return "格式=tracebundle 元数据"
		}
		return "format=tracebundle_metadata"
	default:
		return ""
	}
}

func hitraceConvertDetailKV(lang, key, value string) string {
	if isZh(lang) {
		key = hitraceConvertDetailKeyZh(key)
	}
	return key + "=" + value
}

func hitraceConvertDetailKeyZh(key string) string {
	switch key {
	case "bytes":
		return "字节"
	case "converter":
		return "转换器"
	case "data_type":
		return "数据类型"
	case "plugin":
		return "插件"
	case "plugin_version":
		return "插件版本"
	case "source_offset":
		return "源偏移"
	case "source_bytes":
		return "源字节"
	case "caveats":
		return "提示"
	default:
		return key
	}
}

func hitraceResultHasArtifact(result hitraceconv.Result, typ string) bool {
	for _, artifact := range result.Artifacts {
		if artifact.Type == typ {
			return true
		}
	}
	return false
}

func isHelpArg(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "help", "-h", "--help":
		return true
	default:
		return false
	}
}

// handleHitraceAppend mirrors handleLogAppend: read a trace file
// and concatenate it onto the sticky buffer with a source header.
// Bounded by attachedTraceMaxBytes (the perf-channel cap).
func (r *REPL) handleHitraceAppend(path string) {
	if path == "" {
		r.errorf("/htrace append <path> — missing path argument\n")
		return
	}
	header := "# codrax-source: " + path + "\n"
	combined := r.attachedHitrace
	if combined != "" {
		combined += "\n"
	}
	combined += header
	remaining := r.attachedTraceMaxBytes - len(combined)
	if remaining < 0 {
		combined = combined[:r.attachedTraceMaxBytes]
		r.warn("appended hitrace truncated at %d-byte cap\n", r.attachedTraceMaxBytes)
		r.attachedHitrace = combined
		r.attachedHitraceSource = mergeTraceSourceHints(r.attachedHitraceSource, r.currentTraceSourceHint(path))
		r.success(fmt.Sprintf("appended %s (0 bytes added; total %d bytes)", path, len(r.attachedHitrace)))
		return
	}
	data, truncated, err := readFileLimited(path, remaining)
	if err != nil {
		r.errorf("append hitrace: %v\n", err)
		return
	}
	if r.reportAttachmentTextIssue(attachment.ValidateText(attachment.KindTrace, path, data, truncated)) {
		return
	}
	combined += string(data)
	if truncated {
		r.warn("appended hitrace truncated at %d-byte cap\n", r.attachedTraceMaxBytes)
	}
	r.attachedHitrace = combined
	r.attachedHitraceSource = mergeTraceSourceHints(r.attachedHitraceSource, r.currentTraceSourceHint(path))
	r.success(fmt.Sprintf("appended %s (%d bytes added; total %d bytes)", path, len(data), len(r.attachedHitrace)))
}

func (r *REPL) currentTraceSourceHint(path string) string {
	if strings.TrimSpace(r.pendingHitraceSource) != "" {
		return r.pendingHitraceSource
	}
	return traceSourceHintFromPath(path)
}

func replTraceSourceHint(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return ""
	}
	cmd := strings.ToLower(fields[0])
	switch cmd {
	case "/atrace", "\\atrace":
		return "android_atrace"
	case "/htrace", "\\htrace":
		return "harmony_hitrace"
	default:
		return ""
	}
}

func traceSourceHintFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	switch ext {
	case ".htrace":
		return "harmony_hitrace"
	case ".atrace":
		return "android_atrace"
	default:
		return ""
	}
}

func mergeTraceSourceHints(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" || existing == next {
		return existing
	}
	return "generic_ftrace"
}

// handleLogLoad reads a log file from disk into the sticky buffer.
// Replaces any existing attachment (a `/log append` variant is a
// future add; users can cat files together themselves for now).
func (r *REPL) handleLogLoad(path string) {
	data, truncated, err := readFileLimited(path, r.attachedLogMaxBytes)
	if err != nil {
		r.errorf("load log: %v\n", err)
		return
	}
	if r.reportAttachmentTextIssue(attachment.ValidateText(attachment.KindLog, path, data, truncated)) {
		return
	}
	if truncated {
		r.warn("log truncated at %d-byte cap\n", r.attachedLogMaxBytes)
	}
	r.attachedLog = string(data)
	r.attachedLogAutoRouted = false
	r.success(fmt.Sprintf("attached log loaded: %s (%d bytes)", path, len(data)))
}

func (r *REPL) reportAttachmentTextIssue(err error) bool {
	if err == nil {
		return false
	}
	var issue attachment.TextIssue
	if errors.As(err, &issue) {
		r.errorf("%s\n", issue.Message(r.language, attachment.SurfaceREPL))
		return true
	}
	r.errorf("%v\n", err)
	return true
}

func readFileLimited(path string, limit int) ([]byte, bool, error) {
	if limit < 0 {
		limit = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

// handleLogPaste enters a multi-line capture mode. Every subsequent
// line is appended to the buffer until a sole `/end` line (or EOF)
// terminates capture. Empty capture leaves the prior attachment
// unchanged.
//
// Interactive capture deliberately bypasses the Bubble Tea input
// session: placeholder folding would replace a large paste with a
// token instead of storing the raw log content that log-triage
// actually needs. We read straight from the caller's input stream
// (os.Stdin in production, r.in in tests) one line at a time.
func (r *REPL) handleLogPaste() {
	r.info(pasteCapturePromptLog(r.language))
	scanner := r.captureScanner()
	var buf strings.Builder
	tag := r.currentStickyTag()
	for {
		if r.interactive() {
			if tag != "" {
				fmt.Fprintf(r.out, "  %s log> ", tag)
			} else {
				fmt.Fprint(r.out, "  log> ")
			}
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if trim == "/end" || trim == "\\end" {
			break
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if buf.Len() > r.attachedLogMaxBytes {
			r.warn("log paste hit %d-byte cap; stopping capture\n", r.attachedLogMaxBytes)
			break
		}
	}
	if err := scanner.Err(); err != nil {
		logging.Warning("[repl] log paste scan error: %v", err)
	}
	if buf.Len() == 0 {
		r.info(pasteNoCaptureLog(r.language))
		return
	}
	r.attachedLog = buf.String()
	r.attachedLogAutoRouted = false
	r.success(fmt.Sprintf("attached log captured: %d bytes", buf.Len()))
}

// handlePasteCmd is the terminal-independent fallback for the main
// input. bracketed paste is stripped in common SSH/tmux setups; when
// that happens an auto-fold never fires because the paste arrives as
// individual KeyRunes instead of one tea.KeyMsg{Paste:true}. This
// command reads lines straight from the caller's input stream until
// a lone `/end`, stores the collected bytes in r.pendingPaste, and
// lets the next readInputBubble inject it as a pre-seeded placeholder
// token. User can then edit around the token and submit normally.
func (r *REPL) handlePasteCmd() {
	r.info(pasteCapturePromptGeneric(r.language))
	scanner := r.captureScanner()
	var buf strings.Builder
	tag := r.currentStickyTag()
	for {
		if r.interactive() {
			if tag != "" {
				fmt.Fprintf(r.out, "  %s paste> ", tag)
			} else {
				fmt.Fprint(r.out, "  paste> ")
			}
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if trim == "/end" || trim == "\\end" {
			break
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if buf.Len() > r.attachedLogMaxBytes {
			r.warn("paste hit %d-byte cap; stopping capture\n", r.attachedLogMaxBytes)
			break
		}
	}
	if err := scanner.Err(); err != nil {
		logging.Warning("[repl] /paste scan error: %v", err)
	}
	captured := strings.TrimRight(buf.String(), "\n")
	if captured == "" {
		r.info(pasteNoCaptureGeneric(r.language))
		return
	}
	r.pendingPaste = captured
	chars := 0
	for range captured {
		chars++
	}
	lines := strings.Count(captured, "\n") + 1
	r.success(fmt.Sprintf("captured %d lines, %d chars → placed as [Pasted text #0] in next prompt", lines, chars))
}

// handleLogShow prints a preview of the currently-attached log.
func (r *REPL) handleLogShow() {
	if r.attachedLog == "" {
		r.info(noLogAttached(r.language))
		return
	}
	lines := strings.Split(r.attachedLog, "\n")
	head := lines
	if len(head) > 20 {
		head = lines[:20]
	}
	fmt.Fprintf(r.out, "  attached log: %d bytes, %d lines\n", len(r.attachedLog), len(lines))
	for _, ln := range head {
		fmt.Fprintf(r.out, "    | %s\n", ln)
	}
	if len(lines) > 20 {
		fmt.Fprintf(r.out, "    | ... [%d more lines]\n", len(lines)-20)
	}
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;:]*m`)
var reTrailing = regexp.MustCompile(`(\s|\x1b\[[0-9;:]*m)+$`)

// stripTrailing removes all trailing whitespace and ANSI sequences.
func stripTrailing(s string) string {
	return reTrailing.ReplaceAllString(s, "")
}

// stripANSI removes all ANSI escape sequences to get plain text.
func stripANSI(s string) string {
	return strings.TrimSpace(render.StripANSIOnly(s))
}

func stripANSIOnly(s string) string {
	return render.StripANSIOnly(s)
}

type borderedResponseLine struct {
	text   string
	visual bool
}

func borderedResponseLines(response string) []borderedResponseLine {
	raw := strings.Split(response, "\n")
	tableLines := markdownTableLineMask(raw)
	lines := make([]borderedResponseLine, 0, len(raw))
	inFence := false
	inRenderedVisualBlock := false
	prevBlank := false
	for i, ln := range raw {
		plain := strings.TrimSpace(stripANSIOnly(ln))
		fenceLine := isMarkdownFenceLine(plain)
		lineLooksVisual := shouldPreserveVisualLine(ln)
		if inRenderedVisualBlock && !fenceLine && !lineLooksVisual && stripANSI(ln) != "" && !isRenderedCodeBlockContinuation(ln) {
			inRenderedVisualBlock = false
		}
		visual := inFence || inRenderedVisualBlock || tableLines[i] || lineLooksVisual
		if fenceLine {
			visual = true
		}

		clean := stripTrailing(ln)
		if visual {
			clean = strings.TrimRight(ln, "\r")
		}
		blank := stripANSI(clean) == ""
		if blank && prevBlank && !visual {
			if !inFence && !inRenderedVisualBlock {
				continue
			}
		}
		prevBlank = blank && !visual
		lines = append(lines, borderedResponseLine{text: clean, visual: visual})

		if fenceLine {
			inFence = !inFence
			inRenderedVisualBlock = false
			continue
		}
		if inFence {
			continue
		}
		if isRenderedCodeBlockHeaderLine(plain) || lineLooksVisual {
			inRenderedVisualBlock = true
			continue
		}
	}
	for len(lines) > 0 && stripANSI(lines[0].text) == "" && !lines[0].visual {
		lines = lines[1:]
	}
	for len(lines) > 0 && stripANSI(lines[len(lines)-1].text) == "" && !lines[len(lines)-1].visual {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func markdownTableLineMask(raw []string) []bool {
	return render.MarkdownTableLineMask(raw)
}

func isMarkdownFenceLine(plain string) bool {
	return render.IsMarkdownFenceLine(plain)
}

func isRenderedCodeBlockHeaderLine(plain string) bool {
	return render.IsRenderedCodeBlockHeaderLine(plain)
}

func isRenderedCodeBlockContinuation(s string) bool {
	return render.IsRenderedCodeBlockContinuation(s)
}

// borderedLineFragments returns the visual rows that renderBordered
// should print for one logical response line. Prose may wrap; tabular
// / diagram-like rows are treated as atomic because terminal wrapping
// corrupts their columns, box edges, and arrows. Wide atomic rows pass
// through unchanged: final-answer visual content is user content, and
// the renderer must not rewrite or truncate it just to fit the current
// terminal width.
func borderedLineFragments(s string, maxCols int) []string {
	return borderedLineFragmentsPreserve(s, maxCols, false)
}

func borderedLineFragmentsPreserve(s string, maxCols int, preserve bool) []string {
	if displayWidth(s) <= maxCols {
		return []string{s}
	}
	if preserve || shouldPreserveVisualLine(s) {
		return []string{s}
	}
	return wrapByWidth(s, maxCols)
}

func displayWidth(s string) int {
	return render.DisplayWidth(s)
}

func shouldPreserveVisualLine(s string) bool {
	return render.ShouldPreserveVisualLine(s)
}

// wrapByWidth breaks a line into multiple lines that each fit within
// maxCols display columns. ANSI escape sequences are preserved: they
// are skipped for width calculation, and active SGR styles are replayed
// at the start of each continuation line so colours/bold survive wraps.
func wrapByWidth(s string, maxCols int) []string {
	if s == "" {
		return []string{""}
	}
	var result []string
	var activeSeqs []string // SGR sequences active at current position

	for len(s) > 0 {
		var buf strings.Builder
		// Replay active styles on continuation lines.
		if len(result) > 0 {
			for _, seq := range activeSeqs {
				buf.WriteString(seq)
			}
		}

		w := 0
		i := 0
		hasVisible := false

		for i < len(s) {
			// Detect ANSI CSI sequence: ESC [ <params> <final byte>
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == ':') {
					j++
				}
				if j < len(s) {
					j++ // final byte (m, H, K, …)
				}
				seq := s[i:j]
				buf.WriteString(seq)
				// Track SGR (Select Graphic Rendition) sequences for
				// style continuation across wrapped lines.
				if len(seq) > 0 && seq[len(seq)-1] == 'm' {
					if seq == "\x1b[0m" || seq == "\x1b[m" {
						activeSeqs = nil
					} else {
						activeSeqs = append(activeSeqs, seq)
					}
				}
				i = j
				continue
			}

			ch, size := utf8.DecodeRuneInString(s[i:])
			rw := runewidth.RuneWidth(ch)
			if w+rw > maxCols && hasVisible {
				break
			}
			buf.WriteString(s[i : i+size])
			w += rw
			i += size
			hasVisible = true
		}

		s = s[i:]
		// Reset styles at end of wrapped line so they don't bleed
		// into the "│" prefix on the next line.
		if len(s) > 0 && len(activeSeqs) > 0 {
			buf.WriteString("\x1b[0m")
		}
		result = append(result, buf.String())
	}
	return result
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// ensure huh.ErrUserAborted is treated as EOF for graceful shutdown.
func init() {
	_ = errors.Is(huh.ErrUserAborted, io.EOF) // compile-time check that the type exists
}
