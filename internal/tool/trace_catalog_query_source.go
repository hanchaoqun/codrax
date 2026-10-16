package tool

import (
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reverse only an already published, current preparation receipt. This is an
// execution-ledger coordinate, not a replacement query path or source grant.
// Manifest JSON, bundle children, filenames and equal bytes are not aliases.
func traceCatalogQueryOriginalPath(ctx *types.BusContext, path string) string {
	materials := []*attachment.TraceMaterial{ctx.AttachedTraceMaterial}
	if ctx.TraceInputPreparer != nil {
		materials = append(materials, ctx.TraceInputPreparer.PreparedMaterials()...)
	}
	original := ""
	for _, material := range materials {
		if material == nil || !material.MatchesPath(path) || traceQueryValidateReadyMaterial(ctx, material) != nil {
			continue
		}
		bound, err := filepath.EvalSymlinks(material.SourcePath())
		if err != nil {
			continue
		}
		if _, err := traceCatalogAuthorizedRoot(ctx, bound); err != nil {
			continue
		}
		if original != "" && original != bound {
			return path // Conflicting private origins cannot select a winner.
		}
		original = bound
	}
	if original != "" {
		return original
	}
	return path
}
