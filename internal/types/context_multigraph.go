package types

// SubRepoSnapshot is the BusContext-level snapshot of one
// topology.SubRepo. Mirrors the fields read by non-multigraph-aware
// consumers (REPL renderer, telemetry, write-mode fail-loud paths)
// so they don't need to import the topology package or own a
// *MultiGraph reference.
//
// Field semantics match topology.SubRepo verbatim. Defined here so
// types/ stays free of import cycles; the orchestrator copies fields
// across at Run entry.
type SubRepoSnapshot struct {
	Slug             string   `json:"slug"`
	RootAbs          string   `json:"root_abs"`
	RootRel          string   `json:"root_rel"`
	GitMode          string   `json:"git_mode"`
	PrimaryLangs     []string `json:"primary_langs,omitempty"`
	PrimaryLangsTier int      `json:"primary_langs_tier,omitempty"`
	FileCount        int      `json:"file_count,omitempty"`
}

// IsSingleRepoSnapshot reports the degenerate case (one entry whose
// RootRel == "."). Used by consumers that fast-path single-repo
// behaviour without owning a *MultiGraph reference.
func IsSingleRepoSnapshot(subs []SubRepoSnapshot) bool {
	return len(subs) == 1 && subs[0].RootRel == "."
}

// SubRepoSnapshotByRelPath returns the SubRepoSnapshot owning
// relPathFromParent (longest matching RootRel prefix; parent-root
// catch-all "." used only on no-other-match). Returns nil on miss.
//
// Mirrors topology.RepoTopology.Lookup so consumers that only have
// the snapshot list can still answer "which sub-repo owns this
// path?" without owning a *MultiGraph or *RepoTopology reference.
func SubRepoSnapshotByRelPath(subs []SubRepoSnapshot, relPathFromParent string) *SubRepoSnapshot {
	rel := relPathFromParent
	for len(rel) > 0 && rel[0] == '/' {
		rel = rel[1:]
	}
	if len(rel) >= 2 && rel[0] == '.' && rel[1] == '/' {
		rel = rel[2:]
	}

	if rel == "" || rel == "." {
		for i := range subs {
			if subs[i].RootRel == "." {
				return &subs[i]
			}
		}
		return nil
	}

	var (
		best       *SubRepoSnapshot
		bestLen    = -1
		parentSelf *SubRepoSnapshot
	)
	for i := range subs {
		rr := subs[i].RootRel
		if rr == "." {
			parentSelf = &subs[i]
			continue
		}
		if rel == rr || (len(rel) > len(rr) && rel[:len(rr)] == rr && rel[len(rr)] == '/') {
			if len(rr) > bestLen {
				best = &subs[i]
				bestLen = len(rr)
			}
		}
	}
	if best != nil {
		return best
	}
	return parentSelf
}
