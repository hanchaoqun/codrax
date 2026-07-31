# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T09:30:47Z
- sweep_start_ts: 20260731-023047
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_boundary | FAIL | eval/results/read_combo_log_current_code_boundary-20260731-023047 | log_attachment,answer_regex | log_triage | 97s | 16 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 路由器已给出 `needs_repo=true`，但 analyzer 把“日志行不是当前源码引用”与“请结合当前源码”同时塞进 source exclusion quotes，并令 `current_source_mode=exclude`；源码 lane 被静默关闭。正文正确解释 timeout 观察，却没有读取或引用当前实现，不能回答“结合当前源码”的机制部分。 |
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-023047 | log_attachment | log_triage | 286s | 21 | read=10,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass_with_efficiency_gap | 运行时结论正确：栈顶为 `main.crashy()` / `internal/agent/analyzer.go:100`，并诚实披露与当前 checkout 不一致。两段式日志预处理实际调用 `emit_log_triage` 两次，但预处理为估算 byte segment 消耗多轮，随后又做 8 轮源码探索、10 次读取，总耗时 286s；属于附件定位与可选 current-source 验证未分层的 P2 效率债。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- 自动：1 PASS / 1 FAIL；人工：1 pass-with-efficiency-gap / 1 FAIL。
- correctness 优先项不是中文短语识别，而是 `external_observation_policy` 把“外部行号的引用身份”和“是否允许当前源码证据”共用一个 `source_quotes[]` 载体。一次错铸即可让有效的 current-source obligation 整体消失。
- 修复必须在 analyzer wire schema 上拆开 exclusion proof 与 artifact-citation proof，并继续只消费 typed 字段；不得在下游扫描用户原文或模型答案来猜“不要”的作用域。
- oversized 的答案事实足够，当前仅登记 P2。后续应基于 typed artifact/source obligation 决定是否进入 current-source 验证，并让预处理使用确定性 byte/line 索引；不能按 40KB、panic、Go 或固定工具次数特判。
