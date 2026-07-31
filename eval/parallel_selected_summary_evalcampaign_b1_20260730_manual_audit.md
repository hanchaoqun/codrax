# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T03:39:55Z
- sweep_start_ts: 20260730-203955
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260730-203955 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 127s | 35 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 自动 oracle 被系统补充内容满足；正文漏三个精确时间，D-state 与 io_wait 口径含混；系统附录错误扩成全量因果树并产生互相矛盾的覆盖/计数/交叉校验结论。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260730-203955 | write_apply,answer_regex | none | 344s | 17 | read=10,repo_map=5,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终源码修复与 false/0/空串回归测试正确；但两项小变更被按 source/test 角色拆片，第一次只改生产代码后执行全量验证，必然失败并触发一次多余重规划。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Case 1 — real_trace_c2_dstate_iowait

人工真值复核：

- 三段不可中断等待分别约为 `34579.451701..34579.451839`、`34579.452934..34579.453081`、`34579.471372..34579.471722/743`。
- 三段均带 `iowait=1`，caller 均为 `sync_buffer_read_wi`，合计 `0.635ms`。
- 对外语义应为“D/不可中断族共 3 段，全部被归类为 io_wait；非 IO D-state 为 0”，不能把“非 IO D-state=0”简写成“没有发生 D-state”。

类别级 GAP：

1. `EVAL-B1-T1`：`IntentTrace` 被无条件映射为 `QFCallChain`。trace 是证据来源，不是答案形状；状态事实问题因此继承 `principal_path_edge`，时间/次数/原因没有成为主合同。
2. `EVAL-B1-T2`：系统补采按“所有核心因果族是否齐全”决定，而不按 typed 请求族决定。状态查询已具备 `target_window_states + blocked_reason_census`，仍因缺 rank/chain 自动补 `root_cause_rank + critical_blocking_calls`。
3. `EVAL-B1-T3`：某次 `event_search` 的局部 zero-match refinement 被提升为报告级“没有因果行”，即使最终投影已经有根因与唤醒链。
4. `EVAL-B1-T4`：五态互斥账中的 `d_state` 实为“非 IO D-state”，四态账又把 `d_state+io_wait` 显示为 D-state；标签未说明口径，导致同答内 0 与 0.635ms 看似冲突。
5. `EVAL-B1-T5`：IO-wait 正文交叉校验错误使用 `non-IO d_state + sleep_io` 作 comparator，漏掉独立 `io_wait` typed lane，制造 `0.635 > 0` 假告警。
6. `EVAL-B1-T6`：合并根因行的 `member_count` 是直接成员组数，渲染却一律说“共 N 段”；嵌套聚合时 2 个组可含 3 个原始段。
7. `EVAL-B1-E1`：当前 answer oracle 能被 deterministic supplement/footer 满足，无法证明模型主答案覆盖客户要求；需要 principal-answer 专用 oracle 作用域。

## Case 2 — github_issue_zod_prefault_symptom

- 最终差异正确：truthy 检查改为属性存在性检查；新增 false、0、空字符串三类回归测试；`make check` 通过。
- `EVAL-B1-W1`：online slice 默认同时按 owner/role 拆分，即使整个计划只有“生产修复 + 直接回归测试”两项。第一次 slice 只落生产代码，随后全量验证必然报告“测试未添加”，造成一次额外 verifier/planner/coder 循环。
- 泛化方案：仅对不超过在线 slice 上限且不含 package/CI/权限等隔离路径的小计划保持原子；大计划、跨风险路径和显式隔离规则不变。
