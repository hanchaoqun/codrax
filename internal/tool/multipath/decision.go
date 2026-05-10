// Package multipath replaces the predecessor "multi-path coverage
// parity" pre-complete gate (raiseMultiPathCoverageParity in
// internal/tool/emit_investigation_complete.go, retired 2026-05-01)
// with a layered, anchor-driven verification that works for code
// AND non-code files (config / docs / manifests).
//
// Why the rewrite: the old gate computed a per-file required-lines
// floor as `total_lines * 0.3`. The signal it actually produced was
// "the LLM read fewer than 30 % of the file's bytes", which has
// almost no correlation with answer quality:
//
//   - Large monoliths (e.g. a 4 368-line repl.go) demanded > 1 300
//     lines of pagination even when the answer-bearing code lived in
//     four ~30-line symbol regions totalling ~120 lines.
//   - Small files easily passed regardless of whether the LLM
//     understood them.
//   - Reading thousands of irrelevant lines actively poisoned the
//     LLM context (citations drifted into noise, downstream
//     extractor's hypothesis verdict accuracy degraded).
//
// The replacement separates "did the LLM look at the right place?"
// from "how many lines did the LLM mechanically paginate?". Five
// signals run in priority order. The first one that decides wins:
//
//  L0 — SignalFullyRead (HasFullyRead bypass).
//    Merged ranges already cover every line; demanding more would
//    page past EOF. Inherited from the old gate; preserved as the
//    leading short-circuit.
//
//  L1 — Symbol-anchored verification (precision; CODE files).
//    For each primary anchor file with repomap symbols, identify the
//    question-related symbols whose definitions live in that file.
//    Verify every such symbol's [def_line - SymbolContextLines ..
//    def_end + SymbolContextLines] region is covered. All clear →
//    SignalSymbolAnchored / Skip. Some uncovered →
//    SignalSymbolDemand / DemandSurgicalRead with the missing
//    slivers as the demand.
//
//  L2 — Grounded-evidence count (output quality).
//    If the LLM has emitted >= MinGroundedPerAnchor grounded (Tier 1
//    / Tier 2) evidence items whose Source equals the file, the LLM
//    has demonstrated localised understanding → bypass with
//    SignalEvidenceVerified.
//
//  L3 — Keyword-anchored verification (NON-symbolic files).
//    For files where repomap returns no symbols (typically *.md /
//    *.yaml / *.toml / *.json / *.json5 / *.ini / Dockerfile /
//    Makefile / shell scripts), use the LLM's own grep history to
//    locate answer-bearing lines: every line where grep matched a
//    question entity becomes a keyword anchor (±KeywordContextLines
//    context). All clear → SignalKeywordAnchored / Skip; missing →
//    SignalKeywordDemand / DemandSurgicalRead with the missing
//    slivers. Only fires when the LLM has actually grep'd the file
//    for question entities (anchorLines empty → skip to L4).
//
//  L4 — Small-file full-read demand (NON-symbolic files, bounded).
//    A file whose total line count is <= SmallFileThreshold (default
//    300) is small enough that "read the rest of it" is bounded —
//    typical config files (codrax.yaml, package.json, pyproject.toml,
//    Dockerfile, README.md) fit easily. Demand the unread slivers
//    via MissingContextRegions(file, [1, totalLines], 0) →
//    SignalSmallFileFull / DemandSurgicalRead. The threshold is the
//    boundary "noise cost is bounded by file size" — reading 300
//    lines of irrelevant config has limited blast radius.
//
//  L5 — SignalOpaqueAdvisory (LARGE non-symbolic files, last resort).
//    File is large (> SmallFileThreshold), has no symbols, no
//    grounded evidence, and no keyword grep history pointing into
//    it. The system has no defensible specific demand to make.
//    Emit a non-blocking RepairExpandSearch advisory so the LLM is
//    informed but completion is NOT blocked — the LLM may legitimately
//    judge the file unrelated and proceed.
//
// File-type detection is signal-driven, NOT extension-based: a file
// is "non-symbolic" when oracle.SymbolsInFile returns empty,
// regardless of extension. This is the same red-line discipline as
// types.IsProjectOrientationQuestion (no keyword/extension matching).
//
// The decision engine is a pure function of (file, closure, evidence,
// oracle, entities, keywordAnchors, cfg) — it produces a Decision
// struct that the caller translates into AddPendingRead / AddRepair
// calls. This keeps the orchestration side trivial to test and lets
// the engine itself be covered by table-driven unit tests with no
// closure-state plumbing.
package multipath

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Signal labels which check decided the outcome. Useful for both the
// log-trace breadcrumbs and for tests that lock in the dispatch
// behaviour without re-deriving it from rendered Rationale strings.
type Signal int

