package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Symbol-guided preRead constants. Centralised so the policy is
// inspectable from one spot and the render path can cross-check
// against the anchor selector's invariants (anchor window size caps,
// head budget, dedup tolerance). None of these are YAML knobs — no
// evidence yet that per-repo tuning helps, and the session-11
// "generalisation over project success" rule argues against adding
// tunables we cannot justify.
const (
	preReadMaxFiles          = 2
	preReadLineBudgetPerFile = 200

	// Anchor search.
	preReadMaxAnchorsPerFile = 2
	preReadAnchorExactBonus  = 5.0 // case-insensitive Name == entity
	preReadAnchorSubstrBonus = 2.0 // entity appears as a substring of Name
	preReadAnchorExportBonus = 0.5 // exported symbols win ties
	preReadMinEntityLen      = 3   // skip tokens shorter than this to avoid noise

	// Slice shapes.
	preReadHeadLinesSingle = 40  // head slice when 1 anchor selected
	preReadHeadLinesMulti  = 30  // head slice when 2 anchors selected
	preReadAnchorCtxBefore = 8   // lines of context before anchor.Line
	preReadAnchorCtxAfter  = 8   // lines of context after anchor.EndLine
	preReadAnchorCapTail   = 144 // hard cap on anchor window length
)

// anchorSymbol is a repomap Symbol together with the score that won
// it a slot. Wrapper exists so the sort path and the renderer both
// operate on a typed value rather than a map lookup.
type anchorSymbol struct {
	Sym   repomap.Symbol
	Score float64
}

// anchorWindow is one concrete slice of a file's source. reason is
// human-readable ("head", "anchor: ProcessRequest (function)") and
// shows up in the prompt banner so the LLM knows why it was given
// each slice. Ranges are 1-based inclusive.
type anchorWindow struct {
	Start, End int
	Reason     string
}

// selectAnchorsInFile returns the top matching symbols in `file` for
// the supplied entity list, ranked by score. Reuses repomap's
// TokenizeQuery so scoring aligns with the keyword-search ranker (no
// bespoke tokenizer — session 11 "grep for prior art" discipline).
//
// Returns nil when graph is unavailable, file is unknown, or no
// entity matches a symbol — callers degrade to head-only preRead in
// that case. Cap enforced via maxAnchors.
func selectAnchorsInFile(
	graph *repomap.Graph,
	file string,
	entities []string,
	maxAnchors int,
) []anchorSymbol {
	if graph == nil || file == "" || len(entities) == 0 || maxAnchors <= 0 {
		return nil
	}
	syms := graph.SymbolsInFile(file)
	if len(syms) == 0 {
		return nil
	}
	tokens := retrieve.TokenizeQuery(strings.Join(entities, " "))
	if len(tokens) == 0 {
		return nil
	}
	var scored []anchorSymbol
	for _, sym := range syms {
		if len(sym.Name) < preReadMinEntityLen {
			continue
		}
		nameLower := strings.ToLower(sym.Name)
		score := 0.0
		for _, tok := range tokens {
			tokText := strings.ToLower(tok.Text)
			if len(tokText) < preReadMinEntityLen {
				continue
			}
			switch {
			case nameLower == tokText:
				score += preReadAnchorExactBonus * tok.Weight
			case strings.Contains(nameLower, tokText):
				score += preReadAnchorSubstrBonus * tok.Weight
			}
		}
		if score <= 0 {
			continue
		}
		if sym.Exported {
			score += preReadAnchorExportBonus
		}
		scored = append(scored, anchorSymbol{Sym: sym, Score: score})
	}
	if len(scored) == 0 {
		return nil
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		// Tie-break by Line asc so reproducible ordering matches the
		// "earlier in file wins ties" convention.
		return scored[i].Sym.Line < scored[j].Sym.Line
	})
	if len(scored) > maxAnchors {
		scored = scored[:maxAnchors]
	}
	return scored
}

