package authority

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// fakeLocator mirrors logtriage_test's helper for stand-alone testing.
type fakeLocator struct {
	defs    map[string][]types.SymbolLocation
	inFiles map[string][]types.SymbolLocation
}

func (f *fakeLocator) LocateSymbol(name string) []types.SymbolLocation {
	if f == nil {
		return nil
	}
	return f.defs[name]
}

func (f *fakeLocator) SymbolsInFile(file string) []types.SymbolLocation {
	if f == nil {
		return nil
	}
	return f.inFiles[file]
}

// TestComputeForEvidence_NilBusSafePassthrough catches "we forgot to
// nil-guard". A nil bus must NOT panic — single-shot CLI flows pass
// nil for unit tests.
func TestComputeForEvidence_NilBusSafePassthrough(t *testing.T) {
	item := types.EvidenceItem{Scope: types.ScopeLine}
	o, a, _ := ComputeForEvidence(item, nil)
	if o != types.ClaimOriginUnknown || a != types.AuthorityUnknown {
		t.Errorf("nil bus: got (origin=%q, auth=%q); want zero", o, a)
	}
}

// TestComputeForEvidence_SchemaLevelScopeAlwaysFactual asserts file/
// crossfile/negative scopes pin to factual current_repo regardless of
// log bundle state. These scopes are structural assertions about
// current code; log drift cannot downgrade them.
func TestComputeForEvidence_SchemaLevelScopeAlwaysFactual(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	bus.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Frames: []types.LogFrame{
			{File: "x.go", Line: 10, Func: "Old", Raw: "x.go:10 Old"},
		}}},
	})
	for _, scope := range []types.EvidenceScope{
		types.ScopeFile, types.ScopeCrossfile, types.ScopeNegative,
	} {
		item := types.EvidenceItem{
			Scope:           scope,
			Source:          "x.go",
			LineStart:       10,
			GroundingStatus: types.GroundingGrounded,
			FileRoleLabel:   types.FileRoleConfigCanonical,
		}
		o, a, _ := ComputeForEvidence(item, bus)
		if o != types.ClaimOriginCurrentRepo {
			t.Errorf("scope=%q: Origin = %q; want current_repo", scope, o)
		}
		if a != types.AuthorityFactual {
			t.Errorf("scope=%q: Authority = %q; want factual", scope, a)
		}
	}
}

// TestComputeForEvidence_IllustrativeOnlyForcesIllustrative mirrors
// the documented carve-out: ContextRole=illustrative_only is a
// structural opt-out from causal claims regardless of grounding.
func TestComputeForEvidence_IllustrativeOnlyForcesIllustrative(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       10,
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded,
		ContextRole:     types.EvidenceContextRoleIllustrativeOnly,
	}
	_, a, _ := ComputeForEvidence(item, bus)
	if a != types.AuthorityIllustrative {
		t.Errorf("illustrative_only: Authority = %q; want illustrative", a)
	}
}

// TestComputeForEvidence_UngroundedIsIllustrative mirrors the carve-
// out for ungrounded items.
func TestComputeForEvidence_UngroundedIsIllustrative(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       10,
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingUngrounded,
	}
	_, a, _ := ComputeForEvidence(item, bus)
	if a != types.AuthorityIllustrative {
		t.Errorf("ungrounded: Authority = %q; want illustrative", a)
	}
}

// TestComputeForEvidence_CurrentRepoLineGrounded is the dominant
// no-log case: a grounded line-scoped item gets current_repo +
// factual.
func TestComputeForEvidence_CurrentRepoLineGrounded(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       10,
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}
	o, a, _ := ComputeForEvidence(item, bus)
	if o != types.ClaimOriginCurrentRepo {
		t.Errorf("Origin = %q; want current_repo", o)
	}
	if a != types.AuthorityFactual {
		t.Errorf("Authority = %q; want factual", a)
	}
}

// TestComputeForEvidence_RecoveredFactualWithoutLog: round-3
// refinement — without an attached log/perf bundle, GroundingRecovered
// is plain LLM line-typo correction (not drift). Treat as factual so
// the user doesn't see a fake "log-derived" caveat for an
// in-pipeline self-correction.
func TestComputeForEvidence_RecoveredFactualWithoutLog(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       12,
		GroundingStatus: types.GroundingRecovered,
		GroundingTier:   types.TierFQNameSameFile,
	}
	_, a, _ := ComputeForEvidence(item, bus)
	if a != types.AuthorityFactual {
		t.Errorf("recovered without log: Authority = %q; want factual", a)
	}
}

