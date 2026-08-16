package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
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
	ledger := answerDocObservationLedger(ctx)
	set := types.CompileTraceCausalProjectionSet(ledger)
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
	return renderAnswerDocTraceDecisionHandoffSetWithAggregateFacts(
		set, authority, traceDecisionTypedAggregateFacts(ledger.Records), claims,
	)
}

func renderAnswerDocTraceDecisionHandoffSet(set types.TraceCausalProjectionSet, authority runtimeTraceGuidanceView, acceptedClaims ...[]types.AnswerRelationClaim) string {
	return renderAnswerDocTraceDecisionHandoffSetWithAggregateFacts(set, authority, nil, acceptedClaims...)
}

func renderAnswerDocTraceDecisionHandoffSetWithAggregateFacts(set types.TraceCausalProjectionSet, authority runtimeTraceGuidanceView, aggregateFacts []traceDecisionAggregateFact, acceptedClaims ...[]types.AnswerRelationClaim) string {
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
		b.WriteString("- Exhaustive-decomposition ceiling: describe a target wait, selected window, or elected path duration as fully explained/composed by listed Axis B seats only when one exact typed additive carrier publishes that same subtotal. Otherwise keep the Axis A occupancy/path duration and each Axis B seat separate; describe remaining on-chain work as unpriced or unresolved instead of treating the arithmetic remainder as zero. Do not compute a residual by adding or subtracting overlapping rows unless a typed partition authorizes that arithmetic.\n")
	case hasActual:
		b.WriteString("- Write a concise synthesis of the available actual time occupancy / critical-path work and the next optimization direction. No positive existing-rule eliminable seat is available here; do not invent one.\n")
	case hasEliminable:
		b.WriteString("- Write a concise synthesis of the available existing-rule eliminable seats and the first repair to validate. No separately bound typed actual-occupancy candidate is available here; do not invent one.\n")
	default:
		b.WriteString("- Synthesize only the available target-state, wakeup-path, and evidence-boundary inputs. Do not invent an occupancy or eliminable ranking that is absent below.\n")
	}
	b.WriteString("- Do not add values across arbitrary rows, fix directions, wall-clock and cpu·ms, or overlapping seats. Addition is authorized only by an exact typed additive carrier, such as the target's closed engine-state partition or a same-source disjoint bipartition with its published subtotal. Mutually exclusive partition members may be added to reconstruct that exact partition total; do not misstate mutual exclusion as general non-additivity. A high occupancy is not automatically eliminable; a high eliminable estimate is not automatically proven frame causality.\n")
	b.WriteString("- relation_authority=`typed_pair_only`: different state names, metric families, fix directions, rows, or threads do not by themselves prove independence, containment, overlap, mutual exclusion, or additivity. State one of those pairwise relations only from an exact typed relation/fold carrier or the target's closed engine-state partition. When a pair has no such carrier, say its physical relationship is unresolved and that cross-row addition is not authorized; do not upgrade missing relation evidence into the stronger physical claim that the rows are independent or intrinsically non-additive.\n")
	b.WriteString("- compact_unknowns: evidence_absence_implication=`unknown_not_false`; target_direct_blocking_not_established_does_not_prove_no_external_blocking=`true`; cross_direction_physical_relation=`unresolved_unless_an_exact_pair_row_says_otherwise`; absent_overlap_record_proves_independence=`false`; cause_decomposition_status=`not_closed_by_state_partition_or_ranked_seat_roster`; exhaustive_cause_wording=`requires_one_exact_typed_additive_cause_partition`. Keep separately published values separate and useful, but do not turn an unestablished typed mechanism into a claim that the mechanism is physically absent, summarize an unknown relation as independent/no-overlap, or summarize the listed mechanisms as all causes.\n")
	traceDecisionWriteRepairDirectionAuthority(&b, set)
	traceDecisionWriteRelationClaimHandoff(&b, set, acceptedClaims)
	if authority.CausalUnproven {
		b.WriteString("- causal_conclusion=`unproven`: keep the synthesis useful but calibrated as the strongest candidate / first validation direction, not a proven dropped-frame cause.\n")
	}
	if authority.FrameEvidenceStatus != "" {
		fmt.Fprintf(&b, "- frame_evidence_status=`%s`: no stronger frame/deadline attribution may be invented.\n", authority.FrameEvidenceStatus)
		b.WriteString(renderTraceFrameEvidenceStatusSemantics(authority.FrameEvidenceStatus))
	}
	if traceDecisionHasPreWakeupDependency(set) {
		b.WriteString("- phase_semantics: `pre_wakeup_dependency` is upstream on-chain work overlapping the downstream consumer's pre-wakeup interval. This seat is a ranked work candidate; by itself it does not prove that the consumer waited for this work, waited until it completed, or was directly blocked by it. Use direct-blocker or completion-dependency wording only when a separate typed holder/waiter or blocking relation provides that authority. This seat owns no post-wakeup runnable/dispatch delay; attribute that delay only from the consumer's own typed runnable interval plus same-CPU scheduler ordering.\n")
		b.WriteString("- A typed `priority_inversion_candidate` on that phase prices the dependency's own proven-lower runnable/running supply before the downstream wake. The candidate flag alone proves neither a lock holder/waiter relation nor post-wakeup preemption; treat PI-mutex or RT-promotion changes as validation directions unless separate typed evidence proves the corresponding mechanism.\n")
	}
	if traceDecisionHasEvidenceBoundary(set) {
		b.WriteString("- evidence_boundary_semantics: `missing_wakeup` means the selected trace/query window contains no matching `sched_wakeup` row for a measured sleep interval. Preserve the interval as a target-state symptom and a chain-drill coverage boundary; it does not prove that a physical wakeup was absent, does not identify a blocker, and owns no positive causal/eliminable amount. Window boundaries, event coverage/loss, or an unrepresented wake source remain possible until separately typed evidence resolves them.\n")
	}
	if traceDecisionHasAccountOrRoleRelations(set) {
		b.WriteString("- typed_relation_semantics: consume each row's exact relation fields before comparing values. A `row_state_breakdown` describes only that observation's own state accounting; it does not establish containment, overlap, or identity with another row. `physical_overlap` rows share measured wall clock and are not additive. `resource_completion_closure` connects a completion path to an anchored wait but does not make the completion thread a resource holder. `sched_blocked_reason.caller` is a kernel-reported wait call-site/symbol, not a resource or lock owner; holder language requires a separate typed holder relation.\n")
	}
	b.WriteString("- Input provenance: the projection compiler merges accepted exploration observations with deterministic system-supplement observations when present; each candidate below preserves its own source lane.\n\n")
	traceDecisionWriteTypedAggregateFacts(&b, aggregateFacts)

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
			b.WriteString("  - sleep_cause_authority=`not_provided_by_target_state_account`; zero_d_state_or_iowait_does_not_classify_sleep_reason=`true`. The state partition alone cannot classify S-sleep as normal frame pacing, lock/condition waiting, IPC, timer/event waiting, or another cause; use a typed blocking, wakeup, span, or frame relation before assigning that mechanism.\n")
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
		semanticSpans := traceDecisionSemanticOptimizationSpans(projection, 0)
		if len(semanticSpans) > 0 {
			b.WriteString("- deterministic_semantic_spans (typed in-window work inventory; not automatically target-thread work, causal, or eliminable):\n")
			for _, node := range semanticSpans {
				fmt.Fprintf(&b, "  - subject=`%s`; semantic_class=`%s`; span=`%s`; total=%.3fms",
					strings.TrimSpace(node.Subject), strings.TrimSpace(node.SemanticClass), strings.TrimSpace(node.SpanName), node.ImpactMS)
				if count, maxMS := traceDecisionNodeMultiplicity(node); count > 1 {
					fmt.Fprintf(&b, "; occurrences=%d; member_max=%.3fms", count, maxMS)
				}
				if node.LineStart > 0 {
					fmt.Fprintf(&b, "; lines=%d..%d", node.LineStart, maxInt(node.LineStart, node.LineEnd))
				}
				fmt.Fprintf(&b, "; source_lane=`%s`\n", traceDecisionNodeSourceLane(node))
				traceDecisionWriteSemanticMembers(&b, node, 8)
			}
			b.WriteString("  This inventory is a dedicated visibility lane: state separately whether a span ran on the selected target, another on-chain thread, or only a process/window peer. Its presence neither proves an effect on the target nor proves no effect; the relationship stays unresolved without a typed relation. Its absence must not be inferred from a target-thread-only keyword search when this typed inventory is non-empty. Every member row is a distinct typed span: copy that member's own span name, duration, and line range instead of reusing the family representative's name.\n")
		}

		seats := traceDecisionEliminableSeats(projection, 8)
		if len(seats) > 0 {
			b.WriteString("- axis_B_existing_rule_eliminable (ordered typed seats; cross_row_additivity=`not_authorized_without_exact_pair_carrier`):\n")
			for _, node := range seats {
				fmt.Fprintf(&b, "  - rank=#%d; subject=`%s`; kind=`%s`; effective_attribution=%.3fms",
					node.Rank, strings.TrimSpace(node.Subject), traceDecisionEliminableSeatKind(node), node.EffectiveImpactMS)
				if relationRef := types.TraceAnswerRelationMemberRef(node); relationRef != "" {
					fmt.Fprintf(&b, "; relation_member_ref=`%s`", relationRef)
				}
				traceDecisionWriteModelFacingDirection(&b, node)
				if node.ImpactMS > 0 && node.ImpactMS != node.EffectiveImpactMS {
					fmt.Fprintf(&b, "; window_projection=%.3fms", node.ImpactMS)
				}
				fmt.Fprintf(&b, "; source_lane=`%s`", traceDecisionNodeSourceLane(node))
				traceDecisionWriteNodeIdentity(&b, node)
				traceDecisionWritePhase(&b, node)
				traceDecisionWritePriorityCandidateClaimEnvelope(&b, node)
				traceDecisionWriteNodeRelations(&b, node)
				b.WriteString("\n")
			}
		}
		contextRows := traceDecisionNonCausalContextRows(projection, 6)
		if len(contextRows) > 0 {
			b.WriteString("- contextual_noncausal_rows (these typed rows may constrain absence claims, but they are not target-causal proof and are not additive to either decision axis):\n")
			for _, row := range contextRows {
				node := row.node
				subject := strings.TrimSpace(node.Subject)
				if node.IsAggregateMetric() && !types.TraceCausalProjectionKnownSubject(subject) {
					subject = traceDecisionNodeKind(node)
				}
				unit := strings.TrimSpace(node.Unit)
				if unit == "" {
					unit = "ms"
				}
				caliber := "context_observation"
				if node.IsAggregateMetric() {
					caliber = "aggregate_context_non_target_wall_clock"
				}
				fmt.Fprintf(&b, "  - lane=`%s`; subject=`%s`; kind=`%s`; value=%.3f; unit=`%s`; caliber=`%s`; target_causal_authority=`not_provided`; cross_axis_addition=`forbidden`; source_lane=`%s`",
					row.lane, subject, traceDecisionNodeKind(node), node.ImpactMS,
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

// traceDecisionWriteRepairDirectionAuthority is the compact, single-source
// interpretation of the detailed Axis-B roster. It gives the model the exact
// value role needed for a repair-direction answer without asking it to infer
// arithmetic or physical relations from a long list of seats. This is typed
// prompt context only: it does not inspect prose, reject an answer, or
// materialize a conclusion.
func traceDecisionWriteRepairDirectionAuthority(b *strings.Builder, set types.TraceCausalProjectionSet) {
	if b == nil {
		return
	}
	type directionRecord struct {
		key, value string
		direction  string
		leader     types.TraceCausalProjectionNode
		count      int
	}
	for projectionIndex, projection := range set.Projections {
		seats := traceDecisionEliminableSeats(projection, 0)
		if len(seats) == 0 {
			continue
		}
		byDirection := map[string]directionRecord{}
		for _, node := range seats {
			key, value, ok := traceDecisionModelFacingDirection(node)
			if !ok {
				continue
			}
			identity := key + "\x00" + value
			record := byDirection[identity]
			record.key, record.value, record.direction, record.count = key, value, strings.TrimSpace(node.FixDirection), record.count+1
			if record.leader.Rank == 0 || node.EffectiveImpactMS > record.leader.EffectiveImpactMS ||
				(node.EffectiveImpactMS == record.leader.EffectiveImpactMS && node.Rank < record.leader.Rank) {
				record.leader = node
			}
			byDirection[identity] = record
		}
		if len(byDirection) == 0 {
			continue
		}
		sectionByDirection := map[string]types.TraceAnswerDirectionSection{}
		for _, section := range tool.TraceAnswerDecisionDirectionSections(projection) {
			sectionByDirection[section.Direction] = section
		}
		records := make([]directionRecord, 0, len(byDirection))
		for _, record := range byDirection {
			records = append(records, record)
		}
		sort.SliceStable(records, func(i, j int) bool {
			if records[i].leader.EffectiveImpactMS != records[j].leader.EffectiveImpactMS {
				return records[i].leader.EffectiveImpactMS > records[j].leader.EffectiveImpactMS
			}
			return records[i].value < records[j].value
		})
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", projectionIndex+1)
		}
		fmt.Fprintf(b, "- repair_direction_authority: artifact=`%s`; value_role=`exact_typed_direction_subtotal_when_published_else_single_leader`; joint_total_authority=`not_provided`; unlisted_pair_physical_relation=`unresolved`; direction_independence_authority=`not_provided`; direction_overlap_authority=`exact_physical_overlap_rows_only`; instruction=`do_not_sum_across_directions_or_unlisted_members`.\n", label)
		traceDecisionWriteRepairDirectionRelationRoster(b, projection, label, 8)
		traceDecisionWriteRepairDirectionPresentationPlan(b, projection, label, 8, 12)
		for _, record := range records {
			section, sectionOK := sectionByDirection[record.direction]
			leader := record.leader
			if sectionOK && section.Leader.Rank > 0 {
				leader = section.Leader
			}
			fmt.Fprintf(b, "  - %s=`%s`; member_count=%d; leader_rank=#%d; leader_subject=`%s`; leader_value=%.3fms",
				record.key, record.value, record.count, leader.Rank,
				strings.TrimSpace(leader.Subject), leader.EffectiveImpactMS)
			switch {
			case sectionOK && section.Arithmetic == types.TraceAnswerDirectionArithmeticSubtotal:
				fmt.Fprintf(b, "; same_direction_subtotal_authority=`typed_pairwise_disjoint_section`; published_direction_value=`exact_subtotal`; direction_subtotal=%.3fms; subtotal_member_count=%d",
					section.SubtotalMS, len(section.Members))
				if len(section.MemberRefs) == len(section.Members) {
					fmt.Fprintf(b, "; subtotal_member_refs=`%s`", strings.Join(section.MemberRefs, ","))
				}
			case sectionOK && section.Arithmetic == types.TraceAnswerDirectionArithmeticOverlap:
				b.WriteString("; same_direction_subtotal_authority=`forbidden_by_typed_overlap`; published_direction_value=`leader_only`")
			default:
				b.WriteString("; same_direction_subtotal_authority=`not_provided`; published_direction_value=`leader_only`")
			}
			switch {
			case traceDecisionNodeIsPriorityInversionCandidate(leader):
				b.WriteString("; mechanism_boundary=`lower_priority_dependency_supply_only`; lock_holder_or_priority_inheritance_need=`unproven_without_typed_relation`")
			case strings.TrimSpace(leader.FixDirection) == "frequency_thermal":
				b.WriteString("; mechanism_boundary=`compute_supply_opportunity`; compute_supply_value_role=`frequency_relative_headroom_against_published_ideal_basis`; compute_supply_value_proves_lower_frequency_cause=`false`; compute_supply_value_proves_governance_binding=`false`; policy_ceiling_proves_thermal_throttling_or_actual_binding=`false`")
			case strings.TrimSpace(leader.FixDirection) == "io_dependency":
				b.WriteString("; mechanism_boundary=`typed_io_or_kernel_wait_seat`; kernel_callsite_proves_resource_or_holder=`false`")
			}
			b.WriteString("\n")
		}
	}
}

// traceDecisionRepairDirectionRelation is one compact reasoning relation made
// only from typed projection carriers. It is deliberately prompt-only: the
// model remains responsible for deciding what the relation means for the
// customer's workload and for writing the conclusion.
type traceDecisionRepairDirectionRelation struct {
	scope      string
	directionA string
	directionB string
	memberRefs []string
	physical   string
	addition   string
	valueMS    float64
	valueRole  string
	reasoning  string
}

// traceDecisionRepairDirectionRelations compacts the two exact relationship
// carriers that are otherwise easy to miss in a long Trace handoff:
//   - a same-direction section whose shared arithmetic predicate proved every
//     published member pair disjoint and minted the exact subtotal; and
//   - a reciprocal cross-direction overlap whose two ranked seats resolve by
//     exact board, subject, line envelope and direction identity.
//
// Missing, one-sided, ambiguous, cross-board, or stale carriers mint no row.
// No request text, model prose, similarity or label heuristic participates.
func traceDecisionRepairDirectionRelations(projection types.TraceCausalProjection) []traceDecisionRepairDirectionRelation {
	var out []traceDecisionRepairDirectionRelation
	for _, section := range tool.TraceAnswerDecisionDirectionSections(projection) {
		if section.Arithmetic != types.TraceAnswerDirectionArithmeticSubtotal ||
			section.SubtotalMS <= 0 || len(section.MemberRefs) < 2 ||
			len(section.MemberRefs) != len(section.Members) {
			continue
		}
		out = append(out, traceDecisionRepairDirectionRelation{
			scope:      "same_direction",
			directionA: strings.TrimSpace(section.Direction),
			memberRefs: append([]string(nil), section.MemberRefs...),
			physical:   types.AnswerPhysicalRelationMutuallyExclusive,
			addition:   types.AnswerRelationAdditionAuthorized,
			valueMS:    section.SubtotalMS,
			valueRole:  "published_direction_subtotal",
			reasoning:  "separate_nonoverlapping_contributions_not_overlap_or_competition",
		})
	}

	seats := traceDecisionEliminableSeats(projection, 0)
	type overlapCandidate struct {
		left, right types.TraceCausalProjectionNode
		overlapMS   float64
	}
	var overlaps []overlapCandidate
	seenPairs := map[string]bool{}
	for _, left := range seats {
		leftRef := types.TraceAnswerRelationMemberRef(left)
		if leftRef == "" || strings.TrimSpace(left.FixDirection) == "" {
			continue
		}
		for _, wire := range left.CrossDirectionOverlaps {
			if wire.OverlapMS <= 0 || wire.LineStart <= 0 || wire.LineEnd < wire.LineStart {
				continue
			}
			var partner *types.TraceCausalProjectionNode
			for index := range seats {
				candidate := seats[index]
				if traceDecisionNodeIdentity(candidate) == traceDecisionNodeIdentity(left) ||
					candidate.LineStart != wire.LineStart || candidate.LineEnd != wire.LineEnd ||
					!strings.EqualFold(strings.TrimSpace(candidate.Subject), strings.TrimSpace(left.Subject)) ||
					strings.TrimSpace(candidate.FixDirection) != strings.TrimSpace(wire.Direction) ||
					!traceDecisionSameRankBoard(left, candidate) ||
					types.TraceAnswerRelationMemberRef(candidate) == "" {
					continue
				}
				if partner != nil {
					partner = nil // exact typed identity is ambiguous: fail closed
					break
				}
				copy := candidate
				partner = &copy
			}
			if partner == nil || strings.TrimSpace(partner.FixDirection) == strings.TrimSpace(left.FixDirection) ||
				!traceDecisionHasReciprocalDirectionOverlap(*partner, left, wire.OverlapMS) {
				continue
			}
			rightRef := types.TraceAnswerRelationMemberRef(*partner)
			pairKey := leftRef + "\x00" + rightRef
			if rightRef < leftRef {
				pairKey = rightRef + "\x00" + leftRef
			}
			if seenPairs[pairKey] {
				continue
			}
			seenPairs[pairKey] = true
			overlaps = append(overlaps, overlapCandidate{left: left, right: *partner, overlapMS: wire.OverlapMS})
		}
	}
	sort.SliceStable(overlaps, func(i, j int) bool {
		if overlaps[i].left.Rank != overlaps[j].left.Rank {
			return overlaps[i].left.Rank < overlaps[j].left.Rank
		}
		return overlaps[i].right.Rank < overlaps[j].right.Rank
	})
	for _, overlap := range overlaps {
		leftRef := types.TraceAnswerRelationMemberRef(overlap.left)
		rightRef := types.TraceAnswerRelationMemberRef(overlap.right)
		out = append(out, traceDecisionRepairDirectionRelation{
			scope:      "cross_direction",
			directionA: strings.TrimSpace(overlap.left.FixDirection),
			directionB: strings.TrimSpace(overlap.right.FixDirection),
			memberRefs: []string{leftRef, rightRef},
			physical:   types.AnswerPhysicalRelationOverlap,
			addition:   types.AnswerRelationAdditionForbidden,
			valueMS:    overlap.overlapMS,
			valueRole:  "measured_physical_overlap",
			reasoning:  "shared_physical_time_only_at_the_published_overlap",
		})
	}
	return out
}

func traceDecisionSameRankBoard(left, right types.TraceCausalProjectionNode) bool {
	leftStart, leftEnd, leftOK := traceDecisionNodeQueryWindow(left)
	rightStart, rightEnd, rightOK := traceDecisionNodeQueryWindow(right)
	if !leftOK || !rightOK || !traceDecisionSameWindow(leftStart, leftEnd, rightStart, rightEnd) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(left.RankBoardTarget), strings.TrimSpace(right.RankBoardTarget)) &&
		strings.TrimSpace(left.RankBoardParamsFingerprint) == strings.TrimSpace(right.RankBoardParamsFingerprint)
}

func traceDecisionHasReciprocalDirectionOverlap(from, to types.TraceCausalProjectionNode, overlapMS float64) bool {
	for _, wire := range from.CrossDirectionOverlaps {
		if wire.LineStart == to.LineStart && wire.LineEnd == to.LineEnd &&
			strings.TrimSpace(wire.Direction) == strings.TrimSpace(to.FixDirection) &&
			math.Abs(wire.OverlapMS-overlapMS) <= 0.0005 {
			return true
		}
	}
	return false
}

// traceDecisionWriteRepairDirectionRelationRoster is shared by the detailed
// decision handoff and the final compact boundary so those two prompt faces
// cannot disagree. The cap controls context size; omitted rows remain in the
// lossless typed projection and are explicitly counted.
func traceDecisionWriteRepairDirectionRelationRoster(b *strings.Builder, projection types.TraceCausalProjection, artifact string, limit int) {
	if b == nil {
		return
	}
	relations := traceDecisionRepairDirectionRelations(projection)
	total := len(relations)
	emitted := total
	if limit > 0 && emitted > limit {
		emitted = limit
	}
	if len(traceDecisionEliminableSeats(projection, 0)) < 2 {
		return
	}
	fmt.Fprintf(b, "  - repair_direction_relation_roster: artifact=`%s`; emitted=%d; total=%d; complete=`%t`; source=`typed_projection_relations_only`.\n",
		artifact, emitted, total, emitted == total)
	for _, relation := range relations[:emitted] {
		fmt.Fprintf(b, "    - relation_scope=`%s`; direction_a=`%s`", relation.scope, relation.directionA)
		if relation.directionB != "" {
			fmt.Fprintf(b, "; direction_b=`%s`", relation.directionB)
		}
		fmt.Fprintf(b, "; member_refs=`%s`; physical_relation=`%s`; addition=`%s`; %s=%.3fms; reasoning_boundary=`%s`\n",
			strings.Join(relation.memberRefs, ","), relation.physical, relation.addition,
			relation.valueRole, relation.valueMS, relation.reasoning)
	}
	b.WriteString("    - relation_scope=`unlisted_pairs`; physical_relation=`unresolved`; addition=`forbidden_without_exact_typed_carrier`; independence=`not_authorized`; dependency=`not_authorized`; temporal_order=`not_authorized`.\n")
}

// traceDecisionWriteRepairDirectionPresentationPlan turns the exact direction
// arithmetic decision into a bounded authoring slate. It exists because a
// relation roster and a much larger seat list are easy to recombine
// incorrectly: only the section's exact member refs may participate in its
// published subtotal, while every other member remains a useful standalone
// value with unresolved pairwise arithmetic.
//
// The plan is prompt-only. It neither creates an answer row nor inspects or
// repairs model prose. All identities and values come from the same projection
// and direction-section compiler already used by the deterministic appendix.
func traceDecisionWriteRepairDirectionPresentationPlan(b *strings.Builder, projection types.TraceCausalProjection, artifact string, directionLimit, memberLimit int) {
	if b == nil {
		return
	}
	type directionPlan struct {
		direction string
		key       string
		value     string
		leader    types.TraceCausalProjectionNode
		members   []types.TraceCausalProjectionNode
	}
	byDirection := map[string]directionPlan{}
	for _, node := range traceDecisionEliminableSeats(projection, 0) {
		key, value, ok := traceDecisionModelFacingDirection(node)
		direction := strings.TrimSpace(node.FixDirection)
		if !ok || direction == "" {
			continue
		}
		plan := byDirection[direction]
		plan.direction, plan.key, plan.value = direction, key, value
		plan.members = append(plan.members, node)
		if plan.leader.Rank == 0 || node.EffectiveImpactMS > plan.leader.EffectiveImpactMS ||
			(node.EffectiveImpactMS == plan.leader.EffectiveImpactMS && node.Rank < plan.leader.Rank) {
			plan.leader = node
		}
		byDirection[direction] = plan
	}
	if len(byDirection) == 0 {
		return
	}
	sections := map[string]types.TraceAnswerDirectionSection{}
	for _, section := range tool.TraceAnswerDecisionDirectionSections(projection) {
		sections[strings.TrimSpace(section.Direction)] = section
	}
	plans := make([]directionPlan, 0, len(byDirection))
	for _, plan := range byDirection {
		sort.SliceStable(plan.members, func(i, j int) bool {
			if plan.members[i].Rank != plan.members[j].Rank {
				return plan.members[i].Rank < plan.members[j].Rank
			}
			return plan.members[i].EffectiveImpactMS > plan.members[j].EffectiveImpactMS
		})
		plans = append(plans, plan)
	}
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].leader.EffectiveImpactMS != plans[j].leader.EffectiveImpactMS {
			return plans[i].leader.EffectiveImpactMS > plans[j].leader.EffectiveImpactMS
		}
		return plans[i].direction < plans[j].direction
	})
	total := len(plans)
	emitted := total
	if directionLimit > 0 && emitted > directionLimit {
		emitted = directionLimit
	}
	fmt.Fprintf(b, "  - repair_direction_presentation_plan: artifact=`%s`; emitted=%d; total=%d; complete=`%t`; source=`same_typed_direction_sections_and_ranked_seats`; metadata_not_user_copy=true.\n",
		artifact, emitted, total, emitted == total)
	for _, plan := range plans[:emitted] {
		section, sectionOK := sections[plan.direction]
		headline := plan.leader
		headlineRole := "single_leader"
		headlineValue := headline.EffectiveImpactMS
		var headlineRefs []string
		if ref := types.TraceAnswerRelationMemberRef(headline); ref != "" {
			headlineRefs = []string{ref}
		}
		if sectionOK && section.Arithmetic == types.TraceAnswerDirectionArithmeticSubtotal &&
			section.SubtotalMS > 0 && len(section.MemberRefs) >= 2 && len(section.MemberRefs) == len(section.Members) {
			headlineRole = "exact_typed_subtotal"
			headlineValue = section.SubtotalMS
			headlineRefs = append([]string(nil), section.MemberRefs...)
		}
		headlineSet := make(map[string]bool, len(headlineRefs))
		for _, ref := range headlineRefs {
			headlineSet[ref] = true
		}
		var additionalRefs []string
		membersWithoutRef := 0
		for _, member := range plan.members {
			ref := types.TraceAnswerRelationMemberRef(member)
			if ref == "" {
				membersWithoutRef++
				continue
			}
			if !headlineSet[ref] {
				additionalRefs = append(additionalRefs, ref)
			}
		}
		additionalTotal := len(additionalRefs)
		additionalEmitted := additionalTotal
		if memberLimit > 0 && additionalEmitted > memberLimit {
			additionalEmitted = memberLimit
		}
		fmt.Fprintf(b, "    - direction=`%s`; %s=`%s`; member_count=%d; headline_value_role=`%s`; headline_value=%.3fms; headline_member_refs=`%s`; additional_unresolved_member_refs=`%s`; additional_members_emitted=%d; additional_members_total=%d; additional_members_complete=`%t`; members_without_stable_ref=%d; display_contract=`headline_arithmetic_applies_only_to_headline_member_refs|list_additional_members_as_separate_values|never_plus_join_additional_members|pairwise_relation_unresolved_unless_relation_roster_lists_it|independence_not_authorized`.\n",
			plan.direction, plan.key, plan.value, len(plan.members), headlineRole, headlineValue,
			strings.Join(headlineRefs, ","), strings.Join(additionalRefs[:additionalEmitted], ","),
			additionalEmitted, additionalTotal, additionalEmitted == additionalTotal, membersWithoutRef)
	}
}

