package multigraph

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	"github.com/hanchaoqun/codrax/internal/types"
)

// active_set_gate.go — Phase 1.L1 (2026-05-08 multi-repo UX).
//
// Implementation note: the public API uses types.ActiveSetGateResult
// + types.MultiRepoActiveSetGater so file-system tools (read_file /
// grep / list_files / repo_map) call through the leaf-package
// types.* interface and never need to import the multigraph package
// directly (avoids a cycle: tool→multigraph→topology→tool). The
// MultiGraph receiver below satisfies the interface; tool code does
// `mg, _ := ctx.MultiGraph.(types.MultiRepoActiveSetGater)`.
//
// File-system tools (read_file, grep, repo_map) accept LLM-supplied
// paths. In multi-repo posture the parent dir is NOT a single
// indexed workspace — it is a container of independent sub-repos —
// and the routing layer has chosen a small "active" set per Run.
// Without an L1 gate the LLM could:
//  1. Pass `path=""` or `"."` and trigger a full-parent scan, which
//     on a 50+ sub-repo workspace cripples the Run.
//  2. Pass an explicit path inside a sub-repo currently outside the
//     active set, polluting the answer with content the user did
//     not focus.
//  3. Pass a bare relative path (no sub-repo prefix) and have the
//     tool quietly read the wrong sub-repo, or fail with a confusing
//     "file not found" against the parent dir.
//
// ResolveActiveSetPath answers all three: it pins every tool call
// to an active sub-repo, auto-prefixes unprefixed bare paths when
// there is exactly one match, and surfaces a generic refusal prose
// listing the active set whenever the path falls outside it.
//
// Single-repo bypass: when ctx.MultiGraph is nil OR the topology
// has fewer than two sub-repos, the gate returns the LLM-supplied
// path untouched (single-repo callers stay byte-identical).
//
// Lives in package multigraph (rather than repomap) so the
// internal/tool package can import the helper without an import
// cycle: repomap/tool.go imports internal/tool, but multigraph/
// is leaf-only.

// ResolveActiveSetPath is the *MultiGraph receiver method that
// satisfies types.MultiRepoActiveSetGater. Tool code reaches this
// method through the typed interface — see the active_set_gate.go
// implementation note above.
//
// Standalone form (for tests / multigraph-internal callers) is
// ResolveActiveSetPathStandalone below.
//
// llmPath is the raw value the LLM supplied. The helper handles:
//   - empty / "." → multi-repo refusal (parent-wide scan disallowed)
//   - absolute path → reject if it falls outside active sub-repo
//     RootAbs prefixes
//   - sub-repo-prefixed path (e.g. "repo-greet-go/svc/x.go") → if
//     the sub-repo is active, allow; if inactive, refuse
//   - bare relative path → walk active sub-repos in alphabetical
//     order; allow only when exactly one sub-repo contains the
//     resolved file/dir (caller controls "exists" probe via
//     fileExists callback). Multi-match → refuse with
//     disambiguation.
//
// fileExists may be nil; the caller passes a probe (typically
// os.Stat) when path-existence semantics matter (read_file). For
// repo_map / grep — where the path is a directory and per-sub-repo
// auto-prefix unique-match would require a heavier check — the
// caller passes nil and the gate treats every active sub-repo as
// a candidate; the bare-path multi-active-sub-repo case is then
// refused as ambiguous, forcing the LLM to specify a prefix.
func (m *MultiGraph) ResolveActiveSetPath(
	ctx *types.BusContext,
	toolName string,
	llmPath string,
	fileExists func(absPath string) bool,
) types.ActiveSetGateResult {
	// Single-repo / no-multigraph bypass. m == nil should not happen
	// since the interface is reached through ctx.MultiGraph cast, but
	// keep the guard defensive.
	if m == nil || m.IsSingle() {
		return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
	}
	topo := m.Topology()
	if topo == nil || len(topo.Repos) < 2 {
		return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
	}

	// Active set = topology - PendingSubRepos (RootRel match).
	pendingSet := make(map[string]bool, len(ctx.PendingSubRepos))
	for _, p := range ctx.PendingSubRepos {
		pendingSet[p] = true
	}
	var active []topology.SubRepo
	for _, sr := range topo.Repos {
		if !pendingSet[sr.RootRel] {
			active = append(active, sr)
		}
	}
	if len(active) == 0 {
		// Defensive: every sub-repo flagged inactive. The orchestrator
		// would not normally produce this, but if it does, refuse all
		// tool calls explicitly rather than guess.
		return types.ActiveSetGateResult{
			Allowed:      false,
			RefusalProse: refusalNoActive(toolName),
		}
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].RootRel < active[j].RootRel
	})

	// Empty / "." → parent-wide scan, never allowed in multi-repo.
	cleaned := strings.TrimSpace(llmPath)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "" || cleaned == "." {
		return types.ActiveSetGateResult{
			Allowed:      false,
			RefusalProse: refusalParentWide(toolName, active),
		}
	}

	// Absolute path → match against each sub-repo's RootAbs.
	if filepath.IsAbs(cleaned) {
		for i := range active {
			rootAbs := active[i].RootAbs
			if rootAbs == "" {
				continue
			}
			if cleaned == rootAbs || strings.HasPrefix(cleaned, rootAbs+string(filepath.Separator)) {
				return types.ActiveSetGateResult{
					Allowed:      true,
					ResolvedPath: cleaned,
					SubRepoRootRel: active[i].RootRel,
				}
			}
		}
		// Absolute path outside every active RootAbs.
		return types.ActiveSetGateResult{
			Allowed:      false,
			RefusalProse: refusalOutsideActive(toolName, cleaned, active),
		}
	}

	// Sub-repo-prefixed relative path?
	prefixedSlash := strings.ReplaceAll(cleaned, "\\", "/")
	for i := range active {
		rel := active[i].RootRel
		if rel == "" || rel == "." {
			continue
		}
		if prefixedSlash == rel || strings.HasPrefix(prefixedSlash, rel+"/") {
			return types.ActiveSetGateResult{
				Allowed:      true,
				ResolvedPath: cleaned,
				SubRepoRootRel: active[i].RootRel,
			}
		}
	}

	// Maybe it is prefixed by an INACTIVE sub-repo?
	for i := range topo.Repos {
		sr := topo.Repos[i]
		if !pendingSet[sr.RootRel] {
			continue
		}
		if sr.RootRel == "" || sr.RootRel == "." {
			continue
		}
		if prefixedSlash == sr.RootRel || strings.HasPrefix(prefixedSlash, sr.RootRel+"/") {
			return types.ActiveSetGateResult{
				Allowed:      false,
				RefusalProse: refusalInactivePrefix(toolName, sr.RootRel, active),
			}
		}
	}

	// Bare relative path. Auto-prefix walk: try each active sub-repo;
	// when fileExists is non-nil count matches and require unique;
	// when nil treat every active sub-repo as a candidate (ambiguous
	// → refuse, forcing the LLM to specify a prefix).
	type candidate struct {
		sr       *topology.SubRepo
		resolved string
	}
	var matches []candidate
	for i := range active {
		rel := active[i].RootRel
		var resolved string
		if rel == "" || rel == "." {
			resolved = cleaned
		} else {
			resolved = rel + "/" + cleaned
		}
		if fileExists == nil {
			matches = append(matches, candidate{sr: &active[i], resolved: resolved})
			continue
		}
		if active[i].RootAbs == "" {
			continue
		}
		probe := filepath.Join(active[i].RootAbs, cleaned)
		if fileExists(probe) {
			matches = append(matches, candidate{sr: &active[i], resolved: resolved})
		}
	}
	switch len(matches) {
	case 0:
		return types.ActiveSetGateResult{
			Allowed:      false,
			RefusalProse: refusalNoMatch(toolName, cleaned, active),
		}
	case 1:
		return types.ActiveSetGateResult{
			Allowed:        true,
			ResolvedPath:   matches[0].resolved,
			SubRepoRootRel: matches[0].sr.RootRel,
			AutoPrefixed:   matches[0].resolved != cleaned,
		}
	default:
		candidatePaths := make([]string, 0, len(matches))
		for _, m := range matches {
			candidatePaths = append(candidatePaths, m.resolved)
		}
		return types.ActiveSetGateResult{
			Allowed:      false,
			RefusalProse: refusalAmbiguous(toolName, cleaned, candidatePaths),
		}
	}
}

