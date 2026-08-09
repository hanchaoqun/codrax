# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T05:52:22Z
- sweep_start_ts: 20260808-225221
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260808-225223 | write_apply,write_patch_oracle,answer_contains | none | 128s | 21 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确修改 `main.go` 一行 `retrun -> return`；隔离 worktree 中 `go test -json ./...`、`TestGreet` 与 changed-path coverage 均通过，workflow 进入 verified。planner 前两次分别因空 path 和与改动模块脱节的 standalone probe 被拒，第三次移除无关 probe 后闭环；属 P2 计划效率问题，不影响交付正确性。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260808-225223 | answer_regex,answer_contains | none | 262s | 41 | read=9,repo_map=4,list=0,trace=0,source_lens=0 | midloop=9,inv=1/0,fin_reject=2,unavail=0,prune=1 | fail | 四阶段成员身份已正确为 Analyze/Explore/Extract/Finalize，B415 未复发；但逐行输入、输出、状态载体缺少对齐的上游属性证据，答案自行补出 `ExploreOutputRequestsFactRetry`、不存在的 `BusContext.ExtractOutput` 等弱/错误事实并遗漏 `Mutable`。结构化表头又退化为 `列 2..列 5`。两次成文拒绝中，Mermaid `Note` 内的 `->` 被误当真实边，且 typed anchor 的全限定身份与显示别名未精确归一。B413 仅证明重试显著下降，B414/B415 只有正向生产信号，尚不能据此关闭。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
