package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_projection_v2p0_test.go — V2-P0 行级尺守卫 display pins
// (rank_order_v2_design_20260712.md §6.1 新裁定 A, GREENLIT 2026-07-12):
// count-additivity / composite-score rows leave the chain/◇ ordinal space and
// ride the ⌗ 口径旁栏 — rendered on their channel, caliber-worded, zero
// ordinal chip, zero badge, legend teaching entry riding along. The fixture
// records mirror the engine-actual emission (predicate/claim
// root_cause_caliber_side + tier/type typed notes — the
// traceQueryTypedObservations shape after the ordinal guard).

func v2p0CaliberRecords() []types.ObservationRecord {
	return []types.ObservationRecord{
		{
			ID:              "v2p0-wallclock-io",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			ClaimKey:        "root_cause_primary:io_latency",
			Subject:         "[GT]ColdPool#6-36644",
			Object:          "io_latency",
			Value:           "0.099",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 120, LineEnd: 121},
			RichNotes: []string{
				"tier=primary", "rank=1", "type=io_latency", "impact_ms=0.099",
				"cumulative_impact_ms=0.099", "effective_impact_ms=0.099",
				"chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain",
			},
			Confidence: 0.6,
		},
		{
			ID:              "v2p0-count-row",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_caliber_side",
			ClaimKey:        "root_cause_caliber_side",
			Subject:         "[GT]ColdPool#6-36644",
			Object:          "page_cache_churn",
			Value:           "0.600",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 100, LineEnd: 101},
			RichNotes: []string{
				"tier=caliber_side", "type=page_cache_churn", "impact_ms=0.600",
				"cumulative_impact_ms=0.600", "effective_impact_ms=0.600",
				"chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain",
			},
			Confidence: 0.6,
		},
		{
			ID:              "v2p0-composite-row",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_caliber_side",
			ClaimKey:        "root_cause_caliber_side",
			Subject:         "[GT]ColdPool#6-36644",
			Object:          "block_io_by_inode",
			Value:           "0.198",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 110, LineEnd: 111},
			RichNotes: []string{
				"tier=caliber_side", "type=block_io_by_inode", "impact_ms=0.198",
				"cumulative_impact_ms=0.198", "effective_impact_ms=0.198",
				"chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain",
			},
			Confidence: 0.6,
		},
	}
}

// TestV2P0CaliberSideRowsRenderWithoutOrdinals — the rendering obligation +
// the ordinal ban in one shape: both ⌗ rows stay visible on the ◇ channel
// with their caliber-class words, wear no 邻近影响# chip and no badge, and
// the wall-clock contender keeps the channel's ordinal space.
func TestV2P0CaliberSideRowsRenderWithoutOrdinals(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(v2p0CaliberRecords())
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// Rendering obligation: no silent-disappearance path.
	if !strings.Contains(fence, "页缓存抖动") || !strings.Contains(fence, "块设备IO(inode)") {
		t.Fatalf("caliber-side rows must keep their rendered channel seats:\n%s", fence)
	}
	if !strings.Contains(fence, "⌗口径旁栏·计数当量(非墙钟,不占序数)") {
		t.Fatalf("the count row must speak the ⌗ count-equivalent caliber word:\n%s", fence)
	}
	if !strings.Contains(fence, "⌗口径旁栏·复合分数(非墙钟,不占序数)") {
		t.Fatalf("the composite-score row must speak the ⌗ composite caliber word:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkCaliberSideRow) {
		t.Fatalf("the ⌗ legend teaching entry must ride the rows (词条-图例双向)")
	}
	// Ordinal ban: the caliber rows never wear a channel chip; the wall-clock
	// row owns the channel's ordinal space.
	for _, line := range strings.Split(fence, "\n") {
		if (strings.Contains(line, "页缓存抖动") || strings.Contains(line, "块设备IO(inode)")) &&
			strings.Contains(line, "邻近影响#") {
			t.Fatalf("a caliber-side row must not wear an ordinal chip: %q", line)
		}
	}
}

// TestV2P0CountFamilyClampedSeatDropsSumIdentity — F-7 (冷读 donghu E29,
// 2026-07-12): a count family whose published seat was CLAMPED below the
// member Σ (engine off-chain 0.35×window cap: Σ119.100 → 81.616, single
// member 84.300 > seat) must not claim the「= 计数合计」identity on any
// face — the honest 计数当量(超上限截断) word replaces it and the Σ stays on
// the 原始和 comparison face; an unclamped count family keeps the identity.
func TestV2P0CountFamilyClampedSeatDropsSumIdentity(t *testing.T) {
	clamped := types.TraceCausalProjectionNode{
		FamilyMemberCount: 2,
		FamilyFoldCaliber: "count_sum",
		FamilyMemberSumMS: 119.100,
		EffectiveImpactMS: 81.616,
		ImpactMS:          81.616,
	}
	word, mark, ok := runtimeTraceProjFamilyCaliberWord(clamped, true)
	if !ok || !strings.Contains(word, "计数当量(超上限截断") {
		t.Fatalf("the clamped seat must speak the truncation word, got %q ok=%v", word, ok)
	}
	if strings.Contains(word, "计数合计") || mark != runtimeTraceProjMarkFamilyCountEquivalent {
		t.Fatalf("the clamped seat must not claim the sum identity (word %q mark %v)", word, mark)
	}
	if prefix := runtimeTraceProjFamilyValuePrefix(clamped, true); prefix != "计数当量" {
		t.Fatalf("the 行1 stem must drop the 合计 claim too, got %q", prefix)
	}
	intact := clamped
	intact.EffectiveImpactMS = 119.100
	intact.ImpactMS = 119.100
	if word, _, _ := runtimeTraceProjFamilyCaliberWord(intact, true); !strings.Contains(word, "计数合计(共2项,同线程)") {
		t.Fatalf("an unclamped count family keeps the sum identity, got %q", word)
	}
	if prefix := runtimeTraceProjFamilyValuePrefix(intact, true); prefix != "合计" {
		t.Fatalf("unclamped 行1 stem keeps 合计, got %q", prefix)
	}
}

// TestV2P0CaliberSideWordClassFollowsRegistry — the word helper consumes the
// SHARED registry arm (count vs composite vs generic fallback), never a local
// token list.
func TestV2P0CaliberSideWordClassFollowsRegistry(t *testing.T) {
	count := types.TraceCausalProjectionNode{TypeToken: "blocked_reason"}
	if got := runtimeTraceProjCaliberSideWord(count, true); !strings.Contains(got, "计数当量") {
		t.Fatalf("blocked_reason (registry count additivity) must speak 计数当量, got %q", got)
	}
	composite := types.TraceCausalProjectionNode{TypeToken: "block_io_by_inode"}
	if got := runtimeTraceProjCaliberSideWord(composite, true); !strings.Contains(got, "复合分数") {
		t.Fatalf("block_io_by_inode (typed composite marker) must speak 复合分数, got %q", got)
	}
	unknown := types.TraceCausalProjectionNode{}
	if got := runtimeTraceProjCaliberSideWord(unknown, true); !strings.Contains(got, "⌗口径旁栏(") {
		t.Fatalf("a token-less caliber row keeps the generic form, got %q", got)
	}
	// zh-en 同词: the kernel caliber token 计数当量 renders byte-identically
	// on the EN face (RCM-2 family-word discipline).
	if got := runtimeTraceProjCaliberSideWord(count, false); !strings.Contains(got, "计数当量") || !strings.Contains(got, "count-equivalent") {
		t.Fatalf("EN face must speak the 计数当量 (count-equivalent) word pair, got %q", got)
	}
}
