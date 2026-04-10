// Package logging provides a leveled, file-rotating logger used by the
// codrax binary. The default sink is a single file under a configured
// directory; rotation kicks in once the active file exceeds maxFileBytes
// and a fixed number of historical files are retained.
//
// The package exposes a small free-function API (Error/Warning/Info/Debug)
// that delegates to a process-wide Default logger initialized from main().
// All log records are funneled through this package so the terminal stays
// clean for user interaction in REPL mode; mirroring to stdout is opt-in
// via the -log-stdout flag.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level controls which records are emitted. Lower values are more severe.
// Records with level <= Logger.level are written.
type Level int

const (
	LevelError Level = iota
	LevelWarning
	LevelInfo
	LevelDebug
)

// ParseLevel converts a textual level (case-insensitive) into a Level.
// Unknown values fall back to LevelInfo.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return LevelError
	case "warn", "warning":
		return LevelWarning
	case "debug":
		return LevelDebug
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelError:
		return "ERROR"
	case LevelWarning:
		return "WARN"
	case LevelDebug:
		return "DEBUG"
	default:
		return "INFO"
	}
}

const (
	maxFileBytes  = 4 * 1024 * 1024 // 4 MiB per file before rotation
	maxKeptFiles  = 6               // historical files kept (current + 6 = 7 total)
	defaultName   = "codrax.log"
)

// Logger is a leveled writer with optional stdout mirroring.
//
// Logger is safe for concurrent use; the embedded mutex serializes both
// writes and rotation so a rotation triggered mid-record never tears the
// output. Close releases the underlying file.
type Logger struct {
	mu      sync.Mutex
	level   Level
	file    *rotatingWriter
	stdout  bool
	closed  bool
}

// NewFromFlags constructs a Logger writing to <dir>/codrax.log at the
// given level. If mirrorStdout is true, every record is also written to
// os.Stdout. The directory is created if missing.
func NewFromFlags(dir, level string, mirrorStdout bool) (*Logger, error) {
	if dir == "" {
		dir = "logs"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	rw, err := newRotatingWriter(filepath.Join(dir, defaultName))
	if err != nil {
		return nil, err
	}
	return &Logger{
		level:  ParseLevel(level),
		file:   rw,
		stdout: mirrorStdout,
	}, nil
}

// Close flushes and closes the underlying file. Subsequent writes are
// no-ops. Safe to call multiple times.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	return l.file.Close()
}

// InfoWriter returns an io.Writer that emits each Write as one INFO
// record. It exists so callers can redirect stdlib log.SetOutput onto
// the leveled logger as a safety net for any code still using log.Printf.
func (l *Logger) InfoWriter() io.Writer { return &levelWriter{l: l, lvl: LevelInfo} }

func (l *Logger) log(lvl Level, format string, args ...interface{}) {
	if l == nil || lvl > l.level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	// Strip trailing newlines so we always emit exactly one per record.
	msg = strings.TrimRight(msg, "\n")
	line := fmt.Sprintf("%s %s %s\n",
		time.Now().Format("2006-01-02T15:04:05.000"),
		lvl.String(),
		msg,
	)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	_, _ = l.file.Write([]byte(line))
	if l.stdout {
		_, _ = os.Stdout.Write([]byte(line))
	}
}

func (l *Logger) Error(format string, args ...interface{})   { l.log(LevelError, format, args...) }
func (l *Logger) Warning(format string, args ...interface{}) { l.log(LevelWarning, format, args...) }
func (l *Logger) Info(format string, args ...interface{})    { l.log(LevelInfo, format, args...) }
func (l *Logger) Debug(format string, args ...interface{})   { l.log(LevelDebug, format, args...) }

// levelWriter adapts a Logger to io.Writer at a fixed level.
type levelWriter struct {
	l   *Logger
	lvl Level
}

func (w *levelWriter) Write(p []byte) (int, error) {
	w.l.log(w.lvl, "%s", string(p))
	return len(p), nil
}

// Default is the process-wide logger. Initialize from main() before
// calling any of the package-level convenience functions.
var Default *Logger

// SetDefault installs lg as the package-level Default. Passing nil
// disables logging via the free functions.
func SetDefault(lg *Logger) { Default = lg }

// Free-function API delegates to Default. Calls before SetDefault are
// silently dropped, which is the desired behavior for early-startup
// errors that must surface via os.Stderr instead.
func Error(format string, args ...interface{})   { Default.Error(format, args...) }
func Warning(format string, args ...interface{}) { Default.Warning(format, args...) }
func Info(format string, args ...interface{})    { Default.Info(format, args...) }
func Debug(format string, args ...interface{})   { Default.Debug(format, args...) }

// rotatingWriter is a size-bounded file writer. It tracks the current
// file size and rotates by renaming the active file to .1, shifting
// existing .1..N suffixes outward, and dropping anything beyond
// maxKeptFiles. The current file is always at base path (no suffix).
type rotatingWriter struct {
	path string
	f    *os.File
	size int64
}

func newRotatingWriter(path string) (*rotatingWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	return &rotatingWriter{path: path, f: f, size: stat.Size()}, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	if w.size+int64(len(p)) > maxFileBytes {
		if err := w.rotate(); err != nil {
			// On rotation failure we still try to write to the existing
			// file so a transient FS error does not silently drop logs.
			_, _ = fmt.Fprintf(os.Stderr, "logging: rotate failed: %v\n", err)
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate closes the current file, shifts historical files outward by
// one suffix, drops the oldest beyond maxKeptFiles, and reopens a fresh
// file at the base path.
func (w *rotatingWriter) rotate() error {
	if err := w.f.Close(); err != nil {
		return err
	}
	// Remove the file that would fall off the retention window.
	oldest := fmt.Sprintf("%s.%d", w.path, maxKeptFiles)
	_ = os.Remove(oldest)
	// Shift .N-1 -> .N, ..., .1 -> .2.
	for i := maxKeptFiles - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", w.path, i)
		to := fmt.Sprintf("%s.%d", w.path, i+1)
		if _, err := os.Stat(from); err == nil {
			_ = os.Rename(from, to)
		}
	}
	// Active -> .1
	_ = os.Rename(w.path, w.path+".1")

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.f = f
	w.size = 0
	return nil
}

func (w *rotatingWriter) Close() error {
	if w == nil || w.f == nil {
		return nil
	}
	return w.f.Close()
}
