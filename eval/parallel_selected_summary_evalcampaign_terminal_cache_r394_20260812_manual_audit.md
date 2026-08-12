# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T15:21:13Z
- sweep_start_ts: 20260812-082112
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260812-082113 | primary_answer | none | 189s | 24 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 四条主调用边、容量 guard 与合法 Mermaid 均在；但摘要和末跳仍把 `AuditLog.record` 说成“完成审计落库”。源码已完整读取，最终 prompt 却是 `selected_terminal_body_calls=unproven`。analysis 将请求未逐字命名的 `AuditLog.record` 从 exact sink 降到 `discover`；该模式自动要求 registration/initializer/return 型运行时选择证据。本轮只有静态调用链，故系统拒绝以 grounded leaf `AuditLog.record` 选终点，parser 已知的 `System.out.println` 无法进入最终上下文。这是 endpoint authority 类型缺口，不是模型波动或缓存残留。一次成文拒绝仅修正 diagram typed edge anchors，未改变正文错误。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-082113 | answer_regex,answer_contains | none | 198s | 32 | read=6,repo_map=4,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | 正确区分 `buildAnalysisIR -> gate.RunWith` 与平行 wrapper `gate.Run -> RunWith`，正文、关键函数表和 sequenceDiagram 都保留两条真实方向，没有把 `RunWith` 反画成调用 `Run`。runner PASS 与人工一致。Explorer 有四次 investigation-complete 拒绝/修复，主要来自装饰 member 与 no-directed-path 边界，继续作为 churn 观察量，不以单案加硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
