# HMC330 人工审计摘要

固定两例各一次；CLI均0，A原smoke PASS、B原operator FAIL；**两例完整人工均FAIL**。原代表规格也未通过，root明确closure=none，79唯一＝27已交付＋52开放。全仓98包28679顶层34146子例PASS是独立实现验证，不覆盖人工结论。

- [A完整审计](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-v1/0235-codrax-hmc330-A-audit-v1.md)：完整235MB真实100版抖音原件，9个成功query回执/4唯一载荷；主要账户与量尺边界到场。HTML4表中16行阶段的64量测格、6行状态及2783行原生时序保真，有限PASS。正文仍把小Runnable扩大为排除算力供给、未展示全打点却宣称全部展示，并有0.634/0.6及self概括矛盾；整答FAIL。受理解释错误不等于全被投递，不虚构预算丢核心账户。16µs同刻wake差异仅待验证边界，不定bug或冻后修复。
- [B完整审计](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-v1/0257-codrax-hmc330-B-audit-v1.md)：实际event_search两组8成员/4选/4未选、同代不可变index/selection/SHA及4合法资源补查有限PASS，14帧与原生分组保真。但原operator要求resource_stack声明，本次规格偏移且真实exit1，原有限合同FAIL。正文混含端点原总体与半开投影、把空来源也说成有入口、混声明与代表数量，整答FAIL。错误“统一窗口”确实进入INIT，其它被拒/省略的句子不冒已证污染。

A报告SHA `8ea9c44560fbcb055db16326217b7a310806bccf494da57f182cfda1d5188702`；B报告SHA `4f0d1f574465718bd0b509fa36e959e34f1d53f72936f95014d8a88ab976eeba`。每例3份INIT是同次初始化的3条消息，不是三轮。明确配对A6/B16，A另6歧义群组排除，最大可证请求重叠12.533秒；不声称全部配对无歧义。

全部MD/HTML表和附加表已读审，仅源内容QA，无像素QA；图/patch/根因合同未触发不新增失败或PASS。invalidUTF8原日志、失败operator、被拒JSON/引用和未投递解释均保原件。504件封存、完整测试、参考取舍、ROI与main交付见[本批汇总](parallel_selected_summary_hmc_douyin_representative_scoped_delivery_20261031.md)。
