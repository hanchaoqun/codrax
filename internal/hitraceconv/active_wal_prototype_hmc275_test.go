//go:build darwin

package hitraceconv

// Experimental, test-only capture. This is deliberately not connected to any
// product entry point. A dedicated helper process owns all source descriptors;
// POSIX locks are process-scoped and must not share a process with a writer.

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"golang.org/x/sys/unix"
	"modernc.org/libc"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type hmc275WALVFS struct {
	ctx                           context.Context
	files                         [3]*os.File // main, WAL, SHM: held O_RDONLY, never reopened by SQLite
	identities                    [3]os.FileInfo
	mappings                      map[int32][]byte
	name                          string
	token, cvfs, cname, methods   uintptr
	tls                           *libc.TLS
	reads, shmLocks, deniedWrites int
	registered                    bool
	closeSHMBefore, closeSHMAfter filegeneration.Identity
	closeSHMChanged               bool
}

var hmc275VFSRegistry = struct {
	sync.Mutex
	next  uintptr
	items map[uintptr]*hmc275WALVFS
}{items: make(map[uintptr]*hmc275WALVFS)}

type hmc275VFSFile struct {
	methods, token uintptr
	role           int32
}

func hmc275FP(fn any) uintptr { return (*[2]uintptr)(unsafe.Pointer(&fn))[1] }
func hmc275State(token uintptr) *hmc275WALVFS {
	hmc275VFSRegistry.Lock()
	defer hmc275VFSRegistry.Unlock()
	return hmc275VFSRegistry.items[token]
}
func hmc275FromFile(p uintptr) (*hmc275WALVFS, int32) {
	f := (*hmc275VFSFile)(unsafe.Pointer(p))
	return hmc275State(f.token), f.role
}
func hmc275Lock(fd uintptr, kind int16, start, length int64) error {
	lock := unix.Flock_t{Type: kind, Whence: 0, Start: start, Len: length}
	return unix.FcntlFlock(fd, unix.F_SETLK, &lock)
}

