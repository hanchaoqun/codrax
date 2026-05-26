package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestRenderMarkdownBasicShape locks the top-level markdown
// layout: title → intro → sections → footer. The test does not
// assert every byte — it asserts the structural landmarks that
// downstream consumers (explorer prompts, LLM context builders)
// rely on.
func TestRenderMarkdownBasicShape(t *testing.T) {
	d := &ViewData{
		Type:  "overview",
		Title: "Test View",
		Intro: "> Navigation index only.",
		Sections: []ViewSection{
			{
				Heading: "First Section",
				Items: []ViewItem{
					{Text: "item one"},
					{Text: "item two"},
				},
			},
			{
				Heading: "Second Section",
				Items: []ViewItem{
					{Text: "only item"},
				},
			},
		},
		Footer: "*summary line*",
	}
	got := RenderMarkdown(d)

	wantMarks := []string{
		"# Test View",
		"> Navigation index only.",
		"## First Section",
		"- item one",
		"- item two",
		"## Second Section",
		"- only item",
		"---",
		"*summary line*",
	}
	for _, m := range wantMarks {
		if !strings.Contains(got, m) {
			t.Errorf("missing landmark %q in output:\n%s", m, got)
		}
	}

	// Sanity: second section must come after first, footer after sections.
	firstIdx := strings.Index(got, "## First Section")
	secondIdx := strings.Index(got, "## Second Section")
	footerIdx := strings.Index(got, "*summary line*")
	if !(firstIdx < secondIdx && secondIdx < footerIdx) {
		t.Errorf("order corrupted: first=%d second=%d footer=%d", firstIdx, secondIdx, footerIdx)
	}
}

// TestViewDataRoundTrip verifies the ViewData JSON schema survives
// a marshal/unmarshal cycle, which is the programmatic-consumer
// contract for Phase 3.
func TestViewDataRoundTrip(t *testing.T) {
	orig := &ViewData{
		Type:  "overview",
		Title: "Repository Overview",
		Sections: []ViewSection{
			{
				Heading: "Languages",
				Items: []ViewItem{
					{Text: "**go**: 200 files", Kind: "language"},
				},
			},
			{
				Heading: "Top Files",
				Items: []ViewItem{
					{Text: "1. `main.go` — 5 symbols", File: "main.go", Score: 2.5, Kind: "top_file"},
				},
			},
		},
		Footer: "*200 files, 1500 symbols, 10000 relations*",
	}
	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round ViewData
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Type != orig.Type || round.Title != orig.Title {
		t.Errorf("header fields corrupted: %+v", round)
	}
	if len(round.Sections) != 2 {
		t.Fatalf("section count = %d, want 2", len(round.Sections))
	}
	if round.Sections[1].Items[0].Score != 2.5 {
		t.Errorf("score field corrupted: %+v", round.Sections[1].Items[0])
	}
}

