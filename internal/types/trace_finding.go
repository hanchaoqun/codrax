package types

// TraceFindingSchemaVersion is the persisted single-trace finding schema.
const TraceFindingSchemaVersion = 1

// TraceFindingContract is injected by a trace-batch child run. Ordinary read
// requests leave it disabled and therefore keep the historical tool schema.
type TraceFindingContract struct {
	Required                bool     `json:"required"`
	CandidateSetID          string   `json:"candidate_set_id"`
	FindingSchemaVersion    int      `json:"finding_schema_version"`
	PrimaryCandidateIDs     []string `json:"primary_candidate_ids"`
	ContributorCandidateIDs []string `json:"contributor_candidate_ids"`
	AcceptedEvidenceIDs     []string `json:"accepted_evidence_ids"`
	RegistryHash            string   `json:"registry_hash"`
	CausalCeiling           string   `json:"causal_ceiling"`
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
}

type TraceCauseDecision struct {
	CandidateID        string                   `json:"candidate_id"`
	Status             TraceCausalStatus        `json:"status"`
	Token              TraceCausalTokenSnapshot `json:"token"`
	SubjectRole        string                   `json:"subject_role"`
	UpstreamRole       string                   `json:"upstream_role,omitempty"`
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
