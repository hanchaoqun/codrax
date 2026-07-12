package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	profilerDataTypeProtobuf = uint32(0)
	profilerSessionJSONTag   = "SessionJSON-"
	maxProfilerTextLineBytes = 1024 * 1024
)

type profilerTraceHeader struct {
	Offset        int64
	Length        uint64
	Version       uint32
	Segments      uint32
	DataType      uint32
	PluginName    string
	PluginVersion string
}

type profilerPluginData struct {
	Name               string
	Status             uint32
	Data               []byte
	ClockID            uint64
	ClockIDPresent     bool
	ClockIDAmbiguous   bool
	TvSec              uint64
	TvSecPresent       bool
	TvNsec             uint64
	TvNsecPresent      bool
	TimeTupleAmbiguous bool
	Version            string
	SampleInterval     uint32
}

type profilerPluginDataDecode struct {
	Plugin   profilerPluginData
	Accepted bool
	Issues   []string
}

type profilerContainerExtraction struct {
	Detected           bool
	Kind               string
	Messages           int
	PluginMessages     map[string]int
	StructuredFtrace   int
	MalformedFtrace    int
	UnsupportedFtrace  int
	TextPluginMessages int
	TextRows           int
	StructuredRows     int
	RejectedMessages   int
	StandaloneDetected bool
	TraceCoverage      []TraceDBCoverage
	Caveats            []string
}

type profilerFtraceSummary struct {
	Version           string
	StatsMessages     int
	StartStats        int
	EndStats          int
	TraceClocks       map[string]int
	StatsCPUs         map[uint64]bool
	StartTotals       profilerFtraceCPUTotals
	EndTotals         profilerFtraceCPUTotals
	StartTotalsValid  bool
	EndTotalsValid    bool
	DetailMessages    int
	DetailCPUs        map[uint64]bool
	DetailEventCount  int
	DetailOverwrite   uint64
	DetailOverwriteOK bool
	SymbolCount       int
	SymbolExamples    []string
	ClockDetails      []string
	EventFieldCounts  map[int]int
	Issues            []string
	recognizedMessage bool
}

type profilerFtraceCPUTotals struct {
	Entries       uint64
	Overrun       uint64
	CommitOverrun uint64
	Bytes         uint64
	DroppedEvents uint64
	ReadEvents    uint64
}

type profilerFtraceCPUStats struct {
	Status   uint64
	Clock    string
	PerCPU   []profilerFtracePerCPUStats
	HasStats bool
}

type profilerFtracePerCPUStats struct {
	CPU           uint64
	Entries       uint64
	Overrun       uint64
	CommitOverrun uint64
	Bytes         uint64
	DroppedEvents uint64
	ReadEvents    uint64
}

type profilerFtraceCPUDetail struct {
	CPU              uint64
	EventCount       int
	EventFieldCounts map[int]int
	Overwrite        uint64
	OverwriteValid   bool
}

type profilerFtraceSymbolDetail struct {
	Addr uint64
	Name string
}

type profilerFtraceClockDetail struct {
	ID       uint64
	TimeSec  uint64
	TimeNsec uint64
	ResSec   uint64
	ResNsec  uint64
	HasTime  bool
	HasRes   bool
}

type profilerFtraceEventDescriptor struct {
	Field  int
	Family string
	Name   string
}

var profilerFtraceEventDescriptors = map[int]profilerFtraceEventDescriptor{
	113:  {Field: 113, Family: "binder", Name: "binder_transaction"},
	119:  {Field: 119, Family: "binder", Name: "binder_transaction_received"},
	202:  {Field: 202, Family: "block", Name: "block_bio_complete"},
	204:  {Field: 204, Family: "block", Name: "block_bio_queue"},
	205:  {Field: 205, Family: "block", Name: "block_bio_remap"},
	209:  {Field: 209, Family: "block", Name: "block_rq_complete"},
	210:  {Field: 210, Family: "block", Name: "block_rq_insert"},
	211:  {Field: 211, Family: "block", Name: "block_rq_issue"},
	212:  {Field: 212, Family: "block", Name: "block_rq_remap"},
	410:  {Field: 410, Family: "clock", Name: "clock_set_rate"},
	1000: {Field: 1000, Family: "filemap", Name: "mm_filemap_add_to_page_cache"},
	1001: {Field: 1001, Family: "filemap", Name: "mm_filemap_delete_from_page_cache"},
	1109: {Field: 1109, Family: "trace_marker", Name: "print"},
	1500: {Field: 1500, Family: "irq", Name: "irq_handler_entry"},
	1501: {Field: 1501, Family: "irq", Name: "irq_handler_exit"},
	1502: {Field: 1502, Family: "irq", Name: "softirq_entry"},
	1503: {Field: 1503, Family: "irq", Name: "softirq_exit"},
	1504: {Field: 1504, Family: "irq", Name: "softirq_raise"},
	2002: {Field: 2002, Family: "clock", Name: "clock_set_rate"},
	2003: {Field: 2003, Family: "cpu", Name: "cpu_frequency"},
	2004: {Field: 2004, Family: "cpu", Name: "cpu_frequency_limits"},
	2005: {Field: 2005, Family: "cpu", Name: "cpu_idle"},
	2417: {Field: 2417, Family: "sched", Name: "sched_switch"},
	2420: {Field: 2420, Family: "sched", Name: "sched_wakeup"},
	2421: {Field: 2421, Family: "sched", Name: "sched_wakeup_new"},
	2422: {Field: 2422, Family: "sched", Name: "sched_waking"},
	4002: {Field: 4002, Family: "sched", Name: "sched_blocked_reason"},
	4009: {Field: 4009, Family: "f2fs", Name: "f2fs_sync_file_enter"},
	4010: {Field: 4010, Family: "f2fs", Name: "f2fs_sync_file_exit"},
	4011: {Field: 4011, Family: "f2fs", Name: "f2fs_write_begin"},
	4012: {Field: 4012, Family: "f2fs", Name: "f2fs_write_end"},
	4015: {Field: 4015, Family: "mmc", Name: "mmc_request_done"},
	4016: {Field: 4016, Family: "mmc", Name: "mmc_request_start"},
}

func modernRowSorterCoverage(stats traceDBRowSortStats) TraceDBCoverage {
	coverage := stats.coverage()
	coverage.Family = "builtin_modern_profiler"
	coverage.Table = "__systrace_rows__"
	return coverage
}

