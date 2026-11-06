package tracewire

import "math"

// PerfRecordSpan is a byte range in a direct uncompressed perf.data stream,
// starting at its physical record header (not the body, sample ID, or time).
// The range alone does not identify its source or prove any sample semantics.
type PerfRecordSpan struct {
	HeaderOffset uint64 `json:"header_offset"`
	RecordBytes  uint64 `json:"record_bytes"`
}

func (s PerfRecordSpan) ValidWithin(sourceBytes int64) bool {
	return sourceBytes > 0 && s.HeaderOffset > 0 && s.RecordBytes >= 8 && s.RecordBytes <= math.MaxUint16 &&
		s.HeaderOffset <= uint64(sourceBytes) && s.RecordBytes <= uint64(sourceBytes)-s.HeaderOffset
}

// MaxPerfSamplingStreamRecords is the existing complete-stream retention
// ceiling. A locator index never retains more spans than that consumer can
// read; a larger population remains handled by the legacy unavailable path.
const MaxPerfSamplingStreamRecords = 65536

// PerfRecordIndex maps exact emitted rows to original binary locations. It is
// in-memory only, with a single contiguous derived-line domain. No field in
// the derived sample text can recreate it or change that text's identity.
type PerfRecordIndex struct {
	firstLine int
	spans     []PerfRecordSpan
}

func NewPerfRecordIndex(firstLine int, spans []PerfRecordSpan) *PerfRecordIndex {
	if firstLine < 1 || len(spans) == 0 || len(spans) > MaxPerfSamplingStreamRecords {
		return nil
	}
	return &PerfRecordIndex{firstLine: firstLine, spans: append([]PerfRecordSpan(nil), spans...)}
}

func (i *PerfRecordIndex) ValidWithin(sourceBytes int64) bool {
	if i == nil || len(i.spans) == 0 {
		return false
	}
	for _, span := range i.spans {
		if !span.ValidWithin(sourceBytes) {
			return false
		}
	}
	return true
}

func (i *PerfRecordIndex) AtDerivedLine(line int) (PerfRecordSpan, bool) {
	if i == nil || line < i.firstLine || line-i.firstLine >= len(i.spans) {
		return PerfRecordSpan{}, false
	}
	return i.spans[line-i.firstLine], true
}
