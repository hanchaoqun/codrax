package logtriage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestValidateBundle_EmptyInputReturnsNil covers the early-exit: no
// errors, no residue, no lang / signals → nil so consumers skip.
func TestValidateBundle_EmptyInputReturnsNil(t *testing.T) {
	got := ValidateBundle(ValidateInput{}, "")
	if got != nil {
		t.Errorf("expected nil for empty input, got %+v", got)
	}
}

// TestValidateBundle_ResolvesPathsAndDropsHallucinations builds a
// repo with one real file, emits two frames (one real, one fake), and
// verifies the hallucinated path is cleared while the real one flows
// through to ResolvedFiles.
func TestValidateBundle_ResolvesPathsAndDropsHallucinations(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "internal", "svc", "handler.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := ValidateInput{
		Meta: types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{
			{
				Type: "runtime error",
				Frames: []types.LogFrame{
					// Real frame. Path matches repo.
					{File: "internal/svc/handler.go", Line: 42,
						Func: "svc.Handler", Raw: "frame1", Confidence: 0.9},
					// Hallucinated frame. Path does not exist.
					{File: "internal/fake/nope.go", Line: 10,
						Func: "fake.Nope", Raw: "frame2", Confidence: 0.9},
				},
			},
		},
		RawLogBytes: 100,
	}
	got := ValidateBundle(in, dir)
	if got == nil {
		t.Fatal("nil bundle")
	}
	// ResolvedFiles should contain only the real one.
	want := []string{"internal/svc/handler.go"}
	if !reflect.DeepEqual(got.ResolvedFiles, want) {
		t.Errorf("ResolvedFiles = %v, want %v", got.ResolvedFiles, want)
	}
	// The hallucinated frame's File should be cleared but frame kept.
	f2 := got.Errors[0].Frames[1]
	if f2.File != "" {
		t.Errorf("hallucinated frame.File not cleared: %q", f2.File)
	}
	if f2.Func != "fake.Nope" {
		t.Errorf("hallucinated frame.Func lost: %q", f2.Func)
	}
	// Entities should include both Func last-segments AND the signal name.
	wantEntities := map[string]bool{
		"Handler": true, "Nope": true, "panic": true, "runtime error": true,
	}
	for k := range wantEntities {
		found := false
		for _, e := range got.Entities {
			if e == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Entities missing %q: %v", k, got.Entities)
		}
	}
	// IntentHint should be RootCause (real frame + panic signal).
	if got.IntentHint != types.IntentRootCause {
		t.Errorf("IntentHint = %q, want root_cause", got.IntentHint)
	}
}

func TestValidateBundle_OperationalFailureSignalDerivesRootCauseHint(t *testing.T) {
	got := ValidateBundle(ValidateInput{
		Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalValidation, types.SignalLogic}},
		Errors: []types.LogError{{
			Type:    "ValidationError",
			Message: "missing evidence before completion",
		}},
	}, t.TempDir())
	if got == nil {
		t.Fatal("ValidateBundle returned nil")
	}
	if got.IntentHint != types.IntentRootCause {
		t.Fatalf("IntentHint = %q, want root_cause", got.IntentHint)
	}
}

// TestValidateBundle_RuntimeInternalFilteredOut verifies Go runtime
// / node / java.base paths never reach ResolvedFiles even when the
// LLM emits them.
func TestValidateBundle_RuntimeInternalFilteredOut(t *testing.T) {
	dir := t.TempDir()
	in := ValidateInput{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{
				{File: "/usr/local/go/src/runtime/proc.go", Line: 100,
					Raw: "frame", Confidence: 0.9},
				{File: "node:internal/modules/cjs/loader.js", Line: 50,
					Raw: "frame", Confidence: 0.9},
				{File: "java.base/java/lang/Thread.java", Line: 200,
					Raw: "frame", Confidence: 0.9},
			},
		}},
	}
	got := ValidateBundle(in, dir)
	if got == nil || len(got.ResolvedFiles) != 0 {
		t.Errorf("expected empty ResolvedFiles, got %v",
			got.ResolvedFiles)
	}
}

