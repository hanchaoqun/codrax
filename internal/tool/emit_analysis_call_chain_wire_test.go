package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeCallChainEndpointWireShapePreservesSingleNamedSource(t *testing.T) {
	for _, tc := range []struct {
		name             string
		inputMode        types.CallChainSinkResolutionMode
		runtimeSelection bool
		wantMode         types.CallChainSinkResolutionMode
	}{
		{name: "path mode becomes static conceptual terminal", inputMode: types.CallChainSinkResolutionDiscoverPath, wantMode: types.CallChainSinkResolutionDiscoverTerminal},
		{name: "path mode becomes runtime-selected destination", inputMode: types.CallChainSinkResolutionDiscoverPath, runtimeSelection: true, wantMode: types.CallChainSinkResolutionDiscover},
		{name: "false selection corrects discover", inputMode: types.CallChainSinkResolutionDiscover, wantMode: types.CallChainSinkResolutionDiscoverTerminal},
		{name: "true selection corrects discover terminal", inputMode: types.CallChainSinkResolutionDiscoverTerminal, runtimeSelection: true, wantMode: types.CallChainSinkResolutionDiscover},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := &types.CallChainEndpointProfile{
				Source:                   "FastTokenizer.tokenize",
				SinkMode:                 tc.inputMode,
				RuntimeSelectionRequired: tc.runtimeSelection,
			}
			got, warning := normalizeCallChainEndpointWireShape(profile)
			if got == nil || got.Source != profile.Source || got.Sink != "" || got.SinkMode != tc.wantMode {
				t.Fatalf("single named source was not normalized to the typed discovery lane: %+v", got)
			}
			if !strings.Contains(warning, "structured carrier preserves one named source") {
				t.Fatalf("normalization must remain auditable: %q", warning)
			}
			if profile.SinkMode != tc.inputMode {
				t.Fatalf("normalization mutated the caller-owned profile: %+v", profile)
			}
		})
	}
}

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
		"call_chain", types.AxisCall, profile, hint, nil,
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
				"call_chain", types.AxisCall, base, tc.hint, nil,
			)
			if got != base || warning != "" {
				t.Fatalf("ambiguous/non-authoritative diagram must fail open: got=%+v warning=%q", got, warning)
			}
		})
	}
}

func TestNormalizeCallChainEndpointFromRequiredDiagramRelationScopeUsesUniqueTypedPair(t *testing.T) {
	profile := &types.CallChainEndpointProfile{
		Source:   "buildAnalysisIR",
		SinkMode: types.CallChainSinkResolutionDiscover,
	}
	hint := &types.DiagramHint{
		Kind:               types.DiagramSequence,
		Required:           true,
		RelationScopeQuote: "buildAnalysisIR 到 gate.Run 的调用顺序",
		Participants:       []types.DiagramParticipantHint{},
	}

	got, warning := normalizeCallChainEndpointFromRequiredDiagramParticipants(
		"call_chain",
		types.AxisCall,
		profile,
		hint,
		[]string{"internal/agent/analyzer.go", "buildAnalysisIR", "gate.Run", "unrelated.Helper"},
	)
	if got == nil || !got.ExactActive() || got.Source != "buildAnalysisIR" || got.Sink != "gate.Run" {
		t.Fatalf("relation-scoped typed pair was not normalized to exact endpoints: %+v", got)
	}
	if !strings.Contains(warning, "relation scope contains one unique other typed code identity") {
		t.Fatalf("relation-scope normalization must remain auditable: %q", warning)
	}
}

func TestNormalizeCallChainEndpointFromRequiredDiagramRelationScopeFailsOpenOnAmbiguity(t *testing.T) {
	profile := &types.CallChainEndpointProfile{
		Source:   "pkg.Start",
		SinkMode: types.CallChainSinkResolutionDiscover,
	}
	hint := &types.DiagramHint{
		Kind:               types.DiagramSequence,
		Required:           true,
		RelationScopeQuote: "pkg.Start 到 svc.End 和 audit.End 的调用顺序",
		Participants:       []types.DiagramParticipantHint{},
	}

	got, warning := normalizeCallChainEndpointFromRequiredDiagramParticipants(
		"call_chain",
		types.AxisCall,
		profile,
		hint,
		[]string{"pkg.Start", "svc.End", "audit.End"},
	)
	if got != profile || warning != "" {
		t.Fatalf("ambiguous relation-scoped entities must fail open: got=%+v warning=%q", got, warning)
	}
}
