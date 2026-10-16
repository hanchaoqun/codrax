package tracecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// These are resource ceilings, not statistical sample-size claims.
const (
	MaxRepresentativesPerCluster = 5
	MaxRepresentativeClusters    = 50
)

// RepresentativeOptions controls navigation only. Every unselected query
// remains executable through ordinary query admission; ranking is not severity.
type RepresentativeOptions struct {
	PerCluster  int    `json:"per_cluster"`
	MaxClusters int    `json:"max_clusters"`
	Priority    string `json:"priority"`
}

// QuerySpecification is exact parameter equivalence, not semantic clustering.
// Identity is still local to each member's source; an equal numeric TID across
// captures never establishes the same physical thread or aligned clocks.
type QuerySpecification struct {
	Object     Object          `json:"object"`
	View       string          `json:"view"`
	Window     *Window         `json:"window,omitempty"`
	Parameters json.RawMessage `json:"parameters"`
}

type representativeCluster struct {
	ID            string
	Specification QuerySpecification
	Members       []string
	Selected      []string
	Skipped       map[string]string
	BudgetSkipped bool
}

type representativePlan struct {
	ID        string
	Sequence  uint64
	Options   RepresentativeOptions
	Clusters  []representativeCluster
	Unmatched []string
}

type RepresentativeMemberCoverage struct {
	Members           int             `json:"members"`
	Selected          int             `json:"selected"`
	BudgetNotSelected int             `json:"budget_not_selected"`
	Skipped           int             `json:"source_skipped"`
	Outcomes          map[Outcome]int `json:"member_outcomes"`
	SelectedOutcomes  map[Outcome]int `json:"selected_outcomes"`
}

type RepresentativeCoverage struct {
	RepresentativeMemberCoverage
	Candidates       int `json:"candidates"`
	Clusters         int `json:"clusters"`
	SelectedClusters int `json:"selected_clusters"`
	SkippedClusters  int `json:"budget_skipped_clusters"`
	Unmatched        int `json:"unmatched_candidates"`
	NotInSelection   int `json:"new_queries_not_in_selection"`
}

type RepresentativeMember struct {
	QueryID          string   `json:"query_id"`
	ArtifactID       string   `json:"artifact_id"`
	Path             string   `json:"path"`
	ArtifactRevision string   `json:"artifact_revision"`
	SourceRevision   string   `json:"source_revision,omitempty"`
	SourceStatus     string   `json:"source_status"`
	Selected         bool     `json:"selected"`
	SelectionReason  string   `json:"selection_reason"`
	Outcome          Outcome  `json:"outcome"`
	Attempts         int      `json:"attempt_count"`
	LatestAttempt    *Attempt `json:"latest_attempt,omitempty"`
}

type RepresentativeCluster struct {
	ID            string                       `json:"cluster_id"`
	Specification QuerySpecification           `json:"specification"`
	BudgetSkipped bool                         `json:"budget_skipped"`
	Coverage      RepresentativeMemberCoverage `json:"coverage"`
	Members       []RepresentativeMember       `json:"members"`
}

// RepresentativeSnapshot is a detached, serializable navigation view. It has
// no restoration API, execution authority or evidence weight. Outcomes are
// refreshed from current exact query IDs, while selection membership is fixed.
type RepresentativeSnapshot struct {
	CatalogID      string                  `json:"catalog_id"`
	SelectionID    string                  `json:"selection_id"`
	NavigationOnly bool                    `json:"navigation_only"`
	Options        RepresentativeOptions   `json:"options"`
	Discovery      Discovery               `json:"discovery"`
	Coverage       RepresentativeCoverage  `json:"coverage"`
	Clusters       []RepresentativeCluster `json:"clusters"`
	Unmatched      []Artifact              `json:"unmatched_candidates"`
	NewQueryIDs    []string                `json:"new_query_ids,omitempty"`
}

func normalizeRepresentativeOptions(o RepresentativeOptions) (RepresentativeOptions, error) {
	if o.PerCluster == 0 {
		o.PerCluster = 1
	}
	if o.MaxClusters == 0 {
		o.MaxClusters = 10
	}
	if o.Priority == "" {
		o.Priority = "unexecuted_first"
	}
	if o.PerCluster < 1 || o.PerCluster > MaxRepresentativesPerCluster || o.MaxClusters < 1 || o.MaxClusters > MaxRepresentativeClusters {
		return o, fmt.Errorf("representative resource limits are per_cluster 1..%d and max_clusters 1..%d", MaxRepresentativesPerCluster, MaxRepresentativeClusters)
	}
	switch o.Priority {
	case "unexecuted_first", "retry_first", "largest_input_first":
	default:
		return o, fmt.Errorf("unknown representative navigation priority")
	}
	return o, nil
}

