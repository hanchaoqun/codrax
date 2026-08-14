# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T16:25:29Z
- sweep_start_ts: 20260814-092527
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260814-092529 | answer_regex | none | 121s | 25 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 六条 typed call edge、两条真实分支、完整 walker 角色和合法 Mermaid 均保留，系统未代画/改写。正文把先 collect_files、再 for-loop index_file 的顺序误称为“两个分支并行”，图本身仍按先后绘制；这是模型措辞错误，typed finalizer 提示其实已明确 fan-out 不证明并发，暂不以答案词面硬门修补。 |
| 1 | trace_query_wakeup_causal_runnable | FAIL | eval/results/trace_query_wakeup_causal_runnable-20260814-092529 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 175s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | raw trace_query 已产生两条 wakeup_chain.causal_impacts：app sleep 10ms（context/advisory）与 on-chain worker runnable 8.3ms（prio 20 vs target 52、priority_inversion_candidate=true、effective=8.3ms）。Analyzer 却同时发 intent/scenario=root_cause、is_dimensioned_answer=true 但零 dimensions、scope=bounded_fact_set；scope projector 因而只给 finalizer 目标 app impact，裁掉 worker 根因席。终稿虽正确指出 worker 调度延迟，却错误否定 typed 优先级反转候选并把未证初始 sleep 说成“唯一等待来源”。这是 typed breadth 合同矛盾，不是 finalizer retry、JSON 降级或模型单点波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B815-RUNTIMEBOUNDEDROOTCAUSECONFLICT1/P1`：`bounded_fact_set` 与明确 `intent=root_cause` / `scenario=root_cause` 不可同时成立。旧合同会让投影器按有限事实宽度合法抑制根因链/排序证据，使声明的根因目标不可回答。
- `requested_answer_dimensions.is_dimensioned_answer=true` 但零行不再软降为空 profile；这是精确 JSON 结构矛盾，必须在 analyzer 一次完整重发时修正。根因/目标影响维度使用 typed `causal_attribution`。
- 修复只消费 schema enum、boolean 与数组 cardinality，不扫描用户请求、模型思考或最终答案，不改变 query 结果，也不由系统选择根因。普通诊断上下文中的有限事实查询仍合法，但必须使用非 root-cause intent/scenario。
- Rust 的“并行”属于模型已收到准确软边界后仍发生的一次措辞漂移；先记录观察，不新增 prose 硬门或系统文本改写。