func hmc275OpenVFS(ctx context.Context, path string) (_ *hmc275WALVFS, finalErr error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, fmt.Errorf("prototype requires capture deadline")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if os.Getenv("CODRAX_HMC275_WAL_HELPER") != "1" {
		return nil, fmt.Errorf("prototype requires isolated helper process")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return nil, fmt.Errorf("prototype refuses alias: %v", err)
	}
	if _, err := os.Lstat(path + "-journal"); !os.IsNotExist(err) {
		return nil, fmt.Errorf("journal namespace not clean")
	}
	v := &hmc275WALVFS{ctx: ctx, mappings: make(map[int32][]byte)}
	defer func() {
		if finalErr != nil {
			v.close()
		}
	}()
	for i, suffix := range []string{"", "-wal", "-shm"} {
		fd, err := unix.Open(path+suffix, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return nil, err
		}
		v.files[i] = os.NewFile(uintptr(fd), path+suffix)
		info, err := v.files[i].Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return nil, fmt.Errorf("invalid or oversized source %d: %v", i, err)
		}
		v.identities[i] = info
		current, err := os.Lstat(path + suffix)
		if err != nil || !os.SameFile(info, current) {
			return nil, fmt.Errorf("source object changed at admission")
		}
	}
	var header [20]byte
	if _, err := v.files[0].ReadAt(header[:], 0); err != nil || !bytes.Equal(header[:16], []byte("SQLite format 3\x00")) || header[18] != 2 || header[19] != 2 {
		return nil, fmt.Errorf("prototype requires actual WAL-format SQLite")
	}
	// A live external owner must already hold the SHM dead-man switch. Never
	// initialize, recover, truncate, chown, or create a shared-memory file.
	dms := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 128, Len: 1}
	if err := unix.FcntlFlock(v.files[2].Fd(), unix.F_GETLK, &dms); err != nil || dms.Type == unix.F_UNLCK {
		return nil, fmt.Errorf("no live external SHM owner: %v", err)
	}
	if err := hmc275Lock(v.files[2].Fd(), unix.F_RDLCK, 128, 1); err != nil {
		return nil, err
	}
	if err := hmc275Lock(v.files[0].Fd(), unix.F_RDLCK, 1073741824, 1); err != nil {
		return nil, err
	}
	if err := hmc275Lock(v.files[0].Fd(), unix.F_RDLCK, 1073741826, 510); err != nil {
		return nil, err
	}
	_ = hmc275Lock(v.files[0].Fd(), unix.F_UNLCK, 1073741824, 1)
	// Check the whole admitted tuple again after holding protocol locks. This
	// narrows ordinary namespace races; it does not claim atomic multi-file
	// namespace acquisition or protection against malicious ABA replacement.
	for i, suffix := range []string{"", "-wal", "-shm"} {
		current, err := os.Lstat(path + suffix)
		if err != nil || !os.SameFile(v.identities[i], current) {
			return nil, fmt.Errorf("source tuple changed at locked admission")
		}
	}
	dms = unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 128, Len: 1}
	if err := unix.FcntlFlock(v.files[2].Fd(), unix.F_GETLK, &dms); err != nil || dms.Type == unix.F_UNLCK {
		return nil, fmt.Errorf("external SHM owner left during admission: %v", err)
	}
	v.tls = libc.NewTLS()
	hmc275VFSRegistry.Lock()
	hmc275VFSRegistry.next++
	v.token = hmc275VFSRegistry.next
	hmc275VFSRegistry.items[v.token] = v
	hmc275VFSRegistry.Unlock()
	v.name = fmt.Sprintf("hmc275-ro-%d", v.token)
	v.cname, err = libc.CString(v.name)
	if err != nil {
		return nil, err
	}
	v.cvfs = sqlite3.Xsqlite3_malloc64(v.tls, uint64(unsafe.Sizeof(sqlite3.Tsqlite3_vfs{})))
	v.methods = sqlite3.Xsqlite3_malloc64(v.tls, uint64(unsafe.Sizeof(sqlite3.Tsqlite3_io_methods{})))
	if v.cvfs == 0 || v.methods == 0 {
		return nil, fmt.Errorf("VFS allocation failed")
	}
	base := sqlite3.Xsqlite3_vfs_find(v.tls, 0)
	if base == 0 {
		return nil, fmt.Errorf("default VFS unavailable")
	}
	b := *(*sqlite3.Tsqlite3_vfs)(unsafe.Pointer(base))
	*(*sqlite3.Tsqlite3_vfs)(unsafe.Pointer(v.cvfs)) = sqlite3.Tsqlite3_vfs{
		FiVersion: 2, FszOsFile: int32(unsafe.Sizeof(hmc275VFSFile{})), FmxPathname: 128, FzName: v.cname, FpAppData: v.token,
		FxOpen: hmc275FP(hmc275VFSOpen), FxDelete: hmc275FP(hmc275VFSDelete), FxAccess: hmc275FP(hmc275VFSAccess), FxFullPathname: hmc275FP(hmc275VFSFullPath),
		FxRandomness: b.FxRandomness, FxSleep: b.FxSleep, FxCurrentTime: b.FxCurrentTime, FxCurrentTimeInt64: b.FxCurrentTimeInt64,
	}
	*(*sqlite3.Tsqlite3_io_methods)(unsafe.Pointer(v.methods)) = sqlite3.Tsqlite3_io_methods{
		FiVersion: 2, FxClose: hmc275FP(hmc275VFSClose), FxRead: hmc275FP(hmc275VFSRead), FxWrite: hmc275FP(hmc275VFSWrite), FxTruncate: hmc275FP(hmc275VFSTruncate), FxSync: hmc275FP(hmc275VFSSync), FxFileSize: hmc275FP(hmc275VFSSize),
		FxLock: hmc275FP(hmc275VFSLock), FxUnlock: hmc275FP(hmc275VFSUnlock), FxCheckReservedLock: hmc275FP(hmc275VFSReserved), FxFileControl: hmc275FP(hmc275VFSControl), FxSectorSize: hmc275FP(hmc275VFSSector), FxDeviceCharacteristics: hmc275FP(hmc275VFSCharacteristics),
		FxShmMap: hmc275FP(hmc275VFSShmMap), FxShmLock: hmc275FP(hmc275VFSShmLock), FxShmBarrier: hmc275FP(hmc275VFSShmBarrier), FxShmUnmap: hmc275FP(hmc275VFSShmUnmap),
	}
	if rc := sqlite3.Xsqlite3_vfs_register(v.tls, v.cvfs, 0); rc != sqlite3.SQLITE_OK {
		return nil, fmt.Errorf("register: %d", rc)
	}
	v.registered = true
	return v, nil
}

