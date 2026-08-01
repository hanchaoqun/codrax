package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPersistMergedAnswerDocumentPublishesTypedNegativeScopesBeforeModelProse(t *testing.T) {
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
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
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
		persisted.Blocks[0].SystemGeneratedKind != types.AnswerSystemGeneratedNegativeSearchAuthority ||
		persisted.Blocks[1].Text != modelText {
		t.Fatalf("negative scope authority must lead without rewriting model prose: %+v", persisted)
	}
	if persisted.ExactResolution != nil {
		t.Fatalf("multi-target targetless absence verdict must not survive production persist: %+v", persisted.ExactResolution)
	}
	rendered := render.RenderAnswerDocument(persisted, "zh")
	if strings.Contains(rendered, "当前已验证范围内未找到完全一致的精确目标") {
		t.Fatalf("ambiguous global absence banner leaked into rendered mixed-target answer:\n%s", rendered)
	}
	if !strings.Contains(rendered, "系统权威：否定证据的目标与搜索范围") ||
		!strings.Contains(rendered, modelText) ||
		!strings.Contains(rendered, "0") {
		t.Fatalf("scoped authority, model facts, or present-target scalar was lost:\n%s", rendered)
	}
	surface := types.AnswerBlockVisibleSurface(persisted.Blocks[0])
	for _, want := range []string{
		"系统权威：否定证据的目标与搜索范围",
		"producer=verified_negative_evidence",
		"pattern=`missing_config_key`",
		"scope=`internal/config/runtime.go (file)`",
		"producer=typed_grep_no_match",
		"scope=`cmd; include=*.go`",
		"unlisted_scope_status=unproven",
		"cross_target_borrowing=forbidden",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("negative authority missing %q:\n%s", want, surface)
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
	surface := types.AnswerBlockVisibleSurface(doc.Blocks[0])
	if !strings.Contains(surface, "codrax.yaml.example (file)") ||
		strings.Contains(surface, "typed_grep_no_match") ||
		strings.Contains(surface, "large.bin") {
		t.Fatalf("incomplete grep leaked into authority:\n%s", surface)
	}
}

func TestCurrentSourceNegativeScopeAuthorityRestoresLeadAfterSummaryCanonicalization(t *testing.T) {
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
		authority,
		{ID: "summary", Kind: types.BlockSummary, Text: "model"},
	}}
	if !canonicalizeSummaryLeadBlock(doc) {
		t.Fatal("fixture must reproduce summary canonicalization moving the authority back")
	}
	if doc.Blocks[0].ID != "summary" {
		t.Fatalf("fixture order = %+v", doc.Blocks)
	}
	if !materializeCurrentSourceNegativeScopeAuthority(doc, ctx) {
		t.Fatal("authority should be refreshed and restored to the lead")
	}
	if doc.Blocks[0].SystemGeneratedKind != types.AnswerSystemGeneratedNegativeSearchAuthority ||
		doc.Blocks[1].ID != "summary" ||
		strings.Contains(doc.Blocks[0].Text, "stale") {
		t.Fatalf("authority lead was not restored: %+v", doc.Blocks)
	}
}
