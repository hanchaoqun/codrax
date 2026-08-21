# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T03:05:31Z
- sweep_start_ts: 20260820-200531
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-200531 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 229s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | uncertain | 显式窗、三次 typed 查询、系统补采、四跳唤醒链、11ms 链上 IO 第一席、三条独立 1ms 调度/优先级候选、实际占时/规则可消双账与 Trace 因果投影均保留；邻近/背景未越权，活动流无 4ms/4m 降级。人工 partial：模型把 `fscache_page_wait_on_page_bit` 调用点过推成“具体缓存页/预取/缓存穿透”，又把 `on_chain`、`direct_blocking_authority` 等机器枚举抄到正文；还宣称目标等待“完全由链传导/协作式睡眠”，超过未建立的 direct-blocking 权限。首稿把只允许 summary 的 `trace_causal_claim_caliber` 放到所有块导致一次拒绝；无效 runtime `candidate_role=thread` 被系统静默改为 `other`，没有拒绝但可能污染角色语义。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-200531 | answer_regex,answer_contains | none | 995s | 61 | read=26,repo_map=2,list=0,trace=0,source_lens=0 | midloop=32,inv=9/0,fin_reject=12,unavail=0,prune=0 | fail | runner 字面项通过，但过程与答案质量失败。checkout authority 把四个 stage 支撑点铸在 composite literal 的 `{` 行（46/60/72/84），grounding 却稳定返回语义 `Stage:` 字段行（47/61/73/85），两次 explore 因固定一行偏差反复重查。finalizer 的 rejected patch 事务回滚后，relation lease 针对刚被拒 merged 草稿生成，而下一 patch 仍以首次拒绝稿为 base；8 个 live failure refs 前四个可执行后，第五个稳定报正文无 `Orch->ANA`，形成 12 次拒绝。最终虽有表和答案，但图用 note 代替调度/状态关系，只保留三条 stage precedence，且引用同时暴露 `{` 行与 `Stage:` 行，不能视为健康完成。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
