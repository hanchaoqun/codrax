package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
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
//
// `Depth` is the indent level in the rendered bullet list —
// 0 (the default) produces a top-level `- text` line, 1 produces
// `  - text`, 2 produces `    - text`, and so on. Used by the
// call_path view to visualize a BFS walk; most other views leave
// it at zero.
type ViewItem struct {
	Text  string  `json:"text"`
	File  string  `json:"file,omitempty"`
	Kind  string  `json:"kind,omitempty"`
	Score float64 `json:"score,omitempty"`
	Depth int     `json:"depth,omitempty"`
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
		if item.Depth > 0 {
			b.WriteString(strings.Repeat("  ", item.Depth))
		}
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
func GenerateViewData(g *types.Graph, viewType string, params types.ViewParams) *ViewData {
	switch viewType {
	case "overview", "":
		return buildOverviewData(g, params)
	case "task_map":
		if params.Query == "" {
			return buildOverviewData(g, params)
		}
		return buildTaskMapData(g, params)
	case "file_map":
		return buildFileMapData(g, params)
	case "call_path":
		return buildCallPathData(g, params)
	case "edit_impact":
		return buildEditImpactData(g, params)
	case "semantic_subgraph":
		return buildSemanticSubgraphData(g, params)
	case "relation_map":
		return buildRelationMapData(g, params)
	}
	return nil
}

// buildSemanticSubgraphData produces the structured form of the
// semantic_subgraph view — Phase 5 plan item 8. Emits three
// sections derived from the import graph's topology:
//
//   - Chains  : maximal linear dependency pipelines (retrieve.ComputeChains)
//   - Hubs    : high-degree files (retrieve.ComputeHubs)
//   - Bridges : articulation points (retrieve.ComputeBridges)
//
// TopN controls the cap on each section independently; the default
// 8 keeps the rendered markdown navigable without drowning the
// caller in tail items. Sections with no content are still emitted
// (with an explanatory Intro) so the view has a stable shape.
func buildSemanticSubgraphData(g *types.Graph, params types.ViewParams) *ViewData {
	topN := params.TopN
	if topN <= 0 {
		topN = 8
	}
	d := &ViewData{
		Type:  "semantic_subgraph",
		Title: "Semantic Subgraphs",
	}

	// Chains.
	chains := retrieve.ComputeChains(g, topN)
	chainsSection := ViewSection{Heading: "Chains (linear import pipelines)"}
	if len(chains) == 0 {
		chainsSection.Intro = "No linear dependency chains of length ≥ 3 found."
	}
	for i, c := range chains {
		chainsSection.Items = append(chainsSection.Items, ViewItem{
			Text: fmt.Sprintf("%d. %s (%d files)", i+1, joinChain(c.Files), len(c.Files)),
			Kind: "chain",
		})
	}
	d.Sections = append(d.Sections, chainsSection)

	// Hubs.
	hubs := retrieve.ComputeHubs(g, topN)
	hubsSection := ViewSection{Heading: "Hubs (high-degree files)"}
	if len(hubs) == 0 {
		hubsSection.Intro = "No files with non-zero import degree."
	}
	for _, h := range hubs {
		hubsSection.Items = append(hubsSection.Items, ViewItem{
			Text: fmt.Sprintf("`%s` — fan-in %d, fan-out %d", h.File, h.FanIn, h.FanOut),
			File: h.File,
			Kind: "hub",
		})
	}
	d.Sections = append(d.Sections, hubsSection)

	// Bridges.
	bridges := retrieve.ComputeBridges(g, topN)
	bridgesSection := ViewSection{Heading: "Bridges (articulation points)"}
	if len(bridges) == 0 {
		bridgesSection.Intro = "No articulation points — removing any single file keeps the graph connected."
	}
	for _, br := range bridges {
		bridgesSection.Items = append(bridgesSection.Items, ViewItem{
			Text: fmt.Sprintf("`%s` — degree %d", br.File, br.Degree),
			File: br.File,
			Kind: "bridge",
		})
	}
	d.Sections = append(d.Sections, bridgesSection)

	return d
}

// joinChain renders a chain's file list with ` → ` arrows, falling
// back to a bracketed truncation when the chain is too long to
// display comfortably.
func joinChain(files []string) string {
	const maxInline = 6
	if len(files) <= maxInline {
		return strings.Join(backtick(files), " → ")
	}
	head := backtick(files[:maxInline-1])
	return strings.Join(head, " → ") + fmt.Sprintf(" → … → `%s`", files[len(files)-1])
}

func backtick(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = "`" + s + "`"
	}
	return out
}

