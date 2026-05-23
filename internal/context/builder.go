package context

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/config"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/textfmt"
	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildAgentContext trims a full BusContext into an Agent-scoped view.
// It selects only the facts, tools, and summaries relevant to the given agent and stage.
func BuildAgentContext(bus *types.BusContext, agentName types.AgentName, stage types.PipelineStage) *types.AgentContext {
	objective := ""
	if bus.Mutable != nil {
		objective = bus.Mutable.Objective()
	}
	presentationDirective := strings.TrimSpace(bus.PresentationDirective)

	ac := &types.AgentContext{
		AgentName:             agentName,
		Stage:                 stage,
		TraceID:               bus.TraceID,
		ExploreDispatchKey:    bus.ExploreDispatchKey,
		ExploreDispatchKind:   bus.ExploreDispatchKind,
		Objective:             objective,
		PresentationDirective: presentationDirective,
		MissingPiece:          bus.TaskState.Missing,
		Constraints:           bus.Constraints,
		Preferences:           bus.Preferences,
		Language:              bus.Language,
		RepoRoot:              bus.RepoRoot,
		Branch:                bus.Branch,
		Commit:                bus.Commit,
		WorkDir:               bus.WorkDir,
		MainRepoRoot:          bus.MainRepoRoot,
		Mutable:               bus.Mutable,
		// Multi-repo mirrors. Phase 4.1 introduced these on
		// BusContext + AgentContext; the builder copies them across
		// so agent-scoped tools and the agent prompt builder can
		// surface multi-repo state without reaching back through the
		// full BusContext. Defensive copy for the slices to keep
		// AgentContext mutation isolated from BusContext.
		MultiGraph:                    bus.MultiGraph,
		SubRepos:                      append([]types.SubRepoSnapshot(nil), bus.SubRepos...),
		ActiveSubRepo:                 bus.ActiveSubRepo,
		PendingSubRepos:               append([]string(nil), bus.PendingSubRepos...),
		MultiRepoInactivePreviewCount: bus.MultiRepoInactivePreviewCount,

		// TypedDenials shares the SAME pointer (not a copy) — when
		// a tool call mid-dispatch stamps a new denial, subsequent
		// calls in the loop see it. Kept on bus's pointer so the
		// orchestrator's owning channel is the single source.
		TypedDenials:    &bus.TypedDenials,
		AnalysisIR:      bus.AnalysisIR,
		AttachedLog:     bus.AttachedLog,
		AttachedHitrace: bus.AttachedHitrace,
		// Mirror BusContext.Mode onto the agent view so the analyzer
		// can route mode-conditional behaviour (read-mode quality
		// gate vs write-mode classifier-only) without reaching back
		// to BusContext. Zero-value preserves pre-write-mode
		// behaviour byte-identically.
		Mode: bus.Mode,
		// Read-only handles + value-types that downstream tools need.
		// Pre-this-fix these were stripped at the agent boundary; the
		// recall_memory tool returned "unavailable" and the env_recommend
		// integration was a no-op even in REPL runs that had the
		// orchestrator wire them. They are read-only by construction
		// (interface / pointer-to-immutable / value type), so adding
		// them here does NOT widen the agent's mutation surface — the
		// "agents are a narrow read-only view" contract is preserved.
		Memory:               bus.Memory,
		EnvFacts:             bus.EnvFacts,
		EnvRecommendSettings: bus.EnvRecommendSettings,
		// Phase 2 cancel ctx: Adapter.Chat + ctx-aware tools read
		// AgentContext.Context() to plumb cancellation HTTP-deep.
		Ctx: bus.Ctx,
	}
	// Mirror the validated log-triage bundle onto the AgentContext so
	// analyzer / explorer / extractor / finalizer consumers can read
	// it directly without reaching through ctx.Mutable. Nil-safe: when
	// the log_triage pre-stage did not run (no AttachedLog) or
	// degraded, the field stays nil and every consumer's nil-check is
	// a no-op.
	if bus.Mutable != nil {
		ac.LogTriage = bus.Mutable.LogTriage()
		ac.PerfTrace = bus.Mutable.PerfTrace()
	}

	// Read-mode stage artifact propagation. The explore→extract→
	// finalize chain writes RepoFacts, EvidenceItems, AnswerChains,
	// AnswerSymbols, FlowFindings, StageReports, UnverifiedFindings,
	// SubjectMatches; downstream read-mode agents render them as
	// dedicated prompt sections.
	//
	// Write-mode stages (plan / apply / verify) are hard-excluded
	// from this block — the single chokepoint enforcing the
	// session-35 red line "no read-mode stage artifacts in
	// planner/coder/verifier prompts". Prior attempt (B0 — session
	// 33) left this block ungated; the analyzer's StageReport.Findings
	// (raw LLM lastContent, including <think> preamble like "Let me
	// emit the analysis…") leaked into the planner's Prior Stage
	// Findings section, causing the planner LLM to pattern-match on
	// the analyzer's emit verb and skip emit_change_plan entirely.
	//
	// StageAnalyze is NOT a write stage even when it runs inside
	// the write pipeline as a classifier — see PipelineStage.IsWrite
	// docblock. Analyzer still needs RepoFacts / prior artifacts for
	// its pre-scan rounds, so this gate correctly allows them through.
	//
	// Note on the structured AnalysisIR exception: ac.AnalysisIR is
	// set unconditionally above (line ~37). It is NOT a "stage
	// artifact" — it is the analyzer's typed structured output
	// (entities, sub-topics, required files), which by construction
	// contains no raw LLM prose and therefore cannot reintroduce the
	// session-35 leak. BuildPromptContext renders a small, write-safe
	// subset of it ("Analyzer Pre-scan Findings") for StagePlan only
	// to give the planner a warm start instead of a 5+ iter cold-
	// discovery prelude. See formatAnalyzerPrescanForPlan.
	if !ac.Stage.IsWrite() {
		// Collect relevant facts
		ac.RelevantFacts = extractRelevantFacts(bus.RepoFacts)

		// Collect relevant files from facts
		ac.RelevantFiles = extractRelevantFiles(bus.RepoFacts)
		ac.EvidenceItems = append([]types.EvidenceItem(nil), bus.EvidenceItems...)
		ac.FlowFindings = append([]types.FlowFindingDigest(nil), bus.FlowFindings...)
		ac.AnswerChains = append([]types.AnswerChain(nil), bus.AnswerChains...)
		ac.AnswerSymbols = append([]types.AnswerSymbol(nil), bus.AnswerSymbols...)
		ac.AnswerSymbolCompleteness = bus.AnswerSymbolCompleteness

		// Typed relation hint probe (P3 #6 follow-up, 2026-05-03):
		// system-derived structural-relation candidates injected into
		// the prompt's Structured Evidence section as input HINTING.
		// LLM remains the sole author of doc.Symbols / Summary etc;
		// per the feedback_no_system_backfill_to_user_panel red line
		// the hint never reaches user-facing fields.
		if bus.AnalysisIR != nil {
			graph := analyzerGraphFromBus(bus)
			if hints := ProbeTypedRelations(graph, &bus.AnalysisIR.RequestModel); len(hints) > 0 {
				ac.TypedRelationHints = hints
			}
		}

		// Collect tool summaries
		ac.RelevantToolSummaries = extractToolSummaries(bus.ToolResults)

		// Collect MCP notes
		ac.RelevantMCPNotes = extractMCPNotes(bus.MCPResponses)
		ac.MCPResponses = append([]types.MCPResponse(nil), bus.MCPResponses...)

		// Carry forward all prior stage reports so this agent can read
		// what earlier stages concluded instead of re-deriving it from
		// raw tool dumps. Append-only; the prompt builder formats them.
		ac.PriorReports = stageReportsForAgent(bus.StageReports, ac)

		// CGEC C1: surface UnverifiedFindings written by the analyzer's
		// findings_validator into the AgentContext so BuildPromptContext
		// can render a dedicated warning section in every downstream
		// agent's prompt. Copy is defensive; the closure's internal
		// slice is not shared with the caller.
		if bus.Mutable != nil {
			ac.UnverifiedAnalyzerFindings = mergeUnverifiedFindings(
				bus.Mutable.EvidenceClosure().UnverifiedFindings(),
				extractUnverifiedFindingsFromStageReports(bus.StageReports),
			)
			// CGEC E4+E5: surface SubjectMatch cache so extractor + finalizer
			// prompt builders can render a "Subject Match Summary"
			// directing Turn B's answer extraction / citation selection at
			// the chain the framework believes best matches the question.
			ac.SubjectMatches = bus.Mutable.EvidenceClosure().AllSubjectMatches()
		}
		if bus.AnalysisIR != nil {
			ac.ExpectedAnswerSubject = bus.AnalysisIR.RequestModel.AnswerSubject
		}
		if ac.AgentName == types.AgentFinalizer && finalizerUsesTypedAnswerSupport(ac) {
			ac.RelevantFacts, ac.RelevantFiles = typedSupportFinalizerRepoFacts(bus.RepoFacts, ac, ac.RelevantFacts, ac.RelevantFiles)
		}
	}

	// Propagate pending retry hints only to the stage that owns them.
	// Analyzer has a separate consume-once AnalyzerRetryHint lane; letting
	// the generic TaskState.RetryHint flow into analyzer retries can leak a
	// downstream DAG window directive back into request classification.
	ac.RetryHint = retryHintForAgentContext(bus, stage)

	return ac
}

func retryHintForAgentContext(bus *types.BusContext, stage types.PipelineStage) string {
	if bus == nil {
		return ""
	}
	switch stage {
	case types.StageAnalyze, types.StageWriteAnalyze:
		return ""
	}
	if bus.TaskState.Stage != "" && bus.TaskState.Stage != stage {
		return ""
	}
	return bus.TaskState.RetryHint
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

// canonicalSystemSectionOrder and canonicalUserSectionOrder live in
// section_titles.go alongside the section-title constants. Always
// reference those constants — never write a section title literal.

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
			Title:   SectionAgentIdentity,
			Content: agentIdentityPrompt(ac.AgentName, ac.Stage),
		},
		{
			Title:   SectionReasoningHygiene,
			Content: reasoningHygieneFor(sk),
		},
	}
	if ac.ThinkAloud {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   SectionThinkAloud,
			Content: thinkAloudDirective,
		})
	}

	if len(ac.Constraints) > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   SectionConstraints,
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
			Title:   SectionUserPreferences,
			Content: strings.Join(prefs, "\n"),
		})
	}

	// Skill instructions — merged into system sections
	if ac.MissingPiece != types.MissingNone {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title: SectionPipelineState,
			Content: fmt.Sprintf(
				"Current missing piece for scheduler state: %s.\n"+
					"Treat this as internal orchestration metadata, NOT as part of the user's request, "+
					"NOT as a code entity, and NOT as a search keyword unless the user explicitly asked about it.",
				ac.MissingPiece,
			),
		})
	}

	outputTitle := SectionOutputFormat
	if sk.Name == "explore-skill" {
		outputTitle = SectionExplorationContract
	}

	// P5-B step 3 (2026-05-10) — tier-aware skill rendering. When
	// the skill declares any WorkflowTierB / ProhibitionsTierB items,
	// build the AppliesToContext from the dispatch's runtime state
	// and concat the matching Tier B bodies onto the Tier A list
	// before formatting. Skills without Tier B fields render
	// byte-identical to pre-P5 (the helpers handle the empty case
	// as no-op).
	workflowList := skillTierAwareWorkflow(ac, sk)
	prohibitionList := skillTierAwareProhibitions(ac, sk)

	pc.SystemSections = append(pc.SystemSections,
		types.PromptSection{
			Title:   SectionSkillGoal,
			Content: sk.Goal,
		},
		types.PromptSection{
			Title:   SectionWorkflow,
			Content: formatNumberedList(workflowList),
		},
		types.PromptSection{
			Title:   outputTitle,
			Content: sk.OutputFormat,
		},
	)

	if len(prohibitionList) > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{
			Title:   SectionProhibitions,
			Content: formatBulletList(prohibitionList),
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
			Title:   SectionRetryDirective,
			Content: ac.RetryHint,
		})
	}

	// Phase 2 Tier 2 ERM completeness hint surfacing. The orchestrator
	// runs CompletenessValidators at finalize entry and stamps an
	// LLM-natural FixHint into Mutable when a structural-coverage gap
	// is detected (count question without exec_command output, call-
	// chain answer with too few function mentions, ...). The hint is
	// surfaced only at finalize (the stage producing the user-visible
	// answer) so the LLM can route around the gap when composing the
	// answer body. R6-clean — the FixHint is constructed by the
	// validator with no internal pipeline terms; see
	// internal/agent/erm_completeness.go for the LLM-natural prose.
	if ac.Stage == types.StageFinalize && ac.Mutable != nil {
		if hint := ac.Mutable.Tier2CompletenessHint(); strings.TrimSpace(hint) != "" {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   SectionAnswerCoverageNotes,
				Content: hint,
			})
		}
	}

	// Split REPL-assembled Objective into prior-conversation context and
	// the current request. The two go into separate sections so the LLM
	// cannot confuse conversation continuity with the question to answer.
	// In single-shot mode prior is empty and this degenerates to the
	// historical one-section layout.
	//
	// Write-mode apply/verify stages (NOT plan — planner needs the
	// request as its primary input) suppress the User Request section.
	// The raw user phrasing is a PLANNER-shaped directive ("please
	// generate a plan to fix X"); feeding it to the coder or verifier
	// creates role conflict — the session-35 verifier regression was
	// exactly this: user said "generate a plan", verifier's system
	// prompt said "run tests", the empty BuildInitialInstruction
	// supplement left the User Request section as the dominant signal,
	// and the LLM tried to emit_change_plan (not in its tool list) or
	// shelled out with exec_command writing a diff document to stdin.
	//
	// Apply and verify operate purely on Mutable.ChangePlan; the
	// distilled intent is in plan.Request + plan.Summary which the
	// stage's BuildInitialInstruction can surface if needed.
	suppressUserRequest := ac.Stage == types.StageApply || ac.Stage == types.StageVerify
	if ac.Objective != "" && !suppressUserRequest {
		priorConv, currentReq := types.SplitConversation(ac.Objective)
		if currentReq != "" {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   SectionUserRequest,
				Content: currentReq,
			})
		}
		if section := formatPresentationDirective(ac.PresentationDirective, ac.Language); section != "" {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   SectionPresentationDirective,
				Content: section,
			})
		}
		// Analyzer Pre-scan Findings — write-mode StagePlan only.
		//
		// Architecture: in write mode the analyzer runs as a classifier
		// and its TaskGraph is replaced by the linear plan→apply→verify
		// graph (orchestrator.go:498-520). Without this section the
		// planner starts COLD with only the 80-100 char user request,
		// burning 5+ ReAct iterations re-discovering entities, files,
		// and sub-topics the analyzer already extracted — a structural
		// waste that hits the planner's iteration cap on legitimate
		// feature-add tasks before emit_change_plan can fire.
		//
		// Source: structured fields on AnalysisIR only. Specifically
		//   - RequestModel.AnalyzerHints.{Entities,PrimaryEntities,Keywords}
		//   - RequestModel.SubTopics[]
		//   - EvidencePlan.RequiredFiles
		// All four are validated string lists (emit_analysis tool
		// params) or repo_map graph derivations — none carry raw LLM
		// prose. The session-35 leak vector was StageReports.Findings
		// (raw lastContent including <think> preamble); this section
		// touches NEITHER StageReports NOR Mutable.EvidenceClosure,
		// so it cannot reintroduce the emit_change_plan bypass bug.
		//
		// Stage gate: StagePlan only. Apply/verify consume Mutable.
		// ChangePlan directly and have no use for analyzer hints; the
		// stage's BuildInitialInstruction renders plan-derived context.
		if ac.Stage == types.StagePlan {
			if section := formatAnalyzerPrescanForPlan(ac.AnalysisIR); section != "" {
				pc.UserSections = append(pc.UserSections, types.PromptSection{
					Title:   SectionAnalyzerPrescan,
					Content: section,
				})
			}
		}
		if priorConv != "" && !ac.PriorConvHidden {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title: SectionPriorConversation,
				Content: "The text below is prior-turn conversation for continuity. " +
					"Do NOT treat it as the current question, and do NOT copy its citations or symbols into the answer without re-verifying against the current repo.\n\n" +
					priorConv,
			})
		}
	}

	// Log-triage structured bundle. When the log_triage pre-stage
	// produced a validated LogBundle, render it as a dedicated prompt
	// section BEFORE the raw log — the structured view preserves the
	// Cause chain's nesting (critical for Java Caused-by / Rust
	// #[source] / Python __cause__) so the LLM does not need to
	// re-parse multi-level causality from raw text. Skipped for the
	// log_triager agent itself (it is the producer, not a consumer)
	// and when no bundle was emitted.
	//
	// Size note: the structured section is typically much smaller
	// than the raw log (no boilerplate, no repeated frames across
	// parallel goroutines), so adding it alongside the raw section
	// is a small prompt-budget cost in exchange for consistent
	// causality rendering in downstream answers.
	if ac.AgentName != types.AgentLogTriager && !finalizerUsesTypedAnswerSupport(ac) {
		// Render-time drift surface (Step 2 2026-05-10): pass the
		// AgentContext's MultiGraph through so the renderer can run
		// drift detection per frame and warn when a log frame's path
		// resolves to a current-repo file but the symbol does not
		// match (the self-referential synthetic-log trap). nil
		// MultiGraph → no locator → drift section skipped (graceful
		// degrade for single-shot CLI flows).
		locator := authority.LocatorFromMultiGraph(ac.MultiGraph)
		if section := formatLogTriageStructured(ac.LogTriage, locator); section != "" {
			section = sanitiseSectionForLLM(section, ac)
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   SectionLogTriageExtraction,
				Content: section,
			})
		}
	}
	if ac.AgentName != types.AgentPerfTriager {
		locator := authority.LocatorFromMultiGraph(ac.MultiGraph)
		if section := formatPerfTriageStructured(ac.PerfTrace, locator); section != "" {
			section = sanitiseSectionForLLM(section, ac)
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   SectionPerfTriageExtraction,
				Content: section,
			})
		}
	}

	// Raw attached log body. Kept as a distinct section only when the
	// structured LogBundle is absent or non-authoritative. Once the
	// bundle is a verified panic/crash anchor set, showing the raw log
	// alongside the typed bundle creates a competing semantic channel:
	// runtime tuple payloads like Func(0x0, ...) keep baiting the model
	// into caller-provenance / source-parameter claims that the typed
	// contract explicitly marks as observation-only. In that regime the
	// structured bundle already carries the message, frame tree, call
	// chain, and residue snippets the downstream stages need.
	//
	// Size strategy (mirrors internal/tool/blob for tool results):
	//   - ≤ inlineCap (4 KB): inline the whole body.
	//   - > inlineCap: write to `<WorkDir>/attached_log.txt`, inline
	//     head + tail preview, tell the LLM to read_file the blob path
	//     for paginated access to the middle.
	//
	// Empty AttachedLog is a no-op.
	if !shouldSuppressAttachedRuntimeLog(ac) {
		if section := formatAttachedLog(ac.AttachedLog, ac.WorkDir, attachedLogTriageState(ac)); section != "" {
			section = sanitiseSectionForLLM(section, ac)
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   SectionAttachedRuntimeLog,
				Content: section,
			})
		}
	}

	// Attached HiTrace / atrace — same suppression discipline as the
	// raw log section (see shouldSuppressAttachedRuntimeLog above).
	// A typed PerfBundle with resolved files + a strong perf intent
	// hint already encodes the frames / janks / stalls the downstream
	// stages need; leaving the raw trace in the prompt would re-
	// introduce a competing free-form channel, and runtime trace text
	// is even denser in tuple-shaped tokens (thread ids, hex
	// timestamps, event ids) that bait the LLM into spurious
	// caller-provenance claims.
	if !shouldSuppressAttachedRuntimeTrace(ac) {
		if section := formatAttachedTrace(ac.AttachedHitrace, ac.WorkDir, attachedTraceTriageState(ac)); section != "" {
			section = sanitiseSectionForLLM(section, ac)
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				// Title order matches the user-facing CLI flag order:
				// HiTrace / atrace / systrace / perfetto are all
				// ftrace-compatible siblings that flow through the
				// same channel; the prompt section name lists every
				// supported source so a model that pattern-matches on
				// section title doesn't bias toward a single platform.
				Title:   SectionAttachedPerfTrace,
				Content: section,
			})
		}
	}

	if len(ac.PriorReports) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionPriorStageFindings,
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
			Title:   SectionUnverifiedAnalyzer,
			Content: uf,
		})
	}
	if cfgAbsence := formatExactResolutionHint(ac); cfgAbsence != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionExactResolution,
			Content: cfgAbsence,
		})
	}
	if originBoundary := formatEvidenceOriginBoundaryHint(ac); originBoundary != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionEvidenceOrigin,
			Content: originBoundary,
		})
	}
	if toolValue := formatToolSourcedValueHint(ac); toolValue != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionToolSourcedValue,
			Content: toolValue,
		})
	}
	if mrAdvisory := formatMultiRepoActiveSetAdvisory(ac); mrAdvisory != "" {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionMultiRepoActiveSet,
			Content: mrAdvisory,
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
	// or the expected subject is SubjectUnknown/SubjectGeneric. Generic
	// subject matches are intentionally weak, passive hints; rendering
	// their uniform scores as a ranking directive amplifies noise.
	if (ac.Stage == types.StageExtract || ac.Stage == types.StageFinalize) && !suppressSubjectMatchSummaryForTypedSupportFinalizer(ac) {
		if sm := formatSubjectMatchSummary(ac.SubjectMatches, ac.ExpectedAnswerSubject); sm != "" {
			pc.UserSections = append(pc.UserSections, types.PromptSection{
				Title:   SectionSubjectMatchSummary,
				Content: sm,
			})
		}
	}

	// Raw Tool Outputs from Turn A. The extractor and finalizer otherwise
	// only see structured evidence (emit_evidence items + deterministic
	// concrete_value scans) and never lay eyes on the explorer's raw
	// tool results — so a scalar produced by a one-shot command (
	// `find ... | xargs wc -l`, `grep -c`, `list_files` with 200 hits,
	// or a VCS query such as `git log` / `git blame`) cannot travel to
	// Turn B unless the LLM thought to call
	// emit_evidence for it. But emit_evidence's schema requires
	// source + line_start + anchor_kind, which a command-level scalar
	// doesn't have — physically impossible to emit. Surface the raw
	// summaries here so the finalizer can pull the literal directly.
	//
	// NARROW SCOPE — citation-free command/VCS questions only. We gate
	// on the analyzer's typed citation-free classification. Measurement
	// scalars need command output for the literal; repository-history
	// questions need VCS output whether the requested answer is a scalar
	// hash, a feature summary, a recent-merge list, a comparison, or a
	// history-backed diagnostic. For ordinary source-code explanation /
	// step_list / list_of_symbols / config_value answers the section
	// stays hidden — otherwise the finalizer would quote raw read_file
	// dumps instead of the curated Structured Evidence section.
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
					Title:   SectionRawToolOutputs,
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

	if !skipForExtractor && len(ac.RelevantFacts) > 0 && !suppressKnownFactsForTypedSupportFinalizer(ac) {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionKnownFacts,
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
	if len(ac.AnswerSymbols) > 0 && ac.AnswerSymbolCompleteness == types.CompletenessComplete && !suppressAnswerSymbolsForTypedSupportFinalizer(ac) {
		var symContent strings.Builder
		symContent.WriteString("The prior analysis phase has already identified the answer to this question. " +
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
			Title:   SectionAnswerSymbolsAuth,
			Content: symContent.String(),
		})
	} else if len(ac.AnswerSymbols) > 0 && ac.AnswerSymbolCompleteness == types.CompletenessLowerBound && !suppressAnswerSymbolsForTypedSupportFinalizer(ac) {
		var symContent strings.Builder
		symContent.WriteString(fmt.Sprintf(
			"The prior analysis phase has confirmed the following symbols as part of the answer, "+
				"but the list is a LOWER BOUND — additional symbols may also be part of the answer if the "+
				"evidence below supports them. Your task is to render this floor faithfully AND supplement it "+
				"with any additional symbols you can ground in the %s / %s / %s' Resolution Chains sections.\n\n",
			SectionEvidencePool, SectionDataflowFindings, SectionPriorStageFindings))
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
			Title:   SectionAnswerSymbolsFloor,
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
	evidenceItems := ac.EvidenceItems
	evidencePreamble := knowledgePoolPreamble()
	evidenceOpts := evidenceRenderOptions{
		StrictLocation:            strictEvidenceLoc,
		NeutralizeExactResolution: ac.Stage == types.StageFinalize || ac.Stage == types.StageExtract,
		ExactResolutionContract:   exactResolutionContractForRender(ac),
		ExactResolutionScenario:   exactResolutionScenarioForRender(ac),
		TypedRelationHints:        ac.TypedRelationHints,
	}
	if finalizerUsesTypedAnswerSupport(ac) {
		evidenceItems = typedSupportFinalizerEvidencePool(ac, evidenceItems)
		evidencePreamble = typedSupportKnowledgePoolPreamble()
		evidenceOpts.AuthoritativeSurface = true
		evidenceOpts.TypedRelationHints = nil
	}
	evidence := formatEvidenceItemsWithOptions(evidenceItems, 18, evidenceOpts)
	findings := formatFlowFindings(ac.FlowFindings, 10)
	logging.Debug("[builder] %s/%s: evidence_section_len=%d findings_section_len=%d", ac.AgentName, ac.Stage, len(evidence), len(findings))
	// Phase 1 of Semantic Surface Contract
	// (docs/design/semantic_surface_contract_phases.md §1): emit a
	// debug snapshot of the compiled FacetCoverageContract so we
	// can observe family classification + facet binding on real
	// LLM runs before opening any gate (Phase 4+). The surface plan
	// is built fresh per AgentContext via the existing helper;
	// FacetCoverage is the new field on it. Read-only — Phase 1
	// does not change emit/render behaviour.
	if surfacePlan := types.BuildAnswerSurfacePlanForAgentContext(ac); surfacePlan != nil && surfacePlan.FacetCoverage != nil {
		fc := surfacePlan.FacetCoverage
		logging.Debug("[trace/facet] %s/%s family=%s required=%d optional=%d",
			ac.AgentName, ac.Stage, fc.Family, len(fc.Required), len(fc.Optional))
		for i, req := range fc.Required {
			logging.Debug("[trace/facet] required[%d] kind=%s req=%s forms=%v candidates=%d",
				i, req.Kind, req.Required, req.AcceptableForms, len(req.SourceCandidate))
		}
		for i, req := range fc.Optional {
			logging.Debug("[trace/facet] optional[%d] kind=%s req=%s forms=%v candidates=%d",
				i, req.Kind, req.Required, req.AcceptableForms, len(req.SourceCandidate))
		}
	}
	// Structured Evidence carries the full top-18 evidence dump.
	// Skipped for the extract-skill: that dispatch already sees the
	// top-12 via Prior Stage Findings' Primary Evidence subsection
	// and the curated view via the Turn A digest its evaluator
	// appends. Other skills (finalizer especially) need the full
	// list for citation coverage.
	if !skipForExtractor && evidence != "" {
		// B6-T1 (block_only_carrier.md, 2026-05-03) — section
		// renamed from "Structured Evidence" to "Knowledge &
		// Evidence Pool" to bridge V1's LLM-emitted evidence rows
		// and V2's typed-graph rows under one neutral concept.
		// The pool unifies both provenance lanes (llm_evidence /
		// typed_graph) under a single section header so future
		// typed channels (typed dataflow / typed import-graph /
		// typed call-tree) plug in without forking the prompt.
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionEvidencePool,
			Content: evidencePreamble + evidence,
		})
	}

	// Unverified Leads — items the explorer emit_evidence-grounded as
	// ungrounded. Rendered in every skill by default (design pick #4
	// of the 2026-04-17 redesign) so the finalizer sees the leads but
	// is explicitly told not to cite them. The extractor also benefits
	// from the visibility: it can mention a lead in reasoning text
	// without pulling it into emit_answer_symbol.
	if leads := formatUnverifiedLeads(ac.EvidenceItems, 12, strictEvidenceLoc); leads != "" && !suppressUnverifiedLeadsForTypedSupportFinalizer(ac) {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionUnverifiedLeads,
			Content: leads,
		})
	}

	if findings != "" && !(ac.Stage == types.StageFinalize && priorReportsContainSection(ac.PriorReports, "## Dataflow Findings")) {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionDataflowFindings,
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
	if ac.Mutable != nil && !finalizerUsesTypedAnswerSupport(ac) {
		if verdicts := ac.Mutable.EmittedHypothesisVerdicts(); len(verdicts) > 0 {
			var vc strings.Builder
			vc.WriteString("The prior extraction pass reached the following verdicts on the hypotheses posed during classification. " +
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
				Title:   SectionHypothesisVerdicts,
				Content: vc.String(),
			})
		}
	}

	if len(ac.RelevantFiles) > 0 {
		pc.UserSections = append(pc.UserSections, types.PromptSection{
			Title:   SectionRelevantFiles,
			Content: strings.Join(ac.RelevantFiles, "\n"),
		})
	}

	// Enabled tools from skill suggestions
	pc.EnabledTools = sk.ToolSuggestions

	return pc
}

