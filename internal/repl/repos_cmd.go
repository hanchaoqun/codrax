package repl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/config"
	repomapindex "github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
)

// handleReposCmd dispatches /repos subcommands. Reachable from the
// REPL slash-handler when the canonical line is "/repos[ args]".
//
// Subcommands (design §4.3 Phase 2 commit 2):
//
//	/repos              list discovered sub-repos + active state
//	/repos focus <slug> session-pin a sub-repo into the routing set
//	/repos unfocus      release all pins (no arg) or a specific one
//	/repos refresh      force re-discover
//	/repos cap <N>      session-local override of multi_repo_max_active
//
// Disabled posture (multi_repo_enabled=false): the listing still
// works (informational) but is prefixed with a clear hint pointing at
// the yaml gate. Routing is bypassed regardless.
func (r *REPL) handleReposCmd(line string) {
	args := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "/repos"))
	fields := strings.Fields(args)

	if len(fields) == 0 {
		r.printReposList()
		return
	}

	switch strings.ToLower(fields[0]) {
	case "focus":
		if len(fields) < 2 {
			r.warn("/repos focus <slug-or-path> — sub-repo identifier required (run /repos to list; accepts the slug field OR the path column)")
			return
		}
		r.reposFocus(fields[1])
	case "unfocus":
		if len(fields) >= 2 {
			r.reposUnfocus(fields[1])
		} else {
			r.reposUnfocusAll()
		}
	case "refresh":
		r.reposRefresh()
	case "cap":
		if len(fields) < 2 {
			r.warn("/repos cap <N> — positive integer required (current cap = %d)", r.activeMultiRepoMaxActive())
			return
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n <= 0 {
			r.warn("/repos cap %q — must be a positive integer", fields[1])
			return
		}
		r.reposCap(n)
	default:
		r.warn("/repos: unknown subcommand %q (use focus / unfocus / refresh / cap)", fields[0])
	}
}

