package orchestrator

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceOracleScopeRequest(exclude bool) types.RequestModel {
	rm := types.RequestModel{Buckets: []types.QuestionBucket{{Label: "A", Index: 1}, {Label: "B", Index: 2}}}
	if exclude {
		rm.ExternalObservationPolicy = &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
			ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"Only compare the supplied measurements."},
		}
	}
	return rm
}

func sourceOracleScopeDocument(claims []types.RenderedClaimUse) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "Compare `observedTotalDuration`.", ClaimUses: claims},
		{ID: "metrics", Kind: types.BlockOrderedList, ClaimUses: claims, Items: []types.AnswerBlockItem{
			{ID: "total", Label: "observedTotalDuration"},
			{ID: "busy", Label: "observedBusyDuration"},
			{ID: "idle", Label: "observedIdleDuration"},
		}},
		{ID: "timeline", Kind: types.BlockDiagram, ClaimUses: claims, Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramCallDAG, Body: "graph TD\n observedStartEvent --> observedFinishEvent\n",
		}},
	}}
}

func sourceOracleScopeViolation(kind types.ViolationKind) bool {
	switch kind {
	case types.ViolEnumerationLabelUngrounded, types.ViolEnumerationLabelHallucinated,
		types.ViolEnumerationItemLabelExtractorDrift, types.ViolDiagramEdgeEndpointHallucinated,
		types.ViolInlineIdentifierHallucinated:
		return true
	}
	return false
}

func TestSourceSymbolOracleScopeProductionBundle(t *testing.T) {
	external := []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
	mixed := append(append([]types.RenderedClaimUse{}, external...), types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact})
	mixedRequest := sourceOracleScopeRequest(false)
	mixedRequest.PerfTrace = &types.PerfBundle{Observations: []types.PerfObservation{{Kind: "trace_mark", Subject: "work"}}}
	mixedRequest.ExternalObservationPolicy = &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceAllow, ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}
	for _, tc := range []struct {
		name       string
		rm         types.RequestModel
		claims     []types.RenderedClaimUse
		wantSource bool
	}{
		{"excluded_without_block_claims", sourceOracleScopeRequest(true), nil, false},
		{"external_block_in_source_request", sourceOracleScopeRequest(false), external, false},
		{"source_request", sourceOracleScopeRequest(false), nil, true},
		{"explicit_mixed_request", mixedRequest, nil, true},
		{"mixed_claims_keep_source_oracle", sourceOracleScopeRequest(false), mixed, true},
		{"missing_exclusion_witness_is_not_exclusion", types.RequestModel{ExternalObservationPolicy: &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceExclude}}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := mutWithEvidence([]types.EvidenceItem{{ID: "source", AnchorSymbol: "unrelatedSourceDeclaration"}})
			mut.SetRequestModel(tc.rm)
			mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{{Name: "sourceAlpha"}, {Name: "sourceBeta"}, {Name: "sourceGamma"}}, types.CompletenessClaim(""))
			doc := sourceOracleScopeDocument(tc.claims)
			before, _ := json.Marshal(doc)
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
			denials := types.NewTypedDenialSet()
			bus := &types.BusContext{Mutable: mut, TypedDenials: denials, AnalysisIR: &types.AnalysisIR{RequestModel: tc.rm}}
			view := &types.AnswerSemanticView{Family: types.QFEnumeration}
			vs := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, denialStubOracle{}, denials, bus)
			var sourceViolations []types.Violation
			for _, v := range vs {
				if sourceOracleScopeViolation(v.Kind) {
					sourceViolations = append(sourceViolations, v)
				}
			}
			if tc.wantSource != (len(sourceViolations) > 0) {
				t.Errorf("source oracle applicability=%v: %+v", tc.wantSource, sourceViolations)
			}
			if tc.wantSource && len(sourceViolations) != 5 {
				t.Errorf("all five source naming diagnostics must remain active: %+v", sourceViolations)
			}
			if !tc.wantSource && len(denials.AdvisoryAnswerSurfaceSymbolTokens()) != 0 {
				t.Errorf("external surface acquired repository advisory tokens: %v", denials.AdvisoryAnswerSurfaceSymbolTokens())
			}
			note := enumerationLabelVerificationSupplement(vs, bus, "zh")
			if tc.wantSource != (note != "") {
				t.Errorf("wrong source supplement applicability: %q", note)
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("oracle rewrote model document")
			}
		})
	}
}

func TestSourceSymbolOracleScopeSupplementRejectsStaleCrossDomainAdvisory(t *testing.T) {
	mut := types.NewMutableState("compare")
	mut.SetRequestModel(sourceOracleScopeRequest(true))
	doc := sourceOracleScopeDocument(nil)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	denials := types.NewTypedDenialSet()
	denials.AddAnswerSurfaceAdvisory("observedTotalDuration", "old source-shaped draft")
	bus := &types.BusContext{Mutable: mut, TypedDenials: denials}
	vs := []types.Violation{{Kind: types.ViolEnumerationLabelHallucinated}}
	if note := enumerationLabelVerificationSupplement(vs, bus, "zh"); note != "" {
		t.Fatalf("stale advisory crossed source boundary: %s", note)
	}
	mut.SetRequestModel(sourceOracleScopeRequest(false))
	doc.Blocks[1].ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	if note := enumerationLabelVerificationSupplement(vs, bus, "zh"); note != "" {
		t.Fatalf("external block inherited source advisory: %s", note)
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "source", Kind: types.BlockTable, Items: []types.AnswerBlockItem{{ID: "code", Label: "missingSourceFunction"}}})
	denials.AddAnswerSurfaceAdvisory("missingSourceFunction", "source check")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	note := enumerationLabelVerificationSupplement(vs, bus, "zh")
	if !strings.Contains(note, "missingSourceFunction") || strings.Contains(note, "observedTotalDuration") {
		t.Fatalf("mixed source/external note=%q", note)
	}
	if !reflect.DeepEqual(denials.AdvisoryAnswerSurfaceSymbolTokens(), []string{"observedTotalDuration", "missingSourceFunction"}) {
		t.Fatal("stored advisory history changed")
	}
}
