# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T10:22:48Z
- sweep_start_ts: 20260802-032246
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_blocked_reason_chain | PASS | eval/results/trace_query_blocked_reason_chain-20260802-032248 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 159s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | worker 的 120ms io_wait/caller 数值正确，但模型把睡眠说成持续占 CPU、再把无依赖边的共现升级成 doFrame 根因；runner 的宽词面 oracle 未覆盖因果权限。 |
| 1 | read_combo_analyze_retry_anchor | FAIL | eval/results/read_combo_analyze_retry_anchor-20260802-032248 | answer_regex,answer_contains | none | 186s | 27 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 新 empty-IR join 已被正确读到，但答案仍把 missing emit 写成 nil output、把 auto-correction 写成不消耗 attempt，并漏掉 runTaskGraph 的真实阶段边界。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

### trace_query_blocked_reason_chain

- 确定性事实正确：worker-30 在 2.030–2.150s 为 `io_wait`，120ms，
  `sched_blocked_reason` 的 caller 为 `fscache_page_wait_on_page_bit`。
- `wakeup_chain(pid=30)` 没有 wakeup edge；投影也明确显示
  `chain_shape=flat_untraceable / 链不可上溯`。因此只证明 worker 自身等待，
  不证明 worker 导致 main-20 的 doFrame 变慢。
- 模型却声称 worker 在 io_wait 时“持续占用 CPU”，并据此让 main-20 等待。
  这是调度语义错误：S/D/io_wait 期间线程不在 CPU 上。模型还把 `R+` 切出
  直接称为主动 yield、把无事件支撑的 `waker-10` 称为唤醒者。
- fixture 自身注释也把共现预写成“frame stalls because worker”，属于过硬
  ground truth；自动 oracle 只要求 blocked thread/state/caller，并未要求这条跨主体
  因果，注释与 oracle 矛盾。已修正注释，不新增正文关键词禁止门。
- 显式窗 Trace 因果投影与自动补齐仍在，证明本批不会通过取消投影规避错误。
  最优方案是给模型 scheduler residency 与跨主体 connector 的通用软权威，正文
  仍由模型负责。

### read_combo_analyze_retry_anchor replay

- `analyzeStageOutputUsable` 已进入探索证据并被最终答案正确写成：clean
  `StageOutput`、非 nil IR、TaskGraph 非空四项合取。B39 `ANZERO1` 代码修复生效。
- 但答案把“未调用 emit_analysis”写成 nil `StageOutput`。生产
  `analyzerEvaluator.ParseOutput` 实际返回非 nil `StageOutput`，并在 `Error` 中
  写入 `emit_analysis was not called`；这是用户要求的 `StageOutput/Error` 关系核心。
- `attempt` 在 auto-correction 之前已经递增；修正可避免再 dispatch，但本次
  semantic attempt 已消费。答案的“不消耗重试预算”不准确。
- `dynamicAnalyzeRetries` 的增量是 `(estimated/2) *
  SubTopicRetryBudgetExtra`，不是恒定“每两个子话题加一次”；`max` 是总 semantic
  attempts，transport retry 使用独立预算。
- analyze 在外层 `Run` 中先完成，之后 `runTaskPhase` 才调用 `runTaskGraph`；
  missing-emit/quality exhaustion 的 degraded IR 也由 `Run` 安装。最终主文没有
  `runTaskGraph`，runner 因而 FAIL。该轮说明代码 join 已闭环，但模型没有读取已被
  source localizer 提供的 `analyzer.go` producer；按模型所有权残余记录，不为一个
  case 加答案替换或关键词硬门。

## GAP Decision

- `EVAL-B40-TRACESEM1/P1`：perf pre-triage/final synthesis 缺少一句不可混淆的
  scheduler residency + cross-subject connector 软权威。实现必须只提供准确指导，
  不扫描答案、不拒绝成文、不系统改写结论。
- `EVAL-B40-EVAL1/P1`：blocked-reason fixture 的叙事 ground truth 错把共现写成
  因果；修正注释并保留原 typed 事实 oracle，禁止把错误预期做成更硬 guard。
- `EVAL-B40-READMODEL1/P2-watch`：代码修复已生效，producer/attempt/phase 边界仍被
  模型误读。先按 model-owned evidence selection 观察其它 read mechanics case；
  不为此题面注入固定答案。
