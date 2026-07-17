package orchestrator

// TOOLWIN-FIX pins, orchestrator side (§29.118 / §29.104.20,
// real_trace_campaign_20260705.md).
//
// Classification census: the witness pair proved the lesion is
// label-independent (h3-20260717-051341 intent=explain/enumeration/
// architecture_explain vs h6-20260715-062137 intent=root_cause/mechanism/
// root_cause). The deterministic driver of the lens-first window is the typed
// source_inventory_profile authority (env.SourceInventoryProfileActive), not
// any intent/question_kind/scenario label: EnsureReadStageNodes authors the
// lens probe node into EVERY compiled read TaskGraph, and
// prioritizeSourceInventoryLensWindow keys only on the profile + lens
// execution state. The census test below walks the full closed Intent ×
// Scenario matrix (question_kind is a lossy projection into Intent, so the
// matrix covers its image) through the REAL compiler and window selection and
// pins that every combination reaches the one shared
// ExploreToolSurfaceSourceInventoryLens value — the typed enum is the join to
// the agent-side surface pins in internal/agent/toolwin_fix_test.go, which
// guarantee that surface keeps trace_query whenever the run carries a typed
// trace artifact.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func toolwinTracePreflightBusCtxForTest() *types.BusContext {
	return &types.BusContext{
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Active:                   true,
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "eval/fixtures/real_traces/donghu.ftrace",
				Carrier: "attachment",
			}},
		}),
	}
}

// ② Census: every Intent × Scenario combination with an authoritative typed
// source_inventory_profile compiles to a graph carrying the lens probe node,
// and the scheduler's lens-first selection lands on the single shared
// ExploreToolSurfaceSourceInventoryLens surface for all of them.
func TestToolwinFix_ClassificationCensus_LensWindowSharedAcrossAllLabels(t *testing.T) {
	intents := []types.Intent{
		types.IntentExplain,
		types.IntentRootCause,
		types.IntentTrace,
		types.IntentEnumerate,
		types.IntentConfigQuery,
		types.IntentReturnValue,
		types.IntentUnknown,
	}
	scenarios := []types.Scenario{
		types.ScenarioArchitectureExplain,
		types.ScenarioRootCause,
		types.ScenarioConfigTrace,
		types.ScenarioPerformanceBottleneck,
		types.ScenarioGeneric,
	}
	for _, intent := range intents {
		for _, scenario := range scenarios {
			rm := types.RequestModel{
				RawRequest: "census request",
				Intent:     intent,
				Scenario:   scenario,
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
					Confidence:        0.9,
				},
			}
			if !types.SourceInventoryPrincipalAuthorityActive(rm) {
				t.Fatalf("intent=%s scenario=%s: census fixture lost source-inventory authority; the combo can no longer produce the witness window", intent, scenario)
			}
			out := compiler.Compile(rm, budget.BudgetSignals{})
			window := make([]*types.TaskNode, 0, len(out.TaskGraph.Nodes))
			for i := range out.TaskGraph.Nodes {
				window = append(window, &out.TaskGraph.Nodes[i])
			}
			lensWindow := sourceInventoryLensFirstWindow(window, true, false)
			if len(lensWindow) == 0 {
				t.Fatalf("intent=%s scenario=%s: compiled graph produced no lens-first window", intent, scenario)
			}
			for _, node := range lensWindow {
				if !sourceInventoryLensProbeNode(node) {
					t.Fatalf("intent=%s scenario=%s: lens-first window carried non-lens node %s", intent, scenario, node.ID)
				}
			}
			if surface := exploreToolSurfaceForWindow(lensWindow); surface != types.ExploreToolSurfaceSourceInventoryLens {
				t.Fatalf("intent=%s scenario=%s: lens window surface = %q, want the shared source_inventory_lens surface", intent, scenario, surface)
			}
		}
	}
}

// ① Scheduler-owned one-dispatch policies: an attached-trace run's ACTIVE
// allowlist admits trace_query unless the lane explicitly denies it.
func TestToolwinFix_ReadDispatchPolicyAdmitsTraceQueryWithAttachedTrace(t *testing.T) {
	busCtx := toolwinTracePreflightBusCtxForTest()
	decision := readLoopNextActionDecision{
		Active: true,
		Action: loopkernel.LoopActionAddProof,
	}
	policy := readDispatchPolicyForNextAction(decision, nil)
	if !policy.Active {
		t.Fatal("add_proof policy fixture must be active")
	}
	if policy.AllowsTool("trace_query") {
		t.Fatal("precondition drifted: the authored add_proof allowlist should not already carry trace_query (update this pin if the lane vocabulary changed deliberately)")
	}

	admitted := admitAttachedTraceToolIntoReadDispatchPolicy(policy, busCtx)
	if !admitted.AllowsTool("trace_query") {
		t.Fatalf("attached-trace add_proof window must admit trace_query, allowed=%v", admitted.AllowedTools)
	}
	for _, name := range policy.AllowedTools {
		if !admitted.AllowsTool(name) {
			t.Fatalf("admitting trace_query must not drop %q from the allowlist", name)
		}
	}

	// Idempotent on a second pass.
	twice := admitAttachedTraceToolIntoReadDispatchPolicy(admitted, busCtx)
	if strings.Join(twice.AllowedTools, ",") != strings.Join(admitted.AllowedTools, ",") {
		t.Fatalf("admission must be idempotent: %v vs %v", twice.AllowedTools, admitted.AllowedTools)
	}
}

