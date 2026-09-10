package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// One source line can contain several distinct relations. A row naming only
// their shared endpoint has not selected one of them. Do not manufacture that
// selection by pool order, including through the citation normalizer.
func TestB1642bActualEmitDoesNotRebindAmbiguousEndpointCitation(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		name := "left_first"
		if reverse {
			name = "right_first"
		}
		for _, face := range []string{"full", "patch"} {
			for _, variant := range []string{"legacy", "known_definition", "additional_citation", "selected_evidence_id", "replace_citation_pool", "append_citation_pool"} {
				t.Run(name+"/"+face+"/"+variant, func(t *testing.T) {
					repo := t.TempDir()
					const source = "flow.go"
					const code = "package flow\nfunc Dispatch() {\n    Left(); Right()\n}\nfunc Left() {}\nfunc Right() {}\n"
					if err := os.WriteFile(filepath.Join(repo, source), []byte(code), 0600); err != nil {
						t.Fatal(err)
					}
					def := types.EvidenceItem{ID: "definition", Kind: types.EvidenceDirect, Source: source,
						LineStart: 2, LineEnd: 2, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
						AnchorSymbol: "Dispatch", Subject: "Dispatch", Snippet: "func Dispatch() {",
						GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}
					left := types.EvidenceItem{ID: "left", Kind: types.EvidenceRelationship, Source: source,
						LineStart: 3, LineEnd: 3, Scope: types.ScopeLine, AnchorKind: types.AnchorCall,
						AnchorSymbol: "Left", Subject: "Dispatch", Object: "Left", Predicate: "calls", Snippet: "Left(); Right()",
						GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}
					right := left
					right.ID, right.AnchorSymbol, right.Object = "right", "Right", "Right"
					// The model read the declaration, but the accepted evidence pool
					// records its two calls, not a unique definition-fact ID. This is
					// a legal legacy file:line citation, not a fabricated source line.
					pool := []types.EvidenceItem{left, right}
					if reverse {
						pool[0], pool[1] = pool[1], pool[0]
					}
					if variant == "known_definition" || variant == "selected_evidence_id" {
						pool = append(pool, def)
					}
					mut := types.NewMutableState("explain Dispatch responsibilities")
					mut.SetRepoRoot(repo)
					mut.RecordPreReadSource(source, strings.Split(code, "\n"))
					mut.AppendEvidence(pool)
					bus := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: mut, EvidenceItems: pool,
						AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
					doc := types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
						{ID: "summary", Kind: types.BlockSummary, Text: "Dispatch coordinates two operations; the individual relations remain separate."},
						{ID: "roles", Kind: types.BlockTable, SurfaceRole: types.SurfacePrincipal,
							ClaimUses: []types.RenderedClaimUse{{EvidenceID: "left", ClaimForm: types.ClaimCallEdge}, {EvidenceID: "right", ClaimForm: types.ClaimCallEdge}},
							Columns:   []string{"Owner", "Responsibility"}, Items: []types.AnswerBlockItem{{ID: "owner", Label: "Dispatch", Cells: []string{"Coordinates the operations."}, CitationRef: 0}}},
					}, Citations: []types.Citation{{File: source, Line: 2, LineEnd: 2, Quote: def.Snippet}, {File: source, Line: 3, LineEnd: 3, Quote: left.Snippet}}}
					if variant == "additional_citation" {
						doc.Blocks[1].Items[0].CitationRefs = []int{0, 1}
					}
					if variant == "selected_evidence_id" {
						doc.Blocks[1].Items[0].EvidenceIDs = []string{"definition"}
					}
					if variant == "append_citation_pool" && face == "patch" {
						doc.Citations = doc.Citations[:1]
					}
					item := doc.Blocks[1].Items[0]
					view := types.BuildAnswerSemanticViewForBusContext(bus)
					if hints := preCheckCallChainItemCitationRoleAlignment(&doc, view, bus); len(hints) != 0 {
						t.Errorf("ambiguous endpoint cannot select a directed repair hint: %+v", hints)
					}
					if cit, ok := preEmitUniqueTypedClaimRoleCitationForItemWithContext(newPreEmitCheckContext(bus), item, []types.ClaimForm{types.ClaimCallEdge}); ok {
						t.Errorf("same-location sibling roles were treated as one automatic repair: %+v", cit)
					}
					raw, err := json.Marshal(doc)
					if err != nil {
						t.Fatal(err)
					}
					var wire map[string]any
					if err := json.Unmarshal(raw, &wire); err != nil {
						t.Fatal(err)
					}
					wire["blocks"].([]any)[1].(map[string]any)["items"].([]any)[0].(map[string]any)["citation_ref"] = 0
					if variant == "selected_evidence_id" {
						delete(wire["blocks"].([]any)[1].(map[string]any)["items"].([]any)[0].(map[string]any), "citation_ref")
					}
					raw, _ = json.Marshal(wire)
					var result types.ToolResult
					if face == "patch" {
						mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &doc)
						patch := map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []any{wire["blocks"].([]any)[1]}}
						if variant == "replace_citation_pool" {
							patch["replace_citations"] = doc.Citations
						}
						if variant == "append_citation_pool" {
							patch["append_citations"] = []types.Citation{{File: source, Line: 3, LineEnd: 3, Quote: left.Snippet}}
						}
						raw, _ = json.Marshal(patch)
						result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
					} else {
						result, err = (&EmitAnswerDocument{}).Execute(bus, raw)
					}
					if err != nil || !result.Success {
						t.Fatalf("public emission failed independently of endpoint candidate selection: %v %+v", err, result)
					}
					got := mut.AnswerDocumentV2()
					if got == nil || len(got.Blocks) != 2 || len(got.Blocks[1].Items) != 1 {
						t.Fatalf("model document missing: %+v", got)
					}
					actual := got.Blocks[1].Items[0]
					refs := types.AnswerBlockItemCitationRefs(actual)
					wantRefs := 1
					if variant == "additional_citation" {
						wantRefs = 2
					}
					if len(refs) != wantRefs || got.Citations[refs[0]].File != source || got.Citations[refs[0]].Line != 2 {
						t.Errorf("endpoint-only model citation was replaced by a sibling relation: refs=%v citations=%+v", refs, got.Citations)
					}
					if variant == "additional_citation" && (len(refs) != 2 || got.Citations[refs[1]].Line != 3) {
						t.Fatal("the model's additional citation was lost")
					}
					if variant == "selected_evidence_id" && !reflect.DeepEqual(actual.EvidenceIDs, item.EvidenceIDs) {
						t.Fatal("the model's exact evidence identity was replaced")
					}
					if actual.Label != item.Label || !reflect.DeepEqual(actual.Cells, item.Cells) || got.Blocks[0].Text != doc.Blocks[0].Text || !reflect.DeepEqual(got.Blocks[1].ClaimUses, doc.Blocks[1].ClaimUses) {
						t.Fatal("candidate disambiguation rewrote the model content or relation declarations")
					}
				})
			}
		}
	}
}

