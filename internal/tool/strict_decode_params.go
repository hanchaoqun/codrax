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
		res, retErr := failStrictDecodeWithError(name, time.Now(), err, hints, normalized)
		return normalized, &res, retErr
	}
	return normalized, nil, nil
}
