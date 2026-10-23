package tracecatalog

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
)

// Resource ceilings for private coordination, not coverage/sample-size claims.
const MaxDeliveryBranches = 64
const MaxExportDeliveries = 32

// ExportDelivery is an audit view, never a read/query/completion permission.
// A valid frozen package can cease to match current execution independently.
type ExportDelivery struct {
	Receipt      ExportReceipt `json:"receipt"`
	PackageValid bool          `json:"package_valid"`
	CurrentMatch bool          `json:"current_match"`
	Status       string        `json:"status"`
	Reason       string        `json:"reason,omitempty"`
}

type ExportConflictError struct{ Delivery ExportDelivery }

func (e *ExportConflictError) Error() string {
	return "explicit export target retains its frozen delivery; not a current completion: " + e.Delivery.Status + ": " + e.Delivery.Reason
}

type deliveryIntent struct {
	gate         chan struct{}
	root, target string
	frozen       *deliveryPublication
}

type deliveryPublication struct {
	fingerprint string
	receipt     ExportReceipt
	files       []ExportFile
}

// Register only live mutating handles, never clones made for reads. This keeps
// query execution cheap and does not broadcast another branch's authority into
// Snapshot/Query. The export-only union is bounded and uses the existing Merge.
// Call after releasing c.mu; lock order is deliveryMu -> detached clone -> exportMu.
func (c *Catalog) trackDeliveryBranch() {
	if c == nil || c.run == nil || c.id == "" || c.deliveryView {
		return
	}
	c.run.deliveryMu.Lock()
	defer c.run.deliveryMu.Unlock()
	if c.run.deliveryBranches == nil {
		c.run.deliveryBranches = map[*Catalog]uint64{}
	}
	if _, exists := c.run.deliveryBranches[c]; exists {
		return
	}
	if len(c.run.deliveryBranches) >= MaxDeliveryBranches {
		c.run.deliveryBranchOverflow = true
		return
	}
	c.run.deliveryBranches[c] = uint64(len(c.run.deliveryBranches) + 1)
}

func (c *Catalog) deliverySnapshotLocked() (*Catalog, error) {
	if c.run.deliveryBranchOverflow {
		return nil, fmt.Errorf("live export branch limit exceeded; no partial union is certified")
	}
	branches := make([]*Catalog, 0, len(c.run.deliveryBranches))
	for branch := range c.run.deliveryBranches {
		branches = append(branches, branch)
	}
	sort.Slice(branches, func(i, j int) bool { return c.run.deliveryBranches[branches[i]] < c.run.deliveryBranches[branches[j]] })
	view := c.Clone()
	view.deliveryView = true
	for _, branch := range branches {
		if err := view.Merge(branch); err != nil {
			return nil, err
		}
	}
	return view, nil
}

func (c *Catalog) deliveryFingerprint() string {
	type refFile struct{ Path, Root, SHA256, Generation string }
	type refBinding struct {
		Query, Attempt string
		Files          map[string]refFile
		Issues         map[string]string
	}
	refs := map[string]refBinding{}
	execution := c.CurrentExecution()
	c.run.exportMu.Lock()
	for _, q := range execution.Snapshot.Queries {
		for _, a := range q.Attempts {
			b, ok := c.run.exportRefs[a.ID]
			if !ok {
				continue
			}
			r := refBinding{Query: b.queryID, Attempt: b.attemptDigest, Files: map[string]refFile{}, Issues: b.issues}
			for key, f := range b.files {
				r.Files[key] = refFile{f.path, f.root, f.sha256, f.generation.CacheToken()}
			}
			refs[a.ID] = r
		}
	}
	// Binding maps are immutable after publication, but hash while holding the
	// lock so no future mutation can accidentally turn this into an alias read.
	refsDigest := digest("", refs)
	c.run.exportMu.Unlock()
	representatives, selected := c.Representatives()
	return digest("delivery_revision_", struct {
		Execution       ExecutionView
		Selected        bool
		Representatives RepresentativeSnapshot
		References      string
	}{execution, selected, representatives, refsDigest})
}

func deliveryOutputRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("host output root is required")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return filepath.Abs(canonical)
}

