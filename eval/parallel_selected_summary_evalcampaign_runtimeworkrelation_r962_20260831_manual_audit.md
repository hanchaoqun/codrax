# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T07:48:38Z
- sweep_start_ts: 20260831-004837
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-004838 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 162s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 10ms 窗、typed query、链上排序、实际占时/规则可消双账、自动补采和 Trace 因果投影均保留；非链 IO 仍是背景。B1478 未获生产正证：analyzer 思考识别“确定性工作”，但 typed dimensions 仍只发 causal_contributor_set，没有 runtime_work_relation。模型正文虽列 VerifyClass 0.285ms，却先把 NetworkService runnable 称为“确定性优化工作”，且未以独立工作关系席清楚回答 host→target 唤醒凭证与 semantic-completion/target-wait 未证边界。系统投影正确披露边界，但不能替代模型回答。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260831-004838 | typed_inventory_rowset,dimension_substring,answer_contains | none | 1272s | 67 | read=11,repo_map=3,list=1,trace=0,source_lens=2 | midloop=7,inv=6/6,fin_reject=15,unavail=1,prune=0 | fail | B1479 正证：Cat/Vehicle 未再借 Dog/Service 坐标进入 typed roster，机械 Dog/Service 保留。新 P0：同一精确声明在多次已接受 handoff 中以裸名/完整 declaration/package 装饰多种写法出现，投影 canonicalize 后未再次按 typed member+location 去重，8 个 public class 被扩成 14 行，最终 got14/want8；同类 extend/foreign 变体令 finalizer 反复处理重复 row id，造成 15 次 reject、16 次 patch 与 1272s。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit disposition

- `B1479-INVENTORYROWMEMBERADMISSION1`: production-positive for its exact threat model. Wrong member names can no longer borrow a real declaration coordinate.
- `B1480-INVENTORYDECLARATIONALIASCANON1` (P0): confirmed. After exact typed declaration resolution, presentation aliases must collapse by canonical member + exact source location; same-name declarations at different locations must remain distinct and ambiguous same-location identities must fail open.
- `B1478-RUNTIMEWORKCLASSDISCOVERY1`: production-partial. The soft teaching is present, but this sample did not select the typed answer role. Keep under heterogeneous replay; do not add a request/prose keyword hard gate.
- Trace regression guard: explicit window, chain-only root seats, actual/eliminable dual accounts, semantic-work facts, projection and deterministic supplement all remained present. Background observations did not become root-cause authority.
