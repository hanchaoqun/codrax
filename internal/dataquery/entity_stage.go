package dataquery

import "strings"

// EntityResolutionObligationSatisfied is the single typed predicate for
// whether a data workflow's entity-resolution obligation is discharged. It is
// deliberately double-faced and every satisfaction judgment MUST call it:
//
//   - routing face (internal/dataworkflow): StageFacts.EntityLedgerSatisfied
//     feeds NextStage, MissingValidationStages, HasPostRuleProgress,
//     BuildLedgerGraph "Present"/prerequisites, and
//     ResultIsFinalAnswerCandidate;
//   - validator face (this package): validateRequiredLedgers.
//
// records is the number of entity_resolution records; materialized reports
// that a successful normalize_entities/enrich_records/join_records artifact
// already canonicalized entities onto the working records
// (ArtifactMaterializesEntityStage). A materialized entity stage counts as
// discharged even with zero explicit resolution records: join/enrich outputs
// carry the canonical fields themselves, so demanding a separate ledger that
// the emit-stage action set cannot produce is self-contradictory. When the two
// faces re-derived this judgment independently ("Present = records>0 ||
// materialized" vs "satisfied = records>0"), the stage machine advanced past
// the only entity producers while the validator kept demanding the ledger —
// six repair rounds all hard-rejected and a correct, already-computed answer
// was withheld (eval data_multifile_reference_projection run-2, 2026-07-19;
// audit GAP-3/G10, campaign ledger §29.140/§29.142). Divergent re-derivations
// are pinned out in both directions by
// TestEntityObligationSingleGatePredicate_SourcePin and
// TestEntityObligationTwoAuthoritiesSameJudgment.
func EntityResolutionObligationSatisfied(records int, materialized bool) bool {
	return records > 0 || materialized
}

// LedgerSatisfactionFacts carries the structural facts that participate in
// required-ledger satisfaction but do not live on the Result payload itself.
type LedgerSatisfactionFacts struct {
	// EntityStageMaterialized reports whether any successful round produced
	// an artifact that materializes the entity stage. Cross-round workflow
	// validation must derive it from the full record history (the repl
	// workflow record scan); single-plan validation derives it from the
	// result's own artifacts (ResultMaterializesEntityStage).
	EntityStageMaterialized bool
}

// ArtifactMaterializesEntityStage reports whether an artifact (or any of its
// children) materializes the entity stage. This is the single artifact-level
// authority for that judgment: the repl workflow record scan and the
// runner-local result scan both delegate here, so the routing face and the
// validator face can never disagree about which artifacts count.
func ArtifactMaterializesEntityStage(artifact DataArtifact) bool {
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	if strings.Contains(kind, string(DataActionNormalizeEntities)) ||
		strings.Contains(kind, string(DataActionEnrichRecords)) ||
		strings.Contains(kind, string(DataActionJoinRecords)) {
		return true
	}
	for _, child := range artifact.Children {
		if ArtifactMaterializesEntityStage(child) {
			return true
		}
	}
	return false
}

// ResultMaterializesEntityStage reports whether the result's own artifacts
// materialize the entity stage. Plan-local validation (validateRunnerResult)
// consumes this; cross-round workflow validation additionally scans the full
// record history before calling ValidateResultAgainstContract.
func ResultMaterializesEntityStage(res Result) bool {
	for _, artifact := range res.Artifacts {
		if ArtifactMaterializesEntityStage(artifact) {
			return true
		}
	}
	return false
}
