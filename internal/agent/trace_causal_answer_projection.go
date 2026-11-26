package agent

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	traceClaimClause        = regexp.MustCompile(`[^。；！？\r\n|]+`)
	traceHolderClaim        = regexp.MustCompile(`(?i)持(?:有者(?:是|为|长时间不释放)|锁[方者](?:是|为|在|执行)|锁阻塞|有的.{0,12}锁)|锁的持有方|占着锁|(?:the )?(?:lock )?holder (?:is|was|held)`)
	traceWaiterClaim        = regexp.MustCompile(`(?i)阻塞等待方|等待方(?:是|为)|(?:列为|标为).{0,8}等待方|(?:the )?(?:lock )?waiter (?:is|was)`)
	traceFrameCauseLead     = regexp.MustCompile(`(?i)真正(?:压住|拖长|耽误|卡住).{0,12}?(?:的[，,]?\s*是|的是)|what actually (?:delayed|blocked) the frame was`)
	traceFrameAttribution   = regexp.MustCompile(`(?i)共同拉长|直接导致.{0,12}掉帧|拖长(?:了)?(?:本|这[一]?)帧|(?:使|令)(?:本|这[一]?)帧(?:从|.{0,45}(?:延迟|耗时|横跨|掉帧))|directly caused (?:this |the )?frame (?:delay|miss)`)
	traceWaitClaim          = regexp.MustCompile(`(?i)(?:等待|等着).{0,90}(?:完成|释放)|(?:被|由).{0,55}(?:阻塞|拖着等|卡住)|唤醒(?:依赖|关系).{0,12}(?:导致|造成|撑起)|超支主要来自.{0,40}等待链|(?:waited|waiting) for .{1,80}(?:finish|complete|release)|(?:was |were )?blocked by`)
	traceOverlapClaim       = regexp.MustCompile(`(?i)(?:重叠|重合|overlap).{0,85}(?:导致|造成|证明|指向|caused|proves?|establishes?).{0,60}(?:阻塞|掉帧|持锁|持有者|等待|block|jank|wait|holder)`)
	traceClaimQualification = regexp.MustCompile(`(?i)不证明|不能证明|不足以证明|无法(?:证明|确认)|没有.{0,12}(?:证据|直接绑定)|未证实|未验证|仍待|未确认|尚未|不能说|可能是|(?:does|do|did) not (?:prove|establish)|unproven|not established|no (?:direct )?evidence`)
	traceOwnerLiteral       = regexp.MustCompile(`(?i)owner\s+tid\s*[:=]?\s*(\d+)`)
)

type traceAnswerLockWitness struct {
	owner, peer string
	start, end  float64
	confirmed   bool
}

