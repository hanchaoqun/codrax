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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
)

// Runner is the orchestrator-shaped surface the REPL needs. Defined
// here as an interface so tests can stub it without pulling in the
// full pipeline.
type Runner interface {
	Run(request, repoRoot, branch string) (*types.BusContext, error)
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
}

// REPL drives the interactive prompt.
type REPL struct {
	runner            Runner
	store             *memory.Store
	render            ResultRenderer
	renderer          *render.Renderer
	repoRoot          string
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
}

// New constructs a REPL from a Config.
func New(cfg Config) *REPL {
	r := &REPL{
		runner:             cfg.Runner,
		store:              cfg.Store,
		render:             cfg.Render,
		renderer:           cfg.Renderer,
		repoRoot:           cfg.RepoRoot,
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
		chitchatClassifier: cfg.ChitchatClassifier,
		// Session ID embeds nano + pid so two codrax REPLs launched
		// in the same clock tick (test harness, race) still get
		// disjoint IDs. Consumed by memory.BuildContext via BuildOpts.
		sessionID:           fmt.Sprintf("sess-%d-%d", time.Now().UnixNano(), os.Getpid()),
		currentMode:         types.ModeRead, // B0 sticky mode; /mode rewrites
		planStore:             cfg.PlanStore,
		attachedLogMaxBytes:   cfg.AttachedLogMaxBytes,
		attachedTraceMaxBytes: cfg.AttachedTraceMaxBytes,
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

// info prints an informational message. In interactive mode it uses
// pterm styling; in line-oriented mode it writes plain text to r.out
// so tests can capture it.
func (r *REPL) info(msg string) {
	if r.interactive() {
		pterm.Info.Println(msg)
	} else {
		fmt.Fprintln(r.out, msg)
	}
}

// success prints a success message.
func (r *REPL) success(msg string) {
	if r.interactive() {
		pterm.Success.Println(msg)
	} else {
		fmt.Fprintln(r.out, msg)
	}
}

// errorf prints an error message.
func (r *REPL) errorf(format string, args ...interface{}) {
	if r.interactive() {
		pterm.Error.Printf(format, args...)
	} else {
		fmt.Fprintf(r.out, format, args...)
	}
}

// warn prints a warning message.
func (r *REPL) warn(format string, args ...interface{}) {
	if r.interactive() {
		pterm.Warning.Printf(format, args...)
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

// Loop runs the prompt until /exit, /quit, or EOF.
//
// The interactive readInput session renders its own echo + divider
// above the inline viewport via tea.Printf, so this function only
// owns echo in the line-oriented (scripted/piped) branch.
func (r *REPL) Loop() error {
	r.banner()
	memNudgeShown := false
	for {
		// Sticky-state tag prepended to the prompt so the user has a
		// constant visual reminder of what attachments / mode / plan
		// are live for this turn. Empty when nothing sticky.
		tag := promptStickyTag(
			string(r.currentMode),
			r.attachedLog != "",
			r.attachedHitrace != "",
			r.pendingPlanPath != "",
			r.memoryUnderPressure(),
		)
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
			// Scanner mode: preserve the old echo so piped test output
			// still contains a "> …" marker for visual assertions.
			fmt.Fprintf(r.out, "  > %s\n", display)
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
	fmt.Fprintf(r.out, "\n  %s %s  %s\n", badge, ver, hint)
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
	s, err := r.readInputLines()
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
		cur = "…"
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
func (r *REPL) readInputLines() (string, error) {
	r.scriptedScanner()
	prompt := r.prompt
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
	if r.chitchatClassifier != nil && r.chitchatResponder != nil &&
		!r.attachedLogAutoRouted &&
		strings.TrimSpace(r.attachedLog) == "" &&
		strings.TrimSpace(r.attachedHitrace) == "" {
		if isChat, err := r.chitchatClassifier.Classify(line); err != nil {
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
		r.renderer.StartSpinner()
	}

	busCtx, err := r.runner.Run(effective, r.repoRoot, r.branch)

	if r.renderer != nil {
		// Stop spinner BEFORE printing response so task list comes first.
		r.renderer.StopSpinner()
	}

	if err != nil {
		logging.Error("[repl] orchestrator error: %v", err)
		r.errorf("error: %v\n", err)
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
		fmt.Fprintln(r.out, "  ??")
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
			line, err := r.readInputLines()
			if err == nil {
				confirmed = strings.TrimSpace(strings.ToLower(line)) == "y"
			}
		}
		if !confirmed {
			r.info("clear cancelled")
			break
		}
		if err := r.store.Clear(); err != nil {
			r.errorf("clear failed: %v\n", err)
		} else {
			r.success("conversation memory cleared.")
		}
	case "/history":
		recent := r.store.Recent()
		idx := r.store.Index()
		planRows := r.collectPlanHistory()
		if len(recent) == 0 && len(idx) == 0 && len(planRows) == 0 {
			r.info("(empty)")
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
		r.info("commands: /exit /quit /clear /history /compact /log /paste /chat /mode /plan /approve /reject /verify /worktree /version /help")
		r.info("tip: end a line with \\ for multi-line input")
		r.info("log: /log <path> | /log (paste then /end) | /log clear | /log show")
		r.info("paste: /paste (terminal-independent fallback when bracketed paste is stripped)")
		r.info("chat: /chat <message> (reply without invoking the analysis pipeline)")
		r.info("mode: /mode [read|plan|apply|verify] (show/set; read default; write modes require codrax.yaml :: write_enabled: true)")
		r.info("plan: /plan [show|clear|list] (inspect/manage saved ChangePlans from write-mode Runs)")
		r.info("approve: /approve (consume pending plan — triggers apply + verify inside a git worktree)")
		r.info("reject:  /reject [reason] (discard pending plan without applying)")
		r.info("verify:  /verify [plan-id] (re-run verify against an applied plan; requires preserved worktree)")
		r.info("worktree: /worktree [list|discard <plan-id>] (manage preserved worktrees from successful applies)")
		r.info("history: /history now lists plans applied/attempted alongside recent turns + compacted index")
		r.info("version: /version (or /v) prints the build identifier")
	default:
		r.warn("unknown command %q — try /help\n", cmd)
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
		r.warn("unknown mode %q — expected one of: read, plan, apply, verify\n", rest)
		return
	}
	if target == "" {
		target = types.ModeRead
	}
	// Apply / verify modes normally require a --plan-file; in REPL
	// the user is expected to first /mode plan → generate plan →
	// then /mode apply. B0 does not prevent the transition at
	// /mode time because /approve (B1) will be the real dispatcher
	// for apply; /mode alone is harmless — the apply stage is a
	// stub and will fail-loud on the next dispatch.
	r.currentMode = target
	r.success(modeSwitched(r.language, string(target)))
}

// handlePlanCmd dispatches the `/plan` subcommands. Recognised forms:
//
//	/plan                — synonym for /plan show
//	/plan show           — display the current pending plan from disk
//	/plan clear          — remove the pending plan's file
//	/plan list           — enumerate every saved plan (newest first)
//
// All subcommands require a non-nil PlanStore (cmd/root.go wires
// one for the real REPL; tests that bypass cmd/ leave it nil and
// see a "/plan disabled" message).
func (r *REPL) handlePlanCmd(line string) {
	if r.planStore == nil {
		r.info("/plan disabled (no PlanStore configured)")
		return
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/plan"))
	if rest == "" {
		rest = "show"
	}
	switch rest {
	case "show":
		if r.pendingPlanPath == "" {
			r.info(noPendingPlan(r.language))
			return
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
		fmt.Fprint(r.out, renderPlanDiff(plan, r.repoRoot, 16*1024, 4*1024))
	case "clear":
		if r.pendingPlanPath == "" {
			r.info("no pending plan to clear")
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
			r.info("no plans saved in " + r.planStore.PlanDir())
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
		r.warn("unknown /plan subcommand %q — expected: show, clear, list\n", rest)
	}
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
	_ = line // /approve takes no arguments today
	if r.planStore == nil {
		r.info("/approve disabled (no PlanStore configured)")
		return
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

	// Only pending_approval plans are eligible for /approve. Anything
	// past pending (applied / applied_failed / verify_failed / rejected)
	// has already been through a terminal transition on disk; a second
	// approve would re-provision a worktree and re-run apply+verify
	// wastefully while muddling the Status lifecycle. Clear the pointer
	// so the user can /mode plan a fresh one.
	if plan.Status != "" && plan.Status != types.PlanStatusPending {
		r.warn("approve refused: plan %s is in status %q, not %q. "+
			"Run /mode plan to generate a fresh plan.\n",
			plan.ID, plan.Status, types.PlanStatusPending)
		r.pendingPlanPath = ""
		return
	}

	// Probe both setters up-front. Running Mode=ModeApply against a
	// runner without SetMode would silently fall through to read
	// mode in the orchestrator; without SetPlanPath the apply phase
	// would error trying to load nil. Fail loud rather than dispatching.
	mSetter, mOK := r.runner.(modeSetter)
	pSetter, pOK := r.runner.(planPathSetter)
	if !mOK || !pOK {
		r.warn("/approve requires a runner with SetMode + SetPlanPath (stub runner detected)\n")
		return
	}

	title := approveTitlePrompt(r.language, plan.ID, len(plan.Changes))
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
		s, err := r.readInputLines()
		if err == nil {
			confirmed = strings.TrimSpace(strings.ToLower(s)) == "y"
		}
	}
	if !confirmed {
		r.info(approveCancelled(r.language))
		return
	}

	originalMode := r.currentMode
	mSetter.SetMode(types.ModeApply)
	pSetter.SetPlanPath(r.pendingPlanPath)
	defer func() {
		mSetter.SetMode(originalMode)
		pSetter.SetPlanPath("")
	}()

	request := fmt.Sprintf("/approve %s", plan.ID)
	logging.Info("[repl] dispatching approve: plan=%s path=%s", plan.ID, r.pendingPlanPath)

	if r.renderer != nil {
		r.renderer.StartSpinner()
	}
	busCtx, runErr := r.runner.Run(request, r.repoRoot, r.branch)
	if r.renderer != nil {
		r.renderer.StopSpinner()
	}
	if runErr != nil {
		logging.Error("[repl] approve failed: %v", runErr)
		r.errorf("approve: %v\n", runErr)
		// Drop pendingPlanPath so the user /mode plan's a fresh plan
		// rather than re-running a known-broken apply.
		r.pendingPlanPath = ""
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	logging.Info("[repl] approve result:\n%s", response)

	memResponse := response
	if busCtx != nil && busCtx.TaskState.LastError != "" {
		memResponse = "(approve ended in error — details omitted from memory)"
	}
	if response == "" || response == "(no result)" {
		fmt.Fprintln(r.out, "  ??")
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
	}
	// KindPlan classifies this turn distinctly from chitchat /
	// pipeline so future /history filters + the memory retrieval
	// policy can surface plan outcomes explicitly.
	r.recordTurn(request, request, memResponse, memory.KindPlan)

	// Apply is terminal. On success, the worktree has been discarded
	// (orchestrator defer); on partial failure, TaskState.LastError
	// already surfaced. Clear pendingPlanPath either way — replan if
	// you want to try again.
	r.pendingPlanPath = ""
}

// handleRejectCmd discards the pending ChangePlan without invoking
// the runner. Optional reason text trailing /reject is recorded in
// memory so the conversation's prior-turns block reflects why this
// plan was dropped.
//
// Behaviour mirrors /plan clear with two differences:
//  1. Accepts a free-form reason argument.
//  2. Records a memory turn so /history shows the rejection.
func (r *REPL) handleRejectCmd(line string) {
	if r.planStore == nil {
		r.info("/reject disabled (no PlanStore configured)")
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
			r.info("no log attached")
			return
		}
		r.attachedLog = ""
		r.attachedLogAutoRouted = false
		if setter, ok := r.runner.(attachedLogSetter); ok {
			setter.SetAttachedLog("")
		}
		r.success("attached log cleared")
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
			r.info("no hitrace attached")
			return
		}
		r.attachedHitrace = ""
		if setter, ok := r.runner.(attachedHitraceSetter); ok {
			setter.SetAttachedHitrace("")
		}
		r.success("attached hitrace cleared")
	case rest == "show":
		if r.attachedHitrace == "" {
			r.info("no hitrace attached")
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
	r.info("paste log, terminate with a lone /end line")
	scanner := r.captureScanner()
	var buf strings.Builder
	for {
		if r.interactive() {
			fmt.Fprint(r.out, "  log> ")
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
		r.info("no input captured; attached log unchanged")
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
	r.info("paste content, terminate with a lone /end line; press Enter to cancel an empty capture")
	scanner := r.captureScanner()
	var buf strings.Builder
	for {
		if r.interactive() {
			fmt.Fprint(r.out, "  paste> ")
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
		r.info("no input captured")
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
		r.info("no log attached")
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
