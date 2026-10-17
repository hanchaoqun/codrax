# HMC258 完整人工审计：材料用途与原生展示

日期：2026-10-17。结论：**完整答案0/2通过**。每例一次、进程并行且均正式exit0，没有追加第三例。实现/局部能力、物理产物和完整语义分别裁定；旧原始失败与§257有限PASS不改签。

## 1. 范围与方法

审计当前实际工具调用、接受/拒绝过程、finalizer初始消息、持久化Markdown、HTML主内容、answer-surfaces的primary/principal及root-causes旁路。HTML未做像素截图验收；没有完整最终AnswerDocument JSON，因此仅以实际accepted日志、最终输出和surfaces交叉证明，不把模型初始params冒充系统最终文档。两例均无图，Mermaid/局部图修复自然NOT_TRIGGERED；资源夹具无调度因果见证，不能据本批签Trace因果投影验收。

原件是构造SQLite；A的checkpoint由零模型operator准备，不是自然中断。独立oracle以sqlite3只读查询产生，放在workspace外，不给模型提示单位/边界/身份/分母。B未seed catalog，四份物理原件即使字节相同仍保持独立身份。

共同冻结7069输入，binary SHA `aca6018f7d20859267c6d6d40e35f9bec47a067ebeed7f8fbdb5981ac81a5720`，freeze SHA `42fa62472fe72b3e08645a4c996ca324d3533b467f57cf01ccd2da60feb807f1`。A PID18066、UTC07:02:34.298619–07:07:10.318967；B PID18132、07:02:35.152783–07:14:13.549122。路径来自真实成功工具返回/终态receipt/output_dump，不按mtime猜最新文件。

## 2. 独立事实基线

原生验收oracle窗口为`[10000000000,10050000000)`ns，即`[10.000,10.050)`s，不作时区或domain转换。B实际走通用event_search的秒级inclusive选择器，不能把该结果冒充原生半开窗口集合；这不是宣称检索器算错自己的边界。B每组选择应以采集文件/精确查询为成员，不是任选两条事件；选择策略可在原合同内灵活，不能由oracle预定哪份文件获选。

| A原件／B对应原件 | 请求线程 | 窗内资源事件 | 关联帧 | 实际观测身份 |
|---|---:|---:|---:|---|
| root／session-a、session-c，各自独立 | 101 | 3 | 8 | gallery-main／Gallery |
| root／session-a、session-c，各自独立 | 202 | 1 | 3 | background／Background |
| nested／session-b、session-d，各自独立 | 101 | 1 | 3 | editor-main／ImageEditor |
| nested／session-b、session-d，各自独立 | 202 | 0 | 0 | 无观测名称，不借另一源的background |

A合计5事件（4 Alloc、1 Free）、14关联帧、11唯一物理帧；四个源局部栈键，因此“4个调用栈片段”本身不判错。原始大小为512/512/256/128/512但单位未知。Free深度`[0,2]`仅缺1；晚Alloc深度`[0,0,1]`，malloc与buffer_cleanup同层，不建立二者上下调用关系。保留帧完整不证明真实展开完整；end_ts不是执行耗时/等待，也不自动证明释放、泄漏或响应根因。

## 3. A：局部交接通过，完整成文与有限复验失败

### 3.1 实际执行与物理交付

完整读取137行checkpoint后真实resume，四个当前query_ref各查一次，3success＋1empty；历史成功没有冒充本轮。原名capture-review包内四份查询载荷及report，与当前index/manifest/terminal_export完全一致，链接退出后可读。仅查询结果文件的重复read_file不是新原件尝试。

零模型helper使用真实saved_index、日志原样提取的terminal_export、正式进程收据和output_dump记录的Markdown：PID59190，UTC07:08:28.330286–07:08:31.123963，exit0。输出状态`physical_delivery_smoke_pass_human_pending`，只证明六个适用实物轴，不能改签humanPASS。

### 3.2 材料角色和原生展示

