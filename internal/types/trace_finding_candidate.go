package types

// TraceDecisionCandidateSetSchemaVersion pins the candidate-set carrier.
const TraceDecisionCandidateSetSchemaVersion = 1

// TraceCausalCeiling captures system-owned causal upper bounds for one unit.
type TraceCausalCeiling struct {
	ConclusionUnproven bool     `json:"conclusion_unproven,omitempty"`
	Flags              []string `json:"flags,omitempty"`
	Detail             string   `json:"detail,omitempty"`
}

// TraceEvidenceBoundary is a typed evidence-boundary note (e.g. missing_wakeup).
type TraceEvidenceBoundary struct {
	Code       string `json:"code,omitempty"`
	EvidenceID string `json:"evidence_id,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// TraceCauseCandidate is one program-compiled selectable cause.
type TraceCauseCandidate struct {
	CandidateID      string                   `json:"candidate_id"`
	Token            TraceCausalTokenSnapshot `json:"token"`
	SubjectRole      string                   `json:"subject_role,omitempty"`
	UpstreamRole     string                   `json:"upstream_role,omitempty"`
	CausalShape      string                   `json:"causal_shape,omitempty"`
	Phase            string                   `json:"phase,omitempty"`
	Rank             int                      `json:"rank,omitempty"`
	Tier             string                   `json:"tier,omitempty"`
	BoardFingerprint string                   `json:"board_fingerprint,omitempty"`
	Magnitude        *TypedMagnitude          `json:"magnitude,omitempty"`
	EvidenceRefs     []string                 `json:"evidence_refs,omitempty"`
	Subject          string                   `json:"subject,omitempty"`
	TypeToken        string                   `json:"type_token,omitempty"`
}

// TraceDecisionCandidateSetV1 is the deterministic candidate universe that
// Finalizer prompt rendering and TraceFinding validation share.
type TraceDecisionCandidateSetV1 struct {
	SchemaVersion       int                     `json:"schema_version"`
	CandidateSetID      string                  `json:"candidate_set_id"`
	Artifact            TraceFindingArtifact    `json:"artifact"`
	Scope               TraceFindingScope       `json:"scope"`
	CausalCeiling       TraceCausalCeiling      `json:"causal_ceiling"`
	Symptom             TraceSymptomSummary     `json:"symptom,omitempty"`
	PrimaryEligible     []TraceCauseCandidate   `json:"primary_eligible"`
	ContributorEligible []TraceCauseCandidate   `json:"contributor_eligible"`
	ContextOnly         []TraceCauseCandidate   `json:"context_only,omitempty"`
	EvidenceBoundaries  []TraceEvidenceBoundary `json:"evidence_boundaries,omitempty"`
	AcceptedEvidenceIDs []string                `json:"accepted_evidence_ids,omitempty"`
	RosterComplete      bool                    `json:"roster_complete,omitempty"`
}
