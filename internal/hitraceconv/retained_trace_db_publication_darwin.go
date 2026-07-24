//go:build darwin

package hitraceconv

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func publishSealedConversionFilePlatform(
	ctx context.Context,
	source *sealedConversionFile,
	dir *privateConversionDir,
	outputParent *publishedConversionFilePlatformState,
	leaf, bindingPath, authorityPath string,
	kind sealedConversionPublicationKind,
) (*retainedTraceDBPublication, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || dir == nil || outputParent == nil || outputParent.parentFD < 0 {
		return nil, fmt.Errorf("Darwin %s source authority is incomplete", kind.diagnosticName())
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Darwin %s source before clone: %w", kind.diagnosticName(), err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tempLeaf, err := nextPrivateConversionDirLeaf(kind.privateClonePattern())
	if err != nil {
		return nil, err
	}
	cloneErr := source.withOpenFile(func(sourceFile *os.File) error {
		dir.mu.Lock()
		defer dir.mu.Unlock()
		if dir.terminal || dir.platform.guardFD < 0 {
			return fmt.Errorf("Darwin %s staging authority is closed", kind.diagnosticName())
		}
		if err := dir.validateIdentityLocked(true); err != nil {
			return err
		}
		return unix.Fclonefileat(int(sourceFile.Fd()), dir.platform.guardFD, tempLeaf, unix.CLONE_NOOWNERCOPY)
	})
	if cloneErr != nil {
		return nil, fmt.Errorf("clone %s into private publication generation: %w", kind.sealedSourceName(), cloneErr)
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Darwin %s source after clone: %w", kind.diagnosticName(), err)
	}
	snapshot, err := dir.AdoptRegularChild(tempLeaf, false)
	if err != nil {
		return nil, fmt.Errorf("adopt Darwin %s publication generation: %w", kind.diagnosticName(), err)
	}
	if snapshot.Size() != source.Size() {
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("Darwin %s clone size mismatch: got=%d want=%d", kind.diagnosticName(), snapshot.Size(), source.Size()),
			snapshot.Close(),
		)
	}
	if err := snapshot.Validate(); err != nil {
		return nil, traceDBJoinPreservingSingle(err, snapshot.Close())
	}
	if err := ctx.Err(); err != nil {
		return nil, traceDBJoinPreservingSingle(err, snapshot.Close())
	}
	platform, err := duplicatePublishedConversionParentPlatform(outputParent, kind)
	if err != nil {
		return nil, traceDBJoinPreservingSingle(err, snapshot.Close())
	}
	file, renameErr := snapshot.publishAndDetachOpenFile(func(*os.File) error {
		dir.mu.Lock()
		defer dir.mu.Unlock()
		if dir.terminal || dir.platform.guardFD < 0 || outputParent.parentFD < 0 {
			return fmt.Errorf("Darwin %s authority closed before publication", kind.diagnosticName())
		}
		return unix.RenameatxNp(dir.platform.guardFD, tempLeaf, outputParent.parentFD, leaf, unix.RENAME_EXCL)
	})
	if renameErr != nil {
		platformCloseErr := closePublishedConversionFilePlatform(&platform)
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("atomically publish %s generation: %w", kind.diagnosticName(), renameErr), snapshot.Close(), platformCloseErr,
		)
	}
	publication, err := newRetainedTraceDBPublication(file, platform, kind, leaf, bindingPath, authorityPath, source.Size())
	if err != nil {
		return nil, abortRetainedTraceDBPublication(file, &platform, leaf, kind, err)
	}
	return publication, nil
}
