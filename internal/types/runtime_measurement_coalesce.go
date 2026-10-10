package types

import (
	"encoding/json"
	"reflect"
)

// Coalesce only after all native producers have been collected. A single
// query and a later comparison may publish the same table. Matching public
// bytes alone are insufficient: private completion scope must also agree.
// Conflicting same-key tables remain present so Choices withholds the key.
func coalesceIdenticalRuntimeMeasurementTables(tables []RuntimeMeasurementTable) []RuntimeMeasurementTable {
	seen := make(map[string][]int, len(tables))
	out := make([]RuntimeMeasurementTable, 0, len(tables))
	for _, table := range tables {
		encoded, err := json.Marshal(table)
		if err != nil {
			out = append(out, table)
			continue
		}
		key, duplicate := string(encoded), false
		for _, index := range seen[key] {
			if reflect.DeepEqual(out[index], table) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			seen[key] = append(seen[key], len(out))
			out = append(out, table)
		}
	}
	return out
}
