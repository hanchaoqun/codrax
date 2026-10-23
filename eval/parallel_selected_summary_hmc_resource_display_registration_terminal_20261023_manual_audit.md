# HMC287 人工审计：A 整答 FAIL，B 限定负向目标 PASS

本批两例真实模型并行、各一次；无第三次追跑。A 是普通 CLI 自然采集问题，B 是真实 planner＋脚本恢复 controller＋真实原生执行；两者能力范围不同。完整能力新增0，54开放不减。原始审计封存后可从 `results/hmc_resource_display_registration_terminal_20261023_v2/sealed-v2/tmp/codrax-hmc287-A-manual-v1.md` 和 `codrax-hmc287-B-manual-v1.md` 查完整逐项证据。

## A：总体、窗内成员、栈帧与成文

原件 SHA `a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`，是构造 SQLite，不冒充实机采集。原件声明10.000–10.100秒、差100ms；转换事件首尾10.000–10.050秒、差50ms；资源事件首尾10.005–10.050秒、差45ms；用户窗[10,10.05)，四者不可互换。

| 独立 oracle | 原总体 | 请求窗内 |
| --- | --- | --- |
| 事件 / 关联帧 | 5 / 14 | 4 / 11 |
| 101申请 | 3次 / 1024 bytes | 2次 / 768 bytes |
| 101释放 | 1次 / 512 bytes | 1次 / 512 bytes |
| 202申请 | 1次 / 128 bytes | 1次 / 128 bytes |
| 202释放 | 无记录 | 无记录 |

10.050秒申请256bytes只属原总体，不进半开比较；`heap_size`不换成`all_heap_size`。链7深度0/1/2有3帧；链8深度0/2、缺1且有未知符号；链9深度0/0/1含重复深度与歧义符号。地址-1、IP及原始十六进制值不自动解释为哨兵。深度不证明调用边、方向或执行顺序，事件结束字段不生成释放或等待。

真实调用有9份参数、8份不同结果文件，重复同查询不能算新原件。精确窗多主体结果4/11、101独立3/8、无窗查询继承请求窗、宽[0,60)结果5/14均分开。没有独立202查询，但多主体结果已完整包含202，不能伪称没取到，也不能把worker声称检查两侧当作独立调用证据。

实际 Finalizer 收到7行原件元数据，含100ms原声明；完整MD/HTML附加元数据表也展示三个纳秒原值及背景权限。主表来自精确窗查询，另有自动补齐的宽查询投影表保原5/14、展示窗内4、排除1。计数、大小、主体、右界和完整11帧保留，数值与供给子能力通过。主文未解释原件时长，不把“附表正确”夸大为“模型主动准确讲解100ms”。

完整答案仍 FAIL：

- “窗口内负载完全压在一条线程上/包揽全部3事件”与线程202一事件矛盾；总请求人口为4。
- 业务段称两线程申请“都从 ImageCache 发出”、UIFrame“之上/更外层”、重复深度附近“上一层”，将原始栈深度升为未验证调用关系。前文免责不能为后文关系背书。
- “没有其它资源族或非堆事件出现在这份采集”范围过宽；资源族结果不能证明整份 Trace 没有其它事件。此项不作为唯一失败依据。

同成员的精确查询、101查询、宽投影多表重复，来源身份未混但噪音仍高。实际解释单元投递3/4，重要限制末单元整条省略；原生 notes 和 grounding 另有等价限制，不能认定所有边界均缺失。Finalizer估算上下文88296（非wire tokens），explorer最大104865不能冒充Finalizer。主文一次受理、无patch修补；完整HTML8表，无实际Mermaid/SVG，图关系/时序/布局QA为N/A。根因sidecar为空且合同未激活，不扩签Trace根因质量。

## B：只读登记当前完成与来源终态分开

父PID20322、恢复子21013；模型实际读取value.py 2/2行、test_value.py 4/4行后一次登记，计划changes为空、无新探针，既有断言原样保留。来源apply由真实公开工具/Git生成，两坐标同实际提交；来源从未controller verify，三个前后快照无verify attempt、VerifyRef、Completion，不是单删Completion让Normalize回填。

当前新 invocation `native:5215:18e11d3d0f7232e8:1` 与历史 `native:4f62:18e11d3b3861d660:1` 不同。当前1条断言passed、2条命令为编译检查和unittest，不是2条独立测试；当前证明 verified、整体仍 `unverified/missing_terminal_verify_verdict`，符合限定负向目标。父子各4原件＋1verify引用逐SHA/长度一致；目录副本数不增加执行次数。真正最终原件为typed archive中的 `.final.json`，`final-report.json`兼容别名不误当末代用户报告。

只读凭证校验源/测试SHA、HEAD、diff/status；审查时临时repo已清理，以独立执行收据和已保留原件复核，不声称重新读了不存在的目录。恢复模型请求与A真实重叠，有原始请求/响应时间，不仅是启动屏障。

限定人工 PASS 不代表全部层已统一：planner仍有旧owner提示，最终loop定位摘要仍建议localize；currentproof完成与该旧摘要冲突归18.4/18.5。fixture还明示无完整durable store、物化checkpoint不足、contextpack durability未验证，不能签完整生产持久化/自然controller/全CLI。来源失败、显式owner缺口、缺apply旧负控不被此PASS推翻。

## 封存与验收记账

v1构建代差在零模型阶段发现，正式两例只运行v2；v2全仓的旧表数量/静态审计/固定收据/ZIP token失败全部保原。后继v3另冻7340输入，全仓97包PASS/12无测试/零FAIL；唯一生产变更为已独立验证等价的元数据显式键，不新增模型验收。v2原173件与v3后继52件分开封存，合计225件不覆盖。本批完整能力新增0、共享子能力2组；A整答和B旧提示/最终摘要冲突为两组剩余验收债，旧IO/采样解释及权限自然覆盖继续开放。WAL生产NO-GO。

最终全仓、不可变索引及发布凭证见[批汇总](parallel_selected_summary_hmc_resource_display_registration_terminal_20261023.md)。不得用后继确定性PASS回签A，也不得将B限定负向通过写成完整自然controller通过。
