// Package ground validates the file:line citation attached to each
// LLM-emitted EvidenceItem and assigns a three-way GroundingStatus
// (grounded / recovered / ungrounded). It is called synchronously from
// emit_evidence.Execute so the LLM sees the verdict for each item in
// the same turn it emitted them — no "emit, stop, inspect, re-emit"
// round trip.
//
// Grounding runs multiple tiers. Tier 1 (line_text) reads the
// read_file gutter reconstructed from tool history and checks whether
// the AnchorSymbol shows up as a whole-word token on the cited line
// or its immediate neighbours. Tier 2 (symbol_table) dispatches on
// AnchorKind and consults the repomap Graph for a structural match
// (definition line, call-site line, import line). Recovery tiers
// R1-R5 (shipped in Step 11) try to repair a near-miss by rewriting
// LineStart/Source. Items that fail every tier are labelled
// ungrounded and preserved as "leads" — they stay in the evidence
// stream but must not be emitted as citations downstream.
package ground

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// gutterLineRe matches the per-line gutter emitted by
// tool.renderWithLineGutter: optional spaces, 1-6 digits, U+2502,
// single space. The captured groups are the line number and the
// line text.
var gutterLineRe = regexp.MustCompile(`^\s*(\d{1,6})│ (.*)$`)

// identifierTokenRe extracts identifier-shaped tokens of length ≥3.
var identifierTokenRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// Context carries the per-dispatch inputs the grounding tiers read:
// the reconstructed line index from every read_file in this Run's
// history, and the repomap Graph explorer.keywordSearch built. Both
// are cheap to consult repeatedly — the line index is a map, the
// graph is a pointer already resident in memory.
type Context struct {
	LineIndex map[string]map[int]string // source path → (1-based line → text)
	Graph     *repomap.Graph            // nil when explorer has not populated MutableState yet
}

// BuildContext assembles a grounding Context from the caller's
// BusContext. Pulls the read_file gutter index from the union of
// (a) the per-dispatch running buffer Mutable.DispatchToolResults
// — so tools that fire from inside the current ReAct loop see the
// read_file results that earlier iterations of the SAME loop
// produced — and (b) ctx.ToolResults, which carries prior-stage
// history already flushed into BusContext by applyStageOutput.
// Graph handle comes from Mutable.SearchGraph. All sources are
// nil-tolerant.
func BuildContext(ctx *types.BusContext) *Context {
	gc := &Context{}
	if ctx == nil {
		return gc
	}
	// Prior-stage history first, then layer in the current dispatch
	// so in-dispatch reads win on conflicting line numbers (the LLM
	// may have re-read a file with a different offset/limit this turn).
	var history []types.ToolResult
	if len(ctx.ToolResults) > 0 {
		history = append(history, ctx.ToolResults...)
	}
	if ctx.Mutable != nil {
		if disp := ctx.Mutable.DispatchToolResults(); len(disp) > 0 {
			history = append(history, disp...)
		}
	}
	gc.LineIndex = buildLineIndex(history)
	if ctx.Mutable != nil {
		if g, ok := ctx.Mutable.SearchGraph().(*repomap.Graph); ok {
			gc.Graph = g
		}
	}
	return gc
}

// Report is the per-item feedback the emit_evidence tool renders back
// to the LLM as part of its ToolResult.Summary.
type Report struct {
	ItemID       string                // EvidenceItem.ID after any recovery rewrite
	Status       types.GroundingStatus // copy of item.GroundingStatus for the renderer
	Tier         types.GroundingTier   // which tier decided (empty when Status=ungrounded)
	OriginalLine int                   // LLM's claimed LineStart; useful when a recovery tier rewrote it
	AdjustedLine int                   // final LineStart after recovery (same as OriginalLine when no rewrite)
	Note         string                // free-text explanation: recovery detail or ungrounded reason
}

