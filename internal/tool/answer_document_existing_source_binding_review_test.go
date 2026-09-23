package tool_test

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
)

func TestExistingSourceBindingReviewPartialIdentityDoesNotBorrowVisibleProof(t *testing.T) {
	for _, side := range []string{"from", "to"} {
		t.Run(side, func(t *testing.T) {
			bus, doc := existingSourceBindingPublicFixture(t)
			anchor := &doc.Blocks[1].EdgeAnchors[0]
			if side == "from" {
				anchor.FromIdentity = "other.Run"
			} else {
				anchor.ToIdentity = "other.RunWith"
			}
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil {
				t.Fatal(err)
			}
			if result.Success {
				t.Fatalf("partial contradictory identity must not borrow proof from visible labels: side=%s anchor=%+v", side, anchor)
			}
		})
	}
}
