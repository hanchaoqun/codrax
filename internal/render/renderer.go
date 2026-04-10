package render

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/types"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Renderer consumes pipeline events and produces styled terminal output.
// Uses pterm.Area for in-place updating of task list and status during
// pipeline execution, and Glamour for final markdown rendering.
type Renderer struct {
	glamour *glamour.TermRenderer

	mu        sync.Mutex
	area      *pterm.AreaPrinter
	startTime time.Time
	stage      string // current pipeline stage
	detail     string // tool/sub-agent name for status line
	detailDone bool   // true if the detail refers to a completed call
	tasks     []taskEntry // tracked tasks
	animFrame int
	animStop  chan struct{}
}

type taskEntry struct {
	id     string
	title  string
	status types.TaskStatus
}

// New creates a Renderer.
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

	// Word wrap at 56 chars so that with the "  │ " prefix (4 cells)
	// the total stays under 60 columns — safe for CJK text (2 cells
	// per char) on 80-wide terminals without terminal-level wrapping.
	gr, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(56),
	)

	return &Renderer{glamour: gr}
}

// StartSpinner begins the live status area.
func (r *Renderer) StartSpinner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.area != nil {
		return
	}
	r.tasks = nil
	r.stage = "thinking"
	r.detail = ""
	r.detailDone = false
	r.startTime = time.Now()
	r.animFrame = 0

	a, _ := pterm.DefaultArea.WithRemoveWhenDone(true).Start()
	r.area = a
	r.redraw()

	// Start animation ticker.
	r.animStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.animStop:
				return
			case <-ticker.C:
				r.mu.Lock()
				if r.area == nil {
					r.mu.Unlock()
					return
				}
				r.animFrame++
				r.redraw()
				r.mu.Unlock()
			}
		}
	}()
}

// StopSpinner stops the live area and prints the final task list.
func (r *Renderer) StopSpinner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.area == nil {
		return
	}

	// Stop animation.
	close(r.animStop)

	// Stop area (removes its content due to RemoveWhenDone).
	r.area.Stop()
	r.area = nil

	// Print final task list — green checkmarks, dimmed titles.
	if len(r.tasks) > 0 {
		fmt.Println()
		for _, t := range r.tasks {
			fmt.Printf("  %s %s\n",
				pterm.FgGreen.Sprint("✓"),
				pterm.FgDarkGray.Sprint(t.title))
		}
		fmt.Println()
	}

	r.tasks = nil
	r.stage = ""
	r.detail = ""
}

// Emitter returns an EventEmitter callback bound to this renderer.
func (r *Renderer) Emitter() EventEmitter {
	return func(ev Event) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.area == nil {
			return
		}

		switch ev.Kind {
		case EventAgentThinking:
			r.detail = "thinking"
			r.detailDone = false

		case EventStageStart:
			r.stage = string(ev.Stage)
			r.detail = ""
			r.detailDone = false

		case EventToolCallStart:
			r.detail = ev.ToolName
			r.detailDone = false

		case EventToolCallEnd:
			r.detail = ev.ToolName
			r.detailDone = true

		case EventTransition:
			r.stage = string(ev.ToStage)
			r.detail = ""
			r.detailDone = false

		case EventSubAgentStart:
			r.detail = ev.SubTaskTitle
			r.detailDone = false

		case EventSubAgentEnd:
			r.detail = ev.SubTaskTitle
			r.detailDone = true

		case EventTaskListUpdated:
			if ev.TaskList != nil && len(ev.TaskList.Tasks) > 0 {
				r.tasks = make([]taskEntry, len(ev.TaskList.Tasks))
				for i, t := range ev.TaskList.Tasks {
					r.tasks[i] = taskEntry{id: t.ID, title: t.Title, status: t.Status}
				}
			}

		case EventTaskStatusChanged:
			for i := range r.tasks {
				if r.tasks[i].id == ev.TaskID {
					r.tasks[i].status = ev.TaskStatus
					break
				}
			}
		}

		r.redraw()
	}
}

