package tool

import (
	"encoding/json"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A derived query coordinate retains the producer's exact member in the
// execution ledger. Do not turn an unselected outer path into the first cached
// member, and do not alter the actual query parameters sent to the parser.
func traceCatalogQueryMemberParams(ctx *types.BusContext, path string, raw json.RawMessage) json.RawMessage {
	if ctx == nil || ctx.TraceInputPreparer == nil {
		return raw
	}
	var requested map[string]json.RawMessage
	if json.Unmarshal(raw, &requested) != nil {
		return raw
	}
	if _, supplied := requested["archive_member"]; supplied {
		return raw // preserve invalid explicit attempts, never silently repair them
	}
	for _, material := range ctx.TraceInputPreparer.PreparedMaterials() {
		if material == nil || material.ArchiveMember() == "" || path != material.QueryPath() || traceQueryValidateReadyMaterial(ctx, material) != nil {
			continue
		}
		return traceCatalogMemberParams(raw, material.ArchiveMember())
	}
	return raw
}

func traceCatalogMemberParams(raw json.RawMessage, member string) json.RawMessage {
	var args map[string]json.RawMessage
	if member == "" || json.Unmarshal(raw, &args) != nil || args == nil {
		return raw
	}
	args["archive_member"], _ = json.Marshal(member)
	out, err := json.Marshal(args)
	if err != nil {
		return raw
	}
	return out
}

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
