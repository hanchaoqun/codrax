# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T08:47:07Z
- sweep_start_ts: 20260731-014707
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260731-014707 | log_regex,answer_regex | none | 159s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | Final `17,0,5`、reference-complete 投影与 reconcile 均正确；D1 已恢复完成门。首轮错误投影 `17,5` 被 typed reference grounding 修复一次。审计账只保留 r1/r2/r5 三条 target contribution，active+mapped 但不在 targets 的 r4/GroupB=4 在 join targets 后、compute 前被丢弃，违反“每个 included source record 进入贡献账”的审计闭包，登记 D2。 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260731-014707 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 316s | 39 | read=1,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | 四态值与 Σ 正确，显式窗因果投影/自动补齐仍在；T1 exact witness 已进入 prompt 和系统 caveat。主答案仍把 CPU4 direct max=2.10GHz policy-limit 行解释成“无策略限制/自然降频”，混淆 ceiling presence 与 binding impact，并写出 2.075GHz 高于 2.34GHz 的反向比较。runner 的首个 FAIL 原因另含 E2 大小写假阴性（`Running` vs `running`）。6 次 trace_query 比首轮 9 次少，但 evidence-floor 绕行造成 3 次 completion、2 次 emit_evidence、1 次 read_file，耗时升至 316s，登记 P2。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
