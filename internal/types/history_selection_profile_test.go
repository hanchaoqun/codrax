package types

import "testing"

func TestBuildVCSHistorySelectionAuthoritySelectsLatestMergeFromWiderContext(t *testing.T) {
	rm := &RequestModel{
		Predicates: SemanticPredicates{IsHistoryLookup: true},
		HistorySelectionProfile: &HistorySelectionProfile{
			Mode:     HistorySelectionLatestOne,
			ItemKind: HistorySelectionItemMerge,
			Count:    1,
		},
	}
	results := []ToolResult{{
		ToolName: "git_log",
		Success:  true,
		VCSHistory: &ToolVCSHistory{
			Kind:        ToolVCSHistoryKindGitLog,
			Commits:     []string{"newest", "older"},
			QueryOrder:  "recent",
			QueryLimit:  5,
			MergesOnly:  true,
			FirstParent: true,
		},
	}}
	authority, ok := BuildVCSHistorySelectionAuthority(rm, results)
	if !ok {
		t.Fatal("latest merge should bind to compatible ordered git_log result")
	}
	if len(authority.SelectedCommits) != 1 || authority.SelectedCommits[0] != "newest" {
		t.Fatalf("selected commits=%v, want first typed row", authority.SelectedCommits)
	}
	if authority.QueryLimit != 5 || !authority.MergesOnly || !authority.FirstParent || !authority.Complete {
		t.Fatalf("query authority lost typed parameters: %+v", authority)
	}
}

func TestBuildVCSHistorySelectionAuthorityRejectsIncompatibleUniverse(t *testing.T) {
	rm := &RequestModel{
		Predicates: SemanticPredicates{IsHistoryLookup: true},
		HistorySelectionProfile: &HistorySelectionProfile{
			Mode:     HistorySelectionLatestOne,
			ItemKind: HistorySelectionItemMerge,
		},
	}
	results := []ToolResult{{
		ToolName: "git_log",
		Success:  true,
		Summary:  "merge newest appears only in rendered prose",
		VCSHistory: &ToolVCSHistory{
			Kind:       ToolVCSHistoryKindGitLog,
			Commits:    []string{"ordinary-commit"},
			QueryOrder: "recent",
			QueryLimit: 1,
		},
	}}
	if authority, ok := BuildVCSHistorySelectionAuthority(rm, results); ok {
		t.Fatalf("merge request must not bind an unfiltered commit stream: %+v", authority)
	}
}

func TestBuildVCSHistorySelectionAuthorityRecentNChoosesSmallestSufficientTypedWindow(t *testing.T) {
	rm := &RequestModel{
		Predicates: SemanticPredicates{IsHistoryLookup: true},
		HistorySelectionProfile: &HistorySelectionProfile{
			Mode:     HistorySelectionRecentN,
			ItemKind: HistorySelectionItemCommit,
			Count:    2,
		},
	}
	results := []ToolResult{
		{ToolName: "git_log", Success: true, VCSHistory: &ToolVCSHistory{Kind: ToolVCSHistoryKindGitLog, Commits: []string{"a", "b", "c"}, QueryOrder: "recent", QueryLimit: 10}},
		{ToolName: "git_log", Success: true, VCSHistory: &ToolVCSHistory{Kind: ToolVCSHistoryKindGitLog, Commits: []string{"a", "b"}, QueryOrder: "recent", QueryLimit: 2}},
	}
	authority, ok := BuildVCSHistorySelectionAuthority(rm, results)
	if !ok || authority.QueryLimit != 2 || len(authority.SelectedCommits) != 2 {
		t.Fatalf("recent-N authority=%+v ok=%t", authority, ok)
	}
}