实际finalizer：current_source_lane=excluded、requirement_precision=none，源码evidence收集/引用均0，exact support=0；current_source_records=1只是list_files路径发现，不是checkpoint源码证明。没有导航触发的dataflow或源码附表，原清单仍可读。当前运行时恢复分支PASS；source/write/mixed/取消/换代等负控由确定性测试另计。

初次空runtime_measurements被拒后，两路accepted completion均选择四个summary作为answer_display。最终四份summary各一次，行数1/1/3/0，每表4条Notes，全部保存源、窗、身份、未知单位、深度形态及展开未知；empty保持无观测身份。members未选，不能自动把调查参考扩大成必需展示，也不能声称完整明细已交付。

原生5事件+14帧共19/128行，selector遗漏0，实际初始估计48,449/200,000tokens。成员明细及正确原始信息已充分进入上下文，不是预算截断。旧runtime-only早退在separated handoff前执行，A旧reason/interpretation没有原样重传到finalizer；接受摘要曾有错时间和6事件，但终稿已改正，不能列为最终错值。

### 3.3 最终失败

- Markdown正文/手填表/列表多处`512字节`、`256B`，与原生“单位未声明”矛盾。
- 最终脚注把nested/202“一次”本轮查询说成“两次”；该成员历史也not_executed，不能以历史或重复读结果补次数。
- 多处“栈完整/质量完整”混同源载体保全与真实展开；正文把同depth0的buffer_cleanup称malloc上层，后文又说二者同层，存在内部矛盾。
- 手写summary、表格和列表重复原生数值，正确原生附表未消除主文错误；members完整调用位置没有被选择交付。

上述同样见HTML主内容及primary/principal。没有发现新增调度、等待、泄漏或响应根因结论。长prepared路径/内部术语仍增加阅读成本，归16.5，不用它们替代事实失败的证据。

**A完整FAIL；11.3本批有限最终完成说明复验FAIL。** 终态组件、包和实际集合自身没有退化；自由计数错述归已有16.4并交叉挂11.3，不重新打开稳定实现项或修改§257历史有限PASS。本批不得写“11.3复验通过”。

## 4. B：代表入口未触发，通用取数与成文失败

### 4.1 实际计划与调用

没有representatives/representative_status、选择快照、resource_stack或native_fact_handoff/answer_display。实际discover为四物理源×五个capture/all模板，共20声明计划；4成功、16未执行。另11个observed-only查询身份成功，总31query记录、18次真实event_search尝试，其中3次是同身份重复尝试，原件a/b/c/d分布4/6/3/5。验收期望的八个按线程/窗口原生计划未建立，不冒充实际执行。

因此11.2为**NOT_TRIGGERED**；没有选择快照，不能运行要求快照的代表helper后宣称验收。通用工具组合并不被禁止，但本次既未触发该有限能力，又未正确完成等价的分组/未选/当前进度说明。

前置分析五次拒绝逐次修comparison引用、fact_families及问题形态；调用位置仍被声明required source_location，进度被声明function_or_purpose，源码lane三次阻碍完成。之后有接受闭合并生成答案，不能称永久阻塞。只读扫描证明无当前代码不等于允许伪造源码证据，亦不以问句关键词取消真实独立源码义务。

### 4.2 实际上下文

18次查询均是event_search，十份completion参数没有原生交接；不是新display seal拒绝了合法选择。finalizer有15个通用事件selector，非空目标保留7/2/7/2，共18/32成员对象；21/5广查原始量与目标量已区分。通用表footer的0/128不是所有事实消失，不能伪诊断为128行预算饥饿。

初始估计106,386/200,000tokens，动态用户消息226,334字符，元数据scope多达event-scope-133。这是相关性/重复成本问题，未证明context溢出。正确原生资源帧视图未取，信息仍停在raw/encoded材料和模型摘要。最后reason确实以advisory current synthesis重传，候选member notes再次重复错误覆盖/单位/栈结论；正确限制也同时存在，形成事实表达竞争。