// planAnchorWindows turns a list of anchors plus a file length into a
// merged, sorted, non-overlapping slice list that fits under the
// per-file line budget. The head slice is always present; anchor
// slices are placed at sym.Line ± context with a cap on total anchor
// window height. Adjacent windows are merged so the renderer does not
// emit 3-line gap markers inside what is effectively one read.
func planAnchorWindows(totalLines int, anchors []anchorSymbol) []anchorWindow {
	if totalLines <= 0 {
		return nil
	}
	// Small file: one full-file window.
	if totalLines <= preReadLineBudgetPerFile {
		return []anchorWindow{{
			Start:  1,
			End:    totalLines,
			Reason: "full file",
		}}
	}
	// No usable anchors → head-only window (head budget == full
	// per-file budget so we show as much of the top as possible).
	if len(anchors) == 0 {
		return []anchorWindow{{
			Start:  1,
			End:    min(preReadLineBudgetPerFile, totalLines),
			Reason: "head (no anchor hit)",
		}}
	}
	headLen := preReadHeadLinesSingle
	if len(anchors) >= 2 {
		headLen = preReadHeadLinesMulti
	}
	windows := []anchorWindow{{
		Start:  1,
		End:    min(headLen, totalLines),
		Reason: "head",
	}}
	for _, a := range anchors {
		w := anchorWindowFromSymbol(a.Sym, totalLines)
		if w.Start == 0 {
			continue
		}
		windows = append(windows, w)
	}
	windows = mergeAnchorWindows(windows)
	windows = trimToBudget(windows, preReadLineBudgetPerFile)
	return windows
}

// anchorWindowFromSymbol computes a window for one anchor. Falls
// back to Line + 60 when EndLine is missing (parser didn't emit it —
// common for struct fields or small Go methods).
func anchorWindowFromSymbol(sym repomap.Symbol, totalLines int) anchorWindow {
	if sym.Line <= 0 {
		return anchorWindow{}
	}
	end := sym.EndLine
	if end < sym.Line {
		end = sym.Line + 60
	}
	start := sym.Line - preReadAnchorCtxBefore
	if start < 1 {
		start = 1
	}
	end += preReadAnchorCtxAfter
	if end > totalLines {
		end = totalLines
	}
	// Cap the anchor window height so a 1000-line generated file with
	// one matching top-level function cannot swallow the whole line
	// budget.
	if end-start+1 > preReadAnchorCapTail {
		end = start + preReadAnchorCapTail - 1
		if end > totalLines {
			end = totalLines
		}
	}
	symKind := sym.Kind
	if symKind == "" {
		symKind = "symbol"
	}
	return anchorWindow{
		Start:  start,
		End:    end,
		Reason: fmt.Sprintf("anchor `%s` (%s)", sym.Name, symKind),
	}
}

// mergeAnchorWindows sorts by Start and collapses overlapping or
// adjacent (End+1 == Start) windows. When two windows merge their
// reasons are concatenated so the renderer banner still reports both
// contributions ("head + anchor `Foo`").
func mergeAnchorWindows(in []anchorWindow) []anchorWindow {
	if len(in) == 0 {
		return nil
	}
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Start != in[j].Start {
			return in[i].Start < in[j].Start
		}
		return in[i].End < in[j].End
	})
	out := make([]anchorWindow, 0, len(in))
	cur := in[0]
	for i := 1; i < len(in); i++ {
		next := in[i]
		if next.Start <= cur.End+1 {
			if next.End > cur.End {
				cur.End = next.End
			}
			if next.Reason != "" && !strings.Contains(cur.Reason, next.Reason) {
				cur.Reason = cur.Reason + " + " + next.Reason
			}
			continue
		}
		out = append(out, cur)
		cur = next
	}
	out = append(out, cur)
	return out
}

// trimToBudget shrinks windows from the tail inward when total line
// count exceeds budget. The head window (Start==1) is always kept
// first; subsequent windows lose lines from the end before losing
// lines from the start, so the anchor point stays visible.
func trimToBudget(windows []anchorWindow, budget int) []anchorWindow {
	if budget <= 0 || len(windows) == 0 {
		return windows
	}
	total := 0
	for _, w := range windows {
		total += w.End - w.Start + 1
	}
	if total <= budget {
		return windows
	}
	// Overflow: walk from the end and trim. Keep at least 1 line per
	// window so the banner still surfaces every anchor.
	overflow := total - budget
	for i := len(windows) - 1; i >= 0 && overflow > 0; i-- {
		span := windows[i].End - windows[i].Start + 1
		if span <= 1 {
			continue
		}
		cut := overflow
		if cut > span-1 {
			cut = span - 1
		}
		windows[i].End -= cut
		overflow -= cut
	}
	return windows
}

