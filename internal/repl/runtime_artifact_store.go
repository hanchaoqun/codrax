package repl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/types"
)

const runtimeArtifactStoreSchemaVersion = 1

// latest.json is control-plane metadata, not an artifact payload. Keep its
// admission ceiling deliberately small so a corrupt or attacker-controlled
// snapshot cannot turn REPL startup into an unbounded allocation. A normal
// snapshot is well below 4 KiB; 64 KiB leaves ample room for long source paths
// while remaining a strict metadata-only bound.
const runtimeArtifactLatestMaxBytes int64 = 64 << 10

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

func (s *RuntimeArtifactStore) LoadLatest() (snapshot RuntimeArtifactSnapshot, err error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return RuntimeArtifactSnapshot{}, fmt.Errorf("runtime artifact store disabled")
	}
	path := filepath.Join(s.dir, "latest.json")
	file, opened, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return RuntimeArtifactSnapshot{}, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			snapshot = RuntimeArtifactSnapshot{}
			err = errors.Join(err, fmt.Errorf("close runtime artifact snapshot: %w", closeErr))
		}
	}()
	return readRuntimeArtifactSnapshot(path, file, opened)
}

// readRuntimeArtifactSnapshot consumes only the already-open descriptor. The
// final held/path checks make an atomic replacement, in-place rewrite, or
// symlink retarget during the read a hard failure instead of accepting a JSON
// document whose bytes and pathname came from different generations.
func readRuntimeArtifactSnapshot(path string, file *os.File, opened filegeneration.Identity) (RuntimeArtifactSnapshot, error) {
	var provisionalErr error
	if !opened.Strong() {
		provisionalErr = fmt.Errorf("runtime artifact snapshot strong generation identity is unavailable")
	} else if opened.Size() <= 0 {
		provisionalErr = fmt.Errorf("runtime artifact snapshot is empty")
	} else if opened.Size() > runtimeArtifactLatestMaxBytes {
		provisionalErr = fmt.Errorf(
			"runtime artifact snapshot exceeds metadata limit: got=%d max=%d",
			opened.Size(), runtimeArtifactLatestMaxBytes,
		)
	}

	var snapshot RuntimeArtifactSnapshot
	if provisionalErr == nil {
		data := make([]byte, int(opened.Size()))
		if _, readErr := io.ReadFull(io.NewSectionReader(file, 0, opened.Size()), data); readErr != nil {
			provisionalErr = fmt.Errorf("read runtime artifact snapshot: %w", readErr)
		} else if decodeErr := decodeRuntimeArtifactSnapshot(data, &snapshot); decodeErr != nil {
			provisionalErr = decodeErr
		}
	}

	// Generation failures are authoritative and therefore dominate any JSON or
	// schema error observed from bytes read while the file was changing.
	final, heldErr := filegeneration.FromFile(file)
	if heldErr == nil && !opened.SameVersion(final) {
		heldErr = fmt.Errorf("runtime artifact snapshot held generation changed")
	}
	bound, bindingErr := filegeneration.FromPath(path)
	if bindingErr == nil && !opened.SameVersion(bound) {
		bindingErr = fmt.Errorf("runtime artifact snapshot path generation changed")
	}
	if heldErr != nil || bindingErr != nil {
		return RuntimeArtifactSnapshot{}, errors.Join(heldErr, bindingErr, provisionalErr)
	}
	if provisionalErr != nil {
		return RuntimeArtifactSnapshot{}, provisionalErr
	}
	return snapshot, nil
}

func decodeRuntimeArtifactSnapshot(data []byte, snapshot *RuntimeArtifactSnapshot) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(snapshot); err != nil {
		return fmt.Errorf("decode runtime artifact snapshot: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode runtime artifact snapshot: trailing JSON value")
		}
		return fmt.Errorf("decode runtime artifact snapshot trailing content: %w", err)
	}
	if err := validateRuntimeArtifactSnapshot(*snapshot); err != nil {
		return err
	}
	return nil
}

