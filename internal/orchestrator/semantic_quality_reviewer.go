package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// G5 (post_v2_runtime_gap_remediation, 2026-05-04). SemanticQualityReviewer
// is the second-layer reviewer that catches "answer ships clean but
// thin" — coverage gaps, mechanism elided, diagram spine missing,
// richness candidates with available evidence not surfaced.
//
// Why this is a SEPARATE reviewer from SelfConsistencyReviewer:
//
//   - self-consistency's prompt is carefully shaped to suppress
//     cried-wolf on contradiction-shape patterns; widening its input
//     to facet/diagram/richness signals dilutes that focus.
//   - the typed inputs are different: self-consistency reads two
//     prose blobs + EvidenceAnchorSet, semantic-quality reads
//     coverage summaries + diagram contract + richness candidates.
//   - confidence floors are different (semantic-quality is HIGHER
//     because false positives directly force the LLM to add prose).
//
// Wired AFTER validateFacetCoverage in runContractCheck — only fires
// when no hard facet violations remain (avoid double-noise on
// already-failing answers). Output is SOFT-only via
// ViolAnswerSemanticUnderfilled; operator may promote to STRICT via
// pipeline_contract_strict_kinds when a pipeline guarantees richer
// answers.

// SemanticQualityInput bundles what the reviewer reads. Pure typed
// projections — no heuristic, no similarity, no frequency. The
// downstream LLM forms judgement; the system supplies typed
// attestations of what's covered vs what's available.
type SemanticQualityInput struct {
	OriginalRequest string
	AnswerSummary   string
	AnswerBody      string

	// RequiredFacets summarises the (FacetCoverage.Required) plan.
	// Each entry is a typed projection — Kind / Tier / Promoted /
	// Covered. Reviewer judges whether the answer surfaces the
	// promoted facets; the system does not pre-decide.
	RequiredFacets []SemanticFacetSummary

	// DiagramContract names the (RelationKind, MinExpected,
	// MinSatisfied) tuples from view.DiagramPlan.EdgeRelations.
	// nil when the family does not require a diagram.
	DiagramContract *SemanticDiagramSummary

	// RichnessCandidates lists optional facets whose SourceCandidate
	// is non-empty (typed evidence available) but no block declared
	// the facet_id. Reviewer can suggest surfacing one or more.
	RichnessCandidates []SemanticRichnessSummary

	// EvidenceAnchorSet shares the SST helper with self-consistency
	// (BuildEvidenceAnchorSet). When supplied the reviewer can
	// cross-check fabricated mechanism names — but its primary job
	// here is COVERAGE (not contradiction).
	EvidenceAnchorSet []string
}

// SemanticFacetSummary is the typed projection of one
// FacetCoverageContract.Required entry.
type SemanticFacetSummary struct {
	Kind     string // facet kind, e.g. "principal_mechanism"
	Tier     string // "essential" / "expected"
	Promoted bool   // FacetRequirement.IsPromoted()
	Covered  bool   // any V2 block declared this facet_id
}

// SemanticDiagramSummary is the typed projection of the diagram
// contract.
type SemanticDiagramSummary struct {
	Required        bool
	Edges           []SemanticDiagramEdgeContract
	BlockPresent    bool   // doc has a BlockDiagram entry
	BodyTrimmedLen  int    // diagram body length (post-trim) — 0 means body absent
	BodyExcerpt     string // ≤300 chars, head of body for the reviewer
}

// SemanticDiagramEdgeContract is one EdgeRelations row.
type SemanticDiagramEdgeContract struct {
	RelationKind string // "call" / "guard" / etc.
	MinExpected  int
	MinSatisfied int // typed count of edges resolved to this kind
}

// SemanticRichnessSummary is one optional facet whose evidence is
// available but unsurfaced.
type SemanticRichnessSummary struct {
	Kind          string // facet kind
	EvidenceCount int    // len(SourceCandidate)
}

