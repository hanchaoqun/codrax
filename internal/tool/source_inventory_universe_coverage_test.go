package tool

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/tool/sourceinventory"
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

func TestSourceInventoryExecutionView_ScopedLanguageAndMembership(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/beta/file.ts", Language: repotypes.LangTypeScript},
		{RelPath: "src/alpha/file.cpp", Language: repotypes.LangC},
		{RelPath: "src/config.yaml", IsSpecial: true},
		{RelPath: "tests/alpha_test.py", Language: repotypes.LangPython},
	})

	view := newSourceInventoryExecutionView(graph, []string{"src"})
	all := view.Files("")
	if len(all) != 3 {
		t.Fatalf("scoped view should keep only src files, got %d: %+v", len(all), all)
	}
	if all[0].RelPath != "src/alpha/file.cpp" || all[1].RelPath != "src/beta/file.ts" || all[2].RelPath != "src/config.yaml" {
		t.Fatalf("scoped view should be sorted once by repo path, got %+v", all)
	}
	if !sourceInventoryViewHasInventoryFiles(view) {
		t.Fatalf("scoped view should record language/config inventory files")
	}
	if !sourceInventoryViewContainsFile(view, "src/alpha/file.cpp", nil) ||
		sourceInventoryViewContainsFile(view, "tests/alpha_test.py", nil) {
		t.Fatalf("view membership should be scoped and exact")
	}
	cppFiles := view.Files(repotypes.LangC)
	if len(cppFiles) != 1 || cppFiles[0].RelPath != "src/alpha/file.cpp" {
		t.Fatalf("language-filtered view should reuse the scoped file set, got %+v", cppFiles)
	}
}

func TestSourceInventoryCandidateBudgetDeadlineTruncatesFileScan(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/main.go",
		Language: repotypes.LangGo,
	}})
	view := newSourceInventoryExecutionView(graph, []string{"."})
	set := sourceInventoryFileCandidates(
		nil,
		view,
		nil,
		newSourceInventoryScopeFilter(nil),
		&types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFile}},
		nil,
		false,
		sourceInventoryQueryFilter{},
		sourceInventoryExecBudget{kernel: sourceinventory.NewBudget(sourceinventory.BudgetOptions{
			ForceAdvisoryOnly: true,
			GraphFileCount:    sourceInventoryExecBudgetFileThreshold + 1,
			MaxPerRole:        10,
			MaxScanPerRole:    10,
			Deadline:          time.Now().Add(-time.Second),
		})},
	)
	if !set.truncated || set.complete || len(set.candidates) != 0 {
		t.Fatalf("expired execution budget should truncate before materializing file candidates: %+v", set)
	}
}

func TestSourceInventoryExecBudgetCancellationTruncatesFileScan(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/main.go",
		Language: repotypes.LangGo,
	}})
	view := newSourceInventoryExecutionView(graph, []string{"."})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := &types.BusContext{Ctx: cancelled}
	set := sourceInventoryFileCandidates(
		ctx,
		view,
		nil,
		newSourceInventoryScopeFilter(nil),
		&types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFile}},
		nil,
		false,
		sourceInventoryQueryFilter{},
		sourceInventoryExecBudgetForLens(ctx, types.SourceInventoryLensQuery{}, false, graph),
	)
	if !set.truncated || set.complete || len(set.candidates) != 0 {
		t.Fatalf("cancelled execution budget should truncate before materializing file candidates: %+v", set)
	}
}

func TestSourceInventoryCandidateBudgetCountsScopedQueryMisses(t *testing.T) {
	var files []*repotypes.FileInfo
	for i := 0; i < 12; i++ {
		rel := "src/file" + strconv.Itoa(i) + ".go"
		files = append(files, &repotypes.FileInfo{
			RelPath:  rel,
			Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{
				Name:     "Symbol" + strconv.Itoa(i),
				Kind:     "function",
				File:     rel,
				Line:     10,
				Exported: true,
			}},
		})
	}
	graph := testGraphWithFiles(files)
	view := newSourceInventoryExecutionView(graph, []string{"src"})
	set := sourceInventoryGraphCandidates(
		nil,
		graph,
		view,
		newSourceInventoryGraphSymbolIndex(graph),
		newSourceInventoryScopeFilter(nil),
		[]string{"src"},
		&types.SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction}},
		types.AnswerCandidateRoleFunction,
		sourceInventoryBuildQueryFilter("definitely_absent_query_token"),
		sourceInventoryExecBudget{kernel: sourceinventory.NewBudget(sourceinventory.BudgetOptions{
			ForceAdvisoryOnly: true,
			GraphFileCount:    sourceInventoryExecBudgetFileThreshold + 1,
			MaxPerRole:        10,
			MaxScanPerRole:    3,
		})},
	)
	if !set.truncated || set.complete || len(set.candidates) != 0 {
		t.Fatalf("query-miss scan should be bounded by scoped symbols, got %+v", set)
	}
}

