# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T21:13:28Z
- sweep_start_ts: 20260806-141326
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-141328 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 141s | 30 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、一次 bundle 查询、双轴主要占用/现规则可消量、根因排序、wakeup chain、Trace 因果投影和自动补采均完整；frame evidence=absent/causal=unproven 也披露。模型正文却把 priority_inversion_candidate 延伸成“低优先级依赖导致阻塞”，并建议减少“持有锁的时间”；同页 typed caveat 明确 direct blocker 与锁持有关系未证，构成模型结论与输入证据边界冲突。系统没有改写正文，正确保留红线。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260806-141328 | answer_regex,answer_contains | none | 159s | 24 | read=13,repo_map=4,list=0,trace=0,source_lens=2 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | S26 生产闭环：analyzer 未再把“主要”铸成 public/exported，12 个 production implementer 全部进入 typed member_set、表格与同向 type_relation 图；零 empty-set 重试、零 finalizer reject。模型仍把 blocks[] 二次编码成字符串，但既有 flat-mode 路径完整无损恢复，答案未丢失且零重试；当前归为已安全吸收的模型格式波动，继续观察 JSON 教学心智负担。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `EVAL-B194-MEMBERCARRIER1` 已由生产回放关闭；类型关系图层 S24/S25/S26 合同在本轮零重试通过。
- Trace 回放证明源码 type-relation/typed exclusion 修复没有侵入 runtime causal lane。
- 新确认 `EVAL-B195-PIAUTH1=P1/model-context-adherence`：候选权限和“direct blocker/锁关系未证”虽已进入同一 Finalizer 上下文，但模型仍在正文越过该边界。不得用系统正文替换器修正；后续应降低 prompt 冲突/距离并让候选 caliber 在模型作答所消费的 principal carrier 上就近出现。
- JSON 教学没有发现“同一字段必带又必拒”的系统矛盾。本轮仍发生 `blocks[]` 字符串化，但 lossless recovery 完整恢复、零答案损失和零 retry；暂记模型波动/提示负担观察项，不升级为硬门。
