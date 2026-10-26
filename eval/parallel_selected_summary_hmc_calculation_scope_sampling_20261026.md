# HMC296：合法计算教学、当前因果权限与有限能力轮换

基线 `06a3359438f08a7321d84fe4dcfcb7d6e5e0d7c6`。独立按稳定ID复核：79唯一＝25完整交付＋54开放，18类；31待实施、17部分实施、3待验收、2验收中、1持续。本批完整能力新增0、共享子能力2组；03.2与18.3自然整答仍未通过，不改退出条件、不减少开放数。

## 修复与参考取舍

1. 无工具阶段原规则禁止报告任何上游未直接给出的数值，同次Finalizer却又允许比率、同对象端点差并要求分项求和。公开真实adapter捕获复现该冲突。改为观察值与集合仍来自上游，允许精确已发布操作数的派生计算，保来源/主体/范围/单位/分母及更窄规则；不能从预览重数全集、把差数认成缺失成员或据算术制造物理关系。合计须先有同总体互斥穷尽可加分项，不跨层/端点/重叠时长。性能预分析仍只抄原始量测，不做端点配对或派生计算。
2. 旧空窗探查的`unproven`原会通过session-ANY软提示把当前已选因果链统一降为候选，与已选观察ID精确合同矛盾。公开真实producer先查空窗再查当前链，三处提示真实RED；补实际Finalizer system/user/schema后还复现旧系统答案骨架冲突。修后历史探查局部限制保留，当前总结提示复用已有精确合同，系统骨架同步；不改allowed枚举、原生值、链上业务线索、Trace投影或自动补齐。只有相关ANY信号存在才额外编译合同，普通采样/纯量测不增加该次计算。

参考仓 `core/query_engine.py:283–328` 分离截断前总体与返回明细，`core/llm_contract.py:114–146` 按真实成员/字段回查，启发本批保持结果局部范围而非把某次探查推广到全局。`config/indicators/memory/heap.yaml:35–49,548–576` 将资源行、累计量、同采集字典分开；不将负地址判无效，也不把相邻I/C自动解释成物理配对。`core/extensions/hiperf_binary_parser.py:222–295` 只读解析并按记录时刻排序，`ParsedSample`的时间/CPU/period并非连续执行区间。使用其独立标量oracle核原件，不借BRBE解析器名称授本项目BRBE/SPE能力；不移植文本相似度硬门。

帧专用`FrameEvidenceStatus`/`FrameFlowUnproven`仍有历史汇总适用域风险，留16.4后续公开复现；不称本批已统一全部因果/帧提示。上一批自由解释错句没有新的typed升权证据，不为其重写正确统计器。

## 验证范围

- 算术真实RED为v2；v1是夹具标签选择错误的HARNESS失败，原件保留。GREEN与race各3包、31顶层/17子例PASS；race按启动时版本，不倒签后继系统骨架一句。
- 因果真实RED v1证明当前链被旧探查降级；v2未激活系统骨架而exit0，明确不算RED。补真实附件和PerfTrace后的v3真实RED。最终GREEN v2为2包、10顶层/16子例PASS；正式新分支race v2为2包、5顶层/14子例PASS。race v1正则只选旧相关，不冒充新例覆盖。
- 资源公开SQLite→导出→parser/query及实际Finalizer回归：2包、8顶层/60子例PASS。采样相邻回归：2包、56顶层/52子例PASS；Trace因果投影/合同相邻：1包、26顶层/3子例PASS。
- 最终标准make exit0，CLI SHA `f5f68613f6788fc032a7167cf757f4f44eb31da6fb75c845d01acf9bf515f2d6`。7372仓库输入＋17外部输入冻结，SHA `720d796b34b2fe8df706f25d4368c4ff13d525326d465d7dce266adf76291992`；原件及oracle也在冻结范围，providers仅传路径、不读取归档内容。

## 两例真实自然评测

共同屏障于UTC09:26:57放行，既有`eval/run.sh`各一次、原15步预算，不加问题护栏、不强制工具序列、不补第三例。实际模型请求A为09:27:41.063–09:28:17.506，B为09:27:49.603–09:28:19.456，交叠27.903秒，不仅以CLI屏障代证。结果根 `eval/results/hmc_calculation_scope_sampling_20261026_v1`。

A为原 `trace_resource_identity_inventory.case`，完整2503B authored exported-text，非新真机二进制。进程exit0、机器PASS；5操作/5独立计数观测、地址原始整数/64位位型、0/NULL/空串/未发布等本轮正确。但正文从I/C相邻推“配对式埋点”，把8192→4096→4128说成几乎回到原量级，对未知单位原始数量作异常定性；正确表与有效性边界不能抵消这些解释。03.2原累计量/解释验收仍FAIL。实际投递3/7解释单元[0,1,2]，未投递内容不当污染传播证据。

