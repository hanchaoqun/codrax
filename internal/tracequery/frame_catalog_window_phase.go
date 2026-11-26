package tracequery

import "strings"

type frameWindowUpdate struct {
	window     string
	generation int
	parameters int // Zero is the initial layout of this window generation.
}

func frameWindowUpdateMarker(name string) (window string, initial, ok bool) {
	name, ok = strings.CutPrefix(name, "VRI[")
	if !ok {
		return "", false, false
	}
	window, flags, ok := strings.Cut(name, "]-relayoutWindow#")
	if !ok {
		return "", false, false
	}
	switch flags {
	case "first=true/resize=true/vis=false/params=true/force=false":
		return window, true, true
	case "first=false/resize=false/vis=false/params=true/force=false":
		return window, false, true
	default:
		return "", false, false
	}
}

// frameWindowUpdates records the last update on each frame and assigns an
// ordinal only after observing its first layout on the same emitter.
func frameWindowUpdates(catalog FrameCatalog, tid int) map[int]frameWindowUpdate {
	state := make(map[string]frameWindowUpdate)
	updates := make(map[int]frameWindowUpdate)
	for _, span := range catalog.PointSpans {
		if span.TID != tid || span.Role == "render" || span.FrameLine == 0 {
			continue
		}
		window, initial, ok := frameWindowUpdateMarker(span.Name)
		if !ok {
			continue
		}
		update := state[window]
		if initial {
			update = frameWindowUpdate{window: window, generation: update.generation + 1}
		} else {
			if update.generation == 0 {
				continue
			}
			update.parameters++
		}
		state[window] = update
		updates[span.FrameLine] = update
	}
	return updates
}