// renderTraceFrameEvidenceStatusSemantics is the single prompt source for the
// finite frame-evidence status vocabulary. It is soft reasoning context only:
// no validator scans model prose for these words and no system block uses it
// to author a frame verdict.
func renderTraceFrameEvidenceStatusSemantics(status string) string {
	switch strings.TrimSpace(status) {
	case "absent":
		return "- frame_evidence_status_semantics=`no target-bound frame/deadline evidence was produced in the selected evidence`; this proves neither that a frame drop occurred nor that no frame drop occurred. Do not state either frame-outcome verdict without separate typed frame/deadline evidence.\n"
	case "unavailable":
		return "- frame_evidence_status_semantics=`frame/deadline evidence could not be evaluated on the available coverage`; this proves neither that a frame drop occurred nor that no frame drop occurred. Disclose the coverage limit and do not state either frame-outcome verdict without separate typed frame/deadline evidence.\n"
	default:
		return ""
	}
}

// traceDecisionAggregateFact is a prompt-only carrier for an independently
// measured window aggregate. It deliberately does not become a projection
// node: adding model context must not change root-cause populations, ranks,
// folds, deterministic answer blocks, or the model's conclusion.
type traceDecisionAggregateFact struct {
	Artifact      string
	Kind          string
	Signal        string
	Value         float64
	Unit          string
	WindowStart   float64
	WindowEnd     float64
	EvidenceID    string
	SystemDerived bool
	Calibration   [][2]string
}

