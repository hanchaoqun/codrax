package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// R2.1 — V2 self-consistency reviewer tests (post_shape_residual_audit.md
// 2026-05-04). The V1 dispatch was deleted at B8-T4; the helper
// (shouldReviewConsistencyV2 / renderConsistencyReviewBodyV2 /
// runSelfConsistencyReviewV2) restore the same commit-62 contract
// against the V2 carrier.

// TestShouldReviewConsistencyV2_NeedsSummaryAndBody pins the floor:
// >= 100 char summary + >= 3 body items. Below either floor → skip.
func TestShouldReviewConsistencyV2_NeedsSummaryAndBody(t *testing.T) {
	mkSummary := func(n int) string { return strings.Repeat("x", n) }

	cases := []struct {
		name string
		doc  *types.AnswerDocumentV2
		want bool
	}{
		{
			name: "happy path: 200-char summary + 5 body items",
			doc: &types.AnswerDocumentV2{
				Blocks: []types.AnswerBlock{
					{Kind: types.BlockSummary, Text: mkSummary(200)},
					{Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
						{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}, {Label: "e"},
					}},
				},
			},
			want: true,
		},
		{
			name: "summary < 100 → skip",
			doc: &types.AnswerDocumentV2{
				Blocks: []types.AnswerBlock{
					{Kind: types.BlockSummary, Text: mkSummary(50)},
					{Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
						{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"},
					}},
				},
			},
			want: false,
		},
		{
			name: "summary OK, body items 2 → skip (no section either)",
			doc: &types.AnswerDocumentV2{
				Blocks: []types.AnswerBlock{
					{Kind: types.BlockSummary, Text: mkSummary(150)},
					{Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
						{Label: "a"}, {Label: "b"},
					}},
				},
			},
			want: false,
		},
		{
			name: "summary OK + section block (counts as body)",
			doc: &types.AnswerDocumentV2{
				Blocks: []types.AnswerBlock{
					{Kind: types.BlockSummary, Text: mkSummary(150)},
					{Kind: types.BlockSection, Title: "Detail", Text: "longer body prose"},
				},
			},
			want: true,
		},
		{
			name: "scalar / decision sole content → skip",
			doc: &types.AnswerDocumentV2{
				Blocks: []types.AnswerBlock{
					{Kind: types.BlockSummary, Text: mkSummary(150)},
					{Kind: types.BlockScalar, Text: "42"},
				},
			},
			want: false,
		},
		{
			name: "nil doc → skip",
			doc:  nil,
			want: false,
		},
		{
			name: "empty blocks → skip",
			doc:  &types.AnswerDocumentV2{},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldReviewConsistencyV2(tc.doc)
			if got != tc.want {
				t.Errorf("shouldReviewConsistencyV2 = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRenderConsistencyReviewBodyV2_BlockKinds confirms each body-
// shaped block kind contributes to the rendered text and the
// summary / diagram blocks do NOT (they're either the AnswerSummary
// input or non-prose).
func TestRenderConsistencyReviewBodyV2_BlockKinds(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, Text: "summary text"},
			{Kind: types.BlockOrderedList, Title: "Steps", Items: []types.AnswerBlockItem{
				{Label: "Step1", Text: "do A"},
				{Label: "Step2", Text: "do B"},
			}},
			{Kind: types.BlockBulletList, Items: []types.AnswerBlockItem{
				{Label: "category"},
			}},
			{Kind: types.BlockTable, Text: "| Layer | Result |\n|---|---|\n| CLI | absent |"},
			{Kind: types.BlockSection, Title: "Detail", Text: "section body"},
			{Kind: types.BlockScalar, Text: "42"},
			{Kind: types.BlockDecision, Text: "yes"},
			{Kind: types.BlockCaveat, Text: "scope warning"},
			{Kind: types.BlockDiagram, Text: "graph TD"},
		},
	}
	got := renderConsistencyReviewBodyV2(doc)

	// Body-shape blocks render
	for _, want := range []string{
		"## Steps",
		"1. Step1 — do A",
		"2. Step2 — do B",
		"- category",
		"| Layer | Result |",
		"| CLI | absent |",
		"## Detail",
		"section body",
		"Scalar: 42",
		"Decision: yes",
		"[caveat] scope warning",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing %q in render:\n%s", want, got)
		}
	}
	// Summary + diagram do NOT
	for _, banned := range []string{"summary text", "graph TD"} {
		if strings.Contains(got, banned) {
			t.Errorf("body unexpectedly contains %q:\n%s", banned, got)
		}
	}
}

