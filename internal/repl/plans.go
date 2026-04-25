package repl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PlanStore is the REPL's persistent index of ChangePlans. Single-
// shot mode writes plan JSONs via cmd/root.go's writePlanFile and
// then exits — no index needed. REPL mode needs persistence so:
//
//   - Users can inspect their last plan via `/plan show` without
//     re-running the planner.
//   - `/plan clear` removes stale pending state without wiping
//     files (the files stay on disk as an audit trail).
//   - Multiple plans from one session coexist (e.g. user refined
//     the request and emitted a new plan; PlanStore keeps both).
//
// Thread safety: REPL is single-goroutine per process but the
// store's methods use a mutex anyway because the same directory
// may be shared with a single-shot invocation running in parallel
// (two shells, two processes) and file operations are not atomic.
// Each plan lives in its own <id>.json so writes do not conflict,
// but the mutex serialises the metadata cache.
//
// Format: plans live as JSON under <planDir>/<id>.json. plan.ID
// embeds unix-nano + pid (format: plan-<nano>-<pid>) so no two
// files ever collide. The directory is created on first Save; a
// fresh user with an empty baseDir sees no residue.
//
// B0 minimum: Load, Save, List, Clear. B1 will add richer metadata
// indexing (per-session filter, pending-vs-applied filter,
// trigger-turn lookup) once the write-mode workflows stabilise.
type PlanStore struct {
	mu      sync.Mutex
	planDir string // absolute path, e.g. <CWD>/.codrax/plans
}

// NewPlanStore constructs a PlanStore rooted at planDir. The
// directory is NOT created eagerly — Save does it on first write
// so read-only inspection of a fresh install doesn't leak an empty
// directory. Absolute paths pass through; relative paths stay
// relative (caller is responsible for absolutising upstream, which
// cmd/root.go does).
func NewPlanStore(planDir string) *PlanStore {
	if planDir == "" {
		planDir = ".codrax/plans"
	}
	return &PlanStore{planDir: planDir}
}

// PlanDir returns the directory this store manages. Used by the
// REPL banner / /plan output to show users where their plans live.
func (s *PlanStore) PlanDir() string {
	if s == nil {
		return ""
	}
	return s.planDir
}

