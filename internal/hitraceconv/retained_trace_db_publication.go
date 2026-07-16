package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// retainedTraceDBPublication is the ledger authority for one retained DB
// generation. The final file and its parent directory stay open until the
// outer conversion transaction commits or rolls back.
type retainedTraceDBPublication struct {
	mu            sync.Mutex
	file          *os.File
	identity      filegeneration.Identity
	identityInfo  os.FileInfo
	size          int64
	kind          sealedConversionPublicationKind
	leaf          string
	bindingPath   string
	authorityPath string
	platform      publishedConversionFilePlatformState
	removed       bool
	closed        bool
	closeErr      error
}

// sealedConversionPublicationKind is a closed diagnostic identity for the
// exact-generation publisher. The retained trace DB lane keeps its historical
// wording, while other callers can reuse the same no-replace authority without
// teaching users that an unrelated sidecar is a trace DB.
type sealedConversionPublicationKind uint8

const (
	sealedConversionPublicationRetainedTraceDB sealedConversionPublicationKind = iota + 1
	sealedConversionPublicationOutput
)

func (kind sealedConversionPublicationKind) valid() bool {
	return kind == sealedConversionPublicationRetainedTraceDB || kind == sealedConversionPublicationOutput
}

func (kind sealedConversionPublicationKind) diagnosticName() string {
	if kind == sealedConversionPublicationRetainedTraceDB {
		return "retained trace DB"
	}
	return "sealed conversion output"
}

func (kind sealedConversionPublicationKind) sealedSourceName() string {
	if kind == sealedConversionPublicationRetainedTraceDB {
		return "sealed trace DB"
	}
	return "sealed conversion output"
}

func (kind sealedConversionPublicationKind) privateHandleName() string {
	if kind == sealedConversionPublicationRetainedTraceDB {
		return "codrax-retained-trace-db"
	}
	return "codrax-sealed-conversion-output"
}

func (kind sealedConversionPublicationKind) privateClonePattern() string {
	if kind == sealedConversionPublicationRetainedTraceDB {
		return ".codrax-retained-db-*"
	}
	return ".codrax-sealed-output-*"
}

// sealedConversionPublicationTarget owns a private staging directory rooted
// in the final output's held parent authority. StagingPath deliberately keeps
// the final basename/extension so name-sensitive adapters can consume the
// private generation before the public binding exists.
type sealedConversionPublicationTarget struct {
	StagingPath      string
	FinalPath        string
	Cleanup          func() error
	stagingDir       *privateConversionDir
	finalLeaf        string
	finalBindingPath string
}