func tryConvertProfilerContainer(ctx context.Context, opts Options, inputSize int64, output string, standaloneArtifacts []Artifact, standaloneCaveats []string, standaloneDecisions []PerfProviderDecision, initialTraceDecisions []TraceProviderDecision, initialTraceDBCoverage []TraceDBCoverage) (Result, bool, error) {
	sink, err := newTraceDBRowSink("", 0)
	if err != nil {
		return Result{}, false, err
	}
	sinkClosed := false
	defer func() {
		if !sinkClosed {
			sink.cleanup()
		}
	}()
	extracted, err := extractProfilerContainerSystraceRows(ctx, opts.InputPath, inputSize, sink)
	if err != nil {
		return Result{}, false, err
	}
	if !extracted.Detected {
		return Result{}, false, nil
	}
	result := Result{
		InputPath:          opts.InputPath,
		InputBytes:         inputSize,
		Artifacts:          append([]Artifact(nil), standaloneArtifacts...),
		ProviderDecisions:  append([]PerfProviderDecision(nil), standaloneDecisions...),
		TraceDecisions:     append([]TraceProviderDecision(nil), initialTraceDecisions...),
		TraceDBCoverage:    append([]TraceDBCoverage(nil), initialTraceDBCoverage...),
		TraceCoverage:      append([]TraceDBCoverage(nil), extracted.TraceCoverage...),
		Caveats:            append([]string(nil), extracted.Caveats...),
		MissingFormatCount: 0,
		UnknownEventCount:  extracted.UnsupportedFtrace + extracted.RejectedMessages,
	}
	result.Caveats = append(result.Caveats, standaloneCaveats...)
	if sink.stats.RowsAccepted > 0 {
		out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return Result{}, true, err
		}
		stats, writeErr := sink.writeTo(ctx, out)
		sinkClosed = true
		closeErr := out.Close()
		if writeErr != nil {
			_ = os.Remove(output)
			result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(stats))
			return Result{}, true, writeErr
		}
		if closeErr != nil {
			_ = os.Remove(output)
			result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(stats))
			return Result{}, true, closeErr
		}
		info, err := os.Stat(output)
		if err != nil {
			return Result{}, true, err
		}
		result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(stats))
		result.OutputPath = output
		result.OutputBytes = info.Size()
		result.EventsWritten = stats.RowsWritten
		result.FirstTimestampSec = float64(stats.FirstTSNS) / 1e9
		result.LastTimestampSec = float64(stats.LastTSNS) / 1e9
		result.Artifacts = append([]Artifact{{
			Type:      ArtifactSystrace,
			Path:      output,
			Bytes:     info.Size(),
			Converter: converterVersion + "+openharmony-profiler",
			Caveats:   []string{"generated from OpenHarmony profiler/session plugin payloads"},
		}}, result.Artifacts...)
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderSuccess(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				Artifact{Type: ArtifactSystrace, Path: output},
			),
		)
	} else if len(result.Artifacts) == 0 {
		caveat := "OpenHarmony profiler/session container was detected, but no renderable trace rows or sidecar artifacts were found"
		result.Caveats = append(result.Caveats, caveat)
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderFailure(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				"no_renderable_trace_rows",
				caveat,
			),
		)
	} else {
		caveat := "OpenHarmony profiler/session container was detected, but only sidecar artifacts were produced"
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderFailure(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				"sidecar_only",
				caveat,
			),
		)
	}
	if sink.stats.RowsAccepted == 0 {
		result.TraceCoverage = append(result.TraceCoverage, modernRowSorterCoverage(sink.stats))
	}
	normalizeResultCollections(&result)
	if bundleArtifact, err := writeTraceBundleWithAllCoverage(opts.InputPath, result.OutputPath, result.Artifacts, result.Caveats, result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage); err != nil {
		return Result{}, true, err
	} else if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	normalizeResultCollections(&result)
	return result, true, nil
}

func extractProfilerContainerSystraceRows(ctx context.Context, path string, inputSize int64, sink *traceDBRowSink) (profilerContainerExtraction, error) {
	header, ok, err := readProfilerTraceHeaderAtPath(path, 0, inputSize)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if ok && header.DataType == profilerDataTypeProtobuf {
		return extractProfilerTraceFile(ctx, path, inputSize, header, sink)
	}
	session, err := extractProfilerSessionPackage(ctx, path, sink)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	if session.Detected {
		return session, nil
	}
	return profilerContainerExtraction{}, nil
}

