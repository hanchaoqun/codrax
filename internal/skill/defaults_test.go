package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
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

func TestExploreSkillKeepsDefinitionAndObservedCallAsSeparateTypedFacts(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	workflow := allWorkflowBodies(sk)
	for _, want := range []string{
		"emit that invocation as its own `evidence_kind=relationship`, `anchor_kind=call` item",
		"do not leave the observed edge only inside a definition item's summary",
		"not a requirement that every definition have an incident edge",
		"unsupported edges must never be invented",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("explore relation-authoring guidance missing %q", want)
		}
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
		"projected action enum",
		"explore_code",
		"plan_batch",
		"apply_plan",
		"verify_batch",
		"append_batch",
		"split_batch",
		"replan_batch",
		"finish",
		"block",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("write-controller skill missing %q:\n%s", want, corpus)
		}
	}
	if strings.Contains(corpus, "explore_code, plan_batch, apply_plan, verify_batch") {
		t.Fatalf("write-controller skill must not duplicate the full mode-dependent action enum:\n%s", corpus)
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

func TestChangePlanSkillKeepsVerificationProbeAnOptionalDirectRuntime(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	body := allWorkflowBodies(sk)
	for _, want := range []string{
		"PROBE DECISION FIRST",
		"typed test_surface advertises a native project runner for the changed language/package",
		"local syntax/build repair",
		"the verifier, not the planner, establishes exact changed-path coverage",
		"omit a probe",
		"copy the changed implementation into a standalone program",
		"Never create a probe merely to reread changed source tokens",
		"verification_probes[] are optional source-level programs, not command runners",
		"omit verification_probes[] and put the native build/test command in acceptance_tests[]",
		"never launch an external compiler or test runner from a supported-language wrapper",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("change-plan-skill missing probe authoring boundary %q:\n%s", want, body)
		}
	}
	for _, want := range []string{
		"MUTATIONS ONLY",
		"changes[] contains files whose bytes, path, or existence will actually change",
		"belongs in summary/acceptance_tests[]",
		"never in changes[] as an empty patch, no-op edit, or placeholder file",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("change-plan-skill missing mutation-only boundary %q:\n%s", want, body)
		}
	}
	for _, fixtureSpecific := range []string{
		"WORKED EXAMPLE — fixing typo",
		"retrun fmt.Sprintf",
		"func greet(name string)",
	} {
		if strings.Contains(body, fixtureSpecific) {
			t.Fatalf("change-plan-skill must not teach an eval-shaped patch fixture %q", fixtureSpecific)
		}
	}
}

func TestChangePlanSkillFrontLoadsCanonicalJSONShapeFirstTeaching(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	if len(sk.Workflow) == 0 || sk.Workflow[0] != types.ChangePlanJSONShapeFirstTeaching {
		t.Fatalf("canonical ChangePlan JSON-shape teaching must be the first workflow decision, got: %+v", sk.Workflow)
	}
	joined := strings.Join(sk.Workflow, "\n")
	if strings.Count(joined, types.ChangePlanJSONShapeFirstTeaching) != 1 {
		t.Fatalf("canonical ChangePlan JSON-shape teaching must appear exactly once, got %d", strings.Count(joined, types.ChangePlanJSONShapeFirstTeaching))
	}
	for _, want := range []string{
		"Probe ids are unique across the whole plan",
		"place each logical probe in exactly one carrier",
		"never duplicate the same id/payload across both",
	} {
		if !strings.Contains(sk.Workflow[0], want) {
			t.Fatalf("canonical ChangePlan JSON teaching missing probe-carrier rule %q: %s", want, sk.Workflow[0])
		}
	}
}

func TestChangePlanSkillScopesEmptyChangesProhibitionToTypedExceptions(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	prohibitions := allProhibitionBodies(sk)
	for _, want := range []string{
		"ordinary source-change plan",
		"scheduler-labelled verification_proof_followup",
		"changes: [] together with verification_probes[]",
		"typed passing-probe no-change sentinel",
		"never infer either exception from prose",
	} {
		if !strings.Contains(prohibitions, want) {
			t.Fatalf("empty-changes boundary must state the typed exception %q:\n%s", want, prohibitions)
		}
	}
	if strings.Contains(prohibitions, "do not write a plan whose changes[] array is empty") {
		t.Fatalf("absolute empty-changes prohibition contradicts typed proof/no-change lanes:\n%s", prohibitions)
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

func TestWriteAnalysisSkillTeachesOneGroundingLaneBeforeExactContracts(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("write-analysis-skill")
	if err != nil {
		t.Fatalf("Get(write-analysis-skill) returned error: %v", err)
	}
	corpus := allWorkflowBodies(sk)
	for _, want := range []string{
		"Choose exactly one grounding lane",
		"appears verbatim in raw_request",
		"contract/placement evidence_ref",
		"do not invent an exact command result, output, status, or exception",
		"Omit behavior_contracts[] when expected_outcomes[] already states the goal",
		"otherwise use operator=satisfies only for essential soft behavior",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("write-analysis exact-contract decision teaching missing %q:\n%s", want, corpus)
		}
	}
}

func TestWriteAnalysisSkillPinsNativeJSONCarriersBeforeSemanticGuidance(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("write-analysis-skill")
	if err != nil {
		t.Fatalf("Get(write-analysis-skill): %v", err)
	}
	if len(sk.Workflow) == 0 || sk.Workflow[0] != types.WriteAnalysisJSONShapeFirstTeaching {
		t.Fatalf("write-analysis JSON carrier rule must be the first workflow instruction: %+v", sk.Workflow)
	}
	surface := sk.Goal + "\n" + sk.OutputFormat + "\n" + allWorkflowBodies(sk)
	if got := strings.Count(surface, types.WriteAnalysisJSONShapeFirstTeaching); got != 1 {
		t.Fatalf("write-analysis JSON carrier rule must appear exactly once in the skill prompt, got %d", got)
	}
	for _, want := range []string{
		"behavior_contracts[]",
		"native JSON arrays",
		"never quoted or escaped JSON strings",
		"do not delete a field merely to make decoding pass",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("write-analysis JSON teaching missing %q:\n%s", want, surface)
		}
	}
}

