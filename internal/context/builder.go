package context

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildAgentContext trims a full BusContext into an Agent-scoped view.
// It selects only the facts, tools, and summaries relevant to the given agent and stage.
func BuildAgentContext(bus *types.BusContext, agentName types.AgentName, stage types.PipelineStage) *types.AgentContext {
	tl := bus.Mutable.TaskList()

	ac := &types.AgentContext{
		AgentName:    agentName,
		Stage:        stage,
		Objective:    tl.Objective,
		MissingPiece: bus.TaskState.Missing,
		Constraints:  bus.Constraints,
		Preferences:  bus.Preferences,
		RepoRoot:     bus.RepoRoot,
		Branch:       bus.Branch,
		Commit:       bus.Commit,
		WorkDir:      bus.WorkDir,
		Mutable:      bus.Mutable, // shared pointer; tools mutate through this
	}

	// Set current task info
	if task := tl.CurrentTask(); task != nil {
		ac.CurrentTaskID = task.ID
		ac.CurrentTask = task.Title
		ac.CurrentTaskWriting = task.Writing
		ac.CurrentTaskHighRisk = task.HighRisk
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

// reasoningHygiene is a meta-rule injected into every agent's system
// prompt. It encodes one principle: when the answer is the result of
// a deterministic computation over data, run a tool that produces it
// directly — do not derive it by inspection. Language models reliably
// miscount, missort, and misaggregate even on small lists, so any
// quantitative or structural claim should come from a tool's output,
// not from the model's own reading of an upstream tool result.
//
// Kept here (not in any one agent or skill) so the rule applies to
// the entire pipeline: analyzer routing, explorer fact-gathering,
// planner estimates, verifier counts of failures, etc.
const reasoningHygiene = "Whenever the answer is the result of a deterministic computation over data — counting, summing, sorting, finding extremes, diffing, hashing, filtering by exact criteria — run a tool that produces it directly (e.g. a shell pipeline through exec_command such as `find ... | wc -l`, `grep -c`, `sort | uniq`) and treat the tool's output as authoritative. Never derive such answers by reading a list_files / grep / read_file output yourself; language models miscount and miscompute even on short lists. The same rule applies to facts you intend to record: if a fact has a number, sort order, or set membership in it, that number/order/set must come from a tool, not from your inspection."

// BuildPromptContext assembles the final prompt payload from an AgentContext and Skill config.
func BuildPromptContext(ac *types.AgentContext, sk *skill.Config) *types.PromptContext {
	pc := &types.PromptContext{
		AgentName: ac.AgentName,
		Stage:     ac.Stage,
		SkillName: sk.Name,
	}

	// System sections — identity and constraints. The Objective lives in
	// UserSections instead (see below) so the LLM sees the user request
	// as a real user-role message rather than buried in the system role.
	pc.SystemSections = []types.PromptSection{
		{
			Title: "Agent Identity",
			Content: fmt.Sprintf("You are the %s agent operating in the %s stage.",
				ac.AgentName, ac.Stage),
		},
		{
			Title:   "Reasoning Hygiene",
			Content: reasoningHygiene,
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

	// User sections — task-specific context. Objective comes first so
	// the user request is the most prominent thing the LLM sees in the
	// user role.
	if ac.Objective != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "User Request",
			Content: ac.Objective,
		})
	}

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

// BuildSubAgentContext builds a read-only AgentContext for a SubAgent
// from the shared BusContext. Unlike BuildAgentContext, the
// objective/scope/constraints come from the SubAgentRequest, not
// from the stage config.
//
// Intentionally omits BusContext.Mutable: SubAgents are isolated
// workers that report results via SubAgentResult, not by mutating
// the parent's working state. Leaving ac.Mutable nil here means any
// tool that requires it (currently todo_write) will fail-stop with
// a clear error rather than silently racing against parallel
// sub-agents over the shared task list. The SubAgentReducer is the
// single point at which sub-agent results re-enter the parent's
// state, keeping the per-task aggregation boundary explicit.
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
		WorkDir:      bus.WorkDir,
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