const (
	SignalUnknown Signal = iota

	// SignalFullyRead — the file's merged read ranges already cover
	// every line. Preserves the old HasFullyRead bypass; demanding
	// any more reads would page past EOF.
	SignalFullyRead

	// SignalSymbolAnchored — Layer 1 positive: every question-related
	// symbol region in the file (with its surrounding context window)
	// is already covered. The LLM has touched the answer-bearing
	// parts.
	SignalSymbolAnchored

	// SignalEvidenceVerified — Layer 2: the LLM has emitted at least
	// MinGroundedPerAnchor grounded evidence items from the file.
	// Tier 1 / Tier 2 grounding means the citations are real; we
	// trust the LLM's localised understanding.
	SignalEvidenceVerified

	// SignalSymbolDemand — Layer 1 negative: the file has identifiable
	// question-related symbols but at least one symbol region is
	// uncovered. The Decision carries a surgical Demand slice naming
	// exactly the uncovered slivers.
	SignalSymbolDemand

	// SignalOpaqueAdvisory — Layer 5 (last resort): no usable signal.
	// The Decision is info-only (Action == ActionAdvisoryHint). The
	// LLM may still complete; the hint just nudges toward grep-then-
	// read for large files where the system cannot defend a specific
	// demand.
	SignalOpaqueAdvisory

	// SignalKeywordAnchored — Layer 3 positive: file has no repomap
	// symbols (typically *.md / *.yaml / *.toml / *.json / etc.) but
	// every line where the LLM's prior grep matched a question entity
	// is already covered (with KeywordContextLines context). The LLM
	// has reached every answer-bearing region the grep history
	// points to.
	SignalKeywordAnchored

	// SignalKeywordDemand — Layer 3 negative: non-symbolic file with
	// grep-match anchors, but at least one anchor region is uncovered.
	// Surgical Demand on the missing slivers around grep-hit lines.
	SignalKeywordDemand

	// SignalSmallFileFull — Layer 4: non-symbolic file small enough
	// (FileTotalLines <= SmallFileThreshold) that demanding the
	// remaining unread slivers is bounded-cost. Triggers when L1-L3
	// all yielded no signal — config files / READMEs / Dockerfiles
	// fall here automatically when they are below the size threshold.
	SignalSmallFileFull
)

// String returns the lower-snake-case label used in [CGEC] log lines.
func (s Signal) String() string {
	switch s {
	case SignalFullyRead:
		return "fully_read"
	case SignalSymbolAnchored:
		return "symbol_anchored"
	case SignalEvidenceVerified:
		return "evidence_verified"
	case SignalSymbolDemand:
		return "symbol_demand"
	case SignalKeywordAnchored:
		return "keyword_anchored"
	case SignalKeywordDemand:
		return "keyword_demand"
	case SignalSmallFileFull:
		return "small_file_full"
	case SignalOpaqueAdvisory:
		return "opaque_advisory"
	}
	return "unknown"
}

// Action tells the orchestration caller how to translate the Decision
// into closure mutations. Three values:
//
//   - ActionSkip — file passes; do nothing.
//   - ActionDemandSurgicalRead — block emit_investigation_complete
//     until the named line ranges are read. The caller AddPendingRead
//     with a Rationale that names the uncovered symbol regions.
//   - ActionAdvisoryHint — non-blocking nudge; AddRepair with kind
//     RepairExpandSearch but NO PendingRead so completion is not
//     stalled.
type Action int

const (
	ActionSkip Action = iota
	ActionDemandSurgicalRead
	ActionAdvisoryHint
)

// String returns the lower-snake-case action label.
func (a Action) String() string {
	switch a {
	case ActionSkip:
		return "skip"
	case ActionDemandSurgicalRead:
		return "demand_surgical_read"
	case ActionAdvisoryHint:
		return "advisory_hint"
	}
	return "unknown"
}

// Decision is the per-anchor verdict the engine returns. The caller
// is the only side effect — Decision itself is value-typed and pure.
type Decision struct {
	File    string
	Signal  Signal
	Action  Action
	Demand  []types.LineRange // populated when Action == ActionDemandSurgicalRead
	Symbols []string          // names of the question-related symbols (if any) — for log + rationale
	Reason  string            // short machine-readable reason (logging)
	// Rationale is the exact human-readable text the orchestration
	// layer should hand to AddPendingRead.Rationale or AddRepair.
	// Computed by the engine so the caller does not need to know the
	// presentation format.
	Rationale string
}

