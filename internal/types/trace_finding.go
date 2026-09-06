package types

// TraceFindingSchemaVersion versions the frozen candidate contract.
const TraceFindingSchemaVersion = 1

// TraceFindingContract is the frozen, system-owned typed candidate roster
// for one trace analysis plus the user-facing root-cause report activation
// (RootCauseReportEnabled: the optional `trace_root_causes` selector is
// published only when the roster has selectable on-chain candidates).
//
// V1-5 (colleague_merge_audit §40.16): the legacy `Required` typed-finding
// lane (a model-authored TraceFindingV1 sidecar) was retired on all three
// faces — schema, teaching and decoder — because its only producer forced it
// off and its decoder face alone was hard-rejecting whole answers.
type TraceFindingContract struct {
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
	// ArtifactLabels (V1-4, §40.26) is the fold's partition roster: the
	// distinct trace-file labels of the compiled projections in first-
	// appearance order (the same labels the answer's per-trace sections
	// wear). Artifact above stays the legacy Required-lane single-artifact
	// envelope; this list is what the roster groups by and what the contract
	// identity hashes.
	ArtifactLabels []string `json:"artifact_labels,omitempty"`
}

// MultiArtifact reports whether the contract folds candidates from more than
// one trace file — the roster then groups by artifact and every candidate's
// ArtifactLabel is its partition key.
func (c *TraceFindingContract) MultiArtifact() bool {
	return c != nil && len(c.ArtifactLabels) > 1
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
	// Gated components retain the measured dependency's ready-to-run share
	// and discounted running share. Their presence is independent of the
	// ordinary running supply fold and D/I/O accounting below.
	GatedComponentsPresent     bool    `json:"gated_components_present,omitempty"`
	GatedRunnableMS            float64 `json:"gated_runnable_ms,omitempty"`
	GatedRunningDeficitMS      float64 `json:"gated_running_deficit_ms,omitempty"`
	GatedCapabilitySource      string  `json:"gated_capability_source,omitempty"`
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
	// CausalQualifier (SIDECAR-Q1, user ruling 2026-09-02, colleague_merge_audit
	// §40.28 ②) is the SEAT-LEVEL frame-causality qualifier of this candidate:
	// TraceCausalQualifierFrameUnproven when any of the candidate's own
	// evidence IDs came from a trace_query result whose typed causal rows
	// exist but whose frame evidence is absent/unavailable/unproven — the same
	// evidence-ID-keyed authority the Markdown crown face consults for its
	// 「（帧因果未证）」 qualifier (T3-1 ruling §7.3). Never the session-wide
	// ANY aggregate.
	CausalQualifier string `json:"causal_qualifier"`
	// MechanismQualifier does not inherit the frame-causality result. Empty
	// means no qualifier was supplied, never that the mechanism was proven.
	MechanismQualifier string `json:"mechanism_qualifier,omitempty"`
	// ArtifactLabel (V1-4, §40.26 ①) is the SYSTEM-OWNED partition key of the
	// trace this seat was compiled from — the projection partitioner's
	// ArtifactLabel, the same label the answer's per-trace sections and the
	// public sidecar item wear. Empty only for an identity-less single-trace
	// ledger (never fabricated). Two same-named threads from two trace files
	// are two candidates with two labels.
	ArtifactLabel string `json:"artifact_label,omitempty"`
	// EvidenceFacts (SIDECAR-EVID-1, customer report 2026-09-02 → §40.32) is
	// the SYSTEM-OWNED typed fact bundle behind the public sidecar's
	// `evidence` sentences: window, seat interval, attachment line range,
	// chain relation (depth / branch / credential edge), registry lane and fix
	// direction, state kind. The public evidence is rendered from these
	// facts in customer-readable words — never from internal artifact paths
	// or trace_query result ids, which the customer cannot open. Frozen with
	// the contract; the model never authors it.
	EvidenceFacts *TraceCauseEvidenceFacts `json:"evidence_facts,omitempty"`
}