// TestValidateBundle_LineZeroStaysInEntities confirms the recovery
// contract: frames with Line=0 keep Func tokens flowing into Entities
// but do NOT contribute to ResolvedFiles.
func TestValidateBundle_LineZeroStaysInEntities(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(target, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := ValidateInput{
		Errors: []types.LogError{{
			Type: "oops",
			Frames: []types.LogFrame{
				{File: "foo.go", Line: 0, Func: "pkg.Bar",
					Raw: "line unknown", Confidence: 0.9},
			},
		}},
	}
	got := ValidateBundle(in, dir)
	if got == nil {
		t.Fatal("nil bundle")
	}
	if len(got.ResolvedFiles) != 0 {
		t.Errorf("Line=0 frame should not produce ResolvedFiles: %v",
			got.ResolvedFiles)
	}
	hasBar := false
	for _, e := range got.Entities {
		if e == "Bar" {
			hasBar = true
		}
	}
	if !hasBar {
		t.Errorf("Line=0 frame Func token missing from Entities: %v",
			got.Entities)
	}
}

// TestValidateBundle_LowConfidenceExcludedFromResolvedFiles covers the
// MinResolvedConfidence gate: confidence < 0.6 keeps the frame but
// does not seed ResolvedFiles.
func TestValidateBundle_LowConfidenceExcludedFromResolvedFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(target, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := ValidateInput{
		Errors: []types.LogError{{
			Type: "low-conf",
			Frames: []types.LogFrame{
				{File: "foo.go", Line: 10, Func: "pkg.Maybe",
					Raw: "unsure frame", Confidence: 0.4},
			},
		}},
	}
	got := ValidateBundle(in, dir)
	if len(got.ResolvedFiles) != 0 {
		t.Errorf("low-confidence frame leaked into ResolvedFiles: %v",
			got.ResolvedFiles)
	}
}

// TestValidateBundle_FunctionMismatchExcludedFromResolvedFiles guards
// the "path exists but symbol is absent" failure mode: a frame that
// points at a real repo file but names a function the file does not
// contain must not seed ResolvedFiles.
func TestValidateBundle_FunctionMismatchExcludedFromResolvedFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "internal", "agent", "analyzer.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package agent\n\nfunc runAnalyzer() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := ValidateInput{
		Meta: types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				File:       "internal/agent/analyzer.go",
				Line:       3,
				Func:       "main.writeSession",
				Raw:        "main.writeSession(0x0)",
				Confidence: 0.9,
			}},
		}},
	}
	got := ValidateBundle(in, dir)
	if got == nil {
		t.Fatal("nil bundle")
	}
	if len(got.ResolvedFiles) != 0 {
		t.Fatalf("mismatched function should stay out of ResolvedFiles: %v", got.ResolvedFiles)
	}
	if got.Errors[0].Frames[0].File != "" {
		t.Fatalf("mismatched function should clear frame.File, got %q", got.Errors[0].Frames[0].File)
	}
	hasWriteSession := false
	for _, e := range got.Entities {
		if e == "writeSession" {
			hasWriteSession = true
			break
		}
	}
	if !hasWriteSession {
		t.Fatalf("function token should remain available to Entities, got %v", got.Entities)
	}
}

// TestValidateBundle_EntitiesCapAt32 verifies the Entities cap holds
// when the LLM emits a giant frame count.
func TestValidateBundle_EntitiesCapAt32(t *testing.T) {
	frames := make([]types.LogFrame, 100)
	for i := range frames {
		frames[i] = types.LogFrame{
			Func: "pkg.Func" + strings.Repeat("x", i+1),
			Raw:  "r", Confidence: 0.5,
		}
	}
	in := ValidateInput{
		Errors: []types.LogError{{Type: "lots", Frames: frames}},
	}
	got := ValidateBundle(in, "")
	if got == nil {
		t.Fatal("nil")
	}
	if len(got.Entities) > types.LogBundleCaps.MaxEntities {
		t.Errorf("entities cap violated: got %d, cap %d",
			len(got.Entities), types.LogBundleCaps.MaxEntities)
	}
}

// TestValidateBundle_CauseDepthTruncated verifies a deeply-nested
// Cause chain is cut off at MaxCauseDepth.
func TestValidateBundle_CauseDepthTruncated(t *testing.T) {
	// Build a 10-level chain.
	root := types.LogError{Type: "L0"}
	cur := &root
	for i := 1; i <= 10; i++ {
		cur.Cause = &types.LogError{Type: "L" + string(rune('0'+i))}
		cur = cur.Cause
	}

	in := ValidateInput{Errors: []types.LogError{root}}
	got := ValidateBundle(in, "")
	if got == nil {
		t.Fatal("nil bundle")
	}
	// Count depth.
	depth := 1
	e := &got.Errors[0]
	for e.Cause != nil {
		depth++
		e = e.Cause
	}
	if depth > types.LogBundleCaps.MaxCauseDepth {
		t.Errorf("depth = %d exceeds cap %d", depth,
			types.LogBundleCaps.MaxCauseDepth)
	}
}

