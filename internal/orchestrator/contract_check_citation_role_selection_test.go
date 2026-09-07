package orchestrator

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// r1033's emit gate already accepted the model-selected definition form.
// The regression was the post-emit gate adding the view's alternative import
// form and treating a class label as an assertion of its unique import edge.
func TestB1606ActualEmitDefinitionKeepsClaimSelectionThroughPostCheck(t *testing.T) {
	repo, err := filepath.Abs("../../eval/fixtures/cpp-sink-hierarchy")
	if err != nil {
		t.Fatal(err)
	}
	mut := types.NewMutableState("Sink implementations")
	mut.SetRepoRoot(repo)
	bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		}}}
	for _, row := range []struct {
		name, file, definitionID, importID string
		definitionLine, importLine         int
	}{
		{"ConsoleSink", "console_sink.hpp", "ev-33a12d0dca17e6c0", "ev-79ebd22d4da0a470", 8, 3},
		{"FileSink", "file_sink.hpp", "ev-c11a6e039a6fecbe", "ev-dcdd0250bc521106", 10, 4},
	} {
		file := "include/logx/" + row.file
		bus.EvidenceItems = append(bus.EvidenceItems,
			types.EvidenceItem{ID: row.definitionID, Kind: types.EvidenceDirect, Source: file,
				LineStart: row.definitionLine, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
				AnchorSymbol: row.name, Subject: row.name, Snippet: "class " + row.name + " : public Sink {",
				GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
			types.EvidenceItem{ID: row.importID, Kind: types.EvidenceRelationship, Source: file,
				LineStart: row.importLine, Scope: types.ScopeLine, AnchorKind: types.AnchorImport,
				AnchorSymbol: "logx/sink.hpp", Subject: row.name, Predicate: "声明继承", Object: "Sink",
				Snippet: "#include \"logx/sink.hpp\"", GroundingStatus: types.GroundingGrounded,
				Origin: types.ClaimOriginCurrentRepo})
	}
	mut.AppendEvidence(bus.EvidenceItems)
	raw := json.RawMessage(`{"blocks":[
		{"id":"summary","kind":"summary","text":"ConsoleSink and FileSink directly inherit Sink."},
		{"id":"members","kind":"ordered_list","surface_role":"principal","facet_ids":["enumeration_item"],
		 "claim_uses":[{"claim_form":"definition_fact","facet_id":"enumeration_item"}],"items":[
		 {"id":"console","label":"ConsoleSink","text":"直接继承 Sink；定义于 include/logx/console_sink.hpp:8。","evidence_ids":["ev-33a12d0dca17e6c0"]},
		 {"id":"file","label":"FileSink","text":"直接继承 Sink；定义于 include/logx/file_sink.hpp:10。","evidence_ids":["ev-c11a6e039a6fecbe"]}]}],
		 "citations":[{"file":"include/logx/console_sink.hpp","line":8,"quote":"class ConsoleSink : public Sink {"},
		 {"file":"include/logx/file_sink.hpp","line":10,"quote":"class FileSink : public Sink {"}]}`)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("the existing emit lane must accept the definition-owned draft: result=%+v err=%v", result, err)
	}
	got := mut.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].Items) != 2 {
		t.Fatalf("accepted draft missing: %+v", got)
	}
	for i, want := range []string{"ev-33a12d0dca17e6c0", "ev-c11a6e039a6fecbe"} {
		if !reflect.DeepEqual(got.Blocks[1].Items[i].EvidenceIDs, []string{want}) {
			t.Fatalf("selected evidence changed: %+v", got.Blocks[1].Items[i])
		}
	}
	before, _ := json.Marshal(got)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if view == nil || view.Family != types.QFEnumeration {
		t.Fatalf("fixture must use the real enumeration view: %+v", view)
	}
	if violations := validateCallChainItemCitationRoleAlignment(got, view, mut); len(violations) != 0 {
		t.Fatalf("post-emit invented a relation from an alternative form after accepted definition emit: %+v", violations)
	}
	for _, violation := range runV2BlockOraclesWithOracleContext(context.Background(), got, view, mut, nil, nil, bus) {
		if violation.SuspectedRoot.IRField == "answer_item_citation_role" {
			t.Fatalf("production oracle suite disagrees with the accepted claim selection: %+v", violation)
		}
	}
	after, _ := json.Marshal(mut.AnswerDocumentV2())
	if string(before) != string(after) {
		t.Fatal("claim-role validation changed the persisted model answer")
	}
}

func TestB1606CitationRoleSelectionPreservesExplicitAndLegacyRelations(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable} {
		for _, tc := range []struct {
			name     string
			claims   []types.RenderedClaimUse
			text     string
			citeEdge bool
			want     int
		}{
			{"definition_not_optional_import", []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}}, "Class definition.", false, 0},
			{"explicit_import_endpoint", []types.RenderedClaimUse{{ClaimForm: types.ClaimImportEdge}}, "", false, 1},
			{"explicit_import_arrow", []types.RenderedClaimUse{{ClaimForm: types.ClaimImportEdge}}, "Widget -> Base", false, 1},
			{"explicit_import_correct", []types.RenderedClaimUse{{ClaimForm: types.ClaimImportEdge}}, "Widget -> Base", true, 0},
			{"mixed_explicit_forms", []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}, {ClaimForm: types.ClaimImportEdge}}, "", false, 1},
			{"legacy_no_claims_endpoint", nil, "", false, 1},
			{"legacy_no_claims_arrow", nil, "Widget -> Base", false, 1},
			{"legacy_correct", nil, "", true, 0},
		} {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				mut := types.NewMutableState("role selection")
				mut.AppendEvidence([]types.EvidenceItem{
					{ID: "def", Kind: types.EvidenceDirect, Source: "widget.h", LineStart: 10, AnchorKind: types.AnchorDefinition, Subject: "Widget", GroundingStatus: types.GroundingGrounded},
					{ID: "import", Kind: types.EvidenceRelationship, Source: "widget.h", LineStart: 3, AnchorKind: types.AnchorImport, Subject: "Widget", Object: "Base", GroundingStatus: types.GroundingGrounded},
				})
				line := 10
				if tc.citeEdge {
					line = 3
				}
				doc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "widget.h", Line: line}},
					Blocks: []types.AnswerBlock{{ID: "members", Kind: kind, FacetIDs: []string{string(types.FacetEnumerationItem)},
						ClaimUses: tc.claims, Items: []types.AnswerBlockItem{{ID: "widget", Label: "Widget", Text: tc.text, CitationRef: 0}}}}}
				view := &types.AnswerSemanticView{Family: types.QFEnumeration, OptionalBlocks: []types.BlockRequirement{{
					Kind: kind, FacetIDs: []string{string(types.FacetEnumerationItem)}, AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimImportEdge},
				}}}
				violations := validateCallChainItemCitationRoleAlignment(doc, view, mut)
				if len(violations) != tc.want {
					t.Fatalf("violations=%+v want=%d", violations, tc.want)
				}
				if tc.want > 0 && (!strings.Contains(violations[0].Detail, "Widget -> Base") || !strings.Contains(violations[0].Repair, "widget.h:3")) {
					t.Fatalf("existing exact relation/citation check weakened: %+v", violations)
				}
			})
		}
	}
}
