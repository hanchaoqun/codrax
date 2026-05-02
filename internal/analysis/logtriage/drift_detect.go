package logtriage

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// DefaultDriftLineGap is the per-direction tolerance (in lines) within
// which a log-claimed line and a current-code definition are considered
// "close enough — no drift". Frames whose claimed line falls inside
// [defLine, endLine] of a function with the same name resolve to
// DriftStatusNone regardless of this gap; the gap matters when the
// claim points BEFORE defLine or AFTER endLine, which can happen with
// imprecise stack frames (pre-amble lines, post-return lines).
//
// Tunable but kept at a small constant for now — the AuthorityCeiling
// axis ladder coarsens any sub-line precision into "factual or not",
// so 2-line slack is plenty.
const DefaultDriftLineGap = 2

// DetectDriftForFrame classifies the drift relationship between a log
// frame's claimed (file, line, func) tuple and the current state of
// the repo. Pure function: same inputs always produce the same output;
// no side effects on locator, frame, or any external state.
//
// Algorithm (5 cases, evaluated in priority order):
//
//  1. DriftStatusNone — frame.Func is defined in frame.File and
//     frame.Line falls inside [defLine - gap, endLine + gap]. Log and
//     current code agree.
//
//  2. DriftStatusLineDrift — frame.Func is defined in frame.File but
//     the def line is outside the [frame.Line - gap, frame.Line + gap]
//     window. Same file + same function name; lines have shifted.
//
//  3. DriftStatusTailRename — frame.File is indexed but the symbol
//     locator has no definition for frame.Func anywhere. The function
//     was likely renamed inside frame.File. NOTE: this branch fires
//     only when frame.File contains AT LEAST ONE symbol — an empty
//     file index would be ambiguous (could be DriftStatusUnmappable
//     instead).
//
//  4. DriftStatusFileMoved — frame.File doesn't index frame.Func, but
//     frame.Func IS defined elsewhere in the repo. Returns the new
//     location in AnchoredFile/AnchoredLine.
//
//  5. DriftStatusUnmappable — frame.File and frame.Func cannot be
//     located in current code at all. Function/file removed.
//
// Inputs:
//
//   - frame: the LogFrame being classified. Must have a non-empty
//     File and non-empty Func to produce a useful verdict; missing
//     either yields DriftStatusUnknown (the zero value) so callers
//     default to legacy behaviour.
//
//   - locator: the repo's SymbolLocator. nil locator yields
//     DriftStatusUnknown — drift detection requires real graph
//     access, and the AuthorityCeiling consumer treats Unknown as
//     factual-equivalent for byte-identical legacy behaviour.
//
// Output: a FrameDrift with Status set and AnchoredFile/Line/Func
// populated where the resolution succeeded. OriginalFile/Line/Func
// always reflect the inputs from frame.
func DetectDriftForFrame(frame types.LogFrame, locator types.SymbolLocator) types.FrameDrift {
	out := types.FrameDrift{
		OriginalFile: frame.File,
		OriginalLine: frame.Line,
		OriginalFunc: frame.Func,
	}
	if locator == nil {
		return out
	}
	funcName := normalizeFrameFuncForDrift(frame.Func)
	if funcName == "" || frame.File == "" {
		return out
	}
	frameFile := normalizeFrameFile(frame.File)

	// Candidate definitions of funcName anywhere in repo.
	candidates := locator.LocateSymbol(funcName)

	// Pass 1: same-file match (handles DriftStatusNone + DriftStatusLineDrift).
	for _, loc := range candidates {
		if normalizeFrameFile(loc.File) != frameFile {
			continue
		}
		if frame.Line == 0 {
			// Line missing on the log side; same-file + same-name match
			// is the strongest signal we can give without a line.
			out.Status = types.DriftStatusNone
			out.AnchoredFile = loc.File
			out.AnchoredLine = loc.Line
			out.AnchoredFunc = funcName
			return out
		}
		if lineWithin(frame.Line, loc.Line, loc.EndLine, DefaultDriftLineGap) {
			out.Status = types.DriftStatusNone
			out.AnchoredFile = loc.File
			out.AnchoredLine = loc.Line
			out.AnchoredFunc = funcName
			return out
		}
		// Same file + same function, but the log's claimed line is
		// outside the function's current span — line drift.
		out.Status = types.DriftStatusLineDrift
		out.AnchoredFile = loc.File
		out.AnchoredLine = loc.Line
		out.AnchoredFunc = funcName
		return out
	}

	// Pass 2: cross-file match (DriftStatusFileMoved). Pick the first
	// candidate (locator's stable iteration order is the tiebreaker).
	if len(candidates) > 0 {
		loc := candidates[0]
		out.Status = types.DriftStatusFileMoved
		out.AnchoredFile = loc.File
		out.AnchoredLine = loc.Line
		out.AnchoredFunc = funcName
		return out
	}

	// Pass 3: tail rename — frame.File indexed, frame.Func not present
	// anywhere. The function was renamed within frame.File.
	if siblings := locator.SymbolsInFile(frameFile); len(siblings) > 0 {
		out.Status = types.DriftStatusTailRename
		out.AnchoredFile = frameFile
		// AnchoredLine left at 0 — caller knows the renamed symbol
		// is one of locator.SymbolsInFile but cannot pick which.
		out.AnchoredFunc = ""
		return out
	}

	// Pass 4: unmappable. Both file and function gone (or were never
	// indexed — pre-existing extractor gap, but indistinguishable from
	// "removed" at this layer; the renderer hedges accordingly).
	out.Status = types.DriftStatusUnmappable
	return out
}

