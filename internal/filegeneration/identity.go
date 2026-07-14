package filegeneration

import (
	"fmt"
	"os"
	"reflect"
)

// Identity is the immutable, O(1) generation ledger for one physical file.
// Size, mtime and mode form the portable observation. Strong identities add a
// stable file key plus a change clock so same-size rewrites with restored mtime
// and atomic replacement remain distinguishable.
//
// The fields intentionally stay private. Consumers must compare complete
// identities through SameVersion instead of selecting a weaker subset.
type Identity struct {
	size            int64
	modUnixNano     int64
	mode            uint32
	device          uint64
	inode           uint64
	changePrimary   int64
	changeSecondary int64
	strong          bool
	initialized     bool
}

// FromInfo captures every generation field exposed by os.FileInfo. Unix-like
// systems normally expose device, inode and ctime here. Platforms which need a
// live handle for their strong file identity are completed by FromFile.
func FromInfo(info os.FileInfo) Identity {
	if info == nil {
		return Identity{}
	}
	id := Identity{
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
	device, deviceOK := uintField(sys, "Dev")
	inode, inodeOK := uintField(sys, "Ino")
	changePrimary, changeSecondary, changeOK := changeTime(sys)
	if deviceOK && inodeOK && changeOK {
		id.device = device
		id.inode = inode
		id.changePrimary = changePrimary
		id.changeSecondary = changeSecondary
		id.strong = true
	}
	return id
}

// FromFile captures the strongest identity available for an already-open
// handle. It never reopens file.Name().
func FromFile(file *os.File) (Identity, error) {
	if file == nil {
		return Identity{}, fmt.Errorf("file generation identity: file is nil")
	}
	info, err := file.Stat()
	if err != nil {
		return Identity{}, fmt.Errorf("file generation identity: %w", err)
	}
	return enhanceIdentityFromFile(file, info, FromInfo(info)), nil
}

// FromPath opens path only long enough to capture its strongest identity. It
// is intended for explicit path-binding validation, not for content reads.
func FromPath(path string) (Identity, error) {
	file, err := openPathForIdentity(path)
	if err != nil {
		return Identity{}, err
	}
	id, captureErr := FromFile(file)
	closeErr := file.Close()
	if captureErr != nil {
		return Identity{}, captureErr
	}
	if closeErr != nil {
		return Identity{}, closeErr
	}
	return id, nil
}

// validWindowsStrongIdentity is kept platform-neutral so the Windows
// admission contract has executable tests on every development host.
func validWindowsStrongIdentity(volume, fileIndex uint64, changeTime int64) bool {
	return volume != 0 && fileIndex != 0 && changeTime != 0
}

// NewPortable reconstructs the legacy size/mtime/mode observation used by
// persisted provenance. It is deliberately never strong.
func NewPortable(size, modUnixNano int64, mode os.FileMode) Identity {
	return Identity{
		size: size, modUnixNano: modUnixNano, mode: uint32(mode), initialized: true,
	}
}

func (id Identity) Size() int64        { return id.size }
func (id Identity) ModUnixNano() int64 { return id.modUnixNano }
func (id Identity) Mode() os.FileMode  { return os.FileMode(id.mode) }
func (id Identity) Strong() bool       { return id.strong }
func (id Identity) Initialized() bool  { return id.initialized }

// SameVersion compares the complete available generation tuple. A strong and
// a portable identity never compare equal: silently dropping the strong half
// would turn a precise hard gate into a noisy one.
func (id Identity) SameVersion(other Identity) bool {
	if !id.initialized || !other.initialized {
		return false
	}
	if id.size != other.size || id.modUnixNano != other.modUnixNano || id.mode != other.mode {
		return false
	}
	if id.strong || other.strong {
		return id.strong && other.strong &&
			id.device == other.device && id.inode == other.inode &&
			id.changePrimary == other.changePrimary && id.changeSecondary == other.changeSecondary
	}
	return true
}

func (id Identity) MatchesInfo(info os.FileInfo) bool {
	return id.SameVersion(FromInfo(info))
}

func (id Identity) MatchesFile(file *os.File) bool {
	other, err := FromFile(file)
	return err == nil && id.SameVersion(other)
}

// CacheToken is opaque outside this package's comparison contract. Its shape
// remains stable for existing tracequery cache and provenance keys.
func (id Identity) CacheToken() string {
	if !id.initialized {
		return "uninitialized"
	}
	if id.strong {
		return fmt.Sprintf("strong:%d:%d:%d:%d:%d:%d:%d", id.size, id.modUnixNano, id.mode, id.device, id.inode, id.changePrimary, id.changeSecondary)
	}
	return fmt.Sprintf("portable:%d:%d:%d", id.size, id.modUnixNano, id.mode)
}

func uintField(value reflect.Value, name string) (uint64, bool) {
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

func intField(value reflect.Value, name string) (int64, bool) {
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

func changeTime(value reflect.Value) (int64, int64, bool) {
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
		sec, secOK := intField(stamp, "Sec")
		nsec, nsecOK := intField(stamp, "Nsec")
		if secOK && nsecOK {
			return sec, nsec, true
		}
	}
	// Some Unix Stat_t variants expose scalar ctime members.
	sec, secOK := intField(value, "Ctime")
	if !secOK {
		return 0, 0, false
	}
	for _, name := range []string{"Ctimensec", "CtimeNsec", "Ctime_nsec"} {
		if nsec, ok := intField(value, name); ok {
			return sec, nsec, true
		}
	}
	return sec, 0, true
}