func prepareSealedConversionPublicationTarget(finalPath, pattern string) (sealedConversionPublicationTarget, error) {
	finalPath = strings.TrimSpace(finalPath)
	if finalPath == "" {
		return sealedConversionPublicationTarget{}, fmt.Errorf("sealed conversion output path is required")
	}
	absoluteFinal, err := filepath.Abs(filepath.Clean(finalPath))
	if err != nil {
		return sealedConversionPublicationTarget{}, fmt.Errorf("resolve sealed conversion output path %s: %w", finalPath, err)
	}
	leaf := filepath.Base(absoluteFinal)
	if leaf == "" || leaf == "." || leaf == ".." || filepath.Base(leaf) != leaf {
		return sealedConversionPublicationTarget{}, fmt.Errorf("sealed conversion output file name is invalid: %s", finalPath)
	}
	if err := validatePrivateConversionDirChildNamePlatform(leaf); err != nil {
		return sealedConversionPublicationTarget{}, fmt.Errorf("sealed conversion output file name is invalid: %s: %w", finalPath, err)
	}
	if _, err := os.Lstat(finalPath); err == nil {
		return sealedConversionPublicationTarget{}, fmt.Errorf("output file already exists: %s (delete it first or specify a different output path)", finalPath)
	} else if !os.IsNotExist(err) {
		return sealedConversionPublicationTarget{}, fmt.Errorf("check sealed conversion output path %s: %w", finalPath, err)
	}
	parent := filepath.Dir(absoluteFinal)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return sealedConversionPublicationTarget{}, fmt.Errorf("inspect sealed conversion output directory %s: %w", parent, err)
	}
	if !parentInfo.IsDir() {
		return sealedConversionPublicationTarget{}, fmt.Errorf("sealed conversion output parent is not a directory: %s", parent)
	}
	if strings.TrimSpace(pattern) == "" {
		return sealedConversionPublicationTarget{}, fmt.Errorf("sealed conversion staging pattern is required")
	}
	aliasesFinal, err := sealedConversionStagingPatternAliasesLeaf(pattern, leaf)
	if err != nil {
		return sealedConversionPublicationTarget{}, fmt.Errorf("sealed conversion staging pattern is invalid: %w", err)
	}
	if aliasesFinal {
		return sealedConversionPublicationTarget{}, fmt.Errorf("sealed conversion staging namespace can alias the public output leaf: %q", leaf)
	}
	stagingDir, err := newPrivateConversionDir(parent, pattern)
	if err != nil {
		return sealedConversionPublicationTarget{}, err
	}
	stagingPath, err := stagingDir.ChildPath(leaf)
	if err != nil {
		return sealedConversionPublicationTarget{}, traceDBJoinPreservingSingle(err, stagingDir.FinalizeCleanup())
	}
	return sealedConversionPublicationTarget{
		StagingPath: stagingPath, FinalPath: finalPath, Cleanup: stagingDir.FinalizeCleanup,
		stagingDir: stagingDir, finalLeaf: leaf, finalBindingPath: absoluteFinal,
	}, nil
}

// sealedConversionStagingPatternAliasesLeaf proves namespace separation before
// the private directory is created. nextPrivateConversionDirLeaf always places
// exactly 16 random bytes as 32 lowercase hex digits between the pattern's
// prefix and suffix. Patterns are internal ASCII namespaces; conservative
// Unicode case-fold comparison also rejects aliases on case-insensitive Darwin
// and Windows volumes rather than treating a 2^-128 collision as correctness.
func sealedConversionStagingPatternAliasesLeaf(pattern, leaf string) (bool, error) {
	prefix, suffix, err := splitPrivateConversionDirPattern(pattern)
	if err != nil {
		return false, err
	}
	for index := 0; index < len(pattern); index++ {
		if pattern[index] > 0x7f {
			return false, fmt.Errorf("pattern must use an ASCII-only private namespace")
		}
	}
	if len(leaf) < 32 {
		return false, nil
	}
	for split := 0; split <= len(leaf)-32; split++ {
		middle := leaf[split : split+32]
		hex := true
		for index := 0; index < len(middle); index++ {
			if (middle[index] < '0' || middle[index] > '9') &&
				(middle[index] < 'a' || middle[index] > 'f') &&
				(middle[index] < 'A' || middle[index] > 'F') {
				hex = false
				break
			}
		}
		if hex && strings.EqualFold(leaf[:split], prefix) && strings.EqualFold(leaf[split+32:], suffix) {
			return true, nil
		}
	}
	return false, nil
}

func (target sealedConversionPublicationTarget) finalBindingPaths() (bindingPath, authorityPath string, err error) {
	if target.stagingDir == nil || strings.TrimSpace(target.finalBindingPath) == "" || strings.TrimSpace(target.finalLeaf) == "" {
		return "", "", fmt.Errorf("sealed conversion publication target authority is incomplete")
	}
	if err := validatePrivateConversionDirChildNamePlatform(target.finalLeaf); err != nil {
		return "", "", fmt.Errorf("sealed conversion output final leaf is invalid: %q: %w", target.finalLeaf, err)
	}
	bindingPath = target.finalBindingPath
	authorityPath = filepath.Join(filepath.Dir(target.stagingDir.Path()), target.finalLeaf)
	return bindingPath, authorityPath, nil
}

