package hitraceconv

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

var (
	errSealedConversionFileNotRegular          = errors.New("sealed conversion file is not regular")
	errSealedConversionFileEmpty               = errors.New("sealed conversion file is empty")
	errSealedConversionFileIdentityChanged     = errors.New("sealed conversion file identity changed")
	errSealedConversionFileIdentityUnavailable = errors.New("sealed conversion file strong identity unavailable")
)

// sealedConversionFile is a held, generation-bound child output produced by
// an external conversion tool inside a privateConversionDir. Parsers consume
// this handle directly; the child path is retained only for parent-relative
// binding validation and cleanup.
type sealedConversionFile struct {
	mu       sync.RWMutex
	dir      *privateConversionDir
	name     string
	file     *os.File
	identity filegeneration.Identity
	size     int64
	closed   bool
}

func (dir *privateConversionDir) AdoptRegularChild(name string, requireNonEmpty bool) (*sealedConversionFile, error) {
	sealed, found, err := dir.TryAdoptRegularChild(name, requireNonEmpty)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("adopt private conversion child %q: %w", name, os.ErrNotExist)
	}
	return sealed, nil
}

func (dir *privateConversionDir) TryAdoptRegularChild(name string, requireNonEmpty bool) (*sealedConversionFile, bool, error) {
	if dir == nil {
		return nil, false, fmt.Errorf("private conversion directory authority is missing")
	}
	if _, err := dir.ChildPath(name); err != nil {
		return nil, false, err
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal {
		return nil, false, traceDBJoinPreservingSingle(
			fmt.Errorf("private conversion directory authority is terminal: %s", dir.path), dir.terminalErr,
		)
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		return nil, false, err
	}
	if err := validatePrivateConversionDirSecurityPlatform(dir.path, dir.identity, &dir.platform); err != nil {
		return nil, false, fmt.Errorf("%w: path=%s: %v", errPrivateConversionDirSecurityInvalid, dir.path, err)
	}
	file, info, err := adoptPrivateConversionRegularChildPlatform(&dir.platform, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("adopt private conversion child %q: %w", name, err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()
	fail := func(primary error) (*sealedConversionFile, bool, error) {
		closeOnError = false
		return nil, true, traceDBJoinPreservingSingle(primary, file.Close())
	}
	identity, err := filegeneration.FromFile(file)
	if err != nil {
		return fail(fmt.Errorf("capture sealed conversion child identity %q: %w", name, err))
	}
	if !identity.Strong() {
		return fail(fmt.Errorf("%w: %s", errSealedConversionFileIdentityUnavailable, name))
	}
	if !identity.Mode().IsRegular() {
		return fail(fmt.Errorf("%w: %s", errSealedConversionFileNotRegular, name))
	}
	if err := validatePrivateConversionRegularChildPlatform(&dir.platform, name, file, info); err != nil {
		return fail(fmt.Errorf("bind sealed conversion child %q: %w", name, err))
	}
	confirmed, err := filegeneration.FromFile(file)
	if err != nil {
		return fail(fmt.Errorf("confirm sealed conversion child identity %q: %w", name, err))
	}
	if !confirmed.Strong() {
		return fail(fmt.Errorf("%w: %s", errSealedConversionFileIdentityUnavailable, name))
	}
	if !identity.SameVersion(confirmed) {
		return fail(fmt.Errorf("%w: %s", errSealedConversionFileIdentityChanged, name))
	}
	if !confirmed.Mode().IsRegular() {
		return fail(fmt.Errorf("%w: %s", errSealedConversionFileNotRegular, name))
	}
	if requireNonEmpty && confirmed.Size() <= 0 {
		return fail(fmt.Errorf("%w: %s", errSealedConversionFileEmpty, name))
	}
	closeOnError = false
	return &sealedConversionFile{
		dir: dir, name: name, file: file, identity: confirmed, size: confirmed.Size(),
	}, true, nil
}

func (sealed *sealedConversionFile) Size() int64 {
	if sealed == nil {
		return 0
	}
	return sealed.size
}

func (sealed *sealedConversionFile) ReadAt(buffer []byte, offset int64) (int, error) {
	if sealed == nil {
		return 0, fmt.Errorf("sealed conversion file is nil")
	}
	sealed.mu.RLock()
	defer sealed.mu.RUnlock()
	if sealed.closed || sealed.file == nil {
		return 0, fmt.Errorf("sealed conversion file is closed: %s", sealed.name)
	}
	return sealed.file.ReadAt(buffer, offset)
}

func (sealed *sealedConversionFile) Reader() io.Reader {
	if sealed == nil {
		return io.NewSectionReader(nilReaderAt{}, 0, 0)
	}
	return io.NewSectionReader(sealed, 0, sealed.size)
}

func (sealed *sealedConversionFile) Validate() error {
	if sealed == nil {
		return fmt.Errorf("sealed conversion file is nil")
	}
	sealed.mu.RLock()
	defer sealed.mu.RUnlock()
	if sealed.closed || sealed.file == nil || sealed.dir == nil || !sealed.identity.Initialized() {
		return fmt.Errorf("sealed conversion file is closed or incomplete: %s", sealed.name)
	}
	sealed.dir.mu.Lock()
	defer sealed.dir.mu.Unlock()
	if sealed.dir.terminal {
		return traceDBJoinPreservingSingle(
			fmt.Errorf("private conversion directory authority is terminal: %s", sealed.dir.path), sealed.dir.terminalErr,
		)
	}
	if err := sealed.dir.validateIdentityLocked(true); err != nil {
		return err
	}
	if err := validatePrivateConversionDirSecurityPlatform(sealed.dir.path, sealed.dir.identity, &sealed.dir.platform); err != nil {
		return fmt.Errorf("%w: path=%s: %v", errPrivateConversionDirSecurityInvalid, sealed.dir.path, err)
	}
	current, err := filegeneration.FromFile(sealed.file)
	if err != nil || !current.Strong() || !sealed.identity.SameVersion(current) || current.Size() != sealed.size {
		return fmt.Errorf("%w: %s", errSealedConversionFileIdentityChanged, sealed.name)
	}
	info, err := sealed.file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != sealed.size {
		return fmt.Errorf("%w: %s", errSealedConversionFileIdentityChanged, sealed.name)
	}
	if err := validatePrivateConversionRegularChildPlatform(&sealed.dir.platform, sealed.name, sealed.file, info); err != nil {
		return fmt.Errorf("%w: %s: %v", errSealedConversionFileIdentityChanged, sealed.name, err)
	}
	return nil
}

func (sealed *sealedConversionFile) Close() error {
	if sealed == nil {
		return nil
	}
	sealed.mu.Lock()
	defer sealed.mu.Unlock()
	if sealed.closed {
		return nil
	}
	sealed.closed = true
	if sealed.file == nil {
		return nil
	}
	err := sealed.file.Close()
	sealed.file = nil
	return err
}

func finishSealedConversionFile(sealed *sealedConversionFile, operationErr error) error {
	if sealed == nil {
		return traceDBJoinPreservingSingle(fmt.Errorf("sealed conversion file is nil"), operationErr)
	}
	validationErr := sealed.Validate()
	closeErr := sealed.Close()
	return traceDBJoinPreservingSingle(validationErr, operationErr, closeErr)
}

// nilReaderAt only supplies a well-defined empty reader for a nil sealed
// value; production callers reject nil before requesting Reader.
type nilReaderAt struct{}

func (nilReaderAt) ReadAt([]byte, int64) (int, error) { return 0, io.EOF }
