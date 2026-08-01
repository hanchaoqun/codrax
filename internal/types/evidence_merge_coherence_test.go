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
