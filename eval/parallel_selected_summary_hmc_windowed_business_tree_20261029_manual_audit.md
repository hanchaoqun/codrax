# HMC318 人工审计：正确统计、解释欠项与系统身份桥分开

根代理独立阅读原件、原生payload、最终MD与实际上下文，并与两份独立审计交叉核对。两例机器PASS，统计子项通过不等于整答无缺陷；本批完整能力新增0，79/26/53不变。没有改原断言、追跑第三例、冻后修产品或把未触发算通过。

## A：自然两窗、两线程状态对照

实际run为A/trace_query_windowed_artifact_coverage-20261029-214318，最终MD为.codrax/output/20261029-214531.929-65516.md。20行/2312B原件SHA62a95ccfba79994e192e6328f23b492f7f0510f6916b20f6170028c9f81cce70，源首0.999、末1.051；worker TID200/TGID100，不能升为进程200。

| 所选窗/线程 | 运行ms | 等CPU ms | 睡眠ms | 已归账/未知ms |
| --- | ---: | ---: | ---: | --- |
| 0.990–1.055 main100 | 7 | 1 | 44 | 52/13 |
| 0.990–1.055 worker200 | 9.5 | 1 | 36.5 | 47/18 |
| 1.030–1.050 main100 | 4 | 1 | 15 | 20/0 |
| 1.030–1.050 worker200 | 4.49 | 1 | 14.51 | 20/0 |

四账、逐段CPU与真实起止、覆盖范围在正文与表格均正确。worker宽睡眠31闭合＋5.5开放；窄贡献10.01＋4.5。开放尾真实结束行未知，源末/查询末没有伪装成关闭；宽窗两线程各尾4ms不外推。D/IO标记为零仍保窄口径，不推出没有IO或业务等待。原题统计限定PASS。

实质欠项是MD102声称“窗口累计分母不同，不可直接横向相减”。时长不是比率；同源/同目标/同状态且已证子集可以求已观测差，例如main运行7−4=3ms＝[0.999,1.001)2ms＋[1.050,1.051)1ms。该差不补13ms未知，更不拼重叠窗口总量。故整答仍有语义欠项，归16.4/18.2；不新增原文扫描门，也不假定已定位为某条系统提示直接生成。

八次真实trace_query均成功、四内容payload，四组合各查询两次；调查完成一次受理，答案一次完整受理，无patch或恢复。实际INIT03原生16/16行已到场，解释2/6且明确省略3052B；3份INIT是消息数量，不是3轮重试。current_exact_rows只证明地址，不验证自由算术。HTML main9张表（附件1、模型4、原生4）内容一致，无图不是缺陷；仅源级QA。

实际build/run八次均windowed=false，四payload省略omitempty的index_windowed；**新增Windowed自然NOT_TRIGGERED**，冷/暖/cache/预算/多源不能由本例签验。多目标自动补采未执行不代表四组显式查询未执行；root-causes unavailable也不是已证明无根因。查询标题未知与行区间半开分属不同域；shared query标题与其scope note一致性仍归旧端点欠项，不捏成数值故障。

## B：原04.2业务树完整合同

实际run为B/trace_query_business_tree-20261029-214318，最终MD为.codrax/output/20261029-214604.078-65515.md。16行/1481B原件SHA4d6956cb2befb4bad4124a122d2c6c3433fd9ff363887f6601cd8e113eba71aa；预览封装1571B没有冒原件大小。原QUESTION/EXPECT/15步不改，不加新Mermaid形式或源码义务。

原生及最终数值全部正确：business700/TGID600的OpenDocument inclusive/self10/5、LoadIndex5/4、DecodeRecord1/1ms；各self运行5、3.5、1ms，LoadIndex另就绪0.2、睡眠0.3。OD self为[5.001,5.002)+[5.007,5.011)，LI为[5.002,5.004)+[5.005,5.007)，DR为[5.004,5.005)。背景900的同名OD是独立5.5ms，调度未知不为零；AsyncPrefetch在700、S/F时段5.0025–5.010/7.5ms，不进同步树。明确包含不升源码调用、唤醒或根因。

