package skill

import (
	"strings"
	"testing"
)

// allWorkflowBodies returns the concatenated Workflow + WorkflowTierB
// bodies. P5-B (2026-05-10) introduced the Tier B partition; tests
// that pin rule body presence (regardless of the rule's tier) call
// this helper instead of touching sk.Workflow directly.
func allWorkflowBodies(sk *Config) string {
	parts := append([]string(nil), sk.Workflow...)
	for _, item := range sk.WorkflowTierB {
		parts = append(parts, item.Body)
	}
	return strings.Join(parts, "\n")
}

// allProhibitionBodies — same dual-tier helper for Prohibitions.
func allProhibitionBodies(sk *Config) string {
	parts := append([]string(nil), sk.Prohibitions...)
	for _, item := range sk.ProhibitionsTierB {
		parts = append(parts, item.Body)
	}
	return strings.Join(parts, "\n")
}

func TestExploreSkillOutputFormatStaysToolFirst(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	if strings.Contains(sk.OutputFormat, "\nAnswer:") || strings.Contains(sk.OutputFormat, "\nEvidence:\n") {
		t.Fatalf("explore-skill OutputFormat must not teach answer-shaped labels:\n%s", sk.OutputFormat)
	}
	if !strings.Contains(sk.OutputFormat, "emit_evidence") {
		t.Fatalf("explore-skill OutputFormat must mention emit_evidence:\n%s", sk.OutputFormat)
	}
	if !strings.Contains(sk.OutputFormat, "emit_investigation_complete") {
		t.Fatalf("explore-skill OutputFormat must mention emit_investigation_complete:\n%s", sk.OutputFormat)
	}
}

func TestWriteControllerSkillIsTypedDecisionOnly(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("write-controller-skill")
	if err != nil {
		t.Fatalf("Get(write-controller-skill) returned error: %v", err)
	}
	if len(sk.ToolSuggestions) != 1 || sk.ToolSuggestions[0] != "emit_write_workflow_decision" {
		t.Fatalf("write-controller tool suggestions = %v", sk.ToolSuggestions)
	}
	corpus := strings.ToLower(sk.Goal + "\n" + sk.OutputFormat + "\n" + allWorkflowBodies(sk) + "\n" + allProhibitionBodies(sk))
	for _, want := range []string{
		"typed workflow action",
		"action is the only controller routing signal",
		"emit_write_workflow_decision",
		"explore_code",
		"plan_batch",
		"apply_plan",
		"verify_batch",
		"append_batch",
		"split_batch",
		"replan_batch",
		"ask_user",
		"finish",
		"block",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("write-controller skill missing %q:\n%s", want, corpus)
		}
	}
	for _, banned := range []string{
		"keyword",
		"if the request says",
		"if the user says",
		"summary contains",
		"rationale contains",
		"parse prose",
		"plan_change_batch",
		"apply_ready_plan",
	} {
		if strings.Contains(corpus, banned) {
			t.Fatalf("write-controller skill contains prose-routing smell %q:\n%s", banned, corpus)
		}
	}
}

func TestWriteAnalysisSkillRenderedPlacementGuidanceIsTyped(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("write-analysis-skill")
	if err != nil {
		t.Fatalf("Get(write-analysis-skill) returned error: %v", err)
	}
	corpus := strings.ToLower(sk.Goal + "\n" + sk.OutputFormat + "\n" + allWorkflowBodies(sk) + "\n" + allProhibitionBodies(sk))
	for _, want := range []string{
		"rendered-text placement contracts",
		"behavior_contracts[].placement",
		"surface",
		"anchor",
		"expected text",
		"relation",
		"delimiter",
		"placement_refs[]",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("write-analysis placement guidance missing %q:\n%s", want, corpus)
		}
	}
	for _, banned := range []string{
		"if the request says",
		"if the user says",
		"summary contains",
		"rationale contains",
		"parse prose",
	} {
		if strings.Contains(corpus, banned) {
			t.Fatalf("write-analysis skill contains prose-routing smell %q:\n%s", banned, corpus)
		}
	}
}

