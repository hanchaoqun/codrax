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
