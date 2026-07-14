package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// conversionFileLedger is the ownership authority for files created by one
// conversion attempt. Cleanup never guesses ownership from returned Artifact
// values: a path is removable only when it was registered at creation time and
// still resolves to the exact same file identity.
type conversionFileLedger struct {
	protected []traceCanonicalPath
	created   []createdConversionFile
	byPath    map[string]int
}

type createdConversionFile struct {
	path           string
	identity       os.FileInfo
	authority      conversionOwnedFileAuthority
	authorityBound bool
	removed        bool
	sealed         bool
	size           int64
}

// conversionOwnedFileAuthority keeps a newly-published file bound to the
// exact creator generation until the whole conversion commits or rolls back.
// Path-only records remain supported for ordinary outputs, but retained
// trace_streamer DB publications use this stronger lifecycle.
type conversionOwnedFileAuthority interface {
	Validate() (os.FileInfo, error)
	Remove() error
	Close() error
}

func newConversionFileLedger(protectedPaths ...string) (*conversionFileLedger, error) {
	ledger := &conversionFileLedger{byPath: make(map[string]int)}
	for _, raw := range protectedPaths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		path, err := canonicalTracePath(raw)
		if err != nil {
			return nil, fmt.Errorf("resolve protected conversion path %s: %w", raw, err)
		}
		ledger.protected = append(ledger.protected, path)
	}
	return ledger, nil
}

func newConversionFileLedgerForAuthority(authority *conversionInputAuthority) (*conversionFileLedger, error) {
	protected, err := authority.canonicalIdentity()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(protected.path) == "" || protected.info == nil || !protected.info.Mode().IsRegular() {
		return nil, conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageRoute, authority.DisplayPath(), errors.New("immutable input authority has no regular canonical identity"))
	}
	return &conversionFileLedger{
		protected: []traceCanonicalPath{protected},
		byPath:    make(map[string]int),
	}, nil
}

func (l *conversionFileLedger) recordOpenFile(path string, file *os.File) error {
	if l == nil {
		return fmt.Errorf("conversion file ledger is required to register %s", path)
	}
	if file == nil {
		return fmt.Errorf("record created conversion file %s: file is nil", path)
	}
	identity, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat newly created conversion file %s: %w", path, err)
	}
	return l.recordIdentity(path, identity)
}

func (l *conversionFileLedger) recordIdentity(path string, identity os.FileInfo) error {
	if l == nil {
		return fmt.Errorf("conversion file ledger is required to register %s", path)
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return fmt.Errorf("resolve newly created conversion file %s: %w", path, err)
	}
	if identity == nil || !identity.Mode().IsRegular() {
		return fmt.Errorf("newly created conversion path is not a regular file: %s", path)
	}
	current, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("inspect newly created conversion path %s: %w", path, err)
	}
	if !current.Mode().IsRegular() || !os.SameFile(identity, current) {
		return fmt.Errorf("newly created conversion path changed identity before registration: %s", path)
	}
	canonical, err := canonicalTracePath(abs)
	if err != nil {
		return fmt.Errorf("canonicalize newly created conversion file %s: %w", path, err)
	}
	for _, protected := range l.protected {
		if traceCanonicalPathsEqual(protected, canonical) {
			return fmt.Errorf("refusing to register protected input as a created conversion file: %s", path)
		}
	}
	if canonical.info == nil || !os.SameFile(identity, canonical.info) {
		return fmt.Errorf("canonical conversion path identity differs from the created file: %s", path)
	}
	if index, ok := l.byPath[abs]; ok {
		record := &l.created[index]
		if record.removed {
			// A provider may fail after creating and identity-safely removing an
			// output, then a fallback provider may publish a new generation at
			// the same path. Keep the removed generation for audit/cleanup order,
			// and make the new creator identity authoritative for this path.
			l.byPath[abs] = len(l.created)
			l.created = append(l.created, createdConversionFile{path: abs, identity: identity})
			return nil
		}
		if !os.SameFile(record.identity, identity) {
			return fmt.Errorf("created conversion path changed identity before registration: %s", path)
		}
		return nil
	}
	l.byPath[abs] = len(l.created)
	l.created = append(l.created, createdConversionFile{path: abs, identity: identity})
	return nil
}