// GroundItem runs the tier cascade against one EvidenceItem and
// mutates it in place: GroundingStatus / GroundingTier / GroundingNote
// are always set, and recovery tiers may rewrite LineStart / Source /
// ID. Returns a Report for feedback rendering.
//
// The tier order is fixed:
//
//  1. Tier 1 — line_text: evidence's AnchorSymbol appears as a
//     whole-word token in the read_file gutter for Source at
//     LineStart ±2. This is the strongest signal because it confirms
//     the LLM actually cited content it could see.
//
//  2. Tier 2 — symbol_table: repomap-backed structural match,
//     dispatched on AnchorKind:
//       - definition → SymbolsInFile + sym.Line == LineStart
//       - call       → FileIndex[Source].Relations where
//                      Kind=="call" + ToEP.Name == AnchorSymbol
//                      + Line == LineStart
//       - import     → FileInfo.Imports + imp.Path/Alias match +
//                      imp.Line == LineStart
//       - condition/return/assignment → no repomap primitive, degrade
//                      to line-text scan for the kind's keyword set
//                      (if/when/unless/switch/case/guard for
//                      condition; return/yield for return; := or
//                      standalone = for assignment)
//
//  3. Recovery R1-R5 — implemented in Step 11 of the redesign.
//     Placeholder here so GroundItem can be wired end-to-end first.
//
// If every tier fails, GroundingStatus is set to Ungrounded, the
// Note describes what the grounder looked for, and LineStart is
// preserved (it remains the LLM's claim — a "lead" the finalizer
// can reference in caveats but must not cite).
func GroundItem(it *types.EvidenceItem, gc *Context) Report {
	if it == nil {
		return Report{}
	}
	originalLine := it.LineStart

	// Tier 1: line_text via read_file gutter.
	if tier1LineText(it, gc) {
		it.GroundingStatus = types.GroundingGrounded
		it.GroundingTier = types.TierLineText
		return Report{
			ItemID: it.ID, Status: it.GroundingStatus, Tier: it.GroundingTier,
			OriginalLine: originalLine, AdjustedLine: it.LineStart,
		}
	}

	// Tier 2: symbol_table via repomap dispatch.
	if tier2SymbolTable(it, gc) {
		it.GroundingStatus = types.GroundingGrounded
		it.GroundingTier = types.TierSymbolTable
		return Report{
			ItemID: it.ID, Status: it.GroundingStatus, Tier: it.GroundingTier,
			OriginalLine: originalLine, AdjustedLine: it.LineStart,
		}
	}

	// Recovery tiers. Each may rewrite LineStart / Source; on success
	// GroundingStatus becomes Recovered and GroundingTier names the
	// specific tier. The order is chosen so the cheapest / most
	// precise recoveries run first:
	//
	//  R1 fqname_same_file     — graph lookup in the cited Source
	//  R2 snippet_fuzzy        — needs Snippet, scans ±15 lines
	//  R3 package_symbol       — graph lookup across same-package files
	//  R4 nearest_call         — AnchorKind=call-specific scan
	//  R5 nearest_condition    — AnchorKind=condition-specific scan
	for _, attempt := range recoveryTiers {
		if newSource, newLine, ok := attempt.fn(it, gc); ok {
			if newSource != "" {
				it.Source = newSource
			}
			if newLine > 0 {
				it.LineStart = newLine
				if it.LineEnd > 0 && it.LineEnd < newLine {
					it.LineEnd = newLine
				}
			}
			it.GroundingStatus = types.GroundingRecovered
			it.GroundingTier = attempt.tier
			note := describeRecovery(attempt.tier, originalLine, newLine)
			it.GroundingNote = note
			return Report{
				ItemID: it.ID, Status: it.GroundingStatus, Tier: it.GroundingTier,
				OriginalLine: originalLine, AdjustedLine: it.LineStart,
				Note: note,
			}
		}
	}

	// Fail-through: mark ungrounded with a concrete reason so the
	// LLM can decide how to repair (read the file, change
	// anchor_symbol, or drop the claim as speculative).
	it.GroundingStatus = types.GroundingUngrounded
	it.GroundingTier = ""
	it.GroundingNote = explainUngrounded(it, gc)
	return Report{
		ItemID: it.ID, Status: it.GroundingStatus,
		OriginalLine: originalLine, AdjustedLine: it.LineStart,
		Note: it.GroundingNote,
	}
}

// recoveryAttempt pairs a tier name with its implementation so
// GroundItem's loop stays declarative and new tiers can be slotted
// in without re-ordering boilerplate.
type recoveryAttempt struct {
	tier types.GroundingTier
	// fn returns (newSource, newLine, ok). newSource "" means keep
	// current; newLine <= 0 means keep current. ok=true signals the
	// tier accepted the item.
	fn func(it *types.EvidenceItem, gc *Context) (string, int, bool)
}

var recoveryTiers = []recoveryAttempt{
	{types.TierFQNameSameFile, recoverFQNameSameFile},
	{types.TierSnippetFuzzy, recoverSnippetFuzzy},
	{types.TierPackageSymbol, recoverPackageSymbol},
	{types.TierNearestCall, recoverNearestCall},
	{types.TierNearestCondition, recoverNearestCondition},
}

func describeRecovery(tier types.GroundingTier, original, adjusted int) string {
	if original == 0 || adjusted == 0 || original == adjusted {
		return "recovered via " + string(tier)
	}
	return fmt.Sprintf("recovered via %s (LLM claimed line %d, adjusted to %d)", tier, original, adjusted)
}

