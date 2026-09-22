package attachment

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
)

// TraceExcerpt is a controller-owned view, not a new attachment or permission.
// Its private parent binding cannot be restored from model/persisted JSON.
type TraceExcerpt struct {
	parent   string
	material *TraceMaterial
	scope    TraceExcerptScope
}

// TraceExcerptScope is descriptive provenance only. Line numbers belong to the
// parent preview (which may be converted, truncated or a bundle), never to a
// physical source file. Bytes use half-open offsets in that same preview.
type TraceExcerptScope struct {
	ParentPreviewSHA256 string `json:"parent_preview_sha256"`
	ParentPreviewBytes  int    `json:"parent_preview_bytes"`
	RequestedByteStart  int    `json:"requested_byte_start"`
	RequestedByteEnd    int    `json:"requested_byte_end"`
	ByteStart           int    `json:"byte_start"`
	ByteEnd             int    `json:"byte_end"`
	LineStart           int    `json:"line_start"`
	LineEnd             int    `json:"line_end"`
	LineCoordinates     string `json:"line_coordinates"`
}

// NewTraceExcerpt retains only complete lines inside the selected range. It
// never expands a selection or fabricates a timestamp from a clipped event.
func NewTraceExcerpt(ctx context.Context, parent string, material *TraceMaterial, start, end int) (*TraceExcerpt, error) {
	if start < 0 || end <= start || end > len(parent) {
		return nil, fmt.Errorf("trace excerpt range is outside its parent preview")
	}
	s := TraceExcerptScope{ParentPreviewSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(parent))), ParentPreviewBytes: len(parent), RequestedByteStart: start, RequestedByteEnd: end, LineCoordinates: "parent_preview"}
	if start > 0 && parent[start-1] != '\n' {
		n := strings.IndexByte(parent[start:end], '\n')
		if n < 0 {
			return nil, fmt.Errorf("trace excerpt contains no complete line")
		}
		start += n + 1
	}
	// Prepared previews without an explicit complete-text receipt may end
	// mid-event. EOF is a complete line only for inline or proven full text.
	completeEOF := material == nil || material.SelfContainedText()
	if (end < len(parent) || !completeEOF) && parent[end-1] != '\n' {
		n := strings.LastIndexByte(parent[start:end], '\n')
		if n < 0 {
			return nil, fmt.Errorf("trace excerpt contains no complete line")
		}
		end = start + n + 1
	}
	if start >= end {
		return nil, fmt.Errorf("trace excerpt contains no complete line")
	}
	s.ByteStart, s.ByteEnd = start, end
	s.LineStart = 1 + strings.Count(parent[:start], "\n")
	s.LineEnd = s.LineStart + strings.Count(strings.TrimSuffix(parent[start:end], "\n"), "\n")
	v := &TraceExcerpt{parent: parent, material: material, scope: s}
	if _, _, err := v.Resolve(ctx, parent, material); err != nil {
		return nil, err
	}
	return v, nil
}

// Resolve validates the original receipt, never a slice against a full receipt.
// Legacy inline text is bound to its immutable parent bytes, without granting
// physical-file permissions. The returned scope is a defensive value copy.
func (v *TraceExcerpt) Resolve(ctx context.Context, parent string, material *TraceMaterial) (string, TraceExcerptScope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", TraceExcerptScope{}, err
	}
	if v == nil || v.parent == "" || v.parent != parent || v.material != material || v.scope.ByteEnd <= v.scope.ByteStart {
		return "", TraceExcerptScope{}, fmt.Errorf("trace excerpt no longer belongs to its parent attachment")
	}
	if material != nil {
		if err := material.Validate(ctx, parent); err != nil {
			return "", TraceExcerptScope{}, err
		}
	}
	return parent[v.scope.ByteStart:v.scope.ByteEnd], v.scope, nil
}

func (s TraceExcerptScope) Description() string {
	return fmt.Sprintf("Current extraction fragment: parent-preview bytes [%d,%d), lines %d-%d; these are preview coordinates, not physical source-file lines. This fragment is not the whole attachment.", s.ByteStart, s.ByteEnd, s.LineStart, s.LineEnd)
}
