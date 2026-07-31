package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// addProseLexiconObservationSourceRef projects accepted typed source-reference
// metadata into the information-only answer lexicon. A field name becomes
// quotable only when that field is actually present on this observation;
// absent metadata therefore remains unknown instead of becoming a global
// vocabulary exemption.
func addProseLexiconObservationSourceRef(
	addText func(string),
	ref types.ObservationSourceRef,
) {
	if addText == nil {
		return
	}
	add := func(field, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			addText(field + "=" + value)
		}
	}
	add("kind", string(ref.Kind))
	add("repo", ref.Repo)
	add("path", ref.Path)
	add("commit", ref.Commit)
	add("range", ref.Range)
	add("pathspec", ref.Pathspec)
	add("command", ref.Command)
	add("tool_call_id", ref.ToolCallID)
	add("raw_ref", ref.RawRef)
	add("payload_ref", ref.PayloadRef)
	add("row_set_ref", ref.RowSetRef)
	add("page_ref", ref.PageRef)
	add("artifact_id", ref.ArtifactID)
	add("artifact_kind", ref.ArtifactKind)
	add("time_domain", ref.TimeDomain)
	add("canonical_time_domain", ref.CanonicalTimeDomain)
	add("clock_alignment", ref.ClockAlignment)
	if ref.ClockCalibrated {
		addText("clock_calibrated=true")
	}
	if ref.ClockOffsetSec != nil {
		addText(fmt.Sprintf("clock_offset_sec=%g", *ref.ClockOffsetSec))
	}
	if ref.ClockSlope != nil {
		addText(fmt.Sprintf("clock_slope=%g", *ref.ClockSlope))
	}
	add("url", ref.URL)
	add("fetched_at", ref.FetchedAt)
	add("server", ref.Server)
	add("resource_uri", ref.ResourceURI)
	add("mime_type", ref.MIMEType)
	add("connector", ref.Connector)
}
