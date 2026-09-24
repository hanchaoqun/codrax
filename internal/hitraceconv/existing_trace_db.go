package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// ExistingTraceDBSource binds the bytes consumed by the closed-database route.
// It is input provenance, not an additional causal child or a clock authority.
// For gzip input, Path is the outer source's display locator, while the other
// fields identify the decoded SQLite payload and equal GzipInputProvenance's
// Decoded* tuple. Result.Input* and GzipInputProvenance.Source* identify the
// compressed source. This public record is not a process-local read authority.
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
	if err := validateExistingTraceDBHeader(ctx, source); err != nil {
		return err
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

func validateExistingTraceDBHeader(ctx context.Context, source conversionInputView) error {
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
		result, err = prepareExistingTraceDBFromView(ctx, opts, source, output, "", ledger,
			ExistingTraceDBSource{Path: source.DisplayPath(), Bytes: source.Size(), Generation: source.identity.CacheToken()},
			func() error { return validateExistingTraceDBBoundary(ctx, source) })
		return err
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
