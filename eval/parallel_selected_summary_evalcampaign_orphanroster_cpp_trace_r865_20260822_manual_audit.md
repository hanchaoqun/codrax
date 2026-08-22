# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T15:16:53Z
- sweep_start_ts: 20260822-081651
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-081653 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 153s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗和三次 typed trace_query 均在；完整 threadpool-400→network-300→cookie-200→app-100 唤醒链、11ms 链上 IO 第一席、三个独立 1ms runnable/优先级候选、实际占时/规则可消双账户、业务线索、背景隔离、自动补齐 critical_blocking_calls 与最终 Trace 因果投影完整。无成文拒绝、无固定时间阈值降级、无系统改写模型结论。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260822-081653 | answer_regex,answer_contains | none | 306s | 30 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=5/2,fin_reject=4,unavail=0,prune=0 | partial | 调用链、stderr、console 工厂选择、unique_ptr 注入、虚分派与基类空 flush 的文字结论正确；B1350 生产转正，成文拒绝 11→4。但首轮原生 kind=diagram 被实时 enum 拒绝，模型发现工具说明与 enum 自冲突后改发 section+diagram；兼容拆分生成 section 与派生图，后续只删除图半而留下“调用路径时序图”空标题，并补出重复路径段。图缺失不是证据不足或模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. B1350 获得生产正证：同轮多个孤点/关系修补没有再因未披露 roster 反复停住，C++ 最终成文拒绝由 r864 的 11 次降到 4 次。
2. 新 P1 `B1351-DYNAMICSCHEMATEACHING1`：`emit_answer_document` 的静态说明无条件宣传 diagram block，dispatch-local schema 却因本题没有 typed DiagramPlan 从 `blocks[].kind` enum 和 diagram 字段中删除它。首轮合法原生 diagram 因 enum 被拒；模型明确指出说明/schema 矛盾后，用 `kind=section` 携带 diagram 字段绕过开放对象 schema。该矛盾确定性增加模型心智与重试。
3. 新 P1 `B1352-FUSEDDIAGRAMCOMPANION1`：兼容 splitter 将 `section+diagram` 拆成可见 section 与派生 diagram，但没有给后续 patch 暴露二者的 typed lineage。模型显式删除可选派生 diagram 时，系统既不能替模型删除 companion，又没有要求模型选择 companion 的 remove/retain disposition，于是空标题合法遗留。应在禁止新融合载体后，给历史/恢复路径增加精确同伴处置能力；系统只披露 lineage 和选择，不自删、不改标题或正文。
4. B1349 的畸形 Mermaid 降级本轮没有重复，但因本轮图被显式删除，不构成否证，继续开放。
