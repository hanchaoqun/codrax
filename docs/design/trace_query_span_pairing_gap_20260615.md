# TraceQuery Span End Pairing Gap

## 背景

客户给出 `bindApplication` / `H:touchEventDispatch` 这类 trace span 后，模型曾尝试搜索 `E|pid|spanName` 或只按 span 名查结束行。这个思路不符合 atrace/ftrace marker 语义：

- 同步 span 是 `B|pid|name` 开始，但结束通常是同一 ftrace 线程栈上的 `E|pid` 或裸 `E`，不会重复 `name`。
- 异步 span 是 `S|pid|name|cookie` / `F|pid|name|cookie`，需要按 marker pid、name、cookie 配对，不走 B/E 栈。

## 系统短板

- 解析层只把 B/E/C 识别为 `trace_mark`，没有稳定支持裸 `E` 和 S/F async marker。
- `span_window` 的底层配对语义没有显式覆盖 S/F；zero-match hint 也没有提醒模型不要搜 `E|pid|spanName`。
- prompt/schema 只说 B/E span，没有把“E 不带 name”和“S/F 按 cookie 配”讲清楚，模型容易退回 grep 式搜索。
- 追加复盘：REPL 已经能在 prompt 中看到规则，但一旦模型先用 `grep` 找到 `B|pid|name` 行，`grep` 的 small/no-match/broad 输出没有把“不要搜命名 E、下一步用 `span_window`”带回观察结果；首轮 trace-query-first 守卫只能约束初始工具选择，不能修正后续 grep 结果上的错误推理。

## 修复方案

- `tracequery` 解析 `B/E/C/S/F`，并保留 marker payload pid 到 `span_pid`。
- `event_search` trace_mark 行输出 `span_action/span_pid/span_name/span_value`，`span_window` / `window_stats.trace_spans` 输出 `kind=sync|async`：
  - `sync`: 同一 ftrace 线程 pid 上维护 B/E 栈，`E|pid` 或裸 `E` 弹出最近 begin。
  - `async`: 以 marker pid + span name + cookie 配对 S/F。
- `trace_query` schema、explore skill、perf-triage skill、zero-match hint 同步说明：
  - 不要搜索 `E|pid|spanName` 来证明结束。
  - 用 `span_window(span_name=...)` 或 `event_search(..., event_types=["trace_mark"])` 先定位，再把选定 time/line window 带入后续分析。
- `grep` 作为 runtime artifact fallback 时增加结果级软引导：
  - 命中 `B|pid|name` / `S|pid|name|cookie` marker begin 行时，在观察结果中给出 `trace_marker_span_begin_hint`，指向 `trace_query(view="span_window", span_name=..., line_start=...)`。
  - 搜索形态像 `E|pid|spanName` 且 zero-match 或 broad-match 时，给出 `trace_marker_span_end_shape`，明确 zero-match 不能证明 span 未结束。
  - 该引导只解析工具参数和 trace marker 行结构，只做软提示，不把用户意图或模型散文接入硬逻辑。

## 看护

- `TestSpanWindowPairsNestedBESpanWithUnnamedEnd`: 覆盖嵌套 `bindApplication`，确认 outer span 被最后一个 unnamed E 关闭。
- `TestSpanWindowPairsBareEndOnSameThreadStack`: 覆盖裸 `E`。
- `TestSpanWindowPairsAsyncSFByCookie`: 覆盖 `H:touchEventDispatch` S/F cookie 配对，并确认 `event_search` 能返回 S/F trace_mark 行。
- Prompt/schema tests pin 住 B/E/C/S/F、same ftrace thread stack、`E|<pid>|<span_name>` 禁用提示。
- `TestGrepTool/runtime artifact trace marker begin points to span_window`: 覆盖先 grep 到 begin 行时，工具结果会给出 `span_window` 下一步。
- `TestGrepTool/runtime artifact named end no match is not missing span proof`: 覆盖 `E|pid|spanName` zero-match 不再被模型误读为没有结束。
- `TestGrepToolPromptDocumentsRuntimeArtifactControls`: 固化 grep prompt 中 runtime trace span 的 fallback 规则。