// SemanticQualityResult is the reviewer verdict.
type SemanticQualityResult struct {
	// Sufficient reports whether the reviewer judges the answer
	// surfaces enough of the promoted facets / diagram contract /
	// richness candidates. true = clean; false = at least one
	// Concern listed.
	Sufficient bool

	// Concerns lists up to 5 specific gaps. Each entry names the
	// Topic (facet kind / "diagram_spine" / richness facet kind),
	// what the reviewer observed (Observation), and what it
	// suggests adding (Suggestion).
	Concerns []SemanticQualityConcern

	// Confidence is the reviewer's self-rated certainty 0-1.
	// Higher floor than self-consistency (default 0.85) — false
	// positive directly forces the LLM to add prose.
	Confidence float64

	Reasoning string
}

// SemanticQualityConcern is one specific gap.
type SemanticQualityConcern struct {
	Topic       string // ≤ 60 chars
	Observation string // ≤ 200 chars: what the reviewer saw in the answer
	Suggestion  string // ≤ 200 chars: what to add / expand
}

// SemanticQualityReviewer is the public interface; nil adapter
// yields a reviewer whose Review returns (nil, nil) — disabled.
type SemanticQualityReviewer interface {
	Review(ctx context.Context, in SemanticQualityInput) (*SemanticQualityResult, error)
}

// SemanticQualityMinConfidenceDefault is the conservative floor;
// raise to 0.9 in production once eval-validated.
const SemanticQualityMinConfidenceDefault = 0.85

var semanticQualityTool = llm.ToolSchema{
	Name: "emit_semantic_quality_review",
	Description: "Emit your verdict on whether the answer surfaces enough of the required facets, diagram relations, and available richness. Do NOT flag stylistic, abstraction-level, or terminology differences. Only flag MISSING coverage where typed evidence is available. Do NOT cross into self-contradiction territory — that is another reviewer's job.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "sufficient": {
      "type": "boolean",
      "description": "true when the answer surfaces enough of the supplied promoted facets, diagram-edge minimums, and richness candidates; false ONLY when at least one specific gap is named in concerns[]."
    },
    "concerns": {
      "type": "array",
      "maxItems": 5,
      "description": "List up to 5 specific gaps. Empty when sufficient=true.",
      "items": {
        "type": "object",
        "properties": {
          "topic": {
            "type": "string",
            "maxLength": 60,
            "description": "Concise framing — name the facet kind / diagram contract row / richness facet at issue."
          },
          "observation": {
            "type": "string",
            "maxLength": 200,
            "description": "What the BODY (or the absence in BODY) shows. Verbatim quote or short paraphrase of what was missing."
          },
          "suggestion": {
            "type": "string",
            "maxLength": 200,
            "description": "What to add / surface. Reference the typed evidence available (do not invent new claims)."
          }
        },
        "required": ["topic", "observation", "suggestion"]
      }
    },
    "confidence": {
      "type": "number",
      "minimum": 0,
      "maximum": 1,
      "description": "Your self-rated certainty 0-1. Be conservative — when in doubt prefer sufficient=true with confidence ~0.5; only mark sufficient=false with confidence >= 0.85."
    },
    "reasoning": {
      "type": "string",
      "maxLength": 300
    }
  },
  "required": ["sufficient", "confidence"]
}`),
}

// semanticQualityReviewerSystemPrompt frames the reviewer's scope.
// R5 invariant — describes JUDGEMENT criteria, not what the answer
// should literally contain. R6 invariant — every example is an
// abstract pattern, not a verbatim case study.
const semanticQualityReviewerSystemPrompt = `You are an INDEPENDENT completeness reviewer. The pipeline produced an answer plus typed coverage attestations. Your ONE task: decide whether the answer surfaces ENOUGH of what the typed attestations show is available.

You are NOT checking:
  - Whether SUMMARY and BODY contradict each other (a separate reviewer handles that)
  - Whether the prose is well-written, grammatical, or stylistically optimal
  - Whether the answer should be at a different abstraction level

