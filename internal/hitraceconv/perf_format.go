package hitraceconv

import (
	"io"
	"os"
)

type perfInputFormat string

const (
	perfInputUnknown               perfInputFormat = ""
	perfInputLinuxPerfData         perfInputFormat = "linux_perf_data"
	perfInputSimpleperfReportProto perfInputFormat = "simpleperf_report_sample_proto"
	perfInputPerfTraceText         perfInputFormat = "codrax_perftrace_text"
)

func detectPerfInputFormat(path string) perfInputFormat {
	f, err := os.Open(path)
	if err != nil {
		return perfInputUnknown
	}
	defer f.Close()
	var header [64]byte
	n, err := io.ReadFull(f, header[:])
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return perfInputUnknown
	}
	data := header[:n]
	switch {
	case hasPrefixBytes(data, []byte(perfMagic2)):
		return perfInputLinuxPerfData
	case hasPrefixBytes(data, []byte("SIMPLEPERF")):
		return perfInputSimpleperfReportProto
	case containsBytes(data, []byte("perf_sample:")):
		return perfInputPerfTraceText
	default:
		return perfInputUnknown
	}
}

func hasRawPerfDataMagic(path string) bool {
	return detectPerfInputFormat(path) == perfInputLinuxPerfData
}

func hasPrefixBytes(data, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i := range prefix {
		if data[i] != prefix[i] {
			return false
		}
	}
	return true
}

func containsBytes(data, needle []byte) bool {
	if len(needle) == 0 || len(data) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(data); i++ {
		if hasPrefixBytes(data[i:], needle) {
			return true
		}
	}
	return false
}

func perfCapabilityForRawFallback(inputFormat perfInputFormat) *PerfArtifactCapability {
	return &PerfArtifactCapability{
		ProviderKind:    "raw_fallback",
		ProviderName:    "codrax_raw_perfdata",
		InputFormat:     firstNonEmpty(string(inputFormat), string(perfInputLinuxPerfData)),
		OutputFormat:    "codrax_perftrace",
		TimeDomain:      "perf_data_time_ns",
		TimeAlignment:   "assumed",
		ThreadIdentity:  "pid_tid_from_sample_or_comm",
		CPUIdentity:     "sample_cpu_when_recorded",
		EventWeight:     "period_or_1",
		Symbolization:   "hiperf_saved_symbols_or_unsymbolized_ip",
		Callchain:       "symbolized_when_hiperf_files_symbol_present_else_ip_only",
		DSOLabel:        "mmap_best_effort",
		BuildID:         "feature_build_id_when_present",
		OffCPU:          "hiperf_cpu_off_sched_switch_when_event_desc_present",
		Confidence:      "degraded",
		TraceQueryReady: true,
		Degraded:        true,
		Caveats: []string{
			"raw fallback resolves function names only from saved hiperf symbol sections; without those sections it remains IP/DSO-level",
			"raw fallback can label hiperf --offcpu sched_switch samples when official EVENT_DESC and HIPERF_CPU_OFF features are present, but full off-CPU stack expansion still needs official hiperf report flow",
		},
	}
}

func perfCapabilityForHiperfProto(source string) *PerfArtifactCapability {
	return &PerfArtifactCapability{
		ProviderKind:    "official_harmony",
		ProviderName:    "openharmony_hiperf_report_proto",
		InputFormat:     string(perfInputLinuxPerfData),
		OutputFormat:    "codrax_perftrace",
		TimeDomain:      "monotonic_raw_ns",
		TimeAlignment:   "assumed",
		ThreadIdentity:  "virtual_thread_info_pid_tid_name",
		CPUIdentity:     "unavailable_in_report_sample_proto",
		EventWeight:     "event_count_or_1",
		Symbolization:   "symbol_table_file_function_name",
		Callchain:       "symbolized_when_report_contains_frames",
		DSOLabel:        "symbol_table_file_path",
		BuildID:         "not_exposed_by_report_sample_proto",
		OffCPU:          "not_exposed_by_current_adapter",
		Confidence:      "high_when_official_tool_succeeds",
		TraceQueryReady: true,
		Caveats: []string{
			"OpenHarmony report_sample.proto has no sample CPU field; trace_query must keep cpu=-1 as unknown, not a concrete CPU/core attribution",
			"clock alignment is assumed unless a future capture-level clock map is available",
			"provider source: " + firstNonEmpty(source, "unknown"),
		},
	}
}

