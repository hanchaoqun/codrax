package toolparam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Repair describes one deterministic, schema-proven tool-argument rewrite.
// It is telemetry only; the caller decides whether audit mode logs it or repair
// mode applies the normalized payload.
type Repair struct {
	Path string
	Rule string
	From string
	To   string
}

// Report is returned for both audit and repair modes.
type Report struct {
	Repairs []Repair
}

func (r Report) Changed() bool { return len(r.Repairs) > 0 }

func (r Report) Summary(limit int) string {
	if len(r.Repairs) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(r.Repairs) {
		limit = len(r.Repairs)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		repair := r.Repairs[i]
		parts = append(parts, fmt.Sprintf("%s %s->%s via %s", repair.Path, repair.From, repair.To, repair.Rule))
	}
	if len(r.Repairs) > limit {
		parts = append(parts, fmt.Sprintf("... %d more", len(r.Repairs)-limit))
	}
	return strings.Join(parts, "; ")
}

type schemaNode struct {
	Type       any                        `json:"type"`
	Properties map[string]json.RawMessage `json:"properties"`
	Items      json.RawMessage            `json:"items"`
}

var envelopeCarrierKeyOrder = []string{
	"arguments",
	"parameters",
	"params",
	"input",
	"args",
	"tool_input",
	"toolInput",
	"action_input",
	"actionInput",
}

// Normalize applies bounded, schema-aware compatibility rewrites to a tool
// parameter object. It never fills missing required fields, invents values,
// reads prose, or drops unknown tool-parameter keys. Empty/off policy returns
// raw unchanged.
func Normalize(raw json.RawMessage, schema json.RawMessage, cfg types.ToolParamCompatConfig) (json.RawMessage, Report) {
	mode := cfg.NormalizedMode()
	if mode != types.ToolParamCompatAudit && mode != types.ToolParamCompatRepair {
		return raw, Report{}
	}
	value, ok := decodeJSONValue(raw)
	if !ok {
		return raw, Report{}
	}
	normalized, repairs := normalizeValue(value, schema, "$", cfg)
	if len(repairs) == 0 {
		return raw, Report{}
	}
	report := Report{Repairs: repairs}
	if mode == types.ToolParamCompatAudit {
		return raw, report
	}
	encoded, err := json.Marshal(normalized)
	if err != nil || !json.Valid(encoded) {
		return raw, Report{}
	}
	return json.RawMessage(encoded), report
}

func normalizeValue(value any, schema json.RawMessage, path string, cfg types.ToolParamCompatConfig) (any, []Repair) {
	node, ok := parseSchema(schema)
	if !ok {
		return value, nil
	}

	if schemaExpectsObject(node) {
		if s, ok := value.(string); ok && !typeAllows(node.Type, "string") {
			if decoded, rule, ok := decodeJSONStringAs(s, "object"); ok {
				repairs := []Repair{repair(path, rule, valueKind(value), "object")}
				value = decoded
				if nested, ok := value.(map[string]any); ok {
					out, nestedRepairs := normalizeObject(nested, node, path, cfg)
					return out, append(repairs, nestedRepairs...)
				}
				return value, repairs
			}
		}
		if m, ok := value.(map[string]any); ok {
			if unwrapped, rule, ok := unwrapToolArgumentEnvelope(m, node); ok {
				repairs := []Repair{repair(path, rule, valueKind(value), "object")}
				out, nestedRepairs := normalizeObject(unwrapped, node, path, cfg)
				return out, append(repairs, nestedRepairs...)
			}
			return normalizeObject(m, node, path, cfg)
		}
	}

	if schemaExpectsArray(node) {
		if s, ok := value.(string); ok && !typeAllows(node.Type, "string") {
			if decoded, rule, ok := decodeJSONStringAs(s, "array"); ok {
				repairs := []Repair{repair(path, rule, valueKind(value), "array")}
				value = decoded
				if arr, ok := value.([]any); ok {
					out, nestedRepairs := normalizeArray(arr, node, path, cfg)
					return out, append(repairs, nestedRepairs...)
				}
				return value, repairs
			}
			if cfg.SplitStringArraysEnabled() && arrayItemsAllowOnlyString(node) {
				if arr, ok := splitStringArray(s); ok {
					return arr, []Repair{repair(path, "delimited_string_array", valueKind(value), "array")}
				}
			}
		}
		if arr, ok := value.([]any); ok {
			return normalizeArray(arr, node, path, cfg)
		}
	}

	if s, ok := value.(string); ok && !typeAllows(node.Type, "string") {
		if typeAllows(node.Type, "integer") {
			if v, ok := parseJSONStringInteger(s); ok {
				return v, []Repair{repair(path, "string_integer", valueKind(value), "integer")}
			}
		}
		if typeAllows(node.Type, "number") {
			if v, ok := parseJSONStringNumber(s); ok {
				return v, []Repair{repair(path, "string_number", valueKind(value), "number")}
			}
		}
		if typeAllows(node.Type, "boolean") {
			if v, ok := parseJSONStringBool(s); ok {
				return v, []Repair{repair(path, "string_boolean", valueKind(value), "boolean")}
			}
		}
	}

	return value, nil
}

