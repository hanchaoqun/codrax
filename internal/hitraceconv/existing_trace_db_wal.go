package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// ExistingTraceDBWAL records the other physical input, not a second trace.
// Main-file provenance remains in ExistingTraceDBSource; Snapshot* measures
// the private committed image actually consumed by the sealed SQLite reader.
type ExistingTraceDBWAL struct {
	Path           string
	Bytes          int64
	SHA256         string
	Generation     string
	CommitFrame    int64
	CheckpointOnly bool
	SnapshotBytes  int64
	SnapshotSHA256 string
}

const maxExistingWALPages = 1 << 20
const maxExistingWALBytes int64 = 4 << 30

// A stable-generation WAL reader follows SQLite's documented page overlay
// algorithm, without opening the source in SQLite (including its shared
// memory). It accepts a writer that is idle during preparation, not concurrent
// changing input. Every read and publication rechecks both file generations.
// https://www.sqlite.org/fileformat2.html#wal_file_format
type existingWALView struct {
	main, wal      *conversionInputAuthority
	pageSize, size int64
	pages          map[uint32]int64
	header         [100]byte
	receipt        ExistingTraceDBWAL
}

func existingWALNamespace(main *conversionInputAuthority) error {
	for _, base := range uniqueNonEmptyStrings([]string{main.requestedPath, main.CanonicalPath()}) {
		for _, suffix := range []string{"-journal", "-wal", "-shm"} {
			info, err := os.Lstat(base + suffix)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			candidate, canonicalErr := canonicalTracePath(base + suffix)
			if canonicalErr != nil {
				return canonicalErr
			}
			if suffix == "-journal" || candidate.path != main.CanonicalPath()+suffix || !info.Mode().IsRegular() {
				return fmt.Errorf("%w: unsupported or ambiguous SQLite auxiliary %s", errTraceStreamerDBAuxiliaryState, base+suffix)
			}
		}
	}
	return nil
}

func openExistingWALView(ctx context.Context, main *conversionInputAuthority) (view *existingWALView, err error) {
	if err = existingWALNamespace(main); err != nil {
		return nil, err
	}
	wal, err := openConversionInputAuthority(main.CanonicalPath() + "-wal")
	if err != nil {
		return nil, fmt.Errorf("SQLite WAL header mode requires an available WAL: %w", err)
	}
	defer func() {
		if err != nil {
			err = traceDBJoinPreservingSingle(err, wal.Close())
			view = nil
		}
	}()
	view = &existingWALView{main: main, wal: wal, pages: make(map[uint32]int64)}
	if _, err = main.ReadAt(view.header[:], 0); err != nil {
		return nil, err
	}
	if string(view.header[:16]) != "SQLite format 3\x00" || view.header[18] != 2 || view.header[19] != 2 {
		return nil, fmt.Errorf("WAL snapshot requires SQLite WAL read/write mode")
	}
	view.pageSize = int64(binary.BigEndian.Uint16(view.header[16:18]))
	if view.pageSize == 1 {
		view.pageSize = 65536
	}
	if view.pageSize < 512 || view.pageSize > 65536 || view.pageSize&(view.pageSize-1) != 0 || main.Size()%view.pageSize != 0 || main.Size() > maxExistingWALBytes {
		return nil, fmt.Errorf("WAL snapshot has invalid main-file page geometry")
	}
	if wal.Size() == 0 {
		return finishExistingWALView(ctx, view, 0)
	}
	if wal.Size() < 32 || wal.Size() > maxExistingWALBytes {
		return nil, fmt.Errorf("WAL snapshot length outside supported budget")
	}
	var header [32]byte
	if _, err = wal.ReadAt(header[:], 0); err != nil {
		return nil, err
	}
	magic := binary.BigEndian.Uint32(header[:4])
	var order binary.ByteOrder = binary.LittleEndian
	if magic == 0x377f0683 {
		order = binary.BigEndian
	} else if magic != 0x377f0682 {
		return nil, fmt.Errorf("invalid SQLite WAL magic")
	}
	if binary.BigEndian.Uint32(header[4:8]) != 3007000 || int64(binary.BigEndian.Uint32(header[8:12])) != view.pageSize {
		return nil, fmt.Errorf("unsupported WAL version or mismatched page size")
	}
	s0, s1 := existingWALChecksum(order, header[:24], 0, 0)
	if s0 != binary.BigEndian.Uint32(header[24:28]) || s1 != binary.BigEndian.Uint32(header[28:32]) {
		return nil, fmt.Errorf("invalid SQLite WAL header checksum")
	}
	frameSize := view.pageSize + 24
	if (wal.Size()-32)%frameSize != 0 {
		return nil, fmt.Errorf("incomplete WAL frame; retry after capture settles")
	}
	frame := make([]byte, frameSize)
	pending := make(map[uint32]int64)
	commitFrame := int64(0)
	for offset := int64(32); offset < wal.Size(); offset += frameSize {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if _, err = wal.ReadAt(frame, offset); err != nil {
			return nil, err
		}
		// Old-generation tail frames after a WAL reset are not current pages.
		if !bytes.Equal(frame[8:16], header[16:24]) {
			break
		}
		s0, s1 = existingWALChecksum(order, frame[:8], s0, s1)
		s0, s1 = existingWALChecksum(order, frame[24:], s0, s1)
		if s0 != binary.BigEndian.Uint32(frame[16:20]) || s1 != binary.BigEndian.Uint32(frame[20:24]) {
			return nil, fmt.Errorf("invalid SQLite WAL frame checksum; no partial snapshot published")
		}
		page, count := binary.BigEndian.Uint32(frame[:4]), binary.BigEndian.Uint32(frame[4:8])
		if page == 0 || page > 0xfffffffe || count > 0xfffffffe {
			return nil, fmt.Errorf("invalid WAL page number")
		}
		pending[page] = offset + 24
		if len(pending)+len(view.pages) > maxExistingWALPages {
			return nil, fmt.Errorf("WAL page-index budget exceeded")
		}
		if count == 0 {
			continue
		}
		for page, location := range pending {
			view.pages[page] = location
		}
		clear(pending)
		for page := range view.pages {
			if page > count {
				delete(view.pages, page)
			}
		}
		view.size = int64(count) * view.pageSize
		commitFrame = (offset-32)/frameSize + 1
	}
	return finishExistingWALView(ctx, view, commitFrame)
}

