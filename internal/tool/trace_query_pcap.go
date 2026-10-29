package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/pcapinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryPacketInventorySchema(schema string) string {
	teaching, _ := json.Marshal(tracequery.PacketInventoryTeaching + " " + tracequery.PacketFlowsTeaching)
	return strings.Replace(schema, "The deterministic trace view to compute.", "The deterministic trace view to compute. "+string(teaching[1:len(teaching)-1]), 1)
}

func traceQueryPacketSelectionRepair(hint string) types.ToolResult {
	return types.ToolResult{ToolName: "trace_query", Summary: hint, Timestamp: time.Now(), Repair: &types.ToolRepair{Code: "packet_inventory_selection_invalid", Hint: hint, Fields: []string{"view", "source", "path", "limit"}, Metadata: map[string]string{"stage": types.ToolRepairStageTraceSourceSelection, "status": types.ToolRepairStatusActionRecommended}}}
}

func traceQueryPacketInventoryInput(raw json.RawMessage, p traceQueryParams) *types.ToolResult {
	if !tracequery.IsPacketView(p.View) {
		return nil
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil {
		return nil
	}
	var unsupported []string
	for key := range keys {
		if key != "source" && key != "path" && key != "view" && key != "limit" {
			unsupported = append(unsupported, key)
		}
	}
	sort.Strings(unsupported)
	if len(unsupported) > 0 || p.Limit.Int() < 0 || p.Limit.Int() > pcapinput.MaxLimit {
		r := traceQueryPacketSelectionRepair(fmt.Sprintf("%s accepts source/path and limit=1..%d (omitted/0 uses %d) only; unsupported explicit fields: %v. Remove them and retry this same source. PCAP epoch time must not be filtered using Trace seconds.", tracequery.CanonicalViewName(p.View), pcapinput.MaxLimit, pcapinput.DefaultLimit, unsupported))
		return &r
	}
	return nil
}

func traceQuerySourceIsPCAP(path string) bool {
	f, _, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	_, err = io.ReadFull(f, b[:])
	return err == nil && pcapinput.HasMagic(b[:])
}

func (t *TraceQuery) packetInventory(ctx *types.BusContext, p traceQueryParams, path string) (types.ToolResult, error) {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), Timestamp: time.Now()}, nil
	}
	path = filepath.Clean(physical)
	query, err := tracequery.StreamPacketQuery(contextFromBus(ctx), path, tracequery.ViewPacketInventory, p.Limit.Int())
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), nil
		}
		return types.ToolResult{ToolName: t.Name(), Summary: "Packet capture query failed; no complete inventory was published: " + err.Error(), Timestamp: time.Now()}, nil
	}
	result := *query.Payload.PacketInventory
	payload, err := json.Marshal(query.Payload)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), Timestamp: time.Now()}, nil
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "packet-inventory.json", string(payload))
	if result.Status != "complete" {
		return types.ToolResult{ToolName: t.Name(), Summary: fmt.Sprintf("Packet inventory incomplete: %s at byte %d after %d valid records. This is not a zero-packet or complete-capture result. Partial raw record facts: %s", result.Failure.Code, result.Failure.Offset, result.PacketCount, payloadRef), RawRef: payloadRef, Timestamp: time.Now()}, nil
	}
	if err := query.ValidateCurrentSource(); err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), Timestamp: time.Now()}, nil
	}
	scope := fmt.Sprintf("pcap:%x", sha256.Sum256(append([]byte(query.SourceVersionToken()+"\x00"+strconv.Itoa(p.Limit.Int())+"\x00"), payload...)))
	minTime, maxTime := packetTimestamp(result.MinTimestamp), packetTimestamp(result.MaxTimestamp)
	var text strings.Builder
	fmt.Fprintf(&text, "PCAP packet inventory: source=%s; status=complete; records=%d; members=%d; omitted=%d. Epoch seconds min=%s max=%s (%s precision); captured frame bytes=%d; original frame bytes=%d; truncated records=%d. These lengths are not transport payload or physical bandwidth. No scheduler clock alignment, PID or thread identity.\n", path, result.PacketCount, len(result.Members), result.OmittedPackets, minTime, maxTime, result.Header.Resolution, result.CapturedBytes, result.OriginalBytes, result.TruncatedPackets)
	for _, row := range result.Protocols {
		fmt.Fprintf(&text, "Protocol %s: %d records.\n", row.Label, row.Count)
	}
	for _, row := range result.Members {
		fmt.Fprintf(&text, "Record %d @ byte %d: epoch=%s; captured/original=%d/%d bytes; protocol=%s; %s:%s -> %s:%s; direction=%s; decode=%s; decoder_prefix_limited=%t; capture_truncated=%t.\n", row.Ordinal, row.FileOffset, row.Timestamp.Decimal, row.CapturedBytes, row.OriginalBytes, row.Protocol, packetString(row.SourceAddress), packetPort(row.SourcePort), packetString(row.DestinationAddress), packetPort(row.DestinationPort), row.Direction, row.DecodeStatus, row.DecoderPrefixLimited, row.Truncated)
	}
	fmt.Fprintf(&text, "Full native payload: %s. Reuse the native packet_inventory_observation summary/members/distribution selectors for factual presentation; member preview does not replace the complete record inventory.\n", payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), text.String())
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: path, ArtifactKind: "trace", TimeDomain: "pcap_epoch_seconds", ClockAlignment: "unaligned", PayloadRef: payloadRef, RawRef: rawRef, QueryScopeID: scope}
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#packets", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, ClaimKey: "packet_inventory_observation", Predicate: "packet_inventory_observation", Subject: "packet capture records", Object: "complete", Summary: "Complete file record inventory; bounded packet preview; native epoch clock, lengths and explicit protocol decoding status", ObservedAt: now.Format(time.RFC3339Nano), Confidence: 1}
	publication := packetInventoryPublication(r, result)
	data, err := json.Marshal(publication)
	if err != nil || len(data) > 64<<10 {
		return types.ToolResult{ToolName: t.Name(), Summary: "Packet publication exceeds the bounded native receipt; no truncated publication was accepted", RawRef: payloadRef, Timestamp: now}, nil
	}
	r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
	sealed, ok := types.PublishRuntimeRawMeasurement(r)
	if !ok {
		return types.ToolResult{ToolName: t.Name(), Summary: "Packet raw-fact publication validation failed", RawRef: payloadRef, Timestamp: now}, nil
	}
	if err := query.ValidateCurrentSource(); err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), RawRef: payloadRef, Timestamp: now}, nil
	}
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: preview, RawRef: rawRef, Timestamp: now, Observations: []types.ObservationRecord{sealed}, TraceQuerySourceRead: types.TraceQueryPhysicalSourceReadCandidate(path)}, nil
}

