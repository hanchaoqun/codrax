package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// This wire-shaped expectation deliberately exercises the real producer rather
// than making the initial regression depend on a not-yet-added Go field.
func TestB1620AggregateObservationSeparatesModelNotes(t *testing.T) {
	rm := sourceInventoryProjectionRequestModel(nil)
	obs := sourceInventoryProjectionObservation(
		SourceInventoryObservationMember{Name: "Run", Role: AnswerCandidateRoleFunction, File: "src/a.cj", Line: 7, Language: "cangjie", Note: "may initialize the worker"},
		SourceInventoryObservationMember{Name: "Run", Role: AnswerCandidateRoleFunction, File: "src/b.cj", Line: 12, Language: "cangjie", Note: "may initialize the worker"},
	)
	facts := ProjectSourceInventoryPrincipalRowSetAggregateFacts(nil, obs, rm)
	if len(facts) != 1 || len(facts[0].MemberNotes) != 2 {
		t.Fatalf("expected actual source inventory aggregate with aligned notes: %+v", facts)
	}
	before, _ := json.Marshal(facts)
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: facts, RequestModel: &rm})
	if len(ledger.Records) != 1 {
		t.Fatalf("ledger: %+v", ledger)
	}
	record := ledger.Records[0]
	if record.ClaimAuthority != ObservationClaimAuthorityIndependentlyProven || record.Value != "2" || len(record.SupportRefs) != 2 {
		t.Fatalf("independent member/count authority must survive: %+v", record)
	}
	var wire struct {
		ModelNotes []struct {
			MemberIndex int    `json:"member_index"`
			Member      string `json:"member"`
			Text        string `json:"text"`
			SupportRef  string `json:"support_ref"`
		} `json:"model_notes"`
	}
	raw, _ := json.Marshal(record)
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.ModelNotes) != 2 {
		t.Fatalf("model explanations need their own candidate carrier, not whole-record authority: %s", raw)
	}
	for i, note := range wire.ModelNotes {
		if note.MemberIndex != i || note.Member != facts[0].Members[i] || note.Text != facts[0].MemberNotes[i] || note.SupportRef != facts[0].SupportRefs[i] {
			t.Fatalf("note %d lost exact positional binding: %+v / %+v", i, note, facts[0])
		}
		for _, rich := range record.RichNotes {
			if rich == note.Text {
				t.Fatalf("candidate also escaped into authoritative rich notes: %+v", record)
			}
		}
	}
	projected := ProjectObservationPromptRecords(ledger.Records, &rm, nil, DefaultObservationPromptProjectionOptions(8))
	projectedJSON, _ := json.Marshal(projected)
	if !strings.Contains(string(projectedJSON), `"ModelNotes":[`) || !strings.Contains(string(projectedJSON), "may initialize the worker") {
		t.Fatalf("compact projection lost candidate boundary: %s", projectedJSON)
	}
	after, _ := json.Marshal(facts)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("compilation rewrote original model facts")
	}
}

func TestB1620AggregateRowSetSeparatesModelNotes(t *testing.T) {
	fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Members: []string{"Run"}, MemberNotes: []string{"may initialize the worker"}, SupportRefs: []string{"Run @ src/a.cj:7"}}
	got := observationRowSetJSONLForAggregateFact(fact)
	if !strings.Contains(got, `"model_note":{"member_index":0,"member":"Run","text":"may initialize the worker","support_ref":"Run @ src/a.cj:7"}`) || strings.Contains(got, `"note":`) {
		t.Fatalf("row-set recovery must retain candidate qualification: %s", got)
	}
}