// agentIdentityPrompt renders the system-prompt opening sentence
// without leaking internal stage codenames into the LLM's context.
// The Go-side AgentName / PipelineStage values double as wire-format
// identifiers (e.g. "finalizer" / "log_triager") and are deliberately
// listed in skill.InternalTermsBlocklist; we map each one to a
// user-facing role description so the LLM still gets a clear
// orientation cue without seeing the codename.
//
// Falls back to a neutral generic prompt when AgentName is unknown
// (defensive — the production registry covers every shipping agent
// today).
func agentIdentityPrompt(name types.AgentName, stage types.PipelineStage) string {
	role := ""
	switch name {
	case types.AgentAnalyzer, types.AgentWriteAnalyzer:
		role = "a request-classifier reading the user's question and producing a structured analysis"
	case types.AgentExplorer:
		role = "a code investigator reading source files and collecting grounded evidence"
	case "sub_explorer":
		role = "a focused sub-investigator reading source files within a narrow scope"
	case types.AgentExtractor:
		role = "an answer-slate extractor selecting the typed answer items from the prior investigation's frozen evidence"
	case types.AgentFinalizer:
		role = "an answer author writing the final structured response from typed evidence"
	case types.AgentLogTriager:
		role = "a log structurer parsing the attached runtime log into a typed bundle"
	case types.AgentPerfTriager:
		role = "a performance-trace structurer parsing the attached trace into a typed bundle"
	case types.AgentPlanner:
		role = "a change planner producing a structured plan of file-level edits"
	case types.AgentCoder:
		role = "a patch applier executing the planned edits in a sandbox worktree"
	case types.AgentVerifier:
		role = "a test runner executing the project's tests against the applied changes"
	default:
		if string(name) != "" {
			role = "an AI assistant performing the " + string(name) + " role"
		} else {
			role = "an AI assistant in a code-analysis pipeline"
		}
	}
	_ = stage // stage codename intentionally not surfaced to the LLM — the role above already conveys the position.
	return "You are " + role + "."
}

func shouldSuppressAttachedRuntimeLog(ac *types.AgentContext) bool {
	if ac == nil {
		return false
	}
	if ac.AgentName == types.AgentLogTriager {
		return false
	}
	if finalizerUsesTypedAnswerSupport(ac) {
		return true
	}
	bundle := ac.LogTriage
	if bundle == nil || bundle.IsExternalSource() {
		return false
	}
	return bundleHasAuthoritativeCrashFrames(bundle)
}

