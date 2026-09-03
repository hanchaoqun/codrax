package outputdump

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
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
	// RootCauseJSONPath, when non-empty, receives a guaranteed-delivery
	// machine-readable Trace root-cause artifact. Like the default Trace
	// sibling, this target is written even when no valid model-owned root-cause
	// selection is available; that state is represented by a typed unavailable
	// envelope instead of a missing file or a fabricated empty conclusion.
	RootCauseJSONPath string
	// SuppressDefaultDir skips every write into Args.Dir (no MkdirAll, no
	// prune, no dump files) while the explicit paths above still write.
	// cmd sets it when output_dump_enabled=false but explicit report
	// paths were requested: the explicit CLI request overrides the
	// default dump gate without re-enabling the default dump.
	SuppressDefaultDir bool
}

func (r ExplicitReport) hasTarget() bool {
	return strings.TrimSpace(r.MarkdownPath) != "" || strings.TrimSpace(r.HTMLPath) != "" ||
		strings.TrimSpace(r.RootCauseJSONPath) != ""
}

const ExplicitRootCauseArtifactSchemaVersion = 1

// ExplicitRootCauseStatus* are DELIVERY states of the default artifact
// ("a valid model selection was persisted" / "none was"), never a causal-proof
// assertion: per-item causal truth rides `causal_qualifier` on each root cause
// (SIDECAR-Q1, colleague_merge_audit §40.28 ②).
const (
	ExplicitRootCauseStatusAvailable   = "available"
	ExplicitRootCauseStatusUnavailable = "unavailable"
)

// ExplicitRootCauseArtifact is the stable guaranteed-delivery envelope used
// only by --root-causes-out. The mandatory timestamped default sibling keeps
// top-level TraceRootCauseReportV2 fields plus availability metadata. Keeping availability
// outside the report prevents a missing model selection from being serialized
// as root_causes=[] and misread as a model conclusion that no cause exists.
type ExplicitRootCauseArtifact struct {
	ArtifactSchemaVersion int                           `json:"artifact_schema_version"`
	Status                string                        `json:"status"`
	TraceRootCauses       *types.TraceRootCauseReportV2 `json:"trace_root_causes,omitempty"`
	ReasonCode            string                        `json:"reason_code,omitempty"`
}

// ExplicitReportWrite records one attempted explicit-report write so the
// CLI can print an honest post-run status line (mirroring the --plan-out
// "[plan written: …]" / "plan file write failed" precedent). Err == nil
// means the file landed on disk.
type ExplicitReportWrite struct {
	Kind string // "markdown" | "html" | "root-causes"
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
func writeExplicitReportCopies(r ExplicitReport, body string, rootCauses *types.TraceRootCauseReportV2, unavailableReason string) string {
	if p := strings.TrimSpace(r.MarkdownPath); p != "" {
		recordExplicitReportWrite(ExplicitReportWrite{
			Kind: "markdown",
			Path: p,
			Err:  writeExplicitReportFile(p, []byte(body)),
		})
	}
	p := strings.TrimSpace(r.HTMLPath)
	if p != "" {
		htmlBody, err := BuildHTML(filepath.Base(p), body)
		if err != nil {
			logging.Warning("[output_dump] render html for explicit report %s failed: %v", p, err)
			recordExplicitReportWrite(ExplicitReportWrite{Kind: "html", Path: p, Err: err})
		} else {
			recordExplicitReportWrite(ExplicitReportWrite{
				Kind: "html",
				Path: p,
				Err:  writeExplicitReportFile(p, []byte(htmlBody)),
			})
		}
	}
	write := writeExplicitRootCauseArtifact(r, rootCauses, unavailableReason)
	if write.Err == nil {
		return write.Path
	}
	return ""
}

// EnsureExplicitRootCauseArtifact closes the no-final-answer lane. Normal
// final-answer dumping writes the explicit artifact through WriteResult; if a
// pipeline exits before that hook, the CLI calls this function at the outer
// boundary so an explicitly requested path still receives a typed unavailable
// artifact. It never derives a cause or reads model prose.
func EnsureExplicitRootCauseArtifact(unavailableReason string) {
	r := explicitReportSnapshot()
	if strings.TrimSpace(r.RootCauseJSONPath) == "" || explicitRootCauseWriteRecorded() {
		return
	}
	writeExplicitRootCauseArtifact(r, nil, unavailableReason)
}

func explicitRootCauseWriteRecorded() bool {
	explicitReportMu.Lock()
	defer explicitReportMu.Unlock()
	for _, write := range explicitReportWrites {
		if write.Kind == "root-causes" {
			return true
		}
	}
	return false
}

func writeExplicitRootCauseArtifact(r ExplicitReport, report *types.TraceRootCauseReportV2, unavailableReason string) ExplicitReportWrite {
	p := strings.TrimSpace(r.RootCauseJSONPath)
	if p == "" {
		return ExplicitReportWrite{}
	}
	artifact := ExplicitRootCauseArtifact{
		ArtifactSchemaVersion: ExplicitRootCauseArtifactSchemaVersion,
		Status:                ExplicitRootCauseStatusAvailable,
		TraceRootCauses:       report,
	}
	if report == nil {
		artifact.Status = ExplicitRootCauseStatusUnavailable
		artifact.ReasonCode = strings.TrimSpace(unavailableReason)
		if artifact.ReasonCode == "" {
			artifact.ReasonCode = RootCauseReasonFallbackUnavailable
		}
	}
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err == nil {
		body = append(body, '\n')
		err = writeExplicitReportFile(p, body)
	} else {
		err = fmt.Errorf("encode explicit root-cause artifact: %w", err)
		logging.Warning("[output_dump] %v", err)
	}
	write := ExplicitReportWrite{Kind: "root-causes", Path: p, Err: err}
	recordExplicitReportWrite(write)
	return write
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