// traceDecisionTypedAggregateFacts admits only the producer's exact typed
// aggregate identity/window/value contract. Raw request text, model prose,
// summaries, labels and fuzzy matching are absent from the decision.
func traceDecisionTypedAggregateFacts(records []types.ObservationRecord) []traceDecisionAggregateFact {
	var out []traceDecisionAggregateFact
	seen := map[string]bool{}
	for _, record := range records {
		if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.GroundingPolicy != types.ClaimGroundingHard ||
			strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, types.TraceNoteKeySubjectKind)) != types.TraceCausalSubjectKindAggregateMetric ||
			strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, types.TraceNoteKeyChainRelevance)) != "background" ||
			strings.HasPrefix(strings.TrimSpace(record.Predicate), "root_cause_") ||
			strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, types.TraceNoteKeyRank)) != "" {
			continue
		}
		kind := strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, types.TraceNoteKeyType))
		unit := strings.TrimSpace(record.Unit)
		value, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64)
		windowStart, windowEnd, windowOK := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
		if kind == "" || unit == "" || err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) || !windowOK {
			continue
		}
		artifact := strings.TrimSpace(record.SourceRef.CaptureIdentityPath)
		if artifact == "" {
			artifact = strings.TrimSpace(record.SourceRef.Path)
		}
		if artifact == "" {
			artifact = strings.TrimSpace(record.SourceRef.ArtifactID)
		}
		key := fmt.Sprintf("%s\x00%.6f\x00%.6f\x00%s\x00%s\x00%.9g\x00%s", artifact, windowStart, windowEnd, kind, strings.TrimSpace(record.Predicate), value, unit)
		if seen[key] {
			continue
		}
		seen[key] = true
		fact := traceDecisionAggregateFact{
			Artifact: artifact, Kind: kind, Signal: strings.TrimSpace(record.Predicate),
			Value: value, Unit: unit, WindowStart: windowStart, WindowEnd: windowEnd,
			EvidenceID: record.ID, SystemDerived: record.SystemSupplement,
		}
		for _, calibrationKey := range []string{
			types.TraceNoteKeyPressureDensity,
			types.TraceNoteKeyWindowMS,
			types.TraceNoteKeyIOPressureEvidenceQuality,
			types.TraceNoteKeyIOPressureScoreCaliber,
			"absolute_level",
			"comparison_scope",
			"score_breakdown",
		} {
			if calibrationValue := strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, calibrationKey)); calibrationValue != "" {
				fact.Calibration = append(fact.Calibration, [2]string{calibrationKey, calibrationValue})
			}
		}
		out = append(out, fact)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Artifact != out[j].Artifact {
			return out[i].Artifact < out[j].Artifact
		}
		if out[i].WindowStart != out[j].WindowStart {
			return out[i].WindowStart < out[j].WindowStart
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Signal < out[j].Signal
	})
	return out
}

