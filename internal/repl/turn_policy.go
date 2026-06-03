package repl

// turn_policy.go — structured per-turn routing.
//
// The pre-existing ChitchatClassifier emits a 2-valued decision
// (chitchat | repo_question), which is enough to keep casual
// pleasantries off the analysis pipeline but cannot represent a
// short follow-up that should reuse the *previous answer* without
// reading the repo again ("换成 mermaid 图例", "把上面的结论换成
// 表格"). Pre-fix those follow-ups got routed to repo_question and
// triggered a full analyze → explore → extract cycle that re-read
// SKILL.md / unrelated diagnostic files for tens of seconds before
// the finalizer simply restated the previous answer in a new shape.
//
// TurnPolicy is the structured replacement: every turn is one of
// {local, repo, hybrid, clarify, operation} with a small set of orthogonal
// signals (operation / source / confidence + an optional
// presentation_directive). The LLM emits the policy via
// emit_turn_policy; deterministic guards then patch obvious
// self-contradictions before the dispatcher acts on it.
//
// Design rules carried over from chitchat.go:
//   - Schema is the load-bearing contract; no Go-side keyword
//     matching lives here. The structural enum + its description
//     does the disambiguation.
//   - Default LLM impl satisfies BOTH the legacy ChitchatClassifier
//     (Classify) and the new TurnPolicyClassifier (ClassifyPolicy).
//     Tests that wire stubClassifier (legacy interface only) still
//     hit the binary path; production cmd/root.go's *llmChitchatClassifier
//     transparently picks up the structured path.
//   - Fail-safe: any error / parse failure / low confidence demotes
//     to RouteRepo so a broken classifier cannot starve real code
//     questions.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// TurnRoute is the discrete handler the REPL picks per user turn.
type TurnRoute string

const (
	// RouteLocal — the answer can be produced from the user's
	// current message + previous answer + conversation context.
	// No repository read. Dispatched to the local responder.
	RouteLocal TurnRoute = "local"

	// RouteRepo — the answer requires reading repository files.
	// Dispatched to the existing analysis pipeline unchanged.
	RouteRepo TurnRoute = "repo"

	// RouteHybrid — the answer requires both: re-read the
	// repository AND apply a transformation/presentation that
	// came from the previous answer or the user's framing. RouteRepo
	// may also carry presentation_directive when the current fresh
	// investigation itself asks for a specific view. In both cases
	// the dispatcher carries presentation_directive as typed pipeline
	// metadata; the prompt builder renders it separately from the user
	// request body.
	RouteHybrid TurnRoute = "hybrid"

	// RouteClarify — the user's message references state that
	// does not exist (e.g. "上面那条" with no prior answer). The
	// dispatcher prints a clarify message; no LLM call, no
	// pipeline.
	RouteClarify TurnRoute = "clarify"

	// RouteOperation — the turn asks Codrax to perform a computer
	// operation or generate an external artifact (slides, documents,
	// browser/desktop workflow, etc.). This is deliberately separate
	// from RouteRepo: operation tasks may have side effects and
	// artifact verification requirements, so they must not be routed
	// through the source-evidence pipeline by accident.
	RouteOperation TurnRoute = "operation"
)

// TurnPolicy is the structured classification result. All fields
// optional for stub implementations; the dispatcher applies
// ApplyTurnPolicyGuards before acting on any TurnPolicy so missing
// or self-contradictory fields cannot drive a wrong route.
type TurnPolicy struct {
	Route                 TurnRoute
	NeedsRepoAccess       bool
	NeedsOperationAccess  bool
	Operation             string // chat | transform | summarize | translate | elaborate | investigate | computer_operation | artifact_generation | ...
	OperationKind         string // optional more precise operation capability kind
	Source                string // current_message | last_answer | prior_context | repo | mixed
	RiskLevel             string // none | low | medium | high
	SideEffects           []string
	TargetSurface         string // desktop | browser | file_artifact | office_doc | spreadsheet | slides | external_system | unknown
	RequiresConfirmation  bool
	Confidence            float64 // 0..1; <0.4 demotes to repo
	Reason                string
	PresentationDirective string // free-form, e.g. "mermaid", "markdown table", "brief 3-bullet"
}

// TurnPolicyClassifier is the optional extension interface the REPL
// dispatcher prefers when it is satisfied by the wired
// ChitchatClassifier. Returning a non-nil error MUST be treated as
// "fall through to pipeline" by the caller (matching the legacy
// Classify contract).
type TurnPolicyClassifier interface {
	ClassifyPolicy(ctx context.Context, userLine, priorTurnHint string, hasPriorAnswer bool) (TurnPolicy, error)
}

// LocalResponder is the optional extension interface the dispatcher
// prefers when route=RouteLocal and the wired ChitchatResponder
// satisfies it. The contract:
//   - userLine is the trimmed current message,
//   - priorContext is the BuildContext-assembled prior conversation,
//   - lastAnswer is the full text of the most recent assistant
//     response (may be multi-paragraph),
//   - presentationDirective is the directive the classifier emitted
//     (may be empty).
//
// Implementations MUST NOT claim to read the repository, invent
// file paths / line numbers, or introduce evidence not present in
// lastAnswer / priorContext / userLine. The constraint is enforced
// in the system prompt rather than schema because the local path is
// free-form prose, not a tool call.
type LocalResponder interface {
	RespondLocal(ctx context.Context, userLine, priorContext, lastAnswer, presentationDirective string) (string, error)
}

