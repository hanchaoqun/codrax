// Package tool — emit_analysis_irrelevant_files_test.go (2026-05-10).
//
// L4 of the forced-read remediation: validateAndBuildIrrelevantFiles
// canonicalises and caps the LLM-emitted irrelevant_files array.
package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestValidateAndBuildIrrelevantFiles_NilInput_NilOutput(t *testing.T) {
	if got := validateAndBuildIrrelevantFiles(nil, nil); got != nil {
		t.Errorf("nil: expected nil; got %v", got)
	}
}

func TestValidateAndBuildIrrelevantFiles_HappyPath(t *testing.T) {
	in := []string{"a.go", "b.go", "c.go"}
	got := validateAndBuildIrrelevantFiles(in, nil)
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}

func TestValidateAndBuildIrrelevantFiles_DropsEmpty(t *testing.T) {
	val := &analysisValidationResult{}
	in := []string{"", "  ", "a.go"}
	got := validateAndBuildIrrelevantFiles(in, val)
	if len(got) != 1 || got[0] != "a.go" {
		t.Errorf("empty/whitespace should drop; got %v", got)
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected dropped-warning; got %v", val.Warnings)
	}
}

func TestValidateAndBuildIrrelevantFiles_DropsDuplicates(t *testing.T) {
	val := &analysisValidationResult{}
	in := []string{"a.go", "./a.go", `a.go`, "b.go"}
	got := validateAndBuildIrrelevantFiles(in, val)
	// "a.go" / "./a.go" canonicalise to the same string → dedup.
	if len(got) != 2 {
		t.Errorf("dedup should leave 2 entries; got %v", got)
	}
}

func TestValidateAndBuildIrrelevantFiles_HardCapEnforced(t *testing.T) {
	val := &analysisValidationResult{}
	in := make([]string, irrelevantFilesMax+5)
	for i := range in {
		in[i] = "f" + string(rune('a'+i)) + ".go"
	}
	got := validateAndBuildIrrelevantFiles(in, val)
	if len(got) != irrelevantFilesMax {
		t.Errorf("hard cap = %d, got %d", irrelevantFilesMax, len(got))
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected dropped-warning for over-cap; got %v", val.Warnings)
	}
}

func TestValidateAndBuildIrrelevantFiles_CanonicalisesPaths(t *testing.T) {
	in := []string{`internal\foo.go`, "./internal/bar.go"}
	got := validateAndBuildIrrelevantFiles(in, nil)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0] != "internal/foo.go" {
		t.Errorf("backslash → slash; got %q", got[0])
	}
	if got[1] != "internal/bar.go" {
		t.Errorf("./prefix stripped; got %q", got[1])
	}
}

