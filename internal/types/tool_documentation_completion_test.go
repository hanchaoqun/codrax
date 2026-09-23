package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// These lifecycle tests construct producer output, but never construct or
// assign the private read token or accepted receipt. Model-tool acceptance is
// represented by the public accepted-completion API, not a forged JSON field.
func documentationCompletionResult(t *testing.T, view string, padding int) ToolResult {
	t.Helper()
	content, err := json.Marshal(map[string]any{
		"view": view, "padding": strings.Repeat("x", padding),
		"limitations": []string{"documentation only", "not source or runtime evidence", "complete trailing condition"},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := NormalizeToolDocumentation(ToolDocumentation{Version: ToolDocumentationVersion,
		Schema: "trace_capabilities/v1", Selection: ToolDocumentationSelection{View: view, Detail: true}, Content: content})
	if !ok {
		t.Fatal("invalid producer fixture")
	}
	return ToolResult{ToolName: "trace_capabilities", Success: true,
		Handoff: &ToolHandoffCarrier{Version: ToolHandoffCarrierVersion, ToolName: "trace_capabilities", Documentation: &doc}}
}

func documentationCompletionState() (*MutableState, RequestModel) {
	rm := RequestModel{RawRequest: "Explain documented inputs and limitations",
		ToolDocumentationRequest: &ToolDocumentationRequest{Scope: ToolDocumentationRequestOnly}}
	m := NewMutableState(rm.RawRequest)
	m.SetRequestModel(rm)
	return m, rm
}

func sealDocumentationCompletion(t *testing.T, m *MutableState, rm *RequestModel) {
	t.Helper()
	if !m.AcceptInvestigationCompleteWithBusinessSpanRef("accepted documentation explanation", nil) {
		t.Fatal("ordinary accepted completion unexpectedly refused")
	}
	m.AcceptToolDocumentationCompletion(rm)
}

func TestToolDocumentationCompletionRequiresStampedPublishedResult(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stamp  bool
		append bool
		edit   func(*ToolResult)
		want   bool
	}{
		{name: "current_producer", stamp: true, append: true, want: true},
		{name: "unstamped", append: true},
		{name: "not_published", stamp: true},
		{name: "failed_after_stamp", stamp: true, append: true, edit: func(r *ToolResult) { r.Success = false }},
		{name: "other_tool_after_stamp", stamp: true, append: true, edit: func(r *ToolResult) { r.ToolName = "read_file"; r.Handoff.ToolName = "read_file" }},
		{name: "wrong_carrier_producer", stamp: true, append: true, edit: func(r *ToolResult) { r.Handoff.ToolName = "other" }},
		{name: "wrong_schema", stamp: true, append: true, edit: func(r *ToolResult) { r.Handoff.Documentation.Schema = "other/v1" }},
		{name: "wrong_version", stamp: true, append: true, edit: func(r *ToolResult) { r.Handoff.Documentation.Version++ }},
		{name: "wrong_hash", stamp: true, append: true, edit: func(r *ToolResult) { r.Handoff.Documentation.ContentHash = strings.Repeat("0", 64) }},
		{name: "changed_content_rehashed", stamp: true, append: true, edit: func(r *ToolResult) {
			r.Handoff.Documentation.Content = json.RawMessage(`{"limitations":[],"forged":true}`)
			r.Handoff.Documentation.ContentHash = ""
		}},
		{name: "changed_selection", stamp: true, append: true, edit: func(r *ToolResult) { r.Handoff.Documentation.Selection.View = "other_view" }},
		{name: "changed_detail", stamp: true, append: true, edit: func(r *ToolResult) { r.Handoff.Documentation.Selection.Detail = false }},
		{name: "too_large_whole_document", stamp: true, append: true, edit: func(r *ToolResult) {
			r.Handoff.Documentation.Content = json.RawMessage(`{"padding":"` + strings.Repeat("x", ToolDocumentationMaxBytes) + `"}`)
			r.Handoff.Documentation.ContentHash = ""
		}},
		{name: "summary_not_authority", append: true, edit: func(r *ToolResult) { r.Summary = string(r.Handoff.Documentation.Content); r.Handoff = nil }},
		{name: "JSON_result_replay", stamp: true, append: true, edit: func(r *ToolResult) {
			b, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var replay ToolResult
			if err := json.Unmarshal(b, &replay); err != nil {
				t.Fatal(err)
			}
			*r = replay
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, rm := documentationCompletionState()
			r := documentationCompletionResult(t, "waits", 0)
			if tc.stamp {
				r = m.StampToolDocumentationResult(r)
			}
			if tc.edit != nil {
				tc.edit(&r)
			}
			if tc.append {
				m.AppendDispatchToolResult(r)
			}
			if got := m.ToolDocumentationReady(); got != tc.want {
				t.Fatalf("ready=%t want=%t", got, tc.want)
			}
			sealDocumentationCompletion(t, m, &rm)
			if got := m.HasAcceptedToolDocumentationCompletion(&rm); got != tc.want {
				t.Fatalf("accepted=%t want=%t", got, tc.want)
			}
			if got := len(m.AcceptedToolDocumentationCarriers()) > 0; got != tc.want {
				t.Fatalf("accepted carrier presence=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestToolDocumentationCompletionStampRejectsUnqualifiedProducer(t *testing.T) {
	for _, mode := range []string{"failure", "tool", "schema", "hash"} {
		t.Run(mode, func(t *testing.T) {
			m, _ := documentationCompletionState()
			r := documentationCompletionResult(t, "", 0)
			switch mode {
			case "failure":
				r.Success = false
			case "tool":
				r.ToolName, r.Handoff.ToolName = "other", "other"
			case "schema":
				r.Handoff.Documentation.Schema = "other/v1"
			case "hash":
				r.Handoff.Documentation.ContentHash = "bad"
			}
			m.AppendDispatchToolResult(m.StampToolDocumentationResult(r))
			if m.ToolDocumentationReady() {
				t.Fatal("unqualified producer acquired completion eligibility")
			}
		})
	}
}

func TestToolDocumentationCompletionAcceptanceAndSnapshot(t *testing.T) {
	m, rm := documentationCompletionState()
	r := documentationCompletionResult(t, "waits", 0)
	stamped := m.StampToolDocumentationResult(r)
	if m.ToolDocumentationReady() {
		t.Fatal("stamp alone registered a document")
	}
	m.AppendDispatchToolResult(stamped)
	m.AcceptToolDocumentationCompletion(&rm)
	if m.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("document alone completed investigation")
	}
	m.SetInvestigationComplete("system closure without accepted model tail")
	if m.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("system completion minted a receipt")
	}
	sealDocumentationCompletion(t, m, &rm)
	if !m.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("accepted current document was not sealed")
	}
	got := m.AcceptedToolDocumentationCarriers()
	want := string(got[0].Documentation.Content)
	stamped.Handoff.Documentation.Content[0] = 'X'
	got[0].Documentation.Content[0] = 'Y'
	if string(m.AcceptedToolDocumentationCarriers()[0].Documentation.Content) != want {
		t.Fatal("producer or getter content aliases the accepted document")
	}
	m.SetTurnAArtifacts(TurnAArtifacts{HandoffCarriers: m.AcceptedToolDocumentationCarriers()})
	turnA := m.TurnAArtifacts()
	if !AcceptedToolDocumentationCompletion(turnA) {
		t.Fatal("accepted handoff lost its private receipt")
	}
	turnA.HandoffCarriers[0].Documentation.Content[0] = 'Z'
	if string(m.TurnAArtifacts().HandoffCarriers[0].Documentation.Content) != want {
		t.Fatal("Turn A getter aliases document bytes")
	}
	if got := CompileObservationLedger(ObservationLedgerInput{ToolResults: m.DispatchToolResults()}); len(got.Records) != 0 {
		t.Fatal("documentation acquired runtime observation authority")
	}
	serialized, err := json.Marshal(m.TurnAArtifacts())
	if err != nil {
		t.Fatal(err)
	}
	var replay TurnAArtifacts
	if err := json.Unmarshal(serialized, &replay); err != nil {
		t.Fatal(err)
	}
	if AcceptedToolDocumentationCompletion(&replay) {
		t.Fatal("serialized Turn A retained private authority")
	}
	other, _ := documentationCompletionState()
	other.SetTurnAArtifacts(replay)
	if AcceptedToolDocumentationCompletion(other.TurnAArtifacts()) || other.ToolDocumentationReady() {
		t.Fatal("public handoff reconstruction created current completion authority")
	}
}

func TestToolDocumentationCompletionRunAndResetBoundaries(t *testing.T) {
	t.Run("different_run", func(t *testing.T) {
		m, _ := documentationCompletionState()
		other, rm := documentationCompletionState()
		other.AppendDispatchToolResult(m.StampToolDocumentationResult(documentationCompletionResult(t, "", 0)))
		sealDocumentationCompletion(t, other, &rm)
		if other.ToolDocumentationReady() || other.HasAcceptedToolDocumentationCompletion(&rm) {
			t.Fatal("cross-run stamp replayed")
		}
	})
	t.Run("dispatch_history_reset_preserves_current_run_contract", func(t *testing.T) {
		m, rm := documentationCompletionState()
		m.AppendDispatchToolResult(m.StampToolDocumentationResult(documentationCompletionResult(t, "", 0)))
		m.ResetDispatchToolResults()
		sealDocumentationCompletion(t, m, &rm)
		if !m.HasAcceptedToolDocumentationCompletion(&rm) {
			t.Fatal("stage boundary lost registered current-run document")
		}
	})
	t.Run("completion_reset_and_system_recompletion", func(t *testing.T) {
		m, rm := documentationCompletionState()
		m.AppendDispatchToolResult(m.StampToolDocumentationResult(documentationCompletionResult(t, "", 0)))
		sealDocumentationCompletion(t, m, &rm)
		m.SetTurnAArtifacts(TurnAArtifacts{})
		m.ResetInvestigationComplete()
		if m.HasAcceptedToolDocumentationCompletion(&rm) || AcceptedToolDocumentationCompletion(m.TurnAArtifacts()) {
			t.Fatal("reset retained accepted completion")
		}
		m.SetInvestigationComplete("new system decision")
		if m.HasAcceptedToolDocumentationCompletion(&rm) || AcceptedToolDocumentationCompletion(m.TurnAArtifacts()) {
			t.Fatal("new decision revived old receipt")
		}
	})
	t.Run("turn_a_reset_retires_generation", func(t *testing.T) {
		m, rm := documentationCompletionState()
		r := m.StampToolDocumentationResult(documentationCompletionResult(t, "", 0))
		m.AppendDispatchToolResult(r)
		sealDocumentationCompletion(t, m, &rm)
		m.ResetTurnAArtifacts()
		m.AppendDispatchToolResult(r)
		sealDocumentationCompletion(t, m, &rm)
		if m.ToolDocumentationReady() || m.HasAcceptedToolDocumentationCompletion(&rm) {
			t.Fatal("previous generation replayed after reset")
		}
		m.AppendDispatchToolResult(m.StampToolDocumentationResult(documentationCompletionResult(t, "", 0)))
		sealDocumentationCompletion(t, m, &rm)
		if !m.HasAcceptedToolDocumentationCompletion(&rm) {
			t.Fatal("fresh generation cannot recover")
		}
	})
}

func TestToolDocumentationCompletionForkMerge(t *testing.T) {
	for _, mode := range []string{"fresh_fork_accepts", "uncompleted_fork", "foreign_run", "reset_before_merge", "system_recompletion_cannot_reseal"} {
		t.Run(mode, func(t *testing.T) {
			m, rm := documentationCompletionState()
			if mode == "system_recompletion_cannot_reseal" {
				m.AppendDispatchToolResult(m.StampToolDocumentationResult(documentationCompletionResult(t, "parent", 0)))
				sealDocumentationCompletion(t, m, &rm)
			}
			fork := m.ForkForExploreDispatch()
			if mode == "foreign_run" {
				fork, _ = documentationCompletionState()
			}
			if mode != "system_recompletion_cannot_reseal" {
				fork.AppendDispatchToolResult(fork.StampToolDocumentationResult(documentationCompletionResult(t, "child", 0)))
			}
			if mode == "system_recompletion_cannot_reseal" {
				fork.SetInvestigationComplete("system decided again without accepting documentation")
			} else if mode != "uncompleted_fork" {
				sealDocumentationCompletion(t, fork, &rm)
			}
			if mode == "reset_before_merge" {
				m.ResetTurnAArtifacts()
			}
			m.MergeExploreFork(fork)
			want := mode == "fresh_fork_accepts"
			if got := m.HasAcceptedToolDocumentationCompletion(&rm); got != want {
				t.Fatalf("merged accepted=%t want=%t", got, want)
			}
			m.SetTurnAArtifacts(TurnAArtifacts{})
			if got := AcceptedToolDocumentationCompletion(m.TurnAArtifacts()); got != want {
				t.Fatalf("merged Turn A accepted=%t want=%t", got, want)
			}
			if mode == "uncompleted_fork" && !m.ToolDocumentationReady() {
				t.Fatal("uncompleted fork lost otherwise valid documentation")
			}
		})
	}
}

func TestToolDocumentationCompletionRequestBinding(t *testing.T) {
	for _, mode := range []string{"other_question", "source_obligation", "removed_domain", "mixed_domain"} {
		t.Run(mode, func(t *testing.T) {
			m, rm := documentationCompletionState()
			m.AppendDispatchToolResult(m.StampToolDocumentationResult(documentationCompletionResult(t, "", 0)))
			sealDocumentationCompletion(t, m, &rm)
			m.SetTurnAArtifacts(TurnAArtifacts{})
			changed := rm
			switch mode {
			case "other_question":
				changed.RawRequest = "a different documented contract"
			case "source_obligation":
				changed.UserPinnedFiles = []string{"src/engine.go"}
			case "removed_domain":
				changed.ToolDocumentationRequest = nil
			case "mixed_domain":
				changed.ToolDocumentationRequest = &ToolDocumentationRequest{Scope: ToolDocumentationRequestMixed, DimensionIndices: []int{1}}
			}
			m.SetRequestModel(changed)
			if m.HasAcceptedToolDocumentationCompletion(&changed) || m.HasAcceptedToolDocumentationCompletion(&rm) ||
				AcceptedToolDocumentationCompletion(m.TurnAArtifacts()) || len(m.AcceptedToolDocumentationCarriers()) != 0 {
				t.Fatal("changed current request reused the old completion receipt")
			}
		})
	}
}

func TestToolDocumentationCompletionWholeDocumentBudget(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		count, padding, want int
	}{{"document_count", 10, 0, 8}, {"document_bytes", 3, 50 << 10, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			m, rm := documentationCompletionState()
			var all []ToolHandoffCarrier
			originals := map[string]string{}
			for i := 0; i < tc.count; i++ {
				r := documentationCompletionResult(t, fmt.Sprintf("view-%02d", i), tc.padding)
				all = append(all, *r.Handoff)
				originals[r.Handoff.Documentation.ContentHash] = string(r.Handoff.Documentation.Content)
				m.AppendDispatchToolResult(m.StampToolDocumentationResult(r))
			}
			selected, omitted := SelectToolDocumentation(all)
			if len(selected) != tc.want || omitted != tc.count-tc.want {
				t.Fatalf("selection=%d omitted=%d", len(selected), omitted)
			}
			sealDocumentationCompletion(t, m, &rm)
			kept := m.AcceptedToolDocumentationCarriers()
			if len(kept) != tc.want {
				t.Fatalf("accepted %d complete documents, want %d", len(kept), tc.want)
			}
			used := 0
			for _, c := range kept {
				d := c.Documentation
				if string(d.Content) != originals[d.ContentHash] || !json.Valid(d.Content) || !strings.Contains(string(d.Content), "complete trailing condition") {
					t.Fatal("budget selection clipped a document or its conditions")
				}
				used += len(ToolDocumentationPromptChunk(c))
			}
			if used > ToolDocumentationPromptMaxBytes {
				t.Fatal("accepted documents exceeded whole-document prompt budget")
			}
		})
	}
}

func TestToolDocumentationCompletionConcurrentPublication(t *testing.T) {
	m, rm := documentationCompletionState()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		r := documentationCompletionResult(t, fmt.Sprintf("view-%02d", i), 0)
		wg.Add(1)
		go func(r ToolResult) { defer wg.Done(); m.AppendDispatchToolResult(m.StampToolDocumentationResult(r)) }(r)
	}
	wg.Wait()
	if !m.ToolDocumentationReady() || m.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("concurrent publication changed completion qualification")
	}
	sealDocumentationCompletion(t, m, &rm)
	if len(m.AcceptedToolDocumentationCarriers()) != ToolDocumentationPromptMaxCount {
		t.Fatal("concurrent publication lost bounded complete documents")
	}
}
