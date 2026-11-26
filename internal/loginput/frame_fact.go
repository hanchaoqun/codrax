package loginput

import (
	"encoding/json"
	"strings"
)

// PreparedFrameFact is one complete saved preanalysis record. Shape and source
// bytes are verified at preparation; its conclusions still need Trace evidence.
type PreparedFrameFact struct {
	RecordRef string
	Text      string
	TID       int
	StartNS   int64
	EndNS     int64
}

func (c *Catalog) PreparedFrameFact() (PreparedFrameFact, bool) {
	if c == nil || c.frameFact == nil {
		return PreparedFrameFact{}, false
	}
	return *c.frameFact, true
}

func prepareFrameFact(s preparedSource) *PreparedFrameFact {
	if !s.summary.Complete || s.summary.Records != 1 || s.summary.PhysicalLines != 1 ||
		int64(len(s.preview)) != s.summary.DecodedBytes {
		return nil
	}
	raw := strings.TrimSpace(strings.TrimPrefix(s.preview, "\ufeff"))
	var row struct {
		ObservationID string                     `json:"observation_id"`
		SourceID      string                     `json:"source_id"`
		Kind          string                     `json:"record_kind"`
		Side          string                     `json:"side"`
		MetricKind    string                     `json:"metric_kind"`
		TID           int                        `json:"thread_id"`
		StartNS       *int64                     `json:"start_ns"`
		EndNS         int64                      `json:"end_ns"`
		Count         int                        `json:"count"`
		Metrics       map[string]json.RawMessage `json:"frame_metrics"`
	}
	if json.Unmarshal([]byte(raw), &row) != nil || row.Kind != "frame" ||
		row.Side != "target" || row.MetricKind != "jank" || row.TID <= 0 || row.Count != 1 ||
		row.StartNS == nil || *row.StartNS < 0 || row.EndNS <= *row.StartNS ||
		strings.TrimSpace(row.ObservationID) == "" || strings.TrimSpace(row.SourceID) == "" || len(row.Metrics) == 0 {
		return nil
	}
	return &PreparedFrameFact{RecordRef: s.summary.ID + ":L1-1", Text: raw,
		TID: row.TID, StartNS: *row.StartNS, EndNS: row.EndNS}
}
