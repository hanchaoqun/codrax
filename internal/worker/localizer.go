package worker

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

type LocalizerInput struct {
	ID             string                                `json:"id,omitempty"`
	RunID          string                                `json:"run_id,omitempty"`
	UnitID         string                                `json:"unit_id,omitempty"`
	Goal           string                                `json:"goal,omitempty"`
	BatchID        string                                `json:"batch_id,omitempty"`
	SliceID        string                                `json:"slice_id,omitempty"`
	Review         *types.SourceLocalizationReview       `json:"review,omitempty"`
	Authority      *loopkernel.LocalizationAuthorityView `json:"authority,omitempty"`
	Navigation     *types.RepoMapNavigationCoverage      `json:"navigation,omitempty"`
	CandidatePaths []string                              `json:"candidate_paths,omitempty"`
	Anchors        []types.SourceLocalizationAnchor      `json:"anchors,omitempty"`
	EvidenceRefs   []types.WriteExplorationEvidenceRef   `json:"evidence_refs,omitempty"`
	InputRefs      []loopkernel.ArtifactRef              `json:"input_refs,omitempty"`
	Budget         Budget                                `json:"budget,omitempty"`
}

type LocalizerOutput struct {
	Request      Request                              `json:"request,omitempty"`
	Result       Result                               `json:"result,omitempty"`
	Review       types.SourceLocalizationReview       `json:"review,omitempty"`
	Authority    loopkernel.LocalizationAuthorityView `json:"authority,omitempty"`
	Navigation   types.RepoMapNavigationCoverage      `json:"navigation,omitempty"`
	Requirements types.LocalizationRequirementSet     `json:"requirements,omitempty"`
}

