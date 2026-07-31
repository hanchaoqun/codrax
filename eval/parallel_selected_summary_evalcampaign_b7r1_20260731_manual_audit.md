# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T16:56:16Z
- sweep_start_ts: 20260731-095615
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260731-095616 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 210s | 29 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 根因、链顺序、优先级非反转结论和显式窗因果投影正确；但主答案把前两条 wakeup 事件从 typed 2.016/2.018s 错写成 2.015/2.017s，把累计 latency 混称边传导延迟，并把窗内 20.000ms 与窗外恢复点形成的 20.020ms 混写。投影还把同一 threadpool I/O 段同时列为 E4 causal impact 与 E5 depth-unresolved root_evidence。 |
| 2 | github_issue_dayjs_duration_nan_symptom | PASS | eval/results/github_issue_dayjs_duration_nan_symptom-20260731-095616 | write_apply,answer_regex | none | 311s | 18 | read=11,repo_map=3,list=0,trace=0,source_lens=1 | midloop=2,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 一行 `Number(value)→Number(value ?? 0)` 修复正确且保留完整 duration；Python `make check` 能区分基线失败/修后通过。控制面仍有 GAP：两个 JS 行为 probe 与 npm 都 unavailable，证明补强批重复同一验证后却发布 all_verified。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

### trace_query_wakeup_causal_io_chain

- typed 引擎真值完整：链为 `threadpool-400 → network-300 → cookie-200 → app-100`；三条 `wakeup_chain_edge.wakeup_ts` 分别为 2.016、2.018、2.020s；threadpool 的 IO 段为 2.003–2.014s、11.000ms，caller 为 `fscache_page_wait_on_page_bit`。
- 模型主答案的根因排序和链方向正确，也明确拒绝把“低优先级依赖”直接说成已测量的优先级反转。
- 主答案的前两条 Mermaid 边时间各提前 1ms；“12/2/3ms 传导延迟”混合了 D 段起点、sched-in 和 wakeup 事件，并非同一 edge latency 口径。自动 oracle 只检查实体、顺序和根因词面，故 false PASS。
- 显式时间窗的 `Trace 因果投影`、目标四态、覆盖边界和自动 window_stats 补齐均在；本轮证明前一批对状态查询的收窄没有伤到显式窗因果能力。
- 同一 threadpool IO 物理段在确定性投影中出现两席：E4 是有 depth 的 `wakeup_causal_impact`，E5 是无 depth 的 `root_evidence:io_wait`。两者同 subject/state/value，但 source span 一宽一窄，现有同线范围 R1 未吸收。

### github_issue_dayjs_duration_nan_symptom

- 实际 diff 只有 `src/duration.js` 一行，`tests/duration.test.js` 未被无谓改写；修复对缺字段用 nullish default，不影响已存在的 0 或完整 duration。
- fixture 的 `python3 tests/check_duration.py` 在基线明确失败，在 applied tree 通过，说明补丁满足该 eval 的静态行为 oracle。
- Node 与 npm 不可用，因此两个直接执行 `parseIso/formatDuration` 的行为 probe 均为 `verification_probe_runner_missing`。首轮报告虽然 `Passed=true`，proof review 仍准确留下 3 个 behavior-contract 和 2 个 changed-symbol 未覆盖义务。
- 系统追加 verify-only proof follow-up 后没有获得新证据，第二轮依然 unavailable；旧调度却把该批记为 `batch_verified` 并允许 `all_verified`。这是 typed proof ledger 已知未闭合却被 report-level pass 覆盖的系统矛盾。

## GAP Decision

- `EVAL-B7-T1/P1`：精确 `wakeup_chain_edge` 在 trace_query payload 已存在，但没有稳定成为主答案边时间的 authority；需从 typed edge 进入最终 handoff/确定性边展示解决一类链时间问题，禁止用正文关键词硬审或为本 case 特判。
- `EVAL-B7-T2/P1`：同一 D/IO 物理段跨 `wakeup_causal_impact` 与 `root_evidence` 双席；后续应以 typed subject/state/value/artifact/window/interval containment 建单一吸收点，保留证据但只显示一席。
- `EVAL-B7-T3/P2`：模型把查询窗投影 20.000ms 与窗外 20µs 后的实际恢复点混写；先观察真实 trace 复现，不为一次模型数值波动加题面/正文硬门。
- `EVAL-B7-W1/P1`：proof-only follow-up 必须由 typed proof ledger 判闭合；重复弱验证只能完成为 unverified，不能升级 all_verified。批 R 已施工。
- `EVAL-B7-W2/P2`：verify-only proof follow-up 当前只能复跑旧 plan，稳定 runner-missing 环境下通常无新增证据；先在 W1 诚实降级后观察成本，再决定是否按 typed capability/no-delta 直接跳过重复验证。
