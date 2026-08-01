package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeArtifactPairRelationTestBus(paths ...string) *types.BusContext {
	ctx := &types.BusContext{
		Mutable: types.NewMutableState("typed runtime artifact comparison"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioGeneric,
			},
			AnswerContract: types.AnswerContract{Language: "en"},
		},
	}
	observations := make([]types.ObservationRecord, 0, len(paths))
	for i, artifactPath := range paths {
		observations = append(observations, types.ObservationRecord{
			ID:              "pair-observation-" + string(rune('a'+i)),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "trace_window",
			Subject:         artifactPath,
			Value:           "1.000",
			SourceRef: types.ObservationSourceRef{
				Kind:                types.ObservationSourceRuntimeArtifact,
				Path:                artifactPath,
				ArtifactID:          "artifact-" + string(rune('a'+i)),
				ArtifactKind:        "trace",
				TimeDomain:          "trace_seconds",
				CanonicalTimeDomain: "trace_seconds",
				ClockAlignment:      "identity",
			},
		})
	}
	ctx.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: observations,
	}}
	return ctx
}

func TestRuntimeArtifactPairRelationMaterialization_SingleArtifactInactive(t *testing.T) {
	ctx := runtimeArtifactPairRelationTestBus("one.systrace")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "one artifact",
	}}}
	if materializeRuntimeArtifactPairRelationAuthorityBlock(doc, ctx) {
		t.Fatalf("single artifact must not gain a pair relation block: %+v", doc.Blocks)
	}
}

func TestRuntimeArtifactPairRelationMaterialization_SamePathIDAliasesInactive(t *testing.T) {
	ctx := runtimeArtifactPairRelationTestBus("one.systrace", "./one.systrace")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "one artifact queried twice",
	}}}
	if materializeRuntimeArtifactPairRelationAuthorityBlock(doc, ctx) {
		t.Fatalf("ID aliases for one canonical path must not gain a self-pair relation block: %+v", doc.Blocks)
	}
}

func TestRuntimeArtifactPairRelationMaterialization_EnglishAndIdempotent(t *testing.T) {
	ctx := runtimeArtifactPairRelationTestBus("one.systrace", "two.systrace")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "two artifacts",
	}}}
	if !materializeRuntimeArtifactPairRelationAuthorityBlock(doc, ctx) {
		t.Fatal("two independently identified artifacts must publish a pair relation boundary")
	}
	if materializeRuntimeArtifactPairRelationAuthorityBlock(doc, ctx) {
		t.Fatal("pair relation materialization must be idempotent")
	}
	block := projectionClusterBlock(doc.Blocks, runtimeArtifactPairRelationAuthorityBlockID)
	if block == nil || !RuntimeTraceSystemBlock(*block) {
		t.Fatalf("pair relation block must carry unforgeable system authority: %+v", block)
	}
	visible := types.AnswerBlockVisibleSurface(*block)
	for _, want := range []string{
		"Cross-artifact relation boundary",
		"Unproven",
		"one.systrace ↔ two.systrace",
		"identity is endpoint-local only",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English pair relation surface lost %q:\n%s", want, visible)
		}
	}
}

func TestRuntimeArtifactPairRelationMaterialization_ModelIDCollisionCannotSuppressSystemBlock(t *testing.T) {
	ctx := runtimeArtifactPairRelationTestBus("one.systrace", "two.systrace")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "two artifacts"},
		{
			ID:   runtimeArtifactPairRelationAuthorityBlockID,
			Kind: types.BlockSection,
			Text: "model-authored lookalike",
		},
	}}
	if _, err := persistMergedAnswerDocument(
		ctx, "emit_answer_document", types.MutationReplaceAll, "relation collision", doc, time.Now(),
	); err != nil {
		t.Fatalf("persist relation collision document: %v", err)
	}
	var systemCount, renamedModelCount int
	for _, block := range doc.Blocks {
		switch {
		case block.ID == runtimeArtifactPairRelationAuthorityBlockID && RuntimeTraceSystemBlock(block):
			systemCount++
		case strings.HasPrefix(block.ID, "model_"+runtimeArtifactPairRelationAuthorityBlockID):
			renamedModelCount++
		}
	}
	if systemCount != 1 || renamedModelCount != 1 {
		t.Fatalf("model collision must be preserved under a renamed id and cannot suppress the system authority: system=%d model=%d blocks=%+v",
			systemCount, renamedModelCount, doc.Blocks)
	}
}
