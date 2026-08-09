# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T01:05:31Z
- sweep_start_ts: 20260808-180529
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-180531 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 139s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | compact typed guidance 生效且无通用 Binder/IO 大清单；模型仍把 span-name role 当 UI/RenderService/GPU owning-thread role，并扩写输入处理、渲染工作量、GPU 绘制完成等内部机理。另把 4 个 span duration 之和 37ms 当 pipeline 端到端总时长，甚至写成不存在的 `7.040-7.003`，与 typed 附件包络 40ms 冲突。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-180531 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 42 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B394 生效：604.528/551.600 不再被判高/低。#3 近尾席正确为 caller absent，但模型仍把同 subject 的 #5 fscache/IRQ/hmfs 机理重新绑定到 #3 代表窗，并把 lower-priority dependency candidate 写成已发生反转、导致 post-wakeup CPU delay；frame absent/unproven 与正文根本原因表述冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict

- Runner `2/2 PASS`，人工 `0/2 PASS`。两案均第一次成文、`finalizer_reject=0`、无系统 rewrite；frame 案有一次结构化 blocks string recovery，但答案完整，不是本轮失败根因。
- S37dc 的选择臂确实生效：Finalizer prompt 出现 `Typed trace context precedence`/span authority，旧 Binder/IO/perf/root-cause 通用清单不再重复。Donghu final tail 精确出现 #3 caller absent、aggregate absolute level absent 和 typed-on-chain population。
- B394 可关闭：模型没有再把压力分数叫偏高/较重，只保留“系统背景”；是否间接影响仍应由链上关系限制。
- B392/B393 只能判 partial。失败不是新 hard gate 或答案 mutation，而是更早的 typed producer 与模型探索 carrier 本身仍给出混淆信号。

## New/deeper generalized gaps

1. `EVAL-B395-FRAMEROLEAUTHORITYSOURCE1=P1/HIGH`：`classifyFrameTimelineRoleAuthority` 仅凭 span/name 语义把 ui/render_service/gpu 铸成 `kind=thread_role`，并可硬锁 scheduler target；这与“marker/stage role 不证明 owning-thread role”的最终 guidance 自相矛盾。最优方案是拆分“分析锚选择”和“线程角色结论”：name-derived 角色只能是 `pipeline_stage_role`，可在唯一时作为 frame-stage navigation anchor，但不得发布 thread_role。
2. `EVAL-B396-FRAMEMEASUREMENTCALIBER1=P1/HIGH`：模型把 37ms span duration sum 与 40ms first-start..last-end extent 混成一个“总时长”，现有 authority 没有 typed 同位区分 extent/union/sum/gap。应从 typed frame item intervals 计算四把尺并近席发布；不解析 aggregate label 或模型 prose。
3. `EVAL-B397-TRACESEATCLAIMENVELOPE2=P1/HIGH`：现有 candidate/caller 分散负权限仍被模型同页违反。应给每个最终 leader 合成一个紧凑、枚举式的 `allowed_mechanism_scope` 与 `not_authorized_mechanisms`；candidate 席明确仅为 downstream wakeup 前 dependency supply，D-state 未证席明确禁止 sibling caller、IRQ/storage cause、resource/holder identity。系统不选根因，只约束证据能支持的机理强度。
4. `EVAL-B389` 仍 partial：通用清单减少后 frame context 仍约 59.8K、Donghu 83.4K，长 Trace Decision Inputs、Observation Ledger 与 model aggregate facts 仍重复。后续先修 producer 权限自冲突和同位尺，再按 typed provenance 设计进一步压缩，不能删除精确证据或靠答案关键词门控。

## Preserved invariants

- Donghu 的确定性 Trace 因果投影、显式窗、自动补齐、唤醒链、根因排序和双轴均完整；主因人口仍为 typed on-chain only，邻近/背景没有被系统加冕。
- frame causal relation 仍为 temporal/unproven；没有固定 60Hz/jank verdict。
- 不扫描用户问题、thinking、summary、final prose；不按 Choreographer/fscache 等单词拟合；不由系统删除或替换模型结论。
