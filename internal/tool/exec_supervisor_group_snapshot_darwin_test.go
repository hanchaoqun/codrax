//go:build darwin && cgo

package tool

import (
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestB1715DarwinPublishedGroupSnapshotNative(t *testing.T) {
	members, err := snapshotDarwinPublishedProcessGroup(syscall.Getpgrp())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, member := range members {
		if member.PID == os.Getpid() && member.State != 5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("published current process missing: %+v", members)
	}
	for _, pgid := range []int{-1, 0} {
		if _, err := snapshotDarwinPublishedProcessGroup(pgid); err == nil {
			t.Errorf("invalid PGID %d became a successful empty snapshot", pgid)
		}
	}
}

// Keep a real direct child unreaped, as the guardian owner must. Observe it
// alive, then as a zombie; neither assertion claims that NEW forks are visible.
func TestB1715DarwinPublishedGroupSnapshotUnreapedChild(t *testing.T) {
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readyRead.Close()
	defer readyWrite.Close()
	holdRead, holdWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer holdRead.Close()
	defer holdWrite.Close()
	command := exec.Command("/bin/sh", "-c", "printf r >&3; read line")
	command.Stdin = holdRead
	command.ExtraFiles = []*os.File{readyWrite}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	_ = readyWrite.Close()
	if err := readyRead.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var ready [1]byte
	if _, err := io.ReadFull(readyRead, ready[:]); err != nil || ready[0] != 'r' {
		t.Fatalf("guardian-shaped child readiness failed: %q %v", ready, err)
	}
	pid := command.Process.Pid
	members, err := snapshotDarwinPublishedProcessGroup(pid)
	if err != nil || len(members) != 1 || members[0].PID != pid || members[0].State == 5 {
		t.Fatalf("live private group snapshot: members=%+v err=%v", members, err)
	}
	if _, err := holdWrite.Write([]byte("exit\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		members, err := snapshotDarwinPublishedProcessGroup(pid)
		if err != nil {
			t.Fatal(err)
		}
		if len(members) == 1 && members[0].PID == pid && members[0].State == 5 {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("unreaped child zombie not visible: %+v", members)
		case <-tick.C:
		}
	}
}

func TestB1715DarwinPublishedGroupSnapshotRejectsTruncatedSizes(t *testing.T) {
	for _, tc := range []struct {
		capacity, used, size uint64
	}{
		{32, 33, 8},
		{32, 7, 8},
		{32, 8, 0},
	} {
		if err := validateDarwinPublishedGroupSnapshotSize(tc.capacity, tc.used, tc.size); err == nil {
			t.Errorf("invalid/truncated size became an empty/successful view: %+v", tc)
		}
	}
	for _, used := range []uint64{0, 8, 32} {
		if err := validateDarwinPublishedGroupSnapshotSize(32, used, 8); err != nil {
			t.Errorf("valid published record size %d rejected: %v", used, err)
		}
	}
}
