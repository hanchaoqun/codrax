# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T14:01:21Z
- sweep_start_ts: 20260807-070119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260807-070121 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 175s | 30 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 双轴结构已出现：模型先给真实占时/关键路径候选，再给规则内可消除量；显式窗、唤醒链、根因排序、代表窗和系统补采均保留，系统追加 typed 对账而未替换模型正文。但模型把 `priority_inversion_candidate` 扩写为“同一锁”“低优先级线程阻塞/持锁”，把 VerifyClass span 扩写为“持有类验证锁”，并用 CPU/IO 压力分直接断言双重竞争；final prompt 已明确 `holder_waiter_authority=not_provided`、candidate 不证明持锁/同步阻塞，因此判模型越权/波动，不新增 prose 扫描硬门。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-070121 | answer_regex,answer_contains | none | 203s | 26 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | no-directed-path 主边界已正确：最终分开展示 `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> RunWith`，没有再造反向路径；一次成文拒绝仅补 exact endpoint labels。答案仍漏 `analyzerGraphForNormalize @ analyzer.go:1866`，且把同一 caller 函数体内的 sibling calls 称为“并行调用”，无并发证据。runner 因早段 helper 缺失失败。生产日志未钉 frontier 发射事件，补真实仓库图 pin、typed sink relevance 和发布 debug witness 后再回放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
