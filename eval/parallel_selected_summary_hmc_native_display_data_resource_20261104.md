# HMC351 原生显示数据复用与资源身份轮换

本地日期2026-11-04（America/Los_Angeles），原始过程UTC2026-11-05。全部在main，基线0a64113179530f0d45faa5677c1bbb6c711d9b34，源码a10e9ea14edbf385aba6ef1eab7ec4ffa7f70b42，11Go/431增11删（8生产/3测试）。冻结后源码/测试/教学/helper/case/oracle停写，仅终态审计和五文档收口。前350正常Git推送与原publication均已确认，旧失败不覆盖。

## 结论与分账

- 完整能力累计28，本批新增0；源码交付1有界子能力/共享组，不冒完整父项。
- 原79稳定ID＝28完整＋51开放：18部分、29待实施、2待验收、1验收中、1持续。所有checkbox和状态不变，root closure=none。
- 原自然元宝/资源身份各n1/15steps，两自动smoke PASS，完整人工0/2；03.2有限FAIL，自然重复展示目标FAIL、私有资格分支触发未证。公开/整件接缝PASS不倒签自然FAIL。
- 新已证高影响P0：缺频率时回退限定未随数值进入实际成文上下文，详[手审](parallel_selected_summary_hmc_native_display_data_resource_20261104_manual_audit.md)。旧IO/采样、权限/代表/图、二进制/只读正负覆盖独立留账。WAL生产NO-GO。

## 一组有界共享修复

旧码公开query→actual Finalizer→emit→render RED证明：同一完整原生时序因不同查询Notes，正确地不满足旧整发布身份，却使数据体重复。保留原nativeFactIdentity及原权限；另用现有生产者私有见证，建立只供显示的完整数据key。

key仍保全部原生精度/端点、当前物理源代次、source/target/窗/钟/行及表label/列/行等，只有仍逐query输出的Notes不参与数据体复用。当前原件标题basis/角色也重验；JSON恢复、变异、未知窗、成员域/选集、其它生产者不能借权。仅system完整表可引用前一完整体，模型自选表不压掉，所有doc block/receipt/来源标题/Notes保持；短“数据表N”是读者引用，不显示长内部ID。普通relation/claim/scope回调不绕过，renderer不重新开源、不改doc/ledger/原总体或因果/完成权。

参考仓root/core/preprocess/docs指导、完整architecture/Golden Rules及context_index.py已读，采用索引/产物/阅读角色分离，不搬path-only代次、value覆值、全局业务must_read或噪声/fingerprint文字硬门。无新增模型字段、prompt重复或问答关键词硬门；未声称全部IO/资源生产者、全输出字典或上下文token压缩。

## 原材料与测试

用户目录的完整网易云ftrace82838676B/559592行，SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4；完整元宝63659046B/412546行，SHAbe59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c。并非截取微片替代原件。两个actual query/Finalizer/公开emit/render分别保3735/836行只显示一次、两独立query凭证/完整Notes；doc、ledger、原件和投影不变。零模型接缝不是第三例，也非独立调度oracle。

RED-v1原件含新增测试把HTML实体与原路径直接比较的前提错误；只校正新增测试实体解码，RED-v2仍真实复现automatic/explicit-owner两条旧码重复，旧断言不变。终版相关四包21顶层/148子（3顶层SKIP）、race×2四包36/268（4顶层SKIP）、原4根因/闭合睡眠四包21/141、完整双原件1顶层/2子PASS。终版相关log SHAc4e8b00c33d3c7de4c7ae2e7cdb7a6569cf6e39e4833c81c2be3c6b40be64f36；race SHA3d3ccdbef0fa39409b9ca2a1879fd318336c3b375f9f975ba139d23c714f7855；双整件SHAda7e005e1f181cc3536b914878386cc68cbd206bfd20baa7947f88d49f1d8e9b。

[冻结原件](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0525-codrax-hmc351-frozen-v1.json) SHA1eff7266895fac6999a99d6c2cbc9c1b61241a509c3420548bf232890861df4c，7637输入/365外部/21快照。唯一full UTC03:35:49.435337→03:49:49.010127，OS0/源前后true：98包PASS/12无测试、28765顶层PASS/56SKIP、34595子PASS/14SKIP、268490纯JSON、FAIL/重复/未终态0；log SHA0a1da8d46167fea21e85af7a7c8fc7a129aa9f6e25dfcd2d565531b5c72a5936。CLI SHA c135efe35c76607965cd371b3e9b5974976213b4c3895b54bdb7d4ba2d689ee2。

