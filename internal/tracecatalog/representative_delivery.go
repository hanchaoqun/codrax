package tracecatalog

import (
	"context"
	"fmt"
)

// RepresentativeDelivery is a navigation receipt, not an execution, native
// evidence or analysis-completion permission. Only a successful producer
// publication supplies paths. JSON cannot recreate a live selection.
type RepresentativeDelivery struct {
	Kind                   string                  `json:"kind"`
	Status                 string                  `json:"status"`
	CatalogID              string                  `json:"catalog_id"`
	SelectionID            string                  `json:"selection_id,omitempty"`
	IndexPath              string                  `json:"saved_index,omitempty"`
	IndexSHA256            string                  `json:"saved_index_sha256,omitempty"`
	SelectionPath          string                  `json:"selection_snapshot,omitempty"`
	SelectionSHA256        string                  `json:"selection_snapshot_sha256,omitempty"`
	ExecutionCoverage      *ExecutionCoverage      `json:"execution_coverage,omitempty"`
	RepresentativeCoverage *RepresentativeCoverage `json:"representative_coverage,omitempty"`
	Reason                 string                  `json:"reason,omitempty"`
}

// SaveRepresentativePublication runs at the terminal agent boundary, after
// exploration joins. It includes every tracked live branch (even an attempt
// whose intermediate save failed), without installing that union in any fork.
// It never executes, selects, reads a latest alias, or restores saved authority.
func (c *Catalog) SaveRepresentativePublication(ctx context.Context, path string, authorize func(*Catalog) error) (Publication, error) {
	if c == nil || c.run == nil {
		return Publication{}, ErrForeignRun
	}
	if err := ctx.Err(); err != nil {
		return Publication{}, err
	}
	c.run.deliveryMu.Lock()
	view, err := c.deliverySnapshotLocked()
	c.run.deliveryMu.Unlock()
	if err != nil {
		return Publication{}, err
	}
	if _, selected := view.Representatives(); !selected {
		return Publication{}, fmt.Errorf("no current representative selection")
	}
	if authorize == nil {
		return Publication{}, fmt.Errorf("current catalog authorization is required")
	}
	validateSources := func() error {
		if err := authorize(view); err != nil {
			return err
		}
		// Revalidate the actual original/source universe, not a successful
		// query's visible outcome. Failed/empty attempts remain records.
		for _, artifact := range view.Snapshot().Artifacts {
			if _, err := view.Resolve(ctx, artifact.ID); err != nil {
				return err
			}
		}
		return ctx.Err()
	}
	if err := validateSources(); err != nil {
		return Publication{}, err
	}
	before := digest("", view.CurrentExecution())
	p, err := view.savePublication(ctx, path, true, true)
	if err != nil {
		return Publication{}, err
	}
	// File generations and current path authorization are separate from the
	// execution digest. A change while publishing leaves no certified receipt;
	// already published immutable objects remain historical navigation only.
	if err := validateSources(); err != nil {
		return Publication{}, err
	}
	// This is an honest settled-generation receipt, not a promise about work
	// still running concurrently. Files may have been published if this check
	// fails, but the caller receives no successful delivery/path.
	c.run.deliveryMu.Lock()
	current, err := c.deliverySnapshotLocked()
	c.run.deliveryMu.Unlock()
	if err != nil {
		return Publication{}, err
	}
	if digest("", current.CurrentExecution()) != before || digest("", p.Execution) != before {
		return Publication{}, fmt.Errorf("registered representative execution changed during publication")
	}
	if err := ctx.Err(); err != nil {
		return Publication{}, err
	}
	return p, nil
}
