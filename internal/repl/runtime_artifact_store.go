package repl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const runtimeArtifactStoreSchemaVersion = 1

type RuntimeArtifactRef struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Path          string    `json:"path"`
	Bytes         int       `json:"bytes"`
	SHA256        string    `json:"sha256"`
	Source        string    `json:"source,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type RuntimeArtifactSnapshot struct {
	SchemaVersion int                `json:"schema_version"`
	Log           RuntimeArtifactRef `json:"log,omitempty"`
	Trace         RuntimeArtifactRef `json:"trace,omitempty"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

type RuntimeArtifactStore struct {
	dir string
}

func NewRuntimeArtifactStore(root string) *RuntimeArtifactStore {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return &RuntimeArtifactStore{dir: root}
}

func (s *RuntimeArtifactStore) Put(kind, payload, source string) (RuntimeArtifactRef, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return RuntimeArtifactRef{}, fmt.Errorf("runtime artifact store disabled")
	}
	kind = normalizeRuntimeArtifactKind(kind)
	if kind == "" || strings.TrimSpace(payload) == "" {
		return RuntimeArtifactRef{}, fmt.Errorf("runtime artifact: empty kind or payload")
	}
	sum := sha256.Sum256([]byte(payload))
	sha := hex.EncodeToString(sum[:])
	id := fmt.Sprintf("%s-%s", kind, sha[:16])
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return RuntimeArtifactRef{}, err
	}
	path := filepath.Join(s.dir, id+".txt")
	if err := writeFileAtomic(path, []byte(payload), 0o600); err != nil {
		return RuntimeArtifactRef{}, err
	}
	ref := RuntimeArtifactRef{
		SchemaVersion: runtimeArtifactStoreSchemaVersion,
		ID:            id,
		Kind:          kind,
		Path:          path,
		Bytes:         len(payload),
		SHA256:        sha,
		Source:        strings.TrimSpace(source),
		CreatedAt:     time.Now(),
	}
	data, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return RuntimeArtifactRef{}, err
	}
	if err := writeFileAtomic(filepath.Join(s.dir, id+".json"), data, 0o600); err != nil {
		return RuntimeArtifactRef{}, err
	}
	return ref, nil
}

func (s *RuntimeArtifactStore) SaveLatest(snapshot RuntimeArtifactSnapshot) error {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return nil
	}
	if !snapshot.Log.Valid() && !snapshot.Trace.Valid() {
		return nil
	}
	snapshot.SchemaVersion = runtimeArtifactStoreSchemaVersion
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = time.Now()
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.dir, "latest.json"), data, 0o600)
}

func (s *RuntimeArtifactStore) LoadLatest() (RuntimeArtifactSnapshot, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return RuntimeArtifactSnapshot{}, fmt.Errorf("runtime artifact store disabled")
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "latest.json"))
	if err != nil {
		return RuntimeArtifactSnapshot{}, err
	}
	var snapshot RuntimeArtifactSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return RuntimeArtifactSnapshot{}, err
	}
	if snapshot.SchemaVersion != runtimeArtifactStoreSchemaVersion {
		return RuntimeArtifactSnapshot{}, fmt.Errorf("unsupported runtime artifact snapshot schema %d", snapshot.SchemaVersion)
	}
	return snapshot, nil
}

func (s *RuntimeArtifactStore) Load(ref RuntimeArtifactRef, maxBytes int) (string, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return "", fmt.Errorf("runtime artifact store disabled")
	}
	if !ref.Valid() {
		return "", fmt.Errorf("invalid runtime artifact ref")
	}
	path := filepath.Clean(ref.Path)
	root := filepath.Clean(s.dir)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("runtime artifact path escapes store")
	}
	limit := ref.Bytes
	if maxBytes > 0 && (limit <= 0 || maxBytes < limit) {
		limit = maxBytes
	}
	if limit <= 0 {
		limit = maxBytes
	}
	if limit <= 0 {
		limit = 512 * 1024 * 1024
	}
	data, err := readRuntimeArtifactPrefix(path, limit)
	if err != nil {
		return "", err
	}
	if len(data) > limit {
		data = data[:limit]
	}
	if ref.SHA256 != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != ref.SHA256 && len(data) == ref.Bytes {
			return "", fmt.Errorf("runtime artifact checksum mismatch")
		}
	}
	return string(data), nil
}

func readRuntimeArtifactPrefix(path string, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid runtime artifact read limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(limit)))
}

func (r RuntimeArtifactRef) Valid() bool {
	return strings.TrimSpace(r.ID) != "" &&
		normalizeRuntimeArtifactKind(r.Kind) != "" &&
		strings.TrimSpace(r.Path) != ""
}

func normalizeRuntimeArtifactKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "log":
		return "log"
	case "trace", "hitrace", "atrace", "perftrace", "systrace", "ftrace", "perfetto", "tracebundle":
		return "trace"
	default:
		return ""
	}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
