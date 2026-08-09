# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T08:35:20Z
- sweep_start_ts: 20260809-013518
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260809-013520 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B424 生产闭环：Explorer 只调用一次 completion，零拒绝且未提交当前无 authority 的 relation_claims；显式 2.000..2.020s、自动补采和 Trace 因果投影均保留。主根因只取 typed 链上的 threadpool-400 io_wait 11ms；logger-900 约 20ms 明确为 off-chain/background，不参与加冕。 |
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260809-013520 | log_regex,answer_regex | none | 199s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 答案严格为 {"ids":["u1","u3"]}，业务正确；但 6 批、4 次 repair、2 次 action failure 对三行输入明显过重。精确根因是当前 rank 的 schema enum 已收窄，而原生合法 JSON 未经过 schema 校验，assemble_answer/compute_contributions/filter_records 等越 rank 计划只能到工作流 guard 才被拒，形成长 repair。立案 B425。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- `trace_query_wakeup_background_demotion`: human PASS。B424 将 completion 从 r246 的 3 次/2 拒绝降到 1 次/0 拒绝，没有放宽执行器，也没有吞掉非法 claim。答案保留链上 11ms 主因、3 段链上 runnable 次因和 20ms off-chain logger 背景的权限分层。
- `data_json_strict_ids`: human PASS on answer correctness，operational FAIL on planning efficiency。最终 JSON 无解释污染、顺序和值正确；但 planner 在 typed `allowed_next_actions` 已经精确给出时仍多次发射未来 rank action。该问题不是 JSON 教学不足，而是 `unmarshalReplStructuredToolParams` 只在兼容归一化发生后调用 schema validator；原生、可解析 JSON 绕过了 narrowed enum。
- B425 的最优解不能是扫描模型 thinking/plan prose、自动删除非法 action 或扩写教学。应由 owning tool schema 对精确 opt-in 子树做同 schema 校验，并且只在 reducer 持久化状态足以证明当前 rank 时启用；失败后走现有单次 compact same-schema repair。
- 本轮没有发现 Trace 主因越权：邻近/背景事件只作为影响范围说明和额外排查支撑，不改变 typed on-chain 主根因。