// A zero/header-only/uncommitted WAL contributes no committed pages. The
// checkpoint in the held main file is the readable state, not pending frames.
// Both generations remain bound through publication and later cache reuse.
func finishExistingWALView(ctx context.Context, view *existingWALView, commitFrame int64) (*existingWALView, error) {
	main, wal := view.main, view.wal
	if commitFrame == 0 {
		// Require a self-consistent checkpoint size; never fabricate absent pages
		// or use uncommitted page 1 to repair the main header.
		if !bytes.Equal(view.header[24:28], view.header[92:96]) ||
			int64(binary.BigEndian.Uint32(view.header[28:32])) != main.Size()/view.pageSize {
			return nil, fmt.Errorf("WAL has no commit and main checkpoint size is not authoritative")
		}
		view.size = main.Size()
	}
	if view.size > maxExistingWALBytes {
		return nil, fmt.Errorf("committed SQLite snapshot exceeds byte budget")
	}
	// Reject a claimed extension with missing page images before constructing
	// a sparse file or asking SQLite to interpret zero-filled invented pages.
	for page := uint32(main.Size()/view.pageSize) + 1; int64(page) <= view.size/view.pageSize; page++ {
		if _, ok := view.pages[page]; !ok {
			return nil, fmt.Errorf("committed WAL snapshot is missing extended page %d", page)
		}
	}
	if location, ok := view.pages[1]; ok {
		if _, err := wal.ReadAt(view.header[:], location); err != nil {
			return nil, err
		}
	}
	pageSize := int64(binary.BigEndian.Uint16(view.header[16:18]))
	if pageSize == 1 {
		pageSize = 65536
	}
	if string(view.header[:16]) != "SQLite format 3\x00" || pageSize != view.pageSize || view.header[18] != 2 || view.header[19] != 2 {
		return nil, fmt.Errorf("invalid committed SQLite header")
	}
	// Only the private image is made self-contained. Source bytes are never
	// rewritten/checkpointed and source provenance never measures these bytes.
	view.header[18], view.header[19] = 1, 1
	binary.BigEndian.PutUint32(view.header[28:32], uint32(view.size/view.pageSize))
	copy(view.header[92:96], view.header[24:28])
	digest, err := existingDBViewDigest(ctx, wal)
	if err != nil {
		return nil, err
	}
	view.receipt = ExistingTraceDBWAL{Path: wal.DisplayPath(), Bytes: wal.Size(), SHA256: digest, Generation: wal.identity.CacheToken(), CommitFrame: commitFrame, CheckpointOnly: commitFrame == 0, SnapshotBytes: view.size}
	if err := view.Validate(conversionInputStagePreCommit); err != nil {
		return nil, err
	}
	return view, nil
}

