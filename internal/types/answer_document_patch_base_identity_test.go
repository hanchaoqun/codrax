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
