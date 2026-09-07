package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// traceRankDisplayDirectionSection binds an original published section to a
// complete query domain. It never slices the projection or recalculates a
// subtotal: an unbound receipt remains available in the independent relation
// roster with its original members, arithmetic and donor references.
func traceRankDisplayDirectionSection(projection types.TraceCausalProjection, sections []types.TraceAnswerDirectionSection, node types.TraceCausalProjectionNode) (types.TraceAnswerDirectionSection, bool) {
	identity := types.TraceRankBoardDisplayIdentityFromNode(projection, node)
	direction := strings.TrimSpace(node.FixDirection)
	if !identity.Complete || direction == "" {
		return types.TraceAnswerDirectionSection{}, false
	}
	var selected types.TraceAnswerDirectionSection
	found := false
	for _, section := range sections {
		if strings.TrimSpace(section.Direction) != direction || len(section.Members) == 0 {
			continue
		}
		leaderIdentity := types.TraceRankBoardDisplayIdentityFromNode(projection, section.Leader)
		if !leaderIdentity.Complete || leaderIdentity.Key != identity.Key || strings.TrimSpace(section.Leader.FixDirection) != direction {
			continue
		}
		matches := true
		for _, member := range section.Members {
			memberIdentity := types.TraceRankBoardDisplayIdentityFromNode(projection, member)
			if !memberIdentity.Complete || memberIdentity.Key != identity.Key || strings.TrimSpace(member.FixDirection) != direction {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		if found {
			return types.TraceAnswerDirectionSection{}, false
		}
		selected, found = section, true
	}
	return selected, found
}

// A model-facing subgroup is narrower than a complete query's fix direction.
// A full direction receipt cannot lend its leader or subtotal to one subgroup
// when it also contains seats with another typed display envelope.
func traceRankDisplayModelFacingDirectionSection(projection types.TraceCausalProjection, sections []types.TraceAnswerDirectionSection, group traceRankDirectionDisplayGroup, node types.TraceCausalProjectionNode) (types.TraceAnswerDirectionSection, bool) {
	section, ok := traceRankDisplayDirectionSection(projection, sections, node)
	if !ok {
		return types.TraceAnswerDirectionSection{}, false
	}
	matches := func(member types.TraceCausalProjectionNode) bool {
		key, value, known := traceDecisionModelFacingDirection(member)
		return known && key == group.key && value == group.value
	}
	if !matches(section.Leader) {
		return types.TraceAnswerDirectionSection{}, false
	}
	for _, member := range section.Members {
		if !matches(member) {
			return types.TraceAnswerDirectionSection{}, false
		}
	}
	return section, true
}

type traceRankDirectionDisplayGroup struct {
	identity              types.TraceRankBoardDisplayIdentity
	direction, key, value string
	members               []types.TraceCausalProjectionNode
}

// Inputs are already eligible seats. Grouping confers no admission or new
// relation authority. Unknown domains stay single rows, even with equal labels.
func traceRankDisplayDirectionGroups(projection types.TraceCausalProjection, population []types.TraceCausalProjectionNode) []traceRankDirectionDisplayGroup {
	return traceRankDisplayDirectionGroupsByKind(projection, population, false)
}

// The detailed handoff historically keeps candidate-only display envelopes
// distinct from confirmed mechanism directions. The compact/plan lane keeps
// its original direction-only population; both share the same board domains.
func traceRankDisplayModelFacingDirectionGroups(projection types.TraceCausalProjection, population []types.TraceCausalProjectionNode) []traceRankDirectionDisplayGroup {
	return traceRankDisplayDirectionGroupsByKind(projection, population, true)
}

func traceRankDisplayDirectionGroupsByKind(projection types.TraceCausalProjection, population []types.TraceCausalProjectionNode, splitModelFacing bool) []traceRankDirectionDisplayGroup {
	var out []traceRankDirectionDisplayGroup
	for _, board := range traceFinalRankDisplayGroups(projection, population, 0) {
		byDirection := map[string]int{}
		var groups []traceRankDirectionDisplayGroup
		for _, node := range board.rows {
			key, value, ok := traceDecisionModelFacingDirection(node)
			if !ok {
				continue
			}
			direction := strings.TrimSpace(node.FixDirection)
			groupKey := direction
			if splitModelFacing {
				groupKey = strings.Join([]string{direction, key, value}, "\x00")
			}
			index, found := byDirection[groupKey]
			if !found || !board.identity.Complete {
				index = len(groups)
				groups = append(groups, traceRankDirectionDisplayGroup{identity: board.identity, direction: direction, key: key, value: value})
				if board.identity.Complete {
					byDirection[groupKey] = index
				}
			}
			groups[index].members = append(groups[index].members, node)
		}
		sort.SliceStable(groups, func(i, j int) bool {
			left, right := traceRankDirectionGroupLeader(groups[i]), traceRankDirectionGroupLeader(groups[j])
			if left.EffectiveImpactMS != right.EffectiveImpactMS {
				return left.EffectiveImpactMS > right.EffectiveImpactMS
			}
			return groups[i].direction < groups[j].direction
		})
		out = append(out, groups...)
	}
	return out
}

func traceRankDirectionGroupLeader(group traceRankDirectionDisplayGroup) types.TraceCausalProjectionNode {
	var leader types.TraceCausalProjectionNode
	for _, node := range group.members {
		if leader.Rank == 0 || node.EffectiveImpactMS > leader.EffectiveImpactMS ||
			(node.EffectiveImpactMS == leader.EffectiveImpactMS && node.Rank < leader.Rank) {
			leader = node
		}
	}
	return leader
}

func traceRankWriteDirectionDomain(b *strings.Builder, identity types.TraceRankBoardDisplayIdentity) {
	window := "not_provided"
	if types.TraceCausalProjectionWindowPresent(identity.WindowStartTs, identity.WindowEndTs) {
		window = fmt.Sprintf("%.6f..%.6f", identity.WindowStartTs, identity.WindowEndTs)
	}
	fmt.Fprintf(b, "; artifact_path=`%s`; artifact_id=`%s`; board_target=`%s`; query_window=`%s`; board_params=`%s`; board_identity_complete=`%t`",
		traceRankDisplayField(identity.ArtifactPath), traceRankDisplayField(identity.ArtifactLabel),
		traceRankDisplayField(identity.BoardTarget), window, traceRankDisplayField(identity.BoardParamsFingerprint), identity.Complete)
}