func normalizeObject(in map[string]any, node schemaNode, path string, cfg types.ToolParamCompatConfig) (map[string]any, []Repair) {
	if len(node.Properties) == 0 {
		return in, nil
	}
	valueMap := in
	out, repairs := normalizeObjectPropertyKeys(in, node, path)
	if out != nil {
		valueMap = out
	}
	keys := make([]string, 0, len(node.Properties))
	for key := range node.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		propSchema := node.Properties[key]
		current, exists := valueMap[key]
		if !exists {
			continue
		}
		normalized, fieldRepairs := normalizeValue(current, propSchema, path+"."+key, cfg)
		if len(fieldRepairs) == 0 {
			continue
		}
		if out == nil {
			out = cloneStringAnyMap(valueMap)
		}
		out[key] = normalized
		repairs = append(repairs, fieldRepairs...)
	}
	if out == nil {
		return in, nil
	}
	return out, repairs
}

type keyRepairCandidate struct {
	from string
	to   string
	rule string
}

func normalizeObjectPropertyKeys(in map[string]any, node schemaNode, path string) (map[string]any, []Repair) {
	if len(in) == 0 || len(node.Properties) == 0 {
		return nil, nil
	}
	byTarget := make(map[string][]keyRepairCandidate)
	for key := range in {
		if _, ok := node.Properties[key]; ok {
			continue
		}
		if canonical, rule, ok := schemaPropertyKeyAlias(key, node); ok {
			byTarget[canonical] = append(byTarget[canonical], keyRepairCandidate{
				from: key,
				to:   canonical,
				rule: rule,
			})
		}
	}
	if len(byTarget) == 0 {
		return nil, nil
	}
	targets := make([]string, 0, len(byTarget))
	for target := range byTarget {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	var out map[string]any
	var repairs []Repair
	for _, target := range targets {
		if _, exists := in[target]; exists {
			continue
		}
		candidates := byTarget[target]
		if len(candidates) != 1 {
			continue
		}
		candidate := candidates[0]
		if out == nil {
			out = cloneStringAnyMap(in)
		}
		out[target] = out[candidate.from]
		delete(out, candidate.from)
		repairs = append(repairs, Repair{
			Path: propertyPath(path, candidate.from),
			Rule: candidate.rule,
			From: "key:" + candidate.from,
			To:   "key:" + candidate.to,
		})
	}
	if out == nil {
		return nil, nil
	}
	return out, repairs
}

func schemaPropertyKeyAlias(key string, node schemaNode) (string, string, bool) {
	if len(node.Properties) == 0 {
		return "", "", false
	}
	trimmedSpace := strings.TrimSpace(key)
	if trimmedSpace != key {
		if _, ok := node.Properties[trimmedSpace]; ok {
			return trimmedSpace, "property_key_whitespace", true
		}
	}
	quoteTrimmed := trimJSONKeyQuoteArtifacts(trimmedSpace)
	if quoteTrimmed != trimmedSpace {
		if _, ok := node.Properties[quoteTrimmed]; ok {
			return quoteTrimmed, "property_key_quote_artifact", true
		}
	}
	snake := schemaStyleKeyAlias(quoteTrimmed)
	if snake != quoteTrimmed {
		if _, ok := node.Properties[snake]; ok {
			return snake, "property_key_case_style", true
		}
	}
	return "", "", false
}

func trimJSONKeyQuoteArtifacts(s string) string {
	for {
		next := strings.Trim(s, "\"'`\\")
		if next == s {
			return s
		}
		s = strings.TrimSpace(next)
	}
}

func schemaStyleKeyAlias(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	prevUnderscore := false
	var prev rune
	for i, r := range s {
		switch {
		case r == '-' || unicode.IsSpace(r):
			if b.Len() > 0 && !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
			prev = r
			continue
		case r == '_':
			if b.Len() > 0 && !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
			prev = r
			continue
		case unicode.IsUpper(r):
			if i > 0 && !prevUnderscore && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
		default:
			b.WriteRune(unicode.ToLower(r))
			prevUnderscore = false
		}
		prev = r
	}
	return strings.Trim(b.String(), "_")
}

func propertyPath(parent, key string) string {
	if parent == "" {
		parent = "$"
	}
	return parent + "." + key
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func unwrapToolArgumentEnvelope(in map[string]any, node schemaNode) (map[string]any, string, bool) {
	if len(in) == 0 || len(node.Properties) == 0 {
		return nil, "", false
	}
	if hasSchemaPropertyAtEnvelope(in, node) {
		return nil, "", false
	}
	if carrierKey, carrierValue, ok := singleEnvelopeCarrier(in); ok && envelopeHasOnlyKnownKeys(in) {
		return decodeEnvelopeArgumentObject(carrierValue, node, "tool_argument_envelope_"+carrierKey)
	}

	fnRaw, ok := in["function"]
	if !ok || !envelopeHasOnlyKnownKeys(in) {
		return nil, "", false
	}
	fn, ok := fnRaw.(map[string]any)
	if !ok || hasSchemaPropertyAtEnvelope(fn, node) || !envelopeHasOnlyKnownKeys(fn) {
		return nil, "", false
	}
	carrierKey, carrierValue, ok := singleEnvelopeCarrier(fn)
	if !ok {
		return nil, "", false
	}
	return decodeEnvelopeArgumentObject(carrierValue, node, "tool_function_envelope_"+carrierKey)
}

func decodeEnvelopeArgumentObject(value any, node schemaNode, rulePrefix string) (map[string]any, string, bool) {
	switch v := value.(type) {
	case map[string]any:
		if schemaPropertyHitCount(v, node) == 0 {
			return nil, "", false
		}
		return v, rulePrefix + "_object", true
	case string:
		decoded, rule, ok := decodeJSONStringAs(v, "object")
		if !ok {
			return nil, "", false
		}
		m, ok := decoded.(map[string]any)
		if !ok || schemaPropertyHitCount(m, node) == 0 {
			return nil, "", false
		}
		return m, rulePrefix + "_" + rule, true
	default:
		return nil, "", false
	}
}

func hasSchemaPropertyAtEnvelope(in map[string]any, node schemaNode) bool {
	for key := range in {
		if _, ok := node.Properties[key]; ok {
			return true
		}
	}
	return false
}

func schemaPropertyHitCount(in map[string]any, node schemaNode) int {
	hits := 0
	for key := range in {
		if _, ok := node.Properties[key]; ok {
			hits++
		}
	}
	return hits
}

func singleEnvelopeCarrier(in map[string]any) (string, any, bool) {
	var foundKey string
	var foundValue any
	for _, key := range envelopeCarrierKeyOrder {
		value, ok := in[key]
		if !ok {
			continue
		}
		if foundKey != "" {
			return "", nil, false
		}
		foundKey = key
		foundValue = value
	}
	if foundKey == "" {
		return "", nil, false
	}
	return foundKey, foundValue, true
}

func envelopeHasOnlyKnownKeys(in map[string]any) bool {
	for key := range in {
		if isEnvelopeCarrierKey(key) || isEnvelopeMetadataKey(key) {
			continue
		}
		return false
	}
	return true
}

func isEnvelopeCarrierKey(key string) bool {
	switch key {
	case "arguments", "parameters", "params", "input", "args", "tool_input", "toolInput", "action_input", "actionInput":
		return true
	default:
		return false
	}
}

func isEnvelopeMetadataKey(key string) bool {
	switch key {
	case "id", "call_id", "name", "type", "tool", "tool_name", "function":
		return true
	default:
		return false
	}
}

func normalizeArray(in []any, node schemaNode, path string, cfg types.ToolParamCompatConfig) ([]any, []Repair) {
	if len(node.Items) == 0 {
		return in, nil
	}
	var out []any
	var repairs []Repair
	for i, item := range in {
		normalized, itemRepairs := normalizeValue(item, node.Items, fmt.Sprintf("%s[%d]", path, i), cfg)
		if len(itemRepairs) == 0 {
			continue
		}
		if out == nil {
			out = append([]any(nil), in...)
		}
		out[i] = normalized
		repairs = append(repairs, itemRepairs...)
	}
	if out == nil {
		return in, nil
	}
	return out, repairs
}

func parseSchema(raw json.RawMessage) (schemaNode, bool) {
	if len(raw) == 0 {
		return schemaNode{}, false
	}
	var node schemaNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return schemaNode{}, false
	}
	return node, true
}

func typeAllows(raw any, want string) bool {
	switch v := raw.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func schemaExpectsObject(node schemaNode) bool {
	return typeAllows(node.Type, "object") || (node.Type == nil && len(node.Properties) > 0)
}

func schemaExpectsArray(node schemaNode) bool {
	return typeAllows(node.Type, "array") || (node.Type == nil && len(node.Items) > 0)
}

func arrayItemsAllowOnlyString(node schemaNode) bool {
	child, ok := parseSchema(node.Items)
	if !ok {
		return false
	}
	return typeAllows(child.Type, "string") &&
		!typeAllows(child.Type, "object") &&
		!typeAllows(child.Type, "array") &&
		!typeAllows(child.Type, "integer") &&
		!typeAllows(child.Type, "number") &&
		!typeAllows(child.Type, "boolean")
}

func decodeJSONValue(raw json.RawMessage) (any, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, false
	}
	return value, true
}

func decodeJSONStringAs(s string, want string) (any, string, bool) {
	return decodeJSONStringAsDepth(s, want, 0)
}

func decodeJSONStringAsDepth(s string, want string, depth int) (any, string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, "", false
	}
	if strings.HasPrefix(trimmed, `"`) && depth < 3 {
		decoded, ok := decodeJSONValue(json.RawMessage(trimmed))
		if !ok {
			return nil, "", false
		}
		inner, ok := decoded.(string)
		if !ok {
			return nil, "", false
		}
		value, rule, ok := decodeJSONStringAsDepth(inner, want, depth+1)
		if !ok {
			return nil, "", false
		}
		return value, rule + "_nested", true
	}
	switch want {
	case "object":
		if !strings.HasPrefix(trimmed, "{") {
			return nil, "", false
		}
	case "array":
		if !strings.HasPrefix(trimmed, "[") {
			return nil, "", false
		}
	default:
		return nil, "", false
	}
	if decoded, ok := decodeJSONValue(json.RawMessage(trimmed)); ok {
		return decoded, "json_string_" + want, true
	}
	repaired, changed := repairUnescapedQuotesInJSONStringLiterals(trimmed)
	if !changed {
		return nil, "", false
	}
	decoded, ok := decodeJSONValue(json.RawMessage(repaired))
	if !ok {
		return nil, "", false
	}
	return decoded, "json_string_" + want + "_quote_escape", true
}