// turnPolicyTool is the schema the structured-policy classifier
// emits. Same provenance as chitchatClassifierTool: kept LOCAL to
// this package, NOT registered in tool.Registry, because the
// classifier bypasses the agent framework and makes one direct
// adapter.Chat call.
//
// Enum values are exhaustive on the routing axis and intentionally
// orthogonal on the descriptive axes (operation / source). The
// description of `route` carries the routing semantics so the LLM
// has the contract inline (no separate prompt-engineering required
// to read the schema).
var turnPolicyTool = llm.ToolSchema{
	Name:        "emit_turn_policy",
	Description: "Classify the user's turn into a structured TurnPolicy that drives REPL routing. Exactly one call per turn.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "route": {
      "type": "string",
      "enum": ["local", "repo", "hybrid", "clarify", "operation"],
      "description": "local = answer from current message + previous answer + conversation context; no repo read and no computer access. repo = run the analysis pipeline for source code OR external observations such as attached logs/traces/MCP rows; analyzer may later exclude current source when the user explicitly asks not to inspect code. hybrid = run the pipeline AND apply a transformation/presentation directive from the previous answer or user framing. clarify = user references missing state or an unsafe/underspecified operation and should be asked for clarification. operation = perform a computer operation or generate an external artifact such as querying the current machine/environment, running local commands, file operations, downloading/installing/uninstalling software, SSH/remote-environment work, or PPT/document/spreadsheet/browser/desktop workflows; it is not a source-code/log/trace evidence investigation. When uncertain about a code/log/trace/MCP evidence question, prefer repo. When uncertain about side effects, prefer clarify."
    },
    "needs_repo_access": {
      "type": "boolean",
      "description": "true iff route is repo or hybrid, or an operation explicitly needs fresh repository facts first. Route=repo also covers pipeline analysis of external observations such as logs/traces even when the analyzer later excludes current source. The dispatcher cross-checks this with route; mismatch demotes to a safe default."
    },
    "needs_operation_access": {
      "type": "boolean",
      "description": "true iff the turn needs a computer/artifact/external-skill operation surface. Set true for route=operation. Do not set it for ordinary source, log, trace, MCP, or other external-observation investigation."
    },
    "operation": {
      "type": "string",
      "enum": ["chat", "transform", "summarize", "translate", "elaborate", "investigate", "computer_operation", "artifact_generation", "presentation_generation", "document_generation", "spreadsheet_generation", "browser_operation", "external_skill_workflow"],
      "description": "chat = greeting / pleasantry / capability question that does not require computer access. transform = change the form of the previous answer (mermaid, table, ...). summarize = shorten the previous answer. translate = render in another language. elaborate = expand on previous answer without new evidence. investigate = fresh code/log/trace/MCP/external-observation investigation through the analysis pipeline. computer_operation/artifact_generation/etc. = operation route candidates that should not be run through the code-evidence pipeline. Questions about the current OS, memory, CPU, GPU, installed tools, paths, versions, or filesystem state are computer_operation when answering them requires local command execution."
    },
    "operation_kind": {
      "type": "string",
      "enum": ["", "computer_operation", "artifact_generation", "presentation_generation", "document_generation", "spreadsheet_generation", "browser_operation", "external_skill_workflow"],
      "description": "Optional precise operation kind. Leave empty for non-operation routes. For route=operation, choose the closest concrete capability."
    },
    "source": {
      "type": "string",
      "enum": ["current_message", "last_answer", "prior_context", "repo", "mixed", "external_tool", "artifact"],
      "description": "Where the answer's content comes from. last_answer = derives from the immediately previous response. prior_context = derives from earlier conversation. repo = requires reading repository files. mixed = combination of the above. external_tool/artifact = external observation, operation result, or external skill result."
    },
    "risk_level": {
      "type": "string",
      "enum": ["", "none", "low", "medium", "high"],
      "description": "Operation risk. Empty or none for non-operation routes. Use high for publishing, sending, deleting, destructive desktop changes, or irreversible external-system actions."
    },
    "side_effects": {
      "type": "array",
      "items": {"type":"string", "enum":["local_file_write", "desktop_ui", "browser_ui", "network_read", "network_submit", "external_system_write", "package_install", "package_uninstall", "remote_exec", "destructive"]},
      "description": "Operation side effects. Empty for non-operation routes. local_file_write covers creating files such as PPTX/DOCX/XLSX; package_install/package_uninstall cover installing or removing software; remote_exec covers SSH or remote-environment command execution; network_submit/destructive require confirmation."
    },
    "target_surface": {
      "type": "string",
      "enum": ["", "desktop", "browser", "file_artifact", "office_doc", "spreadsheet", "slides", "external_system", "unknown"],
      "description": "Primary operation surface. Empty for non-operation routes."
    },
    "requires_confirmation": {
      "type": "boolean",
      "description": "true when the operation is high risk, writes to external systems, is destructive, or needs user consent before execution. false for ordinary code investigation and low-risk local artifact generation."
    },
    "confidence": {
      "type": "number",
      "description": "Self-rated confidence in the classification, 0..1. Below 0.4 the dispatcher demotes to repo (safe default)."
    },
    "reason": {
      "type": "string",
      "description": "One short sentence naming the structural signal that justified the decision. Not shown to the user."
    },
    "presentation_directive": {
      "type": "string",
      "description": "Optional. Free-form directive describing the desired final-answer form ('mermaid', 'markdown table', 'brief 3-bullet summary', 'logic flow diagram'). Echoed verbatim into the local responder's system prompt when local, or carried as typed pipeline metadata when repo/hybrid. Preserve the user's wording and language when deriving it from the current message; do not translate Chinese user phrasing into English. It must not be prepended to or rewrite the user request body. Omit when not applicable."
    }
  },
  "required": ["route", "needs_repo_access", "operation", "source", "confidence", "reason"]
}`),
}

// turnPolicySystemPrompt is the LLM-facing routing rule sheet. The
// examples are illustrative on the SHAPE of inputs the classifier
// will see — not an exhaustive list. Each example pairs a user
// phrase with the expected route + (where applicable) the
// presentation_directive. Future additions go through the same
// shape so the prompt grows without coupling to keyword tables.
const turnPolicySystemPrompt = `You route each user turn in a code-analysis REPL into a structured TurnPolicy and emit it via emit_turn_policy.