You ARE checking — and ONLY checking — three coverage dimensions, in this priority order:

  1. PROMOTED FACETS: Each entry in REQUIRED FACETS marked promoted=true MUST be surfaced in the answer. "Surfaced" means the rendered prose / list / diagram presents the facet's content (via the facet_ids attestation OR via clearly-related body text). Promoted=true means typed evidence is available to support the facet — failing to surface a promoted facet is a real coverage gap. covered=true means the answer already declared this facet via facet_ids; flag only when promoted AND covered=false.

  2. DIAGRAM RELATION CONTRACT: Each row in DIAGRAM CONTRACT names a relation kind and a minimum count. When a row's min_satisfied < min_expected AND a diagram block is present, the answer is short on that relation. Flag when the diagram exists but its edge relations fail to meet the typed minimum.

  3. AVAILABLE RICHNESS: RICHNESS CANDIDATES lists facets whose typed evidence is available but not surfaced. These are NOT hard requirements — flag at most ONE richness gap, and only when the gap is glaring (e.g. the answer is principal-only with NO supplemental context AND there are 2+ rich facets unsurfaced). Richness gaps should NOT dominate Concerns; promoted-facet gaps are the primary signal.

DECISION DISCIPLINE (apply before reporting):
  1. Count the promoted-facets that are uncovered. ZERO uncovered = sufficient=true (subject to diagram + richness).
  2. For each candidate Concern, ask: does the typed attestation actually show evidence for this gap? If the attestation lists no SourceCandidate / no min_expected / no body excerpt, the gap is not yours to flag.
  3. Stay within the supplied attestations. Do NOT fetch repo sources, do NOT speculate on missing evidence the prior investigation never produced.
  4. Stylistic preferences ("the answer would read better if it added a section on X") are NOT concerns. Only typed-evidence-available coverage gaps qualify.
  5. When a promoted facet is covered=true but the body's prose for that facet feels thin — that is NOT a concern (abstraction is editorial choice; coverage is the gate).

Output via emit_semantic_quality_review:
  - sufficient=true is the COMMON case; mark it true unless you can name a SPECIFIC promoted/diagram/richness gap supported by the attestations.
  - confidence >= 0.85 to report a gap; below 0.85 mark sufficient=true (rather miss a thin spot than force a rewrite on a defensible answer).
  - When reporting, name the typed signal (facet kind / relation kind / richness facet kind). Use the same language as the answer text in the prose fields.`

// llmSemanticQualityReviewer is the default impl. nil adapter ⇒
// disabled.
type llmSemanticQualityReviewer struct {
	adapter llm.Adapter
}

// NewSemanticQualityReviewer builds the default reviewer.
func NewSemanticQualityReviewer(adapter llm.Adapter) SemanticQualityReviewer {
	return &llmSemanticQualityReviewer{adapter: adapter}
}

// Review dispatches one structured-emit Chat call.
func (r *llmSemanticQualityReviewer) Review(ctx context.Context, in SemanticQualityInput) (*SemanticQualityResult, error) {
	if r == nil || r.adapter == nil {
		return nil, nil
	}
	user := renderSemanticQualityUserMessage(in)
	if strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("semantic_quality_reviewer: empty input")
	}
	messages := []llm.Message{
		{Role: "system", Content: semanticQualityReviewerSystemPrompt},
		{Role: "user", Content: user},
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := r.adapter.Chat(ctx, messages, []llm.ToolSchema{semanticQualityTool}, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return nil, fmt.Errorf("semantic_quality_reviewer llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("semantic_quality_reviewer: LLM returned no tool_call")
	}
	for _, call := range resp.ToolCalls {
		if call.Name != semanticQualityTool.Name {
			logging.Warning("[semantic_quality_reviewer] unexpected tool %q (skipping)", call.Name)
			continue
		}
		return unmarshalSemanticQualityResult(call.Params)
	}
	return nil, fmt.Errorf("semantic_quality_reviewer: no matching tool emit")
}

// unmarshalSemanticQualityResult decodes the LLM's emit args.
func unmarshalSemanticQualityResult(raw json.RawMessage) (*SemanticQualityResult, error) {
	var parsed struct {
		Sufficient bool `json:"sufficient"`
		Concerns   []struct {
			Topic       string `json:"topic"`
			Observation string `json:"observation"`
			Suggestion  string `json:"suggestion"`
		} `json:"concerns"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode emit_semantic_quality_review: %w", err)
	}
	if parsed.Confidence < 0 || parsed.Confidence > 1 {
		return nil, fmt.Errorf("semantic_quality_reviewer: confidence %.2f out of [0,1]", parsed.Confidence)
	}
	out := &SemanticQualityResult{
		Sufficient: parsed.Sufficient,
		Confidence: parsed.Confidence,
		Reasoning:  strings.TrimSpace(parsed.Reasoning),
	}
	for _, c := range parsed.Concerns {
		topic := strings.TrimSpace(c.Topic)
		obs := strings.TrimSpace(c.Observation)
		sug := strings.TrimSpace(c.Suggestion)
		if topic == "" || obs == "" || sug == "" {
			continue
		}
		out.Concerns = append(out.Concerns, SemanticQualityConcern{
			Topic: topic, Observation: obs, Suggestion: sug,
		})
	}
	if !out.Sufficient && len(out.Concerns) == 0 {
		return nil, fmt.Errorf("semantic_quality_reviewer: sufficient=false but no concerns named")
	}
	return out, nil
}

