package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnswerBlockSystemGeneratedKindIsInternalOnly(t *testing.T) {
	doc := AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:                  "system-row-supplement",
			Kind:                BlockTable,
			Title:               "display title",
			SystemGeneratedKind: AnswerSystemGeneratedPrincipalEnumerationMissing,
		}},
	}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal answer document: %v", err)
	}
	raw := string(body)
	for _, forbidden := range []string{
		"system_generated",
		"SystemGeneratedKind",
		string(AnswerSystemGeneratedPrincipalEnumerationMissing),
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("internal system marker leaked into JSON: %q contains %q", raw, forbidden)
		}
	}
}