func NormalizeLocalizerInput(in LocalizerInput) LocalizerInput {
	in.ID = strings.TrimSpace(in.ID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.UnitID = strings.TrimSpace(in.UnitID)
	in.Goal = strings.TrimSpace(in.Goal)
	in.BatchID = strings.TrimSpace(in.BatchID)
	in.SliceID = strings.TrimSpace(in.SliceID)
	in.CandidatePaths = NormalizeScope(Scope{Paths: in.CandidatePaths}).Paths
	in.InputRefs = normalizeArtifactRefs(in.InputRefs)
	in.Budget = NormalizeBudget(in.Budget)
	if in.Review != nil {
		review := types.NormalizeSourceLocalizationReview(*in.Review)
		in.Review = &review
	}
	if in.Navigation != nil {
		coverage := types.NormalizeRepoMapNavigationCoverage(*in.Navigation)
		in.Navigation = &coverage
	}
	in.Anchors = normalizeLocalizerAnchors(in.Anchors)
	in.EvidenceRefs = normalizeLocalizerEvidenceRefs(in.EvidenceRefs)
	return in
}

func BuildLocalizerRequest(in LocalizerInput) Request {
	in = NormalizeLocalizerInput(in)
	paths := append([]string(nil), in.CandidatePaths...)
	if in.Review != nil {
		paths = append(paths, in.Review.SourcePaths...)
		paths = append(paths, in.Review.OwnerMissingPaths...)
		paths = append(paths, in.Review.MissingPaths...)
	}
	if in.Authority != nil {
		paths = append(paths, in.Authority.SourcePaths...)
		paths = append(paths, in.Authority.OwnerMissingPaths...)
	}
	for _, anchor := range in.Anchors {
		paths = append(paths, anchor.Path)
	}
	scope := NormalizeScope(Scope{Paths: paths})
	inputRefs := append([]loopkernel.ArtifactRef(nil), in.InputRefs...)
	if in.Review != nil {
		inputRefs = append(inputRefs, localizerArtifactRef("source_localization_review", *in.Review))
	}
	if in.Authority != nil {
		inputRefs = append(inputRefs, localizerArtifactRef("localization_authority", *in.Authority))
	}
	if in.Navigation != nil {
		inputRefs = append(inputRefs, localizerArtifactRef("repo_map_navigation_coverage", *in.Navigation))
	}
	req := Request{
		ID:        in.ID,
		RunID:     in.RunID,
		UnitID:    in.UnitID,
		Kind:      KindLocalizer,
		Goal:      firstNonEmpty(in.Goal, "produce source owner localization evidence"),
		Scope:     scope,
		InputRefs: inputRefs,
		Budget:    in.Budget,
	}
	if req.ID == "" {
		req.ID = "localizer-" + shortHash(req)
	}
	return NormalizeRequest(req)
}

func RunLocalizer(in LocalizerInput) LocalizerOutput {
	in = NormalizeLocalizerInput(in)
	request := BuildLocalizerRequest(in)
	review := BuildLocalizerReview(in)
	authority := loopkernel.DeriveLocalizationAuthority(&review)
	requirements := types.LocalizationRequirementsFromSourceLocalizationReview(review, 0)
	navigation := types.RepoMapNavigationCoverage{}
	if in.Navigation != nil {
		navigation = types.NormalizeRepoMapNavigationCoverage(*in.Navigation)
	}
	outputRefs := []loopkernel.ArtifactRef{
		localizerArtifactRef("source_localization_review", review),
		localizerArtifactRef("localization_authority", authority),
	}
	if len(requirements.Items) > 0 {
		outputRefs = append(outputRefs, localizerArtifactRef("localization_requirements", requirements))
	}
	if navigation.State != "" {
		outputRefs = append(outputRefs, localizerArtifactRef("repo_map_navigation_coverage", navigation))
	}
	result := Result{
		RequestID:    request.ID,
		RunID:        request.RunID,
		UnitID:       request.UnitID,
		Kind:         KindLocalizer,
		Status:       StatusComplete,
		ReasonCode:   authority.ReasonCode,
		OutputRefs:   outputRefs,
		EvidenceRefs: localizerEvidenceRefIDs(review, navigation),
	}
	return LocalizerOutput{
		Request:      request,
		Result:       NormalizeResult(result),
		Review:       review,
		Authority:    authority,
		Navigation:   navigation,
		Requirements: requirements,
	}
}

func LocalizerInputFromReadArtifacts(id string, ir *types.AnalysisIR, lanes types.ExploreLanePlan, turnA *types.TurnAArtifacts) LocalizerInput {
	input := LocalizerInput{ID: strings.TrimSpace(id)}
	if turnA == nil {
		return NormalizeLocalizerInput(input)
	}
	review := types.SourceLocalizationReviewFromTurnAArtifacts(turnA)
	if review != nil {
		input.Review = review
		input.CandidatePaths = append(input.CandidatePaths, review.SourcePaths...)
		input.CandidatePaths = append(input.CandidatePaths, review.OwnerMissingPaths...)
		input.EvidenceRefs = append(input.EvidenceRefs, review.EvidenceRefs...)
		input.InputRefs = append(input.InputRefs, localizerArtifactRef("read_source_localization", *review))
	}
	if ir != nil {
		coverage := types.RepoMapNavigationCoverageFromReadArtifacts(ir, lanes, turnA)
		coverage = types.NormalizeRepoMapNavigationCoverage(coverage)
		if coverage.State != "" && coverage.State != types.RepoMapNavigationCoverageNotRequired {
			input.Navigation = &coverage
			input.InputRefs = append(input.InputRefs, localizerArtifactRef("read_navigation_coverage", coverage))
		}
	}
	return NormalizeLocalizerInput(input)
}

func LocalizerInputFromWriteWorkflowRun(id string, run types.WriteWorkflowRun, batchID string) LocalizerInput {
	run = types.NormalizeWriteWorkflowRun(run)
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		batchID = run.ActiveBatchID
	}
	input := LocalizerInput{
		ID:      strings.TrimSpace(id),
		RunID:   run.RunID,
		Goal:    run.Goal,
		BatchID: batchID,
	}
	review := loopkernel.LocalizationReviewFromWriteWorkflowRun(run, batchID)
	if review != nil {
		input.Review = review
		input.CandidatePaths = append(input.CandidatePaths, review.SourcePaths...)
		input.CandidatePaths = append(input.CandidatePaths, review.OwnerMissingPaths...)
		input.EvidenceRefs = append(input.EvidenceRefs, review.EvidenceRefs...)
		input.InputRefs = append(input.InputRefs, localizerArtifactRef("write_source_localization", *review))
		authority := loopkernel.DeriveLocalizationAuthority(review)
		input.Authority = &authority
		input.InputRefs = append(input.InputRefs, localizerArtifactRef("write_localization_authority", authority))
	}
	coverage := types.RepoMapNavigationCoverageFromWriteContextPacks(run.ContextPacks, types.WriteConsumerController, batchID, "")
	coverage = types.NormalizeRepoMapNavigationCoverage(coverage)
	if coverage.State != "" && coverage.State != types.RepoMapNavigationCoverageNotRequired {
		input.Navigation = &coverage
		input.InputRefs = append(input.InputRefs, localizerArtifactRef("write_navigation_coverage", coverage))
	}
	return NormalizeLocalizerInput(input)
}

