package render

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Renderer consumes pipeline events and produces styled terminal output.
// It is safe for concurrent use — the orchestrator and agents emit events
// from various goroutines (e.g. sub-agent workers).
type Renderer struct {
	mu      sync.Mutex
	out     io.Writer
	s       *style
	spinner *Spinner

	// Accumulated state for the current run.
	traceID      string
	toolCalls    []toolCallRecord
	subAgents    []subAgentRecord
	stageHistory []stageRecord
	taskSnapshot *types.TaskList

	startTime time.Time
}

type toolCallRecord struct {
	Name     string
	OK       bool
	Duration time.Duration
	Stage    types.PipelineStage
	Agent    types.AgentName
}

type subAgentRecord struct {
	Name       string
	ID         string
	TaskTitle  string
	TaskCount  int
	Error      string
	Duration   time.Duration
	startTime  time.Time
}

type stageRecord struct {
	Stage    types.PipelineStage
	Agent    types.AgentName
	Skill    string
	Duration time.Duration
	Tools    int
	Error    string
	start    time.Time
}

// New creates a Renderer that writes to the given writer. Color output
// is auto-detected from os.Stdout; pass forceColor=true to override.
func New(out io.Writer, forceColor bool) *Renderer {
	color := forceColor
	if !color {
		if f, ok := out.(*os.File); ok {
			color = isTTY(f)
		}
	}
	s := newStyle(color)
	return &Renderer{
		out:     out,
		s:       s,
		spinner: newSpinner(out, s),
	}
}

// Emitter returns an EventEmitter callback bound to this renderer.
// Pass this to the orchestrator and agents.
func (r *Renderer) Emitter() EventEmitter {
	return r.handleEvent
}

// handleEvent is the EventEmitter callback dispatched per event kind.
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
	case EventStageEnd:
		r.onStageEnd(ev)
	case EventAgentThinking:
		r.onAgentThinking(ev)
	case EventAgentResponse:
		r.onAgentResponse(ev)
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
	case EventTransition:
		r.onTransition(ev)
	case EventSkillBound:
		r.onSkillBound(ev)
	}
}

// --- Pipeline ---

func (r *Renderer) onPipelineStart(ev Event) {
	r.traceID = ev.TraceID
	r.startTime = ev.Timestamp
	r.toolCalls = nil
	r.subAgents = nil
	r.stageHistory = nil
	r.taskSnapshot = nil

	r.writeln("")
	r.writef("  %s %s\n",
		r.s.BoldCyan("▶ Pipeline"),
		r.s.Dim(ev.TraceID))
}

func (r *Renderer) onPipelineEnd(ev Event) {
	r.spinner.Stop()
	elapsed := ev.Timestamp.Sub(r.startTime)

	r.writeln("")
	if ev.Error != "" {
		r.writef("  %s %s %s\n",
			r.s.BoldRed("■ Pipeline Failed"),
			r.s.Dim(fmt.Sprintf("(%s)", elapsed.Truncate(time.Millisecond))),
			r.s.Red(ev.Error))
	} else {
		r.writef("  %s %s\n",
			r.s.BoldGreen("■ Pipeline Complete"),
			r.s.Dim(fmt.Sprintf("(%s)", elapsed.Truncate(time.Millisecond))))
	}
}

// --- Stages ---

func (r *Renderer) onStageStart(ev Event) {
	r.spinner.Stop()

	rec := stageRecord{
		Stage: ev.Stage,
		Agent: ev.Agent,
		Skill: ev.Skill,
		start: ev.Timestamp,
	}
	r.stageHistory = append(r.stageHistory, rec)

	r.writeln("")
	r.writef("  %s %s %s %s\n",
		r.s.BoldCyan("┌"),
		r.s.Stage(ev.Stage),
		r.s.ArrowRight(),
		r.s.Agent(ev.Agent))

	r.spinner.Start(fmt.Sprintf("%s: thinking...", ev.Agent))
}