func TestEmitAnalysisExecute_RepairsIrrelevantFilesObjectEntries(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("explain the analyzer")
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["analyzer", "irrelevant", "files"],
		"entities": ["analyzer"],
		"question_kind": "mechanism",
		"irrelevant_files": [
			{"path": "./internal/noise.go", "confidence": 0.2, "rationale": "off-topic"},
			{"file": "internal/no-path.go", "confidence": 0.1},
			"internal/other.go"
		]
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("object-shaped irrelevant_files should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	got := rm.AnalyzerHints.IrrelevantFiles
	if len(got) != 2 || got[0] != "internal/noise.go" || got[1] != "internal/other.go" {
		t.Fatalf("irrelevant files = %+v, want repaired paths", got)
	}
	if !strings.Contains(res.Summary, "repaired 1 object entries") {
		t.Fatalf("summary should disclose object-entry repair, got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped 1 malformed optional entries") {
		t.Fatalf("summary should disclose dropped malformed entry, got %q", res.Summary)
	}
}

func TestReconcilePrincipalScopeIrrelevantFiles_PromotesAllScopeSourceInventoryPaths(t *testing.T) {
	root := t.TempDir()
	relA := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	relB := "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets"
	mustWriteTestFile(t, root, relA, "@Entry\n@Component\n")
	mustWriteTestFile(t, root, relB, "@Builder\n")

	val := &analysisValidationResult{}
	required, irrelevant, scope := reconcilePrincipalScopeIrrelevantFiles(
		&types.BusContext{RepoRoot: root},
		&types.SourceScopeProfile{RequestedScope: types.SourceScopeAll},
		&types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		},
		types.IntentEnumerate,
		types.SemanticPredicates{IsCategoryEnumeration: true},
		nil,
		[]string{relA, relB},
		val,
	)
	if len(irrelevant) != 0 {
		t.Fatalf("principal source-scope paths should be removed from irrelevant_files, got %+v", irrelevant)
	}
	if len(required) != 2 {
		t.Fatalf("principal source-scope paths should be promoted to required_files, got %+v", required)
	}
	if required[0].Path != relA || required[1].Path != relB {
		t.Fatalf("promoted required files = %+v", required)
	}
	if required[0].Confidence < 0.79 || required[1].Confidence < 0.79 {
		t.Fatalf("promoted required files should carry useful confidence, got %+v", required)
	}
	if scope == nil || scope.RequestedScope != types.SourceScopeAll {
		t.Fatalf("explicit all scope should be preserved, got %+v", scope)
	}
	if !containsAny(val.Warnings, "principal source-scope path") {
		t.Fatalf("expected typed contradiction warning, got %+v", val.Warnings)
	}
}

func TestReconcilePrincipalScopeIrrelevantFiles_SynthesizesAllScopeForAuxiliaryInventoryPaths(t *testing.T) {
	root := t.TempDir()
	rel := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	mustWriteTestFile(t, root, rel, "@Entry\n@Component\n")

	val := &analysisValidationResult{}
	required, irrelevant, scope := reconcilePrincipalScopeIrrelevantFiles(
		&types.BusContext{RepoRoot: root},
		nil,
		nil,
		types.IntentEnumerate,
		types.SemanticPredicates{IsCategoryEnumeration: true},
		nil,
		[]string{rel},
		val,
	)
	if len(irrelevant) != 0 {
		t.Fatalf("source-inventory shape without explicit production scope must not hide auxiliary source paths, got %+v", irrelevant)
	}
	if len(required) != 1 || required[0].Path != rel {
		t.Fatalf("auxiliary source path should be promoted to required_files, got %+v", required)
	}
	if scope == nil || scope.RequestedScope != types.SourceScopeAll || !scope.IncludeAuxiliaryAsPrincipal {
		t.Fatalf("auxiliary preservation should synthesize all-scope, got %+v", scope)
	}
	if !containsAny(val.Warnings, "synthesized all-scope") {
		t.Fatalf("expected all-scope synthesis warning, got %+v", val.Warnings)
	}
}

func TestReconcilePrincipalScopeIrrelevantFiles_KeepsQuotedProductionScopeAuxiliaryIrrelevant(t *testing.T) {
	root := t.TempDir()
	rel := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	mustWriteTestFile(t, root, rel, "@Entry\n@Component\n")

	val := &analysisValidationResult{}
	required, irrelevant, scope := reconcilePrincipalScopeIrrelevantFiles(
		&types.BusContext{RepoRoot: root},
		&types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction, SourceQuotes: []string{"production"}},
		&types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		},
		types.IntentEnumerate,
		types.SemanticPredicates{IsCategoryEnumeration: true},
		nil,
		[]string{rel},
		val,
	)
	if len(required) != 0 {
		t.Fatalf("production scope should not promote auxiliary path, got %+v", required)
	}
	if len(irrelevant) != 1 || irrelevant[0] != rel {
		t.Fatalf("production scope should keep auxiliary irrelevant file, got %+v", irrelevant)
	}
	if scope == nil || scope.RequestedScope != types.SourceScopeProduction {
		t.Fatalf("explicit production scope should be preserved, got %+v", scope)
	}
	if containsAny(val.Warnings, "principal source-scope path") {
		t.Fatalf("should not warn when source scope does not allow the path, got %+v", val.Warnings)
	}
}

