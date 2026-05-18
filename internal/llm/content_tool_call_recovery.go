package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// recoverTextToolCalls is an opt-in provider-compatibility shim for small
// OpenAI-compatible models that receive a tool catalog but serialize
// the intended tool call into assistant content instead of the
// protocol-level tool_calls field.
//
// The boundary is intentionally narrow:
//   - only fires when the provider returned zero real tool calls;
//   - only fires when tools were actually supplied and tool_choice
//     did not forbid tools;
//   - always accepts only complete JSON / tool_call envelopes for known
//     tools; compatibility mode may repair a missing trailing closer or
//     a single-quoted JSON argument string after normal JSON decode fails;
//   - only in required-tool stages, also accepts wrapper prose around
//     fenced, tagged, or balanced JSON-object envelopes.
//
// It never reads prose-looking content or invents missing schema
// fields. For recovered text-only calls, it may prune schema-unknown
// object keys before normal tool execution so small local models that
// add commentary fields do not bounce on strict decoding.
func recoverTextToolCalls(resp Response, tools []ToolSchema, opts ChatOptions) Response {
	if len(resp.ToolCalls) > 0 || len(tools) == 0 || opts.ToolChoice == "none" {
		return resp
	}
	calls, ok := parseTextToolCallEnvelope(resp.Content, tools, textToolChoiceRequiresTool(opts.ToolChoice), textToolChoiceForcedName(opts.ToolChoice))
	if !ok || len(calls) == 0 {
		return resp
	}
	resp.Content = ""
	resp.ToolCalls = calls
	resp.StopReason = "tool_use"
	return resp
}

// recoverStructuredTextToolCalls is the provider-agnostic safety layer for
// fully explicit tool-call JSON that arrived in assistant content instead of
// protocol tool_calls. Unlike recoverTextToolCalls, this path is intentionally
// strict enough to run for every model:
//   - the whole assistant content must be a valid JSON value;
//   - the JSON must carry an explicit protocol-like envelope (`name` /
//     `arguments`, `function_call`, or `tool_calls`);
//   - no wrapper prose, fenced extraction, bare-args inference, tool-name keyed
//     maps, missing-closer repair, or unknown-field pruning is attempted here.
//
// This gives compliant remote models a harmless transport safety net while
// keeping local-model-only guessier recovery behind recover_text_tool_calls.
func recoverStructuredTextToolCalls(resp Response, tools []ToolSchema, opts ChatOptions) Response {
	if len(resp.ToolCalls) > 0 || len(tools) == 0 || opts.ToolChoice == "none" {
		return resp
	}
	calls, ok := parseStrictTextToolCallEnvelope(resp.Content, tools)
	if !ok || len(calls) == 0 {
		return resp
	}
	resp.Content = ""
	resp.ToolCalls = calls
	resp.StopReason = "tool_use"
	return resp
}

func parseStrictTextToolCallEnvelope(content string, tools []ToolSchema) ([]ToolCall, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false
	}
	var raw any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, false
	}
	allowed, infos := textToolSchemaInfos(tools)
	if len(allowed) == 0 {
		return nil, false
	}
	calls, ok := parseStrictTextToolCallValue(raw, allowed, textToolSchemaInfoByName(infos))
	if !ok || len(calls) == 0 {
		return nil, false
	}
	return renumberGeneratedContentToolCallIDs(calls), true
}

func parseStrictTextToolCallValue(v any, allowed map[string]bool, infoByName map[string]textToolSchemaInfo) ([]ToolCall, bool) {
	switch x := v.(type) {
	case []any:
		var out []ToolCall
		for _, item := range x {
			calls, ok := parseStrictTextToolCallValue(item, allowed, infoByName)
			if !ok {
				return nil, false
			}
			out = append(out, calls...)
		}
		return out, len(out) > 0
	case map[string]any:
		if rawCalls, ok := firstPresent(x, "tool_calls"); ok {
			items, ok := rawCalls.([]any)
			if !ok || len(items) == 0 {
				return nil, false
			}
			out := make([]ToolCall, 0, len(items))
			for i, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, false
				}
				call, ok := parseSingleStrictTextToolCall(m, allowed, infoByName, i)
				if !ok {
					return nil, false
				}
				out = append(out, call)
			}
			return out, true
		}
		if rawCall, ok := firstPresent(x, "function_call"); ok {
			return parseStrictTextToolCallValue(rawCall, allowed, infoByName)
		}
		call, ok := parseSingleStrictTextToolCall(x, allowed, infoByName, 0)
		if !ok {
			return nil, false
		}
		return []ToolCall{call}, true
	default:
		return nil, false
	}
}

func parseSingleStrictTextToolCall(m map[string]any, allowed map[string]bool, infoByName map[string]textToolSchemaInfo, idx int) (ToolCall, bool) {
	name, args, ok := strictTextToolCallNameAndArgs(m)
	if !ok || !allowed[name] {
		return ToolCall{}, false
	}
	info, hasInfo := infoByName[name]
	var infoPtr *textToolSchemaInfo
	if hasInfo {
		infoPtr = &info
	}
	params, ok := normalizeTextToolCallArgs(args, infoPtr)
	if !ok {
		return ToolCall{}, false
	}
	id := textToolCallStringField(m, "id")
	if id == "" {
		id = fmt.Sprintf("content_tool_call_%d", idx)
	}
	return ToolCall{ID: id, Name: name, Params: params}, true
}

func strictTextToolCallNameAndArgs(m map[string]any) (string, any, bool) {
	if fnRaw, ok := firstPresent(m, "function"); ok {
		fn, ok := fnRaw.(map[string]any)
		if !ok {
			return "", nil, false
		}
		name := textToolCallStringField(fn, "name")
		if name == "" {
			return "", nil, false
		}
		args, hasArgs := firstPresent(fn, "arguments", "parameters")
		if !hasArgs {
			args = map[string]any{}
		}
		return normalizeRecoveredToolCallName(name), args, true
	}
	name := textToolCallStringField(m, "name")
	if name == "" {
		return "", nil, false
	}
	args, ok := firstPresent(m, "arguments", "parameters")
	if !ok {
		args = map[string]any{}
	}
	return normalizeRecoveredToolCallName(name), args, true
}

