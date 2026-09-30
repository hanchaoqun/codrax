# HMC §200：运行时唤醒关系、睡眠依赖与图格式人工审计

- 2026-09-30；固定批次`20260930-005849`，两例并行、各一次；runner 47408 exit 0，不追加第三例。
- live快照`98dc22252`，二进制SHA-256 `09588a04891dc0ca485cbc3d4dbbb277adfe7c07c937cebeac93d943f5c5e9b7`。其后event_search接入及格式修复不是这两份live答案的版本。
- 机器2/2 PASS，完整人工 **0/2**。阅读primary、完整transcript、原始trace、工具过程及实际finalizer消息；没有用机器关键词oracle替代人工结论。

| case / 原件目录 | 机器 / 人工 | 过程与已正确内容 | 保留的问题 |
| --- | --- | --- | --- |
| `trace_query_sleep_summary-20260930-005850` | PASS / FAIL | 218秒、trace_query 2次、无源码读取、上下文36%、finalizer拒绝3次。timeline + 无PID的event_search；11ms分区、S两次/4ms/均值2ms/最长3ms、IO一次/3ms正确；两次唤醒身份和时间正确 | 图删去真实唤醒边；Note续行语法错误；把target的10.007–10.009运行误述为worker；0.5ms的图时间端点写成10.005；末尾错误指称scheduler_concurrency来源 |
| `trace_query_sleep_dependencies-20260930-005850` | PASS / FAIL | 366秒、trace_query 6次、无源码读取、上下文53%；最终图保四条真实唤醒边，四次睡眠含0.8ms和缺唤醒记录；实际finalizer收到native wakeup图锚。investigation原始reject遥测4，finalizer reject 0 | 用低优先级关系直接写优先级反转候选，实际数据仅结构关系且算力折算未知；0.8ms区间端点写5.029而非5.0288；引用行存在偏移；完整投影把21.8ms总体与10/6/5ms局部账合为42.8ms/104%，仍为系统P1待复现修复 |

## 上下文与系统责任

睡眠摘要的关系缺失不是“模型不会画箭头”：live只用event_search看真实唤醒，初版图provider仅接wakeup_chain；普通发射/修补因此拒绝。修后从解析事件行发布独立`scheduler_wakeup_event`，只证明发生过唤醒，不授链上身份、整段等待归因、优先级反转或源码call。窗口校验以请求半开窗内的事件点为准，不能因检索边界扩展而丢真事件，也不能让窗外点进入答案。实际双查询＋bounded/named分类消息回归覆盖了这个接缝。

递归例的finalizer消息在原日志2970行出现“Available runtime diagram anchors”；状态、四段区间、低优先级关系以及`fold_basis_unknown`均到场。探索摘要却把低优先级关系写成“根本原因”，与typed权威并置会污染后续答案。此问题挂16.4，后续应处理证据化交接与摘要权威分层，不靠扫描模型散文硬拒，也不能把一次错误直接定性为随机波动。

投影42.8ms在系统附录而非模型主文，不能归咎成文波动；完整21.8ms与10+6+5ms存在重叠口径。挂04.5/16.4优先修，需公开多query/总体与发生段证据复现并证明去重权限，不以百分比截到100或直接取最大掩盖。

## 用户反馈的图格式

用户先提到`20260930-010454.062-77012.md`，随后粘贴target-41片段。核对本地原件，该片段实际位于`20260930-010225.691-77011.md`；两份均保留，不混同诊断。

- 77012原始sequence在内置Mermaid.js可解析、渲染；图上多余引号来自系统通用源归一化，而原始模型输出未带引号。现将该兼容步骤仅留终端内部。
- 77011有5条物理续行脱离Note；原始浏览器复测明确NEWLINE解析失败。通用修复折为`<br/>`；不吞消息、声明、控制块、空行/注释、降缩进或有歧义的语法；不改变时间、标签文字和关系。
- Markdown、typed diagram、在线HTML、独立HTML及终端使用同一修复；有幂等和拓扑不变负控。浏览器修后成功，SVG 23245 bytes，11行Note文字保留、消息仍为0；这不代表缺边/时间错值已通过。
- 修复副本：`.codrax/output/20260930-010225.691-77011.format-repaired.md`及同名HTML。原始md/html/sidecar与eval原件未覆盖，副本不持有重新签发的答案验收凭证。

## 版本、收据与下一步

生产片：`42041c9b6`原生链图权威，`d96678aac`递归/缺边/容量公共回归和自然问句eval，`98dc22252`时间精度，`1ca519327`事件查询图权威及集成登记，`b6093e746`跨输出Note格式修复与终端引号隔离。末版验证统一见主账本§200。

| 原件 | SHA-256 |
| --- | --- |
| summary `run-1.primary.md` | `3779003427c8e7fc3864886a31e13e03fd0a7a55e5bbe6d25601c29e94dee868` |
| dependencies `run-1.primary.md` | `11339897ae462ea8fe0112e0fb1d8d9fa0f47bf1235088e27d92ad8d2ecf092f` |
| `/tmp/codrax-hmc200-browser-before.json` | `914152c28e85735f419f98536ffe0f3d6074538ad8878e0b349898b3b831bc00` |
| `/tmp/codrax-hmc200-browser-after.json` | `92fa0ebeb9c49c7f4a0bd0260baa526c70a80f452bd25694fe6ceb93aefd7854` |

完整能力累计16/79、63开放；本批三组子能力/修复，完整答案仍2份未通过，五个稳定验收父项不销。下一轮能力轨转17.7二进制剩余接入，缺陷轨优先总体/局部投影重复计量；18.5只读登记重启恢复保留。事件视图及格式修复仅确定性公开回归通过，后续按批轮换live，不重跑第三例求绿。
