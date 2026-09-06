package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestApplyPatchRejectsAmbiguousBaseBeforeOrdering(t *testing.T) {
	prev := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{
		{ID: "lead", Kind: BlockSummary, Text: "first account"},
		{ID: "detail", Kind: BlockSection, Text: "first details"},
		{ID: "lead", Kind: BlockSummary, Text: "revised account"},
		{ID: "detail", Kind: BlockSection, Text: "revised details"},
	}}
	before := cloneAnswerDocumentV2(prev)
	for _, patch := range []*AnswerDocumentV2Patch{
		{ModelBlockOrder: []string{"lead", "detail"}},
		{ReplaceBlocks: []AnswerBlock{{ID: "lead", Kind: BlockSummary, Text: "selected account"}}},
		{RemoveBlockIDs: []string{"lead"}},
	} {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("ambiguous block ids crashed patch: %v", p)
				}
			}()
			out, err := ApplyAnswerDocumentV2Patch(prev, patch)
			if out != nil || err == nil || !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "emit_answer_document") {
				t.Errorf("expected recoverable full-emission error, got %+v / %v", out, err)
			}
		}()
	}
	if !reflect.DeepEqual(prev, before) {
		t.Fatal("failed patch changed retained model draft")
	}
}

func TestPatchBaseIdentityReportsAllConflictingIDsTogether(t *testing.T) {
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{ID: "lead"}, {ID: "detail"}, {ID: "lead"}, {ID: "detail"}, {ID: ""},
	}}
	err := ValidateAnswerDocumentPatchBaseIdentity(doc)
	if err == nil {
		t.Fatal("invalid identity set accepted")
	}
	for _, want := range []string{`duplicate block ID "lead"`, `duplicate block ID "detail"`, "blocks[4] has an invalid block ID"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("independent identity error was withheld: %s", err)
		}
	}
	if strings.Count(err.Error(), "use emit_answer_document") != 1 {
		t.Fatalf("one full repair instruction is sufficient: %s", err)
	}
}
