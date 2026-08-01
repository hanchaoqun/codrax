# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T07:58:10Z
- sweep_start_ts: 20260801-005808
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260801-005810 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 155s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B19f 双轴通过：占用表、原根因排序/唤醒链/可消量/补采均在同一显式窗。主体却在 typed `frame_causality=unproven/frame_evidence_status=absent` 下断言“114.94ms 帧未完成、16ms 预算超约7倍”，并把未证依赖升级成“NetworkService 持有共享锁”；FRAME1 仍开放。占用表另把同一 Cookie sleep 的两套账目无关系地并列，存在误加风险，归入 ARITH2/账目关系融合。 |
| 2 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260801-005810 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 167s | 34 | read=3,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Analyzer 将单目标“是否进入状态/何时/原因/总量”错标为 `call_chain`（scenario=generic、non-diagnostic、full_artifact），使窄 D/IO supplement 失效并 windowless 补跑 `root_cause_rank`；最终新增占用块和完整因果投影，混入全 trace 与 34579.472865..34579.587805 子窗。主体把正确 3 段 0.138/0.147/0.350ms、Σ0.635ms 写成 0.197/17.903/19.565ms、Σ37.665ms。属于 typed 分类冲突 + 发布权限回归，不按模型波动遗留。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
