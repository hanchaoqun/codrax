package tracequery

import (
	"math"
	"sort"
)

type frameMatchTrajectory struct {
	before, after int
	slope         float64
	hasSlope      bool
}

// The trajectory uses a named point's own maximum closed invocation in each
// consecutive frame. Missing spans do not prove where the run started or ended.
func frameMatchTrajectories(rows []frameMatchObservation, center int) map[string]frameMatchTrajectory {
	const radius = 4
	minIndex, maxIndex := max(0, center-radius), min(len(rows)-1, center+radius)
	out := make(map[string]frameMatchTrajectory)
	for point := range rows[center].durations {
		first, last := center, center
		for first > minIndex && rows[first-1].durations[point] > 0 {
			first--
		}
		for last < maxIndex && rows[last+1].durations[point] > 0 {
			last++
		}
		if first == last {
			continue
		}
		trajectory := frameMatchTrajectory{before: -1, after: -1}
		if first > minIndex && len(rows[first-1].durations) != 0 {
			trajectory.before = center - first
		}
		if last < maxIndex && len(rows[last+1].durations) != 0 {
			trajectory.after = last - center
		}
		if trajectory.before < 0 && trajectory.after < 0 {
			continue
		}
		if last-first >= 2 {
			var slopes []float64
			for i := first; i < last; i++ {
				for j := i + 1; j <= last; j++ {
					slope := math.Log1p(rows[j].durations[point]) - math.Log1p(rows[i].durations[point])
					slopes = append(slopes, slope/float64(j-i))
				}
			}
			trajectory.slope, trajectory.hasSlope = medianFrameMatchValues(slopes), true
		}
		out[point] = trajectory
	}
	return out
}

func frameMatchTrajectorySimilarity(a, b map[string]frameMatchTrajectory) (float64, bool) {
	var best, next float64
	observed := false
	boundary := func(x, y int) float64 {
		if x < 0 || y < 0 {
			return 0
		}
		observed = true
		return math.Max(0, 1-math.Abs(float64(x-y))/3)
	}
	for point, left := range a {
		right, ok := b[point]
		if !ok {
			continue
		}
		before, after := boundary(left.before, right.before), boundary(left.after, right.after)
		stronger := math.Max(before, after)
		value := .65*stronger + .20*math.Min(before, after)
		if left.hasSlope && right.hasSlope && left.slope*right.slope > 0 {
			value += .15 * stronger / (1 + math.Abs(left.slope-right.slope)/.20)
		}
		if value > best {
			best, next = value, best
		} else {
			next = math.Max(next, value)
		}
	}
	return best + .15*(1-best)*next, observed
}

func applyFrameMatchTrajectorySupport(candidates []FrameMatchCandidate) {
	var seen []float64
	for _, candidate := range candidates {
		if affinity, ok := candidate.Evidence["trajectory"]; ok {
			seen = append(seen, affinity)
		}
	}
	if len(seen) < 2 {
		return
	}
	background := medianFrameMatchValues(seen)
	for i := range candidates {
		c := &candidates[i]
		if support := c.Evidence["trajectory"] - background; support > 0 {
			c.Score += (1 - c.Score) * .06 * support
			c.Evidence["trajectory_support"] = support
		}
	}
}

func medianFrameMatchValues(values []float64) float64 {
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 0 {
		return (values[mid-1] + values[mid]) / 2
	}
	return values[mid]
}
