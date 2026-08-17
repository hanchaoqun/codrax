package hitraceconv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	errPrivateConversionDirIdentityChanged = errors.New("private conversion directory identity changed")
	errPrivateConversionDirSecurityInvalid = errors.New("private conversion directory security invalid")
)

const (
	privateConversionDirCreateAttempts    = 128
	privateConversionDirCleanupBatch      = 128
	privateConversionDirCleanupEntryLimit = 1 << 20
	conversionRuntimeDirName              = ".codrax"
)

func resolveConversionRuntimeAnchor(configured, output string) (string, error) {
	root := strings.TrimSpace(configured)
	if root == "" {
		output = strings.TrimSpace(output)
		if output == "" {
			return "", fmt.Errorf("conversion output path is required to derive the runtime anchor")
		}
		absoluteOutput, err := filepath.Abs(filepath.Clean(output))
		if err != nil {
			return "", fmt.Errorf("resolve conversion output path for runtime anchor: %w", err)
		}
		root = filepath.Join(filepath.Dir(absoluteOutput), conversionRuntimeDirName)
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve conversion runtime anchor %s: %w", root, err)
	}
	return absoluteRoot, nil
}

// selectSecureConversionRuntimeAnchor keeps the caller-selected anchor unless
// that filesystem precisely fails the private-directory security contract.
// A fallback is never used for permission, identity, I/O, or cleanup errors:
// those remain fail-loud rather than being hidden by a path switch.
func selectSecureConversionRuntimeAnchor(primary, fallback string) (string, error) {
	return selectSecureConversionRuntimeAnchorWithProbe(primary, fallback, func(root string) error {
		dir, err := newRuntimePrivateConversionDir(root, ".anchor-probe-*")
		if err != nil {
			return err
		}
		return dir.FinalizeCleanup()
	})
}

func selectSecureConversionRuntimeAnchorWithProbe(primary, fallback string, probe func(string) error) (string, error) {
	primary = strings.TrimSpace(primary)
	fallback = strings.TrimSpace(fallback)
	if primary == "" || fallback == "" || sameConversionCanonicalPath(primary, fallback) {
		return primary, nil
	}
	if probe == nil {
		return "", fmt.Errorf("conversion runtime anchor probe is required")
	}
	primaryErr := probe(primary)
	if primaryErr == nil {
		return primary, nil
	}
	if !errors.Is(primaryErr, errPrivateConversionDirSecurityInvalid) {
		return "", fmt.Errorf("validate conversion runtime anchor %s: %w", primary, primaryErr)
	}
	fallbackRoot, err := filepath.Abs(filepath.Clean(fallback))
	if err != nil {
		return "", fmt.Errorf("resolve conversion runtime fallback %s: %w", fallback, err)
	}
	if fallbackErr := probe(fallbackRoot); fallbackErr != nil {
		return "", traceDBJoinPreservingSingle(
			fmt.Errorf("conversion runtime anchor cannot enforce private directory security: %s: %w", primary, primaryErr),
			fmt.Errorf("conversion runtime fallback cannot enforce private directory security: %s: %w", fallbackRoot, fallbackErr),
		)
	}
	return fallbackRoot, nil
}

