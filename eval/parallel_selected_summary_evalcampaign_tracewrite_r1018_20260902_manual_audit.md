# Selected Eval Manual Audit Scaffold

- date: 2026-09-02T07:11:58Z
- sweep_start_ts: 20260902-001157
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260902-001158 | write_apply,write_patch_oracle | none | 170s | 27 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass with prose advisory | 只改实现；整数值 float 归一为 int、非整数 float 拒绝；1 个行为探针与 4 个原有 unittest 全通过，计划/变更路径/累计验证一致。模型把 TypeError 的发生点误说为 divmod，实际为后续 date.replace；不影响补丁正确性，不增硬门。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260902-001158 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 251s | 49 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 因果投影/双轴/业务 span/小项与背景边界在场；正文有量纲、CPU/总量矛盾及列头丢失；合法多候选旁路被摘要文本误判重复，root JSON 变 unavailable。 |

## Human Audit Checklist

## r1018 人工复核（2026-09-02）

固定 revision `2fe11db75bab`；仅两路并行，未增第三个模型任务。B1553 已推送后才构建快照。
本组均未走列表候选哈希 ID 修补分支，所以只作为异构无回归/发现新缺陷的证据，不称为 B1553 重试下降对照。

### Trace：投影保住，正文与旁路不能判为通过

答案 `.codrax/output/20260902-001607.490-55380.md`；日志 `run-1.logs/codrax-20260902-001200-000-55380.log`。
4 个 trace_query；明确根因问题，系统正常构造投影而非把状态查询强制升级。主值 65.912ms 供给缺口、36.757ms D 等待、
49.656=0.033+49.623 二分、未入榜 7 条链上与 5 条邻近、19 条未计价占用、业务 span 族均在，
JIT 无链证时仍只给关系未证，不替它铸根因。默认 `.root-causes.json` 必定产生合法 139-byte JSON，但状态 unavailable。

**B1555 / P1 / 系统确认**：首稿选择 12 个不同、已发布的 candidate_id。第 1 项目标自身供给与第 3 项 render_service 供给
均被 binder 格式化为短摘要 `running阶段高负载`；`NormalizeAndValidateTraceRootCauseReport` 用 Summary 作为唯一去重键，
因而在日志 2536 将第 3 项错误判为第 1 项重复，丢弃整个旁路。真正重复 ID 已由 binder 单独校验，不能再用显示词替代身份。
这不是“模型没有选择”或“无链上依据”。修向：冻结候选身份负责有序选择唯一性，显示摘要不负责身份；不自动挑候选、不改正文。

复核更正：初记按正文误写为“供给不足”；实际 compiler 使用 running 的 cpu_work lane，报告映射成 phase_high_load。
真实 roster/registry 重绑定暴露 **B1557 / P1**：供给折算缺口不能被当作高负载阶段占用，非 IO D 也不应直接标为 IO 阻塞。
必须补值口径到报告类别的 typed 链路；B1555 的“不再丢选择”不等于这个语义问题已解决。

**B1556 / P2 / 系统教学与容错确认**：第二轮沿用 full-emit 的 `trace_root_causes`，patch 实际字段是 `replace_trace_root_causes`。
共享上下文只教 full 形，patch 将旧名与外层 schema_version 作为 unknown 字段隔离（日志 2588–2589）。
需对齐两种发射形；只在 canonical 字段缺席且旧形无歧义时移动相同模型载荷，再走原有严格候选验证。
不能用“字符串可解析”作为自动补选或放宽候选依据；无效/冲突仍保留正文与最后有效旁路。

**模型/展示残余，留观不硬拟合**：

- 原表 7 个列名在 patch 中被漏抄，renderer 回退“列 1…列 7”；该轮本可用已发布 add_facet_id 局部操作，
  模型却整体换表并再次遗漏所要求的 member_set。当前提示已明确完整复制 columns，先不加自动正文继承或新拒绝臂。
- 首段将 74.915ms running 说成最大自身状态，而 sleep=118.586ms；并把运行描述成 D 等待来源，无因果证据。
- 同节先正确写 CPU0 running=3.829ms，后把政策影响字段的 0ms 套成 CPU0 无运行；CPU1/2/3 的频率量级写成 920/840/920 kHz。
  原始 CPU 清单和频率/政策影响口径在最终上下文均有，不应扫描成文后由系统替改。
- 写“全部 12 席”，但上下文和投影已声明枚举未完整；把 1.396ms 未归账加进后仍写总和 231.794ms，实际应为 233.190ms。
- 3.077+2.247=5.324ms 则来自真实 typed 同方向非重叠小计，是正确消费，不误记为模型任意跨线程相加。

首稿无硬拒，1 次 answer-dimension 软补充/1 次 patch；正文错误不是“反复成文校验”产生的。
后台活跃流持续接收语义输出，没有因为无可见正文时长触发降级。

### Python 写模式

已审计 `refs/codrax/applied/plan-1788333247417627000-55384`（`e5dc15a0b032`）及 report JSON。
仅 `relativedelta.py` 改动 11+/2-；测试文件原样。`_normalize` 对整数值 float 转 int，对分数 float 及非 int/float 值抛 ValueError。
验证实际执行行为探针与 `python3 -m unittest "test_relativedelta.py" -v`，5/5 通过；changed_path 为
`covered/project_runner/target_behavior`、worktree audit clean，最终 typed 必需义务 1/1；未将 6 条自然语言验收冒充 6 条独立执行。
一次 plan 缺 end_line 精确反馈后修正，无 replan、假绿、测试期望篡改或系统代写答案。

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
