package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
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
	Length        int64
	Version       uint32
	Segments      uint32
	DataType      uint32
	PluginName    string
	PluginVersion string
}

type profilerPluginData struct {
	Name           string
	Status         uint32
	Data           []byte
	ClockID        uint64
	TvSec          uint64
	TvNsec         uint64
	Version        string
	SampleInterval uint32
}

type profilerContainerExtraction struct {
	Detected           bool
	Kind               string
	Messages           int
	PluginMessages     map[string]int
	StructuredFtrace   int
	UnsupportedFtrace  int
	TextPluginMessages int
	TextRows           int
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
	DetailMessages    int
	DetailCPUs        map[uint64]bool
	DetailEventCount  int
	DetailOverwrite   uint64
	SymbolCount       int
	SymbolExamples    []string
	ClockDetails      []string
	EventFieldCounts  map[int]int
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
	2003: {Field: 2003, Family: "cpu", Name: "cpu_frequency"},
	2004: {Field: 2004, Family: "cpu", Name: "cpu_frequency_limits"},
	2005: {Field: 2005, Family: "cpu", Name: "cpu_idle"},
	2417: {Field: 2417, Family: "sched", Name: "sched_switch"},
	2420: {Field: 2420, Family: "sched", Name: "sched_wakeup"},
	2421: {Field: 2421, Family: "sched", Name: "sched_wakeup_new"},
	2422: {Field: 2422, Family: "sched", Name: "sched_waking"},
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
		UnknownEventCount:  extracted.UnsupportedFtrace,
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
			Caveats:   []string{"generated from OpenHarmony profiler/session text trace payloads"},
		}}, result.Artifacts...)
		result.TraceDecisions = append(result.TraceDecisions,
			traceProviderSuccess(
				newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameBuiltinModern), opts, opts.InputPath, output),
				Artifact{Type: ArtifactSystrace, Path: output},
			),
		)
	} else if len(result.Artifacts) == 0 {
		caveat := "OpenHarmony profiler/session container was detected, but no renderable systrace text rows or sidecar artifacts were found"
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
	limit := header.Length
	if limit <= profilerTraceHeaderSize || limit > inputSize {
		limit = inputSize
	}
	out := profilerContainerExtraction{
		Detected:       true,
		Kind:           "openharmony_profiler_trace_file",
		PluginMessages: map[string]int{},
		Caveats: []string{
			fmt.Sprintf("OpenHarmony profiler TraceFileHeader detected: data_type=%d version=0x%x segments=%d length=%d", header.DataType, header.Version, header.Segments, header.Length),
		},
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
		if n <= 0 || off+4+n > limit {
			break
		}
		msg := make([]byte, n)
		if _, err := f.ReadAt(msg, off+4); err != nil {
			return profilerContainerExtraction{}, fmt.Errorf("read profiler message at %d: %w", off+4, err)
		}
		out.Messages++
		if plugin, ok := parseProfilerPluginData(msg); ok {
			name := firstNonEmpty(plugin.Name, "unknown-plugin")
			out.PluginMessages[name]++
			out.Caveats = append(out.Caveats, profilerPluginMetadataCaveat(name, plugin))
			coverage := TraceDBCoverage{
				Family:   "builtin_modern_profiler",
				Table:    "plugin:" + name,
				Role:     "query_ready_export",
				Found:    true,
				RowsRead: 1,
			}
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
			} else if strings.EqualFold(name, "ftrace-plugin") {
				out.StructuredFtrace++
				summary, ok, summaryErr := decodeProfilerFtraceSummary(plugin.Data)
				if summaryErr != nil {
					out.Caveats = append(out.Caveats, fmt.Sprintf("ftrace-plugin structured metadata parse failed: %v", summaryErr))
					coverage.Error = summaryErr.Error()
					out.UnsupportedFtrace++
				} else if ok {
					out.Caveats = append(out.Caveats, profilerFtraceSummaryCaveat(summary))
					structuredRows, structuredCoverage, renderErr := renderProfilerFtraceStructuredRows(plugin.Data, &seq, sink)
					out.TraceCoverage = append(out.TraceCoverage, structuredCoverage...)
					if renderErr != nil {
						coverage.Error = renderErr.Error()
						out.UnsupportedFtrace++
						out.TraceCoverage = append(out.TraceCoverage, coverage)
						return profilerContainerExtraction{}, renderErr
					}
					coverage.RowsEmitted = structuredRows
					if structuredRows > 0 {
						out.TextRows += structuredRows
					}
					if profilerFtraceCoverageHasSkipped(structuredCoverage) || structuredRows == 0 && summary.DetailEventCount > 0 {
						coverage.Skipped = "structured ftrace renderer partial"
						out.UnsupportedFtrace++
					}
				} else {
					coverage.Skipped = "ftrace-plugin payload was not recognized as text systrace or supported structured ftrace"
					out.UnsupportedFtrace++
				}
			} else if len(plugin.Data) == 0 {
				coverage.Skipped = "empty plugin payload"
			} else {
				coverage.Skipped = "plugin payload did not contain systrace-compatible text rows"
			}
			out.TraceCoverage = append(out.TraceCoverage, coverage)
		}
		off += 4 + n
	}
	if out.Messages == 0 {
		out.Caveats = append(out.Caveats, "official profiler header was present, but no length-prefixed ProfilerPluginData messages were readable")
	}
	if out.StructuredFtrace > 0 {
		out.Caveats = append(out.Caveats, fmt.Sprintf("detected %d ftrace-plugin protobuf message(s); Codrax currently renders embedded text trace payloads and preserves sidecars, but does not inline the full generated OpenHarmony TracePluginResult formatter matrix", out.StructuredFtrace))
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
	if plugin.ClockID != 0 {
		parts = append(parts, "clock_id="+profilerPluginClockName(plugin.ClockID))
	}
	if plugin.TvSec != 0 || plugin.TvNsec != 0 {
		parts = append(parts, fmt.Sprintf("tv=%d.%09d", plugin.TvSec, plugin.TvNsec))
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
	summary := profilerFtraceSummary{
		TraceClocks:      map[string]int{},
		StatsCPUs:        map[uint64]bool{},
		DetailCPUs:       map[uint64]bool{},
		EventFieldCounts: map[int]int{},
	}
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire != 2 {
				return nil
			}
			stats, err := decodeProfilerFtraceCPUStats(raw)
			if err != nil {
				return err
			}
			summary.recognizedMessage = true
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
					summary.EndTotals.add(cpu)
				} else {
					summary.StartTotals.add(cpu)
				}
			}
		case 2:
			if wire != 2 {
				return nil
			}
			detail, err := decodeProfilerFtraceCPUDetail(raw)
			if err != nil {
				return err
			}
			summary.recognizedMessage = true
			summary.DetailMessages++
			summary.DetailCPUs[detail.CPU] = true
			summary.DetailEventCount += detail.EventCount
			summary.DetailOverwrite += detail.Overwrite
			for eventField, count := range detail.EventFieldCounts {
				summary.EventFieldCounts[eventField] += count
			}
		case 5:
			if wire != 2 {
				return nil
			}
			symbol, err := decodeProfilerFtraceSymbolDetail(raw)
			if err != nil {
				return err
			}
			summary.recognizedMessage = true
			summary.SymbolCount++
			if symbol.Name != "" && len(summary.SymbolExamples) < 5 {
				if symbol.Addr != 0 {
					summary.SymbolExamples = append(summary.SymbolExamples, fmt.Sprintf("0x%x=%s", symbol.Addr, symbol.Name))
				} else {
					summary.SymbolExamples = append(summary.SymbolExamples, symbol.Name)
				}
			}
		case 6:
			if wire != 2 {
				return nil
			}
			clock, err := decodeProfilerFtraceClockDetail(raw)
			if err != nil {
				return err
			}
			summary.recognizedMessage = true
			if label := profilerFtraceClockDetailLabel(clock); label != "" && len(summary.ClockDetails) < 8 {
				summary.ClockDetails = append(summary.ClockDetails, label)
			}
		case 7:
			if wire == 2 {
				summary.recognizedMessage = true
				summary.Version = string(raw)
			}
		}
		_ = v
		return nil
	})
	return summary, summary.recognizedMessage, err
}

