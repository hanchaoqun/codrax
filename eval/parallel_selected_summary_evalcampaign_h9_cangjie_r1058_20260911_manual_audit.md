# r1058 人工审计：显式窗 Trace 折算 / 隔离仓颉声明枚举

## 执行与判定

源码 `f9e821966849`，built `2026-09-11T03:41:05Z`；此前 B1658=`fe1dccfac`、B1657=`f9e821966` 已分批推送。冻结源码和用例，原参数 `go test ./...` 86有测试包通过（未变包部分缓存），再构建对应HEAD。实际 `2026-09-11T03:41:37Z–03:44:40Z`，immutable snapshot `codrax-selected-20260910-204137`，exact2各一次、1200s，不增第三路或重复求绿。目录名使用本机2026-09-10时间，不把UTC日期差误当另一轮。

| 用例 | 机器结果（保留原判） | 人工结果 | 过程 |
|---|---|---|---|
| H9 单基准折算 | FAIL，182s，缺旧1.023/3.309组合 | PARTIAL：投影与主要计算保留；模型解释多处不准确 | 峰值52%；trace_query5、read3；1成文拒绝、2局部patch |
| 隔离仓颉库存 | PASS，68s | PARTIAL：五项清单/包/引用全对；摘要误称继承 | 峰值28%；repo_map2、read0；0成文拒绝/patch，1次JSON字符串兼容恢复 |

从243例（215 read、25 apply、3 plan）按影响、最近覆盖、来源/语言、当前变更风险轮转。r1057已测真实JS写，本批选择H9（最近r1049）及仓颉（最近r1042），不在同一读图题上追跑。H8旧计价断言、H11记录数词面债在开跑前识别并暂不选；H9运行又暴露混合贡献旧断言未跟随现查询值，预审只凭旧注释判断该组合仍有效是不充分的，见下文。未改变引擎或oracle迎合结果。

## 仓颉：普通源码库存仍有效，模型说明不等于事实

工件 `.codrax/output/20260910-204243.240-33870.{md,html}`；日志 `eval/results/cangjie_repomap_fixture-20260910-204137/run-1.logs/codrax-20260910-204139-000-33870.log`。交付树三个源码文件与fixture SHA256逐一相等；只读路径无源码变化。

1. MD13–17完整列出独立 `extend Cart`、`foreign func native_add`、三类Bridge/Cart/App。包均来自对应package声明：demo.cart/demo.bridge/demo.app，不由目录或图中短名推断；MD22–26行号与引用正确。Item是struct、ohSum是普通wrapper，没有混入要求的集合。
2. MD9把 `extend Cart` 写成“Cart继承父类”，原Cart类14–28已闭合，30是独立extend，源码没有该继承。错误出自模型explorer候选说明（log1257/1590）以及finalizer首emit（2492），不是系统加进摘要。
3. 入模log2316说明note非必填/非proof，2327明确库存本身不证inheritance，2335坏note标candidate/unclassified；五个主行2335/2340/2344–46携完整包。没有确认系统缺乏解释边界或互斥合同，不再加正文关键词门，也不系统删改该错误摘要。
4. B1658普通库存正证：没有因workflow收窄而丢任何清单项，没有“清单完整性补充”额外补表，实际2503–05发布2blocks/5citations。此正证只覆盖普通库存邻臂，不冒称B1658负例在live自然复现，更不签B1657图清理（本题无图需求/图产物）。
5. JSON首稿2492将blocks作为编码字符串，2493既有兼容路径恢复，一次成文接受。唯一completion修补872–884是3members/1note/3refs未对齐，原提示允许省略notes；911改为对齐后917接受。它是ok=true的DOWNGRADED，不混成finalizer硬拒绝。表“项目/类别”重复来自模型label与首cell都填类别，不是系统补第二张表，作为P3展示观察。

## H9：机器旧数值失败与模型解释错误分开记账

工件 `.codrax/output/20260910-204437.509-33871.{md,html,root-causes.json}`；日志 `eval/results/real_trace_h9_conversion_single_basis-20260910-204137/run-1.logs/codrax-20260910-204139-000-33871.log`；主查询blob `.codrax/blob/20260910-204139-000-33871/trace-query-result-2a03f0a1.json`。

