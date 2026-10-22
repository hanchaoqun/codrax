package tool

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const resourceStackHandoffByteBudget = 24 << 10

// Remove complete display frames/events, never truncate a source symbol into
// a different apparent name. The original payload and full census stay intact.
func boundedResourceStackHandoff(p tracequery.ResourceStackResult) tracequery.ResourceStackResult {
	p.Events = append([]tracequery.ResourceStackEvent(nil), p.Events...)
	if p.EventStatistics != nil {
		stats := *p.EventStatistics
		stats.Groups = append([]tracequery.ResourceEventStatistic(nil), stats.Groups...)
		p.EventStatistics = &stats
		// Independent whole-group allowance leaves room for the pre-existing
		// event/frame preview. The full query payload is not mutated.
		used, keep := 0, 0
		for _, group := range stats.Groups {
			encoded, err := json.Marshal(group)
			if err != nil || len(encoded) > (resourceStackHandoffByteBudget/3)-used {
				break
			}
			used += len(encoded)
			keep++
		}
		for len(stats.Groups) > keep {
			omitResourceStatisticGroup(&stats)
		}
	}
	if len(p.Events) > 8 {
		p.OmittedEvents += len(p.Events) - 8
		p.Events = p.Events[:8]
	}
	for i := range p.Events {
		e := &p.Events[i]
		if len(e.Frames) > 32 {
			e.OmittedFrames += len(e.Frames) - 32
			e.Frames = e.Frames[:32]
		}
	}
	for {
		data, err := json.Marshal(p)
		if err == nil && len(data) <= resourceStackHandoffByteBudget && len(TraceResourceStackText(p, 8)) <= resourceStackHandoffByteBudget {
			break
		}
		if p.EventStatistics != nil && len(p.EventStatistics.Groups) > 0 {
			omitResourceStatisticGroup(p.EventStatistics)
			continue
		}
		if len(p.Events) == 0 {
			break
		}
		last := &p.Events[len(p.Events)-1]
		if len(last.Frames) > 0 {
			last.Frames = last.Frames[:len(last.Frames)-1]
			last.OmittedFrames++
		} else {
			p.Events = p.Events[:len(p.Events)-1]
			p.OmittedEvents++
		}
	}
	return p
}

func omitResourceStatisticGroup(s *tracequery.ResourceEventStatistics) {
	last := s.Groups[len(s.Groups)-1]
	s.Groups = s.Groups[:len(s.Groups)-1]
	s.OmittedGroups++
	s.OmittedGroupEvents += last.EventCount
	s.Complete = false
}