// Config carries the tunables the engine reads. Mirrors the relevant
// fields of tool.AnalysisLimits; passed by value so the engine has no
// global state and tests can supply explicit values.
type Config struct {
	// MinGroundedPerAnchor is the Layer 2 threshold: at least this
	// many grounded evidence items (Source == file, GroundingStatus
	// in {grounded, recovered}, GroundingTier in {Tier1, Tier2}) are
	// required to bypass via "evidence verified". Default 2.
	// 0 disables Layer 2.
	MinGroundedPerAnchor int

	// SymbolContextLines is the Layer 1 padding: the closure must
	// cover [def_line - SymbolContextLines, def_end + SymbolContextLines]
	// for every question-related symbol. Default 15.
	SymbolContextLines int

	// KeywordContextLines is the Layer 3 padding: for non-symbolic
	// files where grep matched a question entity at line L, the
	// closure must cover [L - KeywordContextLines, L + KeywordContextLines]
	// to satisfy that anchor. Default 15 (mirrors SymbolContextLines
	// — same "context window around an answer-bearing line" semantic).
	KeywordContextLines int

	// SmallFileThreshold is the Layer 4 boundary: a non-symbolic file
	// with FileTotalLines <= SmallFileThreshold falls to "demand the
	// unread slivers of the whole file" (bounded-cost). Above the
	// threshold, the file falls to Layer 5 advisory (non-blocking).
	// Default 300 — covers typical config files and READMEs without
	// dragging in large architecture docs.
	// 0 disables Layer 4 (large + small files alike fall to advisory).
	SmallFileThreshold int

	// MaxKeywordAnchorsPerFile caps the number of keyword anchors
	// ExtractKeywordAnchors retains per file. When grep hits a
	// high-frequency entity (a couple hundred matches in a single
	// file), the resulting demand would otherwise produce that many
	// surgical read_file calls in runForcedReads — each ~5-10ms,
	// totalling 1-2 seconds of synchronous IO per stall round.
	// Cap defaults to 30; sampled evenly across the matched lines so
	// the gate retains coverage breadth across the whole file rather
	// than clustering on the first matches. 0 disables the cap.
	MaxKeywordAnchorsPerFile int

	// RepoRoot is the absolute repository root used by every path
	// canonicalisation site inside the engine (ground.CanonicalRepoRelative).
	// Required for multi-platform safety:
	//   - POSIX: strips an absolute /repo/... prefix into repo-relative.
	//   - Windows: case-insensitive volume + drive-letter handling.
	//   - macOS: same POSIX path semantics, plus tolerance for case-
	//     insensitive HFS+/APFS by default (CanonicalRepoRelative
	//     handles this).
	// Empty RepoRoot is tolerated — every site degrades to a slash-
	// form cleanup that matches pre-2026-05-01 behaviour for tests
	// that don't care about repo roots. Production callers must pass
	// ctx.RepoRoot or path mismatches WILL leak (grep returning
	// absolute paths against a Phase1Ranking using repo-relative,
	// Windows backslash paths against POSIX-style anchor lists, etc).
	RepoRoot string
}

// SymbolOracle is the small slice of the repomap Graph the engine
// consumes. Hand-rolled so multipath does not have to import the
// repomap package directly (avoiding any future cycle risk and
// keeping the test surface tiny — fakes implement two methods).
type SymbolOracle interface {
	// SymbolsInFile returns the symbols defined in `file`. Each
	// SymbolDef carries the symbol name plus 1-based [Line, EndLine]
	// definition span; EndLine == 0 is tolerated (treated as a
	// single-line definition rooted at Line).
	SymbolsInFile(file string) []SymbolDef
}

// SymbolDef is the minimum projection of a repomap.Symbol the engine
// needs. Defined here (instead of taking repomap.Symbol directly) so
// the package's surface stays decoupled from repomap internals.
type SymbolDef struct {
	Name    string
	Line    int
	EndLine int
}