func (r *Renderer) onStageEnd(ev Event) {
	r.spinner.Stop()

	if len(r.stageHistory) > 0 {
		rec := &r.stageHistory[len(r.stageHistory)-1]
		rec.Duration = ev.Timestamp.Sub(rec.start)
		rec.Error = ev.Error
	}

	// Count tools used in this stage.
	stageTools := 0
	for _, tc := range r.toolCalls {
		if tc.Stage == ev.Stage {
			stageTools++
		}
	}
	if len(r.stageHistory) > 0 {
		r.stageHistory[len(r.stageHistory)-1].Tools = stageTools
	}

	dur := time.Duration(0)
	if len(r.stageHistory) > 0 {
		dur = r.stageHistory[len(r.stageHistory)-1].Duration
	}

	if ev.Error != "" {
		r.writef("  %s %s %s %s\n",
			r.s.BoldRed("└"),
			r.s.Stage(ev.Stage),
			r.s.Dim(fmt.Sprintf("(%s, %d tools)", dur.Truncate(time.Millisecond), stageTools)),
			r.s.Red("error: "+truncMsg(ev.Error, 60)))
	} else {
		r.writef("  %s %s %s\n",
			r.s.BrightBlack("└"),
			r.s.Stage(ev.Stage),
			r.s.Dim(fmt.Sprintf("(%s, %d tools)", dur.Truncate(time.Millisecond), stageTools)))
	}
}

// --- Agent thinking / response ---

func (r *Renderer) onAgentThinking(ev Event) {
	r.spinner.Update(fmt.Sprintf("%s: thinking...", ev.Agent))
}

func (r *Renderer) onAgentResponse(_ Event) {
	// spinner continues; will be updated by next event.
}

// --- Tool calls ---

func (r *Renderer) onToolCallStart(ev Event) {
	r.spinner.Update(fmt.Sprintf("%s: %s", ev.Agent, ev.ToolName))
	r.spinner.SetDetail(ev.ToolCallID)
}

func (r *Renderer) onToolCallEnd(ev Event) {
	r.spinner.Stop()

	rec := toolCallRecord{
		Name:     ev.ToolName,
		OK:       ev.ToolOK,
		Duration: ev.ToolTime,
		Stage:    ev.Stage,
		Agent:    ev.Agent,
	}
	r.toolCalls = append(r.toolCalls, rec)

	icon := r.s.SuccessIcon()
	if !ev.ToolOK {
		icon = r.s.FailIcon()
	}
	r.writef("  %s %s %s %s\n",
		r.s.Bullet(),
		icon,
		r.s.Tool(ev.ToolName),
		r.s.Dim(fmt.Sprintf("(%s)", ev.ToolTime.Truncate(time.Millisecond))))

	// Resume agent thinking spinner.
	r.spinner.Start(fmt.Sprintf("%s: thinking...", ev.Agent))
	r.spinner.SetDetail("")
}

// --- Sub-agents ---

func (r *Renderer) onSubAgentStart(ev Event) {
	r.spinner.Stop()

	rec := subAgentRecord{
		Name:      ev.SubAgentName,
		ID:        ev.SubAgentID,
		TaskTitle: ev.SubTaskTitle,
		TaskCount: ev.SubTaskCount,
		startTime: ev.Timestamp,
	}
	r.subAgents = append(r.subAgents, rec)

	r.writef("  %s %s %s %s\n",
		r.s.Bullet(),
		r.s.BoldMagenta("⊕ sub-agent"),
		r.s.Magenta(ev.SubAgentName),
		r.s.Dim(fmt.Sprintf("(%d sub-tasks)", ev.SubTaskCount)))

	for i := 0; i < ev.SubTaskCount && i < 5; i++ {
		prefix := r.s.TreeMid()
		if i == ev.SubTaskCount-1 || i == 4 {
			prefix = r.s.TreeEnd()
		}
		if ev.SubTaskTitle != "" && i == 0 {
			r.writef("  %s   %s %s\n", r.s.Bullet(), prefix, r.s.Dim(ev.SubTaskTitle))
		}
	}

	r.spinner.Start(fmt.Sprintf("sub-agent %s: running %d tasks...", ev.SubAgentName, ev.SubTaskCount))
}

func (r *Renderer) onSubAgentEnd(ev Event) {
	r.spinner.Stop()

	// Update record with duration.
	for i := len(r.subAgents) - 1; i >= 0; i-- {
		if r.subAgents[i].ID == ev.SubAgentID {
			r.subAgents[i].Duration = ev.Timestamp.Sub(r.subAgents[i].startTime)
			r.subAgents[i].Error = ev.Error
			break
		}
	}

	if ev.Error != "" {
		r.writef("  %s %s %s %s\n",
			r.s.Bullet(),
			r.s.BoldRed("⊖ sub-agent"),
			r.s.Magenta(ev.SubAgentName),
			r.s.Red("error: "+truncMsg(ev.Error, 50)))
	} else {
		r.writef("  %s %s %s %s\n",
			r.s.Bullet(),
			r.s.BrightMagenta("⊕ sub-agent done"),
			r.s.Magenta(ev.SubAgentName),
			r.s.Dim(fmt.Sprintf("(%d tools, %d facts)",
				ev.ToolCallCount, ev.FactCount)))
	}
}