func (l *conversionFileLedger) recordSealedAuthority(path string, size int64, authority conversionOwnedFileAuthority) error {
	if l == nil {
		return fmt.Errorf("conversion file ledger is required to register %s", path)
	}
	if authority == nil {
		return fmt.Errorf("conversion file authority is required to register %s", path)
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return fmt.Errorf("resolve newly published conversion file %s: %w", path, err)
	}
	identity, err := authority.Validate()
	if err != nil {
		return fmt.Errorf("validate newly published conversion file %s: %w", path, err)
	}
	if identity == nil || !identity.Mode().IsRegular() || identity.Size() != size {
		return fmt.Errorf("newly published conversion file is not a sealed regular file: %s", path)
	}
	for _, protected := range l.protected {
		if protected.info != nil && os.SameFile(identity, protected.info) {
			return fmt.Errorf("refusing to register protected input as a published conversion file: %s", path)
		}
	}
	if index, ok := l.byPath[abs]; ok {
		record := &l.created[index]
		if !record.removed {
			return fmt.Errorf("conversion path already has a live creator authority: %s", path)
		}
	}
	l.byPath[abs] = len(l.created)
	l.created = append(l.created, createdConversionFile{
		path: abs, identity: identity, authority: authority, authorityBound: true, sealed: true, size: size,
	})
	return nil
}

func (l *conversionFileLedger) ownsPathIdentity(path string, identity os.FileInfo) bool {
	if l == nil || identity == nil {
		return false
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return false
	}
	index, ok := l.byPath[abs]
	return ok && os.SameFile(l.created[index].identity, identity)
}

func (l *conversionFileLedger) removeOwnedPath(path string) error {
	if l == nil {
		return nil
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return err
	}
	index, ok := l.byPath[abs]
	if !ok {
		return fmt.Errorf("refusing to remove unregistered conversion path: %s", path)
	}
	record := &l.created[index]
	if record.removed {
		return nil
	}
	if record.authority != nil {
		removeErr := record.authority.Remove()
		closeErr := record.authority.Close()
		record.authority = nil
		if removeErr == nil {
			record.removed = true
		}
		return traceDBJoinPreservingSingle(removeErr, closeErr)
	}
	if record.authorityBound {
		return fmt.Errorf("refusing path-only removal of authority-bound conversion generation: %s", path)
	}
	err = removeOwnedConversionPath(abs, record.identity)
	if err == nil {
		record.removed = true
	}
	return err
}

func (l *conversionFileLedger) validateOwnedPaths() error {
	if l == nil {
		return nil
	}
	for _, record := range l.created {
		if record.removed {
			continue
		}
		if !record.sealed {
			return fmt.Errorf("created conversion file was not sealed before commit: %s", record.path)
		}
		var current os.FileInfo
		var err error
		if record.authority != nil {
			current, err = record.authority.Validate()
		} else if record.authorityBound {
			return fmt.Errorf("created conversion file lost its held authority before commit: %s", record.path)
		} else {
			current, err = os.Lstat(record.path)
		}
		if err != nil {
			return fmt.Errorf("validate created conversion file %s before commit: %w", record.path, err)
		}
		if current == nil || !current.Mode().IsRegular() || !os.SameFile(record.identity, current) || current.Size() != record.size {
			return fmt.Errorf("created conversion file changed identity before commit: %s", record.path)
		}
	}
	return nil
}

func (l *conversionFileLedger) releaseOwnedAuthorities() error {
	if l == nil {
		return nil
	}
	var result error
	for index := len(l.created) - 1; index >= 0; index-- {
		record := &l.created[index]
		if record.removed || record.authority == nil {
			continue
		}
		result = traceDBJoinPreservingSingle(result, record.authority.Close())
		record.authority = nil
	}
	return result
}

func (l *conversionFileLedger) sealOwnedPath(path string, size int64) error {
	if l == nil {
		return fmt.Errorf("conversion file ledger is required to seal %s", path)
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return err
	}
	index, ok := l.byPath[abs]
	if !ok {
		return fmt.Errorf("cannot seal unregistered conversion path: %s", path)
	}
	current, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	record := &l.created[index]
	if !current.Mode().IsRegular() || !os.SameFile(record.identity, current) || current.Size() != size {
		return fmt.Errorf("conversion file failed seal identity/size validation: %s", path)
	}
	record.sealed = true
	record.size = size
	return nil
}

func (l *conversionFileLedger) cleanup() error {
	if l == nil {
		return nil
	}
	var cleanupErr error
	for index := len(l.created) - 1; index >= 0; index-- {
		record := &l.created[index]
		if record.removed || record.path == "" || record.identity == nil {
			continue
		}
		if record.authority != nil {
			removeErr := record.authority.Remove()
			closeErr := record.authority.Close()
			record.authority = nil
			cleanupErr = errors.Join(cleanupErr, removeErr, closeErr)
			if removeErr == nil {
				record.removed = true
				continue
			}
			continue
		}
		if record.authorityBound {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("authority-bound conversion generation cannot fall back to path cleanup: %s", record.path))
			continue
		}
		cleanupErr = errors.Join(cleanupErr, removeOwnedConversionPath(record.path, record.identity))
	}
	return cleanupErr
}

