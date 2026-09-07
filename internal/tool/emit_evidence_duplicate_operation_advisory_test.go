package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const duplicateOperationAdvisoryHeader = "Requested-dimension operation ownership advisory"

func newDuplicateOperationAdvisoryContext(t *testing.T) *types.BusContext {
	t.Helper()
	ctx := dimensionOwnershipContext(
		types.RequestedAnswerDimension{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
	)
	ctx.RepoRoot = t.TempDir()
	ctx.WorkDir = ctx.RepoRoot
	var reads []types.ToolResult
	for path, lines := range map[string][]string{
		"x.go": {"package p", "func Consume() {}", "func Produce() {", " Consume()", "}", "func Idle() {}", "func Flush() {", " Consume()", "}"},
		"y.go": {"package p", "func Observe() {}", "func Check() {", " Observe()", "}"},
	} {
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, path), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		seedReadFileHistory(ctx, path, 1, lines...)
		reads = append(reads, ctx.Mutable.TurnAArtifacts().ToolResults...)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: reads})
	return ctx
}

func emitDuplicateOperationAdvisoryItems(t *testing.T, ctx *types.BusContext, items ...map[string]any) *types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	inputBefore := string(params)
	requestBefore, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	result, err := (&EmitEvidence{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("evidence must remain accepted: err=%v result=%+v", err, result)
	}
	requestAfter, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	if string(params) != inputBefore || string(requestBefore) != string(requestAfter) {
		t.Fatal("soft feedback must not mutate tool input or requested dimension ownership")
	}
	return &result
}

func duplicateOperationDefinition(symbol string, line int, indices ...int) map[string]any {
	return map[string]any{
		"scope": "line", "evidence_kind": "mechanism", "source": "x.go", "line_start": line,
		"anchor_kind": "definition", "anchor_symbol": symbol, "requested_dimension_indices": indices,
	}
}

func duplicateOperationCall(source, owner, callee string, line int, indices ...int) map[string]any {
	return map[string]any{
		"scope": "line", "evidence_kind": "mechanism", "source": source, "line_start": line,
		"anchor_kind": "call", "anchor_symbol": callee, "subject": owner, "predicate": "calls", "object": callee,
		"requested_dimension_indices": indices,
	}
}

func requireDuplicateOperationAdvisory(t *testing.T, result *types.ToolResult, wants ...string) {
	t.Helper()
	if !strings.HasPrefix(result.Summary, duplicateOperationAdvisoryHeader) {
		t.Fatalf("current-call ownership feedback must precede the duplicate audit: %s", result.Summary)
	}
	for _, want := range wants {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("ownership feedback missing %q: %s", want, result.Summary)
		}
	}
}

func TestEmitEvidenceDuplicateRetainsCurrentOperationOwnershipAdvisory(t *testing.T) {
	for _, omitIndices := range []bool{false, true} {
		name := "identical_metadata"
		if omitIndices {
			name = "incoming_omits_existing_indices"
		}
		t.Run(name, func(t *testing.T) {
			ctx := newDuplicateOperationAdvisoryContext(t)
			definition := duplicateOperationDefinition("Consume", 2, 1, 3)
			first := emitDuplicateOperationAdvisoryItems(t, ctx, definition)
			requireDuplicateOperationAdvisory(t, first, "identity/definition/context rows")
			before, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
			gateBefore := requestedDimensionEvidenceOwnershipDowngrade(ctx, ctx.Mutable.EmittedEvidence())
			if omitIndices {
				definition = duplicateOperationDefinition("Consume", 2)
			}
			second := emitDuplicateOperationAdvisoryItems(t, ctx, definition)
			if second.Repair == nil || second.Repair.Code != EmitEvidenceDuplicateNoopCode {
				t.Fatalf("the real duplicate/no-progress path must be exercised: %+v", second)
			}
			requireDuplicateOperationAdvisory(t, second, "1 (function_or_purpose)", "3 (function_or_purpose)", "identity/definition/context rows")
			after, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
			if string(before) != string(after) || gateBefore != requestedDimensionEvidenceOwnershipDowngrade(ctx, ctx.Mutable.EmittedEvidence()) {
				t.Fatal("duplicate feedback must not change accepted evidence, indices, or completion qualification")
			}
		})
	}
}

func TestEmitEvidenceDuplicateAdvisoryIsScopedToCurrentRows(t *testing.T) {
	ctx := newDuplicateOperationAdvisoryContext(t)
	definition := duplicateOperationDefinition("Consume", 2, 1, 3)
	emitDuplicateOperationAdvisoryItems(t, ctx, definition)
	unrelated := duplicateOperationDefinition("Idle", 6)
	mixed := emitDuplicateOperationAdvisoryItems(t, ctx, definition, unrelated)
	requireDuplicateOperationAdvisory(t, mixed, "identity/definition/context rows")
	before, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
	result := emitDuplicateOperationAdvisoryItems(t, ctx, unrelated)
	if result.Repair == nil || result.Repair.Code != EmitEvidenceDuplicateNoopCode {
		t.Fatalf("expected the unrelated row's duplicate branch: %+v", result)
	}
	if strings.Contains(result.Summary, duplicateOperationAdvisoryHeader) {
		t.Fatalf("an older indexed definition must not become a session-wide trigger: %s", result.Summary)
	}
	after, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
	if string(before) != string(after) {
		t.Fatal("unrelated duplicate must not mutate evidence")
	}
}

