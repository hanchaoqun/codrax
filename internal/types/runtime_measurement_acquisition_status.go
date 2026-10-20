package types

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// The existing live catalog is the sole source of acquisition state. This
// lookup grants neither read permission nor measurement/evidence authority.
func runtimeMeasurementComparisonCatalog(input ObservationLedgerInput) (*tracecatalog.Catalog, [2]tracecatalog.Artifact, [2]error) {
	var artifacts [2]tracecatalog.Artifact
	var issues [2]error
	if input.runtimeMeasurementPairConsumer == nil || input.RequestModel == nil || input.RequestModel.RuntimeArtifactScopeProfile == nil || !input.RequestModel.RuntimeArtifactScopeProfile.Comparison.Valid() {
		return nil, artifacts, issues
	}
	for _, catalog := range input.runtimeMeasurementPairConsumer.TraceCatalogs() {
		bound := true
		for i, side := range input.RequestModel.RuntimeArtifactScopeProfile.Comparison.Sides() {
			var member bool
			artifacts[i], member, issues[i] = catalog.IndependentArtifactForOriginal(context.Background(), RuntimeArtifactComparisonLocalPath(side.Path, input.RepoRoot))
			bound = bound && member
		}
		if bound {
			return catalog, artifacts, issues
		}
	}
	return nil, artifacts, issues
}

// One table covers pair calls, separate calls and unexecuted declared sides.
// Empty Source intentionally prevents this navigation status from satisfying
// investigation completion or becoming proof of an empty measured population.
func buildRuntimeMeasurementAcquisitionStatus(input ObservationLedgerInput, publications []RuntimeMeasurementPublication) []RuntimeMeasurementPublication {
	catalog, artifacts, issues := runtimeMeasurementComparisonCatalog(input)
	if catalog == nil {
		return buildRuntimeMeasurementSingleSourceMetadata(input)
	}
	scope := input.RequestModel.RuntimeArtifactScopeProfile
	comparison := scope.Comparison
	var windows [2]RuntimeArtifactTimeWindow
	for i := range windows {
		var ok bool
		windows[i], ok = scope.EffectiveComparisonWindow(i)
		if !ok {
			return nil
		}
	}
	raw, _ := json.Marshal(struct {
		Comparison *RuntimeArtifactComparisonScope
		Windows    [2]RuntimeArtifactTimeWindow
	}{comparison, windows})
	hash := sha256.Sum256(append([]byte(catalog.ID()), raw...))
	id := "trace_measurement_acquisition:" + hex.EncodeToString(hash[:12])
	table := RuntimeMeasurementTable{ObservationID: id, View: RuntimeMeasurementSummary, Label: "两份采集的独立取数状态", DefaultPresentation: true,
		Columns: []string{"角色", "状态", "用户来源与范围", "来源/代次", "已保留的原生测量", "设备/工作负载", "查询记录与边界"},
		Notes:   []string{"每份输入独立准备和查询；未指定窗口不补造端点。状态不证明数据为空、可比性、同一时钟、差值或因果。", "状态来自本轮真实准备和查询记录；原生值保留各自单位、分母、窗口、未知与省略。失败历史不覆盖仍有效的成功结果，未执行和失败不能补0。"}}
	states := runtimeMeasurementProducerStates(input)
	queries := catalog.Snapshot().Queries
	for i, side := range comparison.Sides() {
		status, refs, history := runtimeMeasurementAcquisitionSide(artifacts[i], issues[i], queries, publications, states)
		window := "未指定时间窗"
		if windows[i].Valid() {
			window = fmt.Sprintf("[%g,%g) 秒", *windows[i].TimeStart, *windows[i].TimeEnd)
		}
		device, workload, parser := "设备未知", "负载未知", ""
		if issues[i] == nil {
			device, workload, parser = runtimeMeasurementMetadataLabels(runtimeMeasurementMetadataForOriginal(input, RuntimeArtifactComparisonLocalPath(side.Path, input.RepoRoot)))
		}
		metadata := device + " / " + workload
		if parser != "" {
			metadata += " / 解析器原文（不证明设备或负载身份）: " + parser
		}
		table.Rows = append(table.Rows, []string{[]string{"baseline", "current"}[i], status, side.Path + " " + window,
			artifacts[i].Path + " @" + artifacts[i].RevisionID, strings.Join(refs, "; "), metadata, strings.Join(history, "; ")})
	}
	sealRuntimeRawFactTable(&table)
	return []RuntimeMeasurementPublication{{Version: 1, ObservationID: id, Tables: []RuntimeMeasurementTable{table},
		sourcePresentation: &runtimeMeasurementSourcePresentation{}}}
}