// --- Task list ---

func (r *Renderer) onTaskListUpdated(ev Event) {
	r.spinner.Stop()
	if ev.TaskList == nil {
		return
	}
	r.taskSnapshot = ev.TaskList

	r.writeln("")
	r.writef("  %s\n", r.s.Box("Tasks"))
	for _, item := range ev.TaskList.Tasks {
		icon := r.s.TaskIcon(item.Status)
		title := item.Title
		if item.HighRisk {
			title += r.s.Dim(" [high-risk]")
		}
		r.writef("    %s %s\n", icon, title)
	}
	r.writeln("")
	r.spinner.Start("working...")
}

func (r *Renderer) onTaskStatusChanged(ev Event) {
	r.spinner.Stop()

	icon := r.s.TaskIcon(ev.TaskStatus)
	r.writef("  %s %s %s %s\n",
		r.s.Bullet(),
		icon,
		ev.TaskTitle,
		r.s.Dim(fmt.Sprintf("[%s]", ev.TaskStatus)))

	r.spinner.Start("working...")
}

// --- Transition ---

func (r *Renderer) onTransition(ev Event) {
	r.spinner.Stop()

	r.writef("  %s %s %s %s\n",
		r.s.Dim("  "),
		r.s.Stage(ev.FromStage),
		r.s.ArrowRight(),
		r.s.Stage(ev.ToStage))
}

// --- Skill ---

func (r *Renderer) onSkillBound(ev Event) {
	r.spinner.Stop()

	r.writef("  %s skill: %s\n", r.s.Bullet(), r.s.BrightBlue(ev.Skill))
}

// --- Final result rendering ---

