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
// {local, repo, hybrid, clarify} with a small set of orthogonal
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
	"context"
	"encoding/json"
	"fmt"
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
	// came from the previous answer or the user's framing. The
	// dispatcher carries presentation_directive into the pipeline
	// request body; the finalizer's user-preference channel
	// renders it.
	RouteHybrid TurnRoute = "hybrid"

	// RouteClarify — the user's message references state that
	// does not exist (e.g. "上面那条" with no prior answer). The
	// dispatcher prints a clarify message; no LLM call, no
	// pipeline.
	RouteClarify TurnRoute = "clarify"
)

// TurnPolicy is the structured classification result. All fields
// optional for stub implementations; the dispatcher applies
// ApplyTurnPolicyGuards before acting on any TurnPolicy so missing
// or self-contradictory fields cannot drive a wrong route.
type TurnPolicy struct {
	Route                 TurnRoute
	NeedsRepoAccess       bool
	Operation             string  // chat | transform | summarize | translate | elaborate | investigate
	Source                string  // current_message | last_answer | prior_context | repo | mixed
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
      "enum": ["local", "repo", "hybrid", "clarify"],
      "description": "local = answer from current message + previous answer + conversation context; no repo read. repo = read repository, run the analysis pipeline. hybrid = run the pipeline AND apply a transformation/presentation directive from the previous answer or user framing. clarify = user references missing state (e.g. 'the previous answer') that does not exist in this session. When uncertain, prefer repo — over-analyzing wastes cycles, under-analyzing gives wrong answers."
    },
    "needs_repo_access": {
      "type": "boolean",
      "description": "true iff route is repo or hybrid. The dispatcher cross-checks this with route; mismatch demotes to a safe default."
    },
    "operation": {
      "type": "string",
      "enum": ["chat", "transform", "summarize", "translate", "elaborate", "investigate"],
      "description": "chat = greeting / pleasantry / capability question. transform = change the form of the previous answer (mermaid, table, ...). summarize = shorten the previous answer. translate = render in another language. elaborate = expand on previous answer without new evidence. investigate = fresh code investigation."
    },
    "source": {
      "type": "string",
      "enum": ["current_message", "last_answer", "prior_context", "repo", "mixed"],
      "description": "Where the answer's content comes from. last_answer = derives from the immediately previous response. prior_context = derives from earlier conversation. repo = requires reading repository files. mixed = combination of the above."
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
      "description": "Optional. When route is local or hybrid, free-form directive describing the desired final-answer form ('mermaid', 'markdown table', 'brief 3-bullet summary'). Echoed verbatim into the local responder's system prompt OR prepended to the pipeline request body when hybrid. Omit when not applicable."
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

The four routes:

  local   — the answer can be produced from the user's CURRENT MESSAGE
            plus the PREVIOUS ANSWER and CONVERSATION CONTEXT. The
            dispatcher will NOT read repository files. Pick this for
            transformations / summaries / translations / elaborations
            that operate on the previous answer, and for greetings /
            capability questions that don't reference the codebase.

  repo    — the answer requires reading repository files. The
            dispatcher runs the full analysis pipeline. Pick this for
            any fresh code investigation, or when the user explicitly
            asks to re-read / re-confirm the repository.

  hybrid  — the answer requires BOTH: re-read the repository AND apply
            a presentation/transformation that came from the previous
            answer or the user's framing. Example: "把上面的流程图换
            成 mermaid，并重新读仓库确认有没有 IO 分析" — the user
            wants fresh repo evidence (IO 分析) AND a mermaid
            rendering. Set presentation_directive on this route.

  clarify — the user's message references state that does not exist
            in this session: e.g. "换成 mermaid 图例" / "把上面的结论
            换成表格" / "再扩展一下" when there is NO previous answer
            yet (first turn / cleared memory). The dispatcher prints a
            clarify message and does NOT call the LLM again.

needs_repo_access is true iff route ∈ {repo, hybrid}; the dispatcher
re-checks this and corrects mismatches.

operation:
  chat        — greetings, pleasantries, capability/identity questions
                ("你好", "你能做什么", "你是谁")
  transform   — change the form of the previous answer (render as
                mermaid, render as table, reorganise as bullet list)
  summarize   — shorten the previous answer
  translate   — render the previous answer in another language
  elaborate   — expand on the previous answer without new evidence
  investigate — fresh code investigation that needs repo reads

source:
  current_message — answer derives from the user's current input alone
                    (greetings, capability questions)
  last_answer     — answer derives from the previous response
                    (transform, summarize, translate, elaborate)
  prior_context   — answer derives from earlier conversation (not
                    just the immediate previous turn)
  repo            — answer requires repository access
  mixed           — combination (typical for hybrid)

confidence: 0..1 self-rating. Below 0.4 the dispatcher demotes to
repo because the cost of being wrong is higher for local / hybrid
(may produce a bogus answer based on missing context) than for repo
(merely wastes cycles re-reading).

presentation_directive: free-form text echoed verbatim into the
local responder's system prompt (when route=local) OR prepended to
the pipeline request (when route=hybrid). Examples:
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

  Current: "把上面的流程换成 mermaid，同时重新读仓库确认有没
            有 IO 分析" + last_answer_present=true
    → route=hybrid, operation=investigate, source=mixed,
      presentation_directive="mermaid 流程图", needs_repo_access=true,
      confidence≈0.85

  Current: "你好" (any state)
    → route=local, operation=chat, source=current_message,
      confidence≈0.95

  Current: "换成 mermaid 图例" + last_answer_present=false
    → route=clarify, operation=transform, source=last_answer,
      reason="user references previous answer but none exists",
      confidence≈0.9

  Current: "HiTraceAnalyzer 处理鸿蒙 trace 的流程是怎样的"
    → route=repo, operation=investigate, source=repo,
      confidence≈0.9

When uncertain (truly ambiguous), pick repo with confidence ≤0.5 —
the safe default.`

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
		Route                 string  `json:"route"`
		NeedsRepoAccess       bool    `json:"needs_repo_access"`
		Operation             string  `json:"operation"`
		Source                string  `json:"source"`
		Confidence            float64 `json:"confidence"`
		Reason                string  `json:"reason"`
		PresentationDirective string  `json:"presentation_directive"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return zero, fmt.Errorf("turn-policy classifier: unmarshal tool params: %w", err)
	}
	route := TurnRoute(parsed.Route)
	switch route {
	case RouteLocal, RouteRepo, RouteHybrid, RouteClarify:
	default:
		return zero, fmt.Errorf("turn-policy classifier: unknown route %q", parsed.Route)
	}
	return TurnPolicy{
		Route:                 route,
		NeedsRepoAccess:       parsed.NeedsRepoAccess,
		Operation:             parsed.Operation,
		Source:                parsed.Source,
		Confidence:            parsed.Confidence,
		Reason:                strings.TrimSpace(parsed.Reason),
		PresentationDirective: strings.TrimSpace(parsed.PresentationDirective),
	}, nil
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
	// or clarify→repo. Hybrid is consistent and unchanged.
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

	// Self-contradiction the other way: route says repo/hybrid
	// but needs_repo_access is false. Trust the route enum (it's
	// the explicit decision); patch needs_repo_access.
	if p.Route == RouteRepo || p.Route == RouteHybrid {
		p.NeedsRepoAccess = true
	}
	if p.Route == RouteLocal || p.Route == RouteClarify {
		p.NeedsRepoAccess = false
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

	// Confidence floor. Below the floor, demote any non-repo
	// route to repo (safe default). Hybrid stays hybrid because
	// it already runs the pipeline; clarify stays clarify because
	// running the pipeline against a "上面的" reference would
	// produce a worse answer than asking the user to re-state.
	if p.Confidence > 0 && p.Confidence < turnPolicyConfidenceFloor {
		if p.Route == RouteLocal {
			p.Route = RouteRepo
			p.NeedsRepoAccess = true
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
// (a) the optional presentation directive (route=hybrid), (b) the
// optional prior-conversation block from BuildContext, and (c) the
// user's original request line. Sections render in fixed order so
// the finalizer's user-preference channel finds the directive at
// the top, the analyzer reads conversation continuity in the
// middle, and the user's actual request is the last section the
// pipeline sees — matches the legacy "## Prior conversation /
// ## Current request" shape so existing pipeline parsing keeps
// working byte-identically when no directive is present.
//
// This helper is the SINGLE site that synthesises the effective
// request; callers no longer mutate `line` directly. The sole
// caller is dispatch(); test code reaches it via that path.
func composeEffectiveRequest(directive, prior, line string) string {
	d := strings.TrimSpace(directive)
	p := strings.TrimSpace(prior)
	if d == "" && p == "" {
		return line
	}
	var b strings.Builder
	if d != "" {
		b.WriteString("## Presentation directive (apply to final answer)\n")
		b.WriteString(d)
		b.WriteString("\n\n")
	}
	if p != "" {
		b.WriteString("## Prior conversation\n")
		b.WriteString(p)
		b.WriteString("\n\n")
	}
	b.WriteString("## Current request\n")
	b.WriteString(line)
	return b.String()
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
		"route=%s operation=%s needs_repo=%t confidence=%.2f source=%s reason=%q presentation=%q",
		string(p.Route),
		clipForLog(p.Operation, 32),
		p.NeedsRepoAccess,
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
// fixtures continue to compile after composeEffectiveRequest became
// the single producer of the effective-request string. New callers
// should use composeEffectiveRequest directly so the prior-
// conversation block can also be folded in atomically — the wire
// format is the SAME (directive header → optional prior → request
// section) so a test that locks the wrapper format also locks the
// real production output.
func hybridRequestPrefix(directive, request string) string {
	return composeEffectiveRequest(directive, "", request)
}

// localReplyHeader marks a local-route reply so the user can tell
// at a glance the answer came from the side LLM (no repo analysis,
// no plan, no pipeline) — distinct from the chitchat header
// because local replies are NOT casual chat: they're transformations
// of a real prior answer.
func localReplyHeader(lang string) string {
	if isZh(lang) {
		return "  [local] 复用上一轮答案的回复(未读仓库)。如需仓库新证据,请去掉 “换成/把上面的” 类措辞重新提问。"
	}
	return "  [local] reply built from the previous answer (no repo read). For fresh repo evidence, re-ask without the 'transform of the previous answer' framing."
}

// turnPolicyClarifyMessage is what the dispatcher prints when route
// resolves to clarify. Bilingual; specific to the missing-prior-
// answer case (the only clarify trigger today). If a future guard
// adds a different clarify reason we'll branch on it.
func turnPolicyClarifyMessage(lang string) string {
	if isZh(lang) {
		return "  [clarify] 你的请求像是要复用上一条回答(例如 “换成 mermaid”、“把上面的换成表格”),但当前会话还没有可复用的回答。请直接描述你想问什么 —— codrax 会去仓库里读相关代码再回答。"
	}
	return "  [clarify] Your request looks like a follow-up to a previous answer ('换成 mermaid', 'turn the above into a table'), but this session has no prior answer to transform. Re-state the question directly — codrax will read the repository and answer it from scratch."
}

// debugLogTurnPolicy is a small wrapper so callers don't have to
// import logging at every call site. Pinned name for tests that
// might assert log content via a future log-capture hook.
func debugLogTurnPolicy(p TurnPolicy) {
	logging.Debug("[repl/turn_policy] %s", turnPolicyDebugLine(p))
}
