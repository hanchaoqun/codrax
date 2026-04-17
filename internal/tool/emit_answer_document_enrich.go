package tool

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_answer_document_enrich.go — session-8 render-only augmentations.
// The LLM's emit_answer_document call lands the structured answer
// (summary, citations, steps, etc.), and these helpers run
// immediately afterward to populate two fields the LLM never writes:
//
//   - Snippets: up to N short code excerpts extracted from the
//     read_file gutter index at each cited file:line. Adjacent
//     citations in the same file collapse into one snippet so
//     cite(:63) + cite(:65) render as a single 6-line block,
//     not two near-duplicates.
//
//   - RelationDiagram: a vertical ASCII flow (A → B → C) built
//     from evidence items whose predicate is a relationship verb
//     (binds / calls / registers / returns). Empty when no coherent
//     chain exists; rendered as-is when it does.
//
// Both fields are deterministic over ctx.Mutable state and are
// therefore independent of LLM wording — they survive re-runs and
// answer cache invalidations without drift.

// snippetContextLines is the symmetric line window around each
// citation line. 2 before + 2 after gives the reader enough context
// to recognize the code without burying the cited line in noise.
const snippetContextLines = 2

// snippetMaxLines caps the total line span of one rendered snippet
// block so even a tightly-clustered cite pool doesn't overwhelm the
// answer. Adjacent citations collapse into one snippet only when
// the merged span stays under this cap.
const snippetMaxLines = 12

// extractCodeSnippets returns up to maxN code excerpts, one per
// (file, line-range) cluster, derived from the citations pool. Each
// snippet pulls ±snippetContextLines around the citation line from
// the read_file gutter index; citations in the same file whose line
// windows overlap or abut are merged into a single range. Citations
// whose file was never read (no gutter entry) are skipped — the
// snippet would have no real text.
//
// Ordering mirrors the citation pool, so the first snippet maps to
// the most important citation.
func extractCodeSnippets(ctx *types.BusContext, doc *types.AnswerDocument, maxN int) []types.CodeSnippet {
	if ctx == nil || doc == nil || len(doc.Citations) == 0 || maxN <= 0 {
		return nil
	}
	gc := ground.BuildContext(ctx)
	if gc == nil || len(gc.LineIndex) == 0 {
		return nil
	}

	type rangeEntry struct {
		file      string
		startLine int
		endLine   int
		order     int // position in doc.Citations for stable ordering
	}

	// Collect one window per citation. Stable by citation order.
	windows := make([]rangeEntry, 0, len(doc.Citations))
	for i, c := range doc.Citations {
		if _, ok := gc.LineIndex[c.File]; !ok {
			continue
		}
		start := c.Line - snippetContextLines
		if start < 1 {
			start = 1
		}
		end := c.Line + snippetContextLines
		windows = append(windows, rangeEntry{
			file:      c.File,
			startLine: start,
			endLine:   end,
			order:     i,
		})
	}
	if len(windows) == 0 {
		return nil
	}

	// Cluster same-file adjacent windows. Sort by (file, start) so
	// merges can fold consecutively; after merging, re-sort by the
	// earliest citation order of each cluster so the output mirrors
	// citation priority.
	sort.SliceStable(windows, func(i, j int) bool {
		if windows[i].file != windows[j].file {
			return windows[i].file < windows[j].file
		}
		return windows[i].startLine < windows[j].startLine
	})

	type cluster struct {
		file      string
		startLine int
		endLine   int
		firstOrd  int
	}
	clusters := make([]cluster, 0, len(windows))
	for _, w := range windows {
		if len(clusters) > 0 {
			last := &clusters[len(clusters)-1]
			if last.file == w.file && w.startLine <= last.endLine+1 &&
				w.endLine-last.startLine+1 <= snippetMaxLines {
				if w.endLine > last.endLine {
					last.endLine = w.endLine
				}
				if w.order < last.firstOrd {
					last.firstOrd = w.order
				}
				continue
			}
		}
		clusters = append(clusters, cluster{
			file:      w.file,
			startLine: w.startLine,
			endLine:   w.endLine,
			firstOrd:  w.order,
		})
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		return clusters[i].firstOrd < clusters[j].firstOrd
	})

	out := make([]types.CodeSnippet, 0, maxN)
	for _, cl := range clusters {
		if len(out) >= maxN {
			break
		}
		code := renderSnippetBody(gc.LineIndex[cl.file], cl.startLine, cl.endLine)
		if strings.TrimSpace(code) == "" {
			continue
		}
		out = append(out, types.CodeSnippet{
			File:      cl.file,
			StartLine: cl.startLine,
			EndLine:   cl.endLine,
			Language:  languageFromPath(cl.file),
			Code:      code,
		})
	}
	return out
}

