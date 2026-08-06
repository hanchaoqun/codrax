# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T14:18:23Z
- sweep_start_ts: 20260806-071822
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-071823 | typed_inventory_rowset,dimension_substring,answer_contains | none | 130s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 真正 declaration inventory 保持 principal；2 extend + 2 foreign func + 8 public class 的摘要数字、正文成员、路径和 package 全部一致。r99 的“10 vs 8”未复现，判为模型波动 witness，但结构化跨面计数复核仍有泛化价值。无 JSON/成文重试。 |
| 1 | qf_architecture | PASS | eval/results/qf_architecture-20260806-071823 | answer_regex,answer_contains | none | 517s | 39 | read=2,repo_map=16,list=0,trace=0,source_lens=16 | midloop=22,inv=13/1,fin_reject=0,unavail=0,prune=1 | fail | analyzer 单次成功但直接发 intent=enumerate + scenario=architecture_explain + source_inventory(constant,summary)，绕过 S2；系统把已完整的 7 个 PipelineStage 机制集合错误绑定到全仓 constant universe，10 次 completion DOWNGRADED 后才 low-delta force-complete。最终 4 主 stage 的职责正确，但开头仍把 3 个条件 stage 统称“运行时上下文非空”，对 MultiRepoFocus 不精确。自动 PASS 未覆盖 517s 权属循环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
