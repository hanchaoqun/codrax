package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

type readLoopNextActionDecision struct {
	Active          bool
	Action          loopkernel.LoopRecommendedAction
	ReasonCode      string
	ProofState      loopkernel.ProofCoverageAuthorityState
	TruthAction     types.TruthRecommendedAction
	RouteSurface    loopkernel.LoopToolSurface
	RouteReasonCode string
	ToolSuggestions []string
	Policy          types.ReadDispatchPolicy
}

func (s *graphState) setReadLoopNextAction(decision readLoopNextActionDecision) {
	if s == nil || !decision.Active {
		return
	}
	decision.Policy = types.NormalizeReadDispatchPolicy(decision.Policy)
	s.readLoopNextAction = decision
}

func (s *graphState) consumeReadLoopNextAction() (readLoopNextActionDecision, bool) {
	if s == nil || !s.readLoopNextAction.Active {
		return readLoopNextActionDecision{}, false
	}
	decision := s.readLoopNextAction
	s.readLoopNextAction = readLoopNextActionDecision{}
	return decision, true
}

func (o *Orchestrator) recordReadLoopNextActionForRetry(state *graphState, reason string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || state == nil {
		return
	}
	decision, ok := readLoopNextActionDecisionFromMutable(o.busCtx.Mutable)
	if !ok {
		return
	}
	decision.Policy = readDispatchPolicyForNextAction(decision, o.busCtx.Mutable)
	decision = applyRuntimeArtifactOnlyNextActionMenu(decision, o.busCtx, o.busCtx.Mutable)
	state.setReadLoopNextAction(decision)
	logging.Debug("[orchestrator] read loop next-action selected for %s: action=%s reason=%s",
		firstNonEmptyRetryString(reason, "retry"), decision.Action, decision.ReasonCode)
	logging.Debug("[diag orchestrator] phase=read_loop_next_action_selected action=%s reason=%s proof_state=%s truth_action=%s route_surface=%s route_reason=%s policy_active=%t policy_tools=%s trigger=%s",
		firstNonEmptyRetryString(string(decision.Action), "none"),
		firstNonEmptyRetryString(decision.ReasonCode, "none"),
		firstNonEmptyRetryString(string(decision.ProofState), "unknown"),
		firstNonEmptyRetryString(string(decision.TruthAction), "none"),
		firstNonEmptyRetryString(string(decision.RouteSurface), "none"),
		firstNonEmptyRetryString(decision.RouteReasonCode, "none"),
		decision.Policy.IsActive(),
		strings.Join(decision.Policy.AllowedTools, ","),
		firstNonEmptyRetryString(reason, "retry"))
}

func applyReadLoopNextActionHint(state *graphState, hint *string, parallelHints []string) (types.ReadDispatchPolicy, bool) {
	if state == nil {
		return types.ReadDispatchPolicy{}, false
	}
	decision, ok := state.consumeReadLoopNextAction()
	if !ok {
		return types.ReadDispatchPolicy{}, false
	}
	logging.Debug("[diag orchestrator] phase=read_loop_next_action_consumed action=%s reason=%s proof_state=%s truth_action=%s route_surface=%s route_reason=%s policy_active=%t policy_tools=%s",
		firstNonEmptyRetryString(string(decision.Action), "none"),
		firstNonEmptyRetryString(decision.ReasonCode, "none"),
		firstNonEmptyRetryString(string(decision.ProofState), "unknown"),
		firstNonEmptyRetryString(string(decision.TruthAction), "none"),
		firstNonEmptyRetryString(string(decision.RouteSurface), "none"),
		firstNonEmptyRetryString(decision.RouteReasonCode, "none"),
		decision.Policy.IsActive(),
		strings.Join(decision.Policy.AllowedTools, ","))
	actionHint := renderReadLoopNextActionDirective(decision)
	if strings.TrimSpace(actionHint) == "" {
		return decision.Policy, decision.Policy.IsActive()
	}
	if len(parallelHints) > 0 {
		for i := range parallelHints {
			parallelHints[i] = prependRetryHint(actionHint, parallelHints[i])
		}
		return decision.Policy, decision.Policy.IsActive()
	}
	if hint != nil {
		*hint = prependRetryHint(actionHint, *hint)
	}
	return decision.Policy, decision.Policy.IsActive()
}

func readLoopNextActionDecisionFromMutable(m *types.MutableState) (readLoopNextActionDecision, bool) {
	guidance, ok := loopkernel.ReadProofGuidanceFromMutable(m)
	return readLoopNextActionDecisionFromGuidance(guidance, ok)
}

