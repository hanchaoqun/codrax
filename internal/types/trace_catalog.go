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
