package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/pcapinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (t *TraceQuery) packetFlows(ctx *types.BusContext, p traceQueryParams, path string) (types.ToolResult, error) {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), Timestamp: time.Now()}, nil
	}
	path = filepath.Clean(physical)
	result, err := pcapinput.StreamFlows(contextFromBus(ctx), path, p.Limit.Int())
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), nil
		}
		return types.ToolResult{ToolName: t.Name(), Summary: "Packet endpoint query failed; no complete census was published: " + err.Error(), Timestamp: time.Now()}, nil
	}
	payload, err := json.Marshal(tracequery.Result{View: tracequery.ViewPacketFlows, SourcePath: path, PacketFlows: &result.PacketFlowsResult})
	if err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), Timestamp: time.Now()}, nil
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "packet-flows.json", string(payload))
	if result.Status != "complete" || result.FlowCount == nil {
		reason := "incomplete group coverage"
		if result.Failure != nil {
			reason = fmt.Sprintf("%s at byte %d", result.Failure.Code, result.Failure.Offset)
		}
		return types.ToolResult{ToolName: t.Name(), Summary: fmt.Sprintf("Packet endpoint query partial: %s; scanned records=%d, tracked groups=%d, group budget exceeded=%t. Neither a complete census nor global Top N is published. Partial original facts: %s", reason, result.PacketCount, result.TrackedFlowCount, result.GroupBudgetExceeded, payloadRef), RawRef: payloadRef, Timestamp: time.Now()}, nil
	}
	if err := result.ValidateCurrentSource(); err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), RawRef: payloadRef, Timestamp: time.Now()}, nil
	}
	scope := fmt.Sprintf("pcap-flows:%x", sha256.Sum256(append([]byte(result.SourceVersionToken()+"\x00"+strconv.Itoa(p.Limit.Int())+"\x00"), payload...)))
	var text strings.Builder
	fmt.Fprintf(&text, "PCAP endpoint groups: source=%s; status=complete; records=%d; grouped records=%d; ungrouped records=%d; endpoint groups=%d; displayed groups=%d; omitted groups=%d. Ordering is captured frame bytes descending. Epoch observation bounds=%s..%s (%s precision). Groups are not sessions, connections or requests; frame lengths are not payload.\n", path, result.PacketCount, result.GroupedPackets, result.UngroupedPackets, *result.FlowCount, len(result.Members), result.OmittedTrackedFlows, packetTimestamp(result.MinTimestamp), packetTimestamp(result.MaxTimestamp), result.Header.Resolution)
	for _, flow := range result.Members {
		fmt.Fprintf(&text, "%s %s A=%s B=%s: records=%d captured/original=%d/%d bytes; A-to-B=%d B-to-A=%d same-endpoint=%d.\n", flow.Protocol, packetFlowDomain(flow), packetFlowEndpoint(flow.EndpointA), packetFlowEndpoint(flow.EndpointB), flow.Stats.Packets, flow.Stats.CapturedBytes, flow.Stats.OriginalBytes, flow.AToB.Packets, flow.BToA.Packets, flow.SameEndpoint.Packets)
	}
	fmt.Fprintf(&text, "Full native payload: %s. For exact directional counts, frame bytes and epoch bounds reuse packet_flows_observation summary/members/distribution selectors; no copied rows are needed.\n", payloadRef)
	preview, rawRef := StoreBlob(ctx, t.Name(), text.String())
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: path, ArtifactKind: "trace", TimeDomain: "pcap_epoch_seconds", ClockAlignment: "unaligned", PayloadRef: payloadRef, RawRef: rawRef, QueryScopeID: scope}
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#endpoint-groups", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, ClaimKey: "packet_flows_observation", Predicate: "packet_flows_observation", Subject: "packet endpoint groups", Object: "complete", Summary: "Complete packet endpoint grouping; bounded group preview; native epoch clock and directional frame lengths, not sessions or causal relationships", ObservedAt: now.Format(time.RFC3339Nano), Confidence: 1}
	publication := packetFlowsPublication(r, result.PacketFlowsResult)
	data, err := json.Marshal(publication)
	if err != nil || len(data) > 64<<10 {
		return types.ToolResult{ToolName: t.Name(), Summary: "Packet endpoint publication exceeds the bounded native receipt; no truncated publication was accepted", RawRef: payloadRef, Timestamp: now}, nil
	}
	r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
	sealed, ok := types.PublishRuntimeRawMeasurement(r)
	if !ok {
		return types.ToolResult{ToolName: t.Name(), Summary: "Packet endpoint raw-fact publication validation failed", RawRef: payloadRef, Timestamp: now}, nil
	}
	if err := result.ValidateCurrentSource(); err != nil {
		return types.ToolResult{ToolName: t.Name(), Summary: err.Error(), RawRef: payloadRef, Timestamp: now}, nil
	}
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: preview, RawRef: rawRef, Timestamp: now, Observations: []types.ObservationRecord{sealed}, TraceQuerySourceRead: types.TraceQueryPhysicalSourceReadCandidate(path)}, nil
}

func packetFlowEndpoint(e tracewire.PacketFlowEndpoint) string {
	return net.JoinHostPort(e.Address, strconv.Itoa(int(e.Port)))
}

