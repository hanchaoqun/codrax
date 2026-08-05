package agent

import (
	"fmt"
	"strings"

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
	if authority.CausalUnproven {
		b.WriteString("- causal_conclusion=`unproven`: the strongest supported synthesis is a bounded candidate or first validation direction, not a proven dropped-frame/frame-deadline cause.\n")
	}
	if authority.FrameEvidenceStatus != "" {
		fmt.Fprintf(&b, "- frame_evidence_status=`%s`: do not infer a stronger frame/deadline attribution.\n", authority.FrameEvidenceStatus)
	}
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
	b.WriteString("- cross_row_addition=`not_authorized_without_exact_typed_relation`: a row-local state breakdown applies only to that row. Do not merge, decompose, compare as one subtotal, or add values from different rows/threads/fix directions unless one exact typed relation/fold carrier names those members and authorizes that operation.\n")
	b.WriteString("- Preserve directed wakeup/path semantics and typed holder/waiter or overlap relations exactly. Temporal order, adjacency, a candidate flag, or a kernel caller symbol alone does not prove synchronous blocking, lock ownership, post-wakeup preemption, or physical coupling.\n\n")
	return b.String()
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
			fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target_direct_blocking_authority=`unavailable_without_typed_target`; wakeup_path_blocking_authority=`not_implied`.\n", label)
		case len(relations) == 0:
			fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target=`%s`; target_direct_blocking_authority=`not_provided_by_projection`; wakeup_path_blocking_authority=`not_implied`. Describe typed wakeup edges as wakeup/dependency relations, not as a direct blocker.\n", label, target)
		default:
			for _, relation := range relations {
				fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target=`%s`; target_direct_blocking_authority=`typed_waiter_holder`; waiter=`%s`; holder=`%s`; blocking_kind=`%s`; row_identity=`%s`.\n",
					label, target, relation.waiter, relation.holder, relation.kind, relation.rowIdentity)
			}
		}

		leaders := traceFinalFixDirectionLeaders(projection, 6)
		if len(leaders) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- compact_authority artifact=`%s`: fix_direction_summary_authority=`single_published_leader_only`; direction_subtotal_authority=`not_provided_without_exact_fold`. Do not sum same-direction seats merely because their labels share a direction.\n", label)
		for _, node := range leaders {
			fmt.Fprintf(&b, "  - fix_direction=`%s`; leader_rank=#%d; leader_subject=`%s`; leader_effective_attribution=%.3fms; row_identity=`%s`",
				strings.TrimSpace(node.FixDirection), node.Rank, strings.TrimSpace(node.Subject),
				node.EffectiveImpactMS, traceDecisionNodeIdentity(node))
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
			b.WriteString("\n")
		}
	}
	return b.String()
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
	seen := map[string]bool{}
	out := make([]types.TraceCausalProjectionNode, 0, len(seats))
	for _, node := range seats {
		direction := strings.TrimSpace(node.FixDirection)
		if direction == "" || seen[direction] {
			continue
		}
		seen[direction] = true
		out = append(out, node)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
