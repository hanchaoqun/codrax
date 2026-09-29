package traceinput

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

// Bind the database actually consumed to its transport. A gzip locator names
// the original compressed file, but the database receipt measures decoded bytes.
// These serialized diagnostics never replace the held input/material authority.
func validatePreparedSQLiteReceipt(kind, source string, size int64, digest, generation string, result hitraceconv.Result) error {
	db := result.ExistingTraceDBSource
	if kind == string(attachment.BinaryTraceFormatSQLite) {
		if db == nil || db.Path != source || db.Bytes != size || db.SHA256 != digest || db.Generation != generation {
			return fmt.Errorf("existing trace database receipt does not match the held source: %q", source)
		}
		if wal := db.WAL; wal != nil && (wal.Path == "" || (wal.Bytes != 0 && wal.Bytes < 32) || len(wal.SHA256) != 64 || wal.Generation == "" ||
			(wal.CheckpointOnly && wal.CommitFrame != 0) || (!wal.CheckpointOnly && (wal.CommitFrame <= 0 || wal.Bytes < 32)) ||
			wal.SnapshotBytes < 512 || len(wal.SnapshotSHA256) != 64) {
			return fmt.Errorf("incomplete SQLite WAL snapshot receipt: %q", source)
		}
		return nil
	}
	gzip := result.GzipInputProvenance
	if kind == string(attachment.BinaryTraceFormatGZIP) && gzip != nil && gzip.DecodedFormat == string(attachment.BinaryTraceFormatSQLite) {
		if db == nil || db.WAL != nil || db.Path != source || db.Bytes != gzip.DecodedBytes || db.SHA256 != gzip.DecodedSHA256 || db.Generation != gzip.DecodedGeneration {
			return fmt.Errorf("decoded SQLite receipt does not match the gzip transport: %q", source)
		}
		return nil
	}
	if db != nil {
		return fmt.Errorf("unexpected SQLite consumption receipt for input kind %q: %q", kind, source)
	}
	return nil
}
