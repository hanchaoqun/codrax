package tool

// answer_document_projection_coverage_fragment_test.go — CR-3 修复轮追加件
// pins (2026-07-12; 56643 witness: NetworkService-60595 的两条链上 runnable
// 行 7.843/6.754 — 两次独立发生,合法分行 — 从属披露都铸「窗内 runnable 合计
// 20.342ms,链上仅覆盖其中最大片段 X」:第二行(6.754)宣称为假,且同页两行
// 互斥). Among rows sharing (subject, state class, full-window total), only
// the TRUE max keeps 最大片段; siblings speak 另一片段. Single-row groups
// stay byte-identical.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func coverageFragmentNode(covered float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Subject:                   "NetworkService-60595",
		StateKind:                 "runnable",
		ImpactMS:                  covered,
		FullWindowStateMS:         20.342,
		FullWindowStateSource:     "window_stats",
		FullWindowStateSameWindow: true,
	}
}

// TestCoverageFragmentDualChainRows — the witness shape verbatim: the max
// row keeps the 最大片段 word, the smaller sibling is reworded, and the
// mutual exclusion is gone.
func TestCoverageFragmentDualChainRows(t *testing.T) {
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
		{Node: coverageFragmentNode(7.843)},
		{Node: coverageFragmentNode(6.754)},
	}}
	runtimeTraceProjStampCoverageFragmentRank(&model)
	if model.TreeRows[0].CoverageFragmentSecondary {
		t.Fatalf("the true max row must keep primary rank")
	}
	if !model.TreeRows[1].CoverageFragmentSecondary {
		t.Fatalf("the smaller sibling must be stamped secondary")
	}
	maxTag, ok := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[0].Node, true, model.TreeRows[0].CoverageFragmentSecondary, 0)
	if !ok || maxTag.Text != "窗内 runnable 合计 20.342ms,链上仅覆盖其中最大片段 7.843ms(39%)" {
		t.Fatalf("the max row keeps the witness wording verbatim, got %q", maxTag.Text)
	}
	secTag, ok := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[1].Node, true, model.TreeRows[1].CoverageFragmentSecondary, 0)
	if !ok || secTag.Text != "窗内 runnable 合计 20.342ms,本行覆盖其中另一片段 6.754ms(33%)" {
		t.Fatalf("the sibling must speak the honest 另一片段 form, got %q", secTag.Text)
	}
	if strings.Contains(secTag.Text, "最大片段") {
		t.Fatalf("mutual exclusion must die: %q", secTag.Text)
	}
	// EN faces follow the same rank.
	if en, _ := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[1].Node, false, true, 0); !strings.Contains(en.Text, "another fragment of it") ||
		strings.Contains(en.Text, "largest fragment") {
		t.Fatalf("EN sibling wording drifted: %q", en.Text)
	}
}

// TestCoverageFragmentSingleRowByteIdentical — the single-row control: no
// sibling → primary rank → the pre-fix bytes exactly.
func TestCoverageFragmentSingleRowByteIdentical(t *testing.T) {
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
		{Node: coverageFragmentNode(14.597)},
	}}
	runtimeTraceProjStampCoverageFragmentRank(&model)
	if model.TreeRows[0].CoverageFragmentSecondary {
		t.Fatalf("a single-row group is its own max")
	}
	tag, ok := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[0].Node, true, model.TreeRows[0].CoverageFragmentSecondary, 0)
	if !ok || tag.Text != "窗内 runnable 合计 20.342ms,链上仅覆盖其中最大片段 14.597ms(72%)" {
		t.Fatalf("single-row wording must stay byte-identical, got %q", tag.Text)
	}
}

// TestCoverageFragmentGroupBoundaries — different thread, different state
// class, or different full total = different groups; each keeps its own
// max word (never cross-demoted). Ties keep the word on every tied row.
func TestCoverageFragmentGroupBoundaries(t *testing.T) {
	other := coverageFragmentNode(3.0)
	other.Subject = "CookieMonsterCl-59843"
	sleepNode := coverageFragmentNode(2.0)
	sleepNode.StateKind = "sleep"
	tieA, tieB := coverageFragmentNode(5.0), coverageFragmentNode(5.0)
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
		{Node: coverageFragmentNode(7.843)},
		{Node: other}, {Node: sleepNode}, {Node: tieA}, {Node: tieB},
	}}
	runtimeTraceProjStampCoverageFragmentRank(&model)
	if model.TreeRows[1].CoverageFragmentSecondary || model.TreeRows[2].CoverageFragmentSecondary {
		t.Fatalf("cross-thread / cross-class rows are separate groups")
	}
	// Same group ties at 5.0 sit below the 7.843 max → both secondary.
	if !model.TreeRows[3].CoverageFragmentSecondary || !model.TreeRows[4].CoverageFragmentSecondary {
		t.Fatalf("same-group smaller ties demote together")
	}
}