type relationMapSource struct {
	Name string
	File string
	Line int
	Kind string
}

type relationMapRow struct {
	Kind         string
	Source       string
	SourceFile   string
	SourceLine   int
	Target       string
	TargetFile   string
	TargetLine   int
	ObservedFile string
	ObservedLine int
}

// buildRelationMapData renders a model-driven structural relation lens.
//
// The view is navigation only. It reads graph/index structure (calls, imports,
// inheritance, implements, references/type-usage) so a model can choose the
// next small verification window instead of reading broad files. It does not
// decide user intent and it is not citation evidence.
func buildRelationMapData(g *types.Graph, params types.ViewParams) *ViewData {
	topN := params.TopN
	if topN <= 0 {
		topN = 40
	}
	kinds := normalizeRelationMapKinds(params.RelationKinds)
	scopes := normalizeRelationMapScopes(params.Scopes)
	sources := relationMapSources(g, params, scopes, topN)
	rows := relationMapRows(g, sources, scopes, kinds, topN)

	d := &ViewData{
		Type:  "relation_map",
		Title: "Repo Lens: Relation Map",
		Intro: "Advisory structural navigation only. These rows verify graph/navigation facts; use focused `read_file` / `grep` before citing semantic source text.",
	}

	summary := ViewSection{Heading: "Summary"}
	summaryParts := []string{
		fmt.Sprintf("source_candidates=%d", len(sources)),
		fmt.Sprintf("relation_rows=%d", len(rows)),
	}
	if len(scopes) > 0 {
		summaryParts = append(summaryParts, "scopes="+strings.Join(scopes, ","))
	}
	if len(kinds) > 0 {
		summaryParts = append(summaryParts, "relation_kinds="+strings.Join(kinds, ","))
	}
	if strings.TrimSpace(params.Query) != "" {
		summaryParts = append(summaryParts, "query="+params.Query)
	}
	summary.Items = append(summary.Items, ViewItem{Text: strings.Join(summaryParts, " ")})
	d.Sections = append(d.Sections, summary)

	sourceSection := ViewSection{Heading: "Source Candidates (advisory)"}
	if len(sources) == 0 {
		sourceSection.Intro = "No source candidates matched the supplied sources/query/scope. Try a narrower `sources` entry, add a `query`, or use `source_inventory` to locate symbols first."
	} else {
		for _, source := range sources {
			sourceSection.Items = append(sourceSection.Items, ViewItem{
				Text: relationMapSourceText(source),
				File: source.File,
				Kind: source.Kind,
			})
		}
	}
	d.Sections = append(d.Sections, sourceSection)

	rowSection := ViewSection{Heading: "Relation Rows (advisory)"}
	if len(rows) == 0 {
		rowSection.Intro = "No graph-backed relation rows matched the current lens. This is not proof of absence; verify with source reads when absence matters."
	} else {
		for _, row := range rows {
			rowSection.Items = append(rowSection.Items, ViewItem{
				Text: relationMapRowText(row),
				File: row.ObservedFile,
				Kind: row.Kind,
			})
		}
	}
	d.Sections = append(d.Sections, rowSection)

	verify := relationMapVerificationFiles(rows, topN)
	verifySection := ViewSection{Heading: "Suggested Verification Files"}
	if len(verify) == 0 {
		verifySection.Intro = "No concrete files were selected by this relation lens."
	} else {
		for _, file := range verify {
			verifySection.Items = append(verifySection.Items, ViewItem{
				Text: fmt.Sprintf("`%s`", file),
				File: file,
				Kind: "file",
			})
		}
	}
	d.Sections = append(d.Sections, verifySection)
	return d
}

