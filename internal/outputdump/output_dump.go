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
	"github.com/hanchaoqun/codrax/internal/preview"
)

// Ext is the suffix used for both the prune glob and new files.
// Hard-coded so the prune sweep never touches unrelated content the
// operator may have placed in the directory.
const Ext = ".md"

// HTMLExt is the sibling browser-ready artifact written next to every
// markdown dump. The markdown file remains the authoritative raw output; the
// HTML is a deterministic system rendering of the same bytes.
const HTMLExt = ".html"

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

// Result reports the best-effort artifacts written for one answer dump. A
// markdown path can be present while HTML is empty when the secondary render
// failed; answer delivery must not depend on either artifact.
type Result struct {
	MarkdownPath string
	HTMLPath     string
}

// Write persists the rendered final answer + the user question to
// <dir>/<timestamp>-<pid>.md and prunes oldest files past the retention
// cap. It returns the written path on success. Best-effort: every IO
// error is logged at WARN and swallowed, because transcript dumping is
// a UX affordance and must never alter answer delivery.
func Write(a Args) string {
	return WriteResult(a).MarkdownPath
}

// WriteResult persists the markdown dump and a sibling self-contained HTML
// rendering. Retention is counted by markdown dumps: pruning an old .md removes
// its .html sibling, and orphaned canonical .html dumps are cleaned on the same
// sweep.
func WriteResult(a Args) Result {
	if a.Dir == "" {
		return Result{}
	}
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		logging.Warning("[output_dump] mkdir %s failed: %v", a.Dir, err)
		return Result{}
	}
	PruneDir(a.Dir, a.Max)
	name := FileName(a.Now, a.PID)
	path := filepath.Join(a.Dir, name)
	body := BuildBody(a)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		logging.Warning("[output_dump] write %s failed: %v", path, err)
		return Result{}
	}
	logging.Info("[output_dump] wrote %s (%d bytes)", path, len(body))
	result := Result{MarkdownPath: path}
	htmlPath := HTMLPathForMarkdown(path)
	if htmlPath == "" {
		return result
	}
	htmlBody, err := BuildHTML(filepath.Base(path), body)
	if err != nil {
		logging.Warning("[output_dump] render html for %s failed: %v", path, err)
		return result
	}
	if err := os.WriteFile(htmlPath, []byte(htmlBody), 0o644); err != nil {
		logging.Warning("[output_dump] write %s failed: %v", htmlPath, err)
		return result
	}
	logging.Info("[output_dump] wrote %s (%d bytes)", htmlPath, len(htmlBody))
	result.HTMLPath = htmlPath
	return result
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

// BuildHTML renders the already-composed markdown dump body into a
// self-contained HTML page. The source markdown remains the canonical output
// artifact; callers use this only for user-facing browser convenience.
func BuildHTML(title, markdownBody string) (string, error) {
	return preview.RenderStandaloneMarkdownHTML(title, []byte(markdownBody))
}

// HTMLPathForMarkdown returns the sibling HTML path for a markdown dump path.
func HTMLPathForMarkdown(markdownPath string) string {
	markdownPath = strings.TrimSpace(markdownPath)
	if markdownPath == "" {
		return ""
	}
	ext := filepath.Ext(markdownPath)
	if strings.EqualFold(ext, Ext) {
		return strings.TrimSuffix(markdownPath, ext) + HTMLExt
	}
	return markdownPath + HTMLExt
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
// Matching .html siblings are removed with their .md files, and orphaned
// canonical .html dumps are cleaned on the same pass so the two artifact types
// keep the same retention shape.
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
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
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
	pruneOrphanHTMLDumps(dir, entries, names)
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
		htmlPath := HTMLPathForMarkdown(files[i].path)
		if htmlPath != "" {
			if err := os.Remove(htmlPath); err != nil && !os.IsNotExist(err) {
				logging.Warning("[output_dump] prune %s failed: %v", htmlPath, err)
			}
		}
	}
}

func pruneOrphanHTMLDumps(dir string, entries []os.DirEntry, names map[string]bool) {
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, HTMLExt) || !isCanonicalDumpName(name, HTMLExt) {
			continue
		}
		mdName := strings.TrimSuffix(name, HTMLExt) + Ext
		if names[mdName] {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logging.Warning("[output_dump] prune %s failed: %v", path, err)
		}
	}
}

func isCanonicalDumpName(name, ext string) bool {
	if !strings.HasSuffix(name, ext) {
		return false
	}
	stem := strings.TrimSuffix(name, ext)
	if len(stem) < len("20060102-150405.000-1") {
		return false
	}
	if !allDigits(stem[0:8]) || stem[8] != '-' ||
		!allDigits(stem[9:15]) || stem[15] != '.' ||
		!allDigits(stem[16:19]) || stem[19] != '-' {
		return false
	}
	return allDigits(stem[20:])
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
