package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1585BothSelectorTeachingEntrypointsRetainRealLookupCorroboration(t *testing.T) {
	base := b1580ProductionSelectorEvidence(t)
	var parser types.EvidenceItem
	for _, item := range base {
		if item.Producer == types.EvidenceProducerRepoMapDynamicSelectorAssignment && item.Source == "pipeline/registry.py" && item.LineStart == 31 {
			parser = item
		}
	}
	if parser.ID == "" || parser.Subject != "cls" || parser.Object != "REGISTRY" || parser.Predicate != "assigns" {
		t.Fatalf("production enrichment no longer matches the real r1026 parser row: %+v", parser)
	}
	model := parser
	model.ID = "ev-60b09ad3717d93d6"
	model.Kind, model.Predicate, model.Producer = types.EvidenceRelationship, "maps", ""
	for _, reverse := range []bool{false, true} {
		all := append(append([]types.EvidenceItem(nil), base...), model)
		if reverse {
			all = append([]types.EvidenceItem{model}, base...)
		}
		ctx := b1580SelectorContext(all)
		capsule := renderAnswerDocDynamicSelectorResolutionCandidates(ctx, "run_pipeline")
		for _, want := range []string{"JsonPlugin", "CsvPlugin", "not proof of the runtime-selected implementation", "REGISTRY", "cls()"} {
			if !strings.Contains(capsule, want) {
				t.Errorf("candidate teaching withheld after corroboration (reverse=%t), missing %q: %s", reverse, want, capsule)
			}
		}
		var recipes strings.Builder
		anchors := renderAnswerDocDynamicSelectorRelationRecipes(&recipes, ctx)
		if len(anchors) == 0 || !strings.Contains(recipes.String(), "candidate-only permissions") {
			t.Fatalf("relation recipe teaching withheld after corroboration (reverse=%t): %s", reverse, recipes.String())
		}
		for _, anchor := range anchors {
			if anchor.RelationKind == types.DiagramRelCall && anchor.ToIdentity == "JsonPlugin" {
				t.Fatalf("corroboration must not mint a resolve/entry -> candidate call: %+v", anchor)
			}
		}
	}
}

func TestB1585BothSelectorTeachingEntrypointsWithholdRealLookupConflict(t *testing.T) {
	base := b1580ProductionSelectorEvidence(t)
	for _, item := range base {
		if item.Producer != types.EvidenceProducerRepoMapDynamicSelectorAssignment || item.Source != "pipeline/registry.py" || item.LineStart != 31 {
			continue
		}
		conflicting := item
		conflicting.ID = "E-other-index-same-occurrence"
		conflicting.Snippet = "cls = REGISTRY[other]"
		ctx := b1580SelectorContext(append(base, conflicting))
		if got := renderAnswerDocDynamicSelectorResolutionCandidates(ctx, "run_pipeline"); got != "" {
			t.Fatalf("conflicting exact source expressions must withhold candidates: %s", got)
		}
		var recipes strings.Builder
		if got := renderAnswerDocDynamicSelectorRelationRecipes(&recipes, ctx); len(got) != 0 || recipes.Len() != 0 {
			t.Fatalf("conflicting exact source expressions must withhold authoring receipts: %+v\n%s", got, recipes.String())
		}
		return
	}
	t.Fatal("real parser lookup not found")
}
