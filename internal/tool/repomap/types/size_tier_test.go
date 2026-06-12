package types

import "testing"

func TestRepoSizeTierBoundaries(t *testing.T) {
	cases := []struct {
		files int
		want  SizeTier
	}{
		{0, SizeTierTiny}, {149, SizeTierTiny},
		{150, SizeTierSmall}, {499, SizeTierSmall},
		{500, SizeTierMedium}, {4999, SizeTierMedium},
		{5000, SizeTierLarge}, {14999, SizeTierLarge},
		{15000, SizeTierVeryLarge}, {1000000, SizeTierVeryLarge},
	}
	for _, c := range cases {
		if got := RepoSizeTier(c.files); got != c.want {
			t.Errorf("RepoSizeTier(%d) = %v, want %v", c.files, got, c.want)
		}
	}
}

// Monotonic invariant: a larger tier never gets a smaller default
// than a smaller tier (CodeGraph's empirically validated rule — a
// bigger repo must never see LESS than a smaller one).
func TestDefaultTopNMonotonicAcrossTiers(t *testing.T) {
	for view, table := range viewTierDefaults {
		for tier := SizeTierTiny; tier < SizeTierVeryLarge; tier++ {
			if table[tier] > table[tier+1] {
				t.Errorf("%s: tier %v default %d > tier %v default %d — monotonic invariant broken",
					view, tier, table[tier], tier+1, table[tier+1])
			}
		}
	}
}

// Medium+ tiers must keep the historical defaults — the eval bar runs
// against codrax-sized (medium) repos and may not shift.
func TestDefaultTopNMediumKeepsHistoricalDefaults(t *testing.T) {
	for _, c := range []struct {
		view string
		want int
	}{{"task_map", 20}, {"relation_map", 40}, {"source_inventory", 60}} {
		for _, tier := range []SizeTier{SizeTierMedium, SizeTierLarge, SizeTierVeryLarge} {
			if got := DefaultTopN(c.view, tier); got != c.want {
				t.Errorf("DefaultTopN(%s, %v) = %d, want historical %d", c.view, tier, got, c.want)
			}
		}
	}
}

func TestDefaultTopNUntieredViewsReturnZero(t *testing.T) {
	for _, view := range []string{"overview", "file_map", "semantic_subgraph", "call_path", "edit_impact"} {
		if got := DefaultTopN(view, SizeTierTiny); got != 0 {
			t.Errorf("DefaultTopN(%s) = %d, want 0 (untiered — caller keeps historical default)", view, got)
		}
	}
}

func TestGraphSizeTierFallsBackToFilesSlice(t *testing.T) {
	g := &Graph{Files: make([]*FileInfo, 200)}
	if got := GraphSizeTier(g); got != SizeTierSmall {
		t.Errorf("GraphSizeTier(nil FileIndex, 200 Files) = %v, want small", got)
	}
	if got := GraphSizeTier(nil); got != SizeTierMedium {
		t.Errorf("GraphSizeTier(nil) = %v, want medium (neutral)", got)
	}
}
