# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T08:46:40Z
- sweep_start_ts: 20260831-014640
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-014640 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 172s | 41 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 10ms 窗、6 次 typed query、唤醒链、链上根因排序、实际占时/规则可消双账、自动补齐和最终 Trace 因果投影均完整；VerifyClass 0.285ms 也在投影中以规则可消 0、语义完成/目标等待绑定未证正确披露。但模型主摘要再次把 NetworkService 的 runnable 调度等待称为“确定性工作”，没有把调度状态与真正 semantic span 清楚分开；系统投影不能替换模型结论。非链 D/IO/CPU 仍只作背景，无固定 4ms/4m/活动流年龄降级。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-014640 | typed_inventory_rowset,dimension_substring,answer_contains | none | 328s | 29 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=4,inv=4/2,fin_reject=2,unavail=6,prune=0 | fail | B1481 获生产正证：首稿与终稿都使用 canonical `String`，12 条声明精确为 extend=2 / foreign func=2 / public class=8，路径和 package 齐全。过程仍有 2 次系统诱发修补：完整 Markdown 表的 items 是隐藏 sidecar，location gate 却把它们当可见行；随后维度门又要求精确 source-inventory 族/行块重复添加 `member_set`。终稿摘要另写“7 个包”却列 9 项且包含不存在的裸 `demo`，实际 distinct package 为 8；该自由派生算术不以答案 prose 硬门修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