// EvaluateAnchor computes the Decision for one primary-anchor file.
// Pure function; safe to call any number of times on the same inputs.
//
// Inputs:
//
//   - file: repo-relative path (already canonicalised by the caller).
//   - closure: per-Run EvidenceClosure (read snapshot + ranges +
//     totals). MUST be the live one, not a stale copy.
//   - evidence: every emit_evidence item the LLM has produced this Run.
//   - oracle: SymbolOracle backed by the repomap. nil tolerated —
//     Layer 1 is then disabled and the engine falls through to L2-L5.
//   - questionEntities: lower-cased entity names from the analyzer
//     (typically the union of AnalyzerHints.{Primary,Mentioned,
//     Derived,}Entities). nil / empty is fine — L1 then finds no
//     symbols, L3 finds no keyword anchors, engine drops to L4 / L5.
//   - keywordAnchors: 1-based line numbers in `file` where the LLM's
//     prior grep matched a question entity. Pre-computed by the
//     caller via ExtractKeywordAnchors over the live tool history.
//     nil / empty is fine — L3 then no-ops and engine drops to L4.
//   - cfg: tunables. Pass current tool.AnalysisLimits or a literal
//     Config in tests.
//
// The function does NOT mutate any input. The caller is responsible
// for translating Decision into closure mutations.
func EvaluateAnchor(
	file string,
	closure *types.EvidenceClosure,
	evidence []types.EvidenceItem,
	oracle SymbolOracle,
	questionEntities []string,
	keywordAnchors []int,
	cfg Config,
) Decision {
	dec := Decision{File: file}

	// L0 — fully-read short-circuit. A file with merged ranges
	// covering its full line count has no outstanding signal to
	// chase; demanding more would page past EOF.
	if closure != nil && closure.HasFullyRead(file) {
		dec.Signal = SignalFullyRead
		dec.Action = ActionSkip
		dec.Reason = "file fully covered by merged read ranges"
		return dec
	}

	// L1 — symbol-anchored verification (CODE files).
	// Identify question-related symbols defined in this file.
	symbols := questionRelatedSymbols(file, oracle, evidence, questionEntities, cfg.RepoRoot)
	dec.Symbols = symbolNames(symbols)
	if len(symbols) > 0 && closure != nil {
		regions := symbolRegions(symbols)
		missing := closure.MissingContextRegions(file, regions, cfg.SymbolContextLines)
		if len(missing) == 0 {
			dec.Signal = SignalSymbolAnchored
			dec.Action = ActionSkip
			dec.Reason = fmt.Sprintf("all %d question-related symbol regions covered (±%d ctx)", len(symbols), cfg.SymbolContextLines)
			return dec
		}
		// L1 negative — surgical demand. Build a precise rationale
		// naming the missing slivers + the symbols they belong to.
		// The point is to make the LLM read THIS chunk, not a
		// percentage of the file.
		dec.Signal = SignalSymbolDemand
		dec.Action = ActionDemandSurgicalRead
		dec.Demand = missing
		dec.Reason = fmt.Sprintf("%d question-related symbol(s) defined here; %d region(s) still uncovered", len(symbols), len(missing))
		dec.Rationale = renderSymbolDemandRationale(file, dec.Symbols, symbols, missing, cfg.SymbolContextLines)
		return dec
	}

	// L2 — grounded evidence count (any file type).
	if cfg.MinGroundedPerAnchor > 0 {
		count := groundedEvidenceCount(file, evidence, cfg.RepoRoot)
		if count >= cfg.MinGroundedPerAnchor {
			dec.Signal = SignalEvidenceVerified
			dec.Action = ActionSkip
			dec.Reason = fmt.Sprintf("%d grounded evidence item(s) from this file (≥ floor %d)", count, cfg.MinGroundedPerAnchor)
			return dec
		}
	}

	// L3 — keyword-anchored verification (NON-symbolic files with
	// grep history). Each line where prior grep matched a question
	// entity becomes a keyword anchor; same surgical-demand semantic
	// as Layer 1, just sourced from grep instead of repomap.
	if len(keywordAnchors) > 0 && closure != nil {
		regions := singletonRegionsFromLines(keywordAnchors)
		missing := closure.MissingContextRegions(file, regions, cfg.KeywordContextLines)
		if len(missing) == 0 {
			dec.Signal = SignalKeywordAnchored
			dec.Action = ActionSkip
			dec.Reason = fmt.Sprintf("all %d keyword-anchor lines covered (±%d ctx)", len(keywordAnchors), cfg.KeywordContextLines)
			return dec
		}
		dec.Signal = SignalKeywordDemand
		dec.Action = ActionDemandSurgicalRead
		dec.Demand = missing
		dec.Reason = fmt.Sprintf("%d keyword anchor line(s) from grep history; %d region(s) still uncovered", len(keywordAnchors), len(missing))
		dec.Rationale = renderKeywordDemandRationale(file, keywordAnchors, missing, cfg.KeywordContextLines)
		return dec
	}

	// L4 — small-file full-read demand (NON-symbolic, bounded cost).
	// A file small enough that "read the rest of it" is bounded by
	// SmallFileThreshold lines. Catches the typical config / README /
	// Dockerfile case where the answer might be on any line and we
	// have no way to surgically point.
	if closure != nil && cfg.SmallFileThreshold > 0 {
		total := closure.FileTotalLines(file)
		if total > 0 && total <= cfg.SmallFileThreshold {
			missing := closure.MissingContextRegions(file, []int{1, total}, 0)
			if len(missing) > 0 {
				dec.Signal = SignalSmallFileFull
				dec.Action = ActionDemandSurgicalRead
				dec.Demand = missing
				dec.Reason = fmt.Sprintf("non-symbolic file with %d lines (≤ small-file threshold %d); demand the %d unread sliver(s)", total, cfg.SmallFileThreshold, len(missing))
				dec.Rationale = renderSmallFileFullRationale(file, total, cfg.SmallFileThreshold, missing)
				return dec
			}
			// total > 0 + missing == 0 means HasFullyRead would have
			// fired above; defensive fallthrough to advisory.
		}
	}

	// L5 — opaque advisory (last resort). Large file with no symbol,
	// no evidence, no grep anchor. The system has no defensible
	// specific demand. Non-blocking; LLM may judge the file unrelated.
	dec.Signal = SignalOpaqueAdvisory
	dec.Action = ActionAdvisoryHint
	dec.Reason = "no symbol match, no grounded evidence, no keyword grep history, file too large for full-read demand"
	dec.Rationale = renderOpaqueAdvisoryRationale(file, questionEntities)
	return dec
}

