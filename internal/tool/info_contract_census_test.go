package tool

// info_contract_census_test.go — UXG-1 T1 + T2-armB 三向 tripwire census
// (ledger docs/design/real_trace_campaign_20260705.md §29.40; mechanization
// spec scratchpad/info_contract_report.md §④). Kills 病理形A (fields minted
// but never read) and half of 形B (wire fields with no Node mirror and no
// consumed note), and makes the §29.40 exemption adjudication a TYPED
// commitment surface (豁免表=承诺面).
//
// CONTRACT TABLES: every exported field of the projection wire types carries
// exactly one disposition. Statuses and their MECHANICAL arms:
//
//   displayed      — consumed by a display authority source; arm: the field
//                    token (".Field", word-bounded; Token overrides for
//                    method-mediated consumption) MUST appear in the display
//                    authority sources (runtime_tree/_rcr/runtime/_typelabels/
//                    _supplyfold/_rcm, comments stripped).
//   internal_gate  — consumed as a typed gate input only; same presence arm.
//   exempt         — adjudicated word-face exemption; Ref names a W-# row of
//                    the exemption registry (no scan arm — the ruling, not
//                    the code, is the authority).
//   known_gap      — "typed 有据但显示无踪" (the 13 OM findings); arm: the
//                    token must NOT appear in the display authority sources —
//                    when a fix batch wires the face, this census REDDENS
//                    until the row flips to displayed (悄悄修而不销账 and
//                    销账而未修 both bite red).
//
// RankItem (T2 armB) statuses:
//
//   node_mirror    — the wire field has a projection mirror; arm: the mapped
//                    field (same name, Ms→MS-normalized, or the explicit
//                    mapping below) exists on TraceCausalProjectionNode /
//                    TraceCausalProjection (reflection).
//   note_consumed  — the value travels via a registered rich note with a
//                    parsing consumer; arm: the named key is registered with
//                    carrier != display_only.
//   note_displayed — the value travels via a registered display-only rich
//                    note; arm: the named key is registered with carrier ==
//                    display_only.
//   engine_gate    — consumed by tracequery engine logic pre-wire; arm: the
//                    token appears in ../tracequery non-test sources.
//   exempt / known_gap — as above; known_gap additionally arms the reverse
//                    direction: NO same/normalized-name projection field may
//                    exist (a fix batch adding the mirror must flip the row).
//
// Failure direction is conservative (报红必真,漏报可能): token collisions
// across types can only satisfy a presence arm (false green), never redden a
// correct row; collision-prone rows are marked NoScan with the collision
// documented.
//
// 账实一致纪律 (§29.40): every fix batch (v5 P2c / IC-A / IC-L / IC-E / IC-S)
// flips its contract rows known_gap→displayed IN THE SAME COMMIT.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"reflect"
)

type fieldDisposition struct {
	Status string // displayed | internal_gate | exempt | known_gap | node_mirror | note_consumed | note_displayed | engine_gate
	Ref    string // face/gate/W-#/OM-# pointer
	Token  string // optional scan-token override (method-mediated consumption)
	NoScan bool   // documented name collision — presence/absence arms skipped
}

// --- §29.40 exemption registry (豁免表 = 承诺面) -------------------------------
//
// 确凿 7 (W-1/6/8/11/15/17/20) + 维持 11 (W-2/3/4/5/7/9/10/13/18/19/21) +
// 收窄 3 已折 IC-A (W-12/14/16). Every row carries its reason and escalation
// condition; ghost rows (unreferenced W-#) redden.
type infoContractExemption struct {
	Reason     string
	Escalation string
}

var infoContractExemptions = map[string]infoContractExemption{
	"W-1":  {"S1 修根:排序合成分数禁以 ms 事实身份发布 + §29.22 乘子泄漏修根(RankSortBoostedEffectiveMs json:\"-\" 内部排序道)", "无(裁定终局)"},
	"W-2":  {"§29.36.2 裁定:背景通道无序数、#N chip 退役;提及义务=LLM 面 background_rank= note 软引导,Node 字段无显示消费合规(假注释已随 UXG-1 勘正)", "无"},
	"W-3":  {"§28.11-3(a):同值双归因披露只落证据索引 audit 面为设计决定", "无(维持现状)"},
	"W-4":  {"§29.30/.1+§29.37 EPUB:typed 内部门信号,唯一消费=lead 拒冕臂;词面无痕是设计", "effective-published 记号批若给词面,随批翻 displayed"},
	"W-5":  {"三态内部判定;false 经 ⚠实际/⚠跨窗 间接现形;nil-vs-true 不区分=absence never guesses 同法理", "无(true 正向词倾向否,防噪)"},
	"W-6":  {"§29.18 DISP-3 假⚠ donor carve-out:纯判官字段,值不落面是设计", "无(裁定终局)"},
	"W-7":  {"depth 挂靠硬门输入;被拒行有词面结果(链上─/父节点未确认/深度未解析);拒绝理由本身无词面", "回访/冷读再现 B2/B4 类拒因追问 → 升 IC-A(明细一行披露拒因)"},
	"W-8":  {"折叠强制展开选择器;效果结构性可见(行不折叠),无需词面", "无(裁定终局)"},
	"W-9":  {"值不印为 U10 裁定方向;语义经口径词代言,ideal+deficit 在拆解行可见", "回访出现频点覆盖比例数值需求 → 升 IC-A"},
	"W-10": {"Σ==窗硬门自算使显示冗余;字段作 wire 对账载体保留", "随 OM-5(IC-E 批)一并评估是否入 audit 面"},
	"W-11": {"数值语义全走 ImpactMS 族口径车道;三要素行文法取代源观测散文(系统不代写/proof-lane 同源);Summary 是 LLM 面载体", "无(裁定终局)"},
	"W-12": {"端点经 actual_window note 有显示消费(runtime.go 窗标)", "收窄已折 IC-A 顺手项(§29.40):⚠实际 行明细补实际段端点,IC-A 合入时翻 displayed"},
	"W-13": {"coverage 视图+ledger notes 消费;投影面以 MergedQueryWindows 代位", "非合并聚合行逐次发生窗出现独立需求再议"},
	"W-14": {"内部管线 token,入用户面有 no-internal-info 张力", "收窄已折 IC-A 顺手项(§29.40):audit 面补 source token,IC-A 合入时翻 displayed"},
	"W-15": {"合并行上清空,真实区分键经 FamilyMemberRoster 词面到达", "无(裁定终局)"},
	"W-16": {"吸收披露已有 链上并入+身份串+E# 名单;absorber 侧计数缺口小", "收窄已折 IC-A 顺手项(§29.40):链上并入行加共N计数,IC-A 合入时翻 displayed"},
	"W-17": {"经 selected_window note→Node.QueryWindow*,已有窗 chip/窗标词面;非缺口", "无"},
	"W-18": {"perf 车道行是否进入投影树未证实(投影编译零读);LLM 面承载", "先审 perf 行可达投影与否(可达性探针),再议词面"},
	"W-19": {"SupportRefs file:line 与 ArtifactLabel 疑似覆盖物理身份;防并族用途是引擎门", "多物理文件 bundle 内单树混行辨识一次实证后再议"},
	"W-20": {"priority_inversion_gated 理想源 note:显示不凭空合成(no_system_backfill);分量和回退已 pin", "接线留 P0-E,账本宿主 campaign:1272"},
	"W-22": {"R5 §29.88.12 单基准:折算基准=全域最大核最高频点,词面单形不再随基准簇类分叉(按X核满频 demotion 词族退役);字段保留为基准簇类 wire/审计记录", "客户回访出现基准簇类词面需求(如 prime 基准点名)→ 升 IC-A"},
	"W-23": {"XLANE-2 件3:AbsorbedWholeSeatDemotedView=compile 侧(types aggregate)×N fold-key 纯判官(anchorFormKey 第五臂),链上面吸收降道视图的账目身份记忆;词面零读是设计(降道句禁上链上面=三面矛盾修根),消费点在 types 包故不进显示权威扫描集", "无(裁定终局;若未来显示面需要「已吸收降道视图」披露词,经用户裁定升 displayed)"},
	"W-21": {"rcr.go 形态表 SemanticsZH 空的 5 glyph spec:行2 类别词有词面,图例语义骑既有状态图标条目(两列单源表设计)", "P8 图例承诺面尺子下如需独立图例条,经用户裁定再开"},
}

// Non-field exemption attachment sites (surface-level rulings referenced by
// name so the ghost check spans them too).
var infoContractNonFieldExemptionSites = map[string]string{
	"W-20": "note ledger: priority_inversion_gated (internal/types info_contract_notekeys census)",
	"W-21": "legend surface: the five §24.3 form specs without GeneratedLegend semantics (T3 promise census)",
}

// --- §29.40 known-gap registry (13 OM,修复批翻状态用) -------------------------
type infoContractKnownGap struct {
	Summary   string
	HostBatch string
}

var infoContractKnownGaps = map[string]infoContractKnownGap{
	"OM-1":  {"周期源行迟到量分量不落任何面(承诺-数据脱节:图例/阅读参考/明细括注点名分解)", "①明细(a)格随 v5 P2c;②行3 完整修随 IC-A"},
	"OM-2":  {"锁行排队等待者数 BlockingWaiters 零消费(N 定修理方向)", "IC-L(可提前搭 v5 P2c)"},
	"OM-3":  {"下钻边 E#/来历语义零消费(行动指令回证据链断开)", "IC-E(边观测入册通道)"},
	"OM-4":  {"span 三层身份词(kind/category/subcategory)零消费", "v5 P2c"},
	"OM-5":  {"四态账 EvidenceID 铸而不读(发布值不能回指自身证据,违 §29.29 审计闭环)", "IC-E(v5 F10 条款增补)"},
	"OM-6":  {"fold peer TypeWord 写后不读(aabccb6f 删唯一读者;§29.40 裁决=A 臂保留字段)", "v5 P2c(「链上并入」行词面臂)"},
	"OM-7":  {"DrillStatus 投影头部强制披露半场七面全零(RCX① 裁定明文未兑现)", "IC-A"},
	"OM-8":  {"PriorityInversionLockDominated 并存披露注记缺失(§29.40.1:并存事实披露,非降级替换)", "IC-A"},
	"OM-9":  {"◇ 邻近行判据四元(overlap/edge_count/nearest_chain_*)发布即坠零解析", "IC-A(campaign:1397 立账族宿主)"},
	"OM-10": {"跨 ns 锁持有者身份两键(ns 统一/宿主进程)未显(假注释已随 UXG-1 勘正)", "IC-L"},
	"OM-11": {"锁对端状态族 peer_state_* 八键 + wait_object 未显(advisory 反超确定性面);peer_chain_* 伴生", "IC-L"},
	"OM-12": {"非焦点线程四态拆分不可见(dominant 单词掩盖第二可行动轴)", "IC-S(§29.23 成员级论证先行)"},
	"OM-13": {"InheritedTargetBlockedMs 承接注记未接线(注记面从未接线)", "IC-A"},
}

