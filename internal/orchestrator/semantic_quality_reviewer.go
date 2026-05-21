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
// already-failing answers). Ordinary coverage/richness gaps emit the
// soft ViolAnswerSemanticUnderfilled signal; wrong-topic answers emit
// high-severity ViolAnswerTopicMismatch so a topical miss blocks.

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

	// SystemDetectedGaps lists typed gaps the v3 B2 validators
	// already identified (ViolRichnessGlaringGap +
	// ViolPrincipalProseUnderfilled). The reviewer treats these as
	// system-confirmed signals; its job is reverse-check —
	// verify whether BODY in fact addresses each gap via equivalent
	// expression. Reviewer must NOT restate system-detected gaps in
	// Concerns — concerns are ADDITIONS the typed validators missed.
	SystemDetectedGaps []SystemDetectedGap

	// PromotedFacetCoverage is the depth audit for emit-hard facets:
	// DeclaredCount counts how many times a block.FacetIDs / item
	// .ClaimUse.FacetID names this facet kind; AnchoredCount is the
	// subset of those declarations that ALSO carry an evidence_id /
	// claim_form on the same site. A facet with DeclaredCount > 0
	// but AnchoredCount == 0 is a "shallow declaration" — covered by
	// label, not anchored to evidence. Evidence-sufficient SOFT facets
	// stay out of this hard-depth audit so reviewer repair pressure does
	// not turn metadata enrichment into answer rewrites.
	PromotedFacetCoverage []FacetCoverageDepth

	// ClaimBindings is the shared origin/policy handoff compiled from
	// typed aggregate facts and runtime artifacts. The reviewer reads this
	// so it does not reinterpret a VCS, command-measurement, runtime, or
	// negative-search claim as if it needed current-source file:line
	// grounding.
	ClaimBindings []SemanticClaimBindingSummary

	// Observations is the compact Observation Ledger projection shared with
	// finalizer/reviewer consumers. It carries origin/source/span boundaries for
	// current-source evidence, VCS/diff and command outputs, runtime artifacts,
	// and external resources without asking the reviewer to parse raw tool dumps.
	Observations []SemanticObservationSummary
}

// SystemDetectedGap is one typed verdict already raised by the v3
// validators. Reviewer reads to avoid restating; its prompt asks
// reviewer to verify the gap rather than rediscover it.
//
// Kind is rendered to the LLM as a human-readable category label
// (NOT the internal ViolationKind string). Use the enum constants
// below; code that builds gaps from validator output translates
// the typed verdict into one of these strings before stashing.
type SystemDetectedGap struct {
	// Kind is the human-readable category label rendered into the
	// reviewer prompt. One of:
	//   "uncovered enrichment facet with available evidence"
	//   "principal block prose lacks inline grounding"
	Kind string
	// FacetKind is the missing facet (enrichment-gap case).
	FacetKind string
	// BlockID is the offending block id (prose-underfilled case).
	BlockID string
	// EvidenceCount is len(SourceCandidate) for an enrichment gap;
	// for a prose-underfilled gap it is the typed claim_use count
	// on the block.
	EvidenceCount int
}

// Reviewer-facing category labels for SystemDetectedGap.Kind. Kept
// separate from the internal ViolationKind constants so the LLM
// prompt cannot drift into codrax-specific taxonomy strings.
const (
	gapKindEnrichmentUncovered = "uncovered enrichment facet with available evidence"
	gapKindProseUnderfilled    = "principal block prose lacks inline grounding"
)

// FacetCoverageDepth audits one promoted facet's declared vs
// anchored coverage on the rendered V2 doc.
type FacetCoverageDepth struct {
	Kind          string
	DeclaredCount int
	AnchoredCount int
}

// SemanticClaimBindingSummary is the reviewer-facing projection of one
// AnswerClaimBinding. It intentionally carries only typed origin/policy/output
// fields plus compact support counts; the reviewer should judge whether the
// rendered answer respects this boundary, not infer new facts from support
// refs.
type SemanticClaimBindingSummary struct {
	ClaimID            string
	Origin             string
	Policy             string
	Outputs            []string
	Target             string
	SupportRefCount    int
	ExactSourceSupport bool
}

