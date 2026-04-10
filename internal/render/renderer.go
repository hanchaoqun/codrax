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

// Renderer consumes pipeline events and produces styled terminal output.
// It is safe for concurrent use — the orchestrator and agents emit events
// from various goroutines (e.g. sub-agent workers).
type Renderer struct {
	mu      sync.Mutex
	spinner *pterm.SpinnerPrinter
	glamour *glamour.TermRenderer

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
	Name      string
	ID        string
	TaskTitle string
	TaskCount int
	Error     string
	Duration  time.Duration
	startTime time.Time
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

// New creates a Renderer that writes styled output. Color output is
// auto-detected from os.Stdout; pass forceColor=true to override.
func New(_ /* out */ interface{}, forceColor bool) *Renderer {
	// Detect color support.
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

	// Pre-create Glamour renderer.
	gr, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)

	return &Renderer{
		glamour: gr,
	}
}

// Emitter returns an EventEmitter callback bound to this renderer.
// Pass this to the orchestrator and agents.
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

// Banner returns the styled welcome box for the REPL.
func (r *Renderer) Banner() string {
	title := pterm.NewStyle(pterm.Bold, pterm.FgLightCyan).Sprint("✦ CODRAX")
	hints := pterm.FgDarkGray.Sprint("/help for commands · /exit to quit · \\ for multi-line")

	content := title + "\n" + hints

	return pterm.DefaultBox.
		WithBoxStyle(pterm.NewStyle(pterm.FgDarkGray)).
		WithLeftPadding(2).
		WithRightPadding(2).
		Sprint(content)
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
		// spinner continues
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

	// Header
	fmt.Println()
	pterm.DefaultHeader.
		WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).
		WithTextStyle(pterm.NewStyle(pterm.Bold, pterm.FgLightCyan)).
		WithFullWidth().
		Println("CODRAX")

	pterm.FgDarkGray.Printf("  trace: %s\n\n", ev.TraceID)
}

func (r *Renderer) onPipelineEnd(ev Event) {
	r.stopSpinner()
	elapsed := ev.Timestamp.Sub(r.startTime)

	fmt.Println()
	if ev.Error != "" {
		pterm.Error.Printf("Pipeline Failed (%s) %s\n",
			elapsed.Truncate(time.Millisecond), ev.Error)
	} else {
		pterm.Success.Printf("Pipeline Complete (%s)\n",
			elapsed.Truncate(time.Millisecond))
	}
}

// --- Stages ---

func (r *Renderer) onStageStart(ev Event) {
	r.stopSpinner()

	rec := stageRecord{
		Stage: ev.Stage,
		Agent: ev.Agent,
		Skill: ev.Skill,
		start: ev.Timestamp,
	}
	r.stageHistory = append(r.stageHistory, rec)

	fmt.Println()
	stageStr := stageStyle(ev.Stage).Sprint(string(ev.Stage))
	agentStr := pterm.NewStyle(pterm.Bold, pterm.FgLightCyan).Sprint(string(ev.Agent))
	pterm.FgDarkGray.Print("  ┌ ")
	fmt.Printf("%s %s %s\n", stageStr, pterm.FgDarkGray.Sprint("→"), agentStr)

	r.startSpinner(fmt.Sprintf("%s: thinking...", ev.Agent))
}

func (r *Renderer) onStageEnd(ev Event) {
	r.stopSpinner()

	if len(r.stageHistory) > 0 {
		rec := &r.stageHistory[len(r.stageHistory)-1]
		rec.Duration = ev.Timestamp.Sub(rec.start)
		rec.Error = ev.Error
	}

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

	stageStr := stageStyle(ev.Stage).Sprint(string(ev.Stage))
	durStr := pterm.FgDarkGray.Sprintf("(%s, %d tools)", dur.Truncate(time.Millisecond), stageTools)
	if ev.Error != "" {
		pterm.FgDarkGray.Print("  └ ")
		fmt.Printf("%s %s ", stageStr, durStr)
		pterm.FgRed.Printf("error: %s\n", truncMsg(ev.Error, 60))
	} else {
		pterm.FgDarkGray.Print("  └ ")
		fmt.Printf("%s %s\n", stageStr, durStr)
	}
}

// --- Agent thinking ---

func (r *Renderer) onAgentThinking(ev Event) {
	r.updateSpinner(fmt.Sprintf("%s: thinking...", ev.Agent))
}

// --- Tool calls ---

func (r *Renderer) onToolCallStart(ev Event) {
	r.updateSpinner(fmt.Sprintf("%s: %s", ev.Agent, ev.ToolName))
}