### 4.3 最终失败

- “每类两份代表”被改成若干事件，缺源成员选择快照、未选/未匹配/跳过及真实当前执行状态。
- 21/5泛检索记录被称完整native-hook事件总数；记录首尾被称完整50ms或缺失后45ms采集，采集覆盖没有该证明。
- thread202表把10.050秒的thread101事件列为第二代表，主体错误独立成立；同时未披露通用inclusive结果与原生半开窗口集合的差别。
- 源大小单位补B，同深度帧串边、错用其它事件栈；无配对见证却声称分配释放配对/未释放。
- 最终进度以观测时间覆盖替代当前计划/尝试，20计划中16未执行未得到诚实说明。

以上属于共享任务/查询用途、身份/范围/量尺、同源成文与当前状态交接问题，不为每一句建新硬门。最终普通表和正文无原生selector；只有系统跨工件关系边界表，不能据其正确签其它内容。**B完整FAIL，11.2 NOT_TRIGGERED，不销账。**

B独立只读核验v3再次对四原件运行oracle SQL并逐字段比较，SHA前后不变，正式exit0；输出SHA `93fb4c50611cddc833587503f39b33787c645acd73aea667a802afb42525bf45`。辅助v1的Ruby语法错误、v2的日志非UTF-8解析失败均保原；v3仅解析副本时scrub非法字节，不改原日志。三次辅助核验均零模型/零产品查询，不是三次真实评测。初稿“检查进度100%”已被最终patch删除，不列为终稿缺陷。

## 5. 证据定位与SHA

相对根：`eval/results/hmc_material_role_native_delivery_20261016/`。正式收据和独立报告归档于本地`verification-receipts-v1/`。

| 工件 | SHA256 |
|---|---|
| A run.out | `523446819f5ae7f82a0f0bd9a53b8d9fe0781733b588092f66c33754c78c6e09` |
| A output/20261017-000710.237-18066.md | `cf14af9e3e86f6bedaaae2fe7443803c39ddeacac659252a03c583b7d7c0966e` |
| A answer-surfaces | `cd580de6da80fde646bd8ba3e866b3ff1ed6fadc115db7058b31e369cb5b4eae` |
| A manifest | `cb447dbe5e8dc32404d567a76813c6e97f4d22e31aefe8bf6cbce96a30a60a02` |
| A report | `5a1e8d1012c1c34cf2f99b2692acbf4b6e0408b0135edf1c7cbe2d0785f99d3b` |
| B run.out | `104179481d9db9b288c6ff5d7b86289c5f16c95165c5176dab825a0391a7248b` |
| B output/20261017-001413.475-18132.md | `5dd8be0fb7b0a92d03ba2d527b6e940e815d420f00e2366fc99bb8edefdf240b` |
| B answer-surfaces | `38c8c11eb2c93a5617d1377ac5cd1d383505bac2c166be4987298972911e648c` |
| 全仓JSONL | `fbbafa175ff9a2ca328fd4daadd6f796ad6e9d8eee0c2637a0cb9a3d1e97af90` |

## 6. 结算和后续

完整能力新增0，已交付子能力2组，当前验收缺陷3组；79唯一＝24已交付＋55开放不变。完整FAIL、局部PASS和NOT_TRIGGERED并存，不能互相覆盖。当前组归11.2/16.4/16.5/18.4；11.3本批有限复验失败交叉保留，历史实现和原PASS不倒改。

下一能力轨转向17.6二进制低权限原件路径/格式剩余能力，缺陷轨先修任务操作到原生查询与同源交接，双例轮换sample-only和只读登记终态。调查独立解释可能被旧runtime-only早退省略只列待验证风险，先做公开反例；不得以“减少噪音”为由删除链上业务解释、因果投影或自动补齐。无源码/因果权限扩大，无问答原文关键词硬门，无第三例追绿。
