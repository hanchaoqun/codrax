package types

import (
	"reflect"
	"strings"
	"testing"
)

const b1620UnprovenMemberNote = "surface=foreign func Widget; package claimed.wrong; investigate the external implementation"

func b1620AssertCandidateNoteRetained(t *testing.T, note string) {
	t.Helper()
	// The existing note formatter may normalize separators; all original
	// claims and navigation text must remain, not their punctuation bytes.
	for _, part := range strings.Split(b1620UnprovenMemberNote, "; ") {
		if !strings.Contains(note, part) {
			t.Errorf("candidate explanation/navigation %q was lost: %q", part, note)
		}
	}
}

func b1620AssertCandidateFieldsRetained(t *testing.T, row EnumerationDisplayRow) {
	t.Helper()
	if !strings.Contains(strings.Join(row.CandidateSurfaceTerms, "|"), "foreign func Widget") {
		t.Errorf("candidate declaration marker was lost: %q", row.CandidateSurfaceTerms)
	}
	if len(row.CandidateAttributes) != 1 || row.CandidateAttributes[0].Role != AnswerCandidateRolePackage ||
		row.CandidateAttributes[0].Name != "claimed.wrong" || row.CandidateAttributes[0].Source != "" ||
		row.CandidateAttributes[0].Line != 0 || row.CandidateAttributes[0].Location != "" {
		t.Errorf("candidate package must not borrow the member's coordinate as a package declaration: %+v", row.CandidateAttributes)
	}
	var hasModelNoteSource bool
	for _, part := range row.NoteParts {
		if part.Origin == EnumerationDisplayNoteAggregate && part.Text == b1620UnprovenMemberNote {
			hasModelNoteSource = part.SupportRef == "src/Widget.cj:12" && part.ClaimForm == ClaimUnknown && part.EvidenceID == ""
		}
	}
	if row.Location != "src/Widget.cj:12" || !hasModelNoteSource {
		t.Errorf("candidate attribution must remain on the row/note without pretending to prove its value: row=%q notes=%+v", row.Location, row.NoteParts)
	}
}

func b1620FieldAuthorityPlan(note string) (*RequestModel, *AnswerSurfacePlan) {
	rm := &RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{IsCategoryEnumeration: true},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
			RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation, SourceInventoryFieldPackage},
			Confidence:        0.95,
		},
	}
	plan := &AnswerSurfacePlan{StableAggregateFacts: []AnswerAggregateFact{{
		Kind: AnswerAggregateMemberSet, Label: "declarations", Value: "1",
		Role: AnswerAggregateRolePrincipalAnswer, Members: []string{"Widget"},
		SupportRefs: []string{"Widget @ src/Widget.cj:12"}, MemberNotes: []string{note},
	}}}
	return rm, plan
}

func b1620FieldAuthorityRow(t *testing.T, rm *RequestModel, plan *AnswerSurfacePlan) EnumerationDisplayRow {
	t.Helper()
	sets := CompileEnumerationDisplaySets(rm, plan)
	if len(sets) != 1 || len(sets[0].Rows) != 1 {
		t.Fatalf("actual row compilation must preserve the one-member set: %+v", sets)
	}
	row := sets[0].Rows[0]
	if row.DisplayLabel != "Widget" || row.Source != "src/Widget.cj" || row.LineStart != 12 || !row.HasCitation {
		t.Fatalf("declaration identity/location changed: %+v", row)
	}
	return row
}

func b1620FieldAuthorityDeclaration(source, pkg string) EvidenceItem {
	return EvidenceItem{
		ID: "decl-" + source, Subject: "Widget", Object: pkg, Source: source,
		LineStart: 12, AnchorKind: AnchorDefinition, AnchorSymbol: "Widget",
		GroundingStatus: GroundingGrounded, Scope: ScopeLine,
	}
}

