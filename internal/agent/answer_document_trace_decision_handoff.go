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
	authority := answerDocRuntimeTraceGuidanceView(ctx)
	// A generic runtime/log observation ledger can compile an empty projection
	// partition. Only an actual typed trace source may activate this trace-only
	// synthesis handoff; raw request or artifact prose never participates.
	if !authority.RuntimeTrace {
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
	var claims []types.AnswerRelationClaim
	if ctx.Mutable != nil {
		claims = ctx.Mutable.StableInvestigationRelationClaims()
	}
	return renderAnswerDocTraceDecisionHandoffSet(set, authority, claims)
}

func renderAnswerDocTraceDecisionHandoffSet(set types.TraceCausalProjectionSet, authority runtimeTraceGuidanceView, acceptedClaims ...[]types.AnswerRelationClaim) string {
	if len(set.Projections) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Trace Decision Inputs (Model Owns The Conclusion)\n\n")
	b.WriteString("- The deterministic system owns only the typed measurements, rank seats, wakeup paths, and evidence boundary below. You own the diagnosis and final recommendation. Do not present this handoff as a system-authored conclusion and do not merely repeat the rows.\n")
	hasActual, hasEliminable := traceDecisionAxesPresent(set)
	switch {
	case hasActual && hasEliminable:
		b.WriteString("- Write a concise synthesis before the detailed evidence. Compare the two distinct decision axes that are actually available (this distinction is not a claim of physical independence): (A) actual time occupancy / critical-path work, including high-cost work that current formulas do not price, to identify new optimization directions; and (B) existing-rule eliminable impact, to prioritize already-priced repairs. Explain why the leading direction matters and what to verify or change first.\n")
	case hasActual:
		b.WriteString("- Write a concise synthesis of the available actual time occupancy / critical-path work and the next optimization direction. No positive existing-rule eliminable seat is available here; do not invent one.\n")
	case hasEliminable:
		b.WriteString("- Write a concise synthesis of the available existing-rule eliminable seats and the first repair to validate. No independent typed actual-occupancy candidate is available here; do not invent one.\n")
	default:
		b.WriteString("- Synthesize only the available target-state, wakeup-path, and evidence-boundary inputs. Do not invent an occupancy or eliminable ranking that is absent below.\n")
	}
	b.WriteString("- Do not add values across arbitrary rows, fix directions, wall-clock and cpu·ms, or overlapping seats. Addition is authorized only by an exact typed additive carrier, such as the target's closed four-state partition or a same-source disjoint bipartition with its published subtotal. Mutually exclusive partition members may be added to reconstruct that exact partition total; do not misstate mutual exclusion as general non-additivity. A high occupancy is not automatically eliminable; a high eliminable estimate is not automatically proven frame causality.\n")
	b.WriteString("- relation_authority=`typed_pair_only`: different state names, metric families, fix directions, rows, or threads do not by themselves prove independence, containment, overlap, mutual exclusion, or additivity. State one of those pairwise relations only from an exact typed relation/fold carrier or the target's closed four-state partition. When a pair has no such carrier, say its physical relationship is unresolved and that cross-row addition is not authorized; do not upgrade missing relation evidence into the stronger physical claim that the rows are independent or intrinsically non-additive.\n")
	traceDecisionWriteRelationClaimHandoff(&b, set, acceptedClaims)
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
		b.WriteString("- typed_relation_semantics: consume each row's exact relation fields before comparing values. A `row_state_breakdown` describes only that observation's own state accounting; it does not establish containment, overlap, or identity with another row. `physical_overlap` rows share measured wall clock and are not additive. `resource_completion_closure` connects a completion path to an anchored wait but does not make the completion thread a resource holder. `sched_blocked_reason.caller` is a kernel-reported wait call-site/symbol, not a resource or lock owner; holder language requires a separate typed holder relation.\n")
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
			fmt.Fprintf(&b, "- target_state_symptom: subject=`%s`; running=%.3fms; runnable=%.3fms; sleep=%.3fms; d_state=%.3fms; io_wait=%.3fms; total=%.3fms; partition_relation=`mutually_exclusive_and_additive_to_total`; partition_addition_authority=`these_five_members_only`. This describes what the target experienced, not the upstream cause or recoverable amount.\n",
				account.Subject, account.RunningMS, account.RunnableMS, account.SleepMS,
				account.DStateMS, account.IOWaitMS, account.TotalMS)
			b.WriteString("- selected_window_value_authority: when describing the target's state or duration inside `selected_window`, copy the typed `target_state_symptom` values above. Whole-attachment extent, a switch-in after the selected-window end, and pre-triage navigation hypotheses are different calibers and must not replace this selected-window value. This is reasoning guidance only; you still own the conclusion and wording.\n")
		}
		traceDecisionWriteSelfRunnableTwoRulerFacts(&b, projection.SelfRunnableTwoRulerAccountings)
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
			b.WriteString("- axis_B_existing_rule_eliminable (ordered typed seats; cross_row_additivity=`not_authorized_without_exact_pair_carrier`):\n")
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
		contextRows := traceDecisionNonCausalContextRows(projection, 6)
		if len(contextRows) > 0 {
			b.WriteString("- contextual_noncausal_rows (these typed rows may constrain absence claims, but they are not target-causal proof and are not additive to either decision axis):\n")
			for _, row := range contextRows {
				node := row.node
				unit := strings.TrimSpace(node.Unit)
				if unit == "" {
					unit = "ms"
				}
				caliber := "context_observation"
				if node.IsAggregateMetric() {
					caliber = "aggregate_context_non_target_wall_clock"
				}
				fmt.Fprintf(&b, "  - lane=`%s`; subject=`%s`; kind=`%s`; value=%.3f; unit=`%s`; caliber=`%s`; target_causal_authority=`not_provided`; cross_axis_addition=`forbidden`; source_lane=`%s`",
					row.lane, strings.TrimSpace(node.Subject), traceDecisionNodeKind(node), node.ImpactMS,
					unit, caliber, traceDecisionNodeSourceLane(node))
				traceDecisionWriteNodeIdentity(&b, node)
				traceDecisionWriteNodeRelations(&b, node)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func traceDecisionWriteRelationClaimHandoff(b *strings.Builder, set types.TraceCausalProjectionSet, acceptedClaims [][]types.AnswerRelationClaim) {
	if b == nil {
		return
	}
	authorities := types.CompileTraceAnswerRelationAuthorities(set)
	requiredCount := 0
	for _, authority := range authorities {
		if !authority.RequiredForClosure {
			continue
		}
		requiredCount++
		members := authority.MemberRefs
		if authority.Kind == types.AnswerRelationAuthorityCrossRulerBoundary {
			members = append(append([]string(nil), authority.LeftMemberRefs...), authority.RightMemberRefs...)
		}
		fmt.Fprintf(b, "- typed_relation_authority: authority_id=`%s`; member_refs=`%s`; physical_relation=`%s`; addition=`%s`",
			authority.ID, strings.Join(members, ","), authority.PhysicalRelation, authority.Addition)
		if authority.SubtotalValue != nil {
			fmt.Fprintf(b, "; subtotal_value=%.3f; subtotal_unit=`%s`", *authority.SubtotalValue, authority.SubtotalUnit)
		}
		b.WriteString(".\n")
	}
	if requiredCount > 0 {
		b.WriteString("- final_relation_claim_carrier: the typed_relation_authority rows above are precise decision inputs, not a format-only copy obligation. Keep your visible relation explanation consistent. If you choose to publish structured relation metadata, place only the exact authority object on `blocks[i].relation_claims`, never at document-level `$.relation_claims`; submitted metadata is validated, but omitting this optional carrier does not trigger a retry. Deterministic checks never rewrite prose.\n")
	}
	if len(acceptedClaims) == 0 || len(acceptedClaims[0]) == 0 {
		return
	}
	currentClaims, supersededClaims := types.PartitionAnswerRelationClaimsByCurrentAuthorities(acceptedClaims[0], authorities)
	if len(supersededClaims) > 0 {
		fmt.Fprintf(b, "- accepted_model_relation_claims_superseded: count=%d. Later typed evidence replaced these investigation-time declarations; do not copy them into the final document. Use the final typed_relation_authority objects above and revise your own visible conclusion if their values differ. The system does not rewrite your prose.\n", len(supersededClaims))
	}
	if len(currentClaims) == 0 {
		return
	}
	b.WriteString("- accepted_model_relation_claims: these declarations were authored by the investigation model and already accepted against the typed authorities. Use them as decision context and keep your visible conclusion consistent; you do not need to duplicate them in the final document. If you choose to publish `blocks[i].relation_claims`, submitted metadata must remain exact. The system will reject an invalid submitted claim but will not rewrite your prose.\n")
	for _, raw := range currentClaims {
		claim := types.NormalizeAnswerRelationClaim(raw)
		fmt.Fprintf(b, "  - authority_id=`%s`; member_refs=`%s`; physical_relation=`%s`; addition=`%s`",
			claim.AuthorityID, strings.Join(claim.MemberRefs, ","), claim.PhysicalRelation, claim.Addition)
		if claim.SubtotalValue != nil {
			fmt.Fprintf(b, "; subtotal_value=%.3f; subtotal_unit=`%s`", *claim.SubtotalValue, claim.SubtotalUnit)
		}
		b.WriteString("\n")
	}
}

func traceDecisionWriteSelfRunnableTwoRulerFacts(b *strings.Builder, records []types.TraceCausalProjectionSelfRunnableTwoRuler) {
	if b == nil {
		return
	}
	for index, record := range records {
		if index >= 4 || !types.TraceCausalProjectionSelfRunnableTwoRulerValid(record) {
			continue
		}
		fmt.Fprintf(b, "- authorized_relation_fact: family=`self_runnable_two_ruler`; subject=`%s`; self_wall_clock_seats=`%s`; self_wall_clock_subtotal=%.3fms; wakeup_edge_seats=`%s`; wakeup_edge_subtotal=%.3fms; same_ruler_addition=`authorized_to_published_subtotal`; cross_ruler_addition=`forbidden`; cross_ruler_physical_relation=`unresolved`. The two subtotals use different accounting rulers and must not be combined.\n",
			strings.TrimSpace(record.Subject), traceDecisionRulerSeats(record.WallRanks, record.WallEffsMS), record.WallSubtotalMS,
			traceDecisionRulerSeats(record.EdgeRanks, record.EdgeEffsMS), record.EdgeSubtotalMS)
	}
}

func traceDecisionRulerSeats(ranks []int, values []float64) string {
	parts := make([]string, 0, len(ranks))
	for i := range ranks {
		if i >= len(values) {
			break
		}
		parts = append(parts, fmt.Sprintf("#%d:%.3fms", ranks[i], values[i]))
	}
	return strings.Join(parts, ",")
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
		// DStateSplitMS/IOWaitSplitMS are row-local state-accounting fields.
		// They become a cross-row containment pointer only after the projection
		// renderer's exact pair adjudication (same subject/state family, value
		// identity, unique peer and compatible window). Do not mint that relation
		// here from the split alone: doing so gives the answer model stronger
		// authority than the deterministic user-visible projection actually has.
		fmt.Fprintf(b, "; row_state_breakdown=`d_state:%.3fms,io_wait:%.3fms`; state_breakdown_scope=`this_observation_only`; cross_row_relation_authority=`not_provided_by_state_breakdown`",
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

type traceDecisionContextRow struct {
	lane string
	node types.TraceCausalProjectionNode
}

// traceDecisionNonCausalContextRows carries the bounded adjacent/background
// rows that the deterministic projection will publish after finalization. The
// answer model needs these rows to avoid denying evidence that the system will
// later display, while their typed lane keeps them below root-cause and
// eliminable authority. No request or answer prose participates.
func traceDecisionNonCausalContextRows(projection types.TraceCausalProjection, limit int) []traceDecisionContextRow {
	pool := make([]traceDecisionContextRow, 0, len(projection.AdjacentCauses)+len(projection.BackgroundCauses))
	for _, node := range projection.AdjacentCauses {
		pool = append(pool, traceDecisionContextRow{lane: "adjacent", node: node})
	}
	for _, node := range projection.BackgroundCauses {
		pool = append(pool, traceDecisionContextRow{lane: "background", node: node})
	}
	seen := map[string]bool{}
	out := make([]traceDecisionContextRow, 0, len(pool))
	for _, row := range pool {
		node := row.node
		if node.ImpactMS <= 0 || node.IsEvidenceBoundaryRow() ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		key := row.lane + "\x00" + traceDecisionNodeIdentity(node)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].lane != out[j].lane {
			return out[i].lane < out[j].lane
		}
		if out[i].node.ImpactMS != out[j].node.ImpactMS {
			return out[i].node.ImpactMS > out[j].node.ImpactMS
		}
		return traceDecisionNodeIdentity(out[i].node) < traceDecisionNodeIdentity(out[j].node)
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
