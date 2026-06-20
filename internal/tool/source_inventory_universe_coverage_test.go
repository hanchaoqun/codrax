package tool

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestSourceInventoryObservationFromLensDirectChildren_MixedLanguageAndConfigRoles(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"app/routes", "app/components"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{
		"app/package.json",
		"app/config.toml",
		"app/main.ts",
		"app/view.ets",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{RepoRoot: root, Mutable: types.NewMutableState("source inventory")}
	obs := sourceInventoryObservationFromLensDirectChildren(ctx, types.SourceInventoryLensQuery{
		Path: "app",
		Roles: []types.AnswerCandidateRole{
			types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleFile,
			types.AnswerCandidateRoleConfigFile,
		},
	})
	counts := map[types.AnswerCandidateRole]int{}
	for _, set := range obs.Sets {
		counts[set.Role] = set.Count
	}
	if counts[types.AnswerCandidateRolePackage] != 2 ||
		counts[types.AnswerCandidateRoleConfigFile] != 2 ||
		counts[types.AnswerCandidateRoleFile] != 2 {
		t.Fatalf("mixed direct children should classify by structural role, not language/case: counts=%+v obs=%+v", counts, obs)
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

func TestPublishSourceInventoryObservationFromLens_PublishesSourceClassUniverseWithoutCandidates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.py", "def run():\n    pass\n")
	writeTestFile(t, root, "internal/thirdparty/tree-sitter-arkts/corpus/sources/entry.ets", "@Entry\n@Component\nstruct Index { build() {} }\n")
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/main.py",
		Language: repotypes.LangPython,
		Package:  "src",
	}})
	ctx := sourceInventoryTestContext(root, graph, ".", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		SourceQuotes:      []string{"@Entry", "@Component"},
		RequestedFields: []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
		},
		Confidence: 0.90,
	})

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		Query:         "@Entry @Component",
	})
	if !obs.IsActive() || len(obs.Sets) != 0 {
		t.Fatalf("class-only source inventory observation should be active without candidates: %+v", obs)
	}
	counts := map[types.SourcePathRole]int{}
	for _, class := range obs.SourceClasses {
		counts[class.Role] = class.Count
	}
	if counts[types.SourcePathRoleThirdParty] != 1 || counts[types.SourcePathRoleProduction] != 1 {
		t.Fatalf("source class universe should include repo-owned thirdparty and production files: counts=%+v obs=%+v", counts, obs)
	}
	rendered := RenderSourceInventoryObservationView(obs, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
	})
	for _, want := range []string{"source_classes:", "thirdparty:1", "No candidate member rows matched"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered class universe missing %q:\n%s", want, rendered)
		}
	}
}

func TestPublishSourceInventoryObservationFromLens_BudgetsBroadCandidateMaterialization(t *testing.T) {
	files := make([]*repotypes.FileInfo, 0, sourceInventoryCandidateBudgetFileThreshold+100)
	for i := 0; i < sourceInventoryCandidateBudgetFileThreshold+100; i++ {
		rel := "src/pkg" + strconv.Itoa(i) + "/file" + strconv.Itoa(i) + ".ts"
		files = append(files, &repotypes.FileInfo{
			RelPath:  rel,
			Language: "typescript",
			Package:  "pkg" + strconv.Itoa(i),
			Symbols: []repotypes.Symbol{{
				Name:     "run" + strconv.Itoa(i),
				Kind:     "function",
				File:     rel,
				Line:     10,
				Exported: true,
			}},
		})
	}
	graph := testGraphWithFiles(files)
	ctx := sourceInventoryTestContext("", graph, ".", nil)

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		TopN:          50,
	})
	if !obs.IsActive() || len(obs.Sets) != 1 {
		t.Fatalf("broad source_inventory should still return a bounded active observation: %+v", obs)
	}
	wantLimit := 50 * sourceInventoryCandidateBudgetMultiplier
	if obs.Sets[0].Count != wantLimit || len(obs.Sets[0].Members) != wantLimit {
		t.Fatalf("broad lens should materialize exactly the per-role budget %d, got %+v", wantLimit, obs.Sets[0])
	}
	if obs.Complete || obs.Sets[0].Complete {
		t.Fatalf("budget-truncated observation must not be marked complete: %+v", obs)
	}
	if !sourceInventoryStringSliceContains(obs.Provenance, "repo_lens:candidate_budget_truncated") {
		t.Fatalf("budget-truncated observation should carry typed provenance: %+v", obs.Provenance)
	}
	rendered := RenderSourceInventoryObservationView(obs, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		TopN:          10,
	})
	for _, want := range []string{
		"candidate materialization was budget-truncated",
		"bounded navigation sample",
		"rerun a narrower source_inventory lens before exhaustive claims",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered budgeted lens missing %q:\n%s", want, rendered)
		}
	}
}

