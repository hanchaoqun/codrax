# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T03:47:22Z
- sweep_start_ts: 20260801-204720
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260801-204722 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 146s | 28 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 系统 typed 面正确给出 #1 class_verification 4.600ms、#2 target runnable 0.800ms，并给出 frame_causality=unproven / frame absent、wakeup edge 不证明同步阻塞等边界。模型却把 lower_priority_waker 写成“CFS 线程迫使 RT 线程等待”的优先级反转机制，且 caveat 自己承认无 typed priority_inversion impact，正文内部矛盾。系统主要占用表另把同一 app-100 runnable 0.800ms 物理区间按 chain/self 两条发布车道重复显示。runner 的存在性 oracle 仍 PASS，属假绿。 |
| 2 | read_combo_git_diff_hunk_current_code | PASS | eval/results/read_combo_git_diff_hunk_current_code-20260801-204722 | answer_regex | none | 174s | 30 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | 模型主体正确区分最近 docs commit、历史 diff 线索和当前源码。但系统「清单完整性补充」按 members/support_refs 下标猜配，把 trace_query.go:7907 配给 answer_document...go，把 answer_document...go:92 配给测试文件，并追加第二个重复清单；最终答案含确定性错误。根因是纯文件 member_set 的一般支持锚 roster 被误当逐项同序数组。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

1. `EVAL-B34-READANCHOR1/P1` 是系统确定性 gap：当 principal member_set 全部是源码文件路径时，成员本身已提供精确 identity；裸 `support_refs` 可能只是机制锚 roster，不能仅因数组等长就按下标配对。最优通用方案是优先按 canonical file path 同一性连接；无同路径锚保持 uncited，符号/配置等非纯文件清单继续保留既有 positional contract。
2. `EVAL-B34-TRACEPHYS1/P1` 是系统显示 gap：同一主体、同一 typed runnable/sleep/running 状态族、完全相同起止时间的物理状态段可从 chain 与 target-self/rank lane 重复发布。主要占用是物理时间面，必须按物理 interval identity 显示一次；D-state 与 io_wait 是不同校准口径，不能被该规则合并。
3. `EVAL-B34-MODELCAL1/P1-model-owned-open` 是 B33 后第二个不同 Trace witness：finalizer 已收到 typed authority、两轴和禁止推断边界，模型仍把低优先级依赖候选扩大为已发生的优先级反转/同步阻塞，并与自己的 caveat 冲突。不能据此让系统改写正文或替模型选根因；后续方案只能是模型自有的结构化 diagnosis decision/复核回合，或模型评审，不得把用户/模型 prose 关键词扫描接成答案硬门。
4. `EVAL-B34-ORACLE1/P1-open`：两 case 都 runner PASS、人工 FAIL。Trace case 的 oracle 只验证实体/span/时间窗存在，不能验证 causal caliber；read case 只验证 diff/current-source 词面，不能验证系统补表的 row-to-anchor identity。当前看护不是“过硬”，而是覆盖不足、会假绿。修复也不能回到 `EXPECT_NOT_CONTAINS=优先级反转` 这类单词拟合；应使用 typed decision/row identity 或独立模型评审。

## Implemented batches

- `1bf335d90`（`B34-READANCHOR1`）：纯源码路径 member_set 的支持锚改按同路径 identity 绑定；同路径多锚优先保留同位置槽，唯一非同位置同路径锚可正确重连，无匹配则不造 citation。配置优先级等混合/符号清单继续使用原 positional contract。`internal/types` 与 `internal/tool` 全量回归通过（21.461s / 162.238s）。
- `7527c29cc`（`B34-TRACEPHYS1`）：主要占用表对同主体、同 typed 物理状态族、同起止时间的跨发布车道镜像显示一次，保留首个富载体的位置/值；不合并 D-state 与 io_wait，不改因果节点、排序或可消除账。`internal/tool` 全量回归通过（161.401s）。

状态：`EVAL-B34-READANCHOR1=implemented/full-related-tests-pass/replay-later`；
`EVAL-B34-TRACEPHYS1=implemented/full-related-tests-pass/replay-later`；
`EVAL-B34-MODELCAL1=P1/model-owned-open/no-system-rewrite`；
`EVAL-B34-ORACLE1=P1/open`。