// runtimeArtifactOnlyReadRun is the §29.174 RUN2AUDIT-1 F8 typed gate
// for the runtime-artifact-only run shape. Both legs are precise typed
// signals — never keyword sniffing on prose:
//   - the deterministic Run-entry preflight carries a trace artifact
//     (the same carrier admitAttachedTraceToolIntoReadDispatchPolicy
//     consumes, §29.118 TOOLWIN), and
//   - the analyzer emitted the explicit user current-source exclusion
//     (ExcludesCurrentSource: exclude mode + explicit-user-boundary
//     kind + verbatim quote, the CPD #58 enum minted for "不分析代码").
func runtimeArtifactOnlyReadRun(busCtx *types.BusContext) bool {
	if busCtx == nil || !busCtx.RuntimeArtifactPreflight.HasTraceArtifact() {
		return false
	}
	if busCtx.AnalysisIR == nil {
		return false
	}
	return busCtx.AnalysisIR.RequestModel.ExternalObservationPolicy.ExcludesCurrentSource()
}

// repoCodeProofToolFamily is the repository-code proof vocabulary that
// the runnable_2.txt resume checkpoint advertised on a pure-trace run
// (:75 — run_tests / repo_map / read_file / grep offered while the user
// had said "不分析代码" and every proof lane was trace-side).
var repoCodeProofToolFamily = map[string]bool{
	"run_tests": true,
	"repo_map":  true,
	"read_file": true,
	"grep":      true,
}

