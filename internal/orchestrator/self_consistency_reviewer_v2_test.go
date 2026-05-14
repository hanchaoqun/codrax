package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
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
			{Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{
				{Label: "Step1", Text: "do A"},
				{Label: "Step2", Text: "do B"},
			}},
			{Kind: types.BlockBulletList, Items: []types.AnswerBlockItem{
				{Label: "category"},
			}},
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
		"1. Step1 — do A",
		"2. Step2 — do B",
		"- category",
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

func TestSelfConsistencyRowOrderGate_SuppressesSortedVisibleRows(t *testing.T) {
	doc := docWithSelfConsistencyRows(
		[]string{"aggregator", "amplifier", "axis"},
		nil,
	)
	verdict := &SelfConsistencyResult{
		Consistent: false,
		Confidence: 0.94,
		Reasoning:  "The body list is not alphabetically sorted.",
		Contradictions: []SelfConsistencyContradiction{{
			Topic:        "alphabetic row order",
			SummaryClaim: "按子包字母顺序列出",
			BodyClaim:    "aggregator, amplifier, axis",
		}},
	}

	remaining, suppressed := filterDeterministicRowOrderContradictions(doc, verdict)
	if suppressed != 1 {
		t.Fatalf("suppressed = %d, want 1", suppressed)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining = %d, want 0: %+v", len(remaining), remaining)
	}
}

func TestSelfConsistencyRowOrderGate_KeepsGenuineUnsortedRows(t *testing.T) {
	doc := docWithSelfConsistencyRows(
		[]string{"axis", "aggregator", "amplifier"},
		nil,
	)
	verdict := &SelfConsistencyResult{
		Consistent: false,
		Confidence: 0.94,
		Reasoning:  "The body list is not alphabetically sorted.",
		Contradictions: []SelfConsistencyContradiction{{
			Topic:        "alphabetic row order",
			SummaryClaim: "alphabetically sorted list",
			BodyClaim:    "axis, aggregator, amplifier",
		}},
	}

	remaining, suppressed := filterDeterministicRowOrderContradictions(doc, verdict)
	if suppressed != 0 {
		t.Fatalf("suppressed = %d, want 0", suppressed)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining = %d, want 1", len(remaining))
	}
}

func TestSelfConsistencyRowOrderGate_PartiallySuppressesOnlyDeterministicSortedness(t *testing.T) {
	doc := docWithSelfConsistencyRows(
		[]string{"aggregator", "amplifier", "axis"},
		nil,
	)
	verdict := &SelfConsistencyResult{
		Consistent: false,
		Confidence: 0.94,
		Reasoning:  "The body list is not alphabetically sorted and the summary count differs.",
		Contradictions: []SelfConsistencyContradiction{
			{
				Topic:        "alphabetic row order",
				SummaryClaim: "alphabetically sorted list",
				BodyClaim:    "aggregator, amplifier, axis",
			},
			{
				Topic:        "row count",
				SummaryClaim: "25 rows",
				BodyClaim:    "24 rows",
			},
		},
	}

	remaining, suppressed := filterDeterministicRowOrderContradictions(doc, verdict)
	if suppressed != 1 {
		t.Fatalf("suppressed = %d, want 1", suppressed)
	}
	if len(remaining) != 1 || remaining[0].Topic != "row count" {
		t.Fatalf("remaining = %+v, want only row count", remaining)
	}
}

func TestSelfConsistencyRowOrderGate_UsesCitationPathAxisAcrossLanguages(t *testing.T) {
	cases := []struct {
		name  string
		files []string
	}{
		{
			name: "go packages",
			files: []string{
				"internal/analysis/aggregator/aggregator.go",
				"internal/analysis/amplifier/amplifier.go",
				"internal/analysis/axis/axis.go",
			},
		},
		{
			name: "python packages",
			files: []string{
				"pkg/aggregator/__init__.py",
				"pkg/amplifier/__init__.py",
				"pkg/axis/__init__.py",
			},
		},
		{
			name: "scoped typescript packages",
			files: []string{
				"packages/@codrax/aggregator/src/index.ts",
				"packages/@codrax/amplifier/src/index.ts",
				"packages/@codrax/axis/src/index.ts",
			},
		},
		{
			name: "arkts modules",
			files: []string{
				"entry/src/main/ets/aggregator/index.ets",
				"entry/src/main/ets/amplifier/index.ets",
				"entry/src/main/ets/axis/index.ets",
			},
		},
		{
			name: "java packages",
			files: []string{
				"src/main/java/com/acme/aggregator/Handler.java",
				"src/main/java/com/acme/amplifier/Handler.java",
				"src/main/java/com/acme/axis/Handler.java",
			},
		},
		{
			name: "kotlin packages",
			files: []string{
				"src/main/kotlin/com/acme/aggregator/Handler.kt",
				"src/main/kotlin/com/acme/amplifier/Handler.kt",
				"src/main/kotlin/com/acme/axis/Handler.kt",
			},
		},
		{
			name: "rust crates",
			files: []string{
				"crates/aggregator/src/lib.rs",
				"crates/amplifier/src/lib.rs",
				"crates/axis/src/lib.rs",
			},
		},
		{
			name: "native and cangjie dirs",
			files: []string{
				"src/aggregator/main.cj",
				"src/amplifier/foo.cc",
				"src/axis/Foo.cs",
			},
		},
		{
			name: "script and app dirs",
			files: []string{
				"lib/aggregator/foo.rb",
				"lib/amplifier/init.lua",
				"lib/axis/foo.dart",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := docWithSelfConsistencyRows(
				[]string{"zeta visible label", "yankee visible label", "xray visible label"},
				tc.files,
			)
			if !selfConsistencyRowBlockHasSortedAxis(doc, doc.Blocks[1]) {
				t.Fatalf("expected citation path axis to prove sorted order for %v", tc.files)
			}
		})
	}
}

