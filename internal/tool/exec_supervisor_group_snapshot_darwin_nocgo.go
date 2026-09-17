//go:build darwin && !cgo

package tool

import "errors"

func snapshotDarwinPublishedProcessGroup(int) ([]darwinProcessGroupMember, error) {
	return nil, errors.New("Darwin published group snapshot is unsupported without cgo native ABI access")
}
