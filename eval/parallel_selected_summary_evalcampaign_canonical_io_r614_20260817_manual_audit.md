# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T12:11:26Z
- sweep_start_ts: 20260817-051125
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-051127 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 200s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | 显式 2.000..2.020s 内完成 4 次 typed 查询，自动补齐、Trace 因果投影、链上/邻近/背景分区及实际占时/规则可消除量双轴完整；主席为链上 threadpool-400 iowait 11.000ms，后续三个 runnable 段各 1.000ms。模型同时把 kernel call-site `fscache_page_wait_on_page_bit` 外推成具体页面/磁盘/预取争用，并建议跨核亲和性，超过 typed 对象/holder/subsystem 权限，继续归 B965 软引导观察，不加正文硬门。该 fixture 验证 D/iowait+唤醒链，不是 block_rq_issue↔complete cookie 关联的生产证据，后者仍需独立真实用例回放。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-051127 | answer_regex,answer_contains,mermaid_edge_count | none | 466s | 35 | read=20,repo_map=2,list=0,trace=0,source_lens=0 | midloop=14,inv=5/0,fin_reject=3,unavail=0,prune=0 | fail | Runner 只证明有图和表面关键词。首个 completion 把 `[extractor BusContext Mutable]` 判缺失并错误导航到 `forcedReadCancelled`；第二轮仍指向枚举校验 getter。最终正文把 `extractor.go:1989 EmittedAnswerSymbols()` 的读取误写为写入，又把 contract checker 的读取当成 finalizer 交接；图只保留三条 stage precedence 和一个局部 getter 调用，BusContext/Mutable 明示 unproven，未回答请求的数据流。根因是 verified stage component 在 source-symbol 精度筛选前被拆散：同名局部字段使 extractor 独自进入 operation scope，其他 stage 被排除；B970 仅追加 canonical ResolvedAs，面对生产中 ResolvedAs 为空的同名字段无效。B971 改为由 stageauthority typed precedence 独立准入全部阶段，源实体仍走严格 symbol 解析。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
