package outputdump

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// DefaultRootCauseArtifact retains the top-level v2 report fields. Availability
// is output metadata, not a model conclusion and never fed back into the model.
// Consumers must check Status before interpreting an empty RootCauses array.
type DefaultRootCauseArtifact struct {
	types.TraceRootCauseReportV2
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code,omitempty"`
}

func defaultRootCauseArtifact(report *types.TraceRootCauseReportV2, reason string) DefaultRootCauseArtifact {
	artifact := DefaultRootCauseArtifact{
		TraceRootCauseReportV2: types.TraceRootCauseReportV2{
			SchemaVersion: types.TraceRootCauseReportSchemaVersion,
			RootCauses:    []*types.TraceRootCauseItemV2{},
		},
		Status: ExplicitRootCauseStatusUnavailable,
	}
	if report != nil {
		artifact.TraceRootCauseReportV2 = *report
		if artifact.RootCauses == nil {
			artifact.RootCauses = []*types.TraceRootCauseItemV2{}
		}
		artifact.Status = ExplicitRootCauseStatusAvailable
	} else {
		artifact.ReasonCode = strings.TrimSpace(reason)
		if artifact.ReasonCode == "" {
			artifact.ReasonCode = "valid_root_cause_selection_unavailable"
		}
	}
	return artifact
}

func writeDefaultRootCauseSidecar(a Args, markdownPath string) Result {
	if !requiresDefaultRootCauseSidecar(a) {
		return Result{}
	}
	p := RootCauseJSONPathForMarkdown(markdownPath)
	body, err := json.MarshalIndent(defaultRootCauseArtifact(a.RootCauseReport, a.RootCauseUnavailableReason), "", "  ")
	if err != nil {
		logging.Warning("[output_dump] encode trace root-cause report failed; writing empty unavailable artifact: %v", err)
		// This fixed fallback contains no model data that could fail encoding.
		body, _ = json.MarshalIndent(defaultRootCauseArtifact(nil, "root_cause_report_encoding_failed"), "", "  ")
	}
	if err := os.WriteFile(p, append(body, '\n'), 0o644); err != nil {
		logging.Warning("[output_dump] write mandatory root-cause artifact %s failed: %v", p, err)
		return Result{RootCauseJSONError: fmt.Errorf("write trace root-cause artifact %s: %w", p, err)}
	}
	logging.Info("[output_dump] wrote %s (%d bytes)", p, len(body)+1)
	return Result{RootCauseJSONPath: p}
}

// WriteRootCauseOnly closes the early-exit/no-answer lane without fabricating
// a Markdown answer. Explicit output-disable configuration still wins; the
// separate --root-causes-out sink has its own outer-boundary guarantee.
func WriteRootCauseOnly(a Args) Result {
	if a.Dir == "" || explicitReportSnapshot().SuppressDefaultDir || !requiresDefaultRootCauseSidecar(a) {
		return Result{}
	}
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		logging.Warning("[output_dump] mkdir %s failed: %v", a.Dir, err)
		return Result{RootCauseJSONError: fmt.Errorf("create trace root-cause output directory: %w", err)}
	}
	PruneDir(a.Dir, a.Max)
	return writeDefaultRootCauseSidecar(a, filepath.Join(a.Dir, FileName(a.Now, a.PID)))
}

func requiresDefaultRootCauseSidecar(a Args) bool {
	return a.HasTrace || a.RequireRootCauseJSON || a.RootCauseReport != nil
}
