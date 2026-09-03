package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalize_loop_gate_advisory.go — the cluster-closure exit reachability
// advisory (§40.43 R1 finding C; rewritten under §40.43 F-orch 三轮复核
// finding S). The finalize retry loop evaluates, in this order and all
// BEFORE AdvanceRepairExecutionPlan:
//
//  1. P6 finalize repair hard cap        retryUsed >= hardCap
//  2. W2.6 per-root attempt cap          every actionable root at perRootCap
//  3. template retry budget              retryUsed >= ExecutionPolicy.RetryBudget
//  4. per-kind retry cap                 retryUsedForKind(dominant) >= cap
//  5. same-error-class governor          retryUsedForClass(dominant) >= classCap
//  6. low-yield kill                     evidence delta < min_retry_yield
//
// The cluster-closure fail-loud exit inside Advance fires on failure
// number ClusterStableBudget()+1 of a chain in which the same cluster
// persists. Each gate above ships the answer (with caveats) on a failure
// number of its own; when that number is at or before the exit's, the
// gate pre-empts the exit. Pre-emption is TOTAL when it holds for every
// such chain, and CONDITIONAL when it depends on a qualifier the advisory
// states verbatim. The earlier form of this advisory modelled only P6 and
// W2.6 and was therefore silent at the defaults while the same-error-class
// governor shipped on failure 2. ADVISORY ONLY — a log line, never a gate.
//
// Failure arithmetic (n = 1-based finalize contract failure in the chain,
// r = retries recorded on the scheduler outside finalize failures — explore
// fact retries, structurally-empty requeues, floor requeues): retryUsed at
// failure n is n-1+r, so a `retryUsed >= cap` gate fires on failure
// cap+1-r; the per-kind / per-class counters count finalize retries only
// and fire on failure cap+1. A template RetryBudget of 0 or less is total
// pre-emption on failure 1, exactly as the loop reads it (§40.43 F-orch 四轮
// 复核 finding X: `retryUsed >= 0` already holds; P6 defaults when unset).

// finalizeLoopCaps is the resolved set of bounds the advisory reads.
type finalizeLoopCaps struct {
	StableBudget        int
	HardCap             int
	PerRootCap          int
	ClassCap            int
	TemplateRetryBudget int
	MinRetryYield       int
	// KindCaps holds the explicitly configured per-kind caps (settings
	// field name → cap); unconfigured fields fall back to the template
	// budget and are covered by gate 3.
	KindCaps map[string]int
}

// finalizeLoopGate is one pre-empting gate as the advisory states it.
// Total: pre-empts every chain that reaches the exit failure. Otherwise the
// qualifier names the chain property the pre-emption depends on;
// OutsideRetryOnly marks the retryUsed-counting gates whose cap is above
// the cluster budget — they pre-empt only when retries recorded OUTSIDE the
// finalize chain (explore fact retries, requeues) make up the difference,
// so they are stated but never decide the verdict on their own.
type finalizeLoopGate struct {
	Name             string
	LoopOrder        int
	FiresOnFailure   int
	Total            bool
	OutsideRetryOnly bool
	Qualifier        string
}

// clusterExitVerdict is the advisory's conclusion.
type clusterExitVerdict string

const (
	clusterExitReachable   clusterExitVerdict = "reachable"
	clusterExitUnreachable clusterExitVerdict = "unreachable"
	clusterExitConditional clusterExitVerdict = "conditionally unreachable"
)

// clusterExitReachability is the advisory's typed result.
type clusterExitReachability struct {
	Verdict     clusterExitVerdict
	ExitFailure int
	// PreEmpting lists every gate that fires at or before ExitFailure,
	// ordered by failure number, total before conditional before
	// outside-retry-only, then loop order.
	PreEmpting []finalizeLoopGate
	Reason     string
}