func BuildLocalizerReview(in LocalizerInput) types.SourceLocalizationReview {
	in = NormalizeLocalizerInput(in)
	review := types.SourceLocalizationReview{
		Source:  "localizer_worker",
		BatchID: in.BatchID,
		Goal:    in.Goal,
	}
	addPath := func(raw string) {
		scope := NormalizeScope(Scope{Paths: []string{raw}})
		if len(scope.Paths) == 0 {
			return
		}
		path := scope.Paths[0]
		role := types.ClassifySourcePathRole(path)
		if types.SourcePathRoleIsAuxiliary(role) {
			review.AuxiliaryPaths = append(review.AuxiliaryPaths, path)
			return
		}
		review.SourcePaths = append(review.SourcePaths, path)
		review.Anchors = append(review.Anchors, types.SourceLocalizationAnchor{
			Path:        path,
			Role:        role,
			SourceStage: "localizer_worker",
			Kind:        types.SourceLocalizationAnchorScope,
			Strength:    types.SourceLocalizationAnchorSupporting,
			ReasonCode:  "localizer_candidate_requires_owner_anchor",
		})
	}
	for _, path := range in.CandidatePaths {
		addPath(path)
	}
	if in.Authority != nil {
		for _, path := range in.Authority.SourcePaths {
			addPath(path)
		}
		for _, path := range in.Authority.OwnerMissingPaths {
			addPath(path)
		}
	}
	for _, anchor := range in.Anchors {
		review.Anchors = append(review.Anchors, anchor)
		addPath(anchor.Path)
		if anchor.Strength == types.SourceLocalizationAnchorOwner && !types.SourcePathRoleIsAuxiliary(anchor.Role) {
			review.OwnerSupportedPaths = append(review.OwnerSupportedPaths, anchor.Path)
			review.SupportedPaths = append(review.SupportedPaths, anchor.Path)
		}
		if anchor.EvidenceRef != nil {
			review.EvidenceRefs = append(review.EvidenceRefs, *anchor.EvidenceRef)
		}
	}
	review.EvidenceRefs = append(review.EvidenceRefs, in.EvidenceRefs...)
	review = types.NormalizeSourceLocalizationReview(review)
	if in.Review != nil {
		if merged := types.MergeSourceLocalizationReviews(in.Review, &review); merged != nil {
			review = *merged
		}
	}
	review.Source = firstNonEmpty(review.Source, "localizer_worker")
	review.BatchID = firstNonEmpty(review.BatchID, in.BatchID)
	review.Goal = firstNonEmpty(review.Goal, in.Goal)
	review.OwnerMissingPaths = append(review.OwnerMissingPaths, localizerOwnerMissingPaths(review)...)
	if len(review.SourcePaths) > 0 && len(review.OwnerSupportedPaths) == 0 {
		review.ReasonCodes = append(review.ReasonCodes, "localizer_owner_anchor_missing")
		review.Status = types.SourceLocalizationWeak
	} else if len(review.OwnerMissingPaths) > 0 {
		review.ReasonCodes = append(review.ReasonCodes, "localizer_owner_anchor_partial")
		review.Status = types.SourceLocalizationWeak
	}
	if len(review.OwnerSupportedPaths) > 0 && len(review.OwnerMissingPaths) == 0 {
		review.Status = types.SourceLocalizationSupported
		review.ReasonCodes = append(review.ReasonCodes, "localizer_owner_anchor_supported")
	}
	review.EvidenceRefs = append(review.EvidenceRefs, localizerAnchorEvidenceRefs(review.Anchors)...)
	return types.NormalizeSourceLocalizationReview(review)
}

