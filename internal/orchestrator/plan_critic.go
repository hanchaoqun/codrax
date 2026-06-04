package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// PlanCritic is the pre-apply review split. After the planner emits
// a ChangePlan and the 11-stage validator accepts it, an independent
// LLM reads the plan and the user's framing (WriteAnalysisIR) to
// surface obvious problems BEFORE apply touches a worktree.
//
// Pattern mirrors Reflector exactly:
//   - one direct adapter.Chat call (no agent ReAct loop)
//   - structured emit_plan_review tool with a small JSON schema
//   - tool_choice=required so the model cannot return free text
//   - failure paths fall back to silent disable (never block apply)
//
// Routed via providers.yaml :: agents.plan_critic when present, or
// the default LLM otherwise — same convention as reflector. The
// critic's output is INFORMATIONAL only: it never auto-rejects a
// plan. The text is rendered for the operator's eyes via /plan show
// (commit 5 will wire that surface). The system never reads the
// critique to make a programmatic decision — that would be the
// system second-guessing the planner, exactly the trap Reflexion
// avoids.
type PlanCritic interface {
	// Review produces a structured verdict the caller renders into
	// operator-facing prose AND writes into the EvidenceClosure
	// ledger as one ViolPlanCritic per Risk. Returns a zero verdict +
	// nil error when the critic is disabled (nil adapter); returns
	// a zero verdict + non-nil error on any LLM error so the caller
	// logs and continues without blocking the apply path.
	//
	// Block 1 (architecture overhaul 2026-05-02): pre-2026-05-02 the
	// signature was `(string, error)` and the verdict was rendered
	// inside the critic. The structured shape is needed so each Risk
	// becomes a closure Violation with its own Stage / IRField /
	// Confidence — and the prose still renders deterministically via
	// AssemblePlanCritiqueProse.
	Review(ctx context.Context, input PlanCriticInput) (PlanCriticVerdict, error)
}

// PlanCriticVerdict is the structured emit_plan_review output, one
// per Review call. Consumed by stage_hooks.planPostHook to (a) render
// the operator-facing critique prose via AssemblePlanCritiqueProse
// and (b) append one ViolPlanCritic per Risk to the EvidenceClosure
// ledger so Block 3's fallback policy and Block 1's stage-wise
// summary see the reviewer's findings the same way they see every
// other gate's findings.
type PlanCriticVerdict struct {
	// Risks is the LLM-emitted list of one-sentence concerns. Cap
	// at 5 enforced by the schema. Empty list = "plan looks clean".
	Risks []string

	// Confidence is the reviewer's self-rated confidence label
	// (high/medium/low). Mapped to a numeric Confidence float by
	// PlanCriticConfidenceFloat for SuspectedRoot.Confidence.
	Confidence string
}

// IsEmpty reports whether the verdict carries no actionable content
// (no risks regardless of confidence). Convenience for callers that
// want to skip the closure-write loop on empty verdicts.
func (v PlanCriticVerdict) IsEmpty() bool {
	return len(v.Risks) == 0
}

// PlanCriticInput bundles every signal the critic needs to make
// observations from the plan. Mirrors ReflectorInput's shape — same
// "raw structured data, no system editorialising" philosophy.
type PlanCriticInput struct {
	// RawRequest is the user's original ask, verbatim. Lets the
	// critic compare what the plan does against what the user
	// asked for.
	RawRequest string

	// TaskKind / Scope / Summary come from WriteAnalysisIR when
	// present. Empty when commit 1's write_analyzer didn't produce
	// an IR (degraded path).
	TaskKind string
	Scope    string
	Summary  string

	// ExpectedOutcomes carries the LLM-emitted goal-checks from
	// WriteAnalysisIR.Request.ExpectedOutcomes. The critic compares
	// the plan against these to spot "the plan doesn't address
	// outcome X" patterns.
	ExpectedOutcomes []string

	// PlanSummary is the planner's own summary text from
	// ChangePlan.Summary. The critic spots inconsistencies between
	// what the planner says it did and what the changes[] entries
	// actually contain.
	PlanSummary string

	// Changes is the list of files the plan modifies. The critic receives a
	// bounded content preview when the planner emitted a patch or full-file
	// replacement, so it can spot shape-vs-diff mismatches without loading
	// unbounded generated content.
	Changes []PlanCriticChangeRef
}