func packetFlowDomain(f tracewire.PacketFlow) string {
	var tags []string
	for _, v := range f.VLANs {
		tags = append(tags, fmt.Sprintf("0x%04x:%d", v.TPID, v.ID))
	}
	vlan := "无标签"
	if len(tags) > 0 {
		vlan = strings.Join(tags, "/")
	}
	return fmt.Sprintf("link=%d; interface=%s; VLAN=%s; IPv%d", f.LinkType, packetUint32(f.InterfaceIndex), vlan, f.IPVersion)
}

func packetFlowsPublication(r types.ObservationRecord, p tracewire.PacketFlowsResult) types.RuntimeMeasurementPublication {
	notes := []string{"来源：" + p.SourcePath, fmt.Sprintf("完整扫描%d条记录，%d条可归组、%d条未归组；共%d个双向端点组合，展示%d组、省略%d组。每组先对完整记录聚合，再按captured帧字节降序展示。", p.PacketCount, p.GroupedPackets, p.UngroupedPackets, p.TrackedFlowCount, len(p.Members), p.OmittedTrackedFlows), "A/B按端点规范顺序命名，不表示客户端/服务端；双向组合不证明连接、会话或请求响应。来源代次、链路、接口已知性、VLAN栈、IP版本、协议及端点均独立分组。", "两种字节数分别是captured帧字节和original报告帧字节，不相加，不等于传输层载荷、业务字节或硬件带宽。方向零包表示本文件未观察到该方向记录；缺时间保持未知，不推断网络未发生。", "epoch秒保原始精度=" + p.Header.Resolution + "；最早/最晚是相应记录观察范围，不是查询窗、连接持续时间或响应时长。不与Trace时钟自动对齐，不授进程、线程、等待或因果权限。", "A→B/B→A方向不同于Linux cooked incoming/outgoing。分片、未知端口或解码不足按原因未归组；合法端口0保留。完整文件扫描不证明采集未丢包；抓包截断和解码前缀受限分别计数。"}
	table := func(view types.RuntimeMeasurementView, label string, columns []string) types.RuntimeMeasurementTable {
		return types.RuntimeMeasurementTable{ObservationID: r.ID, View: view, Label: label, Columns: columns, Notes: append([]string(nil), notes...)}
	}
	summary := table(types.RuntimeMeasurementSummary, "抓包端点组合统计", []string{"全部记录", "已归组记录", "未归组记录", "端点组合数", "展示组合数", "省略组合数", "文件字节", "captured帧字节", "original帧字节", "截断记录", "最早epoch秒", "最晚epoch秒"})
	summary.DefaultPresentation = true
	summary.Rows = [][]string{{fmt.Sprint(p.PacketCount), fmt.Sprint(p.GroupedPackets), fmt.Sprint(p.UngroupedPackets), fmt.Sprint(p.TrackedFlowCount), fmt.Sprint(len(p.Members)), fmt.Sprint(p.OmittedTrackedFlows), fmt.Sprint(p.SourceBytes), fmt.Sprint(p.CapturedBytes), fmt.Sprint(p.OriginalBytes), fmt.Sprint(p.TruncatedPackets), packetTimestamp(p.MinTimestamp), packetTimestamp(p.MaxTimestamp)}}
	members := table(types.RuntimeMeasurementMembers, "端点组合与逐方向统计（有限组预览）", []string{"展示组", "观察域", "协议", "端点A", "端点B", "方向", "包数", "captured帧字节", "original帧字节", "最早epoch秒", "最晚epoch秒", "截断包数", "解码前缀受限包数"})
	for i, flow := range p.Members {
		directions := []struct {
			label string
			stats tracewire.PacketFlowStats
		}{{"A→B", flow.AToB}, {"B→A", flow.BToA}}
		if flow.EndpointA == flow.EndpointB {
			directions = []struct {
				label string
				stats tracewire.PacketFlowStats
			}{{"同端点", flow.SameEndpoint}}
		}
		for _, direction := range directions {
			s := direction.stats
			members.Rows = append(members.Rows, []string{fmt.Sprint(i + 1), packetFlowDomain(flow), flow.Protocol, packetFlowEndpoint(flow.EndpointA), packetFlowEndpoint(flow.EndpointB), direction.label, fmt.Sprint(s.Packets), fmt.Sprint(s.CapturedBytes), fmt.Sprint(s.OriginalBytes), packetTimestamp(s.MinTimestamp), packetTimestamp(s.MaxTimestamp), fmt.Sprint(s.TruncatedPackets), fmt.Sprint(s.DecoderPrefixLimitedPackets)})
		}
	}
	distribution := table(types.RuntimeMeasurementDistribution, "未归组记录原因", []string{"原因", "记录数", "captured帧字节", "original帧字节", "最早epoch秒", "最晚epoch秒"})
	for _, u := range p.Ungrouped {
		distribution.Rows = append(distribution.Rows, []string{u.Reason, fmt.Sprint(u.Stats.Packets), fmt.Sprint(u.Stats.CapturedBytes), fmt.Sprint(u.Stats.OriginalBytes), packetTimestamp(u.Stats.MinTimestamp), packetTimestamp(u.Stats.MaxTimestamp)})
	}
	return types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: []types.RuntimeMeasurementTable{summary, members, distribution}}
}
