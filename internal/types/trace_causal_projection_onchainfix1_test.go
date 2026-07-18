package types

// trace_causal_projection_onchainfix1_test.go — ONCHAIN-FIX-1 件1 decode pin
// (2026-07-18): the chain_identity_inheritance note lands on its typed node
// field from the production emission form; records without it stay
// zero-valued (absence never judges).

import "testing"

func TestONCHAINFIX1IdentityInheritanceNoteLandsOnNodeField(t *testing.T) {
	marked := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, ObservationRecord{
		ID:        "obs-ofix1-1",
		Subject:   "worker-200",
		Predicate: "critical_blocking d_state_or_io_wait",
		RichNotes: []string{
			"type=d_state_or_io_wait",
			"thread=worker-200",
			"chain_relevance=on_chain",
			"chain_identity_inheritance=true",
			"impact_ms=4.000",
		},
	})
	if !marked.ChainIdentityInheritance {
		t.Fatalf("the identity-inheritance marker must decode: %+v", marked)
	}
	plain := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, ObservationRecord{
		ID:        "obs-ofix1-2",
		Subject:   "plain-600",
		Predicate: "critical_blocking d_state_or_io_wait",
		RichNotes: []string{
			"type=d_state_or_io_wait",
			"thread=plain-600",
			"chain_relevance=on_chain",
			"impact_ms=4.000",
		},
	})
	if plain.ChainIdentityInheritance {
		t.Fatalf("absence never judges — legacy records stay zero-valued: %+v", plain)
	}
}
