# HMC363 — Trace 查询线程身份与原始采样记录位置

`main` 基线 `02c465de560c4ebbf733a37c58e05d87a1a72a31`，源码 `8ecc3596d8ed05b96b1e1affa8997f51d6b75754`。**79 项＝29 完整＋50 开放**（18 部分、28 待实施、2 待验收、1 验收中、1 持续），未改任务勾选。

- **完整能力交付数：本批 0，累计 29。**
- **已交付子能力：2 项。** 当前原生因果名册的查询线程和成员身份交接；直接未压缩原始 perf 的采样树字节定位。两者仅按各自公开回归及完整原件核验交付。
- **剩余验收缺陷：** 两例完整人工失败；旧身份自然复验及直接定位自然分支未触发；常用视图附带树位置缺失，样本状态、系统闭合区间零贡献及不同线程状态的解释仍开放。BRBE 有效边/SPE、只读完整终态、旧 IO/栈/图/权限不销。

## 两轨修复与参考实现

**缺陷轨：** §362 实际 rank 中，查询线程 36435 的两种显示名共用同一 TID/TGID（36435/25820）；另一个原生结果的应用进程为 36434。成文名册只带时间范围和显示名，不能判定应用归属。[原公开失败](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0346-codrax-hmc363-subject-red-v2.log) 中原生工具已执行，4 候选均缺这组身份。参考 `core/preprocess/game_render_ops/bottleneck_scanner.py` 的帧窗祖先遍历按 `itid` 和显式唤醒引用组链，名称另解；`config/indicators/sched/runtime_agg.yaml` 按原生 `thread.itid`/`process.ipid` 归属，时间、名称、线程号是不同筛选轴。Codrax 只复用这个分离设计：查询线程、当前成员与请求里验证过的编号各自携带，纯名称、同窗或探索游标都不凭空证明请求归属。权限不变，链上优先级反转、调度/算力供给、D/IO、确定性语义优化和业务修向不因加入身份被删除；邻近和背景没有新资格。

当前原生值经历史压缩仍按同代保留，实际 Finalizer 名册和旁路从同一候选绑定，模型不能提交或替换这些字段。读取侧只披露确切链的范围，不把缺归属变成硬门或收窄独立的系统量测。既有 JSON 选择器只用 `candidate_id` 和可选描述，额外结构是工具产生的证据，不是新参数。完整网易云原件命中同编号多显示名，提交伪造身份无法改变旁路；旧范围与端点合同保持。

**能力轨：** 参考 `core/extensions/hiperf_binary_parser.py` 的 `build_sample_index` 保存 `(record_header_offset, timestamp_ns, size)` 后按时间排序；后续 `_parse_sample_body` 的采集器/流 ID 是记录属性。独立 [§362 oracle](results/hmc_finalizer_equivalent_20261106_v1/sealed-v1/0234-codrax-hmc362-binary-record-oracle-v2.log) 在完整真实原件找到 164 个不同字节位置，而两组采集器 ID 各复用 82 次。[原公开失败](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0264-codrax-hmc363-binary-red-v1.log) 的两个物理记录在同时间、同线程、同 ID、同权重时缺独立位置。

从已校验记录头获取位置，不二次扫描源；写旧 perftrace 时按实际行序另存至多 65,536 项的内存索引。首版把位置加进旧文本，原「坏记录不改已接纳样本文本」合同四臂失败，保留[原日志](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0312-codrax-hmc363-related-v1.log)，移出文本后原断言通过。位置由确切原件路径/大小/哈希和派生件代次共同绑定，返回前复核原件版本；同字节换代、无准备凭证的派生件、gzip/ZIP 外壳、未产位置的官方路径不借定位权。展示最多仍是每节点 4 参考，不把地址当样本 ID、调度证据或完整人口。

## 冻结验证

- 定向 9 包、289 顶层/232 子通过；race×2 5 包、78 顶层/126 子通过。继续通过原请求预算精确重放 `703751 → 696294 / 700000 B`，没有改变 §360 历史失败。源码/测试/问法/原件冻结后停写。[冻结](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0290-codrax-hmc363-frozen-v1.json) 7673 输入、90 外部、28 源快照。
- 用户目录 75 份 ftrace，实际使用完整网易云 82,838,676 B 与元宝 63,659,046 B 走原生查询→成文无模型对照；额外对网易云整件实际 rank 的线程身份与参考整件二进制 30,062,585 B 的 8 个显示记录头逐一核验原字节。保 164 样本/259 cycles、82/174 与 82/85，源哈希前后一致。[双整件](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0363-codrax-hmc363-whole-real-v2-receipt.json)／[真实线程别名](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0357-codrax-hmc363-whole-net-v2-receipt.json)／[真实字节位置](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0351-codrax-hmc363-whole-binary-v2-receipt.json)。不把这些无模型核验算成第三个真实模型例。
- 唯一全仓 OS0，98 包通过/12 无测试，28,797 顶层/34,659 子通过、62/14 跳过，268,975 个 JSON 行，无失败、重复或未终态；[终态统计](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0291-codrax-hmc363-full-stats-v1.json)。冻结的正式可执行主包已确认；误将 `./cmd` 构建成库的两个预检收据保留，不启动评测。

