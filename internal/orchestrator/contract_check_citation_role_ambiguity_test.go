package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The model's original two mismatched claim_use annotations are not a license
// to replace an independently correct value citation. Correcting those
// annotations still leaves a legitimate literal form on the environment row.
func TestB1642ActualEmitOwnCitationSurvivesMixedRoleCandidates(t *testing.T) {
	for _, corrected := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			name := "original_metadata"
			if corrected {
				name = "correct_metadata"
			}
			if reverse {
				name += "/properties_first"
			} else {
				name += "/lookup_first"
			}
			t.Run(name, func(t *testing.T) {
				repo, err := filepath.Abs("../../eval/fixtures/java-layered-service")
				if err != nil {
					t.Fatal(err)
				}
				const java = "src/main/java/com/clinic/config/ClinicConfig.java"
				const props = "src/main/resources/application.properties"
				mut := types.NewMutableState("configuration layers")
				mut.SetRepoRoot(repo)
				for _, path := range []string{java, props} {
					data, err := os.ReadFile(filepath.Join(repo, path))
					if err != nil {
						t.Fatal(err)
					}
					mut.RecordPreReadSource(path, strings.Split(string(data), "\n"))
				}
				ev := func(id, label, file string, line int, anchor types.AnchorKind, role types.EvidenceDiagramRole, snippet string) types.EvidenceItem {
					return types.EvidenceItem{ID: id, Kind: types.EvidenceDirect, Source: file, LineStart: line, LineEnd: line,
						Scope: types.ScopeLine, AnchorKind: anchor, AnchorSymbol: label, Subject: label,
						DiagramRole: role, Snippet: snippet, GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}
				}
				lookup := ev("lookup", "clinic.max-visits", java, 21, types.AnchorStringLiteral, "", `String fromFile = props.getProperty("clinic.max-visits");`)
				property := ev("property", "clinic.max-visits", props, 1, types.AnchorAssignment, types.EvidenceDiagramRoleConfig, "clinic.max-visits=50")
				env := ev("env", "CLINIC_MAX_VISITS", java, 29, types.AnchorStringLiteral, "", `String fromEnv = System.getenv("CLINIC_MAX_VISITS");`)
				def := ev("default", "DEFAULT_MAX_VISITS", java, 13, types.AnchorDefinition, "", "public static final int DEFAULT_MAX_VISITS = 20;")
				pool := []types.EvidenceItem{lookup, property, env, def}
				if reverse {
					pool[0], pool[1] = pool[1], pool[0]
				}
				mut.AppendEvidence(pool)
				bus := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mut, EvidenceItems: pool,
					AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioConfigTrace,
						AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqConfigMapping)}}}}
				claims := []types.RenderedClaimUse{{EvidenceID: "default", ClaimForm: types.ClaimPrecedenceRole}, {EvidenceID: "property", ClaimForm: types.ClaimLiteralValueFact}, {EvidenceID: "env", ClaimForm: types.ClaimLiteralValueFact}}
				if corrected {
					claims[0].ClaimForm = types.ClaimDefinitionFact
					claims[1].ClaimForm = types.ClaimPrecedenceRole
				}
				doc := types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
					{ID: "summary", Kind: types.BlockSummary, Text: "The default is 20, the packaged value is 50, and a nonblank environment value overrides it."},
					{ID: "layers", Kind: types.BlockTable, SurfaceRole: types.SurfacePrincipal, FacetIDs: []string{string(types.FacetConfigPrecedenceRole)},
						ClaimUses: claims, Columns: []string{"Setting", "Value / source"}, Items: []types.AnswerBlockItem{
							{ID: "default", Label: "DEFAULT_MAX_VISITS", Cells: []string{"20"}, CitationRef: 3},
							{ID: "property", Label: "clinic.max-visits", Cells: []string{"50 from application.properties"}, CitationRef: 0},
							{ID: "env", Label: "CLINIC_MAX_VISITS", Cells: []string{"Runtime value is not observed"}, CitationRef: 2}}},
					{ID: "boundary", Kind: types.BlockCaveat, Text: "The environment lookup is not a measured runtime value."}},
					Citations: []types.Citation{{File: props, Line: 1, Quote: property.Snippet}, {File: java, Line: 21, Quote: lookup.Snippet}, {File: java, Line: 29, Quote: env.Snippet}, {File: java, Line: 13, Quote: def.Snippet}}}
				raw, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				// The internal legacy int uses omitempty; the actual model call
				// explicitly published citation_ref:0. Preserve that wire input.
				var wire map[string]any
				if err := json.Unmarshal(raw, &wire); err != nil {
					t.Fatal(err)
				}
				wire["blocks"].([]any)[1].(map[string]any)["items"].([]any)[1].(map[string]any)["citation_ref"] = 0
				raw, err = json.Marshal(wire)
				if err != nil {
					t.Fatal(err)
				}
				result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
				if err != nil || !result.Success {
					t.Fatalf("real emit failed before role check: %v %+v", err, result)
				}
				got := mut.AnswerDocumentV2()
				if got == nil || len(got.Blocks) < 2 || len(got.Blocks[1].Items) != 3 {
					t.Fatal("accepted original table missing")
				}
				item := got.Blocks[1].Items[1]
				if item.Label != "clinic.max-visits" || !reflect.DeepEqual(item.Cells, doc.Blocks[1].Items[1].Cells) {
					t.Fatal("model value row changed")
				}
				refs := types.AnswerBlockItemCitationRefs(item)
				if len(refs) != 1 || got.Citations[refs[0]].File != props || got.Citations[refs[0]].Line != 1 {
					t.Fatalf("correct selected source changed: %+v refs=%v", got.Citations, refs)
				}
				view := types.BuildAnswerSemanticViewForBusContext(bus)
				if view == nil || view.Family != types.QFConfigPrecedence {
					t.Fatalf("wrong real family: %+v", view)
				}
				before, _ := json.Marshal(got)
				for _, v := range runV2BlockOraclesWithOracleContext(context.Background(), got, view, mut, nil, nil, bus) {
					if v.SuspectedRoot.IRField == "answer_item_citation_role" {
						t.Errorf("valid selected value source misdirected by pool order: %s; %s", v.Detail, v.Repair)
					}
				}
				after, _ := json.Marshal(mut.AnswerDocumentV2())
				if string(before) != string(after) {
					t.Fatal("role check mutated accepted answer")
				}
			})
		}
	}
}

