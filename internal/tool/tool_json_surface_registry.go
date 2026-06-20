package tool

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/types"
)

type jsonSurfaceTool interface {
	Name() string
	Parameters() json.RawMessage
}

var (
	toolJSONSurfaceRegistryOnce sync.Once
	toolJSONSurfaceRegistry     map[string]types.ToolJSONSurfaceDescriptor
)

func attachToolJSONSurfaceMetadata(toolName string, repair *types.ToolRepair) *types.ToolRepair {
	if repair == nil {
		return nil
	}
	surface, ok := toolJSONSurfaceDescriptorForRepair(toolName, repair)
	if !ok {
		return repair
	}
	encoded := types.ToolJSONSurfaceDescriptorJSON(surface)
	if encoded == "" {
		return repair
	}
	if repair.Metadata == nil {
		repair.Metadata = map[string]string{}
	}
	repair.Metadata[types.ToolJSONSurfaceMetadataKey] = encoded
	return repair
}

func toolJSONSurfaceDescriptorForRepair(toolName string, repair *types.ToolRepair) (types.ToolJSONSurfaceDescriptor, bool) {
	if repair == nil {
		return types.ToolJSONSurfaceDescriptor{}, false
	}
	surface, ok := toolJSONSurfaceDescriptorForTool(toolName)
	if !ok {
		surface = types.ToolJSONSurfaceDescriptor{ToolName: toolName}
	}
	surface.ReasonCode = firstNonEmptyToolJSONSurfaceString(repair.Metadata["reason_code"], repair.Metadata["repair_status"], repair.Code)
	surface.FailingFieldPaths = append(surface.FailingFieldPaths, toolJSONSurfaceFailingFieldPaths(repair.Fields, surface.AcceptedFieldPaths)...)
	surface = types.NormalizeToolJSONSurfaceDescriptor(surface)
	return surface, !surface.Empty()
}

func toolJSONSurfaceDescriptorForTool(toolName string) (types.ToolJSONSurfaceDescriptor, bool) {
	toolJSONSurfaceRegistryOnce.Do(func() {
		toolJSONSurfaceRegistry = buildToolJSONSurfaceRegistry()
	})
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return types.ToolJSONSurfaceDescriptor{}, false
	}
	surface, ok := toolJSONSurfaceRegistry[toolName]
	if !ok {
		return types.ToolJSONSurfaceDescriptor{}, false
	}
	return types.NormalizeToolJSONSurfaceDescriptor(surface), true
}

func buildToolJSONSurfaceRegistry() map[string]types.ToolJSONSurfaceDescriptor {
	tools := []jsonSurfaceTool{
		&EmitAnalysis{},
		&EmitAnswerDocument{},
		&EmitAnswerDocumentPatch{},
		&EmitAnswerSymbol{},
		&EmitChangePlan{},
		&EmitEvidence{},
		&EmitHypothesisVerdict{},
		&EmitInvestigationComplete{},
		&EmitLogSegmentation{},
		&EmitLogTriage{},
		&EmitMultiRepoFocus{},
		&EmitPerfSegmentation{},
		&EmitPerfTrace{},
		&EmitPlanChange{},
		&EmitPlanSkeleton{},
		&EmitTestResults{},
		&EmitWriteAnalysis{},
		&EmitWriteWorkflowDecision{},
	}
	out := make(map[string]types.ToolJSONSurfaceDescriptor, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := strings.TrimSpace(tool.Name())
		if name == "" {
			continue
		}
		surface := compileToolJSONSurfaceDescriptor(name, tool.Parameters())
		if !surface.Empty() {
			out[name] = surface
		}
	}
	return out
}

func compileToolJSONSurfaceDescriptor(toolName string, schema json.RawMessage) types.ToolJSONSurfaceDescriptor {
	surface := types.ToolJSONSurfaceDescriptor{ToolName: strings.TrimSpace(toolName)}
	if len(schema) == 0 {
		return surface
	}
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return surface
	}
	acceptedFields := map[string]bool{}
	acceptedEnums := map[string][]string{}
	walkToolJSONSchemaSurface(root, "$", acceptedFields, acceptedEnums, 0)
	for field := range acceptedFields {
		surface.AcceptedFieldPaths = append(surface.AcceptedFieldPaths, field)
	}
	sort.Strings(surface.AcceptedFieldPaths)
	if len(acceptedEnums) > 0 {
		surface.AcceptedEnums = acceptedEnums
	}
	return types.NormalizeToolJSONSurfaceDescriptor(surface)
}

func walkToolJSONSchemaSurface(node map[string]any, path string, acceptedFields map[string]bool, acceptedEnums map[string][]string, depth int) {
	if node == nil || depth > 8 {
		return
	}
	if vals := schemaStringEnumValues(node["enum"]); len(vals) > 0 && path != "" && path != "$" {
		acceptedEnums[path] = vals
	}
	if props, ok := node["properties"].(map[string]any); ok {
		keys := make([]string, 0, len(props))
		for key := range props {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, _ := props[key].(map[string]any)
			childPath := jsonSurfaceChildPath(path, key)
			acceptedFields[childPath] = true
			walkToolJSONSchemaSurface(child, childPath, acceptedFields, acceptedEnums, depth+1)
		}
	}
	if item, ok := node["items"].(map[string]any); ok {
		walkToolJSONSchemaSurface(item, path+"[]", acceptedFields, acceptedEnums, depth+1)
	}
	for _, branchKey := range []string{"oneOf", "anyOf", "allOf"} {
		branches, _ := node[branchKey].([]any)
		for _, branch := range branches {
			child, _ := branch.(map[string]any)
			walkToolJSONSchemaSurface(child, path, acceptedFields, acceptedEnums, depth+1)
		}
	}
}

func schemaStringEnumValues(raw any) []string {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		s, ok := value.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func jsonSurfaceChildPath(parent, key string) string {
	key = strings.TrimSpace(key)
	if parent == "" || parent == "$" {
		return "$." + key
	}
	return parent + "." + key
}

func toolJSONSurfaceFailingFieldPaths(fields, accepted []string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if path := toolJSONSurfaceFieldPath(field, accepted); path != "" {
			out = append(out, path)
		}
	}
	return out
}

func toolJSONSurfaceFieldPath(field string, accepted []string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	if strings.HasPrefix(field, "$.") || strings.HasPrefix(field, "$[") {
		return field
	}
	if strings.HasPrefix(field, "/") {
		parts := strings.Split(strings.Trim(field, "/"), "/")
		if len(parts) == 0 {
			return "$"
		}
		return "$." + strings.Join(parts, ".")
	}
	for _, acceptedPath := range accepted {
		if toolJSONSurfacePathMatchesField(acceptedPath, field) {
			return acceptedPath
		}
	}
	if strings.ContainsAny(field, ".[]") {
		return "$." + strings.TrimPrefix(field, ".")
	}
	return "$." + field
}

func toolJSONSurfacePathMatchesField(path, field string) bool {
	path = strings.TrimSpace(path)
	field = strings.Trim(strings.TrimSpace(field), ".")
	if path == "" || field == "" {
		return false
	}
	if path == field || strings.TrimPrefix(path, "$.") == field {
		return true
	}
	tail := path
	if idx := strings.LastIndex(tail, "."); idx >= 0 {
		tail = tail[idx+1:]
	}
	tail = strings.TrimSuffix(tail, "[]")
	return tail == field
}

func firstNonEmptyToolJSONSurfaceString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
