# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T10:33:57Z
- sweep_start_ts: 20260818-033355
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-033357 | answer_regex,answer_contains | none | 87s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 三个平台与 `cmd_sleep` 主结论正确，但成员复合载体消费了裁剪证据视图：macOS 组错误带入 POSIX 39 行，POSIX 无组合行；终稿把 `cmd_sleep` start/duration 引用错挂到 clock.c:37/17，且只保留 5 条 citation。B1064 第二层已按完整 accepted evidence 修复。 |
| 2 | read_combo_loose_multi_question_units | FAIL | eval/results/read_combo_loose_multi_question_units-20260818-033357 | answer_regex,answer_contains | none | 545s | 47 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=6/0,fin_reject=2,unavail=0,prune=3 | partial | 最终答案实际同时保留配置表与 Mermaid 独立章节，runner 的跨行 regex 没有匹配，属 oracle 假阴性；但配置默认值表述错误、Mermaid 语义过度简化。主要耗时来自 678 行未读时被 realign 到同端点 653，completion 修补债无法清除；B1067 已根修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

### 1. `sr_c_platform_fork`

- Runner PASS 不能签人工 pass。模型找全 Windows、macOS、POSIX 和唯一命令处理函数 `cmd_sleep`，
  但 `member_note_composite_support` 在 Finalizer 输入中显示：Windows=`11/15/17`，
  macOS=`25/28/30/39`，POSIX 缺席，`cmd_sleep=29/31/32/34/37`。39 行属于 POSIX，说明第三个同名
  definition 被 prompt evidence 容量裁掉后，incarnation 分区在不完整视图上运行。
- 根修 B1064 不改分区语义，而把证据源改为当前切片、Turn-A artifacts 和 Mutable accepted
  evidence 的 stable-ID 合并；后到 accepted snapshot 只覆盖同一证据的 metadata。它不铸造关系、
  行为或结论。
- 最终正文的平台说明正确，但调用链三行存在 citation 归属错误：start 行挂 POSIX 定义，duration
  行挂 Windows counter；14 条提交 citation 只有 5 条入册。源码定位/输出维度两个系统补充还使用
  偏内部术语，且“主路径关系未完整呈现”与正文已有调用链不协调，继续作为显示债，不以答案词面门
  删除。

### 2. `read_combo_loose_multi_question_units`

- Runner FAIL 原因是单个 `.*` regex 默认不跨换行；最终答案确有“运行时配置”与“REPL 中 Mermaid”
  两个独立章节。因此 runner verdict 是 oracle 假阴性，不是空答案或两问丢失。该 oracle 只记低优先
  测试债，不为此改生产行为或拟合标题。
- 人工仍为 partial：`RuntimeSettings` 指针字段不是“零值即代码默认值”，其作用是区分 omitted 与
  explicit zero；默认值来自别处。`resolveRuntimeKnobsForApp` 和文件缺失行为也缺精确 citation。
  Mermaid 部分把 `degradedMermaidSourceObviouslyMalformed` 描述成独立展示策略，语义过度简化。
- 545s/27 Explorer 的主因不是模型随机波动。模型按 repair 精确重发 `init -> IntVar` 的 653、678
  两个调用点；但只读了 649–663。旧行号纠偏把未读 678 移到最近已读的同端点 653，stable-ID 随后
  合并，completion ledger 仍要求 678，形成确定性重试环。B1067 要求原提交行也在严格读集后才允许
  realign，未读坐标不再借兄弟行的 line-text 权威。
- Finalizer 的两次拒绝不是矛盾合同：第一稿确有两个 summary；第一轮 patch 只新增 section、忘记
  删除旧 `mermaid-summary`；第二轮用 `remove_block_ids` 正确删除后通过。JSON/patch 教学本轮可执行，
  无畸形 JSON、无旧稿恢复、无空答案。

## 红线与后续

- 两项修复只消费 typed evidence/source coordinates，不扫描用户原始输入、模型 thinking/summary、
  最终答案、case 名或语言关键词，不接管模型结论或图关系。
- 本批未进入 Trace 路径；显式时间窗、Trace 因果投影与自动补齐保持，根因仍只能来自 typed
  on-chain，邻近/背景仅作额外排查，实际占用与规则可消量双轴不互相替代。
- 活跃流没有 fixed 4ms 累计年龄降级、空答案或传输恢复。下一轮仍恰好并发 2 个相同案例，验证
  B1064/B1067 的生产闭环；配置语义若在证据链恢复后仍错误，再按 typed context gap 立案，禁止
  用 case 文案硬化。
