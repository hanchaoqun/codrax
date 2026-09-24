package tracequery

import (
	"fmt"
	"strings"
)

const EventSearchNameLimit = 16

// NormalizeEventSearchNames validates an exact, case-sensitive OR set over
// Event.Name. Unlike event_types, it never aliases names into broader families;
// unlike patterns, it never searches payloads or folds case. Preserve name bytes.
func NormalizeEventSearchNames(view string, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	if CanonicalViewName(view) != FallbackViewEventSearch {
		return nil, fmt.Errorf("event_names is only valid for view=event_search")
	}
	if len(names) > EventSearchNameLimit {
		return nil, fmt.Errorf("event_names received %d names; maximum is %d", len(names), EventSearchNameLimit)
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for i, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("event_names name %d is empty", i+1)
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out, nil
}

func eventMatchesNames(ev Event, names []string) bool {
	if len(names) == 0 {
		return true
	}
	for _, name := range names {
		if ev.Name == name {
			return true
		}
	}
	return false
}

// HasIOActivityEndpoint identifies a parser-admitted endpoint, not a request
// pair, measurement or cause. Used only for nearby measurement navigation.
func HasIOActivityEndpoint(ev Event) bool {
	endpoint, present := ioActivityFromEvent(ev)
	return present && endpoint.admitted
}
