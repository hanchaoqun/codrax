# HMC334：逐例人工验收与不销账边界

完整原件：[本代index](results/hmc_container_shape_representative_binary_20261102_v1/sealed-index-v1.json)；[A原审计](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0160-codrax-hmc334-A-audit-v1.md)、[B原审计](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0182-codrax-hmc334-B-audit-v1.md)。本页汇总而不替原件。源码子能力、原smoke、原有限合同、整答、自然覆盖、全仓和publication分别判定；完整交付新增1只对应18.3。下文INIT字节指collector保留的原日志节块（含日志分隔符），不是provider wire精确字节数或tokens。

## 1. 终态矩阵

| 维度 | A原代表自然入口 | B原sample-only二进制 |
| --- | --- | --- |
| 原问题/预算/次数 | 原样/50steps/1 | 原样/15steps/1 |
| 实际进程与源前后 | OS0/true/true | OS0/true/true |
| 原operator或case | PASS | PASS |
| 原有限合同 | **11.2 FAIL** | **18.3 PASS** |
| 正文/表格/HTML完整人工 | **FAIL** | **PASS** |
| 图与图修补 | 无图，无新增覆盖 | 无图，无新增覆盖 |
| 本批新容器反馈自然命中 | NOT_TRIGGERED | NOT_TRIGGERED |
| 本次只读登记 | NOT_RUN | NOT_RUN |

唯一全仓OS0/前后true、98包28709顶层34289子PASS；实际配对36/13、歧义0、最大重叠53.203秒。启动、barrier、请求完成及有限判决不同；原第333负控PASS不能倒填为本批又运行。

## 2. A：先验收实际执行，不用结论替代过程

输入是两个已有SQLite夹具的各两份完整复制；原件SHA、Git HEAD与独立SQLite oracle在冻结前校验，oracle不进模型目录，无选择/查询种子。唯一CLI与B同字节；真实自然discover自己选择resource_stack而非上代错event_search，accepted线程101/202、半开[10,10.05)及每组2被编译成8个计划。

| 线程规格 | 选中且实际执行 | 未选原计划 |
| --- | --- | --- |
| 101、[10,10.05)、resource_stack | session-b、session-d，各2次成功 | session-a、session-c，not_executed |
| 202、[10,10.05)、resource_stack | session-c、session-a，各2次成功 | session-b、session-d，not_executed |

最终同代15记录＝8声明+7额外；11已尝试/4未执行、15次attempt＝13success+2empty。真实2个查空是额外FreeEvent子串搜索，不能代替资源事件语义；另有literal thread="202"的unknown身份查询，其0匹配不借为精确目标真零。普通capture-all补查不把未选计划改成代表或已执行。

4224行唯一production tuple：index SHA3c6f195ae1d7d26631393430c04a9f4218ea6bc50bac6455780cf45c41e8e6e1、selection SHA7af59f67a0025c0e4b3b0845cb2f2b9db7a2353c8735e8c8ce9fd993d1723c5c。原冻结operator只审该tuple，OS0/current-index-audit.json为representatives_smoke_pass_human_pending；无新query、无latest拼接。这是结构通过，不是完整人工通过。

8个selected/全采集资源payload对独立SQLite原字段逐项核14事件载体/40帧载体，ID/tid/时间/size/address/end_ns及帧depth/IP/符号和库ID/offset/vaddr全部一致。a/c各4事件11帧、b/d各1/3；窗口外10.05事件未进入。free空洞/未知、重复depth0/歧义depth1保留；真实展开unknown，完整帧载体不造调用边。申请/释放独立，768/128/512bytes正确而非净存量或CPU时长。

## 3. A：为何结构绿仍不销11.2

实际第二条INIT传递正确的2组规格、全部4selected、4未选总量及其中3条个体；最后1条（202/b）因导航预算省略，明确`unselected_member_records_displayed=3 omitted=1`，没有声称全名册展示。15记录/11已尝试/4未执行及attempt口径在场。第三条仍有原生表/Notes；未发现OOM或主量测丢数，缺的未选导航另挂预算覆盖债。调查却把已有完整的2组选择规格改为a/c、b/d内容形态：最终7解释单元仅unit0/1整项投递（省3595B），错unit0确实进入实际INIT并被正文重复。不是未传reason的污染；不能拿一个未选导航省略解释已显示分组与选中身份被改写。

封存后独立更正：原A审计`0160-codrax-hmc334-A-audit-v1.md`（SHA cde303fb8bbc62a513eedb2ab96ee6ef54ff12b526114035da56cb10e7c2d271）曾过宽写“4unselected记录”全部到INIT，撤回这个投递完整性表述。精确原件为`A-materials-v1/log-01-finalizer-init-02.txt`（SHA c6f8f4ae8d2d5f50eaf736270f28ffec7b972c828fb5ebb9da5ea714925cdaec）21–25行：3个个体＋1省略，原索引仍4未选。原封存/断言/问句不改、不重封追绿，A有限及整答FAIL、B判决均不变。本页受相邻文档提交保护；publication并不表示重写或重审了不可变原封存。

最终正文/两张重复自写表的A/B“组”是按内容重分的四个文件，不能替用户原线程×窗抽样组；内容对照本可额外表达，却被说成原代表分组。进度再把额外两查空叫“选中”，和原4selected全success矛盾；没有逐组准确列出四个未选原计划。故原有限合同FAIL，不添加新算法/写回/WAL门，也不因旧§328有限PASS替本轮豁免。

