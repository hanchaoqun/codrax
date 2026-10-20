package types

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// Metadata labels consume only a current preparation receipt. They do not
// infer chip/device equivalence, workload comparability or requested windows.
func runtimeMeasurementMetadataLabels(fields []attachment.TraceSourceMetadataField) (string, string, string) {
	var device, workload, parser []string
	devicePresent, workloadPresent := false, false
	for _, field := range fields {
		label := field.Origin + "." + field.Name + "=" + field.State
		if field.State == "observed" || field.State == "invalid" && field.Value != "" {
			label += " " + strconv.Quote(field.Value)
		}
		if field.Detail != "" {
			label += " (" + field.Detail + ")"
		}
		switch field.Kind {
		case "device":
			device = append(device, label)
			devicePresent = devicePresent || field.State != "missing"
		case "workload":
			workload = append(workload, label)
			workloadPresent = workloadPresent || field.State != "missing"
		case "parser":
			parser = append(parser, label)
		}
	}
	d, w := "设备未知", "负载未知"
	if len(device) > 0 {
		if devicePresent {
			d = strings.Join(device, "; ")
		} else {
			d += " (" + strings.Join(device, "; ") + ")"
		}
	}
	if len(workload) > 0 {
		if workloadPresent {
			w = strings.Join(workload, "; ")
		} else {
			w += " (" + strings.Join(workload, "; ") + ")"
		}
	}
	return d, w, strings.Join(parser, "; ")
}

// Single-source queries use the same native table transport as the pair status.
// No metadata means no extra empty table. Source stays empty so this diagnostic
// table cannot stand in for measured data, a user window or investigation work.
func buildRuntimeMeasurementSingleSourceMetadata(input ObservationLedgerInput) []RuntimeMeasurementPublication {
	if input.RequestModel != nil && input.RequestModel.RuntimeArtifactScopeProfile != nil && input.RequestModel.RuntimeArtifactScopeProfile.Comparison != nil {
		return nil
	}
	var out []RuntimeMeasurementPublication
	seen := map[string]bool{}
	for _, group := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range group {
			if !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil {
				continue
			}
			nav := result.TraceQueryWindowReplay.preparedSource
			if nav == nil || nav.material == nil {
				continue
			}
			original := runtimeMeasurementOriginalSource(input, result, ObservationSourceRef{Path: nav.material.QueryPath()})
			if original == "" || seen[original] {
				continue
			}
			fields, err := nav.material.SourceMetadata(context.Background())
			meaningful := false
			for _, field := range fields {
				meaningful = meaningful || field.State != "missing"
			}
			if err != nil || !meaningful {
				continue
			}
			seen[original] = true
			raw, _ := json.Marshal(fields)
			hash := sha256.Sum256(append([]byte(original), raw...))
			id := "trace_source_metadata:" + hex.EncodeToString(hash[:12])
			table := RuntimeMeasurementTable{ObservationID: id, View: RuntimeMeasurementSummary, Label: "采集记录的设备与负载信息", DefaultPresentation: true,
				Columns: []string{"原始采集", "记录位置", "字段类别", "字段", "状态", "原值", "说明"},
				Notes:   []string{"仅展示本次真实准备且仍有效的原件记录；字段不证明设备等同、负载一致、时间同步、量测完整或因果。解析器原文字段不升级为设备或负载身份。"}}
			for _, f := range fields {
				table.Rows = append(table.Rows, []string{original, f.Origin, f.Kind, f.Name, f.State, strconv.Quote(f.Value), f.Detail})
			}
			sealRuntimeRawFactTable(&table)
			out = append(out, RuntimeMeasurementPublication{Version: 1, ObservationID: id, Tables: []RuntimeMeasurementTable{table}})
		}
	}
	return out
}

func runtimeMeasurementMetadataForOriginal(input ObservationLedgerInput, original string) []attachment.TraceSourceMetadataField {
	var materials []*attachment.TraceMaterial
	if input.runtimeMeasurementInputPreparer != nil {
		materials = input.runtimeMeasurementInputPreparer.PreparedMaterials()
	}
	materials = append(materials, input.runtimeMeasurementAttachedMaterial)
	for _, material := range materials {
		if material == nil || !material.MatchesOriginalSource(original) {
			continue
		}
		fields, err := material.SourceMetadata(context.Background())
		if err == nil {
			return fields
		}
	}
	return nil
}
