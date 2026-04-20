package logparse

import (
	"regexp"
	"strconv"
	"strings"
)

// parseNode recognises V8 / Node.js stack trace output.
//
// Canonical shape:
//
//	TypeError: Cannot read property 'x' of undefined
//	    at Object.handler (/src/repo/app.js:42:13)
//	    at async main (/src/repo/main.js:10:5)
//	    at Module._compile (node:internal/modules/cjs/loader:1105:14)
//	    at /src/repo/cli.js:3:1
//
// Frames accept three variants:
//
//  1. `at <func> (<file>:<line>:<col>)`          — named function
//  2. `at async <func> (<file>:<line>:<col>)`    — async function
//  3. `at <file>:<line>:<col>`                    — anonymous / top-level
//
// `node:internal/...` frames are captured (they tell us which
// runtime surface was hit) but are not useful as RequiredFiles seeds
// because the path is a V8-virtual module, not a repo file. We keep
// them in Frames for completeness; the resolver tier drops them.
//
// Entry condition: input must contain at least one line matching one
// of the three frame shapes. The `<ErrType>: <msg>` first-line header
// is common but not required.
var (
	// `[^()]+` in the file group accepts `node:internal/...` URIs
	// whose path segment contains colons; the trailing `:(\d+):(\d+)`
	// anchors force RE2 to backtrack the file group to the longest
	// non-numeric-suffix prefix.
	reNodeFrameNamed     = regexp.MustCompile(`^\s*at\s+(?:async\s+)?(\S[^(]*?)\s+\(([^()]+):(\d+):(\d+)\)\s*$`)
	reNodeFrameAnonymous = regexp.MustCompile(`^\s*at\s+([^\s()]+):(\d+):(\d+)\s*$`)
	reNodeErrHead       = regexp.MustCompile(`^([\w.]+Error|[\w.]+Exception):\s*(.+)$`)
)

func parseNode(raw string) LogBundle {
	lines := strings.Split(raw, "\n")

	var frames []Frame
	var literals []string

	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if trim == "" {
			continue
		}

		// Error header: typically the first non-indented line.
		if !strings.HasPrefix(ln, " ") && !strings.HasPrefix(ln, "\t") {
			if m := reNodeErrHead.FindStringSubmatch(trim); m != nil {
				literals = append(literals, m[1])
				if m[2] != "" {
					literals = append(literals, strings.TrimSpace(m[2]))
				}
				continue
			}
		}

		if m := reNodeFrameNamed.FindStringSubmatch(ln); m != nil {
			line, _ := strconv.Atoi(m[3])
			frames = append(frames, Frame{
				Lang: "node",
				File: m[2],
				Line: line,
				Func: strings.TrimSpace(m[1]),
			})
			continue
		}
		if m := reNodeFrameAnonymous.FindStringSubmatch(ln); m != nil {
			line, _ := strconv.Atoi(m[2])
			frames = append(frames, Frame{
				Lang: "node",
				File: m[1],
				Line: line,
			})
			continue
		}
	}

	if len(frames) == 0 {
		return LogBundle{}
	}
	// Basic sanity — the frame regexes are liberal. A match without
	// any of `.js` / `.ts` / `.mjs` / `.cjs` / `node:` in at least
	// one frame is suspicious and likely a false positive on another
	// language's stack. Require at least one Node-flavoured file.
	nodeFlavoured := false
	for _, f := range frames {
		low := strings.ToLower(f.File)
		if strings.HasSuffix(low, ".js") ||
			strings.HasSuffix(low, ".ts") ||
			strings.HasSuffix(low, ".mjs") ||
			strings.HasSuffix(low, ".cjs") ||
			strings.HasPrefix(f.File, "node:") {
			nodeFlavoured = true
			break
		}
	}
	if !nodeFlavoured {
		return LogBundle{}
	}
	return LogBundle{
		Lang:          "node",
		Frames:        frames,
		ErrorLiterals: literals,
	}
}