func extractProfilerTraceFile(ctx context.Context, path string, inputSize int64, header profilerTraceHeader, sink *traceDBRowSink) (profilerContainerExtraction, error) {
	f, err := os.Open(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer f.Close()
	var limit int64
	out := profilerContainerExtraction{
		Detected:       true,
		Kind:           "openharmony_profiler_trace_file",
		PluginMessages: map[string]int{},
		Caveats: []string{
			fmt.Sprintf("OpenHarmony profiler TraceFileHeader detected: data_type=%d version=0x%x segments=%d length=%d", header.DataType, header.Version, header.Segments, header.Length),
		},
	}
	if header.Length < uint64(profilerTraceHeaderSize) {
		out.RejectedMessages++
		out.Caveats = append(out.Caveats, fmt.Sprintf("profiler TraceFileHeader has invalid declared length=%d below header size=%d; no plugin frame bytes are eligible", header.Length, profilerTraceHeaderSize))
		out.TraceCoverage = append(out.TraceCoverage, profilerContainerEnvelopeCoverage("trace_file_declared_length_invalid"))
		limit = profilerTraceHeaderSize
	} else if header.Length > uint64(inputSize) {
		out.RejectedMessages++
		out.Caveats = append(out.Caveats, fmt.Sprintf("profiler TraceFileHeader is truncated: declared length=%d available=%d; only complete framed messages within the available prefix are eligible", header.Length, inputSize))
		out.TraceCoverage = append(out.TraceCoverage, profilerContainerEnvelopeCoverage("trace_file_declared_length_truncated"))
		limit = inputSize
	} else {
		limit = int64(header.Length)
	}
	off := int64(profilerTraceHeaderSize)
	seq := 0
	for off+4 <= limit {
		if err := ctx.Err(); err != nil {
			return profilerContainerExtraction{}, err
		}
		var lenBuf [4]byte
		if _, err := f.ReadAt(lenBuf[:], off); err != nil {
			if errorsIsEOF(err) {
				break
			}
			return profilerContainerExtraction{}, fmt.Errorf("read profiler message length at %d: %w", off, err)
		}
		n := int64(binary.LittleEndian.Uint32(lenBuf[:]))
		if n == 0 {
			out.Messages++
			out.RejectedMessages++
			out.Caveats = append(out.Caveats, fmt.Sprintf("rejected zero-length ProfilerPluginData frame at offset %d; continued at the next framed sibling", off))
			out.TraceCoverage = append(out.TraceCoverage, profilerRejectedPluginCoverage("plugin_frame_zero_length"))
			off += 4
			continue
		}
		if off+4+n > limit {
			out.Messages++
			out.RejectedMessages++
			out.Caveats = append(out.Caveats, fmt.Sprintf("rejected truncated ProfilerPluginData frame at offset %d: declared=%d available=%d; sibling boundary cannot be recovered", off, n, limit-off-4))
			out.TraceCoverage = append(out.TraceCoverage, profilerRejectedPluginCoverage("plugin_frame_truncated"))
			break
		}
		msg := make([]byte, n)
		if _, err := f.ReadAt(msg, off+4); err != nil {
			return profilerContainerExtraction{}, fmt.Errorf("read profiler message at %d: %w", off+4, err)
		}
		out.Messages++
		decoded := parseProfilerPluginData(msg)
		if decoded.Accepted {
			plugin := decoded.Plugin
			name := firstNonEmpty(plugin.Name, "unknown-plugin")
			out.PluginMessages[name]++
			out.Caveats = append(out.Caveats, profilerPluginMetadataCaveat(name, plugin))
			if len(decoded.Issues) > 0 {
				out.Caveats = append(out.Caveats, fmt.Sprintf("profiler plugin %s metadata degraded: %s", name, profilerTracePluginIssueSummary(decoded.Issues)))
			}
			coverage := TraceDBCoverage{
				Family:   "builtin_modern_profiler",
				Table:    "plugin:" + name,
				Role:     "query_ready_export",
				Found:    true,
				RowsRead: 1,
			}
			if name == "ftrace-plugin" {
				authority := decodeProfilerTracePluginResult(plugin.Data)
				if authority.Disposition == profilerFtracePayloadNotStructured {
					rows, textPayload, rowErr := addStrictSystraceRowsFromBytes(plugin.Data, &seq, sink)
					if rowErr != nil {
						coverage.Error = rowErr.Error()
						out.TraceCoverage = append(out.TraceCoverage, coverage)
						return profilerContainerExtraction{}, rowErr
					}
					coverage.RowsEmitted = rows
					if textPayload {
						out.TextRows += rows
						out.TextPluginMessages++
						out.TraceCoverage = append(out.TraceCoverage, coverage)
						off += 4 + n
						continue
					}
					coverage.Skipped = "ftrace-plugin payload was neither authoritative TracePluginResult protobuf nor a complete strict legacy systrace payload"
					out.UnsupportedFtrace++
					out.TraceCoverage = append(out.TraceCoverage, coverage)
					off += 4 + n
					continue
				}

				if authority.Disposition == profilerFtracePayloadStructured {
					out.StructuredFtrace++
				} else {
					out.MalformedFtrace++
				}
				if issueSummary := profilerTracePluginIssueSummary(authority.Issues); issueSummary != "" {
					out.Caveats = append(out.Caveats, "ftrace-plugin TracePluginResult degraded: "+issueSummary)
				}
				summary, ok, summaryErr := decodeProfilerFtraceSummaryResult(authority)
				if summaryErr != nil {
					out.Caveats = append(out.Caveats, fmt.Sprintf("ftrace-plugin structured metadata parse failed: %v", summaryErr))
					coverage.Error = summaryErr.Error()
					out.UnsupportedFtrace++
				} else {
					if ok {
						out.Caveats = append(out.Caveats, profilerFtraceSummaryCaveat(summary))
						out.TraceCoverage = append(out.TraceCoverage, profilerFtraceSummaryCoverage(summary)...)
					}
					structuredRows, structuredCoverage, renderErr := renderProfilerFtraceStructuredResult(authority, &seq, sink)
					out.TraceCoverage = append(out.TraceCoverage, structuredCoverage...)
					if renderErr != nil {
						coverage.Error = renderErr.Error()
						out.UnsupportedFtrace++
						out.TraceCoverage = append(out.TraceCoverage, coverage)
						return profilerContainerExtraction{}, renderErr
					}
					coverage.RowsEmitted = structuredRows
					if structuredRows > 0 {
						out.StructuredRows += structuredRows
					}
					if ok && len(summary.Issues) > 0 || profilerFtraceCoverageHasSkipped(structuredCoverage) || ok && structuredRows == 0 && summary.DetailEventCount > 0 {
						coverage.Skipped = "structured ftrace renderer partial"
						out.UnsupportedFtrace++
					}
				}
			} else if strings.EqualFold(name, "ftrace-plugin") {
				coverage.Skipped = "non-canonical ftrace-plugin name rejected; structured routing requires exact case-sensitive name=ftrace-plugin"
				out.UnsupportedFtrace++
			} else if len(plugin.Data) == 0 {
				coverage.Skipped = "empty plugin payload"
			} else {
				rows, rowErr := addSystraceRowsFromBytes(plugin.Data, &seq, sink)
				if rowErr != nil {
					coverage.Error = rowErr.Error()
					out.TraceCoverage = append(out.TraceCoverage, coverage)
					return profilerContainerExtraction{}, rowErr
				}
				coverage.RowsEmitted = rows
				if rows > 0 {
					out.TextRows += rows
					out.TextPluginMessages++
				} else {
					coverage.Skipped = "plugin payload did not contain systrace-compatible text rows"
				}
			}
			out.TraceCoverage = append(out.TraceCoverage, coverage)
		} else {
			out.RejectedMessages++
			reason := profilerTracePluginIssueSummary(decoded.Issues)
			if reason == "" {
				reason = "plugin_message_rejected"
			}
			out.Caveats = append(out.Caveats, fmt.Sprintf("rejected ProfilerPluginData message at offset %d: %s", off, reason))
			out.TraceCoverage = append(out.TraceCoverage, profilerRejectedPluginCoverage(reason))
		}
		off += 4 + n
	}
	if remaining := limit - off; remaining > 0 && remaining < 4 {
		out.RejectedMessages++
		out.Caveats = append(out.Caveats, fmt.Sprintf("rejected truncated ProfilerPluginData length prefix at offset %d: available=%d", off, remaining))
		out.TraceCoverage = append(out.TraceCoverage, profilerContainerEnvelopeCoverage("plugin_length_prefix_truncated"))
	}
	if out.Messages == 0 {
		out.Caveats = append(out.Caveats, "official profiler header was present, but no length-prefixed ProfilerPluginData messages were readable")
	}
	if out.StructuredFtrace > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("decoded %d authoritative ftrace-plugin TracePluginResult message(s) and rendered %d structured trace row(s); unsupported or degraded members remain explicit in typed coverage", out.StructuredFtrace, out.StructuredRows))
	}
	if out.MalformedFtrace > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("classified %d ftrace-plugin payload(s) as malformed TracePluginResult; no partial structured or text rows were published", out.MalformedFtrace))
	}
	if out.TextRows > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("extracted %d systrace text row(s) from %d profiler plugin message(s)", out.TextRows, out.TextPluginMessages))
	}
	return out, nil
}