// renderSemanticQualityUserMessage formats the input as labelled
// markdown sections so the reviewer's prompt-instructed signals
// match the input shape.
func renderSemanticQualityUserMessage(in SemanticQualityInput) string {
	var b strings.Builder
	if s := strings.TrimSpace(in.OriginalRequest); s != "" {
		fmt.Fprintf(&b, "## Original user request (context only)\n%s\n\n", s)
	}
	fmt.Fprintf(&b, "## SUMMARY\n%s\n\n", strings.TrimSpace(in.AnswerSummary))
	fmt.Fprintf(&b, "## BODY\n%s\n", strings.TrimSpace(in.AnswerBody))

	if len(in.RequiredFacets) > 0 {
		b.WriteString("\n## REQUIRED FACETS (typed coverage attestation)\n")
		b.WriteString("Each row: facet kind / tier / promoted / covered. Flag entries that are promoted=true but covered=false.\n\n")
		for _, f := range in.RequiredFacets {
			fmt.Fprintf(&b, "- kind=`%s` tier=%s promoted=%t covered=%t\n", f.Kind, f.Tier, f.Promoted, f.Covered)
		}
	}
	if in.DiagramContract != nil && in.DiagramContract.Required {
		b.WriteString("\n## DIAGRAM CONTRACT (typed edge minimums)\n")
		fmt.Fprintf(&b, "Diagram block present in answer: %t. Body length (post-trim): %d.\n", in.DiagramContract.BlockPresent, in.DiagramContract.BodyTrimmedLen)
		if strings.TrimSpace(in.DiagramContract.BodyExcerpt) != "" {
			fmt.Fprintf(&b, "Body excerpt:\n```\n%s\n```\n", in.DiagramContract.BodyExcerpt)
		}
		if len(in.DiagramContract.Edges) > 0 {
			b.WriteString("Edge relation rows (min_expected vs min_satisfied):\n")
			for _, e := range in.DiagramContract.Edges {
				fmt.Fprintf(&b, "- relation_kind=`%s` min_expected=%d min_satisfied=%d\n", e.RelationKind, e.MinExpected, e.MinSatisfied)
			}
		}
	}
	if len(in.RichnessCandidates) > 0 {
		b.WriteString("\n## RICHNESS CANDIDATES (optional facets with typed evidence available)\n")
		b.WriteString("Each row: facet kind / typed evidence count. Flag at most ONE richness gap, and only when glaring.\n\n")
		for _, r := range in.RichnessCandidates {
			fmt.Fprintf(&b, "- kind=`%s` evidence_count=%d\n", r.Kind, r.EvidenceCount)
		}
	}
	if len(in.EvidenceAnchorSet) > 0 {
		b.WriteString("\n## EVIDENCE ANCHORS (typed identifier set, for cross-reference)\n")
		for _, id := range in.EvidenceAnchorSet {
			fmt.Fprintf(&b, "- %s\n", id)
		}
	}
	return b.String()
}

