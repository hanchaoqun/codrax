package tool

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// NativeGrepOpts configures a Go-native regex scan that mimics the
// output format of `grep -rnH` (content mode) or `grep -rl`
// (files-only mode). Used as the last-resort backend when neither
// ripgrep nor grep is available on PATH — see search.go::UseNativeGrep.
type NativeGrepOpts struct {
	Pattern      string // regex source (ERE-compatible subset)
	Root         string // directory or file to scan
	IgnoreCase   bool
	FilesOnly    bool     // emit one path per matching file, no line content
	ContextLines int      // lines of context before/after each match (0 = off, max 20)
	Include      string   // glob filter against basename (filepath.Match syntax, e.g. "*.go")
	ExcludeDirs  []string // directory names to skip at any depth
	ShouldSkip   func(path string, d fs.DirEntry) bool
	MaxFileBytes int // directory search: 0 = default cap, <0 = no limit; explicit single-file roots default to no limit
	MaxMatches   int // stop after this many line-level matches across all files; 0 = no cap
}

// NativeGrepResult carries the rendered output bytes plus a lightweight
// summary used for logging / banners.
type NativeGrepResult struct {
	Output            string // "path:line:content\n…" (or "path\n…" when FilesOnly)
	Matches           int    // number of lines emitted (FilesOnly: files emitted)
	Files             int    // distinct files that contributed at least one match
	Truncated         bool   // MaxMatches hit
	FilesTried        int    // files opened and scanned
	SkippedLargeFiles int    // files skipped by MaxFileBytes
}

// nativeGrepMaxFileBytes caps broad directory scans when the caller does not
// specify a limit. Explicit single-file searches are streamed without this
// cap, because large user-provided logs / systrace / perfetto artifacts are
// often exactly the target being searched. 4 MiB keeps broad scans away from
// pathological generated files, vendored minified JS, and SVG blobs.
const nativeGrepMaxFileBytes = 4 << 20

// NativeGrep runs a pure-Go regex scan and returns output formatted
// to mimic grep -rnH / grep -rl. Honors ctx cancellation at the
// per-file boundary. Excludes binary-looking files via a NUL-byte
// sniff on the first 512 bytes.
func NativeGrep(ctx context.Context, opts NativeGrepOpts) (NativeGrepResult, error) {
	var res NativeGrepResult

	pattern := opts.Pattern
	if pattern == "" {
		return res, fmt.Errorf("native grep: empty pattern")
	}
	if opts.IgnoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return res, fmt.Errorf("native grep: %v", err)
	}

	root := opts.Root
	if root == "" {
		root = "."
	}
	rootIsFile := false
	if info, statErr := os.Stat(root); statErr == nil {
		rootIsFile = !info.IsDir()
	}
	maxBytes := opts.MaxFileBytes
	if maxBytes == 0 {
		if rootIsFile {
			// An explicit file target is usually a user-provided runtime
			// artifact (log/systrace/perfetto/etc.) or a known source file.
			// Scan it line-by-line instead of silently skipping the tail due
			// to the broad-directory safety cap.
			maxBytes = -1
		} else {
			maxBytes = nativeGrepMaxFileBytes
		}
	}

	var out bytes.Buffer
	matchedFiles := make(map[string]bool)

	emitLine := func(path string, lineno int, content string) {
		if opts.FilesOnly {
			if matchedFiles[path] {
				return
			}
			matchedFiles[path] = true
			out.WriteString(path)
			out.WriteByte('\n')
			res.Matches++
			return
		}
		out.WriteString(path)
		out.WriteByte(':')
		fmt.Fprintf(&out, "%d", lineno)
		out.WriteByte(':')
		out.WriteString(content)
		out.WriteByte('\n')
		res.Matches++
		matchedFiles[path] = true
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if IsWindowsReservedDevicePath(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if opts.ShouldSkip != nil && opts.ShouldSkip(path, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if DirNameMatchesExcludePattern(d.Name(), opts.ExcludeDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		if opts.MaxMatches > 0 && res.Matches >= opts.MaxMatches {
			res.Truncated = true
			return filepath.SkipAll
		}
		if opts.Include != "" {
			matched, mErr := filepath.Match(opts.Include, d.Name())
			if mErr != nil || !matched {
				return nil
			}
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if maxBytes > 0 && info.Size() > int64(maxBytes) {
			res.SkippedLargeFiles++
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		defer f.Close()

		sniff := make([]byte, 512)
		n, _ := f.Read(sniff)
		if bytes.IndexByte(sniff[:n], 0) >= 0 {
			return nil
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil
		}

		res.FilesTried++
		rel := relPathForDisplay(root, path)

		reader := bufio.NewReaderSize(f, 64*1024)
		lineno := 0
		ctxLines := opts.ContextLines
		if ctxLines > 20 {
			ctxLines = 20
		}
		var buf []string
		if ctxLines > 0 && !opts.FilesOnly {
			buf = make([]string, 0, ctxLines)
		}
		pendingAfter := 0

		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			raw, readErr := reader.ReadString('\n')
			if len(raw) == 0 && readErr != nil {
				if readErr == io.EOF {
					break
				}
				return readErr
			}
			lineno++
			line := strings.TrimRight(raw, "\r\n")
			if re.MatchString(line) {
				if ctxLines > 0 && !opts.FilesOnly {
					startLine := lineno - len(buf)
					for i, prev := range buf {
						emitLine(rel, startLine+i, prev)
					}
					buf = buf[:0]
				}
				emitLine(rel, lineno, line)
				if opts.FilesOnly {
					return nil
				}
				pendingAfter = ctxLines
				if opts.MaxMatches > 0 && res.Matches >= opts.MaxMatches {
					res.Truncated = true
					return filepath.SkipAll
				}
				continue
			}
			if pendingAfter > 0 {
				emitLine(rel, lineno, line)
				pendingAfter--
				continue
			}
			if ctxLines > 0 && !opts.FilesOnly {
				if len(buf) == ctxLines {
					buf = buf[1:]
				}
				buf = append(buf, line)
			}
			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				return readErr
			}
		}
		return nil
	})

	if walkErr != nil && walkErr != filepath.SkipAll {
		return res, walkErr
	}
	res.Output = out.String()
	res.Files = len(matchedFiles)
	return res, nil
}

// relPathForDisplay mirrors how ripgrep / grep -r render paths: when
// the walk root is a directory we keep the path as-walked (relative
// to the caller's CWD), otherwise we keep the single-file path as
// supplied. Unix and Windows separators both survive unchanged.
func relPathForDisplay(root, path string) string {
	if root == "." || root == "" {
		if strings.HasPrefix(path, "./") {
			return path[2:]
		}
		return path
	}
	return path
}