func TestB1620ModelNotesKeepIdentityThroughMergeAndJSON(t *testing.T) {
	a := ObservationRecord{ID: "a", Origin: AnswerEvidenceOriginSystemInference, SourceRef: ObservationSourceRef{Kind: ObservationSourceModelClaim, Path: "set.json"}, ClaimKey: "functions", Value: "2", ClaimAuthority: ObservationClaimAuthorityIndependentlyProven,
		RichNotes: []string{"Run"}, ModelNotes: []ObservationModelNote{
			{MemberIndex: 0, Member: "Run", Text: "first description", SupportRef: "Run @ src/a.cj:7"},
			{MemberIndex: 1, Member: "Run", Text: "first description", SupportRef: "Run @ src/b.cj:7"},
		}}
	b := a
	b.ID = "b"
	b.ModelNotes = []ObservationModelNote{
		a.ModelNotes[0],
		{MemberIndex: 0, Member: "Run", Text: "different description", SupportRef: "Run @ src/a.cj:7"},
	}
	beforeA, _ := json.Marshal(a)
	beforeB, _ := json.Marshal(b)
	merged := dedupeObservationRecords([]ObservationRecord{a, b, a})
	if len(merged) != 1 || len(merged[0].ModelNotes) != 3 || merged[0].ClaimAuthority != a.ClaimAuthority || merged[0].Value != a.Value {
		t.Fatalf("full note tuple union must not change fact identity or authority: %+v", merged)
	}
	if strings.Contains(strings.Join(merged[0].RichNotes, " "), "description") {
		t.Fatal("merge promoted candidates into ordinary notes")
	}
	raw, _ := json.Marshal(merged[0])
	var decoded ObservationRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(merged[0], decoded) {
		t.Fatalf("JSON lost candidate role/binding: %s", raw)
	}
	merged[0].ModelNotes[0].Text = "mutated"
	afterA, _ := json.Marshal(a)
	afterB, _ := json.Marshal(b)
	if string(beforeA) != string(afterA) || string(beforeB) != string(afterB) {
		t.Fatal("merge aliased its input candidates")
	}
}

func TestB1620ModelNotesRetainUnknownBindingsAndOriginalText(t *testing.T) {
	fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Label: "functions", Members: []string{"Run", "Run", "Extra"},
		MemberNotes: []string{" ", "  possible\n explanation  ", "third", "unbound"}, SupportRefs: []string{"Run @ src/b.cj:9"}}
	got := aggregateFactObservationModelNotes(fact)
	if len(got) != 3 || got[0].MemberIndex != 1 || got[0].Text != "possible\n explanation" || got[0].Member != "Run" || got[0].SupportRef != "" || got[2].MemberIndex != 3 || got[2].Member != "" || got[2].SupportRef != "" {
		t.Fatalf("unknown positional support must stay unknown, not guessed: %+v", got)
	}
	if got := aggregateFactObservationModelNotes(AnswerAggregateFact{MemberNotes: []string{"unknown member"}}); len(got) != 1 || got[0].Member != "" || got[0].SupportRef != "" {
		t.Fatalf("unbound note discarded/promoted: %+v", got)
	}
}

func TestB1620ModelNotesProjectionBudgetAndDefensiveCopy(t *testing.T) {
	record := ObservationRecord{ID: "aggregate:0#system_inference", Origin: AnswerEvidenceOriginSystemInference, ClaimAuthority: ObservationClaimAuthorityIndependentlyProven,
		RichNotes: []string{"first", "second"}, ModelNotes: []ObservationModelNote{
			{MemberIndex: 1, Member: "Run", Text: strings.Repeat("x", 100), SupportRef: "Run @ src/a.cj:7"},
			{MemberIndex: 5, Member: "Run", Text: "another", SupportRef: "Run @ src/b.cj:9"},
			{MemberIndex: 8, Member: "hidden", Text: "third"},
		}}
	opts := DefaultObservationPromptProjectionOptions(8)
	opts.NoteLimit = 2
	opts.NoteMaxLen = 20
	got := ProjectObservationPromptRecords([]ObservationRecord{record}, nil, nil, opts)
	if len(got) != 1 || len(got[0].ModelNotes) != 2 || len(got[0].Notes) != 0 || got[0].ModelNotes[0].MemberIndex != 1 || got[0].ModelNotes[1].MemberIndex != 5 || got[0].ModelNotes[0].SupportRef != record.ModelNotes[0].SupportRef || got[0].ModelNotes[0].Text == record.ModelNotes[0].Text {
		t.Fatalf("compact projection changed old total note budget or binding: %+v", got)
	}
	got[0].ModelNotes[0].Text = "mutated"
	if record.ModelNotes[0].Text != strings.Repeat("x", 100) {
		t.Fatal("projection aliases full payload")
	}
}

