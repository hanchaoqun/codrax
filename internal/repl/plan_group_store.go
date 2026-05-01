package repl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PlanGroupStore manages on-disk PlanGroup files (stage II
// multi-phase write-mode containers). Mirrors PlanStore's
// shape; lives at <runtimeAnchor>/plans/groups/.
//
// Single-phase plans never create a group, so this store is
// empty in pre-stage-II flows (back-compat).
//
// Thread-safety: a per-store mutex guards every disk op so
// concurrent /phase show / .Save / .Clear calls from the REPL
// (interactive event handler + REPL Loop slash dispatch) don't
// race. Cross-process safety is whatever the underlying
// filesystem provides — atomic rename guarantees readers never
// see a half-written file, but two processes saving the same
// group ID simultaneously would race on the rename target. Not
// a concern in practice (group IDs embed PID and unix-nano).
type PlanGroupStore struct {
	mu       sync.Mutex
	groupDir string
}

// NewPlanGroupStore constructs a store rooted at <planDir>/groups/.
// Mirror of NewPlanStore — caller passes the same plan dir so
// /plan list and /phase show coexist in the same anchor.
func NewPlanGroupStore(planDir string) *PlanGroupStore {
	if planDir == "" {
		planDir = ".codrax/plans"
	}
	return &PlanGroupStore{
		groupDir: filepath.Join(planDir, "groups"),
	}
}

// GroupDir returns the absolute on-disk directory the store
// uses. Exposed for /phase show banner rendering and tests.
func (s *PlanGroupStore) GroupDir() string {
	if s == nil {
		return ""
	}
	return s.groupDir
}

// Save writes the group to <groupDir>/<group-id>.json
// atomically. Returns the absolute path so callers can stash a
// pendingGroupPath the way they do pendingPlanPath.
func (s *PlanGroupStore) Save(g *types.PlanGroup) (string, error) {
	if s == nil {
		return "", fmt.Errorf("PlanGroupStore.Save: nil store")
	}
	if g == nil {
		return "", fmt.Errorf("PlanGroupStore.Save: nil group")
	}
	if strings.TrimSpace(g.ID) == "" {
		return "", fmt.Errorf("PlanGroupStore.Save: group.ID empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.groupDir, 0o755); err != nil {
		return "", fmt.Errorf("PlanGroupStore.Save: mkdir %s: %w", s.groupDir, err)
	}
	path := filepath.Join(s.groupDir, g.ID+".json")
	if err := types.WritePlanGroupToFile(g, path); err != nil {
		return "", err
	}
	return path, nil
}

// Load reads a group by ID. Returns (nil, nil) when the file
// doesn't exist (caller treats as "no such group").
func (s *PlanGroupStore) Load(id string) (*types.PlanGroup, error) {
	if s == nil {
		return nil, fmt.Errorf("PlanGroupStore.Load: nil store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("PlanGroupStore.Load: empty id")
	}
	if !validStoreIDPattern.MatchString(id) {
		return nil, fmt.Errorf("PlanGroupStore.Load: invalid group id %q (must match [a-zA-Z0-9_-]+)", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.groupDir, id+".json")
	return types.LoadPlanGroupFromFile(path)
}

// Clear deletes a group's on-disk file. Idempotent — missing
// file returns nil. Used by /phase rollback's terminal path
// (group rolled back) and by tests.
func (s *PlanGroupStore) Clear(id string) error {
	if s == nil {
		return fmt.Errorf("PlanGroupStore.Clear: nil store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("PlanGroupStore.Clear: empty id")
	}
	if !validStoreIDPattern.MatchString(id) {
		return fmt.Errorf("PlanGroupStore.Clear: invalid group id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.groupDir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("PlanGroupStore.Clear: remove %s: %w", path, err)
	}
	return nil
}

// GroupInfo is the lightweight metadata List returns. Mirrors
// PlanInfo's shape — same idea (avoid loading full JSON when
// the caller only needs to enumerate).
type GroupInfo struct {
	ID        string
	Path      string
	Status    string
	Decision  string
	Phases    int
	ActiveIdx int
	ModTime   int64
}

// List enumerates every group file under groupDir, newest
// first. A missing dir returns (nil, nil) — fresh user has no
// groups yet, which isn't an error.
func (s *PlanGroupStore) List() ([]GroupInfo, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.groupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("PlanGroupStore.List: read %s: %w", s.groupDir, err)
	}
	var out []GroupInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.tmp") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		// Probe just the cheap fields. If the JSON is malformed,
		// log via the standard library's drop-on-floor pattern
		// (via a log-not-fatal: the rest of the listing should
		// surface even if one entry rotted).
		var probe struct {
			Status    string `json:"status"`
			Decision  string `json:"decision"`
			Phases    []struct{} `json:"phases"`
			ActiveIdx int    `json:"active_idx"`
		}
		if data, rerr := os.ReadFile(filepath.Join(s.groupDir, name)); rerr == nil {
			_ = json.Unmarshal(data, &probe)
		}
		out = append(out, GroupInfo{
			ID:        id,
			Path:      filepath.Join(s.groupDir, name),
			Status:    probe.Status,
			Decision:  probe.Decision,
			Phases:    len(probe.Phases),
			ActiveIdx: probe.ActiveIdx,
			ModTime:   info.ModTime().UnixNano(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime > out[j].ModTime
	})
	return out, nil
}

// FindActiveGroup returns the most recent non-terminal group,
// or (nil, nil) when no group is in flight. /phase show uses
// this when invoked without an explicit group ID — the typical
// case is the operator just had write_analyzer emit a
// sequential proposal and wants to see what's queued.
func (s *PlanGroupStore) FindActiveGroup() (*types.PlanGroup, error) {
	infos, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, inf := range infos {
		if types.IsTerminalGroupStatus(types.PlanGroupStatus(inf.Status)) {
			continue
		}
		g, err := s.Load(inf.ID)
		if err != nil {
			continue
		}
		if g != nil {
			return g, nil
		}
	}
	return nil, nil
}

