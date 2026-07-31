package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeRuntimeArtifactObservationCitationRefsDetachesOnlyCrossOriginEdge(t *testing.T) {
	ctx := mixedOriginArtifactScalarContext(t)
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID: "runtime-value", Kind: types.BlockScalar,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
				Items: []types.AnswerBlockItem{
					{ID: "wrong-source", CitationRef: 0},
					{ID: "artifact-provenance", CitationRef: 1},
				},
			},
			{
				ID: "mechanism", Kind: types.BlockSummary,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
				Items:     []types.AnswerBlockItem{{ID: "source-proof", CitationRef: 0}},
			},
			{
				ID: "runtime-decision", Kind: types.BlockDecision,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
				Items:     []types.AnswerBlockItem{{ID: "decision-value", CitationRef: 0}},
			},
		},
		Citations: []types.Citation{
			{File: "source.go", Line: 1},
			{File: "customer.systrace", Line: 5},
		},
	}
	pctx := newPreEmitCheckContext(ctx)
	if fixed := normalizeRuntimeArtifactObservationCurrentSourceCitationRefsWithContext(doc, ctx, pctx); fixed != 2 {
		t.Fatalf("fixed=%d, want scalar and decision cross-origin edges detached", fixed)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != -1 {
		t.Fatalf("runtime scalar retained current-source citation_ref=%d", got)
	}
	if got := doc.Blocks[0].Items[1].CitationRef; got != 1 {
		t.Fatalf("runtime artifact provenance was detached: %d", got)
	}
	if got := doc.Blocks[1].Items[0].CitationRef; got != 0 {
		t.Fatalf("sibling mechanism source citation was detached: %d", got)
	}
	if got := doc.Blocks[2].Items[0].CitationRef; got != -1 {
		t.Fatalf("runtime decision retained current-source citation_ref=%d", got)
	}
	records := pctx.detachedCitationDisclosures()
	if len(records) != 2 || records[0].Kind != types.DetachedCitationKindEvidenceOriginMismatch ||
		records[0].BlockID != "runtime-value" || records[0].ItemID != "wrong-source" {
		t.Fatalf("origin-mismatch disclosure drifted: %+v", records)
	}
	if records[1].BlockID != "runtime-decision" || records[1].ItemID != "decision-value" {
		t.Fatalf("decision-shaped origin mismatch was not disclosed: %+v", records)
	}
}

func TestNormalizeRuntimeArtifactObservationCitationRefsTypedNegativeArms(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*types.BusContext, *types.AnswerBlock, *types.Citation)
		wantRef int
	}{
		{
			name: "inactive artifact value profile",
			mutate: func(ctx *types.BusContext, _ *types.AnswerBlock, _ *types.Citation) {
				ctx.AnalysisIR.RequestModel.RuntimeArtifactValueProfile.IsArtifactValueLookup = false
			},
			wantRef: 0,
		},
		{
			name: "source-only claim",
			mutate: func(_ *types.BusContext, block *types.AnswerBlock, _ *types.Citation) {
				block.ClaimUses[0].ClaimForm = types.ClaimLiteralValueFact
			},
			wantRef: 0,
		},
		{
			name: "mixed observation and source claim",
			mutate: func(_ *types.BusContext, block *types.AnswerBlock, _ *types.Citation) {
				block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact})
			},
			wantRef: 0,
		},
		{
			name: "missing claim use",
			mutate: func(_ *types.BusContext, block *types.AnswerBlock, _ *types.Citation) {
				block.ClaimUses = nil
			},
			wantRef: 0,
		},
		{
			name: "artifact citation",
			mutate: func(_ *types.BusContext, _ *types.AnswerBlock, citation *types.Citation) {
				citation.File = "customer.systrace"
			},
			wantRef: 0,
		},
		{
			name: "external path outside current repo",
			mutate: func(_ *types.BusContext, _ *types.AnswerBlock, citation *types.Citation) {
				citation.File = filepath.Join(t.TempDir(), "external.go")
			},
			wantRef: 0,
		},
		{
			name: "nonexistent relative artifact alias",
			mutate: func(_ *types.BusContext, _ *types.AnswerBlock, citation *types.Citation) {
				citation.File = "attached_trace"
			},
			wantRef: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := mixedOriginArtifactScalarContext(t)
			block := types.AnswerBlock{
				ID: "scalar", Kind: types.BlockScalar,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
				Items:     []types.AnswerBlockItem{{ID: "value", CitationRef: 0}},
			}
			citation := types.Citation{File: "source.go", Line: 1}
			tc.mutate(ctx, &block, &citation)
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}, Citations: []types.Citation{citation}}
			normalizeRuntimeArtifactObservationCurrentSourceCitationRefsWithContext(doc, ctx, nil)
			if got := doc.Blocks[0].Items[0].CitationRef; got != tc.wantRef {
				t.Fatalf("citation_ref=%d, want %d", got, tc.wantRef)
			}
		})
	}
}

func TestNormalizeAnswerDocumentForPreEmitWiresArtifactObservationOriginGate(t *testing.T) {
	ctx := mixedOriginArtifactScalarContext(t)
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "runtime-value", Kind: types.BlockScalar, SurfaceRole: types.SurfacePrincipal,
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}},
			Items:     []types.AnswerBlockItem{{ID: "value", CitationRef: 0}},
		}},
		Citations: []types.Citation{{File: "source.go", Line: 1}},
	}
	pctx := newPreEmitCheckContext(ctx)
	normalizeAnswerDocumentForPreEmit("test", doc, &types.AnswerSemanticView{}, ctx, pctx)
	if got := doc.Blocks[0].Items[0].CitationRef; got != -1 {
		t.Fatalf("production normalize chain did not detach cross-origin scalar citation: %d", got)
	}
	records := ctx.Mutable.TakePendingDetachedCitationDisclosures()
	if len(records) != 1 || records[0].Kind != types.DetachedCitationKindEvidenceOriginMismatch {
		t.Fatalf("production normalize chain did not ferry typed disclosure: %+v", records)
	}
	materializeDetachedCitationRefCaveats(doc, ctx, records)
	if len(doc.Caveats) == 0 || !strings.Contains(doc.Caveats[len(doc.Caveats)-1], "源码行可解释机制但不能证明该次运行时测量") {
		t.Fatalf("origin mismatch was not honestly disclosed: %+v", doc.Caveats)
	}
}

func mixedOriginArtifactScalarContext(t *testing.T) *types.BusContext {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutable := types.NewMutableState("mixed artifact scalar citation")
	return &types.BusContext{
		RepoRoot: repo,
		WorkDir:  repo,
		Language: "zh",
		Mutable:  mutable,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeArtifactValueProfile: &types.RuntimeArtifactValueProfile{
				IsArtifactValueLookup: true,
				Target:                "frame duration",
				Value:                 "86.111",
				Unit:                  "ms",
				Confidence:            1,
			},
		}},
	}
}