func profilerPluginMetadataCaveat(name string, plugin profilerPluginData) string {
	var parts []string
	if plugin.Status != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", plugin.Status))
	}
	timeTuplePresent := plugin.TvSecPresent || plugin.TvNsecPresent
	if !plugin.ClockIDAmbiguous && !plugin.TimeTupleAmbiguous {
		if plugin.ClockIDPresent || timeTuplePresent {
			parts = append(parts, "clock_id="+profilerPluginClockName(plugin.ClockID))
		}
		if timeTuplePresent {
			parts = append(parts, fmt.Sprintf("tv=%d.%09d", plugin.TvSec, plugin.TvNsec))
		}
	}
	if plugin.Version != "" {
		parts = append(parts, "version="+plugin.Version)
	}
	if plugin.SampleInterval != 0 {
		parts = append(parts, fmt.Sprintf("sample_interval_ms=%d", plugin.SampleInterval))
	}
	if len(plugin.Data) > 0 {
		parts = append(parts, fmt.Sprintf("payload_bytes=%d", len(plugin.Data)))
	}
	if len(parts) == 0 {
		parts = append(parts, "metadata=present")
	}
	return fmt.Sprintf("profiler plugin %s metadata: %s", name, strings.Join(parts, "; "))
}

func profilerRejectedPluginCoverage(reason string) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_profiler",
		Table:    "plugin:__rejected__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: 1,
		Skipped:  reason,
		FieldSources: map[string]string{
			"schema_profile": "ProfilerPluginData{name=1,status=2,data=3,clock_id=4,tv_sec=5,tv_nsec=6,version=7,sample_interval=8}",
		},
	}
}

func profilerContainerEnvelopeCoverage(reason string) TraceDBCoverage {
	return TraceDBCoverage{
		Family:   "builtin_modern_profiler",
		Table:    "__container_envelope__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: 1,
		Skipped:  reason,
		FieldSources: map[string]string{
			"schema_profile": "TraceFileHeader.length bounds the length-prefixed ProfilerPluginData frame sequence",
		},
	}
}

func profilerPluginClockName(id uint64) string {
	switch id {
	case 0:
		return "REALTIME"
	case 1:
		return "MONOTONIC"
	case 2:
		return "PROCESS_CPUTIME_ID"
	case 3:
		return "THREAD_CPUTIME_ID"
	case 4:
		return "MONOTONIC_RAW"
	case 5:
		return "REALTIME_COARSE"
	case 6:
		return "MONOTONIC_COARSE"
	case 7:
		return "BOOTTIME"
	case 8:
		return "REALTIME_ALARM"
	case 9:
		return "BOOTTIME_ALARM"
	case 10:
		return "SGI_CYCLE"
	case 11:
		return "TAI"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", id)
	}
}

func decodeProfilerFtraceSummary(data []byte) (profilerFtraceSummary, bool, error) {
	return decodeProfilerFtraceSummaryResult(decodeProfilerTracePluginResult(data))
}

func decodeProfilerFtraceSummaryResult(result profilerTracePluginResult) (profilerFtraceSummary, bool, error) {
	summary := profilerFtraceSummary{
		TraceClocks:       map[string]int{},
		StatsCPUs:         map[uint64]bool{},
		DetailCPUs:        map[uint64]bool{},
		EventFieldCounts:  map[int]int{},
		StartTotalsValid:  true,
		EndTotalsValid:    true,
		DetailOverwriteOK: true,
	}
	if result.Disposition == profilerFtracePayloadNotStructured {
		return summary, false, nil
	}
	if result.Disposition == profilerFtracePayloadMalformed {
		return summary, false, nil
	}
	summary.recognizedMessage = true
	for _, raw := range result.CPUStats {
		stats, err := decodeProfilerFtraceCPUStats(raw)
		if err != nil {
			summary.Issues = append(summary.Issues, "ftrace_cpu_stats_malformed_wire")
			continue
		}
		summary.StatsMessages++
		if stats.Clock != "" {
			summary.TraceClocks[stats.Clock]++
		}
		if stats.Status == 1 {
			summary.EndStats++
		} else {
			summary.StartStats++
		}
		for _, cpu := range stats.PerCPU {
			summary.StatsCPUs[cpu.CPU] = true
			if stats.Status == 1 {
				if summary.EndTotalsValid && !summary.EndTotals.add(cpu) {
					summary.EndTotalsValid = false
					summary.EndTotals = profilerFtraceCPUTotals{}
					summary.Issues = append(summary.Issues, "ftrace_cpu_stats_end_aggregate_overflow")
				}
			} else {
				if summary.StartTotalsValid && !summary.StartTotals.add(cpu) {
					summary.StartTotalsValid = false
					summary.StartTotals = profilerFtraceCPUTotals{}
					summary.Issues = append(summary.Issues, "ftrace_cpu_stats_start_aggregate_overflow")
				}
			}
		}
	}
	for _, raw := range result.CPUDetails {
		detail, err := decodeProfilerFtraceCPUDetail(raw)
		if err != nil {
			continue
		}
		summary.DetailMessages++
		summary.DetailCPUs[detail.CPU] = true
		summary.DetailEventCount += detail.EventCount
		if !detail.OverwriteValid {
			summary.DetailOverwriteOK = false
			summary.DetailOverwrite = 0
		} else if summary.DetailOverwriteOK {
			if next, ok := checkedProfilerUint64Add(summary.DetailOverwrite, detail.Overwrite); ok {
				summary.DetailOverwrite = next
			} else {
				summary.DetailOverwriteOK = false
				summary.DetailOverwrite = 0
				summary.Issues = append(summary.Issues, "ftrace_cpu_detail_overwrite_aggregate_overflow")
			}
		}
		for eventField, count := range detail.EventFieldCounts {
			summary.EventFieldCounts[eventField] += count
		}
	}
	for _, raw := range result.Symbols {
		symbol, err := decodeProfilerFtraceSymbolDetail(raw)
		if err != nil {
			summary.Issues = append(summary.Issues, "symbols_detail_malformed_wire")
			continue
		}
		summary.SymbolCount++
		if symbol.Name != "" && len(summary.SymbolExamples) < 5 {
			if symbol.Addr != 0 {
				summary.SymbolExamples = append(summary.SymbolExamples, fmt.Sprintf("0x%x=%s", symbol.Addr, symbol.Name))
			} else {
				summary.SymbolExamples = append(summary.SymbolExamples, symbol.Name)
			}
		}
	}
	for _, raw := range result.Clocks {
		clock, err := decodeProfilerFtraceClockDetail(raw)
		if err != nil {
			summary.Issues = append(summary.Issues, "clocks_detail_malformed_wire")
			continue
		}
		if label := profilerFtraceClockDetailLabel(clock); label != "" && len(summary.ClockDetails) < 8 {
			summary.ClockDetails = append(summary.ClockDetails, label)
		}
	}
	for _, raw := range result.CommDicts {
		if err := decodeProfilerFtraceCommDict(raw); err != nil {
			summary.Issues = append(summary.Issues, "comm_dict_malformed_or_ambiguous")
		}
	}
	if len(result.Versions) == 1 && traceDBSinglePhysicalLine(string(result.Versions[0]), true) {
		summary.Version = string(result.Versions[0])
	} else if len(result.Versions) == 1 {
		summary.Issues = append(summary.Issues, "trace_plugin_version_invalid")
	}
	return summary, summary.recognizedMessage, nil
}

