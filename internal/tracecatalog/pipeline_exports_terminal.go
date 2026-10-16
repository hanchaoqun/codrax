package tracecatalog

import (
	"context"
	"errors"
	"fmt"
)

// Materialize is the terminal agent's one-shot boundary, not a model tool or a
// finalizer-stage heuristic. It runs no queries and does not require successes:
// pending/empty/failed attempts remain honest entries in the exported union.
func (p *PipelineExports) Materialize(ctx context.Context, root *ManagedExportRoot, authorize func(*Catalog) error) ([]ExportRequest, error) {
	if err := p.checkRoot(root); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.sealed {
		p.sealed = true
		for _, intent := range p.orderedLocked() {
			p.materializeOne(ctx, root, intent, authorize)
		}
	}
	return p.requestsLocked(ctx, root, authorize), nil
}

func (p *PipelineExports) materializeOne(ctx context.Context, root *ManagedExportRoot, intent *pendingPipelineExport, authorize func(*Catalog) error) {
	r := &intent.request
	fail := func(code string, err error) {
		r.Status, r.ReasonCode, r.Reason = "failed", code, err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			r.Status = "cancelled"
		}
	}
	if err := ctx.Err(); err != nil {
		fail("publication_cancelled", err)
		return
	}
	if authorize == nil {
		fail("source_authorization", fmt.Errorf("current source authorization is required"))
		return
	}
	if err := authorize(intent.catalog); err != nil {
		fail("source_authorization", err)
		return
	}
	dir, err := root.Directory(ctx, true)
	if err != nil {
		fail("host_destination", err)
		return
	}
	receipt, err := intent.catalog.Export(ctx, dir, r.Target)
	if err != nil {
		fail("publication_failed", err)
		return
	}
	r.Status, r.ReasonCode, r.Reason = "published", "", ""
	r.Delivery = &ExportDelivery{Receipt: receipt, PackageValid: true, CurrentMatch: true, Status: "current"}
}

// Requests is a detached read-only audit projection. Failed materializations
// stay failed; later projection cannot retry, rename or fabricate a receipt.
func (p *PipelineExports) Requests(ctx context.Context, root *ManagedExportRoot, authorize func(*Catalog) error) ([]ExportRequest, error) {
	if err := p.checkRoot(root); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requestsLocked(ctx, root, authorize), nil
}

func (p *PipelineExports) requestsLocked(ctx context.Context, root *ManagedExportRoot, authorize func(*Catalog) error) []ExportRequest {
	var out []ExportRequest
	for _, intent := range p.orderedLocked() {
		r := cloneExportRequest(intent.request)
		// Existing terminal failures/cancellations already describe a finite
		// attempt. Keep that reason, even when the caller is now cancelled.
		if r.Status == "failed" || r.Status == "cancelled" {
			out = append(out, r)
			continue
		}
		var err error
		if authorize == nil {
			err = fmt.Errorf("current source authorization is required")
		} else {
			err = authorize(intent.catalog)
		}
		if err != nil {
			r.Status, r.ReasonCode, r.Reason, r.Delivery = "failed", "source_authorization", err.Error(), nil
		} else if r.Status == "published" {
			r = inspectPipelineDelivery(ctx, root, intent.catalog, r)
		}
		out = append(out, r)
	}
	return out
}

func inspectPipelineDelivery(ctx context.Context, root *ManagedExportRoot, c *Catalog, r ExportRequest) ExportRequest {
	dir, err := root.Directory(ctx, false)
	var rows []ExportDelivery
	if err == nil && dir != "" {
		rows, err = c.ExportDeliveries(ctx, dir)
	}
	r.Delivery = nil
	if err == nil {
		for _, d := range rows {
			if d.Receipt.Target == r.Target {
				r.Delivery = &d
				if !d.PackageValid || !d.CurrentMatch {
					r.Status, r.ReasonCode, r.Reason = "failed", "publication_not_current", d.Reason
				}
				return r
			}
		}
		err = fmt.Errorf("private publication receipt is unavailable")
	}
	r.Status, r.ReasonCode, r.Reason = "failed", "publication_unverified", err.Error()
	return r
}