func TestSourceInventoryGoStringEnumCandidatesCarriesBudgetTruncation(t *testing.T) {
	var files []*repotypes.FileInfo
	for i := 0; i < 12; i++ {
		files = append(files, &repotypes.FileInfo{
			RelPath:  "src/enum" + strconv.Itoa(i) + ".go",
			Language: repotypes.LangGo,
		})
	}
	graph := testGraphWithFiles(files)
	ctx := &types.BusContext{RepoRoot: t.TempDir()}
	set := sourceInventoryGoStringEnumCandidates(
		ctx,
		graph,
		[]string{"src"},
		&types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
			TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
			RequiresConstSet:  true,
		},
		sourceInventoryExecBudget{kernel: sourceinventory.NewBudget(sourceinventory.BudgetOptions{
			ForceAdvisoryOnly: true,
			GraphFileCount:    sourceInventoryExecBudgetFileThreshold + 1,
			MaxPerRole:        10,
			MaxScanPerRole:    3,
		})},
	)
	if !set.truncated || set.complete {
		t.Fatalf("budgeted enum special path must carry execution truncation, got %+v", set)
	}
}

func TestPublishSourceInventoryObservationFromToolObservation_ListFilesIgnoresRenderedAdvisory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mut := types.NewMutableState("source inventory advisory pollution")
	ctx := &types.BusContext{RepoRoot: root, Mutable: mut}
	ok := PublishSourceInventoryObservationFromToolObservation(ctx, types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary: strings.Join([]string{
			"[list_files: path=.]",
			"src",
			"README.md",
			"",
			"Structured source-inventory candidate checklist (verified navigation/count facts, not final answer text):",
			"- for a compact scoped member/count checklist, call repo_map with view=\"source_inventory\"",
			"Use this first as a navigation summary.",
			"- summary: member_rows=0",
			"",
		}, "\n"),
	})
	if !ok {
		t.Fatal("direct list_files should publish real rows and ignore rendered advisory text")
	}
	obs := mut.SourceInventoryObservation()
	var members []string
	memberSet := map[string]bool{}
	for _, set := range obs.Sets {
		for _, member := range set.Members {
			members = append(members, member.Name)
			memberSet[member.Name] = true
		}
	}
	for _, noise := range []string{"repo_map", "member_rows=0", "navigation summary", "Structured source-inventory"} {
		for _, member := range members {
			if strings.Contains(member, noise) {
				t.Fatalf("rendered advisory text entered source-inventory observation: %q in %+v", noise, obs)
			}
		}
	}
	if !memberSet["src"] || !memberSet["README.md"] || len(members) != 2 {
		t.Fatalf("expected only the two real list_files rows, got members=%v observation=%+v", members, obs)
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

func TestPublishSourceInventoryObservationFromLens_PreservesGraphSurfaceTerms(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/ui/Index.ets",
		Language: "arkts",
		Package:  "ui",
		Symbols: []repotypes.Symbol{{
			Name:     "Index",
			Kind:     "component",
			File:     "src/ui/Index.ets",
			Line:     6,
			Exported: true,
			Doc:      "@Entry @Component",
		}},
	}})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
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
		Scopes:        []string{"src"},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		Query:         "@Entry @Component",
	})
	if !obs.IsActive() || len(obs.Sets) != 1 || len(obs.Sets[0].Members) != 1 {
		t.Fatalf("expected one graph-backed source-inventory member: %+v", obs)
	}
	member := obs.Sets[0].Members[0]
	if strings.Join(member.SurfaceTerms, ",") != "@Entry,@Component" {
		t.Fatalf("graph surface terms were not preserved: %+v", member)
	}
	stored := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	if !stored.IsActive() || len(stored.Sets) != 1 || len(stored.Sets[0].Members) != 1 ||
		strings.Join(stored.Sets[0].Members[0].SurfaceTerms, ",") != "@Entry,@Component" {
		t.Fatalf("stored observation lost graph surface terms: %+v", stored)
	}
}

