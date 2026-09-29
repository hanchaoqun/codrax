# HMC §191：答案修补与只读登记人工审计

两例并行×1，30338与2362均正式exit0；机器原判1/2。人工结果为完整启动答案FAIL、只读登记阶段PASS；不能汇总成“完整用户答案1/2”。完整CLI只覆盖一例，登记不运行真实controller模型/最终回答。所有原始失败保留，没有第三例追跑。

## 1. 启动表保留，但不能宣称修补分支live通过

本次只调用一次精确窗event_search，1.000–1.080秒内13行=8启动端点+5系统事件全部进入真正finalizer，原始源行、ipid、起止和时长在场。最终两张表均完整：四段1.010–1.018、1.022–1.026、1.030–1.034、1.040–1.045秒及8/4/4/5ms正确；五个系统事件1.012/1.046/1.050/1.052/1.058的内容正确，缺域/缺事件名标未解析。

finalizer首次提交即被接纳，零patch/零拒绝，因此**未命中新数量修补指导**。不能将本轮没丢表归功于新修复或签live闭环；实际修补路径由公开真实adapter消息和类型/载荷/原子性回归覆盖。表格内容没有丢失，不代表正文正确：

- 摘要称5个阶段，表与源事实仅4个；把两条名称缺失/重复字典的行直接称为同名startup，未说明这是合成展示标签而非原业务名。
- 声称窗内“一次冷启动，ability_init=8ms”；源SQL没有该模式/度量。末尾虽然承认该摘要非窗内观测，却又称其来自“trace文件头部元数据”，错误赋予原始来源权威。
- `plugin.*`、`null_reference`等内部展示仍偏多；本例无图，仅核对Markdown/HTML内容，不宣称全图或截图视觉验收。

机器FAIL仅因5.0的毫秒单位在表头、不在数字旁，是既有文本oracle盲点；本批未修改oracle/原判。人工FAIL独立于该正则，确实存在上述事实错误。

## 2. 新定位的广泛交接缺陷：模型摘要被当成原始元数据

真实日志第677行第一段 `emit_perf_trace` 将窗外0.950–0.958秒LoadPreferences的8ms自行放入 `startup:{mode:cold,ability_init_ms:8}`；第二段第865行没有startup。源码数据和原始SQL未提供冷启动模式或ability_init字段。最终消息第2438行仍有系统格式化的 `**Startup**: mode=cold ability_init=8.0ms`，随后答案照用。

`emit_perf_trace.go:276`复制可选Startup且没有像Observations/Stalls一样标注提取权威；`builder.go:4704`直接展示启动信封，不同于紧随其后的model-extracted导航隔离。这是已定位系统接缝，不是“没给模型足够约束”：实际skill已有“源明确模式/显式毫秒测量才可发”的指令，仍未拦住交接扩权。应统一预分析摘要的来源/权威/范围合同，让未经验证的摘要只作导航；直接原生事实及合法窗外依赖保留，不能扫描答案原文或针对cold/8ms做专门硬门。

该缺陷与“4写5”、未知名误述分别留账，不声称所有错误只有同一根因，也没有证明纯模型波动。纳入01.3/16.4/18.4高ROI队列，与04.3/17.7源记录主体能力关联；不在冻结后插入未完整验证的修复。

## 3. 只读登记阶段：实际命中并通过

实际provider为项目配置的planner；保存的evaluation.json为模型与自然问题记录。01:31:39真实模型并行读取value.py和test_value.py，下一模型请求已收到完整测试；01:31:48提交 `changes:[]` 和精确 `test_value.ValueTest::test_increment` → `increment-result`。没有沿用旧失败的裸类名，未修改测试或新增探针，零发射拒绝。

controller规划授权由当前dispatch产生，登记后撤销；公开保存/加载后私有授权没有复活，再由verify派发产生新执行权限。最终原生断言实际PASS，报告把精确原合同标为project-test见证。旧invocation `native:e983:18d9bd82193a7758:1` 与新 `native:e983:18d9bd85b3f54ec8:2` 不同；源码交付plan与登记plan独立，收据归属原源码提交93db3e811d315d5556b2327c6a212e02e5412ef1。源码/test字节及HEAD不变，执行后授权撤销。

阶段验收可以关闭“真实planner是否得到身份并成功登记＋恢复后新执行消费”的这个退出条件，但18.5持续项、完整CLI及controller收尾不能关闭。fixture明确预置了强合同/已完成来源批次；最终workflow仍in_progress、proof仍planned且no_durable_store，虽经过公开文件存取，但不是默认持久工作流整轮完成。report.passed不能代替workflow终态。

上下文残留：明确只读教学之后仍有Targeted source exploration request通用“before planning edits / grep / repo_map”建议，与当前只有read_file/发射工具不一致；本次未影响执行，记录为01.3/16.4软教学一致性余项。依据typed活动批次和工具表裁剪即可，不增用户心智。

## 4. 边界、原始证据与计数

启动root-causes.json仍schema2、空列表、trace_root_cause_contract_not_active，无链外根因提升。实际请求仍timeout=10m、first_byte_timeout=10m、stall_timeout=5m。本批没有流式等待修改。初次finalizer估算55750 tokens、13条唯一展示行；全流程最高76264，不能把模型prestage峰值当finalizer峰值。

| 原件 | SHA-256 |
| --- | --- |
| 启动run-1.primary.md | 1b8c65d126a4814f966c053dbc79ef5d3b8afb5165e501af311c2a05a6140b4a |
| 启动run-1.logs.all.log | 9564b8fac2dbab2d4b1a0512b6222cc40b6dfd56f2438ce1a4ff185632743096 |
| 登记receipt.json | 579f50c40f8fd3331922c2ed23bc2789e665554fdfe753a6e068a07211e4fadb |
| 登记final-report.json | 7887d2ff8f7c220f2fc73a5152eeb328dc797ef1086be81d175af2eb44e2e42e |
| 登记logs/codrax-20260929-013133-000-59779.log | 2ef0382825340a3d881f763339129c3dad7afbc6f94b399bc171c38d70e0b890 |

目录见[固定双例记录](parallel_selected_summary_hmc_repair_registration_20260929.md)，启动工件`.codrax/output/20260929-013403.550-59397.{md,html,root-causes.json,answer-surfaces.json}`。完整稳定任务新增0，累计16/79、剩余63，重复0；本批通用修补子能力1、既有只读登记的阶段验收退出1，新增完整二进制能力0。五个稳定验收父项及全部旧FAIL保留，不把可复用测试入口冒称新的生产能力。
