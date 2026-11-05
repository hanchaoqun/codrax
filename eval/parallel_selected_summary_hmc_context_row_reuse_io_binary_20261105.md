# HMC355 — 单次上下文完整事实行复用 / 原IO与sample-only轮换

日期2026-11-05，仅main。基线`822d6e8acf8d6deba0bde7246cf95572bef3ac71`，源码`d81067e8d8e98318003831db025eaf41ab4a01c0`，5Go（4生产/1测试）248增12删。文档与源码相邻分批提交；最终远端/干净工作区凭证见[publication](results/hmc_context_row_reuse_io_binary_20261105_v1/publication-v1.json)。

## 1. 数量与两轨结果

- 79唯一：29完整、50开放，17部分/29待实施/2待验收/1验收中/1持续，全部checkbox不变；[核账/全ID](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0378-codrax-hmc355-census-after-v1.json)。
- **完整能力交付数：本批0，累计29。** 原HMC-08.3有限验收仍FAIL，不因机器绿或数值正确销账；上批11.2有限PASS保持。
- **已交付子能力：本批1共享组。** 一次受保护ledger上下文内完整typed事实行精确复用，有公开真实query→Finalizer RED/green；不是完整输出字典、查询去重、来源授权或自然答案降本。
- **剩余验收缺陷：** IO物理队列/卡顿排除越权、采样未知态被解释为执行；新引用自然未触发。前两项为旧解释债，第三为新自然覆盖欠项，不虚增稳定任务ID。只读、图、权限、旧栈/数值限定覆盖另保原账。

## 2. 泛化修复与边界

公开旧码复现同一完整IO行在Requested Runtime Fact Authority与专用IO账户重复。`runtimeContextRowPresentation`只在本次受保护ledger渲染存活，以完整public typed ObservationRecord加原renderer行作精确key：已实际写出完整定义后才可引用；完整源、query、payload、窗口、主体、值、Summary、Notes都参与，未知/不可信/字段有差异保持原全行。引用自身保source_path/query_result，消费者段原总体、owner、量尺、闭合和因果说明不变。独立调度并行度上下文复用同组件；旧独立renderer保持原数据。

不改ledger/doc/任务范围/因果投影，不产生来源/全集/因果/完成凭证，不逃逸到下一dispatch；不放宽私有表资格、不合并不同query的总体、不扫描用户或模型原文。无新模型发射schema；一条引用说明明确它只是同prompt展示导航。13个精确负控和跨dispatch隔离PASS。

局部两账户字节17513→16914，省599B，**尚未扣新增引用教学**，不是全INIT/token节省；自然A6定义/0引用，B0/0，均NOT_TRIGGERED。通用ledger/native/最终表重复、重复查询及跨块整组预算未完成，停止在此局部继续微调。新测试v2误用不存在SourceID编译FAIL保原，仅修新测试为CaptureIdentityPath，旧断言不放宽。

## 3. 参考设计与真实材料

已对照参考完整指导、architecture/Golden Rules及`core/preprocess/io_ops.py`：compute_io_concurrency按20ms重叠桶统计，compute_io_concurrency_agg按SQL发起桶聚合并过滤异常。设计意图是资源概览，不等于连续在途深度/硬件队列/线程阻塞。本项目保原成功配对半开区间sweep、全窗均值、union忙碌时长、request·ms和独立发起总体；不搬桶数、异常阈值或缺值补0。独立整数端点分区核原IOexpected，不喂模型。

认真使用用户完整原件，非裁剪样本，零模型实际query→Finalizer保护：

| 原件 | 完整大小/行数 | SHA-256 |
| --- | --- | --- |
| 网易云105版、后台销毁后冷启动ftrace | 82,838,676 B /559,592行 | 12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4 |
| 元宝105版、容器重启后冷启动ftrace | 63,659,046 B /412,546行 | be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c |

保原数值、完整来源/窗口和Trace因果投影。目录75份ftrace未找到本批所需block_rq_issue/insert/bio_queue协议，不外推IO零；自然IO仍明确使用原authored夹具。自然binary用真实完整30,062,585 B PERFILE2原件，SHAda713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632；原独立reference parser核12000/1393/164/259。它跳过branch_stack/未知位，不冒BRBE/SPE全能力完成。

## 4. 测试、冻结、唯一全仓

终版相关4包116顶层/316子PASS（2顶层SKIP）；race×2三包44/136、原4根因/闭合睡眠四包21/141、双整件1/2 PASS。原有优先级反转、调度/算力供给、D/IO等待、确定性语义优化和链上业务保护未删；相邻/背景仍不升根因。

