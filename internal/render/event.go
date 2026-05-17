// Package render provides a rich CLI rendering system for the codrax pipeline.
//
// The rendering architecture is event-driven: the orchestrator and agents emit
// typed Event values through an EventEmitter callback, and the Renderer
// consumes them to produce styled terminal output in real time. This
// decoupling means the pipeline code never imports terminal libraries and the
// renderer can be swapped (e.g. JSON for CI, plain text for pipes).
package render

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EventKind identifies what happened in the pipeline.
type EventKind int

const (
	// Pipeline lifecycle
	EventPipelineStart EventKind = iota
	EventPipelineEnd

	// Stage lifecycle
	EventStageStart
	EventStageEnd

	// Agent activity
	EventAgentThinking  // LLM call started
	EventAgentResponse  // LLM call returned
	EventLocalWorkStart // local CPU work started outside an LLM request

	// Generic LLM request telemetry. Direct reviewer / classifier
	// calls that bypass BaseAgent emit this before adapter.Chat so the
	// dock can display the effective model and rough request size with
	// the same surface as ReAct agent calls.
	EventLLMRequestStart

	// Tool calls
	EventToolCallStart
	EventToolCallEnd

	// Sub-agent lifecycle
	EventSubAgentStart
	EventSubAgentEnd

	// Task / objective lifecycle
	EventObjectiveStarted
	EventObjectiveDone

	// Agent reasoning text (LLM's thinking before tool calls)
	EventAgentReasoning

	// Durable marker for an assistant response that contains tool
	// calls. Some local / small models write a short prose preface
	// before the call, while others return a pure function-call
	// message. The tool start/end events update the live dock, but
	// fast tools can complete between animation frames and leave no
	// scrollback trace. This event is emitted once per tool-call batch
	// before execution so operators can see that the model did respond
	// and which tools are being used, without relying on the prose
	// channel to describe side effects.
	EventAgentToolCallBatch

	// Stage transition
	EventTransition

	// Skill binding
	EventSkillBound

	// AnalysisIR task graph ready — emitted once by the orchestrator
	// after the analyze phase succeeds. Carries the derived task node
	// list so the renderer can replace its stage-dispatch task rows
	// with the analyzer's actual task / sub-task breakdown.
	EventAnalysisReady

	// Per-TaskNode lifecycle — emitted from the DAG scheduler as
	// nodes enter / leave the running state. Renderers use these to
	// drive node-row transitions (pending → running → done/failed).
	EventTaskNodeStart
	EventTaskNodeEnd

	// Parallel dispatch lifecycle. Emitted by orchestrator-owned
	// bounded fan-out surfaces, currently the multi-sub_topic explorer
	// dispatch. These events are typed UI telemetry only: orchestration
	// decisions are made before prompt rendering, from TaskGraph data
	// and PipelineSettings.MaxParallelism. Renderers use the group/unit
	// IDs to aggregate concurrent "requesting / receiving / tool" agent
	// activity into honest row-1 prose such as "并行请求模型中 · 2 路",
	// instead of letting the latest worker event overwrite the screen
	// with a misleading single-lane "请求模型中".
	EventParallelDispatchStart
	EventParallelDispatchUnitStart
	EventParallelDispatchUnitEnd
	EventParallelDispatchEnd

	// Live preview of streaming assistant content. Emitted by
	// BaseAgent when the LLM adapter surfaces content chunks mid-
	// response (streaming opt-in). Renderer updates the current
	// task row's detail line in place — does NOT print a new line
	// above the area — so the user sees the reply being typed out
	// without the reasoning feed ballooning.
	EventAgentContent

	// Live preview of the finalizer's `summary` field as the
	// emit_answer_document tool-call arguments stream in. PreviewText
	// carries the cumulative decoded summary so far; PreviewRound
	// labels the draft round (1 for first attempt, 2+ for retries
	// after a contract reject so the user knows the visible text is
	// tentative). Renderer manages a pterm.AreaPrinter dedicated to
	// this preview — first chunk stops the spinner and opens the
	// area, subsequent chunks update it in place.
	//
	// EventLivePreviewClear ends the preview: PreviewRejected
	// indicates whether the round was rejected (true → flash a
	// "已重写" marker briefly, then erase) or accepted/finalised
	// (false → erase). Either way the area is gone before the
	// orchestrator's final RenderAnswerDocument-derived bordered
	// answer prints.
	EventLivePreviewChunk
	EventLivePreviewClear

	// Adapter-level retry / fallback signals. Emitted from inside
	// the LLM adapter retry loop (openai.go) and the FallbackAdapter
	// (fallback.go) just before sleeping / swapping. Without these
	// the renderer dock would keep showing "请求模型中" during the
	// retry window even though the request has already failed and
	// the adapter is sleeping in backoff. RetryAttempt is 1-based
	// (1 = "first try failed, about to retry"); RetryDelaySec is
	// the next sleep duration in seconds; ToolName is overloaded
	// here to carry the new provider's model id when this is a
	// fallback event.
	EventAdapterRetry
	EventAdapterFallback

	// Multi-phase plan-group lifecycle (commit 43, stage II UX
	// fix). Pre-commit-43 the orchestrator surfaced phase
	// progression via generic EventAgentReasoning entries
	// rendered as "⋯ [orchestrator-1] ▶ Phase 1 of 3 ..." —
	// the thinking marker was wrong (it implies LLM
	// reasoning, not a structural progression marker) and the
	// inline-line shape didn't reuse the read-mode sub-topic
	// block style operators were already trained on.
	//
	// EventPhaseGroupStart fires once at runPhaseGroup entry
	// with PhaseList populated — the dock renders a structured
	// enumeration block ("多阶段方案识别到 N 个 phase: ①... ②...")
	// mirroring formatSubTopicsBlock so the visual feels like
	// a sibling of analyzer sub-topic enumeration.
	//
	// EventPhaseProgress fires per-phase at start / accept /
	// reject — the dock renders a short status row using
	// "▶ / ✓ / ✗" icons (NOT the thinking marker) and the same statusObjective
	// / statusDetail color scheme.
	EventPhaseGroupStart
	EventPhaseProgress

	// Acceptance-review lifecycle (commit 44). Pre-commit-44
	// the orchestrator's acceptance-check call (a synchronous
	// 5-30s LLM dispatch in phase_scheduler.go) emitted ZERO
	// events while running — dock row 1 froze at the prior
	// stage's "请求模型中" / "调用工具中" activity for the entire
	// review window, indistinguishable from a stalled request.
	//
	// EventAcceptanceReviewStart fires just before the Check
	// dispatch with PhaseIndex/PhaseTotal populated. Dock
	// flips activity to activityAcceptanceReview ("验收审查中" /
	// "acceptance review") so row 1 honestly reflects what the
	// orchestrator is doing.
	//
	// EventAcceptanceReviewEnd fires after the dispatch
	// returns (success OR failure) so the dock can revert to
	// the inter-phase pause state cleanly. Carries no payload
	// beyond the Kind tag — the per-phase verdict is surfaced
	// separately via EventPhaseProgress (commit 43).
	EventAcceptanceReviewStart
	EventAcceptanceReviewEnd

	// Worktree-provisioning lifecycle. Emitted by applyPreHook
	// around the worktree.Create call so the dock can flip row 1
	// to "准备 worktree 中" / "preparing worktree" while the
	// (fast but visible) git worktree add runs. Without this
	// event pair the dock sat on whatever activity the prior
	// stage left, then jumped straight to "调用工具中" once apply
	// dispatched — the worktree provisioning step itself was
	// invisible.
	EventWorktreePreparingStart
	EventWorktreePreparingEnd

	// Baseline-capturing lifecycle. Emitted by
	// tryBaselineFromCacheThenCapture around the captureBaseline
	// run_tests dispatch so the dock can flip row 1 to "抓取基准" /
	// "capturing baseline" during the (slow, can be tens of seconds)
	// baseline test run. Cache hits are fast and emit no events;
	// only the cache-miss + capture path fires the pair.
	EventBaselineCapturingStart
	EventBaselineCapturingEnd

	// Orchestrator soft notice. Emitted by the scheduler / CGEC
	// enforcers / contract checker / write coordinator when a
	// non-LLM control-flow decision needs to surface to the user
	// (retry hints, forced-read, convergence stall, fallback
	// targets, plan-critic review summary, etc.). Pre-this-event
	// these messages flowed through EventAgentReasoning and were
	// rendered with the same `⋯ <stage> · Round N …` LLM-thinking
	// style the renderer uses for genuine LLM reasoning text —
	// users could not distinguish "the LLM is thinking" from "the
	// orchestrator just decided to retry". The new event renders
	// without the thinking marker or stage-round trace label so the two
	// surfaces are visually distinct.
	//
	// NoticeKind classifies the message into one of three buckets
	// (retry-class yellow / info-class gray / progress-class cyan).
	// Reasoning carries the localized message body produced by
	// internal/orchestrator/user_messages.go (each helper already
	// embeds its own ⟳/·/–/›/⊘ glyph; the renderer adds no further
	// prefix).
	EventOrchestratorNotice
)

