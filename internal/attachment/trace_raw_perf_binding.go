package attachment

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// RawPerfRecordSource describes the exact uncompressed original whose byte
// coordinates can be used for inspection. It is not query/read permission.
type RawPerfRecordSource struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func (s RawPerfRecordSource) Valid() bool {
	digest, err := hex.DecodeString(s.SHA256)
	return filepath.IsAbs(s.Path) && filepath.Clean(s.Path) == s.Path && len(digest) == 32 && err == nil && s.Bytes > 0
}

// RawPerfRecordBinding pairs the original with the exact derived perftrace
// written by the raw decoder. It is not restored from text or a manifest.
type RawPerfRecordBinding struct {
	Source       RawPerfRecordSource
	ArtifactPath string
	Artifact     filegeneration.Identity
	original     filegeneration.Identity
	index        *tracewire.PerfRecordIndex
}

// WithDirectRawPerfRecordBinding is called by the preparer only after the
// direct, uncompressed raw provider and its artifact generation were checked.
// Both files must be existing members of this immutable material.
func (m *TraceMaterial) WithDirectRawPerfRecordBinding(ctx context.Context, artifactPath, sourceSHA string, index *tracewire.PerfRecordIndex) (*TraceMaterial, error) {
	if m == nil {
		return nil, fmt.Errorf("raw record binding requires prepared material")
	}
	if err := m.Validate(ctx, m.preview); err != nil {
		return nil, err
	}
	artifact, bound := m.bindings[artifactPath]
	original := m.bindings[m.sourcePath]
	source := RawPerfRecordSource{Path: m.sourcePath, SHA256: sourceSHA, Bytes: original.Size()}
	if !bound || artifactPath == m.sourcePath || m.archiveMember != "" || !source.Valid() || !index.ValidWithin(source.Bytes) {
		return nil, fmt.Errorf("raw record binding requires the exact distinct direct input and derived artifact")
	}
	copy := *m
	copy.rawPerfRecords = &RawPerfRecordBinding{Source: source, ArtifactPath: artifactPath, Artifact: artifact, original: original, index: index}
	return &copy, nil
}

func (b RawPerfRecordBinding) AtDerivedLine(line int) (tracewire.PerfRecordSpan, bool) {
	return b.index.AtDerivedLine(line)
}

// ValidateOriginal rechecks the original after the derived query has run.
// The query never reopens it for content or treats a changed file as current.
func (b RawPerfRecordBinding) ValidateOriginal() error {
	current, err := filegeneration.FromPath(b.Source.Path)
	if err != nil || !b.original.SameVersion(current) {
		return fmt.Errorf("original perf record source changed or is unavailable %q; attach the source again", b.Source.Path)
	}
	return nil
}

func (m *TraceMaterial) RawPerfRecordBinding(ctx context.Context, queryPath string) (RawPerfRecordBinding, bool) {
	if m == nil || m.rawPerfRecords == nil || queryPath != m.queryPath || m.Validate(ctx, m.preview) != nil {
		return RawPerfRecordBinding{}, false
	}
	return *m.rawPerfRecords, true
}
