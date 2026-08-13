# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T19:49:58Z
- sweep_start_ts: 20260813-124957
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-124958 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 190s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail / provenance normalization gap | B737 上下文正证：supply-fold 已明确只表示 headroom，上限记录不证明目标命中/binding，模型正文也复述了这两条；仍 FAIL 的先因是第三个 required dimension 被 provenance normalizer 删除。Analyzer 把用户原话中的 `CPU 频率` 写成 `CPU频率`，quote/label 因唯一空格差异未锚定，系统遂连 schema-valid dimension 一起丢掉而非只丢 quote；bounded scope 被接受，完整因果投影关闭。另有 CPU12=1.53GHz、tail_open=runnable 等模型事实误读。B738 应只扩 exact quote 的空白容忍并加强复合“是否+证据”维度教学，不从 raw/final 文本推 role。 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260813-124958 | answer_regex | none | 211s | 27 | read=3,repo_map=1,list=1,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | 最终正文与合法 sequence 图都完整保留 native/fallback 两路：Python guard、`_fastlex.tokenize_bytes`、PyO3 registration/wrapper、Rust core 和 `_tokenize_slow`。首稿关系门拒绝未证模块/自环/回复边，第一次 patch 因模型漏 `kind` 被 schema 拒，第二次 patch 自愈。两次拒绝属可恢复模型构造问题；最终 Mermaid 合法、没有关系丢失或系统代画。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