func packetTimestamp(t *pcapinput.Timestamp) string {
	if t == nil {
		return "未知（没有包记录）"
	}
	return t.Decimal
}
func packetPort(p *uint16) string {
	if p == nil {
		return "未知"
	}
	return strconv.Itoa(int(*p))
}
func packetString(s string) string {
	if s == "" {
		return "未知"
	}
	return s
}
func packetUint32(p *uint32) string {
	if p == nil {
		return "未知"
	}
	return strconv.FormatUint(uint64(*p), 10)
}

func packetInventoryPublication(r types.ObservationRecord, p tracewire.PacketInventoryResult) types.RuntimeMeasurementPublication {
	notes := []string{"来源：" + p.SourcePath, fmt.Sprintf("完整文件扫描得到%d条包记录；明细预览%d条，省略%d条。完整记录扫描不证明抓包未丢包或业务网络活动完整。", p.PacketCount, len(p.Members), p.OmittedPackets), "时间为PCAP原生epoch秒，精度=" + p.Header.Resolution + "；不映射到Trace调度时间窗。时间范围是记录观察范围，不是用户查询窗或响应耗时。", "captured为记录中保留的帧字节，original为记录报告的原始帧字节；不相加，不等于传输层payload、应用字节、硬件带宽或线程等待。", "协议按已观察头字段解码；未知/截断保留，分片不猜端口；方向仅Linux cooked记录类型证明，Ethernet方向未知。每条ordinal/byte_offset独立，重复字节不去重。接口索引不是进程或线程身份。"}
	notes = append(notes, fmt.Sprintf("每条记录最多解码前%d字节；解码前缀受限与原始抓包截断分别记录，不把未读取的payload解释为零或完整协议内容。", pcapinput.PacketPrefixBytes))
	table := func(view types.RuntimeMeasurementView, label string, columns []string) types.RuntimeMeasurementTable {
		return types.RuntimeMeasurementTable{ObservationID: r.ID, View: view, Label: label, Columns: columns, Notes: append([]string(nil), notes...)}
	}
	summary := table(types.RuntimeMeasurementSummary, "抓包完整记录清单", []string{"包记录数", "文件字节数", "captured帧字节", "original帧字节", "截断包数", "最早epoch秒", "最晚epoch秒", "首条epoch秒", "末条epoch秒", "倒序记录数", "链路类型", "原始链路字段", "snaplen字节", "字节序"})
	summary.DefaultPresentation = true
	summary.Rows = [][]string{{fmt.Sprint(p.PacketCount), fmt.Sprint(p.SourceBytes), fmt.Sprint(p.CapturedBytes), fmt.Sprint(p.OriginalBytes), fmt.Sprint(p.TruncatedPackets), packetTimestamp(p.MinTimestamp), packetTimestamp(p.MaxTimestamp), packetTimestamp(p.FirstTimestamp), packetTimestamp(p.LastTimestamp), fmt.Sprint(p.OutOfOrderPackets), fmt.Sprint(p.Header.LinkType), fmt.Sprintf("0x%08x", p.Header.LinkTypeWord), fmt.Sprint(p.Header.Snaplen), p.Header.ByteOrder}}
	members := table(types.RuntimeMeasurementMembers, "抓包记录预览（非完整成员全集）", []string{"序号", "文件字节偏移", "epoch秒", "captured帧字节", "original帧字节", "是否截断", "协议", "源地址", "源端口", "目的地址", "目的端口", "记录方向", "接口索引", "cooked包类型", "解码状态", "是否分片", "解码前缀受限"})
	for _, row := range p.Members {
		members.Rows = append(members.Rows, []string{fmt.Sprint(row.Ordinal), fmt.Sprint(row.FileOffset), row.Timestamp.Decimal, fmt.Sprint(row.CapturedBytes), fmt.Sprint(row.OriginalBytes), fmt.Sprint(row.Truncated), row.Protocol, packetString(row.SourceAddress), packetPort(row.SourcePort), packetString(row.DestinationAddress), packetPort(row.DestinationPort), row.Direction, packetUint32(row.InterfaceIndex), packetPort(row.PacketType), row.DecodeStatus, fmt.Sprint(row.Fragmented), fmt.Sprint(row.DecoderPrefixLimited)})
	}
	distribution := table(types.RuntimeMeasurementDistribution, "抓包完整记录分类", []string{"分类维度", "已观察类别", "包记录数"})
	for _, group := range []struct {
		label  string
		counts []pcapinput.Count
	}{{"协议", p.Protocols}, {"解码状态", p.DecodeStatuses}, {"记录方向", p.Directions}} {
		for _, row := range group.counts {
			distribution.Rows = append(distribution.Rows, []string{group.label, row.Label, fmt.Sprint(row.Count)})
		}
	}
	distribution.Notes = append(distribution.Notes, "三个分类维度分别覆盖同一完整记录集合；不同维度的计数不能相加。协议分布不是连接数或并发深度。")
	return types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{summary, members, distribution}}
}
