# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T17:37:48Z
- sweep_start_ts: 20260815-103747
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260815-103749 | write_plan,write_patch_oracle | none | 49s | 24 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确读取 `main.py:20`，只生成一个 `retrun -> return` 的 structured patch 计划；没有 apply 或扩大文件面，验收包含 import、函数返回值和其余行不变。 |
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-103749 | answer_regex,answer_contains | none | 134s | 26 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B844-v3 的压缩闭包生效：finalizer 收到 `bridge_literal_terminal`，typed source-role projection 为 `principal=1`，旧弱证据 caveat 消失，最终 `1 / explorer / register / Name()` 事实与两处引用正确。仍有 typed 合同分裂：生产 RequestModel 为 `predicate_axis=register + enumerate + category_enumeration`，但 `is_relational_lookup=false`；`RequiresRelationMemberSetHandoff` 只读后者，提示同时出现 principal relation row 与“no hard principal relation member_set”。表格一行需要两个证据位置而 item 仅有单 citation_ref，导致一次结构拒绝和一轮冗长 patch；最终通过引用池保留两处证据，但输出维度补充仍偏机械。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Conclusion

- Machine: `2/2 PASS`; human: `1 pass / 1 partial`.
- B844-v3 的通用 `DerivedFrom` compaction closure 已由生产规模回放验证：组合 bridge 与终端 return carrier 同时到达 finalizer，旧的无关弱证据 caveat 消失。该实现不扩大 160 条预算，也没有 request/model/final prose 扫描。
- 新立 B847：显式 typed 关系轴（`AxisRegister` / 同类精确关系轴）与冗余 `IsRelationalLookup` 布尔发生分裂时，关系 member-set handoff/authority 必须读同一个 schema 单源；机制说明、scalar role lookup、Trace causal lane 继续排除。
- 新立 B848：一个结构化表格行可同时展示多个独立 evidence/location 维度，但 `AnswerBlockItem` 只有单一 `citation_ref`。先审计通用多锚表示与现有 citation pool/remap 合同，不以本案例列名或答案文字加门。
- B846 本轮未出现无关 hitrace 引用，但 r517 的确定性 witness 仍有效；它与 B848 共享引用身份面，不能以一次不复现关账。
- 本批没有修改 Trace、JSON、Mermaid、Read/Write 路由或活跃流终止策略。显式时间窗 Trace 因果投影和自动补齐保持；链上-only 主因、背景 support-only、实际占用/业务线索与规则计价可消除量双轴保持；系统不替模型写结论；4ms/4s/4m 均不是活跃字节流降级信号。
