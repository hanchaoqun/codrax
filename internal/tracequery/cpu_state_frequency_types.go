package tracequery

const ViewCPUStateFrequency = "cpu_state_frequency"

// Shared by tool schema and view-selection teaching; no extra user fields.
const CPUStateFrequencyTeaching = "Use cpu_state_frequency for per-CPU state × frequency residency over time_start/time_end; omit pid/thread. The tool handles carry-in, interval boundaries and complete-window percentages. Raw ftrace idle 0 remains idle, and active requires an idle exit; native_sql_idle is an observed source code with unverified meaning, never Running/C-state naming. Preserved SQL measure.ts/dur intervals do not fill gaps or overlaps; absent duration/state/frequency remains unknown. Without an independently established capture range, measure row timestamps do not prove whole-capture coverage. Keep source/CPU/topology identity, CPU-time versus wall time, kHz units and omitted counts. Unavailable means missing measurement, not zero. Residency alone proves neither thread execution, power, compute shortage nor a response cause; use thread_timeline/root_cause_rank for independently supported thread causality."

// CPUStateFrequencyResult is a CPU-control observation, never a thread-state
// account, hardware-capacity model, or causal attribution. CPUTimeMs sums the
// full window once per observed CPU; WindowWallMs counts that window only once.
type CPUStateFrequencyResult struct {
	Status         string                 `json:"status"`
	Reason         string                 `json:"reason,omitempty"`
	SourcePath     string                 `json:"source_path"`
	Window         TimeWindow             `json:"window"`
	WindowWallMs   float64                `json:"window_wall_ms"`
	CPUCount       int                    `json:"cpu_count"`
	CPUTimeMs      float64                `json:"cpu_time_ms"`
	KnownJointMs   float64                `json:"known_joint_ms"`
	UnknownJointMs float64                `json:"unknown_joint_ms"`
	OmittedCPUs    int                    `json:"omitted_cpus"`
	CPUs           []CPUStateFrequencyCPU `json:"cpus"`
	Caveats        []string               `json:"caveats,omitempty"`
}

type CPUStateFrequencyCPU struct {
	CPU                  int                         `json:"cpu"`
	CoreClass            string                      `json:"core_class"`
	TopologySource       string                      `json:"topology_source"`
	WindowWallMs         float64                     `json:"window_wall_ms"`
	IdleKnownMs          float64                     `json:"idle_known_ms"`
	FrequencyKnownMs     float64                     `json:"frequency_known_ms"`
	JointKnownMs         float64                     `json:"joint_known_ms"`
	UnknownJointMs       float64                     `json:"unknown_joint_ms"`
	Groups               []CPUStateFrequencyGroup    `json:"groups"`
	GroupCount           int                         `json:"group_count"`
	OmittedGroups        int                         `json:"omitted_groups"`
	Intervals            []CPUStateFrequencyInterval `json:"intervals"`
	TotalIntervals       int                         `json:"total_intervals"`
	OmittedIntervals     int                         `json:"omitted_intervals"`
	IdleUnavailable      string                      `json:"idle_unavailable,omitempty"`
	FrequencyUnavailable string                      `json:"frequency_unavailable,omitempty"`
}

// State=active means an explicit cpu_idle exit was observed. It is not proof
// that a particular thread was running. Raw idle state 0 remains idle index 0;
// no platform-independent C1/C2/C3 naming is inferred.
type CPUStateFrequencyValue struct {
	State string `json:"state"`
	// native_sql_idle labels an observed opaque source code, never an idle
	// entry/exit interpretation. Empty preserves the ftrace control contract.
	StateEncoding  string  `json:"state_encoding,omitempty"`
	StateKnown     bool    `json:"state_known"`
	IdleState      *uint32 `json:"idle_state,omitempty"`
	FrequencyKnown bool    `json:"frequency_known"`
	FrequencyKHz   *int64  `json:"frequency_khz,omitempty"`
}

type CPUStateFrequencyGroup struct {
	CPUStateFrequencyValue
	DurationMs float64 `json:"duration_ms"`
	WindowPct  float64 `json:"window_pct"`
}

type CPUStateFrequencyInterval struct {
	CPUStateFrequencyValue
	StartTs       float64 `json:"start_ts"`
	EndTs         float64 `json:"end_ts"`
	DurationMs    float64 `json:"duration_ms"`
	IdleLine      int     `json:"idle_line,omitempty"`
	FrequencyLine int     `json:"frequency_line,omitempty"`
}

const (
	cpuStateFrequencySampleLimit   = 1 << 20
	cpuStateFrequencyCPULimit      = 64
	cpuStateFrequencyGroupLimit    = 64
	cpuStateFrequencyIntervalLimit = 64
)
