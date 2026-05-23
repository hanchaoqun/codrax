package tool

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func failStrictDecode(name string, now time.Time, err error, hints []MisplacedFieldHint) (types.ToolResult, error) {
	repair := strictDecodeToolRepair(err, hints)
	err = RemapStrictDecodeError(err, hints)
	return failEmitWithRepair(name, now, repair, "invalid params: %v", err)
}

func failStrictDecodeWithError(name string, now time.Time, err error, hints []MisplacedFieldHint) (types.ToolResult, error) {
	repair := strictDecodeToolRepair(err, hints)
	err = RemapStrictDecodeError(err, hints)
	return types.ToolResult{
		ToolName:  name,
		Success:   false,
		Summary:   fmt.Sprintf("invalid params: %v", err),
		Repair:    repair,
		Timestamp: now,
	}, err
}

func strictDecodeToolRepair(err error, hints []MisplacedFieldHint) *types.ToolRepair {
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
