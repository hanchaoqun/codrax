package types

// ObservationSourceKind names the physical namespace of an observation's source
// reference. It is intentionally finer than AnswerEvidenceOrigin: an MCP
// resource and a connector response may both be external-resource evidence, but
// their address shapes and safety rules differ.
type ObservationSourceKind string

const (
	ObservationSourceUnknown          ObservationSourceKind = ""
	ObservationSourceCurrentSource    ObservationSourceKind = "current_source"
	ObservationSourceVCSMetadata      ObservationSourceKind = "vcs_metadata"
	ObservationSourceVCSDiff          ObservationSourceKind = "vcs_diff"
	ObservationSourceRuntimeArtifact  ObservationSourceKind = "runtime_artifact"
	ObservationSourceCommand          ObservationSourceKind = "command"
	ObservationSourceCrossRepoIndex   ObservationSourceKind = "cross_repo_index"
	ObservationSourceExternalDocument ObservationSourceKind = "external_document"
	ObservationSourceWebPage          ObservationSourceKind = "web_page"
	ObservationSourceMCPResource      ObservationSourceKind = "mcp_resource"
	ObservationSourceConnector        ObservationSourceKind = "connector_resource"
)

// ObservationSourceRef is the origin-specific address of the thing that was
// observed. Only the fields that apply to SourceKind should be populated.
type ObservationSourceRef struct {
	Kind         ObservationSourceKind `json:"kind,omitempty"`
	Repo         string                `json:"repo,omitempty"`
	Path         string                `json:"path,omitempty"`
	Commit       string                `json:"commit,omitempty"`
	Range        string                `json:"range,omitempty"`
	Pathspec     string                `json:"pathspec,omitempty"`
	Command      string                `json:"command,omitempty"`
	ToolCallID   string                `json:"tool_call_id,omitempty"`
	RawRef       string                `json:"raw_ref,omitempty"`
	ArtifactID   string                `json:"artifact_id,omitempty"`
	ArtifactKind string                `json:"artifact_kind,omitempty"`
	URL          string                `json:"url,omitempty"`
	FetchedAt    string                `json:"fetched_at,omitempty"`
	Server       string                `json:"server,omitempty"`
	ResourceURI  string                `json:"resource_uri,omitempty"`
	MIMEType     string                `json:"mime_type,omitempty"`
	Connector    string                `json:"connector,omitempty"`
}

// ObservationSpan locates the observation inside SourceRef when that source has
// an addressable interior. It supports current source lines, artifact-local
// lines, VCS hunk coordinates, web/resource selectors, JSON pointers, table
// rows, and trace time ranges without overloading repo citations.
type ObservationSpan struct {
	LineStart   int     `json:"line_start,omitempty"`
	LineEnd     int     `json:"line_end,omitempty"`
	OldLine     int     `json:"old_line,omitempty"`
	NewLine     int     `json:"new_line,omitempty"`
	HunkHeader  string  `json:"hunk_header,omitempty"`
	Paragraph   int     `json:"paragraph,omitempty"`
	Selector    string  `json:"selector,omitempty"`
	JSONPointer string  `json:"json_pointer,omitempty"`
	Row         int     `json:"row,omitempty"`
	TextStart   int     `json:"text_start,omitempty"`
	TextEnd     int     `json:"text_end,omitempty"`
	StartTsMs   float64 `json:"start_ts_ms,omitempty"`
	EndTsMs     float64 `json:"end_ts_ms,omitempty"`
}

// ObservationRecord is the normalized, read-only fact surface compiled from
// accepted producer outputs. It is not a replacement for EvidenceItem; source
// evidence remains the only lane that may become repo citations. The ledger
// gives non-code facts an equally typed route to finalizer/reviewer consumers.
type ObservationRecord struct {
	ID              string               `json:"id"`
	Origin          AnswerEvidenceOrigin `json:"origin"`
	Producer        string               `json:"producer,omitempty"`
	Role            AnswerAggregateRole  `json:"role,omitempty"`
	GroundingPolicy ClaimGroundingPolicy `json:"grounding_policy,omitempty"`
	SourceRef       ObservationSourceRef `json:"source_ref,omitempty"`
	Span            ObservationSpan      `json:"span,omitempty"`
	ClaimKey        string               `json:"claim_key,omitempty"`
	Subject         string               `json:"subject,omitempty"`
	Predicate       string               `json:"predicate,omitempty"`
	Object          string               `json:"object,omitempty"`
	Value           string               `json:"value,omitempty"`
	Unit            string               `json:"unit,omitempty"`
	Negative        bool                 `json:"negative,omitempty"`
	ResultCount     *int                 `json:"result_count,omitempty"`
	Summary         string               `json:"summary,omitempty"`
	RawExcerpt      string               `json:"raw_excerpt,omitempty"`
	RichNotes       []string             `json:"rich_notes,omitempty"`
	SupportRefs     []string             `json:"support_refs,omitempty"`
	ObservedAt      string               `json:"observed_at,omitempty"`
	Scope           string               `json:"scope,omitempty"`
	Confidence      float64              `json:"confidence,omitempty"`
}

// ObservationLedger is the deterministic, compiled fact ledger for a run. Later
// batches will populate it from the existing carriers and make finalizer/reviewer
// consume it first. Keeping the type in internal/types lets all producers depend
// on the same contract without importing tool or agent packages.
type ObservationLedger struct {
	Records []ObservationRecord `json:"records,omitempty"`
}

func (l ObservationLedger) Empty() bool {
	return len(l.Records) == 0
}

// ObservationLedgerInput carries existing accepted producer outputs into the
// ledger compiler. The compiler is intentionally side-effect free and must not
// inspect raw user prose or model free text to classify facts.
type ObservationLedgerInput struct {
	EvidenceItems  []EvidenceItem
	AggregateFacts []AnswerAggregateFact
	ToolResults    []ToolResult
	LogBundle      *LogBundle
	PerfBundle     *PerfBundle
	MCPResponses   []MCPResponse
	RequestModel   *RequestModel
	AnswerContract *AnswerContract
}

// CompileObservationLedger is the Batch-1 no-op skeleton. Batch 2 will add
// producer adapters for the existing carriers. Providing the API now lets tests
// and downstream packages depend on the stable contract without changing prompt
// behavior before the adapters are ready.
func CompileObservationLedger(input ObservationLedgerInput) ObservationLedger {
	_ = input
	return ObservationLedger{}
}

func ObservationSourceKindForOrigin(origin AnswerEvidenceOrigin) ObservationSourceKind {
	switch origin {
	case AnswerEvidenceOriginCurrentSource:
		return ObservationSourceCurrentSource
	case AnswerEvidenceOriginVCSMetadata:
		return ObservationSourceVCSMetadata
	case AnswerEvidenceOriginVCSDiff:
		return ObservationSourceVCSDiff
	case AnswerEvidenceOriginRuntimeArtifact:
		return ObservationSourceRuntimeArtifact
	case AnswerEvidenceOriginCommandMeasurement:
		return ObservationSourceCommand
	case AnswerEvidenceOriginRepoNegativeSearch:
		return ObservationSourceCurrentSource
	case AnswerEvidenceOriginCrossRepoIndex:
		return ObservationSourceCrossRepoIndex
	case AnswerEvidenceOriginExternalDocument:
		return ObservationSourceExternalDocument
	case AnswerEvidenceOriginWebPage:
		return ObservationSourceWebPage
	case AnswerEvidenceOriginMCPResource:
		return ObservationSourceMCPResource
	case AnswerEvidenceOriginConnectorResource:
		return ObservationSourceConnector
	default:
		return ObservationSourceUnknown
	}
}