func decodeProfilerFtraceCPUStats(data []byte) (profilerFtraceCPUStats, error) {
	var stats profilerFtraceCPUStats
	statusCount, clockCount := 0, 0
	statusWrongWire, clockWrongWire, perCPUWrongWire := false, false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			statusCount++
			if wire == 0 {
				stats.Status = v
			} else {
				statusWrongWire = true
			}
		case 2:
			if wire != 2 {
				perCPUWrongWire = true
				return nil
			}
			perCPU, err := decodeProfilerFtracePerCPUStats(raw)
			if err != nil {
				return err
			}
			stats.HasStats = true
			stats.PerCPU = append(stats.PerCPU, perCPU)
		case 3:
			clockCount++
			if wire == 2 {
				stats.Clock = string(raw)
			} else {
				clockWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return stats, err
	}
	if statusCount > 1 || statusWrongWire || stats.Status > 1 {
		return stats, fmt.Errorf("invalid FtraceCpuStatsMsg status field")
	}
	if perCPUWrongWire {
		return stats, fmt.Errorf("wrong-wire FtraceCpuStatsMsg per_cpu_stats field")
	}
	if clockCount > 1 || clockWrongWire || clockCount == 1 && !traceDBSingleToken(stats.Clock) {
		return stats, fmt.Errorf("invalid FtraceCpuStatsMsg trace_clock field")
	}
	return stats, err
}

func decodeProfilerFtracePerCPUStats(data []byte) (profilerFtracePerCPUStats, error) {
	var stats profilerFtracePerCPUStats
	var counts [10]int
	var wrongWire [10]bool
	var rawValues [10]uint64
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		if field < 1 || field > 9 {
			return nil
		}
		expectedWire := 0
		if field == 6 || field == 7 {
			expectedWire = 1
		}
		counts[field]++
		if wire != expectedWire {
			wrongWire[field] = true
			return nil
		}
		rawValues[field] = v
		switch field {
		case 1:
			stats.CPU = v
		case 2:
			stats.Entries = v
		case 3:
			stats.Overrun = v
		case 4:
			stats.CommitOverrun = v
		case 5:
			stats.Bytes = v
		case 8:
			stats.DroppedEvents = v
		case 9:
			stats.ReadEvents = v
		}
		_ = raw
		return nil
	})
	if err != nil {
		return stats, err
	}
	for field := 1; field <= 9; field++ {
		if counts[field] > 1 || wrongWire[field] {
			return stats, fmt.Errorf("invalid PerCpuStatsMsg field %d", field)
		}
	}
	if stats.CPU > uint64(maxTraceDBCPUIndex) {
		return stats, fmt.Errorf("out-of-range PerCpuStatsMsg cpu field")
	}
	for _, field := range []int{6, 7} {
		if counts[field] == 1 {
			value := math.Float64frombits(rawValues[field])
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return stats, fmt.Errorf("non-finite PerCpuStatsMsg field %d", field)
			}
		}
	}
	return stats, err
}

func decodeProfilerFtraceCPUDetail(data []byte) (profilerFtraceCPUDetail, error) {
	detail := profilerFtraceCPUDetail{EventFieldCounts: map[int]int{}, OverwriteValid: true}
	cpuCount := 0
	cpuWrongWire := false
	overwriteCount := 0
	overwriteWrongWire := false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			cpuCount++
			if wire == 0 {
				detail.CPU = v
			} else {
				cpuWrongWire = true
			}
		case 2:
			if wire == 2 {
				fields, err := decodeProfilerFtraceEventFields(raw)
				if err != nil {
					return nil
				}
				detail.EventCount++
				for _, eventField := range fields {
					detail.EventFieldCounts[eventField]++
				}
			}
		case 3:
			overwriteCount++
			if wire == 0 {
				detail.Overwrite = v
			} else {
				overwriteWrongWire = true
			}
		}
		_ = raw
		return nil
	})
	if err != nil {
		return detail, err
	}
	if cpuCount > 1 {
		return detail, fmt.Errorf("duplicate FtraceCpuDetailMsg cpu field")
	}
	if cpuWrongWire {
		return detail, fmt.Errorf("wrong-wire FtraceCpuDetailMsg cpu field")
	}
	if overwriteCount > 1 || overwriteWrongWire {
		detail.Overwrite = 0
		detail.OverwriteValid = false
	}
	if detail.CPU > uint64(maxTraceDBCPUIndex) {
		return detail, fmt.Errorf("out-of-range FtraceCpuDetailMsg cpu field")
	}
	return detail, err
}

func decodeProfilerFtraceEventFields(data []byte) ([]int, error) {
	record, err := decodeProfilerFtraceEventRecord(0, data)
	if err != nil {
		return nil, err
	}
	if len(record.EnvelopeDegradations) > 0 {
		return nil, fmt.Errorf("invalid FtraceEvent envelope: %s", strings.Join(record.EnvelopeDegradations, ","))
	}
	return []int{record.Field}, nil
}

func decodeProfilerFtraceSymbolDetail(data []byte) (profilerFtraceSymbolDetail, error) {
	var symbol profilerFtraceSymbolDetail
	addrCount, nameCount := 0, 0
	addrWrongWire, nameWrongWire := false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			addrCount++
			if wire == 0 {
				symbol.Addr = v
			} else {
				addrWrongWire = true
			}
		case 2:
			nameCount++
			if wire == 2 {
				symbol.Name = string(raw)
			} else {
				nameWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return symbol, err
	}
	if addrCount > 1 || addrWrongWire || nameCount > 1 || nameWrongWire ||
		nameCount == 1 && !traceDBSinglePhysicalLine(symbol.Name, true) {
		return symbol, fmt.Errorf("invalid SymbolsDetailMsg field")
	}
	return symbol, err
}

func decodeProfilerFtraceClockDetail(data []byte) (profilerFtraceClockDetail, error) {
	var clock profilerFtraceClockDetail
	idCount, timeCount, resCount := 0, 0, 0
	idWrongWire, timeWrongWire, resWrongWire := false, false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			idCount++
			if wire == 0 {
				clock.ID = v
			} else {
				idWrongWire = true
			}
		case 2:
			timeCount++
			if wire == 2 {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(raw)
				if err != nil {
					return err
				}
				clock.TimeSec, clock.TimeNsec, clock.HasTime = sec, nsec, true
			} else {
				timeWrongWire = true
			}
		case 3:
			resCount++
			if wire == 2 {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(raw)
				if err != nil {
					return err
				}
				clock.ResSec, clock.ResNsec, clock.HasRes = sec, nsec, true
			} else {
				resWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return clock, err
	}
	if idCount > 1 || idWrongWire || clock.ID > 6 || timeCount > 1 || timeWrongWire || resCount > 1 || resWrongWire {
		return clock, fmt.Errorf("invalid ClockDetailMsg field")
	}
	return clock, err
}