// PhaseInfo is the renderable per-phase projection carried on
// EventPhaseGroupStart. Mirrors TaskNodeInfo's shape — the
// dock formats the slice into a circle-marker enumeration
// block visually parallel to the analyzer sub-topic block.
type PhaseInfo struct {
	Index int    // 0-based; circle marker derived as index+1
	Goal  string // single-line phase goal text
}

// PhaseProgressKind classifies an EventPhaseProgress as
// starting / accepted / rejected so the dock picks the right
// icon and color without inspecting the message text.
type PhaseProgressKind int

const (
	PhaseProgressStart PhaseProgressKind = iota
	PhaseProgressAccepted
	PhaseProgressRejected
)

// OrchestratorNoticeKind classifies an EventOrchestratorNotice into
// one of three visual buckets so the dock picks color without parsing
// the message body.
//
//	retry-class (statusRecoverable / yellow):
//	    system is actively recovering / retrying — gives the user a
//	    visible "we are continuing, one more pass" cue.
//	info-class (statusMeta / dark gray):
//	    decision logged for transparency, no further action expected;
//	    quiet so it does not dominate the dock at orchestrator cadence.
//	progress-class (statusObjective / cyan):
//	    forward milestone or active non-retry work; reads as a peer
//	    of the EventPhaseProgress structural progression line.
//
// The kind is enumerated rather than free-form so the formatter is a
// single switch and a code review can verify every emit site picks
// the right bucket. Adding a new kind is a 1-line enum addition + 1
// formatter case + N call-site updates.
type OrchestratorNoticeKind int

