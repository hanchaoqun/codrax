package tracequery

import (
	"fmt"
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
	}
	return reason
}

func schedulerIntegrityRawCandidate(line string) bool {
	return strings.Contains(line, "sched_switch:") ||
		strings.Contains(line, "sched_wakeup:") ||
		strings.Contains(line, "sched_wakeup_new:") ||
		strings.Contains(line, "sched_waking:") ||
		strings.Contains(line, "sched_migrate_task:")
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
	if !schedulerIntegrityRawCandidate(line) {
		return nil
	}
	m := ftraceLineRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return nil
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	switch rawType {
	case "sched_switch", "sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_migrate_task":
	default:
		return nil
	}
	ts, _ := parseLineTimestamp(line)
	cpu, _ := atoiMaybe(m[4])
	fields := strings.TrimSpace(m[7])
	return schedulerFieldsValidationFailure(lineNo, rawType, ts, cpu, parseKV(fields))
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
		if !ok {
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
	if !schedulerIntegrityRawCandidate(line) {
		return nil
	}
	m := ftraceLineRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return nil
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	switch rawType {
	case "sched_switch", "sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_migrate_task":
	default:
		return nil
	}
	ts, _ := parseLineTimestamp(line)
	cpu, _ := atoiMaybe(m[4])
	return &schedulerRowIntegrityFailure{
		EventName:      rawType,
		Line:           lineNo,
		Ts:             ts,
		CPU:            cpu,
		AffectsAllPIDs: true,
		Fields:         []string{"parser_rejected_row"},
	}
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
			existing.Ts == failure.Ts && strings.Join(existing.Fields, ",") == strings.Join(failure.Fields, ",") {
			return
		}
	}
	if len(idx.schedulerRowIntegrityFailures) >= schedulerRowIntegrityFailureCap {
		idx.schedulerRowIntegrityFailuresCapped = true
		return
	}
	failure.PIDs = append([]int(nil), failure.PIDs...)
	failure.Fields = append([]string(nil), failure.Fields...)
	idx.schedulerRowIntegrityFailures = append(idx.schedulerRowIntegrityFailures, failure)
}

func schedulerRowIntegrityFailureForQuery(idx *Index, q Query, onlyPID int) *schedulerRowIntegrityFailure {
	if idx == nil {
		return nil
	}
	for i := range idx.schedulerRowIntegrityFailures {
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