// ── Citation grounding (emit_answer_document) ───────────────────────
//
// Citations are a leaner shape than EvidenceItems — just (File, Line,
// Quote). They have no AnchorKind / AnchorSymbol, so the full 7-tier
// evidence grounder does not apply. CitationReport classifies a
// citation into three outcomes:
//
//   Valid + QuoteMatched : (File, Line) appears in the read_file gutter
//                          AND the quote's identifier tokens overlap
//                          with the cited line (± neighbours). The
//                          strongest claim — LLM actually saw the text.
//
//   Valid + !QuoteMatched: (File, Line) exists (either in the gutter
//                          without a readable quote, or in the repomap
//                          graph as a symbol range) but the Quote does
//                          not corroborate. Callers typically clear
//                          the Quote so the rendered cite is honest
//                          "file:line" without a fabricated excerpt.
//
//   !Valid               : neither gutter nor graph contains (File, Line).
//                          Callers drop the citation; the emit tool
//                          remaps any CitationRef pointing at it to
//                          CitationRefUnset (-1).

// CitationReport is the verdict returned by GroundCitation. The
// emit_answer_document tool branches on Valid / QuoteMatched / Tier to
// decide drop-vs-clear-vs-keep.
type CitationReport struct {
	Valid        bool
	QuoteMatched bool
	// Tier names the tier that accepted the citation (empty when
	// !Valid). "line_text" means the LLM actually read the file and
	// the cited line is in the read_file gutter index — the strongest
	// proof. "symbol_table" means the line is in a structural region
	// of an indexed file but not in the gutter index. Callers use
	// the tier to enforce the "at least one Tier 1 proven peer" rule
	// when a citation's quote was cleared as fabricated.
	Tier types.GroundingTier
	// Reason is populated on !Valid with a short human-readable
	// diagnostic ("source not read and not in graph", "line out of
	// any symbol range", …) that the tool surfaces in its Summary
	// so the LLM knows which repair to attempt next turn.
	Reason string
}

// GroundCitation validates one Citation against the grounding context.
// No in-place mutation — pure function. Callers mutate on the way out
// (clear Quote, drop item) according to the Report.
//
// When the grounding context carries NO ground-truth sources (no
// read_file gutter index and no repomap graph), grounding cannot
// form a judgement and returns Valid=true unconditionally. This
// shows up in two places: unit tests that construct a bare
// BusContext, and early-pipeline calls that happen before any tool
// has produced a read_file result. Both want the pre-grounding
// structural checks to stand alone.
func GroundCitation(c types.Citation, gc *Context) CitationReport {
	file := strings.TrimSpace(c.File)
	if file == "" || c.Line <= 0 {
		return CitationReport{Reason: "empty file or non-positive line"}
	}
	if gc == nil || (len(gc.LineIndex) == 0 && gc.Graph == nil) {
		return CitationReport{Valid: true}
	}
	// Tier 1: gutter index hit — the LLM actually read this line.
	if gc != nil {
		if fileLines, ok := gc.LineIndex[file]; ok {
			if _, hasLine := fileLines[c.Line]; hasLine {
				quote := strings.TrimSpace(c.Quote)
				if quote == "" {
					return CitationReport{Valid: true, QuoteMatched: false, Tier: types.TierLineText}
				}
				return CitationReport{
					Valid:        true,
					QuoteMatched: quoteCorroboratesLine(quote, fileLines, c.Line, 2),
					Tier:         types.TierLineText,
				}
			}
		}
	}
	// Tier 2: graph lookup — the file is indexed by repomap AND the
	// cited line falls in a region the LLM could plausibly have seen
	// without reading the file. Accepted regions:
	//
	//   (a) inside some symbol's [Line-docRadius, EndLine] — the
	//       -docRadius prefix covers the symbol's doc comment block
	//       (GoDoc, Python docstring preamble, JSDoc) which is a
	//       legitimate citation anchor.
	//   (b) inside the file prologue [1, firstSymbolLine-1] — package
	//       docs, imports, top-level consts. Rescues `facade.go:1`.
	//
	// Earlier contract was "file exists" only, which let the LLM cite
	// ANY line in an indexed file as long as the line number was
	// positive — including mid-file comments and random blank lines.
	// A fabricated line number that happened to fall on an empty
	// separator line between two symbols passed unconditionally. The
	// tighter rule requires the line to be structurally meaningful;
	// genuinely legitimate cites that miss both (a) and (b) can still
	// ride Tier 1 by having the LLM read_file the source first, which
	// is the recovery the rule nudges toward.
	if gc != nil && gc.Graph != nil {
		if fi, ok := gc.Graph.FileIndex[file]; ok && fi != nil {
			if tier2LineInStructuralRegion(fi, c.Line) {
				return CitationReport{Valid: true, QuoteMatched: false, Tier: types.TierSymbolTable}
			}
			return CitationReport{
				Reason: fmt.Sprintf("source %q is indexed but line %d is not inside any symbol range or the file prologue — read the file first so Tier 1 can corroborate", file, c.Line),
			}
		}
	}
	return CitationReport{
		Reason: fmt.Sprintf("source %q not in read_file history and not indexed in the repomap graph", file),
	}
}