// singletonRegionsFromLines packs a slice of 1-based line numbers
// into the flat (start, end) pairs MissingContextRegions consumes,
// treating each line as a single-line region (start == end).
func singletonRegionsFromLines(lines []int) []int {
	out := make([]int, 0, len(lines)*2)
	for _, l := range lines {
		if l <= 0 {
			continue
		}
		out = append(out, l, l)
	}
	return out
}

// questionRelatedSymbols intersects the symbols defined in `file` (per
// the SymbolOracle) with the question-entity surface, augmented by any
// symbols already cited in evidence whose Source matches `file`. Names
// are matched case-insensitively against entity tokens.
//
// Returns deduplicated SymbolDef entries sorted by Line. repoRoot is
// threaded into sameFile so evidence Source comparisons are
// multi-platform-correct (an evidence item with absolute Source on
// Windows still matches a repo-relative anchor file).
func questionRelatedSymbols(
	file string,
	oracle SymbolOracle,
	evidence []types.EvidenceItem,
	entities []string,
	repoRoot string,
) []SymbolDef {
	if oracle == nil {
		return nil
	}
	all := oracle.SymbolsInFile(file)
	if len(all) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(entities))
	for _, e := range entities {
		key := strings.ToLower(strings.TrimSpace(e))
		if key == "" {
			continue
		}
		wanted[key] = true
	}
	for _, ev := range evidence {
		if !sameFile(ev.Source, file, repoRoot) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(ev.AnchorSymbol))
		if key == "" {
			continue
		}
		wanted[key] = true
	}
	if len(wanted) == 0 {
		return nil
	}
	out := make([]SymbolDef, 0)
	seen := make(map[string]bool)
	for _, s := range all {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if key == "" || !wanted[key] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Line < out[j].Line
	})
	return out
}

// symbolRegions packs the symbol [Line, EndLine] pairs into the flat
// int slice MissingContextRegions consumes. EndLine <= 0 is treated
// as a one-line definition at Line.
func symbolRegions(symbols []SymbolDef) []int {
	out := make([]int, 0, len(symbols)*2)
	for _, s := range symbols {
		if s.Line <= 0 {
			continue
		}
		end := s.EndLine
		if end < s.Line {
			end = s.Line
		}
		out = append(out, s.Line, end)
	}
	return out
}

// symbolNames returns the symbol Names for logging / rationale render.
func symbolNames(symbols []SymbolDef) []string {
	if len(symbols) == 0 {
		return nil
	}
	out := make([]string, 0, len(symbols))
	for _, s := range symbols {
		out = append(out, s.Name)
	}
	return out
}

// groundedEvidenceCount counts evidence items whose Source matches
// `file` AND whose grounding verdict is grounded or recovered AND
// whose tier is Tier1 or Tier2 (Tier 3 is line-text degradation —
// weaker signal, conservative to exclude). Items with no grounding
// status set are excluded. repoRoot is threaded into sameFile for
// multi-platform path comparison consistency.
func groundedEvidenceCount(file string, evidence []types.EvidenceItem, repoRoot string) int {
	count := 0
	for _, ev := range evidence {
		if !sameFile(ev.Source, file, repoRoot) {
			continue
		}
		switch ev.GroundingStatus {
		case types.GroundingGrounded, types.GroundingRecovered:
		default:
			continue
		}
		switch ev.GroundingTier {
		case types.TierLineText, types.TierSymbolTable:
			count++
		}
	}
	return count
}

// sameFile compares two file paths after canonicalisation. Caller
// supplies the repo root so multi-platform shapes converge on the
// same key (POSIX absolute, Windows backslash, leading-./ relative
// all collapse to repo-relative POSIX form). Evidence items can
// carry a less-normalised Source than the closure key — running both
// through CanonicalRepoRelative makes the comparison resilient.
//
// repoRoot empty falls back to a slash-form cleanup so tests without
// a real repo still work.
func sameFile(a, b, repoRoot string) bool {
	ca := canonicalisePath(a, repoRoot)
	cb := canonicalisePath(b, repoRoot)
	return ca != "" && ca == cb
}

