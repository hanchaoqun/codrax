# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T13:25:21Z
- sweep_start_ts: 20260812-062520
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260812-062521 | answer_regex | none | 189s | 23 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=4/0,fin_reject=0,unavail=0,prune=0 | fail | 主链拓扑正确且没有关系硬门误拒；但 `walker::collect_files` 最终引用为 `src/main.rs:28(index_file)`。模型未提交 citations[] 却提交悬空索引，系统扩池时把无关引用填入同号槽，并在最终 checker 已报告 INVALID 后按 advisory 接受，登记 B649。另有“绝对路径”窄过度推断，记模型内容校准观察项。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-062521 | answer_regex,answer_contains | none | 223s | 30 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> gate.RunWith` 两支并列汇合，明确否定不存在的 `buildAnalysisIR -> gate.Run`；sequence participant 单一、方向与 typed call rows 一致，零成文拒绝。B633/B634 生产正向。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner `2/2`，人工 `1/2`。
- Go 时序图证明关系教学、规范 endpoint 与 typed call authority 同时生效；未因图关系门烧重试。
- Rust 主链关系没有丢失，但引用修复存在确定性“扩池碰撞”：原始池长为 0，悬空 refs 为 `0,1,3,4,2,5`；修复器依序追加其它证据后，原始 ref=3 意外指向 `index_file`。最终 checker 精确识别错误并给出 `src/main.rs:20/src/walker.rs:6/src/walker.rs:4/...` 候选，仍因通用 citation advisory 放行。
- 登记 `B649-CITATIONPOOLCOLLISION1=confirmed/P0`：所有原始越界引用必须在引用池增长前隔离，之后只能由 typed evidence/source-inventory 重新绑定；限定名的定义回绑使用跨语言 identity segment 契约并保持 owner/唯一性 fail-closed。
- “返回绝对路径”不受源码支持，但属于模型正文推断；当前不以原文扫描、硬门或系统改写处理，继续在后续异构回放观察内容校准的泛化频率。
