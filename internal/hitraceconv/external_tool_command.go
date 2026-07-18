package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const externalToolCommandWaitDelay = 500 * time.Millisecond

var errExternalToolSupervisorAuthority = errors.New("external tool process supervisor authority failed")

type externalToolSupervisorError struct {
	Stage string
	Cause error
}

func (err *externalToolSupervisorError) Error() string {
	if err == nil {
		return errExternalToolSupervisorAuthority.Error()
	}
	return fmt.Sprintf("%s: code=external_tool_supervisor_failed stage=%s: %v",
		errExternalToolSupervisorAuthority, err.Stage, err.Cause)
}

func (err *externalToolSupervisorError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *externalToolSupervisorError) Is(target error) bool {
	return target == errExternalToolSupervisorAuthority
}

func newExternalToolSupervisorError(stage string, cause error) error {
	if cause == nil {
		cause = errors.New("unknown supervisor failure")
	}
	return &externalToolSupervisorError{Stage: stage, Cause: cause}
}

// externalToolCommand is deliberately opaque. Provider code can set the
// environment, but only the shared runner below owns Start, Wait and process-
// tree termination. In particular, callers cannot regain exec.Cmd.Run.
type externalToolCommand struct {
	ctx        context.Context
	cmd        *exec.Cmd
	supervisor *externalToolProcessSupervisor
	waitCancel context.CancelFunc

	mu      sync.Mutex
	claimed bool
}

func newExternalToolCommand(ctx context.Context, executable string, args ...string) (*externalToolCommand, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, waitCancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(waitCtx, executable, args...)
	// The internal context exists only to arm WaitDelay after the platform
	// supervisor has acted. Never let os/exec's direct-child Kill become the
	// cancellation authority.
	cmd.Cancel = nil
	cmd.WaitDelay = externalToolCommandWaitDelay
	supervisor, err := newExternalToolProcessSupervisor(cmd)
	if err != nil {
		waitCancel()
		return nil, newExternalToolSupervisorError("prepare", err)
	}
	return &externalToolCommand{
		ctx: ctx, cmd: cmd, supervisor: supervisor, waitCancel: waitCancel,
	}, nil
}

func (command *externalToolCommand) setEnvironment(environment []string) {
	if command == nil || command.cmd == nil {
		return
	}
	command.cmd.Env = append([]string(nil), environment...)
}

func (command *externalToolCommand) setExtraFiles(files []*os.File) {
	if command == nil || command.cmd == nil {
		return
	}
	command.cmd.ExtraFiles = append([]*os.File(nil), files...)
}

func (command *externalToolCommand) setOutput(stdout, stderr io.Writer) {
	if command == nil || command.cmd == nil {
		return
	}
	command.cmd.Stdout = stdout
	command.cmd.Stderr = stderr
}

func (command *externalToolCommand) path() string {
	if command == nil || command.cmd == nil {
		return ""
	}
	return command.cmd.Path
}

func (command *externalToolCommand) arguments() []string {
	if command == nil || command.cmd == nil {
		return nil
	}
	return append([]string(nil), command.cmd.Args...)
}

func (command *externalToolCommand) extraFileCount() int {
	if command == nil || command.cmd == nil {
		return 0
	}
	return len(command.cmd.ExtraFiles)
}

func (command *externalToolCommand) claim() error {
	if command == nil || command.cmd == nil || command.supervisor == nil || command.waitCancel == nil {
		return newExternalToolSupervisorError("claim", errors.New("command authority is incomplete"))
	}
	command.mu.Lock()
	defer command.mu.Unlock()
	if command.claimed {
		return newExternalToolSupervisorError("claim", errors.New("command authority was already consumed"))
	}
	command.claimed = true
	return nil
}

// runExternalToolCommandUntilExit is the one Start/Wait authority for every
// trace/perf child, including the embedded trace_streamer loader probe.
func runExternalToolCommandUntilExit(command *externalToolCommand, heartbeat func(time.Duration)) (runErr error, started bool) {
	if err := command.claim(); err != nil {
		return err, false
	}
	if ctxErr := command.ctx.Err(); ctxErr != nil {
		closeErr := command.supervisor.close()
		command.waitCancel()
		if closeErr != nil {
			return traceDBJoinPreservingSingle(ctxErr, newExternalToolSupervisorError("close_before_start", closeErr)), false
		}
		return ctxErr, false
	}
	if err := command.cmd.Start(); err != nil {
		closeErr := command.supervisor.close()
		command.waitCancel()
		if closeErr != nil {
			return traceDBJoinPreservingSingle(newExternalToolSupervisorError("close_after_start_failure", closeErr), err), false
		}
		return err, false
	}
	started = true
	if err := command.supervisor.afterStart(command.cmd); err != nil {
		terminateErr := command.supervisor.terminate()
		command.waitCancel()
		waitErr := command.cmd.Wait()
		closeErr := command.supervisor.close()
		hardErr := newExternalToolSupervisorError("assign", err)
		if terminateErr != nil {
			hardErr = traceDBJoinPreservingSingle(hardErr, newExternalToolSupervisorError("terminate_after_assign_failure", terminateErr))
		}
		if closeErr != nil {
			hardErr = traceDBJoinPreservingSingle(hardErr, newExternalToolSupervisorError("close_after_assign_failure", closeErr))
		}
		return traceDBJoinPreservingSingle(hardErr, waitErr), true
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- command.cmd.Wait()
	}()
	var ticker *time.Ticker
	var tickerChannel <-chan time.Time
	if heartbeat != nil {
		ticker = time.NewTicker(progressHeartbeatInterval)
		tickerChannel = ticker.C
		defer ticker.Stop()
	}
	startedAt := time.Now()
	for {
		select {
		case waitErr := <-waitDone:
			return command.finish(waitErr, command.ctx.Err()), true
		case <-command.ctx.Done():
			terminateErr := command.supervisor.terminate()
			command.waitCancel()
			waitErr := <-waitDone
			return command.finishAfterTerminate(waitErr, command.ctx.Err(), terminateErr), true
		case <-tickerChannel:
			heartbeat(time.Since(startedAt))
		}
	}
}

func (command *externalToolCommand) finish(waitErr, contextErr error) error {
	terminateErr := command.supervisor.terminate()
	command.waitCancel()
	return command.finishAfterTerminate(waitErr, contextErr, terminateErr)
}

func (command *externalToolCommand) finishAfterTerminate(waitErr, contextErr, terminateErr error) error {
	closeErr := command.supervisor.close()
	if errors.Is(waitErr, exec.ErrWaitDelay) {
		// WaitDelay closed a pipe retained by a descendant after the direct
		// child exited. The process-tree supervisor has now terminated that
		// descendant, so this is evidence of cleanup rather than a child error.
		waitErr = nil
	}
	var hardErr error
	if terminateErr != nil {
		hardErr = newExternalToolSupervisorError("terminate", terminateErr)
	}
	if closeErr != nil {
		hardErr = traceDBJoinPreservingSingle(hardErr, newExternalToolSupervisorError("close", closeErr))
	}
	if contextErr != nil {
		return traceDBJoinPreservingSingle(contextErr, hardErr, waitErr)
	}
	if hardErr != nil {
		return traceDBJoinPreservingSingle(hardErr, waitErr)
	}
	return waitErr
}