// canonicalisePath is the single canonicalisation site for the
// engine. Always go through ground.CanonicalRepoRelative when
// repoRoot is non-empty (multi-platform: POSIX prefix strip, Windows
// case-insensitive volume, macOS HFS+/APFS tolerance). Empty
// repoRoot degrades to slash-form cleanup matching the pre-fix
// behaviour for unit tests that pass empty roots.
func canonicalisePath(p, repoRoot string) string {
	if p == "" {
		return ""
	}
	if repoRoot != "" {
		return ground.CanonicalRepoRelative(p, repoRoot)
	}
	cleaned := strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	return strings.TrimPrefix(cleaned, "./")
}

// renderSymbolDemandRationale builds the surgical-read rationale
// shown to the LLM. Names every uncovered region's line range AND the
// symbol(s) that region belongs to. Includes a literal read_file
// invocation the LLM can copy-paste — pre-2026-05-01 the absence of
// a copy-pasteable command let the LLM guess wrong offsets across
// retries.
func renderSymbolDemandRationale(
	file string,
	symbolNames []string,
	symbols []SymbolDef,
	missing []types.LineRange,
	contextLines int,
) string {
	var b strings.Builder
	b.WriteString("primary anchor symbol regions still unread. ")
	switch len(symbolNames) {
	case 0:
		// Defensive — shouldn't reach here without symbols
		b.WriteString("Read the missing line ranges below.")
	case 1:
		fmt.Fprintf(&b, "Symbol `%s` is the answer-bearing definition in this file.", symbolNames[0])
	default:
		fmt.Fprintf(&b, "Symbols [%s] are the answer-bearing definitions in this file.", strings.Join(symbolNames, ", "))
	}
	if len(symbols) > 0 {
		b.WriteString(" Definitions span: ")
		parts := make([]string, 0, len(symbols))
		for _, s := range symbols {
			end := s.EndLine
			if end < s.Line {
				end = s.Line
			}
			parts = append(parts, fmt.Sprintf("`%s` lines %d-%d", s.Name, s.Line, end))
		}
		b.WriteString(strings.Join(parts, "; "))
		b.WriteString(".")
	}
	if contextLines > 0 {
		fmt.Fprintf(&b, " Context window ±%d lines.", contextLines)
	}
	if len(missing) > 0 {
		b.WriteString(" Missing line ranges to read: ")
		parts := make([]string, 0, len(missing))
		for _, r := range missing {
			parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString(". ")
		// Surface the first missing range as a copy-pasteable command;
		// remaining ranges are listed above. The LLM can issue one
		// read_file per range or batch them.
		first := missing[0]
		// read_file's offset parameter is 0-based (see
		// internal/tool/builtin.go schema); LineRange.Start is
		// 1-based. types.LineToReadFileOffset bridges them so the
		// suggested invocation actually reads the demanded line
		// (forensic 2026-05-10: emitting offset=first.Start fed the
		// LLM a +1 offset that cycled forever).
		fmt.Fprintf(&b,
			"Use `read_file(path=%q, offset=%d, limit=%d)` for the first range. ",
			file, types.LineToReadFileOffset(first.Start), first.End-first.Start+1,
		)
	}
	b.WriteString("This demand is SURGICAL — read ONLY the named ranges; do not paginate the rest of the file.")
	return b.String()
}

// renderKeywordDemandRationale builds the surgical-read rationale
// for the Layer 3 negative case. Names the keyword-anchor lines and
// the missing slivers around them so the LLM cannot interpret this
// as "read percentage of file".
func renderKeywordDemandRationale(
	file string,
	anchorLines []int,
	missing []types.LineRange,
	contextLines int,
) string {
	var b strings.Builder
	b.WriteString("non-symbolic file with grep-anchor coverage gaps. ")
	switch len(anchorLines) {
	case 1:
		fmt.Fprintf(&b, "Question entity matched at line %d (±%d ctx).", anchorLines[0], contextLines)
	default:
		short := anchorLines
		if len(short) > 8 {
			short = short[:8]
		}
		parts := make([]string, 0, len(short))
		for _, l := range short {
			parts = append(parts, fmt.Sprintf("%d", l))
		}
		suffix := ""
		if len(anchorLines) > len(short) {
			suffix = fmt.Sprintf(" (+%d more)", len(anchorLines)-len(short))
		}
		fmt.Fprintf(&b, "Question entities matched at lines %s%s (±%d ctx).", strings.Join(parts, ", "), suffix, contextLines)
	}
	if len(missing) > 0 {
		b.WriteString(" Missing line ranges to read: ")
		parts := make([]string, 0, len(missing))
		for _, r := range missing {
			parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString(". ")
		first := missing[0]
		// read_file's offset parameter is 0-based (see
		// internal/tool/builtin.go schema); LineRange.Start is
		// 1-based. types.LineToReadFileOffset bridges them so the
		// suggested invocation actually reads the demanded line
		// (forensic 2026-05-10: emitting offset=first.Start fed the
		// LLM a +1 offset that cycled forever).
		fmt.Fprintf(&b,
			"Use `read_file(path=%q, offset=%d, limit=%d)` for the first range. ",
			file, types.LineToReadFileOffset(first.Start), first.End-first.Start+1,
		)
	}
	b.WriteString("This demand is SURGICAL — read ONLY the named ranges; do not paginate the rest of the file.")
	return b.String()
}

// renderSmallFileFullRationale builds the demand text for a small
// non-symbolic file whose unread slivers must be filled in. Names
// the file size + the threshold so the LLM understands WHY a full
// read is being asked here (vs the surgical demand on large files).
func renderSmallFileFullRationale(
	file string,
	totalLines, threshold int,
	missing []types.LineRange,
) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"non-symbolic file `%s` (%d lines ≤ small-file threshold %d). ",
		file, totalLines, threshold,
	)
	b.WriteString("No question-related code symbol identified by repomap (typical for config files / docs / manifests / Dockerfiles / shell scripts), and no question-keyword grep history points into it. ")
	if len(missing) > 0 {
		b.WriteString("For files this small, the system asks for the unread slivers in full so the answer-bearing line cannot be missed. Missing line ranges: ")
		parts := make([]string, 0, len(missing))
		for _, r := range missing {
			parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString(". ")
		first := missing[0]
		// read_file's offset parameter is 0-based (see
		// internal/tool/builtin.go schema); LineRange.Start is
		// 1-based. types.LineToReadFileOffset bridges them so the
		// suggested invocation actually reads the demanded line
		// (forensic 2026-05-10: emitting offset=first.Start fed the
		// LLM a +1 offset that cycled forever).
		fmt.Fprintf(&b,
			"Use `read_file(path=%q, offset=%d, limit=%d)` for the first range. ",
			file, types.LineToReadFileOffset(first.Start), first.End-first.Start+1,
		)
	}
	b.WriteString("This demand is bounded by file size — total read is at most the file's line count.")
	return b.String()
}

// renderOpaqueAdvisoryRationale builds the non-blocking advisory text
// shown to the LLM when no usable signal exists for the file. By the
// time this fires, all four upstream signals have failed: no symbol
// (file is non-symbolic), no grounded evidence, no keyword grep
// history, and the file is too large for full-read demand.
func renderOpaqueAdvisoryRationale(file string, entities []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "primary anchor `%s` is large with no defensible specific demand. ", file)
	b.WriteString("Repomap returned no symbols (non-code file), no grounded evidence has been emitted from it, no prior grep matched a question entity inside it, and the file is too large for an unconditional full-read demand. ")
	if len(entities) > 0 {
		short := entities
		if len(short) > 4 {
			short = short[:4]
		}
		fmt.Fprintf(&b, "Optional next step: grep this file for the question entities [%s] to find specific lines, then read those ranges. ", strings.Join(short, ", "))
	} else {
		b.WriteString("Optional next step: grep this file for question keywords to find specific lines, then read those ranges. ")
	}
	b.WriteString("This is ADVISORY ONLY — emit_investigation_complete will not be blocked. If you have judged the file unrelated to the question, proceed with completion.")
	return b.String()
}

