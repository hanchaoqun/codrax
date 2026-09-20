package tracediag

import (
	"fmt"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Measured groups or groups with explicit endpoint metadata enter this lane.
// The engine Summary already
// duplicates the numeric distribution, so this owner renders its typed fields
// once instead of stripping or interpreting summary prose. Legacy nil groups
// without endpoint metadata continue through the unchanged generic renderer. Identity always precedes
// values so a line cap cannot leave an unbound percentile behind.
func renderStorageLatencyDistributionGroup(group tracequery.StorageLatencySummary, path string, emit func(string)) {
	label := func(value string) string {
		if value == "" {
			return "未说明"
		}
		return clampToken(value)
	}
	source := "未说明"
	if group.SourcePath != "" {
		// Collection-machine paths keep the existing basename/disambiguation
		// policy; displaying a measurement must not expose operator directories.
		source = formatScalarForTag(reflect.ValueOf(group.SourcePath), "source_path")
	}
	thread := "组内线程=" + label(formatInlineStruct(reflect.ValueOf(group.Thread)))
	if group.Layer == "block" {
		thread = "代表提交者=" + label(formatInlineStruct(reflect.ValueOf(group.Thread))) + "（不限定本组线程）"
	}
	emit(fmt.Sprintf("- %s: IO请求组 来源=%s 层=%s 事件族=%s 设备=%s 操作=%s inode=%s 名称=%s %s",
		path, source, label(group.Layer), label(group.Event), label(group.Dev), label(group.Operation), label(group.Inode), label(group.EntryName),
		thread))
	if group.RequestResidenceCaliber != "" {
		start, done := tracequery.IORequestResidenceEndpoints(group.RequestResidenceCaliber)
		if start == "" {
			emit("  请求起止事件: 未说明；不据事件族推定耗时口径。")
		} else {
			emit(fmt.Sprintf("  请求起止事件: %s → %s；完整配对才有耗时，不等于线程阻塞时长。", start, done))
		}
	}
	emit(fmt.Sprintf("  配对记录: 观测计数=%d 已配对=%d 未配对起始=%d 未配对完成=%d 歧义组=%d 暂停配对=%d 字节统计=%d 原组最大耗时=%.3f ms 原组平均耗时=%.3f ms lines=%d-%d ts=%s..%s",
		group.Count, group.PairedCount, group.UnpairedStartCount, group.UnpairedDoneCount, group.AmbiguousCohortCount, group.PairingSuppressedCount,
		group.Bytes, group.MaxLatencyMs, group.AvgLatencyMs, group.LineStart, group.LineEnd, formatSecondsToken(group.StartTs), formatSecondsToken(group.EndTs)))
	if group.RequestLatencyDistribution != nil {
		renderIORequestLatencyDistributionDetail(*group.RequestLatencyDistribution, path+".request_latency_distribution", emit)
	}
	if group.Example != "" {
		emit(fmt.Sprintf("  原始示例: %s", clampToken(group.Example)))
	}
}

// The optional distribution remains under its source/family/device/operation
// group. Unlike the generic scalar walker this exact-type renderer preserves
// measured zero durations. A nil carrier stays absent in the enclosing walker.
// These request-residence statistics establish neither scheduler wait nor
// causal authority; no prose is inspected and no measurement is rewritten.
func renderIORequestLatencyDistributionDetail(distribution tracequery.IORequestLatencyDistribution, path string, emit func(string)) {
	emit(fmt.Sprintf("- %s: 已配对请求=%d 最短=%.3f ms 最长=%.3f ms 平均=%.3f ms P50=%.3f ms P90=%.3f ms P95=%.3f ms P99=%.3f ms",
		path, distribution.SampleCount, distribution.MinMs, distribution.MaxMs, distribution.MeanMs,
		distribution.P50Ms, distribution.P90Ms, distribution.P95Ms, distribution.P99Ms))
	method := "分位数算法未说明"
	if distribution.QuantileMethod == tracequery.IORequestLatencyQuantileMethod {
		method = "按排序样本的位置线性插值"
	}
	population := "样本范围未说明"
	if distribution.SamplePolicy == tracequery.IORequestLatencySamplePolicy {
		population = "该组内与查询范围相交且起止完整配对的全部请求"
	}
	caliber := "耗时口径未说明"
	if distribution.LatencyCaliber == tracequery.IORequestLatencyCaliber {
		caliber = "请求起始至完成的全程耗时，不按窗口边界裁剪"
	}
	emit(fmt.Sprintf("  统计口径: %s；%s；%s；仅描述请求耗时，不等于线程阻塞或响应耗时，也不单独证明链上根因。", population, caliber, method))
}
