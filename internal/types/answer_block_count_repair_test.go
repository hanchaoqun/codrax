package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAnswerBlockCountRepairTypedDirections(t *testing.T) {
	for _, kind := range AllAnswerBlockKinds() {
		req := BlockRequirement{Kind: kind, Required: true, MinCount: 1, MaxCount: 2}
		for _, tc := range []struct {
			actual    int
			operation string
		}{{0, "add_blocks"}, {1, ""}, {2, ""}, {3, "reduce_blocks"}} {
			r := NewAnswerBlockCountRepair(req, tc.actual)
			if tc.operation == "" {
				if r != nil {
					t.Fatal("satisfied requirement acquired repair")
				}
				continue
			}
			if r == nil || r.Operation != tc.operation || !r.Valid() || !strings.Contains(r.Instruction(), string(kind)) {
				t.Fatalf("%s %d: %+v", kind, tc.actual, r)
			}
			data, _ := json.Marshal(r)
			var roundtrip AnswerBlockCountRepair
			if err := json.Unmarshal(data, &roundtrip); err != nil || !reflect.DeepEqual(r, &roundtrip) {
				t.Fatal("lost typed retry direction")
			}
		}
	}
	for _, req := range []BlockRequirement{{Kind: BlockSummary, MinCount: 1}, {Kind: "bad", Required: true, MinCount: 1}} {
		if NewAnswerBlockCountRepair(req, 0) != nil {
			t.Fatal("optional/invalid requirement became executable guidance")
		}
	}
}

func TestAnswerBlockCountRepairPreservesFacetAlternatives(t *testing.T) {
	req := BlockRequirement{Kind: BlockTable, AlternativeKinds: []AnswerBlockKind{BlockBulletList}, Required: true, MinCount: 1, FacetIDs: []string{"target"}}
	blocks := []AnswerBlock{{ID: "table", Kind: BlockTable, FacetIDs: []string{"background"}}}
	r := NewAnswerBlockCountRepair(req, CountAnswerBlocksForRequirement(blocks, req))
	if r == nil || !strings.Contains(r.Instruction(), "facet_ids=target") || !strings.Contains(r.Instruction(), "table/bullet_list") || !strings.Contains(r.Instruction(), "correct that metadata") {
		t.Fatalf("lost scope/alternatives: %+v", r)
	}
	blocks[0].FacetIDs = []string{"target"}
	if NewAnswerBlockCountRepair(req, CountAnswerBlocksForRequirement(blocks, req)) != nil {
		t.Fatal("membership repair should satisfy same requirement")
	}
}
