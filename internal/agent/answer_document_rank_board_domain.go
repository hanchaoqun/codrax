package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// renderTraceFinalPrincipalRankPopulation repeats the exact selected-window
// ordinal population at the final synthesis seam, partitioned by query identity. Earlier rank boards remain
// losslessly available for investigation, but a row measured in a different
// query window is contextual evidence for this answer and cannot retain its
// local board ordinal in the elected-window conclusion. This consumes only
// compiled typed window/rank fields; it neither inspects nor rewrites prose.
func renderTraceFinalPrincipalRankPopulation(set types.TraceCausalProjectionSet, lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	var b strings.Builder
	for index, projection := range set.Projections {
		if !types.TraceCausalProjectionPrincipalWindowAuthoritative(projection) {
			continue
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		// Census and display use the same eligibility selector. The bounded
		// prompt is only a preview, not authority to demote unlisted seats.
		population := types.TraceAnswerDecisionEliminableSeats(projection, 0)
		groups := traceFinalRankDisplayGroups(projection, population, 8)
		excluded := traceFinalDifferentWindowRankedSeats(projection, 8)
		if len(population) == 0 && len(excluded) == 0 {
			continue
		}
		displayed, omittedBoards := 0, 0
		for _, group := range groups {
			displayed += len(group.shown)
			if len(group.shown) == 0 {
				omittedBoards++
			}
		}
		fmt.Fprintf(&b, "- selected_window_population_rows=`%d`; displayed_rows=`%d`; omitted_rows=`%d`; query_board_groups=`%d`; omitted_board_groups=`%d`. This preview does not elect a winning query or permit cross-board addition/rank comparison. ranked_rows_complete describes display coverage of the admitted rows only, not exhaustive query/cause coverage or complete board identity.\n",
			len(population), displayed, len(population)-displayed, len(groups), omittedBoards)
		for _, group := range groups {
			if len(group.shown) == 0 {
				continue
			}
			displayedOrdinals := make([]string, 0, len(group.shown))
			for _, node := range group.shown {
				displayedOrdinals = append(displayedOrdinals, fmt.Sprintf("#%d", node.Rank))
			}
			fmt.Fprintf(&b, "- selected_window_reader_rank_roster artifact=`%s`; selected_window=`%.6f..%.6f`; ranked_row_count=`%d`; emitted_row_count=`%d`; ranked_rows_complete=`%t`; displayed_ordinals=`%s`; artifact_path=`%s`; board_target=`%s`; board_params=`%s`; board_identity_complete=`%t`. Ranks are local to this query board. The model owns the conclusion; not displayed here does not change a row's eligibility or published rank. Use the full typed selected-window population for other ranked rows; an explicit different-window row below remains supporting context only.\n",
				traceDecisionPromptScalar(label), projection.WindowStartTs, projection.WindowEndTs,
				len(group.rows), len(group.shown), len(group.rows) == len(group.shown), strings.Join(displayedOrdinals, ","),
				traceRankDisplayField(group.identity.ArtifactPath), traceRankDisplayField(group.identity.BoardTarget),
				traceRankDisplayField(group.identity.BoardParamsFingerprint), group.identity.Complete)
			if !group.identity.Complete {
				b.WriteString("  - Query identity is incomplete; keep this observed row separate, do not infer a shared board or global ordinal.\n")
			}
			for _, node := range group.shown {
				causeLabel := strings.TrimSpace(tool.TraceRootCauseTypeDisplayLabel(traceDecisionEliminableSeatKind(node), zh))
				if causeLabel == "" {
					if zh {
						causeLabel = "已测链上候选"
					} else {
						causeLabel = "measured on-chain candidate"
					}
				}
				fmt.Fprintf(&b, "  - reader_rank=`#%d`; subject=`%s`; reader_cause_label=%q; effective_attribution=%.3fms",
					node.Rank, traceDecisionPromptScalar(strings.TrimSpace(node.Subject)), causeLabel, node.EffectiveImpactMS)
				if start, end, ok := traceDecisionNodeQueryWindow(node); ok {
					fmt.Fprintf(&b, "; query_window=`%.6f..%.6f`", start, end)
				}
				b.WriteByte('\n')
			}
		}
		for _, node := range excluded {
			fmt.Fprintf(&b, "  - unranked_context_row subject=`%s`; effective_attribution=%.3fms; selected_window_role=`supporting_context_only`; selected_window_ordinal_permission=`forbidden`",
				traceDecisionPromptScalar(strings.TrimSpace(node.Subject)), node.EffectiveImpactMS)
			if start, end, ok := traceDecisionNodeQueryWindow(node); ok {
				fmt.Fprintf(&b, "; row_query_window=`%.6f..%.6f`", start, end)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Presentation groups consume the already-selected population, never a second
// eligibility rule. Unknown identities remain independent rows, not a shared
// guessed board. Caps apply to the whole projection, not once per board.
type traceFinalRankDisplayGroup struct {
	identity types.TraceRankBoardDisplayIdentity
	rows     []types.TraceCausalProjectionNode
	shown    []types.TraceCausalProjectionNode
	sortKey  string
}

func traceFinalRankDisplayGroups(projection types.TraceCausalProjection, population []types.TraceCausalProjectionNode, limit int) []traceFinalRankDisplayGroup {
	var groups []traceFinalRankDisplayGroup
	byKey := make(map[string]int)
	for _, node := range population {
		identity := types.TraceRankBoardDisplayIdentityFromNode(projection, node)
		index, found := byKey[identity.Key]
		if identity.Key == "" || !found {
			index = len(groups)
			key := identity.Key
			if key == "" {
				// Sorting for bounded display grants no shared identity. The
				// original selector already owns exact row deduplication.
				key = "\xff" + traceDecisionNodeIdentity(node)
			} else {
				byKey[key] = index
			}
			groups = append(groups, traceFinalRankDisplayGroup{identity: identity, sortKey: key})
		}
		groups[index].rows = append(groups[index].rows, node)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].sortKey < groups[j].sortKey })
	// The population is already in published rank order; filtering into its
	// own board retains that order. Round-robin preview prevents a large board
	// from hiding every row of another board without choosing a winning query.
	written := 0
	for depth := 0; written < limit; depth++ {
		added := false
		for i := range groups {
			if depth >= len(groups[i].rows) {
				continue
			}
			groups[i].shown = append(groups[i].shown, groups[i].rows[depth])
			written++
			added = true
			if written == limit {
				break
			}
		}
		if !added {
			break
		}
	}
	return groups
}

func traceRankDisplayField(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not_provided"
	}
	return traceDecisionPromptScalar(value)
}

type traceReaderRankDisplayAuthority struct {
	types.TraceRankRosterAuthority
	identity types.TraceRankBoardDisplayIdentity
}

func traceReaderRankDisplayAuthorities(set types.TraceCausalProjectionSet) []traceReaderRankDisplayAuthority {
	var out []traceReaderRankDisplayAuthority
	for _, projection := range set.Projections {
		// The existing constructor still owns seat selection, channel and rank
		// gap checks. Retain the enclosing capture before its label-only view.
		for _, authority := range types.BuildTraceRankRosterAuthorities(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}) {
			identity := types.TraceRankBoardDisplayIdentityFromNode(projection, types.TraceCausalProjectionNode{
				RankBoardTarget: authority.BoardTarget, RankBoardParamsFingerprint: authority.BoardParamsFingerprint,
				RankQueryWindowStartTs: authority.WindowStartTs, RankQueryWindowEndTs: authority.WindowEndTs,
			})
			if identity.Complete {
				out = append(out, traceReaderRankDisplayAuthority{authority, identity})
				continue
			}
			// No loss of admitted facts, and no grouping on an incomplete key.
			// This status is display guidance only, never written to the source.
			for _, seat := range authority.Seats {
				row := authority
				row.Seats = []types.TraceRankRosterSeat{seat}
				row.Complete, row.Status = false, "identity_incomplete"
				out = append(out, traceReaderRankDisplayAuthority{row, identity})
			}
		}
	}
	return out
}
