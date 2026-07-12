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
	maxTag, ok := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[0].Node, true, model.TreeRows[0].CoverageFragmentSecondary)
	if !ok || maxTag.Text != "窗内 runnable 合计 20.342ms,链上仅覆盖其中最大片段 7.843ms(39%)" {
		t.Fatalf("the max row keeps the witness wording verbatim, got %q", maxTag.Text)
	}
	secTag, ok := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[1].Node, true, model.TreeRows[1].CoverageFragmentSecondary)
	if !ok || secTag.Text != "窗内 runnable 合计 20.342ms,本行覆盖其中另一片段 6.754ms(33%)" {
		t.Fatalf("the sibling must speak the honest 另一片段 form, got %q", secTag.Text)
	}
	if strings.Contains(secTag.Text, "最大片段") {
		t.Fatalf("mutual exclusion must die: %q", secTag.Text)
	}
	// EN faces follow the same rank.
	if en, _ := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[1].Node, false, true); !strings.Contains(en.Text, "another fragment of it") ||
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
	tag, ok := runtimeTraceProjFullWindowCoverageTag(model.TreeRows[0].Node, true, model.TreeRows[0].CoverageFragmentSecondary)
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