func (v *hmc275WALVFS) close() error {
	if v == nil {
		return nil
	}
	var closeErr error
	if v.cvfs != 0 {
		if v.registered {
			sqlite3.Xsqlite3_vfs_unregister(v.tls, v.cvfs)
		}
		sqlite3.Xsqlite3_free(v.tls, v.cvfs)
	}
	if v.methods != 0 {
		sqlite3.Xsqlite3_free(v.tls, v.methods)
	}
	if v.cname != 0 {
		libc.Xfree(v.tls, v.cname)
	}
	// An external writer can leave dirty SHM pages behind. On the tested Darwin
	// filesystem, the last munmap (even PROT_READ) can change ctime. This is a
	// terminal rejection, not a claim that read-only mappings have no effects.
	// Compare full held-FD generations while it is still possible to do so;
	// neither reopen by pathname nor weaken SameVersion to bytes/mtime alone.
	if len(v.mappings) > 0 {
		var err error
		v.closeSHMBefore, err = filegeneration.FromFile(v.files[2])
		if err != nil {
			closeErr = fmt.Errorf("observe source before mapping release: %w", err)
		}
		for _, m := range v.mappings {
			if err := unix.Munmap(m); err != nil && closeErr == nil {
				closeErr = fmt.Errorf("release source mapping: %w", err)
			}
		}
		v.closeSHMAfter, err = filegeneration.FromFile(v.files[2])
		if err != nil {
			closeErr = fmt.Errorf("observe source after mapping release: %w", err)
		} else if !v.closeSHMBefore.SameVersion(v.closeSHMAfter) {
			v.closeSHMChanged = true
			closeErr = fmt.Errorf("source generation changed while releasing shared-memory mapping; active-WAL zero-effect capture is unsupported")
		}
	}
	for _, f := range v.files {
		if f != nil {
			_ = f.Close()
		}
	}
	if v.tls != nil {
		v.tls.Close()
	}
	hmc275VFSRegistry.Lock()
	delete(hmc275VFSRegistry.items, v.token)
	hmc275VFSRegistry.Unlock()
	return closeErr
}