func (r *Renderer) onToolCallEnd(ev Event) {
	r.stopSpinner()

	rec := toolCallRecord{
		Name:     ev.ToolName,
		OK:       ev.ToolOK,
		Duration: ev.ToolTime,
		Stage:    ev.Stage,
		Agent:    ev.Agent,
	}
	r.toolCalls = append(r.toolCalls, rec)

	icon := pterm.FgGreen.Sprint("✓")
	if !ev.ToolOK {
		icon = pterm.FgRed.Sprint("✗")
	}
	toolStr := pterm.FgYellow.Sprint(ev.ToolName)
	durStr := pterm.FgDarkGray.Sprintf("(%s)", ev.ToolTime.Truncate(time.Millisecond))
	fmt.Printf("  %s %s %s %s\n", pterm.FgDarkGray.Sprint("│"), icon, toolStr, durStr)

	r.startSpinner(fmt.Sprintf("%s: thinking...", ev.Agent))
}

// --- Sub-agents ---

func (r *Renderer) onSubAgentStart(ev Event) {
	r.stopSpinner()

	rec := subAgentRecord{
		Name:      ev.SubAgentName,
		ID:        ev.SubAgentID,
		TaskTitle: ev.SubTaskTitle,
		TaskCount: ev.SubTaskCount,
		startTime: ev.Timestamp,
	}
	r.subAgents = append(r.subAgents, rec)

	fmt.Printf("  %s %s %s %s\n",
		pterm.FgDarkGray.Sprint("│"),
		pterm.NewStyle(pterm.Bold, pterm.FgMagenta).Sprint("⊕ sub-agent"),
		pterm.FgMagenta.Sprint(ev.SubAgentName),
		pterm.FgDarkGray.Sprintf("(%d sub-tasks)", ev.SubTaskCount))

	if ev.SubTaskTitle != "" {
		fmt.Printf("  %s   %s %s\n",
			pterm.FgDarkGray.Sprint("│"),
			pterm.FgDarkGray.Sprint("└"),
			pterm.FgDarkGray.Sprint(ev.SubTaskTitle))
	}

	r.startSpinner(fmt.Sprintf("sub-agent %s: running %d tasks...", ev.SubAgentName, ev.SubTaskCount))
}

func (r *Renderer) onSubAgentEnd(ev Event) {
	r.stopSpinner()

	for i := len(r.subAgents) - 1; i >= 0; i-- {
		if r.subAgents[i].ID == ev.SubAgentID {
			r.subAgents[i].Duration = ev.Timestamp.Sub(r.subAgents[i].startTime)
			r.subAgents[i].Error = ev.Error
			break
		}
	}

	if ev.Error != "" {
		fmt.Printf("  %s %s %s %s\n",
			pterm.FgDarkGray.Sprint("│"),
			pterm.NewStyle(pterm.Bold, pterm.FgRed).Sprint("⊖ sub-agent"),
			pterm.FgMagenta.Sprint(ev.SubAgentName),
			pterm.FgRed.Sprint("error: "+truncMsg(ev.Error, 50)))
	} else {
		fmt.Printf("  %s %s %s %s\n",
			pterm.FgDarkGray.Sprint("│"),
			pterm.NewStyle(pterm.FgLightMagenta).Sprint("⊕ sub-agent done"),
			pterm.FgMagenta.Sprint(ev.SubAgentName),
			pterm.FgDarkGray.Sprintf("(%d tools, %d facts)", ev.ToolCallCount, ev.FactCount))
	}
}

// --- Task list ---

func (r *Renderer) onTaskListUpdated(ev Event) {
	r.stopSpinner()
	if ev.TaskList == nil {
		return
	}
	r.taskSnapshot = ev.TaskList

	fmt.Println()
	pterm.DefaultSection.WithLevel(2).Println("Tasks")
	items := make([]pterm.BulletListItem, 0, len(ev.TaskList.Tasks))
	for _, item := range ev.TaskList.Tasks {
		icon := taskIcon(item.Status)
		title := item.Title
		if item.HighRisk {
			title += pterm.FgDarkGray.Sprint(" [high-risk]")
		}
		items = append(items, pterm.BulletListItem{
			Level:       0,
			Text:        icon + " " + title,
			Bullet:      "",
			BulletStyle: pterm.NewStyle(),
		})
	}
	pterm.DefaultBulletList.WithItems(items).Render()
	fmt.Println()
	r.startSpinner("working...")
}

func (r *Renderer) onTaskStatusChanged(ev Event) {
	r.stopSpinner()

	icon := taskIcon(ev.TaskStatus)
	statusStr := pterm.FgDarkGray.Sprintf("[%s]", ev.TaskStatus)
	fmt.Printf("  %s %s %s %s\n",
		pterm.FgDarkGray.Sprint("│"), icon, ev.TaskTitle, statusStr)

	r.startSpinner("working...")
}

// --- Transition ---

