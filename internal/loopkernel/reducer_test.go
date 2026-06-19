package loopkernel

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/safety"
)

func TestReduceEventsSortsAndProjectsAuthority(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	localization := LocalizationAuthorityView{
		State:             LocalizationAuthorityObservedOnly,
		ReasonCode:        "localization_observed_without_owner",
		RecommendedAction: LoopActionLocalize,
	}
	permission := DerivePermissionAuthority("test",
		safety.AskPermission("policy", "high_risk", "requires approval"),
	)
	proof := ProofCoverageAuthorityView{
		State:             ProofCoverageWeak,
		ReasonCode:        "proof_weak",
		RecommendedAction: LoopActionAddProof,
		RequiresProof:     true,
	}
	events := []LoopEvent{
		{
			ID:         "e4",
			RunID:      "run-1",
			UnitID:     "unit-1",
			Sequence:   4,
			Kind:       LoopEventProofProjected,
			ReasonCode: "proof_weak",
			Payload:    EventPayload(proof),
			At:         now.Add(4 * time.Second),
		},
		{
			ID:         "e2",
			RunID:      "run-1",
			UnitID:     "unit-1",
			Sequence:   2,
			Kind:       LoopEventLocalizationProjected,
			ReasonCode: "localization_observed_without_owner",
			Payload:    EventPayload(localization),
			At:         now.Add(2 * time.Second),
		},
		{
			ID:       "e1",
			RunID:    "run-1",
			Sequence: 1,
			Kind:     LoopEventRunSeeded,
			At:       now,
		},
		{
			ID:       "e3",
			RunID:    "run-1",
			UnitID:   "unit-1",
			Sequence: 3,
			Kind:     LoopEventPermissionDecided,
			Payload:  EventPayload(permission),
			At:       now.Add(3 * time.Second),
		},
	}

	got := ReduceEvents(events)
	if got.RunID != "run-1" {
		t.Fatalf("run id = %q", got.RunID)
	}
	if got.Status != LoopRunStatusInProgress {
		t.Fatalf("status = %s, want in_progress", got.Status)
	}
	if got.ActiveUnitID != "unit-1" {
		t.Fatalf("active unit = %q", got.ActiveUnitID)
	}
	if got.Localization.State != LocalizationAuthorityObservedOnly {
		t.Fatalf("localization not projected: %+v", got.Localization)
	}
	if got.Permission.State != PermissionAuthorityAsk || !got.Permission.RequiresUser {
		t.Fatalf("permission not projected: %+v", got.Permission)
	}
	if got.Proof.State != ProofCoverageWeak || !got.Proof.RequiresProof {
		t.Fatalf("proof not projected: %+v", got.Proof)
	}
	if len(got.Units) != 1 || got.Units[0].Status != RuntimeUnitObserving {
		t.Fatalf("unit view = %+v, want one observing unit", got.Units)
	}
}

func TestReduceEventsTerminalRunStates(t *testing.T) {
	complete := ReduceEvents([]LoopEvent{{
		RunID: "run-complete",
		Kind:  LoopEventRunCompleted,
	}})
	if complete.Status != LoopRunStatusComplete {
		t.Fatalf("complete status = %s", complete.Status)
	}
	blocked := ReduceEvents([]LoopEvent{{
		RunID: "run-blocked",
		Kind:  LoopEventRunBlocked,
	}})
	if blocked.Status != LoopRunStatusBlocked {
		t.Fatalf("blocked status = %s", blocked.Status)
	}
}

func TestLoopEventStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.json")
	events := []LoopEvent{
		{ID: "later", RunID: "run-1", Sequence: 2, Kind: LoopEventRunCompleted},
		{ID: "seed", RunID: "run-1", Sequence: 1, Kind: LoopEventRunSeeded},
	}
	if err := WriteLoopEventsToFile(events, path); err != nil {
		t.Fatalf("WriteLoopEventsToFile: %v", err)
	}
	got, err := LoadLoopEventsFromFile(path)
	if err != nil {
		t.Fatalf("LoadLoopEventsFromFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d events", len(got))
	}
	if got[0].ID != "seed" || got[1].ID != "later" {
		t.Fatalf("events not normalized/sorted: %+v", got)
	}
	if state := ReduceEvents(got); state.Status != LoopRunStatusComplete {
		t.Fatalf("replayed status = %s", state.Status)
	}
}