// TestExploreSkillR6_NoInternalGateJargon — 2026-05-10 audit. The
// EVIDENCE_FLOOR_WAIVER skill prompt described the waiver's effect
// using internal pipeline gate names ("forced-read and citation-
// floor gates", "leave the gates active"). Those are implementation
// concepts, not user-domain language — R6 forbids them in LLM-
// facing strings. This test pins the user-domain phrasing so a
// future skill-prompt edit cannot regress.
func TestExploreSkillR6_NoInternalGateJargon(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	// Walk every LLM-facing surface on the skill — Goal /
	// Workflow / Prohibitions / OutputFormat all flow into the
	// rendered system prompt verbatim, so any banned token in any
	// of them is a real R6 leak.
	corpus := sk.Goal + "\n" + sk.OutputFormat
	for _, w := range sk.Workflow {
		corpus += "\n" + w
	}
	for _, p := range sk.Prohibitions {
		corpus += "\n" + p
	}
	for _, banned := range []string{
		"forced-read gate",
		"forced-read and citation-floor gates",
		"citation-floor gate",
		"leave the gates active",
		"L1 gate",
		"L2 gate",
		"L3 gate",
		"L4 gate",
		"BusContext",
		"MutableState",
		"AnalysisIR",
	} {
		if strings.Contains(corpus, banned) {
			t.Errorf("internal pipeline term %q leaked into LLM-facing explore-skill text", banned)
		}
	}
}

func TestExploreSkill_TeachesCascadedRepoLensNavigation(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	corpus := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		`repo_map(view="source_inventory")`,
		"model-chosen roles",
		"optional attribute_roles",
		"cascade into narrower source_inventory calls",
		"verify selected behavior or implementation claims with read_file or targeted grep before citing source text",
		"verified navigation facts",
		"not semantic source-code citations",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("explore-skill missing repo lens guidance %q:\n%s", want, corpus)
		}
	}
	for _, forbidden := range []string{
		"do not treat repo_map output as evidence",
		"downstream synthesis",
		"downstream rendering",
		"the framework has",
		"stage's tool allowlist",
		"mid-loop observer",
	} {
		if strings.Contains(corpus, forbidden) {
			t.Fatalf("explore-skill leaked internal mechanism phrase %q:\n%s", forbidden, corpus)
		}
	}
}

func TestExploreSkill_TraceQueryGuidanceIsTraceGated(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	if len(sk.Workflow) < 1 {
		t.Fatalf("explore-skill Workflow too short: %#v", sk.Workflow)
	}
	if strings.Contains(strings.Join(sk.Workflow, "\n"), "TRACE QUERY:") ||
		strings.Contains(strings.Join(sk.Workflow, "\n"), "RUNTIME TRACE FIRST") {
		t.Fatalf("runtime trace guidance must not live in always-rendered explorer workflow:\n%v", sk.Workflow)
	}
	if !strings.Contains(sk.Workflow[0], "PHASE 1") ||
		!strings.Contains(sk.Workflow[0], "repo_map") {
		t.Fatalf("generic source breadth scan should remain the first always-rendered item:\n%s", sk.Workflow[0])
	}
	if len(sk.WorkflowTierB) < 3 {
		t.Fatalf("explore-skill should carry trace guidance in WorkflowTierB: %#v", sk.WorkflowTierB)
	}
	traceTier := strings.Join([]string{sk.WorkflowTierB[0].Body, sk.WorkflowTierB[1].Body, sk.WorkflowTierB[2].Body}, "\n")
	for i := 0; i < 3; i++ {
		if !sk.WorkflowTierB[i].AppliesTo.RequiresTrace {
			t.Fatalf("trace workflow item %d must be gated by RequiresTrace: %+v", i, sk.WorkflowTierB[i].AppliesTo)
		}
	}
	for _, want := range []string{
		"RUNTIME TRACE FIRST",
		"start with `trace_query`",
		"TRACE QUERY:",
		"pattern=\"<literal>\"",
		"not regex",
		"mixed trace+source",
		"PERF SAMPLE PROVENANCE",
	} {
		if !strings.Contains(traceTier, want) {
			t.Fatalf("trace-gated workflow missing %q:\n%s", want, traceTier)
		}
	}
}

