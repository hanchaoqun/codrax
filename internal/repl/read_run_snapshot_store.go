package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

type ReadRunSnapshotStore struct {
	mu     sync.Mutex
	runDir string
}

type ReadRunSnapshotInfo struct {
	ID               string
	Path             string
	Request          string
	TaskGraphHash    string
	TaskNodeCount    int
	NodeStatusCount  int
	ReadFileCount    int
	AcceptedEvidence int
	ModTime          int64
}

func NewReadRunSnapshotStore(planDir string) *ReadRunSnapshotStore {
	if planDir == "" {
		planDir = ".codrax/plans"
	}
	return &ReadRunSnapshotStore{runDir: filepath.Join(planDir, "read_runs")}
}

func (s *ReadRunSnapshotStore) RunDir() string {
	if s == nil {
		return ""
	}
	return s.runDir
}

func (s *ReadRunSnapshotStore) Save(snapshot *types.ReadRunSnapshot) (string, error) {
	if s == nil {
		return "", fmt.Errorf("ReadRunSnapshotStore.Save: nil store")
	}
	if snapshot == nil {
		return "", fmt.Errorf("ReadRunSnapshotStore.Save: nil snapshot")
	}
	normalized := types.NormalizeReadRunSnapshot(*snapshot)
	if strings.TrimSpace(normalized.RunID) == "" {
		return "", fmt.Errorf("ReadRunSnapshotStore.Save: run_id empty")
	}
	if !validStoreIDPattern.MatchString(normalized.RunID) {
		return "", fmt.Errorf("ReadRunSnapshotStore.Save: invalid run id %q", normalized.RunID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.runDir, 0o755); err != nil {
		return "", fmt.Errorf("ReadRunSnapshotStore.Save: mkdir %s: %w", s.runDir, err)
	}
	path := filepath.Join(s.runDir, normalized.RunID+".json")
	if err := types.WriteReadRunSnapshotToFile(&normalized, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *ReadRunSnapshotStore) Load(id string) (*types.ReadRunSnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("ReadRunSnapshotStore.Load: nil store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("ReadRunSnapshotStore.Load: empty id")
	}
	if !validStoreIDPattern.MatchString(id) {
		return nil, fmt.Errorf("ReadRunSnapshotStore.Load: invalid run id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return types.LoadReadRunSnapshotFromFile(filepath.Join(s.runDir, id+".json"))
}

func (s *ReadRunSnapshotStore) Clear(id string) error {
	if s == nil {
		return fmt.Errorf("ReadRunSnapshotStore.Clear: nil store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("ReadRunSnapshotStore.Clear: empty id")
	}
	if !validStoreIDPattern.MatchString(id) {
		return fmt.Errorf("ReadRunSnapshotStore.Clear: invalid run id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.runDir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ReadRunSnapshotStore.Clear: remove %s: %w", path, err)
	}
	return nil
}

func (s *ReadRunSnapshotStore) List() ([]ReadRunSnapshotInfo, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ReadRunSnapshotStore.List: read %s: %w", s.runDir, err)
	}
	var out []ReadRunSnapshotInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.tmp") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !validStoreIDPattern.MatchString(id) {
			continue
		}
		path := filepath.Join(s.runDir, name)
		snapshot, err := types.LoadReadRunSnapshotFromFile(path)
		if err != nil || snapshot == nil {
			continue
		}
		info := ReadRunSnapshotInfo{
			ID:               snapshot.RunID,
			Path:             path,
			Request:          snapshot.Request,
			TaskGraphHash:    snapshot.TaskGraphHash,
			TaskNodeCount:    snapshot.TaskNodeCount,
			NodeStatusCount:  len(snapshot.NodeStatuses),
			ReadFileCount:    len(snapshot.ReadSet),
			AcceptedEvidence: len(snapshot.AcceptedEvidence),
		}
		if stat, err := entry.Info(); err == nil {
			info.ModTime = stat.ModTime().Unix()
		}
		out = append(out, info)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ModTime == out[j].ModTime {
			return out[i].ID < out[j].ID
		}
		return out[i].ModTime > out[j].ModTime
	})
	return out, nil
}

func (s *ReadRunSnapshotStore) LoadComparablePrior(snapshot types.ReadRunSnapshot) (*types.ReadRunSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	target := types.NormalizeReadRunSnapshot(snapshot)
	if !readRunSnapshotComparableKeyComplete(target) {
		return nil, nil
	}
	if !validStoreIDPattern.MatchString(target.RunID) {
		return nil, fmt.Errorf("ReadRunSnapshotStore.LoadComparablePrior: invalid run id %q", target.RunID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	targetPath := filepath.Join(s.runDir, target.RunID+".json")
	targetStat, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ReadRunSnapshotStore.LoadComparablePrior: stat %s: %w", targetPath, err)
	}
	entries, err := os.ReadDir(s.runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ReadRunSnapshotStore.LoadComparablePrior: read %s: %w", s.runDir, err)
	}
	targetMod := targetStat.ModTime().UnixNano()
	var best *types.ReadRunSnapshot
	var bestMod int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.tmp") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if id == target.RunID || !validStoreIDPattern.MatchString(id) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		mod := info.ModTime().UnixNano()
		if mod >= targetMod {
			continue
		}
		candidate, err := types.LoadReadRunSnapshotFromFile(filepath.Join(s.runDir, name))
		if err != nil || candidate == nil || !readRunSnapshotsComparable(target, *candidate) {
			continue
		}
		if best == nil || mod > bestMod || (mod == bestMod && candidate.RunID > best.RunID) {
			best = candidate
			bestMod = mod
		}
	}
	return best, nil
}

func readRunSnapshotsComparable(target, candidate types.ReadRunSnapshot) bool {
	target = types.NormalizeReadRunSnapshot(target)
	candidate = types.NormalizeReadRunSnapshot(candidate)
	if target.RunID == "" || candidate.RunID == "" || target.RunID == candidate.RunID {
		return false
	}
	if !readRunSnapshotComparableKeyComplete(target) || !readRunSnapshotComparableKeyComplete(candidate) {
		return false
	}
	return target.RequestHash == candidate.RequestHash &&
		target.TaskGraphHash == candidate.TaskGraphHash &&
		readRunNormalizedRepoRoot(target.RepoRoot) == readRunNormalizedRepoRoot(candidate.RepoRoot)
}

func readRunSnapshotComparableKeyComplete(snapshot types.ReadRunSnapshot) bool {
	snapshot = types.NormalizeReadRunSnapshot(snapshot)
	return snapshot.RunID != "" &&
		snapshot.RequestHash != "" &&
		snapshot.TaskGraphHash != "" &&
		readRunNormalizedRepoRoot(snapshot.RepoRoot) != ""
}

func readRunNormalizedRepoRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	return filepath.Clean(root)
}
