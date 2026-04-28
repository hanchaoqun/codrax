package render

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ColorMode controls when ANSI escape codes are emitted by
// ColorizeUnifiedDiff. The CLI exposes this via --color, the REPL
// inherits it via Config.ColorMode.
type ColorMode int

const (
	// ColorAuto enables colors when the writer is a TTY AND the
	// NO_COLOR environment variable is unset. This is the default.
	ColorAuto ColorMode = iota
	// ColorAlways forces ANSI escape codes regardless of the writer
	// kind — useful when the user pipes through `less -R` or similar.
	ColorAlways
	// ColorNever strips all ANSI output. Forced by NO_COLOR and by
	// `--color=never`; also chosen when the resolved writer is not a
	// TTY under ColorAuto.
	ColorNever
)

// ParseColorMode maps the CLI flag string to a ColorMode. Unknown
// values fall back to ColorAuto so a typo doesn't break the run.
// Empty string is treated as the default ("auto").
func ParseColorMode(s string) ColorMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "always", "yes", "on", "true":
		return ColorAlways
	case "never", "no", "off", "false":
		return ColorNever
	}
	return ColorAuto
}

// ResolveColorMode collapses the user-supplied mode + writer state +
// environment into a concrete decision (ColorAlways or ColorNever).
// All downstream colorisers consult this function — the rules live
// in one place so the policy stays consistent across REPL output,
// pipeline rendering, and ad-hoc CLI commands.
//
// Order of precedence (highest first):
//
//  1. NO_COLOR environment variable (any non-empty value) — defacto
//     standard, see https://no-color.org . Always wins so users
//     building dumb-terminal tooling can rely on it.
//  2. Explicit ColorAlways — user said yes, honour it even when
//     stdout is being piped (the user may know they will run it
//     through `less -R`).
//  3. Explicit ColorNever — symmetric to the above.
//  4. ColorAuto — TTY detection on the writer. Off when the writer
//     is not a TTY (pipe, file redirect, log capture).
func ResolveColorMode(mode ColorMode, w io.Writer) ColorMode {
	if v := os.Getenv("NO_COLOR"); v != "" {
		return ColorNever
	}
	switch mode {
	case ColorAlways:
		return ColorAlways
	case ColorNever:
		return ColorNever
	}
	if isTTY(w) {
		return ColorAlways
	}
	return ColorNever
}

// isTTY reports whether w writes to a terminal. Returns false for
// any writer that is not an *os.File (bytes.Buffer in tests, etc.)
// — the typical safe answer.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ANSI SGR sequences. Kept inline rather than pulled from lipgloss
// because the colors here are uncomplicated and we don't want a
// lipgloss.Style allocation per line.
const (
	ansiReset  = "\x1b[0m"
	ansiAdd    = "\x1b[32m"            // green
	ansiDel    = "\x1b[31m"            // red
	ansiHunk   = "\x1b[36m"            // cyan
	ansiHeader = "\x1b[1m"             // bold
	ansiMeta   = "\x1b[2m"             // dim
)

// ColorizeUnifiedDiff scans a unified-diff text and wraps each line
// with ANSI SGR codes. The input format is the standard `git diff` /
// `diff -u` shape; nothing fancy:
//
//   - "+++ " / "--- "  → bold (file headers)
//   - "@@ "            → cyan (hunk header)
//   - "+"              → green (added line)
//   - "-"              → red (removed line)
//   - "diff --git ..." → bold + dim
//   - "index ..."      → dim
//   - any other        → unchanged
//
// When mode resolves to ColorNever, the input is returned unchanged
// (R6-style byte identity). The function never errors — input that
// is not a real unified diff is simply rendered with whatever lines
// happen to match the prefixes above.
//
// Note: the dual-prefix detection ("--- " before "-" and "+++ "
// before "+") matters; otherwise file-header lines would be painted
// red/green like deletion/addition lines.
func ColorizeUnifiedDiff(text string, mode ColorMode, w io.Writer) string {
	if ResolveColorMode(mode, w) == ColorNever {
		return text
	}
	if text == "" {
		return text
	}
	var b strings.Builder
	b.Grow(len(text) + 64)
	// Walk line by line; preserve trailing newlines exactly so the
	// caller's existing layout is not disturbed.
	for {
		nl := strings.IndexByte(text, '\n')
		var line string
		if nl < 0 {
			line = text
		} else {
			line = text[:nl]
		}
		colorize := func(prefix string) {
			b.WriteString(prefix)
			b.WriteString(line)
			b.WriteString(ansiReset)
		}
		switch {
		case strings.HasPrefix(line, "+++ "):
			colorize(ansiHeader + ansiAdd)
		case strings.HasPrefix(line, "--- "):
			colorize(ansiHeader + ansiDel)
		case strings.HasPrefix(line, "@@"):
			colorize(ansiHunk)
		case strings.HasPrefix(line, "diff --git"):
			colorize(ansiHeader)
		case strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "old mode"),
			strings.HasPrefix(line, "new mode"),
			strings.HasPrefix(line, "rename from"),
			strings.HasPrefix(line, "rename to"),
			strings.HasPrefix(line, "similarity index"),
			strings.HasPrefix(line, "deleted file"),
			strings.HasPrefix(line, "new file"):
			colorize(ansiMeta)
		case strings.HasPrefix(line, "+ "), line == "+":
			// "+ " with a trailing space is the create-kind body
			// renderer in repl/plan_diff.go (kind=create, kind=delete
			// use a "+ " / "- " prefix). Treat as add.
			colorize(ansiAdd)
		case strings.HasPrefix(line, "- "), line == "-":
			colorize(ansiDel)
		case strings.HasPrefix(line, "+"):
			colorize(ansiAdd)
		case strings.HasPrefix(line, "-"):
			colorize(ansiDel)
		default:
			b.WriteString(line)
		}
		if nl < 0 {
			break
		}
		b.WriteByte('\n')
		text = text[nl+1:]
	}
	return b.String()
}
