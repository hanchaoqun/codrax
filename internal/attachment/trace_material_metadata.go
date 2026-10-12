package attachment

import (
	"context"
	"encoding/json"
)

// TraceSourceMetadataField is an observed capture-header field, not identity,
// comparability, window, or causal authority. Missing/invalid/truncated values
// are never substituted with zero or inferred from a file name.
type TraceSourceMetadataField struct {
	Origin string `json:"origin"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Value  string `json:"value,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// TraceSourceMetadata keeps a producer-owned immutable copy. JSON is diagnostic
// only: unmarshalling does not reconstruct the in-process preparation payload.
type TraceSourceMetadata struct{ fields []TraceSourceMetadataField }

func NewTraceSourceMetadata(fields []TraceSourceMetadataField) TraceSourceMetadata {
	return TraceSourceMetadata{fields: append([]TraceSourceMetadataField(nil), fields...)}
}

func (m TraceSourceMetadata) Fields() []TraceSourceMetadataField {
	return append([]TraceSourceMetadataField(nil), m.fields...)
}

func (m TraceSourceMetadata) MarshalJSON() ([]byte, error) { return json.Marshal(m.fields) }
func (m *TraceSourceMetadata) UnmarshalJSON(raw []byte) error {
	m.fields = nil
	var diagnostic []TraceSourceMetadataField
	if err := json.Unmarshal(raw, &diagnostic); err != nil {
		return err
	}
	return nil
}

// WithSourceMetadata attaches actual decoder output within the existing
// preparation transaction; it cannot replace or renew any source receipt.
func (m *TraceMaterial) WithSourceMetadata(ctx context.Context, metadata TraceSourceMetadata) (*TraceMaterial, error) {
	preview := ""
	if m != nil {
		preview = m.preview
	}
	if err := m.Validate(ctx, preview); err != nil {
		return nil, err
	}
	copy := *m
	copy.sourceMetadata = NewTraceSourceMetadata(metadata.Fields())
	return &copy, nil
}

// SourceMetadata revalidates original, derived and journal generations even
// when no query/publication exists. Returned fields cannot mutate the receipt.
func (m *TraceMaterial) SourceMetadata(ctx context.Context) ([]TraceSourceMetadataField, error) {
	preview := ""
	if m != nil {
		preview = m.preview
	}
	if err := m.Validate(ctx, preview); err != nil {
		return nil, err
	}
	return m.sourceMetadata.Fields(), nil
}