func TestPublishSourceInventoryObservationFromLens_PreservesGraphPackageAsRowAttribute(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/cart/cart.cj",
		Language: "cangjie",
		Package:  "demo.cart",
		Symbols: []repotypes.Symbol{{
			Name:     "extend Cart",
			Kind:     "function",
			File:     "src/cart/cart.cj",
			Line:     30,
			Exported: true,
		}},
	}})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		RequestedFields: []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
		},
		Confidence: 0.90,
	})

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Scopes:        []string{"src"},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
	})
	if !obs.IsActive() || len(obs.Sets) != 1 || len(obs.Sets[0].Members) != 1 {
		t.Fatalf("expected one graph-backed source-inventory member: %+v", obs)
	}
	attrs := obs.Sets[0].Members[0].Attributes
	if len(attrs) != 1 || attrs[0].Role != types.AnswerCandidateRolePackage || attrs[0].Name != "demo.cart" {
		t.Fatalf("graph package should be preserved as row attribute: %+v", obs.Sets[0].Members[0])
	}
}

func TestPublishSourceInventoryObservationFromLens_ProjectsTypedConstructSurface(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "corpus/04_extend_operator.cj",
		Language: "cangjie",
		Package:  "demo.stringext",
		Symbols: []repotypes.Symbol{{
			Name:     "String",
			Kind:     "extend",
			File:     "corpus/04_extend_operator.cj",
			Line:     6,
			Exported: true,
		}},
	}, {
		RelPath:  "corpus/07_foreign_ffi.cj",
		Language: "cangjie",
		Package:  "demo.ffi",
		Symbols: []repotypes.Symbol{{
			Name:     "native_add",
			Kind:     "foreign-func",
			File:     "corpus/07_foreign_ffi.cj",
			Line:     6,
			Exported: true,
			Doc:      "foreign",
		}},
	}, {
		RelPath:  "corpus/02_class_init_methods.cj",
		Language: "cangjie",
		Package:  "demo.greeter",
		Symbols: []repotypes.Symbol{{
			Name:     "Greeter",
			Kind:     "class",
			File:     "corpus/02_class_init_methods.cj",
			Line:     4,
			Exported: true,
			Doc:      "public",
		}},
	}})
	ctx := sourceInventoryTestContext("", graph, ".", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		Confidence:        0.90,
	})

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType},
		IncludeCounts: true,
		TopN:          10,
	})
	rendered := RenderSourceInventoryObservationView(obs, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleType},
		IncludeCounts: true,
		TopN:          10,
	})
	for _, want := range []string{"extend String", "foreign func native_add", "public class Greeter", "demo.stringext", "demo.ffi", "demo.greeter"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("typed construct source_inventory surface missing %q:\n%s", want, rendered)
		}
	}

	filtered := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		Query:         "foreign",
		TopN:          10,
	})
	if len(filtered.Sets) != 1 || len(filtered.Sets[0].Members) != 1 ||
		filtered.Sets[0].Members[0].Name != "native_add" ||
		!containsString(filtered.Sets[0].Members[0].SurfaceTerms, "foreign func native_add") {
		t.Fatalf("typed construct query should isolate foreign-func candidate, got %+v", filtered)
	}
}

