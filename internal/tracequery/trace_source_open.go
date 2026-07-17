package tracequery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func traceSourcePathIsBlockingNamespace(path string) bool {
	return runtime.GOOS == "windows" && filegeneration.IsWindowsNamedPipePath(path)
}

// openTraceSourceRegular opens a trace source without allowing a pathname
// swap to a FIFO/named pipe to block the caller, then binds the returned
// descriptor to the strongest identity available on this platform. Callers
// own the file only on success.
func openTraceSourceRegular(path string) (*os.File, traceFileIdentity, error) {
	return openTraceSourceRegularContext(context.Background(), path)
}

func openTraceSourceRegularContext(ctx context.Context, path string) (*os.File, traceFileIdentity, error) {
	return openTraceSourceRegularContextPolicy(ctx, path, true)
}

func openTraceSourceRegularContextPolicy(ctx context.Context, path string, allowColdAdmission bool) (*os.File, traceFileIdentity, error) {
	file, err := openTraceSourcePath(path)
	if err != nil {
		return nil, traceFileIdentity{}, err
	}
	identity, identityErr := traceFileIdentityFromFile(file)
	if identityErr == nil && (!identity.Mode().IsRegular() || identity.Size() < 0) {
		identityErr = fmt.Errorf("trace source is not a regular file: %s", path)
	}
	if identityErr != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, traceFileIdentity{}, fmt.Errorf("inspect trace source %s: %v; close: %w", path, identityErr, closeErr)
		}
		return nil, traceFileIdentity{}, identityErr
	}
	if admissionErr := validateHeldTraceInput(ctx, file, identity, path, allowColdAdmission); admissionErr != nil {
		if closeErr := file.Close(); closeErr != nil {
			return nil, traceFileIdentity{}, errors.Join(admissionErr, fmt.Errorf("close rejected trace source %s: %w", path, closeErr))
		}
		return nil, traceFileIdentity{}, admissionErr
	}
	// Path-based readers own both authorities: the content verdict above is
	// bound to the held descriptor, then this existing strong check proves the
	// pathname still names that same generation. Held-only callers intentionally
	// skip this second half.
	if bindingErr := validateTraceFileIdentityAfterRead(file, identity, "trace input admission path binding"); bindingErr != nil {
		if closeErr := file.Close(); closeErr != nil {
			return nil, traceFileIdentity{}, errors.Join(bindingErr, fmt.Errorf("close unbound trace source %s: %w", path, closeErr))
		}
		return nil, traceFileIdentity{}, bindingErr
	}
	return file, identity, nil
}

// frozenTraceSectionAtCurrentOffset caps every parser read at the size that
// received text admission. Concurrent appends therefore remain outside the
// parser even though the final generation check will still fail the run.
func frozenTraceSectionAtCurrentOffset(file *os.File, identity traceFileIdentity) (*io.SectionReader, error) {
	if file == nil || !identity.Initialized() || identity.Size() < 0 {
		return nil, fmt.Errorf("trace frozen reader has no valid held generation")
	}
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("trace frozen reader: inspect current offset: %w", err)
	}
	if offset < 0 || offset > identity.Size() {
		return nil, fmt.Errorf("trace frozen reader offset %d is outside admitted size %d", offset, identity.Size())
	}
	return io.NewSectionReader(file, offset, identity.Size()-offset), nil
}

func traceReadErrorAfterIdentity(file *os.File, identity traceFileIdentity, operation string, readErr error) error {
	if readErr == nil {
		return nil
	}
	if identityErr := validateTraceFileIdentityAfterRead(file, identity, operation); identityErr != nil {
		return errors.Join(identityErr, readErr)
	}
	return readErr
}