// TestGenerateViewDataOverview confirms the dual-channel path
// produces the expected section skeleton for the overview view
// from a handcrafted types.Graph, and that RenderMarkdown over the same
// data contains every landmark the legacy markdown emitted.
func TestGenerateViewDataOverview(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:   "internal/a.go",
			Language:  types.LangGo,
			Package:   "a",
			IsSpecial: false,
			Symbols: []types.Symbol{
				{Name: "Foo", Kind: "type", Exported: true},
				{Name: "bar", Kind: "function"},
			},
		},
		{
			RelPath:   "go.mod",
			Language:  "",
			IsSpecial: true,
		},
	}
	g := index.BuildGraph(t.TempDir(), files)
	g.Scores[files[0].RelPath] = 1.5

	d := GenerateViewData(g, "overview", types.ViewParams{TopN: 5})
	if d == nil {
		t.Fatal("GenerateViewData returned nil")
	}
	if d.Type != "overview" {
		t.Errorf("Type = %q", d.Type)
	}
	if d.Title == "" {
		t.Error("Title empty")
	}

	// Expect the four canonical sections: Languages, Project
	// Files, Packages/Modules, Top N Files.
	headings := make(map[string]bool)
	for _, s := range d.Sections {
		headings[s.Heading] = true
	}
	for _, want := range []string{
		"Languages",
		"Project Files",
		"Packages/Modules",
		"Top 5 Files",
	} {
		if !headings[want] {
			t.Errorf("missing section %q; got %v", want, headings)
		}
	}

	md := RenderMarkdown(d)
	for _, want := range []string{
		"# Repository Overview",
		"## Languages",
		"## Project Files",
		"## Packages/Modules",
		"## Top 5 Files",
		"`internal/a.go`",
		"`go.mod`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestGenerateViewDataTaskMap confirms the Phase 4 task_map
// migration produces a structured view data with one per-file
// subsection per ranked relevant file, and that the rendered
// markdown preserves the legacy headline/item shape.
func TestGenerateViewDataTaskMap(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "a.go",
			Language: types.LangGo,
			Package:  "main",
			Symbols: []types.Symbol{
				{Name: "Finalizer", Kind: "type", Line: 5, Exported: true},
				{Name: "AnswerShape", Kind: "type", Line: 20, Exported: true},
			},
		},
		{
			RelPath:  "b.go",
			Language: types.LangGo,
			Package:  "main",
			Symbols: []types.Symbol{
				{Name: "Other", Kind: "function", Line: 10, Exported: true},
			},
		},
	}
	g := index.BuildGraph(t.TempDir(), files)

	d := GenerateViewData(g, "task_map", types.ViewParams{Query: "Finalizer AnswerShape", TopN: 5})
	if d == nil {
		t.Fatal("GenerateViewData(task_map) returned nil")
	}
	if d.Type != "task_map" || d.Query != "Finalizer AnswerShape" {
		t.Errorf("header wrong: %+v", d)
	}
	if len(d.Sections) != 2 || d.Sections[0].Heading != "Navigation Clusters" || d.Sections[1].Heading != "Relevant Files" {
		t.Fatalf("expected navigation clusters plus 'Relevant Files' sections, got %+v", d.Sections)
	}
	subs := d.Sections[1].Subsections
	if len(subs) == 0 {
		t.Fatal("expected at least one file subsection")
	}
	// Locate a.go's subsection — it must have both symbols as
	// matched items. b.go may or may not surface depending on
	// centrality; its matched-item count must be 0 regardless.
	var aSub *ViewSection
	for i := range subs {
		if strings.Contains(subs[i].Heading, "a.go") {
			aSub = &subs[i]
		} else if strings.Contains(subs[i].Heading, "b.go") {
			// b.go has no matching symbol names, so its items
			// should be empty (apart from optional imports lines
			// which don't apply in this no-import fixture).
			if len(subs[i].Items) != 0 {
				t.Errorf("b.go subsection should have no matched items, got %+v", subs[i].Items)
			}
		}
	}
	if aSub == nil {
		t.Fatalf("a.go subsection missing; got %+v", subs)
	}
	if !strings.Contains(aSub.Heading, "score:") {
		t.Errorf("a.go heading missing score: %q", aSub.Heading)
	}
	if len(aSub.Items) != 2 {
		t.Errorf("expected 2 matched items in a.go, got %d: %+v", len(aSub.Items), aSub.Items)
	}

	md := RenderMarkdown(d)
	for _, want := range []string{
		"# Task Map: Finalizer AnswerShape",
		"## Navigation Clusters",
		`repo_map(view="relation_map"`,
		"## Relevant Files",
		"### a.go (score: ",
		"- `Finalizer` type :5",
		"- `AnswerShape` type :20",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestGenerateViewDataTaskMapBroadQueryAdvisory(t *testing.T) {
	g := index.BuildGraph(t.TempDir(), []*types.FileInfo{{
		RelPath:  "a.go",
		Language: types.LangGo,
		Package:  "main",
		Symbols: []types.Symbol{
			{Name: "Serve", Kind: "function", Line: 10, Exported: true},
		},
	}})

	d := GenerateViewData(g, "task_map", types.ViewParams{TopN: 5})
	if d == nil {
		t.Fatal("GenerateViewData(task_map) returned nil")
	}
	md := RenderMarkdown(d)
	for _, want := range []string{
		"Broad task_map because no `query` was supplied",
		`view="relation_map"`,
		"`query`",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("broad task_map should render advisory %q:\n%s", want, md)
		}
	}
}

