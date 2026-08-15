# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T15:54:38Z
- sweep_start_ts: 20260815-085437
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-085439 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B839 生效：Analyzer 没有再把 `5.000s..5.007s` 时间窗铸成 7 项集合，extractor=0、零假“枚举不完整”补块，显式窗、链上 0.800ms runnable 席、5.000ms VerifyClass 实际占用/业务线索、双轴与 Trace 因果投影均保留。人工结论仍有模型语义漂移：正文把窗长 7.000ms 写成“运行 7ms”（typed 状态账明确 running=1.200ms），把 CPU2 上 worker 与 CPU1 runnable 的跨核重叠写成直接资源竞争，并建议独占核；最终 prompt 已逐字携带原始 idle/1→app 行、target state partition、cross_cpu authority 及“不得宣称 direct competition”限制，所以不是缺证据或系统改写，按模型波动留档，不新增正文扫描硬门。 |
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260815-085439 | write_apply,write_patch_oracle | none | 213s | 24 | read=6,repo_map=2,list=1,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B838 获生产正证：pre-suite probe 通过但缺 plan contract ref 后，系统继续执行真实 `python3 -m unittest discover -v`，项目 suite 4/4 PASS，最终交付为“已验证”。补丁在构造阶段把整数值 float 规范化为 int、非整数 float 仍抛 ValueError；保留 applied tree 人工复跑同一 suite 亦 4/4 PASS。probe 告警没有被项目 suite 静默抹除，也没有把 parser/authoring 错误冒充产品失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