// SemanticObservationSummary is the reviewer-facing projection of one
// ObservationRecord. It keeps the same typed origin/source/span contract as the
// finalizer's Observation Ledger prompt while staying compact enough for the
// reviewer budget.
type SemanticObservationSummary struct {
	ID          string
	Origin      string
	Role        string
	Policy      string
	Source      string
	Span        string
	Claim       string
	Summary     string
	Negative    bool
	ResultCount *int
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
	Required       bool
	Edges          []SemanticDiagramEdgeContract
	BlockPresent   bool   // doc has a BlockDiagram entry
	BodyTrimmedLen int    // diagram body length (post-trim) — 0 means body absent
	BodyExcerpt    string // ≤300 chars, head of body for the reviewer
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
	// Topic (public facet label / diagram contract row / enrichment label),
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
	Kind        string // coverage_gap / topic_mismatch / grounding_gap / diagram_gap / richness_gap
	Topic       string // ≤ 60 chars
	Observation string // ≤ 200 chars: what the reviewer saw in the answer
	Suggestion  string // ≤ 200 chars: what to add / expand
	RepairLocus string // local_doc_defect / evidence_gap / analysis_gap / presentation_advisory / safety
}

const (
	semanticConcernCoverageGap   = "coverage_gap"
	semanticConcernTopicMismatch = "topic_mismatch"
	semanticConcernGroundingGap  = "grounding_gap"
	semanticConcernDiagramGap    = "diagram_gap"
	semanticConcernRichnessGap   = "richness_gap"
)

const (
	semanticRepairLocalDocDefect      = "local_doc_defect"
	semanticRepairEvidenceGap         = "evidence_gap"
	semanticRepairAnalysisGap         = "analysis_gap"
	semanticRepairPresentationAdvisor = "presentation_advisory"
	semanticRepairSafety              = "safety"
)

// SemanticQualityTopicMismatchMinConfidence is the lower floor for a
// typed topic-mismatch concern. Topic mismatch is not a richness taste:
// it means the answer's main subject diverged from the current user
// request. Keep the global conservative 0.92 floor for ordinary
// coverage/richness concerns, but accept high-confidence topical misses
// in the 0.85-0.91 band so a wrong-subject answer cannot ship.
const SemanticQualityTopicMismatchMinConfidence = 0.85

// SemanticQualityReviewer is the public interface; nil adapter
// yields a reviewer whose Review returns (nil, nil) — disabled.
type SemanticQualityReviewer interface {
	Review(ctx context.Context, in SemanticQualityInput) (*SemanticQualityResult, error)
}

// 2026-05-10 P4 tightening: raised 0.85 → 0.92. Same rationale as
// the parallel self_consistency tighten — the reviewer at confidence
// 0.85-0.90 was firing on subtle formatting / anchor-depth concerns
// that did not invalidate the answer. 0.92 keeps the
// reviewer's high-confidence facet-coverage gaps while filtering
// the noise band. yaml override
// `pipeline_semantic_quality_min_confidence` lets operators flip
// per-deployment.
//
// SemanticQualityMinConfidenceDefault is the conservative floor;
// raise to 0.9 in production once eval-validated.
const SemanticQualityMinConfidenceDefault = 0.92

