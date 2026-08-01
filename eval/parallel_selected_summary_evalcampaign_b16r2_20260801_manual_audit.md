# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T03:00:22Z
- sweep_start_ts: 20260731-200020
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_path_question_multi_trace_files | PASS | eval/results/trace_query_path_question_multi_trace_files-20260731-200022 | log_regex,answer_regex,answer_contains | none | 105s | 36 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 两个工件隔离、跨工件 authority 与显式窗投影均正确；但 trace1 同一 app-20 runnable 物理事实同时占 root-rank 与 wakeup-impact 两席，各记 5.000ms，形成确定性重复计量。模型另有 10.000ms endpoint 口径波动，typed 板未跟错。 |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260731-200022 | answer_regex,answer_contains | none | 299s | 27 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=0,unavail=0,prune=0 | fail | 12 个生产实现与 3 个 `agent_test.go` 测试桩一同进入“主要实现”主表/主图；当前 typed relation rowset 缺少 source_role，无法确定性分隔 production 与 test/fixture。另有 5 次 investigation_complete、10 次 midloop，存在次级探索成本。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Case 1: multi-trace explicit windows

人工结论为 **fail（system projection duplicate）**，不是跨工件串位：

- `path_trace_state_churn.systrace` 的 app-20 窗为 11.000–11.008s，
  typed state roster 正确给出 running=3ms、runnable=5ms；
- `path_trace_wakeup.systrace` 的 app-100 窗为 1.000–1.010s，
  typed state roster 正确给出 sleep=10ms；
- 系统明确把 shared clock、direct alignment、device/session relation 均标为
  `未证明`，没有把两个工件的局部时间戳当成共享因果时钟；
- 两个工件各自保留完整 Trace 因果投影，最终
  `trace_query_final_projection_blocks=3`。

确定性缺口是 trace1 的同一物理聚合事实重复占席：

```text
E1 app-20 runnable 5.000ms
E2 app-20 runnable 5.000ms
```

E1 来自 root-rank，E2 来自 wakeup causal impact；二者在因果树和指标表均
出现，且 effective 都是 5.000ms。它是既有 `EVAL-B7-T2` 的真实 production
witness：当前 one-seat 吸收只覆盖能由同形 line/span 证明的 twin，无法证明
宽/窄 envelope 下的同一 aggregate membership。

模型把 net-300 1.001200s 唤醒 worker-200、worker-200 1.010000s 唤醒
app-100 的端点差写成 10.000ms；按这两个端点应为 8.8ms。typed projection
仍区分 target sleep=10ms、worker effective=8.3ms/cumulative=9.0ms，系统板
没有跟错。先记 `EVAL-B16-TRMV1 / P3 / model endpoint binding variance`，
不扫描模型 prose、不为单值加硬门；若跨 case 重复，再提升到 typed
edge-duration claim producer。

## Case 2: LoopController implementations

人工结论为 **fail（typed source-role authority gap）**：

- 生产实现 12 个，文件与 type relation 本身正确；
- 另有 3 个 `agent_test.go` 测试桩也进入主要清单和 Mermaid 主图；
- 虽然模型文字把后 3 项叫做测试替身，但 principal inventory 没有 typed
  role 可据此分栏，不能由 renderer 确定性保证“主要生产实现”不被测试夹带。

泛化修复不能扫描请求中的“主要”、也不能按 `_test.go` 在答案阶段打补丁。
应在 typed relation producer/normalizer 增加结构化
`source_role={production,test,fixture,generated,unknown}`，保留完整 rowset，
同时让 principal/auxiliary projection 按 typed role 分席；没有 role authority
时 fail-open 展示但明确 unknown。该设计同时覆盖 caller、implementation、
subtype 等所有 relation inventory，而非只拟合 LoopController。

本席还有 5 次 investigation_complete、10 次 midloop、299s 的探索成本；
它尚未导致事实错误，登记为次级效率审计，不与 source-role 正确性批混修。

## 新增/更新工单

| ID | 优先级 | 类别 | 状态 | 下一步 |
|---|---:|---|---|---|
| EVAL-B7-T2 | P1 | causal projection one-seat | production witness confirmed | 找到唯一 projection absorption 点，以 typed membership/producer provenance 证明 root-rank 与 wakeup-impact 同一物理事实；不得仅凭相等数值合并 |
| EVAL-B16-REL1 | P1 | typed relation source role | filed-design | 扩充 relation row typed source_role 与 principal/auxiliary 投影，跨 relation kind 共用 |
| EVAL-B16-TRMV1 | P3 | model endpoint arithmetic | filed-model-variance | 暂不施工；跨 case 重复后再设计 typed edge-duration claim |
| EVAL-B16-REL2 | P2 | relation exploration churn | filed-audit | 在 REL1 之后审计 dispatch/complete typed reason，不能以 case/path 特判压缩 |
