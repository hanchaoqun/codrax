# HMC279：资源事件原生成文与共享身份保真

基线`63760b117cd5a6a26b5cae2ab6db1ec08b264695`。逐checkbox复核79唯一＝24交付＋55开放；32待实施/16部分实施/3待验收/3验收中/1持续执行。完整父能力本批新增0、累计24；生产子能力2组；两份整答人工均未通过，欠项并入原55，不另加父任务、不回签旧FAIL。

## 1. 本批交付边界

能力轨14.1：将§278完整查询人口的资源事件统计接入原生`distribution`，与已有summary/members共用私有来源、版本、查询窗口及选择器。按观测线程/进程实例、资源族、源操作、子类型和单位分别展示次数、大小状态及精确合计；统计不取自40行事件/128帧预览。已知0、NULL、未提供、非法和负值不混同。共享单位从闭合原生协议取值：heap/mapping为bytes，其它族保持未声明。

普通“按线程分别统计”无需用户枚举所有线程：规范无目标查询可发布三表，但只有当前请求确实没有显式目标才可消费；有明确目标、旧双表、JSON恢复、过期来源或错窗不借此扩权。独立8KiB表预算按整组裁剪，合并64KiB发布预算耗尽时保留已合法的旧双表，并私有标记“统计表未展示、不是零/未执行”；原事件载荷和已有权限不改变。broad没有旧发布资格时保留原rich facts，不凭空授原生表资格。

缺陷轨16.4/16.5：共享Goldmark内联层在自动链接前识别有限的命名空间限定类型参数，HTML和终端保留字面身份，不重写模型Markdown、源码或符号。`<Namespace::Type>`及cv/指针/引用/数组等类型装饰保持原文且HTML转义；不解析任意C++表达式。普通URL、邮件、显式链接、代码、原HTML安全路径和Mermaid不改。裸`<custom::opaque>`与类型语法有歧义时保字面，显式`[label](custom::opaque)`仍表达链接；不声称所有URI均无语义取舍。

参考仓`core/query_engine.py:283–328`完整取数后预览的分层意图适用，但不能照搬丢查询坐标的metadata白名单。`heap.yaml:375–418`按线程申请/释放下钻可复用；生命周期重叠不是事件起点窗口，`heap.yaml:623–665`的SUM不能证明`native_hook_statistic`增量/累计/reset协议。本批不做资源存量、泄漏或未证统计协议。参考`hiperf_ops.py:1208–1243`直接输出原符号，没有共享HTML修复可照抄，故在渲染层泛化处理。

## 2. 验证版本和真实评测

7263输入与实际CLI冻结：清单SHA`63ee686e9554b216221bb49b11a9aeebf112599cdb869cf04306c83f936a95a3`；CLI SHA`0f77a3840fb32ad75a0d4e9c36dd50ab9d86cf86b2e61c8a0ba55d600f46f65b`。`make` PID29926 exit0。资源公共查询→原生选择器→实际Finalizer→摘要/表格/无边事实图、未知/0、分组裁剪、预算fallback、来源/窗口/目标/JSON负控及相邻消费者通过；资源race PID27976 exit0。共享渲染三包525顶层/241子例通过，相关race11顶层/81子例通过，公开原始RED保留。

两例各一次并行，UTC2026-10-22 06:55:08启动，机器2/2，完整人工0/2。没有第三次模型追跑。

| 例 | 正式PID/终态 | 原生能力验收 | 整答人工欠项 |
|---|---|---|---|
| A 自然资源统计 | 35585；07:01:45.563282；exit0 | 未改SQLite原件，全主体目标窗4事件/11帧/3组；distribution显式选择，summary/members自动补齐；2次申请768B、1次释放512B、另一线程1次申请128B，右边界排除 | 源深度被解释为入口/最深/上一层，缺depth1仍造调用顺序 |
| B 原真实sample-only | 35584；07:01:19.199539；exit0 | 完整30,062,585B PERFILE2；12000总样本→窗1393→目标164/259；两个82热点与独立分母正确；自由粗体与系统HTML保完整模板身份 | 未知类型被确定为on-CPU执行；200ms观察窗被写成停顿时长；来源缺证说明仍越权 |

二者原生数值与边界均进入真实成文上下文，不归咎于统计器漏数。A的units-only调查完成虽受理，七个解释单元原文/边界未出现在实际Finalizer INIT，不能宣称解释单元成文交接自然覆盖；原生三个视图实际消费另有证据。A的complete reject计数还把DOWNGRADED/受理混入，需要按真实工具返回区分，不把4次提交写作4次拒绝。两项按原16.4/18.3交接欠项登记。

