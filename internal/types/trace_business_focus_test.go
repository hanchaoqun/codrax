package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func publishBusinessFocusTestRef(t *testing.T, m *MutableState, path string, change func(*TraceBusinessSpanCandidate)) TraceBusinessSpanRef {
	t.Helper()
	candidate := traceBusinessSpanTestCandidate(path)
	if change != nil {
		change(&candidate)
	}
	result := stampTraceBusinessSpanTestResult(m, path, candidate)
	m.AppendDispatchToolResult(result)
	if len(result.TraceBusinessSpanRefs) != 1 {
		t.Fatalf("missing native receipt: %+v", result)
	}
	return result.TraceBusinessSpanRefs[0]
}

func assertBusinessFocus(t *testing.T, m *MutableState, want TraceBusinessFocusStatus, token string) {
	t.Helper()
	status, ref := m.AcceptedTraceBusinessFocus()
	if status != want || ref.Token() != token {
		t.Fatalf("focus = (%s, %q), want (%s, %q)", status, ref.Token(), want, token)
	}
}

func TestTraceBusinessFocusRequiresAcceptedCompletionAndSuccessfulDispatch(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "success", false: "worker_failure"}[success], func(t *testing.T) {
			m := NewMutableState("focus")
			ref := publishBusinessFocusTestRef(t, m, path, nil)
			assertBusinessFocus(t, m, TraceBusinessFocusNone, "")
			ticket := m.BeginTraceBusinessFocusDispatch()
			before := m.InvestigationCompleteGeneration()
			if !m.AcceptInvestigationCompleteWithBusinessSpanRef("accepted", &ref) {
				t.Fatal("current published ref rejected")
			}
			if m.InvestigationCompleteGeneration() != before+1 || !m.IsInvestigationComplete() {
				t.Fatal("completion did not commit atomically with pending selection")
			}
			if status, got := m.AcceptedTraceBusinessFocus(); status == TraceBusinessFocusSelected || got.Token() != "" {
				t.Fatal("tool acceptance alone authorized a focus")
			}
			m.SettleTraceBusinessFocusDispatch(ticket, success)
			if success {
				assertBusinessFocus(t, m, TraceBusinessFocusSelected, ref.Token())
			} else {
				assertBusinessFocus(t, m, TraceBusinessFocusInvalid, "")
				m.SettleTraceBusinessFocusDispatch(ticket, true)
				assertBusinessFocus(t, m, TraceBusinessFocusInvalid, "")
			}
		})
	}
}

func TestTraceBusinessFocusClearsWithoutSelectionAndOnSystemCompletion(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	for _, system := range []bool{false, true} {
		m := NewMutableState("clear")
		ref := publishBusinessFocusTestRef(t, m, path, nil)
		ticket := m.BeginTraceBusinessFocusDispatch()
		m.AcceptInvestigationCompleteWithBusinessSpanRef("first", &ref)
		m.SettleTraceBusinessFocusDispatch(ticket, true)
		assertBusinessFocus(t, m, TraceBusinessFocusSelected, ref.Token())
		ticket = m.BeginTraceBusinessFocusDispatch()
		if system {
			m.SetInvestigationComplete("system closure")
		} else if !m.AcceptInvestigationCompleteWithBusinessSpanRef("accepted without choice", nil) {
			t.Fatal("nil choice rejected")
		}
		m.SettleTraceBusinessFocusDispatch(ticket, true)
		assertBusinessFocus(t, m, TraceBusinessFocusCleared, "")
		if m.StableInvestigationCompleteReason() == "" {
			t.Fatal("focus clear damaged retained closure")
		}
	}
}

