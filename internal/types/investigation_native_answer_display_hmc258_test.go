package types

import (
	"encoding/json"
	"testing"
)

func nativeDisplayFixtureHMC258() (*RequestModel, *RuntimeMeasurementContract, *InvestigationNativeFactHandoff) {
	rm := &RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactCountOrDuration}}}
	c := &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{
		{ObservationID: "source-a/query-window-1", View: RuntimeMeasurementSummary, Columns: []string{"record"}, Rows: [][]string{{"exact 1"}}, Notes: []string{"Unknown is not zero"}},
		{ObservationID: "source-a/query-window-1", View: RuntimeMeasurementMembers, Columns: []string{"frame", "depth"}, Rows: [][]string{{"a", "0"}, {"b", "2"}}, Notes: []string{"Complete retained rows, unwind unknown"}},
		{ObservationID: "source-b/query-window-2", View: RuntimeMeasurementMembers, Columns: []string{"frame", "depth"}, Rows: [][]string{{"a", "0"}, {"c", "0"}}, Notes: []string{"Different physical source, not a call edge"}},
	}, answerDisplayIdentities: map[string]string{"source-a/query-window-1": "source-a/generation-1/query-1", "source-b/query-window-2": "source-b/generation-1/query-2"}}
	h := &InvestigationNativeFactHandoff{RuntimeMeasurements: []AnswerRuntimeMeasurementReceipt{{ObservationID: c.Tables[0].ObservationID, View: RuntimeMeasurementSummary}, {ObservationID: c.Tables[1].ObservationID, View: RuntimeMeasurementMembers}, {ObservationID: c.Tables[2].ObservationID, View: RuntimeMeasurementMembers}}, Interpretation: "Independent clue"}
	h.AnswerDisplay = append([]AnswerRuntimeMeasurementReceipt(nil), h.RuntimeMeasurements[1:]...)
	return rm, c, h
}

func TestHMC258NativeDisplayDoesNotInferIntentOrIdentity(t *testing.T) {
	for _, mode := range []string{"valid", "no_intent", "unselected", "duplicate", "unknown", "event_subset", "source_only", "new_generation", "same_id_changed_notes", "same_id_changed_rows", "no_private_source", "json"} {
		t.Run(mode, func(t *testing.T) {
			rm, c, h := nativeDisplayFixtureHMC258()
			switch mode {
			case "no_intent":
				h.AnswerDisplay = nil
			case "unselected":
				h.RuntimeMeasurements = h.RuntimeMeasurements[:1]
			case "duplicate":
				h.AnswerDisplay = append(h.AnswerDisplay, h.AnswerDisplay[0])
			case "unknown":
				h.AnswerDisplay[0].ObservationID = "not_current"
			case "event_subset":
				h.AnswerDisplay[0].EventSelection = &RuntimeEventSelection{RowIDs: []string{"other"}}
			case "source_only":
				rm.RuntimeQuestionProfile = nil
			case "no_private_source":
				c.answerDisplayIdentities = nil
			}
			sealed := SealInvestigationNativeAnswerDisplay(h, rm, c)
			want := mode == "valid" || mode == "no_intent" || mode == "new_generation" || mode == "same_id_changed_notes" || mode == "same_id_changed_rows" || mode == "json"
			if sealed != want {
				t.Fatalf("seal=%t want=%t", sealed, want)
			}
			if !sealed {
				return
			}
			switch mode {
			case "new_generation":
				c.answerDisplayIdentities[c.Tables[1].ObservationID] = "source-a/generation-2/query-1"
			case "same_id_changed_notes":
				c.Tables[1].Notes[0] = "Changed boundary"
			case "same_id_changed_rows":
				c.Tables[1].Rows[1][1] = "1"
			case "json":
				wire, _ := json.Marshal(h)
				h = &InvestigationNativeFactHandoff{}
				json.Unmarshal(wire, h)
			}
			selected, unavailable := ResolveInvestigationNativeAnswerDisplay(h, rm, c)
			switch mode {
			case "no_intent", "json":
				if len(selected) != 0 || unavailable != 0 {
					t.Fatal("reference or JSON minted private display intent")
				}
			case "new_generation", "same_id_changed_notes", "same_id_changed_rows":
				if len(selected) != 1 || unavailable != 1 || selected[0].ObservationID != c.Tables[2].ObservationID {
					t.Fatal("changed view replaced old data or suppressed independent good view")
				}
			default:
				if len(selected) != 2 || unavailable != 0 || selected[0].View != RuntimeMeasurementMembers {
					t.Fatal("summary replaced requested members")
				}
			}
		})
	}
}

func TestHMC258NativeDisplayAcceptanceLifecycleAndClone(t *testing.T) {
	rm, c, h := nativeDisplayFixtureHMC258()
	if !SealInvestigationNativeAnswerDisplay(h, rm, c) {
		t.Fatal("seal")
	}
	m := NewMutableState("native facts")
	m.SetInvestigationComplete("original")
	m.AcceptInvestigationNativeFactHandoff(h)
	h.AnswerDisplay[0].ObservationID = "mutation"
	h.answerDisplayBindings = nil
	cloned := m.AcceptedInvestigationNativeFactHandoff()
	if out, unavailable := ResolveInvestigationNativeAnswerDisplay(cloned, rm, c.Clone()); len(out) != 2 || unavailable != 0 {
		t.Fatal("clone lost accepted private display or aliased caller")
	}
	cloned.answerDisplayBindings[nativeAnswerDisplayKey(cloned.AnswerDisplay[0])] = "mutation"
	if out, _ := ResolveInvestigationNativeAnswerDisplay(m.AcceptedInvestigationNativeFactHandoff(), rm, c); len(out) != 2 {
		t.Fatal("getter leaked private map")
	}
	fork := m.ForkForExploreDispatch()
	if out, _ := ResolveInvestigationNativeAnswerDisplay(fork.AcceptedInvestigationNativeFactHandoff(), rm, c); len(out) != 2 {
		t.Fatal("same accepted dispatch lost display intent")
	}
	m.SetInvestigationComplete("new accepted closure")
	if m.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("old display crossed accepted completion generation")
	}
	m.MergeExploreFork(fork)
	if m.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("inherited fork resurrected old layout")
	}
}