// PlanCriticChangeRef is one line in the critic's input — a
// shape-level descriptor of one ChangePlan entry.
type PlanCriticChangeRef struct {
	Path           string
	Kind           string
	Rationale      string
	ContentPreview string
}

// planCriticTool is the structured-output schema. The critic emits
// a list of "risks" (each a one-sentence concern) and a confidence
// label. The risks list is informational; the operator (or a future
// /plan show enhancement) reads them. The system does NOT
// auto-reject based on risk count.
var planCriticTool = llm.ToolSchema{
	Name:        "emit_plan_review",
	Description: "Produce a short list of risks you see in the proposed plan, plus a confidence label. The risks are operator-facing observations; do not prescribe fixes — the planner already produced this plan, your job is to surface concerns the operator should consider before approving.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "risks": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Each entry is one short sentence describing a specific concern with the plan: a missing file, an inconsistency between summary and changes, a suspect dependency, a likely-broken caller, a coverage gap. Empty array when the plan looks clean — that is a valid emit. Cap entries at 5; pick the 5 most load-bearing concerns when there are more."
    },
    "confidence": {
      "type": "string",
      "enum": ["high", "medium", "low"],
      "description": "Your confidence in the risks listed. high = each risk is concretely grounded in the plan; medium = at least one risk is a judgement call; low = the plan is unclear and you cannot confidently assess it."
    }
  },
  "required": ["risks", "confidence"]
}`),
}

// planCriticSystemPrompt frames the critic as an independent
// reviewer of code-change proposals. Same neutral tone as the
// reflector's prompt — observe, don't prescribe.
const planCriticSystemPrompt = `You are an independent reviewer of a proposed code-change plan.

The planner produced a ChangePlan that has already passed deterministic validation (dependency closure, syntax check, summary fidelity). Your job is to surface the kind of risks deterministic checks miss: shape-level concerns, alignment with the user's actual ask, suspected gaps in coverage. You are NOT the planner — do not propose alternative plans or rewrites. Your output is a short list of risks the operator should consider before approving.

How to write a good risk list:
- Each risk is one short sentence (≤120 chars). Cite specific paths or rationale text from the input. Vague risks ("the plan might be wrong") are useless — say WHY a specific element is suspect.
- An empty list is the right output when the plan looks clean. Do not invent risks for completeness; honesty matters more than length.
- Only flag what the data shows. If you are unsure whether a concern is real, lower the confidence label rather than over-asserting.
- No prescriptions. Do not write "the planner should…" or "consider doing X". Describe what is suspect; the operator decides.
- Use the same language as the user's request.`

// llmPlanCritic is the default PlanCritic implementation. One Chat
// call per Review, opt-in via providers.yaml :: agents.plan_critic
// or inheriting the default adapter. Nil adapter yields a critic
// whose Review always returns ("", nil) — effectively disabled.
//
// Per-Run dedup cache (commit 15 #5): on verify→plan retry the
// planner can re-emit a structurally similar plan; running the
// critic on each retry pays an LLM call per iteration. The cache
// is keyed by an FNV hash of the rendered user-message bytes —
// when the same content reappears within the same Run, the
// previous critique is returned without a Chat call. Cache lives
// on the llmPlanCritic instance (one per orchestrator), cleared
// implicitly when the orchestrator constructs a fresh critic.
// Hash collisions are tolerable: at worst the operator sees a
// stale-but-still-relevant critique on a different plan, which
// is still better than the no-critique fallback.
type llmPlanCritic struct {
	adapter llm.Adapter
	mu      sync.Mutex
	cache   map[uint32]PlanCriticVerdict // input-hash → structured verdict
}

