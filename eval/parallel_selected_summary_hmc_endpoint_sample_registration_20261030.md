# HMC322：端点删减兼容、共享算术及双例轮换验收

持续main，基线 `d14d2c24fb343e685c282f3e70da982e14c431cc`；最终实现 `ab5834066d310208474e16e0c913f9b9c491a680`，与7份文档紧邻分笔提交。继承HMC321的11项未提交改动并修正其回归，**不把321失败冻结作为已通过版本**。后置远端、提交树、清洁与封存一致性见[publication](results/hmc_endpoint_sample_registration_20261030_v1/publication-v1.json)。

79唯一稳定任务＝27已交付＋52开放，18类；18部分/29待实施/3待验收/1验收中/1持续。**本批完整能力新增0；已交付有界共享子能力2组；本批完整人工A失败、B限定负控通过。** closure=none，既有有限交付不重新计数，不以模型误述无限阻塞其它高ROI修复。

## 两组共享修复

1. 原生业务区间、IO在途、线程/频率时序的真实查询端点传到共享来源标题。私有不可变凭证区分可选Notes删除与事实篡改：逐表保序子序列、重复次数不增加，空/非前缀删除合法；新增/替换/倒序/跨表转移、源/查询/窗/单位/行列改变仍失效。最终当前表重新获得自己的显示凭证，不能借旧表seal；旧raw严校验及JSON/legacy未知边界保留。native finalize只做原允许来源装饰，不由消费者重签事实。
2. 三处系统教学与原生selector统一有界算术：可证同源同量尺分区/互斥self、同状态交集和嵌套差允许；绑定操作数、单位、主体、窗口和集合，未知不补零。不能相加重叠inclusive、跨线程墙钟、RQ/BIO重叠层或根因rank席位；计算不证明物理对应、执行状态、完成或因果。没有新增问答关键词硬门。

原ShortProviderNotes/HMC321/HMC290及原4候选/闭合31ms/请求35ms/合法2ms/47背景断言保留。HMC283仅更新已有原生半开证据的过时unknown文字pin，原数值/分母/成员全保；原绝对禁止算术教学pin随有意合同演进更新，不放宽真实事实断言。

参考 `io_ops.py:507–625`/`io_latency.yaml:205–291` 的桶定位、合法分区COUNT/SUM和summary_only，保本项目精确配对积分，不把桶计数叫硬件并发；参考 `marker_ops.py:465–545` 的实例/父子/self，保直接子并集和未知。说明省略不改变量测身份，但不照搬省略空桶后的分母。详见[参考发现](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0364-codrax-hmc322-reference-findings-v1.md)与[静态独审](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0317-codrax-hmc322-independent-review-v1.md)。

## 公开验证、冻结与唯一全仓

| 最终定向 | 顶层PASS | 子例PASS |
| --- | ---: | ---: |
| 端点新旧公开合同 | 6 | 29 |
| 端点/raw/来源相邻保护 | 23 | 126 |
| 端点race | 8 | 44 |
| 算术/实际finalizer/相邻教学 | 20 | 10 |
| 因果投影/自动补齐保护 | 64 | 293 |

均无跳过。真正RED、首green中新测试空slice与旧Clone规范化nil的期望问题及诊断失败全留原件；只调整新夹具表示，不改生产Clone或旧断言。

CLI/testbin各编译一次，同源码清单SHA `f0aa8e12746620597e0ae608feff016b40c9db4d0b7f3cfdb1273f2c72516185`；CLI SHA `e8d464c800ff00e0819c78d2abd9cda24a9512c5af520812c4463866ac5f6c18`，testbin SHA `c3b9302eaebf214b24f87ab1e6305461445548e3b9e1303d66cb330fd191376d`。冻结7531仓库/234外部/27仓外快照，SHA `7617104d50fa1ef7c4e24c6e63745b35a0f0dcc4f0d3fc3ced2012293c83eaa1`，随后源/测试/注释/helper/输入停写，后置verify通过。