func TestRenderConsistencyReviewBodyV2_IncludesStructuredVisibleSurface(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, Text: "summary text"},
			{
				Kind:    types.BlockTable,
				Title:   "系统按已验证证据补充成员：Kind 常量（1）",
				Columns: []string{"符号名称", "定义位置", "说明"},
				Items: []types.AnswerBlockItem{{
					Label: "KindSymbolPresent",
					Text:  "符号存在性判定",
					Cells: []string{"internal/analysis/criterion/grammar.go:29", "read-mode：检查符号是否存在"},
				}},
			},
		},
	}
	got := renderConsistencyReviewBodyV2(doc)
	for _, want := range []string{
		"## 系统按已验证证据补充成员：Kind 常量（1）",
		"Columns: 符号名称 | 定义位置 | 说明",
		"KindSymbolPresent — 符号存在性判定",
		"internal/analysis/criterion/grammar.go:29",
		"read-mode：检查符号是否存在",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("review body must include final visible surface %q:\n%s", want, got)
		}
	}
}

// TestRenderConsistencyReviewBodyV2_NilSafe confirms graceful nil.
func TestRenderConsistencyReviewBodyV2_NilSafe(t *testing.T) {
	if got := renderConsistencyReviewBodyV2(nil); got != "" {
		t.Errorf("nil doc must yield empty body; got %q", got)
	}
}

// TestItemBodyText covers Label / Text combinations.
func TestItemBodyText(t *testing.T) {
	cases := []struct {
		in   types.AnswerBlockItem
		want string
	}{
		{types.AnswerBlockItem{Label: "L", Text: "T"}, "L — T"},
		{types.AnswerBlockItem{Label: "L"}, "L"},
		{types.AnswerBlockItem{Text: "T"}, "T"},
		{types.AnswerBlockItem{Label: "L", Text: "T", Cells: []string{"C1", "C2"}}, "L — T — C1 | C2"},
		{types.AnswerBlockItem{Cells: []string{"C1", "", "C2"}}, "C1 | C2"},
		{types.AnswerBlockItem{}, ""},
	}
	for _, tc := range cases {
		if got := itemBodyText(tc.in); got != tc.want {
			t.Errorf("itemBodyText(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestClampReasoningForRepair pins the 200-rune cap + newline
// flattening contract.
func TestClampReasoningForRepair(t *testing.T) {
	if got := clampReasoningForRepair("multi\nline\rtext"); got != "multi line text" {
		t.Errorf("newline flattening failed: %q", got)
	}
	long := strings.Repeat("我", 250)
	got := clampReasoningForRepair(long)
	gotRunes := []rune(got)
	if len(gotRunes) != 201 || gotRunes[200] != '…' {
		t.Errorf("rune cap broken; got len=%d last=%q", len(gotRunes), string(gotRunes[len(gotRunes)-1]))
	}
}

// TestBuildSelfContradictionRepair_OrdinalAndContent confirms the
// repair prose names both verbatim claims + reviewer reasoning.
func TestBuildSelfContradictionRepair_OrdinalAndContent(t *testing.T) {
	c := SelfConsistencyContradiction{
		Topic:        "step count",
		SummaryClaim: "9 steps",
		BodyClaim:    "5 steps",
	}
	got := buildSelfContradictionRepair(c, "scope mismatch", 1, 2)
	for _, want := range []string{
		"[1/2]",
		"step count",
		"9 steps",
		"5 steps",
		"Reviewer reasoning: scope mismatch",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("repair missing %q in:\n%s", want, got)
		}
	}
}

func TestFilterDeterministicRowOrderContradictions_UsesTypedKindOnly(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "packages",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{Label: "aggregator"},
				{Label: "amplifier"},
				{Label: "axis"},
			},
		}},
	}
	contradictions := []SelfConsistencyContradiction{
		{
			Kind:         SelfConsistencyContradictionRowOrder,
			Topic:        "package order",
			SummaryClaim: "sorted by package name",
			BodyClaim:    "aggregator, amplifier, axis",
		},
		{
			Kind:         SelfConsistencyContradictionUnknown,
			Topic:        "alphabetic package order",
			SummaryClaim: "sorted by package name",
			BodyClaim:    "aggregator, amplifier, axis",
		},
	}
	got, suppressed := filterDeterministicRowOrderContradictions(doc, contradictions)
	if suppressed != 1 {
		t.Fatalf("suppressed = %d, want 1", suppressed)
	}
	if len(got) != 1 || got[0].Kind != SelfConsistencyContradictionUnknown {
		t.Fatalf("expected only unknown-kind contradiction to remain, got %+v", got)
	}
}

