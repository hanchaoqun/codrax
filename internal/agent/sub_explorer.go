package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/dataflow"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// SubExplorer is a SubAgent that scans files within a given scope
// and discovers facts about the codebase.
type SubExplorer struct {
	base *BaseAgent
}

// NewSubExplorer creates a SubExplorer backed by a BaseAgent with its own evaluator.
func NewSubExplorer(deps *Dependencies) *SubExplorer {
	eval := &subExplorerEvaluator{tools: deps.Tools}
	base := NewBaseAgent("sub_explorer", deps, eval)
	return &SubExplorer{base: base}
}

func (s *SubExplorer) Name() string {
	return "explorer"
}

func (s *SubExplorer) Run(req *types.SubAgentRequest) (*types.SubAgentResult, error) {
	// Shared read: build read-only AgentContext from shared BusContext
	agentCtx := ctxbuilder.BuildSubAgentContext(req.ReadView, req)

	// Build a scan-oriented skill
	sk := &skill.Config{
		Name: "sub_explore",
		Goal: fmt.Sprintf("Explore files in scope [%s] and discover facts", strings.Join(req.Scope, ", ")),
		Workflow: []string{
			"List files in the scoped directories",
			"Read key files to understand structure",
			"Identify types, functions, interfaces, and dependencies",
			"Report discovered facts",
		},
		ToolSuggestions: []string{"read_file", "list_files", "grep"},
		OutputFormat:    "List of discovered facts as JSON",
		Prohibitions:    []string{"Do not modify any files", "Do not explore outside the given scope"},
	}

	// Isolated write: Execute returns StageOutput without touching BusContext
	output, err := s.base.Execute(agentCtx, sk)
	if err != nil {
		return &types.SubAgentResult{
			RequestID: req.ID,
			SubAgent:  req.SubAgent,
			Error:     err.Error(),
		}, err
	}

	// Convert StageOutput to SubAgentResult (private write buffer)
	return &types.SubAgentResult{
		RequestID:     req.ID,
		SubAgent:      req.SubAgent,
		Output:        output.Data,
		Facts:         output.NewFacts,
		EvidenceItems: output.EvidenceItems,
		FlowFindings:  output.FlowFindings,
		Tools:         output.ToolResults,
		MCPResps:      output.MCPResponses,
		Signals:       output.SignalUpdates,
	}, nil
}

// subExplorerEvaluator implements Evaluator, ContinuingEvaluator, and
// SynthesizingEvaluator for the SubExplorer's BaseAgent. Unlike the
// main explorer, it operates within a scoped directory set and has a
// simpler two-phase model: discover files → read and extract evidence.
type subExplorerEvaluator struct {
	tools              *tool.Registry
	objective          string   // user objective, cached for prompts
	scope              []string // scoped directories
	investigationNotes []string // assistant analysis messages
	idleStreak         int      // consecutive no-tool-call rounds
	lastToolCount      int      // tool result count at last check
	structuredEvidence []types.EvidenceItem
	flowFindings       []types.FlowFindingDigest
}

