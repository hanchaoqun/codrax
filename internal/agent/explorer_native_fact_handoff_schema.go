package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type explorerMeasurementPatch struct {
	draft, handoff map[string]json.RawMessage
	schema         llm.ToolSchema
	message        llm.Message
	refs           map[string]string
	rowRepair      *explorerNativeRowRepair
	traceBatch     []llm.ToolCall
}

func newExplorerMeasurementPatch(ctx *types.BusContext, call llm.ToolCall, result types.ToolResult) *explorerMeasurementPatch {
	repair := result.Repair
	if !result.Success && call.Name == "trace_query" && result.ToolName == call.Name &&
		repair != nil && repair.Code == tool.TraceQueryBusinessWindowConflict {
		return newExplorerTraceScopePatch(call, repair, types.StripConversationPrefix(ctx.Mutable.Objective()))
	}
	if result.Success || result.ToolName != explorerCompletionToolName || call.Name != explorerCompletionToolName || repair == nil ||
		(repair.Code != "native_fact_handoff" && repair.Code != "managed_delivery_native_handoff") {
		return nil
	}
	var issues []struct {
		Field string `json:"field"`
		Code  string `json:"code"`
	}
	if json.Unmarshal([]byte(repair.Metadata["native_fact_handoff_violations"]), &issues) != nil || len(issues) != 1 {
		return nil
	}
	patch := &explorerMeasurementPatch{refs: make(map[string]string)}
	if json.Unmarshal(call.Params, &patch.draft) != nil ||
		json.Unmarshal(patch.draft["native_fact_handoff"], &patch.handoff) != nil ||
		patch.handoff == nil {
		return nil
	}
	var choices []types.AnswerRuntimeMeasurementReceipt
	if json.Unmarshal([]byte(repair.Metadata["current_runtime_measurements"]), &choices) != nil || len(choices) == 0 {
		return nil
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil {
		return nil
	}
	if issues[0].Field == "native_fact_handoff.interpretation_units.native_refs" && issues[0].Code == "not_current_selected_native_row_address" {
		return prepareExplorerNativeRowRepair(patch, call.Name, repair, choices, view.RuntimeMeasurementContract)
	}
	if issues[0].Field != "native_fact_handoff.measurement_refs" || issues[0].Code != "select_current_refs_or_native_rows" ||
		len(patch.handoff["measurement_refs"]) != 0 {
		return nil
	}
	tables := view.RuntimeMeasurementContract.Choices()
	var roster []map[string]any
	var aliases []string
	publications := map[string]int{}
	for _, choice := range choices {
		for _, table := range tables {
			if table.ObservationID != choice.ObservationID || table.View != choice.View {
				continue
			}
			alias := fmt.Sprintf("r%d", len(aliases)+1)
			aliases = append(aliases, alias)
			patch.refs[alias] = tool.NativeFactMeasurementRef(choice)
			publication, kind, _ := strings.Cut(choice.ObservationID, "#")
			if publications[publication] == 0 {
				publications[publication] = len(publications) + 1
			}
			item := map[string]any{"ref": alias, "query": publications[publication], "observation": kind, "view": choice.View, "label": table.Label, "default": table.DefaultPresentation}
			for _, note := range table.Notes {
				if strings.HasPrefix(note, "时间窗口") || strings.HasPrefix(note, "Time window") {
					item["window"] = note
					break
				}
			}
			roster = append(roster, item)
			break
		}
	}
	if len(aliases) == 0 {
		return nil
	}
	param, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{"measurement_refs": map[string]any{
			"type": "array", "minItems": 1, "maxItems": 16, "uniqueItems": true,
			"items": map[string]any{"type": "string", "enum": aliases},
		}},
		"required":             []string{"measurement_refs"},
		"additionalProperties": false,
	})
	patch.schema = llm.ToolSchema{Name: call.Name, Description: "Supply just the missing measurement references for the existing investigation.", Parameters: param}
	evidence, _ := json.Marshal(roster)
	patch.message = llm.Message{Role: "user", Content: "The investigation completion was rejected because native_fact_handoff.measurement_refs is missing. Select the whole-frame anchors that support the saved findings: the target thread timeline, one summary per applicable scheduling measurement, and the business marker whose identity matches the requested frame. When a summary is available, do not select its other views as duplicates. The original interpretation keeps its detailed phase and other thread citations, so do not add their tables unless an independent principal finding depends on that table. Preserve exact subject, window and scope. Return only the short refs.\nSaved interpretation units: " +
		string(patch.handoff["interpretation_units"]) + "\nSaved interpretation: " + string(patch.handoff["interpretation"]) + "\nCurrent typed views: " + string(evidence)}
	return patch
}