func TestSelfConsistencyRowOrderGate_RejectsAmbiguousDuplicateCitationAxis(t *testing.T) {
	doc := docWithSelfConsistencyRows(
		[]string{"zeta visible label", "yankee visible label", "xray visible label"},
		[]string{
			"src/main/java/com/acme/aggregator/Alpha.java",
			"src/main/java/com/acme/aggregator/Beta.java",
			"src/main/java/com/acme/axis/Gamma.java",
		},
	)
	verdict := &SelfConsistencyResult{
		Consistent: false,
		Confidence: 0.94,
		Reasoning:  "The body list is not alphabetically sorted.",
		Contradictions: []SelfConsistencyContradiction{{
			Topic:        "alphabetic row order",
			SummaryClaim: "alphabetically sorted list",
			BodyClaim:    "zeta visible label, yankee visible label, xray visible label",
		}},
	}

	remaining, suppressed := filterDeterministicRowOrderContradictions(doc, verdict)
	if suppressed != 0 {
		t.Fatalf("suppressed = %d, want 0 for duplicate path-axis keys", suppressed)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining = %d, want 1", len(remaining))
	}
}

func TestSelfConsistencySortednessDetection_DoesNotCatchSequenceDirection(t *testing.T) {
	c := SelfConsistencyContradiction{
		Topic:        "call order",
		SummaryClaim: "A runs before B",
		BodyClaim:    "B runs before A",
	}
	if selfConsistencyContradictionClaimsSortedness(c, "Direction or order mismatch between stages") {
		t.Fatal("call-chain order contradictions must not be treated as alphabetic sortedness disputes")
	}
}

func TestRunSelfConsistencyReviewV2_SuppressesDeterministicSortednessFalsePositive(t *testing.T) {
	doc := docWithSelfConsistencyRows(
		[]string{"aggregator", "amplifier", "axis"},
		nil,
	)
	mut := types.NewMutableState("list packages alphabetically")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Ctx:      context.Background(),
			Language: "zh",
			Mutable:  mut,
		},
		emit: render.NopEmitter,
		selfConsistencyReviewer: fixedSelfConsistencyReviewer{result: &SelfConsistencyResult{
			Consistent: false,
			Confidence: 0.96,
			Reasoning:  "The body list is not alphabetically sorted.",
			Contradictions: []SelfConsistencyContradiction{{
				Topic:        "alphabetic row order",
				SummaryClaim: "summary says the list is alphabetical",
				BodyClaim:    "body has aggregator, amplifier, axis",
			}},
		}},
		selfConsistencyMinConfidence:          0.8,
		selfConsistencyRewriteOnContradiction: true,
	}

	if got := o.runSelfConsistencyReviewV2(doc, mut); len(got) != 0 {
		t.Fatalf("runSelfConsistencyReviewV2 emitted %d violations, want 0: %+v", len(got), got)
	}
}

type fixedSelfConsistencyReviewer struct {
	result *SelfConsistencyResult
	err    error
}

func (f fixedSelfConsistencyReviewer) Review(context.Context, SelfConsistencyInput) (*SelfConsistencyResult, error) {
	return f.result, f.err
}

func docWithSelfConsistencyRows(labels []string, files []string) *types.AnswerDocumentV2 {
	summary := "This answer enumerates the requested rows and states that the principal list is sorted in a stable, deterministic order for review. "
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "summary",
				Kind: types.BlockSummary,
				Text: summary + summary,
			},
			{
				ID:          "principal_rows",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
			},
		},
	}
	for i, label := range labels {
		item := types.AnswerBlockItem{
			ID:    "row",
			Label: label,
			Text:  "grounded row",
		}
		if i < len(files) {
			item.CitationRef = i
			doc.Citations = append(doc.Citations, types.Citation{File: files[i], Line: 10 + i})
		}
		doc.Blocks[1].Items = append(doc.Blocks[1].Items, item)
	}
	return doc
}
