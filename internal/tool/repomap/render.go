package repomap

import (
	"fmt"
	"strings"
)

// ViewData is the structured intermediate representation of a
// repomap view. Phase 3 introduces it as the primary output shape
// for programmatic consumers; the markdown a caller sees from
// GenerateView is a render-time derivation of this data.
//
// Phase 3 converts only the "overview" view to use this path as a
// pattern for the broader three-layer split planned for Phase 4.
// Other views (file_map, task_map, call_path, edit_impact) still
// produce markdown directly; GenerateViewData returns nil for
// those types so callers can fall back to GenerateView.
type ViewData struct {
	Type     string        `json:"type"`
	Title    string        `json:"title"`
	Query    string        `json:"query,omitempty"`
	Intro    string        `json:"intro,omitempty"`
	Sections []ViewSection `json:"sections,omitempty"`
	Footer   string        `json:"footer,omitempty"`
}

// ViewSection is one logical grouping within a ViewData — a
// heading with zero or more items and optional nested
// subsections for hierarchical views (e.g. the per-file blocks
// inside task_map once that view migrates).
type ViewSection struct {
	Heading     string        `json:"heading,omitempty"`
	Intro       string        `json:"intro,omitempty"`
	Items       []ViewItem    `json:"items,omitempty"`
	Subsections []ViewSection `json:"subsections,omitempty"`
}

// ViewItem is one row inside a ViewSection's Items slice.
// `Text` is the primary rendered label; the other fields are
// hints for programmatic consumers that want to post-process
// results without parsing the rendered markdown.
type ViewItem struct {
	Text  string  `json:"text"`
	File  string  `json:"file,omitempty"`
	Kind  string  `json:"kind,omitempty"`
	Score float64 `json:"score,omitempty"`
}

