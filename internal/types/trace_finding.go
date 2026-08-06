package types

// TraceFindingSchemaVersion is the schema version for TraceFindingV1.
const TraceFindingSchemaVersion = 1

// TraceCausalStatus is the closed status set for a cause decision.
type TraceCausalStatus string

const (
	TraceCausalProven             TraceCausalStatus = "proven"
	TraceCausalSupportedCandidate TraceCausalStatus = "supported_candidate"
	TraceCausalUnresolved         TraceCausalStatus = "unresolved"
)

// TraceConfidence is a coarse confidence label for finding decisions.
type TraceConfidence string

const (
	TraceConfidenceHigh   TraceConfidence = "high"
	TraceConfidenceMedium TraceConfidence = "medium"
	TraceConfidenceLow    TraceConfidence = "low"
)

// TraceCausalTokenSnapshot stores an immutable registry snapshot so types
// never import tracequery (avoids the types↔tracequery cycle). Validation
// against the live registry lives in internal/analysis/tracefinding.
type TraceCausalTokenSnapshot struct {
	Token        string `json:"token"`
	Lane         string `json:"lane"`
	Additivity   string `json:"additivity"`
	SubjectKind  string `json:"subject_kind"`
	FixDirection string `json:"fix_direction,omitempty"`
	RegistryHash string `json:"registry_hash"`
}

// TypedMagnitude carries a numeric impact with full caliber metadata.
type TypedMagnitude struct {
	Value          float64 `json:"value"`
	Unit           string  `json:"unit"`
	Additivity     string  `json:"additivity"`
	Caliber        string  `json:"caliber"`
	WindowDuration float64 `json:"window_duration_ms,omitempty"`
}

// TraceFindingArtifact identifies the physical Trace artifact for one finding.
type TraceFindingArtifact struct {
	Label       string `json:"label,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Path        string `json:"path,omitempty"`
}

// TraceFindingScope binds target/window selectors for one analysis unit.
type TraceFindingScope struct {
	Process     string  `json:"process,omitempty"`
	ThreadRole  string  `json:"thread_role,omitempty"`
	WindowStart float64 `json:"window_start_ts,omitempty"`
	WindowEnd   float64 `json:"window_end_ts,omitempty"`
}

// TraceFindingRevision records audit metadata (not a cache invalidation key).
type TraceFindingRevision struct {
	CodraxCommit string `json:"codrax_commit,omitempty"`
	ContractHash string `json:"contract_hash,omitempty"`
}

// TraceSymptomSummary is a short typed symptom description for one unit.
type TraceSymptomSummary struct {
	Label string `json:"label,omitempty"`
	Text  string `json:"text,omitempty"`
}

// TraceCauseDecision is one primary/contributor cause selection.
type TraceCauseDecision struct {
	CandidateID      string                   `json:"candidate_id"`
	Status           TraceCausalStatus        `json:"status"`
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
	Confidence       TraceConfidence          `json:"confidence,omitempty"`
}

// TraceUnresolvedDecision records that no primary cause could be selected.
type TraceUnresolvedDecision struct {
	ReasonCode string `json:"reason_code,omitempty"`
	RawLabel   string `json:"raw_label,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// TraceFindingCoverage records how complete the finding's evidence surface is.
type TraceFindingCoverage struct {
	RosterComplete bool     `json:"roster_complete,omitempty"`
	Caveats        []string `json:"caveats,omitempty"`
}

// TraceFindingV1 is the typed single-unit causal conclusion sidecar.
// It must not carry BatchID — batches reference findings by AnalysisKey.
type TraceFindingV1 struct {
	SchemaVersion       int                      `json:"schema_version"`
	FindingID           string                   `json:"finding_id"`
	AnalysisKey         string                   `json:"analysis_key,omitempty"`
	Artifact            TraceFindingArtifact     `json:"artifact"`
	Scope               TraceFindingScope        `json:"scope"`
	Revision            TraceFindingRevision     `json:"revision,omitempty"`
	Symptom             TraceSymptomSummary      `json:"symptom,omitempty"`
	PrimaryCause        *TraceCauseDecision      `json:"primary_cause,omitempty"`
	Contributors        []TraceCauseDecision     `json:"contributors,omitempty"`
	Unresolved          *TraceUnresolvedDecision `json:"unresolved,omitempty"`
	EvidenceRefs        []string                 `json:"evidence_refs,omitempty"`
	CounterEvidenceRefs []string                 `json:"counter_evidence_refs,omitempty"`
	Coverage            TraceFindingCoverage     `json:"coverage,omitempty"`
}

// TraceFindingContract is a system-injected run contract controlling whether
// Finalizer schema/validation require or optionally accept TraceFindingV1.
type TraceFindingContract struct {
	// Required forces emit_answer_document to include a validated finding.
	Required bool `json:"required,omitempty"`
	// ShadowOptional projects an optional trace_finding field without failing
	// when absent (P0 shadow mode).
	ShadowOptional bool `json:"shadow_optional,omitempty"`
	// CandidateSetID ties the emit to a compiled candidate set identity.
	CandidateSetID string `json:"candidate_set_id,omitempty"`
	// FindingSchemaVersion pins the accepted finding schema.
	FindingSchemaVersion int `json:"finding_schema_version,omitempty"`
}

// Active reports whether the contract should affect schema projection.
func (c *TraceFindingContract) Active() bool {
	return c != nil && (c.Required || c.ShadowOptional)
}
