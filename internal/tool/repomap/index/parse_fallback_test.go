package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestTierDiscount pins the tier-discount curve — changes here
// ripple into rank outcomes, so we lock the constants.
func TestTierDiscount(t *testing.T) {
	cases := []struct {
		tier int
		want float64
	}{
		{0, 1.0},
		{1, 1.0},
		{2, 0.85},
		{3, 0.6},
		{4, 0.3},
		{99, 0.3},
	}
	for _, c := range cases {
		if got := TierDiscount(c.tier); got != c.want {
			t.Errorf("TierDiscount(%d): want %v, got %v", c.tier, c.want, got)
		}
	}
}

// TestFallbackBannerThresholds pins the per-language thresholds —
// ArkTS 0.4, Cangjie 0.5. Drift here changes when the "grammar is
// stale" banner fires, so lock it.
func TestFallbackBannerThresholds(t *testing.T) {
	if fallbackBannerThreshold[types.LangArkTS] != 0.4 {
		t.Errorf("ArkTS fallback banner threshold drifted; got %v",
			fallbackBannerThreshold[types.LangArkTS])
	}
	if fallbackBannerThreshold[types.LangCangjie] != 0.5 {
		t.Errorf("Cangjie fallback banner threshold drifted; got %v",
			fallbackBannerThreshold[types.LangCangjie])
	}
}

// TestApplyHarmonyOSPostProcess_CjoDeny enforces red line
// L-Cangjie-1: .cjo compiled artefacts never enter repomap.
func TestApplyHarmonyOSPostProcess_CjoDeny(t *testing.T) {
	entries := []FileEntry{
		{RelPath: "src/main.cj", Language: types.LangCangjie},
		{RelPath: "target/output.cjo", Language: ""},
		{RelPath: "src/helper.cj", Language: types.LangCangjie},
	}
	got := applyHarmonyOSPostProcess(".", entries)
	for _, e := range got {
		if e.RelPath == "target/output.cjo" {
			t.Errorf("red line L-Cangjie-1 violated: .cjo passed through %+v", got)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 surviving entries; got %d: %+v", len(got), got)
	}
}

// TestApplyHarmonyOSPostProcess_CacheDeny confirms the
// .cangjie-cache dir tree is rejected wholesale — the build cache
// often holds a lot of source-like content we do not want to index.
func TestApplyHarmonyOSPostProcess_CacheDeny(t *testing.T) {
	entries := []FileEntry{
		{RelPath: ".cangjie-cache/deps/foo.cj", Language: types.LangCangjie},
		{RelPath: "src/a/.cangjie-cache/b.cj", Language: types.LangCangjie},
		{RelPath: "src/real.cj", Language: types.LangCangjie},
	}
	got := applyHarmonyOSPostProcess(".", entries)
	if len(got) != 1 || got[0].RelPath != "src/real.cj" {
		t.Errorf("expected only src/real.cj to survive; got %+v", got)
	}
}
