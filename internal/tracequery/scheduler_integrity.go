package tracequery

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// schedulerRowIntegrityFailure records a critical scheduler row whose exact
// identity/state fields were absent or not parseable. It is deliberately kept
// separate from schedulerOrderViolation: physical row incompleteness and a
// same-lane timestamp rollback are different facts and must be reported to the
// model with different remediation.
type schedulerRowIntegrityFailure struct {
	EventName      string
	Line           int
	Ts             float64
	CPU            int
	PIDs           []int
	AffectsAllPIDs bool
	Fields         []string
	SourcePath     string
	// LocalLine: artifact-local physical line, set at composite rebase time
	// (audit #36); zero when Line is already physical.
	LocalLine int
}

const schedulerRowIntegrityFailureCap = 64

func (f *schedulerRowIntegrityFailure) reason() string {
	if f == nil {
		return ""
	}
	reason := fmt.Sprintf(
		"scheduler_row_parse_incomplete event=%s missing_or_invalid=%s ts=%.6f cpu=%d line=%d",
		f.EventName, strings.Join(f.Fields, ","), f.Ts, f.CPU, f.Line)
	if f.SourcePath != "" {
		reason += fmt.Sprintf(" source=%s", f.SourcePath)
		reason += witnessLocalLineSuffix(f.Line, f.LocalLine)
	}
	return reason
}

func schedulerIntegrityRawCandidate(line string) bool {
	return strings.Contains(line, "sched_switch:") ||
		strings.Contains(line, "sched_wakeup:") ||
		strings.Contains(line, "sched_wakeup_new:") ||
		strings.Contains(line, "sched_waking:") ||
		strings.Contains(line, "sched_migrate_task:") ||
		strings.Contains(line, "sched_pi_setprio:") ||
		strings.Contains(line, "binder_set_priority:")
}

func schedulerPriorityMutationEventName(eventName string) bool {
	switch eventName {
	case "sched_pi_setprio", "binder_set_priority":
		return true
	default:
		return false
	}
}

// schedulerRowValidationFailure validates the precise, event-specific fields
// that scheduler duration/state consumers require. Presence and parseability
// are separate from value: an explicit PID 0 is a valid scheduler identity
// token (idle), while an absent or malformed PID must never silently become 0.
//
// The ftrace header match is the hard event discriminator. A token merely
// mentioned inside another event's payload is not allowed to poison scheduler
// state.
func schedulerRowValidationFailure(lineNo int, line string) *schedulerRowIntegrityFailure {
	var scan lineScan
	scan.reset(lineNo, line)
	return schedulerRowValidationFailureScan(&scan)
}

