# HMC352 计算限定候选与资源、只读负向失败记录

本地2026-11-04，UTC2026-11-05；仅main，基线9ee94848f57928716b998d24a82950ac2c3dc33c。18Go候选（13生产/5测试）未提交。完整能力累计28/本批新增0，正式源码子能力0，1共享候选组延期；79唯一/51开放及原状态、checkbox不变。失败代仅提交五文档，不生成成功publication。

## 有界候选与参考发现

旧码公开RED复现：compute_supply_balance的数值/summary进入实际成文，原生缺频率回退限定却丢失。候选ValueContext将字段适用域、计算方法、逐CPU可用性/真实支配频点覆盖和全部原caveats随共享观测保留；compact的Notes/Summary裁剪不再裁掉数值语义，checkpoint、整条成文预算、语义复核复用同源字段。超预算不选整条，而非只留裸数值。未从问答关键词授硬门，不授来源/完整性/物理关系/根因或完成权限。

参考freq_postprocess_ops.py的freq_compute_load、freq_compute_load_by_thread.yaml及chip_config.py：1024×f/fmax用于归一化估算，不是实测IPC/GB6/指令。参考缺chip配置返回0不能搬为硬件零；本项目保持原合法数值与自动补齐/因果能力，分开模型支持和实际频点覆盖。无新模型发射字段或重复教学。

用户完整网易云82838676B/SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4，元宝63659046B/SHAbe59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c，均实际query→Finalizer；新限定外的旧数值逐字段相同，原件、ledger及原因果投影保持。状态任务未选无关计算行不强制提权；独立选中原计算的实际成文消费者保完整限定。零模型接缝不冒第三例或自然分支通过。

## 唯一全仓失败，冻结后未修源码

最终相关5包200顶层/166子PASS；race×2五包26/44，原因果四包21/141，完整双实采1/2 PASS。早期新测试浮点字符串/JSON转义/必选支持计算前提失败均保原，仅修新增前提，旧断言未降低。

[冻结](results/hmc_value_context_resource_registration_20261104_v1/sealed-v1/0373-codrax-hmc352-frozen-v1.json) SHAcae2fbb2393ab17c998e90ba61cc96b9f3ff114a61701b8734ecef39b3c0d807：7644输入/551外部/29快照。唯一full UTC06:52:03.064654→07:06:27.448804，OS1/源前后true，log SHA9292a807ae2b297baf0a4cb1cfff068339bcf0bf6f5915fe27acdecd44e4c2bd。最终96包PASS/2FAIL/12无测试、28770顶层PASS/2FAIL/57SKIP、34597子PASS/1FAIL/14SKIP，268551纯JSON、重复/未终态0。

两个真实阻塞：TestBusContextCopylocksVetClean发现新整件测试复制含sync.Mutex的AgentContext；TestHMC325NativeRetentionFieldOwnershipInventory/ObservationRecord发现ValueContext未显式分类。下一代分别用既有ShallowClone及显式native-core所有权修复，原针不弱化。早期root决定只写首失败，最终raw/stats补全第二失败，原决定不覆盖。不进行第二full或第三paid重跑求绿。

## 恰好两例并行原评测

原资源case与原缺来源终态harness各一次。精确唯一请求A9/B6、歧义0、7已证重叠对/最大4.197秒，非以进程重叠冒模型重叠。[主审](parallel_selected_summary_hmc_value_context_resource_registration_20261104_manual_audit.md)保留有限合同。

- A资源smoke PASS，03.2有限和整答人工FAIL。5操作/5快照及精确原值/合法差值正确，仍把合法signed位型称越界/哨兵。实际213850B INIT、4原生JSON均无算力计算，新增限定自然NOT_TRIGGERED，不以旧错误未出现签PASS。
- B原测试进程FAIL、目标receipt MISSING、恢复负向终态NOT_TRIGGERED。空对象changes经既有兼容变数组后因空path合法拒绝；controller允许第二planner dispatch并安装零变更登记。旧harness单dispatch前提先FAIL，未进入恢复；不冒预期负例PASS，不改冻结断言，也不把历史source完成当本轮终态。

## 全部51项ROI

逐ID核账见[原census](results/hmc_value_context_resource_registration_20261104_v1/sealed-v1/0367-codrax-hmc352-census-roi-v1.json)；退出合同仍见[统一清单](../docs/design/hmosperf_implementation_tasks_20260917.md)，以下不新增稳定ID。

| 顺序/数量 | 原稳定ID | 下一步 |
|---|---|---|
| P0共享7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 先修已证确定性两针，再自然覆盖计算限定，统一解释适用域/图与预算 |
| P1材料6 | 08.3、18.5、11.2、03.2、14.1、18.2 | 原IO/资源/代表有限验收和只读正负终态轮换，失败不误销 |
| P2复用11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 有真实材料的二进制、CPU/IO/时钟/业务/资源复用；08.5限定先随P0 |
| P3协议材料21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 保版本/材料依赖，13.2模型限定随共享修复，未接协议不整体销账 |
| P4新域6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 媒体/可选适配/建议write链，审批与隔离不变 |

充分事实后的旧采样/IO偶发解释留账，不无限重复单题；只读旧单dispatch前提与恢复未触发独立记录。18.3已交付不重复计数。下一代缺陷轨安全clone/字段分类与自然算力问题，能力轨轮换真实二进制，不补第三例给本代翻绿。

## 封存与发布边界

[709件索引](results/hmc_value_context_resource_registration_20261104_v1/sealed-index-v1.json) SHA93b6d2cf2a26dbfd431ec3073428e3f5ae68a96c151468e9cd9676c2f5e9c89c；seal SHA1b7c4be08906c08484d34c3cc62824db07887371aaa5f4573c0d73ed0aa83a67。前53代24231＋709＝54代24940件，按代非全局去重；物理核前351686/失败344554，其它历史链头。大原件完整身份引用，编译物/失败payload/INIT/HTML/MD本地ignored保留，不读/哈希/复制providers。

五文档的main/远端同代仅由failed-audit-push另核，绝非源码发布收据。候选源码暂留main未提交，新代验收后方可交付；不覆盖本代FAIL/NOT_TRIGGERED。Trace投影/自动补齐、原4根因/闭合睡眠、链上业务/语义线索、合法计算与重试保持，WAL零副作用前不接生产。