var semanticQualityTool = llm.ToolSchema{
	Name:        "emit_semantic_quality_review",
	Description: "Emit your verdict on whether the answer stays on the user's requested topic and surfaces enough of the required facets, diagram relations, and available richness. Do NOT flag stylistic, abstraction-level, or terminology differences. Only flag a wrong-topic answer or MISSING coverage where typed evidence is available. Do NOT cross into self-contradiction territory — that is another reviewer's job.",
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
      "description": "List up to 5 specific gaps. Empty when sufficient=true; required to contain at least one concrete concern when sufficient=false.",
      "items": {
        "type": "object",
        "properties": {
          "topic": {
            "type": "string",
            "maxLength": 60,
            "description": "Concise framing — name the public facet label / diagram contract row / enrichment item at issue."
          },
          "kind": {
            "type": "string",
            "enum": ["coverage_gap", "topic_mismatch", "grounding_gap", "diagram_gap", "richness_gap"],
            "description": "Typed concern family. Use topic_mismatch only when the answer's main subject does not match the current user request."
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
          },
          "repair_locus": {
            "type": "string",
            "enum": ["local_doc_defect", "evidence_gap", "analysis_gap", "presentation_advisory", "safety"],
            "description": "Where the issue can honestly be repaired: local_doc_defect = revise the answer using already supplied evidence; evidence_gap = more investigation or source evidence is needed before the answer can be fixed; analysis_gap = the initial understanding/coverage plan appears wrong; presentation_advisory = clarity/format concern that should not force a rewrite; safety = unsafe or materially wrong answer that must be corrected before shipping."
          }
        },
        "required": ["topic", "kind", "observation", "suggestion", "repair_locus"]
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
  "required": ["sufficient", "concerns", "confidence"]
}`),
}

// semanticQualityReviewerSystemPrompt frames the reviewer's scope.
// R5 invariant — describes JUDGEMENT criteria, not what the answer
// should literally contain. R6 invariant — every example is an
// abstract pattern, not a verbatim case study.
//
// v3 B2 (2026-05-04): reviewer role split into two halves —
//
//	(a) reverse-check the system-detected gaps (already raised by
//	    the typed validators) and confirm whether BODY actually
//	    addresses each via equivalent expression;
//	(b) ADD any coverage gaps the typed validators missed.
//
// Reviewer MUST NOT restate system-detected gaps verbatim — that
// would double-bill the LLM on retry.
const semanticQualityReviewerSystemPrompt = `You are an INDEPENDENT completeness reviewer. The pipeline produced an answer plus typed coverage attestations and a list of system-detected gaps. Your job has TWO parts:

  PART A — REVERSE-CHECK system-detected gaps. The typed validators already identified these gaps; their accuracy is the question. For each entry in SYSTEM-DETECTED GAPS, decide whether the BODY in fact ADDRESSES the gap via equivalent expression (same content under different framing, or covered by adjacent prose). If the BODY truly addresses every system-detected gap → mark sufficient=true (Concerns may stay empty even when the system flagged gaps; the system is conservative and you are the second opinion).

  PART B — ADD coverage gaps the typed validators missed. The PROMOTED FACETS / DIAGRAM CONTRACT / RICHNESS CANDIDATES / EVIDENCE ANCHORS attestations describe what's available. When you see a coverage shortfall the system did NOT flag (e.g. a promoted facet with covered=true but prose only paraphrases without engaging the typed evidence), enumerate it in Concerns. Concerns must be ADDITIONS — never restate a SYSTEM-DETECTED GAP verbatim.

You are NOT checking:
  - Whether SUMMARY and BODY contradict each other (a separate reviewer handles that)
  - Whether the prose is well-written, grammatical, or stylistically optimal
  - Whether the answer should be at a different abstraction level

DEFINITION CLARITY:
  - "inline code anchor" = a Markdown backtick-quoted identifier inside the prose, e.g. ` + "`funcName`" + ` or ` + "`package.Type`" + `. Parenthesized file:line citations like "(foo.go:42)" do NOT count as inline code anchors — they are bibliographic references that sit OUTSIDE the prose stream, not in-prose anchors. When a typed gap mentions "zero inline ` + "`code`" + ` references", verify by reading the prose for actual backtick spans, not for parenthesized file:line.

DECISION DISCIPLINE (apply before reporting):
  1. PART A first: walk every SYSTEM-DETECTED GAP and verify whether BODY addresses it. If yes for all → sufficient=true.
  2. PART B second: scan the typed attestations for shortfalls the system did NOT flag.
  3. TOPIC ALIGNMENT: compare the BODY's main subject with the Original user request. If the BODY answers a different subject, report kind=topic_mismatch. This is not style or abstraction-level feedback; it is wrong-task feedback.
  4. Stay within the supplied attestations. Do NOT fetch repo sources, do NOT speculate on missing evidence the prior investigation never produced.
  5. Stylistic preferences ("the answer would read better if it added a section on X") are NOT concerns. Only wrong-topic answers or typed-evidence-available coverage gaps qualify.
  6. When a promoted facet is covered=true AND the body engages the cited evidence — that is NOT a concern (abstraction is editorial choice; coverage with grounding is the gate).
  7. PROMOTED FACET COVERAGE depth: when a facet has DeclaredCount > 0 but AnchoredCount == 0, the declaration is shallow (label-only). Flag only when the shortfall is structurally important (multiple shallow declarations, or the principal facet is shallow).
  8. REPAIR LOCUS: every concern must classify where repair belongs:
     - local_doc_defect: the answer can be revised with the already supplied evidence.
     - evidence_gap: the answer cannot be corrected without more investigation/source evidence.
     - analysis_gap: the initial understanding or coverage plan appears wrong.
     - presentation_advisory: formatting/clarity issue only; do not force a rewrite.
     - safety: materially wrong or unsafe answer that must be corrected before shipping.

Output via emit_semantic_quality_review:
  - sufficient=true is the COMMON case; mark it true unless you can name a SPECIFIC ADDITIONAL gap supported by the attestations.
  - confidence >= 0.85 to report a gap; below 0.85 mark sufficient=true (rather miss a thin spot than force a rewrite on a defensible answer).
  - When reporting, choose concern kind carefully. Use topic_mismatch only for wrong-subject answers; use coverage_gap / grounding_gap / diagram_gap / richness_gap for missing-surface concerns.
  - Choose repair_locus carefully. Do not call a missing-evidence problem a local_doc_defect. Do not call a formatting preference safety.
  - If you cannot name at least one concrete concern, emit sufficient=true. Never emit sufficient=false with an empty concerns array.
  - When reporting, name the public semantic label (facet label / relation kind / enrichment label, or "requested topic" for topic_mismatch). Use the same language as the answer text in the prose fields. Do NOT restate SYSTEM-DETECTED GAPS — concerns must be ADDITIONS.`

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
		Sufficient bool            `json:"sufficient"`
		Concerns   json.RawMessage `json:"concerns"`
		Confidence float64         `json:"confidence"`
		Reasoning  string          `json:"reasoning"`
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
	concerns, err := decodeSemanticQualityConcerns(parsed.Concerns, parsed.Sufficient)
	if err != nil {
		return nil, fmt.Errorf("decode emit_semantic_quality_review concerns: %w", err)
	}
	out.Concerns = append(out.Concerns, concerns...)
	if !out.Sufficient && len(out.Concerns) == 0 {
		out.Concerns = append(out.Concerns, semanticQualityFallbackConcern(out.Reasoning))
	}
	return out, nil
}

type semanticQualityConcernWire struct {
	Kind        string `json:"kind"`
	Topic       string `json:"topic"`
	Observation string `json:"observation"`
	Suggestion  string `json:"suggestion"`
	RepairLocus string `json:"repair_locus"`
}

func decodeSemanticQualityConcerns(raw json.RawMessage, sufficient bool) ([]SemanticQualityConcern, error) {
	if strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var list []semanticQualityConcernWire
	if err := json.Unmarshal(raw, &list); err == nil {
		return normalizeSemanticQualityConcerns(list), nil
	}
	var single semanticQualityConcernWire
	if err := json.Unmarshal(raw, &single); err == nil {
		return normalizeSemanticQualityConcerns([]semanticQualityConcernWire{single}), nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" || sufficient {
			return nil, nil
		}
		return []SemanticQualityConcern{semanticQualityFallbackConcern(text)}, nil
	}
	var original any
	if err := json.Unmarshal(raw, &original); err == nil {
		if sufficient {
			return nil, nil
		}
		return []SemanticQualityConcern{semanticQualityFallbackConcern(fmt.Sprintf("%v", original))}, nil
	}
	return nil, fmt.Errorf("unsupported concerns payload shape")
}

func normalizeSemanticQualityConcerns(list []semanticQualityConcernWire) []SemanticQualityConcern {
	if len(list) == 0 {
		return nil
	}
	out := make([]SemanticQualityConcern, 0, len(list))
	for _, c := range list {
		topic := strings.TrimSpace(c.Topic)
		kind := normaliseSemanticQualityConcernKind(c.Kind)
		repairLocus := normaliseSemanticQualityRepairLocus(c.RepairLocus, kind)
		obs := strings.TrimSpace(c.Observation)
		sug := strings.TrimSpace(c.Suggestion)
		if topic == "" || obs == "" || sug == "" {
			continue
		}
		out = append(out, SemanticQualityConcern{
			Kind: kind, Topic: topic, Observation: obs, Suggestion: sug, RepairLocus: repairLocus,
		})
	}
	return out
}

func semanticQualityFallbackConcern(reasoning string) SemanticQualityConcern {
	observation := clampReasoningForRepair(reasoning)
	if observation == "" {
		observation = "the reviewer marked the answer insufficient but did not emit a structured concern"
	}
	return SemanticQualityConcern{
		Kind:        semanticConcernCoverageGap,
		Topic:       "semantic quality review",
		Observation: observation,
		Suggestion:  "rewrite using only the typed evidence already supplied, and make the missing coverage or topic alignment described by the reviewer rationale explicit",
		RepairLocus: semanticRepairLocalDocDefect,
	}
}

func normaliseSemanticQualityConcernKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case semanticConcernTopicMismatch:
		return semanticConcernTopicMismatch
	case semanticConcernGroundingGap:
		return semanticConcernGroundingGap
	case semanticConcernDiagramGap:
		return semanticConcernDiagramGap
	case semanticConcernRichnessGap:
		return semanticConcernRichnessGap
	case semanticConcernCoverageGap:
		return semanticConcernCoverageGap
	default:
		return semanticConcernCoverageGap
	}
}

func normaliseSemanticQualityRepairLocus(locus, concernKind string) string {
	switch strings.TrimSpace(locus) {
	case semanticRepairLocalDocDefect:
		return semanticRepairLocalDocDefect
	case semanticRepairEvidenceGap:
		return semanticRepairEvidenceGap
	case semanticRepairAnalysisGap:
		return semanticRepairAnalysisGap
	case semanticRepairPresentationAdvisor:
		return semanticRepairPresentationAdvisor
	case semanticRepairSafety:
		return semanticRepairSafety
	default:
		// Back-compat for older adapters/tests that predate
		// repair_locus. Preserve the historical hard behaviour for
		// explicit wrong-topic verdicts; ordinary coverage concerns
		// remain soft unless separately promoted by operator config.
		return semanticRepairLocalDocDefect
	}
}

func semanticQualityEffectiveConfidenceFloor(defaultFloor float64, verdict *SemanticQualityResult) float64 {
	if defaultFloor <= 0 || defaultFloor > 1 {
		defaultFloor = SemanticQualityMinConfidenceDefault
	}
	if verdict == nil {
		return defaultFloor
	}
	for _, concern := range verdict.Concerns {
		if normaliseSemanticQualityConcernKind(concern.Kind) == semanticConcernTopicMismatch &&
			defaultFloor > SemanticQualityTopicMismatchMinConfidence {
			return SemanticQualityTopicMismatchMinConfidence
		}
	}
	return defaultFloor
}

func filterSemanticQualityConcernsByKind(concerns []SemanticQualityConcern, kind string) []SemanticQualityConcern {
	if len(concerns) == 0 {
		return nil
	}
	kind = normaliseSemanticQualityConcernKind(kind)
	out := make([]SemanticQualityConcern, 0, len(concerns))
	for _, concern := range concerns {
		if normaliseSemanticQualityConcernKind(concern.Kind) == kind {
			out = append(out, concern)
		}
	}
	return out
}

func semanticQualityViolationKind(concern SemanticQualityConcern) types.ViolationKind {
	if normaliseSemanticQualityConcernKind(concern.Kind) == semanticConcernTopicMismatch &&
		normaliseSemanticQualityRepairLocus(concern.RepairLocus, concern.Kind) != semanticRepairPresentationAdvisor {
		return types.ViolAnswerTopicMismatch
	}
	return types.ViolAnswerSemanticUnderfilled
}

// renderSemanticQualityUserMessage formats the input as labelled
// markdown sections so the reviewer's prompt-instructed signals
// match the input shape.
func renderSemanticQualityUserMessage(in SemanticQualityInput) string {
	var b strings.Builder
	if s := strings.TrimSpace(in.OriginalRequest); s != "" {
		fmt.Fprintf(&b, "## Original user request\n%s\n\n", s)
	}
	fmt.Fprintf(&b, "## SUMMARY\n%s\n\n", strings.TrimSpace(in.AnswerSummary))
	fmt.Fprintf(&b, "## BODY\n%s\n", strings.TrimSpace(in.AnswerBody))

	if len(in.RequiredFacets) > 0 {
		b.WriteString("\n## REQUIRED FACETS (typed coverage attestation)\n")
		b.WriteString("Each row: public facet label / tier / promoted / covered. Flag entries that are promoted=true but covered=false.\n\n")
		for _, f := range in.RequiredFacets {
			fmt.Fprintf(&b, "- facet=%q tier=%s promoted=%t covered=%t\n",
				types.AnswerFacetPublicLabelString(f.Kind), f.Tier, f.Promoted, f.Covered)
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
		b.WriteString("Each row: public facet label / typed evidence count. Flag at most ONE richness gap, and only when glaring.\n\n")
		for _, r := range in.RichnessCandidates {
			fmt.Fprintf(&b, "- facet=%q evidence_count=%d\n",
				types.AnswerFacetPublicLabelString(r.Kind), r.EvidenceCount)
		}
	}
	if len(in.EvidenceAnchorSet) > 0 {
		b.WriteString("\n## EVIDENCE ANCHORS (typed identifier set, for cross-reference)\n")
		for _, id := range in.EvidenceAnchorSet {
			fmt.Fprintf(&b, "- %s\n", id)
		}
	}
	if len(in.SystemDetectedGaps) > 0 {
		b.WriteString("\n## SYSTEM-DETECTED GAPS (already raised by structural checks — REVERSE-CHECK these)\n")
		b.WriteString("For each gap: judge whether the BODY above addresses it via equivalent expression. If yes → sufficient=true (you are the second opinion).\n\n")
		for _, g := range in.SystemDetectedGaps {
			switch g.Kind {
			case gapKindEnrichmentUncovered:
				fmt.Fprintf(&b, "- %s: facet=%q (typed evidence rows available: %d)\n",
					g.Kind, types.AnswerFacetPublicLabelString(g.FacetKind), g.EvidenceCount)
			case gapKindProseUnderfilled:
				fmt.Fprintf(&b, "- %s: block_id=`%s` (cited claims: %d, inline-code anchors in prose: 0)\n",
					g.Kind, g.BlockID, g.EvidenceCount)
			default:
				fmt.Fprintf(&b, "- %s\n", g.Kind)
			}
		}
	}
	if len(in.PromotedFacetCoverage) > 0 {
		b.WriteString("\n## PROMOTED FACET COVERAGE DEPTH (declared vs anchored)\n")
		b.WriteString("Each row: public facet label / DeclaredCount / AnchoredCount. DeclaredCount > 0 with AnchoredCount == 0 = label-only declaration without evidence anchor.\n\n")
		for _, c := range in.PromotedFacetCoverage {
			fmt.Fprintf(&b, "- facet=%q declared=%d anchored=%d\n",
				types.AnswerFacetPublicLabelString(c.Kind), c.DeclaredCount, c.AnchoredCount)
		}
	}
	if len(in.ClaimBindings) > 0 {
		b.WriteString("\n## CLAIM BINDINGS (typed origin / gate-policy handoff)\n")
		b.WriteString("Each row: claim_id / origin / policy / requested outputs / target / support_ref count. Origins are evidence provenance, not answer shapes. Do not demand current-source file:line grounding for non-current-source origins; even current-source bindings create citation pressure only when exact source support is present.\n\n")
		for _, cb := range in.ClaimBindings {
			exactSource := ""
			if cb.Origin == string(types.AnswerEvidenceOriginCurrentSource) {
				exactSource = fmt.Sprintf(" exact_source_support=%t", cb.ExactSourceSupport)
			}
			fmt.Fprintf(&b, "- claim_id=%q origin=`%s` policy=`%s` outputs=%s target=%q support_refs=%d%s\n",
				cb.ClaimID, cb.Origin, cb.Policy, strings.Join(cb.Outputs, ","), cb.Target, cb.SupportRefCount, exactSource)
		}
	}
	if len(in.Observations) > 0 {
		b.WriteString("\n## OBSERVATION LEDGER (typed evidence / external-resource view)\n")
		b.WriteString("Each row: record_id / origin / role / policy / source / span / claim / summary. Non-current-source rows are valid observations but are not current-repo citations; evaluate coverage using their origin-specific support.\n\n")
		for _, obs := range in.Observations {
			fmt.Fprintf(&b, "- record_id=%q origin=`%s` role=`%s` policy=`%s` source=%q",
				obs.ID, obs.Origin, obs.Role, obs.Policy, obs.Source)
			if obs.Span != "" {
				fmt.Fprintf(&b, " span=%q", obs.Span)
			}
			if obs.Claim != "" {
				fmt.Fprintf(&b, " claim=%q", obs.Claim)
			}
			if obs.Negative {
				b.WriteString(" negative=true")
			}
			if obs.ResultCount != nil {
				fmt.Fprintf(&b, " result_count=%d", *obs.ResultCount)
			}
			if obs.Summary != "" {
				fmt.Fprintf(&b, " summary=%q", obs.Summary)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func semanticClaimBindingSummaries(bindings []types.AnswerClaimBinding) []SemanticClaimBindingSummary {
	if len(bindings) == 0 {
		return nil
	}
	const maxSemanticClaimBindings = 16
	limit := len(bindings)
	if limit > maxSemanticClaimBindings {
		limit = maxSemanticClaimBindings
	}
	out := make([]SemanticClaimBindingSummary, 0, limit)
	for i := 0; i < limit; i++ {
		b := bindings[i]
		outputs := make([]string, 0, len(b.RequestedOutputs))
		for _, output := range b.RequestedOutputs {
			if output == types.AnswerRequestedOutputUnknown {
				continue
			}
			outputs = append(outputs, string(output))
		}
		out = append(out, SemanticClaimBindingSummary{
			ClaimID:            strings.TrimSpace(b.ClaimID),
			Origin:             string(b.Origin),
			Policy:             string(b.GroundingPolicy),
			Outputs:            outputs,
			Target:             strings.TrimSpace(b.TargetRef),
			SupportRefCount:    len(b.SupportRefs),
			ExactSourceSupport: types.AnswerClaimBindingHasExactCurrentSourceSupport(b),
		})
	}
	return out
}

func semanticObservationSummaries(ledger types.ObservationLedger, rm *types.RequestModel, contract *types.AnswerContract) []SemanticObservationSummary {
	if len(ledger.Records) == 0 {
		return nil
	}
	const maxSemanticObservations = 18
	records := types.PrioritizeObservationRecords(ledger.Records, rm, contract, maxSemanticObservations)
	limit := len(records)
	out := make([]SemanticObservationSummary, 0, limit)
	for i := 0; i < limit; i++ {
		record := records[i]
		out = append(out, SemanticObservationSummary{
			ID:          strings.TrimSpace(record.ID),
			Origin:      string(record.Origin),
			Role:        string(record.Role),
			Policy:      string(record.GroundingPolicy),
			Source:      semanticObservationSource(record.SourceRef),
			Span:        semanticObservationSpan(record.Span),
			Claim:       strings.TrimSpace(record.ClaimKey),
			Summary:     clampSemanticObservationText(record.Summary, 180),
			Negative:    record.Negative,
			ResultCount: record.ResultCount,
		})
	}
	return out
}

func semanticObservationSource(ref types.ObservationSourceRef) string {
	var parts []string
	if ref.Kind != types.ObservationSourceUnknown {
		parts = append(parts, string(ref.Kind))
	}
	for _, value := range []string{
		ref.Repo,
		ref.Path,
		ref.Commit,
		ref.Range,
		ref.Pathspec,
		ref.Command,
		ref.RawRef,
		ref.ArtifactID,
		ref.ArtifactKind,
		ref.URL,
		ref.Server,
		ref.ResourceURI,
		ref.Connector,
	} {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, clampSemanticObservationText(value, 90))
		}
	}
	return strings.Join(parts, " | ")
}

func semanticObservationSpan(span types.ObservationSpan) string {
	var parts []string
	if span.LineStart > 0 {
		if span.LineEnd > 0 && span.LineEnd != span.LineStart {
			parts = append(parts, fmt.Sprintf("lines %d-%d", span.LineStart, span.LineEnd))
		} else {
			parts = append(parts, fmt.Sprintf("line %d", span.LineStart))
		}
	}
	if span.OldLine > 0 {
		parts = append(parts, fmt.Sprintf("old_line %d", span.OldLine))
	}
	if span.NewLine > 0 {
		parts = append(parts, fmt.Sprintf("new_line %d", span.NewLine))
	}
	if span.Row > 0 {
		parts = append(parts, fmt.Sprintf("row %d", span.Row))
	}
	if span.JSONPointer != "" {
		parts = append(parts, fmt.Sprintf("json %s", clampSemanticObservationText(span.JSONPointer, 80)))
	}
	if span.StartTsMs != 0 || span.EndTsMs != 0 {
		parts = append(parts, fmt.Sprintf("%.3f-%.3fms", span.StartTsMs, span.EndTsMs))
	}
	return strings.Join(parts, ", ")
}

func clampSemanticObservationText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max > len(s) {
		max = len(s)
	}
	return strings.TrimSpace(s[:max]) + "…"
}

// countFacetCoverageDepth counts (declared, anchored) pairs for the
// named facet kind on doc:
//   - declared = number of sites (block.FacetIDs
//     / block.ClaimUses[].FacetID) that name this facet
//   - anchored = subset of declared sites that ALSO carry a non-empty
//     EvidenceID or ClaimForm on the same surface
//
// AnchoredCount < DeclaredCount signals shallow coverage (label-only
// declarations); reviewer reads as part of the depth audit.
func countFacetCoverageDepth(doc *types.AnswerDocumentV2, kind string) (declared, anchored int) {
	if doc == nil || kind == "" {
		return 0, 0
	}
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			if strings.TrimSpace(fid) != kind {
				continue
			}
			declared++
			// A block-level FacetIDs declaration is "anchored" when
			// any of its block-level ClaimUses
			// also declares this facet kind WITH a ClaimForm or
			// EvidenceID — proving the claim is grounded in evidence,
			// not just labelled. For list-shaped principal blocks,
			// citation-backed identifier rows (or rows with inline-code
			// anchors in item text) also count as anchored surface. For
			// diagram blocks, a non-empty typed diagram/edge anchor is the
			// visible surface for diagram_spine/component_relation facets;
			// otherwise the reviewer sees "declared but unanchored" even
			// though the user sees a diagram.
			if blockHasAnchoredClaim(b, kind) || blockHasAnchoredListSurface(b) || blockHasAnchoredDiagramSurface(b) {
				anchored++
			}
		}
		for _, cu := range b.ClaimUses {
			if strings.TrimSpace(cu.FacetID) != kind {
				continue
			}
			declared++
			if cu.EvidenceID != "" || cu.ClaimForm != "" {
				anchored++
			}
		}
	}
	return declared, anchored
}

func blockHasAnchoredListSurface(b types.AnswerBlock) bool {
	switch b.Kind {
	case types.BlockOrderedList, types.BlockBulletList:
	default:
		return false
	}
	for _, item := range b.Items {
		if item.CitationRef < 0 {
			continue
		}
		if looksLikeIdentifierShape(strings.TrimSpace(item.Label)) || countInlineCodeSegments(item.Text) > 0 {
			return true
		}
	}
	return false
}

func blockHasAnchoredDiagramSurface(b types.AnswerBlock) bool {
	if b.Kind != types.BlockDiagram {
		return false
	}
	if b.Diagram != nil && strings.TrimSpace(b.Diagram.Body) != "" {
		return true
	}
	return len(b.EdgeAnchors) > 0
}

// blockHasAnchoredClaim reports whether the block has any block-level
// claim_use for the named facet kind that also names a ClaimForm or
// EvidenceID. Used by countFacetCoverageDepth to distinguish label-only
// declarations from grounded ones.
func blockHasAnchoredClaim(b types.AnswerBlock, kind string) bool {
	for _, cu := range b.ClaimUses {
		if strings.TrimSpace(cu.FacetID) == kind && (cu.EvidenceID != "" || cu.ClaimForm != "") {
			return true
		}
	}
	return false
}

// BuildSemanticQualityInput projects the orchestrator's typed state
// into the reviewer input. Pure typed projection (R3 invariant);
// no heuristic, no similarity, no frequency. Caller passes the
// already-built EvidenceAnchorSet (shared SST with self-consistency).
//
// v3 B2 (2026-05-04): also computes SystemDetectedGaps + PromotedFacetCoverage
// from the typed validators (validateRichnessGlaringGap +
// validatePrincipalProseUnderfilled) so reviewer can reverse-check
// rather than rediscover the gaps.
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
	}

	// v3 B2: project SystemDetectedGaps from the typed validators so
	// reviewer reverse-checks rather than rediscovers.
	for _, vio := range validateRichnessGlaringGap(doc, view) {
		facetKind := extractFacetKindFromDetail(vio.Detail)
		evidenceCount := 0
		if facetKind != "" && view.FacetCoverage != nil {
			for _, req := range view.FacetCoverage.Optional {
				if string(req.Kind) == facetKind {
					evidenceCount = len(req.SourceCandidate)
					break
				}
			}
		}
		in.SystemDetectedGaps = append(in.SystemDetectedGaps, SystemDetectedGap{
			Kind:          gapKindEnrichmentUncovered,
			FacetKind:     facetKind,
			EvidenceCount: evidenceCount,
		})
	}
	for _, vio := range validatePrincipalProseUnderfilled(doc) {
		blockID := extractBlockIDFromDetail(vio.Detail)
		typedClaimCount := 0
		for _, b := range doc.Blocks {
			if b.ID != blockID {
				continue
			}
			switch b.Kind {
			case types.BlockSummary, types.BlockSection, types.BlockCaveat:
				typedClaimCount = len(b.ClaimUses)
			case types.BlockOrderedList, types.BlockBulletList:
				for _, item := range b.Items {
					if item.CitationRef >= 0 {
						typedClaimCount++
					}
				}
			}
			break
		}
		in.SystemDetectedGaps = append(in.SystemDetectedGaps, SystemDetectedGap{
			Kind:          gapKindProseUnderfilled,
			BlockID:       blockID,
			EvidenceCount: typedClaimCount,
		})
	}

	// PromotedFacetCoverage depth audit — declared vs anchored per
	// emit-hard facet. AnchoredCount = sites that declare BOTH
	// FacetID AND non-empty EvidenceID/ClaimForm on the same surface.
	if view.FacetCoverage != nil {
		for _, req := range view.FacetCoverage.Required {
			if !req.RequiresHardDeclaration() {
				continue
			}
			kind := strings.TrimSpace(string(req.Kind))
			if kind == "" {
				continue
			}
			declared, anchored := countFacetCoverageDepth(doc, kind)
			in.PromotedFacetCoverage = append(in.PromotedFacetCoverage, FacetCoverageDepth{
				Kind:          kind,
				DeclaredCount: declared,
				AnchoredCount: anchored,
			})
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
		var diagramBlock *types.AnswerBlock
		for i := range doc.Blocks {
			b := &doc.Blocks[i]
			if b.Kind == types.BlockDiagram && b.Diagram != nil {
				diagramBlock = b
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
		relationCounts := semanticQualityDiagramRelationCounts(doc, diagramBlock)
		for _, contract := range view.DiagramPlan.EdgeRelations {
			if contract.Min <= 0 {
				continue
			}
			summary.Edges = append(summary.Edges, SemanticDiagramEdgeContract{
				RelationKind: string(contract.Kind),
				MinExpected:  contract.Min,
				MinSatisfied: relationCounts[contract.Kind],
			})
		}
		in.DiagramContract = summary
	}

	return in
}

func semanticQualityDiagramRelationCounts(doc *types.AnswerDocumentV2, diagramBlock *types.AnswerBlock) map[types.DiagramRelationKind]int {
	counts := make(map[types.DiagramRelationKind]int)
	if doc == nil || diagramBlock == nil || diagramBlock.Diagram == nil {
		return counts
	}
	edges := parseMermaidEdges(diagramBlock.Diagram.Body)
	if len(edges) == 0 {
		return counts
	}
	support := buildDiagramSupportTokens(doc, diagramBlock)
	typedRelIndex := buildTypedRelationIndex(doc)
	for _, edge := range edges {
		if !diagramTokenSupported(edge.from, support) || !diagramTokenSupported(edge.to, support) {
			continue
		}
		resolved := resolveDiagramEdgeRelation(diagramBlock, edge, typedRelIndex)
		rel := resolved.Relation
		if rel != types.DiagramRelUnknown {
			counts[rel]++
		}
	}
	return counts
}