type jsonStringRole uint8

const (
	jsonStringValue jsonStringRole = iota
	jsonStringKey
)

type jsonContainerExpect uint8

const (
	expectValueOrEnd jsonContainerExpect = iota
	expectKeyOrEnd
	expectColon
	expectCommaOrEnd
)

type jsonContainerState struct {
	kind   byte
	expect jsonContainerExpect
}

// RepairUnescapedQuotesInJSONStringLiterals repairs a narrow local-model
// artifact where JSON-shaped content contains unescaped quote bytes inside
// string values. The returned string is only a candidate; callers must parse it
// before accepting it.
func RepairUnescapedQuotesInJSONStringLiterals(s string) (string, bool) {
	return repairUnescapedQuotesInJSONStringLiterals(s)
}

// NormalizeControlCharsInJSONStrings repairs a deterministic local-model
// artifact where otherwise JSON-shaped content contains raw control bytes
// inside string literals. Strict JSON rejects those bytes, but models often
// produce them in multi-line fields such as Mermaid bodies or markdown prose.
//
// The pass is structural: it walks bytes with a JSON-string state machine and
// rewrites raw controls ONLY while inside a string literal. Braces, brackets,
// commas, and all bytes outside strings pass through unchanged. Callers must
// still parse the returned candidate before accepting it.
func NormalizeControlCharsInJSONStrings(s string) (string, bool) {
	// Fast path: scan for any byte < 0x20 (the entire control-char range JSON
	// forbids inside string literals).
	hasControl := false
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 {
			hasControl = true
			break
		}
	}
	if !hasControl {
		return s, false
	}
	var out strings.Builder
	out.Grow(len(s) + 16)
	inString := false
	escapeCount := 0
	changed := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !inString {
			out.WriteByte(c)
			if c == '"' {
				inString = true
				escapeCount = 0
			}
			continue
		}
		if c == '\\' {
			out.WriteByte(c)
			escapeCount++
			continue
		}
		if c == '"' && escapeCount%2 == 0 {
			out.WriteByte(c)
			inString = false
			escapeCount = 0
			continue
		}
		switch c {
		case '\n':
			out.WriteString(`\n`)
			changed = true
		case '\r':
			out.WriteString(`\r`)
			changed = true
		case '\t':
			out.WriteString(`\t`)
			changed = true
		case '\f':
			out.WriteString(`\f`)
			changed = true
		case '\b':
			out.WriteString(`\b`)
			changed = true
		default:
			if c < 0x20 {
				fmt.Fprintf(&out, `\u%04x`, c)
				changed = true
			} else {
				out.WriteByte(c)
			}
		}
		escapeCount = 0
	}
	if !changed {
		return s, false
	}
	return out.String(), true
}

