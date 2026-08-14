package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeCallChainEndpointFromRequiredDiagramParticipantsUsesUniqueOtherParticipant(t *testing.T) {
	profile := &types.CallChainEndpointProfile{
		Source:   "buildAnalysisIR",
		SinkMode: types.CallChainSinkResolutionDiscover,
	}
	hint := &types.DiagramHint{
		Kind:     types.DiagramSequence,
		Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "buildAnalysisIR", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "buildAnalysisIR 到 gate.Run"},
			{Identity: "gate.Run", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "buildAnalysisIR 到 gate.Run"},
		},
	}

	got, warning := normalizeCallChainEndpointFromRequiredDiagramParticipants(
		"call_chain", types.AxisCall, profile, hint,
	)
	if got == nil || !got.ExactActive() || got.Source != "buildAnalysisIR" || got.Sink != "gate.Run" {
		t.Fatalf("required participant pair was not normalized to exact endpoints: %+v", got)
	}
	if !strings.Contains(warning, "required typed call diagram") {
		t.Fatalf("normalization must remain auditable: %q", warning)
	}
	if profile.Sink != "" || profile.SinkMode != types.CallChainSinkResolutionDiscover {
		t.Fatalf("normalization mutated the caller-owned profile: %+v", profile)
	}
}

func TestNormalizeCallChainEndpointFromRequiredDiagramParticipantsFailsOpenWithoutExactPair(t *testing.T) {
	base := &types.CallChainEndpointProfile{
		Source:   "pkg.Start",
		SinkMode: types.CallChainSinkResolutionDiscoverTerminal,
	}
	tests := []struct {
		name string
		hint *types.DiagramHint
	}{
		{
			name: "optional",
			hint: &types.DiagramHint{Kind: types.DiagramSequence, Participants: []types.DiagramParticipantHint{
				{Identity: "pkg.Start", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "pkg.End", Role: types.DiagramParticipantIncidentRequired},
			}},
		},
		{
			name: "context participant",
			hint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: []types.DiagramParticipantHint{
				{Identity: "pkg.Start", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "pkg.End", Role: types.DiagramParticipantContextOnly},
			}},
		},
		{
			name: "three incident participants",
			hint: &types.DiagramHint{Kind: types.DiagramCallDAG, Required: true, Participants: []types.DiagramParticipantHint{
				{Identity: "pkg.Start", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "pkg.Middle", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "pkg.End", Role: types.DiagramParticipantIncidentRequired},
			}},
		},
		{
			name: "non-call visual",
			hint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{
				{Identity: "pkg.Start", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "pkg.End", Role: types.DiagramParticipantIncidentRequired},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, warning := normalizeCallChainEndpointFromRequiredDiagramParticipants(
				"call_chain", types.AxisCall, base, tc.hint,
			)
			if got != base || warning != "" {
				t.Fatalf("ambiguous/non-authoritative diagram must fail open: got=%+v warning=%q", got, warning)
			}
		})
	}
}