// TestValidateBundle_SignalsCanonicalOrder confirms signals end up in
// canonical enum order regardless of input order and are deduped.
func TestValidateBundle_SignalsCanonicalOrder(t *testing.T) {
	in := ValidateInput{
		Meta: types.LogMeta{
			Signals: []types.LogSignal{
				types.SignalDB,
				types.SignalPanic, // canonical order: panic < db
				types.SignalPanic, // dup
				types.LogSignal("bogus"),
			},
		},
		Errors: []types.LogError{{Type: "X"}},
	}
	got := ValidateBundle(in, "")
	want := []types.LogSignal{types.SignalPanic, types.SignalDB}
	if !reflect.DeepEqual(got.Meta.Signals, want) {
		t.Errorf("signals = %v, want %v", got.Meta.Signals, want)
	}
}

// TestValidateBundle_CoverageDerivation sanity-checks the
// 1 - residue/raw formula.
func TestValidateBundle_CoverageDerivation(t *testing.T) {
	in := ValidateInput{
		Meta: types.LogMeta{Lang: "go"},
		Residue: types.LogResidue{
			UnknownChunks: []string{"abc", "defgh"}, // total 8
		},
		Errors:      []types.LogError{{Type: "X"}},
		RawLogBytes: 100,
	}
	got := ValidateBundle(in, "")
	want := 0.92
	if got.Coverage < want-0.001 || got.Coverage > want+0.001 {
		t.Errorf("coverage = %.3f, want %.3f", got.Coverage, want)
	}
}

// TestMergeBundles_SignalUnion_ErrorsConcat pins two-step merge: two
// bundles merge Signals via union and concat Errors in input order.
func TestMergeBundles_SignalUnion_ErrorsConcat(t *testing.T) {
	b1 := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "go",
			Signals: []types.LogSignal{types.SignalPanic},
		},
		Errors: []types.LogError{{Type: "E1"}},
	}
	b2 := &types.LogBundle{
		Meta: types.LogMeta{
			Lang:    "go",
			Signals: []types.LogSignal{types.SignalOOM},
		},
		Errors: []types.LogError{{Type: "E2"}},
	}
	got := MergeBundles([]*types.LogBundle{b1, b2}, 200)
	if got.Meta.Lang != "go" {
		t.Errorf("Lang = %q", got.Meta.Lang)
	}
	if len(got.Errors) != 2 || got.Errors[0].Type != "E1" || got.Errors[1].Type != "E2" {
		t.Errorf("errors concat fail: %+v", got.Errors)
	}
	wantSignals := []types.LogSignal{types.SignalPanic, types.SignalOOM}
	if !reflect.DeepEqual(got.Meta.Signals, wantSignals) {
		t.Errorf("signals union = %v, want %v",
			got.Meta.Signals, wantSignals)
	}
}

