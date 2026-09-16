package orchestrator

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceSymbolOracleScopeMixedDocumentKeepsSourceDiagnosticsLocal(t *testing.T) {
	for _, externalFirst := range []bool{true, false} {
		name := "source_first"
		if externalFirst {
			name = "external_first"
		}
		t.Run(name, func(t *testing.T) {
			mut := mutWithEvidence([]types.EvidenceItem{{ID: "source", AnchorSymbol: "unrelatedSourceDeclaration"}})
			rm := sourceOracleScopeRequest(false)
			mut.SetRequestModel(rm)
			mut.SetEmittedAnswerSymbols([]types.AnswerSymbol{{Name: "sourceAlpha"}, {Name: "sourceBeta"}, {Name: "sourceGamma"}}, types.CompletenessClaim(""))
			external := sourceOracleScopeDocument([]types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}})
			source := sourceOracleScopeDocument(nil)
			for i := range external.Blocks {
				external.Blocks[i].ID = "external-" + external.Blocks[i].ID
				source.Blocks[i].ID = "source-" + source.Blocks[i].ID
			}
			source.Blocks[0].Text = "Inspect `missingSourceDuration`."
			for i, label := range []string{"missingSourceDuration", "missingSourceBusy", "missingSourceIdle"} {
				source.Blocks[1].Items[i].Label = label
			}
			source.Blocks[2].Diagram.Body = "graph TD\n missingSourceCaller --> missingSourceReceiver\n"
			doc := &types.AnswerDocumentV2{}
			if externalFirst {
				doc.Blocks = append(external.Blocks, source.Blocks...)
			} else {
				doc.Blocks = append(source.Blocks, external.Blocks...)
			}
			before, _ := json.Marshal(doc)
			mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
			denials := types.NewTypedDenialSet()
			// An old source-shaped draft recorded this token. Its current
			// external block must neither inherit nor erase that advisory.
			denials.AddAnswerSurfaceAdvisory("observedTotalDuration", "previous source-shaped draft")
			bus := &types.BusContext{Mutable: mut, TypedDenials: denials, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
			view := &types.AnswerSemanticView{Family: types.QFEnumeration}
			violations := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, denialStubOracle{}, denials, bus)
			want := map[types.ViolationKind]string{
				types.ViolEnumerationLabelUngrounded:         blockClusterKey("source-metrics", "block_items_label"),
				types.ViolEnumerationLabelHallucinated:       blockClusterKey("source-metrics", "block_items_label"),
				types.ViolEnumerationItemLabelExtractorDrift: blockClusterKey("source-metrics", "block_items_label"),
				types.ViolDiagramEdgeEndpointHallucinated:    blockClusterKey("source-timeline", "diagram_edges"),
				types.ViolInlineIdentifierHallucinated:       blockClusterKey("source-summary", "inline_identifier"),
			}
			seen := make(map[types.ViolationKind]int)
			for _, violation := range violations {
				if !sourceOracleScopeViolation(violation.Kind) {
					continue
				}
				seen[violation.Kind]++
				if violation.ClusterKey != want[violation.Kind] {
					t.Errorf("source diagnostic leaked across block provenance: %+v", violation)
				}
			}
			for kind := range want {
				if seen[kind] != 1 {
					t.Errorf("source diagnostic %s occurred %d times, want exactly one", kind, seen[kind])
				}
			}
			advisories := denials.AdvisoryAnswerSurfaceSymbolTokens()
			if len(advisories) == 0 || advisories[0] != "observedTotalDuration" {
				t.Fatalf("original advisory history was discarded: %v", advisories)
			}
			for _, token := range advisories[1:] {
				if !strings.HasPrefix(token, "missingSource") {
					t.Errorf("external sibling acquired a new source advisory: %q", token)
				}
			}
			for _, lang := range []string{"zh", "en"} {
				note := enumerationLabelVerificationSupplement(violations, bus, lang)
				if strings.Contains(note, "observed") {
					t.Errorf("historical external token leaked into %s supplement: %s", lang, note)
				}
				for _, label := range []string{"missingSourceDuration", "missingSourceBusy", "missingSourceIdle"} {
					if !strings.Contains(note, label) {
						t.Errorf("source sibling lost %s advisory %q: %s", lang, label, note)
					}
				}
			}
			if !reflect.DeepEqual(advisories, denials.AdvisoryAnswerSurfaceSymbolTokens()) {
				t.Fatal("supplement filtered the persisted advisory history instead of its display")
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("source diagnostics rewrote the mixed model document")
			}
		})
	}
}