func (e *subExplorerEvaluator) BuildInitialPrompt(ctx *types.AgentContext, sk *skill.Config) string {
	// Cross-Run reset — sub_explorer is a process-lifetime singleton
	// (NewSubExplorer called once from RegisterDefaults). Each
	// SubAgentRequest is an independent scoped investigation, so
	// accumulated state from a previous Run() must not leak into
	// notes/idle counters/evidence of the next one. Mirrors the F2
	// fix in explorer.go (project_repl_equivalence_audit.md).
	if ctx.Objective != e.objective {
		e.investigationNotes = nil
		e.idleStreak = 0
		e.lastToolCount = 0
	}
	e.objective = ctx.Objective
	e.scope = ctx.Constraints // scope passed as constraints in sub-agent context
	e.structuredEvidence = nil
	e.flowFindings = nil

	var b strings.Builder
	b.WriteString("## Scoped Investigation\n\n")
	fmt.Fprintf(&b, "**Objective:** %s\n\n", ctx.Objective)

	if len(ctx.Constraints) > 0 {
		b.WriteString("**Scope:** Limit your investigation to these directories/files:\n")
		for _, s := range ctx.Constraints {
			fmt.Fprintf(&b, "- `%s`\n", s)
		}
		b.WriteString("\n")
	}

	b.WriteString("**Strategy:**\n")
	b.WriteString("1. List files in scope to understand what's available\n")
	b.WriteString("2. Grep for key terms related to the objective (files_only=true first; use file_type when the language is obvious)\n")
	b.WriteString("3. Read the most relevant files and extract structured evidence; for line-level grep prefer context_lines=3 before a larger read_file\n\n")

	b.WriteString("**Evidence format** — after each file, extract facts as:\n\n")
	b.WriteString("```\n")
	b.WriteString("## Evidence from [filename]\n")
	b.WriteString("- [DIRECT] `functionName` line N: <what this code establishes>\n")
	b.WriteString("- [CONDITIONAL] `functionName` line N: <what happens> IF <condition>\n")
	b.WriteString("- [REGISTRATION] `functionName` line N: <what is registered, EXACT values>\n")
	b.WriteString("- [MECHANISM] `functionName` line N: <how something works>\n")
	b.WriteString("- [RELATIONSHIP] `symbolA` → `symbolB`: <nature of link>\n")
	b.WriteString("```\n\n")

	b.WriteString("**Rules:**\n")
	b.WriteString("- **Line numbers must come from the gutter.** Every `read_file` result shows each line with its absolute line number in the left gutter (format `   123│ code...`). When you write `line N`, `N` MUST be the exact gutter number — do not estimate, do not interpolate. If you are not certain, leave the `line N` part off.\n")
	b.WriteString("- Extract EVERY fact that might be relevant — err on over-collecting\n")
	b.WriteString("- For short methods (getName, isEnabled, etc.): ALWAYS record the exact return value as [DIRECT]\n")
	b.WriteString("- For [REGISTRATION]: note EXACT concrete values, not summaries\n")
	b.WriteString("- **Large file strategy:** grep first for key identifiers, prefer context_lines=3, then read only matched line ranges\n")
	b.WriteString("- Stay within scope — do not read files outside the specified directories\n")

	return b.String()
}

func (e *subExplorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	// Never hard-stop; use ContinuationPrompt for soft-stop control.
	return false
}

// ContinuationPrompt implements ContinuingEvaluator. Tracks investigation
// progress and pushes for more coverage when the LLM stops too early.
func (e *subExplorerEvaluator) ContinuationPrompt(resp llm.Response, iteration int, continuationCount int, history []types.ToolResult) (string, bool) {
	// Capture investigation notes from content-only responses.
	if resp.Content != "" {
		e.investigationNotes = append(e.investigationNotes, resp.Content)
	}

	// Track idle streak (consecutive rounds with no new tool results).
	if len(history) > e.lastToolCount {
		e.idleStreak = 0
	}
	e.lastToolCount = len(history)
	e.idleStreak++

	// After 2 idle rounds, let the loop terminate.
	if e.idleStreak >= 2 {
		return "", false
	}

	// Check file coverage within scope.
	_, readSet := extractFileCoverage(history)

	// If we haven't read any files yet, push harder.
	if len(readSet) == 0 && continuationCount < 3 {
		return "You have not read any files yet. Use list_files to discover files in scope, " +
			"then read the most relevant ones and extract evidence entries.", true
	}

	// Count evidence quality.
	directCount := 0
	for _, note := range e.investigationNotes {
		for _, line := range strings.Split(note, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [DIRECT]") || strings.HasPrefix(trimmed, "- [REGISTRATION]") {
				directCount++
			}
		}
	}

	// If we have files but weak evidence, push for more extraction.
	if directCount < 2 && continuationCount < 3 {
		var hint strings.Builder
		fmt.Fprintf(&hint, "File coverage: %d files read.\n", len(readSet))
		hint.WriteString("You need at least 2 [DIRECT] or [REGISTRATION] evidence entries. ")
		hint.WriteString("Read more files in scope or re-examine the files you already read — ")
		hint.WriteString("look for return values, registrations, and concrete configurations.")
		return hint.String(), true
	}

	// Gentle coverage reminder if there are still things to explore.
	if len(readSet) < 3 && continuationCount < 4 {
		return fmt.Sprintf("File coverage: %d files read. If there are more relevant files "+
			"in scope, read them now. Otherwise you may stop.", len(readSet)), true
	}

	return "", false
}

