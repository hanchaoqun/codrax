# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T16:20:50Z
- sweep_start_ts: 20260808-092049
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-092050 | answer_regex,answer_contains | none | 139s | 23 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=3,unavail=0,prune=0 | pass | R218 的 typed mismatch 外科提示在生产生效：`AllMainStages -> StageAnalyze` 单独失败后，模型只删除该边，保留已通过的 3 条 stage precedence 边；最终核心图正确。质量残余：一次 patch 漏 `kind` 触发 JSON shape reject；正常核心答案后又附完整重复的“第一稿答案（校验前参考）”，见下方 B369。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-092050 | answer_regex,answer_contains | none | 390s | 39 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=8,unavail=0,prune=0 | fail | analyzer 已按新 schema 显式发 `predicate_axis=flow`，但 explorer 未跟随 `AllMainStages` 到返回值；反而以 `scope=line_range` 把 const 声明 33-36 行伪造成 `StageAnalyze -> StageFinalize` precedence。最终图删成孤立节点，正文仍误称 `EvidenceItems` 经 `WriteAnalysisIR` 持久化、把 no-tool draft recovery 当 Finalize 核心路径。runner oracle 未覆盖 owner/数据流正确性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed systemic gaps

- `EVAL-B369-REVIEWDRAFTDUP1` (P1): accepted repaired core answer is followed by a full duplicate “系统保留内容 / 第一稿答案（校验前参考）”. The review carrier is being published even when it adds no unique model-authored content, increasing contradiction and authority risk. Audit the typed review/display carrier before changing it; do not compare prose or replace the model conclusion.
- `EVAL-B370-LINERANGERELATIONBYPASS1` (P0 / red line): `scope=line_range` only proved range existence and bypassed the semantic AnchorKind grounder. A model could change scope and mint a call/precedence/guard/assignment relation from unrelated source lines. The production witness is `internal/types/enums.go:33-36` const declarations accepted as `StageAnalyze -> StageFinalize` precedence. This reopens only the range-scope arm of B364; the line-scope ordered-value grounder remains correct.
- `EVAL-B366-MECHANISMOWNERCONTEXT1` and `EVAL-B368-FLOWCONTEXTRELEVANCE1` remain open: the handoff contains broad, irrelevant or recovery/test flows but misses verified operation-level producer/transfer/consumer ownership for the requested mechanism.

Neither case invoked Trace. These findings do not authorize any change to explicit-window Trace projection, automatic supplementation, on-chain root-cause eligibility, or the separation of adjacent/background evidence from main-cause seats.
