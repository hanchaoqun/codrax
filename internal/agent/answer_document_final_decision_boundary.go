package agent

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocCallChainFinalEvidenceBoundary keeps the last prompt surface
// aligned with the typed call-chain contract. It is language-agnostic and
// prompt-only: names and request wording never become behavior authority.
func renderAnswerDocCallChainFinalEvidenceBoundary(view *types.AnswerSemanticView) string {
	if view == nil || view.Family != types.QFCallChain {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Final Call-Chain Evidence Boundary\n\n")
	b.WriteString("- You own the explanation. Preserve only directed hops carried by grounded caller-to-callee evidence. A call-site proves that edge, not the callee's body, side effect, storage medium, synchronization mode, or completion semantics.\n")
	b.WriteString("- Describe a terminal endpoint's internal behavior only from a separate grounded definition/mechanism row for that endpoint, and cite that implementation line when the behavior matters. Class names, method names, comments, layer labels, and the wording of the request do not mint implementation authority. If no terminal-body proof is available, say only that the chain reaches or invokes the endpoint.\n")
	b.WriteString("- Keep the model-authored summary useful and concise; this boundary supplies evidence caliber only and does not author a conclusion.\n\n")
	return b.String()
}

// renderAnswerDocTraceFinalDecisionBoundary replays the small set of typed
// authority limits that must govern the model's synthesis after every other
// dynamic section has been rendered. It deliberately does not copy candidate
// labels or choose a cause/recommendation. The deterministic system continues
// to own facts while the model owns the conclusion.
func renderAnswerDocTraceFinalDecisionBoundary(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	authority := answerDocRuntimeTraceGuidanceView(ctx)
	if !authority.RuntimeTrace {
		return ""
	}
	set := types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))
	var requestModel *types.RequestModel
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	if len(set.Projections) == 0 || !types.RuntimeTraceReportMaterializationAllowed(requestModel, set) {
		return ""
	}

	hasActual, hasEliminable := traceDecisionAxesPresent(set)
	var b strings.Builder
	b.WriteString("## Final Trace Decision Boundary (Typed Facts; Model-Owned Conclusion)\n\n")
	b.WriteString("- You own the diagnosis, prioritization, optimization direction, and wording. The system supplies measurements and authority ceilings only; do not merely restate the projection rows.\n")
	if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view != nil && view.TraceCausalClaimContract.Active() {
		allowed := make([]string, 0, len(view.TraceCausalClaimContract.Allowed))
		for _, caliber := range view.TraceCausalClaimContract.Allowed {
			allowed = append(allowed, string(caliber))
		}
		fmt.Fprintf(&b, "- principal_trace_summary_contract: %s Keep the lead/detail wording within the declared `%s` scope. This is your causal-strength declaration; it does not choose the cause. No conclusion is inferred from prose or written for you.\n",
			tool.TraceCausalClaimPrincipalSummaryShape(view.TraceCausalClaimContract.Allowed), strings.Join(allowed, "|"))
		b.WriteString(renderTraceCausalClaimCaliberMapping(view.TraceCausalClaimContract))
	}
	if authority.CausalUnproven {
		b.WriteString("- causal_conclusion=`unproven`: the strongest supported synthesis is a bounded candidate or first validation direction, not a proven dropped-frame/frame-deadline cause.\n")
	}
	if authority.FrameEvidenceStatus != "" {
		fmt.Fprintf(&b, "- frame_evidence_status=`%s`: do not infer a stronger frame/deadline attribution.\n", authority.FrameEvidenceStatus)
		b.WriteString(renderTraceFrameEvidenceStatusSemantics(authority.FrameEvidenceStatus))
	}
	b.WriteString(renderTraceFinalSelectedWindowAuthority(set, authority.FrameEvidenceStatus))
	b.WriteString(renderTraceFinalTimeRoleAuthority(set))
	b.WriteString("- scheduler_state_interval_authority=`typed_state_segments`: a typed wakeup ends the preceding sleep/io_wait segment; time from wakeup until the next sched-in is runnable_wait. Do not extend an IO/D/sleep duration to the later run timestamp or relabel the two state segments as one wait state.\n")
	b.WriteString("- trace_value_caliber_authority=`measured_occupancy_vs_effective_attribution`: measured state occupancy/cumulative duration and effective attribution are different axes. Effective attribution is the published ranking/eliminable value; never call it an actual wait/state duration when a distinct measured occupancy is provided.\n")
	b.WriteString(renderTraceFinalStateValueAuthority(set))
	switch {
	case hasActual && hasEliminable:
		b.WriteString("- available_axes=`actual_occupancy,existing_rule_eliminable`: compare both and explain their different decision use. Actual occupancy, existing-rule eliminable impact, and proven frame causality are distinct calibers; none substitutes for another. Their coexistence does not prove physical independence.\n")
	case hasActual:
		b.WriteString("- available_axes=`actual_occupancy`: identify the measured time concentration and a validation/optimization direction without inventing an existing-rule eliminable amount.\n")
	case hasEliminable:
		b.WriteString("- available_axes=`existing_rule_eliminable`: prioritize the typed repair candidates without inventing a separate actual-occupancy ranking.\n")
	default:
		b.WriteString("- available_axes=`none`: stay within the target-state, path, and evidence-boundary facts; do not invent a ranked cause.\n")
	}
	b.WriteString(renderTraceFinalCompactAuthorityLedger(set))
	b.WriteString(renderTraceFinalAggregateScaleAuthority(traceDecisionTypedAggregateFacts(answerDocObservationLedger(ctx).Records)))
	b.WriteString("- cross_row_addition=`not_authorized_without_exact_typed_relation`: a row-local state breakdown applies only to that row. Do not merge, decompose, compare as one subtotal, or add values from different rows/threads/fix directions unless one exact typed relation/fold carrier names those members and authorizes that operation.\n")
	b.WriteString(renderTraceFinalSynthesisScope(set, authority.FrameEvidenceStatus))
	b.WriteString("- relation_scope=`typed_relations_only`: preserve directed wakeup/path and typed holder/waiter or overlap relations exactly. Temporal order, adjacency, a candidate flag, or a kernel caller symbol alone does not prove synchronous blocking, lock ownership, post-wakeup preemption, or physical coupling.\n\n")
	return b.String()
}

