package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB1620EnumerationExplanationKeepsSeparateSources(t *testing.T) {
	rm := &RequestModel{Intent: IntentEnumerate, Predicates: SemanticPredicates{IsCategoryEnumeration: true}}
	for _, form := range []ClaimForm{ClaimDefinitionFact, ClaimCallEdge} {
		t.Run(string(form), func(t *testing.T) {
			ev := EvidenceItem{ID: "ev-handler", Kind: EvidenceDirect, Subject: "Handler", AnchorSymbol: "Handler", AnchorKind: AnchorDefinition,
				Source: "src/Handler.java", LineStart: 10, Scope: ScopeLine, GroundingStatus: GroundingGrounded, Summary: "source-grounded explanation"}
			if form == ClaimCallEdge {
				ev.AnchorKind = AnchorCall
				ev.Predicate = "calls"
				ev.Object = "run"
			}
			plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{Kind: AnswerAggregateMemberSet, Role: AnswerAggregateRolePrincipalAnswer,
				Label: "handlers", Value: "1", Members: []string{"Handler"}, SupportRefs: []string{"src/Handler.java:10"},
				MemberNotes: []string{"model proposes a broader business interpretation"}}}, SurfaceEvidence: []EvidenceItem{ev}}
			before, _ := json.Marshal(plan)
			sets := CompileEnumerationDisplaySets(rm, plan)
			if len(sets) != 1 || len(sets[0].Rows) != 1 {
				t.Fatalf("identity roster lost: %+v", sets)
			}
			row := sets[0].Rows[0]
			if row.Location != "src/Handler.java:10" || !row.HasCitation || row.ClaimForm != form {
				t.Fatalf("declaration/operation proof changed: %+v", row)
			}
			var evidencePart, modelPart bool
			for _, part := range row.NoteParts {
				switch part.Origin {
				case EnumerationDisplayNoteEvidence:
					evidencePart = part.Text == ev.Summary && part.EvidenceID == ev.ID && part.ClaimForm == form && part.SupportRef == row.Location
				case EnumerationDisplayNoteAggregate:
					modelPart = part.Text == plan.StableAggregateFacts[0].MemberNotes[0] && part.ClaimForm == ClaimUnknown && part.EvidenceID == "" && part.SupportRef == row.Location
				}
			}
			if !evidencePart || !modelPart {
				t.Errorf("one source location must not launder all note parts into the row's claim: %+v", row.NoteParts)
			}
			if !strings.Contains(row.Note, ev.Summary) || !strings.Contains(row.Note, plan.StableAggregateFacts[0].MemberNotes[0]) {
				t.Error("explanatory business content was removed")
			}
			after, _ := json.Marshal(plan)
			if string(before) != string(after) {
				t.Error("source payload was rewritten")
			}
		})
	}
}
