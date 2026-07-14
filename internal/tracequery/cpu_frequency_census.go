package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

// CPU frequency census lane (RFC #71, real_trace_campaign_20260705 §8.2 c4).
//
// Attribution finding: event_search(event_types=cpu_frequency) returns the
// first `limit` CHRONOLOGICAL matches only. On the c4 trace the six 807000kHz
// rows all sat after the 40-row cut, so the model enumerated 9 of 11 tiers
// off the truncated display face and published a wrong frequency range with
// high confidence — despite a correct caveat AND a typed refine hint, which
// are soft signals a model may ignore. This lane is the deterministic
// complement: the SAME match pass that feeds the display rows also aggregates
// the FULL pre-truncation tier ladder (distinct kHz tiers + per-tier row
// counts + cpu set), so range/boundary/exhaustiveness claims never depend on
// the model consuming a refine hint.
//
// Additive-only discipline (WSR #70 process_domain_census precedent): the
// census is nil whenever the display face already shows every matched
// cpu_frequency row — non-truncated results stay byte-identical.

// CPUFrequencyCensusTier is one distinct frequency tier observed among ALL
// matched cpu_frequency rows (pre-truncation), with its row count and the
// cpu_id set that reported it.
type CPUFrequencyCensusTier struct {
	FrequencyKHz int   `json:"frequency_khz"`
	Rows         int   `json:"rows"`
	CPUs         []int `json:"cpus,omitempty"`
}

// CPUFrequencyCensus is the full-window frequency tier ladder aggregated from
// every matched cpu_frequency row BEFORE the chronological display
// truncation. Tiers are ascending by kHz and the tier list is exhaustive for
// the queried window/filters — unlike the Events face, which is capped.
type CPUFrequencyCensus struct {
	// MatchedFrequencyRows counts every cpu_frequency row (Frequency>0) the
	// query matched in the full scan; DisplayedFrequencyRows counts how many
	// of those survived the chronological display cap.
	MatchedFrequencyRows   int `json:"matched_frequency_rows"`
	DisplayedFrequencyRows int `json:"displayed_frequency_rows"`
	// FrequencyLimitRows counts cpu_frequency_limits rows admitted by the
	// compatibility matcher (they carry min/max policy bounds, not ladder
	// samples, so they are excluded from Tiers).
	FrequencyLimitRows int                      `json:"frequency_limit_rows,omitempty"`
	CPUs               []int                    `json:"cpus,omitempty"`
	Tiers              []CPUFrequencyCensusTier `json:"tiers"`
	MinKHz             int                      `json:"min_khz"`
	MaxKHz             int                      `json:"max_khz"`
	LineStart          int                      `json:"line_start,omitempty"`
	LineEnd            int                      `json:"line_end,omitempty"`
	Summary            string                   `json:"summary,omitempty"`
}

// cpuFrequencyCensusAcc accumulates the census in the SAME pass that decides
// display admission, so stream and indexed twins cannot drift on the match
// predicate. O(distinct tiers) memory — safe for GB streaming scans.
type cpuFrequencyCensusAcc struct {
	rows      map[int]int          // kHz → matched row count
	tierCPUs  map[int]map[int]bool // kHz → cpu_id set
	cpus      map[int]bool
	limitRows int
	matched   int
	lineStart int
	lineEnd   int
}

// newCPUFrequencyCensusAcc arms the accumulator only when the query
// explicitly asked for the cpu_frequency event family — the census is the
// frequency-enumeration face's truth row, not a general event census.
func newCPUFrequencyCensusAcc(typeSet map[EventType]bool) *cpuFrequencyCensusAcc {
	if !typeSet[EventCPUFrequency] {
		return nil
	}
	return &cpuFrequencyCensusAcc{
		rows:     map[int]int{},
		tierCPUs: map[int]map[int]bool{},
		cpus:     map[int]bool{},
	}
}

// observe records one MATCHED event (called before any limit cut).
func (a *cpuFrequencyCensusAcc) observe(ev Event) {
	if a == nil {
		return
	}
	switch ev.Type {
	case EventCPUFrequencyLimit:
		if _, ok := isPerCPULimitSample(ev); ok {
			a.limitRows++
		}
	case EventCPUFrequency:
		if !isPerCPUFrequencySample(ev) {
			return
		}
		a.matched++
		a.rows[ev.Frequency]++
		cpu := eventCPUForStats(ev)
		if cpu >= 0 {
			a.cpus[cpu] = true
			set := a.tierCPUs[ev.Frequency]
			if set == nil {
				set = map[int]bool{}
				a.tierCPUs[ev.Frequency] = set
			}
			set[cpu] = true
		}
		if a.lineStart == 0 || (ev.Line > 0 && ev.Line < a.lineStart) {
			a.lineStart = ev.Line
		}
		if ev.Line > a.lineEnd {
			a.lineEnd = ev.Line
		}
	}
}