// --- T1 · TraceCausalProjectionNode contract (B 区) ---------------------------

var nodeFieldContract = map[string]fieldDisposition{
	"Role":                          {Status: "displayed", Ref: "行1 glyph/段位 + 明细层级"},
	"EvidenceID":                    {Status: "displayed", Ref: "[E#] 行尾 + 证据索引"},
	"Subject":                       {Status: "displayed", Ref: "行1 名字场 + 明细 + 证据索引"},
	"Predicate":                     {Status: "displayed", Ref: "证据索引 audit token"},
	"Object":                        {Status: "displayed", Ref: "行1 词位输入(DisplayCauseName) + 明细影响点"},
	"Value":                         {Status: "exempt", Ref: "W-11"},
	"Unit":                          {Status: "exempt", Ref: "W-11"},
	"Summary":                       {Status: "exempt", Ref: "W-11(LLM 面载体)"},
	"SupportRefs":                   {Status: "displayed", Ref: "证据索引 locator"},
	"LineStart":                     {Status: "displayed", Ref: "证据索引 locator 行区间"},
	"LineEnd":                       {Status: "displayed", Ref: "证据索引 locator 行区间"},
	"Rank":                          {Status: "displayed", Ref: "行1 ➊..➎ + 行2 chip(有效持席单门)"},
	"Tier":                          {Status: "displayed", Ref: "明细席位行 + audit token(三 typed 谓词门)"},
	"Causality":                     {Status: "displayed", Ref: "行1 段位 ⛓vs⧗ + audit"},
	"ChainRelevance":                {Status: "displayed", Ref: "行2 链上L# + 通道词权威"},
	"ChainDepth":                    {Status: "displayed", Ref: "行2 链上L# chip + 明细因果位置"},
	"TraceGapKind":                  {Status: "displayed", Ref: "行1 判据二分词(◇盲区行)"},
	"OnChainBasis":                  {Status: "displayed", Ref: "行2 目标自身·确定性优化 限定词(SELF-SEM §29.61.1 单字段谓词)"},
	"ChainBranch":                   {Status: "internal_gate", Ref: "W-7 depth 挂靠门(tree.go 树位判定)"},
	"ImpactMS":                      {Status: "displayed", Ref: "行1 值+bar+% + 明细(a)"},
	"CumulativeImpactMS":            {Status: "displayed", Ref: "行2 链上累计 + 明细(a)"},
	"SpanName":                      {Status: "displayed", Ref: "行1 SpanName 词位 + 明细 span 原文 + audit"},
	"SpanKind":                      {Status: "known_gap", Ref: "OM-4"},
	"SpanCategory":                  {Status: "known_gap", Ref: "OM-4"},
	"SpanSubcategory":               {Status: "known_gap", Ref: "OM-4"},
	"SemanticClass":                 {Status: "displayed", Ref: "行1 类词 + 行3 双口径(语义参赛门)"},
	"StartTs":                       {Status: "displayed", Ref: "行2 窗标 + 明细窗来源"},
	"EndTs":                         {Status: "displayed", Ref: "行2 窗标 + 明细窗来源"},
	"QueryWindowStartTs":            {Status: "displayed", Ref: "行2 来自查询窗 + 明细窗基"},
	"QueryWindowEndTs":              {Status: "displayed", Ref: "行2 来自查询窗 + 明细窗基"},
	"WithinRequestedWindow":         {Status: "internal_gate", Ref: "W-5 三处判定,false→⚠间接"},
	"Confidence":                    {Status: "displayed", Ref: "行2 置信 tier + 明细(a)"},
	"StateKind":                     {Status: "displayed", Ref: "行1 glyph+状态词 + 行2 [状态state]"},
	"DStateRefinedNonIO":            {Status: "displayed", Ref: "件③ arm a: 细化「D-state」词面门(名/形态格/对端措辞族)"},
	"DStateCauseUnprovenRemainder":  {Status: "displayed", Ref: "§29.50.5 件②: 「(原因未证)」余数词面门(cause 词族 qualifier)"},
	"ChainAnchoredMS":               {Status: "displayed", Ref: "RSPA §29.61.10 行2 全窗=锚定+其余 分解 + WO-C1 同源二分句"},
	"ChainAnchorFullMS":             {Status: "displayed", Ref: "RSPA §29.61.10 行2 分解全窗值 + 同源二分句合计"},
	"ChainAnchorRemainderSeat":      {Status: "displayed", Ref: "RSPA ◇ 余段席词面门(调度压力候选/余段词族)"},
	"ChainAnchorOwnershipDivergent": {Status: "displayed", Ref: "RNB-1 §29.88 R2 行2 账目关系(锚定权属失合) 句门 + smr1 配对臂禁入"},
	"ChainAnchorChainLaneMS":        {Status: "displayed", Ref: "RNB-1 行2 双Σ披露(链席自账Σ槽)"},
	"ChainAnchorCensusMS":           {Status: "displayed", Ref: "RNB-1 行2 双Σ披露(锚定账Σ槽)"},
	"ChainCredentialLaneDemoted":    {Status: "displayed", Ref: "RNB-1 §29.88 R4 行2 无链上凭证(整席不入链上榜) 披露行"},
	// HULL-CRED (§29.104 终判③, 2026-07-17): keep-⛓ 逐段凭证三元组.
	"ChainCredentialSegments":        {Status: "internal_gate", Ref: "HULL-CRED §29.104 终判③ 逐段核验词 claim-gated-on-proof 门(清单本体不上句面,tree.go 词面 fork 判定)"},
	"ChainCredentialSegmentDisjoint": {Status: "displayed", Ref: "HULL-CRED §29.104 终判③ 行2 无链上凭证(逐段核验,整席不入链上榜) 披露行(需清单同行)"},
	"ChainCredentialEnvelopeLevel":   {Status: "displayed", Ref: "HULL-CRED §29.104 终判③ 行2 (包络级凭证) 诚实注"},
	// ONCHAIN-FIX-2 件3 (Q6 已追认, 2026-07-18): 行2 (凭证清单不完整,实际锚定不小于所证) 诚实注.
	"ChainCredentialSegmentsTruncated": {Status: "displayed", Ref: "ONCHAIN-FIX-2 件3 行2 (凭证清单不完整,实际锚定不小于所证) 诚实注(需清单同行+链上道)"},
	// ONCHAIN-FIX-1 件1 (2026-07-18): 行2 成员继承(链窗级,无区间凭证) 披露行.
	"ChainIdentityInheritance": {Status: "displayed", Ref: "ONCHAIN-FIX-1 件1 行2 成员继承(链窗级,无区间凭证) 披露行(链上道+无更强凭证词时)"},
	// XLANE-1 件1 (§29.104.2, 2026-07-15): 行2 锚定份由链席代表(整席不入链上榜) 披露行.
	"ChainAnchorRepresentedByChainSeat": {Status: "displayed", Ref: "XLANE-1 §29.104.2 行2 锚定份由链席代表(整席不入链上榜) 披露行"},
	// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): A/B 分账披露族.
	"GatedShareClaimedMS":           {Status: "displayed", Ref: "LEVELMERGE-1 件2 行2 分账披露(已计入反转席份额槽)"},
	"GatedShareFullMS":              {Status: "displayed", Ref: "LEVELMERGE-1 件2 行2 分账披露(修前全账槽,claimed+residual==full)"},
	"GatedShareConstituentSeat":     {Status: "displayed", Ref: "LEVELMERGE-1 件2 A 构成行降道词面门(归因已由反转席计入)"},
	"GatedShareClaimSeats":          {Status: "displayed", Ref: "LEVELMERGE-1 件2 [E#] 反转席互指指针(all-or-nothing 解析)"},
	"GatedShareOverlapDisclosureMS": {Status: "displayed", Ref: "LEVELMERGE-1 件2 裁定④ 其中X ms与[E#](反转席)重叠 fail-open 披露句"},
	// PARTSPLIT-1 (§29.150④): the R4-mirror refusal record quartet — the 行2
	// 分账 sub-line (runtimeTraceProjGatedCompositeEdgeShareTagText) + the ◎
	// non-seat mention's [E#] resolution read all four.
	"GatedCompositeEdgePreShareMS":  {Status: "displayed", Ref: "PARTSPLIT-1 行2 边前份披露(R4拒转·整席不拆)分账 sub-line"},
	"GatedCompositeEdgePostShareMS": {Status: "displayed", Ref: "PARTSPLIT-1 行2 分账 sub-line 边后份槽"},
	"GatedCompositeEdgeAnchorTS":    {Status: "displayed", Ref: "PARTSPLIT-1 行2 分账 sub-line 最晚凭证边槽 + ◎ mention [E#] typed anchor match"},
	"GatedCompositeEdgeAnchorVia":   {Status: "displayed", Ref: "PARTSPLIT-1 行2 分账 sub-line 凭证词槽(runtimeTraceProjHostEdgeViaWordZH)"},
	// R3-IMPL (§29.88.1, 2026-07-15): 行2 唤醒锚定(宿主→目标) 披露句.
	"HostWakeupEdgeAnchorTS":  {Status: "displayed", Ref: "R3-IMPL §29.88.1 行2 唤醒锚定(宿主→目标) 句(边界 ts 槽)"},
	"HostWakeupEdgeAnchorVia": {Status: "displayed", Ref: "R3-IMPL §29.88.1 行2 唤醒锚定(宿主→目标) 句(凭证来源槽)"},
	// RNB-2 件5 AFF-EVID (§29.88.6, 2026-07-15): 行3 CPU约束描述行.
	"CPUConstraintKind":         {Status: "displayed", Ref: "RNB-2 件5 行3 CPU约束描述(判定依据槽)"},
	"CPUConstraintCPUSet":       {Status: "displayed", Ref: "RNB-2 件5 行3 CPU约束描述(cpuset组槽)"},
	"CPUConstraintPolicy":       {Status: "displayed", Ref: "RNB-2 件5 行3 CPU约束描述(restricted 词面门)"},
	"CPUConstraintAllowedCPUs":  {Status: "displayed", Ref: "RNB-2 件5 行3 CPU约束描述(允许核集)"},
	"CPUConstraintExcludedCPUs": {Status: "displayed", Ref: "RNB-2 件5 行3 CPU约束描述(全域对照排除集;R5a 预留)"},
	// R5a (§29.88.4 场景② 按核档, RNB-4 2026-07-15): the obligatory
	// 绑核排除更大核档 mention's proof pair on the 行3 description line.
	"CPUConstraintAllowedMaxTierKHz": {Status: "displayed", Ref: "R5a 行3 绑核排除更大核档(允许核最高档)"},
	"CPUConstraintGlobalMaxTierKHz":  {Status: "displayed", Ref: "R5a 行3 绑核排除更大核档(全域最大核档)"},
	"ResourceCompletionClosure":      {Status: "displayed", Ref: "RSPA M-IO 行2 完成闭合注记"},
	"SystemSupplement":               {Status: "displayed", Ref: "SUPP-CORE 修复轮 件5: E# 审计面 origin=system_supplement 出处 token"},
	"BlockedReasonCaller":            {Status: "displayed", Ref: "件③ 行2 等待对象 披露"},
	"BlockedReasonWindowCount":       {Status: "displayed", Ref: "CR-3 件② 行2/headline 未核销 blocked_reason 残余披露"},
	"BlockedReasonWindowCaller":      {Status: "displayed", Ref: "CR-3 件② 行2/headline 未核销 caller 符号"},
	"ProcessTGID":                    {Status: "displayed", Ref: "CR-3 件③ 明细「进程 tgid=G」行"},
	"ProcessComm":                    {Status: "displayed", Ref: "CR-3 件③ 明细「comm=P」半场"},
	"ThermalCapWitnessed":            {Status: "displayed", Ref: "CR-3 件⑥ 受热限压/运行于(未见证) 词面门"},
	"UndrillableReason":              {Status: "displayed", Ref: "行1 ⊘链止(+wordless 补主语臂)"},
	"EffectiveImpactMS":              {Status: "displayed", Ref: "行1 席位输入 + 行2/行3 有效归因"},
	"EffectiveImpactPublished":       {Status: "internal_gate", Ref: "W-4 lead 拒冕臂(tree.go)"},
	"ActualImpactMS":                 {Status: "displayed", Ref: "行1 ⚠实际 + 明细实际口径行"},
	"SemanticChainProjectedMS":       {Status: "displayed", Ref: "行3 链上计入 + 明细(a)镜像"},
	"ActualTotalMS":                  {Status: "displayed", Ref: "明细实际口径行线程总量半场"},
	"ActualCaliberNote":              {Status: "displayed", Ref: "明细实际口径行判词"},
	"TargetImpactMS":                 {Status: "displayed", Ref: "覆盖句分子(链行)"},
	"DrilldownTarget":                {Status: "displayed", Ref: "lead next-step target 名"},
	"DrilldownEvidenceID":            {Status: "known_gap", Ref: "OM-3"},
	"DrilldownRelation":              {Status: "known_gap", Ref: "OM-3"},
	"MergedEvidenceIDs":              {Status: "displayed", Ref: "证据索引 E#(+N)"},
	"MergedCount":                    {Status: "displayed", Ref: "行1 ×N 五式"},
	"MergedMinMS":                    {Status: "displayed", Ref: "N次(a~b) 区间"},
	"MergedMaxMS":                    {Status: "displayed", Ref: "N次(a~b) 区间"},
	"MergedValuelessCount":           {Status: "displayed", Ref: "合并明细(值缺席成员)"},
	"MergedIntervalUnion":            {Status: "displayed", Ref: "N次union口径"},
	"MergedSumMS":                    {Status: "displayed", Ref: "×N SUM 口径"},
	"MergedQueryWindows":             {Status: "displayed", Ref: "明细窗来源(多窗合并行)"},
	"MergedCrossWindowMax":           {Status: "displayed", Ref: "N次跨窗取最大口径"},
	// RNB-5B 件⑥ (§29.96.2 终判⑥, 2026-07-15): the wire-fold source bit —
	// the 单次最大 equation face's typed trigger (coincidence trigger retired).
	"MergedWireFold": {Status: "internal_gate", Ref: "单次最大来源位(runtimeTraceProjCauseEventFoldRow)"},
	// RNB-5B 件⑦ (§29.96.2 终判⑦, 2026-07-15): the micro anchored-cut-seat
	// fold row marker (其余N项微额锚定席 word family, tree + ◎ board + detail).
	"MicroAnchorFold": {Status: "displayed", Ref: "其余N项微额锚定席(RNB-5B 件⑦)"},
	// RNB-2 件2 (§29.88 W3 病①, 2026-07-15): the merged-cleared bipartition
	// marker — 行2 seed-member qualifier (同源二分账留在各成员).
	"MergedChainAnchorMemberAccounts": {Status: "displayed", Ref: "行2 种子成员账限定词(RNB-2 件2)"},
	"MergedMaxWindowStartTs":          {Status: "displayed", Ref: "跨窗最大成员窗标"},
	"MergedMaxWindowEndTs":            {Status: "displayed", Ref: "跨窗最大成员窗标"},
	"RankQueryWindowStartTs":          {Status: "displayed", Ref: "多榜窗 chip(根因排序#N·窗X)"},
	"RankQueryWindowEndTs":            {Status: "displayed", Ref: "多榜窗 chip"},
	// XLANE-3 件1/件2 (§29.104.2 定谳③, 2026-07-16): the rank board identity
	// triple's target/params halves — the 板锚/参数# chip halves, the board
	// census split, and the 件3 cross-board mutual pointer inputs.
	"RankBoardTarget":               {Status: "displayed", Ref: "板锚 chip 半 + 跨板互指句(XLANE-3)"},
	"RankBoardParamsFingerprint":    {Status: "displayed", Ref: "参数# chip 半 + 板身份键(XLANE-3)"},
	"MergedActualDonorCumulativeMS": {Status: "internal_gate", Ref: "W-6 纯判官字段(DISP-3 假⚠ carve-out)"},
	// CR-2 组③ P7 (2026-07-12): the actual channel's physical interval — the
	// ⚠/超出发生段/区间未发布 word-face containment judge (never printed raw).
	"ActualWindowStartTs":   {Status: "internal_gate", Ref: "CR-2 P7 ⚠ 词面区间包含判官(runtimeTraceProjActualWindowScope)"},
	"ActualWindowEndTs":     {Status: "internal_gate", Ref: "CR-2 P7 ⚠ 词面区间包含判官(runtimeTraceProjActualWindowScope)"},
	"DuplicatePublications": {Status: "displayed", Ref: "行1 N次同值 + 明细重复发布"},
	"MergedSubjects":        {Status: "displayed", Ref: "×N 成员清单"},
	// RUN2FIX-A 件2 (2026-07-20): the fold's MAX-member identity carriers —
	// rendered as the fold row-2 「成员最大 <线程> · <状态> <值>ms」 clause.
	"MergedMaxSubject":   {Status: "displayed", Ref: "折叠行2 成员最大 线程·状态·值"},
	"MergedMaxStateKind": {Status: "displayed", Ref: "折叠行2 成员最大 线程·状态·值"},
	"SecondaryObjects":   {Status: "displayed", Ref: "明细影响点清单"},
	// ENG-2 (复核冷读 CP1-③, 2026-07-12): the absorbed idle-cadence
	// annotation — 「其中 X.XXXms 帧间空闲(等待下一帧)/周期空闲(…)」 on the
	// surviving seat + the matching teaching legend entry.
	"IdleCadenceMS":              {Status: "displayed", Ref: "行内 其中X.XXXms 帧间/周期空闲 注记"},
	"IdleCadenceKind":            {Status: "displayed", Ref: "同上(词面分叉键)+图例条"},
	"PriorityInversionCandidate": {Status: "displayed", Ref: "行1 ⇅ + 行2(典型经 canonical Object 词位)"},
	"RunnableBelowRTPreempted":   {Status: "displayed", Ref: "行2 (优先级低于RT) 尾缀"},
	"OnChainOverflowFold":        {Status: "displayed", Ref: "行1 折叠行(其余N项)"},
	"MergedAllDataGap":           {Status: "displayed", Ref: "全零注(禁席禁徽章门)"},
	"SameValueMembers":           {Status: "displayed", Ref: "证据索引 audit(W-3 设计:仅 audit 面)"},
	"FamilyMemberCount":          {Status: "displayed", Ref: "行1 合计词位 + 明细家族合并"},
	"FamilyMemberMaxMS":          {Status: "displayed", Ref: "明细成员范围"},
	"FamilyMemberMinMS":          {Status: "displayed", Ref: "明细成员范围"},
	"FamilyMemberSumMS":          {Status: "displayed", Ref: "行3 族形/明细"},
	"FamilyFoldCaliber":          {Status: "displayed", Ref: "族口径词"},
	"FamilyMemberRoster":         {Status: "displayed", Ref: "明细成员/区分键"},
	// XLANE-2 件1 (2026-07-17): the complete typed member line-range set —
	// subset-judgment input (the 为[E#]成员子集 pointer + ◎ footnote), and
	// since SPANTOP-1 (§29.131, 2026-07-18) ALSO printed verbatim as the
	// constituent sub-rows' 行a..b cells + the detail stanza's per-member
	// (行a..b) locators — internal_gate → displayed.
	"FamilyMemberLineRanges": {Status: "displayed", Ref: "行4+ 构成子行 行a..b + 明细成员(行a..b);兼 XLANE-2 成员子集判官"},
	// SPANTOP-1 件1 (§29.131, 2026-07-18): the complete per-member wall-clock
	// list — the constituent top-3 sub-row lane (µs identity gated).
	"FamilyMemberWallMS": {Status: "displayed", Ref: "行4+ 构成子行(runtimeTraceProjFamilySpanTopSubRows)"},
	// XLANE-2 件3 (2026-07-17): the absorbed-demoted account memory — a
	// compile-side (types aggregate) fold-key judge with a deliberately
	// word-less face (W-23).
	"AbsorbedWholeSeatDemotedView": {Status: "exempt", Ref: "W-23"},
	// XLANE-2 件2 (2026-07-17): the self-gap semantic-overlap roster — the
	// 行内 其中X与语义席[E#]重叠 clause (resolved at model build).
	"SelfGapSemanticOverlaps": {Status: "displayed", Ref: "行内 其中X与语义席[E#]重叠 clause(XLANE-2 件2)"},
	// AXIOM-V2 (2026-07-18): the fix-direction attribute word (行2 修向 X)
	// and the cross-direction mutual clause (与[E#](修向 X)同段重叠…收益不叠加).
	"FixDirection":           {Status: "displayed", Ref: "行2 修向 X + 图例(AXIOM-V2 件1)+ ◎ 方向节键(ELIM-V2)"},
	"CrossDirectionOverlaps": {Status: "displayed", Ref: "行内 与[E#](修向X)同段重叠…收益不叠加 互指句(AXIOM-V2 件2)+ ◎ ∩ chip 转录(ELIM-V2)"},
	// ELIM-V2 (2026-07-18): the parsed 件3 conservation finding — the ◎
	// 守恒尾行 violation transcription.
	"DirectionConservationExcess": {Status: "displayed", Ref: "◎ 守恒违例行(ELIM-V2 守恒尾行)"},
	"RankFamilyKey":               {Status: "displayed", Ref: "明细链上并入(G1 对账键)"},
	"AbsorbedByRankFamily":        {Status: "displayed", Ref: "明细链上并入 + audit"},
	"AbsorbedInto":                {Status: "displayed", Ref: "明细链上并入"},
	"BackgroundRank":              {Status: "exempt", Ref: "W-2 §29.36.2(词面缺席合规;semlead fold 转移为载体保真)"},
	"Inode":                       {Status: "displayed", Ref: "明细区分键"},
	"Dev":                         {Status: "displayed", Ref: "明细区分键"},
	"SubjectKind":                 {Status: "displayed", Ref: "行1 (跨线程累计,非墙钟) + ≈密度", Token: ".IsAggregateMetric("},
	"BlockingKind":                {Status: "displayed", Ref: "行1 ⊗+词位 + 明细五键行族"},
	"BlockingPeer":                {Status: "displayed", Ref: "行2 持有点对端"},
	"BlockingHolderSite":          {Status: "displayed", Ref: "明细持有点"},
	"BlockingFromSite":            {Status: "displayed", Ref: "明细等待点行"},
	"BlockingWaiters":             {Status: "known_gap", Ref: "OM-2"},
	"BlockingHolderSource":        {Status: "displayed", Ref: "明细持有者来历 + 推断 qualifier"},
	"BlockingOwnerTidRaw":         {Status: "displayed", Ref: "幻影 tid 半场词面"},
	// LOCKNS-FIX 修补 件A (冷读 P2-F1+P3-F7, 2026-07-16): typed presence
	// verdict — 持有者来历 presence 分句 fork (absent 保 legacy 句逐字节).
	"BlockingOwnerTidPresence":    {Status: "displayed", Ref: "明细持有者来历 presence 分句 fork(撞号/comm 不符)"},
	"BlockingHolderHandoff":       {Status: "displayed", Ref: "换手披露三面"},
	"BlockingHolderContradiction": {Status: "displayed", Ref: "自相矛盾撤回披露"},
	// LOCKNS-FIX 件6 / OM-10 关账 (§29.104.12, 2026-07-16): the ②×③
	// identity-unification declaration reaches the 持有者来历 line.
	"BlockingHolderNsUnification": {Status: "displayed", Ref: "明细持有者来历行两道互证括注"},
	// LOCKNS-FIX 件3 (§29.104.12, 2026-07-16): unknown-morphology fail-open
	// disclosure marker.
	"BlockingOwnerKeyUnregistered": {Status: "displayed", Ref: "明细持有者核查行(owner 未解析·形态未注册)"},
	// G10-EN 根修 (QH2-A, 2026-07-14): the typed witness components — the
	// zh/EN withdrawal lanes each word their own sentence from them.
	"BlockingHolderContradictionParts": {Status: "displayed", Ref: "自相矛盾撤回披露(两 lane 各自措辞)"},
	"BlockingSubjectIsHolder":          {Status: "displayed", Ref: "HOLD 朝向词面(twin-port)"},
	// XERR1-FIX 件1/件2/件3 (§29.104.3/.4, 2026-07-15): payload-less
	// blocking_span value-convergence carriage.
	"BlockingValueBasis":             {Status: "displayed", Ref: "件2 词面 fork(阻塞等待/span 包络)+ §24.3 形态族 + 明细值口径行"},
	"BlockingWaitSegmentMS":          {Status: "displayed", Ref: "明细值口径行 Σ 值"},
	"BlockingWaitSleepMS":            {Status: "internal_gate", Ref: "件1 互指 pair 门(sleep 分量>0)"},
	"BlockingSpanEnvelopeMS":         {Status: "displayed", Ref: "明细值口径行 + ⚠ 预算行 X 值"},
	"BlockingWaitBudgetExceeded":     {Status: "displayed", Ref: "行2 ⚠ 披露 + 明细等待预算核查行"},
	"BlockingWaitBudgetNonRunningMS": {Status: "displayed", Ref: "⚠ 预算行 Y 值"},
	"BlockingWaitBudgetRunningMS":    {Status: "displayed", Ref: "⚠ 预算行 Z 值"},
	// XERR1-FIX 修补 件F (冷读 P3-3, 2026-07-16): partial-coverage lower-bound
	// disclosure pair.
	"BlockingWaitCoveragePartial":  {Status: "displayed", Ref: "明细覆盖核查行(收敛值为已证下界)"},
	"BlockingWaitAccountCoveredMS": {Status: "displayed", Ref: "覆盖核查行账目值"},
	"TypeToken":                    {Status: "displayed", Ref: "行1 词位输入 + 行2 类别词 + 明细类型(raw)"},
	"GatedRunnableMS":              {Status: "displayed", Ref: "行1 词位构成 + 行3 恒等式(反转行)"},
	"GatedRunningDeficitMS":        {Status: "displayed", Ref: "行3 拆解(反转行)"},
	"PeriodicSource":               {Status: "displayed", Ref: "行2 周期性信号源 tag + 图例条"},
	"DetectedPeriodMS":             {Status: "displayed", Ref: "行2 周期 tag 值"},
	"PeriodicLatenessMS":           {Status: "known_gap", Ref: "OM-1"},
	"OccupierSummary":              {Status: "displayed", Ref: "行2 同窗占用者尾 tag + 明细"},
	"SupplyFoldComputed":           {Status: "displayed", Ref: "行2 折算 clause 在场信号"},
	"SupplyFoldDeficitMS":          {Status: "displayed", Ref: "行3 恒等式/拆解(供给折算)"},
	"SupplyFoldIdealMS":            {Status: "exempt", Ref: "W-9(值不印;拆解行经口径词代言)"},
	"SupplyFoldKnownMS":            {Status: "exempt", Ref: "W-9"},
	"SupplyFoldUnknownMS":          {Status: "exempt", Ref: "W-9"},
	"SupplyFoldCapabilitySource":   {Status: "displayed", Ref: "折算口径 capability 词面 fork"},
	// CLUSTER-FIX-2 件1 (S1): typed freq_only 五臂 enum 驱动折算口径后缀/句面
	// fork(supplyfold.go capSuffix/clause 消费)。
	"SupplyFoldCapabilityFreqOnlyReason": {Status: "displayed", Ref: "S1 freq_only 五臂口径词面 fork"},
	"GatedCapabilitySource":              {Status: "displayed", Ref: "按全域最大核最高频 capability 披露"},
	// DISPHYG-3 件7 (2026-07-20): gated reason twin — same S1 clause single
	// point as the supply-fold face.
	"GatedCapabilityFreqOnlyReason": {Status: "displayed", Ref: "gated freq_only 口径词面 fork(S1 twin)"},
	// R5 (§29.88.12 单基准, 2026-07-15): the demoted-reference basis words
	// (按X核满频) retired with the demotion arm — the field stays a wire/audit
	// record of the basis cluster's class; no display word forks on it.
	"SupplyFoldReferenceClass":     {Status: "exempt", Ref: "W-22(R5 单基准:词面不再随基准簇类分叉)"},
	"SupplyFoldTopologySource":     {Status: "displayed", Ref: "簇拓扑折算词面 fork"},
	"GatedTopologySource":          {Status: "displayed", Ref: "gated 簇拓扑词面 fork"},
	"ThermalCapKHz":                {Status: "displayed", Ref: "热限压披露句"},
	"RunnableMS":                   {Status: "displayed", Ref: "行2 机制句量级(RN-1 显著性门)"},
	"DStateSplitMS":                {Status: "displayed", Ref: "WO-A1 加法恒等式判定输入(不可相加指针词面)"},
	"IOWaitSplitMS":                {Status: "displayed", Ref: "WO-A1 加法恒等式判定输入(不可相加指针词面)"},
	"OverflowMirrorEvidenceIDs":    {Status: "displayed", Ref: "WO-D1③ 折叠池 headline 多引用镜像 tag"},
	"OverflowProjectionEvidenceID": {Status: "displayed", Ref: "P2-2 折叠池跨口径投影 tag"},
	"FullWindowStateMS":            {Status: "displayed", Ref: "行2 全窗合计 tag + 明细全窗合计行"},
	"FullWindowStateSource":        {Status: "displayed", Ref: "全窗合计来源词"},
	"FullWindowStateWindowStart":   {Status: "displayed", Ref: "全窗合计窗标"},
	"FullWindowStateWindowEnd":     {Status: "displayed", Ref: "全窗合计窗标"},
	"FullWindowStateSameWindow":    {Status: "displayed", Ref: "全窗==查询窗判词(F-2)"},
}

