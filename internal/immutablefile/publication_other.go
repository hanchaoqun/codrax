//go:build !darwin && !linux && !windows

package immutablefile

import (
	"fmt"
	"os"
)

const PublicationSupported = false
const ReadFlags = 0

func RenameNoReplace(_ *os.File, _, _, _ string) error {
	return fmt.Errorf("atomic no-replace result publication unsupported on this platform")
}
