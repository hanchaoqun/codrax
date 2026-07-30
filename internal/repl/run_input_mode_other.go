//go:build !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !linux

package repl

import "errors"

// makeRunInputMode: no cbreak support on this platform — the run
// window is not armed and the pre-window behavior (no reader during
// runs) is preserved. Disclosed in the design ledger.
func makeRunInputMode(fd int) (func(), error) {
	return nil, errors.New("run input window unsupported on this platform")
}
