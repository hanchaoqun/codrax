package attachment

import (
	"context"
	"fmt"
	"path/filepath"
)

// WithQueryArtifacts seals the preparer's already admitted query leaves. File
// bindings also contain transport and metadata files, which are not query
// sources. This process-local set cannot be reconstructed from model JSON.
func (m *TraceMaterial) WithQueryArtifacts(ctx context.Context, paths []string) (*TraceMaterial, error) {
	if m == nil {
		return nil, fmt.Errorf("prepared query material is missing")
	}
	if err := m.Validate(ctx, m.Preview()); err != nil {
		return nil, err
	}
	owned := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == m.sourcePath && path != m.queryPath {
			return nil, fmt.Errorf("query artifact is not a prepared query coordinate: %q", path)
		}
		if _, ok := m.bindings[path]; !ok {
			return nil, fmt.Errorf("query artifact has no prepared generation: %q", path)
		}
		owned[path] = true
	}
	copy := *m
	copy.queryArtifacts = owned
	return &copy, nil
}

// OwnsQueryArtifact is only a source join for independently validated native
// facts. It creates neither an original-path alias nor query/evidence authority.
func (m *TraceMaterial) OwnsQueryArtifact(path string) bool {
	return m != nil && (path == m.queryPath || m.queryArtifacts[path]) &&
		m.Validate(context.Background(), m.preview) == nil
}
