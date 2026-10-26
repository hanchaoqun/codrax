package tracecatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/immutablefile"
)

// Publication is detached navigation data. No Catalog, run validator or current
// source/read permission escapes the publication-only union through this value.
type Publication struct {
	Execution       ExecutionView
	IndexPath       string
	IndexSHA256     string
	LegacyPath      string
	SelectionPath   string
	SelectionSHA256 string
}

func snapshotContentSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Publication is no-clobber and same-content reuse preserves the file's physical
// generation. The mutable legacy alias is written separately, never hardlinked
// to this object. A later alias failure may leave an unreferenced immutable file;
// no successful receipt is returned, and no two-file transaction is claimed.
func publishImmutableSnapshot(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Reuse the shared anchored, no-replace publication and held/path generation
	// checks. No link/unlink ctime window or second material identity system.
	if _, err := immutablefile.Publish(filepath.Dir(path), filepath.Base(path), data); err != nil {
		return err
	}
	return ctx.Err()
}