func hmc275VFSOpen(_ *libc.TLS, p, name, file uintptr, flags int32, out uintptr) int32 {
	v := hmc275State((*sqlite3.Tsqlite3_vfs)(unsafe.Pointer(p)).FpAppData)
	role := int32(-1)
	switch libc.GoString(name) {
	case "capture":
		role = 0
	case "capture-wal":
		role = 1
	}
	if role < 0 || v.ctx.Err() != nil {
		return sqlite3.SQLITE_CANTOPEN
	}
	*(*hmc275VFSFile)(unsafe.Pointer(file)) = hmc275VFSFile{methods: v.methods, token: v.token, role: role}
	if out != 0 {
		*(*int32)(unsafe.Pointer(out)) = sqlite3.SQLITE_OPEN_READONLY
	}
	return sqlite3.SQLITE_OK
}
func hmc275VFSDelete(_ *libc.TLS, p, name uintptr, syncDir int32) int32 {
	hmc275State((*sqlite3.Tsqlite3_vfs)(unsafe.Pointer(p)).FpAppData).deniedWrites++
	return sqlite3.SQLITE_READONLY
}
func hmc275VFSAccess(_ *libc.TLS, p, name uintptr, flags int32, out uintptr) int32 {
	*(*int32)(unsafe.Pointer(out)) = 0
	if flags != sqlite3.SQLITE_ACCESS_READWRITE && (libc.GoString(name) == "capture" || libc.GoString(name) == "capture-wal") {
		*(*int32)(unsafe.Pointer(out)) = 1
	}
	return sqlite3.SQLITE_OK
}
func hmc275VFSFullPath(_ *libc.TLS, p, name uintptr, n int32, out uintptr) int32 {
	s := libc.GoString(name)
	if len(s)+1 > int(n) {
		return sqlite3.SQLITE_CANTOPEN
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(out)), int(n))
	copy(b, s)
	b[len(s)] = 0
	return sqlite3.SQLITE_OK
}
func hmc275VFSClose(_ *libc.TLS, p uintptr) int32 { return sqlite3.SQLITE_OK }
func hmc275VFSRead(_ *libc.TLS, p, out uintptr, n int32, offset int64) int32 {
	v, role := hmc275FromFile(p)
	if v.ctx.Err() != nil || n < 0 || offset < 0 || offset+int64(n) > 16<<20 {
		return sqlite3.SQLITE_IOERR_READ
	}
	v.reads++
	b := unsafe.Slice((*byte)(unsafe.Pointer(out)), int(n))
	got, err := v.files[role].ReadAt(b, offset)
	if err == nil {
		return sqlite3.SQLITE_OK
	}
	if err == io.EOF {
		clear(b[got:])
		return sqlite3.SQLITE_IOERR_SHORT_READ
	}
	return sqlite3.SQLITE_IOERR_READ
}
func hmc275VFSWrite(_ *libc.TLS, p, b uintptr, n int32, o int64) int32 {
	v, _ := hmc275FromFile(p)
	v.deniedWrites++
	return sqlite3.SQLITE_READONLY
}
func hmc275VFSTruncate(_ *libc.TLS, p uintptr, n int64) int32 {
	v, _ := hmc275FromFile(p)
	v.deniedWrites++
	return sqlite3.SQLITE_READONLY
}
func hmc275VFSSync(_ *libc.TLS, p uintptr, flags int32) int32 { return sqlite3.SQLITE_READONLY }
func hmc275VFSSize(_ *libc.TLS, p, out uintptr) int32 {
	v, role := hmc275FromFile(p)
	i, e := v.files[role].Stat()
	if e != nil {
		return sqlite3.SQLITE_IOERR_FSTAT
	}
	*(*int64)(unsafe.Pointer(out)) = i.Size()
	return sqlite3.SQLITE_OK
}
func hmc275VFSLock(_ *libc.TLS, p uintptr, level int32) int32 {
	if level > sqlite3.SQLITE_LOCK_SHARED {
		return sqlite3.SQLITE_READONLY
	}
	return sqlite3.SQLITE_OK
}

// The helper holds the main shared lock for the entire capture, a stronger
// lifetime than SQLite's read transaction. Closing its descriptor releases it.
func hmc275VFSUnlock(_ *libc.TLS, p uintptr, level int32) int32 { return sqlite3.SQLITE_OK }
func hmc275VFSReserved(_ *libc.TLS, p, out uintptr) int32 {
	*(*int32)(unsafe.Pointer(out)) = 0
	return sqlite3.SQLITE_OK
}
func hmc275VFSControl(_ *libc.TLS, p uintptr, op int32, arg uintptr) int32 {
	return sqlite3.SQLITE_NOTFOUND
}
func hmc275VFSSector(_ *libc.TLS, p uintptr) int32          { return 4096 }
func hmc275VFSCharacteristics(_ *libc.TLS, p uintptr) int32 { return 0 }
func hmc275VFSShmMap(_ *libc.TLS, p uintptr, region, size, extend int32, out uintptr) int32 {
	v, _ := hmc275FromFile(p)
	*(*uintptr)(unsafe.Pointer(out)) = 0
	if region < 0 || region > 255 || size != 32768 || v.ctx.Err() != nil {
		return sqlite3.SQLITE_IOERR_SHMMAP
	}
	m := v.mappings[region]
	if m == nil {
		info, err := v.files[2].Stat()
		if err != nil || info.Size() < int64(region+1)*int64(size) {
			return sqlite3.SQLITE_READONLY_CANTINIT
		}
		m, err = unix.Mmap(int(v.files[2].Fd()), int64(region)*int64(size), int(size), unix.PROT_READ, unix.MAP_SHARED)
		if err != nil {
			return sqlite3.SQLITE_IOERR_SHMMAP
		}
		v.mappings[region] = m
	}
	*(*uintptr)(unsafe.Pointer(out)) = uintptr(unsafe.Pointer(&m[0]))
	return sqlite3.SQLITE_READONLY
}
func hmc275VFSShmLock(_ *libc.TLS, p uintptr, offset, n, flags int32) int32 {
	v, _ := hmc275FromFile(p)
	if offset < 0 || n < 1 || offset+n > 8 {
		return sqlite3.SQLITE_IOERR_SHMLOCK
	}
	if flags&sqlite3.SQLITE_SHM_EXCLUSIVE != 0 {
		return sqlite3.SQLITE_BUSY
	}
	kind := int16(unix.F_RDLCK)
	if flags&sqlite3.SQLITE_SHM_UNLOCK != 0 {
		kind = unix.F_UNLCK
	}
	if err := hmc275Lock(v.files[2].Fd(), kind, 120+int64(offset), int64(n)); err != nil {
		return sqlite3.SQLITE_BUSY
	}
	v.shmLocks++
	return sqlite3.SQLITE_OK
}

