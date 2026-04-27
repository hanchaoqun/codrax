package memory

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPolicyFor_Defaults locks the legacy hardcoded values now that
// they live behind types.DefaultMemoryKindPolicies. Any future
// adjustment to these defaults must update both this test AND the
// types-package source — keeping the two in sync is the contract.
func TestPolicyFor_Defaults(t *testing.T) {
	ms := types.ResolvedMemorySettings(types.MemorySettings{})
	cases := []struct {
		k                  Kind
		wantSessionPin     int
		wantRecentBody     int
		wantCompactedCap   int
		wantEntityMul      int
		wantRefsDepth      int
	}{
		{KindChitchat, 3, 1200, 3, 2, 1},
		{KindShell, 3, 1200, 3, 2, 1},
		{KindPipeline, 2, 0, 5, 3, 2},
		{KindPlan, 0, 0, -1, 2, 0},
		{Kind(""), 0, 0, -1, 2, 0},
	}
	for _, tc := range cases {
		t.Run(string(tc.k), func(t *testing.T) {
			p := policyFor(tc.k, ms)
			if p.sessionPinCount != tc.wantSessionPin {
				t.Errorf("sessionPinCount = %d, want %d", p.sessionPinCount, tc.wantSessionPin)
			}
			if p.recentBodyChars != tc.wantRecentBody {
				t.Errorf("recentBodyChars = %d, want %d", p.recentBodyChars, tc.wantRecentBody)
			}
			if p.compactedMatchCap != tc.wantCompactedCap {
				t.Errorf("compactedMatchCap = %d, want %d", p.compactedMatchCap, tc.wantCompactedCap)
			}
			if p.entityScoreMul != tc.wantEntityMul {
				t.Errorf("entityScoreMul = %d, want %d", p.entityScoreMul, tc.wantEntityMul)
			}
			if p.refsChainDepth != tc.wantRefsDepth {
				t.Errorf("refsChainDepth = %d, want %d", p.refsChainDepth, tc.wantRefsDepth)
			}
			if p.entityMinRunes != 3 {
				t.Errorf("entityMinRunes default = %d, want 3", p.entityMinRunes)
			}
			if p.sessionTieBreakerBonus != 1 {
				t.Errorf("sessionTieBreakerBonus default = %d, want 1", p.sessionTieBreakerBonus)
			}
		})
	}
}

// TestPolicyFor_OverridesPerKind verifies the per-Kind override path.
// Operator supplies a non-nil sub-policy for one Kind; the override
// fields replace defaults; other Kinds are unaffected.
func TestPolicyFor_OverridesPerKind(t *testing.T) {
	ms := types.ResolvedMemorySettings(types.MemorySettings{
		Policy: &types.MemoryRetrievalPolicy{
			Pipeline: &types.MemoryKindPolicy{
				SessionPinCount:   7,
				CompactedMatchCap: 12,
				// RecentBodyChars / EntityScoreMul / RefsChainDepth left
				// at zero — must keep the corresponding pipeline default.
			},
		},
	})
	pipe := policyFor(KindPipeline, ms)
	if pipe.sessionPinCount != 7 {
		t.Errorf("override sessionPinCount = %d, want 7", pipe.sessionPinCount)
	}
	if pipe.compactedMatchCap != 12 {
		t.Errorf("override compactedMatchCap = %d, want 12", pipe.compactedMatchCap)
	}
	if pipe.entityScoreMul != 3 {
		t.Errorf("zero-field in override should keep default 3 entityScoreMul; got %d", pipe.entityScoreMul)
	}
	if pipe.recentBodyChars != 0 {
		t.Errorf("zero-field in override should keep default 0 recentBodyChars; got %d", pipe.recentBodyChars)
	}

	chitchat := policyFor(KindChitchat, ms)
	if chitchat.sessionPinCount != 3 {
		t.Errorf("Chitchat must be unaffected by Pipeline override; sessionPinCount = %d, want 3", chitchat.sessionPinCount)
	}
}

// TestPolicyFor_GlobalKnobs covers EntityMinRunes + SessionTieBreakerBonus
// from MemorySettings (vs per-Kind). These were the two scoreIndex-level
// magic numbers (line 1005 / 1019 pre-refactor).
func TestPolicyFor_GlobalKnobs(t *testing.T) {
	ms := types.ResolvedMemorySettings(types.MemorySettings{
		EntityMinRunes:         5,
		SessionTieBreakerBonus: 4,
	})
	p := policyFor(KindPipeline, ms)
	if p.entityMinRunes != 5 {
		t.Errorf("entityMinRunes = %d, want 5", p.entityMinRunes)
	}
	if p.sessionTieBreakerBonus != 4 {
		t.Errorf("sessionTieBreakerBonus = %d, want 4", p.sessionTieBreakerBonus)
	}
	// Any Kind sees the same global tunables.
	pc := policyFor(KindChitchat, ms)
	if pc.entityMinRunes != 5 {
		t.Errorf("entityMinRunes carries across Kinds; chitchat got %d, want 5", pc.entityMinRunes)
	}
}

// TestScoreIndex_RespectsEntityMinRunes verifies that the previously-
// hardcoded `len([]rune(le)) < 3` literal is now driven by config.
// With EntityMinRunes=5, a 3-rune entity should NOT match anymore.
func TestScoreIndex_RespectsEntityMinRunes(t *testing.T) {
	idx := []IndexEntry{{
		ID:       "e1",
		Topic:    "topic",
		Summary:  "summary",
		Entities: []string{"abc"}, // 3 runes — would match under old default
	}}
	// Default (entityMinRunes=3): "abc" inside lowercased request hits.
	defaults := types.ResolvedMemorySettings(types.MemorySettings{})
	defaultPolicy := policyFor(KindPipeline, defaults)
	out := scoreIndex(idx, "see abc here", BuildOpts{Kind: KindPipeline}, defaultPolicy)
	if len(out) != 1 {
		t.Errorf("default min-3-runes should match `abc`; got %d hits", len(out))
	}
	// Override (entityMinRunes=5): 3-rune entity now silently dropped.
	tight := types.ResolvedMemorySettings(types.MemorySettings{EntityMinRunes: 5})
	tightPolicy := policyFor(KindPipeline, tight)
	out = scoreIndex(idx, "see abc here", BuildOpts{Kind: KindPipeline}, tightPolicy)
	if len(out) != 0 {
		t.Errorf("min-5-runes should drop 3-rune `abc`; got %d hits", len(out))
	}
}

// TestScoreIndex_RespectsSessionTieBreakerBonus verifies the
// configurable bonus replaces the hardcoded `s++`.
func TestScoreIndex_RespectsSessionTieBreakerBonus(t *testing.T) {
	idx := []IndexEntry{
		{
			ID:        "e_other",
			Topic:     "topic",
			Summary:   "summary",
			Keywords:  []string{"foo"},
			SessionID: "other",
		},
		{
			ID:        "e_same",
			Topic:     "topic",
			Summary:   "summary",
			Keywords:  []string{"foo"},
			SessionID: "current",
		},
	}
	// With bonus=10, the same-session entry should land first by a
	// wide margin even though both score 1 on the keyword hit.
	ms := types.ResolvedMemorySettings(types.MemorySettings{SessionTieBreakerBonus: 10})
	policy := policyFor(KindPipeline, ms)
	got := scoreIndex(idx, "foo", BuildOpts{Kind: KindPipeline, SessionID: "current"}, policy)
	if len(got) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(got))
	}
	if got[0].ID != "e_same" {
		t.Errorf("with bonus=10, same-session entry should rank first; got %q", got[0].ID)
	}
}
