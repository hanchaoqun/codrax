# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T15:58:07Z
- sweep_start_ts: 20260831-085806
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-085807 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 199s | 45 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B1495 的 analyzer typed 位已生效，显式 10ms 窗、8 次 Trace 查询、唤醒链、根因排序、实际占时/规则可消双账、业务 span、自动补齐和最终 Trace 因果投影均保留；非链项仍只作背景。但模型自己的 principal 回答仍未明确回答 `VerifyClass ...` 0.285ms 与目标的关系凭证及“工作完成/目标等待/帧因果未证”边界，这些精确信息只在系统投影中出现。`fin_reject=0` 证明旧 marker-only 完成检查把“挂了 facet”误当“已经回答”，确认 B1496。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-085807 | typed_inventory_rowset,dimension_substring,answer_contains | none | 200s | 29 | read=0,repo_map=5,list=0,trace=0,source_lens=3 | midloop=2,inv=4/0,fin_reject=0,unavail=6,prune=0 | fail | 12 条 typed declaration 主表本身完整且逐行路径、符号、package、类别正确，2/2/8 分类也正确；但自由摘要自行声称“分布在 5 个包路径”，表中实际有 8 个唯一 package，并称 Cart/String 都出现两次而 String 只出现于 extend 行。核心清单能力通过，未请求的派生计数/概括仍发生模型算术波动；不增加答案原文扫描硬门，继续以 typed 表为权威并在异构回放观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner: 2/2 PASS; human: 0/2 strict PASS.
- Highest-ROI structural gap: `B1496-RUNTIMEWORKROWRECEIPT1`. B1495 only activates the obligation; it does not prove that a visible model-owned block selected an exact typed runtime-work row and an evidence-bounded conclusion.
- Cangjie roster authority is intact. The wrong package-count prose is not promoted into a case-specific hard gate; it remains an untyped model-derived arithmetic/summary observation.