**完整人工FAIL**：MD49/HTML抽取文本128明确说父层“自身”与子层状态重叠，违背原自身去重合同。合法self5+4+1=10、self运行5+3.5+1=9.5可反证；这是核错，不新增要求模型额外作加法。正确表格与稍后边界不能抵消，04.2仍验收中。

八次模型trace_query全部成功，另一次确定性window_stats补采（原日志2460–2469），七份去重payload。先后两次完整答案均拒绝：第一kind=flow不合法，第二已有合法diagram和两条contain锚，却要求三主体missing_unproven_boundary；唯一局部patch补三个unproven后受理。失败原件保留，不把第二稿当已经接受。实际来源绑定、业务树及自动补齐没有因为错误解释被删除。

### 解释到场与系统义务分层

- 实际INIT03:236–248已有正确inclusive/self、全部分段和状态；539–555显示解释为advisory，3/7整单元发布，省略2300B。547的unit1含“层间不可相加”推论，549状态no_native_row_address。最终把含混限制写成self重叠；不能说模型没拿到正确事实，也不能仅据一次观测定为纯随机波动。
- 同一INIT:253关系锚教学Do not sum parent and child durations无inclusive限定，362的measurement-note-23写“不能跨嵌套业务相加”，来自trace_query_business_measurement.go:68的inclusive表。系统教学适用域不精确是已证到场事实，但不宣称这两句直接生成了模型错误句。
- 系统图边界另有真实链：精确两contain锚→三missing_unproven_boundary→patch→受理→answerdoc.go:1377笼统写“用户所请求有向关系未证明”。原生及最终图均3主体2包含边，原题恰要求包含，分析frame_causality_requested/runtime_work_relation_requested均false。
- requested_runtime_participant.go:23、49–89、102桥只接精确线程和threadEndpoints；runtime_relations.go:92的业务实例provider未提供对应请求业务身份。participant_coverage.go（完整文件名answer_document_diagram_participant_coverage.go）:965/1094/3853再将身份映射不足落成关系缺证。下批应接受信来源/查询/线程/实例的业务绑定，区分身份不明与边本身未知，不能按同名认领900或放宽因果资格。

最终HTML源码保一个完整Mermaid图，业务/线程/数值为可见标签，长ID只用于内部绑定，不判为可见长名失败；无旧无fence树折叠证据。11表＝附件1＋模型2＋原生8，其中700时序同源重复展示，作为复用噪音继续留账，不当新实例。八原生表的共享query标题仍未知而scope notes半开，沿旧16.4/18.2端点欠项；不新增04.2退出门。未执行浏览器JS像素QA，不能签视觉效果。

## 留账与原证据

本批3组成文残余：A合法子集差限制、Bself解释/调查摘要冲突、业务图身份桥/边界冲突。新Windowed自然覆盖另欠；旧IO、采样、权限、代表选择与扩大race失败保留。优先共享业务身份桥与量尺适用域，再共享查询标题；能力槽轮换只读登记负控、代表规格，有新增真实材料才扩17.6二进制。WAL零副作用前NO-GO。

两例进程exit0和机器PASS、人工结果、自然覆盖严格分列。真实模型请求各4唯一配对，另A7/6与B11/10歧义组，不猜配；最大可证重叠18.820秒。公开回归/race/唯一全仓绿只签该确定性范围，不能补签以上失败或未触发。

本地388件[封存索引](results/hmc_windowed_business_tree_20261029_v1/sealed-index-v1.json)保全部原件/失败、INIT/参数/payload/MD/HTML；[独立A审计](results/hmc_windowed_business_tree_20261029_v1/sealed-v1/0160-codrax-hmc318-A-audit-v1.md) SHA9aeb6e8bed67e851412d8b60da8e46430dad460f8232133e507274eef26ebd22；[独立B审计](results/hmc_windowed_business_tree_20261029_v1/sealed-v1/0179-codrax-hmc318-B-audit-v1.md) SHAcef1d4dd21eadf5061b3f7a7926b6de55b19f75a43c7febb8b86c16dfed6af8c。主代理复核一致，closure=none；不改旧失败、不追跑第三例。