[冻结](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0416-codrax-hmc355-frozen-v1.json)7650输入/349外部/10仓外快照，SHA`5a500fcc7cf493effe0f4612bb5b0e2b24d29cc421e76b099571212fedea24d2`。此后源/测试/case/oracle/helper停写。唯一full UTC12:06:09.259524–12:20:03.651367，OS0、源前后true：98包PASS/12无测试、28780顶层PASS/58SKIP、34627子PASS/14SKIP，268719纯JSON、FAIL/重复/未终态0；[原统计](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0439-codrax-hmc355-full-stats-v1.json)，log SHA`95ed9d828329d0f1ad95ea47139488c11cf4a2d723658d68448eab0f7c00b778`。

独立startup、15项实际输入/配对负控、CLI构建及两oracle通过。argv capture故意不运行真实CLI，原eval runner缺答案的verdict FAIL保留，不能把capture进程OS0写成模型PASS。没有第二全仓或第三付费补跑。

## 5. 恰好两例真实并行 / 根人工结论

两原自然QUESTION、原附件、n1/15steps，原runner不变，未把防护规则/oracle塞入问题。唯一已配对模型请求A6/B8，歧义0，10真实重叠对，最大29.446s；[实际请求重叠](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0297-codrax-hmc355-actual-request-overlap-v1.json)，不按进程/屏障推断。

| 例 | 机器 | 已核事实/有限路径 | 完整人工 |
| --- | --- | --- | --- |
| 原trace_io_inflight_overview | PASS | 四组24数、8配对/13段、分母/配对覆盖/独立发起PASS | FAIL；08.3有限FAIL |
| 原trace_binary_sample_only，真实30MB | PASS | 准备/原生取数、164/259、CPU记录坐标、完整单帧、合法重试PASS | FAIL；未知态被写执行 |

A低深度被升设备无积压/排除IO卡顿；正确表及尾注不赦免正文。RQ同源同口径9次端点小计合法，BIO不叠加，不为修越权封禁算术。B成功解释unit0自身带“跑过”与unknown边界冲突，实际INIT投递，最终正文沿用；不把未投unit或坏引用拒绝稿冒作污染。详[完整手审](parallel_selected_summary_hmc_context_row_reuse_io_binary_20261105_manual_audit.md)。

审计一度按字母排序blob误判重试先后，已依日志时间/调用ID/完整JSON纠正：先拒六单元、后接受五单元，INIT确为后者；**无已证旧代次串入缺陷**。纠错原件保留，不虚增gap。充分事实模型误述继续留档，不无限阻塞其它高ROI轨。

## 6. 保原与发布

[封存索引](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-index-v1.json)513件，SHA`30324db0c6ed65f9a97307f6273cc3231e779e6f8e335730db8d93cd68f34b10`；[seal](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-receipt-v1.json)SHA`ba0bcfba263701a43492917a70754f8d05362496d4bd1443b72f23cbffb0ba97`。[根决定](results/hmc_context_row_reuse_io_binary_20261105_v1/sealed-v1/0296-codrax-hmc355-acceptance-v1.json)closure=none。累计57代26536件按代非全局去重；物理核前354476/353607/失败352709，其它53代链头。失败、新测试编译错、拒绝稿和操作审计纠错都保原。

不复制凭证；providers只交路径、不读/哈希其内容。大完整原件采用精确路径/大小/哈希reference-only，未拿预览充原件；Go快照仓外并归档为txt，不污染后续测试扫描。五文档汇总在终态后，代码/文档相邻main提交推送，publication独立核原件/冻结/远端/清洁。WAL零副作用未满足，生产NO-GO。

## 7. 剩余50项，按ROI全覆盖

优先级表示工作顺序，不改变原状态；引用都省略共同前缀HMC-。同层内部按所列先后推进，有真实材料/明确根合同者优先；没有证明的权限/图分支保持欠覆盖，不记通过。

| 顺序 | 数量 | 全部开放ID | 推进方式 |
| --- | ---: | --- | --- |
| P0共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | typed可用性/解释适用域，完整事实与量尺同源交接；减少重复教学，完整输出/整组预算；图共享身份关系验收 |
| P1具备材料 | 5 | 08.3、18.5、03.2、14.1、18.2 | IO/资源有限原合同验收；轮换只读登记正负终态及真实格式，不追单例误述求绿 |
| P2复用能力 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 复用实例、资源、帧、源钟、生命周期基础，按源协议和独立分母验收 |
| P3协议/材料依赖 | 21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 先真实协议/配置/身份/时钟及独立oracle，再完整链路，未知不补0/不拼因果 |
| P4新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 媒体/Sendable及写验证，遵循原批准/worktree/风险边界 |

下一批仍为一完整能力轨＋一已证共享缺陷轨。优先系统typed缺测与解释边界、完整输出复用/预算、具真实材料二进制/只读剩余能力。旧IO、采样、栈、权限/图和新自然引用未触发保原；不新增关键词门、不损合法计算/重试、Trace因果投影或自动补齐。
