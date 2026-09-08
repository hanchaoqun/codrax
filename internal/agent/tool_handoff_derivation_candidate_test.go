package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1627CandidateHandoffFinalizerRetainsBoundary(t *testing.T) {
	for _, status := range []types.GroundingStatus{types.GroundingGrounded, types.GroundingRecovered} {
		item := types.EvidenceItem{ID: "candidate-call", Kind: types.EvidenceConcrete, Scope: types.ScopeLine,
			Source: "src/factory.ts", LineStart: 12, LineEnd: 12,
			Subject: "Factory.build", AnchorSymbol: "Factory.build", AnchorKind: types.AnchorCall,
			Predicate: "binds", Object: "Handler", GroundingStatus: status, DerivationCandidate: true}
		plain := item
		plain.ID, plain.DerivationCandidate = "proved-call", false
		carriers := types.ToolHandoffCarriersFromTurnAInputs(nil, []types.EvidenceItem{item, plain}, nil)
		before, _ := json.Marshal(carriers)
		projected := answerDocToolHandoffCarriersForFinalizer(nil, carriers)
		out := renderTypedToolHandoffCarriers("", projected)
		for _, want := range []string{"evidence=`candidate-call`", "evidence=`proved-call`", "@ `src/factory.ts:12`", "anchor=`Factory.build`", "claim_form=`call_edge`", "grounding=`" + string(status) + "`", types.EvidenceDerivationBoundary(item)} {
			if !strings.Contains(out, want) {
				t.Errorf("handoff lost %q:\n%s", want, out)
			}
		}
		if strings.Count(out, types.EvidenceDerivationBoundary(item)) != 1 {
			t.Errorf("limitation leaked onto independent evidence:\n%s", out)
		}
		after, _ := json.Marshal(carriers)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("renderer rewrote durable carrier")
		}
		if again := renderTypedToolHandoffCarriers("", projected); again != out {
			t.Fatal("renderer not idempotent")
		}
	}
}

func TestB1627ModelCannotWriteDerivationCandidateMarker(t *testing.T) {
	emit := &tool.EmitEvidence{}
	if strings.Contains(string(emit.Parameters()), "derivation_candidate") {
		t.Fatal("system marker exposed in model schema")
	}
	for _, value := range []string{"true", "false"} {
		bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("inspect")}
		result, err := emit.Execute(bus, json.RawMessage(`{"items":[{"kind":"concrete","scope":"line","source":"a.go","line_start":1,"summary":"a source clue","derivation_candidate":`+value+`}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if result.Success || len(bus.Mutable.EmittedEvidence()) != 0 || !strings.Contains(result.Summary, "derivation_candidate") {
			t.Fatalf("model marker was accepted or silently stripped: %+v", result)
		}
	}
}