func b1620FieldAuthorityInventory(source string) SourceInventoryObservation {
	return SourceInventoryObservation{
		Active: true, Complete: true, Scopes: []string{"src"},
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleType, Complete: true, Count: 1,
			Members: []SourceInventoryObservationMember{{
				Name: "Widget", File: source, Line: 12,
				SurfaceTerms:  []string{"public class", "public class Widget"},
				CoverageState: SourceInventoryCoverageObserved,
				Attributes: []SourceInventoryObservationAttribute{{
					Name: "actual.scope", Role: AnswerCandidateRolePackage, File: source, Line: 3,
					CoverageState: SourceInventoryCoverageObserved,
				}},
			}},
		}},
	}
}

func TestB1620MemberNotesCannotMintExactEnumerationFields(t *testing.T) {
	for _, withDefinition := range []bool{false, true} {
		name := "support_coordinate_only"
		if withDefinition {
			name = "grounded_code_definition_without_inventory"
		}
		t.Run(name, func(t *testing.T) {
			rm, plan := b1620FieldAuthorityPlan(b1620UnprovenMemberNote)
			if withDefinition {
				plan.SurfaceEvidence = []EvidenceItem{b1620FieldAuthorityDeclaration("src/Widget.cj", "")}
			}
			row := b1620FieldAuthorityRow(t, rm, plan)
			b1620AssertCandidateNoteRetained(t, row.Note)
			b1620AssertCandidateFieldsRetained(t, row)
			if strings.Contains(strings.Join(row.SurfaceTerms, "|"), "foreign func") {
				t.Errorf("member_notes minted exact declaration markers: %q", row.SurfaceTerms)
			}
			if len(row.Attributes) != 0 {
				t.Errorf("member_notes minted exact package attributes: %+v", row.Attributes)
			}
		})
	}
}

func TestB1620ExactEnumerationAttributesRemainIndependentOfMemberNotes(t *testing.T) {
	for _, sourceKind := range []string{"source_inventory", "same_file_grounded_package_declaration"} {
		t.Run(sourceKind, func(t *testing.T) {
			rm, plan := b1620FieldAuthorityPlan("")
			if sourceKind == "source_inventory" {
				plan.SourceInventoryObservation = b1620FieldAuthorityInventory("src/Widget.cj")
			} else {
				plan.SurfaceEvidence = []EvidenceItem{
					b1620FieldAuthorityDeclaration("src/Widget.cj", "actual.scope"),
					{ID: "package", Subject: "package", AnchorSymbol: "actual.scope", Source: "src/Widget.cj", LineStart: 3,
						AnchorKind: AnchorDefinition, GroundingStatus: GroundingGrounded, Scope: ScopeLine},
				}
			}
			before := b1620FieldAuthorityRow(t, rm, plan)
			if len(before.CandidateAttributes) != 0 || len(before.CandidateSurfaceTerms) != 0 {
				t.Fatalf("exact source fields must not be reclassified as model candidates: %+v/%q", before.CandidateAttributes, before.CandidateSurfaceTerms)
			}
			if len(before.Attributes) != 1 || before.Attributes[0].Name != "actual.scope" || before.Attributes[0].Location != "src/Widget.cj:3" {
				t.Fatalf("normal exact source attribute must remain supported: %+v", before.Attributes)
			}
			if sourceKind == "source_inventory" && !strings.Contains(strings.Join(before.SurfaceTerms, "|"), "public class Widget") {
				t.Fatalf("normal exact inventory markers were lost: %q", before.SurfaceTerms)
			}
			plan.StableAggregateFacts[0].MemberNotes[0] = b1620UnprovenMemberNote
			after := b1620FieldAuthorityRow(t, rm, plan)
			if !reflect.DeepEqual(before.Attributes, after.Attributes) || !reflect.DeepEqual(before.SurfaceTerms, after.SurfaceTerms) {
				t.Errorf("model notes changed exact source fields: before=%+v/%q after=%+v/%q", before.Attributes, before.SurfaceTerms, after.Attributes, after.SurfaceTerms)
			}
			b1620AssertCandidateNoteRetained(t, after.Note)
			b1620AssertCandidateFieldsRetained(t, after)
		})
	}
}

