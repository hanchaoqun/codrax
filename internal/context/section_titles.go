package context

import "github.com/hanchaoqun/codrax/internal/types"

// Section title constants — single source of truth.
//
// Section titles are LOAD-BEARING:
//   - canonicalSystemSectionOrder / canonicalUserSectionOrder pin
//     ordering by string equality
//   - Tests look sections up via findSectionTitle / findSystemSection
//     (string equality lookup against pc.SystemSections / pc.UserSections)
//   - Evaluators MUST NOT re-emit any of these titles in their per-
//     dispatch user message append (would duplicate visually identical
//     sections with possibly contradictory directives)
//   - LLM-facing prose that REFERENCES a section by name (e.g., "see
//     the Knowledge & Evidence Pool section below") must use the
//     constant, NOT a literal — otherwise rename drift produces
//     dangling references in the prompt that confuse the model.
//
// Bare-string usage at the call site silently breaks the contract when
// any single site is touched without updating the others. Always
// reference the constants below; never write the title literal again.
//
// Constants are EXPORTED so cross-package code (e.g. agent/
// stage_report_render that mentions section names in prose) can also
// reference the single source. The contract is: this file owns the
// strings; everyone else points here.
const (
	// System-role sections (rendered in the message[0] system block).
	SectionAgentIdentity    = "Agent Identity"
	SectionReasoningHygiene = "Reasoning Hygiene"
	SectionThinkAloud       = "Think Aloud"
	SectionConstraints      = "Constraints"
	SectionUserPreferences  = "User Preferences"
	SectionPipelineState    = "Pipeline State"
	SectionSkillGoal        = "Skill Goal"
	SectionWorkflow         = "Workflow"
	SectionOutputFormat     = "Output Format"
	// SectionExplorationContract — output-format title swap: explore
	// skill renders the same slot as "Exploration Contract"
	// (skill-name conditional in BuildPromptContext).
	SectionExplorationContract = "Exploration Contract"
	SectionProhibitions        = "Prohibitions"

	// User-role sections (rendered into the dispatch user message).
	SectionRetryDirective = "Retry Directive (READ FIRST)"
	// SectionTaughtSchemaLessons (EMITBURN-1 件B, §29.174 RUN2AUDIT-1):
	// structured-handoff schema lessons an earlier explore window of this
	// run already paid for through emit rejects + retry hint. Replayed
	// verbatim into later explore dispatches so a fresh window does not
	// re-learn the same schema contract through fresh rejects. Soft
	// guidance only; content is the already-R6-clean retry-hint segment.
	SectionTaughtSchemaLessons   = "Structured Handoff Lessons From Earlier Rounds"
	SectionAnswerCoverageNotes   = "Answer Coverage Notes (READ FIRST)"
	SectionUserRequest           = "User Request"
	SectionPresentationDirective = types.PresentationDirectiveSectionTitle
	SectionAnalyzerPrescan       = "Analyzer Pre-scan Findings"
	SectionPriorConversation     = "Prior Conversation (reference only)"
	SectionLogTriageExtraction   = "Log Triage — Validated Extraction"
	SectionPerfTriageExtraction  = "Perf Triage — Validated Extraction"
	SectionRuntimeArtifactChoice = "Runtime Artifact Selection"
	SectionAttachedRuntimeLog    = "Attached Runtime Log"
	SectionAttachedPerfTrace     = "Attached Performance Trace (HiTrace / atrace / systrace / perfetto)"
	SectionPriorStageFindings    = "Prior Stage Findings"
	SectionUnverifiedAnalyzer    = "Unverified Analyzer Findings"
	SectionExactResolution       = "Exact Resolution"
	SectionEvidenceOrigin        = "Evidence Origin Boundary"
	SectionMultiRepoActiveSet    = "Multi-Repo Active Set"
	SectionToolSourcedValue      = "Tool-Sourced Value"
	SectionSubjectMatchSummary   = "Subject Match Summary"
	// SectionTraceRootCauseBoard (CR-1 件③, §29.42.2① 单源摘要喂入,
	// docs/design/real_trace_campaign_20260705.md, 2026-07-12): the typed
	// root-cause board summary injected into the answer-rendering dispatch —
	// authoritative order, one line per seated row (effective attribution +
	// caliber + channel + confidence), so the prose never needs to invent a
	// third ordering or a self-made aggregate.
	SectionTraceRootCauseBoard = "Runtime Trace Root-Cause Board (authoritative order)"
	// SectionTraceWaitEvidence (EVID-BR 件①, §29.55.4 F1/R2-F3, 2026-07-13):
	// the typed kernel wait-call-site (sched_blocked_reason caller symbols) +
	// wakeup-source (sched_wakeup waker/timestamp edges) evidence summary —
	// the donghu cold read showed the appendix carried the right caller symbol
	// every run while the model face never saw it (四跑四答案), and the waker
	// question was answered from the wrong lane 11/11 (R2-F3).
	SectionTraceWaitEvidence  = "Runtime Trace Kernel Wait Call-Site & Wakeup Evidence"
	SectionRawToolOutputs     = "Raw Tool Outputs from the Investigation"
	SectionKnownFacts         = "Known Facts"
	SectionAnswerSymbolsAuth  = "Extracted Answer Symbols (authoritative)"
	SectionAnswerSymbolsFloor = "Answer Symbols (lower-bound floor, may extend with cited evidence)"
	SectionEvidencePool       = "Knowledge & Evidence Pool"
	SectionRelationDossier    = "Relation Dossier (advisory)"
	SectionUnverifiedLeads    = "Unverified Leads (not for citation)"
	SectionDataflowFindings   = "Dataflow Findings"
	SectionHypothesisVerdicts = "Hypothesis Verdicts"
	SectionRelevantFiles      = "Relevant Files"
)

// canonicalSystemSectionOrder lists every system-role section title
// BuildPromptContext may emit, in the exact order the LLM sees them.
// The list mirrors the append() sequence in BuildPromptContext. When
// a new system section is added below, also add its constant + entry
// here so a grep reaches both sides.
var canonicalSystemSectionOrder = []string{
	SectionAgentIdentity,
	SectionReasoningHygiene,
	SectionThinkAloud,
	SectionConstraints,
	SectionUserPreferences,
	SectionPipelineState,
	SectionSkillGoal,
	SectionWorkflow,
	SectionOutputFormat,
	SectionProhibitions,
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
	SectionRetryDirective,
	SectionTaughtSchemaLessons, // explore only — replayed schema lessons from earlier windows
	SectionUserRequest,
	SectionPresentationDirective,
	SectionAnalyzerPrescan, // write-mode (StagePlan) only — structured fields from AnalysisIR
	SectionPriorConversation,
	SectionRuntimeArtifactChoice,
	SectionPriorStageFindings, // carries the canonical Resolution Chains subsection
	SectionUnverifiedAnalyzer,
	SectionExactResolution,
	SectionEvidenceOrigin,
	SectionToolSourcedValue,
	SectionMultiRepoActiveSet,
	SectionRawToolOutputs,
	SectionKnownFacts,
	SectionAnswerSymbolsAuth,
	SectionAnswerSymbolsFloor,
	SectionEvidencePool,
	SectionRelationDossier,
	SectionUnverifiedLeads,
	SectionDataflowFindings,
	SectionHypothesisVerdicts,
	SectionRelevantFiles,
}
