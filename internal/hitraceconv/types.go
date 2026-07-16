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
	Progress               ProgressFunc
}

type Artifact struct {
	Type          string                   `json:"type"`
	Path          string                   `json:"path"`
	Bytes         int64                    `json:"bytes"`
	SHA256        string                   `json:"sha256,omitempty"`
	DataType      uint32                   `json:"data_type,omitempty"`
	PluginName    string                   `json:"plugin_name,omitempty"`
	PluginVersion string                   `json:"plugin_version,omitempty"`
	SourceOffset  int64                    `json:"source_offset,omitempty"`
	SourceBytes   int64                    `json:"source_bytes,omitempty"`
	Converter     string                   `json:"converter,omitempty"`
	Trace         *TraceArtifactCapability `json:"trace_capability,omitempty"`
	Perf          *PerfArtifactCapability  `json:"perf_capability,omitempty"`
	Caveats       []string                 `json:"caveats,omitempty"`
	// These paths are factory-only, in-memory receipt bindings. The first is
	// the frozen absolute ledger identity; the second pins the user-facing
	// spelling so later validation cannot relabel a valid generation. Neither
	// is serialized into a bundle.
	traceReceiptBindingPath  string `json:"-"`
	traceReceiptArtifactPath string `json:"-"`
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
