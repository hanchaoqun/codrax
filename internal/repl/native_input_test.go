package repl

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestNativeClearRenderedUsesCursorRow(t *testing.T) {
	var out strings.Builder
	e := &nativeLineInput{
		out:           &out,
		renderedFrame: nativeRenderedFrame{rows: 4, cursorRow: 0},
	}
	e.clearRendered()
	got := out.String()
	if strings.HasPrefix(got, "\r\x1b[3A") {
		t.Fatalf("clear must not move above the frame when cursor is already on the top row: %q", got)
	}
	if n := strings.Count(got, ansiEraseEntireLine); n != 4 {
		t.Fatalf("cleared rows=%d, want 4; output=%q", n, got)
	}

	out.Reset()
	e.renderedFrame = nativeRenderedFrame{rows: 4, cursorRow: 2}
	e.clearRendered()
	got = out.String()
	if !strings.HasPrefix(got, "\r\x1b[2A"+ansiEraseEntireLine) {
		t.Fatalf("clear must move up from the actual cursor row before erasing: %q", got)
	}
	if n := strings.Count(got, ansiEraseEntireLine); n != 4 {
		t.Fatalf("cleared rows=%d, want 4; output=%q", n, got)
	}
}

func TestNativeRenderSlashSuggestionShrinkClearsPreviousFrame(t *testing.T) {
	var out strings.Builder
	e := &nativeLineInput{
		out:       &out,
		fd:        -1,
		prompt:    "❯❯ ",
		termWidth: 80,
		lang:      "zh",
		value:     []rune("/"),
		cursor:    1,
	}
	e.refreshSuggest()
	e.render()
	previous := e.renderedFrame
	if previous.rows <= 1 {
		t.Fatalf("slash suggestion frame should have multiple rows, got %+v", previous)
	}
	if previous.cursorRow != 0 {
		t.Fatalf("input cursor should be on the prompt row, got %+v", previous)
	}

	out.Reset()
	e.value = nil
	e.cursor = 0
	e.refreshSuggest()
	e.render()
	got := out.String()
	if strings.HasPrefix(got, "\r\x1b[") && !strings.HasPrefix(got, "\r"+ansiEraseEntireLine) {
		t.Fatalf("shrinking the frame must clear from the prompt row, not rows above it: %q", got)
	}
	if n := strings.Count(got, ansiEraseEntireLine); n != previous.rows {
		t.Fatalf("shrink clear erased %d rows, want previous frame rows %d; output=%q", n, previous.rows, got)
	}
	if e.renderedFrame.rows != 1 {
		t.Fatalf("empty prompt should render as one transient row, got %+v", e.renderedFrame)
	}
}

func TestNativeFrameForLinesUsesDisplayWidth(t *testing.T) {
	styledWide := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Render("中文abc")
	if rows := nativeTerminalRows(styledWide, 5); rows != 2 {
		t.Fatalf("ANSI-styled CJK width should occupy 2 rows at width 5, got %d", rows)
	}

	frame := nativeFrameForLines([]string{"123456", styledWide}, 5, 4)
	if frame.rows != 4 {
		t.Fatalf("physical frame rows=%d, want 4; frame=%+v", frame.rows, frame)
	}
	if frame.cursorRow != 1 || frame.cursorCol != 1 {
		t.Fatalf("cursor position should be row=1 col=1, got %+v", frame)
	}
}

func TestNativeSubmitClearsTransientFrameBeforeEcho(t *testing.T) {
	var out strings.Builder
	e := &nativeLineInput{
		out:       &out,
		fd:        -1,
		prompt:    "❯❯ ",
		termWidth: 80,
		lang:      "zh",
		value:     []rune("/"),
		cursor:    1,
	}
	e.refreshSuggest()
	e.render()
	previous := e.renderedFrame

	out.Reset()
	_, done := e.submit()
	if !done {
		t.Fatal("slash input should submit as printable content")
	}
	got := out.String()
	if n := strings.Count(got, ansiEraseEntireLine); n != previous.rows {
		t.Fatalf("submit clear erased %d rows, want previous frame rows %d; output=%q", n, previous.rows, got)
	}
	if !strings.Contains(got, "────────────────") {
		t.Fatalf("submit should print the persistent echo after clearing the transient frame: %q", got)
	}
}

func TestNativeSubmit_RejectsUnresolvedPastePlaceholder(t *testing.T) {
	e := &nativeLineInput{
		value: []rune("[Pasted text #0 +1 lines +802 chars]"),
	}

	res, done := e.submit()
	if done {
		t.Fatalf("native submit should reject unresolved placeholder, got %+v", res)
	}
	if strings.TrimSpace(string(e.value)) != "" {
		t.Fatalf("native input should clear unresolved placeholder, got %q", string(e.value))
	}
}

