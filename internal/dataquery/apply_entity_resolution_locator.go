package dataquery

import (
	"fmt"
	"strconv"
	"strings"
)

// entityResolutionItemID preserves the source-record index carried by derived
// artifacts. record.Index is only the ordinal inside the current artifact; a
// filter can compact it and make a later apply_entity_resolutions join attach a
// mapping to the wrong surviving row when the declared base key is
// _source_index. The typed source index is the stable identity used by that
// contract. Invalid/non-positive values fall back to the current ordinal.
func entityResolutionItemID(rel string, record actionRecord, field string) string {
	index := record.Index
	for _, candidate := range []string{"_source_index", "source_index", "row_index"} {
		value := strings.TrimSpace(recordField(record.Fields, candidate))
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			index = parsed
			break
		}
	}
	if index <= 0 {
		index = record.Line
	}
	return fmt.Sprintf("%s#%d:%s", rel, index, field)
}