原v1全仓PID35583，UTC06:55:08.233473–07:09:02.156018，exit1：96包PASS、12无测试、1包FAIL，无非JSON行。唯一失败为`TestHMC249ResourceFactDiagramDoesNotSupplyRelationships`旧“必须两视图”断言（实际三视图）；此前循环已检查所有三表不抵扣关系图、不得掩盖其它图、篡改后失去资格。原始全仓失败先封存，再仅迁移这一测试到精确三视图集合，保全部因果负控；后继验证单列，不把原全仓改签PASS。

## 3. 未完成项与ROI

完整能力新增0/累计24、交付子能力2组、本批整答验收欠项2份（3组解释问题：栈顺序、采样执行状态、窗口/缺证口径）。14.1仍部分实施，statistic生产协议未证；17.6此次只是已有PERFILE2回放，不扩签新设备/格式/版本。WAL未满足零副作用，生产NO-GO，没有生产接线或重复副作用探针。

下一双轨：能力轨05.2→05.3来源绑定的时钟见证及指定窗联查；缺陷轨先验证units-only到成文解释单元的真实交接与执行计数口径，复用原生事实减少冲突，不为本批自由误述加关键词门或无限追跑同题。17.6新真实原件、对应可运行工具及原生宿主收据齐备即提高优先级；只读登记轮换剩余框架/多源授权/实际verifier边界，已通路径不重建。

55项精确无重计分桶：14.1共1项；共同交接/展示和有限验收11项；只读登记轮换2项；基础及可复用聚合12项；真实材料/协议/安全前置8项；其后领域扩展21项。每个ID及依赖见封存`roster-v1.json`、`roi-v1.md`，不是仅汇报总数。

- [完整人工审计](parallel_selected_summary_hmc_resource_surfaces_identity_20261021_manual_audit.md)
- [原始版本证据索引](results/hmc_resource_surfaces_identity_20261021_v1/verification-receipts-v1/artifact-sha256-v1.json)
- [原始全仓失败收据](results/hmc_resource_surfaces_identity_20261021_v1/verification-receipts-v1/process/codrax-hmc279-acceptance-full-v1-receipt.json)

## 4. 后继验证与发布

v1于UTC2026-10-22 07:16:56.564449封存270文件（保26原运行文件、复制244份、26源码快照），索引SHA`ae8fdfc305a61dcb9802075923063751beeb04adc51f6aaaa28dd0f9823914d9`。后继仅`contract_check_resource_facts_hmc249_test.go`一文件变化；7263输入名单一致，生产/CLI/原件/用例逐SHA保持v1。

v2整个orchestrator包PID55363于UTC07:20:48.831447–07:23:16.931259正式exit0，2403顶层/1529子例PASS、6顶层skip、零FAIL/非JSON行；日志SHA`d36f3befdaf8073cf878ce4f4204a48fc2fc3e92c6697978a2fdc1d49013b86f`。定向race PID55443正式exit0，实际匹配HMC249一例，日志SHA`9fd0f1a1fca729ab8ded7f06b6fe0bb8aab1c3640562cef685c8f1f93fe08687`。没有第二次全仓，更没有第三例模型；不把“原全仓＋唯一测试修正后的整包”写成同版全仓全绿。冻结前遗漏这一相邻消费者选择如实记录。

后继另封8文件，索引SHA`ccf6316d94fe213501734da86dc2d5fa31d4d727db3c32ed9376dcce4720a39e`，原270文件逐件复核不变。代码提交`a8d01ebef5fa1b101533155bbe83158a6b088be2`，审计文档独立一笔；普通推送不改写远端历史。最终文档commit、HEAD/origin/main/远端一致、clean及两个索引共278文件复核由下列独立发布收据登记，避免为写自己的hash再制造文档提交循环。

- [test-only版本说明](results/hmc_resource_surfaces_identity_20261021_v1/verification-receipts-v2/codrax-hmc279-orchestrator-followup-v2.md)
- [后继8文件索引](results/hmc_resource_surfaces_identity_20261021_v1/verification-receipts-v2/artifact-sha256-v2.json)
- [最终发布收据](results/hmc_resource_surfaces_identity_20261021_v1/publication-receipts-v1/publication-v1.json)