func parseTextToolCallEnvelope(content string, tools []ToolSchema, allowEmbedded bool, forcedName string) ([]ToolCall, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false
	}
	allowed, schemaInfos := textToolSchemaInfos(tools)
	if len(allowed) == 0 {
		return nil, false
	}
	if calls, ok := parseTextToolCallJSON(content, allowed, schemaInfos, forcedName, allowEmbedded); ok {
		return calls, true
	}
	if blocks, ok := explicitTaggedOnlyTextToolCallBlocks(content); ok {
		out := make([]ToolCall, 0, len(blocks))
		for _, block := range blocks {
			calls, ok := parseTextToolCallJSON(block, allowed, schemaInfos, forcedName, true)
			if !ok || len(calls) == 0 {
				return nil, false
			}
			out = append(out, calls...)
		}
		if len(out) > 0 {
			return renumberGeneratedContentToolCallIDs(out), true
		}
	}
	if !allowEmbedded {
		return nil, false
	}
	blocks := embeddedTextToolCallBlocks(content)
	var out []ToolCall
	for _, block := range blocks {
		calls, ok := parseTextToolCallJSON(block, allowed, schemaInfos, forcedName, true)
		if !ok {
			continue
		}
		out = append(out, calls...)
	}
	if len(out) > 0 {
		return renumberGeneratedContentToolCallIDs(out), true
	}
	// A fenced JSON block may itself contain markdown fences inside JSON
	// strings (for example diagram.body with a ```mermaid payload). The
	// lightweight fence scanner intentionally stops at the next fence and
	// will hand parseTextToolCallJSON an incomplete object in that shape.
	// Fall back to balanced-object extraction over the original content so
	// nested markdown fences inside strings do not make the tool call vanish.
	blocks = embeddedJSONObjectBlocks(content)
	if len(blocks) == 0 {
		return nil, false
	}
	for _, block := range blocks {
		calls, ok := parseTextToolCallJSON(block, allowed, schemaInfos, forcedName, true)
		if !ok {
			continue
		}
		out = append(out, calls...)
	}
	return renumberGeneratedContentToolCallIDs(out), len(out) > 0
}

func parseTextToolCallJSON(content string, allowed map[string]bool, schemaInfos []textToolSchemaInfo, forcedName string, allowBareArgs bool) ([]ToolCall, bool) {
	content = trimWholeTextToolCallEnvelope(content)
	if content == "" {
		return nil, false
	}
	raw, ok := decodeTextToolCallJSON(content)
	if !ok {
		return nil, false
	}
	infoByName := textToolSchemaInfoByName(schemaInfos)
	if m, ok := raw.(map[string]any); ok {
		if calls, ok := parseTextToolKeyedMap(content, m, schemaInfos); ok {
			calls = sanitizeRecoveredToolCallParams(calls, schemaInfos)
			return renumberGeneratedContentToolCallIDs(calls), true
		}
	}
	calls, ok := parseTextToolCallValue(raw, allowed, infoByName)
	if !ok || len(calls) == 0 {
		if allowBareArgs {
			if call, bareOK := parseBareTextToolCallArgs(raw, schemaInfos, forcedName); bareOK {
				return []ToolCall{call}, true
			}
		}
		return nil, false
	}
	calls = sanitizeRecoveredToolCallParams(calls, schemaInfos)
	return renumberGeneratedContentToolCallIDs(calls), true
}

func decodeTextToolCallJSON(content string) (any, bool) {
	var raw any
	for _, candidate := range toolparam.JSONRepairCandidates(content) {
		if err := json.Unmarshal([]byte(candidate), &raw); err == nil {
			return raw, true
		}
	}
	repaired, ok := repairSingleQuotedJSONArgument(content)
	if !ok {
		return nil, false
	}
	if err := json.Unmarshal([]byte(repaired), &raw); err != nil {
		return nil, false
	}
	return raw, true
}

func repairSingleQuotedJSONArgument(content string) (string, bool) {
	s := strings.TrimSpace(content)
	if !strings.HasPrefix(s, "{") {
		return "", false
	}
	for _, key := range textToolArgKeys() {
		keyStart, valueStart, ok := findSingleQuotedJSONArgumentValue(s, key)
		if !ok {
			continue
		}
		valueEnd := strings.LastIndexByte(s[valueStart:], '\'')
		if valueEnd < 0 {
			return "", false
		}
		valueEnd += valueStart
		inner := strings.TrimSpace(s[valueStart:valueEnd])
		if inner == "" || (inner[0] != '{' && inner[0] != '[') {
			return "", false
		}
		var innerRaw any
		if err := json.Unmarshal([]byte(inner), &innerRaw); err != nil {
			return "", false
		}
		candidate := s[:keyStart] + inner + s[valueEnd+1:]
		var raw any
		if err := json.Unmarshal([]byte(candidate), &raw); err == nil {
			return candidate, true
		}
		repaired, ok := toolparam.RepairMissingTrailingJSONClosers(candidate)
		if !ok {
			return "", false
		}
		if err := json.Unmarshal([]byte(repaired), &raw); err != nil {
			return "", false
		}
		return repaired, true
	}
	return "", false
}