func TestB1620ModelNotesToolCarryDoesNotChangeTraceOrLegacy(t *testing.T) {
	for _, withCandidate := range []bool{false, true} {
		record := ObservationRecord{ID: "trace:1", Origin: AnswerEvidenceOriginRuntimeArtifact, ClaimAuthority: ObservationClaimAuthorityDirectObservation,
			Producer: "trace_query", SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "trace.txt", RawRef: "raw", ToolCallID: "q1"},
			Subject: "app(42)", Predicate: "critical_blocking", Value: "1.250", RichNotes: []string{"tier=on_chain", "selected_window=10.000..11.000", "  original notes\nbytes  "}}
		if withCandidate {
			record.ModelNotes = []ObservationModelNote{{MemberIndex: 0, Text: "maybe business explanation"}}
		}
		before, _ := json.Marshal(record)
		ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{record}}}})
		got := findObservationRecord(t, ledger, "trace:1")
		if !reflect.DeepEqual(got.RichNotes, record.RichNotes) || got.Value != record.Value || got.Predicate != record.Predicate || got.ClaimAuthority != record.ClaimAuthority || !reflect.DeepEqual(got.ModelNotes, record.ModelNotes) {
			t.Fatalf("tool carry changed raw Trace/legacy semantics: %+v", got)
		}
		if withCandidate {
			got.ModelNotes[0].Text = "mutated"
			after, _ := json.Marshal(record)
			if string(before) != string(after) {
				t.Fatal("tool observation input aliases compiled candidate")
			}
		}
	}
}

func TestB1620ModelNotesSurviveStableForkAndAcceptedHandoff(t *testing.T) {
	fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Label: "functions", Value: "1", Role: AnswerAggregateRolePrincipalAnswer,
		Provenance: SourceInventoryPrincipalRowSetAggregateProvenance, Members: []string{"Run"}, MemberNotes: []string{"may initialize workers"}, SupportRefs: []string{"Run @ src/a.cj:7"}}
	mut := NewMutableState("q")
	mut.SetInvestigationAggregateFacts([]AnswerAggregateFact{fact})
	mut.RetainInvestigationAggregateFacts()
	fork := mut.ForkForExploreDispatch()
	returned := fork.StableInvestigationAggregateFacts()
	returned[0].MemberNotes[0] = "caller mutation"
	if mut.StableInvestigationAggregateFacts()[0].MemberNotes[0] != fact.MemberNotes[0] || fork.StableInvestigationAggregateFacts()[0].MemberNotes[0] != fact.MemberNotes[0] {
		t.Fatal("stable fork aliases model explanation")
	}
	mut.MergeExploreFork(fork)
	mut.SetTurnAArtifacts(TurnAArtifacts{AcceptedAggregateFacts: []AnswerAggregateFact{fact}})
	for name, input := range map[string]ObservationLedgerInput{
		"agent": ObservationLedgerInputFromAgentContext(&AgentContext{Mutable: mut}, 8),
		"bus":   ObservationLedgerInputFromBusContext(&BusContext{Mutable: mut}, 8),
	} {
		ledger := CompileObservationLedger(input)
		if len(ledger.Records) != 1 || len(ledger.Records[0].ModelNotes) != 1 || ledger.Records[0].ModelNotes[0].Text != fact.MemberNotes[0] || ledger.Records[0].ClaimAuthority != ObservationClaimAuthorityIndependentlyProven {
			t.Fatalf("%s handoff lost candidate role or exact count/declaration authority: %+v", name, ledger)
		}
		if strings.Contains(strings.Join(ledger.Records[0].RichNotes, " "), fact.MemberNotes[0]) {
			t.Fatalf("%s handoff promoted candidate", name)
		}
	}
}

func TestB1620ModelNotesMissingCarrierDoesNotReclassifyHistory(t *testing.T) {
	legacyJSON := `{"id":"aggregate:legacy","origin":"system_inference","claim_authority":"independently_proven","rich_notes":["member explanation from old snapshot"]}`
	var legacy ObservationRecord
	if err := json.Unmarshal([]byte(legacyJSON), &legacy); err != nil {
		t.Fatal(err)
	}
	got := ProjectObservationPromptRecords([]ObservationRecord{legacy}, nil, nil, DefaultObservationPromptProjectionOptions(8))
	if len(got) != 1 || len(got[0].ModelNotes) != 0 || !reflect.DeepEqual(got[0].Notes, legacy.RichNotes) || got[0].ClaimAuthority != legacy.ClaimAuthority {
		t.Fatalf("legacy snapshots must not be guessed/migrated from prose: %+v", got)
	}
}
