# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T00:07:54Z
- sweep_start_ts: 20260801-170753
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-170754 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 119s | 37 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 34579.472865..34579.587805 窗、Trace 因果投影、根因排序、唤醒链、窗内可消除量和系统补采全部保留；B28 继续输出重叠 shard `total_impact=unavailable`，无 14.204。模型答案末尾承认 causal_conclusion=unproven，但主结论仍把 lower_priority_dependency 候选写成“核心瓶颈/直接原因”，并把 pre-wakeup D/runnable 观察串成已证链；B26-PHASE1 继续是模型消费失败，不由系统改写。 |
| 1 | read_combo_analyze_retry_anchor | PASS | eval/results/read_combo_analyze_retry_anchor-20260801-170754 | answer_regex,answer_contains | none | 141s | 27 | read=9,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B29-SPAN2 生效：finalizer principal support 不再出现 fallback/default/nil 的自由 Summary Evidence note，最终答案也不再拼入 `fallbackWriteAnalysisIR`。但答案仍错误称 missing emit 时 `out.Error=""`（ParseOutput 实际发 populated Error）且语义耗尽会终止整个 Run；现行 Run 会对非 transport 错误安装 degraded IR 后继续。根因之一是 analyzer.go 与 runAnalyzePhase 的源码注释仍保留旧“whole Run terminates”合同，与 production control flow 相互矛盾。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B29-SPAN2=covered`：非 load-bearing Summary 已从权威支持面撤销；本轮 live replay 的 write-only fallback 污染消失。
- `EVAL-B29-DOC1/P1=confirmed`：production behavior 已演进为 transport hard-fail / semantic degraded-continue 两分支，但多个代码注释仍描述旧 hard termination，足以误导任何源码解释任务；进入 B29b 注释合同收敛。
- `EVAL-B29-LANE1=P1/filed`：本轮未建立完整 typed execution-path membership；不因 summary 修复有效就提前销账。
- Trace 不变量均在，`EVAL-B26-PHASE1=partial/model-consumption-watch`，不增加 prose gate 或系统答案 replacement。