func perfCapabilityForSimpleperfReportSample(inputFormat perfInputFormat, source string) *PerfArtifactCapability {
	if inputFormat == perfInputUnknown {
		inputFormat = perfInputLinuxPerfData
	}
	return &PerfArtifactCapability{
		ProviderKind:    "official_android",
		ProviderName:    "android_simpleperf_report_sample",
		InputFormat:     string(inputFormat),
		OutputFormat:    "codrax_perftrace",
		TimeDomain:      "simpleperf_record_clock_ns",
		TimeAlignment:   "assumed",
		ThreadIdentity:  "sample_pid_tid_thread_comm",
		CPUIdentity:     "sample_cpu",
		EventWeight:     "period_or_1",
		Symbolization:   "simpleperf_symbol_dso",
		Callchain:       "simpleperf_callchain_entries",
		DSOLabel:        "simpleperf_dso_name",
		BuildID:         "available_to_official_library_not_emitted_by_text_adapter",
		OffCPU:          "supported_by_official_library_when_trace_offcpu_is_recorded",
		Confidence:      "high_when_official_tool_succeeds",
		TraceQueryReady: true,
		Caveats: []string{
			"clock alignment is assumed unless a future capture-level clock map is available",
			"provider source: " + firstNonEmpty(source, "unknown"),
		},
	}
}

func perfCapabilityForSimpleperfReportProto(source string) *PerfArtifactCapability {
	return &PerfArtifactCapability{
		ProviderKind:    "official_android",
		ProviderName:    "android_simpleperf_report_proto",
		InputFormat:     string(perfInputSimpleperfReportProto),
		OutputFormat:    "codrax_perftrace",
		TimeDomain:      "simpleperf_record_clock_ns",
		TimeAlignment:   "assumed",
		ThreadIdentity:  "thread_table_pid_tid_name",
		CPUIdentity:     "unavailable_in_cmd_report_sample_proto",
		EventWeight:     "event_count_or_1",
		Symbolization:   "file_symbol_table",
		Callchain:       "symbolized_when_report_contains_frames",
		DSOLabel:        "file_path",
		BuildID:         "not_exposed_by_cmd_report_sample_proto",
		OffCPU:          "trace_offcpu_flag_and_context_switch_records",
		Confidence:      "high_when_official_proto_is_well_formed",
		TraceQueryReady: true,
		Caveats: []string{
			"Android cmd_report_sample.proto has no sample CPU field; trace_query must keep cpu=-1 as unknown, not a concrete CPU/core attribution",
			"clock alignment is assumed unless a future capture-level clock map is available",
			"provider source: " + firstNonEmpty(source, "unknown"),
		},
	}
}

func perfCapabilityForRawPerfDataArtifact(inputFormat perfInputFormat) *PerfArtifactCapability {
	return &PerfArtifactCapability{
		ProviderKind:    "source_artifact",
		ProviderName:    "perf_data_sidecar",
		InputFormat:     firstNonEmpty(string(inputFormat), string(perfInputLinuxPerfData)),
		OutputFormat:    "perf.data",
		TimeDomain:      "producer_defined",
		TimeAlignment:   "unknown_until_converted",
		ThreadIdentity:  "unknown_until_converted",
		CPUIdentity:     "unknown_until_converted",
		EventWeight:     "unknown_until_converted",
		Symbolization:   "requires_official_or_raw_adapter",
		Callchain:       "requires_official_or_raw_adapter",
		DSOLabel:        "requires_official_or_raw_adapter",
		BuildID:         "requires_official_or_raw_adapter",
		OffCPU:          "requires_official_or_raw_adapter",
		Confidence:      "unconverted",
		TraceQueryReady: false,
		Degraded:        true,
		Caveats: []string{
			"raw perf.data is preserved for audit; trace_query consumes the normalized .perftrace sidecar when generated",
		},
	}
}