// repairUnescapedQuotesInJSONStringLiterals handles a common local-model
// artefact inside string-wrapped JSON payloads:
//
//	{"items":"[{\"summary\":\"uses \"foo\" internally\"}]"}
//
// After the outer JSON string is decoded, the inner array/object still looks
// JSON-shaped but its free-text string value contains unescaped quote bytes.
// The repair is deliberately narrow: it only adds escapes to quote bytes that
// cannot legally terminate the current JSON string according to nearby JSON
// syntax and container state. The caller re-parses the repaired payload before
// accepting it, so malformed or ambiguous input remains unchanged.
func repairUnescapedQuotesInJSONStringLiterals(s string) (string, bool) {
	var out strings.Builder
	out.Grow(len(s))

	stack := make([]jsonContainerState, 0, 8)
	inString := false
	escaped := false
	role := jsonStringValue
	changed := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				out.WriteByte(ch)
				continue
			}
			switch ch {
			case '\\':
				escaped = true
				out.WriteByte(ch)
			case '"':
				if jsonQuoteTerminatesString(s, i, role) {
					inString = false
					out.WriteByte(ch)
					if role == jsonStringKey {
						setTopExpect(stack, expectColon)
					} else {
						markJSONStringValueComplete(stack)
					}
					continue
				}
				out.WriteByte('\\')
				out.WriteByte(ch)
				changed = true
			default:
				out.WriteByte(ch)
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
			role = nextJSONStringRole(stack)
			out.WriteByte(ch)
		case '{':
			stack = append(stack, jsonContainerState{kind: '{', expect: expectKeyOrEnd})
			out.WriteByte(ch)
		case '[':
			stack = append(stack, jsonContainerState{kind: '[', expect: expectValueOrEnd})
			out.WriteByte(ch)
		case ':':
			if len(stack) > 0 && stack[len(stack)-1].kind == '{' {
				stack[len(stack)-1].expect = expectValueOrEnd
			}
			out.WriteByte(ch)
		case ',':
			if len(stack) > 0 {
				switch stack[len(stack)-1].kind {
				case '{':
					stack[len(stack)-1].expect = expectKeyOrEnd
				case '[':
					stack[len(stack)-1].expect = expectValueOrEnd
				}
			}
			out.WriteByte(ch)
		case '}':
			if len(stack) > 0 && stack[len(stack)-1].kind == '{' {
				stack = stack[:len(stack)-1]
				markJSONStringValueComplete(stack)
			}
			out.WriteByte(ch)
		case ']':
			if len(stack) > 0 && stack[len(stack)-1].kind == '[' {
				stack = stack[:len(stack)-1]
				markJSONStringValueComplete(stack)
			}
			out.WriteByte(ch)
		default:
			out.WriteByte(ch)
		}
	}
	if !changed {
		return "", false
	}
	return out.String(), true
}