The five routes:

  local   — the answer can be produced from the user's CURRENT MESSAGE
            plus the PREVIOUS ANSWER and CONVERSATION CONTEXT. The
            dispatcher will NOT read repository files. Pick this for
            transformations / summaries / translations / elaborations
            that operate on the previous answer, and for greetings /
            capability questions that don't reference the codebase or
            require current-machine/computer access.

  repo    — the answer requires reading repository files. The
            dispatcher runs the full analysis pipeline. Pick this for
            any fresh code investigation, or when the user explicitly
            asks to re-read / re-confirm the repository. Also pick this
            for fresh LOG / TRACE / MCP / connector external-observation
            analysis: the same pipeline owns log_triage, perf_triage,
            trace_query, ObservationLedger, and final answer grounding.
            If the user explicitly says not to inspect current code,
            still use this analysis pipeline; the analyzer will carry
            that as an external-observation / no-current-source policy.
            Do NOT reroute external-observation analysis to operation
            merely because it should avoid current source.

  hybrid  — the answer requires BOTH: re-read the repository AND apply
            a presentation/transformation that came from the previous
            answer or the user's framing. Example: "把上面的流程图换
            成 mermaid，并重新读仓库确认有没有 IO 分析" — the user
            wants fresh repo evidence (IO 分析) AND a mermaid
            rendering. Set presentation_directive on this route.

  clarify — the user's message references state that does not exist
            in this session: e.g. "换成 mermaid 图例" / "把上面的结论
            换成表格" / "再扩展一下" when there is NO previous answer
            yet (first turn / cleared memory), OR the user asks for a
            high-risk/underspecified operation that needs confirmation
            or missing details. The dispatcher prints a clarify message
            and does NOT call the LLM again.

  operation — the answer requires a computer operation or artifact
            generation surface, independent of source-code evidence:
            querying the current machine/environment, running local
            commands, inspecting installed tools/versions/filesystem
            state, moving/copying/searching files, downloading files,
            installing or uninstalling packages, operating a remote
            environment through SSH or similar tools, creating
            slides/PPT, documents, spreadsheets, browser or desktop
            workflows, or invoking an external skill workflow.
            This route is NOT for explaining code, logs, traces, MCP rows,
            connector observations, or other evidence artifacts.
            Explicit command-operation file reads/searches/extractions
            stay on route=operation even when the file path is inside
            the current repository; do not reinterpret them as source
            investigation unless the user asks to explain source code,
            architecture, implementation, trace/log root cause, or other
            runtime/external observation semantics. The deciding factor is
            the requested goal: direct computer/file operation = operation;
            evidence interpretation/diagnosis/root-cause = pipeline.
            High-risk or forbidden command-operation requests still use
            route=operation with risk_level=high and requires_confirmation=true;
            the deterministic operation policy will block dangerous commands.
            Do not reroute unsafe computer-operation requests into repo/source
            analysis just because they are unsafe.
            Clear non-code computer operations remain route=operation
            even when they mention commands, package managers, SSH,
            browsers, files, or external systems. Use typed risk and
            side_effects to describe safety; the deterministic
            operation policy decides auto-run, manual approval, or
            denial.
            If the operation must first read repository facts (for
            example "based on this repo, generate a PPT"), set
            needs_repo_access=true as an additional typed signal, but
            keep route=operation so the dispatcher can use the
            operation pipeline once enabled.

needs_repo_access is true iff route ∈ {repo, hybrid}, or route=operation
needs fresh repository facts before producing an artifact. The dispatcher
re-checks this and corrects mismatches.

needs_operation_access is true iff route=operation. Do not set it for
ordinary source, log, trace, MCP, connector, or attached-artifact
external-observation investigation.

operation:
  chat        — greetings, pleasantries, capability/identity questions
                that do not require repository or computer access
                ("你好", "你能做什么", "你是谁")
  transform   — change the form of the previous answer (render as
                mermaid, render as table, reorganise as bullet list)
  summarize   — shorten the previous answer
  translate   — render the previous answer in another language
  elaborate   — expand on the previous answer without new evidence
  investigate — fresh code investigation that needs repo reads
  computer_operation — operate desktop/browser/UI or external tools,
                or run local commands to inspect the current machine,
                environment, installed software, filesystem, versions,
                OS, memory, CPU, GPU, network status, or similar
                runtime facts; also covers package management,
                downloads, file moves/copies, SSH/remote shell work,
                and other explicit non-code computer tasks
  artifact_generation — create a local output artifact
  presentation_generation — create slides/PPT
  document_generation — create a document
  spreadsheet_generation — create or modify a spreadsheet
  browser_operation — operate a browser
  external_skill_workflow — run a configured external skill workflow

operation_kind mirrors the operation capability for route=operation;
leave it empty otherwise.

risk_level / side_effects / target_surface / requires_confirmation:
  - non-operation routes: risk_level="none" or "", side_effects=[],
    target_surface="", requires_confirmation=false.
  - low-risk local artifact generation: risk_level=low,
    side_effects may include local_file_write, confirmation usually false.
  - desktop/browser manipulation: include desktop_ui or browser_ui.
  - software install/uninstall: include package_install or
    package_uninstall and normally requires_confirmation=true.
  - SSH or remote command execution: include remote_exec and normally
    requires_confirmation=true.
  - network submission, external writes, deletes, irreversible changes:
    risk_level=high and requires_confirmation=true.

source:
  current_message — answer derives from the user's current input alone
                    (greetings, capability questions)
  last_answer     — answer derives from the previous response
                    (transform, summarize, translate, elaborate)
  prior_context   — answer derives from earlier conversation (not
                    just the immediate previous turn)
  repo            — answer requires repository access
  mixed           — combination (typical for hybrid)
  external_tool   — derives from an external tool/skill
  artifact        — derives from an output artifact to be produced

confidence: 0..1 self-rating. Below 0.4 the dispatcher demotes to
repo because the cost of being wrong is higher for local / hybrid
(may produce a bogus answer based on missing context) and operation
(may choose a side-effecting route) than for repo (merely wastes cycles
re-reading).

presentation_directive: free-form text echoed verbatim into the
local responder's system prompt (when route=local) OR carried as
typed pipeline metadata (when route=repo/hybrid). Use it for any
current-turn display request, including fresh investigations that ask
for a logic view / table / diagram. Preserve the user's original
wording and language when possible: if the current message says
"详细的设计文档", emit "详细的设计文档", not "detailed design
document". Do NOT rewrite or prepend the user request with this
directive. Examples:
  "mermaid sequence diagram"
  "markdown table with columns: file, function, behaviour"
  "brief 3-bullet summary"
  "翻译成英文"
Omit (empty string) when no directive applies.