// --- T1 · TraceCausalProjection contract (A 区) --------------------------------

var projectionFieldContract = map[string]fieldDisposition{
	"PrimaryRootCause":             {Status: "displayed", Ref: "行族桶来源(F1–F7)"},
	"PrimaryRootCauses":            {Status: "displayed", Ref: "行族桶来源"},
	"OnChainCauses":                {Status: "displayed", Ref: "行族桶来源"},
	"AdjacentCauses":               {Status: "displayed", Ref: "◇ 区段来源"},
	"BackgroundCauses":             {Status: "displayed", Ref: "▒ 区段来源"},
	"SemanticSpans":                {Status: "displayed", Ref: "✦ 语义行来源"},
	"WakeupPath":                   {Status: "displayed", Ref: "树干 rails/edge"},
	"WakeupPathUserElected":        {Status: "internal_gate", Ref: "R2 比较短路"},
	"WakeupPathUserEntityHits":     {Status: "internal_gate", Ref: "W-8 折叠强拆选择器(效果结构性可见)"},
	"WakeupPathBranch":             {Status: "internal_gate", Ref: "W-7 depth 挂靠门"},
	"WakeupPathRootDepth":          {Status: "internal_gate", Ref: "W-7 depth 挂靠门"},
	"WakeupPathQueryWindowStartTs": {Status: "internal_gate", Ref: "W-7 窗对门"},
	"WakeupPathQueryWindowEndTs":   {Status: "internal_gate", Ref: "W-7 窗对门"},
	"SupportingHops":               {Status: "displayed", Ref: "并入树行池 + 证据索引"},
	"WakeupChainRecommendedNotRun": {Status: "displayed", Ref: "flat 三形树头(⊘ 未下钻)"},
	"RootCauseFamilyObserved":      {Status: "displayed", Ref: "树头分叉"},
	"WindowStartTs":                {Status: "displayed", Ref: "满格=窗口/回退尺度句 + 证据索引"},
	"WindowEndTs":                  {Status: "displayed", Ref: "尺度句"},
	"ArtifactPath":                 {Status: "displayed", Ref: "多工件树头"},
	"ArtifactLabel":                {Status: "displayed", Ref: "多工件树头/章节后缀"},
	"CapacityTruncated":            {Status: "displayed", Ref: "证据索引头披露"},
	"QueryWindows":                 {Status: "displayed", Ref: "树头含N个查询窗"},
	"QueryWindowsTruncated":        {Status: "displayed", Ref: "查询窗清单截断注"},
	"AbsorbedChainRows":            {Status: "displayed", Ref: "明细链上并入 + 证据索引"},
	"TargetStateAccount":           {Status: "displayed", Ref: "F10 两行(Σ==窗门) + 图例条"},
	// SPANVIS-1 (2026-07-19): the ◈ pure-advisory mention side channel —
	// tree-fence advisory block + ◎ 旁栏 footnote (word-face single point:
	// runtimeTraceProjBusinessSpanMentionRowText) + the honest omitted row.
	"BusinessSpanMentions":       {Status: "displayed", Ref: "◈ 业务span提示块 + ◎ 旁栏(runtimeTraceProjBusinessSpanMentionLines)"},
	"BusinessSpanMentionOmitted": {Status: "displayed", Ref: "◈ 另有N个span族截断行"},
	// PARTSPLIT-1 (§29.150④): the R4-refusal non-seat mention side channel.
	"GatedCompositeEdgeShareDisclosures": {Status: "displayed", Ref: "PARTSPLIT-1 ◎ 边前份披露(R4拒转·整席不拆)非席 mention 块(runtimeTraceProjElimGatedCompositeEdgeShareMentionLines)"},
	// RULER2-1 (§29.150②): the self runnable two-ruler accounting side
	// channel — the 行2 按两把尺记账 cross-row sentence on the lead seat row.
	"SelfRunnableTwoRulerAccountings": {Status: "displayed", Ref: "RULER2-1 行2 按两把尺记账句(runtimeTraceProjSelfRunnableTwoRulerTagText,lead 行 stamp)"},
	// SELFRUN-DISC (§29.192① (b)): the self supply-fold 「量不了」 absence
	// disclosure side channel — the ◎ auxiliary 另账 折算不可量 row.
	"SelfRunningFoldUnmeasured": {Status: "displayed", Ref: "SELFRUN-DISC ◎ 另账 折算不可量 行(runtimeTraceProjElimSelfFoldUnmeasuredRows,词面单点 runtimeTraceProjSelfFoldUnmeasuredSentence)"},
}