func traceDecisionWriteTypedAggregateFacts(b *strings.Builder, facts []traceDecisionAggregateFact) {
	if b == nil || len(facts) == 0 {
		return
	}
	const limit = 8
	emitted := len(facts)
	if emitted > limit {
		emitted = limit
	}
	fmt.Fprintf(b, "- typed_window_aggregate_context (background measurements for synthesis only; target_causal_authority=`not_provided`; cross_axis_addition=`forbidden`; emitted=%d; total=%d; complete=`%t`):\n", emitted, len(facts), emitted == len(facts))
	for _, fact := range facts[:emitted] {
		fmt.Fprintf(b, "  - artifact=`%s`; selected_window=`%.6f..%.6f`; kind=`%s`; signal=`%s`; value=%.3f; unit=`%s`; evidence_id=`%s`; source_lane=`%s`",
			traceDecisionPromptScalar(fact.Artifact), fact.WindowStart, fact.WindowEnd,
			traceDecisionPromptScalar(fact.Kind), traceDecisionPromptScalar(fact.Signal),
			fact.Value, traceDecisionPromptScalar(fact.Unit), traceDecisionPromptScalar(fact.EvidenceID),
			map[bool]string{true: "system_supplement", false: "model_exploration"}[fact.SystemDerived])
		for _, calibration := range fact.Calibration {
			fmt.Fprintf(b, "; %s=`%s`", traceDecisionPromptScalar(calibration[0]), traceDecisionPromptScalar(calibration[1]))
		}
		b.WriteString("\n")
	}
	b.WriteString("  These facts describe window-level pressure/caliber, not a target-thread cause or a recoverable amount. Use their typed calibration when explaining scale; do not infer severity from the raw number alone when no calibration field is present.\n\n")
}

