package outputdump

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// ExplicitReportVersion describes prior output bytes, not authority to write
// or a retained backup. The actual generation witness stays process-private.
type ExplicitReportVersion struct {
	Exists      bool   `json:"exists"`
	Bytes       int64  `json:"bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Mode        uint32 `json:"mode,omitempty"`
	ModUnixNano int64  `json:"mod_unix_nano,omitempty"`
}

type explicitReportVersion struct {
	ExplicitReportVersion
	generation filegeneration.Identity
	info       os.FileInfo
}

type explicitReportTarget struct {
	path     string
	resolved string
	expected explicitReportVersion
	err      error
}

// A registration serializes its own publications, not external writers.
// Version checks are optimistic checks before commit, not filesystem CAS.
type explicitReportPublication struct {
	mu      sync.Mutex
	active  bool
	targets map[string]*explicitReportTarget
	ops     explicitReportFileOps
}

// The narrow operations seam permits deterministic I/O-failure tests without
// permissions tricks or changing the public WriteResult entry point.
type explicitReportFileOps struct {
	write  func(*os.File, []byte) (int, error)
	sync   func(*os.File) error
	close  func(*os.File) error
	rename func(*os.Root, string, string) error
}

func newExplicitReportPublication(r ExplicitReport) *explicitReportPublication {
	p := &explicitReportPublication{active: true, targets: make(map[string]*explicitReportTarget), ops: explicitReportFileOps{
		write: (*os.File).Write, sync: (*os.File).Sync, close: (*os.File).Close, rename: (*os.Root).Rename,
	}}
	for kind, path := range map[string]string{"markdown": r.MarkdownPath, "html": r.HTMLPath, "root-causes": r.RootCauseJSONPath} {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		target := &explicitReportTarget{path: path}
		target.resolved, target.err = resolveExplicitReportPath(path, 0)
		if target.err == nil {
			target.expected, target.err = captureExplicitReportVersion(context.Background(), target.resolved)
		}
		p.targets[kind] = target
	}
	return p
}

// Resolve existing aliases without creating anything, including a symlink to
// a not-yet-created report. Publication replaces its referent, never the link.
func resolveExplicitReportPath(path string, depth int) (string, error) {
	if depth > 255 {
		return "", fmt.Errorf("explicit report path has too many components or symlinks")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(abs)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(abs), link)
		}
		return resolveExplicitReportPath(link, depth+1)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return "", fmt.Errorf("cannot resolve explicit report parent %q", abs)
	}
	resolved, err := resolveExplicitReportPath(parent, depth+1)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(abs)), nil
}

type explicitReportContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r explicitReportContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func captureExplicitReportVersion(ctx context.Context, path string) (explicitReportVersion, error) {
	var v explicitReportVersion
	if err := ctx.Err(); err != nil {
		return v, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if !info.Mode().IsRegular() {
		return v, fmt.Errorf("explicit report target is not a regular file: %s", path)
	}
	f, generation, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return v, err
	}
	defer f.Close()
	if err := explicitReportSingleLink(f); err != nil {
		return v, err
	}
	info, err = f.Stat()
	if err != nil {
		return v, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, explicitReportContextReader{ctx: ctx, r: f}); err != nil {
		return v, err
	}
	current, err := filegeneration.FromPath(path)
	if err != nil || !generation.MatchesFile(f) || !generation.SameVersion(current) {
		return v, fmt.Errorf("explicit report changed while observing prior version: %s: %w", path, errors.Join(err, errors.New("generation mismatch")))
	}
	v = explicitReportVersion{ExplicitReportVersion: ExplicitReportVersion{
		Exists: true, Bytes: generation.Size(), SHA256: hex.EncodeToString(hash.Sum(nil)),
		Mode: uint32(info.Mode().Perm()), ModUnixNano: generation.ModUnixNano(),
	}, generation: generation, info: info}
	return v, nil
}

func (t *explicitReportTarget) validate(ctx context.Context) error {
	if t.err != nil {
		return t.err
	}
	resolved, err := resolveExplicitReportPath(t.path, 0)
	if err != nil || resolved != t.resolved {
		return fmt.Errorf("explicit report target path changed since registration: %s", t.path)
	}
	current, err := captureExplicitReportVersion(ctx, resolved)
	if err != nil {
		return err
	}
	if current.Exists != t.expected.Exists || (current.Exists &&
		(!current.generation.SameVersion(t.expected.generation) || current.SHA256 != t.expected.SHA256)) {
		return fmt.Errorf("explicit report target changed since registration or previous publication: %s", t.path)
	}
	return nil
}

func publishExplicitReport(ctx context.Context, r ExplicitReport, kind, path string, data []byte) (receipt ExplicitReportWrite) {
	receipt = ExplicitReportWrite{Kind: kind, Path: path}
	defer func() {
		if receipt.Err != nil {
			logging.Warning("[output_dump] write explicit report %s failed: %v", path, receipt.Err)
		}
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	p := r.publication
	if p == nil {
		receipt.Err = fmt.Errorf("explicit report is not registered")
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	target := p.targets[kind]
	if !p.active || target == nil || target.path != path {
		receipt.Err = fmt.Errorf("explicit report registration is not current")
		return
	}
	if receipt.Err = target.validate(ctx); receipt.Err != nil {
		return
	}
	dir := filepath.Dir(target.resolved)
	if receipt.Err = os.MkdirAll(dir, 0o755); receipt.Err != nil {
		return
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		receipt.Err = err
		return
	}
	defer root.Close()
	parentInfo, err := root.Stat(".")
	if err != nil {
		receipt.Err = err
		return
	}
	var random [16]byte
	if _, receipt.Err = rand.Read(random[:]); receipt.Err != nil {
		return
	}
	stage := ".codrax-report-" + hex.EncodeToString(random[:]) + ".tmp"
	mode := os.FileMode(0o644)
	if target.expected.Exists {
		mode = 0o600 // Existing permissions are restored before publication.
	}
	f, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		receipt.Err = err
		return
	}
	defer func() {
		_ = f.Close()
		if !receipt.Published {
			if err := root.Remove(stage); err != nil && !os.IsNotExist(err) {
				receipt.Err = errors.Join(receipt.Err, fmt.Errorf("remove report staging file: %w", err))
			}
		}
	}()
	if target.expected.Exists {
		if receipt.Err = f.Chmod(os.FileMode(target.expected.Mode)); receipt.Err != nil {
			return
		}
	}
	n, err := p.ops.write(f, data)
	if err != nil {
		receipt.Err = err
		return
	}
	if n != len(data) {
		receipt.Err = io.ErrShortWrite
		return
	}
	if receipt.Err = p.ops.sync(f); receipt.Err != nil {
		return
	}
	stageInfo, err := f.Stat()
	if err != nil {
		receipt.Err = err
		return
	}
	if receipt.Err = p.ops.close(f); receipt.Err != nil {
		return
	}
	if receipt.Err = target.validate(ctx); receipt.Err != nil {
		return
	}
	currentParent, err := os.Stat(dir)
	if err != nil || !os.SameFile(parentInfo, currentParent) {
		receipt.Err = fmt.Errorf("explicit report parent changed before publication")
		return
	}
	if receipt.Err = ctx.Err(); receipt.Err != nil {
		return
	}
	if receipt.Err = p.ops.rename(root, stage, filepath.Base(target.resolved)); receipt.Err != nil {
		return
	}
	// Rename is the publication point. Neither a late cancellation nor a
	// later external edit can undo this committed delivery receipt.
	previous := target.expected.ExplicitReportVersion
	digest := sha256.Sum256(data)
	receipt.Published, receipt.Bytes, receipt.SHA256, receipt.Previous = true, int64(len(data)), hex.EncodeToString(digest[:]), &previous
	next, err := captureExplicitReportVersion(context.Background(), target.resolved)
	if err == nil && next.SHA256 == receipt.SHA256 && os.SameFile(stageInfo, next.info) {
		target.expected = next
	} else {
		target.err = fmt.Errorf("explicit report changed after publication; register again before replacing it")
	}
	logging.Info("[output_dump] wrote explicit report %s (%d bytes, sha256=%s)", path, len(data), receipt.SHA256)
	return
}
