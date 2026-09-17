//go:build darwin

package tool

import "fmt"

// darwinProcessGroupMember describes only a published process visible through
// KERN_PROC_PGRP. State is Darwin's native p_stat; 5 denotes a zombie.
//
// This is NOT a complete process-tree census. XNU's proc_iterate skips SIDL,
// and a P_REF_NEW process can join the group before entering allproc. Neither
// an empty snapshot nor one containing only a guardian/zombies proves that
// an in-flight fork or a descendant with closed stdio has been terminated.
// Callers must retain their independent PGID ownership lease while signaling.
type darwinProcessGroupMember struct {
	PID   int
	State int
}

func validateDarwinPublishedGroupSnapshotSize(capacity, used, recordSize uint64) error {
	if recordSize == 0 || used > capacity || used%recordSize != 0 {
		return fmt.Errorf("Darwin published group snapshot has invalid/truncated size: capacity=%d used=%d record_size=%d", capacity, used, recordSize)
	}
	return nil
}
