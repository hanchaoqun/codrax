package logparse

import (
	"regexp"
	"strconv"
	"strings"
)

// parseJava recognises standard JVM stacktrace output including
// `Caused by:` exception chains and `Suppressed:` blocks.
//
// Canonical shape:
//
//	Exception in thread "main" java.lang.NullPointerException: foo
//		at com.example.Service.run(Service.java:42)
//		at com.example.App.main(App.java:10)
//	Caused by: java.lang.IllegalStateException: bar
//		at com.example.Inner.boom(Inner.java:5)
//
// Frame regex also accepts `(Native Method)` / `(Unknown Source)` —
// those produce a Frame with Line=0 and are useful only for the Func
// entity. Inner-class names ("Foo$Bar") are preserved.
//
// Entry condition: the input must contain at least one line that
// starts (after whitespace) with `at ` and matches the frame regex.
// The `Exception in thread` / `Caused by:` anchors are helpful but
// not required — pasted stacks often omit the header.
var (
	// Qualifier accepts anything up to the last `.<method>(`. The
	// leading `(.+?)` captures JPMS module prefixes like
	// `java.base/java.lang.Thread` that older `[\w$.]+` regexes reject.
	reJavaFrame = regexp.MustCompile(`^\s*at\s+(.+?)\.([\w$<>]+)\(([^)]+)\)\s*$`)
	reJavaHead  = regexp.MustCompile(`^(?:Exception in thread "[^"]+"\s+)?([\w.$]+Exception|[\w.$]+Error)(?::\s*(.*))?$`)
	reJavaCaused = regexp.MustCompile(`^Caused by:\s*([\w.$]+)(?::\s*(.*))?$`)
)

func parseJava(raw string) LogBundle {
	lines := strings.Split(raw, "\n")

	var frames []Frame
	var literals []string
	hasAnchor := false

	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if trim == "" {
			continue
		}

		// Exception header.
		if m := reJavaHead.FindStringSubmatch(trim); m != nil {
			hasAnchor = true
			literals = append(literals, m[1]) // exception type
			if len(m) >= 3 && m[2] != "" {
				literals = append(literals, m[2]) // message
			}
			continue
		}
		// Caused-by.
		if m := reJavaCaused.FindStringSubmatch(trim); m != nil {
			hasAnchor = true
			literals = append(literals, m[1])
			if len(m) >= 3 && m[2] != "" {
				literals = append(literals, m[2])
			}
			continue
		}

		m := reJavaFrame.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		qualifier := m[1] // com.example.Service or com.example.Outer$Inner
		method := m[2]
		fileAndLine := m[3]

		file := fileAndLine
		line := 0
		if idx := strings.LastIndex(fileAndLine, ":"); idx >= 0 {
			file = fileAndLine[:idx]
			if n, err := strconv.Atoi(fileAndLine[idx+1:]); err == nil {
				line = n
			}
		}
		// `(Native Method)` / `(Unknown Source)` land in `file` here;
		// keep them but leave Line=0 so downstream code knows there is
		// no useful file:line.
		if file == "Native Method" || file == "Unknown Source" {
			file = ""
		}

		pkg := ""
		if idx := strings.LastIndex(qualifier, "."); idx >= 0 {
			pkg = qualifier[:idx]
		}
		funcName := qualifier + "." + method

		frames = append(frames, Frame{
			Lang: "java",
			File: file,
			Line: line,
			Func: funcName,
			Pkg:  pkg,
		})
		hasAnchor = true
	}

	if len(frames) == 0 {
		if hasAnchor {
			// Exception header present but no frame — return empty
			// bundle so Detect moves on. Literals alone aren't useful
			// without stack anchors in MVP.
			return LogBundle{}
		}
		return LogBundle{}
	}
	return LogBundle{
		Lang:          "java",
		Frames:        frames,
		ErrorLiterals: literals,
	}
}