func TestReconcilePrincipalScopeIrrelevantFiles_RepairsUnquotedProductionScopeAuxiliaryPaths(t *testing.T) {
	root := t.TempDir()
	rel := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	mustWriteTestFile(t, root, rel, "@Entry\n@Component\n")

	val := &analysisValidationResult{}
	required, irrelevant, scope := reconcilePrincipalScopeIrrelevantFiles(
		&types.BusContext{RepoRoot: root},
		&types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction, Confidence: 0.95},
		&types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		},
		types.IntentEnumerate,
		types.SemanticPredicates{IsCategoryEnumeration: true},
		nil,
		[]string{rel},
		val,
	)
	if len(irrelevant) != 0 {
		t.Fatalf("unquoted production scope should not hide auxiliary source path in source-inventory shape, got %+v", irrelevant)
	}
	if len(required) != 1 || required[0].Path != rel {
		t.Fatalf("auxiliary source path should be promoted, got %+v", required)
	}
	if scope == nil || scope.RequestedScope != types.SourceScopeAll || !scope.IncludeAuxiliaryAsPrincipal {
		t.Fatalf("unquoted production should synthesize all-scope after auxiliary preservation, got %+v", scope)
	}
}

func TestEmitAnalysisExecute_RepairsPrincipalSourceScopeIrrelevantFiles(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	root := t.TempDir()
	relA := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	relB := "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets"
	mustWriteTestFile(t, root, relA, "@Entry\n@Component\n")
	mustWriteTestFile(t, root, relB, "@Builder\n")

	mu := types.NewMutableState("仓库里有哪些 @Entry 和 @Builder 装饰器源码")
	payload := withV4Required(`{
		"intent": "enumerate",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["@Entry", "@Builder", "ArkTS"],
		"entities": ["@Entry", "@Builder"],
		"question_kind": "enumeration",
		"source_scope_profile": {
			"requested_scope": "all",
			"include_auxiliary_as_principal": false,
			"confidence": 0.95,
			"rationale": "all repo source classes are principal for this inventory"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function", "method"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["@Entry", "@Builder"],
			"confidence": 0.90,
			"rationale": "decorator inventory"
		},
		"irrelevant_files": [
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets"
		]
	}`)
	payload = strings.Replace(payload, `"is_category_enumeration": false`, `"is_category_enumeration": true`, 1)

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, RepoRoot: root}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("principal source-scope irrelevant_files should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if len(rm.AnalyzerHints.IrrelevantFiles) != 0 {
		t.Fatalf("principal source-scope files should not remain irrelevant, got %+v", rm.AnalyzerHints.IrrelevantFiles)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 2 {
		t.Fatalf("principal source-scope files should be promoted to required hints, got %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	if rm.AnalyzerHints.RequiredFileHints[0].Path != relA || rm.AnalyzerHints.RequiredFileHints[1].Path != relB {
		t.Fatalf("promoted required hints = %+v", rm.AnalyzerHints.RequiredFileHints)
	}
	if rm.SourceScopeProfile == nil || rm.SourceScopeProfile.RequestedScope != types.SourceScopeAll {
		t.Fatalf("source scope should stay all for principal auxiliary source paths, got %+v", rm.SourceScopeProfile)
	}
	if !strings.Contains(res.Summary, "principal source-scope path") {
		t.Fatalf("summary should disclose typed source-scope repair, got %q", res.Summary)
	}
}

func TestEmitAnalysisExecute_SynthesizesAllScopeForSourceInventoryIrrelevantAuxiliaryPaths(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	root := t.TempDir()
	rel := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	mustWriteTestFile(t, root, rel, "@Entry\n@Component\n")

	mu := types.NewMutableState("仓库里有哪些 @Entry 标记的 ArkTS 页面入口")
	payload := withV4Required(`{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["@Entry", "ArkTS"],
		"entities": ["@Entry"],
		"question_kind": "enumeration",
		"irrelevant_files": [
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
		]
	}`)
	payload = strings.Replace(payload, `"is_category_enumeration": false`, `"is_category_enumeration": true`, 1)

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, RepoRoot: root}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("source-inventory auxiliary scope repair should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if len(rm.AnalyzerHints.IrrelevantFiles) != 0 || len(rm.AnalyzerHints.RequiredFileHints) != 1 {
		t.Fatalf("expected irrelevant→required repair, irrelevant=%+v required=%+v", rm.AnalyzerHints.IrrelevantFiles, rm.AnalyzerHints.RequiredFileHints)
	}
	if rm.SourceScopeProfile == nil || rm.SourceScopeProfile.RequestedScope != types.SourceScopeAll || !rm.SourceScopeProfile.IncludeAuxiliaryAsPrincipal {
		t.Fatalf("expected synthesized all-scope profile, got %+v", rm.SourceScopeProfile)
	}
	if !strings.Contains(res.Summary, "synthesized all-scope") {
		t.Fatalf("summary should disclose all-scope synthesis, got %q", res.Summary)
	}
}