func filterRepoCodeProofTools(names []string) []string {
	var out []string
	for _, name := range names {
		if repoCodeProofToolFamily[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

func admitTraceQueryIntoToolList(names, denied []string) []string {
	const traceTool = "trace_query"
	for _, name := range denied {
		if name == traceTool {
			return names
		}
	}
	for _, name := range names {
		if name == traceTool {
			return names
		}
	}
	return append(append([]string(nil), names...), traceTool)
}

// readRunHasAcceptedClosure reports the typed accepted-closure state —
// the same carriers the explore checkpoint's "accepted closure state
// present" fact reads. When closure is already accepted and the loop
// still recommends add_proof, the remaining gap is structural finishing
// (an emit round that has not landed its closure form yet), not weak
// proof collection.
func readRunHasAcceptedClosure(m *types.MutableState) bool {
	if m == nil {
		return false
	}
	if strings.TrimSpace(m.StableInvestigationCompleteReason()) != "" ||
		strings.TrimSpace(m.StableInvestigationResultKind()) != "" {
		return true
	}
	if artifacts := m.TurnAArtifacts(); artifacts != nil {
		return strings.TrimSpace(artifacts.AcceptedClosureReason) != "" ||
			strings.TrimSpace(artifacts.AcceptedResultKind) != ""
	}
	return false
}

// applyRuntimeArtifactOnlyNextActionMenu rewrites the add_proof
// decision's MENU and WORD faces for runtime-artifact-only runs
// (§29.174 RUN2AUDIT-1 F8). Soft-guidance scope only: the repo-code
// proof family is filtered out of the ADVISORY tool vocabulary
// (route_tools / preferred) and trace_query — the only tool that can
// touch the attached artifact — is admitted in its place (DeniedTools
// stays the typed opt-out, same contract as
// admitAttachedTraceToolIntoReadDispatchPolicy); when the typed
// accepted-closure state says the gap is structural finishing, the
// reason word swaps proof_weak → closure_incomplete. Completion
// gates, retry budgets, and the add_proof action itself are untouched.
//
// Policy.AllowedTools is deliberately NOT filtered: it is the
// ENFORCEMENT list (validateExplorerReadDispatchPolicyToolCall hard-
// rejects out-of-list calls in the continuation window), and on a
// trace run read_file / grep stay legitimately executable — read_file
// is the trace_query payload_ref blob drill lane (§29.19 beneficial
// re-probe, §29.121 typed blob observation lane) and grep/read_file
// are trace_query's own documented fallback for unsupported formats.
// Filtering them here would turn a menu fix into a new hard gate,
// which the §29.174 批 discipline forbids (全部=计数/披露/菜单面).
// trace_query is still admitted into the allowlist so the worded
// policy_allowed_tools face matches what install-time admission
// (§29.118 TOOLWIN) will enforce.
func applyRuntimeArtifactOnlyNextActionMenu(decision readLoopNextActionDecision, busCtx *types.BusContext, m *types.MutableState) readLoopNextActionDecision {
	if !decision.Active || !runtimeArtifactOnlyReadRun(busCtx) {
		return decision
	}
	if decision.ReasonCode == "proof_weak" && readRunHasAcceptedClosure(m) {
		decision.ReasonCode = "closure_incomplete"
	}
	denied := decision.Policy.DeniedTools
	decision.ToolSuggestions = admitTraceQueryIntoToolList(filterRepoCodeProofTools(decision.ToolSuggestions), denied)
	if decision.Policy.Active {
		decision.Policy.ReasonCode = decision.ReasonCode
		decision.Policy.AllowedTools = admitTraceQueryIntoToolList(decision.Policy.AllowedTools, denied)
		decision.Policy.PreferredTools = admitTraceQueryIntoToolList(filterRepoCodeProofTools(decision.Policy.PreferredTools), denied)
		decision.Policy = types.NormalizeReadDispatchPolicy(decision.Policy)
	}
	return decision
}

func readLoopNextActionDecisionFromGuidance(guidance loopkernel.ReadProofGuidance, ok bool) (readLoopNextActionDecision, bool) {
	if !ok || !guidance.Active || guidance.HardBlock {
		return readLoopNextActionDecision{}, false
	}
	if guidance.RecommendedAction != loopkernel.LoopActionAddProof {
		return readLoopNextActionDecision{}, false
	}
	route := loopkernel.ToolRouteForAction(guidance.RecommendedAction)
	decision := readLoopNextActionDecision{
		Active:          true,
		Action:          guidance.RecommendedAction,
		ReasonCode:      firstNonEmptyRetryString(guidance.ReasonCode, route.ReasonCode, "proof_weak"),
		ProofState:      guidance.State,
		TruthAction:     guidance.TruthAction,
		RouteSurface:    route.Surface,
		RouteReasonCode: firstNonEmptyRetryString(route.ReasonCode, "loop_tool_route_verification"),
		ToolSuggestions: append([]string(nil), route.ToolSuggestions...),
	}
	decision.Policy = readDispatchPolicyForNextAction(decision, nil)
	return decision, true
}

func readLoopNextActionDecisionSummary(decision readLoopNextActionDecision) string {
	if !decision.Active {
		return ""
	}
	parts := []string{
		fmt.Sprintf("loop next-action=%s", firstNonEmptyRetryString(string(decision.Action), "none")),
		"source=proof_authority",
		fmt.Sprintf("reason=%s", firstNonEmptyRetryString(decision.ReasonCode, "none")),
		fmt.Sprintf("route_surface=%s", firstNonEmptyRetryString(string(decision.RouteSurface), "none")),
		fmt.Sprintf("route_reason=%s", firstNonEmptyRetryString(decision.RouteReasonCode, "none")),
	}
	if len(decision.ToolSuggestions) > 0 {
		parts = append(parts, "route_tools="+strings.Join(decision.ToolSuggestions, ","))
	}
	if policy := types.NormalizeReadDispatchPolicy(decision.Policy); policy.Active {
		parts = append(parts, "policy_allowed_tools="+strings.Join(policy.AllowedTools, ","))
		if len(policy.ScopePaths) > 0 {
			parts = append(parts, "policy_scope_paths="+strings.Join(policy.ScopePaths, ","))
		}
	}
	return strings.Join(parts, " ")
}

func renderReadLoopNextActionDirective(decision readLoopNextActionDecision) string {
	if !decision.Active {
		return ""
	}
	return strings.Join([]string{
		"Read loop typed next action:",
		readLoopNextActionDecisionSummary(decision) + ".",
		"Use the next explore retry as a narrow proof-collection continuation for already accepted evidence. Do not restart broad repository discovery for this action; collect the smallest additional typed proof available through the current stage's allowed tools, or preserve an explicit unverified-proof caveat if no proof route is available.",
	}, "\n")
}

func readDispatchPolicyForNextAction(decision readLoopNextActionDecision, mutable *types.MutableState) types.ReadDispatchPolicy {
	if !decision.Active || decision.Action != loopkernel.LoopActionAddProof {
		return types.ReadDispatchPolicy{}
	}
	return types.NormalizeReadDispatchPolicy(types.ReadDispatchPolicy{
		Active:         true,
		Action:         types.ReadDispatchPolicyActionAddProof,
		ReasonCode:     firstNonEmptyRetryString(decision.ReasonCode, decision.RouteReasonCode, "proof_weak"),
		RouteSurface:   firstNonEmptyRetryString(string(decision.RouteSurface), types.ReadDispatchPolicySurfaceVerify),
		AllowedTools:   []string{"run_tests", "repo_map", "read_file", "grep", "emit_evidence", "emit_investigation_complete"},
		DeniedTools:    []string{"exec_command", "list_files"},
		PreferredTools: append([]string(nil), decision.ToolSuggestions...),
		ScopePaths:     readDispatchPolicyAcceptedEvidencePaths(mutable),
		MaxToolCalls:   3,
		OneShot:        true,
	})
}

func readDispatchPolicyAcceptedEvidencePaths(mutable *types.MutableState) []string {
	if mutable == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
		path = strings.TrimPrefix(path, "./")
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	if artifacts := mutable.TurnAArtifacts(); artifacts != nil {
		for _, ev := range artifacts.EvidenceItems {
			add(ev.Source)
		}
	}
	for _, ev := range mutable.EmittedEvidence() {
		add(ev.Source)
	}
	const maxScopePaths = 8
	if len(out) > maxScopePaths {
		out = out[:maxScopePaths]
	}
	return out
}

// admitAttachedTraceToolIntoReadDispatchPolicy is the TOOLWIN-FIX arm for
// scheduler-owned one-dispatch policies (§29.118 / §29.104.20). Policy
// allowlists are authored per action lane with repository-tool vocabularies;
// on a run that carries a typed runtime trace artifact, an ACTIVE allowlist
// that neither allows nor explicitly denies trace_query would leave the
// bounded explore continuation window trace-blind — the same lesion shape as
// the source-inventory lens window (model told to collect the smallest
// additional typed proof, while the only tool that can touch the attached
// artifact is absent). The typed gate is the Run-entry deterministic
// preflight carrier (BusContext.RuntimeArtifactPreflight); an explicit
// DeniedTools entry stays the typed opt-out for windows that are deliberately
// not investigation windows (the landing-repair lane is emit-only form repair
// and declares trace_query denied on purpose — that denial is honored here).
// Runs without a trace artifact keep every policy byte-identical.
func admitAttachedTraceToolIntoReadDispatchPolicy(policy types.ReadDispatchPolicy, busCtx *types.BusContext) types.ReadDispatchPolicy {
	if busCtx == nil || !busCtx.RuntimeArtifactPreflight.HasTraceArtifact() {
		return policy
	}
	if !policy.Active || len(policy.AllowedTools) == 0 {
		return policy
	}
	const traceTool = "trace_query"
	for _, name := range policy.DeniedTools {
		if name == traceTool {
			return policy
		}
	}
	for _, name := range policy.AllowedTools {
		if name == traceTool {
			return policy
		}
	}
	policy.AllowedTools = append(append([]string(nil), policy.AllowedTools...), traceTool)
	return types.NormalizeReadDispatchPolicy(policy)
}

func (o *Orchestrator) installReadDispatchPolicyForExplore(policy types.ReadDispatchPolicy, active bool) func() {
	if o == nil || o.busCtx == nil {
		return func() {}
	}
	prevPolicy := o.busCtx.ReadDispatchPolicy
	var prevBudget *types.ExploreBudget
	if o.busCtx.Mutable != nil {
		prevBudget = o.busCtx.Mutable.ExploreBudget()
	}
	policy = types.NormalizeReadDispatchPolicy(policy)
	policy = admitAttachedTraceToolIntoReadDispatchPolicy(policy, o.busCtx)
	if active && policy.Active {
		o.busCtx.ReadDispatchPolicy = policy
		if o.busCtx.Mutable != nil {
			o.busCtx.Mutable.SetExploreBudget(tightenExploreBudgetForReadDispatchPolicy(prevBudget, policy))
		}
	}
	return func() {
		o.busCtx.ReadDispatchPolicy = prevPolicy
		if o.busCtx.Mutable != nil {
			o.busCtx.Mutable.SetExploreBudget(prevBudget)
		}
	}
}

func tightenExploreBudgetForReadDispatchPolicy(base *types.ExploreBudget, policy types.ReadDispatchPolicy) *types.ExploreBudget {
	policy = types.NormalizeReadDispatchPolicy(policy)
	if !policy.Active || policy.MaxToolCalls <= 0 {
		if base == nil {
			return nil
		}
		return base.Clone()
	}
	var out *types.ExploreBudget
	if base != nil {
		out = base.Clone()
	} else {
		out = &types.ExploreBudget{}
	}
	if out.PerToolCap == nil {
		out.PerToolCap = map[string]int{}
	}
	for _, name := range policy.AllowedTools {
		if current, ok := out.PerToolCap[name]; !ok || current <= 0 || current > policy.MaxToolCalls {
			out.PerToolCap[name] = policy.MaxToolCalls
		}
	}
	if out.OverallCap <= 0 || out.OverallCap > policy.MaxToolCalls {
		out.OverallCap = policy.MaxToolCalls
	}
	if out.PerToolUsed == nil {
		out.PerToolUsed = map[string]int{}
	}
	return out
}