// projectTraceCausalProse is a report view. It never changes the stored
// document, rendered evidence bindings, or model-owned audit/repair base.
// Phrase recognition selects a claim to check, never evidence for that claim.
func projectTraceCausalProse(ctx *types.AgentContext, doc *types.AnswerDocumentV2, lang string) (*types.AnswerDocumentV2, int) {
	if ctx == nil || doc == nil {
		return doc, 0
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	if rm == nil {
		return doc, 0
	}
	scope := rm.RuntimeArtifactScopeProfile
	start, end, bounded := scope.ExplicitTimeWindow()
	frameQuestion := rm.RuntimeQuestionProfile != nil && rm.RuntimeQuestionProfile.FrameCausalityRequested
	if scope != nil && scope.Comparison != nil || !frameQuestion && ctx.TraceRootCauseBoard == "" {
		return doc, 0
	}
	source := ctx.AttachedHitraceSource
	if ctx.AttachedTraceMaterial != nil {
		source = ctx.AttachedTraceMaterial.QueryPath()
	}
	if source == "" && ctx.TraceRootCauseBoard == "" {
		return doc, 0
	}
	if bounded && len(rm.RuntimeTargets) > 1 {
		if fact, ok := ctx.AttachedLogCatalog.PreparedFrameFact(); ok &&
			types.TraceCausalProjectionPrincipalValueSameWindow(start, end, float64(fact.StartNS)/1e9, float64(fact.EndNS)/1e9) {
			primary := *rm
			primary.RuntimeTargets = nil
			for _, target := range rm.RuntimeTargets {
				if target.Kind == types.RuntimeTargetKindThread && target.PID == fact.TID {
					primary.RuntimeTargets = append(primary.RuntimeTargets, target)
				}
			}
			rm = &primary
		}
	}
	ledger := answerDocObservationLedger(ctx)
	ledger.Records, _ = answerDocSelectedWindowObservationRecords(ctx, ledger.Records)
	rows := make([]types.ObservationRecord, 0, len(ledger.Records))
	for _, record := range ledger.Records {
		if bounded && source != "" && len(rm.RuntimeTargets) == 1 &&
			types.RuntimeArtifactComparisonPathEqual(record.SourceRef.Path, source, ctx.RepoRoot) {
			rows = append(rows, record)
		}
	}
	ledger.Records = rows
	wakers := map[string]bool{}
	for _, edge := range types.BuildTraceWakeupEdgeRoleAuthorities(ledger, rm) {
		wakers[edge.Waker] = true
	}
	logging.Debug("[finalizer] trace causal projection frame=%t window=%t targets=%d scoped_records=%d wakers=%d",
		frameQuestion, bounded, len(rm.RuntimeTargets), len(rows), len(wakers))
	var locks []traceAnswerLockWitness
	for _, record := range ledger.Records {
		if record.Predicate != "critical_blocking" || record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) || record.GroundingPolicy != types.ClaimGroundingHard ||
			!types.ObservationRecordMatchesUserRuntimeTarget(record, rm) ||
			record.Span.EndTs <= record.Span.StartTs || record.Span.StartTs < start || record.Span.EndTs > end {
			continue
		}
		notes := make(map[string]string)
		consistent := true
		for _, note := range record.RichNotes {
			if key, value, ok := strings.Cut(note, "="); ok {
				if existing, seen := notes[key]; !seen || existing == value {
					notes[key] = value
				} else {
					consistent = false
				}
			}
		}
		switch notes[types.TraceNoteKeyBlockingKind] {
		case "monitor_contention", "lock_contention":
		default:
			continue
		}
		origin := notes[types.TraceNoteKeyHolderSource]
		locks = append(locks, traceAnswerLockWitness{
			owner: notes[types.TraceNoteKeyOwnerTidRaw], peer: record.Object,
			start: record.Span.StartTs, end: record.Span.EndTs,
			confirmed: consistent && (origin == tracequery.CounterpartSourceContentionPayload ||
				(origin == tracequery.CounterpartSourceNsSpanDerivation && notes[types.TraceNoteKeyHolderNsUnification] != "")) &&
				notes[types.TraceNoteKeyHolderHandoff] == "" && notes[types.TraceNoteKeyHolderSelfContradiction] == "",
		})
	}
	zh := strings.HasPrefix(strings.ToLower(lang), "zh")
	count := 0
	project := func(text string, verifiedFrameWork *types.AnswerRuntimeWorkRelationReceipt) string {
		lockContext := strings.Contains(text, "锁") || strings.Contains(strings.ToLower(text), "lock")
		return traceClaimClause.ReplaceAllStringFunc(text, func(clause string) string {
			holder := tracePositiveClaim(traceHolderClaim, clause)
			waiter := tracePositiveClaim(traceWaiterClaim, clause) &&
				(lockContext || strings.Contains(clause, "阻塞等待方"))
			wait := tracePositiveClaim(traceWaitClaim, clause)
			overlap := tracePositiveClaim(traceOverlapClaim, clause)
			frameLead := tracePositiveClaim(traceFrameCauseLead, clause)
			frame := frameLead || tracePositiveClaim(traceFrameAttribution, clause)
			span := (strings.Contains(clause, "阻塞跨度") || strings.Contains(clause, "的锁竞争片段")) &&
				!traceClaimQualification.MatchString(clause)
			if !(holder || waiter || wait || overlap || frame || span) {
				return clause
			}
			var actor string
			var namedWaker bool
			for waker := range wakers {
				if waker != "" && strings.Contains(clause, waker) {
					if namedWaker {
						actor = ""
						break
					}
					actor, namedWaker = waker, true
				}
			}
			owner := traceOwnerLiteral.FindStringSubmatch(clause)
			// A rank row can use the inferred peer as its subject. That
			// cannot move the native waiter interval to the peer.
			waitIntervalAttribution := span &&
				(namedWaker || strings.Contains(clause, "定位线程") || strings.Contains(clause, "该线程")) &&
				(len(owner) == 2 || strings.Contains(clause, "该锁") || strings.Contains(clause, "ClassLinker") ||
					strings.Contains(clause, "的锁竞争片段"))
			if holder && !waiter && !waitIntervalAttribution && len(owner) == 2 && !overlap && !frame {
				for _, lock := range locks {
					if lock.confirmed && lock.peer != "" && owner[1] == lock.owner &&
						strings.Contains(clause, lock.peer) && strings.Contains(clause, fmt.Sprintf("%.6f", lock.start)) &&
						strings.Contains(clause, fmt.Sprintf("%.6f", lock.end)) {
						return clause
					}
				}
			}
			if holder || waiter || waitIntervalAttribution {
				count++
				if waiter || waitIntervalAttribution {
					if zh {
						return "该锁的持有者、等待方配对及等待时长归属未证实"
					}
					return "The lock holder, waiter pairing and interval attribution remain unverified"
				}
				if zh {
					return "当前 Trace 证据下，锁持有者身份未证实"
				}
				return "The lock holder identity remains unverified by the current Trace evidence"
			}
			if frame {
				if verifiedFrameWork != nil && strings.Contains(clause, verifiedFrameWork.BoundRow.Subject) &&
					strings.Contains(clause, verifiedFrameWork.BoundRow.WorkLabel) {
					return clause
				}
				count++
				if zh && frameLead {
					return traceFrameCauseLead.ReplaceAllString(clause, "本窗口内的待验证候选包括")
				}
				if frameLead {
					return traceFrameCauseLead.ReplaceAllString(clause, "A candidate within this window is")
				}
				if zh {
					return "相关工作可能影响本帧，直接帧因果未证实"
				}
				return "The observed work remains a candidate; direct frame causality is unverified"
			}
			if wait && namedWaker {
				count++
				if actor == "" {
					if zh {
						return "等待对象和线程间的完成依赖未证实"
					}
					return "The wait object and completion dependency remain unverified"
				}
				if zh {
					return fmt.Sprintf("Trace 观测到 %s 发出的唤醒，等待对象及其与该线程工作完成的关系未证实", actor)
				}
				return fmt.Sprintf("Trace records a wakeup from %s; the wait object and any link to work completion remain unverified", actor)
			}
			if !overlap {
				return clause
			}
			count++
			if zh {
				return "时间重叠无法单独证实直接阻塞或掉帧因果"
			}
			return "Timing overlap alone leaves direct blocking and frame causality unverified"
		})
	}
	out := *doc
	out.Blocks = slices.Clone(doc.Blocks)
	for i := range out.Blocks {
		block := &out.Blocks[i]
		if block.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
			continue
		}
		var provenWork *types.AnswerRuntimeWorkRelationReceipt
		if work := block.RuntimeWorkRelation; work.IsBound() && work.Conclusion == types.RuntimeWorkRelationConclusionCausalContributionSupported &&
			block.TraceCausalClaimCaliber == types.TraceCausalClaimTypedFrame && work.BoundRow.Subject != "" && work.BoundRow.WorkLabel != "" {
			provenWork = work
		}
		block.Title, block.Text = project(block.Title, provenWork), project(block.Text, provenWork)
		block.Items = slices.Clone(block.Items)
		for j := range block.Items {
			item := &block.Items[j]
			item.Label, item.Text = project(item.Label, provenWork), project(item.Text, provenWork)
			item.Cells = slices.Clone(item.Cells)
			for k := range item.Cells {
				item.Cells[k] = project(item.Cells[k], provenWork)
			}
		}
	}
	out.Caveats = slices.Clone(doc.Caveats)
	for i := range out.Caveats {
		out.Caveats[i] = project(out.Caveats[i], nil)
	}
	return &out, count
}

func tracePositiveClaim(pattern *regexp.Regexp, clause string) bool {
	for _, match := range pattern.FindAllStringIndex(clause, -1) {
		prefix := []rune(clause[:match[0]])
		if len(prefix) > 16 {
			prefix = prefix[len(prefix)-16:]
		}
		if !traceClaimQualification.MatchString(string(prefix) + clause[match[0]:match[1]]) {
			return true
		}
	}
	return false
}