## 两例真实评测及人工审计

原问、原整件，各 1 次、15 步，同代正式 CLI。机器 **2/2**，完整人工 **0/2**。唯一配对实际请求 A14/B13，3 个歧义组排除，**23 对重叠，最长 19.141 s**；[重叠证据](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0260-codrax-hmc363-actual-request-overlap-v1.json)。

| 原例 | 可核验结果 | 失败或未触发 |
| --- | --- | --- |
| 网易云整件系统并发 | 2 份同一总体、线程时序与进程概况；26 个汇总值、123 个分布格正确，Markdown/HTML 16 表逐格相同 | 没有 rank 或候选字段，自然未触发。调查把 53.97% 写成几乎一直满载并实际投递；正文把闭合零贡献写成无活动、把 D/IO 为零写成不等 CPU |
| 参考仓完整 sample-only | 附件原件/查询/预览大小各自保真；同窗 164 记录/259 cycles、两单帧热点；Markdown/HTML 5 表一致，因果链止 | 专用视图未调用，`perf_stats`/`perf_timeline`/`window_stats` 附带树没有位置；正文把未知样本升为 on-CPU 和连续执行，且把事件预览不全套到独立完整树 |

A 的 TID 36435 原生就绪 128.273 ms，不能以 D/IO 计数解释；同窗进程 36434 主线程 11.069 ms 就绪是另一对象。调查最后实际接受 6 个单元，只投了原样 `unit[0]`；两条行地址合法不验证满载解释。3/3 生产者注释、2/3 选中视图完整行到场，引用上下文仍有压缩或省略。旁路未启用，不倒签旧身份失败。[A 人工审计](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0211-codrax-hmc363-A-audit-v1.md)／[数字](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0212-codrax-hmc363-A-cells-v1.log)／[身份实际触发检查](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0234-codrax-hmc363-A-subjects-v1.log)。

B 的 3 个自然结果有同窗同分母采样树；直接视图才取位置绑定，说明缺口在共享发布范围，不是 CLI 附件没准备或模型必须换工具。实际成文已有未知样本/无执行状态提示，调查没有解释单元，不能把错句误记为未投解释的污染。A/B 都没生成 SVG/canvas/Mermaid，不冒浏览器像素验收；旁路不可用不等于已证无根因。[B 人工审计](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0236-codrax-hmc363-B-audit-v1.md)／[真实附带树原件定位检查](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0257-codrax-hmc363-B-records-v1.log)。

[封存索引](results/hmc_subject_raw_record_20261106_v1/sealed-index-v1.json) 375 件，SHA-256 `d8ba7ca6fcdc347cbd4af24bf7e659772ea4e9b3eec9c52d39dd1d2b6ea01d7d`；[收据](results/hmc_subject_raw_record_20261106_v1/sealed-receipt-v1.json) `3774c1f5add6f34a0dc6f8757154d520736f2c21d341747969ecfc870868a0fb`；[有限裁决](results/hmc_subject_raw_record_20261106_v1/sealed-v1/0259-codrax-hmc363-acceptance-v1.json) `closure=none`。[发布核对](results/hmc_subject_raw_record_20261106_v1/publication-v1.json)。大原件仅路径/大小/哈希引用，前代及本代原始失败保留。

## 剩余 50 项（ROI 高到低）

1. **P0 共享 7：** 02.4、01.3、16.4、01.2、16.5、12.5、18.4。先公开复现状态/零贡献和引用解释冲突，最小结构化交接当前对象、状态及可主张范围；再合并多层提示及正文/表/图的复用，保不同查询总体、完成权、链上业务及合法计算。不要按用例扫词，事实充分后偶发误述继续留档。
2. **P1 实材 6：** 09.3、18.5、03.2、14.1、18.2、08.3。优先让已有共享树在常用视图复用同代字节位置，真 BRBE/SPE 仍需单独材料及支持证明；轮换只读登记自然重试、来源验证正负和完整 CLI；按证据复验 IO 深度/驻留，不把样本或活动指数升级为时长。
3. **P2 跨域复用 11：** 17.6、04.3、04.5、08.1、08.5、12.2、12.3、05.2、10.2、14.2、17.7。窗、成员、资源和时序/关系/逻辑图复用原生主体、单位和已证边；下一真实状态窗优先挑蜂鸟众包或抖音整件扩大覆盖。活跃 WAL 未证零副作用不进生产。
4. **P3 协议 20：** 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1。先取得真实协议/可核配置，缺测不补零，局部不签完整。
5. **P4 新域 6：** 07.1、07.2、07.3、07.4、15.2、15.3。保审批、工作树、只读边界，依赖实材再做完整验收。