func (p *explorerMeasurementPatch) expand(raw json.RawMessage) json.RawMessage {
	if p.schema.Name == "trace_query" {
		return p.expandTraceScope(raw)
	}
	if p.rowRepair != nil {
		return p.rowRepair.expand(p, raw)
	}
	var response struct {
		Refs []string `json:"measurement_refs"`
	}
	if json.Unmarshal(raw, &response) != nil || len(response.Refs) == 0 || len(response.Refs) > 16 {
		return json.RawMessage(`{"measurement_refs":null}`)
	}
	refs := make([]string, 0, len(response.Refs))
	for _, alias := range response.Refs {
		ref := p.refs[alias]
		if ref == "" || slices.Contains(refs, ref) {
			return json.RawMessage(`{"measurement_refs":null}`)
		}
		refs = append(refs, ref)
	}
	handoff := maps.Clone(p.handoff)
	draft := maps.Clone(p.draft)
	handoff["measurement_refs"], _ = json.Marshal(refs)
	draft["native_fact_handoff"], _ = json.Marshal(handoff)
	out, _ := json.Marshal(draft)
	return out
}

// Use the same current contract as completion and finalization. These bounded
// choices teach existing identities, not publication, population or causal
// authority. Event inventory uses another publication lane and is not offered.
func projectExplorerNativeFactHandoffSchema(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	const field = "native_fact_handoff"
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	choices, displayChoices, required := tool.NativeFactHandoffSchemaChoices(bus)
	compactSelection := tool.UsesCompactNativeFactHandoff(bus)
	out := append([]llm.ToolSchema(nil), schemas...)
	for i, schema := range out {
		if schema.Name != explorerCompletionToolName {
			continue
		}
		if len(choices) == 0 {
			if projected, ok := omitJSONSchemaTopLevelProperty(schema.Parameters, field); ok {
				out[i].Parameters = projected
			}
			continue
		}
		var root, properties, handoff, fields, selections, items map[string]json.RawMessage
		if json.Unmarshal(schema.Parameters, &root) != nil || json.Unmarshal(root["properties"], &properties) != nil ||
			json.Unmarshal(properties[field], &handoff) != nil || json.Unmarshal(handoff["properties"], &fields) != nil ||
			json.Unmarshal(fields["runtime_measurements"], &selections) != nil || json.Unmarshal(selections["items"], &items) != nil {
			continue
		}
		items["enum"], _ = json.Marshal(choices)
		selections["items"], _ = json.Marshal(items)
		fields["runtime_measurements"], _ = json.Marshal(selections)
		projectExplorerNativeRowReferenceSchema(ctx, fields, choices)
		if len(displayChoices) == 0 {
			delete(fields, "answer_display")
		} else {
			var display map[string]json.RawMessage
			if json.Unmarshal(fields["answer_display"], &display) == nil {
				items["enum"], _ = json.Marshal(displayChoices)
				display["items"], _ = json.Marshal(items)
				display["description"], _ = json.Marshal(types.NativeAnswerDisplayTeaching)
				fields["answer_display"], _ = json.Marshal(display)
			}
		}
		handoff["properties"], _ = json.Marshal(fields)
		// New model calls declare the lane explicitly. The wire validator keeps
		// accepting older persisted calls without a role as legacy advisory.
		var handoffRequired []string
		if json.Unmarshal(handoff["required"], &handoffRequired) != nil {
			continue
		}
		if !slices.Contains(handoffRequired, "interpretation_role") {
			handoffRequired = append(handoffRequired, "interpretation_role")
		}
		if compactSelection {
			out[i].Description = "Complete the prepared-frame investigation with one JSON object. In native_fact_handoff select current measurement_refs or native_refs, then write concise independent findings and evidence limits once. reason is one short lifecycle sentence. Use aggregate_facts only for unique required facts that are not already carried by the accepted Trace observations. Use only fields listed for this tool. Use Chinese quotation marks or escaped double quotes in text; JSON arguments cannot contain string operations. " + schema.Description
			refs := make([]string, 0, len(choices))
			for _, choice := range choices {
				refs = append(refs, tool.NativeFactMeasurementRef(choice))
			}
			fields["measurement_refs"], _ = json.Marshal(map[string]any{
				"type": "array", "minItems": 1, "maxItems": 16, "uniqueItems": true,
				"description": "Select existing measurement reference strings exactly as shown for any additional whole tables. Each native_refs row citation already selects its verified table. Supply measurement_refs when no row is cited. Do not add row indices or values to these strings.",
				"items":       map[string]any{"type": "string", "enum": refs},
			})
			delete(fields, "runtime_measurements")
			handoffRequired = slices.DeleteFunc(handoffRequired, func(name string) bool { return name == "runtime_measurements" })
			// A handoff must select a whole table or a row in a current table.
			handoff["allOf"] = json.RawMessage(`[{"anyOf":[
				{"required":["measurement_refs"]},
				{"required":["interpretation_units"],"properties":{"interpretation_units":{"contains":{"required":["native_refs"],"properties":{"native_refs":{"minItems":1}}}}}}
			]}]`)
			handoff["properties"], _ = json.Marshal(fields)
		}
		handoff["required"], _ = json.Marshal(handoffRequired)
		if required {
			handoff["description"], _ = json.Marshal(tool.ManagedDeliveryNativeFactHandoffTeaching)
			var fieldsRequired []string
			_ = json.Unmarshal(root["required"], &fieldsRequired)
			found := false
			for _, name := range fieldsRequired {
				found = found || name == field
			}
			if !found {
				fieldsRequired = append(fieldsRequired, field)
			}
			root["required"], _ = json.Marshal(fieldsRequired)
		}
		if compactSelection {
			handoff["description"], _ = json.Marshal("Before the first completion submission, include at least one current measurement_refs string or a nonempty interpretation_units[].native_refs row citation. answer_display controls presentation only and does not select measurements. The program verifies the selected receipts. Keep independent causal interpretation and scope limits in interpretation or interpretation_units. To choose a business instance, use the separate top-level business_span_ref field.")
		}
		properties[field], _ = json.Marshal(handoff)
		var reason map[string]json.RawMessage
		var description string
		if json.Unmarshal(properties["reason"], &reason) == nil && json.Unmarshal(reason["description"], &description) == nil {
			if required {
				reason["description"], _ = json.Marshal("Lifecycle rationale only. Keep independent analysis in native_fact_handoff.interpretation_units or its legacy interpretation, not in reason. Delivery status and links belong to the terminal component.")
			} else {
				reason["description"], _ = json.Marshal("When native_fact_handoff is selected, reason is lifecycle rationale only; put every independent conclusion, diagnosis and scope boundary in its interpretation_units or legacy interpretation. Otherwise use the following legacy reason contract: " + description)
			}
			if compactSelection {
				reason["description"], _ = json.Marshal("With native_fact_handoff, provide one short lifecycle sentence without repeating its findings, measurements, or limits. If you use the legacy completion without a native handoff, retain the necessary conclusion here.")
			}
			properties["reason"], _ = json.Marshal(reason)
		}
		root["properties"], _ = json.Marshal(properties)
		out[i].Parameters, _ = json.Marshal(root)
	}
	return out
}