// renderTraceFinalTimeRoleAuthority repeats the selected-window and target
// state roles at the final prompt tail, where a whole-attachment preview or a
// later switch-in timestamp cannot outcompete the exact trace_query account.
// It consumes only the typed projection and remains soft reasoning context:
// no model prose is inspected and no answer value is rewritten.
func renderTraceFinalTimeRoleAuthority(set types.TraceCausalProjectionSet) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		if !types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			continue
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		fmt.Fprintf(&b, "- time_role_authority artifact=`%s`; selected_query_window=`%.6f..%.6f`; selected_query_window_duration=%.3fms; attachment_extent_role=`artifact_navigation_only_not_selected_window_duration`; out_of_window_switch_in_role=`separate_event_not_selected_window_state_duration`.\n",
			traceDecisionPromptScalar(label), projection.WindowStartTs, projection.WindowEndTs, projection.WindowDurationMS())
		account := projection.TargetStateAccount
		if account == nil || strings.TrimSpace(account.Subject) == "" || account.TotalMS <= 0 {
			continue
		}
		fmt.Fprintf(&b, "  - selected_window_target_state subject=`%s`; running=%.3fms; runnable=%.3fms; sleep=%.3fms; d_state=%.3fms; io_wait=%.3fms; accounted_total=%.3fms; value_role=`target_thread_wall_clock_partition_inside_selected_query_window`; partition_members=`five_engine_lanes`.\n",
			traceDecisionPromptScalar(strings.TrimSpace(account.Subject)), account.RunningMS, account.RunnableMS,
			account.SleepMS, account.DStateMS, account.IOWaitMS, account.TotalMS)
		b.WriteString("  - sleep_state_semantics=`state_only_mechanism_unproven`; S-state proves a selected-window sleep interval, not that it was normal pacing, downstream-response waiting, lock/condition waiting, IPC, timer/event waiting, or the root cause. A separately typed relation is required for that mechanism.\n")
		b.WriteString("  - duration_selection_rule=`use_the_value_whose_typed_role_matches_the_sentence`; do not replace selected-window sleep/total with whole-attachment extent, and do not extend sleep to a later sched-in after the selected window. A post-wakeup runnable/dispatch duration requires its own typed interval.\n")
	}
	return b.String()
}