func TestB1642PostCitationRoleUniqueWrongAndAmbiguousSources(t *testing.T) {
	lookup := types.EvidenceItem{ID: "lookup", Source: "Config.java", LineStart: 21, AnchorKind: types.AnchorStringLiteral, Subject: "config.key", GroundingStatus: types.GroundingGrounded}
	property := types.EvidenceItem{ID: "property", Source: "application.properties", LineStart: 1, AnchorKind: types.AnchorPrecedence, Subject: "config.key", GroundingStatus: types.GroundingGrounded}
	wrong := types.EvidenceItem{ID: "definition", Source: "Config.java", LineStart: 13, AnchorKind: types.AnchorDefinition, Subject: "Container", GroundingStatus: types.GroundingGrounded}
	for _, tc := range []struct {
		name string
		pool []types.EvidenceItem
		refs []int
		want int
	}{
		{"unique_wrong", []types.EvidenceItem{lookup, wrong}, []int{0}, 1},
		{"ambiguous_wrong_lookup_first", []types.EvidenceItem{lookup, property, wrong}, []int{0}, 0},
		{"ambiguous_wrong_property_first", []types.EvidenceItem{property, lookup, wrong}, []int{0}, 0},
		{"second_current_ref_valid", []types.EvidenceItem{lookup, wrong}, []int{0, 1}, 0},
		{"multiple_wrong_refs", []types.EvidenceItem{lookup, wrong}, []int{0, 2}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("role candidates")
			mut.AppendEvidence(tc.pool)
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "facts", Kind: types.BlockTable, ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimLiteralValueFact}, {ClaimForm: types.ClaimPrecedenceRole}},
				Items: []types.AnswerBlockItem{{ID: "row", Label: "config.key", CitationRefs: tc.refs}}}}, Citations: []types.Citation{{File: wrong.Source, Line: 13}, {File: lookup.Source, Line: 21}, {File: "other.go", Line: 4}}}
			before, _ := json.Marshal(doc)
			got := validateCallChainItemCitationRoleAlignment(doc, nil, mut)
			if len(got) != tc.want {
				t.Fatalf("post role violations=%+v want=%d", got, tc.want)
			}
			if tc.want > 0 && !strings.Contains(got[0].Repair, "Config.java:21") {
				t.Fatal("unique correct target no longer disclosed")
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("post validator mutated model document")
			}
		})
	}
}