// TraceCauseEvidenceFacts — see TraceCauseDecision.EvidenceFacts.
type TraceCauseEvidenceFacts struct {
	ArtifactLabel          string   `json:"artifact_label,omitempty"`
	WindowStartTs          float64  `json:"window_start_ts,omitempty"`
	WindowEndTs            float64  `json:"window_end_ts,omitempty"`
	SeatStartTs            float64  `json:"seat_start_ts,omitempty"`
	SeatEndTs              float64  `json:"seat_end_ts,omitempty"`
	LineStart              int      `json:"line_start,omitempty"`
	LineEnd                int      `json:"line_end,omitempty"`
	TargetSubject          string   `json:"target_subject,omitempty"`
	ChainRelevance         string   `json:"chain_relevance,omitempty"`
	Causality              string   `json:"causality,omitempty"`
	OnChainBasis           string   `json:"on_chain_basis,omitempty"`
	ChainDepth             int      `json:"chain_depth,omitempty"`
	ChainBranch            int      `json:"chain_branch,omitempty"`
	HostWakeupEdgeAnchorTs float64  `json:"host_wakeup_edge_anchor_ts,omitempty"`
	HostWakeupEdgeVia      string   `json:"host_wakeup_edge_via,omitempty"`
	WakeupPath             []string `json:"wakeup_path,omitempty"`
	StateKind              string   `json:"state_kind,omitempty"`
	BlockedReasonCaller    string   `json:"blocked_reason_caller,omitempty"`
	Lane                   string   `json:"lane,omitempty"`
	FixDirection           string   `json:"fix_direction,omitempty"`
	SemanticClass          string   `json:"semantic_class,omitempty"`
	SpanName               string   `json:"span_name,omitempty"`
}

// TraceCausalQualifier values — closed set, always explicit on every public
// surface (a consumer never infers the qualifier from field absence).
//   - proven: the candidate's own trace evidence was checked for frame
//     evidence and none withholds it;
//   - frame_unproven: checked and the frame evidence is absent/unavailable/
//     unproven — the seat-level qualifier the headline wears (T3-1 §7.3);
//   - not_applicable (QUALGATE-1, §40.30 V-QUAL-1 plan A): the request is not
//     a frame/jank question per the analyzer's typed decision, so frame
//     causality is not a claim the report makes — neither proven nor
//     unproven; no headline qualifier, no summary suffix, and it never caps a
//     candidate's status.
const (
	TraceCausalQualifierProven        = "proven"
	TraceCausalQualifierFrameUnproven = "frame_unproven"
	TraceCausalQualifierNotApplicable = "not_applicable"
	// TraceCausalQualifierFrameUnprovenSuffixZH is the ONE user-facing spelling
	// of the frame_unproven qualifier: the Markdown crown headline wears it and
	// the sidecar summary appends it (§40.28 ② 「summary 限定注与头行同词」) —
	// both read this constant, so the two faces cannot drift apart.
	TraceCausalQualifierFrameUnprovenSuffixZH = "（帧因果未证）"
)

// ValidTraceCausalQualifier reports closed-set membership.
func ValidTraceCausalQualifier(v string) bool {
	switch v {
	case TraceCausalQualifierProven, TraceCausalQualifierFrameUnproven, TraceCausalQualifierNotApplicable:
		return true
	}
	return false
}

// AllTraceCausalQualifiers lists the closed set in its documented order (the
// guide table and teaching surfaces render from this one list).
func AllTraceCausalQualifiers() []string {
	return []string{TraceCausalQualifierProven, TraceCausalQualifierFrameUnproven, TraceCausalQualifierNotApplicable}
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

type TraceSymptomSummary struct {
	Kind  string  `json:"kind"`
	Value float64 `json:"value,omitempty"`
	Unit  string  `json:"unit,omitempty"`
}

// cloneTraceMagnitude deep-copies a frozen magnitude (Components included) so
// a contract read never aliases the stored facts.
func cloneTraceMagnitude(in *TypedMagnitude) *TypedMagnitude {
	if in == nil {
		return nil
	}
	out := *in
	if in.Components != nil {
		components := *in.Components
		out.Components = &components
	}
	return &out
}
