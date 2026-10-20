# HMC267 两例独立人工审计

结论：机器2/2，完整答案人工 **0/2**。入口/原生事实的限定PASS与整答FAIL分别保存；79唯一、24已交付、55开放不变。两个独立审计员核原始工具返回、真实Finalizer上下文、MD/HTML及primary/principal答案面，主控复读完整报告。HTML仅核main内容，无浏览器像素或图形验收。

## A：普通命名gzip采样

普通自然路径、无预附：3,141,140B gzip完整解码到30,062,585B真实PERFILE2，原件和只读运输包装身份/字节未变。原件SHA`da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`，gzip SHA`8b81184667832f2e8e46373de75313d4cd8792a789a5727aa823b80479e99591`。实际原生fallback reader成功/degraded，官方simpleperf不可用且未尝试；不是调度trace，不扩签其它格式。

独立参考parser核12000总记录/1393窗内/164目标、权重259；CPU0/1各82记录、权重85/174。CPU记录时间包络分别为`[486489464088122,486489465612497]`与`[486489588095622,486489589609372]`ns。1次模型window_stats＋1次真实系统补齐同源同窗/线程，unique payload为1。完整采样树为1cohort/2节点，各depth0单IP；82refs仅预览4、其余78明确省略，不能当完整展开栈。

限定PASS：入口、完整准备、真实查询、显式窗补齐、原始数值/CPU范围/unknown与引用预览边界；note目录共享命中。完整FAIL：CPU编号被称物理核（无拓扑），符号化IP被称函数入口，period权重被称实际消耗CPU周期。尾部声明权重不是耗时不能抵消这些肯定说法。合法符号业务线索仍保留；限定记录树的depth0叶节点不是另一个FAIL。旧全部CPU0、比例直接变耗时等本次未发生，不乱记。

最终前缀`.codrax/output/20261019-183326.003-40560`：MD SHA`aaa665bbd2b135deb931fe857af06622b4d13a27c581313652b86ee47f082637`；HTML`c7d3f0bcfd2a752b22705063a66f7dc420f4bae8ce0b08c00d98b9191247d879`；answer-surfaces`c1f1ecfa7e3aa5635091db09f686a159b991d45e4eea943e2ec964985c5a76d8`。4最终件、14blob、3准备件、1read-run共22原件；独立报告`codrax-hmc267-sample-manual-audit-v1.md` SHA`b3fcb4646925a202ae616f2270ba8c4c32eaeae8d7c669a3ec370a8ac990ed03`。

## B：双侧量测/采集元数据

两个已有SQLite测试原件，不冒称设备实采。基线SHA`c39d68bb34765770ef535c44b233a03a29b43e73d189eaae7b69f994d184d56a`，当前SHA`2a0f696afff89220b8d5fb8829f70753a1c581858aca9ecd88995af44e808b2f`；capture_id是记录标识，不是原件字节SHA。

只读SQL oracle逐行核13/8选中行、7/4组，各1条无时间原件不混入窗口。`334200000.5`/`250000000.25`、大整数`9007199254740993`/`9007199254740995`、NULL/TEXT/BLOB与原始单位均保真；未知duration不延长、重叠不合并、右端排除。设备元数据中当前height缺失、frame_rate为REAL60并带异常/重复修订边界，不能当正常60Hz。

4尝试＝3成功返回＋1schema拒绝；首组合实读两个来源，后两分侧为pure_memo，故4个逐侧成功收据仅2次实际源读取。两个finalizer拒绝均保留；早期草稿跨侧错配已修，不列最终FAIL。

限定PASS：实际原生查询、逐侧身份/窗口、21原始行/11分组与8张最终表格。完整FAIL：异常帧率推正常Hz、标签推物理设备、gpu_state记录计数推状态翻转、名称推物理频率/负载、部分精确值无近似提示地舍入。原始完整事实与边界在Finalizer已到场，Explorer摘要已含越权单位；无隔离实验，不能证明唯一prompt根因或纯模型波动。

最终前缀`.codrax/output/20261019-183338.645-40559`：MD SHA`10af1bd410ac9c40f3b8d169548d71311e57cd24e5df6dc30f4b5cf24638890e`；HTML`2d7120c5c8de488eaf5a28ee178d52d51930cb9fcf8cca92ebfd90be933f7192`；answer-surfaces`02de3ad107f7a3f3927f767d216084239a5485c7b8d77a62590d0e5ba87ad822`。4最终件、21blob、6准备件、1read-run共32原件；独立报告`codrax-hmc267-dual-manual-audit-v1.md` SHA`b58cbb750deef2db4ca05fc2b08f07635f494a57a001c6df143e98184227235b`。

## 未覆盖与留账

两题无图，图时序/关系修复NOT_TRIGGERED；root-causes sidecar未激活，不等于已证明没有根因。A未自然触发无窗入口、选择错误重试、CRC/取消/预算坏源、不同等价selector或预算溢出；B未触发新增采样展示去重/共享池，原生量测memo不等于新能力命中。公开正反/race仅作为确定性保护证据，不能补签自然人工PASS。

保留全部失败草稿、原始运行日志、工具blob、准备产物及当前最终原件，不追第三例。A旧采样解释FAIL继续16.4/18.3，B同源事实成文/单位身份权限继续01.3/16.4/18.3，均含于55开放。完整报告和54份额外原件映射随[批汇总](parallel_selected_summary_hmc_gzip_native_reuse_20261019.md)的封存索引保存，不靠临时目录长期存在。
