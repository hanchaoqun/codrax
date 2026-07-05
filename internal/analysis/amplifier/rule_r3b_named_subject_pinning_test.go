package amplifier

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestR3bNamedSubjectMustInclude pins the A1 answer-surface lane: a
// resolved user-named symbol on a single-subject explanation pins into
// MustInclude (soft ViolMustInclude enforcement downstream); enumeration
// lanes (R3's territory), runtime-artifact runs, unresolved entities, and
// the 3-pin bound all keep it silent.
func TestR3bNamedSubjectMustInclude(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			EntityProvenance: []types.EntityProvenance{
				{Surface: "gate.Run", Resolution: types.EntityResolutionSymbol, Resolved: true},
				{Surface: "prose concept", Resolution: types.EntityResolutionInferredConcept},
			},
		},
	}
	contract := &types.AnswerContract{}
	obs := r3bNamedSubjectMustInclude(rm, contract)
	if obs == nil {
		t.Fatal("resolved user-named symbol must pin")
	}
	if len(contract.MustInclude) != 1 || contract.MustInclude[0] != "gate.Run" {
		t.Fatalf("MustInclude=%v", contract.MustInclude)
	}
	if len(contract.MustIncludeTerms) != 1 || contract.MustIncludeTerms[0].Source != types.ContractTermSourceAnalyzerEntity {
		t.Fatalf("typed term missing: %+v", contract.MustIncludeTerms)
	}
	if !strings.Contains(obs.After, "gate.Run") {
		t.Fatalf("observation must name the pin: %+v", obs)
	}

	// Dedupe: an already-present term is not double-pinned.
	again := r3bNamedSubjectMustInclude(rm, contract)
	if again != nil || len(contract.MustInclude) != 1 {
		t.Fatalf("dedupe failed: %v %+v", contract.MustInclude, again)
	}

	// Unresolved-only → silent.
	empty := &types.AnswerContract{}
	if got := r3bNamedSubjectMustInclude(types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{EntityProvenance: []types.EntityProvenance{
			{Surface: "MaybeThing", Resolution: types.EntityResolutionSymbol, Resolved: false},
		}},
	}, empty); got != nil || len(empty.MustInclude) != 0 {
		t.Fatalf("unresolved entity must not pin: %v", empty.MustInclude)
	}
}

// QNO F1 (2026-07-05): a resolved SYMBOL surface must pin with the
// typed symbol kind — the inferred kind routes dotted spellings to
// file_stem, whose citation-coverage hit logic made the pin vacuous
// under the Go pkg/pkg.go convention (any stem-`gate` citation
// satisfied a "gate.Run" obligation). Resolved FILE surfaces keep the
// inferred file_stem kind: citation coverage is the correct semantics
// for paths.
func TestR3bNamedSubject_SymbolKindForced(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			EntityProvenance: []types.EntityProvenance{
				{Surface: "gate.Run", Resolution: types.EntityResolutionSymbol, Resolved: true},
				{Surface: "internal/analysis/gate/gate.go", Resolution: types.EntityResolutionFile, Resolved: true},
			},
		},
	}
	contract := &types.AnswerContract{}
	if obs := r3bNamedSubjectMustInclude(rm, contract); obs == nil {
		t.Fatal("expected pins")
	}
	if len(contract.MustIncludeTerms) != 2 {
		t.Fatalf("expected 2 typed terms, got %+v", contract.MustIncludeTerms)
	}
	if contract.MustIncludeTerms[0].Text != "gate.Run" || contract.MustIncludeTerms[0].Kind != types.ContractTermSymbol {
		t.Fatalf("symbol surface must pin Kind=symbol (file_stem citation bypass closed): %+v", contract.MustIncludeTerms[0])
	}
	if contract.MustIncludeTerms[1].Kind != types.ContractTermFileStem {
		t.Fatalf("file surface keeps inferred file_stem kind: %+v", contract.MustIncludeTerms[1])
	}
}

// QNO 备注a (2026-07-05): receiver-paren spellings fail
// AnchorTokenShaped, but the resolved subject is still constrained
// through its token-shaped bare tail. Non-symbol rows and paren
// spellings without a usable tail stay silent.
func TestR3bNamedSubject_ReceiverParenSurfacePinsTail(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			EntityProvenance: []types.EntityProvenance{
				{Surface: "(*Gate).Run", Resolution: types.EntityResolutionSymbol, Resolved: true},
			},
		},
	}
	contract := &types.AnswerContract{}
	if obs := r3bNamedSubjectMustInclude(rm, contract); obs == nil {
		t.Fatal("receiver-paren resolved symbol must pin its bare tail")
	}
	if len(contract.MustInclude) != 1 || contract.MustInclude[0] != "Run" {
		t.Fatalf("expected bare tail pin [Run], got %v", contract.MustInclude)
	}
	if contract.MustIncludeTerms[0].Kind != types.ContractTermSymbol {
		t.Fatalf("tail pin must carry Kind=symbol: %+v", contract.MustIncludeTerms[0])
	}

	// A resolved FILE row with a non-token surface must not enter the
	// tail lane (tails are a symbol-subject concept).
	fileOnly := &types.AnswerContract{}
	if got := r3bNamedSubjectMustInclude(types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{EntityProvenance: []types.EntityProvenance{
			{Surface: "(some path).go", Resolution: types.EntityResolutionFile, Resolved: true},
		}},
	}, fileOnly); got != nil || len(fileOnly.MustInclude) != 0 {
		t.Fatalf("non-token file surface must stay silent: %v", fileOnly.MustInclude)
	}
}
