package types

import (
	"fmt"
	"github.com/hanchaoqun/codrax/internal/attachment"
)

// Descriptive only: serializing scope never restores an extraction receipt or
// upgrades an observation's authority. Parent-preview lines are not file lines.
type PerfObservationSourceScope = attachment.TraceExcerptScope

// PerfExtractionCoverage accounts for the controller's extraction work, not
// trace-query scan completeness, causal coverage, or model answer quality.
type PerfExtractionCoverage struct {
	ParentPreviewBytes    int `json:"parent_preview_bytes"`
	Segments              int `json:"segments"`
	Attempted             int `json:"attempted"`
	Succeeded             int `json:"succeeded"`
	Failed                int `json:"failed"`
	Skipped               int `json:"skipped"`
	Unattempted           int `json:"unattempted"`
	ExtractedPreviewBytes int `json:"extracted_preview_bytes"`
}

func (c *PerfExtractionCoverage) Description() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("Extraction coverage: %d/%d preview bytes in successful fragments (overlaps counted once); %d segments, %d attempted, %d succeeded, %d failed, %d skipped, %d unattempted. Residue coverage is a separate metric; neither establishes full-source scan completeness or causal coverage.", c.ExtractedPreviewBytes, c.ParentPreviewBytes, c.Segments, c.Attempted, c.Succeeded, c.Failed, c.Skipped, c.Unattempted)
}