// TestCoverageFragmentSingleMemberRowJoinsGroup — WO-T1 (SMR-1 批 SMR-S14 E16
// 变体, smr_audit_report §②, 2026-07-12; 56643 witness): a SINGLE-member row
// (3.344, unmerged) shares the (subject, state class, full total 25.847) group
// with the ×3 merged row (19.933) — it must join the A group and demote (its
// pre-fix face claimed 「最大片段 3.344ms(13%)」= 假宣称, three rows on one
// page each self-crowned "largest").
func TestCoverageFragmentSingleMemberRowJoinsGroup(t *testing.T) {
	merged := coverageFragmentNode(19.933)
	merged.Subject = "CookieMonsterCl-59843"
	merged.FullWindowStateMS = 25.847
	merged.MergedCount = 3
	merged.MergedMinMS = 3.344
	merged.MergedMaxMS = 8.307
	single := coverageFragmentNode(3.344)
	single.Subject = "CookieMonsterCl-59843"
	single.FullWindowStateMS = 25.847
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
		{Node: merged}, {Node: single},
	}}
	runtimeTraceProjStampCoverageFragmentRank(&model)
	if model.TreeRows[0].CoverageFragmentSecondary {
		t.Fatalf("the group-max (×3 merged, 19.933) keeps primary rank")
	}
	if !model.TreeRows[1].CoverageFragmentSecondary {
		t.Fatalf("the single-member row (3.344) must join the A group and demote")
	}
	tag, ok := runtimeTraceProjFullWindowCoverageTag(single, true, true, 0)
	if !ok || strings.Contains(tag.Text, "最大片段") {
		t.Fatalf("E16 假宣称 must die: %q", tag.Text)
	}
}

// TestCoverageFragmentMergedRowSpeaksTotalWord — WO-A1 词面统一 (SMR-S14 残余):
// an ×N merged row's covered value is a member SUM — 「最大片段」 on it is a
// false single-fragment claim; the merged form speaks 「链上覆盖合计(共N次)」.
// Unmerged rows keep the pinned bytes (asserted by the tests above).
func TestCoverageFragmentMergedRowSpeaksTotalWord(t *testing.T) {
	merged := coverageFragmentNode(19.933)
	merged.FullWindowStateMS = 25.847
	merged.MergedCount = 3
	tag, ok := runtimeTraceProjFullWindowCoverageTag(merged, true, false, 0)
	if !ok || tag.Text != "窗内 runnable 合计 25.847ms,链上覆盖合计(共3次) 19.933ms(77%)" {
		t.Fatalf("merged rows must speak the n=N total word (片段=假单段词), got %q", tag.Text)
	}
	if en, _ := runtimeTraceProjFullWindowCoverageTag(merged, false, false, 0); strings.Contains(en.Text, "largest fragment") ||
		!strings.Contains(en.Text, "n=3 total") {
		t.Fatalf("EN merged wording drifted: %q", en.Text)
	}
	// The secondary merged form keeps both truths (another slice + n=N total).
	if sec, _ := runtimeTraceProjFullWindowCoverageTag(merged, true, true, 0); strings.Contains(sec.Text, "最大片段") ||
		!strings.Contains(sec.Text, "合计(共3次)") {
		t.Fatalf("secondary merged wording drifted: %q", sec.Text)
	}
}


// TestCoverageFragmentEngineTotalTwinSpeaksTotalWord — 96717 复放追修 pin
// (E12/E15 形): an UNMERGED rank row whose covered value is the µs-identical
// display of its same-span ×3 merged twin (or a same-identity occurrence
// series' additive total) is an engine-side n=N total — 「最大片段」 on it is
// the same false single-fragment claim, so it speaks 链上覆盖合计(×N).
func TestCoverageFragmentEngineTotalTwinSpeaksTotalWord(t *testing.T) {
	tag, ok := runtimeTraceProjFullWindowCoverageTag(coverageFragmentNode(19.933), true, false, 3)
	if !ok || !strings.Contains(tag.Text, "链上覆盖合计(共3次) 19.933ms") {
		t.Fatalf("engine-total twin must speak the n=N total word, got %q", tag.Text)
	}
	// Zero twin count keeps the pinned single-fragment bytes.
	tag, ok = runtimeTraceProjFullWindowCoverageTag(coverageFragmentNode(14.597), true, false, 0)
	if !ok || !strings.Contains(tag.Text, "链上仅覆盖其中最大片段 14.597ms") {
		t.Fatalf("plain rows keep the pinned wording, got %q", tag.Text)
	}
}