func TestFilterDeterministicRowOrderContradictions_DoesNotSuppressUnsortedRows(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "packages",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{Label: "axis"},
				{Label: "aggregator"},
				{Label: "amplifier"},
			},
		}},
	}
	contradictions := []SelfConsistencyContradiction{{
		Kind:         SelfConsistencyContradictionRowOrder,
		Topic:        "package order",
		SummaryClaim: "sorted by package name",
		BodyClaim:    "axis, aggregator, amplifier",
	}}
	got, suppressed := filterDeterministicRowOrderContradictions(doc, contradictions)
	if suppressed != 0 {
		t.Fatalf("suppressed = %d, want 0", suppressed)
	}
	if len(got) != 1 {
		t.Fatalf("expected contradiction to remain, got %+v", got)
	}
}

func TestFilterVCSHistoryRowOrderContradictions_SuppressesGitLogBackedOrder(t *testing.T) {
	mut := types.NewMutableState("最近 3 次提交都做了什么")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "git_log",
		Success:  true,
		Summary: strings.Join([]string{
			"[git_log: count=3 evidence_origin=vcs_metadata]",
			"commit ae1dd6b256fab219104c09447b6ffe3697239b7a",
			"commit 3ae8465b6afe3fb16902d511d51482fefd09a103",
			"commit 125687ab6f1ff7cd1187183fc459efe65be10fb3",
		}, "\n"),
	}}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "commits",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{Label: "ae1dd6b"},
				{Label: "3ae8465"},
				{Label: "125687a"},
			},
		}},
	}
	contradictions := []SelfConsistencyContradiction{
		{
			Kind:         SelfConsistencyContradictionRowOrder,
			Topic:        "commit order",
			SummaryClaim: "按时间倒序排列",
			BodyClaim:    "ae1dd6b, 3ae8465, 125687a",
		},
		{
			Kind:         SelfConsistencyContradictionNumeric,
			Topic:        "count",
			SummaryClaim: "3 commits",
			BodyClaim:    "4 commits",
		},
	}
	got, suppressed := filterVCSHistoryRowOrderContradictions(doc, mut, contradictions)
	if suppressed != 1 {
		t.Fatalf("suppressed = %d, want 1", suppressed)
	}
	if len(got) != 1 || got[0].Kind != SelfConsistencyContradictionNumeric {
		t.Fatalf("expected only non-row-order contradiction to remain, got %+v", got)
	}
}

func TestFilterVCSHistoryRowOrderContradictions_DoesNotSuppressUnsortedGitRows(t *testing.T) {
	mut := types.NewMutableState("最近 3 次提交都做了什么")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "git_log",
		Success:  true,
		Summary: strings.Join([]string{
			"commit ae1dd6b256fab219104c09447b6ffe3697239b7a",
			"commit 3ae8465b6afe3fb16902d511d51482fefd09a103",
			"commit 125687ab6f1ff7cd1187183fc459efe65be10fb3",
		}, "\n"),
	}}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "commits",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{
				{Label: "3ae8465"},
				{Label: "ae1dd6b"},
				{Label: "125687a"},
			},
		}},
	}
	contradictions := []SelfConsistencyContradiction{{
		Kind:         SelfConsistencyContradictionRowOrder,
		Topic:        "commit order",
		SummaryClaim: "按时间倒序排列",
		BodyClaim:    "3ae8465, ae1dd6b, 125687a",
	}}
	got, suppressed := filterVCSHistoryRowOrderContradictions(doc, mut, contradictions)
	if suppressed != 0 {
		t.Fatalf("suppressed = %d, want 0", suppressed)
	}
	if len(got) != 1 {
		t.Fatalf("expected contradiction to remain, got %+v", got)
	}
}