func normalizeRelationMapKinds(raw []string) []string {
	if len(raw) == 0 {
		return []string{"call", "import", "inheritance", "implements", "reference", "type_usage"}
	}
	seen := map[string]bool{}
	var out []string
	for _, item := range raw {
		item = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(item, "_", "-")))
		switch item {
		case "calls", "called-by", "calledby":
			item = "call"
		case "imports", "imported-by", "importedby":
			item = "import"
		case "extends":
			item = "inheritance"
		case "references":
			item = "reference"
		}
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func normalizeRelationMapScopes(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range raw {
		item = strings.Trim(strings.TrimSpace(strings.ReplaceAll(item, `\`, `/`)), "/")
		if item == "" || item == "." || item == "/" {
			item = "."
		}
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func relationMapSources(g *types.Graph, params types.ViewParams, scopes []string, topN int) []relationMapSource {
	if g == nil {
		return nil
	}
	var out []relationMapSource
	seen := map[string]bool{}
	add := func(source relationMapSource) {
		if strings.TrimSpace(source.Name) == "" && strings.TrimSpace(source.File) == "" {
			return
		}
		if source.File != "" && !relationMapFileInScopes(source.File, scopes) {
			return
		}
		key := source.Kind + "\x00" + source.Name + "\x00" + source.File + "\x00" + fmt.Sprintf("%d", source.Line)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, source)
	}
	for _, raw := range params.Sources {
		raw = strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "`")
		if raw == "" {
			continue
		}
		if fi := g.FileIndex[raw]; fi != nil {
			add(relationMapSource{Name: raw, File: raw, Kind: "file"})
			continue
		}
		if relationMapLooksPathOrScope(raw) {
			for _, fi := range relationMapFilesInScope(g, raw) {
				add(relationMapSource{Name: fi.RelPath, File: fi.RelPath, Kind: "file"})
			}
			continue
		}
		for _, sym := range g.SymbolDefs[raw] {
			if sym == nil {
				continue
			}
			add(relationMapSource{Name: sym.Name, File: sym.File, Line: sym.Line, Kind: sym.Kind})
		}
	}
	if len(out) == 0 && strings.TrimSpace(params.Query) != "" {
		tokens := retrieve.TokenizeQuery(params.Query)
		for _, fi := range g.Files {
			if fi == nil || !relationMapFileInScopes(fi.RelPath, scopes) {
				continue
			}
			if relationMapTextMatchesTokens(fi.RelPath+" "+fi.Package, tokens) {
				add(relationMapSource{Name: fi.RelPath, File: fi.RelPath, Kind: "file"})
			}
			for i := range fi.Symbols {
				sym := &fi.Symbols[i]
				if relationMapTextMatchesTokens(sym.Name+" "+sym.Kind+" "+sym.Doc, tokens) {
					add(relationMapSource{Name: sym.Name, File: sym.File, Line: sym.Line, Kind: sym.Kind})
				}
			}
		}
	}
	if len(out) == 0 {
		for _, fi := range relationMapTopRelationFiles(g, scopes, topN) {
			add(relationMapSource{Name: fi.RelPath, File: fi.RelPath, Kind: "file"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Name < out[j].Name
	})
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

func relationMapRows(g *types.Graph, sources []relationMapSource, scopes, kinds []string, topN int) []relationMapRow {
	if g == nil {
		return nil
	}
	allowed := map[string]bool{}
	for _, kind := range kinds {
		allowed[kind] = true
	}
	sourceIndex := relationMapSourceIndex(sources)
	noSources := len(sourceIndex.names) == 0 && len(sourceIndex.files) == 0
	var out []relationMapRow
	seen := map[string]bool{}
	add := func(row relationMapRow) {
		if !relationMapKindAllowed(row.Kind, allowed) {
			return
		}
		if row.ObservedFile != "" && !relationMapFileInScopes(row.ObservedFile, scopes) {
			return
		}
		if !noSources && !relationMapRowTouchesSource(row, sourceIndex) {
			return
		}
		key := strings.Join([]string{row.Kind, row.Source, row.SourceFile, fmt.Sprintf("%d", row.SourceLine), row.Target, row.TargetFile, fmt.Sprintf("%d", row.TargetLine), row.ObservedFile, fmt.Sprintf("%d", row.ObservedLine)}, "\x00")
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, row)
	}
	for _, fi := range g.Files {
		if fi == nil || !relationMapFileInScopes(fi.RelPath, scopes) {
			continue
		}
		for _, target := range g.ImportGraph[fi.RelPath] {
			add(relationMapRow{
				Kind:         "import",
				Source:       fi.RelPath,
				SourceFile:   fi.RelPath,
				Target:       target,
				TargetFile:   target,
				ObservedFile: fi.RelPath,
			})
		}
		for _, rel := range fi.Relations {
			switch rel.Kind {
			case "call":
				src := relationMapEnclosingSymbol(fi, rel.Line)
				target := relationMapCallTarget(g, fi, rel)
				add(relationMapRow{
					Kind:         "call",
					Source:       relationMapSourceName(src, fi.RelPath),
					SourceFile:   relationMapSourceFile(src, fi.RelPath),
					SourceLine:   relationMapSourceLine(src),
					Target:       target.Name,
					TargetFile:   target.File,
					TargetLine:   target.Line,
					ObservedFile: rel.File,
					ObservedLine: rel.Line,
				})
			case "inheritance", "embedding", "reference", "type_usage":
				src := relationMapEnclosingSymbol(fi, rel.Line)
				add(relationMapRow{
					Kind:         rel.Kind,
					Source:       relationMapSourceName(src, fi.RelPath),
					SourceFile:   relationMapSourceFile(src, fi.RelPath),
					SourceLine:   relationMapSourceLine(src),
					Target:       relationMapRelationTargetName(rel),
					TargetFile:   rel.ToEP.File,
					TargetLine:   rel.ToEP.Line,
					ObservedFile: rel.File,
					ObservedLine: rel.Line,
				})
			}
		}
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			for _, ifaceID := range sym.Implements {
				iface := g.SymbolByID[ifaceID]
				if iface == nil {
					continue
				}
				add(relationMapRow{
					Kind:         "implements",
					Source:       sym.Name,
					SourceFile:   sym.File,
					SourceLine:   sym.Line,
					Target:       iface.Name,
					TargetFile:   iface.File,
					TargetLine:   iface.Line,
					ObservedFile: sym.File,
					ObservedLine: sym.Line,
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ObservedFile != out[j].ObservedFile {
			return out[i].ObservedFile < out[j].ObservedFile
		}
		if out[i].ObservedLine != out[j].ObservedLine {
			return out[i].ObservedLine < out[j].ObservedLine
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return relationMapRowText(out[i]) < relationMapRowText(out[j])
	})
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

type relationMapIndex struct {
	names map[string]bool
	files map[string]bool
}

func relationMapSourceIndex(sources []relationMapSource) relationMapIndex {
	idx := relationMapIndex{names: map[string]bool{}, files: map[string]bool{}}
	for _, source := range sources {
		if name := strings.ToLower(strings.TrimSpace(source.Name)); name != "" {
			idx.names[name] = true
		}
		if file := strings.TrimSpace(source.File); file != "" {
			idx.files[file] = true
		}
	}
	return idx
}

func relationMapRowTouchesSource(row relationMapRow, idx relationMapIndex) bool {
	for _, name := range []string{row.Source, row.Target} {
		if idx.names[strings.ToLower(strings.TrimSpace(name))] {
			return true
		}
	}
	for _, file := range []string{row.SourceFile, row.TargetFile, row.ObservedFile} {
		if idx.files[strings.TrimSpace(file)] {
			return true
		}
	}
	return false
}

func relationMapKindAllowed(kind string, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "embedding":
		return allowed["embedding"] || allowed["inheritance"]
	case "type_usage":
		return allowed["type_usage"] || allowed["reference"]
	default:
		return allowed[kind]
	}
}

func relationMapLooksPathOrScope(raw string) bool {
	return strings.Contains(raw, "/")
}

func relationMapFilesInScope(g *types.Graph, scope string) []*types.FileInfo {
	var out []*types.FileInfo
	scope = strings.Trim(strings.TrimSpace(strings.ReplaceAll(scope, `\`, `/`)), "/")
	for _, fi := range g.Files {
		if fi != nil && relationMapFileInScopes(fi.RelPath, []string{scope}) {
			out = append(out, fi)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

func relationMapFileInScopes(file string, scopes []string) bool {
	file = strings.Trim(strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)), "/")
	if len(scopes) == 0 {
		return true
	}
	for _, scope := range scopes {
		scope = strings.Trim(strings.TrimSpace(strings.ReplaceAll(scope, `\`, `/`)), "/")
		if scope == "" || scope == "." {
			return true
		}
		if file == scope || strings.HasPrefix(file, scope+"/") {
			return true
		}
	}
	return false
}

func relationMapTextMatchesTokens(text string, tokens []retrieve.QueryToken) bool {
	text = strings.ToLower(text)
	for _, token := range tokens {
		if token.Text != "" && strings.Contains(text, token.Text) {
			return true
		}
	}
	return false
}

func relationMapTopRelationFiles(g *types.Graph, scopes []string, topN int) []*types.FileInfo {
	type scored struct {
		fi    *types.FileInfo
		score int
	}
	var scoredFiles []scored
	for _, fi := range g.Files {
		if fi == nil || !relationMapFileInScopes(fi.RelPath, scopes) {
			continue
		}
		score := len(fi.Relations) + len(g.ImportGraph[fi.RelPath]) + len(g.ReverseImports[fi.RelPath])
		if score <= 0 {
			continue
		}
		scoredFiles = append(scoredFiles, scored{fi: fi, score: score})
	}
	sort.SliceStable(scoredFiles, func(i, j int) bool {
		if scoredFiles[i].score != scoredFiles[j].score {
			return scoredFiles[i].score > scoredFiles[j].score
		}
		return scoredFiles[i].fi.RelPath < scoredFiles[j].fi.RelPath
	})
	if topN > 0 && len(scoredFiles) > topN {
		scoredFiles = scoredFiles[:topN]
	}
	out := make([]*types.FileInfo, 0, len(scoredFiles))
	for _, item := range scoredFiles {
		out = append(out, item.fi)
	}
	return out
}

func relationMapEnclosingSymbol(fi *types.FileInfo, line int) *types.Symbol {
	if fi == nil || line <= 0 {
		return nil
	}
	var best *types.Symbol
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		if sym.Line <= 0 || sym.Line > line {
			continue
		}
		if best == nil || sym.Line >= best.Line {
			best = sym
		}
	}
	return best
}

func relationMapCallTarget(g *types.Graph, fi *types.FileInfo, rel types.Relation) relationMapSource {
	if g != nil {
		if sym := g.ResolveCallTarget(fi, rel); sym != nil {
			return relationMapSource{Name: sym.Name, File: sym.File, Line: sym.Line, Kind: sym.Kind}
		}
	}
	return relationMapSource{
		Name: relationMapRelationTargetName(rel),
		File: rel.ToEP.File,
		Line: rel.ToEP.Line,
		Kind: "symbol",
	}
}

func relationMapRelationTargetName(rel types.Relation) string {
	if rel.ToEP.Name != "" {
		if rel.ToEP.Receiver != "" {
			return rel.ToEP.Receiver + "." + rel.ToEP.Name
		}
		return rel.ToEP.Name
	}
	if rel.To != "" {
		return rel.To
	}
	return "(unresolved)"
}

func relationMapSourceName(sym *types.Symbol, fallback string) string {
	if sym == nil || strings.TrimSpace(sym.Name) == "" {
		return fallback
	}
	if sym.Parent != "" {
		return sym.Parent + "." + sym.Name
	}
	if sym.Receiver != "" {
		return sym.Receiver + "." + sym.Name
	}
	return sym.Name
}

func relationMapSourceFile(sym *types.Symbol, fallback string) string {
	if sym == nil || strings.TrimSpace(sym.File) == "" {
		return fallback
	}
	return sym.File
}

func relationMapSourceLine(sym *types.Symbol) int {
	if sym == nil {
		return 0
	}
	return sym.Line
}

func relationMapSourceText(source relationMapSource) string {
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = source.File
	}
	parts := []string{"`" + name + "`"}
	if source.Kind != "" {
		parts = append(parts, "kind="+source.Kind)
	}
	if source.File != "" {
		loc := source.File
		if source.Line > 0 {
			loc += fmt.Sprintf(":%d", source.Line)
		}
		parts = append(parts, "@ "+loc)
	}
	return strings.Join(parts, " — ")
}

func relationMapRowText(row relationMapRow) string {
	source := relationMapEndpointText(row.Source, row.SourceFile, row.SourceLine)
	target := relationMapEndpointText(row.Target, row.TargetFile, row.TargetLine)
	observed := strings.TrimSpace(row.ObservedFile)
	if observed != "" && row.ObservedLine > 0 {
		observed += fmt.Sprintf(":%d", row.ObservedLine)
	}
	if observed == "" {
		observed = row.SourceFile
	}
	if observed != "" {
		return fmt.Sprintf("%s `%s` → %s — observed @ %s", row.Kind, source, target, observed)
	}
	return fmt.Sprintf("%s `%s` → %s", row.Kind, source, target)
}

func relationMapEndpointText(name, file string, line int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(file)
	}
	text := name
	if file != "" {
		loc := file
		if line > 0 {
			loc += fmt.Sprintf(":%d", line)
		}
		if text == "" || text == file {
			text = loc
		} else {
			text += " @ " + loc
		}
	}
	if text == "" {
		return "(unknown)"
	}
	return text
}

func relationMapVerificationFiles(rows []relationMapRow, topN int) []string {
	seen := map[string]bool{}
	var out []string
	add := func(file string) {
		file = strings.TrimSpace(file)
		if file == "" || seen[file] {
			return
		}
		seen[file] = true
		out = append(out, file)
	}
	for _, row := range rows {
		add(row.ObservedFile)
		add(row.SourceFile)
		add(row.TargetFile)
	}
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

// buildOverviewData produces the structured form of the overview
// view. It mirrors the legacy viewOverview function's markdown
// output but returns data instead of concatenated strings.
func buildOverviewData(g *types.Graph, params types.ViewParams) *ViewData {
	d := &ViewData{
		Type:  "overview",
		Title: "Repository Overview",
	}
	if params.ShowSourceInventoryHint {
		d.Intro = "Navigation tip: use this overview for architecture/module orientation. For scoped inventories, member lists, counts, or pattern-matching questions, call `repo_map` with `view=\"source_inventory\"` plus `scope`/`scopes` and `roles`; add `attribute_roles` after narrowing to row-local details you need."
	}

	// Languages section — sorted by file count, descending.
	langs := topLanguages(g)
	langSection := ViewSection{Heading: "Languages"}
	for _, lc := range langs {
		langSection.Items = append(langSection.Items, ViewItem{
			Text: fmt.Sprintf("%s: %d files", lc.lang, lc.count),
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
			Text: fmt.Sprintf("%s — %d files, %d symbols", p.name, p.fileCount, p.symCount),
			Kind: "package",
		})
	}
	d.Sections = append(d.Sections, pkgSection)

	// Top files by importance.
	topN := params.TopN
	if topN <= 0 {
		topN = 15
	}
	topFiles := retrieve.TopFiles(g, topN)
	topSection := ViewSection{Heading: fmt.Sprintf("Top %d Files", topN)}
	for _, fi := range topFiles {
		score := g.Scores[fi.RelPath]
		exportedCount := 0
		for _, sym := range fi.Symbols {
			if sym.Exported {
				exportedCount++
			}
		}
		topSection.Items = append(topSection.Items, ViewItem{
			Text:  fmt.Sprintf("`%s` — %d symbols (%d exported)", fi.RelPath, len(fi.Symbols), exportedCount),
			File:  fi.RelPath,
			Score: score,
			Kind:  "top_file",
		})
	}
	d.Sections = append(d.Sections, topSection)
	return d
}

type langCount struct {
	lang  string
	count int
}

func topLanguages(g *types.Graph) []langCount {
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

func topPackages(g *types.Graph) []packageSummary {
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

// buildFileMapData produces the structured form of the file_map
// view. Each of the top-N files becomes its own ViewSection whose
// heading is `<relpath> [<package>]` and whose items are the
// file's symbols grouped by kind in a canonical order. The
// rendered markdown preserves the legacy shape so existing
// consumers see the same landmarks.
func buildFileMapData(g *types.Graph, params types.ViewParams) *ViewData {
	topN := params.TopN
	if topN <= 0 {
		topN = 50
	}
	files := retrieve.TopFiles(g, topN)

	kindOrder := []string{
		"interface", "trait", "class", "struct", "enum",
		"type", "function", "method", "const", "var", "field",
	}

	d := &ViewData{
		Type:  "file_map",
		Title: "File Map",
	}

	for _, fi := range files {
		if len(fi.Symbols) == 0 {
			continue
		}
		heading := fi.RelPath
		if fi.Package != "" {
			heading += " [" + fi.Package + "]"
		}
		section := ViewSection{Heading: heading}

		// Group by kind so the canonical ordering can walk them
		// without re-scanning the slice for every kind.
		groups := make(map[string][]types.Symbol)
		for _, sym := range fi.Symbols {
			groups[sym.Kind] = append(groups[sym.Kind], sym)
		}

		for _, kind := range kindOrder {
			syms, ok := groups[kind]
			if !ok {
				continue
			}
			for _, sym := range syms {
				marker := " "
				if sym.Exported {
					marker = "+"
				}
				line := marker + "`" + sym.Name + "`"
				if sym.Receiver != "" {
					line = marker + "`(" + sym.Receiver + ") " + sym.Name + "`"
				} else if sym.Parent != "" {
					line = marker + "`" + sym.Parent + "." + sym.Name + "`"
				}
				line += fmt.Sprintf(" %s :%d", sym.Kind, sym.Line)
				if sym.Signature != "" {
					line += " `" + sym.Signature + "`"
				}
				if sym.Doc != "" {
					line += " — " + sym.Doc
				}
				section.Items = append(section.Items, ViewItem{
					Text: line,
					File: fi.RelPath,
					Kind: sym.Kind,
				})
			}
		}
		d.Sections = append(d.Sections, section)
	}

	return d
}

// buildCallPathData produces the structured form of the call_path
// view: a single-section BFS walk of the ImportGraph from the
// requested entry point (or the highest-scored file when
// EntryPoint is empty). Each visited file becomes a ViewItem
// whose Depth matches the BFS depth so RenderMarkdown indents it
// correctly. Key symbols on each file are appended to the item
// text as a `→ sym1, sym2, ...` suffix.
func buildCallPathData(g *types.Graph, params types.ViewParams) *ViewData {
	entry := params.EntryPoint
	if entry == "" {
		if top := retrieve.TopFiles(g, 1); len(top) > 0 {
			entry = top[0].RelPath
		}
	}

	d := &ViewData{
		Type:  "call_path",
		Title: "Call Path from " + entry,
	}
	walk := ViewSection{}

	visited := make(map[string]bool)
	type queueItem struct {
		file  string
		depth int
	}
	queue := []queueItem{{entry, 0}}
	const maxDepth = 5

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if visited[item.file] || item.depth > maxDepth {
			continue
		}
		visited[item.file] = true

		fi := g.FileIndex[item.file]
		if fi == nil {
			walk.Items = append(walk.Items, ViewItem{
				Text:  "`" + item.file + "` (not in index)",
				File:  item.file,
				Depth: item.depth,
			})
			continue
		}

		var keySyms []string
		for _, sym := range fi.Symbols {
			if sym.Exported && (sym.Kind == "function" || sym.Kind == "method") {
				keySyms = append(keySyms, sym.Name)
			}
		}
		suffix := ""
		if len(keySyms) > 0 {
			suffix = " → " + strings.Join(abbreviate(keySyms, 5), ", ")
		}
		walk.Items = append(walk.Items, ViewItem{
			Text:  "`" + item.file + "`" + suffix,
			File:  item.file,
			Depth: item.depth,
		})
		for _, dep := range g.ImportGraph[item.file] {
			queue = append(queue, queueItem{dep, item.depth + 1})
		}
	}

	d.Sections = append(d.Sections, walk)
	return d
}

// buildEditImpactData produces the structured form of the
// edit_impact view. Emits up to four sections depending on what
// the target file has: Direct Dependents, Transitive Dependents
// (only when strictly more than direct), Exported Symbols, and
// Dependencies. Target-file-not-found falls back to a single
// empty section so the rendered markdown still has the expected
// "# Edit Impact: <target>" headline.
func buildEditImpactData(g *types.Graph, params types.ViewParams) *ViewData {
	target := params.TargetFile
	if target == "" {
		return &ViewData{
			Type:  "edit_impact",
			Title: "Edit Impact",
			Intro: "No target file specified.",
		}
	}
	d := &ViewData{
		Type:  "edit_impact",
		Title: "Edit Impact: " + target,
	}

	fi := g.FileIndex[target]
	if fi == nil {
		d.Sections = append(d.Sections, ViewSection{
			Intro: "File not found in index.",
		})
		return d
	}

	// Direct dependents.
	directDeps := g.FilesImporting(target)
	directSection := ViewSection{Heading: "Direct Dependents"}
	if len(directDeps) == 0 {
		directSection.Intro = "No files directly import this file."
	}
	for _, dep := range directDeps {
		directSection.Items = append(directSection.Items, ViewItem{
			Text: "`" + dep + "`",
			File: dep,
			Kind: "direct_dep",
		})
	}
	d.Sections = append(d.Sections, directSection)

	// Transitive dependents (only when strictly more than direct).
	transitive := g.TransitiveReverseDeps(target, 5)
	if len(transitive) > len(directDeps) {
		trans := ViewSection{Heading: "Transitive Dependents (up to 5 levels)"}
		directSet := make(map[string]bool, len(directDeps))
		for _, d := range directDeps {
			directSet[d] = true
		}
		for _, dep := range transitive {
			if directSet[dep] {
				continue
			}
			trans.Items = append(trans.Items, ViewItem{
				Text: "`" + dep + "`",
				File: dep,
				Kind: "transitive_dep",
			})
		}
		d.Sections = append(d.Sections, trans)
	}

	// Exported symbols with caller counts.
	exports := ViewSection{Heading: "Exported Symbols"}
	for _, sym := range fi.Symbols {
		if !sym.Exported {
			continue
		}
		refs := len(g.CallersOf(sym.Name))
		suffix := ""
		if refs > 0 {
			suffix = fmt.Sprintf(" (referenced from %d files)", refs)
		}
		exports.Items = append(exports.Items, ViewItem{
			Text: fmt.Sprintf("`%s` %s%s", sym.Name, sym.Kind, suffix),
			File: target,
			Kind: sym.Kind,
		})
	}
	d.Sections = append(d.Sections, exports)

	// Files this file depends on.
	if deps := g.FilesImportedBy(target); len(deps) > 0 {
		depsSection := ViewSection{Heading: "Dependencies (this file imports)"}
		for _, dep := range deps {
			depsSection.Items = append(depsSection.Items, ViewItem{
				Text: "`" + dep + "`",
				File: dep,
				Kind: "imports",
			})
		}
		d.Sections = append(d.Sections, depsSection)
	}

	return d
}

// buildTaskMapData produces the structured form of the task_map
// view. Runs a query-biased retrieve.RankGraphScores pass, then emits one "Relevant
// Files" section with one ViewSection per file and matched
// symbols as Items on that file's subsection. Matched-symbol
// filtering uses the same retrieve.TokenizeQuery tokens that drive
// rank.go's queryMatchScore, so the shown symbols are exactly
// the ones that contributed to the file's rank score.
//
// The legacy view appended "imports: …" and "imported by: …"
// lines directly under each file block as indented bullets.
// ViewItem is a flat list shape, so these become plain items
// at the end of the file's Items slice with the same visible
// prefix, giving identical rendered markdown.
func buildTaskMapData(g *types.Graph, params types.ViewParams) *ViewData {
	ranking := retrieve.RankGraphScores(g, params.Query)

	topN := params.TopN
	if topN <= 0 {
		topN = 20
	}
	relevant := retrieve.TopFilesByScore(g, ranking.Scores, topN)

	// Primary-weight tokens only: sub-tokens would over-match the
	// rendered "matched symbols" list and clutter the human view
	// without adding information beyond what the file score
	// already encodes.
	tokens := retrieve.TokenizeQuery(params.Query)
	primary := tokens[:0]
	for _, t := range tokens {
		if t.Weight >= 1.0 {
			primary = append(primary, t)
		}
	}

	body := ViewSection{Heading: "Relevant Files"}
	for _, fi := range relevant {
		score := ranking.Scores[fi.RelPath]
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
		Type:     "task_map",
		Title:    "Task Map: " + params.Query,
		Query:    params.Query,
		Sections: []ViewSection{body},
	}
}
