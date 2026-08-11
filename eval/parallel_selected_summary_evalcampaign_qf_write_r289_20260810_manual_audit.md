# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T03:07:14Z
- sweep_start_ts: 20260810-200713
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260810-200715 | write_plan,write_patch_oracle | none | 91s | 21 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确单行 `retrun`→`return` patch、路径/old_text/acceptance/probe 均正确，未写工作树。首次把同一 `import_after_fix` probe 同时放在顶层和 change 内，被全局 duplicate-id 门拒绝后一次修正；确认 B502 JSON 教学缺少“每个逻辑 probe 只选一个 carrier、ID 全计划唯一”。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-200715 | answer_regex,answer_contains,mermaid_edge_count | none | 446s | 36 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=3,unavail=0,prune=0 | fail | Explorer 已找到 Analyzer/Explorer/Extractor 对 Mutable 的三条 exact call，正文正确说明 TurnAArtifacts 数据流；首稿图也画出三边，但用组件节点 A/B/C/M 代替 exact callable endpoints，被 validator 正确拒绝后模型删边，终图仅剩 stage precedence 与断开节点。B491 精确 primary-identity 提示使 5 次降到 3 次并最终收敛，但 B500 仍只把 StageReport 128→57：同文件和 ±24 行兄弟 flow/evidence 继续旁路。确认 B500 v4 与 B501 layered component/endpoint 图教学。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- runner: 2/2; human: 1/2.
- `B500-STAGEREPORTSCOPE1`: still partial. Required-diagram scope must not use file fallback when typed endpoint anchors exist, and concrete source evidence must use exact id/location rather than ±24-line proximity.
- `B501-LAYEREDGRAPH1/P1`: when user-facing component identities are broader than typed callable endpoints, teach a two-layer graph: exact endpoint nodes carry copied relations; component subgraphs/labels carry responsibility. Never retarget a typed edge to an abstract component, but also do not delete the edge merely to simplify presentation.
- `B491-PARTALIAS1`: production-positive but not closed; precise primary-identity wording reduced churn and was consumed on the final repair.
- `B502-PLANPROBECARRIER1/P1-small`: ChangePlan JSON-shape SSOT must state that probe ids are plan-global and each logical probe belongs in exactly one of top-level or change-local arrays.
- No model/final prose scanning, system-authored graph/answer, validator relaxation, or Trace machinery change is warranted.