func findSingleQuotedJSONArgumentValue(s, key string) (int, int, bool) {
	token := `"` + key + `"`
	pos := strings.Index(s, token)
	if pos < 0 {
		return 0, 0, false
	}
	i := pos + len(token)
	for i < len(s) && isJSONSpace(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != ':' {
		return 0, 0, false
	}
	i++
	for i < len(s) && isJSONSpace(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != '\'' {
		return 0, 0, false
	}
	return i, i + 1, true
}

func isJSONSpace(ch byte) bool {
	switch ch {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

type textToolSchemaInfo struct {
	name       string
	required   []string
	parameters json.RawMessage
}

func textToolSchemaInfos(tools []ToolSchema) (map[string]bool, []textToolSchemaInfo) {
	allowed := make(map[string]bool, len(tools))
	infos := make([]textToolSchemaInfo, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		allowed[name] = true
		infos = append(infos, textToolSchemaInfo{
			name:       name,
			required:   textToolRequiredFields(tool.Parameters),
			parameters: tool.Parameters,
		})
	}
	return allowed, infos
}

func textToolSchemaInfoByName(infos []textToolSchemaInfo) map[string]textToolSchemaInfo {
	byName := make(map[string]textToolSchemaInfo, len(infos))
	for _, info := range infos {
		byName[info.name] = info
	}
	return byName
}

func textToolRequiredFields(schema json.RawMessage) []string {
	var raw struct {
		Required []string `json:"required"`
	}
	if len(schema) == 0 {
		return nil
	}
	if err := json.Unmarshal(schema, &raw); err != nil {
		return nil
	}
	out := make([]string, 0, len(raw.Required))
	for _, field := range raw.Required {
		if field = strings.TrimSpace(field); field != "" {
			out = append(out, field)
		}
	}
	return out
}

func textToolChoiceRequiresTool(choice string) bool {
	choice = strings.TrimSpace(choice)
	return choice == "required" || textToolChoiceForcedName(choice) != ""
}

func textToolChoiceForcedName(choice string) string {
	choice = strings.TrimSpace(choice)
	if !strings.HasPrefix(choice, "{") {
		return ""
	}
	var raw struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(choice), &raw); err != nil {
		return ""
	}
	if raw.Type != "" && raw.Type != "function" {
		return ""
	}
	return strings.TrimSpace(raw.Function.Name)
}

func parseBareTextToolCallArgs(raw any, infos []textToolSchemaInfo, forcedName string) (ToolCall, bool) {
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 || hasTextToolCallEnvelopeKey(m) {
		return ToolCall{}, false
	}
	if forcedName != "" {
		for _, info := range infos {
			if info.name != forcedName {
				continue
			}
			args := any(m)
			if wrappedArgs, ok := singleFieldTextToolArgs(m); ok {
				args = wrappedArgs
			}
			argsMap, ok := args.(map[string]any)
			if !ok || !textToolRequiredFieldsPresentForInfo(argsMap, info) {
				return ToolCall{}, false
			}
			params, ok := normalizeTextToolCallArgs(args, &info)
			if !ok {
				return ToolCall{}, false
			}
			if pruned, ok := pruneTextToolCallArgs(params, info.parameters); ok {
				params = pruned
			}
			return ToolCall{ID: "content_tool_call_0", Name: info.name, Params: params}, true
		}
		return ToolCall{}, false
	}

	var matched []bareTextToolArgMatch
	for _, info := range infos {
		if score, ok := scoreBareTextToolCallArgs(m, info); ok {
			matched = append(matched, bareTextToolArgMatch{info: info, score: score})
		}
	}
	if len(matched) == 0 {
		return ToolCall{}, false
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score == matched[j].score {
			return matched[i].info.name < matched[j].info.name
		}
		return matched[i].score > matched[j].score
	})
	if len(matched) > 1 && matched[0].score <= matched[1].score+2 {
		return ToolCall{}, false
	}
	params, ok := normalizeTextToolCallArgs(m, &matched[0].info)
	if !ok {
		return ToolCall{}, false
	}
	if pruned, ok := pruneTextToolCallArgs(params, matched[0].info.parameters); ok {
		params = pruned
	}
	return ToolCall{ID: "content_tool_call_0", Name: matched[0].info.name, Params: params}, true
}

type bareTextToolArgMatch struct {
	info  textToolSchemaInfo
	score int
}

func sanitizeRecoveredToolCallParams(calls []ToolCall, infos []textToolSchemaInfo) []ToolCall {
	if len(calls) == 0 || len(infos) == 0 {
		return calls
	}
	byName := make(map[string]textToolSchemaInfo, len(infos))
	for _, info := range infos {
		byName[info.name] = info
	}
	out := append([]ToolCall(nil), calls...)
	for i := range out {
		info, ok := byName[out[i].Name]
		if !ok {
			continue
		}
		if params, ok := pruneTextToolCallArgs(out[i].Params, info.parameters); ok {
			out[i].Params = params
		}
	}
	return out
}

func pruneTextToolCallArgs(params json.RawMessage, schema json.RawMessage) (json.RawMessage, bool) {
	if len(params) == 0 || len(schema) == 0 {
		return params, true
	}
	var val any
	if err := json.Unmarshal(params, &val); err != nil {
		return nil, false
	}
	var schemaVal any
	if err := json.Unmarshal(schema, &schemaVal); err != nil {
		return params, true
	}
	pruned, changed := pruneJSONValueBySchema(val, schemaVal)
	if !changed {
		return params, true
	}
	b, err := json.Marshal(pruned)
	if err != nil {
		return nil, false
	}
	return json.RawMessage(b), true
}

func pruneJSONValueBySchema(val any, schema any) (any, bool) {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return val, false
	}
	if arr, ok := val.([]any); ok {
		itemSchema, ok := schemaMap["items"]
		if !ok {
			return val, false
		}
		changed := false
		out := make([]any, len(arr))
		for i, item := range arr {
			pruned, itemChanged := pruneJSONValueBySchema(item, itemSchema)
			out[i] = pruned
			changed = changed || itemChanged
		}
		if changed {
			return out, true
		}
		return val, false
	}
	m, ok := val.(map[string]any)
	if !ok {
		return val, false
	}
	props, ok := schemaMap["properties"].(map[string]any)
	if !ok || len(props) == 0 || schemaAllowsAdditionalProperties(schemaMap) {
		return val, false
	}
	keyChanged := false
	if canonicalized, ok := canonicalizeJSONMapKeysBySchema(m, props); ok {
		m = canonicalized
		keyChanged = true
	}
	promotedChanged := false
	if promoted, ok := promoteNestedJSONFieldsToCurrentObject(m, props); ok {
		m = promoted
		promotedChanged = true
	}
	out := make(map[string]any, len(m))
	changed := keyChanged || promotedChanged
	for key, item := range m {
		canonicalKey, propSchema, ok := schemaPropertyForInputKey(props, key)
		if !ok {
			changed = true
			continue
		}
		pruned, itemChanged := pruneJSONValueBySchema(item, propSchema)
		out[canonicalKey] = pruned
		changed = changed || itemChanged || canonicalKey != key
	}
	if changed {
		return out, true
	}
	return val, false
}

func canonicalizeJSONMapKeysBySchema(m map[string]any, props map[string]any) (map[string]any, bool) {
	if len(m) == 0 || len(props) == 0 {
		return m, false
	}
	propNames := schemaPropertyNames(props)
	byTarget := make(map[string][]string)
	for key := range m {
		if _, exact := props[key]; exact {
			continue
		}
		if canonical, _, ok := toolparam.SchemaPropertyKeyAlias(key, propNames); ok {
			byTarget[canonical] = append(byTarget[canonical], key)
		}
	}
	if len(byTarget) == 0 {
		return m, false
	}
	targets := make([]string, 0, len(byTarget))
	for target := range byTarget {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	var out map[string]any
	for _, target := range targets {
		if _, exists := m[target]; exists {
			continue
		}
		sources := byTarget[target]
		if len(sources) != 1 {
			continue
		}
		if out == nil {
			out = copyAnyMap(m)
		}
		source := sources[0]
		out[target] = out[source]
		delete(out, source)
	}
	if out == nil {
		return m, false
	}
	return out, true
}

func promoteNestedJSONFieldsToCurrentObject(m map[string]any, props map[string]any) (map[string]any, bool) {
	if len(m) == 0 || len(props) == 0 {
		return m, false
	}
	var out map[string]any
	changed := false
	for childKey, childVal := range m {
		childSchema, ok := props[childKey].(map[string]any)
		if !ok || schemaAllowsAdditionalProperties(childSchema) {
			continue
		}
		childProps, ok := childSchema["properties"].(map[string]any)
		if !ok || len(childProps) == 0 {
			continue
		}
		childMap, ok := childVal.(map[string]any)
		if !ok || len(childMap) == 0 {
			continue
		}
		var childOut map[string]any
		for nestedKey, nestedVal := range childMap {
			if _, _, childOwns := schemaPropertyForInputKey(childProps, nestedKey); childOwns {
				continue
			}
			parent := m
			if changed {
				parent = out
			}
			parentKey, parentSchema, parentAccepts := schemaPropertyForInputKey(props, nestedKey)
			if !parentAccepts {
				continue
			}
			if _, parentAlreadyHas := parent[parentKey]; parentAlreadyHas {
				continue
			}
			if !jsonSchemaAcceptsValue(parentSchema, nestedVal) {
				continue
			}
			if !changed {
				out = copyAnyMap(m)
				changed = true
			}
			if childOut == nil {
				childOut = copyAnyMap(childMap)
			}
			out[parentKey] = nestedVal
			delete(childOut, nestedKey)
			out[childKey] = childOut
		}
	}
	if !changed {
		return m, false
	}
	return out, true
}

func schemaPropertyForInputKey(props map[string]any, key string) (string, any, bool) {
	if propSchema, ok := props[key]; ok {
		return key, propSchema, true
	}
	canonical, _, ok := toolparam.SchemaPropertyKeyAlias(key, schemaPropertyNames(props))
	if !ok {
		return "", nil, false
	}
	propSchema, ok := props[canonical]
	if !ok {
		return "", nil, false
	}
	return canonical, propSchema, true
}

func schemaPropertyNames(props map[string]any) []string {
	if len(props) == 0 {
		return nil
	}
	names := make([]string, 0, len(props))
	for name := range props {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func rawJSONPropertyNames(props map[string]json.RawMessage) []string {
	if len(props) == 0 {
		return nil
	}
	names := make([]string, 0, len(props))
	for name := range props {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func jsonSchemaAcceptsValue(schema any, raw any) bool {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	valueType := jsonSchemaTypeForValue(raw)
	if valueType == "" {
		return false
	}
	if jsonSchemaTypeAllows(schemaMap["type"], valueType) {
		return true
	}
	switch valueType {
	case "object":
		_, ok := schemaMap["properties"]
		return ok
	case "array":
		_, ok := schemaMap["items"]
		return ok
	default:
		return false
	}
}

func copyAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func schemaAllowsAdditionalProperties(schema map[string]any) bool {
	v, ok := schema["additionalProperties"]
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	_, ok = v.(map[string]any)
	return ok
}

func singleFieldTextToolArgs(m map[string]any) (any, bool) {
	if len(m) != 1 {
		return nil, false
	}
	return firstPresent(m, textToolArgKeys()...)
}

func hasTextToolCallEnvelopeKey(m map[string]any) bool {
	return hasAnySchemaLikeKey(m, []string{"name", "tool", "tool_name", "function", "function_call", "tool_calls"})
}

func textToolRequiredFieldsPresentForInfo(m map[string]any, info textToolSchemaInfo) bool {
	if len(info.required) == 0 {
		return false
	}
	schemaMap := textToolSchemaMap(info.parameters)
	props, _ := schemaMap["properties"].(map[string]any)
	names := schemaPropertyNames(props)
	if len(names) == 0 {
		names = info.required
	}
	return textToolRequiredFieldsPresentWithNames(m, info.required, names)
}

func textToolRequiredFieldsPresentWithNames(m map[string]any, required []string, propertyNames []string) bool {
	if len(required) == 0 {
		return false
	}
	for _, field := range required {
		if !mapHasCanonicalSchemaField(m, field, propertyNames) {
			return false
		}
	}
	return true
}

func mapHasCanonicalSchemaField(m map[string]any, field string, propertyNames []string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	if _, ok := m[field]; ok {
		return true
	}
	matches := 0
	for key := range m {
		canonical, _, ok := toolparam.SchemaPropertyKeyAlias(key, propertyNames)
		if !ok || canonical != field {
			continue
		}
		matches++
		if matches > 1 {
			return false
		}
	}
	return matches == 1
}

func scoreBareTextToolCallArgs(m map[string]any, info textToolSchemaInfo) (int, bool) {
	schemaMap := textToolSchemaMap(info.parameters)
	props, _ := schemaMap["properties"].(map[string]any)
	if len(info.required) > 0 {
		names := schemaPropertyNames(props)
		if len(names) == 0 {
			names = info.required
		}
		if !textToolRequiredFieldsPresentWithNames(m, info.required, names) {
			return 0, false
		}
	}
	if len(props) == 0 && len(info.required) == 0 {
		return 0, false
	}
	// Two schema shapes are common in the tool catalog:
	//   1. required-only legacy schemas: recover when every required
	//      field is present, even if the schema omitted properties.
	//   2. optional-only patch/update schemas: recover only when the
	//      bare object contains schema-declared fields with compatible
	//      JSON shapes. This handles tools such as answer-document
	//      patches without naming those tools here.
	score := len(info.required) * 8
	unknown := 0
	known := 0
	compatible := 0
	for key, val := range m {
		if len(props) == 0 {
			continue
		}
		_, propSchema, propKnown := schemaPropertyForInputKey(props, key)
		if !propKnown {
			unknown++
			continue
		}
		known++
		score += 4
		if jsonSchemaAcceptsValue(propSchema, val) {
			compatible++
			score += 2
		}
		nestedScore, nestedOK := scoreBareSchemaValue(val, propSchema)
		if !nestedOK {
			return 0, false
		}
		score += nestedScore
	}
	if unknown > 0 {
		if len(props) > 0 {
			score -= unknown * 3
		}
	}
	if len(info.required) == 0 {
		if compatible == 0 {
			return 0, false
		}
		if unknown > known {
			return 0, false
		}
		score += compatible * 6
	}
	return score, true
}

func textToolSchemaMap(schema json.RawMessage) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(schema, &raw); err != nil {
		return nil
	}
	return raw
}

func scoreBareSchemaValue(val any, schema any) (int, bool) {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return 0, true
	}
	itemSchema, hasItems := schemaMap["items"]
	arr, isArray := val.([]any)
	if !hasItems || !isArray {
		return 0, true
	}
	itemMap, ok := itemSchema.(map[string]any)
	if !ok {
		return 0, true
	}
	itemProps, _ := itemMap["properties"].(map[string]any)
	itemRequired := textRequiredFromSchemaMap(itemMap)
	if len(itemProps) == 0 && len(itemRequired) == 0 {
		return 0, true
	}
	score := 0
	for _, item := range arr {
		itemObj, ok := item.(map[string]any)
		if !ok {
			if len(itemRequired) > 0 {
				return 0, false
			}
			continue
		}
		if len(itemRequired) > 0 {
			itemNames := schemaPropertyNames(itemProps)
			if len(itemNames) == 0 {
				itemNames = itemRequired
			}
			if !textToolRequiredFieldsPresentWithNames(itemObj, itemRequired, itemNames) {
				return 0, false
			}
		}
		score += len(itemRequired) * 4
		unknown := 0
		for key, itemVal := range itemObj {
			_, propSchema, known := schemaPropertyForInputKey(itemProps, key)
			if !known {
				unknown++
				continue
			}
			score += 2
			if jsonSchemaAcceptsValue(propSchema, itemVal) {
				score++
			}
		}
		score -= unknown
	}
	return score, true
}

func textRequiredFromSchemaMap(schema map[string]any) []string {
	raw, ok := schema["required"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func normalizeRecoveredToolCallName(s string) string {
	return strings.TrimSpace(s)
}

func textToolArgKeys() []string {
	return []string{"arguments", "parameters", "input", "params", "args", "tool_input", "action_input"}
}

func textToolNameKeys() []string {
	return []string{"name", "tool", "tool_name", "action", "action_name", "function_name", "fn"}
}

func textToolCallNameAndArgs(m map[string]any) (string, any, bool) {
	if fnRaw, ok := firstPresent(m, "function"); ok {
		if fn, ok := fnRaw.(map[string]any); ok {
			name := textToolCallStringField(fn, "name")
			args, hasArgs := firstPresent(fn, textToolArgKeys()...)
			if name != "" {
				if !hasArgs {
					args = map[string]any{}
				}
				return normalizeRecoveredToolCallName(name), args, true
			}
		}
		if name, ok := fnRaw.(string); ok && strings.TrimSpace(name) != "" {
			args, hasArgs := firstPresent(m, textToolArgKeys()...)
			if !hasArgs {
				args = map[string]any{}
			}
			return normalizeRecoveredToolCallName(name), args, true
		}
	}
	nameVals := make([]string, 0, len(textToolNameKeys()))
	for _, key := range textToolNameKeys() {
		nameVals = append(nameVals, textToolCallStringField(m, key))
	}
	name := firstNonEmptyString(nameVals...)
	if name == "" {
		return "", nil, false
	}
	args, ok := firstPresent(m, textToolArgKeys()...)
	if !ok {
		args = map[string]any{}
	}
	return normalizeRecoveredToolCallName(name), args, true
}

func normalizeTextToolCallArgs(raw any, info *textToolSchemaInfo) (json.RawMessage, bool) {
	if raw == nil {
		return json.RawMessage(`{}`), true
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return json.RawMessage(`{}`), true
		}
		if decoded, ok := decodeTextToolCallJSON(s); ok {
			raw = decoded
		} else {
			if looksLikeJSONContainer(s) {
				return nil, false
			}
			raw = s
		}
	}
	if _, ok := raw.(map[string]any); !ok {
		wrapped, wrapOK := wrapSingleRequiredTextToolArg(raw, info)
		if !wrapOK {
			return nil, false
		}
		raw = wrapped
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	params := json.RawMessage(b)
	if info != nil && len(info.parameters) > 0 {
		params, _ = toolparam.Normalize(params, info.parameters, types.ToolParamCompatConfig{Mode: types.ToolParamCompatRepair})
	}
	return params, true
}

func looksLikeJSONContainer(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

func wrapSingleRequiredTextToolArg(raw any, info *textToolSchemaInfo) (map[string]any, bool) {
	if info == nil || len(info.required) != 1 {
		return nil, false
	}
	field := info.required[0]
	if field == "" || !textToolSchemaPropertyAcceptsValue(info.parameters, field, raw) {
		return nil, false
	}
	return map[string]any{field: raw}, true
}

func textToolSchemaPropertyAcceptsValue(schema json.RawMessage, field string, raw any) bool {
	_, ok := textToolSchemaPropertyAcceptsFieldValue(schema, field, raw)
	return ok
}

func textToolSchemaPropertyAcceptsFieldValue(schema json.RawMessage, field string, raw any) (string, bool) {
	valueType := jsonSchemaTypeForValue(raw)
	if valueType == "" || len(schema) == 0 {
		return "", false
	}
	var objectSchema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &objectSchema); err != nil {
		return "", false
	}
	canonicalField := field
	prop, ok := objectSchema.Properties[field]
	if !ok {
		if canonical, _, aliasOK := toolparam.SchemaPropertyKeyAlias(field, rawJSONPropertyNames(objectSchema.Properties)); aliasOK {
			canonicalField = canonical
			prop, ok = objectSchema.Properties[canonical]
		}
	}
	if !ok || len(prop) == 0 {
		return "", false
	}
	var propSchema struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(prop, &propSchema); err != nil {
		return "", false
	}
	if !jsonSchemaTypeAllows(propSchema.Type, valueType) {
		return "", false
	}
	return canonicalField, true
}

func jsonSchemaTypeForValue(raw any) string {
	switch raw.(type) {
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	default:
		return ""
	}
}

func jsonSchemaTypeAllows(raw any, valueType string) bool {
	switch x := raw.(type) {
	case string:
		return x == valueType || (x == "integer" && valueType == "number")
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok && (s == valueType || (s == "integer" && valueType == "number")) {
				return true
			}
		}
	}
	return false
}

func textToolCallStringField(m map[string]any, key string) string {
	v, ok := firstPresent(m, key)
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstPresent(m map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	for _, key := range keys {
		var matched any
		matches := 0
		for rawKey, value := range m {
			canonical, _, ok := toolparam.SchemaPropertyKeyAlias(rawKey, keys)
			if !ok || canonical != key {
				continue
			}
			matched = value
			matches++
			if matches > 1 {
				return nil, false
			}
		}
		if matches == 1 {
			return matched, true
		}
	}
	return nil, false
}

func hasAnySchemaLikeKey(m map[string]any, keys []string) bool {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	for rawKey := range m {
		if _, _, ok := toolparam.SchemaPropertyKeyAlias(rawKey, keys); ok {
			return true
		}
	}
	return false
}

func firstNonEmptyString(vals ...string) string {
	for _, val := range vals {
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func trimWholeTextToolCallEnvelope(content string) string {
	s := strings.TrimSpace(content)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 3 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") &&
			strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			s = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	for _, tag := range []string{"tool_call", "minimax:tool_call"} {
		open := "<" + tag + ">"
		close := "</" + tag + ">"
		if strings.HasPrefix(s, open) && strings.HasSuffix(s, close) {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, open), close))
		}
	}
	return s
}

type embeddedToolCallBlock struct {
	start int
	end   int
	body  string
}

func explicitTaggedOnlyTextToolCallBlocks(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var blocks []embeddedToolCallBlock
	for _, tag := range []string{"tool_call", "minimax:tool_call"} {
		blocks = append(blocks, embeddedTaggedBlocks(s, "<"+tag+">", "</"+tag+">")...)
	}
	if len(blocks) == 0 {
		return nil, false
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].start < blocks[j].start })
	pos := 0
	for _, block := range blocks {
		if block.start < pos || block.end <= block.start {
			return nil, false
		}
		if strings.TrimSpace(s[pos:block.start]) != "" {
			return nil, false
		}
		pos = block.end
	}
	if strings.TrimSpace(s[pos:]) != "" {
		return nil, false
	}
	return embeddedBlockBodies(blocks), true
}

func embeddedTextToolCallBlocks(s string) []string {
	if blocks := embeddedFencedBlocks(s); len(blocks) > 0 {
		return embeddedBlockBodies(blocks)
	}
	var blocks []embeddedToolCallBlock
	for _, tag := range []string{"tool_call", "minimax:tool_call"} {
		blocks = append(blocks, embeddedTaggedBlocks(s, "<"+tag+">", "</"+tag+">")...)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].start < blocks[j].start })
	return embeddedBlockBodies(blocks)
}

func embeddedBlockBodies(blocks []embeddedToolCallBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if body := strings.TrimSpace(block.body); body != "" {
			out = append(out, body)
		}
	}
	return out
}

