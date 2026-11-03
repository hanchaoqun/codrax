# HMC340：查询身份候选失败审计、有限IO与只读负控

2026-11-03，main基线34264d4ec6e002ebbc7382477fcb9acdc47baac9。79唯一＝28完整＋51开放（18部分/29待实施/2待验收/1验收中/1持续），原勾选不变。完整能力新增0；子能力本批交付0、候选共享修复1组；剩余验收欠项3组。本次只提交审计文档，不发布候选源码，不生成成功publication。

## 双轨与共享缺口

完整轨轮换原自然IO08.3与原只读缺来源负控；共享轨公开复现不同真实query返回相同payload、SourceRef.QueryScopeID不同但旧观测ID相同，导致正确冲突检查隐藏原生表。候选把新typed ID与完整原件覆盖归现有完整query namespace，保无query旧路径、各查询/精确重复稳定，不合并查询、不放宽冲突门/因果/完成权限，不加问答原文词门。3Go/165增5删未提交。

对照参考context_index路径导航与llm_contract字段归属，不把路径/同字节当query，不复制step/type的value覆盖或容差/文本/fingerprint权限门。IO参考io_ops的桶相交数/发起桶不是本项目已配对半开区间积分，不能用来替换原值、合并RQ/BIO或推设备队列。

## 冻前通过不代全仓

相关7包388顶层/1171子PASS，race×2三包26/64、原4候选/闭合睡眠守护3包21/141 PASS。公开旧码RED、早期GREEN及所有尝试见[原始审计](results/hmc_query_scope_io_registration_20261103_v1/sealed-v1/0302-codrax-hmc340-prefreeze-attempts-audit-v1.json)。完整用户抖音235422121B/1696481行实际3nativequery/0模型，保一次冷发布后两热不同limit查询，后两同payload/不同ID，全部原13行账户/实际未选Finalizer自身说明通过，源前后SHA不变；6份raw/payload清理前复制。初版cold/hot元数据不同的夹具前提FAIL保原，非产品RED。旧338整数wall oracle不冒本代重跑或scheduler oracle。

CLI/test正常build与0模型0工具startup/preflight后，冻结7600输入/279外部/13仓外快照，SHA36f280cd4c4a336bd39c2fd659b2da19c05a6e0b9047be46ed1f7d590329f37a。此后源/测试/教学/helper/case/oracle停写。唯一full09:06:49.793928Z→09:21:07.951166Z，OS1、源前后true：97包PASS/1FAIL/12无测试；28726顶层PASS/3FAIL/48SKIP、34355子PASS/1FAIL/14SKIP，267528纯JSON，无重复/未终态，log SHAe4a265a226689144eb0c93a39fc289f8760aceee1bd582eb620b416040c5f716。

三项失败：①重叠event inventory自身46499B/9query×14完整成员通过，但整提示旧130854→136274B超过原门；②HMC333 Extract引用上下文未传；③HMC269来源导航135574B超过原预算。详[回归登记](results/hmc_query_scope_io_registration_20261103_v1/sealed-v1/0258-codrax-hmc340-frozen-regression-audit-v1.md)。最后非变更比较因fatal未执行，不能声称该断言PASS；不提高上限、不放宽断言、本代不再full，也不归因模型波动。

## 恰好两原例各n1

| 例 | 原入口 | OS | 原smoke | 完整人工 |
|---|---|---:|---|---|
| A | 原自然IO15steps；完整构造fixture1839B/1query | 0 | PASS | FAIL，08.3原有限也FAIL |
| B | 原真实planner＋脚本恢复控制器＋新原生执行 | 0 | PASS | 限定PASS，非自然controller/fullCLI |

实际request/response精确A9/B2，歧义0，最大可证重叠3.074秒，不由barrier推定。A四组24数、8成员及12原生表正确，但正文BIO全窗在途说多数空闲，受限请求统计升物理队列排除排队/带宽/线程等待，IRQ完成线程升直接原因；正确表/尾注不豁免。实际5单元只坏unit0投递，省9072B；未投坏unit2不冒污染。单query未触发新碰撞修复分支，不能签自然多查询PASS。

B来源仅真实apply，无verify终态；新进程/新invocation运行现有1断言/2命令，源码/测试/HEAD不变，proof verified却overall unverified/missing_terminal_verify_verdict。父子各4原件/1同代final→report链接和hash匹配，不冒context-pack耐久。详[逐层人工审计](parallel_selected_summary_hmc_query_scope_io_registration_20261103_manual_audit.md)。其它二进制/代表/图本批未运行，不回签旧绿；旧IO/采样/资源充分事实后的误述保原，避免无限追同题。

## 全部51开放项ROI与下一步

| ROI | 数量 | HMC稳定ID（省前缀） | 顺序 |
|---|---:|---|---|
| P0共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 先修已证身份/完整提示预算和引用回归，再统一成文复用 |
| P1现有材料 | 6 | 08.3、18.5、11.2、03.2、14.1、18.2 | 轮换原有限代表/二进制/正负登记；不猜统计协议或重复追绿 |
| P2复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 基于真实材料完成端到端验收 |
| P3协议/来源 | 21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 先证明协议/来源/权限，WAL不绕零副作用 |
| P4新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 依赖/真实材料到位后交付 |

[核账快照](results/hmc_query_scope_io_registration_20261103_v1/sealed-v1/0253-codrax-hmc340-census-roi-v1.json)完整覆盖所有51ID无重漏，原退出合同见统一任务表。剩余3组分别挂原ID：共享预算/引用回归、原有限解释失败、自然多查询/权限/代表/图及剩余格式协议。下一代优先验证紧凑完整身份与完整来源凭证分离，不靠删事实、合并query或阈值拟合；Trace投影/自动补齐、原4根因/闭合睡眠/链上业务、合法计算/重试不变，WAL生产NO-GO。

## 原件封存与失败发布边界

ROOT hmc_query_scope_io_registration_20261103_v1，419件index SHA59bb4a9b322fcf9e341e5a47ce8065c5a5b89ffc2405191546843b486703802f，seal receipt SHA27f6ad02258ad8b63da697dbfa796ef4c8c1b58bba2916ff8da93277221ad25a；[root无闭项决定](results/hmc_query_scope_io_registration_20261103_v1/sealed-v1/0190-codrax-hmc340-acceptance-v1.json)。旧41代17268各ROOT保原，42代按代17687非全局去重；本代物理重核最新339624与更旧链头，不冒全历史payload重hash，失败336/340不补成功publication。

候选源码未交付；本次只推送五文档，候选仍在main待下一代修复验证，不迁其它分支。本地忽略原件不冒已上传；providers（含synthetic）不读/哈希/复制，大原件仅完整外部引用。失败文档推送收据与成功源码publication严格分开。