func TestWriteAnalysisSkillCalibratesMutationRiskWithoutWeakeningApproval(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("write-analysis-skill")
	if err != nil {
		t.Fatalf("Get(write-analysis-skill) returned error: %v", err)
	}
	corpus := strings.ToLower(sk.Goal + "\n" + sk.OutputFormat + "\n" + allWorkflowBodies(sk) + "\n" + allProhibitionBodies(sk))
	for _, want := range []string{
		"mutation's blast radius",
		"not the severity of the pre-existing defect",
		"package-local bugfix",
		"preserves public signatures",
		"reserve high for genuinely broad or high-impact mutation surfaces",
		"never overrides the deterministic approval gate",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("write-analysis risk calibration missing %q:\n%s", want, corpus)
		}
	}
}

func TestWriteSkillsCarryStateTransitionsWithoutProseHardGate(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	analysis, err := r.Get("write-analysis-skill")
	if err != nil {
		t.Fatalf("Get(write-analysis-skill): %v", err)
	}
	planner, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	analysisText := allWorkflowBodies(analysis) + "\n" + analysis.OutputFormat
	for _, want := range []string{
		"STATE-TRANSITION CONTRACTS",
		"behavior_contracts[].transition.steps[]",
		"setup, action, observation, and postcondition",
		"non-initial-state sequence",
		"cross-operation sequence",
		"soft context for planning and verification",
	} {
		if !strings.Contains(analysisText, want) {
			t.Fatalf("write-analysis state-transition guidance missing %q:\n%s", want, analysisText)
		}
	}
	plannerText := allWorkflowBodies(planner)
	for _, want := range []string{
		"STATEFUL VERIFICATION",
		"transition.steps[]",
		"execute those steps in order",
		"Do not replace the sequence with source-token checks",
		"non-initial state",
		"only executed probe/project-runner results own verification authority",
	} {
		if !strings.Contains(plannerText, want) {
			t.Fatalf("planner state-transition guidance missing %q:\n%s", want, plannerText)
		}
	}
	for _, text := range []string{strings.ToLower(analysisText), strings.ToLower(plannerText)} {
		for _, banned := range []string{"if the user says", "summary contains", "rationale contains", "parse prose"} {
			if strings.Contains(text, banned) {
				t.Fatalf("state-transition guidance contains prose-routing smell %q", banned)
			}
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
		"scoped targeted source grep result that prints the exact production match",
		"Never emit from a filename-only or relationship-map row",
		"verified navigation facts",
		"not semantic source-code citations",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("explore-skill missing repo lens guidance %q:\n%s", want, corpus)
		}
	}
	for _, forbidden := range []string{
		"do not treat repo_map output as evidence",
		"Do not emit line-scope evidence from repo_map/grep navigation output alone",
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
	if len(sk.WorkflowTierB) < 5 {
		t.Fatalf("explore-skill should carry trace guidance in WorkflowTierB: %#v", sk.WorkflowTierB)
	}
	// WO-P1 (SMR-1 批, 2026-07-12): the IO type-word single-source directive
	// joined the trace tier at index 3 (before PERF SAMPLE PROVENANCE).
	traceTier := strings.Join([]string{sk.WorkflowTierB[0].Body, sk.WorkflowTierB[1].Body, sk.WorkflowTierB[2].Body, sk.WorkflowTierB[3].Body, sk.WorkflowTierB[4].Body}, "\n")
	for i := 0; i < 5; i++ {
		if !sk.WorkflowTierB[i].AppliesTo.RequiresTrace {
			t.Fatalf("trace workflow item %d must be gated by RequiresTrace: %+v", i, sk.WorkflowTierB[i].AppliesTo)
		}
	}
	for _, want := range []string{
		"RUNTIME TRACE FIRST",
		"start with `trace_query`",
		"TRACE QUERY:",
		".ftrace",
		"parsed_events=0",
		"1s 501ms 565μs 915ns",
		"pattern=\"<literal>\"",
		"not regex",
		"mixed trace+source",
		"TRACE SEMANTIC SPAN ROOT CAUSES",
		"semantic span-work",
		// DCS E6/F3a (ledger §23.1 ruling ③): the double-gated mention
		// obligation — on-chain tier word always mentioned as an optimization
		// point with its window share; non-chain only at background_rank<=3.
		//
		// EVOLUTION RECORD (RCM §24.7.1/§24.10, 2026-07-08,
		// real_trace_campaign_20260705.md §24.12): the obligation moved to
		// FAMILY caliber — same-thread spans of one semantic class arrive as
		// one merged rank row whose projected_impact_ms is the combined
		// window-projection total, the mention carries the merged count +
		// largest member name, and the share denominator is the row's OWN
		// query window (DCS E5 lane). Pin evolved: "projected share of the
		// analysis window" → "projected share of its own query window", plus
		// the family-field vocabulary below.
		// EVOLUTION RECORD (SEM-LEAD §29.7-2 ⑤, 2026-07-10,
		// real_trace_campaign_20260705.md): the "never as the root cause"
		// pin is RETIRED — on-chain semantic rows now compete for (and may
		// be named) the root cause; the pin set asserts the new equal-footing
		// clause + the unconditional mention floor + the class-word naming
		// rule, and the negative pin below keeps the retired ban out.
		"`gc_pause`",
		"ordinary primary/secondary/tertiary root-cause election",
		"MUST NEVER enter the Background board",
		"compete for the root cause on equal footing",
		"name it as the root cause within the selected-window chain ranking by its semantic class",
		"it does not by itself prove that the row caused a particular dropped frame or missed deadline",
		"`causal_conclusion=unproven` or `frame_evidence_status=absent`",
		"strongest selected-window bottleneck / chain candidate",
		"never one member's span name",
		"MUST mention EVERY retained on-chain semantic family",
		"Independently of root-cause TOP N",
		"never omitted merely because their rank row was truncated",
		"projected share of its own query window",
		"member_count",
		"member_roster",
		"combined window-projection total",
		"background_rank<=3",
		// WO-P1 (SMR-1 批 S9-AWEME): the IO type-word single-source soft
		// directive (答案侧一致性走 eval 观察, 本 pin 只保指令在场).
		"TRACE IO TYPE-WORD SINGLE SOURCE",
		"ONE IO type word per physical IO episode",
		"lead with io_wait when present, else io_latency, else io_burst_episode",
		"never as additional causes and never added together",
		"PERF SAMPLE PROVENANCE",
		// 修补轮 件1 (值词库教学批, 2026-07-17): the projection-key sentence
		// speaks the honest geometry — projected_total_ms is a wakeup_chain
		// key, the rank-note spelling is a cumulative echo, and ranking order
		// lives on effective_impact_ms (same word family as the trace_query
		// Description C3② sentence).
		"a rank observation note spelling projected_total_ms only echoes cumulative_impact_ms",
		"ranking order lives on effective_impact_ms (before score)",
		"temporal overlap, the same broad IO/CPU class, or a pressure score does NOT prove a shared device/inode/lock/core",
		"independent background and at most a follow-up investigation direction",
		"never add its duration to chain rows or invent a combined total",
	} {
		if !strings.Contains(traceTier, want) {
			t.Fatalf("trace-gated workflow missing %q:\n%s", want, traceTier)
		}
	}
	// 修补轮 件1 negative pin: the retired phantom-pair-for-ranking claim must
	// not resurface on any explore-skill teaching face.
	if strings.Contains(allWorkflowBodies(sk), "projected_impact_ms/projected_total_ms are the selected-window or target-blocking projection used for ranking") {
		t.Fatalf("retired phantom projection-pair ranking claim resurfaced:\n%s", allWorkflowBodies(sk))
	}
	// SEM-LEAD §29.7-2 ⑤ negative pin: the retired ban wording must not
	// resurface — "never as the root cause" contradicted the ruling's
	// equal-footing crown lane (the mention floor stays as its own clause).
	if strings.Contains(traceTier, "never as the root cause") {
		t.Fatalf("retired semantic-span root-cause ban resurfaced in the trace tier:\n%s", traceTier)
	}
	if strings.Contains(traceTier, "tier=deterministic_optimization") || strings.Contains(traceTier, "largest on-chain one") {
		t.Fatalf("retired semantic-span tier/mention contract resurfaced in the trace tier:\n%s", traceTier)
	}
}

// TestExploreSkill_TraceValueWordsDirective — C1 值词库教学批 (§29.104.16.1
// M5①, 2026-07-17): the wire-token ↔ display-word bridge directive is present,
// trace-gated, and carries the closed mapping (effective_impact_ms → 有效归因
// single canonical zh word, gated components → 全额/折算 display words,
// member_fold_caliber values → family caliber words) plus the negative
// teaching (bare wire tokens never become prose vocabulary; no self-minted
// 「直达」/「有效影响」). Display-word ↔ single-source lockstep is pinned in
// internal/tool (TestValueWordWireMappingLockstep); this pin keeps the
// directive itself in place. Soft guidance only — no hard gate reads it.
func TestExploreSkill_TraceValueWordsDirective(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	if strings.Contains(strings.Join(sk.Workflow, "\n"), "TRACE VALUE WORDS") {
		t.Fatalf("value-word guidance must not live in the always-rendered workflow:\n%v", sk.Workflow)
	}
	var item TierBItem
	found := 0
	for _, candidate := range sk.WorkflowTierB {
		if strings.HasPrefix(candidate.Body, "TRACE VALUE WORDS:") {
			item = candidate
			found++
		}
	}
	if found != 1 {
		t.Fatalf("explore-skill must carry exactly one TRACE VALUE WORDS directive, got %d", found)
	}
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("TRACE VALUE WORDS must be gated by RequiresTrace: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"never write bare `gated_runnable` or `sum_disjoint` as prose vocabulary",
		"`effective_impact_ms`",
		"「有效归因」",
		"never coin substitute words such as 「有效影响」 or 「直达」",
		"one measurement under several names, not mutual corroboration",
		"「runnable(全额)」",
		"「running(折算)」",
		"`member_fold_caliber`",
		"「合计(共N段,同线程)」",
		"「成员最大(共N段,重叠未拆)」",
		"「计数合计(共N项,同线程)」",
		"an honest lower bound, never a sum",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("TRACE VALUE WORDS directive missing %q:\n%s", want, item.Body)
		}
	}
}

