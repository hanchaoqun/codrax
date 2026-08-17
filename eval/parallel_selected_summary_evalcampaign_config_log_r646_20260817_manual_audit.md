# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T22:10:58Z
- sweep_start_ts: 20260817-151057
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | logtri_goroutine_dump | PASS | eval/results/logtri_goroutine_dump-20260817-151058 | log_attachment,answer_regex | log_triage | 100s | 24 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 正确列出 goroutine 15/87/120 与共同的 `main.writeSession` / `fatal error: concurrent map writes`；当前仓无法映射附件栈帧时明确保留外部日志边界，没有把当前源码或系统附录伪装成根因。 |
| 1 | read_combo_config_two_knobs_precedence | FAIL | eval/results/read_combo_config_two_knobs_precedence-20260817-151058 | answer_regex,answer_contains | none | 228s | 30 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=3/1,fin_reject=0,unavail=0,prune=0 | fail | 虽已正确区分 sample config 与 CLI `0` inherit sentinel，仍把无初始化器的 `internal/types/config.go:21` 字段声明提升成“代码默认值 2”；真实生产基线是 `cmd/root.go:3147` 的 `MaxRetriesPerStage: 3`。关键 production-initializer grep 因非法正则失败，系统随后仍发 close-ready，aggregate member note 又可携带该行没有证明的数值。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

1. `logtri_goroutine_dump` 是真实通过：附件证据足以回答，且系统没有要求无意义的当前仓源码补证。
2. 配置案例不是随机措辞波动。失败链为：关键 `grep` 执行失败 → 系统未区分“调用失败”和“成功零命中” → generic close-ready 提前收口 → `member_set.member_notes` 把身份锚扩写成无支撑数值 → Finalizer 复述错误默认值。
3. 高 ROI 根修不扫描用户或模型正文：只消费 `ToolResult{ToolName:"grep", Success:false}` 发一次软恢复；配置 family 则要求无 initializer 的字段声明只证明 identity/type，数值默认值用既有 `aggregate_facts(kind="scalar_value", dimensions.layer="code_default")` 加 production value-bearing `support_ref` 交接。
4. 另有一个非阻塞观察：Analyzer 明确保留两个 `exact_targets` 与三个 `exact_context_roles`，但本轮 `trace/sv` 显示 `exact_resolution_present=false`。`BuildExactResolutionContract` 的多 key 单元构造为正，因此需继续追查生产 IR/semantic-view 接线；它不是本轮错误默认值的直接原因，不能在未定位前硬扩合同。
5. 本轮未修改 Trace、写模式或答案正文。显式时间窗、Trace 因果投影、系统补齐、链上-only 根因以及活跃字节流无固定 4ms 降级均保持。
