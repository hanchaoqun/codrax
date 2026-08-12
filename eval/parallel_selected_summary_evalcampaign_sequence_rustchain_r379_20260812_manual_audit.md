# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T09:49:25Z
- sweep_start_ts: 20260812-024924
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-024926 | answer_regex,answer_contains | none | 198s | 29 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=7,inv=5/1,fin_reject=1,unavail=0,prune=0 | pass | B632 生效：明确不存在 buildAnalysisIR→gate.Run，选择 no_directed_path，并以两条入边汇聚到 RunWith 解释。残余 B634：图中把 gate.RunWith 与短名 RunWith 画成两个节点，未把同一端点收敛为单一身份。 |
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260812-024926 | answer_regex | none | 201s | 27 | read=3,repo_map=3,list=1,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 正确给出 main→run、run→walker::collect_files、collect_files→walk、run→index_file、index_file→Matcher.is_match，并说明 walker 角色。残余 B633：用户只问调用链，Analyzer 却把 diagram_hint.required 设为 true，系统无独立展示权限仍硬要求图。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case audit

- 两案最终结论均由模型给出；系统未替换摘要、未补写调用边、未扫描终稿词面。
- B632 在生产回放中把双有序端点送入 exact/no-directed-path 协议，时序案不再把题设方向当成既成事实。
- B633 是展示权限问题，不是 Rust 抽取器或模型结论问题：普通关系问题可获得可选图建议，但不得仅凭 Analyzer 自报 required 形成硬成文合同。
- B634 是 identity canonicalization 问题：`gate.RunWith` 与同一包上下文中的 `RunWith` 应复用一个节点；修复不得增删任何 call edge。
- 198s/201s 期间流持续活跃，均正常完成；没有固定四分钟年龄降级，也没有旧稿/空答案替代。
