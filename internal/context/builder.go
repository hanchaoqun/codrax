package context

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/design/internal/skill"
	"github.com/hanchaoqun/design/internal/types"
)

// BuildAgentContext trims a full BusContext into an Agent-scoped view.
// It selects only the facts, tools, and summaries relevant to the given agent and stage.
func BuildAgentContext(bus *types.BusContext, agentName types.AgentName, stage types.PipelineStage) *types.AgentContext {
	ac := &types.AgentContext{
		AgentName:    agentName,
		Stage:        stage,
		Objective:    bus.TaskList.Objective,
		MissingPiece: bus.TaskState.Missing,
		Constraints:  bus.Constraints,
		Preferences:  bus.Preferences,
		RepoRoot:     bus.RepoRoot,
		Branch:       bus.Branch,
		Commit:       bus.Commit,
	}

	// Set current task info
	if task := bus.TaskList.CurrentTask(); task != nil {
		ac.CurrentTaskID = task.ID
		ac.CurrentTask = task.Title
		ac.CurrentTaskType = task.Type
	}

	// Collect relevant facts
	ac.RelevantFacts = extractRelevantFacts(bus.RepoFacts)

	// Collect relevant files from facts
	ac.RelevantFiles = extractRelevantFiles(bus.RepoFacts)

	// Collect tool summaries
	ac.RelevantToolSummaries = extractToolSummaries(bus.ToolResults)

	// Collect MCP notes
	ac.RelevantMCPNotes = extractMCPNotes(bus.MCPResponses)

	// Build summaries from signals
	if bus.Signals.HasPlan {
		ac.PlanSummary = findSummaryFromResults(bus.ToolResults, "plan")
	}
	if bus.Signals.HasPatch {
		ac.PatchSummary = findSummaryFromResults(bus.ToolResults, "patch")
	}
	if bus.Signals.DesignReviewPassed {
		ac.ReviewSummary = "Design review passed"
	}
	if bus.Signals.CodeReviewPassed {
		ac.ReviewSummary = "Code review passed"
	}
	if bus.Signals.VerificationPassed {
		ac.VerificationSummary = "Verification passed"
	}

	return ac
}

// BuildPromptContext assembles the final prompt payload from an AgentContext and Skill config.
func BuildPromptContext(ac *types.AgentContext, sk *skill.Config) *types.PromptContext {
	pc := &types.PromptContext{
		AgentName: ac.AgentName,
		Stage:     ac.Stage,
		SkillName: sk.Name,
	}

	// System sections — identity and constraints
	pc.SystemSections = []types.PromptSection{
		{
			Title: "Agent Identity",
			Content: fmt.Sprintf("You are the %s agent operating in the %s stage.",
				ac.AgentName, ac.Stage),
		},
		{
			Title: "Objective",
			Content: ac.Objective,
		},
	}

	if len(ac.Constraints) > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   "Constraints",
			Content: strings.Join(ac.Constraints, "\n"),
		})
	}

	// Skill instructions — merged into system sections
	pc.SystemSections = append(pc.SystemSections,
		types.PromptSection{
			Title:   "Skill Goal",
			Content: sk.Goal,
		},
		types.PromptSection{
			Title:   "Workflow",
			Content: formatNumberedList(sk.Workflow),
		},
		types.PromptSection{
			Title:   "Output Format",
			Content: sk.OutputFormat,
		},
	)

	if len(sk.Prohibitions) > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   "Prohibitions",
			Content: formatBulletList(sk.Prohibitions),
		})
	}

	// User sections — task-specific context
	if ac.CurrentTask != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Current Task",
			Content: fmt.Sprintf("[%s] %s", ac.CurrentTaskID, ac.CurrentTask),
		})
	}

	if len(ac.RelevantFacts) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Known Facts",
			Content: strings.Join(ac.RelevantFacts, "\n"),
		})
	}

	if len(ac.RelevantFiles) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Relevant Files",
			Content: strings.Join(ac.RelevantFiles, "\n"),
		})
	}

	if ac.PlanSummary != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Plan Summary",
			Content: ac.PlanSummary,
		})
	}

	if ac.PatchSummary != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Patch Summary",
			Content: ac.PatchSummary,
		})
	}

	if ac.ReviewSummary != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Review Summary",
			Content: ac.ReviewSummary,
		})
	}

	if ac.VerificationSummary != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Verification Summary",
			Content: ac.VerificationSummary,
		})
	}

	if ac.MissingPiece != types.MissingNone {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Missing Piece",
			Content: fmt.Sprintf("Currently missing: %s", ac.MissingPiece),
		})
	}

	// Enabled tools from skill suggestions
	pc.EnabledTools = sk.ToolSuggestions

	return pc
}

