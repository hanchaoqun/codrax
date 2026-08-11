package types

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TraceRootCauseReportSchemaVersion is the user-facing JSON sidecar schema.
const TraceRootCauseReportSchemaVersion = 1

// TraceRootCauseCategory is the closed, stable root-cause vocabulary exposed
// in the JSON sidecar. Evidence remains concise free text and is deliberately
// not constrained to this vocabulary.
type TraceRootCauseCategory string

const (
	TraceRootCauseIOBlocking            TraceRootCauseCategory = "io_blocking"
	TraceRootCauseLockContention        TraceRootCauseCategory = "lock_contention"
	TraceRootCauseSynchronousBinder     TraceRootCauseCategory = "synchronous_binder"
	TraceRootCausePriorityInversion     TraceRootCauseCategory = "priority_inversion"
	TraceRootCauseGCLongPause           TraceRootCauseCategory = "gc_long_pause"
	TraceRootCauseCPUSchedulingDelay    TraceRootCauseCategory = "cpu_scheduling_delay"
	TraceRootCausePhaseHighLoad         TraceRootCauseCategory = "phase_high_load"
	TraceRootCauseJITCompilation        TraceRootCauseCategory = "jit_compilation"
	TraceRootCauseShaderCompilation     TraceRootCauseCategory = "shader_compilation"
	TraceRootCauseSleepBlocking         TraceRootCauseCategory = "sleep_blocking"
	TraceRootCauseComputeSupplyShortage TraceRootCauseCategory = "compute_supply_shortage"
)

func AllTraceRootCauseCategories() []TraceRootCauseCategory {
	return []TraceRootCauseCategory{
		TraceRootCauseIOBlocking,
		TraceRootCauseLockContention,
		TraceRootCauseSynchronousBinder,
		TraceRootCausePriorityInversion,
		TraceRootCauseGCLongPause,
		TraceRootCauseCPUSchedulingDelay,
		TraceRootCausePhaseHighLoad,
		TraceRootCauseJITCompilation,
		TraceRootCauseShaderCompilation,
		TraceRootCauseSleepBlocking,
		TraceRootCauseComputeSupplyShortage,
	}
}

// TraceRootCauseItemV1 is one structured conclusion plus its corresponding
// concise evidence. Summary is normalized by the runtime from Category and
// the relevant identity field so the wire format stays consistent.
type TraceRootCauseItemV1 struct {
	Category     TraceRootCauseCategory `json:"category"`
	ThreadName   string                 `json:"thread_name,omitempty"`
	ResourceName string                 `json:"resource_name,omitempty"`
	PhaseName    string                 `json:"phase_name,omitempty"`
	Summary      string                 `json:"summary"`
	Evidence     []string               `json:"evidence"`
}

// TraceRootCauseReportV1 is written next to the full Markdown/HTML answer.
// RootCause2 is explicitly nullable because one supported cause is better
// than a fabricated runner-up.
type TraceRootCauseReportV1 struct {
	SchemaVersion int                   `json:"schema_version"`
	RootCause1    *TraceRootCauseItemV1 `json:"root_cause_1"`
	RootCause2    *TraceRootCauseItemV1 `json:"root_cause_2"`
}

// NormalizeAndValidateTraceRootCauseReport validates model-authored semantic
// choices, compacts evidence whitespace, and owns the fixed Chinese summary
// spelling. It never derives a cause from the long answer prose.
func NormalizeAndValidateTraceRootCauseReport(in *TraceRootCauseReportV1) (*TraceRootCauseReportV1, error) {
	if in == nil {
		return nil, fmt.Errorf("trace_root_causes is required")
	}
	if in.SchemaVersion != TraceRootCauseReportSchemaVersion {
		return nil, fmt.Errorf("trace_root_causes schema_version=%d, want %d", in.SchemaVersion, TraceRootCauseReportSchemaVersion)
	}
	if in.RootCause1 == nil {
		return nil, fmt.Errorf("trace_root_causes.root_cause_1 is required")
	}
	out := &TraceRootCauseReportV1{SchemaVersion: TraceRootCauseReportSchemaVersion}
	var err error
	if out.RootCause1, err = normalizeTraceRootCauseItem(in.RootCause1, "root_cause_1"); err != nil {
		return nil, err
	}
	if in.RootCause2 != nil {
		if out.RootCause2, err = normalizeTraceRootCauseItem(in.RootCause2, "root_cause_2"); err != nil {
			return nil, err
		}
		if out.RootCause1.Summary == out.RootCause2.Summary {
			return nil, fmt.Errorf("trace_root_causes.root_cause_2 duplicates root_cause_1")
		}
	}
	return out, nil
}