// schedulerRowValidationFailureScan consumes the shared per-line memo so the
// hot loop pays a single header match and a single parseKV per physical line
// (perf audit #21).
func schedulerRowValidationFailureScan(s *lineScan) *schedulerRowIntegrityFailure {
	if !schedulerIntegrityRawCandidate(s.line) {
		return nil
	}
	m := s.match()
	if len(m) == 0 {
		return nil
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	switch rawType {
	case "sched_switch", "sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_migrate_task":
	default:
		return nil
	}
	ts, _ := s.timestamp()
	cpu, _ := atoiMaybe(m[4])
	kv := s.keyValues()
	if rawType == "sched_switch" && s.schedSwitchKVFailure != "" {
		return &schedulerRowIntegrityFailure{
			EventName:      rawType,
			Line:           s.lineNo,
			Ts:             ts,
			CPU:            cpu,
			AffectsAllPIDs: true,
			Fields:         schedSwitchIntegrityFailureFields(s.schedSwitchKVFailure),
		}
	}
	if s.schedulerTyped.Active && len(s.schedulerTyped.HardFields) != 0 {
		failure := &schedulerRowIntegrityFailure{
			EventName:      rawType,
			Line:           s.lineNo,
			Ts:             ts,
			CPU:            cpu,
			AffectsAllPIDs: s.schedulerTyped.HardAffectsAllPIDs,
			Fields:         append([]string(nil), s.schedulerTyped.HardFields...),
		}
		if s.schedulerTyped.HardPID > 0 {
			failure.PIDs = []int{s.schedulerTyped.HardPID}
		}
		failure.PIDs = append(failure.PIDs, s.schedulerTyped.HardPIDs...)
		sort.Ints(failure.PIDs)
		sort.Strings(failure.Fields)
		return failure
	}
	return schedulerFieldsValidationFailure(s.lineNo, rawType, ts, cpu, kv)
}

func schedSwitchIntegrityFailureFields(detail string) []string {
	base := "sched_switch_core"
	switch {
	case strings.HasPrefix(detail, "prev_pid"):
		base = "prev_pid"
	case strings.HasPrefix(detail, "prev_prio"):
		base = "prev_prio"
	case strings.HasPrefix(detail, "prev_state"):
		base = "prev_state"
	case strings.HasPrefix(detail, "next_comm"):
		base = "next_comm"
	case strings.HasPrefix(detail, "next_pid"):
		base = "next_pid"
	case strings.HasPrefix(detail, "next_prio"):
		base = "next_prio"
	}
	if detail == base {
		return []string{base}
	}
	return []string{base, detail}
}

func schedulerFieldsValidationFailure(lineNo int, rawType string, ts float64, cpu int, kv map[string]string) *schedulerRowIntegrityFailure {
	switch rawType {
	case "sched_switch", "sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_migrate_task":
	default:
		return nil
	}
	failure := &schedulerRowIntegrityFailure{EventName: rawType, Line: lineNo, Ts: ts, CPU: cpu}
	addPID := func(raw string) bool {
		pid, ok := atoiMaybe(raw)
		// ENG audit #47 (§29.25 处置委托 2026-07-10): a parseable NEGATIVE pid is
		// not a scheduler identity. Kernel ftrace never emits one in these
		// rows; accepting it recorded nothing (pid>0 guard below), so the row
		// passed the hard integrity gate while every duration/state consumer
		// skips pid<=0 roles — the transition silently unbooked, reopening the
		// zero/absence ambiguity this file's contract forbids. Explicit pid 0
		// stays valid (idle).
		if !ok || pid < 0 {
			return false
		}
		if pid > 0 {
			for _, existing := range failure.PIDs {
				if existing == pid {
					return true
				}
			}
			failure.PIDs = append(failure.PIDs, pid)
		}
		return true
	}
	requireCPUScalar := func(field, raw string) bool {
		if _, present, valid, _ := parseTraceCPUScalar(raw); present && valid {
			return true
		}
		failure.Fields = append(failure.Fields, field)
		return false
	}

	switch rawType {
	case "sched_switch":
		prevOK := addPID(kv["prev_pid"])
		if !prevOK {
			failure.Fields = append(failure.Fields, "prev_pid")
		}
		nextOK := addPID(kv["next_pid"])
		if !nextOK {
			failure.Fields = append(failure.Fields, "next_pid")
		}
		if strings.TrimSpace(kv["prev_state"]) == "" {
			failure.Fields = append(failure.Fields, "prev_state")
		}
		// If either role identity is unavailable, any requested TID could
		// occupy that role. Target-only duration queries therefore fail closed.
		failure.AffectsAllPIDs = !prevOK || !nextOK
	case "sched_wakeup", "sched_wakeup_new", "sched_waking":
		pidOK := addPID(kv["pid"])
		if !pidOK {
			failure.Fields = append(failure.Fields, "pid")
			failure.AffectsAllPIDs = true
		}
	case "sched_migrate_task":
		pidRaw := firstNonEmpty(kv["target_pid"], kv["task_pid"], kv["pid"], kv["tid"])
		pidOK := addPID(pidRaw)
		if !pidOK {
			failure.Fields = append(failure.Fields, "pid")
			failure.AffectsAllPIDs = true
		}
		requireCPUScalar("orig_cpu", kv["orig_cpu"])
		requireCPUScalar("dest_cpu", kv["dest_cpu"])
	}
	if len(failure.Fields) == 0 {
		return nil
	}
	sort.Strings(failure.Fields)
	sort.Ints(failure.PIDs)
	return failure
}

// schedulerRejectedRowFailure covers a critical row that passed the required
// field check but the parser still rejected (or panicked on) it. This is rare,
// but silently ignoring it would reopen the same zero/absence ambiguity.
func schedulerRejectedRowFailure(lineNo int, line string) *schedulerRowIntegrityFailure {
	var scan lineScan
	scan.reset(lineNo, line)
	return schedulerRejectedRowFailureScan(&scan)
}

func schedulerRejectedRowFailureScan(s *lineScan) *schedulerRowIntegrityFailure {
	if !schedulerIntegrityRawCandidate(s.line) {
		return nil
	}
	m := s.match()
	rawType := ""
	ts, timestampKnown := 0.0, false
	cpu := -1
	if len(m) != 0 {
		rawType = strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
		ts, timestampKnown = s.timestamp()
		if parsedCPU, present, valid, _ := parseTraceCPUScalar(m[4]); present && valid {
			cpu = parsedCPU
		}
	} else {
		// The normal parser intentionally rejects malformed header scalars. A
		// complete physical envelope still has authority to quarantine an exact
		// priority-mutation event, but never to materialize a scheduler state.
		// ProbePhysicalFtraceHeader rejects free prose/quoted payload lookalikes.
		probe, ok := ProbePhysicalFtraceHeader(s.line)
		if !ok || !schedulerPriorityMutationEventName(probe.EventName) {
			return nil
		}
		rawType = probe.EventName
		if probe.TimestampKnown {
			ts = float64(probe.TimestampNS) / 1e9
			timestampKnown = true
		}
		if loose := loosePhysicalFtraceLine(s.line); len(loose) >= 8 {
			if parsedCPU, present, valid, _ := parseTraceCPUScalar(loose[4]); present && valid {
				cpu = parsedCPU
			}
		}
	}
	switch rawType {
	case "sched_switch", "sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_migrate_task", "sched_pi_setprio", "binder_set_priority":
	default:
		return nil
	}
	if !timestampKnown {
		ts = math.NaN()
	}
	fields := []string{"parser_rejected_row"}
	if !timestampKnown {
		fields = append(fields, "timestamp")
	}
	if cpu < 0 {
		fields = append(fields, "cpu")
	}
	failure := &schedulerRowIntegrityFailure{
		EventName:      rawType,
		Line:           s.lineNo,
		Ts:             ts,
		CPU:            cpu,
		AffectsAllPIDs: true,
		Fields:         fields,
	}
	// A malformed header still leaves the exact producer body available. Bind
	// sched_pi_setprio only when its occurrence-aware subject grammar elects
	// one canonical PID; binder remains global until a production-backed wire
	// profile exists.
	if rawType == "sched_pi_setprio" {
		if loose := loosePhysicalFtraceLine(s.line); len(loose) >= 8 {
			_, typed := parsePriorityMutationFields(loose[7])
			if typed.PriorityMutationPID > 0 {
				failure.PIDs = []int{typed.PriorityMutationPID}
				failure.AffectsAllPIDs = false
			}
		}
	}
	return failure
}

func schedulerRowIntegrityFailureRelevantToQuery(f *schedulerRowIntegrityFailure, q Query, onlyPID int) bool {
	if f == nil {
		return false
	}
	if q.LineStart > 0 && f.Line < q.LineStart {
		return false
	}
	if q.LineEnd > 0 && f.Line > q.LineEnd {
		return false
	}
	// A pre-window scheduler boundary can govern the window-head state, so
	// there is intentionally no TimeStart exclusion. A known row after the
	// requested end cannot affect this query.
	if q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && f.Ts > q.TimeEnd {
		return false
	}
	if onlyPID <= 0 || f.AffectsAllPIDs {
		return true
	}
	for _, pid := range f.PIDs {
		if pid == onlyPID {
			return true
		}
	}
	return false
}

func appendSchedulerRowIntegrityFailure(idx *Index, failure schedulerRowIntegrityFailure) {
	if idx == nil {
		return
	}
	for _, existing := range idx.schedulerRowIntegrityFailures {
		if existing.Line == failure.Line && existing.EventName == failure.EventName &&
			(existing.Ts == failure.Ts || math.IsNaN(existing.Ts) && math.IsNaN(failure.Ts)) &&
			strings.Join(existing.Fields, ",") == strings.Join(failure.Fields, ",") {
			return
		}
	}
	priorityOnly := schedulerPriorityMutationEventName(failure.EventName)
	categoryCount := 0
	for i := range idx.schedulerRowIntegrityFailures {
		if schedulerPriorityMutationEventName(idx.schedulerRowIntegrityFailures[i].EventName) == priorityOnly {
			categoryCount++
		}
	}
	if categoryCount >= schedulerRowIntegrityFailureCap {
		if priorityOnly {
			markPriorityMutationIntegrityOverflow(idx, schedulerRowIntegrityFailureSourcePath(idx, failure))
		} else {
			markSchedulerRowIntegrityOverflow(idx, schedulerRowIntegrityFailureSourcePath(idx, failure))
		}
		return
	}
	failure.PIDs = append([]int(nil), failure.PIDs...)
	failure.Fields = append([]string(nil), failure.Fields...)
	idx.schedulerRowIntegrityFailures = append(idx.schedulerRowIntegrityFailures, failure)
}

func markPriorityMutationIntegrityOverflow(idx *Index, sourcePath string) {
	if idx == nil {
		return
	}
	idx.priorityMutationIntegrityFailuresCapped = true
	if idx.priorityMutationIntegrityOverflowGlobal {
		return
	}
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		idx.priorityMutationIntegrityOverflowGlobal = true
		idx.priorityMutationIntegrityOverflowSources = nil
		return
	}
	for _, existing := range idx.priorityMutationIntegrityOverflowSources {
		if existing == sourcePath {
			return
		}
	}
	if len(idx.priorityMutationIntegrityOverflowSources) >= schedulerRowIntegrityFailureCap {
		idx.priorityMutationIntegrityOverflowGlobal = true
		idx.priorityMutationIntegrityOverflowSources = nil
		return
	}
	idx.priorityMutationIntegrityOverflowSources = append(idx.priorityMutationIntegrityOverflowSources, sourcePath)
	sort.Strings(idx.priorityMutationIntegrityOverflowSources)
}

