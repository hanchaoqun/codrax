package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestViolationPolicyCoverageReportCoversEveryKind(t *testing.T) {
	rows := buildViolationPolicyCoverageReport()
	if len(rows) != len(types.AllViolationKinds()) {
		t.Fatalf("coverage row count = %d, want %d", len(rows), len(types.AllViolationKinds()))
	}
	seen := map[types.ViolationKind]bool{}
	for _, row := range rows {
		if seen[row.Kind] {
			t.Fatalf("duplicate coverage row for %q", row.Kind)
		}
		seen[row.Kind] = true
		if _, ok := types.ViolKindSpecFor(row.Kind); !ok {
			t.Fatalf("coverage row for unregistered kind %q", row.Kind)
		}
		if !row.FallbackTarget.IsValid() {
			t.Fatalf("coverage row %q has invalid fallback target %q", row.Kind, row.FallbackTarget)
		}
		if row.FallbackTarget == FallbackBackToAnalyze {
			t.Fatalf("coverage row %q routes to analyzer reclassification, which is not an allowed automatic fallback", row.Kind)
		}
	}
	for _, kind := range types.AllViolationKinds() {
		if !seen[kind] {
			t.Fatalf("missing coverage row for %q", kind)
		}
	}
}

func TestViolationPolicyCoverageReportDefaultHasNoHiddenHardRows(t *testing.T) {
	var hard []violationPolicyCoverageRow
	for _, row := range buildViolationPolicyCoverageReport() {
		if !row.RuntimeSoftDefault {
			hard = append(hard, row)
		}
	}
	if len(hard) != 0 {
		t.Fatalf("default commercial post-emit policy should not hide hard retry rows; got %+v", hard)
	}
}

func TestViolationPolicyCoverageReportPinsPresentationKinds(t *testing.T) {
	rows := buildViolationPolicyCoverageReport()
	byKind := map[types.ViolationKind]violationPolicyCoverageRow{}
	for _, row := range rows {
		byKind[row.Kind] = row
	}
	for _, kind := range []types.ViolationKind{
		types.ViolCitation,
		types.ViolCardinalityShort,
		types.ViolDiagramEdgeEndpointHallucinated,
		types.ViolDiagramRelationLabelOnly,
	} {
		row, ok := byKind[kind]
		if !ok {
			t.Fatalf("missing coverage row for %q", kind)
		}
		if !row.RuntimeSoftDefault {
			t.Fatalf("presentation/noisy kind %q must be soft in default coverage: %+v", kind, row)
		}
	}
}
