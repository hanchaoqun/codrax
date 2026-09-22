package skill_test

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

var runtimeCarrierTeachingCases = []struct {
	name string
	kind types.AnswerBlockKind
}{
	{"runtime_scalar", types.BlockScalar},
	{"runtime_decision", types.BlockDecision},
	{"runtime_enumeration", types.BlockOrderedList},
	{"log_hops", types.BlockOrderedList},
	{"source_scalar", types.BlockScalar},
	{"source_decision", types.BlockDecision},
	{"source_enumeration", types.BlockOrderedList},
	{"source_hops", types.BlockOrderedList},
}

func runtimeCarrierTeachingContext(name, language string) *types.AgentContext {
	ac := summaryScopeTeachingContext("source_mechanism", language)
	if strings.HasPrefix(name, "runtime_") {
		ac = summaryScopeTeachingContext("finite_trace", language)
	}
	rm := &ac.AnalysisIR.RequestModel
	switch name {
	case "runtime_scalar", "source_scalar":
		rm.AnalyzerHints.Kind = string(types.ReqReturnValue)
		rm.Predicates.IsScalarAnswer = true
		rm.Predicates.IsRoleLocateLookup = true
		rm.AnswerSubject = types.AnswerSubject{Kind: types.SubjectStringLiteral, Confidence: 1}
	case "runtime_decision":
		rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeBoundedEffectVerdict
	case "runtime_enumeration", "source_enumeration":
		rm.Intent = types.IntentEnumerate
		rm.AnalyzerHints.Kind = string(types.ReqEnumeration)
		if name == "runtime_enumeration" {
			rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeSystemOverview}
			rm.PerfTrace = &types.PerfBundle{}
		}
	case "log_hops":
		ac = summaryScopeTeachingContext("finite_log", language)
		rm = &ac.AnalysisIR.RequestModel
		rm.RuntimeQuestionProfile = nil
		rm.Intent = types.IntentTrace
		rm.AnalyzerHints.Kind = string(types.ReqCallChain)
	case "source_hops":
		rm.Intent = types.IntentTrace
		rm.AnalyzerHints.Kind = string(types.ReqCallChain)
	}
	return ac
}

func requireRuntimeCarrierTeachingShape(t *testing.T, ac *types.AgentContext, kind types.AnswerBlockKind) {
	t.Helper()
	view := types.BuildAnswerSemanticViewForAgentContext(ac)
	if view == nil {
		t.Fatal("missing compiled answer view")
	}
	for _, blocks := range [][]types.BlockRequirement{view.RequiredBlocks, view.OptionalBlocks} {
		for _, block := range blocks {
			if block.AcceptsKind(kind) {
				return
			}
		}
	}
	if view.Presentation.AllowsBlock(kind) {
		return
	}
	t.Fatalf("fixture must exercise a legal %s carrier, family=%s", kind, view.Family)
}

// Select every independently rendered carrier rule, not arbitrary occurrences
// in user evidence. The shared helper exercises registry -> BuildPromptContext
// -> ToMessages and returns only actual system messages without mutating input.
func runtimeCarrierTeachingParagraphs(t *testing.T, system string) map[string]string {
	t.Helper()
	markers := map[string]string{
		"scalar_output":                 "- `scalar` —",
		"decision_workflow":             "For a `decision` block",
		"decision_output":               "- `decision` —",
		"enumeration_workflow":          "For an enumeration `ordered_list` block",
		"external_enumeration_workflow": "For an `ordered_list` block over enumeration items",
		"hop_workflow":                  "For a hop-chain `ordered_list` block",
	}
	out := make(map[string]string)
	for _, line := range strings.Split(system, "\n") {
		for key, marker := range markers {
			if strings.Contains(line, marker) {
				out[key] = line
			}
		}
	}
	voiceStart, voiceEnd := strings.Index(system, "Prose voice"), strings.Index(system, "Voice register")
	if voiceStart >= 0 && voiceEnd > voiceStart {
		out["prose_voice"] = system[voiceStart:voiceEnd]
	}
	for key := range markers {
		if out[key] == "" {
			t.Errorf("missing independent teaching surface %s", key)
		}
	}
	if out["prose_voice"] == "" {
		t.Error("missing independent general prose voice teaching")
	}
	return out
}