// quoteCorroboratesLine checks whether the citation's Quote shares at
// least one identifier-shaped token with the cited line or its
// immediate neighbours. Prose quotes ("the repomap facade binds …")
// that contain zero identifier tokens fall through as uncorroborated —
// structural, not keyword-list based.
func quoteCorroboratesLine(quote string, fileLines map[int]string, line, radius int) bool {
	text, ok := lookupLineWithNeighbours(fileLines, line, radius)
	if !ok {
		return false
	}
	qTokens := tokenSet(quote)
	if len(qTokens) == 0 {
		// Quote with no identifier tokens is pure prose — we cannot
		// corroborate it against code, so report false so the caller
		// clears it.
		return false
	}
	hTokens := tokenSet(text)
	for t := range qTokens {
		if hTokens[t] {
			return true
		}
	}
	return false
}

// tier2DocRadius is how many lines above a symbol's declaration are
// accepted as part of that symbol's doc-comment preamble. 10 covers
// typical GoDoc blocks (2-6 lines), Python docstrings (up to 10 lines
// before triple-quote opens), and JSDoc blocks without over-matching.
const tier2DocRadius = 10

// tier2LineInStructuralRegion reports whether a cited line falls in a
// region of the file that the repomap parser mapped to structural
// meaning: inside some symbol's body-plus-docblock, or inside the
// prologue (pkg doc / imports / top-level consts) before the first
// symbol. Any line outside those regions is a suspicious citation —
// either a mid-file blank separator or a comment the LLM made up.
func tier2LineInStructuralRegion(fi *repomap.FileInfo, line int) bool {
	if fi == nil || line <= 0 {
		return false
	}
	firstSymbolLine := 0
	for i := range fi.Symbols {
		s := &fi.Symbols[i]
		if s.Line <= 0 {
			continue
		}
		docStart := s.Line - tier2DocRadius
		if docStart < 1 {
			docStart = 1
		}
		end := s.EndLine
		if end < s.Line {
			end = s.Line
		}
		if line >= docStart && line <= end {
			return true
		}
		if firstSymbolLine == 0 || s.Line < firstSymbolLine {
			firstSymbolLine = s.Line
		}
	}
	// Prologue rescue: when the file has any indexed symbol, lines
	// above the first symbol's declaration (minus the doc radius) are
	// the pkg-doc / imports / top-level const region.
	if firstSymbolLine > 0 && line < firstSymbolLine-tier2DocRadius {
		return true
	}
	// Files with no indexed symbols (YAML, JSON, plain config) have no
	// structural regions to test against — accept any positive line so
	// config citations don't regress. Callers that want stricter
	// behaviour on these can add a Tier 1 check.
	if firstSymbolLine == 0 {
		return true
	}
	return false
}

// ── Recovery R1: fqname_same_file ────────────────────────────────────
//
// The item cites Source but LineStart is off. Look up AnchorSymbol in
// graph.SymbolsInFile(Source) — for AnchorKind=definition/return/
// assignment/condition we want the symbol declaration line; for
// AnchorKind=call we prefer the first matching callsite in
// FileInfo.Relations. Returns the first structural match.
func recoverFQNameSameFile(it *types.EvidenceItem, gc *Context) (string, int, bool) {
	if gc == nil || gc.Graph == nil {
		return "", 0, false
	}
	name := preferredSymbolName(it)
	if name == "" {
		return "", 0, false
	}
	shortName := lastDotSegment(name)
	switch it.AnchorKind {
	case types.AnchorCall:
		if fi, ok := gc.Graph.FileIndex[it.Source]; ok && fi != nil {
			for _, rel := range fi.Relations {
				if rel.Kind == "call" && (rel.ToEP.Name == name || rel.ToEP.Name == shortName) && rel.Line > 0 {
					return "", rel.Line, true
				}
			}
		}
	case types.AnchorImport:
		if fi, ok := gc.Graph.FileIndex[it.Source]; ok && fi != nil {
			for _, imp := range fi.Imports {
				if (imp.Path == name || imp.Alias == name) && imp.Line > 0 {
					return "", imp.Line, true
				}
			}
		}
	default:
		for _, sym := range gc.Graph.SymbolsInFile(it.Source) {
			if (sym.Name == name || sym.Name == shortName) && sym.Line > 0 {
				return "", sym.Line, true
			}
		}
	}
	return "", 0, false
}