func decodeProfilerFtraceCPUStats(data []byte) (profilerFtraceCPUStats, error) {
	var stats profilerFtraceCPUStats
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				stats.Status = v
			}
		case 2:
			if wire != 2 {
				return nil
			}
			perCPU, err := decodeProfilerFtracePerCPUStats(raw)
			if err != nil {
				return err
			}
			stats.HasStats = true
			stats.PerCPU = append(stats.PerCPU, perCPU)
		case 3:
			if wire == 2 {
				stats.Clock = string(raw)
			}
		}
		return nil
	})
	return stats, err
}

func decodeProfilerFtracePerCPUStats(data []byte) (profilerFtracePerCPUStats, error) {
	var stats profilerFtracePerCPUStats
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				stats.CPU = v
			}
		case 2:
			if wire == 0 {
				stats.Entries = v
			}
		case 3:
			if wire == 0 {
				stats.Overrun = v
			}
		case 4:
			if wire == 0 {
				stats.CommitOverrun = v
			}
		case 5:
			if wire == 0 {
				stats.Bytes = v
			}
		case 8:
			if wire == 0 {
				stats.DroppedEvents = v
			}
		case 9:
			if wire == 0 {
				stats.ReadEvents = v
			}
		}
		_ = raw
		return nil
	})
	return stats, err
}

func decodeProfilerFtraceCPUDetail(data []byte) (profilerFtraceCPUDetail, error) {
	detail := profilerFtraceCPUDetail{EventFieldCounts: map[int]int{}}
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				detail.CPU = v
			}
		case 2:
			if wire == 2 {
				detail.EventCount++
				fields, err := decodeProfilerFtraceEventFields(raw)
				if err != nil {
					return err
				}
				for _, eventField := range fields {
					detail.EventFieldCounts[eventField]++
				}
			}
		case 3:
			if wire == 0 {
				detail.Overwrite = v
			}
		}
		_ = raw
		return nil
	})
	return detail, err
}