func decodeProfilerFtraceTimeSpec(data []byte) (uint64, uint64, error) {
	var sec, nsec uint64
	secCount, nsecCount := 0, 0
	secWrongWire, nsecWrongWire := false, false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			secCount++
			if wire == 0 {
				sec = v
			} else {
				secWrongWire = true
			}
		case 2:
			nsecCount++
			if wire == 0 {
				nsec = v
			} else {
				nsecWrongWire = true
			}
		}
		_ = raw
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	if secCount > 1 || secWrongWire || sec > uint64(^uint32(0)) ||
		nsecCount > 1 || nsecWrongWire || nsec > uint64(^uint32(0)) || nsec >= 1e9 {
		return 0, 0, fmt.Errorf("invalid ClockDetailMsg TimeSpec field")
	}
	return sec, nsec, err
}

func decodeProfilerFtraceCommDict(data []byte) error {
	tidCount, commCount := 0, 0
	tidWrongWire, commWrongWire := false, false
	var tid uint64
	var comm string
	err := walkProtoFields(data, func(field int, wire int, raw []byte, value uint64) error {
		switch field {
		case 1:
			tidCount++
			if wire == 0 {
				tid = value
			} else {
				tidWrongWire = true
			}
		case 2:
			commCount++
			if wire == 2 {
				comm = string(raw)
			} else {
				commWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if tidCount > 1 || tidWrongWire || tidCount == 1 && uint64(int64(int32(tid))) != tid ||
		commCount > 1 || commWrongWire || commCount == 1 && !traceDBSinglePhysicalLine(comm, true) {
		return fmt.Errorf("invalid CommDictMsg field")
	}
	return nil
}

func (totals *profilerFtraceCPUTotals) add(stats profilerFtracePerCPUStats) bool {
	next := *totals
	var ok bool
	if next.Entries, ok = checkedProfilerUint64Add(next.Entries, stats.Entries); !ok {
		return false
	}
	if next.Overrun, ok = checkedProfilerUint64Add(next.Overrun, stats.Overrun); !ok {
		return false
	}
	if next.CommitOverrun, ok = checkedProfilerUint64Add(next.CommitOverrun, stats.CommitOverrun); !ok {
		return false
	}
	if next.Bytes, ok = checkedProfilerUint64Add(next.Bytes, stats.Bytes); !ok {
		return false
	}
	if next.DroppedEvents, ok = checkedProfilerUint64Add(next.DroppedEvents, stats.DroppedEvents); !ok {
		return false
	}
	if next.ReadEvents, ok = checkedProfilerUint64Add(next.ReadEvents, stats.ReadEvents); !ok {
		return false
	}
	*totals = next
	return true
}

func checkedProfilerUint64Add(left, right uint64) (uint64, bool) {
	if left > ^uint64(0)-right {
		return 0, false
	}
	return left + right, true
}

func profilerFtraceSummaryCaveat(summary profilerFtraceSummary) string {
	var parts []string
	if summary.Version != "" {
		parts = append(parts, "version="+summary.Version)
	}
	parts = append(parts, fmt.Sprintf("stats_messages=%d", summary.StatsMessages))
	if summary.StartStats != 0 || summary.EndStats != 0 {
		parts = append(parts, fmt.Sprintf("stats_start=%d", summary.StartStats))
		parts = append(parts, fmt.Sprintf("stats_end=%d", summary.EndStats))
	}
	if len(summary.TraceClocks) > 0 {
		parts = append(parts, "trace_clock="+joinStringCounts(summary.TraceClocks))
	}
	if len(summary.StatsCPUs) > 0 {
		totals := summary.StartTotals
		totalsValid := summary.StartTotalsValid
		label := "observed"
		if summary.EndStats > 0 {
			totals = summary.EndTotals
			totalsValid = summary.EndTotalsValid
			label = "end"
		}
		parts = append(parts, fmt.Sprintf("stats_cpus=%d", len(summary.StatsCPUs)))
		if totalsValid {
			parts = append(parts, fmt.Sprintf("%s_entries=%d", label, totals.Entries))
			parts = append(parts, fmt.Sprintf("%s_dropped=%d", label, totals.DroppedEvents))
			parts = append(parts, fmt.Sprintf("%s_overrun=%d", label, totals.Overrun))
			parts = append(parts, fmt.Sprintf("%s_commit_overrun=%d", label, totals.CommitOverrun))
			parts = append(parts, fmt.Sprintf("%s_read=%d", label, totals.ReadEvents))
			parts = append(parts, fmt.Sprintf("%s_bytes=%d", label, totals.Bytes))
		}
	}
	if summary.DetailMessages > 0 {
		parts = append(parts, fmt.Sprintf("detail_messages=%d", summary.DetailMessages))
		parts = append(parts, fmt.Sprintf("detail_cpus=%d", len(summary.DetailCPUs)))
		parts = append(parts, fmt.Sprintf("structured_event_records=%d", summary.DetailEventCount))
		if summary.DetailOverwriteOK {
			parts = append(parts, fmt.Sprintf("detail_overwrite=%d", summary.DetailOverwrite))
		}
	}
	if len(summary.EventFieldCounts) > 0 {
		parts = append(parts, "event_families="+joinStringCounts(profilerFtraceEventFamilyCounts(summary.EventFieldCounts)))
		parts = append(parts, "event_names="+joinStringCounts(profilerFtraceEventNameCounts(summary.EventFieldCounts)))
	}
	if summary.SymbolCount > 0 {
		parts = append(parts, fmt.Sprintf("symbols=%d", summary.SymbolCount))
		if len(summary.SymbolExamples) > 0 {
			parts = append(parts, "symbol_examples="+strings.Join(summary.SymbolExamples, ","))
		}
	}
	if len(summary.ClockDetails) > 0 {
		parts = append(parts, "clock_details="+strings.Join(summary.ClockDetails, ","))
	}
	if issueSummary := profilerTracePluginIssueSummary(summary.Issues); issueSummary != "" {
		parts = append(parts, "degraded="+issueSummary)
	}
	return "ftrace-plugin structured metadata: " + strings.Join(parts, "; ")
}

func profilerFtraceSummaryCoverage(summary profilerFtraceSummary) []TraceDBCoverage {
	if len(summary.Issues) == 0 {
		return nil
	}
	return []TraceDBCoverage{{
		Family:   "builtin_modern_ftrace:trace_plugin_metadata",
		Table:    "__trace_plugin_metadata__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: len(summary.Issues),
		Skipped:  profilerTracePluginIssueSummary(summary.Issues),
		FieldSources: map[string]string{
			"schema_profile": "TracePluginResult CPU stats/detail, symbols, clocks, and version metadata",
		},
	}}
}

func profilerFtraceEventFamilyCounts(counts map[int]int) map[string]int {
	out := map[string]int{}
	for field, count := range counts {
		desc, ok := profilerFtraceEventDescriptors[field]
		if !ok {
			out["unknown"] += count
			continue
		}
		out[desc.Family] += count
	}
	return out
}

func profilerFtraceEventNameCounts(counts map[int]int) map[string]int {
	out := map[string]int{}
	for field, count := range counts {
		desc, ok := profilerFtraceEventDescriptors[field]
		if !ok {
			out[fmt.Sprintf("event_field_%d", field)] += count
			continue
		}
		out[desc.Name] += count
	}
	return out
}

func joinStringCounts(values map[string]int) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if values[key] == 1 {
			parts = append(parts, key)
		} else {
			parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
		}
	}
	return strings.Join(parts, ",")
}

func profilerFtraceClockDetailLabel(clock profilerFtraceClockDetail) string {
	name := profilerFtraceClockName(clock.ID)
	var parts []string
	if clock.HasTime {
		parts = append(parts, fmt.Sprintf("time=%d.%09d", clock.TimeSec, clock.TimeNsec))
	}
	if clock.HasRes {
		parts = append(parts, fmt.Sprintf("res=%d.%09d", clock.ResSec, clock.ResNsec))
	}
	if len(parts) == 0 {
		return name
	}
	return name + "(" + strings.Join(parts, "/") + ")"
}

func profilerFtraceClockName(id uint64) string {
	switch id {
	case 1:
		return "BOOTTIME"
	case 2:
		return "REALTIME"
	case 3:
		return "REALTIME_COARSE"
	case 4:
		return "MONOTONIC"
	case 5:
		return "MONOTONIC_COARSE"
	case 6:
		return "MONOTONIC_RAW"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", id)
	}
}

func extractProfilerSessionPackage(ctx context.Context, path string, sink *traceDBRowSink) (profilerContainerExtraction, error) {
	f, err := os.Open(path)
	if err != nil {
		return profilerContainerExtraction{}, err
	}
	defer f.Close()
	if _, ok, err := profilerSessionJSONMarkerOffset(path, 64*1024); err != nil {
		return profilerContainerExtraction{}, err
	} else if !ok {
		return profilerContainerExtraction{}, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return profilerContainerExtraction{}, err
	}
	out := profilerContainerExtraction{
		Detected:       true,
		Kind:           "openharmony_profiler_session_package",
		PluginMessages: map[string]int{},
		Caveats: []string{
			"OpenHarmony profiler session package marker SessionJSON- detected; using section/text extraction instead of legacy binary hitrace segment parsing",
		},
	}
	seq := 0
	coverage := TraceDBCoverage{
		Family: "builtin_modern_profiler",
		Table:  "session:SessionJSON",
		Role:   "query_ready_export",
		Found:  true,
	}
	reader := bufio.NewReaderSize(f, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return profilerContainerExtraction{}, err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			coverage.RowsRead++
			lineRows, rowErr := addSystraceRowsFromBytes(line, &seq, sink)
			if rowErr != nil {
				coverage.Error = rowErr.Error()
				out.TraceCoverage = append(out.TraceCoverage, coverage)
				return profilerContainerExtraction{}, rowErr
			}
			coverage.RowsEmitted += lineRows
			out.TextRows += lineRows
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			coverage.Error = readErr.Error()
			out.TraceCoverage = append(out.TraceCoverage, coverage)
			return profilerContainerExtraction{}, readErr
		}
	}
	if out.TextRows > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("extracted %d systrace text row(s) from profiler session package payload", out.TextRows))
	} else {
		coverage.Skipped = "session package did not contain directly renderable systrace text rows"
		out.Caveats = append(out.Caveats, "session package did not contain directly renderable systrace text rows; attach extracted sidecars or export ftrace/bytrace text with the official profiler tooling")
	}
	out.TraceCoverage = append(out.TraceCoverage, coverage)
	return out, nil
}

