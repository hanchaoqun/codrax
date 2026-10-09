package tracequery

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
)

func ValidPreferredFrameRate(p PreferredFrameRateResult) bool {
	if p.SourcePath == "" || p.TargetPID < 0 || p.TargetScope != TargetScopeProcess || p.TotalSeries < 0 || p.OmittedSeries < 0 || p.TotalSeries != len(p.Series)+p.OmittedSeries || len(p.Series) > PreferredFrameRateSeriesLimit || p.TotalObservations < 0 || p.OmittedObservations < 0 || p.TotalObservations != len(p.Observations)+p.OmittedObservations || len(p.Observations) > ProcessMeasurementsLimit || p.UnpositionedRows < 0 {
		return false
	}
	if p.Status == "unavailable" {
		return p.TotalSeries == 0 && p.TotalObservations == 0 && p.UnpositionedRows == 0
	}
	start, sok := processMeasurementNS(p.Window.StartTs)
	end, eok := processMeasurementNS(p.Window.EndTs)
	if p.Status != "available" || !sok || !eok || end <= start || start < 0 && end > math.MaxInt64+start || p.Window.EndInclusive {
		return false
	}
	seriesSeen := map[string]bool{}
	for _, s := range p.Series {
		identity, _ := json.Marshal([]any{s.SourcePath, s.IPID, s.FilterID, s.PID, s.OwnerStatus, s.UnknownOwnerRowID})
		if seriesSeen[string(identity)] {
			return false
		}
		seriesSeen[string(identity)] = true
		if s.SourcePath == "" || s.Observations < 0 || s.UnpositionedRows < 0 || s.TotalIntervals != len(s.Timeline)+s.OmittedIntervals || s.OmittedIntervals < 0 || len(s.Timeline) > PreferredFrameRateDetailLimit || s.TotalDistribution != len(s.Distribution)+s.OmittedDistribution || s.OmittedDistribution < 0 || len(s.Distribution) > PreferredFrameRateDetailLimit {
			return false
		}
		if s.OwnerStatus == "known" {
			if s.PID == nil || *s.PID <= 0 || s.UnknownOwnerRowID != "" || p.TargetPID > 0 && *s.PID != p.TargetPID {
				return false
			}
		} else {
			if (s.OwnerStatus != "unknown" && s.OwnerStatus != "ambiguous" && s.OwnerStatus != "outside_lifetime") || s.PID != nil || s.UnknownOwnerRowID == "" || p.TargetPID > 0 || s.ProcessName != "" {
				return false
			}
		}
		remaining := end - start
		for _, n := range []int64{s.KnownDurationNS, s.ConflictDurationNS, s.UnknownValueDurationNS, s.UnobservedDurationNS} {
			if n < 0 || n > remaining {
				return false
			}
			remaining -= n
		}
		if remaining != 0 {
			return false
		}
		known := int64(0)
		rates := map[string]bool{}
		for _, d := range s.Distribution {
			r, ok := new(big.Rat).SetString(d.RateHz)
			if !ok || r.Sign() <= 0 || rates[d.RateHz] || d.DurationNS <= 0 || d.DurationNS > s.KnownDurationNS-known || d.Intervals <= 0 || !isFiniteTraceNumber(d.WindowPercent) || math.Abs(d.WindowPercent-float64(d.DurationNS)/float64(end-start)*100) > 1e-9 {
				return false
			}
			rates[d.RateHz] = true
			known += d.DurationNS
		}
		if s.OmittedDistribution == 0 && known != s.KnownDurationNS {
			return false
		}
		prev := start
		stateTime := map[string]int64{}
		rateTime := map[string]int64{}
		for _, r := range s.Timeline {
			if r.StartNS != prev || r.EndNS <= r.StartNS || r.EndNS > end || r.TotalSourceLines != len(r.SourceLines)+r.OmittedSourceLines || r.OmittedSourceLines < 0 || len(r.SourceLines) > 1 {
				return false
			}
			prev = r.EndNS
			stateTime[r.State] += r.EndNS - r.StartNS
			for _, line := range r.SourceLines {
				if line <= 0 {
					return false
				}
			}
			switch r.State {
			case "known":
				v, err := strconv.ParseFloat(r.RateHz, 64)
				if err != nil || !isFiniteTraceNumber(v) || v <= 0 || r.TotalSourceLines <= 0 {
					return false
				}
				rateTime[r.RateHz] += r.EndNS - r.StartNS
			case "conflict", "unknown_value":
				if r.RateHz != "" || r.TotalSourceLines <= 0 {
					return false
				}
			case "unobserved":
				if r.RateHz != "" || r.TotalSourceLines != 0 {
					return false
				}
			default:
				return false
			}
		}
		if s.OmittedIntervals == 0 && prev != end {
			return false
		}
		for state, total := range map[string]int64{"known": s.KnownDurationNS, "conflict": s.ConflictDurationNS, "unknown_value": s.UnknownValueDurationNS, "unobserved": s.UnobservedDurationNS} {
			if stateTime[state] > total || s.OmittedIntervals == 0 && stateTime[state] != total {
				return false
			}
		}
		for _, d := range s.Distribution {
			if rateTime[d.RateHz] > d.DurationNS || s.OmittedIntervals == 0 && rateTime[d.RateHz] != d.DurationNS {
				return false
			}
			delete(rateTime, d.RateHz)
		}
		if s.OmittedDistribution == 0 && len(rateTime) > 0 {
			return false
		}
	}
	seen := map[int64]bool{}
	for _, r := range p.Observations {
		if !r.Record.Valid() || !r.Record.NameKnown || r.Record.Name != "H:PreferredFrameRate" || r.SourcePath == "" || r.Line <= 0 || r.SourceLine <= 0 || seen[r.Record.RowID] {
			return false
		}
		seen[r.Record.RowID] = true
		if p.TargetPID > 0 && (r.Record.PID == nil || *r.Record.PID != p.TargetPID) {
			return false
		}
		selection, left, right := processMeasurementSelection(r.Record, start, end, false)
		if selection == "" || selection != r.Selection || !equalProcessMeasurementPointer(left, r.ClippedStartNS) || !equalProcessMeasurementPointer(right, r.ClippedEndNS) {
			return false
		}
	}
	return true
}