// printReposList renders a tabular view of the topology snapshot,
// the session-local focus + cap state, and the current LRU-resident
// (auto-active) sub-repo set. Phase 5 (2026-05-08) adds per-row
// status markers + ANSI color so the operator can tell at a glance
// which sub-repos are pinned, which the routing fold auto-selected
// for the most recent Run, and which are inactive.
//
// Three states (mutually exclusive):
//   - pinned (★ green)        — operator ran /repos focus
//   - auto-active (+ cyan)    — routing fold loaded into LRU
//   - inactive (· dark gray)  — neither pinned nor in LRU
//
// Non-TTY (pipe / scripted stdin) collapses to plain markers; the
// underlying r.info path bypasses pterm in that mode.
func (r *REPL) printReposList() {
	r.multiRepoMu.Lock()
	topo := r.topology
	focus := append([]string(nil), keysOf(r.multiRepoFocus)...)
	override := r.multiRepoMaxActiveOverride
	mg := r.topology
	_ = mg
	r.multiRepoMu.Unlock()
	sort.Strings(focus)

	if topo == nil {
		r.warn("/repos: no topology snapshot available (cmd/root.go discovery did not populate one — please file a bug)")
		return
	}

	// Auto-active = LRU-resident slugs that are NOT pinned. Pulled
	// from the MultiGraph carrier through the topology.* interface
	// the REPL already has via the orch wiring (avoids importing the
	// multigraph package directly here).
	autoActive := r.multiRepoLRUSnapshot()

	// Phase 6 (2026-05-08): when the LRU is empty (no Run has fired
	// yet so routing fold has not run), preview the slug set the
	// fold WOULD pick under fallback (focus pins + E-channel
	// biggest-first fill). Without this preview the listing shows
	// every non-pinned row as "inactive" pre-Run and the operator
	// has no way to anticipate what the first request will scan.
	//
	// Single-repo guard (2026-05-08 audit): the preview machinery
	// is multi-repo-only (multiRepoPreviewActiveSet returns nil
	// for IsSingle, the routing fold has nothing to pick from).
	// Skipping previewActive here means single-repo /repos output
	// keeps its existing 2-state (pinned / inactive) shape and the
	// trailer "no Run yet" advice does not bait the operator into
	// thinking there is a routing decision waiting to be made.
	previewSlugs := map[string]bool{}
	previewActive := false
	if len(autoActive) == 0 && !topo.IsSingle() {
		previewActive = true
		focusSlugs := keysOf(r.multiRepoFocus)
		for _, slug := range r.multiRepoPreviewActiveSet(focusSlugs) {
			previewSlugs[slug] = true
		}
	}

	cap := r.activeMultiRepoMaxActive()

	header := fmt.Sprintf("multi-repo topology — parent=%s slug=%s sub-repos=%d cap=%d",
		topo.ParentRoot, topo.ParentSlug, len(topo.Repos), cap)
	if !r.multiRepoEnabled {
		header += "  [routing disabled — set multi_repo_enabled=true in codrax.yaml to enable]"
	}
	r.info(header)

	if topo.IsSingle() {
		r.info("  (single-repo mode — parent itself is the only git repo discovered)")
	}

	if len(topo.Repos) == 0 {
		r.info("  (no sub-repos discovered)")
		return
	}

	colorEnabled := r.interactive()
	for _, sr := range topo.Repos {
		state := reposRowStateInactive
		switch {
		case r.isFocused(sr.Slug):
			state = reposRowStatePinned
		case autoActive[sr.Slug]:
			state = reposRowStateAutoActive
		case previewActive && previewSlugs[sr.Slug]:
			state = reposRowStatePreview
		}
		marker, label := reposRowDecor(state, colorEnabled)
		langs := strings.Join(sr.PrimaryLangs, ",")
		if langs == "" {
			langs = "-"
		}
		// Build the row body, then color it based on state.
		body := fmt.Sprintf("%-30s slug=%s git=%s files=%d langs=%s tier=%d",
			sr.RootRel, sr.Slug, displayGitMode(sr.GitMode), sr.FileCount, langs, sr.PrimaryLangsTier)
		if colorEnabled {
			body = reposRowColor(state, body)
		}
		r.info(fmt.Sprintf("  %s %s", marker, body))
		_ = label
	}

	// Legend trailer so the markers are self-explanatory.
	if previewActive {
		// Pre-Run path — explain that the `?` marker is a fallback
		// preview, not the actual active set, so the operator does
		// not assume scanning has started.
		if colorEnabled {
			r.info(fmt.Sprintf("  legend: %s pinned (`/repos focus`)  %s pre-run preview (fallback if no signal)  %s inactive",
				reposRowGlyphColored(reposRowStatePinned),
				reposRowGlyphColored(reposRowStatePreview),
				reposRowGlyphColored(reposRowStateInactive)))
		} else {
			r.info("  legend: ★ pinned (`/repos focus`)  ? pre-run preview (fallback)  · inactive")
		}
		// Trailer notice so the operator knows the preview is
		// tentative. Explicitly says routing fold runs PER question
		// and the actual active set may differ once a question is
		// asked (B/C/D channels then add their own picks).
		if isZh(r.language) {
			r.info("  注:尚未提问,? 标记是无问题信号兜底(focus pin + 文件最多者)的预览;首次提问后 /repos 会显示真实 active 集")
		} else {
			r.info("  note: no Run yet — `?` is a fallback preview (focus pins + biggest sub-repos); /repos after first request shows the real active set")
		}
	} else {
		if colorEnabled {
			r.info(fmt.Sprintf("  legend: %s pinned (`/repos focus`)  %s auto-active (routing fold)  %s inactive",
				reposRowGlyphColored(reposRowStatePinned),
				reposRowGlyphColored(reposRowStateAutoActive),
				reposRowGlyphColored(reposRowStateInactive)))
		} else {
			r.info("  legend: ★ pinned (`/repos focus`)  + auto-active (routing fold)  · inactive")
		}
	}

	if len(focus) > 0 {
		r.info(fmt.Sprintf("  pinned (focus): %s", strings.Join(focus, ", ")))
	}
	if override > 0 {
		r.info(fmt.Sprintf("  cap override (session): %d (yaml value: %d)", override, r.multiRepoMaxActive))
	}
}

// reposRowState classifies a sub-repo's display state in the /repos
// listing. Phase 5 (2026-05-08) introduced the inactive / auto-
// active / pinned trio; Phase 6 (2026-05-08) added the pre-Run
// preview state for sub-repos the routing fold WOULD pick under
// fallback when no question has been asked yet.
type reposRowState int