func profilerSessionJSONMarkerOffset(path string, maxProbe int64) (int64, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()
	if maxProbe <= 0 {
		maxProbe = 64 * 1024
	}
	probe := make([]byte, maxProbe)
	n, err := f.Read(probe)
	if err != nil && err != io.EOF {
		return 0, false, err
	}
	idx := bytes.Index(probe[:n], []byte(profilerSessionJSONTag))
	if idx < 0 {
		return 0, false, nil
	}
	return int64(idx), true, nil
}

func readProfilerTraceHeaderAtPath(path string, off int64, fileSize int64) (profilerTraceHeader, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return profilerTraceHeader{}, false, err
	}
	defer f.Close()
	header, ok := readProfilerTraceHeaderAt(f, off, fileSize)
	return header, ok, nil
}

func readProfilerTraceHeaderAt(r io.ReaderAt, off int64, fileSize int64) (profilerTraceHeader, bool) {
	if off < 0 || off+profilerTraceHeaderSize > fileSize {
		return profilerTraceHeader{}, false
	}
	header := make([]byte, profilerTraceHeaderSize)
	if _, err := r.ReadAt(header, off); err != nil {
		return profilerTraceHeader{}, false
	}
	if binary.LittleEndian.Uint64(header[0:8]) != profilerTraceHeaderMagic {
		return profilerTraceHeader{}, false
	}
	length := binary.LittleEndian.Uint64(header[8:16])
	return profilerTraceHeader{
		Offset:        off,
		Length:        length,
		Version:       binary.LittleEndian.Uint32(header[16:20]),
		Segments:      binary.LittleEndian.Uint32(header[20:24]),
		DataType:      binary.LittleEndian.Uint32(header[56:60]),
		PluginName:    cString(header[profilerPluginNameOffset : profilerPluginNameOffset+profilerPluginNameSize]),
		PluginVersion: cString(header[profilerPluginVersionOffset : profilerPluginVersionOffset+profilerPluginVersionSize]),
	}, true
}