func ensureConversionRuntimeAnchor(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("conversion runtime anchor is required")
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve conversion runtime anchor %s: %w", root, err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o755); err != nil {
		return "", fmt.Errorf("create conversion runtime anchor %s: %w", absoluteRoot, err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return "", fmt.Errorf("inspect conversion runtime anchor %s: %w", absoluteRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("conversion runtime anchor is not a directory: %s", absoluteRoot)
	}
	return absoluteRoot, nil
}

func newRuntimePrivateConversionDir(runtimeAnchor, pattern string) (*privateConversionDir, error) {
	root, err := ensureConversionRuntimeAnchor(runtimeAnchor)
	if err != nil {
		return nil, err
	}
	return newPrivateConversionDir(root, pattern)
}

func nextPrivateConversionDirLeaf(pattern string) (string, error) {
	prefix, suffix, err := splitPrivateConversionDirPattern(pattern)
	if err != nil {
		return "", err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate private conversion directory name: %w", err)
	}
	return prefix + hex.EncodeToString(random) + suffix, nil
}

func splitPrivateConversionDirPattern(pattern string) (prefix, suffix string, err error) {
	for index := 0; index < len(pattern); index++ {
		if os.IsPathSeparator(pattern[index]) {
			return "", "", fmt.Errorf("pattern contains path separator")
		}
	}
	if star := strings.LastIndexByte(pattern, '*'); star >= 0 {
		return pattern[:star], pattern[star+1:], nil
	}
	return pattern, "", nil
}

// privateConversionDir is the single, non-copyable authority for a
// conversion-owned staging directory. Unix root and the platform-specific
// parent/guard handles remain anchored even if a hostile ancestor is renamed;
// Windows intentionally uses only its DELETE-capable NT handle authority.
type privateConversionDir struct {
	mu          sync.Mutex
	path        string
	identity    os.FileInfo
	root        *os.Root
	platform    privateConversionDirPlatformState
	terminal    bool
	terminalErr error
}

func newPrivateConversionDir(parent, pattern string) (*privateConversionDir, error) {
	path, identity, platform, err := createPrivateConversionDirPlatform(parent, pattern)
	if err != nil {
		return nil, err
	}
	abort := func(primary error) (*privateConversionDir, error) {
		removeErr := removePrivateConversionDirRootPlatform(path, identity, &platform)
		closeErr := closePrivateConversionDirPlatform(&platform)
		return nil, traceDBJoinPreservingSingle(primary, removeErr, closeErr)
	}
	if err := validatePrivateConversionDirPublicBindingPlatform(path, identity, &platform); err != nil {
		return abort(fmt.Errorf("%w: %s: %v", errPrivateConversionDirIdentityChanged, path, err))
	}
	root, err := openPrivateConversionDirRootPlatform(path, identity, &platform)
	if err != nil {
		return abort(fmt.Errorf("open private conversion directory root %s: %w", path, err))
	}
	if root != nil {
		rootInfo, rootErr := root.Stat(".")
		if rootErr == nil && (!rootInfo.IsDir() || !os.SameFile(identity, rootInfo)) {
			rootErr = errPrivateConversionDirIdentityChanged
		}
		if rootErr != nil {
			closeErr := root.Close()
			return abort(traceDBJoinPreservingSingle(
				fmt.Errorf("bind private conversion directory root %s: %w", path, rootErr), closeErr,
			))
		}
	}
	dir := &privateConversionDir{path: path, identity: identity, root: root, platform: platform}
	if err := dir.Validate(); err != nil {
		return nil, traceDBJoinPreservingSingle(err, dir.FinalizeCleanup())
	}
	return dir, nil
}

func (dir *privateConversionDir) Path() string {
	if dir == nil {
		return ""
	}
	return dir.path
}

func (dir *privateConversionDir) ChildPath(base string) (string, error) {
	if dir == nil || strings.TrimSpace(dir.path) == "" {
		return "", fmt.Errorf("private conversion directory authority is missing")
	}
	if base == "" || base == "." || base == ".." || !filepath.IsLocal(base) ||
		filepath.IsAbs(base) || filepath.Base(base) != base {
		return "", fmt.Errorf("private conversion directory child name is invalid: %q", base)
	}
	if err := validatePrivateConversionDirChildNamePlatform(base); err != nil {
		return "", fmt.Errorf("private conversion directory child name is invalid: %q: %w", base, err)
	}
	return filepath.Join(dir.path, base), nil
}

func (dir *privateConversionDir) Validate() error {
	if dir == nil {
		return fmt.Errorf("private conversion directory authority is missing")
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal {
		return traceDBJoinPreservingSingle(fmt.Errorf("private conversion directory authority is terminal: %s", dir.path), dir.terminalErr)
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		return err
	}
	if err := validatePrivateConversionDirSecurityPlatform(dir.path, dir.identity, &dir.platform); err != nil {
		return fmt.Errorf("%w: path=%s: %v", errPrivateConversionDirSecurityInvalid, dir.path, err)
	}
	return nil
}

func (dir *privateConversionDir) validateIdentityLocked(requirePublicBinding bool) error {
	if strings.TrimSpace(dir.path) == "" || dir.identity == nil {
		return fmt.Errorf("%w: authority is incomplete", errPrivateConversionDirIdentityChanged)
	}
	if requirePublicBinding {
		if err := validatePrivateConversionDirPublicBindingPlatform(dir.path, dir.identity, &dir.platform); err != nil {
			return fmt.Errorf("%w: public binding mismatch %s: %v", errPrivateConversionDirIdentityChanged, dir.path, err)
		}
	}
	if err := validatePrivateConversionDirIdentityPlatform(dir.path, dir.identity, &dir.platform); err != nil {
		return fmt.Errorf("%w: path=%s: %v", errPrivateConversionDirIdentityChanged, dir.path, err)
	}
	return nil
}

func (dir *privateConversionDir) Cleanup() error {
	if dir == nil {
		return nil
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal {
		return dir.terminalErr
	}
	if err := preparePrivateConversionDirCleanupPlatform(dir.path, dir.identity, dir.root, &dir.platform); err != nil {
		result := fmt.Errorf("prepare private conversion directory cleanup %s: %w", dir.path, err)
		if errors.Is(err, errPrivateConversionDirIdentityChanged) {
			dir.terminalErr = traceDBJoinPreservingSingle(result, dir.closeHandlesLocked())
			dir.terminal = true
			return dir.terminalErr
		}
		return result
	}
	if err := dir.validateIdentityLocked(false); err != nil {
		dir.terminalErr = traceDBJoinPreservingSingle(err, dir.closeHandlesLocked())
		dir.terminal = true
		return dir.terminalErr
	}
	if err := dir.removeChildrenLocked(); err != nil {
		return err
	}
	if err := dir.validateIdentityLocked(false); err != nil {
		dir.terminalErr = traceDBJoinPreservingSingle(err, dir.closeHandlesLocked())
		dir.terminal = true
		return dir.terminalErr
	}
	if err := removePrivateConversionDirRootPlatform(dir.path, dir.identity, &dir.platform); err != nil {
		return fmt.Errorf("remove private conversion directory root %s: %w", dir.path, err)
	}
	var closeErr error
	if dir.root != nil {
		if err := dir.root.Close(); err != nil {
			closeErr = fmt.Errorf("close private conversion directory root %s: %w", dir.path, err)
		}
		dir.root = nil
	}
	if err := closePrivateConversionDirPlatform(&dir.platform); err != nil {
		closeErr = traceDBJoinPreservingSingle(closeErr,
			fmt.Errorf("close private conversion directory platform authority %s: %w", dir.path, err))
	}
	dir.terminal = true
	dir.terminalErr = closeErr
	return closeErr
}

// FinalizeCleanup is the provider-facing terminal cleanup boundary. Cleanup
// intentionally retains its held handles after retryable I/O failures so a
// caller that still owns the authority can retry. Providers are returning to
// the long-lived process and cannot retain that authority; they must close all
// handles after reporting the cleanup failure instead of leaking an FD/handle
// and an apparently live cleanup capability.
func (dir *privateConversionDir) FinalizeCleanup() error {
	if dir == nil {
		return nil
	}
	cleanupErr := dir.Cleanup()
	if cleanupErr == nil {
		return nil
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal {
		return dir.terminalErr
	}
	dir.terminalErr = traceDBJoinPreservingSingle(cleanupErr, dir.closeHandlesLocked())
	dir.terminal = true
	return dir.terminalErr
}

// privateConversionDirCommandBoundaryError preserves cancellation and the
// child-process failure while making a post-command staging-integrity failure
// fail loud. A run error by itself stays on the provider's established typed
// fallback path, so this helper returns nil in that one case.
func privateConversionDirCommandBoundaryError(ctx context.Context, runErr error, dir *privateConversionDir) error {
	var contextErr error
	if ctx != nil {
		contextErr = ctx.Err()
	}
	validationErr := dir.Validate()
	if contextErr != nil {
		return traceDBJoinPreservingSingle(contextErr, runErr, validationErr)
	}
	if validationErr != nil {
		return traceDBJoinPreservingSingle(validationErr, runErr)
	}
	return nil
}

func (dir *privateConversionDir) removeChildrenLocked() error {
	if dir.root == nil {
		return removePrivateConversionDirChildrenPlatform(dir.path, dir.identity, &dir.platform)
	}
	removed := 0
	for {
		file, err := dir.root.Open(".")
		if err != nil {
			return fmt.Errorf("open private conversion directory for cleanup: %w", err)
		}
		names, readErr := file.Readdirnames(privateConversionDirCleanupBatch)
		closeErr := file.Close()
		if closeErr != nil {
			return fmt.Errorf("close private conversion directory cleanup reader: %w", closeErr)
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("enumerate private conversion directory for cleanup: %w", readErr)
		}
		if len(names) == 0 {
			return nil
		}
		if removed > privateConversionDirCleanupEntryLimit-len(names) {
			return fmt.Errorf("private conversion directory cleanup entry limit exceeded: %d", privateConversionDirCleanupEntryLimit)
		}
		for _, name := range names {
			if err := dir.root.RemoveAll(name); err != nil {
				return fmt.Errorf("remove private conversion directory child %q: %w", name, err)
			}
		}
		removed += len(names)
	}
}

func (dir *privateConversionDir) closeHandlesLocked() error {
	var result error
	if dir.root != nil {
		result = traceDBJoinPreservingSingle(result, dir.root.Close())
		dir.root = nil
	}
	result = traceDBJoinPreservingSingle(result, closePrivateConversionDirPlatform(&dir.platform))
	return result
}