func localizerOwnerMissingPaths(review types.SourceLocalizationReview) []string {
	supported := map[string]bool{}
	for _, path := range review.OwnerSupportedPaths {
		supported[strings.TrimSpace(path)] = true
	}
	seenMissing := map[string]bool{}
	for _, path := range review.OwnerMissingPaths {
		seenMissing[strings.TrimSpace(path)] = true
	}
	var out []string
	for _, path := range review.SourcePaths {
		path = strings.TrimSpace(path)
		if path == "" || supported[path] || seenMissing[path] {
			continue
		}
		out = append(out, path)
		seenMissing[path] = true
	}
	return out
}

func normalizeLocalizerAnchors(in []types.SourceLocalizationAnchor) []types.SourceLocalizationAnchor {
	if len(in) == 0 {
		return nil
	}
	review := types.NormalizeSourceLocalizationReview(types.SourceLocalizationReview{Anchors: append([]types.SourceLocalizationAnchor(nil), in...)})
	return append([]types.SourceLocalizationAnchor(nil), review.Anchors...)
}

func normalizeLocalizerEvidenceRefs(in []types.WriteExplorationEvidenceRef) []types.WriteExplorationEvidenceRef {
	if len(in) == 0 {
		return nil
	}
	review := types.NormalizeSourceLocalizationReview(types.SourceLocalizationReview{EvidenceRefs: append([]types.WriteExplorationEvidenceRef(nil), in...)})
	return append([]types.WriteExplorationEvidenceRef(nil), review.EvidenceRefs...)
}

func localizerAnchorEvidenceRefs(anchors []types.SourceLocalizationAnchor) []types.WriteExplorationEvidenceRef {
	var refs []types.WriteExplorationEvidenceRef
	for _, anchor := range anchors {
		if anchor.EvidenceRef != nil {
			refs = append(refs, *anchor.EvidenceRef)
		}
	}
	return normalizeLocalizerEvidenceRefs(refs)
}

func localizerEvidenceRefIDs(review types.SourceLocalizationReview, navigation types.RepoMapNavigationCoverage) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, ref := range review.EvidenceRefs {
		add(ref.ID)
		if ref.ID == "" {
			add(ref.Source)
		}
	}
	for _, anchor := range review.Anchors {
		if anchor.EvidenceRef == nil {
			continue
		}
		add(anchor.EvidenceRef.ID)
		if anchor.EvidenceRef.ID == "" {
			add(anchor.EvidenceRef.Source)
		}
	}
	for _, ref := range navigation.EvidenceRefs {
		add(ref)
	}
	return out
}

func localizerArtifactRef(kind string, value any) loopkernel.ArtifactRef {
	return loopkernel.ArtifactRef{
		Kind: strings.TrimSpace(kind),
		ID:   strings.TrimSpace(kind) + ":" + shortHash(value),
	}
}

func shortHash(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte(fmt.Sprintf("%#v", value))
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:8])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
