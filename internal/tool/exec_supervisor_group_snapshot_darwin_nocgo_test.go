//go:build darwin && !cgo

package tool

import "testing"

func TestB1715DarwinPublishedGroupSnapshotNoCGOIsUnavailable(t *testing.T) {
	members, err := snapshotDarwinPublishedProcessGroup(1)
	if err == nil || members != nil {
		t.Fatalf("unsupported snapshot became a successful empty observation: members=%+v err=%v", members, err)
	}
}
