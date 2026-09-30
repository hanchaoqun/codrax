package hitraceconv

import (
	"fmt"
	"os"
)

// Absence is checked on both requested and canonical namespaces, throughout
// snapshot reads, publication and reuse. Lstat also rejects dangling links.
// This proves only the supplied checkpoint's boundary, never that a caller
// preserved all historical transactions when collecting the capture.
func validateAbsentExistingWAL(main *conversionInputAuthority) error {
	for _, base := range uniqueNonEmptyStrings([]string{main.requestedPath, main.CanonicalPath()}) {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, err := os.Lstat(base + suffix); !os.IsNotExist(err) {
				return fmt.Errorf("%w: absent-WAL checkpoint requires no auxiliary at %s: %v", errTraceStreamerDBAuxiliaryState, base+suffix, err)
			}
		}
	}
	return nil
}

func openExistingWALOrAbsent(main *conversionInputAuthority) (*conversionInputAuthority, error) {
	path := main.CanonicalPath() + "-wal"
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil, validateAbsentExistingWAL(main)
	} else if err != nil {
		return nil, err
	}
	return openConversionInputAuthority(path)
}

func (v *existingWALView) Close() error {
	if v.wal == nil {
		return nil
	}
	return v.wal.Close()
}