// NewPlanCritic builds the default PlanCritic. Nil adapter yields a
// disabled critic — Review always returns ("", nil). yaml-gated by
// pipeline_plan_critic_enabled in cmd/root.go.
func NewPlanCritic(adapter llm.Adapter) PlanCritic {
	return &llmPlanCritic{adapter: adapter}
}

// Review dispatches one structured-emit Chat call. Failure paths
// (nil adapter, no tool call, malformed JSON) return (zero verdict,
// err) so the caller logs WARN and continues. Apply is never blocked.
func (c *llmPlanCritic) Review(ctx context.Context, in PlanCriticInput) (PlanCriticVerdict, error) {
	if c == nil || c.adapter == nil {
		return PlanCriticVerdict{}, nil
	}
	user := renderPlanCriticUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return PlanCriticVerdict{}, fmt.Errorf("plan_critic: empty input")
	}
	// Per-Run dedup (commit 15 #5): skip the LLM call when we've
	// already reviewed identical input within the lifetime of
	// this critic instance. Spares a Chat call per verify→plan
	// retry iteration when the planner re-emits a similar plan.
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(user))
	key := hasher.Sum32()
	c.mu.Lock()
	if c.cache != nil {
		if cached, ok := c.cache[key]; ok {
			c.mu.Unlock()
			logging.Info("[plan_critic] cache hit (input hash=%x); reusing prior verdict", key)
			return cached, nil
		}
	}
	c.mu.Unlock()
	messages := []llm.Message{
		{Role: "system", Content: planCriticSystemPrompt},
		{Role: "user", Content: user},
	}
	tools := []llm.ToolSchema{planCriticTool}
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := c.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return PlanCriticVerdict{}, fmt.Errorf("plan_critic llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return PlanCriticVerdict{}, fmt.Errorf("plan_critic: LLM returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != planCriticTool.Name {
		return PlanCriticVerdict{}, fmt.Errorf("plan_critic: unexpected tool %q", call.Name)
	}
	var parsed struct {
		Risks      []string `json:"risks"`
		Confidence string   `json:"confidence"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return PlanCriticVerdict{}, fmt.Errorf("plan_critic: unmarshal tool params: %w", err)
	}
	verdict := PlanCriticVerdict{
		Risks:      append([]string(nil), parsed.Risks...),
		Confidence: parsed.Confidence,
	}
	logging.Info("[plan_critic] risks=%d confidence=%q", len(parsed.Risks), parsed.Confidence)
	c.mu.Lock()
	if c.cache == nil {
		c.cache = make(map[uint32]PlanCriticVerdict)
	}
	c.cache[key] = verdict
	c.mu.Unlock()
	return verdict, nil
}

// AssemblePlanCritiqueProse renders a PlanCriticVerdict into the
// operator-facing Markdown blob historically returned by Review. The
// rendering is identical to pre-2026-05-02 assemblePlanCritique
// output so /plan show / Mutable.PlanCritique consumers stay
// byte-identical.
func AssemblePlanCritiqueProse(v PlanCriticVerdict) string {
	return assemblePlanCritique(v.Risks, v.Confidence)
}

// PlanCriticConfidenceFloat maps the LLM's confidence label to a
// numeric value in [0, 1] for SuspectedRoot.Confidence. high=0.9,
// medium=0.6, low=0.3, anything else=0.5 (sane neutral default).
// Used by stage_hooks.planPostHook when writing each Risk into the
// EvidenceClosure ledger as a ViolPlanCritic.
func PlanCriticConfidenceFloat(label string) float64 {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "high":
		return 0.9
	case "medium":
		return 0.6
	case "low":
		return 0.3
	}
	return 0.5
}

// renderPlanCriticUserMessage assembles the critic's input as a
// Markdown blob — verbatim, mirrors renderReflectorUserMessage.
func renderPlanCriticUserMessage(in PlanCriticInput) string {
	var b strings.Builder
	if strings.TrimSpace(in.RawRequest) != "" {
		b.WriteString("## User request\n\n")
		b.WriteString(strings.TrimSpace(in.RawRequest))
		b.WriteString("\n\n")
	}
	if in.TaskKind != "" || in.Scope != "" || in.Summary != "" {
		b.WriteString("## Task framing\n\n")
		if in.TaskKind != "" {
			fmt.Fprintf(&b, "- kind: %s\n", in.TaskKind)
		}
		if in.Scope != "" {
			fmt.Fprintf(&b, "- scope: %s\n", in.Scope)
		}
		if in.Summary != "" {
			fmt.Fprintf(&b, "- summary: %s\n", in.Summary)
		}
		b.WriteString("\n")
	}
	if len(in.ExpectedOutcomes) > 0 {
		b.WriteString("## Expected outcomes (from task framing)\n\n")
		for _, o := range in.ExpectedOutcomes {
			fmt.Fprintf(&b, "- %s\n", o)
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(in.PlanSummary) != "" {
		b.WriteString("## Plan summary (from the planner)\n\n")
		b.WriteString(strings.TrimSpace(in.PlanSummary))
		b.WriteString("\n\n")
	}
	if len(in.Changes) > 0 {
		b.WriteString("## Plan changes\n\n")
		for _, c := range in.Changes {
			fmt.Fprintf(&b, "- `%s` (%s) — %s\n", c.Path, c.Kind, c.Rationale)
			if preview := strings.TrimSpace(c.ContentPreview); preview != "" {
				b.WriteString("  content preview:\n")
				for _, line := range strings.Split(preview, "\n") {
					fmt.Fprintf(&b, "  > %s\n", line)
				}
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// assemblePlanCritique formats the LLM's structured output for
// downstream rendering / logging.
//
// Low-confidence suppression (commit 10 #7): when the LLM flagged
// itself as confidence=low, the risks are signal-noise; surfacing
// them in /plan show would crowd out high-confidence critiques on
// other plans (the "cried wolf" effect). Drop them. The decision
// is the LLM's — it self-reports "I'm not sure" — and we honor
// that by hiding rather than over-asserting. The Info log line
// in Review still records the call so operators can grep for
// suppressed cases.
func assemblePlanCritique(risks []string, confidence string) string {
	if len(risks) == 0 {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(confidence), "low") {
		return ""
	}
	var b strings.Builder
	for _, r := range risks {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", r)
	}
	if confidence != "" {
		fmt.Fprintf(&b, "(confidence: %s)\n", strings.TrimSpace(confidence))
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildPlanCriticInput wires the critic's input from BusContext.
// Pulls the user request, the WriteAnalysisIR (when commit 1's
// write_analyzer ran), and the active ChangePlan.
func buildPlanCriticInput(busCtx *types.BusContext) PlanCriticInput {
	in := PlanCriticInput{}
	if busCtx == nil {
		return in
	}
	if mu := busCtx.Mutable; mu != nil {
		in.RawRequest = strings.TrimSpace(mu.Objective())
		if ir := mu.WriteAnalysisIR(); ir != nil {
			in.TaskKind = string(ir.Request.Task.Kind)
			in.Scope = string(ir.Request.Task.Scope)
			in.Summary = ir.Request.Task.Summary
			in.ExpectedOutcomes = append(in.ExpectedOutcomes, ir.Request.ExpectedOutcomes...)
		}
		if plan := mu.ChangePlan(); plan != nil {
			in.PlanSummary = plan.Summary
			for _, c := range plan.Changes {
				in.Changes = append(in.Changes, PlanCriticChangeRef{
					Path:           c.Path,
					Kind:           c.Kind,
					Rationale:      c.Rationale,
					ContentPreview: boundedPlanChangePreview(c),
				})
			}
		}
	}
	return in
}

func boundedPlanChangePreview(c types.FileChange) string {
	preview := strings.TrimSpace(c.Patch)
	if preview == "" {
		preview = strings.TrimSpace(c.NewContent)
	}
	return clampPlanCriticPreview(preview, 1200)
}

func clampPlanCriticPreview(s string, limit int) string {
	s = strings.TrimSpace(s)
	if s == "" || limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "\n...[plan critic preview truncated]"
}
