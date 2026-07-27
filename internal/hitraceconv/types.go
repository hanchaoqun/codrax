package hitraceconv

import "fmt"

const (
	defaultOutputSuffix = ".systrace"
	converterVersion    = "hitraceconv-v1"
)

const (
	ArtifactSystrace    = "systrace"
	ArtifactPerfData    = "perf_data"
	ArtifactPerfTrace   = "perftrace"
	ArtifactTraceDB     = "trace_db"
	ArtifactTraceBundle = "tracebundle"
)

// Options controls one explicit binary HiTrace conversion.
type Options struct {
	InputPath              string
	OutputPath             string
	ArchiveMember          string
	Flavor                 string
	HiperfPath             string
	HiperfSymbolDirs       []string
	SimpleperfReportPath   string
	SimpleperfPythonPath   string
	SimpleperfSymfsDir     string
	SimpleperfKallsymsPath string
	PerfParser             string
	DisablePerfAdapter     bool
	TraceEngine            string
	TraceStreamerPath      string
	TraceDBOutputPath      string
	KeepTraceDB            bool
	TraceStreamerSoDirs    []string
	// RuntimeAnchor owns conversion-private staging. Product entrypoints set
	// this to <CWD>/.codrax; direct library callers default to a .codrax
	// directory beside the selected output.
	RuntimeAnchor string
	Progress      ProgressFunc
}

type Artifact struct {
	Type          string                      `json:"type"`
	Path          string                      `json:"path"`
	Bytes         int64                       `json:"bytes"`
	SHA256        string                      `json:"sha256,omitempty"`
	DataType      uint32                      `json:"data_type,omitempty"`
	PluginName    string                      `json:"plugin_name,omitempty"`
	PluginVersion string                      `json:"plugin_version,omitempty"`
	SourceOffset  int64                       `json:"source_offset,omitempty"`
	SourceBytes   int64                       `json:"source_bytes,omitempty"`
	Converter     string                      `json:"converter,omitempty"`
	Trace         *TraceArtifactCapability    `json:"trace_capability,omitempty"`
	Perf          *PerfArtifactCapability     `json:"perf_capability,omitempty"`
	Standalone    *StandaloneSourceProvenance `json:"standalone_provenance,omitempty"`
	PerfTransform *PerfInputTransform         `json:"perf_input_transform,omitempty"`
	Caveats       []string                    `json:"caveats,omitempty"`
	// These paths are factory-only, in-memory receipt bindings. The first is
	// the frozen absolute ledger identity; the second pins the user-facing
	// spelling so later validation cannot relabel a valid generation. Neither
	// is serialized into a bundle.
	traceReceiptBindingPath  string             `json:"-"`
	traceReceiptArtifactPath string             `json:"-"`
	standaloneReceipt        *standaloneSegment `json:"-"`
}

// PerfInputTransform binds a derived perftrace to the exact compressed source
// artifact and exact privately decoded generation consumed by its provider.
// It is provenance metadata, not an additional causal tracebundle child.
type PerfInputTransform struct {
	Profile            string `json:"profile"`
	SourceArtifactPath string `json:"source_artifact_path"`
	SourceFormat       string `json:"source_format"`
	SourceBytes        int64  `json:"source_bytes"`
	SourceSHA256       string `json:"source_sha256"`
	DecodedFormat      string `json:"decoded_format"`
	DecodedBytes       int64  `json:"decoded_bytes"`
	DecodedSHA256      string `json:"decoded_sha256"`
}

// TraceArchiveProvenance binds one selected archive member to the exact outer
// capture generation which supplied it. The archive is an input origin, not a
// causal tracebundle child, so it deliberately does not participate in the
// schema-v2 capture_id child set.
type TraceArchiveProvenance struct {
	Format        string `json:"format"`
	ArchiveBytes  int64  `json:"archive_bytes"`
	ArchiveSHA256 string `json:"archive_sha256"`
	Member        string `json:"member"`
	MemberBytes   int64  `json:"member_bytes"`
	MemberSHA256  string `json:"member_sha256"`
	Selection     string `json:"selection"`
}

