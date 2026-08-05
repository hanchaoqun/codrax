package types

import (
	"path"
	"sort"
	"strings"
)

const SourceInventoryRowReasonOutsideRequestedPathScope = "outside_requested_path_scope"

// SourceInventoryRequestedPathScopes returns the engine-minted, request-bound
// repository path boundary for a source-inventory request. The producer has
// already joined analyzer-stage tool scope with MentionedEntities provenance;
// consumers deliberately do not reconstruct it from RawRequest or free prose.
func SourceInventoryRequestedPathScopes(rm RequestModel) []string {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return nil
	}
	return normalizeSourceInventoryRequestedPathScopes(rm.AnalyzerHints.SourceInventoryRequestedPathScopes)
}

func normalizeSourceInventoryRequestedPathScopes(scopes []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range scopes {
		scope := NormalizeSourceInventoryRequestedPathScope(raw)
		if scope == "" || scope == "." || seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

// NormalizeSourceInventoryRequestedPathScope canonicalizes one repo-relative
// path scope and rejects root, traversal, absolute, virtual, and multiline
// values. It does not decide whether the scope was user-requested; that
// authority comes from the producer's analyzer-prescan provenance join.
func NormalizeSourceInventoryRequestedPathScope(raw string) string {
	raw = strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)), "`'\" ")
	if raw == "" || strings.ContainsAny(raw, "\n\r\t") || strings.Contains(raw, "://") ||
		path.IsAbs(raw) || sourceInventoryRequestedPathHasWindowsDrive(raw) {
		return ""
	}
	raw = strings.Trim(raw, "/")
	cleaned := path.Clean(raw)
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return ""
	}
	return cleaned
}

func sourceInventoryRequestedPathHasWindowsDrive(raw string) bool {
	return len(raw) >= 3 && ((raw[0] >= 'A' && raw[0] <= 'Z') || (raw[0] >= 'a' && raw[0] <= 'z')) &&
		raw[1] == ':' && raw[2] == '/'
}

func sourceInventoryPathWithinRequestedScopes(candidate string, scopes []string) bool {
	candidate = NormalizeSourceInventoryRequestedPathScope(candidate)
	if candidate == "" {
		return false
	}
	for _, scope := range normalizeSourceInventoryRequestedPathScopes(scopes) {
		if candidate == scope || strings.HasPrefix(candidate, scope+"/") {
			return true
		}
	}
	return false
}

func sourceInventoryRequestedPathFollowupDebt(rm RequestModel, roles []AnswerCandidateRole) SourceInventoryFollowupDebt {
	scopes := SourceInventoryRequestedPathScopes(rm)
	if len(scopes) == 0 {
		return SourceInventoryFollowupDebt{}
	}
	return NormalizeSourceInventoryFollowupDebt(SourceInventoryFollowupDebt{
		Active:     true,
		ReasonCode: SourceInventoryFollowupDebtRequestedPathBoundary,
		Query: SourceInventoryLensQuery{
			Path: ".", Scopes: scopes, Roles: roles,
			IncludeCounts: true, IncludeAttributes: false, TopN: 24,
		},
		Roles: roles,
	})
}

func sourceInventoryRowFromMemberWithinRequestedPathScopes(
	member SourceInventoryObservationMember,
	setRole AnswerCandidateRole,
	principalRoles map[AnswerCandidateRole]bool,
	scope SourceScope,
	rm RequestModel,
) SourceInventoryRow {
	row := sourceInventoryRowFromMember(member, setRole, principalRoles, scope)
	requested := SourceInventoryRequestedPathScopes(rm)
	file := sourceInventoryRowLocationFile(row)
	if len(requested) > 0 && file != "" && !sourceInventoryPathWithinRequestedScopes(file, requested) {
		row.Lane = SourceInventoryRowLaneAudit
		row.ReasonCode = SourceInventoryRowReasonOutsideRequestedPathScope
	}
	return row
}

func sourceInventoryRowLocationFile(row SourceInventoryRow) string {
	if file := strings.TrimSpace(row.Member.File); file != "" {
		return file
	}
	if _, loc, ok := ParseAnswerSupportRefMemberLocation(row.Member.SupportRef); ok {
		return loc.File
	}
	if loc, ok := ParseAnswerSourceLocationSurface(row.Member.SupportRef); ok {
		return loc.File
	}
	if file, ok := ParseAnswerFilePathSurface(row.Member.SupportRef); ok {
		return file
	}
	return ""
}