// RenderResult formats a completed BusContext into styled terminal output.
// This replaces the plain-text renderResult in main.go.
func (r *Renderer) RenderResult(busCtx *types.BusContext) string {
	if busCtx == nil {
		return "(no result)\n"
	}
	var b strings.Builder

	s := r.s

	// Header.
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n", s.Box("Pipeline Summary")))
	b.WriteString(fmt.Sprintf("    Trace:    %s\n", s.Dim(busCtx.TraceID)))
	b.WriteString(fmt.Sprintf("    Stage:    %s\n", s.Stage(busCtx.PipelineStage)))

	// Completion status.
	if busCtx.TaskState.IsTerminal {
		b.WriteString(fmt.Sprintf("    Status:   %s\n", s.BoldGreen("completed")))
	} else {
		b.WriteString(fmt.Sprintf("    Status:   %s\n", s.BoldYellow("incomplete")))
	}

	// Stage history.
	if len(r.stageHistory) > 0 {
		b.WriteString(fmt.Sprintf("    Stages:   "))
		for i, sr := range r.stageHistory {
			if i > 0 {
				b.WriteString(s.Dim(" → "))
			}
			b.WriteString(s.Stage(sr.Stage))
		}
		b.WriteString("\n")
	} else if len(busCtx.TaskState.Completed) > 0 {
		b.WriteString(fmt.Sprintf("    Stages:   %s\n", s.Dim(strings.Join(busCtx.TaskState.Completed, " → "))))
	}

	// Statistics.
	b.WriteString(fmt.Sprintf("    Facts:    %s\n", s.Cyan(fmt.Sprintf("%d", len(busCtx.RepoFacts)))))

	toolOK, toolFail := 0, 0
	for _, tc := range r.toolCalls {
		if tc.OK {
			toolOK++
		} else {
			toolFail++
		}
	}
	totalTools := len(busCtx.ToolResults)
	if totalTools == 0 {
		totalTools = toolOK + toolFail
	}
	toolStr := fmt.Sprintf("%d", totalTools)
	if toolFail > 0 {
		toolStr += s.Red(fmt.Sprintf(" (%d failed)", toolFail))
	}
	b.WriteString(fmt.Sprintf("    Tools:    %s\n", toolStr))

	if len(busCtx.MCPResponses) > 0 {
		b.WriteString(fmt.Sprintf("    MCP:      %s\n", s.Cyan(fmt.Sprintf("%d", len(busCtx.MCPResponses)))))
	}
	if len(r.subAgents) > 0 {
		b.WriteString(fmt.Sprintf("    SubAgents: %s\n", s.Magenta(fmt.Sprintf("%d", len(r.subAgents)))))
	}

	if busCtx.TaskState.LastError != "" {
		b.WriteString(fmt.Sprintf("    Error:    %s\n", s.Red(truncMsg(busCtx.TaskState.LastError, 80))))
	}

	// Task list.
	if busCtx.Mutable != nil {
		tl := busCtx.Mutable.TaskList()
		if len(tl.Tasks) > 0 {
			b.WriteString("\n")
			b.WriteString(fmt.Sprintf("  %s\n", s.Box("Tasks")))
			for _, item := range tl.Tasks {
				icon := s.TaskIcon(item.Status)
				title := item.Title
				if item.HighRisk {
					title += s.Dim(" [high-risk]")
				}
				complexity := ""
				if item.Complexity != "" && item.Complexity != "simple" {
					complexity = s.Dim(fmt.Sprintf(" (%s)", item.Complexity))
				}
				b.WriteString(fmt.Sprintf("    %s %s%s\n", icon, title, complexity))
			}

			// Task results.
			b.WriteString("\n")
			b.WriteString(fmt.Sprintf("  %s\n", s.Box("Results")))
			for _, item := range tl.Tasks {
				icon := s.TaskIcon(item.Status)
				b.WriteString(fmt.Sprintf("\n  %s %s\n", icon, s.Bold(item.Title)))
				if item.Result != "" {
					rendered := s.renderMarkdown(strings.TrimRight(item.Result, "\n"))
					for _, line := range strings.Split(rendered, "\n") {
						b.WriteString(fmt.Sprintf("    %s\n", line))
					}
				} else {
					b.WriteString(fmt.Sprintf("    %s\n", s.Dim("(no result)")))
				}
			}
		}
	}

	// Tool call details (collapsed summary).
	if len(r.toolCalls) > 0 {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s\n", s.Box("Tool Calls")))
		// Group by stage.
		stageTools := make(map[types.PipelineStage][]toolCallRecord)
		var stageOrder []types.PipelineStage
		for _, tc := range r.toolCalls {
			if _, seen := stageTools[tc.Stage]; !seen {
				stageOrder = append(stageOrder, tc.Stage)
			}
			stageTools[tc.Stage] = append(stageTools[tc.Stage], tc)
		}
		for _, stage := range stageOrder {
			tcs := stageTools[stage]
			b.WriteString(fmt.Sprintf("    %s (%d calls)\n", s.Stage(stage), len(tcs)))
			// Aggregate by tool name.
			nameCount := make(map[string]int)
			nameOK := make(map[string]int)
			var nameOrder []string
			for _, tc := range tcs {
				if nameCount[tc.Name] == 0 {
					nameOrder = append(nameOrder, tc.Name)
				}
				nameCount[tc.Name]++
				if tc.OK {
					nameOK[tc.Name]++
				}
			}
			for _, name := range nameOrder {
				cnt := nameCount[name]
				ok := nameOK[name]
				icon := s.SuccessIcon()
				if ok < cnt {
					icon = s.FailIcon()
				}
				b.WriteString(fmt.Sprintf("      %s %s ×%d\n", icon, s.Tool(name), cnt))
			}
		}
	}

	// Sub-agent details.
	if len(r.subAgents) > 0 {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s\n", s.Box("Sub-Agents")))
		for _, sa := range r.subAgents {
			icon := s.SuccessIcon()
			if sa.Error != "" {
				icon = s.FailIcon()
			}
			b.WriteString(fmt.Sprintf("    %s %s %s\n",
				icon,
				s.Magenta(sa.Name),
				s.Dim(fmt.Sprintf("(%d tasks, %s)",
					sa.TaskCount, sa.Duration.Truncate(time.Millisecond)))))
		}
	}

	b.WriteString("\n")
	return b.String()
}

// --- helpers ---

func (r *Renderer) writef(format string, args ...interface{}) {
	r.spinner.writeMu.Lock()
	fmt.Fprintf(r.out, format, args...)
	r.spinner.writeMu.Unlock()
}

func (r *Renderer) writeln(s string) {
	r.spinner.writeMu.Lock()
	fmt.Fprintln(r.out, s)
	r.spinner.writeMu.Unlock()
}

func truncMsg(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