func TestPublishSourceInventoryObservationFromLens_BudgetsBroadCandidateMaterialization(t *testing.T) {
	files := make([]*repotypes.FileInfo, 0, sourceInventoryExecBudgetFileThreshold+100)
	for i := 0; i < sourceInventoryExecBudgetFileThreshold+100; i++ {
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
	wantPage := 50
	if obs.Sets[0].Count != wantPage || len(obs.Sets[0].Members) != wantPage {
		t.Fatalf("broad lens should materialize exactly the current page %d, got %+v", wantPage, obs.Sets[0])
	}
	if obs.Sets[0].Total <= wantPage {
		t.Fatalf("broad lens should retain a typed total/next-page lower bound, got %+v", obs.Sets[0])
	}
	if obs.Complete || obs.Sets[0].Complete {
		t.Fatalf("budget-truncated observation must not be marked complete: %+v", obs)
	}
	if !sourceInventoryStringSliceContains(obs.Provenance, "repo_lens:candidate_budget_truncated") {
		t.Fatalf("budget-truncated observation should carry typed provenance: %+v", obs.Provenance)
	}
	if obs.Execution == nil || !obs.Execution.Budgeted || !obs.Execution.CandidateBudgetTruncated {
		t.Fatalf("budget-truncated observation should carry typed execution state: %+v", obs.Execution)
	}
	if obs.Page == nil || obs.Page.Total <= wantPage || obs.Page.Limit != 50 || obs.Page.NextCursor != "50" || obs.Page.Complete {
		t.Fatalf("budget-truncated observation should carry typed page cursor state: %+v", obs.Page)
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

func TestPublishSourceInventoryObservationFromLens_CursorPaginatesBeforeMaterialization(t *testing.T) {
	files := make([]*repotypes.FileInfo, 0, sourceInventoryExecBudgetFileThreshold+100)
	for i := 0; i < sourceInventoryExecBudgetFileThreshold+100; i++ {
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
	query := types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
		TopN:          25,
	}

	first := PublishSourceInventoryObservationFromLens(ctx, query)
	if !first.IsActive() || len(first.Sets) != 1 || first.Sets[0].Count != 25 || first.Page == nil || first.Page.NextCursor != "25" {
		t.Fatalf("first page should be a bounded current page with a cursor: %+v", first)
	}
	query.Cursor = first.Page.NextCursor
	second := PublishSourceInventoryObservationFromLens(ctx, query)
	if !second.IsActive() || len(second.Sets) != 1 || second.Sets[0].Count != 25 || second.Page == nil || second.Page.Offset != 25 || second.Page.NextCursor != "50" {
		t.Fatalf("second page should resume from typed cursor: %+v", second)
	}
	seenFirst := map[string]bool{}
	for _, member := range first.Sets[0].Members {
		seenFirst[member.Key] = true
	}
	for _, member := range second.Sets[0].Members {
		if seenFirst[member.Key] {
			t.Fatalf("cursor page should not rematerialize first-page member %q; first=%+v second=%+v", member.Key, first.Sets[0], second.Sets[0])
		}
	}
	rendered := RenderSourceInventoryObservationView(second, query)
	if !strings.Contains(rendered, "page_offset: 25") ||
		!strings.Contains(rendered, "showing rows [25,50)") ||
		!strings.Contains(rendered, "next_cursor=50") ||
		strings.Contains(rendered, "No candidate member rows matched") {
		t.Fatalf("second page render should not apply cursor offset twice:\n%s", rendered)
	}
}

func TestPublishSourceInventoryObservationFromLens_BudgetsBroadNoMatchScan(t *testing.T) {
	files := make([]*repotypes.FileInfo, 0, sourceInventoryExecBudgetMaxScanPerRole+100)
	for i := 0; i < sourceInventoryExecBudgetMaxScanPerRole+100; i++ {
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

func TestSourceInventoryLensExecutionGap_SatisfiedByClosureOnlyLensMarker(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance: []string{
			"request_traits:typed_source_enumeration_query",
			"pre_explore_typed_request",
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
	mut.EvidenceClosure().RecordSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance:   []string{"repo_lens:tool_query", "repo_lens:roles", "repo_lens:scopes"},
		Lens:         []string{"source_class_universe", "count"},
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleThirdParty,
			Count:    6,
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
		}},
	}

	if satisfied := SourceInventoryLensExecutionGapForContext(ctx); satisfied.Blocking {
		t.Fatalf("closure-carried repo_lens marker should satisfy source_inventory lens execution, got %+v", satisfied)
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

func TestSourceInventoryLensExecutionGap_SatisfiedByZeroResultRepoLensTypedObservation(t *testing.T) {
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
	if gap := SourceInventoryLensExecutionGapForContext(ctx); !gap.Blocking {
		t.Fatalf("rendered summary text alone must not satisfy source_inventory lens execution, got %+v", gap)
	}

	mut.ResetDispatchToolResults()
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary:  "Repo Lens: no source-inventory observation is available for the requested typed scope/role slice.",
		Observations: []types.ObservationRecord{{
			ID:        "repo_map:zero#navigation:source_inventory",
			Origin:    types.AnswerEvidenceOriginCrossRepoIndex,
			Producer:  "repo_map",
			Predicate: types.RepoMapNavigationObservationPredicate,
			Object:    string(types.RepoMapNavigationRouteSourceInventory),
		}},
	})
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("typed zero-result source_inventory lens observation should count as executed, got %+v", gap)
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

func TestSourceInventoryAcceptedClosureCoversExactUniverse_SurvivesMergedIncompleteRoleSet(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta"})
	obs := ctx.Mutable.SourceInventoryObservation()
	obs.Complete = false
	obs.Sets[0].Complete = false
	obs.Sets[0].Total = 20
	obs.Sets[0].Members = append(obs.Sets[0].Members, types.SourceInventoryObservationMember{
		Name:          "support",
		Key:           "other/support",
		SupportRef:    "other/support",
		Role:          types.AnswerCandidateRolePackage,
		File:          "other/support",
		CoverageState: types.SourceInventoryCoverageObserved,
	})
	obs.Sets[0].Count = len(obs.Sets[0].Members)
	ctx.Mutable.SetSourceInventoryObservation(obs)

	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "packages",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"alpha", "beta"},
	}}
	if !SourceInventoryAcceptedClosureCoversExactUniverse(ctx, facts) {
		t.Fatalf("member-level exact universe proof should survive a merged incomplete role set")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_AllSameLanguageFamiliesCovered(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMember("Bridge", types.AnswerCandidateRoleType, "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", 15),
		sourceInventoryRequestedUniverseMember("Cart", types.AnswerCandidateRoleType, "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", 14),
		sourceInventoryRequestedUniverseMember("Greeter", types.AnswerCandidateRoleType, "internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj", 6),
		sourceInventoryRequestedUniverseMember("native_add", types.AnswerCandidateRoleFunction, "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", 6),
		sourceInventoryRequestedUniverseMember("runOnMainThread", types.AnswerCandidateRoleFunction, "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj", 12),
	}, []types.SourceInventorySourceClassCount{{
		Role:    types.SourcePathRoleFixture,
		Count:   3,
		Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"},
	}, {
		Role:    types.SourcePathRoleThirdParty,
		Count:   8,
		Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj"},
	}, {
		Role:    types.SourcePathRoleProduction,
		Count:   20,
		Samples: []string{"internal/tool/source_inventory_universe_coverage.go"},
	}})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Cangjie requested constructs",
		Value:   "5",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Bridge", "Cart", "Greeter", "native_add", "runOnMainThread"},
	}}
	if !SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("covered same-language fixture+thirdparty exact universes should close requested universe")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_BlocksUncoveredSameLanguageClass(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMember("Bridge", types.AnswerCandidateRoleType, "eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj", 15),
		sourceInventoryRequestedUniverseMember("Cart", types.AnswerCandidateRoleType, "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", 14),
	}, []types.SourceInventorySourceClassCount{{
		Role:    types.SourcePathRoleFixture,
		Count:   3,
		Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"},
	}, {
		Role:    types.SourcePathRoleThirdParty,
		Count:   8,
		Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/02_class_init_methods.cj"},
	}})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Cangjie requested constructs",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Bridge", "Cart"},
	}}
	if SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("fixture-only coverage must not close while same-language thirdparty census remains uncovered")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_PrincipalAggregateFamilyIgnoresSupportLanguageDebt(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMemberWithLanguage("runRoot", types.AnswerCandidateRoleFunction, "cmd/root.go", 10, "go"),
		sourceInventoryRequestedUniverseMemberWithLanguage("parseCangjie", types.AnswerCandidateRoleFunction, "internal/tool/repomap/index/cangjie_parser.go", 20, "go"),
	}, []types.SourceInventorySourceClassCount{{
		Role:    types.SourcePathRoleFixture,
		Count:   3,
		Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"},
	}, {
		Role:    types.SourcePathRoleThirdParty,
		Count:   8,
		Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj"},
	}, {
		Role:    types.SourcePathRoleProduction,
		Count:   155,
		Samples: []string{"cmd/root.go", "internal/tool/repomap/index/cangjie_parser.go"},
	}})
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"runOnMainThread @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:12",
		},
	}}
	if !SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("line-backed principal source-family coverage should not be blocked by non-intersecting support-language inventory debt")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_ProductionScopeStillUsesPrincipalFamily(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMemberWithLanguage("runRoot", types.AnswerCandidateRoleFunction, "cmd/root.go", 10, "go"),
		sourceInventoryRequestedUniverseMemberWithLanguage("parseCangjie", types.AnswerCandidateRoleFunction, "internal/tool/repomap/index/cangjie_parser.go", 20, "go"),
	}, []types.SourceInventorySourceClassCount{{
		Role:    types.SourcePathRoleFixture,
		Count:   3,
		Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"},
	}, {
		Role:    types.SourcePathRoleThirdParty,
		Count:   8,
		Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj"},
	}, {
		Role:    types.SourcePathRoleProduction,
		Count:   155,
		Samples: []string{"cmd/root.go"},
	}})
	ctx.AnalysisIR.RequestModel.SourceScopeProfile = &types.SourceScopeProfile{
		RequestedScope: types.SourceScopeProduction,
		Confidence:     0.9,
	}
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"runOnMainThread @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:12",
		},
	}}
	if !SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("production-scope request model must not bypass typed principal source-family coverage")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_AggregateFamilyCanCloseWithoutExactUniverseRows(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext(nil, []types.SourceInventorySourceClassCount{{
		Role:    types.SourcePathRoleFixture,
		Count:   3,
		Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"},
	}, {
		Role:    types.SourcePathRoleThirdParty,
		Count:   8,
		Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj"},
	}, {
		Role:    types.SourcePathRoleProduction,
		Count:   155,
		Samples: []string{"cmd/root.go"},
	}})
	ctx.AnalysisIR.RequestModel.SourceScopeProfile = &types.SourceScopeProfile{
		RequestedScope: types.SourceScopeProduction,
		Confidence:     0.9,
	}
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"runOnMainThread @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:12",
		},
	}}
	if !SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("principal source-family coverage plus source-class census should close even when repo_map rows are not exact-universe carriers")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_AggregateFamilyRequiresSourceClassCensus(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext(nil, nil)
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
		},
	}}
	if SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("aggregate source locations alone must not close requested universe without a typed source-class census")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_ClassLanguageMatrixIgnoresUnrelatedClasses(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext(nil, []types.SourceInventorySourceClassCount{{
		Role:      types.SourcePathRoleProduction,
		Count:     923,
		Languages: []types.SourceInventoryLanguageCount{{Language: "go", Count: 900}, {Language: "python", Count: 23}},
	}, {
		Role:      types.SourcePathRoleTest,
		Count:     992,
		Languages: []types.SourceInventoryLanguageCount{{Language: "go", Count: 980}, {Language: "typescript", Count: 12}},
	}, {
		Role:      types.SourcePathRoleFixture,
		Count:     88,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 3}, {Language: "json", Count: 85}},
	}, {
		Role:      types.SourcePathRoleThirdParty,
		Count:     14,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 8}, {Language: "c", Count: 6}},
	}})
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"runOnMainThread @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:12",
		},
	}}
	if !SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("class-language matrix should not treat unrelated production/test languages as same-family debt")
	}
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_ClassLanguageMatrixBlocksMissingSameLanguageClass(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext(nil, []types.SourceInventorySourceClassCount{{
		Role:      types.SourcePathRoleFixture,
		Count:     3,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 3}},
	}, {
		Role:      types.SourcePathRoleThirdParty,
		Count:     8,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 8}},
	}})
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
		},
	}}
	if SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("missing same-language thirdparty class should still block requested-universe closure")
	}
}

