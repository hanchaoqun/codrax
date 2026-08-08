# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T16:48:23Z
- sweep_start_ts: 20260808-094822
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-094823 | answer_regex,answer_contains | none | 189s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=6,unavail=0,prune=0 | pass | B370 生产闭环：Explorer 只发定义事实，const 声明未再伪造 precedence；最终 3 条 stage 顺序边由真实 `AllMainStages` returned-value carrier 支撑并正确呈现。模型首轮未按 Ordered-flow handoff 发关系，导致 6 次拒绝和第二次 finalizer。最终核心答案正确，但又重复附整份“第一稿答案”，B369 再现。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260808-094823 | answer_regex,answer_contains | none | 253s | 31 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=5,unavail=0,prune=0 | fail | B370 正确挡住所有无证 assignment/call/precedence；因上下文没有请求相关的 operation-level producer/transfer/consumer 关系，模型最终删除全部箭头，未回答“数据流”。正文仍误称 Orchestrator 按 `preStages` 调四主阶段、BusContext 是唯一通道、Extractor 生成 AnswerChains、Finalizer 生成 StageReports/Signals。authority 同时写 `grounded_callsite_facts=0`、`explicit_typed_directed_relations=0` 和误导性的 `ordered_path_authority=typed_flow_paths_present`；128/24816 flow 主要为 test/helper/recovery 路径。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Runner: 2/2 PASS; human: 1/2. The declared oracles still accept a node-only “data flow” diagram and unsupported owner prose.
- `EVAL-B370-LINERANGERELATIONBYPASS1` is production-closed: unrelated const ranges no longer mint directed relation authority.
- `EVAL-B369-REVIEWDRAFTDUP1` is reproduced: a complete accepted answer is followed by a duplicate rejected first draft.
- `EVAL-B366-MECHANISMOWNERCONTEXT1` and `EVAL-B368-FLOWCONTEXTRELEVANCE1` remain the highest correctness gap. The system must select operation-level facts relevant to the requested producer/transfer/consumer path; it must not relax relation validation or let definitions mint edges.
- `EVAL-B371-LINERANGEEXISTENCEAUTH1` (P1) was found by adjacent code audit: ordinary range grounding comments promise file bounds/gutter coverage, but the implementation still authorizes a non-semantic range without proving any cited line exists. Handle separately from B370 so semantic relation closure stays auditable.

No Trace query ran. Trace explicit-window projection, automatic supplementation, on-chain-only root-cause eligibility, and background/adjacent evidence boundaries remain unchanged.
