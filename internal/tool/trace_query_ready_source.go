package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceQueryInputPreparationTeaching = "Normal analysis runs automatically prepare supported binary file paths and closed, self-contained TraceStreamer SQLite databases before querying, reusing the complete prepared material within the run; no separate model conversion call is needed. SQLite is recognized by content, not filename extension, read from a private snapshot without invoking a converter or changing the original database. WAL/header-WAL, SHM or journal state is currently refused; use a closed self-contained export. Previews are bounded but queries use the complete admitted material. Originals stay unchanged and evidence line references address the readable query material. Unsupported, malformed or inventory-only captures fail without substituting a different source. Binary stdin/inline is not supported. Low-level parsers remain text/bundle only; codrax trace convert --input <binary-trace-path> remains the explicit binary conversion interface."

func traceQueryValidateReadyMaterial(ctx *types.BusContext, material *attachment.TraceMaterial) error {
	preview := material.Preview()
	if ctx != nil && material == ctx.AttachedTraceMaterial {
		preview = ctx.AttachedHitrace
	}
	if err := material.Validate(contextFromBus(ctx), preview); err != nil {
		return err
	}
	if ctx != nil && material != ctx.AttachedTraceMaterial && ctx.TraceInputPreparer != nil {
		// A successful query must still refer to the same run receipt and the
		// engine-selected member universe, including sibling bundle promotion.
		current, err := ctx.TraceInputPreparer.Prepare(contextFromBus(ctx), material.QueryPath())
		if err != nil {
			return err
		}
		if current != material {
			return fmt.Errorf("prepared trace receipt changed during query")
		}
	}
	return nil
}

// resolveReadyTraceQuerySource is the product-level preparation boundary. The
// selector stays side-effect free; index/stream parsers still accept only text
// or admitted bundles and never start a converter. Explicit failed selections
// are returned unchanged, without borrowing another capture or attachment.
func resolveReadyTraceQuerySource(ctx *types.BusContext, p traceQueryParams) (string, string, *attachment.TraceMaterial, *types.ToolResult) {
	path, label, reject := resolveTraceQuerySource(ctx, p)
	if reject != nil || ctx == nil {
		return path, label, nil, reject
	}
	var material *attachment.TraceMaterial
	var err error
	if label == "attached_trace" {
		material = ctx.AttachedTraceMaterial
		if material != nil {
			err = material.Validate(contextFromBus(ctx), ctx.AttachedHitrace)
		}
	} else if ctx.TraceInputPreparer != nil {
		material, err = ctx.TraceInputPreparer.Prepare(contextFromBus(ctx), path)
		if err == nil && material == nil {
			err = fmt.Errorf("trace preparation returned no complete material")
		}
	}
	if err != nil {
		failure := traceQueryPreparedMaterialFailure(path, p.View, err)
		return path, label, nil, &failure
	}
	if material != nil {
		path = material.QueryPath()
	}
	return path, label, material, nil
}

// Only published, still-valid in-process receipts can identify the source and
// query coordinates as one selection. Do not turn bundle children, matching
// basenames or model-authored paths into aliases. This never prepares a file.
func traceQueryPreparedNamedPath(ctx *types.BusContext, path string) string {
	if ctx == nil || ctx.TraceInputPreparer == nil {
		return path
	}
	for _, material := range ctx.TraceInputPreparer.PreparedMaterials() {
		if material == nil || !material.MatchesPath(path) {
			continue
		}
		if material.Validate(contextFromBus(ctx), material.Preview()) == nil {
			return material.QueryPath()
		}
	}
	return path
}

func traceQueryReferencedTraceTokens(ctx *types.BusContext, value string) []string {
	var tokens []string
	for _, token := range types.RuntimeArtifactPathTokensInText(value) {
		if types.RuntimeArtifactPathKind(token) == "trace" {
			tokens = append(tokens, token)
		}
	}
	// A prepared receipt proves a file's role independently of its extension.
	// Match the entire typed carrier, never substrings of request/answer prose.
	// This covers generic .sys and extensionless captures after admission.
	if ctx != nil && ctx.TraceInputPreparer != nil {
		path := resolveToolPath(ctx, strings.TrimSpace(value))
		for _, material := range ctx.TraceInputPreparer.PreparedMaterials() {
			if material != nil && material.MatchesPath(path) && material.Validate(contextFromBus(ctx), material.Preview()) == nil {
				tokens = append(tokens, path)
				break
			}
		}
	}
	return tokens
}
