package orchestrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) ensureChangePlanPath() string {
	if o == nil || o.busCtx == nil {
		return ""
	}
	if path := strings.TrimSpace(o.busCtx.PlanPath); path != "" {
		o.planPath = path
		o.reportDir = filepath.Dir(path)
		return path
	}
	if o.busCtx.Mutable == nil {
		return ""
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		return ""
	}
	if o.planSaver != nil {
		path, err := o.planSaver.Save(plan)
		if err != nil {
			logging.Warning("[orchestrator] ChangePlan persist fallback failed: %v", err)
		} else if strings.TrimSpace(path) != "" {
			o.busCtx.PlanPath = path
			o.planPath = path
			o.generatedPlanPath = true
			o.reportDir = filepath.Dir(path)
			return path
		}
	}
	if workDir := strings.TrimSpace(o.busCtx.WorkDir); workDir != "" {
		stem := writeWorkflowArtifactFileStem(plan.ID)
		if stem == "" {
			return ""
		}
		path := filepath.Join(workDir, "plans", stem+".json")
		o.busCtx.PlanPath = path
		o.planPath = path
		o.generatedPlanPath = true
		o.reportDir = filepath.Dir(path)
		return path
	}
	return ""
}

func (o *Orchestrator) persistCurrentChangePlanSnapshot() {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return
	}
	path := o.ensureChangePlanPath()
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := writePlanSnapshotPreservingIdentity(plan, path); err != nil {
		logging.Warning("[orchestrator] ChangePlan snapshot persist failed: %v", err)
		return
	}
	logging.Info("[orchestrator] ChangePlan snapshot persisted: %s", path)
	o.persistImmutablePlanIDSnapshot(plan, path)
}

func writeWorkflowArtifactFileStem(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), ".")
}

// An explicitly imported filename remains a live result alias. Preserve its
// old identity before replacement, including when that filename is itself the
// canonical ID key (or reaches it through a directory symlink). Both snapshot
// writers use this boundary; a failed preservation must leave the alias intact.
func writePlanSnapshotPreservingIdentity(plan *types.ChangePlan, path string) error {
	prior, priorBytes, err := readPlanSnapshotForPreservation(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read prior plan before replacing alias: %w", err)
	}
	if prior != nil && plan != nil && prior.ID != plan.ID {
		stem := writeWorkflowArtifactFileStem(prior.ID)
		if stem == "" {
			return fmt.Errorf("cannot retain prior plan with empty identity")
		}
		identityPath := filepath.Join(filepath.Dir(path), stem+".json")
		if planSnapshotPathsSameFile(identityPath, path) {
			identityPath = retainedPlanIdentityPath(filepath.Dir(path), stem)
		}
		if planSnapshotPathsSameFile(identityPath, path) {
			return fmt.Errorf("retained plan destination aliases the live result")
		}
		if existing, _, readErr := readPlanSnapshotForPreservation(identityPath); readErr == nil && existing.ID != prior.ID {
			return fmt.Errorf("retained plan destination belongs to another identity")
		} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("read retained plan destination: %w", readErr)
		}
		if err := os.MkdirAll(filepath.Dir(identityPath), 0o755); err != nil {
			return fmt.Errorf("create retained plan directory: %w", err)
		}
		// Do not reserialize: normal plan writes upgrade valid legacy shapes
		// and discard unknown fields. Historical originals stay byte-exact.
		if err := types.AtomicWriteFileSync(identityPath, priorBytes, 0o644); err != nil {
			return fmt.Errorf("retain prior plan before replacing alias: %w", err)
		}
		retained, err := os.ReadFile(identityPath)
		if err != nil || !bytes.Equal(priorBytes, retained) {
			return fmt.Errorf("retained prior plan did not round trip unchanged")
		}
	}
	return types.WritePlanToFile(plan, path)
}

// Persistence may update a still-incomplete plan of the same identity. Reading
// its serialized shape here is only for lossless preservation, not admission:
// every source/recovery consumer still uses LoadChangePlanFromFile validation.
func readPlanSnapshotForPreservation(path string) (*types.ChangePlan, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var plan types.ChangePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, nil, err
	}
	return &plan, data, nil
}

func planSnapshotPathsSameFile(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	left, leftErr := os.Stat(a)
	right, rightErr := os.Stat(b)
	return leftErr == nil && rightErr == nil && os.SameFile(left, right)
}

func retainedPlanIdentityPath(dir, stem string) string {
	return filepath.Join(dir, "retained-plans", stem+".json")
}

// A live alias can contain a successor with a different ID. Never return that
// successor for a historical lookup; only the exact ID-addressed retained copy
// is eligible, and normal source/contract/physical-byte checks still apply.
func loadPlanIdentityArtifact(dir, planID string) (*types.ChangePlan, error) {
	stem := writeWorkflowArtifactFileStem(planID)
	for _, path := range []string{filepath.Join(dir, stem+".json"), retainedPlanIdentityPath(dir, stem)} {
		plan, err := types.LoadChangePlanFromFile(path)
		if err == nil && plan != nil && plan.ID == planID {
			return plan, nil
		}
	}
	return nil, fmt.Errorf("exact plan artifact %q unavailable: %w", planID, os.ErrNotExist)
}

// persistImmutablePlanIDSnapshot retains an id-addressed sibling when
// PlanPath is a stable import/result alias that a later replan may overwrite.
// Workflow history and final proof assembly refer to exact plan IDs, not to
// whichever plan owns the alias at the end of the run.
func (o *Orchestrator) persistImmutablePlanIDSnapshot(plan *types.ChangePlan, aliasPath string) {
	if plan == nil {
		return
	}
	stem := writeWorkflowArtifactFileStem(plan.ID)
	dir := filepath.Dir(aliasPath)
	if stem == "" || dir == "" {
		return
	}
	identityPath := filepath.Join(dir, stem+".json")
	if filepath.Clean(identityPath) == filepath.Clean(aliasPath) {
		return
	}
	if err := types.WritePlanToFile(plan, identityPath); err != nil {
		logging.Warning("[orchestrator] ChangePlan immutable snapshot persist failed: %v", err)
		return
	}
	logging.Info("[orchestrator] ChangePlan immutable snapshot persisted: %s", identityPath)
}
