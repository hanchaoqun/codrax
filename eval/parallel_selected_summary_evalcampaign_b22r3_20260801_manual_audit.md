# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T15:56:40Z
- sweep_start_ts: 20260801-085639
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260801-085640 | answer_regex,answer_contains | none | 123s | 22 | read=8,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 默认值 50 与三级优先级结论正确；但正文把真实的 `cmd.Flags().Changed("pipeline-max-steps")` 写成不存在的 `flagMaxSteps.Changed()`。Explorer 已读到真代码，却因 condition anchor 使用错误 symbol 被拒后没有重新铸造 `Changed` call evidence；最终精确 API 关系无 citation 权限。 |
| 1 | patch_go_typo | PASS | eval/results/patch_go_typo-20260801-085640 | write_apply,write_patch_oracle,answer_contains | none | 170s | 18 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | patch 字节正确且同包 Go probe 已跑通；但同一 probe 中只执行首个 TestX，行为测试未运行；成功 native probe 又被标成 `verification_probe_unclassified`。同时缺 3 个 fallback contract refs 时上游跳过项目 suite、下游追加必然无法闭合的 cumulative review，最终只能 `accept_unverified`。按验证闭环判 fail。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