// ── Recovery R2: snippet_fuzzy ───────────────────────────────────────
//
// Needs item.Snippet. Scans ±15 lines around LineStart in the
// read_file gutter for the best token-overlap match with Snippet.
// Accepts when overlap ≥ 60% of Snippet's identifier tokens; in that
// case returns the matched line number.
func recoverSnippetFuzzy(it *types.EvidenceItem, gc *Context) (string, int, bool) {
	if gc == nil {
		return "", 0, false
	}
	snippet := strings.TrimSpace(it.Snippet)
	if snippet == "" {
		return "", 0, false
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok {
		return "", 0, false
	}
	want := tokenSet(snippet)
	if len(want) == 0 {
		return "", 0, false
	}
	bestLine, bestScore := 0, 0.0
	threshold := 0.6
	radius := 15
	if it.LineStart <= 0 {
		return "", 0, false
	}
	for i := it.LineStart - radius; i <= it.LineStart+radius; i++ {
		text, exists := fileLines[i]
		if !exists {
			continue
		}
		have := tokenSet(text)
		if len(have) == 0 {
			continue
		}
		var overlap int
		for w := range want {
			if have[w] {
				overlap++
			}
		}
		score := float64(overlap) / float64(len(want))
		if score > bestScore {
			bestScore = score
			bestLine = i
		}
	}
	if bestScore >= threshold && bestLine > 0 {
		return "", bestLine, true
	}
	return "", 0, false
}

// ── Recovery R3: package_symbol ──────────────────────────────────────
//
// Only fires for definition and import anchors. For those kinds the
// AnchorSymbol *is* a cross-file semantic locator: "this symbol's
// declaration is in file X" is a well-posed question, and when the
// LLM pastes a neighbouring file by mistake (e.g. cited
// explorer.go when the type is defined in sub_explorer.go) R3 can
// repair it.
//
// Call / condition / return / assignment / unspecified anchors are
// file-local events — the call site lives where the LLM said it
// does, even when a same-named definition exists in another file.
// The old unguarded implementation rewrote agent.go:900 (an
// `if _, err := b.deps.SubAgents.Get(...); err == nil {` condition)
// to registry.go:30 (the SubAgentRegistry.Get method definition)
// because both carry a `Get` symbol — a textbook same-name drift.
// The guard shuts the door on that entire class.
func recoverPackageSymbol(it *types.EvidenceItem, gc *Context) (string, int, bool) {
	if gc == nil || gc.Graph == nil {
		return "", 0, false
	}
	// Gate on AnchorKind: cross-file package lookup is only
	// meaningful for definition-sited anchors. Empty AnchorKind
	// (legacy pre-redesign items) is also gated out because we
	// cannot tell whether the item is a definition or a call site.
	switch it.AnchorKind {
	case types.AnchorDefinition, types.AnchorImport:
		// ok, continue.
	default:
		return "", 0, false
	}
	name := preferredSymbolName(it)
	if name == "" {
		return "", 0, false
	}
	shortName := lastDotSegment(name)
	defs, ok := gc.Graph.SymbolDefs[name]
	if !ok || len(defs) == 0 {
		defs, ok = gc.Graph.SymbolDefs[shortName]
		if !ok {
			return "", 0, false
		}
	}
	originalPkg, originalDir := packageOrDir(gc.Graph, it.Source)
	for _, sym := range defs {
		if sym == nil || sym.File == "" || sym.Line <= 0 {
			continue
		}
		if sym.File == it.Source {
			continue // handled by R1
		}
		candPkg, candDir := packageOrDir(gc.Graph, sym.File)
		if originalPkg != "" && candPkg != "" && originalPkg == candPkg {
			return sym.File, sym.Line, true
		}
		if originalDir != "" && candDir != "" && originalDir == candDir {
			return sym.File, sym.Line, true
		}
	}
	return "", 0, false
}

func packageOrDir(g *repomap.Graph, path string) (pkg, dir string) {
	if fi, ok := g.FileIndex[path]; ok && fi != nil {
		pkg = fi.Package
	}
	if idx := strings.LastIndex(path, "/"); idx > 0 {
		dir = path[:idx]
	}
	return pkg, dir
}

// ── Recovery R4: nearest_call ────────────────────────────────────────
//
// AnchorKind=call with R1-R3 missed — scan FileInfo.Relations for the
// single closest callsite matching AnchorSymbol and snap LineStart
// to it. Distance capped so we don't leap across file halves.
func recoverNearestCall(it *types.EvidenceItem, gc *Context) (string, int, bool) {
	if gc == nil || gc.Graph == nil || it.AnchorKind != types.AnchorCall {
		return "", 0, false
	}
	fi, ok := gc.Graph.FileIndex[it.Source]
	if !ok || fi == nil {
		return "", 0, false
	}
	name := preferredSymbolName(it)
	if name == "" {
		return "", 0, false
	}
	shortName := lastDotSegment(name)
	bestLine, bestDist := 0, math.MaxInt
	for _, rel := range fi.Relations {
		if rel.Kind != "call" {
			continue
		}
		if rel.ToEP.Name != name && rel.ToEP.Name != shortName {
			continue
		}
		if rel.Line <= 0 {
			continue
		}
		d := rel.Line - it.LineStart
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = rel.Line
		}
	}
	if bestLine > 0 && bestDist <= 40 {
		return "", bestLine, true
	}
	return "", 0, false
}

