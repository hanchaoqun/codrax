# r522 人工审计：多引用生产采用与写模式对照

- 基线：`main@a72a9d72b`
- 二进制：`0.1.20260815`，revision `a72a9d72bb55`
- 并行度：严格 `2`
- 案例：`qf_relation_subagent_registry`、`patch_python_typo`
- Runner：`2/2 PASS`
- 人工：`1 pass / 1 partial`

## 1. `patch_python_typo`：pass

计划只修改 `main.py` 中 `retrun -> return` 的一行，`kind=patch`、目标路径与三条验收标准正确；没有
扩大文件范围，也没有把计划阶段冒充 apply/verify。61 秒完成，无成文重试、JSON 恢复或固定年龄流降级。

## 2. `qf_relation_subagent_registry`：partial

事实结论正确：默认注册总数为 `1`，成员为 `explorer`；注册调用与 `Name()` 返回位置均在模型推理和
正文中出现。B844/B849 的 typed relation authority 继续稳定，未回归旧的
`No evidence-authorized principal relation member_set` 或弱证据 caveat。

但 B848 尚未获得生产闭环。Finalizer 明确提交两条 citation：

- `internal/agent/subagent.go:64`：`r.Register(NewSubExplorer(deps))`
- `internal/agent/sub_explorer.go:33`：`return "explorer"`

同一个 `explorer` item 仍只填写 `citation_ref=1`，没有使用新 `citation_refs[]`。归一化之后两个原始 pool
slot 均被裁，条目只保留系统从 typed label/evidence 重定位的 `sub_explorer.go:32`；注册引用只在第二轮
count patch 中重新 append。说明 full/patch/lifecycle carrier 已可用，但当前 JSON 教学没有让模型稳定采用。
不能用硬门强迫每个条目多引用，也不能按“注册/Name”列名或答案词面自动补锚；下一步应强化通用、就近的
soft shape teaching：当模型自己已选择多个独立证据来支持同一 item 时，首锚放 `citation_ref`，其余放
`citation_refs`，不要提交未绑定 pool citation。

## 3. B846 再次确定性复现：系统铸造错误 scalar 引用

第二轮为了补 typed `count` 维度新增 scalar `1`，模型把 append 后的注册引用设为 `citation_ref=1`。
`normalizeScalarLiteralCitationRefsWithContext` 没识别该 `1` 来自 accepted principal `member_set.value`，
而是在全 evidence 中按裸数字匹配挑出 `internal/agent/explorer.go:19917`：

```text
if dot := strings.LastIndex(p, "."); dot > 0 && dot < len(p)-1 {
```

该行与默认注册总数无关，却进入 scalar 行和最终引用区。这是系统生成的错误证据身份，不是模型波动，优先级
高于继续扩充多引用教学。最优根修是让 source-literal citation normalizer 对 typed derived aggregate
fail-open：当 scalar exact value 已由 accepted principal aggregate fact 提供时，不再跨全 evidence 按同值
搜索源码 literal；保留模型引用或由 aggregate 自己的 typed support/authority 车道复核。普通源码常量的
definition/assignment/return 精确重绑继续保留。

## 4. 过程与不变量

- Registry 161 秒结束；无 finalizer reject，只有一次 typed requested-dimension 补齐，不是矛盾合同。
- 没有畸形 JSON、旧稿恢复、空答案或 active-stream 固定年龄降级。
- 本轮不是 Trace 案例；实现批和回放均未改显式时间窗、Trace 因果投影、自动补齐或根因选举。
- 主因继续只允许 typed on-chain；邻近/背景 support-only；系统不替模型写事实或结论。

## 5. 状态与下一批

- `B848-MULTIAXISTABLEROWCITATIONCARDINALITY1=carrier-implemented/production-authoring-partial-r522`
- `B846-PATCHCITATIONIDENTITYREMAP1=P1-confirmed/r517+r522/system-false-citation`
- 下一批顺序：B846 typed aggregate carve-out → B848 就近 soft teaching → 同两案 r523 生产回放。
