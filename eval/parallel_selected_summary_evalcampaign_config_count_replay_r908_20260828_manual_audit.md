# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T22:28:38Z
- sweep_start_ts: 20260828-152836
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-152838 | answer_regex,answer_contains | none | 185s | 28 | read=21,repo_map=0,list=0,trace=0,source_lens=0 | midloop=15,inv=3/0,fin_reject=1,unavail=0,prune=0 | pass | 模型正文正确解释 `Decode`、默认 50、YAML 覆盖和 CLI `Changed` 分支；一次把 3 个 summary 合成 1 个的结构 patch 成功，没有丢答案。B1412 重复 atom 未回归；但 21 次 read/15 次 midloop 比 r907 更重，且模型表已覆盖默认→YAML→CLI 后系统仍追加同义“成员清单补充”，B1413 继续开放。 |
| 2 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260828-152838 | answer_regex,answer_contains | none | 488s | 45 | read=45,repo_map=4,list=1,trace=0,source_lens=4 | midloop=10,inv=8/2,fin_reject=1,unavail=1,prune=0 | fail | runner 正确报 `dynamic_scalar_binding_missing:kind_constants:30`。第 14 轮已验收正确 3 type/5 function/30 constants；后续并行调查却提交含 15 个不存在成员的 45 常量集。typed same-member gate 正确排除错误集，但同时把先前已验收 30 行集覆盖丢失，finalizer 只收到 3 type+3 function 的 6 行 principal contract。终稿把常量写成 45、只列 26 个，事实和完整性均失败。B1415 exact-row 修复本轮未获得常量载体生产正证；新立 B1416。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- 新确认 `B1416-REJECTEDCURRENTERASESACCEPTEDSET1/P1`：并行/后续 completion 的错误成员集在 typed grounding 校验后被排除时，当前实现会把此前已经成功验收的同集合事实一并丢掉。错误 retry 没有权限缩减 accepted closure。
- B1410 还有一个同根边界：稳定 5 行函数集与后续 3 行子集只因 `unit=functions` / `unit=个函数` 显示漂移未被识别为同 bucket。精确 member strict-subset + compatible label/dimensions 才应决定 superset 保留，free-form unit 不能独立否决。
- 最优实现只消费结构化事实和 typed rejection indexes：保留 unit 漂移下的已验收 superset；对被明确排除的 current fact，只恢复其成员完全包含的 accepted stable fact。不得复制错误 current 的任何成员，不得合并 crossing set，也不从请求、思考、终稿或 Markdown 扫描数量。