func embeddedFencedBlocks(s string) []embeddedToolCallBlock {
	var blocks []embeddedToolCallBlock
	for pos := 0; pos < len(s); {
		relStart := strings.Index(s[pos:], "```")
		if relStart < 0 {
			break
		}
		start := pos + relStart
		afterStart := s[start+3:]
		lineEnd := strings.Index(afterStart, "\n")
		if lineEnd < 0 {
			break
		}
		bodyStart := start + 3 + lineEnd + 1
		endRel := strings.Index(s[bodyStart:], "```")
		if endRel < 0 {
			break
		}
		bodyEnd := bodyStart + endRel
		end := bodyEnd + 3
		blocks = append(blocks, embeddedToolCallBlock{start: start, end: end, body: s[bodyStart:bodyEnd]})
		pos = end
	}
	return blocks
}

func embeddedTaggedBlocks(s, open, close string) []embeddedToolCallBlock {
	var blocks []embeddedToolCallBlock
	for pos := 0; pos < len(s); {
		relStart := strings.Index(s[pos:], open)
		if relStart < 0 {
			break
		}
		start := pos + relStart
		bodyStart := start + len(open)
		endRel := strings.Index(s[bodyStart:], close)
		if endRel < 0 {
			break
		}
		bodyEnd := bodyStart + endRel
		end := bodyEnd + len(close)
		blocks = append(blocks, embeddedToolCallBlock{start: start, end: end, body: s[bodyStart:bodyEnd]})
		pos = end
	}
	return blocks
}

