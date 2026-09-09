package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1620NoteTeachingContext(summary bool, anchor types.AnchorKind) *types.AgentContext {
	fields := []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation, types.SourceInventoryFieldPackage}
	if summary {
		fields = append(fields, types.SourceInventoryFieldSummary)
	}
	mut := types.NewMutableState("")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "source members", Value: "1", Role: types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Dispatch"}, MemberNotes: []string{"Dispatch executes every branch and owns the worker"},
		SupportRefs: []string{"Dispatch @ src/dispatch.go:12"},
	}})
	mut.SetInvestigationComplete("complete")
	mut.SetInvestigationResultKind("resolved")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:                 types.IntentEnumerate,
		Predicates:             types.SemanticPredicates{IsCategoryEnumeration: true},
		SourceInventoryProfile: &types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}, RequestedFields: fields, Confidence: 1},
	}}}
	if anchor != "" {
		ctx.EvidenceItems = []types.EvidenceItem{{ID: "dispatch-op", Kind: types.EvidenceDirect, Subject: "Dispatch", AnchorSymbol: "Dispatch", AnchorKind: anchor,
			Source: "src/dispatch.go", LineStart: 12, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			Summary: "dispatches the selected worker", Snippet: "worker.Run()"}}
	}
	return ctx
}

func TestB1620FinalizerNoteTeachingPreservesCandidateWithoutMandatoryCopy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary bool
		anchor  types.AnchorKind
	}{
		{"mechanical_without_evidence", false, ""},
		{"mechanical_with_definition", false, types.AnchorDefinition},
		{"summary_with_definition", true, types.AnchorDefinition},
		{"summary_with_call", true, types.AnchorCall},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1620NoteTeachingContext(tc.summary, tc.anchor)
			before, err := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
			if err != nil {
				t.Fatal(err)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{
				"## Principal Enumeration Rows", "Dispatch executes every branch and owns the worker",
				"row_id=", "location=`src/dispatch.go:12`", "citation_key=`src/dispatch.go:12`",
				"A non-empty `note` is retained candidate explanation, not an extra output requirement or proof",
				"When `summary` or another typed explanatory dimension is requested",
				"explain the supported portion on the same row",
				"declaration/operation evidence", "does not prove the entire note",
				`origin="aggregate_member_note"`, `explanation=candidate`,
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("missing teaching %q:\n%s", want, prompt)
				}
			}
			if tc.anchor != "" {
				if !strings.Contains(prompt, `origin="evidence_summary"`) || !strings.Contains(prompt, `evidence_id="dispatch-op"`) {
					t.Fatal("actual compiled principal row lost separately attributed source explanation")
				}
			}
			for _, forbidden := range []string{
				"When any row has a non-empty `note`, render that note",
				"use `note` to keep the answer explanatory instead of a dry symbol dump",
				"This request's member_set is a source operation-site set",
				"the principal `member_set` rows are the requested write/call/registration/entry points",
				"Grounded definition evidence from read files and the requested source scope is the detail/citation authority",
			} {
				if strings.Contains(prompt, forbidden) {
					t.Errorf("unconditional note/operation authority remains %q", forbidden)
				}
			}
			after, err := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("prompt must not rewrite the retained model facts or notes")
			}
			schema := string((&tool.EmitAnswerDocument{}).ParametersFor(ctx))
			if !json.Valid([]byte(schema)) {
				t.Fatal("projected schema is invalid JSON")
			}
			if !strings.Contains(string((&tool.EmitAnswerDocument{}).Parameters()), `"source_inventory_row_id"`) {
				t.Fatal("existing item/citation identity disappeared from canonical schema")
			}
			for _, internal := range []string{`"note_parts"`, `"model_notes"`, `"candidate_attributes"`, `"candidate_surface_terms"`} {
				if strings.Contains(schema, internal) {
					t.Fatalf("system note attribution became a model JSON obligation: %s", internal)
				}
			}
		})
	}
}

