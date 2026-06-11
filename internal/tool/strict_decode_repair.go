package tool

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func failStrictDecode(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, "", "", false)
}

func failStrictDecodeWithError(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, "", "", true)
}

func failStrictDecodeMessage(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, prefix, suffix string) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, prefix, suffix, false)
}

func failStrictDecodeWithErrorMessage(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, prefix, suffix string) (types.ToolResult, error) {
	return strictDecodeFailure(name, now, err, hints, raw, prefix, suffix, true)
}

func strictDecodeFailure(name string, now time.Time, err error, hints []MisplacedFieldHint, raw []byte, prefix, suffix string, returnErr bool) (types.ToolResult, error) {
	repair := strictDecodeToolRepair(err, hints, raw)
	remapped := RemapStrictDecodeErrorWithRaw(err, hints, raw)
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

func strictDecodeToolRepair(err error, hints []MisplacedFieldHint, raw []byte) *types.ToolRepair {
	if err == nil {
		return nil
	}
	if field := extractUnknownFieldName(err); field != "" {
		for _, h := range hints {
			if h.Field != field {
				continue
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
			Hint:   "Remove this unknown field or move its value into a valid schema field without changing the answer facts.",
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