// RenderMarkdown produces the markdown view that corresponds to a
// ViewData value. This is the single translation point from the
// structured representation to the human-readable output — no
// other code in repomap should concatenate markdown by hand for
// views that already have a ViewData.
//
// The format mirrors what the legacy views.go functions produced
// so downstream consumers (explorer prompts, LLM context
// builders, file-coverage analysis) continue to see the same
// shape of markdown.
func RenderMarkdown(d *ViewData) string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	if d.Title != "" {
		b.WriteString("# ")
		b.WriteString(d.Title)
		b.WriteString("\n\n")
	}
	if d.Intro != "" {
		b.WriteString(d.Intro)
		if !strings.HasSuffix(d.Intro, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	for i := range d.Sections {
		renderSection(&b, &d.Sections[i], 2)
	}
	if d.Footer != "" {
		b.WriteString("\n---\n")
		b.WriteString(d.Footer)
		if !strings.HasSuffix(d.Footer, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderSection(b *strings.Builder, s *ViewSection, depth int) {
	if s.Heading != "" {
		b.WriteString(strings.Repeat("#", depth))
		b.WriteString(" ")
		b.WriteString(s.Heading)
		b.WriteString("\n\n")
	}
	if s.Intro != "" {
		b.WriteString(s.Intro)
		if !strings.HasSuffix(s.Intro, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	for _, item := range s.Items {
		b.WriteString("- ")
		b.WriteString(item.Text)
		b.WriteString("\n")
	}
	if len(s.Items) > 0 {
		b.WriteString("\n")
	}
	for i := range s.Subsections {
		renderSection(b, &s.Subsections[i], depth+1)
	}
}

// GenerateViewData returns the structured ViewData for view types
// that have migrated to the dual-channel contract. For view types
// that still use the legacy markdown-direct path, GenerateViewData
// returns nil and callers should fall back to GenerateView.
//
// Phase 3 migrated "overview" as the initial pattern. Phase 4
// migrates the remaining views as a precondition for the
// three-layer split.
func GenerateViewData(g *Graph, viewType string, params ViewParams) *ViewData {
	switch viewType {
	case "overview", "":
		return buildOverviewData(g, params)
	case "task_map":
		if params.Query == "" {
			return buildOverviewData(g, params)
		}
		return buildTaskMapData(g, params)
	}
	return nil
}

// buildOverviewData produces the structured form of the overview
// view. It mirrors the legacy viewOverview function's markdown
// output but returns data instead of concatenated strings.
func buildOverviewData(g *Graph, params ViewParams) *ViewData {
	d := &ViewData{
		Type:  "overview",
		Title: "Repository Overview",
		Intro: "> **This is a navigation index, not evidence.** Use it to decide which files to read or grep next. Do not cite repo_map output as a source of truth — always verify by reading the actual file.",
	}

	// Languages section — sorted by file count, descending.
	langs := topLanguages(g)
	langSection := ViewSection{Heading: "Languages"}
	for _, lc := range langs {
		langSection.Items = append(langSection.Items, ViewItem{
			Text: fmt.Sprintf("**%s**: %d files", lc.lang, lc.count),
			Kind: "language",
		})
	}
	d.Sections = append(d.Sections, langSection)

	// Project files (go.mod, Cargo.toml, ...) — skipped when none.
	if len(g.Metadata.SpecialFiles) > 0 {
		pfs := ViewSection{Heading: "Project Files"}
		for _, f := range g.Metadata.SpecialFiles {
			pfs.Items = append(pfs.Items, ViewItem{
				Text: fmt.Sprintf("`%s`", f),
				File: f,
				Kind: "special_file",
			})
		}
		d.Sections = append(d.Sections, pfs)
	}

	// Packages / modules — sorted by file count, descending.
	pkgSection := ViewSection{Heading: "Packages/Modules"}
	for _, p := range topPackages(g) {
		pkgSection.Items = append(pkgSection.Items, ViewItem{
			Text: fmt.Sprintf("**%s** — %d files, %d symbols", p.name, p.fileCount, p.symCount),
			Kind: "package",
		})
	}
	d.Sections = append(d.Sections, pkgSection);

	// Top files by importance.
	topN := params.TopN
	if topN <= 0 {
		topN = 15
	}
	topFiles := TopFiles(g, topN)
	topSection := ViewSection{Heading: fmt.Sprintf("Top %d Files (by importance)", topN)}
	for i, fi := range topFiles {
		score := g.Scores[fi.RelPath]
		exportedCount := 0
		for _, sym := range fi.Symbols {
			if sym.Exported {
				exportedCount++
			}
		}
		topSection.Items = append(topSection.Items, ViewItem{
			Text:  fmt.Sprintf("%d. `%s` — %d symbols (%d exported), score %.1f", i+1, fi.RelPath, len(fi.Symbols), exportedCount, score),
			File:  fi.RelPath,
			Score: score,
			Kind:  "top_file",
		})
	}
	d.Sections = append(d.Sections, topSection)

	d.Footer = fmt.Sprintf("*%d files, %d symbols, %d relations*",
		g.Metadata.FileCount, g.Metadata.SymbolCount, g.Metadata.RelationCount)
	return d
}

type langCount struct {
	lang  string
	count int
}

func topLanguages(g *Graph) []langCount {
	out := make([]langCount, 0, len(g.Metadata.Languages))
	for l, c := range g.Metadata.Languages {
		out = append(out, langCount{l, c})
	}
	sortByCountDesc(out)
	return out
}

type packageSummary struct {
	name      string
	fileCount int
	symCount  int
}

func topPackages(g *Graph) []packageSummary {
	pkgs := make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Package == "" {
			continue
		}
		pkgs[fi.Package] = append(pkgs[fi.Package], fi.RelPath)
	}
	out := make([]packageSummary, 0, len(pkgs))
	for name, files := range pkgs {
		symCount := 0
		for _, f := range files {
			if fi, ok := g.FileIndex[f]; ok {
				symCount += len(fi.Symbols)
			}
		}
		out = append(out, packageSummary{name: name, fileCount: len(files), symCount: symCount})
	}
	// Sort by file count descending, tie-break by name for
	// deterministic output across runs.
	sortPackages(out)
	return out
}

func sortByCountDesc(s []langCount) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && (s[j].count > s[j-1].count || (s[j].count == s[j-1].count && s[j].lang < s[j-1].lang)); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sortPackages(s []packageSummary) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && (s[j].fileCount > s[j-1].fileCount || (s[j].fileCount == s[j-1].fileCount && s[j].name < s[j-1].name)); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// buildTaskMapData produces the structured form of the task_map
// view. Runs a query-biased RankGraph pass (same side-effect as
// the legacy viewTaskMap function), then emits one "Relevant
// Files" section with one ViewSection per file and matched
// symbols as Items on that file's subsection. Matched-symbol
// filtering uses the same TokenizeQuery tokens that drive
// rank.go's queryMatchScore, so the shown symbols are exactly
// the ones that contributed to the file's rank score.
//
// The legacy view appended "imports: …" and "imported by: …"
// lines directly under each file block as indented bullets.
// ViewItem is a flat list shape, so these become plain items
// at the end of the file's Items slice with the same visible
// prefix, giving identical rendered markdown.
func buildTaskMapData(g *Graph, params ViewParams) *ViewData {
	RankGraph(g, params.Query)

	topN := params.TopN
	if topN <= 0 {
		topN = 20
	}
	relevant := TopFiles(g, topN)

	// Primary-weight tokens only: sub-tokens would over-match the
	// rendered "matched symbols" list and clutter the human view
	// without adding information beyond what the file score
	// already encodes.
	tokens := TokenizeQuery(params.Query)
	primary := tokens[:0]
	for _, t := range tokens {
		if t.Weight >= 1.0 {
			primary = append(primary, t)
		}
	}

	body := ViewSection{Heading: "Relevant Files"}
	for _, fi := range relevant {
		score := g.Scores[fi.RelPath]
		if score <= 0 {
			continue
		}
		fileSection := ViewSection{
			Heading: fmt.Sprintf("%s (score: %.1f)", fi.RelPath, score),
		}
		for _, sym := range fi.Symbols {
			nameLower := strings.ToLower(sym.Name)
			matched := false
			for _, t := range primary {
				if strings.Contains(nameLower, t.Text) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			line := fmt.Sprintf("`%s` %s :%d", sym.Name, sym.Kind, sym.Line)
			if sym.Signature != "" {
				line += " `" + sym.Signature + "`"
			}
			if sym.Doc != "" {
				line += " — " + sym.Doc
			}
			fileSection.Items = append(fileSection.Items, ViewItem{
				Text: line,
				File: fi.RelPath,
				Kind: sym.Kind,
			})
		}
		if deps := g.FilesImportedBy(fi.RelPath); len(deps) > 0 {
			fileSection.Items = append(fileSection.Items, ViewItem{
				Text: "  - imports: " + strings.Join(abbreviate(deps, 5), ", "),
				Kind: "imports",
			})
		}
		if importers := g.FilesImporting(fi.RelPath); len(importers) > 0 {
			fileSection.Items = append(fileSection.Items, ViewItem{
				Text: "  - imported by: " + strings.Join(abbreviate(importers, 5), ", "),
				Kind: "imported_by",
			})
		}
		body.Subsections = append(body.Subsections, fileSection)
	}

	return &ViewData{
		Type:  "task_map",
		Title: "Task Map: " + params.Query,
		Query: params.Query,
		Intro: "> **Navigation index only.** Use these results to decide which files to read or grep — do not treat them as evidence.",
		Sections: []ViewSection{body},
	}
}
