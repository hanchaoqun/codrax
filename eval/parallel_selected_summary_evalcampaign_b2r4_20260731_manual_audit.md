# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T09:25:37Z
- sweep_start_ts: 20260731-022537
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- snapshot: `main@79e5d0215827`
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260731-022537 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 133s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | T3 真实覆盖：`Trace 因果投影` 恢复（runner 计 2 个投影块），frequency authority caveat 同时出厂；自动补采没有膨胀，模型 trace_query 降至 3 次。四态与 typed CPU0/CPU4 limit roster 正确。剩余 FAIL 是 principal 正文仍把 policy ceiling、实际低频与 thermal binding 混为一谈，并把 CPU0/CPU4/CPU12 的 limit count/身份写乱；系统权限块明确给出正确 CPU0/CPU4、line/ts/count 以及 binding impact 未证。禁止扫描模型正文做 hard gate，本项转为 T2 typed-claim-schema/模型服从性债，不继续按本 case 拟合。 |
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260731-022537 | log_regex,answer_regex | none | 208s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | D1/D2 连续防回退：贡献账仍为 4，GroupB=4 保留在 reconcile，最终 complete-reference projection 为 `17,0,5`。本轮 12 data rounds/3 repair；曾跨 rank 组合 producer/consumer、首次 assemble_answer 缺 complete_reference 参数而投成 GroupA/GroupB/GroupC，再由 typed completion 修复。correctness 闭合，P3 效率债继续开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
