# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T23:54:06Z
- sweep_start_ts: 20260801-165404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_analyze_retry_anchor | PASS | eval/results/read_combo_analyze_retry_anchor-20260801-165406 | answer_regex,answer_contains | none | 134s | 33 | read=6,repo_map=4,list=0,trace=0,source_lens=2 | midloop=6,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Runner 只钉名词与“不静默零值”，没有检验路径/数值真相。真实默认 `MaxRetriesPerStage=3`，YAML 仅显式覆盖；`dynamicAnalyzeRetries` 接收已解析 int，不存在 nil 回退。read analyze 耗尽走 `buildDegradedSemanticIR/buildDegradedFallbackIR`，答案却拼入只属于 write_analyzer 的 `fallbackWriteAnalysisIR`。同一批还复现 B21-CALLEE1/SPAN1：定义/单行锚点的自由 summary 被支持面重新发布为 Evidence note，越权承载跨函数行为。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-165406 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 135s | 43 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B28-SHARD1 已覆盖：6 个重叠 block_rq shard 发布 `total_impact=unavailable`、`cross_shard_additivity=forbidden_overlapping_windows`，14.204 不再进入日志/答案。人工仍 FAIL：模型无视已提供的 pre-wakeup phase/authority，把低优先级 waker 写成“直接原因/持有调度依赖”，把 blocked_reason caller 写成 holder，并在 frame_causality=unproven 下宣称“卡顿完全来自调度”。这是 B26-PHASE1 的重复模型消费失败；系统不扫描/改写答案，暂留观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B28-SHARD1=covered`：重叠 shard 的跨窗加法权限已撤销，max/window/support refs 保留。
- `EVAL-B26-PHASE1=partial/model-consumption-watch`：typed 阶段边界与 caller/holder 角色已经进入 finalizer，模型本轮仍误用；按红线不以答案关键词门或系统替换结论强行纠正。
- `EVAL-B21-CALLEE1/SPAN1=reproduced`：支持面把非 load-bearing 的模型自由 summary 重新抬成主 Evidence note，是可确定的系统扩权口，进入 B29a 通用修复。
- 新登记 `EVAL-B29-LANE1/P1`：机制调查没有 typed execution-lane/path membership；同文件 read/write 相邻实现均可进入 principal evidence，模型可拼接互斥路径。先撤销 summary 扩权；路径成员权威需独立 typed 设计，不能按函数名或用户/答案 prose 做特判。