// Export coordinates only the same live lineage and explicitly named target.
// It does not execute/reuse query results or select a different target. Host
// authorization for the original roster is still required by the tool caller.
func (c *Catalog) Export(ctx context.Context, outputRoot, target string) (ExportReceipt, error) {
	if c == nil || c.run == nil {
		return ExportReceipt{}, ErrForeignRun
	}
	if err := ctx.Err(); err != nil {
		return ExportReceipt{}, err
	}
	if err := validExportTarget(target); err != nil {
		return ExportReceipt{}, err
	}
	root, err := deliveryOutputRoot(outputRoot)
	if err != nil {
		return ExportReceipt{}, err
	}
	c.trackDeliveryBranch()
	key := root + "\x00" + target
	c.run.deliveryMu.Lock()
	if c.run.deliveries == nil {
		c.run.deliveries = map[string]*deliveryIntent{}
	}
	intent := c.run.deliveries[key]
	if intent == nil {
		if len(c.run.deliveries) >= MaxExportDeliveries {
			c.run.deliveryMu.Unlock()
			return ExportReceipt{}, fmt.Errorf("current export target limit reached")
		}
		intent = &deliveryIntent{root: root, target: target, gate: make(chan struct{}, 1)}
		intent.gate <- struct{}{}
		c.run.deliveries[key] = intent
	}
	c.run.deliveryMu.Unlock()
	select {
	case <-ctx.Done():
		return ExportReceipt{}, ctx.Err()
	case <-intent.gate:
	}
	defer func() { intent.gate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return ExportReceipt{}, err
	}
	c.run.deliveryMu.Lock()
	view, err := c.deliverySnapshotLocked()
	c.run.deliveryMu.Unlock()
	if err != nil {
		return ExportReceipt{}, err
	}
	if intent.frozen != nil {
		delivery := view.inspectDelivery(ctx, intent.frozen)
		if err := ctx.Err(); err != nil {
			return ExportReceipt{}, err
		}
		if !delivery.CurrentMatch {
			return ExportReceipt{}, &ExportConflictError{Delivery: delivery}
		}
		// Recheck the exact union after validation; a concurrently completed
		// query must not receive the previous revision's completion receipt.
		c.run.deliveryMu.Lock()
		latest, err := c.deliverySnapshotLocked()
		if err == nil && latest.deliveryFingerprint() != intent.frozen.fingerprint {
			delivery.CurrentMatch, delivery.Status, delivery.Reason = false, "execution_changed", "Current registered plans, attempts, selection or reference bindings changed. The frozen package remains an audit artifact."
			err = &ExportConflictError{Delivery: delivery}
		}
		c.run.deliveryMu.Unlock()
		if err != nil {
			return ExportReceipt{}, err
		}
		receipt := cloneExportReceipt(delivery.Receipt)
		receipt.Reused = true
		return receipt, nil
	}
	fingerprint := view.deliveryFingerprint()
	guard := func() (func(), error) {
		c.run.deliveryMu.Lock()
		latest, err := c.deliverySnapshotLocked()
		if err == nil && latest.deliveryFingerprint() != fingerprint {
			err = fmt.Errorf("current export roster changed during publication; retry the same explicit target after current attempts settle")
		}
		if err != nil {
			c.run.deliveryMu.Unlock()
			return nil, err
		}
		return c.run.deliveryMu.Unlock, nil
	}
	receipt, err := view.exportNew(ctx, root, target, guard, func(receipt ExportReceipt, files []ExportFile) {
		// Only the successful producer rename mints this private publication.
		// Cancellation or external damage during later verification must retain
		// the physical delivery's audit record, never fabricate current success.
		intent.frozen = &deliveryPublication{fingerprint: fingerprint, receipt: cloneExportReceipt(receipt), files: append([]ExportFile(nil), files...)}
	})
	if err != nil {
		return ExportReceipt{}, err
	}
	if _, err := validateDeliveryPackage(ctx, intent.frozen); err != nil {
		return ExportReceipt{}, fmt.Errorf("package published but verification failed: %w", err)
	}
	return cloneExportReceipt(receipt), nil
}

// ExportDeliveries returns only private current-run publications. The caller
// must authorize source/root access first, as for Export. Disk JSON cannot add
// entries. No records is a cheap no-op; invalid frozen files remain audit rows,
// never completion. Each result is detached from the private stored receipt.
func (c *Catalog) ExportDeliveries(ctx context.Context, outputRoot string) ([]ExportDelivery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.run == nil {
		return nil, nil
	}
	c.run.deliveryMu.Lock()
	if len(c.run.deliveries) == 0 {
		c.run.deliveryMu.Unlock()
		return nil, nil
	}
	intents := make([]*deliveryIntent, 0, len(c.run.deliveries))
	for _, intent := range c.run.deliveries {
		intents = append(intents, intent)
	}
	c.run.deliveryMu.Unlock()
	root, err := deliveryOutputRoot(outputRoot)
	if err != nil {
		return nil, err
	}
	sort.Slice(intents, func(i, j int) bool { return intents[i].target < intents[j].target })
	var out []ExportDelivery
	for _, intent := range intents {
		if intent.root != root {
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-intent.gate:
		}
		c.run.deliveryMu.Lock()
		view, viewErr := c.deliverySnapshotLocked()
		c.run.deliveryMu.Unlock()
		if viewErr == nil && intent.frozen != nil {
			out = append(out, view.inspectDelivery(ctx, intent.frozen))
		}
		intent.gate <- struct{}{}
		if viewErr != nil {
			return nil, viewErr
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func cloneExportReceipt(r ExportReceipt) ExportReceipt {
	r.ExecutionCoverage.PlannedOutcomes = cloneOutcomeCounts(r.ExecutionCoverage.PlannedOutcomes)
	r.ExecutionCoverage.ObservedOutcomes = cloneOutcomeCounts(r.ExecutionCoverage.ObservedOutcomes)
	r.ExecutionCoverage.AttemptOutcomes = cloneOutcomeCounts(r.ExecutionCoverage.AttemptOutcomes)
	return r
}

func cloneOutcomeCounts(in map[Outcome]int) map[Outcome]int {
	out := make(map[Outcome]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