func normalizeTraceRootCauseItem(in *TraceRootCauseItemV1, field string) (*TraceRootCauseItemV1, error) {
	if in == nil {
		return nil, nil
	}
	out := &TraceRootCauseItemV1{
		Category:     in.Category,
		ThreadName:   compactTraceRootCauseField(in.ThreadName),
		ResourceName: compactTraceRootCauseField(in.ResourceName),
		PhaseName:    compactTraceRootCauseField(in.PhaseName),
	}
	if !validTraceRootCauseCategory(out.Category) {
		return nil, fmt.Errorf("trace_root_causes.%s.category=%q is unsupported", field, out.Category)
	}
	if traceRootCauseNeedsThread(out.Category) && out.ThreadName == "" {
		return nil, fmt.Errorf("trace_root_causes.%s.thread_name is required for category %q", field, out.Category)
	}
	if out.Category == TraceRootCauseLockContention && out.ResourceName == "" {
		return nil, fmt.Errorf("trace_root_causes.%s.resource_name is required for lock_contention", field)
	}
	if out.Category == TraceRootCausePhaseHighLoad && out.PhaseName == "" {
		return nil, fmt.Errorf("trace_root_causes.%s.phase_name is required for phase_high_load", field)
	}
	if len(in.Evidence) == 0 || len(in.Evidence) > 4 {
		return nil, fmt.Errorf("trace_root_causes.%s.evidence must contain 1 to 4 concise entries", field)
	}
	for index, evidence := range in.Evidence {
		evidence = compactTraceRootCauseField(evidence)
		if evidence == "" {
			return nil, fmt.Errorf("trace_root_causes.%s.evidence[%d] is empty", field, index)
		}
		if utf8.RuneCountInString(evidence) > 240 {
			return nil, fmt.Errorf("trace_root_causes.%s.evidence[%d] exceeds 240 characters", field, index)
		}
		out.Evidence = append(out.Evidence, evidence)
	}
	out.Summary = traceRootCauseSummary(*out)
	return out, nil
}

func validTraceRootCauseCategory(category TraceRootCauseCategory) bool {
	for _, candidate := range AllTraceRootCauseCategories() {
		if category == candidate {
			return true
		}
	}
	return false
}

func traceRootCauseNeedsThread(category TraceRootCauseCategory) bool {
	switch category {
	case TraceRootCauseIOBlocking,
		TraceRootCauseSynchronousBinder,
		TraceRootCausePriorityInversion,
		TraceRootCauseCPUSchedulingDelay,
		TraceRootCauseJITCompilation,
		TraceRootCauseShaderCompilation,
		TraceRootCauseSleepBlocking:
		return true
	default:
		return false
	}
}

func traceRootCauseSummary(item TraceRootCauseItemV1) string {
	switch item.Category {
	case TraceRootCauseIOBlocking:
		return item.ThreadName + "线程IO阻塞"
	case TraceRootCauseLockContention:
		return item.ResourceName + "锁竞争"
	case TraceRootCauseSynchronousBinder:
		return item.ThreadName + "线程同步binder"
	case TraceRootCausePriorityInversion:
		return item.ThreadName + "线程优先级反转"
	case TraceRootCauseGCLongPause:
		return "GC耗时长"
	case TraceRootCauseCPUSchedulingDelay:
		return item.ThreadName + "线程CPU调度延迟"
	case TraceRootCausePhaseHighLoad:
		return item.PhaseName + "阶段高负载"
	case TraceRootCauseJITCompilation:
		return item.ThreadName + "线程JIT编译耗时"
	case TraceRootCauseShaderCompilation:
		return item.ThreadName + "线程Shader编译"
	case TraceRootCauseSleepBlocking:
		return item.ThreadName + "线程阻塞"
	case TraceRootCauseComputeSupplyShortage:
		return "供给不足"
	default:
		return ""
	}
}

func compactTraceRootCauseField(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func cloneTraceRootCauseReportV1(in *TraceRootCauseReportV1) *TraceRootCauseReportV1 {
	if in == nil {
		return nil
	}
	out := *in
	if in.RootCause1 != nil {
		cause := *in.RootCause1
		cause.Evidence = append([]string(nil), in.RootCause1.Evidence...)
		out.RootCause1 = &cause
	}
	if in.RootCause2 != nil {
		cause := *in.RootCause2
		cause.Evidence = append([]string(nil), in.RootCause2.Evidence...)
		out.RootCause2 = &cause
	}
	return &out
}