func embeddedJSONObjectBlocks(s string) []string {
	var blocks []string
	for pos := 0; pos < len(s); {
		startRel := strings.IndexByte(s[pos:], '{')
		if startRel < 0 {
			break
		}
		start := pos + startRel
		end, ok := balancedJSONObjectEnd(s, start)
		if !ok {
			return nil
		}
		blocks = append(blocks, strings.TrimSpace(s[start:end]))
		pos = end
	}
	return blocks
}

func balancedJSONObjectEnd(s string, start int) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func parseTextToolCallValue(v any, allowed map[string]bool, infoByName map[string]textToolSchemaInfo) ([]ToolCall, bool) {
	switch x := v.(type) {
	case []any:
		var out []ToolCall
		for _, item := range x {
			calls, ok := parseTextToolCallValue(item, allowed, infoByName)
			if !ok {
				return nil, false
			}
			out = append(out, calls...)
		}
		return out, len(out) > 0
	case map[string]any:
		if rawCalls, ok := firstPresent(x, "tool_calls"); ok {
			return parseTextToolCallsArray(rawCalls, allowed, infoByName)
		}
		if rawCall, ok := firstPresent(x, "function_call"); ok {
			return parseTextToolCallValue(rawCall, allowed, infoByName)
		}
		call, ok := parseSingleTextToolCall(x, allowed, infoByName, 0)
		if !ok {
			return nil, false
		}
		return []ToolCall{call}, true
	default:
		return nil, false
	}
}

