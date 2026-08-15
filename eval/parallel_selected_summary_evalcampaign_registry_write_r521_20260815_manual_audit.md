# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T18:30:01Z
- sweep_start_ts: 20260815-113000
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260815-113001 | write_plan,write_patch_oracle | none | 59s | 24 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | `main.py:20` 的 `retrun`→`return` 单行计划、目标路径和三项验收均准确；没有执行越界改动，也没有空计划/旧稿恢复。 |
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-113001 | answer_regex,answer_contains | none | 100s | 27 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | B849 生产闭环：Relation Dossier 已出现 `provenance=typed_evidence, source_kind=registry_identity_chain`，`1 / explorer` 结论正确，r520 的 `No evidence-authorized...` 与弱证据 caveat 均消失。残余 B848 被再次确定性复现：模型提交注册行和 `Name()` 返回两条 citation，但表格行只有一个 `citation_ref`，注册行因 `unused_pool_entry_pruned` 被清理；最终只剩 `sub_explorer.go:33` 的引用，系统补充 owner 锚虽保住 `subagent.go:64` 定位，却不能替代模型正文的多轴引用。另有一次表格 `cells[]` 与 `label/text` 互斥合同修补，错误提示精确且第二轮成功，非矛盾合同，但增加 7s 成文开销。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Machine verdict: `2 / 2 PASS`; human verdict: `1 pass / 1 partial`.
- `B849-REGISTRATIONTARGETROLE1` and the remaining production closure of `B844-BRIDGELITERALREGISTRYRELATIONAUTHORITY1` are closed by the registry replay.
- `B848-MULTIAXISTABLEROWCITATIONCARDINALITY1` remains P1 and is now the next citation-identity batch: one visible item must be able to carry several independently typed evidence anchors without prose/column-name inference.
- `B846-PATCHCITATIONIDENTITYREMAP1` remains independently open; this replay did not exercise its scalar-literal remap witness.
- No fixed-age active-stream degradation was observed. The 100s registry run completed normally; no Trace, causal-projection, background/root-cause, or system-authorship path changed in this batch.