另有源深度0/1/2被写为malloc→reserve→render方向链。问题不是箭头字面本身，而是方向调用语义没有独立调用/完整展开证据，且和原生“不是有序调用链”边界冲突。正文值基本正确不豁免该关系错误，整答FAIL。

7次completion处于两个正常explore dispatch，最后1次完整发射即受理，无patch/container形状失败；这不算多跑模型case。A55工具声明全部可解析，不和55参数blob重复相加；15查询、11唯一JSON，真实重复尝试保留。

最终MD51094B/SHA5cd2de2831b7dfcb1ea1af3d35c00bb4792b67557d12a923d967690d90f69dde，HTML2834923B/SHAff75bf1bb2f76b351465f9ded210df980f9e7f7b04985c7628120610ecbb3241。19表＝2自写+13原生资源+4原件元数据；原生数据/终态附注与索引同代，自写错误正文与HTML都有。表比较原false只来自首表code反引号，非渲染丢数；source-only，不冒像素QA。无图/附件图或局部图patch。

## 4. B：完整原件、真实查询与原有限边界

原始test_brbe.data30062585B/SHA da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632，普通CLI真实完整解析12000条；bundle11596B、预览14549907B和原件大小分列。非converter stub，不冒BRBE解码。8次真实Explorer查询/8成功结果，7唯一JSON；重复目标时间线不算第9次查询。遗漏目标/窗的地址检索自然自动补齐；明确扩大窗不改。

目标半开[486489.4,486489.6)在完整所选1393样本中命中tid1609/TGID1514/generation1共164条/259cycles；两个完整深度0叶，各82条：内核地址174权重、OnDataEvent_85。记录占比各50%，权重占比67.18%/32.82%；函数时间包络分别[.588096,.589609]与[.464088,.465612]（前缀486489），CPU1/CPU0仅记录坐标。树2节点、164/259守恒，单热点4引用预览/78省略不否定完整聚合。宽窗515/1156、412/932有自己的范围，终稿不借为目标窗根因。

最终两原生表与正文使用同一目标、单位、窗口、分母和包络。unknown保持执行状态未确立；cycles不换耗时或CPU忙碌，IP-only不猜内核函数或完整栈，122.484ms记录间隔不当成阻塞/执行时长。明确缺调度、唤醒/off-CPU栈及标定，现有分布不足以解释停顿；没有邻近背景升主因。原HMC18.3剩余反例边界通过。

event_search分别164匹配/50返回和过滤82匹配/20返回，终稿未拿预览证明全集不存在；其同段总述宜显式再写过滤集82，是表达精度建议而非捏造第二查询总数。下一步配置/频率建议须结合前文标定CPU频率条件，不视为本次已把cycles变成时间。历史CLI/REPL尾部与显式窗子路径已验证，不暗加所有未来格式/设备/图的退出要求。

## 5. B：上游债与终稿分开

2次completion首缺reason拒绝、次受理；1次full emit受理、0patch。受理5单元中的unit3粗算两段各10ms/180ms无采样、unit4说CPU消耗、reason说CPU执行，仍是上游错误，但本次实际只投unit0/1，未投后三个和reason，不能冒最终上下文污染。unit1把内核地址“其它线程没命中”的负断言贴到回调单元，确有适用域错绑；地址当前不认证该文字，最终答案未重复此断言，共享债仍开放。

同一finalizer3条INIT原字节88046/9073/225901B，显示2/5单元、省3159B；两个可选引用描述省略，新描述自然正向NOT_TRIGGERED。完整原生unknown/量尺/包络和缺调度边界均在实际INIT，能支撑本次正确答案；不拿同次3消息当3次成文，不冒未见provider wire。

最终MD29089B/SHA ecec0cb0bfc59d5d7e1c0b7151e6990ec0907d3dd722f4fe918afbaa65f841a8，HTML2809223B/SHA9d33c5289cd2b3ce29078939aa051e26a8d5dba9b11a5389c6f8c1402da32e94。4表＝附件/2热点/元数据；HTML与MD单元格一致、surface摘要匹配，无图或附件图，source-only。扩大窗自动质量附录按自己总体保留，不被主答借为原因。

B有限及整答PASS，只关闭累计原18.3；旧采样FAIL原件全部保留，不声称波动已根治。性能advisory210s、context估计122700/61%、4次history prune留成本账；首分析结构重试和只读shell失败保原，不归功本批新反馈。

## 6. 发布与后继边界

新共享反馈只有公开RED→GREEN、相关/race/full证明，两例均未触发，不声称自然修补轮次下降。完整网易云原件82838676B实测1次query/0模型，原627.645/194.581账户及实际消息描述通过；没有第三例模型或重新全审旧oracle。

[root决定](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0204-codrax-hmc334-acceptance-v1.json)只授权18.3；A有限/整答失败、B上游错绑、可选描述/容器自然未覆盖分列。旧IO/权限/图、全局预算、h2/h3、更多格式/平台、只读登记自然/负向轮换继续留账；WAL零副作用前NO-GO，Trace投影和自动补齐不改。[publication](results/hmc_container_shape_representative_binary_20261102_v1/publication-v1.json)只核main/远端/冻结/全仓及本次明确闭项，不为其它分支或历史失败补签。