// Save writes plan as JSON to <planDir>/<plan.ID>.json. The parent
// directory is MkdirAll-ed. Plan content is re-marshalled with
// indent for operator readability. Returns the written absolute
// path or an error with context.
//
// Idempotency: a second Save with the same plan.ID OVERWRITES the
// prior file. Plans are immutable-once-applied conceptually but
// the REPL's /plan clear workflow sometimes regenerates to the
// same ID; a stricter "no-overwrite" rule would force the caller
// to double-bookkeep.
func (s *PlanStore) Save(plan *types.ChangePlan) (string, error) {
	if s == nil {
		return "", fmt.Errorf("PlanStore.Save: nil store")
	}
	if plan == nil {
		return "", fmt.Errorf("PlanStore.Save: nil plan")
	}
	if strings.TrimSpace(plan.ID) == "" {
		return "", fmt.Errorf("PlanStore.Save: plan.ID must be non-empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.planDir, 0o755); err != nil {
		return "", fmt.Errorf("PlanStore.Save: mkdir %s: %w", s.planDir, err)
	}
	path := filepath.Join(s.planDir, plan.ID+".json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", fmt.Errorf("PlanStore.Save: marshal plan %s: %w", plan.ID, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("PlanStore.Save: write %s: %w", path, err)
	}
	return path, nil
}

// Load reads <planDir>/<id>.json and decodes the ChangePlan. Returns
// (nil, err) when the file is missing, unreadable, or the JSON does
// not match the ChangePlan schema. Callers typically use this in
// /plan show and /approve workflows; the REPL treats a load error
// as "plan was deleted by hand" and prompts the user.
func (s *PlanStore) Load(id string) (*types.ChangePlan, error) {
	if s == nil {
		return nil, fmt.Errorf("PlanStore.Load: nil store")
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("PlanStore.Load: empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.planDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("PlanStore.Load: read %s: %w", path, err)
	}
	var plan types.ChangePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("PlanStore.Load: unmarshal %s: %w", path, err)
	}
	return &plan, nil
}

// PlanInfo is the lightweight metadata PlanStore.List returns for
// each plan. Avoids loading full plan JSON when the caller only
// needs to enumerate (e.g. /plan ls output).
type PlanInfo struct {
	ID      string // plan-<nano>-<pid>
	Path    string // absolute file path
	SizeB   int64  // file size in bytes
	ModTime int64  // last modification unix-nano
	Status  string // PlanStatus* from types/change_plan.go; empty when JSON
	// was unreadable (List logs + continues so one corrupt file
	// doesn't break enumeration).
}

// List enumerates every <id>.json file under planDir and returns
// their metadata sorted by ModTime descending (newest first). A
// missing planDir returns (nil, nil) — fresh user has no plans and
// that's not an error.
//
// Errors during per-file stat are logged (via fmt.Fprintf to stderr)
// but do not abort — one corrupt entry should not block the /plan
// show command.
func (s *PlanStore) List() ([]PlanInfo, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.planDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("PlanStore.List: read dir %s: %w", s.planDir, err)
	}
	var out []PlanInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		// Skip sibling ChangeReport files; they live next to plans
		// as <id>.report.json and would otherwise double each plan
		// row in /plan list + /history.
		if strings.HasSuffix(name, ".report.json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlanStore.List: stat %s: %v\n", name, err)
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		// Peek into the plan JSON for Status. List already walks
		// the whole dir so one extra read per plan is cheap; the
		// alternative (holding an in-memory cache) adds complexity
		// without speed benefit at typical plan counts (<100).
		status := ""
		if data, rerr := os.ReadFile(filepath.Join(s.planDir, name)); rerr == nil {
			var probe struct {
				Status string `json:"status"`
			}
			if jerr := json.Unmarshal(data, &probe); jerr == nil {
				status = probe.Status
			}
		}
		out = append(out, PlanInfo{
			ID:      id,
			Path:    filepath.Join(s.planDir, name),
			SizeB:   info.Size(),
			ModTime: info.ModTime().UnixNano(),
			Status:  status,
		})
	}
	// Newest-first so the user's recent plans appear at the top of
	// /plan show listings.
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime > out[j].ModTime
	})
	return out, nil
}

// UpdateStatus reads the plan JSON, mutates the Status (and
// optionally AppliedCommitSHA / WorktreePath / AppliedAt when
// the caller passes non-zero values), and rewrites the file.
// Used by the apply stage hook + the verify stage hook to track plan lifecycle
// so /plan list can show which plans were applied vs failed.
//
// Idempotent: calling with the same status re-serialises without
// behavioural change. Missing plan file → error with the path;
// the caller typically logs and proceeds because failure to
// persist status doesn't break the apply / verify flow itself.
//
// Thread-safe against concurrent reads via s.mu but NOT against
// concurrent writers; the REPL is single-goroutine so this is
// fine for codrax's current usage model.
func (s *PlanStore) UpdateStatus(id, status string, appliedAt *time.Time) error {
	if s == nil {
		return fmt.Errorf("PlanStore.UpdateStatus: nil store")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("PlanStore.UpdateStatus: empty id")
	}
	plan, err := s.Load(id)
	if err != nil {
		return fmt.Errorf("UpdateStatus load: %w", err)
	}
	plan.Status = status
	if appliedAt != nil {
		plan.AppliedAt = appliedAt
	}
	if _, err := s.Save(plan); err != nil {
		return fmt.Errorf("UpdateStatus save: %w", err)
	}
	return nil
}

// Clear removes <id>.json from the plan directory. Returns nil when
// the file is already absent (idempotent). Used by /plan clear to
// discard a plan the user has decided against.
//
// Side effects: removes the JSON file. Does NOT remove any git
// worktree artifacts — those are managed by the worktree package's
// Discard / PruneDeadSessions path independent of plan files.
func (s *PlanStore) Clear(id string) error {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("PlanStore.Clear: empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.planDir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("PlanStore.Clear: remove %s: %w", path, err)
	}
	return nil
}
