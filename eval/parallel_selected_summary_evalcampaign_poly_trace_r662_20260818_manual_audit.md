# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T04:08:12Z
- sweep_start_ts: 20260817-210809
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260817-210812 | answer_regex | none | 116s | 26 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B1041 enum 归一生产触发，但完整 source 仍被 provenance 候选集漏掉：Analyzer entities 拆成 `FastTokenizer`/`tokenize`，归一后的 `FastTokenizer.tokenize` 因不在 exact_targets/entities 又降回 discover_path。最终答案列出 Python→native→PyO3 wrapper→core→best_merge，patch 提交入口 call、guard、wrapper→core、core→helper 等 anchors，较 r661 更完整；但 `_fastlex.tokenize_bytes -> py.tokenize_bytes` 仍无 typed bridge，fallback 明明已读却写“需进一步查找”，故不能仅凭 PASS 收账。 |
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-210812 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 213s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-model-caveat | 显式 1.000..1.010 窗、五态、跨 CPU 唤醒、worker-200 链上首席 8.300ms、实际占时/现规则可消双轴、完整 Trace 因果投影、邻近/背景权限均保留。模型前置 prose 把 `wakee_target_cpu=1` 误述为切到 CPU2，并把“优先级反转候选”说成实测确认；系统投影和证据索引仍保持正确候选口径及 CPU 字段。上下文已有 waker/target CPU 和 candidate 边界，暂归模型服从/算术表述观察，不由系统改写模型结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

1. Runner `2/2 PASS`，人工为 Poly partial、Trace pass-with-model-caveat；活跃流均等到完整结果，无固定 4ms
   或累计年龄降级。
2. `B1041` 第一层生产生效但未闭环。日志同时出现“discover_path→discover_terminal”和“source 非 exact
   current-request identity→discover_path”，证明 gap 在 provenance candidate wiring，不在 enum repair。
3. 第二层根修把 schema 已选的 source/sink 本身送入既有 `MentionedEntitiesFromRawRequest` 精确身份边界校验。
   只有完整 typed identity 逐字存在于当前请求才进入 provenance roster；`FastTokenizer` + `tokenize` 的拆分
   entity 不再导致完整 source 丢失。它不扫描答案/推理，不按关键词分类；预扫描或虚构 identity 不在请求中仍
   fail-closed。
4. Poly 的原生导出桥仍须在 endpoint lane 真正保留后回放判断；本批不按可见 prose 数箭头，不把 registration
   当 call，不由系统补边或代画图。
5. Trace 对照证明显式窗因果路径未受影响；同时冻结模型两处越界为 B999 观察。系统已给出精确 CPU1 target 与
   candidate 权限，故禁止为该单例新增答案关键词门或 renderer 改写。