priorTurn / last_answer_present signals (rendered ABOVE the current
message when present):
  - last_answer_present=true means a previous assistant answer
    exists in this session. References to "上面/前面/that one/the
    previous" can be honoured.
  - last_answer_present=false means there is no previous answer.
    Any request that targets a previous answer (transform /
    summarize / translate / elaborate of "上面的") MUST route to
    clarify.
  - priorTurn carries the prior turn's kind + topic. Use only for
    disambiguation; do not over-weight it.
  - attachment=true on priorTurn means a runtime log / perf trace
    is sticky on this REPL session. The local handler does NOT
    consume attachments — they're only processed by the pipeline's
    log_triage / perf_triage pre-stages. Routing rules:
      * If the current message references the attachment content
        (the user is asking about the panic / exception / metric /
        error / behaviour described by the attachment), route to
        repo (fresh investigation) or hybrid (when a presentation
        directive is also requested). The pipeline reads the
        attachment.
      * If the current message is a transform / summarize / etc.
        of a PREVIOUS answer that already covered the attachment
        (last_answer_present=true), route to local — the previous
        answer already encodes the attachment-derived facts, so
        the transformation can reuse it without re-reading the
        attachment.
      * If the current message is unrelated to the attachment
        (memory-meta, greeting, capability question, transformation
        of a non-attachment-related prior answer), classify by the
        current message normally; the attachment stays sticky for
        the next pipeline turn.
      * Treat attachment=true as a SOFT signal: weight toward
        repo/hybrid only when the message references the attached
        content. Never as a hard route — a clear non-attachment
        signal in the message itself wins.

Examples (illustrative, NOT exhaustive — judge by structure):

  Current: "换成 mermaid 图例" + last_answer_present=true
    → route=local, operation=transform, source=last_answer,
      presentation_directive="mermaid 图例", confidence≈0.9

  Current: "把上面的结论换成表格" + last_answer_present=true
    → route=local, operation=transform, source=last_answer,
      presentation_directive="markdown 表格", confidence≈0.9

  Current: "重新读一下仓库确认这个流程"
    → route=repo, operation=investigate, source=repo,
      confidence≈0.85

  Current: "只分析这个 trace，不要看代码，找一下 jank 原因"
    → route=repo, needs_operation_access=false,
      operation=investigate, source=artifact,
      confidence≈0.9

  Current: "只看这段客户日志，不要读取源码，判断系统 gap"
    → route=repo, needs_operation_access=false,
      operation=investigate, source=artifact,
      confidence≈0.9

  Current: "根据 MCP 返回的外部观测解释现象，不要看代码"
    → route=repo, needs_operation_access=false,
      operation=investigate, source=external_tool,
      confidence≈0.85

  Current: "把上面的流程换成 mermaid，同时重新读仓库确认有没
            有 IO 分析" + last_answer_present=true
    → route=hybrid, operation=investigate, source=mixed,
      presentation_directive="mermaid 流程图", needs_repo_access=true,
      confidence≈0.85

  Current: "你好" (any state)
    → route=local, operation=chat, source=current_message,
      confidence≈0.95

  Current: "当前机器是什么操作系统，内存多大，CPU/GPU 信息是什么"
    → route=operation, needs_repo_access=false,
      needs_operation_access=true,
      operation=computer_operation,
      operation_kind=computer_operation,
      source=current_message, risk_level=low,
      side_effects=[], target_surface=desktop,
      requires_confirmation=false, confidence≈0.85

  Current: "查一下本机 go/node/python 版本和可执行路径"
    → route=operation, needs_repo_access=false,
      needs_operation_access=true,
      operation=computer_operation,
      operation_kind=computer_operation,
      source=current_message, risk_level=low,
      side_effects=[], target_surface=desktop,
      requires_confirmation=false, confidence≈0.85

  Current: "帮我安装 ffmpeg 并确认版本"
    → route=operation, needs_repo_access=false,
      needs_operation_access=true,
      operation=computer_operation,
      operation_kind=computer_operation,
      source=current_message, risk_level=medium,
      side_effects=["package_install"], target_surface=desktop,
      requires_confirmation=true, confidence≈0.85

  Current: "通过 ssh 登到测试机检查磁盘空间"
    → route=operation, needs_repo_access=false,
      needs_operation_access=true,
      operation=computer_operation,
      operation_kind=computer_operation,
      source=current_message, risk_level=medium,
      side_effects=["remote_exec"], target_surface=external_system,
      requires_confirmation=true, confidence≈0.85

  Current: "换成 mermaid 图例" + last_answer_present=false
    → route=clarify, operation=transform, source=last_answer,
      reason="user references previous answer but none exists",
      confidence≈0.9

  Current: "HiTraceAnalyzer 处理鸿蒙 trace 的流程是怎样的"
    → route=repo, operation=investigate, source=repo,
      confidence≈0.9

  Current: "生成一份关于这个项目架构的 PPT"
    → route=operation, needs_repo_access=true,
      needs_operation_access=true,
      operation=presentation_generation,
      operation_kind=presentation_generation,
      source=mixed, risk_level=low,
      side_effects=["local_file_write"], target_surface=slides,
      requires_confirmation=false, confidence≈0.85

  Current: "请作为电脑操作读取 docs/design/foo.md，提取某段任务，
            不要分析源码"
    → route=operation, needs_repo_access=false,
      needs_operation_access=true,
      operation=computer_operation,
      operation_kind=computer_operation,
      source=current_message, risk_level=low,
      side_effects=[], target_surface=desktop,
      requires_confirmation=false, confidence≈0.85

  Current: "打开浏览器登录后台并删除这个项目"
    → route=clarify OR route=operation with
      risk_level=high, side_effects=["browser_ui","network_submit","destructive"],
      requires_confirmation=true. If key details or consent are missing,
      prefer clarify.

