package tracequery

import (
	"math"
	"sort"
)

type frameMatchPointGroup struct {
	start uint64
	names map[string]bool
}

func frameMatchPointGroups(catalog FrameCatalog, tid int) []frameMatchPointGroup {
	const maxGapNS, maxGroupNS = 8_000_000, 20_000_000
	type call struct {
		name       string
		start, end uint64
	}
	var calls []call
	counts := make(map[string]int)
	for _, point := range catalog.PointSpans {
		if point.Role == "render" || point.TID != tid || point.EndNS <= point.StartNS || point.EndNS-point.StartNS > maxGroupNS {
			continue
		}
		name := frameMatchPoint(point.Name)
		if name == "jank_marker" {
			continue
		}
		calls = append(calls, call{name, point.StartNS, point.EndNS})
		counts[name]++
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].start < calls[j].start })
	var groups []frameMatchPointGroup
	group := frameMatchPointGroup{}
	var previous uint64
	for _, c := range calls {
		if counts[c.name] > 3 {
			continue
		}
		if len(group.names) != 0 && (c.start-previous > maxGapNS || c.end-group.start > maxGroupNS) {
			if len(group.names) >= 3 {
				groups = append(groups, group)
			}
			group = frameMatchPointGroup{}
		}
		if group.names == nil {
			group = frameMatchPointGroup{start: c.start, names: make(map[string]bool)}
		}
		group.names[c.name] = true
		previous = c.start
	}
	if len(group.names) >= 3 {
		groups = append(groups, group)
	}
	return groups
}

// The trace clock only enters as a difference from the candidate frame. A
// matching episode is a group of several observations with consistent shifts.
func frameMatchEpisode(groups []frameMatchPointGroup, center uint64) map[string]float64 {
	out := make(map[string]float64)
	first := sort.Search(len(groups), func(i int) bool {
		return groups[i].start >= center || center-groups[i].start <= 160_000_000
	})
	for _, group := range groups[first:] {
		if group.start > center && group.start-center > 85_000_000 {
			break
		}
		offset := (float64(group.start) - float64(center)) / 1e6
		for name := range group.names {
			if old, ok := out[name]; !ok || math.Abs(offset) < math.Abs(old) {
				out[name] = offset
			}
		}
	}
	return out
}

func frameMatchEpisodeSimilarity(a, b map[string]float64) (float64, bool) {
	var deltas []float64
	for key, x := range a {
		if y, ok := b[key]; ok {
			deltas = append(deltas, x-y)
		}
	}
	if len(deltas) < 3 {
		return 0, false
	}
	shift := medianFrameMatchValues(deltas)
	coherent := 0
	for _, d := range deltas {
		if math.Abs(d-shift) <= 20 {
			coherent++
		}
	}
	if coherent < 3 {
		return 0, true
	}
	return 2 * float64(coherent) / float64(len(a)+len(b)) / (1 + math.Abs(shift)/50), true
}

func applyFrameMatchEpisodeSupport(candidates []FrameMatchCandidate) {
	values := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		values = append(values, candidate.Evidence["point_episode"])
	}
	if len(values) == 0 {
		return
	}
	background := medianFrameMatchValues(values)
	for i := range candidates {
		c := &candidates[i]
		if support := c.Evidence["point_episode"] - background; support > 0 {
			c.Score += (1 - c.Score) * .20 * support
			c.Evidence["point_episode_support"] = support
		}
	}
}
