package tracequery

import (
	"fmt"
	"path/filepath"
	"strings"
)

// perfBundleAdmissionSummary is the parse-time audit record for a companion
// perftrace. Clock compatibility proves only timestamp comparability; it does
// not prove that a text row in that artifact is scheduler evidence, nor that a
// sample's PID/TID/CPU coordinates are trustworthy. The typed bundle
// capability is therefore a second, independent hard gate.
type perfBundleAdmissionSummary struct {
	CapabilityDeclared     bool
	TraceQueryReady        bool
	ThreadIdentity         string
	ThreadIdentityProven   bool
	CPUIdentity            string
	CPUIdentityProven      bool
	ObservedEvents         int
	SharedPerfSamples      int
	OmittedNotReady        int
	OmittedNonPerf         int
	OmittedSchedulerRows   int
	ThreadIdentityScrubbed int
	CPUIdentityScrubbed    int
}

// applyPerfBundleAdmission admits only normalized perf_sample rows from a
// bundle perftrace. The current PerfArtifactCapability schema attests sample
// identity/quality; it has no field that can attest scheduler-row provenance.
// Consequently sched_switch/wakeup/CPU-control/etc. rows in a .perftrace are
// never shared causal evidence, even when the sample capability itself is
// otherwise valid.
//
// A trace_query_ready sample with an unproven thread or CPU identity remains
// useful as anonymous symbol/DSO/event inventory. Its unsafe coordinates are
// erased before any thread/CPU-scoped query can consume it. An artifact that is
// not explicitly trace_query_ready contributes no event to the shared stream;
// its physical source and observed-event count remain in TraceArtifacts and
// the admission caveat below.
func applyPerfBundleAdmission(events []Event, capability *traceBundlePerfCapability) ([]Event, perfBundleAdmissionSummary) {
	summary := perfBundleAdmissionSummary{ObservedEvents: len(events)}
	if capability != nil {
		summary.CapabilityDeclared = true
		summary.TraceQueryReady = capability.TraceQueryReady
		summary.ThreadIdentity = strings.TrimSpace(capability.ThreadIdentity)
		summary.CPUIdentity = strings.TrimSpace(capability.CPUIdentity)
		summary.ThreadIdentityProven = perfThreadIdentityCapabilityProven(summary.ThreadIdentity)
		summary.CPUIdentityProven = perfCPUIdentityCapabilityProven(summary.CPUIdentity)
	}
	if !summary.TraceQueryReady {
		summary.OmittedNotReady = len(events)
		for i := range events {
			if perfBundleRowIsSchedulerOrCPU(events[i].Type) {
				summary.OmittedSchedulerRows++
			}
		}
		return nil, summary
	}

	// Compact in place. Event is intentionally large; allocating a second
	// max-sized child slice merely to discard unattested rows can transiently
	// double a dense bundle's memory footprint.
	admitted := events[:0]
	for i := range events {
		ev := events[i]
		if ev.Type != EventPerfSample || ev.PerfFields == nil {
			summary.OmittedNonPerf++
			if perfBundleRowIsSchedulerOrCPU(ev.Type) {
				summary.OmittedSchedulerRows++
			}
			continue
		}
		if !summary.ThreadIdentityProven {
			scrubPerfSampleThreadIdentity(&ev)
			summary.ThreadIdentityScrubbed++
		}
		if !summary.CPUIdentityProven || (ev.PerfFields.CPUKnown != nil && !*ev.PerfFields.CPUKnown) {
			scrubPerfSampleCPUIdentity(&ev)
			summary.CPUIdentityScrubbed++
		}
		admitted = append(admitted, ev)
		summary.SharedPerfSamples++
	}
	for i := len(admitted); i < len(events); i++ {
		events[i] = Event{}
	}
	return admitted, summary
}

// These are the closed, converter-owned enum values currently emitted by
// internal/hitraceconv, plus the two legacy normalized-manifest values already
// accepted by production fixtures. Arbitrary prose such as "available" must
// not become a hard-gate signal.
func perfThreadIdentityCapabilityProven(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pid_tid",
		"pid_tid_from_sample_or_comm",
		"virtual_thread_info_pid_tid_name",
		"sample_pid_tid_thread_comm",
		"thread_table_pid_tid_name":
		return true
	default:
		return false
	}
}

func perfCPUIdentityCapabilityProven(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sample_cpu",
		"sample_cpu_optional",
		"sample_cpu_when_recorded":
		return true
	default:
		return false
	}
}

func scrubPerfSampleThreadIdentity(ev *Event) {
	if ev == nil {
		return
	}
	ev.Comm = ""
	ev.PID = 0
	ev.TGID = 0
	if ev.PerfFields != nil {
		ev.PerfFields.PID = 0
		ev.PerfFields.TID = 0
		ev.PerfFields.Comm = ""
	}
}

func scrubPerfSampleCPUIdentity(ev *Event) {
	if ev == nil {
		return
	}
	ev.CPU = -1
	if ev.PerfFields != nil {
		ev.PerfFields.CPUKnown = boolPtr(false)
	}
}

func perfBundleRowIsSchedulerOrCPU(typ EventType) bool {
	switch typ {
	case EventSchedSwitch, EventSchedWakeup, EventSchedWaking,
		EventSchedBlockedReason, EventSchedStat,
		EventCPUIdle, EventCPUFrequency, EventCPUFrequencyLimit,
		EventCPUConstraint, EventClockSetRate:
		return true
	default:
		return false
	}
}

func (summary perfBundleAdmissionSummary) caveat(sourcePath string) string {
	threadIdentity := traceBundleCompactValue(summary.ThreadIdentity)
	if threadIdentity == "" {
		threadIdentity = "missing"
	}
	cpuIdentity := traceBundleCompactValue(summary.CPUIdentity)
	if cpuIdentity == "" {
		cpuIdentity = "missing"
	}
	return fmt.Sprintf(
		"tracebundle_perf_admission artifact=%s capability_declared=%t trace_query_ready=%t thread_identity=%s thread_identity_proven=%t cpu_identity=%s cpu_identity_proven=%t observed_events=%d shared_perf_samples=%d omitted_not_ready=%d omitted_non_perf=%d scheduler_or_cpu_rows_omitted=%d thread_identity_scrubbed=%d cpu_identity_scrubbed=%d policy=perf_sample_only",
		filepath.Base(sourcePath), summary.CapabilityDeclared, summary.TraceQueryReady,
		threadIdentity, summary.ThreadIdentityProven, cpuIdentity, summary.CPUIdentityProven,
		summary.ObservedEvents, summary.SharedPerfSamples, summary.OmittedNotReady,
		summary.OmittedNonPerf, summary.OmittedSchedulerRows,
		summary.ThreadIdentityScrubbed, summary.CPUIdentityScrubbed,
	)
}