// StandaloneSourceProvenance discloses the authenticated physical source of a
// raw perf.data sidecar. It never attaches to a derived perftrace: that file's
// own SHA and receipt describe different bytes.
type StandaloneSourceProvenance struct {
	Profile         string `json:"profile"`
	LayoutAuthority string `json:"layout_authority"`
	WriterProfile   string `json:"writer_profile"`
}

// TraceArtifactCapability is the receipt-derived analysis contract for a
// converter-owned systrace. Inventory existence and trace_query readiness are
// deliberately separate: a zero-known builtin/Profiler output can remain a
// useful compatibility artifact without claiming causal-analysis capability.
type TraceArtifactCapability struct {
	ProviderKind          string `json:"provider_kind"`
	ProviderName          string `json:"provider_name"`
	OutputFormat          string `json:"output_format"`
	ValidationProfile     string `json:"validation_profile"`
	Rows                  int    `json:"rows"`
	Known                 int    `json:"known"`
	AuthoritativeKnown    int    `json:"authoritative_known"`
	AdvisoryRows          int    `json:"advisory_rows"`
	IntentionalUnknown    int    `json:"intentional_unknown"`
	IntentionalHeaderOnly int    `json:"intentional_header_only"`
	TraceQueryReady       bool   `json:"trace_query_ready"`
}

type PerfArtifactCapability struct {
	ProviderKind           string                      `json:"provider_kind,omitempty"`
	ProviderName           string                      `json:"provider_name,omitempty"`
	InputFormat            string                      `json:"input_format,omitempty"`
	OutputFormat           string                      `json:"output_format,omitempty"`
	TimeDomain             string                      `json:"time_domain,omitempty"`
	TimeAlignment          string                      `json:"time_alignment,omitempty"`
	ThreadIdentity         string                      `json:"thread_identity,omitempty"`
	CPUIdentity            string                      `json:"cpu_identity,omitempty"`
	EventWeight            string                      `json:"event_weight,omitempty"`
	Symbolization          string                      `json:"symbolization,omitempty"`
	Callchain              string                      `json:"callchain,omitempty"`
	DSOLabel               string                      `json:"dso_label,omitempty"`
	BuildID                string                      `json:"build_id,omitempty"`
	OffCPU                 string                      `json:"off_cpu,omitempty"`
	Confidence             string                      `json:"confidence,omitempty"`
	TraceQueryReady        bool                        `json:"trace_query_ready"`
	Degraded               bool                        `json:"degraded,omitempty"`
	RawCaptureCompleteness *RawPerfCaptureCompleteness `json:"raw_perf_capture_completeness,omitempty"`
	RawCaptureResidual     *RawPerfCaptureResidual     `json:"raw_perf_capture_residual,omitempty"`
	RawSampleAdmission     *RawPerfSampleAdmission     `json:"raw_perf_sample_admission,omitempty"`
	Caveats                []string                    `json:"caveats,omitempty"`
}

// RawPerfCaptureCompleteness is the parser-owned record census intended for
// later receipt binding. It describes only records physically present in the
// owned perf record stream; it never claims that the producer or transport
// captured every event that occurred on the device.
type RawPerfCaptureCompleteness struct {
	Profile           string                `json:"profile"`
	Source            string                `json:"source"`
	SampleRecords     RawPerfRecordCensus   `json:"sample_records"`
	LostRecords       RawPerfRecordCensus   `json:"lost_records"`
	LostSampleRecords RawPerfRecordCensus   `json:"lost_sample_records"`
	AuxRecords        RawPerfRecordCensus   `json:"aux_records"`
	LostEvents        RawPerfAggregateTotal `json:"lost_events"`
	LostSamples       RawPerfAggregateTotal `json:"lost_samples"`
	AuxBytes          RawPerfAggregateTotal `json:"aux_bytes"`
}

// RawPerfCaptureResidual is a receipt-bound side profile for record families
// intentionally absent from RawPerfCaptureCompleteness v1. It counts observed
// record headers only and makes no claim that THROTTLE/UNTHROTTLE payloads were
// semantically validated.
type RawPerfCaptureResidual struct {
	Profile           string `json:"profile"`
	Source            string `json:"source"`
	ThrottleRecords   uint64 `json:"throttle_records"`
	UnthrottleRecords uint64 `json:"unthrottle_records"`
}

