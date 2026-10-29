package attachment

import (
	"context"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// TraceMaterialDisplay is a read-only presentation snapshot, not a query or
// evidence receipt. Its sizes belong to separate physical objects; PreviewBytes
// includes the complete model envelope, including producer source headers.
type TraceMaterialDisplay struct {
	SourcePath, QueryPath     string
	SourceFormat, QueryFormat string
	SourceBytes, QueryBytes   int64
	PreviewBytes              int64
	ArchiveMember             string
	fileGenerations           map[string]filegeneration.Identity
}

// FileGeneration exposes only the already validated presentation observation
// for an exact bound file. It does not admit new paths or grant query authority.
func (m TraceMaterialDisplay) FileGeneration(path string) (filegeneration.Identity, bool) {
	id, ok := m.fileGenerations[path]
	return id, ok
}

// WithDisplayFormats retains the preparer's content classification. It cannot
// renew a generation, authorize a reader or restore a serialized receipt.
func (m *TraceMaterial) WithDisplayFormats(ctx context.Context, source, query string) (*TraceMaterial, error) {
	if m == nil {
		return nil, m.Validate(ctx, "")
	}
	if err := m.Validate(ctx, m.Preview()); err != nil {
		return nil, err
	}
	copy := *m
	copy.sourceFormat, copy.queryFormat = source, query
	return &copy, nil
}

// DisplayMetadata never substitutes preview length or a later filesystem stat
// for the held generation's byte count. Any stale member invalidates this
// snapshot; callers may still describe the preview, but not current file size.
func (m *TraceMaterial) DisplayMetadata(ctx context.Context, preview string) (TraceMaterialDisplay, error) {
	if err := m.Validate(ctx, preview); err != nil {
		return TraceMaterialDisplay{}, err
	}
	files := make(map[string]filegeneration.Identity, len(m.bindings))
	for path, id := range m.bindings {
		files[path] = id
	}
	return TraceMaterialDisplay{
		SourcePath: m.sourcePath, QueryPath: m.queryPath,
		SourceFormat: m.sourceFormat, QueryFormat: m.queryFormat,
		SourceBytes: m.bindings[m.sourcePath].Size(), QueryBytes: m.bindings[m.queryPath].Size(),
		PreviewBytes: int64(len(m.preview)), ArchiveMember: m.archiveMember,
		fileGenerations: files,
	}, nil
}