func TestB1620EnumerationAttributesCannotBorrowSameNamedMemberAcrossFiles(t *testing.T) {
	for _, sourceKind := range []string{"source_inventory", "grounded_package_declaration"} {
		t.Run(sourceKind, func(t *testing.T) {
			rm, plan := b1620FieldAuthorityPlan("")
			if sourceKind == "source_inventory" {
				plan.SourceInventoryObservation = b1620FieldAuthorityInventory("other/Widget.cj")
			} else {
				plan.SurfaceEvidence = []EvidenceItem{
					b1620FieldAuthorityDeclaration("src/Widget.cj", "actual.scope"),
					{ID: "other-package", Subject: "package", AnchorSymbol: "actual.scope", Source: "other/Widget.cj", LineStart: 3,
						AnchorKind: AnchorDefinition, GroundingStatus: GroundingGrounded, Scope: ScopeLine},
				}
			}
			row := b1620FieldAuthorityRow(t, rm, plan)
			if len(row.Attributes) != 0 || strings.Contains(strings.Join(row.SurfaceTerms, "|"), "public class") {
				t.Fatalf("same member name borrowed source attributes from another file: %+v/%q", row.Attributes, row.SurfaceTerms)
			}
		})
	}
}

func TestB1620EnumerationEvidenceNoteKeepsItsActualSource(t *testing.T) {
	for _, scenario := range []string{"same_name_other_file", "same_location_different_claim", "candidate_evidence"} {
		t.Run(scenario, func(t *testing.T) {
			rm, plan := b1620FieldAuthorityPlan("")
			ev := b1620FieldAuthorityDeclaration("other/Widget.cj", "")
			ev.ID = "selected-note"
			ev.Snippet = "public class Widget {}"
			ev.Summary = "separate source explanation"
			wantRowForm := ClaimUnknown
			if scenario == "same_location_different_claim" {
				// Existing row selection favors the exact AnchorSymbol; the
				// existing explanatory scorer favors this longer definition
				// note. Neither selection algorithm is changed by this test.
				call := EvidenceItem{ID: "row-call", Subject: "Widget", Predicate: "calls", Object: "Run",
					AnchorSymbol: "Widget", AnchorKind: AnchorCall, Source: "src/Widget.cj", LineStart: 12,
					GroundingStatus: GroundingGrounded, Scope: ScopeLine, Summary: "short"}
				ev.Source = "src/Widget.cj"
				ev.Subject = "Details"
				ev.Object = "Widget"
				ev.AnchorSymbol = "WidgetDetails"
				ev.Summary = strings.Repeat("a longer independently sourced explanation ", 12)
				plan.SurfaceEvidence = append(plan.SurfaceEvidence, call)
				wantRowForm = ClaimCallEdge
			}
			if scenario == "candidate_evidence" {
				ev.Source = "src/Widget.cj"
				ev.DerivationCandidate = true
				wantRowForm = ClaimDefinitionFact
			}
			plan.SurfaceEvidence = append(plan.SurfaceEvidence, ev)
			row := b1620FieldAuthorityRow(t, rm, plan)
			if row.ClaimForm != wantRowForm {
				t.Fatalf("fixture did not exercise the existing intended row claim: got=%s want=%s", row.ClaimForm, wantRowForm)
			}
			var found bool
			for _, part := range row.NoteParts {
				if part.Origin != EnumerationDisplayNoteEvidence {
					continue
				}
				found = true
				if part.EvidenceID != ev.ID || part.Text != strings.TrimSpace(ev.Summary) {
					t.Fatalf("fixture did not select the intended explanatory evidence: %+v", part)
				}
				wantClaim := ClaimDefinitionFact
				if ev.DerivationCandidate {
					wantClaim = ClaimUnknown
				}
				wantRef := ev.Source + ":12"
				if part.ClaimForm != wantClaim || part.SupportRef != wantRef {
					t.Errorf("explanation borrowed row proof instead of retaining its actual evidence scope: got=%+v want_claim=%s want_ref=%s", part, wantClaim, wantRef)
				}
			}
			if !found {
				t.Fatal("explanatory evidence must remain inspectable")
			}
		})
	}
}
