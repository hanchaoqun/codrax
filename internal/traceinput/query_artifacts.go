package traceinput

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

// bindQueryArtifacts runs after bindBundle's complete schema/digest/admission
// validation. It records only readable query leaves, not raw capture, SQLite,
// preparation receipt or metadata bindings. Validate pins the same generation
// before and after inspecting the bounded manifest.
func bindQueryArtifacts(ctx context.Context, material *attachment.TraceMaterial) (_ *attachment.TraceMaterial, err error) {
	if !strings.HasSuffix(strings.ToLower(material.QueryPath()), ".tracebundle.json") {
		return material, nil
	}
	if err := material.Validate(ctx, material.Preview()); err != nil {
		return nil, err
	}
	snapshot, err := tracebundle.Open(ctx, material.QueryPath())
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, snapshot.Close()) }()
	var manifest struct {
		Systrace  string `json:"systrace"`
		Artifacts []struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"artifacts"`
	}
	if err := snapshot.Decode(&manifest); err != nil {
		return nil, err
	}
	var paths []string
	add := func(path string) {
		if path == "" {
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(material.QueryPath()), filepath.FromSlash(path))
		}
		paths = append(paths, filepath.Clean(path))
	}
	add(manifest.Systrace)
	for _, artifact := range manifest.Artifacts {
		if artifact.Type == "systrace" || artifact.Type == "perftrace" {
			add(artifact.Path)
		}
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	return material.WithQueryArtifacts(ctx, paths)
}
