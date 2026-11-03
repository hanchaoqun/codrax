# HMC341：紧凑完整身份候选、代表规格及sample-only失败审计

2026-11-03，main基线830a14ee0b663294807dd0be164521bee4ed7f05。79唯一＝28完整＋51开放（18部分/29待实施/2待验收/1验收中/1持续），勾选不变。完整能力新增0；本批已交付子能力0，共享候选1组；剩余验收欠项4组。唯一full失败，本次只推失败文档，不提交候选源码、不生成成功publication。

## 双轨与泛化方案

完整轨轮换原代表自然入口及sample-only真实二进制；共享轨先公开重现§340三旧FAIL，分离完整来源凭证与运输定位键。铸造时完整SourceRef（完整query/源/owner/raw/payload）全长SHA-256作bounded opaque ID，后续时钟/逐行来源装饰仍保原；不合并相同payload的不同查询。两个展示消费者统一精确新旧格式解析，完整publication/Notes/字段/来源/窗口/总体与当前私有读取门仍独立。8Go/341增19删，无原断言/预算/问题/oracle放宽。

参考context_index/llm_contract的来源导航和字段归属、select/package的manifest与独立交付意图；不复制参考value覆盖、固定5/50、缺值补0、latest恢复、容差/文本/fingerprint权限门。无问答原文关键词硬门，保Trace投影、原4根因/闭合睡眠、自动补齐、链上业务与合法计算/重试。

原库存9查询14成员与45558B自身段保真，原整提示127029B通过；HMC333 Extract/来源导航及HMC230/HMC275/HMC302展示回归恢复。新测试误把后装饰ref当铸造ref、遗漏两个旧坐标消费者的RED及全部中间失败保原，旧源绿不代最终源。

## 冻前通过与唯一full失败

最终相关7包452顶层/1303子、race×2三包72/268、原因果守护3包21/141、代表公开68/105、sample-only独立oracle PASS。[最终收据](results/hmc_compact_identity_representative_binary_20261103_v1/sealed-v1/0443-codrax-hmc341-prefreeze-final-checks-v1.json)保完整过程。完整用户抖音235422121B/1696481行实际3nativequery/0模型，冷199及热200/201后两同payload/不同ID、原13行/self227.101ms/未显式选择时actual Finalizer说明通过；6raw/payload清理前复制，源前后hash不变。不冒重跑旧338独立整数oracle或第三例模型。

冻结7602输入/666外部/16快照，SHAc88343cf2c8872b01680b5217907be4d6656c52c22fc49c397b45d391bc631b6后停写。唯一full09:51:19.383750Z→10:05:30.544638Z，OS1、源前后true：97包PASS/1FAIL/12无测试；28727顶层PASS/4FAIL/48SKIP，34373子PASS/1FAIL/14SKIP，268654纯JSON，无重复/未终态。log SHAd3c779daf057deead61fdcf4aa32f908dc3840ca5435ef046f5593424a3eae4d。

- XLANE3两步typed target、混合legacy无名板、拓扑指纹三旧测试仍按旧运输ID前缀选步，新opaque下未命中。负向删除实际未执行、两步误归p1；不能冒生产关系已坏，也不能豁免FAIL。下一代用精确SourceRef选步、补负控确实命中检查，保全部数值/板/指纹/关系断言。
- 原B1631 full-artifact window_stats确实丢D状态账户主范围：same-result来源检查已通过，但完整覆盖与账户ID namespace不一致，主范围降supporting。下一代先统一铸造前parent receipt，不能放宽consumer/source门或原断言。该末段FAIL由最终raw/full统计保留；冻结中只读定位稿只记录当时前三项，不追改原件。

本代不再full，不提交候选源码。

## 两原例各n1、真实重叠

| 例 | 原入口 | OS | 原检查 | 有限与完整人工 |
|---|---|---:|---|---|
| A | 原代表自然题50steps，四全量构造SQLite非实机四录制 | 0 | 原operator FAIL/OS1 | 11.2有限FAIL、整答FAIL |
| B | 原sample-only15steps，30062585B真实二进制 | 0 | 原case smoke PASS | 本轮FAIL，不倒改旧18.3关闭 |

实际请求A15/B10精确配对、0歧义、最大真实重叠30.669s，不用barrier代签。A自然选thread_timeline，8计划2组4选4未选、17记录14尝试/9extra、终态index/selection同代；原resource_stack选择规格未满足，后补资源查询不能改原选择。8资源JSON10事件/28帧对独立SQLite oracle 266字段零差异，正文仍栈深度升方向、缺调度区间误称仅截断；全部4未选已到实际INIT，非旧缺尾。

B目标164/259、82/174及82/85、CPU记录坐标/完整包络正确；完整2节点聚合与event_search预览独立。正文却未知sample kind升执行位置、未解析IP升函数入口、用预览截断否定完整统计。正确表/尾注不豁免，旧采样解释债留原。实际只投[0,3]/5解释单元，unit3总体质量误套热点已转发，未投坏文不冒污染。只读登记本批NOT_RUN，不拿340负控补签；图/patch与其它二进制未触发。

## 全部51项ROI

| ROI | 数量 | HMC稳定ID（省前缀） | 顺序 |
|---|---:|---|---|
| P0共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 先统一parent receipt/坐标消费者准入，再任务事实族与成文复用 |
| P1现有材料 | 6 | 08.3、18.5、11.2、03.2、14.1、18.2 | 轮换原有限资源、IO/二进制与正负只读终态；不扩合同或猜统计协议 |
| P2复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 真实材料的端到端交付 |
| P3协议/来源 | 21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 先证来源/协议/权限，WAL不绕零副作用 |
| P4新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 依赖及真实材料齐备后 |

[稳定ID核账](results/hmc_compact_identity_representative_binary_20261103_v1/sealed-v1/0309-codrax-hmc341-census-roi-v1.json)覆盖51ID无重漏。4组欠项是：全采集主范围/测试准入；任务规格与选择执行；原生/正文解释；自然权限/图及剩余协议覆盖。前两先补已证系统/工装缺口，充分事实后的偶发误述保原，不无限追代表或采样同题。原退出条件不增图/算法/全格式门槛，WAL生产NO-GO。

## 原件及失败发布

879件index SHA14c4c890940b6209ba485e2e5a9d78bf2d895f0fb5815ee6d7de7a281bbf9ced，seal receipt SHA0bd288c0008b51bf65bd1f40ad9b5a4052842c39b2e7814041b873946d87f992。[root closure none](results/hmc_compact_identity_representative_binary_20261103_v1/sealed-v1/0224-codrax-hmc341-acceptance-v1.json)，两原例完整人工FAIL不销账。[逐层手审](parallel_selected_summary_hmc_compact_identity_representative_binary_20261103_manual_audit.md)区分原值、投递、解释和未触发。

旧42代17687原ROOT不改，43代按代18566非全局去重；物理重核340的419和最新成功339的624及更旧链头，不冒全历史payload重hash。大原件仅完整引用，providers含synthetic不读/哈希/复制；本地忽略原件不冒全部上传。失败340/341无成功publication；本次只推五审计文档，源码候选留main继续新代修复，不迁其它分支。