func TestExploreSkill_SourceOperationSiteSetHandoff(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	corpus := allWorkflowBodies(sk)
	for _, want := range []string{
		"Source operation-site sets",
		"all write points",
		"principal `aggregate_facts.member_set`",
		"support_refs",
		"target constants, paths",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("source operation-site handoff guidance missing %q:\n%s", want, corpus)
		}
	}
}

func TestExploreSkill_CoverageBeforeCompletionIsLimitedToStructuralCoverageObligations(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	corpus := allWorkflowBodies(sk)
	for _, want := range []string{
		"structural coverage obligation",
		"exhaustive-coverage demand",
		"declared item count",
		"partition into named groups",
		"Mechanism, architecture, and call-chain explanations do NOT require every navigation candidate to be exhausted",
		"read and cite the load-bearing files that prove the flow",
		"collateral candidates as optional navigation hints",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("explore-skill coverage guidance missing %q:\n%s", want, corpus)
		}
	}
}

func TestFinalizerSkillStepListPrefersDiagramsWhenHelpful(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	for _, want := range []string{
		"Even when the Diagram Contract does NOT require one",
		"directly serves the user's requested answer",
		"Do not add a diagram as generic enrichment",
		"do not let a diagram replace the block shape",
	} {
		if !strings.Contains(sk.OutputFormat, want) {
			t.Fatalf("finalize-skill OutputFormat missing %q:\n%s", want, sk.OutputFormat)
		}
	}
}

func TestFinalizerSkillKeepsInternalJargonOutOfUserProse(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	for _, want := range []string{
		"Keep internal pipeline jargon out of the user-facing prose",
		"\"grounded\"",
		"'grep' / 'read_file' / 'repo_map' found nothing.",
	} {
		if !strings.Contains(sk.OutputFormat, want) {
			t.Fatalf("answer-document-skill OutputFormat missing %q:\n%s", want, sk.OutputFormat)
		}
	}
}

func TestFinalizerSkill_DoesNotTeachRetiredV1AnswerPayloads(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, banned := range []string{
		"value{literal, citation_ref}",
		"value{key, literal, citation_ref}",
		"boolean{decision, rationale, citation_ref}",
		"boolean.rationale",
		"include the candidate in symbols[]",
		"symbols_completeness",
		"retired carrier shape",
	} {
		if strings.Contains(blob, banned) {
			t.Fatalf("answer-document-skill must not teach retired V1 payload %q:\n%s", banned, blob)
		}
	}
	for _, want := range []string{
		// scalar / decision V2 guidance — per-kind rules 129/130 carry
		// the active phrasing; rule 121 keeps the V1-rejection note.
		"put the literal directly in the block's `text` field as the rendered value",
		"Otherwise put the verdict at the START of the block's `text` field",
		"Put the verdict and the core reasoning together in the decision block's `text` field",
		"top-level `value` / `boolean` payloads are not part of this tool's schema",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("answer-document-skill missing V2 guidance %q:\n%s", want, blob)
		}
	}
}

func TestFinalizerSkill_TeachesNativeBlocksArrayContract(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"native JSON array",
		"do NOT quote it as a string containing escaped JSON",
		"not as a JSON-encoded string with escaped quotes",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("answer-document-skill missing native blocks[] guidance %q:\n%s", want, blob)
		}
	}
}

func TestFinalizerSkill_TeachesTypedDiagramRelationAuthority(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	// P5-B (2026-05-10): edge_anchors rule moved to WorkflowTierB
	// (RequiresDiagram applicability gate). Test reads from the
	// combined Tier A + Tier B surface so the assertion is
	// tier-agnostic — it pins rule presence regardless of where
	// the body lives in the Config struct.
	blob := strings.Join([]string{sk.Goal, sk.OutputFormat, allWorkflowBodies(sk)}, "\n")
	for _, want := range []string{
		"`edge_anchors` is the OPTIONAL block-level array for diagram-edge typed anchors",
		"relation_kind?: <one of call|guard|import|precedence|contain|observe>",
		"PREFERRED: set `relation_kind` directly",
		"the authoritative semantic relation",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("answer-document-skill missing typed diagram relation guidance %q:\n%s", want, blob)
		}
	}
}

