# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T06:46:57Z
- sweep_start_ts: 20260818-234655
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260818-234657 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 160s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 实质数值与权限边界正确：运行 157.248ms、可运行 5.604ms、S=70.338ms、scheduler-marked D/IO=0，且明确 CPU policy witness 与目标切片绑定未证。runner 只认英文 Running/Runnable，并要求过窄的 policy 语序，因此为 oracle false negative。B1139 读者卡已生产生效，但终稿仍复制一次 `dominant_state_slice_representative`，说明目标 CPU running/frequency 联接还缺自然语言终缝。有限事实题不物化完整因果投影是正确 scope。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260818-234657 | answer_regex,answer_contains,mermaid_edge_count | none | 940s | 46 | read=54,repo_map=4,list=0,trace=0,source_lens=2 | midloop=29,inv=33/0,fin_reject=1,unavail=3,prune=3 | partial | 终稿覆盖四阶段与部分 BusContext/Mutable 关系，但图缺少阶段到共享上下文的关键数据流，并画出一条随后又声明未证的 Mutable 虚线。Analyzer 发射 12 个实体后，R2 按名称前后缀把同一个耦合关系题扩成 6 个 subtopic；probe 与每条 evidence worker 又各自执行全请求 participant gate，重复追逐同一缺口，累计 116 explorer iterations、54 次 read、33 次 completion、29 次 midloop。runner PASS 掩盖了确定性编排/收敛 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
