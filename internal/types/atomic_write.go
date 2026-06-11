package types

import (
	"fmt"
	"os"
)

// AtomicWriteFileSync writes data through the tmp → fsync → rename
// sequence so no reader ever observes a partial file AND a power loss
// after return cannot leave a truncated artifact: plain tmp+rename
// guarantees atomicity against concurrent readers but the renamed
// file's CONTENT may still live only in the page cache. Every durable
// write-mode artifact (plans, approval records, workflow runs,
// reports, taxonomies) goes through this single seam.
func AtomicWriteFileSync(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("atomic write %s: open: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic write %s: write: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic write %s: sync: %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic write %s: close: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic write %s: rename: %w", path, err)
	}
	return nil
}