func TestFinalizerSkill_TeachesTextReferenceAndExplicitRelationSurface(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	blob := strings.Join([]string{sk.Goal, sk.OutputFormat, allWorkflowBodies(sk)}, "\n")
	for _, want := range []string{
		"`text_reference_fact`",
		"visible source / config / doc / comment text itself is the evidence",
		"explicit edge surface",
		"Boundary / comparison / exclusion prose",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("answer-document-skill missing typed carrier guidance %q:\n%s", want, blob)
		}
	}
}

func TestFinalizerSkill_ClarifiesFacetIDAndVerticalDiagramPreference(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"claim annotations use singular `facet_id`",
		"plural `facet_ids` belongs on the block",
		"`flowchart TD` by default",
		"keep participant labels short because actors render horizontally",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("answer-document-skill missing clarified V2 contract guidance %q:\n%s", want, blob)
		}
	}
}

func TestFinalizerSkill_TypedDecisionVerdictIsCarrier(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	blob := strings.Join([]string{sk.Goal, sk.OutputFormat, allWorkflowBodies(sk), strings.Join(sk.Prohibitions, "\n")}, "\n")
	for _, want := range []string{
		"For a principal `decision` block that carries an active typed verdict field",
		"that verdict field is the carrier",
		"do not guess a `claim_uses[]` form",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("answer-document-skill missing typed decision carrier guidance %q:\n%s", want, blob)
		}
	}
}

// TestFinalizerSkill_TeachesAbstractionLevelMatching pins the
// abstraction-level matching workflow rule (added 2026-05-05 for
// task #8). The rule defends against the LLM-jitter failure mode
// where finalizer describes pipeline stages / agents / steps using
// implementation chains ("X calls Y which builds Z") instead of
// conceptual responsibility ("X is responsible for <outcome>") on
// "what does each X do" enumeration questions. The workflow sub-
// strings asserted here are the structurally load-bearing pieces:
// the rule's trigger condition, the positive/negative pattern, and
// the explicit non-applicability carve-out for "how does X work"
// mechanism questions. If a future refactor drops or reword this
// rule, the test fails with a pointer back to task #8.
func TestFinalizerSkill_TeachesAbstractionLevelMatching(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	// P5-B (2026-05-10): abstraction-level matching rule moved to
	// WorkflowTierB (Intents=[enumerate]+PrincipalKinds=[ordered_list]).
	// Combined Tier A + Tier B surface so the body assertion stays
	// stable regardless of where the rule lives.
	blob := strings.Join([]string{sk.Goal, sk.OutputFormat, allWorkflowBodies(sk)}, "\n")

	// Trigger phrase — recognises the question shape that activates
	// the rule. Two language surfaces (English + Chinese) so the
	// rule fires on both 'what does each X do' and '每个 X 负责什么'.
	for _, trigger := range []string{
		"what does each X do",
		"每个 X 负责什么",
	} {
		if !strings.Contains(blob, trigger) {
			t.Errorf("abstraction-level matching rule missing trigger phrase %q (task #8 regression)", trigger)
		}
	}

	// Positive vs negative pattern — the rule distinguishes a
	// conceptual responsibility answer from an implementation-
	// chain regression. Both must appear so the LLM has a clear
	// pair to compare against.
	if !strings.Contains(blob, "is responsible for") {
		t.Error("abstraction-level rule missing positive pattern 'is responsible for' (task #8)")
	}
	if !strings.Contains(blob, "IMPLEMENTATION CHAIN") {
		t.Error("abstraction-level rule missing negative pattern 'IMPLEMENTATION CHAIN' (task #8)")
	}

	// Non-applicability carve-out — mechanism / how-does-it-work
	// enumerations legitimately want implementation chains. The
	// rule must not bleed into those question shapes.
	if !strings.Contains(blob, "how does each X work") {
		t.Error("abstraction-level rule must carve out 'how does each X work' mechanism enumerations (task #8)")
	}
}

