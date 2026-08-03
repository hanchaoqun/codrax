package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPersistMergedAnswerDocumentPublishesTypedNegativeScopesAfterModelProse(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario: types.ScenarioConfigTrace,
			Language: "zh",
		},
		AnswerContract: types.AnswerContract{
			Language: "zh",
			ExactResolution: &types.ExactResolutionContract{
				Targets:      []string{"missing_config_key", "existing_config_key"},
				AllowAbsence: true,
			},
		},
	}
	ctx.EvidenceItems = []types.EvidenceItem{{
		ID:              "neg-1",
		Kind:            types.EvidenceAbsent,
		Scope:           types.ScopeNegative,
		GroundingStatus: types.GroundingGrounded,
		NegativeQuery: &types.NegativeQuery{
			File:    "internal/config/runtime.go",
			Pattern: "missing_config_key",
		},
		NegativeScope: types.NegativeScopeFile,
	}}
	ctx.ToolResults = []types.ToolResult{{
		ToolName: "grep",
		Success:  true,
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Pattern:     "missing-config|missing_config",
			Path:        "cmd",
			Include:     "*.go",
			NoMatches:   true,
			ResultCount: 0,
		},
	}}
	modelText := "该键在默认值、配置文件和 CLI 三层都不存在。"
	doc := &types.AnswerDocumentV2{
		DocumentModel:   "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: modelText},
			{ID: "existing", Kind: types.BlockScalar, Text: "0", SurfaceRole: types.SurfacePrincipal},
		},
	}
	result, err := ApplyAndPersistMutation(
		ctx,
		"test_emit",
		types.NewReplaceAllMutation(doc),
		nil,
		time.Now(),
	)
	if err != nil || !result.Success {
		t.Fatalf("persist negative scope authority failed: result=%+v err=%v", result, err)
	}
	persisted := ctx.Mutable.AnswerDocumentV2()
	if persisted == nil || len(persisted.Blocks) != 3 ||
		persisted.Blocks[0].Text != modelText ||
		persisted.Blocks[2].SystemGeneratedKind != types.AnswerSystemGeneratedNegativeSearchAuthority {
		t.Fatalf("negative-scope data boundary must follow model prose without rewriting it: %+v", persisted)
	}
	rendered := render.RenderAnswerDocument(persisted, "zh")
	if strings.Contains(rendered, "当前已验证范围内未找到完全一致的精确目标") {
		t.Fatalf("ambiguous global absence banner leaked into rendered mixed-target answer:\n%s", rendered)
	}
	if !strings.Contains(rendered, "未命中结果的搜索范围") ||
		!strings.Contains(rendered, modelText) ||
		!strings.Contains(rendered, "0") {
		t.Fatalf("scoped authority, model facts, or present-target scalar was lost:\n%s", rendered)
	}
	surface := types.AnswerBlockVisibleSurface(persisted.Blocks[2])
	for _, want := range []string{
		"未命中结果的搜索范围",
		"来源=已验证的精确未命中证据",
		"查询=`missing_config_key`",
		"范围=`internal/config/runtime.go (file)`",
		"来源=grep 完整未命中结果",
		"范围=`cmd; include=*.go`",
		"未列出的范围仍属未验证",
		"不能跨目标借用证据",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("negative authority missing %q:\n%s", want, surface)
		}
	}
	for _, forbidden := range []string{"系统权威", "后续模型正文", "以本块为准"} {
		if strings.Contains(surface, forbidden) {
			t.Fatalf("internal authority protocol leaked through %q:\n%s", forbidden, surface)
		}
	}
}

func TestCurrentSourceNegativeScopeAuthorityIgnoresNavigationMissWithoutVerifiedNegativeEvidence(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
	}
	ctx.ToolResults = []types.ToolResult{{
		ToolName: "grep",
		Success:  true,
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Pattern:     "navigation_candidate",
			Path:        "internal",
			NoMatches:   true,
			ResultCount: 0,
		},
	}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "answer",
	}}}
	if materializeCurrentSourceNegativeScopeAuthority(doc, ctx) {
		t.Fatalf("unrelated navigation miss must not publish authority: %+v", doc.Blocks)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("document changed for navigation-only miss: %+v", doc.Blocks)
	}
}