func parseProfilerPluginData(data []byte) profilerPluginDataDecode {
	var decoded profilerPluginDataDecode
	var counts [9]int
	var valid [9]bool
	var uintValues [9]uint64
	var byteValues [9][]byte

	err := walkProtoFields(data, func(field int, wire int, raw []byte, value uint64) error {
		if field < 1 || field > 8 {
			return nil
		}
		counts[field]++
		expectedWire := 0
		if field == 1 || field == 3 || field == 7 {
			expectedWire = 2
		}
		if wire != expectedWire {
			decoded.Issues = append(decoded.Issues, fmt.Sprintf("plugin_field%d_wrong_wire", field))
			return nil
		}
		if counts[field] > 1 {
			return nil
		}
		valid[field] = true
		if wire == 2 {
			byteValues[field] = raw
		} else {
			uintValues[field] = value
		}
		return nil
	})
	if err != nil {
		decoded.Issues = append(decoded.Issues, "plugin_message_malformed_wire")
	}
	for field := 1; field <= 8; field++ {
		if counts[field] > 1 {
			decoded.Issues = append(decoded.Issues, fmt.Sprintf("plugin_field%d_duplicate", field))
			valid[field] = false
		}
	}

	hardRejected := err != nil
	if counts[1] != 1 || !valid[1] {
		hardRejected = true
		if counts[1] == 0 {
			decoded.Issues = append(decoded.Issues, "plugin_name_missing")
		}
	} else if name := string(byteValues[1]); !traceDBSingleToken(name) {
		hardRejected = true
		valid[1] = false
		decoded.Issues = append(decoded.Issues, "plugin_name_invalid")
	} else {
		decoded.Plugin.Name = name
	}
	if counts[3] > 1 || counts[3] == 1 && !valid[3] {
		hardRejected = true
	} else if counts[3] == 1 {
		decoded.Plugin.Data = byteValues[3]
	}

	if counts[2] == 1 && valid[2] {
		if uintValues[2] > uint64(^uint32(0)) {
			valid[2] = false
			decoded.Issues = append(decoded.Issues, "plugin_status_out_of_range")
		} else {
			decoded.Plugin.Status = uint32(uintValues[2])
		}
	}
	if counts[4] == 1 && valid[4] {
		if uintValues[4] > 11 {
			valid[4] = false
			decoded.Issues = append(decoded.Issues, "plugin_clock_id_out_of_range")
		} else {
			decoded.Plugin.ClockID = uintValues[4]
			decoded.Plugin.ClockIDPresent = true
		}
	}
	if counts[5] == 1 && valid[5] {
		decoded.Plugin.TvSec = uintValues[5]
		decoded.Plugin.TvSecPresent = true
	}
	if counts[6] == 1 && valid[6] {
		if uintValues[6] >= 1e9 {
			valid[6] = false
			decoded.Issues = append(decoded.Issues, "plugin_tv_nsec_out_of_range")
		} else {
			decoded.Plugin.TvNsec = uintValues[6]
			decoded.Plugin.TvNsecPresent = true
		}
	}
	if counts[7] == 1 && valid[7] {
		version := string(byteValues[7])
		if !traceDBSinglePhysicalLine(version, true) {
			valid[7] = false
			decoded.Issues = append(decoded.Issues, "plugin_version_invalid")
		} else {
			decoded.Plugin.Version = version
		}
	}
	if counts[8] == 1 && valid[8] {
		if uintValues[8] > uint64(^uint32(0)) {
			valid[8] = false
			decoded.Issues = append(decoded.Issues, "plugin_sample_interval_out_of_range")
		} else {
			decoded.Plugin.SampleInterval = uint32(uintValues[8])
		}
	}
	decoded.Plugin.ClockIDAmbiguous = counts[4] > 0 && (counts[4] != 1 || !valid[4])
	decoded.Plugin.TimeTupleAmbiguous = counts[5] > 0 && (counts[5] != 1 || !valid[5]) ||
		counts[6] > 0 && (counts[6] != 1 || !valid[6])

	if hardRejected {
		decoded.Plugin = profilerPluginData{}
		return decoded
	}
	decoded.Accepted = true
	return decoded
}

func addSystraceRowsFromBytes(data []byte, seq *int, sink *traceDBRowSink) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	rows := 0
	for start := 0; start < len(data); {
		end := start
		for end < len(data) && data[end] != '\n' && data[end] != 0 {
			end++
		}
		part := bytes.TrimSpace(data[start:end])
		if end < len(data) {
			start = end + 1
		} else {
			start = len(data)
		}
		if len(part) == 0 || len(part) > maxProfilerTextLineBytes {
			continue
		}
		line := string(part)
		if line == "" {
			continue
		}
		ts, ok := systraceLineTimestampNS(line)
		if !ok {
			continue
		}
		if sink == nil {
			return rows, fmt.Errorf("systrace row sink is nil")
		}
		if err := sink.add(renderedRow{tsNS: ts, seq: *seq, line: line}); err != nil {
			return rows, err
		}
		(*seq)++
		rows++
	}
	return rows, nil
}

func systraceLineTimestampNS(line string) (uint64, bool) {
	for searchFrom := 0; searchFrom < len(line); {
		colon := strings.Index(line[searchFrom:], ":")
		if colon < 0 {
			return 0, false
		}
		colon += searchFrom
		if colon > 0 {
			prefix := line[:colon]
			fields := strings.Fields(prefix)
			if len(fields) > 0 {
				if ts, err := strconv.ParseFloat(fields[len(fields)-1], 64); err == nil && ts >= 0 {
					rest := strings.TrimSpace(line[colon+1:])
					nextColon := strings.Index(rest, ":")
					if nextColon > 0 && isTraceEventName(rest[:nextColon]) {
						return uint64(ts * 1e9), true
					}
				}
			}
		}
		searchFrom = colon + 1
	}
	return 0, false
}

func isTraceEventName(event string) bool {
	if event == "" || len(event) > 128 {
		return false
	}
	for _, r := range event {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func readProtoKey(data []byte, off *int) (field int, wire int, ok bool) {
	key, ok := readProtoVarint(data, off)
	if !ok || key == 0 {
		return 0, 0, false
	}
	return int(key >> 3), int(key & 0x7), true
}

func readProtoVarint(data []byte, off *int) (uint64, bool) {
	var out uint64
	for shift := uint(0); shift < 64 && *off < len(data); shift += 7 {
		b := data[*off]
		*off++
		out |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return out, true
		}
	}
	return 0, false
}

func readProtoBytes(data []byte, off *int) ([]byte, bool) {
	n, ok := readProtoVarint(data, off)
	if !ok || n > uint64(len(data)-*off) {
		return nil, false
	}
	start := *off
	*off += int(n)
	return data[start:*off], true
}

func readProtoString(data []byte, off *int) (string, bool) {
	b, ok := readProtoBytes(data, off)
	if !ok {
		return "", false
	}
	return string(b), true
}

func skipProtoField(data []byte, off *int, wire int) bool {
	switch wire {
	case 0:
		_, ok := readProtoVarint(data, off)
		return ok
	case 1:
		if *off+8 > len(data) {
			return false
		}
		*off += 8
		return true
	case 2:
		n, ok := readProtoVarint(data, off)
		if !ok || n > uint64(len(data)-*off) {
			return false
		}
		*off += int(n)
		return true
	case 5:
		if *off+4 > len(data) {
			return false
		}
		*off += 4
		return true
	default:
		return false
	}
}

func errorsIsEOF(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
