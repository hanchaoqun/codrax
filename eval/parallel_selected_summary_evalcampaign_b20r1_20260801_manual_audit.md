# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T11:36:49Z
- sweep_start_ts: 20260801-043648
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260801-043649 | write_apply,write_patch_oracle | none | 180s | 19 | read=8,repo_map=3,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 生产 diff 对当前 fixture 正确，人工在 applied tree 独立运行 unittest 为 4/4 PASS；但产品只执行了 1 个 months/non-integer probe，已检测到的 `python/unittest@.` 被记为 `suite_skipped`。ChangeReport 明示缺少 years/4 个 fallback 合同及 baseline，final proof=`weak`，最终报告却发布 completion=`verified/all_batches_verified` 和“测试通过”。日志显示 typed truth ledger 已尝试改成 `truth_ledger_weak_requires_proof`，随后被 completed workflow 的 `workflow_already_complete` 转回 finish，属于弱证据被完成态洗白的 P0 状态机 GAP，runner PASS 为假阳性。 |
| 1 | github_issue_gson_lazy_number_symptom | PASS | eval/results/github_issue_gson_lazy_number_symptom-20260801-043649 | write_apply,write_patch_oracle | none | 195s | 18 | read=7,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 diff 仅修改生产类，新增基于 wrapped value 的 equals/hashCode，Number 转换方法和测试文件未改。JDK/Maven 缺失后 `make check` 的项目级 source oracle 真执行并覆盖唯一 changed path，report proof=`strong`、completion=`verified` 一致；无第二 ChangePlan、无重复 apply。首次计划因冗余重叠 edit 被 old_text guard 拒绝，模型修复时丢掉 Java probe，但本轮由项目 runner 补足，记作低优先级过程波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
