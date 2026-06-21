package types

import "strings"

// SourceInventoryExactAbsenceNeedsInventoryProof reports whether a
// source-inventory-lane exact absence answer is trying to stand on a generic
// negative query while a typed repo source-class universe says the candidate
// universe still exists and has not been closed by a complete empty
// source-inventory set. This is intentionally structural: it consumes the
// analyzer-emitted SourceInventoryProfile plus SourcePathRole-derived class
// counts, never user prose or model rationale.
func SourceInventoryExactAbsenceNeedsInventoryProof(profile *SourceInventoryProfile, observation SourceInventoryObservation) (string, bool) {
	if profile == nil || !profile.Active() {
		return "", false
	}
	classes := sourceInventoryPositiveSourceClasses(observation.SourceClasses)
	if len(classes) == 0 {
		return "", false
	}
	roles := profile.PrincipalTargetRoles()
	if len(roles) == 0 {
		return "", false
	}
	if sourceInventoryObservationHasCompleteZeroSetsForRoles(observation, roles) &&
		sourceInventorySourceClassesComplete(classes) &&
		!sourceInventoryObservationExecutionIncompleteForAbsence(observation) {
		return "", false
	}
	return "source_classes=" + SourceInventorySourceClassSummary(classes, 8) +
		"; principal_roles=" + sourceInventoryPrincipalRoleSummary(roles, 8) +
		sourceInventoryExactAbsenceExecutionSummary(observation, classes), true
}

// SourceInventorySourceClassSummary renders a compact typed class-count matrix
// for repair hints and audit logs. It is deterministic display only; callers
// decide policy from the typed counts above.
func SourceInventorySourceClassSummary(classes []SourceInventorySourceClassCount, limit int) string {
	classes = sourceInventoryPositiveSourceClasses(classes)
	if len(classes) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(classes) {
		limit = len(classes)
	}
	parts := make([]string, 0, limit+1)
	for i, class := range classes {
		if i >= limit {
			parts = append(parts, "...")
			break
		}
		label := strings.TrimSpace(string(class.Role))
		if label == "" {
			label = string(SourcePathRoleUnknown)
		}
		suffix := ""
		if !class.Complete {
			suffix = "+"
		}
		parts = append(parts, label+":"+itoaSourceInventory(class.Count)+suffix)
	}
	return strings.Join(parts, ", ")
}

func sourceInventoryPositiveSourceClasses(in []SourceInventorySourceClassCount) []SourceInventorySourceClassCount {
	if len(in) == 0 {
		return nil
	}
	out := make([]SourceInventorySourceClassCount, 0, len(in))
	for _, class := range normalizeSourceInventorySourceClassCounts(cloneSourceInventorySourceClassCounts(in)) {
		if class.Count <= 0 || class.Role == SourcePathRoleUnknown {
			continue
		}
		out = append(out, class)
	}
	return out
}

func sourceInventoryObservationHasCompleteZeroSetsForRoles(observation SourceInventoryObservation, roles []AnswerCandidateRole) bool {
	if len(roles) == 0 {
		return false
	}
	required := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role == AnswerCandidateRoleUnknown {
			continue
		}
		required[role] = true
	}
	if len(required) == 0 {
		return false
	}
	covered := map[AnswerCandidateRole]bool{}
	for _, set := range observation.Sets {
		if !required[set.Role] {
			continue
		}
		if !set.Complete || set.Count != 0 || len(set.Members) != 0 {
			continue
		}
		covered[set.Role] = true
	}
	return len(covered) == len(required)
}

func sourceInventorySourceClassesComplete(classes []SourceInventorySourceClassCount) bool {
	return SourceInventorySourceClassesComplete(classes)
}

func sourceInventoryObservationExecutionIncompleteForAbsence(observation SourceInventoryObservation) bool {
	if !observation.Complete {
		return true
	}
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		return true
	}
	return observation.Page != nil && !observation.Page.Complete
}

func sourceInventoryExactAbsenceExecutionSummary(observation SourceInventoryObservation, classes []SourceInventorySourceClassCount) string {
	var parts []string
	if !sourceInventorySourceClassesComplete(classes) {
		parts = append(parts, "source_class_universe_incomplete")
	}
	if !observation.Complete {
		parts = append(parts, "observation_incomplete")
	}
	if observation.Execution != nil && observation.Execution.CandidateBudgetTruncated {
		parts = append(parts, "candidate_budget_truncated")
	}
	if observation.Page != nil && !observation.Page.Complete {
		parts = append(parts, "page_incomplete")
	}
	if len(parts) == 0 {
		return ""
	}
	return "; execution=" + strings.Join(parts, ",")
}

func sourceInventoryPrincipalRoleSummary(roles []AnswerCandidateRole, limit int) string {
	if len(roles) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(roles) {
		limit = len(roles)
	}
	parts := make([]string, 0, limit+1)
	seen := map[AnswerCandidateRole]bool{}
	for _, role := range roles {
		if role == AnswerCandidateRoleUnknown || seen[role] {
			continue
		}
		seen[role] = true
		if len(parts) >= limit {
			parts = append(parts, "...")
			break
		}
		parts = append(parts, string(role))
	}
	return strings.Join(parts, ", ")
}

func itoaSourceInventory(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
