package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ANSI escape sequences. When stdout is not a terminal (piped to a file
// or another process) the renderer disables colors automatically.
const (
	esc   = "\033["
	reset = esc + "0m"

	bold = esc + "1m"
	dim  = esc + "2m"

	fgBlack   = esc + "30m"
	fgRed     = esc + "31m"
	fgGreen   = esc + "32m"
	fgYellow  = esc + "33m"
	fgBlue    = esc + "34m"
	fgMagenta = esc + "35m"
	fgCyan    = esc + "36m"
	fgWhite   = esc + "37m"

	fgBrightBlack   = esc + "90m"
	fgBrightRed     = esc + "91m"
	fgBrightGreen   = esc + "92m"
	fgBrightYellow  = esc + "93m"
	fgBrightBlue    = esc + "94m"
	fgBrightMagenta = esc + "95m"
	fgBrightCyan    = esc + "96m"
)

// isTTY reports whether the given file is a terminal.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// style wraps text in ANSI codes. When color is disabled, returns text as-is.
type style struct {
	enabled bool
}

func newStyle(color bool) *style {
	return &style{enabled: color}
}

func (s *style) apply(codes string, text string) string {
	if !s.enabled || codes == "" {
		return text
	}
	return codes + text + reset
}

func (s *style) Bold(text string) string          { return s.apply(bold, text) }
func (s *style) Dim(text string) string           { return s.apply(dim, text) }
func (s *style) Red(text string) string            { return s.apply(fgRed, text) }
func (s *style) Green(text string) string          { return s.apply(fgGreen, text) }
func (s *style) Yellow(text string) string         { return s.apply(fgYellow, text) }
func (s *style) Blue(text string) string           { return s.apply(fgBlue, text) }
func (s *style) Magenta(text string) string        { return s.apply(fgMagenta, text) }
func (s *style) Cyan(text string) string           { return s.apply(fgCyan, text) }
func (s *style) BrightBlack(text string) string    { return s.apply(fgBrightBlack, text) }
func (s *style) BrightGreen(text string) string    { return s.apply(fgBrightGreen, text) }
func (s *style) BrightYellow(text string) string   { return s.apply(fgBrightYellow, text) }
func (s *style) BrightBlue(text string) string     { return s.apply(fgBrightBlue, text) }
func (s *style) BrightCyan(text string) string     { return s.apply(fgBrightCyan, text) }
func (s *style) BrightMagenta(text string) string  { return s.apply(fgBrightMagenta, text) }

func (s *style) BoldCyan(text string) string    { return s.apply(bold+fgCyan, text) }
func (s *style) BoldGreen(text string) string   { return s.apply(bold+fgGreen, text) }
func (s *style) BoldRed(text string) string     { return s.apply(bold+fgRed, text) }
func (s *style) BoldYellow(text string) string  { return s.apply(bold+fgYellow, text) }
func (s *style) BoldBlue(text string) string    { return s.apply(bold+fgBlue, text) }
func (s *style) BoldMagenta(text string) string { return s.apply(bold+fgMagenta, text) }

// stageColor returns a consistent color for each pipeline stage.
func (s *style) stageColor(stage types.PipelineStage) string {
	switch stage {
	case types.StageAnalyze:
		return fgCyan
	case types.StageExplore:
		return fgBlue
	case types.StagePlan:
		return fgMagenta
	case types.StageDesignReview:
		return fgYellow
	case types.StageImplement:
		return fgGreen
	case types.StageCodeReview:
		return fgYellow
	case types.StageVerify:
		return fgBrightCyan
	case types.StageFinalize:
		return fgBrightGreen
	default:
		return fgWhite
	}
}

func (s *style) Stage(stage types.PipelineStage) string {
	return s.apply(bold+s.stageColor(stage), string(stage))
}

func (s *style) Agent(agent types.AgentName) string {
	return s.apply(bold+fgBrightCyan, string(agent))
}

func (s *style) Tool(name string) string {
	return s.apply(fgYellow, name)
}

func (s *style) TaskIcon(status types.TaskStatus) string {
	switch status {
	case types.TaskDone:
		return s.apply(fgGreen, "✓")
	case types.TaskInProgress:
		return s.apply(fgCyan, "●")
	case types.TaskFailed:
		return s.apply(fgRed, "✗")
	case types.TaskBlocked:
		return s.apply(fgYellow, "⊘")
	default:
		return s.apply(fgBrightBlack, "○")
	}
}

func (s *style) SuccessIcon() string { return s.apply(fgGreen, "✓") }
func (s *style) FailIcon() string    { return s.apply(fgRed, "✗") }
func (s *style) ArrowRight() string  { return s.apply(fgBrightBlack, "→") }
func (s *style) Bullet() string      { return s.apply(fgBrightBlack, "│") }
func (s *style) TreeEnd() string     { return s.apply(fgBrightBlack, "└") }
func (s *style) TreeMid() string     { return s.apply(fgBrightBlack, "├") }

// Box draws a bordered section header.
func (s *style) Box(title string) string {
	line := strings.Repeat("─", 50)
	return fmt.Sprintf("%s %s %s",
		s.apply(fgBrightBlack, "─"),
		s.apply(bold, title),
		s.apply(fgBrightBlack, line[:max(0, 48-len(title))]))
}

// Separator draws a dim horizontal rule.
func (s *style) Separator() string {
	return s.apply(fgBrightBlack, strings.Repeat("─", 50))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
