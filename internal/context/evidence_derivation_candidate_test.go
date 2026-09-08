package context

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1627DossierSeparatesCandidateFromIndependentEvidence(t *testing.T) {
	candidate := types.EvidenceItem{
		ID: "candidate", Kind: types.EvidenceConcrete, Scope: types.ScopeLine,
		Subject: "Registrar", Predicate: "binds ONLY", Object: "Widget",
		Source: "pkg/wiring.rb", LineStart: 17, LineEnd: 17,
		AnchorKind: types.AnchorCall, AnchorSymbol: "register",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		DerivationCandidate: true,
	}
	exact := candidate
	exact.ID, exact.DerivationCandidate = "independent", false
	for _, items := range [][]types.EvidenceItem{{candidate}, {candidate, exact}, {exact, candidate}} {
		before := append([]types.EvidenceItem(nil), items...)
		got := formatRelationDossierEvidence(items)
		if !strings.Contains(got, "candidate") || !strings.Contains(got, "pkg/wiring.rb:17") || !strings.Contains(got, "Registrar -> Widget") {
			t.Errorf("candidate navigation/position missing: %s", got)
		}
		verified := strings.Count(got, "verified Registrar -> Widget")
		want := 0
		if len(items) == 2 {
			want = 1
		}
		if verified != want {
			t.Errorf("candidate and exact record merged or promoted: verified=%d want=%d\n%s", verified, want, got)
		}
		if !reflect.DeepEqual(items, before) {
			t.Fatal("dossier mutated evidence")
		}
	}
	pool := formatEvidenceItems([]types.EvidenceItem{candidate}, 10, true)
	if !strings.Contains(pool, types.EvidenceDerivationBoundary(candidate)) || !strings.Contains(pool, "pkg/wiring.rb:17") {
		t.Fatalf("standalone evidence pool hid candidate boundary or source: %s", pool)
	}
}

func TestB1627DossierCandidateGuardRemainsInspectable(t *testing.T) {
	item := types.EvidenceItem{
		Kind: types.EvidenceConditional, Scope: types.ScopeLine,
		Subject: "enabled", Predicate: "guards", Object: "run",
		Condition: "enabled", Source: "pkg/worker.py", LineStart: 3,
		AnchorKind: types.AnchorCondition, Snippet: "if enabled:",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		DerivationCandidate: true,
	}
	got := formatRelationDossierEvidence([]types.EvidenceItem{item})
	if strings.Contains(got, "verified guard") || !strings.Contains(got, "candidate") || !strings.Contains(got, "pkg/worker.py:3") {
		t.Fatalf("guard candidate was hidden/promoted: %s", got)
	}
}

func TestB1627DossierAndPoolKeepCandidateBoundaryThroughPromptBuild(t *testing.T) {
	candidate := types.EvidenceItem{
		ID: "candidate", Kind: types.EvidenceConcrete, Scope: types.ScopeLine,
		Subject: "Registrar", Predicate: "binds", Object: "Widget",
		Source: "pkg/wiring.rb", LineStart: 17, LineEnd: 17,
		AnchorKind: types.AnchorCall, AnchorSymbol: "register",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		DerivationCandidate: true,
	}
	exact := candidate
	exact.ID, exact.DerivationCandidate = "independent", false
	bus := &types.BusContext{
		Mutable:       types.NewMutableState("inspect registration"),
		EvidenceItems: []types.EvidenceItem{candidate, exact},
	}
	before := append([]types.EvidenceItem(nil), bus.EvidenceItems...)
	for _, lane := range []struct {
		name      types.AgentName
		stage     types.PipelineStage
		skillName string
	}{
		{types.AgentExplorer, types.StageExplore, "explore-skill"},
		{types.AgentFinalizer, types.StageFinalize, "finalize-skill"},
	} {
		t.Run(string(lane.name), func(t *testing.T) {
			ac := BuildAgentContext(bus, lane.name, lane.stage)
			pc := BuildPromptContext(ac, &skill.Config{Name: lane.skillName})
			for _, title := range []string{SectionEvidencePool, SectionRelationDossier} {
				section := findSectionTitle(pc, title)
				if section == nil || !strings.Contains(section.Content, types.EvidenceDerivationBoundary(candidate)) || !strings.Contains(section.Content, "pkg/wiring.rb:17") {
					t.Fatalf("%s lost the candidate source/boundary: %+v", title, section)
				}
			}
			pool := findSectionTitle(pc, SectionEvidencePool)
			if !strings.Contains(pool.Content, "Each row's derivation and grounding boundaries take precedence") {
				t.Fatalf("pool lane teaching contradicts candidate rows: %s", pool.Content)
			}
			dossier := findSectionTitle(pc, SectionRelationDossier)
			if strings.Count(dossier.Content, "verified Registrar -> Widget") != 1 {
				t.Fatalf("independent exact evidence hidden or candidate promoted: %s", dossier.Content)
			}
			if !reflect.DeepEqual(ac.EvidenceItems, before) || !reflect.DeepEqual(bus.EvidenceItems, before) {
				t.Fatal("prompt construction changed evidence facts")
			}
		})
	}
}