// TestGenerateViewDataFileMap confirms file_map produces one
// ViewSection per file with symbols grouped by kind in the
// canonical order, and that the rendered markdown preserves the
// exported-marker (+) and receiver-qualified bullet shapes.
func TestGenerateViewDataFileMap(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "foo.go",
			Language: types.LangGo,
			Package:  "foo",
			Symbols: []types.Symbol{
				{Name: "Doer", Kind: "interface", Line: 5, Exported: true},
				{Name: "Impl", Kind: "struct", Line: 10, Exported: true},
				{Name: "Do", Kind: "method", Line: 15, Exported: true, Receiver: "Impl"},
				{Name: "helper", Kind: "function", Line: 30},
			},
		},
	}
	g := index.BuildGraph(t.TempDir(), files)

	d := GenerateViewData(g, "file_map", types.ViewParams{TopN: 10})
	if d == nil {
		t.Fatal("GenerateViewData(file_map) returned nil")
	}
	if d.Type != "file_map" || d.Title != "File Map" {
		t.Errorf("header wrong: %+v", d)
	}
	if len(d.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(d.Sections))
	}
	sec := d.Sections[0]
	if sec.Heading != "foo.go [foo]" {
		t.Errorf("heading = %q, want %q", sec.Heading, "foo.go [foo]")
	}
	// Canonical order: interface → struct → function → method.
	wantOrder := []string{"Doer", "Impl", "helper", "Do"}
	if len(sec.Items) != len(wantOrder) {
		t.Fatalf("expected %d items, got %d: %+v", len(wantOrder), len(sec.Items), sec.Items)
	}
	for i, w := range wantOrder {
		if !strings.Contains(sec.Items[i].Text, "`"+w) && !strings.Contains(sec.Items[i].Text, ") "+w) {
			t.Errorf("item[%d] = %q, want containing %q", i, sec.Items[i].Text, w)
		}
	}

	md := RenderMarkdown(d)
	for _, want := range []string{
		"# File Map",
		"## foo.go [foo]",
		"+`Doer` interface :5",
		"+`Impl` struct :10",
		"+`(Impl) Do` method :15",
		" `helper` function :30",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestGenerateViewDataUnknownReturnsNil pins the dual-channel
// contract: any view type that is not one of the five supported
// names must return nil so GenerateView's fallback path can
// redirect to "overview".
func TestGenerateViewDataUnknownReturnsNil(t *testing.T) {
	g := index.BuildGraph(t.TempDir(), nil)
	if got := GenerateViewData(g, "unknown", types.ViewParams{}); got != nil {
		t.Errorf("GenerateViewData(unknown) = %+v, want nil", got)
	}
}

// TestGenerateViewDataCallPath locks the call_path BFS structure
// — one ViewSection with BFS-depth-tagged items, rendered with
// two-space indent per depth step.
func TestGenerateViewDataCallPath(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "main.go", Language: types.LangGo, Package: "main", Symbols: []types.Symbol{
			{Name: "Run", Kind: "function", Line: 10, Exported: true},
		}},
		{RelPath: "lib.go", Language: types.LangGo, Package: "main", Symbols: []types.Symbol{
			{Name: "Helper", Kind: "function", Line: 5, Exported: true},
		}},
		{RelPath: "deep.go", Language: types.LangGo, Package: "main"},
	}
	g := index.BuildGraph(t.TempDir(), files)
	// Hand-wire the import graph.
	g.ImportGraph["main.go"] = []string{"lib.go"}
	g.ImportGraph["lib.go"] = []string{"deep.go"}

	d := GenerateViewData(g, "call_path", types.ViewParams{EntryPoint: "main.go"})
	if d == nil {
		t.Fatal("GenerateViewData(call_path) returned nil")
	}
	if d.Type != "call_path" || !strings.Contains(d.Title, "main.go") {
		t.Errorf("header wrong: %+v", d)
	}
	if len(d.Sections) != 1 || len(d.Sections[0].Items) != 3 {
		t.Fatalf("expected 1 section with 3 items, got %+v", d.Sections)
	}
	depths := []int{d.Sections[0].Items[0].Depth, d.Sections[0].Items[1].Depth, d.Sections[0].Items[2].Depth}
	wantDepths := []int{0, 1, 2}
	for i := range depths {
		if depths[i] != wantDepths[i] {
			t.Errorf("depth[%d] = %d, want %d", i, depths[i], wantDepths[i])
		}
	}

	md := RenderMarkdown(d)
	for _, want := range []string{
		"# Call Path from main.go",
		"- `main.go` → Run",
		"  - `lib.go` → Helper",
		"    - `deep.go`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestGenerateViewDataEditImpact locks the edit_impact four-
// section structure and the exported-symbol caller-count
// suffix.
func TestGenerateViewDataEditImpact(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath: "target.go", Language: types.LangGo, Package: "main",
			Symbols: []types.Symbol{
				{Name: "PublicAPI", Kind: "function", Line: 5, Exported: true},
				{Name: "private", Kind: "function", Line: 10},
			},
		},
		{
			RelPath: "caller.go", Language: types.LangGo, Package: "main",
			Relations: []types.Relation{
				{Kind: "call", From: "caller.go", To: "PublicAPI", File: "caller.go", Line: 5},
			},
		},
		{RelPath: "dep.go", Language: types.LangGo, Package: "main"},
	}
	g := index.BuildGraph(t.TempDir(), files)
	g.ReverseImports["target.go"] = []string{"caller.go"}
	g.ImportGraph["target.go"] = []string{"dep.go"}

	d := GenerateViewData(g, "edit_impact", types.ViewParams{TargetFile: "target.go"})
	if d == nil {
		t.Fatal("GenerateViewData(edit_impact) returned nil")
	}
	if !strings.Contains(d.Title, "target.go") {
		t.Errorf("title wrong: %q", d.Title)
	}
	headings := make([]string, len(d.Sections))
	for i, s := range d.Sections {
		headings[i] = s.Heading
	}
	// Transitive Dependents section is omitted when it would equal
	// the Direct Dependents list, so we expect exactly three.
	wantHeadings := []string{"Direct Dependents", "Exported Symbols", "Dependencies (this file imports)"}
	if len(headings) != len(wantHeadings) {
		t.Fatalf("headings = %v, want %v", headings, wantHeadings)
	}
	for i := range wantHeadings {
		if headings[i] != wantHeadings[i] {
			t.Errorf("headings[%d] = %q, want %q", i, headings[i], wantHeadings[i])
		}
	}

	md := RenderMarkdown(d)
	for _, want := range []string{
		"# Edit Impact: target.go",
		"## Direct Dependents",
		"- `caller.go`",
		"## Exported Symbols",
		"- `PublicAPI` function (referenced from 1 files)",
		"## Dependencies (this file imports)",
		"- `dep.go`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	// Private symbols must not leak.
	if strings.Contains(md, "`private`") {
		t.Errorf("private symbol leaked:\n%s", md)
	}
}