func validateRuntimeArtifactSnapshot(snapshot RuntimeArtifactSnapshot) error {
	if snapshot.SchemaVersion != runtimeArtifactStoreSchemaVersion {
		return fmt.Errorf("unsupported runtime artifact snapshot schema %d", snapshot.SchemaVersion)
	}
	if snapshot.UpdatedAt.IsZero() {
		return fmt.Errorf("runtime artifact snapshot has no updated_at")
	}
	validRefs := 0
	for _, candidate := range []struct {
		lane string
		kind string
		ref  RuntimeArtifactRef
	}{
		{lane: "log", kind: "log", ref: snapshot.Log},
		{lane: "trace", kind: "trace", ref: snapshot.Trace},
	} {
		if candidate.ref == (RuntimeArtifactRef{}) {
			continue
		}
		if !candidate.ref.Valid() {
			return fmt.Errorf("runtime artifact snapshot has invalid %s ref", candidate.lane)
		}
		if candidate.ref.SchemaVersion != runtimeArtifactStoreSchemaVersion {
			return fmt.Errorf(
				"unsupported runtime artifact snapshot %s ref schema %d",
				candidate.lane, candidate.ref.SchemaVersion,
			)
		}
		if normalizeRuntimeArtifactKind(candidate.ref.Kind) != candidate.kind {
			return fmt.Errorf("runtime artifact snapshot %s lane has kind %q", candidate.lane, candidate.ref.Kind)
		}
		if candidate.ref.Bytes <= 0 {
			return fmt.Errorf("runtime artifact snapshot %s ref has invalid byte size %d", candidate.lane, candidate.ref.Bytes)
		}
		if len(candidate.ref.SHA256) != sha256.Size*2 || candidate.ref.SHA256 != strings.ToLower(candidate.ref.SHA256) {
			return fmt.Errorf("runtime artifact snapshot %s ref has invalid sha256", candidate.lane)
		}
		decoded, decodeErr := hex.DecodeString(candidate.ref.SHA256)
		if decodeErr != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("runtime artifact snapshot %s ref has invalid sha256", candidate.lane)
		}
		if candidate.ref.CreatedAt.IsZero() {
			return fmt.Errorf("runtime artifact snapshot %s ref has no created_at", candidate.lane)
		}
		validRefs++
	}
	if validRefs == 0 {
		return fmt.Errorf("runtime artifact snapshot has no artifact refs")
	}
	return nil
}

