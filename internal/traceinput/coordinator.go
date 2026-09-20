package traceinput

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Coordinator owns successful preparation receipts for one Run. Share this
// pointer with sibling tool/agent contexts, but create a fresh coordinator for
// the next Run. It never turns a named path into a sticky attachment and does
// not delete committed query material when a tool call or Run returns: answers
// may still cite that material. Options must use a durable runtime anchor, not
// a temporary tool-output directory. InputPath is supplied by Prepare instead.
type Coordinator struct {
	mu       sync.Mutex
	opts     Options
	convert  converter
	ready    map[string]*coordinatedMaterial
	aliases  map[string]string
	inflight map[string]*preparationFlight
}

type preparationFlight struct {
	done chan struct{}
	err  error
}

type coordinatedMaterial struct {
	material        *attachment.TraceMaterial
	version         tracequery.TraceSourceVersion
	sourceCanonical string
	queryCanonical  string
}

func NewCoordinator(opts Options) *Coordinator {
	return newCoordinator(opts, hitraceconv.PrepareFile)
}

func newCoordinator(opts Options, convert converter) *Coordinator {
	opts.InputPath = ""
	return &Coordinator{
		opts: opts, convert: convert,
		ready: make(map[string]*coordinatedMaterial), aliases: make(map[string]string),
		inflight: make(map[string]*preparationFlight),
	}
}

// PreparedMaterials returns a stable, duplicate-free snapshot of receipts
// already committed by this coordinator. It performs no filesystem access or
// preparation and confers no new authority. Consumers must Validate each
// receipt before using it; invalidated successes deliberately remain present
// so Prepare cannot silently replace their generation within the same Run.
func (c *Coordinator) PreparedMaterials() []*attachment.TraceMaterial {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	seen := make(map[*attachment.TraceMaterial]bool, len(c.ready))
	materials := make([]*attachment.TraceMaterial, 0, len(c.ready))
	for _, entry := range c.ready {
		if entry != nil && !seen[entry.material] {
			seen[entry.material] = true
			materials = append(materials, entry.material)
		}
	}
	c.mu.Unlock()
	sort.Slice(materials, func(i, j int) bool {
		if materials[i].SourcePath() != materials[j].SourcePath() {
			return materials[i].SourcePath() < materials[j].SourcePath()
		}
		return materials[i].QueryPath() < materials[j].QueryPath()
	})
	return materials
}

// Prepare shares one in-flight preparation for each exact canonical source
// path. A canceled waiter does not cancel another caller's work. The leader's
// context owns its preparation; if that leader is canceled, cleanup finishes
// before waiters are released, and a still-live waiter may become the next
// leader. Other failures are shared with current waiters but never negatively
// cached, so a later call may retry without a new coordinator.
//
// Successful receipts are indexed by their original and query paths, including
// exact canonical spellings. A seen symlink spelling is bound to its first
// canonical target and cannot switch captures during this Run. Every
// hit validates all bound file generations and the exact source universe
// selected by the query engine (including an admitted sibling bundle). An
// invalidated successful receipt
// stays in the map and fails closed instead of silently mixing a newly
// converted capture into this Run's existing observations. No basename,
// directory, extension or file-content heuristic merges capture identities.
func (c *Coordinator) Prepare(ctx context.Context, absolutePath string) (*attachment.TraceMaterial, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.convert == nil {
		return nil, fmt.Errorf("trace input coordinator is not initialized")
	}
	if filegeneration.IsWindowsNamedPipePath(absolutePath) || !filepath.IsAbs(absolutePath) {
		return nil, fmt.Errorf("trace preparation requires an absolute regular-file path: %q", absolutePath)
	}
	path := filepath.Clean(absolutePath)
	key := preparationPathKey(path)
	canonical, err := preparationCanonicalPath(ctx, path)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if entry := c.ready[key]; entry != nil {
			expected := c.aliases[key]
			c.mu.Unlock()
			return entry.validateAlias(ctx, path, expected)
		}
		if entry := c.ready[canonical]; entry != nil {
			// Register the exact spelling even if receipt validation fails: a
			// stale capture must not be retried later by retargeting this alias.
			c.ready[key], c.aliases[key] = entry, canonical
			c.mu.Unlock()
			return entry.validateAlias(ctx, path, canonical)
		}
		if flight := c.inflight[canonical]; flight != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-flight.done:
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if flight.err != nil && !errors.Is(flight.err, context.Canceled) && !errors.Is(flight.err, context.DeadlineExceeded) {
				return nil, flight.err
			}
			if err := validatePreparationAlias(ctx, path, canonical); err != nil {
				return nil, err
			}
			continue // cache hit, or retry after the canceled leader's rollback
		}
		flight := &preparationFlight{done: make(chan struct{})}
		c.inflight[canonical] = flight
		c.mu.Unlock()

		entry, err := c.prepareOne(ctx, path, canonical)
		c.mu.Lock()
		if err == nil {
			err = c.publishLocked(entry)
		}
		flight.err = err
		delete(c.inflight, canonical)
		close(flight.done)
		c.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return entry.validateAlias(ctx, path, canonical)
	}
}

func (c *Coordinator) publishLocked(entry *coordinatedMaterial) error {
	bindings := map[string]string{
		preparationPathKey(entry.material.SourcePath()): entry.sourceCanonical,
		entry.sourceCanonical:                           entry.sourceCanonical,
		preparationPathKey(entry.material.QueryPath()):  entry.queryCanonical,
		entry.queryCanonical:                            entry.queryCanonical,
	}
	for key := range bindings {
		if prior := c.ready[key]; prior != nil && prior != entry {
			return fmt.Errorf("trace path already belongs to another prepared receipt: %q", key)
		}
	}
	for key, target := range bindings {
		c.ready[key], c.aliases[key] = entry, target
	}
	return nil
}

