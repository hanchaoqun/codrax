package tracequery

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

type cpuStateFrequencySample struct {
	ts    float64
	value uint32
	line  int
	known bool
}

type cpuStateFrequencyLane struct {
	idle, freq           []cpuStateFrequencySample
	idleIssue, freqIssue string
}

type cpuStateFrequencyCollector struct {
	lanes                map[int]*cpuStateFrequencyLane
	idleIssue, freqIssue string
	samples              int
	overflow             bool
}

func newCPUStateFrequencyCollector() *cpuStateFrequencyCollector {
	return &cpuStateFrequencyCollector{lanes: map[int]*cpuStateFrequencyLane{}}
}

func (c *cpuStateFrequencyCollector) observe(ev Event) bool {
	if ev.Type != EventCPUIdle && (ev.Type != EventCPUFrequency || ev.Name == "clock_set_rate") {
		return true
	}
	cpu := eventCPUForStats(ev)
	if cpu < 0 {
		if ev.Type == EventCPUIdle {
			c.idleIssue = "unattributed_invalid_cpu_control"
		} else {
			c.freqIssue = "unattributed_invalid_cpu_control"
		}
		return true
	}
	lane := c.lanes[cpu]
	if lane == nil {
		lane = &cpuStateFrequencyLane{}
		c.lanes[cpu] = lane
	}
	values, issue := &lane.idle, &lane.idleIssue
	value := uint32(ev.State)
	known := !ev.CPUInputInvalid && eventCPUScalarKnown(ev) && ev.State <= math.MaxUint32
	if ev.Type == EventCPUFrequency {
		values, issue = &lane.freq, &lane.freqIssue
		_, khz, ok := perCPUFrequencyTransitionValues(ev)
		known, value = ok, uint32(khz)
	}
	if len(*values) > 0 && ev.Ts < (*values)[len(*values)-1].ts {
		*issue = "physical_cpu_lane_timestamp_rollback"
	}
	if !known {
		*issue = "malformed_cpu_control_sample"
	}
	c.samples++
	if c.samples > cpuStateFrequencySampleLimit {
		c.overflow = true
		return false
	}
	*values = append(*values, cpuStateFrequencySample{ev.Ts, value, ev.Line, known})
	return true
}

