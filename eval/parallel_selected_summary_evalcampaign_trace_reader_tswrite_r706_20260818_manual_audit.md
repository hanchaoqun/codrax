# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T22:11:53Z
- sweep_start_ts: 20260818-151151
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_napi_force_wasi_env_symptom | FAIL | eval/results/github_issue_napi_force_wasi_env_symptom-20260818-151153 | write_apply,answer_regex | none | 182s | 25 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | 生产补丁精确且 `make check` 通过；本机没有 Node，JavaScript 行为探针为 runner_missing，故 final verdict 诚实保持 unverified，不把 source_static 冒充行为验证。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260818-151153 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 225s | 44 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass-with-caveat | B1110b/B1111 获生产正证；显式窗、链上根因、双轴和因果投影完整。系统覆盖边界仍泄漏枚举权限 raw key/value，已作为 B1113 在本批统一根修，待下一 Trace 回放取生产正证。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Case 1 — real_trace_h7_self_seat_full_spectrum

- 人工结论：`pass-with-caveat`。模型正文首先给出 233.190ms 显式窗口的五态账，并把自身 running
  65.912ms 供给折算、D-state 36.757ms、链上小额依赖及链外背景分开；`Trace 因果投影`、自动补采、
  主要时间占用/现规则可消除量双轴和业务 span 线索均保留。邻近 runnable 压力明确不参与主因序数。
- B1111 获正证：状态总账 sleep=118.586ms；主要占用表的 sleep=78.630ms 明确附带“本表所列相关片段
  的累计，非该状态全窗合计；全窗值见上方状态分区”，不再把链上片段误作全窗状态。
- B1110b 获正证：Finalizer 的排名直接交接使用“不可中断等待/IO延迟/调度延迟/优先级反转候选”等
  读者标签；模型正文不再复制 `d_state_or_io_wait/io_latency/runnable_wait/priority_inversion_candidate`
  等 wire cause token。`bounded_window_candidate` 只存在于隐藏 JSON 字段/诊断日志，没有进入客户正文，
  也再次验证 B967 的软教学修复稳定。
- 新确认 B1113：系统自有的「Trace 因果投影覆盖边界」仍显示
  `enumeration_status=incomplete/compacted_views/boundaries/emitted/total`。这不是模型波动，而是覆盖权威
  renderer 的确定性展示债；同一函数的生命周期、目标身份和帧证据分支也仍有 raw key/value 潜在泄漏。
  本批已在单一 renderer 上统一改为中英文读者语言，raw wire 继续保留在 typed ledger/诊断工件。
- Finalizer 0 reject，active byte stream 持续 225s 后正常完成，没有因 4ms 或固定累计年龄降级。一次
  unavailable tool attempt 未改变 Trace 权威路径或最终结论。

## Case 2 — github_issue_napi_force_wasi_env_symptom

- 人工结论：`pass-with-caveat`。工作树只修改 `cli/src/api/templates/js-binding.ts` 的条件：从任意非空
  环境变量都触发 WASI，改为仅 `'true'` 或 `'error'` 触发；`false/0/空串/undefined` 的语义与 fixture
  既有六个回归场景一致，没有改测试期望绕过问题。
- `make check` 成功，包含 Python self-test 与静态合同检查；但预先声明的 JavaScript 行为探针因本机
  无 Node 报 `runner_missing`。changed path 只有 `capability=source_static`，因此 deterministic final
  verdict 为 `impact_targets_unverified`，客户面明确写“未完全验证”。这是正确的证明边界，不是代码
  失败，也不应以 Python 静态 oracle 伪装 JavaScript 实际执行。
- Controller 一度把 report `status=passed` 误读为可选择 `all_verified`，后置 typed 终验仍正确降为
  unverified。当前已有精确 capability 边界和 deterministic 兜底；在缺少 Node 的 fixture 上继续追加
  同类探针不会产生新证据，因此本轮不立新代码 GAP、不降低验证杆。
