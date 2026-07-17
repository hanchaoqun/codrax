package hitraceconv

import (
	"context"
	"fmt"
	"io"
	"os"
)

type perfInputFormat string

const (
	perfInputUnknown               perfInputFormat = ""
	perfInputLinuxPerfData         perfInputFormat = "linux_perf_data"
	perfInputGzipPerfData          perfInputFormat = "gzip_perf_data"
	perfInputSimpleperfReportProto perfInputFormat = "simpleperf_report_sample_proto"
	perfInputPerfTraceText         perfInputFormat = "codrax_perftrace_text"
)

func (format perfInputFormat) valid() bool {
	switch format {
	case perfInputUnknown, perfInputLinuxPerfData, perfInputGzipPerfData, perfInputSimpleperfReportProto, perfInputPerfTraceText:
		return true
	default:
		return false
	}
}

func detectPerfInputFormatFromView(ctx context.Context, input conversionInputView, stage conversionInputStage) (format perfInputFormat, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := completeConversionInputStage(ctx, input, stage, nil); err != nil {
		return perfInputUnknown, err
	}
	defer func() {
		err = completeConversionInputStage(ctx, input, stage, err)
		if err != nil {
			format = perfInputUnknown
		}
	}()
	if input == nil || input.Size() < 0 {
		return perfInputUnknown, fmt.Errorf("perf input view is incomplete")
	}
	length := input.Size()
	if length > conversionInputProbeSize {
		length = conversionInputProbeSize
	}
	probe := make([]byte, int(length))
	if length > 0 {
		n, readErr := input.ReadAt(probe, 0)
		if readErr != nil && readErr != io.EOF {
			return perfInputUnknown, readErr
		}
		if n != len(probe) {
			return perfInputUnknown, io.ErrUnexpectedEOF
		}
	}
	return detectPerfInputFormatProbe(probe), nil
}

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
	return detectPerfInputFormatProbe(header[:n])
}

// detectPerfInputFormatProbe is the content-only classifier used by the
// conversion transaction after its immutable input authority has captured the
// one fixed-size route probe. Path-based status inspection may use the wrapper
// above, but production conversion must not reopen or reclassify the source.
func detectPerfInputFormatProbe(data []byte) perfInputFormat {
	if len(data) > conversionInputProbeSize {
		data = data[:conversionInputProbeSize]
	}
	switch {
	case hasPrefixBytes(data, []byte(perfMagic2)):
		return perfInputLinuxPerfData
	case hasPrefixBytes(data, []byte{0x1f, 0x8b}):
		return perfInputGzipPerfData
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
		InputFormat:     firstNonEmpty(string(inputFormat), "unknown"),
		OutputFormat:    "codrax_perftrace",
		TimeDomain:      "perf_data_time_ns",
		TimeAlignment:   "assumed",
		ThreadIdentity:  "present_valid_sample_pid_tid_only",
		CPUIdentity:     "present_valid_sample_cpu_else_unknown",
		EventWeight:     "present_valid_period_zero_as_sample_count",
		Symbolization:   "hiperf_saved_symbols_or_unsymbolized_ip",
		Callchain:       "symbolized_when_hiperf_files_symbol_present_else_ip_only",
		DSOLabel:        "mmap_best_effort",
		BuildID:         "feature_build_id_when_present",
		OffCPU:          "hiperf_cpu_off_sched_switch_when_event_desc_present",
		Confidence:      "degraded",
		TraceQueryReady: false,
		Degraded:        true,
		Caveats: []string{
			"raw fallback resolves function names only from saved hiperf symbol sections; without those sections it remains IP/DSO-level",
			"raw fallback can label hiperf --offcpu sched_switch samples when official EVENT_DESC and HIPERF_CPU_OFF features are present, but full off-CPU stack expansion still needs official hiperf report flow",
			"structurally parsed samples without required time, thread identity, or period remain receipt-bound inventory and never receive synthesized coordinates or weight",
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
		TraceQueryReady: false,
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
		TraceQueryReady: false,
		Caveats: []string{
			"official report_sample.py text carries six decimal places; timestamps preserve microsecond precision and do not restore discarded sub-microsecond nanoseconds",
			"a present zero PID or TID is retained as idle/pseudo sample inventory and is not upgraded into an ordinary selectable thread by its display comm",
			"a present zero period is normalized to one sample count under the declared period_or_1 policy",
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
		TraceQueryReady: false,
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
		InputFormat:     firstNonEmpty(string(inputFormat), "unknown"),
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
			"raw perf.data is preserved for audit; trace_query consumes normalized .perftrace only when its validation receipt proves query-ready sample rows",
		},
	}
}
