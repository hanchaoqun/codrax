package tracequery

import (
	"math"
	"sort"
)

type frameMatchLandmark struct {
	currentMS, baselineMS float64
	points                int
}

// frameMatchLandmarks pairs unique point names on the application thread, then
// corroborates them as short groups present on both clocks. A monotonic chain
// of groups anchors operation progress without using capture start or frame IDs.
func frameMatchLandmarks(current, baseline FrameCatalog, currentTID, baselineTID int) []frameMatchLandmark {
	left, right := uniqueFrameLandmarkPoints(current, currentTID), uniqueFrameLandmarkPoints(baseline, baselineTID)
	type pointPair struct {
		currentMS, baselineMS float64
		name                  string
	}
	var pairs []pointPair
	for name, currentMS := range left {
		if baselineMS, ok := right[name]; ok {
			pairs = append(pairs, pointPair{currentMS, baselineMS, name})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		a, b := pairs[i], pairs[j]
		if a.currentMS != b.currentMS {
			return a.currentMS < b.currentMS
		}
		if a.baselineMS != b.baselineMS {
			return a.baselineMS < b.baselineMS
		}
		return a.name < b.name
	})
	const groupMS = 20
	var groups []frameMatchLandmark
	for head := 0; head < len(pairs); {
		start := pairs[head]
		end := head
		var currentTimes, baselineTimes []float64
		for end < len(pairs) && pairs[end].currentMS-start.currentMS <= groupMS {
			pair := pairs[end]
			if math.Abs(pair.baselineMS-start.baselineMS) <= groupMS {
				currentTimes = append(currentTimes, pair.currentMS)
				baselineTimes = append(baselineTimes, pair.baselineMS)
			}
			end++
		}
		if len(currentTimes) < 3 {
			head++
			continue
		}
		groups = append(groups, frameMatchLandmark{medianFrameMatchValues(currentTimes), medianFrameMatchValues(baselineTimes), len(currentTimes)})
		head = end
	}
	return frameMatchLandmarkChain(groups, groupMS)
}

func uniqueFrameLandmarkPoints(catalog FrameCatalog, tid int) map[string]float64 {
	points := make(map[string]float64)
	repeated := make(map[string]bool)
	for _, point := range catalog.PointSpans {
		if point.TID != tid || point.Role == "render" || point.EndNS <= point.StartNS {
			continue
		}
		name := frameMatchPoint(point.Name)
		if name == "jank_marker" || repeated[name] {
			continue
		}
		if _, exists := points[name]; exists {
			delete(points, name)
			repeated[name] = true
		} else {
			points[name] = float64(point.StartNS) / 1e6
		}
	}
	return points
}

func frameMatchLandmarkChain(groups []frameMatchLandmark, separationMS float64) []frameMatchLandmark {
	if len(groups) == 0 {
		return nil
	}
	previous, support := make([]int, len(groups)), make([]int, len(groups))
	best := 0
	for i, group := range groups {
		previous[i] = -1
		support[i] = group.points
		for j, earlier := range groups[:i] {
			weight := support[j] + group.points
			if group.currentMS-earlier.currentMS >= separationMS &&
				group.baselineMS-earlier.baselineMS >= separationMS && weight > support[i] {
				previous[i], support[i] = j, weight
			}
		}
		if support[i] > support[best] {
			best = i
		}
	}
	var chain []frameMatchLandmark
	for i := best; i >= 0; i = previous[i] {
		chain = append(chain, groups[i])
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain
}

func frameMatchLandmarkProjection(chain []frameMatchLandmark, currentMS float64) (baselineMS, scale float64, ok bool) {
	at := sort.Search(len(chain), func(i int) bool { return chain[i].currentMS > currentMS })
	if at == 0 || at == len(chain) {
		return 0, 0, false
	}
	left, right := chain[at-1], chain[at]
	currentWidth, baselineWidth := right.currentMS-left.currentMS, right.baselineMS-left.baselineMS
	ratio := baselineWidth / currentWidth
	if currentWidth > 2000 || ratio < .25 || ratio > 4 {
		return 0, 0, false
	}
	return left.baselineMS + (currentMS-left.currentMS)*ratio, math.Max(50, .15*baselineWidth), true
}

func rerankByFrameMatchLandmarks(candidates []FrameMatchCandidate, chain []frameMatchLandmark, targetStartNS uint64) {
	expectedMS, scale, ok := frameMatchLandmarkProjection(chain, float64(targetStartNS)/1e6)
	if !ok {
		return
	}
	// Keep local behavioral matching as the primary stage. Only revisit its
	// shortlist when observed groups bracket the target on both captures.
	shortlist := candidates[:min(len(candidates), 10)]
	for i := range shortlist {
		c := &shortlist[i]
		residualMS := float64(c.Frame.StartNS)/1e6 - expectedMS
		position := math.Exp(-math.Abs(residualMS) / scale)
		c.Evidence["landmark_position"] = position
		c.Evidence["landmark_residual_ms"] = residualMS
		c.Evidence["score_before_landmarks"] = c.Score
		c.Score = .85*c.Score + .15*position
	}
	sort.SliceStable(shortlist, func(i, j int) bool { return shortlist[i].Score > shortlist[j].Score })
}
