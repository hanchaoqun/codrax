# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T00:27:18Z
- sweep_start_ts: 20260813-172717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-172718 | answer_regex,answer_contains,mermaid_edge_count | none | 218s | 36 | read=9,repo_map=4,list=0,trace=0,source_lens=2 | midloop=9,inv=2/0,fin_reject=4,unavail=0,prune=0 | partial | B752 的 exact-node receipt 没有触发：模型改用业务 node id。三条阶段顺序和一条真实 call 最终保留，但 data-flow 因 Mermaid unsafe-id 改写为 `codraxNode1` 后与 anchor/receipt 表示不一致而被删。四次拒绝还含一次 replace patch 漏 `kind`。自动 PASS 的最小边数没有识别关系缩水。 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-172718 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 292s | 53 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=1 | partial | 正文完整给出 157.248/5.604/70.338/0ms、CPU4=2100MHz direct policy limit 与 target binding 未证；FAIL 仅因 oracle 不接受表格里的 `d_state + io_wait（D 状态）`。但 Analyzer 连续两次仍发 `bounded_fact_set`，且正文混入“#1 根因席位”等内部词并把策略环境、算力折算与目标绑定写得偏拥挤，B753 从观察升级为持续 schema 心智问题。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `B754-DIAGRAMTYPEDRECEIPTTOPOLOGY1`: exact recipe identity 不能以“模型必须照抄 n#”为前提。最优修复只用完整连通分量的 node topology、direction、relation_kind 与 typed receipt 做唯一图同构；不读取可见 label，不造边，不改答案。
- `B754-PATCHKINDINHERIT1`: `replace_blocks` 已以 exact previous block id 指定载体；漏写 kind 时精确继承旧 enum 是无损 JSON 自愈。unknown replacement、add block、显式非法 kind 仍 fail-closed。
- `B753-RUNTIMEBOUNDEDEFFECTPRODUCTIONCLASSIFICATION1`: 第二次生产复放仍把被动 target-effect 问法折成 observed fact，已排除单轮随机波动。先收紧 schema/skill 的通用被动约束判定教学；不得扫描用户/答案关键词，也不得由系统替模型选择 yes/no/unproven。
- `active-stream-4ms-degrade`: 两案均无该降级。只要连接仍交付字节，4ms 内没有完整 answer 不构成失败；byte liveness 继续刷新，只有 caller deadline/cancel、首字节超时、byte-stall、transport/decode failure 可结束或进入披露式恢复。