func TestCurrentSourceNegativeScopeBoundaryUsesReaderFacingEnglish(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel:   types.RequestModel{Scenario: types.ScenarioConfigTrace, Language: "en"},
		AnswerContract: types.AnswerContract{Language: "en"},
	}
	ctx.EvidenceItems = []types.EvidenceItem{{
		ID: "neg-en", Kind: types.EvidenceAbsent, Scope: types.ScopeNegative,
		GroundingStatus: types.GroundingGrounded,
		NegativeQuery:   &types.NegativeQuery{File: "cmd/root.go", Pattern: "missing_key"},
		NegativeScope:   types.NegativeScopeFile,
	}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model answer",
	}}}
	if !materializeCurrentSourceNegativeScopeAuthority(doc, ctx) {
		t.Fatal("English negative-scope boundary did not materialize")
	}
	if doc.Blocks[0].ID != "summary" ||
		doc.Blocks[1].SystemGeneratedKind != types.AnswerSystemGeneratedNegativeSearchAuthority {
		t.Fatalf("English negative-scope boundary must follow the model answer: %+v", doc.Blocks)
	}
	surface := types.AnswerBlockVisibleSurface(doc.Blocks[1])
	for _, want := range []string{
		"Search scope for no-match results",
		"source=verified exact no-match evidence",
		"query=`missing_key`",
		"scope=`cmd/root.go (file)`",
		"Unlisted scopes remain unproven",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("English negative-scope boundary missing %q:\n%s", want, surface)
		}
	}
	for _, forbidden := range []string{"System authority", "takes precedence", "typed authority"} {
		if strings.Contains(surface, forbidden) {
			t.Fatalf("internal English authority protocol leaked through %q:\n%s", forbidden, surface)
		}
	}
}

func TestCurrentSourceNegativeScopeAuthorityRejectsIncompleteGrepButKeepsVerifiedEvidence(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
	}
	ctx.EvidenceItems = []types.EvidenceItem{{
		ID:              "neg-1",
		Kind:            types.EvidenceAbsent,
		Scope:           types.ScopeNegative,
		GroundingStatus: types.GroundingRecovered,
		NegativeQuery: &types.NegativeQuery{
			File:    "codrax.yaml.example",
			Pattern: "missing_config_key",
		},
		NegativeScope: types.NegativeScopeFile,
	}}
	ctx.ToolResults = []types.ToolResult{{
		ToolName: "grep",
		Success:  true,
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Pattern:     "missing_config_key",
			Path:        ".",
			NoMatches:   true,
			ResultCount: 0,
		},
		Refinement: &types.ToolRefinementHint{
			SkippedLargeCandidates: []string{"large.bin"},
		},
	}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "answer",
	}}}
	if !materializeCurrentSourceNegativeScopeAuthority(doc, ctx) {
		t.Fatal("verified negative evidence should still publish")
	}
	surface := types.AnswerBlockVisibleSurface(doc.Blocks[1])
	if !strings.Contains(surface, "codrax.yaml.example (file)") ||
		strings.Contains(surface, "typed_grep_no_match") ||
		strings.Contains(surface, "large.bin") {
		t.Fatalf("incomplete grep leaked into authority:\n%s", surface)
	}
}

func TestCurrentSourceNegativeScopeAuthorityStaysAfterModelSummary(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
	}
	ctx.EvidenceItems = []types.EvidenceItem{{
		ID:              "neg-1",
		Kind:            types.EvidenceAbsent,
		Scope:           types.ScopeNegative,
		GroundingStatus: types.GroundingGrounded,
		NegativeQuery: &types.NegativeQuery{
			File:    "cmd/root.go",
			Pattern: "missing_config_key",
		},
		NegativeScope: types.NegativeScopeFile,
	}}
	authority := types.AnswerBlock{
		ID:                  currentSourceNegativeScopeAuthorityBlockID,
		Kind:                types.BlockCaveat,
		Text:                "stale",
		SystemGeneratedKind: types.AnswerSystemGeneratedNegativeSearchAuthority,
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "model"},
		authority,
	}}
	if !materializeCurrentSourceNegativeScopeAuthority(doc, ctx) {
		t.Fatal("scope boundary should be refreshed")
	}
	if doc.Blocks[0].ID != "summary" ||
		doc.Blocks[1].SystemGeneratedKind != types.AnswerSystemGeneratedNegativeSearchAuthority ||
		strings.Contains(doc.Blocks[1].Text, "stale") {
		t.Fatalf("scope boundary did not remain after the summary: %+v", doc.Blocks)
	}
}