// SynthesisPrompt implements SynthesizingEvaluator. Produces a clean
// synthesis call with all collected evidence.
func (e *subExplorerEvaluator) SynthesisPrompt(ctx *types.AgentContext, toolResults []types.ToolResult) (string, bool) {
	// Only synthesize if we have evidence-bearing results.
	hasEvidence := false
	for _, r := range toolResults {
		if r.Success && e.toolConfidence(r.ToolName) > 0.5 {
			hasEvidence = true
			break
		}
	}
	if !hasEvidence {
		return "", false
	}

	e.ensureStructuredEvidence(ctx, toolResults)

	var digest strings.Builder
	digest.WriteString("Synthesize your findings from the scoped investigation.\n\n")
	fmt.Fprintf(&digest, "## Objective\n%s\n\n", e.objective)

	if len(e.investigationNotes) > 0 {
		digest.WriteString("## Evidence Collected\n\n")
		for i, note := range e.investigationNotes {
			if len(note) > 1200 {
				note = note[:1200] + "\n... [truncated]"
			}
			fmt.Fprintf(&digest, "### Evidence Set %d\n%s\n\n", i+1, note)
		}
	}

	if section := formatEvidenceSection(e.structuredEvidence, types.EvidenceConcrete, "Structured Concrete Evidence", 12); section != "" {
		digest.WriteString(section)
	}
	if section := formatFlowFindingsSection(e.flowFindings, "Resolved Dataflow Paths", 8, false); section != "" {
		digest.WriteString(section)
	}
	if section := formatEvidenceSection(e.structuredEvidence, types.EvidenceConditional, "Condition Guards", 8); section != "" {
		digest.WriteString(section)
	}
	if section := formatFlowFindingsSection(e.flowFindings, "Conflicts / Unknowns", 6, true); section != "" {
		digest.WriteString(section)
	}

	// Compact file list.
	digest.WriteString("## Files Read\n\n")
	for _, r := range toolResults {
		if r.Success && r.ToolName == "read_file" {
			first := strings.SplitN(r.Summary, "\n", 2)[0]
			digest.WriteString("- " + first + "\n")
		}
	}
	digest.WriteString("\n")

	digest.WriteString("## Instructions\n\n")
	digest.WriteString("Provide a comprehensive summary of what you found:\n")
	digest.WriteString("- Name SPECIFIC types, functions, and values — not categories\n")
	digest.WriteString("- Ground claims in file:line citations\n")
	digest.WriteString("- If evidence is incomplete or uncertain, say so explicitly\n")

	return digest.String(), true
}

