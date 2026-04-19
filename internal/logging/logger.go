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
	"sort"
	"strconv"
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
	maxFileBytes        = 4 * 1024 * 1024 // 4 MiB per file before rotation
	defaultMaxTotalFiles = 7              // default retention count
	filePrefix          = "codrax-"
	fileSuffix          = ".log"
	// fileTimeLayout is the timestamp embedded in each log filename.
	// The layout sorts lexicographically the same way it sorts
	// chronologically, so plain os.ReadDir + sort.Strings is enough
	// to find the newest/oldest files.
	fileTimeLayout = "20060102-150405-000"
)

// FileTimeLayout is the exported filename-timestamp layout. Other
// runtime-artifact producers (blob session dirs) reuse it so operators
// can correlate logs and blobs by the timestamp prefix.
const FileTimeLayout = fileTimeLayout

// maxTotalFiles governs log retention. Defaults to defaultMaxTotalFiles;
// SetMaxTotalFiles lets cmd/root.go override it from codrax.yaml
// before any logger is constructed.
var maxTotalFiles = defaultMaxTotalFiles

// SetMaxTotalFiles overrides the retention cap. Non-positive values
// are ignored so callers can pass an unresolved config field without
// extra plumbing.
func SetMaxTotalFiles(n int) {
	if n > 0 {
		maxTotalFiles = n
	}
}

// IsPidAlive exposes the build-tagged pid liveness probe so other
// runtime-artifact subsystems (blob session pruning) can share the
// same multi-instance safety guarantee without duplicating the
// Unix/Windows implementation.
func IsPidAlive(pid int) bool { return isPidAlive(pid) }

// Filename format: codrax-<timestamp>-<pid>.log. The PID suffix is
// what makes the multi-instance scenario safe: each running codrax
// owns a disjoint set of log files, so two instances pointed at the
// same -log-dir never share an fd, never race on rotation, and the
// retention sweeper can tell from the filename alone whether a file
// belongs to a process that is still running. Within one instance,
// rotation produces multiple files all carrying the same PID and a
// later timestamp, which still sort chronologically.

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

