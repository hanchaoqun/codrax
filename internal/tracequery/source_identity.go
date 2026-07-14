package tracequery

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// traceFileIdentity remains a package-local name so the rest of tracequery
// does not expose the shared implementation. The interpretation itself lives
// only in filegeneration and is also consumed by trace conversion.
type traceFileIdentity = filegeneration.Identity

func traceFileIdentityFromInfo(info os.FileInfo) traceFileIdentity {
	return filegeneration.FromInfo(info)
}

func traceAnchorKeyForIdentity(path string, identity traceFileIdentity) traceAnchorKey {
	return traceAnchorKey{
		path:     path,
		size:     identity.Size(),
		modUnix:  identity.ModUnixNano(),
		identity: identity.CacheToken(),
		version:  ParserVersion,
	}
}

func traceAnchorKeyForInfo(path string, info os.FileInfo) traceAnchorKey {
	return traceAnchorKeyForIdentity(path, traceFileIdentityFromInfo(info))
}

// TraceSourceVersion is an opaque, immutable identity for the complete source
// universe selected by a trace path (the physical trace plus any sibling
// artifacts, or a bundle manifest plus every child).  Multi-stage consumers
// use it to prevent one report from mixing evidence read from different file
// generations.  The private token retains the strong size/mtime/mode/dev/
// inode/ctime ledger; Fingerprint exposes only a deterministic digest.
type TraceSourceVersion struct {
	path        string
	sourceBytes int64
	token       string
	fingerprint string
}

func CaptureTraceSourceVersion(path string) (TraceSourceVersion, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return TraceSourceVersion{}, fmt.Errorf("trace source version: path is empty")
	}
	path = canonicalTraceIndexPath(path)
	entry, err := os.Stat(path)
	if err != nil {
		return TraceSourceVersion{}, fmt.Errorf("trace source version: %w", err)
	}
	bytes, token, err := traceIndexSourceIdentity(path, entry)
	if err != nil {
		return TraceSourceVersion{}, fmt.Errorf("trace source version: %w", err)
	}
	sum := sha256.Sum256([]byte(token))
	return TraceSourceVersion{
		path:        path,
		sourceBytes: bytes,
		token:       token,
		fingerprint: fmt.Sprintf("sha256:%x", sum[:]),
	}, nil
}

// Validate verifies that path still resolves to exactly the same source
// universe.  It intentionally fails when called with a different canonical
// path even if that path currently aliases identical bytes: provenance is part
// of the run contract.
func (v TraceSourceVersion) Validate(path string) error {
	if v.path == "" || v.token == "" {
		return fmt.Errorf("trace source version is uninitialized")
	}
	current, err := CaptureTraceSourceVersion(path)
	if err != nil {
		return err
	}
	if current.path != v.path || current.sourceBytes != v.sourceBytes || current.token != v.token {
		return fmt.Errorf("trace source universe changed during multi-stage collection; discard mixed-version results and retry")
	}
	return nil
}

func (v TraceSourceVersion) Fingerprint() string { return v.fingerprint }
func (v TraceSourceVersion) SourceBytes() int64  { return v.sourceBytes }

// validateTraceFileIdentityAfterRead closes the streaming TOCTOU window. A
// source can be replaced or rewritten after the initial stat/open check while
// a long scan is in progress; publishing mixed-version rows (or caching their
// anchors) would create evidence that never existed in one physical artifact.
func validateTraceFileIdentityAfterRead(file *os.File, opened traceFileIdentity, operation string) error {
	if file == nil || !opened.Initialized() {
		return fmt.Errorf("trace source identity unavailable after %s", operation)
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("trace source identity check after %s: %w", operation, err)
	}
	if !opened.MatchesInfo(finalInfo) {
		return fmt.Errorf("trace source identity changed during %s; discard mixed-version streaming results and retry", operation)
	}
	pathInfo, err := os.Stat(file.Name())
	if err != nil {
		return fmt.Errorf("trace source path identity check after %s: %w", operation, err)
	}
	if !opened.MatchesInfo(pathInfo) {
		return fmt.Errorf("trace source path was replaced during %s; discard stale-path streaming results and retry", operation)
	}
	return nil
}