// redraw rebuilds the area content from current state. Must be called
// with r.mu held.
func (r *Renderer) redraw() {
	var b strings.Builder
	elapsed := time.Since(r.startTime).Truncate(time.Second)
	frame := spinnerFrames[r.animFrame%len(spinnerFrames)]

	// Task list.
	if len(r.tasks) > 0 {
		for _, t := range r.tasks {
			icon := statusIcon(t.status)
			color := statusColor(t.status)
			fmt.Fprintf(&b, "  %s %s\n", color.Sprint(icon), t.title)
		}
		b.WriteString("\n")
	}

	// Status line: ⠋ stage · detail · 12s
	// Each element has a distinct color for easy scanning.
	var statusParts []string
	statusParts = append(statusParts, pterm.FgWhite.Sprint(r.stage))
	if r.detail != "" {
		if r.detailDone {
			statusParts = append(statusParts,
				pterm.FgGreen.Sprint("✓")+" "+pterm.FgDarkGray.Sprint(r.detail))
		} else {
			statusParts = append(statusParts,
				pterm.FgCyan.Sprint("►")+" "+pterm.FgGray.Sprint(r.detail))
		}
	}
	status := strings.Join(statusParts, pterm.FgDarkGray.Sprint(" · "))
	fmt.Fprintf(&b, "  %s %s %s %s",
		pterm.FgCyan.Sprint(frame),
		status,
		pterm.FgDarkGray.Sprint("·"),
		pterm.FgWhite.Sprint(elapsed))

	r.area.Update(b.String())
}

// RenderResult formats a completed BusContext into styled terminal output.
func (r *Renderer) RenderResult(busCtx *types.BusContext) string {
	if busCtx == nil {
		return "(no result)\n"
	}
	var b strings.Builder

	if busCtx.TaskState.LastError != "" {
		b.WriteString(fmt.Sprintf("\n  error: %s\n", busCtx.TaskState.LastError))
	}

	if busCtx.Mutable != nil {
		tl := busCtx.Mutable.TaskList()
		for _, item := range tl.Tasks {
			if item.Result != "" {
				clean := stripAgentLabels(item.Result)
				if clean == "" {
					continue
				}
				rendered := r.renderMarkdown(clean)
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

// Patterns to strip agent-internal labels from user-facing output.
var reAgentLabels = regexp.MustCompile(`(?m)^(\*\*)?(Answer|Evidence|Caveat)(\*\*)?:\s*$`)
var reAgentLabelInline = regexp.MustCompile(`(?m)^(\*\*)?(Answer|Evidence|Caveat)(\*\*)?:\s*`)

// stripAgentLabels removes "Answer:", "Evidence:", "Caveat:" labels
// (plain or **bold** markdown) from agent output.
func stripAgentLabels(s string) string {
	// Remove standalone label lines first (e.g. "Evidence:\n").
	s = reAgentLabels.ReplaceAllString(s, "")
	// Remove inline label prefix (e.g. "Answer: some text").
	s = reAgentLabelInline.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func (r *Renderer) renderMarkdown(text string) string {
	if r.glamour != nil {
		rendered, err := r.glamour.Render(text)
		if err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return text
}

func statusIcon(s types.TaskStatus) string {
	switch s {
	case types.TaskDone:
		return "✓"
	case types.TaskInProgress:
		return "►"
	case types.TaskFailed:
		return "✗"
	case types.TaskBlocked:
		return "⊘"
	default:
		return "○"
	}
}

func statusColor(s types.TaskStatus) pterm.Style {
	switch s {
	case types.TaskDone:
		return *pterm.NewStyle(pterm.FgGreen)
	case types.TaskInProgress:
		return *pterm.NewStyle(pterm.FgCyan)
	case types.TaskFailed:
		return *pterm.NewStyle(pterm.FgRed)
	case types.TaskBlocked:
		return *pterm.NewStyle(pterm.FgYellow)
	default:
		return *pterm.NewStyle(pterm.FgDarkGray)
	}
}