## 恰好两例原自然评测

启动/argv/物理原件屏障和源前后核验保原。精确唯一请求/响应A14/B8，A另8歧义忙组不猜配对；12已证跨例重叠对，最大19.213s，不以进程或屏障重叠冒真实模型重叠。A原日志64请求/64响应、B8/8，与85/8工具声明不是同一计数。

| 原例 | 实际输入/上下文 | 正确部分 | 人审/欠项 |
|---|---|---|---|
| A 原元宝 activityStart | 完整63.7MB；17原生JSON；INIT98913+49606+327256＝475775B | 两实例和主子阶段inclusive/self；128业务行与独立整件整数B/E0差异；2211最终时序行保真 | 整答FAIL；已闭实例被说尾部超窗，父完整/裁窗错配；两份836行重复，自然复用目标FAIL |
| B 原资源身份 | 原2503B authored文本，不是raw binary；5原生JSON；INIT115901+8426+172884＝297211B | 5操作/5独立快照、精确signed/hex/子类缺失状态、合法差值；数字吻合不证配对 | 03.2有限和整答FAIL；原始身份/有效性混用；缺频率限定丢失令正文断言零低频损失 |

A实际unit0将闭合与可能超窗混写并进入INIT；其它坏self单元未投不冒污染。B legacy完成reason是原始字段/状态事实，未含供给排除结论，不冒reason污染。A/B全部85/8参数blob可解，声明与参数分列不相加；非UTF8原日志9/1行、执行拒绝/修正原件保留。缺失引用候选93/8含日志截断/ellipsis，不当作93/8真实不可读源。无图/Mermaid/浏览器像素QA，图不暗加为退出门；HTML/MD仅源级核验。

## 全部51项ROI，无重漏

原退出合同和依赖见[统一清单](../docs/design/hmosperf_implementation_tasks_20260917.md)，逐ID核账见[冻结census](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0489-codrax-hmc351-census-roi-v1.json)。以下不是新增稳定任务。

| 顺序/数量 | 原稳定ID | 依据 |
|---|---|---|
| P0共享7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 先补数值与实测/模型/回退/未知限定同源交接，再补解释适用域、自然复用/整组预算、统一JSON与图；不靠措辞硬门 |
| P1材料验收6 | 08.3、18.5、11.2、03.2、14.1、18.2 | IO在途、轮换、代表、资源位型/统计与只读登记；原出口验收，不新增WAL/栈/图门 |
| P2原生复用11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 业务/时钟、IO/CPU、帧连接、真实二进制/安全SQLite、可比性与资源代次；08.5的限定缺失随共享P0优先 |
| P3协议/材料21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 版本化协议/真实材料；13.2回退/模型资格先随共享P0补，不把其它未接协议整体销账 |
| P4新域6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 媒体生命周期、可选静态适配和建议write链，保持风险/审批/worktree/验证 |

新共享P0挂01.3/16.4/08.5/13.2/18.4，不另建重复ID；自然显示资格/预算及原窗/闭合端点到解释继续留账。随后完整轨轮换真实材料二进制和只读负向终态；本付费pair未跑这些不签PASS。旧采样/IO充分事实后的偶发误述不无限阻塞，18.3已交付不重复计数。

## 原件封存与发布

[686件索引](results/hmc_native_display_data_resource_20261104_v1/sealed-index-v1.json) SHA40f68e1ddd30767d3c616d5a22922f9eb196fe0280c2e77ac5436e3b9807da55；[seal](results/hmc_native_display_data_resource_20261104_v1/sealed-receipt-v1.json) SHA146799affa98d856d6909e0867ac137cd9e35154e45b35c19989eab5847a4ccf；[root acceptance](results/hmc_native_display_data_resource_20261104_v1/sealed-v1/0427-codrax-hmc351-acceptance-v1.json) closure=none。前52代23545＋686＝53代24231件，按代非全局去重；物理重核350621/失败344554，其余历史链头。大trace按完整身份引用，124MB编译物与失败原始payload/INIT/HTML/MD留本地ignored树，不塞Git；providers不读/哈希/复制。

源码和本批五文档为main相邻提交，远端/清洁状态以[独立publication guard](results/hmc_native_display_data_resource_20261104_v1/publication-v1.json)为准，不以本地commit或候选同步倒签验收。Trace因果投影、自动补齐、原4根因/闭合睡眠、链上业务/语义线索、合法计算与重试保持；WAL未满足零副作用前不接生产。
