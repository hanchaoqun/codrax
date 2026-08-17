# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T18:20:38Z
- sweep_start_ts: 20260817-112036
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-112038 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 203s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 主窗 1.000000..1.010000、链上 #1 worker-200 8.300ms、Trace 因果投影、邻近/背景分层均正确；模型仍在正文复制 `priority_inversion_candidate`，且把候选机制写得接近已证阻塞，客户语言与证据口径仍需减负。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-112038 | answer_regex,answer_contains,mermaid_edge_count | none | 219s | 36 | read=5,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=1,unavail=1,prune=0 | partial | `BusContext` 包含 `Mutable` 的无箭头 grouping 已进入终稿，stage precedence 保留，拒绝/patch 从 r632 的 3/3 降为 1/1；图仍把 `precedence @ file:line`、`call @ file:line` 暴露给读者，正文还把部分未证局部读写描述成端到端数据流。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

1. B996 获得生产正证：parser-owned owner/member/type 合取只产生无箭头包含关系，未虚构
   `BusContext/Mutable` 与四阶段之间的有向边。相比 r632，QF 时长由 533s 降为 219s，
   finalizer reject/patch 由 3/3 降为 1/1。
2. Trace 正对照没有退化：显式时间窗、链上-only 根因、规则计价可消除量和背景压力分权保持；
   系统没有替换模型结论，活跃字节流跨过 4ms 后正常完成。
3. 新确认 B997：typed wire/control token 虽应留在 JSON 与审计载体，模型仍会把
   `priority_inversion_candidate`（历史另有 `bounded_window_candidate`）复制进客户正文。
   最优方案是最终合成尾部给出本轮实际枚举的自然语言映射；不扫描、不拒绝、不改写输出。
4. 新确认 B998：图的关系事实正确，但可见标签仍承载 relation kind、validator/recipe 词和源码位置，
   使架构图更像内部审计工件。应把精确位置留在 citation/anchor，把可见图文改为业务职责/动作；
   该指导不得增删、反转或连接关系。
5. QF 正文对“每阶段产出作为下一阶段输入”、Explorer/Finalizer 直接读写 Mutable 的表述超过当前
   typed relation 证明，暂记 B999 观察项；优先通过更精确上下文和异构回放判断，不设答案词面硬门。
