package types

import (
	"fmt"
	"sort"
	"sync"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// A turn shares lineage identities, not mutable query state. Independent
// explorer discoveries of the same frozen roster can then merge attempts
// without accepting a foreign run or reconstructing authority from JSON.
type traceCatalogLineages struct {
	mu   sync.Mutex
	byID map[string]*tracecatalog.Catalog
}

type stagedTraceAdmissionCatalog struct {
	catalog    *tracecatalog.Catalog
	generation *traceSourceReadGeneration
	objective  string
}

// StageTraceCatalogForInvestigation bridges run-entry preparation to the
// scheduler's existing fresh-cycle reset. Only an unqueried independent input
// roster may cross once; no query, read, fact or old-fork receipt is retained.
func (m *MutableState) StageTraceCatalogForInvestigation(c *tracecatalog.Catalog) error {
	if m == nil || c == nil {
		return fmt.Errorf("current input catalog is required")
	}
	s := c.Snapshot()
	if len(s.Queries) != 0 || len(s.Artifacts) != 2 {
		return fmt.Errorf("only an unqueried input roster can be staged")
	}
	for _, a := range s.Artifacts {
		if !c.IndependentInput(a.ID) {
			return fmt.Errorf("independent operation binding is required")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.traceCatalogs[s.ID] != c {
		return fmt.Errorf("catalog is not installed in the current run")
	}
	m.traceAdmissionCatalog = &stagedTraceAdmissionCatalog{catalog: c.Clone(), generation: m.traceSourceReadGeneration, objective: m.objective}
	return nil
}

// ClearStagedTraceCatalogForInvestigation is also called on Run's early exits;
// a completed admission must never become an unrelated run's navigation.
func (m *MutableState) ClearStagedTraceCatalogForInvestigation() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.traceAdmissionCatalog = nil
}

func (m *MutableState) resetTraceCatalogsForInvestigationLocked() {
	staged := m.traceAdmissionCatalog
	m.traceAdmissionCatalog = nil
	m.traceCatalogs = nil
	m.traceCatalogLineages = &traceCatalogLineages{}
	if staged != nil && staged.generation != nil && staged.generation == m.traceSourceReadGeneration && staged.objective == m.objective {
		c := staged.catalog
		// The staged clone has no attempts or validators for query evidence.
		m.traceCatalogs = map[string]*tracecatalog.Catalog{c.ID(): c}
		m.traceCatalogLineages.byID = map[string]*tracecatalog.Catalog{c.ID(): c.Clone()}
	}
}

// TraceCatalogs returns current run-local navigation handles, never restored
// JSON. Callers must use the catalog's generation checks before file access.
func (m *MutableState) TraceCatalogs() []*tracecatalog.Catalog {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.traceCatalogs))
	for key := range m.traceCatalogs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*tracecatalog.Catalog, 0, len(keys))
	for _, key := range keys {
		out = append(out, m.traceCatalogs[key])
	}
	return out
}

func (m *MutableState) InstallTraceCatalog(c *tracecatalog.Catalog) (*tracecatalog.Catalog, error) {
	if m == nil || c == nil {
		return nil, fmt.Errorf("trace catalog requires current run state")
	}
	id := c.Snapshot().ID
	if id == "" {
		return nil, fmt.Errorf("trace catalog has no identity")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if prior := m.traceCatalogs[id]; prior != nil {
		return prior, nil
	}
	if len(m.traceCatalogs) >= 8 {
		return nil, fmt.Errorf("trace catalog limit reached: narrow or reuse a current catalog")
	}
	if m.traceCatalogs == nil {
		m.traceCatalogs = make(map[string]*tracecatalog.Catalog)
	}
	if m.traceCatalogLineages == nil {
		m.traceCatalogLineages = &traceCatalogLineages{}
	}
	lineages := m.traceCatalogLineages
	lineages.mu.Lock()
	defer lineages.mu.Unlock()
	if prior := lineages.byID[id]; prior != nil {
		c = prior.Clone()
	} else {
		if len(lineages.byID) >= 8 {
			return nil, fmt.Errorf("trace catalog turn-wide limit reached")
		}
		if lineages.byID == nil {
			lineages.byID = make(map[string]*tracecatalog.Catalog)
		}
		lineages.byID[id] = c.Clone()
	}
	m.traceCatalogs[id] = c
	return c, nil
}

func cloneTraceCatalogs(in map[string]*tracecatalog.Catalog) map[string]*tracecatalog.Catalog {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*tracecatalog.Catalog, len(in))
	for id, catalog := range in {
		out[id] = catalog.Clone()
	}
	return out
}

func (m *MutableState) mergeTraceCatalogsLocked(generation *traceSourceReadGeneration, in map[string]*tracecatalog.Catalog) {
	if generation == nil || generation != m.traceSourceReadGeneration {
		return
	}
	for id, catalog := range in {
		// Only a live fork of this turn can introduce a newly discovered
		// catalog. Existing lineages still merge through the core's run check.
		if current := m.traceCatalogs[id]; current != nil {
			_ = current.Merge(catalog)
		} else if len(m.traceCatalogs) < 8 {
			if m.traceCatalogs == nil {
				m.traceCatalogs = make(map[string]*tracecatalog.Catalog)
			}
			m.traceCatalogs[id] = catalog.Clone()
		}
	}
}
