# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T06:05:16Z
- sweep_start_ts: 20260828-230514
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_gson_lazy_number | FAIL | eval/results/github_issue_gson_lazy_number-20260828-230516 | write_apply,write_patch_oracle | none | 131s | 27 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (implementation) / honest-unverified | 仅修改生产 `LazilyParsedNumber.java`；`equals/hashCode` 均基于 `value`，测试与检查脚本未改；`make check` 通过。宿主没有 Java runtime，故行为验证明确为 `runner_missing/unverified`，runner FAIL 正确，不能伪绿。 |
| 1 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260828-230516 | primary_answer | none | 154s | 27 | read=8,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | 六条静态调用关系、容量守卫和 Mermaid 方向均正确，但把内存 `rows.add` 写成“持久化”、把 `System.out.println` 延续为“审计落库”，没有明确声明标准输出不等于数据库/持久化。连续多次历史回放同形失败，确认为跨语言调用链的 observed-operation/business-effect 权限 gap，不是关系丢失或一次模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论与处置

1. `github_issue_gson_lazy_number` 的代码实现正确，失败只表示当前宿主无法执行 Java 行为测试。系统保留 source-static 通过证据，同时拒绝把它升级成“已验证”，不构成新的生产代码 gap。
2. `sr_java_call_chain` 的 parser、读取闭包和 typed relation handoff 均完整：最终保留 `VisitController.create -> VisitService.schedule`、配置读取、计数查询、插入、审计调用和 `AuditLog.record -> System.out.println` 六条边，图语法与方向均有效。
3. 真正缺口是 endpoint profile 自相矛盾：Analyzer 同时发出 `sink_mode=discover` 与 `runtime_selection_profile=false`。前者要求运行时实现选择，后者明确本题不是运行时选择；现有 wire-normalizer 的注释承诺依据 typed selection flag 选择 `discover`/`discover_terminal`，实现却只修正 `discover_path` 输入。
4. `EVAL-B1441-CALLCHAINOBSERVEDEFFECT1/P1` 采用通用根修：一源空终点时由 typed runtime-selection boolean 确定 discovery lane；false 的 `discover` 归一为 `discover_terminal`，true 的 `discover_terminal` 归一为 `discover`。探索与成文共享跨语言 observed-operation/business-effect 软边界，要求逐跳陈述真实操作，方法名、类名、层名、精确 sink、运行时目标或图叶都不能自行证明持久化、存储、交付、写入等更强效果。
5. 修复不读取“落库”“stdout”、语言名、用户原文、模型 reasoning 或最终答案做硬门；不删除/创建关系，不替模型选择终点或改写结论。显式时间窗 Trace、因果投影、自动补齐、链上根因、两维根因与可消除量均不经过本 source-code endpoint 归一化车道。
