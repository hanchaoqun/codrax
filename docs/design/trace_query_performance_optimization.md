# trace_query 大 trace 性能优化设计

## 背景

客户日志 `slow_trace_1.log` / `slow_trace_2.log` 暴露出两类慢点：

- 模型在已经定位过时间窗后，后续 `trace_query` 调用有时丢失 `time_start/time_end`，例如只传 `thread=com.whatsapp` 调用 `scheduler_latency_stats`，导致大 trace 上全量扫描。
- 同一个大 trace 在同一轮内被多个 `trace_query` 并发调用时，当前缓存只能复用已完成的索引，不能合并正在进行的构建；并且同一文件的相对路径/绝对路径会形成不同 cache key。

这些问题只属于 runtime trace 工具路径，不能影响源码分析、`repo_map`、`grep/read_file`、MCP 或外部观察 + 源码混合分析路径。

## 红线

- 不新增工具参数，避免扩大 JSON 兼容面；现有字段继续走统一 JSON 修复链。
- 不改变 `trace_query` 的证据语义；所有结果仍是 runtime artifact 观测，不是源码引用。
- 不对源码场景做任何 hard gate。
- 大 trace 的保护只对 `trace_query` 重型视图生效；`event_search`、`span_window`、`frame_timeline` 等轻量/导航视图仍可作为定位入口。

## 根因

1. `BuildIndexWithOptions` 使用传入 path 作为 cache key。绝对路径和相对路径指向同一文件时不能复用索引。
2. `sync.Map` 只缓存完成后的索引。多个并发 `trace_query` 同时构建同一索引时会重复 parse。
3. windowed 查询发现 full index cache 后直接返回 full index，后续视图仍会扫全量事件。
4. `recipe`、`root_cause_rank` 等复合视图内部会重复计算 `WindowStats`、`SchedulerLatency`、`FramePipeline`。
5. 大 trace 重型视图在缺少时间窗/行窗/span/thread/pid/pattern 等缩窄条件时没有统一 guard。

## 方案

### P0.1 path canonicalize

在 `internal/tracequery.BuildIndexWithOptions` 内对 path 做 `filepath.Abs` + `filepath.Clean`，cache key 和 `Index.Path` 使用规范路径。失败时退回 clean 原路径。

### P0.2 index singleflight

新增 tracequery 内部 in-flight build 表：

- key 使用 canonical path + size + mtime + parser version + window key。
- 同 key 并发构建只允许第一个 goroutine parse；其它 goroutine 等待结果。
- 完成后写入现有 `indexCache`。

### P0.3 full-cache 派生 windowed index

当 windowed 查询命中 full index cache 时，不直接返回 full index，而是从 full index 过滤出轻量窗口索引：

- 保留原始 `LineCount`、`Size`、`ModTime`、flavor 信息。
- `Events` 仅保留 time/line padded window 内事件。
- `Windowed=true`，并保留 `IndexTimeStart/End`、`IndexLineStart/End`。

这样后续 `ComputeWindowStats` / `BuildSchedulerLatencyStats` 等仍只扫窗口事件。

### P0.4 大 trace 重型视图 guard

对大 trace 的无缩窄重型视图返回可恢复提示，不直接执行全量计算：

- 视图：`scheduler_latency_stats`、`root_cause_rank`、`window_stats`、`critical_blocking_calls`、`evidence_pack`、`recipe`。
- 缩窄条件：`time_start/time_end`、`line_start/line_end`、`span_name`、`pattern`。`pid/thread` 只能选目标，不能约束大 trace 的时间范围，所以不能单独绕过 guard。
- 返回内容包含 `next_call_hint`，建议先用 `event_search(pattern=...)`、`span_window` 或带时间窗重试。

### P1.1 复合视图内部复用

在 `tracequery.Run` 内引入单次调用本地 cache：

- `WindowStats` 只算一次。
- `SchedulerLatency` 支持接收已有 `WindowStats`，避免二次扫描。
- `FramePipeline` / `FrameTimeline` 在 recipe 内共享。

### P1.2 阶段日志和模型教学

- 在 `TraceQuery.Execute` 关键阶段输出 debug/info 诊断：build_index、run_view、marshal/store_summary。
- 更新工具描述和 explorer trace 教学：已选窗口后，后续 trace_query 必须继续带同一 `time_start/time_end` 或 `line_start/line_end`；重型视图不要丢窗口。

## 开发任务清单

- [x] 添加设计文档和任务清单。
- [x] `BuildIndexWithOptions` path canonicalize。
- [x] `BuildIndexWithOptions` singleflight。
- [x] full index 派生 windowed index。
- [x] 大 trace 重型视图 guard。
- [x] `Run` 内部 view 结果复用。
- [x] `BuildSchedulerLatencyStatsFromStats` 或等价复用入口。
- [x] trace_query 阶段诊断日志。
- [x] 工具描述 / explorer prompt 教学更新。
- [x] 单元测试：相对/绝对路径 cache、singleflight 基础、full cache 派生 windowed、guard、schema/prompt 教学覆盖。
- [x] `go test ./internal/tracequery ./internal/tool ./internal/agent`。
- [x] `go test ./...`。
