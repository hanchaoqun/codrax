package context

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
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
		AnalysisIR:   bus.AnalysisIR,
	}

	// Set current task info
	if task := tl.CurrentTask(); task != nil {
		ac.CurrentTaskID = task.ID
		ac.CurrentTask = task.Title
		ac.CurrentTaskDescription = task.Description
	}

	// Collect relevant facts
	ac.RelevantFacts = extractRelevantFacts(bus.RepoFacts)

	// Collect relevant files from facts
	ac.RelevantFiles = extractRelevantFiles(bus.RepoFacts)
	ac.EvidenceItems = append([]types.EvidenceItem(nil), bus.EvidenceItems...)
	ac.FlowFindings = append([]types.FlowFindingDigest(nil), bus.FlowFindings...)
	ac.AnswerChains = append([]types.AnswerChain(nil), bus.AnswerChains...)
	ac.AnswerSymbols = append([]types.AnswerSymbol(nil), bus.AnswerSymbols...)
	ac.AnswerSymbolCompleteness = bus.AnswerSymbolCompleteness

	// Collect tool summaries
	ac.RelevantToolSummaries = extractToolSummaries(bus.ToolResults)

	// Collect MCP notes
	ac.RelevantMCPNotes = extractMCPNotes(bus.MCPResponses)

	// Carry forward all prior stage reports so this agent can read
	// what earlier stages concluded instead of re-deriving it from
	// raw tool dumps. Append-only; the prompt builder formats them.
	ac.PriorReports = bus.StageReports

	// Propagate any pending retry hint from the previous self-looped
	// dispatch. Forward transitions clear this on the BusContext side,
	// so an agent only sees a hint that was meant for itself.
	ac.RetryHint = bus.TaskState.RetryHint

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

	// User preferences (e.g. default response language) flow from
	// BusContext.Preferences → AgentContext.Preferences and are rendered
	// as a system section so every agent's final answer honors them.
	// Empty preferences leave the prompt untouched, keeping the old
	// zero-config behavior when the feature is disabled.
	if len(ac.Preferences) > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   "User Preferences",
			Content: strings.Join(ac.Preferences, "\n"),
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

	// User sections — task-specific context. RetryHint comes first
	// so the LLM cannot ignore it; if the previous dispatch of this
	// same stage flagged itself as insufficient, the corrective
	// directive must override the model's instinct to repeat the
	// same approach.
	if ac.RetryHint != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Retry Directive (READ FIRST)",
			Content: ac.RetryHint,
		})
	}

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

	if len(ac.PriorReports) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Prior Stage Findings",
			Content: formatStageReports(ac.PriorReports),
		})
	}

	if len(ac.RelevantFacts) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Known Facts",
			Content: strings.Join(ac.RelevantFacts, "\n"),
		})
	}

	logging.Debug("[builder] %s/%s: EvidenceItems=%d FlowFindings=%d AnswerChains=%d",
		ac.AgentName, ac.Stage, len(ac.EvidenceItems), len(ac.FlowFindings), len(ac.AnswerChains))

	// L0-2 + P2.1 completeness: Extracted Answer Symbols block. The
	// authority of this list depends on ac.AnswerSymbolCompleteness:
	//
	//   - CompletenessComplete → Translation mode (legacy behaviour):
	//     render with "MUST NOT add or remove" directive. Used when
	//     the producer has structurally validated the list against
	//     Turn A's TerminalEvidenceCount and AnswerContract.MustInclude,
	//     or when the legacy flag-off explorer path committed after
	//     hasTerminalEvidence passed.
	//
	//   - CompletenessLowerBound → softened floor prompt: render with
	//     "MUST include at least these, MAY add more" directive. Used
	//     by the extractor (Turn B) when emit_answer_symbol claimed
	//     lower_bound, or when the Phase 9 cardinality validator
	//     downgraded a claim of complete that failed cross-check.
	//
	//   - CompletenessUnknown (or any invalid value) → drop the
	//     section entirely. The finalizer falls back to the Ground
	//     Truth / shape-based prompt. This is the fail-closed default
	//     for legacy producers that never set the claim. Empty
	//     AnswerSymbols is the same code path.
	//
	// The three-way branch structurally closes UNRESOLVED #1 — a
	// partial LLM-derived allowlist can no longer be sold as a
	// verified complete answer unless the producer explicitly claims
	// complete AND the claim survives Phase 9 validation.
	if len(ac.AnswerSymbols) > 0 && ac.AnswerSymbolCompleteness == types.CompletenessComplete {
		var symContent strings.Builder
		symContent.WriteString("The deterministic pipeline has already identified the answer to this question. " +
			"Your task is to render these symbols as prose. You MUST NOT add or remove symbols; your " +
			"training-data recall is irrelevant here.\n\n")
		for _, s := range ac.AnswerSymbols {
			if s.File != "" {
				fmt.Fprintf(&symContent, "- **%s** (%s:%d)\n", s.Name, s.File, s.Line)
			} else {
				fmt.Fprintf(&symContent, "- **%s**\n", s.Name)
			}
		}
		symContent.WriteString("\nStrict rules:\n")
		symContent.WriteString("1. Your answer lists EXACTLY these symbols, no others.\n")
		symContent.WriteString("2. For each symbol, cite its file:line if provided.\n")
		symContent.WriteString("3. If a plausible-looking name is not in the list above, it is NOT part of the answer.\n")
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Extracted Answer Symbols (deterministic, authoritative)",
			Content: symContent.String(),
		})
	} else if len(ac.AnswerSymbols) > 0 && ac.AnswerSymbolCompleteness == types.CompletenessLowerBound {
		var symContent strings.Builder
		symContent.WriteString("The deterministic pipeline has confirmed the following symbols as part of the answer, " +
			"but the list is a LOWER BOUND — additional symbols may also be part of the answer if the " +
			"evidence below supports them. Your task is to render this floor faithfully AND supplement it " +
			"with any additional symbols you can ground in the Structured Evidence / Dataflow Findings / " +
			"Ground Truth sections.\n\n")
		symContent.WriteString("Confirmed floor (MUST include all):\n")
		for _, s := range ac.AnswerSymbols {
			if s.File != "" {
				fmt.Fprintf(&symContent, "- **%s** (%s:%d)\n", s.Name, s.File, s.Line)
			} else {
				fmt.Fprintf(&symContent, "- **%s**\n", s.Name)
			}
		}
		symContent.WriteString("\nRules:\n")
		symContent.WriteString("1. Every symbol in the floor above MUST appear in your answer with its file:line citation.\n")
		symContent.WriteString("2. You MAY add additional symbols, but ONLY if they are supported by a file:line anchor " +
			"in the evidence sections below. Training-data recall alone is NOT sufficient.\n")
		symContent.WriteString("3. Any symbol you add must be cited the same way as the floor symbols.\n")
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Answer Symbols (deterministic floor, may extend with cited evidence)",
			Content: symContent.String(),
		})
	}
	// else: unknown/empty → drop the section; finalizer falls back to
	// Ground Truth + shape-based prompt downstream.

	// Answer chains get the highest priority — these are deterministic
	// resolution chains that the system has identified as directly
	// answering the user's question. The finalizer should use these
	// as the answer skeleton, not re-derive from raw evidence.
	//
	// This is the SINGLE legal flatten point for AnswerChain per the
	// architecture principle "prose only at the LLM boundary" —
	// identifyAnswerChains stays structured and the rendering happens
	// here, in the prompt assembler.
	if len(ac.AnswerChains) > 0 {
		var chainContent strings.Builder
		chainContent.WriteString("The following facts were extracted deterministically from source code and directly answer the question. " +
			"Use them as the primary basis for your answer — do NOT contradict or ignore them:\n\n")
		for _, chain := range ac.AnswerChains {
			chainContent.WriteString("- " + renderAnswerChainForPrompt(chain) + "\n")
		}
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Ground Truth (deterministic, verified from source code)",
			Content: chainContent.String(),
		})
	}

	evidence := formatEvidenceItems(ac.EvidenceItems, 18)
	findings := formatFlowFindings(ac.FlowFindings, 10)
	logging.Debug("[builder] %s/%s: evidence_section_len=%d findings_section_len=%d", ac.AgentName, ac.Stage, len(evidence), len(findings))
	if evidence != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Structured Evidence",
			Content: evidence,
		})
	}

	if findings != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Dataflow Findings",
			Content: findings,
		})
	}

	// P2.1 Phase 10 — Hypothesis Verdicts section. Rendered when the
	// extractor (Turn B) has emitted per-hypothesis verdicts via
	// emit_hypothesis_verdict and the orchestrator's drain hook has
	// applied the status mutations to AnalysisIR. The buffer is
	// read directly from Mutable (not plumbed through a new
	// BusContext field) because the verdicts are read-only after the
	// extractor exits and the narrower AgentContext already aliases
	// Mutable for this kind of late read. Legacy paths without the
	// extractor simply produce an empty buffer and the section is
	// dropped.
	if ac.Mutable != nil {
		if verdicts := ac.Mutable.EmittedHypothesisVerdicts(); len(verdicts) > 0 {
			var vc strings.Builder
			vc.WriteString("The extractor (Turn B) reached the following verdicts on the hypotheses the analyzer posed. " +
				"When writing the final answer, carry these verdicts forward: confirmed hypotheses become load-bearing " +
				"claims, rejected ones become caveats, and inconclusive ones are acknowledged as open questions. " +
				"Cite the file:line anchor from the verdict whenever you reference the conclusion.\n\n")
			for _, v := range verdicts {
				fmt.Fprintf(&vc, "- **%s** → **%s**", v.HypothesisID, v.Status)
				if rationale := strings.TrimSpace(v.Rationale); rationale != "" {
					fmt.Fprintf(&vc, ": %s", rationale)
				}
				if cite := strings.TrimSpace(v.Citation); cite != "" {
					fmt.Fprintf(&vc, " *(`%s`)*", cite)
				}
				vc.WriteString("\n")
			}
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   "Hypothesis Verdicts",
				Content: vc.String(),
			})
		}
	}

	if len(ac.RelevantFiles) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Relevant Files",
			Content: strings.Join(ac.RelevantFiles, "\n"),
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
// the parent's working state. Any tool that requires Mutable (emit_*
// channels) will fail-stop with a clear error rather than silently
// racing against parallel sub-agents over the shared state. The
// SubAgentReducer is the single point at which sub-agent results
// re-enter the parent's state.
func BuildSubAgentContext(bus *types.BusContext, req *types.SubAgentRequest) *types.AgentContext {
	ac := &types.AgentContext{
		AgentName:    types.AgentName(req.SubAgent),
		Stage:        bus.PipelineStage,
		Objective:    req.Objective,
		Constraints:  append(append([]string{}, req.Scope...), req.Constraints...),
		MissingPiece: bus.TaskState.Missing,
		RepoRoot:     bus.RepoRoot,
		Branch:       bus.Branch,
		Commit:       bus.Commit,
		WorkDir:      bus.WorkDir,
	}

	// Shared read from BusContext
	ac.RelevantFacts = extractRelevantFacts(bus.RepoFacts)
	ac.RelevantFiles = filterFilesByScope(bus.RepoFacts, req.Scope)
	ac.EvidenceItems = filterEvidenceItemsByScope(bus.EvidenceItems, req.Scope)
	ac.FlowFindings = filterFlowFindingsByEvidence(ac.EvidenceItems, bus.FlowFindings)
	ac.RelevantToolSummaries = extractToolSummaries(bus.ToolResults)
	ac.RelevantMCPNotes = extractMCPNotes(bus.MCPResponses)

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

// maxFactValueLen caps the Value field in Known Facts to prevent
// multi-KB tool outputs (full file reads, large grep results) from
// flooding downstream agent prompts. The explorer's synthesis already
// distills these into Prior Stage Findings; Known Facts only need
// enough context for the downstream agent to verify provenance.
const maxFactValueLen = 512

func extractRelevantFacts(facts []types.RepoFact) []string {
	result := make([]string, 0, len(facts))
	for _, f := range facts {
		val := f.Value
		if len(val) > maxFactValueLen {
			val = val[:maxFactValueLen] + "... [truncated]"
		}
		result = append(result, fmt.Sprintf("[%s] %s = %s (source: %s, confidence: %.2f)",
			f.Key, f.Key, val, f.Source, f.Confidence))
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

func filterEvidenceItemsByScope(items []types.EvidenceItem, scope []string) []types.EvidenceItem {
	if len(scope) == 0 {
		return append([]types.EvidenceItem(nil), items...)
	}
	var filtered []types.EvidenceItem
	for _, item := range items {
		if item.Source == "" {
			continue
		}
		for _, prefix := range scope {
			if strings.HasPrefix(item.Source, prefix) {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func filterFlowFindingsByEvidence(items []types.EvidenceItem, findings []types.FlowFindingDigest) []types.FlowFindingDigest {
	if len(items) == 0 {
		return append([]types.FlowFindingDigest(nil), findings...)
	}
	evidenceSet := make(map[string]bool, len(items))
	sourceSet := make(map[string]bool, len(items))
	for _, item := range items {
		evidenceSet[item.ID] = true
		if item.Source != "" {
			sourceSet[item.Source] = true
		}
	}
	var filtered []types.FlowFindingDigest
	for _, finding := range findings {
		matched := false
		for _, id := range finding.EvidenceIDs {
			if evidenceSet[id] {
				matched = true
				break
			}
		}
		if !matched {
			for _, source := range finding.Sources {
				if sourceSet[source] {
					matched = true
					break
				}
			}
		}
		if !matched {
			for _, sink := range finding.Sinks {
				if sourceSet[sink] {
					matched = true
					break
				}
			}
		}
		if matched {
			filtered = append(filtered, finding)
		}
	}
	return filtered
}

// evidenceUngroundedSuffix mirrors agent.ungroundedSuffix. The
// grounding pass tags Producer with this suffix when an item's
// LineStart fails every validation tier; see
// internal/agent/evidence.go. Duplicated as a literal here to avoid
// an internal/context → internal/agent import. Keep in sync.
const evidenceUngroundedSuffix = "/ungrounded"

func formatEvidenceItems(items []types.EvidenceItem, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	var b strings.Builder
	for i, item := range items {
		if i >= limit {
			break
		}
		line := item.Summary
		if line == "" {
			parts := []string{fmt.Sprintf("[%s]", item.Kind)}
			if item.Subject != "" {
				parts = append(parts, item.Subject)
			}
			if item.Predicate != "" {
				parts = append(parts, item.Predicate)
			}
			if item.Object != "" {
				parts = append(parts, item.Object)
			}
			line = strings.Join(parts, " ")
			if item.Condition != "" {
				line += " IF " + item.Condition
			}
		}
		if item.Source != "" {
			line += fmt.Sprintf(" (%s", item.Source)
			if item.LineStart > 0 {
				line += fmt.Sprintf(":%d", item.LineStart)
				if item.LineEnd > item.LineStart {
					line += fmt.Sprintf("-%d", item.LineEnd)
				}
			}
			line += ")"
		}
		if strings.HasSuffix(item.Producer, evidenceUngroundedSuffix) {
			line += " [UNGROUNDED: cite without line number]"
		}
		b.WriteString("- " + line + "\n")
	}
	if len(items) > limit {
		fmt.Fprintf(&b, "... and %d more evidence items\n", len(items)-limit)
	}
	return strings.TrimSpace(b.String())
}

func formatFlowFindings(findings []types.FlowFindingDigest, limit int) string {
	if len(findings) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(findings) {
		limit = len(findings)
	}
	var b strings.Builder
	for i, finding := range findings {
		if i >= limit {
			break
		}
		line := strings.Join(finding.Path, " -> ")
		if line == "" {
			line = strings.Join(finding.Hops, " -> ")
		}
		if line == "" {
			line = strings.Join(append(append([]string{}, finding.Sources...), finding.Sinks...), " -> ")
		}
		if line == "" {
			line = finding.ID
		}
		if len(finding.Conditions) > 0 {
			line += " IF " + strings.Join(finding.Conditions, " AND ")
		}
		if finding.UnsupportedReason != "" {
			line += " [uncertain: " + finding.UnsupportedReason + "]"
		}
		b.WriteString("- " + line + "\n")
	}
	if len(findings) > limit {
		fmt.Fprintf(&b, "... and %d more dataflow findings\n", len(findings)-limit)
	}
	return strings.TrimSpace(b.String())
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

func formatStageReports(reports []types.StageReport) string {
	var b strings.Builder
	for i, r := range reports {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "### [%s / %s]\n%s", r.Stage, r.Agent, r.Findings)
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

// renderAnswerChainForPrompt flattens a typed AnswerChain into the
// one-line display string that gets inlined into the finalizer's
// Ground Truth prompt section. This is the SINGLE legal flatten
// point for AnswerChain per the architecture principle "prose only
// at the LLM boundary" — identifyAnswerChains stays structured.
//
// Format matches the legacy identifyAnswerChains display string so
// prompt regressions are impossible:
//
//	<summary> (<source>:<line>)
//
// When Summary is empty, falls back to a [Kind] Subject Predicate
// Object synthesis. When Source is empty, the source suffix is
// dropped. Pure formatting — zero logic.
func renderAnswerChainForPrompt(c types.AnswerChain) string {
	ev := c.Item
	display := ev.Summary
	if display == "" {
		display = fmt.Sprintf("[%s] %s %s %s", ev.Kind, ev.Subject, ev.Predicate, ev.Object)
	}
	if ev.Source != "" {
		display += fmt.Sprintf(" (%s", ev.Source)
		if ev.LineStart > 0 {
			display += fmt.Sprintf(":%d", ev.LineStart)
		}
		display += ")"
	}
	return display
}
