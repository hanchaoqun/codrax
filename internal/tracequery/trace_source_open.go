package tracequery

import (
	"fmt"
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
	return file, identity, nil
}
