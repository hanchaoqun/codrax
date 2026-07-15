// Package tracebundle provides bounded, generation-bound intake for trace
// bundle manifests. It deliberately validates the generic JSON envelope before
// callers decode a schema so forward-compatible unknown fields cannot bypass
// the manifest resource budget.
package tracebundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const (
	// MaxManifestBytes is the inclusive byte ceiling for one trace bundle
	// manifest. The reader always probes one byte beyond this boundary.
	MaxManifestBytes  int64 = 4 << 20
	manifestReadChunk       = 64 << 10
)

var (
	ErrTooLarge          = errors.New("tracebundle manifest exceeds the 4 MiB limit")
	ErrNotRegular        = errors.New("tracebundle manifest is not a regular file")
	ErrWeakIdentity      = errors.New("tracebundle manifest strong file identity is unavailable")
	ErrGenerationChanged = errors.New("tracebundle manifest generation changed")
	ErrInvalidManifest   = errors.New("tracebundle manifest JSON is invalid")
	ErrClosed            = errors.New("tracebundle manifest snapshot is closed")
)

// Snapshot owns the immutable manifest bytes and the open file description
// which witnessed them. Close releases that authority. A Snapshot is safe for
// concurrent Decode, Validate, and Close calls.
type Snapshot struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	data     []byte
	identity filegeneration.Identity
	closed   bool
}

// Open reads and validates one manifest while retaining the exact file
// generation that supplied its bytes. No Snapshot is returned unless the held
// file and the requested path still name the same strong generation after the
// complete bounded read and JSON preflight.
func Open(ctx context.Context, path string) (_ *Snapshot, err error) {
	if ctx == nil {
		return nil, fmt.Errorf("tracebundle manifest open: context is nil")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	// Reject Win32/NT pipe namespaces before filepath.Abs can delegate to a
	// platform path resolver. The normalized absolute spelling is checked again
	// below so both raw and derived forms share the same closed set.
	if err := preflightManifestPath(path); err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("tracebundle manifest path: %w", err)
	}
	absPath = filepath.Clean(absPath)
	if err := preflightManifestPath(absPath); err != nil {
		return nil, err
	}

	// This first observation rejects directories, FIFOs, devices, and already
	// oversized files before any content handle or allocation is attempted.
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("tracebundle manifest stat: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: path=%q mode=%s", ErrNotRegular, absPath, info.Mode())
	}
	if info.Size() < 0 || info.Size() > MaxManifestBytes {
		return nil, tooLargeError(absPath, info.Size())
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	file, err := openManifestFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("tracebundle manifest open: %w", err)
	}
	defer func() {
		if err != nil {
			if closeErr := file.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("tracebundle manifest cleanup close: %w", closeErr))
			}
		}
	}()

	identity, err := filegeneration.FromFile(file)
	if err != nil {
		return nil, fmt.Errorf("tracebundle manifest held identity: %w", err)
	}
	if !identity.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: path=%q mode=%s", ErrNotRegular, absPath, identity.Mode())
	}
	if identity.Size() < 0 || identity.Size() > MaxManifestBytes {
		return nil, tooLargeError(absPath, identity.Size())
	}
	if !identity.Strong() {
		return nil, fmt.Errorf("%w: path=%q", ErrWeakIdentity, absPath)
	}

	data, err := readManifest(ctx, file, identity.Size(), absPath)
	if err != nil {
		return nil, err
	}
	if err := validateJSONEnvelope(ctx, data); err != nil {
		return nil, err
	}

	confirmed, err := filegeneration.FromFile(file)
	if err != nil {
		return nil, fmt.Errorf("%w: held identity validation: %w", ErrGenerationChanged, err)
	}
	if !confirmed.Strong() {
		return nil, fmt.Errorf("%w: path=%q", ErrWeakIdentity, absPath)
	}
	if int64(len(data)) != identity.Size() || !identity.SameVersion(confirmed) {
		return nil, fmt.Errorf("%w: held file changed while reading path=%q", ErrGenerationChanged, absPath)
	}

	resolved, err := filegeneration.FromPath(absPath)
	if err != nil {
		return nil, fmt.Errorf("%w: path identity validation: %w", ErrGenerationChanged, err)
	}
	if !resolved.Strong() {
		return nil, fmt.Errorf("%w: path=%q", ErrWeakIdentity, absPath)
	}
	if !identity.SameVersion(resolved) {
		return nil, fmt.Errorf("%w: requested path no longer names held file path=%q", ErrGenerationChanged, absPath)
	}

	return &Snapshot{
		file:     file,
		path:     absPath,
		data:     data,
		identity: identity,
	}, nil
}