// TestExploreSkill_TraceComparisonGuidanceIsComparisonGated — CMP-6 pin:
// the cross-trace comparison directive (per-trace span anchoring + aligned
// windows + window-length normalization) exists exactly once, is gated by the
// typed comparison form (RequiresTraceComparison), and never leaks into the
// always-rendered workflow or the plain RequiresTrace tier.
//
// EVOLUTION RECORD (§21 SG-2b, real_trace_campaign_20260705.md, cmp_01 audit
// dim A③(a), 2026-07-07): the pin now also asserts the same-caliber
// causal-drilldown clauses — the reliable sequence continues into
// `wakeup_chain` on `state_drilldown` chain_required=true, and a causal
// drilldown view run on one trace must be mirrored with the same view and
// parameters over the other trace's own span-aligned window before any
// comparative root-cause conclusion. Soft guidance only; the typed
// RequiresTraceComparison gate is unchanged.
func TestExploreSkill_TraceComparisonGuidanceIsComparisonGated(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	if strings.Contains(strings.Join(sk.Workflow, "\n"), "TRACE COMPARISON") {
		t.Fatalf("comparison guidance must not live in the always-rendered workflow:\n%v", sk.Workflow)
	}
	var comparison []TierBItem
	for _, item := range sk.WorkflowTierB {
		if item.AppliesTo.RequiresTraceComparison {
			comparison = append(comparison, item)
		}
	}
	if len(comparison) != 1 {
		t.Fatalf("explore-skill must carry exactly one comparison-gated workflow item, got %d", len(comparison))
	}
	item := comparison[0]
	if item.AppliesTo.RequiresTrace {
		t.Fatalf("the comparison item is gated by the comparison form alone (which already implies traces): %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"TRACE COMPARISON",
		"PER TRACE",
		"event_search",
		"span_window",
		"tid/pid",
		"start/end timestamps",
		"span-aligned window",
		"dividing each side's value by its own window length",
		"normalized densities",
		// §21 SG-2b: same-caliber causal-drilldown clauses.
		"`state_drilldown` row that reports `chain_required=true` with `wakeup_chain`",
		"Causal drilldown must stay same-caliber",
		"`wakeup_chain` or `critical_blocking_calls`",
		"SAME drilldown view with the SAME parameters over the other trace's own span-aligned window",
		"sampling gap, not evidence",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("comparison directive missing %q:\n%s", want, item.Body)
		}
	}
	// The directive must not render on a plain single-trace dispatch and must
	// render on the comparison form (ShouldRender wiring pin).
	if item.ShouldRender(AppliesToContext{HasTrace: true}) {
		t.Fatalf("single-trace dispatch must not admit the comparison directive")
	}
	if !item.ShouldRender(AppliesToContext{HasTrace: true, HasTraceComparison: true}) {
		t.Fatalf("comparison form must admit the comparison directive")
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

func TestExploreSkill_CallChainFrontierHandoffSeparatesSiblingAndReverseEdges(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	corpus := allWorkflowBodies(sk)
	for _, want := range []string{
		"CALL-CHAIN FRONTIER HANDOFF",
		"sibling-edge SET",
		"exact caller, callee, and callsite",
		"typed request carries a completeness obligation",
		"entry/early-return guards",
		"separate `conditional` + `condition` rows",
		"as its own `relationship` + `call` row",
		"Never fold a guard and its guarded call into one item",
		"language-neutral",
		"ArkTS",
		"Cangjie",
		"reverse or parallel direct call",
		"a definition row or waiver rationale cannot substitute for the edge",
		"bounded semantic descent, not exhaustive traversal",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("explore-skill call-chain frontier guidance missing %q:\n%s", want, corpus)
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

func TestFinalizerSkillDiagramBodyTeachingMatchesProjectedSchemaAndRenderer(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	for _, want := range []string{
		"`diagram.body` is the RAW Mermaid source only",
		"Do NOT add opening/closing Markdown fences",
		"`subgraph ... end` grouping is accepted",
		"legacy fallback where Mermaid is embedded directly inside a summary/section `text` field",
	} {
		if !strings.Contains(sk.OutputFormat, want) {
			t.Fatalf("answer-document-skill diagram teaching missing %q:\n%s", want, sk.OutputFormat)
		}
	}
	for _, stale := range []string{
		"The fence itself is REQUIRED",
		"`subgraph` nesting construct are NOT in the rendering subset",
	} {
		if strings.Contains(sk.OutputFormat, stale) {
			t.Fatalf("answer-document-skill retained contradictory diagram teaching %q", stale)
		}
	}
}

func TestFinalizerSkillPatchTeachingUsesCanonicalFourOperationSemantics(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	if count := strings.Count(sk.OutputFormat, types.AnswerDocumentPatchOperationTeaching); count != 1 {
		t.Fatalf("canonical patch operation teaching count=%d, want 1", count)
	}
	for _, want := range []string{
		"`remove_block_ids`",
		"omitting a previous block id from all four operations does not delete it",
		"never wrap an array or object payload in a JSON string",
	} {
		if !strings.Contains(sk.OutputFormat, want) {
			t.Fatalf("answer-document patch teaching missing %q", want)
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

func TestFinalizerSkillUsesProjectedSchemaAsSingleJSONFieldAuthority(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	blob := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"the only authority for JSON field names, value types, required fields, and allowed enum values",
		"content-placement guide, not a second JSON schema",
		"one complete structured tool call per attempt",
		"rejected attempts may be corrected",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("answer-document-skill missing schema-ownership guidance %q:\n%s", want, blob)
		}
	}
	for _, duplicateCarrierTeaching := range []string{
		"native JSON array",
		"JSON-encoded string with escaped quotes",
		"do NOT quote it as a string containing escaped JSON",
	} {
		if strings.Contains(blob, duplicateCarrierTeaching) {
			t.Fatalf("static skill must defer JSON carrier shape to the projected tool schema instead of repeating %q:\n%s", duplicateCarrierTeaching, blob)
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
		types.GroundedSourceDiagramEdgeOwnershipContract,
		types.GroundedSourceDiagramRelationEvidenceContract,
		"Flow/architecture edges also need their honest relation owner",
		"relation_kind: <one of " + BuildDiagramRelationKindList() + ">",
		"`type_relation`, `register`, `callback`, `assignment`, `data_flow`, and `return` are typed-only",
		"PREFERRED: declare the relation directly",
		"the sole typed relation authority",
		"dashed reply `callee-->>caller` is a response/return, not a reverse call",
		"exact message operation resolves to one unique typed call edge",
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
		"Evidence-entailment boundary",
		"call-site citation authorizes only caller -> callee",
		"Never widen one evidence item's free-form summary",
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

// TestFinalizerSkill_TraceProseDisciplinesAreTraceGated — SG 批 (账本
// real_trace_campaign_20260705.md §10.2/§11.2/§12.2): three trace-only prose
// disciplines live in the answer-document skill's WorkflowTierB, each gated by
// RequiresTrace, never in the always-rendered workflow:
//   - PERIODIC-SOURCE DISCOUNT (§10-C4): prose must consume the discounted
//     effective_impact_ms, raw sum only as a labelled comparison figure;
//   - ON-CHAIN BLOCKING DISPOSITION (Q4-K 修4): a near-target-length on-chain
//     blocking observation must be explicitly disposed of in prose;
//   - WINDOW-STATS CORE NUMBERS (§11-N7 soft face): headline window-stats
//     numbers carry their measurement-basis window.
func TestFinalizerSkill_TraceProseDisciplinesAreTraceGated(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	always := strings.Join(sk.Workflow, "\n")
	for _, header := range []string{"PERIODIC-SOURCE DISCOUNT", "ON-CHAIN BLOCKING DISPOSITION", "WINDOW-STATS CORE NUMBERS"} {
		if strings.Contains(always, header) {
			t.Fatalf("%s must not live in the always-rendered workflow", header)
		}
	}
	find := func(header string) TierBItem {
		t.Helper()
		for _, item := range sk.WorkflowTierB {
			if strings.Contains(item.Body, header) {
				return item
			}
		}
		t.Fatalf("answer-document-skill WorkflowTierB missing %q", header)
		return TierBItem{}
	}
	discount := find("PERIODIC-SOURCE DISCOUNT")
	if !discount.AppliesTo.RequiresTrace {
		t.Fatalf("periodic-source discount item must be RequiresTrace-gated: %+v", discount.AppliesTo)
	}
	for _, want := range []string{"periodic_source=true", "effective_impact_ms", "discounted value", "comparison figure", "NEVER as the primary impact number"} {
		if !strings.Contains(discount.Body, want) {
			t.Errorf("periodic-source discount item missing %q:\n%s", want, discount.Body)
		}
	}
	disposition := find("ON-CHAIN BLOCKING DISPOSITION")
	if !disposition.AppliesTo.RequiresTrace {
		t.Fatalf("blocking-disposition item must be RequiresTrace-gated: %+v", disposition.AppliesTo)
	}
	for _, want := range []string{"chain_relevance=on_chain", "same order of magnitude", "root-cause carrier", "subordinate"} {
		if !strings.Contains(disposition.Body, want) {
			t.Errorf("blocking-disposition item missing %q:\n%s", want, disposition.Body)
		}
	}
	windowStats := find("WINDOW-STATS CORE NUMBERS")
	if !windowStats.AppliesTo.RequiresTrace {
		t.Fatalf("window-stats basis item must be RequiresTrace-gated: %+v", windowStats.AppliesTo)
	}
	for _, want := range []string{"measurement basis", "aligned/occurrence window", "denominator"} {
		if !strings.Contains(windowStats.Body, want) {
			t.Errorf("window-stats basis item missing %q:\n%s", want, windowStats.Body)
		}
	}
}

// TestAnalysisSkill_RuntimeFocusIdentityGuidance — SG 批 Q4-E 腿1 (soft
// entity-kind steering): the classification prompt distinguishes thread/
// process identities (analysis subjects → entities + typed runtime_targets
// with kind) from frame/sequence ordinals (event locators — never
// runtime_targets, never the anchor subject).
func TestAnalysisSkill_RuntimeFocusIdentityGuidance(t *testing.T) {
	cfg := BuildAnalysisSkill()
	if cfg == nil {
		t.Fatal("BuildAnalysisSkill returned nil")
	}
	// The focus-identities teaching spans two prompt faces: the OutputFormat
	// paragraph and the Workflow rules (fast-path reminder + emission field
	// checklist), so the pin scans their join.
	out := strings.Join(append([]string{cfg.Goal, cfg.OutputFormat}, cfg.Workflow...), "\n")
	for _, want := range []string{
		"Runtime-artifact focus identities",
		"`runtime_target_profile.declaration=named_target`",
		"A named_target declaration without runtime_targets is rejected",
		"EVENT LOCATORS",
		"not focus subjects",
		// SUPP-TARGET (§29.90.1, 2026-07-15) prompt-face teaching: the
		// classifier variant that copied the thread identity into entities
		// but skipped the typed lane (h2 20260714-221545) traced to the
		// runtime_targets lane being absent from BOTH the emission field
		// checklist and the no-pre-scan trace fast path. The two reminders
		// below are the fix; dropping either re-opens the leak.
		"the Runtime-artifact focus identities rule applies on this no-pre-scan path too",
		"runtime_targets (required when runtime_target_profile.declaration=named_target",
		"Runtime question scope (REQUIRED)",
		"`runtime_question_profile`",
		"`bounded_fact_set`",
		"do not relabel it as causal diagnosis merely because it asks for the recorded `reason`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("classification prompt missing runtime focus-identity guidance token %q", want)
		}
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

func TestChangePlanSkillGeneratedArtifactVerificationUsesArtifactBoundary(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("change-plan-skill")
	if err != nil {
		t.Fatalf("Get(change-plan-skill): %v", err)
	}
	wf := allWorkflowBodies(sk)
	for _, want := range []string{
		"verify the produced artifact",
		"generated artifact's own lexical/runtime scope",
		"cannot prove that the generated output parses, resolves names, executes, or preserves behavior",
		"renders/builds the artifact",
		"generated-output parser/scope check",
	} {
		if !strings.Contains(wf, want) {
			t.Fatalf("generated-artifact verification guidance missing %q:\n%s", want, wf)
		}
	}
	if strings.Count(wf, generatedArtifactVerificationDirective) != 1 {
		t.Fatalf("generated-artifact directive must appear exactly once")
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

func TestLogTriageSkillUsesCanonicalJSONShapeFirstTeaching(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("log-triage-skill")
	if err != nil {
		t.Fatalf("log-triage-skill missing: %v", err)
	}
	if !strings.Contains(sk.OutputFormat, types.LogTriageJSONShapeFirstTeaching) {
		t.Fatalf("log-triage output format must lead with the canonical JSON-shape teaching:\n%s", sk.OutputFormat)
	}
	if strings.Count(sk.OutputFormat, types.LogTriageJSONShapeFirstTeaching) != 1 {
		t.Fatalf("canonical JSON-shape teaching must appear exactly once, got %d", strings.Count(sk.OutputFormat, types.LogTriageJSONShapeFirstTeaching))
	}
	joined := strings.Join(append([]string{sk.OutputFormat}, sk.Workflow...), "\n")
	for _, want := range []string{
		"cause_relation={authority:'explicit_artifact_marker'",
		"Similar error text, adjacent timestamps",
		"omit both fields for peer/adjacent errors",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("log-triage cause authority teaching missing %q:\n%s", want, joined)
		}
	}
}

func TestAnswerDocumentSkillFrontLoadsProjectedSchemaOwnership(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("answer-document-skill missing: %v", err)
	}
	if len(sk.Workflow) == 0 || !strings.Contains(sk.Workflow[0], "projected `emit_answer_document` tool schema") {
		t.Fatalf("projected schema ownership must be the first workflow decision, got: %+v", sk.Workflow)
	}
	joined := strings.Join(append([]string{sk.Goal, sk.OutputFormat}, sk.Workflow...), "\n")
	if strings.Contains(joined, types.AnswerDocumentJSONShapeFirstTeaching) {
		t.Fatal("static skill must not duplicate the schema-near canonical carrier teaching")
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
		"short evidence excerpt copied VERBATIM from the attached log",
		"Omit evidence when no short exact excerpt exists",
	} {
		if !strings.Contains(blob, want) {
			t.Errorf("log-triage-skill missing observation guidance %q", want)
		}
	}
}

func TestLogTriageSkillSeparatesExplicitErrorsFromConcurrentSnapshots(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("log-triage-skill")
	if err != nil {
		t.Fatalf("log-triage-skill missing: %v", err)
	}
	corpus := sk.Goal + "\n" + sk.OutputFormat + "\n" + allWorkflowBodies(sk) + "\n" + allProhibitionBodies(sk)
	for _, want := range []string{
		"explicit error occurrence",
		"has no error/exception header of its own is NOT a sibling error",
		"observations[] thread_snapshot",
		"does not prove that the thread crashed",
		"copied VERBATIM from the attached log",
	} {
		if !strings.Contains(corpus, want) {
			t.Fatalf("log-triage snapshot authority guidance missing %q:\n%s", want, corpus)
		}
	}
	if strings.Contains(corpus, "one entry per logical error (per goroutine in a Go panic dump") {
		t.Fatalf("legacy per-goroutine error fanout guidance survived:\n%s", corpus)
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

// EVOLUTION RECORD (§29.25 裁定① 2026-07-10, real_trace_campaign_20260705.md):
// this stale-ban pin (requires 41-159/>159, forbids 41-139/>139) locked in the
// 5d91b433 boundary flip before any witness existed. The user ruling
// confirmed 41-159 with production witnesses —
// customlogs/format_census_berlin.txt (prio 142×756604 / 157×3170 / 159×140 /
// 140×3212, prio>139 total 763186), customlogs/format_census.txt (VerifyClass
// record_trace_20260606: 157×36 / 140×21), customlogs/cust_trace_vc_710.txt
// (prio=53→ohos_rt production verdict) — so the pin now stands on a recorded
// domain fact. If the boundary is ever re-litigated, update the ledger ruling
// FIRST, then evolve this pin with a new EVOLUTION RECORD.
func TestRuntimeTraceSkillsPinHarmonyMicrokernelPriorityBoundary(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	explore, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("explore-skill missing: %v", err)
	}
	perf, err := r.Get("perf-triage-skill")
	if err != nil {
		t.Fatalf("perf-triage-skill missing: %v", err)
	}
	corpora := []struct {
		name  string
		body  string
		wants []string
	}{
		{name: "explore", body: allWorkflowBodies(explore), wants: []string{
			"41-159", ">159", "system_or_kernel/raw", "HarmonyOS/hitrace means larger numeric priority is higher",
		}},
		{name: "perf_triage", body: strings.Join(perf.Workflow, "\n"), wants: []string{
			"41-159", ">159", "system_or_kernel/raw", "prio=140", "prio=159", "prio=160", "prio=301",
		}},
	}
	for _, corpus := range corpora {
		for _, want := range corpus.wants {
			if !strings.Contains(corpus.body, want) {
				t.Fatalf("%s priority guidance missing %q", corpus.name, want)
			}
		}
		for _, stale := range []string{"41-139", ">139"} {
			if strings.Contains(corpus.body, stale) {
				t.Fatalf("%s priority guidance retained stale boundary %q", corpus.name, stale)
			}
		}
	}
}

func TestRuntimeTraceSkillsKeepPriorityInversionBroadAndEvidenceCalibrated(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	explore, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("explore-skill missing: %v", err)
	}
	perf, err := r.Get("perf-triage-skill")
	if err != nil {
		t.Fatalf("perf-triage-skill missing: %v", err)
	}
	for _, corpus := range []struct {
		name string
		body string
	}{
		{name: "explore", body: allWorkflowBodies(explore)},
		{name: "perf_triage", body: strings.Join(perf.Workflow, "\n")},
	} {
		for _, want := range []string{
			"lower_priority_waker", "lower_priority_dependency",
			"effective_impact_ms", "cross-CPU weak-core/compute-supply running deficit",
			"runnable time in full", "same-CPU preemption",
		} {
			if !strings.Contains(corpus.body, want) {
				t.Fatalf("%s inversion guidance missing %q", corpus.name, want)
			}
		}
	}
}

func TestPerfTriageSkillKeepsModelObservationsAdvisory(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	perf, err := r.Get("perf-triage-skill")
	if err != nil {
		t.Fatalf("perf-triage-skill missing: %v", err)
	}
	body := strings.Join(perf.Workflow, "\n")
	for _, want := range []string{
		"model-extracted navigation candidates, not causal proof",
		"Keep the measured slow interval separate",
		"model-extracted navigation facts",
		"deterministic trace_query results remain authoritative",
		"Do not turn runnable time",
		"context-switch overhead",
		"frame-budget extrapolation",
		"S, D, or io_wait is not occupying a CPU",
		"typed wakeup/IPC/lock/flow/dependency edge",
		"relationship unproven",
		"t_sleep→t_wake is sleep/blocking until wake",
		"t_wake→t_run is runnable scheduling delay",
		"Never call the pre-wakeup sleep interval or total non-running interval wakeup latency",
		"never call t_run the wakeup timestamp",
		"`call_semantics=reply`",
		"source thread is sending a reply",
		"`call_semantics=sync_request`",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("perf-triage authority guidance missing %q", want)
		}
	}
	for _, internalJargon := range []string{
		"The validator stamps",
		"downstream must",
		"downstream agents",
	} {
		if strings.Contains(body, internalJargon) {
			t.Fatalf("perf-triage task guidance leaked pipeline jargon %q", internalJargon)
		}
	}
}

func TestPerfTriageDoesNotTeachDefault60HzJankAuthority(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	perf, err := r.Get("perf-triage-skill")
	if err != nil {
		t.Fatalf("perf-triage-skill missing: %v", err)
	}
	body := perf.Goal + "\n" + strings.Join(perf.Workflow, "\n")
	for _, want := range []string{
		"Frame verdict authority",
		"current emit_perf_trace payload has no validator-owned refresh-rate/frame-deadline carrier",
		"keep `janky` false/omitted and omit janks[]",
		"still leave the verdict unproven here",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("perf triage frame authority teaching missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"60-fps frame budget", "16.67 ms", "exceeds ~16.6 ms"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("perf triage still teaches an authority-free default threshold %q:\n%s", forbidden, body)
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

// TestAnswerSkillInferredAttributionCoversNsSpanDerivation pins the LCK-2
// extension of the SG-A2 INFERRED ATTRIBUTION DISCLOSURE item (§18.E/§18.E.1,
// deferred skill half delivered 2026-07-07): the ns-span-derivation source
// lane and both LCK-2 note keys must be taught, the process-level arm must
// forbid thread-specific attribution, the identity-unification note must
// upgrade to a cross-corroborated claim that still cites both lanes, and the
// original wakeup-edge sentence stays verbatim.
func TestAnswerSkillInferredAttributionCoversNsSpanDerivation(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	var body string
	for _, item := range sk.WorkflowTierB {
		if strings.HasPrefix(item.Body, "INFERRED ATTRIBUTION DISCLOSURE:") {
			body = item.Body
			if !item.AppliesTo.RequiresTrace {
				t.Fatalf("SG-A2 item must stay RequiresTrace-gated; got %+v", item.AppliesTo)
			}
		}
	}
	if body == "" {
		t.Fatalf("answer-document-skill lost the INFERRED ATTRIBUTION DISCLOSURE Tier B item")
	}
	for _, want := range []string{
		// Original wakeup-edge sentence stays verbatim (no regression).
		"rows carrying holder_source=wakeup_edge or peer_source=wakeup_edge, or flagged presumptive",
		"presumed from the wakeup edge, not directly observed",
		// LCK-2 ns-span-derivation lane.
		"holder_source=ns_span_derivation",
		"trace_mark emission pairs",
		"thread-level, or process-level when a holder_host_process note is present",
		"attribute to the process, never to a specific thread",
		"not read directly from the payload",
		// Identity-unification upgrade.
		"holder_ns_unification note upgrades the claim",
		"cross-corroborated fact while still citing both lanes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SG-A2 body missing %q:\n%s", want, body)
		}
	}
}

// TestWriteExecutionSkillsKeepExecCommandL6 is the enforcing structural pin
// for the CLAUDE.md L6 red line (audit #7, campaign ledger §29.26 审计总收账,
// 2026-07-11): the write EXECUTION skills — code-write-skill and
// test-execute-skill — MUST keep "exec_command" in ToolSuggestions, because
// the worktree sandbox contains the blast radius and the coder/verifier need
// generic shell probes (verify a file wrote, git status, ls). Before this pin
// an edit dropping exec_command compiled and passed the entire suite silently.
//
// Scope note: the planner (change-plan-skill) is deliberately NOT in this
// set — it uses typed dry-run probes instead, and its NEGATIVE pin lives in
// TestChangePlanSkill_BatchLocalPlanningWorkflow ("planner must not expose
// generic exec_command"). Both directions together are the L6 contract.
func TestWriteExecutionSkillsKeepExecCommandL6(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	for _, name := range []string{"code-write-skill", "test-execute-skill"} {
		sk, err := r.Get(name)
		if err != nil {
			t.Fatalf("Get(%s): %v", name, err)
		}
		found := false
		for _, tool := range sk.ToolSuggestions {
			if tool == "exec_command" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("L6 red line: %s must keep exec_command in ToolSuggestions (worktree contains blast radius); got %v", name, sk.ToolSuggestions)
		}
	}
}
