# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T23:52:22Z
- sweep_start_ts: 20260808-165220
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260808-165222 | log_regex,answer_regex | none | 36s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | `instructions.md` 以 required `planner_distilled` 材料进入 coverage contract，`users.json` 由一次 `custom_transform` 消费；终态严格为 `{"ids":["u1","u3"]}`。1 batch、0 repair、0 failed action、0 JSON recovery/contract violation。规则材料未作为 action input 重读是 planner-distilled 的既定权威车道，不是静默漏读。 |
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-165222 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 131s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S37da 生产生效：`frame_timeline`/`frame_flow` 均一次返回 Expected/UI/RS/GPU 四成员，身份分别为 `tid/tgid/cpu=20/20/1,20/20/1,2096/1716/0,300/300/2`，没有 CPU20/CPU300，也没有把 target 当成员过滤器。失败点有二：perf-triage prompt/观测仍以固定 60Hz/16.67ms 宣称“帧预算内”；Finalizer 虽收到 `frame_flow_causality=unproven, relation=temporal_sequence` 精确提示，结构化 `edge_anchors` 仍发 `call`，图和正文写“提交帧”“GPU 提交/完成依赖”，把时序邻接升级成未证因果。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `EVAL-B386-TRACECROSSTHREADSCOPE1` 与 `EVAL-B387-TRACESPANFIELDIDENTITY1` 可按生产正例关闭。两次查询各含四个跨线程成员，字段身份与原始 trace 的 CPU 列一致；本轮没有补读原文来猜 CPU。
2. `EVAL-B388-TRACEDEADLINECALIBER1` 只能记 partial。`trace_query` 已发布 `frame_deadline_authority=not_provided` / `refresh_rate_authority=not_provided`，最终主稿没有再下“未丢帧”结论；但更早的 perf-triage 单源教学和结构化 observations 仍把 16.67ms 当默认帧预算，给后续模型提供了相反上下文。新立 `EVAL-B390-PRETRIAGEJANKAUTH1=P0/HIGH`：无 typed deadline/refresh authority 时，只能发布测得 span/extent 与待验证 slow-frame candidate，不能由 Codrax 常量铸造 jank/no-jank 权威。
3. 新立 `EVAL-B391-TRACETEMPORALEDGEOWNERSHIP1=P0/HIGH`。最终 prompt 已明确未证帧边只能作为 temporal adjacency，但结构化 diagram block 自报 `app -> RS relation_kind=call`，另一个 RS→GPU 为 `observe`，且可零拒绝通过。最优边界不是扫描“提交/完成”等正文词，而是给 runtime 图一个 schema-valid `temporal` relation owner，并在“本报告只有 unproven frame temporal edges、没有任何 typed causal row”这一精确车道要求所有可见边使用该 owner；有真实 wakeup/IPC/lock/flow 证据的混合报告不得被会话级 ANY-sticky 降权。
4. `EVAL-B389-TRACEFINALCONTEXTCOMPLIANCE1` 继续开放。帧案 Finalizer 首轮上下文约 58.6K token，精确边界位于长 prompt 尾部；模型已读到边界却仍在 thinking、正文与图 metadata 三处残留因果化措辞。后续应按 typed carrier 去重、近席投影，不能靠扫描用户请求或模型原文硬门。
5. 数据案未发现 JSON 教学矛盾：规则材料、输出合同、action 和终态一致，0 次“成文校验未通过”。两案均无系统替写/删除模型主稿；Trace 系统附注只披露精确权限边界，没有把链外邻近/背景升级成根因。