// ExtractKeywordAnchors walks the supplied tool history for grep
// results and returns, per file, the 1-based line numbers where any
// question entity matched. Result map keys are canonicalised
// repo-relative paths; per-file int slices are sorted ascending,
// deduplicated, and bounded by MaxKeywordAnchorsPerFile (when > 0)
// via even sampling so the surgical-read demand cannot explode on
// high-frequency entity hits.
//
// "Match" is defined as: a grep summary line of the shape
// `path:lineno:content` (the canonical match-line format ripgrep /
// GNU grep -H produce) where the content portion contains at least
// one entity token as a case-insensitive substring. Context-line
// shapes (`path-lineno-content`) are intentionally skipped — we
// want lines the LLM CHOSE to query for, not the surrounding context.
//
// entities is the union of analyzer-derived entity names (typically
// AnalyzerHints.PrimaryEntities + Entities + DerivedEntities). Tokens
// shorter than 3 characters are dropped (case-insensitive match would
// hit too much noise).
//
// repoRoot is the absolute repository root used to canonicalise grep
// result paths (multi-platform: POSIX absolute → repo-relative,
// Windows backslash → POSIX slash, leading-./ → strip; same
// CanonicalRepoRelative semantics every other gate consumer uses).
// Empty repoRoot tolerated for tests; production callers must pass
// ctx.RepoRoot or a grep returning absolute paths will land on a
// different map key than the Phase1Ranking lookup at the gate site.
//
// nil-tolerant on every input: empty history or empty entities
// yields an empty map.
func ExtractKeywordAnchors(history []types.ToolResult, entities []string, repoRoot string) map[string][]int {
	return ExtractKeywordAnchorsCapped(history, entities, repoRoot, 0)
}

