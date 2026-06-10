package types

import (
	"strings"
	"testing"
)

func TestAddPreStageDegradation_BoundsAndKinds(t *testing.T) {
	m := NewMutableState("deg")
	m.AddPreStageDegradation(PreStageDegradation{Stage: StageLogTriage, Kind: "weird", Summary: strings.Repeat("x", 1000)})
	got := m.PreStageDegradations()
	if len(got) != 1 {
		t.Fatalf("expected one record, got %d", len(got))
	}
	if got[0].Kind != PreStageDegradationDispatchError {
		t.Fatalf("unknown kind must normalize to dispatch_error, got %s", got[0].Kind)
	}
	if len(got[0].Summary) > 410 {
		t.Fatalf("summary must be bounded, got %d bytes", len(got[0].Summary))
	}
}

func TestLogFrames_CarryArtifactTimeWindow(t *testing.T) {
	b := &PerfBundle{Stalls: []PerfStall{{Symbol: "DataLoader.load", File: "src/loader.ets", Line: 42, StartTsMs: 928080, DurationMs: 350}}}
	frames := b.LogFrames()
	if len(frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(frames))
	}
	if frames[0].ArtifactStartTsMs != 928080 || frames[0].ArtifactDurationMs != 350 {
		t.Fatalf("trace-projected frame must keep its time window, got %+v", frames[0])
	}
}
