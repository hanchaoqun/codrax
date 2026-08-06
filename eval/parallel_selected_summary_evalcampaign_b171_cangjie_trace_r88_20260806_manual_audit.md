# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T08:34:12Z
- sweep_start_ts: 20260806-013410
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-013412 | typed_inventory_rowset,dimension_substring,answer_contains | none | 128s | 20 | read=3,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=3,prune=0 | fail | 2/2/8 三族清单和 package 均完整，零 finalizer reject、原生 JSON；但 `public class Cart` 的可见位置是 Cart.cj:14，系统保留/回填的引用却落到同名 `extend Cart` 的 Cart.cj:30。根因是不同 surface_family 的同名 type 共用候选角色，block 又漏发 `source_inventory_family`，引用修复只看到同名/同 package，无法区分族。三个 unavailable read_file 是 lens 已闭合后的低优先级效率噪声。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-013412 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 201s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | 显式窗、目标状态账、根因排序、wakeup 链、窗内可消除量、两轴、frame=absent 限定、投影与自动补齐均完整；但模型把三个 IO 席相加成约24ms、把 candidate 写成持有低优先级依赖、把 wakeup hop 写成额外等待传递，并产生 23.994ms/77% 错算。系统已给出不可相加/无 holder/wakeup 不等于 blocker 的精确约束且算术附注未改正文，属模型遵循波动。另有系统上下文 gap：探索已见 `cpu_pressure=604.528ms`，最终 Trace Decision Inputs 未携带该 typed 背景行，模型反而声称无显著 CPU 压力。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
