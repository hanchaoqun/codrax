package tracequery

import "github.com/hanchaoqun/codrax/internal/tracewire"

// PerfCallchainIdentity qualifies the legacy raw callchain label. Only an
// exact producer-frame sequence has a path identity; an opaque label can be
// counted but cannot establish frame boundaries or a prefix-tree branch.
type PerfCallchainIdentity struct {
	Form       string `json:"form"`
	Reason     string `json:"reason,omitempty"`
	ID         string `json:"id,omitempty"`
	FrameCount int    `json:"frame_count,omitempty"`
}

const PerfRecordedFieldsValueBasis = "recorded_sample_fields_not_prefix_node"

const PerfCallchainIntegrityTeaching = "Flat top_symbols/top_dso weights count recorded sample fields, including samples without verified frame boundaries; they are not prefix-node inclusive/self weights. Only exact_frames callchain identities and perf_sampling_tree nodes establish recorded frame paths. Opaque callchain labels remain original text, not delimited frames. Frame statuses and callchain_known/unknown counts describe verified frame boundaries; producer callchain_statuses are raw declarations, not proof of exact frames or complete unwinding."

// The tree, legacy aggregate and quality inventory share one admission rule.
// A producer's symbolized/partial label alone never verifies frame boundaries.
func perfSampleFrameData(pf *PerfFields) (tracewire.PerfFrames, string) {
	if pf == nil || pf.FramesV1 == "" {
		return tracewire.PerfFrames{}, "legacy_frame_boundaries_unavailable"
	}
	frames, ok := tracewire.DecodePerfFrames(pf.FramesV1)
	if !ok {
		return tracewire.PerfFrames{}, "malformed_frame_carrier"
	}
	if frames.Status != "available" {
		return frames, frames.Status
	}
	return frames, ""
}

func perfCallchainIdentity(pf *PerfFields, frames tracewire.PerfFrames, reason string) (string, *PerfCallchainIdentity) {
	if reason != "" {
		// Keep unknown records discoverable by their original flat label, but
		// never share a bucket with a verified path or a different failure kind.
		return samplingID("opaque_flat_label", reason, pf.DSO, pf.Callchain, pf.Symbol, pf.IP), &PerfCallchainIdentity{Form: "opaque_flat_label", Reason: reason}
	}
	parts := []string{"exact_frames"}
	for _, f := range frames.Frames {
		// SourceRef/SourceDepth locate each occurrence, not the path identity.
		parts = append(parts, f.Kind, f.Symbol, f.DSO, f.IP)
	}
	id := samplingID(parts...)
	return id, &PerfCallchainIdentity{Form: "exact_frames", ID: id, FrameCount: len(frames.Frames)}
}
