# r524 人工审计：多引用单源教学与写探针省略回放

- 基线：`main@65df90844`
- 二进制：`0.1.20260815`，revision `65df90844418`
- 并行度：严格 `2`
- 案例：`qf_relation_subagent_registry`、`patch_python_typo`
- Runner：`2/2 PASS`
- 人工：`1 pass / 1 partial`

## 1. `qf_relation_subagent_registry`：事实正确，多引用仍未生产采用

答案的集合事实仍正确：默认注册总数为 `1`，唯一成员是 `explorer`；没有 finalizer reject、畸形 JSON、旧稿恢复或
空答案。模型提交了三条 citation：注册调用 `subagent.go:64`、`Name()` 定义 `sub_explorer.go:32` 和返回值
`sub_explorer.go:33`。

但 table row 仍只有 `citation_ref=1`，没有 `citation_refs[]`；summary 的 citation carrier 又只指向返回值行。
未被任何 item 引用的注册调用因此按 `unused_pool_entry_pruned` 清理，最终引用区只有 `Name()` 定义与返回值，
没有覆盖同一可见行里“通过 `RegisterDefaultSubAgents` 注册”的独立事实。共享教学已在同一 prompt 中唯一出现，
故旧的静态/动态 singular 合同漂移已经排除；B848 从“提示竞争”收窄为“模型仍把 citation pool 误当文档级背书”。

最优下一步仍是软的运输语义补强：明确 pool slot 本身不为正文提供全局背书，未绑定 slot 会被清理；相邻 block 的
引用不能替当前 item 的事实背书。不能扫描表头/正文词语自动补锚，不能把未选择证据塞入数组，也不能新增“每行必须
多引”的内容硬门。

## 2. `patch_python_typo`：计划正确，B850 升级为确定性上下文缺口

最终计划仍只修改 `main.py` 第 20 行 `retrun -> return`，范围和验收项正确。但 planner 首稿再次为本地 Python
语法修复添加 `import main` probe；probe 没有 assert/异常出口或 `expected_stdout`，被正确 fail-closed 拒绝，第二稿
删除 probe 后通过。

这是连续两轮生产复现，不再记单次模型波动。现有教学只在 typed `test_surface` 已广告“原生项目 runner”时建议省略
probe；该最小仓没有 pytest/unittest runner，却有 verify 阶段系统拥有的 changed-file syntax preflight / `py_compile`
fallback。Planner prompt 未明确交付这条能力，模型因而合理地认为必须自己造 import probe。最优修复是把“本地纯语法/
解析修复由 verify 阶段语法预检兜底，可省略 probe”写入统一软教学；不放宽 probe 校验，也不硬删模型 probe。

## 3. 不变量

- 本轮不是 Trace 案例；没有改显式时间窗、Trace 因果投影、自动补齐、链上根因选举或背景分层。
- 不扫描用户原文、模型思考或最终答案正文作硬门；系统不替模型选证据、写事实或改结论。
- 活跃流没有按 4ms/4s/4m 或累计年龄降级。

状态：

- `B848-MULTIAXISTABLEROWCITATIONCARDINALITY1=P1-partial/prompt-single-source-positive/pool-global-mental-model-open`
- `B850-PROBEOMISSIONGUIDANCECHURN1=P2-confirmed/context-capability-gap`
- `B846-PATCHCITATIONIDENTITYREMAP1=implemented/static-production-seam-pinned/no-r524-scalar-fire`
- `active-stream-fixed-age-degrade=forbidden/not-observed`