func newRetainedTraceDBPublication(
	file *os.File,
	platform publishedConversionFilePlatformState,
	kind sealedConversionPublicationKind,
	leaf, bindingPath, authorityPath string,
	allowedSize int64,
) (*retainedTraceDBPublication, error) {
	if file == nil || !kind.valid() || strings.TrimSpace(leaf) == "" || allowedSize < 0 {
		return nil, fmt.Errorf("%s publication authority is incomplete", kind.diagnosticName())
	}
	identity, err := filegeneration.FromFile(file)
	if err != nil {
		return nil, fmt.Errorf("capture %s publication identity: %w", kind.diagnosticName(), err)
	}
	if !identity.Strong() || !identity.Mode().IsRegular() || identity.Size() != allowedSize {
		return nil, fmt.Errorf("%s publication has invalid generation: strong=%t mode=%s size=%d want=%d", kind.diagnosticName(),
			identity.Strong(), identity.Mode(), identity.Size(), allowedSize)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s publication: %w", kind.diagnosticName(), err)
	}
	publication := &retainedTraceDBPublication{
		file: file, identity: identity, identityInfo: info, size: allowedSize, kind: kind,
		leaf: leaf, bindingPath: bindingPath, authorityPath: authorityPath, platform: platform,
	}
	if _, err := publication.Validate(); err != nil {
		return nil, err
	}
	return publication, nil
}

func (publication *retainedTraceDBPublication) Validate() (os.FileInfo, error) {
	if publication == nil {
		return nil, fmt.Errorf("retained trace DB publication authority is nil")
	}
	publication.mu.Lock()
	defer publication.mu.Unlock()
	return publication.validateLocked()
}

func (publication *retainedTraceDBPublication) validateLocked() (os.FileInfo, error) {
	name := publication.kind.diagnosticName()
	if publication.closed || publication.removed || publication.file == nil || !publication.identity.Initialized() {
		return nil, fmt.Errorf("%s publication authority is closed or removed: %s", name, publication.bindingPath)
	}
	current, err := filegeneration.FromFile(publication.file)
	if err != nil || !current.Strong() || !publication.identity.SameVersion(current) {
		return nil, fmt.Errorf("%s publication generation changed: %s", name, publication.bindingPath)
	}
	info, err := publication.file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != publication.size ||
		!os.SameFile(publication.identityInfo, info) {
		return nil, fmt.Errorf("%s publication identity changed: %s", name, publication.bindingPath)
	}
	if err := validatePublishedConversionFilePlatform(&publication.platform, publication.leaf, publication.file, info, publication.kind); err != nil {
		return nil, fmt.Errorf("validate %s parent-relative binding %s: %w", name, publication.bindingPath, err)
	}
	for _, path := range dedupeStrings([]string{publication.bindingPath, publication.authorityPath}) {
		if _, err := inspectOwnedSealedGenerationPath(
			path, info, publication.identity, publication.size,
		); err != nil {
			return nil, fmt.Errorf("validate %s public binding %s: %w", name, path, err)
		}
	}
	return info, nil
}

func (publication *retainedTraceDBPublication) Remove() error {
	if publication == nil {
		return nil
	}
	publication.mu.Lock()
	defer publication.mu.Unlock()
	if publication.removed {
		return nil
	}
	if publication.closed || publication.file == nil {
		return fmt.Errorf("%s publication authority closed before rollback: %s", publication.kind.diagnosticName(), publication.bindingPath)
	}
	if _, err := publication.validateLocked(); err != nil {
		return err
	}
	if err := removePublishedConversionFilePlatform(&publication.platform, publication.leaf, publication.file, publication.kind); err != nil {
		return fmt.Errorf("remove %s publication %s: %w", publication.kind.diagnosticName(), publication.bindingPath, err)
	}
	publication.removed = true
	return nil
}

func (publication *retainedTraceDBPublication) Close() error {
	if publication == nil {
		return nil
	}
	publication.mu.Lock()
	defer publication.mu.Unlock()
	if publication.closed {
		return publication.closeErr
	}
	publication.closed = true
	var fileErr error
	if publication.file != nil {
		fileErr = publication.file.Close()
		publication.file = nil
	}
	platformErr := closePublishedConversionFilePlatform(&publication.platform)
	publication.closeErr = traceDBJoinPreservingSingle(fileErr, platformErr)
	return publication.closeErr
}

