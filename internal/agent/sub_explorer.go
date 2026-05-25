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
	deps *Dependencies
}

// NewSubExplorer creates a SubExplorer factory. Each Run builds a fresh
// BaseAgent/evaluator pair so parallel sub-agent branches cannot share mutable
// investigation state.
func NewSubExplorer(deps *Dependencies) *SubExplorer {
	return &SubExplorer{deps: deps}
}

func (s *SubExplorer) Name() string {
	return "explorer"
}

func (s *SubExplorer) Run(req *types.SubAgentRequest) (*types.SubAgentResult, error) {
	// Shared read: build read-only AgentContext from shared BusContext
	agentCtx := ctxbuilder.BuildSubAgentContext(req.ReadView, req)
	deps := s.deps
	if deps == nil {
		deps = &Dependencies{}
	}
	eval := &subExplorerEvaluator{tools: deps.Tools}
	base := NewBaseAgent("sub_explorer", deps, eval)

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
	output, err := base.Execute(agentCtx, sk)
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
		InvestigationNotes: subExplorerAdvisoryNotes(
			req,
			eval.investigationNotes,
			output.StageReport,
		),
		Tools:    output.ToolResults,
		MCPResps: output.MCPResponses,
		Signals:  output.SignalUpdates,
	}, nil
}

const (
	subExplorerAdvisoryMaxNotes     = 3
	subExplorerAdvisoryMaxNoteBytes = 32768
)

func subExplorerAdvisoryNotes(req *types.SubAgentRequest, notes []string, stageReport string) []string {
	candidates := append([]string(nil), notes...)
	if strings.TrimSpace(stageReport) != "" {
		candidates = append(candidates, stageReport)
	}
	if len(candidates) == 0 {
		return nil
	}
	start := len(candidates) - subExplorerAdvisoryMaxNotes
	if start < 0 {
		start = 0
	}
	seen := make(map[string]struct{}, len(candidates)-start)
	out := make([]string, 0, len(candidates)-start)
	for _, note := range candidates[start:] {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		if _, ok := seen[note]; ok {
			continue
		}
		seen[note] = struct{}{}
		out = append(out, formatSubExplorerAdvisoryNote(req, truncateSubExplorerAdvisoryNote(note)))
	}
	return out
}

func formatSubExplorerAdvisoryNote(req *types.SubAgentRequest, note string) string {
	if req == nil {
		return "Sub-agent advisory result (not a citation):\n\n" + note
	}
	var b strings.Builder
	b.WriteString("Sub-agent advisory result (not a citation):")
	if req.ID != "" {
		fmt.Fprintf(&b, " request_id=%s", req.ID)
	}
	if req.SubAgent != "" {
		fmt.Fprintf(&b, " sub_agent=%s", req.SubAgent)
	}
	if req.Objective != "" {
		fmt.Fprintf(&b, " objective=%q", req.Objective)
	}
	if len(req.Scope) > 0 {
		fmt.Fprintf(&b, " scope=%q", strings.Join(req.Scope, ", "))
	}
	b.WriteString("\n\n")
	b.WriteString(note)
	return b.String()
}

func truncateSubExplorerAdvisoryNote(note string) string {
	if len(note) <= subExplorerAdvisoryMaxNoteBytes {
		return note
	}
	cut := subExplorerAdvisoryMaxNoteBytes
	for cut > 0 && (note[cut]&0xC0) == 0x80 {
		cut--
	}
	if cut <= 0 {
		cut = subExplorerAdvisoryMaxNoteBytes
	}
	return strings.TrimSpace(note[:cut]) + "\n\n[advisory note truncated]"
}