const (
	// retry-class — yellow ⟳ (actual retry / re-run / rewrite)
	NoticeRetry                                 OrchestratorNoticeKind = iota // generic stage retry hint
	NoticeForcedRead                                                          // CGEC E2 forced-read fill (progress-class ›)
	NoticeAnswerCheckRetry                                                    // post-finalize answer-contract backtrack
	NoticeFallbackFinalizerOnly                                               // Block 3 fallback: re-run finalizer only
	NoticeFallbackBackToExtract                                               // Block 3 fallback: re-run extract layer
	NoticeFallbackBackToExplore                                               // Block 3 fallback: re-run explore layer
	NoticeFinalizing                                                          // pre-finalize composition cue (progress-class ›)
	NoticeSelfConsistencyStart                                                // self-consistency reviewer dispatch (progress-class ›)
	NoticeSelfConsistencyContradictionRewriting                               // contradictions found, rewriting answer
	NoticeNoToolCall                                                          // must-emit stage got zero tool_calls; re-prompting
	NoticeProceedingWithoutExtract                                            // pre-finalize extract dispatch failed; proceeding with prior evidence (progress-class ›)
	NoticeSemanticQualityReviewStart                                          // P7 (2026-05-10) — semantic-quality reviewer dispatching (progress-class ›)

	// info-class — gray (passive informational)
	NoticeConvergenceStall                   // CGEC I4 plateau finalizing on current evidence
	NoticeAbandonRead                        // forced-read abandoned (file unreadable)
	NoticeFallbackFailLoud                   // Block 3 terminal: shipping with caveat
	NoticeUpstreamCap                        // upstream-fallback budget reached
	NoticeYieldKill                          // yield-delta kill: retry produced no progress
	NoticePlanReview                         // write-mode plan_critic review summary
	NoticeSelfConsistencyContradictionLogged // contradictions found, NOT rewriting (advisory only)
	NoticeFinalizeRepairCap                  // P6/P7 (2026-05-10) — finalize repair-loop hard cap reached; ship doc + residual-concerns caveat (info-class ·)
	NoticeLowGrounding                       // evidence groundedness below advisory profile floor; run continues

	// progress-class — cyan › (forward milestone / current work, not retry)
	NoticeInvestigationReady // explorer signaled investigation_complete

	// 2026-05-08 — multi-repo scan progress (Phase 4 + UX audit).
	// One pre-event when MultiGraph.EnsureLoaded begins building
	// a sub-repo graph and one post-event on completion. The
	// scan-end is split by outcome so the dock can colour the
	// glyph correctly: success → green ✓, failure → red ✗.
	// scan-start uses gray · (in-progress, NOT retry — ⟳ is
	// reserved for retry semantics).
	NoticeMultiRepoScanStart // · scan begins for one sub-repo
	NoticeMultiRepoScanOK    // ✓ scan complete (success)
	NoticeMultiRepoScanFail  // ✗ scan failed

	// 2026-05-17 — single-repo repomap scan progress. Unlike
	// multi-repo scan notices, these include file-count progress from
	// the repomap parser so a large single repository no longer leaves
	// the dock on a vague "preparing pipeline" state.
	NoticeRepoMapScanStart    // · repo index scan begins
	NoticeRepoMapScanProgress // · repo index scan progress
	NoticeRepoMapScanOK       // ✓ repo index ready
	NoticeRepoMapScanFail     // ✗ repo index scan failed
)

