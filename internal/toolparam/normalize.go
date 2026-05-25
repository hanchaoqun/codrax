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
	Enum       []json.RawMessage          `json:"enum"`
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
	var syntaxRepairs []Repair
	if !ok {
		if repaired, changed := RemoveTrailingCommasBeforeJSONClosers(string(raw)); changed {
			if decoded, decodedOK := decodeJSONValue(json.RawMessage(repaired)); decodedOK {
				value = decoded
				ok = true
				raw = json.RawMessage(repaired)
				syntaxRepairs = append(syntaxRepairs, repair("$", "json_trailing_comma", "invalid_json", "json"))
			}
		}
	}
	if !ok {
		return raw, Report{}
	}
	normalized, repairs := normalizeValue(value, schema, "$", cfg)
	repairs = append(syntaxRepairs, repairs...)
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
		if m, ok := value.(map[string]any); ok && !typeAllows(node.Type, "object") && arrayItemsExpectObject(node) {
			repairs := []Repair{repair(path, "singleton_object_array", valueKind(value), "array")}
			value = []any{m}
			out, nestedRepairs := normalizeArray(value.([]any), node, path, cfg)
			return out, append(repairs, nestedRepairs...)
		}
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
			if arrayItemsAllowOnlyString(node) {
				if arr, ok := singletonStringArray(s); ok {
					return arr, []Repair{repair(path, "singleton_string_array", valueKind(value), "array")}
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

	if s, ok := value.(string); ok && schemaAllowsStringEnumRepair(node) {
		if v, rule, ok := decodeJSONStringEnumString(s, node); ok {
			return v, []Repair{repair(path, rule, valueKind(value), "string_enum")}
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

// SchemaPropertyKeyAlias returns a canonical schema property name for a
// malformed-but-unambiguous model-emitted key. The match is deliberately
// structural: whitespace/quote artifacts, casing/separator style drift, and a
// final id/ids singular/plural drift are accepted only when exactly one schema
// property can own the key. The exact key is not considered an alias.
func SchemaPropertyKeyAlias(key string, propertyNames []string) (string, string, bool) {
	return schemaPropertyKeyAliasFromNames(key, schemaPropertyNameSet(propertyNames))
}

func schemaPropertyKeyAlias(key string, node schemaNode) (string, string, bool) {
	return schemaPropertyKeyAliasFromNames(key, schemaNodePropertyNameSet(node))
}

func schemaNodePropertyNameSet(node schemaNode) map[string]struct{} {
	if len(node.Properties) == 0 {
		return nil
	}
	names := make(map[string]struct{}, len(node.Properties))
	for name := range node.Properties {
		if name = strings.TrimSpace(name); name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func schemaPropertyNameSet(propertyNames []string) map[string]struct{} {
	if len(propertyNames) == 0 {
		return nil
	}
	names := make(map[string]struct{}, len(propertyNames))
	for _, name := range propertyNames {
		if name = strings.TrimSpace(name); name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func schemaPropertyKeyAliasFromNames(key string, names map[string]struct{}) (string, string, bool) {
	if len(names) == 0 {
		return "", "", false
	}
	trimmedSpace := strings.TrimSpace(key)
	if trimmedSpace != key {
		if _, ok := names[trimmedSpace]; ok {
			return trimmedSpace, "property_key_whitespace", true
		}
	}
	quoteTrimmed := trimJSONKeyQuoteArtifacts(trimmedSpace)
	if quoteTrimmed != trimmedSpace {
		if _, ok := names[quoteTrimmed]; ok {
			return quoteTrimmed, "property_key_quote_artifact", true
		}
	}
	if canonical, ok := compositeSchemaPropertyKeyAlias(quoteTrimmed, names); ok {
		return canonical, "property_key_composite_fragment", true
	}
	if canonical, ok := schemaStringSuffixPropertyAlias(quoteTrimmed, names); ok {
		return canonical, "property_key_string_suffix", true
	}
	snake := schemaStyleKeyAlias(quoteTrimmed)
	if snake != quoteTrimmed {
		if _, ok := names[snake]; ok {
			return snake, "property_key_case_style", true
		}
		if canonical, ok := schemaStringSuffixPropertyAlias(snake, names); ok {
			return canonical, "property_key_string_suffix", true
		}
	}
	if canonical, ok := uniqueSchemaPropertyFingerprintAlias(quoteTrimmed, names, false); ok {
		return canonical, "property_key_fingerprint", true
	}
	if canonical, ok := uniqueSchemaPropertyFingerprintAlias(quoteTrimmed, names, true); ok {
		return canonical, "property_key_id_plural", true
	}
	return "", "", false
}

func schemaStringSuffixPropertyAlias(key string, names map[string]struct{}) (string, bool) {
	const suffix = "_str"
	if !strings.HasSuffix(key, suffix) || len(key) <= len(suffix) {
		return "", false
	}
	base := strings.TrimSpace(strings.TrimSuffix(key, suffix))
	if base == "" {
		return "", false
	}
	if _, ok := names[base]; ok {
		return base, true
	}
	return "", false
}

func compositeSchemaPropertyKeyAlias(key string, names map[string]struct{}) (string, bool) {
	if key == "" || len(names) == 0 || !strings.ContainsAny(key, ",\"'`\\") {
		return "", false
	}
	fragments := strings.FieldsFunc(key, func(r rune) bool {
		switch r {
		case ',', '"', '\'', '`', '\\':
			return true
		default:
			return false
		}
	})
	matched := ""
	for _, fragment := range fragments {
		fragment = trimJSONKeyQuoteArtifacts(strings.TrimSpace(fragment))
		if fragment == "" {
			continue
		}
		candidates := []string{fragment}
		if styled := schemaStyleKeyAlias(fragment); styled != fragment {
			candidates = append(candidates, styled)
		}
		for _, candidate := range candidates {
			if _, ok := names[candidate]; !ok {
				continue
			}
			if matched != "" && matched != candidate {
				return "", false
			}
			matched = candidate
		}
		if canonical, ok := uniqueSchemaPropertyFingerprintAlias(fragment, names, false); ok {
			if matched != "" && matched != canonical {
				return "", false
			}
			matched = canonical
		}
	}
	if matched == "" || matched == key {
		return "", false
	}
	return matched, true
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

func uniqueSchemaPropertyFingerprintAlias(key string, names map[string]struct{}, allowIDPlural bool) (string, bool) {
	keyFingerprint := schemaPropertyFingerprint(key)
	if keyFingerprint == "" {
		return "", false
	}
	keyFingerprints := map[string]struct{}{keyFingerprint: {}}
	if allowIDPlural {
		for _, variant := range schemaIDPluralFingerprintVariants(keyFingerprint) {
			keyFingerprints[variant] = struct{}{}
		}
	}
	var matched string
	for name := range names {
		nameFingerprint := schemaPropertyFingerprint(name)
		if nameFingerprint == "" {
			continue
		}
		if _, ok := keyFingerprints[nameFingerprint]; !ok {
			continue
		}
		if matched != "" && matched != name {
			return "", false
		}
		matched = name
	}
	if matched == "" || matched == key {
		return "", false
	}
	return matched, true
}

func schemaPropertyFingerprint(s string) string {
	s = trimJSONKeyQuoteArtifacts(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func schemaIDPluralFingerprintVariants(fingerprint string) []string {
	switch {
	case strings.HasSuffix(fingerprint, "ids") && len(fingerprint) > 3:
		return []string{strings.TrimSuffix(fingerprint, "s")}
	case strings.HasSuffix(fingerprint, "id") && len(fingerprint) > 2:
		return []string{fingerprint + "s"}
	default:
		return nil
	}
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

func arrayItemsExpectObject(node schemaNode) bool {
	child, ok := parseSchema(node.Items)
	if !ok {
		return false
	}
	return schemaExpectsObject(child)
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

func schemaAllowsStringEnumRepair(node schemaNode) bool {
	if len(node.Enum) == 0 {
		return false
	}
	return typeAllows(node.Type, "string") || node.Type == nil
}

func decodeJSONStringEnumString(s string, node schemaNode) (string, string, bool) {
	enumValues := schemaStringEnumValues(node)
	if len(enumValues) == 0 {
		return "", "", false
	}
	if _, ok := enumValues[s]; ok {
		return "", "", false
	}
	current := strings.TrimSpace(s)
	for depth := 0; depth < 3; depth++ {
		if !strings.HasPrefix(current, `"`) {
			return "", "", false
		}
		decoded, ok := decodeJSONValue(json.RawMessage(current))
		if !ok {
			return "", "", false
		}
		inner, ok := decoded.(string)
		if !ok {
			return "", "", false
		}
		if _, ok := enumValues[inner]; ok {
			rule := "json_string_enum"
			if depth > 0 {
				rule = "json_string_enum_nested"
			}
			return inner, rule, true
		}
		current = strings.TrimSpace(inner)
	}
	return "", "", false
}

func schemaStringEnumValues(node schemaNode) map[string]struct{} {
	if len(node.Enum) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(node.Enum))
	for _, raw := range node.Enum {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		out[s] = struct{}{}
	}
	return out
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

// JSONRepairCandidates returns deterministic syntax-repair candidates for
// JSON-shaped local-model output. The original string is always first. Every
// later candidate is only a candidate: callers must parse it successfully
// before accepting it.
func JSONRepairCandidates(s string) []string {
	type jsonRepairPass struct {
		fn func(string) (string, bool)
	}
	passes := []jsonRepairPass{
		{fn: RemoveTrailingCommasBeforeJSONClosers},
		{fn: NormalizeControlCharsInJSONStrings},
		{fn: TrimTransportCloseTagsAfterJSONValue},
		{fn: RepairMissingTrailingJSONClosers},
		{fn: RepairUnescapedQuotesInJSONStringLiterals},
	}
	const maxCandidates = 32
	candidates := make([]string, 0, 8)
	seen := map[string]bool{}
	add := func(candidate string) bool {
		if candidate == "" || seen[candidate] || len(candidates) >= maxCandidates {
			return false
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
		return true
	}
	add(s)
	for i := 0; i < len(candidates) && len(candidates) < maxCandidates; i++ {
		base := candidates[i]
		for _, pass := range passes {
			repaired, changed := pass.fn(base)
			if !changed {
				continue
			}
			add(repaired)
		}
	}
	return candidates
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
	if repaired, changed := RemoveTrailingCommasBeforeJSONClosers(trimmed); changed {
		if decoded, ok := decodeJSONValue(json.RawMessage(repaired)); ok {
			return decoded, "json_string_" + want + "_trailing_comma", true
		}
	}
	if repaired, changed := NormalizeControlCharsInJSONStrings(trimmed); changed {
		if decoded, ok := decodeJSONValue(json.RawMessage(repaired)); ok {
			return decoded, "json_string_" + want + "_control_chars", true
		}
	}
	if repaired, changed := RepairMissingTrailingJSONClosers(trimmed); changed {
		if decoded, ok := decodeJSONValue(json.RawMessage(repaired)); ok {
			return decoded, "json_string_" + want + "_missing_closer", true
		}
	}
	if repaired, changed := RepairUnescapedQuotesInJSONStringLiterals(trimmed); changed {
		if decoded, ok := decodeJSONValue(json.RawMessage(repaired)); ok {
			return decoded, "json_string_" + want + "_quote_escape", true
		}
	}
	for _, candidate := range JSONRepairCandidates(trimmed) {
		if candidate == trimmed {
			continue
		}
		decoded, ok := decodeJSONValue(json.RawMessage(candidate))
		if !ok {
			continue
		}
		return decoded, "json_string_" + want + "_syntax", true
	}
	return nil, "", false
}

// TrimTransportCloseTagsAfterJSONValue repairs a deterministic tool-call
// transport artifact where a complete JSON object/array is followed by a
// wrapper close tag inside a string-wrapped schema field, for example
// `[{...}]</parameter>`. The JSON prefix must already be complete and the
// suffix must contain only known wrapper close tags, so this never guesses
// missing payload content.
func TrimTransportCloseTagsAfterJSONValue(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return s, false
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return s, false
	}
	offset := dec.InputOffset()
	if offset <= 0 || offset >= int64(len(trimmed)) {
		return s, false
	}
	suffix := strings.TrimSpace(trimmed[offset:])
	if !isTransportCloseTagSuffix(suffix) {
		return s, false
	}
	prefix := strings.TrimSpace(trimmed[:offset])
	if prefix == "" {
		return s, false
	}
	return prefix, true
}

func isTransportCloseTagSuffix(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for s != "" {
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, "</") {
			return false
		}
		end := strings.IndexByte(s, '>')
		if end < len("</x") {
			return false
		}
		name := strings.TrimSpace(s[len("</"):end])
		if !isKnownTransportCloseTag(name) {
			return false
		}
		s = strings.TrimSpace(s[end+1:])
	}
	return true
}

func isKnownTransportCloseTag(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "parameter", "parameters", "argument", "arguments", "tool_call", "tool-call", "function_call", "function-call":
		return true
	default:
		return false
	}
}

// RemoveTrailingCommasBeforeJSONClosers repairs a deterministic local-model
// JSON syntax artifact:
//
//	{"blocks":[{"kind":"diagram","diagram":{"body":"flowchart TD",},},]}
//
// Strict JSON rejects commas immediately before `}` / `]`. The pass is purely
// structural: it walks the payload with a JSON-string state machine and removes
// only commas whose next non-space byte is a closing object/array delimiter.
// Commas inside string literals are untouched. Callers must parse the returned
// candidate before accepting it.
func RemoveTrailingCommasBeforeJSONClosers(s string) (string, bool) {
	if !strings.Contains(s, ",") {
		return s, false
	}
	var out strings.Builder
	out.Grow(len(s))
	inString := false
	escaped := false
	changed := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			out.WriteByte(ch)
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
			out.WriteByte(ch)
		case ',':
			j := i + 1
			for j < len(s) && isJSONWhitespaceByte(s[j]) {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				changed = true
				continue
			}
			out.WriteByte(ch)
		default:
			out.WriteByte(ch)
		}
	}
	if !changed {
		return s, false
	}
	return out.String(), true
}

// RepairMissingTrailingJSONClosers repairs a deterministic local-model JSON
// syntax artifact where the model emits a JSON-shaped value but omits one or
// more closing delimiters, or closes a parent container before closing a child:
//
//	[{"kind":"diagram","diagram":{"body":"flowchart TD"}]
//
// The pass is structural. It walks the payload with a JSON-string state
// machine, inserts only missing `}` / `]` delimiters needed to restore nesting,
// and refuses unterminated strings or obviously incomplete tails such as `,`,
// `:`, `{`, or `[`. Callers must parse the returned candidate before accepting
// it.
func RepairMissingTrailingJSONClosers(content string) (string, bool) {
	s := strings.TrimSpace(content)
	if s == "" || (s[0] != '{' && s[0] != '[') {
		return "", false
	}
	last := s[len(s)-1]
	if last == ',' || last == ':' || last == '{' || last == '[' {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	stack := make([]byte, 0, 8)
	inString := false
	escaped := false
	changed := false
	insertedClosers := 0
	const maxInsertedClosers = 16
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			b.WriteByte(ch)
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
			b.WriteByte(ch)
		case '{':
			stack = append(stack, '}')
			b.WriteByte(ch)
		case '[':
			stack = append(stack, ']')
			b.WriteByte(ch)
		case '}', ']':
			next, inserted, ok := closeJSONDelimiterRepairStack(stack, ch, &b)
			if !ok {
				return "", false
			}
			if inserted > 0 {
				changed = true
				insertedClosers += inserted
				if insertedClosers > maxInsertedClosers {
					return "", false
				}
			}
			stack = next
			b.WriteByte(ch)
		default:
			b.WriteByte(ch)
		}
	}
	if inString || len(stack) == 0 && !changed {
		return "", false
	}
	if len(stack) > maxInsertedClosers-insertedClosers {
		return "", false
	}
	for i := len(stack) - 1; i >= 0; i-- {
		b.WriteByte(stack[i])
	}
	return b.String(), true
}

func closeJSONDelimiterRepairStack(stack []byte, close byte, b *strings.Builder) ([]byte, int, bool) {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] != close {
			continue
		}
		inserted := 0
		for j := len(stack) - 1; j > i; j-- {
			b.WriteByte(stack[j])
			inserted++
		}
		return stack[:i], inserted, true
	}
	return stack, 0, false
}

func isJSONWhitespaceByte(ch byte) bool {
	switch ch {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
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

func singletonStringArray(s string) ([]any, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return nil, false
	}
	for _, r := range s {
		switch r {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return nil, false
		}
	}
	return []any{s}, true
}

func parseJSONStringInteger(s string) (int64, bool) {
	s = trimJSONStringNumericArtifactPrefix(s)
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
	s = trimJSONStringNumericArtifactPrefix(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func trimJSONStringNumericArtifactPrefix(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return ""
	}
	switch s[0] {
	case ':', '=':
		candidate := strings.TrimSpace(s[1:])
		if candidate == "" {
			return s
		}
		switch candidate[0] {
		case '+', '-', '.', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return candidate
		}
	}
	return s
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
