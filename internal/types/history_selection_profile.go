package types

// HistorySelectionMode declares which positions in an ordered repository-
// history result the current request asks the answer to treat as principal.
// It is analyzer-authored request shape, not a conclusion about any commit.
type HistorySelectionMode string

const (
	HistorySelectionNotApplicable HistorySelectionMode = "not_applicable"
	HistorySelectionLatestOne     HistorySelectionMode = "latest_one"
	HistorySelectionEarliestOne   HistorySelectionMode = "earliest_one"
	HistorySelectionRecentN       HistorySelectionMode = "recent_n"
	HistorySelectionOldestN       HistorySelectionMode = "oldest_n"
	HistorySelectionBoundedRange  HistorySelectionMode = "bounded_range"
	HistorySelectionUnspecified   HistorySelectionMode = "unspecified"
)

func AllHistorySelectionModes() []HistorySelectionMode {
	return []HistorySelectionMode{
		HistorySelectionNotApplicable,
		HistorySelectionLatestOne,
		HistorySelectionEarliestOne,
		HistorySelectionRecentN,
		HistorySelectionOldestN,
		HistorySelectionBoundedRange,
		HistorySelectionUnspecified,
	}
}

func (m HistorySelectionMode) IsValid() bool {
	for _, candidate := range AllHistorySelectionModes() {
		if m == candidate {
			return true
		}
	}
	return false
}

// HistorySelectionItemKind distinguishes the history universe requested by the
// user from the order inside that universe. The value is matched only against
// typed VCS tool parameters; commit subjects and answer prose are never read.
type HistorySelectionItemKind string

const (
	HistorySelectionItemNotApplicable HistorySelectionItemKind = "not_applicable"
	HistorySelectionItemCommit        HistorySelectionItemKind = "commit"
	HistorySelectionItemMerge         HistorySelectionItemKind = "merge"
	HistorySelectionItemNonMerge      HistorySelectionItemKind = "non_merge"
	HistorySelectionItemMatching      HistorySelectionItemKind = "matching_commit"
	HistorySelectionItemUnspecified   HistorySelectionItemKind = "unspecified"
)

func AllHistorySelectionItemKinds() []HistorySelectionItemKind {
	return []HistorySelectionItemKind{
		HistorySelectionItemNotApplicable,
		HistorySelectionItemCommit,
		HistorySelectionItemMerge,
		HistorySelectionItemNonMerge,
		HistorySelectionItemMatching,
		HistorySelectionItemUnspecified,
	}
}

func (k HistorySelectionItemKind) IsValid() bool {
	for _, candidate := range AllHistorySelectionItemKinds() {
		if k == candidate {
			return true
		}
	}
	return false
}

// HistorySelectionProfile carries request-side selection semantics. SourceQuote
// is validated at emit time but consumers use only the closed enums and Count.
// This keeps model classification in the soft-guidance lane.
type HistorySelectionProfile struct {
	Mode        HistorySelectionMode     `json:"mode"`
	ItemKind    HistorySelectionItemKind `json:"item_kind"`
	Count       int                      `json:"count,omitempty"`
	SourceQuote string                   `json:"source_quote,omitempty"`
	Confidence  float64                  `json:"confidence,omitempty"`
	Rationale   string                   `json:"rationale,omitempty"`
}

func (p *HistorySelectionProfile) Active() bool {
	return p != nil && p.Mode != HistorySelectionNotApplicable && p.Mode != HistorySelectionUnspecified
}

func (p *HistorySelectionProfile) PrincipalCount() int {
	if p == nil {
		return 0
	}
	switch p.Mode {
	case HistorySelectionLatestOne, HistorySelectionEarliestOne:
		return 1
	case HistorySelectionRecentN, HistorySelectionOldestN:
		if p.Count > 0 {
			return p.Count
		}
	}
	return 0
}

// VCSHistorySelectionAuthority is the conjunction of request-side selection
// semantics and one compatible ordered tool result. It is safe prompt context:
// no raw request, commit subject, summary text, or answer prose participates.
type VCSHistorySelectionAuthority struct {
	Mode            HistorySelectionMode
	ItemKind        HistorySelectionItemKind
	SelectedCommits []string
	QueryOrder      string
	QueryLimit      int
	MergesOnly      bool
	NoMerges        bool
	FirstParent     bool
	Complete        bool
	Reason          string
}

// BuildVCSHistorySelectionAuthority returns a principal selection only when a
// typed request profile and a compatible typed git-log result agree on order
// and item universe. Ambiguous/unsupported shapes fail open to no authority.
func BuildVCSHistorySelectionAuthority(rm *RequestModel, results []ToolResult) (VCSHistorySelectionAuthority, bool) {
	if rm == nil || !rm.Predicates.IsHistoryLookup || rm.HistorySelectionProfile == nil {
		return VCSHistorySelectionAuthority{}, false
	}
	profile := rm.HistorySelectionProfile
	want := profile.PrincipalCount()
	if want <= 0 {
		return VCSHistorySelectionAuthority{}, false
	}
	wantOrder := "recent"
	if profile.Mode == HistorySelectionEarliestOne || profile.Mode == HistorySelectionOldestN {
		wantOrder = "oldest"
	}
	bestIndex := -1
	bestLimit := int(^uint(0) >> 1)
	for i := range results {
		result := results[i]
		history := result.VCSHistory
		if !result.Success || history == nil || history.Kind != ToolVCSHistoryKindGitLog || len(history.Commits) < want {
			continue
		}
		order := history.QueryOrder
		if order == "" {
			order = "recent"
		}
		if order != wantOrder || !historySelectionToolUniverseMatches(profile.ItemKind, *history) {
			continue
		}
		limit := history.QueryLimit
		if limit <= 0 {
			limit = len(history.Commits)
		}
		if limit < want || limit >= bestLimit {
			continue
		}
		bestIndex = i
		bestLimit = limit
	}
	if bestIndex < 0 {
		return VCSHistorySelectionAuthority{}, false
	}
	history := results[bestIndex].VCSHistory
	selected := append([]string(nil), history.Commits[:want]...)
	queryOrder := history.QueryOrder
	if queryOrder == "" {
		queryOrder = "recent"
	}
	return VCSHistorySelectionAuthority{
		Mode:            profile.Mode,
		ItemKind:        profile.ItemKind,
		SelectedCommits: selected,
		QueryOrder:      queryOrder,
		QueryLimit:      history.QueryLimit,
		MergesOnly:      history.MergesOnly,
		NoMerges:        history.NoMerges,
		FirstParent:     history.FirstParent,
		Complete:        len(selected) == want,
		Reason:          "request_selection_and_ordered_tool_result_agree",
	}, true
}

func historySelectionToolUniverseMatches(kind HistorySelectionItemKind, history ToolVCSHistory) bool {
	switch kind {
	case HistorySelectionItemMerge:
		return history.MergesOnly && !history.NoMerges
	case HistorySelectionItemNonMerge:
		return history.NoMerges && !history.MergesOnly
	case HistorySelectionItemCommit:
		return !history.MergesOnly && !history.NoMerges
	default:
		return false
	}
}