func rollbackOpenConversionFile(path string, file *os.File) error {
	if file == nil {
		return nil
	}
	identity, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil {
		return traceDBJoinPreservingSingle(statErr, closeErr)
	}
	return traceDBJoinPreservingSingle(closeErr, removeOwnedConversionPath(path, identity))
}

func removeOwnedConversionPath(path string, identity os.FileInfo) error {
	if strings.TrimSpace(path) == "" || identity == nil {
		return nil
	}
	current, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect created conversion file %s for cleanup: %w", path, err)
	}
	if !os.SameFile(identity, current) {
		return fmt.Errorf("refusing to remove created conversion path whose file identity changed: %s", path)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove created conversion file %s: %w", path, err)
	}
	return nil
}

func removeOwnedConversionDir(path string, identity os.FileInfo) error {
	if strings.TrimSpace(path) == "" || identity == nil {
		return nil
	}
	current, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect conversion staging directory %s for cleanup: %w", path, err)
	}
	if !current.IsDir() || !os.SameFile(identity, current) {
		return fmt.Errorf("refusing to remove conversion staging directory whose identity changed: %s", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove conversion staging directory %s: %w", path, err)
	}
	return nil
}

func joinConversionCleanupError(primary error, ledger *conversionFileLedger) error {
	cleanupErr := ledger.cleanup()
	if primary == nil {
		return cleanupErr
	}
	if cleanupErr == nil {
		return primary
	}
	return errors.Join(primary, cleanupErr)
}

func openOwnedConversionFile(path string, ledger *conversionFileLedger) (*os.File, error) {
	if ledger == nil {
		return nil, fmt.Errorf("conversion file ledger is required to create %s", path)
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if err := ledger.recordOpenFile(path, out); err != nil {
		return nil, traceDBJoinPreservingSingle(err, rollbackOpenConversionFile(path, out))
	}
	return out, nil
}

func finishOwnedConversionFile(path string, out *os.File, ledger *conversionFileLedger, nonEmpty bool, writeErrors ...error) (os.FileInfo, error) {
	if ledger == nil {
		if out == nil {
			return nil, fmt.Errorf("conversion file ledger is required to finish %s", path)
		}
		return nil, traceDBJoinPreservingSingle(fmt.Errorf("conversion file ledger is required to finish %s", path), rollbackOpenConversionFile(path, out))
	}
	if out == nil {
		return nil, fmt.Errorf("conversion output handle is nil for %s", path)
	}
	closeErr := out.Close()
	primary := traceDBJoinPreservingSingle(nil, append(writeErrors, closeErr)...)
	if primary != nil {
		return nil, traceDBJoinPreservingSingle(primary, ledger.removeOwnedPath(path))
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(path))
	}
	if !info.Mode().IsRegular() || !ledger.ownsPathIdentity(path, info) || (nonEmpty && info.Size() <= 0) {
		err := fmt.Errorf("conversion file publication failed identity/regular-file validation: %s", path)
		return nil, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(path))
	}
	if err := ledger.sealOwnedPath(path, info.Size()); err != nil {
		return nil, traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(path))
	}
	return info, nil
}

func runConversionFileTransaction(ctx context.Context, protected string, work func(*conversionFileLedger) error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ledger, err := newConversionFileLedger(protected)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			err = joinConversionCleanupError(err, ledger)
		}
	}()
	err = work(ledger)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = ledger.validateOwnedPaths(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	committed = true
	return nil
}

// runConversionInputTransaction gives standalone file-conversion APIs the
// same immutable source-generation and output-ownership transaction used by
// ConvertFile. The work callback must consume authority directly; reopening
// inputPath inside work would reintroduce an A->B->A source-generation gap.
func runConversionInputTransaction(ctx context.Context, inputPath string, work func(*conversionInputAuthority, *conversionFileLedger) error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	authority, err := openConversionInputAuthority(inputPath)
	if err != nil {
		return err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, authority.Close())
	}()
	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			err = joinConversionCleanupError(err, ledger)
		}
	}()
	if work == nil {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageRoute,
			authority.DisplayPath(),
			errors.New("nil conversion input transaction callback"),
		)
	}
	if err = work(authority, ledger); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = authority.Validate(conversionInputStagePreCommit); err != nil {
		return err
	}
	if err = authority.Close(); err != nil {
		return err
	}
	if err = ledger.validateOwnedPaths(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	committed = true
	return nil
}
