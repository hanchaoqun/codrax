package types

import (
	"strings"
	"testing"
)

func TestMergeEvidenceItemByStableIDSparseConditionRetryCannotSplitCarrier(t *testing.T) {
	base := EvidenceItem{
		ID:              "ev-config-guard",
		Kind:            EvidenceConditional,
		Scope:           ScopeLine,
		Subject:         "pipeline-max-steps override guard",
		Predicate:       "guards",
		Condition:       `!cmd.Flags().Changed("pipeline-max-steps")`,
		Source:          "cmd/root.go",
		LineStart:       2664,
		LineEnd:         2664,
		AnchorKind:      AnchorCondition,
		AnchorSymbol:    "Changed",
		OwnerSymbol:     "run",
		Snippet:         `if !cmd.Flags().Changed("pipeline-max-steps") {`,
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}
	sparse := base
	sparse.Subject = "flagMaxSteps"
	sparse.Predicate = "assigns"
	sparse.Object = "mergedMaxSteps"
	sparse.Condition = ""
	sparse.AnchorSymbol = "flagMaxSteps"
	sparse.OwnerSymbol = ""
	sparse.Snippet = "if !cmd.Flags().Changed(\"pipeline-max-steps\") {\n\tflagMaxSteps = mergedMaxSteps\n}"

	got := MergeEvidenceItemByStableID(base, sparse)
	if got.Condition != base.Condition || got.AnchorSymbol != "Changed" {
		t.Fatalf("sparse retry split the typed condition carrier: %+v", got)
	}
	if got.Subject != base.Subject || got.Predicate != base.Predicate || got.Object != "" || got.Snippet != base.Snippet {
		t.Fatalf("sparse retry overwrote coherent claim fields: %+v", got)
	}
}

func TestMergeEvidenceItemByStableIDRicherCorrectionReplacesCarrierAtomically(t *testing.T) {
	base := EvidenceItem{
		ID:              "ev-corrected-statement",
		Kind:            EvidenceConditional,
		Scope:           ScopeLine,
		Subject:         "old guard",
		Predicate:       "guards",
		Condition:       "legacyEnabled",
		Source:          "config.go",
		LineStart:       42,
		AnchorKind:      AnchorCondition,
		AnchorSymbol:    "legacyEnabled",
		Snippet:         "if legacyEnabled {",
		GroundingStatus: GroundingRecovered,
		GroundingTier:   TierNearestCondition,
	}
	corrected := base
	corrected.Kind = EvidenceDirect
	corrected.Subject = "effectiveLimit"
	corrected.Predicate = "assigns"
	corrected.Object = "configuredLimit"
	corrected.Condition = ""
	corrected.AnchorKind = AnchorAssignment
	corrected.AnchorSymbol = "effectiveLimit"
	corrected.Snippet = "effectiveLimit = configuredLimit"
	corrected.GroundingStatus = GroundingGrounded
	corrected.GroundingTier = TierLineText

	got := MergeEvidenceItemByStableID(base, corrected)
	if got.AnchorKind != AnchorAssignment || got.Condition != "" || got.Object != "configuredLimit" {
		t.Fatalf("richer correction did not replace the carrier atomically: %+v", got)
	}
}

func TestMergeEvidenceItemByStableIDCompleteRelationEndpointsPromoteCarrierAtomically(t *testing.T) {
	base := EvidenceItem{
		ID:              "ev-factory-selection",
		Kind:            EvidenceRegistration,
		Scope:           ScopeLine,
		Predicate:       "constructs",
		Object:          "ConsoleSink",
		Condition:       `kind == "console"`,
		Source:          "src/registry.cpp",
		LineStart:       15,
		LineEnd:         15,
		AnchorKind:      AnchorReturn,
		AnchorSymbol:    "create",
		OwnerSymbol:     "create",
		GroundingStatus: GroundingGrounded,
		GroundingTier:   TierLineText,
	}
	complete := base
	complete.Subject = "SinkRegistry::create"
	complete.Condition = ""

	got := MergeEvidenceItemByStableID(base, complete)
	if got.Subject != "SinkRegistry::create" || got.Object != "ConsoleSink" {
		t.Fatalf("complete same-anchor relation endpoints were not promoted: %+v", got)
	}
	if got.Condition != "" {
		t.Fatalf("endpoint promotion must replace the carrier atomically, not splice the old condition: %+v", got)
	}
	if ClaimFormOf(got) != ClaimRegistrationEdge {
		t.Fatalf("promoted carrier claim form = %q, want %q", ClaimFormOf(got), ClaimRegistrationEdge)
	}
}

func TestMergeEvidenceItemByStableIDConflictingRelationEndpointDoesNotPromote(t *testing.T) {
	base := EvidenceItem{
		ID: "ev-selection", Kind: EvidenceRegistration, Scope: ScopeLine,
		Predicate: "binds", Object: "OldBackend", Condition: "enabled",
		Source: "src/registry.cpp", LineStart: 15, AnchorKind: AnchorReturn,
		AnchorSymbol: "create", GroundingStatus: GroundingGrounded,
	}
	conflict := base
	conflict.Subject = "Registry::create"
	conflict.Object = "OtherBackend"
	conflict.Condition = ""

	got := MergeEvidenceItemByStableID(base, conflict)
	if got.Subject != "" || got.Object != "OldBackend" || got.Condition != "enabled" {
		t.Fatalf("conflicting endpoint retry must not promote or splice the carrier: %+v", got)
	}
}

func TestConditionAuthoritativeSurfacesDoNotPublishBodyAssignment(t *testing.T) {
	item := EvidenceItem{
		Kind:         EvidenceConditional,
		Scope:        ScopeLine,
		Subject:      "flagMaxSteps",
		Predicate:    "assigns",
		Object:       "mergedMaxSteps",
		Condition:    `!cmd.Flags().Changed("pipeline-max-steps")`,
		Source:       "cmd/root.go",
		LineStart:    2664,
		AnchorKind:   AnchorCondition,
		AnchorSymbol: "Changed",
		Snippet:      "if !cmd.Flags().Changed(\"pipeline-max-steps\") {\n\tflagMaxSteps = mergedMaxSteps\n}",
	}
	for name, got := range map[string]string{
		"authoritative": EvidenceAuthoritativeSurfaceText(item, false),
		"resolution":    FormatExactResolutionSeed(item),
	} {
		if !strings.Contains(got, "guard condition IF") || !strings.Contains(got, "Changed") {
			t.Fatalf("%s surface lost guard authority: %q", name, got)
		}
		if strings.Contains(got, "assigns") || strings.Contains(got, "flagMaxSteps = mergedMaxSteps") {
			t.Fatalf("%s surface leaked body assignment: %q", name, got)
		}
	}
}