func TestEmitEvidenceDuplicateAdvisoryClearsAfterExactOperationAmendment(t *testing.T) {
	ctx := newDuplicateOperationAdvisoryContext(t)
	definition := duplicateOperationDefinition("Consume", 2, 1, 3)
	emitDuplicateOperationAdvisoryItems(t, ctx, definition)
	call := duplicateOperationCall("x.go", "Produce", "Consume", 4)
	initial := emitDuplicateOperationAdvisoryItems(t, ctx, call)
	requireDuplicateOperationAdvisory(t, initial, "1 (function_or_purpose)", "3 (function_or_purpose)")
	duplicate := emitDuplicateOperationAdvisoryItems(t, ctx, call)
	requireDuplicateOperationAdvisory(t, duplicate, "1 (function_or_purpose)")

	call = duplicateOperationCall("x.go", "Produce", "Consume", 4, 1)
	amended := emitDuplicateOperationAdvisoryItems(t, ctx, call)
	if !strings.Contains(amended.Summary, "Updated 1 existing evidence item") {
		t.Fatalf("ordinary model-selected index amendment must remain available: %s", amended.Summary)
	}
	requireDuplicateOperationAdvisory(t, amended, "3 (function_or_purpose)")
	if strings.Contains(amended.Summary, "missing operation-owned indices 1 (") {
		t.Fatalf("amended operation must satisfy its own dimension: %s", amended.Summary)
	}
	closed := emitDuplicateOperationAdvisoryItems(t, ctx, duplicateOperationCall("x.go", "Flush", "Consume", 8, 3))
	for name, result := range map[string]*types.ToolResult{
		"new operation closes":  closed,
		"old definition replay": emitDuplicateOperationAdvisoryItems(t, ctx, definition),
		"old operation replay":  emitDuplicateOperationAdvisoryItems(t, ctx, call),
	} {
		if strings.Contains(result.Summary, duplicateOperationAdvisoryHeader) {
			t.Errorf("%s: covered ownership must clear the advisory: %s", name, result.Summary)
		}
	}
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, ctx.Mutable.EmittedEvidence()); got != "" {
		t.Fatalf("the unchanged completion predicate must agree that operations now cover both seats: %s", got)
	}
	for _, item := range ctx.Mutable.EmittedEvidence() {
		switch {
		case item.AnchorKind == types.AnchorDefinition && item.AnchorSymbol == "Consume":
			if !reflect.DeepEqual(item.RequestedDimensionIndices, []int{1, 3}) {
				t.Fatalf("definition metadata must not be moved or erased: %+v", item)
			}
		case item.AnchorKind == types.AnchorCall && item.LineStart == 4:
			if !reflect.DeepEqual(item.RequestedDimensionIndices, []int{1}) {
				t.Fatalf("only the model-selected index belongs to the first operation: %+v", item)
			}
		case item.AnchorKind == types.AnchorCall && item.LineStart == 8:
			if !reflect.DeepEqual(item.RequestedDimensionIndices, []int{3}) {
				t.Fatalf("only the model-selected index belongs to the second operation: %+v", item)
			}
		}
	}
}

func TestEmitEvidenceDuplicateAdvisoryPreservesExactFileOwnership(t *testing.T) {
	ctx := newDuplicateOperationAdvisoryContext(t)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "x.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "y.go", Confidence: 0.95, RequestedDimensionIndices: []int{3}},
	}
	wrongFile := duplicateOperationCall("y.go", "Check", "Observe", 4, 1, 3)
	emitDuplicateOperationAdvisoryItems(t, ctx, wrongFile)
	result := emitDuplicateOperationAdvisoryItems(t, ctx, wrongFile)
	requireDuplicateOperationAdvisory(t, result, "1 (function_or_purpose) @ x.go", "sibling file", "exact file")
	if got := requestedDimensionEvidenceOwnershipDowngrade(ctx, ctx.Mutable.EmittedEvidence()); !strings.Contains(got, "1 (function_or_purpose) @ x.go") || strings.Contains(got, "3 (function_or_purpose)") {
		t.Fatalf("soft feedback must match the existing hard predicate: %s", got)
	}
	closed := emitDuplicateOperationAdvisoryItems(t, ctx, duplicateOperationCall("x.go", "Produce", "Consume", 4, 1))
	if strings.Contains(closed.Summary, duplicateOperationAdvisoryHeader) || requestedDimensionEvidenceOwnershipDowngrade(ctx, ctx.Mutable.EmittedEvidence()) != "" {
		t.Fatalf("exact-source operation must close both feedback and completion need: %s", closed.Summary)
	}
}