func TestPublishSourceInventoryObservationFromLens_BudgetsBroadNoMatchScan(t *testing.T) {
	files := make([]*repotypes.FileInfo, 0, sourceInventoryCandidateScanBudgetMaxPerRole+100)
	for i := 0; i < sourceInventoryCandidateScanBudgetMaxPerRole+100; i++ {
		rel := "src/pkg" + strconv.Itoa(i) + "/file" + strconv.Itoa(i) + ".ts"
		files = append(files, &repotypes.FileInfo{
			RelPath:  rel,
			Language: "typescript",
			Package:  "pkg" + strconv.Itoa(i),
			Symbols: []repotypes.Symbol{{
				Name:     "run" + strconv.Itoa(i),
				Kind:     "function",
				File:     rel,
				Line:     10,
				Exported: true,
			}},
		})
	}
	graph := testGraphWithFiles(files)
	ctx := sourceInventoryTestContext("", graph, ".", nil)

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		TopN:          10,
		Query:         "definitely_absent_symbol",
	})
	if !obs.IsActive() || len(obs.Sets) != 1 {
		t.Fatalf("broad no-match scan should preserve an active incomplete observation: %+v", obs)
	}
	if obs.Complete || obs.Sets[0].Complete {
		t.Fatalf("broad no-match scan must be incomplete, got %+v", obs)
	}
	if obs.Sets[0].Count != 0 || len(obs.Sets[0].Members) != 0 {
		t.Fatalf("no-match scan should not invent members, got %+v", obs.Sets[0])
	}
	if !sourceInventoryStringSliceContains(obs.Provenance, "repo_lens:candidate_budget_truncated") {
		t.Fatalf("scan-budgeted observation should carry typed truncation provenance: %+v", obs.Provenance)
	}
	rendered := RenderSourceInventoryObservationView(obs, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		TopN:          10,
	})
	for _, want := range []string{
		"candidate materialization was budget-truncated",
		"bounded navigation sample",
		"before claiming exhaustive coverage",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered scan-budgeted lens missing %q:\n%s", want, rendered)
		}
	}
}

func TestSourceInventoryLensQueryScopes_PathRelativeAliasMatrix(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "aggregator/aggregator.go", Language: "go", Package: "aggregator"},
		{RelPath: "subject/taxonomy.go", Language: "go", Package: "subject"},
	})
	ctx := sourceInventoryTestContext("", graph, "internal/analysis", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		Confidence:        0.95,
	})
	for _, tc := range []struct {
		name   string
		path   string
		scopes []string
		want   []string
	}{
		{name: "path only scoped root", path: "internal/analysis", want: []string{"."}},
		{name: "explicit dot", path: "internal/analysis", scopes: []string{"."}, want: []string{"."}},
		{name: "path identical", path: "internal/analysis", scopes: []string{"internal/analysis"}, want: []string{"."}},
		{name: "child relative", path: "internal/analysis", scopes: []string{"aggregator"}, want: []string{"aggregator"}},
		{name: "child full", path: "internal/analysis", scopes: []string{"internal/analysis/aggregator"}, want: []string{"aggregator"}},
		{name: "duplicate aliases", path: "internal/analysis", scopes: []string{"aggregator", "internal/analysis/aggregator"}, want: []string{"aggregator"}},
	} {
		got := sourceInventoryLensQueryScopes(ctx, graph, types.SourceInventoryLensQuery{
			Path:   tc.path,
			Scopes: tc.scopes,
			Roles:  []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		})
		if !sameStringSlice(got, tc.want) {
			t.Fatalf("%s: scopes=%+v want=%+v", tc.name, got, tc.want)
		}
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

func TestPublishSourceInventoryObservationFromLens_ClassOnlyRenderExcludesPriorRows(t *testing.T) {
	root := t.TempDir()
	for rel, body := range map[string]string{
		"internal/analysis/aggregator/aggregator.go": "package aggregator\nfunc Aggregate() {}\n",
		"internal/analysis/subject/taxonomy.go":      "package subject\nfunc Score() {}\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "internal/analysis/aggregator/aggregator.go",
			Language: "go",
			Package:  "aggregator",
			Symbols: []repotypes.Symbol{{
				Name:     "Aggregate",
				Kind:     "function",
				File:     "internal/analysis/aggregator/aggregator.go",
				Line:     12,
				Exported: true,
			}},
		},
	})
	ctx := sourceInventoryTestContext(root, graph, "internal/analysis", nil)
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
		Query:         "definitely-no-current-lens-row",
		IncludeCounts: true,
	})
	if !renderObs.IsActive() || len(renderObs.SourceClasses) == 0 {
		t.Fatalf("class-only current lens should remain active via source classes: %+v", renderObs)
	}
	if len(renderObs.Sets) != 0 {
		t.Fatalf("class-only visible lens should not inherit prior direct-child rows: %+v", renderObs)
	}
	stored := ctx.Mutable.SourceInventoryObservation()
	if !stored.IsActive() || len(sourceInventoryExactUniverseSets(stored)) == 0 {
		t.Fatalf("prior exact universe should remain stored for coverage checks: %+v", stored)
	}
}