// TaskNodeInfo is the renderable summary of a TaskGraph node carried
// on EventAnalysisReady. It is a projection of types.TaskNode onto the
// fields the renderer needs — id for matching, type for row icon /
// ordering, objective for the label. Hidden nodes (counterfactual,
// probe) are filtered out by the orchestrator before this list is
// emitted so the renderer does not need to know the filtering rules.
type TaskNodeInfo struct {
	ID        string
	Type      string
	Objective string
}

// Event is a single lifecycle occurrence emitted by the pipeline.
type Event struct {
	Kind      EventKind
	Timestamp time.Time

	// Pipeline
	TraceID string

	// Stage / agent
	Stage types.PipelineStage
	Agent types.AgentName
	Skill string

	// ReAct iteration (0-based, carried on EventAgentThinking)
	Iteration int

	// LLM request context telemetry. Carried on EventAgentThinking and
	// EventLLMRequestStart. ContextTokensEstimate is a rough pre-flight
	// token estimate for the request about to be sent to the model,
	// including messages plus tool schemas. ContextWindowTokens is the
	// adapter-declared context window. Both are best-effort UI telemetry;
	// zero means "unknown / hide".
	ContextTokensEstimate int
	ContextWindowTokens   int

	// LLM request model telemetry. ModelID is the adapter's effective
	// model identifier for the request being dispatched. Empty means
	// unknown / hide.
	ModelID string

	// Tool call
	ToolName   string
	ToolCallID string
	ToolNames  []string
	ToolDetail string // short arg summary, e.g. file path or command
	// ToolParamsJSON carries raw tool-call parameters for render-only
	// summaries of selected structured tools. It must never drive
	// orchestration logic.
	ToolParamsJSON string
	ToolOK         bool
	ToolTime       time.Duration
	// ToolResultSummary carries the completed tool result's concise
	// textual payload for renderers that surface selected tool-specific
	// user-facing summaries. Empty for tools whose result should remain
	// model-only.
	ToolResultSummary string

	// Sub-agent
	SubAgentName string
	SubAgentID   string
	SubTaskTitle string
	SubTaskCount int

	// Objective (formerly Task)
	Objective string

	// Agent reasoning (think-aloud text from LLM)
	Reasoning string

	// Transition
	FromStage types.PipelineStage
	ToStage   types.PipelineStage
	Reason    string

	// Counts / stats (for end events)
	ToolCallCount int
	MCPCallCount  int
	FactCount     int
	Error         string

	// Analysis-ready payload: the analyzer's task graph projected onto
	// renderer-consumable fields. Populated only on EventAnalysisReady.
	TaskNodes []TaskNodeInfo

	// Per-node lifecycle payload — populated on EventTaskNodeStart /
	// EventTaskNodeEnd. NodeID is the TaskGraph node identifier; the
	// renderer matches these against the TaskNodes list emitted with
	// EventAnalysisReady to locate the row to update.
	NodeID        string
	NodeKind      string
	NodeObjective string

	// NodeSkipped marks an EventTaskNodeEnd that completed via the
	// SkipOnFirstVisit short-circuit (plan loaded from disk for
	// /approve --plan-file) or the --skip-verify short-circuit
	// (verify dispatched without running tests). Renderer uses this
	// to pick a "已加载" / "已跳过" phrase variant instead of the
	// regular "已 X" — the LLM didn't actually run, so claiming the
	// stage's normal completion phrase would mislead the user.
	NodeSkipped bool

	// Parallel dispatch payload. ParallelGroupID is stable for one
	// bounded fan-out wave. ParallelUnitID identifies one worker inside
	// that group, usually the focused explorer dispatch key derived
	// from the TaskNode window. ParallelTotal is the number of units in
	// the wave; Parallelism is the configured concurrent worker cap
	// after clamping. ParallelUnitIDs is declaration-ordered and is
	// populated on EventParallelDispatchStart.
	ParallelGroupID string
	ParallelUnitID  string
	ParallelTotal   int
	Parallelism     int
	ParallelUnitIDs []string

	// EventLivePreviewChunk / EventLivePreviewClear payload.
	//   PreviewText     — cumulative decoded summary text so far
	//   PreviewRound    — 1-based round counter (1 = first finalize
	//                     attempt; ≥2 means earlier rounds were
	//                     rejected by AnswerContract / Tier1 floor)
	//   PreviewRejected — only meaningful on EventLivePreviewClear.
	//                     true → contract rejected this round, area
	//                     gets a "已重写" marker before erase. false
	//                     → final clean erase before bordered answer.
	PreviewText     string
	PreviewRound    int
	PreviewRejected bool

	// Multi-phase plan-group fields (commit 43).
	//   PhaseList         — populated on EventPhaseGroupStart;
	//                       full N-phase enumeration the dock
	//                       renders into a sub-topic-style block.
	//   PhaseIndex        — 0-based phase index for an
	//                       EventPhaseProgress event.
	//   PhaseTotal        — total phase count, ditto.
	//   PhaseProgressKind — start / accepted / rejected.
	//   PhaseDetail       — phase goal (start) or rejection
	//                       reasoning (rejected); empty for
	//                       accepted (terse confirmation).
	PhaseList         []PhaseInfo
	PhaseIndex        int
	PhaseTotal        int
	PhaseProgressKind PhaseProgressKind
	PhaseDetail       string

	// EventAdapterRetry / EventAdapterFallback payload.
	//   RetryAttempt — 1-based index of the failed attempt
	//   RetryDelay   — backoff duration before next attempt
	//   RetryReason  — short human phrase ("rate limit" / "stream stalled")
	//   FallbackFrom / FallbackTo — provider model ids on swap
	RetryAttempt int
	RetryDelay   time.Duration
	RetryReason  string
	FallbackFrom string
	FallbackTo   string

	// EventOrchestratorNotice payload.
	//   NoticeKind  — three-bucket classification (retry / info /
	//                 progress) controlling glyph color without
	//                 parsing the message body. Reasoning carries
	//                 the localized text (with its own embedded
	//                 ⟳/·/–/›/⊘ glyph chosen by user_messages.go).
	NoticeKind OrchestratorNoticeKind
}

// EventEmitter is the callback signature for pipeline event delivery.
// Implementations must be safe for concurrent use.
type EventEmitter func(Event)

// NopEmitter discards all events. Used as the default when no renderer
// is attached so callers never need nil checks.
func NopEmitter(Event) {}
