# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T07:08:34Z
- sweep_start_ts: 20260809-000833
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260809-000834 | log_regex,answer_regex | none | 49s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终输出严格为 `{"ids":["u1","u3"]}`，顺序和值均与输入一致；没有 JSON 解码、结构恢复、字符串抢救或降级成文。唯一 repair 是首轮 action 未消费 required material `instructions.md`，typed material-coverage gate 精确拒绝后，第二轮显式读取该文件并闭环。系统没有从模型原文推断/补写 JSON 内容。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260809-000834 | answer_regex,answer_contains | none | 323s | 42 | read=16,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=2/1,fin_reject=2,unavail=0,prune=2 | fail | B421 的不可满足 assignment→call 双合同未再出现；本轮两次拒绝分别来自模型首稿虚构 stage call/return，以及首个 patch 新增无证 self-call/重复 call，均为真实证据边界而非合同冲突。第三稿结构通过、表头和四行属性明显改善，但摘要仍错误声称四个 stage 都由 `runReadSchedulerLoop`/`executeStageRequest` 调度；源码中 Analyze 在 task phase 之前由 `runAnalyzePhase` 独立执行。最终 required sequence 图只显示 `runTaskGraph -> runReadSchedulerLoop -> executeStageRequest` 两条已证调用，用 Note 代替四 stage 的关系，未真正回答 analyze→finalizer 的完整时序。根因是 flow completion 只要求任意一条 operation carrier，逐成员 note 也没有逐属性/执行边界 typed 证明；记 B422。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
