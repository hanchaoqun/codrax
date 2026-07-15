package tool

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func decodeStrictToolParams(name string, raw json.RawMessage, schema json.RawMessage, dst any, hints []MisplacedFieldHint) (json.RawMessage, *types.ToolResult, error) {
	normalized := applyStructuredPayloadCompat(name, raw, schema)
	dec := json.NewDecoder(strings.NewReader(string(normalized)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		// LT-HYG decoder-remap hint (§29.75 立案, 2026-07-14): the schema is
		// in hand on this entry — fabricated-field rejections teach the real
		// top-level parameter list (reflected, never hand-copied).
		res, retErr := failStrictDecodeWithErrorSchema(name, time.Now(), err, hints, normalized, schema)
		return normalized, &res, retErr
	}
	return normalized, nil, nil
}

func decodeStrictNormalizedToolParams(name string, normalized json.RawMessage, dst any, hints []MisplacedFieldHint) (json.RawMessage, *types.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(normalized)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		res, retErr := failStrictDecodeWithError(name, time.Now(), err, hints, normalized)
		return normalized, &res, retErr
	}
	return normalized, nil, nil
}
