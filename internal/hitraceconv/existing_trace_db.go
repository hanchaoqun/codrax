package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// ExistingTraceDBSource binds the bytes consumed by the closed-database route.
// It is input provenance, not an additional causal child or a clock authority.
type ExistingTraceDBSource struct {
	Path       string
	Bytes      int64
	SHA256     string
	Generation string
}

// ValidateExistingTraceDBSource checks the single-file SQLite boundary without
// opening SQLite or creating any source-side files. Callers retaining material
// must separately compare the original source generation: this check alone is
// not an authorization to replace a previously accepted source.
func ValidateExistingTraceDBSource(ctx context.Context, path string) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := openConversionInputAuthority(path)
	if err != nil {
		return err
	}
	defer func() { err = traceDBJoinPreservingSingle(err, source.Close()) }()
	return validateExistingTraceDBBoundary(ctx, source)
}

func validateExistingTraceDBBoundary(ctx context.Context, source *conversionInputAuthority) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := source.Validate(conversionInputStagePreCommit); err != nil {
		return err
	}
	var header [100]byte
	if _, err := source.ReadAt(header[:], 0); err != nil {
		return fmt.Errorf("existing trace DB requires a complete SQLite header: %w", err)
	}
	if string(header[:16]) != "SQLite format 3\x00" {
		return fmt.Errorf("existing trace DB is not SQLite format 3")
	}
	// Even a checkpointed WAL database is outside this single-file contract.
	// Do not ask SQLite to recover it, checkpoint it, or silently ignore a WAL.
	if header[18] != 1 || header[19] != 1 {
		return fmt.Errorf("existing trace DB requires rollback-journal header mode; WAL or unknown read/write modes are not supported")
	}
	for _, path := range uniqueNonEmptyStrings([]string{source.requestedPath, source.CanonicalPath()}) {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, err := os.Lstat(path + suffix); err == nil {
				return fmt.Errorf("%w: %s", errTraceStreamerDBAuxiliaryState, path+suffix)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect existing trace DB auxiliary %s: %w", path+suffix, err)
			}
		}
	}
	return completeConversionInputStage(ctx, source, conversionInputStagePreCommit, nil)
}