// ③ Negative arms: no trace artifact → policy byte-identical; the emit-only
// landing-repair lane's EXPLICIT trace_query denial is honored (that window is
// deliberately not an investigation window — it repairs the completion form of
// an already-closed investigation and denies every evidence tool).
func TestToolwinFix_ReadDispatchPolicyNegativeArms(t *testing.T) {
	decision := readLoopNextActionDecision{
		Active: true,
		Action: loopkernel.LoopActionAddProof,
	}
	addProof := readDispatchPolicyForNextAction(decision, nil)

	// No trace artifact → unchanged.
	plain := admitAttachedTraceToolIntoReadDispatchPolicy(addProof, &types.BusContext{})
	if strings.Join(plain.AllowedTools, ",") != strings.Join(addProof.AllowedTools, ",") {
		t.Fatalf("no-attachment policy must stay byte-identical: %v vs %v", plain.AllowedTools, addProof.AllowedTools)
	}
	if admitAttachedTraceToolIntoReadDispatchPolicy(addProof, nil).AllowsTool("trace_query") {
		t.Fatal("nil bus context must not admit trace_query")
	}

	// Landing repair: real lane constructor, explicit deny honored.
	busCtx := toolwinTracePreflightBusCtxForTest()
	landing, _, active := localLandingRepairDispatchPolicy(
		[]types.RepairDirective{{
			Kind:   types.RepairStructuredHandoff,
			Origin: types.RepairOriginCompletionFormPrefix + "reason",
		}},
		nil, nil, nil, "", "", nil,
	)
	if !active {
		t.Fatal("landing-repair policy fixture must be active")
	}
	admitted := admitAttachedTraceToolIntoReadDispatchPolicy(landing, busCtx)
	if admitted.AllowsTool("trace_query") {
		t.Fatal("landing repair explicitly denies trace_query; the attached-trace arm must honor the typed opt-out")
	}
	if strings.Join(admitted.AllowedTools, ",") != strings.Join(landing.AllowedTools, ",") {
		t.Fatalf("landing-repair policy must stay byte-identical: %v vs %v", admitted.AllowedTools, landing.AllowedTools)
	}

	// Inactive / unrestricted policies stay untouched.
	if got := admitAttachedTraceToolIntoReadDispatchPolicy(types.ReadDispatchPolicy{}, busCtx); got.Active || len(got.AllowedTools) != 0 {
		t.Fatalf("inactive policy must stay untouched: %+v", got)
	}
}

// ①(install joint) the single policy install choke point applies the
// attached-trace admission before the policy reaches the bus and the budget.
func TestToolwinFix_InstallReadDispatchPolicyAdmitsTraceQuery(t *testing.T) {
	busCtx := toolwinTracePreflightBusCtxForTest()
	busCtx.Mutable = types.NewMutableState("toolwin install")
	o := &Orchestrator{busCtx: busCtx}
	decision := readLoopNextActionDecision{
		Active: true,
		Action: loopkernel.LoopActionAddProof,
	}
	policy := readDispatchPolicyForNextAction(decision, nil)

	restore := o.installReadDispatchPolicyForExplore(policy, true)
	defer restore()
	installed := types.NormalizeReadDispatchPolicy(busCtx.ReadDispatchPolicy)
	if !installed.Active {
		t.Fatal("policy must be installed on the bus")
	}
	if !installed.AllowsTool("trace_query") {
		t.Fatalf("installed attached-trace policy must allow trace_query, allowed=%v", installed.AllowedTools)
	}
	eb := busCtx.Mutable.ExploreBudget()
	if eb == nil {
		t.Fatal("policy install must tighten the explore budget")
	}
	if cap, ok := eb.PerToolCap["trace_query"]; !ok || cap <= 0 {
		t.Fatalf("tightened budget must cap trace_query alongside the other allowed tools, got %v", eb.PerToolCap)
	}
}
