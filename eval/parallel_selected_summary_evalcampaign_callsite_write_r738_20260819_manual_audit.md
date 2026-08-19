# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T18:15:11Z
- sweep_start_ts: 20260819-111510
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-111511 | answer_regex,answer_contains,mermaid_edge_count | none | 607s | 31 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=13,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail（形式绿、语义未满足） | B1175 获生产正证：成文从 r737 六次 patch/超时降为一次 patch/完成。但 Analyzer 发出 `is_cross_component=true + predicate_axis=flow + entities=7 + participants=[]`，relation/dimension quote 又都未带参与者；旧一致性门漏过。终稿图删除 Analyzer/Explorer/Extractor/Finalizer/BusContext，只画 5 条底层 helper 边，runner 仍 PASS，未满足用户指定的组件数据流。探索期还把 `llm.SummaryExtractor` 错当 extractor agent；正确锚补出后无 typed 撤销通道，旧 recovered 行持续占账，造成 22 explorer iter/5 次 completion。 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-111511 | log_regex,write_apply,answer_regex,answer_contains | none | 1110s | 26 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail（诚实保留 verify_failed） | 首批 patch 只把相邻 `(10,10)` 对分别变成 rank，五换行仍为 `[300,300,10]`，项目测试正确失败。随后普通 tests_failed replan 两轮分别输出约 428k/353k 活跃语义字节，读过 test/source 仍未发出新 plan，最终由 eval caller deadline 取消；系统没有按 4ms/固定年龄降级，也没有洗绿，保留 recovery ref 并披露 verify_failed。B1176 的 proof-only follow-up 分支没有到达，本轮不能判其生产失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论与下一批

1. `B1175-DIAGRAMJOINCANDIDATEPRIORITY1` 生产有效：有 typed crossing frontier 时不再被局部 participant 症状压住，最终成文只拒一次；但它不能弥补上游 participant slate 本身为空，不能把这次 runner PASS 记为语义闭环。
2. 新 P0/P1 `B1177-DIAGRAMCROSSCOMPONENTPARTICIPANTCONSISTENCY1`：required relation diagram 若同时声明 `is_cross_component=true`、明确 relation axis、至少两个 typed entities，却提交空 participant slate，应在 analysis schema seam fail-loud。模型可补显式 current-request participant，也可撤回错误 cross-component/entities；系统不得推导 node/edge。该修复已按生产形补 pin。
3. 新 P1 `B1178-EVIDENCEEXPLICITRETRACTION1`：`emit_evidence` 只支持同 stable identity 的 metadata amendment；当错误恢复到另一个文件/符号时，正确新行无法撤掉旧行。最优形是 model-owned、exact-selector、unique-match 的 typed retraction/supersession，作用于本模型已发证据；歧义/系统证据 fail-closed，不按 summary 相似度自动删。
4. 写案例是普通功能失败，不是 proof-only 缺证。B1176 尚未到达，继续由单元/跨模块 pin 守住，待一个“功能先绿、随后只缺 proof”的生产回放再收账。
5. 新观察 `B1179-WRITEREPLANACTIVESTREAMMINDLOAD1`：tests_failed planner 在已读 exact failure/source 后仍产生超长重复推理，消耗 1110s。先以异构案例复现确认；优先收窄 typed replan context 与软教学，不能按关键词截断 reasoning，也不能把活跃字节流按固定年龄降级。
6. eval caller deadline 是合法 cancellation authority；本轮 active stream 持续保活，未发生 4ms 降级。取消后用户可见输出诚实披露补丁已保留、验证失败和 recovery ref，但 final artifact 仍为 `run_status=in_progress/verdict missing`，记作终态持久化观察，不据单例扩大修改。
7. 本轮不改 Trace。显式窗因果投影、自动补齐、链上-only 主因、实际占用/链上业务线索与规则计价可消除量双轴保持；邻近/背景不能晋升主因，系统不改模型答案或结论。
