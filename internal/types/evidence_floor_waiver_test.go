// Package types — evidence_floor_waiver_test.go (2026-05-10).
//
// Pins the typed escape lane contract: enum validation, IsActive
// semantics, defensive copy on MutableState, value-list canonicality
// so schema description and validator stay in sync.
package types

import (
	"strings"
	"testing"
)

func TestEvidenceFloorWaiverReason_IsValid(t *testing.T) {
	for _, r := range EvidenceFloorWaiverReasonValues() {
		if !r.IsValid() {
			t.Errorf("declared enum value %q must report IsValid()=true", r)
		}
	}
	for _, bad := range []EvidenceFloorWaiverReason{
		"",
		"external",      // missing _only_log
		"foo",
		"EXTERNAL_ONLY_LOG", // case-sensitive
		"external_log",  // missing _only_
	} {
		if bad.IsValid() {
			t.Errorf("unrecognised value %q must report IsValid()=false", bad)
		}
	}
}

func TestEvidenceFloorWaiverReason_ValuesIncludesAllFour(t *testing.T) {
	values := EvidenceFloorWaiverReasonValues()
	want := map[EvidenceFloorWaiverReason]bool{
		EvidenceFloorWaiverExternalLog:          true,
		EvidenceFloorWaiverExternalTrace:        true,
		EvidenceFloorWaiverNoRepoIntersection:   true,
		EvidenceFloorWaiverInformationalRuntime: true,
	}
	for _, v := range values {
		delete(want, v)
	}
	if len(want) > 0 {
		t.Errorf("declared values missing from EvidenceFloorWaiverReasonValues: %v", want)
	}
	if len(values) != 4 {
		t.Errorf("expected exactly 4 reasons, got %d", len(values))
	}
}

func TestEvidenceFloorWaiverReason_ValuesIsCopy(t *testing.T) {
	a := EvidenceFloorWaiverReasonValues()
	b := EvidenceFloorWaiverReasonValues()
	if len(a) > 0 {
		a[0] = "mutated"
	}
	if b[0] == "mutated" {
		t.Error("EvidenceFloorWaiverReasonValues must return defensive copies; caller mutation leaked")
	}
}

func TestEvidenceFloorWaiver_IsActive(t *testing.T) {
	// Happy path: valid reason + non-empty rationale → active.
	good := &EvidenceFloorWaiver{
		Reason:    EvidenceFloorWaiverExternalLog,
		Rationale: "log frames reference services not defined in this repo",
	}
	if !good.IsActive() {
		t.Error("valid waiver must report IsActive()=true")
	}

	// Nil receiver.
	var nilW *EvidenceFloorWaiver
	if nilW.IsActive() {
		t.Error("nil waiver must report IsActive()=false")
	}

	// Empty rationale.
	noRat := &EvidenceFloorWaiver{Reason: EvidenceFloorWaiverExternalLog}
	if noRat.IsActive() {
		t.Error("waiver without rationale must report IsActive()=false (audit trail required)")
	}
	whiteRat := &EvidenceFloorWaiver{Reason: EvidenceFloorWaiverExternalLog, Rationale: "   "}
	if whiteRat.IsActive() {
		t.Error("waiver with whitespace-only rationale must report IsActive()=false")
	}

	// Bad reason.
	badReason := &EvidenceFloorWaiver{Reason: "bogus", Rationale: "rationale"}
	if badReason.IsActive() {
		t.Error("waiver with invalid reason must report IsActive()=false")
	}

	// Empty reason.
	emptyReason := &EvidenceFloorWaiver{Rationale: "rationale"}
	if emptyReason.IsActive() {
		t.Error("waiver with empty reason must report IsActive()=false")
	}
}

func TestMutableState_EvidenceFloorWaiverRoundTrip(t *testing.T) {
	m := NewMutableState("test")
	if m.EvidenceFloorWaiver() != nil {
		t.Error("fresh MutableState must report nil waiver")
	}

	in := &EvidenceFloorWaiver{
		Reason:    EvidenceFloorWaiverNoRepoIntersection,
		Rationale: "synthetic log frames represent different build",
	}
	m.SetEvidenceFloorWaiver(in)

	out := m.EvidenceFloorWaiver()
	if out == nil {
		t.Fatal("waiver lost on round-trip")
	}
	if out.Reason != in.Reason {
		t.Errorf("Reason round-trip: got %q want %q", out.Reason, in.Reason)
	}
	if out.Rationale != in.Rationale {
		t.Errorf("Rationale round-trip: got %q want %q", out.Rationale, in.Rationale)
	}
}

func TestMutableState_EvidenceFloorWaiverDefensiveCopy(t *testing.T) {
	// Caller mutation of the original input must not leak into stored state.
	m := NewMutableState("test")
	in := &EvidenceFloorWaiver{
		Reason:    EvidenceFloorWaiverExternalLog,
		Rationale: "original",
	}
	m.SetEvidenceFloorWaiver(in)
	in.Rationale = "mutated by caller after Set"

	out := m.EvidenceFloorWaiver()
	if out.Rationale != "original" {
		t.Errorf("stored rationale corrupted by caller mutation: got %q", out.Rationale)
	}

	// Read-side mutation must not leak back either.
	out.Rationale = "mutated by reader"
	out2 := m.EvidenceFloorWaiver()
	if out2.Rationale != "original" {
		t.Errorf("stored rationale corrupted by reader mutation: got %q", out2.Rationale)
	}
}

func TestMutableState_EvidenceFloorWaiverNilClears(t *testing.T) {
	m := NewMutableState("test")
	m.SetEvidenceFloorWaiver(&EvidenceFloorWaiver{
		Reason:    EvidenceFloorWaiverExternalLog,
		Rationale: "rationale",
	})
	m.SetEvidenceFloorWaiver(nil)
	if got := m.EvidenceFloorWaiver(); got != nil {
		t.Errorf("nil Set must clear waiver, got %+v", got)
	}
}

func TestMutableState_EvidenceFloorWaiverNilSafe(t *testing.T) {
	var m *MutableState
	m.SetEvidenceFloorWaiver(&EvidenceFloorWaiver{Reason: EvidenceFloorWaiverExternalLog, Rationale: "x"})
	if got := m.EvidenceFloorWaiver(); got != nil {
		t.Errorf("nil MutableState must report nil waiver, got %+v", got)
	}
}

func TestEvidenceFloorWaiver_R6Audit(t *testing.T) {
	// The reason enum values are LLM-facing (they appear in the
	// schema description and the LLM types them on the wire).
	// They must NOT carry internal pipeline jargon — keep them
	// neutral / domain-meaningful so an operator reading a waiver
	// log line understands what the model claimed.
	for _, r := range EvidenceFloorWaiverReasonValues() {
		s := string(r)
		for _, banned := range []string{
			"explorer", "extractor", "finalizer", "analyzer",
			"BusContext", "MutableState",
			"forced_read", "citation_floor", "L1", "L2", "L3", "L4",
		} {
			if strings.Contains(s, banned) {
				t.Errorf("internal term %q leaked into LLM-facing reason value %q", banned, s)
			}
		}
	}
}