// renderTraceCausalClaimCaliberMapping keeps the report-local JSON enum and
// its evidence-status vocabulary visibly distinct. The model still chooses
// both the diagnosis and the declaration; this text only explains the exact
// wire values already projected by the typed contract.
func renderTraceCausalClaimCaliberMapping(contract *types.TraceCausalClaimContract) string {
	if contract == nil || !contract.Active() {
		return ""
	}
	allowed := make(map[types.TraceCausalClaimCaliber]bool, len(contract.Allowed))
	for _, caliber := range contract.Allowed {
		allowed[caliber] = true
	}
	var parts []string
	if allowed[types.TraceCausalClaimNoConclusion] {
		parts = append(parts, "`no_causal_conclusion` only when the lead makes no cause or candidate attribution")
	}
	if allowed[types.TraceCausalClaimBoundedWindow] {
		parts = append(parts, "`bounded_window_candidate` when the lead names or ranks selected-window candidates while keeping frame/deadline causality unproven")
	}
	if allowed[types.TraceCausalClaimTypedChain] {
		parts = append(parts, "`typed_chain_cause` only when the lead's causal attribution is bounded by a typed causal chain")
	}
	if allowed[types.TraceCausalClaimTypedFrame] {
		parts = append(parts, "`typed_frame_cause` only when typed frame/deadline causality supports that claim")
	}
	if len(parts) == 0 {
		return ""
	}
	return "- trace_causal_claim_caliber_mapping: " + strings.Join(parts, "; ") + ". Evidence-status values such as `unproven` are not JSON enum values for this field.\n"
}

// renderTraceFinalSelectedWindowAuthority prevents attachment previews and
// pre-triage navigation rows from silently widening a typed selected window.
// A producer-owned typed relation may still bind evidence across windows; the
// prompt does not inspect or reject model prose.
func renderTraceFinalSelectedWindowAuthority(set types.TraceCausalProjectionSet, frameEvidenceStatus string) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		if !types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			continue
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		fmt.Fprintf(&b, "- selected_window_authority artifact=`%s`; selected_window=`%.6f..%.6f`; out_of_window_artifact_preview=`navigation_only_not_selected_window_evidence`; a preview/triage row outside this interval cannot establish selected-window state, event order, duration, frame boundary, completion, or deadline unless a separate typed relation explicitly binds it into this projection.\n",
			traceDecisionPromptScalar(label), projection.WindowStartTs, projection.WindowEndTs)
		if frameEvidenceStatus == "absent" || frameEvidenceStatus == "unavailable" {
			fmt.Fprintf(&b, "  frame_boundary_authority=`not_provided`; frame_evidence_status=`%s`; do not turn an unbound preview marker into this selected window's frame boundary or cadence explanation.\n", frameEvidenceStatus)
		}
	}
	return b.String()
}