// subExplorerEvaluator implements Evaluator and LoopController for
// the SubExplorer's BaseAgent. Unlike the
// main explorer, it operates within a scoped directory set and has a
// simpler two-phase model: discover files → read and extract evidence.
//
// Loop-control state that USED to live on this struct (idleStreak,
// lastToolCount) has been lifted into LoopPolicy; this evaluator now
// just detects conditions and returns a LoopSignal. The policy owns
// idle counting, throttling, and budget enforcement.
type subExplorerEvaluator struct {
	tools              *tool.Registry
	objective          string   // user objective, cached for prompts
	scope              []string // scoped directories
	repoRoot           string   // repository root, cached from BuildInitialInstruction so Observe can canonicalise read_file banners with absolute paths (session 22)
	investigationNotes []string // assistant analysis messages
	structuredEvidence []types.EvidenceItem
	flowFindings       []types.FlowFindingDigest
}

func (e *subExplorerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, sk *skill.Config) string {
	// Cross-Run reset — sub_explorer is a process-lifetime singleton
	// (NewSubExplorer called once from RegisterDefaults). Each
	// SubAgentRequest is an independent scoped investigation, so
	// accumulated state from a previous Run() must not leak into
	// notes/evidence of the next one. Mirrors the F2 fix in
	// explorer.go (project_repl_equivalence_audit.md). Idle counting
	// and tool-count deltas moved to LoopPolicy, which constructs a
	// fresh loopPolicyState per Execute call — no reset needed here.
	if ctx.Objective != e.objective {
		e.investigationNotes = nil
	}
	e.objective = ctx.Objective
	e.scope = ctx.Constraints // scope passed as constraints in sub-agent context
	e.repoRoot = ctx.RepoRoot
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

	b.WriteString("**Strategy** (adapt to what you find):\n")
	b.WriteString("1. List files in scope to understand what's available\n")
	b.WriteString("2. Grep for key terms related to the objective — use `files_only=true` for discovery, line-level grep with `context_lines=3` for targeted search\n")
	b.WriteString("3. Read the most relevant files and extract structured evidence\n")
	b.WriteString("4. Use `grep` + `read_file` together: grep to locate patterns efficiently, read_file for full context when needed\n\n")

	b.WriteString("**Evidence format** (examples — adapt the tags and structure to what you find):\n\n")
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
	b.WriteString("- Stay within scope — do not read files outside the specified directories\n")

	return b.String()
}

func (e *subExplorerEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	// Never hard-stop; use LoopController.Observe for soft-stop control.
	return false
}

