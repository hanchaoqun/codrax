package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func dimensionOwnershipContext(dims ...types.RequestedAnswerDimension) *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("dimension ownership"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions:          dims,
			},
		}},
	}
}

func TestRequestedDimensionEvidenceOwnershipRequiresIndependentOperationRows(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	evidence := []types.EvidenceItem{{
		Kind: types.EvidenceRegistration, AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded,
		RequestedDimensionIndices: []int{3},
	}}
	got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence)
	if !strings.Contains(got, "1 (function_or_purpose)") || strings.Contains(got, "3 (function_or_purpose)") {
		t.Fatalf("downgrade=%q", got)
	}

	evidence = append(evidence, types.EvidenceItem{
		Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded,
		RequestedDimensionIndices: []int{1},
	})
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence); got != "" {
		t.Fatalf("independently supported dimensions should close, got %q", got)
	}
}

func TestRequestedDimensionEvidenceOwnershipDoesNotTreatDefinitionAsMechanism(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 2, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	)
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1}},
		{Kind: types.EvidenceConditional, AnchorKind: types.AnchorCondition, GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{2}},
	}
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence); !strings.Contains(got, "1 (function_or_purpose)") {
		t.Fatalf("identity-only definition must not close a mechanism dimension: %q", got)
	}
	evidence[0].Kind = types.EvidenceMechanism
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence); !strings.Contains(got, "1 (function_or_purpose)") {
		t.Fatalf("mechanism-kind definition must not close an operation dimension: %q", got)
	}
}

func TestRequestedDimensionEvidenceOwnershipLeavesSingleExplanationOnExistingFloor(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, nil); got != "" {
		t.Fatalf("single explanation must keep existing floor contract, got %q", got)
	}
}

func TestRequestedDimensionEvidenceOwnershipRequiresExactBoundFileOperation(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd/root.go", Confidence: 0.95, RequestedDimensionIndices: []int{3}},
	}
	evidence := []types.EvidenceItem{
		{Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall, Source: "cmd/root.go", GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1, 3}},
	}
	got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence)
	if !strings.Contains(got, "1 (function_or_purpose) @ config/load.go") || strings.Contains(got, "3 (function_or_purpose) @ cmd/root.go") {
		t.Fatalf("file-scoped downgrade=%q", got)
	}
	evidence = append(evidence, types.EvidenceItem{
		Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall, Source: "config/load.go", GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1},
	})
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, evidence); got != "" {
		t.Fatalf("exact file operations should close all seats: %q", got)
	}
}

func TestEmitEvidenceDimensionOwnershipAdvisoryAcceptsRowsAndNamesMissingOperationIndices(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	definition := types.EvidenceItem{
		Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1, 3},
	}
	operation := types.EvidenceItem{
		Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall,
		GroundingStatus: types.GroundingGrounded,
	}
	got := renderRequestedDimensionOperationOwnershipAdvisory(ctx,
		[]types.EvidenceItem{definition, operation}, []types.EvidenceItem{definition, operation})
	for _, want := range []string{
		"accepted evidence is unchanged",
		"1 (function_or_purpose)",
		"3 (function_or_purpose)",
		"identity/definition/context rows",
		"ownership is never inferred or copied automatically",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("advisory missing %q: %q", want, got)
		}
	}
}

func TestEmitEvidenceDimensionOwnershipAdvisoryClearsWhenOperationsOwnEveryIndex(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	)
	items := []types.EvidenceItem{
		{Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1}},
		{Kind: types.EvidenceConditional, AnchorKind: types.AnchorCondition, GroundingStatus: types.GroundingRecovered, RequestedDimensionIndices: []int{3}},
	}
	if got := renderRequestedDimensionOperationOwnershipAdvisory(ctx, items, items); got != "" {
		t.Fatalf("fully owned dimensions must not produce advisory: %q", got)
	}
}

func TestEmitEvidenceDimensionOwnershipAdvisoryNamesWrongFileSeat(t *testing.T) {
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd/root.go", Confidence: 0.95, RequestedDimensionIndices: []int{3}},
	}
	items := []types.EvidenceItem{
		{Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall, Source: "cmd/root.go", GroundingStatus: types.GroundingGrounded, RequestedDimensionIndices: []int{1, 3}},
	}
	got := renderRequestedDimensionOperationOwnershipAdvisory(ctx, items, items)
	for _, want := range []string{"1 (function_or_purpose) @ config/load.go", "sibling file", "exact file"} {
		if !strings.Contains(got, want) {
			t.Fatalf("wrong-file advisory missing %q: %q", want, got)
		}
	}
}

func TestEmitEvidenceDimensionOwnershipAdvisorySkipsUnrelatedAndSingleDimensionCalls(t *testing.T) {
	multi := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 2, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	)
	unrelated := []types.EvidenceItem{{
		Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, GroundingStatus: types.GroundingGrounded,
	}}
	if got := renderRequestedDimensionOperationOwnershipAdvisory(multi, unrelated, unrelated); got != "" {
		t.Fatalf("unrelated identity emit must not be spammed: %q", got)
	}

	single := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	operation := []types.EvidenceItem{{Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded}}
	if got := renderRequestedDimensionOperationOwnershipAdvisory(single, operation, operation); got != "" {
		t.Fatalf("single explanation keeps the existing floor: %q", got)
	}
}

func TestEmitEvidenceExecutePublishesDimensionOwnershipAdvisoryWithoutRejectingEvidence(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "x.go"), []byte("package p\nfunc F() {}\nfunc G() { F() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	ctx.RepoRoot = repo
	ctx.WorkDir = repo
	seedReadFileHistory(ctx, "x.go", 1, "package p", "func F() {}", "func G() { F() }")
	result, err := (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{
		"items":[
			{"scope":"line","evidence_kind":"direct","source":"x.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"F","requested_dimension_indices":[1,3]},
			{"scope":"line","evidence_kind":"mechanism","subject":"G","predicate":"calls","object":"F","source":"x.go","line_start":3,"anchor_kind":"call","anchor_symbol":"F"}
		]
	}`))
	if err != nil || !result.Success {
		t.Fatalf("evidence should remain accepted: err=%v result=%+v", err, result)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got < 2 {
		t.Fatalf("accepted evidence was not committed: got=%d summary=%s", got, result.Summary)
	}
	if !strings.Contains(result.Summary, "Requested-dimension operation ownership advisory") ||
		!strings.Contains(result.Summary, "accepted evidence is unchanged") {
		t.Fatalf("successful result did not carry the early soft advisory: %s", result.Summary)
	}
	if advisory, accepted := strings.Index(result.Summary, "Requested-dimension operation ownership advisory"), strings.Index(result.Summary, "emit_evidence accepted"); advisory < 0 || accepted < 0 || advisory > accepted {
		t.Fatalf("actionable advisory must precede the long evidence audit: %s", result.Summary)
	}
}