// RawPerfSampleAdmission binds structurally parsed PERF_RECORD_SAMPLE
// candidates to the subset that is safe to publish as queryable perf rows.
// Reason counters are mutually exclusive primary verdicts and close exactly
// over InventoryOnly; they never upgrade an unverified coordinate.
type RawPerfSampleAdmission struct {
	Profile         string `json:"profile"`
	Source          string `json:"source"`
	Candidates      uint64 `json:"candidates"`
	QueryRows       uint64 `json:"query_rows"`
	InventoryOnly   uint64 `json:"inventory_only"`
	MissingTID      uint64 `json:"missing_tid"`
	InvalidIdentity uint64 `json:"invalid_identity"`
	MissingTime     uint64 `json:"missing_time"`
	MissingPeriod   uint64 `json:"missing_period"`
	InvalidPeriod   uint64 `json:"invalid_period"`
	InvalidCPU      uint64 `json:"invalid_cpu"`
}

// RawPerfRecordCensus closes one physical record family. Accepted and Rejected
// are disjoint and their sum must equal Physical.
type RawPerfRecordCensus struct {
	Physical uint64 `json:"physical"`
	Accepted uint64 `json:"accepted"`
	Rejected uint64 `json:"rejected"`
}

// RawPerfAggregateTotal distinguishes a genuine exact zero from a dimension
// that was never reported, and withdraws numeric authority after overflow or a
// malformed aggregate record.
type RawPerfAggregateTotal struct {
	State  string `json:"state"`
	Value  uint64 `json:"value"`
	Reason string `json:"reason,omitempty"`
}

type PerfProviderDecision struct {
	Stage           string `json:"stage,omitempty"`
	ProviderKind    string `json:"provider_kind,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	InputPath       string `json:"input_path,omitempty"`
	InputFormat     string `json:"input_format,omitempty"`
	OutputPath      string `json:"output_path,omitempty"`
	ParserMode      string `json:"parser_mode,omitempty"`
	Selected        bool   `json:"selected"`
	Attempted       bool   `json:"attempted"`
	Succeeded       bool   `json:"succeeded"`
	Fallback        bool   `json:"fallback"`
	TraceQueryReady bool   `json:"trace_query_ready"`
	ArtifactPath    string `json:"artifact_path,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Caveat          string `json:"caveat,omitempty"`
}

type TraceProviderDecision struct {
	Stage           string `json:"stage,omitempty"`
	ProviderKind    string `json:"provider_kind,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	InputPath       string `json:"input_path,omitempty"`
	OutputPath      string `json:"output_path,omitempty"`
	DBPath          string `json:"db_path,omitempty"`
	EngineMode      string `json:"engine_mode,omitempty"`
	Selected        bool   `json:"selected"`
	Attempted       bool   `json:"attempted"`
	Succeeded       bool   `json:"succeeded"`
	Fallback        bool   `json:"fallback"`
	TraceQueryReady bool   `json:"trace_query_ready"`
	ArtifactPath    string `json:"artifact_path,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Caveat          string `json:"caveat,omitempty"`
}

type TraceDBCoverage struct {
	Family                 string                      `json:"family,omitempty"`
	ArtifactPath           string                      `json:"artifact_path,omitempty"`
	Table                  string                      `json:"table"`
	Role                   string                      `json:"role,omitempty"`
	Found                  bool                        `json:"found"`
	FieldSources           map[string]string           `json:"field_sources,omitempty"`
	Metrics                map[string]int64            `json:"metrics,omitempty"`
	ColumnsPresent         []string                    `json:"columns_present,omitempty"`
	ColumnsMissing         []string                    `json:"columns_missing,omitempty"`
	RowsRead               int                         `json:"rows_read,omitempty"`
	RowsEmitted            int                         `json:"rows_emitted,omitempty"`
	PeakBuffered           int                         `json:"peak_buffered_rows,omitempty"`
	PeakBufferedBytes      uint64                      `json:"peak_buffered_bytes,omitempty"`
	SpillChunks            int                         `json:"spill_chunks,omitempty"`
	TempBytes              int64                       `json:"temp_bytes,omitempty"`
	CurrentLiveTempBytes   uint64                      `json:"current_live_temp_bytes,omitempty"`
	PeakLiveTempBytes      uint64                      `json:"peak_live_temp_bytes,omitempty"`
	PeakOpenRunFDs         int                         `json:"peak_open_run_fds,omitempty"`
	MergePasses            int                         `json:"merge_passes,omitempty"`
	ElapsedUS              int64                       `json:"elapsed_us,omitempty"`
	Skipped                string                      `json:"skipped,omitempty"`
	Error                  string                      `json:"error,omitempty"`
	CaptureCompleteness    *TraceCaptureCompleteness   `json:"capture_completeness,omitempty"`
	RawCaptureCompleteness *RawPerfCaptureCompleteness `json:"raw_perf_capture_completeness,omitempty"`
	RawCaptureResidual     *RawPerfCaptureResidual     `json:"raw_perf_capture_residual,omitempty"`
	RawSampleAdmission     *RawPerfSampleAdmission     `json:"raw_perf_sample_admission,omitempty"`
}

