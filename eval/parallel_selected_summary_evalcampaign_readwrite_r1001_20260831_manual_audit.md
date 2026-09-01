# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T06:18:16Z
- sweep_start_ts: 20260831-231814
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault_symptom | FAIL | eval/results/github_issue_zod_prefault_symptom-20260831-231816 | write_apply,answer_regex | none | 155s | 27 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 补丁准确把 `_prefault` 真值门改为存在性门，并补 false/0/空串与既有 default 保留回归；`make check` 仅以 Python 正则检查 TypeScript 源形，系统正确把最终完成态降为 unverified。新 GAP B1530：同一 final report 的 proof=strong、proof_ledger=verified 与 completion=unverified 相互冲突。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260831-231816 | answer_regex,answer_contains | none | 539s | 49 | read=8,repo_map=4,list=0,trace=0,source_lens=1 | midloop=14,inv=3/0,fin_reject=6,unavail=0,prune=1 | fail | 首稿表格/正文基本可用，但 sequence 图扩写了大量无 typed operation 的 dispatch/state edges，关系门正确拒绝；随后五次原子 patch 因新节点显示名字段遗漏与一次畸形 action 枚举反复失败，最终恢复未通过校验的旧图并重复正文。B1531：模型已明确选择可读 node id 时仍强制逐端点重复 visible label，产生高心智/重试放大；应限于语法层安全吸收，不能放宽关系权威。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
