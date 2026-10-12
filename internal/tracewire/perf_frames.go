package tracewire

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

const MaxPerfFrames = 256
const MaxPerfFramesBytes = 24 << 10

// PerfFrame is one exact producer frame, in root-to-leaf order. Identity uses
// the complete symbol/DSO/IP tuple, never a delimited display label. SourceRef
// and SourceDepth locate the original frame; they are not call authority.
type PerfFrame struct {
	Kind        string `json:"kind,omitempty"`
	Symbol      string `json:"symbol"`
	DSO         string `json:"dso"`
	IP          string `json:"ip"`
	SourceRef   string `json:"source_ref"`
	SourceDepth int64  `json:"source_depth"`
}

type PerfFrames struct {
	Version int         `json:"version"`
	Status  string      `json:"status"`
	Frames  []PerfFrame `json:"frames,omitempty"`
}

func EncodePerfFrames(frames []PerfFrame, status string) string {
	if status == "" {
		status = "available"
	}
	p := PerfFrames{Version: 1, Status: status, Frames: frames}
	if !validPerfFrames(p) {
		p.Status, p.Frames = "frame_budget_or_metadata_unavailable", nil
	}
	data, _ := json.Marshal(p)
	if len(data) > MaxPerfFramesBytes {
		data, _ = json.Marshal(PerfFrames{Version: 1, Status: "frame_budget_or_metadata_unavailable"})
	}
	return string(data)
}

func DecodePerfFrames(raw string) (PerfFrames, bool) {
	var p PerfFrames
	if len(raw) > MaxPerfFramesBytes || !utf8.ValidString(raw) {
		return p, false
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || !validPerfFrames(p) {
		return PerfFrames{}, false
	}
	// Reject duplicate fields and noncanonical forms, so no decoder can select
	// a different identity from the same carrier. All writers use this codec.
	b, _ := json.Marshal(p)
	return p, bytes.Equal(b, []byte(raw))
}

func validPerfFrames(p PerfFrames) bool {
	if p.Version != 1 || len(p.Frames) > MaxPerfFrames {
		return false
	}
	switch p.Status {
	case "available":
		if len(p.Frames) == 0 {
			return false
		}
	case "missing", "source_depths_unavailable", "frame_budget_or_metadata_unavailable":
		return len(p.Frames) == 0
	default:
		return false
	}
	for _, f := range p.Frames {
		if f.Symbol == "" && f.IP == "" {
			return false
		}
		if f.Kind != "" && f.Kind != "frame" && f.Kind != "context_marker" {
			return false
		}
		if f.SourceDepth < 0 || f.SourceRef == "" {
			return false
		}
		for _, s := range []string{f.Symbol, f.DSO, f.IP, f.SourceRef} {
			if len(s) > MaxPerfMetadataBytes || !utf8.ValidString(s) {
				return false
			}
		}
	}
	return true
}
