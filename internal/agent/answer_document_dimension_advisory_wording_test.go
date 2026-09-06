package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDimensionAdvisoryDistinguishesUnconfirmedBindingFromMissingContent(t *testing.T) {
	dimension := types.RequestedAnswerDimension{Label: "实现类型所在文件", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 2}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{dimension}},
	}}}
	for _, tc := range []struct {
		name        string
		block       types.AnswerBlock
		wantMissing bool
	}{
		{
			name: "complete_visible_roster_without_binding",
			block: types.AnswerBlock{ID: "members", Kind: types.BlockSection, Title: "实现类型所在文件", Items: []types.AnswerBlockItem{
				{ID: "a", Label: "Alpha", Text: "internal/alpha.go:10"}, {ID: "b", Label: "Beta", Text: "internal/beta.go:20"},
			}},
			wantMissing: true,
		},
		{
			name: "same_visible_roster_with_binding",
			block: types.AnswerBlock{ID: "members", Kind: types.BlockSection, Title: "实现类型所在文件", FacetIDs: []string{"member_set"}, Items: []types.AnswerBlockItem{
				{ID: "a", Label: "Alpha", Text: "internal/alpha.go:10"}, {ID: "b", Label: "Beta", Text: "internal/beta.go:20"},
			}},
		},
		{
			name: "diagram_without_independent_roster",
			block: types.AnswerBlock{ID: "diagram", Kind: types.BlockDiagram, Title: "类型关系", Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramArchitecture, Language: "mermaid", Body: "flowchart TD\n  Alpha --> Contract\n  Beta --> Contract",
			}},
			wantMissing: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{tc.block}}
			before, _ := json.Marshal(doc)
			missing := requestedAnswerDimensionsRequiringPatchRetry(missingRequestedAnswerDimensionsInDocument(ctx, doc))
			if (len(missing) > 0) != tc.wantMissing {
				t.Fatalf("existing typed coverage contract changed: missing=%+v", missing)
			}
			if tc.wantMissing {
				for _, lang := range []string{"zh", "en"} {
					hint := requestedAnswerDimensionCoverageHint(ctx, missing, lang)
					assertDimensionAdvisoryNeutralWording(t, hint, lang)
					if !strings.Contains(hint, `facet_ids:["member_set"]`) || !strings.Contains(hint, "replace_blocks") {
						t.Fatalf("existing precise ownership/patch instruction disappeared: %s", hint)
					}
				}
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("coverage guidance modified the model-authored answer")
			}
		})
	}
}

func TestDimensionAdvisoryNeutralWordingKeepsUserLabelsAndDimensionOrder(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		hint := requestedAnswerDimensionCoverageHint(nil, []types.RequestedAnswerDimension{
			{Label: "影响", Role: types.RequestedAnswerDimensionImpact, Index: 3, Required: true},
			{Label: "作用", Role: types.RequestedAnswerDimensionFunctionOrPurpose, Index: 1, Required: true},
		}, lang)
		assertDimensionAdvisoryNeutralWording(t, hint, lang)
		if strings.Index(hint, "作用") < 0 || strings.Index(hint, "作用") >= strings.Index(hint, "影响") {
			t.Fatalf("user-facing dimension order changed: %s", hint)
		}
		if strings.Contains(hint, "function_or_purpose") {
			t.Fatalf("user-label list exposed internal role: %s", hint)
		}
	}
}

func assertDimensionAdvisoryNeutralWording(t *testing.T, hint, lang string) {
	t.Helper()
	var want, forbidden []string
	if lang == "zh" {
		want = []string{"这不等于正文确实缺失", "先核对现有内容", "只补相应归属或绑定", "确实缺少内容时", "待核对维度："}
		forbidden = []string{"最终可见答案遗漏了", "缺失维度："}
	} else {
		want = []string{"does not prove that visible content is missing", "Check the existing content first", "repair only its ownership or binding", "If content is actually absent", "Dimensions to check:"}
		forbidden = []string{"The visible final answer omitted", "Missing dimensions:"}
	}
	for _, token := range want {
		if !strings.Contains(hint, token) {
			t.Errorf("missing neutral advisory guidance %q: %s", token, hint)
		}
	}
	for _, token := range forbidden {
		if strings.Contains(hint, token) {
			t.Errorf("typed coverage miss still asserts a prose omission %q: %s", token, hint)
		}
	}
}
