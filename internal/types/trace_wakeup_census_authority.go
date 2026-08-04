package types

import (
	"sort"
	"strconv"
	"strings"
)

// TraceTargetWakeupCensusPairAuthority is one complete-window waker -> target
// pair carried by a trace_query wakeup_edge_census record.
type TraceTargetWakeupCensusPairAuthority struct {
	Waker          string
	Count          int
	SleepExitCount int
	DExitCount     int
	OtherExitCount int
	SplitAvailable bool
	FirstTimestamp string
	LastTimestamp  string
	SourceRecordID string
}

// TraceTargetWakeupCensusAuthority is the target-wakee, cap-immune census for
// one trace_query result. Complete means every published target pair was
// consumed without a duplicate/count or exit-partition contradiction.
type TraceTargetWakeupCensusAuthority struct {
	Scope          string
	Target         string
	Window         string
	Complete       bool
	TotalCount     int
	SleepExitCount int
	DExitCount     int
	OtherExitCount int
	SplitAvailable bool
	Pairs          []TraceTargetWakeupCensusPairAuthority
}

// BuildTraceTargetWakeupCensusAuthorities compiles the target-only wakeup
// inventory from typed trace_query records. It never reads request or answer
// prose. The producer's target_wakee marker is the completeness credential:
// target pairs are immune to both the engine pair cap and publication row cap.
func BuildTraceTargetWakeupCensusAuthorities(ledger ObservationLedger) []TraceTargetWakeupCensusAuthority {
	type group struct {
		authority TraceTargetWakeupCensusAuthority
		pairs     map[string]TraceTargetWakeupCensusPairAuthority
	}
	groups := map[string]*group{}
	order := make([]string, 0)

	for _, record := range ledger.Records {
		if !RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			strings.TrimSpace(record.Predicate) != "wakeup_edge_census" ||
			!traceObservationRichNoteBool(record.RichNotes, TraceNoteKeyWakeupEdgeCensusTargetWakee) {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(record.Value))
		if err != nil || count <= 0 {
			continue
		}
		scope := traceWakeupCensusRecordScope(record.ID)
		window := traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySelectedWindow)
		target := strings.TrimSpace(record.Object)
		key := strings.Join([]string{scope, target, window}, "\x00")
		g := groups[key]
		if g == nil {
			g = &group{
				authority: TraceTargetWakeupCensusAuthority{
					Scope:    scope,
					Target:   target,
					Window:   window,
					Complete: true,
				},
				pairs: map[string]TraceTargetWakeupCensusPairAuthority{},
			}
			groups[key] = g
			order = append(order, key)
		}

		pair := TraceTargetWakeupCensusPairAuthority{
			Waker:          strings.TrimSpace(record.Subject),
			Count:          count,
			FirstTimestamp: traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupEdgeCensusFirstTs),
			LastTimestamp:  traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupEdgeCensusLastTs),
			SourceRecordID: strings.TrimSpace(record.ID),
		}
		sleepRaw := traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupEdgeCensusSleepExit)
		dRaw := traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupEdgeCensusDExit)
		otherRaw := traceObservationRichNoteValue(record.RichNotes, TraceNoteKeyWakeupEdgeCensusOtherExit)
		pair.SplitAvailable = sleepRaw != "" || dRaw != "" || otherRaw != ""
		if pair.SplitAvailable {
			var ok bool
			if pair.SleepExitCount, ok = traceWakeupCensusOptionalCount(sleepRaw); !ok {
				g.authority.Complete = false
			}
			if pair.DExitCount, ok = traceWakeupCensusOptionalCount(dRaw); !ok {
				g.authority.Complete = false
			}
			if pair.OtherExitCount, ok = traceWakeupCensusOptionalCount(otherRaw); !ok {
				g.authority.Complete = false
			}
			if pair.SleepExitCount+pair.DExitCount+pair.OtherExitCount != pair.Count {
				g.authority.Complete = false
			}
		}

		pairKey := pair.Waker + "\x00" + target
		if prior, exists := g.pairs[pairKey]; exists {
			if prior != pair {
				g.authority.Complete = false
			}
			continue
		}
		g.pairs[pairKey] = pair
		g.authority.Pairs = append(g.authority.Pairs, pair)
	}

	out := make([]TraceTargetWakeupCensusAuthority, 0, len(order))
	seenAuthorities := map[string]bool{}
	for _, key := range order {
		g := groups[key]
		sort.SliceStable(g.authority.Pairs, func(i, j int) bool {
			if g.authority.Pairs[i].Count != g.authority.Pairs[j].Count {
				return g.authority.Pairs[i].Count > g.authority.Pairs[j].Count
			}
			return g.authority.Pairs[i].Waker < g.authority.Pairs[j].Waker
		})
		allSplit := len(g.authority.Pairs) > 0
		for _, pair := range g.authority.Pairs {
			g.authority.TotalCount += pair.Count
			if !pair.SplitAvailable {
				allSplit = false
				continue
			}
			g.authority.SleepExitCount += pair.SleepExitCount
			g.authority.DExitCount += pair.DExitCount
			g.authority.OtherExitCount += pair.OtherExitCount
		}
		g.authority.SplitAvailable = allSplit
		identity := traceTargetWakeupCensusAuthorityIdentity(g.authority)
		if seenAuthorities[identity] {
			continue
		}
		seenAuthorities[identity] = true
		out = append(out, g.authority)
	}
	return out
}

// traceTargetWakeupCensusAuthorityIdentity excludes query-result scope and
// source record IDs deliberately: repeated exploration/supplement queries can
// prove one identical target/window census. Only the exact typed roster,
// totals, split and timestamps collapse; any semantic disagreement remains a
// separate authority instead of being guessed away.
func traceTargetWakeupCensusAuthorityIdentity(authority TraceTargetWakeupCensusAuthority) string {
	var b strings.Builder
	fmtFields := []string{
		authority.Target, authority.Window, strconv.FormatBool(authority.Complete),
		strconv.Itoa(authority.TotalCount), strconv.Itoa(authority.SleepExitCount),
		strconv.Itoa(authority.DExitCount), strconv.Itoa(authority.OtherExitCount),
		strconv.FormatBool(authority.SplitAvailable),
	}
	b.WriteString(strings.Join(fmtFields, "\x00"))
	for _, pair := range authority.Pairs {
		b.WriteByte('\x01')
		b.WriteString(strings.Join([]string{
			pair.Waker, strconv.Itoa(pair.Count), strconv.Itoa(pair.SleepExitCount),
			strconv.Itoa(pair.DExitCount), strconv.Itoa(pair.OtherExitCount),
			strconv.FormatBool(pair.SplitAvailable), pair.FirstTimestamp, pair.LastTimestamp,
		}, "\x00"))
	}
	return b.String()
}

func traceWakeupCensusRecordScope(id string) string {
	id = strings.TrimSpace(id)
	if before, _, ok := strings.Cut(id, "#wakeup_edge_census:"); ok {
		return before
	}
	return id
}

func traceWakeupCensusOptionalCount(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil && value >= 0
}