func TestNativeTryAppendRunesInlineAvoidsRedrawForPlainAppend(t *testing.T) {
	var out strings.Builder
	e := &nativeLineInput{
		out:           &out,
		prompt:        "❯❯ ",
		termWidth:     80,
		lang:          "zh",
		renderedFrame: nativeRenderedFrame{rows: 1},
	}

	if !e.tryAppendRunesInline([]rune("你")) {
		t.Fatal("plain append should be written inline")
	}
	if got := out.String(); got != "你" {
		t.Fatalf("inline append output=%q, want only appended text", got)
	}
	if strings.Contains(out.String(), "\r") || strings.Contains(out.String(), ansiEraseEntireLine) {
		t.Fatalf("inline append must not redraw the prompt/frame: %q", out.String())
	}
	if got := string(e.value); got != "你" {
		t.Fatalf("value=%q, want 你", got)
	}
	if e.renderedFrame.rows != 1 || e.renderedFrame.cursorCol <= inputPromptDisplayWidth(e.prompt) {
		t.Fatalf("inline append should advance the tracked real cursor, frame=%+v", e.renderedFrame)
	}
}

func TestNativeTryAppendRunesInlineKeepsSlashSuggestionsOnRenderPath(t *testing.T) {
	var out strings.Builder
	e := &nativeLineInput{
		out:           &out,
		prompt:        "❯❯ ",
		termWidth:     80,
		lang:          "zh",
		renderedFrame: nativeRenderedFrame{rows: 1},
	}

	if e.tryAppendRunesInline([]rune("/")) {
		t.Fatal("slash input should use the render path so suggestions can appear")
	}
	if out.String() != "" {
		t.Fatalf("slash render path should not write inline output, got %q", out.String())
	}
	if got := string(e.value); got != "" {
		t.Fatalf("slash render path should not mutate value inline, got %q", got)
	}
}

func TestNativeTryAppendRunesInlineFallsBackNearRightEdge(t *testing.T) {
	var out strings.Builder
	e := &nativeLineInput{
		out:           &out,
		prompt:        "❯❯ ",
		termWidth:     inputPromptDisplayWidth("❯❯ ") + 9,
		value:         []rune("abcdefgh"),
		cursor:        8,
		renderedFrame: nativeRenderedFrame{rows: 1},
	}

	if e.tryAppendRunesInline([]rune("i")) {
		t.Fatal("append that would overflow the visible input window should use full render")
	}
	if out.String() != "" {
		t.Fatalf("overflow fallback should not write inline output, got %q", out.String())
	}
}

func TestNativeSlashSuggestShowsHitraceConvertSubcommand(t *testing.T) {
	e := &nativeLineInput{
		value: []rune("/htrace "),
		lang:  "zh",
	}
	matches := e.filterSuggestions()
	if len(matches) == 0 {
		t.Fatal("expected /htrace subcommand suggestions")
	}
	var found bool
	var foundToolsStatus bool
	for _, suggestion := range matches {
		if suggestion.display == "/htrace convert [opts] <binary> [out.systrace]" {
			found = true
			if suggestion.insert != "/htrace convert" {
				t.Fatalf("convert insert=%q, want /htrace convert", suggestion.insert)
			}
			if !suggestion.needsArg || !strings.Contains(suggestion.help, "二进制") {
				t.Fatalf("convert suggestion should carry arg hint + zh help: %+v", suggestion)
			}
		}
		if suggestion.display == "/htrace tools-status" {
			foundToolsStatus = true
			if suggestion.insert != "/htrace tools-status" {
				t.Fatalf("tools-status insert=%q, want /htrace tools-status", suggestion.insert)
			}
			if suggestion.needsArg || !strings.Contains(suggestion.help, "trace_streamer") {
				t.Fatalf("tools-status suggestion should not need args and should explain status: %+v", suggestion)
			}
		}
	}
	if !found {
		t.Fatalf("convert subcommand not suggested: %+v", matches)
	}
	if !foundToolsStatus {
		t.Fatalf("tools-status subcommand not suggested: %+v", matches)
	}
}

func TestNativeSlashSuggestAtraceReusesHtraceSubcommands(t *testing.T) {
	e := &nativeLineInput{
		value: []rune("/atrace c"),
		lang:  "en",
	}
	matches := e.filterSuggestions()
	if len(matches) != 2 {
		t.Fatalf("expected /atrace c to suggest clear and convert, got %+v", matches)
	}
	var displays []string
	for _, suggestion := range matches {
		displays = append(displays, suggestion.display)
	}
	got := strings.Join(displays, ",")
	if !strings.Contains(got, "/atrace convert [opts] <binary> [out.systrace]") ||
		!strings.Contains(got, "/atrace clear") {
		t.Fatalf("unexpected /atrace c suggestions: %s", got)
	}
}

func TestNativeAcceptSubcommandWithArgsDoesNotSubmitOnEnter(t *testing.T) {
	e := &nativeLineInput{
		value:       []rune("/htrace "),
		cursor:      len([]rune("/htrace ")),
		lang:        "zh",
		showSuggest: true,
		slashSel:    0,
	}
	consumed := e.acceptSuggestion(true)
	if !consumed {
		t.Fatal("enter on an argument-taking subcommand should keep editing instead of submitting")
	}
	if got := string(e.value); got != "/htrace append " {
		t.Fatalf("accepted value=%q, want /htrace append ", got)
	}
}
