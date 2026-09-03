package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// TestShippingAnswerPathsDoNotCallVisibleModelContentMutators is an ownership
// tripwire, not a prose-content gate. The named legacy helpers all mutate or
// remove user-visible model-authored fields. Precise facts may be projected in
// a separately marked system block, or returned as typed guidance, but these
// helpers must never be reconnected to full emit, patch emit, or the shared
// recovery normalization path.
func TestShippingAnswerPathsDoNotCallVisibleModelContentMutators(t *testing.T) {
	banned := map[string]bool{
		"normalizeDiagramDefinitionLabelsByEvidence":            true,
		"normalizeVisibleSourceLocationCarriers":                true,
		"normalizeQualifiedItemLabelsByUniqueEnclosingFunction": true,
		"materializeRequiredModelSurfaceTerms":                  true,
		"materializeRequiredModelSurfaceTermsIntoMarkdownTable": true,
		"materializeSurfaceTermsIntoItem":                       true,
		"normalizePrincipalSupportSurfaceTermSupplement":        true,
		"normalizeRuntimeObservationOnlyDecisionBlocks":         true,
		"normalizeRuntimeArtifactVisibleCitationSentinels":      true,
		"normalizeExternalObservationVisibleCitationSentinels":  true,
		"compileEnumerationDisplayTableRows":                    true,
		"normalizeEnumerationDisplayRequestedFieldSurfaces":     true,
		"normalizePrincipalEnumerationRowBlocks":                true,
		"canonicalizeSummaryLeadBlock":                          true,
		"normalizeInactiveTypedDecisionVerdictFields":           true,
		"normalizeExcessRequiredSummaryBlocks":                  true,
		"normalizeImplicitDefinitionClaimUses":                  true,
		"normalizeAutoRepairableRequiredFacetIDs":               true,
		"normalizeSuppressedExactResolutionAnswerSurface":       true,
		"normalizeAmbiguousMultiTargetAbsentExactResolution":    true,
		"normalizeAbsentExactResolutionScalarBlocks":            true,
		"normalizeObservedArtifactClaimUseCarriers":             true,
		"normalizeCitationBackedPrincipalClaimUses":             true,
		"normalizePrincipalSupportMemberCarriers":               true,
		"normalizeMergedDiagramPayloadKinds":                    true,
	}
	for _, tc := range []struct {
		file string
		fn   string
	}{
		{file: "emit_answer_document_v2.go", fn: "executeAnswerDocumentV2"},
		{file: "emit_answer_document_v2.go", fn: "normalizeAnswerDocumentForPreEmit"},
		{file: "emit_answer_document_patch.go", fn: "Execute"},
		// V2-1 (§40.17): the patch-normalizer chain and the base constructor
		// were hoisted out of Execute; the guard keeps covering the moved code.
		{file: "emit_answer_document_patch.go", fn: "normalizeAnswerDocumentPatchForBase"},
		{file: "emit_answer_document_patch.go", fn: "buildAnswerDocumentPatchBase"},
		{file: "emit_answer_document_patch.go", fn: "stageAnswerDocumentPatchGeneration"},
		{file: "answer_document_mutation_runtime.go", fn: "persistMergedAnswerDocument"},
		{file: "answer_document_mutation_runtime.go", fn: "normalizeAnswerDocumentRowsBeforePersist"},
		{file: "answer_document_pre_emit_check.go", fn: "normalizeViewCompatibleAnswerDocument"},
		{file: "answer_document_text_recovery.go", fn: "decodeRecoveredAnswerDocumentV2"},
		{file: "answer_document_text_recovery.go", fn: "visibleAnswerDocumentFromRaw"},
	} {
		assertFunctionDoesNotCall(t, tc.file, tc.fn, banned)
	}
}

func assertFunctionDoesNotCall(t *testing.T, file, fn string, banned map[string]bool) {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	found := false
	for _, decl := range parsed.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn || fd.Body == nil {
			continue
		}
		found = true
		ast.Inspect(fd.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if ok && banned[ident.Name] {
				t.Errorf("%s:%s reconnects visible model-content mutator %s", file, fn, ident.Name)
			}
			return true
		})
	}
	if !found {
		t.Fatalf("function %s not found in %s", fn, file)
	}
}
