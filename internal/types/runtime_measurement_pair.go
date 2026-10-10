package types

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// RuntimeMeasurementPairRef holds immutable producer data and run-local source
// receipts. Go value copies are safe; JSON/history cannot recreate authority.
type RuntimeMeasurementPairRef struct {
	report     string
	owner      *MutableState
	generation *traceSourceReadGeneration
	replay     [2]TraceQueryWindowReplayRef
	material   [2]*attachment.TraceMaterial
	// Preserve producer-private coverage through the report's JSON copy-out.
	// These decoded values are never exposed or reconstructed from JSON.
	publications [2][]RuntimeMeasurementPublication
}

// NativeMeasurementPairSide is an internal producer input, not a model schema.
type NativeMeasurementPairSide struct {
	Request  json.RawMessage
	Result   ToolResult
	Material *attachment.TraceMaterial
}

type RuntimeMeasurementPairSideReport struct {
	Role           string                                `json:"role"`
	Request        json.RawMessage                       `json:"request,omitempty"`
	Status         string                                `json:"status"`
	Detail         string                                `json:"detail,omitempty"`
	Generation     string                                `json:"source_generation,omitempty"`
	Device         string                                `json:"device"`
	Workload       string                                `json:"workload"`
	Publications   []RuntimeMeasurementPublication       `json:"publications,omitempty"`
	ProducerStates []RuntimeMeasurementPairProducerState `json:"producer_states,omitempty"`
}

type RuntimeMeasurementPairProducerState struct {
	ObservationID string `json:"observation_id"`
	Predicate     string `json:"predicate"`
	State         string `json:"state"`
}

type RuntimeMeasurementPairReport struct {
	ID       string                              `json:"id"`
	Sides    [2]RuntimeMeasurementPairSideReport `json:"sides"`
	Boundary string                              `json:"boundary"`
}

// NewNativeMeasurementPair retains only publications independently accepted by
// the existing native decoder. Values, unknowns and per-view rulers are never
// read back from formatted text or synthesized from the other side.
func NewNativeMeasurementPair(owner *MutableState, sides [2]NativeMeasurementPairSide) RuntimeMeasurementPairRef {
	ref := RuntimeMeasurementPairRef{owner: owner}
	if owner != nil {
		owner.mu.RLock()
		ref.generation = owner.traceSourceReadGeneration
		owner.mu.RUnlock()
	}
	report := RuntimeMeasurementPairReport{Boundary: "两侧独立取数；来源、窗口、单位、分母和覆盖分别保留。设备未知、负载未知不由名称推断；并列展示不证明可比性、同一时钟、差值或因果。"}
	for i, input := range sides {
		side := &report.Sides[i]
		side.Role = []string{"baseline", "current"}[i]
		side.Device, side.Workload = "设备未知", "负载未知"
		side.Request = append(json.RawMessage(nil), input.Request...)
		side.Status = "not_requested"
		if len(input.Request) == 0 {
			continue
		}
		side.Status, side.Detail = "failed", input.Result.Summary
		if input.Result.TraceViewCancellation != nil {
			side.Status = "cancelled"
			continue
		}
		if !input.Result.Success || input.Result.ToolName != "trace_query" || input.Result.ReusedFromRunMemo {
			continue
		}
		side.Status, side.Detail = "unavailable", "查询完成，但没有可绑定的原生测量表；不是值为0。"
		for _, record := range input.Result.Observations {
			if publication, ok := DecodeRuntimeMeasurementPublication(record); ok {
				side.Publications = append(side.Publications, publication)
				side.ProducerStates = append(side.ProducerStates, RuntimeMeasurementPairProducerState{record.ID, record.Predicate, record.Object})
			}
		}
		if len(side.Publications) == 0 {
			continue
		}
		ref.publications[i] = side.Publications
		side.Status, side.Detail = "available", "原生表保留各自单位、分母、区间、未知和省略；没有记录不等于量值为0。"
		allUnavailable := true
		for _, state := range side.ProducerStates {
			if state.State != "unavailable" {
				allUnavailable = false
			}
		}
		if allUnavailable {
			side.Status, side.Detail = "unavailable", "查询完成，但生产者明确量测不可用；保留其空表和缺测说明，不补0。"
		}
		ref.replay[i], ref.material[i] = input.Result.TraceQueryWindowReplay, input.Material
		if ref.replay[i].source.generation == nil {
			ref.replay[i] = input.Result.TraceStatistics.replay
		}
		side.Generation = ref.replay[i].source.identity.CacheToken()
		if input.Material != nil {
			if identity, err := filegeneration.FromPath(input.Material.SourcePath()); err == nil {
				side.Generation = identity.CacheToken()
			}
		}
	}
	raw, _ := json.Marshal(report)
	digest := sha256.Sum256(raw)
	report.ID = "trace_measurement_pair:" + hex.EncodeToString(digest[:12])
	raw, _ = json.Marshal(report)
	ref.report = string(raw)
	return ref
}

