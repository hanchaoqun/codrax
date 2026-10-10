# HMC §223 固定双例人工审计

- 日期：2026-10-10；冻结实现：`4757a43cc3c6`，构建 dirty 仅文档。
- 快照：`codrax-selected-20261010-025404`；并行2、每题1次、超时1200秒。两道自然问题原字节未改，不追加第三例求绿。
- 原始根目录：`eval/results/hmc_named_native_handoff_20261010`。机器进程19405正式exit0；独立审计与主代理复核均完成。首轮全仓与后续测试迁移另在统一账本登记，不倒签此次live版本。
- 完整父项新增0，累计20/79、59开放；原生取数/事实交接子能力与整份答案验收分开计数。

| 用例 | 机器 | 完整人工 | 秒 | 已证进展 | 未通过内容 |
| --- | --- | --- | ---: | --- | --- |
| trace_dual_measurement_records | PASS，实际成功结果头 | FAIL | 167 | 命名输入真实准备，双侧13+8条进入最终原生表 | 正文按名称合并独立序列，写成4/3组而非7/4组 |
| log_shared_sources | PASS，原有调用/附件门 | FAIL | 291 | 实际9条原生事实、来源与原值齐全，最终不再断言missing导致abort | 模型表漏孤立行；同请求/同资产/同时间段越权；重叠原生表重复展示 |

## 1. 双侧量测：取数和原生展示通过，摘要不通过

目录：`trace_dual_measurement_records-20261010-025405`。以`run-1.primary.md`及完整`run-1.answer-transcript.md`为最终答案，不把探索thinking或早期草稿算成最终缺陷。

原日志`run-1.logs/codrax-20261010-025407-000-61407.log`第897–923行显示命名`.data`实际查询，窗口分别1–2及4–4.5秒，两次原生build后ToolResult成功。随后两个探索分支各重复两次查询，总计6次trace_query；仍有最初两次二进制read_file、受限exec和无效emit_evidence的噪声，但不再发生§222七次动态工具缺席拒绝，未从邻近SQL冒认数据库事实。

准备收据分别把两原件映射到独立tracebundle。原件SHA256重算与收据一致：baseline `66dfd3c7d271bce9c2f43e965cafd395f09cfd0143532eaa4f479d644f2f0eef`；current `8dda4b0563338715f7ce67b8238077219502125094baaa2cb023216d9ca961ab`。实际载荷`trace-query-result-a058a494.json`/`trace-query-result-79756e85.json`均available，分别13/8条、omitted=0、unpositioned=1。

最终两张原生表完整保留13+8条：filter10/20同名不同身份、carry-in原区间和裁剪区间、INTEGER/REAL/TEXT/BLOB、0/NULL、9007199254740993与9007199254740995、负持续和NULL持续的未知终点、右界排除与无法定位记录。最终没有MHz/活动状态或响应根因推断。每侧available和跨工件时钟边界均保留。

但最终`primary.md:1`写baseline“4种过滤序列”、current“3种过滤序列”，是名称级概括覆盖独立身份。实际baseline为7组（5个唯一引用+2个未决记录组）、current4组。finalizer上下文第2529/2542行已有完整7/4组与13/8成员，故不是字段截断或预算漏投。首次emit第2683行成功，patch第2736行保留summary，错误进入最终接受稿。原生表正确不能替正文签PASS。摘要还把未知/无效持续概括成“时间点而非区间”，应保留“观测在此时刻，终点未知”的区别。

此次live采用独立single queries，不宣称命中comparison所有出口。实际pair13+8+2状态的完整Run见确定性公开测试；准备期坏一侧隔离、全局terminal与原请求逐源窗口绑定仍未完成，10.1不销账。

## 2. 双源日志：事实齐全，解释与重复展示未闭环

目录：`log_shared_sources-20261010-025405`。原日志`run-1.logs/codrax-20261010-025407-000-61408.log`第1365行真实log_query成功；载荷`log-query-result-731ccbc8.json`保留9条（app6/kernel3）。逐源后续查询另返回3和6条，中间一次参数错误为失败结果，不混入成功计数。

原件与来源hash一致：app `3882ff8d7305eface3a2692cace418d3f889086fffde6a4eaba3d6c4344c95ad`，压缩kernel `537220494c94938cd684a6a48a258b7edbce1961d7bcfecc2438f5c453f0b5aa`。原生表保完整源/代次、物理续行范围、未知PID/TID、坏日期/孤立续行/unknown状态和精确boot纳秒9007199254740993；匿名kernel记录未继承相邻身份。

最终emit第2913行、patch第2960行均成功；完整transcript是本节行号基准：

- L16称app6条均“有效”，实际3 parsed、1 malformed、1 orphan、1 unknown；L20称“全部8条可解析”，模型表L43–50漏app L5孤立续行。原生表L56–64完整9条，两者自相矛盾。
- L29称kernel仅storage有民用时间，实际observer也有；模型表混排来源民用时间，不能用未校准时间建立全局序列。
- L31把共同rx17称为同一业务请求；L33把kernel lookup称为同一缺失资产，但kernel没有catalog.bin；L34称共处同一时间段，缺跨源校准依据。最终明确不能确认跨进程因果，较§222不再断言missing导致abort，但尚未完全守住实体/关系边界。
- L35/L50把unknown文本与parsed匿名kernel记录称为孤儿，混淆身份未知和解析状态。
- 原生表按三次查询展示9+3+6=18行，是9个唯一记录的重复，并额外存在8行模型副本。原生表自身保真不等于整份答案去重/解释正确。

系统级过程证据：finalizer INIT第2713–2729行给完整9行preview（省略0）并教“系统自动展示，不要重抄”，但post_emit_advisory第2922–2936行仍要求承载成员的块关联member_set。最终接受patch遂新增8行模型表、正文9改8。优先方案是让系统原生块履行对应typed成员维度，并按精确source generation/record identity处理重叠查询；不是继续补prompt或扫描正文数字。此为原生展示与维度覆盖的共享接缝，挂16.4/18.4。

本次triage接受稿为errors=3/observations=7（原日志315），首稿把observations写为JSON字符串被拒后自行修正。新Facts投影有确定性0/1/2错误正反，不能用本例倒签低错误数分支；Meta.Summary仍含未证明解释，是本批未扩展的同权限族后继。探索稿曾捏造跨源1ms差值，但最终已未保留，不把该草稿当最终失败点。

## 3. 留案与收据边界

未加正文/问题关键词门，未改变用户问题。两份机器PASS原件及两份人工FAIL同时保留；未证明剩余误述为模型波动。下一优先为10.1独立侧准备/terminal与逐源窗口、16.4/18.4原生事实覆盖维度及重叠展示共享交接；不针对序列名称、年份或请求号逐个加规则。

无图、无写模式、无只读登记自然分支，不替旧图或写模式FAIL补签。Trace链上因果投影/自动补齐及活跃流等待策略没有改动。

42份量测工具载荷、26份日志工具载荷与两组完整准备产物已按原字节归档在结果根目录`tool-blobs/`及`prepared-inputs/`，目录diff一致。测试、race、构建与全仓JSONL的原始失败/补救收据存`validation-receipts/`，最终正式状态见统一账本§223。
