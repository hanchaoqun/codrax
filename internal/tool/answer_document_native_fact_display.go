package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Refresh only authenticated system blocks. Model-authored tables/prose are
// never rewritten or scanned, and an exact model selector retains ownership.
func materializeNativeFactDisplay(doc *types.AnswerDocumentV2, ctx *types.BusContext, view *types.AnswerSemanticView) {
	if doc == nil || ctx == nil || view == nil {
		return
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	if rm == nil && ctx.Mutable != nil {
		rm = ctx.Mutable.RequestModel()
	}
	retained := make([]types.AnswerBlock, 0, len(doc.Blocks))
	used := map[string]bool{}
	selected := map[string][]types.RuntimeMeasurementTable{}
	selectedSurfaces := map[string][]types.RuntimeMeasurementTable{}
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind == types.AnswerSystemGeneratedNativeFacts {
			continue
		}
		retained = append(retained, block)
		used[block.ID] = true
		if block.RuntimeMeasurement != nil && runtimeMeasurementBindingValid(block, view) {
			receipt := *block.RuntimeMeasurement
			if types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) {
				exact := receipt.ObservationID + "\x00" + string(receipt.View)
				selectedSurfaces[exact] = append(selectedSurfaces[exact], *receipt.BoundTable)
				if block.Kind != types.BlockTable {
					continue
				}
				// Group counts and other surfaces cannot replace the complete
				// member table. Match an ungrouped projection to its exact native
				// population below; a subset/field selection is not full coverage.
				if receipt.EventSelection != nil && len(receipt.EventSelection.GroupBy) > 0 {
					continue
				}
				canonical := receipt
				canonical.EventSelection = nil
				key := types.RuntimeMeasurementDisplayKey(canonical, view.RuntimeMeasurementContract)
				selected[key] = append(selected[key], *receipt.BoundTable)
			}
		}
	}
	doc.Blocks = retained
	var planned []types.AnswerRuntimeMeasurementReceipt
	unavailable := 0
	if ctx.Mutable != nil && contextFromBus(ctx).Err() == nil {
		planned, unavailable = types.ResolveInvestigationNativeAnswerDisplay(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), rm, view.RuntimeMeasurementContract)
	}
	plannedKeys := map[string]bool{}
	for _, receipt := range planned {
		plannedKeys[receipt.ObservationID+"\x00"+string(receipt.View)] = true
	}
	const capNoticeEN = "Some native fact tables were omitted because the document reached its block limit."
	const capNoticeZH = "报告达到区块上限，部分原生事实表未展示。"
	doc.Caveats = slices.DeleteFunc(doc.Caveats, func(caveat string) bool {
		return caveat == capNoticeEN || caveat == capNoticeZH
	})
	// Explicit handoff views take the available slots before automatic views.
	// These refreshed tables must not make a model document fail the block cap.
	truncated := false
	for _, receipt := range append(planned, types.RuntimeNativeFactDisplaySelections(rm, view.RuntimeMeasurementContract)...) {
		key := types.RuntimeMeasurementDisplayKey(receipt, view.RuntimeMeasurementContract)
		if !types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) {
			continue
		}
		covered := false
		exact := receipt.ObservationID + "\x00" + string(receipt.View)
		if plannedKeys[exact] {
			for _, table := range selectedSurfaces[exact] {
				if reflect.DeepEqual(table, *receipt.BoundTable) {
					covered = true
					selected[key] = append(selected[key], table)
					break
				}
			}
		}
		for _, table := range selected[key] {
			if reflect.DeepEqual(table.Columns, receipt.BoundTable.Columns) && reflect.DeepEqual(table.Rows, receipt.BoundTable.Rows) &&
				(!plannedKeys[exact] || runtimeMeasurementDisplayTablesEqual(table, *receipt.BoundTable)) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		if len(doc.Blocks) >= maxBlocksPerDoc {
			truncated = true
			break
		}
		digest := sha256.Sum256([]byte(receipt.ObservationID + "\x00" + string(receipt.View)))
		base := fmt.Sprintf("native-facts-%x", digest[:8])
		id := base
		for n := 1; used[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		used[id] = true
		selected[key] = append(selected[key], *receipt.BoundTable)
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: id, Kind: types.BlockTable, RuntimeMeasurement: &receipt,
			SystemGeneratedKind: types.AnswerSystemGeneratedNativeFacts})
	}
	if unavailable > 0 {
		text := fmt.Sprintf("已选择的原生事实视图中有 %d 项当前无法重新核对来源或查询范围；未用旧摘要或预览值补填。", unavailable)
		if len(doc.Blocks) < maxBlocksPerDoc {
			id := "native-display-unavailable"
			for n := 1; used[id]; n++ {
				id = fmt.Sprintf("native-display-unavailable-%d", n)
			}
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: id, Kind: types.BlockSection, Text: text,
				SystemGeneratedKind: types.AnswerSystemGeneratedNativeFacts})
		} else if !slices.Contains(doc.Caveats, text) {
			doc.Caveats = append(doc.Caveats, text)
		}
	}
	if truncated {
		text := capNoticeEN
		if strings.HasPrefix(strings.ToLower(requestedAnswerDocumentLanguage(ctx)), "zh") {
			text = capNoticeZH
		}
		doc.Caveats = append(doc.Caveats, text)
	}
}

// Called only within the same current producer-qualified display key. Keep
// every public table field exact; remove just the independent query selector.
// Private source/fact credentials were already checked by receipt binding and
// key qualification, and are never transferred between the two selectors.
func runtimeMeasurementDisplayTablesEqual(a, b types.RuntimeMeasurementTable) bool {
	a.ObservationID, b.ObservationID = "", ""
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}
