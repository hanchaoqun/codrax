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

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// Runner is the orchestrator-shaped surface the REPL needs. Defined
// here as an interface so tests can stub it without pulling in the
// full pipeline.
type Runner interface {
	Run(request, repoRoot, branch string) (*types.BusContext, error)
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

// attachedHitraceGetter mirrors attachedLogGetter for the perf channel.
type attachedHitraceGetter interface {
	AttachedHitrace() string
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
	Prompt     string // primary prompt, e.g. ">"
	PromptCont string // continuation prompt, e.g. "."
	Banner     string // printed once at start; empty → default badge

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

	// PlanStore persists B0 write-mode ChangePlans for the REPL
	// session. Nil disables the /plan slash command family —
	// useful for tests and for single-shot invocations that never
	// construct a REPL. cmd/root.go wires a real store under
	// <runtime-anchor>/plans when the REPL starts.
	PlanStore *PlanStore

	// AttachedLogMaxBytes caps every REPL attach surface (`/log
	// <path>`, `/log` paste mode, splitPastedLog auto-route) so a
	// runaway paste cannot balloon the REPL process memory. Mirrors
	// cmd's maxAttachedLogBytes — both are driven by
	// codrax.yaml :: log_attach_max_bytes. Zero or negative →
	// DefaultAttachedLogMaxBytes (50 MB), matching the CLI default.
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
	// REPL transition into a non-read mode (`/mode plan|apply|verify`,
	// `/approve`). When false, the REPL refuses the transition with a
	// clear error pointing at the yaml knob — the alternative was the
	// pre-fix silent state where /mode plan accepted, the planner
	// dispatched, the analyzer failed in a confusing way ("hypothesis
	// coverage" / "context canceled"), and the user had no idea
	// write_enabled was the cause. cmd/root.go forwards
	// runtime_settings.WriteEnabled.
	WriteEnabled bool

	// WriteAutoInitRepo mirrors the resolved auto-init authorization
	// (yaml `write_auto_init_repo` OR CLI `--auto-init-repo`). When
	// true, the REPL's /approve flow skips the interactive y/N
	// consent prompt for bare/headless repos and silently
	// authorizes the orchestrator to scaffold. When false (default),
	// /approve runs DetectRepoState before dispatching and prompts
	// for consent if the target needs init.
	WriteAutoInitRepo bool

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
}

// REPL drives the interactive prompt.
type REPL struct {
	runner            Runner
	store             *memory.Store
	render            ResultRenderer
	renderer          *render.Renderer
	repoRoot          string
	runtimeAnchor     string
	worktreeKeepTTL   time.Duration
	worktreeKeepMaxCount int
	branch            string
	in                io.Reader
	out               io.Writer
	prompt            string
	promptCont        string
	bannerText        string
	scanner           *bufio.Scanner // lazy-init for line-oriented mode
	pasteFoldMinChars int            // per-session paste-fold threshold (runes)
	version           string
	buildTime         string
	language          string

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
	attachedHitrace string

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
	// until explicit /mode read.
	currentMode types.PipelineMode

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
	// (rejects `/mode plan|apply|verify`) and by handleApprove (rejects
	// /approve) so a user with `write_enabled: false` in codrax.yaml
	// gets a clean error at the slash-command surface instead of a
	// confusing analyzer/planner failure deep inside the pipeline.
	writeEnabled bool

	// writeAutoInitRepo mirrors Config.WriteAutoInitRepo. When true
	// the /approve flow skips the y/N consent prompt and silently
	// authorizes the orchestrator's auto-init path on bare /
	// commitless repos. When false, /approve calls
	// worktree.DetectRepoState first; if NeedsInit() returns true,
	// the user is prompted before any state mutation.
	writeAutoInitRepo bool

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
	runInFlight     atomic.Bool
	cancelSigOnce   sync.Once // installs the signal handler exactly once per REPL lifetime
	lastCancelSig   atomic.Int64 // unix nano of the previous Ctrl+C while in-flight; backs the double-tap escalation

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
		runner:             cfg.Runner,
		store:              cfg.Store,
		render:             cfg.Render,
		renderer:           cfg.Renderer,
		repoRoot:             cfg.RepoRoot,
		runtimeAnchor:        cfg.RuntimeAnchor,
		worktreeKeepTTL:      cfg.WorktreeKeepTTL,
		worktreeKeepMaxCount: cfg.WorktreeKeepMaxCount,
		branch:             cfg.Branch,
		in:                 cfg.In,
		out:                cfg.Out,
		prompt:             cfg.Prompt,
		promptCont:         cfg.PromptCont,
		bannerText:         cfg.Banner,
		pasteFoldMinChars:  cfg.PasteFoldMinChars,
		version:            cfg.Version,
		buildTime:          cfg.BuildTime,
		language:           cfg.Language,
		chitchatResponder:  cfg.ChitchatResponder,
		memory:             cfg.Memory,
		envSettings:        types.ResolvedEnvRecommendSettings(cfg.EnvSettings),
		colorMode:          cfg.ColorMode,
		chitchatClassifier: cfg.ChitchatClassifier,
		// Session ID embeds nano + pid so two codrax REPLs launched
		// in the same clock tick (test harness, race) still get
		// disjoint IDs. Consumed by memory.BuildContext via BuildOpts.
		sessionID:           fmt.Sprintf("sess-%d-%d", time.Now().UnixNano(), os.Getpid()),
		currentMode:         types.ModeRead, // B0 sticky mode; /mode rewrites
		planStore:             cfg.PlanStore,
		attachedLogMaxBytes:   cfg.AttachedLogMaxBytes,
		attachedTraceMaxBytes: cfg.AttachedTraceMaxBytes,
		writeEnabled:          cfg.WriteEnabled,
		writeAutoInitRepo:     cfg.WriteAutoInitRepo,
		settingsPath:          cfg.SettingsPath,
	}
	if r.version == "" {
		r.version = "dev"
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
var (
	subduedInfoPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgCyan),
			Text:  "•",
		},
	}
	subduedSuccessPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgGreen),
			Text:  "✓",
		},
	}
	subduedWarnPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgYellow),
			Text:  "⚠",
		},
	}
	subduedErrorPrinter = pterm.PrefixPrinter{
		Prefix: pterm.Prefix{
			Style: pterm.NewStyle(pterm.FgRed),
			Text:  "✗",
		},
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

// currentStickyTag returns the per-turn sticky-state marker
// promptStickyTag would compose for the REPL's current state. Single
// call site so every prompt-rendering surface (main input, paste/log
// capture mode, multi-line continuation, scripted-mode echo) sees the
// SAME tag for a given turn — without this helper each surface
// re-derived the tag (or worse, omitted it) and the user lost
// visibility of [mode:plan] / [log] / [trace] / [plan] / [mem!] mid-
// flow.
func (r *REPL) currentStickyTag() string {
	// Probe git branch fresh per call — this fires once per user
	// input, so the cost is bounded (one local git exec ~1ms) and
	// the user sees branch changes from another terminal at the
	// VERY NEXT prompt cycle. No caching; correctness > 1ms.
	return promptStickyTag(
		string(r.currentMode),
		gitBranchProbe(r.repoRoot),
		r.attachedLog != "",
		r.attachedHitrace != "",
		r.pendingPlanPath != "",
		r.memoryUnderPressure(),
	)
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
// Behaviour by state:
//
//   - !runInFlight (idle prompt): operator hit Ctrl+C with no Run
//     active. Drive cleanup ourselves and exit cleanly. Bubbletea's
//     bracketed-paste / readline modes don't handle SIGINT
//     gracefully on every shell, so an explicit cleanup-and-exit
//     mirrors the historical worktree-handler behaviour without the
//     race.
//   - runInFlight + first signal within doubleSigCancelWindow: call
//     runner.Cancel("Ctrl+C") and stamp lastCancelSig. The pipeline
//     unwinds at its next checkpoint; the user sees the standard
//     "✗ canceled" rendering when Run returns.
//   - runInFlight + second signal within doubleSigCancelWindow:
//     escalate — operator explicitly asked twice. Drive worktree
//     cleanup + os.Exit. We do NOT just re-raise: signal.Reset would
//     unsubscribe BOTH our channel and worktree's, leaving Go's
//     default disposition which exits without cleanup.
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
				if !r.runInFlight.Load() {
					// Idle prompt: clean worktrees and exit. Mirrors
					// the historical behaviour but routes through us
					// instead of the suppressed package handler.
					worktree.CleanActiveSessions()
					fmt.Fprintln(r.out, "  Goodbye!")
					os.Exit(130)
				}
				now := time.Now().UnixNano()
				prev := r.lastCancelSig.Swap(now)
				if prev > 0 && time.Duration(now-prev) < doubleSigCancelWindow {
					// Second signal within the window — escalate to
					// exit. Drive cleanup ourselves since the package
					// handler is suppressed.
					worktree.CleanActiveSessions()
					fmt.Fprintln(r.out, "  Goodbye!")
					os.Exit(130)
				}
				canceller.Cancel("Ctrl+C")
				// Also cancel any in-flight chitchat turn ctx — chitchat
				// runs on the REPL goroutine outside runInFlightWrap, so
				// orchestrator's Cancel doesn't reach it. The per-turn
				// cancel func (set by chitchatDispatch / classifier
				// dispatch) is the surface that closes the chitchat
				// HTTP socket immediately.
				r.cancelTurn()
				r.warn("%s\n", cancelInProgressMsg(r.language))
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
			if quit := r.handleSlash(cmd); quit {
				return nil
			}
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
	badge := pterm.NewStyle(pterm.BgBlue, pterm.FgWhite, pterm.Bold).Sprint(" CODRAX ")
	// Compact banner version: strip the -dirty build-state suffix so a
	// long-running interactive session isn't dominated by build noise.
	// The full identifier is still available via /version.
	shortVer := strings.TrimSuffix(r.version, "-dirty")
	ver := pterm.FgGray.Sprintf("v%s", shortVer)
	hint := pterm.FgDarkGray.Sprint("/help · /exit")
	// Surface git branch up-front so the operator sees which branch
	// codrax is operating against (and which one /merge will fast-
	// forward into when --branch is omitted) before they type any
	// command. Empty when the path isn't a git repo (rare for
	// codrax usage; possible during /mode plan auto-init scaffold).
	branchInfo := ""
	if br := gitBranchProbe(r.repoRoot); br != "" {
		branchInfo = pterm.FgDarkGray.Sprintf("  git:%s", br)
	}
	fmt.Fprintf(r.out, "\n  %s %s%s  %s\n", badge, ver, branchInfo, hint)
	// One-line capability summary so the user sees at startup which
	// modes are available and which yaml file backs the config. With
	// write_enabled=false the line names the gate explicitly so the
	// user does not have to wait until /mode plan to discover it.
	if cap := bannerCapabilityLine(r.language, r.writeEnabled, r.settingsPath); cap != "" {
		fmt.Fprintf(r.out, "  %s\n", pterm.FgDarkGray.Sprint(cap))
	}
	if summary := r.memorySummaryLine(); summary != "" {
		fmt.Fprintf(r.out, "  %s\n", summary)
	}
	for _, h := range r.degradedEnvHints() {
		fmt.Fprintf(r.out, "  %s\n", h)
	}
	fmt.Fprintln(r.out)
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
			hints = append(hints, warn("⚠ 搜索后端:grep")+dim(" (装 ripgrep 可进一步提速)"))
		} else {
			hints = append(hints, warn("⚠ Search backend: grep")+dim(" (install ripgrep for faster scans)"))
		}
	case "native":
		// Both missing — Go regex walker. Correct but noticeably
		// slower on large repos.
		if zh {
			hints = append(hints, warn("⚠ 搜索后端:Go 内置扫描器")+dim(" (装 ripgrep 可大幅提速)"))
		} else {
			hints = append(hints, warn("⚠ Search backend: native Go scanner")+dim(" (install ripgrep for a 10× speedup)"))
		}
	}
	// "rg" → no hint
	if gitMissing {
		if zh {
			hints = append(hints, warn("⚠ 未检测到 git")+dim(" (repomap 走文件遍历;git_diff / git_log 不可用)"))
		} else {
			hints = append(hints, warn("⚠ git not detected")+dim(" (repomap falls back to filesystem walk; git_diff / git_log disabled)"))
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
// In interactive mode it delegates to a Bubble Tea session
// (readInputInteractive) that carries paste-folding, history, and
// slash-command autocomplete. When driven by an io.Reader (tests,
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

// readInputInteractive runs the Bubble Tea input session, looping for
// trailing-"\" continuation. Each invocation can fold its own pastes
// into independent placeholder slots.
func (r *REPL) readInputInteractive(prompt string) (string, string, error) {
	cur := prompt
	var expandedParts, displayParts []string
	for {
		res, err := r.readInputBubble(cur, len(expandedParts) > 0)
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
		// plan-mode input does not lose the [mode:plan] marker after
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

	// Auto chit-chat classification. Opt-in; nil classifier means the
	// REPL falls back to explicit /chat only. An attached log (sticky
	// or auto-routed) is strong evidence of a code question, so the
	// classifier is skipped in that case to save an LLM call. Fail-
	// safe: any classifier error (nil adapter, chat error, unparseable
	// response, unknown decision) routes to the pipeline unchanged,
	// so a broken classifier cannot silently misroute real questions.
	// `/mode plan|apply|verify` is an explicit user intent to drive
	// the write pipeline — skip the classifier so a "no repo context"
	// verdict cannot reroute a write-mode turn to chit-chat.
	// `/mode plan|apply|verify` is an explicit user intent to drive
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
	if r.currentMode == types.ModeRead &&
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
		if hasAttach := r.attachedLog != "" || r.attachedHitrace != "" ||
			r.attachedLogAutoRouted; hasAttach {
			if hint != "" {
				hint += " attachment=true"
			} else {
				hint = "attachment=true"
			}
		}
		// Per-turn ctx so a Ctrl+C during the classifier LLM call
		// closes the HTTP socket immediately. Cleared after dispatch
		// completes so the chitchat path can install its own ctx.
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

	prior := r.store.BuildContext(line, memory.BuildOpts{
		Kind:      memory.KindPipeline,
		SessionID: r.sessionID,
	})
	effective := line
	if prior != "" {
		effective = "## Prior conversation\n" + prior + "\n\n## Current request\n" + line
	}

	logging.Info("[repl] dispatching request: %s", oneLine(line))

	// Propagate sticky attached-log to the runner. Runners without
	// SetAttachedLog simply skip this step (tests).
	if setter, ok := r.runner.(attachedLogSetter); ok {
		setter.SetAttachedLog(r.attachedLog)
	}
	// Same propagation for the perf channel.
	if setter, ok := r.runner.(attachedHitraceSetter); ok {
		setter.SetAttachedHitrace(r.attachedHitrace)
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

	if r.renderer != nil {
		r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	}

	busCtx, err := r.runInFlightWrap(func() (*types.BusContext, error) {
		return r.runner.Run(effective, r.repoRoot, r.branch)
	})

	if r.renderer != nil {
		// Stop spinner BEFORE printing response so task list comes first.
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
				r.info(fmt.Sprintf("plan saved: %s", path))
				// Action nudge: surface the next-step slash commands so
				// the user does not have to remember /approve / /reject
				// after seeing the bordered plan summary above.
				for _, line := range planReadyNudge(r.language, plan.ID, len(plan.Changes)) {
					r.info(line)
				}
			}
		}
	}

	response := strings.TrimSpace(r.render(busCtx))
	logging.Info("[repl] final answer:\n%s", response)

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
	if busCtx != nil && busCtx.TaskState.LastError != "" {
		memResponse = "(previous attempt ended in error — details omitted from memory)"
	}

	// Skip rendering if no meaningful content.
	if response == "" || response == "(no result)" {
		fmt.Fprintln(r.out, emptyResponseHint(r.language))
		r.recordTurn(display, line, memResponse, memory.KindPipeline)
		return
	}

	r.renderBordered(response)
	r.recordTurn(display, line, memResponse, memory.KindPipeline)
}

// renderBordered prints model output with a continuous left border.
// Shared by the pipeline dispatch path and the /chat chitchat path so
// both answer surfaces get identical visual treatment. Trailing ANSI
// escapes and whitespace are stripped so the bar aligns cleanly.
func (r *REPL) renderBordered(response string) {
	raw := strings.Split(response, "\n")
	var lines []string
	prevBlank := false
	for _, ln := range raw {
		clean := stripTrailing(ln)
		blank := stripANSI(clean) == ""
		if blank && prevBlank {
			continue
		}
		prevBlank = blank
		lines = append(lines, clean)
	}
	for len(lines) > 0 && stripANSI(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && stripANSI(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	bar := pterm.FgWhite.Sprint("│")
	// Wrap each line to fit the terminal, accounting for the
	// "  │ " prefix (4 display cols + 2 margin).
	maxContent := 60
	if w, _, werr := term.GetSize(int(os.Stdout.Fd())); werr == nil && w > 10 {
		maxContent = w - 6
	}
	fmt.Fprintf(r.out, "  %s\n", bar)
	for _, ln := range lines {
		wrapped := wrapByWidth(ln, maxContent)
		for _, wl := range wrapped {
			fmt.Fprintf(r.out, "  %s %s\n", bar, wl)
		}
	}
	fmt.Fprintf(r.out, "  %s\n\n", bar)
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

// DefaultAttachedLogMaxBytes is the out-of-the-box 50 MB cap on
// every REPL attach surface (/log + /htrace). Consumed by New when
// Config.AttachedLogMaxBytes is not set; the cmd layer populates
// Config from codrax.yaml :: log_attach_max_bytes so both CLI and
// REPL paths honour the same override. Mirrors
// cmd/root.go :: defaultAttachedLogMaxBytes.
// DefaultAttachedLogMaxBytes is the REPL-side default cap. Mirrors
// cmd.defaultAttachedLogMaxBytes — the two constants must agree so
// a unit test that bypasses initApp sees the same baseline as a
// real CLI run. Raised from 1 MB → 50 MB in 2026-04 to match real
// HarmonyOS / Android log + trace volumes (hdc / adb captures
// commonly exceed 10 MB; the previous 1 MB silently truncated tails
// where the actual error frames lived).
const DefaultAttachedLogMaxBytes = 50 * 1024 * 1024 // 50 MB

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
	case "/cancel":
		// Slash-command fallback for terminals where Ctrl+C is
		// swallowed by tmux, screen, or a terminal multiplexer the
		// operator can't reconfigure. Emits the same Cancel(reason)
		// the Ctrl+C path emits, so the rendering / state-saving
		// behaviour is identical.
		r.handleCancelCmd(line)
		return false
	case "/exit", "/quit":
		fmt.Fprintln(r.out, "  Goodbye!")
		return true
	case "/clear":
		peers, perr := r.store.LivePeerCount()
		if perr != nil {
			logging.Warning("[repl] live peer count failed: %v", perr)
		}
		msg := "/clear wipes this conversation memory (MEMORY.md + turns/)."
		if peers == 1 {
			msg += " 1 other live codrax instance shares this directory."
		} else if peers > 1 {
			msg += fmt.Sprintf(" %d other live codrax instances share this directory.", peers)
		}
		confirmed := false
		if r.interactive() {
			err := huh.NewConfirm().
				Title(msg).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed).
				Run()
			if err != nil {
				confirmed = false
			}
		} else {
			// Line-oriented mode: print message and read y/n.
			fmt.Fprintln(r.out, msg)
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
		// Auto-generated from slashCommands so the /help output and
		// the autocomplete panel can never drift apart. Table-driven
		// keeps the bilingual contract and lists EVERY command — the
		// pre-T4 hardcoded list omitted /htrace and /atrace.
		for _, line := range helpLines(r.language) {
			r.info(line)
		}
	default:
		r.warn("%s", unknownSlashCommand(r.language, cmd))
	}
	return false
}

// handleModeCmd dispatches the `/mode` subcommands. Recognised forms:
//
//	/mode                  — print current mode
//	/mode read|plan|apply|verify — set current mode
//
// The /mode state is sticky for the REPL session; every subsequent
// dispatch calls runner.SetMode(currentMode) so the orchestrator
// observes the selection. Entering plan / apply / verify before
// codrax.yaml has write_enabled=true will silently stay local to
// REPL state — the orchestrator's resolveWriteMode validation runs
// at initApp time (CLI layer), not per-turn, so REPL-level /mode
// does NOT re-validate. The orchestrator's the plan stage hook /
// the apply stage hook / the verify stage hook dispatch is what eventually
// surfaces any issue (e.g. "planner agent not wired").
func (r *REPL) handleModeCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/mode"))
	if rest == "" {
		r.info(fmt.Sprintf("current mode: %s (use /mode <read|plan|apply|verify> to change)",
			r.currentMode))
		return
	}
	target := types.PipelineMode(strings.ToLower(rest))
	if !target.IsValid() {
		r.warn("%s", unknownModeValue(r.language, rest))
		return
	}
	if target == "" {
		target = types.ModeRead
	}
	// L2 gate: every non-read mode requires `write_enabled: true` in
	// codrax.yaml. Without this check the REPL silently accepted the
	// transition; the actual failure surfaced deep inside the pipeline
	// as a confusing analyzer / planner error (e.g. "hypothesis
	// coverage rejected", "context canceled"). cmd/root.go's
	// resolveWriteMode covers --mode flag, but REPL's /mode bypassed
	// it entirely. Reject up-front with a precise hint.
	if target != types.ModeRead && !r.writeEnabled {
		for _, line := range writeModeDisabled(r.language, "/mode "+string(target), r.settingsPath) {
			r.warn("%s\n", line)
		}
		return
	}
	// Apply / verify modes normally require a --plan-file; in REPL
	// the user is expected to first /mode plan → generate plan →
	// then /mode apply. B0 does not prevent the transition at
	// /mode time because /approve (B1) will be the real dispatcher
	// for apply; /mode alone is harmless — the apply stage is a
	// stub and will fail-loud on the next dispatch.
	r.currentMode = target
	r.success(modeSwitched(r.language, string(target)))
	// Workflow hint: explain in 1-3 lines what the new mode actually
	// does. Empty for ModeRead (no special workflow). Surfaced once per
	// /mode transition so a user new to write mode does not have to
	// read the docs to find /approve / /reject / /mode read.
	for _, line := range modeWorkflowHint(r.language, string(target)) {
		r.info(line)
	}
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
	// /plan show <plan-id> — explicit target. Falls through to the
	// existing show path with the loaded plan's ID; r.pendingPlanPath
	// is rebound to the named plan so subsequent /approve targets the
	// same plan without retyping the ID.
	if showID := strings.TrimSpace(strings.TrimPrefix(rest, "show ")); showID != rest && showID != "" {
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
				// When write is disabled, "/mode plan" itself would bounce
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
		if len(plan.TargetPaths) > 0 {
			fmt.Fprintf(r.out, "    targets: %s\n", strings.Join(plan.TargetPaths, ", "))
		}
		if plan.Summary != "" {
			fmt.Fprintf(r.out, "    summary: %s\n", oneLine(plan.Summary))
		}
		// Per-change diff preview so the user /approves with full
		// knowledge. Caps: 16 KB total / 4 KB per change — enough to
		// inspect a small surgical plan, bounded so a monster plan
		// doesn't flood the terminal; the cap message tells the user
		// to read the plan JSON for full detail.
		fmt.Fprintln(r.out, "\n  diff preview:")
		fmt.Fprint(r.out, render.ColorizeUnifiedDiff(
			renderPlanDiff(plan, r.repoRoot, 16*1024, 4*1024),
			r.colorMode, r.out))
		// Footer: name the next slash commands so the user does not
		// have to remember them after reading the diff. Only printed
		// when the plan is still actionable (pending_approval); a plan
		// already consumed by apply / verify shows status above and
		// the action menu would be misleading.
		if plan.Status == types.PlanStatusPending {
			for _, line := range planShowFooter(r.language) {
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
			fmt.Fprintf(r.out, "    - [%s] %s  status=%s  (%d bytes)\n",
				ts, inf.ID, status, inf.SizeB)
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
	var rows []string
	for _, inf := range infos {
		status := inf.Status
		if status == "" {
			status = "unknown"
		}
		reportPath := strings.TrimSuffix(inf.Path, ".json") + ".report.json"
		row := fmt.Sprintf("%s  status=%s", inf.ID, status)
		if data, rerr := os.ReadFile(reportPath); rerr == nil {
			// Minimal JSON probe so we don't pull the full
			// types.ChangeReport shape into this package beyond
			// what /history needs to render.
			var probe struct {
				Passed      bool `json:"passed"`
				TestResults []struct {
					AssertionID string `json:"assertion_id"`
					Passed      bool   `json:"passed"`
				} `json:"test_results"`
			}
			if jerr := json.Unmarshal(data, &probe); jerr == nil {
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
			} else {
				row += "  (report unreadable)"
			}
		} else {
			row += "  (no report)"
		}
		rows = append(rows, row)
	}
	return rows
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
//  2. pendingPlanPath non-empty — user must run a /mode plan
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
	planArg, mergeTo, skipVerify := parseApproveArgs(line)
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
	//      noPendingPlan info; the user runs /mode plan or /plan
	//      list to see what's available.
	if planArg != "" {
		full, err := r.planStore.Load(planArg)
		if err != nil || full == nil {
			r.errorf("approve: %s", planNotFound(r.language, planArg))
			return
		}
		r.pendingPlanPath = filepath.Join(r.planStore.PlanDir(), planArg+".json")
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
	// out of /mode plan). verify_failed is the env-fix retry path:
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

	// Multi-pending hint. When PlanStore carries more than one
	// pending plan, surface the count + how to target a specific
	// one before the confirm prompt — this prevents the operator
	// from approving "the wrong one" when /plan list has many
	// rows and they forget which is the latest.
	if extras := countOtherPendingPlans(r.planStore, plan.ID); extras > 0 {
		r.info(otherPendingPlansHint(r.language, plan.ID, extras))
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

	title := approveTitlePrompt(r.language, plan.ID, len(plan.Changes), skipVerify)
	confirmed := false
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
	if !confirmed {
		r.info(approveCancelled(r.language))
		return
	}

	// Bare-directory scaffolding gate. Before handing the plan to the
	// orchestrator, probe the target repo. If it's not a git repo, or
	// the repo has no HEAD commit yet, `git worktree add --detach HEAD`
	// inside worktree.Create will fail. Three-tier authorization:
	//
	//   1. yaml/CLI pre-authorized (writeAutoInitRepo == true) →
	//      forward the flag to the orchestrator and proceed silently.
	//   2. interactive REPL → prompt y/N; on yes, forward; on no,
	//      print the recovery hint and bail (plan stays pending).
	//   3. non-interactive REPL (scripted) without pre-auth → bail
	//      with the same hint.
	//
	// The orchestrator's apply pre-hook does the actual EnsureInitialCommit
	// + worktree.Create call so all writes happen in one place.
	if state, derr := worktree.DetectRepoState(r.repoRoot); derr == nil && state.NeedsInit() {
		if !r.writeAutoInitRepo {
			if !r.interactive() {
				r.errorf("%s", approveBareDirNoAuthMsg(r.language, r.repoRoot, state.String()))
				return
			}
			autoConsent := false
			if err := huh.NewConfirm().
				Title(autoInitConsentTitle(r.language, r.repoRoot, state.String())).
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
		}
		// Per-Run authorization: forward to orchestrator. We restore
		// the prior value in the defer so subsequent /approve calls
		// against a different (already-initialized) repo don't keep
		// the gate lifted.
		if setter, ok := r.runner.(autoInitRepoSetter); ok {
			setter.SetAutoInitRepo(true)
			defer setter.SetAutoInitRepo(r.writeAutoInitRepo)
		}
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
	// though the orchestrator discards the AnalysisIR in write mode
	// (BuildWriteTaskGraph supersedes the analyzer's TaskGraph), the
	// classifier still has to terminate cleanly. Use plan.Summary so
	// the analyzer sees code-question content; fall back to a generic
	// phrasing when the planner left the summary blank.
	request := approveDispatchRequest(plan)
	logging.Info("[repl] dispatching approve: plan=%s path=%s", plan.ID, r.pendingPlanPath)

	if r.renderer != nil {
		r.renderer.StartSpinnerWithCancelHint(spinnerCancelHint(r.language))
	}
	busCtx, runErr := r.runInFlightWrap(func() (*types.BusContext, error) {
		return r.runner.Run(request, r.repoRoot, r.branch)
	})
	if r.renderer != nil {
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
	// adds an italic "Re-plan with /mode plan..." line INSIDE the
	// markdown body, but bordered+italic is easy to miss. Print a
	// plain stderr-style hint OUTSIDE the bordered render so the
	// next-steps are unambiguous. /approve does NOT auto-retry-replan
	// (intentional: the approve gate's contract is "exactly the plan
	// I reviewed lands"), so the recovery path is explicit user
	// action — re-run plan-mode incorporating the failure.
	if busCtx != nil && busCtx.TaskState.LastError != "" {
		for _, line := range approveFailedNudge(r.language) {
			r.info(line)
		}
	} else {
		// Success path nudge: after /approve completed, currentMode is
		// restored to whatever it was before (typically ModePlan since
		// the user just emitted a plan). Without this hint the user's
		// next prompt would generate ANOTHER plan — usually not what
		// they want right after a successful apply. Point at /mode read
		// for further questions and /mode plan for another change.
		for _, line := range applyDoneNudge(r.language) {
			r.info(line)
		}
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
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/approve"))
	if rest == "" {
		return "", "", false
	}
	tokens := strings.Fields(rest)
	for i, tok := range tokens {
		switch {
		case tok == "--skip-verify":
			skipVerify = true
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
	return planID, mergeTo, skipVerify
}

// parseMergeToArg is the legacy single-purpose helper kept as a
// thin wrapper for tests + downstream callers that already use it.
// New code should prefer parseApproveArgs.
func parseMergeToArg(line string) string {
	_, mergeTo, _ := parseApproveArgs(line)
	return mergeTo
}

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
	for _, line := range mergeSuccess(r.language, res.Strategy, res.FinalBranch, len(res.CommitsLanded)) {
		r.info(line)
	}
	// Worktree successfully folded back — discard it so /worktree
	// list stays honest. This is opt-in via the merge step; users
	// who don't run /merge keep the preserved worktree as before.
	if err := worktree.DiscardByPath(worktreePath, mainRoot); err != nil {
		logging.Warning("[repl] post-merge worktree discard failed: %v", err)
	}
	// Clear the WorktreePath field on disk so /worktree list and
	// /merge no longer show this entry as a candidate. Status stays
	// `applied` — the plan history is permanent.
	if err := r.clearWorktreePathOnAppliedPlan(worktreePath); err != nil {
		logging.Warning("[repl] post-merge plan WorktreePath clear failed: %v", err)
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
	for _, line := range mergeSuccess(r.language, res.Strategy, res.FinalBranch, len(res.CommitsLanded)) {
		r.info(line)
	}
	logging.Info("[repl] post-merge: plan %s landed via recovery ref %s (worktree was already discarded)",
		planID, ref)
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
	for _, tok := range strings.Fields(rest) {
		switch {
		case strings.HasPrefix(tok, "--branch="):
			target = strings.TrimSpace(strings.TrimPrefix(tok, "--branch="))
		case tok == "--include-failed", tok == "--force":
			// --include-failed (preferred name) and --force (alias)
			// allow merging a verify_failed plan. Use case: tests
			// require infra the local box can't run (integration DB,
			// external API, GPU); the operator reviews the diff,
			// decides the failures are acceptable, and merges to
			// run tests in CI. Without this flag /merge skips
			// verify_failed plans because the safe default is "only
			// merge code that passed verify".
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
		wt           string
		planID       string
		planStatus   string
		recoverySHA  string // populated when surface 1 misses but surface 2 hits
		recoveryRef  string
	)
	for _, inf := range infos {
		eligible := inf.Status == types.PlanStatusApplied ||
			(includeFailed && inf.Status == types.PlanStatusVerifyFailed)
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
	if wt == "" && recoveryRef == "" {
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
	if r.planStore == nil {
		r.info(commandDisabled(r.language, "/reject", noPlanStoreReason(r.language)))
		return
	}
	if r.pendingPlanPath == "" {
		r.info(noPendingPlanReject(r.language))
		return
	}
	reason := strings.TrimSpace(strings.TrimPrefix(line, "/reject"))
	id := strings.TrimSuffix(filepath.Base(r.pendingPlanPath), ".json")
	if err := r.planStore.Clear(id); err != nil {
		r.errorf("reject: %v\n", err)
		return
	}
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
	data, err := os.ReadFile(path)
	if err != nil {
		r.errorf("append log: %v\n", err)
		return
	}
	header := "# codrax-source: " + path + "\n"
	combined := r.attachedLog
	if combined != "" {
		combined += "\n"
	}
	combined += header + string(data)
	if len(combined) > r.attachedLogMaxBytes {
		r.warn("appended log truncated: %d → %d bytes\n", len(combined), r.attachedLogMaxBytes)
		combined = combined[:r.attachedLogMaxBytes]
	}
	r.attachedLog = combined
	r.attachedLogAutoRouted = false
	r.success(fmt.Sprintf("appended %s (%d bytes added; total %d bytes)", path, len(data), len(r.attachedLog)))
}

// handleHitraceCmd is the perf-channel companion to handleLogCmd.
// Mirrors the surface for HiTrace / Android-systrace attachments:
//
//	/htrace <path>          — load file from disk (replaces any prior)
//	/htrace append <path>   — append additional file with source header
//	/htrace clear           — clear the current attachment
//	/htrace show            — print byte count + head of the attachment
//	/htrace                 — bare invocation prints usage; no paste
//	                          mode (traces are usually multi-MB hdc
//	                          / adb dumps, not hand-typed)
func (r *REPL) handleHitraceCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/htrace"))
	switch {
	case rest == "":
		r.info("/htrace <path> | append <path> | clear | show — attach a HiTrace / atrace / systrace / perfetto file")
	case rest == "clear":
		if r.attachedHitrace == "" {
			r.info(noTraceAttached(r.language))
			return
		}
		r.attachedHitrace = ""
		if setter, ok := r.runner.(attachedHitraceSetter); ok {
			setter.SetAttachedHitrace("")
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
	default:
		data, err := os.ReadFile(rest)
		if err != nil {
			r.errorf("load hitrace: %v\n", err)
			return
		}
		if len(data) > r.attachedTraceMaxBytes {
			r.warn("hitrace truncated: %d → %d bytes\n", len(data), r.attachedTraceMaxBytes)
			data = data[:r.attachedTraceMaxBytes]
		}
		// Single-path load also gets the source header so the LLM
		// sees a consistent boundary marker shape regardless of
		// how the trace got attached (single load / append / CLI).
		header := "# codrax-source: " + rest + "\n"
		body := header + string(data)
		if len(body) > r.attachedTraceMaxBytes {
			body = body[:r.attachedTraceMaxBytes]
		}
		r.attachedHitrace = body
		r.success(fmt.Sprintf("attached hitrace loaded: %s (%d bytes)", rest, len(data)))
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
	data, err := os.ReadFile(path)
	if err != nil {
		r.errorf("append hitrace: %v\n", err)
		return
	}
	header := "# codrax-source: " + path + "\n"
	combined := r.attachedHitrace
	if combined != "" {
		combined += "\n"
	}
	combined += header + string(data)
	if len(combined) > r.attachedTraceMaxBytes {
		r.warn("appended hitrace truncated: %d → %d bytes\n", len(combined), r.attachedTraceMaxBytes)
		combined = combined[:r.attachedTraceMaxBytes]
	}
	r.attachedHitrace = combined
	r.success(fmt.Sprintf("appended %s (%d bytes added; total %d bytes)", path, len(data), len(r.attachedHitrace)))
}

// handleLogLoad reads a log file from disk into the sticky buffer.
// Replaces any existing attachment (a `/log append` variant is a
// future add; users can cat files together themselves for now).
func (r *REPL) handleLogLoad(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		r.errorf("load log: %v\n", err)
		return
	}
	if len(data) > r.attachedLogMaxBytes {
		r.warn("log truncated: %d → %d bytes\n", len(data), r.attachedLogMaxBytes)
		data = data[:r.attachedLogMaxBytes]
	}
	r.attachedLog = string(data)
	r.attachedLogAutoRouted = false
	r.success(fmt.Sprintf("attached log loaded: %s (%d bytes)", path, len(data)))
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

var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var reTrailing = regexp.MustCompile(`(\s|\x1b\[[0-9;]*m)+$`)

// stripTrailing removes all trailing whitespace and ANSI sequences.
func stripTrailing(s string) string {
	return reTrailing.ReplaceAllString(s, "")
}

// stripANSI removes all ANSI escape sequences to get plain text.
func stripANSI(s string) string {
	return strings.TrimSpace(reANSI.ReplaceAllString(s, ""))
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
