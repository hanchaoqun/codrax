package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WritePlanGroupToFile serialises a PlanGroup to disk as
// indented JSON. Atomic write (.tmp + rename) mirrors
// WritePlanToFile / WriteBestPlanReportPair so a process crash
// mid-write leaves the previous on-disk version intact.
//
// Failure is non-fatal at the call site: the orchestrator logs
// a warning and continues. The in-memory group is still
// authoritative for the rest of the Run; the disk copy is
// strictly for crash recovery + REPL inspection.
func WritePlanGroupToFile(g *PlanGroup, path string) error {
	if g == nil {
		return fmt.Errorf("WritePlanGroupToFile: nil group")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WritePlanGroupToFile: empty path")
	}
	if strings.TrimSpace(g.ID) == "" {
		return fmt.Errorf("WritePlanGroupToFile: group.ID empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WritePlanGroupToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("WritePlanGroupToFile: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("WritePlanGroupToFile: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("WritePlanGroupToFile: rename %s: %w", tmp, err)
	}
	return nil
}

// LoadPlanGroupFromFile reads + parses one PlanGroup from disk.
// Returns (nil, nil) when the file does not exist (typical for
// single-phase plans / pre-stage-II plans). Returns a non-nil
// error only when the file exists but cannot be parsed.
//
// Decision-shape gate: a file whose Decision is unknown to the
// current build returns an error so the caller can refuse
// rather than silently misinterpret. Stage II only knows
// "linear"; future stages will add cases.
func LoadPlanGroupFromFile(path string) (*PlanGroup, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("LoadPlanGroupFromFile: read %s: %w", path, err)
	}
	var g PlanGroup
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("LoadPlanGroupFromFile: parse %s: %w", path, err)
	}
	if g.Decision != "" && g.Decision != "linear" {
		return nil, fmt.Errorf("LoadPlanGroupFromFile %s: unsupported group decision %q (this build only knows 'linear'); refusing to load to avoid silent misinterpretation", path, g.Decision)
	}
	return &g, nil
}
