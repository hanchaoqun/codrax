package types

// trace_causal_projection_hullcred_test.go — HULL-CRED (§29.104 终判③,
// 2026-07-17) strict-decode pins: the chain_credential_segments note decodes
// ALL-OR-NOTHING (any malformed entry, or a set beyond the engine-mirrored
// cap, yields nil — a partial or corrupt inventory could fake a per-segment
// adjudication), and the three notes land on their typed node fields from the
// production emission form.

import (
	"fmt"
	"strings"
	"testing"
)

func TestHULLCREDCredentialSegmentsStrictDecode(t *testing.T) {
	// Production emission form (criticalBlockingCredentialSegmentEntries +
	// "|" join): every entry parses to its exact pair.
	got := traceCausalProjectionParseCredentialSegments("1.002000..1.012000|1.032000..1.045000")
	if len(got) != 2 || got[0] != [2]float64{1.002, 1.012} || got[1] != [2]float64{1.032, 1.045} {
		t.Fatalf("valid inventory must decode to exact pairs: %+v", got)
	}
	// EVOLUTION RECORD (§29.183 G8, 2026-07-21): the former "zero start"
	// reject arm inverted — a segment starting at exactly ts=0 is a legal
	// timestamp in a rebased trace, and the all-or-nothing INTEGRITY
	// semantics live in the parse errors / end>start / whole-set nil-out,
	// not in a start>0 boundary. The (0,0) zero pair still nils the set.
	for name, raw := range map[string]string{
		"empty":            "",
		"missing dots":     "1.002000-1.012000",
		"bad float":        "1.002000..x",
		"zero pair":        "0.000000..0.000000",
		"negative start":   "-1.0..2.0",
		"end == start":     "1.002000..1.002000",
		"end < start":      "1.012000..1.002000",
		"one bad of two":   "1.002000..1.012000|9..8",
		"trailing garbage": "1.002000..1.012000|",
	} {
		if out := traceCausalProjectionParseCredentialSegments(raw); out != nil {
			t.Fatalf("%s must decode to nil (all-or-nothing), got %+v", name, out)
		}
	}
	if out := traceCausalProjectionParseCredentialSegments("0.000000..1.012000"); len(out) != 1 || out[0] != [2]float64{0, 1.012} {
		t.Fatalf("a rebased zero-start segment must decode (§29.183 G8), got %+v", out)
	}
	// Over-cap sets reject WHOLE (the engine never mints one — corrupt or
	// foreign artifacts must not adjudicate).
	var over []string
	for i := 0; i <= TraceCausalProjectionChainCredentialSegmentCap; i++ {
		over = append(over, fmt.Sprintf("%.6f..%.6f", 1.0+float64(i)*0.002, 1.001+float64(i)*0.002))
	}
	if out := traceCausalProjectionParseCredentialSegments(strings.Join(over, "|")); out != nil {
		t.Fatalf("over-cap inventory must decode to nil, got %d entries", len(out))
	}
	// At-cap sets decode (the engine's maximum legal emission).
	if out := traceCausalProjectionParseCredentialSegments(strings.Join(over[:TraceCausalProjectionChainCredentialSegmentCap], "|")); len(out) != TraceCausalProjectionChainCredentialSegmentCap {
		t.Fatalf("at-cap inventory must decode whole, got %d entries", len(out))
	}
}

// TestHULLCREDNotesLandOnNodeFields — the three chain_credential_* notes
// decode from a production-shaped observation record onto the typed node
// trio; records without them stay zero-valued (absence never judges).
func TestHULLCREDNotesLandOnNodeFields(t *testing.T) {
	records := []ObservationRecord{{
		ID:        "obs-hullcred-1",
		Subject:   "worker-200",
		Predicate: "critical_blocking d_state_or_io_wait",
		RichNotes: []string{
			"type=d_state_or_io_wait",
			"thread=worker-200",
			"chain_relevance=adjacent",
			"chain_credential_lane_demoted=true",
			"chain_credential_segments=1.002000..1.012000|1.032000..1.045000",
			"chain_credential_segment_disjoint=true",
			"impact_ms=23.000",
		},
	}, {
		ID:        "obs-hullcred-2",
		Subject:   "env-500",
		Predicate: "critical_blocking io_wait",
		RichNotes: []string{
			"type=io_wait",
			"thread=env-500",
			"chain_relevance=on_chain",
			"chain_credential_envelope_level=true",
			"impact_ms=66.000",
		},
	}, {
		ID:        "obs-hullcred-3",
		Subject:   "plain-600",
		Predicate: "critical_blocking d_state_or_io_wait",
		RichNotes: []string{
			"type=d_state_or_io_wait",
			"thread=plain-600",
			"chain_relevance=on_chain",
			"impact_ms=4.000",
		},
	}}
	byID := map[string]TraceCausalProjectionNode{}
	for _, record := range records {
		node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
		byID[record.ID] = node
	}
	disjoint := byID["obs-hullcred-1"]
	if !disjoint.ChainCredentialLaneDemoted || !disjoint.ChainCredentialSegmentDisjoint {
		t.Fatalf("disjoint markers must decode: %+v", disjoint)
	}
	if len(disjoint.ChainCredentialSegments) != 2 || disjoint.ChainCredentialSegments[0] != [2]float64{1.002, 1.012} {
		t.Fatalf("segment inventory must decode beside the marker: %+v", disjoint.ChainCredentialSegments)
	}
	if disjoint.ChainCredentialEnvelopeLevel {
		t.Fatalf("the demoted record must not wear the envelope marker: %+v", disjoint)
	}
	envelope := byID["obs-hullcred-2"]
	if !envelope.ChainCredentialEnvelopeLevel || envelope.ChainCredentialSegmentDisjoint || len(envelope.ChainCredentialSegments) != 0 {
		t.Fatalf("envelope marker must decode alone: %+v", envelope)
	}
	plain := byID["obs-hullcred-3"]
	if plain.ChainCredentialEnvelopeLevel || plain.ChainCredentialSegmentDisjoint || len(plain.ChainCredentialSegments) != 0 || plain.ChainCredentialLaneDemoted {
		t.Fatalf("legacy records must stay zero-valued on the trio (absence never judges): %+v", plain)
	}
}