1. 五次模型trace_query均为17597/13762.791708..13763.024898；三次read消费查询结果，不是源码分析。另有系统frame_root_cause_bundle同窗补采（log1486–1492，2.187s，blob823cbdd5），不能说模型第六次查询。MD95起一份完整投影；MD64起实际占时轴、窗内可消轴和业务span均在。根因旁路9781字节，`status=available`，五项是模型选择，系统未替模型補全十三席。JIT2.388ms在MD17明确关系未证、不入链上根因；邻近/背景保持隔离。
2. 本次typed keva-3为runnable1.319 + running缺口2.286040979 =3.605040980ms，模型MD25/36/52及投影采用此值；旧case要求1.023+2.286=3.309而失败。物理闭包：原trace唤醒13762.927735，链局部窗起13762.927752，首次切入运行13762.928048，窗内R后缀0.296ms、窗外0.017ms。B1636恢复缺失的窗内后缀，旧r1049同局部窗入模值1.023加0.296得到当前1.319；不是回滚合法归账来追旧值。没有重跑旧二进制，证据是原事件、同局部窗、旧日志、当前工件和确定代码改动。预审主51.735针有效，但未逐分量核查混合组合，不能把旧注释当当前精确针。
3. 正确保留的数值：主依赖143.499ms运行/51.735ms供给折算缺口、自身7.305/4.958ms、IO15.304ms、keva混合席3.605/3.514ms。MD72目标状态7.305+1.067+222.632=231.004ms、未归账2.186ms，未伪装全窗完整；实际耗时、累计影响、估算缺口不可混为同一量。
4. 模型解释错误独立于旧oracle：MD21折算公式不成立，把核能力与频率基准简化成仅最高频；MD27的0.476实际来自binder:496_9，keva-1没有已发布的原生ideal字段；MD37实测wall-clock列放折算1.248；MD29自身running缺口4.958/7.305约67.9%，不能说“几乎全部”。MD34/39/40把聚合IO称单请求/折算并暴露`fold=sum_disjoint`内部值。MD48又把keva线程io_wait席的sync_buffer_read_wi借给.ugc.aweme.lite的15.304ms IO家族；后者原row无blocked_reason_caller，不是同线程另席尚可混用。log2025/2033–39/2266及3004/3009已明确主体与口径隔离，typed发布未先混配，不靠系统改写答案解决。
5. 首稿runtime_work_relation选择不符合当前typed选择域，产生1次精确拒绝；log3233局部patch选择原生二元组，3236兼容迁移误放的block_field_edits_v1到block_receipt_edits_v1后accepted。随后requested_dimensions展示advisory再触发一次同值patch，3297同样恢复后接受；停止原因是成功后重复调用防空转，不是活跃流超时或模型回答被删除。不能用模型thinking中的双前缀引文证明实际schema自冲突；实际单前缀已成功。
6. **B1659教学可改善点，不是互斥合同。** log3131原summary只有observed_artifact_fact，缺runtime_work_relation facet；首patch只修receipt，没有补此归属。evaluator当前覆盖谓词要求双facet/external claim/bound receipt，所以展示提示有据；但16930的运行时工作关系提示只泛称facet/claim，未像邻接关系路径给精确JSON，还重复教模型选择已有效的receipt。本批先保留人工失败，下一小批将初始与retry教学同源为精确结构形，并明确已有绑定保持、只补缺的归属，不改schema/接受谓词，不自动选block/工作/结论，不按本trace内容写规则。
7. 模型前端把一般“卡顿”自行选成frame_causality_requested及runtime_work_relation_requested（log577），而原教学338已区分generic stutter与明确帧问题；本轮不从原请求关键词反向改分类，不认作后端schema冲突。主短榜log2007–2016展示8/13并在2017指向其他测量，最终MD886也披露不完整；对“各线程running”仍漏binder等小项，需与完整query/其他入模人口分开审，不能说短榜本身已全给或报告已穷尽。旧raw occupancy/供给子集显示域的已记债保留，不借本批局部正确签双轴所有消费面闭环。

## 流式保护、边界与后续

- 本轮H9最长单LLM39.366s；总182s不是>4分钟单流见证。已有实际SSE测试count3通过34.344s，覆盖隐藏推理/工具增量、分帧字节、keepalive超过旧cap及真实停滞/取消/明确deadline。活跃连接不因4ms或旧4m没有可见答案降级，本轮没有此降级。
- 两例没有Mermaid，图回归仅有B1657公共sequence/flowchart与全仓收据，不能宣称本轮live验证所有图/语言。
- Gradle当前执行来源B1651b仍P1：不能把旧XML授为当前断言；只清scope还会污染Passed/失败归类。单纯截断所有未绑定XML会损失真实fresh结果断言能力，不混进本批悄然启用。完整afterTest来源方案、原生cache/multi-task验证及历史证明债继续独立施工，Maven/CTest既有绑定不回退。
- H8/H9/H11测试契约按现实现与已裁定语义独立复算；确定性数值验证与LLM回答质量应分开，禁止为单轮措辞或模型误读加硬门或改引擎凑旧值。
