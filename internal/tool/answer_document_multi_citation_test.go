package tool

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeEmitAnswerBlockCanonicalizesExplicitMultipleCitationRefs(t *testing.T) {
	var wire emitAnswerBlockV2
	if err := json.Unmarshal([]byte(`{
		"id":"evidence","kind":"ordered_list","items":[
			{"id":"row","label":"explorer","citation_refs":[0,"1",1,-1]}
		]
	}`), &wire); err != nil {
		t.Fatal(err)
	}
	block, err := NormalizeEmitAnswerBlock(wire, "blocks[0]")
	if err != nil {
		t.Fatal(err)
	}
	if len(block.Items) != 1 {
		t.Fatalf("items=%+v", block.Items)
	}
	item := block.Items[0]
	if item.CitationRef != 0 || !slices.Equal(item.CitationRefs, []int{1}) {
		t.Fatalf("multiple refs not canonicalized primary-first: %+v", item)
	}
}

func TestEmitAnswerDocumentV2PreservesAndRemapsExplicitMultipleCitationRefs(t *testing.T) {
	bus := newV2TestBusContext()
	tool := &EmitAnswerDocument{}
	raw := json.RawMessage(`{
		"blocks":[{"id":"evidence","kind":"ordered_list","items":[
			{"id":"row","label":"explorer","text":"registration and Name evidence","citation_refs":[0,2]}
		]}],
		"citations":[
			{"file":"register.go","line":10},
			{"file":"unused.go","line":20},
			{"file":"name.go","line":30}
		]
	}`)
	res, err := tool.Execute(bus, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("emit failed: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].Items) != 1 {
		t.Fatalf("document missing: %+v", doc)
	}
	if got := doc.Citations; len(got) != 2 || got[0].File != "register.go" || got[1].File != "name.go" {
		t.Fatalf("multiple-ref citation pool was pruned incorrectly: %+v", got)
	}
	item := doc.Blocks[0].Items[0]
	if got := types.AnswerBlockItemCitationRefs(item); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("remapped item refs=%v item=%+v", got, item)
	}
	if !strings.Contains(res.Summary, "citations_pruned_unused=1") {
		t.Fatalf("missing citation ledger: %s", res.Summary)
	}
}

func TestRemapPatchBlockCitationRefsRemapsEveryExplicitAnchor(t *testing.T) {
	blocks := []types.AnswerBlock{{
		ID: "evidence", Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{
			ID: "row", Label: "combined", CitationRef: 0, CitationRefs: []int{2},
		}},
	}}
	if changed := remapPatchBlockCitationRefs(blocks, map[int]int{0: 3, 2: 4}); changed != 2 {
		t.Fatalf("changed=%d blocks=%+v", changed, blocks)
	}
	if got := types.AnswerBlockItemCitationRefs(blocks[0].Items[0]); !slices.Equal(got, []int{3, 4}) {
		t.Fatalf("refs=%v", got)
	}
}

func TestEmitAnswerDocumentPatchPreservesMultipleCitationRefsEndToEnd(t *testing.T) {
	bus := newPatchTestBusContext()
	raw := json.RawMessage(`{
		"unchanged_block_ids":["s1"],
		"replace_blocks":[{
			"id":"list1","kind":"ordered_list",
			"claim_uses":[{"claim_form":"call_edge"}],
			"items":[{
				"id":"i1","label":"A","text":"definition and call evidence",
				"citation_ref":0,"citation_refs":[1]
			}]
		}],
		"append_citations":[{"file":"call.go","line":20}]
	}`)
	res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("patch failed: %+v", res)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) != 2 {
		t.Fatalf("patched document missing: %+v", doc)
	}
	if got := types.AnswerBlockItemCitationRefs(doc.Blocks[1].Items[0]); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("patch lost a citation anchor: %v", got)
	}
}

func TestNormalizeUnusedCitationPoolDoesNotInferSecondaryAnchor(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID: "one", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "row", Label: "visible", CitationRef: 0}},
		}},
		Citations: []types.Citation{{File: "used.go", Line: 1}, {File: "unbound.go", Line: 2}},
	}
	normalizeUnusedCitationPoolEntries(doc, nil)
	if got := types.AnswerBlockItemCitationRefs(doc.Blocks[0].Items[0]); !slices.Equal(got, []int{0}) {
		t.Fatalf("system invented a secondary citation: %v", got)
	}
}

func TestAnswerDocumentFullAndPatchSchemasExposeSameMultipleCitationCarrier(t *testing.T) {
	full := string((&EmitAnswerDocument{}).Parameters())
	patch := string(BuildAnswerDocumentPatchParametersFor(nil))
	for name, schema := range map[string]string{"full": full, "patch": patch} {
		if !strings.Contains(schema, `"citation_refs"`) {
			t.Fatalf("%s schema omits item citation_refs", name)
		}
	}
}
