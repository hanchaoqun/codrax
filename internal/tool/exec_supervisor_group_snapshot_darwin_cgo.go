//go:build darwin && cgo

package tool

/*
#include <errno.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/sysctl.h>
#include <sys/proc.h>

// Read native ABI records rather than hard-coding kinfo_proc offsets. A
// failed/short-capacity read is an error, never an empty group observation.
static int codrax_read_published_group(int pgid, void **records,
    size_t *used, size_t *capacity) {
    int mib[4] = {CTL_KERN, KERN_PROC, KERN_PROC_PGRP, pgid};
    size_t required = 0;
    *records = NULL;
    *used = 0;
    *capacity = 0;
    if (sysctl(mib, 4, NULL, &required, NULL, 0) < 0) {
        return errno;
    }
    if (required > 16 * 1024 * 1024) {
        return E2BIG;
    }
    if (required == 0) {
        return 0;
    }
    void *buffer = calloc(1, required);
    if (buffer == NULL) {
        return ENOMEM;
    }
    size_t actual = required;
    if (sysctl(mib, 4, buffer, &actual, NULL, 0) < 0) {
        int saved_errno = errno;
        free(buffer);
        return saved_errno;
    }
    *records = buffer;
    *used = actual;
    *capacity = required;
    return 0;
}
*/
import "C"

import (
	"fmt"
	"syscall"
	"unsafe"
)

// snapshotDarwinPublishedProcessGroup observes only published, sysctl-visible
// members, including zombies. It must never be used as an all-tree-clean proof.
//
// XNU source authority: kern_sysctl.c sysctl_prochandle uses proc_iterate;
// kern_proc.c proc_iterate skips SIDL and resolves each PID again. forkproc
// enters a new group before pinsertchild publishes the child in allproc.
// https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/kern_sysctl.c
// https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/kern_proc.c
// https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/kern_fork.c
func snapshotDarwinPublishedProcessGroup(pgid int) ([]darwinProcessGroupMember, error) {
	if pgid <= 0 || uint64(pgid) > uint64(1<<31-1) {
		return nil, fmt.Errorf("Darwin published group snapshot requires a positive pid_t PGID: %d", pgid)
	}
	for attempt := 0; attempt < 3; attempt++ {
		var records unsafe.Pointer
		var used, capacity C.size_t
		code := C.codrax_read_published_group(C.int(pgid), &records, &used, &capacity)
		if code != 0 {
			if code == C.ENOMEM && attempt < 2 {
				continue // The table may have grown between sizing and reading.
			}
			return nil, fmt.Errorf("Darwin published group snapshot: %w", syscall.Errno(code))
		}
		defer C.free(records)
		if err := validateDarwinPublishedGroupSnapshotSize(uint64(capacity), uint64(used), uint64(C.sizeof_struct_kinfo_proc)); err != nil {
			return nil, err
		}
		count := int(uint64(used) / uint64(C.sizeof_struct_kinfo_proc))
		if count == 0 {
			return nil, nil // Only an empty published view, not an empty tree.
		}
		if records == nil {
			return nil, fmt.Errorf("Darwin published group snapshot returned records without a buffer")
		}
		rows := unsafe.Slice((*C.struct_kinfo_proc)(records), count)
		members := make([]darwinProcessGroupMember, 0, count)
		for _, row := range rows {
			pid := int(row.kp_proc.p_pid)
			if pid <= 0 {
				return nil, fmt.Errorf("Darwin published group snapshot returned invalid PID %d", pid)
			}
			if int(row.kp_eproc.e_pgid) != pgid {
				return nil, fmt.Errorf("Darwin published group snapshot changed group while reading PID %d", pid)
			}
			members = append(members, darwinProcessGroupMember{PID: pid, State: int(row.kp_proc.p_stat)})
		}
		return members, nil
	}
	return nil, fmt.Errorf("Darwin published group snapshot exhausted bounded retries")
}
