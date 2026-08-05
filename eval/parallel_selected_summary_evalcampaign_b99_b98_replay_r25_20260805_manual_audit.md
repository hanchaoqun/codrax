# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T09:11:50Z
- sweep_start_ts: 20260805-021148
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260805-021150 | answer_regex,answer_contains | none | 221s | 23 | read=7,repo_map=3,list=0,trace=0,source_lens=3 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 3/5/30 production roster、逐成员引用和变量排除正确，B98-A 生效；B98-B 的五步 recipe 让首稿一次 patch 即通过。但答案把没有 `iota` 的普通 `const` block 反复称为 `iota block`。根因不是 row lens，而是 analyzer 在源码验证前把猜测 `iota` 放入 keywords，后续 explorer 又把该词写进 evidence summary/member_set；旧 oracle 未钉范围说明真实性，形成 runner 假绿。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260805-021150 | answer_regex,answer_contains | none | 284s | 27 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=5/1,fin_reject=1,unavail=0,prune=0 | fail | B98-C 生效：Explorer 补发 `gate.Run -> RunWith`，最终正确判为 `buildAnalysisIR` 与 `gate.Run` 在 `RunWith` 并行汇聚、无 source→requested-sink 路径。首稿因 endpoint item 缺失及一个 Mermaid alias 写错而一次 patch；patch 插入两条 definition row 后，模型把 inherited `citation_ref` 当行序整体平移，14 条原有 edge item 绑定到相邻源码行，现有 typed repair 只纠正 3 条，最终错误通过。旧 oracle 只看名称/图形，未核 citation binding，形成 runner 假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

两个 runner PASS 均判为 human FAIL，但性质不同：

- inventory 的成员/数量闭环正确，错误来自未验证 analyzer 关键词污染后续 typed handoff；`EVAL-B98-SCOPEAGGREGATE1` 与 `EVAL-B98-REPAIRSHAPE1` 可按生产正证关闭。
- sequence 的 topology 结论正确，说明 `EVAL-B98-ENDPOINTTOPOLOGY1` 的 soft guidance 获得生产正证；失败来自 answer patch 的稳定条目引用索引漂移，与调用图判定无关。

新立案并于同批施工：`EVAL-B99-INVENTORYCONTEXTHYGIENE1`、`EVAL-B99-PATCHCITATIONDRIFT1`。前者只重建机械 source-inventory 的 soft keyword context；后者只在稳定 block/item ID、除引用外结构完全相同且新引用精确等于旧引用加行位移时恢复 inherited citation。均不扫描 request/think/final prose 作 hard gate、不修改模型结论、不进入 RootCauseTrace 或显式时间窗 Trace 权限面。
