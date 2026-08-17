# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T10:38:55Z
- sweep_start_ts: 20260817-033854
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260817-033855 | write_apply,answer_regex | none | 132s | 25 | read=4,repo_map=2,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | 生产补丁与 falsy-value 测试均正确；`make check` 只提供 Python source-shape 校验，当前环境无 Node 行为执行，因此 deterministic verdict 诚实保持 `unverified:production_verification_source_static_only`。不是代码失败，不能为 runner 变绿降低验证杆。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-033855 | answer_regex,answer_contains,mermaid_edge_count | none | 244s | 35 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=2,unavail=0,prune=0 | partial | B963 生产闭环：Analyzer 的无关字段重发保留 required diagram/六参与者。首稿未证多边被正确拒绝，终图只保留三条 stage precedence，并对 Mutable/BusContext 显式 unproven；但正文仍越过同一 typed boundary 宣称全阶段读写/传递，且边标签 `precedence` 偏内部术语。上下文已精准，不能用正文关键词门或系统改写处理。补采还暴露 B966：缺失参与者导航被一个普遍 `BusContext` 参数误导到 finalizer 内部辅助调用，未优先读 extractor→carrier 的真实交接点。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B963-REQUIREDPRESENTATIONREPAIRLOSS1=production-closed-r609`：修复无关字段后，required diagram、关系逐字范围与 participant roster 均保留。
- `B964-RELATIONSPINEFALSEGREEN1=production-positive-r609/visual-honest`：未证边没有进入终图，Mutable/BusContext 以可见断开边界呈现；正文服从仍为模型波动/上下文使用观察项，不以 prose gate 或系统代写闭环。
- `B966-TYPEDCARRIERNAVIGATIONOWNER1=confirmed`：精确补读只按“某处完整传入 carrier 参数”排序，可能把全仓通用上下文参数导航到无关组件；最优修复是优先 parser-owned caller/callee 同时命中另一缺失 participant 的候选。该信号只排序软导航，不生成 evidence、关系边或答案结论。
- 本轮没有 Trace case；既有显式窗因果投影、自动补齐、typed on-chain 主因与背景隔离合同未改。活跃流也没有 4ms/固定累计年龄降级。