B为原 `trace_binary_sample_only.case`，完整30,062,585B PERFILE2实采原件，与参考源SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`一致；零模型operator准备新跑独立标量oracle，12检查通过，没有预种生产查询。真实CLI默认准备并取目标164记录/259 cycles权重，两热点各82、权重174/85、CPU0/1及unknown边界正确。机器PASS、完整人工FAIL：正文把两簇记录包络称“两段各约1.5ms的实际执行”，把合法子窗零采样称约122ms“无执行期”并定位停顿；样本类型仍未知，没有调度区间证明这些结论。实际投递2/5解释单元[0,1]，其中unit1已有类似越权。18.3继续待验收；09.4不重复核销，旧错误不称已证偶发波动。

新计算教学实际投递不等于已改善整答；“历史未证探查＋当前已选typed_chain合同”的新作用域分支在这两例未形成，自然覆盖NOT_TRIGGERED，只有公开确定性正反回归通过。未触发与失败分别留账。

## 下一ROI与保留项

完整能力新增0/累计25，已交付共享子能力2组，剩余54项。新确认的参数适用域接缝优先：B两次正式`bucket_ms=20/10`完整传到Query，但`perf_timeline`静默忽略它，实际以缺省`min_duration_ms=1`分桶；schema却只把后者教为短区间过滤，成功回执无忽略说明。代码链为`trace_query.go:264,268,1618–1620`→`query.go:1484–1485,5332–5344`，不是模型拼错或JSON丢字段，不代表实际支持20ms；仅只读定位，尚无公共RED/修复，不归因本轮执行状态错句，不冒称新增PMU能力。独立报告SHA `75d34d10636b092c0ecedff1ec7e634466d45bacbcd8d4686aa7d8689aa959be`随v1封存，挂16.4/18.3/18.4。

随后将帧专用历史汇总与当前合同的作用域问题做公开正反复现，能力槽按有材料的原有限合同轮换代表选择/只读登记；不继续占槽追跑采样/IO求绿。03.2本轮身份与缺值已改善，但累计量/解释未过；18.3入口、完整尾部和原生取数已具备，残门仍是诚实解释。17.6缺新的真实格式/工具/原生平台凭据，05.2/05.3与14.1保协议前提，17.7 WAL零副作用未达仍生产NO-GO。

## 唯一全仓与分代复验

唯一全仓PID59612，UTC09:26:55.324227–09:42:06.193372，exit1：95包PASS、2包FAIL、12无测试；28335顶层PASS/2FAIL/41SKIP，33048子例PASS/2FAIL/14SKIP。250337行均为JSON，无重复终态/未结束项；日志SHA `470efc81c3797ab3b0f06c1c6e2c95bb417c75ff9aae7a132b7dd2b894d7fee5`，7372＋17输入执行后逐SHA未变。失败仅为context旧“does NOT have permission…”和skill zh/en旧“when frame/deadline…”字面pin，没有隐藏其它测试失败。

保留v1全仓FAIL；全部冻结运行结束后先封211份原件，索引SHA `bf1ee4862130f8cfd2e0d7b0d6f6607f431cfee0a745862e525c7fca64c1fd5c`。随后仅迁移`internal/context/perf_triage_measurement_authority_test.go`和`internal/skill/defaults_summary_scope_test.go`两处措辞断言，保更窄预分析规则、背景不升链及全部负控。v2冻结SHA `21de4a77693a5b3b3e3ed772178680436dd83cb0c27ac8a5136e0307f750fb80`，与v1仅上述两测试不同，所有生产代码/CLI/案例/原件不变。

完整context/skill及race各2包、621顶层/392子例PASS，零FAIL/SKIP/重复终态/未结束项；日志分别SHA `f7c17cd18aee087db676c6c1b79338b61ff143e59738d9d55692d111ad655e1e`、`5c233ee6e8a0e22c3a8dee636147107ecc94c4a25edf592d304cc0c61b45b700`，v2输入执行后逐SHA一致。没有第二次全仓或第三例模型，不把局部绿或机器2/2回签全仓/整答通过。

后继另封14件，索引SHA `e603816b8c5671d9d55796bbc668e54491d5ea40524b4eddc8ce5afab2235325`，合计225件；211件v1归档逐SHA不变。独立postpin审查SHA `c29f22ff0d5a0ef49c0f7bb43049417d79d719955a27f9301b8d0732dff0f2de`确认仅两测试变化、全部正负控保留，未自行重跑或预签发布。

代码/测试提交 `b892d3702fc463b1767c5aac136ee974df48cc46`，文档另笔普通推送。原件目录与最终终态定位如下；发布收据生成在两笔实际推送后，不能以本段计划文字代替成功：

- [v1失败原件索引](results/hmc_calculation_scope_sampling_20261026_v1/sealed-index-v1.json)、[v2后继索引](results/hmc_calculation_scope_sampling_20261026_v1/sealed-index-v2.json)、[v2冻结清单](results/hmc_calculation_scope_sampling_20261026_v1/frozen-v2.json)。
- [最终发布核验](results/hmc_calculation_scope_sampling_20261026_v1/publication-v2.json)：核代码/文档父子关系、两提交树7372输入、17外部输入、两份封存、远端main、clean及79/25/54；原全仓仍记`FAIL_preserved`。

逐例证据见[人工审计](parallel_selected_summary_hmc_calculation_scope_sampling_20261026_manual_audit.md)。