func TestEmitAnalysisExecute_OverridesUnquotedProductionScopeForSourceInventoryAuxiliaryPaths(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	root := t.TempDir()
	rel := "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
	mustWriteTestFile(t, root, rel, "@Entry\n@Component\n")

	mu := types.NewMutableState("仓库里有哪些 @Entry 标记的 ArkTS 页面入口")
	payload := withV4Required(`{
		"intent": "enumerate",
		"scenario": "generic",
		"complexity": "moderate",
		"keywords": ["@Entry", "ArkTS"],
		"entities": ["@Entry"],
		"question_kind": "enumeration",
		"source_scope_profile": {
			"requested_scope": "production",
			"confidence": 0.95,
			"rationale": "model inferred production from repository layout"
		},
		"source_inventory_profile": {
			"is_source_inventory": true,
			"target_roles": ["function"],
			"requested_fields": ["name", "location"],
			"source_quotes": ["@Entry"],
			"confidence": 0.90
		},
		"irrelevant_files": [
			"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"
		]
	}`)
	payload = strings.Replace(payload, `"is_category_enumeration": false`, `"is_category_enumeration": true`, 1)

	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu, RepoRoot: root}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("unquoted production source scope repair should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	if len(rm.AnalyzerHints.IrrelevantFiles) != 0 || len(rm.AnalyzerHints.RequiredFileHints) != 1 {
		t.Fatalf("expected irrelevant→required repair, irrelevant=%+v required=%+v", rm.AnalyzerHints.IrrelevantFiles, rm.AnalyzerHints.RequiredFileHints)
	}
	if rm.SourceScopeProfile == nil || rm.SourceScopeProfile.RequestedScope != types.SourceScopeAll || !rm.SourceScopeProfile.IncludeAuxiliaryAsPrincipal {
		t.Fatalf("expected unquoted production to be overridden by all-scope, got %+v", rm.SourceScopeProfile)
	}
}

func mustWriteTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// === schema smoke ===

func TestEmitAnalysisSchema_IrrelevantFilesPresent(t *testing.T) {
	emitAnalysisSchemaOnce.Do(buildEmitAnalysisSchema)
	if !strings.Contains(string(emitAnalysisSchemaCache), `"irrelevant_files"`) {
		t.Error("emit_analysis schema should declare irrelevant_files property")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `OFF-TOPIC`) {
		t.Error("irrelevant_files description should mention OFF-TOPIC")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `plain strings only`) {
		t.Error("irrelevant_files description should explicitly require string entries")
	}
}
