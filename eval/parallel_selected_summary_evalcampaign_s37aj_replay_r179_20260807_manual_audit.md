# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T18:13:21Z
- sweep_start_ts: 20260807-111320
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260807-111321 | dimension_substring,answer_contains | none | 57s | 20 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 首稿一次通过；typed inventory 完整呈现 1 个 extend、1 个 foreign func、3 个 public class，member/package/file/line 全部正确，表格无重复、无泛化 caveat。Analyzer 本轮 `has_per_member_table=false`，表格属于模型自主选择，因此只能证明 S37aj 没有干扰普通 source-inventory 载体选择，不能当作 sole-table 缺失拒绝臂的 production witness。Explorer thinking 一度误称 DAML，但没有进入 typed evidence 或最终答案，按模型波动观察。 |
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-111321 | log_regex,answer_regex | none | 61s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终输出严格只有 `{"ids":["u1","u3"]}`，规则、筛选、顺序和 json_only 合同均正确。过程仍固定多一轮 repair：模型把 `instructions.md` 标为 `script_consumed`，首个 custom_transform 却只调用 `json_load('users.json')`；`required_material_scheduling` 精确拒绝后才补 `read_text('instructions.md')`。该同形已在多个回放复现，确认新的 usage-mode/脚本消费心智 gap；硬门本身正确，不应降级或扫描最终 JSON。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
