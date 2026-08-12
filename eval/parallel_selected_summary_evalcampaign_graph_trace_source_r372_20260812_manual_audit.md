# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T07:40:45Z
- sweep_start_ts: 20260812-004044
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260812-004045 | answer_regex,answer_contains,mermaid_edge_count | none | 158s | 28 | read=3,repo_map=4,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B623 阻止错误 context_only 后，Analyzer 改为直接漏掉 Mutable；BusContext 的 source_quote 仍是 Mutable/BusContext，但 participant slate 无 Mutable。最终 Mermaid 只画四阶段，BusContext 断开、Mutable 缺席。立案并修复 B625。 |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260812-004045 | trace_attachment,answer_regex | perf_triage+trace_query | 321s | 37 | read=5,repo_map=0,list=0,trace=1,source_lens=0 | midloop=6,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | B624 边界已贯通 prompt/handoff，H:、emitter-LIFO、scheduler absence 均改善；但答案把 SQL raw-marker recovery 分支称为通用“完整链路”，并称 parseTraceMarkValidated 提取时间戳，definition 摘要仍扩权。B624 production-partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

Runner `2/2`，人工 `0/2`。逻辑图 158s；trace/current-source 321s。后者超过四分钟但模型流保持活跃并正常产出最终答案，未回退旧稿、空答案或时间年龄降级，B615 再获生产正证。

### 1. qf_logic_view_read_pipeline

- Analyzer 首轮不再把 Mutable 标成 context_only，但 participant slate 仅包含 Analyzer/Explorer/Extractor/Finalizer/BusContext；`BusContext.source_quote="Mutable/BusContext"` 同时点名了 Mutable，系统却没有检查同一 typed provenance quote 中的 sibling 是否有独立 row。
- Explorer typed plan 因而只有 BusContext boundary obligation，没有 Mutable obligation。最终 Mermaid 仍只画四阶段 precedence，BusContext 断开，Mutable 完全缺席；正文声称跨阶段状态经 BusContext.Mutable 传递，却没有可见关系证据。
- 立案 B625：一个已接纳 participant 的 source_quote 若同时精确点名另一个 Analyzer typed entity，该 co-listed entity 也必须有独立 participant row；否则要求模型补 row 或把 quote 收窄到不点名 sibling 的精确短语。它不从完整请求猜参与者，不检查终稿，也不替模型创建边。

### 2. read_combo_trace_current_source_explanation

- B624 获得生产正证：Finalizer prompt、Evidence report 和 typed handoff 均明确渲染 `definition_site_only / executable_body=unproven`。相比 r371，答案不再声称 H: 被 instance-tag 逻辑剥离，正确采用 emitter/HeaderPID lane 的 LIFO 恢复，并继续遵守 scheduler absence 权限。
- 仍判 fail：答案把 `internal/hitraceconv/source_raw_marker_sync_recovery.go` 的 SQL/raw marker 恢复路径表述成 HiTrace span 的通用“完整链路”，没有与 tracequery 文本解析/其他 authority path 分支区分；并称 `parseTraceMarkValidated` 提取时间戳，实际它只解析 pipe payload，时间戳来自外层 row/event。definition refs 虽有 ceiling，模型提交的 aggregate member_notes/reason 仍携带函数体解释并被复述，故 B624 只能记 production-partial。
- 60Hz/16.7ms 仍作为举例出现，但答案明确说明 typed deadline 缺失、不能定谳 jank；这是模型软服从波动，不为该数字加终稿关键词硬门。

## 不变量

- Trace 显式窗、链上根因/业务线索、占时与可消除量双维、因果投影和自动补齐未改。
- 无请求/模型 reasoning/终稿原文扫描硬门；无系统答案替换或关系补边。
