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

const InvestigationNativeRowReferenceTeaching = "For native facts used by this unit, prefer native_refs with an exact selected observation_id/view and original zero-based row_indices instead of copying values into text or aggregate_facts. Use each table's own source, subject, range, units and notes; equal values do not make different measures interchangeable. Keep independent reasoning in text. A native_ref validates only a row address, never the accompanying explanation, calculation, causal relation or diagram note. Legacy support_refs remain unverified locators."

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

func investigationNativeRowBindings(h *InvestigationNativeFactHandoff, c *RuntimeMeasurementContract) (map[string]string, bool) {
	selected, ok := BindInvestigationNativeFactHandoff(h, c)
	if !ok {
		return nil, false
	}
	tables := map[string]RuntimeMeasurementTable{}
	for _, receipt := range selected {
		tables[receipt.ObservationID+"\x00"+string(receipt.View)] = *receipt.BoundTable
	}
	bindings := map[string]string{}
	for i, unit := range h.InterpretationUnits {
		if !unit.Valid() {
			return nil, false
		}
		seen := map[string]bool{}
		for j, ref := range unit.NativeRefs {
			key := ref.ObservationID + "\x00" + string(ref.View)
			table, found := tables[key]
			source, current := table.NativeSourceTitleBasis()
			if !found || !current || seen[key] {
				return nil, false
			}
			seen[key] = true
			for _, row := range ref.RowIndices {
				if row >= len(table.Rows) {
					return nil, false
				}
			}
			raw, err := json.Marshal([]any{source, table, ref})
			if err != nil {
				return nil, false
			}
			digest := sha256.Sum256(raw)
			bindings[fmt.Sprintf("%d/%d", i, j)] = hex.EncodeToString(digest[:])
		}
	}
	return bindings, true
}

// Seal only after the public completion selector binds against fresh native
// sources. JSON retains addresses, never this private comparison credential.
func SealInvestigationNativeRowReferences(h *InvestigationNativeFactHandoff, c *RuntimeMeasurementContract) bool {
	if h == nil {
		return false
	}
	h.nativeRowBindings = nil
	bindings, ok := investigationNativeRowBindings(h, c)
	if ok {
		h.nativeRowBindings = bindings
	}
	return ok
}

// Rebinding verifies an address and its unchanged source/query/table, not prose
// correctness or prompt publication. The caller also requires current accepted
// completion ownership; a reconstructed handoff cannot acquire that ownership.
func InvestigationNativeRowReferencesCurrent(h *InvestigationNativeFactHandoff, c *RuntimeMeasurementContract) bool {
	if h == nil || len(h.nativeRowBindings) == 0 {
		return false
	}
	current, ok := investigationNativeRowBindings(h, c)
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
