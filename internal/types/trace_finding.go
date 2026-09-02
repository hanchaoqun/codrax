package types

// TraceFindingSchemaVersion is the persisted single-trace finding schema.
const TraceFindingSchemaVersion = 1

// TraceFindingContract carries the legacy batch candidate snapshot plus the
// independent user-facing root-cause report activation for trace analyses.
type TraceFindingContract struct {
	Required                bool                      `json:"required"`
	RootCauseReportEnabled  bool                      `json:"root_cause_report_enabled,omitempty"`
	CandidateSetID          string                    `json:"candidate_set_id"`
	FindingSchemaVersion    int                       `json:"finding_schema_version"`
	PrimaryCandidateIDs     []string                  `json:"primary_candidate_ids"`
	ContributorCandidateIDs []string                  `json:"contributor_candidate_ids"`
	Candidates              []TraceFindingCandidateV1 `json:"candidates,omitempty"`
	AcceptedEvidenceIDs     []string                  `json:"accepted_evidence_ids"`
	RegistryHash            string                    `json:"registry_hash"`
	CausalCeiling           string                    `json:"causal_ceiling"`
	Artifact                TraceFindingArtifact      `json:"artifact"`
	Scope                   TraceFindingScope         `json:"scope"`
	Symptom                 TraceSymptomSummary       `json:"symptom"`
	FindingID               string                    `json:"finding_id"`
	AnalysisKey             string                    `json:"analysis_key"`
	ContractHash            string                    `json:"contract_hash"`
}

// TraceFindingCandidateV1 is a deterministic candidate snapshot compiled
// from one trace's typed rank/projection records. The finalizer may choose a
// candidate, but it may not rewrite these system-owned fields.
type TraceFindingCandidateV1 struct {
	PrimaryEligible     bool               `json:"primary_eligible"`
	ContributorEligible bool               `json:"contributor_eligible"`
	Decision            TraceCauseDecision `json:"decision"`
}

type TraceCausalStatus string

const (
	TraceCausalProven             TraceCausalStatus = "proven"
	TraceCausalSupportedCandidate TraceCausalStatus = "supported_candidate"
	TraceCausalUnresolved         TraceCausalStatus = "unresolved"
)

// TraceCausalTokenSnapshot freezes the registry semantics used by one run.
// It deliberately uses strings so types does not depend on tracequery.
type TraceCausalTokenSnapshot struct {
	Token        string `json:"token"`
	Lane         string `json:"lane"`
	Additivity   string `json:"additivity"`
	SubjectKind  string `json:"subject_kind"`
	FixDirection string `json:"fix_direction,omitempty"`
	RegistryHash string `json:"registry_hash"`
}

type TypedMagnitude struct {
	Value          float64 `json:"value"`
	Unit           string  `json:"unit"`
	Additivity     string  `json:"additivity"`
	Caliber        string  `json:"caliber"`
	WindowDuration float64 `json:"window_duration_ms,omitempty"`
	// Components preserve the producer's accounting, not a second estimate.
	// They distinguish a folded running deficit from raw work and D from IO.
	Components *TraceMagnitudeComponents `json:"components,omitempty"`
}

type TraceMagnitudeComponents struct {
	SupplyFoldComputed         bool    `json:"supply_fold_computed,omitempty"`
	SupplyFoldDeficitMS        float64 `json:"supply_fold_deficit_ms,omitempty"`
	SupplyFoldIdealMS          float64 `json:"supply_fold_ideal_ms,omitempty"`
	SupplyFoldKnownMS          float64 `json:"supply_fold_known_ms,omitempty"`
	SupplyFoldUnknownMS        float64 `json:"supply_fold_unknown_ms,omitempty"`
	SupplyFoldCapabilitySource string  `json:"supply_fold_capability_source,omitempty"`
	DStateRefinedNonIO         bool    `json:"d_state_refined_non_io,omitempty"`
	DStateMS                   float64 `json:"d_state_ms,omitempty"`
	IOWaitMS                   float64 `json:"io_wait_ms,omitempty"`
}

type TraceCauseDecision struct {
	CandidateID        string                   `json:"candidate_id"`
	Status             TraceCausalStatus        `json:"status"`
	Token              TraceCausalTokenSnapshot `json:"token"`
	SubjectName        string                   `json:"subject_name,omitempty"`
	SubjectRole        string                   `json:"subject_role"`
	UpstreamRole       string                   `json:"upstream_role,omitempty"`
	ResourceName       string                   `json:"resource_name,omitempty"`
	PhaseName          string                   `json:"phase_name,omitempty"`
	BlockingKind       string                   `json:"blocking_kind,omitempty"`
	CausalShape        string                   `json:"causal_shape"`
	Phase              string                   `json:"phase"`
	Rank               int                      `json:"rank,omitempty"`
	Tier               string                   `json:"tier,omitempty"`
	BoardFingerprint   string                   `json:"board_fingerprint,omitempty"`
	NormalizedEventKey string                   `json:"normalized_event_key,omitempty"`
	NormalizedStackKey string                   `json:"normalized_stack_key,omitempty"`
	Magnitude          *TypedMagnitude          `json:"magnitude,omitempty"`
	EvidenceRefs       []string                 `json:"evidence_refs"`
	Confidence         string                   `json:"confidence"`
}

type TraceFindingArtifact struct {
	ArtifactID   string `json:"artifact_id"`
	ContentHash  string `json:"content_hash"`
	DisplayLabel string `json:"display_label,omitempty"`
}

type TraceFindingScope struct {
	ProfileFamily string `json:"profile_family"`
	TargetRole    string `json:"target_role"`
	Phase         string `json:"phase"`
}

type TraceFindingRevision struct {
	CodraxCommit string `json:"codrax_commit,omitempty"`
	ContractHash string `json:"contract_hash"`
}

type TraceSymptomSummary struct {
	Kind  string  `json:"kind"`
	Value float64 `json:"value,omitempty"`
	Unit  string  `json:"unit,omitempty"`
}

type TraceUnresolvedDecision struct {
	Reason   string `json:"reason"`
	RawLabel string `json:"raw_label,omitempty"`
}

type TraceFindingCoverage struct {
	Complete bool     `json:"complete"`
	Caveats  []string `json:"caveats,omitempty"`
}

// TraceFindingV1 is the structured, reusable conclusion for one physical trace.
type TraceFindingV1 struct {
	SchemaVersion       int                      `json:"schema_version"`
	FindingID           string                   `json:"finding_id"`
	AnalysisKey         string                   `json:"analysis_key"`
	Artifact            TraceFindingArtifact     `json:"artifact"`
	Scope               TraceFindingScope        `json:"scope"`
	Revision            TraceFindingRevision     `json:"revision"`
	Symptom             TraceSymptomSummary      `json:"symptom"`
	PrimaryCause        *TraceCauseDecision      `json:"primary_cause,omitempty"`
	Contributors        []TraceCauseDecision     `json:"contributors,omitempty"`
	Unresolved          *TraceUnresolvedDecision `json:"unresolved,omitempty"`
	EvidenceRefs        []string                 `json:"evidence_refs"`
	CounterEvidenceRefs []string                 `json:"counter_evidence_refs,omitempty"`
	Coverage            TraceFindingCoverage     `json:"coverage"`
}