func TestSourceSymbolOracleScopeDoesNotBypassRuntimeRelationEvidence(t *testing.T) {
	for _, mode := range []string{"source_lane_excluded", "external_only_claims"} {
		t.Run(mode, func(t *testing.T) {
			doc, view, mut, bus := runtimeTemporalPostCheckFixture()
			rm := sourceOracleScopeRequest(false)
			mut.SetRequestModel(rm)
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{
				ID: "unproven-source", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramCallDAG, Language: "mermaid",
					Body: "graph TD\n SourceFlowCaller --> SourceFlowReceiver\n"},
				EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "SourceFlowCaller", ToNode: "SourceFlowReceiver",
					FromIdentity: "SourceFlowCaller", ToIdentity: "SourceFlowReceiver", RelationKind: types.DiagramRelCall}},
			})
			strictRelations := func(violations []types.Violation) []types.Violation {
				var out []types.Violation
				for _, violation := range violations {
					if violation.Kind == types.ViolDiagramCallEdgeUnproven || violation.Kind == types.ViolDiagramRelationLabelOnly {
						out = append(out, violation)
					}
				}
				return out
			}
			baseline := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, denialStubOracle{}, nil, bus)
			wantRelations := strictRelations(baseline)
			foundSourceFailure, foundSourceNameDiagnostic := false, false
			for _, violation := range baseline {
				if violation.Kind == types.ViolDiagramCallEdgeUnproven && violation.ClusterKey == blockClusterKey("unproven-source", "diagram_call_edge_evidence") {
					foundSourceFailure = true
				}
				if violation.Kind == types.ViolDiagramEdgeEndpointHallucinated {
					foundSourceNameDiagnostic = true
				}
			}
			if !foundSourceFailure || !foundSourceNameDiagnostic {
				t.Fatalf("fixture must independently exercise strict relation proof and soft source-name checks: %+v", baseline)
			}
			if mode == "source_lane_excluded" {
				rm = sourceOracleScopeRequest(true)
				mut.SetRequestModel(rm)
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			} else {
				for i := range doc.Blocks {
					doc.Blocks[i].ClaimUses = []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation}}
				}
			}
			before, _ := json.Marshal(doc)
			denials := types.NewTypedDenialSet()
			bus.TypedDenials = denials
			got := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, denialStubOracle{}, denials, bus)
			for _, violation := range got {
				if sourceOracleScopeViolation(violation.Kind) {
					t.Errorf("source-name check ignored its external scope: %+v", violation)
				}
			}
			if !reflect.DeepEqual(strictRelations(got), wantRelations) {
				t.Fatalf("source-name applicability changed independent relation authority: got=%+v want=%+v", strictRelations(got), wantRelations)
			}
			for _, violation := range strictRelations(got) {
				if violation.Kind == types.ViolDiagramCallEdgeUnproven && violation.ClusterKey != blockClusterKey("unproven-source", "diagram_call_edge_evidence") {
					t.Errorf("valid report-local temporal block acquired a source relation violation: %+v", violation)
				}
			}
			if len(denials.AdvisoryAnswerSurfaceSymbolTokens()) != 0 {
				t.Errorf("scope exclusion stamped source-name advisories: %v", denials.AdvisoryAnswerSurfaceSymbolTokens())
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("relation validation rewrote model-authored blocks or edge anchors")
			}
		})
	}
}
