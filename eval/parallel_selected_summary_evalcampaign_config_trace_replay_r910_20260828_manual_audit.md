# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T23:06:30Z
- sweep_start_ts: 20260828-160629
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-160630 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 219s | 49 | read=0,repo_map=0,list=0,trace=16,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=1 | partial | 显式 34579.472865..34579.587805 窗、自动补采、四节点唤醒链、链上根因排序、实际占时/规则可消双账户、业务 span、背景隔离与完整 Trace 因果投影均在。模型却把已明确重叠且不可直加的 23.994ms 与 19.041ms 相加成 43.035ms，并把 wakeup 时序写成“等待下游完成工作后发出唤醒”、把 fscache 调用点扩写成已知页面读完成机理；同轮 typed prompt 已精确声明 cross-seat addition forbidden、work-completion/direct-blocking/backend authority 均未提供，故属于既有 B407 与 B1269/B1271 的模型遵循复现，不以终稿扫描做硬门，也不由系统改写。一次 finalizer reject 为畸形 caveat JSON，下一轮模型自行修正。 |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-160630 | answer_regex,answer_contains | none | 243s | 29 | read=22,repo_map=1,list=0,trace=0,source_lens=1 | midloop=10,inv=8/0,fin_reject=0,unavail=0,prune=0 | partial | B1417 获生产正证：finalizer accepted_evidence_handoff 只剩 grounded=10，旧的 unspecified runtime.go:1702 候选及错误引用均消失。最终答案正确给出默认 50、YAML 字段、CLI Changed 守卫和相关源码，但把“未显式 CLI 时 flagMaxSteps 仍为 0”写错（实际 cmd/root.go:2689-2690 直接赋 mergedMaxSteps），且表内优先级把 code constant 放在 YAML 之上，与同答正文 code→YAML→CLI 自相矛盾。explorer/finalizer 上下文已经准确，按模型推理波动留观，不新增自由正文硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
