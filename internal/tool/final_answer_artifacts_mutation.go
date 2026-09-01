package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizeMisplacedTraceRootCauseSchemaVersion repairs one unambiguous
// structural carrier drift seen in production tool calls: the model authors
// the ordered candidate selections inside trace_root_causes but places the
// fixed schema_version discriminator at the document top level. The public
// answer schema has no top-level schema_version field, and the nested report
// is otherwise complete, so moving only the exact current constant is
// lossless. Candidate IDs, their order, visible answer text, and every
// conclusion remain untouched. Wrong/ambiguous values fail open to the
// existing optional-sidecar validation path.
func normalizeMisplacedTraceRootCauseSchemaVersion(raw json.RawMessage, reportField string) (json.RawMessage, bool) {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return raw, false
	}
	outerVersion, ok := root["schema_version"]
	if !ok || !isExactTraceRootCauseSchemaVersion(outerVersion) {
		return raw, false
	}
	var report map[string]json.RawMessage
	if json.Unmarshal(root[reportField], &report) != nil {
		return raw, false
	}
	if _, exists := report["schema_version"]; exists {
		return raw, false
	}
	if _, exists := report["root_causes"]; !exists {
		return raw, false
	}
	canonicalVersion, err := json.Marshal(types.TraceRootCauseReportSchemaVersion)
	if err != nil {
		return raw, false
	}
	report["schema_version"] = canonicalVersion
	reportRaw, err := json.Marshal(report)
	if err != nil {
		return raw, false
	}
	root[reportField] = reportRaw
	delete(root, "schema_version")
	repaired, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return repaired, true
}

func isExactTraceRootCauseSchemaVersion(raw json.RawMessage) bool {
	var version int
	if json.Unmarshal(raw, &version) == nil {
		return version == types.TraceRootCauseReportSchemaVersion
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return false
	}
	return text == strconv.Itoa(types.TraceRootCauseReportSchemaVersion)
}

// resolveTraceFindingForEmit enforces the opt-in contract before the shared
// document persist path is allowed to commit either artifact.
func resolveTraceFindingForEmit(ctx *types.BusContext, submitted *types.TraceFindingV1, patch bool) (*types.TraceFindingV1, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.Required {
		if submitted != nil {
			return nil, fmt.Errorf("trace_finding is not enabled for this request")
		}
		return nil, nil
	}
	finding := submitted
	if patch && finding == nil {
		finding = ctx.Mutable.TraceFinding()
	}
	if err := tracefinding.Validate(finding, contract); err != nil {
		return nil, err
	}
	return finding, nil
}

// resolveTraceRootCauseReportForEmit validates and binds the optional selector.
// Patch emits inherit the last accepted report when they only repair answer
// blocks. Sidecar errors are returned for diagnostics but never own whether
// the full answer mutation is accepted.
func resolveTraceRootCauseReportForEmit(ctx *types.BusContext, submitted json.RawMessage, patch bool) (*types.TraceRootCauseReportV2, error) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.RootCauseReportEnabled {
		if len(strings.TrimSpace(string(submitted))) > 0 && string(submitted) != "null" {
			return nil, fmt.Errorf("trace_root_causes is not enabled for this request")
		}
		return nil, nil
	}
	if len(strings.TrimSpace(string(submitted))) == 0 || string(submitted) == "null" {
		if patch {
			return ctx.Mutable.TraceRootCauseReport(), nil
		}
		return nil, nil
	}
	var selection types.TraceRootCauseReportV2
	if err := json.Unmarshal(submitted, &selection); err != nil {
		if patch {
			return ctx.Mutable.TraceRootCauseReport(), fmt.Errorf("decode optional trace_root_causes selector: %w", err)
		}
		return nil, fmt.Errorf("decode optional trace_root_causes selector: %w", err)
	}
	report, err := tracefinding.BindRootCauseReportSelection(&selection, contract)
	if err != nil && patch {
		return ctx.Mutable.TraceRootCauseReport(), err
	}
	return report, err
}
