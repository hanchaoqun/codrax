# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T15:40:08Z
- sweep_start_ts: 20260806-084007
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260806-084008 | typed_inventory_rowset,answer_contains | none | 112s | 21 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 4 个 @Entry 与 2 个 @Builder 均逐项命中 typed rowset，文件/行号和 thirdparty 边界正确；无 finalizer reject、畸形 JSON 或恢复降级。 |
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260806-084008 | typed_inventory_rowset,dimension_substring,answer_contains | none | 710s | 35 | read=8,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=17,unavail=1,prune=0 | fail | typed 事实本应为 2 extend、2 foreign func、8 public class；同一 `extend Cart` 被完备性合同要求 ADD、又被越界合同要求 REMOVE，17 次拒绝后降级，extend 行引用还被通用 type 修复从 Cart.cj:30 错绑到同名 public class 的 Cart.cj:14。非模型 JSON 错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap verdict

- `EVAL-B184-CONTRACTSAME1=P0/confirmed`：完备性、越界性与 citation 修复消费了不同优先级的 source-inventory 身份域；同一 typed 行收到互斥修改合同，属于系统红线，不是模型波动。
- `EVAL-B184-CITEMONO1=P0/confirmed`：精确 `(location, surface_family)` 行绑定先运行，随后被更弱的通用 `candidate_role=type` 修复覆盖；同名跨 family 时把 `extend Cart@30` 改成 `public class Cart@14`。
- `EVAL-B184-RETRYSTORM1=P0/confirmed`：互斥合同的 ADD/REMOVE 指纹交替变化，既有 same-cause breaker 无法识别，最终烧满 17 轮。根修应消除合同分裂，并对“已知成员、绑定错误”只发一个 typed row-id/citation 修复，不把它称为 extraneous。
- JSON 审计：本案没有 malformed JSON。第 9 轮出现 `blocks` string 兼容输入，但系统已恢复结构；持续失败来自 typed 合同矛盾。不得通过扩大模糊 JSON 猜测、扫描模型散文或生成系统替代答案来掩盖。
