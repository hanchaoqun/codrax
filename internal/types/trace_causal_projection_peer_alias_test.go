package types

// trace_causal_projection_peer_alias_test.go — Lane R R4-2 (2026-07-03, H18):
// the readfile_de E1/E2 shape where ONE monitor contention renders twice
// because the lock owner is written two ways ("NetworkKit_AssetsUtil_
// Operate_0-42067" vs "pid=42067"). The aggregation layer folds the pid-handle
// variant into the named variant when subject, TypeToken, the integer pid and
// the time span all agree.

import "testing"

func peerAliasNode(id, peer string, impact, startTs, endTs float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID: id, Subject: "NWeb_RenderMain-42591", Predicate: "critical_blocking",
		Object: "monitor_contention", TypeToken: "blocking_span",
		BlockingKind: "monitor_contention", BlockingPeer: peer,
		ImpactMS: impact, CumulativeImpactMS: impact,
		StartTs: startTs, EndTs: endTs, ChainRelevance: "on_chain",
	}
}

func TestTraceCausalProjectionMergePeerAliasesFoldsPidVariant(t *testing.T) {
	named := peerAliasNode("E1", "NetworkKit_AssetsUtil_Operate_0-42067", 112.223, 10.000, 10.200)
	pidRow := peerAliasNode("E2", "pid=42067", 112.214, 10.050, 10.210)
	out := traceCausalProjectionMergePeerAliases([]TraceCausalProjectionNode{named, pidRow})
	if len(out) != 1 {
		t.Fatalf("alias pair must fold to one row: %+v", out)
	}
	got := out[0]
	if got.BlockingPeer != "NetworkKit_AssetsUtil_Operate_0-42067" || got.EvidenceID != "E1" {
		t.Fatalf("the NAMED variant must survive: %+v", got)
	}
	if got.ImpactMS != 112.223 || got.CumulativeImpactMS != 112.223 {
		t.Fatalf("projected impact must keep the larger measurement: %+v", got)
	}
	if len(got.MergedEvidenceIDs) != 1 || got.MergedEvidenceIDs[0] != "E2" {
		t.Fatalf("loser evidence id must union in: %+v", got.MergedEvidenceIDs)
	}

	// Order-independent: pid variant first still folds into the named one, and
	// a larger pid-side measurement is kept as the max.
	pidFirst := peerAliasNode("E2", "pid=42067", 112.230, 10.050, 10.210)
	out = traceCausalProjectionMergePeerAliases([]TraceCausalProjectionNode{pidFirst, named})
	if len(out) != 1 || out[0].BlockingPeer != "NetworkKit_AssetsUtil_Operate_0-42067" {
		t.Fatalf("pid-first order must still keep the named variant: %+v", out)
	}
	if out[0].ImpactMS != 112.230 {
		t.Fatalf("max measurement must win regardless of which side carried it: %+v", out[0])
	}
}

func TestTraceCausalProjectionMergePeerAliasesPreciseGuards(t *testing.T) {
	named := peerAliasNode("E1", "NetworkKit_AssetsUtil_Operate_0-42067", 112.223, 10.000, 10.200)
	cases := map[string]TraceCausalProjectionNode{
		"different pid integer": peerAliasNode("E2", "pid=42068", 112.214, 10.050, 10.210),
		"no span overlap":       peerAliasNode("E2", "pid=42067", 112.214, 10.300, 10.400),
		"different subject": func() TraceCausalProjectionNode {
			n := peerAliasNode("E2", "pid=42067", 112.214, 10.050, 10.210)
			n.Subject = "OtherThread-99"
			return n
		}(),
		"different type token": func() TraceCausalProjectionNode {
			n := peerAliasNode("E2", "pid=42067", 112.214, 10.050, 10.210)
			n.TypeToken = "d_state_or_io_wait"
			return n
		}(),
		"missing own span": peerAliasNode("E2", "pid=42067", 112.214, 0, 0),
		"two named peers":  peerAliasNode("E2", "OtherOwner-42067", 112.214, 10.050, 10.210),
	}
	for name, other := range cases {
		out := traceCausalProjectionMergePeerAliases([]TraceCausalProjectionNode{named, other})
		if len(out) != 2 {
			t.Fatalf("%s: rows must NOT merge: %+v", name, out)
		}
	}
}

func TestTraceCausalProjectionAggregateForPresentationFoldsPeerAliases(t *testing.T) {
	projection := TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{
			peerAliasNode("E1", "NetworkKit_AssetsUtil_Operate_0-42067", 112.223, 10.000, 10.200),
			peerAliasNode("E2", "pid=42067", 112.214, 10.050, 10.210),
		},
	}
	traceCausalProjectionAggregateForPresentation(&projection)
	if len(projection.OnChainCauses) != 1 {
		t.Fatalf("end-to-end aggregation must fold the alias pair: %+v", projection.OnChainCauses)
	}
	if projection.OnChainCauses[0].BlockingPeer != "NetworkKit_AssetsUtil_Operate_0-42067" {
		t.Fatalf("named variant must survive end to end: %+v", projection.OnChainCauses[0])
	}
}
