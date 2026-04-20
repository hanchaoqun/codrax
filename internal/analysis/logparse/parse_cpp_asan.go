package logparse

import (
	"regexp"
	"strconv"
	"strings"
)

// parseCppASAN recognises AddressSanitizer and UndefinedBehaviorSanitizer
// diagnostic output. Both sanitizers share the same header and frame
// shape, differing only in the error class on the SUMMARY line.
//
// Canonical shape:
//
//	==12345==ERROR: AddressSanitizer: heap-use-after-free on address 0x...
//	READ of size 4 at 0x... thread T0
//	    #0 0x401234 in Foo::bar() /src/repo/foo.cpp:42:5
//	    #1 0x401567 in main /src/repo/main.cpp:10:3
//	    #2 0x401890 in __libc_start_main /build/glibc/.../libc-start.c:308
//
// Frame regex accepts the `func file:line(:col)?` shape and rejects
// the gdb-style `func at file:line` shape so parseCppGDB can claim
// those lines on a later pass. Column numbers are dropped.
//
// Entry condition: the input must contain an ASAN / UBSAN anchor. We
// accept both the full `==NNN==ERROR: AddressSanitizer:` header and
// the shorter `SUMMARY: AddressSanitizer:` form some tools emit. For
// UBSAN we also accept `runtime error:` which is the canonical UBSAN
// prefix.
var (
	reSanitFrame = regexp.MustCompile(`^\s*#\d+\s+0x[0-9a-fA-F]+\s+in\s+(\S.*?)\s+(\S+?):(\d+)(?::\d+)?\s*$`)
)

func parseCppASAN(raw string) LogBundle {
	hasHeader := strings.Contains(raw, "AddressSanitizer") ||
		strings.Contains(raw, "UndefinedBehaviorSanitizer") ||
		strings.Contains(raw, "LeakSanitizer") ||
		strings.Contains(raw, "ThreadSanitizer") ||
		strings.Contains(raw, "MemorySanitizer")
	if !hasHeader {
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

		// Diagnostic header / summary lines → error literals.
		if strings.Contains(trim, "ERROR: AddressSanitizer:") ||
			strings.Contains(trim, "ERROR: UndefinedBehaviorSanitizer:") ||
			strings.Contains(trim, "ERROR: LeakSanitizer:") ||
			strings.Contains(trim, "ERROR: ThreadSanitizer:") ||
			strings.Contains(trim, "ERROR: MemorySanitizer:") ||
			strings.Contains(trim, "SUMMARY: ") ||
			strings.Contains(trim, "runtime error:") {
			literals = append(literals, trim)
			// fall through — the header line itself is not a frame
		}

		// Reject gdb-style frames (those contain " at " between func
		// and file). parseCppGDB claims them on its own pass.
		if strings.Contains(ln, " at ") && reSanitFrame.FindStringSubmatchIndex(ln) != nil {
			// Double-check: if the regex still matches AND " at " is
			// inside the func portion, accept as ASAN — but it's
			// safer to defer to gdb in ambiguous cases.
			continue
		}
		m := reSanitFrame.FindStringSubmatch(ln)
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