// NewFromFlags constructs a Logger writing to a timestamped file inside
// dir at the given level. If mirrorStdout is true, every record is also
// written to os.Stdout. The directory is created if missing.
//
// Filenames are of the form codrax-YYYYMMDD-HHMMSS-mmm.log. On startup
// the newest existing file is reused if it is still under the size cap;
// otherwise a fresh file is created. Old files are pruned so the
// directory never holds more than maxTotalFiles entries.
func NewFromFlags(dir, level string, mirrorStdout bool) (*Logger, error) {
	if dir == "" {
		dir = "logs"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	rw, err := newRotatingWriter(dir)
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

// IsDebug reports whether debug-level records would be written. Use it
// to guard state-gathering work that is only needed for debug output.
func (l *Logger) IsDebug() bool { return l != nil && l.level >= LevelDebug }

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

// IsDebug reports whether the Default logger would emit debug records.
func IsDebug() bool { return Default.IsDebug() }

// HintBodyMax is the byte cap shared by every hint-body Debug log
// (analyzer must-emit, orchestrator window hint, explorer mid-loop,
// explorer end-of-stage retry, agent mid-loop inject). Centralising
// the cap keeps the trace stream uniform regardless of which subsystem
// produced the hint.
const HintBodyMax = 1000

// Truncate clips s to max bytes and appends a marker noting how many
// bytes were dropped. The marker is essential so a downstream reader
// can tell "the hint really ended here" from "the log truncated".
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("...[truncated %d bytes]", len(s)-max)
}

// rotatingWriter is a size-bounded file writer that names each file
// after its creation timestamp and the owning process's PID:
//
//	codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log
//
// Every writer always opens a fresh file at startup. Resuming into a
// pre-existing file is intentionally NOT supported because the PID
// suffix already isolates this process from any other writer in the
// same directory; resume would defeat that property by sharing an fd
// with a previous (now exited) PID's last file. Slightly more files
// on disk in exchange for zero cross-instance write races.
//
// When a write would push the active file past the cap, the writer
// closes the file, opens a new one (same PID, new timestamp), and
// asks pruneOldFiles to enforce the retention cap — skipping any
// files whose embedded PID still belongs to a running process.
type rotatingWriter struct {
	dir  string
	f    *os.File
	size int64
}

func newRotatingWriter(dir string) (*rotatingWriter, error) {
	w := &rotatingWriter{dir: dir}
	if err := w.openNew(); err != nil {
		return nil, err
	}
	// Sweep stale files left behind by previous (exited) instances on
	// startup so a freshly-launched binary doesn't inherit a hoard of
	// orphan logs from earlier crashes.
	pruneOldFiles(dir)
	return w, nil
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

// rotate closes the current file, opens a new timestamped one, and
// prunes the directory back to the retention window.
func (w *rotatingWriter) rotate() error {
	if err := w.f.Close(); err != nil {
		return err
	}
	if err := w.openNew(); err != nil {
		return err
	}
	pruneOldFiles(w.dir)
	return nil
}

// openNew creates a new log file using the current timestamp and the
// owning process's PID. The PID suffix means two processes can never
// pick the same name; the time+counter loop only protects against the
// (vanishingly unlikely) case of a single process rotating twice
// within the same millisecond.
func (w *rotatingWriter) openNew() error {
	pid := os.Getpid()
	base := fmt.Sprintf("%s%s-%d", filePrefix, time.Now().Format(fileTimeLayout), pid)
	path := filepath.Join(w.dir, base+fileSuffix)
	for i := 1; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(w.dir, fmt.Sprintf("%s-%d%s", base, i, fileSuffix))
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	w.f = f
	w.size = 0
	return nil
}

// pidFromLogFilename extracts the PID embedded in a log filename. The
// canonical format is codrax-<timestamp>-<pid>.log; rotation
// collisions add a "-<n>" suffix before .log, which we strip first.
// Returns 0 when the filename does not match (e.g. legacy files from
// before per-PID naming, or unrelated files dropped into the dir),
// and 0 is treated by callers as "owner unknown, eligible to prune".
func pidFromLogFilename(name string) int {
	if !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileSuffix) {
		return 0
	}
	core := strings.TrimSuffix(strings.TrimPrefix(name, filePrefix), fileSuffix)
	// core is now <timestamp>-<pid> or <timestamp>-<pid>-<n>.
	// Split from the right: the PID is the rightmost numeric segment
	// that is not a small collision counter. We try the last segment
	// first, then the second-to-last if the last looks like a small
	// collision counter (i.e. when there are at least 4 segments:
	// date, time, ms, pid, [collision]).
	parts := strings.Split(core, "-")
	if len(parts) < 4 {
		return 0 // legacy filename, no PID encoded
	}
	// Try last segment as PID first (the common case).
	if pid, err := strconv.Atoi(parts[len(parts)-1]); err == nil && pid > 0 {
		// If there are 5+ segments AND the last looks small (< 10000)
		// AND the second-to-last is a plausible PID, prefer that one.
		if len(parts) >= 5 && pid < 10000 {
			if maybePid, err := strconv.Atoi(parts[len(parts)-2]); err == nil && maybePid > 0 {
				return maybePid
			}
		}
		return pid
	}
	return 0
}

// listLogFiles returns absolute paths of all codrax-*.log files in dir,
// sorted ascending so the newest is at the end. Non-matching entries
// are ignored.
func listLogFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list log dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, filePrefix) && strings.HasSuffix(name, fileSuffix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = filepath.Join(dir, n)
	}
	return out, nil
}

// pruneOldFiles deletes the oldest log files until at most
// maxTotalFiles remain in dir, while never deleting a file that
// belongs to a process that is still running. The PID-liveness check
// is what makes the multi-instance scenario safe: instance A's
// rotation must not delete instance B's active file out from under
// it. Files whose PID we cannot determine (legacy names, third-party
// drops) are treated as owner-unknown and pruned normally — they
// can't be active codrax logs by definition.
//
// Best-effort: filesystem errors abort the sweep silently, on the
// theory that retention is a hygiene concern, not a correctness one.
func pruneOldFiles(dir string) {
	files, err := listLogFiles(dir)
	if err != nil {
		return
	}
	selfPid := os.Getpid()
	for len(files) > maxTotalFiles {
		// Walk from oldest forward, deleting the first file that is
		// safe to remove. If we make it through the whole slice
		// without deleting anything, every remaining file is owned
		// by a live peer and we have to leave the dir over-cap until
		// one of those peers exits.
		removed := false
		for i, path := range files {
			pid := pidFromLogFilename(filepath.Base(path))
			if pid != 0 && pid != selfPid && isPidAlive(pid) {
				continue // owned by a live peer, leave it alone
			}
			if err := os.Remove(path); err == nil {
				files = append(files[:i], files[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			return
		}
	}
}

func (w *rotatingWriter) Close() error {
	if w == nil || w.f == nil {
		return nil
	}
	return w.f.Close()
}