func TestB1642bWeakRepairGuardKeepsExplicitRoleAndCellBoundaries(t *testing.T) {
	left := types.EvidenceItem{ID: "left", Kind: types.EvidenceRelationship, Source: "flow.go", LineStart: 3, LineEnd: 3,
		AnchorKind: types.AnchorCall, Subject: "Dispatch", Object: "Left", GroundingStatus: types.GroundingGrounded}
	right := left
	right.ID, right.Object = "right", "Right"
	for _, tc := range []struct {
		name, label, text string
		cells             []string
		pool              []types.EvidenceItem
		wantRef           int
	}{
		{"cell_endpoint_ambiguous", "", "", []string{"Dispatch", "Coordinates operations"}, []types.EvidenceItem{left, right}, 0},
		{"explicit_arrow_selects_role", "Dispatch", "Dispatch -> Left", nil, []types.EvidenceItem{left, right}, 1},
		{"unique_endpoint_still_repaired", "Dispatch", "", nil, []types.EvidenceItem{left}, 1},
		{"exact_duplicate_still_repaired", "Dispatch", "", nil, []types.EvidenceItem{left, left}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("citation repair boundaries")
			mut.AppendEvidence(tc.pool)
			bus := &types.BusContext{Mutable: mut}
			item := types.AnswerBlockItem{ID: "owner", Label: tc.label, Text: tc.text, Cells: tc.cells, CitationRef: 0}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "roles", Kind: types.BlockTable,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}}, Items: []types.AnswerBlockItem{item}}},
				Citations: []types.Citation{{File: "flow.go", Line: 2}, {File: "flow.go", Line: 3, LineEnd: 3}}}
			normalizeItemCitationRefsByUniquePreEmitCandidateWithContext(doc, nil, bus, newPreEmitCheckContext(bus))
			got := doc.Blocks[0].Items[0]
			if got.CitationRef != tc.wantRef || got.Label != item.Label || got.Text != item.Text || !reflect.DeepEqual(got.Cells, item.Cells) {
				t.Fatalf("precise role/cell selection changed: %+v want citation %d", got, tc.wantRef)
			}
		})
	}
}