func TestRuntimeCarrierTeachingRejectsSourceOnlyDefaults(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, tc := range runtimeCarrierTeachingCases {
			t.Run(language+"/"+tc.name, func(t *testing.T) {
				ac := runtimeCarrierTeachingContext(tc.name, language)
				requireRuntimeCarrierTeachingShape(t, ac, tc.kind)
				paragraphs := runtimeCarrierTeachingParagraphs(t, summaryScopeSystemTeaching(t, ac))
				for surface, phrases := range map[string][]string{
					"scalar_output": {
						"State what the value means, where it comes from in the call graph / config lineage, and why the cited file:line is the authoritative source.",
					},
					"decision_workflow": {
						"name the invariant or guard that forces the answer, reference the load-bearing identifiers with inline `code`, and explain the mechanism at whatever depth the subtlety requires.",
						"For non-typed decision blocks, set the block's `claim_uses=[{claim_form=guard_condition}]` or `[{claim_form=definition_fact}]` matching whichever the cited line shape is.",
					},
					"decision_output": {
						"Put the verdict and the core reasoning together in the decision block's `text` field, and explain the mechanism at the depth the subtlety requires.",
					},
					"enumeration_workflow": {
						"Declare the block's `claim_uses=[{claim_form=definition_fact}]` (or `call_edge` / `assignment_fact` when the cited lines are call sites or assignments) at block level.",
					},
					"external_enumeration_workflow": {
						"every item's resolved `file/line` (pointed at via top-level `items[i].citation_ref`) must be a real repo anchor where the named identifier actually appears.",
						"either drop the item or keep the answer in summary / caveat prose without pretending the item is repo-grounded",
					},
					"hop_workflow": {
						"emit `items[]` with one entry per distinct branch or mechanism hop.",
						"stays unsupported unless a repo file:line evidence row really backs it",
					},
					"prose_voice": {
						"- Describe what the code DOES and why it matters, not the raw relation between functions (who calls whom at which line). A step body or a rationale is a mechanism description, not a machine-readable edge list.",
					},
				} {
					for _, phrase := range phrases {
						if strings.Contains(paragraphs[surface], phrase) {
							t.Errorf("%s still makes carrier shape imply source-code authority/mechanism demand: %q", surface, phrase)
						}
					}
				}
			})
		}
	}
}

