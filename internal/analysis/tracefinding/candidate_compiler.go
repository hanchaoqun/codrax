package tracefinding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const CandidateCompilerVersion = "trace-candidate-v2"

// CompileCandidateContract freezes the deterministic candidate rows that the
// finalizer may select. It consumes typed trace records only; final answer
// prose is deliberately not an input.
func CompileCandidateContract(ledger types.ObservationLedger, set types.TraceCausalProjectionSet, authority SeatFrameCausalityAuthority) (*types.TraceFindingContract, error) {
	registryHash, err := RegistryHash()
	if err != nil {
		return nil, err
	}
	// SIDECAR-Q1 (§40.28 ②): the contract-level ceiling is DERIVED from the
	// seat-level qualifiers below (unproven iff any admitted candidate's own
	// evidence is frame-unproven) — it is a summary for the legacy Required
	// view, never an input to any candidate's qualifier.
	ceiling := types.TraceCausalQualifierProven
	if !authority.Applicable {
		// QUALGATE-1 (§40.30): not a frame question — the ceiling makes no
		// frame claim and never caps a status (only frame_unproven caps).
		ceiling = types.TraceCausalQualifierNotApplicable
	}
	contract := &types.TraceFindingContract{
		Required:             true,
		FindingSchemaVersion: types.TraceFindingSchemaVersion,
		RegistryHash:         registryHash,
		CausalCeiling:        ceiling,
		AcceptedEvidenceIDs:  acceptedEvidenceIDs(ledger),
		Scope: types.TraceFindingScope{
			ProfileFamily: "runtime_trace",
			TargetRole:    "unknown",
			Phase:         "unknown",
		},
		Symptom: types.TraceSymptomSummary{Kind: "runtime_trace"},
	}

	seen := map[string]bool{}
	for _, projection := range set.Projections {
		applyProjectionMetadata(contract, projection)
		for _, node := range candidateNodes(projection) {
			candidate, ok := compileCandidate(projection, node, registryHash, authority)
			if !ok || seen[candidate.Decision.CandidateID] {
				continue
			}
			seen[candidate.Decision.CandidateID] = true
			if candidate.Decision.CausalQualifier == types.TraceCausalQualifierFrameUnproven {
				contract.CausalCeiling = types.TraceCausalQualifierFrameUnproven
			}
			contract.Candidates = append(contract.Candidates, candidate)
			if candidate.PrimaryEligible {
				contract.PrimaryCandidateIDs = append(contract.PrimaryCandidateIDs, candidate.Decision.CandidateID)
			}
			if candidate.ContributorEligible {
				contract.ContributorCandidateIDs = append(contract.ContributorCandidateIDs, candidate.Decision.CandidateID)
			}
			contract.AcceptedEvidenceIDs = append(contract.AcceptedEvidenceIDs, candidate.Decision.EvidenceRefs...)
		}
	}
	contract.AcceptedEvidenceIDs = sortedUnique(contract.AcceptedEvidenceIDs)
	contract.PrimaryCandidateIDs = sortedUnique(contract.PrimaryCandidateIDs)
	contract.ContributorCandidateIDs = sortedUnique(contract.ContributorCandidateIDs)
	sort.SliceStable(contract.Candidates, func(i, j int) bool {
		a, b := contract.Candidates[i].Decision, contract.Candidates[j].Decision
		if a.Rank != b.Rank {
			return rankSortValue(a.Rank) < rankSortValue(b.Rank)
		}
		return a.CandidateID < b.CandidateID
	})
	if len(contract.Candidates) > 0 {
		first := contract.Candidates[0].Decision
		contract.Scope.Phase = first.Phase
		if contract.Scope.TargetRole == "unknown" {
			contract.Scope.TargetRole = first.SubjectRole
		}
	}

	identity := struct {
		Version      string                          `json:"version"`
		RegistryHash string                          `json:"registry_hash"`
		Ceiling      string                          `json:"causal_ceiling"`
		Artifact     types.TraceFindingArtifact      `json:"artifact"`
		Scope        types.TraceFindingScope         `json:"scope"`
		Candidates   []types.TraceFindingCandidateV1 `json:"candidates"`
	}{CandidateCompilerVersion, registryHash, contract.CausalCeiling, contract.Artifact, contract.Scope, contract.Candidates}
	b, err := json.Marshal(identity)
	if err != nil {
		return nil, fmt.Errorf("marshal trace candidate contract: %w", err)
	}
	sum := sha256.Sum256(b)
	contract.CandidateSetID = "candidate-set-" + hex.EncodeToString(sum[:])
	contract.ContractHash = hex.EncodeToString(sum[:])
	contract.AnalysisKey = "trace-analysis-" + hex.EncodeToString(sum[:])
	contract.FindingID = "finding-" + hex.EncodeToString(sum[:16])
	return contract, nil
}

