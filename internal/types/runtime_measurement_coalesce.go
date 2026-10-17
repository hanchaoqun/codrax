package types

import (
	"encoding/json"
	"reflect"
)

// A single query and a later comparison may publish the same complete
// receipt. Coalesce only whole publications, including private completion
// scope. The absence of a current-source display witness (for example on a
// memo copy) is not a conflicting fact: an independently qualified, identical
// publication retains its own display witness. No other authority is merged.
// A conflict in any view withholds the entire observation; equal sibling
// views cannot accidentally choose one version of its source.
func coalesceRuntimeMeasurementPublications(publications []RuntimeMeasurementPublication) []RuntimeMeasurementTable {
	first := make(map[string]int, len(publications))
	encoded := make(map[string]string, len(publications))
	conflicting := make(map[string]bool)
	displayWitnesses := make(map[string][]bool, len(publications))
	factIdentities := make(map[string][]string, len(publications))
	for i, publication := range publications {
		key := publication.ObservationID
		raw, err := json.Marshal(publication)
		if err != nil || key == "" {
			conflicting[key] = true
			continue
		}
		if index, exists := first[key]; exists {
			if encoded[key] != string(raw) || !runtimeMeasurementPublicationFactsEqual(publications[index], publication) {
				conflicting[key] = true
			} else {
				for j, table := range publication.Tables {
					displayWitnesses[key][j] = displayWitnesses[key][j] || table.nativeTraceDisplayPopulation
					if prior := factIdentities[key][j]; prior != "" && table.nativeFactIdentity != "" && prior != table.nativeFactIdentity {
						conflicting[key] = true
					} else if table.nativeFactIdentity != "" {
						factIdentities[key][j] = table.nativeFactIdentity
					}
				}
			}
			continue
		}
		first[key], encoded[key] = i, string(raw)
		displayWitnesses[key] = make([]bool, len(publication.Tables))
		factIdentities[key] = make([]string, len(publication.Tables))
		for j, table := range publication.Tables {
			displayWitnesses[key][j] = table.nativeTraceDisplayPopulation
			factIdentities[key][j] = table.nativeFactIdentity
		}
	}
	var out []RuntimeMeasurementTable
	for i, publication := range publications {
		if !conflicting[publication.ObservationID] && first[publication.ObservationID] == i {
			for j, table := range publication.Tables {
				retained := table.Clone()
				retained.nativeTraceDisplayPopulation = displayWitnesses[publication.ObservationID][j]
				retained.nativeFactIdentity = factIdentities[publication.ObservationID][j]
				out = append(out, retained)
			}
		}
	}
	return out
}

func runtimeMeasurementPublicationFactsEqual(a, b RuntimeMeasurementPublication) bool {
	// These are already decoded and source-qualified publications. Ignore only
	// the new positive display witness; full Source, every public value, old
	// private coverage scope and native-log qualification must still agree.
	a.Tables = append([]RuntimeMeasurementTable(nil), a.Tables...)
	b.Tables = append([]RuntimeMeasurementTable(nil), b.Tables...)
	a.answerDisplayIdentity, b.answerDisplayIdentity = "", ""
	for i := range a.Tables {
		a.Tables[i].nativeTraceDisplayPopulation = false
		a.Tables[i].nativeFactIdentity = ""
	}
	for i := range b.Tables {
		b.Tables[i].nativeTraceDisplayPopulation = false
		b.Tables[i].nativeFactIdentity = ""
	}
	return reflect.DeepEqual(a, b)
}
