# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T04:21:31Z
- sweep_start_ts: 20260813-212127
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-212131 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 140s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B763 的 representative 口径已进入 prompt，模型也识别 CPU12/CPU4 两个 bucket，但终稿仍把全 CPU running=157.248ms 写成“在 CPU12 上”，并把 off-CPU runnable=5.604ms 加成“有效 CPU 占用”162.852ms；CPU4 又从正文频率结论中丢失。确认 B764：running/runnable 的 typed scope 仍不够自解释。 |
| 2 | github_issue_chrono_duration_min | PASS | eval/results/github_issue_chrono_duration_min-20260813-212131 | write_apply,write_patch_oracle | none | 179s | 24 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 自动 PASS 被人工推翻。补丁把 `try_milliseconds` 放在 `impl Duration` 外，Rust 不可编译；唯一 `make check` 是 Python 源码形状 oracle。报告已精确标注两条 Rust 路径 `capability=source_static`，模型也识别弱证明并请求 verify_batch，但 transition recovery 把请求改成 finish 后没有再次经过 typed finish authority，错误发布 `all_batches_verified`。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. `B763-THREADCPULOADREPRESENTATIVECPUCALIBER1` 获上下文正证：`cpu_scope=dominant_state_slice_representative_not_exclusive` 到达 Finalizer，探索阶段明确列出目标 CPU12 96.081ms 与 CPU4 35.960ms。它解决了“代表 CPU=唯一 CPU”的直接误导，但没有关闭值域口径。
2. 新确认 `B764-THREADCPUSTATECALIBER1/P1`：`thread_cpu_load.running_ms` 是全窗口、跨 CPU 的线程 running 总量；`runnable_ms` 是 off-CPU 调度等待。当前 wire 没有分别携带 `running_scope` 与 `runnable_scope`，模型把 157.248ms 绑定 CPU12，又把 runnable 加入 CPU occupancy。最优修复是 typed 字段口径 + 同源软教学，不扫描或改写答案。
3. 新确认 `B765-WRITETERMINALAUTHORITYBYPASS1/P0`：普通模型 finish 已有 source-static 降级门，但 `enforceControllerWorkflowTransition` 生成的 recovery finish、budget/direct-completion 两条终止捷径可绕过该门。r468 的精确链为：typed report=`source_static` → 模型请求动态 verify → `workflow_already_complete` 恢复成 finish → 直接签 `verified`。这是系统裁决层假绿，不是模型波动。
4. B765 按“所有系统铸造终止决定也必须经过同一 typed authority”根修：transition recovery 重新进入 normalizer；budget shortcut 先过 finish gate；applied-pending terminal verify 在无后续 controller turn 时直接把 durable attempt/batch/run 标为 unverified。静态检查的 `passed` 工件保留，不把弱证明改成代码失败。
5. 本批没有触及 Read/Trace 因果投影、显式窗、自动补齐、链上-only 主因、实际占用/可消除量双轴或模型答案所有权；没有 raw request/model/final prose 关键词门。活跃字节流也没有 4ms 累计年龄降级。
