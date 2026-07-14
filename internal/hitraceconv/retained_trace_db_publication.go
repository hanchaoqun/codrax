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
	leaf          string
	bindingPath   string
	authorityPath string
	platform      publishedConversionFilePlatformState
	removed       bool
	closed        bool
}

func newRetainedTraceDBPublication(
	file *os.File,
	platform publishedConversionFilePlatformState,
	leaf, bindingPath, authorityPath string,
	allowedSize int64,
) (*retainedTraceDBPublication, error) {
	if file == nil || strings.TrimSpace(leaf) == "" || allowedSize < 0 {
		return nil, fmt.Errorf("retained trace DB publication authority is incomplete")
	}
	identity, err := filegeneration.FromFile(file)
	if err != nil {
		return nil, fmt.Errorf("capture retained trace DB publication identity: %w", err)
	}
	if !identity.Strong() || !identity.Mode().IsRegular() || identity.Size() != allowedSize {
		return nil, fmt.Errorf("retained trace DB publication has invalid generation: strong=%t mode=%s size=%d want=%d",
			identity.Strong(), identity.Mode(), identity.Size(), allowedSize)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat retained trace DB publication: %w", err)
	}
	publication := &retainedTraceDBPublication{
		file: file, identity: identity, identityInfo: info, size: allowedSize,
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
	if publication.closed || publication.removed || publication.file == nil || !publication.identity.Initialized() {
		return nil, fmt.Errorf("retained trace DB publication authority is closed or removed: %s", publication.bindingPath)
	}
	current, err := filegeneration.FromFile(publication.file)
	if err != nil || !current.Strong() || !publication.identity.SameVersion(current) {
		return nil, fmt.Errorf("retained trace DB publication generation changed: %s", publication.bindingPath)
	}
	info, err := publication.file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != publication.size ||
		!os.SameFile(publication.identityInfo, info) {
		return nil, fmt.Errorf("retained trace DB publication identity changed: %s", publication.bindingPath)
	}
	if err := validatePublishedConversionFilePlatform(&publication.platform, publication.leaf, publication.file, info); err != nil {
		return nil, fmt.Errorf("validate retained trace DB parent-relative binding %s: %w", publication.bindingPath, err)
	}
	for _, path := range dedupeStrings([]string{publication.bindingPath, publication.authorityPath}) {
		currentPath, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("validate retained trace DB public binding %s: %w", path, err)
		}
		if !currentPath.Mode().IsRegular() || currentPath.Size() != publication.size || !os.SameFile(info, currentPath) {
			return nil, fmt.Errorf("retained trace DB public binding changed: %s", path)
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
		return fmt.Errorf("retained trace DB publication authority closed before rollback: %s", publication.bindingPath)
	}
	if _, err := publication.validateLocked(); err != nil {
		return err
	}
	if err := removePublishedConversionFilePlatform(&publication.platform, publication.leaf, publication.file); err != nil {
		return fmt.Errorf("remove retained trace DB publication %s: %w", publication.bindingPath, err)
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
		return nil
	}
	publication.closed = true
	var fileErr error
	if publication.file != nil {
		fileErr = publication.file.Close()
		publication.file = nil
	}
	platformErr := closePublishedConversionFilePlatform(&publication.platform)
	return traceDBJoinPreservingSingle(fileErr, platformErr)
}

func abortRetainedTraceDBPublication(
	file *os.File,
	platform *publishedConversionFilePlatformState,
	leaf string,
	primary error,
) error {
	if file == nil {
		return traceDBJoinPreservingSingle(primary, closePublishedConversionFilePlatform(platform))
	}
	removeErr := removePublishedConversionFilePlatform(platform, leaf, file)
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
	publication, err := publishSealedConversionFilePlatform(ctx, source, target.stagingDir, leaf, bindingPath, authorityPath)
	if err != nil {
		return err
	}
	info, err := publication.Validate()
	if err != nil {
		return abortRetainedTraceDBPublication(publication.file, &publication.platform, leaf, err)
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
