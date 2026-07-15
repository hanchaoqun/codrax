package hitraceconv

import (
	"os/exec"
	"strings"
	"time"
)

const (
	ProgressStatusStarted  = "started"
	ProgressStatusProgress = "progress"
	ProgressStatusComplete = "complete"
	ProgressStatusFailed   = "failed"

	progressHeartbeatInterval = 5 * time.Second
)

type ProgressFunc func(ProgressEvent)

type ProgressEvent struct {
	Stage      string
	Status     string
	Message    string
	Path       string
	OutputPath string
	BytesDone  int64
	BytesTotal int64
	Records    int
	Elapsed    time.Duration
}

func emitProgress(opts Options, event ProgressEvent) {
	if opts.Progress == nil {
		return
	}
	event.Stage = strings.TrimSpace(event.Stage)
	event.Status = strings.TrimSpace(event.Status)
	if event.Status == "" {
		event.Status = ProgressStatusProgress
	}
	opts.Progress(event)
}

func progressStarted(opts Options, stage, message, path, outputPath string) time.Time {
	start := time.Now()
	emitProgress(opts, ProgressEvent{
		Stage:      stage,
		Status:     ProgressStatusStarted,
		Message:    message,
		Path:       path,
		OutputPath: outputPath,
	})
	return start
}

func progressFinished(opts Options, stage, message, path, outputPath string, start time.Time, status string) {
	emitProgress(opts, ProgressEvent{
		Stage:      stage,
		Status:     status,
		Message:    message,
		Path:       path,
		OutputPath: outputPath,
		Elapsed:    time.Since(start),
	})
}

func runCommandWithProgress(opts Options, cmd *exec.Cmd, stage, message string) ([]byte, error) {
	output, err, start, started := runCommandWithProgressUntilExit(opts, cmd, stage, message)
	status := ProgressStatusComplete
	if err != nil {
		status = ProgressStatusFailed
	}
	terminalMessage := terminalProgressMessage(message, status)
	if !started {
		terminalMessage = "external command failed to start"
	}
	progressFinished(opts, stage, terminalMessage, cmd.Path, "", start, status)
	return output, err
}

// runCommandWithProgressUntilExit emits start/heartbeat events but deliberately
// leaves the terminal event to the caller. Providers with post-exit authority
// gates use this form so a child exit cannot be reported as successful before
// source, staging, and output generations have been validated.
func runCommandWithProgressUntilExit(opts Options, cmd *exec.Cmd, stage, message string) ([]byte, error, time.Time, bool) {
	start := progressStarted(opts, stage, message, cmd.Path, "")
	var output boundedCommandBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return output.Bytes(), err, start, false
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	ticker := time.NewTicker(progressHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return output.Bytes(), err, start, true
		case <-ticker.C:
			emitProgress(opts, ProgressEvent{
				Stage:   stage,
				Status:  ProgressStatusProgress,
				Message: message,
				Path:    cmd.Path,
				Elapsed: time.Since(start),
			})
		}
	}
}

func terminalProgressMessage(message, status string) string {
	message = strings.TrimSpace(message)
	switch strings.TrimSpace(status) {
	case ProgressStatusComplete:
		switch message {
		case "running trace_streamer SQLite DB export":
			return "completed trace_streamer SQLite DB export"
		case "running official simpleperf adapter":
			return "completed official simpleperf adapter command"
		case "running official hiperf adapter":
			return "completed official hiperf adapter command"
		}
	case ProgressStatusFailed:
		switch message {
		case "running trace_streamer SQLite DB export":
			return "trace_streamer SQLite DB export failed"
		case "running official simpleperf adapter":
			return "official simpleperf adapter command failed"
		case "running official hiperf adapter":
			return "official hiperf adapter command failed"
		}
	}
	return message
}

type boundedCommandBuffer struct {
	buf       []byte
	truncated bool
}

func (b *boundedCommandBuffer) Write(p []byte) (int, error) {
	const max = 4096
	remaining := max - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) <= remaining {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}
	b.buf = append(b.buf, p[:remaining]...)
	b.truncated = true
	return len(p), nil
}

func (b *boundedCommandBuffer) Bytes() []byte {
	if !b.truncated {
		return append([]byte(nil), b.buf...)
	}
	out := append([]byte(nil), b.buf...)
	out = append(out, []byte("\n[output truncated]")...)
	return out
}