func TestB1620FinalizerSeparatesCandidateFieldsFromExactInventoryFields(t *testing.T) {
	ctx := b1620NoteTeachingContext(false, types.AnchorDefinition)
	facts := ctx.Mutable.StableInvestigationAggregateFacts()
	facts[0].MemberNotes = []string{"surface=foreign func Dispatch; package claimed.wrong; investigate its external implementation"}
	ctx.Mutable.SetInvestigationAggregateFacts(facts)
	ctx.Mutable.RetainInvestigationAggregateFacts()
	ctx.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active: true, Complete: true, Scopes: []string{"src"},
		Sets: []types.SourceInventoryObservationSet{{Role: types.AnswerCandidateRoleFunction, Complete: true, Count: 1,
			Members: []types.SourceInventoryObservationMember{{Name: "Dispatch", File: "src/dispatch.go", Line: 12,
				CoverageState: types.SourceInventoryCoverageObserved, SurfaceTerms: []string{"func", "func Dispatch"},
				Attributes: []types.SourceInventoryObservationAttribute{{Name: "actual.scope", Role: types.AnswerCandidateRolePackage, File: "src/dispatch.go", Line: 1, CoverageState: types.SourceInventoryCoverageObserved}},
			}},
		}},
	})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	var row string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "row_id=") && strings.Contains(line, "member=`Dispatch`") {
			row = line
			break
		}
	}
	for _, want := range []string{"attributes=[`package:actual.scope`]", "candidate_attributes=[`package:claimed.wrong`]", "candidate_surface_terms=", "foreign func Dispatch", `origin="aggregate_member_note"`, "investigate its external implementation"} {
		if !strings.Contains(row, want) {
			t.Errorf("actual compiled row lost separate field %q: %s", want, row)
		}
	}
	if strings.Contains(row, ", attributes=[`package:claimed.wrong`]") || strings.Contains(row, "surface_family=`foreign func`") {
		t.Fatalf("candidate became an exact field: %s", row)
	}
	if !strings.Contains(prompt, "retained navigation hints, not exact row dimensions or required answer fields") {
		t.Fatal("candidate field display lacks its use boundary")
	}
}

func TestB1620AggregateNoteAuthorityDefaultsToCandidateAtEveryPosition(t *testing.T) {
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Members: []string{"A", "B", "C"},
		MemberNotes: []string{"arbitrary interpretation", "observed call explanation", "unlocated interpretation"},
		SupportRefs: []string{"A @ src/a.go:10", "B @ src/a.go:20", ""}}
	for _, tc := range []struct {
		name     string
		evidence []types.EvidenceItem
	}{
		{"no_evidence", nil},
		{"definition_and_call", []types.EvidenceItem{
			{Source: "src/a.go", LineStart: 10, AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded},
			{Source: "src/a.go", LineStart: 20, AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]string(nil), fact.MemberNotes...)
			got := aggregateMemberNoteSupportAuthority(fact, tc.evidence)
			for _, want := range []string{"1:", "2:", "3:", "candidate_description", "claim_form_only"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing bounded note authority %q in %s", want, got)
				}
			}
			if len(tc.evidence) != 0 {
				for _, want := range []string{"1:definition_fact{source_shape_authority=definition_site_only executable_body=unproven}", "2:call_edge"} {
					if !strings.Contains(got, want) {
						t.Errorf("lost precise support form %q in %s", want, got)
					}
				}
			}
			if !reflect.DeepEqual(before, fact.MemberNotes) {
				t.Fatal("authority projection changed notes")
			}
		})
	}
}

func TestB1620ShortSupportListCannotBindTheFirstMemberNote(t *testing.T) {
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet,
		Members: []string{"A", "B", "C"}, MemberNotes: []string{"first candidate", "second candidate", "third candidate"},
		SupportRefs: []string{"C @ src/c.go:10"}}
	evidence := []types.EvidenceItem{
		{Source: "src/c.go", LineStart: 10, OwnerSymbol: "C", AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded},
		{Source: "src/c.go", LineStart: 12, OwnerSymbol: "C", AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded},
	}
	got := aggregateMemberNoteSupportAuthority(fact, evidence)
	if strings.Contains(got, "definition_fact") || !strings.Contains(got, "1:support_unclassified") || !strings.Contains(got, "3:support_unclassified") {
		t.Fatalf("short support list was guessed to align to the first note: %s", got)
	}
	if got := aggregateMemberNoteCompositeSupport(fact, evidence); got != "" {
		t.Fatalf("short support list borrowed a composite owner for the first note: %s", got)
	}
	fact.SupportRefs = []string{"", "", "C @ src/c.go:10"}
	if got := aggregateMemberNoteSupportAuthority(fact, evidence); !strings.Contains(got, "3:definition_fact") || !strings.Contains(got, "1:support_unclassified") {
		t.Fatalf("complete positional slots must retain exact third-member support: %s", got)
	}
	if got := aggregateMemberNoteCompositeSupport(fact, evidence); !strings.Contains(got, "3:{definition_fact@src/c.go:10, call_edge@src/c.go:12}") {
		t.Fatalf("complete positional slots lost the supported composite member: %s", got)
	}
	fact.Members = []string{"C", "B", "A"}
	fact.MemberNotes = []string{"first candidate"}
	fact.SupportRefs = []string{"C @ src/c.go:10", "", ""}
	if got := aggregateMemberNoteSupportAuthority(fact, evidence); !strings.Contains(got, "1:definition_fact") {
		t.Fatalf("a shorter note array retains its original index when member/ref slots are complete: %s", got)
	}
}

