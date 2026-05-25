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
	// Threshold bands documented in description (LLM-facing).
	if !strings.Contains(string(emitAnalysisSchemaCache), `0.8`) {
		t.Error("required_files description should mention 0.8 threshold band")
	}
}
