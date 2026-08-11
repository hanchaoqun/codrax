# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T10:38:06Z
- sweep_start_ts: 20260811-033805
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260811-033806 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 160s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B521 有正效：不再把 11+1+1+1=14ms 明写成 20ms 完整分解，11ms IO 根因、3 个 1ms 调度席、链上/背景权限均正确。但模型仍写“app-100 的 wakeup 延迟等于前面三个节点各自阻塞时间之和”，typed 席并无该加法权威；B521 仅 partial。投影重复进一步扩大：threadpool iowait 显示 E5(+2)，target sleep 与 cookie/network sleep 仍跨来源重复，B522 继续开放。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260811-033806 | answer_regex,answer_contains | none | 338s | 41 | read=25,repo_map=2,list=1,trace=0,source_lens=0 | midloop=14,inv=2/0,fin_reject=1,unavail=0,prune=1 | fail | 初稿确有 sequenceDiagram；Analyzer 首次 emit 的 required sequence hint 因首个推断参与者 `Orchestrator` 的 source_quote 不在 CURRENT request 而被整次拒绝，模型随后删除整个 diagram_hint。最终 contract 变为 has_diagram=false，关系门把图当 optional 并明确允许 remove，patch 删除 d1 后通过。不是模型随机漏图，而是 visual kind/required 与 participant provenance 错误耦合的 B523。B520 因 required boundary contract 未建立而未获生产验收。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B521-TRACEADDITIVE1`: production-partial。显式 14=20 已消失，但仍有同类“各节点阻塞时间之和”无权威加法断言；不增加答案关键词硬门，继续审计 handoff 的数值关系清晰度。
- `B522-TRACEDEDUP1`: audit-open。三次 trace_query 使同一 iowait 席显示 `E5(+2)`，另有 target/intermediate sleep 跨来源重复；必须按 measurement/source/interval/caliber 核对，禁止 subject+value 粗去重。
- `B523-DIAGRAMAUTHDECOUPLE1`: confirmed/P1-high。显式 required diagram 的 kind/required 必须独立于 participant row provenance；无当前请求出处的推断 participant 只能局部剥离，不能让整张图降为 optional。
- `B517-STREAMLIVE1`: 本批 338s pipeline 正常得到模型答案，未触发 1× timeout 系统降级；属于无回归观察，不是 hidden-reasoning 专项生产关闭证据。