func schedulerRowIntegrityFailureSourcePath(idx *Index, failure schedulerRowIntegrityFailure) string {
	if strings.TrimSpace(failure.SourcePath) != "" {
		return failure.SourcePath
	}
	if idx == nil {
		return ""
	}
	if len(idx.TraceArtifacts) == 1 && idx.TraceArtifacts[0].CausalCompatible {
		return idx.TraceArtifacts[0].SourcePath
	}
	if len(idx.TraceArtifacts) == 0 && idx.Path != "" && !traceBundlePath(idx.Path) {
		return idx.Path
	}
	return ""
}

// markSchedulerRowIntegrityOverflow preserves the smallest exact physical
// scope available after the bounded witness ledger fills. Source-local caps
// fail-close only that tracebundle child; an unknown/unbounded source raises
// the explicit global bit. The source list is itself bounded, and overflowing
// it fails closed globally instead of silently forgetting a child.
func markSchedulerRowIntegrityOverflow(idx *Index, sourcePath string) {
	if idx == nil {
		return
	}
	idx.schedulerRowIntegrityFailuresCapped = true
	if idx.schedulerRowIntegrityOverflowGlobal {
		return
	}
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		idx.schedulerRowIntegrityOverflowGlobal = true
		idx.schedulerRowIntegrityOverflowSources = nil
		return
	}
	for _, existing := range idx.schedulerRowIntegrityOverflowSources {
		if existing == sourcePath {
			return
		}
	}
	if len(idx.schedulerRowIntegrityOverflowSources) >= schedulerRowIntegrityFailureCap {
		idx.schedulerRowIntegrityOverflowGlobal = true
		idx.schedulerRowIntegrityOverflowSources = nil
		return
	}
	idx.schedulerRowIntegrityOverflowSources = append(idx.schedulerRowIntegrityOverflowSources, sourcePath)
	sort.Strings(idx.schedulerRowIntegrityOverflowSources)
}