func (c *Coordinator) prepareOne(ctx context.Context, path, canonical string) (entry *coordinatedMaterial, err error) {
	opts := c.opts
	opts.InputPath = path
	pending, err := begin(ctx, opts, c.convert)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, pending.Discard()) }()
	// Freeze the engine's selected universe while preparation still owns its
	// unpublished output. Plain text can select a provenance-bound sibling
	// bundle; checking only the original text generation would miss that input.
	query := pending.material.QueryPath()
	version, err := tracequery.CaptureTraceSourceVersionContext(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := validatePreparationAlias(ctx, path, canonical); err != nil {
		return nil, err
	}
	queryCanonical, err := preparationCanonicalPath(ctx, query)
	if err != nil {
		return nil, err
	}
	material, err := pending.Commit(ctx)
	if err != nil {
		return nil, err
	}
	return &coordinatedMaterial{material: material, version: version, sourceCanonical: canonical, queryCanonical: queryCanonical}, nil
}

func (entry *coordinatedMaterial) validateAlias(ctx context.Context, path, canonical string) (*attachment.TraceMaterial, error) {
	if err := validatePreparationAlias(ctx, path, canonical); err != nil {
		return nil, err
	}
	material, err := entry.validate(ctx)
	if err != nil {
		return nil, err
	}
	if err := validatePreparationAlias(ctx, path, canonical); err != nil {
		return nil, err
	}
	return material, nil
}

func (entry *coordinatedMaterial) validate(ctx context.Context) (*attachment.TraceMaterial, error) {
	material := entry.material
	if err := material.Validate(ctx, material.Preview()); err != nil {
		return nil, err
	}
	if err := entry.version.ValidateContext(ctx, material.QueryPath()); err != nil {
		return nil, err
	}
	return material, nil
}

func preparationPathKey(path string) string {
	// Lexical aliases remain exact even on Windows: a directory can opt into
	// case-sensitive entries. Only filesystem-verified canonical paths share
	// a receipt; globally folding the full path would merge distinct entries.
	return filepath.Clean(path)
}

func preparationCanonicalPath(ctx context.Context, path string) (string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	// EvalSymlinks preserves the caller's case on macOS, even on a
	// case-insensitive volume. Resolve the actual directory-entry spelling
	// component by component so another spelling cannot re-prepare a stale
	// success. Exact entries win: independent case-sensitive entries and
	// distinct hardlink directory entries remain independent inputs.
	canonical = filepath.Clean(canonical)
	if !filepath.IsAbs(canonical) {
		return "", fmt.Errorf("trace canonical path is not absolute: %q", canonical)
	}
	volume := filepath.VolumeName(canonical)
	tail := strings.TrimPrefix(canonical[len(volume):], string(filepath.Separator))
	if runtime.GOOS == "windows" && len(volume) == 2 && volume[1] == ':' {
		// Drive letters follow Windows' case-insensitive namespace;
		// directory and filename case below that root is never folded.
		// UNC spelling remains conservative until native platform coverage.
		volume = strings.ToUpper(volume)
	}
	root := volume + string(filepath.Separator)
	current := root
	for _, component := range strings.Split(tail, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		name, err := preparationDirectoryEntry(ctx, current, component)
		if err != nil {
			return "", err
		}
		current = filepath.Join(current, name)
	}
	return preparationPathKey(current), nil
}

// Case folding only narrows directory-entry candidates. The authority is the
// exact selected filesystem object, not case similarity, equal bytes or a
// shared inode elsewhere. ReadDir is paged to keep large directories bounded
// in memory; cancellation is observed between pages and entries.
func preparationDirectoryEntry(ctx context.Context, parent, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	requested := filepath.Join(parent, name)
	wanted, err := os.Lstat(requested)
	if err != nil {
		return "", err
	}
	if wanted.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("trace canonical entry changed to a symlink: %q", requested)
	}
	dir, err := os.Open(parent)
	if err != nil {
		return "", fmt.Errorf("trace source identity requires listing parent directory %q: %w", parent, err)
	}
	defer dir.Close()
	parentInfo, err := dir.Stat()
	if err != nil {
		return "", err
	}
	finish := func(entryName string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		currentParent, err := os.Stat(parent)
		if err != nil {
			return "", err
		}
		current, err := os.Lstat(filepath.Join(parent, entryName))
		if err != nil {
			return "", err
		}
		selected, err := os.Lstat(requested)
		if err != nil {
			return "", err
		}
		if !os.SameFile(parentInfo, currentParent) || !os.SameFile(wanted, current) || !os.SameFile(wanted, selected) {
			return "", fmt.Errorf("trace directory entry changed during path resolution: %q", requested)
		}
		return entryName, nil
	}
	var candidate string
	ambiguous := false
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		entries, readErr := dir.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if entry.Name() == name {
				return finish(name)
			}
			if !strings.EqualFold(entry.Name(), name) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return "", err
			}
			if os.SameFile(wanted, info) {
				if candidate != "" && candidate != entry.Name() {
					ambiguous = true
				}
				candidate = entry.Name()
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return "", fmt.Errorf("trace source identity requires listing parent directory %q: %w", parent, readErr)
			}
			break
		}
	}
	if candidate == "" || ambiguous {
		return "", fmt.Errorf("trace path has no unique physical directory entry: %q", requested)
	}
	return finish(candidate)
}

func validatePreparationAlias(ctx context.Context, path, expected string) error {
	current, err := preparationCanonicalPath(ctx, path)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("trace path changed its canonical target during this run: %q", path)
	}
	return nil
}
