package tracequery

import (
	"encoding/json"
	"math"
	"math/big"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// These are complete event-flow groups for the selected source/window, not
// allocation stock or producer statistic reports. Display limits do not apply.
type ResourceEventStatistics struct {
	Basis              string                   `json:"basis"`
	Complete           bool                     `json:"complete"`
	EventCount         int                      `json:"event_count"`
	TotalGroups        int                      `json:"total_groups"`
	OmittedGroups      int                      `json:"omitted_groups"`
	OmittedGroupEvents int                      `json:"omitted_group_events"`
	Groups             []ResourceEventStatistic `json:"groups"`
}

type ResourceEventStatisticKey struct {
	PID       int                       `json:"pid"`
	TID       int                       `json:"tid"`
	IPID      int64                     `json:"ipid,string"`
	ITID      int64                     `json:"itid,string"`
	Thread    string                    `json:"thread"`
	Process   string                    `json:"process,omitempty"`
	Family    string                    `json:"family"`
	Operation string                    `json:"operation"`
	Action    string                    `json:"action"`
	SizeUnit  string                    `json:"source_size_unit"`
	Subtype   tracewire.ResourceSubtype `json:"subtype"`
}

type ResourceEventStatistic struct {
	ResourceEventStatisticKey
	EventCount            int `json:"event_count"`
	KnownSizeEvents       int `json:"known_size_events"`
	NullSizeEvents        int `json:"null_size_events"`
	UnavailableSizeEvents int `json:"unavailable_size_events"`
	InvalidSizeEvents     int `json:"invalid_size_events"`
	NegativeSizeEvents    int `json:"negative_size_events"`
	// Empty when no nonnegative known size exists; known zero is "0".
	KnownSizeTotal string `json:"known_source_size_total,omitempty"`
}

const resourceEventStatisticsBasis = "native_hook_event_start_time"

func resourceEventStatisticKey(e tracewire.ResourceEvent) ResourceEventStatisticKey {
	family, action, unit, _ := tracewire.ResourceOperationDescriptor(e.Operation)
	subtype := tracewire.ResourceSubtype{ID: tracewire.ResourceScalar{Status: "unavailable"}, Name: tracewire.ResourceText{Status: "unavailable"}}
	if e.Subtype != nil {
		subtype = *e.Subtype
	}
	return ResourceEventStatisticKey{PID: e.PID, TID: e.TID, IPID: e.IPID, ITID: e.ITID, Thread: e.Thread, Process: e.Process, Family: family, Operation: e.Operation, Action: action, SizeUnit: unit, Subtype: subtype}
}

func resourceStatisticIdentity(k ResourceEventStatisticKey) string {
	b, _ := json.Marshal(k)
	return string(b)
}

func buildResourceEventStatistics(events []*ResourceStackEvent, cancel *runCancelState) *ResourceEventStatistics {
	if cancel.sample() {
		return nil
	}
	p := &ResourceEventStatistics{Basis: resourceEventStatisticsBasis, Complete: true, EventCount: len(events), Groups: []ResourceEventStatistic{}}
	groups := map[string]*ResourceEventStatistic{}
	var keys []string
	for _, event := range events {
		if cancel.tick() {
			return nil
		}
		key := resourceEventStatisticKey(event.Source)
		id := resourceStatisticIdentity(key)
		g := groups[id]
		if g == nil {
			g = &ResourceEventStatistic{ResourceEventStatisticKey: key}
			groups[id] = g
			keys = append(keys, id)
		}
		g.EventCount++
		size := event.Source.Size
		switch size.Status {
		case "null":
			g.NullSizeEvents++
		case "unavailable":
			g.UnavailableSizeEvents++
		case "invalid":
			g.InvalidSizeEvents++
		case "known":
			n, ok := size.Integer()
			if !ok {
				g.InvalidSizeEvents++
				continue
			}
			if n < 0 {
				g.NegativeSizeEvents++
				continue
			}
			g.KnownSizeEvents++
			total := new(big.Int)
			if g.KnownSizeTotal != "" {
				total.SetString(g.KnownSizeTotal, 10)
			}
			g.KnownSizeTotal = total.Add(total, big.NewInt(n)).String()
		}
	}
	// First-observation order follows the already ordered event population;
	// avoid a second uninterruptible sort of possibly long identity strings.
	for _, key := range keys {
		if cancel.tick() {
			return nil
		}
		p.Groups = append(p.Groups, *groups[key])
	}
	p.TotalGroups = len(p.Groups)
	if cancel.sample() {
		return nil
	}
	return p
}

func validResourceEventStatistics(p ResourceStackResult) bool {
	s := p.EventStatistics
	if s.Basis != resourceEventStatisticsBasis || s.EventCount != p.MatchedEvents || s.TotalGroups < 0 || s.OmittedGroups < 0 || s.OmittedGroups > s.TotalGroups || s.TotalGroups-s.OmittedGroups != len(s.Groups) || s.Complete != (s.OmittedGroups == 0) || s.OmittedGroupEvents < s.OmittedGroups || s.OmittedGroupEvents > s.EventCount || (s.OmittedGroups == 0 && s.OmittedGroupEvents != 0) {
		return false
	}
	groups := map[string]ResourceEventStatistic{}
	count := 0
	for _, g := range s.Groups {
		family, action, unit, ok := tracewire.ResourceOperationDescriptor(g.Operation)
		if !ok || family != g.Family || action != g.Action || unit != g.SizeUnit || g.PID <= 0 || g.TID <= 0 || g.PID > math.MaxInt32 || g.TID > math.MaxInt32 || g.IPID <= 0 || g.ITID <= 0 || len(g.Thread) > 4096 || !utf8.ValidString(g.Thread) || len(g.Process) > 4096 || !utf8.ValidString(g.Process) || !g.Subtype.ID.Valid() || !g.Subtype.Name.Valid() || !resourceStackOwnerMatches(g.PID, g.TID, g.Thread, p.TargetPID, p.TargetThread, p.TargetScope) {
			return false
		}
		if g.EventCount <= 0 || g.EventCount > s.EventCount-count {
			return false
		}
		remaining := g.EventCount
		for _, n := range []int{g.KnownSizeEvents, g.NullSizeEvents, g.UnavailableSizeEvents, g.InvalidSizeEvents, g.NegativeSizeEvents} {
			if n < 0 || n > remaining {
				return false
			}
			remaining -= n
		}
		if remaining != 0 {
			return false
		}
		if g.KnownSizeEvents == 0 {
			if g.KnownSizeTotal != "" {
				return false
			}
		} else {
			n, ok := new(big.Int).SetString(g.KnownSizeTotal, 10)
			if !ok || n.Sign() < 0 || n.String() != g.KnownSizeTotal {
				return false
			}
		}
		id := resourceStatisticIdentity(g.ResourceEventStatisticKey)
		if _, exists := groups[id]; exists {
			return false
		}
		groups[id] = g
		count += g.EventCount
	}
	if count != s.EventCount-s.OmittedGroupEvents {
		return false
	}
	// Even a preview must belong to a reported group; with all rows available
	// the aggregate can additionally be checked byte-for-byte against its rows.
	visible := make([]*ResourceStackEvent, len(p.Events))
	for i := range p.Events {
		visible[i] = &p.Events[i]
	}
	computed := buildResourceEventStatistics(visible, nil)
	unshownEvents := 0
	for _, g := range computed.Groups {
		full, ok := groups[resourceStatisticIdentity(g.ResourceEventStatisticKey)]
		if !ok {
			unshownEvents += g.EventCount
			continue
		}
		if g.EventCount > full.EventCount || g.KnownSizeEvents > full.KnownSizeEvents || g.NullSizeEvents > full.NullSizeEvents || g.UnavailableSizeEvents > full.UnavailableSizeEvents || g.InvalidSizeEvents > full.InvalidSizeEvents || g.NegativeSizeEvents > full.NegativeSizeEvents {
			return false
		}
		if g.KnownSizeEvents > 0 {
			a, _ := new(big.Int).SetString(g.KnownSizeTotal, 10)
			b, _ := new(big.Int).SetString(full.KnownSizeTotal, 10)
			if b == nil || a.Cmp(b) > 0 {
				return false
			}
		}
		if p.OmittedEvents == 0 && g != full {
			return false
		}
	}
	if unshownEvents > s.OmittedGroupEvents {
		return false
	}
	if p.OmittedEvents == 0 {
		// nil and [] both describe zero groups. Validate semantic counts and
		// exact group values, not incidental JSON slice representation/order.
		return s.TotalGroups == computed.TotalGroups && s.OmittedGroups == len(computed.Groups)-len(s.Groups) && unshownEvents == s.OmittedGroupEvents
	}
	return true
}
