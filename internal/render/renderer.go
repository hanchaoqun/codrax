package render

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Spinner frames for in-progress animation.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Renderer consumes pipeline events and produces styled terminal output.
// It is safe for concurrent use — the orchestrator and agents emit events
// from various goroutines (e.g. sub-agent workers).
type Renderer struct {
	mu      sync.Mutex
	glamour *glamour.TermRenderer

	startTime time.Time

	// Task list state (becomes persistent at pipeline end).
	tasks     []types.TaskItem
	treeLines int

	// Pipeline status state (ephemeral).
	currentStage types.PipelineStage
	activeTools  map[string]string // toolCallID → toolName
	statusLines  int

	// Animation goroutine.
	animFrame   int
	animStop    chan struct{}
	animRunning bool
}

// New creates a Renderer that writes styled output. Color output is
// auto-detected from os.Stdout; pass forceColor=true to override.
func New(_ /* out */ interface{}, forceColor bool) *Renderer {
	color := forceColor
	if !color {
		info, err := os.Stdout.Stat()
		if err == nil {
			color = (info.Mode() & os.ModeCharDevice) != 0
		}
	}
	if !color {
		pterm.DisableColor()
	}

	gr, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)

	return &Renderer{
		glamour:     gr,
		activeTools: make(map[string]string),
	}
}

// Emitter returns an EventEmitter callback bound to this renderer.
func (r *Renderer) Emitter() EventEmitter {
	return r.handleEvent
}

// PromptInput returns the styled "❯" input prompt for the REPL.
func (r *Renderer) PromptInput() string {
	return pterm.NewStyle(pterm.Bold, pterm.FgLightCyan).Sprint("❯")
}

// PromptContinue returns the styled "…" continuation prompt for
// multi-line input.
func (r *Renderer) PromptContinue() string {
	return pterm.NewStyle(pterm.FgDarkGray).Sprint("…")
}

// InputBorderTop returns the top border for the input box.
func (r *Renderer) InputBorderTop() string {
	return pterm.FgDarkGray.Sprint("╭" + strings.Repeat("─", 58) + "╮")
}

// InputBorderBottom returns the bottom border for the input box.
func (r *Renderer) InputBorderBottom() string {
	return pterm.FgDarkGray.Sprint("╰" + strings.Repeat("─", 58) + "╯")
}

// Banner returns the styled welcome box for the REPL.
func (r *Renderer) Banner() string {
	hints := pterm.FgDarkGray.Sprint("/help for commands · /exit to quit · \\ for multi-line")
	return pterm.DefaultBox.
		WithBoxStyle(pterm.NewStyle(pterm.FgDarkGray)).
		WithLeftPadding(2).
		WithRightPadding(2).
		Sprint(hints)
}

// ---------- event dispatch ----------

func (r *Renderer) handleEvent(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch ev.Kind {
	case EventPipelineStart:
		r.onPipelineStart(ev)
	case EventPipelineEnd:
		r.onPipelineEnd(ev)
	case EventStageStart:
		r.onStageStart(ev)
	case EventAgentThinking:
		r.onAgentThinking(ev)
	case EventToolCallStart:
		r.onToolCallStart(ev)
	case EventToolCallEnd:
		r.onToolCallEnd(ev)
	case EventSubAgentStart:
		r.onSubAgentStart(ev)
	case EventSubAgentEnd:
		r.onSubAgentEnd(ev)
	case EventTaskListUpdated:
		r.onTaskListUpdated(ev)
	case EventTaskStatusChanged:
		r.onTaskStatusChanged(ev)
	}
}

// ---------- pipeline lifecycle ----------

func (r *Renderer) onPipelineStart(ev Event) {
	r.startTime = ev.Timestamp
	r.tasks = nil
	r.treeLines = 0
	r.statusLines = 0
	r.currentStage = ""
	r.activeTools = make(map[string]string)
	r.animFrame = 0
	r.redraw()
	r.startAnimation()
}

func (r *Renderer) onPipelineEnd(ev Event) {
	r.stopAnimation()

	// Clear entire dynamic area.
	up := r.treeLines + r.statusLines
	if up > 0 {
		fmt.Printf("\033[%dA", up)
	}
	fmt.Print("\033[J")

	// Reprint tree with final statuses — permanent.
	r.treeLines = 0
	r.statusLines = 0
	if len(r.tasks) > 0 {
		r.printTree()
	}

	// Release tracking.
	r.treeLines = 0
	r.statusLines = 0
	r.tasks = nil
	r.currentStage = ""
	r.activeTools = make(map[string]string)
}

// ---------- stage ----------

func (r *Renderer) onStageStart(ev Event) {
	r.currentStage = ev.Stage
	r.activeTools = make(map[string]string)
	r.redraw()
}

// ---------- agent / tool ----------

func (r *Renderer) onAgentThinking(ev Event) {
	// No-op: animation goroutine handles visual updates.
}

func (r *Renderer) onToolCallStart(ev Event) {
	r.activeTools[ev.ToolCallID] = ev.ToolName
	r.redraw()
}

func (r *Renderer) onToolCallEnd(ev Event) {
	delete(r.activeTools, ev.ToolCallID)
	r.redraw()
}

// ---------- sub-agents ----------

func (r *Renderer) onSubAgentStart(ev Event) {
	r.redraw()
}

func (r *Renderer) onSubAgentEnd(ev Event) {
	r.redraw()
}

// ---------- task tree ----------