// BuildSemanticQualityInput projects the orchestrator's typed state
// into the reviewer input. Pure typed projection (R3 invariant);
// no heuristic, no similarity, no frequency. Caller passes the
// already-built EvidenceAnchorSet (shared SST with self-consistency).
func BuildSemanticQualityInput(
	originalRequest, summary, body string,
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	anchorSet []string,
) SemanticQualityInput {
	in := SemanticQualityInput{
		OriginalRequest:   originalRequest,
		AnswerSummary:     summary,
		AnswerBody:        body,
		EvidenceAnchorSet: anchorSet,
	}
	if doc == nil || view == nil {
		return in
	}

	// Coverage map for required + optional facets.
	covered := make(map[string]bool, 8)
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
		for _, item := range b.Items {
			if item.ClaimUse != nil && item.ClaimUse.FacetID != "" {
				covered[strings.TrimSpace(item.ClaimUse.FacetID)] = true
			}
		}
	}

	if view.FacetCoverage != nil {
		for _, req := range view.FacetCoverage.Required {
			if req.EffectivePromotionPolicy() == types.PromotionAdvisoryOnly {
				continue
			}
			in.RequiredFacets = append(in.RequiredFacets, SemanticFacetSummary{
				Kind:     string(req.Kind),
				Tier:     string(req.EffectiveTier()),
				Promoted: req.IsPromoted(),
				Covered:  covered[strings.TrimSpace(string(req.Kind))],
			})
		}
		// Richness candidates: optional facets with typed evidence
		// available but uncovered.
		for _, req := range view.FacetCoverage.Optional {
			if len(req.SourceCandidate) == 0 {
				continue
			}
			if covered[strings.TrimSpace(string(req.Kind))] {
				continue
			}
			in.RichnessCandidates = append(in.RichnessCandidates, SemanticRichnessSummary{
				Kind:          string(req.Kind),
				EvidenceCount: len(req.SourceCandidate),
			})
		}
	}

	if view.DiagramPlan != nil && view.DiagramPlan.Required {
		summary := &SemanticDiagramSummary{Required: true}
		// Find a diagram block in the doc.
		var diagBody string
		for _, b := range doc.Blocks {
			if b.Kind == types.BlockDiagram && b.Diagram != nil {
				summary.BlockPresent = true
				diagBody = b.Diagram.Body
				summary.BodyTrimmedLen = len(strings.TrimSpace(diagBody))
				break
			}
		}
		// Excerpt: head 300 chars.
		if summary.BodyTrimmedLen > 0 {
			body := strings.TrimSpace(diagBody)
			const cap = 300
			if len(body) > cap {
				body = body[:cap]
			}
			summary.BodyExcerpt = body
		}
		// Edge contract rows. Counting min_satisfied is approximate
		// here (the validator's typed-relation index is not
		// re-instantiated). We surface MinExpected verbatim and a
		// MinSatisfied derived from doc.Blocks[].EdgeAnchors that
		// declare a typed RelationKind matching the contract row.
		typedKindCounts := make(map[types.DiagramRelationKind]int)
		for _, b := range doc.Blocks {
			for _, a := range b.EdgeAnchors {
				if a.RelationKind.IsValid() {
					typedKindCounts[a.RelationKind]++
				}
			}
		}
		for _, contract := range view.DiagramPlan.EdgeRelations {
			if contract.Min <= 0 {
				continue
			}
			summary.Edges = append(summary.Edges, SemanticDiagramEdgeContract{
				RelationKind: string(contract.Kind),
				MinExpected:  contract.Min,
				MinSatisfied: typedKindCounts[contract.Kind],
			})
		}
		in.DiagramContract = summary
	}

	return in
}