func TestExtractSkill_DoesNotTeachLegacySymbolsArray(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("extract-skill")
	if err != nil {
		t.Fatalf("Get(extract-skill) returned error: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, banned := range []string{
		"Every item in symbols[]",
		"emit answer_symbol in symbols[]",
		"shape-based prompt",
		"Mechanism-shape answers",
		"mechanism-shaped",
		"enumeration / call-chain (the slate IS the answer)",
	} {
		if strings.Contains(blob, banned) {
			t.Fatalf("extract-skill must not teach legacy symbols[] payload wording %q:\n%s", banned, blob)
		}
	}
	for _, want := range []string{
		"emit_answer_symbol.items[]",
		"the answer is the terminal that the chain RESOLVES TO",
		"final rendering answers from prose / blocks only",
		"explicitly renders an `Anchor skeleton (one per sub-topic)` block",
		"Classification sub_topics alone are guidance, not a hard slate obligation",
		"Requested Set Boundary block declares an explicit count N",
		"Plain single-topic call-chain / root-cause / mechanism questions WITHOUT case (b) or (c) do NOT use emit_answer_symbol",
		"does NOT explicitly say `This dispatch does NOT require emit_answer_symbol`",
		"principal set has NOT already been accepted as an aggregate_facts.member_set",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("extract-skill missing updated answer-symbol slate guidance %q:\n%s", want, blob)
		}
	}
}

// TestChangePlanSkill_BatchLocalPlanningWorkflow verifies the planner
// stays scoped to the active controller batch. Workflow expansion is a
// typed controller concern; the planner emits one bounded ChangePlan.
func TestChangePlanSkill_BatchLocalPlanningWorkflow(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	wf := strings.Join([]string{sk.Goal, sk.OutputFormat, allWorkflowBodies(sk), allProhibitionBodies(sk)}, "\n")
	for _, want := range []string{
		"active write workflow batch",
		"BATCH CONTEXT FIRST",
		"BATCH-LOCAL EXPLORATION",
		"BOUNDED PLAN",
		"WriteContextPack",
		"EMIT THE BOUNDED PLAN THROUGH ONE STRUCTURED PATH",
		"write_plan_repair_pack",
	} {
		if !strings.Contains(wf, want) {
			t.Fatalf("change-plan-skill workflow should mention %q; got:\n%s", want, wf)
		}
	}
	for _, want := range []string{"repo_map", "grep", "read_file", "run_tests", "dry_run=true"} {
		if !strings.Contains(wf, want) {
			t.Errorf("batch-local exploration should reference tool %q; got:\n%s", want, wf)
		}
	}
	suggestions := map[string]bool{}
	for _, name := range sk.ToolSuggestions {
		suggestions[name] = true
	}
	if suggestions["exec_command"] {
		t.Fatalf("planner must not expose generic exec_command; use typed dry-run probes instead: %v", sk.ToolSuggestions)
	}
	if !suggestions["run_tests"] {
		t.Fatalf("planner should expose run_tests for dry_run=true probes: %v", sk.ToolSuggestions)
	}
	for _, banned := range []string{
		"PHASE A",
		"CHOOSE EMISSION MODE",
		"plan_change_batch",
		"apply_ready_plan",
		"if the user says",
		"summary contains",
		"rationale contains",
		"parse prose",
	} {
		if strings.Contains(wf, banned) {
			t.Fatalf("change-plan-skill contains unsupported routing smell %q:\n%s", banned, wf)
		}
	}
}

// TestChangePlanSkill_RollingBatchWorkflowGuidance pins the planner-facing
// guidance that broad writes should unfold as bounded batches, not one
// all-at-once ChangePlan. This is prompt teaching only; scheduler hard gates
// remain typed and deterministic.
func TestChangePlanSkill_RollingBatchWorkflowGuidance(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	wf := strings.Join(sk.Workflow, "\n")
	for _, want := range []string{
		"BOUNDED PLAN",
		"smallest useful ChangePlan",
		"applied and verified before the controller chooses another action",
		"acceptance_tests",
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("rolling batch workflow should mention %q; got:\n%s", want, wf)
		}
	}
}

