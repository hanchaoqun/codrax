# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T12:57:34Z
- sweep_start_ts: 20260814-055733
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260814-055735 | answer_regex,answer_contains | none | 136s | 26 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | required sequence 的 relation_scope_quote 与 entities 明确含 buildAnalysisIR/gate.Run，但 participants=[] 且 discover 空 sink 获接纳；Explorer 只查 buildAnalysisIR 同体调用，漏掉真实 gate.Run -> RunWith@gate.go:135，终稿和图均不满足题设终点。B797。 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260814-055734 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 259s | 34 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 主值正确：Running=157.248ms、Runnable=5.604ms、Sleep=70.338ms、D/IO=0，且区分策略上限存在与目标绑定未证。失败含 oracle 对“D 态”词形及限制边界措辞过窄；正文另有 CPU0/CPU1 笔误、把完整 Sleep 过度联系 IO Burst、内部 typed token 泄露。此题是 bounded facts/单一 effect verdict，不要求全量 Trace 因果投影；Analyzer 将 effect verdict 标 observed_value 是模型分类波动，继续异构回放，不做原文硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
