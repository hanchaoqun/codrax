package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceCapabilities performs no capture access or query effects. Its output
// is tool documentation with a run-local read ticket, never a repository fact
// or runtime observation; only dispatcher publication records the read.
type TraceCapabilities struct {
	ReadOnly
	NonEvidenceTool
}

func (*TraceCapabilities) Name() string { return "trace_capabilities" }
func (*TraceCapabilities) Description() string {
	return "Discover implemented trace_query views, measurement units, required events/identities, input-format conditions and known gaps. No attachment is needed. Defaults to a compact catalog; select view and detail=true for measurement contracts (including referenced components). This is static metadata, not evidence that a capture supports a measurement, not measured zero and not causal proof. It performs no trace query or conversion."
}

func (*TraceCapabilities) Parameters() json.RawMessage {
	view, _ := traceCapabilityQueryViewSchema()
	view["description"] = json.RawMessage(`"Optional trace_query view; omit to list all views. Tool-entry aliases are accepted; this does not change the low-level engine alias grammar."`)
	body, _ := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"view":   view,
			"detail": map[string]any{"type": "boolean", "default": false, "description": "Include per-metric outputs/units/prerequisites/limitations and input formats. Composite views include their referenced components' metric contracts."},
		},
	})
	return body
}

// Read alias metadata from the executing tool schema, not a copied catalog
// table. These are explicitly tool-entry aliases, not engine-wide aliases.
func traceCapabilityQueryViewSchema() (map[string]json.RawMessage, error) {
	var schema struct {
		Properties map[string]map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		return nil, err
	}
	view := schema.Properties["view"]
	if view == nil {
		return nil, fmt.Errorf("trace_query has no view schema")
	}
	return view, nil
}

func (*TraceCapabilities) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	out := types.ToolResult{ToolName: "trace_capabilities"}
	var input struct {
		View   string `json:"view"`
		Detail bool   `json:"detail"`
	}
	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		out.Summary = "Invalid catalog parameters: " + err.Error()
		return out, nil
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		out.Summary = "Invalid catalog parameters: expected one JSON object"
		return out, nil
	}
	viewSchema, err := traceCapabilityQueryViewSchema()
	if err != nil {
		return out, err
	}
	var aliases map[string]string
	if err := json.Unmarshal(viewSchema["x-codrax-enum-aliases"], &aliases); err != nil {
		return out, err
	}
	view := strings.TrimSpace(input.View)
	if canonical, ok := aliases[view]; ok {
		view = canonical
	}
	catalog, err := tracequery.TraceCapabilities(view, input.Detail)
	if err != nil {
		out.Summary = err.Error()
		return out, nil
	}
	payload := struct {
		tracequery.CapabilityCatalog
		ToolEntryAliases map[string]string `json:"tool_entry_aliases"`
	}{catalog, aliases}
	body, err := json.Marshal(payload)
	if err != nil {
		return out, err
	}
	doc, ok := types.NormalizeToolDocumentation(types.ToolDocumentation{
		Version: types.ToolDocumentationVersion, Schema: "trace_capabilities/v1",
		Selection: types.ToolDocumentationSelection{View: view, Detail: input.Detail}, Content: body,
	})
	if !ok {
		return out, fmt.Errorf("catalog documentation exceeds its bounded JSON contract")
	}
	out.Handoff = &types.ToolHandoffCarrier{Version: types.ToolHandoffCarrierVersion, ToolName: out.ToolName, Documentation: &doc}
	out.Success, out.Summary = true, string(body)
	if ctx != nil && ctx.Mutable != nil {
		out = ctx.Mutable.StampToolDocumentationResult(out)
	}
	return out, nil
}
