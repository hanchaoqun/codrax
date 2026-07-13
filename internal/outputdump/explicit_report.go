package outputdump

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// ExplicitReport carries CLI-requested extra output paths for the final
// answer report (--report-md / --report-html). It is registered once by
// cmd before the run and consumed by WriteResult, which writes the SAME
// markdown bytes (and the same RenderStandaloneMarkdownHTML product) it
// writes to the default .codrax/output dump. Paths are used verbatim:
// no timestamp is appended, missing parent directories are created, and
// an existing file is overwritten — an explicit user path is explicit
// intent.
type ExplicitReport struct {
	// MarkdownPath, when non-empty, receives a byte-identical copy of the
	// default dump's markdown body.
	MarkdownPath string
	// HTMLPath, when non-empty, receives the standalone HTML rendering of
	// the same markdown body (same BuildHTML/RenderStandaloneMarkdownHTML
	// pipeline as the default dump's .html sibling; the embedded <title>
	// is this file's base name).
	HTMLPath string
	// SuppressDefaultDir skips every write into Args.Dir (no MkdirAll, no
	// prune, no dump files) while the explicit paths above still write.
	// cmd sets it when output_dump_enabled=false but explicit report
	// paths were requested: the explicit CLI request overrides the
	// default dump gate without re-enabling the default dump.
	SuppressDefaultDir bool
}

func (r ExplicitReport) hasTarget() bool {
	return strings.TrimSpace(r.MarkdownPath) != "" || strings.TrimSpace(r.HTMLPath) != ""
}

// ExplicitReportWrite records one attempted explicit-report write so the
// CLI can print an honest post-run status line (mirroring the --plan-out
// "[plan written: …]" / "plan file write failed" precedent). Err == nil
// means the file landed on disk.
type ExplicitReportWrite struct {
	Kind string // "markdown" | "html"
	Path string
	Err  error
}

var (
	explicitReportMu     sync.Mutex
	explicitReportCfg    ExplicitReport
	explicitReportWrites []ExplicitReportWrite
)

// SetExplicitReport installs (or, with a zero value, clears) the explicit
// report targets and resets the recorded write statuses. Best-effort dump
// semantics are unchanged when no target is set.
func SetExplicitReport(r ExplicitReport) {
	explicitReportMu.Lock()
	defer explicitReportMu.Unlock()
	explicitReportCfg = r
	explicitReportWrites = nil
}

// ExplicitReportWrites returns a snapshot of the write attempts recorded
// since the last SetExplicitReport call. Empty means WriteResult never ran
// with explicit targets configured (e.g. the run produced no final-answer
// transcript).
func ExplicitReportWrites() []ExplicitReportWrite {
	explicitReportMu.Lock()
	defer explicitReportMu.Unlock()
	return append([]ExplicitReportWrite(nil), explicitReportWrites...)
}

func explicitReportSnapshot() ExplicitReport {
	explicitReportMu.Lock()
	defer explicitReportMu.Unlock()
	return explicitReportCfg
}

func recordExplicitReportWrite(w ExplicitReportWrite) {
	explicitReportMu.Lock()
	defer explicitReportMu.Unlock()
	explicitReportWrites = append(explicitReportWrites, w)
}

// writeExplicitReportCopies writes the explicit report targets from the
// single already-composed markdown body. The two targets are independent:
// a markdown failure never blocks the HTML write and vice versa. Failures
// are logged at WARN with the offending path and recorded for the CLI
// status surface — like the default dump, they must never alter answer
// delivery.
func writeExplicitReportCopies(r ExplicitReport, body string) {
	if p := strings.TrimSpace(r.MarkdownPath); p != "" {
		recordExplicitReportWrite(ExplicitReportWrite{
			Kind: "markdown",
			Path: p,
			Err:  writeExplicitReportFile(p, []byte(body)),
		})
	}
	p := strings.TrimSpace(r.HTMLPath)
	if p == "" {
		return
	}
	htmlBody, err := BuildHTML(filepath.Base(p), body)
	if err != nil {
		logging.Warning("[output_dump] render html for explicit report %s failed: %v", p, err)
		recordExplicitReportWrite(ExplicitReportWrite{Kind: "html", Path: p, Err: err})
		return
	}
	recordExplicitReportWrite(ExplicitReportWrite{
		Kind: "html",
		Path: p,
		Err:  writeExplicitReportFile(p, []byte(htmlBody)),
	})
}

func writeExplicitReportFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logging.Warning("[output_dump] mkdir %s for explicit report %s failed: %v", dir, path, err)
			return err
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logging.Warning("[output_dump] write explicit report %s failed: %v", path, err)
		return err
	}
	logging.Info("[output_dump] wrote explicit report %s (%d bytes)", path, len(data))
	return nil
}