唯一full `go test -json ./... -count=1 -timeout=30m`，PID7242，10:30:24.881651Z→10:41:48.738448Z，实际exit0：98包PASS/12无测试、28599顶层/34015子例PASS，原43/14 SKIP，零FAIL/非JSON/重复或未终态。[统计](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0312-codrax-hmc322-full-stats-v1.json)，原日志SHA `7503c97606296c5a5efe397c4a8eca31f91a754d4b2da5fd757bd0516f39421c`。

## 恰好两例并行、各一次

| 侧 | 实际范围 | 进程/原机器 | 人工 |
| --- | --- | --- | --- |
| A 原 `trace_binary_sample_only` | 完整PERFILE2，15步/n1；真实CLI | exit0 / PASS | FAIL：unknown类型升执行状态 |
| B 原HMC287缺来源终态负控 | 真实planner＋脚本恢复controller＋独立进程public RunTests | exit0 / PASS | 限定PASS；整体仍unverified |

实际请求A9/B2完整配对、零歧义，最大可证重叠 **0.074秒**；不是用进程重叠冒模型并行。B不冒自然controller/fullCLI，子进程属于同一问题恢复，不是第三例。[人工细审](parallel_selected_summary_hmc_endpoint_sample_registration_20261030_manual_audit.md)。

A完整原件30,062,585B/SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`；历史独立参考parser标量oracle按原封存绑定，本次不重跑。9份成功payload＝7次显式成功＋2系统补采；2次显式查询和1次shell拒绝原件保留。tid1609/tgid1514，164条/259权重；CPU0回调82/85、CPU1内核82/174；完整包络与独立82匹配/20预览分开。3份INIT已有unknown/执行状态未确立，正文仍肯定在CPU运行，末尾caveat不能抵消。未发现完整错误调查reason重传或裁剪丢关键事实；122725峰值和一次裁剪仅关注项，不新立失败门。

B实际两轮planner完整读源码/测试后一次空改动登记；父7246/子7364。本轮1断言通过/1native invocation/2命令，与历史分开；source无verify/Completion、proof verified、整体unverified/missing_terminal_verify_verdict。源码/测试/HEAD不变，授权撤销；父子真正.final.json同字节并指向同代内容寻址report，不是兼容alias。context-pack耐久、Markdown及自然controller未覆盖。

## 剩余范围与ROI

当前重点9组未完问题（非全局穷举、不增加稳定ID）：完整/窄窗业务账户；等待总体/预览范围；D/IO未知分类；同源重复和跨块预算；采样状态解释；IO物理/驻留/集合解释；业务等待/caller/比例解释；失败JSON修补终态；代表选择自然规格。321原算术解释失败不回签，系统教学已修，相关自然复验另排。新Notes删除自然复发布和模型图/局部修补未触发，旧权限/Windowed/扩大race/登记反馈欠项继续保留。

优先修真实Donghu已证的共享账户交接：原完整self3.938存在，却被16条前缀及128行展示预算挤出，不是ID碰撞。按精确源/查询/实例/窗整体交接及公平预算，不粗提上限、不松完整性门、不复制根因。完整能力轨轮换11.2和有新增真实材料的17.6；100版Donghu例仍NOT_RUN，105版已实际使用。完整[52项ROI分层](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0218-codrax-hmc322-census-roi-v1.md)与[根裁定](results/hmc_endpoint_sample_registration_20261030_v1/sealed-v1/0392-codrax-hmc322-root-acceptance-v1.md)保留。

426件[封存索引](results/hmc_endpoint_sample_registration_20261030_v1/sealed-index-v1.json)，SHA `25e9d3323465d04695e4ff61e33c211f2205d7c71445d52eaf4ea7daea00670a`；旧23档7703件/493来源快照身份逐字节保真，18历史污染是其子集。本批27仓外源码快照不回流Go树。日志非法UTF8原字节、失败、完整INIT/参数/payload/成文均留存；原件与同字节全文仅引用，派生材料不按大小删。HTML仅源审，不冒像素QA；凭据不读取/哈希/复制，raw本地归档不冒Git上传。WAL零副作用前生产NO-GO。