func parseTextToolCallsArray(raw any, allowed map[string]bool, infoByName map[string]textToolSchemaInfo) ([]ToolCall, bool) {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, false
	}
	out := make([]ToolCall, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		call, ok := parseSingleTextToolCall(m, allowed, infoByName, i)
		if !ok {
			return nil, false
		}
		out = append(out, call)
	}
	return out, true
}

func parseSingleTextToolCall(m map[string]any, allowed map[string]bool, infoByName map[string]textToolSchemaInfo, idx int) (ToolCall, bool) {
	name, args, ok := textToolCallNameAndArgs(m)
	if !ok || !allowed[name] {
		return ToolCall{}, false
	}
	info, hasInfo := infoByName[name]
	var infoPtr *textToolSchemaInfo
	if hasInfo {
		infoPtr = &info
	}
	params, ok := normalizeTextToolCallArgs(args, infoPtr)
	if !ok {
		return ToolCall{}, false
	}
	id := textToolCallStringField(m, "id")
	if id == "" {
		id = fmt.Sprintf("content_tool_call_%d", idx)
	}
	return ToolCall{ID: id, Name: name, Params: params}, true
}

type textToolKeyAlias struct {
	name  string
	info  textToolSchemaInfo
	exact bool
	order int
}

type parsedTextToolKeyCall struct {
	key   string
	pos   int
	order int
	call  ToolCall
}

