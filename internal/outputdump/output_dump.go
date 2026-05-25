package outputdump

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// Ext is the suffix used for both the prune glob and new files.
// Hard-coded so the prune sweep never touches unrelated content the
// operator may have placed in the directory.
const Ext = ".md"

// Args bundles the inputs Write needs. Every caller supplies the raw
// user request text and the raw markdown answer body that reached the
// user's visible answer surface. Terminal chrome, ANSI styling, and
// preview hints are intentionally excluded.
type Args struct {
	Dir        string
	Max        int
	Request    string
	Answer     string
	HasLog     bool
	LogBytes   int
	HasTrace   bool
	TraceBytes int
	Now        time.Time
	PID        int
}

// Write persists the rendered final answer + the user question to
// <dir>/<timestamp>-<pid>.md and prunes oldest files past the retention
// cap. It returns the written path on success. Best-effort: every IO
// error is logged at WARN and swallowed, because transcript dumping is
// a UX affordance and must never alter answer delivery.
func Write(a Args) string {
	if a.Dir == "" {
		return ""
	}
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		logging.Warning("[output_dump] mkdir %s failed: %v", a.Dir, err)
		return ""
	}
	PruneDir(a.Dir, a.Max)
	name := FileName(a.Now, a.PID)
	path := filepath.Join(a.Dir, name)
	body := BuildBody(a)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		logging.Warning("[output_dump] write %s failed: %v", path, err)
		return ""
	}
	logging.Info("[output_dump] wrote %s (%d bytes)", path, len(body))
	return path
}

// FileName returns the canonical filename, matching tool.SessionDirName's
// `YYYYMMDD-HHMMSS.mmm-<pid>` shape so dump files line up 1:1 with
// .codrax/blob/<id>/ session dirs when the blob layout is enabled.
func FileName(now time.Time, pid int) string {
	if now.IsZero() {
		now = time.Now()
	}
	if pid == 0 {
		pid = os.Getpid()
	}
	return fmt.Sprintf("%s-%d%s",
		now.Format("20060102-150405.000"),
		pid,
		Ext,
	)
}

// BuildBody composes the two-section markdown body. No frontmatter: by
// user contract the file carries exactly two H1 sections. Attachments
// surface as quoted footnote lines under the request so reproduction
// context is captured without bloating the header.
func BuildBody(a Args) string {
	var b strings.Builder
	b.WriteString("# 问题\n\n")
	req := strings.TrimRight(a.Request, "\n")
	if req == "" {
		req = "(empty)"
	}
	b.WriteString(req)
	b.WriteString("\n")
	if a.HasLog {
		fmt.Fprintf(&b, "\n> 附件: log (%s)\n", HumanBytes(a.LogBytes))
	}
	if a.HasTrace {
		fmt.Fprintf(&b, "\n> 附件: htrace (%s)\n", HumanBytes(a.TraceBytes))
	}
	b.WriteString("\n# 回答\n\n")
	ans := strings.TrimRight(a.Answer, "\n")
	if ans == "" {
		ans = "(empty)"
	}
	ans = mermaidcompat.NormalizeMarkdownMermaidFences(ans)
	b.WriteString(ans)
	b.WriteString("\n")
	return b.String()
}

// HumanBytes formats a byte count for the attachment footnote.
// Coarse-grained on purpose: the dump line is informational, not a
// precise measurement.
func HumanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// PruneDir keeps the most-recent max-1 *.md files under dir (by mtime),
// reserving one slot for the incoming write. max <= 0 disables pruning.
// Errors abort the sweep silently; retention is hygiene, not correctness.
func PruneDir(dir string, max int) {
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) {
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
	keepExisting := max - 1
	if keepExisting < 0 {
		keepExisting = 0
	}
	if len(files) <= keepExisting {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].mod.Equal(files[j].mod) {
			return files[i].path < files[j].path
		}
		return files[i].mod.Before(files[j].mod)
	})
	for i := 0; i < len(files)-keepExisting; i++ {
		if err := os.Remove(files[i].path); err != nil {
			logging.Warning("[output_dump] prune %s failed: %v", files[i].path, err)
		}
	}
}