func nextJSONStringRole(stack []jsonContainerState) jsonStringRole {
	if len(stack) == 0 {
		return jsonStringValue
	}
	top := stack[len(stack)-1]
	if top.kind == '{' && top.expect == expectKeyOrEnd {
		return jsonStringKey
	}
	return jsonStringValue
}

func jsonQuoteTerminatesString(s string, quoteIndex int, role jsonStringRole) bool {
	next, ok := nextNonSpaceByte(s, quoteIndex+1)
	if !ok {
		return true
	}
	if role == jsonStringKey {
		return next == ':'
	}
	switch next {
	case ',', '}', ']':
		return true
	default:
		return false
	}
}

func nextNonSpaceByte(s string, start int) (byte, bool) {
	for i := start; i < len(s); i++ {
		switch s[i] {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return s[i], true
		}
	}
	return 0, false
}

func setTopExpect(stack []jsonContainerState, expect jsonContainerExpect) {
	if len(stack) == 0 {
		return
	}
	stack[len(stack)-1].expect = expect
}

func markJSONStringValueComplete(stack []jsonContainerState) {
	if len(stack) == 0 {
		return
	}
	stack[len(stack)-1].expect = expectCommaOrEnd
}

func splitStringArray(s string) ([]any, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return nil, false
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return nil, false
	}
	out := make([]any, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func parseJSONStringInteger(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	for i, r := range s {
		if (r == '+' || r == '-') && i == 0 {
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

func parseJSONStringNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func parseJSONStringBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func valueKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float64, int, int64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func repair(path, rule, from, to string) Repair {
	return Repair{Path: path, Rule: rule, From: from, To: to}
}
