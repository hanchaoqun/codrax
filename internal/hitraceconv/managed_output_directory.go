package hitraceconv

import "fmt"

// ManagedOutputDirectory exposes the existing held-directory authority to
// attachment preparation. It owns a unique private output directory; callers
// must call Cleanup on failure or Close to retain successful publications.
// It must not be copied. No cleanup operation reopens a pathname as authority.
type ManagedOutputDirectory struct {
	dir *privateConversionDir
}

// NewManagedOutputDirectory applies the converter's single platform security
// and fallback policy. The returned path is canonical, matching its held
// parent/guard handles rather than a mutable parent symlink spelling.
func NewManagedOutputDirectory(anchor, fallback, pattern string) (*ManagedOutputDirectory, error) {
	root, err := selectSecureConversionRuntimeAnchor(anchor, fallback)
	if err != nil {
		return nil, err
	}
	dir, err := newRuntimePrivateConversionDir(root, pattern)
	if err != nil {
		return nil, err
	}
	return &ManagedOutputDirectory{dir: dir}, nil
}

func (managed *ManagedOutputDirectory) Path() string {
	if managed == nil || managed.dir == nil {
		return ""
	}
	return managed.dir.Path()
}

func (managed *ManagedOutputDirectory) Validate() error {
	if managed == nil || managed.dir == nil {
		return fmt.Errorf("managed output directory authority is missing")
	}
	return managed.dir.Validate()
}

// Cleanup is terminal even when cleanup fails. The existing authority keeps
// recursive deletion rooted in held handles, refuses replaced directory
// entries, and releases all handles before returning to a long-lived caller.
func (managed *ManagedOutputDirectory) Cleanup() error {
	if managed == nil || managed.dir == nil {
		return nil
	}
	return managed.dir.FinalizeCleanup()
}

// Close validates before releasing authority and retains successful output.
// A validation failure leaves authority available for Cleanup. Once release
// starts, the authority becomes terminal even if a handle close fails: later
// Cleanup returns that release error and can never roll back via a stale path.
func (managed *ManagedOutputDirectory) Close() error {
	return managed.closeWithRelease(nil)
}

// closeWithRelease keeps failure injection local to one invocation; product
// callers always use the existing held-handle closer through Close.
func (managed *ManagedOutputDirectory) closeWithRelease(release func(*privateConversionDir) error) error {
	if managed == nil || managed.dir == nil {
		return nil
	}
	dir := managed.dir
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal {
		return dir.terminalErr
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		return err
	}
	if err := validatePrivateConversionDirSecurityPlatform(dir.path, dir.identity, &dir.platform); err != nil {
		return fmt.Errorf("%w: path=%s: %v", errPrivateConversionDirSecurityInvalid, dir.path, err)
	}
	if release == nil {
		release = (*privateConversionDir).closeHandlesLocked
	}
	dir.terminalErr = release(dir)
	dir.terminal = true
	return dir.terminalErr
}