When uncertain (truly ambiguous), pick repo with confidence ≤0.5 —
the safe default for possible code questions. For possible operations with
unclear side effects, pick clarify.`

// ClassifyPolicy is the structured-output classifier path. Returns
// the parsed TurnPolicy verbatim — guards are applied by the caller
// (dispatch) so callers also see the raw model output for telemetry.
//
// Error returns: any path that cannot produce a valid TurnPolicy
// surfaces as an error so the dispatcher's gate falls through to
// pipeline (matching the legacy Classify contract). This includes:
// nil adapter, empty userLine, chat error, no tool call, wrong tool
// name, malformed params JSON, unknown route enum.
func (c *llmChitchatClassifier) ClassifyPolicy(ctx context.Context, userLine, priorTurnHint string, hasPriorAnswer bool) (TurnPolicy, error) {
	var zero TurnPolicy
	if ctx == nil {
		ctx = context.Background()
	}
	if c.adapter == nil {
		return zero, fmt.Errorf("turn-policy classifier not configured: no LLM adapter")
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return zero, fmt.Errorf("turn-policy classifier: empty user line")
	}

	// Build user content. Same shape as the binary classifier:
	// optional priorTurn header, then explicit last_answer_present
	// flag (load-bearing — the system prompt teaches the LLM that
	// false routes "transform of 上面" to clarify), then the
	// current message section.
	var b strings.Builder
	if hint := strings.TrimSpace(priorTurnHint); hint != "" {
		b.WriteString("## priorTurn: ")
		b.WriteString(hint)
		b.WriteString("\n")
	}
	if hasPriorAnswer {
		b.WriteString("## last_answer_present: true\n\n")
	} else {
		b.WriteString("## last_answer_present: false\n\n")
	}
	b.WriteString("## current: ")
	b.WriteString(userLine)

	messages := []llm.Message{
		{Role: "system", Content: turnPolicySystemPrompt},
		{Role: "user", Content: b.String()},
	}
	tools := []llm.ToolSchema{turnPolicyTool}
	resp, err := c.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return zero, fmt.Errorf("turn-policy classifier llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return zero, fmt.Errorf("turn-policy classifier: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != turnPolicyTool.Name {
		return zero, fmt.Errorf("turn-policy classifier: unexpected tool %q", call.Name)
	}
	var parsed struct {
		Route                 string                   `json:"route"`
		NeedsRepoAccess       flexiblePolicyBool       `json:"needs_repo_access"`
		NeedsOperationAccess  flexiblePolicyBool       `json:"needs_operation_access"`
		Operation             string                   `json:"operation"`
		OperationKind         string                   `json:"operation_kind"`
		Source                string                   `json:"source"`
		RiskLevel             string                   `json:"risk_level"`
		SideEffects           flexiblePolicyStringList `json:"side_effects"`
		TargetSurface         string                   `json:"target_surface"`
		RequiresConfirmation  flexiblePolicyBool       `json:"requires_confirmation"`
		Confidence            flexiblePolicyFloat      `json:"confidence"`
		Reason                string                   `json:"reason"`
		PresentationDirective string                   `json:"presentation_directive"`
	}
	if err := unmarshalTurnPolicyParams(call.Params, &parsed); err != nil {
		return zero, fmt.Errorf("turn-policy classifier: unmarshal tool params: %w", err)
	}
	route := TurnRoute(parsed.Route)
	switch route {
	case RouteLocal, RouteRepo, RouteHybrid, RouteClarify, RouteOperation:
	default:
		return zero, fmt.Errorf("turn-policy classifier: unknown route %q", parsed.Route)
	}
	operation := strings.TrimSpace(parsed.Operation)
	operationKind := strings.TrimSpace(parsed.OperationKind)
	if operationKind == "" && isOperationLikeOperation(operation) {
		operationKind = operation
	}
	return TurnPolicy{
		Route:                 route,
		NeedsRepoAccess:       bool(parsed.NeedsRepoAccess),
		NeedsOperationAccess:  bool(parsed.NeedsOperationAccess),
		Operation:             operation,
		OperationKind:         operationKind,
		Source:                strings.TrimSpace(parsed.Source),
		RiskLevel:             strings.TrimSpace(parsed.RiskLevel),
		SideEffects:           []string(parsed.SideEffects),
		TargetSurface:         strings.TrimSpace(parsed.TargetSurface),
		RequiresConfirmation:  bool(parsed.RequiresConfirmation),
		Confidence:            float64(parsed.Confidence),
		Reason:                strings.TrimSpace(parsed.Reason),
		PresentationDirective: strings.TrimSpace(parsed.PresentationDirective),
	}, nil
}

// flexiblePolicyBool/flexiblePolicyFloat/flexiblePolicyStringList are local to
// the direct REPL classifier path. emit_turn_policy is not registered in the
// agent tool registry, so it cannot use BaseAgent's tool-param compatibility
// normalizer. Keeping these permissive decoders here gives the same resilience
// for common LLM JSON slips (string booleans, string numbers, scalar lists)
// without changing any production agent tool surface.
type flexiblePolicyBool bool

type flexiblePolicyString string

func (s *flexiblePolicyString) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		*s = flexiblePolicyString(strings.TrimSpace(str))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		*s = flexiblePolicyString(number.String())
		return nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			*s = "true"
		} else {
			*s = "false"
		}
		return nil
	}
	return fmt.Errorf("invalid string value %s", string(raw))
}

func (b *flexiblePolicyBool) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*b = false
		return nil
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err == nil {
		*b = flexiblePolicyBool(v)
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "", "false", "0", "no", "n":
			*b = false
			return nil
		case "true", "1", "yes", "y":
			*b = true
			return nil
		}
	}
	return fmt.Errorf("invalid boolean value %s", string(raw))
}

type flexiblePolicyFloat float64

func (f *flexiblePolicyFloat) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*f = 0
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		v, err := strconv.ParseFloat(number.String(), 64)
		if err == nil {
			*f = flexiblePolicyFloat(v)
			return nil
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err == nil {
			*f = flexiblePolicyFloat(v)
			return nil
		}
	}
	return fmt.Errorf("invalid number value %s", string(raw))
}

type flexiblePolicyStringList []string

func (l *flexiblePolicyStringList) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*l = nil
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		*l = cleanPolicyStringList(arr)
		return nil
	}
	var arrAny []any
	if err := json.Unmarshal(raw, &arrAny); err == nil {
		out := make([]string, 0, len(arrAny))
		for _, v := range arrAny {
			switch x := v.(type) {
			case string:
				out = append(out, x)
			case float64:
				out = append(out, strconv.FormatFloat(x, 'f', -1, 64))
			case bool:
				out = append(out, strconv.FormatBool(x))
			case nil:
				continue
			default:
				encoded, err := json.Marshal(x)
				if err == nil {
					out = append(out, string(encoded))
				}
			}
		}
		*l = cleanPolicyStringList(out)
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		*l = cleanPolicyStringList(splitPolicyStringList(s))
		return nil
	}
	return fmt.Errorf("invalid string list value %s", string(raw))
}

func splitPolicyStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '，' || r == '；' || r == '|'
	})
	if len(parts) == 0 {
		return []string{s}
	}
	return parts
}

func cleanPolicyStringList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func unmarshalTurnPolicyParams(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err == nil {
		return nil
	}
	repaired, ok := repairTurnPolicyParamsJSON(raw)
	if !ok {
		return json.Unmarshal(raw, v)
	}
	if err := json.Unmarshal(repaired, v); err != nil {
		return err
	}
	logging.Warning("[repl/turn_policy] emit_turn_policy params auto-repaired (LLM-corrupted JSON: structural repair)")
	return nil
}

func repairTurnPolicyParamsJSON(raw []byte) ([]byte, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, false
	}
	if json.Valid(raw) {
		return raw, false
	}
	if obj, ok := firstJSONObject(raw); ok && json.Valid(obj) {
		return obj, !bytes.Equal(obj, raw)
	}
	return nil, false
}

func firstJSONObject(raw []byte) ([]byte, bool) {
	start := bytes.IndexByte(raw, '{')
	if start < 0 {
		return nil, false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				obj := raw[start : i+1]
				return stripPreTerminatorCommas(obj), true
			}
		}
	}
	return nil, false
}

func stripPreTerminatorCommas(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inString {
			out = append(out, c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(raw) && (raw[j] == ' ' || raw[j] == '\n' || raw[j] == '\r' || raw[j] == '\t') {
				j++
			}
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// turnPolicyConfidenceFloor is the threshold below which the
// dispatcher demotes any non-repo route to repo. 0.4 is deliberate
// — ANY classifier that hesitates on a turn earns the safe default.
// Above 0.4 the model has committed and we honour it.
const turnPolicyConfidenceFloor = 0.4

// presentationDirectiveCap caps the runes of the LLM-emitted
// directive that propagate downstream. Defends against a runaway
// LLM that produces multi-paragraph "directives" — these would be
// embedded into the local responder system prompt or the pipeline
// request body, so an unbounded directive can both inflate the
// prompt budget and confuse the recipient. 200 runes accommodates
// "mermaid sequence diagram with X / Y / Z swimlanes" while
// rejecting ASCII-art or full-paragraph instructions.
const presentationDirectiveCap = 200

// ApplyTurnPolicyGuards patches obvious self-contradictions in a
// TurnPolicy before the dispatcher acts on it. The guards are
// deterministic and structural — no keyword matching. Each guard
// covers a SPECIFIC failure mode the LLM may produce; documented
// inline so future maintainers can audit each rule against the
// production trace it was added for.
//
// hasAttachment is the structural fact that a runtime log / perf
// trace is sticky on the REPL session this turn (sticky /log,
// sticky /htrace, or splitPastedLog auto-route). The guard pair
// for it lives at the bottom: an unread attachment + no prior
// answer + route=local is structurally impossible to fulfill (the
// local responder cannot consume the attachment), so the route
// is demoted to repo.
func ApplyTurnPolicyGuards(p TurnPolicy, hasPriorAnswer, hasAttachment bool) TurnPolicy {
	// Cap the directive length so a runaway LLM cannot inflate
	// the downstream prompt. Truncate on rune boundary (UTF-8
	// safe) and add ellipsis so a downstream reader can tell
	// the directive was clipped.
	if r := []rune(p.PresentationDirective); len(r) > presentationDirectiveCap {
		p.PresentationDirective = string(r[:presentationDirectiveCap]) + "…"
	}

	// Self-contradiction: route says no repo but
	// needs_repo_access is true. The boolean is the more
	// reliable signal (it's a primary axis), so demote local→repo
	// or clarify→repo. Hybrid is consistent and unchanged. Operation
	// keeps its route because "read repo first, then generate an
	// artifact" is a valid mixed operation shape; later operation
	// dispatch decides how to satisfy that typed request.
	if p.Route == RouteLocal && p.NeedsRepoAccess {
		p.Route = RouteRepo
	}
	if p.Route == RouteClarify && p.NeedsRepoAccess {
		// User asked for repo investigation but the model also
		// flagged missing state — prefer the repo path since the
		// pipeline can read the actual code regardless of
		// conversation continuity.
		p.Route = RouteRepo
	}

	// Self-contradiction on the operation axis: a route or boolean hints at
	// operation access, but the typed payload says this is an analysis-only
	// investigation over repo / runtime artifact / external observation
	// facts. This can happen when a user explicitly says "do not inspect
	// source code" and the classifier drifts toward "operation" even though
	// the analysis pipeline is still the right lane for log/trace/MCP/
	// connector evidence. Prefer the typed operation/source/side-effect tuple
	// over a lone route enum or boolean.
	if isAnalysisOnlyPolicy(p) && (p.Route == RouteOperation || p.NeedsOperationAccess) {
		if p.Route == RouteOperation {
			p.Route = RouteRepo
			p.NeedsRepoAccess = true
		}
		p.NeedsOperationAccess = false
	}

	// Self-contradiction on the operation axis: the route says local/repo,
	// but the typed operation fields say a computer/artifact operation is
	// needed. Trust the typed operation signal rather than sending the turn
	// to local chat or source analysis. This is structural, not prose-based:
	// it only consumes schema fields emitted by the classifier.
	if p.Route != RouteOperation && hasOperationSignal(p) {
		p.Route = RouteOperation
		p.NeedsOperationAccess = true
	}

	// Self-contradiction the other way: route says repo/hybrid
	// but needs_repo_access is false. Trust the route enum (it's
	// the explicit decision); patch needs_repo_access.
	if p.Route == RouteRepo || p.Route == RouteHybrid {
		p.NeedsRepoAccess = true
	}
	if p.Route == RouteLocal || p.Route == RouteClarify {
		p.NeedsRepoAccess = false
	}
	if p.Route == RouteOperation {
		p.NeedsOperationAccess = true
		if p.OperationKind == "" && isOperationLikeOperation(p.Operation) {
			p.OperationKind = p.Operation
		}
		if p.RiskLevel == "" {
			p.RiskLevel = "low"
		}
		if p.TargetSurface == "" {
			p.TargetSurface = "unknown"
		}
	} else {
		p.NeedsOperationAccess = false
		if !isOperationLikeOperation(p.Operation) {
			p.OperationKind = ""
		}
		if p.RiskLevel == "" {
			p.RiskLevel = "none"
		}
		if len(p.SideEffects) == 0 {
			p.SideEffects = nil
		}
		if p.TargetSurface == "" {
			p.TargetSurface = ""
		}
	}

	// Missing prior-answer guard. When the LLM picks a route that
	// references the previous answer (transform / summarize /
	// translate / elaborate of "上面的") but no prior answer
	// exists, redirect to clarify. Keeps the local responder from
	// hallucinating a "previous answer" out of thin air.
	needsPriorAnswer := p.Source == "last_answer" ||
		p.Operation == "transform" ||
		p.Operation == "summarize" ||
		p.Operation == "translate" ||
		p.Operation == "elaborate"
	if !hasPriorAnswer && needsPriorAnswer && (p.Route == RouteLocal || p.Route == RouteHybrid) {
		p.Route = RouteClarify
		p.NeedsRepoAccess = false
	}

	// Confidence floor. Below the floor, demote local/operation to repo
	// (safe default for possibly-code questions). Hybrid stays hybrid
	// because it already runs the pipeline; clarify stays clarify because
	// running the pipeline against a "上面的" reference would produce a
	// worse answer than asking the user to re-state.
	if p.Confidence > 0 && p.Confidence < turnPolicyConfidenceFloor {
		if p.Route == RouteLocal || p.Route == RouteOperation {
			p.Route = RouteRepo
			p.NeedsRepoAccess = true
			p.NeedsOperationAccess = false
		}
	}

	// Unread-attachment guard. The local responder reuses ONLY the
	// previous answer text — it cannot read the attached log /
	// perf trace because those are consumed by log_triage /
	// perf_triage inside the pipeline (chitchat path explicitly
	// ignores them per chitchatDispatch). So when there is a
	// sticky attachment AND no prior answer that already covers
	// it, route=local cannot produce a faithful reply: it would
	// either fabricate around the attachment or ignore it. Demote
	// to repo so the pre-stages process the attachment normally.
	//
	// hasPriorAnswer=true is the load-bearing carve-out: a
	// previous pipeline turn already consumed the attachment and
	// produced an answer; "换成 X 形式" of that answer is a
	// legitimate transform that does NOT need the attachment to
	// be re-read. This is the structural difference between
	// "fresh attachment" and "transform of attachment-derived
	// answer". Same shape as the no-prior guard above: the
	// dispatcher's two facts (hasPriorAnswer × hasAttachment)
	// classify the four cases without any keyword matching.
	if hasAttachment && !hasPriorAnswer && p.Route == RouteLocal {
		p.Route = RouteRepo
		p.NeedsRepoAccess = true
	}
	return p
}

// composeEffectiveRequest assembles the runner-bound request from
// (a) the optional prior-conversation block from BuildContext and
// (b) the user's original request line. Presentation directives are
// carried out-of-band via presentationDirectiveSetter; they must not
// be concatenated into the objective because the objective feeds the
// UI status line, repo_map task maps, and memory.
//
// This helper is the SINGLE site that synthesises the effective
// request; callers no longer mutate `line` directly. The sole
// caller is dispatch(); test code reaches it via that path.
func composeEffectiveRequest(prior, line string) string {
	p := strings.TrimSpace(prior)
	if p == "" {
		return line
	}
	var b strings.Builder
	b.WriteString("## Prior conversation\n")
	b.WriteString(p)
	b.WriteString("\n\n")
	b.WriteString("## Current request\n")
	b.WriteString(line)
	return b.String()
}

func isOperationLikeOperation(op string) bool {
	switch strings.TrimSpace(op) {
	case "computer_operation", "artifact_generation", "presentation_generation",
		"document_generation", "spreadsheet_generation", "browser_operation",
		"external_skill_workflow":
		return true
	default:
		return false
	}
}

func hasOperationSignal(p TurnPolicy) bool {
	return p.NeedsOperationAccess ||
		isOperationLikeOperation(p.Operation) ||
		isOperationLikeOperation(p.OperationKind)
}

func isAnalysisOnlyPolicy(p TurnPolicy) bool {
	if strings.TrimSpace(p.Operation) != "investigate" {
		return false
	}
	if isOperationLikeOperation(p.OperationKind) {
		return false
	}
	if len(p.SideEffects) > 0 {
		return false
	}
	switch strings.TrimSpace(p.TargetSurface) {
	case "", "unknown":
	default:
		return false
	}
	switch strings.TrimSpace(p.RiskLevel) {
	case "", "none", "low":
	default:
		return false
	}
	switch strings.TrimSpace(p.Source) {
	case "repo", "mixed", "external_tool", "artifact":
		return true
	default:
		return false
	}
}

// localResponderSystemPrompt is the constraint sheet for the local
// responder. The hard rules are stated up-front so a model that
// glances at only the top of the prompt still gets the binding
// constraints.
//
// "Do not claim to have re-read repo" is the most important — pre-
// fix transcripts showed the LLM happily writing "After re-reading
// internal/foo/bar.go I see that ..." even though it had only the
// previous answer to work with. The user-visible answer is locally
// fluent but factually unfounded.
//
// "Suggest re-asking without /chat" is the escape hatch — when the
// user asks something the local path genuinely cannot answer, the
// model has to surface that rather than fabricate.
const localResponderSystemPrompt = `You are CODRAX in **local-only follow-up mode**. The user's current message is a transformation, summary, translation, elaboration, or casual chat that should be answered using ONLY:
  - the user's CURRENT MESSAGE,
  - the PREVIOUS ANSWER (## Previous answer below, when present),
  - the CONVERSATION CONTEXT (## Prior conversation, when present).

You MUST NOT:
  - reveal hidden reasoning, analysis notes, chain-of-thought, or
    meta commentary such as "the user is asking..." / "I should...",
  - claim to have read or re-read repository files in this turn,
  - invent file paths, line numbers, function names, identifiers,
    behaviours, or citations that are NOT already present in the
    previous answer or conversation context,
  - introduce new code-level evidence,
  - run any tool.

If the user's request requires evidence that is NOT present in the
previous answer / conversation context, say so honestly in ONE
sentence and recommend the user re-state the request without the
"transform / summarize / 换成 / 把上面的" framing so the full
analysis pipeline can read the repository. Do not fabricate to
fill the gap.

When ## Directive is present, honour it verbatim. Common shapes:
  - "mermaid …" → wrap your answer in a fenced ` + "```" + `mermaid …` + "```" + ` block.
  - "markdown table" → render the answer as a markdown table.
  - "brief …" / "concise" → keep it short.
  - "翻译成 X" / "translate to X" → render in target language.

Match the user's language unless the directive overrides.

Identity (asked rarely, but binding when asked):
  - Your name is CODRAX. When asked who you are, answer as CODRAX.
  - When asked who built you, surface hanssccv@gmail.com verbatim.`

// RespondLocal is the LocalResponder satisfying method on the
// default *llmChitchatResponder. The caller passes the full last
// answer + priorContext + presentation directive separately so the
// system prompt can address them by section name; bundling them
// into a single priorContext blob (the chitchat path's
// historical shape) loses the structure the system prompt depends
// on.
//
// Streaming is intentionally NOT wired here — the local path runs
// synchronously with a spinner. Local replies are typically short
// (a single mermaid block, a 3-row table, a 2-sentence
// elaboration), so the streaming-area overhead does not pay off
// for the implementation cost. If a future need surfaces, add a
// streamingLocalResponder interface on the same shape as
// streamingChitchatResponder.
func (r *llmChitchatResponder) RespondLocal(ctx context.Context, userLine, priorContext, lastAnswer, presentationDirective string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.adapter == nil {
		return "", fmt.Errorf("local responder not configured: no LLM adapter")
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return "", fmt.Errorf("local responder: empty user line")
	}

	var b strings.Builder
	if d := strings.TrimSpace(presentationDirective); d != "" {
		b.WriteString("## Directive\n")
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	if last := strings.TrimSpace(lastAnswer); last != "" {
		b.WriteString("## Previous answer\n")
		b.WriteString(last)
		b.WriteString("\n\n")
	}
	if prior := strings.TrimSpace(priorContext); prior != "" {
		b.WriteString("## Prior conversation\n")
		b.WriteString(prior)
		b.WriteString("\n\n")
	}
	b.WriteString("## Current message\n")
	b.WriteString(userLine)

	messages := []llm.Message{
		{Role: "system", Content: localResponderSystemPrompt},
		{Role: "user", Content: b.String()},
	}
	resp, err := r.adapter.Chat(ctx, messages, nil, llm.ChatOptions{})
	if err != nil {
		return "", fmt.Errorf("local llm call: %w", err)
	}
	reply := strings.TrimSpace(resp.Content)
	if reply == "" {
		return "", fmt.Errorf("local llm returned empty content")
	}
	return reply, nil
}

// turnPolicyDebugLine renders a one-line summary of the resolved
// policy for the debug log. Format pinned by tests so a future
// edit that drops a field surfaces immediately. Each value is
// trimmed / clamped so a long reason cannot wrap mid-line in the
// log file.
func turnPolicyDebugLine(p TurnPolicy) string {
	return fmt.Sprintf(
		"route=%s operation=%s operation_kind=%s needs_repo=%t needs_operation=%t risk=%s side_effects=%s target=%s confirm=%t confidence=%.2f source=%s reason=%q presentation=%q",
		string(p.Route),
		clipForLog(p.Operation, 32),
		clipForLog(p.OperationKind, 32),
		p.NeedsRepoAccess,
		p.NeedsOperationAccess,
		clipForLog(p.RiskLevel, 16),
		clipForLog(strings.Join(p.SideEffects, ","), 80),
		clipForLog(p.TargetSurface, 32),
		p.RequiresConfirmation,
		p.Confidence,
		clipForLog(p.Source, 32),
		oneLineClamp(p.Reason, 120),
		clipForLog(p.PresentationDirective, 80),
	)
}

// clipForLog clips a tag-shaped string to n runes; intended for
// the small enum-like fields in turnPolicyDebugLine where a multi-
// rune blowup in any field would garble the log line. oneLineClamp
// already covers the longer reason field.
func clipForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); n > 0 && len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// hybridRequestPrefix is a thin wrapper retained so external test
// fixtures continue to compile after presentation directives moved to
// typed runner metadata. It intentionally does NOT embed directive in
// the request body.
func hybridRequestPrefix(directive, request string) string {
	_ = directive
	return composeEffectiveRequest("", request)
}

// localRouteSummary returns the (label, segments) pair the renderer
// folds into the dock shutdown line for a local-route reply. The
// pipeline default ("◆ 已结束 · 4 阶段 · 总耗时 7s · …") would render
// misleading zeros for local routes (no stages / tools / iterations),
// so the dock swaps in "◇ <label> · <segments> · 总耗时 Xs" — same
// color family, hollow diamond signalling the lighter path, segments
// describing what actually happened.
//
// Wording is policy.Source-aware so the badge is truthful:
//
//	Source == "last_answer" → "复用上一轮答案 · 未读仓库" (derived from prior)
//	otherwise               → "未读仓库 · 纯模型生成" (no false reuse claim)
//
// Single LLM-emitted field, structural branch, no keyword table.
func localRouteSummary(lang string, policy TurnPolicy) (string, []string) {
	zh := isZh(lang)
	if zh {
		if policy.Source == "last_answer" {
			return "本地回复", []string{"复用上一轮答案", "未读仓库"}
		}
		return "本地回复", []string{"未读仓库", "纯模型生成"}
	}
	if policy.Source == "last_answer" {
		return "local reply", []string{"reused previous answer", "no repo read"}
	}
	return "local reply", []string{"no repo read", "pure model output"}
}

// clarifyRouteSummary returns the (label, segments) pair for the
// clarify route — emitted when the LLM proposed a transform/summarize
// but no prior answer exists to operate on. Same shape as
// localRouteSummary; rendered via Renderer.EmitLightRouteSummary
// (clarify never starts a spinner so the value is printed directly,
// not folded into a dock shutdown).
func clarifyRouteSummary(lang string) (string, []string) {
	if isZh(lang) {
		return "需要澄清", []string{"没有上一轮答案可复用,请直接描述你想问什么"}
	}
	return "clarify", []string{"no prior answer to reuse — re-state the question directly"}
}

// debugLogTurnPolicy is a small wrapper so callers don't have to
// import logging at every call site. Pinned name for tests that
// might assert log content via a future log-capture hook.
func debugLogTurnPolicy(p TurnPolicy) {
	logging.Debug("[repl/turn_policy] %s", turnPolicyDebugLine(p))
}
