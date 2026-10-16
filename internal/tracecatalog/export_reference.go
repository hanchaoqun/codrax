package tracecatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// These are resource ceilings, not evidence coverage or query-result limits.
const MaxExportFileBytes = 16 << 20
const MaxExportMaterialBytes = 64 << 20
const MaxExportFiles = 512

var ErrExportReferenceStale = errors.New("result reference changed after query completion")

type exportFileBinding struct {
	path, root, sha256 string
	generation         filegeneration.Identity
}

type exportAttemptBinding struct {
	queryID, attemptDigest string
	files                  map[string]exportFileBinding
	issues                 map[string]string
}

// CaptureExportReferences is called only at the live query producer's completion
// boundary. It binds original result files, never an input source or JSON replay.
// The host-owned output root is the same root used by that producer to write its
// payloads. Missing/oversized references affect later export, not query outcome.
// Its private bindings are shared by live clones and deliberately not serialized.
func (c *Catalog) CaptureExportReferences(ctx context.Context, queryID, attemptID, outputRoot string) {
	if c == nil || c.run == nil {
		return
	}
	c.mu.Lock()
	q, ok := c.queries[queryID]
	q = cloneQuery(q)
	c.mu.Unlock()
	if !ok {
		return
	}
	var attempt *Attempt
	for i := range q.Attempts {
		if q.Attempts[i].ID == attemptID {
			a := q.Attempts[i]
			attempt = &a
			break
		}
	}
	if attempt == nil {
		return
	}
	b := exportAttemptBinding{queryID: queryID, attemptDigest: digest("", *attempt), files: map[string]exportFileBinding{}, issues: map[string]string{}}
	for _, ref := range []string{attempt.PayloadRef, attempt.RawRef} {
		if ref == "" {
			continue
		}
		if _, seen := b.files[ref]; seen {
			continue
		}
		binding, _, err := readExportFile(ctx, outputRoot, ref, nil)
		if err != nil {
			b.issues[ref] = boundedMessage(err.Error())
			continue
		}
		b.files[ref] = binding
	}
	c.run.exportMu.Lock()
	defer c.run.exportMu.Unlock()
	if c.run.exportRefs == nil {
		c.run.exportRefs = map[string]exportAttemptBinding{}
	}
	// A repeated producer hook cannot replace the bytes bound to an attempt.
	if _, exists := c.run.exportRefs[attemptID]; !exists {
		c.run.exportRefs[attemptID] = b
	}
}

func readExportFile(ctx context.Context, outputRoot, path string, expected *exportFileBinding) (exportFileBinding, []byte, error) {
	if err := ctx.Err(); err != nil {
		return exportFileBinding{}, nil, err
	}
	root, err := filepath.EvalSymlinks(outputRoot)
	if err != nil || strings.TrimSpace(outputRoot) == "" {
		return exportFileBinding{}, nil, fmt.Errorf("producer output root unavailable")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return exportFileBinding{}, nil, err
	}
	if !filepath.IsAbs(path) {
		return exportFileBinding{}, nil, fmt.Errorf("result reference is not an absolute producer path")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return exportFileBinding{}, nil, err
	}
	// A host output root itself may use a platform alias (/var on macOS).
	// Permit exactly that root normalization, not child symlinks or escapes.
	originalRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		return exportFileBinding{}, nil, err
	}
	rel, err := filepath.Rel(originalRoot, filepath.Clean(path))
	if err != nil || rel == "." || !filepath.IsLocal(rel) || canonical != filepath.Join(root, rel) {
		return exportFileBinding{}, nil, fmt.Errorf("result reference is outside its exact producer output root or traverses a symbolic link")
	}
	f, generation, err := filegeneration.OpenRegularReadOnly(canonical)
	if err != nil {
		return exportFileBinding{}, nil, err
	}
	defer f.Close()
	if generation.Size() > MaxExportFileBytes {
		return exportFileBinding{}, nil, fmt.Errorf("result reference exceeds %d-byte export limit", MaxExportFileBytes)
	}
	if expected != nil && (expected.path != canonical || expected.root != root || !expected.generation.SameVersion(generation)) {
		return exportFileBinding{}, nil, fmt.Errorf("%w: generation differs", ErrExportReferenceStale)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxExportFileBytes+1))
	if err != nil {
		return exportFileBinding{}, nil, err
	}
	if len(data) > MaxExportFileBytes {
		return exportFileBinding{}, nil, fmt.Errorf("result reference exceeds export limit")
	}
	if err := ctx.Err(); err != nil {
		return exportFileBinding{}, nil, err
	}
	after, err := filegeneration.FromFile(f)
	if err != nil || !generation.SameVersion(after) {
		return exportFileBinding{}, nil, fmt.Errorf("result changed while reading")
	}
	current, err := filegeneration.FromPath(canonical)
	if err != nil || !generation.SameVersion(current) {
		return exportFileBinding{}, nil, fmt.Errorf("result path changed while reading")
	}
	currentPath, err := filepath.EvalSymlinks(path)
	if err != nil || currentPath != canonical {
		return exportFileBinding{}, nil, fmt.Errorf("result path changed while reading")
	}
	sum := sha256.Sum256(data)
	got := exportFileBinding{path: canonical, root: root, sha256: hex.EncodeToString(sum[:]), generation: generation}
	if expected != nil && got.sha256 != expected.sha256 {
		return exportFileBinding{}, nil, fmt.Errorf("%w: bytes differ", ErrExportReferenceStale)
	}
	return got, data, nil
}

func (c *Catalog) exportBinding(queryID string, attempt Attempt) (exportAttemptBinding, bool) {
	c.run.exportMu.Lock()
	defer c.run.exportMu.Unlock()
	b, ok := c.run.exportRefs[attempt.ID]
	return b, ok && b.queryID == queryID && b.attemptDigest == digest("", attempt)
}
