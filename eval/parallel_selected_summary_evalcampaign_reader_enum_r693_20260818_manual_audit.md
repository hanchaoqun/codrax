# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T16:10:01Z
- sweep_start_ts: 20260818-090959
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-091001 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 215s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B1088 目标修复获正证：`bounded_window_candidate` 只出现在模型提交的 JSON caliber，未进入可见正文；显式 1.000..1.010 主窗、3 次 typed 查询、自动补采、Trace 因果投影、链上-only #1、邻近/背景降格和实际占用/规则可消除量双轴均在，零成文拒绝。仍有三个独立质量问题：模型把 worker 的 9.000ms 链上累计误称为 9.000ms runnable（实测 runnable=8.300ms）；把窗尾 1.010000 唤醒后的 1.010020 sched-in 说成窗内“立即进入运行、无额外 runnable”，越过所选窗；`dominant_state`、`synchronous_blocker_authority=not_provided` 等其他控制词仍进入正文。登记 B1091，不能靠答案扫描/替换关闭。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-091001 | answer_regex,answer_contains | none | 383s | 33 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=3/0,fin_reject=4,unavail=0,prune=0 | partial | B1088 普通关系面获正证：Finalizer 收到中文 reader fact，正文不再出现 `shared_callee_boundary`，并正确保留 `buildAnalysisIR -> RunWith`、`gate.Run -> RunWith` 两条真边和 inward fan-in 时序图。四次拒绝中首轮是畸形 `blocks` 字符串且恢复会丢 ordered_list，系统正确 fail-loud；第二轮暴露新 B1090：native JSON 给非图 summary/caveat 附空 `diagram:{}`，解码为非 nil 后 validator 误报 `diagram.body is empty`；后两轮是模型把 18 条 supporting sibling calls 放入 principal relation carrier、再给 supporting list 错挂 `call_edge`，validator 正确拒绝。终稿仍把 sibling calls 称作“buildAnalysisIR 到 RunWith 之间的中间函数”，属于 B1089 角色到成文 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

1. `B1088-INTERNALENUMCUSTOMERSURFACE1` 在本轮两个原始 witness 上均获 production-positive：非回填 call-graph status 已从 Finalizer prose context 消失；必须回填的 Trace caliber 仍在 JSON schema/field 中且没有进入客户正文。这证明本批没有通过删字段或系统代写换取表面结果。
2. 不能因此宣称所有内部词汇债已清零。Trace 模型正文仍复制其他 authority/status key，说明“只在尾部加软提醒”不足以普遍抵消前文高显著 raw 载体。`B1091-TRACECONTROLVOCABREADERSURFACE1` 需从 typed producer/serializer 侧继续做机器字段与 reader fact 分层，禁止最终答案字符串门。
3. `B1090-EMPTYOPTIONALCOMPOSITENORMALIZE1` 为确定性结构 gap：仅当可选复合字段严格为空、且其载体类型与 block kind 不匹配时，可在 JSON 解码/canonicalization seam 折叠为 absent；任何 body/kind/language/edge 内容非空都必须保留并继续严格验证。该规则可覆盖 `diagram:{}` 同族，而不是为本 case 特判。
4. Trace 正文的 9.000ms/8.300ms 口径错配与窗尾外推分别登记为 `B1092-TRACEVALUECOLUMNROLE1`、`B1093-SELECTEDWINDOWPOSTBOUNDARY1`；系统投影本体数值和主窗正确，错误发生在模型合成。后续应缩短并精确化 value-role/post-window reader handoff，不得改写模型结论。
