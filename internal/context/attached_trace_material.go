package context

import (
	stdcontext "context"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

// A prepared preview is not a replacement trace file. Keep the full material
// path on the typed transport and never overwrite attached_trace.txt with it.
func formatPreparedTrace(raw string, state attachedRuntimeTriageState, degradedSummary string, opts attachedTraceRenderOptions) string {
	if err := opts.Material.Validate(stdcontext.Background(), raw); err != nil {
		return "The attached Trace source or its converted materials changed or became unavailable. Reattach the original source before deriving new evidence; the old preview is not a usable substitute.\n"
	}
	info := preparedTraceBundlePromptInfo(opts.Material.QueryPath())
	// The bundle reader and source receipt must describe the same generation.
	if err := opts.Material.Validate(stdcontext.Background(), raw); err != nil {
		return "The attached Trace materials changed while reading their metadata. Reattach the original source before deriving new evidence.\n"
	}
	opts.SampleOnly = info.sampleOnly
	preamble := info.text + attachedTracePreamble(state, opts)
	if state == attachedTriageUnavailable {
		preamble += degradedTriageNote(degradedSummary)
	}
	preamble += fmt.Sprintf("Original Trace source: `%s`. Complete query material: `%s`. The text below is only a bounded preview, not the complete trace; missing rows in this preview do not prove event absence. Preview line numbers are preview-local, not physical evidence coordinates. Conversion alone proves no scheduling, wakeup or frame-causal coverage; use the capabilities and coverage returned by queries.\n", opts.Material.SourcePath(), opts.Material.QueryPath())
	if opts.PreferTraceQuery {
		preamble += "Use trace_query with source=\"attached_trace\" and omit path; it reads the complete prepared material, including events beyond this preview. Preserve explicit time/line bounds and read query-returned evidence coordinates.\n"
	} else if opts.ReadFileAvailable {
		preamble += "Read the complete query material for additional facts; if it is a bundle manifest, its members retain separate source identities and capabilities.\n"
	} else {
		preamble += "This stage has no trace query or raw-file reader. Record only visible observations and leave unobserved regions for subsequent evidence collection.\n"
	}
	if len(raw) <= attachedLogInlineCap {
		return preamble + "\n```text\n" + renderAttachedArtifactLines(raw, 1) + "\n```"
	}
	return preamble + "\n" + renderAttachedArtifactPreviewBlock(buildAttachedArtifactPreview(raw), "")
}

// Reuse existing bounded capability disclosure from the complete, held bundle
// metadata, never from the clipped text preview or a guessed filename class.
type preparedTracePromptInfo struct {
	text       string
	sampleOnly bool
}

func preparedTraceBundlePromptInfo(path string) preparedTracePromptInfo {
	if !strings.HasSuffix(strings.ToLower(path), ".tracebundle.json") {
		return preparedTracePromptInfo{}
	}
	snapshot, err := tracebundle.Open(stdcontext.Background(), path)
	if err != nil {
		return preparedTracePromptInfo{}
	}
	var metadata attachedTraceBundleMetadata
	err = snapshot.Decode(&metadata)
	validErr := snapshot.Validate()
	closeErr := snapshot.Close()
	if err != nil || validErr != nil || closeErr != nil {
		return preparedTracePromptInfo{}
	}
	var b strings.Builder
	b.WriteString("The complete query material is a tracebundle, not just the visible text preview. Manifest counters are disclosures until trace_query reconciles each artifact with its receipt; absence from the preview does not show absence from the bundle.\n")
	b.WriteString(strings.Join(attachedTraceBundleManifestPartsFromMetadata(path, metadata), " "))
	b.WriteString("\n")
	groups := []hitraceconv.PerfCaptureArtifactGroup{{Scope: path, Artifacts: metadata.Artifacts}}
	for _, disclosure := range hitraceconv.PerfCaptureDisclosuresForGroups(groups) {
		if boundary := hitraceconv.FormatPerfCapturePromptBoundary("en", disclosure); boundary != "" {
			b.WriteString(boundary + "\n")
		}
		if next := hitraceconv.FormatPerfCaptureNextBoundary("en", disclosure); next != "" {
			b.WriteString(next + "\n")
		}
	}
	// This is a soft navigation restriction from validated, complete bundle
	// metadata, not a capability grant or a scan of model/user prose. A mixed
	// bundle retains its scheduling lane; per-artifact query receipts still
	// decide which facts are usable.
	hasTrace := strings.TrimSpace(metadata.Systrace) != ""
	for _, artifact := range metadata.Artifacts {
		if artifact.Type == "systrace" {
			hasTrace = true
		}
	}
	return preparedTracePromptInfo{
		text: b.String(), sampleOnly: !hasTrace && hitraceconv.QueryReadyPerfTracePath(metadata.Artifacts) != "",
	}
}
