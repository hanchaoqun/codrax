# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T00:09:34Z
- sweep_start_ts: 20260817-170932
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_config_two_knobs_precedence | PASS | eval/results/read_combo_config_two_knobs_precedence-20260817-170934 | answer_regex,answer_contains | none | 158s | 30 | read=12,repo_map=0,list=1,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 两个键都给出 YAML 字段、CLI flag、真实代码默认值 50/3 及覆盖条件；Explorer 读取 `cmd/root.go:3147`。成文拒绝/patch 从 r651 的 14/13 降为 0/0，引用无重复膨胀。探索期仍有 evidence-anchor 修补往返，但未污染答案。 |
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-170934 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 用户窗、自动补采、worker-200 链上 #1、8.300ms 可消量及背景压力边界均正确。与 r651 对比确认 B1030 并非已消失：本轮三个模型查询均为精确窗，而 r651 的 10.020ms 宽窗同事实抢占了后续精确补采行的 rank-window 身份。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- `B1027-CONFIGVALUEMATRIXPREDICATE1`、`B1028-ROWIDPATCHAUTHORITY1`、`B1029-PATCHCITATIONIDEMPOTENCE1` 获生产正证：正确值、逐键覆盖、零成文拒绝、零补丁和无重复引用同时成立；配置 case 用时从 987s 降到 158s。
- `B1030-TRACECHAINROOTCLASSIFICATION1` 根因收窄为 typed 窗载体的先来顺序污染。r651 先发布 `1.000000..1.010020` 的 rank 行，系统后补 `1.000000..1.010000` 精确行；R1 同事实合并保留宽窗 donor，精确行被吸收，selected-window roster 随后正确地拒绝宽窗，却因此错误得到零链上席。r652 没有宽窗查询，故自然通过，不能据此关闭。
- 最优解是在编译端只用 validated explicit-scope + typed query window，使精确请求窗的既有事实优先成为同事实 survivor。系统不新增 rank、数值、链关系或结论；若没有精确 typed carrier，保持旧序与 fail-closed 行为。
- 活跃流无固定 4ms 降级；本轮未扫描用户/模型 prose 做硬门，也未由系统改写模型正文。
