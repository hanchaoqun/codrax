# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T13:44:50Z
- sweep_start_ts: 20260812-064449
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260812-064451 | answer_regex | none | 151s | 23 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=5/1,fin_reject=0,unavail=0,prune=0 | fail | 模型本轮提交完整 citations[]，B649 悬空 ref 车道未触发；backtick normalizer 将标签 `walker::collect_files` 的正确 callsite 引用移到正文次级符号 `walk` 的定义。最终 checker 精确报 INVALID 仍 advisory 出厂。登记 B650。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-064451 | answer_regex,answer_contains | none | 256s | 32 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | B649 生产触发并隔离 1 条悬空 ref；但模型反转 wrapper 关系，声称 `gate.RunWith -> gate.Run`，首稿图又用未规范 alias，关系门拒绝后 patch skeleton 删除质量门主边。最终正文仍保留错误方向、图丢主关系。登记 B651，不能归为纯模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner `2/2`，人工 `0/2`。两项均正常生成答案，151s/256s 活跃流没有按年龄降级、旧稿替代或空答。
- B649 的 pool-growth quarantine 在 Go 生产触发，证明第一层机制有效；Rust 则暴露同根的第二条覆盖路径：正文 backtick helper 抢占结构化 label 的引用身份。
- 登记 `B650-CITATIONLABELBODYPRIORITY1=P0`：symbol-like `item.label` 是唯一主身份；item body 中的 code token 只能给非符号/展示型标签补引用，不能把一个符号行的引用移向 helper/callee/peer。label 与 typed evidence 已对齐时保留 callsite；definition fallback 仍由 block claim form 与唯一性决定。
- 登记 `B651-SEQUENCEENDPOINTDIRECTIONLOSS1=P0`：typed authority 已有 `buildAnalysisIR -> gate.RunWith` 和 `gate.Run -> gate.RunWith`，上下文/图 repair 却没让模型稳定消费，最终出现 `gate.RunWith -> gate.Run` 反向正文，且硬门只修图不修正文。后续应从 typed endpoint recipe/alias/skeleton 单源修正，不扫描或替换正文。