func TestPublishSourceInventoryObservationFromLens_RenderIsCurrentQueryOnly(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "aggregator/aggregator.go",
			Language: "go",
			Package:  "aggregator",
			Symbols: []repotypes.Symbol{
				{Name: "Aggregator", Kind: "type", File: "aggregator/aggregator.go", Line: 10, Exported: true},
				{Name: "Aggregate", Kind: "function", File: "aggregator/aggregator.go", Line: 20, Exported: true},
			},
		},
	})
	ctx := sourceInventoryTestContext("", graph, "internal/analysis", nil)
	first := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:  "internal/analysis",
		Roles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
	})
	if !first.IsActive() || len(first.Sets) != 1 || first.Sets[0].Role != types.AnswerCandidateRoleFunction {
		t.Fatalf("first function lens unexpected: %+v", first)
	}
	second := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:  "internal/analysis",
		Roles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
	})
	if !second.IsActive() || len(second.Sets) != 1 || second.Sets[0].Role != types.AnswerCandidateRoleType {
		t.Fatalf("visible lens should not accumulate prior role sets: %+v", second)
	}
}

func TestSourceInventoryGraphSymbolIndexDedupeAndFileLookup(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "kernel/sched/core.c",
			Language: "c",
			Symbols: []repotypes.Symbol{
				{Name: "schedule", Kind: "function", File: "kernel/sched/core.c", Line: 42, Exported: true},
			},
		},
		{
			RelPath:  "mm/memory.c",
			Language: "c",
			Symbols: []repotypes.Symbol{
				{Name: "handle_mm_fault", Kind: "function", File: "mm/memory.c", Line: 77, Exported: true},
			},
		},
	})
	graph.SymbolByID = map[repotypes.SymbolID]*repotypes.Symbol{}
	for _, defs := range graph.SymbolDefs {
		for _, sym := range defs {
			graph.SymbolByID[repotypes.SymbolID(sym.File+"::"+sym.Name)] = sym
		}
	}

	index := newSourceInventoryGraphSymbolIndex(graph)
	if index == nil || len(index.all) != 2 {
		t.Fatalf("index should dedupe SymbolByID and SymbolDefs pointers, got %+v", index)
	}
	kernelSyms := index.symbolsForFile("./kernel/sched/core.c")
	if len(kernelSyms) != 1 || kernelSyms[0].Name != "schedule" {
		t.Fatalf("file lookup should normalize and return only local symbols, got %+v", kernelSyms)
	}
	kernelDirSyms := index.symbolsForDir("kernel/sched")
	if len(kernelDirSyms) != 1 || kernelDirSyms[0].Name != "schedule" {
		t.Fatalf("dir lookup should return subtree-local symbols, got %+v", kernelDirSyms)
	}
	mmSyms := index.symbolsForFile("kernel/../mm/memory.c")
	if len(mmSyms) != 0 {
		t.Fatalf("file lookup should not path-clean traversal aliases into another repo path, got %+v", mmSyms)
	}
}

