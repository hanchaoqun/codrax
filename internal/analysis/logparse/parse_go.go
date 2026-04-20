package logparse

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// parseGo recognises the Go runtime panic / goroutine stack format.
//
// Canonical shape:
//
//	panic: runtime error: index out of range [5] with length 3
//
//	goroutine 1 [running]:
//	main.crashyThing(...)
//		/src/repo/main.go:42 +0x1e
//	runtime.goexit()
//		/usr/local/go/src/runtime/asm_amd64.s:1571 +0x1
//
// The function/file pair is always on two consecutive lines: a
// function signature (no leading whitespace) followed by a tab-
// indented "<abs-or-rel path>:<line> +0x<hex>". We walk the input
// line by line, detecting function lines and peeking at the next line
// for its matching file:line.
//
// Entry condition: the input must contain `goroutine ` followed by a
// digit and `[` — the canonical stack header that no other language
// emits. A bare "panic: foo" without a goroutine stack is rejected
// (no frames to extract) so Detect can try other parsers.
var (
	reGoFileLine = regexp.MustCompile(`^\t(.+?):(\d+) \+0x[0-9a-fA-F]+$`)
	reGoFuncLine = regexp.MustCompile(`^([A-Za-z0-9_./\-\*\(\)]+)\(.*\)$`)
)

func parseGo(raw string) LogBundle {
	if !strings.Contains(raw, "goroutine ") || !strings.Contains(raw, "[") {
		return LogBundle{}
	}
	lines := strings.Split(raw, "\n")

	var frames []Frame
	var literals []string

	// Error literals: every line starting with "panic:" (the runtime
	// wrote it) and every line starting with "fatal error:".
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "panic:") {
			msg := strings.TrimSpace(strings.TrimPrefix(trim, "panic:"))
			if msg != "" {
				literals = append(literals, msg)
			}
		}
		if strings.HasPrefix(trim, "fatal error:") {
			msg := strings.TrimSpace(strings.TrimPrefix(trim, "fatal error:"))
			if msg != "" {
				literals = append(literals, msg)
			}
		}
	}

	for i := 0; i+1 < len(lines); i++ {
		funcLine := lines[i]
		fileLine := lines[i+1]

		// Quick reject: the file line must be tab-indented.
		if !strings.HasPrefix(fileLine, "\t") {
			continue
		}
		fileMatch := reGoFileLine.FindStringSubmatch(fileLine)
		if fileMatch == nil {
			continue
		}
		// Function line must not start with whitespace and must look
		// like a function signature. Skip `created by ...` prelude
		// lines — they are informational and point at the wrong frame.
		if funcLine == "" || funcLine[0] == '\t' || funcLine[0] == ' ' {
			continue
		}
		if strings.HasPrefix(funcLine, "created by ") {
			continue
		}

		funcName := funcLine
		if m := reGoFuncLine.FindStringSubmatch(funcLine); m != nil {
			funcName = m[1]
		}

		line, _ := strconv.Atoi(fileMatch[2])
		frames = append(frames, Frame{
			Lang: "go",
			File: fileMatch[1],
			Line: line,
			Func: funcName,
			Pkg:  goPkgFromFile(fileMatch[1]),
		})
		// The file line is consumed; skip over it on the next
		// iteration so it is not re-examined as a function line.
		i++
	}

	if len(frames) == 0 {
		return LogBundle{ErrorLiterals: literals}
	}
	return LogBundle{
		Lang:          "go",
		Frames:        frames,
		ErrorLiterals: literals,
	}
}

// goPkgFromFile returns the dir portion of a Go source path as the
// best-effort "package" hint. For "/src/repo/internal/agent/foo.go"
// this returns "internal/agent" when the path contains "/internal/",
// otherwise the immediate parent directory name. Used by the analyzer
// to seed AnalyzerHints.Entities with the package path so
// domain-boost ranking in keyword_search picks up sibling files.
func goPkgFromFile(path string) string {
	if path == "" {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	if idx := strings.LastIndex(dir, "/internal/"); idx >= 0 {
		return dir[idx+1:] // "internal/agent"
	}
	if dir == "." || dir == "/" || dir == "" {
		return ""
	}
	// Fall back to the last two path segments when available.
	parts := strings.Split(dir, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return parts[len(parts)-1]
}
