package logparse

import (
	"regexp"
	"strconv"
	"strings"
)

// parseCppGDB recognises gdb `bt` / core-dump output.
//
// Canonical shape:
//
//	#0  0x00007f8c in Foo::bar (this=0x12, x=5) at /src/foo.cpp:42
//	#1  0x00007f8d in main () at /src/main.cpp:10
//	#2  0x00007f8e in __libc_start_main (...) from /lib/libc.so.6
//
// Key anchor: " at <file>:<line>" AFTER the function signature. The
// `from /lib/...` form (no debug info, only shared-library path) is
// skipped because it has no line number and no repo-local source to
// point at.
//
// Entry condition: must find at least one line matching the full
// regex. parseCppASAN runs first in Detect's priority and will claim
// any line that looks ASAN-shaped, so we will not see sanitizer
// output here.
var (
	reGDBFrame = regexp.MustCompile(`^#\d+\s+0x[0-9a-fA-F]+\s+in\s+(.+?)\s+\([^)]*\)\s+at\s+(\S+):(\d+)\s*$`)
)

func parseCppGDB(raw string) LogBundle {
	if !strings.Contains(raw, " at ") || !strings.Contains(raw, "#") {
		return LogBundle{}
	}
	lines := strings.Split(raw, "\n")

	var frames []Frame
	var literals []string

	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if trim == "" {
			continue
		}

		// Optional crash-banner lines. gdb prints a signal summary
		// like "Program received signal SIGSEGV, Segmentation fault."
		// on core-dump open — treat those as literals.
		if strings.HasPrefix(trim, "Program received signal") ||
			strings.HasPrefix(trim, "Thread ") && strings.Contains(trim, "received signal") {
			literals = append(literals, trim)
			continue
		}

		m := reGDBFrame.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		funcName := strings.TrimSpace(m[1])
		file := m[2]
		line, _ := strconv.Atoi(m[3])

		frames = append(frames, Frame{
			Lang: "cpp",
			File: file,
			Line: line,
			Func: funcName,
		})
	}

	if len(frames) == 0 {
		return LogBundle{}
	}
	return LogBundle{
		Lang:          "cpp",
		Frames:        frames,
		ErrorLiterals: literals,
	}
}
