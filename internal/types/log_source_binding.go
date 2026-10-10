package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/loginput"
)

// LogSourceBinding is system-derived provenance, never an emit parameter.
// Its private receipt survives ordinary value copies but not JSON restore.
// Generation is a material version, not a process lifetime or clock bridge.
type LogSourceBinding struct {
	Status         string `json:"status"`
	SourceID       string `json:"source_id,omitempty"`
	Name           string `json:"name,omitempty"`
	Path           string `json:"path,omitempty"`
	Generation     string `json:"generation,omitempty"`
	OriginalSHA256 string `json:"original_sha256,omitempty"`
	DecodedSHA256  string `json:"decoded_sha256,omitempty"`
	RecordID       string `json:"record_id,omitempty"`
	FirstLine      int64  `json:"first_line,omitempty"`
	LastLine       int64  `json:"last_line,omitempty"`
	ByteStart      int64  `json:"byte_start,omitempty"`
	ByteEnd        int64  `json:"byte_end,omitempty"`
	proof          [32]byte
}

// NewLogSourceBinding accepts only the in-process full-source read receipt.
// A caller-built/decoded location cannot mint original-source authority.
func NewLogSourceBinding(location loginput.ExcerptLocation) *LogSourceBinding {
	b := &LogSourceBinding{Status: location.Status}
	if !location.Verified() {
		return b
	}
	s := location.Source
	b.SourceID, b.Name, b.Path = s.ID, s.Name, s.Path
	b.Generation, b.OriginalSHA256, b.DecodedSHA256 = s.Generation, s.OriginalSHA256, s.DecodedSHA256
	b.RecordID = location.RecordID
	b.FirstLine, b.LastLine, b.ByteStart, b.ByteEnd = location.FirstLine, location.LastLine, location.ByteStart, location.ByteEnd
	b.proof = b.digest()
	return b
}

func (b *LogSourceBinding) digest() [32]byte { raw, _ := json.Marshal(b); return sha256.Sum256(raw) }

func (b *LogSourceBinding) IsVerified() bool {
	return b != nil && b.Status == "unique" && b.SourceID != "" && b.FirstLine > 0 && b.LastLine >= b.FirstLine && b.proof != [32]byte{} && b.proof == b.digest()
}

func (b *LogSourceBinding) Clone() *LogSourceBinding {
	if b == nil {
		return nil
	}
	copy := *b
	return &copy
}

func (b *LogSourceBinding) IdentityKey() string {
	if !b.IsVerified() {
		return ""
	}
	return fmt.Sprintf("%s:%s:%d-%d", b.SourceID, b.Generation, b.ByteStart, b.ByteEnd)
}

// LocationLabel is a concise runtime-log address, not a source-code citation.
// Unverified snapshots must not print saved physical coordinates as authority.
func (b *LogSourceBinding) LocationLabel() string {
	if b == nil {
		return ""
	}
	if !b.IsVerified() {
		if b.Status == "unique" {
			return "log_location=unverified_snapshot"
		}
		return "log_location=" + b.Status
	}
	return fmt.Sprintf("log_source=%q source_id=%s decoded_lines=%d-%d", firstNonEmptyString(b.Path, b.Name), b.SourceID, b.FirstLine, b.LastLine)
}

func applyLogSourceBinding(binding *LogSourceBinding, row ObservationRecord) ObservationRecord {
	if binding == nil {
		return row
	} // historical caller-owned bundles remain legacy
	row.Span.LineStart, row.Span.LineEnd = 0, 0
	if !binding.IsVerified() {
		row.RichNotes = append(row.RichNotes, binding.LocationLabel())
		return row
	}
	row.SourceRef.ArtifactID = binding.SourceID
	row.SourceRef.Path = binding.Path
	row.SourceRef.ArtifactKind = "runtime_log"
	row.Span.LineStart, row.Span.LineEnd = int(binding.FirstLine), int(binding.LastLine)
	row.RichNotes = append(row.RichNotes, binding.LocationLabel(), "source_generation="+binding.Generation, "source_sha256="+binding.OriginalSHA256, "decoded_sha256="+binding.DecodedSHA256, "line_coordinates=decoded_original_physical_lines")
	return row
}