func traceDecisionRichNoteValue(notes []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	if prefix == "=" {
		return ""
	}
	for _, note := range notes {
		if strings.HasPrefix(note, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(note, prefix))
		}
	}
	return ""
}

func traceDecisionPromptScalar(value string) string {
	value = strings.Join(strings.Fields(strings.ReplaceAll(value, "`", "'")), " ")
	const maxRunes = 240
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes]) + "…"
	}
	return value
}

func traceDecisionWriteRelationClaimHandoff(b *strings.Builder, set types.TraceCausalProjectionSet, acceptedClaims [][]types.AnswerRelationClaim) {
	if b == nil {
		return
	}
	authorities := types.CompileTraceAnswerRelationAuthorities(set)
	authorityCount := 0
	for _, authority := range authorities {
		authorityCount++
		claim := types.AnswerRelationClaimForAuthority(authority)
		copyJSON, err := json.Marshal(claim)
		if err != nil {
			continue
		}
		fmt.Fprintf(b, "- typed_relation_authority: relation_claim_copy=`%s`; copy_policy=`optional_prefer_omit`; typed_authority_auto_carried=`true`", copyJSON)
		members := claim.MemberRefs
		if authority.Kind == types.AnswerRelationAuthorityOverlappingMembers &&
			len(authority.MemberValuesMS) == len(members) {
			valueParts := make([]string, 0, len(members))
			for index, member := range members {
				valueParts = append(valueParts, fmt.Sprintf("%s:%.3fms", member, authority.MemberValuesMS[index]))
			}
			fmt.Fprintf(b, ".\n  relation_diagnostic_only: not_json_claim_fields=`true`; member_values=`%s`; fix_direction=`%s`; chain_lane=`%s`; members_independent=`false`",
				strings.Join(valueParts, ","), authority.FixDirection, authority.ChainLane)
			if authority.MeasuredOverlapMS != nil {
				fmt.Fprintf(b, "; measured_envelope_overlap=%.3fms", *authority.MeasuredOverlapMS)
			}
			if authority.ComparisonRule != "" {
				fmt.Fprintf(b, "; comparison_rule=`%s`", authority.ComparisonRule)
			}
			if authority.ComparisonValueMS != nil {
				fmt.Fprintf(b, "; comparison_value=%.3fms", *authority.ComparisonValueMS)
			}
			b.WriteString("; arithmetic_instruction=`preserve_each_member_but_never_publish_their_sum_as_a_total_or_eliminable_amount`")
		}
		b.WriteString(".\n")
	}
	if authorityCount > 0 {
		b.WriteString("- final_relation_claim_carrier: the typed authorities above are precise decision inputs and are already carried automatically. Prefer omitting optional `blocks[i].relation_claims`. If structured metadata is useful, copy only one complete `relation_claim_copy` JSON object; never copy any `relation_diagnostic_only` field, and never put claims at document-level `$.relation_claims`. Submitted metadata is validated, but omission does not trigger a retry. Deterministic checks never rewrite prose.\n")
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
	traceDecisionWriteNodeBlockingReasonAuthority(b, node)
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

// traceDecisionWriteNodeBlockingReasonAuthority keeps kernel wait-callsite
// evidence local to the exact projection seat that carries it. A sibling seat
// may share the same subject and state family while representing the honest
// unproven remainder; subject/name proximity never transfers a caller.
func traceDecisionWriteNodeBlockingReasonAuthority(b *strings.Builder, node types.TraceCausalProjectionNode) {
	if b == nil {
		return
	}
	if node.DStateCauseUnprovenRemainder {
		b.WriteString("; blocking_reason_authority=`not_provided_by_this_seat`; blocked_reason_caller=`not_provided`; sibling_caller_transfer=`forbidden`; allowed_mechanism_scope=`measured_state_occupancy_with_unknown_blocking_reason`; not_authorized_mechanisms=`sibling_caller,irq_or_storage_cause,resource_or_holder_identity,cross_row_delay`")
		if node.BlockedReasonWindowCount > 0 {
			fmt.Fprintf(b, "; window_blocked_reason_records=%d; window_record_binding_to_this_seat=`not_provided`", node.BlockedReasonWindowCount)
		}
		return
	}
	if caller := strings.TrimSpace(node.BlockedReasonCaller); caller != "" {
		fmt.Fprintf(b, "; blocked_reason_caller=`%s`; caller_role=`kernel_reported_wait_callsite`; caller_scope=`this_seat_only`; sibling_caller_transfer=`forbidden`; holder_authority=`not_provided_by_caller`; allowed_mechanism_scope=`kernel_reported_wait_callsite_for_this_seat`; not_authorized_mechanisms=`sibling_or_cross_row_cause,holder_identity,resource_identity`", caller)
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
	out = traceDecisionPreferProjectionWindowNodes(projection, out)
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
		if strings.TrimSpace(node.BlockingKind) == "" {
			b.WriteString("; mechanism_ceiling=`on_chain_prewakeup_work_candidate_only`; target_wait_for_work_authority=`not_provided_by_this_seat`; work_completion_dependency_authority=`not_provided_by_this_seat`; direct_blocking_authority=`not_provided_by_this_seat`")
		}
		b.WriteString("; post_wakeup_delay_authority=`not_provided_by_this_seat`")
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
	out = traceDecisionPreferProjectionWindowNodes(projection, out)
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

// traceDecisionSemanticOptimizationSpans keeps typed deterministic semantic
// work visible independently of the generic Axis-A top-N. A small JIT/class
// verification/GC family can otherwise disappear behind longer scheduler and
// business-span rows even when the user asks for that exact work class. This
// is prompt-only typed transport: it neither promotes the span onto the
// elected wakeup chain nor authors a diagnosis.
func traceDecisionSemanticOptimizationSpans(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	out := make([]types.TraceCausalProjectionNode, 0, len(projection.SemanticSpans))
	seen := map[string]bool{}
	for _, node := range projection.SemanticSpans {
		if node.ImpactMS <= 0 || strings.TrimSpace(node.SemanticClass) == "" ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		key := traceDecisionSemanticInventoryIdentity(node)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	out = traceDecisionPreferProjectionWindowNodes(projection, out)
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

func traceDecisionSemanticInventoryIdentity(node types.TraceCausalProjectionNode) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%.6f\x00%d\x00%d\x00%d\x00",
		strings.TrimSpace(node.Subject), strings.TrimSpace(node.SemanticClass), strings.TrimSpace(node.SpanName),
		node.ImpactMS, node.LineStart, node.LineEnd, node.FamilyMemberCount)
	for index, member := range node.FamilyMemberRoster {
		fmt.Fprintf(&b, "%d:%s\x01", index, strings.TrimSpace(member))
	}
	for index, lineRange := range node.FamilyMemberLineRanges {
		fmt.Fprintf(&b, "%d:%d..%d\x01", index, lineRange[0], lineRange[1])
	}
	for index, value := range node.FamilyMemberWallMS {
		fmt.Fprintf(&b, "%d:%.6f\x01", index, value)
	}
	return b.String()
}

func traceDecisionWriteSemanticMembers(b *strings.Builder, node types.TraceCausalProjectionNode, limit int) {
	if b == nil || node.FamilyMemberCount <= 0 ||
		len(node.FamilyMemberRoster) != node.FamilyMemberCount ||
		len(node.FamilyMemberLineRanges) != node.FamilyMemberCount ||
		len(node.FamilyMemberWallMS) != node.FamilyMemberCount {
		return
	}
	count := node.FamilyMemberCount
	if limit > 0 && count > limit {
		count = limit
	}
	for index := 0; index < count; index++ {
		rangeValue := node.FamilyMemberLineRanges[index]
		fmt.Fprintf(b, "    - member_%d span=`%s`; duration=%.3fms; lines=%d..%d\n",
			index+1, traceDecisionPromptScalar(node.FamilyMemberRoster[index]),
			node.FamilyMemberWallMS[index], rangeValue[0], rangeValue[1])
	}
	if count < node.FamilyMemberCount {
		fmt.Fprintf(b, "    - members_omitted=%d; complete_roster_available_in_projection=`true`\n", node.FamilyMemberCount-count)
	}
}

func traceDecisionEliminableSeats(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	return types.TraceAnswerDecisionEliminableSeats(projection, limit)
}

// traceDecisionPreferProjectionWindowNodes prevents a local drilldown board
// from competing as the principal decision board when the projection also
// carries rows measured over its exact requested/elected window. Other-window
// rows remain losslessly available in the observation ledger and projection
// appendix; this high-salience decision handoff simply keeps one caliber.
// Absence of an exact-window row preserves the previous bounded evidence.
func traceDecisionPreferProjectionWindowNodes(projection types.TraceCausalProjection, nodes []types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
	if len(nodes) == 0 || !types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
		return nodes
	}
	matching := make([]types.TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		start, end, ok := traceDecisionNodeQueryWindow(node)
		if !ok || !traceDecisionSameWindow(start, end, projection.WindowStartTs, projection.WindowEndTs) {
			continue
		}
		matching = append(matching, node)
	}
	if len(matching) == 0 {
		return nodes
	}
	return matching
}

func traceDecisionNodeQueryWindow(node types.TraceCausalProjectionNode) (float64, float64, bool) {
	if types.TraceCausalProjectionWindowPresent(node.RankQueryWindowStartTs, node.RankQueryWindowEndTs) {
		return node.RankQueryWindowStartTs, node.RankQueryWindowEndTs, true
	}
	if types.TraceCausalProjectionWindowPresent(node.QueryWindowStartTs, node.QueryWindowEndTs) {
		return node.QueryWindowStartTs, node.QueryWindowEndTs, true
	}
	return 0, 0, false
}

func traceDecisionSameWindow(aStart, aEnd, bStart, bEnd float64) bool {
	return absFloat(aStart-bStart) <= types.TraceCausalProjectionSameWindowToleranceS &&
		absFloat(aEnd-bEnd) <= types.TraceCausalProjectionSameWindowToleranceS
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
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
	if types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
		matching := make([]traceDecisionContextRow, 0, len(out))
		for _, row := range out {
			start, end, ok := traceDecisionNodeQueryWindow(row.node)
			if ok && traceDecisionSameWindow(start, end, projection.WindowStartTs, projection.WindowEndTs) {
				matching = append(matching, row)
			}
		}
		if len(matching) > 0 {
			out = matching
		}
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
		kept := append([]traceDecisionContextRow(nil), out[:limit]...)
		// Adjacent rows sort before background rows by design, but a global
		// cap must not turn "adjacent exists" into "background absent". Keep
		// the legacy first-N order and replace only the final row when the cap
		// would otherwise erase the entire background lane. The replacement is
		// the background lane's own highest-valued row from the same typed
		// window; no prose or label matching participates.
		hasBackground := false
		for _, row := range kept {
			if row.lane == "background" {
				hasBackground = true
				break
			}
		}
		if !hasBackground {
			for _, row := range out[limit:] {
				if row.lane == "background" {
					kept[len(kept)-1] = row
					break
				}
			}
		}
		out = kept
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

// traceDecisionEliminableSeatKind keeps the producer's exact typed cause kind
// visible on the decision row. StateKind remains useful for occupancy rows,
// but it is too lossy for a ranked composite seat: a
// priority_inversion_candidate whose dominant state is runnable must not be
// presented to the answer model as an ordinary runnable row. This is prompt
// display only; it does not change admission, ranking, or answer validation.
func traceDecisionEliminableSeatKind(node types.TraceCausalProjectionNode) string {
	if kind := strings.TrimSpace(node.TypeToken); kind != "" {
		return kind
	}
	return traceDecisionNodeKind(node)
}

// traceDecisionNodeIsPriorityInversionCandidate reads only producer-owned
// typed fields. The bool is the preferred carrier; TypeToken covers ranked
// aggregate rows whose state-oriented merge retained the exact cause token but
// not the duplicate display flag. Object, fix-direction and prose are
// deliberately excluded so an ordinary runnable/lock-priority row cannot be
// upgraded by inference.
func traceDecisionNodeIsPriorityInversionCandidate(node types.TraceCausalProjectionNode) bool {
	if node.PriorityInversionCandidate {
		return true
	}
	switch strings.TrimSpace(node.TypeToken) {
	case "priority_inversion_candidate", "priority_inversion_runnable_wait":
		return true
	default:
		return false
	}
}

// traceDecisionWriteModelFacingDirection keeps the registry's internal
// grouping token from becoming a stronger mechanism claim in model context.
// A measured priority-inversion candidate without a typed holder/waiter row is
// a priority/dependency SUPPLY validation direction, not proof of a lock.
// Ranking, projection grouping, values, and deterministic report rendering
// continue to consume node.FixDirection unchanged.
func traceDecisionWriteModelFacingDirection(b *strings.Builder, node types.TraceCausalProjectionNode) {
	if b == nil {
		return
	}
	key, value, ok := traceDecisionModelFacingDirection(node)
	if ok {
		fmt.Fprintf(b, "; %s=`%s`", key, value)
	}
}

func traceDecisionModelFacingDirection(node types.TraceCausalProjectionNode) (key, value string, ok bool) {
	if strings.TrimSpace(node.FixDirection) == "" {
		return "", "", false
	}
	if traceDecisionNodeIsPriorityInversionCandidate(node) {
		return "validation_direction", "priority_or_dependency_supply", true
	}
	return "fix_direction", strings.TrimSpace(node.FixDirection), true
}

// traceDecisionWritePriorityCandidateClaimEnvelope is the single concise
// model-facing semantic envelope for an unconfirmed candidate seat. It reads
// typed node fields only and remains prompt-only: no user/model/final prose is
// inspected, no gate consumes the words, and no answer is authored or edited.
func traceDecisionWritePriorityCandidateClaimEnvelope(b *strings.Builder, node types.TraceCausalProjectionNode) {
	if b == nil || !traceDecisionNodeIsPriorityInversionCandidate(node) {
		return
	}
	b.WriteString("; claim_envelope=`measured_lower_priority_dependency_supply_candidate`")
	b.WriteString("; candidate_mechanism_authority=`lower_priority_dependency_only`")
	b.WriteString("; allowed_mechanism_scope=`measured_dependency_scheduler_supply_before_downstream_wakeup`")
	b.WriteString("; not_authorized_mechanisms=`priority_inversion_occurrence,post_wakeup_delay,lock_or_holder_waiter,synchronous_blocking`")
	b.WriteString("; synchronous_blocker_authority=`not_provided_by_candidate_seat`")
	b.WriteString("; holder_waiter_authority=`not_provided_by_candidate_seat`")
	b.WriteString("; conclusion_caliber=`validation_candidate`")
	if traceDecisionNodePhase(node) == "pre_wakeup_dependency" {
		b.WriteString("; priority_candidate_scope=`dependency_scheduler_supply_before_downstream_wakeup`")
		b.WriteString("; post_wakeup_preemption_authority=`not_provided_by_this_seat`")
	}
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
