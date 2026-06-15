# TraceQuery Span End Pairing Gap

## 背景

客户给出 `bindApplication` / `H:touchEventDispatch` 这类 trace span 后，模型曾尝试搜索 `E|pid|spanName` 或只按 span 名查结束行。这个思路不符合 atrace/ftrace marker 语义：

- 同步 span 是 `B|pid|name` 开始，但结束通常是同一 ftrace 线程栈上的 `E|pid` 或裸 `E`，不会重复 `name`。
- 异步 span 是 `S|pid|name|cookie` / `F|pid|name|cookie`，需要按 marker pid、name、cookie 配对，不走 B/E 栈。

## 系统短板

- 解析层只把 B/E/C 识别为 `trace_mark`，没有稳定支持裸 `E` 和 S/F async marker。
- `span_window` 的底层配对语义没有显式覆盖 S/F；zero-match hint 也没有提醒模型不要搜 `E|pid|spanName`。
- prompt/schema 只说 B/E span，没有把“E 不带 name”和“S/F 按 cookie 配”讲清楚，模型容易退回 grep 式搜索。

## 修复方案

- `tracequery` 解析 `B/E/C/S/F`，并保留 marker payload pid 到 `span_pid`。
- `event_search` trace_mark 行输出 `span_action/span_pid/span_name/span_value`，`span_window` / `window_stats.trace_spans` 输出 `kind=sync|async`：
  - `sync`: 同一 ftrace 线程 pid 上维护 B/E 栈，`E|pid` 或裸 `E` 弹出最近 begin。
  - `async`: 以 marker pid + span name + cookie 配对 S/F。
- `trace_query` schema、explore skill、perf-triage skill、zero-match hint 同步说明：
  - 不要搜索 `E|pid|spanName` 来证明结束。
  - 用 `span_window(span_name=...)` 或 `event_search(..., event_types=["trace_mark"])` 先定位，再把选定 time/line window 带入后续分析。

## 看护

- `TestSpanWindowPairsNestedBESpanWithUnnamedEnd`: 覆盖嵌套 `bindApplication`，确认 outer span 被最后一个 unnamed E 关闭。
- `TestSpanWindowPairsBareEndOnSameThreadStack`: 覆盖裸 `E`。
- `TestSpanWindowPairsAsyncSFByCookie`: 覆盖 `H:touchEventDispatch` S/F cookie 配对，并确认 `event_search` 能返回 S/F trace_mark 行。
- Prompt/schema tests pin 住 B/E/C/S/F、same ftrace thread stack、`E|<pid>|<span_name>` 禁用提示。
