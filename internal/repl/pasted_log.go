package repl

import (
	"regexp"
	"strings"
)

// splitPastedLog scans a REPL request body for a contiguous block of
// log-shaped lines (≥ minLogLines matches in a row) and, when found,
// returns (cleanedRequest, detectedLog). The detected block is
// removed from the cleanedRequest so the analyzer's TermGraph /
// keyword expansion stays clean; the block is surfaced separately so
// the caller can push it onto BusContext.AttachedLog and let the
// log_triage pre-stage parse it with the LLM.
//
// When no contiguous block meets the thresholds, returns (line, "").
//
// Heuristic intentionally conservative: a single `foo.go:42`
// reference in prose does NOT trigger; we require minLogLines (3)
// consecutive matches AND minLogBytes (50) total bytes so a user
// asking "where is foo.go:42 handled" is not mis-routed.
func splitPastedLog(line string) (cleaned, detected string) {
	const (
		minLogLines = 3
		minLogBytes = 50
	)
	if len(line) < minLogBytes {
		return line, ""
	}
	lines := strings.Split(line, "\n")
	if len(lines) < minLogLines {
		return line, ""
	}
	// Scan for the longest run where log-shaped lines dominate.
	// Python tracebacks and Go panic blocks interleave frame lines
	// with source excerpts / function headers that don't match any
	// pattern, so a strict "all consecutive must match" rule would
	// miss the common case. Allow up to maxGap non-matching lines
	// inside a run; the run extends as long as the next matching
	// line appears within maxGap of the last match.
	const maxGap = 1
	bestStart, bestEnd, bestMatches := -1, -1, 0
	curStart, curMatches, lastMatchIdx := -1, 0, -1
	for i, ln := range lines {
		if isLogShapedLine(ln) {
			if curStart < 0 {
				curStart = i
			}
			curMatches++
			lastMatchIdx = i
			continue
		}
		if curStart < 0 {
			continue
		}
		if i-lastMatchIdx > maxGap {
			if curMatches > bestMatches {
				bestStart, bestEnd, bestMatches = curStart, lastMatchIdx+1, curMatches
			}
			curStart, curMatches, lastMatchIdx = -1, 0, -1
		}
	}
	if curMatches > bestMatches {
		bestStart, bestEnd, bestMatches = curStart, lastMatchIdx+1, curMatches
	}
	if bestMatches < minLogLines || bestStart < 0 {
		return line, ""
	}
	block := strings.Join(lines[bestStart:bestEnd], "\n")
	if len(block) < minLogBytes {
		return line, ""
	}
	// Reassemble request with the block removed. Collapse the gap to
	// a single blank line so surrounding natural-language stays
	// readable.
	var b strings.Builder
	if bestStart > 0 {
		b.WriteString(strings.Join(lines[:bestStart], "\n"))
	}
	if bestStart > 0 && bestEnd < len(lines) {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if bestEnd < len(lines) {
		b.WriteString(strings.Join(lines[bestEnd:], "\n"))
	}
	cleaned = strings.TrimSpace(b.String())
	return cleaned, block
}

// logLinePatterns are the fragments splitPastedLog treats as
// "definitely log-shaped". Each pattern matches a single physical
// line; splitPastedLog requires minLogLines matches inside the
// block (with a small gap tolerance for tracebacks that interleave
// frames and source excerpts) before promoting it.
//
// Grouped by abstract signal, NOT by language, so adding a new
// runtime means broadening the relevant group rather than appending
// a language-specific entry. Negative tests in pasted_log_test.go
// guard against prose / code / markdown false positives.
var logLinePatterns = []*regexp.Regexp{
	// ── 1. Timestamp prefix ────────────────────────────────────────
	// ISO-ish with date + time: `2026-04-21T14:54:37.362` or
	// `[2026-04-21 14:54:37]`.
	regexp.MustCompile(`^[\s\[]*\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`),
	// Syslog / journal: `Apr 21 14:54:37 hostname daemon[123]: ...`
	regexp.MustCompile(`^[\s\[]*[A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}\b`),
	// Time-only prefix followed by a non-space token:
	// `14:54:37.362 INFO ...`
	regexp.MustCompile(`^[\s\[]*\d{2}:\d{2}:\d{2}([.,]\d+)?\s+\S`),

	// ── 2. Log-level prefix ────────────────────────────────────────
	// Canonical names, common short forms, bracketed variants.
	// Case-insensitive: zap/logrus/slog often ship lowercase; Java
	// util.logging uses `SEVERE`/`FINE`. Android logcat's single-
	// letter tags are intentionally skipped (too generic to be
	// distinguishable from prose).
	regexp.MustCompile(`(?i)^[\s\[]*(ERROR|ERR|WARNING|WARN|INFO|INF|DEBUG|DBG|TRACE|FATAL|NOTICE|CRITICAL|CRIT|SEVERE|FINE|FINER|FINEST)\b`),

	// ── 3. Stack frame — "at X(Y:N)" style ────────────────────────
	// Covers Java / Kotlin / Scala (`at com.foo.Bar.baz(File.java:42)`)
	// AND JavaScript / Node (`at Object.bar (/app/foo.js:42:13)`),
	// where the function name and opening paren are separated by a
	// space.
	regexp.MustCompile(`^\s+at\s+[\w.$<>/-]+\s*\([^)]*:\d+(:\d+)?\)`),

	// ── 4. Stack frame — file.ext:line ────────────────────────────
	// Most runtimes print a path-like prefix with a line number on
	// their stack frames. Broad extension set covers the languages
	// a codrax user is plausibly pasting from. Anchored at start of
	// line with optional leading whitespace / `at ` to avoid firing
	// on a file:line mentioned mid-sentence in prose.
	regexp.MustCompile(`^\s+(at\s+)?/?[\w./_-]+\.(go|js|mjs|cjs|ts|tsx|jsx|py|pyi|rb|rs|c|cc|cpp|cxx|h|hh|hpp|java|kt|kts|cs|fs|php|swift|m|mm|scala|clj|ex|exs|dart|lua|pl|pm|zig):\d+($|[\s:+])`),

	// Python-specific frame header (no file extension needed).
	regexp.MustCompile(`^\s*File "[^"]+", line \d+`),

	// ── 5. Stack frame — numbered hex prefix ──────────────────────
	// GDB / AddressSanitizer: `    #0 0x7f3a... in func at file:42`
	// Rust: `   1: 0x... - std::panicking::...`
	regexp.MustCompile(`^\s*(#\d+|\d+:)\s+0x[0-9a-fA-F]+\b`),

	// ── 6. Error / panic / exception header ───────────────────────
	// Language-agnostic "something bad starts here" markers,
	// anchored at column 0 (no leading whitespace) so they don't
	// fire on `panic("foo")` in source code — source-code paste has
	// an indented `panic(...)` call.
	regexp.MustCompile(`^(panic|runtime error|fatal error)\b`),          // Go
	regexp.MustCompile(`^goroutine \d+ \[`),                             // Go dump header
	regexp.MustCompile(`^thread ['"][^'"]+['"] panicked\b`),             // Rust
	regexp.MustCompile(`^Traceback \(most recent call last\)`),          // Python
	regexp.MustCompile(`^[\w.]+(Exception|Error|Panic|Fault)\b[:\s]`),   // Java / JS / .NET / etc.
	regexp.MustCompile(`^\s*(Caused by|caused by):\s`),                  // Java cause chain

	// ── 7. Compiler / linker diagnostic ───────────────────────────
	// `foo.c:42:10: error: ...` / `bar.rs:7:5: warning: ...`
	regexp.MustCompile(`^[\w./_-]+\.\w+:\d+:\d+:\s+(error|warning|note|fatal)\b`),
}

// isLogShapedLine reports whether ln matches any of logLinePatterns.
func isLogShapedLine(ln string) bool {
	if ln == "" {
		return false
	}
	for _, re := range logLinePatterns {
		if re.MatchString(ln) {
			return true
		}
	}
	return false
}