const (
	reposRowStateInactive reposRowState = iota
	reposRowStatePreview // pre-Run fallback preview (no LRU yet)
	reposRowStateAutoActive
	reposRowStatePinned
)

// reposRowDecor returns the marker glyph + (unused legend label).
// Color is applied only when the REPL is in interactive (TTY) mode
// — pipe / scripted callers see plain ASCII so log scraping stays
// stable.
func reposRowDecor(state reposRowState, colorEnabled bool) (marker, label string) {
	switch state {
	case reposRowStatePinned:
		if colorEnabled {
			return pterm.NewStyle(pterm.FgGreen, pterm.Bold).Sprint("★"), "pinned"
		}
		return "★", "pinned"
	case reposRowStateAutoActive:
		if colorEnabled {
			return pterm.NewStyle(pterm.FgCyan).Sprint("+"), "auto-active"
		}
		return "+", "auto-active"
	case reposRowStatePreview:
		// Yellow `?` to telegraph "tentative — will fire on the
		// next Run if no question-specific signal applies".
		if colorEnabled {
			return pterm.NewStyle(pterm.FgYellow).Sprint("?"), "preview"
		}
		return "?", "preview"
	default:
		if colorEnabled {
			return pterm.NewStyle(pterm.FgDarkGray).Sprint("·"), "inactive"
		}
		return "·", "inactive"
	}
}

// reposRowGlyphColored returns the colored marker glyph alone, used
// for the legend trailer when color is enabled.
func reposRowGlyphColored(state reposRowState) string {
	m, _ := reposRowDecor(state, true)
	return m
}

// reposRowColor wraps the row body in the state's color so the
// pinned line jumps off the page, the auto-active line reads as
// "currently in scope", and the inactive lines recede.
func reposRowColor(state reposRowState, body string) string {
	switch state {
	case reposRowStatePinned:
		return pterm.NewStyle(pterm.FgGreen, pterm.Bold).Sprint(body)
	case reposRowStateAutoActive:
		return pterm.NewStyle(pterm.FgCyan).Sprint(body)
	case reposRowStatePreview:
		return pterm.NewStyle(pterm.FgYellow).Sprint(body)
	default:
		return pterm.NewStyle(pterm.FgDarkGray).Sprint(body)
	}
}

// multiRepoLRUSnapshot returns the slug → bool map of currently
// LRU-resident sub-repos by reflecting MultiGraph.AllGraphs through
// a typed interface. The REPL's topology/multigraph wiring lives
// in cmd/root.go's onMultiRepoFocusChange / SetCap / SetFocus
// callbacks; we cannot reach the MultiGraph itself from this
// package without an import cycle, so the access goes through the
// types.* interface BusContext.MultiGraph already exposes.
//
// Falls back to empty map when no MultiGraph has been wired (single-
// repo / pre-multi-repo callers) — in that case the listing simply
// has no auto-active rows.
func (r *REPL) multiRepoLRUSnapshot() map[string]bool {
	if r == nil {
		return map[string]bool{}
	}
	snapper, ok := r.multigraphForListing.(interface{ ActiveSlugSnapshot() map[string]bool })
	if !ok || snapper == nil {
		return map[string]bool{}
	}
	return snapper.ActiveSlugSnapshot()
}

// multiRepoPreviewActiveSet returns the slug list the routing fold
// would pick under fallback (focus pins + E-channel biggest-first
// fill) if a Run started right now. Phase 6 (2026-05-08): used by
// /repos when the LRU is empty (pre-first-Run) so the operator can
// see which sub-repos would be auto-active before issuing a query.
//
// Reflects MultiGraph.PreviewActiveSet through a typed interface
// the same way multiRepoLRUSnapshot does (avoids the import cycle).
// Empty slice when MultiGraph is nil or single-repo.
func (r *REPL) multiRepoPreviewActiveSet(focusSlugs []string) []string {
	if r == nil {
		return nil
	}
	previewer, ok := r.multigraphForListing.(interface {
		PreviewActiveSet(focusSlugs []string) []string
	})
	if !ok || previewer == nil {
		return nil
	}
	return previewer.PreviewActiveSet(focusSlugs)
}