// RegistryHash snapshots the causal-token meanings used by a finding.
func RegistryHash() (string, error) {
	type registryRow struct {
		Token        string `json:"token"`
		Lane         string `json:"lane"`
		Additivity   string `json:"additivity"`
		SubjectKind  string `json:"subject_kind"`
		FixDirection string `json:"fix_direction"`
		RowToken     bool   `json:"row_token"`
	}
	rows := make([]registryRow, 0, len(tracequery.CausalTokenUniverse()))
	for _, token := range tracequery.CausalTokenUniverse() {
		spec, ok := tracequery.CausalTokenSpecFor(token)
		if !ok {
			continue
		}
		rows = append(rows, registryRow{
			Token: token, Lane: string(spec.Lane), Additivity: string(spec.Additivity),
			SubjectKind: string(spec.Subject), FixDirection: string(tracequery.CausalTokenFixDirectionFor(token)), RowToken: spec.RowToken,
		})
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("marshal causal token registry: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func candidateNodes(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	if len(projection.RankedSeats) > 0 {
		return projection.RankedSeats
	}
	out := make([]types.TraceCausalProjectionNode, 0, len(projection.PrimaryRootCauses)+len(projection.OnChainCauses))
	out = append(out, projection.PrimaryRootCauses...)
	out = append(out, projection.OnChainCauses...)
	if projection.PrimaryRootCause != nil {
		out = append(out, *projection.PrimaryRootCause)
	}
	return out
}

func compileCandidate(projection types.TraceCausalProjection, node types.TraceCausalProjectionNode, registryHash string, authority SeatFrameCausalityAuthority) (types.TraceFindingCandidateV1, bool) {
	if node.Rank <= 0 || node.IsTargetSelfStateRow() || node.IsContextOnlyRow() ||
		node.IsCaliberSideRow() || node.IsEvidenceBoundaryRow() || node.IsAggregateMetric() ||
		node.OnChainOverflowFold || node.Unit == types.TraceObservationUnitCompositeScore ||
		(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
		return types.TraceFindingCandidateV1{}, false
	}
	token := firstNonEmpty(strings.ToLower(strings.TrimSpace(node.TypeToken)), strings.ToLower(strings.TrimSpace(node.Object)), strings.ToLower(strings.TrimSpace(node.Predicate)))
	spec, ok := tracequery.CausalTokenSpecFor(token)
	if !ok || !spec.RowToken {
		return types.TraceFindingCandidateV1{}, false
	}
	evidence := sortedUnique(append([]string{strings.TrimSpace(node.EvidenceID)}, node.SupportRefs...))
	if len(evidence) == 0 {
		return types.TraceFindingCandidateV1{}, false
	}
	phase := "unknown"
	if node.ChainDepth > 0 {
		phase = "pre_wakeup_dependency"
	}
	subjectRole, upstreamRole := candidateRoles(projection, node)
	shape := firstNonEmpty(strings.TrimSpace(node.Causality), strings.TrimSpace(node.ChainRelevance), strings.TrimSpace(node.BlockingKind), "ranked_cause")
	value := node.ImpactMS
	caliber := "window_projection"
	if node.EffectiveImpactPublished {
		value = node.EffectiveImpactMS
		caliber = "effective_attribution"
	}
	if value <= 0 {
		return types.TraceFindingCandidateV1{}, false
	}
	var magnitude *types.TypedMagnitude
	unit := strings.TrimSpace(node.Unit)
	if unit == "" {
		unit = "ms"
	}
	magnitude = &types.TypedMagnitude{
		Value: value, Unit: unit, Additivity: string(spec.Additivity), Caliber: caliber,
		WindowDuration: projection.WindowDurationMS(),
	}
	if node.SupplyFoldComputed || node.DStateRefinedNonIO || node.DStateSplitMS > 0 || node.IOWaitSplitMS > 0 {
		magnitude.Components = &types.TraceMagnitudeComponents{
			SupplyFoldComputed: node.SupplyFoldComputed, SupplyFoldDeficitMS: node.SupplyFoldDeficitMS,
			SupplyFoldIdealMS: node.SupplyFoldIdealMS, SupplyFoldKnownMS: node.SupplyFoldKnownMS,
			SupplyFoldUnknownMS: node.SupplyFoldUnknownMS, SupplyFoldCapabilitySource: node.SupplyFoldCapabilitySource,
			DStateRefinedNonIO: node.DStateRefinedNonIO, DStateMS: node.DStateSplitMS, IOWaitMS: node.IOWaitSplitMS,
		}
	}
	// SIDECAR-Q1: seat-level qualifier from the candidate's OWN evidence IDs
	// (the exact provider the crown face consults) — never a session
	// aggregate; QUALGATE-1: not_applicable when the typed request gate is
	// closed.
	qualifier := authority.SeatQualifier(append([]string{node.EvidenceID}, node.MergedEvidenceIDs...)...)
	status := types.TraceCausalSupportedCandidate
	if qualifier != types.TraceCausalQualifierFrameUnproven && candidateCausalityExplicitlyProven(node.Causality) {
		status = types.TraceCausalProven
	}
	decision := types.TraceCauseDecision{
		Status:          status,
		CausalQualifier: qualifier,
		Token: types.TraceCausalTokenSnapshot{
			Token: token, Lane: string(spec.Lane), Additivity: string(spec.Additivity),
			SubjectKind: string(spec.Subject), FixDirection: string(tracequery.CausalTokenFixDirectionFor(token)), RegistryHash: registryHash,
		},
		SubjectName: strings.TrimSpace(node.Subject), SubjectRole: subjectRole, UpstreamRole: upstreamRole,
		ResourceName: firstNonEmpty(strings.TrimSpace(node.SpanName), strings.TrimSpace(node.BlockingHolderSite), strings.TrimSpace(node.BlockingFromSite)),
		PhaseName:    firstNonEmpty(strings.TrimSpace(node.SpanName), strings.TrimSpace(node.SpanCategory), strings.TrimSpace(node.SemanticClass)),
		BlockingKind: strings.TrimSpace(node.BlockingKind), CausalShape: shape, Phase: phase,
		Rank: node.Rank, Tier: strings.TrimSpace(node.Tier), BoardFingerprint: strings.TrimSpace(node.RankBoardParamsFingerprint),
		NormalizedEventKey: token, NormalizedStackKey: firstNonEmpty(strings.TrimSpace(node.BlockingHolderSite), strings.TrimSpace(node.BlockingFromSite)),
		Magnitude: magnitude, EvidenceRefs: evidence, Confidence: confidenceLabel(node.Confidence),
	}
	decision.CandidateID = candidateID(projection, node, decision)
	// Root-cause selection is stricter than contributor retention: only the
	// producer's exact typed on-chain lane may be selected. An empty, adjacent,
	// or background relevance never becomes a root merely because it happened
	// to receive a rank.
	onChain := strings.EqualFold(strings.TrimSpace(node.ChainRelevance), "on_chain")
	return types.TraceFindingCandidateV1{
		PrimaryEligible:     onChain,
		ContributorEligible: true,
		Decision:            decision,
	}, true
}

func candidateCausalityExplicitlyProven(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "proven" || strings.HasPrefix(value, "proven_")
}

func applyProjectionMetadata(contract *types.TraceFindingContract, projection types.TraceCausalProjection) {
	if contract.Artifact.ArtifactID == "" {
		identity := firstNonEmpty(strings.TrimSpace(projection.ArtifactPath), strings.TrimSpace(projection.ArtifactLabel), "runtime-trace")
		sum := sha256.Sum256([]byte(identity))
		contract.Artifact = types.TraceFindingArtifact{
			ArtifactID: "artifact-" + hex.EncodeToString(sum[:16]), DisplayLabel: strings.TrimSpace(projection.ArtifactLabel),
		}
	}
	if account := projection.TargetStateAccount; account != nil {
		if subject := strings.TrimSpace(account.Subject); subject != "" {
			contract.Scope.TargetRole = "target_thread"
		}
		if account.TotalMS > 0 {
			contract.Symptom = types.TraceSymptomSummary{Kind: "target_window_duration", Value: account.TotalMS, Unit: "ms"}
		}
	}
}

func candidateRoles(projection types.TraceCausalProjection, node types.TraceCausalProjectionNode) (string, string) {
	if node.IsAggregateMetric() {
		return "aggregate_metric", ""
	}
	if node.BlockingSubjectIsHolder {
		return "lock_holder", "target_thread"
	}
	target := ""
	if projection.TargetStateAccount != nil {
		target = strings.TrimSpace(projection.TargetStateAccount.Subject)
	}
	if target != "" && strings.EqualFold(strings.TrimSpace(node.Subject), target) {
		return "target_thread", ""
	}
	if node.ChainDepth > 0 {
		return "upstream_dependency", "target_thread"
	}
	return "causal_worker", ""
}

func candidateID(projection types.TraceCausalProjection, node types.TraceCausalProjectionNode, decision types.TraceCauseDecision) string {
	parts := []string{
		CandidateCompilerVersion, projection.ArtifactPath, projection.ArtifactLabel,
		node.EvidenceID, node.RankBoardParamsFingerprint, decision.Token.Token,
		fmt.Sprintf("%d", node.Rank), decision.SubjectRole, decision.Phase,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "candidate-" + hex.EncodeToString(sum[:16])
}

func acceptedEvidenceIDs(ledger types.ObservationLedger) []string {
	var out []string
	for _, record := range ledger.Records {
		out = append(out, record.ID)
		out = append(out, record.SupportRefs...)
	}
	return sortedUnique(out)
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func rankSortValue(rank int) int {
	if rank <= 0 {
		return int(^uint(0) >> 1)
	}
	return rank
}

func confidenceLabel(value float64) string {
	switch {
	case value >= 0.8:
		return "high"
	case value >= 0.5:
		return "medium"
	case value > 0:
		return "low"
	default:
		return "unknown"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
