//go:build !darwin && !linux

package tracecatalog

import "fmt"

// Do not silently downgrade to replace-capable rename on another platform.
func exportRenameNoReplace(_ uintptr, _, _ string) error {
	return fmt.Errorf("atomic no-replace evidence package publication is not supported on this platform")
}