// renderPreReadFile formats one file's slices as a prompt section.
// Banner format mirrors the read_file tool's gutter convention so
// CGEC's LineIndex consumers could in principle parse preRead
// content the same way (Commit A does not wire that today; the
// fields are identical to keep the door open without enabling it).
func renderPreReadFile(path string, allLines []string, windows []anchorWindow) string {
	if len(windows) == 0 || len(allLines) == 0 {
		return ""
	}
	total := len(allLines)
	var b strings.Builder
	fmt.Fprintf(&b, "#### `%s`", path)
	if len(windows) == 1 && windows[0].Start == 1 && windows[0].End >= total {
		fmt.Fprintf(&b, " (full file, %d lines)\n", total)
	} else {
		fmt.Fprintf(&b, " (%d slices of %d total lines — issue read_file to fill gaps)\n", len(windows), total)
	}
	prevEnd := 0
	for i, w := range windows {
		if w.Start > prevEnd+1 {
			gapStart := prevEnd + 1
			if gapStart < 1 {
				gapStart = 1
			}
			if prevEnd > 0 {
				fmt.Fprintf(&b, "\n(gap: lines %d-%d omitted)\n", gapStart, w.Start-1)
			}
		}
		fmt.Fprintf(&b, "\n**Slice %d (%s, lines %d-%d):**\n```\n", i+1, w.Reason, w.Start, w.End)
		end := w.End
		if end > total {
			end = total
		}
		for line := w.Start; line <= end; line++ {
			src := ""
			if line-1 < len(allLines) {
				src = allLines[line-1]
			}
			fmt.Fprintf(&b, "%d\t%s\n", line, src)
		}
		b.WriteString("```\n")
		prevEnd = w.End
	}
	if prevEnd < total {
		fmt.Fprintf(&b, "(tail: lines %d-%d omitted)\n", prevEnd+1, total)
	}
	b.WriteString("\n")
	return b.String()
}

// symbolGuidedPreRead is the top-level entry point for Commit B's
// preRead strategy. When graph or entities are unavailable it returns
// ("", nil), letting the caller fall back to the head-only S2a path.
// Otherwise it renders per-file multi-slice content aligned with the
// analyzer's entity hits and emits preReadInjection entries whose
// Ranges match the slices exactly — so CGEC's HasReadLine agrees
// with what the LLM actually saw, line for line.
func symbolGuidedPreRead(
	repoRoot string,
	files []string,
	graph *repomap.Graph,
	entities []string,
	maxFiles int,
) (rendered string, injections []preReadInjection) {
	if repoRoot == "" || len(files) == 0 || maxFiles <= 0 {
		return "", nil
	}
	var b strings.Builder
	for _, f := range files {
		if len(injections) >= maxFiles {
			break
		}
		absPath := filepath.Join(repoRoot, f)
		data, err := os.ReadFile(absPath)
		if err != nil {
			logging.Debug("[explorer] preRead skipped %q: %v (analyzer RequiredFiles may contain ghost path)", f, err)
			continue
		}
		allLines := strings.Split(string(data), "\n")
		anchors := selectAnchorsInFile(graph, f, entities, preReadMaxAnchorsPerFile)
		windows := planAnchorWindows(len(allLines), anchors)
		if len(windows) == 0 {
			continue
		}
		b.WriteString(renderPreReadFile(f, allLines, windows))
		ranges := make([]types.LineRange, 0, len(windows))
		for _, w := range windows {
			ranges = append(ranges, types.LineRange{Start: w.Start, End: w.End})
		}
		injections = append(injections, preReadInjection{
			Path:   f,
			Ranges: ranges,
		})
		if len(anchors) > 0 {
			logging.Debug("[explorer] preRead file=%s anchors=%d windows=%d firstAnchor=%s@%d",
				f, len(anchors), len(windows), anchors[0].Sym.Name, anchors[0].Sym.Line)
		} else {
			logging.Debug("[explorer] preRead file=%s no-anchor → head-only (%d windows)", f, len(windows))
		}
	}
	return b.String(), injections
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// repomapGraphFromCtx extracts the cached repomap graph from the
// explorer's AgentContext. Returns nil when the Mutable side-channel
// is absent (sub-agent path, unit tests) or when the analyzer stage
// did not populate it — callers degrade to head-only preRead.
func repomapGraphFromCtx(ctx *types.AgentContext) *repomap.Graph {
	if ctx == nil {
		return nil
	}
	if ctx.Mutable != nil {
		if g, ok := ctx.Mutable.SearchGraph().(*repomap.Graph); ok {
			return g
		}
	}
	if g, ok := ctx.SearchGraph.(*repomap.Graph); ok {
		return g
	}
	return nil
}
