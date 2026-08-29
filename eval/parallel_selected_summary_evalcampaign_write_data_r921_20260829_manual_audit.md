# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T04:06:34Z
- sweep_start_ts: 20260828-210632
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260828-210634 | log_regex,answer_regex | none | 138s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格只输出 `17,0,5`；四份材料全覆盖，active 过滤、标签映射、3 条贡献记录、差值 0 对账和 targets 顺序投影均闭合。初始模型一次计划四个有依赖层级的动作，确定性执行器安全拆成 8 批完成；属于安全吸收后的过程观察，不据单例新增硬门。 |
| 1 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260828-210634 | write_apply,write_patch_oracle | none | 150s | 27 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (implementation) / honest-unverified (runtime) | 正确仅修改生产 Java 文件，按 `value` 实现 equals/hashCode，未改测试；Make source-static 检查通过。宿主缺 Java，行为测试诚实保持 `runner_missing/unverified`，runner FAIL 符合 `allow_unverified=false`。新 B1436：初始队列已有 Java direct-main，Make 后的能力升级仍重复追加同一候选，导致同一 `javac` 缺失命令执行并披露两次。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- Data：人工 PASS；没有把内部 ledger 词面泄漏到严格最终输出。
- Write：实现人工 PASS，运行时只能诚实判未验证；不得把 source-static 检查提升为 Java 行为验证，也不得通过放宽 eval 的 `allow_unverified` 掩盖宿主缺 JDK。
- `B1436-TESTSURFACEQUEUEIDENTITY1`：同一次 `run_tests` 的升级器只屏蔽已执行候选，没有屏蔽初始队列中尚未执行的同身份候选。它是跨 runner 的通用队列幂等缺口，不是 Java case 拟合；修复应以 `(runner, framework, working_dir)` typed identity 统一表示“已执行或已排队”。