// TestComputeForEvidence_RecoveredConditionalWithLog: same item with
// an attached log bundle — recovery now signals likely drift between
// the artifact and current code, so soften to conditional.
func TestComputeForEvidence_RecoveredConditionalWithLog(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	bus.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Frames: []types.LogFrame{
			{File: "other.go", Line: 1, Func: "X", Raw: "other.go:1 X"},
		}}},
	})
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       12,
		GroundingStatus: types.GroundingRecovered,
		GroundingTier:   types.TierFQNameSameFile,
	}
	_, a, _ := ComputeForEvidence(item, bus)
	if a != types.AuthorityConditional {
		t.Errorf("recovered with log: Authority = %q; want conditional", a)
	}
}

// TestComputeForEvidence_LogMatchPerfectCrossSource: a log frame at
// the same (file, line) as the evidence item, with the symbol present
// at the same line in the repo. Both sides confirm — Origin=cross,
// Authority=factual.
func TestComputeForEvidence_LogMatchPerfectCrossSource(t *testing.T) {
	SetSymbolLocatorProvider(func(graph any) types.SymbolLocator {
		return &fakeLocator{
			defs: map[string][]types.SymbolLocation{
				"Run": {{File: "x.go", Line: 100, EndLine: 200}},
			},
		}
	})
	defer SetSymbolLocatorProvider(nil)

	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	bus.Mutable.SetSearchGraph(struct{}{})
	bus.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Frames: []types.LogFrame{
			{File: "x.go", Line: 100, Func: "Run", Raw: "x.go:100 Run"},
		}}},
	})
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       100,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Run",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}
	o, a, reason := ComputeForEvidence(item, bus)
	if o != types.ClaimOriginCrossSource {
		t.Errorf("Origin = %q; want cross_source (reason=%q)", o, reason)
	}
	if a != types.AuthorityFactual {
		t.Errorf("Authority = %q; want factual (reason=%q)", a, reason)
	}
}

// TestComputeForEvidence_LogMatchLineDriftConditional: log frame says
// line 50, current code has Run at line 100 — line drift, ceiling
// downgrades to conditional.
func TestComputeForEvidence_LogMatchLineDriftConditional(t *testing.T) {
	SetSymbolLocatorProvider(func(graph any) types.SymbolLocator {
		return &fakeLocator{
			defs: map[string][]types.SymbolLocation{
				"Run": {{File: "x.go", Line: 100, EndLine: 110}},
			},
		}
	})
	defer SetSymbolLocatorProvider(nil)

	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	bus.Mutable.SetSearchGraph(struct{}{})
	bus.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Frames: []types.LogFrame{
			{File: "x.go", Line: 50, Func: "Run", Raw: "x.go:50 Run"},
		}}},
	})
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "x.go",
		LineStart:       50,
		AnchorSymbol:    "Run",
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded,
	}
	o, a, reason := ComputeForEvidence(item, bus)
	if o != types.ClaimOriginLog {
		t.Errorf("Origin = %q; want log (reason=%q)", o, reason)
	}
	if a != types.AuthorityConditional {
		t.Errorf("Authority = %q; want conditional (reason=%q)", a, reason)
	}
}

// TestComputeForEvidence_LogMatchFileMovedHistorical: log says the
// function was in old/x.go, current code has it in new/y.go. file_
// moved → historical.
func TestComputeForEvidence_LogMatchFileMovedHistorical(t *testing.T) {
	SetSymbolLocatorProvider(func(graph any) types.SymbolLocator {
		return &fakeLocator{
			defs: map[string][]types.SymbolLocation{
				"Run": {{File: "new/y.go", Line: 200, EndLine: 250}},
			},
		}
	})
	defer SetSymbolLocatorProvider(nil)

	bus := &types.BusContext{Mutable: types.NewMutableState("test-objective")}
	bus.Mutable.SetSearchGraph(struct{}{})
	bus.Mutable.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Frames: []types.LogFrame{
			{File: "old/x.go", Line: 30, Func: "Run", Raw: "old/x.go:30 Run"},
		}}},
	})
	item := types.EvidenceItem{
		Scope:           types.ScopeLine,
		Source:          "old/x.go",
		LineStart:       30,
		AnchorSymbol:    "Run",
		AnchorKind:      types.AnchorDefinition,
		GroundingStatus: types.GroundingGrounded,
	}
	o, a, reason := ComputeForEvidence(item, bus)
	if o != types.ClaimOriginLog {
		t.Errorf("Origin = %q; want log (reason=%q)", o, reason)
	}
	if a != types.AuthorityHistorical {
		t.Errorf("Authority = %q; want historical (reason=%q)", a, reason)
	}
}
