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
	leaf, bindingPath, authorityPath string,
) (*retainedTraceDBPublication, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || dir == nil {
		return nil, fmt.Errorf("Darwin retained trace DB source authority is incomplete")
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Darwin retained trace DB source before clone: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tempLeaf, err := nextPrivateConversionDirLeaf(".codrax-retained-db-*")
	if err != nil {
		return nil, err
	}
	cloneErr := source.withOpenFile(func(sourceFile *os.File) error {
		dir.mu.Lock()
		defer dir.mu.Unlock()
		if dir.terminal || dir.platform.guardFD < 0 {
			return fmt.Errorf("Darwin retained trace DB staging authority is closed")
		}
		if err := dir.validateIdentityLocked(true); err != nil {
			return err
		}
		return unix.Fclonefileat(int(sourceFile.Fd()), dir.platform.guardFD, tempLeaf, unix.CLONE_NOOWNERCOPY)
	})
	if cloneErr != nil {
		return nil, fmt.Errorf("clone sealed trace DB into private publication generation: %w", cloneErr)
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Darwin retained trace DB source after clone: %w", err)
	}
	snapshot, err := dir.AdoptRegularChild(tempLeaf, false)
	if err != nil {
		return nil, fmt.Errorf("adopt Darwin retained trace DB publication generation: %w", err)
	}
	if snapshot.Size() != source.Size() {
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("Darwin retained trace DB clone size mismatch: got=%d want=%d", snapshot.Size(), source.Size()),
			snapshot.Close(),
		)
	}
	if err := snapshot.Validate(); err != nil {
		return nil, traceDBJoinPreservingSingle(err, snapshot.Close())
	}
	if err := ctx.Err(); err != nil {
		return nil, traceDBJoinPreservingSingle(err, snapshot.Close())
	}
	platform, err := duplicatePublishedConversionParentPlatform(dir)
	if err != nil {
		return nil, traceDBJoinPreservingSingle(err, snapshot.Close())
	}
	file, renameErr := snapshot.publishAndDetachOpenFile(func(*os.File) error {
		dir.mu.Lock()
		defer dir.mu.Unlock()
		if dir.terminal || dir.platform.guardFD < 0 || dir.platform.parentFD < 0 {
			return fmt.Errorf("Darwin retained trace DB authority closed before publication")
		}
		return unix.RenameatxNp(dir.platform.guardFD, tempLeaf, dir.platform.parentFD, leaf, unix.RENAME_EXCL)
	})
	if renameErr != nil {
		platformCloseErr := closePublishedConversionFilePlatform(&platform)
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("atomically publish retained trace DB generation: %w", renameErr), snapshot.Close(), platformCloseErr,
		)
	}
	publication, err := newRetainedTraceDBPublication(file, platform, leaf, bindingPath, authorityPath, source.Size())
	if err != nil {
		return nil, abortRetainedTraceDBPublication(file, &platform, leaf, err)
	}
	return publication, nil
}
