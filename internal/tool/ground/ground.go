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
	"sort"
	"strconv"
	"strings"

	"sync"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// crossRepoOracle is the package-level cross-sub-repo SymbolOracle
// looksLikeCodeIdentifier consults as a last-fallback when the
// per-graph SymbolDefs check misses. Set by BuildContext from the
// BusContext.MultiGraph carrier (P4-cross-sub-repo Sc 6, 2026-05-08);
// scoped to the most recent BuildContext call (sequential per Run,
// no concurrent overwrite within a single user-facing dispatch).
//
// nil disables the fan-out path and reverts to legacy single-graph
// classification (single-shot tests, multi_repo_enabled=false).
var (
	crossRepoOracleMu sync.RWMutex
	crossRepoOracle   types.SymbolOracle
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
//
// RepoRoot is the absolute path of the target repository, used to
// canonicalise every file / source / citation path to the same
// repo-relative form before Tier 1 / Tier 2 lookups. Without it,
// a citation to `/mnt/d/repo/README.md:7` could not match a
// LineIndex built from a `read_file path=README.md` banner.
type Context struct {
	LineIndex map[string]map[int]string // source path → (1-based line → text)
	Graph     *repomap.Graph            // nil when explorer has not populated MutableState yet
	RepoRoot  string                    // for path canonicalisation; empty is tolerated (no normalisation)

	// MultiGraphOracle is the cross-sub-repo SymbolOracle wired
	// from BusContext.MultiGraph (P4-cross-sub-repo, 2026-05-08).
	// When non-nil, looksLikeCodeIdentifier and other ground-side
	// "is this a real symbol?" checks fan out across every active
	// sub-repo's index — eliminates the false-negative where a
	// real identifier defined in a non-routed sub-repo was
	// classified as plain English. Single-repo posture is
	// byte-equivalent (mg.Oracle() forwards through one graph).
	MultiGraphOracle types.SymbolOracle
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
	// Sources, layered oldest → newest so in-dispatch reads win on
	// conflicting line numbers (the LLM may have re-read a file
	// with a different offset/limit this turn):
	//
	//   1. TurnA snapshot's ToolResults — explorer's historical
	//      read_file calls, frozen into Mutable.TurnAArtifacts
	//      when the explorer stage ended. Finalizer-time citation
	//      grounding needs this or else every cite lands in Tier 2
	//      (pre-session-8 bug: LLM cited `subagent.go:64`, explorer
	//      had read the whole file, but BuildContext couldn't see
	//      the gutter → "LLM never read any cited file" rule
	//      dropped every citation).
	//
	//   2. BusContext.ToolResults — tool results applyStageOutput
	//      propagated between stages. Currently empty for the
	//      explorer → finalizer handoff (explorer returns
	//      StageOutput.ToolResults=[], historical reads live only
	//      on TurnAArtifacts), but preserved as a source for
	//      future stages that might use it and for sub-agents
	//      whose parent set ToolResults directly.
	//
	//   3. Mutable.DispatchToolResults — the current ReAct loop's
	//      in-dispatch buffer. Empty for the finalizer (it has no
	//      file-reading tools) but essential for emit_evidence in
	//      the explorer loop: the grounder runs mid-loop and needs
	//      to see read_file results from earlier iterations of
	//      the SAME dispatch.
	var history []types.ToolResult
	if ctx.Mutable != nil {
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil && len(ta.ToolResults) > 0 {
			history = append(history, ta.ToolResults...)
		}
	}
	if len(ctx.ToolResults) > 0 {
		history = append(history, ctx.ToolResults...)
	}
	if ctx.Mutable != nil {
		if disp := ctx.Mutable.DispatchToolResults(); len(disp) > 0 {
			history = append(history, disp...)
		}
	}
	gc.RepoRoot = ctx.RepoRoot
	gc.LineIndex = buildLineIndex(history, ctx.RepoRoot)
	if ctx.Mutable != nil {
		if g, ok := ctx.Mutable.SearchGraph().(*repomap.Graph); ok {
			gc.Graph = g
		}
	}
	// P4-cross-sub-repo (Sc 6, 2026-05-08): the cross-sub-repo oracle
	// is wired separately by SetCrossRepoOracle (from tool layer that
	// has access to multigraph) — ground.go cannot import multigraph
	// directly without a tool→ground→multigraph→topology→tool
	// cycle. The oracle is consulted as a fallback inside
	// looksLikeCodeIdentifier; nil disables the fan-out path.
	return gc
}

// SetCrossRepoOracle installs the package-level cross-sub-repo
// SymbolOracle that looksLikeCodeIdentifier consults as a fallback
// when the per-graph SymbolDefs check misses. Called by the tool
// layer's emit_evidence / emit_investigation_complete entry points
// after BuildContext, since those have access to *multigraph.MultiGraph
// via repomap. Pass nil to clear (single-shot tests reset between
// runs to avoid cross-test pollution).
func SetCrossRepoOracle(oracle types.SymbolOracle) {
	crossRepoOracleMu.Lock()
	crossRepoOracle = oracle
	crossRepoOracleMu.Unlock()
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
//     - definition → SymbolsInFile + sym.Line == LineStart
//     - call       → FileIndex[Source].Relations where
//     Kind=="call" + ToEP.Name == AnchorSymbol
//     + Line == LineStart
//     - import     → FileInfo.Imports + imp.Path/Alias match +
//     imp.Line == LineStart
//     - condition/return/assignment → no repomap primitive, degrade
//     to line-text scan for the kind's keyword set
//     (if/when/unless/switch/case/guard for
//     condition; return/yield for return; := or
//     standalone = for assignment)
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

	// Canonicalise Source up-front so every tier's LineIndex /
	// FileIndex / SymbolsInFile lookup on `it.Source` lands on the
	// same key as buildLineIndex / repomap's RelPath. Otherwise an
	// evidence item with an absolute Source silently falls through
	// Tier 1 and Tier 2 even when the explorer did read the file.
	if gc != nil && it.Source != "" {
		it.Source = CanonicalRepoRelative(it.Source, gc.RepoRoot)
	}

	// Tier 1: line_text via read_file gutter.
	if tier1LineText(it, gc) {
		attachGroundedLineSnippet(it, gc)
		it.GroundingStatus = types.GroundingGrounded
		it.GroundingTier = types.TierLineText
		return Report{
			ItemID: it.ID, Status: it.GroundingStatus, Tier: it.GroundingTier,
			OriginalLine: originalLine, AdjustedLine: it.LineStart,
		}
	}

	// Tier 2: symbol_table via repomap dispatch.
	if tier2SymbolTable(it, gc) {
		attachGroundedLineSnippet(it, gc)
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
			attachGroundedLineSnippet(it, gc)
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

// attachGroundedLineSnippet upgrades line-scoped evidence with the
// actual grounded source line once grounding has fixed the final
// Source/LineStart pair. Deterministic downstream channels should
// prefer this anchor-local code surface over any free-form semantic
// prose the LLM previously attached to Subject/Predicate/Object.
func attachGroundedLineSnippet(it *types.EvidenceItem, gc *Context) {
	if it == nil || gc == nil || it.Source == "" || it.LineStart <= 0 {
		return
	}
	if it.Scope != "" && it.Scope != types.ScopeLine {
		return
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok {
		return
	}
	lineNo := it.LineStart
	if matched, matchedOK := findSnippetLine(fileLines, it.LineStart, 2, it.Snippet); matchedOK {
		lineNo = matched
	} else {
		switch it.AnchorKind {
		case types.AnchorCall:
			if matched, matchedOK := findCallCorroboratingLine(fileLines, it.LineStart, 2, it, gc.Graph); matchedOK {
				lineNo = matched
			}
		case types.AnchorCondition, types.AnchorReturn, types.AnchorAssignment:
			if matched, matchedOK := findCorroboratingLine(fileLines, it.LineStart, 2, it, gc.Graph); matchedOK {
				lineNo = matched
			}
		default:
			if it.AnchorSymbol != "" {
				if matched, matchedOK := findAnchorLine(fileLines, it.LineStart, 2, it.AnchorSymbol); matchedOK {
					lineNo = matched
				}
			} else if matched, matchedOK := findCorroboratingLine(fileLines, it.LineStart, 2, it, gc.Graph); matchedOK {
				lineNo = matched
			}
		}
	}
	lineText, ok := fileLines[lineNo]
	if !ok {
		return
	}
	lineText = strings.TrimSpace(lineText)
	if lineText == "" {
		return
	}
	it.Snippet = lineText
}

func findSnippetLine(fileLines map[int]string, center, radius int, snippet string) (int, bool) {
	want := normalizeGroundedSnippetLine(snippet)
	if want == "" {
		return 0, false
	}
	if text, ok := fileLines[center]; ok && normalizeGroundedSnippetLine(text) == want {
		return center, true
	}
	bestLine, bestDist := 0, radius+1
	for i := center - radius; i <= center+radius; i++ {
		if i == center || i <= 0 {
			continue
		}
		text, ok := fileLines[i]
		if !ok || normalizeGroundedSnippetLine(text) != want {
			continue
		}
		d := i - center
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = i
		}
	}
	if bestLine > 0 {
		return bestLine, true
	}
	return 0, false
}

func normalizeGroundedSnippetLine(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
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
	// Canonicalise file to match the form LineIndex (built via the
	// same helper) and FileIndex (repomap's RelPath, already
	// repo-relative) already use as keys. Handles the "LLM read with
	// relative path but cited with absolute" failure mode that caused
	// kept=0 dropped=12 on explanation-shape answers.
	file = CanonicalRepoRelative(file, gc.RepoRoot)
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
				Reason: fmt.Sprintf("source %q is indexed but line %d is not inside any symbol range or the file prologue — %sread the file first so Tier 1 can corroborate, or pick a line inside one of the symbols above",
					file, c.Line, nearestSymbolsHint(fi, c.Line)),
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
// that contain zero identifier tokens fall through to a byte-level
// substring check — covers Unicode literals (Chinese / Japanese /
// Korean prompt strings, Cyrillic identifiers etc.) the ASCII
// identifierTokenRe cannot tokenise.
//
// The fallback only fires when the ASCII path would have hard-rejected
// the quote (zero tokens). Quotes that DO carry ASCII tokens still
// take the original token-set route byte-identically; this keeps the
// false-positive surface unchanged for the common code-symbol case
// where token-set's word-boundary semantics matter.
func quoteCorroboratesLine(quote string, fileLines map[int]string, line, radius int) bool {
	text, ok := lookupLineWithNeighbours(fileLines, line, radius)
	if !ok {
		return false
	}
	qTokens := tokenSet(quote)
	if len(qTokens) == 0 {
		return nonASCIISubstringMatch(quote, text)
	}
	hTokens := tokenSet(text)
	for t := range qTokens {
		if hTokens[t] {
			return true
		}
	}
	return false
}

// nonASCIISubstringMatch reports whether trimmed needle appears as a
// byte-level substring of haystack. Used as the deterministic fallback
// for quote / anchor checks when the ASCII identifierTokenRe extracts
// zero tokens — i.e. the input is a Unicode literal, a punctuation-only
// string, or any text the regex cannot tokenise.
//
// Trimming both sides handles the common case where the LLM emits the
// quote with leading / trailing whitespace that is not present in the
// raw source line. An empty needle after trim is conservatively rejected
// (can never corroborate anything).
func nonASCIISubstringMatch(needle, haystack string) bool {
	n := strings.TrimSpace(needle)
	if n == "" {
		return false
	}
	return strings.Contains(haystack, n)
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

// nearestSymbolsHint builds a "Candidate symbols nearby: ..." string
// for a citation that fell outside any structural region. Lists up to
// 3 symbols closest to the rejected line, sorted by absolute
// distance (ties broken by earlier-line-first). Empty result when the
// file has zero indexed symbols — callers should skip the prefix in
// that case. Always ends with ". " so it can be slotted inline before
// the existing repair sentence.
//
// The hint is the payload of C+: instead of just telling the LLM
// "line N is wrong, read the file", give it three concrete lines
// that ARE valid anchors so it can fix in one round-trip.
func nearestSymbolsHint(fi *repomap.FileInfo, line int) string {
	if fi == nil || len(fi.Symbols) == 0 {
		return ""
	}
	type scored struct {
		s    *repomap.Symbol
		dist int
	}
	ranked := make([]scored, 0, len(fi.Symbols))
	for i := range fi.Symbols {
		s := &fi.Symbols[i]
		if s.Line <= 0 {
			continue
		}
		d := line - s.Line
		if d < 0 {
			d = -d
		}
		ranked = append(ranked, scored{s: s, dist: d})
	}
	if len(ranked) == 0 {
		return ""
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].dist != ranked[j].dist {
			return ranked[i].dist < ranked[j].dist
		}
		return ranked[i].s.Line < ranked[j].s.Line
	})
	limit := 3
	if len(ranked) < limit {
		limit = len(ranked)
	}
	var b strings.Builder
	b.WriteString("Candidate symbols nearby: ")
	for i := 0; i < limit; i++ {
		s := ranked[i].s
		if i > 0 {
			b.WriteString(", ")
		}
		if s.EndLine > s.Line {
			fmt.Fprintf(&b, "%s (lines %d-%d)", s.Name, s.Line, s.EndLine)
		} else {
			fmt.Fprintf(&b, "%s (line %d)", s.Name, s.Line)
		}
	}
	b.WriteString(". ")
	return b.String()
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
	switch it.AnchorKind {
	case types.AnchorCall:
		candidates := preferredCallTargetNames(it)
		if len(candidates) == 0 {
			return "", 0, false
		}
		if fi, ok := gc.Graph.FileIndex[it.Source]; ok && fi != nil {
			for _, rel := range fi.Relations {
				if rel.Kind != "call" || rel.Line <= 0 {
					continue
				}
				relName := strings.TrimSpace(rel.ToEP.Name)
				if relName == "" {
					relName = strings.TrimSpace(rel.To)
				}
				for _, candidate := range candidates {
					if relName == candidate || relName == lastDotSegment(candidate) {
						return "", rel.Line, true
					}
				}
			}
		}
	case types.AnchorImport:
		name := preferredSymbolName(it)
		if name == "" {
			return "", 0, false
		}
		if fi, ok := gc.Graph.FileIndex[it.Source]; ok && fi != nil {
			for _, imp := range fi.Imports {
				if (imp.Path == name || imp.Alias == name) && imp.Line > 0 {
					return "", imp.Line, true
				}
			}
		}
	default:
		name := preferredSymbolName(it)
		if name == "" {
			return "", 0, false
		}
		shortName := lastDotSegment(name)
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
	if it.LineStart <= 0 {
		return "", 0, false
	}
	want := tokenSet(snippet)
	radius := 15
	if len(want) == 0 {
		// Unicode-only snippet (Chinese / Japanese / pure punctuation
		// LLM evidence): the ASCII regex extracts zero tokens. Fall
		// back to byte-level substring scan within the same ±radius
		// window. Strict equivalence to "found exact snippet at line"
		// — no fractional threshold because we don't have a token
		// vocabulary to score against. Keeps the recovery feature
		// available for non-ASCII evidence instead of silently
		// dropping it.
		for i := it.LineStart - radius; i <= it.LineStart+radius; i++ {
			text, exists := fileLines[i]
			if !exists {
				continue
			}
			if nonASCIISubstringMatch(snippet, text) {
				return "", i, true
			}
		}
		return "", 0, false
	}
	bestLine, bestScore := 0, 0.0
	threshold := 0.6
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
//
// Comment-line rejection: when the matching line is a pure comment
// (either a single-line `//` / `#` comment or inside a `/* ... */`
// or Python `""" ... """` block), Tier 1 refuses to accept it. The
// anchor_symbol being mentioned by prose is not evidence of a real
// anchor — the LLM often cites a nearby comment that narrates what
// a function DID elsewhere in the codebase. Rejecting here forces
// the cascade into Tier 2 (symbol_table) and recovery, where the
// repomap graph can find the genuine definition.
func tier1LineText(it *types.EvidenceItem, gc *Context) bool {
	if it.Source == "" || it.LineStart <= 0 || gc == nil {
		return false
	}
	fileLines, ok := gc.LineIndex[it.Source]
	if !ok {
		return false
	}
	if it.AnchorKind == types.AnchorCall {
		matchedLine, ok := findCallCorroboratingLine(fileLines, it.LineStart, 2, it, gc.Graph)
		if !ok {
			return false
		}
		if isLineComment(fileLines, matchedLine, it.Source) {
			return false
		}
		return true
	}
	configSurface := configSurfaceAllowsLooseLineGrounding(it)
	// Anchor-first check: when the LLM supplied AnchorSymbol, pick the
	// specific line in ±2 that actually contains the token, then comment-
	// check that line. Per-line resolution is needed because the
	// comment-exclusion gate is line-level, not window-level.
	if it.AnchorSymbol != "" {
		matchedLine, ok := findAnchorLine(fileLines, it.LineStart, 2, it.AnchorSymbol)
		if ok {
			if isLineComment(fileLines, matchedLine, it.Source) && !configSurface {
				return false
			}
			return true
		}
		if !configSurface {
			return false
		}
		// Config-file surfaces (YAML / JSON / TOML / INI / etc.) often
		// carry precedence or binding facts on comment / key lines whose
		// decisive anchor is the line itself rather than a durable code
		// identifier. When the LLM guessed an anchor_symbol poorly, fall
		// back to corroborating the actual line text against the emitted
		// semantic fields instead of forcing a symbol-token repair loop.
		text, exists := lookupLineWithNeighbours(fileLines, it.LineStart, 2)
		if !exists {
			return false
		}
		if !configSurfaceLineCorroborates(text, it, gc.Graph) {
			return false
		}
		matchedLine, ok = findConfigSurfaceCorroboratingLine(fileLines, it.LineStart, 2, it, gc.Graph)
		if !ok {
			return false
		}
		return true
	}
	// Legacy fallback for items missing AnchorSymbol: use the
	// Subject/Object/Condition code-shaped identifier union across the
	// ±2 window, then comment-check the specific line that matched.
	text, exists := lookupLineWithNeighbours(fileLines, it.LineStart, 2)
	if !exists {
		return false
	}
	if !lineCorroborates(text, it.Subject, it.Object, it.Condition, gc.Graph) {
		return false
	}
	matchedLine, ok := findCorroboratingLine(fileLines, it.LineStart, 2, it, gc.Graph)
	if !ok {
		// Corroborated via the aggregated window text but not any
		// individual line (unusual but possible). Be conservative: reject.
		return false
	}
	if isLineComment(fileLines, matchedLine, it.Source) && !configSurface {
		return false
	}
	return true
}

func configSurfaceAllowsLooseLineGrounding(it *types.EvidenceItem) bool {
	if it == nil || it.Source == "" {
		return false
	}
	role := it.DiagramRole
	if role == types.EvidenceDiagramRoleUnknown {
		role = it.RequestedDiagramRole
	}
	return role == types.EvidenceDiagramRoleConfig &&
		types.LooksLikeConfigFilePath(it.Source)
}

func configSurfaceLineCorroborates(lineText string, it *types.EvidenceItem, graph *repomap.Graph) bool {
	if strings.TrimSpace(lineText) == "" || it == nil {
		return false
	}
	return lineCorroborates(
		lineText,
		strings.Join([]string{it.Subject, it.Object, it.Condition, it.Summary}, "\n"),
		"",
		"",
		graph,
	)
}

func findConfigSurfaceCorroboratingLine(fileLines map[int]string, center, radius int, it *types.EvidenceItem, graph *repomap.Graph) (int, bool) {
	if text, ok := fileLines[center]; ok {
		if configSurfaceLineCorroborates(text, it, graph) {
			return center, true
		}
	}
	bestLine, bestDist := 0, radius+1
	for i := center - radius; i <= center+radius; i++ {
		if i == center || i <= 0 {
			continue
		}
		text, ok := fileLines[i]
		if !ok {
			continue
		}
		if !configSurfaceLineCorroborates(text, it, graph) {
			continue
		}
		d := i - center
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = i
		}
	}
	if bestLine > 0 {
		return bestLine, true
	}
	return 0, false
}

// findAnchorLine locates the specific line within [center-radius,
// center+radius] that contains the anchor token. Prefers the exact
// center line when it matches; otherwise returns the nearest neighbour.
// Matching accepts both the full anchor and its last dotted segment
// (so `Receiver.Method` matches a line bearing just `Method`).
// VerifyLineAnchor returns (matchedLine, true) when anchor appears as
// a whole word at source:line ±radius and the matched line is not a
// pure comment. Returns (_, false) when the file is not in the line
// index (never read_file'd), when anchor is empty, or when no
// non-comment line within ±radius contains anchor.
//
// This is the public entry point used by callers outside the grounder
// itself — Fix D routes emit_answer_symbol item grounding through it
// so extractor-stage hallucinations ("X calls Y at call site:line"
// mis-read as "X defined at line") are rejected before they reach
// Mutable.SetEmittedAnswerSymbols, rather than surfacing only at
// finalizer citation time when the retry budget is already spent.
func VerifyLineAnchor(gc *Context, source string, line int, anchor string, radius int) (int, bool) {
	if gc == nil || source == "" || line <= 0 || anchor == "" {
		return 0, false
	}
	fileLines, ok := gc.LineIndex[source]
	if !ok {
		return 0, false
	}
	if matched, ok := findNonCommentAnchorLine(fileLines, line, radius, anchor, source); ok {
		return matched, true
	}
	if matched, ok := findAdjacentDocCommentDefinitionLine(fileLines, line, 12, anchor, source); ok {
		return matched, true
	}
	return 0, false
}

// HasFileInIndex reports whether source has been read_file'd (or
// otherwise materialised into the line index) in this context. Used
// by callers that need to distinguish "line text disagreed with the
// claim" (reject) from "we have no line text to check against" (let
// the item through and rely on downstream grounding).
func HasFileInIndex(gc *Context, source string) bool {
	if gc == nil || source == "" {
		return false
	}
	_, ok := gc.LineIndex[source]
	return ok
}

func findAnchorLine(fileLines map[int]string, center, radius int, anchor string) (int, bool) {
	seg := lastDotSegment(anchor)
	// When the anchor itself yields zero ASCII identifier tokens
	// (Chinese / Japanese / Korean literals, Unicode-only identifiers,
	// punctuation-bearing string literals), the ASCII token-set path
	// is structurally unable to corroborate it. Switch to byte-level
	// substring matching against each line in the window. ASCII
	// anchors take the original path byte-identically — guarded by
	// internal/tool/ground/ground_unicode_test.go to keep token-set
	// precision intact for the dominant code-symbol case.
	anchorTokens := tokenSet(anchor)
	useSubstring := len(anchorTokens) == 0
	// Center-first: if the claimed line matches, take it.
	if text, ok := fileLines[center]; ok {
		if lineContainsAnchor(text, anchor, seg, useSubstring) {
			return center, true
		}
	}
	// Nearest-neighbour scan.
	bestLine, bestDist := 0, radius+1
	for i := center - radius; i <= center+radius; i++ {
		if i == center || i <= 0 {
			continue
		}
		text, ok := fileLines[i]
		if !ok {
			continue
		}
		if !lineContainsAnchor(text, anchor, seg, useSubstring) {
			continue
		}
		d := i - center
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = i
		}
	}
	if bestLine > 0 {
		return bestLine, true
	}
	return 0, false
}

func findNonCommentAnchorLine(fileLines map[int]string, center, radius int, anchor, source string) (int, bool) {
	seg := lastDotSegment(anchor)
	anchorTokens := tokenSet(anchor)
	useSubstring := len(anchorTokens) == 0
	if text, ok := fileLines[center]; ok &&
		lineContainsAnchor(text, anchor, seg, useSubstring) &&
		!isLineComment(fileLines, center, source) {
		return center, true
	}
	bestLine, bestDist := 0, radius+1
	for i := center - radius; i <= center+radius; i++ {
		if i == center || i <= 0 || isLineComment(fileLines, i, source) {
			continue
		}
		text, ok := fileLines[i]
		if !ok || !lineContainsAnchor(text, anchor, seg, useSubstring) {
			continue
		}
		d := i - center
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = i
		}
	}
	if bestLine > 0 {
		return bestLine, true
	}
	return 0, false
}

func findAdjacentDocCommentDefinitionLine(fileLines map[int]string, center, maxLookahead int, anchor, source string) (int, bool) {
	if maxLookahead <= 0 || !isLineComment(fileLines, center, source) {
		return 0, false
	}
	seg := lastDotSegment(anchor)
	anchorTokens := tokenSet(anchor)
	useSubstring := len(anchorTokens) == 0
	for i := center + 1; i <= center+maxLookahead; i++ {
		text, ok := fileLines[i]
		if !ok {
			return 0, false
		}
		if strings.TrimSpace(text) == "" || isLineComment(fileLines, i, source) {
			continue
		}
		if lineContainsAnchor(text, anchor, seg, useSubstring) {
			return i, true
		}
		return 0, false
	}
	return 0, false
}

func lineContainsAnchor(text, anchor, seg string, useSubstring bool) bool {
	if useSubstring {
		return nonASCIISubstringMatch(anchor, text)
	}
	have := tokenSet(text)
	return have[anchor] || (seg != anchor && have[seg])
}

// findCorroboratingLine is the legacy-fallback counterpart of
// findAnchorLine: locates the specific line within the ±radius window
// whose tokenSet satisfies lineCorroborates. Used for evidence items
// that were constructed without AnchorSymbol.
func findCorroboratingLine(fileLines map[int]string, center, radius int, it *types.EvidenceItem, graph *repomap.Graph) (int, bool) {
	if text, ok := fileLines[center]; ok {
		if lineCorroborates(text, it.Subject, it.Object, it.Condition, graph) {
			return center, true
		}
	}
	bestLine, bestDist := 0, radius+1
	for i := center - radius; i <= center+radius; i++ {
		if i == center || i <= 0 {
			continue
		}
		text, ok := fileLines[i]
		if !ok {
			continue
		}
		if !lineCorroborates(text, it.Subject, it.Object, it.Condition, graph) {
			continue
		}
		d := i - center
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = i
		}
	}
	if bestLine > 0 {
		return bestLine, true
	}
	return 0, false
}

func findCallCorroboratingLine(fileLines map[int]string, center, radius int, it *types.EvidenceItem, graph *repomap.Graph) (int, bool) {
	if text, ok := fileLines[center]; ok {
		if lineCorroboratesCallSite(text, it, graph, center) {
			return center, true
		}
	}
	bestLine, bestDist := 0, radius+1
	for i := center - radius; i <= center+radius; i++ {
		if i == center || i <= 0 {
			continue
		}
		text, ok := fileLines[i]
		if !ok {
			continue
		}
		if !lineCorroboratesCallSite(text, it, graph, i) {
			continue
		}
		d := i - center
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestLine = i
		}
	}
	if bestLine > 0 {
		return bestLine, true
	}
	return 0, false
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

// conditionKeywords carries the surface-level lexical signatures the
// recoverNearestCondition tier scans for in the ±10-line window
// around a missed AnchorCondition citation. Every language repomap
// supports (Go / Java / Kotlin / Cangjie / ArkTS / Python / JS / TS /
// Rust / C / C++ / Swift / Ruby / Lua / Proto / Obj-C / CUDA) maps
// at least one of its conditional / dispatch constructs to one of
// these patterns:
//
//	if / if(            — universal ASCII conditional
//	when / when(        — Kotlin pattern entry, Ruby `when` inside `case`
//	unless              — Ruby
//	switch / switch(    — Go / Java / JS / TS / C / C++ / Swift / Obj-C / CUDA
//	case                — Go / Java / Ruby / C / C++ / Swift case-arm
//	guard               — Swift early-exit
//	match / match(      — Rust, Python 3.10+, Cangjie pattern-matching
//	select { / select{  — Go channel select (the "select {" form
//	                      avoids false-positive matches against bare
//	                      uppercase SQL "SELECT" tokens)
//	try { / try: / try{ — exception-handling entry: Java / JS / TS /
//	                      Kotlin / Cangjie / Swift / Rust experimental
//	                      use `try {`; Python uses `try:`. The "{" /
//	                      ":" suffix avoids matching prose like
//	                      "try harder".
//	catch { / catch ( / catch  — Java / JS / TS / Kotlin / Cangjie /
//	                      C++ / Swift / Obj-C @catch. Three variants
//	                      cover `catch(Exception e)` (no space),
//	                      `catch (Exception e)` (with space), and
//	                      Swift's `do { ... } catch { ... }` form.
//	except / except:    — Python `except ValueError:` and bare
//	                      `except:`
//	rescue              — Ruby `rescue StandardError => e`
//	finally { / finally: / finally{
//	                    — Java / JS / TS / Kotlin / Cangjie / C++ /
//	                      Swift use `finally {`; Python uses
//	                      `finally:`.
//	ensure              — Ruby
//	throw / raise       — explicit failure-dispatch sites: Java /
//	                      JS / TS / Kotlin / Cangjie / Swift / C++ /
//	                      Obj-C @throw use `throw`; Python / Ruby
//	                      use `raise`. Treated as conditional-class
//	                      because LLMs cite throw / raise lines when
//	                      explaining "the code raises an error
//	                      under condition X".
//
// elseif / elsif / elif (Lua / Ruby / Python continuation forms) are
// intentionally NOT listed: they always trail an `if` within the
// ±10 radius the recovery scans, so the original `if` line is the
// one the recovery snaps to. Adding the trailing forms would only
// increase noise.
//
// SectionParse / SymbolTable tiers still cover language-specific
// AST nodes; this list is the LAST-RESORT lexical fallback so the
// grounder never fails-loud on a missed condition citation in any
// supported language.
var conditionKeywords = []string{
	"if ", "if(",
	"when ", "when(",
	"unless ",
	"switch ", "switch(",
	"case ",
	"guard ",
	"match ", "match(",
	"select {", "select{",
	"try {", "try:", "try{",
	"catch ", "catch(", "catch {",
	"except ", "except:",
	"rescue ",
	"finally {", "finally:", "finally{",
	"ensure ",
	"throw ", "raise ",
}
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
	candidates := preferredCallTargetNames(it)
	if len(candidates) == 0 {
		return false
	}
	for _, rel := range fi.Relations {
		if rel.Kind != "call" {
			continue
		}
		if rel.Line != it.LineStart {
			continue
		}
		relName := strings.TrimSpace(rel.ToEP.Name)
		if relName == "" {
			relName = strings.TrimSpace(rel.To)
		}
		for _, candidate := range candidates {
			if relName == candidate || relName == lastDotSegment(candidate) {
				return true
			}
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

func lineCorroboratesCallSite(lineText string, it *types.EvidenceItem, graph *repomap.Graph, lineNo int) bool {
	candidates := preferredCallTargetNames(it)
	if len(candidates) == 0 {
		return false
	}
	if graphLineHasCallToAnyTarget(graph, it.Source, lineNo, candidates) {
		return true
	}
	if looksLikeCallableDefinitionLine(lineText) {
		return false
	}
	for _, target := range candidates {
		if !lineContainsCallTarget(lineText, target) {
			continue
		}
		if looksLikeCallSyntax(lineText, target) {
			return true
		}
	}
	return false
}

func graphLineHasCallToAnyTarget(graph *repomap.Graph, source string, line int, targets []string) bool {
	if graph == nil || source == "" || line <= 0 || len(targets) == 0 {
		return false
	}
	fi, ok := graph.FileIndex[source]
	if !ok || fi == nil {
		return false
	}
	want := make(map[string]bool, len(targets)*2)
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		want[target] = true
		if leaf := lastDotSegment(target); leaf != "" {
			want[leaf] = true
		}
	}
	if len(want) == 0 {
		return false
	}
	for _, rel := range fi.Relations {
		if rel.Kind != "call" || rel.Line != line {
			continue
		}
		if want[rel.ToEP.Name] {
			return true
		}
		if want[strings.TrimSpace(rel.To)] {
			return true
		}
	}
	return false
}

func lineContainsCallTarget(lineText, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	have := tokenSet(lineText)
	if len(have) == 0 {
		return false
	}
	if have[target] {
		return true
	}
	leaf := lastDotSegment(target)
	return leaf != target && have[leaf]
}

func looksLikeCallableDefinitionLine(lineText string) bool {
	lower := strings.ToLower(strings.TrimSpace(lineText))
	switch {
	case strings.HasPrefix(lower, "func "):
		return true
	case strings.HasPrefix(lower, "def "):
		return true
	case strings.HasPrefix(lower, "function "):
		return true
	case strings.HasPrefix(lower, "fn "):
		return true
	default:
		return false
	}
}

func looksLikeCallSyntax(lineText, target string) bool {
	lineText = strings.TrimSpace(lineText)
	target = strings.TrimSpace(target)
	if lineText == "" || target == "" {
		return false
	}
	candidates := []string{target}
	if leaf := lastDotSegment(target); leaf != "" && leaf != target {
		candidates = append(candidates, leaf)
	}
	for _, candidate := range candidates {
		patterns := []string{
			candidate + "(",
			candidate + " (",
			"." + candidate + "(",
			"." + candidate + " (",
			"->" + candidate + "(",
			"->" + candidate + " (",
			"::" + candidate + "(",
			"::" + candidate + " (",
		}
		for _, pattern := range patterns {
			if strings.Contains(lineText, pattern) {
				return true
			}
		}
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
func buildLineIndex(history []types.ToolResult, repoRoot string) map[string]map[int]string {
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
		// Canonicalise to repo-relative so lookups from GroundCitation
		// / GroundItem (which also canonicalise their inputs) land on
		// the same key regardless of whether the LLM read the file via
		// relative or absolute path.
		path = CanonicalRepoRelative(path, repoRoot)
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
	// Strip forced-read trace prefixes (`[forced_read] `,
	// `[forced_read surgical] `) before parsing. runForcedReads
	// prepends these so operators can grep the trace, but the
	// prefix's own `[` collides with the banner's `[path: ...]`
	// shape — without this strip the parser would split on the
	// FIRST `: ` it found inside the prefix, producing a malformed
	// path key that misses every LineIndex lookup. The grounder
	// then reports "ungrounded" for citations to the surgical-read
	// content even though the LLM did receive it.
	banner = StripForcedReadPrefix(banner)
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

// looksLikeCodeIdentifier classifies a token as a code identifier
// vs plain English. Heuristic order:
//  1. CamelCase / mixedCase + has both upper and lower → identifier
//  2. snake_case (contains '_') → identifier
//  3. lowercase-only ambiguous: defer to graph.SymbolDefs[tok]
//     existence check.
//
// Cross-sub-repo enrichment (P4-cross-sub-repo Sc 6, 2026-05-08):
// the package-level CrossRepoOracle (set by BuildContext from the
// BusContext.MultiGraph carrier) is consulted as an additional
// fallback before returning false on lowercase-only tokens, so
// identifiers defined only in non-routed sub-repos are still
// classified as identifiers. Concurrent BuildContext calls are
// serialised inside BuildContext (which is per-Run) so the
// package-level overwrite is sequential.
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
	// Per-graph fallback (legacy + single-repo).
	if graph != nil {
		if defs, ok := graph.SymbolDefs[tok]; ok && len(defs) > 0 {
			return true
		}
	}
	// Cross-sub-repo fan-out fallback.
	crossRepoOracleMu.RLock()
	oracle := crossRepoOracle
	crossRepoOracleMu.RUnlock()
	if oracle != nil {
		if found, _ := oracle.SymbolExists(tok); found {
			return true
		}
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

func preferredCallTargetNames(it *types.EvidenceItem) []string {
	if it == nil {
		return nil
	}
	seen := make(map[string]bool, 3)
	out := make([]string, 0, 3)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(it.AnchorSymbol)
	add(it.Object)
	add(it.Subject)
	return out
}