func readManifest(ctx context.Context, file *os.File, size int64, path string) ([]byte, error) {
	capacity := size + 1
	if capacity > MaxManifestBytes+1 {
		capacity = MaxManifestBytes + 1
	}
	data := make([]byte, 0, int(capacity))
	buf := make([]byte, manifestReadChunk)

	for {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		remaining := MaxManifestBytes + 1 - int64(len(data))
		if remaining <= 0 {
			return nil, tooLargeError(path, int64(len(data)))
		}
		chunk := len(buf)
		if int64(chunk) > remaining {
			chunk = int(remaining)
		}
		n, readErr := file.Read(buf[:chunk])
		if n > 0 {
			data = append(data, buf[:n]...)
			if int64(len(data)) > MaxManifestBytes {
				return nil, tooLargeError(path, int64(len(data)))
			}
		}
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return data, nil
			}
			return nil, fmt.Errorf("tracebundle manifest read: %w", readErr)
		}
		if n == 0 {
			return nil, fmt.Errorf("tracebundle manifest read: %w", io.ErrNoProgress)
		}
	}
}

func tooLargeError(path string, size int64) error {
	return fmt.Errorf("%w: path=%q size=%d limit=%d", ErrTooLarge, path, size, MaxManifestBytes)
}

func contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tracebundle manifest intake canceled: %w", err)
	}
	return nil
}

// Decode unmarshals the already-preflighted bytes into dst. Unknown fields
// remain allowed. Callers should Validate after all generation-dependent work
// and before publishing its result.
func (s *Snapshot) Decode(dst any) error {
	if s == nil {
		return ErrClosed
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	data := s.data
	s.mu.Unlock()
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("tracebundle manifest decode: %w", err)
	}
	return nil
}

// Validate proves that both the retained handle and the requested path still
// identify the generation admitted by Open.
func (s *Snapshot) Validate() error {
	if s == nil {
		return ErrClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.file == nil {
		return ErrClosed
	}

	held, err := filegeneration.FromFile(s.file)
	if err != nil {
		return fmt.Errorf("%w: held identity validation: %w", ErrGenerationChanged, err)
	}
	if !held.Strong() {
		return fmt.Errorf("%w: path=%q", ErrWeakIdentity, s.path)
	}
	if !s.identity.SameVersion(held) {
		return fmt.Errorf("%w: held file path=%q", ErrGenerationChanged, s.path)
	}

	resolved, err := filegeneration.FromPath(s.path)
	if err != nil {
		return fmt.Errorf("%w: path identity validation: %w", ErrGenerationChanged, err)
	}
	if !resolved.Strong() {
		return fmt.Errorf("%w: path=%q", ErrWeakIdentity, s.path)
	}
	if !s.identity.SameVersion(resolved) {
		return fmt.Errorf("%w: requested path no longer names held file path=%q", ErrGenerationChanged, s.path)
	}
	return nil
}

// Close releases the held generation. It is idempotent.
func (s *Snapshot) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	file := s.file
	s.file = nil
	if file == nil {
		return nil
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("tracebundle manifest close: %w", err)
	}
	return nil
}

func (s *Snapshot) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Snapshot) Size() int64 {
	if s == nil {
		return 0
	}
	return s.identity.Size()
}

func (s *Snapshot) Identity() filegeneration.Identity {
	if s == nil {
		return filegeneration.Identity{}
	}
	return s.identity
}