// normalizeFrameFuncForDrift extracts the bare function/method name
// from a LogFrame.Func that may carry receiver / package / argument
// decoration. Examples (input → output):
//
//	"main.main"                               → "main"
//	"runtime.gopark"                          → "gopark"
//	"(*MutableState).SetLogTriage"            → "SetLogTriage"
//	"github.com/foo/bar.(*Server).Run"        → "Run"
//	"com.example.Foo.bar"                     → "bar"
//	"Foo::bar(int, char*)"                    → "bar"
//
// Mirrors the tail-extraction logic the existing
// LogSourceDriftAnchor pipeline uses, simplified to the minimum
// needed for symbol-table lookup.
func normalizeFrameFuncForDrift(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Strip a balanced trailing argument list. We walk from the END
	// looking for an unmatched '(' so prefix occurrences (e.g. Go's
	// receiver "(*MutableState).SetLogTriage") are preserved.
	if strings.HasSuffix(s, ")") {
		depth := 0
		open := -1
		for i := len(s) - 1; i >= 0; i-- {
			switch s[i] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					open = i
				}
			}
			if open >= 0 {
				break
			}
		}
		if open >= 0 {
			s = strings.TrimSpace(s[:open])
		}
	}
	// Take last segment after "::" (C++/Rust namespace).
	if idx := strings.LastIndex(s, "::"); idx >= 0 {
		s = s[idx+2:]
	}
	// Take last segment after "." (Go/Java/Python). Receiver decoration
	// like "(*Type)" before the method name is shed in the same pass
	// because LastIndexByte('.') hits the dot AFTER the closing ')'.
	if idx := strings.LastIndexByte(s, '.'); idx >= 0 {
		s = s[idx+1:]
	}
	return strings.TrimSpace(s)
}

// normalizeFrameFile produces a forward-slashed, cleaned repo-relative
// path for stable map keys / comparisons. Mirrors
// types/answer_surface_plan.go's normalisation so the two sides agree.
func normalizeFrameFile(p string) string {
	s := strings.TrimSpace(strings.ReplaceAll(p, `\`, `/`))
	if s == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(s))
}

// lineWithin reports whether claimed falls inside [defLine - gap,
// endLine + gap] using inclusive bounds. endLine == 0 collapses to
// "any line at or after defLine" (some extractors emit only Line, not
// EndLine — we do not penalise their imprecision).
func lineWithin(claimed, defLine, endLine, gap int) bool {
	if claimed <= 0 || defLine <= 0 {
		return false
	}
	lo := defLine - gap
	if lo < 1 {
		lo = 1
	}
	hi := endLine
	if hi <= 0 {
		hi = defLine + gap
	} else {
		hi = hi + gap
	}
	return claimed >= lo && claimed <= hi
}
