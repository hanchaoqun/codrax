# r523 人工审计：聚合值分域与多引用教学生产回放

- 基线：`main@73f35a807`
- 二进制：`0.1.20260815`，revision `73f35a8078b9`
- 并行度：严格 `2`
- 案例：`qf_relation_subagent_registry`、`patch_python_typo`
- Runner：`2/2 PASS`
- 人工：`1 pass / 1 partial`

## 1. `patch_python_typo`：pass，附一条低优先级 churn 观察

最终计划只修改 `main.py` 第 20 行 `retrun -> return`，路径、patch 和验收边界正确，无范围扩张。

Planner 前两次仍为可选 probe 付出拒绝：第一次只有 `import main`，第二次只打印成功文本；第三次才给出明确失败
exit 与 `expected_stdout`。工具 fail-closed 正确，且 prompt 已明确“本地语法修复优先项目 runner、probe 可省略”，
因此先记 `B850-PROBEOMISSIONGUIDANCECHURN1=P2-observe/model-following-variance`，不为本例增加新硬门。

## 2. `qf_relation_subagent_registry`：事实 pass，引用完备性 partial

结论仍正确：默认注册总数 `1`，完整成员只有 `explorer`。没有旧 relation authority caveat，也无 finalizer reject。

Finalizer 明确选择并提交两条 citation：

- `internal/agent/sub_explorer.go:33`：`Name()` 返回 `explorer`
- `internal/agent/subagent.go:64`：默认注册调用

但同一 table row 仍只发 `citation_ref=0`，没有发 `citation_refs[]`。归一化随后把两条原始 pool slot 都按 unused
裁掉，再仅保留一个 `sub_explorer.go:32` typed member support。最终正文虽同时声称 Name 返回与默认注册，引用区却只
覆盖 Name 返回，注册证据没有结构化绑定。因此 B848 不能关闭。

## 3. 新确认根因：同一 finalizer prompt 内的 citation carrier 教学漂移

本轮已排除“字段不可用”：full schema、tool description、解析器和 renderer 都支持 `citation_refs[]`。真正冲突在后置
静态 finalizer instruction：schema-near 规则已讲 primary + additional refs，但 workflow、Output Format 与动态
enumeration checklist 仍多次只说“引用只放 `items[i].citation_ref` / 每项设置 `citation_ref=N`”。模型按后置、重复的
旧单数形输出，形成系统自身教学竞争。

最优方案是建立一个共享 item-citation carrier teaching，并让 schema-near、静态 skill 和动态 checklist 使用同一语义：
单锚用 `citation_ref`；同一 item 已由模型选择多个独立锚时，主锚留 singular、额外锚放 `citation_refs`；不重复、
不添加未选择锚。scalar/decision 的单锚例子继续保留。不能添加“每行必须多引”硬门，也不能从表头、用户输入、答案
prose 或未绑定 citation pool 猜关系。

## 4. B846 回放边界

错误 `internal/agent/explorer.go:19917` 引用未再出现。但本轮模型把总数写在 table text，而不是 scalar block，故
source-literal normalizer 没有命中；这能证明无回归，不能替代新增 production seam 的确定性 pin。B846 记为
`implemented/static-production-seam-pinned`，后续遇到真实 principal aggregate scalar 时再补生产观察，不为构造覆盖而
硬改答案形状。

## 5. 过程与不变量

- Registry 134 秒、write 67 秒；无畸形 JSON、旧稿恢复、空答案或 fixed-age stream 降级。
- 本轮不是 Trace 案例；没有改显式时间窗、Trace 因果投影、自动补齐或根因选举。
- 主因继续只允许 typed on-chain；邻近/背景 support-only；系统不替模型写事实或结论。

状态：

- `B846-PATCHCITATIONIDENTITYREMAP1=implemented/static-production-seam-pinned/no-r523-scalar-fire`
- `B848-MULTIAXISTABLEROWCITATIONCARDINALITY1=P1-partial/prompt-contract-drift-confirmed`
- `B850-PROBEOMISSIONGUIDANCECHURN1=P2-observe/model-following-variance`
- `active-stream-fixed-age-degrade=forbidden/not-observed`
