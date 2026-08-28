# r905 人工审计：配置 scalar 所有权与多成员集合汇合

- 时间：2026-08-28
- 冻结二进制：`./.codrax/tmp/codrax-selected-20260828-141826`
- 并发：恰好 2 路
- 用例：`qf_config_precedence`、`qf_multi_member_set_count_caveat`

| 用例 | runner | 人工结论 | 核心判断 |
|---|---|---|---|
| qf_config_precedence | PASS，152s | partial | B1407 生效；默认值 50 不再被当作优先级集合基数，但解析/合并机制措辞和引用关联仍不完整 |
| qf_multi_member_set_count_caveat | FAIL，375s | fail | 动态基准为 5 个公开函数，答案只列 4 个并漏掉 `SetExternalArtifactFloor` |

## 1. B1407 生产回放结论

配置答案仍显示代码默认值 `50`，但日志中已无 `visible_count=50` 或把它与 5 个优先级成员对账的错误。显式成员集合的数量校验仍在，说明
“普通 scalar 不借位 aggregate count、真正 count 继续校验”的边界生效，B1407 可记为生产正证。

该答案仍不是人工全绿：正文把 YAML 解码和 `initApp` 合并阶段写在一起，最终引用只保留默认值、flag 注册和示例配置三项；模型已经读取的
`LoadRuntimeSettings`、`yaml.NewDecoder`、`KnownFields(true)`、`Decode(&s)` 以及 `Changed` guard 没有形成完整用户可核对链。末尾还称
`mergedMaxSteps` 未被当前证据确认。B1408 的“模型自声明 owner 不能成为语义权威”、B1409 的 block/citation 关联和 B1396 的已解决证据上下文
仍保持开放，runner PASS 不能替代人工判断。

## 2. B1410：已验收 principal member_set 被后续窄标签静默覆盖

该失败不是动态 oracle 漂移。仓库命令与源码均给出 5 个 production exported function：`Eval`、`EvalAll`、
`SetExternalArtifactFloor`、`IsRegistered`、`RegisteredKinds`。过程中的前两个调查单元也分别提交了同一 5 项集合及逐项当前源码支持；第三个调查单元
只读取 `eval.go` 前段后，提交了 4 项集合，漏掉位于 `eval.go:1126` 的 `SetExternalArtifactFloor`。

系统已有 monotonic carry-forward 与“严格超集优先”仲裁，但 carry-forward 只接受标签逐字相同。前三次标签分别为
`公开函数（function，不含测试函数）`、`公开函数（排除测试函数）`、`公开函数（function）`，最后一份因此绕过旧集合，稳定事实池被 4 项末值替换。
Finalizer 的上下文同时仍有 `SetExternalArtifactFloor` 的 grounded definition row，却只收到 4 项 principal roster，最终按错误硬 roster 发射 4 项。

这属于 `B1410-PRINCIPALMEMBERSETSUPERSETLABELDRIFT1/P1`。根修不由系统补第 5 项：只有当 later set 是 accepted set 的严格子集，kind/role/unit/
dimensions、结构化标签族和逐项支持均兼容时，accepted superset 才继续进入已有仲裁；交叉集合、等大不同集合、无支持集合均不猜。模型先前提交的 5 项仍是
唯一内容来源，系统只防止它被后续窄标签静默丢弃。

## 3. B1411：完整 30 行仍被软计数器读成 28

最终表格实际列出 30 个 Kind 常量，summary 也明确写 30，日志却连续产生
`label="Kind 常量" expected_count=30 visible_count=28`，并在答案末尾追加“部分项的证据支持稍弱”。这是独立的
`B1411-STRUCTUREDROSTERCARDINALITYUNDERCOUNT1/P2`：完整 structured roster 已在场时，软计数器仍用局部可见文本投影出 28。它没有造成本轮 runner
主失败，但会制造错误 caveat 和修补心智。后续应直接消费同一 block 的结构化 row ownership/cardinality，而不是扩大文本数字或成员名扫描。

## 4. 边界与下一步

- 没有读取用户请求、模型 reasoning、最终答案或 Mermaid 文本来铸造新硬门。
- 没有系统生成成员、关系、结论、措辞或布局。
- 本批不涉及 Trace 运行路径；显式时间窗、因果投影、自动补采、链上根因和背景隔离保持不变。
- 顺序：B1410 全测并提交 → 恰好 2 路生产回放验证 5 项集合与 B1407 → B1408 语义 owner 根修；B1411/B1409/B1396 按证据强度排后。

状态：`r905=runner-pass-1/fail-1,human-config-partial+enumeration-fail`；
`B1407=production-positive/core-closed`；
`B1410=confirmed+implemented/full-suite-pass+build-pass/pending-replay`；
`B1411=confirmed/P2`；
`B1408/B1409/B1396=open`；
`system-answer/conclusion/member/relation-selection=none`；
`request/model/final-prose/mermaid-content-new-hard-scan=none`。