func TestB1620EnumerationNotePartProjectionIsBoundedAndNeverWholeNoteProof(t *testing.T) {
	row := types.EnumerationDisplayRow{Note: "legacy explanation preserved", NoteParts: []types.EnumerationDisplayNotePart{
		{Text: "owns every worker", Origin: types.EnumerationDisplayNoteAggregate, SupportRef: "src/api.ts:4"},
		{Text: "definition summary", Origin: types.EnumerationDisplayNoteEvidence, EvidenceID: "ev-def", ClaimForm: types.ClaimDefinitionFact, SupportRef: "src/api.ts:4"},
		{Text: "calls selected worker", Origin: types.EnumerationDisplayNoteAnchor, EvidenceID: "ev-call", ClaimForm: types.ClaimCallEdge, SupportRef: "src/api.ts:20"},
		{Text: "returns result", Origin: types.EnumerationDisplayNoteStep, SupportRef: "src/api.ts:21"},
	}}
	before, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	got := renderAnswerDocEnumerationRowNoteParts(row)
	for _, want := range []string{"aggregate_member_note", "evidence_summary", "anchor_summary", "answer_step", "owns every worker", "definition_site_only", "executable_body=unproven", "calls selected worker", "ev-call", "src/api.ts:20", "explanation=candidate", "ceiling=claim_form_only"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing explanation attribution %q in %s", want, got)
		}
	}
	after, _ := json.Marshal(row)
	if string(before) != string(after) {
		t.Fatal("render changed Note or NoteParts")
	}
	for len(row.NoteParts) < 11 {
		row.NoteParts = append(row.NoteParts, types.EnumerationDisplayNotePart{Text: "other candidate"})
	}
	if got := renderAnswerDocEnumerationRowNoteParts(row); !strings.Contains(got, "note_parts_shown=8/11") {
		t.Fatalf("bounded provenance must disclose omissions: %s", got)
	}
	legacy := renderAnswerDocEnumerationRowNoteParts(types.EnumerationDisplayRow{Note: "unclassified original"})
	if !strings.Contains(legacy, "origin=unclassified_candidate") || !strings.Contains(legacy, "support=unclassified") {
		t.Fatalf("legacy note lacks conservative scope: %s", legacy)
	}
}

func TestB1620CandidateEvidenceDoesNotSupplyNoteClaimFormCeiling(t *testing.T) {
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRoleSupportingCoverage, Members: []string{"Dispatch"}, MemberNotes: []string{"dispatches to a worker"}, SupportRefs: []string{"Dispatch @ src/a.go:10"}}
	evidence := []types.EvidenceItem{
		{ID: "def", Source: "src/a.go", LineStart: 10, OwnerSymbol: "Dispatch", AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded},
		{ID: "call", Source: "src/a.go", LineStart: 12, OwnerSymbol: "Dispatch", AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded},
	}
	if got := aggregateMemberNoteSupportAuthority(fact, evidence); !strings.Contains(got, "definition_fact") {
		t.Fatalf("precise definition scope disappeared: %s", got)
	}
	if got := aggregateMemberNoteCompositeSupport(fact, evidence); !strings.Contains(got, "call_edge@src/a.go:12") {
		t.Fatalf("precise operation scope disappeared: %s", got)
	}
	for i := range evidence {
		evidence[i].DerivationCandidate = true
	}
	got := aggregateMemberNoteSupportAuthority(fact, evidence)
	if !strings.Contains(got, "candidate_description") || !strings.Contains(got, "support_unclassified") || strings.Contains(got, "definition_fact") {
		t.Fatalf("candidate emitted accepted source shape: %s", got)
	}
	if got := aggregateMemberNoteCompositeSupport(fact, evidence); got != "" {
		t.Fatalf("candidate emitted composite accepted support: %s", got)
	}
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{}, EvidenceItems: evidence}
	if got := renderStructuredAggregateFactsForContext(ctx, []types.AnswerAggregateFact{fact}); !strings.Contains(got, "dispatches to a worker") {
		t.Fatalf("candidate explanation was deleted: %s", got)
	}
}

func TestB1620FinalizerObservationLedgerDisplaysModelNotesOutsideRecordAuthority(t *testing.T) {
	for _, anchor := range []types.AnchorKind{"", types.AnchorDefinition, types.AnchorCall} {
		t.Run(string(anchor), func(t *testing.T) {
			ctx := b1620NoteTeachingContext(true, anchor)
			got := renderAnswerDocObservationLedger(ctx)
			var row string
			for _, line := range strings.Split(got, "\n") {
				if strings.Contains(line, "`aggregate:0#") {
					row = line
					break
				}
			}
			at := strings.Index(row, "model_notes/advisory=")
			if at < 0 {
				t.Fatalf("actual finalizer observation row lost model note attribution:\n%s", got)
			}
			if strings.Contains(row[:at], "Dispatch executes every branch and owns the worker") {
				t.Fatalf("candidate explanation still rides proven record fields: %s", row)
			}
			for _, want := range []string{"Dispatch executes every branch and owns the worker", "not covered by record claim_authority", "support_ref locates the member, not proof of the explanation", `"member_index":0`, `"member":"Dispatch"`, `"support_ref":"Dispatch @ src/dispatch.go:12"`} {
				if !strings.Contains(row[at:], want) {
					t.Errorf("candidate/source context missing %q in %s", want, row)
				}
			}
		})
	}
}
