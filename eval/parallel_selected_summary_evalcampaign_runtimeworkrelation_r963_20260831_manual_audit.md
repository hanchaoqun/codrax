# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T08:29:30Z
- sweep_start_ts: 20260831-012929
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-012930 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 46 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Analyzer 第二次提交同时保留 causal_contributor_set、runtime_work_relation、evidence_source、observed_value；模型 principal block 实际占用 runtime_work_relation 席，并回答 VerifyClass 0.285ms、effective=0。显式 10ms 窗、3 次 typed query、已证唤醒链、NetworkService 5.951ms 链上第一席、目标状态账、实际占时/规则可消双账、自动补齐、最终 Trace 因果投影和非链 D/IO 背景隔离均完整，B1478 获生产正证。但模型仍把 NetworkService runnable 调度等待称为“确定性优化工作”，并把 VerifyClass 描述为重叠在唤醒路径上而未在主回答清楚保留 semantic-completion/target-wait binding 未证边界；typed 投影虽正确但不能替模型回答，故人工 fail。该措辞仅记模型语义波动/soft-context 观察，不增加答案原文硬门。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-012930 | typed_inventory_rowset,dimension_substring,answer_contains | none | 485s | 27 | read=0,repo_map=5,list=0,trace=0,source_lens=5 | midloop=4,inv=13/6,fin_reject=1,unavail=6,prune=0 | pass | 最终答案精确列出 extend=2、foreign func=2、public class=8；12 条唯一声明的路径、符号和 package 完整，Cat/Vehicle 与重复声明别名均未再进入结果，B1480 获生产正证。过程仍有 1 次不必要成文拒绝：typed roster 把说明性成员 String (extend operator) 当规范身份，而第一稿正确使用 String；根因是 model member 的无标签 support_ref 只有精确 file:line，canonicalizer 错把空 ref label 当冲突。已立 B1481，以“成员标签匹配 + 唯一 typed 坐标；有 ref label 时才额外校验”统一修复，歧义/错名继续 fail-closed。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