// renderTraceFinalAggregateScaleAuthority distinguishes a measured aggregate
// value/density from an absolute severity category. It is derived only from
// typed calibration fields and remains soft reasoning guidance.
func renderTraceFinalAggregateScaleAuthority(facts []traceDecisionAggregateFact) string {
	missing := make(map[string]bool)
	for _, fact := range facts {
		hasAbsoluteLevel := false
		for _, calibration := range fact.Calibration {
			if calibration[0] == "absolute_level" && strings.TrimSpace(calibration[1]) != "" {
				hasAbsoluteLevel = true
				break
			}
		}
		if !hasAbsoluteLevel {
			key := strings.TrimSpace(fact.Signal)
			if key == "" {
				key = strings.TrimSpace(fact.Kind)
			}
			if key != "" {
				missing[key] = true
			}
		}
	}
	if len(missing) == 0 {
		return ""
	}
	keys := make([]string, 0, len(missing))
	for key := range missing {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return fmt.Sprintf("- aggregate_absolute_level_authority=`not_provided`; affected_signals=`%s`; numeric value or density may be compared only within a typed comparison/calibration scope and does not by itself mean low/medium/high or serious/not-serious. Use the neutral form `observed value/density; absolute level unavailable without calibration` when the raw aggregate is relevant; do not supply an absolute severity adjective.\n", strings.Join(keys, ","))
}

// renderTraceFinalCompactAuthorityLedger brings two high-cost relation
// decisions to the final prompt tail: whether the selected target has an exact
// typed waiter/holder row, and which single seat leads each typed fix
// direction. It does not choose a diagnosis or calculate a direction subtotal.
// Inputs are projection fields only; user/model/final prose never participates.
func renderTraceFinalCompactAuthorityLedger(set types.TraceCausalProjectionSet) string {
	if len(set.Projections) == 0 {
		return ""
	}
	var b strings.Builder
	for index, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		target := ""
		if projection.TargetStateAccount != nil {
			target = strings.TrimSpace(projection.TargetStateAccount.Subject)
		}
		relations := traceFinalTargetBlockingRelations(projection, target)
		switch {
		case target == "":
			fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target_direct_blocking_authority=`unavailable_without_typed_target`; direct_blocking_decision=`not_established`; wakeup_path_blocking_authority=`not_implied`. If the question asks for a direct blocker, disclose that the typed target is unavailable instead of promoting a wakeup peer or adjacent blocking row.\n", label)
		case len(relations) == 0:
			fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target=`%s`; target_direct_blocking_authority=`not_provided_by_projection`; direct_blocking_decision=`not_established`; wakeup_path_blocking_authority=`not_implied`. If the question asks for a direct blocker, say that no typed direct blocker was established for this target. Describe wakeup edges as wakeup/dependency relations; do not promote a wakeup peer, IRQ peer, kernel caller, adjacent row, or another thread's blocking interval into the target's direct blocker.\n", label, target)
		default:
			for _, relation := range relations {
				fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target=`%s`; target_direct_blocking_authority=`typed_waiter_holder`; direct_blocking_decision=`established_by_typed_relation`; waiter=`%s`; holder=`%s`; blocking_kind=`%s`; row_identity=`%s`.\n",
					label, target, relation.waiter, relation.holder, relation.kind, relation.rowIdentity)
			}
		}

		leaders := traceFinalFixDirectionLeaders(projection, 6)
		if len(leaders) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- compact_authority artifact=`%s`: fix_direction_summary_authority=`single_published_leader_only`; direction_subtotal_authority=`not_provided_without_exact_fold`. Do not sum same-direction seats merely because their labels share a direction.\n", label)
		for _, node := range leaders {
			key, value, ok := traceDecisionModelFacingDirection(node)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "  - %s=`%s`", key, value)
			fmt.Fprintf(&b, "; leader_rank=#%d; leader_subject=`%s`; leader_effective_attribution=%.3fms; row_identity=`%s`",
				node.Rank, strings.TrimSpace(node.Subject), node.EffectiveImpactMS, traceDecisionNodeIdentity(node))
			if stateKind := strings.TrimSpace(node.StateKind); stateKind != "" {
				fmt.Fprintf(&b, "; leader_state_kind=`%s`", stateKind)
			}
			if measured := traceFinalMeasuredStateOccupancy(node); measured > 0 {
				fmt.Fprintf(&b, "; leader_measured_state_occupancy=%.3fms", measured)
			}
			if node.StartTs > 0 && node.EndTs > node.StartTs {
				fmt.Fprintf(&b, "; occurrence_interval=`%.6f..%.6f`", node.StartTs, node.EndTs)
			}
			if start, end, ok := traceDecisionNodeQueryWindow(node); ok {
				role := "supporting_query_window"
				if traceDecisionSameWindow(start, end, projection.WindowStartTs, projection.WindowEndTs) {
					role = "requested_or_elected_window"
				}
				fmt.Fprintf(&b, "; query_window=`%.6f..%.6f`; window_role=`%s`", start, end, role)
			}
			if traceDecisionNodePhase(node) == "pre_wakeup_dependency" {
				b.WriteString("; impact_phase=`pre_wakeup_dependency`; post_wakeup_delay_authority=`not_provided_by_this_seat`")
			}
			traceDecisionWritePriorityCandidateClaimEnvelope(&b, node)
			traceDecisionWriteNodeBlockingReasonAuthority(&b, node)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderTraceFinalSynthesisScope is the last, compact population/authority
// reminder before the model writes. It composes only typed projection lanes
// and frame status. It does not scan or rewrite prose and it does not choose a
// root cause; its job is to prevent a long evidence appendix from obscuring
// the already-established semantic ceiling.
func renderTraceFinalSynthesisScope(set types.TraceCausalProjectionSet, frameEvidenceStatus string) string {
	if len(set.Projections) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("- final_synthesis_scope: principal_root_cause_population=`typed_on_chain_only`; adjacent_and_background_role=`supporting_context_and_additional_investigation_only`; actual_occupancy_and_existing_rule_eliminable=`separate_decision_axes`.\n")
	seen := make(map[string]bool)
	emitted := 0
	for _, projection := range set.Projections {
		for _, node := range traceDecisionEliminableSeats(projection, 8) {
			if emitted >= 3 || !traceDecisionNodeIsPriorityInversionCandidate(node) {
				continue
			}
			key := traceDecisionNodeIdentity(node)
			if seen[key] {
				continue
			}
			seen[key] = true
			fmt.Fprintf(&b, "  - candidate_subject=`%s`; effective_attribution=%.3fms", strings.TrimSpace(node.Subject), node.EffectiveImpactMS)
			traceDecisionWritePriorityCandidateClaimEnvelope(&b, node)
			b.WriteString("\n")
			emitted++
		}
	}
	if frameEvidenceStatus == "absent" || frameEvidenceStatus == "unavailable" {
		fmt.Fprintf(&b, "  - frame_claim_scope=`selected_window_observations_only`; frame_evidence_status=`%s`; out_of_window_marker_role=`navigation_only`; frame_boundary_completion_deadline_authority=`not_provided`.\n", frameEvidenceStatus)
	}
	return b.String()
}

// renderTraceFinalStateValueAuthority surfaces only typed rows where the
// measured state duration and published effective attribution differ. The
// compact distinction is deliberately prompt-only: it gives the model exact
// caliber without inspecting or rewriting its prose. Rows are bounded and
// deduped by typed identity so exploratory duplicates cannot flood the tail.
func renderTraceFinalStateValueAuthority(set types.TraceCausalProjectionSet) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		pool := make([]types.TraceCausalProjectionNode, 0,
			len(projection.RankedSeats)+len(projection.OnChainCauses)+len(projection.BackgroundCauses))
		pool = append(pool, projection.RankedSeats...)
		pool = append(pool, projection.OnChainCauses...)
		pool = append(pool, projection.BackgroundCauses...)
		seen := map[string]bool{}
		emitted := 0
		for _, node := range pool {
			identity := traceDecisionNodeIdentity(node)
			if seen[identity] {
				continue
			}
			seen[identity] = true
			stateKind := strings.TrimSpace(node.StateKind)
			measured := traceFinalMeasuredStateOccupancy(node)
			effective := node.EffectiveImpactMS
			if stateKind == "" || measured <= 0 || effective <= 0 || math.Abs(measured-effective) < 0.0005 {
				continue
			}
			fmt.Fprintf(&b, "- state_value_authority artifact=`%s`; subject=`%s`; state_kind=`%s`; measured_state_occupancy=%.3fms; effective_attribution=%.3fms; relation=`distinct_do_not_substitute`; row_identity=`%s`",
				traceDecisionPromptScalar(label), traceDecisionPromptScalar(strings.TrimSpace(node.Subject)),
				traceDecisionPromptScalar(stateKind), measured, effective, traceDecisionPromptScalar(identity))
			if node.StartTs > 0 && node.EndTs > node.StartTs {
				fmt.Fprintf(&b, "; occurrence_interval=`%.6f..%.6f`", node.StartTs, node.EndTs)
			}
			b.WriteString("\n")
			emitted++
			if emitted >= 8 {
				break
			}
		}
	}
	return b.String()
}

