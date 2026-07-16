package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
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
	path            string
	identity        os.FileInfo
	authority       conversionOwnedFileAuthority
	authorityBound  bool
	removed         bool
	sealed          bool
	size            int64
	sealedIdentity  filegeneration.Identity
	traceValidation *publishedOwnedTraceValidation
}

// publishedOwnedTraceValidation is the ledger-side, public-generation binding
// of an opaque held-file validation receipt. The source validation may precede
// a platform snapshot/clone, so publishedIdentity deliberately binds the
// receipt to the exact generation owned by the transaction after publication.
type publishedOwnedTraceValidation struct {
	receipt           ownedTraceValidationReceipt
	publishedIdentity filegeneration.Identity
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
	sealedIdentity, err := captureOwnedSealedGeneration(abs, identity, size)
	if err != nil {
		return fmt.Errorf("capture newly published conversion generation %s: %w", path, err)
	}
	if confirmed, validateErr := authority.Validate(); validateErr != nil || confirmed == nil || !os.SameFile(identity, confirmed) || confirmed.Size() != size {
		return fmt.Errorf("newly published conversion file changed while its generation was captured: %s", path)
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
		sealedIdentity: sealedIdentity,
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
		generation, generationErr := captureOwnedSealedGeneration(record.path, record.identity, record.size)
		if generationErr != nil || !record.sealedIdentity.Initialized() || !record.sealedIdentity.SameVersion(generation) {
			return fmt.Errorf("created conversion file changed sealed generation before commit: %s", record.path)
		}
		if record.traceValidation != nil {
			if err := validateOwnedTraceValidationReceipt(record.traceValidation.receipt); err != nil ||
				!record.traceValidation.publishedIdentity.SameVersion(record.sealedIdentity) ||
				record.traceValidation.receipt.size != record.size {
				return fmt.Errorf("created conversion file lost its owned trace validation binding before commit: %s", record.path)
			}
		}
		if record.authority != nil {
			confirmed, confirmErr := record.authority.Validate()
			if confirmErr != nil || confirmed == nil || !os.SameFile(record.identity, confirmed) || confirmed.Size() != record.size {
				return fmt.Errorf("created conversion authority changed during commit validation: %s", record.path)
			}
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
	sealedIdentity, err := captureOwnedSealedGeneration(abs, record.identity, size)
	if err != nil {
		return fmt.Errorf("capture sealed conversion generation %s: %w", path, err)
	}
	record.sealed = true
	record.size = size
	record.sealedIdentity = sealedIdentity
	return nil
}

func (l *conversionFileLedger) recordOwnedTraceValidation(path string, receipt ownedTraceValidationReceipt) error {
	if l == nil {
		return fmt.Errorf("conversion file ledger is required to bind owned trace validation")
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return err
	}
	index, ok := l.byPath[abs]
	if !ok {
		return fmt.Errorf("cannot bind validation to unregistered conversion path: %s", path)
	}
	record := &l.created[index]
	if record.removed || !record.sealed || !record.sealedIdentity.Initialized() || record.identity == nil || receipt.size != record.size {
		return fmt.Errorf("cannot bind validation to a non-live sealed conversion generation: %s", path)
	}
	if record.traceValidation != nil {
		return fmt.Errorf("conversion generation already has an owned trace validation receipt: %s", path)
	}
	if err := validateOwnedTraceValidationReceipt(receipt); err != nil {
		return err
	}
	record.traceValidation = &publishedOwnedTraceValidation{
		receipt: receipt, publishedIdentity: record.sealedIdentity,
	}
	return nil
}

func (l *conversionFileLedger) ownedTraceValidation(path string) (publishedOwnedTraceValidation, bool) {
	if l == nil {
		return publishedOwnedTraceValidation{}, false
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return publishedOwnedTraceValidation{}, false
	}
	index, ok := l.byPath[abs]
	if !ok {
		return publishedOwnedTraceValidation{}, false
	}
	record := &l.created[index]
	if record.removed || !record.sealed || record.traceValidation == nil ||
		!record.traceValidation.publishedIdentity.SameVersion(record.sealedIdentity) ||
		record.traceValidation.receipt.size != record.size ||
		validateOwnedTraceValidationReceipt(record.traceValidation.receipt) != nil {
		return publishedOwnedTraceValidation{}, false
	}
	return *record.traceValidation, true
}

func captureOwnedSealedGeneration(path string, expected os.FileInfo, size int64) (identity filegeneration.Identity, err error) {
	if strings.TrimSpace(path) == "" || expected == nil || size < 0 {
		return filegeneration.Identity{}, fmt.Errorf("sealed conversion generation inputs are incomplete")
	}
	before, err := os.Lstat(path)
	if err != nil {
		return filegeneration.Identity{}, err
	}
	if !before.Mode().IsRegular() || before.Size() != size || !os.SameFile(expected, before) {
		return filegeneration.Identity{}, fmt.Errorf("sealed conversion path does not match its creator identity")
	}
	file, err := openConversionInputFile(path)
	if err != nil {
		return filegeneration.Identity{}, err
	}
	defer func() {
		err = traceDBJoinPreservingSingle(err, file.Close())
	}()
	opened, err := file.Stat()
	if err != nil {
		return filegeneration.Identity{}, err
	}
	identity, err = filegeneration.FromFile(file)
	if err != nil {
		return filegeneration.Identity{}, err
	}
	if !opened.Mode().IsRegular() || opened.Size() != size || !os.SameFile(expected, opened) ||
		!identity.Strong() || !identity.Mode().IsRegular() || identity.Size() != size {
		return filegeneration.Identity{}, fmt.Errorf("sealed conversion descriptor does not match its creator generation")
	}
	after, err := os.Lstat(path)
	if err != nil {
		return filegeneration.Identity{}, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) || after.Size() != size {
		return filegeneration.Identity{}, fmt.Errorf("sealed conversion path changed during generation capture")
	}
	finalIdentity, err := filegeneration.FromFile(file)
	if err != nil {
		return filegeneration.Identity{}, fmt.Errorf("recapture sealed conversion descriptor generation: %w", err)
	}
	if !identity.SameVersion(finalIdentity) {
		return filegeneration.Identity{}, fmt.Errorf("sealed conversion descriptor changed during generation capture")
	}
	return identity, nil
}

// heldSealedOwnedFile keeps the exact causal child descriptor open from its
// digest measurement through manifest publication. The ledger index remains
// stable even when publishing the manifest appends another created record.
type heldSealedOwnedFile struct {
	ledger         *conversionFileLedger
	recordIndex    int
	path           string
	file           *os.File
	opened         os.FileInfo
	sealedIdentity filegeneration.Identity
}

// holdAndMeasureSealedOwnedPath hashes only a live, sealed generation owned by
// this transaction and returns its still-open descriptor. The caller must keep
// the hold until the manifest has been sealed and revalidated, then close it.
func (l *conversionFileLedger) holdAndMeasureSealedOwnedPath(ctx context.Context, path string) (bytes int64, sha string, held *heldSealedOwnedFile, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, "", nil, err
	}
	if l == nil {
		return 0, "", nil, fmt.Errorf("conversion file ledger is required to measure %s", path)
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return 0, "", nil, err
	}
	index, ok := l.byPath[abs]
	if !ok {
		return 0, "", nil, fmt.Errorf("causal child is not owned by this conversion: %s", path)
	}
	record := &l.created[index]
	if record.removed || !record.sealed || record.identity == nil || !record.sealedIdentity.Initialized() {
		return 0, "", nil, fmt.Errorf("causal child is not a live sealed conversion generation: %s", path)
	}
	if record.authority != nil {
		current, validateErr := record.authority.Validate()
		if validateErr != nil || current == nil || !os.SameFile(record.identity, current) || current.Size() != record.size {
			return 0, "", nil, fmt.Errorf("validate causal child authority before measurement: %s", path)
		}
	} else if record.authorityBound {
		return 0, "", nil, fmt.Errorf("causal child lost its held publication authority: %s", path)
	}

	file, err := openConversionInputFile(abs)
	if err != nil {
		return 0, "", nil, err
	}
	keepOpen := false
	defer func() {
		if !keepOpen {
			err = traceDBJoinPreservingSingle(err, file.Close())
		}
	}()
	opened, err := file.Stat()
	if err != nil {
		return 0, "", nil, err
	}
	if !opened.Mode().IsRegular() || opened.Size() != record.size || !os.SameFile(record.identity, opened) {
		return 0, "", nil, fmt.Errorf("causal child descriptor differs from its ledger generation: %s", path)
	}
	if err := tracebundle.ValidateFile(ctx, file, record.sealedIdentity); err != nil {
		return 0, "", nil, fmt.Errorf("validate causal child before measurement %s: %w", path, err)
	}
	bytes, sha, measuredIdentity, err := tracebundle.MeasureFile(ctx, file)
	if err != nil {
		return 0, "", nil, fmt.Errorf("measure causal child %s: %w", path, err)
	}
	if bytes != record.size || !record.sealedIdentity.SameVersion(measuredIdentity) {
		return 0, "", nil, fmt.Errorf("causal child changed during measurement: %s", path)
	}
	held = &heldSealedOwnedFile{
		ledger: l, recordIndex: index, path: abs, file: file, opened: opened, sealedIdentity: record.sealedIdentity,
	}
	if err := held.Validate(ctx); err != nil {
		return 0, "", nil, fmt.Errorf("validate causal child after measurement %s: %w", path, err)
	}
	keepOpen = true
	return bytes, sha, held, nil
}

// Validate proves that both the held descriptor and its public path still
// denote the exact sealed generation captured by the conversion ledger.
func (held *heldSealedOwnedFile) Validate(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if held == nil || held.ledger == nil || held.file == nil || held.opened == nil ||
		held.recordIndex < 0 || held.recordIndex >= len(held.ledger.created) {
		return fmt.Errorf("held causal child is not initialized")
	}
	record := &held.ledger.created[held.recordIndex]
	if record.path != held.path || record.removed || !record.sealed || record.identity == nil ||
		!held.sealedIdentity.SameVersion(record.sealedIdentity) {
		return fmt.Errorf("held causal child no longer matches its ledger record: %s", held.path)
	}
	if record.authority != nil {
		current, validateErr := record.authority.Validate()
		if validateErr != nil || current == nil || !os.SameFile(record.identity, current) || current.Size() != record.size {
			return fmt.Errorf("held causal child authority changed: %s", held.path)
		}
	} else if record.authorityBound {
		return fmt.Errorf("held causal child lost its publication authority: %s", held.path)
	}
	if err := tracebundle.ValidateFile(ctx, held.file, held.sealedIdentity); err != nil {
		return fmt.Errorf("held causal child descriptor changed: %s: %w", held.path, err)
	}
	current, err := os.Lstat(held.path)
	if err != nil || !current.Mode().IsRegular() || current.Size() != record.size || !os.SameFile(held.opened, current) {
		return fmt.Errorf("held causal child path changed: %s", held.path)
	}
	if err := tracebundle.ValidateFile(ctx, held.file, held.sealedIdentity); err != nil {
		return fmt.Errorf("held causal child descriptor changed during path validation: %s: %w", held.path, err)
	}
	if record.authority != nil {
		current, validateErr := record.authority.Validate()
		if validateErr != nil || current == nil || !os.SameFile(record.identity, current) || current.Size() != record.size {
			return fmt.Errorf("held causal child authority changed during validation: %s", held.path)
		}
	}
	return nil
}

func (held *heldSealedOwnedFile) Close() error {
	if held == nil || held.file == nil {
		return nil
	}
	file := held.file
	held.file = nil
	return file.Close()
}

func closeHeldSealedOwnedFiles(heldFiles []*heldSealedOwnedFile) error {
	var result error
	for index := len(heldFiles) - 1; index >= 0; index-- {
		result = traceDBJoinPreservingSingle(result, heldFiles[index].Close())
	}
	return result
}

func (l *conversionFileLedger) sealedOwnedFileInfo(path string) (os.FileInfo, error) {
	if l == nil {
		return nil, fmt.Errorf("conversion file ledger is required to inspect %s", path)
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return nil, err
	}
	index, ok := l.byPath[abs]
	if !ok {
		return nil, fmt.Errorf("causal child is not owned by this conversion: %s", path)
	}
	record := &l.created[index]
	if record.removed || !record.sealed || record.identity == nil || !record.sealedIdentity.Initialized() {
		return nil, fmt.Errorf("causal child is not a live sealed conversion generation: %s", path)
	}
	return record.identity, nil
}

func (l *conversionFileLedger) validateSealedOwnedPath(ctx context.Context, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil {
		return fmt.Errorf("conversion file ledger is required to validate %s", path)
	}
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return err
	}
	index, ok := l.byPath[abs]
	if !ok {
		return fmt.Errorf("conversion path is not owned: %s", path)
	}
	record := &l.created[index]
	if record.removed || !record.sealed || !record.sealedIdentity.Initialized() {
		return fmt.Errorf("conversion path is not a live sealed generation: %s", path)
	}
	if record.authority != nil {
		current, validateErr := record.authority.Validate()
		if validateErr != nil || current == nil || !os.SameFile(record.identity, current) || current.Size() != record.size {
			return fmt.Errorf("validate held conversion authority: %s", path)
		}
	} else if record.authorityBound {
		return fmt.Errorf("conversion path lost its held authority: %s", path)
	}
	generation, err := captureOwnedSealedGeneration(abs, record.identity, record.size)
	if err != nil || !record.sealedIdentity.SameVersion(generation) {
		return fmt.Errorf("conversion path changed sealed generation: %s", path)
	}
	if record.authority != nil {
		current, validateErr := record.authority.Validate()
		if validateErr != nil || current == nil || !os.SameFile(record.identity, current) || current.Size() != record.size {
			return fmt.Errorf("held conversion authority changed during validation: %s", path)
		}
	}
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
	if err = ledger.releaseOwnedAuthorities(); err != nil {
		return fmt.Errorf("release conversion publication authorities: %w", err)
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
	if err = ledger.releaseOwnedAuthorities(); err != nil {
		return fmt.Errorf("release conversion publication authorities: %w", err)
	}
	committed = true
	return nil
}
