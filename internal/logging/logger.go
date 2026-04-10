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
	maxTotalFiles = 7               // total files kept in the log dir
	filePrefix    = "codrax-"
	fileSuffix    = ".log"
	// fileTimeLayout is the timestamp embedded in each log filename.
	// The layout sorts lexicographically the same way it sorts
	// chronologically, so plain os.ReadDir + sort.Strings is enough
	// to find the newest/oldest files.
	fileTimeLayout = "20060102-150405-000"
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

// rotatingWriter is a size-bounded file writer that names each file
// after its creation timestamp:
//
//	codrax-YYYYMMDD-HHMMSS-mmm.log
//
// On startup the newest existing file is reused (appended to) if it
// is still under the size cap; otherwise a fresh file is created with
// the current timestamp. When a write would push the active file past
// the cap, it closes the file, opens a new one, and prunes the
// directory back to maxTotalFiles entries by deleting the oldest.
type rotatingWriter struct {
	dir  string
	f    *os.File
	size int64
}

func newRotatingWriter(dir string) (*rotatingWriter, error) {
	files, err := listLogFiles(dir)
	if err != nil {
		return nil, err
	}

	// Try to resume into the newest existing file if it has room.
	if len(files) > 0 {
		newest := files[len(files)-1]
		stat, err := os.Stat(newest)
		if err == nil && stat.Size() < maxFileBytes {
			f, err := os.OpenFile(newest, os.O_WRONLY|os.O_APPEND, 0o644)
			if err == nil {
				return &rotatingWriter{dir: dir, f: f, size: stat.Size()}, nil
			}
		}
	}

	// Otherwise open a fresh timestamped file.
	w := &rotatingWriter{dir: dir}
	if err := w.openNew(); err != nil {
		return nil, err
	}
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

// openNew creates a new log file using the current timestamp. If a
// file with that exact name already exists (e.g. two rotations within
// the same millisecond), it appends a numeric suffix to make the name
// unique.
func (w *rotatingWriter) openNew() error {
	base := filePrefix + time.Now().Format(fileTimeLayout)
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
// maxTotalFiles remain in dir.
func pruneOldFiles(dir string) {
	files, err := listLogFiles(dir)
	if err != nil {
		return
	}
	for len(files) > maxTotalFiles {
		_ = os.Remove(files[0])
		files = files[1:]
	}
}

func (w *rotatingWriter) Close() error {
	if w == nil || w.f == nil {
		return nil
	}
	return w.f.Close()
}