type pendingTextToolKeyCall struct {
	key     string
	rawArgs any
	alias   textToolKeyAlias
}

func parseTextToolKeyedMap(source string, m map[string]any, infos []textToolSchemaInfo) ([]ToolCall, bool) {
	if len(m) == 0 || len(infos) == 0 || hasTextToolCallEnvelopeKey(m) {
		return nil, false
	}
	aliases := textToolKeyAliasMap(infos)
	if len(aliases) == 0 {
		return nil, false
	}
	pending := make([]pendingTextToolKeyCall, 0, len(m))
	shared := make(map[string]any)
	seen := make(map[string]bool, len(m))
	for key, rawArgs := range m {
		key = strings.TrimSpace(key)
		alias, ok := textToolKeyAliasForInputKey(aliases, key)
		if !ok {
			shared[key] = rawArgs
			continue
		}
		if seen[alias.name] {
			return nil, false
		}
		pending = append(pending, pendingTextToolKeyCall{
			key:     key,
			rawArgs: rawArgs,
			alias:   alias,
		})
		seen[alias.name] = true
	}
	if len(pending) == 0 {
		return nil, false
	}
	sharedByCall, ok := partitionSharedTextToolKeyedArgs(shared, pending)
	if !ok {
		return nil, false
	}
	parsed := make([]parsedTextToolKeyCall, 0, len(pending))
	for i, item := range pending {
		params, ok := normalizeTextToolKeyedArgsWithShared(item.rawArgs, &item.alias.info, item.alias.exact, sharedByCall[i])
		if !ok {
			return nil, false
		}
		parsed = append(parsed, parsedTextToolKeyCall{
			key:   item.key,
			pos:   textToolJSONKeyPosition(source, item.key),
			order: item.alias.order,
			call: ToolCall{
				Name:   item.alias.name,
				Params: params,
			},
		})
	}
	if len(parsed) == 0 {
		return nil, false
	}
	sort.SliceStable(parsed, func(i, j int) bool {
		if parsed[i].pos >= 0 && parsed[j].pos >= 0 && parsed[i].pos != parsed[j].pos {
			return parsed[i].pos < parsed[j].pos
		}
		if (parsed[i].pos >= 0) != (parsed[j].pos >= 0) {
			return parsed[i].pos >= 0
		}
		if parsed[i].order != parsed[j].order {
			return parsed[i].order < parsed[j].order
		}
		return parsed[i].key < parsed[j].key
	})
	out := make([]ToolCall, len(parsed))
	for i, item := range parsed {
		item.call.ID = fmt.Sprintf("content_tool_call_%d", i)
		out[i] = item.call
	}
	return out, true
}

func partitionSharedTextToolKeyedArgs(shared map[string]any, pending []pendingTextToolKeyCall) ([]map[string]any, bool) {
	out := make([]map[string]any, len(pending))
	if len(shared) == 0 {
		return out, true
	}
	for key, val := range shared {
		if strings.TrimSpace(key) == "" {
			return nil, false
		}
		matched := -1
		var matchedField string
		for i, item := range pending {
			if field, ok := textToolSchemaPropertyAcceptsFieldValue(item.alias.info.parameters, key, val); ok {
				if matched >= 0 {
					return nil, false
				}
				matched = i
				matchedField = field
			}
		}
		if matched < 0 {
			return nil, false
		}
		if out[matched] == nil {
			out[matched] = make(map[string]any)
		}
		out[matched][matchedField] = val
	}
	return out, true
}

