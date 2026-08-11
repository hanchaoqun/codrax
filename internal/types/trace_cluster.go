package types

const TraceRootCauseClusterSchemaVersion = 1

type TraceCauseFingerprintV1 struct {
	Version            string `json:"version"`
	Token              string `json:"token"`
	Lane               string `json:"lane"`
	SubjectRole        string `json:"subject_role"`
	UpstreamRole       string `json:"upstream_role,omitempty"`
	CausalShape        string `json:"causal_shape"`
	Phase              string `json:"phase"`
	NormalizedEventKey string `json:"normalized_event_key,omitempty"`
	NormalizedStackKey string `json:"normalized_stack_key,omitempty"`
	RegistryHash       string `json:"registry_hash"`
}

type TraceFindingRef struct {
	FindingID   string `json:"finding_id"`
	AnalysisKey string `json:"analysis_key"`
}

type TraceClusterMember struct {
	UnitID      string `json:"unit_id"`
	FindingID   string `json:"finding_id"`
	AnalysisKey string `json:"analysis_key"`
}

type TraceClusterMetricBucket struct {
	Key        string    `json:"key"`
	Unit       string    `json:"unit"`
	Additivity string    `json:"additivity"`
	Caliber    string    `json:"caliber"`
	Values     []float64 `json:"values"`
}

type TraceRootCauseCluster struct {
	ClusterID            string                     `json:"cluster_id"`
	Level                string                     `json:"level"`
	ParentClusterID      string                     `json:"parent_cluster_id,omitempty"`
	Fingerprint          TraceCauseFingerprintV1    `json:"fingerprint"`
	CanonicalLabel       string                     `json:"canonical_label"`
	PrimaryMembers       []TraceClusterMember       `json:"primary_members"`
	ContributorMembers   []TraceClusterMember       `json:"contributor_members,omitempty"`
	PrimaryCount         int                        `json:"primary_count"`
	ShareOfAllSuccessful float64                    `json:"share_of_all_successful"`
	ShareOfResolved      float64                    `json:"share_of_resolved"`
	MetricBuckets        []TraceClusterMetricBucket `json:"metric_buckets,omitempty"`
	Representatives      []TraceFindingRef          `json:"representatives"`
	MergeBasis           []string                   `json:"merge_basis"`
	AmbiguityStatus      string                     `json:"ambiguity_status"`
	Singleton            bool                       `json:"singleton"`
}

type TraceUnresolvedMember struct {
	UnitID    string `json:"unit_id"`
	FindingID string `json:"finding_id"`
	Reason    string `json:"reason"`
}

type TraceBatchFailure struct {
	UnitID string `json:"unit_id"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

type ClusterInvariantReport struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

type TraceRootCauseClusterSetV1 struct {
	SchemaVersion      int                     `json:"schema_version"`
	BatchID            string                  `json:"batch_id"`
	FingerprintVersion string                  `json:"fingerprint_version"`
	InputUnitCount     int                     `json:"input_unit_count"`
	SuccessfulCount    int                     `json:"successful_count"`
	ResolvedCount      int                     `json:"resolved_count"`
	UnresolvedCount    int                     `json:"unresolved_count"`
	FailedCount        int                     `json:"failed_count"`
	Clusters           []TraceRootCauseCluster `json:"clusters"`
	Unresolved         []TraceUnresolvedMember `json:"unresolved"`
	Failures           []TraceBatchFailure     `json:"failures"`
	Invariants         ClusterInvariantReport  `json:"invariants"`
}
