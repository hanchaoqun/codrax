package logparse

import (
	"regexp"
	"strconv"
	"strings"
)

// parsePython recognises the standard CPython traceback format.
//
// Canonical shape:
//
//	Traceback (most recent call last):
//	  File "/src/repo/app.py", line 42, in handle
//	    result = compute(x)
//	  File "/src/repo/util.py", line 10, in compute
//	    raise ValueError("bad input")
//	ValueError: bad input
//
// Frames appear as two-line pairs: a `File "..."` line followed by the
// source snippet (which we ignore). The final non-indented line is the
// exception type + message.
//
// Entry condition: the input must contain a Traceback header. Bare
// exceptions without the header are rejected so one-liners like
// "ValueError: oops" do not trigger the root-cause workflow — they
// carry no actionable file:line anchor.
var (
	rePythonFrame = regexp.MustCompile(`^\s*File\s+"([^"]+)",\s+line\s+(\d+)(?:,\s+in\s+(\S+))?\s*$`)
	rePythonExc   = regexp.MustCompile(`^([\w.]+Error|[\w.]+Exception|[\w.]+Warning):\s*(.*)$`)
)

func parsePython(raw string) LogBundle {
	if !strings.Contains(raw, "Traceback (most recent call last):") &&
		!strings.Contains(raw, "Traceback (most recent call first):") {
		return LogBundle{}
	}
	lines := strings.Split(raw, "\n")

	var frames []Frame
	var literals []string

	for _, ln := range lines {
		if m := rePythonFrame.FindStringSubmatch(ln); m != nil {
			line, _ := strconv.Atoi(m[2])
			funcName := ""
			if len(m) >= 4 {
				funcName = m[3]
			}
			frames = append(frames, Frame{
				Lang: "python",
				File: m[1],
				Line: line,
				Func: funcName,
			})
			continue
		}
		trim := strings.TrimSpace(ln)
		if trim == "" {
			continue
		}
		// Only unindented lines can be the exception tail.
		if len(ln) > 0 && (ln[0] == ' ' || ln[0] == '\t') {
			continue
		}
		if m := rePythonExc.FindStringSubmatch(trim); m != nil {
			literals = append(literals, m[1])
			if m[2] != "" {
				literals = append(literals, m[2])
			}
		}
	}

	if len(frames) == 0 {
		return LogBundle{}
	}
	return LogBundle{
		Lang:          "python",
		Frames:        frames,
		ErrorLiterals: literals,
	}
}
