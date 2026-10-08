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
	for len(p.Events) > 0 {
		data, err := json.Marshal(p)
		if err == nil && len(data) <= resourceStackHandoffByteBudget && len(TraceResourceStackText(p, 8)) <= resourceStackHandoffByteBudget {
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
