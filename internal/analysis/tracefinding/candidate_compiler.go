package tracefinding

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CompileTraceDecisionCandidateSet builds the deterministic candidate universe
// from typed projection / roster authorities. It never reads Finalizer prose.
func CompileTraceDecisionCandidateSet(bus *types.BusContext) (types.TraceDecisionCandidateSetV1, error) {
	if bus == nil {
		return types.TraceDecisionCandidateSetV1{}, fmt.Errorf("nil bus context")
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 64))
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) == 0 {
		return types.TraceDecisionCandidateSetV1{}, fmt.Errorf("no trace causal projections available")
	}
	return CompileTraceDecisionCandidateSetFromProjection(set)
}

// CompileTraceDecisionCandidateSetFromProjection compiles candidates from an
// already-built projection set (shared by prompt handoff and emit validation).
func CompileTraceDecisionCandidateSetFromProjection(set types.TraceCausalProjectionSet) (types.TraceDecisionCandidateSetV1, error) {
	if len(set.Projections) == 0 {
		return types.TraceDecisionCandidateSetV1{}, fmt.Errorf("empty projection set")
	}
	// P0 single-unit finding: use the first projection as the unit scope.
	// Multi-artifact projection remains a caveat, not batch clustering.
	proj := set.Projections[0]
	rosters := types.BuildTraceRankRosterAuthorities(set)
	out := types.TraceDecisionCandidateSetV1{
		SchemaVersion:  types.TraceDecisionCandidateSetSchemaVersion,
		Artifact:       types.TraceFindingArtifact{Label: strings.TrimSpace(proj.ArtifactLabel)},
		Scope:          types.TraceFindingScope{WindowStart: proj.WindowStartTs, WindowEnd: proj.WindowEndTs},
		RosterComplete: true,
	}
	evidenceSeen := map[string]bool{}
	primarySeen := map[string]bool{}
	for _, roster := range rosters {
		if !roster.Complete {
			out.RosterComplete = false
		}
		for _, seat := range roster.Seats {
			cand, err := candidateFromSeat(roster, seat)
			if err != nil {
				continue
			}
			for _, id := range cand.EvidenceRefs {
				if id != "" && !evidenceSeen[id] {
					evidenceSeen[id] = true
					out.AcceptedEvidenceIDs = append(out.AcceptedEvidenceIDs, id)
				}
			}
			tier := strings.ToLower(strings.TrimSpace(seat.Tier))
			switch {
			case strings.Contains(tier, "context"):
				out.ContextOnly = append(out.ContextOnly, cand)
			case seat.Rank == 1 || strings.Contains(tier, "primary") || strings.Contains(tier, "on_chain") || strings.Contains(tier, "eliminable"):
				if !primarySeen[cand.CandidateID] {
					primarySeen[cand.CandidateID] = true
					out.PrimaryEligible = append(out.PrimaryEligible, cand)
				}
				out.ContributorEligible = append(out.ContributorEligible, cand)
			default:
				out.ContributorEligible = append(out.ContributorEligible, cand)
			}
		}
	}
	// Evidence-boundary tokens stay out of primary authority.
	for _, node := range append(append([]types.TraceCausalProjectionNode{}, proj.PrimaryRootCauses...), proj.RankedSeats...) {
		if !node.IsEvidenceBoundaryRow() {
			continue
		}
		code := firstNonEmpty(node.TypeToken, node.Object, node.Predicate)
		if code == "" {
			continue
		}
		boundary := types.TraceEvidenceBoundary{
			Code:       code,
			EvidenceID: strings.TrimSpace(node.EvidenceID),
			Detail:     strings.TrimSpace(node.Subject),
		}
		out.EvidenceBoundaries = append(out.EvidenceBoundaries, boundary)
		if boundary.EvidenceID != "" && !evidenceSeen[boundary.EvidenceID] {
			evidenceSeen[boundary.EvidenceID] = true
			out.AcceptedEvidenceIDs = append(out.AcceptedEvidenceIDs, boundary.EvidenceID)
		}
	}
	if len(set.Projections) > 1 {
		out.CausalCeiling.Flags = append(out.CausalCeiling.Flags, "multi_projection_unit_uses_first")
	}
	out.CandidateSetID = candidateSetID(out)
	return out, nil
}

func candidateFromSeat(roster types.TraceRankRosterAuthority, seat types.TraceRankRosterSeat) (types.TraceCauseCandidate, error) {
	token := strings.TrimSpace(seat.Type)
	if token == "" {
		return types.TraceCauseCandidate{}, fmt.Errorf("empty seat type")
	}
	snap, err := SnapshotToken(token)
	if err != nil {
		// Unregistered tokens cannot be cluster/finding authority; skip seat.
		return types.TraceCauseCandidate{}, err
	}
	id := "cand:" + token + ":" + strconv.Itoa(seat.Rank) + ":" + strings.TrimSpace(seat.Subject)
	cand := types.TraceCauseCandidate{
		CandidateID:      id,
		Token:            snap,
		Rank:             seat.Rank,
		Tier:             strings.TrimSpace(seat.Tier),
		BoardFingerprint: strings.TrimSpace(roster.BoardParamsFingerprint),
		Subject:          strings.TrimSpace(seat.Subject),
		TypeToken:        token,
		SubjectRole:      "unknown",
		CausalShape:      strings.TrimSpace(seat.ChainRelevance),
		Phase:            "unknown",
	}
	if seat.EvidenceID != "" {
		cand.EvidenceRefs = []string{strings.TrimSpace(seat.EvidenceID)}
	}
	if seat.EffectiveImpactPublished {
		cand.Magnitude = &types.TypedMagnitude{
			Value:      seat.EffectiveImpactMS,
			Unit:       "ms",
			Additivity: snap.Additivity,
			Caliber:    "effective_impact",
		}
	}
	return cand, nil
}

func candidateSetID(set types.TraceDecisionCandidateSetV1) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "v%d|%s|%.6f|%.6f|%d|%d|%d",
		set.SchemaVersion, set.Artifact.Label, set.Scope.WindowStart, set.Scope.WindowEnd,
		len(set.PrimaryEligible), len(set.ContributorEligible), len(set.AcceptedEvidenceIDs))
	for _, c := range set.PrimaryEligible {
		_, _ = fmt.Fprintf(h, "|p:%s", c.CandidateID)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