func TestRuntimeCarrierTeachingUsesActualEvidenceAndPreservesMembers(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, tc := range runtimeCarrierTeachingCases {
			t.Run(language+"/"+tc.name, func(t *testing.T) {
				ac := runtimeCarrierTeachingContext(tc.name, language)
				requireRuntimeCarrierTeachingShape(t, ac, tc.kind)
				paragraphs := runtimeCarrierTeachingParagraphs(t, summaryScopeSystemTeaching(t, ac))
				// Every independent teaching surface must state its own evidence
				// boundary. A correct scalar/summary rule cannot mask a conflicting
				// decision, enumeration, hop, or general-voice rule elsewhere.
				for surface, phrases := range map[string][]string{
					"scalar_output": {
						"schema-selected literal carrier",
						"subject, meaning, actual source, and relevant measurement scope",
						"Explain a call graph / config lineage only for a requested source-derived value supported by that evidence",
						"An external measurement or recorded identity does not require a current-source citation",
						"retain its artifact/command provenance and do not invent file:line proof",
					},
					"decision_workflow": {
						"explain the accepted facts and conditions supporting the verdict, with mechanism details only where needed and evidenced",
						"For non-typed decision blocks, choose `claim_uses` from the projected allowed forms to match the actual evidence",
						"external observations do not become code guard/definition facts merely because the answer is a verdict",
						"leave that item uncited when the decision is sourced from log / external trace rather than repo code",
					},
					"decision_output": {
						"schema-selected verdict carrier and rationale fields",
						"supporting observations, conditions, and evidence boundary at the depth the question needs",
						"include mechanism details when needed and evidenced, not merely because this is a decision",
					},
					"enumeration_workflow": {
						"prior extraction slate and required members",
						"Choose block-level `claim_uses` from the projected allowed forms to match the evidence",
						"source declarations, calls, and assignments use their supported code forms",
						"accepted runtime/artifact observations use `external_observation` when allowed",
						"source symbol, subject/object, or recorded artifact identity",
						"do not invent labels or require an external identity to appear in a current-source snippet",
					},
					"external_enumeration_workflow": {
						"any current-source file:line citation must be a real grounded repo anchor for the claimed fact",
						"preserve the requested members and required list carrier",
						"do not drop an item or move it to summary merely because it lacks a current-source location",
						"Use `external_observation` when allowed by the projected schema and retain the artifact/command/history provenance",
						"a recorded external stack location remains an artifact fact unless independently grounded in the current checkout",
					},
					"hop_workflow": {
						"one entry per distinct supported hop",
						"Explain each code step's behavior, guard, or effect from its own evidence",
						"only the relationships established within that artifact's scope",
						"one item is one logical hop, do not collapse two",
						"current-source claims need grounded repo evidence, while accepted external observations retain their own provenance",
						"a recorded frame does not prove an extra caller/callee or causal edge",
						"When an item intentionally asserts a proved directed relation, include a compact explicit edge surface",
						"keep boundary, comparison, and exclusion context as prose without an arrow",
					},
					"prose_voice": {
						"requested facets and each fact's evidence source determine what to explain",
						"a block shape does not turn an observation into a code mechanism",
						"For requested code-behavior explanations: Describe what the code DOES and why it matters, with evidence for each behavior",
						"For runtime/artifact facts, describe the supported observations, scope, and uncertainty",
						"add a mechanism explanation only when requested or needed for the answer and independently supported",
					},
				} {
					for _, phrase := range phrases {
						if !strings.Contains(paragraphs[surface], phrase) {
							t.Errorf("%s lost required evidence/member/relationship teaching %q", surface, phrase)
						}
					}
				}
			})
		}
	}
}

func TestRuntimeCarrierTeachingPreservesEvidenceAndSchemaBoundaries(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		for _, tc := range runtimeCarrierTeachingCases {
			t.Run(language+"/"+tc.name, func(t *testing.T) {
				ac := runtimeCarrierTeachingContext(tc.name, language)
				requireRuntimeCarrierTeachingShape(t, ac, tc.kind)
				system := summaryScopeSystemTeaching(t, ac)
				runtimeCarrierTeachingParagraphs(t, system)
				for _, want := range []string{
					"projected `emit_answer_document` tool schema",
					"Do not reconstruct a second schema from prose guidance",
					"Required Answer Blocks",
					"Evidence-entailment boundary (all code explanations)",
					"A call-site citation authorizes only caller -> callee",
					"Guard/return/assignment claims require their own grounded condition/return/assignment evidence",
					"When an explanation crosses functions or stages, cite separate evidence for each hop",
					types.AnswerDocumentItemCitationCarrierTeaching,
					types.GroundedSourceDiagramEdgeOwnershipContract,
					types.GroundedSourceDiagramRelationEvidenceContract,
					"Grounded log frames preserve their recorded identities and within-stack order",
					"without inventing cross-stack continuations or additional caller/callee links",
					"Runtime Trace diagrams retain their separate typed causal authority",
					"Display choices neither create nor remove typed evidence, projection, or automatic completion authority",
				} {
					if !strings.Contains(system, want) {
						t.Errorf("carrier-teaching repair lost an independent authority contract %q", want)
					}
				}
			})
		}
	}
}