func traceFinalMeasuredStateOccupancy(node types.TraceCausalProjectionNode) float64 {
	if node.CumulativeImpactMS > 0 {
		return node.CumulativeImpactMS
	}
	return node.ImpactMS
}

type traceFinalBlockingRelation struct {
	waiter      string
	holder      string
	kind        string
	rowIdentity string
}

func traceFinalTargetBlockingRelations(projection types.TraceCausalProjection, target string) []traceFinalBlockingRelation {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	pool := make([]types.TraceCausalProjectionNode, 0,
		len(projection.RankedSeats)+len(projection.PrimaryRootCauses)+len(projection.OnChainCauses)+
			len(projection.AdjacentCauses)+len(projection.BackgroundCauses)+len(projection.SupportingHops))
	pool = append(pool, projection.RankedSeats...)
	pool = append(pool, projection.PrimaryRootCauses...)
	pool = append(pool, projection.OnChainCauses...)
	pool = append(pool, projection.AdjacentCauses...)
	pool = append(pool, projection.BackgroundCauses...)
	pool = append(pool, projection.SupportingHops...)
	seen := map[string]bool{}
	out := make([]traceFinalBlockingRelation, 0, 2)
	for _, node := range pool {
		if strings.TrimSpace(node.BlockingKind) == "" ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		subject := strings.TrimSpace(node.Subject)
		peer := strings.TrimSpace(node.BlockingPeer)
		waiter, holder := "", ""
		switch {
		case !node.BlockingSubjectIsHolder && subject == target:
			waiter, holder = subject, peer
		case node.BlockingSubjectIsHolder && peer == target:
			waiter, holder = peer, subject
		default:
			continue
		}
		if holder == "" {
			holder = "unresolved"
		}
		identity := traceDecisionNodeIdentity(node)
		key := waiter + "\x00" + holder + "\x00" + strings.TrimSpace(node.BlockingKind) + "\x00" + identity
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, traceFinalBlockingRelation{
			waiter: waiter, holder: holder, kind: strings.TrimSpace(node.BlockingKind), rowIdentity: identity,
		})
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func traceFinalFixDirectionLeaders(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	seats := traceDecisionEliminableSeats(projection, 0)
	onChain := traceFinalOnChainSeatIdentities(projection)
	leaders := map[string]types.TraceCausalProjectionNode{}
	for _, node := range seats {
		direction := strings.TrimSpace(node.FixDirection)
		if direction == "" {
			continue
		}
		current, ok := leaders[direction]
		if !ok || traceFinalDirectionSeatBefore(node, current, onChain) {
			leaders[direction] = node
		}
	}
	out := make([]types.TraceCausalProjectionNode, 0, len(leaders))
	for _, node := range leaders {
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EffectiveImpactMS != out[j].EffectiveImpactMS {
			return out[i].EffectiveImpactMS > out[j].EffectiveImpactMS
		}
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		return traceDecisionNodeIdentity(out[i]) < traceDecisionNodeIdentity(out[j])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// traceFinalOnChainSeatIdentities mirrors the published eliminable board's
// authority boundary. Rank ordinals are local to a query board/channel, so an
// adjacent rank #1 cannot displace an on-chain rank #3 merely because its
// ordinal is smaller. Evidence identity is the join key; absence of an
// on-chain roster preserves the legacy all-seat fallback.
func traceFinalOnChainSeatIdentities(projection types.TraceCausalProjection) map[string]bool {
	out := map[string]bool{}
	if projection.PrimaryRootCause != nil {
		out[traceDecisionNodeIdentity(*projection.PrimaryRootCause)] = true
	}
	for _, nodes := range [][]types.TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.OnChainCauses} {
		for _, node := range nodes {
			out[traceDecisionNodeIdentity(node)] = true
		}
	}
	return out
}

func traceFinalDirectionSeatBefore(candidate, current types.TraceCausalProjectionNode, onChain map[string]bool) bool {
	if len(onChain) > 0 {
		candidateOnChain := onChain[traceDecisionNodeIdentity(candidate)]
		currentOnChain := onChain[traceDecisionNodeIdentity(current)]
		if candidateOnChain != currentOnChain {
			return candidateOnChain
		}
	}
	if candidate.EffectiveImpactMS != current.EffectiveImpactMS {
		return candidate.EffectiveImpactMS > current.EffectiveImpactMS
	}
	if candidate.Rank != current.Rank {
		return candidate.Rank < current.Rank
	}
	return traceDecisionNodeIdentity(candidate) < traceDecisionNodeIdentity(current)
}
