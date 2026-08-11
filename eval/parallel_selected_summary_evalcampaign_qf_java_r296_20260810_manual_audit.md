# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T05:58:11Z
- sweep_start_ts: 20260810-225809
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260810-225811 | primary_answer | none | 149s | 23 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 五条调用边、容量 guard 与引用均正确，但终点实现仍未被读取/提供：答案把 `AuditLog.record` 写成“在同一次数据库事务上下文中写出审计日志”，实际 `AuditLog.record` body 是 `System.out.println`。B508 v1 的 producer 错把 sibling leaf `VisitRepository.countOpenVisits -> String.startsWith` 当 selected-terminal body fact；`AuditLog.record -> System.out.println` 缺席。精确根因为 initializer `audit -> AuditLog` 的 RHS 与规范化 call target `AuditLog.record` 未被 selection join 接受，随后 discover-sink 错误回退到任意图叶子；Explorer 也没有精确读取 `AuditLog.record` body。不是模型波动。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260810-225811 | answer_regex,answer_contains | none | 366s | 39 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 正文和阶段表基本正确，但完整 sequence 首稿因缺 typed edge authority 被拒；第二稿仍混用 participant label 与 typed endpoint，又被拒。第三稿只保留 3 条 stage precedence 与 3 条 implementation call，形成三组互不相连的关系，未表达一次请求从 Analyze 到 Finalize 的完整调度时序。B509 正向保持；B510 从 watch 升为 confirmed context-salience/evidence-skeleton gap。严格门拒绝无证边本身正确，不能放宽。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
