package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func failStrictDecode(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, nil, "", "", false)
}

func failStrictDecodeWithError(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, nil, "", "", true)
}

// failStrictDecodeWithErrorSchema is failStrictDecodeWithError plus the
// tool's own JSON schema (LT-HYG decoder-remap hint, §29.75 立案,
// 2026-07-14): an unknown-field rejection that matches no misplaced-field
// hint appends the schema's top-level parameter list to the LLM-facing
// error text, so a model that INVENTED a parameter (witness: `ignore_case`
// on trace_query — a grep field) retries against the real parameter set
// instead of guessing again. The list is reflected from the schema bytes,
// never hand-copied.
func failStrictDecodeWithErrorSchema(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, schema json.RawMessage) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, schema, "", "", true)
}

func failStrictDecodeMessage(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, prefix, suffix string) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, nil, prefix, suffix, false)
}

func failStrictDecodeWithErrorMessage(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, prefix, suffix string) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, nil, prefix, suffix, true)
}

func strictDecodeFailure(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, schema json.RawMessage, prefix, suffix string, returnErr bool) (types.ToolResult, error) {
	repair := attachToolJSONSurfaceMetadata(name, strictDecodeToolRepair(err, hints, raw))
	remapped := RemapStrictDecodeErrorWithRaw(err, hints, raw)
	// LT-HYG decoder-remap hint (§29.75 立案, 2026-07-14): fabricated-field
	// guidance. Only the unknown-field class with NO APPLYING hint gets the
	// list — an applying hint already names the correct paths/name, and the
	// other decode classes (string-carrier, element-shape) are not naming
	// problems. Schema-less callers (nil schema) are byte-identical. The
	// same top-level-gated predicate (strictDecodeHintFor, R6-2) decides
	// every hint consumer, so a nested near-miss that gets NO rename
	// teaching does get the parameter list.
	if field := extractUnknownFieldName(err); field != "" && strictDecodeHintFor(hints, field, raw) == nil {
		if known := strictDecodeSchemaTopLevelFields(schema); len(known) > 0 {
			remapped = fmt.Errorf("%w; this tool accepts only these parameters: %s", remapped, strings.Join(known, ", "))
		}
	}
	res := types.ToolResult{
		ToolName:  name,
		Success:   false,
		Summary:   fmt.Sprintf("%sinvalid params: %v%s", prefix, remapped, suffix),
		Repair:    repair,
		Timestamp: now,
	}
	if returnErr {
		return res, remapped
	}
	return res, nil
}

// strictDecodeSchemaTopLevelFields reflects the top-level "properties" key
// names out of a JSON-schema document in DECLARATION order (a token walk, not
// a map round-trip, so the teaching order the schema author chose — view and
// path first on trace_query — survives). Any parse irregularity returns nil:
// the list is an additive hint, never a gate.
func strictDecodeSchemaTopLevelFields(schema json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(schema)))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil
		}
		if key != "properties" {
			// Skip this key's whole value.
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil
			}
			continue
		}
		open, err := dec.Token()
		if err != nil || open != json.Delim('{') {
			return nil
		}
		var fields []string
		for dec.More() {
			nameTok, err := dec.Token()
			if err != nil {
				return nil
			}
			name, ok := nameTok.(string)
			if !ok {
				return nil
			}
			fields = append(fields, name)
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil
			}
		}
		return fields
	}
	return nil
}

func strictDecodeToolRepair(err error, hints []MisplacedFieldHint, raw []byte) *types.ToolRepair {
	if err == nil {
		return nil
	}
	if field := extractUnknownFieldName(err); field != "" {
		if h := strictDecodeHintFor(hints, field, raw); h != nil {
			// Wrong-NAME form (EVALFIX-2A; top-level gated per R6-2 by
			// strictDecodeHintFor): the relocate wording below is the
			// reverse instruction for a misspelled key — fork to a
			// rename repair.
			if h.CanonicalName != "" {
				return &types.ToolRepair{
					Code:   "tool_param_misnamed_field",
					Fields: []string{h.CanonicalName},
					Hint:   "Rename the key to the listed field name and keep the value unchanged.",
					Metadata: map[string]string{
						"field":          field,
						"canonical_name": h.CanonicalName,
					},
				}
			}
			return &types.ToolRepair{
				Code:   "tool_param_misplaced_field",
				Fields: append([]string(nil), h.CorrectPaths...),
				Hint:   "Relocate the value to one of the listed schema paths. Do not rename, delete, or paraphrase the answer content.",
				Metadata: map[string]string{
					"field":              field,
					"invalid_containers": strings.Join(h.ContainerNames, ","),
				},
			}
		}
		return &types.ToolRepair{
			Code:   "tool_param_unknown_field",
			Fields: []string{field},
			// Fix-first ordering (EMITBURN-1 件4, §29.173): relocating the
			// value is the taught path; dropping the field name is the last
			// resort and never licenses deleting the answer content it held.
			Hint: "Move this field's value into a valid schema field; remove the unknown field name only as a last resort, without changing or dropping the answer facts.",
			Metadata: map[string]string{
				"field": field,
			},
		}
	}
	if field := extractCannotUnmarshalStringField(err); field != "" {
		if rawFieldValueKind(raw, field) == '[' {
			return &types.ToolRepair{
				Code:   "tool_param_array_element_shape",
				Fields: []string{field},
				Hint:   "Keep this field as a native JSON array, but re-emit every entry as an object carrying the schema's per-entry fields — plain-string entries are not accepted.",
				Metadata: map[string]string{
					"field": field,
				},
			}
		}
		_, kind := extractCannotUnmarshalStringTarget(err)
		innerState := rawJSONStringCarrierInnerState(raw, field, kind)
		if innerState == jsonStringCarrierInnerMalformed || innerState == jsonStringCarrierInnerWrongShape {
			return &types.ToolRepair{
				Code:   "tool_param_json_string_carrier_unrecoverable",
				Fields: []string{field},
				Hint:   "The field is a string whose inner structured JSON is malformed or has the wrong shape, so no lossless repair was applied. Re-emit it as the schema's native JSON array/object, preserve every intended entry, and do not omit the field to make decoding pass.",
				Metadata: map[string]string{
					"field":       field,
					"inner_state": string(innerState),
				},
			}
		}
		return &types.ToolRepair{
			Code:   "tool_param_json_string_carrier",
			Fields: []string{field},
			Hint:   "Emit this field as a native JSON array/object value, not as a JSON-encoded string. Preserve the same rows and prose inside the native structure.",
			Metadata: map[string]string{
				"field": field,
			},
		}
	}
	return nil
}