func TestPublishSourceInventoryObservationFromLens_DefersBroadAttributesWithNarrowingHint(t *testing.T) {
	files := make([]*repotypes.FileInfo, 0, sourceInventoryBroadToolLensAttributeFileThreshold+2)
	for i := 0; i < sourceInventoryBroadToolLensAttributeFileThreshold+2; i++ {
		rel := "kernel/file" + strconv.Itoa(i) + ".c"
		files = append(files, &repotypes.FileInfo{
			RelPath:  rel,
			Language: "c",
			Symbols: []repotypes.Symbol{{
				Name:     "func_" + strconv.Itoa(i),
				Kind:     "function",
				File:     rel,
				Line:     10,
				Exported: true,
			}},
		})
	}
	graph := testGraphWithFiles(files)
	ctx := sourceInventoryTestContext("", graph, ".", nil)

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:              ".",
		Scopes:            []string{"kernel"},
		Roles:             []types.AnswerCandidateRole{types.AnswerCandidateRoleFile},
		AttributeRoles:    []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeAttributes: true,
		IncludeCounts:     true,
	})
	if !obs.IsActive() || !sourceInventoryStringSliceContains(obs.Provenance, "repo_lens:attributes_deferred_broad_scope") {
		t.Fatalf("broad attribute lens should stay active and carry deferred provenance: %+v", obs)
	}
	if len(obs.Sets) == 0 || len(obs.Sets[0].Members) == 0 {
		t.Fatalf("broad deferred lens should still return member/count rows: %+v", obs)
	}
	for _, member := range obs.Sets[0].Members {
		if len(member.Attributes) != 0 {
			t.Fatalf("broad deferred lens must not expand row-local attributes: %+v", member)
		}
	}
	rendered := RenderSourceInventoryObservationView(obs, types.SourceInventoryLensQuery{
		Path:              ".",
		Scopes:            []string{"kernel"},
		Roles:             []types.AnswerCandidateRole{types.AnswerCandidateRoleFile},
		AttributeRoles:    []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeAttributes: true,
		IncludeCounts:     true,
	})
	for _, want := range []string{
		"row-local attributes were deferred",
		"choose a narrower `scope` or selected member",
		`include_attributes=true`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("deferred broad lens render missing %q:\n%s", want, rendered)
		}
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

func TestSourceInventoryCandidateUniverseCoverageGap_IgnoresAdvisoryRowsWithoutExactProvenance(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"repomap_graph"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    3,
			Members: []types.SourceInventoryObservationMember{
				{Name: "alpha", Key: "src/alpha", Role: types.AnswerCandidateRolePackage},
				{Name: "beta", Key: "src/beta", Role: types.AnswerCandidateRolePackage},
				{Name: "gamma", Key: "src/gamma", Role: types.AnswerCandidateRolePackage},
			},
		}},
	})
	gap := SourceInventoryCandidateUniverseCoverageGap(&types.BusContext{Mutable: mut}, []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Members: []string{"alpha", "beta"},
	}})
	if gap.IsActive() {
		t.Fatalf("advisory graph rows without exact provenance must not hard-block: %+v", gap)
	}
}

func TestSourceInventoryLensExecutionGap_AdvisoryAndListFilesDoNotSatisfyLens(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"source_inventory_profile", "repomap_graph", "pre_explore_typed_request"},
		Sets: []types.SourceInventoryAdvisorySet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Candidates: []types.SourceInventoryAdvisoryCandidate{{
				Member:     "run",
				Key:        "src/app.py::run",
				SupportRef: "run: src/app.py:7",
				Role:       types.AnswerCandidateRoleFunction,
				File:       "src/app.py",
				Line:       7,
				Language:   "python",
			}},
		}},
	})
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{sourceInventoryExactUniverseProvenanceListFilesDirect},
		Lens:         []string{"direct_children", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFile,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "app.py",
				Key:        "src/app.py",
				SupportRef: "src/app.py",
				Provenance: []string{sourceInventoryExactUniverseProvenanceListFilesDirect},
				Role:       types.AnswerCandidateRoleFile,
				File:       "src/app.py",
			}},
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
				Confidence: 0.9,
			},
		}},
	}
	gap := SourceInventoryLensExecutionGapForContext(ctx)
	if !gap.Blocking || !gap.HasAdvisory || !gap.HasListFiles {
		t.Fatalf("advisory/list_files state should still require source_inventory lens execution, got %+v", gap)
	}
	if len(gap.Roles) != 1 || gap.Roles[0] != types.AnswerCandidateRoleFunction {
		t.Fatalf("gap should preserve typed profile roles, got %+v", gap.Roles)
	}
}