// StreamCPUStateFrequency reads the same generation once, without an event
// index. Only bounded CPU-control samples are retained; the full physical
// prefix proves carry-in and lane order. StreamScan owns cancellation and
// held-file/path generation checks. Composite identities are not merged.
func StreamCPUStateFrequency(ctx context.Context, path string, q Query) (Result, error) {
	if err := ValidateViewName(q.View); err != nil {
		return Result{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateCPUStateFrequencyQuery(q); err != nil {
		return Result{}, err
	}
	c := newCPUStateFrequencyCollector()
	idx, err := StreamScan(ctx, path, q.TraceFlavorHint, c.observe)
	if err != nil {
		return Result{}, err
	}
	q = normalizeQuery(idx, q)
	q = q.WithRunContext(ctx)
	p := c.finish(idx, q)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{View: ViewCPUStateFrequency, SourcePath: idx.Path, TimeUnit: "seconds", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd,
		LineCount: idx.LineCount, ScannedLineCount: idx.ScannedLineCount, UnparsedLineCount: idx.UnparsedLines,
		EventCount: idx.ParsedKnown, ParseLinePanics: idx.ParseLinePanics, ClockRegressions: idx.ClockRegressions,
		TraceArtifacts: idx.TraceArtifacts, CPUStateFrequency: p, Caveats: p.Caveats}, nil
}

func validateCPUStateFrequencyQuery(q Query) error {
	if !finiteCPUStateFrequency(q.TimeStart) || !finiteCPUStateFrequency(q.TimeEnd) || q.TimeEnd > 0 && q.TimeEnd <= q.TimeStart {
		return fmt.Errorf("cpu_state_frequency requires a finite positive-width time window")
	}
	if q.PID != 0 || q.Thread != "" || q.ThreadInput != "" || q.TargetScope == TargetScopeProcess {
		return fmt.Errorf("cpu_state_frequency is a CPU-global observation; thread/process selectors cannot be applied")
	}
	if q.LineStart != 0 || q.LineEnd != 0 || q.Pattern != "" || len(q.Patterns) != 0 || q.SpanName != "" || len(q.EventTypes) != 0 || len(q.EventNames) != 0 || len(q.EventFieldFilters) != 0 || len(q.TraceMarkActions) != 0 {
		return fmt.Errorf("cpu_state_frequency requires a time window without event/line/span filters")
	}
	return nil
}

func buildCPUStateFrequency(idx *Index, q Query) *CPUStateFrequencyResult {
	c := newCPUStateFrequencyCollector()
	if err := validateCPUStateFrequencyQuery(q); err != nil {
		return unavailableCPUStateFrequency(idx.Path, q, err.Error())
	}
	if idx.TraceDBTextCarrierRows > 0 {
		return unavailableCPUStateFrequency(idx.Path, q, "sql_measure_interval_semantics_not_preserved")
	}
	if idx.Windowed || idx.RelationScoped || len(idx.TraceArtifacts) > 1 || len(idx.TraceArtifacts) == 1 && idx.TraceArtifacts[0].SourcePath != idx.Path {
		return unavailableCPUStateFrequency(idx.Path, q, "requires_complete_single_source_cpu_control_scan")
	}
	for _, ev := range idx.Events {
		if q.runCancel.tick() || !c.observe(ev) {
			break
		}
	}
	return c.finish(idx, q)
}

func unavailableCPUStateFrequency(path string, q Query, reason string) *CPUStateFrequencyResult {
	return &CPUStateFrequencyResult{Status: "unavailable", Reason: reason, SourcePath: path,
		Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}, WindowWallMs: (q.TimeEnd - q.TimeStart) * 1000}
}

func (c *cpuStateFrequencyCollector) finish(idx *Index, q Query) *CPUStateFrequencyResult {
	p := unavailableCPUStateFrequency(idx.Path, q, "invalid_or_empty_time_window")
	if !finiteCPUStateFrequency(q.TimeStart) || !finiteCPUStateFrequency(q.TimeEnd) || q.TimeEnd <= q.TimeStart {
		return p
	}
	if c.overflow {
		p.Reason = "cpu_control_sample_limit"
		return p
	}
	if idx.TraceDBTextCarrierRows > 0 {
		p.Reason = "sql_measure_interval_semantics_not_preserved"
		return p
	}
	// Rejected physical rows do not enter the callback. The parser's typed
	// integrity receipt still prevents malformed transitions from disappearing.
	ownedRejected := map[int]bool{}
	for _, f := range idx.cpuInputIntegrityFailures {
		if f.Field == "control_row" && validTraceCPUIndex(f.CPU) {
			ownedRejected[f.Line] = true
		}
	}
	for _, f := range idx.cpuInputIntegrityFailures {
		if f.Field == "header_cpu" && ownedRejected[f.Line] {
			continue
		}
		profile := cpuScalarProfileForName(f.EventName)
		if profile != cpuScalarProfileIdle && profile != cpuScalarProfileFrequency {
			continue
		}
		// Scalar/control_row receipts carry payload CPU, not emitter CPU.
		if validTraceCPUIndex(f.CPU) && f.Field != "header_cpu" && f.Field != "cpu_id" {
			lane := c.lanes[f.CPU]
			if lane == nil {
				lane = &cpuStateFrequencyLane{}
				c.lanes[f.CPU] = lane
			}
			if profile == cpuScalarProfileIdle {
				lane.idleIssue = "rejected_cpu_idle_input"
			} else {
				lane.freqIssue = "rejected_cpu_frequency_input"
			}
		} else if profile == cpuScalarProfileIdle {
			c.idleIssue = "rejected_cpu_idle_input"
		} else {
			c.freqIssue = "rejected_cpu_frequency_input"
		}
	}
	if idx.cpuInputIntegrityFailuresCapped {
		c.idleIssue, c.freqIssue = "cpu_input_audit_truncated", "cpu_input_audit_truncated"
	}
	p.Status, p.Reason = "available", ""
	p.Caveats = []string{
		"Raw cpu_idle index 0 is idle, not Running; active requires an explicit idle-exit marker. State names C1/C2/C3 are not inferred.",
		"Frequency is kHz. Missing/zero-frequency transitions remain unknown; no following sample, other CPU or cluster donates missing coverage.",
		"Per-CPU percentages use the full selected window. CPU-time sums that window once per observed CPU, not wall-clock latency or a complete hardware CPU census.",
		"CPU-control overlap is observational context, not thread execution, power consumption, priority inversion, compute shortage or response root cause.",
	}
	var cpus []int
	for cpu, lane := range c.lanes {
		if cpuStateFrequencyHasPrefix(lane.idle, q.TimeEnd) || cpuStateFrequencyHasPrefix(lane.freq, q.TimeEnd) {
			cpus = append(cpus, cpu)
		}
	}
	sort.Ints(cpus)
	explicit := parseCoreTopology(q.CoreTopology)
	for _, cpu := range cpus {
		if q.runCancel.tick() {
			return p
		}
		lane := c.lanes[cpu]
		if c.idleIssue != "" {
			lane.idleIssue = c.idleIssue
		}
		if c.freqIssue != "" {
			lane.freqIssue = c.freqIssue
		}
		row := buildCPUStateFrequencyCPU(cpu, lane, q)
		if class := explicit[cpu]; class != "" {
			row.CoreClass, row.TopologySource = class, "explicit"
		}
		p.CPUCount++
		p.CPUTimeMs += p.WindowWallMs
		p.KnownJointMs += row.JointKnownMs
		p.UnknownJointMs += row.UnknownJointMs
		if len(p.CPUs) < cpuStateFrequencyCPULimit {
			p.CPUs = append(p.CPUs, row)
		} else {
			p.OmittedCPUs++
		}
	}
	if p.CPUCount == 0 {
		p.Status, p.Reason = "unavailable", "no_attributed_cpu_control_samples_before_window_end"
	}
	return p
}

func cpuStateFrequencyHasPrefix(samples []cpuStateFrequencySample, end float64) bool {
	for _, sample := range samples {
		if sample.ts < end {
			return true
		}
	}
	return false
}

func finiteCPUStateFrequency(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }

// Rejected envelopes cannot silently disappear from a state timeline. Only
// the outer physical event column and strict payload identify a lane to
// withhold; this does not recover an event or any numeric measurement.
func cpuStateFrequencyRejectedFailure(s *lineScan) *cpuInputIntegrityFailure {
	m := s.match()
	if len(m) == 0 {
		m = loosePhysicalFtraceLine(s.line)
	}
	if len(m) == 0 {
		return nil
	}
	name := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	profile := cpuScalarProfileForName(name)
	if profile != cpuScalarProfileIdle && profile != cpuScalarProfileFrequency {
		return nil
	}
	_, typed := parseCPUScalarTypedFields(name, strings.TrimSpace(m[7]))
	cpu := -1
	if typed.CPUKnown {
		cpu = typed.CPU
	}
	ts, _ := parseTraceTimestampSeconds(m[5])
	return &cpuInputIntegrityFailure{EventName: name, Field: "control_row", ReasonCode: "cpu_control_row_rejected", CPU: cpu, Ts: ts, Line: s.lineNo}
}
