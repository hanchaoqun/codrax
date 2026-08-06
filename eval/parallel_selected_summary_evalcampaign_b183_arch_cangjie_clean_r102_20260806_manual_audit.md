# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T14:55:33Z
- sweep_start_ts: 20260806-075532
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_architecture | PASS | eval/results/qf_architecture-20260806-075533 | answer_regex,answer_contains | none | 124s | 27 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S4 生效：support-only source inventory 未再投影系统 principal rows，finalizer 零拒绝，答案也不再附带无关源码声明。但模型把宽 `AllStages`/`builtinStageBindings` 中存在的 `StageMultiRepoFocus` 解释为当前 read-mode conditional pre-stage，给出 3+4=7；窄执行权威 `ReadModeConditionalPreStageBindings()` 实际只返回 LogTriage/PerfTriage，当前 topology 是 2+4=6。模型已读取并发出该窄函数证据，却让自己错误的 7-member aggregate 覆盖它。立案 DECLACTIVE1。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-075533 | typed_inventory_rowset,dimension_substring,answer_contains | none | 139s | 21 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 真正 source declaration inventory 保持 principal：2 个 extend、2 个 foreign function、8 个 public class，摘要、正文、路径、符号和 package 与 typed rowset 一致；无 JSON 修复、成文拒绝或系统补写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