func textToolKeyAliasForInputKey(aliases map[string]textToolKeyAlias, key string) (textToolKeyAlias, bool) {
	if alias, ok := aliases[key]; ok {
		return alias, true
	}
	keys := make([]string, 0, len(aliases))
	for candidate := range aliases {
		keys = append(keys, candidate)
	}
	sort.Strings(keys)
	canonical, _, ok := toolparam.SchemaPropertyKeyAlias(key, keys)
	if !ok {
		return textToolKeyAlias{}, false
	}
	alias, ok := aliases[canonical]
	return alias, ok
}

func textToolKeyAliasMap(infos []textToolSchemaInfo) map[string]textToolKeyAlias {
	aliases := make(map[string]textToolKeyAlias, len(infos))
	ambiguous := make(map[string]bool)
	for i, info := range infos {
		for _, candidate := range textToolKeyAliases(info.name) {
			if candidate.key == "" || (ambiguous[candidate.key] && !candidate.exact) {
				continue
			}
			alias := textToolKeyAlias{name: info.name, info: info, exact: candidate.exact, order: i}
			if prev, ok := aliases[candidate.key]; ok && prev.name != info.name {
				switch {
				case prev.exact && !candidate.exact:
					continue
				case !prev.exact && candidate.exact:
					aliases[candidate.key] = alias
					ambiguous[candidate.key] = false
				default:
					delete(aliases, candidate.key)
					ambiguous[candidate.key] = true
				}
				continue
			}
			aliases[candidate.key] = alias
		}
	}
	return aliases
}

type textToolKeyAliasCandidate struct {
	key   string
	exact bool
}

func textToolKeyAliases(name string) []textToolKeyAliasCandidate {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	out := []textToolKeyAliasCandidate{{key: name, exact: true}}
	if plural := pluralTextToolAlias(name); plural != "" && plural != name {
		out = append(out, textToolKeyAliasCandidate{key: plural, exact: false})
	}
	if base, ok := strings.CutPrefix(name, "emit_"); ok && base != "" {
		out = append(out, textToolKeyAliasCandidate{key: base, exact: false})
		if plural := pluralTextToolAlias(base); plural != "" && plural != base {
			out = append(out,
				textToolKeyAliasCandidate{key: plural, exact: false},
				textToolKeyAliasCandidate{key: "emit_" + plural, exact: false},
			)
		}
	}
	return uniqueTextToolKeyAliases(out)
}

func uniqueTextToolKeyAliases(in []textToolKeyAliasCandidate) []textToolKeyAliasCandidate {
	seen := make(map[string]bool, len(in))
	out := make([]textToolKeyAliasCandidate, 0, len(in))
	for _, item := range in {
		if item.key == "" || seen[item.key] {
			continue
		}
		seen[item.key] = true
		out = append(out, item)
	}
	return out
}

func pluralTextToolAlias(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, "s") {
		return s
	}
	if strings.HasSuffix(s, "y") && len(s) > 1 {
		prev := s[len(s)-2]
		if !strings.ContainsRune("aeiou", rune(prev)) {
			return strings.TrimSuffix(s, "y") + "ies"
		}
	}
	return s + "s"
}

func normalizeTextToolKeyedArgs(raw any, info *textToolSchemaInfo, exact bool) (json.RawMessage, bool) {
	return normalizeTextToolKeyedArgsWithShared(raw, info, exact, nil)
}

func normalizeTextToolKeyedArgsWithShared(raw any, info *textToolSchemaInfo, exact bool, shared map[string]any) (json.RawMessage, bool) {
	args, ok := unwrapTextToolKeyedArgs(raw)
	if !ok {
		return nil, false
	}
	if info != nil && !exact && len(info.required) == 0 {
		return nil, false
	}
	argMap, mapOK := args.(map[string]any)
	if !mapOK {
		if wrapped, wrapOK := wrapSingleRequiredTextToolArg(args, info); wrapOK {
			argMap = wrapped
			args = wrapped
		} else if wrapped, wrapOK := wrapNamedTextToolArg(args, info, "items"); wrapOK {
			argMap = wrapped
			args = wrapped
		}
	}
	if len(shared) > 0 {
		if argMap == nil {
			return nil, false
		}
		merged := copyAnyMap(argMap)
		for key, val := range shared {
			if _, exists := merged[key]; exists {
				continue
			}
			merged[key] = val
		}
		argMap = merged
		args = merged
	}
	if info != nil && !exact && len(info.required) > 0 && !textToolRequiredFieldsPresentForInfo(argMap, *info) {
		return nil, false
	}
	return normalizeTextToolCallArgs(args, info)
}

func unwrapTextToolKeyedArgs(raw any) (any, bool) {
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return map[string]any{}, true
		}
		if decoded, ok := decodeTextToolCallJSON(s); ok {
			raw = decoded
		} else {
			if looksLikeJSONContainer(s) {
				return nil, false
			}
			raw = s
		}
	}
	if m, ok := raw.(map[string]any); ok {
		if inner, ok := singleFieldTextToolArgs(m); ok {
			return unwrapTextToolKeyedArgs(inner)
		}
	}
	return raw, true
}

func wrapNamedTextToolArg(raw any, info *textToolSchemaInfo, field string) (map[string]any, bool) {
	if info == nil || strings.TrimSpace(field) == "" {
		return nil, false
	}
	if !textToolSchemaPropertyAcceptsValue(info.parameters, field, raw) {
		return nil, false
	}
	return map[string]any{field: raw}, true
}

func textToolJSONKeyPosition(source, key string) int {
	if source == "" || key == "" {
		return -1
	}
	quoted, err := json.Marshal(key)
	if err != nil {
		return -1
	}
	return strings.Index(source, string(quoted))
}

func renumberGeneratedContentToolCallIDs(calls []ToolCall) []ToolCall {
	for i := range calls {
		if calls[i].ID == "" || strings.HasPrefix(calls[i].ID, "content_tool_call_") {
			calls[i].ID = fmt.Sprintf("content_tool_call_%d", i)
		}
	}
	return calls
}
