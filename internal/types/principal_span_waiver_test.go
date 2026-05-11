package types

import "testing"

func TestPrincipalSpanWaiverReason_IsValid(t *testing.T) {
	for _, reason := range PrincipalSpanWaiverReasonValues() {
		if !reason.IsValid() {
			t.Errorf("PrincipalSpanWaiverReasonValues() returned %q but IsValid()=false", reason)
		}
	}
	for _, bad := range []PrincipalSpanWaiverReason{
		"", "unknown_reason", "EndpointsDirectlyAdjacent", "external_log",
	} {
		if bad.IsValid() {
			t.Errorf("PrincipalSpanWaiverReason(%q).IsValid() = true; want false", bad)
		}
	}
}

func TestPrincipalSpanWaiverReasonValues_IsCanonical(t *testing.T) {
	expected := []PrincipalSpanWaiverReason{
		PrincipalSpanWaiverEndpointsDirectlyAdjacent,
		PrincipalSpanWaiverNoIntermediateUserCode,
		PrincipalSpanWaiverPlatformBridgeIntermediates,
		PrincipalSpanWaiverInlinedCall,
		PrincipalSpanWaiverRuntimeDispatchedCall,
		PrincipalSpanWaiverExternalModuleContinuation,
	}
	got := PrincipalSpanWaiverReasonValues()
	if len(got) != len(expected) {
		t.Fatalf("PrincipalSpanWaiverReasonValues() len=%d want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("PrincipalSpanWaiverReasonValues()[%d] = %q; want %q", i, got[i], expected[i])
		}
	}
}

func TestPrincipalSpanWaiver_IsActive(t *testing.T) {
	cases := []struct {
		name string
		w    *PrincipalSpanWaiver
		want bool
	}{
		{"nil", nil, false},
		{"zero value", &PrincipalSpanWaiver{}, false},
		{"valid reason no rationale", &PrincipalSpanWaiver{
			Reason: PrincipalSpanWaiverInlinedCall,
		}, false},
		{"valid reason whitespace rationale", &PrincipalSpanWaiver{
			Reason: PrincipalSpanWaiverInlinedCall, Rationale: "   \t\n",
		}, false},
		{"invalid reason with rationale", &PrincipalSpanWaiver{
			Reason: "made_up", Rationale: "some explanation",
		}, false},
		{"valid + rationale", &PrincipalSpanWaiver{
			Reason:    PrincipalSpanWaiverEndpointsDirectlyAdjacent,
			Rationale: "source and sink are on the same statement",
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.w.IsActive(); got != c.want {
				t.Errorf("IsActive() = %v; want %v", got, c.want)
			}
		})
	}
}
