# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T06:43:16Z
- sweep_start_ts: 20260805-234315
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-234316 | answer_regex,answer_contains | none | 124s | 26 | read=4,repo_map=1,list=1,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 四阶段身份与 Mermaid 顺序正确，成文零拒绝；但 4 条阶段职责只绑定 2 条 enum/type 身份引用，仍被 aggregate completion 接受并送入 Finalizer。答案进一步把 dataflow.Analyze 误称为 read-mode 流程入口，且“图后说明”被放到图前。runner oracle 只覆盖名称/顺序，未覆盖职责证据，因此机器 PASS 不等于人审正确。 |
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260805-234316 | log_regex,answer_regex | none | 163s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终精确输出 17,0,5；GroupA 两行 10+7、GroupX 缺失补 0、GroupC=5，GroupB=4 因不在 reference 目标集中被排除。首轮 assemble 缺 reference_path/reference_key_field 后按 typed repair 补齐，4 contributions、9 条 coverage、10 decisions 与最终 reconciliation 全通过；无 JSON carrier/element repair。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B167-MEMBERNOTEAUTH1=P1/confirmed`：current-source architecture/mechanism `member_set` 的 `member_notes` 可以在没有逐成员 `support_refs` 时定义 completion boundary；裸代码符号绕过了只保护 decorated member 的既有门。
- 最优修复是让已有 JSON 教学与 emit-time typed 校验同源：此类 roster 要么完全不发 `member_notes`，要么 `members/member_notes/support_refs` 三表等长、非空、同序。判据只读结构化 aggregate 与 evidence origin；runtime/Trace/VCS/connector carrier 保持豁免。
- `EVAL-B167-QFPRESENT1=P2/observe`：图后说明位置是一次成文遵约波动；不以扫描答案 prose 的 hard gate 修复。
- `data_multifile_reference_projection=human-pass`；一次 typed repair 有效且没有形成 JSON 心智负担增量。