// TestSemanticSubgraphView covers the Phase 5 item 8 view. Builds
// a small fixture graph with a 4-file linear chain, an obvious hub,
// and an articulation point, and asserts that each landmark lands
// in the correct rendered section.
func TestSemanticSubgraphView(t *testing.T) {
	g := &types.Graph{
		FileIndex:      map[string]*types.FileInfo{},
		ImportGraph:    map[string][]string{},
		ReverseImports: map[string][]string{},
	}
	add := func(f string, deps ...string) {
		if g.FileIndex[f] == nil {
			g.FileIndex[f] = &types.FileInfo{RelPath: f}
		}
		for _, d := range deps {
			if g.FileIndex[d] == nil {
				g.FileIndex[d] = &types.FileInfo{RelPath: d}
			}
			g.ImportGraph[f] = append(g.ImportGraph[f], d)
			g.ReverseImports[d] = append(g.ReverseImports[d], f)
		}
	}
	// Linear chain: pipe_a → pipe_b → pipe_c → pipe_d.
	add("pipe_a", "pipe_b")
	add("pipe_b", "pipe_c")
	add("pipe_c", "pipe_d")
	// Hub: three importers + one dependency.
	add("cli_a", "hub")
	add("cli_b", "hub")
	add("cli_c", "hub")
	add("hub", "util")
	// Articulation point: left — cut — right.
	add("left", "cut")
	add("cut", "right")

	d := GenerateViewData(g, "semantic_subgraph", types.ViewParams{})
	if d == nil {
		t.Fatalf("GenerateViewData returned nil")
	}
	if d.Type != "semantic_subgraph" {
		t.Errorf("Type = %q, want semantic_subgraph", d.Type)
	}
	if len(d.Sections) != 3 {
		t.Fatalf("want 3 sections, got %d", len(d.Sections))
	}
	md := RenderMarkdown(d)

	for _, want := range []string{
		"# Semantic Subgraphs",
		"## Chains (linear import pipelines)",
		"## Hubs (high-degree files)",
		"## Bridges (articulation points)",
		"`pipe_a` → `pipe_b` → `pipe_c` → `pipe_d`",
		"`hub` — fan-in 3, fan-out 1",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("semantic_subgraph markdown missing %q:\n%s", want, md)
		}
	}
	// Cut vertex must appear in the Bridges section.
	bridgeIdx := strings.Index(md, "## Bridges")
	if bridgeIdx < 0 || !strings.Contains(md[bridgeIdx:], "`cut`") {
		t.Errorf("expected `cut` as a bridge, got:\n%s", md)
	}
}

