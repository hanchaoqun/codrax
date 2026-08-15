# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T16:38:12Z
- sweep_start_ts: 20260815-093810
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-093812 | answer_regex,answer_contains | none | 123s | 26 | read=4,repo_map=1,list=0,trace=0,source_lens=1 | midloop=7,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | 事实正确：默认成员只有 `explorer`，注册点和 `Name()` 返回点均被读取并引用。但前两次 completion 为一个成员提交两个 positional `support_refs`，与公开的一成员一 ref 合同不符，第三次改为单一 labelled ref 后通过，属于模型合同违例而非系统矛盾。真正 GAP 是确定性的 `RegisterDefaultSubAgents -> SubExplorer.Name -> "explorer"` 双锚链未被 typed relation provider 消费，正确成员集仍显示 `fact_authority=advisory_model_inference/principal_contract=not_authorized`，Finalizer 因而回退到四个 supporting symbol 并追加误导性的“证据较弱”系统 caveat。立案 B844。 |
| 2 | github_issue_dayjs_duration_nan_symptom | FAIL | eval/results/github_issue_dayjs_duration_nan_symptom-20260815-093812 | write_apply,answer_regex | none | 181s | 24 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 修补方向与落地 diff 正确：`Number(value)` 改为 `Number(value || 0)`，fixture 已有 PT1H 与完整 duration 回归测试，`make check` 的 Python source-shape 检查通过。环境没有 `node/npm`，行为 probe 和 `npm test --` 均为 `runner_missing`；系统保留静态通过结果但最终诚实签 `unverified`，没有把 source-shape 冒充 JS 行为绿。这是执行环境不可用，不是待修系统 GAP，也不能通过降验证杆让 runner 变绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