func (r *Renderer) onTaskListUpdated(ev Event) {
	if ev.TaskList == nil {
		return
	}
	r.tasks = make([]types.TaskItem, len(ev.TaskList.Tasks))
	copy(r.tasks, ev.TaskList.Tasks)
	r.redraw()
}

func (r *Renderer) onTaskStatusChanged(ev Event) {
	for i := range r.tasks {
		if r.tasks[i].ID == ev.TaskID {
			r.tasks[i].Status = ev.TaskStatus
			break
		}
	}
	r.redraw()
}

// ---------- drawing ----------

// redraw clears the dynamic area and reprints task list + status line.
// Must be called with r.mu held.
func (r *Renderer) redraw() {
	// Erase dynamic area.
	up := r.treeLines + r.statusLines
	if up > 0 {
		fmt.Printf("\033[%dA", up)
	}
	fmt.Print("\033[J")
	r.treeLines = 0
	r.statusLines = 0

	// Print task tree.
	r.printTree()

	// Print pipeline status line.
	r.printStatusLine()
}

// printTree renders the task list with status icons. Sets r.treeLines.
func (r *Renderer) printTree() {
	if len(r.tasks) == 0 {
		return
	}

	fmt.Println() // blank before
	n := 1
	for _, item := range r.tasks {
		icon := r.taskIcon(item.Status)
		fmt.Printf("  %s %s\n", icon, item.Title)
		n++
	}
	fmt.Println() // blank after
	n++
	r.treeLines = n
}

// printStatusLine renders the pipeline status: stage(tool1, tool2)...
// Sets r.statusLines.
func (r *Renderer) printStatusLine() {
	line := r.buildPipelineStatus()
	if line != "" {
		fmt.Println(line)
		r.statusLines = 1
	}
}

// buildPipelineStatus formats: ⠋ stage(tool1, tool2)...
// When no tasks exist, the spinner prefix is always shown.
// When tasks exist, the spinner prefix is omitted (task lines carry the animation).
func (r *Renderer) buildPipelineStatus() string {
	stage := string(r.currentStage)
	if stage == "" {
		stage = "thinking"
	}

	// Collect unique active tool names.
	var tools []string
	seen := map[string]bool{}
	for _, name := range r.activeTools {
		if !seen[name] {
			seen[name] = true
			tools = append(tools, name)
		}
	}

	var content string
	if len(tools) > 0 {
		content = fmt.Sprintf("%s(%s)", stage, strings.Join(tools, ", "))
	} else {
		content = stage
	}

	if len(r.tasks) == 0 {
		// No tasks yet — show animated spinner prefix.
		frame := spinnerFrames[r.animFrame%len(spinnerFrames)]
		return pterm.FgDarkGray.Sprintf("  %s %s...",
			pterm.FgCyan.Sprint(frame), content)
	}

	// Tasks visible — static status line, no spinner prefix.
	return pterm.FgDarkGray.Sprintf("  %s...", content)
}

// taskIcon returns a colored icon for task status. In-progress tasks
// use the current animation frame.
func (r *Renderer) taskIcon(status types.TaskStatus) string {
	switch status {
	case types.TaskDone:
		return pterm.FgGreen.Sprint("✓")
	case types.TaskInProgress:
		frame := spinnerFrames[r.animFrame%len(spinnerFrames)]
		return pterm.FgCyan.Sprint(frame)
	case types.TaskFailed:
		return pterm.FgRed.Sprint("✗")
	case types.TaskBlocked:
		return pterm.FgYellow.Sprint("⊘")
	default: // pending
		return pterm.FgDarkGray.Sprint("−")
	}
}

// ---------- animation goroutine ----------

func (r *Renderer) startAnimation() {
	// Must be called with r.mu held.
	if r.animRunning {
		return
	}
	r.animStop = make(chan struct{})
	r.animRunning = true

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.animStop:
				return
			case <-ticker.C:
				r.mu.Lock()
				if !r.animRunning {
					r.mu.Unlock()
					return
				}
				r.animFrame = (r.animFrame + 1) % len(spinnerFrames)
				r.redraw()
				r.mu.Unlock()
			}
		}
	}()
}

func (r *Renderer) stopAnimation() {
	// Must be called with r.mu held.
	if !r.animRunning {
		return
	}
	r.animRunning = false
	close(r.animStop)
}

// ---------- final result rendering ----------

// RenderResult formats a completed BusContext into styled terminal output.
// The task tree is already visible on screen, so this only renders the
// markdown answer text.
func (r *Renderer) RenderResult(busCtx *types.BusContext) string {
	if busCtx == nil {
		return "(no result)\n"
	}
	var b strings.Builder

	if busCtx.TaskState.LastError != "" {
		b.WriteString(fmt.Sprintf("\n  %s %s\n",
			pterm.FgRed.Sprint("error:"), busCtx.TaskState.LastError))
	}

	if busCtx.Mutable != nil {
		tl := busCtx.Mutable.TaskList()
		for _, item := range tl.Tasks {
			if item.Result != "" {
				rendered := r.renderMarkdown(item.Result)
				b.WriteString(rendered)
				b.WriteString("\n")
			}
		}
	}

	if b.Len() == 0 {
		b.WriteString("(no result)\n")
	}
	return b.String()
}

// ---------- helpers ----------

func (r *Renderer) renderMarkdown(text string) string {
	if r.glamour != nil {
		rendered, err := r.glamour.Render(text)
		if err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return text
}
