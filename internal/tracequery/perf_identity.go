package tracequery

import "strings"

const perfSourceOnlyResolution = "perf_source_only"

func perfSampleCPUIsExplicitNoClaim(kv map[string]string) bool {
	if kv == nil {
		return false
	}
	return strings.TrimSpace(kv["cpu"]) == "-1" && strings.TrimSpace(kv["cpu_known"]) == "false"
}

// perfSampleIsSourceOnlyIdentity is the single hard-negative predicate for
// SQL-perf rows that carry useful symbol/DSO inventory but no proved trace
// thread generation. Each typed negative claim is independently decisive:
// thread_identity_known=false or lifecycle_unverified=true cannot be rescued
// by a contradictory/missing resolution token. The legacy arm preserves
// safety for historical trace_streamer_db rows that emitted only
// resolution=perf_source_only.
//
// Keep this gate on precise converter-owned tokens. perf_source_* coordinates
// are audit payload, never authority, and therefore deliberately do not
// participate in the decision.
func perfSampleIsSourceOnlyIdentity(ev Event) bool {
	if ev.Type != EventPerfSample || ev.PerfFields == nil {
		return false
	}
	pf := ev.PerfFields
	if pf.ThreadIdentityKnown != nil && !*pf.ThreadIdentityKnown {
		return true
	}
	if pf.LifecycleUnverified != nil && *pf.LifecycleUnverified {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(pf.Source), "trace_streamer_db") &&
		strings.EqualFold(strings.TrimSpace(pf.Resolution), perfSourceOnlyResolution)
}

// perfSampleHasKnownCPU is the sole perf CPU-identity admission predicate.
// A numerically plausible Event.CPU is insufficient when the typed wire says
// cpu_known=false (or when the typed token is absent on a synthetic Event).
func perfSampleHasKnownCPU(ev Event) bool {
	return ev.Type == EventPerfSample &&
		ev.PerfFields != nil &&
		ev.PerfFields.CPUKnown != nil &&
		*ev.PerfFields.CPUKnown &&
		validTraceCPUIndex(ev.CPU)
}

// perfSampleIsOnCPU is the closed semantic gate for treating a known sample
// CPU as an execution coordinate. off_cpu and unknown samples remain useful
// global inventory, but their CPU (when present) must not enter CPU-scoped
// joins, rosters, or timeline CPU dimensions as on-CPU work.
func perfSampleIsOnCPU(ev Event) bool {
	return ev.Type == EventPerfSample &&
		ev.PerfFields != nil &&
		strings.TrimSpace(ev.PerfFields.SampleKind) == "on_cpu"
}

// perfSampleHasOnCPUExecutionCoordinate is the one admission predicate for
// concrete perf CPU execution attribution. Keep callers on this combined
// claim: a known event CPU and on_cpu semantics are independently necessary
// and neither may substitute for the other.
func perfSampleHasOnCPUExecutionCoordinate(ev Event) bool {
	return perfSampleHasKnownCPU(ev) && perfSampleIsOnCPU(ev)
}

func perfSampleOnCPUExecutionCPU(ev Event) (int, bool) {
	if !perfSampleHasOnCPUExecutionCoordinate(ev) {
		return -1, false
	}
	return ev.CPU, true
}

func perfSampleMatchesExecutionThread(ev Event, thread ThreadRef) bool {
	return perfSampleHasOnCPUExecutionCoordinate(ev) && perfSampleMatchesThread(ev, thread)
}

// normalizePerfSampleClaims erases transport/header placeholders before an
// Event enters the index. It keeps symbolization and perf_source_* audit
// inventory intact while making both unsafe dimensions impossible to revive
// through later selectors or aggregates.
func normalizePerfSampleClaims(ev *Event) {
	if ev == nil || ev.Type != EventPerfSample || ev.PerfFields == nil {
		return
	}
	if !perfSampleHasKnownCPU(*ev) {
		scrubPerfSampleCPUIdentity(ev)
	}
	if perfSampleIsSourceOnlyIdentity(*ev) {
		scrubPerfSampleThreadIdentity(ev)
	}
}

// perfWireBool parses the exact boolean scalar vocabulary used by the
// converter-owned thread_identity_known/lifecycle_unverified wire fields.
// Free-form words such as "unknown" or "available" must not drive a hard
// identity gate.
func perfWireBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}
