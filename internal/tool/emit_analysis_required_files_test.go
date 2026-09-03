// Package tool — emit_analysis_required_files_test.go (2026-05-10).
//
// L3-T2 of the forced-read remediation: validateAndBuildRequiredFileHints
// is the per-entry validator + canonicaliser for the LLM-emitted
// `required_files` array. Pin the exact behaviour the IR consumer
// (explorer) relies on.
package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type requiredFileHintResolveTestGater struct {
	aliases map[string]string
}

func (g requiredFileHintResolveTestGater) ResolveActiveSetPath(ctx *types.BusContext, _, llmPath string, fileExists func(string) bool) types.ActiveSetGateResult {
	if dst, ok := g.aliases[llmPath]; ok {
		if fileExists != nil && (ctx == nil || !fileExists(filepath.Join(ctx.RepoRoot, filepath.FromSlash(dst)))) {
			return types.ActiveSetGateResult{Allowed: false, RefusalProse: "not found"}
		}
		return types.ActiveSetGateResult{Allowed: true, ResolvedPath: dst, AutoPrefixed: true}
	}
	return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
}

func (g requiredFileHintResolveTestGater) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true}
}

func TestValidateAndBuildRequiredFileHints_NilInput_NilOutput(t *testing.T) {
	got := validateAndBuildRequiredFileHints(nil, nil)
	if got != nil {
		t.Errorf("nil input should return nil; got %v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_EmptyInput_NilOutput(t *testing.T) {
	got := validateAndBuildRequiredFileHints([]emitRequiredFileParam{}, nil)
	if got != nil {
		t.Errorf("empty input should return nil; got %v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_HappyPath(t *testing.T) {
	in := []emitRequiredFileParam{
		{Path: "internal/foo.go", Confidence: 0.9, Rationale: "primary"},
		{Path: "internal/bar.go", Confidence: 0.6, Rationale: "secondary"},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 2 {
		t.Fatalf("got %d hints, want 2", len(got))
	}
	if got[0].Path != "internal/foo.go" || got[0].Confidence != 0.9 || got[0].Rationale != "primary" {
		t.Errorf("hint[0] = %+v", got[0])
	}
}

func TestNormalizeRequiredFileDimensionOwnershipKeepsOnlyHighConfidenceExplanationSeats(t *testing.T) {
	profile := &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
		{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: types.RequestedAnswerDimensionObservedValue, Required: true},
		{Index: 3, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
	val := &analysisValidationResult{}
	got := normalizeRequiredFileDimensionOwnership([]types.RequiredFileHint{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1, 2}},
		{Path: "cmd/root.go", Confidence: 0.7, RequestedDimensionIndices: []int{3}},
	}, profile, val)
	if len(got[0].RequestedDimensionIndices) != 1 || got[0].RequestedDimensionIndices[0] != 1 {
		t.Fatalf("high-confidence ownership=%+v", got[0].RequestedDimensionIndices)
	}
	if len(got[1].RequestedDimensionIndices) != 0 {
		t.Fatalf("low-confidence ownership must be dropped: %+v", got[1].RequestedDimensionIndices)
	}
	if len(val.Warnings) != 1 || !strings.Contains(val.Warnings[0], "dropped 2 requested_dimension_indices") {
		t.Fatalf("warnings=%+v", val.Warnings)
	}
}

// EVOLUTION RECORD (V4-4, colleague_merge_audit §40.22): this pin used to
// assert that an unclassified high-confidence file and an owner-less
// dimension REJECT the whole emission (the B1405 arms (a)/(c)). Both judged
// the completeness of content the model never declared, at a stage that does
// not read file bodies; the only taught escape was "lower confidence below
// 0.8". They are retired: the same fixture is now ACCEPTED and compiles into
// the typed soft marker that exploration resolves.
func TestValidateRequiredFileDimension_UnresolvedOwnerAccepted(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"config/load.go", "cmd/root.go"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	profile := &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
		{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: types.RequestedAnswerDimensionObservedValue, Required: true},
		{Index: 3, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
	raw := []emitRequiredFileParam{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd/root.go", Confidence: 0.95},
	}
	hints := normalizeRequiredFileDimensionOwnership(validateAndBuildRequiredFileHintsWithContext(
		&types.BusContext{RepoRoot: root}, raw, nil,
	), profile, nil)
	if got := validateRequiredFileDimensionContradictions(
		&types.BusContext{RepoRoot: root}, raw, profile, map[int]bool{1: true, 2: true, 3: true},
	); got != "" {
		t.Fatalf("an owner the model could not name is not a contradiction: %s", got)
	}
	got := types.CompileDimensionOwnerUnresolved(profile, hints)
	want := &types.DimensionOwnerUnresolved{DimensionIndices: []int{3}, UnclassifiedFiles: []string{"cmd/root.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unresolved-owner marker=%+v want %+v", got, want)
	}
}

func TestValidateRequiredFileDimensionContradictions_RejectsIndexOutsideDeclaredSet(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config/load.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
		{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
	raw := []emitRequiredFileParam{{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1, 3}}}
	got := validateRequiredFileDimensionContradictions(&types.BusContext{RepoRoot: root}, raw, profile, map[int]bool{1: true, 2: true})
	if !strings.Contains(got, "reference index [3] outside the declared requested_answer_dimensions index set [1 2]") {
		t.Fatalf("undeclared index must be a precise contradiction: %q", got)
	}
	// The index arm judges the model's DECLARED set, not the normalized
	// profile: a dimension the normalizer dropped as unanchored is still one
	// the model declared, so binding to it is a soft drop, never a reject.
	if got := validateRequiredFileDimensionContradictions(&types.BusContext{RepoRoot: root}, raw, profile, map[int]bool{1: true, 2: true, 3: true}); got != "" {
		t.Fatalf("declared-but-dropped dimension must not become a model contradiction: %q", got)
	}
	// No declared set at all → nothing to contradict; the soft drop owns it.
	if got := validateRequiredFileDimensionContradictions(&types.BusContext{RepoRoot: root}, raw, profile, nil); got != "" {
		t.Fatalf("absent dimension set must stay soft: %q", got)
	}
	// Low-confidence entries carry inert indices (soft-dropped); not judged.
	low := []emitRequiredFileParam{{Path: "config/load.go", Confidence: 0.6, RequestedDimensionIndices: []int{9}}}
	if got := validateRequiredFileDimensionContradictions(&types.BusContext{RepoRoot: root}, low, profile, map[int]bool{1: true, 2: true}); got != "" {
		t.Fatalf("low-confidence indices are inert and must stay soft: %q", got)
	}
}

func TestDeclaredRequestedDimensionIndicesUsesPositionalDefault(t *testing.T) {
	yes := true
	no := false
	got := declaredRequestedDimensionIndices(&emitRequestedAnswerDimensionsParam{IsDimensionedAnswer: &yes, Dimensions: []emitRequestedAnswerDimensionParam{
		{Label: "a"}, {Label: "b", Index: 5}, {Label: "c"},
	}})
	if !reflect.DeepEqual(got, map[int]bool{1: true, 5: true, 3: true}) {
		t.Fatalf("declared set=%v", got)
	}
	if declaredRequestedDimensionIndices(nil) != nil || declaredRequestedDimensionIndices(&emitRequestedAnswerDimensionsParam{IsDimensionedAnswer: &no, Dimensions: []emitRequestedAnswerDimensionParam{{Label: "a"}}}) != nil {
		t.Fatal("no declared dimension set must yield nil")
	}
}

func TestValidateRequiredFileDimensionResponsibilityDeclarationsAcceptsOwnerAndNavigationOnly(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"config/load.go", "docs/navigation.go"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	navigationOnly := true
	profile := &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
		{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
	raw := []emitRequiredFileParam{
		{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1, 2}},
		{Path: "docs/navigation.go", Confidence: 0.9, RequestedDimensionNavigationOnly: &navigationOnly},
	}
	hints := normalizeRequiredFileDimensionOwnership(validateAndBuildRequiredFileHintsWithContext(
		&types.BusContext{RepoRoot: root}, raw, nil,
	), profile, nil)
	if got := validateRequiredFileDimensionContradictions(
		&types.BusContext{RepoRoot: root}, raw, profile, map[int]bool{1: true, 2: true},
	); got != "" {
		t.Fatalf("complete typed responsibility declaration rejected: %s", got)
	}
	if !hints[1].RequestedDimensionNavigationOnly || hints[0].RequestedDimensionNavigationOnly {
		t.Fatalf("navigation-only classification must persist on the typed hint: %+v", hints)
	}
	if marker := types.CompileDimensionOwnerUnresolved(profile, hints); marker != nil {
		t.Fatalf("fully declared ownership must not raise the unresolved marker: %+v", marker)
	}
}

func TestValidateRequiredFileDimensionResponsibilityDeclarationsRejectsOwnerNavigationConflictOnce(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config/load.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	navigationOnly := true
	profile := &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{
		{Index: 1, Role: types.RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: types.RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
	raw := []emitRequiredFileParam{{
		Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1, 2},
		RequestedDimensionNavigationOnly: &navigationOnly,
	}}
	got := validateRequiredFileDimensionContradictions(
		&types.BusContext{RepoRoot: root}, raw, profile, map[int]bool{1: true, 2: true},
	)
	if !strings.Contains(got, "files declared both operation-owner and navigation-only=[config/load.go]") {
		t.Fatalf("owner/navigation conflict not reported precisely: %s", got)
	}
	for _, retired := range []string{"unclassified", "without a high-confidence file owner", "lower confidence"} {
		if strings.Contains(got, retired) {
			t.Fatalf("retired completeness wording %q must not return: %s", retired, got)
		}
	}
}

func TestEmitAnalysisExecutePersistsRequiredFileDimensionOwnership(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"config/load.go", "cmd/root.go"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mu := types.NewMutableState("解释解析机制、默认值和 CLI 覆盖方式")
	payload := withV4Required(`{
		"intent":"explain",
		"scenario":"config_trace",
		"complexity":"moderate",
		"keywords":["config","parse","default","override"],
		"entities":["Config"],
		"question_kind":"config_mapping",
		"predicate_axis":"configure",
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.95,
			"dimensions":[
				{"index":1,"label":"解析机制","role":"function_or_purpose","required":true,"source_quote":"解析机制"},
				{"index":2,"label":"默认值","role":"observed_value","required":true,"source_quote":"默认值"},
				{"index":3,"label":"CLI 覆盖方式","role":"function_or_purpose","required":true,"source_quote":"CLI 覆盖方式"}
			]
		},
		"required_files":[
			{"path":"config/load.go","confidence":0.95,"requested_dimension_indices":[1]},
			{"path":"cmd/root.go","confidence":0.95,"requested_dimension_indices":[3]}
		]
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{RepoRoot: root, Mutable: mu}, json.RawMessage(payload))
	if err != nil || !res.Success {
		t.Fatalf("emit_analysis: err=%v result=%+v", err, res)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.AnalyzerHints.RequiredFileHints) != 2 {
		t.Fatalf("request model hints=%+v", rm)
	}
	if got := rm.AnalyzerHints.RequiredFileHints[0].RequestedDimensionIndices; len(got) != 1 || got[0] != 1 {
		t.Fatalf("loader ownership=%+v", got)
	}
	if got := rm.AnalyzerHints.RequiredFileHints[1].RequestedDimensionIndices; len(got) != 1 || got[0] != 3 {
		t.Fatalf("CLI ownership=%+v", got)
	}
}

// twoDimensionOwnershipPayload is the shared e2e fixture: two required
// explanation-operation dimensions plus the given required_files entries.
func twoDimensionOwnershipPayload(requiredFiles string) string {
	return withV4Required(`{
		"intent":"explain",
		"scenario":"config_trace",
		"complexity":"moderate",
		"keywords":["config","parse","override"],
		"entities":["Config"],
		"question_kind":"config_mapping",
		"predicate_axis":"configure",
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.95,
			"dimensions":[
				{"index":1,"label":"解析机制","role":"function_or_purpose","required":true,"source_quote":"解析机制"},
				{"index":2,"label":"覆盖方式","role":"branch_behavior","required":true,"source_quote":"覆盖方式"}
			]
		},
		"required_files":[` + requiredFiles + `]
	}`)
}

func ownershipFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "config/load.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// EVOLUTION RECORD (V4-4, colleague_merge_audit §40.22): formerly
// TestEmitAnalysisExecuteRejectsAmbiguousHighConfidenceFileResponsibility —
// one high-confidence file with no declared role was a whole-emission hard
// reject. Analysis does not read file bodies, so this is legitimate model
// uncertainty: the emission is accepted, the typed marker is persisted and
// the summary discloses it for the exploration stage.
func TestEmitAnalysisExecute_AcceptsUnresolvedOwnerWithTypedMarker(t *testing.T) {
	root := ownershipFixtureRoot(t)
	mu := types.NewMutableState("解释解析机制和覆盖方式")
	payload := twoDimensionOwnershipPayload(`{"path":"config/load.go","confidence":0.95,"rationale":"implementation entry"}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{RepoRoot: root, Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("legitimately unknown ownership must be accepted: %+v", res)
	}
	got := mu.RequestModel().AnalyzerHints.DimensionOwnerUnresolved
	want := &types.DimensionOwnerUnresolved{DimensionIndices: []int{1, 2}, UnclassifiedFiles: []string{"config/load.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unresolved-owner marker=%+v want %+v", got, want)
	}
	for _, disclosure := range []string{
		"required_files ownership left unresolved for exploration",
		"without a declared high-confidence file owner=[1 2]",
		"without a declared role=[config/load.go]",
	} {
		if !strings.Contains(res.Summary, disclosure) {
			t.Fatalf("summary must disclose the soft marker %q:\n%s", disclosure, res.Summary)
		}
	}
	for _, retired := range []string{"unclassified high-confidence files", "lower confidence below 0.8"} {
		if strings.Contains(res.Summary, retired) {
			t.Fatalf("retired completeness reject wording %q leaked into the accepted summary:\n%s", retired, res.Summary)
		}
	}
}

func TestEmitAnalysisExecute_RejectsUndeclaredDimensionIndex(t *testing.T) {
	root := ownershipFixtureRoot(t)
	mu := types.NewMutableState("解释解析机制和覆盖方式")
	payload := twoDimensionOwnershipPayload(`{"path":"config/load.go","confidence":0.95,"requested_dimension_indices":[3]}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{RepoRoot: root, Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || mu.RequestModel() != nil {
		t.Fatalf("an index outside the declared dimension set is a hard contradiction: %+v", res)
	}
	if !strings.Contains(res.Summary, "reference index [3] outside the declared requested_answer_dimensions index set [1 2]") {
		t.Fatalf("rejection must name the undeclared index:\n%s", res.Summary)
	}

	// Counter-case: dimension 3 IS declared by the model but its quote is not
	// anchored in the request, so the profile normalizer drops it. A binding
	// to it is a system-side soft drop with a warning, never a model
	// contradiction (the hard arm judges the DECLARED set).
	mu = types.NewMutableState("解释解析机制和覆盖方式")
	dropped := strings.Replace(payload,
		`{"index":2,"label":"覆盖方式","role":"branch_behavior","required":true,"source_quote":"覆盖方式"}`,
		`{"index":2,"label":"覆盖方式","role":"branch_behavior","required":true,"source_quote":"覆盖方式"},
				{"index":3,"label":"默认值","role":"function_or_purpose","required":true,"source_quote":"默认值"}`, 1)
	if dropped == payload {
		t.Fatal("fixture did not add the unanchored dimension")
	}
	res, err = (&EmitAnalysis{}).Execute(&types.BusContext{RepoRoot: root, Mutable: mu}, json.RawMessage(dropped))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("binding to a declared-but-dropped dimension must stay soft: %+v", res)
	}
	if !strings.Contains(res.Summary, "requested_answer_dimensions ignored unanchored dimension 默认值") ||
		!strings.Contains(res.Summary, "dropped 1 requested_dimension_indices") {
		t.Fatalf("soft drop must be disclosed, not rejected:\n%s", res.Summary)
	}
}

func TestEmitAnalysisExecute_OwnerAndNavigationOnlyOnOneFileStillRejects(t *testing.T) {
	root := ownershipFixtureRoot(t)
	mu := types.NewMutableState("解释解析机制和覆盖方式")
	payload := twoDimensionOwnershipPayload(`{"path":"config/load.go","confidence":0.95,"requested_dimension_indices":[1,2],"requested_dimension_navigation_only":true}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{RepoRoot: root, Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || mu.RequestModel() != nil {
		t.Fatalf("owner ∧ navigation-only on one file is a contradiction and must not persist: %+v", res)
	}
	if !strings.Contains(res.Summary, "files declared both operation-owner and navigation-only=[config/load.go]") {
		t.Fatalf("contradiction must be named precisely:\n%s", res.Summary)
	}
}

func TestEmitAnalysisExecute_PersistsNavigationOnlyClassification(t *testing.T) {
	root := ownershipFixtureRoot(t)
	mu := types.NewMutableState("解释解析机制和覆盖方式")
	payload := twoDimensionOwnershipPayload(`{"path":"config/load.go","confidence":0.95,"requested_dimension_navigation_only":true}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{RepoRoot: root, Mutable: mu}, json.RawMessage(payload))
	if err != nil || !res.Success {
		t.Fatalf("navigation-only declaration rejected: err=%v res=%+v", err, res)
	}
	hints := mu.RequestModel().AnalyzerHints.RequiredFileHints
	if len(hints) != 1 || !hints[0].RequestedDimensionNavigationOnly {
		t.Fatalf("navigation-only classification must survive emit: %+v", hints)
	}
	got := mu.RequestModel().AnalyzerHints.DimensionOwnerUnresolved
	want := &types.DimensionOwnerUnresolved{DimensionIndices: []int{1, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("classified navigation file must not be listed as unclassified: %+v want %+v", got, want)
	}
}

func TestEmitAnalysisExecute_RepairsRequiredFilesStringEntries(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("只分析 eval/fixtures/runtime_path_panic.log 这个日志文件，不分析代码")
	payload := `{
		"intent": "root_cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"intent_confidence": 0.9,
		"complexity_confidence": 0.8,
		"kind_confidence": 0.9,
		"keywords": ["panic", "log", "runtime"],
		"entities": ["runtime_path_panic.log"],
		"question_kind": "diagnostic",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.9
		},
		"answer_role_profile": {
			"is_role_binding_requested": false,
			"confidence": 0.7
		},
		"error_granularity_profile": {
			"is_granularity_question": false,
			"confidence": 0.7
		},
		"runtime_artifact_scope_profile": {
			"requested_scope": "not_applicable",
			"confidence": 0.7
		},
		"completeness_obligation": {"required": false, "source_quote": ""},
		"required_files": ["eval/fixtures/runtime_path_panic.log"],
		"external_observation_policy": {
			"artifact_citation_mode": "external_only",
			"current_source_mode": "default",
			"confidence": 0.9
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("string-shaped required_files should be repaired, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil {
		t.Fatal("RequestModel not persisted")
	}
	got := rm.AnalyzerHints.RequiredFileHints
	if len(got) != 1 || got[0].Path != "eval/fixtures/runtime_path_panic.log" {
		t.Fatalf("required_files = %+v, want repaired log path", got)
	}
	if !strings.Contains(res.Summary, "required_files: repaired 1 string entries to object shape") {
		t.Fatalf("summary should disclose required_files repair, got %q", res.Summary)
	}
}

func TestValidateAndBuildRequiredFileHints_WindowsBackslashCanonicalised(t *testing.T) {
	in := []emitRequiredFileParam{
		{Path: `internal\foo.go`, Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 1 || got[0].Path != "internal/foo.go" {
		t.Errorf("backslash should be canonicalised to slash; got %+v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_DotSlashPrefixStripped(t *testing.T) {
	in := []emitRequiredFileParam{
		{Path: "./internal/foo.go", Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 1 || got[0].Path != "internal/foo.go" {
		t.Errorf("./prefix should be stripped; got %+v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_ActiveSetAutoPrefixesExistingFile(t *testing.T) {
	root := t.TempDir()
	rel := "CodeAgent/packages/core/src/mcp/token-storage/types.ts"
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("export type OAuthCredentials = {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: root,
		MultiGraph: requiredFileHintResolveTestGater{aliases: map[string]string{
			"packages/core/src/mcp/token-storage/types.ts": rel,
		}},
	}
	val := &analysisValidationResult{}
	got := validateAndBuildRequiredFileHintsWithContext(ctx, []emitRequiredFileParam{{
		Path:       "packages/core/src/mcp/token-storage/types.ts",
		Confidence: 0.9,
		Rationale:  "direct credential type definition",
	}}, val)
	if len(got) != 1 {
		t.Fatalf("got %d hints, want 1; warnings=%v", len(got), val.Warnings)
	}
	if got[0].Path != rel {
		t.Fatalf("hint path = %q, want %q", got[0].Path, rel)
	}
	if !containsAny(val.Warnings, "normalized") {
		t.Fatalf("expected normalization warning, got %v", val.Warnings)
	}
}

func TestValidateAndBuildRequiredFileHints_StripsRedundantRepoLabelInSingleRepo(t *testing.T) {
	parent := t.TempDir()
	repoRoot := filepath.Join(parent, "CodeAgent")
	rel := "packages/core/src/mcp/token-storage/types.ts"
	if err := os.MkdirAll(filepath.Join(repoRoot, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, rel), []byte("export type OAuthCredentials = {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repoRoot}
	got := validateAndBuildRequiredFileHintsWithContext(ctx, []emitRequiredFileParam{{
		Path:       "CodeAgent/" + rel,
		Confidence: 0.9,
	}}, nil)
	if len(got) != 1 || got[0].Path != rel {
		t.Fatalf("redundant repo label should be stripped to %q, got %+v", rel, got)
	}
}

func TestValidateAndBuildRequiredFileHints_DropsUnresolvableContextPath(t *testing.T) {
	root := t.TempDir()
	val := &analysisValidationResult{}
	got := validateAndBuildRequiredFileHintsWithContext(&types.BusContext{RepoRoot: root}, []emitRequiredFileParam{{
		Path:       "missing/file.ts",
		Confidence: 0.9,
	}}, val)
	if got != nil {
		t.Fatalf("missing context path should be dropped, got %+v", got)
	}
	if !containsAny(val.Warnings, "existing active file") {
		t.Fatalf("expected unresolved-path warning, got %v", val.Warnings)
	}
}

func TestProjectAnalyzerPrescanRequiredFileHints_SkipsLowConfidenceUnboundedInventory(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"internal/tool/builtin.go",
		"internal/skill/defaults.go",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("package sample\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mut := types.NewMutableState("source inventory")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"[grep: 2 matching files]",
			"[grep params: pattern=.ets include=* files_only=true]",
			"./internal/tool/builtin.go",
			"./internal/skill/defaults.go",
		}, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindGrep,
			Include:        "*",
			FilesOnly:      true,
			ResultCount:    2,
			CandidateFiles: []string{"./internal/tool/builtin.go", "./internal/skill/defaults.go"},
		},
	})
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.45,
		},
	}
	ctx := &types.BusContext{Mutable: mut, RepoRoot: root}

	if added := projectAnalyzerPrescanRequiredFileHints(ctx, &rm, nil); added != 0 {
		t.Fatalf("low-confidence unbounded source-inventory prescan must not project required files, added=%d hints=%+v", added, rm.AnalyzerHints.RequiredFileHints)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 0 {
		t.Fatalf("required_file hints should stay empty, got %+v", rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestProjectAnalyzerPrescanRequiredFileHints_HighConfidenceRepoWideStillNeedsBoundedScope(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"internal/tool/source_inventory_language_census.go",
		"internal/skill/defaults.go",
		"cmd/root.go",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("package sample\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mut := types.NewMutableState("repo-wide source inventory")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "[grep: 3 matching files]\n./internal/tool/source_inventory_language_census.go\n./internal/skill/defaults.go\n./cmd/root.go",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindGrep,
			FilesOnly:      true,
			ResultCount:    3,
			CandidateFiles: []string{"internal/tool/source_inventory_language_census.go", "internal/skill/defaults.go", "cmd/root.go"},
		},
	})
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType, types.AnswerCandidateRoleFunction},
			SourceQuotes:      []string{"extend 块", "foreign func 声明", "public class"},
			Confidence:        0.95,
		},
	}
	ctx := &types.BusContext{Mutable: mut, RepoRoot: root}

	if added := projectAnalyzerPrescanRequiredFileHints(ctx, &rm, nil); added != 0 {
		t.Fatalf("high-confidence repo-wide source-inventory must not project unbounded prescan support files, added=%d hints=%+v", added, rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestProjectAnalyzerPrescanRequiredFileHints_LowConfidenceUsesBoundedListFilesBeforeGrepNoise(t *testing.T) {
	root := t.TempDir()
	noiseFiles := []string{
		"internal/tool/builtin.go",
		"internal/skill/defaults.go",
		"internal/orchestrator/topology.go",
		"internal/types/comment_extract.go",
	}
	corpusFiles := []string{
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/03_state_management.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/06_entry_ability_stage_model.ets",
	}
	for _, rel := range append(append([]string{}, noiseFiles...), corpusFiles...) {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("sample\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mut := types.NewMutableState("source inventory")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join(append([]string{
			"[grep: 4 matching files]",
			"[grep params: pattern=@Entry files_only=true]",
		}, noiseFiles...), "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindGrep,
			FilesOnly:      true,
			ResultCount:    len(noiseFiles),
			CandidateFiles: noiseFiles,
		},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary: strings.Join(append([]string{
			"[list_files: path=. recursive=true include=*.ets include_auxiliary=true]",
		}, corpusFiles...), "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:             types.ToolPathDiscoveryKindListFiles,
			Path:             ".",
			Recursive:        true,
			Include:          "*.ets",
			IncludeAuxiliary: true,
			ResultCount:      len(corpusFiles),
			CandidateFiles:   corpusFiles,
		},
	})
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.45,
		},
		SourceScopeProfile: &types.SourceScopeProfile{
			RequestedScope: types.SourceScopeProduction,
			SourceQuotes:   []string{"仓库里有哪些 @Entry 标记的 ArkTS 页面入口"},
			Confidence:     0.95,
		},
	}
	ctx := &types.BusContext{Mutable: mut, RepoRoot: root}

	if added := projectAnalyzerPrescanRequiredFileHints(ctx, &rm, nil); added != len(corpusFiles) {
		t.Fatalf("bounded list_files source universe should project before grep noise, added=%d hints=%+v", added, rm.AnalyzerHints.RequiredFileHints)
	}
	for i, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Path != corpusFiles[i] {
			t.Fatalf("hint[%d]=%q, want %q (all hints=%+v)", i, hint.Path, corpusFiles[i], rm.AnalyzerHints.RequiredFileHints)
		}
	}
}

func TestProjectAnalyzerPrescanRequiredFileHints_ExplicitAuxiliaryExclusionBlocksCorpusProjection(t *testing.T) {
	root := t.TempDir()
	corpusFiles := []string{
		"fixtures/arkts/entry.ets",
		"fixtures/arkts/builder.ets",
		"fixtures/arkts/state.ets",
		"fixtures/arkts/styles.ets",
		"fixtures/arkts/list.ets",
		"fixtures/arkts/ability.ets",
	}
	for _, rel := range corpusFiles {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("sample\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mut := types.NewMutableState("production source inventory")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary: strings.Join(append([]string{
			"[list_files: path=. recursive=true include=*.ets include_auxiliary=true]",
		}, corpusFiles...), "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:             types.ToolPathDiscoveryKindListFiles,
			Path:             ".",
			Recursive:        true,
			Include:          "*.ets",
			IncludeAuxiliary: true,
			ResultCount:      len(corpusFiles),
			CandidateFiles:   corpusFiles,
		},
	})
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
		SourceScopeProfile: &types.SourceScopeProfile{
			RequestedScope: types.SourceScopeProduction,
			Confidence:     0.95,
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.95,
		},
		AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
			IsExclusionRequested: true,
			ExcludedCandidateRoles: []types.AnswerCandidateRole{
				types.AnswerCandidateRoleFixture,
				types.AnswerCandidateRoleExample,
			},
			SourceQuotes: []string{"fixtures"},
			Confidence:   0.95,
		},
	}
	ctx := &types.BusContext{Mutable: mut, RepoRoot: root}

	if added := projectAnalyzerPrescanRequiredFileHints(ctx, &rm, nil); added != 0 {
		t.Fatalf("explicit auxiliary exclusion must block corpus projection, added=%d hints=%+v", added, rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestProjectAnalyzerPrescanRequiredFileHints_AllowsBoundedInventorySupplement(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"src/pages/a.ets",
		"src/pages/b.ets",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("@Entry\nstruct Page {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mut := types.NewMutableState("source inventory")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"[grep: 2 matching files]",
			"[grep params: pattern=@Entry include=*.ets files_only=true]",
			"./src/pages/a.ets",
			"./src/pages/b.ets",
		}, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindGrep,
			Include:        "*.ets",
			FilesOnly:      true,
			ResultCount:    2,
			CandidateFiles: []string{"./src/pages/a.ets", "./src/pages/b.ets"},
		},
	})
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqEnumeration),
			RequiredFileHints: []types.RequiredFileHint{{
				Path:       "src/pages/a.ets",
				Confidence: 0.95,
			}},
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.45,
		},
	}
	ctx := &types.BusContext{Mutable: mut, RepoRoot: root}

	if added := projectAnalyzerPrescanRequiredFileHints(ctx, &rm, nil); added != 1 {
		t.Fatalf("bounded source-inventory prescan should supplement one peer file, added=%d hints=%+v", added, rm.AnalyzerHints.RequiredFileHints)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 2 || rm.AnalyzerHints.RequiredFileHints[1].Path != "src/pages/b.ets" {
		t.Fatalf("required_file hints = %+v, want src/pages/b.ets appended", rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestProjectAnalyzerPrescanRequiredFileHints_IgnoresSummaryOnlyCandidateRows(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"src/pages/a.ets",
		"src/pages/b.ets",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("@Entry\nstruct Page {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mut := types.NewMutableState("source inventory")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"[grep: 2 matching files]",
			"[grep params: pattern=@Entry include=*.ets files_only=true]",
			"./src/pages/a.ets",
			"./src/pages/b.ets",
		}, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Include:     "*.ets",
			FilesOnly:   true,
			ResultCount: 2,
		},
	})
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			Confidence:        0.95,
		},
	}
	ctx := &types.BusContext{Mutable: mut, RepoRoot: root}

	if added := projectAnalyzerPrescanRequiredFileHints(ctx, &rm, nil); added != 0 {
		t.Fatalf("summary-only prescan rows must not project required files, added=%d hints=%+v", added, rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestValidateAndBuildRequiredFileHints_PreservesRuntimeArtifactPath(t *testing.T) {
	root := t.TempDir()
	val := &analysisValidationResult{}
	got := validateAndBuildRequiredFileHintsWithContext(&types.BusContext{RepoRoot: root}, []emitRequiredFileParam{{
		Path:       "../customlogs/xxx_all.systrace",
		Confidence: 0.95,
		Rationale:  "user named a runtime trace artifact path",
	}}, val)
	if len(got) != 1 {
		t.Fatalf("runtime artifact path should be preserved, got %+v warnings=%v", got, val.Warnings)
	}
	if got[0].Path != "../customlogs/xxx_all.systrace" {
		t.Fatalf("runtime artifact path = %q, want ../customlogs/xxx_all.systrace", got[0].Path)
	}
	if containsAny(val.Warnings, "existing active file") {
		t.Fatalf("runtime artifact path should not be warned as missing current source, got %v", val.Warnings)
	}
}

func TestEmitAnalysisExecute_ProjectsRuntimeArtifactPathFromRequest(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	SetAnalysisLimits(AnalysisLimits{WarnBelowKeywords: 0, RejectBelowKeywords: 0})

	mu := types.NewMutableState("只分析 ../customlogs/xxx_all.systrace 这个 trace 文件，不分析代码")
	ctx := &types.BusContext{
		RepoRoot: t.TempDir(),
		Mutable:  mu,
	}
	payload := `{
		"intent": "root_cause",
		"scenario": "root_cause",
		"complexity": "moderate",
		"intent_confidence": 0.92,
		"complexity_confidence": 0.85,
		"kind_confidence": 0.9,
		"keywords": ["systrace", "trace", "runtime"],
		"entities": ["com.baidu.tieba"],
		"question_kind": "mechanism",
		"predicates": {
			"is_scalar_answer": false,
			"is_role_locate_lookup": false,
			"is_count_question": false,
			"is_cross_component": false,
			"is_relational_lookup": false,
			"is_category_enumeration": false,
			"is_history_lookup": false,
			"is_diagnostic_question": true,
			"has_per_member_table": false
		},
		"diagnostic_profile": {
			"is_diagnostic": true,
			"current_risk": false,
			"historical_regression": false,
			"current_version_check": false,
			"confidence": 0.95
		},
		"answer_role_profile": {
			"is_role_binding_requested": false,
			"confidence": 0.8
		},
		"error_granularity_profile": {
			"is_granularity_question": false,
			"confidence": 0.8
		},
		"runtime_artifact_scope_profile": {
			"requested_scope": "unspecified",
			"confidence": 0.8
		},
		"completeness_obligation": {"required": false, "source_quote": ""},
		"external_observation_policy": {
			"artifact_citation_mode": "external_only",
			"confidence": 0.95,
			"rationale": "runtime trace only"
		}
	}`
	res, err := (&EmitAnalysis{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("emit_analysis should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || !rm.HasRuntimeArtifactPathReference() {
		t.Fatalf("runtime artifact path reference should be projected from current request, rm=%+v", rm)
	}
	if len(rm.AnalyzerHints.RequiredFileHints) != 1 ||
		rm.AnalyzerHints.RequiredFileHints[0].Path != "../customlogs/xxx_all.systrace" {
		t.Fatalf("required file hints = %+v", rm.AnalyzerHints.RequiredFileHints)
	}
}

func TestEmitAnalysisExecute_NormalizesRequiredFilesBeforePersistAndSummary(t *testing.T) {
	root := t.TempDir()
	rel := "CodeAgent/packages/core/src/mcp/token-storage/types.ts"
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("export type OAuthCredentials = {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("OAuthCredentials 的格式是怎样的？")
	ctx := &types.BusContext{
		RepoRoot: root,
		Mutable:  mu,
		MultiGraph: requiredFileHintResolveTestGater{aliases: map[string]string{
			"packages/core/src/mcp/token-storage/types.ts": rel,
		}},
	}
	payload := withV4Required(`{
		"intent": "explain",
		"scenario": "architecture_explain",
		"complexity": "moderate",
		"keywords": ["OAuthCredentials", "credential", "decrypt"],
		"entities": ["OAuthCredentials"],
		"question_kind": "mechanism",
		"required_files": [
			{"path": "packages/core/src/mcp/token-storage/types.ts", "confidence": 0.9}
		]
	}`)
	res, err := (&EmitAnalysis{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("emit_analysis should succeed, got %q", res.Summary)
	}
	rm := mu.RequestModel()
	if rm == nil || len(rm.AnalyzerHints.RequiredFileHints) != 1 {
		t.Fatalf("required file hint not persisted: rm=%+v", rm)
	}
	if got := rm.AnalyzerHints.RequiredFileHints[0].Path; got != rel {
		t.Fatalf("persisted required file = %q, want %q", got, rel)
	}
	if !strings.Contains(res.Summary, `required_files=["`+rel+`"]`) {
		t.Fatalf("summary should expose normalized required_files, got %q", res.Summary)
	}
	if strings.Contains(res.Summary, `required_files=["packages/core`) {
		t.Fatalf("summary should not expose stale unprefixed required_files, got %q", res.Summary)
	}
}

func TestValidateAndBuildRequiredFileHints_EmptyPathDropped(t *testing.T) {
	val := &analysisValidationResult{}
	in := []emitRequiredFileParam{
		{Path: "", Confidence: 0.9},
		{Path: "  ", Confidence: 0.9},
		{Path: "internal/foo.go", Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != 1 {
		t.Errorf("empty/whitespace paths should be dropped; got %d hints", len(got))
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected a dropped-warning; got %v", val.Warnings)
	}
}

func TestValidateAndBuildRequiredFileHints_NaNConfidenceDropped(t *testing.T) {
	val := &analysisValidationResult{}
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: math.NaN()},
		{Path: "b.go", Confidence: 0.9},
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != 1 || got[0].Path != "b.go" {
		t.Errorf("NaN confidence should drop entry; got %+v", got)
	}
}

func TestValidateAndBuildRequiredFileHints_OutOfRangeClamped(t *testing.T) {
	val := &analysisValidationResult{}
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: -0.5},
		{Path: "b.go", Confidence: 1.5},
		{Path: "c.go", Confidence: 0.5},
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != 3 {
		t.Fatalf("clamping should keep all 3 entries; got %d", len(got))
	}
	if got[0].Confidence != 0 {
		t.Errorf("negative confidence should clamp to 0; got %v", got[0].Confidence)
	}
	if got[1].Confidence != 1 {
		t.Errorf("over-1 confidence should clamp to 1; got %v", got[1].Confidence)
	}
	if !containsAny(val.Warnings, "clamped") {
		t.Errorf("expected a clamping warning; got %v", val.Warnings)
	}
}

func TestValidateAndBuildRequiredFileHints_RationaleTruncated(t *testing.T) {
	long := strings.Repeat("x", requiredFileHintRationaleMaxChars+50)
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: 0.9, Rationale: long},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 1 {
		t.Fatalf("got %d hints", len(got))
	}
	if !strings.HasSuffix(got[0].Rationale, "…") {
		t.Errorf("over-cap rationale should end in …; got %q", got[0].Rationale)
	}
	if len([]rune(got[0].Rationale)) > requiredFileHintRationaleMaxChars {
		t.Errorf("rationale runes = %d, want ≤ %d", len([]rune(got[0].Rationale)), requiredFileHintRationaleMaxChars)
	}
}

func TestValidateAndBuildRequiredFileHints_HardCapEnforced(t *testing.T) {
	val := &analysisValidationResult{}
	in := make([]emitRequiredFileParam, requiredFileHintsMax+5)
	for i := range in {
		in[i] = emitRequiredFileParam{Path: "f" + string(rune('a'+i)) + ".go", Confidence: 0.9}
	}
	got := validateAndBuildRequiredFileHints(in, val)
	if len(got) != requiredFileHintsMax {
		t.Errorf("hard cap = %d, got %d", requiredFileHintsMax, len(got))
	}
	if !containsAny(val.Warnings, "dropped") {
		t.Errorf("expected dropped-warning for over-cap entries; got %v", val.Warnings)
	}
}

func TestValidateAndBuildRequiredFileHints_LowConfidenceKept(t *testing.T) {
	// Threshold gating happens at consumer (explorer), not here.
	// Even confidence=0 entries pass through to preserve "I'm unsure"
	// signal. The consumer drops them at <0.5.
	in := []emitRequiredFileParam{
		{Path: "a.go", Confidence: 0.1, Rationale: "low"},
		{Path: "b.go", Confidence: 0.0},
	}
	got := validateAndBuildRequiredFileHints(in, nil)
	if len(got) != 2 {
		t.Errorf("low-confidence entries should pass through; got %d", len(got))
	}
}

// helper
func containsAny(s []string, sub string) bool {
	for _, x := range s {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

// === skill prompt smoke test ===

func TestEmitAnalysisSchema_RequiredFilesPresent(t *testing.T) {
	emitAnalysisSchemaOnce.Do(buildEmitAnalysisSchema)
	if !strings.Contains(string(emitAnalysisSchemaCache), `"required_files"`) {
		t.Error("emit_analysis schema should declare required_files property")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `"confidence"`) {
		t.Error("required_files items should declare confidence property")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `"rationale"`) {
		t.Error("required_files items should declare rationale property")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `"requested_dimension_indices"`) {
		t.Error("required_files items should declare optional requested dimension ownership")
	}
	if !strings.Contains(string(emitAnalysisSchemaCache), `"requested_dimension_navigation_only"`) {
		t.Error("required_files items should declare the explicit navigation-only alternative")
	}
	// Threshold bands documented in description (LLM-facing).
	if !strings.Contains(string(emitAnalysisSchemaCache), `0.8`) {
		t.Error("required_files description should mention 0.8 threshold band")
	}
}

// §40.47 fold-in (finding A0): a runtime-artifact path in the request prose
// under an external-only citation policy is projected as a SYSTEM hint (0.8,
// no indices). The unresolved-owner marker describes the model's own
// declarations, so a fully declared model roster plus that system hint must
// leave the marker nil, while the projected hint itself stays (typed origin).
func TestEmitAnalysisExecute_SystemProjectedRuntimeArtifactHintNeverEntersOwnerUnresolvedMarker(t *testing.T) {
	root := ownershipFixtureRoot(t)
	raw := "根据 /tmp/app.systrace 解释 Config 的解析机制和覆盖方式"
	mu := types.NewMutableState(raw)
	payload := withV4Required(`{
		"intent":"explain",
		"scenario":"config_trace",
		"complexity":"moderate",
		"keywords":["config","parse","override"],
		"entities":["Config"],
		"question_kind":"config_mapping",
		"predicate_axis":"configure",
		"requested_answer_dimensions":{
			"is_dimensioned_answer":true,
			"confidence":0.95,
			"dimensions":[
				{"index":1,"label":"解析机制","role":"function_or_purpose","required":true,"source_quote":"解析机制"},
				{"index":2,"label":"覆盖方式","role":"branch_behavior","required":true,"source_quote":"覆盖方式"}
			]
		},
		"external_observation_policy":{"artifact_citation_mode":"external_only","current_source_mode":"default","confidence":0.9},
		"required_files":[{"path":"config/load.go","confidence":0.95,"rationale":"implementation entry","requested_dimension_indices":[1,2]}]
	}`)
	res, err := (&EmitAnalysis{}).Execute(&types.BusContext{RepoRoot: root, Mutable: mu}, json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || mu.RequestModel() == nil {
		t.Fatalf("fully declared ownership must be accepted: %+v", res)
	}
	hints := mu.RequestModel().AnalyzerHints
	if hints.DimensionOwnerUnresolved != nil {
		t.Fatalf("system-projected hint must not become a model declaration gap: marker=%+v hints=%+v", hints.DimensionOwnerUnresolved, hints.RequiredFileHints)
	}
	if strings.Contains(res.Summary, "without a declared role") {
		t.Fatalf("summary labels a system-projected hint as a model omission:\n%s", res.Summary)
	}
	var projected *types.RequiredFileHint
	for i := range hints.RequiredFileHints {
		if hints.RequiredFileHints[i].Path == "/tmp/app.systrace" {
			projected = &hints.RequiredFileHints[i]
		}
	}
	if projected == nil {
		t.Fatalf("runtime artifact path must still be projected for artifact-lane routing: %+v", hints.RequiredFileHints)
	}
	if projected.ModelDeclared() || projected.Origin != types.RequiredFileHintOriginRuntimeArtifactPath {
		t.Fatalf("projected hint must carry its typed system origin, got %+v", *projected)
	}
	for _, hint := range hints.RequiredFileHints {
		if hint.Path == "config/load.go" && !hint.ModelDeclared() {
			t.Fatalf("model-declared hint lost its origin: %+v", hint)
		}
	}
}
