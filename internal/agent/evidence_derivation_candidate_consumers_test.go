package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1627ConsumerCandidate() types.EvidenceItem {
	return types.EvidenceItem{
		ID: "candidate-binding", Kind: types.EvidenceConcrete, Scope: types.ScopeLine,
		Subject: "Register", Predicate: "binds ONLY", Object: "Widget",
		Source: "fixture.go", LineStart: 2, LineEnd: 2,
		AnchorKind: types.AnchorCall, AnchorSymbol: "registry.Register",
		Snippet:         "func Register() { registry.Register(Widget) }",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		DerivationCandidate: true,
	}
}

func TestB1627CandidateDoesNotCompleteRequirementOrTerminal(t *testing.T) {
	cases := []struct {
		name        string
		kind        types.EvidenceKind
		predicate   string
		requirement types.RequirementKind
	}{
		{"binding", types.EvidenceConcrete, "binds ONLY", types.ReqRegistration},
		{"registration", types.EvidenceRegistration, "registers", types.ReqRegistration},
		{"return", types.EvidenceConcrete, "returns", types.ReqReturnValue},
		{"call", types.EvidenceRelationship, "calls", types.ReqCallChain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := b1627ConsumerCandidate()
			item.Kind, item.Predicate = tc.kind, tc.predicate
			before := item
			if !item.IsCitable() || item.DisplayLocation(true) == "" {
				t.Fatal("fixture must remain source-locatable")
			}
			reqs := []EvidenceRequirement{{Kind: tc.requirement, Entities: []string{"Register", "Widget"}, Status: "unsatisfied"}}
			got := checkRequirementSatisfaction(reqs, nil, []types.EvidenceItem{item}, types.ComplexityModerate)
			if got[0].Status == "satisfied" {
				t.Errorf("candidate completed factual requirement: %+v", got)
			}
			if hasTerminalEvidence([]types.EvidenceItem{item}) || hasGroundedTerminalEvidence([]types.EvidenceItem{item}) {
				t.Error("source-locatable candidate became a terminal fact")
			}
			if hasGroundedRequirementCarrier([]types.EvidenceItem{item}, reqs) {
				t.Error("candidate became a grounded completion carrier")
			}
			if countEvidenceByKinds([]types.EvidenceItem{item}, nil, tc.kind) != 0 || countEvidenceForRequirement([]types.EvidenceItem{item}, nil, tc.requirement) != 0 {
				t.Error("candidate inflated shared factual counters")
			}
			if !reflect.DeepEqual(item, before) {
				t.Fatal("consumer changed candidate content")
			}
		})
	}
}

func TestB1627CandidateCanBeFollowedByExactModelEvidence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte("package fixture\nfunc Register() {\n registry.Register(Widget)\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: t.TempDir(), Mutable: types.NewMutableState("registration")}
	read, err := (&tool.ReadFile{}).Execute(bus, json.RawMessage(`{"path":"fixture.go"}`))
	if err != nil || !read.Success {
		t.Fatalf("actual source read: %v %+v", err, read)
	}
	bus.Mutable.AppendDispatchToolResult(read)
	emitted, err := (&tool.EmitEvidence{}).Execute(bus, json.RawMessage(`{"items":[{"kind":"registration","scope":"line","subject":"Register","predicate":"registers","object":"Widget","source":"fixture.go","line_start":3,"summary":"Register passes Widget to registry.Register","anchor_kind":"call","anchor_symbol":"registry.Register"}]}`))
	if err != nil || !emitted.Success {
		t.Fatalf("actual explicit evidence emission: %v %+v", err, emitted)
	}
	exact := bus.Mutable.EmittedEvidence()
	if len(exact) != 1 || types.EvidenceIsDerivationCandidate(exact[0]) || !exact[0].IsCitable() || exact[0].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("explicit grounded evidence unavailable: %+v", exact)
	}
	candidate := b1627ConsumerCandidate()
	for _, pool := range [][]types.EvidenceItem{{candidate, exact[0]}, {exact[0], candidate}} {
		req := []EvidenceRequirement{{Kind: types.ReqRegistration, Entities: []string{"Register"}, Status: "unsatisfied"}}
		if got := checkRequirementSatisfaction(req, nil, pool, types.ComplexityModerate); got[0].Status != "satisfied" {
			t.Fatalf("independent exact evidence could not close requirement: %+v", got)
		}
		if !hasGroundedTerminalEvidence(pool) || !hasGroundedRequirementCarrier(pool, req) {
			t.Fatal("exact evidence lost the existing completion escape")
		}
	}
	if !candidate.DerivationCandidate {
		t.Fatal("new exact evidence silently upgraded old candidate")
	}
}

func TestB1627StageReportKeepsCandidateWithoutAnswerAuthority(t *testing.T) {
	item := b1627ConsumerCandidate()
	before := item
	chains := []types.AnswerChain{{Item: item, StrictOK: true, Score: 1}}
	got := renderExplorerStageReport("registration", "call_chain", nil, []types.EvidenceItem{item}, chains, nil, nil, nil, false)
	for _, want := range []string{"## Primary Evidence", "## Resolution Chains", "Register", "Widget", "fixture.go:2", types.EvidenceDerivationBoundary(item)} {
		if !strings.Contains(got, want) {
			t.Errorf("candidate navigation lost %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"do NOT contradict or ignore", "is the ANSWER TERMINAL", "directly answer the question"} {
		if strings.Contains(got, bad) {
			t.Errorf("report promoted a candidate into answer authority: %q", bad)
		}
	}
	if !reflect.DeepEqual(item, before) || !chains[0].StrictOK {
		t.Fatal("display changed evidence or request-predicate classification")
	}
}
