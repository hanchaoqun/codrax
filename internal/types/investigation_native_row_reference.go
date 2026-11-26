package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	MaxInvestigationNativeRowReferences = 8
	MaxInvestigationNativeRowIndices    = 32
)

const InvestigationNativeRowReferenceTeaching = "For native facts used by this unit, prefer native_refs with an exact selected observation_id/view and original zero-based row_indices instead of copying values into text or aggregate_facts. Cite raw observations without an offered table (such as wakeup edges) by their exact IDs in support_refs. Use each table's own source, subject, range, units and notes; equal values do not make different measures interchangeable. Keep independent reasoning in text. A native_ref validates only a row address, never the accompanying explanation, calculation, causal relation or diagram note. Legacy support_refs remain unverified locators."

// A row address has no model-authored measurement, source alias or scope label.
// It resolves only inside one exact current producer table and its query.
type InvestigationNativeRowReference struct {
	ObservationID string                 `json:"observation_id"`
	View          RuntimeMeasurementView `json:"view"`
	RowIndices    []int                  `json:"row_indices"`
}

func (r InvestigationNativeRowReference) valid() bool {
	if strings.TrimSpace(r.ObservationID) == "" || !r.View.IsValid() || len(r.RowIndices) == 0 || len(r.RowIndices) > MaxInvestigationNativeRowIndices {
		return false
	}
	seen := map[int]bool{}
	for _, i := range r.RowIndices {
		if i < 0 || seen[i] {
			return false
		}
		seen[i] = true
	}
	return true
}

type InvalidInvestigationNativeRow struct {
	Unit int `json:"unit"`
	Ref  int `json:"ref"`
}

func investigationNativeRowBindings(h *InvestigationNativeFactHandoff, c *RuntimeMeasurementContract) (map[string]string, []InvalidInvestigationNativeRow, bool) {
	selected, ok := BindInvestigationNativeFactHandoff(h, c)
	if !ok {
		return nil, nil, false
	}
	tables := map[string]RuntimeMeasurementTable{}
	for _, receipt := range selected {
		tables[receipt.ObservationID+"\x00"+string(receipt.View)] = *receipt.BoundTable
	}
	bindings := map[string]string{}
	var invalid []InvalidInvestigationNativeRow
	for i, unit := range h.InterpretationUnits {
		if !unit.Valid() {
			return nil, nil, false
		}
		seen := map[string]bool{}
		for j, ref := range unit.NativeRefs {
			key := ref.ObservationID + "\x00" + string(ref.View)
			table, found := tables[key]
			source, current := table.NativeSourceTitleBasis()
			valid := found && current && !seen[key]
			for _, row := range ref.RowIndices {
				if row >= len(table.Rows) {
					valid = false
					break
				}
			}
			if !valid {
				invalid = append(invalid, InvalidInvestigationNativeRow{Unit: i, Ref: j})
				continue
			}
			seen[key] = true
			raw, err := json.Marshal([]any{source, table, ref})
			if err != nil {
				return nil, nil, false
			}
			digest := sha256.Sum256(raw)
			bindings[fmt.Sprintf("%d/%d", i, j)] = hex.EncodeToString(digest[:])
		}
	}
	return bindings, invalid, len(invalid) == 0
}

// Seal only after the public completion selector binds against fresh native
// sources. JSON retains addresses, never this private comparison credential.
func SealInvestigationNativeRowReferences(h *InvestigationNativeFactHandoff, c *RuntimeMeasurementContract) (bool, []InvalidInvestigationNativeRow) {
	if h == nil {
		return false, nil
	}
	h.nativeRowBindings = nil
	bindings, invalid, ok := investigationNativeRowBindings(h, c)
	if ok {
		h.nativeRowBindings = bindings
	}
	return ok, invalid
}

// Rebinding verifies an address and its unchanged source/query/table, not prose
// correctness or prompt publication. The caller also requires current accepted
// completion ownership; a reconstructed handoff cannot acquire that ownership.
func InvestigationNativeRowReferencesCurrent(h *InvestigationNativeFactHandoff, c *RuntimeMeasurementContract) bool {
	if h == nil || len(h.nativeRowBindings) == 0 {
		return false
	}
	current, _, ok := investigationNativeRowBindings(h, c)
	if !ok || len(current) != len(h.nativeRowBindings) {
		return false
	}
	for key, digest := range current {
		if h.nativeRowBindings[key] != digest {
			return false
		}
	}
	return true
}