func (e *subExplorerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error) {
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}

	// Extract facts from successful tool results, using each tool's
	// declared Confidence.
	var facts []types.RepoFact
	sources := make(map[string]struct{})
	for _, r := range toolResults {
		if r.Success {
			confidence := e.toolConfidence(r.ToolName)
			facts = append(facts, types.RepoFact{
				Key:         r.ToolName,
				Value:       r.Summary,
				Source:      logicalFactSource(r.Summary, r.ToolName),
				EvidenceRef: r.RawRef,
				Confidence:  confidence,
			})
			if confidence > 0.5 {
				sources[r.ToolName] = struct{}{}
			}
		}
	}

	e.ensureStructuredEvidence(ctx, toolResults)

	// Multi-dimensional quality check (same criteria as main explorer,
	// but with relaxed thresholds for scoped sub-tasks).
	_, readSet := extractFileCoverage(toolResults)
	directCount := 0
	for _, note := range e.investigationNotes {
		for _, line := range strings.Split(note, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [DIRECT]") || strings.HasPrefix(trimmed, "- [REGISTRATION]") {
				directCount++
			}
		}
	}

	toolDiversity := len(sources) >= 2
	fileCoverage := len(readSet) >= 1
	evidenceQuality := directCount >= 1
	hasEnough := toolDiversity && fileCoverage && evidenceQuality

	signals := &types.ExecutionSignals{HasEnoughFacts: hasEnough}

	rankedEvidence := rankEvidenceByRelevance(e.objective, e.structuredEvidence, readSet)
	rankedFindings := rankFindingsByRelevance(e.objective, e.flowFindings)

	return &StageOutput{
		Data:          json.RawMessage(fmt.Sprintf(`{"result": %q}`, lastContent)),
		NewFacts:      facts,
		EvidenceItems: rankedEvidence,
		FlowFindings:  rankedFindings,
		SignalUpdates: signals,
	}, nil
}

func (e *subExplorerEvaluator) toolConfidence(name string) float64 {
	if e.tools == nil {
		return 0.8
	}
	t, err := e.tools.Get(name)
	if err != nil {
		return 0.8
	}
	return t.Confidence()
}

func (e *subExplorerEvaluator) DetermineMissingPiece(ctx *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output.SignalUpdates != nil && output.SignalUpdates.HasEnoughFacts {
		return types.MissingPlan
	}
	return types.MissingFacts
}

func (e *subExplorerEvaluator) ensureStructuredEvidence(ctx *types.AgentContext, toolResults []types.ToolResult) {
	if len(e.structuredEvidence) > 0 || len(e.flowFindings) > 0 {
		return
	}

	parsed := parseEvidenceItems(e.investigationNotes, "sub_explorer.llm")
	graph, candidates := buildScopedSearchGraph(ctx, toolResults, e.scope)
	// Deterministic line grounding — see groundEvidenceItems and
	// memory/project_fake_green_audit_2026_04_14.md Pattern 2.
	// Runs here so the grounded items flow through regardless of
	// whether dataflow analysis fires below.
	parsed = groundEvidenceItems(parsed, graph, toolResults)
	if graph == nil || !needsDataflowAnalysis(e.objective, parsed) {
		e.structuredEvidence = parsed
		return
	}

	result := dataflow.Analyze(graph, dataflow.Options{
		RepoRoot:        ctx.RepoRoot,
		Question:        e.objective,
		Scope:           e.scope,
		CandidateFiles:  candidates,
		WorkDir:         ctx.WorkDir,
		MaxFiles:        40,
		MaxIterations:   6,
		MaxNodesPerFunc: 400,
	})
	e.structuredEvidence = mergeEvidenceItems(parsed, result.Evidence)
	e.flowFindings = mergeFlowFindings(result.Findings)
}

func buildScopedSearchGraph(ctx *types.AgentContext, toolResults []types.ToolResult, scope []string) (*repomap.Graph, []string) {
	if ctx.RepoRoot == "" {
		return nil, nil
	}
	graph, err := repomap.BuildOrLoadGraph(ctx.RepoRoot, egrepQueryFromObjective(ctx.Objective))
	if err != nil || graph == nil {
		return nil, nil
	}
	_, readSet := extractFileCoverage(toolResults)
	candidateSet := make(map[string]bool)
	for file := range readSet {
		candidateSet[file] = true
	}
	for file := range graph.FileIndex {
		for _, prefix := range scope {
			if !looksLikePath(prefix) {
				continue
			}
			if strings.HasPrefix(file, prefix) {
				candidateSet[file] = true
			}
		}
	}
	var candidates []string
	for file := range candidateSet {
		candidates = append(candidates, file)
	}
	sort.Strings(candidates)
	return graph, candidates
}

func egrepQueryFromObjective(objective string) string {
	fields := strings.Fields(objective)
	if len(fields) > 12 {
		fields = fields[:12]
	}
	return strings.Join(fields, " ")
}