// PrepareExistingTraceDB prepares a self-contained, closed TraceStreamer SQLite
// database. It never invokes a converter or opens the user database in SQLite.
// The exact private snapshot uses the existing sealed VFS, semantic exporters,
// full-table fidelity carrier and owned publication receipts. Inventory-only
// databases are not accepted as queryable traces. KeepTraceDB needs no extra
// copy: the original remains untouched; TraceDBOutputPath is not this route's
// output. Provider/archive options do not select a different engine here.
func PrepareExistingTraceDB(ctx context.Context, opts Options) (result Result, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(opts.TraceDBOutputPath) != "" || strings.TrimSpace(opts.ArchiveMember) != "" {
		return Result{}, fmt.Errorf("existing trace DB preparation does not produce a second DB or select an archive member")
	}
	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = DefaultOutputPath(strings.TrimSpace(opts.InputPath))
	}
	err = runConversionInputTransaction(ctx, opts.InputPath, func(source *conversionInputAuthority, ledger *conversionFileLedger) (workErr error) {
		if err := validateExistingTraceDBBoundary(ctx, source); err != nil {
			return err
		}
		if err := preflightExistingTraceDBOutputs(source, output); err != nil {
			return err
		}
		anchor, err := resolveConversionRuntimeAnchor(opts.RuntimeAnchor, output)
		if err != nil {
			return err
		}
		anchor, err = selectSecureConversionRuntimeAnchor(anchor, opts.RuntimeAnchorFallback)
		if err != nil {
			return err
		}
		ledger.stagingRoot = anchor
		staging, err := newRuntimePrivateConversionDir(anchor, "existing-db-*")
		if err != nil {
			return err
		}
		defer func() { workErr = traceDBJoinPreservingSingle(workErr, staging.FinalizeCleanup()) }()
		start := progressStarted(opts, "existing_trace_db_snapshot", "preparing closed SQLite trace snapshot", source.DisplayPath(), output)
		lease, err := newExternalToolInputLeaseWithProgress(ctx, source, staging, sealedTraceDBVirtualName, externalToolInputSnapshotOnly, nil)
		if err != nil {
			return err
		}
		sealed, err := sealExternalToolInputSnapshot(ctx, lease, staging)
		if err != nil {
			return err
		}
		defer func() { workErr = traceDBJoinPreservingSingle(workErr, sealed.Close()) }()
		progressFinished(opts, "existing_trace_db_snapshot", "closed SQLite trace snapshot prepared", source.DisplayPath(), output, start, ProgressStatusComplete)
		if err := validateExistingTraceDBBoundary(ctx, source); err != nil {
			return err
		}
		hasher := sha256.New()
		if _, err := copyCancellableRange(ctx, hasher, io.NewSectionReader(sealed, 0, sealed.Size()), nil); err != nil {
			return err
		}
		start = progressStarted(opts, "existing_trace_db_export", "reading closed SQLite trace snapshot", source.DisplayPath(), output)
		exported, err := exportTraceDBToSystraceFromSealedWithLedger(ctx, sealed, source.DisplayPath(), output, ledger)
		if err != nil {
			return err
		}
		if exported.Artifact.Trace == nil || !exported.Artifact.Trace.TraceQueryReady {
			return fmt.Errorf("existing SQLite DB contains no query-ready trace rows under the supported schema, owner and clock contracts")
		}
		decision := newTraceProviderDecision(traceProviderStageTraceBody, traceProviderByName(traceProviderNameTraceStreamer), Options{TraceEngine: traceEngineTraceStreamer}, source.DisplayPath(), output)
		decision, err = traceProviderPublished(decision, exported.Artifact, ledger)
		if err != nil {
			return err
		}
		decision.Caveat = "existing closed SQLite database normalized read-only; trace_streamer executable was not invoked"
		result = Result{
			InputPath: source.DisplayPath(), InputBytes: source.Size(), OutputPath: exported.Artifact.Path,
			OutputBytes: exported.OutputBytes, EventsWritten: exported.EventsWritten,
			FirstTimestampSec: exported.FirstTimestampSec, LastTimestampSec: exported.LastTimestampSec,
			Artifacts: []Artifact{exported.Artifact}, TraceDecisions: []TraceProviderDecision{decision},
			TraceDBCoverage: exported.Coverage, TraceCoverage: exported.TraceCoverage,
			Caveats:               append([]string{decision.Caveat}, traceDBSemanticQualityCaveats(exported.Coverage)...),
			ExistingTraceDBSource: &ExistingTraceDBSource{Path: source.DisplayPath(), Bytes: source.Size(), SHA256: hex.EncodeToString(hasher.Sum(nil)), Generation: source.identity.CacheToken()},
		}
		if err := finalizeResultTraceBundleWithLedger(ctx, source.DisplayPath(), output, &result, ledger); err != nil {
			return err
		}
		progressFinished(opts, "existing_trace_db_export", "closed SQLite trace exported", source.DisplayPath(), output, start, ProgressStatusComplete)
		if err := sealed.Validate(); err != nil {
			return err
		}
		return validateExistingTraceDBBoundary(ctx, source)
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func preflightExistingTraceDBOutputs(source *conversionInputAuthority, output string) error {
	// Never create SQLite sidecars even when a caller explicitly picks one as
	// the output filename. Resolve parent symlinks for prospective paths too.
	for _, path := range []string{output, traceSidecarBase(source.DisplayPath(), output) + ".tracebundle.json"} {
		candidate, err := canonicalTracePath(path)
		if err != nil {
			return err
		}
		for _, base := range uniqueNonEmptyStrings([]string{source.requestedPath, source.CanonicalPath()}) {
			for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
				protected, err := canonicalTracePath(base + suffix)
				if err != nil {
					return err
				}
				if candidate.path == protected.path {
					return fmt.Errorf("existing trace DB output overlaps the source or SQLite auxiliary path: %s", path)
				}
			}
		}
	}
	return preflightTracePublicationPaths(Options{}, source.DisplayPath(), output, true)
}
