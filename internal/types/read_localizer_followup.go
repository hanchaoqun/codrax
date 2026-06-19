package types

import "strings"

type ReadLocalizerFollowupState string

const (
	ReadLocalizerFollowupNotRequired ReadLocalizerFollowupState = "not_required"
	ReadLocalizerFollowupNeeded      ReadLocalizerFollowupState = "needed"
)

type ReadLocalizerFollowup struct {
	State                ReadLocalizerFollowupState `json:"state,omitempty"`
	ReasonCode           string                     `json:"reason_code,omitempty"`
	Source               string                     `json:"source,omitempty"`
	CandidatePaths       []string                   `json:"candidate_paths,omitempty"`
	RequiredRoutes       []RepoMapNavigationRoute   `json:"required_routes,omitempty"`
	MissingRoutes        []RepoMapNavigationRoute   `json:"missing_routes,omitempty"`
	EvidenceRequirements []string                   `json:"evidence_requirements,omitempty"`
}

func CloneReadLocalizerFollowupPtr(in *ReadLocalizerFollowup) *ReadLocalizerFollowup {
	if in == nil {
		return nil
	}
	out := NormalizeReadLocalizerFollowup(*in)
	if out.State == "" || out.State == ReadLocalizerFollowupNotRequired {
		return nil
	}
	return &out
}

func NormalizeReadLocalizerFollowup(in ReadLocalizerFollowup) ReadLocalizerFollowup {
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	in.Source = strings.TrimSpace(in.Source)
	in.CandidatePaths = dedupSourceLocalizationPaths(in.CandidatePaths, sourceLocalizationMaxPaths)
	in.RequiredRoutes = normalizeRepoMapNavigationRoutes(in.RequiredRoutes)
	in.MissingRoutes = normalizeRepoMapNavigationRoutes(in.MissingRoutes)
	in.EvidenceRequirements = normalizeReadLocalizerFollowupStrings(in.EvidenceRequirements, 16)
	switch in.State {
	case ReadLocalizerFollowupNeeded, ReadLocalizerFollowupNotRequired:
	default:
		if len(in.CandidatePaths) > 0 || len(in.MissingRoutes) > 0 || len(in.EvidenceRequirements) > 0 {
			in.State = ReadLocalizerFollowupNeeded
		} else {
			in.State = ReadLocalizerFollowupNotRequired
		}
	}
	if in.State == ReadLocalizerFollowupNeeded && in.ReasonCode == "" {
		switch {
		case len(in.MissingRoutes) > 0 && len(in.CandidatePaths) > 0:
			in.ReasonCode = "read_localizer_owner_and_navigation_missing"
		case len(in.MissingRoutes) > 0:
			in.ReasonCode = "read_localizer_navigation_missing"
		default:
			in.ReasonCode = "read_localizer_owner_anchor_missing"
		}
	}
	if in.State == ReadLocalizerFollowupNotRequired {
		return ReadLocalizerFollowup{}
	}
	return in
}

func DeriveReadLocalizerFollowup(review *SourceLocalizationReview, coverage *RepoMapNavigationCoverage) *ReadLocalizerFollowup {
	var requirements LocalizationRequirementSet
	if SourceLocalizationReviewHasSignal(review) {
		requirements = LocalizationRequirementsFromSourceLocalizationReview(*review, 0)
	}
	normalizedCoverage := RepoMapNavigationCoverage{}
	if coverage != nil {
		normalizedCoverage = NormalizeRepoMapNavigationCoverage(*coverage)
	}
	needsOwner := requirements.OpenItems > 0 && len(requirements.MissingOwnerAnchorPaths) > 0
	needsNavigation := normalizedCoverage.State == RepoMapNavigationCoverageMissing ||
		normalizedCoverage.State == RepoMapNavigationCoveragePartial
	if !needsOwner && !needsNavigation {
		return nil
	}
	out := ReadLocalizerFollowup{
		State:          ReadLocalizerFollowupNeeded,
		Source:         "read_localizer_followup",
		CandidatePaths: readLocalizerCandidatePaths(review, requirements),
	}
	if normalizedCoverage.State != "" && normalizedCoverage.State != RepoMapNavigationCoverageNotRequired {
		out.RequiredRoutes = append(out.RequiredRoutes, normalizedCoverage.RequiredRoutes...)
		out.MissingRoutes = append(out.MissingRoutes, normalizedCoverage.MissingRoutes...)
		out.EvidenceRequirements = append(out.EvidenceRequirements, readLocalizerNavigationRequirementRows(normalizedCoverage, 8)...)
	}
	if requirements.OpenItems > 0 {
		out.EvidenceRequirements = append(out.EvidenceRequirements, LocalizationRequirementEvidenceRows(requirements, 8)...)
	}
	switch {
	case needsOwner && needsNavigation:
		out.ReasonCode = "read_localizer_owner_and_navigation_missing"
	case needsNavigation:
		out.ReasonCode = "read_localizer_navigation_missing"
	default:
		out.ReasonCode = "read_localizer_owner_anchor_missing"
	}
	normalized := NormalizeReadLocalizerFollowup(out)
	if normalized.State == "" || normalized.State == ReadLocalizerFollowupNotRequired {
		return nil
	}
	return &normalized
}

func readLocalizerCandidatePaths(review *SourceLocalizationReview, requirements LocalizationRequirementSet) []string {
	var paths []string
	paths = append(paths, requirements.MissingOwnerAnchorPaths...)
	paths = append(paths, requirements.MissingPriorContextPaths...)
	paths = append(paths, requirements.CandidatePaths...)
	if SourceLocalizationReviewHasSignal(review) {
		normalized := NormalizeSourceLocalizationReview(*review)
		paths = append(paths, normalized.OwnerMissingPaths...)
		paths = append(paths, normalized.MissingPaths...)
		paths = append(paths, normalized.SourcePaths...)
	}
	return dedupSourceLocalizationPaths(paths, sourceLocalizationMaxPaths)
}

func readLocalizerNavigationRequirementRows(coverage RepoMapNavigationCoverage, limit int) []string {
	if limit <= 0 {
		limit = 8
	}
	coverage = NormalizeRepoMapNavigationCoverage(coverage)
	if coverage.State == RepoMapNavigationCoverageNotRequired || len(coverage.MissingRoutes) == 0 {
		return nil
	}
	var out []string
	for _, route := range coverage.MissingRoutes {
		route = NormalizeRepoMapNavigationRoute(string(route))
		if !route.Valid() {
			continue
		}
		parts := []string{
			"repo_map_navigation_requirement",
			"route=" + string(route),
		}
		if coverage.ReasonCode != "" {
			parts = append(parts, "reason="+coverage.ReasonCode)
		}
		parts = append(parts, "required=repo_map_navigation_route")
		out = append(out, strings.Join(parts, " "))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeReadLocalizerFollowupStrings(in []string, limit int) []string {
	if limit <= 0 {
		limit = 16
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out
}