// shouldSuppressAttachedRuntimeTrace mirrors
// shouldSuppressAttachedRuntimeLog for the perf-trace channel.
// perf_triager always sees the raw trace (it is the producer);
// finalizers running typed-answer-support mode always suppress it
// (typed support lanes carry the authoritative summary). For all
// other agents the gate is "typed PerfBundle is non-nil + carries a
// performance IntentHint + has at least one resolved file" — at that
// point the structured Frames / Janks / Stalls / Startup view is
// the authoritative anchor set, and the raw trace's tuple-dense
// payload (thread ids, hex addresses, μs timestamps) only bait the
// LLM into rationalising those numbers as causation.
func shouldSuppressAttachedRuntimeTrace(ac *types.AgentContext) bool {
	if ac == nil {
		return false
	}
	if ac.AgentName == types.AgentPerfTriager {
		return false
	}
	if finalizerUsesTypedAnswerSupport(ac) {
		return true
	}
	bundle := ac.PerfTrace
	if bundle == nil {
		return false
	}
	return bundleHasAuthoritativePerfFrames(bundle)
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
// BuildSubAgentContext narrows the parent BusContext into a child
// AgentContext for sub-agent dispatch. The bulk of the typed-signal
// propagation (MultiGraph / TypedDenials / PendingSubRepos / Memory /
// Ctx / EnvFacts / EnvRecommendSettings) lives in
// types.SubAgentContext — the single canonical narrowing helper. This
// function then layers the per-sub-agent scope filtering on top:
// RepoFacts / EvidenceItems / FlowFindings / ToolResults / MCPResponses
// are stage-scoped artifacts the parent has accumulated; the sub-agent
// receives only the slice that overlaps its declared scope.
//
// Mutable is intentionally DROPPED by types.SubAgentContext so the
// sub-agent cannot mutate parent state. The repomap SearchGraph
// pointer (read-only) is propagated separately via Mutable.SearchGraph
// so the sub-agent doesn't pay a BuildOrLoadGraph round-trip on every
// dispatch.
func BuildSubAgentContext(bus *types.BusContext, req *types.SubAgentRequest) *types.AgentContext {
	ac := types.SubAgentContext(bus, req)

	// Per-sub-agent scope filtering. Parent BusContext carries full
	// stage artifact lists; project them down to what the sub-agent
	// declared as its scope.
	if bus != nil {
		ac.RelevantFacts = extractRelevantFacts(bus.RepoFacts)
		ac.RelevantFiles = filterFilesByScope(bus.RepoFacts, req.Scope)
		ac.EvidenceItems = filterEvidenceItemsByScope(bus.EvidenceItems, req.Scope)
		ac.FlowFindings = filterFlowFindingsByEvidence(ac.EvidenceItems, bus.FlowFindings)
		ac.RelevantToolSummaries = extractToolSummaries(bus.ToolResults)
		ac.RelevantMCPNotes = extractMCPNotes(bus.MCPResponses)
		// SearchGraph is the only Mutable-derived state propagated to
		// sub-agents — read-only graph handle, no mutation surface.
		if bus.Mutable != nil {
			ac.SearchGraph = bus.Mutable.SearchGraph()
		}
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
// Structured Evidence sections. Ungrounded and diagnostic-only items
// are NOT rendered here; they flow through
// formatUnverifiedLeads into a dedicated "Unverified Leads" section
// so the finalizer cannot accidentally treat them as answer-grade citations.
//
// strictLocation toggles session-8 "no non-Tier-1 lines reach Turn B"
// behaviour: when true, Recovered items show `(file)` without a line
// and get a `[recovered — line not read; re-run read_file before
// citing]` tag so the LLM can't silently pick up an anchor that the
// finalizer-time grounder will later reject. When false, the
// historical "(file:line) [recovered]" format ships so the explorer
// itself (self-referencing earlier windows) still sees its own
// recovered lines.
// producerRank mirrors agent.evidenceSortRank locally so
// formatEvidenceItems can re-apply the rank ordering at render time
// without introducing an import cycle or exposing a package-boundary
// helper purely for cosmetics. Both sides must agree on the band
// mapping; any change to one requires the same change in the other.
func producerRank(item types.EvidenceItem) int {
	switch {
	case item.Producer == "explorer.emit_evidence":
		return 0
	case strings.HasPrefix(item.Producer, "dataflow."):
		return 2
	default:
		return 1
	}
}

type evidenceRenderOptions struct {
	StrictLocation            bool
	NeutralizeExactResolution bool
	AuthoritativeSurface      bool
	ExactResolutionContract   *types.ExactResolutionContract
	ExactResolutionScenario   types.Scenario

	// TypedRelationHints, when non-empty, drives the typed_graph
	// appendix rendered into the SAME Structured Evidence section
	// after the LLM emit_evidence rows (P3 #6 follow-up, 2026-05-03).
	// Dedup against the items slice ensures no (Subject, Object,
	// AnchorKind) tuple is shown twice across the two provenance
	// lanes. Empty/nil leaves rendering byte-identical to the
	// pre-typed-relation behaviour.
	TypedRelationHints []types.TypedRelationHint
}

func exactResolutionContractForRender(ac *types.AgentContext) *types.ExactResolutionContract {
	if ac == nil || ac.AnalysisIR == nil {
		return nil
	}
	return ac.AnalysisIR.AnswerContract.ExactResolution
}

func exactResolutionScenarioForRender(ac *types.AgentContext) types.Scenario {
	if ac == nil || ac.AnalysisIR == nil {
		return types.ScenarioGeneric
	}
	return ac.AnalysisIR.RequestModel.Scenario
}

// knowledgePoolPreamble returns the LLM-facing description that
// prefixes the unified Knowledge & Evidence Pool section (B6-T1
// rename). Two short sentences in LLM-natural language; no
// internal Go terminology (R4 red line). Future typed-channel
// additions only need to add a Provenance value; this preamble
// does not need to enumerate them by name.
func knowledgePoolPreamble() string {
	return "The pool below unifies evidence the investigation collected (provenance=llm_evidence) " +
		"with structurally-derived candidates from the project's typed graph (provenance=typed_graph). " +
		"Both lanes are authoritative grounding for citations; treat them as one source of truth and pick whichever rows your answer needs.\n\n"
}

func typedSupportKnowledgePoolPreamble() string {
	return "Use the pool below as citation coverage only. Build user-visible principal claims from the typed support lanes above; do not introduce new principal conclusions from this pool alone.\n\n"
}

func formatEvidenceItems(items []types.EvidenceItem, limit int, strictLocation bool) string {
	return formatEvidenceItemsWithOptions(items, limit, evidenceRenderOptions{
		StrictLocation: strictLocation,
	})
}

func formatEvidenceItemsWithOptions(items []types.EvidenceItem, limit int, opts evidenceRenderOptions) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	// Defensive re-rank by producer-rank before taking the top-N slice.
	// The upstream pool is already merge-sorted by rank, but several
	// explorer paths (rankEvidenceByRelevanceWithSubject, diversity
	// cap) re-order the slice between the merge and this render step.
	// Without this safety net, rank-1 programmatic items (concrete_values
	// on alphabetically-early files like cmd/root.go) flood the top-N
	// before any LLM-emitted item gets a slot, even though the LLM's
	// emissions are the most on-topic facts available.
	//
	// Keep this O(N), not O(N log N): large-repo runs can carry tens
	// of thousands of evidence rows while this renderer usually needs
	// only the top ~18. Producer ranks are three fixed bands, so stable
	// buckets preserve the old sort order without full-slice sorting.
	renderItems := selectEvidenceItemsForRender(items, limit)
	// Diagnostic: top-25 producer histogram. Retained because operators
	// investigating "why didn't my emit show up in Structured Evidence"
	// benefit from the real producer distribution at the rendering site.
	//
	// Phase 0 of the Semantic Surface Contract rollout
	// (docs/design/semantic_surface_contract_phases.md §0): each trace
	// row also carries the deterministic ClaimForm projection so
	// future Phase-4 validators have a documented signal to reason
	// about. The per-row claim_form value is read-only — Phase 0
	// does NOT change emission or grounding behaviour.
	if len(items) > 0 {
		counts := map[string]int{}
		salienceCounts := map[string]int{}
		lockedSalience := 0
		for i, it := range items {
			if i >= 25 {
				break
			}
			salience := string(it.Salience.Resolve())
			if it.Salience.IsSet() {
				salience = string(it.Salience)
			} else {
				salience = "unset"
			}
			salienceCounts[salience]++
			if it.SalienceLockedForScoring() {
				lockedSalience++
			}
			logging.Debug("[trace/fev] %d producer=%q src=%s:%d subj=%q kind=%q claim_form=%q grounding=%s salience=%q",
				i, it.Producer, it.Source, it.LineStart, it.Subject, it.Kind, types.ClaimFormOf(it), it.GroundingStatus, salience)
			counts[it.Producer]++
		}
		logging.Debug("[trace/fev] total=%d top25 producer histogram: %v", len(items), counts)
		logging.Debug("[trace/fev] total=%d top25 salience histogram: %v", len(items), salienceCounts)
		if lockedSalience > 8 {
			logging.Warning("[trace/fev] high salience-locked evidence count in top25: locked=%d", lockedSalience)
		}
	}
	var b strings.Builder
	written := 0
	for _, item := range renderItems {
		if !isStructuredEvidenceItem(item) {
			continue
		}
		if written >= limit {
			break
		}
		line := evidencePromptLine(item, opts)
		if strings.TrimSpace(line) == "" {
			continue
		}
		if loc := item.DisplayLocation(opts.StrictLocation); loc != "" {
			line += " (" + loc + ")"
		}
		if item.GroundingStatus == types.GroundingRecovered {
			if opts.StrictLocation {
				line += " [recovered – line not read; re-run read_file before citing]"
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
		over := countStructuredEvidenceItems(items) - written
		if over > 0 {
			fmt.Fprintf(&b, "... and %d more evidence items\n", over)
		}
	}
	// P3 #6 follow-up (2026-05-03) — append typed-graph relation rows
	// when present, deduped against the LLM-emit rows above by
	// (Subject, Object, AnchorKind). Per the
	// feedback_no_system_backfill_to_user_panel red line, these flow
	// only into the LLM's prompt, never into AnswerDocument fields.
	// Per the user's "no split sections" feedback, the typed rows
	// share the same Structured Evidence header; the Provenance tag
	// distinguishes them inline.
	if appendix := renderTypedRelationAppendix(opts.TypedRelationHints, items); appendix != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(appendix)
	}
	return strings.TrimSpace(b.String())
}

func selectEvidenceItemsForRender(items []types.EvidenceItem, limit int) []types.EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	buckets := [3][]types.EvidenceItem{}
	for _, item := range items {
		rank := producerRank(item)
		if rank < 0 {
			rank = 0
		}
		if rank >= len(buckets) {
			rank = len(buckets) - 1
		}
		buckets[rank] = append(buckets[rank], item)
	}
	out := make([]types.EvidenceItem, 0, limit)
	for _, bucket := range buckets {
		for _, item := range bucket {
			if len(out) >= limit {
				return out
			}
			out = append(out, item)
		}
	}
	return out
}

// renderTypedRelationAppendix produces the typed_graph-tagged rows
// for the Structured Evidence section. Empty when no hints OR every
// hint member is already covered by an LLM-emitted EvidenceItem.
//
// Dedup key: (SourceName, MemberName, AnchorKind) — matches the
// (Subject, Object, AnchorKind) tuple LLM emit_evidence rows would
// use when describing the same relation member, so the LLM never
// sees the same (interface, implementer) pair listed twice across
// the two provenance lanes.
func renderTypedRelationAppendix(hints []types.TypedRelationHint, llmItems []types.EvidenceItem) string {
	if len(hints) == 0 {
		return ""
	}
	covered := make(map[string]bool, len(llmItems))
	for _, it := range llmItems {
		if it.Subject == "" || it.Object == "" {
			continue
		}
		covered[typedRelationDedupKey(it.Subject, it.Object, it.AnchorKind)] = true
	}
	var out strings.Builder
	for _, h := range hints {
		if h.SourceName == "" || len(h.Members) == 0 {
			continue
		}
		ak := types.TypedRelationAnchorKind(h.Relation)
		var rendered []types.TypedRelationMember
		for _, m := range h.Members {
			if covered[typedRelationDedupKey(h.SourceName, m.Name, ak)] {
				continue
			}
			rendered = append(rendered, m)
		}
		if len(rendered) == 0 {
			continue
		}
		for _, m := range rendered {
			line := evidenceLineForTypedMember(h, m, ak)
			out.WriteString("- " + line + "\n")
		}
	}
	if out.Len() == 0 {
		return ""
	}
	return strings.TrimRight(out.String(), "\n")
}

// evidenceLineForTypedMember renders one typed_graph-provenance row
// in the same shape (subject — predicate — object — file:line) that
// llm_evidence rows use. The "[typed_graph]" Provenance tag at the
// end is the only structural difference, so the LLM sees a uniform
// table where Provenance is metadata, not a section divider.
func evidenceLineForTypedMember(h types.TypedRelationHint, m types.TypedRelationMember, ak types.AnchorKind) string {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(h.Relation)
	b.WriteString("] ")
	b.WriteString(h.SourceName)
	b.WriteString(" — ")
	b.WriteString(m.Name)
	if m.File != "" {
		b.WriteString(" (")
		b.WriteString(m.File)
		if m.Line > 0 {
			fmt.Fprintf(&b, ":%d", m.Line)
		}
		b.WriteString(")")
	}
	if ak != "" {
		b.WriteString(" anchor_kind=")
		b.WriteString(string(ak))
	}
	b.WriteString(" provenance=typed_graph")
	if h.SourceKind != "" {
		b.WriteString(" source_kind=")
		b.WriteString(h.SourceKind)
	}
	if m.Kind != "" {
		b.WriteString(" member_kind=")
		b.WriteString(m.Kind)
	}
	return b.String()
}

// typedRelationDedupKey is the canonical (Subject, Object, AnchorKind)
// fingerprint used to suppress typed_graph rows that an LLM emit row
// already covers. AnchorKind comparison is by string value so the
// empty / unknown case maps cleanly.
func typedRelationDedupKey(subject, object string, ak types.AnchorKind) string {
	return subject + "\x00" + object + "\x00" + string(ak)
}

func evidencePromptLine(item types.EvidenceItem, opts evidenceRenderOptions) string {
	if opts.AuthoritativeSurface {
		return types.EvidenceAuthoritativeSurfaceText(item, true)
	}
	contract := (*types.ExactResolutionContract)(nil)
	if opts.NeutralizeExactResolution {
		contract = opts.ExactResolutionContract
	}
	return types.EvidencePreferredSurfaceText(item, contract, true)
}

func isStructuredEvidenceItem(item types.EvidenceItem) bool {
	if item.GroundingStatus == types.GroundingUngrounded {
		return false
	}
	switch item.Kind {
	case types.EvidenceUnresolved, types.EvidenceTruncated:
		return false
	}
	return true
}

// formatUnverifiedLeads renders non-citation material: LLM claims
// whose GroundingStatus is Ungrounded plus deterministic diagnostic
// items such as unresolved dataflow or analysis truncation notices.
// These are discussion hints, never emitted as citations.
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
		if isUnverifiedLeadItem(it) {
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
	b.WriteString("These items are diagnostic leads, not answer-grade citations: either the grounder could not validate the file:line citation, or deterministic dataflow marked the path as unresolved/truncated. Treat them as discussion hints; do NOT emit them in the answer's citations[] field.\n\n")
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
			if item.Kind == types.EvidenceUnresolved || item.Kind == types.EvidenceTruncated {
				loc = item.Source
			}
			line += " (" + loc + ")"
		}
		switch item.Kind {
		case types.EvidenceUnresolved:
			line += " [unresolved]"
		case types.EvidenceTruncated:
			line += " [analysis_truncated]"
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

func isUnverifiedLeadItem(item types.EvidenceItem) bool {
	if item.GroundingStatus == types.GroundingUngrounded {
		return true
	}
	switch item.Kind {
	case types.EvidenceUnresolved, types.EvidenceTruncated:
		return true
	}
	return false
}

func countStructuredEvidenceItems(items []types.EvidenceItem) int {
	n := 0
	for _, it := range items {
		if isStructuredEvidenceItem(it) {
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

// isCitationFreeValueAnswer reports whether the IR says this answer is
// a scalar value whose authoritative literal can come from tool / VCS /
// external output rather than from a repo file:line citation.
//
// The discriminator mirrors analyzer.buildAnalysisIR's citation-free
// value carve-out. Using the IR state as the gate keeps the rule 1:1
// with its producer and avoids a second keyword table here.
func isCitationFreeValueAnswer(ac *types.AgentContext) bool {
	if ac == nil || ac.Mutable == nil || ac.AnalysisIR == nil {
		return false
	}
	c := ac.AnalysisIR.AnswerContract
	if c.CitationReq.Required {
		return false
	}
	rm := ac.AnalysisIR.RequestModel
	if rm.Predicates.IsHistoryLookup {
		return true
	}
	// Citation-free scalar — the analyzer's measurement-scalar
	// carve-out drops CitationReq.Required for command-level returns
	// (wc -l, grep -c, etc.). Combined with the typed IsScalarAnswer
	// predicate (the LLM's classification of "answer is one literal"),
	// this is the precise signal for surfacing raw command output.
	return rm.Predicates.IsScalarAnswer
}

// shouldRenderRawToolOutputs gates the Raw Tool Outputs section to
// citation-free value questions on extract/finalize only. Explorer
// already has the live tool transcript; analyzer never reaches this
// block.
func shouldRenderRawToolOutputs(ac *types.AgentContext) bool {
	if ac.Stage != types.StageExtract && ac.Stage != types.StageFinalize {
		return false
	}
	return isCitationFreeValueAnswer(ac)
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
	rawToolOutputTotalCapBytes    = 16000
	rawToolOutputVCSHeadBytes     = 12000
	rawToolOutputVCSTailBytes     = 2000
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
// path validator — then gets stuck in a retry loop that never
// produces a valid document. Citation-free scalar answers should use
// the schema's explicit no-citation carrier, not fake source citations.
const rawToolOutputPreamble = "These are the raw outputs of commands run during the investigation. " +
	"Use them as the source of TRUTH for citation-free command / VCS answers: scalar measurements, commit hashes, subject lines, feature summaries, recent-merge lists, commit comparisons, and history-backed diagnostics. " +
	"These tool outputs are NOT repo files — they MUST NOT appear in citations[]. " +
	"When the user asked for one literal scalar, emit a `scalar` block whose `text` starts with the literal taken directly from the tool output tail, and attach a one-element uncited item anchor. When the user asked for a summary, list, comparison, or diagnostic, use `summary` / `section` / `table` / `ordered_list` blocks and cite command/VCS provenance in prose with uncited items only where an item needs an anchor; do not collapse the answer into a bare scalar just because the evidence came from git or a shell command.\n\n"

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
	head, tail := rawToolOutputCaps(toolName)
	var body string
	if len(summary) <= head+tail {
		body = summary
	} else {
		body = summary[:head] + "\n...[trimmed " + fmt.Sprint(len(summary)-head-tail) + " bytes]...\n" + summary[len(summary)-tail:]
	}
	return fmt.Sprintf("- **%s** (%d bytes):\n```\n%s\n```\n", toolName, len(summary), body)
}

func rawToolOutputCaps(toolName string) (int, int) {
	switch toolName {
	case "git_log", "git_history_search":
		return rawToolOutputVCSHeadBytes, rawToolOutputVCSTailBytes
	case "git_show":
		return rawToolOutputVCSHeadBytes / 2, rawToolOutputVCSTailBytes / 2
	default:
		return rawToolOutputPerCallHeadBytes, rawToolOutputPerCallTailBytes
	}
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

func formatPresentationDirective(directive, lang string) string {
	directive = strings.TrimSpace(directive)
	if directive == "" {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(lang), "en") {
		return "这是从用户当前请求中提取的结构化展示要求。只用于选择最终答案的展示字段，例如 diagram_hint、表格、标量答案或决策结论。" +
			"不要把它当作仓库代码、代码实体、搜索查询、事实证据或历史对话内容。\n\n" +
			directive
	}
	return "Structured current-turn presentation requirement derived from the user's current request. " +
		"Use it only to choose final-answer presentation fields such as diagram_hint, table, scalar, or decision. " +
		"Do NOT treat it as repository code, a code entity, a search query, factual evidence, or prior-conversation content.\n\n" +
		directive
}

func finalizerUsesTypedAnswerSupport(ac *types.AgentContext) bool {
	if ac == nil || ac.AgentName != types.AgentFinalizer {
		return false
	}
	plan := types.BuildAnswerSupportPlanForAgentContext(ac)
	return plan != nil && len(plan.Lanes) > 0
}

// AttachedLogBlobName is the canonical filename codrax writes the
// full log body to under `<WorkDir>/` when the attached log exceeds
// the inline cap. Kept public so the read_file tool can recognise it
// as a blob-backed attachment (avoids a repo path check false-positive).
const AttachedLogBlobName = "attached_log.txt"

// AttachedTraceBlobName is the perf-trace companion to
// AttachedLogBlobName. Kept distinct so a run carrying both a panic
// log and a performance trace cannot overwrite one attachment blob
// with the other inside the shared WorkDir.
const AttachedTraceBlobName = "attached_trace.txt"

// renderBugClassesSection produces the LLM-facing canonical-pattern
// block for the log_triage / perf_triage prompt section.
// `modality` is "log" or "trace" — adjusts user-facing terminology
// (frames vs spans/tags, failure vs observation) while keeping the
// anti-hallucination guard generic.
//
// Two regimes:
//
//   - detected non-empty → render canonical bilingual labels +
//     matched signature substrings + generic instruction to use those
//     terms verbatim and avoid speculating about cause from unrelated
//     repository code.
//   - detected empty → render an "unknown / business-domain pattern"
//     guide so the LLM understands no standard pattern matched and
//     should address the user's question using terminology drawn
//     from the input's own content (custom business operation names,
//     application-specific identifiers, third-party domain terms).
//     This regime does NOT presuppose the user is asking about a
//     failure — performance characterisation, business audit, or
//     any other intent is equally valid.
//
// R6 / R4 compliance:
//   - No internal pipeline terminology (no "BugClass", "TypedDenials",
//     "registry", phase names)
//   - No fixture-specific examples — only canonical labels from the
//     bundle's actual detection appear; instructions are generic
//   - Generic enough to apply to any input modality (log / trace /
//     future MCP) and any user intent — failure analysis,
//     performance investigation, business observation, audit
//   - Does NOT constrain the user's question intent: even when known
//     patterns are detected, the LLM is free to address whatever
//     dimension the user actually asked about; the canonical labels
//     are a vocabulary aid, not a topic redirect
//
// renderPrimaryErrorSignal (Fix-C 2026-05-10) emits a high-priority
// section quoting the verbatim runtime error MESSAGE before the
// Detected Patterns + Error tree sections. Frames may be unresolved
// (synthetic / vendored / generated code) but the runtime's own
// error string is always authoritative. Empty bundle / no errors
// returns "" (skipped section).
//
// When ≥ 2 distinct frames across all errors share the same
// (file, line) coordinate AND any error matches a race-class
// pattern via inline scan of the message text, surface a
// corroborating "parallel concurrency" sub-bullet so the LLM
// recognises the data-race pattern even when the BugClass
// detector's first-match-only behaviour missed a finer pattern.
//
// Cross-language: works on any language whose log triager
// populates LogError.Type + Message. The parallel-frame
// detector is purely structural (path coordinate counts),
// no language-specific code.
//
// Design doc: docs/design/analyzer_failure_remediation.md §3.3.
func renderPrimaryErrorSignal(bundle *types.LogBundle) string {
	if bundle == nil || len(bundle.Errors) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Primary Error Signal\n\n")
	b.WriteString("The runtime emitted these verbatim error messages before the stack frames were captured. " +
		"They are the highest-confidence diagnostic signal — anchor your reasoning on these, especially when frames below are marked `(unresolved)`. " +
		"Do NOT invent a different theory by reading frame file:line refs in isolation.\n\n")
	for i := range bundle.Errors {
		renderPrimaryErrorEntry(&b, &bundle.Errors[i], i+1)
	}
	// Parallel-frame detection: count occurrences of each
	// (file, line) coordinate across ALL frames in this bundle.
	// A coordinate hit ≥ 2 times is a STRUCTURAL signal — multiple
	// frames at the same point in code — but not necessarily a
	// concurrency signal. Recursion, repeated exception wrapping,
	// fan-out callbacks, or just unrelated errors at the same
	// instrumentation point can all produce duplicate
	// coordinates. We surface the structural co-occurrence as a
	// neutral fact and ONLY pair it with the "race / concurrent"
	// interpretation when a deterministic signal corroborates:
	//
	//   - BugClass detector flagged BugClassRace on the raw log
	//     (covers "concurrent map writes", "race detected", etc.
	//     across Go / Python / Java / Rust / C++ runtimes), OR
	//   - the verbatim error message contains a race-vocabulary
	//     keyword (cross-language / cross-locale, narrow set —
	//     no broad heuristics).
	//
	// Without the corroboration the framing stays neutral so the
	// LLM can't be led to a wrong concurrency theory by a non-
	// race duplicate-coordinate pattern.
	parallel := detectParallelFrameCoordinates(bundle)
	if len(parallel) > 0 {
		raceCorroborated := bundleHasRaceCorroboration(bundle)
		if raceCorroborated {
			b.WriteString("\n**Parallel-frame signal (race-corroborated)**: multiple goroutines / threads / tasks crashed at the SAME line coordinate AND the runtime error / detected pattern signals a data race. " +
				"This is a strong corroborating signal for a concurrent / racy access pattern (e.g. data race, lock contention) — " +
				"weight the runtime error message above this hint when deciding the root cause.\n")
		} else {
			b.WriteString("\n**Parallel-frame signal (neutral)**: multiple frames share the SAME line coordinate. " +
				"This is a structural co-occurrence — NOT necessarily a concurrency or race indicator. " +
				"Common non-race causes include recursion, repeated exception wrapping, fan-out callbacks, or independent errors hitting the same instrumentation point. " +
				"Use the verbatim error message above as the authoritative signal.\n")
		}
		for _, p := range parallel {
			fmt.Fprintf(&b, "  - `%s:%d` — %d occurrences across the captured frames\n", p.file, p.line, p.count)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// bundleHasRaceCorroboration reports whether the bundle carries a
// deterministic signal that the parallel-frame coordinate is
// concurrency-related. Two corroborators (cross-language):
//
//  1. The BugClass detector classified ≥ 1 entry as BugClassRace.
//     The detector covers Go / Python / Java / Rust / C++ runtime
//     race signatures via internal/analysis/logtriage/bug_class_registry.go.
//     This is a typed signal — first-match-only but precise.
//
//  2. Any error's Type or Message contains a race-vocabulary
//     keyword. Narrow set (cross-language / cross-locale), kept
//     minimal so non-race errors don't accidentally trip it:
//     "race", "concurrent", "data race", "数据竞争", "并发",
//     "deadlock", "死锁".
//
// Returns false when neither lane fires. Used by the Primary Error
// Signal renderer to gate the "race-corroborated" framing — without
// this gate, structural co-occurrence (recursion / fan-out /
// exception wrapping) was being framed as concurrency, biasing the
// LLM toward incorrect race-condition theories. 2026-05-10 P2
// audit follow-up.
func bundleHasRaceCorroboration(bundle *types.LogBundle) bool {
	if bundle == nil {
		return false
	}
	for _, bc := range bundle.Meta.BugClasses {
		if bc.Class == types.BugClassRace {
			return true
		}
	}
	keywords := []string{
		// Lower-case the input once + match these lower-cased.
		// Narrow on purpose — broad keywords ("lock", "thread")
		// would false-fire on non-race errors mentioning lock
		// names or threading models incidentally.
		"data race", "race condition", "race detected", "race-detect",
		"concurrent map", "concurrent write", "concurrent access",
		"deadlock", "livelock",
		// Chinese vocabulary covering the same concepts.
		"数据竞争", "竞态", "并发写", "并发访问", "死锁", "活锁",
	}
	for _, e := range bundle.Errors {
		if matchesAnyRaceKeyword(e.Type, keywords) || matchesAnyRaceKeyword(e.Message, keywords) {
			return true
		}
		if e.Cause != nil {
			if matchesAnyRaceKeyword(e.Cause.Type, keywords) || matchesAnyRaceKeyword(e.Cause.Message, keywords) {
				return true
			}
		}
	}
	return false
}

// matchesAnyRaceKeyword reports whether s (case-insensitive)
// contains any of the keywords. Empty s returns false.
func matchesAnyRaceKeyword(s string, keywords []string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, k := range keywords {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

// renderPrimaryErrorEntry renders one top-level error's verbatim
// type + message + immediate cause chain heading. Avoids the
// "Type — Message" inline format used by renderLogError so the
// message gets its own visual emphasis.
func renderPrimaryErrorEntry(b *strings.Builder, e *types.LogError, index int) {
	if e == nil {
		return
	}
	if e.Type != "" && e.Message != "" {
		fmt.Fprintf(b, "%d. **%s**: `%s`\n", index, e.Type, sanitizeForInlineCode(e.Message))
	} else if e.Type != "" {
		fmt.Fprintf(b, "%d. **%s**\n", index, e.Type)
	} else if e.Message != "" {
		fmt.Fprintf(b, "%d. `%s`\n", index, sanitizeForInlineCode(e.Message))
	} else {
		return
	}
	// Walk Cause chain at primary-signal depth so the operator
	// sees the "X caused by Y" semantics at the top.
	if e.Cause != nil {
		fmt.Fprintf(b, "   - caused by **%s**", e.Cause.Type)
		if e.Cause.Message != "" {
			fmt.Fprintf(b, ": `%s`", sanitizeForInlineCode(e.Cause.Message))
		}
		b.WriteString("\n")
	}
}

// sanitizeForInlineCode strips characters that would break the
// inline backtick rendering. Keeps the message human-readable;
// only neutralises the backtick itself + control chars.
func sanitizeForInlineCode(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "`", "'")
	// Strip ASCII control chars (newlines / tabs flatten to space).
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 {
			if r == '\n' || r == '\t' || r == '\r' {
				b.WriteRune(' ')
			}
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 200 {
		out = out[:200] + "..."
	}
	return out
}

// parallelFrameHit names a (file, line) coordinate that appears in
// ≥ 2 frames across the bundle.
type parallelFrameHit struct {
	file  string
	line  int
	count int
}

// detectParallelFrameCoordinates walks every Error → Frames in the
// bundle (including nested Cause chains) and returns the (file,
// line) coordinates that appear ≥ 2 times. Sorted by count
// descending then file asc for deterministic output.
//
// Frames with empty File (frame validator marked unresolvable) are
// included — the LLM can still infer "3 frames at the same
// unresolved coordinate" as a co-occurrence signal even when none
// resolve to real code.
func detectParallelFrameCoordinates(bundle *types.LogBundle) []parallelFrameHit {
	if bundle == nil {
		return nil
	}
	type key struct {
		file string
		line int
	}
	hits := map[key]int{}
	var walk func(e *types.LogError)
	walk = func(e *types.LogError) {
		if e == nil {
			return
		}
		for _, f := range e.Frames {
			// Use Raw text fallback when File is empty so synthetic
			// frames still co-locate. Strip leading/trailing space.
			fileKey := strings.TrimSpace(f.File)
			if fileKey == "" {
				// Try to extract <path>:<line> from Raw — fall back
				// to skipping the frame when neither File nor a
				// pasrable Raw is present. We don't pattern-match
				// the raw form here: when File is "", the validator
				// already failed to corroborate, and using Raw text
				// as a key would be brittle. Skip.
				continue
			}
			if f.Line <= 0 {
				continue
			}
			hits[key{file: fileKey, line: f.Line}]++
		}
		walk(e.Cause)
	}
	for i := range bundle.Errors {
		walk(&bundle.Errors[i])
	}
	if len(hits) == 0 {
		return nil
	}
	var out []parallelFrameHit
	for k, v := range hits {
		if v >= 2 {
			out = append(out, parallelFrameHit{file: k.file, line: k.line, count: v})
		}
	}
	// Sort: highest count first, then file asc.
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].file < out[j].file
	})
	return out
}

func renderBugClassesSection(detected []types.DetectedBugClass, modality string) string {
	if modality != "trace" {
		modality = "log" // default
	}
	inputNoun := "log"
	contentDescriptor := "the function names, the message text, the call sequence"
	emptyDescriptor := "the exception type, the error message, the application module names visible in frames"
	if modality == "trace" {
		inputNoun = "trace"
		contentDescriptor = "the span/tag names, the durations, the call sequence"
		emptyDescriptor = "the tag names, the span operations, the application module names visible in events"
	}

	var b strings.Builder
	// Top-of-section preamble. Fires regardless of bug_class state
	// (matched OR empty) so the "read + judge" framing applies to
	// every log shape — crash signatures, iteration / retry
	// patterns, capacity / throttling traces, config drift, business
	// audit trails, anything else. The classifier output below is
	// FACTUAL CONTEXT that may help with terminology priming when it
	// matches; the model's reading of the raw content is always the
	// authority on what the log shows. Generalised on 2026-05-10
	// after customer-reported scenarios where bug_class registry
	// (necessarily a finite enumeration) had no useful entry for the
	// log shape and the model needed unambiguous permission to form
	// its own classification.
	b.WriteString("### How to read this " + inputNoun + "\n")
	b.WriteString("Read the raw " + inputNoun + " content carefully and form your own judgment about what it shows. " +
		"This applies to every shape the input may take — crash / panic / exception, " +
		"iteration storm / retry loop / forced-read cycle, capacity / throttling / OOM warning, " +
		"timing / span / latency observation, config drift / version mismatch, business audit trail, " +
		"informational breadcrumb, or any combination. " +
		"Anything classified by the registry below is FACTUAL CONTEXT (terminology priming when a known signature matches), " +
		"NOT a verdict that constrains your reading. " +
		"If the registry's labels do not fit what you actually see in the " + inputNoun + ", trust your reading and use the " +
		inputNoun + "'s own vocabulary — do NOT force-fit into a class that does not match.\n\n")

	if len(detected) > 0 {
		b.WriteString("### Detected Patterns\n")
		b.WriteString("The raw " + inputNoun + " contains one or more well-known signatures:\n\n")
		for _, d := range detected {
			label := d.HumanLabel()
			if label == "" {
				continue
			}
			b.WriteString("- **" + label + "**")
			if sig := strings.TrimSpace(d.MatchedSignature); sig != "" {
				flat := strings.ReplaceAll(strings.ReplaceAll(sig, "\n", " "), "\r", " ")
				b.WriteString(" — matched: `" + flat + "`")
			}
			b.WriteString("\n")
		}
		// 2026-05-10 user-intent-aligned reframe.
		//
		// Three structural fixes here, all rooted in the user red-line
		// "user-question intent drives the answer focus, the system
		// must NOT correct user intent":
		//
		//   1. Removed "USE THESE EXACT TERMS rather than paraphrasing"
		//      — that phrase forced terminology even when the user
		//      asked about something else (timing / scope / audit /
		//      etc.). Replaced with intent-conditional guidance.
		//
		//   2. Replaced "VOCABULARY AID, not a topic redirect" with
		//      "FACTUAL CONTEXT (what the runtime / detector
		//      classified)". The "VOCABULARY AID" framing was too
		//      passive; "FACTUAL CONTEXT" makes clear the labels are
		//      data, not lexical hints, while still leaving the user
		//      question in charge.
		//
		//   3. Reordered evidence priority — formerly listed function
		//      names FIRST, which biased models to interpret arg
		//      dumps (e.g. (0x0)) as the primary signal and override
		//      the explicit runtime classification. New ordering puts
		//      message text + classification first when the question
		//      is about failure KIND, with frames / args presented as
		//      observation-only at the time of capture (concrete
		//      example + explicit "NOT diagnostic of failure KIND").
		//
		// Forensic anchor: 2026-05-10 sweep b3jmwty80
		// logtri_goroutine_dump — model misread "concurrent map
		// writes" as "nil map writes" because (0x0) arg interpretation
		// dominated the classification. The Detected Patterns section
		// gave the right canonical label but framed it as optional
		// vocabulary; the model treated it as one term among many and
		// chose its own.
		b.WriteString("\nThese canonical labels are FACTUAL CONTEXT (what the runtime / detector classified). " +
			"User-question intent drives the answer focus:\n\n" +
			"  - When the user asks about FAILURE KIND (root cause, common problem, what went wrong, what kind of error), " +
			"the canonical label above IS the runtime's authoritative answer for the failure kind — use the term verbatim.\n" +
			"  - When the user asks about anything ELSE (timing, scope, who, when, frequency, audit, " +
			"performance characterisation, business behaviour), let that intent drive — frames / timestamps / signals are richer for those dimensions, " +
			"with the classification as supplementary fact only. Do NOT force the canonical term into an answer that does not need it.\n\n" +
			"Ground details from the " + inputNoun + "'s own content (" + contentDescriptor + "). " +
			"Do NOT search the surrounding repository for code that happens to resemble names mentioned in the " + inputNoun + " " +
			"unless a current cited code line explicitly proves the connection. Repository code is supplementary.\n\n" +
			"Argument-dump values shown alongside frames (e.g. `(0x0)`, `null`, pointer-literal tuples, register snapshots) are observation-only " +
			"— they record register state at the moment of capture. They are NOT diagnostic of WHAT KIND of failure the runtime classified " +
			"(the canonical label above is). They become useful only when the user asks about state at the time of capture.\n\n")
		return b.String()
	}
	// Empty — unknown / business-domain regime. Modality-neutral guidance
	// that does NOT presuppose the user is asking about a failure.
	// 2026-05-10 strengthened broad-coverage version: spans crash,
	// resource / capacity, concurrency / timing, config / version,
	// connectivity / dependency, build / test / deployment, security /
	// policy, behavioural / audit, AI / agent / tool-call iteration,
	// and clean-baseline shapes — across whatever language / runtime /
	// application stack the user's repo happens to use. The registry
	// is enumeration-shaped by construction (finite regex patterns);
	// this branch is the catch-all that MUST be modality-agnostic and
	// shape-agnostic. The list below is intentionally broad and
	// non-exhaustive: its job is to widen the model's mental space,
	// not to be a checklist.
	b.WriteString("### Pattern Classification\n")
	b.WriteString("The raw " + inputNoun + " did not match any cross-language / cross-platform standard signature in the registry. " +
		"This is NORMAL for many real-world " + inputNoun + " shapes — the registry covers a finite set of canonical crash / runtime-failure signatures, " +
		"and useful " + inputNoun + "s come in many other shapes. The list below is non-exhaustive guidance for the kinds of patterns " +
		"the " + inputNoun + " may show; READ the " + inputNoun + "'s own content first and form the classification yourself from what you see:\n" +
		"  - Failure / crash patterns the registry didn't recognise — application-specific panic format, custom exception type, " +
		"non-stdlib stack frames, runtime-specific abort signature, sanitizer-style output the registry doesn't index\n" +
		"  - Iteration / retry / feedback-loop patterns — the same step repeating with the same complaint across many entries, " +
		"forced-read or forced-action cycles, exponential-backoff cascades, deadletter / poison-message accumulation, agent or LLM " +
		"tool-call retry storms\n" +
		"  - Resource / capacity / throttling / budget signals — quota or rate-limit exhaustion, OOMKilled / out-of-memory abort, " +
		"disk / inode full, file-descriptor / handle exhaustion, connection-pool / thread-pool saturation, GC pressure or pause warnings, " +
		"swap thrash, cgroup / container limit reached\n" +
		"  - Concurrency / ordering / timing / latency observations — lock contention, replication lag, ordering anomalies, " +
		"timeout cascades, heartbeat miss, slow-query / N+1, scheduler starvation, span / trace duration spikes\n" +
		"  - Configuration / drift / version-skew patterns — settings differing from a reference, schema-version mismatch, " +
		"feature-flag inconsistency, environment-variable shape difference, missing / wrong default, RBAC / permission denial, " +
		"TLS / cipher / cert handshake mismatch\n" +
		"  - Connectivity / dependency / discovery patterns — DNS resolution failure, endpoint mismatch, downstream service unavailable, " +
		"connection churn / leak / reset-by-peer, broken pipe, network partition, service-discovery cache stale\n" +
		"  - Build / test / deployment / orchestration patterns — CI / build failure cascade, test flake / hang / xfail, " +
		"container start failure (image pull / exec format / entrypoint), pod CrashLoopBackoff / Evicted / Pending, " +
		"cron schedule miss / job overlap, rolling-update partial failure\n" +
		"  - Security / policy / integrity patterns — auth denial, policy violation, checksum / signature / hash failure, " +
		"PII / secret-leak indicator, suspicious-access anomaly, audit-policy fault, encryption / decryption failure\n" +
		"  - Database / persistence / cache patterns — connection pool exhaustion, transaction lock timeout, deadlock graph, " +
		"replication lag, sequential-scan warning, cache stampede / miss-storm, key eviction surge\n" +
		"  - Build / runtime hot-path / GC / JIT patterns — JIT compile bailout, deopt cascade, GC overhead percentage, " +
		"allocation hot path, fragmentation / large-page churn\n" +
		"  - AI / agent / tool-call patterns — LLM iteration loop, tool-call retry storm, prompt-context-window overflow, " +
		"hallucination indicator (claimed X but tool returned Y), prompt-injection detection, model-cost / spend anomaly\n" +
		"  - Behavioural / business / audit observations — domain event sequence, audit trail, metric drift / KPI anomaly, " +
		"workflow step ordering, debug breadcrumb, sampled-trace observation\n" +
		"  - Informational baseline — no anomaly at all (clean run, performance baseline, expected debug output)\n\n" +
		"This taxonomy covers many common shapes but is deliberately incomplete — your repository's stack, language, runtime, " +
		"orchestration platform, and business domain may produce shapes outside it. Read the " + inputNoun + "'s own content (" + emptyDescriptor + ") " +
		"and address the user's question from what you actually see. " +
		"Do NOT force-fit into any category above when none of them describes what the " + inputNoun + " shows; coin a description in the " +
		inputNoun + "'s own vocabulary instead. " +
		"Do NOT speculate that any name in the " + inputNoun + " resembles an unrelated repository symbol just because the names look similar. " +
		"When the " + inputNoun + " carries identifiers that are NOT defined in this repository, " +
		"treat them as opaque external names: cite them verbatim and address the user's question from the " +
		inputNoun + "'s own causality / evidence, not from repository introspection.\n\n")
	return b.String()
}

// sanitiseSectionForLLM applies ac.TypedDenials.Sanitise to a
// pre-rendered prompt section so denied tokens (paths the input
// frame referenced but the typed gate could not corroborate, symbols
// the oracle marked as unknown, etc.) get redacted to neutral
// "<unverified-...>" markers BEFORE the section reaches the LLM
// prompt assembly.
//
// L2 of the negative-knowledge enforcement pyramid (R3 second-axis):
//   - L1 tool-call gate (read_file / grep / repo_map) refuses calls
//     naming the same tokens
//   - L2 (this) — prevents the LLM from extracting the tokens out of
//     prose context (frame.Raw / attached log body / trace tags) to
//     bypass L1
//   - L3 answer validator catches answer prose that names denied
//     tokens without an "unverified" caveat
//
// nil-safe: empty TypedDenials passes the section through verbatim
// (zero behavioural regression for runs without any input-side
// gate firing — the dominant case).
func sanitiseSectionForLLM(section string, ac *types.AgentContext) string {
	if ac == nil || ac.TypedDenials == nil || ac.TypedDenials.Len() == 0 {
		return section
	}
	return ac.TypedDenials.Sanitise(section)
}

const (
	attachedLogInlineCap = 4 * 1024 // ≤ 4 KB → inline whole body
	attachedLogHeadCap   = 2 * 1024 // head preview when blobbed
	attachedLogTailCap   = 1 * 1024 // tail preview when blobbed
)

type attachedRuntimeTriageState int

const (
	attachedTriageUnavailable attachedRuntimeTriageState = iota
	attachedTriageProducer
	attachedTriageStructured
)

func attachedLogTriageState(ac *types.AgentContext) attachedRuntimeTriageState {
	if ac != nil && ac.AgentName == types.AgentLogTriager {
		return attachedTriageProducer
	}
	if ac != nil && ac.LogTriage != nil {
		return attachedTriageStructured
	}
	return attachedTriageUnavailable
}

func attachedTraceTriageState(ac *types.AgentContext) attachedRuntimeTriageState {
	if ac != nil && ac.AgentName == types.AgentPerfTriager {
		return attachedTriageProducer
	}
	if ac != nil && ac.PerfTrace != nil {
		return attachedTriageStructured
	}
	return attachedTriageUnavailable
}

func attachedLogPreamble(state attachedRuntimeTriageState) string {
	lineNote := "Every line in the fenced block carries an artifact-local gutter `N│`; " +
		"use that N only as the attached-log line number, not as a repository source citation. " +
		"Source file:line frames remain the literal file:line tokens inside the log text.\n\n"
	switch state {
	case attachedTriageProducer:
		return "The user attached the runtime log below alongside their question. " +
			"Prepare a structured summary for this raw log, preserving stack-frame order, error chains, and literal file:line anchors. " +
			lineNote
	case attachedTriageStructured:
		return "The user attached the runtime log below alongside their question. " +
			"A structured log summary is already available above and is the preferred source for stack frames, file:line anchors, function names, and cause chains. Consult the raw log only for literal message text, artifact line numbers, or frames not visible in the structured summary. " +
			lineNote
	default:
		return "The user attached the runtime log below alongside their question. " +
			"No structured log summary is available in this prompt. Treat this raw log as unparsed input: establish any stack-frame file:line anchors yourself before relying on them later. " +
			lineNote
	}
}

func attachedTracePreamble(state attachedRuntimeTriageState) string {
	lineNote := "Every line in the fenced block carries an artifact-local gutter `N│`; " +
		"use that N only as the attached-trace line number / event row, not as a repository source citation. " +
		"Trace timestamps, span names, and source-frame tokens remain the literal text after the gutter.\n\n"
	switch state {
	case attachedTriageProducer:
		return "The user attached the performance trace below alongside their question. " +
			"Prepare a structured summary for this raw trace, capturing hotspots, stalls, frame spans, and startup or jank envelopes. " +
			lineNote
	case attachedTriageStructured:
		return "The user attached the performance trace below alongside their question. " +
			"A structured performance summary is already available above and is the preferred source for hotspots, stalls, frame spans, and startup or jank envelopes. Consult the raw trace only for literal timestamps, thread names, artifact line numbers, or event tags not visible in the structured summary. " +
			lineNote
	default:
		return "The user attached the performance trace below alongside their question. " +
			"No structured performance summary is available in this prompt. Treat this raw trace as unparsed input: derive hotspots, stalls, timestamps, and thread/event anchors yourself before relying on them later. " +
			lineNote
	}
}

func normalizeAttachedArtifactText(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	return raw
}

func renderAttachedArtifactLines(raw string, startLineNo int) string {
	if raw == "" {
		return ""
	}
	return textfmt.LineGutter(strings.Split(raw, "\n"), startLineNo)
}

type attachedArtifactPreview struct {
	head          string
	tail          string
	tailStartLine int
	elidedBytes   int
}

func buildAttachedArtifactPreview(raw string) attachedArtifactPreview {
	headEnd := attachedLogHeadCap
	if headEnd > len(raw) {
		headEnd = len(raw)
	}
	if idx := strings.LastIndex(raw[:headEnd], "\n"); idx > 0 {
		headEnd = idx
	}
	tailStart := len(raw) - attachedLogTailCap
	if tailStart < headEnd {
		tailStart = headEnd
	}
	if tailStart > 0 && tailStart < len(raw) {
		if idx := strings.Index(raw[tailStart:], "\n"); idx >= 0 {
			tailStart += idx + 1
		}
	}
	if tailStart >= len(raw) {
		tailStart = len(raw) - attachedLogTailCap
		if tailStart < headEnd {
			tailStart = headEnd
		}
	}
	head := raw[:headEnd]
	tail := raw[tailStart:]
	return attachedArtifactPreview{
		head:          head,
		tail:          tail,
		tailStartLine: 1 + strings.Count(raw[:tailStart], "\n"),
		elidedBytes:   tailStart - headEnd,
	}
}

func renderAttachedArtifactPreviewBlock(preview attachedArtifactPreview, blobPath string) string {
	var b strings.Builder
	b.WriteString("```text\n")
	if head := renderAttachedArtifactLines(preview.head, 1); head != "" {
		b.WriteString(head)
		b.WriteByte('\n')
	}
	if blobPath != "" {
		fmt.Fprintf(&b, "... [%d bytes elided — read %s for exact middle artifact-line anchors] ...\n", preview.elidedBytes, blobPath)
	} else {
		fmt.Fprintf(&b, "... [%d bytes elided] ...\n", preview.elidedBytes)
	}
	if tail := renderAttachedArtifactLines(preview.tail, preview.tailStartLine); tail != "" {
		b.WriteString(tail)
		b.WriteByte('\n')
	}
	b.WriteString("```")
	return b.String()
}

// formatAttachedLog renders the user-attached runtime log excerpt as a
// prompt section. Two size regimes keep the overall prompt bounded:
//
//   - ≤ attachedLogInlineCap (4 KB): inline the whole body with
//     artifact-local line gutters. Typical panic / short exception /
//     single-stack sanitizer report lands here — LLM sees everything
//     without indirection, including stable attached-log line numbers.
//   - > attachedLogInlineCap: write the full body to
//     `<workDir>/attached_log.txt` (mirrors the tool/blob pattern),
//     inline a head+tail preview with artifact-local line gutters for
//     visible lines, and instruct the LLM to use `read_file` on the
//     blob path for paginated access to the middle. The explorer has
//     read_file in its tool allowlist. When the structured triage
//     bundle is absent, the preamble tells the LLM this raw artifact
//     is unparsed instead of implying pre-stage success.
//
// Returns "" for empty input so the caller can skip the section.
// Falls back to head+tail inline when workDir is empty or the blob
// write fails (no-op degrade, no error surfaces to the caller).
func formatAttachedLog(raw, workDir string, state attachedRuntimeTriageState) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	raw = normalizeAttachedArtifactText(raw)
	preamble := attachedLogPreamble(state)

	if len(raw) <= attachedLogInlineCap {
		return preamble + "```text\n" + renderAttachedArtifactLines(raw, 1) + "\n```"
	}

	// Over cap — attempt blob offload.
	blobPath := ""
	if workDir != "" {
		target := filepath.Join(workDir, AttachedLogBlobName)
		if err := os.WriteFile(target, []byte(raw), 0o644); err == nil {
			blobPath = target
		} else {
			logging.Warning("[context] attached-log blob write failed: %v (falling back to head+tail)", err)
		}
	}

	preview := buildAttachedArtifactPreview(raw)

	if blobPath != "" {
		return preamble +
			fmt.Sprintf("Total log size: %d bytes. Preview below shows head + tail with artifact-local line gutters; "+
				"the middle (%d B) is elided. The complete log is saved to `%s` — "+
				"use `read_file` with offset+limit on that path to paginate through the "+
				"elided region if you need exact line anchors beyond the preview.\n\n",
				len(raw), preview.elidedBytes, blobPath) +
			renderAttachedArtifactPreviewBlock(preview, blobPath)
	}

	// Fallback: no workDir or write failed. Degrade to head+tail with
	// an elision marker so the caller still gets bounded rendering.
	return preamble +
		fmt.Sprintf("Total log size: %d bytes; showing head + tail with artifact-local line gutters, middle elided.\n\n",
			len(raw)) +
		renderAttachedArtifactPreviewBlock(preview, "")
}

// formatAttachedTrace mirrors formatAttachedLog for performance traces
// but keeps the blob filename and preamble specific to the trace
// channel. This avoids prompt leakage from the runtime-log wording
// and prevents blob-path collisions when a run carries both
// attachments.
func formatAttachedTrace(raw, workDir string, state attachedRuntimeTriageState) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	raw = normalizeAttachedArtifactText(raw)
	preamble := attachedTracePreamble(state)

	if len(raw) <= attachedLogInlineCap {
		return preamble + "```text\n" + renderAttachedArtifactLines(raw, 1) + "\n```"
	}

	blobPath := ""
	if workDir != "" {
		target := filepath.Join(workDir, AttachedTraceBlobName)
		if err := os.WriteFile(target, []byte(raw), 0o644); err == nil {
			blobPath = target
		} else {
			logging.Warning("[context] attached-trace blob write failed: %v (falling back to head+tail)", err)
		}
	}

	preview := buildAttachedArtifactPreview(raw)

	if blobPath != "" {
		return preamble +
			fmt.Sprintf("Total trace size: %d bytes. Preview below shows head + tail with artifact-local line gutters; "+
				"the middle (%d B) is elided. The complete trace is saved to `%s` — "+
				"use `read_file` with offset+limit on that path to paginate through the "+
				"elided region if you need exact event-line anchors beyond the preview.\n\n",
				len(raw), preview.elidedBytes, blobPath) +
			renderAttachedArtifactPreviewBlock(preview, blobPath)
	}

	return preamble +
		fmt.Sprintf("Total trace size: %d bytes; showing head + tail with artifact-local line gutters, middle elided.\n\n",
			len(raw)) +
		renderAttachedArtifactPreviewBlock(preview, "")
}

// formatLogTriageStructured renders the validated LogBundle as an
// audit-friendly prompt section. Complements formatAttachedLog (raw
// log body) by giving downstream agents — especially the finalizer
// rendering multi-level Cause chains — a pre-structured view that
// does not require re-parsing the raw text.
//
// Audit discipline the renderer follows:
//
//  1. Provenance is explicit. Frames whose File survived os.Stat
//     verification in the repo are marked `★ resolved`; frames with
//     File cleared (hallucination-filtered) show no star. The
//     accompanying legend tells the LLM which frames may be cited as
//     file:line and which must stay contextual.
//  2. Raw field is always rendered. Every frame carries the original
//     log text it was extracted from; an auditor reading a trace can
//     cross-check the LLM's downstream answer against what the log
//     actually said at each frame, without re-fetching the raw body.
//  3. Confidence is surfaced. The LLM sees the 0.0-1.0 self-assessed
//     certainty per frame and can soften its own claims when the
//     confidence is below the 0.6 floor.
//  4. Signals and Coverage are explicit. The LLM knows what
//     categorical classification happened and how much of the log
//     was left unstructured in Residue.
//  5. Cause chain depth is visible. Nested "caused by" blocks show
//     exception wrapping (Java Caused-by / Rust #[source] / Python
//     __cause__) at the correct nesting level — no prose
//     re-interpretation needed.
//  6. No internal-pipeline leakage. The renderer does NOT mention
//     the extractor agent, ValidateBundle, stage boundaries, or any
//     scaffolding detail — just "this data was validated" + the
//     provenance flags that back the claim.
//
// Returns "" when bundle is nil or entirely empty (zero Meta + zero
// Errors + zero Residue). Caller uses that as the skip signal.
// renderLogTriageFrameDrift surfaces per-frame drift verdicts as a
// prompt section. Only fires when the locator is non-nil (single-
// shot CLI flows pass nil → graceful degrade) AND at least one
// frame has a drift status that warrants warning the model
// (LineDrift / TailRename / FileMoved / Unmappable). When every
// frame is None (perfect agreement) OR Unknown (insufficient
// signal), the section is suppressed — the LLM does not need a
// "frames are fine" preamble that wastes prompt budget.
//
// Frames are aggregated by drift status so the LLM gets one
// directive per status class instead of a per-frame deluge.
//
// Returns "" when nothing to surface; the caller appends only when
// non-empty.
func renderLogTriageFrameDrift(bundle *types.LogBundle, locator types.SymbolLocator) string {
	if bundle == nil {
		return ""
	}
	frames := make([]types.LogFrame, 0, 8)
	types.WalkLogFrames(bundle, func(frame types.LogFrame) {
		frames = append(frames, frame)
	})
	return renderRuntimeFrameDriftWarning(frames, locator)
}

func renderPerfTriageFrameDrift(bundle *types.PerfBundle, locator types.SymbolLocator) string {
	if bundle == nil {
		return ""
	}
	return renderRuntimeFrameDriftWarning(bundle.LogFrames(), locator)
}

func renderRuntimeFrameDriftWarning(frames []types.LogFrame, locator types.SymbolLocator) string {
	if locator == nil || len(frames) == 0 {
		return ""
	}
	// Collect frames that warrant warning. Aggregate by drift status
	// so the section renders compactly.
	bucket := make(map[types.FrameDriftStatus][]types.LogFrame)
	for _, frame := range frames {
		if frame.File == "" || frame.Func == "" {
			continue
		}
		drift := logtriage.DetectDriftForFrame(frame, locator)
		switch drift.Status {
		case types.DriftStatusLineDrift,
			types.DriftStatusTailRename,
			types.DriftStatusFileMoved,
			types.DriftStatusUnmappable:
			bucket[drift.Status] = append(bucket[drift.Status], frame)
		}
	}
	if len(bucket) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("### Frame ↔ current-code drift warning\n")
	b.WriteString("The attached log's frames have been compared against current repository code. " +
		"The following frames may be MISLEADING if you read the current file at the cited line to explain the log's behaviour — " +
		"the current content may belong to a different build / version / commit / synthetic context than what the log captured. " +
		"Treat these frames as OPAQUE OBSERVATION: cite the log's own message / classification / sequence as the authoritative " +
		"signal, and only use current-repo content to explain the log when an explicit grounded evidence anchor proves the connection.\n\n")

	// Order the buckets by severity so the most-misleading drift
	// shapes appear first.
	order := []types.FrameDriftStatus{
		types.DriftStatusUnmappable,
		types.DriftStatusTailRename,
		types.DriftStatusFileMoved,
		types.DriftStatusLineDrift,
	}
	for _, status := range order {
		frames, ok := bucket[status]
		if !ok || len(frames) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- **%s** (%d frame(s)):\n", driftStatusHumanLabel(status), len(frames))
		// Cap the per-bucket frame list so a log with 100 parallel
		// goroutines doesn't bloat the prompt.
		const perBucketCap = 6
		for i, frame := range frames {
			if i >= perBucketCap {
				fmt.Fprintf(&b, "  - ... and %d more\n", len(frames)-perBucketCap)
				break
			}
			fmt.Fprintf(&b, "  - %s:%d %s — %s\n", frame.File, frame.Line, frame.Func, driftStatusGuidance(status))
		}
	}
	b.WriteString("\nDo NOT use the current content of these files at these lines to construct the answer's causal story. " +
		"Anchor the answer in the log's verbatim message (the runtime's authoritative classification) and the log's own " +
		"observed sequence. If you genuinely need to inspect current code at a drifted location for adjacent context, " +
		"treat any read content as suggestive only — never as the definitive explanation of the log's failure.\n\n")
	return b.String()
}

// driftStatusHumanLabel returns the LLM-facing label for a drift
// status. Bilingual-neutral / no internal jargon (R6).
func driftStatusHumanLabel(s types.FrameDriftStatus) string {
	switch s {
	case types.DriftStatusLineDrift:
		return "Line drift (same file + function, line numbers shifted)"
	case types.DriftStatusTailRename:
		return "Function renamed (same file, function name no longer present)"
	case types.DriftStatusFileMoved:
		return "Code moved across files (function exists elsewhere in current repo)"
	case types.DriftStatusUnmappable:
		return "Unmappable (function and file cannot be located in current repo)"
	}
	return string(s)
}

// driftStatusGuidance is the per-status one-liner suggesting how the
// LLM should treat frames in that bucket.
func driftStatusGuidance(s types.FrameDriftStatus) string {
	switch s {
	case types.DriftStatusLineDrift:
		return "function exists but current line numbers differ from the log — line-number citations from current code may be wrong"
	case types.DriftStatusTailRename:
		return "function is not at this name in current code — current content at the path is unrelated to the log's claim"
	case types.DriftStatusFileMoved:
		return "function is in a different file now — the log's path predates a refactor"
	case types.DriftStatusUnmappable:
		return "neither file nor function can be matched in current code — synthetic, removed, or external"
	}
	return "drift detected — treat as observation only"
}

func formatLogTriageStructured(bundle *types.LogBundle, locator types.SymbolLocator) string {
	if bundle == nil {
		return ""
	}
	if bundle.Meta.Lang == "" && len(bundle.Errors) == 0 &&
		len(bundle.Observations) == 0 &&
		len(bundle.Residue.UnknownChunks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The attached runtime log was parsed into the structured view below. " +
		"Prefer this view for citing frames, reading typed operational observations, and reasoning about the error chain. " +
		"When the validated frame set is already authoritative, downstream stages " +
		"may omit the raw log section to avoid competing interpretations of runtime-only " +
		"tuple payloads; rely on the structured bundle and residue snippets below.\n\n")
	b.WriteString("Stack-frame argument annotations attached by the runtime artifact's panic / exception / traceback dumper " +
		"(argument-register or pointer-literal tuples, native-method placeholders, locals snapshots — exact form varies by " +
		"language and runtime) are observation-only encodings. Do NOT map their positional values to a specific receiver, " +
		"source parameter, caller-side provenance, or exact downstream branch unless a current cited code line explicitly " +
		"proves that mapping.\n\n")

	// Fix-C (2026-05-10) ── Primary Error Signal ──
	// The runtime emits a verbatim error MESSAGE (e.g. "fatal
	// error: concurrent map writes" / "RuntimeError: config
	// unavailable") before the stack frames are captured. That
	// message is the highest-confidence diagnostic signal — when
	// frames fail to resolve (synthetic / generated / vendored
	// code), the LLM should still anchor on the message rather
	// than concocting a theory from unresolved file:line refs.
	//
	// This section renders BEFORE Detected Patterns + Errors tree
	// so the LLM sees the message at maximum salience. When N
	// goroutines/threads hit the SAME (file, line) coordinate,
	// surface that as a corroborating "parallel concurrency"
	// signal — strong evidence of a race condition independent
	// of the BugClass detector's first-match-only behaviour.
	//
	// Forensic anchor: 2026-05-10 sweep b3qhs96tz logtri_goroutine_dump
	// — model misread "concurrent map writes" → "nil map write"
	// because frame list ranked higher than error message.
	b.WriteString(renderPrimaryErrorSignal(bundle))

	// ── Detected patterns (cross-language, deterministic) ──
	// Surfaces canonical bilingual terminology for any well-known
	// signature found in the raw log. When no pattern fires, emit
	// the unknown / business-domain guidance so the LLM addresses
	// the user's question using log-content terminology rather than
	// inventing a category or speculating from unrelated repository
	// symbols.
	b.WriteString(renderBugClassesSection(bundle.Meta.BugClasses, "log"))

	// ── Front-loaded external-source directive ────────────────
	//
	// Session 22 fix: when the log_triage bundle carries ≥1 Error
	// but ZERO ResolvedFiles, the attached log's stack frames do
	// not resolve to any file in this repo — the customer-trace
	// pattern where a Python/Node/foreign traceback is pasted into
	// a Go repo's REPL. Without an upfront directive, the LLM
	// spends a whole explore dispatch trying to ground log-frame
	// literals against repo code, finds nothing, and at the
	// finalize stage the emit_answer_document literal-grounding
	// gate rejects the citation — burning a full cycle before the
	// LLM learns to leave external observations uncited (observed:
	// 16 min on the partial eval case).
	//
	// Surfacing the directive at the TOP of the log-triage section
	// means every agent (analyzer / explorer / extractor /
	// finalizer) sees it in iter 0 and can act before any tool
	// call is burned on a dead-end.
	if bundle.IsExternalSource() {
		b.WriteString("⚠ **External-source log**: the attached log's stack frames do NOT resolve to any file in this repo (resolved_files=0). The answer must come from the log's own semantics — do NOT open repo files hoping to ground the log's frame literals, they are not there.\n")
		b.WriteString("  - For a BlockScalar answer (single literal, optionally with config-key facet), leave the value uncited and state in `summary` that the literal is drawn from the attached log (no grounded repo source).\n")
		b.WriteString("  - The literal-grounding gate on emit_answer_document rejects citations whose cited line does NOT contain the literal; do not borrow an unrelated repo citation just to satisfy a source habit.\n")
		b.WriteString("  - For an ordered hop-chain block or a summary-led explanation answer, cite log content by paraphrasing frames, not by inventing file:line anchors in this repo.\n")
		b.WriteString("  - For an answer-symbol slate or a multi-topic anchor skeleton, set symbols_completeness=\"unknown\" and omit items[] entirely — those channels require repo-grounded file:line anchors, which external-log content cannot satisfy. The summary prose is the answer.\n\n")
	}

	// Frame-drift warning (Step 2 2026-05-10). Detects the self-
	// referential synthetic-log trap where the attached log's frame
	// paths resolve to current-repo files but the function name or
	// line claimed in the frame does not match what current code has
	// at that location. The 2026-05-10 logtri_goroutine_dump failure
	// is the canonical case: a synthetic Go panic dumps frames
	// pointing at `internal/agent/analyzer.go:100` (which is a real
	// codrax file) for a function `main.writeSession` that does not
	// exist in current code; the model read the real file at that
	// line and confabulated a story unrelated to the actual panic.
	//
	// Surfacing drift status here lets the model see "this frame's
	// path is in the repo but the symbol does not match what current
	// code carries there — treat the frame as observation-only" and
	// avoid grounding the answer against the misleading current
	// content.
	if section := renderLogTriageFrameDrift(bundle, locator); section != "" {
		b.WriteString(section)
	}

	// ── Meta block ────────────────────────────────────────────
	if bundle.Meta.Lang != "" {
		fmt.Fprintf(&b, "- Language: %s\n", bundle.Meta.Lang)
	}
	if len(bundle.Meta.Signals) > 0 {
		sigs := make([]string, len(bundle.Meta.Signals))
		for i, s := range bundle.Meta.Signals {
			sigs[i] = string(s)
		}
		fmt.Fprintf(&b, "- Signals: %s\n", strings.Join(sigs, ", "))
	}
	fmt.Fprintf(&b, "- Coverage: %.2f", bundle.Coverage)
	residueBytes := 0
	for _, c := range bundle.Residue.UnknownChunks {
		residueBytes += len(c)
	}
	if residueBytes > 0 {
		fmt.Fprintf(&b, " (unstructured residue: %d bytes in %d chunks)",
			residueBytes, len(bundle.Residue.UnknownChunks))
	}
	b.WriteString("\n")
	if bundle.IntentHint != "" {
		fmt.Fprintf(&b, "- Intent hint: %s\n", bundle.IntentHint)
	}
	b.WriteString("\n")

	// ── Operational observations ───────────────────────────────
	//
	// Not every useful runtime artifact is an exception tree. Codrax
	// self-logs, product telemetry, and customer operation logs often
	// expose "answer was retried", "line mapping drifted", "topic
	// mismatch was reported", or "state changed" without a stack
	// frame. Render those as typed observations so downstream agents
	// can align the investigation to the user's diagnostic ask without
	// treating unknown_chunks as a hidden evidence source.
	if len(bundle.Observations) > 0 {
		b.WriteString("### Operational observations\n\n")
		b.WriteString("These are structured non-stack facts extracted from the log. " +
			"They are runtime observations, not repo file:line citations by themselves. " +
			"When the current request asks whether an observed issue still exists, answer in two lanes: what the log observed, and what current code evidence proves now.\n\n")
		for i, obs := range bundle.Observations {
			fmt.Fprintf(&b, "  %d. kind=%s", i+1, obs.Kind)
			if obs.Severity != "" {
				fmt.Fprintf(&b, " severity=%s", obs.Severity)
			}
			if obs.LineStart > 0 {
				if obs.LineEnd > obs.LineStart {
					fmt.Fprintf(&b, " log_lines=%d-%d", obs.LineStart, obs.LineEnd)
				} else {
					fmt.Fprintf(&b, " log_line=%d", obs.LineStart)
				}
			}
			fmt.Fprintf(&b, " diagnostic=%t confidence=%.2f", obs.Diagnostic, obs.Confidence)
			if obs.Subject != "" {
				fmt.Fprintf(&b, " subject=`%s`", obs.Subject)
			}
			fmt.Fprintf(&b, "\n     summary: %s\n", truncateForPrompt(obs.Summary, 240))
			if obs.Evidence != "" {
				fmt.Fprintf(&b, "     evidence: %s\n", truncateForPrompt(obs.Evidence, 240))
			}
		}
		b.WriteString("Observation log_line/log_lines are artifact-local anchors from the attached log, not repository source citations.\n")
		b.WriteString("\n")
	}

	// ── Errors tree ────────────────────────────────────────────
	if len(bundle.Errors) > 0 {
		if len(bundle.Errors) > 1 {
			fmt.Fprintf(&b, "### Errors (%d parallel snapshots)\n\n", len(bundle.Errors))
		} else {
			b.WriteString("### Error\n\n")
		}
		for i := range bundle.Errors {
			renderLogError(&b, &bundle.Errors[i], 0, i+1)
			b.WriteString("\n")
		}
	}

	// ── Call Chain (innermost → outer) ─────────────────────────
	//
	// Session 22 fix F3.1. The Errors tree renders frames as sibling
	// bullets in stack order, but the caller→callee relationship is
	// implicit: the LLM has to know Go panic / Java stack / Python
	// traceback conventions to read which frame called which. When the
	// attached log is a panic / crash, the frames ARE the call chain
	// the answer's diagram should draw — rendering that chain
	// explicitly, with roles spelled out, gives the LLM a ready-made
	// skeleton and stops it from inventing callers from the ranker's
	// "structurally relevant" neighbours.
	//
	// Gate on Signals ∩ {panic, crash}: stack-frame ordering is
	// semantically meaningful for runtime crashes but NOT for e.g.
	// validation / logic / db error logs where frames may be a
	// middleware chain rather than a failure call path.
	if section := renderLogCallChain(bundle); section != "" {
		b.WriteString(section)
	}

	// ── Unknown chunks (brief) ─────────────────────────────────
	if len(bundle.Residue.UnknownChunks) > 0 {
		b.WriteString("### Unstructured residue\n\n")
		b.WriteString("The extractor could not map these chunks to the error tree; " +
			"they are included verbatim for context only and are NOT citeable anchors:\n\n")
		for i, chunk := range bundle.Residue.UnknownChunks {
			trimmed := strings.TrimSpace(chunk)
			if trimmed == "" {
				continue
			}
			fmt.Fprintf(&b, "  %d. %s\n", i+1, truncateForPrompt(trimmed, 200))
		}
		b.WriteString("\n")
	}

	// ── Provenance legend (audit anchor) ───────────────────────
	b.WriteString("### Provenance legend\n\n")
	b.WriteString("- `★ resolved` on a frame means the file path was verified to " +
		"exist in the repository. These frames are safe to cite as `file:line` " +
		"in answers and evidence.\n")
	b.WriteString("- Frames without `★ resolved` had their File cleared because " +
		"the path could not be verified in the repo (build-prefix strip failed, " +
		"path escaped the repo, file does not exist) or their confidence fell " +
		"below the 0.6 cite floor. Use them for context (function name, package, " +
		"error type) but do NOT cite their line numbers.\n")
	b.WriteString("- Each frame carries the `raw:` original log text. Cross-check " +
		"this against any quote you attribute to that frame in your answer.\n")

	return strings.TrimRight(b.String(), "\n")
}

// formatPerfTriageStructured renders the validated PerfBundle as an
// audit-friendly prompt section. Mirrors formatLogTriageStructured
// for the performance channel: gives downstream agents a structured
// view of jank windows, main-thread stalls, cold-start timing, and
// signals without re-parsing the raw HiTrace / atrace text.
//
// Returns "" when bundle is nil or carries no actionable content
// (zero frames + zero janks + zero stalls + nil startup + zero
// residue). Caller skips the section.
func formatPerfTriageStructured(bundle *types.PerfBundle, locator types.SymbolLocator) string {
	if bundle == nil {
		return ""
	}
	if len(bundle.Frames) == 0 && len(bundle.Janks) == 0 &&
		len(bundle.Stalls) == 0 && bundle.Startup == nil &&
		len(bundle.Observations) == 0 &&
		len(bundle.Residue) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The attached performance trace was parsed into the structured view below. " +
		"Prefer this view for citing jank frames, stall symbols, and cold-start " +
		"measurements — the full raw trace is still in the next section for " +
		"context the structured schema did not capture. When the current request asks " +
		"whether an observed trace symptom still exists, answer in two lanes: what the trace " +
		"observed, and what current code evidence proves now.\n\n")

	// ── Detected patterns (same engine the log_triage section uses) ──
	// Always render — non-empty surfaces canonical terminology for
	// any well-known signature embedded in the trace stream
	// (deadlock, race, OOM, etc.); empty surfaces the unknown /
	// business-domain guidance so the LLM addresses the user's
	// question (whether that is failure cause, performance
	// characterisation, business audit, or any other intent) using
	// trace-content terminology rather than inventing a category or
	// speculating from unrelated repository symbols. Symmetric with
	// the log_triage section to avoid input-modality skew.
	b.WriteString(renderBugClassesSection(bundle.Meta.BugClasses, "trace"))

	if bundle.IsExternalSource() {
		b.WriteString("⚠ **External-source trace**: the attached trace's structured observations do NOT resolve to any file in this repo (resolved_files=0). The answer must come from the trace's own timing / jank / stall semantics — do NOT open repo files hoping to ground trace literals that are not in this checkout.\n")
		b.WriteString("  - For scalar or summary claims drawn directly from the trace, leave the claim uncited unless a current repo line literally states the same claim.\n")
		b.WriteString("  - Quote trace span names, tags, stall symbols, and timing values as runtime observations; do not invent file:line anchors in this repo.\n\n")
	}

	if section := renderPerfTriageFrameDrift(bundle, locator); section != "" {
		b.WriteString(section)
	}

	// Meta block
	if bundle.Meta.Source != "" {
		fmt.Fprintf(&b, "- Source: %s\n", bundle.Meta.Source)
	}
	if bundle.Meta.DurationMs > 0 {
		fmt.Fprintf(&b, "- Duration: %.1fms\n", bundle.Meta.DurationMs)
	}
	if bundle.Meta.AppPID != 0 {
		fmt.Fprintf(&b, "- App PID: %d\n", bundle.Meta.AppPID)
	}
	if len(bundle.Meta.Signals) > 0 {
		fmt.Fprintf(&b, "- Signals: %s\n", strings.Join(bundle.Meta.Signals, ", "))
	}
	if bundle.IntentHint != "" {
		fmt.Fprintf(&b, "- Intent hint: %s\n", bundle.IntentHint)
	}
	fmt.Fprintf(&b, "- Coverage: %.2f\n\n", bundle.Coverage)
	resolvedFile := make(map[string]bool, len(bundle.ResolvedFiles))
	for _, file := range bundle.ResolvedFiles {
		file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
		if file != "" {
			resolvedFile[file] = true
		}
	}

	// Startup envelope
	if bundle.Startup != nil {
		fmt.Fprintf(&b, "**Startup**: mode=%s", bundle.Startup.Mode)
		if bundle.Startup.AppLaunchMs > 0 {
			fmt.Fprintf(&b, " app_launch=%.1fms", bundle.Startup.AppLaunchMs)
		}
		if bundle.Startup.AbilityInitMs > 0 {
			fmt.Fprintf(&b, " ability_init=%.1fms", bundle.Startup.AbilityInitMs)
		}
		if bundle.Startup.FirstFrameMs > 0 {
			fmt.Fprintf(&b, " first_frame=%.1fms", bundle.Startup.FirstFrameMs)
		}
		b.WriteString("\n\n")
	}

	if len(bundle.Observations) > 0 {
		fmt.Fprintf(&b, "**Trace observations** (%d):\n", len(bundle.Observations))
		for i, obs := range bundle.Observations {
			label := strings.TrimSpace(obs.Subject)
			if label == "" {
				label = "observation"
			}
			fmt.Fprintf(&b, "  [%d] %s", i+1, label)
			if obs.LineStart > 0 {
				if obs.LineEnd > obs.LineStart {
					fmt.Fprintf(&b, " trace_lines=%d-%d", obs.LineStart, obs.LineEnd)
				} else {
					fmt.Fprintf(&b, " trace_line=%d", obs.LineStart)
				}
			}
			if obs.DurationMs > 0 {
				fmt.Fprintf(&b, " duration=%.3fms", obs.DurationMs)
			}
			if len(obs.Tags) > 0 {
				fmt.Fprintf(&b, " tags=%s", strings.Join(obs.Tags, " → "))
			}
			if obs.Summary != "" {
				fmt.Fprintf(&b, " — %s", obs.Summary)
			}
			if obs.Evidence != "" {
				fmt.Fprintf(&b, "\n      evidence: %s", obs.Evidence)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Janks — most actionable, list every entry up to schema cap.
	if len(bundle.Janks) > 0 {
		fmt.Fprintf(&b, "**Janks** (%d):\n", len(bundle.Janks))
		for i, j := range bundle.Janks {
			fmt.Fprintf(&b, "  [%d] start=%.1fms duration=%.1fms",
				i+1, j.StartTsMs, j.DurationMs)
			if j.TriggerSpan != "" {
				fmt.Fprintf(&b, " trigger=`%s`", j.TriggerSpan)
			}
			if j.Reason != "" {
				fmt.Fprintf(&b, " reason=%s", j.Reason)
			}
			b.WriteString("\n")
			if len(j.Tags) > 0 {
				fmt.Fprintf(&b, "      tags: %s\n", strings.Join(j.Tags, " → "))
			}
		}
		b.WriteString("\n")
	}

	// Stalls — main-thread blocking calls. file:line is citation-grade.
	if len(bundle.Stalls) > 0 {
		fmt.Fprintf(&b, "**Stalls** (%d):\n", len(bundle.Stalls))
		for i, s := range bundle.Stalls {
			fmt.Fprintf(&b, "  [%d] start=%.1fms duration=%.1fms",
				i+1, s.StartTsMs, s.DurationMs)
			if s.Kind != "" {
				fmt.Fprintf(&b, " kind=%s", s.Kind)
			}
			if s.Symbol != "" {
				fmt.Fprintf(&b, " symbol=`%s`", s.Symbol)
			}
			if s.File != "" {
				file := strings.TrimSpace(strings.ReplaceAll(s.File, `\`, `/`))
				if s.Line > 0 {
					if resolvedFile[file] {
						fmt.Fprintf(&b, " (%s:%d ★ resolved)", s.File, s.Line)
					} else {
						fmt.Fprintf(&b, " (%s:%d observed, unresolved)", s.File, s.Line)
					}
				} else if resolvedFile[file] {
					fmt.Fprintf(&b, " (%s ★ resolved)", s.File)
				} else {
					fmt.Fprintf(&b, " (%s observed, unresolved)", s.File)
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Frames — janky-only summary (full list would flood prompt).
	jankyFrames := 0
	for _, f := range bundle.Frames {
		if f.Janky {
			jankyFrames++
		}
	}
	if jankyFrames > 0 {
		fmt.Fprintf(&b, "**Janky frames**: %d / %d (≥ %.1fms 60Hz budget)\n\n",
			jankyFrames, len(bundle.Frames), types.PerfFrameBudget60HzMs)
	}

	// Residue
	if len(bundle.Residue) > 0 {
		fmt.Fprintf(&b, "**Residue** (%d unstructured chunks):\n", len(bundle.Residue))
		for i, r := range bundle.Residue {
			snippet := r
			if len(snippet) > 160 {
				snippet = snippet[:160] + "…"
			}
			fmt.Fprintf(&b, "  - [%d] %s\n", i+1, snippet)
		}
		b.WriteString("\n")
	}

	// Audit legend
	b.WriteString("Citation contract:\n")
	b.WriteString("- Stalls marked `★ resolved` carry a repo-relative path that survived os.Stat verification — those are citation-grade.\n")
	b.WriteString("- Jank `trigger` and `tags` fields are tracing_mark_write tag literals from the trace — quote them verbatim, do NOT translate or paraphrase.\n")
	b.WriteString("- Trace observations with `trace_line=N` are artifact-local line anchors, not repository source citations.\n")
	b.WriteString("- Coverage < 1.0 means some trace bytes ended up in residue; treat residue chunks as advisory context, not as primary evidence.\n")

	return strings.TrimRight(b.String(), "\n")
}

// renderLogError walks a LogError and its Cause chain, writing the
// markdown-like tree into b. depth is the current nesting level
// (0 = top-level error); index is the human 1-based numbering for
// the outermost-level sibling list. Cause chain depth is bounded by
// logtriage.ValidateBundle (LogBundleCaps.MaxCauseDepth = 5) — this
// function does not re-truncate; it trusts the upstream contract.
func renderLogError(b *strings.Builder, e *types.LogError, depth, index int) {
	if e == nil {
		return
	}
	indent := strings.Repeat("   ", depth)
	if depth == 0 {
		fmt.Fprintf(b, "%d. **%s**", index, e.Type)
	} else {
		fmt.Fprintf(b, "%s↳ caused by **%s**", indent, e.Type)
	}
	if e.Message != "" {
		fmt.Fprintf(b, " — %s", truncateForPrompt(e.Message, 200))
	}
	b.WriteString("\n")

	for i := range e.Frames {
		renderLogFrame(b, &e.Frames[i], depth)
	}

	if e.Cause != nil {
		b.WriteString("\n")
		renderLogError(b, e.Cause, depth+1, 0)
	}
}

// renderLogFrame writes one frame with its provenance markers. Frames
// whose File is non-empty passed the os.Stat repo-verification in
// logtriage.ValidateBundle and are citeable; other frames render
// without the ★ marker and without a file:line column so the LLM
// cannot accidentally cite them as authoritative.
func renderLogFrame(b *strings.Builder, f *types.LogFrame, depth int) {
	indent := strings.Repeat("   ", depth+1)
	resolved := f.File != "" && f.Line > 0
	if resolved {
		fmt.Fprintf(b, "%s- ★ resolved `%s:%d`", indent, f.File, f.Line)
		if f.Func != "" {
			fmt.Fprintf(b, " in `%s`", f.Func)
		}
	} else {
		// Unresolved / low-confidence / partial frame.
		var head string
		switch {
		case f.Func != "":
			head = fmt.Sprintf("in `%s`", f.Func)
		case f.Pkg != "":
			head = fmt.Sprintf("in package `%s`", f.Pkg)
		default:
			head = "(no symbol)"
		}
		fmt.Fprintf(b, "%s- (unresolved) %s", indent, head)
	}
	if f.Pkg != "" && resolved {
		fmt.Fprintf(b, " (pkg: `%s`)", f.Pkg)
	}
	fmt.Fprintf(b, " — confidence %.2f", f.Confidence)
	if raw := sanitizeRuntimeFrameRaw(strings.TrimSpace(f.Raw)); raw != "" {
		fmt.Fprintf(b, "\n%s  raw: `%s`", indent, truncateForPrompt(raw, 200))
	}
	b.WriteString("\n")
}

// renderLogCallChain emits the "### Call Chain (innermost → outer)"
// block when the bundle's Signals include panic or crash AND the
// primary error has at least two resolved frames. The block spells
// out the caller→callee roles one frame at a time so the finalizer
// can draw the answer's call-DAG diagram verbatim from the block,
// instead of inventing callers from the structural-ranker candidates
// that landed in Analyzer's Required Files.
//
// Scope decisions:
//
//   - Signals gate: only {panic, crash}. For oom / timeout / validation
//     / db / network / permission / logic, the frames may represent a
//     middleware pipeline rather than a failure call path, so labeling
//     them "caller" / "callee" would over-claim.
//   - Errors[0] only: when the bundle carries multiple parallel
//     snapshots (Go goroutine dump), the validator has already sorted
//     them with the most-significant first. Rendering all parallel
//     snapshots as separate chains would add noise for a small gain.
//   - Resolved frames only: an unresolved frame's file cannot be
//     cited, so putting it in the skeleton risks the literal-grounding
//     gate rejecting the answer. Unresolved frames remain visible in
//     the Errors tree above for context.
//
// Returns the empty string when no block is warranted so the caller
// can simply `b.WriteString(section)` without a nil guard.
func renderLogCallChain(bundle *types.LogBundle) string {
	if bundle == nil || len(bundle.Errors) == 0 {
		return ""
	}
	if !logBundleSignalsIncludeCrash(bundle) {
		return ""
	}
	resolved := make([]types.LogFrame, 0, len(bundle.Errors[0].Frames))
	for _, f := range bundle.Errors[0].Frames {
		if f.File == "" || f.Line <= 0 {
			continue
		}
		resolved = append(resolved, f)
	}
	if len(resolved) < 2 {
		return ""
	}

	// Pre-2026-04-30 this section emitted a bare ``` fence with
	// `innermost failure: … ↑ caller:` ASCII art AND told the model
	// "the answer's ASCII diagram should draw [from these frames]
	// verbatim". That directive was the load-bearing reason the
	// model kept emitting ASCII art in answers despite every
	// downstream surface (skill prompt, Diagram Contract,
	// missing_diagram rejection, retry hints, all five seed
	// renderers) preferring Mermaid: this section carried the
	// CONCRETE frame data the model would actually use, and its
	// directive contradicted the Mermaid preference.
	//
	// Now: emit the call-chain reference as a ```mermaid``` flowchart
	// via the canonical types.RenderLogDiagramFence renderer, and
	// instruct the model to use Mermaid form. This puts the
	// concrete-data injection in agreement with every other
	// Mermaid-preferring surface.
	bundleSubset := &types.LogBundle{
		Errors: []types.LogError{{Frames: resolved}},
	}
	mermaidFence := types.RenderLogDiagramFence(bundleSubset)
	if mermaidFence == "" {
		// types.RenderLogDiagramFence requires ≥2 frames; if the
		// resolved list is shorter, skip the section entirely
		// rather than emitting a degenerate stub. The earlier
		// `len(resolved) < 2` guard at the top of the caller
		// already handles this, but defend defensively.
		return ""
	}
	var b strings.Builder
	b.WriteString("### Call chain (innermost → outer)\n\n")
	b.WriteString("The panic / crash frames above describe the call chain the answer's " +
		"diagram should draw. If your summary contains a call-chain / sequence / flow " +
		"diagram, use the ` ```mermaid ` flowchart shape below as a grounded FLOOR — " +
		"every node here is already evidence-grounded so it is the safest starting " +
		"point. You MAY extend the chain with additional grounded callers or branch " +
		"into related grounded paths if your investigation supports a richer mechanism. " +
		"The HARD rule is that every file named in the diagram must appear in this " +
		"list or in citations[] (the diagram grounding gate rejects unknown file names " +
		"inside fenced code blocks). PREFERRED form: ` ```mermaid ` flowchart or " +
		"sequenceDiagram. ASCII art is the fallback only when the Mermaid subset cannot " +
		"express the shape.\n\n")
	b.WriteString(mermaidFence)
	b.WriteString("\n\n")
	return b.String()
}

// logBundleSignalsIncludeCrash reports whether Meta.Signals contains
// panic or crash. Isolated for test clarity — the gate logic is
// exactly one set-membership check but the intent (runtime failure vs.
// middleware trace) is worth naming.
func logBundleSignalsIncludeCrash(bundle *types.LogBundle) bool {
	if bundle == nil {
		return false
	}
	for _, s := range bundle.Meta.Signals {
		if s == types.SignalPanic || s == types.SignalCrash {
			return true
		}
	}
	return false
}

// bundleHasAuthoritativeCrashFrames is true when the typed LogBundle
// is rich enough that suppressing the raw log section will not strand
// the LLM. Three conjoint conditions:
//
//  1. At least one signal is panic / crash / oom — non-crash logs
//     route through different downstream lanes that still need raw
//     prose.
//  2. The bundle resolved at least 2 files OR a majority (≥50%) of
//     its top-level frames. The bare ">=1 resolved file" gate was
//     too lax: Java-basename-glob commonly resolves only 1 of 5
//     frames, and suppressing the raw log there hides the 4
//     unresolved frames the LLM would otherwise be able to read in
//     prose. Either of these signals proves the structured view
//     has enough coverage to stand alone.
//
// The constants are package-private — they encode a correctness
// boundary (when does the typed view actually substitute for the
// raw text), not a tuning knob for operators.
func bundleHasAuthoritativeCrashFrames(bundle *types.LogBundle) bool {
	if bundle == nil || len(bundle.ResolvedFiles) == 0 {
		return false
	}
	if !logBundleSignalsIncludeCrash(bundle) {
		return false
	}
	const minResolvedAbsolute = 2
	if len(bundle.ResolvedFiles) >= minResolvedAbsolute {
		return true
	}
	totalTopFrames := 0
	for _, e := range bundle.Errors {
		totalTopFrames += len(e.Frames)
	}
	if totalTopFrames == 0 {
		// Header-only bundle (signal present but no frames). Stay on
		// the safe side and DO suppress: there is no useful raw text
		// to hide either way.
		return true
	}
	// resolved/total ≥ 1/2  ⇔  2*resolved ≥ total
	return 2*len(bundle.ResolvedFiles) >= totalTopFrames
}

// bundleHasAuthoritativePerfFrames mirrors
// bundleHasAuthoritativeCrashFrames for PerfBundle. Authoritative
// requires (a) at least one resolved file (so structured frames
// carry repo-grounded anchors) AND (b) the perf-triage stage tagged
// the bundle with the performance intent hint (set by
// derivePerfLayer4 when any jank / stall / cold-start signal fires).
// Either condition alone is insufficient: bare resolved files with
// no perf signal are usually unrelated trace noise, and a perf
// intent without resolved anchors falls back to the raw trace as
// the only legible channel.
// perfIntentHintAuthoritative is the verbatim value
// derivePerfLayer4 / MergePerfBundles assign to PerfBundle.IntentHint
// when at least one jank / stall / cold-start signal is present. We
// compare against the literal — no IntentPerformance constant exists
// (see derivePerfLayer4 in internal/tool/emit_perf_trace.go and
// MergePerfBundles in internal/analysis/perftriage/merge.go).
const perfIntentHintAuthoritative = "performance"

func bundleHasAuthoritativePerfFrames(bundle *types.PerfBundle) bool {
	if bundle == nil || len(bundle.ResolvedFiles) == 0 {
		return false
	}
	return strings.TrimSpace(bundle.IntentHint) == perfIntentHintAuthoritative
}

// runtimeFrameTupleAtomRe matches a single tuple atom — the kind of
// value that appears as a register-level argument in a runtime
// stack-frame dump. Coverage spans every common runtime's panic
// surface, not just Go's:
//
//   - Hex pointer literals: 0x0, 0xc0000064c0
//   - Null sentinels:       nil (Go), <nil>, null (JS / Rust),
//     None (Python)
//   - Booleans:             true / false (Go / Rust / Python /
//     Java / Kotlin / C++23)
//   - Integers:             0, -1, 42 (any signed decimal)
//   - Decimals / scientific: 3.14, -1.5, 1.5e+09, 2.0E-3
//
// A line is scrubbed to `Func(...)` only when EVERY tuple element
// matches this atom shape, so a real string / object / multi-arg
// payload like `Func("hello", 42)` is left intact (the string isn't
// an atom). The common runtime-level "all-pointer / all-null tuple"
// pattern that bait LLMs into mapping `0x0` onto "the receiver was
// nil" gets neutered without false positives on legitimate calls.
var runtimeFrameTupleAtomRe = regexp.MustCompile(
	`^(?:` +
		`0x[0-9a-fA-F]+` + // hex pointer
		`|nil|<nil>|null|None` + // null sentinels (Go/JS/Rust/Python)
		`|true|false` + // booleans
		`|[+-]?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?` + // int / decimal / scientific
		`)$`)

func sanitizeRuntimeFrameRaw(raw string) string {
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return raw
	}
	// Walk every line — multi-line raw frames (typical Go panic dumps:
	// `goroutine 1 [running]:` header line + `main.Foo(0xc0000064c0,
	// 0x0, 0x42)` body line + `\t/path/main.go:42 +0x88` location
	// line) carry the load-bearing tuple on a non-first line. Only
	// scrubbing lines[0] would let the body line leak intact and the
	// LLM would still rationalise the tuple into a caller-provenance
	// claim. The per-line sanitiser is a no-op on lines that don't
	// match the tuple shape, so this is safe to apply unconditionally.
	for i := range lines {
		lines[i] = sanitizeRuntimeFrameTupleLine(lines[i])
	}
	return strings.Join(lines, "\n")
}

func sanitizeRuntimeFrameTupleLine(line string) string {
	if line == "" {
		return ""
	}
	trimmedLeft := strings.TrimLeft(line, " \t")
	prefix := line[:len(line)-len(trimmedLeft)]
	content := strings.TrimSpace(trimmedLeft)
	if content == "" {
		return line
	}
	open := strings.LastIndex(content, "(")
	close := strings.LastIndex(content, ")")
	if open <= 0 || close <= open || close != len(content)-1 {
		return line
	}
	args := strings.TrimSpace(content[open+1 : close])
	if args == "" {
		return line
	}
	parts := strings.Split(args, ",")
	if len(parts) == 0 {
		return line
	}
	for _, part := range parts {
		if !runtimeFrameTupleAtomRe.MatchString(strings.TrimSpace(part)) {
			return line
		}
	}
	return prefix + content[:open] + "(...)"
}

// truncateForPrompt clips a string for prompt rendering. Unlike
// truncateStr (log-line digest), this one appends an explicit
// ellipsis marker so the LLM sees that truncation happened and does
// not assume the quoted text is complete.
func truncateForPrompt(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// Trim to a rune boundary so multibyte UTF-8 does not split.
	trimmed := s
	if max < len(s) {
		trimmed = s[:max]
		for len(trimmed) > 0 && !isUTF8Boundary(trimmed[len(trimmed)-1]) {
			trimmed = trimmed[:len(trimmed)-1]
		}
	}
	return trimmed + "…[truncated]"
}

// isUTF8Boundary reports whether b is a valid start of a UTF-8 rune
// or a continuation byte's terminal slot. Used by truncateForPrompt
// to avoid emitting broken multibyte sequences into the prompt.
func isUTF8Boundary(b byte) bool {
	return b < 0x80 || (b&0xC0) == 0xC0 || (b&0xC0) != 0x80
}

// stripThinkBlocks removes any <think>…</think> spans (case-insensitive,
// DOTALL — content may span newlines) from LLM narrative text before
// it enters a downstream prompt section. Reasoning traces are agent-
// internal; letting them flow into Prior Stage Findings lets a later
// LLM pattern-match on verbs like "Let me emit the analysis" and
// take actions meant for the earlier stage — the session-35
// regression that motivated PipelineStage.IsWrite(). The strip is
// defense-in-depth alongside the IsWrite gate: if a future refactor
// accidentally surfaces StageReports for write-mode stages again,
// the absence of <think> preamble still reduces the attack surface.
// Read-mode agents benefit too (explorer / extractor / finalizer no
// longer see the analyzer's internal deliberation).
var thinkBlockRe = regexp.MustCompile(`(?is)<think>.*?</think>\s*`)

func stripThinkBlocks(s string) string {
	if s == "" || !strings.Contains(strings.ToLower(s), "<think>") {
		return s
	}
	return thinkBlockRe.ReplaceAllString(s, "")
}

func formatStageReports(reports []types.StageReport) string {
	var b strings.Builder
	for i, r := range reports {
		if i > 0 {
			b.WriteString("\n\n")
		}
		findings := stripThinkBlocks(r.Findings)
		fmt.Fprintf(&b, "### %s\n%s", stageReportPromptTitle(r), findings)
	}
	return b.String()
}

func stageReportPromptTitle(r types.StageReport) string {
	switch r.Agent {
	case types.AgentAnalyzer, types.AgentWriteAnalyzer:
		return "Prior request analysis result"
	case types.AgentExplorer, "sub_explorer":
		return "Prior code investigation result"
	case types.AgentExtractor:
		return "Prior answer-slate extraction result"
	case types.AgentFinalizer:
		return "Prior answer drafting result"
	case types.AgentLogTriager:
		return "Prior runtime-log extraction result"
	case types.AgentPerfTriager:
		return "Prior performance-trace extraction result"
	case types.AgentPlanner:
		return "Prior change-planning result"
	case types.AgentCoder:
		return "Prior patch-application result"
	case types.AgentVerifier:
		return "Prior verification result"
	default:
		switch r.Stage {
		case types.StageAnalyze, types.StageWriteAnalyze:
			return "Prior request analysis result"
		case types.StageExplore:
			return "Prior code investigation result"
		case types.StageExtract:
			return "Prior answer-slate extraction result"
		case types.StageFinalize:
			return "Prior answer drafting result"
		case types.StagePlan:
			return "Prior change-planning result"
		case types.StageApply:
			return "Prior patch-application result"
		case types.StageVerify:
			return "Prior verification result"
		default:
			return "Prior stage result"
		}
	}
}

func stageReportsForAgent(reports []types.StageReport, ac *types.AgentContext) []types.StageReport {
	if len(reports) == 0 {
		return nil
	}
	if ac == nil {
		return dedupeStageReportsForPrompt(reports, nil)
	}
	// In typed-support finalizer mode, *all* free-form StageReports
	// become competing semantic channels beside the compiled support
	// lanes. Even the analyzer report can contain an eager narrative
	// lead ("the root cause is already clear...") that outruns the
	// grounded support contract. The finalizer should rebuild its
	// principal claims from typed support lanes, known facts, and
	// citations only; StageReports remain useful upstream but must not
	// survive into the final synthesis prompt.
	if ac.AgentName == types.AgentFinalizer && finalizerUsesTypedAnswerSupport(ac) {
		return nil
	}
	if ac.AgentName == types.AgentExtractor && ac.Mutable != nil {
		if ta := ac.Mutable.TurnAArtifacts(); ta != nil && (len(ta.EvidenceItems) > 0 || strings.TrimSpace(ta.AcceptedClosureReason) != "") {
			return nil
		}
	}
	filtered := make([]types.StageReport, 0, len(reports))
	for _, report := range reports {
		if report.Stage == ac.Stage || report.Agent == ac.AgentName {
			continue
		}
		switch report.Agent {
		case types.AgentLogTriager:
			if ac.LogTriage != nil {
				continue
			}
		case types.AgentPerfTriager:
			if ac.PerfTrace != nil {
				continue
			}
		}
		filtered = append(filtered, report)
	}
	return dedupeStageReportsForPrompt(filtered, ac)
}

func dedupeStageReportsForPrompt(reports []types.StageReport, _ *types.AgentContext) []types.StageReport {
	if len(reports) == 0 {
		return nil
	}
	filtered := make([]types.StageReport, 0, len(reports))
	seen := make(map[string]struct{}, len(reports))
	for _, report := range reports {
		findings := strings.TrimSpace(stripThinkBlocks(report.Findings))
		if findings == "" {
			continue
		}
		report.Findings = findings
		key := strings.Join([]string{
			string(report.Stage),
			string(report.Agent),
			stageReportPromptTitle(report),
			findings,
		}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, report)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func typedSupportFinalizerRepoFacts(
	repoFacts []types.RepoFact,
	ac *types.AgentContext,
	fallbackFacts []string,
	fallbackFiles []string,
) ([]string, []string) {
	if len(repoFacts) == 0 || ac == nil || !finalizerUsesTypedAnswerSupport(ac) {
		return fallbackFacts, fallbackFiles
	}
	_, allowedFiles := typedSupportFinalizerSupportCeiling(ac)
	if len(allowedFiles) == 0 {
		return fallbackFacts, fallbackFiles
	}
	filteredFacts := make([]types.RepoFact, 0, len(repoFacts))
	for _, fact := range repoFacts {
		source := strings.TrimSpace(strings.ReplaceAll(fact.Source, `\`, `/`))
		if source == "" || !allowedFiles[source] {
			continue
		}
		filteredFacts = append(filteredFacts, fact)
	}
	if len(filteredFacts) == 0 {
		return fallbackFacts, fallbackFiles
	}
	return extractRelevantFacts(filteredFacts), extractRelevantFiles(filteredFacts)
}

func typedSupportFinalizerEvidencePool(ac *types.AgentContext, items []types.EvidenceItem) []types.EvidenceItem {
	if len(items) == 0 || ac == nil || !finalizerUsesTypedAnswerSupport(ac) {
		return items
	}
	allowedLocations, allowedFiles := typedSupportFinalizerSupportCeiling(ac)
	if len(allowedLocations) == 0 && len(allowedFiles) == 0 {
		return items
	}
	filtered := make([]types.EvidenceItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(types.EvidenceAuthoritativeSurfaceText(item, false)) == "" {
			continue
		}
		if len(allowedLocations) > 0 {
			if !allowedLocations[supportEntryLocationForEvidenceItem(item)] {
				continue
			}
			filtered = append(filtered, item)
			continue
		}
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source == "" || !allowedFiles[source] {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func typedSupportFinalizerSupportCeiling(ac *types.AgentContext) (map[string]bool, map[string]bool) {
	plan := types.BuildAnswerSupportPlanForAgentContext(ac)
	if plan == nil || len(plan.Lanes) == 0 {
		return nil, nil
	}
	allowedLocations := map[string]bool{}
	allowedFiles := map[string]bool{}
	for _, lane := range plan.Lanes {
		for _, entry := range lane.Entries {
			location := strings.TrimSpace(strings.ReplaceAll(entry.Location, `\`, `/`))
			if location != "" {
				allowedLocations[location] = true
			}
			file := supportPlanEntryFile(location)
			if file == "" {
				continue
			}
			allowedFiles[file] = true
		}
	}
	if len(allowedLocations) == 0 && len(allowedFiles) == 0 {
		return nil, nil
	}
	return allowedLocations, allowedFiles
}

func suppressKnownFactsForTypedSupportFinalizer(ac *types.AgentContext) bool {
	return ac != nil && ac.AgentName == types.AgentFinalizer && finalizerUsesTypedAnswerSupport(ac)
}

func suppressAnswerSymbolsForTypedSupportFinalizer(ac *types.AgentContext) bool {
	return ac != nil && ac.AgentName == types.AgentFinalizer && finalizerUsesTypedAnswerSupport(ac)
}

func suppressSubjectMatchSummaryForTypedSupportFinalizer(ac *types.AgentContext) bool {
	return ac != nil && ac.AgentName == types.AgentFinalizer && finalizerUsesTypedAnswerSupport(ac)
}

func suppressUnverifiedLeadsForTypedSupportFinalizer(ac *types.AgentContext) bool {
	return ac != nil && ac.AgentName == types.AgentFinalizer && finalizerUsesTypedAnswerSupport(ac)
}

func supportPlanEntryFile(location string) string {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`))
	if location == "" {
		return ""
	}
	if idx := strings.LastIndex(location, ":"); idx > 0 {
		if linePart := strings.TrimSpace(location[idx+1:]); linePart != "" {
			if isAllDigits(linePart) {
				return strings.TrimSpace(location[:idx])
			}
		}
	}
	return location
}

func supportEntryLocationForEvidenceItem(item types.EvidenceItem) string {
	src := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if src == "" {
		return ""
	}
	if item.LineStart > 0 {
		return fmt.Sprintf("%s:%d", src, item.LineStart)
	}
	return src
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func priorReportsContainSection(reports []types.StageReport, heading string) bool {
	if heading == "" {
		return false
	}
	for _, r := range reports {
		if strings.Contains(r.Findings, heading) {
			return true
		}
	}
	return false
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
	if len(matches) == 0 || expected.Kind == types.SubjectUnknown || expected.Kind == types.SubjectGeneric {
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

func extractUnverifiedFindingsFromStageReports(reports []types.StageReport) []types.UnverifiedFinding {
	var out []types.UnverifiedFinding
	for _, report := range reports {
		text := report.Findings
		for {
			start := strings.Index(text, "~~")
			if start < 0 {
				break
			}
			rest := text[start+2:]
			end := strings.Index(rest, "~~")
			if end < 0 {
				break
			}
			token := strings.TrimSpace(strings.Trim(rest[:end], "`\"' "))
			after := rest[end+2:]
			if token != "" && annotatedMissFollows(after) {
				out = append(out, types.UnverifiedFinding{
					Token:  token,
					Kind:   inferUnverifiedFindingKind(token),
					Reason: "unverified in prior stage report",
				})
			}
			text = after
		}
	}
	return out
}

func annotatedMissFollows(text string) bool {
	if len(text) > 120 {
		text = text[:120]
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "unverified") ||
		strings.Contains(lower, "repo graph") ||
		strings.Contains(text, "未验证") ||
		strings.Contains(text, "未在 repo") ||
		strings.Contains(text, "鏈獙")
}

func inferUnverifiedFindingKind(token string) string {
	if strings.ContainsAny(token, `/\`) {
		return "path"
	}
	return "symbol"
}

func mergeUnverifiedFindings(groups ...[]types.UnverifiedFinding) []types.UnverifiedFinding {
	seen := make(map[string]bool)
	var out []types.UnverifiedFinding
	for _, group := range groups {
		for _, f := range group {
			if strings.TrimSpace(f.Token) == "" {
				continue
			}
			kind := strings.TrimSpace(f.Kind)
			if kind == "" {
				kind = "symbol"
			}
			key := kind + "\x00" + f.Token
			if seen[key] {
				continue
			}
			seen[key] = true
			f.Kind = kind
			out = append(out, f)
		}
	}
	return out
}

// formatAnalyzerPrescanForPlan renders the structured analyzer
// pre-scan signals that the planner needs to skip its own re-discovery
// of entities, sub-topics, and relevant files. Only fields that are
// LLM-validated string lists or repo_map graph derivations are
// surfaced — never raw lastContent (which carries <think> preamble
// and is the session-35 leak vector).
//
// Returns "" when the IR is nil or every field is empty so the
// caller can skip the section unconditionally.
func formatAnalyzerPrescanForPlan(ir *types.AnalysisIR) string {
	if ir == nil {
		return ""
	}
	rm := ir.RequestModel
	hints := rm.AnalyzerHints
	requiredFiles := ir.EvidencePlan.RequiredFiles

	// Prefer deterministic MentionedEntities (verbatim RawRequest
	// surfaces) over the analyzer-authored PrimaryEntities shortlist,
	// then fall back to the merged breadth list.
	entities := hints.MentionedEntities
	if len(entities) == 0 {
		entities = hints.PrimaryEntities
	}
	if len(entities) == 0 {
		entities = hints.Entities
	}

	if len(entities) == 0 && len(hints.Keywords) == 0 &&
		len(rm.SubTopics) == 0 && len(requiredFiles) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The analyze stage already classified the request and ran an entity/file pre-scan against the repo graph. ")
	b.WriteString("Use the structured findings below to jump straight to the files and symbols that matter — you do NOT need to re-discover them with repo_map / list_files / grep before reading.\n")

	if len(rm.SubTopics) > 0 {
		b.WriteString("\n### Sub-topics\n")
		for _, st := range rm.SubTopics {
			summary := strings.TrimSpace(st.Summary)
			if summary == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s", summary)
			if len(st.Entities) > 0 {
				fmt.Fprintf(&b, " — entities: %s", strings.Join(st.Entities, ", "))
			}
			b.WriteByte('\n')
		}
	}

	if len(entities) > 0 {
		b.WriteString("\n### Code entities (analyzer-extracted)\n")
		for _, e := range entities {
			if e = strings.TrimSpace(e); e != "" {
				fmt.Fprintf(&b, "- %s\n", e)
			}
		}
	}

	if len(requiredFiles) > 0 {
		b.WriteString("\n### Pre-scored relevant files (top of repo_map ranking)\n")
		b.WriteString("These paths scored highest for the question's entity query — read them first if your plan needs to touch this area.\n")
		for _, f := range requiredFiles {
			if f = strings.TrimSpace(f); f != "" {
				fmt.Fprintf(&b, "- %s\n", f)
			}
		}
	}

	return b.String()
}

func formatExactResolutionHint(ac *types.AgentContext) string {
	if ac == nil || ac.AnalysisIR == nil || ac.AnalysisIR.AnswerContract.ExactResolution == nil {
		return ""
	}
	contract := ac.AnalysisIR.AnswerContract.ExactResolution
	pending := types.ExactResolutionPendingTargets(contract, ac.UnverifiedAnalyzerFindings)
	if len(pending) == 0 {
		return ""
	}
	label := strings.TrimSpace(contract.TargetLabel)
	if label == "" {
		label = "target"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The user's requested exact %s could not yet be verified against the repo graph:\n", label)
	for _, target := range pending {
		fmt.Fprintf(&b, "- `%s`\n", target)
	}
	b.WriteString("\nTreat this as an exact-resolution / absence-candidate task. Do not rename the target, map it to a nearby item, or say it corresponds to a different key / symbol / path unless the repo contains explicit alias, parser-mapping, or documented-synonym proof that names the exact target.")
	if contract.AllowAbsence {
		b.WriteString(" If no exact target or explicit alias exists, the answer may end in an exact-absence conclusion.")
	}
	if hint := strings.TrimSpace(contract.RelatedContextScopeHint); hint != "" {
		fmt.Fprintf(&b, " When you inspect nearby context, keep it within the %s before jumping to unrelated namespaces.", hint)
	}
	if scopeTerms := types.ExactResolutionFocusTerms(contract); len(scopeTerms) > 0 {
		fmt.Fprintf(&b, " Useful local-scope terms for focused follow-up: %s.", strings.Join(scopeTerms, ", "))
	}
	if contract.TargetKind == types.SubjectConfigKey &&
		contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded {
		b.WriteString(" For exact config-key traces, a broad family root by itself is not enough to choose the next file: prefer config-file layers, config structs/tags, binding/merge code, and operator-override surfaces before generic implementation files that merely mention the family root.")
	}
	if ac.Stage == types.StageExplore {
		b.WriteString(" Read same-scope anchors first, then close the investigation with `emit_investigation_complete(result_kind=\"absence\", absence_justification=...)` instead of completing a positive substitute chain if the exact target remains absent.")
	}
	return b.String()
}

// formatMultiRepoActiveSetAdvisory composes the L0 LLM advisory that
// tells the explorer / extractor / finalizer which sub-repos its
// file-system tools (read_file / grep / repo_map) may operate within
// and surfaces a short preview of out-of-active sub-repos so the LLM
// can name the ones the user might want to pin via `/repos focus`.
//
// 2026-05-08 (u7a-style multi-repo gap remediation): without this
// section the LLM has no way to know the workspace contains multiple
// sub-repos — it would see only the user request and the per-tool
// results, then "blind-fire" tool calls against arbitrary paths and
// hit the L1 gate (Phase 1.L1) repeatedly. Surfacing the active set
// up front cuts the wasted ReAct iterations.
//
// Render rule:
//   - When ac.SubRepos has < 2 entries the workspace is single-repo
//     (or one sub-repo) — no advisory; the existing prompt sections
//     already convey the scope.
//   - active = SubRepos minus PendingSubRepos; sorted alphabetically
//     by RootRel for deterministic, stable rendering across Runs.
//   - Inactive preview is capped at MultiRepoInactivePreviewCount
//     (yaml-clamped at config load; default 2; hard ceiling 3).
//
// R6 audit: every line is generic prose with no internal pipeline
// terminology (no stage names, no slug values, no routing channel
// letters). Examples are placeholder-free; all identifiers come from
// the workspace itself.
func formatMultiRepoActiveSetAdvisory(ac *types.AgentContext) string {
	if ac == nil || len(ac.SubRepos) < 2 {
		return ""
	}
	pendingSet := make(map[string]bool, len(ac.PendingSubRepos))
	for _, p := range ac.PendingSubRepos {
		pendingSet[p] = true
	}
	var active, inactive []types.SubRepoSnapshot
	for _, sr := range ac.SubRepos {
		if pendingSet[sr.RootRel] {
			inactive = append(inactive, sr)
		} else {
			active = append(active, sr)
		}
	}
	if len(active) == 0 {
		// Defensive: every snapshot was marked pending. The orchestrator
		// would not normally produce this state, so don't render an
		// advisory whose active list is empty (it would mislead the LLM).
		return ""
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].RootRel < active[j].RootRel
	})
	sort.Slice(inactive, func(i, j int) bool {
		return inactive[i].RootRel < inactive[j].RootRel
	})

	previewN := ac.MultiRepoInactivePreviewCount
	if previewN <= 0 {
		// Builder fallback when the orchestrator stamped a zero
		// (single-shot tests / very old fixtures). The clamp helper
		// would have rejected a negative value already.
		previewN = config.MultiRepoInactivePreviewCountDefault
	}

	var b strings.Builder
	b.WriteString("Active sub-repos for this question (file-system tool calls — read_file, grep, repo_map — must stay within these):\n")
	for _, sr := range active {
		writeSubRepoLine(&b, sr)
	}
	showInactivePreview := ac.AgentName != types.AgentFinalizer
	if len(inactive) > 0 && showInactivePreview {
		visible := previewN
		if visible > len(inactive) {
			visible = len(inactive)
		}
		b.WriteString("\nOut of active set (paths inside these will be refused by the file-system tools):\n")
		for i := 0; i < visible; i++ {
			writeSubRepoLine(&b, inactive[i])
		}
		if extra := len(inactive) - visible; extra > 0 {
			fmt.Fprintf(&b, "  ... and %d more\n", extra)
		}
	}
	if showInactivePreview {
		b.WriteString("\nIf the user's question genuinely needs to consult a sub-repo currently out of the active set, surface that requirement in plain prose at the top of your final answer — the user will adjust the workspace scope and re-ask. Do not attempt file-system tool calls against an out-of-set path; they will be refused.\n")
	}
	b.WriteString("\nWhen citing a path inside an active sub-repo, prefer the full sub-repo-prefixed form (e.g. `<sub-repo>/path/to/file.go`). If you supply a bare relative path that is not prefixed by any active sub-repo, the file-system tools will try to resolve it under each active sub-repo in alphabetical order and use the first unique match; an ambiguous bare path (matching multiple active sub-repos) or one that matches none is refused.\n")
	return b.String()
}

func writeSubRepoLine(b *strings.Builder, sr types.SubRepoSnapshot) {
	langs := strings.Join(sr.PrimaryLangs, ", ")
	if langs != "" {
		fmt.Fprintf(b, "- %s (%s)\n", sr.RootRel, langs)
	} else {
		fmt.Fprintf(b, "- %s\n", sr.RootRel)
	}
}

func formatToolSourcedValueHint(ac *types.AgentContext) string {
	if ac == nil || ac.AnalysisIR == nil || !isCitationFreeValueAnswer(ac) {
		return ""
	}
	var b strings.Builder
	b.WriteString("This is a citation-free value question: the authoritative literal may come from deterministic tool output or VCS metadata rather than from a repo file:line.\n")
	switch ac.Stage {
	case types.StageExplore:
		b.WriteString("- Treat command / VCS output as authoritative when it directly answers the question.\n")
		b.WriteString("- Do NOT emit file:line evidence just to mirror a command result. Use `emit_evidence` only for real repo anchors you actually read.\n")
		b.WriteString("- For VCS history/diff answers, do NOT set `evidence_floor_waiver`; that waiver is only for attached external logs/traces. Carry git findings through `emit_investigation_complete.reason` and `aggregate_facts`.\n")
		if rm := ac.AnalysisIR.RequestModel; rm.Predicates.IsHistoryLookup &&
			!types.CompileAnswerIntentContract(rm, &ac.AnalysisIR.AnswerContract).HasOrigin(types.AnswerEvidenceOriginCurrentSource) {
			b.WriteString("- This is a pure VCS-history handoff: after `git_log` / `git_show` / `git_diff` have answered the question, do not call `read_file` or `emit_evidence` just to satisfy source-code habits. Close with `emit_investigation_complete(reason, aggregate_facts)` and preserve commit-by-commit summaries there.\n")
		}
		b.WriteString("- If one repo anchor is needed to disambiguate the target, read it once; otherwise a tool-only investigation may complete cleanly.\n")
	case types.StageExtract, types.StageFinalize:
		b.WriteString("- When the literal comes from command output / VCS metadata rather than repo code, leave the item uncited and explain the provenance in `summary`.\n")
		b.WriteString("- Do NOT copy tool outputs into `citations[]`; those entries are reserved for repo file:line anchors.\n")
		b.WriteString("- In visible prose, say the fact came from repository history / diff output / command output; never mention internal citation carriers or `citations[]` to the user.\n")
		b.WriteString("- When summarizing multiple commits, keep grouping/count language exactly aligned with the VCS list; avoid approximate phrases such as 'near half' unless the exact count supports them.\n")
		b.WriteString("- Do not introduce module/component/category counts unless that exact count is present in the VCS/command output or in `aggregate_facts`; prefer qualitative grouping when only the commit list is available.\n")
		b.WriteString("- For VCS history lists, each commit item should state both the observed change and the effect/impact that follows from the subject/stat/name output. If the available VCS output proves only a subject/stat, say that boundary instead of filling generic path-only prose.\n")
	}
	return b.String()
}

func formatEvidenceOriginBoundaryHint(ac *types.AgentContext) string {
	if ac == nil || ac.AnalysisIR == nil {
		return ""
	}
	// The finalizer already receives the same unified contract from
	// answer_document_evaluator.renderAnswerDocUnifiedIntentContract and
	// renderAnswerDocClaimBindings. Keep this builder section upstream
	// only, otherwise finalizer prompts carry duplicate, slightly
	// different copies of the same policy.
	if ac.Stage != types.StageExplore && ac.Stage != types.StageExtract {
		return ""
	}
	contract := types.CompileAnswerIntentContract(ac.AnalysisIR.RequestModel, &ac.AnalysisIR.AnswerContract)
	if !answerIntentContractHasNonSourceOrigin(contract) {
		return ""
	}
	var b strings.Builder
	b.WriteString("Evidence origins are separate from answer shape. Do not collapse the answer to a scalar, list, or table just because one origin supplies a literal value.\n")
	fmt.Fprintf(&b, "- Active evidence origins: %s.\n", renderEvidenceOriginList(contract.Origins))
	if len(contract.RequestedOutputs) > 0 {
		fmt.Fprintf(&b, "- Requested answer shapes to preserve: %s.\n", renderRequestedOutputList(contract.RequestedOutputs))
	}
	b.WriteString("- `current_source` facts may use `emit_evidence` and file:line citations after the source line was read and grounded.\n")
	b.WriteString("- Non-current-source facts are first-class evidence in their own lane. Do not convert VCS history, diff hunks, attached logs/traces, command measurements, negative searches, or repo-index facts into fake current-source `emit_evidence` rows.\n")
	b.WriteString("- Keep lanes separate when a question mixes origins: VCS diff/log/trace facts prove what happened historically or externally; current-source claims still need current-source evidence.\n")
	if answerIntentContractHasMixedCurrentAndNonSourceOrigin(contract) {
		b.WriteString("- Mixed-origin lane plan: first collect each non-current-source observation with its producer tool and preserve its typed origin in `reason` / `aggregate_facts`; then read current-source anchors only for present-checkout implementation claims. If both lanes discuss the same target, keep both summaries instead of converting one lane into the other's citation or repeating the same search.\n")
	}
	switch ac.Stage {
	case types.StageExplore:
		b.WriteString("- During exploration, use the producer tool for the origin (`git_log`/`git_show`/`git_diff`, log/perf triage bundles, `exec_command`, grep negative searches, or repo_map) and hand off the result through `emit_investigation_complete.reason` plus structured `aggregate_facts` when a count, list, scalar, absence, or grouping must be preserved.\n")
		b.WriteString("- Use `emit_evidence` only when you have a real current-checkout source anchor to cite. Old/deleted diff lines, stack-frame coordinates, command output rows, and zero-result searches are not current-source anchors.\n")
	case types.StageExtract:
		b.WriteString("- During extraction, preserve the origin label and requested output together. A VCS/diff/log/command aggregate can be principal without becoming an answer-symbol file:line slate.\n")
	}
	if contract.HasOrigin(types.AnswerEvidenceOriginVCSDiff) {
		b.WriteString("- Diff evidence may refer to old/deleted/renamed paths or line numbers. Treat those as historical patch facts; verify the current checkout separately only when the user also asked about current behavior.\n")
	}
	if contract.HasOrigin(types.AnswerEvidenceOriginRuntimeArtifact) {
		b.WriteString("- Runtime log/trace observations are valid observed facts even when no stack frame maps to the active checkout. Do not invent current-source citations to make them look grounded.\n")
	}
	if contract.HasOrigin(types.AnswerEvidenceOriginRepoNegativeSearch) {
		b.WriteString("- A `negative_search` fact must preserve its repo/scope/query/result_count boundary. It proves bounded repository absence, not global unknowability outside that boundary.\n")
		b.WriteString("- For git history/diff output, attached logs, traces, command output, or repo-map/index output, use `negative_observation` with origin/target-or-query/scope/result_count/searched_at instead of pretending the absence is a repository grep result.\n")
	}
	return b.String()
}

func answerIntentContractHasMixedCurrentAndNonSourceOrigin(contract types.AnswerIntentContract) bool {
	return contract.HasOrigin(types.AnswerEvidenceOriginCurrentSource) && answerIntentContractHasNonSourceOrigin(contract)
}

func answerIntentContractHasNonSourceOrigin(contract types.AnswerIntentContract) bool {
	for _, origin := range contract.Origins {
		if origin != types.AnswerEvidenceOriginUnknown && origin != types.AnswerEvidenceOriginCurrentSource {
			return true
		}
	}
	return false
}

func renderEvidenceOriginList(origins []types.AnswerEvidenceOrigin) string {
	if len(origins) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin == types.AnswerEvidenceOriginUnknown {
			continue
		}
		parts = append(parts, "`"+string(origin)+"`")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func renderRequestedOutputList(outputs []types.AnswerRequestedOutput) string {
	if len(outputs) == 0 {
		return "summary"
	}
	parts := make([]string, 0, len(outputs))
	for _, output := range outputs {
		if output == types.AnswerRequestedOutputUnknown {
			continue
		}
		parts = append(parts, "`"+string(output)+"`")
	}
	if len(parts) == 0 {
		return "summary"
	}
	return strings.Join(parts, ", ")
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
	display := strings.TrimSpace(types.EvidenceDeterministicSurfaceText(ev, false))
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
		return "You MUST write every natural-language response in Simplified Chinese (简体中文). This is a hard requirement set by the project configuration — do not switch to English prose even if the user writes the question in English. Summaries, step descriptions, rationales, captions, and any other natural-language content are all in Chinese. Use Chinese visibility/access-control wording such as 公开、非公开、导出、非导出 in natural-language prose; keep source-code tokens like `public`, `private`, `protected`, code identifiers, file paths, type names, and function names in their original form."
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
	return "Reply in the same natural language as the user's question. Ignore code identifiers, file paths, and technical terms (e.g. `explorer`, `subagent`, `internal/agent/foo.go`) when judging the question's language — a sentence whose prose is Chinese but which mentions English symbols is still a Chinese question. When the question is ambiguous or contains no natural-language prose, default to Simplified Chinese (简体中文). Always keep code identifiers, file paths, and technical terms in their original form in your reply. If the answer language is Chinese, use Chinese visibility/access-control wording such as 公开、非公开、导出、非导出 in natural-language prose; keep source-code tokens like `public`, `private`, `protected` unchanged only when quoting or naming code."
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
		return "The user's question is written in Chinese. You MUST write your answer in Simplified Chinese (简体中文). This is a hard requirement — do not switch to English prose for the summary, step descriptions, rationales, captions, or any other natural-language content. Keep code identifiers, file paths, type names, and function names in their original form. Use Chinese visibility/access-control wording such as 公开、非公开、导出、非导出 in natural-language prose; keep source-code tokens like `public`, `private`, `protected` unchanged only when quoting or naming code."
	}
	// Latin assertion: require enough letters to avoid flagging a
	// single-word query. 20 letters is roughly a short English
	// sentence.
	if latin >= 20 && cjk == 0 {
		return "The user's question is written in English. You MUST write your answer in English. Keep code identifiers, file paths, type names, and function names in their original form."
	}
	return ""
}
