//go:build !unix

package cpulimit

import "errors"

// setThreadNice is unsupported off Unix; Apply degrades to a no-op
// Result and callers fall back to the worker-reservation knob.
func setThreadNice(int) error {
	return errors.New("scheduling niceness unsupported on this platform")
}