func TestTraceBusinessFocusResetRevokesPendingAndActive(t *testing.T) {
	resets := map[string]func(*MutableState){
		"exhausted_system_closure": func(m *MutableState) { m.RecordExploreBacktrackExhausted("no fresh accepted model choice") },
		"turn_a":                   (*MutableState).ResetTurnAArtifacts,
		"completion":               (*MutableState).ResetInvestigationComplete,
		"fallback":                 func(m *MutableState) { m.ResetForFallback(FallbackResetTargetExplore) },
		"reopen_before_supplement": func(m *MutableState) { m.ResetSystemTraceSupplementForExploreReopen() },
	}
	for name, reset := range resets {
		for _, active := range []bool{false, true} {
			t.Run(name+map[bool]string{true: "/active", false: "/pending"}[active], func(t *testing.T) {
				m := NewMutableState("reset")
				path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
				ref := publishBusinessFocusTestRef(t, m, path, nil)
				ticket := m.BeginTraceBusinessFocusDispatch()
				m.AcceptInvestigationCompleteWithBusinessSpanRef("accepted", &ref)
				if active {
					m.SettleTraceBusinessFocusDispatch(ticket, true)
				}
				reset(m)
				m.SettleTraceBusinessFocusDispatch(ticket, true)
				if status, got := m.AcceptedTraceBusinessFocus(); status == TraceBusinessFocusSelected || got.Token() != "" {
					t.Fatal("reset/reopen revived stale selection")
				}
			})
		}
	}
}