func schedulerRowIntegrityFailureForQuery(idx *Index, q Query, onlyPID int) *schedulerRowIntegrityFailure {
	if idx == nil {
		return nil
	}
	for i := range idx.schedulerRowIntegrityFailures {
		if schedulerPriorityMutationEventName(idx.schedulerRowIntegrityFailures[i].EventName) {
			continue
		}
		if schedulerRowIntegrityFailureRelevantToQuery(&idx.schedulerRowIntegrityFailures[i], q, onlyPID) {
			copy := idx.schedulerRowIntegrityFailures[i]
			return &copy
		}
	}
	if idx.schedulerRowIntegrityFailuresCapped {
		return &schedulerRowIntegrityFailure{
			EventName:      "scheduler_integrity_audit",
			Line:           -1,
			CPU:            -1,
			AffectsAllPIDs: true,
			Fields:         []string{"audit_truncated"},
		}
	}
	return nil
}

type schedulerStateIntegrityFailure struct {
	code       string
	reasonText string
}

func (f *schedulerStateIntegrityFailure) reason() string {
	if f == nil {
		return ""
	}
	return f.reasonText
}

func schedulerStateIntegrityFailureForQuery(idx *Index, q Query, onlyPID int) *schedulerStateIntegrityFailure {
	if malformed := schedulerRowIntegrityFailureForQuery(idx, q, onlyPID); malformed != nil {
		return &schedulerStateIntegrityFailure{code: "scheduler_row_parse_incomplete", reasonText: malformed.reason()}
	}
	if order := schedulerStateOrderViolationForQuery(idx, q, onlyPID); order != nil {
		return &schedulerStateIntegrityFailure{code: "scheduler_lane_timestamp_regressed", reasonText: order.reason()}
	}
	return nil
}