var hmc275Barrier atomic.Uint32

func hmc275VFSShmBarrier(_ *libc.TLS, p uintptr)                       { hmc275Barrier.Add(0) }
func hmc275VFSShmUnmap(_ *libc.TLS, p uintptr, deleteFlag int32) int32 { return sqlite3.SQLITE_OK }

// The private output remains unreturned until both backup DONE and Finish.
// On any error/cancellation only this function's fresh temporary directory is
// removed. Existing public preparation/SameVersion code is not involved.
func hmc275Capture(ctx context.Context, source, outputParent string, pinned func(*hmc275WALVFS) error, stepped func(int)) (result string, finalErr error) {
	return hmc275CaptureAtAdmission(ctx, source, outputParent, nil, pinned, stepped)
}

// The admission hook is test-only synchronization, after held-source protocol
// locks and tuple checks but before SQLite establishes its read transaction.
func hmc275CaptureAtAdmission(ctx context.Context, source, outputParent string, admitted, pinned func(*hmc275WALVFS) error, stepped func(int)) (result string, finalErr error) {
	v, err := hmc275OpenVFS(ctx, source)
	if err != nil {
		return "", err
	}
	var dir string
	defer func() {
		// Finish lifecycle checks before publishing or deleting the private
		// directory. A late close failure must not leak a successful path.
		if closeErr := v.close(); closeErr != nil {
			if finalErr == nil {
				finalErr = closeErr
			} else {
				finalErr = fmt.Errorf("%w; additionally: %v", finalErr, closeErr)
			}
		}
		if finalErr == nil {
			finalErr = ctx.Err()
		}
		if finalErr != nil {
			result = ""
			if dir != "" {
				_ = os.RemoveAll(dir)
			}
		}
	}()
	if admitted != nil {
		if err = admitted(v); err != nil {
			return "", err
		}
	}
	dir, err = os.MkdirTemp(outputParent, "hmc275-private-")
	if err != nil {
		return "", err
	}
	db, err := sql.Open("sqlite", "file:capture?mode=ro&vfs="+v.name)
	if err != nil {
		return "", err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN"); err != nil {
		return "", err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var count int
	if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&count); err != nil {
		return "", err
	}
	if pinned != nil {
		if err = pinned(v); err != nil {
			return "", err
		}
	}
	dst := filepath.Join(dir, "snapshot.db")
	err = conn.Raw(func(raw any) error {
		b, err := raw.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		}).NewBackup(dst)
		if err != nil {
			return err
		}
		finished := false
		defer func() {
			if !finished {
				_ = b.Finish()
			}
		}()
		for steps := 0; steps < 4096; steps++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, err := b.Step(1)
			if err != nil {
				return err
			}
			if b.PageCount() > 4096 {
				return fmt.Errorf("snapshot page budget exceeded")
			}
			if stepped != nil {
				stepped(steps + 1)
			}
			if !more {
				err = b.Finish()
				finished = true
				return err
			}
		}
		return fmt.Errorf("snapshot step budget exceeded")
	})
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return dst, nil
}