// clusterClosureExitReachability evaluates every gate that precedes
// AdvanceRepairExecutionPlan against the cluster exit's failure number.
func clusterClosureExitReachability(caps finalizeLoopCaps) clusterExitReachability {
	exit := caps.StableBudget + 1
	out := clusterExitReachability{Verdict: clusterExitReachable, ExitFailure: exit}
	add := func(g finalizeLoopGate) {
		if g.FiresOnFailure <= exit {
			out.PreEmpting = append(out.PreEmpting, g)
		}
	}
	retryUsedGate := func(name string, order, cap int) {
		if cap <= 0 {
			return
		}
		if cap <= caps.StableBudget {
			add(finalizeLoopGate{Name: name, LoopOrder: order, FiresOnFailure: cap + 1, Total: true,
				Qualifier: "counts every scheduler retry; fires no later than failure " + fmt.Sprint(cap+1)})
			return
		}
		add(finalizeLoopGate{Name: name, LoopOrder: order, FiresOnFailure: exit, OutsideRetryOnly: true,
			Qualifier: fmt.Sprintf("only when at least %d retr%s were recorded outside the finalize chain (explore fact retries / structurally-empty or floor requeues)",
				cap-caps.StableBudget, plural(cap-caps.StableBudget, "y", "ies"))})
	}
	retryUsedGate("P6 finalize repair hard cap "+fmt.Sprint(caps.HardCap), 1, caps.HardCap)
	if caps.PerRootCap > 0 {
		add(finalizeLoopGate{Name: "W2.6 per-root attempt cap " + fmt.Sprint(caps.PerRootCap), LoopOrder: 2, FiresOnFailure: caps.PerRootCap,
			Qualifier: "only while the actionable root set is unchanged since the chain began (a new root keeps the run open)"})
	}
	if caps.TemplateRetryBudget <= 0 {
		// Finding X: a non-positive template budget is exhausted on the first failure (retryUsed 0 >= 0) — never "no gate".
		add(finalizeLoopGate{Name: "template retry budget " + fmt.Sprint(caps.TemplateRetryBudget), LoopOrder: 3, FiresOnFailure: 1, Total: true,
			Qualifier: "a template retry budget of 0 or less is exhausted before the first retry; the first contract failure ships"})
	} else {
		retryUsedGate("template retry budget "+fmt.Sprint(caps.TemplateRetryBudget), 3, caps.TemplateRetryBudget)
	}
	kindNames := make([]string, 0, len(caps.KindCaps))
	for name := range caps.KindCaps {
		kindNames = append(kindNames, name)
	}
	sort.Strings(kindNames)
	for _, name := range kindNames {
		if cap := caps.KindCaps[name]; cap > 0 {
			add(finalizeLoopGate{Name: fmt.Sprintf("per-kind retry cap %s=%d", name, cap), LoopOrder: 4, FiresOnFailure: cap + 1,
				Qualifier: "only when the dominant violation kind is one this cap covers and every round was billed to the kind ledger (answer-rewrite-only downgrade rounds are exempt)"})
		}
	}
	if caps.ClassCap > 0 {
		add(finalizeLoopGate{Name: "same-error-class governor cap " + fmt.Sprint(caps.ClassCap), LoopOrder: 5, FiresOnFailure: caps.ClassCap + 1,
			Qualifier: "only while the dominant retry class is unchanged across the chain (always true for a single stuck cluster)"})
	}
	if caps.MinRetryYield > 0 {
		add(finalizeLoopGate{Name: "low-yield kill min_retry_yield=" + fmt.Sprint(caps.MinRetryYield), LoopOrder: 6, FiresOnFailure: 2,
			Qualifier: "only when a round that re-runs an upstream stage yields an evidence delta below the threshold"})
	}
	sort.SliceStable(out.PreEmpting, func(i, j int) bool {
		a, b := out.PreEmpting[i], out.PreEmpting[j]
		if a.FiresOnFailure != b.FiresOnFailure {
			return a.FiresOnFailure < b.FiresOnFailure
		}
		if a.Total != b.Total {
			return a.Total
		}
		if a.OutsideRetryOnly != b.OutsideRetryOnly {
			return !a.OutsideRetryOnly
		}
		return a.LoopOrder < b.LoopOrder
	})
	for _, g := range out.PreEmpting {
		if g.Total {
			out.Verdict = clusterExitUnreachable
			break
		}
		if !g.OutsideRetryOnly {
			out.Verdict = clusterExitConditional
		}
	}
	out.Reason = out.render()
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (r clusterExitReachability) render() string {
	if len(r.PreEmpting) == 0 {
		return fmt.Sprintf("%s: no gate precedes the cluster exit on failure %d", r.Verdict, r.ExitFailure)
	}
	parts := make([]string, 0, len(r.PreEmpting))
	for _, g := range r.PreEmpting {
		mode := "conditional"
		switch {
		case g.Total:
			mode = "total"
		case g.OutsideRetryOnly:
			mode = "outside-chain retries only"
		}
		parts = append(parts, fmt.Sprintf("%s ships on failure %d (%s: %s)", g.Name, g.FiresOnFailure, mode, g.Qualifier))
	}
	return fmt.Sprintf("%s — the cluster exit needs failure %d; %s", r.Verdict, r.ExitFailure, strings.Join(parts, "; "))
}

// finalizeLoopCaps resolves the live bounds for this run: the per-run P6
// cap and settings, the package-level cluster / per-root / class knobs,
// and the IR's template retry budget.
func (o *Orchestrator) finalizeLoopCaps(ir *types.AnalysisIR) finalizeLoopCaps {
	caps := finalizeLoopCaps{
		StableBudget: ClusterStableBudget(),
		HardCap:      o.finalizeRepairHardCapValue(),
		PerRootCap:   maxRepairAttemptsPerRootValue,
		ClassCap:     sameErrorClassRetryCap(),
	}
	if ir != nil {
		caps.TemplateRetryBudget = ir.TaskGraph.ExecutionPolicy.RetryBudget
	}
	if o != nil {
		caps.MinRetryYield = o.settings.ViolationBudget.MinRetryYield
		byKind := o.settings.RetryBudgetByKind
		for name, cap := range map[string]int{
			"shape_violation":     byKind.ShapeViolation,
			"citation_violation":  byKind.CitationViolation,
			"literal_form_failed": byKind.LiteralFormFailed,
			"ghost_anchor":        byKind.GhostAnchor,
			"self_ref_literal":    byKind.SelfRefLiteral,
			"other":               byKind.Other,
		} {
			if cap > 0 {
				if caps.KindCaps == nil {
					caps.KindCaps = map[string]int{}
				}
				caps.KindCaps[name] = cap
			}
		}
	}
	return caps
}

// logClusterClosureExitReachabilityOnce emits the advisory once per
// Orchestrator at scheduler entry (the first point where the per-run hard
// cap, the package-level knobs and the IR's template budget are all
// resolved). Returns the advisory text and whether it was emitted by this
// call, for the pin. Nothing is emitted when the exit is reachable.
func (o *Orchestrator) logClusterClosureExitReachabilityOnce(ir *types.AnalysisIR) (string, bool) {
	if o == nil || o.clusterClosureExitAdvisoryLogged {
		return "", false
	}
	o.clusterClosureExitAdvisoryLogged = true
	r := clusterClosureExitReachability(o.finalizeLoopCaps(ir))
	if r.Verdict == clusterExitReachable {
		return "", false
	}
	msg := "[orchestrator] advisory: cluster-closure fail-loud exit " + r.Reason
	logging.Info("%s", msg)
	return msg, true
}
