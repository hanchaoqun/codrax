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
	start := progressStarted(opts, stage, message, cmd.Path, "")
	var output boundedCommandBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		progressFinished(opts, stage, "external command failed to start", cmd.Path, "", start, ProgressStatusFailed)
		return output.Bytes(), err
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
			status := ProgressStatusComplete
			if err != nil {
				status = ProgressStatusFailed
			}
			progressFinished(opts, stage, terminalProgressMessage(message, status), cmd.Path, "", start, status)
			return output.Bytes(), err
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
			return "completed official simpleperf adapter"
		case "running official hiperf adapter":
			return "completed official hiperf adapter"
		}
	case ProgressStatusFailed:
		switch message {
		case "running trace_streamer SQLite DB export":
			return "trace_streamer SQLite DB export failed"
		case "running official simpleperf adapter":
			return "official simpleperf adapter failed"
		case "running official hiperf adapter":
			return "official hiperf adapter failed"
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
