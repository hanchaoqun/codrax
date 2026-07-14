package hitraceconv

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
	"time"
)

const sealedTraceDBVirtualName = "trace_streamer_export.db"

// sealedTraceDBFS is an exact-one-file read-only filesystem for modernc's
// SQLite VFS. Every Open receives an independent seek cursor over the same
// held, fixed-size conversion child; no operation reopens the staging path.
type sealedTraceDBFS struct {
	sealed *sealedConversionFile
	size   int64
}

func newSealedTraceDBFS(sealed *sealedConversionFile) (*sealedTraceDBFS, error) {
	if sealed == nil {
		return nil, fmt.Errorf("sealed trace DB authority is nil")
	}
	if sealed.Size() <= 0 {
		return nil, fmt.Errorf("sealed trace DB is empty")
	}
	return &sealedTraceDBFS{sealed: sealed, size: sealed.Size()}, nil
}

func (filesystem *sealedTraceDBFS) Open(name string) (fs.File, error) {
	if filesystem == nil || filesystem.sealed == nil || filesystem.size <= 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: os.ErrInvalid}
	}
	if name != sealedTraceDBVirtualName {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &sealedTraceDBVFSFile{
		reader: io.NewSectionReader(filesystem.sealed, 0, filesystem.size),
		info: sealedTraceDBFileInfo{
			name: sealedTraceDBVirtualName,
			size: filesystem.size,
		},
	}, nil
}

type sealedTraceDBVFSFile struct {
	mu     sync.Mutex
	reader *io.SectionReader
	info   sealedTraceDBFileInfo
	closed bool
}

func (file *sealedTraceDBVFSFile) Read(buffer []byte) (int, error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.closed || file.reader == nil {
		return 0, os.ErrClosed
	}
	return file.reader.Read(buffer)
}

func (file *sealedTraceDBVFSFile) Seek(offset int64, whence int) (int64, error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.closed || file.reader == nil {
		return 0, os.ErrClosed
	}
	return file.reader.Seek(offset, whence)
}

func (file *sealedTraceDBVFSFile) Stat() (fs.FileInfo, error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.closed || file.reader == nil {
		return nil, os.ErrClosed
	}
	return file.info, nil
}

func (file *sealedTraceDBVFSFile) Close() error {
	if file == nil {
		return nil
	}
	file.mu.Lock()
	defer file.mu.Unlock()
	file.closed = true
	file.reader = nil
	return nil
}

type sealedTraceDBFileInfo struct {
	name string
	size int64
}

func (info sealedTraceDBFileInfo) Name() string  { return info.name }
func (info sealedTraceDBFileInfo) Size() int64   { return info.size }
func (sealedTraceDBFileInfo) Mode() fs.FileMode  { return 0o400 }
func (sealedTraceDBFileInfo) ModTime() time.Time { return time.Time{} }
func (sealedTraceDBFileInfo) IsDir() bool        { return false }
func (sealedTraceDBFileInfo) Sys() any           { return nil }
