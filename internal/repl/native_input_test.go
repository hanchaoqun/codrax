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
