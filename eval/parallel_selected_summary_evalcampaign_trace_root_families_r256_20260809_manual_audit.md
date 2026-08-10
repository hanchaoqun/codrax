# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T01:11:56Z
- sweep_start_ts: 20260809-181154
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260809-181156 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 115s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 根因与双轴正确：链上 #1 `VerifyClass com.example.Foo` class_verification 4.600ms、#2 app-100 runnable_wait 0.800ms；原始 span 5.000ms 与有效归因 4.600ms 未互换，frame evidence absent/unproven 被保留。正文后段正确区分 wakeup=5.005000 与 switch-in=5.005800，但事件序列仍写成“5.005800 被唤醒后切换入运行”，同页残留一次 wakeup/switch-in 合词，B439 判 production-partial，不以 prose hard gate 修。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260809-181156 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 123s | 42 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、系统补采与所有重要 typed 链上家族均保留：PI #1 23.994ms=#runnable 23.748+#running 折算0.246，PI #2 19.041ms=18.876+0.165，D/IO 10.433/7.386/6.673/3.550ms，目标 running 算力供给缺口10.331ms，VerifyClass 0.285ms；背景未加冕，frame causality unproven。三项上下文 GAP：摘要把两项 PI 压成仅 runnable，漏 running 供给分量；typed pressure 无绝对标尺仍称“较高水位”；全窗 wakeup census=36 与链关联 sleep 聚合实例=12 被正文写成“12次唤醒”。确认 B440/B441/B442，均只修 typed 软上下文。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- 两例均首次成文、`finalizer_reject=0`，没有“成文校验未通过”重试；runner PASS 不等于人工全绿。
- `EVAL-B439-TRACEWAKEVSRUN1`：production partial。高显著度根因段已正确区分 5.005000 wakeup 与 5.005800 switch-in，但事件序列仍有一次合词；继续用 typed 时间线软教学观察，禁止扫描正文硬拒或系统改写。
- `EVAL-B440-TRACEPIRUNSUPPLYCONTEXT1=P1`：优先级反转的 compact root-family 教学遗漏 typed running compute-supply deficit，导致摘要压掉已存在的折算分量。
- `EVAL-B441-TRACECONTEXTABSLEVELNEUTRAL1=P1`：`absolute_level_authority=not_provided` 已准确，却缺少中性可复制表述，模型重复铸造“较高/中等”等绝对档位。
- `EVAL-B442-TRACECOUNTCLARITY1=P1`：wakeup census、causal seat occurrences/merged_count 是不同 typed 计数口径，当前高显著度上下文未并列说明。
- 三项最优方案均为 typed prompt-only：补全候选家族语义、提供无标尺中性话术、明确计数口径；不读取用户或模型原文，不新增答案 hard gate，不改变投影/排序/数值，也不代替模型结论。
