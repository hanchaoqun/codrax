package tool

import (
	"os"
	"path/filepath"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPublishSourceInventoryObservationFromToolObservation_ListFilesDirectUniverse(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"src/alpha", "src/beta"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"src/app.yaml", "src/main.py"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mut := types.NewMutableState("source inventory")
	ctx := &types.BusContext{RepoRoot: root, Mutable: mut}
	ok := PublishSourceInventoryObservationFromToolObservation(ctx, types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary:  "[list_files: path=src recursive=false]\nsrc/alpha\nsrc/beta\nsrc/app.yaml\nsrc/main.py\n",
	})
	if !ok {
		t.Fatal("direct list_files should publish an exact source-inventory observation")
	}
	obs := mut.SourceInventoryObservation()
	if !obs.IsActive() || !obs.Complete {
		t.Fatalf("observation not active/complete: %+v", obs)
	}
	counts := map[types.AnswerCandidateRole]int{}
	for _, set := range obs.Sets {
		counts[set.Role] = set.Count
		for _, member := range set.Members {
			if !containsString(member.Provenance, sourceInventoryExactUniverseProvenanceListFilesDirect) {
				t.Fatalf("member missing exact provenance: %+v", member)
			}
		}
	}
	if counts[types.AnswerCandidateRolePackage] != 2 ||
		counts[types.AnswerCandidateRoleConfigFile] != 1 ||
		counts[types.AnswerCandidateRoleFile] != 1 {
		t.Fatalf("unexpected role counts: %+v in %+v", counts, obs.Sets)
	}
}

func TestSourceInventoryObservationFromLensDirectChildren_ExactUniverseForExplicitRoles(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"internal/analysis/aggregator", "internal/analysis/subject"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"internal/analysis/config.yaml", "internal/analysis/notes.md"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{RepoRoot: root, Mutable: types.NewMutableState("source inventory")}
	obs := sourceInventoryObservationFromLensDirectChildren(ctx, types.SourceInventoryLensQuery{
		Path:   "internal",
		Scopes: []string{"analysis"},
		Roles: []types.AnswerCandidateRole{
			types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleConfigFile,
		},
	})
	if !obs.IsActive() || !obs.Complete {
		t.Fatalf("observation not active/complete: %+v", obs)
	}
	counts := map[types.AnswerCandidateRole]int{}
	for _, set := range obs.Sets {
		counts[set.Role] = set.Count
		for _, member := range set.Members {
			if !containsString(member.Provenance, sourceInventoryExactUniverseProvenanceRepoLensDirectChildren) {
				t.Fatalf("member missing repo-lens exact provenance: %+v", member)
			}
			if member.Role == types.AnswerCandidateRoleFile {
				t.Fatalf("explicit roles should not publish plain file rows: %+v", member)
			}
		}
	}
	if counts[types.AnswerCandidateRolePackage] != 2 || counts[types.AnswerCandidateRoleConfigFile] != 1 {
		t.Fatalf("unexpected role counts: %+v in %+v", counts, obs.Sets)
	}
	universes := sourceInventoryExactUniverseSets(obs)
	if len(universes) != 2 {
		t.Fatalf("repo-lens direct provenance should opt into exact universes, got %+v", universes)
	}
	for _, universe := range universes {
		if universe.scope != "internal/analysis" {
			t.Fatalf("scope should remain repo-relative and query-driven, got %+v", universe)
		}
	}
	if scopes := sourceInventoryLensDirectChildScopes(types.SourceInventoryLensQuery{
		Path:   "internal/analysis",
		Scopes: []string{"../outside"},
	}); len(scopes) != 0 {
		t.Fatalf("parent traversal scope must not be normalized into a sibling scan: %+v", scopes)
	}
}

func TestSourceInventoryLensQueryScopes_PathRelativeScopedRoot(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "aggregator/aggregator.go", Language: "go", Package: "aggregator"},
		{RelPath: "subject/taxonomy.go", Language: "go", Package: "subject"},
	})
	ctx := sourceInventoryTestContext("", graph, "internal/analysis", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		Confidence:        0.95,
	})
	scopes := sourceInventoryLensQueryScopes(ctx, graph, types.SourceInventoryLensQuery{
		Path:   "internal/analysis",
		Scopes: []string{"internal/analysis"},
		Roles:  []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
	})
	if len(scopes) != 1 || scopes[0] != "." {
		t.Fatalf("path-identical scope should resolve to scoped graph root, got %+v", scopes)
	}
	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:          "internal/analysis",
		Scopes:        []string{"internal/analysis"},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		IncludeCounts: true,
	})
	if !obs.IsActive() || len(obs.Sets) != 1 || obs.Sets[0].Count != 2 {
		t.Fatalf("path-identical scope should not duplicate root and child scopes: %+v", obs)
	}
}