// --- T1 · TargetStateAccount / QueryWindow contracts (C 区) --------------------
//
// Field names of these small structs collide with Node/RankItem names in the
// display sources (e.g. account.EvidenceID vs node.EvidenceID), so scan arms
// are disabled row-by-row with the collision documented; the registration,
// ghost, and registry-reference arms still apply.

var targetStateAccountContract = map[string]fieldDisposition{
	"Subject":                {Status: "displayed", Ref: "F10 账主语", NoScan: true},
	"RunningMS":              {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"RunnableMS":             {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"SleepMS":                {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"DStateMS":               {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"IOWaitMS":               {Status: "displayed", Ref: "F10 两行分量", NoScan: true},
	"SleepIOWaitMS":          {Status: "displayed", Ref: "其中IO等待从句(Σ门忽略)", NoScan: true},
	"TotalMS":                {Status: "exempt", Ref: "W-10", NoScan: true},
	"DeterministicRunningMS": {Status: "displayed", Ref: "F10 确定性运行从句", NoScan: true},
	// ANSWERFACE-1 件2 (§29.140 G6, 2026-07-19): boundary-fold disclosure
	// quartet — the 「含未覆盖段 X 折入」 in-term clause (Σ门忽略,披露非加项).
	"HeadCarryMS":    {Status: "displayed", Ref: "含未覆盖段折入从句(Σ门忽略)", NoScan: true},
	"HeadCarryState": {Status: "displayed", Ref: "含未覆盖段折入从句(车道选择)", NoScan: true},
	"TailOpenMS":     {Status: "displayed", Ref: "含未覆盖段折入从句(Σ门忽略)", NoScan: true},
	"TailOpenState":  {Status: "displayed", Ref: "含未覆盖段折入从句(车道选择)", NoScan: true},
	"WindowStartTs":  {Status: "internal_gate", Ref: "F-2 容差锚窗门", NoScan: true},
	"WindowEndTs":    {Status: "internal_gate", Ref: "F-2 容差锚窗门", NoScan: true},
	"EvidenceID":     {Status: "known_gap", Ref: "OM-5", NoScan: true},
}

var queryWindowContract = map[string]fieldDisposition{
	"StartTs": {Status: "displayed", Ref: "窗清单/audit 载体", NoScan: true},
	"EndTs":   {Status: "displayed", Ref: "窗清单/audit 载体", NoScan: true},
}

// --- T1 · display-layer fold-peer carrier (OM-6 host struct) -------------------

var rankFoldPeerContract = map[string]fieldDisposition{
	"TypeWord":           {Status: "known_gap", Ref: "OM-6"},
	"Rank":               {Status: "displayed", Ref: "行2 榜位(fold-adopted)", NoScan: true},
	"Confidence":         {Status: "displayed", Ref: "行2 置信(fold-adopted)", NoScan: true},
	"EvidenceTag":        {Status: "displayed", Ref: "行1 [E#+E#] bracket + 明细根因排序行"},
	"CumulativeImpactMS": {Status: "internal_gate", Ref: "W-A 累计相等 fold guard + 覆盖分子不变量", NoScan: true},
	"DisplayImpactMS":    {Status: "internal_gate", Ref: "bar scale/unadmitted-disclosure MAX 不变量"},
	"TargetImpactMS":     {Status: "internal_gate", Ref: "覆盖分子不变量(COV D-1)", NoScan: true},
}

// --- T2 armB · RootCauseRankItem contract (D 区) --------------------------------

// rankItemNodeMirror maps wire field → projection mirror where the names
// differ (same-name and Ms→MS-normalized mirrors resolve automatically).
var rankItemNodeMirror = map[string]string{
	"Type":                         "TypeToken",
	"Thread":                       "Subject",
	"DominantState":                "StateKind",
	"LatenessMs":                   "PeriodicLatenessMS",
	"ProjectedImpactMs":            "ImpactMS",
	"GatedClusterTopology":         "GatedTopologySource",
	"SupplyFoldBasis":              "SupplyFoldComputed",
	"HolderSite":                   "BlockingHolderSite",
	"SubjectIsLockHolder":          "BlockingSubjectIsHolder",
	"HolderSource":                 "BlockingHolderSource",
	"OwnerTidRaw":                  "BlockingOwnerTidRaw",
	"OwnerTidPresence":             "BlockingOwnerTidPresence",
	"HolderNsUnification":          "BlockingHolderNsUnification",
	"HolderHandoff":                "BlockingHolderHandoff",
	"HolderSelfContradiction":      "BlockingHolderContradiction",
	"HolderSelfContradictionParts": "BlockingHolderContradictionParts",
	"MemberCount":                  "FamilyMemberCount",
	"MemberLineRanges":             "FamilyMemberLineRanges",
	"MemberWallMs":                 "FamilyMemberWallMS",
	"MemberRoster":                 "FamilyMemberRoster",
	"MemberMaxMs":                  "FamilyMemberMaxMS",
	"MemberMinMs":                  "FamilyMemberMinMS",
	"MemberSumMs":                  "FamilyMemberSumMS",
	"MemberFoldCaliber":            "FamilyFoldCaliber",
	"AbsorbedIntoFamily":           "AbsorbedInto",
	"AbsorbedChainRows":            "AbsorbedChainRows", // projection-level mirror
}

var rankItemContract = map[string]fieldDisposition{
	"Rank":                          {Status: "node_mirror", Ref: "Node.Rank"},
	"Tier":                          {Status: "node_mirror", Ref: "Node.Tier"},
	"BackgroundRank":                {Status: "node_mirror", Ref: "Node.BackgroundRank(W-2 词面豁免在 Node 侧)"},
	"Type":                          {Status: "node_mirror", Ref: "Node.TypeToken"},
	"SubjectKind":                   {Status: "node_mirror", Ref: "Node.SubjectKind"},
	"Thread":                        {Status: "node_mirror", Ref: "Node.Subject"},
	"PhysicalSourcePath":            {Status: "exempt", Ref: "W-19"},
	"PerfContext":                   {Status: "exempt", Ref: "W-18"},
	"PerfContexts":                  {Status: "exempt", Ref: "W-18"},
	"StartTs":                       {Status: "node_mirror", Ref: "Node.StartTs"},
	"EndTs":                         {Status: "node_mirror", Ref: "Node.EndTs"},
	"ActualStartTs":                 {Status: "exempt", Ref: "W-12(收窄已折 IC-A)"},
	"ActualEndTs":                   {Status: "exempt", Ref: "W-12(收窄已折 IC-A)"},
	"DominantState":                 {Status: "node_mirror", Ref: "Node.StateKind"},
	"RunningMs":                     {Status: "known_gap", Ref: "OM-12"},
	"RunnableMs":                    {Status: "node_mirror", Ref: "Node.RunnableMS"},
	"SleepMs":                       {Status: "known_gap", Ref: "OM-12"},
	"DStateMs":                      {Status: "known_gap", Ref: "OM-12"},
	"IOWaitMs":                      {Status: "known_gap", Ref: "OM-12"},
	"DStateAllNonIOProven":          {Status: "note_consumed", Ref: "dstate_all_noniowait → Node.DStateRefinedNonIO"},
	"DStateCauseUnprovenRemainder":  {Status: "note_consumed", Ref: "dstate_cause_unproven_remainder → Node.DStateCauseUnprovenRemainder"},
	"ChainAnchoredMs":               {Status: "note_consumed", Ref: "chain_anchored → Node.ChainAnchoredMS(RSPA §29.61.10 行2 分解)"},
	"ChainAnchorFullMs":             {Status: "note_consumed", Ref: "chain_anchor_full → Node.ChainAnchorFullMS(RSPA 行2 分解+WO-C1 同源二分句)"},
	"ChainAnchorRemainderSeat":      {Status: "note_consumed", Ref: "chain_anchor_remainder_seat → Node.ChainAnchorRemainderSeat(RSPA ◇ 余段席词面门)"},
	"ChainAnchorOwnershipDivergent": {Status: "note_consumed", Ref: "chain_anchor_ownership_divergent → Node.ChainAnchorOwnershipDivergent(RNB-1 案A' 账目关系句门)"},
	"ChainAnchorChainLaneMs":        {Status: "note_consumed", Ref: "chain_anchor_chain_lane → Node.ChainAnchorChainLaneMS(RNB-1 双Σ披露)"},
	"ChainAnchorCensusMs":           {Status: "note_consumed", Ref: "chain_anchor_census → Node.ChainAnchorCensusMS(RNB-1 双Σ披露)"},
	"ChainCredentialLaneDemoted":    {Status: "note_consumed", Ref: "chain_credential_lane_demoted → Node.ChainCredentialLaneDemoted(RNB-1 R4 整席不入链上榜披露行)"},
	// ONCHAIN-FIX-1 件1 (2026-07-18): the interval-less identity-inheritance
	// admission marker (emitted only on the current on-chain lane).
	"ChainIdentityInheritance": {Status: "note_consumed", Ref: "chain_identity_inheritance → Node.ChainIdentityInheritance(ONCHAIN-FIX-1 行2 身份继承披露行)"},
	// ONCHAIN-FIX-2 件1 (包络泛化, 2026-07-18): the rank-lane envelope-tier
	// honest word (same key/legend as the critical face; emitted only on the
	// current keep-⛓ lane).
	"ChainCredentialEnvelopeLevel": {Status: "note_consumed", Ref: "chain_credential_envelope_level → Node.ChainCredentialEnvelopeLevel(ONCHAIN-FIX-2 件1 行2 (包络级凭证) 诚实注,rank 面复用 HULL-CRED 词)"},
	// XLANE-1 件1 (§29.104.2, 2026-07-15): the represented-by-chain-seat marker.
	"ChainAnchorRepresentedByChainSeat": {Status: "note_consumed", Ref: "chain_anchor_represented_by_chain_seat → Node.ChainAnchorRepresentedByChainSeat(XLANE-1 锚定份由链席代表披露行)"},
	// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): the gated-share split
	// family → Node.GatedShare* (行2 分账披露行 + [E#] 互指 + 裁定④ 句).
	"GatedShareClaimedMs":           {Status: "note_consumed", Ref: "gated_share_claimed → Node.GatedShareClaimedMS(件2 行2 分账披露)"},
	"GatedShareFullMs":              {Status: "note_consumed", Ref: "gated_share_full → Node.GatedShareFullMS(件2 行2 分账披露)"},
	"GatedShareConstituentSeat":     {Status: "note_consumed", Ref: "gated_share_constituent_seat → Node.GatedShareConstituentSeat(件2 A 构成行词面门)"},
	"GatedShareClaimSeats":          {Status: "note_consumed", Ref: "gated_share_claim_seats → Node.GatedShareClaimSeats(件2 [E#] 反转席互指)"},
	"GatedShareOverlapDisclosureMs": {Status: "note_consumed", Ref: "gated_share_overlap → Node.GatedShareOverlapDisclosureMS(件2 裁定④ fail-open 披露句)"},
	// PARTSPLIT-1 (§29.150④): the R4-mirror refusal record quartet.
	"GatedCompositeEdgePreShareMs":  {Status: "note_consumed", Ref: "gated_composite_edge_pre_share → Node.GatedCompositeEdgePreShareMS(行2 分账披露)"},
	"GatedCompositeEdgePostShareMs": {Status: "note_consumed", Ref: "gated_composite_edge_post_share → Node.GatedCompositeEdgePostShareMS(行2 分账披露)"},
	"GatedCompositeEdgeAnchorTs":    {Status: "note_consumed", Ref: "gated_composite_edge_anchor_ts → Node.GatedCompositeEdgeAnchorTS(行2 分账披露 + ◎ mention anchor match)"},
	"GatedCompositeEdgeAnchorVia":   {Status: "note_consumed", Ref: "gated_composite_edge_anchor_via → Node.GatedCompositeEdgeAnchorVia(行2 分账披露 凭证词)"},
	// R3-IMPL (§29.88.1, 2026-07-15): the host-edge-anchored semantic seat's
	// credential disclosure pair → Node.HostWakeupEdgeAnchor* (行2 边锚定句).
	"HostWakeupEdgeAnchorTs":  {Status: "note_consumed", Ref: "host_wakeup_edge_anchor_ts → Node.HostWakeupEdgeAnchorTS(R3 行2 唤醒锚定(宿主→目标) 句)"},
	"HostWakeupEdgeAnchorVia": {Status: "note_consumed", Ref: "host_wakeup_edge_anchor_via → Node.HostWakeupEdgeAnchorVia(R3 行2 唤醒锚定(宿主→目标) 句)"},
	// RNB-2 件5 AFF-EVID (§29.88.6, 2026-07-15): affinity/cpuset judgment
	// payload quintet → Node.CPUConstraint* (行3 CPU约束描述行).
	"CPUConstraintKind":              {Status: "note_consumed", Ref: "cpu_constraint_kind → Node.CPUConstraintKind(RNB-2 件5 约束描述行)"},
	"CPUConstraintCPUSet":            {Status: "note_consumed", Ref: "cpu_constraint_cpuset → Node.CPUConstraintCPUSet(RNB-2 件5 约束描述行)"},
	"CPUConstraintPolicy":            {Status: "note_consumed", Ref: "cpu_constraint_policy → Node.CPUConstraintPolicy(RNB-2 件5 restricted 词面门)"},
	"CPUConstraintAllowedCPUs":       {Status: "note_consumed", Ref: "cpu_constraint_allowed_cpus → Node.CPUConstraintAllowedCPUs(RNB-2 件5 允许核集)"},
	"CPUConstraintExcludedCPUs":      {Status: "note_consumed", Ref: "cpu_constraint_excluded_cpus → Node.CPUConstraintExcludedCPUs(RNB-2 件5 全域对照排除集;R5a 预留)"},
	"CPUConstraintAllowedMaxTierKHz": {Status: "note_consumed", Ref: "cpu_constraint_allowed_max_tier_khz → Node.CPUConstraintAllowedMaxTierKHz(R5a 按核档)"},
	"CPUConstraintGlobalMaxTierKHz":  {Status: "note_consumed", Ref: "cpu_constraint_global_max_tier_khz → Node.CPUConstraintGlobalMaxTierKHz(R5a 按核档)"},
	"ResourceCompletionClosure":      {Status: "note_consumed", Ref: "resource_completion_closure → Node.ResourceCompletionClosure(RSPA M-IO 完成闭合注记)"},
	"BlockedReasonCaller":            {Status: "node_mirror", Ref: "Node.BlockedReasonCaller"},
	"BlockedReasonWindowCount":       {Status: "node_mirror", Ref: "CR-3 件② Node.BlockedReasonWindowCount"},
	"BlockedReasonWindowCaller":      {Status: "node_mirror", Ref: "CR-3 件② Node.BlockedReasonWindowCaller"},
	"ProcessComm":                    {Status: "note_consumed", Ref: "process_comm → Node.ProcessComm(CR-3 件③,+板摘要席位)"},
	"ImpactMs":                       {Status: "node_mirror", Ref: "Node.ImpactMS"},
	"ProjectedImpactMs":              {Status: "node_mirror", Ref: "Node.ImpactMS(窗口投影值;projected_impact_ms note 为 display-only 回声)"},
	"CumulativeImpactMs":             {Status: "node_mirror", Ref: "Node.CumulativeImpactMS"},
	"EffectiveImpactMs":              {Status: "node_mirror", Ref: "Node.EffectiveImpactMS"},
	"RankSortBoostedEffectiveMs":     {Status: "exempt", Ref: "W-1(json:\"-\" 内部排序道)"},
	"GatedRunnableMs":                {Status: "node_mirror", Ref: "Node.GatedRunnableMS"},
	"GatedRunningDeficitMs":          {Status: "node_mirror", Ref: "Node.GatedRunningDeficitMS"},
	"GatedCapabilitySource":          {Status: "node_mirror", Ref: "Node.GatedCapabilitySource"},
	"GatedClusterTopology":           {Status: "node_mirror", Ref: "Node.GatedTopologySource"},
	// DISPHYG-3 件7: gated reason twin (same-name mirror).
	"GatedCapabilityFreqOnlyReason": {Status: "node_mirror", Ref: "Node.GatedCapabilityFreqOnlyReason"},
	// TQ-PRIORITY-POINT-AUTHORITY: these three fields are hard engine inputs
	// to the relation-scoped effective account. The publication layer also
	// emits their registered audit notes, but the display projection does not
	// independently re-adjudicate priority authority.
	"PriorityRelationCaliber":             {Status: "engine_gate", Ref: "single priority authority proof caliber gates inversion minting"},
	"PriorityRelationProvenLowerMs":       {Status: "engine_gate", Ref: "only proven-lower relation slices enter Effective"},
	"PriorityRelationUnknownOrNonLowerMs": {Status: "engine_gate", Ref: "unknown/equal/higher remainder contributes zero"},
	"PriorityRelationArtifactSources":     {Status: "note_displayed", Ref: "priority_relation_artifact_sources → public physical-provenance note/banner"},
	"PeriodicSource":                      {Status: "node_mirror", Ref: "Node.PeriodicSource"},
	"DetectedPeriodMs":                    {Status: "node_mirror", Ref: "Node.DetectedPeriodMS"},
	"LatenessMs":                          {Status: "node_mirror", Ref: "Node.PeriodicLatenessMS(镜像在;显示缺口挂 Node 侧 OM-1)"},
	"SupplyFoldDeficitMs":                 {Status: "node_mirror", Ref: "Node.SupplyFoldDeficitMS"},
	"SupplyFoldIdealMs":                   {Status: "node_mirror", Ref: "Node.SupplyFoldIdealMS(W-9 值不印在 Node 侧)"},
	"SupplyFoldBasis":                     {Status: "node_mirror", Ref: "Node.SupplyFoldComputed(fold_basis 在场信号)"},
	"TargetImpactMs":                      {Status: "node_mirror", Ref: "Node.TargetImpactMS"},
	"ActualImpactMs":                      {Status: "node_mirror", Ref: "Node.ActualImpactMS"},
	"ActualTotalMs":                       {Status: "node_mirror", Ref: "Node.ActualTotalMS"},
	"Score":                               {Status: "exempt", Ref: "W-1 S1 修根"},
	"Confidence":                          {Status: "node_mirror", Ref: "Node.Confidence"},
	"LineStart":                           {Status: "node_mirror", Ref: "Node.LineStart"},
	"LineEnd":                             {Status: "node_mirror", Ref: "Node.LineEnd"},
	"Source":                              {Status: "exempt", Ref: "W-14(收窄已折 IC-A)"},
	"Causality":                           {Status: "node_mirror", Ref: "Node.Causality"},
	"ChainRelevance":                      {Status: "node_mirror", Ref: "Node.ChainRelevance"},
	"OnChainBasis":                        {Status: "node_mirror", Ref: "Node.OnChainBasis(SELF-SEM §29.61.1)"},
	"ChainDepth":                          {Status: "node_mirror", Ref: "Node.ChainDepth"},
	"ChainBranch":                         {Status: "node_mirror", Ref: "Node.ChainBranch(W-7 门在 Node 侧)"},
	"OverlapMs":                           {Status: "known_gap", Ref: "OM-9"},
	"EdgeCount":                           {Status: "known_gap", Ref: "OM-9"},
	"NearestChainThread":                  {Status: "known_gap", Ref: "OM-9"},
	"NearestChainWindow":                  {Status: "known_gap", Ref: "OM-9"},
	"OccurrenceWindows":                   {Status: "exempt", Ref: "W-13"},
	"StatsWindowStartTs":                  {Status: "note_consumed", Ref: "note:selected_window → Node.QueryWindow*(W-17 非缺口)"},
	"StatsWindowEndTs":                    {Status: "note_consumed", Ref: "note:selected_window → Node.QueryWindow*(W-17 非缺口)"},
	"BlockingKind":                        {Status: "node_mirror", Ref: "Node.BlockingKind"},
	"BlockingPeer":                        {Status: "node_mirror", Ref: "Node.BlockingPeer"},
	"HolderSite":                          {Status: "node_mirror", Ref: "Node.BlockingHolderSite"},
	"BlockingFromSite":                    {Status: "node_mirror", Ref: "Node.BlockingFromSite"},
	"SubjectIsAnalysisTarget":             {Status: "engine_gate", Ref: "SYM §24.13 裁定一 target_self_state 降级门(query.go/root_cause_rank_capacity.go);效果经 tier 词面现形"},
	"RunnableBelowRTPreempted":            {Status: "node_mirror", Ref: "Node.RunnableBelowRTPreempted"},
	"SubjectIsLockHolder":                 {Status: "node_mirror", Ref: "Node.BlockingSubjectIsHolder"},
	"HolderSource":                        {Status: "node_mirror", Ref: "Node.BlockingHolderSource"},
	"OwnerTidRaw":                         {Status: "node_mirror", Ref: "Node.BlockingOwnerTidRaw"},
	// LOCKNS-FIX 修补 件A (2026-07-16): typed presence verdict beside the raw
	// tid (持有者来历 presence 分句 fork).
	"OwnerTidPresence": {Status: "node_mirror", Ref: "Node.BlockingOwnerTidPresence"},
	// LOCKNS-FIX 件6 (§29.104.12, 2026-07-16): OM-10 关账 for the unification
	// half — the declaration now rides Node.BlockingHolderNsUnification into
	// the 持有者来历 line. The process-level identity note stays the open
	// half (IC-L neighborhood).
	"HolderNsUnification":            {Status: "node_mirror", Ref: "Node.BlockingHolderNsUnification"},
	"HolderHostProcess":              {Status: "known_gap", Ref: "OM-10(进程级半场;unification 半场已关账)"},
	"HolderHandoff":                  {Status: "node_mirror", Ref: "Node.BlockingHolderHandoff"},
	"HolderSelfContradiction":        {Status: "node_mirror", Ref: "Node.BlockingHolderContradiction"},
	"HolderSelfContradictionParts":   {Status: "node_mirror", Ref: "Node.BlockingHolderContradictionParts"},
	"DrillStatus":                    {Status: "known_gap", Ref: "OM-7"},
	"InheritedTargetBlockedMs":       {Status: "known_gap", Ref: "OM-13"},
	"PriorityInversionLockDominated": {Status: "known_gap", Ref: "OM-8"},
	"SpanName":                       {Status: "node_mirror", Ref: "Node.SpanName"},
	"SpanKind":                       {Status: "node_mirror", Ref: "Node.SpanKind(镜像在;显示缺口挂 Node 侧 OM-4)"},
	"SpanCategory":                   {Status: "node_mirror", Ref: "Node.SpanCategory(OM-4 在 Node 侧)"},
	"SpanSubcategory":                {Status: "node_mirror", Ref: "Node.SpanSubcategory(OM-4 在 Node 侧)"},
	"SemanticClass":                  {Status: "node_mirror", Ref: "Node.SemanticClass"},
	"MemberCount":                    {Status: "node_mirror", Ref: "Node.FamilyMemberCount"},
	"MemberRoster":                   {Status: "node_mirror", Ref: "Node.FamilyMemberRoster"},
	"MemberLineRanges":               {Status: "node_mirror", Ref: "Node.FamilyMemberLineRanges(XLANE-2 件1)"},
	"MemberWallMs":                   {Status: "node_mirror", Ref: "Node.FamilyMemberWallMS(SPANTOP-1 件1)"},
	"SelfGapSemanticOverlaps":        {Status: "node_mirror", Ref: "Node.SelfGapSemanticOverlaps(XLANE-2 件2)"},
	// AXIOM-V2 (2026-07-18): 件1/件2 mirror into node fields; the 件3 audit
	// pair rides the generic detail-audit note rendering only (立案素材 —
	// deliberately no display word face, 宁漏勿假指 on the un-pointable arm).
	"FixDirection":                     {Status: "node_mirror", Ref: "Node.FixDirection(AXIOM-V2 件1)"},
	"CrossDirectionOverlaps":           {Status: "node_mirror", Ref: "Node.CrossDirectionOverlaps(AXIOM-V2 件2)"},
	"CrossDirectionOverlapUndisclosed": {Status: "note_displayed", Ref: "note:cross_direction_overlap_undisclosed 审计面(AXIOM-V2 件3)"},
	"DirectionConservationExcess":      {Status: "node_mirror", Ref: "Node.DirectionConservationExcess(ELIM-V2 守恒尾行)"},
	// P3MEASURE-1 (§29.169, 2026-07-20): the silent on-chain measurement —
	// display_only audit wire, model/user double-invisible BY DESIGN (the
	// note_displayed status here asserts carrier==display_only, i.e. no
	// parsing consumer may ever appear without reddening the census; the
	// no-rendered-face half is pinned by the flagship A/B + the p3m_
	// consumer-absence pin). Advisory-only red line: never a hard gate.
	"P3MCounterfactualValidMs":   {Status: "note_displayed", Ref: "note:p3m_counterfactual_valid_ms 静默量测审计面(P3MEASURE-1)"},
	"P3MCounterfactualInvalidMs": {Status: "note_displayed", Ref: "note:p3m_counterfactual_invalid_ms 静默量测审计面(P3MEASURE-1)"},
	"P3MEdgeWitnessedMs":         {Status: "note_displayed", Ref: "note:p3m_edge_witnessed_ms 静默量测审计面(P3MEASURE-1)"},
	"P3MDisposition":             {Status: "note_displayed", Ref: "note:p3m_disposition 静默量测审计面(P3MEASURE-1)"},
	"MemberMaxMs":                {Status: "node_mirror", Ref: "Node.FamilyMemberMaxMS"},
	"MemberMinMs":                {Status: "node_mirror", Ref: "Node.FamilyMemberMinMS"},
	"MemberSumMs":                {Status: "node_mirror", Ref: "Node.FamilyMemberSumMS"},
	"MemberFoldCaliber":          {Status: "node_mirror", Ref: "Node.FamilyFoldCaliber"},
	"MemberKey":                  {Status: "exempt", Ref: "W-15"},
	"Inode":                      {Status: "node_mirror", Ref: "Node.Inode"},
	"Dev":                        {Status: "node_mirror", Ref: "Node.Dev"},
	"TraceGapKind":               {Status: "node_mirror", Ref: "Node.TraceGapKind"},
	"RankFamilyKey":              {Status: "node_mirror", Ref: "Node.RankFamilyKey"},
	"AbsorbedChainRows":          {Status: "node_mirror", Ref: "Projection.AbsorbedChainRows"},
	"AbsorbedRankRows":           {Status: "exempt", Ref: "W-16(收窄已折 IC-A)"},
	"AbsorbedByRankFamily":       {Status: "node_mirror", Ref: "Node.AbsorbedByRankFamily"},
	"AbsorbedIntoFamily":         {Status: "node_mirror", Ref: "Node.AbsorbedInto"},
	"Summary":                    {Status: "exempt", Ref: "W-11(LLM 面载体)"},
}

// --- census machinery -----------------------------------------------------------

var infoContractDisplayAuthorityFiles = []string{
	"answer_document_mutation_runtime_tree.go",
	"answer_document_mutation_runtime_rcr.go",
	"answer_document_mutation_runtime.go",
	"answer_document_mutation_runtime_typelabels.go",
	"answer_document_mutation_runtime_supplyfold.go",
	"answer_document_mutation_runtime_rcm.go",
	// SMR-1 批 (2026-07-12): the relation-arm passes (WO-A1/D2/D3/C1/B1).
	"answer_document_mutation_runtime_smr1.go",
	// XERR1-FIX 件1 (2026-07-15): the blocking↔sleep 互指 pair arm.
	"answer_document_mutation_runtime_xerr1.go",
	// XLANE-2 件1 (2026-07-17): the semantic member-subset judgment pass.
	"answer_document_mutation_runtime_xlane2.go",
	// LEVELMERGE-1 件2/件3 (2026-07-18): the gated-share split + aggregate↔
	// member cross-reference stamp passes.
	"answer_document_mutation_runtime_levelmerge.go",
	// AXIOM-V2 件2 (2026-07-18): the cross-direction mutual-clause
	// resolution pass + the fix-direction word table.
	"answer_document_mutation_runtime_axiomv2.go",
	// ELIM-V2 方向分组制 (2026-07-18): the ◎ direction sections, the ∩ chip
	// transcription and the 守恒尾行 consumer.
	"answer_document_mutation_runtime_elim.go",
	// PARTSPLIT-1 (§29.150④, 2026-07-19): the R4-refusal pre-edge-share
	// disclosure faces (行2 分账 sub-line + ◎ non-seat mention block).
	"answer_document_mutation_runtime_partsplit.go",
}

func readDisplayAuthoritySources(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, name := range infoContractDisplayAuthorityFiles {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(uxg1StripComments(string(raw)))
		b.WriteString("\n")
	}
	return b.String()
}

func readTracequeryEngineSources(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir("../tracequery")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("../tracequery", name))
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(uxg1StripComments(string(raw)))
		b.WriteString("\n")
	}
	return b.String()
}

func identTokenAppears(src, field, override string) bool {
	if override != "" {
		return strings.Contains(src, override)
	}
	re := regexp.MustCompile(`\.` + regexp.QuoteMeta(field) + `\b`)
	return re.MatchString(src)
}

// infoContractCheckRegistration: registration + ghost + registry-reference
// arms shared by every contract table.
func infoContractCheckRegistration(t *testing.T, table string, typ reflect.Type, contract map[string]fieldDisposition, statuses map[string]bool, usedW, usedOM map[string]bool) {
	t.Helper()
	for _, f := range reflect.VisibleFields(typ) {
		if !f.IsExported() || f.Anonymous {
			continue
		}
		if _, ok := contract[f.Name]; !ok {
			t.Errorf("%s: 新字段 %s 未登记信息契约表(%v 中四选一)", table, f.Name, keysOf(statuses))
		}
	}
	for name, d := range contract {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("%s: 契约表幽灵行 %s(字段已不存在,同步删行)", table, name)
		}
		if !statuses[d.Status] {
			t.Errorf("%s: 字段 %s 状态 %q 不在本表合法集 %v", table, name, d.Status, keysOf(statuses))
		}
		for _, w := range regexp.MustCompile(`W-\d+`).FindAllString(d.Ref, -1) {
			usedW[w] = true
			if _, ok := infoContractExemptions[w]; !ok {
				t.Errorf("%s: 字段 %s 引用未登记豁免 %s", table, name, w)
			}
		}
		for _, om := range regexp.MustCompile(`OM-\d+`).FindAllString(d.Ref, -1) {
			usedOM[om] = true
			if _, ok := infoContractKnownGaps[om]; !ok {
				t.Errorf("%s: 字段 %s 引用未登记 known_gap %s", table, name, om)
			}
		}
		if d.Status == "exempt" && !regexp.MustCompile(`W-\d+`).MatchString(d.Ref) {
			t.Errorf("%s: exempt 行 %s 必须引用 W-# 豁免号", table, name)
		}
		if d.Status == "known_gap" && !regexp.MustCompile(`OM-\d+`).MatchString(d.Ref) {
			t.Errorf("%s: known_gap 行 %s 必须引用 OM-# 立案号", table, name)
		}
	}
}

func keysOf(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

var infoContractT1Statuses = map[string]bool{
	"displayed": true, "internal_gate": true, "exempt": true, "known_gap": true,
}

// infoContractTypesLedgerOMRefs reads the OM-# references out of the
// types-side note-key ledger block (internal/types
// info_contract_notekeys_census_test.go :: infoContractNoteKeyLedger) — the
// surface that carries the note-family half of the known-gap filings. Only
// the ledger var block is scanned, never the whole file (error-message prose
// must not mint references).
func infoContractTypesLedgerOMRefs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../types/info_contract_notekeys_census_test.go")
	if err != nil {
		t.Fatalf("types note-key 账本表面缺失(F3 引用臂断):%v", err)
	}
	src := string(raw)
	start := strings.Index(src, "var infoContractNoteKeyLedger = map[string]string{")
	if start < 0 {
		t.Fatal("types note-key 账本块 infoContractNoteKeyLedger 缺失")
	}
	src = src[start:]
	if end := strings.Index(src, "\n}"); end >= 0 {
		src = src[:end]
	}
	seen := map[string]bool{}
	var out []string
	for _, om := range regexp.MustCompile(`OM-\d+`).FindAllString(src, -1) {
		if !seen[om] {
			seen[om] = true
			out = append(out, om)
		}
	}
	return out
}

var infoContractRankStatuses = map[string]bool{
	"node_mirror": true, "note_consumed": true, "note_displayed": true, "engine_gate": true, "exempt": true, "known_gap": true,
}

// TestInfoContractFieldCensus — T1 registration/ghost/reference arms over all
// contract tables, plus the exemption/known-gap registry completeness checks
// (known_gap 引用集恰=13 OM; 豁免表零幽灵).
func TestInfoContractFieldCensus(t *testing.T) {
	usedW, usedOM := map[string]bool{}, map[string]bool{}
	infoContractCheckRegistration(t, "Node", reflect.TypeOf(types.TraceCausalProjectionNode{}), nodeFieldContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "Projection", reflect.TypeOf(types.TraceCausalProjection{}), projectionFieldContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "TargetStateAccount", reflect.TypeOf(types.TraceCausalProjectionTargetStateAccount{}), targetStateAccountContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "QueryWindow", reflect.TypeOf(types.TraceCausalProjectionQueryWindow{}), queryWindowContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "RankFoldPeer", reflect.TypeOf(runtimeTraceProjRankFoldPeer{}), rankFoldPeerContract, infoContractT1Statuses, usedW, usedOM)
	infoContractCheckRegistration(t, "RankItem", reflect.TypeOf(tracequery.RootCauseRankItem{}), rankItemContract, infoContractRankStatuses, usedW, usedOM)

	// The known-gap reference set must be EXACTLY the 13 OM findings (§29.40
	// acceptance: T1 known_gap 列表恰=13 OM). OM-11 has no wire FIELD (its
	// carriers are the peer_state_*/wait_object/peer_chain_* notes), so its
	// reference comes from the REAL types-side ledger surface — the
	// infoContractNoteKeyLedger block of the note-key census — read
	// mechanically here instead of being decreed (修复轮 F3: "恰13" 不得钦定).
	for _, om := range infoContractTypesLedgerOMRefs(t) {
		usedOM[om] = true
		if _, ok := infoContractKnownGaps[om]; !ok {
			t.Errorf("types 侧 note-key 账本引用了未登记的 %s", om)
		}
	}
	if !usedOM["OM-11"] {
		t.Errorf("OM-11 无任何表面引用(types note-key 账本应承载其 note 族半场)")
	}
	for om := range infoContractKnownGaps {
		if !usedOM[om] {
			t.Errorf("known_gap 登记 %s 无任何契约行引用(幽灵立案)", om)
		}
	}
	for om := range usedOM {
		if _, ok := infoContractKnownGaps[om]; !ok {
			t.Errorf("契约行引用了未登记的 %s", om)
		}
	}
	if len(infoContractKnownGaps) != 13 {
		t.Errorf("known_gap 登记数 %d ≠ 13(§29.40 恰 13 OM)", len(infoContractKnownGaps))
	}
	// Exemption ghost check (field rows + non-field attachment sites).
	for w := range infoContractNonFieldExemptionSites {
		usedW[w] = true
	}
	for w := range infoContractExemptions {
		if !usedW[w] {
			t.Errorf("豁免登记 %s 无任何契约行/表面引用(幽灵豁免)", w)
		}
	}
	// R5 (§29.88.12, 2026-07-15): +W-22 — the demoted-reference word family
	// retired, SupplyFoldReferenceClass moved displayed→exempt under it.
	// XLANE-2 件3 (2026-07-17): +W-23 — the absorbed-demoted account memory
	// (compile-side fold-key judge, word-less by design).
	if len(infoContractExemptions) != 23 {
		t.Errorf("豁免登记数 %d ≠ 23(§29.40 全裁决 + R5 W-22 + XLANE-2 W-23)", len(infoContractExemptions))
	}
}

// TestInfoContractDisplayedClaimsHaveRealConsumers — T1 第二/第三臂 (杀
// OM-6/OM-10/W-2 假指针病 + 悄悄修而不销账): displayed/internal_gate rows
// must have a real token in the display authority; known_gap rows must NOT.
func TestInfoContractDisplayedClaimsHaveRealConsumers(t *testing.T) {
	src := readDisplayAuthoritySources(t)
	check := func(table string, contract map[string]fieldDisposition) {
		for name, d := range contract {
			if d.NoScan {
				continue
			}
			switch d.Status {
			case "displayed", "internal_gate":
				if !identTokenAppears(src, name, d.Token) {
					t.Errorf("%s.%s 声明 %s 但显示权威零引用(假指针;Ref=%s)", table, name, d.Status, d.Ref)
				}
			case "known_gap":
				if identTokenAppears(src, name, d.Token) {
					t.Errorf("%s.%s 已获显示权威引用,契约表须翻 displayed 并销 %s(账实一致纪律)", table, name, d.Ref)
				}
			}
		}
	}
	check("Node", nodeFieldContract)
	check("Projection", projectionFieldContract)
	check("TargetStateAccount", targetStateAccountContract)
	check("QueryWindow", queryWindowContract)
	check("RankFoldPeer", rankFoldPeerContract)
}

// TestInfoContractRankItemWireDisposition — T2 armB: node-mirror existence,
// known-gap reverse tripwire, note registration, engine-gate presence.
func TestInfoContractRankItemWireDisposition(t *testing.T) {
	nodeType := reflect.TypeOf(types.TraceCausalProjectionNode{})
	projType := reflect.TypeOf(types.TraceCausalProjection{})
	mirrorExists := func(field string) (string, bool) {
		if mapped, ok := rankItemNodeMirror[field]; ok {
			if _, ok := nodeType.FieldByName(mapped); ok {
				return mapped, true
			}
			if _, ok := projType.FieldByName(mapped); ok {
				return mapped, true
			}
			return mapped, false
		}
		for _, candidate := range []string{field, strings.TrimSuffix(field, "Ms") + "MS"} {
			if _, ok := nodeType.FieldByName(candidate); ok {
				return candidate, true
			}
		}
		return field, false
	}
	engineSrc := readTracequeryEngineSources(t)
	for name, d := range rankItemContract {
		switch d.Status {
		case "node_mirror":
			if mapped, ok := mirrorExists(name); !ok {
				t.Errorf("RankItem.%s 声明 node_mirror 但投影无 %s 字段(断链)", name, mapped)
			}
		case "known_gap":
			if mapped, ok := mirrorExists(name); ok {
				t.Errorf("RankItem.%s 已获投影镜像 %s,契约表须翻 node_mirror 并销 %s", name, mapped, d.Ref)
			}
		case "note_consumed":
			key := strings.TrimPrefix(d.Ref, "note:")
			key = strings.FieldsFunc(key, func(r rune) bool { return r == ' ' || r == '(' })[0]
			row, ok := types.TraceNoteKeyLookup(key)
			if !ok {
				t.Errorf("RankItem.%s 声明 note_consumed 但键 %q 未注册", name, key)
			} else if row.Carrier == types.TraceNoteCarrierDisplayOnly {
				t.Errorf("RankItem.%s 的载体键 %q 是 display_only(无解析消费者)——note_consumed 声明为假", name, key)
			}
		case "note_displayed":
			key := strings.TrimPrefix(d.Ref, "note:")
			key = strings.FieldsFunc(key, func(r rune) bool { return r == ' ' || r == '(' })[0]
			row, ok := types.TraceNoteKeyLookup(key)
			if !ok {
				t.Errorf("RankItem.%s 声明 note_displayed 但键 %q 未注册", name, key)
			} else if row.Carrier != types.TraceNoteCarrierDisplayOnly {
				t.Errorf("RankItem.%s 的载体键 %q 不是 display_only——note_displayed 声明为假", name, key)
			}
		case "engine_gate":
			if !identTokenAppears(engineSrc, name, d.Token) {
				t.Errorf("RankItem.%s 声明 engine_gate 但 tracequery 非测试源零引用", name)
			}
		}
	}
}