func (r *Renderer) onTransition(ev Event) {
	r.stopSpinner()

	fromStr := stageStyle(ev.FromStage).Sprint(string(ev.FromStage))
	toStr := stageStyle(ev.ToStage).Sprint(string(ev.ToStage))
	fmt.Printf("    %s %s %s\n", fromStr, pterm.FgDarkGray.Sprint("→"), toStr)
}

// --- Skill ---

func (r *Renderer) onSkillBound(ev Event) {
	r.stopSpinner()

	fmt.Printf("  %s skill: %s\n",
		pterm.FgDarkGray.Sprint("│"),
		pterm.FgLightBlue.Sprint(ev.Skill))
}

// --- Final result rendering ---

// RenderResult formats a completed BusContext into styled terminal output.
func (r *Renderer) RenderResult(busCtx *types.BusContext) string {
	if busCtx == nil {
		return "(no result)\n"
	}
	var b strings.Builder

	// ── Pipeline Summary ──
	b.WriteString("\n")
	b.WriteString(sectionHeader("Pipeline Summary"))

	b.WriteString(fmt.Sprintf("    Trace:    %s\n", pterm.FgDarkGray.Sprint(busCtx.TraceID)))
	b.WriteString(fmt.Sprintf("    Stage:    %s\n", stageStyle(busCtx.PipelineStage).Sprint(string(busCtx.PipelineStage))))

	if busCtx.TaskState.IsTerminal {
		b.WriteString(fmt.Sprintf("    Status:   %s\n", pterm.NewStyle(pterm.Bold, pterm.FgGreen).Sprint("completed")))
	} else {
		b.WriteString(fmt.Sprintf("    Status:   %s\n", pterm.NewStyle(pterm.Bold, pterm.FgYellow).Sprint("incomplete")))
	}

	// Stage history.
	if len(r.stageHistory) > 0 {
		b.WriteString("    Stages:   ")
		for i, sr := range r.stageHistory {
			if i > 0 {
				b.WriteString(pterm.FgDarkGray.Sprint(" → "))
			}
			b.WriteString(stageStyle(sr.Stage).Sprint(string(sr.Stage)))
		}
		b.WriteString("\n")
	} else if len(busCtx.TaskState.Completed) > 0 {
		b.WriteString(fmt.Sprintf("    Stages:   %s\n", pterm.FgDarkGray.Sprint(strings.Join(busCtx.TaskState.Completed, " → "))))
	}

	// Statistics.
	b.WriteString(fmt.Sprintf("    Facts:    %s\n", pterm.FgCyan.Sprintf("%d", len(busCtx.RepoFacts))))

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
		toolStr += pterm.FgRed.Sprintf(" (%d failed)", toolFail)
	}
	b.WriteString(fmt.Sprintf("    Tools:    %s\n", toolStr))

	if len(busCtx.MCPResponses) > 0 {
		b.WriteString(fmt.Sprintf("    MCP:      %s\n", pterm.FgCyan.Sprintf("%d", len(busCtx.MCPResponses))))
	}
	if len(r.subAgents) > 0 {
		b.WriteString(fmt.Sprintf("    SubAgents: %s\n", pterm.FgMagenta.Sprintf("%d", len(r.subAgents))))
	}

	if busCtx.TaskState.LastError != "" {
		b.WriteString(fmt.Sprintf("    Error:    %s\n", pterm.FgRed.Sprint(truncMsg(busCtx.TaskState.LastError, 80))))
	}

	// Task list + Tool calls + Results.
	if busCtx.Mutable != nil {
		tl := busCtx.Mutable.TaskList()
		if len(tl.Tasks) > 0 {
			// ── Tasks ──
			b.WriteString("\n")
			b.WriteString(sectionHeader("Tasks"))
			for _, item := range tl.Tasks {
				icon := taskIcon(item.Status)
				title := item.Title
				if item.HighRisk {
					title += pterm.FgDarkGray.Sprint(" [high-risk]")
				}
				complexity := ""
				if item.Complexity != "" && item.Complexity != "simple" {
					complexity = pterm.FgDarkGray.Sprintf(" (%s)", item.Complexity)
				}
				b.WriteString(fmt.Sprintf("    %s %s%s\n", icon, title, complexity))
			}

			// ── Tool Calls ── (before Results)
			if len(r.toolCalls) > 0 {
				b.WriteString("\n")
				b.WriteString(sectionHeader("Tool Calls"))
				r.writeToolCalls(&b)
			}

			// ── Results ── (Glamour-rendered markdown)
			b.WriteString("\n")
			b.WriteString(sectionHeader("Results"))
			for _, item := range tl.Tasks {
				icon := taskIcon(item.Status)
				b.WriteString(fmt.Sprintf("\n  %s %s\n", icon, pterm.Bold.Sprint(item.Title)))
				if item.Result != "" {
					rendered := r.renderMarkdown(item.Result)
					for _, line := range strings.Split(strings.TrimRight(rendered, "\n"), "\n") {
						b.WriteString(fmt.Sprintf("    %s\n", line))
					}
				} else {
					b.WriteString(fmt.Sprintf("    %s\n", pterm.FgDarkGray.Sprint("(no result)")))
				}
			}
		}
	} else if len(r.toolCalls) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionHeader("Tool Calls"))
		r.writeToolCalls(&b)
	}

	// Sub-agent details.
	if len(r.subAgents) > 0 {
		b.WriteString("\n")
		b.WriteString(sectionHeader("Sub-Agents"))
		for _, sa := range r.subAgents {
			icon := pterm.FgGreen.Sprint("✓")
			if sa.Error != "" {
				icon = pterm.FgRed.Sprint("✗")
			}
			b.WriteString(fmt.Sprintf("    %s %s %s\n",
				icon,
				pterm.FgMagenta.Sprint(sa.Name),
				pterm.FgDarkGray.Sprintf("(%d tasks, %s)",
					sa.TaskCount, sa.Duration.Truncate(time.Millisecond))))
		}
	}

	// Footer hint.
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n", pterm.FgDarkGray.Sprint("Use `codrax -request \"...\"` for single-shot mode")))
	b.WriteString("\n")
	return b.String()
}