// renderSnippetBody pulls the given line range out of the gutter
// index and returns a gutter-free, \n-joined string. Missing lines
// (outside the read range) are rendered as empty so the snippet
// stays contiguous in display even when the read window didn't
// cover the full ±2.
func renderSnippetBody(fileLines map[int]string, start, end int) string {
	if fileLines == nil {
		return ""
	}
	var b strings.Builder
	for ln := start; ln <= end; ln++ {
		text, ok := fileLines[ln]
		if !ok {
			// Hit the edge of the read window — stop rather than
			// pad with blank lines that misrepresent the file.
			break
		}
		b.WriteString(text)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// languageFromPath maps a repo-relative file extension to a
// language tag for fenced code blocks. Covers the ten languages
// codrax's keyword/grep path routinely indexes; unknown extensions
// return "" so the renderer emits an untagged fence.
func languageFromPath(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".sh":
		return "sh"
	case ".md":
		return "markdown"
	}
	return ""
}

// relationshipPredicates is the allowlist of evidence predicates
// worth rendering as relationship edges in the diagram. Drawn from
// the concrete_values extractor's predicate vocabulary. Anything
// outside this set (prose "assigns", "references") is skipped —
// those relationships are too vague to visualize cleanly.
var relationshipPredicates = map[string]string{
	"calls":        "calls",
	"binds":        "binds",
	"binds ONLY":   "binds",
	"registers":    "registers",
	"returns":      "returns",
	"defines":      "defines",
	"delegates to": "→",
	"implements":   "implements",
	"embeds":       "embeds",
}

// diagramNode carries the display text for one node in the relation
// graph plus its adjacency to the next node in the chain.
type diagramNode struct {
	label string
	edge  string // edge label to the NEXT node ("calls", "binds", ...)
}

// buildRelationDiagram builds a vertical ASCII flow from the
// evidence buffer. Two-tier algorithm:
//
//   Tier 1 (preferred): evidence items already carry a pre-built
//   resolution chain in their Summary field
//   (Kind=EvidenceDataflowPath, Predicate="resolution_chain").
//   These are produced by the explorer's concrete_values extractor
//   and reflect a genuine cross-file data flow like
//   "`RegisterDefaultSubAgents()` binds NewSubExplorer(deps) →
//   `SubExplorer.Run()` assigns ...". Parsing this directly gives
//   the diagram an authoritative chain without rebuilding the
//   graph from raw edges.
//
//   Tier 2 (fallback): walk the edge graph (Subject, Predicate,
//   Object) as before. Used when no resolution_chain evidence
//   exists (sub-agent stage, evidence-lite pipelines, test paths).
//
// Returns "" when neither tier produces a chain of length ≥ 2.
// Linear-chain only — branches drop because ASCII diamonds hurt
// more than they help.
func buildRelationDiagram(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	evidence := ctx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		return ""
	}

	// Tier 1 — resolution_chain evidence. Pick the first item whose
	// summary parses into ≥ 2 nodes; that is already a curated
	// cross-file walk and is strictly better than re-discovering
	// the chain from (S, P, O) triples.
	for _, e := range evidence {
		if e.Kind != types.EvidenceDataflowPath || e.Predicate != "resolution_chain" {
			continue
		}
		chain := parseResolutionChainSummary(e.Summary)
		if len(chain) >= 2 {
			return renderASCIIChain(chain)
		}
	}
	// Edge list: subject -[edge]-> object. Only keep the first
	// occurrence of each (subject, object) tuple so accidental
	// dupes don't stretch the chain artificially.
	type edge struct{ from, to, label string }
	edges := make([]edge, 0, len(evidence))
	seen := make(map[string]bool)
	for _, e := range evidence {
		label, ok := relationshipPredicates[strings.TrimSpace(strings.ToLower(e.Predicate))]
		if !ok {
			// Try raw (case-preserved) lookup — the predicate
			// table is canonical-case but some producers emit
			// camel-case strings.
			if label2, ok2 := relationshipPredicates[strings.TrimSpace(e.Predicate)]; ok2 {
				label = label2
				ok = true
			}
		}
		if !ok {
			continue
		}
		from := strings.TrimSpace(e.Subject)
		to := strings.TrimSpace(e.Object)
		if from == "" || to == "" || from == to {
			continue
		}
		key := from + "\x1f" + to
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, edge{from: from, to: to, label: label})
	}
	if len(edges) < 1 {
		return ""
	}

	// Build adjacency + in-degree. Pick the starting node with the
	// highest out-degree that has the lowest in-degree (preferring
	// a root over a mid-chain node).
	outAdj := make(map[string][]edge)
	inDeg := make(map[string]int)
	for _, e := range edges {
		outAdj[e.from] = append(outAdj[e.from], e)
		inDeg[e.to]++
		if _, ok := inDeg[e.from]; !ok {
			inDeg[e.from] = 0
		}
	}
	var root string
	for node := range outAdj {
		if inDeg[node] != 0 {
			continue
		}
		if root == "" || len(outAdj[node]) > len(outAdj[root]) {
			root = node
		}
	}
	if root == "" {
		// Cyclic graph — just start from the first edge's source.
		root = edges[0].from
	}

	// Walk linearly, preferring the first successor at each step.
	// Cap at 6 nodes to keep the diagram readable; drop branches.
	const maxNodes = 6
	visited := make(map[string]bool)
	chain := []diagramNode{{label: root}}
	visited[root] = true
	cur := root
	for len(chain) < maxNodes {
		nexts := outAdj[cur]
		if len(nexts) == 0 {
			break
		}
		var chosen *edge
		for i := range nexts {
			if !visited[nexts[i].to] {
				chosen = &nexts[i]
				break
			}
		}
		if chosen == nil {
			break
		}
		chain[len(chain)-1].edge = chosen.label
		chain = append(chain, diagramNode{label: chosen.to})
		visited[chosen.to] = true
		cur = chosen.to
	}
	if len(chain) < 2 {
		return ""
	}
	return renderASCIIChain(chain)
}

