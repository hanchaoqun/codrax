package render

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestParseColorMode covers the three explicit settings + auto
// fallback. Empty / unknown strings collapse to ColorAuto so a typo
// in the CLI flag doesn't surprise the user with no color.
func TestParseColorMode(t *testing.T) {
	cases := map[string]ColorMode{
		"always": ColorAlways,
		"yes":    ColorAlways,
		"on":     ColorAlways,
		"true":   ColorAlways,
		"never":  ColorNever,
		"no":     ColorNever,
		"off":    ColorNever,
		"false":  ColorNever,
		"auto":   ColorAuto,
		"":       ColorAuto,
		"junk":   ColorAuto, // typo → auto, never silently fail-loud
		"  ALWAYS  ": ColorAlways, // trim + lowercase
	}
	for in, want := range cases {
		if got := ParseColorMode(in); got != want {
			t.Errorf("ParseColorMode(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestResolveColorMode_NoColorEnvWins covers the highest-precedence
// override: any non-empty NO_COLOR forces ColorNever even when the
// user passed --color=always. This is the no-color.org defacto
// standard and dumb-terminal tooling depends on it.
func TestResolveColorMode_NoColorEnvWins(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, mode := range []ColorMode{ColorAuto, ColorAlways, ColorNever} {
		if got := ResolveColorMode(mode, os.Stdout); got != ColorNever {
			t.Errorf("NO_COLOR=1 + mode=%v: got %v, want ColorNever", mode, got)
		}
	}
}

// TestResolveColorMode_ExplicitOverridesTTY covers the user-said-yes
// path: ColorAlways must be honoured even when the writer is not a
// TTY (e.g. user pipes through `less -R`).
func TestResolveColorMode_ExplicitOverridesTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var buf bytes.Buffer // not a TTY
	if got := ResolveColorMode(ColorAlways, &buf); got != ColorAlways {
		t.Errorf("ColorAlways with non-TTY writer: got %v, want ColorAlways", got)
	}
	if got := ResolveColorMode(ColorNever, &buf); got != ColorNever {
		t.Errorf("ColorNever with non-TTY writer: got %v, want ColorNever", got)
	}
}

// TestResolveColorMode_AutoOffWhenNotTTY guards the most common
// case: pipe/file redirect under default mode → no escape codes
// (no terminal pollution in CI logs / files).
func TestResolveColorMode_AutoOffWhenNotTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var buf bytes.Buffer
	if got := ResolveColorMode(ColorAuto, &buf); got != ColorNever {
		t.Errorf("ColorAuto with bytes.Buffer (non-TTY): got %v, want ColorNever", got)
	}
}

// TestColorizeUnifiedDiff_DisabledIsByteIdentical is the R6-style
// drift guard for the colorizer: when the resolver decides "off",
// the input passes through unchanged. Future refactors that
// accidentally always emit escapes break this test.
func TestColorizeUnifiedDiff_DisabledIsByteIdentical(t *testing.T) {
	t.Setenv("NO_COLOR", "1") // force off
	in := "diff --git a/x b/x\n@@ -1,2 +1,2 @@\n-old\n+new\n"
	var buf bytes.Buffer
	got := ColorizeUnifiedDiff(in, ColorAlways, &buf)
	if got != in {
		t.Errorf("byte-identity violation under NO_COLOR=1:\n got: %q\n in:  %q", got, in)
	}
}

// TestColorizeUnifiedDiff_AddDelRecognised is the headline behaviour:
// `+`/`-` lines pick up green/red, `@@` picks up cyan, file headers
// pick up bold+color. Asserts presence of escape codes around the
// expected payload.
func TestColorizeUnifiedDiff_AddDelRecognised(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	in := strings.Join([]string{
		"--- a/x.go",
		"+++ b/x.go",
		"@@ -1,2 +1,2 @@",
		"-old line",
		"+new line",
		" context",
	}, "\n") + "\n"
	got := ColorizeUnifiedDiff(in, ColorAlways, os.Stdout)
	checks := []struct {
		want   string
		reason string
	}{
		{ansiHeader + ansiDel + "--- a/x.go" + ansiReset, "minus header bold+red"},
		{ansiHeader + ansiAdd + "+++ b/x.go" + ansiReset, "plus header bold+green"},
		{ansiHunk + "@@ -1,2 +1,2 @@" + ansiReset, "hunk header cyan"},
		{ansiDel + "-old line" + ansiReset, "removal red"},
		{ansiAdd + "+new line" + ansiReset, "addition green"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.want) {
			t.Errorf("missing %s; got=%q", c.reason, got)
		}
	}
	// Context line passes through verbatim — no escape code wrapping.
	if !strings.Contains(got, " context\n") {
		t.Errorf("context line should pass through unchanged; got=%q", got)
	}
	if strings.Contains(got, ansiAdd+" context") || strings.Contains(got, ansiDel+" context") {
		t.Errorf("context line wrapped with color: %q", got)
	}
}

// TestColorizeUnifiedDiff_PrefixSpaceFormFromPlanShow covers the
// kind=create / kind=delete shape from internal/repl/plan_diff.go:
// "+ <line>" / "- <line>" with explicit space prefix. These must
// also colorize so /plan show looks consistent with udiff output.
func TestColorizeUnifiedDiff_PrefixSpaceFormFromPlanShow(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	in := "+ added line\n- removed line\n"
	got := ColorizeUnifiedDiff(in, ColorAlways, os.Stdout)
	if !strings.Contains(got, ansiAdd+"+ added line"+ansiReset) {
		t.Errorf("plan_diff.go '+ ' prefix not green: %q", got)
	}
	if !strings.Contains(got, ansiDel+"- removed line"+ansiReset) {
		t.Errorf("plan_diff.go '- ' prefix not red: %q", got)
	}
}

// TestColorizeUnifiedDiff_PreservesTrailingNewline guards the layout
// invariant — a diff that ends with "\n" still ends with "\n" after
// colorisation; one without doesn't gain one. Caller-supplied layout
// is sacred.
func TestColorizeUnifiedDiff_PreservesTrailingNewline(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	withNL := "+x\n"
	got := ColorizeUnifiedDiff(withNL, ColorAlways, os.Stdout)
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("trailing newline lost: %q", got)
	}
	withoutNL := "+x"
	got = ColorizeUnifiedDiff(withoutNL, ColorAlways, os.Stdout)
	if strings.HasSuffix(got, "\n") {
		t.Errorf("trailing newline gained: %q", got)
	}
}

// TestColorizeUnifiedDiff_EmptyInputIsEmpty is a tiny guard against
// the off-by-one bug where the loop terminator condition is wrong
// for "".
func TestColorizeUnifiedDiff_EmptyInputIsEmpty(t *testing.T) {
	if got := ColorizeUnifiedDiff("", ColorAlways, os.Stdout); got != "" {
		t.Errorf("empty input must produce empty output; got %q", got)
	}
}
