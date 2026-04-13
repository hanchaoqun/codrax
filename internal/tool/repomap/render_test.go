package repomap

import (
	"encoding/json"
	"strings"
	"testing"
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
// from a handcrafted Graph, and that RenderMarkdown over the same
// data contains every landmark the legacy markdown emitted.
func TestGenerateViewDataOverview(t *testing.T) {
	files := []*FileInfo{
		{
			RelPath:   "internal/a.go",
			Language:  LangGo,
			Package:   "a",
			IsSpecial: false,
			Symbols: []Symbol{
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
	g := BuildGraph(t.TempDir(), files)
	g.Scores[files[0].RelPath] = 1.5

	d := GenerateViewData(g, "overview", ViewParams{TopN: 5})
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
		"Top 5 Files (by importance)",
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
		"## Top 5 Files (by importance)",
		"`internal/a.go`",
		"`go.mod`",
		"---",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestGenerateViewDataNonOverviewReturnsNil keeps the dual-channel
// contract explicit — Phase 3 only migrates "overview"; every
// other view type falls back to the legacy markdown path and
// GenerateViewData must return nil for them.
func TestGenerateViewDataNonOverviewReturnsNil(t *testing.T) {
	g := BuildGraph(t.TempDir(), nil)
	for _, t2 := range []string{"file_map", "task_map", "call_path", "edit_impact", "unknown"} {
		if got := GenerateViewData(g, t2, ViewParams{}); got != nil {
			t.Errorf("GenerateViewData(%q) = %+v, want nil", t2, got)
		}
	}
}