// Report is a fresh copy for audit output. State revalidation never mutates the
// original report; one replaced source invalidates only that side's tables.
func (r RuntimeMeasurementPairRef) Report() (RuntimeMeasurementPairReport, bool) {
	return r.reportFor(r.owner)
}

func (r RuntimeMeasurementPairRef) reportFor(consumer *MutableState) (RuntimeMeasurementPairReport, bool) {
	var report RuntimeMeasurementPairReport
	if r.report == "" || json.Unmarshal([]byte(r.report), &report) != nil {
		return report, false
	}
	for i := range report.Sides {
		side := &report.Sides[i]
		if len(side.Publications) == 0 {
			continue
		}
		current := consumer != nil && r.generation != nil
		if current {
			consumer.mu.RLock()
			current = consumer.traceSourceReadGeneration == r.generation
			consumer.mu.RUnlock()
		}
		if current && r.material[i] == nil {
			_, _, current = consumer.ResolveTraceQueryWindowReplay(r.replay[i])
		}
		if current && r.material[i] != nil {
			current = r.material[i].Validate(context.Background(), r.material[i].Preview()) == nil
			for _, publication := range side.Publications {
				current = current && publication.Source.Path == r.material[i].QueryPath()
			}
		}
		if !current {
			side.Status, side.Detail, side.Publications = "stale", "来源或本轮代次已变化，旧测量不再作为当前值；另一侧独立保留。", nil
			continue
		}
		// report and publications were minted together. Copy each full table
		// only after source/epoch validation; no returned slice aliases the
		// retained producer value, including its private coverage scope.
		for j, publication := range r.publications[i] {
			for k, table := range publication.Tables {
				side.Publications[j].Tables[k] = table.Clone()
			}
		}
	}
	return report, true
}

func buildRuntimeMeasurementPairPublications(input ObservationLedgerInput) []RuntimeMeasurementPublication {
	var out []RuntimeMeasurementPublication
	for _, group := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range group {
			if result.ToolName != "trace_query" {
				continue
			}
			report, ok := result.RuntimeMeasurementPair.reportFor(input.runtimeMeasurementPairConsumer)
			if !ok {
				continue
			}
			status := RuntimeMeasurementTable{ObservationID: report.ID, View: RuntimeMeasurementSummary, Label: "两份采集的独立取数状态", DefaultPresentation: true,
				Columns: []string{"角色", "状态", "本侧查询请求", "来源/代次", "原生测量引用与实际窗口", "设备/工作负载", "单位、分母与覆盖"}, Notes: []string{report.Boundary, "角色和本侧窗口来自查询选择；不另授予原始用户问题中的逐源窗口绑定。失败/取消/未请求不能补0。完整原生值在各侧独立表中。"}}
			for _, side := range report.Sides {
				var references, sources []string
				for _, publication := range side.Publications {
					start, end, known := TraceObservationContinuousQueryWindow(publication.Source)
					var requested *RuntimeArtifactScopeProfile
					if input.RequestModel != nil {
						requested = input.RequestModel.RuntimeArtifactScopeProfile
					}
					if requested.HasExplicitTimeWindows() && known && !requested.ContainsExplicitTimeWindow(start, end) {
						side.Status, side.Detail = "outside_requested_window", "实际查询不在已接受的用户窗口中；保留取数状态，不作为该窗口测量。"
						continue
					}
					ruler := "连续窗口未知"
					if known {
						ruler = fmt.Sprintf("[%g,%g) 秒", start, end)
					}
					references = append(references, publication.ObservationID+" "+ruler)
					sources = append(sources, publication.Source.Path+" @"+side.Generation)
					origin := ObservationRecord{Origin: AnswerEvidenceOriginRuntimeArtifact, SourceRef: publication.Source}
					if !runtimeMeasurementCoverageSourceAllowed(origin, input) {
						for i := range publication.Tables {
							publication.Tables[i].coverageScope = nil
						}
					}
					if requested.HasExplicitTimeWindows() && !known {
						for i, table := range publication.Tables {
							publication.Tables[i] = runtimeMeasurementUnverifiedWindowTable(table)
						}
					}
					out = append(out, publication)
				}
				status.Rows = append(status.Rows, []string{side.Role, side.Status, string(side.Request), strings.Join(sources, "; "), strings.Join(references, "; "), side.Device + " / " + side.Workload, side.Detail})
			}
			out = append(out, RuntimeMeasurementPublication{Version: 1, ObservationID: report.ID, Tables: []RuntimeMeasurementTable{status}})
		}
	}
	return out
}