func TestSourceInventoryLensExecutionGap_TypedQueryAdvisoryRequiresLens(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance: []string{
			"request_traits:typed_source_enumeration_query",
			"request_traits:query_root_scope",
			"pre_explore_typed_request",
			"repomap_graph",
		},
		Sets: []types.SourceInventoryAdvisorySet{
			{
				Role:     types.AnswerCandidateRoleType,
				Complete: true,
				Candidates: []types.SourceInventoryAdvisoryCandidate{{
					Member:     "Index",
					Key:        "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets::Index",
					SupportRef: "Index: internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7",
					Role:       types.AnswerCandidateRoleType,
					File:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
					Line:       7,
					Language:   "arkts",
				}},
			},
			{
				Role:     types.AnswerCandidateRoleFunction,
				Complete: true,
				Candidates: []types.SourceInventoryAdvisoryCandidate{{
					Member:     "GlobalCard",
					Key:        "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets::GlobalCard",
					SupportRef: "GlobalCard: internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets:26",
					Role:       types.AnswerCandidateRoleFunction,
					File:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
					Line:       26,
					Language:   "arkts",
				}},
			},
		},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}

	gap := SourceInventoryLensExecutionGapForContext(ctx)
	if !gap.Blocking || !gap.HasAdvisory || gap.HasListFiles {
		t.Fatalf("typed query advisory should require executable source_inventory lens, got %+v", gap)
	}
	if !sameAnswerRoles(gap.Roles, []types.AnswerCandidateRole{
		types.AnswerCandidateRoleType,
		types.AnswerCandidateRoleFunction,
	}) {
		t.Fatalf("gap roles = %+v, want advisory roles", gap.Roles)
	}
	if len(gap.Scopes) != 1 || gap.Scopes[0] != "." {
		t.Fatalf("gap scopes = %+v, want root query scope", gap.Scopes)
	}

	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance:   []string{"repo_lens:tool_query"},
		Lens:         []string{"members", "symbols", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleType,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "Index",
				Key:        "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets::Index",
				SupportRef: "Index: internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7",
				Provenance: []string{"repo_lens:tool_query"},
				Role:       types.AnswerCandidateRoleType,
				File:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
				Line:       7,
			}},
		}},
	})
	if satisfied := SourceInventoryLensExecutionGapForContext(ctx); satisfied.Blocking {
		t.Fatalf("repo_lens tool observation should satisfy synthetic query lane, got %+v", satisfied)
	}
}

func TestSourceInventoryLensExecutionGap_SatisfiedByRepoLensAdvisoryProvenance(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance: []string{
			"request_traits:typed_source_enumeration_query",
			"repo_lens:tool_query",
			"repomap_graph",
		},
		Sets: []types.SourceInventoryAdvisorySet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Candidates: []types.SourceInventoryAdvisoryCandidate{{
				Member:     "GlobalCard",
				Key:        "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets::GlobalCard",
				SupportRef: "GlobalCard: internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets:26",
				Role:       types.AnswerCandidateRoleFunction,
				File:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line:       26,
				Language:   "arkts",
			}},
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("repo_lens advisory provenance should satisfy executable lens gate, got %+v", gap)
	}
}

func TestSourceInventoryLensExecutionGap_SatisfiedByRepoLensObservation(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"repo_lens:tool_query"},
		Lens:         []string{"members", "symbols", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "run",
				Key:           "src/app.py::run",
				SupportRef:    "run: src/app.py:7",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "src/app.py",
				Line:          7,
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
		}},
	}
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("repo_lens:tool_query source-inventory observation should satisfy execution gate, got %+v", gap)
	}
}