// ToMessages converts a PromptContext into a flat message list for the LLM.
func ToMessages(pc *types.PromptContext) []Message {
	var messages []Message

	// System message
	var systemParts []string
	for _, s := range pc.SystemSections {
		systemParts = append(systemParts, fmt.Sprintf("## %s\n%s", s.Title, s.Content))
	}
	if len(systemParts) > 0 {
		messages = append(messages, Message{
			Role:    "system",
			Content: strings.Join(systemParts, "\n\n"),
		})
	}

	// User message
	var userParts []string
	for _, s := range pc.UserSections {
		userParts = append(userParts, fmt.Sprintf("## %s\n%s", s.Title, s.Content))
	}
	if len(userParts) > 0 {
		messages = append(messages, Message{
			Role:    "user",
			Content: strings.Join(userParts, "\n\n"),
		})
	}

	return messages
}

// Message is a simplified message struct for prompt building.
type Message struct {
	Role    string
	Content string
}

// BuildSubAgentContext builds a read-only AgentContext for a SubAgent from the shared BusContext.
// Unlike BuildAgentContext, the objective/scope/constraints come from the SubAgentRequest,
// not from the stage config.
func BuildSubAgentContext(bus *types.BusContext, req *types.SubAgentRequest) *types.AgentContext {
	ac := &types.AgentContext{
		AgentName:    types.AgentName(req.SubAgent),
		Stage:        bus.PipelineStage,
		Objective:    req.Objective,
		Constraints:  req.Constraints,
		MissingPiece: bus.TaskState.Missing,
		RepoRoot:     bus.RepoRoot,
		Branch:       bus.Branch,
		Commit:       bus.Commit,
	}

	// Shared read from BusContext
	ac.RelevantFacts = extractRelevantFacts(bus.RepoFacts)
	ac.RelevantFiles = filterFilesByScope(bus.RepoFacts, req.Scope)
	ac.RelevantToolSummaries = extractToolSummaries(bus.ToolResults)
	ac.RelevantMCPNotes = extractMCPNotes(bus.MCPResponses)

	if bus.Signals.HasPlan {
		ac.PlanSummary = findSummaryFromResults(bus.ToolResults, "plan")
	}
	if bus.Signals.HasPatch {
		ac.PatchSummary = findSummaryFromResults(bus.ToolResults, "patch")
	}

	return ac
}

// filterFilesByScope returns only files whose source path matches one of the scope prefixes.
func filterFilesByScope(facts []types.RepoFact, scope []string) []string {
	if len(scope) == 0 {
		return extractRelevantFiles(facts)
	}
	seen := make(map[string]bool)
	var files []string
	for _, f := range facts {
		if f.Source == "" || seen[f.Source] {
			continue
		}
		for _, s := range scope {
			if strings.HasPrefix(f.Source, s) {
				seen[f.Source] = true
				files = append(files, f.Source)
				break
			}
		}
	}
	return files
}

// --- helpers ---

func extractRelevantFacts(facts []types.RepoFact) []string {
	result := make([]string, 0, len(facts))
	for _, f := range facts {
		result = append(result, fmt.Sprintf("[%s] %s = %s (source: %s, confidence: %.2f)",
			f.Key, f.Key, f.Value, f.Source, f.Confidence))
	}
	return result
}

func extractRelevantFiles(facts []types.RepoFact) []string {
	seen := make(map[string]bool)
	var files []string
	for _, f := range facts {
		if f.Source != "" && !seen[f.Source] {
			seen[f.Source] = true
			files = append(files, f.Source)
		}
	}
	return files
}

func extractToolSummaries(results []types.ToolResult) []string {
	summaries := make([]string, 0, len(results))
	for _, r := range results {
		status := "OK"
		if !r.Success {
			status = "FAIL"
		}
		summaries = append(summaries, fmt.Sprintf("[%s] %s: %s", status, r.ToolName, r.Summary))
	}
	return summaries
}

func extractMCPNotes(responses []types.MCPResponse) []string {
	notes := make([]string, 0, len(responses))
	for _, r := range responses {
		status := "OK"
		if !r.Success {
			status = "FAIL"
		}
		notes = append(notes, fmt.Sprintf("[%s] %s.%s: %s", status, r.ServerName, r.Method, r.Summary))
	}
	return notes
}

func findSummaryFromResults(results []types.ToolResult, keyword string) string {
	for _, r := range results {
		if r.Success && strings.Contains(strings.ToLower(r.Summary), keyword) {
			return r.Summary
		}
	}
	return ""
}

func formatNumberedList(items []string) string {
	var b strings.Builder
	for i, item := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, item)
	}
	return b.String()
}

func formatBulletList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	return b.String()
}