func TestPublishSourceInventoryObservationFromLens_RenderExcludesPriorExactUniverse(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "aggregator/aggregator.go",
			Language: "go",
			Package:  "aggregator",
			Symbols: []repotypes.Symbol{{
				Name:     "Aggregate",
				Kind:     "function",
				File:     "aggregator/aggregator.go",
				Line:     132,
				Exported: true,
			}},
		},
		{
			RelPath:  "subject/taxonomy.go",
			Language: "go",
			Package:  "subject",
			Symbols: []repotypes.Symbol{{
				Name:     "Score",
				Kind:     "function",
				File:     "subject/taxonomy.go",
				Line:     41,
				Exported: true,
			}},
		},
	})
	ctx := sourceInventoryTestContext("", graph, "internal/analysis", nil)
	ctx.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"internal/analysis"},
		Provenance:   []string{sourceInventoryExactUniverseProvenanceListFilesDirect},
		Lens:         []string{"direct_children", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    2,
			Members: []types.SourceInventoryObservationMember{
				{Name: "aggregator", Key: "internal/analysis/aggregator", File: "internal/analysis/aggregator", Role: types.AnswerCandidateRolePackage, Provenance: []string{sourceInventoryExactUniverseProvenanceListFilesDirect}},
				{Name: "subject", Key: "internal/analysis/subject", File: "internal/analysis/subject", Role: types.AnswerCandidateRolePackage, Provenance: []string{sourceInventoryExactUniverseProvenanceListFilesDirect}},
			},
		}},
	})

	renderObs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:          "internal/analysis",
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
	})
	if !renderObs.IsActive() || len(renderObs.Sets) != 1 || renderObs.Sets[0].Role != types.AnswerCandidateRoleFunction {
		t.Fatalf("visible lens should render only current query roles, got %+v", renderObs)
	}
	if renderObs.Sets[0].Count != 2 {
		t.Fatalf("visible function lens should not include prior package universe: %+v", renderObs.Sets[0])
	}
	stored := ctx.Mutable.SourceInventoryObservation()
	if !stored.IsActive() || len(sourceInventoryExactUniverseSets(stored)) == 0 {
		t.Fatalf("prior exact universe should remain stored for coverage checks: %+v", stored)
	}
}

func TestSourceInventoryCandidateUniverseCoverageGap_BlocksHighAlignmentOnly(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	gap := SourceInventoryCandidateUniverseCoverageGap(ctx, []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "packages",
		Value:   "2",
		Members: []string{"alpha", "beta"},
	}})
	if !gap.Blocking || len(gap.Missing) != 1 || gap.Missing[0].Name != "gamma" {
		t.Fatalf("high-alignment partial member_set should block, got %+v", gap)
	}

	ctx = sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma", "delta"})
	gap = SourceInventoryCandidateUniverseCoverageGap(ctx, []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "packages",
		Value:   "1",
		Members: []string{"alpha"},
	}})
	if gap.Blocking || gap.IsActive() {
		t.Fatalf("single coincidental overlap must not turn a broad universe into a blocking contract, got %+v", gap)
	}
}

func TestSourceInventoryCandidateUniverseCoverageGap_ExplicitExclusionSatisfiesUniverse(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	gap := SourceInventoryCandidateUniverseCoverageGap(ctx, []types.AnswerAggregateFact{{
		Kind:     types.AnswerAggregateMemberSet,
		Label:    "packages",
		Value:    "2",
		Members:  []string{"alpha", "beta"},
		Excluded: []string{"gamma"},
	}})
	if gap.IsActive() {
		t.Fatalf("explicit excluded member should satisfy exact universe contract, got %+v", gap)
	}
}

func sourceInventoryUniverseTestContext(names []string) *types.BusContext {
	members := make([]types.SourceInventoryObservationMember, 0, len(names))
	for _, name := range names {
		members = append(members, types.SourceInventoryObservationMember{
			Name:          name,
			Key:           "src/" + name,
			SupportRef:    "src/" + name,
			Provenance:    []string{sourceInventoryExactUniverseProvenanceListFilesDirect},
			Role:          types.AnswerCandidateRolePackage,
			File:          "src/" + name,
			CoverageState: types.SourceInventoryCoverageObserved,
		})
	}
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{sourceInventoryExactUniverseProvenanceListFilesDirect},
		Lens:         []string{"direct_children", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    len(members),
			Members:  members,
		}},
	})
	return &types.BusContext{Mutable: mut}
}
