# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T11:34:17Z
- sweep_start_ts: 20260821-043417
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-043417 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 217s | 34 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 系统面正确保留显式 2.000..2.020s 主窗、4 节点唤醒链、链上 11.000ms IO 首席、3 个各 1.000ms 反转候选、占用/可消除双账与完整 Trace 因果投影；邻近/背景未加冕，且无固定 4ms/4m 降级。模型正文却把三段等待表述为“共同叠加出”20ms，并把 IO 写成“阻塞整链”，超出 typed caveat 的关系权限，判定为既有 B1269/B1271 软引导残余，不由系统改写结论。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-043417 | answer_regex,answer_contains | none | 384s | 44 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=14,inv=4/0,fin_reject=3,unavail=0,prune=0 | partial | 表格与正文基本可用；analyzer 本轮显式拆成两个 subtopic，因此不能把 B1291 的自动耦合修正宣称为生产命中。finalizer 经 3 次关系修补后使用 B1288 的 live addition_ref 成功补齐三条 typed precedence，证明原子新增通道有效；但模型以 analyze/explorer 等未声明别名写边，Mermaid 隐式再建参与者，造成 ANA/EXP/EXT/FIN 声明节点与关系节点分裂，图视觉上仍像丢关系，确认 B1292。另有一次未知 JSON 字段后自纠，暂记教学/模型噪声。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
