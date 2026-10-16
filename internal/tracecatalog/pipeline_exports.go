package tracecatalog

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ExportRequest is the status of an accepted named delivery, not a publication
// receipt. Only Delivery can carry a verified physical package. These public
// fields never reconstruct the private request or its live execution lineage.
type ExportRequest struct {
	IntentID   string          `json:"intent_id"`
	CatalogID  string          `json:"catalog_id"`
	Target     string          `json:"target"`
	Status     string          `json:"status"`
	ReasonCode string          `json:"reason_code,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Delivery   *ExportDelivery `json:"delivery,omitempty"`
}

type pendingPipelineExport struct {
	catalog *Catalog
	request ExportRequest
}

// PipelineExports is a host-created, per-Run capability for deferring only
// managed pipeline deliveries until the terminal agent handoff. It is shared
// by live forks, never serialized, restored, or enabled by a stage string.
type PipelineExports struct {
	mu          sync.Mutex
	root        *ManagedExportRoot
	initialized bool
	sealed      bool
	requests    map[string]*pendingPipelineExport
}

func NewPipelineExports(root *ManagedExportRoot) *PipelineExports {
	return &PipelineExports{root: root, initialized: root != nil, requests: map[string]*pendingPipelineExport{}}
}

func (p *PipelineExports) checkRoot(root *ManagedExportRoot) error {
	if p == nil || !p.initialized || p.root == nil || p.root != root {
		return fmt.Errorf("pending export scope was not initialized for this host delivery root")
	}
	return nil
}

// SameLiveLineage compares private handles, not public IDs or decoded JSON.
// It is a narrow ownership check; it grants no source/query authorization.
func (c *Catalog) SameLiveLineage(other *Catalog) bool {
	return c != nil && other != nil && c.run != nil && c.run == other.run && c.id == other.id
}

// Accept retains the original target without allocating a directory or
// freezing an execution snapshot. The tool caller must first authorize inputs.
func (p *PipelineExports) Accept(ctx context.Context, root *ManagedExportRoot, c *Catalog, target string) (ExportRequest, error) {
	if err := p.checkRoot(root); err != nil {
		return ExportRequest{}, err
	}
	if c == nil || c.run == nil || c.id == "" {
		return ExportRequest{}, ErrForeignRun
	}
	if err := validExportTarget(target); err != nil {
		return ExportRequest{}, err
	}
	if _, err := root.Directory(ctx, false); err != nil {
		return ExportRequest{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ExportRequest{}, err
	}
	if prior := p.requests[target]; prior != nil {
		if !prior.catalog.SameLiveLineage(c) {
			return ExportRequest{}, fmt.Errorf("named export target belongs to another current catalog lineage")
		}
		r := cloneExportRequest(prior.request)
		if r.Status == "published" {
			r = inspectPipelineDelivery(ctx, root, prior.catalog, r)
		}
		return r, nil
	}
	if p.sealed {
		return ExportRequest{}, fmt.Errorf("terminal delivery is sealed; no new export request was accepted")
	}
	if len(p.requests) >= MaxExportDeliveries {
		return ExportRequest{}, fmt.Errorf("current export target limit reached")
	}
	r := ExportRequest{IntentID: digest("export_intent_", []string{root.run, c.ID(), target}), CatalogID: c.ID(), Target: target, Status: "pending_publication"}
	p.requests[target] = &pendingPipelineExport{catalog: c, request: r}
	return r, nil
}

func (p *PipelineExports) orderedLocked() []*pendingPipelineExport {
	keys := make([]string, 0, len(p.requests))
	for key := range p.requests {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*pendingPipelineExport, 0, len(keys))
	for _, key := range keys {
		out = append(out, p.requests[key])
	}
	return out
}

func cloneExportRequest(r ExportRequest) ExportRequest {
	if r.Delivery != nil {
		d := *r.Delivery
		d.Receipt = cloneExportReceipt(d.Receipt)
		r.Delivery = &d
	}
	return r
}
