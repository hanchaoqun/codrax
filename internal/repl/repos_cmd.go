package repl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

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

// printReposList renders a tabular view of the topology snapshot and
// the session-local focus + cap state.
func (r *REPL) printReposList() {
	r.multiRepoMu.Lock()
	topo := r.topology
	focus := append([]string(nil), keysOf(r.multiRepoFocus)...)
	override := r.multiRepoMaxActiveOverride
	r.multiRepoMu.Unlock()
	sort.Strings(focus)

	if topo == nil {
		r.warn("/repos: no topology snapshot available (cmd/root.go discovery did not populate one — please file a bug)")
		return
	}

	cap := r.activeMultiRepoMaxActiveLocked(override)

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

	for _, sr := range topo.Repos {
		marker := " "
		if r.isFocused(sr.Slug) {
			marker = "*"
		}
		langs := strings.Join(sr.PrimaryLangs, ",")
		if langs == "" {
			langs = "-"
		}
		r.info(fmt.Sprintf("  %s %-30s slug=%s git=%s files=%d langs=%s tier=%d",
			marker, sr.RootRel, sr.Slug, displayGitMode(sr.GitMode), sr.FileCount, langs, sr.PrimaryLangsTier))
	}

	if len(focus) > 0 {
		r.info(fmt.Sprintf("  pinned (focus): %s", strings.Join(focus, ", ")))
	}
	if override > 0 {
		r.info(fmt.Sprintf("  cap override (session): %d (yaml value: %d)", override, r.multiRepoMaxActive))
	}
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

func displayGitMode(mode string) string {
	if mode == "" {
		return "-"
	}
	return mode
}
