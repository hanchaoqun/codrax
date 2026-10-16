//go:build !darwin && !linux && !windows

package tool

import (
	"fmt"
	"os"
)

const immutableBlobPublicationSupported = false
const immutableBlobReadFlags = 0

func immutableBlobRenameNoReplace(_ *os.File, _, _, _ string) error {
	return fmt.Errorf("atomic no-replace result publication unsupported on this platform")
}
