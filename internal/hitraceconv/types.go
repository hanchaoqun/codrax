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
}

type Artifact struct {
	Type          string                  `json:"type"`
	Path          string                  `json:"path"`
	Bytes         int64                   `json:"bytes,omitempty"`
	DataType      uint32                  `json:"data_type,omitempty"`
	PluginName    string                  `json:"plugin_name,omitempty"`
	PluginVersion string                  `json:"plugin_version,omitempty"`
	SourceOffset  int64                   `json:"source_offset,omitempty"`
	SourceBytes   int64                   `json:"source_bytes,omitempty"`
	Converter     string                  `json:"converter,omitempty"`
	Perf          *PerfArtifactCapability `json:"perf_capability,omitempty"`
	Caveats       []string                `json:"caveats,omitempty"`
}

type PerfArtifactCapability struct {
	ProviderKind    string   `json:"provider_kind,omitempty"`
	ProviderName    string   `json:"provider_name,omitempty"`
	InputFormat     string   `json:"input_format,omitempty"`
	OutputFormat    string   `json:"output_format,omitempty"`
	TimeDomain      string   `json:"time_domain,omitempty"`
	TimeAlignment   string   `json:"time_alignment,omitempty"`
	ThreadIdentity  string   `json:"thread_identity,omitempty"`
	CPUIdentity     string   `json:"cpu_identity,omitempty"`
	EventWeight     string   `json:"event_weight,omitempty"`
	Symbolization   string   `json:"symbolization,omitempty"`
	Callchain       string   `json:"callchain,omitempty"`
	DSOLabel        string   `json:"dso_label,omitempty"`
	BuildID         string   `json:"build_id,omitempty"`
	OffCPU          string   `json:"off_cpu,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	TraceQueryReady bool     `json:"trace_query_ready,omitempty"`
	Degraded        bool     `json:"degraded,omitempty"`
	Caveats         []string `json:"caveats,omitempty"`
}

// Result summarizes a completed conversion.
type Result struct {
	InputPath          string
	OutputPath         string
	BundlePath         string
	Artifacts          []Artifact
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