// TestChangePlanSkill_DebugWorkflowOnRetry verifies Module F's
// description of how to read failure data on retry — not as
// prescription ("if X then Y") but as method ("read X, decide which
// side is wrong").
func TestChangePlanSkill_DebugWorkflowOnRetry(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	wf := strings.Join(sk.Workflow, "\n")
	if !strings.Contains(wf, "VERIFY FEEDBACK ON RETRY") {
		t.Fatalf("workflow must include VERIFY FEEDBACK ON RETRY guidance; got:\n%s", wf)
	}
	for _, want := range []string{
		"modify the test",
		"production code",
		"structural wiring/config",
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("retry feedback guidance should mention %q; got:\n%s", want, wf)
		}
	}
	for _, banned := range []string{"Most common cause", "DO NOT raise the cap", "DEBUG WORKFLOW"} {
		if strings.Contains(wf, banned) {
			t.Errorf("retry feedback guidance must not contain prescriptive prose %q", banned)
		}
	}
}

func TestTestExecuteSkill_NoTestsRunnersGuidanceMentionsSurfaceEscalation(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("test-execute-skill")
	if err != nil {
		t.Fatalf("Get(test-execute-skill): %v", err)
	}
	blob := strings.Join(sk.Workflow, "\n")
	for _, want := range []string{
		"NoTestsRunners",
		"run_tests automatically escalates",
		"typed test surface",
		"run_tests owns syntax fallback",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("test execute skill missing %q; got:\n%s", want, blob)
		}
	}
	for _, bad := range []string{
		"The verdict is PASSED — that is a clean run with no test work to do",
		"Just stop.",
		"Always supply `runner`",
		"runner parameter set to your chosen runner",
		"Use the `suite` parameter",
		"inspect the worktree with list_files",
	} {
		if strings.Contains(blob, bad) {
			t.Fatalf("test execute skill still contains stale NoTestsRunners guidance %q; got:\n%s", bad, blob)
		}
	}
}

// TestSkills_NoInternalGoNamesInPrompts pins the audit-2026-04-30
// red line: LLM-facing skill prompts (Workflow / Goal / OutputFormat
// / Prohibitions) must NOT contain Go-internal identifiers like
// `Mutable.ChangePlan` / `ctx.RepoRoot` / `WriteClosure.AppliedSet`
// or system error codes (W1, W1b, V1-V4) that the LLM has no way to
// interpret. Tool names (emit_change_plan, apply_patch, run_tests,
// emit_test_results, emit_plan_skeleton, emit_plan_change) ARE the
// LLM's interface and are explicitly allowed.
func TestSkills_NoInternalGoNamesInPrompts(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	bannedTokens := []string{
		"Mutable.ChangePlan",
		"Mutable.ChangeReport",
		"ctx.RepoRoot",
		"ctx.Mutable",
		"WriteClosure",
		"AppliedSet",
		"DisallowUnknownFields",
		"ChangeUnit", // Go type name
		"answerDocumentEvaluator",
		"plannerEvaluator",
		"explorerEvaluator",
		"verifierEvaluator",
	}
	// V1/V2/V3/V4 + W1/W1b internal validator codes — checked as
	// whole tokens to avoid catching legitimate text like "v1.0"
	// or "Windows Vista 1".
	bannedCodeTokens := []string{
		"V1) ", "V2) ", "V3) ", "V4) ",
		" W1 ", " W1b ", "(W1)", "(W1b)",
	}
	for _, name := range []string{
		"explore-skill",
		"change-plan-skill",
		"code-write-skill",
		"test-execute-skill",
		"answer-document-skill",
	} {
		sk, err := r.Get(name)
		if err != nil {
			continue // skill not always registered
		}
		blob := strings.Join(append(append(append(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), sk.Prohibitions...), sk.ToolSuggestions...), ""), "\n")
		for _, banned := range bannedTokens {
			if strings.Contains(blob, banned) {
				t.Errorf("skill %q must not leak Go-internal identifier %q in LLM-facing prompts", name, banned)
			}
		}
		for _, banned := range bannedCodeTokens {
			if strings.Contains(blob, banned) {
				t.Errorf("skill %q must not leak system error code %q in LLM-facing prompts (use neutral phrasing like \"the validator\")", name, banned)
			}
		}
	}
}

