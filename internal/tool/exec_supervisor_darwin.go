//go:build darwin

package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

var errDarwinTreeCleanupUnproven = errors.New("supervisor: cleanup incomplete: native published-member snapshots cannot prove termination of unobservable in-flight forks or processes that left the group")

// Darwin's group signal walks a membership snapshot. An in-flight fork can
// therefore survive one signal and retain the target's output pipes. A sibling
// guardian is joined before the target's sole Wait is started. Its unreaped
// process (including zombie state) holds the group identity during retries.
// Published-member snapshots guide best-effort cleanup, never all-tree proof.
func supervisedRunPlatform(ctx context.Context, cmd *exec.Cmd, _ SupervisedRunOptions) SupervisedResult {
	if err := ctx.Err(); err != nil {
		return SupervisedResult{ExitKind: supervisedUnixContextExitKind(err), Err: err}
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid, cmd.SysProcAttr.Pgid = true, 0
	interrupted := make(chan struct{})
	var interruptOnce sync.Once
	if cmd.Cancel != nil {
		// The callback records intent only. A single lifecycle owner signals
		// the group, and never does so after releasing its guardian lease.
		// No callback waits for guardian setup, including setup failures.
		cmd.Cancel = func() error {
			interruptOnce.Do(func() { close(interrupted) })
			return nil
		}
	}
	if err := cmd.Start(); err != nil {
		return SupervisedResult{ExitKind: SupervisedExitNormal, Err: err}
	}
	lease, err := startDarwinProcessGroupLease(cmd.Process.Pid)
	if err != nil {
		// The target has not been waited/reaped yet, so its own PID still
		// reserves this group identity for this one best-effort signal.
		_ = killSupervisedUnixProcessGroup(cmd)
		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()
		failure := errors.Join(ctx.Err(), fmt.Errorf("supervisor: guardian setup failed: %w", err), waitForExistingWait(waitCh), errDarwinTreeCleanupUnproven)
		logging.Warning("[supervisor] Darwin guardian setup failed; cleanup incomplete: %v", failure)
		return SupervisedResult{ExitKind: supervisedUnixContextExitKind(ctx.Err()), Err: failure}
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	var waitErr error
	waited := false
	select {
	case waitErr = <-waitCh:
		waited = true
	case <-ctx.Done():
	case <-interrupted:
	}
	wasInterrupted := ctx.Err() != nil
	select {
	case <-interrupted:
		wasInterrupted = true
	default:
	}
	if !wasInterrupted {
		// Closing the guardian's own stdin affects no target output or exit
		// receipt. This path never sends a group signal.
		releaseErr := lease.release(time.Now().Add(killWaitTimeout))
		return SupervisedResult{ExitKind: classifyUnixExit(waitErr), Err: errors.Join(waitErr, releaseErr)}
	}

	deadline := time.Now().Add(killWaitTimeout)
	waitErr, cleanupErr := cleanupDarwinProcessGroup(lease, waitCh, waited, waitErr, deadline)
	releaseErr := lease.release(deadline)
	cause := ctx.Err()
	if cause == nil {
		// Cmd's private context exposes cancellation intent but no error
		// accessor. Do not invent whether it was cancel or deadline.
		cause = errors.New("supervisor: command context interrupted execution")
	}
	failure := errors.Join(cause, waitErr, cleanupErr, releaseErr, errDarwinTreeCleanupUnproven)
	logging.Warning("[supervisor] Darwin cancellation cleanup incomplete (no all-descendant termination proof): %v", failure)
	return SupervisedResult{ExitKind: supervisedUnixContextExitKind(ctx.Err()), Err: failure}
}

type darwinProcessGroupLease struct {
	guardian *exec.Cmd
	input    *os.File
	pgid     int
	released bool
}

func startDarwinProcessGroupLease(pgid int) (*darwinProcessGroupLease, error) {
	if pgid <= 0 {
		return nil, errors.New("invalid target process group")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	// An absolute, plain executable avoids shell/PATH interpretation and
	// another CommandContext watcher. Its sole work is waiting for pipe EOF.
	guardian := exec.Command("/bin/cat")
	guardian.Env = []string{"LC_ALL=C"}
	guardian.Stdin = reader
	guardian.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid}
	if err := guardian.Start(); err != nil {
		_ = writer.Close()
		return nil, err
	}
	return &darwinProcessGroupLease{guardian: guardian, input: writer, pgid: pgid}, nil
}

func (l *darwinProcessGroupLease) signal() error {
	if l == nil || l.released || l.guardian == nil || l.guardian.Process == nil || l.pgid <= 0 {
		return errors.New("supervisor: process-group lease is unavailable")
	}
	return syscall.Kill(-l.pgid, syscall.SIGKILL)
}

func (l *darwinProcessGroupLease) release(deadline time.Time) error {
	if l == nil || l.released {
		return nil
	}
	l.released = true // No later group signal is permitted, even on Wait error.
	_ = l.input.Close()
	// This exact unreaped child remains ours; do not signal the old PGID.
	_ = l.guardian.Process.Kill()
	waitCh := make(chan error, 1)
	go func() { waitCh <- l.guardian.Wait() }()
	err := waitForExistingWaitWithin(waitCh, time.Until(deadline))
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil // Expected guardian SIGKILL, not target execution evidence.
	}
	return err
}

func cleanupDarwinProcessGroup(l *darwinProcessGroupLease, waitCh <-chan error, waited bool, waitErr error, deadline time.Time) (error, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(max(time.Until(deadline), 0))
	defer timer.Stop()
	var observationErr error
	for {
		if err := l.signal(); err != nil {
			observationErr = fmt.Errorf("supervisor: leased group signal failed: %w", err)
		}
		if !waited {
			select {
			case waitErr = <-waitCh:
				waited = true
			default:
			}
		}
		members, err := snapshotDarwinPublishedProcessGroup(l.pgid)
		guardianSeen, visibleLive := false, false
		if err != nil {
			observationErr = fmt.Errorf("supervisor: published group snapshot unavailable: %w", err)
		} else {
			for _, member := range members {
				if member.PID == l.guardian.Process.Pid {
					guardianSeen = true
					continue
				}
				if member.State != 5 { // Native SZOMB, not a textual process label.
					visibleLive = true
				}
			}
			if !guardianSeen {
				observationErr = errors.New("supervisor: unreaped guardian absent from published group snapshot")
			}
		}
		if waited && ((err == nil && guardianSeen && !visibleLive) || err != nil) {
			// A final leased sweep improves best-effort coverage but is not a
			// quiescence certificate: NEW/escaped members remain unobservable.
			finalErr := l.signal()
			return waitErr, errors.Join(observationErr, finalErr)
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			return waitErr, errors.Join(observationErr, errSupervisedWaitIncomplete)
		}
	}
}