func TestGenerateViewDataRelationMapAdvisory(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "internal/pipeline/analyzer.go",
			Language: types.LangGo,
			Package:  "pipeline",
			Symbols: []types.Symbol{
				{Name: "Run", Kind: "function", File: "internal/pipeline/analyzer.go", Line: 10, Exported: true},
			},
			Relations: []types.Relation{{
				Kind: "call",
				To:   "Process",
				File: "internal/pipeline/analyzer.go",
				Line: 12,
				ToEP: types.RelationEndpoint{Name: "Process"},
			}},
		},
		{
			RelPath:  "internal/pipeline/process.go",
			Language: types.LangGo,
			Package:  "pipeline",
			Symbols: []types.Symbol{
				{Name: "Process", Kind: "function", File: "internal/pipeline/process.go", Line: 5, Exported: true},
			},
		},
		{
			RelPath:  "java/app/Child.java",
			Language: types.LangJava,
			Package:  "app",
			Symbols: []types.Symbol{
				{Name: "Child", Kind: "class", File: "java/app/Child.java", Line: 3, Exported: true},
			},
			Relations: []types.Relation{{
				Kind: "inheritance",
				To:   "Base",
				File: "java/app/Child.java",
				Line: 3,
				ToEP: types.RelationEndpoint{Name: "Base", File: "java/app/Base.java", Line: 2},
			}},
		},
		{
			RelPath:  "java/app/Base.java",
			Language: types.LangJava,
			Package:  "app",
			Symbols: []types.Symbol{
				{Name: "Base", Kind: "class", File: "java/app/Base.java", Line: 2, Exported: true},
			},
		},
	}
	g := index.BuildGraph(t.TempDir(), files)

	d := GenerateViewData(g, "relation_map", types.ViewParams{
		Query:         "Run Child",
		Scopes:        []string{"internal", "java"},
		RelationKinds: []string{"call", "inheritance"},
		TopN:          10,
	})
	if d == nil {
		t.Fatal("GenerateViewData(relation_map) returned nil")
	}
	md := RenderMarkdown(d)
	for _, want := range []string{
		"# Repo Lens: Relation Map",
		"Advisory structural navigation only",
		"source_candidates=3 relation_rows=2",
		"`Run` — kind=function — @ internal/pipeline/analyzer.go:10",
		"call `Run @ internal/pipeline/analyzer.go:10` → Process @ internal/pipeline/process.go:5 — observed @ internal/pipeline/analyzer.go:12",
		"inheritance `Child @ java/app/Child.java:3` → Base @ java/app/Base.java:2 — observed @ java/app/Child.java:3",
		"## Suggested Verification Files",
		"`internal/pipeline/analyzer.go`",
		"`internal/pipeline/process.go`",
		"`java/app/Child.java`",
		"`java/app/Base.java`",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("relation_map missing %q:\n%s", want, md)
		}
	}
}

func TestGenerateViewDataRelationMapBroadFallbackAdvisesNarrowing(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath:  "a.go",
			Language: types.LangGo,
			Symbols: []types.Symbol{
				{Name: "Run", Kind: "function", File: "a.go", Line: 10},
			},
			Relations: []types.Relation{{
				Kind: "call",
				To:   "Next",
				File: "a.go",
				Line: 12,
				ToEP: types.RelationEndpoint{Name: "Next"},
			}},
		},
		{
			RelPath:  "b.go",
			Language: types.LangGo,
			Symbols: []types.Symbol{
				{Name: "Next", Kind: "function", File: "b.go", Line: 5},
			},
		},
	}
	g := index.BuildGraph(t.TempDir(), files)

	d := GenerateViewData(g, "relation_map", types.ViewParams{TopN: 10})
	if d == nil {
		t.Fatal("GenerateViewData(relation_map) returned nil")
	}
	md := RenderMarkdown(d)
	for _, want := range []string{
		"mode=broad_fallback",
		"Broad fallback because no `sources`, `query`, or `scope` was supplied",
		"rerun with concrete `sources`, `scope`/`scopes`, or `query`",
		"call `Run @ a.go:10` → Next @ b.go:5",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("broad fallback relation_map missing %q:\n%s", want, md)
		}
	}
}