func TestSourceInventoryCompletionAuthorityForContext_UsesMissingClassLanguageScopes(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMemberWithLanguage("RootType", types.AnswerCandidateRoleType, "cmd/root.go", 10, "go"),
	}, []types.SourceInventorySourceClassCount{{
		Role:      types.SourcePathRoleFixture,
		Count:     3,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 3, Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"}}},
	}, {
		Role:      types.SourcePathRoleThirdParty,
		Count:     8,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 8, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"}}},
	}, {
		Role:      types.SourcePathRoleProduction,
		Count:     923,
		Languages: []types.SourceInventoryLanguageCount{{Language: "go", Count: 923, Samples: []string{"cmd/root.go"}}},
	}})
	obs := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	obs.Lens = []string{"members", "source_class_universe", "repo_languages"}
	ctx.Mutable.SetSourceInventoryObservation(obs)
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"extend String @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj:6",
		},
	}}
	authority := sourceInventoryCompletionAuthorityForContext(ctx, types.SourceInventoryObservationFromMutable(ctx.Mutable), facts)
	if !authority.Blocking || !authority.FollowupDebt.IsActive() {
		t.Fatalf("authority should block with missing same-language class debt, got %+v", authority)
	}
	if !sourceInventoryTestStringSliceContains(authority.FollowupDebt.Query.Scopes, "eval/fixtures/testdata/cangjie_minimal/bridge") {
		t.Fatalf("follow-up scopes should target missing fixture cangjie scope, got %+v", authority.FollowupDebt.Query.Scopes)
	}
	if sourceInventoryTestStringSliceContains(authority.FollowupDebt.Query.Scopes, ".") {
		t.Fatalf("follow-up scopes should not fall back to repo root when class-language samples exist: %+v", authority.FollowupDebt.Query.Scopes)
	}
}

