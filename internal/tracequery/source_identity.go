package tracequery

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// traceFileIdentity is the private, low-cost version ledger for one opened
// trace artifact. Size and mtime are the portable baseline. On Unix-like
// systems os.FileInfo.Sys() also exposes device, inode, and ctime; those fields
// detect both atomic replacement and same-size in-place rewrites even when a
// caller restores mtime. We deliberately do not hash trace contents here:
// multi-GB captures must keep source validation O(1).
//
// The reflection is intentional. Linux and Darwin spell the ctime member
// differently (Ctim vs Ctimespec), while both expose the same exported scalar
// fields. Keeping that adaptation here avoids platform-specific syscall types
// leaking through the parser and retains a portable size+mtime+mode fallback
// on systems whose FileInfo does not expose a stable file/change identity.
type traceFileIdentity struct {
	size        int64
	modUnixNano int64
	mode        uint32
	device      uint64
	inode       uint64
	changeSec   int64
	changeNsec  int64
	strong      bool
	initialized bool
}

func traceFileIdentityFromInfo(info os.FileInfo) traceFileIdentity {
	if info == nil {
		return traceFileIdentity{}
	}
	id := traceFileIdentity{
		size:        info.Size(),
		modUnixNano: info.ModTime().UnixNano(),
		mode:        uint32(info.Mode()),
		initialized: true,
	}
	sys := reflect.ValueOf(info.Sys())
	for sys.IsValid() && (sys.Kind() == reflect.Pointer || sys.Kind() == reflect.Interface) {
		if sys.IsNil() {
			return id
		}
		sys = sys.Elem()
	}
	if !sys.IsValid() || sys.Kind() != reflect.Struct {
		return id
	}
	device, deviceOK := traceIdentityUintField(sys, "Dev")
	inode, inodeOK := traceIdentityUintField(sys, "Ino")
	changeSec, changeNsec, changeOK := traceIdentityChangeTime(sys)
	if deviceOK && inodeOK && changeOK {
		id.device = device
		id.inode = inode
		id.changeSec = changeSec
		id.changeNsec = changeNsec
		id.strong = true
	}
	return id
}

func traceIdentityUintField(value reflect.Value, name string) (uint64, bool) {
	field := value.FieldByName(name)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		got := field.Int()
		if got < 0 {
			return 0, false
		}
		return uint64(got), true
	default:
		return 0, false
	}
}

func traceIdentityIntField(value reflect.Value, name string) (int64, bool) {
	field := value.FieldByName(name)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		got := field.Uint()
		if got > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(got), true
	default:
		return 0, false
	}
}

func traceIdentityChangeTime(value reflect.Value) (int64, int64, bool) {
	// Linux/BSD generally use Ctim; Darwin uses Ctimespec.
	for _, name := range []string{"Ctim", "Ctimespec"} {
		stamp := value.FieldByName(name)
		for stamp.IsValid() && stamp.Kind() == reflect.Pointer {
			if stamp.IsNil() {
				break
			}
			stamp = stamp.Elem()
		}
		if !stamp.IsValid() || stamp.Kind() != reflect.Struct {
			continue
		}
		sec, secOK := traceIdentityIntField(stamp, "Sec")
		nsec, nsecOK := traceIdentityIntField(stamp, "Nsec")
		if secOK && nsecOK {
			return sec, nsec, true
		}
	}
	// Some Unix Stat_t variants expose scalar ctime members.
	sec, secOK := traceIdentityIntField(value, "Ctime")
	if !secOK {
		return 0, 0, false
	}
	for _, name := range []string{"Ctimensec", "CtimeNsec", "Ctime_nsec"} {
		if nsec, ok := traceIdentityIntField(value, name); ok {
			return sec, nsec, true
		}
	}
	return sec, 0, true
}

func (id traceFileIdentity) sameVersion(other traceFileIdentity) bool {
	if !id.initialized || !other.initialized {
		return false
	}
	if id.size != other.size || id.modUnixNano != other.modUnixNano || id.mode != other.mode {
		return false
	}
	if id.strong || other.strong {
		return id.strong && other.strong &&
			id.device == other.device && id.inode == other.inode &&
			id.changeSec == other.changeSec && id.changeNsec == other.changeNsec
	}
	return true
}

func (id traceFileIdentity) matchesInfo(info os.FileInfo) bool {
	return id.sameVersion(traceFileIdentityFromInfo(info))
}

func (id traceFileIdentity) cacheToken() string {
	if !id.initialized {
		return "uninitialized"
	}
	if id.strong {
		return fmt.Sprintf("strong:%d:%d:%d:%d:%d:%d:%d", id.size, id.modUnixNano, id.mode, id.device, id.inode, id.changeSec, id.changeNsec)
	}
	return fmt.Sprintf("portable:%d:%d:%d", id.size, id.modUnixNano, id.mode)
}

func traceAnchorKeyForIdentity(path string, identity traceFileIdentity) traceAnchorKey {
	return traceAnchorKey{
		path:     path,
		size:     identity.size,
		modUnix:  identity.modUnixNano,
		identity: identity.cacheToken(),
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
	if file == nil || !opened.initialized {
		return fmt.Errorf("trace source identity unavailable after %s", operation)
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("trace source identity check after %s: %w", operation, err)
	}
	if !opened.matchesInfo(finalInfo) {
		return fmt.Errorf("trace source identity changed during %s; discard mixed-version streaming results and retry", operation)
	}
	pathInfo, err := os.Stat(file.Name())
	if err != nil {
		return fmt.Errorf("trace source path identity check after %s: %w", operation, err)
	}
	if !opened.matchesInfo(pathInfo) {
		return fmt.Errorf("trace source path was replaced during %s; discard stale-path streaming results and retry", operation)
	}
	return nil
}