// Observe implements LoopController. The sub_explorer only cares
// about the soft-stop phase — nothing to detect mid-loop because it
// runs a simple discover→read→extract flow without the complex
// phase state the main explorer tracks. The mid-loop path returns
// an empty signal so the policy just continues.
//
// Detection logic (soft-stop only):
//
//   - Captures any assistant content into investigationNotes for
//     later synthesis. This is a side effect on the evaluator's
//     detection state, not loop-control state.
//   - Checks file coverage within scope. No files read and we still
//     have continuation budget → hint HARD to use list_files.
//   - Counts [DIRECT] / [REGISTRATION] evidence entries in the
//     collected notes. Too few (<2) and still within budget → hint
//     for more extraction.
//   - At least 3 files read is the "plenty" threshold; below that
//     with remaining budget → gentle coverage reminder.
//
// Loop-policy concerns (idle streak, throttle, dedup, budget) are
// NOT implemented here — LoopPolicy owns all of them. This evaluator
// returns the raw detection result and the policy decides.
func (e *subExplorerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseSoftStop {
		return LoopSignal{}
	}

	// Capture investigation notes from content-only responses. The
	// side effect is on detection state, not loop-control state, so
	// it stays on the evaluator.
	if obs.Response.Content != "" {
		e.investigationNotes = append(e.investigationNotes, obs.Response.Content)
	}

	// File coverage within scope.
	_, readSet, _ := extractFileCoverage(obs.AllToolResults, e.repoRoot)

	// If we haven't read any files yet, push HARD.
	if len(readSet) == 0 && obs.ContinuationsUsed < 3 {
		return LoopSignal{
			HintRequested: true,
			HintKey:       "sub_explorer.no_files_read",
			Hint: "You have not read any files yet. Use list_files to discover files in scope, " +
				"then read the most relevant ones and extract evidence entries.",
		}
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
	if directCount < 2 && obs.ContinuationsUsed < 3 {
		var hint strings.Builder
		fmt.Fprintf(&hint, "File coverage: %d files read.\n", len(readSet))
		hint.WriteString("You need at least 2 [DIRECT] or [REGISTRATION] evidence entries. ")
		hint.WriteString("Read more files in scope or re-examine the files you already read — ")
		hint.WriteString("look for return values, registrations, and concrete configurations.")
		return LoopSignal{
			HintRequested: true,
			HintKey:       "sub_explorer.weak_evidence",
			Hint:          hint.String(),
		}
	}

	// Gentle coverage reminder if there are still things to explore.
	if len(readSet) < 3 && obs.ContinuationsUsed < 4 {
		return LoopSignal{
			HintRequested: true,
			HintKey:       "sub_explorer.coverage_reminder",
			Hint: fmt.Sprintf("File coverage: %d files read. If there are more relevant files "+
				"in scope, read them now. Otherwise you may stop.", len(readSet)),
		}
	}

	// Nothing to nudge; the policy will accept the soft-stop.
	return LoopSignal{}
}

func (e *subExplorerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, mcpResponses []types.MCPResponse) (*StageOutput, error) {
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
	_, readSet, _ := extractFileCoverage(toolResults, e.repoRoot)
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
		Data:          json.RawMessage(`{}`),
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
		return types.MissingNone
	}
	return types.MissingFacts
}

func (e *subExplorerEvaluator) ensureStructuredEvidence(ctx *types.AgentContext, toolResults []types.ToolResult) {
	if len(e.structuredEvidence) > 0 || len(e.flowFindings) > 0 {
		return
	}

	parsed := parseEvidenceItems(e.investigationNotes, "sub_explorer.llm")
	graph, candidates := buildScopedSearchGraph(ctx, toolResults, e.scope)
	// Grounding moved upstream into emit_evidence.Execute (Tier 1/2 +
	// recovery). Sub-agents currently do not route through emit_evidence
	// (Mutable is nil for them), so items surfaced via notes arrive
	// ungrounded here; downstream rendering treats empty GroundingStatus
	// like ungrounded for citation-pool purposes.
	if graph == nil || !needsDataflowAnalysis(requestModelFromContext(ctx), parsed) {
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
		MaxItemsPerFile: 50,
	})
	e.structuredEvidence = mergeEvidenceItems(parsed, result.Evidence)
	e.flowFindings = mergeFlowFindings(result.Findings)
}

func buildScopedSearchGraph(ctx *types.AgentContext, toolResults []types.ToolResult, scope []string) (*repomap.Graph, []string) {
	if ctx.RepoRoot == "" {
		return nil, nil
	}
	// Reuse the main explorer's graph when the sub-agent dispatch
	// carries one. BuildSubAgentContext copies the handle from
	// bus.Mutable.SearchGraph(), so every sub-agent spawned after the
	// main explorer's keyword_search skips a second BuildOrLoadGraph
	// (~50-500ms per call depending on cache state). Falls back to
	// building fresh when the handle is missing (tests, unusual flow).
	var graph *repomap.Graph
	if ctx.SearchGraph != nil {
		if g, ok := ctx.SearchGraph.(*repomap.Graph); ok {
			graph = g
		}
	}
	if graph == nil {
		g, err := repomap.GraphFromAgentContextOrLoad(ctx, ctx.RepoRoot, egrepQueryFromObjective(ctx.Objective))
		if err != nil || g == nil {
			return nil, nil
		}
		graph = g
	}
	_, readSet, _ := extractFileCoverage(toolResults, ctx.RepoRoot)
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