func TestSourceInventoryResolvedCompletionDowngrade_RendersFollowupDebtScope(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMemberWithLanguage("RootType", types.AnswerCandidateRoleType, "cmd/root.go", 10, "go"),
	}, []types.SourceInventorySourceClassCount{{
		Role:      types.SourcePathRoleFixture,
		Count:     3,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 3, Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"}}},
	}, {
		Role:      types.SourcePathRoleThirdParty,
		Count:     8,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 8, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj"}}},
	}})
	obs := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	obs.Lens = []string{"members", "source_class_universe", "repo_languages"}
	ctx.Mutable.SetSourceInventoryObservation(obs)
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"extend String @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/04_extend_operator.cj:6",
		},
	}}
	if downgrade := sourceInventoryResolvedCompletionDowngrade(ctx, "resolved", facts); downgrade == "" {
		t.Fatal("expected downgrade for missing same-language fixture class")
	}
	repairs := ctx.Mutable.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 {
		t.Fatal("downgrade should add a repair directive")
	}
	subject := repairs[len(repairs)-1].Subject
	if !strings.Contains(subject, `scopes=["eval/fixtures/testdata/cangjie_minimal/bridge"]`) {
		t.Fatalf("repair should render narrowed follow-up scope, got %q", subject)
	}
	if strings.Contains(subject, `scopes=["."]`) {
		t.Fatalf("repair should not preserve stale root scope when follow-up debt is narrower: %q", subject)
	}
}

