package types

import "testing"

func TestTerminationProfile_KindWritePreservesDegradation(t *testing.T) {
	m := NewMutableState("term")
	m.SetTerminationProfile(TerminationProfile{Kind: TerminationStopCondition})
	m.MarkTerminationFloorDegraded("ratio 20% < 50%")
	// A later kind write (e.g. hard stall after stop condition) must not
	// erase the degradation flag.
	m.SetTerminationProfile(TerminationProfile{Kind: TerminationHardStall})
	tp := m.TerminationProfile()
	if tp == nil || tp.Kind != TerminationHardStall {
		t.Fatalf("kind should follow the latest writer, got %+v", tp)
	}
	if !tp.FloorDegraded || tp.Detail == "" {
		t.Fatalf("degradation must survive kind rewrites, got %+v", tp)
	}
}

func TestNormalizeTerminationKind_BoundsToEnum(t *testing.T) {
	if got := NormalizeTerminationKind("weird"); got != TerminationNormal {
		t.Fatalf("unknown kind must normalize, got %s", got)
	}
	if got := NormalizeTerminationKind(TerminationBlockedDAG); got != TerminationBlockedDAG {
		t.Fatalf("enum kinds pass through, got %s", got)
	}
}