// parseResolutionChainSummary parses a concrete_values-produced
// resolution chain summary into diagramNode slice. The producer's
// format is a sequence of segments joined by " → " where each
// segment has shape "`FunctionName()` verb argument" — the
// function name is backticked, verb is a single lowercase word
// ("binds", "returns", "calls", "assigns"), argument is whatever
// follows until the next " → " or end-of-string.
//
// Example input:
//
//	`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Run()` assigns sk := &skill.Config{
//
// Produces nodes:
//
//	[RegisterDefaultSubAgents] --binds--> [SubExplorer.Run] --assigns--> [sk := &skill.Config{ (terminal)]
//
// The terminal argument of the LAST segment becomes the final
// node so the reader sees where the chain ends. Chains that don't
// parse cleanly (missing backticks / no segments) return nil and
// the caller falls through to Tier 2.
func parseResolutionChainSummary(summary string) []diagramNode {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	segments := strings.Split(summary, " → ")
	if len(segments) < 2 {
		return nil
	}
	nodes := make([]diagramNode, 0, len(segments)+1)
	for i, seg := range segments {
		seg = strings.TrimSpace(seg)
		// Expect: `<name>()` <verb> <rest...>
		//       or  <name> <verb> <rest...>   (non-backticked producers)
		name, verb, rest := splitChainSegment(seg)
		if name == "" {
			return nil
		}
		nodes = append(nodes, diagramNode{label: name, edge: verb})
		// For the final segment, also append the terminal value
		// node (the "rest" — a literal or expression the chain
		// resolves TO). Skip when empty or when it duplicates the
		// previous name.
		if i == len(segments)-1 {
			rest = strings.TrimSpace(rest)
			if len(rest) > 60 {
				rest = rest[:60] + "…"
			}
			if rest != "" && rest != name {
				nodes = append(nodes, diagramNode{label: rest})
			}
		}
	}
	// The edge of the last node is unused (renderASCIIChain only
	// draws N-1 edges for N nodes). Leave it as-is.
	return nodes
}