// ── Recovery R5: nearest_condition ───────────────────────────────────
//
// AnchorKind=condition with R1-R3 missed — scan lineIndex[Source]
// within ±10 of LineStart for a line starting with one of the
// condition keywords. Snaps to that line. Distance-capped.
func recoverNearestCondition(it *types.EvidenceItem, gc *Context) (string, int, bool) {
	if gc == nil || it.AnchorKind != types.AnchorCondition {
		return "", 0, false
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok {
		return "", 0, false
	}
	radius := 10
	bestLine, bestDist := 0, math.MaxInt
	for i := it.LineStart - radius; i <= it.LineStart+radius; i++ {
		text, exists := fileLines[i]
		if !exists {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(text))
		matched := false
		for _, kw := range conditionKeywords {
			if strings.HasPrefix(lower, kw) || strings.Contains(" "+lower, " "+kw) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		d := i - it.LineStart
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = i
		}
	}
	if bestLine > 0 {
		return "", bestLine, true
	}
	return "", 0, false
}

// ── Tier 1 ────────────────────────────────────────────────────────────

// tier1LineText returns true when the AnchorSymbol (falling back to
// the union of Subject/Object/Condition tokens when AnchorSymbol is
// empty, for tests that pass old-shape items) appears as a whole word
// in the read_file gutter text for Source at LineStart±2.
func tier1LineText(it *types.EvidenceItem, gc *Context) bool {
	if it.Source == "" || it.LineStart <= 0 || gc == nil {
		return false
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok {
		return false
	}
	text, exists := lookupLineWithNeighbours(fileLines, it.LineStart, 2)
	if !exists {
		return false
	}
	// Anchor-first check: when the LLM supplied AnchorSymbol, the
	// line must contain that exact token. This is the precise path
	// we want — it avoids the old Subject/Object heuristic that
	// matched same-name methods on wrong receivers.
	if it.AnchorSymbol != "" {
		have := tokenSet(text)
		if have[it.AnchorSymbol] {
			return true
		}
		// Also accept the last dotted segment (so `Receiver.Method`
		// supplied as AnchorSymbol still matches a line containing
		// just `Method`).
		if seg := lastDotSegment(it.AnchorSymbol); seg != it.AnchorSymbol && have[seg] {
			return true
		}
		// Keyword fallback for the AnchorKinds repomap cannot index
		// structurally: if the LLM claimed this was a condition or
		// return or assignment, Tier 1 additionally accepts the line
		// when the kind-specific keyword is present. This means a
		// condition claim grounded via a line that contains the
		// anchor_symbol AND starts with `if`/`when`/… is still
		// reported as TierLineText — the dispatch lives in Tier 2
		// only for the repomap-native cases.
		return false
	}
	// Legacy fallback for items missing AnchorSymbol: use the
	// Subject/Object/Condition code-shaped identifier union.
	return lineCorroborates(text, it.Subject, it.Object, it.Condition, gc.Graph)
}

// ── Tier 2 ────────────────────────────────────────────────────────────

// tier2SymbolTable dispatches on AnchorKind to the repomap-backed
// structural check. Returns true on a hit; false when the kind is
// not supported natively (the caller then falls to recovery tiers
// or ungrounded).
//
// AnchorKinds condition/return/assignment degrade to a keyword scan
// on the cited line: repomap does not extract control-flow shapes,
// but we can still corroborate the shape by looking at the line text.
// This degradation keeps the required-AnchorKind contract usable
// across every evidence shape without demanding graph support repomap
// doesn't have.
func tier2SymbolTable(it *types.EvidenceItem, gc *Context) bool {
	if it.Source == "" || it.LineStart <= 0 {
		return false
	}
	// The AnchorKind is required on new items but may be empty on
	// items that predate the redesign (tests, legacy flows). Treat
	// empty as "try the repomap-native kinds that make sense on
	// Subject/AnchorSymbol".
	switch it.AnchorKind {
	case types.AnchorDefinition, "":
		if graphMatchDefinition(it, gc) {
			return true
		}
		if it.AnchorKind == types.AnchorDefinition {
			return false
		}
		fallthrough
	case types.AnchorCall:
		if graphMatchCall(it, gc) {
			return true
		}
		if it.AnchorKind == types.AnchorCall {
			return false
		}
	}
	switch it.AnchorKind {
	case types.AnchorImport:
		return graphMatchImport(it, gc)
	case types.AnchorCondition:
		return lineContainsAnyKeyword(it, gc, conditionKeywords)
	case types.AnchorReturn:
		return lineContainsAnyKeyword(it, gc, returnKeywords)
	case types.AnchorAssignment:
		return lineContainsAssignment(it, gc)
	}
	return false
}

var conditionKeywords = []string{"if ", "if(", "when ", "when(", "unless ", "switch ", "switch(", "case ", "guard "}
var returnKeywords = []string{"return ", "return\t", "return(", "return;", "yield "}

func graphMatchDefinition(it *types.EvidenceItem, gc *Context) bool {
	if gc == nil || gc.Graph == nil {
		return false
	}
	name := preferredSymbolName(it)
	if name == "" {
		return false
	}
	for _, sym := range gc.Graph.SymbolsInFile(it.Source) {
		if sym.Line != it.LineStart {
			continue
		}
		if sym.Name == name || lastDotSegment(name) == sym.Name {
			return true
		}
	}
	return false
}

func graphMatchCall(it *types.EvidenceItem, gc *Context) bool {
	if gc == nil || gc.Graph == nil {
		return false
	}
	fi, ok := gc.Graph.FileIndex[it.Source]
	if !ok || fi == nil {
		return false
	}
	name := preferredSymbolName(it)
	if name == "" {
		return false
	}
	shortName := lastDotSegment(name)
	for _, rel := range fi.Relations {
		if rel.Kind != "call" {
			continue
		}
		if rel.Line != it.LineStart {
			continue
		}
		if rel.ToEP.Name == name || rel.ToEP.Name == shortName {
			return true
		}
	}
	return false
}

func graphMatchImport(it *types.EvidenceItem, gc *Context) bool {
	if gc == nil || gc.Graph == nil {
		return false
	}
	fi, ok := gc.Graph.FileIndex[it.Source]
	if !ok || fi == nil {
		return false
	}
	name := strings.TrimSpace(it.AnchorSymbol)
	if name == "" {
		name = strings.TrimSpace(it.Subject)
	}
	if name == "" {
		return false
	}
	for _, imp := range fi.Imports {
		if imp.Line != it.LineStart {
			continue
		}
		if imp.Path == name || imp.Alias == name {
			return true
		}
	}
	return false
}

func lineContainsAnyKeyword(it *types.EvidenceItem, gc *Context, keywords []string) bool {
	if gc == nil {
		return false
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok {
		return false
	}
	text, exists := lookupLineWithNeighbours(fileLines, it.LineStart, 0)
	if !exists {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, kw := range keywords {
		if strings.HasPrefix(lower, kw) || strings.Contains(" "+lower, " "+kw) {
			return true
		}
	}
	return false
}

func lineContainsAssignment(it *types.EvidenceItem, gc *Context) bool {
	if gc == nil {
		return false
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok {
		return false
	}
	text, exists := lookupLineWithNeighbours(fileLines, it.LineStart, 0)
	if !exists {
		return false
	}
	// Accept `:=` anywhere on the line, or a bare `=` that is not
	// part of `==`/`!=`/`<=`/`>=`. The repo has no curated stopword
	// list for this by design — the rule is structural.
	if strings.Contains(text, ":=") {
		return true
	}
	for i := 0; i < len(text); i++ {
		if text[i] != '=' {
			continue
		}
		prev := byte(' ')
		if i > 0 {
			prev = text[i-1]
		}
		next := byte(' ')
		if i+1 < len(text) {
			next = text[i+1]
		}
		if prev == '=' || prev == '!' || prev == '<' || prev == '>' || next == '=' {
			continue
		}
		return true
	}
	return false
}

// ── ungrounded explanation ───────────────────────────────────────────

func explainUngrounded(it *types.EvidenceItem, gc *Context) string {
	var parts []string
	if gc == nil || len(gc.LineIndex) == 0 {
		parts = append(parts, "no read_file history available in this dispatch")
	} else if _, ok := gc.LineIndex[it.Source]; !ok {
		parts = append(parts, "source "+it.Source+" not present in read_file history (call read_file on it first)")
	} else if it.AnchorSymbol != "" {
		parts = append(parts, "anchor_symbol "+strconv.Quote(it.AnchorSymbol)+" not found as a whole-word token near line "+strconv.Itoa(it.LineStart))
	} else {
		parts = append(parts, "no anchor_symbol provided and Subject/Object identifiers did not match any token near line "+strconv.Itoa(it.LineStart))
	}
	if it.AnchorKind != "" && gc != nil && gc.Graph != nil {
		if _, ok := gc.Graph.FileIndex[it.Source]; !ok {
			parts = append(parts, "source not indexed in repomap graph")
		}
	}
	return strings.Join(parts, "; ")
}

// ── shared helpers (moved from internal/agent/evidence.go) ───────────

// buildLineIndex walks tool history and reconstructs, for every
// read_file result that carries a gutter, a map from absolute line
// number → line text (without the gutter prefix). Subsequent
// read_file calls on the same Source merge into the existing map so
// the latest content for each line wins.
func buildLineIndex(history []types.ToolResult) map[string]map[int]string {
	out := make(map[string]map[int]string)
	for _, r := range history {
		if !r.Success || r.ToolName != "read_file" {
			continue
		}
		body := r.Summary
		if !strings.HasPrefix(body, "[") {
			continue
		}
		idx := strings.Index(body, "\n")
		if idx < 0 {
			continue
		}
		path := parseBannerPath(body[:idx])
		if path == "" {
			continue
		}
		fileMap, ok := out[path]
		if !ok {
			fileMap = make(map[int]string)
			out[path] = fileMap
		}
		for _, gutterLine := range strings.Split(body[idx+1:], "\n") {
			m := gutterLineRe.FindStringSubmatch(gutterLine)
			if m == nil {
				continue
			}
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			fileMap[n] = m[2]
		}
	}
	return out
}

func parseBannerPath(banner string) string {
	if !strings.HasPrefix(banner, "[") {
		return ""
	}
	banner = strings.TrimPrefix(banner, "[")
	colon := strings.Index(banner, ": ")
	if colon < 1 {
		return ""
	}
	return banner[:colon]
}

func lookupLineWithNeighbours(fileLines map[int]string, n, radius int) (string, bool) {
	if _, ok := fileLines[n]; !ok {
		return "", false
	}
	var b strings.Builder
	for i := n - radius; i <= n+radius; i++ {
		if text, ok := fileLines[i]; ok {
			b.WriteString(text)
			b.WriteByte('\n')
		}
	}
	return b.String(), true
}

// lineCorroborates is the legacy Subject/Object/Condition matcher,
// kept for items that were constructed without AnchorSymbol (tests
// or deterministic concrete_value items). Identical semantics to the
// old lineCorroboratesEvidence.
func lineCorroborates(lineText, subject, object, condition string, graph *repomap.Graph) bool {
	want := extractCodeIdentifiers(subject+" "+object+" "+condition, graph)
	if len(want) == 0 {
		return false
	}
	have := tokenSet(lineText)
	for _, w := range want {
		if have[w] {
			return true
		}
	}
	return false
}

func tokenSet(s string) map[string]bool {
	out := make(map[string]bool)
	for _, tok := range identifierTokenRe.FindAllString(s, -1) {
		out[tok] = true
	}
	return out
}

// extractCodeIdentifiers walks a free-text fragment and returns
// identifier-shaped tokens that look like real code identifiers —
// CamelCase, snake_case, or known to the repo's symbol graph.
// Lowercase common English words are rejected purely on structural
// grounds, no curated stopword list.
func extractCodeIdentifiers(text string, graph *repomap.Graph) []string {
	tokens := identifierTokenRe.FindAllString(text, -1)
	if len(tokens) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(tokens))
	var out []string
	for _, t := range tokens {
		if seen[t] {
			continue
		}
		if !looksLikeCodeIdentifier(t, graph) {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func looksLikeCodeIdentifier(tok string, graph *repomap.Graph) bool {
	hasUpper, hasLower, hasUnderscore := false, false, false
	for _, r := range tok {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r == '_':
			hasUnderscore = true
		}
	}
	if hasUpper && hasLower {
		return true
	}
	if hasUnderscore {
		return true
	}
	if graph == nil {
		return false
	}
	if defs, ok := graph.SymbolDefs[tok]; ok && len(defs) > 0 {
		return true
	}
	return false
}

func lastDotSegment(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 && idx < len(s)-1 {
		return s[idx+1:]
	}
	return s
}

// preferredSymbolName returns AnchorSymbol if set, else falls back to
// Subject. Consumer of the grounder is expected to always set
// AnchorSymbol on new items, but the fallback keeps legacy callers
// functional.
func preferredSymbolName(it *types.EvidenceItem) string {
	if s := strings.TrimSpace(it.AnchorSymbol); s != "" {
		return s
	}
	return strings.TrimSpace(it.Subject)
}
