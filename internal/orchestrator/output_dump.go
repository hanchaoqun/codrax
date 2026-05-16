package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// outputDumpExt is the suffix used for both the prune glob and new
// files. Hard-coded so the prune sweep never touches unrelated content
// the operator may have placed in the directory.
const outputDumpExt = ".md"

// dumpFinalOutputArgs bundles the inputs writeFinalOutputDump needs.
// Pulled into a struct so the caller (recordTaskFinalize) reads as one
// site instead of an eight-arg function call.
type dumpFinalOutputArgs struct {
	dir      string // absolute directory, must already exist or be MkdirAll-able
	max      int    // retention cap; <=0 disables prune
	request  string // raw user request text (the "# 问题" body)
	answer   string // rendered markdown body (the "# 回答" body)
	hasLog   bool   // --log / /log was attached
	logBytes int    // length of attached log payload (0 when hasLog=false)
	hasTrace bool   // --htrace / --atrace / /htrace was attached
	traceB   int    // length of attached trace payload
	now      time.Time
	pid      int
}

// writeFinalOutputDump persists the rendered final answer + the user
// question to <dir>/<timestamp>-<pid>.md and prunes oldest files past
// the retention cap. It returns the written path on success. Best-
// effort: every IO error is logged at WARN and swallowed — the dump
// must never block or alter the user-facing answer path.
func writeFinalOutputDump(a dumpFinalOutputArgs) string {
	if a.dir == "" {
		return ""
	}
	if err := os.MkdirAll(a.dir, 0o755); err != nil {
		logging.Warning("[output_dump] mkdir %s failed: %v", a.dir, err)
		return ""
	}
	pruneOutputDumpDir(a.dir, a.max)
	name := outputDumpFileName(a.now, a.pid)
	path := filepath.Join(a.dir, name)
	body := buildOutputDumpBody(a)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		logging.Warning("[output_dump] write %s failed: %v", path, err)
		return ""
	}
	logging.Info("[output_dump] wrote %s (%d bytes)", path, len(body))
	return path
}

// outputDumpFileName returns the canonical filename for a dump,
// matching tool.SessionDirName's `YYYYMMDD-HHMMSS-mmm-<pid>` shape so
// dump files line up 1:1 with .codrax/blob/<id>/ session dirs when the
// blob layout is enabled.
func outputDumpFileName(now time.Time, pid int) string {
	return fmt.Sprintf("%s-%d%s",
		now.Format("20060102-150405.000"),
		pid,
		outputDumpExt,
	)
}

// buildOutputDumpBody composes the two-section markdown body. No
// frontmatter — by user contract the file carries exactly two H1
// sections. Attachments surface as quoted footnote lines under the
// request so reproduction context is captured without bloating the
// header.
func buildOutputDumpBody(a dumpFinalOutputArgs) string {
	var b strings.Builder
	b.WriteString("# 问题\n\n")
	req := strings.TrimRight(a.request, "\n")
	if req == "" {
		req = "(empty)"
	}
	b.WriteString(req)
	b.WriteString("\n")
	if a.hasLog {
		fmt.Fprintf(&b, "\n> 附件: log (%s)\n", humanBytes(a.logBytes))
	}
	if a.hasTrace {
		fmt.Fprintf(&b, "\n> 附件: htrace (%s)\n", humanBytes(a.traceB))
	}
	b.WriteString("\n# 回答\n\n")
	ans := strings.TrimRight(a.answer, "\n")
	if ans == "" {
		ans = "(empty)"
	}
	b.WriteString(ans)
	b.WriteString("\n")
	return b.String()
}

// humanBytes formats a byte count for the attachment footnote.
// Coarse-grained on purpose: the dump line is informational, not a
// precise measurement.
func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// pruneOutputDumpDir keeps the most-recent `max` *.md files under dir
// (by mtime) and deletes the rest. max <= 0 disables. Errors abort the
// sweep silently; retention is hygiene, not correctness.
func pruneOutputDumpDir(dir string, max int) {
	if max <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type entry struct {
		path string
		mod  time.Time
	}
	files := make([]entry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), outputDumpExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, entry{
			path: filepath.Join(dir, e.Name()),
			mod:  info.ModTime(),
		})
	}
	// Reserve a slot for the file we are about to write: we delete
	// down to `max-1` so the directory ends at exactly `max` post-write.
	target := max - 1
	if target < 0 {
		target = 0
	}
	if len(files) <= target {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].mod.Before(files[j].mod)
	})
	excess := len(files) - target
	for i := 0; i < excess; i++ {
		if err := os.Remove(files[i].path); err != nil {
			logging.Warning("[output_dump] prune %s failed: %v", files[i].path, err)
		}
	}
}