func TestSourceInventoryLensExecutionGap_SatisfiedByZeroResultRepoLensToolOutput(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary:  "Repo Lens: no source-inventory observation is available for the requested typed scope/role slice.",
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
		}},
	}
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("zero-result source_inventory lens tool output should count as executed, got %+v", gap)
	}
}

func TestSourceInventorySourceScope_TypedQueryRootScopeIncludesAuxiliarySources(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Scopes:       []string{"."},
		Provenance: []string{
			"request_traits:typed_source_enumeration_query",
			"request_traits:query_root_scope",
			"pre_explore_typed_request",
		},
		Sets: []types.SourceInventoryAdvisorySet{{
			Role:     types.AnswerCandidateRoleType,
			Complete: true,
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
		}},
	}
	if !sourceInventorySourceInRequestedScope(ctx, "fixtures/arkts/entry.ets") {
		t.Fatal("typed repository-wide source inventory query should include auxiliary source classes")
	}
}

func TestSourceInventorySourceScope_ProductionInventoryWithoutExplicitExclusionIncludesAuxiliarySources(t *testing.T) {
	ctx := &types.BusContext{
		Mutable: types.NewMutableState("source inventory"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				SourceQuotes:      []string{"decorated components"},
				Confidence:        0.9,
			},
		}},
	}
	if !sourceInventorySourceInRequestedScope(ctx, "fixtures/arkts/entry.ets") {
		t.Fatal("production source scope without explicit auxiliary exclusion should not hard-filter repo-owned auxiliary sources in source_inventory")
	}
}

func TestSourceInventorySourceScope_ExplicitProductionInventoryStillFiltersAuxiliarySources(t *testing.T) {
	ctx := &types.BusContext{
		Mutable: types.NewMutableState("source inventory"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleFixture,
					types.AnswerCandidateRoleExample,
				},
				SourceQuotes: []string{"fixture", "example"},
				Confidence:   0.9,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.9,
			},
		}},
	}
	if sourceInventorySourceInRequestedScope(ctx, "fixtures/arkts/entry.ets") {
		t.Fatal("explicit production source inventory should keep auxiliary sources out of principal candidates")
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

func TestSourceInventoryAcceptedClosureCoversExactUniverse_RelationMemberSet(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "package -> entrypoint",
		Value:   "3",
		Members: []string{"alpha -> Start", "beta -> Build", "gamma -> Run"},
	}}
	if !SourceInventoryAcceptedClosureCoversExactUniverse(ctx, facts) {
		t.Fatalf("relation member_set should prove the exact principal universe is covered")
	}
}

func TestSourceInventoryAcceptedClosureCoversExactUniverse_RequiresFullCoverage(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	partial := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "package -> entrypoint",
		Value:   "2",
		Members: []string{"alpha -> Start", "beta -> Build"},
	}}
	if SourceInventoryAcceptedClosureCoversExactUniverse(ctx, partial) {
		t.Fatalf("partial model member_set must not prove exact-universe closure")
	}

	withExclusion := []types.AnswerAggregateFact{{
		Kind:     types.AnswerAggregateMemberSet,
		Label:    "package -> entrypoint",
		Value:    "2",
		Members:  []string{"alpha -> Start", "beta -> Build"},
		Excluded: []string{"gamma"},
	}}
	if !SourceInventoryAcceptedClosureCoversExactUniverse(ctx, withExclusion) {
		t.Fatalf("model-authored exclusion should satisfy the exact-universe boundary")
	}
}

func TestSourceInventoryAcceptedClosureCoversExactUniverse_RejectsExclusionOnly(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta"})
	facts := []types.AnswerAggregateFact{{
		Kind:     types.AnswerAggregateExcluded,
		Label:    "not answer members",
		Value:    "2",
		Excluded: []string{"alpha", "beta"},
	}}
	if SourceInventoryAcceptedClosureCoversExactUniverse(ctx, facts) {
		t.Fatalf("exclusion-only boundary must not masquerade as accepted answer closure")
	}
}

func sameStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameAnswerRoles(got, want []types.AnswerCandidateRole) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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
