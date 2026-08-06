# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T13:58:53Z
- sweep_start_ts: 20260806-065851
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-065853 | typed_inventory_rowset,dimension_substring,answer_contains | none | 144s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 真正的 Cangjie declaration inventory 保持 principal，2 extend + 2 foreign func + 8 public class 的权威行全部正确且无 JSON/成文重试；但答案摘要自报 public class=10，正文实际只列权威的 8 项，自动 rowset oracle 未校验同答计数，形成可见自相矛盾。 |
| 1 | qf_architecture | PASS | eval/results/qf_architecture-20260806-065853 | answer_regex,answer_contains | none | 569s | 39 | read=2,repo_map=20,list=0,trace=0,source_lens=19 | midloop=19,inv=9/0,fin_reject=0,unavail=0,prune=1 | fail | 第一轮 typed analysis 是 intent=explain/scenario=generic/question_kind=mechanism，且 required stage_or_workflow 明确，但 source inventory profile 仍获 principal authority；系统随后把 subtopic 改写成 source quote 并因与 primary entities 不相交而硬拒，第二轮漂成 enumeration 后触发全仓 census。最终误称 ReadModeConditionalPreStageBindings() 包含 MultiRepoFocus，并把 StageExtract 写成“抽取最终答案/生成 evidence”。自动 PASS 未覆盖过程失控与语义误述。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
