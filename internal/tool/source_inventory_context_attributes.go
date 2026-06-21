package tool

import (
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const sourceInventoryGraphPackageRowContextReason = "graph_package_row_context"

func sourceInventoryObservationWithGraphContextAttributes(ctx *types.BusContext, observation types.SourceInventoryObservation) types.SourceInventoryObservation {
	if !observation.IsActive() || ctx == nil || ctx.Mutable == nil {
		return observation
	}
	graph, _ := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if graph == nil || graph.FileIndex == nil {
		return observation
	}
	out := types.CloneSourceInventoryObservation(observation)
	changed := false
	for setIdx := range out.Sets {
		for memberIdx := range out.Sets[setIdx].Members {
			member := &out.Sets[setIdx].Members[memberIdx]
			if !sourceInventoryRoleCarriesGraphContext(member.Role) {
				continue
			}
			attr, ok := sourceInventoryGraphPackageAttributeForMember(graph, *member)
			if !ok || sourceInventoryMemberHasAttribute(member.Attributes, attr.Role, attr.Name) {
				continue
			}
			member.Attributes = append(member.Attributes, attr)
			changed = true
		}
	}
	if !changed {
		return observation
	}
	return out
}

func sourceInventoryRoleCarriesGraphContext(role types.AnswerCandidateRole) bool {
	switch role {
	case types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleType,
		types.AnswerCandidateRoleConstant,
		types.AnswerCandidateRoleVariable,
		types.AnswerCandidateRoleField,
		types.AnswerCandidateRoleRoute,
		types.AnswerCandidateRoleHelper,
		types.AnswerCandidateRoleAgent,
		types.AnswerCandidateRoleOther:
		return true
	default:
		return false
	}
}

func sourceInventoryGraphPackageAttributeForMember(graph *repotypes.Graph, member types.SourceInventoryObservationMember) (types.SourceInventoryObservationAttribute, bool) {
	fi := sourceInventoryGraphFileInfoForMember(graph, member)
	if fi == nil {
		return types.SourceInventoryObservationAttribute{}, false
	}
	pkg := strings.TrimSpace(fi.Package)
	if pkg == "" {
		return types.SourceInventoryObservationAttribute{}, false
	}
	file := strings.TrimSpace(fi.RelPath)
	if file == "" {
		file = strings.TrimSpace(member.File)
	}
	return types.SourceInventoryObservationAttribute{
		Name:          pkg,
		Key:           sourceInventoryGraphPackageAttributeKey(pkg),
		SupportRef:    pkg + ": " + strings.TrimSpace(file),
		Note:          "declared package/module scope",
		Role:          types.AnswerCandidateRolePackage,
		Exported:      true,
		File:          file,
		Language:      strings.TrimSpace(fi.Language),
		CoverageState: types.SourceInventoryCoverageObserved,
		Reason:        sourceInventoryGraphPackageRowContextReason,
	}, true
}

func sourceInventoryGraphFileInfoForMember(graph *repotypes.Graph, member types.SourceInventoryObservationMember) *repotypes.FileInfo {
	if graph == nil || graph.FileIndex == nil {
		return nil
	}
	for _, raw := range sourceInventoryMemberFileCandidates(member) {
		file := sourceInventoryNormalizeGraphFile(raw)
		if file == "" {
			continue
		}
		if fi := graph.FileIndex[file]; fi != nil {
			return fi
		}
		for key, fi := range graph.FileIndex {
			if fi == nil {
				continue
			}
			if sourceInventoryNormalizeGraphFile(key) == file || sourceInventoryNormalizeGraphFile(fi.RelPath) == file {
				return fi
			}
		}
	}
	return nil
}

func sourceInventoryMemberFileCandidates(member types.SourceInventoryObservationMember) []string {
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			out = append(out, raw)
		}
	}
	add(member.File)
	if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(member.SupportRef); ok {
		add(loc.File)
	}
	return out
}

func sourceInventoryGraphPackageAttributeKey(pkg string) string {
	key := aggregateMemberKey(pkg)
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(pkg))
	}
	return key
}

func sourceInventoryMemberHasAttribute(attrs []types.SourceInventoryObservationAttribute, role types.AnswerCandidateRole, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, attr := range attrs {
		if attr.Role == role && strings.EqualFold(strings.TrimSpace(attr.Name), name) {
			return true
		}
	}
	return false
}