// reposFocus pins a sub-repo into the active routing set. The
// argument may be EITHER the sub-repo's Slug (the `slug=...` field
// shown by /repos) or its RootRel (the first column the listing
// renders, e.g. "repo-greet-go" or "repo-c/nested"). topology.Resolve
// tries Slug first then RootRel, so users can copy-paste whichever
// is more memorable.
func (r *REPL) reposFocus(token string) {
	r.multiRepoMu.Lock()
	var sr *topology.SubRepo
	if r.topology != nil {
		sr = r.topology.Resolve(token)
	}
	if sr == nil {
		r.multiRepoMu.Unlock()
		r.warn("/repos focus: no sub-repo with slug or path %q (run /repos to list)", token)
		return
	}
	r.multiRepoFocus[sr.Slug] = true
	pinned := keysOf(r.multiRepoFocus)
	cb := r.onMultiRepoFocusChange
	r.multiRepoMu.Unlock()
	if cb != nil {
		cb(pinned)
	}
	if sr.RootRel != token && sr.Slug != token {
		// User passed neither — defensive log, should not happen
		// because Resolve already matched on one of them. Guard
		// against future Resolve refactors that loosen matching.
		r.info(fmt.Sprintf("/repos focus: pinned %s [%s] (will stay active across turns until /repos unfocus)", sr.RootRel, sr.Slug))
		return
	}
	if sr.Slug == token {
		r.info(fmt.Sprintf("/repos focus: pinned %s (will stay active across turns until /repos unfocus)", sr.Slug))
		return
	}
	// User passed RootRel — echo both so the next /repos unfocus is unambiguous.
	r.info(fmt.Sprintf("/repos focus: pinned %s [slug=%s] (will stay active across turns until /repos unfocus)", sr.RootRel, sr.Slug))
}

// reposUnfocus releases a single pin. Like reposFocus, the argument
// can be a Slug or a RootRel. The pin is keyed by Slug internally,
// so a RootRel input is canonicalised to the matching Slug before
// the delete.
func (r *REPL) reposUnfocus(token string) {
	r.multiRepoMu.Lock()
	slug := token
	if r.topology != nil {
		if sr := r.topology.Resolve(token); sr != nil {
			slug = sr.Slug
		}
	}
	if !r.multiRepoFocus[slug] {
		r.multiRepoMu.Unlock()
		r.warn("/repos unfocus: %q is not pinned", token)
		return
	}
	delete(r.multiRepoFocus, slug)
	pinned := keysOf(r.multiRepoFocus)
	cb := r.onMultiRepoFocusChange
	r.multiRepoMu.Unlock()
	if cb != nil {
		cb(pinned)
	}
	r.info(fmt.Sprintf("/repos unfocus: released %s", slug))
}

func (r *REPL) reposUnfocusAll() {
	r.multiRepoMu.Lock()
	n := len(r.multiRepoFocus)
	r.multiRepoFocus = map[string]bool{}
	cb := r.onMultiRepoFocusChange
	r.multiRepoMu.Unlock()
	if cb != nil {
		cb(nil)
	}
	r.info(fmt.Sprintf("/repos unfocus: released %d pinned slug(s)", n))
}

func (r *REPL) reposRefresh() {
	r.multiRepoMu.Lock()
	prev := r.topology
	parentRoot := r.repoRoot
	anchor := r.runtimeAnchor
	r.multiRepoMu.Unlock()

	if parentRoot == "" {
		r.warn("/repos refresh: REPL has no RepoRoot configured")
		return
	}

	// Force a full BFS by skipping LoadOrDiscover (we always want
	// the latest disk state for an explicit refresh).
	parentSlug := repomapindex.CacheDirSlug(parentRoot)
	opts := topology.Options{Depth: 4, MinFiles: 1, RuntimeAnchor: anchor}
	if prev != nil && prev.ParentSlug != "" {
		// Preserve the slug we previously had so Save lands at the
		// same on-disk file (defensive; CacheDirSlug is deterministic
		// so this should be a no-op, but it guards against drift).
		_ = parentSlug
	}
	fresh, err := topology.Discover(parentRoot, opts)
	if err != nil {
		r.warn("/repos refresh: discover failed: %v", err)
		return
	}
	if fresh == nil {
		r.warn("/repos refresh: empty result")
		return
	}
	if anchor != "" {
		_ = fresh.Save(anchor)
	}

	r.multiRepoMu.Lock()
	r.topology = fresh
	// Clean stale focus pins (slugs that no longer correspond to a
	// discovered sub-repo).
	for slug := range r.multiRepoFocus {
		if fresh.SubRepoBySlug(slug) == nil {
			delete(r.multiRepoFocus, slug)
		}
	}
	cb := r.onMultiRepoRefresh
	r.multiRepoMu.Unlock()
	if cb != nil {
		cb(fresh)
	}

	r.info(fmt.Sprintf("/repos refresh: re-discovered %d sub-repo(s) under %s", len(fresh.Repos), parentRoot))
	r.printReposList()
}