func refusalParentWide(toolName string, active []topology.SubRepo) string {
	return fmt.Sprintf(
		"%s refused: an empty / `.` path would scan the workspace's parent directory, "+
			"which spans multiple independent sub-repos. Pass a path inside one of the "+
			"currently active sub-repos: %s. If your investigation truly needs more "+
			"breadth, surface that requirement in your final answer in plain prose so "+
			"the user can adjust the workspace scope and re-ask.",
		toolName, formatActiveList(active))
}

func refusalNoActive(toolName string) string {
	return fmt.Sprintf(
		"%s refused: the workspace currently has no active sub-repos. Surface this in "+
			"your final answer in plain prose so the user can adjust the workspace scope.",
		toolName)
}

func refusalOutsideActive(toolName, path string, active []topology.SubRepo) string {
	return fmt.Sprintf(
		"%s refused: the path %q falls outside every active sub-repo. The active sub-repos "+
			"are: %s. If your investigation needs to consult a sub-repo currently outside "+
			"the active set, surface that requirement in plain prose at the top of your "+
			"final answer — the user will adjust the workspace scope and re-ask.",
		toolName, path, formatActiveList(active))
}

func refusalInactivePrefix(toolName, inactiveRel string, active []topology.SubRepo) string {
	return fmt.Sprintf(
		"%s refused: the path is inside %q, which is currently outside the active sub-repo "+
			"set. The active sub-repos are: %s. If your investigation needs that sub-repo, "+
			"surface the requirement in plain prose at the top of your final answer — the "+
			"user will adjust the workspace scope and re-ask.",
		toolName, inactiveRel, formatActiveList(active))
}

func refusalNoMatch(toolName, path string, active []topology.SubRepo) string {
	return fmt.Sprintf(
		"%s refused: the path %q was not found inside any active sub-repo. The active "+
			"sub-repos are: %s. Either prefix the path with the sub-repo it lives in (e.g. "+
			"`<sub-repo>/%s`), or, if the path belongs to an inactive sub-repo, surface "+
			"that requirement in plain prose at the top of your final answer.",
		toolName, path, formatActiveList(active), path)
}

func refusalAmbiguous(toolName, path string, candidates []string) string {
	return fmt.Sprintf(
		"%s refused: the bare path %q could resolve into multiple active sub-repos: %s. "+
			"Re-issue the call with the sub-repo prefix that matches the file you want.",
		toolName, path, strings.Join(candidates, ", "))
}

func formatActiveList(active []topology.SubRepo) string {
	if len(active) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(active))
	for _, sr := range active {
		parts = append(parts, sr.RootRel)
	}
	return strings.Join(parts, ", ")
}
