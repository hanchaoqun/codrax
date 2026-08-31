# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T07:27:18Z
- sweep_start_ts: 20260831-002716
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-002718 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 156s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 10ms 窗、typed 查询、链上根因排序、四态账、实际占时/规则可消双账户与 Trace 因果投影均保留；系统投影也正确披露 VerifyClass 0.285ms 只有 host→target 唤醒凭证、语义完成/目标等待绑定未证、规则可消 0。分析器却只发出 function_or_purpose + relation_path + causal_contributor_set，没有 runtime_work_relation，模型主回答因此未独立回答“是否存在该类运行时工作及其成本/关系边界”。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260831-002718 | typed_inventory_rowset,dimension_substring,answer_contains | none | 373s | 29 | read=10,repo_map=2,list=0,trace=0,source_lens=2 | midloop=7,inv=4/0,fin_reject=1,unavail=2,prune=0 | fail | B1476 生效：1601s→373s，重复 exact-universe 不再制造 13/8 清册。新故障是最后一次 explorer completion 把源码 Dog/Service 错写为 Cat/Vehicle，却绑定 Dog/Service 的真实位置；location+family-only 的 prompt-row 准入把错名铸成 Principal Enumeration Rows。首版答案已按源码写对 Dog/Service，修补轮反被错误 typed roster 拉回 Cat/Vehicle，最终 public class=9。不是语言专例或纯模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