// TraceCaptureCompleteness is the bounded, typed interpretation of the
// trace_streamer stat table. It qualifies absence-based conclusions; it never
// proves source/transport completeness and never overrides a positively
// observed trace event.
type TraceCaptureCompleteness struct {
	State            string                          `json:"state"`
	RowsAccepted     int                             `json:"rows_accepted,omitempty"`
	Received         uint64                          `json:"received,omitempty"`
	DataLost         uint64                          `json:"data_lost,omitempty"`
	NotMatch         uint64                          `json:"not_match,omitempty"`
	NotSupported     uint64                          `json:"not_supported,omitempty"`
	InvalidData      uint64                          `json:"invalid_data,omitempty"`
	InfoIssues       uint64                          `json:"info_issues,omitempty"`
	WarnIssues       uint64                          `json:"warn_issues,omitempty"`
	ErrorIssues      uint64                          `json:"error_issues,omitempty"`
	FatalIssues      uint64                          `json:"fatal_issues,omitempty"`
	NonzeroIssueRows int                             `json:"nonzero_issue_rows,omitempty"`
	Issues           []TraceCaptureCompletenessIssue `json:"issues,omitempty"`
	IssuesCompacted  int                             `json:"issues_compacted,omitempty"`
	IntegrityIssues  []string                        `json:"integrity_issues,omitempty"`
}

type TraceCaptureCompletenessIssue struct {
	EventName string `json:"event_name"`
	StatType  string `json:"stat_type"`
	Count     uint64 `json:"count"`
	Source    string `json:"source"`
	Severity  string `json:"severity"`
}

type PerfClockAlignment struct {
	ArtifactPath    string   `json:"artifact_path,omitempty"`
	PerfTimeDomain  string   `json:"perf_time_domain,omitempty"`
	TraceTimeDomain string   `json:"trace_time_domain,omitempty"`
	OffsetSec       *float64 `json:"offset_sec,omitempty"`
	Slope           *float64 `json:"slope,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	Calibrated      bool     `json:"calibrated"`
	Source          string   `json:"source,omitempty"`
	Caveats         []string `json:"caveats,omitempty"`
}

// Result summarizes a completed conversion.
type Result struct {
	InputPath          string
	ArchiveProvenance  *TraceArchiveProvenance
	OutputPath         string
	BundlePath         string
	Artifacts          []Artifact
	ProviderDecisions  []PerfProviderDecision
	TraceDecisions     []TraceProviderDecision
	TraceDBCoverage    []TraceDBCoverage
	TraceCoverage      []TraceDBCoverage
	InputBytes         int64
	OutputBytes        int64
	EventsWritten      int
	MissingFormatCount int
	UnknownEventCount  int
	FirstTimestampSec  float64
	LastTimestampSec   float64
	Caveats            []string
}

// DefaultOutputPath appends the fixed text-trace suffix to the source path.
func DefaultOutputPath(input string) string {
	return input + defaultOutputSuffix
}

func (r Result) Summary() string {
	return fmt.Sprintf("%s -> %s, %d events, %d skipped missing format(s), %d unsupported renderer row(s)",
		r.InputPath, r.OutputPath, r.EventsWritten, r.MissingFormatCount, r.UnknownEventCount)
}