// --- helpers ---

func (r *Renderer) writeToolCalls(b *strings.Builder) {
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
		b.WriteString(fmt.Sprintf("    %s (%d calls)\n",
			stageStyle(stage).Sprint(string(stage)), len(tcs)))
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
			icon := pterm.FgGreen.Sprint("✓")
			if ok < cnt {
				icon = pterm.FgRed.Sprint("✗")
			}
			b.WriteString(fmt.Sprintf("      %s %s ×%d\n", icon, pterm.FgYellow.Sprint(name), cnt))
		}
	}
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

func (r *Renderer) startSpinner(msg string) {
	if r.spinner != nil && r.spinner.IsActive {
		r.spinner.UpdateText(msg)
		return
	}
	sp, _ := pterm.DefaultSpinner.
		WithRemoveWhenDone(true).
		WithText(msg).
		Start()
	r.spinner = sp
}

func (r *Renderer) updateSpinner(msg string) {
	if r.spinner != nil && r.spinner.IsActive {
		r.spinner.UpdateText(msg)
	} else {
		r.startSpinner(msg)
	}
}

func (r *Renderer) stopSpinner() {
	if r.spinner != nil && r.spinner.IsActive {
		r.spinner.Stop()
	}
}

// stageStyle returns a PTerm style for a given pipeline stage.
func stageStyle(stage types.PipelineStage) *pterm.Style {
	switch stage {
	case types.StageAnalyze:
		return pterm.NewStyle(pterm.Bold, pterm.FgCyan)
	case types.StageExplore:
		return pterm.NewStyle(pterm.Bold, pterm.FgBlue)
	case types.StagePlan:
		return pterm.NewStyle(pterm.Bold, pterm.FgMagenta)
	case types.StageDesignReview:
		return pterm.NewStyle(pterm.Bold, pterm.FgYellow)
	case types.StageImplement:
		return pterm.NewStyle(pterm.Bold, pterm.FgGreen)
	case types.StageCodeReview:
		return pterm.NewStyle(pterm.Bold, pterm.FgYellow)
	case types.StageVerify:
		return pterm.NewStyle(pterm.Bold, pterm.FgLightCyan)
	case types.StageFinalize:
		return pterm.NewStyle(pterm.Bold, pterm.FgLightGreen)
	default:
		return pterm.NewStyle(pterm.Bold, pterm.FgWhite)
	}
}

// taskIcon returns a colored icon for task status.
func taskIcon(status types.TaskStatus) string {
	switch status {
	case types.TaskDone:
		return pterm.FgGreen.Sprint("✓")
	case types.TaskInProgress:
		return pterm.FgCyan.Sprint("●")
	case types.TaskFailed:
		return pterm.FgRed.Sprint("✗")
	case types.TaskBlocked:
		return pterm.FgYellow.Sprint("⊘")
	default:
		return pterm.FgDarkGray.Sprint("○")
	}
}

// sectionHeader draws a section header with a decorative line.
func sectionHeader(title string) string {
	line := strings.Repeat("─", max(0, 48-len(title)))
	return fmt.Sprintf("  %s %s %s\n",
		pterm.FgDarkGray.Sprint("─"),
		pterm.Bold.Sprint(title),
		pterm.FgDarkGray.Sprint(line))
}

func truncMsg(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
