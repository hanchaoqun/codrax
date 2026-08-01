# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T16:57:01Z
- sweep_start_ts: 20260801-095700
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260801-095701 | write_apply,write_patch_oracle,answer_contains | none | 108s | 18 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单一 plan、单一一行 patch，仅将 `retrun` 改为 `return`；隔离 worktree 内 `go test -json ./...` 成功，changed path 为 project_runner covered，最终 verified 与 report 一致。worktree 保留来自 eval 专用 `pipeline_keep_worktree_on_success=true`，用于 post-apply oracle，不是清理失败。低优先级 eval 噪声：固定写流程 10 个合法 dispatch 被统一阈值 8 标成 high_pipeline_dispatches。 |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260801-095701 | answer_regex,answer_contains | none | 137s | 22 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 默认值 50、YAML 指针覆盖与 CLI 最终优先均正确；2664 支持面只发布 `initApp guard condition IF !cmd.Flags().Changed(...)`，正文准确说明未显式传 CLI 时 `mergedMaxSteps` 回写 `flagMaxSteps`。50 引用 `cmd/root.go:88`，没有再引用 PipelineMaxStepsCeil 或产生 line/quote 错位。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