func abortRetainedTraceDBPublication(
	file *os.File,
	platform *publishedConversionFilePlatformState,
	leaf string,
	kind sealedConversionPublicationKind,
	primary error,
) error {
	if file == nil {
		return traceDBJoinPreservingSingle(primary, closePublishedConversionFilePlatform(platform))
	}
	removeErr := removePublishedConversionFilePlatform(platform, leaf, file, kind)
	closeErr := file.Close()
	platformCloseErr := closePublishedConversionFilePlatform(platform)
	return traceDBJoinPreservingSingle(primary, removeErr, closeErr, platformCloseErr)
}

func publishRetainedTraceDBOutputs(
	ctx context.Context,
	target traceStreamerDBTarget,
	outputs *sealedTraceStreamerDBOutputs,
	ledger *conversionFileLedger,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !target.Retained || strings.TrimSpace(target.FinalPath) == "" {
		return nil
	}
	if outputs == nil || outputs.main == nil || ledger == nil {
		return fmt.Errorf("retained trace DB publication inputs are incomplete")
	}
	if err := outputs.validate(); err != nil {
		return fmt.Errorf("validate sealed trace_streamer output set before publication: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	companionBindingPath, _, bindingErr := target.finalBindingPaths(target.finalCompanionLeaf())
	if bindingErr != nil && outputs.companionPresent {
		return bindingErr
	}
	companionPublished := false
	if outputs.companionPresent {
		if err := publishOneRetainedTraceDBFile(ctx, target, outputs.companion, target.finalCompanionLeaf(), ledger); err != nil {
			return fmt.Errorf("publish trace DB timestamp companion: %w", err)
		}
		companionPublished = true
	}
	if err := ctx.Err(); err != nil {
		if companionPublished {
			return traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(companionBindingPath))
		}
		return err
	}
	if err := publishOneRetainedTraceDBFile(ctx, target, outputs.main, target.finalLeaf, ledger); err != nil {
		if companionPublished {
			return traceDBJoinPreservingSingle(err, ledger.removeOwnedPath(companionBindingPath))
		}
		return err
	}
	return nil
}

func publishOneRetainedTraceDBFile(
	ctx context.Context,
	target traceStreamerDBTarget,
	source *sealedConversionFile,
	leaf string,
	ledger *conversionFileLedger,
) error {
	if source == nil || strings.TrimSpace(leaf) == "" {
		return fmt.Errorf("sealed retained trace DB source is incomplete")
	}
	bindingPath, authorityPath, err := target.finalBindingPaths(leaf)
	if err != nil {
		return err
	}
	return publishSealedConversionFileWithBinding(
		ctx,
		source,
		target.stagingDir,
		leaf,
		bindingPath,
		authorityPath,
		sealedConversionPublicationRetainedTraceDB,
		ledger,
	)
}

// publishSealedConversionFileNoReplace publishes one already-sealed private
// generation through the same exact authority as retained trace DB outputs.
// It never opens, copies, renames, or removes the public path directly.
func publishSealedConversionFileNoReplace(
	ctx context.Context,
	target sealedConversionPublicationTarget,
	source *sealedConversionFile,
	ledger *conversionFileLedger,
) error {
	return publishSealedConversionFileNoReplaceWithValidation(ctx, target, source, ledger, nil)
}

func publishSealedConversionFileNoReplaceWithValidation(
	ctx context.Context,
	target sealedConversionPublicationTarget,
	source *sealedConversionFile,
	ledger *conversionFileLedger,
	validatePublished func(*retainedTraceDBPublication) error,
) error {
	if source == nil {
		return fmt.Errorf("sealed conversion output source is incomplete")
	}
	if ledger == nil {
		return fmt.Errorf("sealed conversion output ledger is required")
	}
	if source.dir != target.stagingDir || source.name != target.finalLeaf {
		return fmt.Errorf("sealed conversion output source does not belong to its publication target")
	}
	bindingPath, authorityPath, err := target.finalBindingPaths()
	if err != nil {
		return err
	}
	err = publishSealedConversionFileWithBindingValidation(
		ctx,
		source,
		target.stagingDir,
		target.finalLeaf,
		bindingPath,
		authorityPath,
		sealedConversionPublicationOutput,
		ledger,
		validatePublished,
	)
	if err != nil {
		return fmt.Errorf("publish sealed conversion output: %w", err)
	}
	return nil
}

// publishSealedConversionFileWithBinding is the compatibility wrapper for
// outputs which do not carry a semantic validation receipt. It delegates to
// the same exact publication/ledger throat with no post-snapshot callback.
func publishSealedConversionFileWithBinding(
	ctx context.Context,
	source *sealedConversionFile,
	stagingDir *privateConversionDir,
	leaf, bindingPath, authorityPath string,
	kind sealedConversionPublicationKind,
	ledger *conversionFileLedger,
) error {
	return publishSealedConversionFileWithBindingValidation(
		ctx, source, stagingDir, leaf, bindingPath, authorityPath, kind, ledger, nil,
	)
}

// publishSealedConversionFileWithBindingValidation is the only
// ledger-registration throat for exact no-replace publication. The optional
// callback attests the held public snapshot after its first strong binding and
// before recordSealedAuthority captures the transaction generation.
func publishSealedConversionFileWithBindingValidation(
	ctx context.Context,
	source *sealedConversionFile,
	stagingDir *privateConversionDir,
	leaf, bindingPath, authorityPath string,
	kind sealedConversionPublicationKind,
	ledger *conversionFileLedger,
	validatePublished func(*retainedTraceDBPublication) error,
) error {
	if source == nil || stagingDir == nil || ledger == nil || !kind.valid() || strings.TrimSpace(leaf) == "" ||
		strings.TrimSpace(bindingPath) == "" || strings.TrimSpace(authorityPath) == "" {
		return fmt.Errorf("%s publication inputs are incomplete", kind.diagnosticName())
	}
	publication, err := publishSealedConversionFilePlatform(
		ctx,
		source,
		stagingDir,
		leaf,
		bindingPath,
		authorityPath,
		kind,
	)
	if err != nil {
		return err
	}
	info, err := publication.Validate()
	if err != nil {
		return abortRetainedTraceDBPublication(publication.file, &publication.platform, leaf, kind, err)
	}
	if validatePublished != nil {
		if err := validatePublished(publication); err != nil {
			return traceDBJoinPreservingSingle(err, publication.Remove(), publication.Close())
		}
		info, err = publication.Validate()
		if err != nil {
			return traceDBJoinPreservingSingle(err, publication.Remove(), publication.Close())
		}
	}
	if err := ledger.recordSealedAuthority(bindingPath, info.Size(), publication); err != nil {
		return traceDBJoinPreservingSingle(err, publication.Remove(), publication.Close())
	}
	return nil
}

func (target traceStreamerDBTarget) finalCompanionLeaf() string {
	if target.finalLeaf == "" {
		return ""
	}
	return target.finalLeaf + ".ohos.ts"
}

func (target traceStreamerDBTarget) finalBindingPaths(leaf string) (bindingPath, authorityPath string, err error) {
	if target.stagingDir == nil || strings.TrimSpace(target.finalBindingPath) == "" || strings.TrimSpace(leaf) == "" {
		return "", "", fmt.Errorf("retained trace DB target authority is incomplete")
	}
	if err := validatePrivateConversionDirChildNamePlatform(leaf); err != nil {
		return "", "", fmt.Errorf("retained trace DB final leaf is invalid: %q: %w", leaf, err)
	}
	bindingPath = filepath.Join(filepath.Dir(target.finalBindingPath), leaf)
	authorityPath = filepath.Join(filepath.Dir(target.stagingDir.Path()), leaf)
	return bindingPath, authorityPath, nil
}