// SelectRepresentatives freezes the current query roster. The caller must
// perform current host authorization/source checks before this navigation
// action. No query is executed and no input/window permission is added here.
func (c *Catalog) SelectRepresentatives(ctx context.Context, options RepresentativeOptions) (RepresentativeSnapshot, error) {
	defer c.trackDeliveryBranch()
	o, err := normalizeRepresentativeOptions(options)
	if err != nil {
		return RepresentativeSnapshot{}, err
	}
	if c == nil || c.run == nil {
		return RepresentativeSnapshot{}, ErrForeignRun
	}
	if err := ctx.Err(); err != nil {
		return RepresentativeSnapshot{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	groups := map[string]*representativeCluster{}
	matched := map[string]bool{}
	for _, q := range c.queries {
		// Plans are canonicalized at registration. Source/path are already
		// absent from public tool plans; strip only those exact top-level keys
		// for generic callers, never filters, target kind, scopes or units.
		args := map[string]json.RawMessage{}
		if err := json.Unmarshal(q.Parameters, &args); err != nil {
			return RepresentativeSnapshot{}, err
		}
		delete(args, "source")
		delete(args, "path")
		parameters, err := json.Marshal(args)
		if err != nil {
			return RepresentativeSnapshot{}, err
		}
		spec := QuerySpecification{Object: q.Object, View: q.View, Window: q.Window, Parameters: parameters}
		id := digest("cluster_", spec)
		if groups[id] == nil {
			groups[id] = &representativeCluster{ID: id, Specification: spec, Skipped: map[string]string{}}
		}
		groups[id].Members = append(groups[id].Members, q.ID)
		matched[q.ArtifactID] = true
	}
	p := &representativePlan{Options: o}
	for _, group := range groups {
		sort.Slice(group.Members, func(i, j int) bool {
			a, b := c.queries[group.Members[i]], c.queries[group.Members[j]]
			if o.Priority == "unexecuted_first" && (len(a.Attempts) == 0) != (len(b.Attempts) == 0) {
				return len(a.Attempts) == 0
			}
			if o.Priority == "retry_first" {
				ar, br := a.Outcome == OutcomeFailure || a.Outcome == OutcomeCanceled, b.Outcome == OutcomeFailure || b.Outcome == OutcomeCanceled
				if ar != br {
					return ar
				}
			}
			if o.Priority == "largest_input_first" {
				aa, ba := c.artifacts[a.ArtifactID], c.artifacts[b.ArtifactID]
				ak, bk := aa.Status != "unavailable" && aa.Status != "stale", ba.Status != "unavailable" && ba.Status != "stale"
				if ak != bk {
					return ak
				} // Unknown bytes are not zero-size evidence.
				if ak && aa.Bytes != ba.Bytes {
					return aa.Bytes > ba.Bytes
				}
			}
			return a.ID < b.ID
		})
		p.Clusters = append(p.Clusters, *group)
	}
	sort.Slice(p.Clusters, func(i, j int) bool {
		if len(p.Clusters[i].Members) != len(p.Clusters[j].Members) {
			return len(p.Clusters[i].Members) > len(p.Clusters[j].Members)
		}
		return p.Clusters[i].ID < p.Clusters[j].ID
	})
	for i := range p.Clusters {
		g := &p.Clusters[i]
		g.BudgetSkipped = i >= o.MaxClusters
		for _, id := range g.Members {
			a := c.artifacts[c.queries[id].ArtifactID]
			if a.Status == "unavailable" || a.Status == "stale" {
				g.Skipped[id] = a.Status
				continue
			}
			if !g.BudgetSkipped && len(g.Selected) < o.PerCluster {
				g.Selected = append(g.Selected, id)
			}
		}
	}
	for id := range c.artifacts {
		if !matched[id] {
			p.Unmatched = append(p.Unmatched, id)
		}
	}
	sort.Strings(p.Unmatched)
	p.ID = digest("selection_", struct {
		Catalog   string
		Options   RepresentativeOptions
		Clusters  []representativeCluster
		Unmatched []string
	}{c.id, o, p.Clusters, p.Unmatched})
	if err := ctx.Err(); err != nil {
		return RepresentativeSnapshot{}, err
	}
	p.Sequence = c.run.sequence.Add(1)
	c.representatives = cloneRepresentativePlan(p)
	return c.representativeSnapshotLocked(), nil
}

// Representatives never re-ranks after execution. A fresh explicit selection
// is required to replace the frozen roster; new plans are separately listed.
func (c *Catalog) Representatives() (RepresentativeSnapshot, bool) {
	if c == nil {
		return RepresentativeSnapshot{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.representatives == nil {
		return RepresentativeSnapshot{}, false
	}
	return c.representativeSnapshotLocked(), true
}

func representativeCoverage() RepresentativeMemberCoverage {
	return RepresentativeMemberCoverage{Outcomes: map[Outcome]int{}, SelectedOutcomes: map[Outcome]int{}}
}

func countRepresentativeMember(c *RepresentativeMemberCoverage, m RepresentativeMember) {
	c.Members++
	c.Outcomes[m.Outcome]++
	if m.Selected {
		c.Selected++
		c.SelectedOutcomes[m.Outcome]++
		return
	}
	if m.SelectionReason == "source_unavailable" || m.SelectionReason == "source_stale" {
		c.Skipped++
		return
	}
	c.BudgetNotSelected++
}

func (c *Catalog) representativeSnapshotLocked() RepresentativeSnapshot {
	p := c.representatives
	s := RepresentativeSnapshot{CatalogID: c.id, SelectionID: p.ID, NavigationOnly: true, Options: p.Options, Discovery: c.discovery, Coverage: RepresentativeCoverage{RepresentativeMemberCoverage: representativeCoverage()}}
	s.Discovery.Issues = append([]Issue(nil), s.Discovery.Issues...)
	s.Coverage.Candidates = len(c.artifacts)
	known := map[string]bool{}
	for _, original := range p.Clusters {
		g := RepresentativeCluster{ID: original.ID, Specification: cloneQuerySpecification(original.Specification), BudgetSkipped: original.BudgetSkipped, Coverage: representativeCoverage()}
		chosen := map[string]bool{}
		for _, id := range original.Selected {
			chosen[id] = true
		}
		for _, id := range original.Members {
			known[id] = true
			q := cloneQuery(c.queries[id])
			a := c.artifacts[q.ArtifactID]
			m := RepresentativeMember{QueryID: id, ArtifactID: a.ID, Path: a.Path, ArtifactRevision: q.ArtifactRevision, SourceRevision: a.SourceRevision, SourceStatus: a.Status, Selected: chosen[id], Outcome: q.Outcome, Attempts: len(q.Attempts), SelectionReason: "per_cluster_budget"}
			if original.BudgetSkipped {
				m.SelectionReason = "cluster_budget"
			}
			if reason := original.Skipped[id]; reason != "" {
				m.SelectionReason = "source_" + reason
			}
			if m.Selected {
				m.SelectionReason = "soft_priority"
			}
			if len(q.Attempts) > 0 {
				latest := q.Attempts[len(q.Attempts)-1]
				m.LatestAttempt = &latest
			}
			g.Members = append(g.Members, m)
			countRepresentativeMember(&g.Coverage, m)
			countRepresentativeMember(&s.Coverage.RepresentativeMemberCoverage, m)
		}
		if g.BudgetSkipped {
			s.Coverage.SkippedClusters++
		} else if len(original.Selected) > 0 {
			s.Coverage.SelectedClusters++
		}
		s.Clusters = append(s.Clusters, g)
	}
	s.Coverage.Clusters = len(s.Clusters)
	for _, id := range p.Unmatched {
		a := c.artifacts[id]
		a.Issues = append([]Issue(nil), a.Issues...)
		s.Unmatched = append(s.Unmatched, a)
	}
	s.Coverage.Unmatched = len(s.Unmatched)
	for id := range c.queries {
		if !known[id] {
			s.NewQueryIDs = append(s.NewQueryIDs, id)
		}
	}
	sort.Strings(s.NewQueryIDs)
	s.Coverage.NotInSelection = len(s.NewQueryIDs)
	return s
}

func cloneQuerySpecification(s QuerySpecification) QuerySpecification {
	q := cloneQuery(QueryRecord{Plan: Plan{Parameters: s.Parameters, Window: s.Window}})
	s.Parameters, s.Window = q.Parameters, q.Window
	return s
}

func cloneRepresentativePlan(p *representativePlan) *representativePlan {
	if p == nil {
		return nil
	}
	o := *p
	o.Unmatched = append([]string(nil), p.Unmatched...)
	o.Clusters = make([]representativeCluster, len(p.Clusters))
	for i, g := range p.Clusters {
		o.Clusters[i] = g
		o.Clusters[i].Specification = cloneQuerySpecification(g.Specification)
		o.Clusters[i].Members = append([]string(nil), g.Members...)
		o.Clusters[i].Selected = append([]string(nil), g.Selected...)
		o.Clusters[i].Skipped = map[string]string{}
		for id, reason := range g.Skipped {
			o.Clusters[i].Skipped[id] = reason
		}
	}
	return &o
}
