package types

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// TraceTargetStateScopeAuthority is the wording boundary carried by one
// compiled target_window_states account. Every duration is scoped to the
// target thread's own wall-clock state partition; none is a CPU-wide
// utilization or saturation measurement.
type TraceTargetStateScopeAuthority struct {
	ArtifactLabel  string
	Subject        string
	WindowStartTs  float64
	WindowEndTs    float64
	WindowMS       float64
	RunningMS      float64
	RunnableMS     float64
	SleepMS        float64
	DStateMS       float64
	IOWaitMS       float64
	SleepIOWaitMS  float64
	TotalMS        float64
	UnaccountedMS  float64
	CoverageStatus string
	HeadCarryMS    float64
	HeadCarryState string
	TailOpenMS     float64
	TailOpenState  string
	EvidenceID     string
}

const traceTargetStateCoverageToleranceMS = 0.002

// BuildTraceTargetStateScopeAuthorities compiles the target-thread scope
// authorities from the already-selected projection accounts. It deliberately
// consumes the compiled projection rather than all raw target-state records so
// explicit-window election and supplemental-window separation remain owned by
// the existing projection compiler.
func BuildTraceTargetStateScopeAuthorities(set TraceCausalProjectionSet) []TraceTargetStateScopeAuthority {
	out := make([]TraceTargetStateScopeAuthority, 0, len(set.Projections))
	seen := map[string]bool{}
	for _, projection := range set.Projections {
		account := projection.TargetStateAccount
		if account == nil || strings.TrimSpace(account.Subject) == "" || account.TotalMS <= 0 {
			continue
		}
		windowMS := 0.0
		if account.WindowEndTs > account.WindowStartTs {
			windowMS = (account.WindowEndTs - account.WindowStartTs) * 1000
		}
		// An account above its own selected window is impossible and cannot
		// become answer authority. Allow only µs-level representation drift.
		if windowMS > 0 && account.TotalMS > windowMS+traceTargetStateCoverageToleranceMS {
			continue
		}
		coverageStatus := "window_unknown"
		unaccountedMS := 0.0
		if windowMS > 0 {
			coverageStatus = "complete"
			if windowMS-account.TotalMS > traceTargetStateCoverageToleranceMS {
				coverageStatus = "partial_unaccounted"
				unaccountedMS = windowMS - account.TotalMS
			}
		}
		key := strings.Join([]string{
			strings.TrimSpace(projection.ArtifactPath),
			strings.TrimSpace(account.Subject),
			strings.TrimSpace(account.EvidenceID),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, TraceTargetStateScopeAuthority{
			ArtifactLabel:  strings.TrimSpace(projection.ArtifactLabel),
			Subject:        strings.TrimSpace(account.Subject),
			WindowStartTs:  account.WindowStartTs,
			WindowEndTs:    account.WindowEndTs,
			WindowMS:       windowMS,
			RunningMS:      account.RunningMS,
			RunnableMS:     account.RunnableMS,
			SleepMS:        account.SleepMS,
			DStateMS:       account.DStateMS,
			IOWaitMS:       account.IOWaitMS,
			SleepIOWaitMS:  account.SleepIOWaitMS,
			TotalMS:        account.TotalMS,
			UnaccountedMS:  unaccountedMS,
			CoverageStatus: coverageStatus,
			HeadCarryMS:    account.HeadCarryMS,
			HeadCarryState: strings.TrimSpace(account.HeadCarryState),
			TailOpenMS:     account.TailOpenMS,
			TailOpenState:  strings.TrimSpace(account.TailOpenState),
			EvidenceID:     strings.TrimSpace(account.EvidenceID),
		})
	}
	return out
}

// TraceTargetWaitSummaryAuthority is the complete occurrence-level companion
// to TraceTargetStateScopeAuthority. It is compiled only when one deterministic
// trace_query result carries a complete aggregate record plus exactly the
// declared number of same-result occurrence rows. This avoids the prompt's
// bounded eight-row projection without parsing model prose or rebuilding
// scheduler intervals from neighboring events.
type TraceTargetWaitSummaryAuthority struct {
	ArtifactLabel          string
	Subject                string
	WindowStartTs          float64
	WindowEndTs            float64
	Count                  int
	WallClockMS            float64
	DStateOccurrences      int
	IOWaitOccurrences      int
	SleepIOWaitOccurrences int
	OtherWaitOccurrences   int
	Callers                []string
	RecordID               string
}

// BuildTraceTargetWaitSummaryAuthorities returns complete, same-result wait
// summaries for typed user runtime targets. Repeated identical queries
// deduplicate; conflicting summaries for the same artifact/subject/window
// fail closed.
func BuildTraceTargetWaitSummaryAuthorities(ledger ObservationLedger, rm *RequestModel) []TraceTargetWaitSummaryAuthority {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	type candidate struct {
		key         string
		fingerprint string
		authority   TraceTargetWaitSummaryAuthority
	}
	var candidates []candidate
	for _, aggregate := range ledger.Records {
		if aggregate.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(aggregate.Producer) ||
			aggregate.GroundingPolicy != ClaimGroundingHard ||
			strings.TrimSpace(aggregate.Predicate) != "target_window_wait_occurrences" ||
			strings.TrimSpace(aggregate.Object) != "complete" ||
			!ObservationRecordMatchesUserRuntimeTarget(aggregate, rm) ||
			aggregate.ResultCount == nil ||
			aggregate.Span.EndTs <= aggregate.Span.StartTs {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(aggregate.Value))
		if err != nil || count <= 0 || count != *aggregate.ResultCount {
			continue
		}
		scopePrefix, ok := strings.CutSuffix(strings.TrimSpace(aggregate.ID), "#target_window_wait_occurrences")
		if !ok || scopePrefix == "" {
			continue
		}
		rowPrefix := scopePrefix + "#target_window_wait_occurrence:"
		rows := make(map[int]ObservationRecord, count)
		conflict := false
		for _, row := range ledger.Records {
			if !strings.HasPrefix(strings.TrimSpace(row.ID), rowPrefix) ||
				row.Origin != AnswerEvidenceOriginRuntimeArtifact ||
				!RuntimeObservationProducerIsDeterministicQuery(row.Producer) ||
				row.GroundingPolicy != ClaimGroundingHard ||
				strings.TrimSpace(row.Predicate) != "target_window_wait_occurrence" ||
				!strings.EqualFold(strings.TrimSpace(row.Subject), strings.TrimSpace(aggregate.Subject)) ||
				!traceTargetWaitSameResultSource(aggregate, row) {
				continue
			}
			ordinal, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(row.ID), rowPrefix))
			if err != nil || ordinal <= 0 || ordinal > count {
				conflict = true
				break
			}
			if prior, exists := rows[ordinal]; exists {
				if traceTargetWaitOccurrenceFingerprint(prior) != traceTargetWaitOccurrenceFingerprint(row) {
					conflict = true
				}
				continue
			}
			rows[ordinal] = row
		}
		if conflict || len(rows) != count {
			continue
		}
		authority := TraceTargetWaitSummaryAuthority{
			ArtifactLabel: traceTargetStateAuthorityArtifactLabel(aggregate.SourceRef),
			Subject:       strings.TrimSpace(aggregate.Subject),
			WindowStartTs: aggregate.Span.StartTs,
			WindowEndTs:   aggregate.Span.EndTs,
			Count:         count,
			RecordID:      strings.TrimSpace(aggregate.ID),
		}
		callers := map[string]bool{}
		for ordinal := 1; ordinal <= count; ordinal++ {
			row := rows[ordinal]
			duration, err := strconv.ParseFloat(strings.TrimSpace(row.Value), 64)
			if err != nil || duration < 0 || strings.TrimSpace(row.Unit) != "ms" ||
				row.Span.EndTs < row.Span.StartTs ||
				row.Span.StartTs < aggregate.Span.StartTs-0.000002 ||
				row.Span.EndTs > aggregate.Span.EndTs+0.000002 ||
				math.Abs((row.Span.EndTs-row.Span.StartTs)*1000-duration) > 0.002 {
				conflict = true
				break
			}
			fields, ok := traceTargetWaitOccurrenceObjectFields(row.Object)
			if !ok {
				conflict = true
				break
			}
			authority.WallClockMS += duration
			switch {
			case fields["state"] == "d_sleep":
				authority.DStateOccurrences++
			case fields["state"] == "io_wait":
				authority.IOWaitOccurrences++
			case fields["state"] == "s_sleep" && fields["iowait"] == "1":
				authority.SleepIOWaitOccurrences++
			default:
				authority.OtherWaitOccurrences++
			}
			if caller := strings.TrimSpace(fields["caller"]); caller != "" && caller != "unknown" {
				callers[caller] = true
			}
		}
		if conflict {
			continue
		}
		for caller := range callers {
			authority.Callers = append(authority.Callers, caller)
		}
		sort.Strings(authority.Callers)
		key := fmt.Sprintf("%s\x00%s\x00%.6f\x00%.6f",
			strings.ToLower(authority.ArtifactLabel),
			strings.ToLower(authority.Subject),
			authority.WindowStartTs,
			authority.WindowEndTs,
		)
		fingerprint := fmt.Sprintf("%d|%s|%d|%d|%d|%d|%s",
			authority.Count,
			strconv.FormatFloat(authority.WallClockMS, 'g', -1, 64),
			authority.DStateOccurrences,
			authority.IOWaitOccurrences,
			authority.SleepIOWaitOccurrences,
			authority.OtherWaitOccurrences,
			strings.Join(authority.Callers, "\x00"),
		)
		candidates = append(candidates, candidate{key: key, fingerprint: fingerprint, authority: authority})
	}
	byKey := map[string]TraceTargetWaitSummaryAuthority{}
	fingerprints := map[string]string{}
	conflicted := map[string]bool{}
	for _, candidate := range candidates {
		if prior, ok := fingerprints[candidate.key]; ok && prior != candidate.fingerprint {
			conflicted[candidate.key] = true
			continue
		}
		fingerprints[candidate.key] = candidate.fingerprint
		if _, ok := byKey[candidate.key]; !ok {
			byKey[candidate.key] = candidate.authority
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		if !conflicted[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]TraceTargetWaitSummaryAuthority, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func traceTargetWaitSameResultSource(aggregate, row ObservationRecord) bool {
	a, b := aggregate.SourceRef, row.SourceRef
	if a.Kind != b.Kind ||
		strings.TrimSpace(a.ArtifactID) != strings.TrimSpace(b.ArtifactID) ||
		strings.TrimSpace(a.Path) != strings.TrimSpace(b.Path) ||
		strings.TrimSpace(a.PayloadRef) != strings.TrimSpace(b.PayloadRef) ||
		strings.TrimSpace(a.RawRef) != strings.TrimSpace(b.RawRef) {
		return false
	}
	aggregateAt := strings.TrimSpace(aggregate.ObservedAt)
	rowAt := strings.TrimSpace(row.ObservedAt)
	return aggregateAt == "" || rowAt == "" || aggregateAt == rowAt
}

func traceTargetWaitOccurrenceObjectFields(raw string) (map[string]string, bool) {
	fields := map[string]string{}
	for _, token := range strings.Split(strings.TrimSpace(raw), ";") {
		pair := strings.SplitN(strings.TrimSpace(token), "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" {
			return nil, false
		}
		fields[strings.TrimSpace(pair[0])] = strings.TrimSpace(pair[1])
	}
	state := fields["state"]
	iowait := fields["iowait"]
	if state == "" || (iowait != "0" && iowait != "1" && iowait != "unknown") {
		return nil, false
	}
	return fields, true
}

func traceTargetWaitOccurrenceFingerprint(record ObservationRecord) string {
	return fmt.Sprintf("%s|%s|%.9f|%.9f|%s|%s",
		strings.TrimSpace(record.Subject),
		strings.TrimSpace(record.Object),
		record.Span.StartTs,
		record.Span.EndTs,
		strings.TrimSpace(record.Value),
		strings.TrimSpace(record.Unit),
	)
}

func traceTargetStateAuthorityArtifactLabel(ref ObservationSourceRef) string {
	// Match the causal projection's typed artifact identity. Attached traces
	// carry Path=.../attached_trace.txt plus lane marker
	// ArtifactID=attached_trace; preferring the marker here made occurrence
	// rows from the same result fail to pair with the projection state card.
	if _, label, _ := traceCausalProjectionArtifactIdentity(ObservationRecord{SourceRef: ref}); label != "" {
		return label
	}
	return strings.TrimSpace(ref.ArtifactID)
}
