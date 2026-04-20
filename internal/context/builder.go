package context

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildAgentContext trims a full BusContext into an Agent-scoped view.
// It selects only the facts, tools, and summaries relevant to the given agent and stage.
func BuildAgentContext(bus *types.BusContext, agentName types.AgentName, stage types.PipelineStage) *types.AgentContext {
	objective := ""
	if bus.Mutable != nil {
		objective = bus.Mutable.Objective()
	}

	ac := &types.AgentContext{
		AgentName:    agentName,
		Stage:        stage,
		Objective:    objective,
		MissingPiece: bus.TaskState.Missing,
		Constraints:  bus.Constraints,
		Preferences:  bus.Preferences,
		Language:     bus.Language,
		RepoRoot:     bus.RepoRoot,
		Branch:       bus.Branch,
		Commit:       bus.Commit,
		WorkDir:      bus.WorkDir,
		Mutable:      bus.Mutable,
		AnalysisIR:   bus.AnalysisIR,
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

	// CGEC C1: surface UnverifiedFindings written by the analyzer's
	// findings_validator into the AgentContext so BuildPromptContext
	// can render a dedicated warning section in every downstream
	// agent's prompt. Copy is defensive; the closure's internal
	// slice is not shared with the caller.
	if bus.Mutable != nil {
		ac.UnverifiedAnalyzerFindings = bus.Mutable.EvidenceClosure().UnverifiedFindings()
		// CGEC E4+E5: surface SubjectMatch cache so extractor + finalizer
		// prompt builders can render a "Subject Match Summary"
		// directing Turn B's answer extraction / citation selection at
		// the chain the framework believes best matches the question.
		ac.SubjectMatches = bus.Mutable.EvidenceClosure().AllSubjectMatches()
	}
	if bus.AnalysisIR != nil {
		ac.ExpectedAnswerSubject = bus.AnalysisIR.RequestModel.AnswerSubject
	}

	// Propagate any pending retry hint from the previous self-looped
	// dispatch. Forward transitions clear this on the BusContext side,
	// so an agent only sees a hint that was meant for itself.
	ac.RetryHint = bus.TaskState.RetryHint

	return ac
}

// thinkAloudDirective is injected into every agent's system prompt.
// It instructs the LLM to emit 1-2 sentences of reasoning before each
// batch of tool calls, so the investigation trace is human-readable
// and users can follow the agent's logic in real time (once the
// rendering layer surfaces assistant text). Token overhead is minimal
// (20-40 tokens per iteration) relative to the tool-result payloads.
const thinkAloudDirective = "You may include 1-2 sentences of reasoning as text content alongside your tool calls. " +
	"IMPORTANT: reasoning goes in the assistant message text; tool calls go through the function-calling mechanism (tool_use blocks). " +
	"NEVER write tool-call JSON in your text content — that does not execute the tool. " +
	"Do NOT produce a text-only response without actual tool calls — always pair your reasoning with real function-calling tool_use blocks. " +
	"Use the same language as the user's question. Write PLAIN TEXT only — no markdown headers, no bold, no bullets, no code blocks. Keep it to 1-2 short sentences."

// reasoningHygieneShell is the "don't miscount, use a tool" meta-rule
// for stages whose allowlist includes exec_command — today only the
// explorer. The LLM is encouraged to run a shell pipeline when it
// needs a deterministic count / sort / filter because the explorer
// can actually make that call.
const reasoningHygieneShell = "Whenever the answer is the result of a deterministic computation over data — counting, summing, sorting, finding extremes, diffing, hashing, filtering by exact criteria — run a tool that produces it directly (e.g. a shell pipeline through exec_command such as `find ... | wc -l`, `grep -c`, `sort | uniq`) and treat the tool's output as authoritative. Never derive such answers by reading a list_files / grep / read_file output yourself; language models miscount and miscompute even on short lists. The same rule applies to facts you intend to record: if a fact has a number, sort order, or set membership in it, that number/order/set must come from a tool, not from your inspection."

// reasoningHygieneNoTool is the variant for stages whose allowlist
// contains ONLY emit_* output channels — extract-skill and
// answer-document-skill today. These stages never run a counting /
// filtering tool of their own; everything quantitative they report
// must come from upstream evidence. Telling them to "run a shell
// pipeline" would waste a tool-call turn on an unavailable name.
const reasoningHygieneNoTool = "Whenever the answer is the result of a deterministic computation over data — counting, summing, sorting, finding extremes, filtering by exact criteria — the number you report must come directly from upstream evidence, not from your own re-counting of text you received. This stage consumes prior-stage tool results and does NOT have permission to run counting tools of its own, so if a specific number is not already present in the evidence above, omit the claim or mark it uncertain rather than producing one. Language models miscount and miscompute even on short lists."

// reasoningHygieneReadOnly returns the variant for stages whose
// allowlist includes read-only navigation tools (grep / repo_map /
// list_files / read_file) but NOT exec_command — today the analyzer.
// The concrete tool names inlined into the text are picked from the
// caller's own allowlist so the advice never mentions a tool the LLM
// cannot invoke.
func reasoningHygieneReadOnly(avail map[string]bool) string {
	var tools []string
	// Listed in the order the LLM is most likely to reach for them,
	// so the sentence reads naturally even when a subset is present.
	for _, name := range []string{"repo_map", "grep", "list_files", "read_file"} {
		if avail[name] {
			tools = append(tools, name)
		}
	}
	toolList := strings.Join(tools, ", ")
	return "Whenever the answer is the result of a deterministic computation over data — counting, summing, sorting, finding extremes, filtering by exact criteria — treat the exact output of the tools you DO have (" + toolList + ") as authoritative rather than re-deriving the number yourself. Language models miscount and miscompute even on short lists, so any quantitative claim must either come straight from a tool result you can cite (e.g. a grep match count, a list_files entry count) or not be made at all. If no tool in the list above can produce the exact number deterministically, mark the claim as uncertain instead of guessing."
}

// reasoningHygieneFor renders the "don't miscount, use a tool" meta-
// rule adapted to the capability profile of the skill running this
// dispatch. The underlying PRINCIPLE is invariant — language models
// miscount, so any quantitative claim must come from a tool —
// but the concrete ADVICE changes with the allowlist: telling the
// extractor to "run a shell pipeline through exec_command" is worse
// than useless when extract-skill physically blocks exec_command.
// The LLM would waste a tool-call turn on a name the schema set
// does not contain, or worse, fabricate a fact it thought the tool
// produced.
//
// Variant selection:
//
//	exec_command in allowlist                                → shell
//	grep / list_files / repo_map / read_file in allowlist    → read-only
//	neither of the above (emit_* channels only)              → no-tool
//
// See TestReasoningHygiene_* in builder_test.go for the pinned
// matrix.
func reasoningHygieneFor(sk *skill.Config) string {
	avail := skillToolSet(sk)

	switch {
	case avail["exec_command"]:
		return reasoningHygieneShell
	case avail["grep"] || avail["list_files"] || avail["repo_map"] || avail["read_file"]:
		return reasoningHygieneReadOnly(avail)
	default:
		return reasoningHygieneNoTool
	}
}

// skillToolSet materialises sk.ToolSuggestions into a map for O(1)
// capability checks inside reasoningHygieneFor. Defensive against a
// nil skill — the loader contract guarantees a non-nil config at
// every BaseAgent call site, but the builder is also exercised
// directly from tests with minimal fixtures and a nil Config is
// easy to produce there.
func skillToolSet(sk *skill.Config) map[string]bool {
	if sk == nil {
		return nil
	}
	out := make(map[string]bool, len(sk.ToolSuggestions))
	for _, name := range sk.ToolSuggestions {
		out[name] = true
	}
	return out
}

// isExtractorSkill reports whether the dispatch is Turn B (extractor).
// Used by BuildPromptContext to skip sections that carry zero signal
// for the extractor: raw tool-result dumps (Known Facts) it cannot
// act on, and the un-curated full evidence list (Structured Evidence)
// that duplicates the already-ranked Primary Evidence visible in
// Prior Stage Findings plus the Turn A transcript digest the
// extractor evaluator appends separately. Mirrors reasoningHygieneFor's
// skill-aware dispatch rather than stage-name coupling — a rename
// of the pipeline stage name would not silently break this gate.
//
// Not generalised to "any emit-only skill" (which would also match
// answer-document-skill) because the finalizer still reads Structured
// Evidence to broaden its citation pool; only the extractor has an
// alternative evidence channel via its BuildInitialInstruction digest.
func isExtractorSkill(sk *skill.Config) bool {
	return sk != nil && sk.Name == "extract-skill"
}

// canonicalSystemSectionOrder lists every system-role section title
// BuildPromptContext may emit, in the exact order the LLM sees them.
// The list is purely documentary — it pins the contract between the
// builder and the evaluators and mirrors the append() sequence in
// BuildPromptContext. When a new system section is added below, also
// add its title here so a grep reaches both sides.
var canonicalSystemSectionOrder = []string{
	"Agent Identity",
	"Reasoning Hygiene",
	"Think Aloud",
	"Constraints",
	"User Preferences",
	"Pipeline State",
	"Skill Goal",
	"Workflow",
	"Output Format",
	"Prohibitions",
}

// canonicalUserSectionOrder does the same for user-role sections.
// Note that several of these are conditional — they only appear when
// their backing AgentContext field is non-empty — but the relative
// ORDER is fixed. Evaluators (BuildInitialInstruction) must not re-emit
// any of these titles: they append a separate user message after the
// builder's output, so a duplicate title produces two visually
// identical sections and contradictory directives when the two sides
// drift.
var canonicalUserSectionOrder = []string{
	"Retry Directive (READ FIRST)",
	"User Request",
	"Prior Conversation (reference only)",
	"Prior Stage Findings", // carries the canonical Resolution Chains subsection
	"Known Facts",
	"Extracted Answer Symbols (deterministic, authoritative)",
	"Answer Symbols (deterministic floor, may extend with cited evidence)",
	"Structured Evidence",
	"Unverified Leads (not for citation)",
	"Dataflow Findings",
	"Hypothesis Verdicts",
	"Relevant Files",
}

// BuildPromptContext assembles the final prompt payload from an
// AgentContext and Skill config.
//
// Skill vs Evaluator contract (see docs/architecture.md §3.3):
//
// This function owns every STATIC prompt surface — identity, hygiene,
// constraints, preferences, the skill's declarative sections (Goal /
// Workflow / Output Format / Prohibitions), and the generic per-
// dispatch context (user request, retry directive, prior findings,
// facts, evidence, flow findings, hypothesis verdicts, relevant files).
// Internal orchestration state like MissingPiece is rendered in the
// system block as Pipeline State so it cannot be mistaken for user
// wording. The canonical titles and their relative order are
// pinned by canonicalSystemSectionOrder and canonicalUserSectionOrder
// above so additions stay grep-visible from both the builder and the
// evaluator sides.
//
// Evaluators contribute the DYNAMIC, stage-specific supplement through
// Evaluator.BuildInitialInstruction, which BaseAgent appends as an extra
// user-role message AFTER this function's output. Those supplements
// MUST NOT re-emit any title listed in the canonical section arrays —
// doing so produces two visually identical sections and lets the two
// sides silently drift. The analyzer returns an empty supplement (its
// entire per-dispatch context is already carried by this builder);
// the extractor and finalizer append genuinely new sections (Turn A
// digest, resolved target shape, cardinality baseline, prior slate)
// that this builder cannot produce generically.
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
			Content: reasoningHygieneFor(sk),
		},
	}
	if ac.ThinkAloud {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   "Think Aloud",
			Content: thinkAloudDirective,
		})
	}

	if len(ac.Constraints) > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   "Constraints",
			Content: strings.Join(ac.Constraints, "\n"),
		})
	}

	// User preferences: static entries from BusContext.Preferences
	// plus the dynamic language directive derived from BusContext.Language.
	var prefs []string
	prefs = append(prefs, ac.Preferences...)
	// Extract the current-turn question for language detection. The
	// REPL may have packed prior conversation into Objective; only
	// the current request should drive detection (otherwise a cross-
	// language history turn would flip the assertion).
	_, currentReq := types.SplitConversation(ac.Objective)
	if langPref := languageDirective(ac.Language, currentReq); langPref != "" {
		prefs = append(prefs, langPref)
	}
	if len(prefs) > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   "User Preferences",
			Content: strings.Join(prefs, "\n"),
		})
	}

	// Skill instructions — merged into system sections
	if ac.MissingPiece != types.MissingNone {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title: "Pipeline State",
			Content: fmt.Sprintf(
				"Current missing piece for scheduler state: %s.\n"+
					"Treat this as internal orchestration metadata, NOT as part of the user's request, "+
					"NOT as a code entity, and NOT as a search keyword unless the user explicitly asked about it.",
				ac.MissingPiece,
			),
		})
	}

	outputTitle := "Output Format"
	if sk.Name == "explore-skill" {
		outputTitle = "Exploration Contract"
	}

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
			Title:   outputTitle,
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
	//
	// Invariant: the hint body MUST NOT carry its own "Retry
	// Directive" H2. The section title rendered below is the only
	// heading the LLM should see; producers that populate RetryHint
	// (hint.Composer.Render, orchestrator contract-check hints) must
	// skip the heading. TestBuildPromptContext_NoDuplicateRetryDirective
	// in builder_test.go locks this so a regression can't silently
	// reintroduce the double heading.
	if ac.RetryHint != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Retry Directive (READ FIRST)",
			Content: ac.RetryHint,
		})
	}

	// Split REPL-assembled Objective into prior-conversation context and
	// the current request. The two go into separate sections so the LLM
	// cannot confuse conversation continuity with the question to answer.
	// In single-shot mode prior is empty and this degenerates to the
	// historical one-section layout.
	if ac.Objective != "" {
		priorConv, currentReq := types.SplitConversation(ac.Objective)
		if currentReq != "" {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   "User Request",
				Content: currentReq,
			})
		}
		if priorConv != "" && !ac.PriorConvHidden {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title: "Prior Conversation (reference only)",
				Content: "The text below is prior-turn conversation for continuity. " +
					"Do NOT treat it as the current question, and do NOT copy its citations or symbols into the answer without re-verifying against the current repo.\n\n" +
					priorConv,
			})
		}
	}

	if len(ac.PriorReports) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Prior Stage Findings",
			Content: formatStageReports(ac.PriorReports),
		})
	}

	// CGEC C1: Unverified Analyzer Findings. findings_validator (I1)
	// flags path / symbol tokens the analyzer referenced that the
	// repo graph could not confirm. Render them as a dedicated
	// warning section so the agent explicitly distrusts these rather
	// than mining them out of the annotated StageReport prose.
	// Consolidates what would otherwise be scattered ~~strikethrough~~
	// annotations into a single grep-able block the operator can
	// also match in trace.
	if uf := formatUnverifiedFindings(ac.UnverifiedAnalyzerFindings); uf != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Unverified Analyzer Findings",
			Content: uf,
		})
	}

	// CGEC E4 + E5: Subject Match Summary. Renders the top chains
	// rankChainsBySubject scored against the expected AnswerSubject
	// so the extractor and finalizer can bias their answer-symbol
	// extraction / leading-citation selection toward the
	// highest-scoring chain. Only rendered for extract/finalize
	// stages — the explorer already consulted the ranker directly in
	// its synthesis; surfacing it again to the explorer prompt
	// would be redundant. Renders nothing when the cache is empty
	// or the expected subject is SubjectUnknown (ranker contract
	// says match=0 for every chain in that state, so there is no
	// signal to render).
	if ac.Stage == types.StageExtract || ac.Stage == types.StageFinalize {
		if sm := formatSubjectMatchSummary(ac.SubjectMatches, ac.ExpectedAnswerSubject); sm != "" {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   "Subject Match Summary",
				Content: sm,
			})
		}
	}

	// Raw Tool Outputs from Turn A. The extractor and finalizer otherwise
	// only see structured evidence (emit_evidence items + deterministic
	// concrete_value scans) and never lay eyes on the explorer's raw
	// tool results — so a scalar produced by a one-shot command (
	// `find ... | xargs wc -l`, `grep -c`, `list_files` with 200 hits)
	// cannot travel to Turn B unless the LLM thought to call
	// emit_evidence for it. But emit_evidence's schema requires
	// source + line_start + anchor_kind, which a command-level scalar
	// doesn't have — physically impossible to emit. Surface the raw
	// summaries here so the finalizer can pull the literal directly.
	//
	// NARROW SCOPE — measurement-scalar questions only. We gate on
	// AnswerContract.RequiredAnswerShape == ShapeValue AND
	// CitationReq.Required == false; that pair is set EXCLUSIVELY by
	// the measurement-scalar carve-out in analyzer.buildAnalysisIR on
	// "how many X / 统计 / size of Y" questions. For every other
	// question shape (explanation / step_list / list_of_symbols /
	// boolean / config_value / value with file:line-citable returns)
	// the section stays hidden — otherwise the finalizer would quote
	// raw read_file dumps instead of the curated Structured Evidence
	// section, and explain-class answers would degrade.
	//
	// Explorer-only source: reading TurnAArtifacts.ToolResults scopes
	// the section to the explore stage's work and excludes the
	// analyzer's pre-scan noise.
	//
	// Stage-gated: only extractor + finalizer see this section.
	// Explorer has the raw results inline in its own ReAct transcript;
	// analyzer never reaches this block.
	if shouldRenderRawToolOutputs(ac) {
		if ta := ac.Mutable.TurnAArtifacts(); ta != nil && len(ta.ToolResults) > 0 {
			if rendered := formatRawToolOutputs(ta.ToolResults); rendered != "" {
				pc.UserSections = append(pc.UserSections, types.PromptSection{
					Title:   "Raw Tool Outputs from the Investigation",
					Content: rendered,
				})
			}
		}
	}

	// Extract-skill trim (Turn B noise reduction). See isExtractorSkill
	// for the rationale: the extractor has no investigation tools,
	// raw tool-result dumps are inert, and the full evidence list
	// duplicates data the extractor already sees through the Prior
	// Stage Findings Primary Evidence subsection plus its
	// BuildInitialInstruction Turn A digest. In the 2026-04-17 audit
	// the two skipped sections accounted for ~70% of the extractor's
	// prompt, diluting the load-bearing signal.
	skipForExtractor := isExtractorSkill(sk)

	if !skipForExtractor && len(ac.RelevantFacts) > 0 {
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
			"Prior Stage Findings' Resolution Chains sections.\n\n")
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

	// AnswerChains are no longer rendered as a separate "Ground Truth"
	// section here — the explorer's stage report (see stage_report_render.go)
	// already carries them under Prior Stage Findings' Resolution
	// Chains subsection with the same "do NOT contradict, terminal is
	// the answer" directive text. Rendering the same chain list twice
	// with different headers was pure signal dilution; the duplicate
	// was ~5% of the extractor prompt and ~1-2% of the finalizer
	// prompt. Consolidated 2026-04-17.

	// Session-8 rule: at Turn B (extract / finalize) hide LineStart of
	// non-Tier-1-grounded items so downstream LLMs cannot cite a
	// recovered anchor that the finalizer's stricter citation
	// grounder will later reject. Turn A (explore) still sees its
	// own recovered lines so iterative investigation can reference
	// earlier-window anchors.
	strictEvidenceLoc := ac.Stage == types.StageExtract || ac.Stage == types.StageFinalize
	evidence := formatEvidenceItems(ac.EvidenceItems, 18, strictEvidenceLoc)
	findings := formatFlowFindings(ac.FlowFindings, 10)
	logging.Debug("[builder] %s/%s: evidence_section_len=%d findings_section_len=%d", ac.AgentName, ac.Stage, len(evidence), len(findings))
	// Structured Evidence carries the full top-18 evidence dump.
	// Skipped for the extract-skill: that dispatch already sees the
	// top-12 via Prior Stage Findings' Primary Evidence subsection
	// and the curated view via the Turn A digest its evaluator
	// appends. Other skills (finalizer especially) need the full
	// list for citation coverage.
	if !skipForExtractor && evidence != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Structured Evidence",
			Content: evidence,
		})
	}

	// Unverified Leads — items the explorer emit_evidence-grounded as
	// ungrounded. Rendered in every skill by default (design pick #4
	// of the 2026-04-17 redesign) so the finalizer sees the leads but
	// is explicitly told not to cite them. The extractor also benefits
	// from the visibility: it can mention a lead in reasoning text
	// without pulling it into emit_answer_symbol.
	if leads := formatUnverifiedLeads(ac.EvidenceItems, 12, strictEvidenceLoc); leads != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   "Unverified Leads (not for citation)",
			Content: leads,
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
	// Propagate the graph handle — Mutable is intentionally not
	// aliased for sub-agents (they must not mutate parent state), but
	// the repomap graph is a read-only pointer and reusing it spares
	// every sub-agent a BuildOrLoadGraph round-trip.
	if bus.Mutable != nil {
		ac.SearchGraph = bus.Mutable.SearchGraph()
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

// maxFactValueLen caps the Value field in Known Facts to prevent
// multi-KB tool outputs (full file reads, large grep results) from
// flooding downstream agent prompts. The explorer's synthesis already
// distills these into Prior Stage Findings; Known Facts only need
// enough context for the downstream agent to verify provenance.
const maxFactValueLen = 512

// trimKnownFactValue strips tool-specific noise from a fact value
// before it is rendered into the Known Facts section. Right now
// grep is the sole special case: its Summary is a match-count
// header followed by a full file-path list. The paths already live
// in "Relevant Files" / the stage report, so quoting them again
// inside Known Facts is pure duplication (trace 1776450670620195562
// showed the same 20-file list being repeated across 5 grep calls).
// The helper keeps just the header line (e.g. "[grep: 94 matching
// files]") and discards the body. Other tool kinds fall through
// to the generic length-cap path below.
func trimKnownFactValue(key, val string) string {
	switch key {
	case "grep":
		// First line carries "[grep: N matching files]" — the only
		// useful summary. Drop the path body (it leaks into the
		// prompt as pure duplicate of Relevant Files).
		if nl := strings.IndexByte(val, '\n'); nl > 0 {
			return val[:nl]
		}
		return val
	}
	return val
}

func extractRelevantFacts(facts []types.RepoFact) []string {
	result := make([]string, 0, len(facts))
	for _, f := range facts {
		val := trimKnownFactValue(f.Key, f.Value)
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

// formatEvidenceItems renders evidence for Primary Evidence /
// Structured Evidence sections. Ungrounded items (GroundingStatus ==
// Ungrounded) are NOT rendered here — they flow through
// formatUnverifiedLeads into a dedicated "Unverified Leads" section
// so the finalizer cannot accidentally cite them.
//
// strictLocation toggles session-8 "no non-Tier-1 lines reach Turn B"
// behaviour: when true, Recovered items show `(file)` without a line
// and get a `[recovered — line not read; re-run read_file before
// citing]` tag so the LLM can't silently pick up an anchor that the
// finalizer-time grounder will later reject. When false, the
// historical "(file:line) [recovered]" format ships so the explorer
// itself (self-referencing earlier windows) still sees its own
// recovered lines.
func formatEvidenceItems(items []types.EvidenceItem, limit int, strictLocation bool) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	var b strings.Builder
	written := 0
	for _, item := range items {
		if item.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		if written >= limit {
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
		if loc := item.DisplayLocation(strictLocation); loc != "" {
			line += " (" + loc + ")"
		}
		if item.GroundingStatus == types.GroundingRecovered {
			if strictLocation {
				line += " [recovered — line not read; re-run read_file before citing]"
			} else {
				line += " [recovered]"
			}
		}
		b.WriteString("- " + line + "\n")
		written++
	}
	if len(items) > written {
		// Note the distinction: "items" here has already excluded the
		// ungrounded entries for written purposes, but the tally still
		// shows the full count of grounded/recovered items beyond limit.
		over := countGroundedOrRecovered(items) - written
		if over > 0 {
			fmt.Fprintf(&b, "... and %d more evidence items\n", over)
		}
	}
	return strings.TrimSpace(b.String())
}

// formatUnverifiedLeads renders items whose GroundingStatus is
// Ungrounded. These are LLM claims the grounder could not validate:
// treated as discussion hints, never emitted as citations.
//
// strictLocation gates the LineStart: when true (Turn B / finalize)
// the LineStart is stripped entirely so downstream LLMs cannot even
// SEE the fabricated number — DisplayLocation returns just `file`.
// When false (Turn A self-reference) the historical "file:line"
// format is preserved so the explorer can cross-reference its own
// speculative claims across iterations.
func formatUnverifiedLeads(items []types.EvidenceItem, limit int, strictLocation bool) string {
	leads := make([]types.EvidenceItem, 0, len(items))
	for _, it := range items {
		if it.GroundingStatus == types.GroundingUngrounded {
			leads = append(leads, it)
		}
	}
	if len(leads) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(leads) {
		limit = len(leads)
	}
	var b strings.Builder
	b.WriteString("These items were proposed by the explorer but the grounder could not validate the file:line citation (anchor_symbol not found at the cited line, or the file was never read). Treat them as discussion hints — do NOT emit them in the answer's citations[] field.\n\n")
	for i, item := range leads {
		if i >= limit {
			break
		}
		line := item.Summary
		if line == "" {
			parts := []string{fmt.Sprintf("[%s]", item.Kind)}
			if item.Subject != "" {
				parts = append(parts, item.Subject)
			}
			line = strings.Join(parts, " ")
		}
		if loc := item.DisplayLocation(strictLocation); loc != "" {
			line += " (" + loc + ")"
		}
		if note := strings.TrimSpace(item.GroundingNote); note != "" {
			line += " — " + note
		}
		b.WriteString("- " + line + "\n")
	}
	if len(leads) > limit {
		fmt.Fprintf(&b, "... and %d more unverified leads\n", len(leads)-limit)
	}
	return strings.TrimSpace(b.String())
}

func countGroundedOrRecovered(items []types.EvidenceItem) int {
	n := 0
	for _, it := range items {
		if it.GroundingStatus != types.GroundingUngrounded {
			n++
		}
	}
	return n
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

// shouldRenderRawToolOutputs gates the Raw Tool Outputs section to
// measurement-scalar questions only. The discriminator mirrors the
// upstream carve-out in analyzer.buildAnalysisIR: ShapeValue +
// CitationReq.Required == false is the exact pair that carve-out
// sets, and no other code path sets it. Using the IR state as the
// gate keeps the rule 1:1 with its producer — there is no prefix
// regex duplication and no risk of drift.
//
// All three conditions must hold:
//
//   - Stage is extract OR finalize. Explorer has raw results in its
//     own transcript; analyzer never reaches this section.
//   - Mutable + AnalysisIR + AnswerContract are populated. Legacy
//     test paths (nil Mutable, nil IR) fall through cleanly.
//   - AnswerContract.RequiredAnswerShape == ShapeValue AND
//     CitationReq.Required == false. Other shapes (explanation,
//     step_list, list_of_symbols, boolean, config_value, value-with-
//     citation) are untouched so the finalizer's citation-grounded
//     answer construction for deep-investigation questions stays
//     exactly as it was.
func shouldRenderRawToolOutputs(ac *types.AgentContext) bool {
	if ac.Stage != types.StageExtract && ac.Stage != types.StageFinalize {
		return false
	}
	if ac.Mutable == nil {
		return false
	}
	if ac.AnalysisIR == nil {
		return false
	}
	c := ac.AnalysisIR.AnswerContract
	if c.RequiredAnswerShape != types.ShapeValue {
		return false
	}
	if c.CitationReq.Required {
		return false
	}
	return true
}

// Raw Tool Outputs rendering — size knobs.
//
// The per-tool head+tail pair is deliberately asymmetric: the head is
// large enough to show the start of a typical exec_command listing
// (20+ lines), and the tail is **always** preserved (never truncated
// away) because shell tools that summarise put the summary at the
// END — `wc -l`'s `NNNN total`, `grep -c`'s count, `ls -l | wc -l`,
// `find ... | wc -l`. Clipping the tail was exactly how the 73396
// regression lost its answer. Absolute total cap bounds the prompt
// inflation across all tool calls in one dispatch.
const (
	rawToolOutputPerCallHeadBytes = 800
	rawToolOutputPerCallTailBytes = 400
	rawToolOutputTotalCapBytes    = 4000
)

// rawToolOutputSkipTools is the set of tool names whose results are
// ALREADY rendered in other sections and would just duplicate signal
// if surfaced in Raw Tool Outputs. emit_* tools are explicit structured
// channels (evidence, answer symbols, hypothesis verdicts, answer
// document); their raw Summary is a confirmation echo, not new
// information. repo_map's raw output is a structural digest already
// covered by the Investigation Summary.
var rawToolOutputSkipTools = map[string]bool{
	"emit_evidence":               true,
	"emit_investigation_complete": true,
	"emit_answer_symbol":          true,
	"emit_hypothesis_verdict":     true,
	"emit_answer_document":        true,
	"emit_analysis":               true,
	"propose_sub_agents":          true,
	"repo_map":                    true,
}

// rawToolOutputPreamble instructs the finalizer on how to consume the
// tool outputs: they are scalar-bearing references, NOT entries in the
// emit_answer_document.citations[] pool. Without this line the LLM
// tries to cite "tool:exec_command" as a file — rejected by the
// path validator — then gets stuck in a retry loop that never produces
// a valid document. For measurement-scalar questions (shape=value +
// no citation floor) citation_ref=-1 on the value is the correct
// answer.
const rawToolOutputPreamble = "These are the raw outputs of commands the explorer ran during the investigation. " +
	"Use them as the source of TRUTH for scalar answers (counts, totals, sizes, version numbers). " +
	"These tool outputs are NOT repo files — they MUST NOT appear in citations[]. " +
	"For a measurement-scalar answer (shape=value, no citation floor) emit value{literal, citation_ref:-1} " +
	"with the scalar taken directly from the tool output tail; -1 is the correct choice because the " +
	"answer is a command-level measurement with no file:line anchor.\n\n"

// formatRawToolOutputs renders the successful subset of Turn A's tool
// results as a bulleted section. Each call shows head + (mid-trim
// marker) + tail so a long `wc -l` listing still ends with its
// `NNNN total` line. Stops appending once the cumulative rendered
// size crosses rawToolOutputTotalCapBytes; a trailing "... (N more
// tool calls omitted)" line flags the cap so the LLM knows the
// section is not exhaustive.
func formatRawToolOutputs(results []types.ToolResult) string {
	var b strings.Builder
	rendered := 0
	for i, r := range results {
		if !r.Success {
			continue
		}
		if rawToolOutputSkipTools[r.ToolName] {
			continue
		}
		summary := strings.TrimSpace(r.Summary)
		if summary == "" {
			continue
		}
		chunk := formatRawToolSummary(r.ToolName, summary)
		if b.Len()+len(chunk) > rawToolOutputTotalCapBytes {
			remaining := 0
			for j := i; j < len(results); j++ {
				if results[j].Success && !rawToolOutputSkipTools[results[j].ToolName] {
					remaining++
				}
			}
			if remaining > 0 {
				fmt.Fprintf(&b, "- ... (%d more tool call(s) omitted to bound prompt size)\n", remaining)
			}
			break
		}
		b.WriteString(chunk)
		rendered++
	}
	if rendered == 0 {
		return ""
	}
	return rawToolOutputPreamble + strings.TrimRight(b.String(), "\n")
}

// formatRawToolSummary renders one ToolResult.Summary with a bounded
// head + tail preview. The tail is always preserved because shell
// tools put their summarising scalar at the END of output.
func formatRawToolSummary(toolName, summary string) string {
	head := rawToolOutputPerCallHeadBytes
	tail := rawToolOutputPerCallTailBytes
	var body string
	if len(summary) <= head+tail {
		body = summary
	} else {
		body = summary[:head] + "\n...[trimmed " + fmt.Sprint(len(summary)-head-tail) + " bytes]...\n" + summary[len(summary)-tail:]
	}
	return fmt.Sprintf("- **%s** (%d bytes):\n```\n%s\n```\n", toolName, len(summary), body)
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

// subjectMatchRenderCap bounds the number of top chains rendered in
// the Subject Match Summary section.
const subjectMatchRenderCap = 5

// subjectMatchFloor is the minimum score a chain needs for the
// helper to mention it. Chains below the floor are the G5-style
// "off-target" signal — rendering them as candidates would mislead
// the downstream LLM. If ZERO chains clear the floor we emit a
// warning body instead of a candidate list.
const subjectMatchFloor = 0.2

// formatSubjectMatchSummary renders "## Subject Match Summary" body
// for extractor / finalizer prompts. Sort chains by score descending,
// keep the top-K above subjectMatchFloor, and precede the list with
// the expected subject kind so the LLM knows what "match" means.
// Returns "" when nothing meaningful to render.
func formatSubjectMatchSummary(matches map[string]float64, expected types.AnswerSubject) string {
	if len(matches) == 0 || expected.Kind == types.SubjectUnknown {
		return ""
	}
	type scored struct {
		chain string
		score float64
	}
	entries := make([]scored, 0, len(matches))
	for c, s := range matches {
		entries = append(entries, scored{chain: c, score: s})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})
	var b strings.Builder
	fmt.Fprintf(&b, "Expected answer subject: `%s` (confidence %.2f)\n", expected.Kind, expected.Confidence)
	var kept int
	for _, e := range entries {
		if e.score < subjectMatchFloor {
			break
		}
		if kept == 0 {
			b.WriteString("Chain candidates (sorted by subject match, highest first):\n")
		}
		fmt.Fprintf(&b, "  - score=%.2f  %s\n", e.score, e.chain)
		kept++
		if kept >= subjectMatchRenderCap {
			break
		}
	}
	if kept == 0 {
		fmt.Fprintf(&b, "No chain scored above %.2f for the expected subject. Treat the chain producer's output with SKEPTICISM — the explorer's chains may be about the wrong kind of token.\n", subjectMatchFloor)
	} else {
		b.WriteString("Prefer the top-scored chain when selecting the primary answer symbol / leading citation.")
	}
	return b.String()
}

// unverifiedFindingsRenderCap bounds the number of entries rendered
// in the Unverified Analyzer Findings section so a pathological
// analyzer output cannot flood downstream prompts. When the cap is
// hit the trailing "... and N more" line keeps the information loss
// visible. C4 caps the render, not the underlying slice, so the
// preCompleteContractCheck (C2) still sees every finding.
const unverifiedFindingsRenderCap = 12

// formatUnverifiedFindings produces the markdown body for the
// "Unverified Analyzer Findings" section. Dedupes by Token+Kind
// (AppendUnverifiedFinding already dedupes but belt-and-suspenders
// for older closures), caps at unverifiedFindingsRenderCap, and
// returns "" when nothing to render so the caller can suppress the
// section title entirely on a clean analyzer run.
//
// Format:
//
//	The analyzer referenced these items but findings_validator could not
//	confirm them against the repo graph. Treat them as UNRELIABLE — do
//	not cite them, do not assume their contents, grep the repo to
//	confirm or disprove.
//	  - `path` internal/agent/foo.go — file does not exist in repo
//	  - `symbol` BarHandler — symbol not found in graph
//	  ... and 3 more.
func formatUnverifiedFindings(finds []types.UnverifiedFinding) string {
	if len(finds) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(finds))
	var uniq []types.UnverifiedFinding
	for _, f := range finds {
		key := f.Kind + "\x00" + f.Token
		if seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, f)
	}
	var b strings.Builder
	b.WriteString("The analyzer referenced these items but findings_validator could not confirm them against the repo graph. Treat them as UNRELIABLE — do not cite them, do not assume their contents, grep the repo to confirm or disprove.\n")
	max := unverifiedFindingsRenderCap
	if len(uniq) < max {
		max = len(uniq)
	}
	for i := 0; i < max; i++ {
		f := uniq[i]
		label := f.Kind
		if label == "" {
			label = "token"
		}
		fmt.Fprintf(&b, "  - `%s` %s — %s\n", label, f.Token, f.Reason)
	}
	if len(uniq) > max {
		fmt.Fprintf(&b, "  ... and %d more.\n", len(uniq)-max)
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

// languageDirective maps a language code to a concise prompt
// directive. Returns "" when the feature is disabled.
//
// Priority model (config wins):
//   - lang == "" / "off" / "none"  → no directive, LLM chooses freely
//   - lang == "auto"               → follow the user's question language
//     (hard assertion prepended when detectable;
//     base "same-language" rule as fallback)
//   - any other value              → HARD imperative locked to that language,
//     regardless of the question language.
//     This is the config-priority path: a
//     persistent "lang: zh" config forces
//     Chinese output even when the user asks
//     in English.
//
// question is the user's current request (after conversation-prefix
// strip). Only consulted on the `auto` path; ignored when the config
// names a concrete language.
func languageDirective(lang, question string) string {
	switch lang {
	case "", "off", "none":
		return ""
	case "auto", "follow":
		base := languageDirectiveAutoBase()
		if assertion := detectedLanguageAssertion(question); assertion != "" {
			return assertion + "\n\n" + base
		}
		return base
	default:
		return lockedLanguageDirective(lang)
	}
}

// lockedLanguageDirective is the config-priority directive: a hard
// imperative to write in `lang` regardless of what language the user
// asked in. Used when `codrax.yaml` names a concrete language (zh /
// en / fr / ...). The directive never consults question language.
func lockedLanguageDirective(lang string) string {
	switch lang {
	case "zh", "zh-CN", "zh-cn", "cn", "chinese":
		return "You MUST write every natural-language response in Simplified Chinese (简体中文). This is a hard requirement set by the project configuration — do not switch to English prose even if the user writes the question in English. Summaries, step descriptions, rationales, captions, and any other natural-language content are all in Chinese. Always keep code identifiers, file paths, type names, and function names in their original form."
	case "en", "en-US", "english":
		return "You MUST write every natural-language response in English. This is a hard requirement set by the project configuration — do not switch to another language even if the user writes the question in a different language. Always keep code identifiers, file paths, type names, and function names in their original form."
	default:
		return fmt.Sprintf("You MUST write every natural-language response in %s. This is a hard requirement set by the project configuration — do not switch to another language even if the user writes the question in a different language. Always keep code identifiers, file paths, type names, and function names in their original form.", lang)
	}
}

// languageDirectiveAutoBase is the conditional "same-language" rule
// used on the `auto` path when the question script is not
// confidently detectable (too short, pure code). detect-from-question
// assertion, when it fires, is prepended to this by languageDirective.
func languageDirectiveAutoBase() string {
	return "Reply in the same natural language as the user's question. Ignore code identifiers, file paths, and technical terms (e.g. `explorer`, `subagent`, `internal/agent/foo.go`) when judging the question's language — a sentence whose prose is Chinese but which mentions English symbols is still a Chinese question. When the question is ambiguous or contains no natural-language prose, default to Simplified Chinese (简体中文). Always keep code identifiers, file paths, and technical terms in their original form in your reply."
}

// detectedLanguageAssertion returns a hard-assertion directive when
// the question's dominant natural-language script is detectable, or
// "" when the question is empty / purely code / ambiguous. Script
// detection is deliberately simple: count Han/Hiragana/Katakana/
// Hangul codepoints vs Latin letters. CJK dominance → Chinese/
// Japanese/Korean assertion; Latin dominance → English. Mixed or
// sparse prose falls through to the conditional base directive.
func detectedLanguageAssertion(question string) string {
	cjk, latin := 0, 0
	for _, r := range question {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF, // Han
			r >= 0x3040 && r <= 0x309F, // Hiragana
			r >= 0x30A0 && r <= 0x30FF, // Katakana
			r >= 0xAC00 && r <= 0xD7AF: // Hangul syllables
			cjk++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latin++
		}
	}
	// CJK assertion: any non-trivial Han/kana/hangul presence wins.
	// A single CJK character in a question like "explorer是怎么调用
	// subagent的？" (6 CJK) reliably signals a Chinese question even
	// when technical symbols push latin count higher.
	if cjk >= 3 {
		return "The user's question is written in Chinese. You MUST write your answer in Simplified Chinese (简体中文). This is a hard requirement — do not switch to English prose for the summary, step descriptions, rationales, captions, or any other natural-language content. Keep code identifiers, file paths, type names, and function names in their original form."
	}
	// Latin assertion: require enough letters to avoid flagging a
	// single-word query. 20 letters is roughly a short English
	// sentence.
	if latin >= 20 && cjk == 0 {
		return "The user's question is written in English. You MUST write your answer in English. Keep code identifiers, file paths, type names, and function names in their original form."
	}
	return ""
}
