package types

import "strings"

// PathInsidePendingSubRepo reports whether file resolves into one of
// pendingRootRels. Exact RootRel match OR `rootRel + "/"` prefix
// matches, mirroring MultiRepoActiveSetGater.ResolveActiveSetPath's
// prefix walk (active_set_gate.go). pendingRootRels is a slice of
// RootRels as published on BusContext.PendingSubRepos.
//
// Single source of truth for the "is this path inside a sub-repo the
// routing fold left inactive for this question" decision. Consult
// before queueing forced-reads, projecting symbol hits, building
// required-file lists, or otherwise treating a path as actionable —
// the file-system tools refuse reads on the same set, so an
// unfiltered upstream produces work the explorer cannot satisfy.
func PathInsidePendingSubRepo(file string, pendingRootRels []string) bool {
	if file == "" || len(pendingRootRels) == 0 {
		return false
	}
	clean := strings.ReplaceAll(strings.TrimSpace(file), "\\", "/")
	if clean == "" {
		return false
	}
	for _, rootRel := range pendingRootRels {
		trimmed := strings.TrimSpace(rootRel)
		if trimmed == "" || trimmed == "." {
			continue
		}
		if clean == trimmed || strings.HasPrefix(clean, trimmed+"/") {
			return true
		}
	}
	return false
}