func runtimeMeasurementProducerStates(input ObservationLedgerInput) map[string]string {
	states := map[string]string{}
	for _, group := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range group {
			for _, record := range result.Observations {
				if _, ok := DecodeRuntimeMeasurementPublication(record); ok {
					// Raw state is part of the producer-sealed projection. A valid
					// catalog attempt proves execution, not a changed Object field.
					// Keep that execution history but never turn an unsealed state
					// into empty/available or overwrite an intact same-ID result.
					if rawMeasurementPredicate(record.Predicate) && !runtimeRawMeasurementValid(record) {
						if _, exists := states[record.ID]; !exists {
							states[record.ID] = "unavailable"
						}
						continue
					}
					states[record.ID] = record.Object
				}
			}
			if report, ok := result.RuntimeMeasurementPair.reportFor(input.runtimeMeasurementPairConsumer); ok {
				for _, side := range report.Sides {
					for _, state := range side.ProducerStates {
						states[state.ObservationID] = state.State
					}
				}
			}
		}
	}
	return states
}

func runtimeMeasurementAcquisitionSide(artifact tracecatalog.Artifact, sourceErr error, queries []tracecatalog.QueryRecord, publications []RuntimeMeasurementPublication, states map[string]string) (string, []string, []string) {
	status := "not_executed"
	var references, history []string
	for _, issue := range artifact.Issues {
		if issue.Code == "preparation_failed" {
			status = "prep_failed"
		}
		history = append(history, issue.Code+": "+issue.Message)
	}
	counts := map[tracecatalog.Outcome]int{}
	accepted := map[string]bool{}
	available, empty := false, false
	for _, query := range queries {
		if query.ArtifactID != artifact.ID {
			continue
		}
		for _, attempt := range query.Attempts {
			counts[attempt.Outcome]++
			if attempt.Error != nil {
				history = append(history, query.View+" "+string(attempt.Outcome)+": "+attempt.Error.Code+" "+attempt.Error.Message)
			}
			if attempt.Outcome != tracecatalog.OutcomeSuccess && attempt.Outcome != tracecatalog.OutcomeEmpty {
				continue
			}
			for _, publication := range publications {
				if attempt.PayloadRef == "" || publication.Source.PayloadRef != attempt.PayloadRef || accepted[publication.ObservationID] {
					continue
				}
				qualified := false
				for _, table := range publication.Tables {
					qualified = qualified || table.DefaultPresentation
				}
				if !qualified {
					continue // Supplementary/unknown windows do not satisfy this side.
				}
				accepted[publication.ObservationID] = true
				state := states[publication.ObservationID]
				available = available || state != "unavailable" && state != "empty"
				empty = empty || state == "empty" || attempt.Outcome == tracecatalog.OutcomeEmpty
				window := "查询连续窗口未知"
				if start, end, known := TraceObservationContinuousQueryWindow(publication.Source); known {
					window = fmt.Sprintf("[%g,%g) 秒", start, end)
				}
				references = append(references, publication.ObservationID+" "+window)
			}
		}
	}
	for _, outcome := range []tracecatalog.Outcome{tracecatalog.OutcomeSuccess, tracecatalog.OutcomeEmpty, tracecatalog.OutcomeFailure, tracecatalog.OutcomeCanceled, tracecatalog.OutcomeStale} {
		if counts[outcome] > 0 {
			history = append(history, fmt.Sprintf("%s=%d", outcome, counts[outcome]))
		}
	}
	switch {
	case errors.Is(sourceErr, tracecatalog.ErrStale):
		status, references = "stale", nil
	case errors.Is(sourceErr, context.Canceled):
		status, references = "cancelled", nil
	case errors.Is(sourceErr, tracecatalog.ErrUnavailable):
		status = "unavailable"
	case available:
		status = "available"
	case empty:
		status = "empty"
	case counts[tracecatalog.OutcomeSuccess]+counts[tracecatalog.OutcomeEmpty] > 0:
		status = "unavailable"
		history = append(history, "查询已执行，但没有当前范围内可用的原生量测；不是量值为0。")
	case status == "prep_failed":
	case counts[tracecatalog.OutcomeCanceled] > 0:
		status = "cancelled"
	case counts[tracecatalog.OutcomeStale] > 0:
		status = "stale"
	case counts[tracecatalog.OutcomeFailure] > 0:
		status = "query_failed"
	}
	sort.Strings(references)
	return status, references, history
}