// TestLogTriageSkill_AdvertisesPerformanceSignal pins the
// 2026-05-02 SignalPerformance enum addition into the log-triage
// skill prompt. Without explicitly listing 'performance' in the
// canonical-enum sentence the LLM sees, slow-but-not-cancelled
// patterns (e.g. "slow API call took 5s", "frame skipped",
// "GC pause 800ms") would keep landing in 'timeout'
// (prefix-misuse) or 'other' (information loss) even though the
// JSON schema offers the new value.
//
// Lock both the new value's presence and the original 10 enum
// values so a future edit cannot silently drop guidance.
func TestLogTriageSkill_AdvertisesPerformanceSignal(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("log-triage-skill")
	if err != nil {
		t.Fatalf("log-triage-skill missing: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	if !strings.Contains(blob, "performance") {
		t.Errorf("log-triage-skill must mention 'performance' signal in the canonical-enum sentence")
	}
	for _, must := range []string{"panic", "crash", "oom", "timeout", "permission", "db", "network", "validation", "logic", "other"} {
		if !strings.Contains(blob, must) {
			t.Errorf("log-triage-skill missing original enum value %q after performance addition", must)
		}
	}
}

func TestLogTriageSkill_TeachesOperationalObservations(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("log-triage-skill")
	if err != nil {
		t.Fatalf("log-triage-skill missing: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"observations[]",
		"topic_mismatch",
		"line_mapping",
		"retry_cycle",
		"diagnostic=true",
	} {
		if !strings.Contains(blob, want) {
			t.Errorf("log-triage-skill missing observation guidance %q", want)
		}
	}
}

// TestMultiSourceMarker_LogTriageSkillTeachesIt pins the contract:
// the CLI loadMultiPathSlice + REPL handleLogAppend insert
// `# codrax-source: <path>` separators between concatenated log
// files. The log-triage skill MUST teach the LLM to recognize
// this token so multi-source attribution survives end-to-end.
// A rename of the marker requires updating BOTH the CLI loader
// and this skill prompt simultaneously — this test catches drift.
func TestMultiSourceMarker_LogTriageSkillTeachesIt(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("log-triage-skill")
	if err != nil {
		t.Fatalf("log-triage-skill missing: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	if !strings.Contains(blob, "# codrax-source:") {
		t.Errorf("log-triage-skill must teach the `# codrax-source:` multi-attach marker (CLI/REPL rely on it)")
	}
}

// TestMultiSourceMarker_PerfTriageSkillTeachesIt is the perf-channel
// companion. Both perf-triage-skill (single-shot) and
// perf-segmentation-skill (two-step) must teach the marker.
func TestMultiSourceMarker_PerfTriageSkillTeachesIt(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	for _, name := range []string{"perf-triage-skill", "perf-segmentation-skill"} {
		sk, err := r.Get(name)
		if err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
		blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
		if !strings.Contains(blob, "# codrax-source:") {
			t.Errorf("%s must teach the `# codrax-source:` multi-attach marker", name)
		}
	}
}

func TestRuntimeTriageSkills_DoNotAdvertiseReadFileAsAlwaysAvailable(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	for _, name := range []string{
		"log-triage-skill",
		"perf-triage-skill",
		"log-segmentation-skill",
		"perf-segmentation-skill",
	} {
		t.Run(name, func(t *testing.T) {
			sk, err := r.Get(name)
			if err != nil {
				t.Fatalf("%s missing: %v", name, err)
			}
			staticPrompt := strings.Join([]string{
				sk.Goal,
				strings.Join(sk.Workflow, "\n"),
				sk.OutputFormat,
				strings.Join(sk.Prohibitions, "\n"),
			}, "\n")
			if strings.Contains(staticPrompt, "read_file") {
				t.Fatalf("%s static prompt must not name read_file; pagination availability is projected through ToolSuggestions/tool schema", name)
			}
			if !stringSliceContains(sk.ToolSuggestions, "read_file") {
				t.Fatalf("%s should still list read_file in ToolSuggestions so blob-backed attachments can expose the schema", name)
			}
		})
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