func (r *REPL) reposCap(n int) {
	clamped := config.ClampMultiRepoMaxActive(n)
	r.multiRepoMu.Lock()
	r.multiRepoMaxActiveOverride = clamped
	cb := r.onMultiRepoCapChange
	r.multiRepoMu.Unlock()
	if cb != nil {
		cb(clamped)
	}
	if clamped != n {
		// Tell the user we clamped — surprise prevention.
		r.info(fmt.Sprintf(
			"/repos cap: requested %d clamped to %d (hard ceiling %d). yaml multi_repo_max_active=%d",
			n, clamped, config.MultiRepoMaxActiveCeiling, r.multiRepoMaxActive))
		return
	}
	r.info(fmt.Sprintf("/repos cap: session-local override set to %d (yaml multi_repo_max_active=%d)", clamped, r.multiRepoMaxActive))
}

// === Read accessors used by Phase 4 routing fold (read-only) ===

// MultiRepoFocusSnapshot returns a copy of the session-pinned slug
// set. Safe to call from any goroutine; the returned map is owned
// by the caller.
func (r *REPL) MultiRepoFocusSnapshot() map[string]bool {
	r.multiRepoMu.Lock()
	defer r.multiRepoMu.Unlock()
	out := make(map[string]bool, len(r.multiRepoFocus))
	for k, v := range r.multiRepoFocus {
		out[k] = v
	}
	return out
}

// MultiRepoActiveCap returns the effective LRU cap (session override
// if set, else the Config value). Phase 4 consumes this when sizing
// MultiGraph.
func (r *REPL) MultiRepoActiveCap() int {
	r.multiRepoMu.Lock()
	defer r.multiRepoMu.Unlock()
	return r.activeMultiRepoMaxActiveLocked(r.multiRepoMaxActiveOverride)
}

// Topology returns the current topology snapshot pointer (may be nil).
// Snapshot is treated as immutable; refresh swaps the pointer.
func (r *REPL) Topology() *topology.RepoTopology {
	r.multiRepoMu.Lock()
	defer r.multiRepoMu.Unlock()
	return r.topology
}

// === Helpers ===

func (r *REPL) activeMultiRepoMaxActive() int {
	r.multiRepoMu.Lock()
	defer r.multiRepoMu.Unlock()
	return r.activeMultiRepoMaxActiveLocked(r.multiRepoMaxActiveOverride)
}

// activeMultiRepoMaxActiveLocked must be called with multiRepoMu held.
//
// Returns the effective LRU cap, picking — in priority order — the
// session-local override (`/repos cap N`), the yaml value, then the
// shared default. All three flow through ClampMultiRepoMaxActive at
// their respective set sites, so this getter never returns a value
// above the hard ceiling.
func (r *REPL) activeMultiRepoMaxActiveLocked(override int) int {
	if override > 0 {
		return override
	}
	if r.multiRepoMaxActive > 0 {
		return r.multiRepoMaxActive
	}
	return config.MultiRepoMaxActiveDefault
}

func (r *REPL) isFocused(slug string) bool {
	r.multiRepoMu.Lock()
	defer r.multiRepoMu.Unlock()
	return r.multiRepoFocus[slug]
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// focusMapFromSlugs constructs the REPL's session-focus map from a
// caller-supplied slug slice. Used by Provider.InitialFocusSlugs
// (--focus CLI flag) so the REPL boots with the operator's startup
// pins already in place: /repos lists them as ★ pinned, the prompt
// sticky tag carries [focus:X], and the routing fold's A channel
// honours them on every Run. Nil / empty input returns an empty
// non-nil map so the REPL's pin-set lookups stay nil-safe.
func focusMapFromSlugs(slugs []string) map[string]bool {
	out := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out[s] = true
	}
	return out
}

func displayGitMode(mode string) string {
	if mode == "" {
		return "-"
	}
	return mode
}