// TestMergeEntities_CapAndDedup verifies union semantics + cap.
// nil oracle = pre-commit-52 byte-identical behaviour (no validation).
func TestMergeEntities_CapAndDedup(t *testing.T) {
	existing := []string{"a", "b"}
	fromBundle := []string{"b", "c", "d"}
	got := MergeEntities(existing, fromBundle, nil)
	want := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// stubOracle is the minimal types.SymbolOracle for the merge tests.
// Any name in `valid` reports tier=1; everything else absent.
type stubOracle struct {
	valid map[string]bool
}

func (s *stubOracle) SymbolExists(name string) (bool, int) {
	if s == nil || s.valid == nil {
		return false, 0
	}
	if s.valid[name] {
		return true, 1
	}
	return false, 0
}

// SymbolExistsFlat (Fix E) — same valid-set as SymbolExists; the
// logtriage merge tests don't exercise cross-form behaviour (their
// hallucination scenario uses unique fake names), so this stub
// just delegates to the exact path. Production code uses the
// repomap graphOracle which has the real flat index.
func (s *stubOracle) SymbolExistsFlat(name string) (bool, int) {
	return s.SymbolExists(name)
}

// TestMergeEntities_OracleDropsHallucinated pins the commit-52 P1
// contract: bundle-extracted entities the LLM made up (no matching
// symbol in the repomap) get dropped before merge. Entities already
// in `existing` (not from this bundle) are preserved unconditionally —
// the oracle gate applies only to bundle additions, so a previously
// validated entity is never retroactively invalidated.
func TestMergeEntities_OracleDropsHallucinated(t *testing.T) {
	oracle := &stubOracle{valid: map[string]bool{"RealHandler": true, "AlsoReal": true}}
	existing := []string{"PreservedEntity"}
	fromBundle := []string{"RealHandler", "HallucinatedFunc", "AlsoReal", "AnotherFake"}
	got := MergeEntities(existing, fromBundle, oracle)
	want := []string{"PreservedEntity", "RealHandler", "AlsoReal"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("oracle-gated merge: got %v, want %v", got, want)
	}
}

// TestMergeEntities_OracleNilIsPassThrough pins the back-compat
// contract: passing nil oracle reproduces the pre-commit-52 merge
// (no validation, every dedup-survivor merged).
func TestMergeEntities_OracleNilIsPassThrough(t *testing.T) {
	existing := []string{"x"}
	fromBundle := []string{"AnyName1", "AnyName2"}
	got := MergeEntities(existing, fromBundle, nil)
	want := []string{"x", "AnyName1", "AnyName2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nil oracle should pass through: got %v, want %v", got, want)
	}
}

// stubTierOracle reports a configurable tier for each valid name so
// the tier-floor gate (Tier 3+ rejected) can be exercised.
type stubTierOracle struct {
	tiers map[string]int
}

func (s *stubTierOracle) SymbolExists(name string) (bool, int) {
	if s == nil || s.tiers == nil {
		return false, 0
	}
	if t, ok := s.tiers[name]; ok {
		return true, t
	}
	return false, 0
}

// SymbolExistsFlat (Fix E) — delegate to exact path; tier-floor
// tests don't exercise cross-form lookup.
func (s *stubTierOracle) SymbolExistsFlat(name string) (bool, int) {
	return s.SymbolExists(name)
}

// TestMergeEntities_OracleRejectsTier3Plus pins that low-tier
// matches don't count as validation: a Tier 4 (path-only) symbol
// hit is dropped just like an absent symbol.
func TestMergeEntities_OracleRejectsTier3Plus(t *testing.T) {
	oracle := &stubTierOracle{tiers: map[string]int{
		"Tier1Real": 1,
		"Tier2Real": 2,
		"Tier3Weak": 3,
		"Tier4Weak": 4,
	}}
	got := MergeEntities(nil,
		[]string{"Tier1Real", "Tier2Real", "Tier3Weak", "Tier4Weak"},
		oracle)
	want := []string{"Tier1Real", "Tier2Real"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tier-3+ rejection: got %v, want %v", got, want)
	}
}

// TestMergeResolvedFiles_LogPrependsRanker pins the merge contract:
// log-derived files outrank ranker output.
func TestMergeResolvedFiles_LogPrependsRanker(t *testing.T) {
	log := []string{"frame/one.go", "frame/two.go"}
	ranked := []string{"ranker/a.go", "frame/one.go" /* dup */, "ranker/b.go"}
	got := MergeResolvedFiles(log, ranked)
	want := []string{"frame/one.go", "frame/two.go", "ranker/a.go", "ranker/b.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestValidateBundle_NoRepoRootSkipsPathResolution covers the
// filesystem-free path (unit tests, dry runs). Paths cleared, Funcs
// survive to Entities.
func TestValidateBundle_NoRepoRootSkipsPathResolution(t *testing.T) {
	in := ValidateInput{
		Errors: []types.LogError{{
			Type: "T",
			Frames: []types.LogFrame{
				{File: "wherever.go", Line: 10, Func: "pkg.Handler",
					Raw: "r", Confidence: 0.9},
			},
		}},
	}
	got := ValidateBundle(in, "")
	if got == nil {
		t.Fatal("nil")
	}
	// File should be cleared because repoRoot="" disables resolution.
	if got.Errors[0].Frames[0].File != "" {
		t.Errorf("expected File cleared, got %q",
			got.Errors[0].Frames[0].File)
	}
	hasHandler := false
	for _, e := range got.Entities {
		if e == "Handler" {
			hasHandler = true
		}
	}
	if !hasHandler {
		t.Errorf("entity missing: %v", got.Entities)
	}
}