func existingWALChecksum(order binary.ByteOrder, data []byte, s0, s1 uint32) (uint32, uint32) {
	for len(data) >= 8 {
		s0 += order.Uint32(data[:4]) + s1
		s1 += order.Uint32(data[4:8]) + s0
		data = data[8:]
	}
	return s0, s1
}

func existingDBViewDigest(ctx context.Context, view conversionInputView) (string, error) {
	hash := sha256.New()
	_, err := copyCancellableRange(ctx, hash, io.NewSectionReader(view, 0, view.Size()), nil)
	if err != nil {
		return "", err
	}
	if err := completeConversionInputStage(ctx, view, conversionInputStagePreCommit, nil); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (v *existingWALView) Size() int64         { return v.size }
func (v *existingWALView) DisplayPath() string { return v.main.DisplayPath() }
func (v *existingWALView) Validate(stage conversionInputStage) error {
	if err := v.main.Validate(stage); err != nil {
		return err
	}
	if err := v.wal.Validate(stage); err != nil {
		return err
	}
	return existingWALNamespace(v.main)
}
func (v *existingWALView) ReadAt(p []byte, off int64) (int, error) {
	if err := v.Validate(conversionInputStageExternalTool); err != nil {
		return 0, err
	}
	if off < 0 {
		return 0, fmt.Errorf("negative WAL snapshot offset")
	}
	n := 0
	for len(p) > 0 && off < v.size {
		length := min(int64(len(p)), v.pageSize-off%v.pageSize, v.size-off)
		reader, location := conversionInputView(v.main), off
		if pageOffset, ok := v.pages[uint32(off/v.pageSize)+1]; ok {
			reader, location = v.wal, pageOffset+off%v.pageSize
		}
		got, err := reader.ReadAt(p[:length], location)
		for i := 0; i < got && off+int64(i) < int64(len(v.header)); i++ {
			p[i] = v.header[off+int64(i)]
		}
		n, off, p = n+got, off+int64(got), p[got:]
		if err != nil {
			return n, err
		}
	}
	if len(p) > 0 {
		return n, io.EOF
	}
	return n, v.Validate(conversionInputStageExternalTool)
}

// ValidateExistingTraceDBReceipt is a reuse check, not snapshot admission.
// A WAL change invalidates prepared material even if the main file's mtime
// did not change. Serialized receipts never bypass generation checks.
func ValidateExistingTraceDBReceipt(ctx context.Context, path string, receipt *ExistingTraceDBSource) (err error) {
	if receipt == nil || receipt.WAL == nil {
		return ValidateExistingTraceDBSource(ctx, path)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	main, err := openConversionInputAuthority(path)
	if err != nil {
		return err
	}
	defer func() { err = traceDBJoinPreservingSingle(err, main.Close()) }()
	if receipt.Path != main.DisplayPath() || receipt.Generation != main.identity.CacheToken() || receipt.Bytes != main.Size() {
		return fmt.Errorf("SQLite main source differs from preparation receipt")
	}
	if err = existingWALNamespace(main); err != nil {
		return err
	}
	if receipt.WAL.Path != main.CanonicalPath()+"-wal" {
		return fmt.Errorf("SQLite WAL locator differs from main source")
	}
	wal, err := openConversionInputAuthority(receipt.WAL.Path)
	if err != nil {
		return err
	}
	defer func() { err = traceDBJoinPreservingSingle(err, wal.Close()) }()
	if receipt.WAL.Generation != wal.identity.CacheToken() || receipt.WAL.Bytes != wal.Size() {
		return fmt.Errorf("SQLite WAL changed after preparation")
	}
	return completeConversionInputStage(ctx, wal, conversionInputStagePreCommit, main.Validate(conversionInputStagePreCommit))
}