// finalize builds the census against the DISPLAYED rows. Nil when the display
// face already carries every matched cpu_frequency row — the census only
// exists to undo a truncation, never to restate a complete face (keeps
// non-truncated results byte-identical).
func (a *cpuFrequencyCensusAcc) finalize(displayed []EventView) *CPUFrequencyCensus {
	if a == nil || a.matched == 0 {
		return nil
	}
	shown := 0
	for _, ev := range displayed {
		if isPerCPUFrequencySample(ev.Event) {
			shown++
		}
	}
	if a.matched <= shown {
		return nil
	}
	census := &CPUFrequencyCensus{
		MatchedFrequencyRows:   a.matched,
		DisplayedFrequencyRows: shown,
		FrequencyLimitRows:     a.limitRows,
		CPUs:                   sortedIntSet(a.cpus),
		LineStart:              a.lineStart,
		LineEnd:                a.lineEnd,
	}
	census.Tiers = make([]CPUFrequencyCensusTier, 0, len(a.rows))
	for khz, rows := range a.rows {
		census.Tiers = append(census.Tiers, CPUFrequencyCensusTier{
			FrequencyKHz: khz,
			Rows:         rows,
			CPUs:         sortedIntSet(a.tierCPUs[khz]),
		})
	}
	sort.Slice(census.Tiers, func(i, j int) bool {
		return census.Tiers[i].FrequencyKHz < census.Tiers[j].FrequencyKHz
	})
	census.MinKHz = census.Tiers[0].FrequencyKHz
	census.MaxKHz = census.Tiers[len(census.Tiers)-1].FrequencyKHz
	census.Summary = fmt.Sprintf(
		"full-window census of all %d matched cpu_frequency row(s) (display shows first %d chronological row(s) only): %d distinct tier(s) %d..%dkHz across cpus %s; tier ladder is exhaustive for the queried window/filters",
		census.MatchedFrequencyRows, census.DisplayedFrequencyRows, len(census.Tiers),
		census.MinKHz, census.MaxKHz, joinInts(census.CPUs))
	return census
}

// ComputeCPUFrequencyCensus is the indexed twin of the streaming inline
// accumulation: one in-memory scan over idx.Events with the SAME admission
// predicate as EventSearch (eventInQuery), minus the display limit.
func ComputeCPUFrequencyCensus(idx *Index, q Query, displayed []EventView) *CPUFrequencyCensus {
	if idx == nil {
		return nil
	}
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, t := range q.EventTypes {
		if t != "" {
			typeSet[t] = true
		}
	}
	actionSet := traceMarkActionFilterSet(q.TraceMarkActions)
	acc := newCPUFrequencyCensusAcc(typeSet)
	if acc == nil {
		return nil
	}
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			// SUPP-CANCEL: a truncated census is a claim-of-absence hazard;
			// abandon whole (the caller's attach gate discards it).
			return nil
		}
		if !eventInQuery(ev, q, typeSet, actionSet) {
			continue
		}
		acc.observe(ev)
	}
	return acc.finalize(displayed)
}

// EvidenceFact projects the census onto the evidence-pack lane so the true
// tier ladder rides into the typed observation face (evidence_fact records)
// alongside the truncated per-row facts — the answer-side numeric face gets
// the boundary truth deterministically. Prepended by the census producers so
// the display/record caps can never fold it away.
func (c *CPUFrequencyCensus) EvidenceFact() EvidenceFact {
	return EvidenceFact{
		Subject:    "cpu_frequency",
		Predicate:  "frequency_tier_census",
		Object:     fmt.Sprintf("%d..%dkHz", c.MinKHz, c.MaxKHz),
		Summary:    c.Summary + "; tiers khz×rows=" + FormatCPUFrequencyCensusTiers(c.Tiers, cpuFrequencyCensusTierDisplayCap),
		LineStart:  c.LineStart,
		LineEnd:    c.LineEnd,
		Confidence: 0.85,
	}
}

// cpuFrequencyCensusTierDisplayCap bounds the compact tier listing. OPP
// ladders are hardware-bounded (typically ≤20 per cluster), so the cap is a
// defensive width guard, not an expected fold.
const cpuFrequencyCensusTierDisplayCap = 24

// FormatCPUFrequencyCensusTiers renders the compact ascending "khz×rows"
// listing shared by the banner row and the evidence fact (single home so the
// two faces cannot drift). A defensive over-cap fold keeps BOTH ladder
// endpoints — the census exists for boundary claims, so min/max tiers must
// survive any fold.
func FormatCPUFrequencyCensusTiers(tiers []CPUFrequencyCensusTier, cap int) string {
	if len(tiers) == 0 {
		return ""
	}
	format := func(t CPUFrequencyCensusTier) string {
		return fmt.Sprintf("%d×%d", t.FrequencyKHz, t.Rows)
	}
	if cap <= 2 || len(tiers) <= cap {
		parts := make([]string, 0, len(tiers))
		for _, t := range tiers {
			parts = append(parts, format(t))
		}
		return strings.Join(parts, ",")
	}
	head := cap / 2
	tail := cap - head - 1
	parts := make([]string, 0, cap+1)
	for _, t := range tiers[:head] {
		parts = append(parts, format(t))
	}
	parts = append(parts, fmt.Sprintf("(+%d mid tier(s) folded; ladder endpoints retained)", len(tiers)-head-tail))
	for _, t := range tiers[len(tiers)-tail:] {
		parts = append(parts, format(t))
	}
	return strings.Join(parts, ",")
}

func joinInts(in []int) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in))
	for _, v := range in {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, ",")
}
