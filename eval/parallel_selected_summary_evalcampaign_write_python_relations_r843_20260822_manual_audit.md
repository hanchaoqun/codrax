# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T05:26:27Z
- sweep_start_ts: 20260821-222626
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260821-222627 | write_apply,write_patch_oracle,answer_contains | none | 89s | 26 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 仅将 `main.c` 的 `retrun buf;` 改为 `return buf;`；真实 apply、`make test`、ChangeReport、verification evidence 和交付 ref 全部闭合，主仓未改。编译生成的未跟踪 `main` 被 typed worktree audit 明确披露且未纳入交付，没有空证据签绿或累计验证域丢失。 |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260821-222627 | answer_regex,answer_contains | none | 285s | 37 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=6,unavail=0,prune=0 | pass | 终稿正确说明 `run_pipeline -> resolve`、`REGISTRY[name]` 查找、`cls()` 实例化、executor callback、`JsonPlugin` 与 `@register` 的导入期绑定；无 raw relation enum 或 `AUTHOR_BUSINESS_ACTION` 泄漏。过程/视觉为 partial：分析器同时发出 `sink_mode=discover` 与 `runtime_selection_profile=false`，动态选择车道被归一成普通终点发现；finalizer 只得到 5 个断开关系组件。模型连续 6 次用未证 call/反向 register/动态桥拼时序图，证据门正确拒绝，最终删除可选图。确认 B1328 typed 自一致性 gap，关系门不应放宽。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
