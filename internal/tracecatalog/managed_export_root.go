package tracecatalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ManagedExportRoot is a host-created, per-Run delivery destination. It is
// shared across live agent forks, never serialized or recovered from JSON.
// Delivery is independent of temporary blobs and their retention policy.
// No directory is created until the user-requested export is actually called.
type ManagedExportRoot struct {
	mu          sync.Mutex
	anchor, run string
	path        string
	identity    os.FileInfo
	initErr     error
}

// NewManagedExportRoot accepts only the host's stable runtime anchor, not a
// model/user tool path. Separate calls allocate isolated namespaces, even for
// the same package target. The host must provision the anchor itself.
func NewManagedExportRoot(anchor string) *ManagedExportRoot {
	r := &ManagedExportRoot{anchor: anchor}
	if anchor == "" || !filepath.IsAbs(anchor) {
		r.initErr = fmt.Errorf("absolute host runtime anchor is required for persistent delivery")
		return r
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		r.initErr = err
		return r
	}
	r.run = "run-" + hex.EncodeToString(id[:])
	return r
}

// Directory returns the current host destination. create=false is a read-only
// lookup: an unrequested export does not leave empty directories behind.
// Published packages are retained until explicitly removed by the user; this
// is not an archival backup or a new permission to read original captures.
func (r *ManagedExportRoot) Directory(ctx context.Context, create bool) (string, error) {
	if r == nil {
		return "", fmt.Errorf("persistent host delivery destination is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.initErr != nil {
		return "", r.initErr
	}
	if !filepath.IsAbs(r.anchor) || r.run == "" {
		return "", fmt.Errorf("persistent delivery scope was not initialized by the host")
	}
	if r.path != "" {
		info, err := os.Lstat(r.path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(r.identity, info) {
			return "", fmt.Errorf("managed delivery directory changed or is unavailable")
		}
		canonical, err := filepath.EvalSymlinks(r.path)
		if err != nil || canonical != r.path {
			return "", fmt.Errorf("managed delivery directory ancestry changed")
		}
		return r.path, nil
	}
	if !create {
		return "", nil
	}
	anchor, err := filepath.EvalSymlinks(r.anchor)
	if err != nil {
		return "", err
	}
	host, err := os.OpenRoot(anchor)
	if err != nil {
		return "", err
	}
	defer host.Close()
	if err := host.Mkdir("deliveries", 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := host.Lstat("deliveries")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("host deliveries location is not a plain directory")
	}
	deliveries, err := host.OpenRoot("deliveries")
	if err != nil {
		return "", err
	}
	defer deliveries.Close()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := deliveries.Mkdir(r.run, 0700); err != nil {
		return "", fmt.Errorf("cannot acquire fresh host delivery namespace: %w", err)
	}
	info, err = deliveries.Lstat(r.run)
	if err != nil {
		return "", err
	}
	path := filepath.Join(anchor, "deliveries", r.run)
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) || current.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("managed delivery namespace changed during creation")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return "", fmt.Errorf("managed delivery ancestry changed during creation")
	}
	r.path, r.identity = path, info
	return r.path, nil
}