func TestSourceInventoryCompletionAuthorityForContext_UsesAcceptedEvidenceWhenFactMembersDropPaths(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMemberWithLanguage("RootType", types.AnswerCandidateRoleType, "cmd/root.go", 10, "go"),
	}, []types.SourceInventorySourceClassCount{{
		Role:      types.SourcePathRoleFixture,
		Count:     3,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 3, Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"}}},
	}, {
		Role:      types.SourcePathRoleThirdParty,
		Count:     8,
		Languages: []types.SourceInventoryLanguageCount{{Language: "cangjie", Count: 8, Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj"}}},
	}, {
		Role:      types.SourcePathRoleTest,
		Count:     9,
		Languages: []types.SourceInventoryLanguageCount{{Language: "go", Count: 9, Samples: []string{"internal/tool/source_inventory_test.go"}}},
	}})
	ctx.Mutable.EvidenceClosure().AppendAcceptedEvidenceRefs([]types.AcceptedEvidenceRef{{
		ID:              "ev-native-add",
		Subject:         "native_add",
		Source:          "internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj",
		LineStart:       6,
		SourcePathRole:  types.SourcePathRoleThirdParty,
		GroundingStatus: types.GroundingGrounded,
	}, {
		ID:              "ev-support-go",
		Subject:         "parseCangjie",
		Source:          "internal/tool/repomap/index/cangjie_parser.go",
		LineStart:       20,
		SourcePathRole:  types.SourcePathRoleProduction,
		GroundingStatus: types.GroundingGrounded,
	}})
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "foreign func 声明",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"native_add (package demo.ffi) @ line 6",
		},
	}}
	authority := sourceInventoryCompletionAuthorityForContext(ctx, types.SourceInventoryObservationFromMutable(ctx.Mutable), facts)
	if !authority.Blocking || !authority.FollowupDebt.IsActive() {
		t.Fatalf("authority should use accepted evidence to derive missing same-language debt, got %+v", authority)
	}
	if !sourceInventoryTestStringSliceContains(authority.FollowupDebt.Query.Scopes, "eval/fixtures/testdata/cangjie_minimal/bridge") {
		t.Fatalf("follow-up scopes should target missing fixture cangjie scope, got %+v", authority.FollowupDebt.Query.Scopes)
	}
	if sourceInventoryTestStringSliceContains(authority.FollowupDebt.Query.Scopes, "internal/tool") {
		t.Fatalf("unmentioned support evidence must not inject Go test/tool debt: %+v", authority.FollowupDebt.Query.Scopes)
	}
}

func sourceInventoryTestStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSourceInventoryAcceptedClosureCoversRequestedUniverse_PrincipalAggregateFamilyBlocksMissingSameLanguageClass(t *testing.T) {
	ctx := sourceInventoryRequestedUniverseTestContext([]types.SourceInventoryObservationMember{
		sourceInventoryRequestedUniverseMemberWithLanguage("runRoot", types.AnswerCandidateRoleFunction, "cmd/root.go", 10, "go"),
	}, []types.SourceInventorySourceClassCount{{
		Role:    types.SourcePathRoleFixture,
		Count:   3,
		Samples: []string{"eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj"},
	}, {
		Role:    types.SourcePathRoleThirdParty,
		Count:   8,
		Samples: []string{"internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj"},
	}, {
		Role:    types.SourcePathRoleProduction,
		Count:   155,
		Samples: []string{"cmd/root.go"},
	}})
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "principal source constructs",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
		},
	}}
	if SourceInventoryAcceptedClosureCoversRequestedUniverse(ctx, facts) {
		t.Fatalf("principal source-family coverage must still block when a same-language source class remains uncovered")
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

func sourceInventoryRequestedUniverseTestContext(members []types.SourceInventoryObservationMember, classes []types.SourceInventorySourceClassCount) *types.BusContext {
	setsByRole := map[types.AnswerCandidateRole][]types.SourceInventoryObservationMember{}
	var roles []types.AnswerCandidateRole
	for _, member := range members {
		if _, ok := setsByRole[member.Role]; !ok {
			roles = append(roles, member.Role)
		}
		setsByRole[member.Role] = append(setsByRole[member.Role], member)
	}
	sets := make([]types.SourceInventoryObservationSet, 0, len(roles))
	for _, role := range roles {
		setMembers := setsByRole[role]
		sets = append(sets, types.SourceInventoryObservationSet{
			Role:     role,
			Complete: false,
			Count:    len(setMembers),
			Total:    len(setMembers) + 20,
			Members:  setMembers,
		})
	}
	mut := types.NewMutableState("source inventory requested universe")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:        true,
		AdvisoryOnly:  true,
		Complete:      false,
		Scopes:        []string{"."},
		SourceClasses: classes,
		Execution:     &types.SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets:          sets,
	})
	return &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType, types.AnswerCandidateRoleFunction},
				Confidence:        0.95,
			},
		}},
	}
}

func sourceInventoryRequestedUniverseMember(name string, role types.AnswerCandidateRole, file string, line int) types.SourceInventoryObservationMember {
	return sourceInventoryRequestedUniverseMemberWithLanguage(name, role, file, line, "cangjie")
}

func sourceInventoryRequestedUniverseMemberWithLanguage(name string, role types.AnswerCandidateRole, file string, line int, language string) types.SourceInventoryObservationMember {
	return types.SourceInventoryObservationMember{
		Name:          name,
		Key:           name,
		SupportRef:    name + ": " + file + ":" + strconv.Itoa(line),
		Provenance:    []string{sourceInventoryExactUniverseProvenanceRepoLensDirectChildren},
		Role:          role,
		File:          file,
		Line:          line,
		Language:      language,
		CoverageState: types.SourceInventoryCoverageObserved,
	}
}
