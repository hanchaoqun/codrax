package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocTraceDecisionHandoff gives the answer-writing model a compact,
// high-salience view of the same typed projection that is materialized after
// finalize. The system owns the measurements and causal ceiling; the model
// still owns diagnosis, prioritization, and user-facing conclusions.
//
// This is deliberately prompt-only. It neither creates an AnswerBlock nor
// inspects/replaces model prose, and no answer hard gate consumes it.
func renderAnswerDocTraceDecisionHandoff(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	set := types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))
	var requestModel *types.RequestModel
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	// Reuse the exact publication authority consumed by the post-finalize
	// projection materializer. A bounded fact request must not be widened into
	// causal synthesis merely because exploration happened to collect a causal
	// row; explicit typed windows and causal/relation scopes remain authorized.
	if len(set.Projections) == 0 || !types.RuntimeTraceReportMaterializationAllowed(requestModel, set) {
		return ""
	}
	return renderAnswerDocTraceDecisionHandoffSet(set, answerDocRuntimeTraceGuidanceView(ctx))
}

func renderAnswerDocTraceDecisionHandoffSet(set types.TraceCausalProjectionSet, authority runtimeTraceGuidanceView) string {
	if len(set.Projections) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Trace Decision Inputs (Model Owns The Conclusion)\n\n")
	b.WriteString("- The deterministic system owns only the typed measurements, rank seats, wakeup paths, and evidence boundary below. You own the diagnosis and final recommendation. Do not present this handoff as a system-authored conclusion and do not merely repeat the rows.\n")
	hasActual, hasEliminable := traceDecisionAxesPresent(set)
	switch {
	case hasActual && hasEliminable:
		b.WriteString("- Write a concise synthesis before the detailed evidence. Compare the two independent decision axes that are actually available: (A) actual time occupancy / critical-path work, including high-cost work that current formulas do not price, to identify new optimization directions; and (B) existing-rule eliminable impact, to prioritize already-priced repairs. Explain why the leading direction matters and what to verify or change first.\n")
	case hasActual:
		b.WriteString("- Write a concise synthesis of the available actual time occupancy / critical-path work and the next optimization direction. No positive existing-rule eliminable seat is available here; do not invent one.\n")
	case hasEliminable:
		b.WriteString("- Write a concise synthesis of the available existing-rule eliminable seats and the first repair to validate. No independent typed actual-occupancy candidate is available here; do not invent one.\n")
	default:
		b.WriteString("- Synthesize only the available target-state, wakeup-path, and evidence-boundary inputs. Do not invent an occupancy or eliminable ranking that is absent below.\n")
	}
	b.WriteString("- Never add values across rows, fix directions, wall-clock and cpu·ms, or overlapping seats. A high occupancy is not automatically eliminable; a high eliminable estimate is not automatically proven frame causality.\n")
	if authority.CausalUnproven {
		b.WriteString("- causal_conclusion=`unproven`: keep the synthesis useful but calibrated as the strongest candidate / first validation direction, not a proven dropped-frame cause.\n")
	}
	if authority.FrameEvidenceStatus != "" {
		fmt.Fprintf(&b, "- frame_evidence_status=`%s`: no stronger frame/deadline attribution may be invented.\n", authority.FrameEvidenceStatus)
	}
	if traceDecisionHasPreWakeupDependency(set) {
		b.WriteString("- phase_semantics: `pre_wakeup_dependency` measures an upstream thread while its downstream consumer has not yet been woken. It may explain the consumer's sleep/blocked interval, but it is not the consumer's post-wakeup runnable/dispatch delay. Attribute post-wakeup delay only from the consumer's own typed runnable interval plus same-CPU scheduler ordering; never infer that a CFS dependency preempted an RT consumer after wake from this seat.\n")
		b.WriteString("- A typed `priority_inversion_candidate` on that phase prices the dependency's own proven-lower runnable/running supply before the downstream wake. The candidate flag alone proves neither a lock holder/waiter relation nor post-wakeup preemption; treat PI-mutex or RT-promotion changes as validation directions unless separate typed evidence proves the corresponding mechanism.\n")
	}
	if traceDecisionHasEvidenceBoundary(set) {
		b.WriteString("- evidence_boundary_semantics: `missing_wakeup` means the selected trace/query window contains no matching `sched_wakeup` row for a measured sleep interval. Preserve the interval as a target-state symptom and a chain-drill coverage boundary; it does not prove that a physical wakeup was absent, does not identify a blocker, and owns no positive causal/eliminable amount. Window boundaries, event coverage/loss, or an unrepresented wake source remain possible until separately typed evidence resolves them.\n")
	}
	if traceDecisionHasAccountOrRoleRelations(set) {
		b.WriteString("- typed_relation_semantics: consume each row's exact relation fields before comparing values. `embedded_components` are already inside their parent row; `physical_overlap` rows share measured wall clock; neither relation is additive. `resource_completion_closure` connects a completion path to an anchored wait but does not make the completion thread a resource holder. `sched_blocked_reason.caller` is a kernel-reported wait call-site/symbol, not a resource or lock owner; holder language requires a separate typed holder relation.\n")
	}
	b.WriteString("- Input provenance: the projection compiler merges accepted exploration observations with deterministic system-supplement observations when present; each candidate below preserves its own source lane.\n\n")

	for index, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		fmt.Fprintf(&b, "### Decision input: `%s`\n\n", label)
		if types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			fmt.Fprintf(&b, "- selected_window=`%.6f..%.6f`; window_ms=%.3f\n",
				projection.WindowStartTs, projection.WindowEndTs, projection.WindowDurationMS())
		}
		if account := projection.TargetStateAccount; account != nil && strings.TrimSpace(account.Subject) != "" && account.TotalMS > 0 {
			fmt.Fprintf(&b, "- target_state_symptom: subject=`%s`; running=%.3fms; runnable=%.3fms; sleep=%.3fms; d_state=%.3fms; io_wait=%.3fms; total=%.3fms. This describes what the target experienced, not the upstream cause or recoverable amount.\n",
				account.Subject, account.RunningMS, account.RunnableMS, account.SleepMS,
				account.DStateMS, account.IOWaitMS, account.TotalMS)
		}
		if len(projection.WakeupPath) > 0 {
			fmt.Fprintf(&b, "- elected_wakeup_path=`%s`; use it to connect observations, but do not upgrade the path itself beyond the typed causal ceiling.\n",
				strings.Join(projection.WakeupPath, " -> "))
			b.WriteString("  - wakeup_path_semantics: an edge `A -> B` proves the typed wakeup/dependency relation carried by that path. By itself it does not prove that B synchronously blocked waiting for A, that A held B's lock/resource, or that every hop formed one continuously blocking call chain. Use stronger blocked-wait/holder wording only when a separate typed blocking or holder relation provides that authority.\n")
		}
		for _, node := range traceDecisionEvidenceBoundaries(projection, 8) {
			fmt.Fprintf(&b, "- evidence_boundary: subject=`%s`; kind=`%s`; status=`no_matching_sched_wakeup_row_in_selected_window`; positive_blocker_authority=`not_provided`; causal_identity=`unresolved`",
				strings.TrimSpace(node.Subject), traceDecisionNodeKind(node))
			if node.EndTs > node.StartTs {
				fmt.Fprintf(&b, "; observed_sleep_interval=`%.6f..%.6f`; interval_ms=%.3f",
					node.StartTs, node.EndTs, (node.EndTs-node.StartTs)*1000)
			} else if node.ImpactMS > 0 {
				fmt.Fprintf(&b, "; observed_sleep_interval=`unavailable`; interval_ms=%.3f", node.ImpactMS)
			}
			fmt.Fprintf(&b, "; source_lane=`%s`\n", traceDecisionNodeSourceLane(node))
		}

		actual := traceDecisionActualOccupancyCandidates(projection, 8)
		if len(actual) > 0 || len(projection.BusinessSpanMentions) > 0 {
			b.WriteString("- axis_A_actual_occupancy_candidates (not a conclusion; compare only compatible calibers):\n")
			for _, node := range actual {
				fmt.Fprintf(&b, "  - subject=`%s`; kind=`%s`; window_projection=%.3fms",
					strings.TrimSpace(node.Subject), traceDecisionNodeKind(node), node.ImpactMS)
				if node.CumulativeImpactMS > 0 && node.CumulativeImpactMS != node.ImpactMS {
					fmt.Fprintf(&b, "; chain_total=%.3fms", node.CumulativeImpactMS)
				}
				if count, maxMS := traceDecisionNodeMultiplicity(node); count > 1 {
					fmt.Fprintf(&b, "; occurrences=%d; member_max=%.3fms", count, maxMS)
				}
				if node.LineStart > 0 {
					fmt.Fprintf(&b, "; lines=%d..%d", node.LineStart, maxInt(node.LineStart, node.LineEnd))
				}
				fmt.Fprintf(&b, "; source_lane=`%s`", traceDecisionNodeSourceLane(node))
				traceDecisionWriteNodeIdentity(&b, node)
				traceDecisionWritePhase(&b, node)
				traceDecisionWriteNodeRelations(&b, node)
				b.WriteString("\n")
			}
			business := traceDecisionBusinessSpanCandidates(projection.BusinessSpanMentions, 5)
			for _, span := range business {
				fmt.Fprintf(&b, "  - subject=`%s`; kind=`business_span_family`; span=`%s`; total=%.3fms; occurrences=%d; member_max=%.3fms; lines=%d..%d; basis=`%s`; source_lane=`projection_side_channel`\n",
					span.Subject, span.Name, span.TotalMS, span.Count, span.MaxMS,
					span.StartLine, maxInt(span.StartLine, span.EndLine), span.Basis)
			}
		}

		seats := traceDecisionEliminableSeats(projection, 8)
		if len(seats) > 0 {
			b.WriteString("- axis_B_existing_rule_eliminable (ordered typed seats; cross_row_additivity=`forbidden`):\n")
			for _, node := range seats {
				fmt.Fprintf(&b, "  - rank=#%d; subject=`%s`; kind=`%s`; effective_attribution=%.3fms",
					node.Rank, strings.TrimSpace(node.Subject), traceDecisionNodeKind(node), node.EffectiveImpactMS)
				if node.FixDirection != "" {
					fmt.Fprintf(&b, "; fix_direction=`%s`", node.FixDirection)
				}
				if node.ImpactMS > 0 && node.ImpactMS != node.EffectiveImpactMS {
					fmt.Fprintf(&b, "; window_projection=%.3fms", node.ImpactMS)
				}
				fmt.Fprintf(&b, "; source_lane=`%s`", traceDecisionNodeSourceLane(node))
				traceDecisionWriteNodeIdentity(&b, node)
				traceDecisionWritePhase(&b, node)
				if node.PriorityInversionCandidate && traceDecisionNodePhase(node) == "pre_wakeup_dependency" {
					b.WriteString("; priority_candidate_scope=`dependency_scheduler_supply`; post_wakeup_preemption_authority=`not_provided_by_this_seat`; holder_waiter_authority=`not_provided_by_candidate_flag`")
				}
				traceDecisionWriteNodeRelations(&b, node)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func traceDecisionHasAccountOrRoleRelations(set types.TraceCausalProjectionSet) bool {
	for _, projection := range set.Projections {
		for _, lane := range [][]types.TraceCausalProjectionNode{
			projection.RankedSeats,
			projection.PrimaryRootCauses,
			projection.OnChainCauses,
			projection.SemanticSpans,
		} {
			for _, node := range lane {
				if len(node.CrossDirectionOverlaps) > 0 || node.DStateSplitMS > 0 ||
					node.IOWaitSplitMS > 0 || node.ResourceCompletionClosure ||
					strings.TrimSpace(node.BlockedReasonCaller) != "" ||
					strings.TrimSpace(node.BlockingKind) != "" {
					return true
				}
			}
		}
	}
	return false
}

func traceDecisionWriteNodeIdentity(b *strings.Builder, node types.TraceCausalProjectionNode) {
	if b == nil {
		return
	}
	if evidenceID := strings.TrimSpace(node.EvidenceID); evidenceID != "" {
		fmt.Fprintf(b, "; row_identity=`%s`", evidenceID)
	}
}

// traceDecisionWriteNodeRelations exposes already-compiled typed relations to
// the answer-writing model. It never derives a relation from prose, subject
// names, or approximate values, and it never rejects or rewrites an answer.
func traceDecisionWriteNodeRelations(b *strings.Builder, node types.TraceCausalProjectionNode) {
	if b == nil {
		return
	}
	if node.DStateSplitMS > 0 || node.IOWaitSplitMS > 0 {
		fmt.Fprintf(b, "; embedded_components=`d_state:%.3fms,io_wait:%.3fms`; component_relation=`already_inside_parent_row`; addition_with_parent=`forbidden`",
			node.DStateSplitMS, node.IOWaitSplitMS)
	}
	for index, overlap := range node.CrossDirectionOverlaps {
		if index >= 3 || overlap.OverlapMS <= 0 || overlap.LineStart <= 0 || overlap.LineEnd < overlap.LineStart {
			continue
		}
		fmt.Fprintf(b, "; physical_overlap_%d=`%.3fms@lines:%d..%d`; peer_fix_direction=`%s`; overlap_basis=`%s`; overlap_addition=`forbidden`",
			index+1, overlap.OverlapMS, overlap.LineStart, overlap.LineEnd,
			strings.TrimSpace(overlap.Direction), strings.TrimSpace(overlap.Basis))
	}
	if node.ResourceCompletionClosure {
		b.WriteString("; completion_relation=`resource_completion_closure_for_anchored_wait`; completion_thread_holder_authority=`not_provided`")
	}
	if caller := strings.TrimSpace(node.BlockedReasonCaller); caller != "" {
		fmt.Fprintf(b, "; blocked_reason_caller=`%s`; caller_role=`kernel_reported_wait_callsite`; holder_authority=`not_provided_by_caller`", caller)
	}
	if strings.TrimSpace(node.BlockingKind) == "" {
		return
	}
	peer := strings.TrimSpace(node.BlockingPeer)
	if node.BlockingSubjectIsHolder {
		b.WriteString("; subject_lock_role=`typed_holder`")
		if peer != "" {
			fmt.Fprintf(b, "; blocked_waiter=`%s`", peer)
		}
		return
	}
	b.WriteString("; subject_lock_role=`blocked_waiter`")
	if peer != "" {
		fmt.Fprintf(b, "; typed_lock_holder=`%s`", peer)
	}
}

func traceDecisionHasEvidenceBoundary(set types.TraceCausalProjectionSet) bool {
	for _, projection := range set.Projections {
		if len(traceDecisionEvidenceBoundaries(projection, 1)) > 0 {
			return true
		}
	}
	return false
}

func traceDecisionEvidenceBoundaries(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
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
	out := make([]types.TraceCausalProjectionNode, 0, len(pool))
	for _, node := range pool {
		if !node.IsEvidenceBoundaryRow() ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		key := fmt.Sprintf("%s\x00%s\x00%.9f\x00%.9f\x00%.6f\x00%d\x00%d",
			strings.TrimSpace(node.Subject), traceDecisionNodeKind(node),
			node.StartTs, node.EndTs, node.ImpactMS, node.LineStart, node.LineEnd)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartTs != out[j].StartTs {
			return out[i].StartTs < out[j].StartTs
		}
		return traceDecisionNodeIdentity(out[i]) < traceDecisionNodeIdentity(out[j])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func traceDecisionHasPreWakeupDependency(set types.TraceCausalProjectionSet) bool {
	for _, projection := range set.Projections {
		for _, lane := range [][]types.TraceCausalProjectionNode{
			projection.RankedSeats,
			projection.PrimaryRootCauses,
			projection.OnChainCauses,
			projection.SemanticSpans,
		} {
			for _, node := range lane {
				if traceDecisionNodePhase(node) == "pre_wakeup_dependency" {
					return true
				}
			}
		}
	}
	return false
}

// traceDecisionNodePhase is a prompt-only semantic projection of the engine's
// typed chain depth. A positive depth is minted only for an upstream chain
// member measured in the edge-closed interval before it wakes its downstream
// consumer. The zero/absent lane stays unknown: legacy records and target
// depth zero share that wire shape, so absence must not guess a phase.
func traceDecisionNodePhase(node types.TraceCausalProjectionNode) string {
	if node.ChainDepth > 0 {
		return "pre_wakeup_dependency"
	}
	return ""
}

func traceDecisionWritePhase(b *strings.Builder, node types.TraceCausalProjectionNode) {
	if b == nil {
		return
	}
	if phase := traceDecisionNodePhase(node); phase != "" {
		fmt.Fprintf(b, "; impact_phase=`%s`", phase)
	}
}

func traceDecisionAxesPresent(set types.TraceCausalProjectionSet) (actual, eliminable bool) {
	for _, projection := range set.Projections {
		if len(traceDecisionActualOccupancyCandidates(projection, 1)) > 0 ||
			len(traceDecisionBusinessSpanCandidates(projection.BusinessSpanMentions, 1)) > 0 {
			actual = true
		}
		if len(traceDecisionEliminableSeats(projection, 1)) > 0 {
			eliminable = true
		}
		if actual && eliminable {
			return true, true
		}
	}
	return actual, eliminable
}

func traceDecisionActualOccupancyCandidates(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	pool := make([]types.TraceCausalProjectionNode, 0,
		len(projection.PrimaryRootCauses)+len(projection.OnChainCauses)+len(projection.SemanticSpans))
	pool = append(pool, projection.PrimaryRootCauses...)
	pool = append(pool, projection.OnChainCauses...)
	pool = append(pool, projection.SemanticSpans...)
	seen := map[string]bool{}
	out := make([]types.TraceCausalProjectionNode, 0, len(pool))
	for _, node := range pool {
		if node.ImpactMS <= 0 || node.IsTargetSelfStateRow() || node.IsAggregateMetric() ||
			node.OnChainOverflowFold || node.Unit == types.TraceObservationUnitCompositeScore ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		// Axis A is time actually spent in a typed state/span. A priced
		// composite seat without an underlying state/span belongs only to axis B.
		if strings.TrimSpace(node.StateKind) == "" && strings.TrimSpace(node.SemanticClass) == "" &&
			strings.TrimSpace(node.SpanName) == "" {
			continue
		}
		key := traceDecisionNodeIdentity(node)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ImpactMS != out[j].ImpactMS {
			return out[i].ImpactMS > out[j].ImpactMS
		}
		return traceDecisionNodeIdentity(out[i]) < traceDecisionNodeIdentity(out[j])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func traceDecisionEliminableSeats(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	pool := append([]types.TraceCausalProjectionNode(nil), projection.RankedSeats...)
	if len(pool) == 0 {
		pool = append(pool, projection.PrimaryRootCauses...)
		pool = append(pool, projection.OnChainCauses...)
	}
	seen := map[string]bool{}
	out := make([]types.TraceCausalProjectionNode, 0, len(pool))
	for _, node := range pool {
		if node.Rank <= 0 || node.EffectiveImpactMS <= 0 || node.IsTargetSelfStateRow() ||
			node.IsAggregateMetric() || node.OnChainOverflowFold ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		key := fmt.Sprintf("%d\x00%s", node.Rank, traceDecisionNodeIdentity(node))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		if out[i].EffectiveImpactMS != out[j].EffectiveImpactMS {
			return out[i].EffectiveImpactMS > out[j].EffectiveImpactMS
		}
		return traceDecisionNodeIdentity(out[i]) < traceDecisionNodeIdentity(out[j])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func traceDecisionBusinessSpanCandidates(in []types.TraceCausalProjectionBusinessSpanMention, limit int) []types.TraceCausalProjectionBusinessSpanMention {
	out := make([]types.TraceCausalProjectionBusinessSpanMention, 0, len(in))
	for _, span := range in {
		if strings.TrimSpace(span.Subject) == "" || strings.TrimSpace(span.Name) == "" ||
			span.Count <= 0 || span.TotalMS <= 0 || span.MaxMS <= 0 {
			continue
		}
		out = append(out, span)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TotalMS != out[j].TotalMS {
			return out[i].TotalMS > out[j].TotalMS
		}
		return out[i].Subject+"\x00"+out[i].Name < out[j].Subject+"\x00"+out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func traceDecisionNodeIdentity(node types.TraceCausalProjectionNode) string {
	if id := strings.TrimSpace(node.EvidenceID); id != "" {
		return id
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%.6f\x00%.6f\x00%.6f",
		strings.TrimSpace(node.Subject), strings.TrimSpace(node.Predicate),
		strings.TrimSpace(node.Object), node.ImpactMS, node.StartTs, node.EndTs)
}

func traceDecisionNodeKind(node types.TraceCausalProjectionNode) string {
	for _, value := range []string{node.SemanticClass, node.StateKind, node.Object, node.Predicate} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "runtime_candidate"
}

func traceDecisionNodeMultiplicity(node types.TraceCausalProjectionNode) (int, float64) {
	if node.FamilyMemberCount > 1 {
		maxMS := node.FamilyMemberMaxMS
		if maxMS <= 0 {
			maxMS = node.ImpactMS
		}
		return node.FamilyMemberCount, maxMS
	}
	if node.MergedCount > 1 {
		maxMS := node.MergedMaxMS
		if maxMS <= 0 {
			maxMS = node.ImpactMS
		}
		return node.MergedCount, maxMS
	}
	return 1, node.ImpactMS
}

func traceDecisionNodeSourceLane(node types.TraceCausalProjectionNode) string {
	if node.SystemSupplement {
		return "deterministic_system_supplement"
	}
	return "accepted_exploration"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