func (s *RuntimeArtifactStore) Load(ref RuntimeArtifactRef, maxBytes int) (payload string, err error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return "", fmt.Errorf("runtime artifact store disabled")
	}
	if !ref.Valid() {
		return "", fmt.Errorf("invalid runtime artifact ref")
	}
	if ref.SchemaVersion != runtimeArtifactStoreSchemaVersion {
		return "", fmt.Errorf("unsupported runtime artifact ref schema %d", ref.SchemaVersion)
	}
	if ref.Bytes <= 0 {
		return "", fmt.Errorf("runtime artifact ref has invalid byte size %d", ref.Bytes)
	}
	if len(ref.SHA256) != sha256.Size*2 || ref.SHA256 != strings.ToLower(ref.SHA256) {
		return "", fmt.Errorf("runtime artifact ref has invalid sha256")
	}
	expectedSHA, err := hex.DecodeString(ref.SHA256)
	if err != nil || len(expectedSHA) != sha256.Size {
		return "", fmt.Errorf("runtime artifact ref has invalid sha256")
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

	// OpenRegularReadOnly owns the platform-specific safety boundary: Unix-like
	// systems use O_NONBLOCK and Windows rejects named-pipe namespaces. Keep the
	// returned descriptor as the sole content authority until its held
	// generation and the path binding have both been rechecked below.
	file, opened, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return "", fmt.Errorf("restore runtime artifact %q: %w", ref.ID, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			payload = ""
			err = errors.Join(err, fmt.Errorf("close runtime artifact %q: %w", ref.ID, closeErr))
		}
	}()

	kind := attachment.KindLog
	if normalizeRuntimeArtifactKind(ref.Kind) == "trace" {
		kind = attachment.KindTrace
	}
	var provisionalErr error
	if !opened.Strong() {
		provisionalErr = fmt.Errorf("runtime artifact strong generation identity is unavailable for %q", ref.ID)
	} else if opened.Size() != int64(ref.Bytes) {
		provisionalErr = fmt.Errorf(
			"runtime artifact size mismatch for %q: got=%d want=%d",
			ref.ID, opened.Size(), ref.Bytes,
		)
	}
	if provisionalErr == nil {
		maxLineBytes := 0
		if kind == attachment.KindTrace {
			maxLineBytes = attachment.TracePhysicalLineMaxBytes
		}
		if validateErr := attachment.ValidateTextReaderAtFull(
			context.Background(), kind, path, file, opened.Size(), maxLineBytes,
		); validateErr != nil {
			provisionalErr = fmt.Errorf("validate runtime artifact %q: %w", ref.ID, validateErr)
		}
	}

	var prefix []byte
	if provisionalErr == nil {
		prefixBytes := ref.Bytes
		if maxBytes > 0 && maxBytes < prefixBytes {
			// Keep the first dropped byte as a boundary witness for
			// CutPrefixRuneSafe. Capturing exactly maxBytes could end on a
			// partial multi-byte rune and make physical integrity look like
			// cap truncation.
			prefixBytes = maxBytes + 1
		}
		prefix, provisionalErr = measureRuntimeArtifact(file, opened.Size(), prefixBytes, expectedSHA, ref.ID)
	}

	// Identity failures dominate provisional content failures. In particular,
	// replacement or in-place mutation must never be mislabeled as an ordinary
	// checksum/text problem or accepted as cap truncation.
	final, heldErr := filegeneration.FromFile(file)
	if heldErr == nil && !opened.SameVersion(final) {
		heldErr = fmt.Errorf("runtime artifact held generation changed for %q", ref.ID)
	}
	bound, bindingErr := filegeneration.FromPath(path)
	if bindingErr == nil && !opened.SameVersion(bound) {
		bindingErr = fmt.Errorf("runtime artifact path generation changed for %q", ref.ID)
	}
	if heldErr != nil || bindingErr != nil {
		return "", errors.Join(heldErr, bindingErr, provisionalErr)
	}
	if provisionalErr != nil {
		return "", provisionalErr
	}

	payload = string(prefix)
	if maxBytes > 0 {
		payload = types.CutPrefixRuneSafe(payload, maxBytes)
	}
	return payload, nil
}

func measureRuntimeArtifact(file *os.File, size int64, prefixBytes int, expectedSHA []byte, id string) ([]byte, error) {
	if file == nil || size < 0 || prefixBytes < 0 || int64(prefixBytes) > size {
		return nil, fmt.Errorf("runtime artifact %q has invalid measurement bounds", id)
	}
	prefix := make([]byte, prefixBytes)
	digest := sha256.New()
	buffer := make([]byte, 256<<10)
	for offset := int64(0); offset < size; {
		want := int64(len(buffer))
		if remaining := size - offset; remaining < want {
			want = remaining
		}
		n, readErr := file.ReadAt(buffer[:int(want)], offset)
		if n != int(want) {
			if readErr == nil {
				readErr = io.ErrUnexpectedEOF
			}
			return nil, fmt.Errorf(
				"read runtime artifact %q at offset %d: got=%d want=%d: %w",
				id, offset, n, want, readErr,
			)
		}
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("read runtime artifact %q at offset %d: %w", id, offset, readErr)
		}
		_, _ = digest.Write(buffer[:n])
		if offset < int64(prefixBytes) {
			copyEnd := int64(n)
			if remainingPrefix := int64(prefixBytes) - offset; copyEnd > remainingPrefix {
				copyEnd = remainingPrefix
			}
			copy(prefix[int(offset):], buffer[:int(copyEnd)])
		}
		offset += int64(n)
	}
	if actual := digest.Sum(nil); !equalRuntimeArtifactDigest(actual, expectedSHA) {
		return nil, fmt.Errorf("runtime artifact checksum mismatch for %q", id)
	}
	return prefix, nil
}

func equalRuntimeArtifactDigest(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for i := range left {
		difference |= left[i] ^ right[i]
	}
	return difference == 0
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