func TestTraceBusinessFocusResultSourceMatchesOriginalPublication(t *testing.T) {
	m := NewMutableState("original result")
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	candidate := traceBusinessSpanTestCandidate(path)
	result := stampTraceBusinessSpanTestResult(m, path, candidate)
	m.AppendDispatchToolResult(result)
	ref := result.TraceBusinessSpanRefs[0]
	if !TraceBusinessSpanResultSourceMatches(ref, result) {
		t.Fatal("original publication did not match")
	}
	failed := result
	failed.Success = false
	if TraceBusinessSpanResultSourceMatches(ref, failed) {
		t.Fatal("failed query counted as current instance observation")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var replay ToolResult
	if err := json.Unmarshal(payload, &replay); err != nil {
		t.Fatal(err)
	}
	if TraceBusinessSpanResultSourceMatches(ref, replay) {
		t.Fatal("public JSON restored source provenance")
	}
	m.ResetTurnAArtifacts()
	newRef := publishBusinessFocusTestRef(t, m, path, nil)
	if TraceBusinessSpanResultSourceMatches(newRef, result) {
		t.Fatal("old run generation satisfied fresh instance")
	}
	if err := os.Rename(path, filepath.Join(filepath.Dir(path), "old.trace")); err != nil {
		t.Fatal(err)
	}
	writeTraceReadTestFile(t, filepath.Dir(path), "capture.trace")
	newRef = publishBusinessFocusTestRef(t, m, path, nil)
	if TraceBusinessSpanResultSourceMatches(newRef, result) {
		t.Fatal("old inode result satisfied replacement instance")
	}
}

func TestTraceBusinessFocusInheritedCompletionAndOldForkCannotReauthorize(t *testing.T) {
	parent := NewMutableState("stale fork")
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	ref := publishBusinessFocusTestRef(t, parent, path, nil)
	initial := parent.BeginTraceBusinessFocusDispatch()
	parent.AcceptInvestigationCompleteWithBusinessSpanRef("parent accepted", &ref)
	parent.SettleTraceBusinessFocusDispatch(initial, true)
	group := parent.BeginTraceBusinessFocusDispatch()
	fork := parent.ForkForExploreDispatch()
	forkTicket := fork.BeginTraceBusinessFocusDispatch()
	fork.SettleTraceBusinessFocusDispatch(forkTicket, true)
	parent.MergeExploreFork(fork)
	parent.SettleParallelTraceBusinessFocusDispatch(group, []*MutableState{fork}, []*MutableState{fork}, true)
	assertBusinessFocus(t, parent, TraceBusinessFocusCleared, "")
	group = parent.BeginTraceBusinessFocusDispatch()
	fork = parent.ForkForExploreDispatch()
	forkTicket = fork.BeginTraceBusinessFocusDispatch()
	fork.AcceptInvestigationCompleteWithBusinessSpanRef("fresh fork", &ref)
	fork.SettleTraceBusinessFocusDispatch(forkTicket, true)
	parent.ResetInvestigationComplete()
	newGroup := parent.BeginTraceBusinessFocusDispatch()
	parent.MergeExploreFork(fork)
	parent.SettleParallelTraceBusinessFocusDispatch(newGroup, []*MutableState{fork}, []*MutableState{fork}, true)
	parent.SettleParallelTraceBusinessFocusDispatch(group, []*MutableState{fork}, []*MutableState{fork}, true)
	assertBusinessFocus(t, parent, TraceBusinessFocusCleared, "")
}

func TestTraceBusinessFocusParallelInvalidEligibleChoiceCannotFallBack(t *testing.T) {
	for _, scenario := range []string{"worker_failed", "stale_after_worker", "no_choice_failed"} {
		t.Run(scenario, func(t *testing.T) {
			parent := NewMutableState("invalid eligible")
			path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
			group := parent.BeginTraceBusinessFocusDispatch()
			fork := parent.ForkForExploreDispatch()
			worker := fork.BeginTraceBusinessFocusDispatch()
			ref := publishBusinessFocusTestRef(t, fork, path, nil)
			selection := &ref
			if scenario == "no_choice_failed" {
				selection = nil
			}
			fork.AcceptInvestigationCompleteWithBusinessSpanRef("accepted before worker end", selection)
			fork.SettleTraceBusinessFocusDispatch(worker, scenario == "stale_after_worker")
			parent.MergeExploreFork(fork)
			if scenario == "stale_after_worker" {
				if err := os.Rename(path, filepath.Join(filepath.Dir(path), "old.trace")); err != nil {
					t.Fatal(err)
				}
				writeTraceReadTestFile(t, filepath.Dir(path), "capture.trace")
			}
			parent.SettleParallelTraceBusinessFocusDispatch(group, []*MutableState{fork}, []*MutableState{fork}, true)
			if scenario == "no_choice_failed" {
				status, got := parent.AcceptedTraceBusinessFocus()
				if status == TraceBusinessFocusInvalid || status == TraceBusinessFocusSelected || got.Token() != "" {
					t.Fatalf("ordinary no-choice recovery changed: %s", status)
				}
			} else {
				assertBusinessFocus(t, parent, TraceBusinessFocusInvalid, "")
			}
			if !parent.IsInvestigationComplete() {
				t.Fatal("new invalid focus altered accepted partial closure")
			}
		})
	}
}

func TestTraceBusinessFocusOriginalReceiptAndTicketCannotReplay(t *testing.T) {
	m := NewMutableState("receipt")
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	ref := publishBusinessFocusTestRef(t, m, path, nil)
	ticket := m.BeginTraceBusinessFocusDispatch()
	if !m.AcceptInvestigationCompleteWithBusinessSpanRef("accepted", &ref) {
		t.Fatal("accept failed")
	}
	encoded, err := json.Marshal(ticket)
	if err != nil {
		t.Fatal(err)
	}
	var replay TraceBusinessFocusDispatch
	if err := json.Unmarshal(encoded, &replay); err != nil {
		t.Fatal(err)
	}
	m.SettleTraceBusinessFocusDispatch(replay, true)
	if status, _ := m.AcceptedTraceBusinessFocus(); status == TraceBusinessFocusSelected {
		t.Fatal("JSON ticket granted authority")
	}
	m.SettleTraceBusinessFocusDispatch(ticket, true)
	assertBusinessFocus(t, m, TraceBusinessFocusSelected, ref.Token())
	if err := os.Rename(path, filepath.Join(filepath.Dir(path), "original.trace")); err != nil {
		t.Fatal(err)
	}
	writeTraceReadTestFile(t, filepath.Dir(path), "capture.trace")
	assertBusinessFocus(t, m, TraceBusinessFocusInvalid, "")
	before := m.InvestigationCompleteGeneration()
	if m.AcceptInvestigationCompleteWithBusinessSpanRef("stale", &ref) || m.InvestigationCompleteGeneration() != before {
		t.Fatal("stale ref advanced completion")
	}
}

func TestTraceBusinessFocusForkMergesDoNotGrantAuthority(t *testing.T) {
	parent := NewMutableState("parent")
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
	parentTicket := parent.BeginTraceBusinessFocusDispatch()
	fork := parent.ForkForExploreDispatch()
	ticket := fork.BeginTraceBusinessFocusDispatch()
	ref := publishBusinessFocusTestRef(t, fork, path, nil)
	fork.AcceptInvestigationCompleteWithBusinessSpanRef("accepted", &ref)
	fork.SettleTraceBusinessFocusDispatch(ticket, true)
	parent.MergeExploreForkPublishedTools(fork)
	parent.MergeExploreFork(fork)
	assertBusinessFocus(t, parent, TraceBusinessFocusNone, "")
	if _, ok := parent.ResolveTraceBusinessSpanRef(ref.Token()); !ok {
		t.Fatal("published navigation was lost")
	}
	parent.SettleParallelTraceBusinessFocusDispatch(parentTicket, []*MutableState{fork}, []*MutableState{fork}, true)
	assertBusinessFocus(t, parent, TraceBusinessFocusSelected, ref.Token())
}

func TestTraceBusinessFocusParallelConflictAndEquivalentTokens(t *testing.T) {
	for _, scenario := range []string{"same_instance", "different_instance", "clear_and_selected", "failed_sibling", "failed_clear_sibling", "only_loser", "parent_canceled"} {
		t.Run(scenario, func(t *testing.T) {
			parent := NewMutableState("parent")
			path := writeTraceReadTestFile(t, t.TempDir(), "capture.trace")
			parentTicket := parent.BeginTraceBusinessFocusDispatch()
			var forks []*MutableState
			var refs []TraceBusinessSpanRef
			for i := 0; i < 2; i++ {
				fork := parent.ForkForExploreDispatch()
				ticket := fork.BeginTraceBusinessFocusDispatch()
				ref := publishBusinessFocusTestRef(t, fork, path, func(c *TraceBusinessSpanCandidate) {
					if scenario == "different_instance" && i == 1 {
						c.EndTs += .001
						c.EndLine++
					}
				})
				var selection *TraceBusinessSpanRef = &ref
				if (scenario == "clear_and_selected" || scenario == "failed_clear_sibling") && i == 1 {
					selection = nil
				}
				fork.AcceptInvestigationCompleteWithBusinessSpanRef("accepted", selection)
				fork.SettleTraceBusinessFocusDispatch(ticket, !((scenario == "failed_sibling" || scenario == "failed_clear_sibling") && i == 1))
				parent.MergeExploreForkPublishedTools(fork)
				forks, refs = append(forks, fork), append(refs, ref)
			}
			if refs[0].Token() == refs[1].Token() {
				t.Fatal("test needs independent tokens")
			}
			eligible := forks[:1]
			if scenario == "only_loser" {
				eligible = nil
			}
			parent.SettleParallelTraceBusinessFocusDispatch(parentTicket, eligible, forks, scenario != "parent_canceled")
			switch scenario {
			case "same_instance", "failed_sibling", "failed_clear_sibling":
				assertBusinessFocus(t, parent, TraceBusinessFocusSelected, refs[0].Token())
			case "different_instance", "clear_and_selected":
				assertBusinessFocus(t, parent, TraceBusinessFocusConflict, "")
			default:
				if status, ref := parent.AcceptedTraceBusinessFocus(); status == TraceBusinessFocusSelected || ref.Token() != "" {
					t.Fatal("unqualified branch granted focus")
				}
			}
		})
	}
}
