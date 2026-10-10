package types

import (
	"encoding/json"
	"reflect"
)

// A single query and a later comparison may publish the same complete
// receipt. Coalesce only whole publications, including private completion
// scope. A conflict in any view withholds the entire observation; equal
// sibling views cannot accidentally choose one version of its source.
func coalesceRuntimeMeasurementPublications(publications []RuntimeMeasurementPublication) []RuntimeMeasurementTable {
	first := make(map[string]int, len(publications))
	encoded := make(map[string]string, len(publications))
	conflicting := make(map[string]bool)
	for i, publication := range publications {
		key := publication.ObservationID
		raw, err := json.Marshal(publication)
		if err != nil || key == "" {
			conflicting[key] = true
			continue
		}
		if index, exists := first[key]; exists {
			if encoded[key] != string(raw) || !reflect.DeepEqual(publications[index], publication) {
				conflicting[key] = true
			}
			continue
		}
		first[key], encoded[key] = i, string(raw)
	}
	var out []RuntimeMeasurementTable
	for i, publication := range publications {
		if !conflicting[publication.ObservationID] && first[publication.ObservationID] == i {
			out = append(out, publication.Tables...)
		}
	}
	return out
}