// ExtractKeywordAnchorsCapped is the cap-aware variant. maxPerFile
// > 0 limits each file's anchor count via even sampling so the
// resulting surgical demand cannot produce hundreds of read_file
// calls when a high-frequency entity matches in many lines of a
// single file. maxPerFile == 0 preserves the unbounded behaviour
// for callers that have other reasons to keep every match (tests).
func ExtractKeywordAnchorsCapped(history []types.ToolResult, entities []string, repoRoot string, maxPerFile int) map[string][]int {
	if len(history) == 0 || len(entities) == 0 {
		return nil
	}
	wanted := make([]string, 0, len(entities))
	for _, e := range entities {
		key := strings.ToLower(strings.TrimSpace(e))
		if len(key) < 3 {
			continue
		}
		wanted = append(wanted, key)
	}
	if len(wanted) == 0 {
		return nil
	}
	tmp := make(map[string]map[int]bool)
	for _, r := range history {
		if !r.Success || r.ToolName != "grep" {
			continue
		}
		for _, raw := range strings.Split(r.Summary, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || line == "--" || line[0] == '[' {
				continue
			}
			path, lineno, content, ok := parseGrepMatchLine(line)
			if !ok {
				continue
			}
			path = canonicalisePath(path, repoRoot)
			if path == "" {
				continue
			}
			lc := strings.ToLower(content)
			matched := false
			for _, kw := range wanted {
				if strings.Contains(lc, kw) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			if tmp[path] == nil {
				tmp[path] = make(map[int]bool)
			}
			tmp[path][lineno] = true
		}
	}
	if len(tmp) == 0 {
		return nil
	}
	out := make(map[string][]int, len(tmp))
	for path, lineSet := range tmp {
		lines := make([]int, 0, len(lineSet))
		for l := range lineSet {
			lines = append(lines, l)
		}
		sort.Ints(lines)
		if maxPerFile > 0 && len(lines) > maxPerFile {
			lines = sampleEvenly(lines, maxPerFile)
		}
		out[path] = lines
	}
	return out
}

// sampleEvenly returns at most n entries from src by taking every
// k-th element where k = floor(len(src) / n) (rounded so we cover
// the full range without exceeding n entries). Always preserves
// the first AND last entries so the boundary anchors stay
// represented. src MUST be sorted; output preserves order.
func sampleEvenly(src []int, n int) []int {
	if n <= 0 || len(src) <= n {
		return src
	}
	if n == 1 {
		return []int{src[0]}
	}
	out := make([]int, 0, n)
	// Step in float so we hit endpoints exactly. n samples → n-1
	// intervals between out[0] and out[n-1].
	step := float64(len(src)-1) / float64(n-1)
	for i := 0; i < n; i++ {
		idx := int(float64(i)*step + 0.5)
		if idx >= len(src) {
			idx = len(src) - 1
		}
		out = append(out, src[idx])
	}
	// Dedup in case rounding produced consecutive identical
	// indices (n very close to len(src) - 1).
	dedup := out[:0]
	for i, v := range out {
		if i == 0 || v != out[i-1] {
			dedup = append(dedup, v)
		}
	}
	return dedup
}

// parseGrepMatchLine splits a single grep output line of the
// canonical `path:lineno:content` shape. Returns ok=false on any
// other shape (e.g. context lines `path-lineno-content`, files-only
// `path`, or noise).
//
// Path may itself contain colons (Windows drives, archive members).
// Strategy: scan for the FIRST `:` immediately followed by a run of
// digits + another `:` — that's the (lineno) separator. Everything
// before it is the path; the digits are the lineno; everything after
// the second `:` is content.
func parseGrepMatchLine(line string) (path string, lineno int, content string, ok bool) {
	for i := 0; i < len(line); i++ {
		if line[i] != ':' {
			continue
		}
		j := i + 1
		for j < len(line) && line[j] >= '0' && line[j] <= '9' {
			j++
		}
		if j == i+1 || j >= len(line) || line[j] != ':' {
			continue
		}
		n := 0
		for k := i + 1; k < j; k++ {
			n = n*10 + int(line[k]-'0')
		}
		if n <= 0 {
			continue
		}
		return line[:i], n, line[j+1:], true
	}
	return "", 0, "", false
}
