package render

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type parallelActivity struct {
	groupID     string
	stage       types.PipelineStage
	agent       types.AgentName
	total       int
	parallelism int
	laneLabels  []string
	order       []string
	units       map[string]*parallelActivityUnit
}

type parallelActivityUnit struct {
	active bool
	done   bool
	kind   activityKind
	tail   string
	detail string
	err    string
}

type parallelActivitySnapshot struct {
	active      bool
	stage       types.PipelineStage
	total       int
	parallelism int
	activeUnits int
	doneUnits   int
	requesting  int
	receiving   int
	tool        int
	retrying    int
	streamTail  string
	toolDetail  string
	laneLabels  []string
}

func newParallelActivity(ev Event) *parallelActivity {
	p := &parallelActivity{
		groupID:     strings.TrimSpace(ev.ParallelGroupID),
		stage:       ev.Stage,
		agent:       ev.Agent,
		total:       ev.ParallelTotal,
		parallelism: ev.Parallelism,
		laneLabels:  cleanParallelLaneLabels(ev.ParallelLaneLabels),
		units:       make(map[string]*parallelActivityUnit),
	}
	for _, id := range ev.ParallelUnitIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		p.ensureUnit(id)
	}
	if p.total <= 0 {
		p.total = len(p.order)
	}
	return p
}

func (p *parallelActivity) matches(ev Event) bool {
	if p == nil {
		return false
	}
	if ev.ParallelGroupID == "" {
		return false
	}
	return p.groupID == "" || ev.ParallelGroupID == p.groupID
}

func (p *parallelActivity) appliesToStageKey(stageKey string) bool {
	if p == nil {
		return false
	}
	return parallelStageAppliesToStageKey(p.stage, stageKey)
}

func (p *parallelActivity) ensureUnit(id string) *parallelActivityUnit {
	if p == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if p.units == nil {
		p.units = make(map[string]*parallelActivityUnit)
	}
	if u := p.units[id]; u != nil {
		return u
	}
	u := &parallelActivityUnit{}
	p.units[id] = u
	p.order = append(p.order, id)
	if p.total < len(p.order) {
		p.total = len(p.order)
	}
	return u
}

func (p *parallelActivity) startUnit(ev Event) {
	if !p.matches(ev) {
		return
	}
	u := p.ensureUnit(ev.ParallelUnitID)
	if u == nil {
		return
	}
	u.active = true
	u.done = false
	u.kind = activityWaitingNode
	u.tail = ""
	u.detail = ""
	u.err = ""
}

func (p *parallelActivity) endUnit(ev Event) {
	if !p.matches(ev) {
		return
	}
	u := p.ensureUnit(ev.ParallelUnitID)
	if u == nil {
		return
	}
	u.active = false
	u.done = true
	u.kind = activityNone
	u.tail = ""
	u.detail = ""
	u.err = ev.Error
}

func (p *parallelActivity) setUnitActivity(ev Event, kind activityKind, detail, tail string) bool {
	if !p.matches(ev) {
		return false
	}
	u := p.ensureUnit(ev.ParallelUnitID)
	if u == nil {
		return false
	}
	u.active = true
	u.done = false
	u.kind = kind
	u.detail = strings.TrimSpace(detail)
	u.tail = strings.TrimSpace(tail)
	return true
}

func (p *parallelActivity) unitOrdinal(id string) (ordinal int, total int, ok bool) {
	if p == nil {
		return 0, 0, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, 0, false
	}
	if p.units[id] == nil {
		return 0, 0, false
	}
	for i, candidate := range p.order {
		if candidate == id {
			total = p.total
			if total <= 0 {
				total = len(p.order)
			}
			return i + 1, total, true
		}
	}
	return 0, 0, false
}

func (p *parallelActivity) snapshot() *parallelActivitySnapshot {
	if p == nil {
		return nil
	}
	s := &parallelActivitySnapshot{
		active:      true,
		stage:       p.stage,
		total:       p.total,
		parallelism: p.parallelism,
		laneLabels:  cleanParallelLaneLabels(p.laneLabels),
	}
	if s.total <= 0 {
		s.total = len(p.order)
	}
	for _, id := range p.order {
		u := p.units[id]
		if u == nil {
			continue
		}
		if u.done {
			s.doneUnits++
			continue
		}
		if !u.active {
			continue
		}
		s.activeUnits++
		switch u.kind {
		case activityRequesting:
			s.requesting++
		case activityReceiving:
			s.receiving++
			if s.streamTail == "" {
				s.streamTail = u.tail
			}
		case activityCallingTool:
			s.tool++
			if s.toolDetail == "" {
				s.toolDetail = u.detail
			}
		case activityRetrying, activitySwitchingProvider:
			s.retrying++
		}
	}
	if s.parallelism <= 0 {
		s.parallelism = s.activeUnits
	}
	return s
}

func cleanParallelLaneLabels(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, label := range in {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, label)
	}
	return out
}

func parallelSnapshotAppliesToStageKey(p *parallelActivitySnapshot, stageKey string) bool {
	if p == nil || !p.active {
		return false
	}
	return parallelStageAppliesToStageKey(p.stage, stageKey)
}

func parallelStageAppliesToStageKey(parallelStage types.PipelineStage, stageKey string) bool {
	key := canonicalStageKey(stageKey)
	if key == "" {
		return true
	}
	pKey := canonicalStageKey(string(parallelStage))
	if pKey == "" {
		return true
	}
	if pKey == "explore" {
		return isExploreFamilyStageKey(key)
	}
	return pKey == key
}

func isExploreFamilyStageKey(key string) bool {
	switch canonicalStageKey(key) {
	case "explore", "evidence", "validate", "reconcile":
		return true
	}
	return false
}
