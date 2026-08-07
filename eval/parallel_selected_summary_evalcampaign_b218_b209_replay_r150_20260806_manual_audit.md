# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T06:08:59Z
- sweep_start_ts: 20260806-230857
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-230859 | answer_regex,answer_contains | none | 202s | 21 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | B236 typed `entry_role`/component class 已进真实 finalizer 上下文，但模型仍把互不连通的 factory selection、Logger 注入与 runtime dispatch 写成一条“完整路径”；样例没有 factory 产物交给 Logger 的调用点。图门诚实拒绝未证图边，模型随后删图；最终另有 2 条 citation detach 披露。说明 B236 carrier 有效但不足，需把 directed hop 与 non-hop support lane 结构分开，不能扫描/改写正文硬纠结论。 |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-230859 | answer_regex,answer_contains | none | 429s | 28 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=8,inv=2/0,fin_reject=5,unavail=0,prune=0 | pass | 最终类、`run_pipeline -> resolve -> cls()`、callback、MRO/super 协作链及 `@register("json")` 作用均正确；B235 已生产闭环，decorator 引用保持 `pipeline/plugins.py:17`。但过程 5 次 reject：当只剩 ordered-list 的两个未证 direct-call anchor 时，系统仍选择 `answer_doc.patch_optional_diagram_call_edge` 并误导删图；一次 patch 又把 `replace_blocks` 发成畸形 JSON 字符串。结果正确，修复路由与 JSON 删除教学为确定性系统 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r150 audit verdict

- Runner：2/2 PASS；人工正确性：1/2。
- `EVAL-B235-CITEREBIND1`：closed，精确 decorator 引用在生产回放保持单调。
- `EVAL-B236-PHASEBRIDGE1`：partial，typed segment 已发布，但当前 principal call-chain carrier 仍允许 non-hop fact 与 directed hop 共用 ordered-list 心智模型。
- `EVAL-B237-PATCHSCOPE1`：P0/P1 confirmed。`ViolDiagramCallEdgeUnproven` 是历史族名，既可由 diagram body 产生，也可由任意 block 的 `edge_anchors[]` 产生；finalizer 仅凭 violation kind 把 ordered-list 违规误路由为 optional-diagram-only 修复。
- `EVAL-B238-PATCHDELETE1`：P1 confirmed。patch schema 有 `remove_block_ids`，但初始/通用 patch 教学只列 keep/replace/add，模型需要自行猜测“省略块不等于删除”，增加畸形 JSON 与无效 patch 概率。
- 两项修复都必须使用 typed repair metadata/schema operation，不读取用户原文、模型 thinking 或最终答案 prose；不得系统删除图、重写答案或替模型补桥。