// splitChainSegment pulls (name, verb, rest) out of a single
// segment. Accepts three producer shapes:
//
//   `Name()` verb rest…
//   `Name`    verb rest…
//    Name     verb rest…
//
// Returns ("", "", "") on unparseable input.
func splitChainSegment(seg string) (name, verb, rest string) {
	// Strip leading backtick-wrapped name if present.
	if strings.HasPrefix(seg, "`") {
		if end := strings.Index(seg[1:], "`"); end > 0 {
			name = strings.TrimSpace(seg[1 : 1+end])
			// Strip trailing "()" — diagram labels read cleaner
			// without the empty call parens.
			name = strings.TrimSuffix(name, "()")
			tail := strings.TrimSpace(seg[1+end+1:])
			verb, rest = splitVerbRest(tail)
			return name, verb, rest
		}
	}
	// Non-backticked: take the first word as the name.
	first, tail := cutFirstWord(seg)
	if first == "" {
		return "", "", ""
	}
	name = strings.TrimSuffix(first, "()")
	verb, rest = splitVerbRest(tail)
	return name, verb, rest
}

// splitVerbRest interprets the post-name tail as "verb rest…".
// Handles multi-word verbs the concrete_values producer emits
// ("binds ONLY", "delegates to") by peeking one extra word when
// the first word is a known verb root. Bare single-word fallback
// otherwise.
func splitVerbRest(tail string) (verb, rest string) {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return "", ""
	}
	first, remainder := cutFirstWord(tail)
	lower := strings.ToLower(first)
	// Two-word verb forms the producer uses.
	if lower == "binds" || lower == "delegates" {
		second, r2 := cutFirstWord(remainder)
		sLower := strings.ToLower(second)
		if sLower == "only" || sLower == "to" || sLower == "first" {
			return first + " " + second, r2
		}
	}
	return first, remainder
}

// cutFirstWord returns (firstWord, rest) on the leading space
// boundary. Leading whitespace is stripped; the rest preserves
// original internal whitespace.
func cutFirstWord(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", ""
	}
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimLeft(s[idx:], " \t")
}

// renderASCIIChain prints the chain as a centered vertical flow:
//
//	  RegisterDefaultSubAgents
//	           │ binds
//	           ▼
//	     NewSubExplorer
//	           │ returns
//	           ▼
//	      SubExplorer.Run
//
// Width is the longest label + small padding; each node centered
// relative to that width. Edges use box-drawing characters for a
// clean render in monospace terminals.
func renderASCIIChain(chain []diagramNode) string {
	if len(chain) < 2 {
		return ""
	}
	maxLabel := 0
	for _, n := range chain {
		if l := displayWidth(n.label); l > maxLabel {
			maxLabel = l
		}
	}
	// Pad to at least 10 so short labels don't render flush-left.
	if maxLabel < 10 {
		maxLabel = 10
	}
	var b strings.Builder
	for i, n := range chain {
		// Center the label within maxLabel columns.
		padLeft := (maxLabel - displayWidth(n.label)) / 2
		fmt.Fprintf(&b, "%s%s\n", strings.Repeat(" ", padLeft), n.label)
		if i < len(chain)-1 {
			barCol := strings.Repeat(" ", maxLabel/2)
			edge := n.edge
			if edge == "" {
				edge = "→"
			}
			fmt.Fprintf(&b, "%s│ %s\n", barCol, edge)
			fmt.Fprintf(&b, "%s▼\n", barCol)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// displayWidth approximates visual width of a label for centering.
// Treats non-ASCII runes as double-width (CJK characters in
// identifiers, emojis), ASCII as single-width. Good enough for
// monospace terminal centering without pulling in a unicode-width
// dependency.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r < 0x7f {
			w++
		} else {
			w += 2
		}
	}
	return w
}

// parseResolutionChainSummary + splitChainSegment + splitVerbRest +
// cutFirstWord have the contract documented above. The tests
// exercising them live alongside buildRelationDiagram in
// emit_answer_document_test.go's Tier-1 parsing cases.