func decodeProfilerFtraceEventFields(data []byte) ([]int, error) {
	var fields []int
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1, 2, 3, 50:
			return nil
		default:
			if field >= 100 && wire == 2 {
				fields = append(fields, field)
			}
		}
		_ = raw
		_ = v
		return nil
	})
	return fields, err
}

func decodeProfilerFtraceSymbolDetail(data []byte) (profilerFtraceSymbolDetail, error) {
	var symbol profilerFtraceSymbolDetail
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				symbol.Addr = v
			}
		case 2:
			if wire == 2 {
				symbol.Name = string(raw)
			}
		}
		return nil
	})
	return symbol, err
}

func decodeProfilerFtraceClockDetail(data []byte) (profilerFtraceClockDetail, error) {
	var clock profilerFtraceClockDetail
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				clock.ID = v
			}
		case 2:
			if wire == 2 {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(raw)
				if err != nil {
					return err
				}
				clock.TimeSec, clock.TimeNsec, clock.HasTime = sec, nsec, true
			}
		case 3:
			if wire == 2 {
				sec, nsec, err := decodeProfilerFtraceTimeSpec(raw)
				if err != nil {
					return err
				}
				clock.ResSec, clock.ResNsec, clock.HasRes = sec, nsec, true
			}
		}
		return nil
	})
	return clock, err
}

func decodeProfilerFtraceTimeSpec(data []byte) (uint64, uint64, error) {
	var sec, nsec uint64
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				sec = v
			}
		case 2:
			if wire == 0 {
				nsec = v
			}
		}
		_ = raw
		return nil
	})
	return sec, nsec, err
}

func (totals *profilerFtraceCPUTotals) add(stats profilerFtracePerCPUStats) {
	totals.Entries += stats.Entries
	totals.Overrun += stats.Overrun
	totals.CommitOverrun += stats.CommitOverrun
	totals.Bytes += stats.Bytes
	totals.DroppedEvents += stats.DroppedEvents
	totals.ReadEvents += stats.ReadEvents
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
		label := "observed"
		if summary.EndStats > 0 {
			totals = summary.EndTotals
			label = "end"
		}
		parts = append(parts, fmt.Sprintf("stats_cpus=%d", len(summary.StatsCPUs)))
		parts = append(parts, fmt.Sprintf("%s_entries=%d", label, totals.Entries))
		parts = append(parts, fmt.Sprintf("%s_dropped=%d", label, totals.DroppedEvents))
		parts = append(parts, fmt.Sprintf("%s_overrun=%d", label, totals.Overrun))
		parts = append(parts, fmt.Sprintf("%s_commit_overrun=%d", label, totals.CommitOverrun))
		parts = append(parts, fmt.Sprintf("%s_read=%d", label, totals.ReadEvents))
		parts = append(parts, fmt.Sprintf("%s_bytes=%d", label, totals.Bytes))
	}
	if summary.DetailMessages > 0 {
		parts = append(parts, fmt.Sprintf("detail_messages=%d", summary.DetailMessages))
		parts = append(parts, fmt.Sprintf("detail_cpus=%d", len(summary.DetailCPUs)))
		parts = append(parts, fmt.Sprintf("structured_event_records=%d", summary.DetailEventCount))
		parts = append(parts, fmt.Sprintf("detail_overwrite=%d", summary.DetailOverwrite))
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
	return "ftrace-plugin structured metadata: " + strings.Join(parts, "; ")
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
	length := int64(binary.LittleEndian.Uint64(header[8:16]))
	if length < profilerTraceHeaderSize {
		return profilerTraceHeader{}, false
	}
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

func parseProfilerPluginData(data []byte) (profilerPluginData, bool) {
	var out profilerPluginData
	off := 0
	for off < len(data) {
		field, wire, ok := readProtoKey(data, &off)
		if !ok {
			return out, false
		}
		switch field {
		case 1:
			if wire != 2 {
				return out, false
			}
			out.Name, ok = readProtoString(data, &off)
		case 2:
			var v uint64
			v, ok = readProtoVarint(data, &off)
			out.Status = uint32(v)
		case 3:
			if wire != 2 {
				return out, false
			}
			out.Data, ok = readProtoBytes(data, &off)
		case 4:
			out.ClockID, ok = readProtoVarint(data, &off)
		case 5:
			out.TvSec, ok = readProtoVarint(data, &off)
		case 6:
			out.TvNsec, ok = readProtoVarint(data, &off)
		case 7:
			if wire != 2 {
				return out, false
			}
			out.Version, ok = readProtoString(data, &off)
		case 8:
			var v uint64
			v, ok = readProtoVarint(data, &off)
			out.SampleInterval = uint32(v)
		default:
			ok = skipProtoField(data, &off, wire)
		}
		if !ok {
			return out, false
		}
	}
	return out, out.Name != "" || len(out.Data) > 0
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
